// SPDX-License-Identifier: MIT

package handlers

import (
	"fmt"
	"html/template"
	"log"
	"net/http"
	"strings"
	"sync"

	"infix/webui/internal/restconf"
	"infix/webui/internal/schema"

	"github.com/kernelkit/infix/src/features"
)

// ─── RESTCONF paths ──────────────────────────────────────────────────────────

const (
	staticCPPSuffix  = "/ietf-routing:routing/control-plane-protocols/control-plane-protocol=infix-routing%3Astatic,static"
	staticRtSuffix   = staticCPPSuffix + "/static-routes"
	staticIPv4Suffix = staticRtSuffix + "/ietf-ipv4-unicast-routing:ipv4"
	staticIPv6Suffix = staticRtSuffix + "/ietf-ipv6-unicast-routing:ipv6"
)

// ─── RESTCONF JSON types ──────────────────────────────────────────────────────

type staticRoutesJSON struct {
	IPv4 *staticIPJSON `json:"ietf-ipv4-unicast-routing:ipv4,omitempty"`
	IPv6 *staticIPJSON `json:"ietf-ipv6-unicast-routing:ipv6,omitempty"`
}

type staticIPJSON struct {
	Routes []staticRouteJSON `json:"route"`
}

type staticRouteJSON struct {
	Prefix  string            `json:"destination-prefix"`
	NextHop staticNextHopJSON `json:"next-hop"`
}

type staticNextHopJSON struct {
	Address   string `json:"next-hop-address,omitempty"`
	Interface string `json:"outgoing-interface,omitempty"`
}

// ─── Template data ────────────────────────────────────────────────────────────

type cfgRouteEntry struct {
	Family    string // "ipv4" or "ipv6"
	Prefix    string
	NextHop   string
	Interface string
}

type cfgRoutesPageData struct {
	PageData
	Loading bool
	Routes  []cfgRouteEntry // IPv4, then IPv6
	Ifaces  []string        // outgoing interfaces to choose from
	// Routing protocols, in images with FRR
	Protocols    []rtRow
	Addable      []rtProto // not yet set up, offered by the add dialog
	NoForwarding []rtNoForwarding
	Desc         map[string]string
	Error        string
}

// ─── Handler ─────────────────────────────────────────────────────────────────

// ConfigureRoutesHandler serves the Configure > Routing page.
type ConfigureRoutesHandler struct {
	Template *template.Template
	RC       restconf.Fetcher
	Schema   *schema.Cache
}

// Overview renders the Configure > Routing page reading from the candidate.
// GET /configure/routes
func (h *ConfigureRoutesHandler) Overview(w http.ResponseWriter, r *http.Request) {
	data := cfgRoutesPageData{
		PageData: newPageData(w, r, "configure-routes", "Routing"),
	}

	mgr := h.Schema.Manager()
	data.Loading = mgr == nil
	protocols := mgr != nil && data.Capabilities.Has(features.Routing)

	// The protocols and the interfaces
	ctx := r.Context()
	var (
		cpps   []rtProtoJSON
		ifaces []ifaceJSON
		err    error
		wg     sync.WaitGroup
	)
	wg.Add(1)
	go func() { defer wg.Done(); cpps, err = h.fetchProtocols(ctx) }()
	wg.Add(1)
	go func() { defer wg.Done(); ifaces = h.fetchRoutingIfaces(ctx) }()
	wg.Wait()
	if err != nil {
		log.Printf("configure routes: %v", err)
		data.Error = "Could not read the routing configuration"
	}

	if mgr != nil {
		rt := "/ietf-routing:routing/control-plane-protocols/control-plane-protocol/static-routes/ietf-ipv4-unicast-routing:ipv4/route"
		data.Desc = map[string]string{
			"prefix":    schema.DescriptionOf(mgr, rt+"/destination-prefix"),
			"nexthop":   schema.DescriptionOf(mgr, rt+"/next-hop/next-hop-address"),
			"interface": schema.DescriptionOf(mgr, rt+"/next-hop/outgoing-interface"),
		}
		if protocols && err == nil {
			buildRoutingProtocols(mgr, cpps, ifaces, &data)
		}
	}

	if entry := findInstance(cpps, "infix-routing:static"); entry != nil {
		if entry.StaticRoutes.IPv4 != nil {
			for _, rt := range entry.StaticRoutes.IPv4.Routes {
				data.Routes = append(data.Routes, cfgRouteEntry{
					Family:    "ipv4",
					Prefix:    rt.Prefix,
					NextHop:   rt.NextHop.Address,
					Interface: rt.NextHop.Interface,
				})
			}
		}
		if entry.StaticRoutes.IPv6 != nil {
			for _, rt := range entry.StaticRoutes.IPv6.Routes {
				data.Routes = append(data.Routes, cfgRouteEntry{
					Family:    "ipv6",
					Prefix:    rt.Prefix,
					NextHop:   rt.NextHop.Address,
					Interface: rt.NextHop.Interface,
				})
			}
		}
	}

	var used []string
	for _, rt := range data.Routes {
		if rt.Interface != "" {
			used = append(used, rt.Interface)
		}
	}
	data.Ifaces = ifaceChoices(ifaces, used)

	tmplName := "configure-routes.html"
	if r.Header.Get("HX-Request") == "true" {
		tmplName = "content"
	}
	if err := h.Template.ExecuteTemplate(w, tmplName, data); err != nil {
		log.Printf("template error: %v", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
	}
}

// AddRoute adds a static route to the candidate datastore.
// POST /configure/routes
func (h *ConfigureRoutesHandler) AddRoute(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}

	prefix := strings.TrimSpace(r.FormValue("prefix"))
	nexthop := strings.TrimSpace(r.FormValue("nexthop"))
	iface := strings.TrimSpace(r.FormValue("interface"))

	// The family follows from the destination, e.g. 10.0.0.0/8 or 2001:db8::/32
	family := "ipv4"
	if strings.Contains(prefix, ":") {
		family = "ipv6"
	}
	ipKey, _, _ := familyKeys(family)
	if prefix == "" {
		renderSaveError(w, fmt.Errorf("destination prefix is required"))
		return
	}
	if nexthop == "" && iface == "" {
		renderSaveError(w, fmt.Errorf("next-hop address or outgoing interface is required"))
		return
	}

	nhMap := map[string]any{}
	if nexthop != "" {
		nhMap["next-hop-address"] = nexthop
	}
	if iface != "" {
		nhMap["outgoing-interface"] = iface
	}

	// PATCH at the routing root so the CPP is created if absent (merge semantics).
	// ipv4/ipv6 containers live under the static-routes intermediate container.
	body := map[string]any{
		"ietf-routing:routing": map[string]any{
			"control-plane-protocols": map[string]any{
				"control-plane-protocol": []map[string]any{{
					"type": "infix-routing:static",
					"name": "static",
					"static-routes": map[string]any{
						ipKey: map[string]any{
							"route": []map[string]any{{
								"destination-prefix": prefix,
								"next-hop":           nhMap,
							}},
						},
					},
				}},
			},
		},
	}
	if err := h.RC.Patch(r.Context(), candidatePath+"/ietf-routing:routing", body); err != nil {
		log.Printf("configure routes add %q: %v", prefix, err)
		renderSaveError(w, err)
		return
	}
	renderSavedRedirect(w, "Route added", "/configure/routes")
}

// UpdateRoute replaces the next-hop of an existing static route.
// PUT /configure/routes
func (h *ConfigureRoutesHandler) UpdateRoute(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	prefix := strings.TrimSpace(r.FormValue("prefix"))
	nexthop := strings.TrimSpace(r.FormValue("nexthop"))
	iface := strings.TrimSpace(r.FormValue("interface"))

	_, suffix, ok := familyKeys(r.FormValue("family"))
	if !ok || prefix == "" {
		renderSaveError(w, fmt.Errorf("invalid address family or prefix"))
		return
	}
	if nexthop == "" && iface == "" {
		renderSaveError(w, fmt.Errorf("next-hop address or outgoing interface is required"))
		return
	}

	nhMap := map[string]any{}
	if nexthop != "" {
		nhMap["next-hop-address"] = nexthop
	}
	if iface != "" {
		nhMap["outgoing-interface"] = iface
	}

	var routeKey string
	switch r.FormValue("family") {
	case "ipv4":
		routeKey = "ietf-ipv4-unicast-routing:route"
	case "ipv6":
		routeKey = "ietf-ipv6-unicast-routing:route"
	}

	path := candidatePath + suffix + "/route=" + restconf.EscapeKey(prefix)
	body := map[string]any{
		routeKey: []map[string]any{{
			"destination-prefix": prefix,
			"next-hop":           nhMap,
		}},
	}
	if err := h.RC.Put(r.Context(), path, body); err != nil {
		log.Printf("configure routes update %q: %v", prefix, err)
		renderSaveError(w, err)
		return
	}
	renderSavedRedirect(w, "Route updated", "/configure/routes")
}

// DeleteRoute removes a static route from the candidate datastore.
// DELETE /configure/routes?family=ipv4&prefix=10.0.0.0/24
func (h *ConfigureRoutesHandler) DeleteRoute(w http.ResponseWriter, r *http.Request) {
	prefix := r.URL.Query().Get("prefix")
	_, suffix, ok := familyKeys(r.URL.Query().Get("family"))
	if !ok || prefix == "" {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}

	path := candidatePath + suffix + "/route=" + restconf.EscapeKey(prefix)
	if err := h.RC.Delete(r.Context(), path); err != nil {
		log.Printf("configure routes delete %q: %v", prefix, err)
		renderSaveError(w, err)
		return
	}
	renderSavedRedirect(w, "Route deleted", "/configure/routes")
}

// ─── Helpers ─────────────────────────────────────────────────────────────────

// familyKeys maps an address family string ("ipv4" or "ipv6") to the
// YANG module-qualified ipv4/ipv6 key and the RESTCONF path suffix for that
// family's route list. ok is false for unknown family values.
func familyKeys(family string) (ipKey, suffix string, ok bool) {
	switch family {
	case "ipv4":
		return "ietf-ipv4-unicast-routing:ipv4", staticIPv4Suffix, true
	case "ipv6":
		return "ietf-ipv6-unicast-routing:ipv6", staticIPv6Suffix, true
	}
	return "", "", false
}
