// SPDX-License-Identifier: MIT

package handlers

import (
	"html/template"
	"log"
	"net/http"
	"sort"
	"strings"

	"infix/webui/internal/restconf"
)

type QoSHandler struct {
	Template *template.Template
	RC       restconf.Fetcher
}

type qosRow struct {
	Name           string
	Profile        string
	TrafficClasses int
	RateLimit      string // "100 Mbit/s", empty for none
	Offload        string // "none", or the stages the fabric runs
}

type qosPageData struct {
	PageData
	Error string
	Rows  []qosRow
}

// Overview lists what each port runs: the profile the daemon applied,
// its traffic classes, the rate limit in effect and the stages the
// fabric took.  Operational data only.
func (h *QoSHandler) Overview(w http.ResponseWriter, r *http.Request) {
	data := qosPageData{PageData: newPageData(w, r, "qos", "QoS")}

	var raw interfacesWrapper
	if err := h.RC.Get(r.Context(), "/data/ietf-interfaces:interfaces", &raw); err != nil {
		log.Printf("restconf qos: %v", err)
		data.Error = "Failed to fetch interface data"
	}
	for _, iface := range raw.Interfaces.Interface {
		if iface.QoS == nil || iface.QoS.Capabilities == nil {
			continue
		}
		row := qosRow{Name: iface.Name, Profile: iface.QoS.Profile, TrafficClasses: 8}
		if iface.QoS.Capabilities.MaxTrafficClasses > 0 {
			row.TrafficClasses = iface.QoS.Capabilities.MaxTrafficClasses
		}
		if iface.QoS.RateLimit != nil && iface.QoS.RateLimit.Rate > 0 {
			row.RateLimit = formatBitRate(uint64(iface.QoS.RateLimit.Rate))
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
