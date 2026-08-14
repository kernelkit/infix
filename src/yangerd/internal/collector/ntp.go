package collector

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"net"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/facebook/time/ntp/chrony"
	"github.com/kernelkit/infix/src/yangerd/internal/tree"
)

// chronySock is chronyd's command (cmdmon) Unix socket.  The UDP
// command port would suffice for most queries, but serverstats is
// PERMIT_AUTH in chronyd, which only the Unix socket satisfies.
const chronySock = "/run/chrony/chronyd.sock"

// cmdmonClient is the subset of chrony.Client used by NTPCollector,
// broken out so tests can fake chronyd replies.
type cmdmonClient interface {
	Communicate(packet chrony.RequestPacket) (chrony.ResponsePacket, error)
}

// dialChrony connects to chronyd's cmdmon socket.  SOCK_DGRAM over
// AF_UNIX has no connection state, so the client must bind its own
// socket for the replies, in a directory chronyd can write to -- the
// same dance chronyc does.
func dialChrony() (cmdmonClient, func() error, error) {
	local := &net.UnixAddr{
		Name: fmt.Sprintf("/run/chrony/yangerd.%d.sock", os.Getpid()),
		Net:  "unixgram",
	}
	remote := &net.UnixAddr{Name: chronySock, Net: "unixgram"}

	os.Remove(local.Name) /* stale socket from a crashed run */
	conn, err := net.DialUnix("unixgram", local, remote)
	if err != nil {
		return nil, nil, err
	}

	closeFn := func() error {
		err := conn.Close()
		os.Remove(local.Name)
		return err
	}

	// chronyd runs unprivileged and must be able to send replies here
	if err := os.Chmod(local.Name, 0666); err != nil {
		closeFn()
		return nil, nil, err
	}
	if err := conn.SetDeadline(time.Now().Add(2 * time.Second)); err != nil {
		closeFn()
		return nil, nil, err
	}
	return &chrony.Client{Connection: conn}, closeFn, nil
}

// ntpSource pairs a source's data and stats replies, fetched by the
// same cmdmon source index.  stats may be nil if that request failed.
type ntpSource struct {
	data  *chrony.ReplySourceData
	stats *chrony.ReplySourceStats
}

// NTPCollector gathers ietf-ntp operational data from chronyd over the
// native cmdmon protocol (the same channel chronyc uses), plus ss to
// detect the NTP listening port.
type NTPCollector struct {
	cmd      CommandRunner
	dial     func() (cmdmonClient, func() error, error)
	interval time.Duration
}

// NewNTPCollector creates an NTPCollector with the given dependencies.
func NewNTPCollector(cmd CommandRunner, interval time.Duration) *NTPCollector {
	return &NTPCollector{cmd: cmd, dial: dialChrony, interval: interval}
}

// Name implements Collector.
func (c *NTPCollector) Name() string { return "ntp" }

// Interval implements Collector.
func (c *NTPCollector) Interval() time.Duration { return c.interval }

// Collect implements Collector.  It produces two tree keys:
//   - "ietf-ntp:ntp" — associations, clock state, server status, and
//     server statistics (RFC 9249).
//   - "ietf-system:system-state" — merged infix-system:ntp/sources/source
//     list with address, mode, state, stratum and poll for each chrony
//     source (Infix augmentation of ietf-system).
func (c *NTPCollector) Collect(ctx context.Context, t *tree.Tree) error {
	var srcs []ntpSource

	ntp := make(map[string]interface{})

	client, closeConn, err := c.dial()
	if err == nil {
		defer closeConn()

		srcs = getSources(client)
		addAssociations(ntp, srcs)
		addClockState(client, ntp)
		addServerStats(client, ntp)

		// Only probe the listening port when chronyd actually answered;
		// otherwise a stale ss line would keep the tree key alive after
		// chronyd stopped.
		if len(ntp) > 0 {
			c.addServerStatus(ctx, ntp)
		}
	}

	if len(ntp) > 0 {
		if data, err := json.Marshal(ntp); err == nil {
			t.Set("ietf-ntp:ntp", data)
		}
	} else {
		// chronyd is not running (NTP unconfigured, or disabled by a
		// config change).  Drop the key so data from a previous run does
		// not linger -- yangerd outlives config resets, so stale state
		// would otherwise survive until restart.
		t.Delete("ietf-ntp:ntp")
	}

	// Always refresh the Infix NTP sources under system-state, even when
	// empty.  Otherwise a source that disappears from chrony (e.g. a DHCP
	// lease without option 42, or NTP turned off) lingers as stale
	// operational data -- a phantom "selected" server chronyc no longer
	// reports.  Merge only overwrites the keys it is given, so we must
	// hand it an empty source list to clear a previously-populated one.
	sources := addSources(srcs)
	if sources == nil {
		sources = map[string]interface{}{
			"sources": map[string]interface{}{
				"source": []interface{}{},
			},
		}
	}
	if data, err := json.Marshal(map[string]interface{}{
		"infix-system:ntp": sources,
	}); err == nil {
		t.Merge("ietf-system:system-state", data)
	}

	return nil
}

// getSources fetches source data and stats for every chrony source.
// Data and stats share the same cmdmon index space, so no address
// matching is needed.
func getSources(client cmdmonClient) []ntpSource {
	resp, err := client.Communicate(chrony.NewSourcesPacket())
	if err != nil {
		return nil
	}
	sources, ok := resp.(*chrony.ReplySources)
	if !ok {
		return nil
	}

	srcs := make([]ntpSource, 0, sources.NSources)
	for i := 0; i < sources.NSources; i++ {
		resp, err := client.Communicate(chrony.NewSourceDataPacket(int32(i)))
		if err != nil {
			continue
		}
		data, ok := resp.(*chrony.ReplySourceData)
		if !ok {
			continue
		}

		src := ntpSource{data: data}
		if resp, err := client.Communicate(chrony.NewSourceStatsPacket(int32(i))); err == nil {
			if stats, ok := resp.(*chrony.ReplySourceStats); ok {
				src.stats = stats
			}
		}
		srcs = append(srcs, src)
	}

	return srcs
}

// addAssociations builds the associations/association list from chrony
// source data and stats.
func addAssociations(ntp map[string]interface{}, srcs []ntpSource) {
	modeMap := map[chrony.ModeType]string{
		chrony.SourceModeClient: "ietf-ntp:client",
		chrony.SourceModePeer:   "ietf-ntp:active",
	}

	var associations []interface{}
	for _, src := range srcs {
		d := src.data

		// Skip reference clocks — they have refids, not addresses
		if d.Mode == chrony.SourceModeRef {
			continue
		}

		// YANG requires stratum 1..16
		stratum := int(d.Stratum)
		if stratum < 1 || stratum > 16 {
			continue
		}

		mode := modeMap[d.Mode]
		if mode == "" {
			mode = "ietf-ntp:client"
		}

		assoc := map[string]interface{}{
			"address":      d.IPAddr.String(),
			"local-mode":   mode,
			"isconfigured": true,
			"stratum":      stratum,
			"reach":        int(d.Reachability),
			"poll":         int(d.Poll),
			"now":          int(d.SinceSample),
		}

		// Current sync source
		if d.State == chrony.SourceStateSync {
			assoc["prefer"] = true
		}

		// Offset: prefer sourcestats estimate over the last sample.
		// Convert seconds → milliseconds with 3 fraction digits
		if src.stats != nil {
			assoc["offset"] = fmt.Sprintf("%.3f", src.stats.EstimatedOffset*1000.0)
			assoc["dispersion"] = fmt.Sprintf("%.3f", src.stats.StandardDeviation*1000.0)
		} else {
			assoc["offset"] = fmt.Sprintf("%.3f", d.LatestMeas*1000.0)
		}

		// Delay: error estimate of the last sample, seconds → milliseconds
		assoc["delay"] = fmt.Sprintf("%.3f", math.Abs(d.LatestMeasErr)*1000.0)

		associations = append(associations, assoc)
	}

	if len(associations) > 0 {
		ntp["associations"] = map[string]interface{}{
			"association": associations,
		}
	}
}

// sourceStateMap maps chrony source states to YANG infix-system
// source-state enum values.
var sourceStateMap = map[chrony.SourceStateType]string{
	chrony.SourceStateSync:        "selected",
	chrony.SourceStateCandidate:   "candidate",
	chrony.SourceStateOutlier:     "outlier",
	chrony.SourceStateUnreach:     "unusable",
	chrony.SourceStateFalseTicker: "falseticker",
	chrony.SourceStateJittery:     "unstable",
}

// sourceModeMap maps chrony source modes to YANG infix-system
// source-mode enum values.
var sourceModeMap = map[chrony.ModeType]string{
	chrony.SourceModeClient: "server",
	chrony.SourceModePeer:   "peer",
	chrony.SourceModeRef:    "local-clock",
}

// addSources builds the infix-system:ntp/sources/source list.
// Reference clocks and sources with invalid stratum are skipped,
// matching the Python yanger ietf_system.py add_ntp() behaviour.
func addSources(srcs []ntpSource) map[string]interface{} {
	var sources []interface{}
	for _, src := range srcs {
		d := src.data

		if d.Mode == chrony.SourceModeRef {
			continue
		}
		if d.Stratum > 16 {
			continue
		}

		mode := sourceModeMap[d.Mode]
		if mode == "" {
			mode = "server"
		}
		state := sourceStateMap[d.State]
		if state == "" {
			continue
		}

		sources = append(sources, map[string]interface{}{
			"address": d.IPAddr.String(),
			"mode":    mode,
			"state":   state,
			"stratum": int(d.Stratum),
			"poll":    int(d.Poll),
		})
	}

	if len(sources) == 0 {
		return nil
	}

	return map[string]interface{}{
		"sources": map[string]interface{}{
			"source": sources,
		},
	}
}

// chrony LeapStatus from tracking: 0 normal, 1 insert, 2 delete,
// 3 not synchronised.
const leapUnsynchronised = 3

// clockRefid renders the tracking reference ID in a form the RFC 9249
// refid union accepts: an IPv4 address, a uint32, or exactly four
// characters.  The uint32 member must be a JSON number -- libyang
// rejects number-typed union members encoded as strings.
func clockRefid(t *chrony.Tracking) interface{} {
	if t.IPAddr != nil && !t.IPAddr.IsUnspecified() {
		if ip4 := t.IPAddr.To4(); ip4 != nil {
			return ip4.String()
		}
		// IPv6 sources have no representable address: chrony
		// stores a hash of it in the refid
		return t.RefID
	}
	if t.RefID != 0 {
		// Reference clock, e.g. "GPS": RFC 5905 refids are four
		// bytes, space-padded
		if s := refidToASCII(t.RefID); s != "" {
			return (s + "    ")[:4]
		}
		// Non-printable refid, e.g. chronyd's local reference
		// 0x7F7F0101: render as the pseudo-IP it encodes
		refid := t.RefID
		return fmt.Sprintf("%d.%d.%d.%d",
			refid>>24, refid>>16&0xff, refid>>8&0xff, refid&0xff)
	}
	return "0.0.0.0"
}

// refidToASCII decodes a printable refid name like "GPS", or returns ""
// when any byte is non-printable (a hash or pseudo-IP, not a name).
func refidToASCII(refid uint32) string {
	var s []byte

	for i := 3; i >= 0; i-- {
		c := byte(refid >> (8 * i))
		if c == 0 {
			continue
		}
		if c < ' ' || c > '~' {
			return ""
		}
		s = append(s, c)
	}

	return string(s)
}

// addClockState fills the clock-state container from chrony tracking.
func addClockState(client cmdmonClient, ntp map[string]interface{}) {
	resp, err := client.Communicate(chrony.NewTrackingPacket())
	if err != nil {
		return
	}
	tracking, ok := resp.(*chrony.ReplyTracking)
	if !ok {
		return
	}

	ss := make(map[string]interface{})

	// Stratum: chronyd uses 0 for "not synchronized", YANG requires 1-16
	stratum := int(tracking.Stratum)
	if stratum == 0 {
		stratum = 16
	}

	if stratum == 16 {
		ss["clock-state"] = "ietf-ntp:unsynchronized"
	} else {
		ss["clock-state"] = "ietf-ntp:synchronized"
	}
	ss["clock-stratum"] = stratum

	ss["clock-refid"] = clockRefid(&tracking.Tracking)

	// Frequencies (ppm → Hz with nominal 1GHz)
	nominal := 1000000000.0
	actual := nominal * (1.0 + tracking.FreqPPM/1000000.0)
	ss["nominal-freq"] = fmt.Sprintf("%.4f", nominal)
	ss["actual-freq"] = fmt.Sprintf("%.4f", actual)

	// Clock precision (fixed estimate, ~1µs)
	ss["clock-precision"] = -20

	// Clock offset (seconds → milliseconds)
	ss["clock-offset"] = fmt.Sprintf("%.3f", tracking.CurrentCorrection*1000.0)

	// Root delay and dispersion (seconds → milliseconds)
	ss["root-delay"] = fmt.Sprintf("%.3f", tracking.RootDelay*1000.0)
	ss["root-dispersion"] = fmt.Sprintf("%.3f", tracking.RootDispersion*1000.0)

	// Reference time (ISO 8601)
	if !tracking.RefTime.IsZero() && tracking.RefTime.Unix() > 0 {
		ss["reference-time"] = tracking.RefTime.UTC().Format("2006-01-02T15:04:05.000") + "Z"
	}

	// Sync state based on leap status
	if tracking.LeapStatus == leapUnsynchronised || stratum == 16 {
		ss["sync-state"] = "ietf-ntp:clock-never-set"
	} else {
		ss["sync-state"] = "ietf-ntp:clock-synchronized"
	}

	// Infix augmentations
	ss["infix-ntp:last-offset"] = fmt.Sprintf("%.9f", tracking.LastOffset)
	ss["infix-ntp:rms-offset"] = fmt.Sprintf("%.9f", tracking.RMSOffset)
	ss["infix-ntp:residual-freq"] = fmt.Sprintf("%.3f", tracking.ResidFreqPPM)
	ss["infix-ntp:skew"] = fmt.Sprintf("%.3f", tracking.SkewPPM)
	ss["infix-ntp:update-interval"] = fmt.Sprintf("%.1f", tracking.LastUpdateInterval)

	ntp["clock-state"] = map[string]interface{}{
		"system-status": ss,
	}
}

// addServerStatus adds the refclock-master stratum and listening port.
// Must be called after addClockState so clock-state is available.
func (c *NTPCollector) addServerStatus(ctx context.Context, ntp map[string]interface{}) {
	// Reuse stratum from clock-state if already populated
	if cs, ok := ntp["clock-state"].(map[string]interface{}); ok {
		if ss, ok := cs["system-status"].(map[string]interface{}); ok {
			if stratum, ok := ss["clock-stratum"]; ok {
				ntp["refclock-master"] = map[string]interface{}{
					"master-stratum": stratum,
				}
			}
		}
	}

	// Detect NTP listening port via ss
	ssOut, err := c.cmd.Run(ctx, "ss", "-ulnp")
	if err != nil {
		return
	}

	for _, line := range splitLines(string(ssOut)) {
		if !strings.Contains(line, "chronyd") {
			continue
		}
		// Skip loopback (command socket)
		if strings.Contains(line, "127.0.0.1") || strings.Contains(line, "[::1]") {
			continue
		}

		fields := strings.Fields(line)
		if len(fields) >= 5 {
			localAddr := fields[3]
			idx := strings.LastIndex(localAddr, ":")
			if idx >= 0 {
				portStr := localAddr[idx+1:]
				if port, err := strconv.Atoi(portStr); err == nil {
					ntp["port"] = port
					break
				}
			}
		}
	}
}

// addServerStats fills ntp-statistics from chrony server stats.  The
// reply version depends on the chronyd version, but all carry the NTP
// packets received/dropped counters.  chronyd does not count sent
// packets, so packet-sent/packet-sent-fail are not reported.
func addServerStats(client cmdmonClient, ntp map[string]interface{}) {
	resp, err := client.Communicate(chrony.NewServerStatsPacket())
	if err != nil {
		return
	}

	var received, dropped uint64
	switch r := resp.(type) {
	case *chrony.ReplyServerStats:
		received, dropped = uint64(r.NTPHits), uint64(r.NTPDrops)
	case *chrony.ReplyServerStats2:
		received, dropped = uint64(r.NTPHits), uint64(r.NTPDrops)
	case *chrony.ReplyServerStats3:
		received, dropped = uint64(r.NTPHits), uint64(r.NTPDrops)
	case *chrony.ReplyServerStats4:
		received, dropped = r.NTPHits, r.NTPDrops
	default:
		return
	}

	ntp["ntp-statistics"] = map[string]interface{}{
		"packet-received": received,
		"packet-dropped":  dropped,
	}
}

// splitLines splits text into non-empty lines.
func splitLines(text string) []string {
	var lines []string
	for _, line := range strings.Split(text, "\n") {
		line = strings.TrimSpace(line)
		if line != "" {
			lines = append(lines, line)
		}
	}
	return lines
}
