// SPDX-License-Identifier: MIT

package handlers

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"infix/webui/internal/restconf"
	"infix/webui/internal/schema"
	"infix/webui/internal/testutil"
)

// repoSchema loads infix-qos and the modules it imports from src/confd,
// so the page renders its selects from the real schema.  goyang drops
// every augment when a module is missing, so only this closed set is
// taken rather than the whole directory.
func repoSchema(t *testing.T, rc restconf.Fetcher) *schema.Cache {
	t.Helper()
	src := filepath.Join("..", "..", "..", "confd", "yang", "confd")
	dir := t.TempDir()
	for _, name := range []string{"infix-qos", "ietf-system", "ietf-netconf-acm", "iana-crypt-hash",
		"ietf-inet-types", "ietf-yang-types", "ieee802-dot1q-types"} {
		files, _ := filepath.Glob(filepath.Join(src, name+"@*.yang"))
		if len(files) == 0 {
			t.Skipf("no %s module in %s", name, src)
		}
		abs, _ := filepath.Abs(files[len(files)-1])
		if err := os.Symlink(abs, filepath.Join(dir, filepath.Base(abs))); err != nil {
			t.Fatal(err)
		}
	}
	c := schema.NewCache(rc, dir, "")
	if err := c.LoadFromCache(); err != nil {
		t.Fatal(err)
	}
	if c.Manager() == nil {
		t.Fatal("no schema manager")
	}
	return c
}

func TestConfigureQoSOverview(t *testing.T) {
	mock := testutil.NewMockFetcher()
	mock.SetResponse(candidatePath+"/ietf-system:system", map[string]any{"ietf-system:system": map[string]any{
		"infix-qos:qos-profile": []map[string]any{
			{"name": "edge", "description": "cameras",
				"ingress": map[string]any{"trust": "dscp-pcp"},
				"egress": map[string]any{"traffic-class": []map[string]any{
					{"id": 1, "algorithm": qosAlgoETS, "bandwidth": 67},
					{"id": 0, "algorithm": qosAlgoETS, "bandwidth": 33},
				}, "remark": map[string]any{"dscp": "from-priority"}}},
			{"name": "tsn-ring", "egress": map[string]any{
				"traffic-class-table": map[string]any{"preset": "ieee-sr"},
				"traffic-class":       []map[string]any{{"id": 7, "algorithm": qosAlgoCBS}, {"id": 6, "algorithm": qosAlgoCBS}},
			}},
		}},
	})
	mock.SetResponse(candidatePath+"/ietf-interfaces:interfaces", map[string]any{
		"ietf-interfaces:interfaces": map[string]any{"interface": []map[string]any{
			{"name": "e1", "infix-interfaces:qos": map[string]any{"profile": "tsn-ring"}},
			{"name": "e2", "infix-interfaces:qos": map[string]any{"profile": "tsn-ring"}},
			{"name": "e3", "infix-interfaces:qos": map[string]any{"profile": "edge"}},
		}},
	})
	h := &ConfigureQoSHandler{
		Template: pageTemplate(t, "configure-qos.html"),
		RC:       mock,
		Schema:   repoSchema(t, mock),
	}
	w := httptest.NewRecorder()
	h.Overview(w, testRequest(http.MethodGet, "/configure/qos"))
	if w.Code != http.StatusOK {
		t.Fatalf("want 200 got %d", w.Code)
	}
	body := w.Body.String()
	for _, want := range []string{"DSCP, then PCP, then the port default.", "ieee, classes 1 and 0 weighted",
		"ieee-sr, classes 7 and 6 shaped", "e1, e2", "cameras", `hx-delete="/configure/qos/profiles/edge"`, "+ Add profile",
		`name="default-priority"`, `min="0" max="7"`, `min="1" max="100"`, `name="remark-dscp"`,
		// the schema's defaults fill what the profile leaves out
		`value="pcp" title="PCP of tagged frames, then the port default." selected`,
		`value="ietf" title="RFC 4594`, `value="none" title="Leave DSCP untouched." selected`,
		`value="` + qosAlgoStrict + `" title="Indicates the strict priority transmission selection algorithm." selected`,
		`value="` + qosAlgoCBS + `"`, `value="ieee" title="PCP n maps to priority n.`,
		`path=/ietf-system:system/infix-qos:qos-profile=edge/ingress/trust&redirect=/configure/qos`,
		`path=/ietf-system:system/infix-qos:qos-profile=edge/egress/traffic-class=1&redirect=/configure/qos`,
		`<optgroup label="From template">`, `<option value="edge" title="Port facing hosts, classified by DSCP.">Edge</option>`} {
		if !strings.Contains(body, want) {
			t.Errorf("missing %q", want)
		}
	}
}

func TestQoSProfileFromForm(t *testing.T) {
	form := url.Values{
		"description":      {"uplinks"},
		"trust":            {"pcp"},
		"default-priority": {"2"},
		"pcp-preset":       {"ieee"},
		"dscp-preset":      {"msb"},
		"table-preset":     {"ieee-sr"},
		"algorithm-7":      {qosAlgoCBS},
		"algorithm-1":      {qosAlgoETS},
		"bandwidth-1":      {"70"},
		"algorithm-0":      {qosAlgoETS},
		"bandwidth-0":      {"30"},
		"algorithm-5":      {qosAlgoStrict},
		"remark-pcp":       {"none"},
		"remark-dscp":      {"from-priority"},
	}
	req := httptest.NewRequest(http.MethodPost, "/configure/qos/profiles/up", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	if err := req.ParseForm(); err != nil {
		t.Fatal(err)
	}
	p, err := qosProfileFromForm(req, "up")
	if err != nil {
		t.Fatal(err)
	}
	if p.Name != "up" || p.Description != "uplinks" || p.Ingress.Trust != "pcp" || *p.Ingress.DefaultPriority != 2 {
		t.Errorf("ingress: %+v", p.Ingress)
	}
	if p.Ingress.PCPMap.Preset != "ieee" || p.Ingress.DSCPMap.Preset != "msb" || p.Egress.Table.Preset != "ieee-sr" {
		t.Errorf("presets: %+v %+v %+v", p.Ingress.PCPMap, p.Ingress.DSCPMap, p.Egress.Table)
	}
	if len(p.Egress.Classes) != 3 {
		t.Fatalf("classes: strict entries must be left out, got %+v", p.Egress.Classes)
	}
	if p.Egress.Classes[0].ID != 0 || p.Egress.Classes[0].Bandwidth != 30 || p.Egress.Classes[2].ID != 7 || p.Egress.Classes[2].Algorithm != qosAlgoCBS {
		t.Errorf("classes: %+v", p.Egress.Classes)
	}
	if p.Egress.Remark.PCP != "none" || p.Egress.Remark.DSCP != "from-priority" {
		t.Errorf("remark: %+v", p.Egress.Remark)
	}

	form.Set("bandwidth-1", "")
	req = httptest.NewRequest(http.MethodPost, "/configure/qos/profiles/up", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.ParseForm()
	if _, err := qosProfileFromForm(req, "up"); err == nil {
		t.Errorf("a weighted class without a share was accepted")
	}
}

func TestAddProfileRequiresName(t *testing.T) {
	h := &ConfigureQoSHandler{RC: testutil.NewMockFetcher()}
	req := httptest.NewRequest(http.MethodPost, "/configure/qos/profiles", strings.NewReader("name="))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()
	h.AddProfile(w, req)
	if w.Code != http.StatusUnprocessableEntity {
		t.Errorf("want 422 got %d", w.Code)
	}
}

func TestQoSProfileKeepsCustomMaps(t *testing.T) {
	custom := map[string]any{"entry": []any{map[string]any{"dscp": 46.0, "priority": 5.0}}}
	table := map[string]any{"priority0": 1.0, "priority7": 7.0}
	current := map[string]any{
		"name":    "edge",
		"ingress": map[string]any{"trust": "dscp", "dscp-map": custom},
		"egress":  map[string]any{"traffic-class-table": table},
	}
	var p qosProfileJSON
	if err := json.Unmarshal([]byte(`{"name":"edge","ingress":{"dscp-map":{"entry":[{"dscp":46,"priority":5}]}},
		"egress":{"traffic-class-table":{"priority0":1,"priority7":7}}}`), &p); err != nil {
		t.Fatal(err)
	}
	row := qosProfileRowOf(p, nil, qosDefaults{Classes: 8, DSCPPreset: "ietf", TablePreset: "ieee"})
	if row.DSCPPreset != qosCustom || row.TablePreset != qosCustom || row.PCPPreset != "" {
		t.Fatalf("row shows %q %q %q", row.PCPPreset, row.DSCPPreset, row.TablePreset)
	}

	// The editor sends the custom marker back for what it cannot edit
	form := qosProfileJSON{
		Name:        "edge",
		Description: "renamed",
		Ingress:     &qosIngressJSON{Trust: "dscp", PCPMap: &qosMapJSON{Preset: "ieee"}, DSCPMap: &qosMapJSON{Preset: qosCustom}},
		Egress:      &qosEgressJSON{Table: &qosTableJSON{Preset: qosCustom}},
	}
	body, err := qosProfileBody(form, current)
	if err != nil {
		t.Fatal(err)
	}
	in := body["ingress"].(map[string]any)
	eg := body["egress"].(map[string]any)
	if !reflect.DeepEqual(in["dscp-map"], custom) || !reflect.DeepEqual(eg["traffic-class-table"], table) {
		t.Errorf("custom nodes not kept: %v %v", in["dscp-map"], eg["traffic-class-table"])
	}
	if in["pcp-map"].(map[string]any)["preset"] != "ieee" || body["description"] != "renamed" {
		t.Errorf("edited fields lost: %v", body)
	}

	// Without a profile to copy from, the marker is dropped rather than sent
	body, _ = qosProfileBody(form, nil)
	if _, ok := body["ingress"].(map[string]any)["dscp-map"]; ok {
		t.Errorf("custom marker sent to the device: %v", body)
	}
}

// Every value a template sets must be one the model offers for its leaf,
// so a template cannot drift from the schema unnoticed.
func TestQoSTemplatesMatchSchema(t *testing.T) {
	mgr := repoSchema(t, testutil.NewMockFetcher()).Manager()
	var walk func(path string, v any)
	walk = func(path string, v any) {
		switch v := v.(type) {
		case map[string]any:
			for k, c := range v {
				walk(path+"/"+k, c)
			}
		case string:
			for _, o := range schema.OptionsFor(mgr, path) {
				if o.Value == v || o.Label == v {
					return
				}
			}
			t.Errorf("%s: %q is not an option of the model", path, v)
		}
	}
	for _, tpl := range qosTemplates {
		walk(qosProfilesPath+"/ingress", tpl.ingress)
		walk(qosProfilesPath+"/egress", tpl.egress)
	}
}

func TestAddProfileFromTemplate(t *testing.T) {
	var got map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewDecoder(r.Body).Decode(&got) //nolint:errcheck
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()
	h := &ConfigureQoSHandler{RC: restconf.NewClient(srv.URL, false)}

	post := func(form string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodPost, "/configure/qos/profiles", strings.NewReader(form))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		w := httptest.NewRecorder()
		h.AddProfile(w, req)
		return w
	}

	post("name=wan&template=edge")
	b, _ := json.Marshal(got)
	for _, want := range []string{`"name":"wan"`, `"trust":"dscp"`, `"pcp":"from-priority"`} {
		if !strings.Contains(string(b), want) {
			t.Errorf("edge profile %s lacks %s", b, want)
		}
	}
	if w := post("name=x&template=nope"); w.Code != http.StatusUnprocessableEntity {
		t.Errorf("unknown template: want 422 got %d", w.Code)
	}
}

// The QoS guide carries every template as a CLI recipe.
func TestQoSTemplatesHaveRecipes(t *testing.T) {
	doc, err := os.ReadFile(filepath.Join("..", "..", "..", "..", "doc", "qos.md"))
	if err != nil {
		t.Skip(err)
	}
	for _, tpl := range qosTemplates {
		if !strings.Contains(string(doc), "<b>edit system qos-profile "+tpl.Key+"</b>") {
			t.Errorf("doc/qos.md has no recipe for template %q", tpl.Key)
		}
	}
}

// fakeRousette answers like the device: a list read without a key is
// refused, and every reply nests the profiles under system.
func fakeRousette(t *testing.T, profiles string, put *map[string]any) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.EscapedPath()
		switch {
		case r.Method == http.MethodPut:
			json.NewDecoder(r.Body).Decode(put) //nolint:errcheck
			w.WriteHeader(http.StatusNoContent)
		case strings.HasSuffix(path, "infix-qos:qos-profile"):
			w.Header().Set("Content-Type", "application/yang-data+json")
			w.WriteHeader(http.StatusBadRequest)
			io.WriteString(w, `{"ietf-restconf:errors":{"error":[{"error-type":"application","error-tag":"operation-failed","error-message":"List requires 1 keys"}]}}`) //nolint:errcheck
		default:
			w.Header().Set("Content-Type", "application/yang-data+json")
			io.WriteString(w, `{"ietf-system:system":{"infix-qos:qos-profile":`+profiles+`}}`) //nolint:errcheck
		}
	}))
}

func TestFetchQoSProfilesReadsThroughSystem(t *testing.T) {
	srv := fakeRousette(t, `[{"name":"wan"},{"name":"edge"}]`, nil)
	defer srv.Close()
	profiles, err := fetchQoSProfiles(context.Background(), restconf.NewClient(srv.URL, false))
	if err != nil {
		t.Fatal(err)
	}
	if len(profiles) != 2 || profiles[0].Name != "edge" || profiles[1].Name != "wan" {
		t.Errorf("profiles %+v", profiles)
	}
}

// A custom DSCP map the editor cannot show survives a save of the rest.
func TestSaveProfileKeepsCustomMap(t *testing.T) {
	var put map[string]any
	srv := fakeRousette(t, `[{"name":"edge","ingress":{"trust":"dscp","dscp-map":{"entry":[{"dscp":46,"priority":5}]}}}]`, &put)
	defer srv.Close()
	h := &ConfigureQoSHandler{RC: restconf.NewClient(srv.URL, false)}
	form := url.Values{"trust": {"dscp"}, "dscp-preset": {qosCustom}, "description": {"hosts"}}
	req := httptest.NewRequest(http.MethodPost, "/configure/qos/profiles/edge", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.SetPathValue("name", "edge")
	h.SaveProfile(httptest.NewRecorder(), req)

	b, _ := json.Marshal(put)
	if !strings.Contains(string(b), `"dscp-map":{"entry":[{"dscp":46,"priority":5}]}`) {
		t.Errorf("custom map lost: %s", b)
	}
}

// A leaf the editor has no field for survives a save.
func TestQoSProfileKeepsUnknownLeaves(t *testing.T) {
	current := map[string]any{
		"name":    "edge",
		"ingress": map[string]any{"trust": "pcp", "default-drop-precedence": "yellow"},
	}
	form := qosProfileJSON{Name: "edge", Ingress: &qosIngressJSON{Trust: "dscp"}}
	body, err := qosProfileBody(form, current)
	if err != nil {
		t.Fatal(err)
	}
	in := body["ingress"].(map[string]any)
	if in["default-drop-precedence"] != "yellow" || in["trust"] != "dscp" {
		t.Errorf("ingress saved as %v", in)
	}

	// A field the editor does hold is cleared when the form leaves it out
	current["description"] = "old"
	body, _ = qosProfileBody(form, current)
	if _, ok := body["description"]; ok {
		t.Errorf("cleared description kept: %v", body)
	}
}
