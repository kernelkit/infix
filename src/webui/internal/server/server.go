// SPDX-License-Identifier: MIT

package server

import (
	"html/template"
	"io/fs"
	"net/http"

	"github.com/kernelkit/webui/internal/auth"
	"github.com/kernelkit/webui/internal/handlers"
	"github.com/kernelkit/webui/internal/restconf"
)

// New creates a fully wired http.Handler with all routes and middleware.
func New(
	store *auth.SessionStore,
	rc *restconf.Client,
	templateFS fs.FS,
	staticFS fs.FS,
) (http.Handler, error) {
	// Parse templates per page so each can define its own "content" block
	// without collisions.
	loginTmpl, err := template.ParseFS(templateFS, "pages/login.html")
	if err != nil {
		return nil, err
	}
	dashTmpl, err := template.ParseFS(templateFS, "layouts/*.html", "pages/dashboard.html")
	if err != nil {
		return nil, err
	}
	fwTmpl, err := template.ParseFS(templateFS, "layouts/*.html", "pages/firewall.html")
	if err != nil {
		return nil, err
	}
	ksTmpl, err := template.ParseFS(templateFS, "layouts/*.html", "pages/keystore.html")
	if err != nil {
		return nil, err
	}
	ifTmpl, err := template.ParseFS(templateFS, "layouts/*.html", "pages/interfaces.html")
	if err != nil {
		return nil, err
	}
	ifDetailTmpl, err := template.ParseFS(templateFS, "layouts/*.html", "pages/iface-detail.html", "fragments/iface-counters.html")
	if err != nil {
		return nil, err
	}
	ifCountersTmpl, err := template.ParseFS(templateFS, "fragments/iface-counters.html")
	if err != nil {
		return nil, err
	}
	fwrTmpl, err := template.ParseFS(templateFS, "layouts/*.html", "pages/firmware.html")
	if err != nil {
		return nil, err
	}
	routingTmpl, err := template.ParseFS(templateFS, "layouts/*.html", "pages/routing.html")
	if err != nil {
		return nil, err
	}
	wifiTmpl, err := template.ParseFS(templateFS, "layouts/*.html", "pages/wifi.html")
	if err != nil {
		return nil, err
	}
	vpnTmpl, err := template.ParseFS(templateFS, "layouts/*.html", "pages/vpn.html")
	if err != nil {
		return nil, err
	}
	dhcpTmpl, err := template.ParseFS(templateFS, "layouts/*.html", "pages/dhcp.html")
	if err != nil {
		return nil, err
	}
	ntpTmpl, err := template.ParseFS(templateFS, "layouts/*.html", "pages/ntp.html")
	if err != nil {
		return nil, err
	}
	lldpTmpl, err := template.ParseFS(templateFS, "layouts/*.html", "pages/lldp.html")
	if err != nil {
		return nil, err
	}
	containersTmpl, err := template.ParseFS(templateFS, "layouts/*.html", "pages/containers.html")
	if err != nil {
		return nil, err
	}

	login := &auth.LoginHandler{
		Store:    store,
		RC:       rc,
		Template: loginTmpl,
	}

	dash := &handlers.DashboardHandler{
		Template: dashTmpl,
		RC:       rc,
	}

	fw := &handlers.FirewallHandler{
		Template: fwTmpl,
		RC:       rc,
	}

	ks := &handlers.KeystoreHandler{
		Template: ksTmpl,
		RC:       rc,
	}

	iface := &handlers.InterfacesHandler{
		Template:         ifTmpl,
		DetailTemplate:   ifDetailTmpl,
		CountersTemplate: ifCountersTmpl,
		RC:               rc,
	}

	sys := &handlers.SystemHandler{
		RC:       rc,
		Template: fwrTmpl,
	}

	routing := &handlers.RoutingHandler{Template: routingTmpl, RC: rc}
	wifi := &handlers.WiFiHandler{Template: wifiTmpl, RC: rc}
	vpn := &handlers.VPNHandler{Template: vpnTmpl, RC: rc}
	dhcp := &handlers.DHCPHandler{Template: dhcpTmpl, RC: rc}
	ntp := &handlers.NTPHandler{Template: ntpTmpl, RC: rc}
	lldp := &handlers.LLDPHandler{Template: lldpTmpl, RC: rc}
	containers := &handlers.ContainersHandler{Template: containersTmpl, RC: rc}

	mux := http.NewServeMux()

	// Auth routes (public).
	mux.HandleFunc("GET /login", login.ShowLogin)
	mux.HandleFunc("POST /login", login.DoLogin)
	mux.HandleFunc("POST /logout", login.DoLogout)

	// Static assets (public).
	staticServer := http.FileServerFS(staticFS)
	mux.Handle("GET /assets/", http.StripPrefix("/assets/", staticServer))

	// Authenticated routes.
	mux.HandleFunc("GET /{$}", dash.Index)
	mux.HandleFunc("GET /interfaces", iface.Overview)
	mux.HandleFunc("GET /interfaces/{name}", iface.Detail)
	mux.HandleFunc("GET /interfaces/{name}/counters", iface.Counters)
	mux.HandleFunc("GET /firewall", fw.Overview)
	mux.HandleFunc("GET /keystore", ks.Overview)
	mux.HandleFunc("GET /firmware", sys.Firmware)
	mux.HandleFunc("POST /firmware/install", sys.FirmwareInstall)
	mux.HandleFunc("POST /reboot", sys.Reboot)
	mux.HandleFunc("GET /device-status", sys.DeviceStatus)
	mux.HandleFunc("GET /config", sys.DownloadConfig)
	mux.HandleFunc("GET /routing", routing.Overview)
	mux.HandleFunc("GET /wifi", wifi.Overview)
	mux.HandleFunc("GET /vpn", vpn.Overview)
	mux.HandleFunc("GET /dhcp", dhcp.Overview)
	mux.HandleFunc("GET /ntp", ntp.Overview)
	mux.HandleFunc("GET /lldp", lldp.Overview)
	mux.HandleFunc("GET /containers", containers.Overview)

	handler := authMiddleware(store, mux)
	handler = csrfMiddleware(handler)
	handler = securityHeadersMiddleware(handler)
	return handler, nil
}
