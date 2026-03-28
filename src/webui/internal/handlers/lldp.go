// SPDX-License-Identifier: MIT

package handlers

import (
	"context"
	"encoding/json"
	"html/template"
	"log"
	"net/http"
	"strings"

	"github.com/kernelkit/webui/internal/restconf"
)

// ─── LLDP types ──────────────────────────────────────────────────────────────

// LLDPNeighbor is a remote system seen via LLDP.
type LLDPNeighbor struct {
	LocalPort    string
	ChassisID    string
	SystemName   string
	PortID       string
	PortDesc     string
	SystemDesc   string
	Capabilities string // comma-separated
	MgmtAddress  string
}

// ─── Page data ───────────────────────────────────────────────────────────────

type lldpPageData struct {
	PageData
	Neighbors []LLDPNeighbor
	Error     string
}

// ─── Handler ─────────────────────────────────────────────────────────────────

// LLDPHandler serves the LLDP neighbors page.
type LLDPHandler struct {
	Template *template.Template
	RC       *restconf.Client
}

// Overview renders the LLDP page (GET /lldp).
func (h *LLDPHandler) Overview(w http.ResponseWriter, r *http.Request) {
	data := lldpPageData{
		PageData: newPageData(r, "lldp", "LLDP Neighbors"),
	}

	ctx := context.WithoutCancel(r.Context())

	var raw struct {
		LLDP struct {
			Port []struct {
				Name              string `json:"name"`
				DestMACAddress    string `json:"dest-mac-address"`
				RemoteSystemsData []struct {
					ChassisID                 string          `json:"chassis-id"`
					PortID                    string          `json:"port-id"`
					PortDesc                  string          `json:"port-desc"`
					SystemName                string          `json:"system-name"`
					SystemDescription         string          `json:"system-description"`
					SystemCapabilitiesEnabled json.RawMessage `json:"system-capabilities-enabled"`
					ManagementAddress         []struct {
						Address string `json:"address"`
					} `json:"management-address"`
				} `json:"remote-systems-data"`
			} `json:"port"`
		} `json:"ieee802-dot1ab-lldp:lldp"`
	}
	if err := h.RC.Get(ctx, "/data/ieee802-dot1ab-lldp:lldp", &raw); err != nil {
		log.Printf("restconf lldp: %v", err)
		data.Error = "Failed to fetch LLDP data"
	} else {
		for _, port := range raw.LLDP.Port {
			for _, rs := range port.RemoteSystemsData {
				mgmt := ""
				if len(rs.ManagementAddress) > 0 {
					mgmt = rs.ManagementAddress[0].Address
				}
				data.Neighbors = append(data.Neighbors, LLDPNeighbor{
					LocalPort:    port.Name,
					ChassisID:    rs.ChassisID,
					SystemName:   rs.SystemName,
					PortID:       rs.PortID,
					PortDesc:     rs.PortDesc,
					SystemDesc:   rs.SystemDescription,
					Capabilities: parseLLDPCapabilities(rs.SystemCapabilitiesEnabled),
					MgmtAddress:  mgmt,
				})
			}
		}
	}

	tmplName := "lldp.html"
	if r.Header.Get("HX-Request") == "true" {
		tmplName = "content"
	}
	if err := h.Template.ExecuteTemplate(w, tmplName, data); err != nil {
		log.Printf("template error: %v", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
	}
}

// ─── Helpers ─────────────────────────────────────────────────────────────────

// parseLLDPCapabilities turns the YANG system-capabilities-enabled bits
// value into a readable comma-separated string.
func parseLLDPCapabilities(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	// Try plain string first (some implementations encode as "bridge router")
	var s string
	if json.Unmarshal(raw, &s) == nil {
		parts := strings.Fields(s)
		return strings.Join(parts, ", ")
	}
	// Try array of strings
	var arr []string
	if json.Unmarshal(raw, &arr) == nil {
		return strings.Join(arr, ", ")
	}
	// Fallback: return raw minus braces
	trimmed := strings.TrimSpace(string(raw))
	if trimmed == "{}" || trimmed == "null" || trimmed == "[]" {
		return ""
	}
	return trimmed
}
