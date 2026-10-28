// SPDX-License-Identifier: MIT

package handlers

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
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
	mock.SetResponse(candidatePath+qosProfilesPath, map[string]any{
		"infix-qos:qos": map[string]any{"profile": []map[string]any{
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
	for _, want := range []string{"DSCP of IP frames first, then PCP of tagged frames.", "ieee, classes 1 and 0 weighted",
		"ieee-sr, classes 7 and 6 shaped", "e1, e2", "cameras", `hx-delete="/configure/qos/profiles/edge"`, "+ Add profile",
		`name="default-priority"`, `min="0" max="7"`, `min="1" max="100"`, `name="remark-dscp"`,
		// the schema's defaults fill what the profile leaves out
		`value="pcp" title="Believe the PCP of VLAN-tagged frames." selected`,
		`value="ietf" title="RFC 4594`, `value="none" title="Leave DSCP untouched." selected`,
		`value="` + qosAlgoStrict + `" title="Indicates the strict priority transmission selection algorithm." selected`,
		`value="` + qosAlgoCBS + `"`, `value="ieee" title="The 8P0D default`} {
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
