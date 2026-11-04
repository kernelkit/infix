package collector

import (
	"reflect"
	"strconv"
	"strings"
	"testing"

	"github.com/kernelkit/infix/src/yangerd/internal/testutil"
)

type fakeCPU struct {
	compatible    string
	cur, min, max int // kHz, 0 for no cpufreq
}

func cpuFS(cpus []fakeCPU, files map[string][]byte) *testutil.MockFileReader {
	fs := &testutil.MockFileReader{Files: map[string][]byte{"/run/system.json": []byte(`{"compatible":["test"]}`)}, Globs: map[string][]string{}}
	for k, v := range files {
		fs.Files[k] = v
	}
	var dirs []string
	for n, cpu := range cpus {
		dir := sysCPU + "/cpu" + strconv.Itoa(n)
		dirs = append(dirs, dir)
		if cpu.compatible != "" {
			fs.Files[dir+"/of_node/compatible"] = []byte(cpu.compatible)
		}
		if cpu.max > 0 {
			fs.Files[dir+"/cpufreq/related_cpus"] = []byte(strconv.Itoa(n) + "\n")
			fs.Files[dir+"/cpufreq/scaling_cur_freq"] = []byte(strconv.Itoa(cpu.cur) + "\n")
			fs.Files[dir+"/cpufreq/cpuinfo_min_freq"] = []byte(strconv.Itoa(cpu.min) + "\n")
			fs.Files[dir+"/cpufreq/cpuinfo_max_freq"] = []byte(strconv.Itoa(cpu.max) + "\n")
		}
	}
	fs.Globs[sysCPU+"/cpu[0-9]*"] = dirs
	return fs
}

func cpuClustersOf(t *testing.T, fs *testutil.MockFileReader) (map[string]interface{}, []interface{}) {
	t.Helper()
	runner := &testutil.MockRunner{Results: map[string][]byte{}, Errors: map[string]error{}}
	cpu := getComponentByName(collectHardware(t, newHardwareCollector(runner, fs)), "cpu")
	if cpu == nil {
		t.Fatal("expected a cpu component")
	}
	if cpu["class"] != "iana-hardware:cpu" || cpu["parent"] != "mainboard" {
		t.Fatalf("cpu component mismatch: %v", cpu)
	}
	data, ok := cpu["infix-hardware:cpu"].(map[string]interface{})
	if !ok {
		return cpu, nil
	}
	return cpu, data["cluster"].([]interface{})
}

func wantCluster(t *testing.T, got interface{}, want map[string]interface{}) {
	t.Helper()
	cl := got.(map[string]interface{})
	norm := make(map[string]interface{}, len(cl))
	for k, v := range cl {
		if s, ok := v.(string); ok {
			norm[k] = s
		} else {
			norm[k] = toInt(v)
		}
	}
	if !reflect.DeepEqual(norm, want) {
		t.Fatalf("cluster mismatch:\n got %v\nwant %v", norm, want)
	}
}

func TestCPUSingleCluster(t *testing.T) {
	a55 := fakeCPU{"arm,cortex-a55\x00arm,armv8\x00", 1992000, 408000, 1992000}
	fs := cpuFS([]fakeCPU{a55, a55, a55, a55}, map[string][]byte{
		"/run/system.json": []byte(`{"compatible":["sinovoip,rk3568-bpi-r2pro","rockchip,rk3568"]}`),
	})

	cpu, clusters := cpuClustersOf(t, fs)
	if cpu["mfg-name"] != "Rockchip" || cpu["model-name"] != "RK3568" {
		t.Fatalf("SoC name mismatch: %v %v", cpu["mfg-name"], cpu["model-name"])
	}
	if len(clusters) != 1 {
		t.Fatalf("expected one cluster, got %v", clusters)
	}
	wantCluster(t, clusters[0], map[string]interface{}{
		"id": 0, "model": "Cortex-A55", "cores": 4,
		"frequency": 1992, "min-frequency": 408, "max-frequency": 1992,
	})
}

// RK3588: the A76 pairs are separate clock domains with the same range.
func TestCPUBigLittle(t *testing.T) {
	a55 := fakeCPU{"arm,cortex-a55\x00", 1800000, 408000, 1800000}
	a76a := fakeCPU{"arm,cortex-a76\x00", 1200000, 408000, 2256000}
	a76b := fakeCPU{"arm,cortex-a76\x00", 2256000, 408000, 2256000}
	fs := cpuFS([]fakeCPU{a55, a55, a55, a55, a76a, a76a, a76b, a76b}, map[string][]byte{
		"/run/system.json": []byte(`{"compatible":["radxa,rock-5b","rockchip,rk3588"]}`),
	})

	_, clusters := cpuClustersOf(t, fs)
	if len(clusters) != 2 {
		t.Fatalf("expected two clusters, got %v", clusters)
	}
	wantCluster(t, clusters[0], map[string]interface{}{
		"id": 0, "model": "Cortex-A55", "cores": 4,
		"frequency": 1800, "min-frequency": 408, "max-frequency": 1800,
	})
	wantCluster(t, clusters[1], map[string]interface{}{
		"id": 4, "model": "Cortex-A76", "cores": 4,
		"frequency": 2256, "min-frequency": 408, "max-frequency": 2256,
	})
}

// No device tree, the model comes from /proc/cpuinfo.
func TestCPUx86(t *testing.T) {
	c0 := fakeCPU{"", 800000, 800000, 2200000}
	c1 := fakeCPU{"", 2100000, 800000, 2200000}
	fs := cpuFS([]fakeCPU{c0, c1}, map[string][]byte{
		"/run/system.json": []byte(`{}`),
		"/proc/cpuinfo":    []byte("processor\t: 0\nvendor_id\t: GenuineIntel\nmodel name\t: Intel(R) Atom(TM) CPU C3558 @ 2.20GHz\n\nprocessor\t: 1\nmodel name\t: Intel(R) Atom(TM) CPU C3558 @ 2.20GHz\n"),
	})

	cpu, clusters := cpuClustersOf(t, fs)
	if cpu["mfg-name"] != "Intel" || cpu["model-name"] != "Atom CPU C3558" {
		t.Fatalf("x86 name mismatch: %v %v", cpu["mfg-name"], cpu["model-name"])
	}
	if len(clusters) != 1 {
		t.Fatalf("expected one cluster, got %v", clusters)
	}
	wantCluster(t, clusters[0], map[string]interface{}{
		"id": 0, "cores": 2,
		"frequency": 2100, "min-frequency": 800, "max-frequency": 2200,
	})
}

func TestCPUNoFrequencyScaling(t *testing.T) {
	a53 := fakeCPU{compatible: "arm,cortex-a53\x00"}
	_, clusters := cpuClustersOf(t, cpuFS([]fakeCPU{a53, a53}, nil))
	if len(clusters) != 1 {
		t.Fatalf("expected one cluster, got %v", clusters)
	}
	wantCluster(t, clusters[0], map[string]interface{}{"id": 0, "model": "Cortex-A53", "cores": 2})
}

func TestCPUModelNames(t *testing.T) {
	for in, want := range map[string]string{
		"arm,cortex-a72\x00":         "Cortex-A72",
		"sifive,u74-mc\x00riscv\x00": "U74-MC",
		"spacemit,x60\x00":           "X60",
		"riscv\x00":                  "",
	} {
		if got := coreModel(in); got != want {
			t.Errorf("coreModel(%q) = %q, want %q", in, got, want)
		}
	}

	runner := &testutil.MockRunner{Results: map[string][]byte{}, Errors: map[string]error{}}
	for in, want := range map[string][2]string{
		"AuthenticAMD|AMD Ryzen 7 5800X 8-Core Processor":       {"AMD", "Ryzen 7 5800X"},
		"GenuineIntel|Intel(R) Core(TM) i7-8650U CPU @ 1.90GHz": {"Intel", "Core i7-8650U CPU"},
		"AuthenticAMD|QEMU Virtual CPU version 2.5+":            {"AMD", "QEMU Virtual CPU version 2.5+"},
	} {
		id, name, _ := strings.Cut(in, "|")
		fs := &testutil.MockFileReader{Files: map[string][]byte{
			"/proc/cpuinfo": []byte("vendor_id\t: " + id + "\nmodel name\t: " + name + "\n"),
		}}
		vendor, model := newHardwareCollector(runner, fs).cpuinfoModel()
		if vendor != want[0] || model != want[1] {
			t.Errorf("cpuinfoModel(%q) = %q %q, want %q", in, vendor, model, want)
		}
	}
}
