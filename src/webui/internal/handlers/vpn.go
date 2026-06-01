// SPDX-License-Identifier: MIT

package handlers

import (
	"context"
	"fmt"
	"html/template"
	"log"
	"net/http"
	"sync"
	"time"

	"github.com/kernelkit/webui/internal/restconf"
)

// wgConfigJSON is a local extension for fetching WireGuard configuration
// fields (like listen-port) that are not in the shared wireGuardJSON type.
type wgConfigJSON struct {
	ListenPort int `json:"listen-port"`
}

// wgIfaceConfigWrapper is used to fetch per-interface WireGuard config.
type wgIfaceConfigWrapper struct {
	WireGuard *wgConfigJSON `json:"infix-interfaces:wireguard"`
}

// WGPeer holds display-ready data for a single WireGuard peer.
type WGPeer struct {
	PublicKey      string
	PublicKeyShort string // first 8 chars + "..."
	Endpoint       string // "IP:port" or empty
	Status         string // "up" or "down"
	LastHandshake  string // relative time, e.g. "2 min ago" or "never"
	RxBytes        string // human-readable
	TxBytes        string // human-readable
}

// WGTunnel holds display-ready data for a single WireGuard interface.
type WGTunnel struct {
	Name       string
	ListenPort int
	Addresses  []string // IP addresses from ietf-ip
	OperStatus string
	Peers      []WGPeer
}

// vpnData is the template data struct for the VPN page.
type vpnData struct {
	PageData
	Tunnels []WGTunnel
	Error   string
}

// VPNHandler serves the VPN/WireGuard status page.
type VPNHandler struct {
	Template *template.Template
	RC       *restconf.Client
}

// Overview renders the VPN page (GET /vpn).
func (h *VPNHandler) Overview(w http.ResponseWriter, r *http.Request) {
	data := vpnData{
		PageData: newPageData(r, "vpn", "VPN"),
	}

	// Detach from the request context so that RESTCONF calls survive
	// browser connection resets (common during login redirects).
	ctx := context.WithoutCancel(r.Context())

	var ifaces interfacesWrapper
	if err := h.RC.Get(ctx, "/data/ietf-interfaces:interfaces", &ifaces); err != nil {
		log.Printf("restconf interfaces (vpn): %v", err)
		data.Error = "Could not fetch interface information"
		h.render(w, r, data)
		return
	}

	// Filter for WireGuard interfaces only.
	var wgIfaces []ifaceJSON
	for _, iface := range ifaces.Interfaces.Interface {
		if iface.Type == "infix-if-type:wireguard" {
			wgIfaces = append(wgIfaces, iface)
		}
	}

	if len(wgIfaces) == 0 {
		h.render(w, r, data)
		return
	}

	// Fetch per-interface WireGuard config concurrently.
	tunnels := make([]WGTunnel, len(wgIfaces))
	var wg sync.WaitGroup

	for i, iface := range wgIfaces {
		wg.Add(1)
		go func(idx int, iface ifaceJSON) {
			defer wg.Done()
			tunnels[idx] = buildWGTunnel(ctx, h.RC, iface)
		}(i, iface)
	}

	wg.Wait()
	data.Tunnels = tunnels
	h.render(w, r, data)
}

// buildWGTunnel constructs a WGTunnel from an interface and optional config fetch.
func buildWGTunnel(ctx context.Context, rc *restconf.Client, iface ifaceJSON) WGTunnel {
	tunnel := WGTunnel{
		Name:       iface.Name,
		OperStatus: iface.OperStatus,
	}

	// Collect IP addresses.
	if iface.IPv4 != nil {
		for _, a := range iface.IPv4.Address {
			tunnel.Addresses = append(tunnel.Addresses, fmt.Sprintf("%s/%d", a.IP, int(a.PrefixLength)))
		}
	}
	if iface.IPv6 != nil {
		for _, a := range iface.IPv6.Address {
			tunnel.Addresses = append(tunnel.Addresses, fmt.Sprintf("%s/%d", a.IP, int(a.PrefixLength)))
		}
	}

	// Fetch ListenPort from config endpoint (separate from oper-state).
	var cfgWrap wgIfaceConfigWrapper
	path := fmt.Sprintf("/data/ietf-interfaces:interfaces/interface=%s/infix-interfaces:wireguard", iface.Name)
	if err := rc.Get(ctx, path, &cfgWrap); err == nil && cfgWrap.WireGuard != nil {
		tunnel.ListenPort = cfgWrap.WireGuard.ListenPort
	}

	// Build peers from embedded peer-status.
	if iface.WireGuard != nil && iface.WireGuard.PeerStatus != nil {
		for _, p := range iface.WireGuard.PeerStatus.Peer {
			peer := WGPeer{
				PublicKey:      p.PublicKey,
				PublicKeyShort: shortKey(p.PublicKey),
				Status:         p.ConnectionStatus,
				LastHandshake:  relativeTime(p.LatestHandshake),
			}
			if p.EndpointAddress != "" {
				peer.Endpoint = fmt.Sprintf("%s:%d", p.EndpointAddress, p.EndpointPort)
			}
			if p.Transfer != nil {
				peer.RxBytes = humanBytes(int64(p.Transfer.RxBytes))
				peer.TxBytes = humanBytes(int64(p.Transfer.TxBytes))
			} else {
				peer.RxBytes = "0 B"
				peer.TxBytes = "0 B"
			}
			tunnel.Peers = append(tunnel.Peers, peer)
		}
	}

	return tunnel
}

// shortKey returns the first 8 characters of a WireGuard public key followed by "...".
func shortKey(key string) string {
	if len(key) <= 8 {
		return key
	}
	return key[:8] + "..."
}

// relativeTime converts an RFC3339 timestamp to a human-readable relative time string.
// Returns "never" if the timestamp is empty or cannot be parsed.
func relativeTime(ts string) string {
	if ts == "" {
		return "never"
	}
	t, err := time.Parse(time.RFC3339, ts)
	if err != nil {
		// Try RFC3339Nano as fallback.
		t, err = time.Parse(time.RFC3339Nano, ts)
		if err != nil {
			return "never"
		}
	}
	if t.IsZero() {
		return "never"
	}

	d := time.Since(t)
	switch {
	case d < 0:
		return "just now"
	case d < time.Minute:
		return fmt.Sprintf("%d sec ago", int(d.Seconds()))
	case d < time.Hour:
		mins := int(d.Minutes())
		if mins == 1 {
			return "1 min ago"
		}
		return fmt.Sprintf("%d min ago", mins)
	case d < 24*time.Hour:
		hours := int(d.Hours())
		if hours == 1 {
			return "1 hour ago"
		}
		return fmt.Sprintf("%d hours ago", hours)
	default:
		days := int(d.Hours()) / 24
		if days == 1 {
			return "1 day ago"
		}
		return fmt.Sprintf("%d days ago", days)
	}
}

// render executes the correct template based on whether it's an htmx request.
func (h *VPNHandler) render(w http.ResponseWriter, r *http.Request, data vpnData) {
	tmplName := "vpn.html"
	if r.Header.Get("HX-Request") == "true" {
		tmplName = "content"
	}
	if err := h.Template.ExecuteTemplate(w, tmplName, data); err != nil {
		log.Printf("template error (vpn): %v", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
	}
}
