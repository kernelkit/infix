// SPDX-License-Identifier: MIT

package handlers

import (
	"fmt"
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
	Trust          string
	TrafficClasses int
	Shaper         string // "100 Mbps", empty for none
	Reserved       string // "7: 50 Mbps, 6: 20 Mbps"
	Schedule       string // "1000 µs cycle, 4 entries"
	Offload        string // "none", or the stages the fabric runs
}

type qosPageData struct {
	PageData
	Error string
	Rows  []qosRow
	Desc  map[string]string // column help, from the model
}

// qosRunning names the reservations and gate schedule a port runs.
func qosRunning(iface *ifaceJSON) (reserved, schedule string) {
	st := tsnStatusOf(iface)
	if st == nil {
		return "", ""
	}
	var parts []string
	for _, s := range st.Shapers {
		parts = append(parts, fmt.Sprintf("%d: %s Mbps", s.Class, s.IdleMbit))
	}
	reserved = strings.Join(parts, ", ")
	if n := len(st.Gates); n > 0 {
		schedule = scheduleText(st.CycleUs, n)
	}
	if st.Pending && schedule != "" {
		schedule += ", change pending"
	} else if st.Pending {
		schedule = "change pending"
	}
	return reserved, schedule
}

// Overview lists the QoS each port runs.  Operational data only.
func (h *QoSHandler) Overview(w http.ResponseWriter, r *http.Request) {
	data := qosPageData{PageData: newPageData(w, r, "qos", "QoS")}
	if h.Schema != nil {
		mgr := h.Schema.Manager()
		qos := "/ietf-interfaces:interfaces/interface/infix-interfaces:qos"
		ifp := "/ietf-interfaces:interfaces/interface"
		data.Desc = map[string]string{
			"trust":         schema.DescriptionOf(mgr, qos+"/ingress/trust"),
			"reserved":      schema.DescriptionOf(mgr, ifp+"/ieee802-dot1dc-cbsa-if:cbsa"),
			"schedule":      schema.DescriptionOf(mgr, ifp+"/ieee802-dot1dc-sched-if:gate-parameter-table"),
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
	ports := qosPorts(raw.Interfaces.Interface)
	for i := range raw.Interfaces.Interface {
		iface := &raw.Interfaces.Interface[i]
		if !ports[iface.Name] {
			continue
		}
		row := qosRow{Name: iface.Name, Profile: iface.QoS.ActiveProfile, TrafficClasses: qosClassCount(iface)}
		if iface.QoS.Ingress != nil {
			row.Trust = iface.QoS.Ingress.Trust
		}
		row.Reserved, row.Schedule = qosRunning(iface)
		if iface.QoS.Shaper != nil && iface.QoS.Shaper.Rate > 0 {
			row.Shaper = formatBitRate(uint64(iface.QoS.Shaper.Rate))
		}
		row.Offload = "none"
		if len(iface.QoS.Capabilities.Offload) > 0 {
			row.Offload = strings.Join(iface.QoS.Capabilities.Offload, ", ")
		}
		data.Rows = append(data.Rows, row)
	}
	sort.Slice(data.Rows, func(i, j int) bool { return naturalLess(data.Rows[i].Name, data.Rows[j].Name) })

	tmplName := "qos.html"
	if r.Header.Get("HX-Request") == "true" {
		tmplName = "content"
	}
	if err := h.Template.ExecuteTemplate(w, tmplName, data); err != nil {
		log.Printf("template error: %v", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
	}
}
