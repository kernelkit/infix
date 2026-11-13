// SPDX-License-Identifier: MIT

package handlers

import (
	"html/template"
	"log"
	"net/http"
	"sort"
	"strings"

	"infix/webui/internal/restconf"
	"infix/webui/internal/schema"
)

type QoSHandler struct {
	Template *template.Template
	RC       restconf.Fetcher
	Schema   *schema.Cache
}

type qosRow struct {
	Name           string
	Profile        string
	TrafficClasses int
	Shaper         string // "100 Mbps", empty for none
	Offload        string // "none", or the stages the fabric runs
}

type qosPageData struct {
	PageData
	Error string
	Rows  []qosRow
	Desc  map[string]string // column help, from the model
}

// Overview lists what each port runs: the profile the daemon applied,
// its traffic classes, the egress shaper in effect and the stages the
// fabric took.  Operational data only.
func (h *QoSHandler) Overview(w http.ResponseWriter, r *http.Request) {
	data := qosPageData{PageData: newPageData(w, r, "qos", "QoS")}
	if h.Schema != nil {
		mgr := h.Schema.Manager()
		qos := "/ietf-interfaces:interfaces/interface/infix-interfaces:qos"
		data.Desc = map[string]string{
			"profile":       schema.DescriptionOf(mgr, qos+"/active-profile"),
			"classes":       schema.DescriptionOf(mgr, qos+"/capabilities/max-traffic-classes"),
			"egress-shaper": schema.DescriptionOf(mgr, qos+"/egress-shaper"),
			"offload":       schema.DescriptionOf(mgr, qos+"/capabilities/offload"),
		}
	}

	var raw interfacesWrapper
	if err := h.RC.Get(r.Context(), "/data/ietf-interfaces:interfaces", &raw); err != nil {
		log.Printf("restconf qos: %v", err)
		data.Error = "Failed to fetch interface data"
	}
	for _, iface := range raw.Interfaces.Interface {
		if iface.QoS == nil || iface.QoS.Capabilities == nil {
			continue
		}
		row := qosRow{Name: iface.Name, Profile: iface.QoS.ActiveProfile, TrafficClasses: 8}
		if iface.QoS.Capabilities.MaxTrafficClasses > 0 {
			row.TrafficClasses = iface.QoS.Capabilities.MaxTrafficClasses
		}
		if iface.QoS.Shaper != nil && iface.QoS.Shaper.Rate > 0 {
			row.Shaper = formatBitRate(uint64(iface.QoS.Shaper.Rate))
		}
		row.Offload = "none"
		if len(iface.QoS.Capabilities.Offload) > 0 {
			row.Offload = strings.Join(iface.QoS.Capabilities.Offload, ", ")
		}
		data.Rows = append(data.Rows, row)
	}
	sort.Slice(data.Rows, func(i, j int) bool { return data.Rows[i].Name < data.Rows[j].Name })

	tmplName := "qos.html"
	if r.Header.Get("HX-Request") == "true" {
		tmplName = "content"
	}
	if err := h.Template.ExecuteTemplate(w, tmplName, data); err != nil {
		log.Printf("template error: %v", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
	}
}
