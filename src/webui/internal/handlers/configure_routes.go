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

// ─── RESTCONF paths ──────────────────────────────────────────────────────────

const (
	staticCPPSuffix  = "/ietf-routing:routing/control-plane-protocols/control-plane-protocol=infix-routing%3Astatic,static"
	staticRtSuffix   = staticCPPSuffix + "/static-routes"
	staticIPv4Suffix = staticRtSuffix + "/ietf-ipv4-unicast-routing:ipv4"
	staticIPv6Suffix = staticRtSuffix + "/ietf-ipv6-unicast-routing:ipv6"
)

// ─── RESTCONF JSON types ──────────────────────────────────────────────────────

type staticCPPWrapper struct {
	Routing struct {
		CPPs struct {
			CPP []staticCPPJSON `json:"control-plane-protocol"`
		} `json:"control-plane-protocols"`
	} `json:"ietf-routing:routing"`
}

// staticCPPJSON mirrors control-plane-protocol. The ipv4/ipv6 containers are
// augmented into the child static-routes container, not directly into the CPP.
type staticCPPJSON struct {
	Type         string           `json:"type"`
	Name         string           `json:"name"`
	StaticRoutes staticRoutesJSON `json:"static-routes"`
}

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
	Prefix    string
	NextHop   string
	Interface string
}

type cfgRoutesPageData struct {
	PageData
	Loading    bool
	IPv4Routes []cfgRouteEntry
	IPv6Routes []cfgRouteEntry
	Desc       map[string]string
	Error      string
}

// ─── Handler ─────────────────────────────────────────────────────────────────

// ConfigureRoutesHandler serves the Configure > Routes page.
type ConfigureRoutesHandler struct {
	Template *template.Template
	RC       restconf.Fetcher
	Schema   *schema.Cache
}

// Overview renders the Configure > Routes page reading from the candidate.
// GET /configure/routes
func (h *ConfigureRoutesHandler) Overview(w http.ResponseWriter, r *http.Request) {
	data := cfgRoutesPageData{
		PageData: newPageData(r, "configure-routes", "Configure: Routes"),
	}

	mgr := h.Schema.Manager()
	data.Loading = mgr == nil
	if mgr != nil {
		rt := "/ietf-routing:routing/control-plane-protocols/control-plane-protocol/static-routes/ietf-ipv4-unicast-routing:ipv4/route"
		data.Desc = map[string]string{
			"prefix":    schema.DescriptionOf(mgr, rt+"/destination-prefix"),
			"nexthop":   schema.DescriptionOf(mgr, rt+"/next-hop/next-hop-address"),
			"interface": schema.DescriptionOf(mgr, rt+"/next-hop/outgoing-interface"),
		}
	}

	cpp, err := h.fetchStaticCPP(r.Context())
	if err != nil {
		log.Printf("configure routes: %v", err)
		data.Error = "Could not read static routes"
	}
	if len(cpp.Routing.CPPs.CPP) > 0 {
		entry := cpp.Routing.CPPs.CPP[0]
		if entry.StaticRoutes.IPv4 != nil {
			for _, rt := range entry.StaticRoutes.IPv4.Routes {
				data.IPv4Routes = append(data.IPv4Routes, cfgRouteEntry{
					Prefix:    rt.Prefix,
					NextHop:   rt.NextHop.Address,
					Interface: rt.NextHop.Interface,
				})
			}
		}
		if entry.StaticRoutes.IPv6 != nil {
			for _, rt := range entry.StaticRoutes.IPv6.Routes {
				data.IPv6Routes = append(data.IPv6Routes, cfgRouteEntry{
					Prefix:    rt.Prefix,
					NextHop:   rt.NextHop.Address,
					Interface: rt.NextHop.Interface,
				})
			}
		}
	}

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

	ipKey, _, ok := familyKeys(r.FormValue("family"))
	if !ok {
		renderSaveError(w, fmt.Errorf("invalid address family"))
		return
	}
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

	path := candidatePath + suffix + "/route=" + url.PathEscape(prefix)
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

	path := candidatePath + suffix + "/route=" + url.PathEscape(prefix)
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

// fetchStaticCPP reads the static control-plane-protocol entry from the
// candidate datastore, falling back to running on 404.
func (h *ConfigureRoutesHandler) fetchStaticCPP(ctx context.Context) (staticCPPWrapper, error) {
	var cpp staticCPPWrapper
	err := h.RC.Get(ctx, candidatePath+staticCPPSuffix, &cpp)
	if err == nil {
		return cpp, nil
	}
	var rcErr *restconf.Error
	if errors.As(err, &rcErr) && rcErr.StatusCode == http.StatusNotFound {
		runErr := h.RC.Get(ctx, "/data"+staticCPPSuffix, &cpp)
		if runErr == nil {
			return cpp, nil
		}
		var rcRun *restconf.Error
		if errors.As(runErr, &rcRun) && rcRun.StatusCode == http.StatusNotFound {
			return cpp, nil // no static routes configured — not an error
		}
		return cpp, runErr
	}
	return cpp, err
}
