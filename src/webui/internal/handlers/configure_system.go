// SPDX-License-Identifier: MIT

package handlers

import (
	"encoding/json"
	"fmt"
	"html/template"
	"log"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"

	"infix/webui/internal/restconf"
	"infix/webui/internal/schema"
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
	Name            string        `json:"name"`
	UDP             cfgNTPUDPJSON `json:"udp"`
	AssociationType string        `json:"association-type,omitempty"`
	IBurst          bool          `json:"iburst,omitempty"`
	Prefer          bool          `json:"prefer,omitempty"`
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
	Loading         bool   // true while YANG schema is still downloading
	Error           string
	Hostname        string
	Contact         string
	Location        string
	Timezone        string
	CurrentDatetime string // device clock from system-state, empty when unavailable
	MotdBanner      string // decoded from YANG binary
	TextEditor      string // e.g. "infix-system:emacs"

	// Schema-enriched fields — only populated when Loading is false.
	TextEditorOptions []schema.IdentityOption
	TimezoneOptions   []string          // bare timezone names for select
	Desc              map[string]string // leaf name → YANG description
}

type cfgNTPPageData struct {
	PageData
	Error      string
	NTP        cfgNTPJSON
	AssocTypes []string          // association-type enum: server / pool / peer
	Desc       map[string]string // leaf name → YANG description (field-info ⓘ)
}

type cfgDNSPageData struct {
	PageData
	Error string
	DNS   cfgDNSJSON
	Desc  map[string]string // leaf name → YANG description (field-info ⓘ)
}

// ─── Handler ─────────────────────────────────────────────────────────────────

// ConfigureSystemHandler serves the Configure > General, NTP Client, and
// DNS Client pages, all of which share the same candidate-datastore source.
type ConfigureSystemHandler struct {
	Template    *template.Template
	NTPTemplate *template.Template
	DNSTemplate *template.Template
	RC          restconf.Fetcher
	Schema      *schema.Cache
}

const candidatePath = "/ds/ietf-datastores:candidate"

// loadSystem reads /ietf-system:system from the candidate datastore, falling
// back to running when candidate is uninitialised. The returned errMsg is
// non-empty only for real errors that should surface to the user.
func (h *ConfigureSystemHandler) loadSystem(r *http.Request) (cfgSystemJSON, string) {
	var raw cfgSystemWrapper
	if err := h.RC.Get(r.Context(), candidatePath+"/ietf-system:system", &raw); err != nil {
		if !restconf.IsNotFound(err) {
			log.Printf("configure system: %v", err)
			return cfgSystemJSON{}, "Could not read candidate configuration"
		}
		if fallErr := h.RC.Get(r.Context(), "/data/ietf-system:system", &raw); fallErr != nil && !restconf.IsNotFound(fallErr) {
			log.Printf("configure system (running fallback): %v", fallErr)
			return cfgSystemJSON{}, "Could not read system configuration"
		}
	}
	return raw.System, ""
}

// render swaps the root template for "content" on HTMX requests so a partial
// reply skips the base shell.  pageName is the same key passed to newPageData
// (e.g. "configure-ntp") — the corresponding template file is "<pageName>.html".
func (h *ConfigureSystemHandler) render(w http.ResponseWriter, r *http.Request, tmpl *template.Template, pageName string, data any) {
	tmplName := pageName + ".html"
	if r.Header.Get("HX-Request") == "true" {
		tmplName = "content"
	}
	if err := tmpl.ExecuteTemplate(w, tmplName, data); err != nil {
		log.Printf("template error: %v", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
	}
}

// Overview renders the Configure > General page (identity, clock, preferences).
// GET /configure/system
func (h *ConfigureSystemHandler) Overview(w http.ResponseWriter, r *http.Request) {
	data := cfgSystemPageData{
		PageData: newPageData(w, r, "configure-system", "General"),
	}

	// Fetch candidate config and operational clock in parallel — they hit
	// different RESTCONF resources and the round-trips are independent.
	var (
		s         cfgSystemJSON
		errMsg    string
		clockResp struct {
			SystemState struct {
				Clock struct {
					CurrentDatetime string `json:"current-datetime"`
				} `json:"clock"`
			} `json:"ietf-system:system-state"`
		}
		clockErr error
		wg       sync.WaitGroup
	)
	wg.Add(2)
	go func() { defer wg.Done(); s, errMsg = h.loadSystem(r) }()
	go func() {
		defer wg.Done()
		clockErr = h.RC.Get(r.Context(), "/data/ietf-system:system-state/clock", &clockResp)
	}()
	wg.Wait()

	data.Error = errMsg
	if errMsg == "" {
		data.Hostname = s.Hostname
		data.Contact = s.Contact
		data.Location = s.Location
		data.Timezone = s.Clock.TimezoneName
		data.MotdBanner = string(s.MotdBanner)
		data.TextEditor = s.TextEditor
	}

	// Operational clock for the Date & Time card.  Best-effort; the
	// template renders "Device clock unavailable" when CurrentDatetime
	// stays empty.  Truncate the RFC 3339 offset; the Timezone row
	// below gives the user the zone context.
	if clockErr == nil {
		dt := clockResp.SystemState.Clock.CurrentDatetime
		if len(dt) > 19 {
			dt = dt[:19]
		}
		data.CurrentDatetime = dt
	}

	mgr := h.Schema.Manager()
	data.Loading = mgr == nil
	if mgr != nil {
		const sys = "/ietf-system:system"
		data.TextEditorOptions = schema.OptionsFor(mgr, sys+"/infix-system:text-editor")
		if data.TextEditor == "" {
			for _, opt := range data.TextEditorOptions {
				if opt.IsDefault {
					data.TextEditor = opt.Value
					break
				}
			}
		}
		data.Desc = map[string]string{
			"hostname":    schema.DescriptionOf(mgr, sys+"/hostname"),
			"contact":     schema.DescriptionOf(mgr, sys+"/contact"),
			"location":    schema.DescriptionOf(mgr, sys+"/location"),
			"timezone":    schema.DescriptionOf(mgr, sys+"/clock/timezone-name"),
			"text-editor": schema.DescriptionOf(mgr, sys+"/infix-system:text-editor"),
			"motd-banner": schema.DescriptionOf(mgr, sys+"/infix-system:motd-banner"),
		}
		for _, opt := range schema.OptionsFor(mgr, sys+"/clock/timezone-name") {
			data.TimezoneOptions = append(data.TimezoneOptions, schema.StripModulePrefix(opt.Value))
		}
	}

	h.render(w, r, h.Template, "configure-system", data)
}

// OverviewNTP renders the Configure > NTP Client page.
// GET /configure/ntp
func (h *ConfigureSystemHandler) OverviewNTP(w http.ResponseWriter, r *http.Request) {
	data := cfgNTPPageData{
		PageData: newPageData(w, r, "configure-ntp", "NTP Client"),
	}
	data.AssocTypes = []string{"server", "pool", "peer"}
	if mgr := h.Schema.Manager(); mgr != nil {
		srv := "/ietf-system:system/ntp/server"
		data.Desc = map[string]string{
			"name":    schema.DescriptionOf(mgr, srv+"/name"),
			"address": schema.DescriptionOf(mgr, srv+"/udp/address"),
			"port":    schema.DescriptionOf(mgr, srv+"/udp/port"),
			"assoc":   schema.DescriptionOf(mgr, srv+"/association-type"),
			"iburst":  schema.DescriptionOf(mgr, srv+"/iburst"),
			"prefer":  schema.DescriptionOf(mgr, srv+"/prefer"),
		}
	}
	s, errMsg := h.loadSystem(r)
	data.Error = errMsg
	if errMsg == "" {
		data.NTP = s.NTP
	}
	h.render(w, r, h.NTPTemplate, "configure-ntp", data)
}

// OverviewDNS renders the Configure > DNS Client page.
// GET /configure/dns
func (h *ConfigureSystemHandler) OverviewDNS(w http.ResponseWriter, r *http.Request) {
	data := cfgDNSPageData{
		PageData: newPageData(w, r, "configure-dns", "DNS Client"),
	}
	if mgr := h.Schema.Manager(); mgr != nil {
		dr := "/ietf-system:system/dns-resolver"
		data.Desc = map[string]string{
			"search":  schema.DescriptionOf(mgr, dr+"/search"),
			"name":    schema.DescriptionOf(mgr, dr+"/server/name"),
			"address": schema.DescriptionOf(mgr, dr+"/server/udp-and-tcp/address"),
			"port":    schema.DescriptionOf(mgr, dr+"/server/udp-and-tcp/port"),
		}
	}
	s, errMsg := h.loadSystem(r)
	data.Error = errMsg
	if errMsg == "" {
		data.DNS = s.DNS
	}
	h.render(w, r, h.DNSTemplate, "configure-dns", data)
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

	// Empty value = the "UTC (default)" placeholder.  Treat it as a leaf
	// delete so the candidate matches Infix's "unset means UTC" convention;
	// swallow data-missing for idempotency (the leaf may already be absent).
	if r.FormValue("timezone") == "" {
		err := h.RC.Delete(r.Context(), candidatePath+"/ietf-system:system/clock/timezone-name")
		if err != nil && !restconf.IsDataMissing(err) {
			log.Printf("configure system clock: %v", err)
			renderSaveError(w, err)
			return
		}
		renderSaved(w, "Timezone saved")
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
	renderSaved(w, "Timezone saved")
}

// SaveNTP replaces the NTP server list in the candidate datastore.
// ntpCandPath is the NTP container in the candidate datastore.
const ntpCandPath = candidatePath + "/ietf-system:system/ietf-system:ntp"

func ntpServerCandPath(name string) string {
	return ntpCandPath + "/server=" + url.PathEscape(name)
}

// ntpServerFromForm builds the RESTCONF body for one server, omitting the
// udp transport when no address is given (empty inet:host is invalid) — so a
// server can be added by name and have its address filled in via the
// expand-to-edit form afterwards.
func ntpServerFromForm(r *http.Request, name string) map[string]any {
	srv := map[string]any{
		"name":             name,
		"association-type": defaultStr(r.FormValue("association-type"), "server"),
		"iburst":           r.FormValue("iburst") == "true",
		"prefer":           r.FormValue("prefer") == "true",
	}
	if addr := strings.TrimSpace(r.FormValue("address")); addr != "" {
		udp := map[string]any{"address": addr}
		if p, err := strconv.ParseUint(strings.TrimSpace(r.FormValue("port")), 10, 16); err == nil && p > 0 {
			udp["port"] = p
		}
		srv["udp"] = udp
	}
	return srv
}

func defaultStr(v, def string) string {
	if strings.TrimSpace(v) == "" {
		return def
	}
	return v
}

// AddNTPServer adds an NTP server, creating + enabling the NTP container on
// first use, then re-renders the page.
// POST /configure/system/ntp/servers
func (h *ConfigureSystemHandler) AddNTPServer(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	name := strings.TrimSpace(r.FormValue("name"))
	if name == "" {
		renderSaveError(w, fmt.Errorf("server name is required"))
		return
	}
	// PATCH the system (always present) with the NTP container nested: it is
	// created + enabled on first use and the server merges into the list.
	body := map[string]any{
		"ietf-system:system": map[string]any{
			"ietf-system:ntp": map[string]any{
				"enabled": true,
				"server":  []any{ntpServerFromForm(r, name)},
			},
		},
	}
	if err := h.RC.Patch(r.Context(), candidatePath+"/ietf-system:system", body); err != nil {
		log.Printf("configure ntp add server: %v", err)
		renderSaveError(w, err)
		return
	}
	renderSavedRedirect(w, "NTP server added", "/configure/ntp")
}

// SaveNTPServer replaces one NTP server's configuration (per-row edit).
// POST /configure/system/ntp/servers/{name}
func (h *ConfigureSystemHandler) SaveNTPServer(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	name := r.PathValue("name")
	body := map[string]any{"ietf-system:server": []any{ntpServerFromForm(r, name)}}
	if err := h.RC.Put(r.Context(), ntpServerCandPath(name), body); err != nil {
		log.Printf("configure ntp save server %q: %v", name, err)
		renderSaveError(w, err)
		return
	}
	renderSaved(w, "Server saved")
}

// DeleteNTPServer removes an NTP server.
// DELETE /configure/system/ntp/servers/{name}
func (h *ConfigureSystemHandler) DeleteNTPServer(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	if err := h.RC.Delete(r.Context(), ntpServerCandPath(name)); err != nil && !restconf.IsNotFound(err) {
		log.Printf("configure ntp delete server %q: %v", name, err)
		renderSaveError(w, err)
		return
	}
	renderSavedRedirect(w, "NTP server deleted", "/configure/ntp")
}

// dnsCandPath is the DNS resolver container in the candidate datastore.
const dnsCandPath = candidatePath + "/ietf-system:system/ietf-system:dns-resolver"

func dnsServerCandPath(name string) string {
	return dnsCandPath + "/server=" + url.PathEscape(name)
}

func dnsServerFromForm(r *http.Request, name string) map[string]any {
	srv := map[string]any{"name": name}
	if addr := strings.TrimSpace(r.FormValue("address")); addr != "" {
		ut := map[string]any{"address": addr}
		if p, err := strconv.ParseUint(strings.TrimSpace(r.FormValue("port")), 10, 16); err == nil && p > 0 {
			ut["port"] = p
		}
		srv["udp-and-tcp"] = ut
	}
	return srv
}

// AddDNSServer adds a DNS server, creating the resolver container on first
// use, then re-renders the page.
// POST /configure/system/dns/servers
func (h *ConfigureSystemHandler) AddDNSServer(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	name := strings.TrimSpace(r.FormValue("name"))
	if name == "" {
		renderSaveError(w, fmt.Errorf("server name is required"))
		return
	}
	body := map[string]any{
		"ietf-system:system": map[string]any{
			"ietf-system:dns-resolver": map[string]any{"server": []any{dnsServerFromForm(r, name)}},
		},
	}
	if err := h.RC.Patch(r.Context(), candidatePath+"/ietf-system:system", body); err != nil {
		log.Printf("configure dns add server: %v", err)
		renderSaveError(w, err)
		return
	}
	renderSavedRedirect(w, "DNS server added", "/configure/dns")
}

// SaveDNSServer replaces one DNS server's configuration (per-row edit).
// POST /configure/system/dns/servers/{name}
func (h *ConfigureSystemHandler) SaveDNSServer(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	name := r.PathValue("name")
	body := map[string]any{"ietf-system:server": []any{dnsServerFromForm(r, name)}}
	if err := h.RC.Put(r.Context(), dnsServerCandPath(name), body); err != nil {
		log.Printf("configure dns save server %q: %v", name, err)
		renderSaveError(w, err)
		return
	}
	renderSaved(w, "Server saved")
}

// DeleteDNSServer removes a DNS server.
// DELETE /configure/system/dns/servers/{name}
func (h *ConfigureSystemHandler) DeleteDNSServer(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	if err := h.RC.Delete(r.Context(), dnsServerCandPath(name)); err != nil && !restconf.IsNotFound(err) {
		log.Printf("configure dns delete server %q: %v", name, err)
		renderSaveError(w, err)
		return
	}
	renderSavedRedirect(w, "DNS server deleted", "/configure/dns")
}

// AddDNSSearch appends a search domain to the resolver's search leaf-list.
// POST /configure/system/dns/search
func (h *ConfigureSystemHandler) AddDNSSearch(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	domain := strings.TrimSpace(r.FormValue("domain"))
	if domain == "" {
		renderSaveError(w, fmt.Errorf("domain is required"))
		return
	}
	// POST appends to the leaf-list; if the resolver container doesn't exist
	// yet, create it with this domain via a merge on the system.
	err := h.RC.PostJSON(r.Context(), dnsCandPath, map[string]any{"ietf-system:search": []any{domain}})
	if restconf.IsNotFound(err) {
		err = h.RC.Patch(r.Context(), candidatePath+"/ietf-system:system", map[string]any{
			"ietf-system:system": map[string]any{
				"ietf-system:dns-resolver": map[string]any{"search": []any{domain}},
			},
		})
	}
	if err != nil {
		log.Printf("configure dns add search: %v", err)
		renderSaveError(w, err)
		return
	}
	renderSavedRedirect(w, "Search domain added", "/configure/dns")
}

// DeleteDNSSearch removes a search domain.
// DELETE /configure/system/dns/search/{domain}
func (h *ConfigureSystemHandler) DeleteDNSSearch(w http.ResponseWriter, r *http.Request) {
	domain := r.PathValue("domain")
	path := dnsCandPath + "/search=" + url.PathEscape(domain)
	if err := h.RC.Delete(r.Context(), path); err != nil && !restconf.IsNotFound(err) {
		log.Printf("configure dns delete search %q: %v", domain, err)
		renderSaveError(w, err)
		return
	}
	renderSavedRedirect(w, "Search domain removed", "/configure/dns")
}

// ─── Form parsing helpers ─────────────────────────────────────────────────────


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

// renderSavedRedirect logs a cfgSaved activity entry and then navigates HTMX to
// the given page path (targeting #content). Use this instead of a bare HX-Location
// for Add/Delete operations that redirect back to the listing page after success.
func renderSavedRedirect(w http.ResponseWriter, msg, path string) {
	b, _ := json.Marshal(msg)
	w.Header().Set("HX-Trigger", `{"cfgSaved":`+string(b)+`}`)
	w.Header().Set("HX-Location", `{"path":"`+path+`","target":"#content"}`)
	w.WriteHeader(http.StatusNoContent)
}

// renderSaveError writes an inline error for HTMX. HX-Trigger ensures forms with
// hx-swap="none" still receive the cfgError event (body swap alone would be silenced).
// RESTCONF errors surface their server-side Message; all other errors fall back to
// err.Error() so handler-level validation messages reach the user instead of being
// flattened to a generic "Save failed".
func renderSaveError(w http.ResponseWriter, err error) {
	msg := "Save failed"
	if re, ok := err.(*restconf.Error); ok && re.Message != "" {
		msg = re.Message
	} else if err != nil && err.Error() != "" {
		msg = err.Error()
	}
	w.Header().Set("Content-Type", "text/html")
	b, _ := json.Marshal(msg)
	w.Header().Set("HX-Trigger", `{"cfgError":`+string(b)+`}`)
	w.WriteHeader(http.StatusUnprocessableEntity)
	w.Write([]byte(`<span class="cfg-save-error">` + template.HTMLEscapeString(msg) + `</span>`))
}
