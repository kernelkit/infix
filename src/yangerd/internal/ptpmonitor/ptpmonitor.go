// Package ptpmonitor keeps the ieee1588-ptp-tt subtree in sync with the
// running ptp4l instances.  One ptp4l process runs per instance-index,
// with its config at /etc/linuxptp/ptp4l-<idx>.conf and its management
// (UDS) socket at /var/run/ptp4l-<idx>.
//
// Instead of polling, each instance connection subscribes to ptp4l's
// push notifications (SUBSCRIBE_EVENTS_NP): port state transitions
// deliver PORT_DATA_SET, every clock update delivers TIME_STATUS_NP,
// and BMCA reselection delivers PARENT_DATA_SET.  The near-static data
// sets are fetched at connect and refreshed on a slow timer, which also
// renews the subscription.
package ptpmonitor

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	ptp "github.com/facebook/time/ptp/protocol"
	"github.com/kernelkit/infix/src/yangerd/internal/backoff"
	"github.com/kernelkit/infix/src/yangerd/internal/tree"
)

const (
	treeKey = "ieee1588-ptp-tt:ptp"

	// scanInterval is how often the set of configured instances is
	// re-discovered.  confd (re)writes the config files and
	// (re)starts ptp4l on configuration changes.
	scanInterval = 10 * time.Second

	// refreshInterval bounds the staleness of the data sets that
	// have no notification (default-ds, time-properties-ds, port
	// statistics), and paces subscription renewal.
	refreshInterval = 30 * time.Second

	// subscribeDuration must comfortably exceed refreshInterval so
	// the subscription never lapses between renewals.
	subscribeDuration = 180 /* seconds */
)

// Overridable in tests.
var (
	confDir = "/etc/linuxptp"
	sockDir = "/var/run"
)

var confRe = regexp.MustCompile(`ptp4l-(\d+)\.conf$`)

// PTPMonitor supervises one connection per running ptp4l instance and
// publishes the combined operational state.
type PTPMonitor struct {
	tree *tree.Tree
	log  *slog.Logger

	mu        sync.Mutex
	instances map[uint16]*instance
}

// New creates a PTPMonitor.
func New(t *tree.Tree, log *slog.Logger) *PTPMonitor {
	if log == nil {
		log = slog.Default()
	}
	return &PTPMonitor{
		tree:      t,
		log:       log,
		instances: make(map[uint16]*instance),
	}
}

// Run discovers ptp4l instances and supervises their connections until
// ctx is cancelled.
func (m *PTPMonitor) Run(ctx context.Context) error {
	tick := time.NewTicker(scanInterval)
	defer tick.Stop()

	for {
		m.scan(ctx)
		select {
		case <-ctx.Done():
			m.mu.Lock()
			for _, inst := range m.instances {
				inst.stop()
			}
			m.mu.Unlock()
			return ctx.Err()
		case <-tick.C:
		}
	}
}

// scan reconciles the running instance connections with the configured
// ptp4l instances.
func (m *PTPMonitor) scan(ctx context.Context) {
	confs, _ := filepath.Glob(confDir + "/ptp4l-*.conf")

	found := make(map[uint16]string)
	for _, conf := range confs {
		match := confRe.FindStringSubmatch(conf)
		if match == nil {
			continue
		}
		var idx uint16
		fmt.Sscanf(match[1], "%d", &idx)
		found[idx] = conf
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	for idx, inst := range m.instances {
		if _, ok := found[idx]; !ok {
			inst.stop()
			delete(m.instances, idx)
		}
	}

	changed := false
	for idx, conf := range found {
		if _, ok := m.instances[idx]; ok {
			continue
		}
		inst := newInstance(idx, conf, m.log, m.publish)
		m.instances[idx] = inst
		go inst.run(ctx)
		changed = true
	}

	if changed || len(found) < len(m.instances) {
		m.publishLocked()
	}
}

// publish rebuilds the whole subtree from the current instance states.
func (m *PTPMonitor) publish() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.publishLocked()
}

func (m *PTPMonitor) publishLocked() {
	var idxs []int
	for idx := range m.instances {
		idxs = append(idxs, int(idx))
	}
	sort.Ints(idxs)

	var list []interface{}
	for _, idx := range idxs {
		if data := m.instances[uint16(idx)].snapshot(); data != nil {
			list = append(list, data)
		}
	}

	if len(list) == 0 {
		m.tree.Delete(treeKey)
		return
	}

	data, err := json.Marshal(map[string]interface{}{
		"instances": map[string]interface{}{
			"instance": list,
		},
	})
	if err != nil {
		return
	}
	m.tree.Set(treeKey, data)
}

// instance is one supervised ptp4l management connection.
type instance struct {
	idx     uint16
	conf    string
	sock    string
	log     *slog.Logger
	publish func()
	cancel  context.CancelFunc

	mu    sync.Mutex
	state *instanceState
}

// instanceState holds the last known data sets for one instance.
type instanceState struct {
	instanceType string
	ifaces       []string

	defaultDS *ptp.DefaultDataSetTLV
	currentDS *ptp.CurrentDataSetTLV
	parentDS  *ptp.ParentDataSetTLV
	timeProps *timePropertiesDataSetTLV
	ports     map[uint16]*portDataSetTLV
	portStats map[uint16]*ptp.PortStatsNPTLV
}

func newInstance(idx uint16, conf string, log *slog.Logger, publish func()) *instance {
	return &instance{
		idx:     idx,
		conf:    conf,
		sock:    fmt.Sprintf("%s/ptp4l-%d", sockDir, idx),
		log:     log.With("ptp-instance", idx),
		publish: publish,
	}
}

func (i *instance) stop() {
	if i.cancel != nil {
		i.cancel()
	}
}

// snapshot renders the current state as a YANG instance list entry, or
// nil when the instance has no data (ptp4l not answering).
func (i *instance) snapshot() map[string]interface{} {
	i.mu.Lock()
	defer i.mu.Unlock()

	s := i.state
	if s == nil || s.defaultDS == nil {
		return nil
	}

	inst := map[string]interface{}{
		"instance-index": int(i.idx),
		"default-ds":     defaultDSMap(s.defaultDS, s.instanceType),
	}
	if s.currentDS != nil {
		inst["current-ds"] = currentDSMap(s.currentDS)
	}
	if s.parentDS != nil {
		inst["parent-ds"] = parentDSMap(s.parentDS)
	}
	if s.timeProps != nil {
		inst["time-properties-ds"] = timePropertiesDSMap(s.timeProps)
	}

	var nums []int
	for num := range s.ports {
		nums = append(nums, int(num))
	}
	sort.Ints(nums)

	var ports []interface{}
	for _, num := range nums {
		pd := s.ports[uint16(num)]
		entry := map[string]interface{}{
			"port-index": num,
			"port-ds":    portDSMap(pd),
		}
		if num >= 1 && num <= len(s.ifaces) {
			entry["underlying-interface"] = s.ifaces[num-1]
		}
		if stats, ok := s.portStats[uint16(num)]; ok {
			entry["ieee802-dot1as-gptp:port-statistics-ds"] = portStatsMap(stats)
		}
		ports = append(ports, entry)
	}
	if len(ports) > 0 {
		inst["ports"] = map[string]interface{}{"port": ports}
	}

	return inst
}

// run supervises the connection: connect, fill, subscribe, consume
// events, and reconnect with backoff when ptp4l goes away.
func (i *instance) run(ctx context.Context) {
	ctx, i.cancel = context.WithCancel(ctx)
	defer i.cancel()

	bo := backoff.Default()
	delay := bo.Initial

	for {
		if ctx.Err() != nil {
			return
		}

		err := i.session(ctx)
		if ctx.Err() != nil {
			return
		}
		i.log.Debug("ptp: session ended, reconnecting", "err", err, "delay", delay)

		// Drop stale state so a dead ptp4l disappears from
		// operational instead of lingering.
		i.mu.Lock()
		i.state = nil
		i.mu.Unlock()
		i.publish()

		if err := backoff.Sleep(ctx, delay); err != nil {
			return
		}
		delay = bo.Next(delay)
	}
}

// session runs one connected episode against ptp4l.
func (i *instance) session(ctx context.Context) error {
	local := &net.UnixAddr{
		Name: fmt.Sprintf("%s/yangerd-ptp-%d.sock", sockDir, i.idx),
		Net:  "unixgram",
	}
	remote := &net.UnixAddr{Name: i.sock, Net: "unixgram"}

	os.Remove(local.Name) /* stale socket from a crashed run */
	conn, err := net.DialUnix("unixgram", local, remote)
	if err != nil {
		return err
	}
	defer func() {
		conn.Close()
		os.Remove(local.Name)
	}()

	// ptp4l runs unprivileged and must be able to send replies here
	if err := os.Chmod(local.Name, 0666); err != nil {
		return err
	}

	// Terminate the blocking read loop on shutdown
	go func() {
		<-ctx.Done()
		conn.SetReadDeadline(time.Now())
	}()

	// gPTP (802.1AS) instances only accept management messages
	// carrying their transport-specific nibble
	sdoID := transportSpecificFromConf(i.conf)

	i.mu.Lock()
	i.state = &instanceState{
		instanceType: instanceTypeFromConf(i.conf),
		ifaces:       portInterfaces(i.conf),
		ports:        make(map[uint16]*portDataSetTLV),
		portStats:    make(map[uint16]*ptp.PortStatsNPTLV),
	}
	i.mu.Unlock()

	var seq uint16
	send := func(pkt *ptp.Management) error {
		seq++
		pkt.SetSequence(seq)
		b, err := pkt.MarshalBinary()
		if err != nil {
			return err
		}
		_, err = conn.Write(b)
		return err
	}

	refresh := func() error {
		pkts := getRequests(sdoID)
		pkts = append(pkts, subscribeRequest(sdoID, subscribeDuration,
			notifyPortState, notifyTimeSync, notifyParentDataSet))
		for _, pkt := range pkts {
			if err := send(pkt); err != nil {
				return err
			}
		}
		return nil
	}

	if err := refresh(); err != nil {
		return err
	}

	buf := make([]byte, 2048)
	lastData := time.Now()
	for {
		conn.SetReadDeadline(time.Now().Add(refreshInterval))
		n, err := conn.Read(buf)
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if err != nil {
			var nerr net.Error
			if errors.As(err, &nerr) && nerr.Timeout() {
				// Quiet period: refresh slow data sets and
				// renew the subscription.  Give up if ptp4l
				// has not answered anything for a while.
				if time.Since(lastData) > 3*refreshInterval {
					return fmt.Errorf("ptp4l not responding")
				}
				if err := refresh(); err != nil {
					return err
				}
				continue
			}
			return err
		}

		tlv, err := decodePacket(buf[:n])
		if err != nil {
			var merr *mgmtError
			if errors.As(err, &merr) {
				// Expected for data sets an instance type
				// does not implement (e.g. TCs have no
				// current/parent DS)
				i.log.Debug("ptp: management error", "err", merr)
				lastData = time.Now()
				continue
			}
			i.log.Debug("ptp: decode failed", "err", err)
			continue
		}

		lastData = time.Now()
		if i.apply(tlv) {
			i.publish()
		}
	}
}

// apply folds a received TLV into the instance state.  Returns true
// when the state changed in a way worth publishing.
func (i *instance) apply(tlv ptp.ManagementTLV) bool {
	i.mu.Lock()
	defer i.mu.Unlock()

	s := i.state
	if s == nil {
		return false
	}

	switch t := tlv.(type) {
	case *ptp.DefaultDataSetTLV:
		if s.instanceType == "" {
			if t.NumberPorts > 1 {
				s.instanceType = "bc"
			} else {
				s.instanceType = "oc"
			}
		}
		s.defaultDS = t
	case *ptp.CurrentDataSetTLV:
		s.currentDS = t
	case *ptp.ParentDataSetTLV:
		s.parentDS = t
	case *timePropertiesDataSetTLV:
		s.timeProps = t
	case *portDataSetTLV:
		s.ports[t.PortIdentity.PortNumber] = t
	case *ptp.PortStatsNPTLV:
		s.portStats[t.PortIdentity.PortNumber] = t
	case *ptp.TimeStatusNPTLV:
		// Pushed on every clock update: keep the current-ds
		// offset live between CURRENT_DATA_SET refreshes
		if s.currentDS != nil {
			s.currentDS.OffsetFromMaster = ptp.TimeInterval(t.MasterOffsetNS << 16)
		}
	case *subscribeEventsNPTLV:
		// Acknowledgement of our subscription
		return false
	default:
		return false
	}

	return true
}

// portInterfaces returns the ordered interface names from a ptp4l conf
// (non-global section headers).
func portInterfaces(conf string) []string {
	data, err := os.ReadFile(conf)
	if err != nil {
		return nil
	}

	var ifaces []string
	for _, line := range strings.Split(string(data), "\n") {
		s := strings.TrimSpace(line)
		if strings.HasPrefix(s, "[") && strings.HasSuffix(s, "]") && s != "[global]" {
			ifaces = append(ifaces, s[1:len(s)-1])
		}
	}
	return ifaces
}

// transportSpecificFromConf reads the transportSpecific keyword from a
// ptp4l conf.  confd writes 1 for the ieee802-dot1as profile and 0
// otherwise.
func transportSpecificFromConf(conf string) uint8 {
	data, err := os.ReadFile(conf)
	if err != nil {
		return 0
	}

	for _, line := range strings.Split(string(data), "\n") {
		fields := strings.Fields(line)
		if len(fields) == 2 && fields[0] == "transportSpecific" {
			v, err := strconv.ParseUint(strings.TrimPrefix(fields[1], "0x"), 16, 4)
			if err == nil {
				return uint8(v)
			}
		}
	}
	return 0
}

// instanceTypeFromConf reads the clockType keyword from a ptp4l conf.
// Returns "" when not present (OC/BC derived from numberPorts instead).
func instanceTypeFromConf(conf string) string {
	data, err := os.ReadFile(conf)
	if err != nil {
		return ""
	}

	for _, line := range strings.Split(string(data), "\n") {
		fields := strings.Fields(line)
		if len(fields) == 2 && fields[0] == "clockType" {
			switch strings.ToUpper(fields[1]) {
			case "P2P_TC":
				return "p2p-tc"
			case "E2E_TC":
				return "e2e-tc"
			case "BOUNDARY_CLOCK":
				return "bc"
			}
		}
	}
	return ""
}
