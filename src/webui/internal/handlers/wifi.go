// SPDX-License-Identifier: MIT

package handlers

import (
	"context"
	"fmt"
	"html/template"
	"log"
	"net/http"
	"sync"

	"github.com/kernelkit/webui/internal/restconf"
)

// wifiRadioHWJSON extends the hardware component wifi-radio container with
// operational fields from infix-hardware YANG that are not in wifiRadioJSON.
// wifiRadioJSON (defined in interfaces.go) only covers survey data; this
// struct captures the full operational state returned by RESTCONF.
// wifiMaxIfJSON maps the max-interfaces container from infix-hardware YANG.
type wifiMaxIfJSON struct {
	AP      int `json:"ap"`
	Station int `json:"station"`
	Monitor int `json:"monitor"`
}

type wifiRadioHWJSON struct {
	Channel       interface{}     `json:"channel"` // uint16 or "auto"
	Band          string          `json:"band"`
	Frequency     int             `json:"frequency"` // MHz, operational
	Noise         int             `json:"noise"`     // dBm, operational
	Driver        string          `json:"driver"`
	Bands         []wifiBandJSON  `json:"bands"`
	MaxInterfaces *wifiMaxIfJSON  `json:"max-interfaces"`
	Survey        *wifiSurveyJSON `json:"survey"`
}

type wifiBandJSON struct {
	Band       string `json:"band"`
	Name       string `json:"name"`
	HTCapable  bool   `json:"ht-capable"`
	VHTCapable bool   `json:"vht-capable"`
	HECapable  bool   `json:"he-capable"`
}

// hwComponentWiFiJSON is a minimal hardware component used for the wifi page.
// We reuse hardwareWrapper but need to decode wifi-radio with the richer struct.
type hwComponentWiFiJSON struct {
	Name      string           `json:"name"`
	Class     string           `json:"class"`
	MfgName   string           `json:"mfg-name"`
	WiFiRadio *wifiRadioHWJSON `json:"infix-hardware:wifi-radio"`
}

type hardwareWiFiWrapper struct {
	Hardware struct {
		Component []hwComponentWiFiJSON `json:"component"`
	} `json:"ietf-hardware:hardware"`
}

// WiFiRadio is the template data for a single physical radio.
type WiFiRadio struct {
	Name         string
	Channel      string
	Band         string
	Frequency    int
	Noise        int
	Driver       string
	Manufacturer string
	Standards    string
	MaxAP        string
	HTCapable    bool
	VHTCapable   bool
	HECapable    bool
	Bands        []WiFiBand
	SurveySVG    template.HTML
	Interfaces   []WiFiInterface
}

type WiFiBand struct {
	Band       string
	Name       string
	HTCapable  bool
	VHTCapable bool
	HECapable  bool
}

// ChannelSurvey holds processed survey data for one channel.
type ChannelSurvey struct {
	Frequency  int
	Channel    int
	InUse      bool
	Noise      int
	ActiveTime int64
	BusyTime   int64
	UtilPct    int // BusyTime/ActiveTime * 100
}

// WiFiInterface is the template data for a virtual WiFi interface.
type WiFiInterface struct {
	Name       string
	Mode       string // "ap" or "station"
	SSID       string
	OperStatus string
	StatusUp   bool
	// AP mode
	APClients []WiFiClient
	// Station mode
	Signal      string
	SignalCSS   string
	RxSpeed     string
	TxSpeed     string
	ScanResults []WiFiScan
}

// WiFiClient is the template data for a connected station client.
type WiFiClient struct {
	MAC       string
	Signal    string
	SignalCSS string
	ConnTime  string
	RxBytes   string
	TxBytes   string
	RxSpeed   string
	TxSpeed   string
}

// WiFiScan is the template data for a scan result entry.
type WiFiScan struct {
	SSID       string
	BSSID      string
	Signal     string
	SignalCSS  string
	Channel    string
	Encryption string
}

// wifiData is the top-level template data for the WiFi page.
type wifiData struct {
	CsrfToken    string
	PageTitle    string
	ActivePage   string
	Capabilities *Capabilities
	Radios       []WiFiRadio
	Error        string
}

// WiFiHandler serves the WiFi status page.
type WiFiHandler struct {
	Template *template.Template
	RC       *restconf.Client
}

// Overview renders the WiFi page (GET /wifi).
func (h *WiFiHandler) Overview(w http.ResponseWriter, r *http.Request) {
	data := wifiData{
		CsrfToken:    csrfToken(r.Context()),
		PageTitle:    "WiFi",
		ActivePage:   "wifi",
		Capabilities: CapabilitiesFromContext(r.Context()),
	}

	// Detach from the request context so that RESTCONF calls survive
	// browser connection resets.
	ctx := context.WithoutCancel(r.Context())

	var (
		hw           hardwareWiFiWrapper
		ifaces       interfacesWrapper
		hwErr, ifErr error
		wg           sync.WaitGroup
	)

	wg.Add(2)
	go func() {
		defer wg.Done()
		hwErr = h.RC.Get(ctx, "/data/ietf-hardware:hardware", &hw)
	}()
	go func() {
		defer wg.Done()
		ifErr = h.RC.Get(ctx, "/data/ietf-interfaces:interfaces", &ifaces)
	}()
	wg.Wait()

	if hwErr != nil {
		log.Printf("wifi: restconf hardware: %v", hwErr)
		data.Error = "Could not fetch hardware information"
	}
	if ifErr != nil {
		log.Printf("wifi: restconf interfaces: %v", ifErr)
		if data.Error == "" {
			data.Error = "Could not fetch interface information"
		}
	}

	if hwErr == nil && ifErr == nil {
		data.Radios = buildWiFiRadios(hw.Hardware.Component, ifaces.Interfaces.Interface)
	}

	tmplName := "wifi.html"
	if r.Header.Get("HX-Request") == "true" {
		tmplName = "content"
	}
	if err := h.Template.ExecuteTemplate(w, tmplName, data); err != nil {
		log.Printf("wifi: template error: %v", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
	}
}

// buildWiFiRadios assembles the WiFiRadio slice from hardware components
// and interface data, matching interfaces to their radio by name.
func buildWiFiRadios(components []hwComponentWiFiJSON, ifaces []ifaceJSON) []WiFiRadio {
	var radios []WiFiRadio

	for _, c := range components {
		if c.WiFiRadio == nil {
			continue
		}
		r := c.WiFiRadio

		radio := WiFiRadio{
			Name:         c.Name,
			Band:         r.Band,
			Frequency:    r.Frequency,
			Noise:        r.Noise,
			Driver:       r.Driver,
			Channel:      wifiChannelString(r.Channel),
			Manufacturer: c.MfgName,
		}

		// Capability flags: check per-band capabilities; if any band supports
		// HT/VHT/HE, mark the radio as capable.
		var bandNames []string
		for _, b := range r.Bands {
			name := b.Name
			if name == "" {
				name = b.Band
			}
			radio.Bands = append(radio.Bands, WiFiBand{
				Band:       b.Band,
				Name:       name,
				HTCapable:  b.HTCapable,
				VHTCapable: b.VHTCapable,
				HECapable:  b.HECapable,
			})
			if b.Name != "" {
				bandNames = append(bandNames, b.Name)
			}
			if b.HTCapable {
				radio.HTCapable = true
			}
			if b.VHTCapable {
				radio.VHTCapable = true
			}
			if b.HECapable {
				radio.HECapable = true
			}
		}

		// Derive standards string from aggregated capabilities (matches CLI).
		var standards []string
		if radio.HTCapable {
			standards = append(standards, "11n")
		}
		if radio.VHTCapable {
			standards = append(standards, "11ac")
		}
		if radio.HECapable {
			standards = append(standards, "11ax")
		}
		if len(standards) > 0 {
			radio.Standards = joinStrings(standards, "/")
		}

		// Max AP count from max-interfaces container.
		if r.MaxInterfaces != nil && r.MaxInterfaces.AP > 0 {
			radio.MaxAP = fmt.Sprintf("%d", r.MaxInterfaces.AP)
		}

		// Generate channel survey SVG if survey data exists.
		if r.Survey != nil && len(r.Survey.Channel) > 0 {
			radio.SurveySVG = renderSurveySVG(r.Survey.Channel)
		}

		// Attach wifi interfaces that reference this radio.
		radio.Interfaces = buildWiFiInterfaces(c.Name, ifaces)

		radios = append(radios, radio)
	}

	return radios
}

// buildWiFiInterfaces returns the WiFiInterface entries for all virtual
// interfaces that reference the given radio name.
func buildWiFiInterfaces(radioName string, ifaces []ifaceJSON) []WiFiInterface {
	var result []WiFiInterface

	for _, iface := range ifaces {
		if iface.WiFi == nil || iface.WiFi.Radio != radioName {
			continue
		}

		wi := WiFiInterface{
			Name:       iface.Name,
			OperStatus: iface.OperStatus,
			StatusUp:   iface.OperStatus == "up",
		}

		if ap := iface.WiFi.AccessPoint; ap != nil {
			wi.Mode = "ap"
			wi.SSID = ap.SSID
			for _, s := range ap.Stations.Station {
				wi.APClients = append(wi.APClients, buildWiFiClient(s))
			}
		} else if st := iface.WiFi.Station; st != nil {
			wi.Mode = "station"
			wi.SSID = st.SSID
			if st.SignalStrength != nil {
				sig := *st.SignalStrength
				wi.Signal = fmt.Sprintf("%d dBm", sig)
				wi.SignalCSS = wifiSignalCSS(sig)
			}
			if st.RxSpeed > 0 {
				wi.RxSpeed = fmt.Sprintf("%.1f Mbps", float64(st.RxSpeed)/10)
			}
			if st.TxSpeed > 0 {
				wi.TxSpeed = fmt.Sprintf("%.1f Mbps", float64(st.TxSpeed)/10)
			}
			for _, sr := range st.ScanResults {
				wi.ScanResults = append(wi.ScanResults, buildWiFiScan(sr))
			}
		}

		result = append(result, wi)
	}

	return result
}

// buildWiFiClient converts a wifiStaJSON to a WiFiClient template entry.
func buildWiFiClient(s wifiStaJSON) WiFiClient {
	c := WiFiClient{
		MAC:      s.MACAddress,
		ConnTime: formatDuration(int64(s.ConnectedTime)),
		RxBytes:  humanBytes(int64(s.RxBytes)),
		TxBytes:  humanBytes(int64(s.TxBytes)),
		RxSpeed:  fmt.Sprintf("%.1f Mbps", float64(s.RxSpeed)/10),
		TxSpeed:  fmt.Sprintf("%.1f Mbps", float64(s.TxSpeed)/10),
	}
	if s.SignalStrength != nil {
		sig := *s.SignalStrength
		c.Signal = fmt.Sprintf("%d dBm", sig)
		c.SignalCSS = wifiSignalCSS(sig)
	}
	return c
}

// buildWiFiScan converts a wifiScanResultJSON to a WiFiScan template entry.
func buildWiFiScan(sr wifiScanResultJSON) WiFiScan {
	enc := "Open"
	if len(sr.Encryption) > 0 {
		parts := make([]string, 0, len(sr.Encryption))
		for _, e := range sr.Encryption {
			parts = append(parts, e)
		}
		enc = joinStrings(parts, ", ")
	}
	s := WiFiScan{
		SSID:       sr.SSID,
		BSSID:      sr.BSSID,
		Channel:    fmt.Sprintf("%d", sr.Channel),
		Encryption: enc,
	}
	if sr.SignalStrength != nil {
		sig := *sr.SignalStrength
		s.Signal = fmt.Sprintf("%d dBm", sig)
		s.SignalCSS = wifiSignalCSS(sig)
	}
	return s
}

// wifiSignalCSS returns a CSS class based on signal strength in dBm.
func wifiSignalCSS(sig int) string {
	switch {
	case sig >= -50:
		return "signal-excellent"
	case sig >= -60:
		return "signal-good"
	case sig >= -70:
		return "signal-ok"
	default:
		return "signal-poor"
	}
}

// wifiChannelString converts the YANG channel union value to a display string.
// The JSON may arrive as a float64 (number) or string ("auto").
func wifiChannelString(v interface{}) string {
	if v == nil {
		return ""
	}
	switch val := v.(type) {
	case string:
		return val
	case float64:
		return fmt.Sprintf("%d", int(val))
	case int:
		return fmt.Sprintf("%d", val)
	default:
		return fmt.Sprintf("%v", val)
	}
}

// joinStrings joins a slice of strings with a separator.
func joinStrings(parts []string, sep string) string {
	result := ""
	for i, p := range parts {
		if i > 0 {
			result += sep
		}
		result += p
	}
	return result
}
