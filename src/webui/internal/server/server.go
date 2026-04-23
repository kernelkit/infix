// SPDX-License-Identifier: MIT

package server

import (
	"context"
	"html/template"
	"io/fs"
	"net/http"

	"github.com/kernelkit/webui/internal/auth"
	"github.com/kernelkit/webui/internal/handlers"
	"github.com/kernelkit/webui/internal/restconf"
	"github.com/kernelkit/webui/internal/schema"
)

// New creates a fully wired http.Handler with all routes and middleware.
func New(
	store *auth.SessionStore,
	rc *restconf.Client,
	schemaCache *schema.Cache,
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
	ksTmpl, err := template.ParseFS(templateFS, "layouts/*.html", "fragments/configure-toolbar.html", "pages/configure-keystore.html")
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
	mdnsTmpl, err := template.ParseFS(templateFS, "layouts/*.html", "pages/mdns.html")
	if err != nil {
		return nil, err
	}
	nacmTmpl, err := template.ParseFS(templateFS, "layouts/*.html", "pages/nacm.html")
	if err != nil {
		return nil, err
	}
	servicesTmpl, err := template.ParseFS(templateFS, "layouts/*.html", "pages/services.html")
	if err != nil {
		return nil, err
	}
	containersTmpl, err := template.ParseFS(templateFS, "layouts/*.html", "pages/containers.html")
	if err != nil {
		return nil, err
	}
	cfgSysTmpl, err := template.ParseFS(templateFS, "layouts/*.html", "fragments/configure-toolbar.html", "pages/configure-system.html")
	if err != nil {
		return nil, err
	}
	cfgUsersTmpl, err := template.ParseFS(templateFS, "layouts/*.html", "fragments/configure-toolbar.html", "pages/configure-users.html")
	if err != nil {
		return nil, err
	}
	yangTreeTmpl, err := template.ParseFS(templateFS,
		"layouts/*.html",
		"fragments/configure-toolbar.html",
		"fragments/yang-tree-node.html",
		"pages/yang-tree.html")
	if err != nil {
		return nil, err
	}
	yangFuncs := template.FuncMap{"stripPrefix": schema.StripModulePrefix}
	yangFragTmpl, err := template.New("frag").Funcs(yangFuncs).ParseFS(templateFS,
		"fragments/yang-tree-node.html",
		"fragments/yang-node-detail.html",
		"fragments/yang-leaf-group.html",
		"fragments/yang-list-table.html")
	if err != nil {
		return nil, err
	}

	login := &auth.LoginHandler{
		Store:    store,
		RC:       rc,
		Template: loginTmpl,
		OnLogin: func(ctx context.Context) {
			schemaCache.RefreshBackground(ctx)
		},
	}

	dash := &handlers.DashboardHandler{
		Template: dashTmpl,
		RC:       rc,
	}

	fw := &handlers.FirewallHandler{
		Template: fwTmpl,
		RC:       rc,
	}

	cfgKs := &handlers.ConfigureKeystoreHandler{
		Template: ksTmpl,
		RC:       rc,
		Schema:   schemaCache,
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
	mdns := &handlers.MDNSHandler{Template: mdnsTmpl, RC: rc}
	nacm := &handlers.NACMHandler{Template: nacmTmpl, RC: rc}
	services := &handlers.ServicesHandler{Template: servicesTmpl, RC: rc}
	containers := &handlers.ContainersHandler{Template: containersTmpl, RC: rc}
	cfg := &handlers.ConfigureHandler{RC: rc}
	cfgSys := &handlers.ConfigureSystemHandler{Template: cfgSysTmpl, RC: rc, Schema: schemaCache}
	cfgUsers := &handlers.ConfigureUsersHandler{Template: cfgUsersTmpl, RC: rc, Schema: schemaCache}
	schemaH := &handlers.SchemaHandler{Cache: schemaCache}
	dataH := &handlers.DataHandler{RC: rc, Schema: schemaCache}
	treeH := &handlers.TreeHandler{
		Cache:    schemaCache,
		RC:       rc,
		PageTmpl: yangTreeTmpl,
		FragTmpl: yangFragTmpl,
	}

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
	mux.HandleFunc("GET /keystore", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/configure/keystore", http.StatusMovedPermanently)
	})
	mux.HandleFunc("GET /firmware", sys.Firmware)
	mux.HandleFunc("GET /firmware/progress", sys.FirmwareProgress)
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
	mux.HandleFunc("GET /mdns", mdns.Overview)
	mux.HandleFunc("GET /nacm", nacm.Overview)
	mux.HandleFunc("GET /services", services.Overview)
	mux.HandleFunc("GET /containers", containers.Overview)

	// Configure routes.
	mux.HandleFunc("POST /configure/enter",          cfg.Enter)
	mux.HandleFunc("POST /configure/apply",          cfg.Apply)
	mux.HandleFunc("POST /configure/apply-and-save", cfg.ApplyAndSave)
	mux.HandleFunc("POST /configure/abort",          cfg.Abort)
	mux.HandleFunc("POST /configure/save",           cfg.Save)
	mux.HandleFunc("GET /configure/system",           cfgSys.Overview)
	mux.HandleFunc("POST /configure/system/identity",     cfgSys.SaveIdentity)
	mux.HandleFunc("POST /configure/system/clock",        cfgSys.SaveClock)
	mux.HandleFunc("PUT /configure/system/ntp",           cfgSys.SaveNTP)
	mux.HandleFunc("PUT /configure/system/dns",           cfgSys.SaveDNS)
	mux.HandleFunc("POST /configure/system/preferences",  cfgSys.SavePreferences)
	mux.HandleFunc("GET /configure/users",                cfgUsers.Overview)
	mux.HandleFunc("POST /configure/users",               cfgUsers.AddUser)
	mux.HandleFunc("DELETE /configure/users/{name}",      cfgUsers.DeleteUser)
	mux.HandleFunc("POST /configure/users/{name}/shell",    cfgUsers.UpdateShell)
	mux.HandleFunc("POST /configure/users/{name}/password", cfgUsers.ChangePassword)
	mux.HandleFunc("POST /configure/users/{name}/keys",     cfgUsers.AddKey)
	mux.HandleFunc("DELETE /configure/users/{name}/keys/{keyname}", cfgUsers.DeleteKey)
	mux.HandleFunc("GET /configure/keystore",                              cfgKs.Overview)
	mux.HandleFunc("POST /configure/keystore/symmetric",                   cfgKs.AddSymKey)
	mux.HandleFunc("POST /configure/keystore/symmetric/{name}",            cfgKs.UpdateSymKey)
	mux.HandleFunc("DELETE /configure/keystore/symmetric/{name}",          cfgKs.DeleteSymKey)
	mux.HandleFunc("POST /configure/keystore/asymmetric",                  cfgKs.AddAsymKey)
	mux.HandleFunc("POST /configure/keystore/asymmetric/{name}",           cfgKs.UpdateAsymKey)
	mux.HandleFunc("DELETE /configure/keystore/asymmetric/{name}",         cfgKs.DeleteAsymKey)
	mux.HandleFunc("POST /configure/keystore/asymmetric/{name}/certs",              cfgKs.AddCert)
	mux.HandleFunc("POST /configure/keystore/asymmetric/{name}/certs/{certname}",  cfgKs.UpdateCert)
	mux.HandleFunc("DELETE /configure/keystore/asymmetric/{name}/certs/{certname}", cfgKs.DeleteCert)

	// Schema API routes (authenticated).
	mux.HandleFunc("GET /api/schema",          schemaH.Schema)
	mux.HandleFunc("GET /api/schema/children", schemaH.Children)

	// Data API route (authenticated) — raw RESTCONF JSON passthrough.
	mux.HandleFunc("GET /api/data", dataH.Get)

	// YANG tree UI routes (authenticated).
	mux.HandleFunc("GET /configure/tree",           treeH.Overview)
	mux.HandleFunc("GET /configure/tree/children",  treeH.TreeChildren)
	mux.HandleFunc("GET /configure/tree/node",      treeH.TreeNode)
	mux.HandleFunc("PUT /configure/tree/node",      treeH.SaveLeaf)
	mux.HandleFunc("DELETE /configure/tree/node",   treeH.DeleteLeaf)
	mux.HandleFunc("PUT /configure/tree/group",      treeH.SaveGroup)
	mux.HandleFunc("GET /configure/tree/list-add",   treeH.AddListRowForm)
	mux.HandleFunc("POST /configure/tree/list-row",  treeH.SaveListRow)
	mux.HandleFunc("DELETE /configure/tree/list-row", treeH.DeleteListRow)

	handler := authMiddleware(store, mux)
	handler = csrfMiddleware(handler)
	handler = securityHeadersMiddleware(handler)
	return handler, nil
}
