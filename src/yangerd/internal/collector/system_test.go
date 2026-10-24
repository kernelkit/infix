package collector

import (
	"encoding/json"
	"fmt"
	"testing"

	"github.com/kernelkit/infix/src/yangerd/internal/testutil"
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

func newTestServices(initctl string) (*Services, *testutil.MockRunner) {
	runner := &testutil.MockRunner{
		Results: map[string][]byte{"initctl -j": []byte(initctl)},
		Errors:  map[string]error{},
	}
	return NewServices(runner), runner
}

func serviceList(t *testing.T, raw json.RawMessage) []interface{} {
	t.Helper()
	if raw == nil {
		t.Fatal("no services overlay")
	}
	state := make(map[string]interface{})
	if err := json.Unmarshal(raw, &state); err != nil {
		t.Fatalf("unmarshal overlay: %v", err)
	}
	svcs, ok := state["infix-system:services"].(map[string]interface{})
	if !ok {
		t.Fatalf("missing infix-system:services in %s", raw)
	}
	list, _ := svcs["service"].([]interface{})
	return list
}

func TestServicesLive(t *testing.T) {
	s, _ := newTestServices(testInitctlJSON)
	list := serviceList(t, s.Live())
	if len(list) != 2 {
		t.Fatalf("expected 2 services, got %v", list)
	}

	svc0 := list[0].(map[string]interface{})
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

func TestServicesOverlayHasOnlyServices(t *testing.T) {
	s, _ := newTestServices(testInitctlJSON)
	state := make(map[string]interface{})
	if err := json.Unmarshal(s.Live(), &state); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(state) != 1 {
		t.Fatalf("the overlay must not clobber other system-state keys, got %v", state)
	}
}

func TestServicesInitctlFailureKeepsLast(t *testing.T) {
	s, runner := newTestServices(testInitctlJSON)
	first := s.Live()
	if len(serviceList(t, first)) != 2 {
		t.Fatalf("expected 2 services first, got %s", first)
	}

	runner.Errors["initctl -j"] = fmt.Errorf("not available")
	if got := s.Live(); string(got) != string(first) {
		t.Fatalf("a failed read must serve the last good list, got %s", got)
	}
}

func TestServicesInitctlFailureWithoutLast(t *testing.T) {
	runner := &testutil.MockRunner{
		Results: map[string][]byte{},
		Errors:  map[string]error{"initctl -j": fmt.Errorf("not available")},
	}
	if got := NewServices(runner).Live(); got != nil {
		t.Fatalf("nothing read yet and initctl failing must give no overlay, got %s", got)
	}
}

func TestServicesNilFields(t *testing.T) {
	s, _ := newTestServices(`[{"identity":"minimal","pid":999,"status":"running","description":"Minimal service"}]`)
	list := serviceList(t, s.Live())
	if len(list) != 1 {
		t.Fatalf("expected 1 service, got %d", len(list))
	}

	svc := list[0].(map[string]interface{})
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
