// SPDX-License-Identifier: MIT

package handlers

import (
	"fmt"
	"html/template"
	"log"
	"net/http"
	"sort"
	"strings"

	"github.com/kernelkit/webui/internal/restconf"
)

// ─── RESTCONF JSON types ──────────────────────────────────────────────────────

type mdnsWrapper struct {
	MDNS mdnsJSON `json:"infix-services:mdns"`
}

type mdnsJSON struct {
	Enabled    *yangBool        `json:"enabled"`
	Domain     string           `json:"domain"`
	Hostname   string           `json:"hostname"`
	Interfaces mdnsIfacesJSON   `json:"interfaces"`
	Reflector  mdnsReflJSON     `json:"reflector"`
	Neighbors  mdnsNeighborsJSON `json:"neighbors"`
}

type mdnsIfacesJSON struct {
	Allow []string `json:"allow"`
	Deny  []string `json:"deny"`
}

type mdnsReflJSON struct {
	Enabled       *yangBool `json:"enabled"`
	ServiceFilter []string  `json:"service-filter"`
}

type mdnsNeighborsJSON struct {
	Neighbor []mdnsNeighborJSON `json:"neighbor"`
}

type mdnsNeighborJSON struct {
	Hostname string           `json:"hostname"`
	Address  []string         `json:"address"`
	LastSeen string           `json:"last-seen"`
	Service  []mdnsServiceJSON `json:"service"`
}

type mdnsServiceJSON struct {
	Name string   `json:"name"`
	Type string   `json:"type"`
	Port uint16   `json:"port"`
	Txt  []string `json:"txt"`
}

// ─── Template data ────────────────────────────────────────────────────────────

type mdnsPageData struct {
	PageData
	Enabled     bool
	EnabledText string
	Domain      string
	Allow       string
	Deny        string
	Reflector   bool
	SvcFilter   string
	Neighbors   []mdnsNeighborEntry
	Error       string
}

type mdnsNeighborEntry struct {
	Hostname    string
	PrimaryAddr string
	ExtraAddrs  []string // additional addresses shown in fold-out
	LastSeen    string   // HH:MM:SS
	Services    []mdnsSvcEntry
}

type mdnsSvcEntry struct {
	Label string        // "https", "ssh", etc.
	Port  uint16
	URL   template.URL  // non-empty for http/https — safe to use in href
}

// ─── Handler ─────────────────────────────────────────────────────────────────

// MDNSHandler serves the mDNS overview page.
type MDNSHandler struct {
	Template *template.Template
	RC       *restconf.Client
}

// Overview renders the mDNS page (GET /mdns).
func (h *MDNSHandler) Overview(w http.ResponseWriter, r *http.Request) {
	data := mdnsPageData{
		PageData: newPageData(r, "mdns", "mDNS"),
	}

	var raw mdnsWrapper
	if err := h.RC.Get(r.Context(), "/data/infix-services:mdns", &raw); err != nil {
		log.Printf("restconf mdns: %v", err)
		data.Error = "Could not fetch mDNS data"
	} else {
		m := raw.MDNS
		data.Enabled = m.Enabled == nil || bool(*m.Enabled)
		if data.Enabled {
			data.EnabledText = "Active"
		} else {
			data.EnabledText = "Inactive"
		}
		data.Domain = m.Domain
		if data.Domain == "" {
			data.Domain = "local"
		}
		data.Allow = strings.Join(m.Interfaces.Allow, ", ")
		data.Deny = strings.Join(m.Interfaces.Deny, ", ")
		data.Reflector = m.Reflector.Enabled != nil && bool(*m.Reflector.Enabled)
		data.SvcFilter = strings.Join(m.Reflector.ServiceFilter, ", ")

		for _, n := range m.Neighbors.Neighbor {
			data.Neighbors = append(data.Neighbors, buildMDNSNeighbor(n))
		}
		sort.Slice(data.Neighbors, func(i, j int) bool {
			return data.Neighbors[i].Hostname < data.Neighbors[j].Hostname
		})
	}

	tmplName := "mdns.html"
	if r.Header.Get("HX-Request") == "true" {
		tmplName = "content"
	}
	if err := h.Template.ExecuteTemplate(w, tmplName, data); err != nil {
		log.Printf("template error: %v", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
	}
}

// ─── Helpers ─────────────────────────────────────────────────────────────────

func buildMDNSNeighbor(n mdnsNeighborJSON) mdnsNeighborEntry {
	// mdnsSortAddrs puts IPv4 first, then global IPv6, then link-local — so
	// addrs[0] is already the best address for URL construction.
	addrs := mdnsSortAddrs(n.Address)

	primary := ""
	if len(addrs) > 0 {
		primary = addrs[0]
	}

	entry := mdnsNeighborEntry{
		Hostname:    n.Hostname,
		PrimaryAddr: primary,
		LastSeen:    mdnsLastSeen(n.LastSeen),
	}
	if len(addrs) > 1 {
		entry.ExtraAddrs = addrs[1:]
	}

	for _, svc := range n.Service {
		entry.Services = append(entry.Services, buildMDNSSvc(svc, primary))
	}
	return entry
}

func buildMDNSSvc(svc mdnsServiceJSON, addr string) mdnsSvcEntry {
	meta := mdnsParseTxt(svc.Txt)

	var rawURL string
	switch svc.Type {
	case "_http._tcp":
		if meta.adminurl != "" {
			rawURL = meta.adminurl
		} else {
			rawURL = fmt.Sprintf("http://%s:%d%s", addr, svc.Port, meta.path)
		}
	case "_https._tcp":
		if meta.adminurl != "" {
			rawURL = meta.adminurl
		} else {
			rawURL = fmt.Sprintf("https://%s:%d%s", addr, svc.Port, meta.path)
		}
	}

	// Use the DNS-SD instance name for clickable services so that two https
	// entries on different ports ("Infix NOS" vs "ttyd") are distinguishable.
	label := mdnsSvcLabel(svc.Type)
	if rawURL != "" && svc.Name != "" {
		label = svc.Name
	}

	return mdnsSvcEntry{
		Label: label,
		Port:  svc.Port,
		URL:   template.URL(rawURL), // #nosec G203 — URL built from trusted operational data
	}
}

// "_https._tcp" → "https", "_netconf-ssh._tcp" → "netconf-ssh"
func mdnsSvcLabel(stype string) string {
	s := strings.TrimPrefix(stype, "_")
	if idx := strings.Index(s, "._"); idx >= 0 {
		s = s[:idx]
	}
	return s
}

func mdnsSortAddrs(addrs []string) []string {
	out := make([]string, len(addrs))
	copy(out, addrs)
	sort.SliceStable(out, func(i, j int) bool {
		return mdnsAddrPrio(out[i]) < mdnsAddrPrio(out[j])
	})
	return out
}

func mdnsAddrPrio(a string) int {
	if !strings.Contains(a, ":") {
		return 0 // IPv4
	}
	if strings.HasPrefix(strings.ToLower(a), "fe80:") {
		return 2 // link-local IPv6
	}
	return 1 // global IPv6
}

func mdnsLastSeen(ts string) string {
	if ts == "" {
		return ""
	}
	parts := strings.SplitN(ts, "T", 2)
	if len(parts) < 2 {
		return ts
	}
	t := parts[1]
	if len(t) >= 8 {
		return t[:8]
	}
	return t
}

type mdnsTxtMeta struct {
	path     string
	adminurl string
}

func mdnsParseTxt(records []string) mdnsTxtMeta {
	var m mdnsTxtMeta
	for _, r := range records {
		switch {
		case m.path == "" && strings.HasPrefix(r, "path="):
			m.path = r[5:]
		case m.adminurl == "" && strings.HasPrefix(r, "adminurl="):
			m.adminurl = r[9:]
		}
		if m.path != "" && m.adminurl != "" {
			break
		}
	}
	return m
}
