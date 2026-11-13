package collector

import (
	"encoding/json"
	"fmt"
	"testing"

	"github.com/kernelkit/infix/src/yangerd/internal/testutil"
)

const webState = `{
	"name": "web",
	"id": "abc123",
	"image": "oci:/lib/oci/curios-httpd-latest",
	"image-id": "cd9d0aaf81be",
	"cmdline": "/usr/bin/tini -- /usr/sbin/httpd -f -v",
	"status": "created",
	"started": 1700000000,
	"networks": [{"name": "docker0", "ifname": "eth0", "type": "bridge", "addresses": ["172.17.0.2"]}],
	"publish": ["127.0.0.1:8080:80", "8443:443/tcp"],
	"cgroup": "system/container@web/payload"
}`

const hostState = `{
	"name": "ntpd",
	"id": "def456",
	"image": "oci:/lib/oci/curios-ntpd-latest",
	"image-id": "0cb6059c0f41",
	"cmdline": "/usr/sbin/ntpd -n",
	"status": "created",
	"networks": null,
	"publish": [],
	"cgroup": "system/container@ntpd/payload"
}`

func fixture(states map[string]string, files map[string][]byte) *testutil.MockFileReader {
	fs := &testutil.MockFileReader{Files: map[string][]byte{}, Globs: map[string][]string{}}
	for name, data := range states {
		path := fmt.Sprintf("/run/containers/%s/state.json", name)
		fs.Globs[StateGlob] = append(fs.Globs[StateGlob], path)
		fs.Files[path] = []byte(data)
	}
	for path, data := range files {
		fs.Files[path] = data
	}
	return fs
}

func runner(running ...string) *testutil.MockRunner {
	r := &testutil.MockRunner{Results: map[string][]byte{}, Errors: map[string]error{}}
	for _, name := range running {
		r.Results["crun state "+name] = []byte(`{"id":"` + name + `","status":"running","pid":1234}`)
	}
	return r
}

func collectContainers(t *testing.T, r *testutil.MockRunner, fs *testutil.MockFileReader) []map[string]interface{} {
	t.Helper()

	raw := CollectContainers(r, fs)
	if raw == nil {
		t.Fatal("no container data collected")
	}

	var out struct {
		Container []map[string]interface{} `json:"container"`
	}
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatalf("unmarshal containers: %v", err)
	}
	return out.Container
}

func TestContainerBasicInfo(t *testing.T) {
	fs := fixture(map[string]string{"web": webState}, nil)
	containers := collectContainers(t, runner("web"), fs)
	if len(containers) != 1 {
		t.Fatalf("expected 1 container, got %d", len(containers))
	}

	c := containers[0]
	if c["name"] != "web" || c["id"] != "abc123" {
		t.Fatalf("name/id mismatch: %v", c)
	}
	if c["image"] != "oci:/lib/oci/curios-httpd-latest" || c["image-id"] != "cd9d0aaf81be" {
		t.Fatalf("image mismatch: %v", c)
	}
	if c["running"] != true {
		t.Fatalf("expected running, got %v", c["running"])
	}
	if c["cmdline"] != "/usr/bin/tini -- /usr/sbin/httpd -f -v" {
		t.Fatalf("cmdline mismatch: %v", c["cmdline"])
	}
}

func TestContainerHostNetwork(t *testing.T) {
	fs := fixture(map[string]string{"ntpd": hostState}, nil)
	c := collectContainers(t, runner("ntpd"), fs)[0]

	net, ok := c["network"].(map[string]interface{})
	if !ok {
		t.Fatalf("missing network: %v", c)
	}
	if net["host"] != true {
		t.Fatalf("expected host network true, got %v", net["host"])
	}
}

func TestContainerBridgeNetwork(t *testing.T) {
	fs := fixture(map[string]string{"web": webState}, nil)
	c := collectContainers(t, runner("web"), fs)[0]

	net := c["network"].(map[string]interface{})
	ifaces := net["interface"].([]interface{})
	if len(ifaces) != 1 || ifaces[0].(map[string]interface{})["name"] != "docker0" {
		t.Fatalf("interface mismatch: %v", ifaces)
	}
	publish := net["publish"].([]interface{})
	if len(publish) != 2 {
		t.Fatalf("expected 2 published ports, got %d", len(publish))
	}
	if publish[0] != "127.0.0.1:8080:80" || publish[1] != "8443:443/tcp" {
		t.Fatalf("publish mismatch: %v", publish)
	}
}

func TestContainerCgroupLimits(t *testing.T) {
	fs := fixture(map[string]string{"web": webState}, map[string][]byte{
		"/sys/fs/cgroup/system/container@web/payload/memory.max": []byte("1073741824\n"),
		"/sys/fs/cgroup/system/container@web/payload/cpu.max":    []byte("200000 100000\n"),
	})
	c := collectContainers(t, runner(), fs)[0]

	limit, ok := c["resource-limit"].(map[string]interface{})
	if !ok {
		t.Fatalf("missing resource-limit: %v", c)
	}
	if limit["memory"] != "1048576" {
		t.Fatalf("memory limit: expected 1048576 KiB, got %v", limit["memory"])
	}
	if limit["cpu"] != float64(2000) {
		t.Fatalf("cpu limit: expected 2000 millicores, got %v", limit["cpu"])
	}
}

func TestContainerCgroupUnlimited(t *testing.T) {
	fs := fixture(map[string]string{"web": webState}, map[string][]byte{
		"/sys/fs/cgroup/system/container@web/payload/memory.max": []byte("max\n"),
		"/sys/fs/cgroup/system/container@web/payload/cpu.max":    []byte("max 100000\n"),
	})
	c := collectContainers(t, runner(), fs)[0]

	if _, ok := c["resource-limit"]; ok {
		t.Fatalf("resource-limit should be omitted for unlimited cgroup values: %v", c["resource-limit"])
	}
}

func TestContainerResourceStats(t *testing.T) {
	fs := fixture(map[string]string{"web": webState}, map[string][]byte{
		"/sys/fs/cgroup/system/container@web/payload/memory.current": []byte("5242880\n"),
		"/sys/fs/cgroup/system/container@web/payload/cpu.stat":       []byte("usage_usec 1500000\nuser_usec 1000000\n"),
		"/sys/fs/cgroup/system/container@web/payload/pids.current":   []byte("3\n"),
		"/sys/fs/cgroup/system/container@web/payload/io.stat":        []byte("8:0 rbytes=2048 wbytes=4096 rios=1 wios=2\n"),
	})
	r := runner("web")
	r.Results["ip -n web -s -j link show"] = []byte(`[
		{"ifname": "lo", "stats64": {"rx": {"bytes": 999}, "tx": {"bytes": 999}}},
		{"ifname": "eth0", "stats64": {"rx": {"bytes": 10240}, "tx": {"bytes": 20480}}}
	]`)
	c := collectContainers(t, r, fs)[0]

	usage, ok := c["resource-usage"].(map[string]interface{})
	if !ok {
		t.Fatalf("missing resource-usage: %v", c)
	}
	if usage["memory"] != "5120" {
		t.Fatalf("memory: expected 5120 KiB, got %v", usage["memory"])
	}
	if usage["pids"] != float64(3) {
		t.Fatalf("pids: expected 3, got %v", usage["pids"])
	}
	bio := usage["block-io"].(map[string]interface{})
	if bio["read"] != "2" || bio["write"] != "4" {
		t.Fatalf("block-io mismatch: %v", bio)
	}
	nio := usage["net-io"].(map[string]interface{})
	if nio["received"] != "10" || nio["sent"] != "20" {
		t.Fatalf("net-io mismatch: %v", nio)
	}
}

func TestContainerStopped(t *testing.T) {
	fs := fixture(map[string]string{"web": webState}, nil)
	c := collectContainers(t, runner(), fs)[0]

	if c["running"] != false {
		t.Fatalf("expected not running: %v", c)
	}
	if c["status"] != "created" {
		t.Fatalf("status: expected created, got %v", c["status"])
	}
	if _, ok := c["resource-usage"]; ok {
		t.Fatalf("stopped container must not include resource-usage: %v", c["resource-usage"])
	}
	net := c["network"].(map[string]interface{})
	publish := net["publish"].([]interface{})
	if len(publish) != 0 {
		t.Fatalf("stopped container must not include published ports, got %v", publish)
	}
}

func TestContainerMultiple(t *testing.T) {
	fs := fixture(map[string]string{"web": webState, "ntpd": hostState}, nil)
	containers := collectContainers(t, runner("web"), fs)
	if len(containers) != 2 {
		t.Fatalf("expected 2 containers, got %d", len(containers))
	}
}

// A container whose setup failed has no image yet; empty strings must
// not be emitted, they fail the YANG patterns and sink the whole entry.
func TestContainerSetupFailed(t *testing.T) {
	fs := fixture(map[string]string{"web": `{"name": "web", "status": "crashed (1)", "pid": 0}`}, nil)
	c := collectContainers(t, runner(), fs)[0]
	for _, key := range []string{"id", "image", "image-id", "cmdline"} {
		if _, ok := c[key]; ok {
			t.Fatalf("%s must be absent when unknown: %v", key, c)
		}
	}
	if c["status"] != "crashed (1)" || c["running"] != false {
		t.Fatalf("status/running: %v", c)
	}
}

func TestContainerNone(t *testing.T) {
	fs := fixture(nil, nil)
	if raw := CollectContainers(runner(), fs); raw != nil {
		t.Fatalf("expected nil without containers, got %s", raw)
	}
}

func TestContainerNetworks(t *testing.T) {
	fs := fixture(map[string]string{"web": webState, "ntpd": hostState}, nil)
	networks := ContainerNetworks(fs)
	if len(networks) != 1 || len(networks["docker0"]) != 1 || networks["docker0"][0] != "web" {
		t.Fatalf("networks = %v", networks)
	}
}

func TestParseCgroupMemory(t *testing.T) {
	if got := parseCgroupMemory("max"); got != 0 {
		t.Fatalf("max: expected 0, got %d", got)
	}
	if got := parseCgroupMemory("1048576"); got != 1024 {
		t.Fatalf("1048576: expected 1024 KiB, got %d", got)
	}
}

func TestParseCgroupCPU(t *testing.T) {
	if got := parseCgroupCPU("max 100000"); got != 0 {
		t.Fatalf("max: expected 0, got %d", got)
	}
	if got := parseCgroupCPU("50000 100000"); got != 500 {
		t.Fatalf("50000/100000: expected 500 millicores, got %d", got)
	}
}
