package collector

import (
	"context"
	"encoding/json"
	"errors"
	"net"
	"strings"
	"testing"
	"time"

	"github.com/facebook/time/ntp/chrony"
	"github.com/kernelkit/infix/src/yangerd/internal/testutil"
	"github.com/kernelkit/infix/src/yangerd/internal/tree"
)

const testSSOutput = `State  Recv-Q Send-Q Local Address:Port Peer Address:Port Process
UNCONN 0      0      0.0.0.0:123         0.0.0.0:*     users:(("chronyd",pid=5441,fd=5))
UNCONN 0      0      127.0.0.1:323       0.0.0.0:*     users:(("chronyd",pid=5441,fd=1))
`

// fakeChrony answers cmdmon requests from canned replies.  A nil reply
// (or err set) makes the corresponding request fail, mimicking a dead
// or partially-responding chronyd.
type fakeChrony struct {
	err         error
	data        []*chrony.ReplySourceData
	stats       []*chrony.ReplySourceStats
	tracking    *chrony.ReplyTracking
	serverStats chrony.ResponsePacket
}

func (f *fakeChrony) Communicate(packet chrony.RequestPacket) (chrony.ResponsePacket, error) {
	if f.err != nil {
		return nil, f.err
	}
	switch p := packet.(type) {
	case *chrony.RequestSources:
		return &chrony.ReplySources{NSources: len(f.data)}, nil
	case *chrony.RequestSourceData:
		if int(p.Index) >= len(f.data) || f.data[p.Index] == nil {
			return nil, errors.New("no such source")
		}
		return f.data[p.Index], nil
	case *chrony.RequestSourceStats:
		if int(p.Index) >= len(f.stats) || f.stats[p.Index] == nil {
			return nil, errors.New("no stats")
		}
		return f.stats[p.Index], nil
	case *chrony.RequestTracking:
		if f.tracking == nil {
			return nil, errors.New("no tracking")
		}
		return f.tracking, nil
	case *chrony.RequestServerStats:
		if f.serverStats == nil {
			return nil, errors.New("no serverstats")
		}
		return f.serverStats, nil
	}
	return nil, errors.New("unexpected request")
}

func v4(a, b, c, d uint8) net.IP {
	return net.IPv4(a, b, c, d)
}

func sourceData(ip net.IP, mode chrony.ModeType, state chrony.SourceStateType,
	stratum uint16, poll int16, reach uint16, since uint32,
	latestMeas, latestMeasErr float64) *chrony.ReplySourceData {
	return &chrony.ReplySourceData{
		SourceData: chrony.SourceData{
			IPAddr:        ip,
			Poll:          poll,
			Stratum:       stratum,
			State:         state,
			Mode:          mode,
			Reachability:  reach,
			SinceSample:   since,
			LatestMeas:    latestMeas,
			LatestMeasErr: latestMeasErr,
		},
	}
}

func sourceStats(offset, stddev float64) *chrony.ReplySourceStats {
	return &chrony.ReplySourceStats{
		SourceStats: chrony.SourceStats{
			EstimatedOffset:   offset,
			StandardDeviation: stddev,
		},
	}
}

// fullFakeChrony mirrors the source mix of the old chronyc fixtures:
// selected/candidate servers, an outlier peer, a GPS refclock, and an
// unreachable stratum-0 server (no stats).
func fullFakeChrony() *fakeChrony {
	return &fakeChrony{
		data: []*chrony.ReplySourceData{
			sourceData(v4(10, 0, 0, 1), chrony.SourceModeClient, chrony.SourceStateSync,
				2, 6, 0o377, 32, 0.000123, 0.000456),
			sourceData(v4(10, 0, 0, 2), chrony.SourceModeClient, chrony.SourceStateCandidate,
				3, 7, 0o377, 64, -0.000789, 0.001234),
			sourceData(v4(10, 0, 0, 3), chrony.SourceModePeer, chrony.SourceStateOutlier,
				4, 6, 0o177, 128, 0.001500, 0.002000),
			sourceData(nil, chrony.SourceModeRef, chrony.SourceStateSync,
				1, 4, 0o377, 16, 0.000001, 0.000010),
			sourceData(v4(10, 0, 0, 4), chrony.SourceModeClient, chrony.SourceStateUnreach,
				0, 6, 0, 0, 0, 0),
		},
		stats: []*chrony.ReplySourceStats{
			sourceStats(0.000050, 0.000100),
			sourceStats(-0.000300, 0.000200),
			sourceStats(0.001000, 0.000500),
			nil,
			nil,
		},
		tracking: &chrony.ReplyTracking{
			Tracking: chrony.Tracking{
				RefID:              0xC0A80001,
				IPAddr:             []byte{192, 168, 0, 1},
				Stratum:            2,
				LeapStatus:         0,
				RefTime:            time.Unix(1700000000, 123000000),
				CurrentCorrection:  0.000045,
				LastOffset:         -0.000012,
				RMSOffset:          0.000025,
				FreqPPM:            -1.5,
				ResidFreqPPM:       0.003,
				SkewPPM:            0.050,
				RootDelay:          0.004500,
				RootDispersion:     0.001200,
				LastUpdateInterval: 64.0,
			},
		},
		serverStats: &chrony.ReplyServerStats4{
			ServerStats4: chrony.ServerStats4{
				NTPHits:  1000,
				NTPDrops: 5,
			},
		},
	}
}

func unsyncFakeChrony() *fakeChrony {
	return &fakeChrony{
		tracking: &chrony.ReplyTracking{
			Tracking: chrony.Tracking{
				LeapStatus: 3, // not synchronised
			},
		},
	}
}

func newNTPCollector(runner *testutil.MockRunner, fake *fakeChrony) *NTPCollector {
	c := NewNTPCollector(runner, 60*time.Second)
	if fake != nil {
		c.dial = func() (cmdmonClient, func() error, error) {
			return fake, func() error { return nil }, nil
		}
	} else {
		c.dial = func() (cmdmonClient, func() error, error) {
			return nil, nil, errors.New("connection refused")
		}
	}
	return c
}

func ssRunner() *testutil.MockRunner {
	return &testutil.MockRunner{
		Results: map[string][]byte{
			"ss -ulnp": []byte(testSSOutput),
		},
		Errors: map[string]error{},
	}
}

func ntpCollect(t *testing.T, fake *fakeChrony) (map[string]interface{}, *tree.Tree) {
	t.Helper()
	c := newNTPCollector(ssRunner(), fake)
	tr := tree.New()
	if err := c.Collect(context.Background(), tr); err != nil {
		t.Fatalf("Collect failed: %v", err)
	}
	raw := tr.Get("ietf-ntp:ntp")
	if raw == nil {
		t.Fatal("missing ietf-ntp:ntp in tree")
	}
	var out map[string]interface{}
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatalf("unmarshal ntp: %v", err)
	}
	return out, tr
}

func TestNTPCollectorNameAndInterval(t *testing.T) {
	c := newNTPCollector(ssRunner(), fullFakeChrony())
	if c.Name() != "ntp" {
		t.Fatalf("expected name 'ntp', got %q", c.Name())
	}
	if c.Interval() != 60*time.Second {
		t.Fatalf("expected interval 60s, got %v", c.Interval())
	}
}

func TestNTPAssociations(t *testing.T) {
	out, _ := ntpCollect(t, fullFakeChrony())
	assocContainer := out["associations"].(map[string]interface{})
	assocs := assocContainer["association"].([]interface{})

	// 5 sources minus GPS refclock minus stratum-0 (10.0.0.4) = 3
	if len(assocs) != 3 {
		t.Fatalf("expected 3 associations (refclock+stratum0 filtered), got %d", len(assocs))
	}

	byAddr := make(map[string]map[string]interface{})
	for _, a := range assocs {
		am := a.(map[string]interface{})
		byAddr[am["address"].(string)] = am
	}

	// 10.0.0.1: selected server, stratum 2
	a1 := byAddr["10.0.0.1"]
	if a1 == nil {
		t.Fatal("missing association for 10.0.0.1")
	}
	if a1["local-mode"] != "ietf-ntp:client" {
		t.Fatalf("10.0.0.1 mode: expected ietf-ntp:client, got %v", a1["local-mode"])
	}
	if a1["prefer"] != true {
		t.Fatalf("10.0.0.1 should be preferred (selected source)")
	}
	if toInt(a1["stratum"]) != 2 {
		t.Fatalf("10.0.0.1 stratum: expected 2, got %v", a1["stratum"])
	}
	// Reach: 377 octal = 255 decimal
	if toInt(a1["reach"]) != 255 {
		t.Fatalf("10.0.0.1 reach: expected 255, got %v", a1["reach"])
	}
	// Offset should come from sourcestats (0.000050s → 0.050ms)
	if a1["offset"] != "0.050" {
		t.Fatalf("10.0.0.1 offset: expected '0.050', got %v", a1["offset"])
	}
	// Dispersion from sourcestats std_dev (0.000100s → 0.100ms)
	if a1["dispersion"] != "0.100" {
		t.Fatalf("10.0.0.1 dispersion: expected '0.100', got %v", a1["dispersion"])
	}
	// Delay from last sample error estimate (0.000456s → 0.456ms)
	if a1["delay"] != "0.456" {
		t.Fatalf("10.0.0.1 delay: expected '0.456', got %v", a1["delay"])
	}

	// 10.0.0.3: peer mode
	a3 := byAddr["10.0.0.3"]
	if a3 == nil {
		t.Fatal("missing association for 10.0.0.3")
	}
	if a3["local-mode"] != "ietf-ntp:active" {
		t.Fatalf("10.0.0.3 mode: expected ietf-ntp:active, got %v", a3["local-mode"])
	}
	// Should NOT be preferred (outlier)
	if _, hasPrefer := a3["prefer"]; hasPrefer {
		t.Fatal("10.0.0.3 should not be preferred")
	}
}

func TestNTPSources(t *testing.T) {
	_, tr := ntpCollect(t, fullFakeChrony())

	raw := tr.Get("ietf-system:system-state")
	if raw == nil {
		t.Fatal("missing ietf-system:system-state in tree")
	}
	var state map[string]interface{}
	if err := json.Unmarshal(raw, &state); err != nil {
		t.Fatalf("unmarshal system-state: %v", err)
	}

	ntpData, ok := state["infix-system:ntp"].(map[string]interface{})
	if !ok {
		t.Fatal("missing infix-system:ntp in system-state")
	}
	sourcesContainer, ok := ntpData["sources"].(map[string]interface{})
	if !ok {
		t.Fatal("missing sources in infix-system:ntp")
	}
	sources, ok := sourcesContainer["source"].([]interface{})
	if !ok {
		t.Fatal("missing source list in sources")
	}

	// 5 sources minus GPS refclock = 4 (stratum 0 is kept)
	if len(sources) != 4 {
		t.Fatalf("expected 4 sources, got %d", len(sources))
	}

	byAddr := make(map[string]map[string]interface{})
	for _, s := range sources {
		sm := s.(map[string]interface{})
		byAddr[sm["address"].(string)] = sm
	}

	// 10.0.0.1: selected server
	s1 := byAddr["10.0.0.1"]
	if s1 == nil {
		t.Fatal("missing source 10.0.0.1")
	}
	if s1["state"] != "selected" {
		t.Fatalf("10.0.0.1 state: expected selected, got %v", s1["state"])
	}
	if s1["mode"] != "server" {
		t.Fatalf("10.0.0.1 mode: expected server, got %v", s1["mode"])
	}
	if toInt(s1["stratum"]) != 2 {
		t.Fatalf("10.0.0.1 stratum: expected 2, got %v", s1["stratum"])
	}
	if toInt(s1["poll"]) != 6 {
		t.Fatalf("10.0.0.1 poll: expected 6, got %v", s1["poll"])
	}

	// 10.0.0.2: candidate server
	s2 := byAddr["10.0.0.2"]
	if s2 == nil {
		t.Fatal("missing source 10.0.0.2")
	}
	if s2["state"] != "candidate" {
		t.Fatalf("10.0.0.2 state: expected candidate, got %v", s2["state"])
	}
	if s2["mode"] != "server" {
		t.Fatalf("10.0.0.2 mode: expected server, got %v", s2["mode"])
	}

	// 10.0.0.3: outlier peer
	s3 := byAddr["10.0.0.3"]
	if s3 == nil {
		t.Fatal("missing source 10.0.0.3")
	}
	if s3["state"] != "outlier" {
		t.Fatalf("10.0.0.3 state: expected outlier, got %v", s3["state"])
	}
	if s3["mode"] != "peer" {
		t.Fatalf("10.0.0.3 mode: expected peer, got %v", s3["mode"])
	}

	// 10.0.0.4: unreachable server (stratum 0)
	s4 := byAddr["10.0.0.4"]
	if s4 == nil {
		t.Fatal("missing source 10.0.0.4")
	}
	if s4["state"] != "unusable" {
		t.Fatalf("10.0.0.4 state: expected unusable, got %v", s4["state"])
	}
	if toInt(s4["stratum"]) != 0 {
		t.Fatalf("10.0.0.4 stratum: expected 0, got %v", s4["stratum"])
	}
}

// ntpSourceCount returns the number of infix-system:ntp sources in the
// system-state tree key, failing the test if the subtree is missing.
func ntpSourceCount(t *testing.T, tr *tree.Tree) int {
	t.Helper()

	raw := tr.Get("ietf-system:system-state")
	if raw == nil {
		t.Fatal("system-state not set")
	}

	var data map[string]json.RawMessage
	if err := json.Unmarshal(raw, &data); err != nil {
		t.Fatalf("unmarshal system-state: %v", err)
	}
	ntpRaw, ok := data["infix-system:ntp"]
	if !ok {
		t.Fatal("infix-system:ntp not present")
	}

	var ntp struct {
		Sources struct {
			Source []json.RawMessage `json:"source"`
		} `json:"sources"`
	}
	if err := json.Unmarshal(ntpRaw, &ntp); err != nil {
		t.Fatalf("unmarshal infix-system:ntp: %v", err)
	}
	return len(ntp.Sources.Source)
}

func TestNTPSourcesEmpty(t *testing.T) {
	fake := &fakeChrony{tracking: fullFakeChrony().tracking}

	c := newNTPCollector(ssRunner(), fake)
	tr := tree.New()
	c.Collect(context.Background(), tr)

	// With no chrony sources the collector must still write an empty
	// source list, so a previously-reported source cannot linger as
	// stale operational data.
	if n := ntpSourceCount(t, tr); n != 0 {
		t.Fatalf("expected empty NTP source list, got %d", n)
	}
}

// When chronyd stops (NTP disabled via config reset), the whole
// ietf-ntp:ntp key must disappear -- yangerd outlives config resets, so
// a key that is only ever Set when non-empty would keep stale data from
// a previous run forever.
func TestNTPTreeKeyRemovedWhenChronydStops(t *testing.T) {
	tr := tree.New()

	newNTPCollector(ssRunner(), fullFakeChrony()).Collect(context.Background(), tr)
	if tr.Get("ietf-ntp:ntp") == nil {
		t.Fatal("expected ietf-ntp:ntp after first poll")
	}

	stopped := &fakeChrony{err: errors.New("read timeout")}
	newNTPCollector(ssRunner(), stopped).Collect(context.Background(), tr)
	if data := tr.Get("ietf-ntp:ntp"); data != nil {
		t.Fatalf("stale ietf-ntp:ntp survived chronyd stop: %s", data)
	}
}

// A source that disappears from chrony (e.g. a DHCP NTP server that is no
// longer offered) must be cleared from operational, not left stale.
func TestNTPSourcesClearedWhenGone(t *testing.T) {
	tr := tree.New()

	newNTPCollector(ssRunner(), fullFakeChrony()).Collect(context.Background(), tr)
	if ntpSourceCount(t, tr) == 0 {
		t.Fatal("expected NTP sources after first poll")
	}

	noSources := &fakeChrony{tracking: fullFakeChrony().tracking}
	newNTPCollector(ssRunner(), noSources).Collect(context.Background(), tr)
	if n := ntpSourceCount(t, tr); n != 0 {
		t.Fatalf("stale NTP sources not cleared: got %d, want 0", n)
	}
}

func TestNTPClockStateSynchronized(t *testing.T) {
	out, _ := ntpCollect(t, fullFakeChrony())
	cs := out["clock-state"].(map[string]interface{})
	ss := cs["system-status"].(map[string]interface{})

	if ss["clock-state"] != "ietf-ntp:synchronized" {
		t.Fatalf("clock-state: expected synchronized, got %v", ss["clock-state"])
	}
	if toInt(ss["clock-stratum"]) != 2 {
		t.Fatalf("clock-stratum: expected 2, got %v", ss["clock-stratum"])
	}
	// refid is the sync source address
	if ss["clock-refid"] != "192.168.0.1" {
		t.Fatalf("clock-refid: expected '192.168.0.1', got %v", ss["clock-refid"])
	}
	if ss["sync-state"] != "ietf-ntp:clock-synchronized" {
		t.Fatalf("sync-state: expected clock-synchronized, got %v", ss["sync-state"])
	}
	if toInt(ss["clock-precision"]) != -20 {
		t.Fatalf("clock-precision: expected -20, got %v", ss["clock-precision"])
	}

	// Verify nominal/actual freq strings
	if ss["nominal-freq"] != "1000000000.0000" {
		t.Fatalf("nominal-freq: expected '1000000000.0000', got %v", ss["nominal-freq"])
	}
	if ss["actual-freq"] != "999998500.0000" {
		t.Fatalf("actual-freq: expected '999998500.0000', got %v", ss["actual-freq"])
	}

	// Clock offset (0.000045s → 0.045ms)
	if ss["clock-offset"] != "0.045" {
		t.Fatalf("clock-offset: expected '0.045', got %v", ss["clock-offset"])
	}

	// Infix augmentations
	if ss["infix-ntp:update-interval"] != "64.0" {
		t.Fatalf("update-interval: expected '64.0', got %v", ss["infix-ntp:update-interval"])
	}

	// Reference time should be an ISO timestamp
	refTime, ok := ss["reference-time"].(string)
	if !ok || !strings.HasPrefix(refTime, "2023-") {
		t.Fatalf("reference-time should be 2023-* ISO timestamp, got %v", ss["reference-time"])
	}
}

func TestNTPClockStateUnsynchronized(t *testing.T) {
	c := newNTPCollector(ssRunner(), unsyncFakeChrony())
	tr := tree.New()
	c.Collect(context.Background(), tr)

	raw := tr.Get("ietf-ntp:ntp")
	if raw == nil {
		t.Fatal("expected ietf-ntp:ntp even when unsynchronized")
	}
	var out map[string]interface{}
	json.Unmarshal(raw, &out)

	cs := out["clock-state"].(map[string]interface{})
	ss := cs["system-status"].(map[string]interface{})

	if ss["clock-state"] != "ietf-ntp:unsynchronized" {
		t.Fatalf("clock-state: expected unsynchronized, got %v", ss["clock-state"])
	}
	// Stratum 0 → 16
	if toInt(ss["clock-stratum"]) != 16 {
		t.Fatalf("clock-stratum: expected 16 (mapped from 0), got %v", ss["clock-stratum"])
	}
	if ss["sync-state"] != "ietf-ntp:clock-never-set" {
		t.Fatalf("sync-state: expected clock-never-set, got %v", ss["sync-state"])
	}
	if ss["clock-refid"] != "0.0.0.0" {
		t.Fatalf("clock-refid: expected '0.0.0.0', got %v", ss["clock-refid"])
	}
}

func TestNTPServerPort(t *testing.T) {
	out, _ := ntpCollect(t, fullFakeChrony())

	// Should find port 123 from the non-loopback ss line
	if toInt(out["port"]) != 123 {
		t.Fatalf("port: expected 123, got %v", out["port"])
	}
}

func TestNTPRefclockMaster(t *testing.T) {
	out, _ := ntpCollect(t, fullFakeChrony())
	master := out["refclock-master"].(map[string]interface{})
	if toInt(master["master-stratum"]) != 2 {
		t.Fatalf("master-stratum: expected 2, got %v", master["master-stratum"])
	}
}

func TestNTPServerStats(t *testing.T) {
	out, _ := ntpCollect(t, fullFakeChrony())
	stats := out["ntp-statistics"].(map[string]interface{})

	if toInt(stats["packet-received"]) != 1000 {
		t.Fatalf("packet-received: expected 1000, got %v", stats["packet-received"])
	}
	if toInt(stats["packet-dropped"]) != 5 {
		t.Fatalf("packet-dropped: expected 5, got %v", stats["packet-dropped"])
	}
	// chronyd does not count sent packets; the old chronyc CSV parsing
	// reported auth/interleaved counters under these names by mistake.
	if _, ok := stats["packet-sent"]; ok {
		t.Fatal("packet-sent should not be reported")
	}
	if _, ok := stats["packet-sent-fail"]; ok {
		t.Fatal("packet-sent-fail should not be reported")
	}
}

func TestNTPChronydUnreachable(t *testing.T) {
	c := newNTPCollector(ssRunner(), nil) // dial fails
	tr := tree.New()
	err := c.Collect(context.Background(), tr)
	if err != nil {
		t.Fatalf("Collect should not error when chronyd unavailable: %v", err)
	}
	if tr.Get("ietf-ntp:ntp") != nil {
		t.Fatal("should not set ietf-ntp:ntp when nothing to report")
	}
}

func TestNTPRefclockRefid(t *testing.T) {
	// A refclock-synced chronyd has no source IP; the ASCII refid
	// (e.g. "GPS") is reported instead
	fake := &fakeChrony{
		tracking: &chrony.ReplyTracking{
			Tracking: chrony.Tracking{
				RefID:   0x47505300, // "GPS\0"
				Stratum: 1,
				RefTime: time.Unix(1700000000, 0),
			},
		},
	}

	c := newNTPCollector(ssRunner(), fake)
	tr := tree.New()
	c.Collect(context.Background(), tr)

	var out map[string]interface{}
	json.Unmarshal(tr.Get("ietf-ntp:ntp"), &out)
	cs := out["clock-state"].(map[string]interface{})
	ss := cs["system-status"].(map[string]interface{})

	// Space-padded to four chars: the RFC 9249 refid union only
	// accepts an IPv4 address, a uint32, or exactly four characters
	if ss["clock-refid"] != "GPS " {
		t.Fatalf("clock-refid: expected 'GPS ', got %q", ss["clock-refid"])
	}
}

func TestClockRefid(t *testing.T) {
	tests := []struct {
		name string
		in   chrony.Tracking
		want interface{}
	}{
		{
			name: "ipv4 source",
			in:   chrony.Tracking{IPAddr: []byte{192, 168, 0, 1}, RefID: 0xC0A80001},
			want: "192.168.0.1",
		},
		{
			// The uint32 union member must be a JSON number; libyang
			// rejects "1126301404" as a string
			name: "ipv6 source falls back to numeric refid",
			in: chrony.Tracking{
				IPAddr: []byte{0x20, 0x01, 0x0d, 0xb8, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 1},
				RefID:  0x4321FEDC,
			},
			want: uint32(0x4321FEDC),
		},
		{
			name: "refclock ascii refid",
			in:   chrony.Tracking{RefID: 0x47505300}, // "GPS\0"
			want: "GPS ",
		},
		{
			name: "local reference renders as pseudo-IP",
			in:   chrony.Tracking{RefID: 0x7F7F0101},
			want: "127.127.1.1",
		},
		{
			name: "unsynchronized",
			in:   chrony.Tracking{},
			want: "0.0.0.0",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := clockRefid(&tc.in); got != tc.want {
				t.Fatalf("clockRefid() = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestSplitLines(t *testing.T) {
	input := "line1\n\nline2\n  \nline3\n"
	got := splitLines(input)
	if len(got) != 3 {
		t.Fatalf("expected 3 lines, got %d: %v", len(got), got)
	}
	if got[0] != "line1" || got[1] != "line2" || got[2] != "line3" {
		t.Fatalf("unexpected lines: %v", got)
	}
}
