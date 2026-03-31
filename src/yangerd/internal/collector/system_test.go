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
	testResolvHead = `nameserver 8.8.8.8
nameserver 1.1.1.1
search example.com local.lan
options timeout:2 attempts:3
`

	testResolvconfOutput = `nameserver 10.0.0.1 # eth0
nameserver 10.0.0.2 # eth1
search dhcp.example.com # eth0
`

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
			"initctl -j":          []byte(testInitctlJSON),
			"/sbin/resolvconf -l": []byte(testResolvconfOutput),
		},
		Errors: map[string]error{},
	}

	fs := &testutil.MockFileReader{
		Files: map[string][]byte{
			"/etc/resolv.conf.head": []byte(testResolvHead),
		},
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

func TestSystemCollectorDNS(t *testing.T) {
	c, _, _ := newTestCollector()
	state := collectToState(t, c)

	dns, ok := state["infix-system:dns-resolver"].(map[string]interface{})
	if !ok {
		t.Fatal("missing infix-system:dns-resolver in system-state")
	}

	servers, ok := dns["server"].([]interface{})
	if !ok {
		t.Fatal("missing server list in dns-resolver")
	}

	// 2 static (resolv.conf.head) + 2 DHCP (resolvconf -l) = 4 total
	if len(servers) != 4 {
		t.Fatalf("expected 4 DNS servers, got %d: %v", len(servers), servers)
	}

	s0 := servers[0].(map[string]interface{})
	if s0["address"] != "8.8.8.8" || s0["origin"] != "static" {
		t.Fatalf("server[0]: expected 8.8.8.8/static, got %v", s0)
	}
	s1 := servers[1].(map[string]interface{})
	if s1["address"] != "1.1.1.1" || s1["origin"] != "static" {
		t.Fatalf("server[1]: expected 1.1.1.1/static, got %v", s1)
	}

	s2 := servers[2].(map[string]interface{})
	if s2["address"] != "10.0.0.1" || s2["origin"] != "dhcp" {
		t.Fatalf("server[2]: expected 10.0.0.1/dhcp, got %v", s2)
	}
	if s2["interface"] != "eth0" {
		t.Fatalf("server[2] interface: expected eth0, got %v", s2["interface"])
	}

	search, ok := dns["search"].([]interface{})
	if !ok || len(search) < 2 {
		t.Fatalf("expected search domains, got %v", dns["search"])
	}

	options, ok := dns["options"].(map[string]interface{})
	if !ok {
		t.Fatal("missing options in dns-resolver")
	}
	if int(options["timeout"].(float64)) != 2 {
		t.Fatalf("dns timeout: expected 2, got %v", options["timeout"])
	}
	if int(options["attempts"].(float64)) != 3 {
		t.Fatalf("dns attempts: expected 3, got %v", options["attempts"])
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
			"initctl -j":          fmt.Errorf("not available"),
			"/sbin/resolvconf -l": fmt.Errorf("not available"),
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
			"initctl -j":          []byte(`[{"identity":"minimal","pid":999,"status":"running","description":"Minimal service"}]`),
			"/sbin/resolvconf -l": []byte(""),
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

func TestSystemCollectorNoDNSEmptyArray(t *testing.T) {
	runner := &testutil.MockRunner{
		Results: map[string][]byte{
			"initctl -j":          []byte(testInitctlJSON),
			"/sbin/resolvconf -l": []byte(""),
		},
		Errors: map[string]error{},
	}

	fs := &testutil.MockFileReader{
		Files: map[string][]byte{},
		Globs: map[string][]string{},
	}

	c := NewSystemCollector(runner, fs, 60*time.Second)
	state := collectToState(t, c)

	dns, ok := state["infix-system:dns-resolver"].(map[string]interface{})
	if !ok {
		t.Fatal("missing infix-system:dns-resolver")
	}
	servers, ok := dns["server"].([]interface{})
	if !ok {
		t.Fatal("dns server should be an array, not null")
	}
	if len(servers) != 0 {
		t.Fatalf("expected empty server array, got %d servers", len(servers))
	}
}
