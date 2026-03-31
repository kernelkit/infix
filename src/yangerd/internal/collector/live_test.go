package collector

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/kernelkit/infix/src/yangerd/internal/testutil"
)

func TestLiveClock(t *testing.T) {
	fs := &testutil.MockFileReader{
		Files: map[string][]byte{
			"/proc/uptime": []byte("12345.67 23456.78\n"),
		},
	}

	before := time.Now().Truncate(time.Second)
	clock := liveClock(fs)
	after := time.Now().Truncate(time.Second).Add(time.Second)

	if clock == nil {
		t.Fatal("expected non-nil clock")
	}

	cur, ok := clock["current-datetime"].(string)
	if !ok || cur == "" {
		t.Fatal("missing current-datetime")
	}
	parsed, err := time.Parse("2006-01-02T15:04:05-07:00", cur)
	if err != nil {
		t.Fatalf("invalid datetime format: %v", err)
	}
	if parsed.Before(before) || parsed.After(after) {
		t.Fatalf("current-datetime %v not between %v and %v", parsed, before, after)
	}

	boot, ok := clock["boot-datetime"].(string)
	if !ok || boot == "" {
		t.Fatal("missing boot-datetime")
	}
	_, err = time.Parse("2006-01-02T15:04:05-07:00", boot)
	if err != nil {
		t.Fatalf("invalid boot-datetime format: %v", err)
	}
}

func TestLiveClockMissingFile(t *testing.T) {
	fs := &testutil.MockFileReader{
		Files: map[string][]byte{},
	}
	if clock := liveClock(fs); clock != nil {
		t.Fatalf("expected nil on missing /proc/uptime, got %v", clock)
	}
}

func TestLiveMemory(t *testing.T) {
	fs := &testutil.MockFileReader{
		Files: map[string][]byte{
			"/proc/meminfo": []byte("MemTotal:        1024000 kB\nMemFree:          512000 kB\nMemAvailable:     768000 kB\nBuffers:           64000 kB\n"),
		},
	}

	mem := liveMemory(fs)
	if mem == nil {
		t.Fatal("expected non-nil memory")
	}

	checks := map[string]string{
		"total":     "1024000",
		"free":      "512000",
		"available": "768000",
	}
	for key, expected := range checks {
		got, ok := mem[key].(string)
		if !ok || got != expected {
			t.Fatalf("memory[%q]: expected %q, got %v", key, expected, mem[key])
		}
	}

	if _, has := mem["Buffers"]; has {
		t.Fatal("unexpected Buffers field in memory output")
	}
}

func TestLiveMemoryMissingFile(t *testing.T) {
	fs := &testutil.MockFileReader{
		Files: map[string][]byte{},
	}
	if mem := liveMemory(fs); mem != nil {
		t.Fatalf("expected nil on missing /proc/meminfo, got %v", mem)
	}
}

func TestLiveLoadAvg(t *testing.T) {
	fs := &testutil.MockFileReader{
		Files: map[string][]byte{
			"/proc/loadavg": []byte("0.42 0.31 0.15 2/123 4567\n"),
		},
	}

	la := liveLoadAvg(fs)
	if la == nil {
		t.Fatal("expected non-nil load average")
	}

	checks := map[string]string{
		"load-1min":  "0.42",
		"load-5min":  "0.31",
		"load-15min": "0.15",
	}
	for key, expected := range checks {
		got, ok := la[key].(string)
		if !ok || got != expected {
			t.Fatalf("load-average[%q]: expected %q, got %v", key, expected, la[key])
		}
	}
}

func TestLiveLoadAvgMissingFile(t *testing.T) {
	fs := &testutil.MockFileReader{
		Files: map[string][]byte{},
	}
	if la := liveLoadAvg(fs); la != nil {
		t.Fatalf("expected nil on missing /proc/loadavg, got %v", la)
	}
}

func TestLiveSystemState(t *testing.T) {
	fs := &testutil.MockFileReader{
		Files: map[string][]byte{
			"/proc/uptime":  []byte("100.0 200.0\n"),
			"/proc/meminfo": []byte("MemTotal:        2048000 kB\nMemFree:          1024000 kB\nMemAvailable:     1536000 kB\n"),
			"/proc/loadavg": []byte("1.00 0.50 0.25 3/200 9999\n"),
		},
	}

	raw := LiveSystemState(fs)
	if raw == nil {
		t.Fatal("expected non-nil LiveSystemState output")
	}

	var state map[string]interface{}
	if err := json.Unmarshal(raw, &state); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if _, ok := state["clock"]; !ok {
		t.Fatal("missing clock in live state")
	}

	resource, ok := state["infix-system:resource-usage"].(map[string]interface{})
	if !ok {
		t.Fatal("missing infix-system:resource-usage in live state")
	}
	if _, ok := resource["memory"]; !ok {
		t.Fatal("missing memory in resource-usage")
	}
	if _, ok := resource["load-average"]; !ok {
		t.Fatal("missing load-average in resource-usage")
	}
}

func TestLiveSystemStatePartialFailure(t *testing.T) {
	fs := &testutil.MockFileReader{
		Files: map[string][]byte{
			"/proc/loadavg": []byte("0.10 0.20 0.30 1/50 1234\n"),
		},
	}

	raw := LiveSystemState(fs)
	if raw == nil {
		t.Fatal("expected non-nil even with partial data")
	}

	var state map[string]interface{}
	json.Unmarshal(raw, &state)

	if _, ok := state["clock"]; ok {
		t.Fatal("clock should be absent when /proc/uptime is missing")
	}

	resource := state["infix-system:resource-usage"].(map[string]interface{})
	if _, ok := resource["memory"]; ok {
		t.Fatal("memory should be absent when /proc/meminfo is missing")
	}
	if _, ok := resource["load-average"]; !ok {
		t.Fatal("load-average should be present")
	}
}
