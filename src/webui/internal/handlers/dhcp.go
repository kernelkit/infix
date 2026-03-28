// SPDX-License-Identifier: MIT

package handlers

import (
	"context"
	"fmt"
	"html/template"
	"log"
	"net/http"
	"time"

	"github.com/kernelkit/webui/internal/restconf"
)

// ─── DHCP types ──────────────────────────────────────────────────────────────

// DHCPLease is a single active DHCP lease.
type DHCPLease struct {
	Address  string
	MAC      string
	Hostname string
	Expires  string // relative or "never"
	ClientID string
}

// DHCPStats holds DHCP packet counters.
type DHCPStats struct {
	InDiscoveries int64
	InRequests    int64
	InReleases    int64
	OutOffers     int64
	OutAcks       int64
	OutNaks       int64
}

// DHCPData is the parsed DHCP server state.
type DHCPData struct {
	Enabled bool
	Leases  []DHCPLease
	Stats   DHCPStats
}

// ─── Page data ───────────────────────────────────────────────────────────────

type dhcpPageData struct {
	CsrfToken    string
	PageTitle    string
	ActivePage   string
	Capabilities *Capabilities
	DHCP         *DHCPData
	Error        string
}

// ─── Handler ─────────────────────────────────────────────────────────────────

// DHCPHandler serves the DHCP status page.
type DHCPHandler struct {
	Template *template.Template
	RC       *restconf.Client
}

// Overview renders the DHCP page (GET /dhcp).
func (h *DHCPHandler) Overview(w http.ResponseWriter, r *http.Request) {
	data := dhcpPageData{
		CsrfToken:    csrfToken(r.Context()),
		PageTitle:    "DHCP Server",
		ActivePage:   "dhcp",
		Capabilities: CapabilitiesFromContext(r.Context()),
	}

	ctx := context.WithoutCancel(r.Context())

	var raw struct {
		DHCP struct {
			Enabled yangBool `json:"enabled"`
			Leases  struct {
				Lease []struct {
					Address  string `json:"address"`
					PhysAddr string `json:"phys-address"`
					Hostname string `json:"hostname"`
					Expires  string `json:"expires"`
					ClientID string `json:"client-id"`
				} `json:"lease"`
			} `json:"leases"`
			Statistics struct {
				OutOffers     yangInt64 `json:"out-offers"`
				OutAcks       yangInt64 `json:"out-acks"`
				OutNaks       yangInt64 `json:"out-naks"`
				InDiscoveries yangInt64 `json:"in-discovers"`
				InRequests    yangInt64 `json:"in-requests"`
				InReleases    yangInt64 `json:"in-releases"`
			} `json:"statistics"`
		} `json:"infix-dhcp-server:dhcp-server"`
	}
	if err := h.RC.Get(ctx, "/data/infix-dhcp-server:dhcp-server", &raw); err != nil {
		log.Printf("restconf dhcp-server: %v", err)
		data.Error = "Failed to fetch DHCP data"
	} else {
		d := raw.DHCP
		dhcp := &DHCPData{
			Enabled: bool(d.Enabled),
			Stats: DHCPStats{
				InDiscoveries: int64(d.Statistics.InDiscoveries),
				InRequests:    int64(d.Statistics.InRequests),
				InReleases:    int64(d.Statistics.InReleases),
				OutOffers:     int64(d.Statistics.OutOffers),
				OutAcks:       int64(d.Statistics.OutAcks),
				OutNaks:       int64(d.Statistics.OutNaks),
			},
		}
		for _, l := range d.Leases.Lease {
			dhcp.Leases = append(dhcp.Leases, DHCPLease{
				Address:  l.Address,
				MAC:      l.PhysAddr,
				Hostname: l.Hostname,
				Expires:  formatDHCPExpiry(l.Expires),
				ClientID: l.ClientID,
			})
		}
		data.DHCP = dhcp
	}

	tmplName := "dhcp.html"
	if r.Header.Get("HX-Request") == "true" {
		tmplName = "content"
	}
	if err := h.Template.ExecuteTemplate(w, tmplName, data); err != nil {
		log.Printf("template error: %v", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
	}
}

// ─── Helpers ─────────────────────────────────────────────────────────────────

// formatDHCPExpiry converts a YANG date-and-time or "never" string to a
// human-readable relative expiry string.
func formatDHCPExpiry(s string) string {
	if s == "" || s == "never" {
		return "never"
	}
	t, err := time.Parse(time.RFC3339, s)
	if err != nil {
		// Try without timezone
		t, err = time.Parse("2006-01-02T15:04:05", s)
		if err != nil {
			return s
		}
	}
	d := time.Until(t)
	if d < 0 {
		d = -d
		return "expired " + formatRelDuration(d) + " ago"
	}
	return "in " + formatRelDuration(d)
}

// formatRelDuration formats a time.Duration in a compact human-readable form.
func formatRelDuration(d time.Duration) string {
	switch {
	case d >= 24*time.Hour:
		return fmt.Sprintf("%dd", int(d.Hours())/24)
	case d >= time.Hour:
		return fmt.Sprintf("%dh%dm", int(d.Hours()), int(d.Minutes())%60)
	case d >= time.Minute:
		return fmt.Sprintf("%dm", int(d.Minutes()))
	default:
		return fmt.Sprintf("%ds", int(d.Seconds()))
	}
}
