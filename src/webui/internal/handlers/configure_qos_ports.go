// SPDX-License-Identifier: MIT

package handlers

import (
	"context"
	"fmt"
	"html/template"
	"log"
	"net/http"
	"sort"
	"strings"
	"sync"

	"infix/webui/internal/restconf"
	"infix/webui/internal/schema"
)

// qosPortRow is a port on Configure > QoS > Ports.
type qosPortRow struct {
	Name       string
	Profile    string
	Own        bool          // the port carries settings of its own
	ShaperMbit string        // empty for none
	Editor     qosProfileRow // the port's own settings, defaults filled in
}

type cfgQoSPortsPageData struct {
	cfgQoSPageData
	Ports        []qosPortRow
	ProfileNames []string
	QoSBounds    qosBounds
}

// tsnPortRow is a port on Configure > TSN.
type tsnPortRow struct {
	Row      cfgIfaceRow
	Reserved string // "class 7: 50 Mbps, class 6: 20 Mbps"
	Schedule string // "1000 µs cycle, 4 entries"
}

type cfgTSNPageData struct {
	PageData
	Error     string
	Loading   bool
	Ports     []tsnPortRow
	Classes   []int  // the classes a reservation can be for, top down
	SRTable   string // what the stream reservation class table does, from the model
	QoSBounds qosBounds
	Desc      map[string]string
}

// qosPortData reads what the port pages are built from: the interfaces
// as configured and as they run, and the profiles.
func (h *ConfigureInterfacesHandler) qosPortData(ctx context.Context) (ifaces, oper []ifaceJSON, profiles []qosProfileJSON, err error) {
	var (
		operWrap interfacesWrapper
		wg       sync.WaitGroup
	)
	wg.Add(3)
	go func() { defer wg.Done(); ifaces, err = h.fetchAllInterfaces(ctx) }()
	go func() {
		defer wg.Done()
		if e := h.RC.Get(ctx, "/data/ietf-interfaces:interfaces", &operWrap); e != nil {
			log.Printf("configure qos ports: operational: %v", e)
		}
	}()
	go func() {
		defer wg.Done()
		var e error
		if profiles, e = fetchQoSProfiles(ctx, h.RC); e != nil {
			log.Printf("configure qos ports: profiles: %v", e)
		}
	}()
	wg.Wait()
	return ifaces, operWrap.Interfaces.Interface, profiles, err
}

// qosPorts names the interfaces that report QoS capabilities, loopback
// aside.
func qosPorts(oper []ifaceJSON) map[string]bool {
	ports := map[string]bool{}
	for _, iface := range oper {
		if iface.QoS != nil && iface.QoS.Capabilities != nil && schema.StripModulePrefix(iface.Type) != "loopback" {
			ports[iface.Name] = true
		}
	}
	return ports
}

// OverviewQoSPorts lists the ports with the profile each is on, or an
// editor for its own settings.
// GET /configure/qos/ports
func (h *ConfigureInterfacesHandler) OverviewQoSPorts(w http.ResponseWriter, r *http.Request) {
	data := cfgQoSPortsPageData{cfgQoSPageData: cfgQoSPageData{PageData: newPageData(w, r, "configure-qos", "QoS")}}
	mgr := h.Schema.Manager()
	data.Loading = mgr == nil
	var def qosDefaults
	if mgr != nil {
		def = fillQoSEditor(&data.cfgQoSPageData, mgr)
		data.QoSBounds = qosBoundsOf(mgr)
		ifPath := "/ietf-interfaces:interfaces/interface/infix-interfaces:qos"
		data.Desc["port-profile"] = schema.DescriptionOf(mgr, ifPath+"/profile")
		data.Desc["port-shaper"] = schema.DescriptionOf(mgr, ifPath+"/egress-shaper")
	}

	ifaces, oper, profiles, err := h.qosPortData(context.WithoutCancel(r.Context()))
	if err != nil {
		log.Printf("configure qos ports: %v", err)
		data.Error = "Could not read the interfaces"
	}
	for _, p := range profiles {
		data.ProfileNames = append(data.ProfileNames, p.Name)
	}
	ports := qosPorts(oper)
	for _, iface := range ifaces {
		if !ports[iface.Name] {
			continue
		}
		row := qosPortRow{Name: iface.Name}
		own := qosProfileJSON{Name: iface.Name}
		if q := iface.QoS; q != nil {
			row.Profile = q.Profile
			row.Own = q.Custom()
			if q.Shaper != nil && q.Shaper.Rate > 0 {
				row.ShaperMbit = mbitText(int64(q.Shaper.Rate))
			}
			own.Ingress, own.Egress = q.Ingress, q.Egress
		}
		row.Editor = qosProfileRowOf(own, nil, def)
		data.Ports = append(data.Ports, row)
	}
	sort.Slice(data.Ports, func(i, j int) bool { return naturalLess(data.Ports[i].Name, data.Ports[j].Name) })

	h.render(w, r, h.QoSPortsTemplate, "configure-qos-ports.html", data)
}

func (h *ConfigureInterfacesHandler) render(w http.ResponseWriter, r *http.Request, tmpl *template.Template, name string, data any) {
	if r.Header.Get("HX-Request") == "true" {
		name = "content"
	}
	if err := tmpl.ExecuteTemplate(w, name, data); err != nil {
		log.Printf("template error: %v", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
	}
}

// SaveQoSPort sets where a port takes its QoS settings from: the
// model's defaults, a profile, or the settings the editor holds.
// POST /configure/qos/ports/{name}
func (h *ConfigureInterfacesHandler) SaveQoSPort(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	name := r.PathValue("name")
	qosPath := ifacePath(name) + "/infix-interfaces:qos"
	ctx := r.Context()
	fail := func(err error) {
		log.Printf("configure qos port %s: %v", name, err)
		renderSaveError(w, err)
	}

	qos := map[string]any{}
	// The profile and the port's own settings are the two cases of a
	// choice, and a merge keeps list entries the form dropped, so what
	// was there goes first.
	drop := []string{"/profile", "/ingress", "/egress"}
	rate := strings.TrimSpace(r.FormValue("rate"))
	if rate == "" {
		drop = append(drop, "/egress-shaper")
	} else {
		bits, err := mbitField(rate, "the egress shaper rate")
		if err != nil {
			renderSaveError(w, err)
			return
		}
		qos["egress-shaper"] = map[string]any{"rate": bits}
	}

	source := r.FormValue("source")
	switch {
	case source == "default":
	case strings.HasPrefix(source, "profile:"):
		qos["profile"] = strings.TrimPrefix(source, "profile:")
	case source == "own":
		p, err := qosProfileFromForm(r, name)
		if err != nil {
			renderSaveError(w, err)
			return
		}
		// A read below a list entry comes back nested from the top
		var current struct {
			Interfaces struct {
				Interface []struct {
					QoS map[string]any `json:"infix-interfaces:qos"`
				} `json:"interface"`
			} `json:"ietf-interfaces:interfaces"`
		}
		if err := h.RC.Get(ctx, qosPath, &current); err != nil && !restconf.IsNotFound(err) {
			fail(err)
			return
		}
		var cur map[string]any
		if len(current.Interfaces.Interface) > 0 {
			cur = current.Interfaces.Interface[0].QoS
		}
		body, err := qosProfileBody(p, cur)
		if err != nil {
			fail(err)
			return
		}
		for _, node := range []string{"ingress", "egress"} {
			if body[node] != nil {
				qos[node] = body[node]
			}
		}
	default:
		renderSaveError(w, fmt.Errorf("no such settings source %q", source))
		return
	}

	for _, node := range drop {
		if err := h.RC.Delete(ctx, qosPath+node); err != nil && !restconf.IsNotFound(err) {
			fail(err)
			return
		}
	}
	if len(qos) > 0 {
		if err := h.RC.Patch(ctx, qosPath, map[string]any{"infix-interfaces:qos": qos}); err != nil {
			fail(err)
			return
		}
	}
	renderSavedRedirect(w, "QoS saved", "/configure/qos/ports")
}

// tsnSummary says in a few words what reservations and schedule a port
// is configured with.
func tsnSummary(row cfgIfaceRow) (reserved, schedule string) {
	var parts []string
	for _, s := range row.QoSShapers {
		if s.IdleMbit != "" {
			parts = append(parts, fmt.Sprintf("class %d: %s Mbps", s.ID, s.IdleMbit))
		}
	}
	reserved = strings.Join(parts, ", ")
	if g := row.QoSGates; len(g.Entries) > 0 {
		entries := "entries"
		if len(g.Entries) == 1 {
			entries = "entry"
		}
		schedule = fmt.Sprintf("%d %s", len(g.Entries), entries)
		if g.CycleUs != "" {
			schedule = g.CycleUs + " µs cycle, " + schedule
		}
		if !g.Enabled {
			schedule += ", off"
		}
	}
	return reserved, schedule
}

// OverviewTSN lists the ports with their reservations and gate
// schedule, and an editor for both per port.
// GET /configure/tsn
func (h *ConfigureInterfacesHandler) OverviewTSN(w http.ResponseWriter, r *http.Request) {
	data := cfgTSNPageData{PageData: newPageData(w, r, "configure-tsn", "TSN")}
	mgr := h.Schema.Manager()
	data.Loading = mgr == nil
	if mgr != nil {
		data.QoSBounds = qosBoundsOf(mgr)
		ifPath := "/ietf-interfaces:interfaces/interface"
		gates := ifPath + "/ieee802-dot1dc-sched-if:gate-parameter-table"
		data.Desc = map[string]string{
			"tsn-cbs":       schema.DescriptionOf(mgr, ifPath+"/ieee802-dot1dc-cbsa-if:cbsa"),
			"tsn-idle":      schema.DescriptionOf(mgr, ifPath+"/ieee802-dot1dc-cbsa-if:cbsa/cbsa-parameter-table/admin-idle-slope"),
			"tsn-gates":     schema.DescriptionOf(mgr, gates),
			"tsn-list":      schema.DescriptionOf(mgr, gates+"/admin-control-list"),
			"tsn-cycle":     schema.DescriptionOf(mgr, gates+"/admin-cycle-time"),
			"tsn-interval":  schema.DescriptionOf(mgr, gates+"/admin-control-list/gate-control-entry/time-interval-value"),
			"tsn-base":      schema.DescriptionOf(mgr, gates+"/admin-base-time"),
			"tsn-extension": schema.DescriptionOf(mgr, gates+"/admin-cycle-time-extension"),
			"tsn-max-sdu":   schema.DescriptionOf(mgr, gates+"/queue-max-sdu-table/queue-max-sdu"),
		}
		for _, o := range schema.OptionsFor(mgr, qosProfileLeafs().TablePreset) {
			if o.Value == qosTableSR {
				data.SRTable = o.Description
			}
		}
	}
	for c := qosNumClasses - 1; c >= 0; c-- {
		data.Classes = append(data.Classes, c)
	}

	ifaces, oper, profiles, err := h.qosPortData(context.WithoutCancel(r.Context()))
	if err != nil {
		log.Printf("configure tsn: %v", err)
		data.Error = "Could not read the interfaces"
	}
	ports := qosPorts(oper)
	for _, row := range h.buildRows(ifaces, oper, profiles) {
		if !ports[row.Name] {
			continue
		}
		p := tsnPortRow{Row: row}
		p.Reserved, p.Schedule = tsnSummary(row)
		data.Ports = append(data.Ports, p)
	}
	sort.Slice(data.Ports, func(i, j int) bool { return naturalLess(data.Ports[i].Row.Name, data.Ports[j].Row.Name) })

	h.render(w, r, h.TSNTemplate, "configure-tsn.html", data)
}
