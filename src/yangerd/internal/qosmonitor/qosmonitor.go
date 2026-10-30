// Package qosmonitor keeps the QoS operational state of each port, its
// capabilities.  The qdisc tree is read through a long-lived `tc` batch
// process and the DCB tables over rtnetlink, one query per port, on
// rtnetlink TC and DCB events and on link events.
package qosmonitor

import (
	"context"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/kernelkit/infix/src/yangerd/internal/backoff"
	"github.com/mdlayher/netlink"
	"golang.org/x/sys/unix"
)

const (
	// settle lets a confd script finish its tc and dcb commands, each
	// one an event, before the ports are read.
	settle = 50 * time.Millisecond

	sizeofTcmsg   = 20
	sizeofDcbmsg  = 4
	dcbAttrIfname = 1
)

// Querier answers one batch command, see ipbatch.Batch.
type Querier interface {
	Query(command string) (json.RawMessage, error)
}

// DCBReader reads a port's DCB tables, see DCB.
type DCBReader interface {
	Tables(ifname string) (app, rewr bool, err error)
}

// link is the part of an `ip -json -d link show` row used here.
type link struct {
	Name     string `json:"ifname"`
	Index    int    `json:"ifindex"`
	TxQueues int    `json:"num_tx_queues"`
}

// Monitor follows the QoS state of every port.
type Monitor struct {
	tc       Querier
	dcb      DCBReader
	links    func() json.RawMessage
	log      *slog.Logger
	onUpdate func(map[string]json.RawMessage)

	sysfs string

	mu         sync.Mutex
	pending    map[string]struct{}
	pendingIdx map[int]struct{}
	last       map[string]string // what onUpdate was last given, by port
	kick       chan struct{}
}

// New creates a Monitor reading qdiscs through tc and DCB tables through
// dcb, see DCB, for the interfaces in links, an `ip -json -d link show`
// dump.
func New(tc Querier, dcb DCBReader, links func() json.RawMessage, log *slog.Logger) *Monitor {
	return &Monitor{
		tc:         tc,
		dcb:        dcb,
		links:      links,
		log:        log,
		sysfs:      "/sys/class/net",
		pending:    make(map[string]struct{}),
		pendingIdx: make(map[int]struct{}),
		last:       make(map[string]string),
		kick:       make(chan struct{}, 1),
	}
}

// SetOnUpdate sets the callback given, for every port that changed, its
// members of the interface entry as a JSON object, nil when it has none.
func (m *Monitor) SetOnUpdate(fn func(map[string]json.RawMessage)) {
	m.onUpdate = fn
}

// Refresh queues a re-read of one port.
func (m *Monitor) Refresh(ifname string) {
	m.mu.Lock()
	m.pending[ifname] = struct{}{}
	m.mu.Unlock()
	m.wake()
}

func (m *Monitor) refreshIndex(index int) {
	m.mu.Lock()
	m.pendingIdx[index] = struct{}{}
	m.mu.Unlock()
	m.wake()
}

func (m *Monitor) wake() {
	select {
	case m.kick <- struct{}{}:
	default:
	}
}

// Sweep queues a re-read of every port, and of every port reported
// before, for start and a lost event stream.
func (m *Monitor) Sweep() {
	m.mu.Lock()
	for _, l := range m.linkTable() {
		m.pending[l.Name] = struct{}{}
	}
	for name := range m.last {
		m.pending[name] = struct{}{}
	}
	m.mu.Unlock()
	m.wake()
}

// Run serves refresh requests and the rtnetlink stream until ctx is
// cancelled.
func (m *Monitor) Run(ctx context.Context) error {
	go m.worker(ctx)
	return backoff.Retry(ctx, m.log, "qosmonitor", m.receive)
}

func (m *Monitor) receive(ctx context.Context) error {
	conn, err := netlink.Dial(unix.NETLINK_ROUTE, &netlink.Config{
		Groups: 1<<(unix.RTNLGRP_TC-1) | 1<<(unix.RTNLGRP_DCB-1),
	})
	if err != nil {
		return fmt.Errorf("dial rtnetlink: %w", err)
	}
	defer conn.Close()

	// Receive has no deadline; closing the socket is what ends it.
	stop := context.AfterFunc(ctx, func() { conn.Close() })
	defer stop()

	// Whatever changed before the subscription, or while it was down.
	m.Sweep()

	for {
		msgs, err := conn.Receive()
		if err != nil {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			return fmt.Errorf("receive rtnetlink message: %w", err)
		}
		for _, msg := range msgs {
			m.dispatch(msg)
		}
	}
}

// dispatch queues a refresh of the port a qdisc or DCB notification is
// about.  Filter and class notifications share the TC group and change
// nothing reported here.
func (m *Monitor) dispatch(msg netlink.Message) {
	switch msg.Header.Type {
	case unix.RTM_NEWQDISC, unix.RTM_DELQDISC:
		if len(msg.Data) >= sizeofTcmsg {
			m.refreshIndex(int(int32(binary.NativeEndian.Uint32(msg.Data[4:8]))))
		}
	case unix.RTM_SETDCB, unix.RTM_GETDCB:
		if name := dcbIfname(msg.Data); name != "" {
			m.Refresh(name)
		} else {
			m.Sweep()
		}
	}
}

func dcbIfname(data []byte) string {
	if len(data) < sizeofDcbmsg {
		return ""
	}
	ad, err := netlink.NewAttributeDecoder(data[sizeofDcbmsg:])
	if err != nil {
		return ""
	}
	for ad.Next() {
		if ad.Type() == dcbAttrIfname {
			return ad.String()
		}
	}
	return ""
}

func (m *Monitor) worker(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case <-m.kick:
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(settle):
		}
		m.drain()
	}
}

// drain reads every pending port and hands on what changed, at once.
func (m *Monitor) drain() {
	links := make(map[string]link)
	byIndex := make(map[int]string)
	for _, l := range m.linkTable() {
		links[l.Name] = l
		byIndex[l.Index] = l.Name
	}

	m.mu.Lock()
	names := m.pending
	m.pending = make(map[string]struct{})
	for index := range m.pendingIdx {
		if name, ok := byIndex[index]; ok {
			names[name] = struct{}{}
		}
	}
	m.pendingIdx = make(map[int]struct{})
	m.mu.Unlock()

	changed := make(map[string]json.RawMessage)
	for name := range names {
		var raw json.RawMessage
		if l, ok := links[name]; ok {
			if members := build(m.read(l)); members != nil {
				var err error
				if raw, err = json.Marshal(members); err != nil {
					m.log.Warn("qosmonitor: marshal", "ifname", name, "err", err)
					continue
				}
			}
		}

		m.mu.Lock()
		prev := m.last[name]
		if raw == nil {
			delete(m.last, name)
		} else {
			m.last[name] = string(raw)
		}
		m.mu.Unlock()

		if prev != string(raw) {
			changed[name] = raw
		}
	}

	if len(changed) > 0 && m.onUpdate != nil {
		m.onUpdate(changed)
	}
}

func (m *Monitor) linkTable() []link {
	var rows []link
	json.Unmarshal(m.links(), &rows)
	return rows
}

// read asks the kernel about one port.
func (m *Monitor) read(l link) port {
	p := port{TxQueues: l.TxQueues, Driver: m.driver(l.Name)}

	// By index: tc caches names for the life of the process, so a name
	// reused by a new interface would resolve to the old one.
	var err error
	if p.Qdiscs, err = m.tc.Query("qdisc show dev if" + strconv.Itoa(l.Index)); err != nil {
		m.log.Debug("qosmonitor: tc", "ifname", l.Name, "err", err)
	}
	if trustOrders[p.Driver] != nil {
		if p.App, p.Rewr, err = m.dcb.Tables(l.Name); err != nil {
			m.log.Debug("qosmonitor: dcb", "ifname", l.Name, "err", err)
		}
	}
	return p
}

func (m *Monitor) driver(ifname string) string {
	data, err := os.ReadFile(filepath.Join(m.sysfs, ifname, "device", "uevent"))
	if err != nil {
		return ""
	}
	for _, line := range strings.Split(string(data), "\n") {
		if drv, ok := strings.CutPrefix(line, "DRIVER="); ok {
			return strings.TrimSpace(drv)
		}
	}
	return ""
}
