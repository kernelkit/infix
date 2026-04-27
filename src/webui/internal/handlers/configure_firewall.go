// SPDX-License-Identifier: MIT

package handlers

import (
	"context"
	"errors"
	"fmt"
	"html/template"
	"log"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/kernelkit/webui/internal/restconf"
	"github.com/kernelkit/webui/internal/schema"
)

const fwConfigPath = candidatePath + "/infix-firewall:firewall"

// ─── RESTCONF read wrappers ───────────────────────────────────────────────────

// cfgFwWrapper wraps the firewall presence container for GET responses.
// Using a pointer distinguishes "present" from "absent".
type cfgFwWrapper struct {
	Firewall *firewallJSON `json:"infix-firewall:firewall,omitempty"`
}

// cfgFwZoneWrapper is used when reading a single zone by path.
type cfgFwZoneWrapper struct {
	Zone []zoneJSON `json:"infix-firewall:zone"`
}

// ─── Template display rows ────────────────────────────────────────────────────

type cfgZoneRow struct {
	zoneJSON
	IfaceCount  int
	IfaceSet    map[string]bool
	ServiceSet  map[string]bool
	ServicesTxt string // fallback when ServiceOptions unavailable
	NetworksTxt string // comma-separated, shown read-only when zone uses networks
}

type cfgPolicyRow struct {
	policyJSON
	IngressDisplay string
	EgressDisplay  string
	MasqDisplay    string
	IngressSet     map[string]bool
	EgressSet      map[string]bool
}

type cfgServiceRow struct {
	fwServiceJSON
	PortsDisplay string // "tcp:80,443; udp:53" — at-a-glance
}

// cfgFwSvcWrapper is used when reading a single service by path.
type cfgFwSvcWrapper struct {
	Service []fwServiceJSON `json:"infix-firewall:service"`
}

// ─── Template data ────────────────────────────────────────────────────────────

type cfgFirewallPageData struct {
	PageData
	Loading             bool
	Active              bool // firewall presence container exists
	Enabled             bool
	Default             string
	Logging             string
	Zones               []cfgZoneRow
	ZoneNames           []string // for policy ingress/egress multi-select
	Policies            []cfgPolicyRow
	Services            []cfgServiceRow
	ProtoOptions        []schema.IdentityOption
	Desc                map[string]string
	LoggingOptions      []schema.IdentityOption
	ActionOptions       []schema.IdentityOption
	PolicyActionOptions []schema.IdentityOption
	ServiceOptions      []schema.IdentityOption
	AllInterfaces       []string
	Error               string
}

// ─── Handler ─────────────────────────────────────────────────────────────────

// ConfigureFirewallHandler serves the Configure > Firewall page.
type ConfigureFirewallHandler struct {
	Template *template.Template
	RC       restconf.Fetcher
	Schema   *schema.Cache
}

// Overview renders the Configure > Firewall page.
// GET /configure/firewall
func (h *ConfigureFirewallHandler) Overview(w http.ResponseWriter, r *http.Request) {
	data := cfgFirewallPageData{
		PageData: newPageData(r, "configure-firewall", "Configure: Firewall"),
	}

	mgr := h.Schema.Manager()
	data.Loading = mgr == nil
	if mgr != nil {
		fwPath := "/infix-firewall:firewall"
		zPath := fwPath + "/zone"
		pPath := fwPath + "/policy"
		sPath := fwPath + "/service"
		data.Desc = map[string]string{
			"enabled":             schema.DescriptionOf(mgr, fwPath+"/enabled"),
			"default":             schema.DescriptionOf(mgr, fwPath+"/default"),
			"logging":             schema.DescriptionOf(mgr, fwPath+"/logging"),
			"zone-name":           schema.DescriptionOf(mgr, zPath+"/name"),
			"zone-action":         schema.DescriptionOf(mgr, zPath+"/action"),
			"zone-description":    schema.DescriptionOf(mgr, zPath+"/description"),
			"zone-interface":      schema.DescriptionOf(mgr, zPath+"/interface"),
			"zone-service":        schema.DescriptionOf(mgr, zPath+"/service"),
			"policy-name":         schema.DescriptionOf(mgr, pPath+"/name"),
			"policy-action":       schema.DescriptionOf(mgr, pPath+"/action"),
			"policy-ingress":      schema.DescriptionOf(mgr, pPath+"/ingress"),
			"policy-egress":       schema.DescriptionOf(mgr, pPath+"/egress"),
			"policy-masquerade":   schema.DescriptionOf(mgr, pPath+"/masquerade"),
			"service-name":        schema.DescriptionOf(mgr, sPath+"/name"),
			"service-description": schema.DescriptionOf(mgr, sPath+"/description"),
			"service-destination": schema.DescriptionOf(mgr, sPath+"/destination"),
			"service-port-lower":  schema.DescriptionOf(mgr, sPath+"/port/lower"),
			"service-port-upper":  schema.DescriptionOf(mgr, sPath+"/port/upper"),
			"service-port-proto":  schema.DescriptionOf(mgr, sPath+"/port/proto"),
		}
		data.LoggingOptions = schema.OptionsFor(mgr, fwPath+"/logging")
		data.ActionOptions = schema.OptionsFor(mgr, zPath+"/action")
		data.PolicyActionOptions = schema.OptionsFor(mgr, pPath+"/action")
		data.ServiceOptions = schema.OptionsFor(mgr, zPath+"/service")
		data.ProtoOptions = schema.OptionsFor(mgr, sPath+"/port/proto")
	}

	fw, active, err := h.fetchFirewall(r.Context())
	if err != nil {
		log.Printf("configure firewall: %v", err)
		data.Error = "Could not read firewall configuration"
	}
	data.Active = active
	if active && fw != nil {
		if fw.Enabled == nil {
			data.Enabled = true // YANG default
		} else {
			data.Enabled = bool(*fw.Enabled)
		}
		data.Default = fw.Default
		data.Logging = fw.Logging
		for _, z := range fw.Zone {
			ifaceSet := make(map[string]bool, len(z.Interface))
			for _, iface := range z.Interface {
				ifaceSet[iface] = true
			}
			svcSet := make(map[string]bool, len(z.Service))
			for _, svc := range z.Service {
				svcSet[svc] = true
			}
			data.Zones = append(data.Zones, cfgZoneRow{
				zoneJSON:    z,
				IfaceCount:  len(z.Interface),
				IfaceSet:    ifaceSet,
				ServiceSet:  svcSet,
				ServicesTxt: strings.Join(z.Service, "\n"),
				NetworksTxt: strings.Join(z.Network, ", "),
			})
			data.ZoneNames = append(data.ZoneNames, z.Name)
		}
		for _, p := range fw.Policy {
			masq := "—"
			if p.Masquerade {
				masq = "Yes"
			}
			ingressSet := make(map[string]bool, len(p.Ingress))
			for _, z := range p.Ingress {
				ingressSet[z] = true
			}
			egressSet := make(map[string]bool, len(p.Egress))
			for _, z := range p.Egress {
				egressSet[z] = true
			}
			data.Policies = append(data.Policies, cfgPolicyRow{
				policyJSON:     p,
				IngressDisplay: strings.Join(p.Ingress, ", "),
				EgressDisplay:  strings.Join(p.Egress, ", "),
				MasqDisplay:    masq,
				IngressSet:     ingressSet,
				EgressSet:      egressSet,
			})
		}
		for _, s := range fw.Service {
			data.Services = append(data.Services, cfgServiceRow{
				fwServiceJSON:  s,
				PortsDisplay: formatServicePorts(s.Port),
			})
		}
		// Custom services are leafref'd by the zone service leaf-list — surface
		// them in the dropdown alongside the YANG-defined well-known identities
		// so the zone editor can pick either.
		for _, s := range fw.Service {
			data.ServiceOptions = append(data.ServiceOptions, schema.IdentityOption{
				Value: s.Name,
				Label: s.Name,
			})
		}
	}

	data.AllInterfaces = h.fetchInterfaceNames(r.Context())

	tmplName := "configure-firewall.html"
	if r.Header.Get("HX-Request") == "true" {
		tmplName = "content"
	}
	if err := h.Template.ExecuteTemplate(w, tmplName, data); err != nil {
		log.Printf("template error: %v", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
	}
}

// Enable creates the firewall presence container with an initial "trusted" zone.
// POST /configure/firewall/enable
func (h *ConfigureFirewallHandler) Enable(w http.ResponseWriter, r *http.Request) {
	body := map[string]any{
		"infix-firewall:firewall": map[string]any{
			"enabled": true,
			"logging": "off",
			"default": "trusted",
			"zone": []map[string]any{{
				"name":   "trusted",
				"action": "accept",
			}},
		},
	}
	if err := h.RC.Put(r.Context(), fwConfigPath, body); err != nil {
		log.Printf("configure firewall enable: %v", err)
		renderSaveError(w, err)
		return
	}
	renderSavedRedirect(w, "Firewall enabled", "/configure/firewall")
}

// SaveSettings patches the global firewall settings (enabled, logging, default zone).
// POST /configure/firewall/settings
func (h *ConfigureFirewallHandler) SaveSettings(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	body := map[string]any{
		"infix-firewall:firewall": map[string]any{
			"enabled": r.FormValue("enabled") == "true",
			"logging": r.FormValue("logging"),
			"default": r.FormValue("default"),
		},
	}
	if err := h.RC.Patch(r.Context(), fwConfigPath, body); err != nil {
		log.Printf("configure firewall settings: %v", err)
		renderSaveError(w, err)
		return
	}
	renderSaved(w, "Settings saved")
}

// AddZone creates a new zone in the firewall candidate.
// POST /configure/firewall/zones
func (h *ConfigureFirewallHandler) AddZone(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	name := strings.TrimSpace(r.FormValue("name"))
	if name == "" {
		renderSaveError(w, fmt.Errorf("zone name is required"))
		return
	}
	zone := map[string]any{
		"name":   name,
		"action": r.FormValue("action"),
	}
	if desc := strings.TrimSpace(r.FormValue("description")); desc != "" {
		zone["description"] = desc
	}
	body := map[string]any{"infix-firewall:zone": []map[string]any{zone}}
	if err := h.RC.Put(r.Context(), fwConfigPath+"/zone="+url.PathEscape(name), body); err != nil {
		log.Printf("configure firewall zone add %q: %v", name, err)
		renderSaveError(w, err)
		return
	}
	renderSavedRedirect(w, "Zone added", "/configure/firewall")
}

// DeleteZone removes a zone from the firewall.
// DELETE /configure/firewall/zones/{name}
func (h *ConfigureFirewallHandler) DeleteZone(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	if err := h.RC.Delete(r.Context(), fwConfigPath+"/zone="+url.PathEscape(name)); err != nil {
		log.Printf("configure firewall zone delete %q: %v", name, err)
		renderSaveError(w, err)
		return
	}
	renderSavedRedirect(w, "Zone deleted", "/configure/firewall")
}

// SaveZone updates a zone's action, description, interfaces, and services.
// Uses read-modify-write to preserve fields not managed by this UI (network,
// port-forward). Note: port-forward entries are lost on save (Phase 3 limitation).
// POST /configure/firewall/zones/{name}
func (h *ConfigureFirewallHandler) SaveZone(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	name := r.PathValue("name")

	var wrap cfgFwZoneWrapper
	if err := h.RC.Get(r.Context(), fwConfigPath+"/zone="+url.PathEscape(name), &wrap); err != nil {
		log.Printf("configure firewall zone save %q: GET: %v", name, err)
		renderSaveError(w, err)
		return
	}
	cur := zoneJSON{Name: name}
	if len(wrap.Zone) > 0 {
		cur = wrap.Zone[0]
	}

	cur.Action = r.FormValue("action")
	cur.Description = strings.TrimSpace(r.FormValue("description"))

	if len(cur.Network) == 0 {
		ifaces := r.Form["interfaces"]
		if ifaces == nil {
			ifaces = []string{}
		}
		cur.Interface = ifaces
	}
	svcs := r.Form["services"]
	if svcs == nil {
		svcs = []string{}
	}
	cur.Service = svcs

	zone := map[string]any{
		"name":      cur.Name,
		"action":    cur.Action,
		"interface": cur.Interface,
		"service":   cur.Service,
	}
	if cur.Description != "" {
		zone["description"] = cur.Description
	}
	if len(cur.Network) > 0 {
		zone["network"] = cur.Network
	}
	body := map[string]any{"infix-firewall:zone": []map[string]any{zone}}
	if err := h.RC.Put(r.Context(), fwConfigPath+"/zone="+url.PathEscape(name), body); err != nil {
		log.Printf("configure firewall zone save %q: PUT: %v", name, err)
		renderSaveError(w, err)
		return
	}
	renderSavedRedirect(w, "Zone saved", "/configure/firewall")
}

// ResetZoneLeafList clears a leaf-list (interface or service) on a zone by
// re-PUTting the zone container without that field. RFC 8040 leaf-list
// DELETE requires per-entry key predicates, so a bulk clear has to go
// through the parent.
func (h *ConfigureFirewallHandler) resetZoneLeafList(w http.ResponseWriter, r *http.Request, leaf string) {
	name := r.PathValue("name")
	var wrap cfgFwZoneWrapper
	if err := h.RC.Get(r.Context(), fwConfigPath+"/zone="+url.PathEscape(name), &wrap); err != nil {
		log.Printf("configure firewall zone reset %s/%s: GET: %v", name, leaf, err)
		renderSaveError(w, err)
		return
	}
	cur := zoneJSON{Name: name}
	if len(wrap.Zone) > 0 {
		cur = wrap.Zone[0]
	}
	switch leaf {
	case "interface":
		cur.Interface = nil
	case "service":
		cur.Service = nil
	}

	zone := map[string]any{
		"name":   cur.Name,
		"action": cur.Action,
	}
	if cur.Description != "" {
		zone["description"] = cur.Description
	}
	if len(cur.Interface) > 0 {
		zone["interface"] = cur.Interface
	}
	if len(cur.Network) > 0 {
		zone["network"] = cur.Network
	}
	if len(cur.Service) > 0 {
		zone["service"] = cur.Service
	}
	body := map[string]any{"infix-firewall:zone": []map[string]any{zone}}
	if err := h.RC.Put(r.Context(), fwConfigPath+"/zone="+url.PathEscape(name), body); err != nil {
		log.Printf("configure firewall zone reset %s/%s: PUT: %v", name, leaf, err)
		renderSaveError(w, err)
		return
	}
	renderSaved(w, "Reset to default")
}

// ResetZoneInterfaces clears the zone's interface leaf-list.
// DELETE /configure/firewall/zones/{name}/interfaces
func (h *ConfigureFirewallHandler) ResetZoneInterfaces(w http.ResponseWriter, r *http.Request) {
	h.resetZoneLeafList(w, r, "interface")
}

// ResetZoneServices clears the zone's service leaf-list.
// DELETE /configure/firewall/zones/{name}/services
func (h *ConfigureFirewallHandler) ResetZoneServices(w http.ResponseWriter, r *http.Request) {
	h.resetZoneLeafList(w, r, "service")
}

// AddPortForward inserts a new DNAT rule into the zone's port-forward list.
// POST /configure/firewall/zones/{name}/port-forwards
func (h *ConfigureFirewallHandler) AddPortForward(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	zone := r.PathValue("name")
	lower, err := strconv.Atoi(strings.TrimSpace(r.FormValue("lower")))
	if err != nil || lower < 1 || lower > 65535 {
		renderSaveError(w, fmt.Errorf("lower port must be 1–65535"))
		return
	}
	proto := strings.TrimSpace(r.FormValue("proto"))
	if proto == "" {
		renderSaveError(w, fmt.Errorf("protocol is required"))
		return
	}
	pf := map[string]any{"lower": lower, "proto": proto}
	if up := strings.TrimSpace(r.FormValue("upper")); up != "" {
		if uv, err := strconv.Atoi(up); err == nil && uv >= lower && uv <= 65535 {
			pf["upper"] = uv
		}
	}
	to := map[string]any{}
	if addr := strings.TrimSpace(r.FormValue("to-addr")); addr != "" {
		to["addr"] = addr
	}
	if tp := strings.TrimSpace(r.FormValue("to-port")); tp != "" {
		if tpv, err := strconv.Atoi(tp); err == nil {
			to["port"] = tpv
		}
	}
	if len(to) > 0 {
		pf["to"] = to
	}
	body := map[string]any{"infix-firewall:port-forward": []map[string]any{pf}}
	keyPath := fmt.Sprintf("%s/zone=%s/port-forward=%d,%s", fwConfigPath, url.PathEscape(zone), lower, url.PathEscape(proto))
	if err := h.RC.Put(r.Context(), keyPath, body); err != nil {
		log.Printf("configure firewall port-forward add %s/%d/%s: %v", zone, lower, proto, err)
		renderSaveError(w, err)
		return
	}
	renderSavedRedirect(w, "Port-forward added", "/configure/firewall")
}

// DeletePortForward removes a DNAT rule from the zone.
// DELETE /configure/firewall/zones/{name}/port-forwards/{lower}/{proto}
func (h *ConfigureFirewallHandler) DeletePortForward(w http.ResponseWriter, r *http.Request) {
	zone := r.PathValue("name")
	lower := r.PathValue("lower")
	proto := r.PathValue("proto")
	keyPath := fmt.Sprintf("%s/zone=%s/port-forward=%s,%s", fwConfigPath, url.PathEscape(zone), url.PathEscape(lower), url.PathEscape(proto))
	if err := h.RC.Delete(r.Context(), keyPath); err != nil {
		log.Printf("configure firewall port-forward delete %s/%s/%s: %v", zone, lower, proto, err)
		renderSaveError(w, err)
		return
	}
	renderSavedRedirect(w, "Port-forward removed", "/configure/firewall")
}

// AddPolicy creates a new inter-zone forwarding policy.
// POST /configure/firewall/policies
func (h *ConfigureFirewallHandler) AddPolicy(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	name := strings.TrimSpace(r.FormValue("name"))
	ingress := r.Form["ingress"]
	egress := r.Form["egress"]
	if name == "" {
		renderSaveError(w, fmt.Errorf("policy name is required"))
		return
	}
	if len(ingress) == 0 || len(egress) == 0 {
		renderSaveError(w, fmt.Errorf("policy requires at least one ingress and one egress zone"))
		return
	}
	policy := map[string]any{
		"name":    name,
		"action":  r.FormValue("action"),
		"ingress": ingress,
		"egress":  egress,
	}
	if r.FormValue("masquerade") == "on" {
		policy["masquerade"] = true
	}
	body := map[string]any{"infix-firewall:policy": []map[string]any{policy}}
	if err := h.RC.Put(r.Context(), fwConfigPath+"/policy="+url.PathEscape(name), body); err != nil {
		log.Printf("configure firewall policy add %q: %v", name, err)
		renderSaveError(w, err)
		return
	}
	renderSavedRedirect(w, "Policy added", "/configure/firewall")
}

// DeletePolicy removes an inter-zone forwarding policy.
// DELETE /configure/firewall/policies/{name}
func (h *ConfigureFirewallHandler) DeletePolicy(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	if err := h.RC.Delete(r.Context(), fwConfigPath+"/policy="+url.PathEscape(name)); err != nil {
		log.Printf("configure firewall policy delete %q: %v", name, err)
		renderSaveError(w, err)
		return
	}
	renderSavedRedirect(w, "Policy deleted", "/configure/firewall")
}

// SavePolicy updates an existing policy's action, ingress, egress, masquerade.
// POST /configure/firewall/policies/{name}
func (h *ConfigureFirewallHandler) SavePolicy(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	name := r.PathValue("name")
	ingress := r.Form["ingress"]
	egress := r.Form["egress"]
	if len(ingress) == 0 || len(egress) == 0 {
		renderSaveError(w, fmt.Errorf("policy requires at least one ingress and one egress zone"))
		return
	}
	policy := map[string]any{
		"name":       name,
		"action":     r.FormValue("action"),
		"ingress":    ingress,
		"egress":     egress,
		"masquerade": r.FormValue("masquerade") == "true",
	}
	body := map[string]any{"infix-firewall:policy": []map[string]any{policy}}
	if err := h.RC.Put(r.Context(), fwConfigPath+"/policy="+url.PathEscape(name), body); err != nil {
		log.Printf("configure firewall policy save %q: %v", name, err)
		renderSaveError(w, err)
		return
	}
	renderSavedRedirect(w, "Policy saved", "/configure/firewall")
}

// ─── Services CRUD ───────────────────────────────────────────────────────────

// parseServicePorts converts the form's per-port arrays into the YANG body
// shape. Empty rows (no lower port) are silently dropped, so the user can
// leave trailing rows blank without invalidating the submission.
func parseServicePorts(r *http.Request) []map[string]any {
	lower := r.Form["port-lower"]
	upper := r.Form["port-upper"]
	proto := r.Form["port-proto"]
	out := []map[string]any{}
	for i, lo := range lower {
		lo = strings.TrimSpace(lo)
		if lo == "" {
			continue
		}
		lower, err := strconv.Atoi(lo)
		if err != nil {
			continue
		}
		entry := map[string]any{"lower": lower}
		if i < len(proto) && proto[i] != "" {
			entry["proto"] = proto[i]
		}
		if i < len(upper) {
			up := strings.TrimSpace(upper[i])
			if up != "" {
				if uv, err := strconv.Atoi(up); err == nil {
					entry["upper"] = uv
				}
			}
		}
		out = append(out, entry)
	}
	return out
}

// formatServicePorts renders the port list as a one-line summary for the
// services table — "tcp:80,443-445; udp:53".
func formatServicePorts(ports []fwServicePortJSON) string {
	if len(ports) == 0 {
		return ""
	}
	byProto := map[string][]string{}
	order := []string{}
	for _, p := range ports {
		if _, ok := byProto[p.Proto]; !ok {
			order = append(order, p.Proto)
		}
		one := fmt.Sprintf("%d", int(p.Lower))
		if int(p.Upper) > int(p.Lower) {
			one = fmt.Sprintf("%d-%d", int(p.Lower), int(p.Upper))
		}
		byProto[p.Proto] = append(byProto[p.Proto], one)
	}
	parts := make([]string, 0, len(order))
	for _, proto := range order {
		parts = append(parts, fmt.Sprintf("%s:%s", proto, strings.Join(byProto[proto], ",")))
	}
	return strings.Join(parts, "; ")
}

// AddService creates a new user-defined service.
// POST /configure/firewall/services
func (h *ConfigureFirewallHandler) AddService(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	name := strings.TrimSpace(r.FormValue("name"))
	if name == "" {
		renderSaveError(w, fmt.Errorf("service name is required"))
		return
	}
	svc := map[string]any{"name": name}
	if desc := strings.TrimSpace(r.FormValue("description")); desc != "" {
		svc["description"] = desc
	}
	if dest := strings.TrimSpace(r.FormValue("destination")); dest != "" {
		svc["destination"] = dest
	}
	if ports := parseServicePorts(r); len(ports) > 0 {
		svc["port"] = ports
	}
	body := map[string]any{"infix-firewall:service": []map[string]any{svc}}
	if err := h.RC.Put(r.Context(), fwConfigPath+"/service="+url.PathEscape(name), body); err != nil {
		log.Printf("configure firewall service add %q: %v", name, err)
		renderSaveError(w, err)
		return
	}
	renderSavedRedirect(w, "Service added", "/configure/firewall")
}

// SaveService updates an existing user-defined service.
// POST /configure/firewall/services/{name}
func (h *ConfigureFirewallHandler) SaveService(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	name := r.PathValue("name")
	svc := map[string]any{"name": name}
	if desc := strings.TrimSpace(r.FormValue("description")); desc != "" {
		svc["description"] = desc
	}
	if dest := strings.TrimSpace(r.FormValue("destination")); dest != "" {
		svc["destination"] = dest
	}
	if ports := parseServicePorts(r); len(ports) > 0 {
		svc["port"] = ports
	}
	body := map[string]any{"infix-firewall:service": []map[string]any{svc}}
	if err := h.RC.Put(r.Context(), fwConfigPath+"/service="+url.PathEscape(name), body); err != nil {
		log.Printf("configure firewall service save %q: %v", name, err)
		renderSaveError(w, err)
		return
	}
	renderSaved(w, "Service saved")
}

// DeleteService removes a user-defined service.
// DELETE /configure/firewall/services/{name}
func (h *ConfigureFirewallHandler) DeleteService(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	if err := h.RC.Delete(r.Context(), fwConfigPath+"/service="+url.PathEscape(name)); err != nil {
		log.Printf("configure firewall service delete %q: %v", name, err)
		renderSaveError(w, err)
		return
	}
	renderSavedRedirect(w, "Service deleted", "/configure/firewall")
}

// ─── Helpers ─────────────────────────────────────────────────────────────────

// fetchInterfaceNames returns configured interface names from candidate (fallback running).
func (h *ConfigureFirewallHandler) fetchInterfaceNames(ctx context.Context) []string {
	var wrap interfacesWrapper
	if err := h.RC.Get(ctx, candidatePath+"/ietf-interfaces:interfaces", &wrap); err != nil {
		h.RC.Get(ctx, "/data/ietf-interfaces:interfaces", &wrap) //nolint:errcheck
	}
	names := make([]string, 0, len(wrap.Interfaces.Interface))
	for _, iface := range wrap.Interfaces.Interface {
		names = append(names, iface.Name)
	}
	return names
}

// fetchFirewall reads the firewall presence container from candidate,
// falling back to running. Returns (nil, false, nil) when absent everywhere.
func (h *ConfigureFirewallHandler) fetchFirewall(ctx context.Context) (*firewallJSON, bool, error) {
	var wrap cfgFwWrapper
	err := h.RC.Get(ctx, fwConfigPath, &wrap)
	if err == nil {
		return wrap.Firewall, wrap.Firewall != nil, nil
	}
	var rcErr *restconf.Error
	if errors.As(err, &rcErr) && rcErr.StatusCode == http.StatusNotFound {
		runErr := h.RC.Get(ctx, "/data/infix-firewall:firewall", &wrap)
		if runErr == nil {
			return wrap.Firewall, wrap.Firewall != nil, nil
		}
		var rcRun *restconf.Error
		if errors.As(runErr, &rcRun) && rcRun.StatusCode == http.StatusNotFound {
			return nil, false, nil
		}
		return nil, false, runErr
	}
	return nil, false, err
}
