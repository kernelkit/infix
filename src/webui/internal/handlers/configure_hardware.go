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
	"strconv"
	"strings"
	"sync"

	"infix/webui/internal/restconf"
	"infix/webui/internal/schema"
)

// Configure > Hardware: curated management of components in ietf-hardware /
// infix-hardware that have configurable leaves — USB ports (lock/unlock),
// WiFi radios (country code, channel, band) and GPS receivers (presence).
//
// The page mirrors the Configure > Interfaces model: the main table lists
// only components present in running/candidate config; an "Add hardware"
// picker offers detected-but-unconfigured components from operational.
// Status > Hardware shows the full detected inventory (sensors, chassis,
// VPD, …) — this page stays focused on what the user can configure.

const hwCandPath = candidatePath + "/ietf-hardware:hardware"

// hwCompCfgRow is one configured component in the main table. Per-class
// fields are populated only for the matching Class. IsUSB/IsWiFi/IsGPS
// spare the template from dispatching on stringly-typed Class slugs.
type hwCompCfgRow struct {
	Name         string
	Class        string
	ClassDisplay string
	Description  string
	IsUSB        bool
	IsWiFi       bool
	IsGPS        bool

	// USB-specific.
	Unlocked bool // admin-state == "unlocked"

	// WiFi-specific.
	CountryCode string
	Channel     string
	Band        string

	// Schema descriptions carried per-row so the fold-out forms are
	// self-contained — Go templates can't pass extra arguments through
	// {{template}}.
	CountryOptions  []schema.IdentityOption
	BandOptions     []schema.IdentityOption
	DescDescription string
	DescAdminState  string
	DescCountry     string
	DescBand        string
	DescChannel     string
}

// hwAvailable is a detected-but-unconfigured component shown in the Add
// picker. Class drives which class-specific Add fields the inline form
// renders.
type hwAvailable struct {
	Name         string
	Class        string
	ClassDisplay string
}

type cfgHardwarePageData struct {
	PageData
	Loading        bool
	Configured     []hwCompCfgRow
	AvailableUSB   []hwAvailable
	AvailableWiFi  []hwAvailable
	AvailableGPS   []hwAvailable
	CountryOptions []schema.IdentityOption
	BandOptions    []schema.IdentityOption
	Desc           map[string]string
	Error          string
}

// HasAvailable reports whether any detected component can be added. Used
// by the template to hide the "+ Add hardware" affordance entirely when
// the user has already configured everything.
func (d cfgHardwarePageData) HasAvailable() bool {
	return len(d.AvailableUSB)+len(d.AvailableWiFi)+len(d.AvailableGPS) > 0
}

// ConfigureHardwareHandler serves the Configure > Hardware page.
type ConfigureHardwareHandler struct {
	Template *template.Template
	RC       restconf.Fetcher
	Schema   *schema.Cache
}

// Overview renders the Configure > Hardware page.
// GET /configure/hardware
func (h *ConfigureHardwareHandler) Overview(w http.ResponseWriter, r *http.Request) {
	data := cfgHardwarePageData{
		PageData: newPageData(r, "configure-hardware", "Configure: Hardware"),
	}

	mgr := h.Schema.Manager()
	data.Loading = mgr == nil
	if mgr != nil {
		compPath := "/ietf-hardware:hardware/component"
		radioPath := compPath + "/infix-hardware:wifi-radio"
		data.Desc = map[string]string{
			"description":  schema.DescriptionOf(mgr, compPath+"/description"),
			"admin-state":  schema.DescriptionOf(mgr, compPath+"/state/admin-state"),
			"country-code": schema.DescriptionOf(mgr, radioPath+"/country-code"),
			"channel":      schema.DescriptionOf(mgr, radioPath+"/channel"),
			"band":         schema.DescriptionOf(mgr, radioPath+"/band"),
		}
		data.CountryOptions = schema.OptionsFor(mgr, radioPath+"/country-code")
		data.BandOptions = schema.OptionsFor(mgr, radioPath+"/band")
	}

	var (
		cfgWrap  hardwareWrapper
		operWrap hardwareWrapper
		cfgErr   error
		operErr  error
		wg       sync.WaitGroup
	)
	wg.Add(2)
	go func() { defer wg.Done(); cfgErr = h.RC.Get(r.Context(), hwCandPath, &cfgWrap) }()
	go func() {
		defer wg.Done()
		operErr = h.RC.Get(r.Context(), "/data/ietf-hardware:hardware", &operWrap)
	}()
	wg.Wait()

	if cfgErr != nil && !restconf.IsNotFound(cfgErr) {
		// 404 just means nothing is configured yet — proceed with empty list.
		log.Printf("configure hardware: candidate fetch: %v", cfgErr)
		data.Error = "Could not read hardware configuration"
	}
	if operErr != nil {
		log.Printf("configure hardware: operational fetch: %v", operErr)
	}

	configured := make(map[string]bool, len(cfgWrap.Hardware.Component))
	for _, c := range cfgWrap.Hardware.Component {
		configured[c.Name] = true
		class := shortClass(c.Class)
		if !isConfigurableClass(class) {
			continue
		}
		data.Configured = append(data.Configured, h.buildRow(c, class, data))
	}
	sort.SliceStable(data.Configured, func(i, j int) bool {
		if data.Configured[i].Class != data.Configured[j].Class {
			return data.Configured[i].Class < data.Configured[j].Class
		}
		return data.Configured[i].Name < data.Configured[j].Name
	})

	for _, c := range operWrap.Hardware.Component {
		if configured[c.Name] {
			continue
		}
		class := shortClass(c.Class)
		if !isConfigurableClass(class) {
			continue
		}
		avail := hwAvailable{Name: c.Name, Class: class, ClassDisplay: hwClassDisplay(class)}
		switch class {
		case classUSB:
			data.AvailableUSB = append(data.AvailableUSB, avail)
		case classWiFi:
			data.AvailableWiFi = append(data.AvailableWiFi, avail)
		case classGPS:
			data.AvailableGPS = append(data.AvailableGPS, avail)
		}
	}
	sortAvail(data.AvailableUSB)
	sortAvail(data.AvailableWiFi)
	sortAvail(data.AvailableGPS)

	tmplName := "configure-hardware.html"
	if r.Header.Get("HX-Request") == "true" {
		tmplName = "content"
	}
	if err := h.Template.ExecuteTemplate(w, tmplName, data); err != nil {
		log.Printf("template error: %v", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
	}
}

func (h *ConfigureHardwareHandler) buildRow(c hwComponentJSON, class string, data cfgHardwarePageData) hwCompCfgRow {
	row := hwCompCfgRow{
		Name:            c.Name,
		Class:           class,
		ClassDisplay:    hwClassDisplay(class),
		Description:     c.Description,
		DescDescription: data.Desc["description"],
		DescAdminState:  data.Desc["admin-state"],
	}
	switch class {
	case classUSB:
		row.IsUSB = true
		row.Unlocked = c.State != nil && c.State.AdminState == adminStateUnlocked
	case classWiFi:
		row.IsWiFi = true
		row.CountryOptions = data.CountryOptions
		row.BandOptions = data.BandOptions
		row.DescCountry = data.Desc["country-code"]
		row.DescBand = data.Desc["band"]
		row.DescChannel = data.Desc["channel"]
		if c.WiFiRadio != nil {
			row.CountryCode = c.WiFiRadio.CountryCode
			row.Band = c.WiFiRadio.Band
			row.Channel = wifiChannelString(c.WiFiRadio.Channel)
		}
	case classGPS:
		row.IsGPS = true
	}
	return row
}

// SaveUSBPort writes description and admin-state for a USB component.
// Per-leaf PUT so the component-list entry is auto-created when it lived
// only in operational before — component-level PATCH 404s otherwise.
// POST /configure/hardware/usb/{name}
func (h *ConfigureHardwareHandler) SaveUSBPort(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	name := r.PathValue("name")
	state := adminStateLocked
	if r.FormValue("enabled") == "true" {
		state = adminStateUnlocked
	}
	if err := h.saveDescription(r.Context(), name, r.FormValue("description")); err != nil {
		log.Printf("configure hardware usb %s description: %v", name, err)
		renderSaveError(w, err)
		return
	}
	if err := h.putAdminState(r.Context(), name, state); err != nil {
		log.Printf("configure hardware usb %s state: %v", name, err)
		renderSaveError(w, err)
		return
	}
	renderSavedRedirect(w, name+" "+state, "/configure/hardware")
}

// SaveWiFiRadio writes description and the wifi-radio container.
// POST /configure/hardware/wifi/{name}
func (h *ConfigureHardwareHandler) SaveWiFiRadio(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	name := r.PathValue("name")
	radio, err := parseWiFiRadio(r)
	if err != nil {
		renderSaveError(w, err)
		return
	}
	if err := h.saveDescription(r.Context(), name, r.FormValue("description")); err != nil {
		log.Printf("configure hardware wifi %s description: %v", name, err)
		renderSaveError(w, err)
		return
	}
	if err := h.putWiFiRadio(r.Context(), name, radio); err != nil {
		log.Printf("configure hardware wifi %s: %v", name, err)
		renderSaveError(w, err)
		return
	}
	renderSavedRedirect(w, "Radio "+name+" saved", "/configure/hardware")
}

// SaveGPS writes description for an already-configured GPS component. The
// gps-receiver presence container is the configured signal; once added it
// is left in place by Save and only removed by DeleteComponent.
// POST /configure/hardware/gps/{name}
func (h *ConfigureHardwareHandler) SaveGPS(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	name := r.PathValue("name")
	if err := h.saveDescription(r.Context(), name, r.FormValue("description")); err != nil {
		log.Printf("configure hardware gps %s description: %v", name, err)
		renderSaveError(w, err)
		return
	}
	renderSavedRedirect(w, "GPS "+name+" saved", "/configure/hardware")
}

// CreateHardware adds a detected-but-unconfigured component to running
// in a single PUT — name+class plus the class-specific child container —
// so the candidate never sees a half-configured component if the request
// is interrupted between writes.
// POST /configure/hardware
func (h *ConfigureHardwareHandler) CreateHardware(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	name := strings.TrimSpace(r.FormValue("name"))
	class := r.FormValue("class")
	if name == "" || class == "" {
		renderSaveError(w, fmt.Errorf("name and class are required"))
		return
	}
	yangClass, ok := yangClassFromSlug(class)
	if !ok {
		renderSaveError(w, fmt.Errorf("unknown hardware class %q", class))
		return
	}

	comp := map[string]any{"name": name, "class": yangClass}
	switch class {
	case classUSB:
		comp["state"] = map[string]any{"admin-state": adminStateUnlocked}
	case classWiFi:
		radio, err := parseWiFiRadio(r)
		if err != nil {
			renderSaveError(w, err)
			return
		}
		comp["infix-hardware:wifi-radio"] = radio
	case classGPS:
		comp["infix-hardware:gps-receiver"] = map[string]any{}
	}
	body := map[string]any{"ietf-hardware:component": []any{comp}}
	if err := h.RC.Put(r.Context(), hwComponentPath(name), body); err != nil {
		log.Printf("configure hardware create %s: %v", name, err)
		renderSaveError(w, err)
		return
	}
	renderSavedRedirect(w, name+" added", "/configure/hardware")
}

func (h *ConfigureHardwareHandler) putAdminState(ctx context.Context, name, state string) error {
	return h.RC.Put(ctx, hwComponentPath(name)+"/state/admin-state",
		map[string]any{"ietf-hardware:admin-state": state})
}

func (h *ConfigureHardwareHandler) putWiFiRadio(ctx context.Context, name string, radio map[string]any) error {
	return h.RC.Put(ctx, hwComponentPath(name)+"/infix-hardware:wifi-radio",
		map[string]any{"infix-hardware:wifi-radio": radio})
}

// parseWiFiRadio builds the wifi-radio body from form fields. Country
// code is mandatory; band and channel are optional and only included
// when non-empty so we don't clobber YANG defaults with empty strings.
func parseWiFiRadio(r *http.Request) (map[string]any, error) {
	country := strings.TrimSpace(r.FormValue("country-code"))
	if country == "" {
		return nil, fmt.Errorf("country code is required")
	}
	radio := map[string]any{"country-code": country}
	if band := strings.TrimSpace(r.FormValue("band")); band != "" {
		radio["band"] = band
	}
	if ch := strings.TrimSpace(r.FormValue("channel")); ch != "" {
		// YANG union: uint16 (1..196) or the literal "auto". Coerce
		// numeric input so the RESTCONF payload type-matches the union.
		if ch == "auto" {
			radio["channel"] = "auto"
		} else {
			n, err := strconv.Atoi(ch)
			if err != nil || n < 1 || n > 196 {
				return nil, fmt.Errorf("channel must be 'auto' or a number 1–196")
			}
			radio["channel"] = n
		}
	}
	return radio, nil
}

// saveDescription PUTs or DELETEs the description leaf depending on whether
// the form value is empty — empty means "no user-supplied description", and
// PUT-ing the empty string would leave a stray empty leaf around.
func (h *ConfigureHardwareHandler) saveDescription(ctx context.Context, name, desc string) error {
	desc = strings.TrimSpace(desc)
	path := hwComponentPath(name) + "/description"
	if desc == "" {
		if err := h.RC.Delete(ctx, path); err != nil && !restconf.IsNotFound(err) {
			return err
		}
		return nil
	}
	return h.RC.Put(ctx, path, map[string]any{"ietf-hardware:description": desc})
}

// DeleteComponent removes the entire running-config entry for a component,
// leaving only the discovered operational state. Reverts the component to
// YANG/operational defaults until the user adds it again.
// DELETE /configure/hardware/{name}
func (h *ConfigureHardwareHandler) DeleteComponent(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	if err := h.RC.Delete(r.Context(), hwComponentPath(name)); err != nil && !restconf.IsNotFound(err) {
		log.Printf("configure hardware delete %s: %v", name, err)
		renderSaveError(w, err)
		return
	}
	renderSavedRedirect(w, name+" configuration removed", "/configure/hardware")
}

func hwComponentPath(name string) string {
	return hwCandPath + "/component=" + url.PathEscape(name)
}

func isConfigurableClass(class string) bool {
	switch class {
	case classUSB, classWiFi, classGPS:
		return true
	}
	return false
}

func yangClassFromSlug(slug string) (string, bool) {
	switch slug {
	case classUSB:
		return "infix-hardware:usb", true
	case classWiFi:
		return "infix-hardware:wifi", true
	case classGPS:
		return "infix-hardware:gps", true
	}
	return "", false
}

// hwClassDisplay turns the short class slug into a human-readable label.
func hwClassDisplay(class string) string {
	switch class {
	case classChassis:
		return "Chassis"
	case classUSB:
		return "USB port"
	case classWiFi:
		return "WiFi radio"
	case classGPS:
		return "GPS receiver"
	}
	return class
}

func sortAvail(s []hwAvailable) {
	sort.SliceStable(s, func(i, j int) bool { return s[i].Name < s[j].Name })
}
