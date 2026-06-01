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
	Policies            []cfgPolicyRow
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
		data.Desc = map[string]string{
			"enabled":           schema.DescriptionOf(mgr, fwPath+"/enabled"),
			"default":           schema.DescriptionOf(mgr, fwPath+"/default"),
			"logging":           schema.DescriptionOf(mgr, fwPath+"/logging"),
			"zone-name":         schema.DescriptionOf(mgr, zPath+"/name"),
			"zone-action":       schema.DescriptionOf(mgr, zPath+"/action"),
			"zone-description":  schema.DescriptionOf(mgr, zPath+"/description"),
			"zone-interface":    schema.DescriptionOf(mgr, zPath+"/interface"),
			"zone-service":      schema.DescriptionOf(mgr, zPath+"/service"),
			"policy-name":       schema.DescriptionOf(mgr, pPath+"/name"),
			"policy-action":     schema.DescriptionOf(mgr, pPath+"/action"),
			"policy-ingress":    schema.DescriptionOf(mgr, pPath+"/ingress"),
			"policy-egress":     schema.DescriptionOf(mgr, pPath+"/egress"),
			"policy-masquerade": schema.DescriptionOf(mgr, pPath+"/masquerade"),
		}
		data.LoggingOptions = schema.OptionsFor(mgr, fwPath+"/logging")
		data.ActionOptions = schema.OptionsFor(mgr, zPath+"/action")
		data.PolicyActionOptions = schema.OptionsFor(mgr, pPath+"/action")
		data.ServiceOptions = schema.OptionsFor(mgr, zPath+"/service")
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
		}
		for _, p := range fw.Policy {
			masq := "—"
			if p.Masquerade {
				masq = "Yes"
			}
			data.Policies = append(data.Policies, cfgPolicyRow{
				policyJSON:     p,
				IngressDisplay: strings.Join(p.Ingress, ", "),
				EgressDisplay:  strings.Join(p.Egress, ", "),
				MasqDisplay:    masq,
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

// AddPolicy creates a new inter-zone forwarding policy.
// POST /configure/firewall/policies
func (h *ConfigureFirewallHandler) AddPolicy(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	name := strings.TrimSpace(r.FormValue("name"))
	ingress := strings.Fields(r.FormValue("ingress"))
	egress := strings.Fields(r.FormValue("egress"))
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
	ingress := strings.Fields(r.FormValue("ingress"))
	egress := strings.Fields(r.FormValue("egress"))
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
