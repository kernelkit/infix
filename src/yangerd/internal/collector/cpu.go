package collector

import (
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

const sysCPU = "/sys/devices/system/cpu"

// x86 model name noise: Intel(R) Atom(TM) CPU C3558 @ 2.20GHz
var cpuModelNoiseRe = regexp.MustCompile(`\((R|TM|tm)\)|@ *[0-9.]+ *GHz|\b(\d+|Dual|Quad|Six|Eight)[ -]Core Processor\b`)

// SoC vendors by device tree prefix, others are capitalized
var socVendors = map[string]string{
	"brcm":     "Broadcom",
	"fsl":      "NXP",
	"mediatek": "MediaTek",
	"nxp":      "NXP",
	"qcom":     "Qualcomm",
	"spacemit": "SpacemiT",
	"st":       "ST",
	"starfive": "StarFive",
	"ti":       "TI",
	"xlnx":     "Xilinx",
}

var x86Vendors = map[string]string{
	"GenuineIntel": "Intel",
	"AuthenticAMD": "AMD",
}

// cpuInfo is what the cpu component has that is fixed for the boot.
type cpuInfo struct {
	vendor, model string
	clusters      []cpuCluster
}

type cpuCluster struct {
	id, cores int
	model     string
	min, max  int   // MHz, 0 when unknown
	clocks    []int // one CPU of each cpufreq policy in the cluster
}

// cpuComponentFor is the SoC, with the cores and clocks of its clusters.
// Without either cores or die sensors there is nothing to say about it.
func (c *HardwareCollector) cpuComponentFor(systemjson map[string]interface{}, sensors []interface{}) []interface{} {
	c.cpuOnce.Do(func() { c.cpu = c.readCPUInfo(systemjson) })

	hasSensor := false
	for _, raw := range sensors {
		if sensor, ok := raw.(map[string]interface{}); ok && sensor["parent"] == cpuComponent {
			hasSensor = true
			break
		}
	}
	if len(c.cpu.clusters) == 0 && !hasSensor {
		return nil
	}

	component := map[string]interface{}{
		"name":   cpuComponent,
		"class":  "iana-hardware:cpu",
		"parent": "mainboard",
		"state": map[string]interface{}{
			"admin-state": "unknown",
			"oper-state":  "enabled",
		},
	}
	if c.cpu.vendor != "" {
		component["mfg-name"] = c.cpu.vendor
	}
	if c.cpu.model != "" {
		component["model-name"] = c.cpu.model
	}
	if len(c.cpu.clusters) > 0 {
		list := make([]interface{}, 0, len(c.cpu.clusters))
		for _, cl := range c.cpu.clusters {
			entry := map[string]interface{}{"id": cl.id, "cores": cl.cores}
			if cl.model != "" {
				entry["model"] = cl.model
			}
			if cur := c.clusterClock(cl); cur > 0 {
				entry["frequency"] = cur
			}
			if cl.min > 0 {
				entry["min-frequency"] = cl.min
			}
			if cl.max > 0 {
				entry["max-frequency"] = cl.max
			}
			list = append(list, entry)
		}
		component["infix-hardware:cpu"] = map[string]interface{}{"cluster": list}
	}
	return []interface{}{component}
}

// clusterClock is the highest current clock of the cluster's policies.
func (c *HardwareCollector) clusterClock(cl cpuCluster) int {
	max := 0
	for _, n := range cl.clocks {
		if khz, ok := c.readSensorInt(cpuPath(n, "cpufreq/scaling_cur_freq")); ok && khz/1000 > max {
			max = khz / 1000
		}
	}
	return max
}

func cpuPath(n int, file string) string {
	return sysCPU + "/cpu" + strconv.Itoa(n) + "/" + file
}

// readCPUInfo groups the CPUs into runs of the same core type and clock
// range.  Each cpufreq policy is a clock domain, x86 has one per core.
func (c *HardwareCollector) readCPUInfo(systemjson map[string]interface{}) cpuInfo {
	var info cpuInfo
	info.vendor, info.model = socName(systemjson)
	if info.model == "" {
		info.vendor, info.model = c.cpuinfoModel()
	}

	dirs, _ := c.fs.Glob(sysCPU + "/cpu[0-9]*")
	var cpus []int
	for _, dir := range dirs {
		if n, err := strconv.Atoi(strings.TrimPrefix(filepath.Base(dir), "cpu")); err == nil {
			cpus = append(cpus, n)
		}
	}
	sort.Ints(cpus)

	clocked := make(map[int]bool) // CPUs whose policy is already read
	for _, n := range cpus {
		compatible, _ := c.readSensorString(cpuPath(n, "of_node/compatible"))
		model := coreModel(compatible)
		min, _ := c.readSensorInt(cpuPath(n, "cpufreq/cpuinfo_min_freq"))
		max, hasFreq := c.readSensorInt(cpuPath(n, "cpufreq/cpuinfo_max_freq"))
		min, max = min/1000, max/1000

		var clock []int
		if hasFreq && !clocked[n] {
			clock = []int{n}
			clocked[n] = true
			related, _ := c.readSensorString(cpuPath(n, "cpufreq/related_cpus"))
			for _, f := range strings.Fields(related) {
				if m, err := strconv.Atoi(f); err == nil {
					clocked[m] = true
				}
			}
		}

		if last := len(info.clusters) - 1; last >= 0 {
			cl := &info.clusters[last]
			if cl.model == model && cl.min == min && cl.max == max {
				cl.cores++
				cl.clocks = append(cl.clocks, clock...)
				continue
			}
		}
		info.clusters = append(info.clusters, cpuCluster{id: n, cores: 1, model: model, min: min, max: max, clocks: clock})
	}
	return info
}

// cpuinfoModel is the x86 vendor and model name, without trademarks and
// clock: Intel, Atom CPU C3558.
func (c *HardwareCollector) cpuinfoModel() (string, string) {
	data, err := c.fs.ReadFile("/proc/cpuinfo")
	if err != nil {
		return "", ""
	}
	var vendor, model string
	for _, line := range strings.Split(string(data), "\n") {
		key, val, ok := strings.Cut(line, ":")
		if !ok {
			continue
		}
		switch strings.TrimSpace(key) {
		case "vendor_id":
			vendor = x86Vendors[strings.TrimSpace(val)]
		case "model name":
			model = strings.Join(strings.Fields(cpuModelNoiseRe.ReplaceAllString(val, " ")), " ")
		}
		if model != "" {
			break
		}
	}
	if vendor != "" {
		model = strings.TrimSpace(strings.TrimPrefix(model, vendor))
	}
	return vendor, model
}

// coreModel turns the first device tree compatible of a core into a
// name: arm,cortex-a55 -> Cortex-A55, sifive,u74-mc -> U74-MC.
func coreModel(compatible string) string {
	first, _, _ := strings.Cut(compatible, "\x00")
	_, name, ok := strings.Cut(first, ",")
	if !ok || name == "" {
		return ""
	}
	parts := strings.Split(name, "-")
	for i, p := range parts {
		if i == 0 && len(p) > 3 {
			parts[i] = strings.ToUpper(p[:1]) + p[1:]
		} else {
			parts[i] = strings.ToUpper(p)
		}
	}
	return strings.Join(parts, "-")
}

// socName is the vendor and model of the SoC, the last device tree
// compatible of the board: rockchip,rk3568 -> Rockchip, RK3568.
func socName(systemjson map[string]interface{}) (string, string) {
	compatible, _ := systemjson["compatible"].([]interface{})
	if len(compatible) == 0 {
		return "", ""
	}
	last, _ := compatible[len(compatible)-1].(string)
	vendor, model, ok := strings.Cut(last, ",")
	if !ok || vendor == "" || model == "" {
		return "", ""
	}
	if name, ok := socVendors[vendor]; ok {
		vendor = name
	} else {
		vendor = strings.ToUpper(vendor[:1]) + vendor[1:]
	}
	return vendor, strings.ToUpper(model)
}
