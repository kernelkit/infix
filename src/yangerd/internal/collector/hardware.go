package collector

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/kernelkit/infix/src/yangerd/internal/tree"
)

var hwSensorTypeRe = regexp.MustCompile(`.*_(phy|sfp|fan|temp|sensor|psu|cpu|gpu|memory|disk)\d*$`)
var hwSensorSuffixExtractRe = regexp.MustCompile(`.*_((?:phy|sfp|fan|temp|sensor|psu|cpu|gpu|memory|disk)\d*)$`)
var hwTrailingNumUnderscoreRe = regexp.MustCompile(`_(\d+)$`)
var hwPhyNumRe = regexp.MustCompile(`(\d+)$`)

// HardwareCollector gathers ietf-hardware operational data.
type HardwareCollector struct {
	cmd      CommandRunner
	fs       FileReader
	interval time.Duration

	enableWifi bool
	enableGPS  bool
}

// NewHardwareCollector creates a HardwareCollector with the given dependencies.
func NewHardwareCollector(cmd CommandRunner, fs FileReader, interval time.Duration, enableWifi, enableGPS bool) *HardwareCollector {
	return &HardwareCollector{
		cmd:        cmd,
		fs:         fs,
		interval:   interval,
		enableWifi: enableWifi,
		enableGPS:  enableGPS,
	}
}

// Name implements Collector.
func (c *HardwareCollector) Name() string { return "hardware" }

// Interval implements Collector.
func (c *HardwareCollector) Interval() time.Duration { return c.interval }

// Collect implements Collector. It produces one tree key:
// "ietf-hardware:hardware".
func (c *HardwareCollector) Collect(ctx context.Context, t *tree.Tree) error {
	systemjson := c.readSystemJSON()

	components := make([]interface{}, 0)
	components = append(components, c.motherboard_component(systemjson)...)
	components = append(components, c.vpd_components(systemjson)...)
	components = append(components, c.usb_port_components(systemjson)...)
	components = append(components, c.hwmon_sensor_components(ctx)...)
	components = append(components, c.thermal_sensor_components(ctx)...)

	if c.enableWifi {
		components = append(components, c.wifi_radio_components(ctx)...)
	}
	if c.enableGPS {
		components = append(components, c.gps_receiver_components(ctx)...)
	}

	hardware := map[string]interface{}{
		"component": components,
	}

	if data, err := json.Marshal(hardware); err == nil {
		t.Set("ietf-hardware:hardware", data)
	}

	return nil
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

func (c *HardwareCollector) motherboard_component(systemjson map[string]interface{}) []interface{} {
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

func vpd_vendor_extensions(data interface{}) []interface{} {
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

func (c *HardwareCollector) vpd_components(systemjson map[string]interface{}) []interface{} {
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

		if board, ok := vpdItem["board"].(string); ok && board != "" {
			component["name"] = board
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
					if ext := vpd_vendor_extensions(val); len(ext) > 0 {
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

func (c *HardwareCollector) usb_port_components(systemjson map[string]interface{}) []interface{} {
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

func normalize_sensor_name(name string) string {
	name = strings.TrimSuffix(name, "-thermal")
	name = strings.TrimSuffix(name, "_thermal")

	if m := hwSensorSuffixExtractRe.FindStringSubmatch(name); len(m) > 1 {
		name = m[1]
	}

	name = hwTrailingNumUnderscoreRe.ReplaceAllString(name, "$1")
	return name
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

func (c *HardwareCollector) listDir(ctx context.Context, dir string) ([]string, error) {
	out, err := c.cmd.Run(ctx, "ls", dir)
	if err != nil {
		return nil, err
	}
	return splitLines(string(out)), nil
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

func (c *HardwareCollector) get_wifi_phy_info(ctx context.Context) map[string]map[string]interface{} {
	phyInfo := make(map[string]map[string]interface{})

	listOut, err := c.cmd.Run(ctx, "/usr/libexec/infix/iw.py", "list")
	if err != nil {
		return phyInfo
	}

	var phys []interface{}
	if err := json.Unmarshal(listOut, &phys); err != nil {
		return phyInfo
	}

	for _, phyRaw := range phys {
		phy, ok := phyRaw.(string)
		if !ok || phy == "" {
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

	devOut, err := c.cmd.Run(ctx, "/usr/libexec/infix/iw.py", "dev")
	if err == nil {
		var devMap map[string]interface{}
		if json.Unmarshal(devOut, &devMap) == nil {
			for phyNum, ifacesRaw := range devMap {
				phyName, ok := phyNumToName[phyNum]
				if !ok {
					continue
				}
				ifaces, ok := ifacesRaw.([]interface{})
				if !ok || len(ifaces) == 0 {
					continue
				}
				iface, ok := ifaces[0].(string)
				if !ok {
					continue
				}
				if entry, ok := phyInfo[phyName]; ok {
					entry["iface"] = iface
				}
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

func (c *HardwareCollector) hwmon_sensor_components(ctx context.Context) []interface{} {
	components := make([]interface{}, 0)
	deviceSensors := make(map[string][]map[string]interface{})

	hwmonEntries, err := c.listDir(ctx, "/sys/class/hwmon")
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
		if devName, ok := c.readSensorString(hwmonPath + "/device/name"); ok && devName != "" {
			deviceName = devName
		}

		baseName := normalize_sensor_name(deviceName)
		if baseName == "" {
			continue
		}

		entries, err := c.listDir(ctx, hwmonPath)
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
				sensor = baseName + "-" + normalize_sensor_name(rawLabel)
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
				sensor = baseName + "-" + normalize_sensor_name(rawLabel)
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
					sensor = baseName + "-" + normalize_sensor_name(rawLabel)
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
				sensor = baseName + "-" + normalize_sensor_name(rawLabel)
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
				sensor = baseName + "-" + normalize_sensor_name(rawLabel)
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
				sensor = baseName + "-" + normalize_sensor_name(rawLabel)
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

	for baseName, sensors := range deviceSensors {
		if len(sensors) > 1 {
			components = append(components, map[string]interface{}{
				"name":  baseName,
				"class": "iana-hardware:module",
			})
			for _, sensor := range sensors {
				sensor["parent"] = baseName
				components = append(components, sensor)
			}
			continue
		}
		for _, sensor := range sensors {
			components = append(components, sensor)
		}
	}

	wifiInfo := c.get_wifi_phy_info(ctx)
	for _, componentRaw := range components {
		component, ok := componentRaw.(map[string]interface{})
		if !ok {
			continue
		}
		name, ok := component["name"].(string)
		if !ok {
			continue
		}
		if strings.HasPrefix(name, "radio") {
			if phy, ok := wifiInfo[name]; ok {
				if desc, ok := phy["description"].(string); ok && desc != "" {
					component["description"] = desc
				}
			}
		}
	}

	return components
}

func (c *HardwareCollector) thermal_sensor_components(ctx context.Context) []interface{} {
	components := make([]interface{}, 0)

	entries, err := c.listDir(ctx, "/sys/class/thermal")
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

		components = append(components, sensorComponent(normalize_sensor_name(zoneType), temp, "celsius", "milli", ""))
	}

	return components
}

func (c *HardwareCollector) get_survey_data(ctx context.Context, ifname string) []interface{} {
	if ifname == "" {
		return nil
	}
	out, err := c.cmd.Run(ctx, "/usr/libexec/infix/iw.py", "survey", ifname)
	if err != nil {
		return nil
	}

	var survey []interface{}
	if err := json.Unmarshal(out, &survey); err != nil {
		return nil
	}

	channels := make([]interface{}, 0, len(survey))
	for _, entryRaw := range survey {
		entry, ok := entryRaw.(map[string]interface{})
		if !ok {
			continue
		}
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

func (c *HardwareCollector) get_phy_info(ctx context.Context, phyName string) map[string]interface{} {
	out, err := c.cmd.Run(ctx, "/usr/libexec/infix/iw.py", "info", phyName)
	if err != nil {
		return map[string]interface{}{}
	}

	var phyInfo map[string]interface{}
	if err := json.Unmarshal(out, &phyInfo); err != nil {
		return map[string]interface{}{}
	}
	return phyInfo
}

func convert_iw_phy_info_for_yanger(phyInfo map[string]interface{}) map[string]interface{} {
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
			"name": strDefault(band["name"], "Unknown"),
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

func (c *HardwareCollector) wifi_radio_components(ctx context.Context) []interface{} {
	components := make([]interface{}, 0)
	wifiInfo := c.get_wifi_phy_info(ctx)

	for phyName, phyData := range wifiInfo {
		component := map[string]interface{}{
			"name":        phyName,
			"class":       "infix-hardware:wifi",
			"description": strDefault(phyData["description"], "WiFi Radio"),
		}

		wifiRadioData := make(map[string]interface{})
		iwInfo := c.get_phy_info(ctx, phyName)
		phyDetails := convert_iw_phy_info_for_yanger(iwInfo)

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

		iface := strDefault(phyData["iface"], "")
		if channels := c.get_survey_data(ctx, iface); len(channels) > 0 {
			wifiRadioData["survey"] = map[string]interface{}{
				"channel": channels,
			}
		}

		if len(wifiRadioData) > 0 {
			component["infix-hardware:wifi-radio"] = wifiRadioData
		}

		components = append(components, component)
	}

	return components
}

func gpsd_poll(ctx context.Context) map[string]interface{} {
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

func (c *HardwareCollector) gps_receiver_components(ctx context.Context) []interface{} {
	components := make([]interface{}, 0)
	gpsDevices := make(map[string]map[string]string)

	for i := 0; i < 4; i++ {
		devPath := fmt.Sprintf("/dev/gps%d", i)
		if _, err := c.cmd.Run(ctx, "ls", devPath); err != nil {
			continue
		}
		actual, err := c.cmd.Run(ctx, "readlink", "-f", devPath)
		if err != nil {
			continue
		}
		actualPath := strings.TrimSpace(string(actual))
		if actualPath == "" {
			continue
		}
		gpsDevices[actualPath] = map[string]string{
			"name":    fmt.Sprintf("gps%d", i),
			"symlink": devPath,
		}
	}

	if len(gpsDevices) == 0 {
		return components
	}

	poll := gpsd_poll(ctx)
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

		ppsPath := fmt.Sprintf("/dev/pps%s", strings.TrimPrefix(name, "gps"))
		if _, err := c.cmd.Run(ctx, "ls", ppsPath); err == nil {
			gpsData["pps-available"] = true
		} else {
			gpsData["pps-available"] = false
		}

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
