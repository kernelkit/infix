package collector

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
)

// StateGlob matches the state file ixcont keeps per container.
const StateGlob = "/run/containers/*/state.json"

const cgroupRoot = "/sys/fs/cgroup"

// collectTimeout bounds one full collection: a crun stuck on a container
// in a bad state must not wedge the container monitor.
const collectTimeout = 60 * time.Second

// containerState is what ixcont writes to state.json.  Networks is nil
// for a container sharing the host's network namespace.
type containerState struct {
	Name     string              `json:"name"`
	ID       string              `json:"id"`
	Image    string              `json:"image"`
	ImageID  string              `json:"image-id"`
	Cmdline  string              `json:"cmdline"`
	Status   string              `json:"status"`
	Started  float64             `json:"started"`
	Networks *[]containerNetwork `json:"networks"`
	Publish  []string            `json:"publish"`
	Cgroup   string              `json:"cgroup"`
}

type containerNetwork struct {
	Name      string   `json:"name"`
	Ifname    string   `json:"ifname"`
	Type      string   `json:"type"`
	Addresses []string `json:"addresses"`
}

// cpuSample remembers the last cgroup CPU reading per container, so
// the next collection can report a percentage over the interval.
type cpuSample struct {
	usec uint64
	at   time.Time
}

var (
	cpuMu   sync.Mutex
	cpuPrev = map[string]cpuSample{}
)

// containerCollector gathers infix-containers operational data.
type containerCollector struct {
	cmd CommandRunner
	fs  FileReader
}

// loadStates reads every container state file, in name order.
func loadStates(fs FileReader) []containerState {
	paths, err := fs.Glob(StateGlob)
	if err != nil {
		return nil
	}
	var states []containerState
	for _, path := range paths {
		data, err := fs.ReadFile(path)
		if err != nil {
			continue
		}
		var st containerState
		if err := json.Unmarshal(data, &st); err != nil || st.Name == "" {
			log.Printf("collector containers: %s: %v", path, err)
			continue
		}
		states = append(states, st)
	}
	return states
}

// ContainerNetworks maps each container network to the containers using
// it, all of them, so a stopped container still shows up in the list of
// a network it shares.
func ContainerNetworks(fs FileReader) map[string][]string {
	networks := map[string][]string{}
	for _, st := range loadStates(fs) {
		if st.Networks == nil {
			continue
		}
		for _, n := range *st.Networks {
			networks[n.Name] = append(networks[n.Name], st.Name)
		}
	}
	return networks
}

// CollectContainers runs a full container collection and returns the
// result as JSON suitable for tree.Set("infix-containers:containers"),
// or nil when no container exists, so an enabled but idle container
// feature does not surface as operational data.
func CollectContainers(cmd CommandRunner, fs FileReader) json.RawMessage {
	ctx, cancel := context.WithTimeout(context.Background(), collectTimeout)
	defer cancel()

	c := &containerCollector{cmd: cmd, fs: fs}
	containers := []interface{}{}
	for _, st := range loadStates(fs) {
		containers = append(containers, c.container(ctx, st))
	}
	if len(containers) == 0 {
		return nil
	}

	data, err := json.Marshal(map[string]interface{}{"container": containers})
	if err != nil {
		return nil
	}
	return data
}

// running asks crun, the only authority on whether the process is alive.
func (c *containerCollector) running(ctx context.Context, name string) bool {
	var st struct {
		Status string `json:"status"`
	}
	if err := runJSON(ctx, c.cmd, &st, "crun", "state", name); err != nil {
		return false
	}
	return st.Status == "running"
}

func (c *containerCollector) cgroupFile(st containerState, name string) string {
	data, err := c.fs.ReadFile(filepath.Join(cgroupRoot, st.Cgroup, name))
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(data))
}

func (c *containerCollector) limits(st containerState) map[string]interface{} {
	memVal := parseCgroupMemory(c.cgroupFile(st, "memory.max"))
	cpuVal := parseCgroupCPU(c.cgroupFile(st, "cpu.max"))
	if memVal <= 0 && cpuVal <= 0 {
		return nil
	}
	result := make(map[string]interface{})
	if memVal > 0 {
		result["memory"] = strconv.Itoa(memVal)
	}
	if cpuVal > 0 {
		result["cpu"] = cpuVal
	}
	return result
}

// cpuStatUsage returns usage_usec from a cgroup v2 cpu.stat.
func cpuStatUsage(stat string) uint64 {
	for _, line := range strings.Split(stat, "\n") {
		fields := strings.Fields(line)
		if len(fields) == 2 && fields[0] == "usage_usec" {
			v, _ := strconv.ParseUint(fields[1], 10, 64)
			return v
		}
	}
	return 0
}

// ioStatBytes sums rbytes and wbytes over all devices in io.stat.
func ioStatBytes(stat string) (uint64, uint64) {
	var rd, wr uint64
	for _, line := range strings.Split(stat, "\n") {
		for _, field := range strings.Fields(line) {
			key, val, ok := strings.Cut(field, "=")
			if !ok {
				continue
			}
			v, _ := strconv.ParseUint(val, 10, 64)
			switch key {
			case "rbytes":
				rd += v
			case "wbytes":
				wr += v
			}
		}
	}
	return rd, wr
}

func (c *containerCollector) netIO(ctx context.Context, st containerState) (uint64, uint64, bool) {
	if st.Networks == nil {
		return 0, 0, false
	}
	var links []struct {
		Ifname string `json:"ifname"`
		Stats  struct {
			RX struct {
				Bytes uint64 `json:"bytes"`
			} `json:"rx"`
			TX struct {
				Bytes uint64 `json:"bytes"`
			} `json:"tx"`
		} `json:"stats64"`
	}
	if err := runJSON(ctx, c.cmd, &links, "ip", "-n", st.Name, "-s", "-j", "link", "show"); err != nil {
		return 0, 0, false
	}
	var rx, tx uint64
	for _, l := range links {
		if l.Ifname == "lo" {
			continue
		}
		rx += l.Stats.RX.Bytes
		tx += l.Stats.TX.Bytes
	}
	return rx, tx, true
}

func (c *containerCollector) usage(ctx context.Context, st containerState) map[string]interface{} {
	rusage := make(map[string]interface{})

	if mem, err := strconv.ParseUint(c.cgroupFile(st, "memory.current"), 10, 64); err == nil {
		rusage["memory"] = strconv.FormatUint(mem/1024, 10)
	}

	now := time.Now()
	usec := cpuStatUsage(c.cgroupFile(st, "cpu.stat"))
	cpuMu.Lock()
	prev, ok := cpuPrev[st.Name]
	cpuPrev[st.Name] = cpuSample{usec: usec, at: now}
	cpuMu.Unlock()
	if ok && usec >= prev.usec && now.After(prev.at) {
		pct := float64(usec-prev.usec) / float64(now.Sub(prev.at).Microseconds()) * 100
		rusage["cpu"] = fmt.Sprintf("%.2f", pct)
	} else {
		rusage["cpu"] = "0.00"
	}

	if rd, wr := ioStatBytes(c.cgroupFile(st, "io.stat")); rd > 0 || wr > 0 {
		bio := make(map[string]interface{})
		if rd > 0 {
			bio["read"] = strconv.FormatUint(rd/1024, 10)
		}
		if wr > 0 {
			bio["write"] = strconv.FormatUint(wr/1024, 10)
		}
		rusage["block-io"] = bio
	}

	if rx, tx, ok := c.netIO(ctx, st); ok && (rx > 0 || tx > 0) {
		nio := make(map[string]interface{})
		if rx > 0 {
			nio["received"] = strconv.FormatUint(rx/1024, 10)
		}
		if tx > 0 {
			nio["sent"] = strconv.FormatUint(tx/1024, 10)
		}
		rusage["net-io"] = nio
	}

	if pids, err := strconv.Atoi(c.cgroupFile(st, "pids.current")); err == nil {
		rusage["pids"] = pids
	}

	return rusage
}

func (c *containerCollector) network(st containerState, running bool) map[string]interface{} {
	if st.Networks == nil {
		return map[string]interface{}{"host": true}
	}

	ifaces := []interface{}{}
	for _, n := range *st.Networks {
		ifaces = append(ifaces, map[string]interface{}{"name": n.Name})
	}
	publish := []interface{}{}
	if running {
		for _, p := range st.Publish {
			publish = append(publish, p)
		}
	}
	return map[string]interface{}{"interface": ifaces, "publish": publish}
}

func (c *containerCollector) container(ctx context.Context, st containerState) map[string]interface{} {
	running := c.running(ctx, st.Name)

	status := st.Status
	if running {
		status = "Up " + time.Since(time.Unix(int64(st.Started), 0)).Round(time.Second).String()
	}

	out := map[string]interface{}{
		"name":    st.Name,
		"running": running,
		"status":  status,
		"network": c.network(st, running),
	}
	// A failed setup leaves only name and status behind; an empty
	// string fails the YANG patterns and would drop the whole entry.
	for key, val := range map[string]string{"id": st.ID, "image": st.Image, "image-id": st.ImageID, "cmdline": st.Cmdline} {
		if val != "" {
			out[key] = val
		}
	}
	if limits := c.limits(st); limits != nil {
		out["resource-limit"] = limits
	}
	if running {
		out["resource-usage"] = c.usage(ctx, st)
	}

	return out
}

func parseCgroupMemory(memStr string) int {
	memStr = strings.TrimSpace(memStr)
	if memStr == "" || memStr == "max" {
		return 0
	}

	memBytes, err := strconv.ParseUint(memStr, 10, 64)
	if err != nil {
		return 0
	}

	return int(memBytes / 1024)
}

func parseCgroupCPU(cpuStr string) int {
	cpuStr = strings.TrimSpace(cpuStr)
	if cpuStr == "" {
		return 0
	}

	parts := strings.Fields(cpuStr)
	if len(parts) != 2 || parts[0] == "max" {
		return 0
	}

	quota, err := strconv.Atoi(parts[0])
	if err != nil {
		return 0
	}
	period, err := strconv.Atoi(parts[1])
	if err != nil || period == 0 {
		return 0
	}

	return (quota * 1000) / period
}
