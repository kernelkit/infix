// SPDX-License-Identifier: MIT

package handlers

import (
	"html/template"
	"log"
	"net/http"
	"sort"
	"strings"

	"github.com/kernelkit/webui/internal/restconf"
)

// RESTCONF JSON structures for infix-firewall:firewall.

type firewallWrapper struct {
	Firewall firewallJSON `json:"infix-firewall:firewall"`
}

type firewallJSON struct {
	Enabled  *yangBool    `json:"enabled"` // YANG default: true; nil means enabled
	Default  string       `json:"default"`
	Logging  string       `json:"logging"`
	Lockdown yangBool     `json:"lockdown"`
	Zone     []zoneJSON   `json:"zone"`
	Policy   []policyJSON `json:"policy"`
}

type zoneJSON struct {
	Name        string            `json:"name"`
	Action      string            `json:"action"`
	Interface   []string          `json:"interface"`
	Network     []string          `json:"network"`
	Service     []string          `json:"service"`
	PortForward []portForwardJSON `json:"port-forward"`
	Immutable   bool              `json:"immutable"`
}

type portForwardJSON struct {
	Port     string `json:"port"`
	Protocol string `json:"protocol"`
	ToAddr   string `json:"to-addr"`
	ToPort   string `json:"to-port"`
}

type policyJSON struct {
	Name       string    `json:"name"`
	Action     string    `json:"action"`
	Priority   yangInt64 `json:"priority"`
	Ingress    []string  `json:"ingress"`
	Egress     []string  `json:"egress"`
	Service    []string  `json:"service"`
	Masquerade bool      `json:"masquerade"`
	Immutable  bool      `json:"immutable"`
}

// Template data structures.

type firewallData struct {
	PageData
	Enabled bool
	EnabledText  string
	DefaultZone  string
	Lockdown     bool
	Logging      string
	ZoneNames    []string
	Matrix       []matrixRow
	Zones        []zoneEntry
	Policies     []policyEntry
	Error        string
}

type matrixRow struct {
	Zone  string
	Cells []matrixCell
}

type matrixCell struct {
	Class  string
	Symbol string
}

type zoneEntry struct {
	Name       string
	Action     string
	Interfaces string
	Networks   string
	Services   string
}

type policyEntry struct {
	Name       string
	Action     string
	Priority   int64
	Ingress    string
	Egress     string
	Services   string
	Masquerade bool
}

// FirewallHandler serves the firewall overview page.
type FirewallHandler struct {
	Template *template.Template
	RC       *restconf.Client
}

// Overview renders the firewall overview (GET /firewall).
func (h *FirewallHandler) Overview(w http.ResponseWriter, r *http.Request) {
	data := firewallData{
		PageData: newPageData(r, "firewall", "Firewall"),
	}

	var fw firewallWrapper
	if err := h.RC.Get(r.Context(), "/data/infix-firewall:firewall", &fw); err != nil {
		log.Printf("restconf firewall: %v", err)
		data.Error = "Could not fetch firewall configuration"
	} else {
		f := fw.Firewall
		data.Enabled = f.Enabled == nil || bool(*f.Enabled)
		if data.Enabled {
			data.EnabledText = "Active"
		} else {
			data.EnabledText = "Inactive"
		}
		data.DefaultZone = f.Default
		data.Lockdown = bool(f.Lockdown)
		data.Logging = f.Logging
		if data.Logging == "" {
			data.Logging = "off"
		}

		for _, z := range f.Zone {
			data.Zones = append(data.Zones, zoneEntry{
				Name:       z.Name,
				Action:     z.Action,
				Interfaces: strings.Join(z.Interface, ", "),
				Networks:   strings.Join(z.Network, ", "),
				Services:   strings.Join(z.Service, ", "),
			})
		}

		for _, p := range f.Policy {
			data.Policies = append(data.Policies, policyEntry{
				Name:       p.Name,
				Action:     p.Action,
				Priority:   int64(p.Priority),
				Ingress:    strings.Join(p.Ingress, ", "),
				Egress:     strings.Join(p.Egress, ", "),
				Services:   strings.Join(p.Service, ", "),
				Masquerade: p.Masquerade,
			})
		}

		sort.Slice(data.Policies, func(i, j int) bool {
			return data.Policies[i].Priority < data.Policies[j].Priority
		})

		data.ZoneNames, data.Matrix = buildMatrix(f.Zone, f.Policy)
	}

	tmplName := "firewall.html"
	if r.Header.Get("HX-Request") == "true" {
		tmplName = "content"
	}
	if err := h.Template.ExecuteTemplate(w, tmplName, data); err != nil {
		log.Printf("template error: %v", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
	}
}

// buildMatrix creates the zone-to-zone traffic flow matrix.
// Zones are listed along both axes with HOST (the device itself) prepended.
// Each cell shows whether traffic from the row zone to the column zone
// is allowed, denied, or conditional.
func buildMatrix(zones []zoneJSON, policies []policyJSON) ([]string, []matrixRow) {
	if len(zones) == 0 {
		return nil, nil
	}

	names := []string{"HOST"}
	zoneAction := map[string]string{}
	for _, z := range zones {
		names = append(names, z.Name)
		zoneAction[z.Name] = z.Action
	}

	// Sort policies by priority for evaluation.
	sorted := make([]policyJSON, len(policies))
	copy(sorted, policies)
	sort.Slice(sorted, func(i, j int) bool {
		return int64(sorted[i].Priority) < int64(sorted[j].Priority)
	})

	rows := make([]matrixRow, len(names))
	for i, src := range names {
		rows[i] = matrixRow{Zone: src, Cells: make([]matrixCell, len(names))}
		for j, dst := range names {
			switch {
			case src == "HOST" && dst == "HOST":
				rows[i].Cells[j] = matrixCell{Class: "matrix-self", Symbol: "\u2014"}
			case src == "HOST":
				// Traffic originating from the device is always allowed.
				rows[i].Cells[j] = matrixCell{Class: "matrix-allow", Symbol: "\u2713"}
			case src == dst:
				rows[i].Cells[j] = matrixCell{Class: "matrix-self", Symbol: "\u2014"}
			case dst == "HOST":
				// Input to device: governed by zone action + policies.
				rows[i].Cells[j] = zoneToHost(src, zoneAction, sorted)
			default:
				// Forwarding between zones: governed by policies.
				rows[i].Cells[j] = evalForward(src, dst, sorted)
			}
		}
	}

	return names, rows
}

// zoneToHost determines traffic flow from a zone to the device (HOST).
// Policies are checked first; if none gives a verdict, the zone's
// default action is used.
func zoneToHost(zone string, zoneAction map[string]string, policies []policyJSON) matrixCell {
	if v := evalPolicies(zone, "HOST", policies); v != "" {
		return makeCell(v)
	}
	if zoneAction[zone] == "accept" {
		return matrixCell{Class: "matrix-allow", Symbol: "\u2713"}
	}
	return matrixCell{Class: "matrix-deny", Symbol: "\u2717"}
}

// evalForward determines traffic flow between two different zones.
func evalForward(src, dst string, policies []policyJSON) matrixCell {
	if v := evalPolicies(src, dst, policies); v != "" {
		return makeCell(v)
	}
	return matrixCell{Class: "matrix-deny", Symbol: "\u2717"}
}

// evalPolicies walks the sorted policy list and returns the first terminal
// verdict (accept/reject/drop) for traffic from src to dst.
// "continue" policies are skipped (they don't produce a final verdict).
func evalPolicies(src, dst string, policies []policyJSON) string {
	for _, p := range policies {
		if !matchesZone(src, p.Ingress) || !matchesZone(dst, p.Egress) {
			continue
		}
		if p.Action == "continue" {
			continue
		}
		return p.Action
	}
	return ""
}

// matchesZone checks whether zone appears in list, treating "ANY" as a wildcard.
func matchesZone(zone string, list []string) bool {
	for _, z := range list {
		if z == zone || z == "ANY" {
			return true
		}
	}
	return false
}

func makeCell(verdict string) matrixCell {
	if verdict == "accept" {
		return matrixCell{Class: "matrix-allow", Symbol: "\u2713"}
	}
	return matrixCell{Class: "matrix-deny", Symbol: "\u2717"}
}
