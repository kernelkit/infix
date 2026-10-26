package collector

import (
	"encoding/json"
	"fmt"
	"math"
	"path/filepath"
	"strings"

	"github.com/kernelkit/infix/src/yangerd/internal/mmmonitor"
)

// modemRunDir is where modemd keeps per-modem state, modem0, modem1, ...
const modemRunDir = "/run/modemd"

// weakSignal is the signal quality, in percent, below which a
// registered modem raises a warning alarm.  modemd uses the same.
const weakSignal = 20

// MMModemState, from -1 (failed).
var modemStates = []string{
	"failed", "unknown", "initializing", "locked", "disabled", "disabling",
	"enabling", "enabled", "searching", "registered", "disconnecting",
	"connecting", "connected",
}

// MMModemStateFailedReason
var modemFailedReasons = []string{
	"none", "unknown", "sim-missing", "sim-error", "unknown",
	"esim-without-profiles",
}

// MMModemPowerState
var modemPowerStates = []string{"unknown", "off", "low", "on"}

// MMModem3gppRegistrationState, the ones the model has.
var modemRegistration = map[uint32]string{
	0: "idle", 1: "home", 2: "searching", 3: "denied", 5: "roaming",
}

// MMModem3gppPacketServiceState
var modemPacketService = []string{"unknown", "detached", "attached"}

// MMModem3gppFacility, by bit.
var modemFacilities = []string{
	"sim", "fixed-dialing", "phone-sim", "phone-first-sim", "phone-network",
	"phone-network-subset", "phone-service-provider", "phone-corporate",
}

// MMModemAccessTechnology, by bit.  LTE Cat-M and NB-IoT have no enum
// of their own in the model.
var modemAccessTechs = []string{
	"pots", "gsm", "gsm-compact", "gprs", "edge", "umts", "hsdpa", "hsupa",
	"hspa", "hspa-plus", "1xrtt", "evdo0", "evdoa", "evdob", "lte", "5gnr",
	"lte", "lte",
}

// MMModemLock values that leave the SIM unusable until unlocked.  PIN2
// and PUK2 only guard SIM services like fixed dialing.
var simLocks = map[uint32]string{2: "pin-required", 4: "puk-required"}

const (
	mmStateFailed      = -1
	mmFailedSimMissing = 2
	mmFailedSimError   = 3
)

// SetModemSource turns on modem reporting, modems is called on each GET.
func (c *HardwareCollector) SetModemSource(modems func() []mmmonitor.Modem) {
	c.modems = modems
}

// readModems names each modem ModemManager manages after the hardware
// component it belongs to.  Unnamed modems are not reported.
func (c *HardwareCollector) readModems(systemjson map[string]interface{}) map[string]mmmonitor.Modem {
	if c.modems == nil {
		return nil
	}
	names := c.modemNames(systemjson)
	named := make(map[string]mmmonitor.Modem)
	for _, m := range c.modems() {
		if name := names[m.Device]; m.Device != "" && name != "" {
			named[name] = m
		}
	}
	return named
}

// modemNames maps the USB device of each modem to the component it
// belongs to: the one modemd manages it with, otherwise the name the
// probe gave it at boot.
func (c *HardwareCollector) modemNames(systemjson map[string]interface{}) map[string]string {
	names := make(map[string]string)
	for _, p := range probedModems(systemjson) {
		names[asString(p["devpath"])] = fmt.Sprintf("modem%d", toInt(p["index"]))
	}
	files, _ := c.fs.Glob(modemRunDir + "/modem*/device")
	for _, f := range files {
		if data, err := c.fs.ReadFile(f); err == nil {
			names[strings.TrimSpace(string(data))] = filepath.Base(filepath.Dir(f))
		}
	}
	return names
}

func probedModems(systemjson map[string]interface{}) []map[string]interface{} {
	raw, _ := systemjson["modem"].([]interface{})
	out := make([]map[string]interface{}, 0, len(raw))
	for _, r := range raw {
		if p, ok := r.(map[string]interface{}); ok {
			out = append(out, p)
		}
	}
	return out
}

// modemComponents is a modem and a SIM component for each modem
// ModemManager manages, and a modem component for each modem the probe
// found that it does not, so that 'show hardware' has its uri.
func (c *HardwareCollector) modemComponents(named map[string]mmmonitor.Modem, systemjson map[string]interface{}) []interface{} {
	probed := make(map[string]map[string]interface{})
	for _, p := range probedModems(systemjson) {
		if dev := asString(p["devpath"]); dev != "" {
			probed[dev] = p
		}
	}

	components := make([]interface{}, 0)
	used := make(map[string]bool)
	for _, name := range sortedKeys(named) {
		m := named[name]
		phys := probed[m.Device]
		delete(probed, m.Device)
		used[name] = true

		ms := c.modemState(name, m)
		component := map[string]interface{}{
			"name":  name,
			"class": "infix-hardware:modem",
		}
		if phys != nil {
			if uri := asString(phys["uri"]); uri != "" {
				component["uri"] = []interface{}{uri}
			}
			usbIDs(ms, phys)
		}

		var stamps map[string]interface{}
		c.modemFile(name, "state.json", &stamps)
		if v := asString(stamps["last-change"]); v != "" {
			ms["last-change"] = v
		}
		state := map[string]interface{}{"oper-state": modemOperState(m.State)}
		if alarm := modemAlarm(m); alarm != "" {
			state["alarm-state"] = alarm
		}
		slc := asString(stamps["state-last-changed"])
		if slc != "" {
			state["state-last-changed"] = slc
		}
		component["state"] = state
		component["infix-hardware:modem-state"] = ms
		components = append(components, component)

		components = append(components, simComponent(m, name, slc))
	}

	for _, dev := range sortedKeys(probed) {
		phys := probed[dev]
		idx := toInt(phys["index"])
		name := asString(phys["name"])
		if name == "" {
			name = fmt.Sprintf("modem%d", idx)
		}
		for used[name] {
			idx++
			name = fmt.Sprintf("modem%d", idx)
		}
		used[name] = true

		component := map[string]interface{}{
			"name":  name,
			"class": "infix-hardware:modem",
			"state": map[string]interface{}{"oper-state": "unknown"},
		}
		if uri := asString(phys["uri"]); uri != "" {
			component["uri"] = []interface{}{uri}
		}
		ms := make(map[string]interface{})
		usbIDs(ms, phys)
		if len(ms) > 0 {
			component["infix-hardware:modem-state"] = ms
		}
		components = append(components, component)
	}

	return components
}

func (c *HardwareCollector) modemState(name string, m mmmonitor.Modem) map[string]interface{} {
	ms := make(map[string]interface{})
	setString := func(key, val string) {
		if val != "" {
			ms[key] = val
		}
	}
	setString("manufacturer", m.Manufacturer)
	setString("model", m.Model)
	setString("hardware-revision", m.HardwareRevision)
	setString("firmware-version", m.Revision)
	setString("serial-number", m.EquipmentIdentifier)
	if m.Sim != nil {
		setString("imsi", m.Sim.Imsi)
		setString("iccid", m.Sim.Iccid)
	}

	carrier := ""
	if !c.modemFile(name, "current-carrier", &carrier) || carrier == "" {
		carrier = "default"
	}
	ms["selected-carrier"] = carrier
	var carriers []string
	if c.modemFile(name, "supported-carriers", &carriers) {
		if l := nonEmpty(carriers); len(l) > 0 {
			ms["supported-carrier"] = l
		}
	}
	if l := nonEmpty(m.OwnNumbers); len(l) > 0 {
		ms["phone-number"] = l
	}

	ms["state"] = modemStateName(m.State)
	if m.State == mmStateFailed {
		ms["state-failed-reason"] = enumName(modemFailedReasons, m.StateFailedReason, "unknown")
	}
	ms["power-state"] = enumName(modemPowerStates, m.PowerState, "unknown")
	if locks := bitNames(modemFacilities, m.FacilityLocks); len(locks) > 0 {
		ms["enabled-locks"] = locks
	}

	ms["signal-quality"] = m.SignalQuality
	ms["signal-quality-recent"] = m.SignalRecent
	if v, ok := m.Signal["rssi"]; ok {
		ms["signal-rssi"] = int(math.Round(v))
	}
	if v, ok := m.Signal["rsrp"]; ok {
		ms["signal-rsrp"] = int(math.Round(v))
	}
	if v, ok := m.Signal["rsrq"]; ok {
		ms["signal-rsrq"] = fmt.Sprintf("%.1f", v)
	}
	if v, ok := m.Signal["snr"]; ok {
		ms["signal-sinr"] = fmt.Sprintf("%.1f", v)
	}

	cell := map[string]interface{}{
		"registration-state": modemRegistrationName(m.Registration),
		"service-state":      enumName(modemPacketService, m.PacketService, "unknown"),
		"network-type":       accessTechName(m.AccessTech),
	}
	if m.OperatorName != "" {
		cell["operator-name"] = m.OperatorName
	}
	if m.OperatorCode != "" {
		cell["operator-id"] = m.OperatorCode
	}
	ms["cellular"] = cell

	if loc := c.modemLocation(name); loc != nil {
		ms["location-state"] = loc
	}

	return ms
}

// modemLocation is the last location modemd read from the modem.
func (c *HardwareCollector) modemLocation(name string) map[string]interface{} {
	var data map[string]interface{}
	c.modemFile(name, "location/data.json", &data)
	loc := make(map[string]interface{})
	for _, key := range []string{"source", "latitude", "longitude", "altitude",
		"cell-id", "lac", "tac", "mcc", "mnc", "last-change"} {
		if v, ok := data[key]; ok && v != nil {
			loc[key] = v
		}
	}
	if len(loc) == 0 {
		return nil
	}
	return loc
}

// modemFile reads one of modemd's JSON files of a modem into v.
func (c *HardwareCollector) modemFile(name, file string, v interface{}) bool {
	data, err := c.fs.ReadFile(filepath.Join(modemRunDir, name, file))
	return err == nil && json.Unmarshal(data, v) == nil
}

func simComponent(m mmmonitor.Modem, modem, lastChanged string) map[string]interface{} {
	slot := m.PrimarySimSlot
	if slot == 0 {
		slot = 1
	}
	ss := map[string]interface{}{"slot": slot}

	var lock string
	switch {
	case m.Sim != nil:
		lock = "unlocked"
		if l, ok := simLocks[m.UnlockRequired]; ok {
			lock = l
		}
		if m.Sim.OperatorName != "" {
			ss["operator-name"] = m.Sim.OperatorName
		}
	case m.SimPath == "" || (m.State == mmStateFailed && m.StateFailedReason == mmFailedSimMissing):
		lock = "not-inserted"
	default:
		lock = "unknown"
	}
	ss["state"] = lock

	state := map[string]interface{}{"oper-state": simOperState(lock)}
	if lastChanged != "" {
		state["state-last-changed"] = lastChanged
	}
	return map[string]interface{}{
		"name":                     "sim" + strings.TrimPrefix(modem, "modem"),
		"class":                    "infix-hardware:sim",
		"state":                    state,
		"infix-hardware:sim-state": ss,
	}
}

func usbIDs(ms, phys map[string]interface{}) {
	if v := asString(phys["vid"]); v != "" {
		ms["usb-vendor-id"] = v
	}
	if v := asString(phys["pid"]); v != "" {
		ms["usb-product-id"] = v
	}
}

func modemStateName(state int32) string {
	i := int(state) + 1
	if i < 0 || i >= len(modemStates) {
		return "unknown"
	}
	return modemStates[i]
}

func modemOperState(state int32) string {
	switch modemStateName(state) {
	case "enabling", "enabled", "searching", "registered", "connecting",
		"connected", "disconnecting":
		return "enabled"
	case "failed", "disabled", "disabling", "locked":
		return "disabled"
	}
	return "unknown"
}

// modemAlarm is major for a failure the operator can fix, a SIM or PIN
// problem, minor for any other, and a warning for weak signal.
func modemAlarm(m mmmonitor.Modem) string {
	switch {
	case m.State == mmStateFailed && (m.StateFailedReason == mmFailedSimMissing || m.StateFailedReason == mmFailedSimError):
		return "major"
	case m.State == mmStateFailed:
		return "minor"
	}
	switch modemStateName(m.State) {
	case "registered", "connected":
		if m.SignalQuality < weakSignal {
			return "warning"
		}
	}
	return ""
}

func simOperState(lock string) string {
	switch lock {
	case "not-inserted":
		return "disabled"
	case "unlocked", "pin-required", "puk-required":
		return "enabled"
	}
	return "unknown"
}

func modemRegistrationName(reg uint32) string {
	if name, ok := modemRegistration[reg]; ok {
		return name
	}
	return "unknown"
}

// accessTechName is the most advanced technology in use.
func accessTechName(bits uint32) string {
	for i := len(modemAccessTechs) - 1; i >= 0; i-- {
		if bits&(1<<i) != 0 {
			return modemAccessTechs[i]
		}
	}
	return "unknown"
}

func enumName(names []string, val uint32, def string) string {
	if int(val) < len(names) {
		return names[val]
	}
	return def
}

func bitNames(names []string, bits uint32) []interface{} {
	out := make([]interface{}, 0)
	for i, name := range names {
		if bits&(1<<i) != 0 {
			out = append(out, name)
		}
	}
	return out
}

func nonEmpty(list []string) []interface{} {
	out := make([]interface{}, 0, len(list))
	for _, s := range list {
		if s != "" {
			out = append(out, s)
		}
	}
	return out
}

// WwanStates is the infix-interfaces:wwan augment of each modem
// interface, keyed by interface name: the modem it runs on and the state
// of its bearer.  A connected bearer names its interface, one that is not
// belongs to wwanN of modemN.
func (c *HardwareCollector) WwanStates() map[string]interface{} {
	named := c.readModems(c.readSystemJSON())
	out := make(map[string]interface{})
	for _, name := range sortedKeys(named) {
		for i, b := range named[name].Bearers {
			ifname := b.Interface
			if ifname == "" {
				if i > 0 {
					continue
				}
				ifname = "wwan" + strings.TrimPrefix(name, "modem")
			}
			if _, ok := out[ifname]; ok {
				continue
			}
			out[ifname] = map[string]interface{}{
				"modem":        name,
				"bearer-state": bearerState(b),
			}
		}
	}
	return out
}

func bearerState(b mmmonitor.Bearer) map[string]interface{} {
	bs := map[string]interface{}{
		"path":      b.Path,
		"connected": b.Connected,
	}
	if b.Interface != "" {
		bs["interface"] = b.Interface
	}
	if !b.Connected && b.ConnError != "" {
		bs["connection-failed-reason"] = b.ConnError
	}
	if b.IPv4 != "" {
		bs["ipv4-address"] = b.IPv4
		if b.IPv4Prefix > 0 {
			bs["ipv4-prefix-length"] = b.IPv4Prefix
		}
	}
	if b.IPv6 != "" {
		bs["ipv6-address"] = b.IPv6
		if b.IPv6Prefix > 0 {
			bs["ipv6-prefix-length"] = b.IPv6Prefix
		}
	}
	for key, leaf := range map[string]string{
		"rx-bytes":       "in-bytes",
		"tx-bytes":       "out-bytes",
		"total-rx-bytes": "total-in-bytes",
		"total-tx-bytes": "total-out-bytes",
		"total-duration": "total-duration",
	} {
		if v, ok := b.Stats[key]; ok {
			// uint64 leaves are strings in JSON
			bs[leaf] = fmt.Sprintf("%d", v)
		}
	}
	return bs
}
