// SPDX-License-Identifier: MIT

package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"html/template"
	"log"
	"math"
	"net"
	"net/http"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"time"

	"infix/webui/internal/restconf"
)

// yangInt64 unmarshals a YANG numeric value that RESTCONF encodes as a
// JSON string (e.g. "1024000") or, occasionally, as a bare number.
type yangInt64 int64

func (y *yangInt64) UnmarshalJSON(b []byte) error {
	var s string
	if json.Unmarshal(b, &s) == nil {
		v, err := strconv.ParseInt(s, 10, 64)
		if err != nil {
			return err
		}
		*y = yangInt64(v)
		return nil
	}
	var v int64
	if err := json.Unmarshal(b, &v); err != nil {
		return err
	}
	*y = yangInt64(v)
	return nil
}

// yangBool unmarshals a YANG boolean that RESTCONF may encode as a
// JSON string ("true"/"false") or as a bare boolean.
type yangBool bool

func (y *yangBool) UnmarshalJSON(b []byte) error {
	var s string
	if json.Unmarshal(b, &s) == nil {
		v, err := strconv.ParseBool(s)
		if err != nil {
			return err
		}
		*y = yangBool(v)
		return nil
	}
	var v bool
	if err := json.Unmarshal(b, &v); err != nil {
		return err
	}
	*y = yangBool(v)
	return nil
}

// yangFloat64 unmarshals a YANG decimal value that RESTCONF may encode
// as a JSON string (e.g. "0.12") or as a bare number.
type yangFloat64 float64

func (y *yangFloat64) UnmarshalJSON(b []byte) error {
	var s string
	if json.Unmarshal(b, &s) == nil {
		v, err := strconv.ParseFloat(s, 64)
		if err != nil {
			return err
		}
		*y = yangFloat64(v)
		return nil
	}
	var v float64
	if err := json.Unmarshal(b, &v); err != nil {
		return err
	}
	*y = yangFloat64(v)
	return nil
}

// RESTCONF JSON structures for ietf-system:system-state.

type systemStateWrapper struct {
	SystemState systemState `json:"ietf-system:system-state"`
}

type systemState struct {
	Platform platform      `json:"platform"`
	Clock    clock         `json:"clock"`
	Software software      `json:"infix-system:software"`
	Resource resourceUsage `json:"infix-system:resource-usage"`
	DNS      *dnsResolver  `json:"infix-system:dns-resolver"`
	NTP      *ntpSources   `json:"infix-system:ntp"`
}

// dnsResolver mirrors the effective resolver state — servers carry an
// origin (static vs dhcp) and, for dhcp, the interface they arrived on.
type dnsResolver struct {
	Server []dnsServer `json:"server"`
	Search []string    `json:"search"`
}

type dnsServer struct {
	Address   string `json:"address"`
	Origin    string `json:"origin"`
	Interface string `json:"interface"`
}

type ntpSources struct {
	Sources struct {
		Source []ntpSource `json:"source"`
	} `json:"sources"`
}

type ntpSource struct {
	Address string `json:"address"`
	State   string `json:"state"`
}

type platform struct {
	OSName    string `json:"os-name"`
	OSVersion string `json:"os-version"`
	Machine   string `json:"machine"`
}

type clock struct {
	BootDatetime    string `json:"boot-datetime"`
	CurrentDatetime string `json:"current-datetime"`
}

type software struct {
	Booted string         `json:"booted"`
	Slot   []softwareSlot `json:"slot"`
}

type softwareSlot struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}

type resourceUsage struct {
	Memory      memoryInfo     `json:"memory"`
	LoadAverage loadAverage    `json:"load-average"`
	Filesystem  []filesystemFS `json:"filesystem"`
}

type memoryInfo struct {
	Total     yangInt64 `json:"total"`
	Free      yangInt64 `json:"free"`
	Available yangInt64 `json:"available"`
}

type loadAverage struct {
	Load1min  yangFloat64 `json:"load-1min"`
	Load5min  yangFloat64 `json:"load-5min"`
	Load15min yangFloat64 `json:"load-15min"`
}

type filesystemFS struct {
	MountPoint string    `json:"mount-point"`
	Size       yangInt64 `json:"size"`
	Used       yangInt64 `json:"used"`
	Available  yangInt64 `json:"available"`
}

// RESTCONF JSON structures for ietf-hardware:hardware.

// Short forms of hardware-class identities — see shortClass(). Kept here so
// the dashboard and Status > Hardware handlers route the same way.
const (
	classChassis = "chassis" // ietf-hardware:chassis
	classUSB     = "usb"     // infix-hardware:usb
	classWiFi    = "wifi"    // infix-hardware:wifi
	classGPS     = "gps"     // infix-hardware:gps
)

// sensorStatusOK matches the ietf-hardware sensor-status "ok" enum value.
const sensorStatusOK = "ok"

// admin-state values for configurable ietf-hardware components. The infix
// deviation in infix-hardware.yang restricts the base type to these two.
const (
	adminStateLocked   = "locked"
	adminStateUnlocked = "unlocked"
)

type hardwareWrapper struct {
	Hardware struct {
		Component []hwComponentJSON `json:"component"`
	} `json:"ietf-hardware:hardware"`
}

type hwComponentJSON struct {
	Name        string         `json:"name"`
	Class       string         `json:"class"`
	Description string         `json:"description"`
	Parent      string         `json:"parent"`
	MfgName     string         `json:"mfg-name"`
	ModelName   string         `json:"model-name"`
	SerialNum   string         `json:"serial-num"`
	HardwareRev string         `json:"hardware-rev"`
	PhysAddress string         `json:"infix-hardware:phys-address"`
	WiFiRadio   *wifiRadioHWJSON `json:"infix-hardware:wifi-radio"`
	GPSReceiver *struct{}        `json:"infix-hardware:gps-receiver"`
	SensorData  *struct {
		ValueType  string    `json:"value-type"`
		Value      yangInt64 `json:"value"`
		ValueScale string    `json:"value-scale"`
		OperStatus string    `json:"oper-status"`
	} `json:"sensor-data"`
	State *struct {
		AdminState string `json:"admin-state"`
		OperState  string `json:"oper-state"`
	} `json:"state"`
}

// Template data structures.

type dashboardData struct {
	PageData
	Hostname     string
	Contact      string
	Location     string
	OSName       string
	OSVersion    string
	Machine      string
	CurrentTime  string
	Software     string
	Uptime       string
	MemTotal     int64
	MemUsed      int64
	MemPercent   int
	MemClass     string
	Load1        string
	Load5        string
	Load15       string
	CPUClass     string
	Disks        []diskEntry
	Board        boardInfo
	KeyVitals    []sensorEntry // Overview's at-a-glance subset: CPU/SoC + wifi-radio temperatures and fan RPMs. Status > Hardware has the full inventory.
	// Connectivity card.
	Gateways      []gatewayEntry
	InternetProbe string // address pinged for the Internet reachability row
	DNSServers    []dnsServer
	DNSSearch     []string
	NTPSync       string // "" / the selected NTP source address
	// Addresses card.
	Addresses []ifaceAddrEntry
	Error     string
}

// gatewayEntry is a default route's next-hop.
type gatewayEntry struct {
	Addr  string
	Iface string
}

// ifaceAddrEntry is one L3 interface's addresses for the Addresses card.
type ifaceAddrEntry struct {
	Name  string
	Addrs []string
	Up    bool
}

type boardInfo struct {
	Model        string
	Manufacturer string
	SerialNum    string
	HardwareRev  string
	BaseMAC      string
}

type sensorEntry struct {
	Name  string
	Value string
	Type  string // "temperature", "fan", "voltage", etc.
}

type wifiEntry struct {
	Name         string
	Manufacturer string
	Bands        string // all supported bands, e.g. "2.4 GHz, 5 GHz"
	Standards    string
	MaxAP        int
}

type diskEntry struct {
	Mount     string
	Size      string
	Available string
	Percent   int
	Class     string // "" / "is-warn" / "is-crit"
	ReadOnly  bool
}

// internetProbe is the address the Connectivity card pings for its Internet
// reachability row — a well-known, stable anycast resolver.
const internetProbe = "1.1.1.1"

// DashboardHandler serves the main dashboard page.
type DashboardHandler struct {
	Template *template.Template
	RC       *restconf.Client
}

// Index renders the dashboard (GET /).
func (h *DashboardHandler) Index(w http.ResponseWriter, r *http.Request) {
	data := dashboardData{
		PageData: newPageData(w, r, "dashboard", "Overview"),
	}

	// Detach from the request context so that RESTCONF calls survive
	// browser connection resets (common during login redirects).
	// The RESTCONF client's own 10 s timeout still bounds each call.
	ctx := context.WithoutCancel(r.Context())
	var (
		state   systemStateWrapper
		hw      hardwareWrapper
		sysConf struct {
			System struct {
				Hostname string `json:"hostname"`
				Contact  string `json:"contact"`
				Location string `json:"location"`
			} `json:"ietf-system:system"`
		}
		ifaces                   interfacesWrapper
		routes                   ribWrapper
		stateErr, hwErr, confErr error
		wg                       sync.WaitGroup
	)

	wg.Add(5)
	go func() {
		defer wg.Done()
		stateErr = h.RC.Get(ctx, "/data/ietf-system:system-state", &state)
	}()
	go func() {
		defer wg.Done()
		hwErr = h.RC.Get(ctx, "/data/ietf-hardware:hardware", &hw)
	}()
	go func() {
		defer wg.Done()
		confErr = h.RC.Get(ctx, "/data/ietf-system:system", &sysConf)
	}()
	// Connectivity/Addresses cards are best-effort: a failure here logs but
	// doesn't fault the whole dashboard, so the card simply renders empty.
	go func() {
		defer wg.Done()
		if err := h.RC.Get(ctx, "/data/ietf-interfaces:interfaces", &ifaces); err != nil {
			log.Printf("restconf interfaces: %v", err)
		}
	}()
	go func() {
		defer wg.Done()
		if err := h.RC.Get(ctx, "/data/ietf-routing:routing", &routes); err != nil {
			log.Printf("restconf routing: %v", err)
		}
	}()
	wg.Wait()

	if stateErr != nil {
		log.Printf("restconf system-state: %v", stateErr)
		data.Error = "Could not fetch system information — retrying…"
		// Post-upgrade / fresh-boot race: yanger or sysrepo not ready
		// yet. Schedule a page-level meta-refresh so the dashboard
		// self-recovers instead of stranding the user on a stale
		// error banner.
		data.RetryAfter = 5
	} else {
		ss := state.SystemState
		data.OSName = ss.Platform.OSName
		data.OSVersion = ss.Platform.OSVersion
		data.Machine = ss.Platform.Machine
		if data.Machine == "arm64" {
			data.Machine = "aarch64"
		}
		data.Software = softwareVersion(ss.Software)
		data.Uptime = computeUptime(ss.Clock.BootDatetime, ss.Clock.CurrentDatetime)
		data.CurrentTime = formatCurrentTime(ss.Clock.CurrentDatetime)

		total := int64(ss.Resource.Memory.Total)
		avail := int64(ss.Resource.Memory.Available)
		data.MemTotal = total / 1024 // KiB → MiB
		data.MemUsed = (total - avail) / 1024
		if total > 0 {
			data.MemPercent = int(float64(total-avail) / float64(total) * 100)
		}

		switch {
		case data.MemPercent >= 90:
			data.MemClass = "is-crit"
		case data.MemPercent >= 70:
			data.MemClass = "is-warn"
		default:
			data.MemClass = ""
		}

		la := ss.Resource.LoadAverage
		if la1 := float64(la.Load1min); la1 >= 0.9 {
			data.CPUClass = "is-crit"
		} else if la1 >= 0.7 {
			data.CPUClass = "is-warn"
		}

		data.Load1 = strconv.FormatFloat(float64(la.Load1min), 'f', 2, 64)
		data.Load5 = strconv.FormatFloat(float64(la.Load5min), 'f', 2, 64)
		data.Load15 = strconv.FormatFloat(float64(la.Load15min), 'f', 2, 64)

		for _, fs := range ss.Resource.Filesystem {
			size := int64(fs.Size)
			used := int64(fs.Used)
			avail := int64(fs.Available)
			pct := 0
			if size > 0 {
				pct = int(float64(used) / float64(size) * 100)
			}
			// Read-only signature: used == size, no slack at all.
			// Squashfs/erofs rootfs reports this — pinning it at 100 %
			// for the lifetime of the running image, with nothing the
			// operator can do about it. Skip the crit/warn coloring so
			// it doesn't read as an actionable alert.
			readOnly := size > 0 && used == size && avail == 0
			diskClass := ""
			if !readOnly {
				switch {
				case pct >= 90:
					diskClass = "is-crit"
				case pct >= 70:
					diskClass = "is-warn"
				}
			}
			data.Disks = append(data.Disks, diskEntry{
				Mount:     fs.MountPoint,
				Size:      humanKiB(size),
				Available: humanKiB(avail),
				Percent:   pct,
				Class:     diskClass,
				ReadOnly:  readOnly,
			})
		}
	}

	if hwErr != nil {
		log.Printf("restconf hardware: %v", hwErr)
	} else {
		// Two passes: first build a name → class map so keyVital can tell
		// whether a celsius sensor lives on a wifi component (and thus
		// belongs in Key Vitals).
		classByName := make(map[string]string, len(hw.Hardware.Component))
		for _, c := range hw.Hardware.Component {
			classByName[c.Name] = shortClass(c.Class)
		}
		for _, c := range hw.Hardware.Component {
			if shortClass(c.Class) == classChassis {
				data.Board = boardInfo{
					Model:        c.ModelName,
					Manufacturer: c.MfgName,
					SerialNum:    c.SerialNum,
					HardwareRev:  c.HardwareRev,
					BaseMAC:      c.PhysAddress,
				}
			}
			if v, ok := keyVital(c, classByName); ok {
				data.KeyVitals = append(data.KeyVitals, v)
			}
		}
	}

	if confErr != nil {
		log.Printf("restconf system config: %v", confErr)
	} else {
		data.Hostname = sysConf.System.Hostname
		data.Contact = sysConf.System.Contact
		data.Location = sysConf.System.Location
	}

	// Connectivity & Addresses cards (best-effort, independent of the above).
	data.Gateways = defaultGateways(routes)
	data.InternetProbe = internetProbe
	if stateErr == nil {
		if dns := state.SystemState.DNS; dns != nil {
			data.DNSServers = dns.Server
			data.DNSSearch = dns.Search
		}
		if ntp := state.SystemState.NTP; ntp != nil {
			for _, s := range ntp.Sources.Source {
				if s.State == "selected" {
					data.NTPSync = s.Address
					break
				}
			}
		}
	}
	data.Addresses = ifaceAddresses(ifaces)

	tmplName := "dashboard.html"
	if r.Header.Get("HX-Request") == "true" {
		tmplName = "content"
	}
	if err := h.Template.ExecuteTemplate(w, tmplName, data); err != nil {
		log.Printf("template error: %v", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
	}
}

// defaultGateways extracts the installed default-route next-hops (v4 and v6).
func defaultGateways(rw ribWrapper) []gatewayEntry {
	var ribs []ribJSON
	if rw.Routing != nil {
		ribs = rw.Routing.Ribs.Rib
	} else if rw.Ribs != nil {
		ribs = rw.Ribs.Rib
	}
	var gws []gatewayEntry
	for _, rib := range ribs {
		for _, rt := range rib.Routes.Route {
			if rt.Active == nil {
				continue // only installed routes
			}
			switch rt.destinationPrefix() {
			case "0.0.0.0/0", "::/0":
			default:
				continue
			}
			iface, addr := rt.NextHop.resolve()
			if addr == "" {
				continue
			}
			gws = append(gws, gatewayEntry{Addr: addr, Iface: iface})
		}
	}
	return gws
}

// ifaceAddresses lists every interface carrying an IP address, loopback last.
func ifaceAddresses(iw interfacesWrapper) []ifaceAddrEntry {
	var l3, lo []ifaceAddrEntry
	for _, ifc := range iw.Interfaces.Interface {
		var addrs []string
		for _, ip := range []*ipCfg{ifc.IPv4, ifc.IPv6} {
			if ip == nil {
				continue
			}
			for _, a := range ip.Address {
				addrs = append(addrs, fmt.Sprintf("%s/%d", a.IP, int(a.PrefixLength)))
			}
		}
		if len(addrs) == 0 {
			continue
		}
		e := ifaceAddrEntry{Name: ifc.Name, Addrs: addrs, Up: ifc.OperStatus == "up"}
		if prettyIfType(ifc.Type) == ifTypeLoopback {
			lo = append(lo, e)
		} else {
			l3 = append(l3, e)
		}
	}
	return append(l3, lo...)
}

// Reachability pings an address and returns a tiny indicator — a pulsing green
// dot when it replies, a red ✗ otherwise.  Connectivity-card slots load it
// async (hx-trigger="load") so the dashboard render isn't blocked on a probe.
// The target is validated as a literal IP so the ping argument can never become
// an arbitrary host; a link-local IPv6 is scoped with its egress interface.
func (h *DashboardHandler) Reachability(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	ip := r.URL.Query().Get("ip")
	addr := net.ParseIP(ip)
	if addr == nil {
		fmt.Fprint(w, `<span class="status-dot reach-pending" title="unknown"></span>`)
		return
	}
	target := ip
	if iface := r.URL.Query().Get("iface"); iface != "" && addr.IsLinkLocalUnicast() && validZone(iface) {
		target = ip + "%" + iface
	}

	ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
	defer cancel()
	if err := exec.CommandContext(ctx, "ping", "-c", "1", "-W", "1", target).Run(); err != nil {
		fmt.Fprintf(w, `<span class="reach-x" title="No reply from %s">&#10007;</span>`,
			template.HTMLEscapeString(ip))
		return
	}
	fmt.Fprintf(w, `<span class="status-dot status-up reach-pulse" title="%s replied"></span>`,
		template.HTMLEscapeString(ip))
}

// validZone guards the IPv6 zone (interface name) passed to ping: a plain
// interface name, no shell metacharacters (exec args aren't shell-parsed, but
// keep it tight).
func validZone(s string) bool {
	for _, c := range s {
		ok := c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' ||
			c >= '0' && c <= '9' || c == '.' || c == '_' || c == '-'
		if !ok {
			return false
		}
	}
	return s != ""
}

// softwareVersion returns the version string for the booted software slot.
func softwareVersion(sw software) string {
	for _, slot := range sw.Slot {
		if slot.Name == sw.Booted {
			return slot.Version
		}
	}
	return ""
}

// computeUptime returns a human-readable uptime string from RFC3339 timestamps.
func computeUptime(boot, now string) string {
	bootT, err := time.Parse(time.RFC3339, boot)
	if err != nil {
		return ""
	}
	nowT, err := time.Parse(time.RFC3339, now)
	if err != nil {
		nowT = time.Now()
	}

	d := nowT.Sub(bootT)
	days := int(d.Hours()) / 24
	hours := int(d.Hours()) % 24
	mins := int(d.Minutes()) % 60

	switch {
	case days > 0:
		return fmt.Sprintf("%dd %dh %dm", days, hours, mins)
	case hours > 0:
		return fmt.Sprintf("%dh %dm", hours, mins)
	default:
		return fmt.Sprintf("%dm", mins)
	}
}

// formatCurrentTime formats an RFC3339 timestamp as "2006-01-02 15:04:05 +00:00".
func formatCurrentTime(s string) string {
	t, err := time.Parse(time.RFC3339, s)
	if err != nil {
		return ""
	}
	return t.UTC().Format("2006-01-02 15:04:05 +00:00")
}

// keyVital picks the dashboard's "Key Vitals" rows out of the hardware
// component stream — the small at-a-glance subset for the Overview page:
// CPU/SoC/core temperatures, wifi-radio temperatures, SFP temperatures,
// and any fan RPM. Everything else (board temps, voltages, per-port
// sensors, …) lives on Status > Hardware. classByName maps every
// component's Name → short class so we can identify a celsius sensor's
// parent without a second scan per call.
func keyVital(c hwComponentJSON, classByName map[string]string) (sensorEntry, bool) {
	if c.SensorData == nil || c.SensorData.OperStatus != sensorStatusOK {
		return sensorEntry{}, false
	}
	switch c.SensorData.ValueType {
	case "celsius":
		switch {
		case c.Name == "cpu", c.Name == "soc", c.Name == "core":
			// CPU / SoC / core temperatures.
		case classByName[c.Parent] == classWiFi:
			// WiFi radio temperatures whose sensor-data lives under
			// the radio component as a child.
		case strings.HasPrefix(c.Name, "radio"),
			strings.HasPrefix(c.Name, "phy"):
			// WiFi radio temperatures whose sensor-data is a
			// standalone iana-hardware:sensor (yanger labels these
			// radio0, phy0, … per its normaliser). The parent-class
			// branch above won't catch them.
		case strings.HasPrefix(c.Name, "sfp"):
			// SFP module temperatures (per ietf_hardware.py
			// normalisation, names canonicalise to sfp0, sfp1, ...).
		default:
			return sensorEntry{}, false
		}
	case "rpm":
		// every fan qualifies
	default:
		return sensorEntry{}, false
	}
	return sensorEntry{
		Name:  c.Name,
		Value: formatSensor(c.SensorData.ValueType, int64(c.SensorData.Value), c.SensorData.ValueScale),
		Type:  c.SensorData.ValueType,
	}, true
}

// summarizeWiFiRadio collapses an ietf-hardware component's wifi-radio
// container into the row-shaped wifiEntry used by both the dashboard and
// the Status > Hardware page. Detail beyond this summary (per-channel,
// per-band capabilities) lives on the dedicated /wifi page.
func summarizeWiFiRadio(c hwComponentJSON) wifiEntry {
	e := wifiEntry{Name: c.Name, Manufacturer: c.MfgName}
	if c.WiFiRadio == nil {
		return e
	}
	var bandNames []string
	var ht, vht, he bool
	for _, b := range c.WiFiRadio.Bands {
		ht = ht || b.HTCapable
		vht = vht || b.VHTCapable
		he = he || b.HECapable
		name := b.Name
		if name == "" {
			name = b.Band
		}
		if name != "" {
			bandNames = append(bandNames, name)
		}
	}
	if len(bandNames) == 0 && c.WiFiRadio.Band != "" {
		bandNames = append(bandNames, c.WiFiRadio.Band)
	}
	e.Bands = strings.Join(bandNames, ", ")
	var stds []string
	if ht {
		stds = append(stds, "11n")
	}
	if vht {
		stds = append(stds, "11ac")
	}
	if he {
		stds = append(stds, "11ax")
	}
	e.Standards = strings.Join(stds, "/")
	if c.WiFiRadio.MaxInterfaces != nil {
		e.MaxAP = c.WiFiRadio.MaxInterfaces.AP
	}
	return e
}

// shortClass strips the YANG module prefix from a hardware class identity.
func shortClass(full string) string {
	if i := strings.LastIndex(full, ":"); i >= 0 {
		return full[i+1:]
	}
	return full
}

// formatSensor converts a raw sensor value to a human-readable string,
// matching the formatting used by cli_pretty.
func formatSensor(valueType string, value int64, scale string) string {
	v := float64(value)
	switch scale {
	case "milli":
		v /= 1000
	case "micro":
		v /= 1000000
	}
	switch valueType {
	case "celsius":
		return fmt.Sprintf("%.1f\u00b0C", v)
	case "rpm":
		return fmt.Sprintf("%.0f RPM", v)
	case "volts-DC":
		return fmt.Sprintf("%.2f VDC", v)
	case "amperes":
		return fmt.Sprintf("%.2f A", v)
	case "watts":
		return fmt.Sprintf("%.2f W", v)
	default:
		return fmt.Sprintf("%.1f", v)
	}
}

// humanBytes converts bytes to a human-readable string (B, KiB, MiB, GiB, TiB).
func humanBytes(b int64) string {
	v := float64(b)
	for _, unit := range []string{"B", "KiB", "MiB", "GiB", "TiB"} {
		if v < 1024 || unit == "TiB" {
			if v == math.Trunc(v) {
				return fmt.Sprintf("%.0f %s", v, unit)
			}
			return fmt.Sprintf("%.1f %s", v, unit)
		}
		v /= 1024
	}
	return fmt.Sprintf("%.1f PiB", v)
}

// humanKiB converts KiB to a human-readable string using binary (IEC) units.
func humanKiB(kib int64) string {
	v := float64(kib)
	for _, unit := range []string{"KiB", "MiB", "GiB", "TiB"} {
		if v < 1024 || unit == "TiB" {
			if v == math.Trunc(v) {
				return fmt.Sprintf("%.0f %s", v, unit)
			}
			return fmt.Sprintf("%.1f %s", v, unit)
		}
		v /= 1024
	}
	return fmt.Sprintf("%.1f PiB", v)
}
