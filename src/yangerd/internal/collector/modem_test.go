package collector

import (
	"reflect"
	"testing"

	"github.com/kernelkit/infix/src/yangerd/internal/mmmonitor"
	"github.com/kernelkit/infix/src/yangerd/internal/testutil"
)

const probedEM05 = `{"modem":[{"index":0,"name":"modem0",` +
	`"devpath":"/sys/devices/platform/soc/11200000.usb/usb1/1-1/1-1.1",` +
	`"vid":"2c7c","pid":"0125","uri":"usb:///sys/bus/usb/devices/1-1.1"}]}`

func connectedEM05() mmmonitor.Modem {
	return mmmonitor.Modem{
		Path:                "/org/freedesktop/ModemManager1/Modem/0",
		Device:              "/sys/devices/platform/soc/11200000.usb/usb1/1-1/1-1.1",
		Manufacturer:        "Quectel Incorporated",
		Model:               "LTE Module",
		Revision:            "EM05EFAR06A06M4G",
		EquipmentIdentifier: "867752050900000",
		State:               11,
		PowerState:          3,
		AccessTech:          1<<14 | 1<<9,
		SignalQuality:       35,
		SignalRecent:        true,
		UnlockRequired:      1,
		SimPath:             "/org/freedesktop/ModemManager1/SIM/0",
		Registration:        1,
		OperatorName:        "Fello",
		OperatorCode:        "24001",
		FacilityLocks:       1 << 1,
		PacketService:       2,
		Signal:              map[string]float64{"rssi": -69, "rsrp": -98.4, "rsrq": -9},
		Sim: &mmmonitor.Sim{
			Imsi:         "240017540000000",
			Iccid:        "89460100170000000000",
			OperatorName: "Fello",
		},
		Bearers: []mmmonitor.Bearer{{
			Path:       "/org/freedesktop/ModemManager1/Bearer/0",
			Interface:  "wwan0",
			Connected:  true,
			IPv4:       "10.35.120.97",
			IPv4Prefix: 30,
			Stats:      map[string]uint64{"rx-bytes": 1234, "total-duration": 510},
		}},
	}
}

// probedFS has the probe's view of the EM05 and nothing from modemd.
func probedFS() *testutil.MockFileReader {
	return &testutil.MockFileReader{Files: map[string][]byte{
		"/run/system.json": []byte(probedEM05),
	}, Globs: map[string][]string{}}
}

func modemCollector(fs *testutil.MockFileReader, modems ...mmmonitor.Modem) *HardwareCollector {
	runner := &testutil.MockRunner{Results: map[string][]byte{}, Errors: map[string]error{}}
	c := newHardwareCollector(runner, fs)
	c.SetModemSource(func() []mmmonitor.Modem { return modems })
	return c
}

func TestHardwareModem(t *testing.T) {
	fs := &testutil.MockFileReader{Files: map[string][]byte{
		"/run/system.json":          []byte(probedEM05),
		"/run/modemd/modem0/device": []byte("/sys/devices/platform/soc/11200000.usb/usb1/1-1/1-1.1\n"),
		"/run/modemd/modem0/state.json": []byte(`{"last-change":"2026-10-25T07:34:40+00:00",` +
			`"state-last-changed":"2026-10-25T07:24:21+00:00"}`),
	}, Globs: map[string][]string{
		"/run/modemd/modem*/device": {"/run/modemd/modem0/device"},
	}}
	components := collectHardware(t, modemCollector(fs, connectedEM05()))

	modem := getComponentByName(components, "modem0")
	if modem == nil {
		t.Fatal("modem0 missing")
	}
	if modem["class"] != "infix-hardware:modem" {
		t.Errorf("class = %v", modem["class"])
	}
	if !reflect.DeepEqual(modem["uri"], []interface{}{"usb:///sys/bus/usb/devices/1-1.1"}) {
		t.Errorf("uri = %v", modem["uri"])
	}
	state := modem["state"].(map[string]interface{})
	if state["oper-state"] != "enabled" || state["state-last-changed"] != "2026-10-25T07:24:21+00:00" {
		t.Errorf("state = %v", state)
	}
	if _, ok := state["alarm-state"]; ok {
		t.Errorf("alarm-state = %v, want none", state["alarm-state"])
	}

	ms := modem["infix-hardware:modem-state"].(map[string]interface{})
	want := map[string]interface{}{
		"manufacturer":          "Quectel Incorporated",
		"firmware-version":      "EM05EFAR06A06M4G",
		"serial-number":         "867752050900000",
		"imsi":                  "240017540000000",
		"usb-vendor-id":         "2c7c",
		"usb-product-id":        "0125",
		"selected-carrier":      "default",
		"state":                 "connected",
		"power-state":           "on",
		"enabled-locks":         []interface{}{"fixed-dialing"},
		"signal-quality":        float64(35),
		"signal-quality-recent": true,
		"signal-rssi":           float64(-69),
		"signal-rsrp":           float64(-98),
		"signal-rsrq":           "-9.0",
		"last-change":           "2026-10-25T07:34:40+00:00",
	}
	for key, val := range want {
		if !reflect.DeepEqual(ms[key], val) {
			t.Errorf("modem-state %s = %#v, want %#v", key, ms[key], val)
		}
	}
	if _, ok := ms["state-failed-reason"]; ok {
		t.Error("state-failed-reason reported for a working modem")
	}
	cell := ms["cellular"].(map[string]interface{})
	wantCell := map[string]interface{}{
		"registration-state": "home",
		"service-state":      "attached",
		"operator-name":      "Fello",
		"operator-id":        "24001",
		"network-type":       "lte",
	}
	if !reflect.DeepEqual(cell, wantCell) {
		t.Errorf("cellular = %v, want %v", cell, wantCell)
	}

	sim := getComponentByName(components, "sim0")
	if sim == nil {
		t.Fatal("sim0 missing")
	}
	ss := sim["infix-hardware:sim-state"].(map[string]interface{})
	wantSim := map[string]interface{}{"slot": float64(1), "state": "unlocked", "operator-name": "Fello"}
	if !reflect.DeepEqual(ss, wantSim) {
		t.Errorf("sim-state = %v, want %v", ss, wantSim)
	}
	if sim["state"].(map[string]interface{})["oper-state"] != "enabled" {
		t.Errorf("sim state = %v", sim["state"])
	}
}

// A modem has the name of the component modemd manages it with,
// whatever the probe called it at boot.
func TestHardwareModemManagedName(t *testing.T) {
	fs := &testutil.MockFileReader{Files: map[string][]byte{
		"/run/system.json":          []byte(probedEM05),
		"/run/modemd/modem2/device": []byte("/sys/devices/platform/soc/11200000.usb/usb1/1-1/1-1.1"),
	}, Globs: map[string][]string{
		"/run/modemd/modem*/device": {"/run/modemd/modem2/device"},
	}}
	components := collectHardware(t, modemCollector(fs, connectedEM05()))

	modem := getComponentByName(components, "modem2")
	if modem == nil {
		t.Fatal("modem2 missing")
	}
	if !reflect.DeepEqual(modem["uri"], []interface{}{"usb:///sys/bus/usb/devices/1-1.1"}) {
		t.Errorf("uri = %v", modem["uri"])
	}
	if getComponentByName(components, "modem0") != nil {
		t.Error("probed modem0 reported next to the pinned modem2")
	}
	if getComponentByName(components, "sim2") == nil {
		t.Error("sim2 missing")
	}
}

// A modem ModemManager does not manage is still listed with its uri,
// it is what the operator needs to configure it.
func TestHardwareModemUnmanaged(t *testing.T) {
	fs := probedFS()
	components := collectHardware(t, modemCollector(fs))

	modem := getComponentByName(components, "modem0")
	if modem == nil {
		t.Fatal("modem0 missing")
	}
	if modem["state"].(map[string]interface{})["oper-state"] != "unknown" {
		t.Errorf("state = %v", modem["state"])
	}
	ms := modem["infix-hardware:modem-state"].(map[string]interface{})
	if !reflect.DeepEqual(ms, map[string]interface{}{"usb-vendor-id": "2c7c", "usb-product-id": "0125"}) {
		t.Errorf("modem-state = %v", ms)
	}
	if getComponentByName(components, "sim0") != nil {
		t.Error("sim0 reported for an unmanaged modem")
	}
}

func TestHardwareModemSimMissing(t *testing.T) {
	m := connectedEM05()
	m.State = -1
	m.StateFailedReason = 2
	m.SimPath = ""
	m.Sim = nil
	m.Bearers = nil

	fs := probedFS()
	components := collectHardware(t, modemCollector(fs, m))

	modem := getComponentByName(components, "modem0")
	state := modem["state"].(map[string]interface{})
	if state["oper-state"] != "disabled" || state["alarm-state"] != "major" {
		t.Errorf("state = %v", state)
	}
	ms := modem["infix-hardware:modem-state"].(map[string]interface{})
	if ms["state"] != "failed" || ms["state-failed-reason"] != "sim-missing" {
		t.Errorf("state %v reason %v", ms["state"], ms["state-failed-reason"])
	}
	if _, ok := ms["imsi"]; ok {
		t.Error("imsi without a SIM")
	}

	sim := getComponentByName(components, "sim0")
	if sim["infix-hardware:sim-state"].(map[string]interface{})["state"] != "not-inserted" {
		t.Errorf("sim-state = %v", sim["infix-hardware:sim-state"])
	}
	if sim["state"].(map[string]interface{})["oper-state"] != "disabled" {
		t.Errorf("sim state = %v", sim["state"])
	}
}

func TestHardwareModemWeakSignal(t *testing.T) {
	m := connectedEM05()
	m.SignalQuality = 12
	fs := probedFS()
	components := collectHardware(t, modemCollector(fs, m))

	state := getComponentByName(components, "modem0")["state"].(map[string]interface{})
	if state["alarm-state"] != "warning" {
		t.Errorf("alarm-state = %v, want warning", state["alarm-state"])
	}
}

// A connected bearer names its interface, the first bearer of a modem
// that is not connected belongs to wwanN of modemN.
func TestWwanStates(t *testing.T) {
	up := connectedEM05()
	down := connectedEM05()
	down.Device = "/sys/devices/platform/soc/11200000.usb/usb1/1-1/1-1.2"
	down.Bearers = []mmmonitor.Bearer{{
		Path:      "/org/freedesktop/ModemManager1/Bearer/1",
		ConnError: "Serving network not allowed",
	}}

	fs := &testutil.MockFileReader{Files: map[string][]byte{
		"/run/system.json":          []byte(probedEM05),
		"/run/modemd/modem0/device": []byte("/sys/devices/platform/soc/11200000.usb/usb1/1-1/1-1.1"),
		"/run/modemd/modem1/device": []byte("/sys/devices/platform/soc/11200000.usb/usb1/1-1/1-1.2"),
	}, Globs: map[string][]string{
		"/run/modemd/modem*/device": {"/run/modemd/modem0/device", "/run/modemd/modem1/device"},
	}}
	got := modemCollector(fs, up, down).WwanStates()

	want := map[string]interface{}{
		"wwan0": map[string]interface{}{"bearer-state": map[string]interface{}{
			"path":               "/org/freedesktop/ModemManager1/Bearer/0",
			"connected":          true,
			"interface":          "wwan0",
			"ipv4-address":       "10.35.120.97",
			"ipv4-prefix-length": uint32(30),
			"in-bytes":           "1234",
			"total-duration":     "510",
		}},
		"wwan1": map[string]interface{}{"bearer-state": map[string]interface{}{
			"path":                     "/org/freedesktop/ModemManager1/Bearer/1",
			"connected":                false,
			"connection-failed-reason": "Serving network not allowed",
		}},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("WwanStates() =\n%#v\nwant\n%#v", got, want)
	}
}

func TestAccessTechName(t *testing.T) {
	for bits, want := range map[uint32]string{
		0:             "unknown",
		1 << 1:        "gsm",
		1<<14 | 1<<9:  "lte",
		1<<15 | 1<<14: "5gnr",
		1 << 17:       "lte",
		1 << 20:       "unknown",
	} {
		if got := accessTechName(bits); got != want {
			t.Errorf("accessTechName(%#x) = %q, want %q", bits, got, want)
		}
	}
}

// An EP06-E on an IoT SIM: roaming, and a SIM with PIN2 set, which
// only guards fixed dialing and leaves the SIM unlocked.
func TestHardwareModemRoamingPin2(t *testing.T) {
	m := connectedEM05()
	m.Device = "/sys/devices/platform/fd880000.usb/usb2/2-1"
	m.Registration = 5
	m.UnlockRequired = 3

	fs := &testutil.MockFileReader{Files: map[string][]byte{
		"/run/system.json": []byte(`{"modem":[{"index":0,"name":"modem0",` +
			`"devpath":"/sys/devices/platform/fd880000.usb/usb2/2-1",` +
			`"vid":"2c7c","pid":"0306","uri":"usb:///sys/bus/usb/devices/2-1"}]}`),
	}, Globs: map[string][]string{}}
	components := collectHardware(t, modemCollector(fs, m))

	ms := getComponentByName(components, "modem0")["infix-hardware:modem-state"].(map[string]interface{})
	if reg := ms["cellular"].(map[string]interface{})["registration-state"]; reg != "roaming" {
		t.Errorf("registration-state = %v, want roaming", reg)
	}
	ss := getComponentByName(components, "sim0")["infix-hardware:sim-state"].(map[string]interface{})
	if ss["state"] != "unlocked" {
		t.Errorf("sim state = %v, want unlocked", ss["state"])
	}
}
