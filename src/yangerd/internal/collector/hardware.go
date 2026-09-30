package collector

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode"

	"github.com/kernelkit/infix/src/yangerd/internal/nl80211"
	"github.com/kernelkit/infix/src/yangerd/internal/tree"
)

// Per-port devices: mt7915_phy0 -> phy0, marvell_alaska_phy7 -> phy7
var hwPortDeviceRe = regexp.MustCompile(`.*_((?:phy|sfp)\d*)$`)
var hwTrailingNumUnderscoreRe = regexp.MustCompile(`_(\d+)$`)
var hwPhyNumRe = regexp.MustCompile(`(\d+)$`)

const cpuComponent = "cpu"

const hardwareKey = "ietf-hardware:hardware"

// hwmon device names and thermal zone types that report an SoC die
// temperature, after normalization: a plain cpu/soc/core, Intel and AMD
// (coretemp, k10temp), Microchip SparX-5 and LAN969x (s5-temp), or a
// Marvell CN913x application (ap) or communication (cp<N>) processor
// cluster, optionally as the "-thermal" zone the DT names it.  Anything
// else after the dash (cpu-fan, soc-vdd) is a different device.
//
// Recognizing vendor names cannot be avoided, but this is the only place
// it happens.  Northbound, the sensors are found through the class of
// their parent component, see doc/hardware.md.
var socTempSourceRe = regexp.MustCompile(`^(cpu\d*|soc\d*|core\d*|coretemp|k10temp|s5-temp|ap|cp\d+)(-thermal(-.*)?)?$`)

// HardwareCollector gathers ietf-hardware operational data.  The
// inventory, radios and GPS receivers are polled; sensors are read when
// someone asks, see Live.
type HardwareCollector struct {
	cmd      CommandRunner
	fs       FileReader
	interval time.Duration

	enableWifi bool
	enableGPS  bool

	mu       sync.Mutex
	gps      []interface{}                     // last GPS poll
	radios   []interface{}                     // radio capabilities, without survey
	radioIf  map[string]string                 // radio name -> interface for its survey
	wifiInfo map[string]map[string]interface{} // PHY info the radios were built from
	reported map[string]bool                   // duplicate names already warned about

	radioRefresh chan struct{}
}

// NewHardwareCollector creates a HardwareCollector with the given dependencies.
func NewHardwareCollector(cmd CommandRunner, fs FileReader, interval time.Duration, enableWifi, enableGPS bool) *HardwareCollector {
	return &HardwareCollector{
		cmd:        cmd,
		fs:         fs,
		interval:   interval,
		enableWifi: enableWifi,
		enableGPS:  enableGPS,
		reported:   make(map[string]bool),

		radioRefresh: make(chan struct{}, 1),
	}
}

// Name implements Collector.
func (c *HardwareCollector) Name() string { return "hardware" }

// Interval implements Collector.
func (c *HardwareCollector) Interval() time.Duration { return c.interval }

// Collect implements Collector. It produces one tree key:
// "ietf-hardware:hardware".
func (c *HardwareCollector) Collect(ctx context.Context, t *tree.Tree) error {
	var gps []interface{}
	if c.enableGPS {
		gps = c.gpsReceiverComponents(ctx)
	}

	c.mu.Lock()
	c.gps = gps
	c.mu.Unlock()

	// Everything else is read by Live when asked, but the tree only
	// calls a provider for a key that exists.
	if t.GetCached(hardwareKey) == nil {
		t.Set(hardwareKey, json.RawMessage(`{}`))
	}
	return nil
}

// RequestRadioRefresh asks RunRadios to rebuild the radio capabilities.
// Called from nl80211 events: a phy came or went, the regulatory domain
// or the set of interfaces on a phy changed.
func (c *HardwareCollector) RequestRadioRefresh() {
	select {
	case c.radioRefresh <- struct{}{}:
	default:
	}
}

// RunRadios keeps the radio capabilities, which only change on nl80211
// events, built from the kernel.  It builds them once at start.
func (c *HardwareCollector) RunRadios(ctx context.Context) error {
	if !c.enableWifi {
		<-ctx.Done()
		return ctx.Err()
	}
	for {
		radios, ifaces, wifiInfo := c.radioCapabilities(ctx)
		c.mu.Lock()
		c.radios, c.radioIf, c.wifiInfo = radios, ifaces, wifiInfo
		c.mu.Unlock()

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-c.radioRefresh:
		}
	}
}

// Live is the tree provider for the hardware key.  The inventory is a
// cheap read of system.json and sysfs, sensors and radio surveys have
// no events, so all of it is read when asked.
func (c *HardwareCollector) Live() json.RawMessage {
	return c.assemble(context.Background())
}

func (c *HardwareCollector) assemble(ctx context.Context) json.RawMessage {
	systemjson := c.readSystemJSON()
	inventory := make([]interface{}, 0)
	inventory = append(inventory, c.motherboardComponent(systemjson)...)
	inventory = append(inventory, c.vpdComponents(systemjson)...)
	inventory = append(inventory, c.usbPortComponents(systemjson)...)

	c.mu.Lock()
	radios := cloneComponents(c.radios)
	ifaces := c.radioIf
	gps := cloneComponents(c.gps)
	wifiInfo := c.wifiInfo
	c.mu.Unlock()

	c.addSurveys(ctx, radios, ifaces)
	sensors := c.sensorComponents(ctx, wifiInfo)

	components := make([]interface{}, 0, len(inventory)+len(sensors)+len(radios)+len(gps)+1)
	components = append(components, inventory...)
	components = append(components, cpuComponentFor(sensors)...)
	components = append(components, sensors...)
	components = append(components, radios...)
	components = append(components, gps...)

	data, err := json.Marshal(map[string]interface{}{
		"component": c.uniqueNames(components),
	})
	if err != nil {
		return nil
	}
	return data
}

// addSurveys adds each radio's channel survey, read now: the counters
// move all the time and nl80211 sends no events for them.  radios are
// clones, the wifi-radio container is copied before it is changed.
func (c *HardwareCollector) addSurveys(ctx context.Context, radios []interface{}, ifaces map[string]string) {
	if len(radios) == 0 {
		return
	}
	client, err := nl80211.Dial()
	if err != nil {
		return
	}
	defer client.Close()

	for _, raw := range radios {
		component, ok := raw.(map[string]interface{})
		if !ok {
			continue
		}
		name, _ := component["name"].(string)
		channels := c.surveyData(ctx, client, ifaces[name])
		if len(channels) == 0 {
			continue
		}
		radio := map[string]interface{}{}
		if old, ok := component["infix-hardware:wifi-radio"].(map[string]interface{}); ok {
			for k, v := range old {
				radio[k] = v
			}
		}
		radio["survey"] = map[string]interface{}{"channel": channels}
		component["infix-hardware:wifi-radio"] = radio
	}
}

// cloneComponents copies the component maps, uniqueNames may rename
// one and the snapshot must stay as polled.
func cloneComponents(components []interface{}) []interface{} {
	out := make([]interface{}, 0, len(components))
	for _, raw := range components {
		if component, ok := raw.(map[string]interface{}); ok {
			clone := make(map[string]interface{}, len(component))
			for k, v := range component {
				clone[k] = v
			}
			out = append(out, clone)
			continue
		}
		out = append(out, raw)
	}
	return out
}

// sensorComponents reads every sensor.  Thermal zones first: the
// kernel mirrors each one as an hwmon device, which carries nothing the
// zone does not.
func (c *HardwareCollector) sensorComponents(ctx context.Context, wifiInfo map[string]map[string]interface{}) []interface{} {
	sensors := c.thermalSensorComponents(ctx)
	mirrored := make(map[string]bool, len(sensors))
	for _, raw := range sensors {
		if name, ok := raw.(map[string]interface{})["name"].(string); ok {
			mirrored[name] = true
		}
	}
	sensors = append(sensors, c.hwmonSensorComponents(ctx, mirrored)...)
	return adoptWifiSensors(sensors, wifiInfo)
}

func (c *HardwareCollector) readSystemJSON() map[string]interface{} {
	data, err := c.fs.ReadFile("/run/system.json")
	if err != nil {
		return map[string]interface{}{}
	}

	out := make(map[string]interface{})
	if err := json.Unmarshal(data, &out); err != nil {
		log.Printf("collector hardware: system.json: %v", err)
		return map[string]interface{}{}
	}

	return out
}

func (c *HardwareCollector) motherboardComponent(systemjson map[string]interface{}) []interface{} {
	if len(systemjson) == 0 {
		return nil
	}

	component := map[string]interface{}{
		"name":  "mainboard",
		"class": "iana-hardware:chassis",
		"state": map[string]interface{}{
			"admin-state": "unknown",
			"oper-state":  "enabled",
		},
	}

	if v, ok := systemjson["vendor"].(string); ok && v != "" {
		component["mfg-name"] = v
	}
	if v, ok := systemjson["product-name"].(string); ok && v != "" {
		component["model-name"] = v
	}
	if v, ok := systemjson["serial-number"].(string); ok && v != "" {
		component["serial-num"] = v
	}
	if v, ok := systemjson["part-number"].(string); ok && v != "" {
		component["hardware-rev"] = v
	}
	if v, ok := systemjson["mac-address"].(string); ok && v != "" {
		component["infix-hardware:phys-address"] = v
	}

	return []interface{}{component}
}

func vpdVendorExtensions(data interface{}) []interface{} {
	raw, ok := data.([]interface{})
	if !ok {
		return nil
	}

	vendorExtensions := make([]interface{}, 0, len(raw))
	for _, item := range raw {
		pair, ok := item.([]interface{})
		if !ok || len(pair) < 2 {
			continue
		}
		vendorExtensions = append(vendorExtensions, map[string]interface{}{
			"iana-enterprise-number": pair[0],
			"extension-data":         pair[1],
		})
	}

	return vendorExtensions
}

func (c *HardwareCollector) vpdComponents(systemjson map[string]interface{}) []interface{} {
	vpdRaw, ok := systemjson["vpd"].(map[string]interface{})
	if !ok {
		return nil
	}

	components := make([]interface{}, 0, len(vpdRaw))
	for _, vpdItemRaw := range vpdRaw {
		vpdItem, ok := vpdItemRaw.(map[string]interface{})
		if !ok {
			continue
		}

		component := map[string]interface{}{
			"class":                   "infix-hardware:vpd",
			"infix-hardware:vpd-data": map[string]interface{}{},
		}

		// Board authors name these in the device tree, as "cpu", "power",
		// "product", short words that collide with everything else sharing
		// the component namespace.  Say what they are.
		if board, ok := vpdItem["board"].(string); ok && board != "" {
			component["name"] = "vpd-" + board
		}

		dataRaw, ok := vpdItem["data"].(map[string]interface{})
		if ok {
			if mfgDateStr, ok := dataRaw["manufacture-date"].(string); ok && mfgDateStr != "" {
				if mfgDate, err := time.Parse("01/02/2006 15:04:05", mfgDateStr); err == nil {
					component["mfg-date"] = mfgDate.UTC().Format("2006-01-02T15:04:05Z")
				}
			}

			if mfg, ok := dataRaw["manufacturer"].(string); ok && mfg != "" {
				component["mfg-name"] = mfg
			}
			if model, ok := dataRaw["product-name"].(string); ok && model != "" {
				component["model-name"] = model
			}
			if serial, ok := dataRaw["serial-number"].(string); ok && serial != "" {
				component["serial-num"] = serial
			}

			vpdData, ok := component["infix-hardware:vpd-data"].(map[string]interface{})
			if !ok {
				vpdData = make(map[string]interface{})
				component["infix-hardware:vpd-data"] = vpdData
			}
			for key, val := range dataRaw {
				if val == nil {
					continue
				}
				if key == "vendor-extension" {
					if ext := vpdVendorExtensions(val); len(ext) > 0 {
						vpdData["infix-hardware:vendor-extension"] = ext
					}
					continue
				}
				vpdData[key] = val
			}
		}

		if _, ok := component["name"]; ok {
			components = append(components, component)
		}
	}

	return components
}

func (c *HardwareCollector) usbPortComponents(systemjson map[string]interface{}) []interface{} {
	usbPortsRaw, ok := systemjson["usb-ports"].([]interface{})
	if !ok {
		return nil
	}

	components := make([]interface{}, 0, len(usbPortsRaw))
	for _, usbPortRaw := range usbPortsRaw {
		usbPort, ok := usbPortRaw.(map[string]interface{})
		if !ok {
			continue
		}

		name, ok := usbPort["name"].(string)
		if !ok || name == "" {
			continue
		}
		path, ok := usbPort["path"].(string)
		if !ok || path == "" {
			continue
		}

		authorizedDefault, err := c.fs.ReadFile(path + "/authorized_default")
		if err != nil {
			continue
		}

		state := "locked"
		if strings.TrimSpace(string(authorizedDefault)) == "1" {
			state = "unlocked"
		}

		components = append(components, map[string]interface{}{
			"name":  name,
			"class": "infix-hardware:usb",
			"state": map[string]interface{}{
				"admin-state": state,
				"oper-state":  "enabled",
			},
		})
	}

	return components
}

// normalizeSensorName makes a list key out of a device name:
// sfp_2 -> sfp2, mt7915_phy0 -> phy0, cpu_thermal -> cpu-thermal.  A
// thermal zone and the hwmon device the kernel mirrors it as differ only
// in their separators, so the two spellings of one sensor come out
// identical, which is how the mirror is spotted.
func normalizeSensorName(name string) string {
	if m := hwPortDeviceRe.FindStringSubmatch(name); len(m) > 1 {
		name = m[1]
	}

	name = hwTrailingNumUnderscoreRe.ReplaceAllString(name, "$1")
	return strings.ReplaceAll(name, "_", "-")
}

// cpuComponentFor is the SoC that die temperature sensors belong to.  Only
// created when something references it, boards without a die sensor have
// nothing to say about their SoC.
func cpuComponentFor(sensors []interface{}) []interface{} {
	for _, raw := range sensors {
		if sensor, ok := raw.(map[string]interface{}); ok && sensor["parent"] == cpuComponent {
			return []interface{}{map[string]interface{}{
				"name":   cpuComponent,
				"class":  "iana-hardware:cpu",
				"parent": "mainboard",
				"state": map[string]interface{}{
					"admin-state": "unknown",
					"oper-state":  "enabled",
				},
			}}
		}
	}
	return nil
}

// uniqueNames renames duplicate component names "<name>-1", "<name>-2"
// and so on.  Components are keyed by name, so a duplicate fails every
// client parsing the tree.  Producers avoid collisions by construction,
// this is the net under them.  A renamed component keeps any children
// pointing at the original name, so it is a last resort, not a mechanism
// to rely on.  Each name is warned about once, this runs on every GET.
func (c *HardwareCollector) uniqueNames(components []interface{}) []interface{} {
	taken := make(map[string]bool, len(components))

	for _, raw := range components {
		component, ok := raw.(map[string]interface{})
		if !ok {
			continue
		}
		name, _ := component["name"].(string)
		if !taken[name] {
			taken[name] = true
			continue
		}

		unique := name
		for seq := 1; taken[unique]; seq++ {
			unique = fmt.Sprintf("%s-%d", name, seq)
		}
		c.mu.Lock()
		reported := c.reported[name]
		c.reported[name] = true
		c.mu.Unlock()
		if !reported {
			log.Printf("collector hardware: duplicate component %q, renaming one of them %q", name, unique)
		}
		component["name"] = unique
		taken[unique] = true
	}

	return components
}

// titleCase capitalizes the first letter of every word, like Python's
// str.title(): "volts-DC" -> "Volts-Dc".
func titleCase(s string) string {
	out := []rune(strings.ToLower(s))
	start := true
	for i, r := range out {
		if start && unicode.IsLetter(r) {
			out[i] = unicode.ToUpper(r)
		}
		start = !unicode.IsLetter(r)
	}
	return string(out)
}

func humanizeSensorLabel(label string) string {
	if label == "" {
		return ""
	}
	parts := strings.Fields(strings.ReplaceAll(label, "_", " "))
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		if part == strings.ToUpper(part) {
			out = append(out, part)
			continue
		}
		r := []rune(strings.ToLower(part))
		if len(r) == 0 {
			continue
		}
		r[0] = []rune(strings.ToUpper(string(r[0])))[0]
		out = append(out, string(r))
	}
	return strings.Join(out, " ")
}

func sensorComponent(name string, value int, valueType, valueScale, label string) map[string]interface{} {
	component := map[string]interface{}{
		"name":  name,
		"class": "iana-hardware:sensor",
		"sensor-data": map[string]interface{}{
			"value":           value,
			"value-type":      valueType,
			"value-scale":     valueScale,
			"value-precision": 0,
			"value-timestamp": yangDateTime(time.Now()),
			"oper-status":     "ok",
		},
	}

	if d := humanizeSensorLabel(label); d != "" {
		component["description"] = d
	}

	return component
}

// listDir names the entries of dir, sorted as the filesystem lists them.
func (c *HardwareCollector) listDir(dir string) ([]string, error) {
	matches, err := c.fs.Glob(dir + "/*")
	if err != nil {
		return nil, err
	}
	names := make([]string, 0, len(matches))
	for _, match := range matches {
		names = append(names, filepath.Base(match))
	}
	return names, nil
}

func (c *HardwareCollector) readSensorString(path string) (string, bool) {
	data, err := c.fs.ReadFile(path)
	if err != nil {
		return "", false
	}
	return strings.TrimSpace(string(data)), true
}

func (c *HardwareCollector) readSensorInt(path string) (int, bool) {
	data, err := c.fs.ReadFile(path)
	if err != nil {
		return 0, false
	}
	v, err := strconv.Atoi(strings.TrimSpace(string(data)))
	if err != nil {
		return 0, false
	}
	return v, true
}

func sensorName(baseName, sensorNum string) string {
	if sensorNum == "1" || sensorNum == "0" {
		return baseName
	}
	return baseName + sensorNum
}

func (c *HardwareCollector) wifiPhyInfo(ctx context.Context, client *nl80211.Client) map[string]map[string]interface{} {
	phyInfo := make(map[string]map[string]interface{})
	if err := ctx.Err(); err != nil {
		return phyInfo
	}

	phys, err := client.ListPhys()
	if err != nil {
		return phyInfo
	}

	for _, phy := range phys {
		if phy == "" {
			continue
		}
		phyInfo[phy] = map[string]interface{}{
			"band":        "Unknown",
			"iface":       "",
			"description": "WiFi Radio",
		}
	}

	phyNumToName := make(map[string]string)
	for phyName := range phyInfo {
		m := hwPhyNumRe.FindStringSubmatch(phyName)
		if len(m) > 1 {
			phyNumToName[m[1]] = phyName
		}
	}

	devMap, err := client.PhyInterfaces()
	if err == nil {
		for phyNum, ifaces := range devMap {
			phyName, ok := phyNumToName[phyNum]
			if !ok {
				continue
			}
			if len(ifaces) == 0 {
				continue
			}
			if entry, ok := phyInfo[phyName]; ok {
				entry["iface"] = ifaces[0]
			}
		}
	}

	for phy, info := range phyInfo {
		band := strDefault(info["band"], "Unknown")
		iface := strDefault(info["iface"], "")
		switch {
		case iface != "" && band != "Unknown":
			info["description"] = "WiFi Radio " + phy
		case band != "Unknown":
			info["description"] = "WiFi Radio (" + band + ")"
		case iface != "":
			info["description"] = "WiFi Radio " + phy
		default:
			info["description"] = "WiFi Radio"
		}
	}

	return phyInfo
}

// hwmonSensorComponents lists the hwmon sensors.  Devices named in
// mirrored are skipped, see the thermal zones.
func (c *HardwareCollector) hwmonSensorComponents(ctx context.Context, mirrored map[string]bool) []interface{} {
	components := make([]interface{}, 0)
	deviceSensors := make(map[string][]map[string]interface{})
	order := make([]string, 0) // devices in discovery order, like the kernel lists them

	hwmonEntries, err := c.listDir("/sys/class/hwmon")
	if err != nil {
		return components
	}

	for _, entry := range hwmonEntries {
		if !strings.HasPrefix(entry, "hwmon") {
			continue
		}
		hwmonPath := "/sys/class/hwmon/" + entry

		deviceName, ok := c.readSensorString(hwmonPath + "/name")
		if !ok || deviceName == "" {
			continue
		}
		// With THERMAL_HWMON the kernel mirrors every thermal zone as an
		// hwmon device, named after the zone with the separators changed.
		if mirrored[normalizeSensorName(deviceName)] {
			continue
		}
		if devName, ok := c.readSensorString(hwmonPath + "/device/name"); ok && devName != "" {
			deviceName = devName
		}

		baseName := normalizeSensorName(deviceName)
		if baseName == "" {
			continue
		}
		if _, seen := deviceSensors[baseName]; !seen {
			deviceSensors[baseName] = nil
			order = append(order, baseName)
		}

		entries, err := c.listDir(hwmonPath)
		if err != nil {
			continue
		}

		fanFiles := make([]string, 0)
		for _, e := range entries {
			if strings.HasPrefix(e, "fan") && strings.HasSuffix(e, "_input") {
				fanFiles = append(fanFiles, e)
			}
		}

		for _, e := range entries {
			if !strings.HasPrefix(e, "temp") || !strings.HasSuffix(e, "_input") {
				continue
			}
			sensorNum := strings.TrimPrefix(strings.SplitN(e, "_", 2)[0], "temp")
			value, ok := c.readSensorInt(hwmonPath + "/" + e)
			if !ok {
				continue
			}
			label := ""
			sensor := ""
			if rawLabel, ok := c.readSensorString(fmt.Sprintf("%s/temp%s_label", hwmonPath, sensorNum)); ok {
				label = rawLabel
				sensor = baseName + "-" + normalizeSensorName(rawLabel)
			} else {
				sensor = sensorName(baseName, sensorNum)
			}
			deviceSensors[baseName] = append(deviceSensors[baseName], sensorComponent(sensor, value, "celsius", "milli", label))
		}

		for _, e := range fanFiles {
			sensorNum := strings.TrimPrefix(strings.SplitN(e, "_", 2)[0], "fan")
			value, ok := c.readSensorInt(hwmonPath + "/" + e)
			if !ok {
				continue
			}
			label := ""
			sensor := ""
			if rawLabel, ok := c.readSensorString(fmt.Sprintf("%s/fan%s_label", hwmonPath, sensorNum)); ok {
				label = rawLabel
				sensor = baseName + "-" + normalizeSensorName(rawLabel)
			} else {
				sensor = sensorName(baseName, sensorNum)
			}
			deviceSensors[baseName] = append(deviceSensors[baseName], sensorComponent(sensor, value, "rpm", "units", label))
		}

		if len(fanFiles) == 0 {
			for _, e := range entries {
				if !strings.HasPrefix(e, "pwm") {
					continue
				}
				n := strings.TrimPrefix(e, "pwm")
				if _, err := strconv.Atoi(n); err != nil {
					continue
				}
				pwmRaw, ok := c.readSensorInt(hwmonPath + "/" + e)
				if !ok {
					continue
				}
				sensorNum := n
				value := int((float64(pwmRaw) / 255.0) * 100.0 * 1000.0)
				label := "PWM Fan"
				sensor := ""
				if rawLabel, ok := c.readSensorString(fmt.Sprintf("%s/pwm%s_label", hwmonPath, sensorNum)); ok {
					label = rawLabel
					sensor = baseName + "-" + normalizeSensorName(rawLabel)
				} else {
					sensor = sensorName(baseName, sensorNum)
				}
				deviceSensors[baseName] = append(deviceSensors[baseName], sensorComponent(sensor, value, "other", "milli", label))
			}
		}

		for _, e := range entries {
			if !strings.HasPrefix(e, "in") || !strings.HasSuffix(e, "_input") {
				continue
			}
			sensorNum := strings.TrimPrefix(strings.SplitN(e, "_", 2)[0], "in")
			value, ok := c.readSensorInt(hwmonPath + "/" + e)
			if !ok {
				continue
			}
			label := "voltage"
			sensor := ""
			if rawLabel, ok := c.readSensorString(fmt.Sprintf("%s/in%s_label", hwmonPath, sensorNum)); ok {
				label = rawLabel
				sensor = baseName + "-" + normalizeSensorName(rawLabel)
			} else {
				if sensorNum == "0" {
					sensor = baseName + "-voltage"
				} else {
					sensor = baseName + "-voltage" + sensorNum
				}
			}
			deviceSensors[baseName] = append(deviceSensors[baseName], sensorComponent(sensor, value, "volts-DC", "milli", label))
		}

		for _, e := range entries {
			if !strings.HasPrefix(e, "curr") || !strings.HasSuffix(e, "_input") {
				continue
			}
			sensorNum := strings.TrimPrefix(strings.SplitN(e, "_", 2)[0], "curr")
			value, ok := c.readSensorInt(hwmonPath + "/" + e)
			if !ok {
				continue
			}
			label := "current"
			sensor := ""
			if rawLabel, ok := c.readSensorString(fmt.Sprintf("%s/curr%s_label", hwmonPath, sensorNum)); ok {
				label = rawLabel
				sensor = baseName + "-" + normalizeSensorName(rawLabel)
			} else {
				if sensorNum == "1" {
					sensor = baseName + "-current"
				} else {
					sensor = baseName + "-current" + sensorNum
				}
			}
			deviceSensors[baseName] = append(deviceSensors[baseName], sensorComponent(sensor, value, "amperes", "milli", label))
		}

		for _, e := range entries {
			if !strings.HasPrefix(e, "power") || !strings.HasSuffix(e, "_input") {
				continue
			}
			sensorNum := strings.TrimPrefix(strings.SplitN(e, "_", 2)[0], "power")
			value, ok := c.readSensorInt(hwmonPath + "/" + e)
			if !ok {
				continue
			}
			label := "power"
			sensor := ""
			if rawLabel, ok := c.readSensorString(fmt.Sprintf("%s/power%s_label", hwmonPath, sensorNum)); ok {
				label = rawLabel
				sensor = baseName + "-" + normalizeSensorName(rawLabel)
			} else {
				if sensorNum == "1" {
					sensor = baseName + "-power"
				} else {
					sensor = baseName + "-power" + sensorNum
				}
			}
			deviceSensors[baseName] = append(deviceSensors[baseName], sensorComponent(sensor, value, "watts", "micro", label))
		}
	}

	for _, baseName := range order {
		sensors := deviceSensors[baseName]
		if len(sensors) == 0 {
			continue
		}
		parent := ""
		switch {
		case socTempSourceRe.MatchString(baseName):
			// SoC die sensors belong to the CPU, whatever the vendor
			// called the hwmon device
			parent = cpuComponent
		case len(sensors) > 1:
			// Multi-sensor devices, like SFP modules, head their own
			parent = baseName
			components = append(components, map[string]interface{}{
				"name":  baseName,
				"class": "iana-hardware:module",
			})
		}

		for _, sensor := range sensors {
			if parent != "" {
				sensor["parent"] = parent
			}
			components = append(components, sensor)
		}
	}

	return components
}

// adoptWifiSensors gives a radio its name back.  A WiFi PHY's hwmon
// device is named after the radio, so whatever hwmonSensorComponents
// built for it took the radio's name before wifiRadioComponents gets
// there, and uniqueNames would rename the radio instead, breaking the
// wifi/radio leafref that interfaces are bound by.  Its sensors hang off
// it, the way the die sensors hang off the CPU, and the module head a
// multi-sensor device would get is dropped: the radio component already
// is one.
func adoptWifiSensors(components []interface{}, wifiInfo map[string]map[string]interface{}) []interface{} {
	out := make([]interface{}, 0, len(components))
	for _, raw := range components {
		component, ok := raw.(map[string]interface{})
		if !ok {
			continue
		}
		name, _ := component["name"].(string)
		if _, radio := wifiInfo[name]; !radio {
			out = append(out, component)
			continue
		}

		if component["class"] == "iana-hardware:module" {
			continue // the radio heads its own sensors
		}

		kind := "sensor"
		if sd, ok := component["sensor-data"].(map[string]interface{}); ok {
			kind = strDefault(sd["value-type"], kind)
		}
		if kind == "celsius" {
			component["name"] = name + "-temp"
			component["description"] = "Temperature"
		} else {
			component["name"] = name + "-" + kind
			component["description"] = titleCase(kind)
		}
		component["parent"] = name
		out = append(out, component)
	}

	return out
}

func (c *HardwareCollector) thermalSensorComponents(ctx context.Context) []interface{} {
	components := make([]interface{}, 0)

	entries, err := c.listDir("/sys/class/thermal")
	if err != nil {
		return components
	}

	for _, entry := range entries {
		if !strings.HasPrefix(entry, "thermal_zone") {
			continue
		}
		zonePath := "/sys/class/thermal/" + entry
		zoneType, ok := c.readSensorString(zonePath + "/type")
		if !ok || zoneType == "" {
			continue
		}
		temp, ok := c.readSensorInt(zonePath + "/temp")
		if !ok {
			continue
		}

		component := sensorComponent(normalizeSensorName(zoneType), temp, "celsius", "milli", "")
		if socTempSourceRe.MatchString(component["name"].(string)) {
			component["parent"] = cpuComponent
		}
		components = append(components, component)
	}

	return components
}

func (c *HardwareCollector) surveyData(ctx context.Context, client *nl80211.Client, ifname string) []interface{} {
	if err := ctx.Err(); err != nil {
		return nil
	}
	if ifname == "" {
		return nil
	}
	iface, err := net.InterfaceByName(ifname)
	if err != nil {
		return nil
	}

	survey, err := client.Survey(iface.Index)
	if err != nil {
		return nil
	}

	channels := make([]interface{}, 0, len(survey))
	for _, entry := range survey {
		channel := map[string]interface{}{
			"frequency": entry["frequency"],
			"in-use":    entry["in_use"],
		}
		setIfPresent(channel, "noise", entry, "noise")
		setIfPresent(channel, "active-time", entry, "active_time")
		setIfPresent(channel, "busy-time", entry, "busy_time")
		setIfPresent(channel, "receive-time", entry, "receive_time")
		setIfPresent(channel, "transmit-time", entry, "transmit_time")
		channels = append(channels, channel)
	}

	return channels
}

func (c *HardwareCollector) phyInfo(ctx context.Context, client *nl80211.Client, phyName string) map[string]interface{} {
	if err := ctx.Err(); err != nil {
		return map[string]interface{}{}
	}
	phyInfo, err := client.PhyInfo(phyName)
	if err != nil {
		return map[string]interface{}{}
	}

	return phyInfo
}

func convertPhyInfo(phyInfo map[string]interface{}) map[string]interface{} {
	result := map[string]interface{}{
		"bands":          []interface{}{},
		"driver":         nil,
		"manufacturer":   "Unknown",
		"max-interfaces": map[string]interface{}{},
	}

	bandsRaw, _ := phyInfo["bands"].([]interface{})
	bands := make([]interface{}, 0, len(bandsRaw))
	for _, bandRaw := range bandsRaw {
		band, ok := bandRaw.(map[string]interface{})
		if !ok {
			continue
		}
		bandData := map[string]interface{}{
			"band": strconv.Itoa(toInt(band["band"])),
		}
		if name := strDefault(band["name"], ""); name != "" {
			bandData["name"] = name
		}
		if v, ok := band["ht_capable"].(bool); ok && v {
			bandData["ht-capable"] = true
		}
		if v, ok := band["vht_capable"].(bool); ok && v {
			bandData["vht-capable"] = true
		}
		if v, ok := band["he_capable"].(bool); ok && v {
			bandData["he-capable"] = true
		}
		bands = append(bands, bandData)
	}
	result["bands"] = bands

	if driver, ok := phyInfo["driver"].(string); ok && driver != "" {
		result["driver"] = driver
	}
	if manufacturer, ok := phyInfo["manufacturer"].(string); ok && manufacturer != "" {
		result["manufacturer"] = manufacturer
	}

	maxInterfaces := make(map[string]interface{})
	ifCombRaw, _ := phyInfo["interface_combinations"].([]interface{})
	for _, combRaw := range ifCombRaw {
		comb, ok := combRaw.(map[string]interface{})
		if !ok {
			continue
		}
		limitsRaw, _ := comb["limits"].([]interface{})
		for _, limitRaw := range limitsRaw {
			limit, ok := limitRaw.(map[string]interface{})
			if !ok {
				continue
			}
			typesRaw, _ := limit["types"].([]interface{})
			hasAP := false
			for _, t := range typesRaw {
				if s, ok := t.(string); ok && s == "AP" {
					hasAP = true
					break
				}
			}
			if !hasAP {
				continue
			}
			apMax := toInt(limit["max"])
			if cur, ok := maxInterfaces["ap"]; !ok || apMax > toInt(cur) {
				maxInterfaces["ap"] = apMax
			}
		}
	}
	result["max-interfaces"] = maxInterfaces

	return result
}

func channelFromFrequency(freq int) (int, bool) {
	switch {
	case freq >= 2412 && freq <= 2484:
		return (freq - 2407) / 5, true
	case freq >= 5170 && freq <= 5825:
		return (freq - 5000) / 5, true
	case freq >= 5955 && freq <= 7115:
		return (freq - 5950) / 5, true
	default:
		return 0, false
	}
}

// radioCapabilities lists the radios without their survey, the
// interface each survey is read from, and the PHY info they were built
// from so the sensors can be matched to them.
func (c *HardwareCollector) radioCapabilities(ctx context.Context) ([]interface{}, map[string]string, map[string]map[string]interface{}) {
	components := make([]interface{}, 0)
	ifaces := map[string]string{}
	wifiInfo := map[string]map[string]interface{}{}
	client, err := nl80211.Dial()
	if err != nil {
		return components, ifaces, wifiInfo
	}
	defer client.Close()

	wifiInfo = c.wifiPhyInfo(ctx, client)

	for phyName, phyData := range wifiInfo {
		component := map[string]interface{}{
			"name":        phyName,
			"class":       "infix-hardware:wifi",
			"description": strDefault(phyData["description"], "WiFi Radio"),
		}

		wifiRadioData := make(map[string]interface{})
		iwInfo := c.phyInfo(ctx, client, phyName)
		phyDetails := convertPhyInfo(iwInfo)

		if manufacturer := strDefault(phyDetails["manufacturer"], "Unknown"); manufacturer != "Unknown" {
			component["mfg-name"] = manufacturer
		}

		if bands, ok := phyDetails["bands"].([]interface{}); ok && len(bands) > 0 {
			wifiRadioData["bands"] = bands
		}
		if driver := strDefault(phyDetails["driver"], ""); driver != "" {
			wifiRadioData["driver"] = driver
		}
		if maxIf, ok := phyDetails["max-interfaces"].(map[string]interface{}); ok && len(maxIf) > 0 {
			wifiRadioData["max-interfaces"] = maxIf
		}

		setIfPresent(wifiRadioData, "max-txpower", iwInfo, "max_txpower")

		supportedChannelsMap := make(map[int]bool)
		bandsRaw, _ := iwInfo["bands"].([]interface{})
		for _, bandRaw := range bandsRaw {
			band, ok := bandRaw.(map[string]interface{})
			if !ok {
				continue
			}
			freqsRaw, _ := band["frequencies"].([]interface{})
			for _, freqRaw := range freqsRaw {
				freq := toInt(freqRaw)
				if channel, ok := channelFromFrequency(freq); ok {
					supportedChannelsMap[channel] = true
				}
			}
		}
		if len(supportedChannelsMap) > 0 {
			supported := make([]int, 0, len(supportedChannelsMap))
			for ch := range supportedChannelsMap {
				supported = append(supported, ch)
			}
			sort.Ints(supported)
			supportedIface := make([]interface{}, 0, len(supported))
			for _, ch := range supported {
				supportedIface = append(supportedIface, ch)
			}
			wifiRadioData["supported-channels"] = supportedIface
		}

		wifiRadioData["num-virtual-interfaces"] = toInt(iwInfo["num_virtual_interfaces"])

		ifaces[phyName] = strDefault(phyData["iface"], "")

		if len(wifiRadioData) > 0 {
			component["infix-hardware:wifi-radio"] = wifiRadioData
		}

		components = append(components, component)
	}

	return components, ifaces, wifiInfo
}

func gpsdPoll(ctx context.Context) map[string]interface{} {
	dialer := &net.Dialer{Timeout: 500 * time.Millisecond}
	conn, err := dialer.DialContext(ctx, "tcp", "127.0.0.1:2947")
	if err != nil {
		return map[string]interface{}{}
	}
	defer conn.Close()

	_ = conn.SetDeadline(time.Now().Add(500 * time.Millisecond))

	reader := bufio.NewReader(conn)
	_, _ = reader.ReadBytes('\n')

	if _, err := conn.Write([]byte("?WATCH={\"enable\":true,\"json\":true};\n?POLL;\n")); err != nil {
		return map[string]interface{}{}
	}

	buf := bytes.Buffer{}
	for i := 0; i < 5; i++ {
		chunk := make([]byte, 4096)
		n, err := conn.Read(chunk)
		if err != nil || n == 0 {
			break
		}
		buf.Write(chunk[:n])
		for _, line := range splitLines(buf.String()) {
			var msg map[string]interface{}
			if json.Unmarshal([]byte(line), &msg) != nil {
				continue
			}
			if cls, ok := msg["class"].(string); ok && cls == "POLL" {
				return msg
			}
		}
	}

	return map[string]interface{}{}
}

func countUsedSatellites(sats []interface{}) int {
	used := 0
	for _, satRaw := range sats {
		sat, ok := satRaw.(map[string]interface{})
		if !ok {
			continue
		}
		if v, ok := sat["used"].(bool); ok && v {
			used++
		}
	}
	return used
}

func (c *HardwareCollector) gpsReceiverComponents(ctx context.Context) []interface{} {
	components := make([]interface{}, 0)
	gpsDevices := make(map[string]map[string]string)

	devPaths, _ := c.fs.Glob("/dev/gps[0-3]")
	for _, devPath := range devPaths {
		actual, err := c.cmd.Run(ctx, "readlink", "-f", devPath)
		if err != nil {
			continue
		}
		actualPath := strings.TrimSpace(string(actual))
		if actualPath == "" {
			continue
		}
		gpsDevices[actualPath] = map[string]string{
			"name":    filepath.Base(devPath),
			"symlink": devPath,
		}
	}

	if len(gpsDevices) == 0 {
		return components
	}

	poll := gpsdPoll(ctx)
	active := toInt(poll["active"])

	tpvByDev := make(map[string]map[string]interface{})
	tpvRaw, _ := poll["tpv"].([]interface{})
	for _, itemRaw := range tpvRaw {
		item, ok := itemRaw.(map[string]interface{})
		if !ok {
			continue
		}
		dev, _ := item["device"].(string)
		if dev != "" {
			tpvByDev[dev] = item
		}
	}

	skyByDev := make(map[string]map[string]interface{})
	skyRaw, _ := poll["sky"].([]interface{})
	for _, itemRaw := range skyRaw {
		item, ok := itemRaw.(map[string]interface{})
		if !ok {
			continue
		}
		dev, _ := item["device"].(string)
		if dev != "" {
			skyByDev[dev] = item
		}
	}

	for actualPath, dev := range gpsDevices {
		name := dev["name"]
		symlink := dev["symlink"]

		component := map[string]interface{}{
			"name":        name,
			"class":       "infix-hardware:gps",
			"description": "GPS/GNSS Receiver",
		}

		gpsData := make(map[string]interface{})
		gpsData["device"] = symlink

		tpv := tpvByDev[actualPath]
		if tpv == nil {
			tpv = tpvByDev[symlink]
		}
		if tpv == nil && len(tpvByDev) == 1 {
			for _, v := range tpvByDev {
				tpv = v
			}
		}

		sky := skyByDev[actualPath]
		if sky == nil {
			sky = skyByDev[symlink]
		}
		if sky == nil && len(skyByDev) == 1 {
			for _, v := range skyByDev {
				sky = v
			}
		}

		gpsData["activated"] = active > 0 && len(tpv) > 0

		if driver, ok := tpv["driver"].(string); ok && driver != "" {
			gpsData["driver"] = driver
		}

		switch toInt(tpv["mode"]) {
		case 2:
			gpsData["fix-mode"] = "2d"
		case 3:
			gpsData["fix-mode"] = "3d"
		default:
			gpsData["fix-mode"] = "none"
		}

		if lat, ok := tpv["lat"]; ok {
			gpsData["latitude"] = fmt.Sprintf("%.6f", toFloat64(lat))
		}
		if lon, ok := tpv["lon"]; ok {
			gpsData["longitude"] = fmt.Sprintf("%.6f", toFloat64(lon))
		}
		if alt, ok := tpv["altHAE"]; ok {
			gpsData["altitude"] = fmt.Sprintf("%.1f", toFloat64(alt))
		}

		satVis := 0
		satUsed := 0
		if sky != nil {
			sats, _ := sky["satellites"].([]interface{})
			if len(sats) > 0 {
				satVis = len(sats)
				satUsed = countUsedSatellites(sats)
			}
			if satVis == 0 {
				satVis = toInt(zeroIfNil(sky["nSat"]))
				if satVis == 0 {
					satVis = toInt(zeroIfNil(sky["satellites_visible"]))
				}
			}
			if satUsed == 0 {
				satUsed = toInt(zeroIfNil(sky["uSat"]))
				if satUsed == 0 {
					satUsed = toInt(zeroIfNil(sky["satellites_used"]))
				}
			}
		}

		if satVis == 0 {
			satVis = toInt(zeroIfNil(tpv["nSat"]))
			if satVis == 0 {
				satVis = toInt(zeroIfNil(tpv["satellites_visible"]))
			}
		}
		if satUsed == 0 {
			satUsed = toInt(zeroIfNil(tpv["uSat"]))
			if satUsed == 0 {
				satUsed = toInt(zeroIfNil(tpv["satellites_used"]))
			}
		}

		if satUsed > satVis {
			satVis = satUsed
		}
		gpsData["satellites-visible"] = satVis
		gpsData["satellites-used"] = satUsed

		pps, _ := c.fs.Glob(fmt.Sprintf("/dev/pps%s", strings.TrimPrefix(name, "gps")))
		gpsData["pps-available"] = len(pps) > 0

		component["infix-hardware:gps-receiver"] = gpsData
		components = append(components, component)
	}

	return components
}

func toFloat64(v interface{}) float64 {
	switch n := v.(type) {
	case float64:
		return n
	case float32:
		return float64(n)
	case int:
		return float64(n)
	case int64:
		return float64(n)
	case json.Number:
		f, _ := n.Float64()
		return f
	case string:
		f, _ := strconv.ParseFloat(n, 64)
		return f
	default:
		return 0
	}
}
