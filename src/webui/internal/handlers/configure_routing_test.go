// SPDX-License-Identifier: MIT

package handlers

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"infix/webui/internal/schema"
	"infix/webui/internal/testutil"

	"github.com/kernelkit/infix/src/features"
)

// OSPFv2 with area 0 on br0 (cost set in the advanced editor) and wan,
// and RIPv2 on br0, as the candidate holds them.
const routingFixture = `{"ietf-routing:routing":{"control-plane-protocols":{"control-plane-protocol":[
 {"type":"infix-routing:static","name":"static"},
 {"type":"infix-routing:ospfv2","name":"default","ietf-ospf:ospf":{
  "areas":{"area":[{"area-id":"0.0.0.0","area-type":"normal-area","interfaces":{"interface":[
   {"name":"br0","cost":42,"bfd":{"enabled":true}},{"name":"wan","bfd":{"enabled":true}}]}}]},
  "infix-ospf:redistribute":{"redistribute":[{"protocol":"connected"},{"protocol":"ospf"}]}}},
 {"type":"infix-routing:ripv2","name":"default","ietf-rip:rip":{
  "interfaces":{"interface":[{"interface":"br0"}]}}}]}}}`

const routingIfacesFixture = `{"ietf-interfaces:interfaces":{"interface":[
 {"name":"br0","type":"infix-if-type:bridge","ietf-ip:ipv4":{"forwarding":true},"ietf-ip:ipv6":{}},
 {"name":"lan1","type":"infix-if-type:ethernet","infix-interfaces:bridge-port":{"bridge":"br0"}},
 {"name":"lo","type":"infix-if-type:loopback","ietf-ip:ipv4":{}},
 {"name":"wan","type":"infix-if-type:ethernet","ietf-ip:ipv4":{}}]}}`

// The modules the rows are gated on: ietf-routing's protocol list, with
// the OSPF and RIP containers augmented in as on a device.
var routingSchemaYang = map[string]string{
	"ietf-routing.yang": `module ietf-routing {
  yang-version 1.1; namespace "urn:test:routing"; prefix rt;
  container routing { container control-plane-protocols { list control-plane-protocol {
    key "type name"; leaf type { type string; } leaf name { type string; } } } }
}`,
	"ietf-ospf.yang": `module ietf-ospf {
  yang-version 1.1; namespace "urn:test:ospf"; prefix ospf;
  import ietf-routing { prefix rt; }
  identity area-type;
  identity normal-area { base area-type; }
  identity stub-nssa-area { base area-type; }
  identity stub-area { base stub-nssa-area; }
  identity nssa-area { base stub-nssa-area; }
  augment "/rt:routing/rt:control-plane-protocols/rt:control-plane-protocol" {
    container ospf { container areas { list area { key area-id;
      leaf area-id { type string; description "Area ID."; }
      leaf area-type { type identityref { base area-type; } default normal-area; description "Area type."; }
      container interfaces { list interface { key name; leaf name { type string; }
        container bfd { leaf enabled { type boolean; description "BFD on the interface."; } } } } } } } }
}`,
	"ietf-rip.yang": `module ietf-rip {
  yang-version 1.1; namespace "urn:test:rip"; prefix rip;
  import ietf-routing { prefix rt; }
  augment "/rt:routing/rt:control-plane-protocols/rt:control-plane-protocol" {
    container rip { leaf enabled { type boolean; } } }
}`,
}

func routingCPPs(t *testing.T) []rtProtoJSON {
	t.Helper()
	var w rtProtoWrapper
	if err := json.Unmarshal([]byte(routingFixture), &w); err != nil {
		t.Fatal(err)
	}
	return w.Routing.CPPs.CPP
}

func routingIfaces(t *testing.T) []ifaceJSON {
	t.Helper()
	var w interfacesWrapper
	if err := json.Unmarshal([]byte(routingIfacesFixture), &w); err != nil {
		t.Fatal(err)
	}
	return w.Interfaces.Interface
}

func routingSchemaDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	for name, text := range routingSchemaYang {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(text), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

func routingSchema(t *testing.T) *schema.Manager {
	t.Helper()
	mgr, err := schema.Load(routingSchemaDir(t))
	if err != nil {
		t.Fatal(err)
	}
	return mgr
}

// routingSchemaCache is the schema as a handler holds it.
func routingSchemaCache(t *testing.T) *schema.Cache {
	t.Helper()
	c := schema.NewCache(nil, routingSchemaDir(t), "")
	if err := c.LoadFromCache(); err != nil {
		t.Fatal(err)
	}
	return c
}

const stubArea, nssaArea, normalArea = "ietf-ospf:stub-area", "ietf-ospf:nssa-area", "ietf-ospf:normal-area"

var ospfv2, ospfv3, ripv2 = rtProtos[0], rtProtos[1], rtProtos[2]

func TestOSPFSetupOf(t *testing.T) {
	o := findInstance(routingCPPs(t), ospfv2.Type).OSPF
	s := ospfSetupOf(o)
	if len(s.Areas) != 1 || s.Areas[0].ID != "0.0.0.0" || s.Areas[0].Type != "normal-area" ||
		!slices.Equal(s.Areas[0].Ifaces, []string{"br0", "wan"}) {
		t.Errorf("areas %+v", s.Areas)
	}
	if !s.BFD || s.BFDMixed || !s.Connected || s.Static {
		t.Errorf("setup %+v", s)
	}

	// BFD on one of the two interfaces only
	off := false
	o.Areas.Area[0].Interfaces.Interface[1].BFD.Enabled = &off
	if s := ospfSetupOf(o); !s.BFDMixed || s.BFD {
		t.Errorf("BFD on one of two interfaces: %+v", s)
	}
}

func TestParseOSPFGlobal(t *testing.T) {
	f, err := parseOSPFGlobal(url.Values{"router-id": {" 10.0.0.1 "}, "bfd": {"on"}, "redist-static": {"on"}}, ospfv2)
	if err != nil || f.RouterID != "10.0.0.1" || !f.BFD || !f.BFDChanged || !f.Static || f.Connected {
		t.Errorf("form %+v, %v", f, err)
	}
	if _, err := parseOSPFGlobal(url.Values{"router-id": {"fe80::1"}}, ospfv2); err == nil || !strings.Contains(err.Error(), "not an IPv4 address") {
		t.Errorf("IPv6 router ID: %v", err)
	}
	if _, err := parseOSPFGlobal(url.Values{}, ospfv3); err == nil || !strings.Contains(err.Error(), "needs a router ID") {
		t.Errorf("OSPFv3 without router ID: %v", err)
	}
}

func TestOSPFGlobalEdit(t *testing.T) {
	cur := findInstance(routingCPPs(t), ospfv2.Type).OSPF
	base := instancePath(ospfv2.Type, "default")
	ob := base + "/ietf-ospf:ospf"

	// BFD off everywhere, static added, connected dropped, areas kept
	removes, body := ospfGlobalEdit(base, cur, ospfGlobalForm{BFDChanged: true, Static: true}, true)
	if !slices.Equal(removes, []string{ob + "/infix-ospf:redistribute/redistribute=connected"}) {
		t.Errorf("removes %q", removes)
	}
	got, _ := json.Marshal(body)
	want := `{"ietf-ospf:ospf":{"areas":{"area":[{"area-id":"0.0.0.0","interfaces":{"interface":[` +
		`{"bfd":{"enabled":false},"name":"br0"},{"bfd":{"enabled":false},"name":"wan"}]}}]},` +
		`"infix-ospf:redistribute":{"redistribute":[{"protocol":"static"}]}}}`
	if string(got) != want {
		t.Errorf("body\n got %s\nwant %s", got, want)
	}

	// BFD unchanged: the areas are not touched
	_, body = ospfGlobalEdit(base, cur, ospfGlobalForm{BFD: true, Connected: true}, true)
	if got, _ := json.Marshal(body); strings.Contains(string(got), "areas") {
		t.Errorf("areas touched: %s", got)
	}
}

func TestAreaTypes(t *testing.T) {
	opts := schema.OptionsFor(routingSchema(t), ospfAreaTypePath)
	got, _ := json.Marshal(areaTypes(opts, "stub-area"))
	want := `[{"Value":"ietf-ospf:normal-area","Label":"normal","Selected":false},{"Value":"ietf-ospf:nssa-area","Label":"NSSA","Selected":false},` +
		`{"Value":"ietf-ospf:stub-area","Label":"stub","Selected":true},{"Value":"ietf-ospf:stub-nssa-area","Label":"stub-NSSA","Selected":false}]`
	if string(got) != want {
		t.Errorf("types\n got %s\nwant %s", got, want)
	}
	if got := areaTypes(opts, ""); !got[0].Selected || got[1].Selected {
		t.Errorf("default not selected for a new area: %+v", got)
	}
}

func TestParseOSPFArea(t *testing.T) {
	cur := findInstance(routingCPPs(t), ospfv2.Type).OSPF
	types := schema.OptionsFor(routingSchema(t), ospfAreaTypePath)
	a, err := parseOSPFArea(url.Values{"area-id": {"1"}, "area-type": {stubArea}, "ifaces": {"lo"}}, cur, "", types)
	if err != nil || a.ID != "0.0.0.1" || a.Type != stubArea || !slices.Equal(a.Ifaces, []string{"lo"}) {
		t.Errorf("new area %+v, %v", a, err)
	}
	a, err = parseOSPFArea(url.Values{"area-type": {normalArea}, "ifaces": {"wan", "br0"}}, cur, "0.0.0.0", types)
	if err != nil || a.ID != "0.0.0.0" || a.Type != normalArea || !slices.Equal(a.Ifaces, []string{"br0", "wan"}) {
		t.Errorf("existing area %+v, %v", a, err)
	}
	if _, err := parseOSPFArea(url.Values{"area-type": {nssaArea}}, cur, "0.0.0.0", types); err == nil || !strings.Contains(err.Error(), "can not be a NSSA area") {
		t.Errorf("backbone as NSSA: %v", err)
	}
	if _, err := parseOSPFArea(url.Values{"area-type": {"stub"}}, cur, "0.0.0.1", types); err == nil || !strings.Contains(err.Error(), "unknown area type") {
		t.Errorf("type not in the model: %v", err)
	}
	for want, form := range map[string]url.Values{
		"already exists":     {"area-id": {"0.0.0.0"}},
		"dotted quad":        {"area-id": {"backbone"}},
		"can only be in one": {"area-id": {"1"}, "ifaces": {"wan"}},
	} {
		form.Set("area-type", normalArea)
		if _, err := parseOSPFArea(form, cur, "", types); err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("%v: error %v, want %q", form, err, want)
		}
	}
}

func TestOSPFAreaEdit(t *testing.T) {
	cur := findInstance(routingCPPs(t), ospfv2.Type).OSPF
	base := instancePath(ospfv2.Type, "default")

	// wan leaves area 0, lo joins it, with BFD like the rest
	removes, body := ospfAreaEdit(base, cur, ospfArea{ID: "0.0.0.0", Type: normalArea, Ifaces: []string{"br0", "lo"}}, true)
	if !slices.Equal(removes, []string{base + "/ietf-ospf:ospf/areas/area=0.0.0.0/interfaces/interface=wan"}) {
		t.Errorf("removes %q", removes)
	}
	got, _ := json.Marshal(body)
	want := `{"ietf-ospf:ospf":{"areas":{"area":[{"area-id":"0.0.0.0","area-type":"ietf-ospf:normal-area",` +
		`"interfaces":{"interface":[{"name":"br0"},{"bfd":{"enabled":true},"enabled":true,"name":"lo"}]}}]}}}`
	if string(got) != want {
		t.Errorf("body\n got %s\nwant %s", got, want)
	}

	// a new stub area without BFD
	removes, body = ospfAreaEdit(base, cur, ospfArea{ID: "0.0.0.1", Type: stubArea, Ifaces: []string{"lo"}}, false)
	got, _ = json.Marshal(body)
	if len(removes) != 0 || !strings.Contains(string(got), `"area-type":"ietf-ospf:stub-area"`) || strings.Contains(string(got), "bfd") {
		t.Errorf("new area: removes %q, body %s", removes, got)
	}
}

func TestRIPEdit(t *testing.T) {
	cur := findInstance(routingCPPs(t), ripv2.Type).RIP
	removes, body := ripEdit(instancePath(ripv2.Type, "default"), cur, []string{"wan"}, true, false, false)
	if len(removes) != 1 || !strings.HasSuffix(removes[0], "/ietf-rip:rip/interfaces/interface=br0") {
		t.Errorf("removes %q", removes)
	}
	got, _ := json.Marshal(body)
	want := `{"ietf-rip:rip":{"infix-rip:redistribute":{"redistribute":[{"protocol":"connected"}]},"interfaces":{"interface":[{"interface":"wan"}]}}}`
	if string(got) != want {
		t.Errorf("body\n got %s\nwant %s", got, want)
	}
}

func TestAddBody(t *testing.T) {
	got, _ := json.Marshal(addBody(ospfv3, []string{"br0"}, "10.0.0.1"))
	want := `{"ietf-ospf:ospf":{"areas":{"area":[{"area-id":"0.0.0.0","area-type":"ietf-ospf:normal-area",` +
		`"interfaces":{"interface":[{"enabled":true,"name":"br0"}]}}]},"explicit-router-id":"10.0.0.1"}}`
	if string(got) != want {
		t.Errorf("OSPF\n got %s\nwant %s", got, want)
	}
	if got, _ := json.Marshal(addBody(ripv2, nil, "")); string(got) != `{"ietf-rip:rip":{}}` {
		t.Errorf("RIP without interfaces %s", got)
	}
}

func TestAreaChoices(t *testing.T) {
	s := ospfSetupOf(findInstance(routingCPPs(t), ospfv2.Type).OSPF)
	s.Areas = append(s.Areas, ospfAreaSetup{ID: "0.0.0.1", Type: "stub-area", Ifaces: []string{"lo"}})
	got, _ := json.Marshal(areaChoices(routingIfaces(t), &s, "0.0.0.0"))
	want := `[{"Name":"br0","Checked":true,"Other":""},{"Name":"lo","Checked":false,"Other":"0.0.0.1"},{"Name":"wan","Checked":true,"Other":""}]`
	if string(got) != want {
		t.Errorf("area 0 choices\n got %s\nwant %s", got, want)
	}
	if got, _ := json.Marshal(areaChoices(routingIfaces(t), &s, "")); !strings.Contains(string(got), `{"Name":"br0","Checked":false,"Other":"0.0.0.0"}`) {
		t.Errorf("new area choices %s", got)
	}
}

func TestRoutingIfaces(t *testing.T) {
	ifaces := routingIfaces(t)
	if got := ifaceChoices(ifaces, nil); !slices.Equal(got, []string{"br0", "lo", "wan"}) {
		t.Errorf("choices %q, a bridge port is no choice", got)
	}
	if got := ifaceChoices(ifaces, []string{"lan1"}); !slices.Contains(got, "lan1") {
		t.Errorf("choices %q drop a port already in use", got)
	}

	nf := noForwarding(ifaces, map[string][]string{"br0": {"ipv4", "ipv6"}, "lo": {"ipv4"}, "wan": {"ipv6", "ipv4"}})
	got, _ := json.Marshal(nf)
	if string(got) != `[{"Name":"br0","Families":["ipv6"]},{"Name":"wan","Families":["ipv4","ipv6"]}]` {
		t.Errorf("no forwarding %s", got)
	}
}

func protoRequestCtx(caps map[string]bool) context.Context {
	return ContextWithCapabilities(context.Background(), NewCapabilities(caps))
}

// SaveProtocol writes one YANG Patch, and nothing without FRR.
func TestSaveProtocol(t *testing.T) {
	rc := testutil.NewMockFetcher()
	var cpps rtProtoWrapper
	json.Unmarshal([]byte(routingFixture), &cpps) //nolint:errcheck
	rc.SetResponse(candidatePath+cppConfigPath, cpps)
	h := &ConfigureRoutesHandler{RC: rc, Schema: &schema.Cache{}}

	form := url.Values{"router-id": {"10.0.0.1"}, "bfd": {"on"}, "bfd-was": {"on"}, "redist-connected": {"on"}}
	put := func(caps map[string]bool) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodPut, "/configure/routes/protocol/ospfv2", strings.NewReader(form.Encode())).WithContext(protoRequestCtx(caps))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		req.SetPathValue("key", "ospfv2")
		w := httptest.NewRecorder()
		h.SaveProtocol(w, req)
		return w
	}
	if w := put(nil); w.Code != http.StatusNotFound || len(rc.Patches) != 0 {
		t.Fatalf("without FRR: status %d, patches %d", w.Code, len(rc.Patches))
	}
	if w := put(map[string]bool{features.Routing: true}); w.Code != http.StatusNoContent || len(rc.Patches) != 1 {
		t.Fatalf("status %d, patches %d", w.Code, len(rc.Patches))
	}
	edits := rc.Patches[0].Edits
	if len(edits) != 1 || edits[0].Op != "merge" || edits[0].Target != instancePath(ospfv2.Type, "default") {
		t.Fatalf("edits %+v", edits)
	}
	got, _ := json.Marshal(edits[0].Value)
	if !strings.Contains(string(got), `"explicit-router-id":"10.0.0.1"`) || strings.Contains(string(got), "areas") {
		t.Errorf("body %s", got)
	}
}

// The area endpoints: add with interfaces, save, delete, all one patch.
func TestAreaHandlers(t *testing.T) {
	rc := testutil.NewMockFetcher()
	var cpps rtProtoWrapper
	json.Unmarshal([]byte(routingFixture), &cpps) //nolint:errcheck
	rc.SetResponse(candidatePath+cppConfigPath, cpps)
	h := &ConfigureRoutesHandler{RC: rc, Schema: routingSchemaCache(t)}
	do := func(method, path, area string, form url.Values, fn http.HandlerFunc) *httptest.ResponseRecorder {
		req := httptest.NewRequest(method, path, strings.NewReader(form.Encode())).WithContext(protoRequestCtx(map[string]bool{features.Routing: true}))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		req.SetPathValue("key", "ospfv2")
		if area != "" {
			req.SetPathValue("area", area)
		}
		w := httptest.NewRecorder()
		fn(w, req)
		return w
	}
	if w := do(http.MethodPost, "/configure/routes/protocol/ospfv2/areas", "", url.Values{"area-id": {"1"}, "area-type": {stubArea}, "ifaces": {"lo"}}, h.AddArea); w.Code != http.StatusNoContent {
		t.Fatalf("add area: %d %s", w.Code, w.Body.String())
	}
	if got, _ := json.Marshal(rc.Patches[0].Edits[0].Value); !strings.Contains(string(got), `"area-id":"0.0.0.1"`) || !strings.Contains(string(got), `"name":"lo"`) {
		t.Errorf("add area body %s", got)
	}
	if w := do(http.MethodPost, "/configure/routes/protocol/ospfv2/areas", "", url.Values{"area-id": {"1"}, "area-type": {normalArea}, "ifaces": {"wan"}}, h.AddArea); w.Code != http.StatusUnprocessableEntity {
		t.Errorf("interface of another area accepted: %d", w.Code)
	}
	if w := do(http.MethodPut, "/configure/routes/protocol/ospfv2/areas/0.0.0.0", "0.0.0.0", url.Values{"area-type": {normalArea}, "ifaces": {"br0"}}, h.SaveArea); w.Code != http.StatusNoContent {
		t.Fatalf("save area: %d %s", w.Code, w.Body.String())
	}
	if e := rc.Patches[len(rc.Patches)-1].Edits; len(e) != 2 || e[0].Op != "remove" || !strings.HasSuffix(e[0].Target, "/interfaces/interface=wan") {
		t.Errorf("save area edits %+v", e)
	}
	if w := do(http.MethodDelete, "/configure/routes/protocol/ospfv2/areas/0.0.0.0", "0.0.0.0", nil, h.DeleteArea); w.Code != http.StatusNoContent {
		t.Fatalf("delete area: %d", w.Code)
	}
	if e := rc.Patches[len(rc.Patches)-1].Edits; len(e) != 1 || e[0].Op != "remove" || !strings.HasSuffix(e[0].Target, "/ietf-ospf:ospf/areas/area=0.0.0.0") {
		t.Errorf("delete area edits %+v", e)
	}
	req := httptest.NewRequest(http.MethodDelete, "/configure/routes/protocol/ripv2/areas/0.0.0.0", nil).WithContext(protoRequestCtx(map[string]bool{features.Routing: true}))
	req.SetPathValue("key", "ripv2")
	w := httptest.NewRecorder()
	h.DeleteArea(w, req)
	if w.Code != http.StatusNotFound {
		t.Errorf("RIP has no areas: %d", w.Code)
	}
}

// AddProtocol creates the instance, with the router ID OSPFv3 needs.
func TestAddProtocol(t *testing.T) {
	rc := testutil.NewMockFetcher()
	h := &ConfigureRoutesHandler{RC: rc, Schema: &schema.Cache{}}
	post := func(form url.Values) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodPost, "/configure/routes/protocol", strings.NewReader(form.Encode())).
			WithContext(protoRequestCtx(map[string]bool{features.Routing: true}))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		w := httptest.NewRecorder()
		h.AddProtocol(w, req)
		return w
	}
	if w := post(url.Values{"proto": {"ospfv3"}, "ifaces": {"br0"}}); w.Code != http.StatusUnprocessableEntity || len(rc.Patches) != 0 {
		t.Errorf("OSPFv3 without router ID: status %d, patches %d", w.Code, len(rc.Patches))
	}
	if w := post(url.Values{"proto": {"bgp"}}); w.Code != http.StatusNotFound {
		t.Errorf("unknown protocol: status %d", w.Code)
	}
	if w := post(url.Values{"proto": {"ripng"}, "ifaces": {"br0", "wan"}}); w.Code != http.StatusNoContent || len(rc.Patches) != 1 {
		t.Fatalf("RIPng: status %d, patches %d", w.Code, len(rc.Patches))
	}
	got, _ := json.Marshal(rc.Patches[0].Edits[0].Value)
	if !strings.Contains(string(got), `"type":"infix-routing:ripng"`) || !strings.Contains(string(got), `{"interface":"wan"}`) {
		t.Errorf("RIPng %s", got)
	}
}

// The rows of the configured protocols, what is left to add, and how
// they render
// The rows of the configured protocols, their areas, what is left to
// add, and how they render
func TestRoutingProtocolsRender(t *testing.T) {
	data := cfgRoutesPageData{Desc: map[string]string{}}
	buildRoutingProtocols(routingSchema(t), routingCPPs(t), routingIfaces(t), &data)
	if len(data.Protocols) != 2 || data.Protocols[0].Key != "ospfv2" || data.Protocols[1].Key != "ripv2" {
		t.Fatalf("protocols %+v", data.Protocols)
	}
	ospf := data.Protocols[0]
	if ospf.Interfaces() != "br0, wan" || len(ospf.Areas) != 1 || !ospf.Areas[0].Last || ospf.Areas[0].Label() != "Area 0.0.0.0" {
		t.Errorf("OSPFv2 row %+v", ospf)
	}
	var addable []string
	for _, p := range data.Addable {
		addable = append(addable, p.Key)
	}
	if !slices.Equal(addable, []string{"ospfv3", "ripng"}) {
		t.Errorf("addable %q", addable)
	}
	if got, _ := json.Marshal(data.NoForwarding); string(got) != `[{"Name":"wan","Families":["ipv4"]}]` {
		t.Errorf("no forwarding %s", got)
	}
	if data.Desc["area-id"] == "" || data.Desc["bfd"] == "" {
		t.Errorf("hover texts %q", data.Desc)
	}

	tmpl := realTemplates(t, nil, "layouts/*.html", "fragments/configure-toolbar.html", "pages/configure-routes.html")
	data.Ifaces = []string{"br0", "lo", "wan"}
	render := func(name string, v any) string {
		var b strings.Builder
		if err := tmpl.ExecuteTemplate(&b, name, v); err != nil {
			t.Fatal(err)
		}
		return b.String()
	}
	out := render("rt-protocols", &data)
	for _, want := range []string{
		`data-target="proto-ospfv2"`, "OSPFv2 <span", "(IPv4)", "br0, wan", // the OSPFv2 row
		`data-target="area-ospfv2-0.0.0.0"`, "Area 0.0.0.0", `class="iface-tree-col tree-last"`, // its area
		`hx-put="/configure/routes/protocol/ospfv2/areas/0.0.0.0"`, `name="ifaces" value="wan" checked`,
		`hx-delete="/configure/routes/protocol/ospfv2/areas/0.0.0.0"`,
		`id="key-detail-proto-ripv2"`, // the RIPv2 fold-out
		`name="forward" value="wan/ipv4"`, "through it.",
		`data-tip="` + data.Desc["area-type"], // hover text
		`data-show-modal="rt-add-dialog"`, `data-show-modal="rt-area-dialog-ospfv2"`,
	} {
		if !strings.Contains(out, want) {
			t.Errorf("protocols lack %q", want)
		}
	}
	if strings.Contains(out, "Routing for") || strings.Contains(out, `name="area-count"`) {
		t.Error("the old columns and area rows are still there")
	}

	out = render("rt-area-dialog", ospf)
	if !strings.Contains(out, `hx-post="/configure/routes/protocol/ospfv2/areas"`) ||
		!strings.Contains(out, `value="br0"  disabled> br0 <small>(area 0.0.0.0)</small>`) {
		t.Errorf("area dialog %s", out)
	}
	out = render("rt-add-dialog", &data)
	if !strings.Contains(out, `<option value="ospfv3" data-ospf="true">OSPFv3 (IPv6)</option>`) || strings.Contains(out, `value="ospfv2"`) {
		t.Errorf("add dialog offers %s", out)
	}
}

func TestStaticRoutesRender(t *testing.T) {
	tmpl := realTemplates(t, nil, "layouts/*.html", "fragments/configure-toolbar.html", "pages/configure-routes.html")
	data := cfgRoutesPageData{Routes: []cfgRouteEntry{
		{Family: "ipv4", Prefix: "0.0.0.0/0", NextHop: "192.168.0.1"},
		{Family: "ipv6", Prefix: "::/0", NextHop: "fe80::1", Interface: "wan"},
	}, Ifaces: []string{"br0", "wan"}}
	var b strings.Builder
	if err := tmpl.ExecuteTemplate(&b, "content", data); err != nil {
		t.Fatal(err)
	}
	out := b.String()
	if strings.Count(out, `class="card-header">Static Routes`) != 1 {
		t.Error("static routes not in one table")
	}
	for _, want := range []string{
		`hx-delete="/configure/routes?family=ipv4&prefix=0.0.0.0%2F0"`,
		`hx-delete="/configure/routes?family=ipv6&prefix=%3A%3A%2F0"`,
		`data-show-modal="route-add-dialog"`, `id="route-add-dialog"`,
		`<option value="wan" selected>wan</option>`, // the IPv6 route's interface
	} {
		if !strings.Contains(out, want) {
			t.Errorf("page lacks %q", want)
		}
	}
}
