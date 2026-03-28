// SPDX-License-Identifier: MIT

package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"html/template"
	"log"
	"math"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/kernelkit/webui/internal/restconf"
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
	Firmware     string
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
	WiFiRadios   []wifiEntry
	SensorGroups []sensorGroup
	Error        string
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

type sensorGroup struct {
	Parent  string
	Sensors []sensorEntry
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
}

// DashboardHandler serves the main dashboard page.
type DashboardHandler struct {
	Template *template.Template
	RC       *restconf.Client
}

// Index renders the dashboard (GET /).
func (h *DashboardHandler) Index(w http.ResponseWriter, r *http.Request) {
	data := dashboardData{
		PageData: newPageData(r, "dashboard", "Dashboard"),
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
		stateErr, hwErr, confErr error
		wg                       sync.WaitGroup
	)

	wg.Add(3)
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
	wg.Wait()

	if stateErr != nil {
		log.Printf("restconf system-state: %v", stateErr)
		data.Error = "Could not fetch system information"
	} else {
		ss := state.SystemState
		data.OSName = ss.Platform.OSName
		data.OSVersion = ss.Platform.OSVersion
		data.Machine = ss.Platform.Machine
		if data.Machine == "arm64" {
			data.Machine = "aarch64"
		}
		data.Firmware = firmwareVersion(ss.Software)
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
			pct := 0
			if size > 0 {
				pct = int(float64(used) / float64(size) * 100)
			}
			diskClass := ""
			switch {
			case pct >= 90:
				diskClass = "is-crit"
			case pct >= 70:
				diskClass = "is-warn"
			}
			data.Disks = append(data.Disks, diskEntry{
				Mount:     fs.MountPoint,
				Size:      humanKiB(size),
				Available: humanKiB(int64(fs.Available)),
				Percent:   pct,
				Class:     diskClass,
			})
		}
	}

	if hwErr != nil {
		log.Printf("restconf hardware: %v", hwErr)
	} else {
		sensorMap := make(map[string][]sensorEntry)
		var sensorParents []string

		for _, c := range hw.Hardware.Component {
			class := shortClass(c.Class)
			if class == "chassis" {
				data.Board = boardInfo{
					Model:        c.ModelName,
					Manufacturer: c.MfgName,
					SerialNum:    c.SerialNum,
					HardwareRev:  c.HardwareRev,
					BaseMAC:      c.PhysAddress,
				}
			}
			if c.SensorData != nil && c.SensorData.OperStatus == "ok" {
				entry := sensorEntry{
					Name:  c.Name,
					Value: formatSensor(c.SensorData.ValueType, int64(c.SensorData.Value), c.SensorData.ValueScale),
					Type:  c.SensorData.ValueType,
				}
				p := c.Parent
				if _, ok := sensorMap[p]; !ok {
					sensorParents = append(sensorParents, p)
				}
				sensorMap[p] = append(sensorMap[p], entry)
			}
			if c.WiFiRadio != nil {
				var stds []string
				var ht, vht, he bool
				var bandNames []string
				for _, b := range c.WiFiRadio.Bands {
					if b.HTCapable {
						ht = true
					}
					if b.VHTCapable {
						vht = true
					}
					if b.HECapable {
						he = true
					}
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
				if ht {
					stds = append(stds, "11n")
				}
				if vht {
					stds = append(stds, "11ac")
				}
				if he {
					stds = append(stds, "11ax")
				}
				maxAP := 0
				if c.WiFiRadio.MaxInterfaces != nil {
					maxAP = c.WiFiRadio.MaxInterfaces.AP
				}
				data.WiFiRadios = append(data.WiFiRadios, wifiEntry{
					Name:         c.Name,
					Manufacturer: c.MfgName,
					Bands:        strings.Join(bandNames, ", "),
					Standards:    strings.Join(stds, "/"),
					MaxAP:        maxAP,
				})
			}
		}
		for _, p := range sensorParents {
			data.SensorGroups = append(data.SensorGroups, sensorGroup{
				Parent:  p,
				Sensors: sensorMap[p],
			})
		}
	}

	if confErr != nil {
		log.Printf("restconf system config: %v", confErr)
	} else {
		data.Hostname = sysConf.System.Hostname
		data.Contact = sysConf.System.Contact
		data.Location = sysConf.System.Location
	}

	tmplName := "dashboard.html"
	if r.Header.Get("HX-Request") == "true" {
		tmplName = "content"
	}
	if err := h.Template.ExecuteTemplate(w, tmplName, data); err != nil {
		log.Printf("template error: %v", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
	}
}

// firmwareVersion returns the version string for the booted software slot.
func firmwareVersion(sw software) string {
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
