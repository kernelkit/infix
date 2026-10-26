package mmmonitor

import (
	"reflect"
	"testing"

	"github.com/godbus/dbus/v5"
)

// Shaped like a Quectel EM05 connected on LTE, as GetManagedObjects and
// GetAll return it.
func TestModems(t *testing.T) {
	objs := map[dbus.ObjectPath]map[string]props{
		"/org/freedesktop/ModemManager1/Modem/0": {
			ifModem: {
				"Device":              dbus.MakeVariant("/sys/devices/platform/soc/11200000.usb/usb1/1-1/1-1.1"),
				"Manufacturer":        dbus.MakeVariant("Quectel Incorporated"),
				"Model":               dbus.MakeVariant("LTE Module"),
				"Revision":            dbus.MakeVariant("EM05EFAR06A06M4G"),
				"EquipmentIdentifier": dbus.MakeVariant("867752050900000"),
				"OwnNumbers":          dbus.MakeVariant([]string{}),
				"State":               dbus.MakeVariant(int32(11)),
				"StateFailedReason":   dbus.MakeVariant(uint32(0)),
				"PowerState":          dbus.MakeVariant(uint32(3)),
				"AccessTechnologies":  dbus.MakeVariant(uint32(1 << 14)),
				"SignalQuality":       dbus.MakeVariant([]interface{}{uint32(19), true}),
				"UnlockRequired":      dbus.MakeVariant(uint32(1)),
				"Sim":                 dbus.MakeVariant(dbus.ObjectPath("/org/freedesktop/ModemManager1/SIM/0")),
				"Bearers":             dbus.MakeVariant([]dbus.ObjectPath{"/org/freedesktop/ModemManager1/Bearer/0"}),
			},
			if3gpp: {
				"RegistrationState":    dbus.MakeVariant(uint32(1)),
				"OperatorName":         dbus.MakeVariant("Fello"),
				"OperatorCode":         dbus.MakeVariant("24001"),
				"EnabledFacilityLocks": dbus.MakeVariant(uint32(2)),
				"PacketServiceState":   dbus.MakeVariant(uint32(2)),
			},
			ifSignal: {
				"Nr5g": dbus.MakeVariant(map[string]dbus.Variant{}),
				"Lte": dbus.MakeVariant(map[string]dbus.Variant{
					"rssi": dbus.MakeVariant(-69.0),
					"rsrp": dbus.MakeVariant(-106.0),
					"rsrq": dbus.MakeVariant(-12.0),
					"snr":  dbus.MakeVariant(14.0),
				}),
			},
		},
		// Not a modem, e.g. an SMS object
		"/org/freedesktop/ModemManager1/SMS/1": {service + ".Sms": {}},
	}

	refs := map[string]props{
		"/org/freedesktop/ModemManager1/Bearer/0": {
			"Interface":       dbus.MakeVariant("wwan0"),
			"Connected":       dbus.MakeVariant(true),
			"ConnectionError": dbus.MakeVariant([]interface{}{"", ""}),
			"Ip4Config": dbus.MakeVariant(map[string]dbus.Variant{
				"address": dbus.MakeVariant("10.35.120.97"),
				"prefix":  dbus.MakeVariant(uint32(30)),
			}),
			"Ip6Config": dbus.MakeVariant(map[string]dbus.Variant{"method": dbus.MakeVariant(uint32(0))}),
			"Stats": dbus.MakeVariant(map[string]dbus.Variant{
				"rx-bytes":       dbus.MakeVariant(uint64(1234)),
				"total-duration": dbus.MakeVariant(uint32(510)),
			}),
		},
		"/org/freedesktop/ModemManager1/SIM/0": {
			"Imsi":          dbus.MakeVariant("240017540000000"),
			"SimIdentifier": dbus.MakeVariant("89460100170000000000"),
			"OperatorName":  dbus.MakeVariant("Fello"),
		},
	}

	got := modems(objs, refs)
	if len(got) != 1 {
		t.Fatalf("got %d modems, want 1", len(got))
	}
	m := got[0]

	if m.Device != "/sys/devices/platform/soc/11200000.usb/usb1/1-1/1-1.1" {
		t.Errorf("Device = %q", m.Device)
	}
	if m.State != 11 || m.PowerState != 3 || m.AccessTech != 1<<14 {
		t.Errorf("State/PowerState/AccessTech = %d/%d/%d", m.State, m.PowerState, m.AccessTech)
	}
	if m.SignalQuality != 19 || !m.SignalRecent {
		t.Errorf("SignalQuality = %d recent %v, want 19 true", m.SignalQuality, m.SignalRecent)
	}
	if m.OperatorName != "Fello" || m.Registration != 1 || m.FacilityLocks != 2 || m.PacketService != 2 {
		t.Errorf("3GPP = %+v", m)
	}
	wantSignal := map[string]float64{"rssi": -69, "rsrp": -106, "rsrq": -12, "snr": 14}
	if !reflect.DeepEqual(m.Signal, wantSignal) {
		t.Errorf("Signal = %v, want %v", m.Signal, wantSignal)
	}
	if m.Sim == nil || m.Sim.Iccid != "89460100170000000000" || m.Sim.OperatorName != "Fello" {
		t.Errorf("Sim = %+v", m.Sim)
	}

	if len(m.Bearers) != 1 {
		t.Fatalf("got %d bearers, want 1", len(m.Bearers))
	}
	b := m.Bearers[0]
	if b.Interface != "wwan0" || !b.Connected || b.ConnError != "" {
		t.Errorf("bearer = %+v", b)
	}
	if b.IPv4 != "10.35.120.97" || b.IPv4Prefix != 30 || b.IPv6 != "" {
		t.Errorf("bearer addresses = %+v", b)
	}
	if b.Stats["rx-bytes"] != 1234 || b.Stats["total-duration"] != 510 {
		t.Errorf("bearer stats = %v", b.Stats)
	}
}

// No SIM is an object path of "/", and a failed property read leaves
// the SIM out rather than the modem.
func TestModemsNoSim(t *testing.T) {
	objs := map[dbus.ObjectPath]map[string]props{
		"/org/freedesktop/ModemManager1/Modem/3": {
			ifModem: {
				"Device": dbus.MakeVariant("/sys/devices/usb2/2-1"),
				"Sim":    dbus.MakeVariant(dbus.ObjectPath("/")),
				"State":  dbus.MakeVariant(int32(-1)),
			},
		},
	}
	got := modems(objs, nil)
	if len(got) != 1 {
		t.Fatalf("got %d modems, want 1", len(got))
	}
	if got[0].SimPath != "" || got[0].Sim != nil || got[0].State != -1 {
		t.Errorf("modem = %+v", got[0])
	}
	if got[0].Signal != nil || len(got[0].Bearers) != 0 {
		t.Errorf("signal %v bearers %v, want none", got[0].Signal, got[0].Bearers)
	}
}
