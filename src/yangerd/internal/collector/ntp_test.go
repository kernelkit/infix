package collector

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/kernelkit/infix/src/yangerd/internal/testutil"
	"github.com/kernelkit/infix/src/yangerd/internal/tree"
)

const testChronycSources = `^,*,10.0.0.1,2,6,377,32,+0.000123,,0.000456
^,+,10.0.0.2,3,7,377,64,-0.000789,,0.001234
=,-,10.0.0.3,4,6,177,128,+0.001500,,0.002000
#,,GPS,1,4,377,16,+0.000001,,0.000010
^,?,10.0.0.4,0,6,0,0,+0.000000,,0.000000`

const testChronycSourcestats = `10.0.0.1,15,8,256,0.001,-0.002,+0.000050,0.000100
10.0.0.2,12,6,128,-0.005,0.003,-0.000300,0.000200
10.0.0.3,8,4,64,0.010,-0.008,+0.001000,0.000500`

const testChronycTracking = `C0A80001,router.local,2,1700000000.123,0.000045,-0.000012,0.000025,-1.500,0.003,0.050,0.004500,0.001200,64.0,Normal`

const testChronycTrackingUnsync = `00000000,,0,0.000,0.000000,0.000000,0.000000,0.000,0.000,0.000,0.000000,0.000000,0.0,Not synchronised`

const testChronycServerstats = `1000,5,200,3,1,8192,2,950,10`

const testSSOutput = `State  Recv-Q Send-Q Local Address:Port Peer Address:Port Process
UNCONN 0      0      0.0.0.0:123         0.0.0.0:*     users:(("chronyd",pid=5441,fd=5))
UNCONN 0      0      127.0.0.1:323       0.0.0.0:*     users:(("chronyd",pid=5441,fd=1))
`

func newNTPCollector(runner *testutil.MockRunner) *NTPCollector {
	return NewNTPCollector(runner, 60*time.Second)
}

func ntpCollect(t *testing.T, runner *testutil.MockRunner) (map[string]interface{}, *tree.Tree) {
	t.Helper()
	c := newNTPCollector(runner)
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

func fullNTPRunner() *testutil.MockRunner {
	return &testutil.MockRunner{
		Results: map[string][]byte{
			"chronyc -c sources":     []byte(testChronycSources),
			"chronyc -c sourcestats": []byte(testChronycSourcestats),
			"chronyc -c tracking":    []byte(testChronycTracking),
			"chronyc -c serverstats": []byte(testChronycServerstats),
			"ss -ulnp":               []byte(testSSOutput),
		},
		Errors: map[string]error{},
	}
}

func TestNTPCollectorNameAndInterval(t *testing.T) {
	c := newNTPCollector(fullNTPRunner())
	if c.Name() != "ntp" {
		t.Fatalf("expected name 'ntp', got %q", c.Name())
	}
	if c.Interval() != 60*time.Second {
		t.Fatalf("expected interval 60s, got %v", c.Interval())
	}
}

func TestNTPAssociations(t *testing.T) {
	out, _ := ntpCollect(t, fullNTPRunner())
	assocContainer := out["associations"].(map[string]interface{})
	assocs := assocContainer["association"].([]interface{})

	// 5 sources minus GPS refclock (#) minus stratum-0 (10.0.0.4) = 3
	if len(assocs) != 3 {
		t.Fatalf("expected 3 associations (refclock+stratum0 filtered), got %d", len(assocs))
	}

	byAddr := make(map[string]map[string]interface{})
	for _, a := range assocs {
		am := a.(map[string]interface{})
		byAddr[am["address"].(string)] = am
	}

	// 10.0.0.1: selected server (*), stratum 2
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

	// 10.0.0.3: peer mode (=)
	a3 := byAddr["10.0.0.3"]
	if a3 == nil {
		t.Fatal("missing association for 10.0.0.3")
	}
	if a3["local-mode"] != "ietf-ntp:active" {
		t.Fatalf("10.0.0.3 mode: expected ietf-ntp:active, got %v", a3["local-mode"])
	}
	// Should NOT be preferred (state is -)
	if _, hasPrefer := a3["prefer"]; hasPrefer {
		t.Fatal("10.0.0.3 should not be preferred")
	}
}

func TestNTPSources(t *testing.T) {
	_, tr := ntpCollect(t, fullNTPRunner())

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

	// 5 sources minus GPS refclock (#) minus stratum-0 (10.0.0.4) = 3
	if len(sources) != 3 {
		t.Fatalf("expected 3 sources, got %d", len(sources))
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
}

func TestNTPSourcesEmpty(t *testing.T) {
	runner := &testutil.MockRunner{
		Results: map[string][]byte{
			"chronyc -c tracking": []byte(testChronycTracking),
		},
		Errors: map[string]error{},
	}

	c := newNTPCollector(runner)
	tr := tree.New()
	c.Collect(context.Background(), tr)

	raw := tr.Get("ietf-system:system-state")
	if raw != nil {
		t.Fatal("should not set system-state when no sources available")
	}
}

func TestNTPClockStateSynchronized(t *testing.T) {
	out, _ := ntpCollect(t, fullNTPRunner())
	cs := out["clock-state"].(map[string]interface{})
	ss := cs["system-status"].(map[string]interface{})

	if ss["clock-state"] != "ietf-ntp:synchronized" {
		t.Fatalf("clock-state: expected synchronized, got %v", ss["clock-state"])
	}
	if toInt(ss["clock-stratum"]) != 2 {
		t.Fatalf("clock-stratum: expected 2, got %v", ss["clock-stratum"])
	}
	// refid from name "router.local" → padded/truncated to 4 chars: "rout"
	if ss["clock-refid"] != "rout" {
		t.Fatalf("clock-refid: expected 'rout', got %v", ss["clock-refid"])
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
	runner := &testutil.MockRunner{
		Results: map[string][]byte{
			"chronyc -c tracking": []byte(testChronycTrackingUnsync),
		},
		Errors: map[string]error{},
	}

	c := newNTPCollector(runner)
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
}

func TestNTPServerPort(t *testing.T) {
	out, _ := ntpCollect(t, fullNTPRunner())

	// Should find port 123 from the non-loopback ss line
	if toInt(out["port"]) != 123 {
		t.Fatalf("port: expected 123, got %v", out["port"])
	}
}

func TestNTPRefclockMaster(t *testing.T) {
	out, _ := ntpCollect(t, fullNTPRunner())
	master := out["refclock-master"].(map[string]interface{})
	if toInt(master["master-stratum"]) != 2 {
		t.Fatalf("master-stratum: expected 2, got %v", master["master-stratum"])
	}
}

func TestNTPServerStats(t *testing.T) {
	out, _ := ntpCollect(t, fullNTPRunner())
	stats := out["ntp-statistics"].(map[string]interface{})

	if toInt(stats["packet-received"]) != 1000 {
		t.Fatalf("packet-received: expected 1000, got %v", stats["packet-received"])
	}
	if toInt(stats["packet-dropped"]) != 5 {
		t.Fatalf("packet-dropped: expected 5, got %v", stats["packet-dropped"])
	}
	if toInt(stats["packet-sent"]) != 950 {
		t.Fatalf("packet-sent: expected 950, got %v", stats["packet-sent"])
	}
	if toInt(stats["packet-sent-fail"]) != 10 {
		t.Fatalf("packet-sent-fail: expected 10, got %v", stats["packet-sent-fail"])
	}
}

func TestNTPAllCommandsFail(t *testing.T) {
	runner := &testutil.MockRunner{
		Results: map[string][]byte{},
		Errors:  map[string]error{},
	}

	c := newNTPCollector(runner)
	tr := tree.New()
	err := c.Collect(context.Background(), tr)
	if err != nil {
		t.Fatalf("Collect should not error when chronyc unavailable: %v", err)
	}
	if tr.Get("ietf-ntp:ntp") != nil {
		t.Fatal("should not set ietf-ntp:ntp when nothing to report")
	}
}

func TestNTPRefidHexToIPv4(t *testing.T) {
	// When refid name is empty, hex ref-ID should be converted to dotted notation
	runner := &testutil.MockRunner{
		Results: map[string][]byte{
			"chronyc -c tracking": []byte("C0A80101,,2,1700000000.0,0.000001,0.000000,0.000000,-1.0,0.0,0.0,0.001,0.001,64.0,Normal"),
		},
		Errors: map[string]error{},
	}

	c := newNTPCollector(runner)
	tr := tree.New()
	c.Collect(context.Background(), tr)

	var out map[string]interface{}
	json.Unmarshal(tr.Get("ietf-ntp:ntp"), &out)
	cs := out["clock-state"].(map[string]interface{})
	ss := cs["system-status"].(map[string]interface{})

	// C0A80101 → 192.168.1.1
	if ss["clock-refid"] != "192.168.1.1" {
		t.Fatalf("clock-refid: expected '192.168.1.1', got %v", ss["clock-refid"])
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
