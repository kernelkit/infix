package mmmonitor

import (
	"github.com/godbus/dbus/v5"

	"github.com/kernelkit/infix/src/yangerd/internal/numconv"
)

const (
	service  = "org.freedesktop.ModemManager1"
	root     = "/org/freedesktop/ModemManager1"
	ifModem  = service + ".Modem"
	if3gpp   = service + ".Modem.Modem3gpp"
	ifSignal = service + ".Modem.Signal"
	ifBearer = service + ".Bearer"
	ifSim    = service + ".Sim"
)

// Modem is one modem as ModemManager reports it.  Enumerations are
// ModemManager's own values, see ModemManager-enums.h.
type Modem struct {
	Path   string
	Device string // sysfs path of the USB device

	Manufacturer        string
	Model               string
	Revision            string
	HardwareRevision    string
	EquipmentIdentifier string
	OwnNumbers          []string

	State             int32
	StateFailedReason uint32
	PowerState        uint32
	AccessTech        uint32 // MMModemAccessTechnology bits
	SignalQuality     uint32
	SignalRecent      bool
	UnlockRequired    uint32
	PrimarySimSlot    uint32
	SimPath           string

	Registration  uint32
	OperatorName  string
	OperatorCode  string
	FacilityLocks uint32 // MMModem3gppFacility bits
	PacketService uint32

	// Extended signal of the most advanced technology that has any,
	// keys as ModemManager names them: rssi, rsrp, rsrq, snr.
	Signal map[string]float64

	Bearers []Bearer
	Sim     *Sim
}

// Bearer is one data connection of a modem.
type Bearer struct {
	Path       string
	Interface  string
	Connected  bool
	ConnError  string
	IPv4       string
	IPv4Prefix uint32
	IPv6       string
	IPv6Prefix uint32
	Stats      map[string]uint64 // rx-bytes, total-duration, ...
}

// Sim is the active SIM card of a modem.
type Sim struct {
	Imsi         string
	Iccid        string
	OperatorName string
}

type props = map[string]dbus.Variant

// modems builds the modem list from the modem objects and the
// properties of their bearers and SIMs, by object path.
func modems(objs map[dbus.ObjectPath]map[string]props, refs map[string]props) []Modem {
	out := make([]Modem, 0, len(objs))
	for path, ifaces := range objs {
		gen, ok := ifaces[ifModem]
		if !ok {
			continue
		}
		gpp := ifaces[if3gpp]

		m := Modem{
			Path:                string(path),
			Device:              str(gen["Device"]),
			Manufacturer:        str(gen["Manufacturer"]),
			Model:               str(gen["Model"]),
			Revision:            str(gen["Revision"]),
			HardwareRevision:    str(gen["HardwareRevision"]),
			EquipmentIdentifier: str(gen["EquipmentIdentifier"]),
			OwnNumbers:          strs(gen["OwnNumbers"]),
			State:               i32(gen["State"]),
			StateFailedReason:   u32(gen["StateFailedReason"]),
			PowerState:          u32(gen["PowerState"]),
			AccessTech:          u32(gen["AccessTechnologies"]),
			UnlockRequired:      u32(gen["UnlockRequired"]),
			PrimarySimSlot:      u32(gen["PrimarySimSlot"]),
			SimPath:             objPath(gen["Sim"]),
			Registration:        u32(gpp["RegistrationState"]),
			OperatorName:        str(gpp["OperatorName"]),
			OperatorCode:        str(gpp["OperatorCode"]),
			FacilityLocks:       u32(gpp["EnabledFacilityLocks"]),
			PacketService:       u32(gpp["PacketServiceState"]),
			Signal:              signal(ifaces[ifSignal]),
		}
		if q, ok := gen["SignalQuality"].Value().([]interface{}); ok && len(q) == 2 {
			m.SignalQuality, _ = q[0].(uint32)
			m.SignalRecent, _ = q[1].(bool)
		}

		if paths, ok := gen["Bearers"].Value().([]dbus.ObjectPath); ok {
			for _, bp := range paths {
				if p := refs[string(bp)]; p != nil {
					m.Bearers = append(m.Bearers, bearer(string(bp), p))
				}
			}
		}
		if m.SimPath != "" {
			if p := refs[m.SimPath]; p != nil {
				m.Sim = &Sim{
					Imsi:         str(p["Imsi"]),
					Iccid:        str(p["SimIdentifier"]),
					OperatorName: str(p["OperatorName"]),
				}
			}
		}

		out = append(out, m)
	}
	return out
}

// signalTechs in order of preference, the most advanced first.
var signalTechs = []string{"Nr5g", "Lte", "Umts", "Gsm", "Evdo", "Cdma"}

func signal(p props) map[string]float64 {
	for _, tech := range signalTechs {
		vals, ok := p[tech].Value().(map[string]dbus.Variant)
		if !ok {
			continue
		}
		out := make(map[string]float64)
		for _, key := range []string{"rssi", "rsrp", "rsrq", "snr"} {
			if f, ok := vals[key].Value().(float64); ok {
				out[key] = f
			}
		}
		if len(out) > 0 {
			return out
		}
	}
	return nil
}

func bearer(path string, p props) Bearer {
	b := Bearer{
		Path:      path,
		Interface: str(p["Interface"]),
		Connected: boolean(p["Connected"]),
		Stats:     make(map[string]uint64),
	}
	if e, ok := p["ConnectionError"].Value().([]interface{}); ok && len(e) == 2 {
		b.ConnError, _ = e[1].(string)
	}
	if c, ok := p["Ip4Config"].Value().(map[string]dbus.Variant); ok {
		b.IPv4, b.IPv4Prefix = str(c["address"]), u32(c["prefix"])
	}
	if c, ok := p["Ip6Config"].Value().(map[string]dbus.Variant); ok {
		b.IPv6, b.IPv6Prefix = str(c["address"]), u32(c["prefix"])
	}
	if s, ok := p["Stats"].Value().(map[string]dbus.Variant); ok {
		for k, v := range s {
			b.Stats[k] = numconv.Uint64(v.Value())
		}
	}
	return b
}

func str(v dbus.Variant) string {
	s, _ := v.Value().(string)
	return s
}

func strs(v dbus.Variant) []string {
	s, _ := v.Value().([]string)
	return s
}

func u32(v dbus.Variant) uint32 {
	n, _ := v.Value().(uint32)
	return n
}

func i32(v dbus.Variant) int32 {
	n, _ := v.Value().(int32)
	return n
}

func boolean(v dbus.Variant) bool {
	b, _ := v.Value().(bool)
	return b
}

// objPath is the object a property points at, or "" for none, which
// ModemManager sends as "/".
func objPath(v dbus.Variant) string {
	p, _ := v.Value().(dbus.ObjectPath)
	if p == "/" {
		return ""
	}
	return string(p)
}
