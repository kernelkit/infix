package collector

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/kernelkit/infix/src/yangerd/internal/tree"
)

var sizeRe = regexp.MustCompile(`(?i)^\s*([0-9.]+)\s*([KMGT]?I?B)?\s*$`)

// ContainerCollector gathers infix-containers operational data.
type ContainerCollector struct {
	cmd      CommandRunner
	fs       FileReader
	interval time.Duration
}

// NewContainerCollector creates a ContainerCollector with the given dependencies.
func NewContainerCollector(cmd CommandRunner, fs FileReader, interval time.Duration) *ContainerCollector {
	return &ContainerCollector{cmd: cmd, fs: fs, interval: interval}
}

// Name implements Collector.
func (c *ContainerCollector) Name() string { return "containers" }

// Interval implements Collector.
func (c *ContainerCollector) Interval() time.Duration { return c.interval }

// Collect implements Collector. It produces one tree key:
// "infix-containers:containers".
func (c *ContainerCollector) Collect(ctx context.Context, t *tree.Tree) error {
	data := c.collectJSON(ctx)
	t.Set("infix-containers:containers", data)
	return nil
}

func (c *ContainerCollector) collectJSON(ctx context.Context) json.RawMessage {
	containers := []interface{}{}

	psList := c.podmanPS(ctx)
	for _, ps := range psList {
		cont := c.container(ctx, ps)
		if cont != nil {
			containers = append(containers, cont)
		}
	}

	out := map[string]interface{}{
		"container": containers,
	}

	data, err := json.Marshal(out)
	if err != nil {
		return json.RawMessage(`{"container":[]}`)
	}
	return data
}

// CollectContainers runs a full container collection and returns the
// result as JSON suitable for tree.Set("infix-containers:containers").
func CollectContainers(cmd CommandRunner, fs FileReader) json.RawMessage {
	c := &ContainerCollector{cmd: cmd, fs: fs}
	return c.collectJSON(context.TODO())
}

func (c *ContainerCollector) podmanPS(ctx context.Context) []map[string]interface{} {
	out, err := c.cmd.Run(ctx, "podman", "ps", "-a", "--format=json")
	if err != nil {
		log.Printf("collector containers: ps: %v", err)
		return nil
	}

	var list []map[string]interface{}
	if err := json.Unmarshal(out, &list); err == nil {
		return list
	}

	var generic []interface{}
	if err := json.Unmarshal(out, &generic); err != nil {
		log.Printf("collector containers: ps parse: %v", err)
		return nil
	}

	for _, item := range generic {
		if m, ok := item.(map[string]interface{}); ok {
			list = append(list, m)
		}
	}

	return list
}

func (c *ContainerCollector) podmanInspect(ctx context.Context, name string) map[string]interface{} {
	out, err := c.cmd.Run(ctx, "podman", "inspect", name)
	if err != nil {
		log.Printf("collector containers: inspect %s: %v", name, err)
		return map[string]interface{}{}
	}

	var list []map[string]interface{}
	if err := json.Unmarshal(out, &list); err == nil && len(list) > 0 {
		return list[0]
	}

	var generic []interface{}
	if err := json.Unmarshal(out, &generic); err == nil {
		for _, item := range generic {
			if m, ok := item.(map[string]interface{}); ok {
				return m
			}
		}
	}

	var single map[string]interface{}
	if err := json.Unmarshal(out, &single); err == nil {
		return single
	}

	log.Printf("collector containers: inspect %s parse: invalid json", name)
	return map[string]interface{}{}
}

func (c *ContainerCollector) resourceStats(ctx context.Context, name string) map[string]interface{} {
	out, err := c.cmd.Run(ctx, "podman", "stats", "--no-stream", "--format", "json", "--no-reset", name)
	if err != nil {
		log.Printf("collector containers: stats %s: %v", name, err)
		return nil
	}

	var statsList []map[string]interface{}
	if err := json.Unmarshal(out, &statsList); err != nil {
		var single map[string]interface{}
		if err2 := json.Unmarshal(out, &single); err2 != nil {
			log.Printf("collector containers: stats %s parse: %v", name, err)
			return nil
		}
		statsList = append(statsList, single)
	}

	if len(statsList) == 0 {
		return nil
	}

	stat := statsList[0]
	rusage := make(map[string]interface{})

	if memUsage, ok := stat["mem_usage"].(string); ok {
		parts := strings.SplitN(memUsage, "/", 2)
		if len(parts) == 2 {
			memKiB := parseSizeKiB(strings.TrimSpace(parts[0]))
			rusage["memory"] = strconv.Itoa(memKiB)
		}
	}

	if cpuPercent, ok := stat["cpu_percent"].(string); ok {
		cpuPercent = strings.TrimSpace(strings.TrimSuffix(cpuPercent, "%"))
		if cpuVal, err := strconv.ParseFloat(cpuPercent, 64); err == nil {
			rusage["cpu"] = fmt.Sprintf("%.2f", cpuVal)
		}
	}

	if blockIO, ok := stat["block_io"].(string); ok {
		parts := strings.SplitN(blockIO, "/", 2)
		if len(parts) == 2 {
			readKiB := parseSizeKiB(strings.TrimSpace(parts[0]))
			writeKiB := parseSizeKiB(strings.TrimSpace(parts[1]))

			bio := make(map[string]interface{})
			if readKiB > 0 {
				bio["read"] = strconv.Itoa(readKiB)
			}
			if writeKiB > 0 {
				bio["write"] = strconv.Itoa(writeKiB)
			}
			rusage["block-io"] = bio
		}
	}

	if netIO, ok := stat["net_io"].(string); ok {
		parts := strings.SplitN(netIO, "/", 2)
		if len(parts) == 2 {
			rxKiB := parseSizeKiB(strings.TrimSpace(parts[0]))
			txKiB := parseSizeKiB(strings.TrimSpace(parts[1]))

			nio := make(map[string]interface{})
			if rxKiB > 0 {
				nio["received"] = strconv.Itoa(rxKiB)
			}
			if txKiB > 0 {
				nio["sent"] = strconv.Itoa(txKiB)
			}
			rusage["net-io"] = nio
		}
	}

	if pids, ok := stat["pids"]; ok {
		pidInt := toInt(pids)
		rusage["pids"] = pidInt
	}

	if len(rusage) == 0 {
		return nil
	}

	return rusage
}

func (c *ContainerCollector) readCgroupLimits(inspect map[string]interface{}) map[string]interface{} {
	stateRaw, ok := inspect["State"]
	if !ok {
		return nil
	}
	state, ok := stateRaw.(map[string]interface{})
	if !ok {
		return nil
	}

	cgroupPath, ok := state["CgroupPath"].(string)
	if !ok || cgroupPath == "" {
		return nil
	}

	cgroupBase := "/sys/fs/cgroup" + cgroupPath
	memVal := 0
	cpuVal := 0

	if data, err := c.fs.ReadFile(filepath.Join(cgroupBase, "memory.max")); err == nil {
		memVal = parseCgroupMemory(strings.TrimSpace(string(data)))
	}

	if data, err := c.fs.ReadFile(filepath.Join(cgroupBase, "cpu.max")); err == nil {
		cpuVal = parseCgroupCPU(strings.TrimSpace(string(data)))
	}

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

func (c *ContainerCollector) network(ps map[string]interface{}, inspect map[string]interface{}) map[string]interface{} {
	networkSettingsRaw, hasNetworkSettings := inspect["NetworkSettings"]
	if hasNetworkSettings {
		if networkSettings, ok := networkSettingsRaw.(map[string]interface{}); ok {
			if networksRaw, ok := networkSettings["Networks"]; ok {
				if networks, ok := networksRaw.(map[string]interface{}); ok {
					if _, ok := networks["host"]; ok {
						return map[string]interface{}{"host": true}
					}
				}
			}
		}
	}

	net := map[string]interface{}{
		"interface": []interface{}{},
		"publish":   []interface{}{},
	}

	networks := asStringSlice(ps["Networks"])
	ifaces := net["interface"].([]interface{})
	for _, n := range networks {
		ifaces = append(ifaces, map[string]interface{}{"name": n})
	}
	net["interface"] = ifaces

	running := strings.EqualFold(asString(ps["State"]), "running")
	if !running {
		return net
	}

	portsRaw, ok := ps["Ports"]
	if !ok {
		return net
	}

	ports, ok := portsRaw.([]interface{})
	if !ok || len(ports) == 0 {
		return net
	}

	publish := net["publish"].([]interface{})
	for _, portRaw := range ports {
		port, ok := portRaw.(map[string]interface{})
		if !ok {
			continue
		}

		hostIP := asString(port["host_ip"])
		hostPort := asString(port["host_port"])
		if hostPort == "" {
			hostPort = strconv.Itoa(toInt(port["host_port"]))
		}
		containerPort := asString(port["container_port"])
		if containerPort == "" {
			containerPort = strconv.Itoa(toInt(port["container_port"]))
		}
		protocol := asString(port["protocol"])

		if hostPort == "0" || hostPort == "" || containerPort == "0" || containerPort == "" || protocol == "" {
			continue
		}

		addr := ""
		if hostIP != "" {
			addr = hostIP + ":"
		}

		publish = append(publish, fmt.Sprintf("%s%s:%s/%s", addr, hostPort, containerPort, protocol))
	}
	net["publish"] = publish

	return net
}

func (c *ContainerCollector) container(ctx context.Context, ps map[string]interface{}) map[string]interface{} {
	names := asStringSlice(ps["Names"])
	if len(names) == 0 {
		return nil
	}

	name := names[0]
	running := strings.EqualFold(asString(ps["State"]), "running")

	out := map[string]interface{}{
		"name":     name,
		"id":       asString(ps["Id"]),
		"image":    asString(ps["Image"]),
		"image-id": asString(ps["ImageID"]),
		"running":  running,
		"status":   asString(ps["Status"]),
	}

	cmd := strings.Join(asStringSlice(ps["Command"]), " ")
	if cmd != "" {
		out["command"] = cmd
	}

	inspect := c.podmanInspect(ctx, name)
	if net := c.network(ps, inspect); len(net) > 0 {
		out["network"] = net
	}

	if limits := c.readCgroupLimits(inspect); limits != nil {
		out["resource-limit"] = limits
	}

	if running {
		if usage := c.resourceStats(ctx, name); usage != nil {
			out["resource-usage"] = usage
		}
	}

	return out
}

func asString(v interface{}) string {
	s, ok := v.(string)
	if ok {
		return s
	}
	return ""
}

func asStringSlice(v interface{}) []string {
	switch vv := v.(type) {
	case []string:
		return vv
	case []interface{}:
		out := make([]string, 0, len(vv))
		for _, e := range vv {
			if s, ok := e.(string); ok && s != "" {
				out = append(out, s)
			}
		}
		return out
	case string:
		if vv == "" {
			return nil
		}
		return splitLines(vv)
	default:
		return nil
	}
}

func parseSizeKiB(sizeStr string) int {
	if strings.TrimSpace(sizeStr) == "" {
		return 0
	}

	m := sizeRe.FindStringSubmatch(strings.ToUpper(strings.TrimSpace(sizeStr)))
	if len(m) < 2 {
		return 0
	}

	value, err := strconv.ParseFloat(m[1], 64)
	if err != nil {
		return 0
	}

	unit := "B"
	if len(m) >= 3 && m[2] != "" {
		unit = strings.ToUpper(m[2])
	}

	multipliers := map[string]float64{
		"B":   1.0 / 1024.0,
		"KB":  1000.0 / 1024.0,
		"KIB": 1,
		"MB":  (1000.0 * 1000.0) / 1024.0,
		"MIB": 1024,
		"GB":  (1000.0 * 1000.0 * 1000.0) / 1024.0,
		"GIB": 1024 * 1024,
		"TB":  (1000.0 * 1000.0 * 1000.0 * 1000.0) / 1024.0,
		"TIB": 1024 * 1024 * 1024,
	}

	mult, ok := multipliers[unit]
	if !ok {
		mult = 1
	}

	return int(value * mult)
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
