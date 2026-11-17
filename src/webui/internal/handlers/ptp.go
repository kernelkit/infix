// SPDX-License-Identifier: MIT

package handlers

import (
	"html/template"
	"log"
	"net/http"
	"sort"
	"strconv"

	"infix/webui/internal/restconf"
	"infix/webui/internal/schema"
)

const ptpInstance = "/ieee1588-ptp-tt:ptp/instances/instance"

// PTPHandler serves the PTP status page.
type PTPHandler struct {
	Template *template.Template
	RC       restconf.Fetcher
	Schema   *schema.Cache
}

type ptpPortRow struct {
	Index     int
	Interface string
	State     string
	StateHelp string
	Up        bool   // the port takes part: sends or receives time
	Delay     string // delay mechanism
	LinkDelay string // "1498 ns", empty when not measured
	Sync      string // "rx / tx"
	Announce  string
	PDelay    string
	HasStats  bool
}

type ptpInstanceView struct {
	Index       int
	Type        string // "Boundary Clock", from the model
	Domain      int
	ClockID     string
	Grandmaster string
	IsGM        bool
	Synced      bool // a port follows a time transmitter
	Priority    string
	GMPriority  string
	ClockClass  string
	GMClass     string
	TimeSource  string
	Offset      string
	MeanDelay   string
	Steps       int
	Ports       []ptpPortRow
	HasStats    bool
}

type ptpPageData struct {
	PageData
	Error     string
	Instances []ptpInstanceView
	Desc      map[string]string // field help, from the model
}

type ptpClockQuality struct {
	ClockClass string `json:"clock-class"`
}

type ptpWrapper struct {
	PTP struct {
		Instances struct {
			Instance []ptpOperInstance `json:"instance"`
		} `json:"instances"`
	} `json:"ieee1588-ptp-tt:ptp"`
}

type ptpOperInstance struct {
	Index     int `json:"instance-index"`
	DefaultDS struct {
		ClockIdentity string          `json:"clock-identity"`
		ClockQuality  ptpClockQuality `json:"clock-quality"`
		Priority1     int             `json:"priority1"`
		Priority2     int             `json:"priority2"`
		DomainNumber  int             `json:"domain-number"`
		InstanceType  string          `json:"instance-type"`
	} `json:"default-ds"`
	CurrentDS struct {
		StepsRemoved int    `json:"steps-removed"`
		Offset       string `json:"offset-from-time-transmitter"`
		MeanDelay    string `json:"mean-delay"`
	} `json:"current-ds"`
	ParentDS struct {
		GrandmasterIdentity string          `json:"grandmaster-identity"`
		GrandmasterQuality  ptpClockQuality `json:"grandmaster-clock-quality"`
		GrandmasterPrio1    int             `json:"grandmaster-priority1"`
		GrandmasterPrio2    int             `json:"grandmaster-priority2"`
	} `json:"parent-ds"`
	TimePropertiesDS struct {
		TimeSource string `json:"time-source"`
	} `json:"time-properties-ds"`
	Ports struct {
		Port []struct {
			Index     int    `json:"port-index"`
			Interface string `json:"underlying-interface"`
			PortDS    struct {
				State         string `json:"port-state"`
				MeanLinkDelay string `json:"mean-link-delay"`
				DelayMech     string `json:"delay-mechanism"`
			} `json:"port-ds"`
			Stats *struct {
				RxSync     uint64 `json:"rx-sync-count"`
				TxSync     uint64 `json:"tx-sync-count"`
				RxAnnounce uint64 `json:"rx-announce-count"`
				TxAnnounce uint64 `json:"tx-announce-count"`
				RxPDelay   uint64 `json:"rx-pdelay-req-count"`
				TxPDelay   uint64 `json:"tx-pdelay-req-count"`
			} `json:"ieee802-dot1as-gptp:port-statistics-ds"`
		} `json:"port"`
	} `json:"ports"`
}

// synced reports whether the instance is the grandmaster, or follows one.
func (in *ptpOperInstance) synced() (isGM, follows bool) {
	id := in.DefaultDS.ClockIdentity
	isGM = id != "" && id == in.ParentDS.GrandmasterIdentity
	for _, p := range in.Ports.Port {
		follows = follows || p.PortDS.State == "time-receiver"
	}
	return isGM, follows
}

// ptpNanos formats a time-interval, nanoseconds scaled by 2^16 in a
// string, as whole nanoseconds.
func ptpNanos(scaled string) string {
	v, err := strconv.ParseInt(scaled, 10, 64)
	if err != nil {
		return ""
	}
	return strconv.FormatInt(v>>16, 10) + " ns"
}

func ptpPair(a, b int) string {
	return strconv.Itoa(a) + " / " + strconv.Itoa(b)
}

func ptpCount(rx, tx uint64) string {
	return strconv.FormatUint(rx, 10) + " / " + strconv.FormatUint(tx, 10)
}

// Overview shows each PTP instance: who it follows, how far off it is,
// and what its ports do.  Operational data only.
func (h *PTPHandler) Overview(w http.ResponseWriter, r *http.Request) {
	data := ptpPageData{PageData: newPageData(w, r, "ptp", "PTP")}

	var types, states map[string]string
	if h.Schema != nil {
		mgr := h.Schema.Manager()
		types = schema.OptionDescriptions(schema.OptionsFor(mgr, ptpInstance+"/default-ds/instance-type"))
		states = schema.OptionDescriptions(schema.OptionsFor(mgr, ptpInstance+"/ports/port/port-ds/port-state"))
		data.Desc = map[string]string{
			"clock-identity": schema.DescriptionOf(mgr, ptpInstance+"/default-ds/clock-identity"),
			"grandmaster":    schema.DescriptionOf(mgr, ptpInstance+"/parent-ds/grandmaster-identity"),
			"priority":       schema.DescriptionOf(mgr, ptpInstance+"/default-ds/priority1"),
			"clock-class":    schema.DescriptionOf(mgr, ptpInstance+"/default-ds/clock-quality/clock-class"),
			"time-source":    schema.DescriptionOf(mgr, ptpInstance+"/time-properties-ds/time-source"),
			"offset":         schema.DescriptionOf(mgr, ptpInstance+"/current-ds/offset-from-time-transmitter"),
			"mean-delay":     schema.DescriptionOf(mgr, ptpInstance+"/current-ds/mean-delay"),
			"steps":          schema.DescriptionOf(mgr, ptpInstance+"/current-ds/steps-removed"),
			"delay":          schema.DescriptionOf(mgr, ptpInstance+"/ports/port/port-ds/delay-mechanism"),
			"link-delay":     schema.DescriptionOf(mgr, ptpInstance+"/ports/port/port-ds/mean-link-delay"),
		}
	}

	var raw ptpWrapper
	if err := h.RC.Get(r.Context(), "/data/ieee1588-ptp-tt:ptp", &raw); err != nil && !restconf.IsNotFound(err) {
		log.Printf("restconf ptp: %v", err)
		data.Error = "Failed to fetch PTP data"
	}
	for i := range raw.PTP.Instances.Instance {
		in := &raw.PTP.Instances.Instance[i]
		d := in.DefaultDS
		isGM, follows := in.synced()
		view := ptpInstanceView{
			Index:       in.Index,
			Type:        d.InstanceType,
			Domain:      d.DomainNumber,
			ClockID:     d.ClockIdentity,
			Grandmaster: in.ParentDS.GrandmasterIdentity,
			IsGM:        isGM,
			Synced:      follows,
			Priority:    ptpPair(d.Priority1, d.Priority2),
			GMPriority:  ptpPair(in.ParentDS.GrandmasterPrio1, in.ParentDS.GrandmasterPrio2),
			ClockClass:  schema.StripModulePrefix(d.ClockQuality.ClockClass),
			GMClass:     schema.StripModulePrefix(in.ParentDS.GrandmasterQuality.ClockClass),
			TimeSource:  schema.StripModulePrefix(in.TimePropertiesDS.TimeSource),
			Offset:      ptpNanos(in.CurrentDS.Offset),
			MeanDelay:   ptpNanos(in.CurrentDS.MeanDelay),
			Steps:       in.CurrentDS.StepsRemoved,
		}
		if label := types[d.InstanceType]; label != "" {
			view.Type = label
		}
		for _, p := range in.Ports.Port {
			state := p.PortDS.State
			row := ptpPortRow{
				Index:     p.Index,
				Interface: p.Interface,
				State:     state,
				StateHelp: states[state],
				Up:        state == "time-receiver" || state == "time-transmitter",
				Delay:     p.PortDS.DelayMech,
				LinkDelay: ptpNanos(p.PortDS.MeanLinkDelay),
			}
			if s := p.Stats; s != nil {
				row.HasStats, view.HasStats = true, true
				row.Sync = ptpCount(s.RxSync, s.TxSync)
				row.Announce = ptpCount(s.RxAnnounce, s.TxAnnounce)
				row.PDelay = ptpCount(s.RxPDelay, s.TxPDelay)
			}
			view.Ports = append(view.Ports, row)
		}
		sort.Slice(view.Ports, func(i, j int) bool { return view.Ports[i].Index < view.Ports[j].Index })
		data.Instances = append(data.Instances, view)
	}

	tmplName := "ptp.html"
	if r.Header.Get("HX-Request") == "true" {
		tmplName = "content"
	}
	if err := h.Template.ExecuteTemplate(w, tmplName, data); err != nil {
		log.Printf("template error: %v", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
	}
}
