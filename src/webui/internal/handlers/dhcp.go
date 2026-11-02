// SPDX-License-Identifier: MIT

package handlers

import (
	"context"
	"fmt"
	"html/template"
	"log"
	"net/http"
	"net/netip"
	"slices"
	"time"

	"infix/webui/internal/restconf"
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

// DHCPSubnet is a served subnet and its leases, in address order.
type DHCPSubnet struct {
	Subnet      string // empty for leases outside every configured subnet
	Description string
	Leases      []DHCPLease
}

// DHCPData is the parsed DHCP server state.
type DHCPData struct {
	Enabled bool
	Subnets []DHCPSubnet // only those with leases, in subnet order
	Stats   DHCPStats
}

// ─── Page data ───────────────────────────────────────────────────────────────

type dhcpPageData struct {
	PageData
	DHCP  *DHCPData
	Error string
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
		PageData: newPageData(w, r, "dhcp", "DHCP"),
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
	if err := h.RC.Get(ctx, "/data/infix-dhcp-server:dhcp-server", &raw); err != nil && !restconf.IsNotFound(err) {
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
		var conf struct {
			DHCP struct {
				Subnet []struct {
					Subnet      string `json:"subnet"`
					Description string `json:"description"`
				} `json:"subnet"`
			} `json:"infix-dhcp-server:dhcp-server"`
		}
		if err := h.RC.Get(ctx, "/ds/ietf-datastores:running/infix-dhcp-server:dhcp-server", &conf); err != nil && !restconf.IsNotFound(err) {
			log.Printf("restconf dhcp-server config: %v", err)
		}
		var subnets []DHCPSubnet
		for _, sn := range conf.DHCP.Subnet {
			subnets = append(subnets, DHCPSubnet{Subnet: sn.Subnet, Description: sn.Description})
		}
		var leases []DHCPLease
		for _, l := range d.Leases.Lease {
			leases = append(leases, DHCPLease{
				Address:  l.Address,
				MAC:      l.PhysAddr,
				Hostname: l.Hostname,
				Expires:  formatDHCPExpiry(l.Expires),
				ClientID: l.ClientID,
			})
		}
		dhcp.Subnets = groupDHCPLeases(subnets, leases)
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

// groupDHCPLeases puts each lease in the subnet that holds its address,
// or in a trailing group without a subnet, and sorts both by address.
func groupDHCPLeases(subnets []DHCPSubnet, leases []DHCPLease) []DHCPSubnet {
	prefix := func(s DHCPSubnet) netip.Prefix {
		p, _ := netip.ParsePrefix(s.Subnet)
		return p
	}
	addr := func(l DHCPLease) netip.Addr {
		a, _ := netip.ParseAddr(l.Address)
		return a
	}

	slices.SortFunc(subnets, func(a, b DHCPSubnet) int {
		return prefix(a).Addr().Compare(prefix(b).Addr())
	})
	slices.SortFunc(leases, func(a, b DHCPLease) int {
		return addr(a).Compare(addr(b))
	})

	other := DHCPSubnet{}
	for _, l := range leases {
		i := slices.IndexFunc(subnets, func(s DHCPSubnet) bool {
			return prefix(s).Contains(addr(l))
		})
		if i < 0 {
			other.Leases = append(other.Leases, l)
		} else {
			subnets[i].Leases = append(subnets[i].Leases, l)
		}
	}

	groups := slices.DeleteFunc(subnets, func(s DHCPSubnet) bool { return len(s.Leases) == 0 })
	if len(other.Leases) > 0 {
		groups = append(groups, other)
	}
	return groups
}

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
