package collector

import (
	"fmt"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

const usbDevices = "/sys/bus/usb/devices"

// usbBusIDRe matches a device entry in sysfs, e.g. 1-1.2: port 2 of
// the hub on port 1 of bus 1.  Interfaces carry a :config.number suffix
// and root hubs are named usbN.
var usbBusIDRe = regexp.MustCompile(`^\d+-[\d.]+$`)

var usbClasses = map[string]string{
	"01": "audio",
	"02": "communications",
	"03": "hid",
	"05": "physical",
	"06": "image",
	"07": "printer",
	"08": "mass-storage",
	"09": "hub",
	"0a": "cdc-data",
	"0b": "smart-card",
	"0d": "content-security",
	"0e": "video",
	"0f": "personal-healthcare",
	"10": "audio-video",
	"dc": "diagnostic",
	"e0": "wireless",
	"ef": "miscellaneous",
	"fe": "application-specific",
	"ff": "vendor-specific",
}

// usbPortComponents builds one component per USB port named in
// system.json, followed by the devices attached to it.  A port covers
// every root hub listed under its paths, a USB 3 controller has one per
// speed, and is unlocked only when all of them accept new devices by
// default.
func (c *HardwareCollector) usbPortComponents(systemjson map[string]interface{}) []interface{} {
	usbPortsRaw, ok := systemjson["usb-ports"].([]interface{})
	if !ok {
		return nil
	}

	entries, _ := c.listDir(usbDevices)
	devices := make([]string, 0, len(entries))
	for _, entry := range entries {
		if usbBusIDRe.MatchString(entry) {
			devices = append(devices, entry)
		}
	}
	sort.Slice(devices, func(i, j int) bool {
		return usbBusIDLess(devices[i], devices[j])
	})

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

		state := "unlocked"
		buses := make(map[string]bool)
		pathsRaw, _ := usbPort["paths"].([]interface{})
		for _, pathRaw := range pathsRaw {
			path, ok := pathRaw.(string)
			if !ok || path == "" {
				continue
			}
			authorizedDefault, ok := c.readSensorString(filepath.Join(path, "authorized_default"))
			if !ok {
				continue
			}
			buses[filepath.Base(path)] = true
			if authorizedDefault != "1" {
				state = "locked"
			}
		}
		if len(buses) == 0 {
			continue
		}

		attached := make([]string, 0)
		for _, device := range devices {
			if buses["usb"+strings.SplitN(device, "-", 2)[0]] {
				attached = append(attached, device)
			}
		}

		usage := "idle"
		if len(attached) > 0 {
			usage = "active"
		}
		components = append(components, map[string]interface{}{
			"name":  name,
			"class": "infix-hardware:usb",
			"state": map[string]interface{}{
				"admin-state": state,
				"oper-state":  "enabled",
				"usage-state": usage,
			},
		})
		for _, device := range attached {
			components = append(components, c.usbDeviceComponent(name, device, entries))
		}
	}

	return components
}

// usbBusIDLess orders bus ids by their numbers: 1-1.2 before 1-1.10.
func usbBusIDLess(a, b string) bool {
	as := strings.FieldsFunc(a, func(r rune) bool { return r == '-' || r == '.' })
	bs := strings.FieldsFunc(b, func(r rune) bool { return r == '-' || r == '.' })
	for i := 0; i < len(as) && i < len(bs); i++ {
		ai, _ := strconv.Atoi(as[i])
		bi, _ := strconv.Atoi(bs[i])
		if ai != bi {
			return ai < bi
		}
	}
	return len(as) < len(bs)
}

// usbDeviceComponent describes the device at busid, parented to port.
func (c *HardwareCollector) usbDeviceComponent(port, busid string, entries []string) map[string]interface{} {
	path := filepath.Join(usbDevices, busid)
	component := map[string]interface{}{
		"name":   "usb-" + busid,
		"class":  "infix-hardware:usb-device",
		"parent": port,
		"uri":    []interface{}{"usb://" + path},
	}

	for _, attr := range [][2]string{
		{"manufacturer", "mfg-name"},
		{"product", "model-name"},
		{"serial", "serial-num"},
	} {
		if data, ok := c.readSensorString(filepath.Join(path, attr[0])); ok && data != "" {
			component[attr[1]] = data
		}
	}

	if bcd, ok := c.readSensorString(filepath.Join(path, "bcdDevice")); ok && len(bcd) == 4 {
		if major, err := strconv.Atoi(bcd[:2]); err == nil {
			if _, err := strconv.Atoi(bcd[2:]); err == nil {
				component["hardware-rev"] = fmt.Sprintf("%d.%s", major, bcd[2:])
			}
		}
	}

	device := map[string]interface{}{}
	for _, attr := range [][2]string{
		{"idVendor", "vendor-id"},
		{"idProduct", "product-id"},
		{"version", "usb-version"},
		{"speed", "speed"},
	} {
		data, ok := c.readSensorString(filepath.Join(path, attr[0]))
		if !ok || data == "" {
			continue
		}
		if strings.HasSuffix(attr[1], "-id") {
			data = strings.ToLower(data)
		}
		device[attr[1]] = data
	}

	if authorized, ok := c.readSensorString(filepath.Join(path, "authorized")); ok {
		device["authorized"] = authorized == "1"
	}

	if interfaces := c.usbInterfaces(busid, entries); len(interfaces) > 0 {
		device["interface"] = interfaces
	}

	component["infix-hardware:usb-device"] = device
	return component
}

// usbInterfaces lists the functions of the device at busid.  The
// kernel only creates them once the device is authorized.
func (c *HardwareCollector) usbInterfaces(busid string, entries []string) []interface{} {
	names := make([]string, 0)
	for _, entry := range entries {
		if strings.HasPrefix(entry, busid+":") {
			names = append(names, entry)
		}
	}
	sort.Strings(names)

	interfaces := make([]interface{}, 0, len(names))
	for _, name := range names {
		path := filepath.Join(usbDevices, name)
		numberHex, ok := c.readSensorString(filepath.Join(path, "bInterfaceNumber"))
		if !ok {
			continue
		}
		number, err := strconv.ParseUint(numberHex, 16, 8)
		if err != nil {
			continue
		}
		iface := map[string]interface{}{"number": int(number)}
		if code, ok := c.readSensorString(filepath.Join(path, "bInterfaceClass")); ok && code != "" {
			if class, ok := usbClasses[code]; ok {
				code = class
			}
			iface["class"] = code
		}
		interfaces = append(interfaces, iface)
	}

	return interfaces
}

// usbDeviceParents points WiFi radios, modems and GPS receivers at the
// USB device they live on.  gpsTTY maps a receiver's component name to the
// tty behind its /dev/gpsN link.
func (c *HardwareCollector) usbDeviceParents(components []interface{}, gpsTTY map[string]string) []interface{} {
	devices := make(map[string]bool)
	for _, raw := range components {
		component, ok := raw.(map[string]interface{})
		if ok && component["class"] == "infix-hardware:usb-device" {
			devices[component["name"].(string)] = true
		}
	}
	if len(devices) == 0 {
		return components
	}

	phys := make(map[string]string)
	matches, _ := c.fs.Glob(usbDevices + "/*:*/ieee80211/*")
	for _, match := range matches {
		iface := filepath.Base(filepath.Dir(filepath.Dir(match)))
		phys[filepath.Base(match)] = "usb-" + strings.SplitN(iface, ":", 2)[0]
	}

	for _, raw := range components {
		component, ok := raw.(map[string]interface{})
		if !ok {
			continue
		}
		name, _ := component["name"].(string)

		parent := ""
		switch component["class"] {
		case "infix-hardware:wifi":
			parent = phys[name]
		case "infix-hardware:modem":
			uris, _ := component["uri"].([]interface{})
			for _, raw := range uris {
				if uri, _ := raw.(string); strings.HasPrefix(uri, "usb:") {
					parent = "usb-" + filepath.Base(uri)
				}
			}
		case "infix-hardware:gps":
			busid := c.usbTTYBusID(gpsTTY[name])
			if busid != "" {
				parent = "usb-" + busid
				if _, ok := component["uri"]; !ok {
					component["uri"] = []interface{}{"usb://" + filepath.Join(usbDevices, busid)}
				}
			}
		}
		if devices[parent] {
			component["parent"] = parent
		}
	}

	return components
}

// usbTTYBusID finds the USB device behind a tty, e.g. ttyACM0 -> 1-1.2.
// The tty's device link lands on the USB interface, or for a usb-serial
// port one level below it, and the kernel resolves .. after following
// the link, so the device directory is found by climbing until idVendor
// shows up.  Not cleaned with filepath, that would fold the .. away.
func (c *HardwareCollector) usbTTYBusID(tty string) string {
	if tty == "" {
		return ""
	}
	dir := "/sys/class/tty/" + tty + "/device"
	for i := 0; i < 3; i++ {
		dir += "/.."
		if _, ok := c.readSensorString(dir + "/idVendor"); !ok {
			continue
		}
		busnum, ok := c.readSensorString(dir + "/busnum")
		if !ok {
			return ""
		}
		devpath, ok := c.readSensorString(dir + "/devpath")
		if !ok {
			return ""
		}
		return busnum + "-" + devpath
	}
	return ""
}
