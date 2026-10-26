// SPDX-License-Identifier: MIT

package handlers

import (
	"context"
	"fmt"
	"html/template"
	"log"
	"net/http"
	"net/url"
	"sort"
	"strings"

	"infix/webui/internal/restconf"
)

const (
	classModem = "modem" // infix-hardware:modem
	classSIM   = "sim"   // infix-hardware:sim
)

// modemStateJSON mirrors infix-hardware:modem-state.
type modemStateJSON struct {
	Manufacturer     string     `json:"manufacturer"`
	Model            string     `json:"model"`
	HardwareRevision string     `json:"hardware-revision"`
	FirmwareVersion  string     `json:"firmware-version"`
	SerialNumber     string     `json:"serial-number"`
	USBVendorID      string     `json:"usb-vendor-id"`
	USBProductID     string     `json:"usb-product-id"`
	IMSI             string     `json:"imsi"`
	ICCID            string     `json:"iccid"`
	SelectedCarrier  string     `json:"selected-carrier"`
	State            string     `json:"state"`
	FailedReason     string     `json:"state-failed-reason"`
	PowerState       string     `json:"power-state"`
	EnabledLocks     []string   `json:"enabled-locks"`
	PhoneNumber      []string   `json:"phone-number"`
	SignalQuality    *yangInt64 `json:"signal-quality"`
	SignalRecent     *bool      `json:"signal-quality-recent"`
	SignalRSSI       *yangInt64 `json:"signal-rssi"`
	SignalRSRP       *yangInt64 `json:"signal-rsrp"`
	SignalRSRQ       string     `json:"signal-rsrq"`
	SignalSINR       string     `json:"signal-sinr"`
	LastChange       string     `json:"last-change"`
	Cellular         *struct {
		Registration string `json:"registration-state"`
		ServiceState string `json:"service-state"`
		OperatorName string `json:"operator-name"`
		OperatorID   string `json:"operator-id"`
		NetworkType  string `json:"network-type"`
	} `json:"cellular"`
	Location *struct {
		Source    string     `json:"source"`
		Latitude  string     `json:"latitude"`
		Longitude string     `json:"longitude"`
		Altitude  string     `json:"altitude"`
		CellID    *yangInt64 `json:"cell-id"`
		LAC       *yangInt64 `json:"lac"`
		TAC       *yangInt64 `json:"tac"`
		MCC       string     `json:"mcc"`
		MNC       string     `json:"mnc"`
	} `json:"location-state"`
}

// simStateJSON mirrors infix-hardware:sim-state.
type simStateJSON struct {
	Slot         *yangInt64 `json:"slot"`
	State        string     `json:"state"`
	OperatorName string     `json:"operator-name"`
}

// modemSummary is one modem as the status pages show it.
type modemSummary struct {
	Name         string
	Manufacturer string
	Model        string
	State        string
	StateClass   string // status-dot class
	Network      string // e.g. "LTE"
	Operator     string
	Roaming      bool
	HasSignal    bool
	SignalPct    int
	SignalWord   string // excellent, good, poor, bad, as in the CLI
	SignalClass  string // health-bar-fill modifier
	SignalDetail string // e.g. "RSRP -98 dBm · SINR 14.0 dB"
	SIM          *simSummary
}

// simSummary is one SIM card as the status pages show it.
type simSummary struct {
	Name       string
	Slot       string
	State      string
	StateClass string
	Operator   string
}

// networkNames are the access technologies spelled the way people say them.
var networkNames = map[string]string{
	"5gnr": "5G NR", "lte": "LTE", "umts": "UMTS", "hspa": "HSPA",
	"hspa-plus": "HSPA+", "hsdpa": "HSDPA", "hsupa": "HSUPA", "edge": "EDGE",
	"gprs": "GPRS", "gsm": "GSM", "gsm-compact": "GSM Compact",
}

func networkName(t string) string {
	if t == "" || t == "unknown" {
		return ""
	}
	if n, ok := networkNames[t]; ok {
		return n
	}
	return strings.ToUpper(t)
}

func summarizeModem(c hwComponentJSON) modemSummary {
	m := modemSummary{Name: c.Name, State: "unknown", StateClass: "status-down"}
	ms := c.ModemState
	if ms == nil {
		return m
	}
	m.Manufacturer, m.Model = ms.Manufacturer, ms.Model
	if ms.State != "" {
		m.State = ms.State
	}
	switch m.State {
	case "connected", "registered":
		m.StateClass = "status-up"
	case "failed", "disabled", "unknown":
		m.StateClass = "status-down"
	default:
		m.StateClass = "status-warn"
	}
	if cell := ms.Cellular; cell != nil {
		m.Network = networkName(cell.NetworkType)
		m.Operator = cell.OperatorName
		m.Roaming = cell.Registration == "roaming"
	}
	if ms.SignalQuality != nil && m.Network != "" {
		m.HasSignal = true
		m.SignalPct = int(*ms.SignalQuality)
		switch {
		case m.SignalPct >= 80:
			m.SignalWord = "excellent"
		case m.SignalPct >= 60:
			m.SignalWord = "good"
		case m.SignalPct >= 40:
			m.SignalWord, m.SignalClass = "poor", "is-warn"
		default:
			m.SignalWord, m.SignalClass = "bad", "is-crit"
		}
	}
	var parts []string
	if ms.SignalRSRP != nil {
		parts = append(parts, fmt.Sprintf("RSRP %d dBm", int64(*ms.SignalRSRP)))
	} else if ms.SignalRSSI != nil {
		parts = append(parts, fmt.Sprintf("RSSI %d dBm", int64(*ms.SignalRSSI)))
	}
	if ms.SignalRSRQ != "" {
		parts = append(parts, "RSRQ "+ms.SignalRSRQ+" dB")
	}
	if ms.SignalSINR != "" {
		parts = append(parts, "SINR "+ms.SignalSINR+" dB")
	}
	m.SignalDetail = strings.Join(parts, " · ")
	return m
}

func summarizeSIM(c hwComponentJSON) simSummary {
	s := simSummary{Name: c.Name, State: "unknown", StateClass: "status-down"}
	ss := c.SimState
	if ss == nil {
		return s
	}
	if ss.Slot != nil {
		s.Slot = fmt.Sprintf("%d", int64(*ss.Slot))
	}
	if ss.State != "" {
		s.State = ss.State
	}
	switch s.State {
	case "unlocked":
		s.StateClass = "status-up"
	case "pin-required", "puk-required":
		s.StateClass = "status-warn"
	}
	s.Operator = ss.OperatorName
	return s
}

// simOf is the SIM component of a modem, modemN has simN.
func simOf(modem string) string {
	return "sim" + strings.TrimPrefix(modem, "modem")
}

// infoRow is one label and value in a detail card.
type infoRow struct {
	Label string
	Value string
}

// modemBearer is one data connection of a modem, from its interface.
type modemBearer struct {
	Iface     string
	Connected bool
	Addresses []string
	Duration  string
	Failure   string
}

// modemDetailData is the template data for a modem's detail page.
type modemDetailData struct {
	PageData
	Name     string
	Error    string
	Summary  modemSummary
	Hardware []infoRow
	Status   []infoRow
	Cellular []infoRow
	SIM      []infoRow
	Location []infoRow
	Bearers  []modemBearer
}

// ModemHandler serves a modem's detail page and its actions.
type ModemHandler struct {
	Template *template.Template
	RC       *restconf.Client
}

// Detail renders GET /hardware/modem/{name}, the WebUI's
// 'show modem <name>'.
func (h *ModemHandler) Detail(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	data := modemDetailData{
		PageData: newPageData(w, r, "hardware", name),
		Name:     name,
	}
	ctx := context.WithoutCancel(r.Context())

	var hw hardwareWrapper
	if err := h.RC.Get(ctx, "/data/ietf-hardware:hardware", &hw); err != nil {
		log.Printf("restconf hardware (modem %s): %v", name, err)
		data.Error = "Could not fetch modem information"
	} else if !buildModemDetail(&data, hw.Hardware.Component) {
		data.Error = fmt.Sprintf("No modem %s", name)
	}

	var ifs interfacesWrapper
	if err := h.RC.Get(ctx, "/data/ietf-interfaces:interfaces", &ifs); err == nil {
		data.Bearers = modemBearers(name, ifs.Interfaces.Interface)
	}

	tmplName := "modem.html"
	if r.Header.Get("HX-Request") == "true" {
		tmplName = "content"
	}
	if err := h.Template.ExecuteTemplate(w, tmplName, data); err != nil {
		log.Printf("template error: %v", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
	}
}

// Restart handles POST /hardware/modem/{name}/restart.
func (h *ModemHandler) Restart(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	w.Header().Set("Content-Type", "text/html")
	path := "/data/ietf-hardware:hardware/component=" + url.PathEscape(name) + "/infix-hardware:restart"
	if err := h.RC.CallRPC(r.Context(), path, nil, nil); err != nil {
		log.Printf("modem %s restart: %v", name, err)
		fmt.Fprintf(w, `<div class="alert alert-error card-alert">Restart failed: %s</div>`,
			template.HTMLEscapeString(err.Error()))
		return
	}
	fmt.Fprint(w, `<div class="alert alert-success card-alert">&#10003; Restarting, the modem reconnects by itself</div>`)
}

// buildModemDetail fills the detail cards, false when there is no such modem.
func buildModemDetail(d *modemDetailData, comps []hwComponentJSON) bool {
	var modem, sim *hwComponentJSON
	for i := range comps {
		switch {
		case comps[i].Name == d.Name && shortClass(comps[i].Class) == classModem:
			modem = &comps[i]
		case comps[i].Name == simOf(d.Name) && shortClass(comps[i].Class) == classSIM:
			sim = &comps[i]
		}
	}
	if modem == nil {
		return false
	}
	d.Summary = summarizeModem(*modem)

	add := func(rows *[]infoRow, label, value string) {
		if value != "" {
			*rows = append(*rows, infoRow{label, value})
		}
	}
	ms := modem.ModemState
	if ms == nil {
		ms = &modemStateJSON{}
	}
	add(&d.Hardware, "Manufacturer", ms.Manufacturer)
	add(&d.Hardware, "Model", ms.Model)
	add(&d.Hardware, "Hardware Revision", ms.HardwareRevision)
	add(&d.Hardware, "Firmware Version", ms.FirmwareVersion)
	add(&d.Hardware, "Serial Number", ms.SerialNumber)
	if ms.USBVendorID != "" && ms.USBProductID != "" {
		add(&d.Hardware, "USB ID", ms.USBVendorID+":"+ms.USBProductID)
	}
	if len(modem.URI) > 0 {
		add(&d.Hardware, "URI", modem.URI[0])
	}
	add(&d.Hardware, "IMSI", ms.IMSI)
	add(&d.Hardware, "ICCID", ms.ICCID)
	add(&d.Hardware, "Phone Number", strings.Join(ms.PhoneNumber, ", "))
	add(&d.Hardware, "Carrier Profile", ms.SelectedCarrier)

	add(&d.Status, "Power State", ms.PowerState)
	add(&d.Status, "Failure", ms.FailedReason)
	add(&d.Status, "Locks", strings.Join(ms.EnabledLocks, ", "))
	add(&d.Status, "Last Update", formatLastChange(ms.LastChange))

	if cell := ms.Cellular; cell != nil {
		add(&d.Cellular, "Registration", cell.Registration)
		add(&d.Cellular, "Service State", cell.ServiceState)
		add(&d.Cellular, "Operator", cell.OperatorName)
		add(&d.Cellular, "Operator ID", cell.OperatorID)
		add(&d.Cellular, "Network Type", networkName(cell.NetworkType))
	}

	if sim != nil {
		s := summarizeSIM(*sim)
		add(&d.SIM, "Name", s.Name)
		add(&d.SIM, "Slot", s.Slot)
		add(&d.SIM, "Lock State", s.State)
		add(&d.SIM, "Operator", s.Operator)
	}

	if loc := ms.Location; loc != nil {
		add(&d.Location, "Source", loc.Source)
		if loc.Latitude != "" && loc.Longitude != "" {
			add(&d.Location, "Position", loc.Latitude+", "+loc.Longitude)
		}
		add(&d.Location, "Altitude", loc.Altitude)
		if loc.CellID != nil {
			add(&d.Location, "Cell ID", fmt.Sprintf("%d", int64(*loc.CellID)))
		}
		if loc.MCC != "" {
			add(&d.Location, "MCC / MNC", loc.MCC+" / "+loc.MNC)
		}
	}
	return true
}

// modemBearers are the interfaces that run on the modem.
func modemBearers(modem string, ifaces []ifaceJSON) []modemBearer {
	var out []modemBearer
	for _, iface := range ifaces {
		w := iface.Wwan
		if w == nil || w.Modem != modem {
			continue
		}
		b := modemBearer{Iface: iface.Name}
		if bs := w.BearerState; bs != nil {
			b.Connected = bs.Connected
			if bs.IPv4 != "" {
				b.Addresses = append(b.Addresses, fmt.Sprintf("%s/%d", bs.IPv4, int64(bs.IPv4Prefix)))
			}
			if bs.IPv6 != "" {
				b.Addresses = append(b.Addresses, fmt.Sprintf("%s/%d", bs.IPv6, int64(bs.IPv6Prefix)))
			}
			if bs.Connected && bs.TotalDuration > 0 {
				b.Duration = formatUptime(uint32(bs.TotalDuration))
			}
			if !bs.Connected {
				b.Failure = bs.FailedReason
			}
		}
		out = append(out, b)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Iface < out[j].Iface })
	return out
}

// wwanJSON mirrors the operational infix-interfaces:wwan augment.
type wwanJSON struct {
	Modem       string `json:"modem"`
	BearerState *struct {
		Connected     bool      `json:"connected"`
		FailedReason  string    `json:"connection-failed-reason"`
		IPv4          string    `json:"ipv4-address"`
		IPv4Prefix    yangInt64 `json:"ipv4-prefix-length"`
		IPv6          string    `json:"ipv6-address"`
		IPv6Prefix    yangInt64 `json:"ipv6-prefix-length"`
		TotalDuration yangInt64 `json:"total-duration"`
	} `json:"bearer-state"`
}
