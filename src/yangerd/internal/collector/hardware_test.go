package collector

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/kernelkit/infix/src/yangerd/internal/testutil"
	"github.com/kernelkit/infix/src/yangerd/internal/tree"
)

func collectHardware(t *testing.T, c *HardwareCollector) []interface{} {
	t.Helper()

	tr := tree.New()
	tr.RegisterProvider(hardwareKey, c.Live)
	if err := c.Collect(context.Background(), tr); err != nil {
		t.Fatalf("Collect failed: %v", err)
	}

	raw := tr.Get("ietf-hardware:hardware")
	if raw == nil {
		t.Fatal("missing ietf-hardware:hardware in tree")
	}

	var out map[string]interface{}
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatalf("unmarshal hardware: %v", err)
	}

	components, ok := out["component"].([]interface{})
	if !ok {
		t.Fatalf("component list missing or invalid: %v", out["component"])
	}

	return components
}

func getComponentByName(components []interface{}, name string) map[string]interface{} {
	for _, c := range components {
		m, ok := c.(map[string]interface{})
		if !ok {
			continue
		}
		if m["name"] == name {
			return m
		}
	}
	return nil
}

func containsComponentWithClass(components []interface{}, class string) bool {
	for _, c := range components {
		m, ok := c.(map[string]interface{})
		if !ok {
			continue
		}
		if m["class"] == class {
			return true
		}
	}
	return false
}

func newHardwareCollector(r *testutil.MockRunner, fs *testutil.MockFileReader) *HardwareCollector {
	return NewHardwareCollector(r, fs, 30*time.Second, false, false)
}

func TestHardwareMotherboard(t *testing.T) {
	runner := &testutil.MockRunner{Results: map[string][]byte{}, Errors: map[string]error{}}
	fs := &testutil.MockFileReader{Files: map[string][]byte{
		"/run/system.json": []byte(`{"vendor":"Acme","product-name":"Router-1","serial-number":"SN123","part-number":"PN99","mac-address":"00:11:22:33:44:55"}`),
	}, Globs: map[string][]string{}}

	components := collectHardware(t, newHardwareCollector(runner, fs))
	mb := getComponentByName(components, "mainboard")
	if mb == nil {
		t.Fatal("mainboard component not found")
	}

	if mb["class"] != "iana-hardware:chassis" {
		t.Fatalf("mainboard class: expected chassis, got %v", mb["class"])
	}
	if mb["mfg-name"] != "Acme" || mb["model-name"] != "Router-1" || mb["serial-num"] != "SN123" {
		t.Fatalf("mainboard identity fields mismatch: %v", mb)
	}
	if mb["hardware-rev"] != "PN99" {
		t.Fatalf("mainboard hardware-rev mismatch: %v", mb["hardware-rev"])
	}
	if mb["infix-hardware:phys-address"] != "00:11:22:33:44:55" {
		t.Fatalf("mainboard phys-address mismatch: %v", mb["infix-hardware:phys-address"])
	}

	state, ok := mb["state"].(map[string]interface{})
	if !ok {
		t.Fatalf("mainboard state missing: %v", mb["state"])
	}
	if state["admin-state"] != "unknown" || state["oper-state"] != "enabled" {
		t.Fatalf("mainboard state mismatch: %v", state)
	}
}

func TestHardwareVPD(t *testing.T) {
	runner := &testutil.MockRunner{Results: map[string][]byte{}, Errors: map[string]error{}}
	fs := &testutil.MockFileReader{Files: map[string][]byte{
		"/run/system.json": []byte(`{
			"vpd": {
				"slot0": {
					"board": "board0",
					"data": {
						"manufacture-date": "04/11/2026 13:14:15",
						"manufacturer": "VPD Inc",
						"product-name": "X1",
						"serial-number": "VPD-123",
						"foo": "bar",
						"vendor-extension": [[32473, "aa55"]]
					}
				}
			}
		}`),
	}, Globs: map[string][]string{}}

	components := collectHardware(t, newHardwareCollector(runner, fs))
	vpd := getComponentByName(components, "vpd-board0")
	if vpd == nil {
		t.Fatal("vpd component vpd-board0 not found")
	}
	if vpd["class"] != "infix-hardware:vpd" {
		t.Fatalf("vpd class mismatch: %v", vpd["class"])
	}
	if vpd["mfg-date"] != "2026-04-11T13:14:15Z" {
		t.Fatalf("mfg-date mismatch: %v", vpd["mfg-date"])
	}
	if vpd["serial-num"] != "VPD-123" {
		t.Fatalf("serial-num mismatch: %v", vpd["serial-num"])
	}

	vpdData, ok := vpd["infix-hardware:vpd-data"].(map[string]interface{})
	if !ok {
		t.Fatalf("vpd-data missing: %v", vpd["infix-hardware:vpd-data"])
	}
	if vpdData["foo"] != "bar" {
		t.Fatalf("vpd-data foo mismatch: %v", vpdData["foo"])
	}
	extList, ok := vpdData["infix-hardware:vendor-extension"].([]interface{})
	if !ok || len(extList) != 1 {
		t.Fatalf("vendor-extension missing: %v", vpdData["infix-hardware:vendor-extension"])
	}
	ext := extList[0].(map[string]interface{})
	if toInt(ext["iana-enterprise-number"]) != 32473 || ext["extension-data"] != "aa55" {
		t.Fatalf("vendor-extension mismatch: %v", ext)
	}
}

// A port is unlocked only when every root hub under it accepts devices
// by default: usb-b spans a USB 2 and a USB 3 hub and the latter is
// locked.  A port whose hubs are not in sysfs is left out.
func TestHardwareUSBPorts(t *testing.T) {
	runner := &testutil.MockRunner{Results: map[string][]byte{}, Errors: map[string]error{}}
	fs := &testutil.MockFileReader{Files: map[string][]byte{
		"/run/system.json": []byte(`{"usb-ports":[` +
			`{"name":"usb-a","paths":["/sys/bus/usb/devices/usb1"]},` +
			`{"name":"usb-b","paths":["/sys/bus/usb/devices/usb2","/sys/bus/usb/devices/usb3"]},` +
			`{"name":"usb-c","paths":["/sys/bus/usb/devices/usb4"]}]}`),
		"/sys/bus/usb/devices/usb1/authorized_default": []byte("1\n"),
		"/sys/bus/usb/devices/usb2/authorized_default": []byte("1\n"),
		"/sys/bus/usb/devices/usb3/authorized_default": []byte("0\n"),
	}, Globs: map[string][]string{}}

	components := collectHardware(t, newHardwareCollector(runner, fs))
	usbA := getComponentByName(components, "usb-a")
	usbB := getComponentByName(components, "usb-b")
	if usbA == nil || usbB == nil {
		t.Fatalf("usb components missing: usb-a=%v usb-b=%v", usbA, usbB)
	}
	aState := usbA["state"].(map[string]interface{})
	bState := usbB["state"].(map[string]interface{})
	if aState["admin-state"] != "unlocked" || bState["admin-state"] != "locked" {
		t.Fatalf("usb admin-state mismatch: a=%v b=%v", aState, bState)
	}
	if usbC := getComponentByName(components, "usb-c"); usbC != nil {
		t.Fatalf("usb-c has no hub in sysfs, got %v", usbC)
	}
}

func TestHardwareHwmonTemp(t *testing.T) {
	runner := &testutil.MockRunner{Results: map[string][]byte{}, Errors: map[string]error{}}
	fs := &testutil.MockFileReader{Files: map[string][]byte{
		"/run/system.json":                    []byte(`{}`),
		"/sys/class/hwmon/hwmon0/name":        []byte("cpu_thermal\n"),
		"/sys/class/hwmon/hwmon0/temp1_input": []byte("42000\n"),
		"/sys/class/hwmon/hwmon0/temp1_label": []byte("cpu_temp\n"),
	}, Globs: map[string][]string{
		"/sys/class/hwmon/*":        {"/sys/class/hwmon/hwmon0"},
		"/sys/class/hwmon/hwmon0/*": {"/sys/class/hwmon/hwmon0/name", "/sys/class/hwmon/hwmon0/temp1_input", "/sys/class/hwmon/hwmon0/temp1_label"},
	}}

	components := collectHardware(t, newHardwareCollector(runner, fs))
	if !containsComponentWithClass(components, "iana-hardware:sensor") {
		t.Fatalf("expected at least one sensor component: %v", components)
	}
	sensor := getComponentByName(components, "cpu-thermal-cpu-temp")
	if sensor == nil {
		t.Fatalf("expected temp sensor cpu-thermal-cpu-temp, got: %v", components)
	}
	sd := sensor["sensor-data"].(map[string]interface{})
	if toInt(sd["value"]) != 42000 || sd["value-type"] != "celsius" || sd["value-scale"] != "milli" {
		t.Fatalf("temp sensor-data mismatch: %v", sd)
	}
	if sensor["parent"] != "cpu" {
		t.Fatalf("die sensor must hang off the cpu, got parent %v", sensor["parent"])
	}
	if cpu := getComponentByName(components, "cpu"); cpu == nil || cpu["class"] != "iana-hardware:cpu" || cpu["parent"] != "mainboard" {
		t.Fatalf("expected cpu component under mainboard, got %v", cpu)
	}
}

func TestHardwareHwmonFan(t *testing.T) {
	runner := &testutil.MockRunner{Results: map[string][]byte{}, Errors: map[string]error{}}
	fs := &testutil.MockFileReader{Files: map[string][]byte{
		"/run/system.json":                   []byte(`{}`),
		"/sys/class/hwmon/hwmon1/name":       []byte("pwmfan\n"),
		"/sys/class/hwmon/hwmon1/fan1_input": []byte("3200\n"),
	}, Globs: map[string][]string{
		"/sys/class/hwmon/*":        {"/sys/class/hwmon/hwmon1"},
		"/sys/class/hwmon/hwmon1/*": {"/sys/class/hwmon/hwmon1/name", "/sys/class/hwmon/hwmon1/fan1_input"},
	}}

	components := collectHardware(t, newHardwareCollector(runner, fs))
	sensor := getComponentByName(components, "pwmfan")
	if sensor == nil {
		t.Fatalf("expected fan sensor pwmfan, got: %v", components)
	}
	sd := sensor["sensor-data"].(map[string]interface{})
	if toInt(sd["value"]) != 3200 || sd["value-type"] != "rpm" || sd["value-scale"] != "units" {
		t.Fatalf("fan sensor-data mismatch: %v", sd)
	}
	if _, ok := sensor["parent"]; ok {
		t.Fatalf("a lone fan has no parent, got %v", sensor["parent"])
	}
	if getComponentByName(components, "cpu") != nil {
		t.Fatal("no die sensor, so no cpu component")
	}
}

func TestHardwareHwmonVoltage(t *testing.T) {
	runner := &testutil.MockRunner{Results: map[string][]byte{}, Errors: map[string]error{}}
	fs := &testutil.MockFileReader{Files: map[string][]byte{
		"/run/system.json":                  []byte(`{}`),
		"/sys/class/hwmon/hwmon2/name":      []byte("ina3221\n"),
		"/sys/class/hwmon/hwmon2/in1_input": []byte("12000\n"),
		"/sys/class/hwmon/hwmon2/in1_label": []byte("VCC\n"),
	}, Globs: map[string][]string{
		"/sys/class/hwmon/*":        {"/sys/class/hwmon/hwmon2"},
		"/sys/class/hwmon/hwmon2/*": {"/sys/class/hwmon/hwmon2/name", "/sys/class/hwmon/hwmon2/in1_input", "/sys/class/hwmon/hwmon2/in1_label"},
	}}

	components := collectHardware(t, newHardwareCollector(runner, fs))
	sensor := getComponentByName(components, "ina3221-VCC")
	if sensor == nil {
		t.Fatalf("expected voltage sensor ina3221-VCC, got: %v", components)
	}
	if sensor["description"] != "VCC" {
		t.Fatalf("expected VCC description, got %v", sensor["description"])
	}
	sd := sensor["sensor-data"].(map[string]interface{})
	if toInt(sd["value"]) != 12000 || sd["value-type"] != "volts-DC" || sd["value-scale"] != "milli" {
		t.Fatalf("voltage sensor-data mismatch: %v", sd)
	}
}

func TestHardwareHwmonMultiSensor(t *testing.T) {
	runner := &testutil.MockRunner{Results: map[string][]byte{}, Errors: map[string]error{}}
	fs := &testutil.MockFileReader{Files: map[string][]byte{
		"/run/system.json":                     []byte(`{}`),
		"/sys/class/hwmon/hwmon3/name":         []byte("sfp_2\n"),
		"/sys/class/hwmon/hwmon3/temp1_input":  []byte("33000\n"),
		"/sys/class/hwmon/hwmon3/temp1_label":  []byte("temp1\n"),
		"/sys/class/hwmon/hwmon3/fan1_input":   []byte("2000\n"),
		"/sys/class/hwmon/hwmon3/fan1_label":   []byte("fan1\n"),
		"/sys/class/hwmon/hwmon3/curr1_input":  []byte("1500\n"),
		"/sys/class/hwmon/hwmon3/power1_input": []byte("2500000\n"),
	}, Globs: map[string][]string{
		"/sys/class/hwmon/*":        {"/sys/class/hwmon/hwmon3"},
		"/sys/class/hwmon/hwmon3/*": {"/sys/class/hwmon/hwmon3/name", "/sys/class/hwmon/hwmon3/temp1_input", "/sys/class/hwmon/hwmon3/temp1_label", "/sys/class/hwmon/hwmon3/fan1_input", "/sys/class/hwmon/hwmon3/fan1_label", "/sys/class/hwmon/hwmon3/curr1_input", "/sys/class/hwmon/hwmon3/power1_input"},
	}}

	components := collectHardware(t, newHardwareCollector(runner, fs))
	parent := getComponentByName(components, "sfp2")
	if parent == nil || parent["class"] != "iana-hardware:module" {
		t.Fatalf("expected sfp2 parent module, got: %v", parent)
	}

	children := 0
	hasCurrent := false
	hasPower := false
	for _, compRaw := range components {
		comp, ok := compRaw.(map[string]interface{})
		if !ok {
			continue
		}
		if comp["parent"] != "sfp2" {
			continue
		}
		children++
		sd, _ := comp["sensor-data"].(map[string]interface{})
		if sd != nil && sd["value-type"] == "amperes" {
			hasCurrent = true
		}
		if sd != nil && sd["value-type"] == "watts" {
			hasPower = true
		}
	}
	if children < 4 {
		t.Fatalf("expected at least 4 child sensors, got %d", children)
	}
	if !hasCurrent || !hasPower {
		t.Fatalf("expected current and power sensors under parent: current=%v power=%v", hasCurrent, hasPower)
	}
}

func TestHardwareThermalZone(t *testing.T) {
	runner := &testutil.MockRunner{Results: map[string][]byte{}, Errors: map[string]error{}}
	fs := &testutil.MockFileReader{Files: map[string][]byte{
		"/run/system.json":                      []byte(`{}`),
		"/sys/class/thermal/thermal_zone0/type": []byte("cpu-thermal\n"),
		"/sys/class/thermal/thermal_zone0/temp": []byte("39000\n"),
	}, Globs: map[string][]string{
		"/sys/class/thermal/*": {"/sys/class/thermal/thermal_zone0"},
	}}

	components := collectHardware(t, newHardwareCollector(runner, fs))
	sensor := getComponentByName(components, "cpu-thermal")
	if sensor == nil {
		t.Fatalf("expected thermal sensor cpu-thermal, got %v", components)
	}
	sd := sensor["sensor-data"].(map[string]interface{})
	if toInt(sd["value"]) != 39000 || sd["value-type"] != "celsius" {
		t.Fatalf("thermal sensor mismatch: %v", sd)
	}
	if sensor["parent"] != "cpu" {
		t.Fatalf("thermal die sensor must hang off the cpu, got %v", sensor["parent"])
	}
}

func TestHardwareNormalizeSensorName(t *testing.T) {
	tests := []struct {
		in   string
		want string
	}{
		{in: "sfp_2", want: "sfp2"},
		{in: "mt7915_phy0", want: "phy0"},
		{in: "marvell_alaska_tomte_phy7", want: "phy7"},
		{in: "cpu_thermal", want: "cpu-thermal"},
		{in: "gpu-thermal", want: "gpu-thermal"},
		{in: "s5_temp", want: "s5-temp"},
		{in: "pwmfan", want: "pwmfan"},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.in, func(t *testing.T) {
			got := normalizeSensorName(tt.in)
			if got != tt.want {
				t.Errorf("normalizeSensorName(%q): expected %q, got %q", tt.in, tt.want, got)
			}
		})
	}
}

func TestHardwareGracefulDegradation(t *testing.T) {
	runner := &testutil.MockRunner{Results: map[string][]byte{}, Errors: map[string]error{}}
	fs := &testutil.MockFileReader{Files: map[string][]byte{}, Globs: map[string][]string{}}

	tr := tree.New()
	c := newHardwareCollector(runner, fs)
	tr.RegisterProvider(hardwareKey, c.Live)
	if err := c.Collect(context.Background(), tr); err != nil {
		t.Fatalf("Collect should not fail when all probes fail: %v", err)
	}

	raw := tr.Get("ietf-hardware:hardware")
	if raw == nil {
		t.Fatal("expected ietf-hardware:hardware key even on probe failures")
	}

	var out map[string]interface{}
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatalf("unmarshal hardware: %v", err)
	}
	components, ok := out["component"].([]interface{})
	if !ok {
		t.Fatalf("component list missing: %v", out["component"])
	}
	if len(components) != 0 {
		t.Fatalf("expected empty component list on total failure, got %d (%v)", len(components), components)
	}
}

func TestHardwareGPSDeviceNotFound(t *testing.T) {
	// Bug 5: When /dev/gps* doesn't exist, readlink -f still succeeds
	// (returns canonical form of non-existent path). Verify the existence
	// check prevents phantom GPS components.
	runner := &testutil.MockRunner{Results: map[string][]byte{}, Errors: map[string]error{}}
	fs := &testutil.MockFileReader{Files: map[string][]byte{
		"/run/system.json": []byte(`{}`),
	}, Globs: map[string][]string{}}

	components := collectHardware(t, newHardwareCollector(runner, fs))
	for _, c := range components {
		m, ok := c.(map[string]interface{})
		if !ok {
			continue
		}
		if m["class"] == "infix-hardware:gps" {
			t.Fatalf("phantom GPS component should not exist when /dev/gps* missing: %v", m)
		}
	}
}

// The synthetic case from test/case/statd/sensors: an SoC die
// temperature from hwmon (s5_temp) and one from a thermal zone
// (cpu-thermal) both land under the cpu component, the fan does not, the
// hwmon mirror of the zone is skipped, and the VPD board named "cpu"
// does not collide with the cpu component.
func TestHardwareSyntheticSensors(t *testing.T) {
	runner := &testutil.MockRunner{Results: map[string][]byte{}, Errors: map[string]error{}}
	fs := &testutil.MockFileReader{Files: map[string][]byte{
		"/run/system.json": []byte(`{
			"vendor": "Microchip",
			"product-name": "EV23X71A",
			"mac-address": "00:a0:85:00:03:00",
			"vpd": {
				"cpu": {
					"board": "cpu",
					"available": true,
					"trusted": true,
					"data": {
						"product-name": "CPU board",
						"serial-number": "0123456789"
					}
				}
			}
		}`),
		"/sys/class/hwmon/hwmon0/name":          []byte("s5_temp\n"),
		"/sys/class/hwmon/hwmon0/temp1_input":   []byte("59500\n"),
		"/sys/class/hwmon/hwmon1/name":          []byte("pwmfan\n"),
		"/sys/class/hwmon/hwmon1/fan1_input":    []byte("3200\n"),
		"/sys/class/hwmon/hwmon2/name":          []byte("cpu_thermal\n"),
		"/sys/class/hwmon/hwmon2/temp1_input":   []byte("48000\n"),
		"/sys/class/thermal/thermal_zone0/type": []byte("cpu-thermal\n"),
		"/sys/class/thermal/thermal_zone0/temp": []byte("48000\n"),
	}, Globs: map[string][]string{
		"/sys/class/hwmon/*":        {"/sys/class/hwmon/hwmon0", "/sys/class/hwmon/hwmon1", "/sys/class/hwmon/hwmon2"},
		"/sys/class/hwmon/hwmon0/*": {"/sys/class/hwmon/hwmon0/name", "/sys/class/hwmon/hwmon0/temp1_input"},
		"/sys/class/hwmon/hwmon1/*": {"/sys/class/hwmon/hwmon1/name", "/sys/class/hwmon/hwmon1/fan1_input"},
		"/sys/class/hwmon/hwmon2/*": {"/sys/class/hwmon/hwmon2/name", "/sys/class/hwmon/hwmon2/temp1_input"},
		"/sys/class/thermal/*":      {"/sys/class/thermal/thermal_zone0", "/sys/class/thermal/cooling_device0"},
	}}

	components := collectHardware(t, newHardwareCollector(runner, fs))

	var names []string
	for _, raw := range components {
		names = append(names, raw.(map[string]interface{})["name"].(string))
	}
	want := []string{"mainboard", "vpd-cpu", "cpu", "cpu-thermal", "s5-temp", "pwmfan"}
	if fmt.Sprint(names) != fmt.Sprint(want) {
		t.Fatalf("component order: want %v, got %v", want, names)
	}

	if vpd := getComponentByName(components, "vpd-cpu"); vpd["class"] != "infix-hardware:vpd" || vpd["model-name"] != "CPU board" {
		t.Fatalf("vpd-cpu mismatch: %v", vpd)
	}
	cpu := getComponentByName(components, "cpu")
	if cpu["class"] != "iana-hardware:cpu" || cpu["parent"] != "mainboard" {
		t.Fatalf("cpu mismatch: %v", cpu)
	}
	if st := cpu["state"].(map[string]interface{}); st["admin-state"] != "unknown" || st["oper-state"] != "enabled" {
		t.Fatalf("cpu state mismatch: %v", st)
	}
	for name, value := range map[string]int{"cpu-thermal": 48000, "s5-temp": 59500} {
		sensor := getComponentByName(components, name)
		sd := sensor["sensor-data"].(map[string]interface{})
		if sensor["parent"] != "cpu" || toInt(sd["value"]) != value || sd["value-type"] != "celsius" {
			t.Fatalf("%s mismatch: %v", name, sensor)
		}
	}
	fan := getComponentByName(components, "pwmfan")
	if _, ok := fan["parent"]; ok {
		t.Fatalf("fan must not be under the cpu: %v", fan)
	}
	if sd := fan["sensor-data"].(map[string]interface{}); toInt(sd["value"]) != 3200 || sd["value-type"] != "rpm" {
		t.Fatalf("fan mismatch: %v", fan)
	}
}

// A hwmon device named after a WiFi radio must not steal the radio's
// component name: its sensors hang off the radio, and the module head a
// multi-sensor device would get is dropped.
func TestHardwareAdoptWifiSensors(t *testing.T) {
	components := []interface{}{
		map[string]interface{}{"name": "radio0", "class": "iana-hardware:module"},
		sensorComponent("radio0", 41000, "celsius", "milli", ""),
		sensorComponent("radio0-VCC", 3300, "volts-DC", "milli", "VCC"),
		sensorComponent("pwmfan", 3200, "rpm", "units", ""),
	}
	components[1].(map[string]interface{})["parent"] = "radio0"
	components[2].(map[string]interface{})["parent"] = "radio0"
	wifiInfo := map[string]map[string]interface{}{"radio0": {}}

	got := adoptWifiSensors(components, wifiInfo)
	if len(got) != 3 {
		t.Fatalf("module head must be dropped, got %v", got)
	}
	temp := getComponentByName(got, "radio0-temp")
	if temp == nil || temp["parent"] != "radio0" || temp["description"] != "Temperature" {
		t.Fatalf("radio temp sensor mismatch: %v", temp)
	}
	if getComponentByName(got, "radio0") != nil {
		t.Fatal("no sensor may keep the radio's name")
	}
	if getComponentByName(got, "pwmfan") == nil {
		t.Fatal("unrelated sensors must pass through")
	}
}

func TestHardwareUniqueNames(t *testing.T) {
	components := []interface{}{
		map[string]interface{}{"name": "cpu"},
		map[string]interface{}{"name": "cpu"},
		map[string]interface{}{"name": "cpu-1"},
		map[string]interface{}{"name": "cpu"},
	}

	c := newHardwareCollector(&testutil.MockRunner{}, &testutil.MockFileReader{})
	var names []string
	for _, raw := range c.uniqueNames(components) {
		names = append(names, raw.(map[string]interface{})["name"].(string))
	}
	want := []string{"cpu", "cpu-1", "cpu-1-1", "cpu-2"}
	if fmt.Sprint(names) != fmt.Sprint(want) {
		t.Fatalf("want %v, got %v", want, names)
	}
	if !c.reported["cpu"] || !c.reported["cpu-1"] || len(c.reported) != 2 {
		t.Fatalf("duplicates remembered, got %v", c.reported)
	}
}

// A band whose name is unknown carries no name leaf rather than "Unknown".
func TestHardwareBandNameOptional(t *testing.T) {
	out := convertPhyInfo(map[string]interface{}{
		"bands": []interface{}{
			map[string]interface{}{"band": 1, "name": "2.4 GHz"},
			map[string]interface{}{"band": 2},
		},
	})
	bands := out["bands"].([]interface{})
	if bands[0].(map[string]interface{})["name"] != "2.4 GHz" {
		t.Fatalf("known band name lost: %v", bands[0])
	}
	if _, ok := bands[1].(map[string]interface{})["name"]; ok {
		t.Fatalf("unknown band must have no name: %v", bands[1])
	}
}

// Sensor readings come from the GET, not the last poll: with Live
// registered as provider, a changed sysfs value shows without Collect.
func TestHardwareLiveReadsSensorsOnGet(t *testing.T) {
	runner := &testutil.MockRunner{Results: map[string][]byte{}, Errors: map[string]error{}}
	fs := &testutil.MockFileReader{Files: map[string][]byte{
		"/run/system.json":                    []byte(`{"vendor": "ACME", "product-name": "Box"}`),
		"/sys/class/hwmon/hwmon0/name":        []byte("s5_temp\n"),
		"/sys/class/hwmon/hwmon0/temp1_input": []byte("40000\n"),
	}, Globs: map[string][]string{
		"/sys/class/hwmon/*":        {"/sys/class/hwmon/hwmon0"},
		"/sys/class/hwmon/hwmon0/*": {"/sys/class/hwmon/hwmon0/name", "/sys/class/hwmon/hwmon0/temp1_input"},
		"/sys/class/thermal/*":      {},
	}}

	tr := tree.New()
	c := newHardwareCollector(runner, fs)
	tr.RegisterProvider("ietf-hardware:hardware", c.Live)
	if err := c.Collect(context.Background(), tr); err != nil {
		t.Fatalf("Collect failed: %v", err)
	}

	reading := func() interface{} {
		var out map[string]interface{}
		if err := json.Unmarshal(tr.Get("ietf-hardware:hardware"), &out); err != nil {
			t.Fatalf("unmarshal hardware: %v", err)
		}
		sensor := getComponentByName(out["component"].([]interface{}), "s5-temp")
		if sensor == nil {
			t.Fatalf("s5-temp missing: %v", out)
		}
		return sensor["sensor-data"].(map[string]interface{})["value"]
	}

	if v := reading(); v != float64(40000) {
		t.Fatalf("initial reading = %v, want 40000", v)
	}

	fs.Files["/sys/class/hwmon/hwmon0/temp1_input"] = []byte("61000\n")
	if v := reading(); v != float64(61000) {
		t.Fatalf("reading after sysfs change = %v, want 61000 without a new poll", v)
	}

	mainboard := func() map[string]interface{} {
		var out map[string]interface{}
		json.Unmarshal(tr.Get("ietf-hardware:hardware"), &out)
		return getComponentByName(out["component"].([]interface{}), "mainboard")
	}
	if mainboard() == nil {
		t.Fatal("polled inventory must be kept in the live view")
	}
}

// Only die sensors, and the "-thermal" zones the DT names them, belong
// to the CPU; a fan or a rail that merely starts with cpu/soc does not.
func TestSocTempSource(t *testing.T) {
	yes := []string{"cpu", "cpu0", "soc", "core1", "coretemp", "k10temp", "s5-temp", "ap", "cp0", "cpu-thermal", "soc-thermal-1"}
	no := []string{"cpu-fan", "soc-vdd", "ap-power", "cpufreq", "pwmfan", "sfp2"}
	for _, name := range yes {
		if !socTempSourceRe.MatchString(name) {
			t.Errorf("%q must be a die sensor", name)
		}
	}
	for _, name := range no {
		if socTempSourceRe.MatchString(name) {
			t.Errorf("%q must not be a die sensor", name)
		}
	}
}

// The poll no longer builds the inventory: a GET reads system.json when
// asked, so a change shows without another poll.
func TestHardwareInventoryReadOnGet(t *testing.T) {
	runner := &testutil.MockRunner{Results: map[string][]byte{}, Errors: map[string]error{}}
	fs := &testutil.MockFileReader{Files: map[string][]byte{
		"/run/system.json": []byte(`{"vendor":"Acme","product-name":"Router-1"}`),
	}, Globs: map[string][]string{}}

	tr := tree.New()
	c := newHardwareCollector(runner, fs)
	tr.RegisterProvider(hardwareKey, c.Live)
	if err := c.Collect(context.Background(), tr); err != nil {
		t.Fatalf("Collect failed: %v", err)
	}
	if cached := string(tr.GetCached(hardwareKey)); cached != "{}" {
		t.Fatalf("poll stored %s, want the {} placeholder", cached)
	}

	model := func() interface{} {
		var out map[string]interface{}
		if err := json.Unmarshal(tr.Get(hardwareKey), &out); err != nil {
			t.Fatalf("unmarshal hardware: %v", err)
		}
		mb := getComponentByName(out["component"].([]interface{}), "mainboard")
		if mb == nil {
			t.Fatal("mainboard missing")
		}
		return mb["model-name"]
	}

	if got := model(); got != "Router-1" {
		t.Fatalf("model-name = %v, want Router-1", got)
	}
	fs.Files["/run/system.json"] = []byte(`{"vendor":"Acme","product-name":"Router-2"}`)
	if got := model(); got != "Router-2" {
		t.Fatalf("model-name = %v, want Router-2 without a new poll", got)
	}
}

// Without WiFi there are no radios to keep: RunRadios only waits for
// shutdown.
func TestHardwareRunRadiosIdleWithoutWifi(t *testing.T) {
	runner := &testutil.MockRunner{Results: map[string][]byte{}, Errors: map[string]error{}}
	fs := &testutil.MockFileReader{Files: map[string][]byte{}, Globs: map[string][]string{}}
	c := newHardwareCollector(runner, fs)

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- c.RunRadios(ctx) }()

	c.RequestRadioRefresh()
	c.RequestRadioRefresh()
	select {
	case err := <-done:
		t.Fatalf("RunRadios returned early: %v", err)
	case <-time.After(50 * time.Millisecond):
	}

	cancel()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("RunRadios did not stop on cancel")
	}
}

// The QEMU stick from test/case/statd/system: a hub on usb1 with the
// stick behind it, both parented to the port, the stick not authorized
// because the port is locked.  Expected output as the Python yanger
// produced it.
func TestHardwareUSBDevices(t *testing.T) {
	runner := &testutil.MockRunner{Results: map[string][]byte{}, Errors: map[string]error{}}
	fs := &testutil.MockFileReader{Files: map[string][]byte{
		"/run/system.json": []byte(`{"usb-ports":[` +
			`{"name":"usb1","paths":["/sys/bus/usb/devices/usb1"]},` +
			`{"name":"usb2","paths":["/sys/bus/usb/devices/usb2"]}]}`),
		"/sys/bus/usb/devices/usb1/authorized_default":    []byte("2\n"),
		"/sys/bus/usb/devices/usb2/authorized_default":    []byte("2\n"),
		"/sys/bus/usb/devices/1-0:1.0/bInterfaceClass":    []byte("09\n"),
		"/sys/bus/usb/devices/1-0:1.0/bInterfaceNumber":   []byte("00\n"),
		"/sys/bus/usb/devices/1-1/authorized":             []byte("1\n"),
		"/sys/bus/usb/devices/1-1/bcdDevice":              []byte("0663\n"),
		"/sys/bus/usb/devices/1-1/idProduct":              []byte("0610\n"),
		"/sys/bus/usb/devices/1-1/idVendor":               []byte("05e3\n"),
		"/sys/bus/usb/devices/1-1/manufacturer":           []byte("GenesysLogic\n"),
		"/sys/bus/usb/devices/1-1/product":                []byte("USB2.1 Hub\n"),
		"/sys/bus/usb/devices/1-1/speed":                  []byte("480\n"),
		"/sys/bus/usb/devices/1-1/version":                []byte(" 2.00\n"),
		"/sys/bus/usb/devices/1-1:1.0/bInterfaceClass":    []byte("09\n"),
		"/sys/bus/usb/devices/1-1:1.0/bInterfaceNumber":   []byte("00\n"),
		"/sys/bus/usb/devices/1-1.2/authorized":           []byte("0\n"),
		"/sys/bus/usb/devices/1-1.2/bcdDevice":            []byte("0100\n"),
		"/sys/bus/usb/devices/1-1.2/idProduct":            []byte("5581\n"),
		"/sys/bus/usb/devices/1-1.2/idVendor":             []byte("0781\n"),
		"/sys/bus/usb/devices/1-1.2/manufacturer":         []byte("SanDisk\n"),
		"/sys/bus/usb/devices/1-1.2/product":              []byte("Ultra\n"),
		"/sys/bus/usb/devices/1-1.2/serial":               []byte("4C530001230920115031\n"),
		"/sys/bus/usb/devices/1-1.2/speed":                []byte("480\n"),
		"/sys/bus/usb/devices/1-1.2/version":              []byte(" 3.20\n"),
		"/sys/bus/usb/devices/1-1.2:1.0/bInterfaceClass":  []byte("08\n"),
		"/sys/bus/usb/devices/1-1.2:1.0/bInterfaceNumber": []byte("00\n"),
	}, Globs: map[string][]string{
		"/sys/bus/usb/devices/*": {
			"/sys/bus/usb/devices/1-0:1.0", "/sys/bus/usb/devices/1-1.2:1.0",
			"/sys/bus/usb/devices/1-1.2", "/sys/bus/usb/devices/1-1",
			"/sys/bus/usb/devices/1-1:1.0", "/sys/bus/usb/devices/usb1",
			"/sys/bus/usb/devices/usb2",
		},
	}}

	components := collectHardware(t, newHardwareCollector(runner, fs))
	got := make([]interface{}, 0)
	for _, raw := range components {
		if class, _ := raw.(map[string]interface{})["class"].(string); strings.HasPrefix(class, "infix-hardware:usb") {
			got = append(got, raw)
		}
	}

	var want []interface{}
	if err := json.Unmarshal([]byte(`[
{"name": "usb1", "class": "infix-hardware:usb", "state": {"admin-state": "locked", "oper-state": "enabled", "usage-state": "active"}},
{"name": "usb-1-1", "class": "infix-hardware:usb-device", "parent": "usb1", "uri": ["usb:///sys/bus/usb/devices/1-1"], "mfg-name": "GenesysLogic", "model-name": "USB2.1 Hub", "hardware-rev": "6.63", "infix-hardware:usb-device": {"vendor-id": "05e3", "product-id": "0610", "usb-version": "2.00", "speed": "480", "authorized": true, "interface": [{"number": 0, "class": "hub"}]}},
{"name": "usb-1-1.2", "class": "infix-hardware:usb-device", "parent": "usb1", "uri": ["usb:///sys/bus/usb/devices/1-1.2"], "mfg-name": "SanDisk", "model-name": "Ultra", "serial-num": "4C530001230920115031", "hardware-rev": "1.00", "infix-hardware:usb-device": {"vendor-id": "0781", "product-id": "5581", "usb-version": "3.20", "speed": "480", "authorized": false, "interface": [{"number": 0, "class": "mass-storage"}]}},
{"name": "usb2", "class": "infix-hardware:usb", "state": {"admin-state": "locked", "oper-state": "enabled", "usage-state": "idle"}}
]`), &want); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, want) {
		gotJSON, _ := json.MarshalIndent(got, "", "  ")
		wantJSON, _ := json.MarshalIndent(want, "", "  ")
		t.Fatalf("usb components\n got: %s\nwant: %s", gotJSON, wantJSON)
	}
}

// A radio and a GPS receiver on USB point at their device.  The
// receiver's tty is a usb-serial port, two levels below the device,
// and gets a uri to anchor it by; a radio on the SoC stays as is.
func TestHardwareUSBDeviceParents(t *testing.T) {
	runner := &testutil.MockRunner{Results: map[string][]byte{
		"readlink -f /dev/gps0": []byte("/dev/ttyUSB0\n"),
	}, Errors: map[string]error{}}
	fs := &testutil.MockFileReader{Files: map[string][]byte{
		"/run/system.json": []byte(`{"usb-ports":[{"name":"usb","paths":["/sys/bus/usb/devices/usb1"]}]}`),
		"/sys/bus/usb/devices/usb1/authorized_default": []byte("1\n"),
		"/sys/bus/usb/devices/1-1/authorized":          []byte("1\n"),
		"/sys/bus/usb/devices/1-2/authorized":          []byte("1\n"),
		"/sys/class/tty/ttyUSB0/device/../../idVendor": []byte("1a86\n"),
		"/sys/class/tty/ttyUSB0/device/../../busnum":   []byte("1\n"),
		"/sys/class/tty/ttyUSB0/device/../../devpath":  []byte("2\n"),
	}, Globs: map[string][]string{
		"/dev/gps[0-3]": {"/dev/gps0"},
		"/sys/bus/usb/devices/*": {
			"/sys/bus/usb/devices/1-1", "/sys/bus/usb/devices/1-1:1.0",
			"/sys/bus/usb/devices/1-2", "/sys/bus/usb/devices/1-2:1.0",
			"/sys/bus/usb/devices/usb1",
		},
		"/sys/bus/usb/devices/*:*/ieee80211/*": {"/sys/bus/usb/devices/1-1:1.0/ieee80211/phy1"},
	}}

	c := NewHardwareCollector(runner, fs, 30*time.Second, false, true)
	c.mu.Lock()
	c.radios = []interface{}{
		map[string]interface{}{"name": "phy0", "class": "infix-hardware:wifi"},
		map[string]interface{}{"name": "phy1", "class": "infix-hardware:wifi"},
	}
	c.mu.Unlock()

	components := collectHardware(t, c)
	if phy0 := getComponentByName(components, "phy0"); phy0["parent"] != nil {
		t.Fatalf("phy0 is not on USB, got parent %v", phy0["parent"])
	}
	if phy1 := getComponentByName(components, "phy1"); phy1["parent"] != "usb-1-1" {
		t.Fatalf("phy1 parent = %v, want usb-1-1", phy1["parent"])
	}
	gps := getComponentByName(components, "gps0")
	if gps == nil {
		t.Fatal("gps0 missing")
	}
	if gps["parent"] != "usb-1-2" {
		t.Fatalf("gps0 parent = %v, want usb-1-2", gps["parent"])
	}
	if !reflect.DeepEqual(gps["uri"], []interface{}{"usb:///sys/bus/usb/devices/1-2"}) {
		t.Fatalf("gps0 uri = %v", gps["uri"])
	}
}

// gpsd keeps serving the last fix it parsed for as long as the device
// is open.  A fix older than the POLL reply is reported as lost.
func TestHardwareGPSStaleFix(t *testing.T) {
	poll := func(pollTime, fixTime string) map[string]interface{} {
		var reply map[string]interface{}
		raw := `{"class":"POLL","time":"` + pollTime + `","active":1,
			"tpv":[{"class":"TPV","device":"/dev/ttyUSB0","driver":"NMEA0183","mode":3,
				"time":"` + fixTime + `","lat":27.9881,"lon":86.925,"altHAE":8895.9}],
			"sky":[{"class":"SKY","device":"/dev/ttyUSB0","nSat":8,"uSat":8}]}`
		if err := json.Unmarshal([]byte(raw), &reply); err != nil {
			t.Fatal(err)
		}
		return reply
	}

	for _, tc := range []struct {
		name     string
		fixTime  string
		fixMode  string
		position bool
		used     float64
	}{
		{"fresh", "2026-10-21T17:26:27.000Z", "3d", true, 8},
		{"stale", "2026-10-21T17:00:00.000Z", "none", false, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			runner := &testutil.MockRunner{Results: map[string][]byte{
				"readlink -f /dev/gps0": []byte("/dev/ttyUSB0\n"),
			}, Errors: map[string]error{}}
			fs := &testutil.MockFileReader{Files: map[string][]byte{}, Globs: map[string][]string{
				"/dev/gps[0-3]": {"/dev/gps0"},
			}}

			c := NewHardwareCollector(runner, fs, 30*time.Second, false, true)
			c.gpsPoll = func(context.Context) map[string]interface{} {
				return poll("2026-10-21T17:26:30.000Z", tc.fixTime)
			}

			gps := getComponentByName(collectHardware(t, c), "gps0")
			if gps == nil {
				t.Fatal("gps0 missing")
			}
			rx := gps["infix-hardware:gps-receiver"].(map[string]interface{})
			if rx["activated"] != true {
				t.Fatalf("activated = %v, want true", rx["activated"])
			}
			if rx["fix-mode"] != tc.fixMode {
				t.Fatalf("fix-mode = %v, want %s", rx["fix-mode"], tc.fixMode)
			}
			if _, ok := rx["latitude"]; ok != tc.position {
				t.Fatalf("latitude present = %v, want %v", ok, tc.position)
			}
			if rx["satellites-used"] != tc.used {
				t.Fatalf("satellites-used = %v, want %v", rx["satellites-used"], tc.used)
			}
		})
	}
}
