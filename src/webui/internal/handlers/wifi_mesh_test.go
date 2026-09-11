// SPDX-License-Identifier: MIT

package handlers

import (
	"bytes"
	"encoding/json"
	"html/template"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"regexp"
	"strings"
	"testing"
)

// wifiIfacesFixture is operational data, shaped the way statd delivers
// it: config-only leaves (mesh forwarding, AP roaming) are absent,
// because the operational subscription replaces the running config for
// the whole interfaces subtree. wifiCfgFixture is the candidate config
// the configure page renders from.
const wifiIfacesFixture = `{"ietf-interfaces:interfaces":{"interface":[
{"name":"wifi0-mesh","type":"infix-if-type:wifi","oper-status":"up",
 "infix-interfaces:wifi":{"radio":"phy0","mesh-point":{"mesh-id":"backhaul","forwarding":false,
   "security":{},
   "peers":{"peer":[{"mac-address":"02:00:00:00:00:01","signal-strength":-55,"connected-time":90,
     "rx-bytes":"2048","tx-bytes":"4096","rx-speed":"650","tx-speed":"1200"}]}}}},
{"name":"wifi1-ap","type":"infix-if-type:wifi","oper-status":"up",
 "infix-interfaces:wifi":{"radio":"phy1","access-point":{"ssid":"office","security":{},"roaming":{}}}},
{"name":"wifi2","type":"infix-if-type:wifi","oper-status":"up",
 "infix-interfaces:wifi":{"radio":"phy2","station":{"ssid":"upstream","bssid":"aa:bb:cc:dd:ee:ff","signal-strength":-61}}},
{"name":"wifi3-mesh","type":"infix-if-type:wifi","oper-status":"down",
 "infix-interfaces:wifi":{"radio":"phy3","mesh-point":{"mesh-id":"lonely","security":{"secret":"mesh-psk"}}}}
]}}`

const wifiCfgFixture = `{"ietf-interfaces:interfaces":{"interface":[
{"name":"wifi0-mesh","type":"infix-if-type:wifi",
 "infix-interfaces:wifi":{"radio":"phy0","mesh-point":{"mesh-id":"backhaul","forwarding":false,
   "security":{"secret":"mesh-psk"}}}},
{"name":"wifi1-ap","type":"infix-if-type:wifi",
 "infix-interfaces:wifi":{"radio":"phy1","access-point":{"ssid":"office","security":{"mode":"wpa3-personal","secret":"psk"},
   "roaming":{"dot11k":{},"dot11r":{"mobility-domain":"hash"},"dot11v":{"band-steering":false},"okc":false}}}},
{"name":"wifi2","type":"infix-if-type:wifi",
 "infix-interfaces:wifi":{"radio":"phy2","station":{"ssid":"upstream","security":{"secret":"psk"}}}},
{"name":"wifi3-mesh","type":"infix-if-type:wifi",
 "infix-interfaces:wifi":{"radio":"phy3","mesh-point":{"mesh-id":"lonely","security":{"secret":"mesh-psk"}}}}
]}}`

func decodeWiFiFixture(t *testing.T) []ifaceJSON {
	t.Helper()
	var w interfacesWrapper
	if err := json.Unmarshal([]byte(wifiIfacesFixture), &w); err != nil {
		t.Fatalf("decode fixture: %v", err)
	}
	return w.Interfaces.Interface
}

func TestBuildWiFiInterfaces_MeshPeersAndBSSID(t *testing.T) {
	ifaces := decodeWiFiFixture(t)

	mesh := buildWiFiInterfaces("phy0", ifaces)
	if len(mesh) != 1 || mesh[0].Mode != "mesh" || mesh[0].SSID != "backhaul" {
		t.Fatalf("mesh: got %+v", mesh)
	}
	if len(mesh[0].Clients) != 1 || mesh[0].Forwarding != "Disabled" {
		t.Errorf("mesh peers/forwarding: %+v", mesh[0])
	}
	// A driver that reports no mesh_fwding must not be read as "off".
	quiet := decodeWiFiFixture(t)
	quiet[0].WiFi.MeshPoint.Forwarding = nil
	if got := buildWiFiInterfaces("phy0", quiet)[0].Forwarding; got != "" {
		t.Errorf("unreported forwarding = %q, want blank", got)
	}
	if mesh[0].Clients[0].Signal != "-55 dBm" || mesh[0].Clients[0].SignalCSS != "signal-good" {
		t.Errorf("peer signal: %+v", mesh[0].Clients[0])
	}

	ap := buildWiFiInterfaces("phy1", ifaces)
	if len(ap) != 1 || ap[0].Mode != "ap" {
		t.Fatalf("ap: got %+v", ap)
	}

	sta := buildWiFiInterfaces("phy2", ifaces)
	if len(sta) != 1 || sta[0].BSSID != "aa:bb:cc:dd:ee:ff" {
		t.Fatalf("station bssid: got %+v", sta)
	}
}

func TestWiFiRoamingSummary(t *testing.T) {
	tr := true
	cases := []struct {
		name string
		in   *wifiRoamingJSON
		want string
	}{
		{"nil", nil, ""},
		{"okc default", &wifiRoamingJSON{OKC: &tr}, ""},
		{"okc off alone", &wifiRoamingJSON{OKC: new(bool)}, "no OKC"},
		{"k", &wifiRoamingJSON{Dot11k: &struct{}{}}, "802.11k"},
		{"r explicit md", &wifiRoamingJSON{Dot11r: &wifiDot11rJSON{MobilityDomain: "ab12"}}, "802.11r (domain ab12)"},
		{"r default md omitted", &wifiRoamingJSON{Dot11r: &wifiDot11rJSON{}}, "802.11r (domain 4f57)"},
		{"v default steering", &wifiRoamingJSON{Dot11v: &wifiDot11vJSON{}}, "802.11v (band steering)"},
	}
	for _, c := range cases {
		if got := wifiRoamingSummary(c.in); got != c.want {
			t.Errorf("%s: got %q want %q", c.name, got, c.want)
		}
	}
}

func TestBuildDetailData_MeshAndStation(t *testing.T) {
	ifaces := decodeWiFiFixture(t)
	req := httptest.NewRequest(http.MethodGet, "/interfaces/x", nil)

	d := buildDetailData(req, &ifaces[0])
	if d.WiFiMode != "Mesh Point" || d.WiFiMeshID != "backhaul" || d.WiFiForwarding != "Disabled" {
		t.Errorf("mesh detail: %+v", d)
	}
	if d.WiFiPeerCount != "1" || d.WiFiStaTitle != "Mesh Peers" || len(d.WiFiStations) != 1 {
		t.Errorf("mesh peers: count=%q title=%q n=%d", d.WiFiPeerCount, d.WiFiStaTitle, len(d.WiFiStations))
	}

	d = buildDetailData(req, &ifaces[1])
	if d.WiFiMode != "Access Point" || d.WiFiStaTitle != "Connected Stations" {
		t.Errorf("ap detail: %+v", d)
	}

	d = buildDetailData(req, &ifaces[2])
	if d.WiFiBSSID != "aa:bb:cc:dd:ee:ff" {
		t.Errorf("station bssid: %q", d.WiFiBSSID)
	}

	e := makeIfaceEntry(ifaces[0], nil)
	if e.Detail != "Mesh, mesh-id: backhaul, peers: 1" {
		t.Errorf("list detail: %q", e.Detail)
	}
}

func roamingForm(t *testing.T, v url.Values) *http.Request {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/configure/interfaces/wifi0/wifi", strings.NewReader(v.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	if err := req.ParseForm(); err != nil {
		t.Fatal(err)
	}
	return req
}

func TestWiFiRoamingFromForm(t *testing.T) {
	roaming, err := wifiRoamingFromForm(roamingForm(t, url.Values{
		"dot11k": {"on"}, "dot11r": {"on"}, "mobility-domain": {"AB12"}, "nas-identifier": {"auto"},
		"dot11v": {"on"}, "band-steering": {"on"}, "okc": {"on"},
	}))
	if err != nil {
		t.Fatal(err)
	}
	r := roaming["dot11r"].(map[string]any)
	if r["mobility-domain"] != "ab12" || r["nas-identifier"] != "auto" {
		t.Errorf("dot11r = %v", r)
	}
	// okc and band-steering are on: both at their YANG default, so absent.
	if _, ok := roaming["okc"]; ok {
		t.Errorf("okc should be absent when on: %v", roaming)
	}
	if _, ok := roaming["dot11v"].(map[string]any)["band-steering"]; ok {
		t.Errorf("band-steering should be absent when on: %v", roaming)
	}
	if _, ok := roaming["dot11k"]; !ok {
		t.Error("dot11k missing")
	}

	// Unticked presence containers and blank sub-fields are simply absent;
	// SaveWifi PUTs the whole container so that is enough to clear them.
	roaming, err = wifiRoamingFromForm(roamingForm(t, url.Values{"dot11r": {"on"}, "mobility-domain": {"hash"}}))
	if err != nil {
		t.Fatal(err)
	}
	for _, k := range []string{"dot11k", "dot11v"} {
		if _, ok := roaming[k]; ok {
			t.Errorf("%s should be absent", k)
		}
	}
	r = roaming["dot11r"].(map[string]any)
	if roaming["okc"] != false || r["mobility-domain"] != "hash" {
		t.Errorf("roaming = %v", roaming)
	}
	// Mesh: the checkbox posts "true" ahead of its hidden "false"
	// companion, so an unticked box arrives as "false".
	mp, err := wifiMeshPointFromForm(roamingForm(t, url.Values{"mesh-id": {" backhaul "}, "secret": {"k"}, "forwarding": {"false"}}))
	if err != nil || mp["mesh-id"] != "backhaul" || mp["forwarding"] != false {
		t.Errorf("mesh-point = %v, %v", mp, err)
	}
	mp, err = wifiMeshPointFromForm(roamingForm(t, url.Values{"mesh-id": {"m"}, "secret": {"k"}, "forwarding": {"true", "false"}}))
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := mp["forwarding"]; ok {
		t.Error("forwarding at default should be absent")
	}
	if _, err := wifiMeshPointFromForm(roamingForm(t, url.Values{"mesh-id": {"m"}})); err == nil {
		t.Error("expected error without PSK")
	}
	if _, ok := r["nas-identifier"]; ok {
		t.Error("blank nas-identifier should be absent")
	}

	if _, err := wifiRoamingFromForm(roamingForm(t, url.Values{"dot11r": {"on"}, "mobility-domain": {"xyz"}})); err == nil {
		t.Error("expected error for bad mobility domain")
	}
}

// realTemplates parses the on-disk templates the way server.go does, so
// the tests catch field-name typos in the mesh/roaming markup. Only the
// configure-interfaces page is parsed with a FuncMap in production, so
// funcs is nil for the others.
func realTemplates(t *testing.T, funcs template.FuncMap, patterns ...string) *template.Template {
	t.Helper()
	tmpl, err := template.New("").Funcs(funcs).ParseFS(os.DirFS("../../templates"), patterns...)
	if err != nil {
		t.Fatalf("parse templates: %v", err)
	}
	return tmpl
}

func TestWiFiPageRendersMeshAndRoaming(t *testing.T) {
	tmpl := realTemplates(t, nil, "layouts/*.html", "pages/wifi.html")
	ifaces := decodeWiFiFixture(t)
	comps := []hwComponentWiFiJSON{
		{Name: "phy0", WiFiRadio: &wifiRadioHWJSON{Band: "5GHz"}},
		{Name: "phy1", WiFiRadio: &wifiRadioHWJSON{Band: "2.4GHz"}},
		{Name: "phy2", WiFiRadio: &wifiRadioHWJSON{Band: "5GHz"}},
		{Name: "phy3", WiFiRadio: &wifiRadioHWJSON{Band: "5GHz"}},
	}
	data := wifiData{Radios: buildWiFiRadios(comps, ifaces)}

	var buf bytes.Buffer
	if err := tmpl.ExecuteTemplate(&buf, "content", data); err != nil {
		t.Fatalf("render: %v", err)
	}
	out := buf.String()
	for _, want := range []string{
		"backhaul", "02:00:00:00:00:01", `class="signal-good"`,
		"aa:bb:cc:dd:ee:ff", "lonely", "No mesh peers connected",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("wifi page missing %q", want)
		}
	}
	if strings.Contains(out, "signal-signal-") {
		t.Error("doubled signal- class prefix")
	}
	if !strings.Contains(out, "<th>Forwarding</th><td>Disabled") {
		t.Error("wifi page missing the kernel's mesh forwarding state")
	}
	// Config-only leaves have no place on a status page.
	for _, never := range []string{"Roaming", "802.11r"} {
		if strings.Contains(out, never) {
			t.Errorf("wifi page shows config-only %q", never)
		}
	}
}

func TestIfaceDetailRendersMesh(t *testing.T) {
	tmpl := realTemplates(t, nil, "layouts/*.html", "pages/iface-detail.html", "fragments/iface-counters.html")
	ifaces := decodeWiFiFixture(t)
	req := httptest.NewRequest(http.MethodGet, "/interfaces/wifi0-mesh", nil)
	data := buildDetailData(req, &ifaces[0])

	var buf bytes.Buffer
	if err := tmpl.ExecuteTemplate(&buf, "content", data); err != nil {
		t.Fatalf("render: %v", err)
	}
	out := buf.String()
	for _, want := range []string{"Mesh Point", "backhaul", "Mesh Peers", "02:00:00:00:00:01", `class="signal-good"`} {
		if !strings.Contains(out, want) {
			t.Errorf("detail page missing %q", want)
		}
	}
	if !strings.Contains(out, "Mesh Forwarding") {
		t.Error("detail page missing the kernel's mesh forwarding state")
	}
	for _, never := range []string{"<th>Roaming</th>"} {
		if strings.Contains(out, never) {
			t.Errorf("detail page shows config-only %q", never)
		}
	}
	if strings.Contains(out, "signal-signal-") || strings.Contains(out, `class="signal-bad"`) {
		t.Error("detail page uses a signal class with no CSS rule")
	}
}

func TestConfigureInterfacesRendersMeshAndRoamingEditors(t *testing.T) {
	tmpl := realTemplates(t, IfaceTemplateFuncs(), "layouts/*.html", "fragments/configure-toolbar.html",
		"fragments/wizard-psk-picker.html", "fragments/wizard-wgkey-picker.html",
		"fragments/wizard-radio-picker.html", "pages/configure-interfaces.html")
	// The configure page reads candidate config, not operational.
	var cw interfacesWrapper
	if err := json.Unmarshal([]byte(wifiCfgFixture), &cw); err != nil {
		t.Fatalf("decode config fixture: %v", err)
	}
	cfgIfaces := cw.Interfaces.Interface

	rows := make([]cfgIfaceRow, 0, len(cfgIfaces))
	for _, iface := range cfgIfaces {
		row := cfgIfaceRow{ifaceJSON: iface, TypeSlug: "wifi", TypeDisplay: "WiFi", IsWifi: true, Desc: map[string]string{}}
		switch {
		case iface.WiFi.AccessPoint != nil:
			row.WifiMode = "access-point"
		case iface.WiFi.MeshPoint != nil:
			row.WifiMode = "mesh-point"
		case iface.WiFi.Station != nil:
			row.WifiMode = "station"
		}
		row.ConfigTags = configSummary(&row)
		rows = append(rows, row)
	}
	data := cfgIfacePageData{
		Interfaces:  rows,
		Desc:        map[string]string{},
		WizardNames: map[string]string{"wifi": "wifi3"},
	}

	var buf bytes.Buffer
	if err := tmpl.ExecuteTemplate(&buf, "content", data); err != nil {
		t.Fatalf("render: %v", err)
	}
	out := buf.String()
	for _, want := range []string{
		`value="mesh-point" checked`, `name="mesh-id"`, `value="backhaul"`,
		`name="forwarding"`, `Roaming (802.11k/r/v)`,
		`name="dot11k" checked`, `name="mobility-domain"`, `value="hash"`, `data-fold-target="wifi-row-wifi1-ap-dot11r-md wifi-row-wifi1-ap-dot11r-nas"`,
		`add-iface-wifi-mode-mesh`, `add-iface-wifi-meshid-row`,
		`<span class="iface-tag">roaming</span>`,
	} {
		if !strings.Contains(out, want) {
			t.Errorf("configure page missing %q", want)
		}
	}
	// Forwarding is explicitly false in the fixture, so the box is unticked.
	if regexp.MustCompile(`name="forwarding"\s+checked`).MatchString(out) {
		t.Error("forwarding checkbox should not be checked")
	}
}

// A station's editor must still offer the access-point and mesh-point
// fields, hidden and disabled, so the mode can be switched in place.
func TestConfigureInterfacesRendersModeSwitch(t *testing.T) {
	tmpl := realTemplates(t, IfaceTemplateFuncs(), "layouts/*.html", "fragments/configure-toolbar.html",
		"fragments/wizard-psk-picker.html", "fragments/wizard-wgkey-picker.html",
		"fragments/wizard-radio-picker.html", "pages/configure-interfaces.html")

	var cw interfacesWrapper
	if err := json.Unmarshal([]byte(wifiCfgFixture), &cw); err != nil {
		t.Fatalf("decode config fixture: %v", err)
	}
	var sta ifaceJSON
	for _, iface := range cw.Interfaces.Interface {
		if iface.WiFi != nil && iface.WiFi.Station != nil {
			sta = iface
		}
	}
	row := cfgIfaceRow{ifaceJSON: sta, TypeSlug: "wifi", IsWifi: true, WifiMode: "station", Desc: map[string]string{}}

	var buf bytes.Buffer
	err := tmpl.ExecuteTemplate(&buf, "content", cfgIfacePageData{
		Interfaces: []cfgIfaceRow{row}, Desc: map[string]string{}, WizardNames: map[string]string{},
	})
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	out := buf.String()

	for _, want := range []string{
		`name="mode"`, `value="station" checked`, `value="access-point" `, `value="mesh-point" `,
		`data-wifi-editor`, `data-wifi-modes="mesh-point"`, `data-wifi-modes="access-point"`,
	} {
		if !strings.Contains(out, want) {
			t.Errorf("station editor missing %q", want)
		}
	}
	// The other modes' inputs must be disabled, or they would submit and
	// their required fields would block the form.
	if !regexp.MustCompile(`name="mesh-id"[^>]*disabled`).MatchString(out) {
		t.Error("mesh-id should be disabled in a station editor")
	}
	if !regexp.MustCompile(`(?s)<details class="cfg-fold" data-wifi-modes="access-point"\s+hidden`).MatchString(out) {
		t.Error("roaming section should be hidden in a station editor")
	}
	if regexp.MustCompile(`name="ssid"[^>]*disabled`).MatchString(out) {
		t.Error("ssid should stay enabled in a station editor")
	}
}

// Every block that starts hidden inside a form must be one a reveal hook
// controls, or its required fields silently block the form they sit in:
// a display:none control is still validated, and the browser cannot
// focus it to say why, so the Save button looks dead. app.js disables
// the blocks named by data-show and data-fold-target on load, and
// enables them again when they are opened.
func TestConfigureInterfacesHiddenBlocksAreDisablable(t *testing.T) {
	tmpl := realTemplates(t, IfaceTemplateFuncs(), "layouts/*.html", "fragments/configure-toolbar.html",
		"fragments/wizard-psk-picker.html", "fragments/wizard-wgkey-picker.html",
		"fragments/wizard-radio-picker.html", "pages/configure-interfaces.html")

	var cw interfacesWrapper
	if err := json.Unmarshal([]byte(wifiCfgFixture), &cw); err != nil {
		t.Fatalf("decode config fixture: %v", err)
	}
	row := cfgIfaceRow{ifaceJSON: cw.Interfaces.Interface[0], TypeSlug: "wifi", IsWifi: true,
		WifiMode: "mesh-point", Desc: map[string]string{}}

	var buf bytes.Buffer
	err := tmpl.ExecuteTemplate(&buf, "content", cfgIfacePageData{
		Interfaces: []cfgIfaceRow{row}, Desc: map[string]string{}, WizardNames: map[string]string{},
	})
	if err != nil {
		t.Fatalf("render: %v", err)
	}

	out := buf.String()
	revealed := map[string]bool{}
	for _, m := range regexp.MustCompile(`data-(?:show|fold-target)="([^"]*)"`).FindAllStringSubmatch(out, -1) {
		for _, id := range strings.Fields(m[1]) {
			revealed[id] = true
		}
	}
	id := regexp.MustCompile(`id="([^"]*)"`)
	for _, loc := range regexp.MustCompile(`<div[^>]*\bhidden\b[^>]*>`).FindAllStringIndex(out, -1) {
		tag := out[loc[0]:loc[1]]
		if m := id.FindStringSubmatch(tag); m != nil && revealed[m[1]] {
			continue
		}
		for _, ctrl := range findRequiredControls(out[loc[1]:closingDiv(out, loc[1])]) {
			t.Errorf("hidden block %s is opened by no reveal hook, so its required control blocks the form: %s", tag, ctrl)
		}
	}
}

// closingDiv returns the offset of the </div> matching a <div> that ends
// at start.
func closingDiv(s string, start int) int {
	depth, i := 1, start
	for depth > 0 {
		open, close := strings.Index(s[i:], "<div"), strings.Index(s[i:], "</div>")
		if close < 0 {
			return len(s)
		}
		if open >= 0 && open < close {
			depth, i = depth+1, i+open+4
			continue
		}
		depth, i = depth-1, i+close+6
	}
	return i
}

func findRequiredControls(s string) []string {
	var out []string
	for _, c := range regexp.MustCompile(`<(?:input|select|textarea)[^>]*>`).FindAllString(s, -1) {
		if strings.Contains(c, "required") && !strings.Contains(c, "disabled") {
			out = append(out, strings.Join(strings.Fields(c), " "))
		}
	}
	return out
}

// A component name is a list key. Under a group heading it is the
// description that names the reading, and failing that the name with the
// heading it already sits under taken off the front.
func TestSensorLabel(t *testing.T) {
	for _, c := range []struct{ name, parent, desc, want string }{
		{"radio0-temp", "radio0", "Temperature", "Temperature"},
		{"cpu-thermal", "cpu", "", "Thermal"},
		{"sfp1-RX_power", "sfp1", "", "Rx Power"},
		{"e1", "", "", "e1"},
		{"odd-one", "cpu", "", "odd-one"},
	} {
		if got := sensorLabel(c.name, c.parent, c.desc); got != c.want {
			t.Errorf("sensorLabel(%q, %q, %q) = %q, want %q", c.name, c.parent, c.desc, got, c.want)
		}
	}
}
