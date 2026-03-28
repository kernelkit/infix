// SPDX-License-Identifier: MIT

package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"html/template"
	"log"
	"net/http"
	"strings"

	"github.com/kernelkit/webui/internal/restconf"
)

// ─── NTP types ───────────────────────────────────────────────────────────────

// NTPAssoc is a single NTP association/peer.
type NTPAssoc struct {
	Address string
	Stratum int
	RefID   string
	Reach   string // octal string
	Poll    int
	Offset  string // ms
	Delay   string // ms
}

// NTPData is the parsed NTP state.
type NTPData struct {
	Synchronized bool
	Stratum      int
	RefID        string
	Offset       string // ms
	RootDelay    string // ms
	Associations []NTPAssoc
}

// ─── Page data ───────────────────────────────────────────────────────────────

type ntpPageData struct {
	CsrfToken    string
	PageTitle    string
	ActivePage   string
	Capabilities *Capabilities
	NTP          *NTPData
	Error        string
}

// ─── Handler ─────────────────────────────────────────────────────────────────

// NTPHandler serves the NTP status page.
type NTPHandler struct {
	Template *template.Template
	RC       *restconf.Client
}

// Overview renders the NTP page (GET /ntp).
func (h *NTPHandler) Overview(w http.ResponseWriter, r *http.Request) {
	data := ntpPageData{
		CsrfToken:    csrfToken(r.Context()),
		PageTitle:    "NTP",
		ActivePage:   "ntp",
		Capabilities: CapabilitiesFromContext(r.Context()),
	}

	ctx := context.WithoutCancel(r.Context())

	var raw struct {
		NTP struct {
			ClockState struct {
				SystemStatus struct {
					ClockState   string          `json:"clock-state"`
					ClockStratum int             `json:"clock-stratum"`
					ClockRefID   json.RawMessage `json:"clock-refid"`
					ClockOffset  yangFloat64     `json:"clock-offset"`
					RootDelay    yangFloat64     `json:"root-delay"`
				} `json:"system-status"`
			} `json:"clock-state"`
			Associations struct {
				Association []struct {
					Address string          `json:"address"`
					Stratum int             `json:"stratum"`
					RefID   json.RawMessage `json:"refid"`
					Reach   uint8           `json:"reach"`
					Poll    int             `json:"poll"`
					Offset  yangFloat64     `json:"offset"`
					Delay   yangFloat64     `json:"delay"`
				} `json:"association"`
			} `json:"associations"`
		} `json:"ietf-ntp:ntp"`
	}
	if err := h.RC.Get(ctx, "/data/ietf-ntp:ntp", &raw); err != nil {
		log.Printf("restconf ntp: %v", err)
		data.Error = "Failed to fetch NTP data"
	} else {
		ss := raw.NTP.ClockState.SystemStatus
		synced := strings.Contains(ss.ClockState, "synchronized") &&
			!strings.Contains(ss.ClockState, "unsynchronized")
		ntp := &NTPData{
			Synchronized: synced,
			Stratum:      ss.ClockStratum,
			RefID:        rawJSONString(ss.ClockRefID),
			Offset:       fmt.Sprintf("%.3f ms", float64(ss.ClockOffset)),
			RootDelay:    fmt.Sprintf("%.3f ms", float64(ss.RootDelay)),
		}
		for _, a := range raw.NTP.Associations.Association {
			ntp.Associations = append(ntp.Associations, NTPAssoc{
				Address: a.Address,
				Stratum: a.Stratum,
				RefID:   rawJSONString(a.RefID),
				Reach:   fmt.Sprintf("%o", a.Reach),
				Poll:    a.Poll,
				Offset:  fmt.Sprintf("%.3f ms", float64(a.Offset)),
				Delay:   fmt.Sprintf("%.3f ms", float64(a.Delay)),
			})
		}
		data.NTP = ntp
	}

	tmplName := "ntp.html"
	if r.Header.Get("HX-Request") == "true" {
		tmplName = "content"
	}
	if err := h.Template.ExecuteTemplate(w, tmplName, data); err != nil {
		log.Printf("template error: %v", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
	}
}

// ─── Helpers ─────────────────────────────────────────────────────────────────

// rawJSONString extracts the unquoted string value from a JSON raw message
// that may be a string, number, or other scalar.
func rawJSONString(b json.RawMessage) string {
	if len(b) == 0 {
		return ""
	}
	// Try string
	var s string
	if json.Unmarshal(b, &s) == nil {
		return s
	}
	// Fall back to raw bytes (number, etc.)
	return strings.Trim(string(b), `"`)
}
