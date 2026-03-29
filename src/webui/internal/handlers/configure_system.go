// SPDX-License-Identifier: MIT

package handlers

import (
	"encoding/json"
	"errors"
	"html/template"
	"log"
	"net/http"
	"strconv"
	"strings"

	"github.com/kernelkit/webui/internal/restconf"
)

// ─── RESTCONF JSON types (candidate datastore) ────────────────────────────────

type cfgSystemWrapper struct {
	System cfgSystemJSON `json:"ietf-system:system"`
}

type cfgSystemJSON struct {
	Contact    string       `json:"contact"`
	Hostname   string       `json:"hostname"`
	Location   string       `json:"location"`
	Clock      cfgClockJSON `json:"clock"`
	NTP        cfgNTPJSON   `json:"ntp"`
	DNS        cfgDNSJSON   `json:"dns-resolver"`
	MotdBanner []byte       `json:"infix-system:motd-banner,omitempty"`
	TextEditor string       `json:"infix-system:text-editor,omitempty"`
}

type cfgClockJSON struct {
	TimezoneName string `json:"timezone-name"`
}

type cfgNTPJSON struct {
	Enabled bool              `json:"enabled"`
	Servers []cfgNTPServerJSON `json:"server"`
}

type cfgNTPServerJSON struct {
	Name   string        `json:"name"`
	UDP    cfgNTPUDPJSON `json:"udp"`
	Prefer bool          `json:"prefer"`
}

type cfgNTPUDPJSON struct {
	Address string `json:"address"`
	Port    uint16 `json:"port,omitempty"`
}

type cfgDNSJSON struct {
	Search  []string           `json:"search"`
	Servers []cfgDNSServerJSON `json:"server"`
}

type cfgDNSServerJSON struct {
	Name       string              `json:"name"`
	UDPAndTCP  cfgDNSAddrJSON      `json:"udp-and-tcp"`
}

type cfgDNSAddrJSON struct {
	Address string `json:"address"`
	Port    uint16 `json:"port,omitempty"`
}

// ─── Template data ────────────────────────────────────────────────────────────

type cfgSystemPageData struct {
	PageData
	Error      string
	Hostname   string
	Contact    string
	Location   string
	Timezone   string
	NTP        cfgNTPJSON
	DNS        cfgDNSJSON
	MotdBanner string // decoded from YANG binary
	TextEditor string // e.g. "infix-system:emacs"
}

// ─── Handler ─────────────────────────────────────────────────────────────────

// ConfigureSystemHandler serves the Configure > System page.
type ConfigureSystemHandler struct {
	Template *template.Template
	RC       restconf.Fetcher
}

const candidatePath = "/ds/ietf-datastores:candidate"

// Overview renders the Configure > System page reading from the candidate datastore.
// GET /configure/system
func (h *ConfigureSystemHandler) Overview(w http.ResponseWriter, r *http.Request) {
	data := cfgSystemPageData{
		PageData: newPageData(r, "configure-system", "Configure: System"),
	}

	var raw cfgSystemWrapper
	if err := h.RC.Get(r.Context(), candidatePath+"/ietf-system:system", &raw); err != nil {
		var rcErr *restconf.Error
		if errors.As(err, &rcErr) && rcErr.StatusCode == http.StatusNotFound {
			// Candidate not initialised — read from running as fallback.
			if fallErr := h.RC.Get(r.Context(), "/data/ietf-system:system", &raw); fallErr != nil {
				var rcFall *restconf.Error
				if !errors.As(fallErr, &rcFall) || rcFall.StatusCode != http.StatusNotFound {
					log.Printf("configure system (running fallback): %v", fallErr)
					data.Error = "Could not read system configuration"
				}
			}
		} else {
			log.Printf("configure system: %v", err)
			data.Error = "Could not read candidate configuration"
		}
	}
	if data.Error == "" {
		s := raw.System
		data.Hostname = s.Hostname
		data.Contact = s.Contact
		data.Location = s.Location
		data.Timezone = s.Clock.TimezoneName
		data.NTP = s.NTP
		data.DNS = s.DNS
		data.MotdBanner = string(s.MotdBanner)
		data.TextEditor = s.TextEditor
	}

	tmplName := "configure-system.html"
	if r.Header.Get("HX-Request") == "true" {
		tmplName = "content"
	}
	if err := h.Template.ExecuteTemplate(w, tmplName, data); err != nil {
		log.Printf("template error: %v", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
	}
}

// SaveIdentity patches hostname / contact / location to the candidate datastore.
// POST /configure/system/identity
func (h *ConfigureSystemHandler) SaveIdentity(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}

	body := map[string]any{
		"ietf-system:system": map[string]any{
			"hostname": r.FormValue("hostname"),
			"contact":  r.FormValue("contact"),
			"location": r.FormValue("location"),
		},
	}
	if err := h.RC.Patch(r.Context(), candidatePath+"/ietf-system:system", body); err != nil {
		log.Printf("configure system identity: %v", err)
		renderSaveError(w, err)
		return
	}
	renderSaved(w, "Identity saved")
}

// SaveClock patches the timezone to the candidate datastore.
// POST /configure/system/clock
func (h *ConfigureSystemHandler) SaveClock(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}

	body := map[string]any{
		"ietf-system:system": map[string]any{
			"clock": map[string]any{
				"timezone-name": r.FormValue("timezone"),
			},
		},
	}
	if err := h.RC.Patch(r.Context(), candidatePath+"/ietf-system:system", body); err != nil {
		log.Printf("configure system clock: %v", err)
		renderSaveError(w, err)
		return
	}
	renderSaved(w, "Clock saved")
}

// SaveNTP replaces the NTP server list in the candidate datastore.
// PUT /configure/system/ntp
func (h *ConfigureSystemHandler) SaveNTP(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}

	servers := parseNTPServers(r)
	ntp := map[string]any{"enabled": true}
	if len(servers) > 0 {
		ntp["server"] = servers
	}
	body := map[string]any{"ietf-system:ntp": ntp}
	if err := h.RC.Put(r.Context(), candidatePath+"/ietf-system:system/ntp", body); err != nil {
		log.Printf("configure system ntp: %v", err)
		renderSaveError(w, err)
		return
	}
	renderSaved(w, "NTP saved")
}

// SaveDNS replaces the DNS resolver config in the candidate datastore.
// PUT /configure/system/dns
func (h *ConfigureSystemHandler) SaveDNS(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}

	search := parseSearchList(r)
	servers := parseDNSServers(r)

	// Omit empty lists entirely — sending null for a YANG list is invalid.
	dnsResolver := map[string]any{}
	if len(search) > 0 {
		dnsResolver["search"] = search
	}
	if len(servers) > 0 {
		dnsResolver["server"] = servers
	}
	body := map[string]any{
		"ietf-system:dns-resolver": dnsResolver,
	}
	if err := h.RC.Put(r.Context(), candidatePath+"/ietf-system:system/dns-resolver", body); err != nil {
		log.Printf("configure system dns: %v", err)
		renderSaveError(w, err)
		return
	}
	renderSaved(w, "DNS saved")
}

// ─── Form parsing helpers ─────────────────────────────────────────────────────

// parseNTPServers extracts NTP server entries from form values.
// Fields: ntp_name_N, ntp_addr_N, ntp_port_N, ntp_prefer_N (checkbox).
func parseNTPServers(r *http.Request) []cfgNTPServerJSON {
	var servers []cfgNTPServerJSON
	for i := 0; ; i++ {
		name := strings.TrimSpace(r.FormValue("ntp_name_" + strconv.Itoa(i)))
		if name == "" {
			break
		}
		addr := strings.TrimSpace(r.FormValue("ntp_addr_" + strconv.Itoa(i)))
		port, _ := strconv.ParseUint(r.FormValue("ntp_port_"+strconv.Itoa(i)), 10, 16)
		prefer := r.FormValue("ntp_prefer_"+strconv.Itoa(i)) == "on"
		srv := cfgNTPServerJSON{
			Name:   name,
			UDP:    cfgNTPUDPJSON{Address: addr, Port: uint16(port)},
			Prefer: prefer,
		}
		servers = append(servers, srv)
	}
	return servers
}

// parseSearchList extracts DNS search domains from form values.
// Fields: dns_search_N (one per domain).
func parseSearchList(r *http.Request) []string {
	var search []string
	for i := 0; ; i++ {
		v := strings.TrimSpace(r.FormValue("dns_search_" + strconv.Itoa(i)))
		if v == "" {
			break
		}
		search = append(search, v)
	}
	return search
}

// parseDNSServers extracts DNS server entries from form values.
// Fields: dns_name_N, dns_addr_N, dns_port_N.
func parseDNSServers(r *http.Request) []cfgDNSServerJSON {
	var servers []cfgDNSServerJSON
	for i := 0; ; i++ {
		name := strings.TrimSpace(r.FormValue("dns_name_" + strconv.Itoa(i)))
		if name == "" {
			break
		}
		addr := strings.TrimSpace(r.FormValue("dns_addr_" + strconv.Itoa(i)))
		port, _ := strconv.ParseUint(r.FormValue("dns_port_"+strconv.Itoa(i)), 10, 16)
		srv := cfgDNSServerJSON{
			Name:      name,
			UDPAndTCP: cfgDNSAddrJSON{Address: addr, Port: uint16(port)},
		}
		servers = append(servers, srv)
	}
	return servers
}

// SavePreferences patches infix-system augmented fields (motd-banner, text-editor).
// POST /configure/system/preferences
func (h *ConfigureSystemHandler) SavePreferences(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}

	sysPatch := map[string]any{}
	if motd := r.FormValue("motd_banner"); motd != "" {
		sysPatch["infix-system:motd-banner"] = []byte(motd)
	}
	if editor := r.FormValue("text_editor"); editor != "" {
		sysPatch["infix-system:text-editor"] = editor
	}
	if len(sysPatch) == 0 {
		renderSaved(w, "Preferences saved")
		return
	}

	body := map[string]any{"ietf-system:system": sysPatch}
	if err := h.RC.Patch(r.Context(), candidatePath+"/ietf-system:system", body); err != nil {
		log.Printf("configure system preferences: %v", err)
		renderSaveError(w, err)
		return
	}
	renderSaved(w, "Preferences saved")
}

// ─── Response helpers ─────────────────────────────────────────────────────────

// renderSaved writes a success indicator for HTMX to swap into the Save button.
func renderSaved(w http.ResponseWriter, msg string) {
	w.Header().Set("Content-Type", "text/html")
	w.Header().Set("HX-Trigger", `{"cfgSaved":"`+msg+`"}`)
	w.WriteHeader(http.StatusOK)
}

// renderSaveError writes an inline error message for HTMX to swap.
func renderSaveError(w http.ResponseWriter, err error) {
	msg := "Save failed"
	if re, ok := err.(*restconf.Error); ok && re.Message != "" {
		msg = re.Message
	}
	w.Header().Set("Content-Type", "text/html")
	w.WriteHeader(http.StatusUnprocessableEntity)
	// Encode the message safely for HTML output.
	b, _ := json.Marshal(msg)
	_ = b // used below via template-escaped string
	w.Write([]byte(`<span class="cfg-save-error">` + template.HTMLEscapeString(msg) + `</span>`))
}
