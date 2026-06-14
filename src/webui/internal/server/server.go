// SPDX-License-Identifier: MIT

package server

import (
	"context"
	"fmt"
	"html/template"
	"io/fs"
	"net/http"

	"infix/webui/internal/auth"
	"infix/webui/internal/handlers"
	"infix/webui/internal/restconf"
	"infix/webui/internal/schema"
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
	loginTmpl, err := template.ParseFS(templateFS, "layouts/icons.html", "pages/login.html")
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
	swTmpl, err := template.ParseFS(templateFS, "layouts/*.html", "pages/software.html")
	if err != nil {
		return nil, err
	}
	sysCtrlTmpl, err := template.ParseFS(templateFS, "layouts/*.html", "pages/system-control.html")
	if err != nil {
		return nil, err
	}
	backupTmpl, err := template.ParseFS(templateFS, "layouts/*.html", "pages/backup.html")
	if err != nil {
		return nil, err
	}
	logsTmpl, err := template.ParseFS(templateFS, "layouts/*.html", "pages/logs.html")
	if err != nil {
		return nil, err
	}
	diagTmpl, err := template.ParseFS(templateFS, "layouts/*.html", "pages/diagnostics.html")
	if err != nil {
		return nil, err
	}
	routingTmpl, err := template.ParseFS(templateFS, "layouts/*.html", "pages/routing.html")
	if err != nil {
		return nil, err
	}
	hwTmpl, err := template.ParseFS(templateFS, "layouts/*.html", "pages/hardware.html")
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
	cfgNTPTmpl, err := template.ParseFS(templateFS, "layouts/*.html", "fragments/configure-toolbar.html", "pages/configure-ntp.html")
	if err != nil {
		return nil, err
	}
	cfgDNSTmpl, err := template.ParseFS(templateFS, "layouts/*.html", "fragments/configure-toolbar.html", "pages/configure-dns.html")
	if err != nil {
		return nil, err
	}
	cfgUsersTmpl, err := template.ParseFS(templateFS, "layouts/*.html", "fragments/configure-toolbar.html", "pages/configure-users.html")
	if err != nil {
		return nil, err
	}
	cfgRoutesTmpl, err := template.ParseFS(templateFS, "layouts/*.html", "fragments/configure-toolbar.html", "pages/configure-routes.html")
	if err != nil {
		return nil, err
	}
	cfgFwTmpl, err := template.ParseFS(templateFS, "layouts/*.html", "fragments/configure-toolbar.html", "pages/configure-firewall.html")
	if err != nil {
		return nil, err
	}
	cfgHwTmpl, err := template.ParseFS(templateFS, "layouts/*.html", "fragments/configure-toolbar.html", "pages/configure-hardware.html")
	if err != nil {
		return nil, err
	}
	ifFuncs := template.FuncMap{
		"shortPMD": handlers.ShortenPMD,
		"add":      func(a, b int) int { return a + b },
		"deref": func(v any) any {
			switch p := v.(type) {
			case *bool:
				if p != nil {
					return *p
				}
			case *uint32:
				if p != nil {
					return *p
				}
			case *int:
				if p != nil {
					return *p
				}
			}
			return nil
		},
		// dict lets callers pass keyed args to nested templates, e.g.
		// {{template "foo" (dict "Key" .X "Selected" "")}}.
		"dict": func(values ...any) (map[string]any, error) {
			if len(values)%2 != 0 {
				return nil, fmt.Errorf("dict: odd argument count")
			}
			m := make(map[string]any, len(values)/2)
			for i := 0; i < len(values); i += 2 {
				k, ok := values[i].(string)
				if !ok {
					return nil, fmt.Errorf("dict: non-string key at position %d", i)
				}
				m[k] = values[i+1]
			}
			return m, nil
		},
	}
	cfgIfTmpl, err := template.New("").Funcs(ifFuncs).ParseFS(templateFS, "layouts/*.html", "fragments/configure-toolbar.html", "fragments/wizard-psk-picker.html", "fragments/wizard-wgkey-picker.html", "fragments/wizard-radio-picker.html", "pages/configure-interfaces.html")
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
		"fragments/yang-list-table.html",
		"layouts/icons.html")
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
		Schema:           schemaCache,
	}

	sys := &handlers.SystemHandler{
		RC:          rc,
		Template:    swTmpl,
		SysCtrlTmpl: sysCtrlTmpl,
		BackupTmpl:  backupTmpl,
	}
	logs := &handlers.LogsHandler{Template: logsTmpl}
	diag := &handlers.DiagnosticsHandler{RC: rc, Template: diagTmpl}

	routing := &handlers.RoutingHandler{Template: routingTmpl, RC: rc}
	wifi := &handlers.WiFiHandler{Template: wifiTmpl, RC: rc}
	hw := &handlers.HardwareHandler{Template: hwTmpl, RC: rc}
	vpn := &handlers.VPNHandler{Template: vpnTmpl, RC: rc}
	dhcp := &handlers.DHCPHandler{Template: dhcpTmpl, RC: rc}
	ntp := &handlers.NTPHandler{Template: ntpTmpl, RC: rc}
	lldp := &handlers.LLDPHandler{Template: lldpTmpl, RC: rc}
	mdns := &handlers.MDNSHandler{Template: mdnsTmpl, RC: rc}
	nacm := &handlers.NACMHandler{Template: nacmTmpl, RC: rc}
	services := &handlers.ServicesHandler{Template: servicesTmpl, RC: rc}
	containers := &handlers.ContainersHandler{Template: containersTmpl, RC: rc}
	cfg := &handlers.ConfigureHandler{RC: rc}
	cfgSys := &handlers.ConfigureSystemHandler{
		Template:    cfgSysTmpl,
		NTPTemplate: cfgNTPTmpl,
		DNSTemplate: cfgDNSTmpl,
		RC:          rc,
		Schema:      schemaCache,
	}
	cfgUsers := &handlers.ConfigureUsersHandler{Template: cfgUsersTmpl, RC: rc, Schema: schemaCache}
	cfgRoutes := &handlers.ConfigureRoutesHandler{Template: cfgRoutesTmpl, RC: rc, Schema: schemaCache}
	cfgFw := &handlers.ConfigureFirewallHandler{Template: cfgFwTmpl, RC: rc, Schema: schemaCache}
	cfgHw := &handlers.ConfigureHardwareHandler{Template: cfgHwTmpl, RC: rc, Schema: schemaCache}
	cfgIf := &handlers.ConfigureInterfacesHandler{Template: cfgIfTmpl, RC: rc, Schema: schemaCache}
	schemaH := &handlers.SchemaHandler{Cache: schemaCache}
	dataH := &handlers.DataHandler{RC: rc, Schema: schemaCache}
	treeH := &handlers.TreeHandler{
		Cache:    schemaCache,
		RC:       rc,
		PageTmpl: yangTreeTmpl,
		FragTmpl: yangFragTmpl,
	}
	statusTreeH := &handlers.TreeHandler{
		Cache:    schemaCache,
		RC:       rc,
		PageTmpl: yangTreeTmpl,
		FragTmpl: yangFragTmpl,
		ReadOnly: true,
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
	mux.HandleFunc("GET /software", sys.Software)
	mux.HandleFunc("GET /software/progress", sys.SoftwareProgress)
	mux.HandleFunc("POST /software/install",     sys.SoftwareInstall)
	mux.HandleFunc("POST /software/upload",      sys.SoftwareUpload)
	mux.HandleFunc("POST /software/boot-order",  sys.SetBootOrder)
	mux.HandleFunc("POST /reboot", sys.Reboot) // kept for software page "Reboot to activate"
	mux.HandleFunc("GET /config", sys.DownloadConfig)
	mux.HandleFunc("GET /maintenance/logs",                    logs.Overview)
	mux.HandleFunc("GET /maintenance/logs/{name}",             logs.Fragment)
	mux.HandleFunc("GET /maintenance/logs/{name}/earlier",     logs.Earlier)
	mux.HandleFunc("GET /maintenance/logs/{name}/tail",        logs.Tail)
	mux.HandleFunc("GET /maintenance/logs/{name}/download",    logs.Download)
	mux.HandleFunc("GET /maintenance/diagnostics",             diag.Overview)
	mux.HandleFunc("GET /maintenance/diagnostics/run",         diag.Run)
	mux.HandleFunc("GET /maintenance/diagnostics/resolve",     diag.Resolve)
	mux.HandleFunc("GET /maintenance/backup",                  sys.Backup)
	mux.HandleFunc("POST /maintenance/backup/restore",         sys.RestoreConfig)
	mux.HandleFunc("POST /maintenance/support-bundle",         sys.SupportBundle)
	mux.HandleFunc("GET /maintenance/system",                  sys.SystemControl)
	mux.HandleFunc("POST /maintenance/system/reboot",          sys.Reboot)
	mux.HandleFunc("POST /maintenance/system/shutdown",        sys.Shutdown)
	mux.HandleFunc("POST /maintenance/system/factory-default", sys.FactoryDefault)
	mux.HandleFunc("POST /maintenance/system/factory-reset",   sys.FactoryReset)
	mux.HandleFunc("POST /maintenance/system/datetime",        sys.SetDatetime)
	mux.HandleFunc("GET /routing", routing.Overview)
	mux.HandleFunc("GET /wifi", wifi.Overview)
	mux.HandleFunc("GET /hardware", hw.Overview)
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
	mux.HandleFunc("DELETE /configure/leaf",         cfg.DeleteLeaf)
	mux.HandleFunc("GET /configure/system",           cfgSys.Overview)
	mux.HandleFunc("GET /configure/ntp",              cfgSys.OverviewNTP)
	mux.HandleFunc("GET /configure/dns",              cfgSys.OverviewDNS)
	mux.HandleFunc("POST /configure/system/identity",     cfgSys.SaveIdentity)
	mux.HandleFunc("POST /configure/system/clock",        cfgSys.SaveClock)
	mux.HandleFunc("POST /configure/system/ntp/servers",          cfgSys.AddNTPServer)
	mux.HandleFunc("POST /configure/system/ntp/servers/{name}",   cfgSys.SaveNTPServer)
	mux.HandleFunc("DELETE /configure/system/ntp/servers/{name}", cfgSys.DeleteNTPServer)
	mux.HandleFunc("POST /configure/system/dns/servers",          cfgSys.AddDNSServer)
	mux.HandleFunc("POST /configure/system/dns/servers/{name}",   cfgSys.SaveDNSServer)
	mux.HandleFunc("DELETE /configure/system/dns/servers/{name}", cfgSys.DeleteDNSServer)
	mux.HandleFunc("POST /configure/system/dns/search",           cfgSys.AddDNSSearch)
	mux.HandleFunc("DELETE /configure/system/dns/search/{domain}", cfgSys.DeleteDNSSearch)
	mux.HandleFunc("POST /configure/system/preferences",  cfgSys.SavePreferences)
	mux.HandleFunc("GET /configure/interfaces",                          cfgIf.Overview)
	mux.HandleFunc("POST /configure/interfaces",                         cfgIf.CreateInterface)
	mux.HandleFunc("POST /configure/interfaces/wizard/sym-key",          cfgIf.WizardCreateSymKey)
	mux.HandleFunc("POST /configure/interfaces/wizard/asym-key",         cfgIf.WizardCreateAsymKey)
	mux.HandleFunc("POST /configure/interfaces/wizard/wg-genkey",        cfgIf.WizardGenerateWGKey)
	mux.HandleFunc("POST /configure/interfaces/wizard/radio",            cfgIf.WizardCreateRadio)
	mux.HandleFunc("POST /configure/interfaces/{name}",                  cfgIf.SaveGeneral)
	mux.HandleFunc("DELETE /configure/interfaces/{name}",                cfgIf.DeleteInterface)
	mux.HandleFunc("POST /configure/interfaces/{name}/ipv4",              cfgIf.AddIPv4)
	mux.HandleFunc("DELETE /configure/interfaces/{name}/ipv4/{ip}",       cfgIf.DeleteIPv4)
	mux.HandleFunc("POST /configure/interfaces/{name}/ipv4/settings",                cfgIf.SaveIPv4Settings)
	mux.HandleFunc("POST /configure/interfaces/{name}/ipv4/dhcp/settings",           cfgIf.SaveIPv4DHCPSettings)
	mux.HandleFunc("POST /configure/interfaces/{name}/ipv4/dhcp/options",            cfgIf.AddIPv4DHCPOption)
	mux.HandleFunc("DELETE /configure/interfaces/{name}/ipv4/dhcp/options/{id}",     cfgIf.DeleteIPv4DHCPOption)
	mux.HandleFunc("POST /configure/interfaces/{name}/ipv6",                         cfgIf.AddIPv6)
	mux.HandleFunc("DELETE /configure/interfaces/{name}/ipv6/{ip}",                  cfgIf.DeleteIPv6)
	mux.HandleFunc("POST /configure/interfaces/{name}/ipv6/settings",                cfgIf.SaveIPv6Settings)
	mux.HandleFunc("POST /configure/interfaces/{name}/ipv6/dhcp/settings",           cfgIf.SaveIPv6DHCPSettings)
	mux.HandleFunc("POST /configure/interfaces/{name}/ipv6/dhcp/options",            cfgIf.AddIPv6DHCPOption)
	mux.HandleFunc("DELETE /configure/interfaces/{name}/ipv6/dhcp/options/{id}",     cfgIf.DeleteIPv6DHCPOption)
	mux.HandleFunc("POST /configure/interfaces/{name}/ethernet",                cfgIf.SaveEthernet)
	mux.HandleFunc("DELETE /configure/interfaces/{name}/ethernet/advertised",   cfgIf.ResetEthernetAdvertised)
	mux.HandleFunc("POST /configure/interfaces/{name}/bridge-port",      cfgIf.SaveBridgePort)
	mux.HandleFunc("DELETE /configure/interfaces/{name}/bridge-port",    cfgIf.DeleteBridgePort)
	mux.HandleFunc("POST /configure/interfaces/{name}/wifi",             cfgIf.SaveWifi)
	mux.HandleFunc("POST /configure/interfaces/{name}/bridge",           cfgIf.SaveBridge)
	mux.HandleFunc("POST /configure/interfaces/{name}/bridge/stp",       cfgIf.SaveBridgeSTP)
	mux.HandleFunc("POST /configure/interfaces/{name}/bridge/multicast", cfgIf.SaveBridgeMulticast)
	mux.HandleFunc("POST /configure/interfaces/{name}/bridge/vlans",     cfgIf.AddVLAN)
	mux.HandleFunc("POST /configure/interfaces/{name}/bridge/vlans/{vid}", cfgIf.SaveVLAN)
	mux.HandleFunc("DELETE /configure/interfaces/{name}/bridge/vlans/{vid}", cfgIf.DeleteVLAN)
	mux.HandleFunc("POST /configure/interfaces/{name}/lag",              cfgIf.SaveLAG)
	mux.HandleFunc("POST /configure/interfaces/{name}/lag/members",      cfgIf.SaveLAGMembers)
	mux.HandleFunc("POST /configure/interfaces/{name}/lag-port",         cfgIf.SaveLagPort)
	mux.HandleFunc("DELETE /configure/interfaces/{name}/lag-port",       cfgIf.DeleteLagPort)
	mux.HandleFunc("GET /configure/firewall",                        cfgFw.Overview)
	mux.HandleFunc("POST /configure/firewall/enable",               cfgFw.Enable)
	mux.HandleFunc("POST /configure/firewall/settings",             cfgFw.SaveSettings)
	mux.HandleFunc("POST /configure/firewall/zones",                cfgFw.AddZone)
	mux.HandleFunc("POST /configure/firewall/zones/{name}",         cfgFw.SaveZone)
	mux.HandleFunc("DELETE /configure/firewall/zones/{name}",       cfgFw.DeleteZone)
	mux.HandleFunc("DELETE /configure/firewall/zones/{name}/interfaces", cfgFw.ResetZoneInterfaces)
	mux.HandleFunc("DELETE /configure/firewall/zones/{name}/services",   cfgFw.ResetZoneServices)
	mux.HandleFunc("POST /configure/firewall/zones/{name}/port-forwards", cfgFw.AddPortForward)
	mux.HandleFunc("DELETE /configure/firewall/zones/{name}/port-forwards/{lower}/{proto}", cfgFw.DeletePortForward)
	mux.HandleFunc("POST /configure/firewall/policies",             cfgFw.AddPolicy)
	mux.HandleFunc("POST /configure/firewall/policies/{name}",     cfgFw.SavePolicy)
	mux.HandleFunc("DELETE /configure/firewall/policies/{name}",   cfgFw.DeletePolicy)
	mux.HandleFunc("POST /configure/firewall/services",            cfgFw.AddService)
	mux.HandleFunc("POST /configure/firewall/services/{name}",     cfgFw.SaveService)
	mux.HandleFunc("DELETE /configure/firewall/services/{name}",   cfgFw.DeleteService)
	mux.HandleFunc("GET /configure/hardware",                       cfgHw.Overview)
	mux.HandleFunc("POST /configure/hardware",                      cfgHw.CreateHardware)
	mux.HandleFunc("POST /configure/hardware/usb/{name}",           cfgHw.SaveUSBPort)
	mux.HandleFunc("POST /configure/hardware/wifi/{name}",          cfgHw.SaveWiFiRadio)
	mux.HandleFunc("POST /configure/hardware/gps/{name}",           cfgHw.SaveGPS)
	mux.HandleFunc("DELETE /configure/hardware/{name}",             cfgHw.DeleteComponent)
	mux.HandleFunc("GET /configure/routes",               cfgRoutes.Overview)
	mux.HandleFunc("POST /configure/routes",              cfgRoutes.AddRoute)
	mux.HandleFunc("PUT /configure/routes",               cfgRoutes.UpdateRoute)
	mux.HandleFunc("DELETE /configure/routes",            cfgRoutes.DeleteRoute)
	mux.HandleFunc("GET /configure/users",                cfgUsers.Overview)
	mux.HandleFunc("POST /configure/users",               cfgUsers.AddUser)
	mux.HandleFunc("DELETE /configure/users/{name}",      cfgUsers.DeleteUser)
	mux.HandleFunc("POST /configure/users/{name}/shell",    cfgUsers.UpdateShell)
	mux.HandleFunc("POST /configure/users/{name}/password", cfgUsers.ChangePassword)
	mux.HandleFunc("POST /configure/users/{name}/keys",     cfgUsers.AddKey)
	mux.HandleFunc("DELETE /configure/users/{name}/keys/{keyname}", cfgUsers.DeleteKey)
	mux.HandleFunc("POST /configure/users/groups/{name}/members",          cfgUsers.AddGroupMembers)
	mux.HandleFunc("DELETE /configure/users/groups/{name}/members/{user}", cfgUsers.RemoveGroupMember)
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
	mux.HandleFunc("PUT /configure/tree/presence",    treeH.TogglePresence)
	mux.HandleFunc("DELETE /configure/tree/presence", treeH.TogglePresence)
	mux.HandleFunc("DELETE /configure/tree/container", treeH.DeleteContainer)

	// Status tree (read-only operational view).
	mux.HandleFunc("GET /status/tree",          statusTreeH.Overview)
	mux.HandleFunc("GET /status/tree/children", statusTreeH.TreeChildren)
	mux.HandleFunc("GET /status/tree/node",     statusTreeH.TreeNode)

	handler := authMiddleware(store, mux)
	handler = csrfMiddleware(handler)
	handler = securityHeadersMiddleware(handler)
	return handler, nil
}
