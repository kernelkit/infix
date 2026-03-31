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

func collectContainers(t *testing.T, runner *testutil.MockRunner, fs *testutil.MockFileReader) map[string]interface{} {
	t.Helper()

	c := NewContainerCollector(runner, fs, 30*time.Second)
	tr := tree.New()
	if err := c.Collect(context.Background(), tr); err != nil {
		t.Fatalf("Collect failed: %v", err)
	}

	raw := tr.Get("infix-containers:containers")
	if raw == nil {
		t.Fatal("missing infix-containers:containers in tree")
	}

	out := make(map[string]interface{})
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatalf("unmarshal containers: %v", err)
	}

	return out
}

func containerList(t *testing.T, data map[string]interface{}) []interface{} {
	t.Helper()

	containers, ok := data["container"].([]interface{})
	if !ok {
		t.Fatalf("missing container list: %v", data)
	}

	return containers
}

func TestContainerBasicInfo(t *testing.T) {
	runner := &testutil.MockRunner{
		Results: map[string][]byte{
			"podman ps -a --format=json": []byte(`[
				{
					"Names": ["web"],
					"Id": "abc123",
					"Image": "docker.io/library/nginx:latest",
					"ImageID": "sha256:image",
					"State": "running",
					"Status": "Up 2 hours",
					"Command": ["nginx", "-g", "daemon off;"],
					"Networks": ["podman0"],
					"Ports": []
				}
			]`),
			"podman inspect web": []byte(`[{}]`),
			"podman stats --no-stream --format json --no-reset web": []byte(`[]`),
		},
		Errors: map[string]error{},
	}

	fs := &testutil.MockFileReader{Files: map[string][]byte{}, Globs: map[string][]string{}}

	out := collectContainers(t, runner, fs)
	containers := containerList(t, out)
	if len(containers) != 1 {
		t.Fatalf("expected 1 container, got %d", len(containers))
	}

	c := containers[0].(map[string]interface{})
	if c["name"] != "web" {
		t.Fatalf("name: expected web, got %v", c["name"])
	}
	if c["id"] != "abc123" {
		t.Fatalf("id: expected abc123, got %v", c["id"])
	}
	if c["image"] != "docker.io/library/nginx:latest" {
		t.Fatalf("image mismatch: %v", c["image"])
	}
	if c["status"] != "Up 2 hours" {
		t.Fatalf("status mismatch: %v", c["status"])
	}
	if c["command"] != "nginx -g daemon off;" {
		t.Fatalf("command mismatch: %v", c["command"])
	}
	if c["running"] != true {
		t.Fatalf("running expected true, got %v", c["running"])
	}
}

func TestContainerHostNetwork(t *testing.T) {
	runner := &testutil.MockRunner{
		Results: map[string][]byte{
			"podman ps -a --format=json": []byte(`[
				{
					"Names": ["hostnet"],
					"Id": "id1",
					"Image": "img",
					"ImageID": "sha256:1",
					"State": "running",
					"Status": "Up",
					"Command": ["sleep", "60"],
					"Networks": ["podman0"],
					"Ports": [{"host_ip":"", "host_port":8080, "container_port":80, "protocol":"tcp"}]
				}
			]`),
			"podman inspect hostnet":                                    []byte(`[{"NetworkSettings":{"Networks":{"host":{}}}}]`),
			"podman stats --no-stream --format json --no-reset hostnet": []byte(`[]`),
		},
		Errors: map[string]error{},
	}

	fs := &testutil.MockFileReader{Files: map[string][]byte{}, Globs: map[string][]string{}}
	out := collectContainers(t, runner, fs)

	c := containerList(t, out)[0].(map[string]interface{})
	net, ok := c["network"].(map[string]interface{})
	if !ok {
		t.Fatalf("missing network: %v", c)
	}
	if net["host"] != true {
		t.Fatalf("expected host network true, got %v", net["host"])
	}
}

func TestContainerBridgeNetwork(t *testing.T) {
	runner := &testutil.MockRunner{
		Results: map[string][]byte{
			"podman ps -a --format=json": []byte(`[
				{
					"Names": ["bridge"],
					"Id": "id2",
					"Image": "img",
					"ImageID": "sha256:2",
					"State": "running",
					"Status": "Up",
					"Command": ["app"],
					"Networks": ["podman0", "br0"],
					"Ports": [
						{"host_ip":"127.0.0.1", "host_port":8080, "container_port":80, "protocol":"tcp"},
						{"host_ip":"", "host_port":8443, "container_port":443, "protocol":"tcp"}
					]
				}
			]`),
			"podman inspect bridge": []byte(`[{"NetworkSettings":{"Networks":{"bridge":{}}}}]`),
			"podman stats --no-stream --format json --no-reset bridge": []byte(`[]`),
		},
		Errors: map[string]error{},
	}

	fs := &testutil.MockFileReader{Files: map[string][]byte{}, Globs: map[string][]string{}}
	out := collectContainers(t, runner, fs)

	c := containerList(t, out)[0].(map[string]interface{})
	net := c["network"].(map[string]interface{})

	ifaces := net["interface"].([]interface{})
	if len(ifaces) != 2 {
		t.Fatalf("expected 2 interfaces, got %d", len(ifaces))
	}
	if ifaces[0].(map[string]interface{})["name"] != "podman0" {
		t.Fatalf("interface[0] mismatch: %v", ifaces[0])
	}

	publish := net["publish"].([]interface{})
	if len(publish) != 2 {
		t.Fatalf("expected 2 published ports, got %d", len(publish))
	}
	if publish[0] != "127.0.0.1:8080:80/tcp" {
		t.Fatalf("publish[0] mismatch: %v", publish[0])
	}
	if publish[1] != "8443:443/tcp" {
		t.Fatalf("publish[1] mismatch: %v", publish[1])
	}
}

func TestContainerCgroupLimits(t *testing.T) {
	cgroupPath := "/machine.slice/libpod-abc.scope"
	runner := &testutil.MockRunner{
		Results: map[string][]byte{
			"podman ps -a --format=json": []byte(`[
				{"Names":["limited"],"Id":"id3","Image":"img","ImageID":"sha256:3","State":"exited","Status":"Exited","Command":["app"],"Networks":[],"Ports":[]}
			]`),
			"podman inspect limited": []byte(fmt.Sprintf(`[{"State":{"CgroupPath":%q},"NetworkSettings":{"Networks":{"bridge":{}}}}]`, cgroupPath)),
		},
		Errors: map[string]error{},
	}

	fs := &testutil.MockFileReader{
		Files: map[string][]byte{
			"/sys/fs/cgroup/machine.slice/libpod-abc.scope/memory.max": []byte("1073741824\n"),
			"/sys/fs/cgroup/machine.slice/libpod-abc.scope/cpu.max":    []byte("200000 100000\n"),
		},
		Globs: map[string][]string{},
	}

	out := collectContainers(t, runner, fs)
	c := containerList(t, out)[0].(map[string]interface{})

	limit, ok := c["resource-limit"].(map[string]interface{})
	if !ok {
		t.Fatalf("missing resource-limit: %v", c)
	}
	if limit["memory"] != "1048576" {
		t.Fatalf("memory limit expected 1048576, got %v", limit["memory"])
	}
	if toInt(limit["cpu"]) != 2000 {
		t.Fatalf("cpu limit expected 2000, got %v", limit["cpu"])
	}
}

func TestContainerCgroupUnlimited(t *testing.T) {
	cgroupPath := "/machine.slice/libpod-unlimited.scope"
	runner := &testutil.MockRunner{
		Results: map[string][]byte{
			"podman ps -a --format=json": []byte(`[
				{"Names":["nolimit"],"Id":"id4","Image":"img","ImageID":"sha256:4","State":"exited","Status":"Exited","Command":["app"],"Networks":[],"Ports":[]}
			]`),
			"podman inspect nolimit": []byte(fmt.Sprintf(`[{"State":{"CgroupPath":%q},"NetworkSettings":{"Networks":{"bridge":{}}}}]`, cgroupPath)),
		},
		Errors: map[string]error{},
	}

	fs := &testutil.MockFileReader{
		Files: map[string][]byte{
			"/sys/fs/cgroup/machine.slice/libpod-unlimited.scope/memory.max": []byte("max\n"),
			"/sys/fs/cgroup/machine.slice/libpod-unlimited.scope/cpu.max":    []byte("max 100000\n"),
		},
		Globs: map[string][]string{},
	}

	out := collectContainers(t, runner, fs)
	c := containerList(t, out)[0].(map[string]interface{})
	if _, ok := c["resource-limit"]; ok {
		t.Fatalf("resource-limit should be omitted for unlimited cgroup values: %v", c["resource-limit"])
	}
}

func TestContainerResourceStats(t *testing.T) {
	runner := &testutil.MockRunner{
		Results: map[string][]byte{
			"podman ps -a --format=json": []byte(`[
				{
					"Names":["stats"],"Id":"id5","Image":"img","ImageID":"sha256:5",
					"State":"running","Status":"Up","Command":["app"],"Networks":["podman0"],"Ports":[]
				}
			]`),
			"podman inspect stats": []byte(`[{"NetworkSettings":{"Networks":{"bridge":{}}}}]`),
			"podman stats --no-stream --format json --no-reset stats": []byte(`[
				{
					"mem_usage":"123.4MB / 1.5GB",
					"cpu_percent":"12.34%",
					"block_io":"1.2MB / 3.4GB",
					"net_io":"1.2MB / 3.4GB",
					"pids":5
				}
			]`),
		},
		Errors: map[string]error{},
	}

	fs := &testutil.MockFileReader{Files: map[string][]byte{}, Globs: map[string][]string{}}
	out := collectContainers(t, runner, fs)

	c := containerList(t, out)[0].(map[string]interface{})
	usage, ok := c["resource-usage"].(map[string]interface{})
	if !ok {
		t.Fatalf("missing resource-usage: %v", c)
	}

	if usage["memory"] != "120507" {
		t.Fatalf("memory usage expected 120507, got %v", usage["memory"])
	}
	if usage["cpu"] != "12.34" {
		t.Fatalf("cpu usage expected 12.34, got %v", usage["cpu"])
	}
	bio := usage["block-io"].(map[string]interface{})
	if bio["read"] != "1171" {
		t.Fatalf("block-io read expected 1171, got %v", bio["read"])
	}
	if bio["write"] != "3320312" {
		t.Fatalf("block-io write expected 3320312, got %v", bio["write"])
	}
	nio := usage["net-io"].(map[string]interface{})
	if nio["received"] != "1171" {
		t.Fatalf("net-io received expected 1171, got %v", nio["received"])
	}
	if nio["sent"] != "3320312" {
		t.Fatalf("net-io sent expected 3320312, got %v", nio["sent"])
	}
	if toInt(usage["pids"]) != 5 {
		t.Fatalf("pids expected 5, got %v", usage["pids"])
	}
}

func TestContainerStopped(t *testing.T) {
	runner := &testutil.MockRunner{
		Results: map[string][]byte{
			"podman ps -a --format=json": []byte(`[
				{
					"Names":["stopped"],"Id":"id6","Image":"img","ImageID":"sha256:6",
					"State":"exited","Status":"Exited (0)","Command":["app"],
					"Networks":["podman0"],"Ports":[{"host_ip":"","host_port":8080,"container_port":80,"protocol":"tcp"}]
				}
			]`),
			"podman inspect stopped": []byte(`[{"NetworkSettings":{"Networks":{"bridge":{}}}}]`),
		},
		Errors: map[string]error{},
	}

	fs := &testutil.MockFileReader{Files: map[string][]byte{}, Globs: map[string][]string{}}
	out := collectContainers(t, runner, fs)

	c := containerList(t, out)[0].(map[string]interface{})
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
	runner := &testutil.MockRunner{
		Results: map[string][]byte{
			"podman ps -a --format=json": []byte(`[
				{"Names":["one"],"Id":"id7","Image":"img1","ImageID":"sha256:7","State":"running","Status":"Up","Command":["a"],"Networks":[],"Ports":[]},
				{"Names":["two"],"Id":"id8","Image":"img2","ImageID":"sha256:8","State":"exited","Status":"Exited","Command":["b"],"Networks":[],"Ports":[]}
			]`),
			"podman inspect one": []byte(`[{}]`),
			"podman stats --no-stream --format json --no-reset one": []byte(`[]`),
			"podman inspect two": []byte(`[{}]`),
		},
		Errors: map[string]error{},
	}

	fs := &testutil.MockFileReader{Files: map[string][]byte{}, Globs: map[string][]string{}}
	out := collectContainers(t, runner, fs)

	containers := containerList(t, out)
	if len(containers) != 2 {
		t.Fatalf("expected 2 containers, got %d", len(containers))
	}
	if containers[0].(map[string]interface{})["name"] != "one" {
		t.Fatalf("first container name mismatch: %v", containers[0])
	}
	if containers[1].(map[string]interface{})["name"] != "two" {
		t.Fatalf("second container name mismatch: %v", containers[1])
	}
}

func TestParseSizeKiB(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected int
	}{
		{name: "mb", input: "1.5MB", expected: 1464},
		{name: "kb", input: "512kB", expected: 500},
		{name: "gib", input: "2GiB", expected: 2097152},
		{name: "mib", input: "64MiB", expected: 65536},
		{name: "bytes", input: "2048B", expected: 2},
		{name: "empty", input: "", expected: 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parseSizeKiB(tt.input)
			if got != tt.expected {
				t.Errorf("parseSizeKiB(%q): expected %d, got %d", tt.input, tt.expected, got)
			}
		})
	}
}

func TestParseCgroupMemory(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected int
	}{
		{name: "max", input: "max", expected: 0},
		{name: "bytes", input: "1073741824", expected: 1048576},
		{name: "empty", input: "", expected: 0},
		{name: "invalid", input: "abc", expected: 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parseCgroupMemory(tt.input)
			if got != tt.expected {
				t.Errorf("parseCgroupMemory(%q): expected %d, got %d", tt.input, tt.expected, got)
			}
		})
	}
}

func TestParseCgroupCPU(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected int
	}{
		{name: "max", input: "max 100000", expected: 0},
		{name: "limited", input: "50000 100000", expected: 500},
		{name: "empty", input: "", expected: 0},
		{name: "invalid", input: "abc", expected: 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parseCgroupCPU(tt.input)
			if got != tt.expected {
				t.Errorf("parseCgroupCPU(%q): expected %d, got %d", tt.input, tt.expected, got)
			}
		})
	}
}

func TestContainerGracefulDegradation(t *testing.T) {
	runner := &testutil.MockRunner{
		Results: map[string][]byte{},
		Errors: map[string]error{
			"podman ps -a --format=json": fmt.Errorf("podman not found"),
		},
	}

	fs := &testutil.MockFileReader{Files: map[string][]byte{}, Globs: map[string][]string{}}
	out := collectContainers(t, runner, fs)

	containers := containerList(t, out)
	if len(containers) != 0 {
		t.Fatalf("expected no containers when podman ps fails, got %d", len(containers))
	}
}
