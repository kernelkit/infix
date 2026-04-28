package collector

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/kernelkit/infix/src/yangerd/internal/testutil"
	"github.com/kernelkit/infix/src/yangerd/internal/tree"
)

const (
	testInitctlJSON = `[
  {
    "identity": "sshd",
    "pid": 123,
    "status": "running",
    "description": "OpenSSH daemon",
    "memory": 4096000,
    "uptime": 3600,
    "restarts": 2
  },
  {
    "identity": "sysklogd",
    "pid": 456,
    "status": "running",
    "description": "System logger",
    "memory": 2048000,
    "uptime": 7200,
    "restarts": 0
  }
]`
)

func newTestCollector() (*SystemCollector, *testutil.MockRunner, *testutil.MockFileReader) {
	runner := &testutil.MockRunner{
		Results: map[string][]byte{
			"initctl -j": []byte(testInitctlJSON),
		},
		Errors: map[string]error{},
	}

	fs := &testutil.MockFileReader{
		Files: map[string][]byte{},
		Globs: map[string][]string{},
	}

	c := NewSystemCollector(runner, fs, 60*time.Second)
	return c, runner, fs
}

func collectToState(t *testing.T, c *SystemCollector) map[string]interface{} {
	t.Helper()
	tr := tree.New()
	if err := c.Collect(context.Background(), tr); err != nil {
		t.Fatalf("Collect failed: %v", err)
	}

	stateRaw := tr.Get("ietf-system:system-state")
	if stateRaw == nil {
		t.Fatal("missing ietf-system:system-state in tree")
	}

	state := make(map[string]interface{})
	if err := json.Unmarshal(stateRaw, &state); err != nil {
		t.Fatalf("unmarshal system-state: %v", err)
	}
	return state
}

func TestSystemCollectorName(t *testing.T) {
	c, _, _ := newTestCollector()
	if c.Name() != "system" {
		t.Fatalf("expected name 'system', got %q", c.Name())
	}
}

func TestSystemCollectorInterval(t *testing.T) {
	c, _, _ := newTestCollector()
	if c.Interval() != 60*time.Second {
		t.Fatalf("expected interval 60s, got %v", c.Interval())
	}
}

func TestSystemCollectorServices(t *testing.T) {
	c, _, _ := newTestCollector()
	state := collectToState(t, c)

	svcs, ok := state["infix-system:services"].(map[string]interface{})
	if !ok {
		t.Fatal("missing infix-system:services in system-state")
	}

	serviceList, ok := svcs["service"].([]interface{})
	if !ok || len(serviceList) != 2 {
		t.Fatalf("expected 2 services, got %v", svcs["service"])
	}

	svc0 := serviceList[0].(map[string]interface{})
	if svc0["name"] != "sshd" {
		t.Fatalf("service[0] name: expected sshd, got %v", svc0["name"])
	}
	if int(svc0["pid"].(float64)) != 123 {
		t.Fatalf("service[0] pid: expected 123, got %v", svc0["pid"])
	}

	stats := svc0["statistics"].(map[string]interface{})
	if stats["memory-usage"] != "4096000" {
		t.Fatalf("service[0] memory-usage: expected '4096000', got %v", stats["memory-usage"])
	}
	if stats["uptime"] != "3600" {
		t.Fatalf("service[0] uptime: expected '3600', got %v", stats["uptime"])
	}
	if int(stats["restart-count"].(float64)) != 2 {
		t.Fatalf("service[0] restart-count: expected 2, got %v", stats["restart-count"])
	}
}

func TestSystemCollectorCommandFailureGraceful(t *testing.T) {
	runner := &testutil.MockRunner{
		Results: map[string][]byte{},
		Errors: map[string]error{
			"initctl -j": fmt.Errorf("not available"),
		},
	}

	fs := &testutil.MockFileReader{
		Files: map[string][]byte{},
		Globs: map[string][]string{},
	}

	c := NewSystemCollector(runner, fs, 60*time.Second)
	tr := tree.New()
	err := c.Collect(context.Background(), tr)
	if err != nil {
		t.Fatalf("Collect should not return error on partial failures: %v", err)
	}

	if tr.Get("ietf-system:system-state") == nil {
		t.Fatal("ietf-system:system-state should be set even with command failures")
	}
}

func TestSystemCollectorTreeKeys(t *testing.T) {
	c, _, _ := newTestCollector()
	tr := tree.New()
	c.Collect(context.Background(), tr)

	keys := tr.Keys()
	if len(keys) != 1 {
		t.Fatalf("expected exactly 1 tree key, got %d: %v", len(keys), keys)
	}
	if keys[0] != "ietf-system:system-state" {
		t.Fatalf("expected tree key 'ietf-system:system-state', got %q", keys[0])
	}
}

func TestSystemCollectorServicesNilFields(t *testing.T) {
	runner := &testutil.MockRunner{
		Results: map[string][]byte{
			"initctl -j": []byte(`[{"identity":"minimal","pid":999,"status":"running","description":"Minimal service"}]`),
		},
		Errors: map[string]error{},
	}

	fs := &testutil.MockFileReader{
		Files: map[string][]byte{},
		Globs: map[string][]string{},
	}

	c := NewSystemCollector(runner, fs, 60*time.Second)
	state := collectToState(t, c)

	svcs := state["infix-system:services"].(map[string]interface{})
	serviceList := svcs["service"].([]interface{})
	if len(serviceList) != 1 {
		t.Fatalf("expected 1 service, got %d", len(serviceList))
	}

	svc := serviceList[0].(map[string]interface{})
	stats := svc["statistics"].(map[string]interface{})

	if stats["memory-usage"] != "0" {
		t.Fatalf("nil memory should become '0', got %v", stats["memory-usage"])
	}
	if stats["uptime"] != "0" {
		t.Fatalf("nil uptime should become '0', got %v", stats["uptime"])
	}
	if int(stats["restart-count"].(float64)) != 0 {
		t.Fatalf("nil restarts should become 0, got %v", stats["restart-count"])
	}
}
