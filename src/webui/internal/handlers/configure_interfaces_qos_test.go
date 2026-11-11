// SPDX-License-Identifier: MIT

package handlers

import (
	"bytes"
	"encoding/json"
	"io"
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

const tsnIfaceFixture = `{"ietf-interfaces:interfaces": {"interface": [
  {"name": "e1", "type": "infix-if-type:ethernet",
   "infix-interfaces:qos": {"profile": "tsn-ring"},
   "ieee802-dot1dc-cbsa-if:cbsa": {"cbsa-parameter-table": [
     {"traffic-class": 7, "admin-idle-slope": "20000000"},
     {"traffic-class": 5, "admin-idle-slope": "1000000"}]},
   "ieee802-dot1dc-sched-if:gate-parameter-table": {
     "gate-enabled": true,
     "admin-cycle-time": {"numerator": 1, "denominator": 10000},
     "admin-base-time": {"seconds": "0", "nanoseconds": 200},
     "admin-cycle-time-extension": 5000,
     "queue-max-sdu-table": [{"traffic-class": 5, "queue-max-sdu": 200}],
     "admin-control-list": {"gate-control-entry": [
       {"index": 1, "operation-name": "ieee802-dot1q-sched:set-gate-states", "gate-states-value": 63, "time-interval-value": 80000},
       {"index": 0, "operation-name": "ieee802-dot1q-sched:set-gate-states", "gate-states-value": 192, "time-interval-value": 20000}]}}},
  {"name": "e2", "type": "infix-if-type:ethernet"}
]}}`

const tsnOperFixture = `{"ietf-interfaces:interfaces": {"interface": [
  {"name": "e1", "type": "infix-if-type:ethernet",
   "infix-interfaces:qos": {"capabilities": {"max-traffic-classes": 8}},
   "ieee802-dot1dc-cbsa-if:cbsa": {"cbsa-parameter-table": [{"traffic-class": 7, "oper-idle-slope": "20000000"}]},
   "ieee802-dot1dc-sched-if:gate-parameter-table": {
     "oper-cycle-time": {"numerator": 1, "denominator": 10000},
     "config-pending": true,
     "oper-control-list": {"gate-control-entry": [
       {"index": 0, "gate-states-value": 192, "time-interval-value": 20000},
       {"index": 1, "gate-states-value": 63, "time-interval-value": 80000}]}}},
  {"name": "e2", "type": "infix-if-type:ethernet",
   "infix-interfaces:qos": {"capabilities": {"max-traffic-classes": 4}}}
]}}`

func tsnProfiles() []qosProfileJSON {
	return []qosProfileJSON{{Name: "tsn-ring", Egress: &qosEgressJSON{
		Table:   &qosTableJSON{Preset: "ieee-sr"},
		Classes: []qosClassJSON{{ID: 7, Algorithm: qosAlgoCBS}, {ID: 6, Algorithm: qosAlgoCBS}},
	}}}
}

func tsnRows(t *testing.T) []cfgIfaceRow {
	t.Helper()
	var cfg, oper interfacesWrapper
	if err := json.Unmarshal([]byte(tsnIfaceFixture), &cfg); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal([]byte(tsnOperFixture), &oper); err != nil {
		t.Fatal(err)
	}
	h := &ConfigureInterfacesHandler{}
	return h.buildRows(cfg.Interfaces.Interface, oper.Interfaces.Interface, tsnProfiles())
}

func TestQoSTSNRows(t *testing.T) {
	rows := tsnRows(t)
	e1 := rows[0]
	want := []qosShaperRow{
		{ID: 7, Shaped: true, IdleMbit: "20"},
		{ID: 6, Shaped: true},
		{ID: 5, Shaped: false, IdleMbit: "1"},
	}
	if len(e1.QoSShapers) != len(want) {
		t.Fatalf("shapers: %+v", e1.QoSShapers)
	}
	for i, w := range want {
		if e1.QoSShapers[i] != w {
			t.Errorf("shaper %d: got %+v want %+v", i, e1.QoSShapers[i], w)
		}
	}
	g := e1.QoSGates
	if !g.Enabled || g.CycleUs != "100" || g.BaseSeconds != "0" || g.BaseNs != 200 || g.ExtensionUs != "5" {
		t.Errorf("gates: %+v", g)
	}
	if len(g.Entries) != 2 || g.Entries[0].IntervalUs != "20" || g.Entries[1].IntervalUs != "80" {
		t.Fatalf("entries not in index order: %+v", g.Entries)
	}
	if g.Entries[0].Open != [8]bool{false, false, false, false, false, false, true, true} {
		t.Errorf("entry 0 gates: %+v", g.Entries[0].Open)
	}
	if g.MaxSDU[5] != "200" || g.MaxSDU[7] != "" {
		t.Errorf("max sdu: %+v", g.MaxSDU)
	}

	e2 := rows[1]
	if len(e2.QoSGates.Classes) != 4 || e2.QoSGates.Classes[0] != 3 {
		t.Errorf("a four queue port lists classes 3 to 0, got %v", e2.QoSGates.Classes)
	}
	// A port with no profile shapes on its own: every class has a row
	if len(e2.QoSShapers) != 4 || e2.QoSShapers[0].ID != 3 || e2.QoSShapers[0].Shaped || e2.QoSGates.Enabled {
		t.Errorf("port without tsn settings: %+v %+v", e2.QoSShapers, e2.QoSGates)
	}
}

func TestConfigureInterfacesRendersTSN(t *testing.T) {
	tmpl := realTemplates(t, IfaceTemplateFuncs(), "layouts/*.html", "fragments/configure-toolbar.html",
		"fragments/wizard-psk-picker.html", "fragments/wizard-wgkey-picker.html",
		"fragments/wizard-radio-picker.html", "pages/configure-interfaces.html")
	rows := tsnRows(t)
	data := cfgIfacePageData{
		PageData:    PageData{Capabilities: NewCapabilities(map[string]bool{"qos": true, "tsn": true, "egress-shaper": true})},
		Interfaces:  rows,
		Desc:        map[string]string{},
		QoSProfiles: []string{"tsn-ring"},
		QoSBounds:   qosBounds{RateLo: "0.000008", SlopeHi: "18446744073709.55", IntervalHi: "4294967.295", MaxSDULo: "0"},
	}
	var buf bytes.Buffer
	if err := tmpl.ExecuteTemplate(&buf, "content", data); err != nil {
		t.Fatalf("render: %v", err)
	}
	out := buf.String()
	for _, gone := range []string{"running 2 entries", "not running", "In effect", "Gate Control List"} {
		if strings.Contains(out, gone) {
			t.Errorf("status %q on the configure page", gone)
		}
	}
	for _, want := range []string{
		`hx-post="/configure/interfaces/e1/qos"`, `<option value="tsn-ring" selected>`,
		`hx-post="/configure/interfaces/e1/cbs"`, `name="idle_7"`, `value="20"`, `(not shaped by the profile)`,
		`hx-post="/configure/interfaces/e1/gates"`, `name="gate_enabled" value="on" checked`,
		`name="cycle"`, `name="entry_open_1" value="5" checked`, `name="entry_interval_0"`,
		`id="gate-entry-tpl-e1"`, `data-template="gate-entry-tpl-e1"`, `name="entry_interval___N__"`, `name="maxsdu_5"`,
		`hx-post="/configure/interfaces/e2/cbs"`, `name="idle_3"`, `hx-post="/configure/interfaces/e2/gates"`,
		`href="/configure/qos" hx-get="/configure/qos" hx-target="#content" hx-swap="innerHTML"`,
		`Cycle <span class="unit">(µs)</span>`, "Time-Aware Shaper",
		`hx-delete="/configure/leaf?path=/ietf-interfaces:interfaces/interface=e1/infix-interfaces:qos/egress-shaper&redirect=/configure/interfaces"`,
		`hx-delete="/configure/interfaces/e1/cbs/7"`, `hx-delete="/configure/interfaces/e1/gates/max-sdu"`,
		`min="0.000008"`, `max="18446744073709.55" step="any" name="idle_7"`,
		`max="4294967.295" step="any" name="entry_interval_0"`, `min="0" max="" step="1" name="maxsdu_5"`,
	} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q", want)
		}
	}
}

func gatesForm(t *testing.T, form url.Values) (map[string]any, error) {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/configure/interfaces/e1/gates", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	if err := req.ParseForm(); err != nil {
		t.Fatal(err)
	}
	return qosGatesFromForm(req)
}

func TestQoSGatesFromForm(t *testing.T) {
	table, err := gatesForm(t, url.Values{
		"gate_enabled":     {"on"},
		"cycle":            {"100"},
		"base_s":           {"0"},
		"base_ns":          {"200"},
		"entry_open_0":     {"7", "6"},
		"entry_interval_0": {"20"},
		"entry_open_2":     {"5", "4", "3", "2", "1", "0"},
		"entry_interval_2": {"80"},
		"extension":        {"5"},
		"maxsdu_5":         {"200"},
	})
	if err != nil {
		t.Fatal(err)
	}
	got, _ := json.Marshal(table)
	want := `{"admin-base-time":{"nanoseconds":200,"seconds":"0"},"admin-control-list":{"gate-control-entry":[` +
		`{"gate-states-value":192,"index":0,"operation-name":"ieee802-dot1q-sched:set-gate-states","time-interval-value":20000},` +
		`{"gate-states-value":63,"index":1,"operation-name":"ieee802-dot1q-sched:set-gate-states","time-interval-value":80000}]},` +
		`"admin-cycle-time":{"denominator":10000,"numerator":1},"admin-cycle-time-extension":5000,"gate-enabled":true,` +
		`"queue-max-sdu-table":[{"queue-max-sdu":200,"traffic-class":5}]}`
	if string(got) != want {
		t.Errorf("got  %s\nwant %s", got, want)
	}

	if _, err := gatesForm(t, url.Values{"gate_enabled": {"on"}}); err == nil {
		t.Errorf("an enabled list without entries was accepted")
	}
	table, err = gatesForm(t, url.Values{"base_s": {"0"}, "base_ns": {"7"}})
	if err != nil || table == nil || table["gate-enabled"] != false {
		t.Errorf("a disabled list with a base time: %v %v", table, err)
	}
	table, err = gatesForm(t, url.Values{"cycle": {""}, "base_s": {"0"}, "base_ns": {"0"}, "entry_interval_0": {""}})
	if err != nil || table != nil {
		t.Errorf("an empty form must remove the table: %v %v", table, err)
	}
}

func postForm(t *testing.T, h http.HandlerFunc, target, name string, form url.Values) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, target, strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.SetPathValue("name", name)
	w := httptest.NewRecorder()
	h(w, req)
	return w
}

func TestSaveCBS(t *testing.T) {
	rc := &recordingFetcher{MockFetcher: testutil.NewMockFetcher()}
	h := &ConfigureInterfacesHandler{RC: rc}
	w := postForm(t, h.SaveCBS, "/configure/interfaces/e1/cbs", "e1", url.Values{"idle_7": {"20"}, "idle_6": {""}})
	if w.Code != http.StatusOK || rc.putCalls != 1 {
		t.Fatalf("code %d puts %d: %s", w.Code, rc.putCalls, w.Body.String())
	}
	if rc.lastPath != candidatePath+"/ietf-interfaces:interfaces/interface=e1/ieee802-dot1dc-cbsa-if:cbsa" {
		t.Errorf("path %s", rc.lastPath)
	}
	got, _ := json.Marshal(rc.lastBody)
	if want := `{"ieee802-dot1dc-cbsa-if:cbsa":{"cbsa-parameter-table":[{"admin-idle-slope":"20000000","traffic-class":7}]}}`; string(got) != want {
		t.Errorf("body %s", got)
	}

	w = postForm(t, h.SaveCBS, "/configure/interfaces/e1/cbs", "e1", url.Values{"idle_7": {""}})
	if w.Code != http.StatusOK || len(rc.deletePaths) != 1 || !strings.HasSuffix(rc.deletePaths[0], "/ieee802-dot1dc-cbsa-if:cbsa") {
		t.Errorf("empty table must delete: %d %v", w.Code, rc.deletePaths)
	}

	w = postForm(t, h.SaveCBS, "/configure/interfaces/e1/cbs", "e1", url.Values{"idle_7": {"-1"}})
	if w.Code == http.StatusOK && !strings.Contains(w.Body.String(), "above zero") {
		t.Errorf("a negative slope was saved: %s", w.Body.String())
	}
}

func TestSaveGates(t *testing.T) {
	rc := &recordingFetcher{MockFetcher: testutil.NewMockFetcher()}
	h := &ConfigureInterfacesHandler{RC: rc}
	w := postForm(t, h.SaveGates, "/configure/interfaces/e1/gates", "e1",
		url.Values{"gate_enabled": {"on"}, "entry_open_0": {"7"}, "entry_interval_0": {"20"}})
	if w.Code != http.StatusOK || rc.putCalls != 1 || !strings.HasSuffix(rc.lastPath, "/ieee802-dot1dc-sched-if:gate-parameter-table") {
		t.Fatalf("code %d puts %d path %s: %s", w.Code, rc.putCalls, rc.lastPath, w.Body.String())
	}
	w = postForm(t, h.SaveGates, "/configure/interfaces/e1/gates", "e1", url.Values{})
	if w.Code != http.StatusOK || len(rc.deletePaths) != 1 {
		t.Errorf("empty form must delete: %d %v", w.Code, rc.deletePaths)
	}
}

// The details page shows what the port runs; nothing when it runs no shaper.
func TestTSNStatus(t *testing.T) {
	var oper interfacesWrapper
	if err := json.Unmarshal([]byte(tsnOperFixture), &oper); err != nil {
		t.Fatal(err)
	}
	st := tsnStatusOf(&oper.Interfaces.Interface[0])
	if st == nil {
		t.Fatal("no status for a port that shapes")
	}
	if len(st.Shapers) != 1 || st.Shapers[0] != (ifaceShaperStatus{7, "20"}) {
		t.Errorf("shapers: %+v", st.Shapers)
	}
	if st.CycleUs != "100" || !st.Pending || len(st.Gates) != 2 ||
		st.Gates[0] != (ifaceGateStatus{0, "7, 6", "20"}) || st.Gates[1].Open != "5, 4, 3, 2, 1, 0" {
		t.Errorf("gates: %+v", st)
	}
	if st := tsnStatusOf(&oper.Interfaces.Interface[1]); st != nil {
		t.Errorf("status for a port that runs no shaper: %+v", st)
	}
}

// On a port with its own settings a slope shapes its class and a cleared
// one stops it; with a profile only the slopes are written.
func TestSaveCBSShapesClasses(t *testing.T) {
	for _, tc := range []struct {
		name, qos string
		want      []string
	}{
		{"own settings", `{"egress":{"traffic-class":[{"id":6,"algorithm":"ieee802-dot1q-types:credit-based-shaper"}]}}`, []string{
			"GET /ds/ietf-datastores:candidate/ietf-interfaces:interfaces/interface=e1/infix-interfaces:qos",
			"DELETE /ds/ietf-datastores:candidate/ietf-interfaces:interfaces/interface=e1/infix-interfaces:qos/egress/traffic-class=6/algorithm",
			`PATCH /ds/ietf-datastores:candidate/ietf-interfaces:interfaces/interface=e1/infix-interfaces:qos {"infix-interfaces:qos":{"egress":{"traffic-class":[{"algorithm":"ieee802-dot1q-types:credit-based-shaper","id":7}]}}}`,
			`PUT /ds/ietf-datastores:candidate/ietf-interfaces:interfaces/interface=e1/ieee802-dot1dc-cbsa-if:cbsa {"ieee802-dot1dc-cbsa-if:cbsa":{"cbsa-parameter-table":[{"admin-idle-slope":"20000000","traffic-class":7}]}}`,
		}},
		{"profile", `{"profile":"tsn-ring"}`, []string{
			"GET /ds/ietf-datastores:candidate/ietf-interfaces:interfaces/interface=e1/infix-interfaces:qos",
			`PUT /ds/ietf-datastores:candidate/ietf-interfaces:interfaces/interface=e1/ieee802-dot1dc-cbsa-if:cbsa {"ieee802-dot1dc-cbsa-if:cbsa":{"cbsa-parameter-table":[{"admin-idle-slope":"20000000","traffic-class":7}]}}`,
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var got []string
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				body, _ := io.ReadAll(r.Body)
				line := r.Method + " " + r.URL.EscapedPath()
				if len(body) > 0 {
					var v any
					json.Unmarshal(body, &v) //nolint:errcheck
					b, _ := json.Marshal(v)
					line += " " + string(b)
				}
				got = append(got, line)
				if r.Method == http.MethodGet {
					w.Header().Set("Content-Type", "application/yang-data+json")
					io.WriteString(w, `{"ietf-interfaces:interfaces":{"interface":[{"name":"e1","infix-interfaces:qos":`+tc.qos+`}]}}`) //nolint:errcheck
					return
				}
				w.WriteHeader(http.StatusNoContent)
			}))
			defer srv.Close()

			h := &ConfigureInterfacesHandler{RC: restconf.NewClient(srv.URL, false)}
			req := httptest.NewRequest(http.MethodPost, "/configure/interfaces/e1/cbs",
				strings.NewReader(url.Values{"idle_7": {"20"}}.Encode()))
			req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
			req.SetPathValue("name", "e1")
			h.SaveCBS(httptest.NewRecorder(), req)

			if strings.Join(got, "\n") != strings.Join(tc.want, "\n") {
				t.Errorf("requests:\n%s\nwant:\n%s", strings.Join(got, "\n"), strings.Join(tc.want, "\n"))
			}
		})
	}
}

// Empty inline containers, as the candidate holds them, are not settings.
func TestQoSCustom(t *testing.T) {
	for _, tc := range []struct {
		json string
		want bool
	}{
		{`{"egress": {"traffic-class-table": {}, "remark": {}}}`, false},
		{`{"egress": {"traffic-class": [{"id": 7, "algorithm": "ieee802-dot1q-types:credit-based-shaper"}]}}`, true},
		{`{"ingress": {"trust": "dscp"}}`, true},
		{`{"profile": "edge"}`, false},
	} {
		var q qosIfaceJSON
		if err := json.Unmarshal([]byte(tc.json), &q); err != nil {
			t.Fatal(err)
		}
		if q.Custom() != tc.want {
			t.Errorf("%s: custom %v, want %v", tc.json, !tc.want, tc.want)
		}
	}
}

func TestIfaceDetailRendersTSN(t *testing.T) {
	tmpl := realTemplates(t, nil, "layouts/*.html", "pages/iface-detail.html", "fragments/iface-counters.html", "fragments/hw-detail.html")
	var oper interfacesWrapper
	if err := json.Unmarshal([]byte(tsnOperFixture), &oper); err != nil {
		t.Fatal(err)
	}
	for i, want := range [][]string{
		{"Credit-Based Shaper", "Class 7", "20 Mbps", "Time-Aware Shaper", "100 µs", "Change pending", "7, 6", "80 µs"},
		nil,
	} {
		iface := &oper.Interfaces.Interface[i]
		data := buildDetailData(httptest.NewRequest(http.MethodGet, "/interfaces/"+iface.Name, nil), iface)
		data.TSN = tsnStatusOf(iface)
		var buf bytes.Buffer
		if err := tmpl.ExecuteTemplate(&buf, "content", data); err != nil {
			t.Fatalf("render: %v", err)
		}
		out := buf.String()
		for _, w := range want {
			if !strings.Contains(out, w) {
				t.Errorf("%s: missing %q", iface.Name, w)
			}
		}
		if want == nil && strings.Contains(out, "Shapers") {
			t.Errorf("%s: shaper card on a port that runs none", iface.Name)
		}
	}
}

// Resetting a slope on a port with its own settings stops shaping the
// class too; the other slopes keep theirs.
func TestResetCBS(t *testing.T) {
	var got []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = append(got, r.Method+" "+r.URL.EscapedPath())
		if r.Method == http.MethodGet {
			w.Header().Set("Content-Type", "application/yang-data+json")
			if strings.HasSuffix(r.URL.Path, "cbsa") {
				io.WriteString(w, `{"ietf-interfaces:interfaces":{"interface":[{"name":"e1","ieee802-dot1dc-cbsa-if:cbsa":{"cbsa-parameter-table":[{"traffic-class":7,"admin-idle-slope":"20000000"},{"traffic-class":6,"admin-idle-slope":"1000000"}]}}]}}`) //nolint:errcheck
				return
			}
			io.WriteString(w, `{"ietf-interfaces:interfaces":{"interface":[{"name":"e1","infix-interfaces:qos":{"egress":{"traffic-class":[{"id":7,"algorithm":"ieee802-dot1q-types:credit-based-shaper"},{"id":6,"algorithm":"ieee802-dot1q-types:credit-based-shaper"}]}}}]}}`) //nolint:errcheck
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()

	h := &ConfigureInterfacesHandler{RC: restconf.NewClient(srv.URL, false)}
	req := httptest.NewRequest(http.MethodDelete, "/configure/interfaces/e1/cbs/7", nil)
	req.SetPathValue("name", "e1")
	req.SetPathValue("class", "7")
	h.ResetCBS(httptest.NewRecorder(), req)

	base := "/ds/ietf-datastores:candidate/ietf-interfaces:interfaces/interface=e1"
	want := []string{
		"GET " + base + "/ieee802-dot1dc-cbsa-if:cbsa",
		"DELETE " + base + "/ieee802-dot1dc-cbsa-if:cbsa/cbsa-parameter-table=7",
		"GET " + base + "/infix-interfaces:qos",
		"DELETE " + base + "/infix-interfaces:qos/egress/traffic-class=7/algorithm",
	}
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Errorf("requests:\n%s\nwant:\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
}

func TestResetMaxSDU(t *testing.T) {
	var got []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		got = append(got, r.Method+" "+r.URL.EscapedPath()+" "+string(body))
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()

	h := &ConfigureInterfacesHandler{RC: restconf.NewClient(srv.URL, false)}
	req := httptest.NewRequest(http.MethodDelete, "/configure/interfaces/e1/gates/max-sdu", nil)
	req.SetPathValue("name", "e1")
	h.ResetMaxSDU(httptest.NewRecorder(), req)

	// One YANG patch removing every class's entry; remove, unlike
	// delete, does not fail on an entry that is not there
	if len(got) != 1 || !strings.HasPrefix(got[0], "PATCH /ds/ietf-datastores:candidate ") ||
		strings.Count(got[0], `"operation":"remove"`) != qosNumClasses ||
		!strings.Contains(got[0], "gate-parameter-table/queue-max-sdu-table=5") {
		t.Errorf("requests %v", got)
	}
}

// The form's bounds are the model's, in the form's units.
func TestScaledBounds(t *testing.T) {
	dir := t.TempDir()
	mod := `module t { namespace "urn:t"; prefix t;
  container c {
    leaf rate { type uint64 { range "8..max"; } }
    leaf ns { type uint32; }
  }
}`
	if err := os.WriteFile(filepath.Join(dir, "t.yang"), []byte(mod), 0o644); err != nil {
		t.Fatal(err)
	}
	c := schema.NewCache(nil, dir, "")
	if err := c.LoadFromCache(); err != nil || c.Manager() == nil {
		t.Fatalf("load: %v", err)
	}
	if lo, hi := scaledBounds(c.Manager(), "/t:c/rate", 1e6); lo != "0.000008" || hi != "18446744073709.55" {
		t.Errorf("rate in Mbps: %s..%s", lo, hi)
	}
	if lo, hi := scaledBounds(c.Manager(), "/t:c/ns", 1e3); lo != "0" || hi != "4294967.295" {
		t.Errorf("ns in µs: %s..%s", lo, hi)
	}
	if lo, hi := scaledBounds(c.Manager(), "/t:c/none", 1); lo != "" || hi != "" {
		t.Errorf("missing leaf: %q..%q", lo, hi)
	}
}

// The parsers keep a value within its integer width, nothing more: the
// model's range is the device's to check.
func TestFieldParsersBounds(t *testing.T) {
	if _, err := mbitField("2000000", "rate"); err != nil {
		t.Errorf("2 Tbps refused: %v", err)
	}
	if _, err := mbitField("2e13", "rate"); err == nil {
		t.Error("a rate beyond uint64 accepted")
	}
	req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader("a=4294967.295&b=4294968"))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.ParseForm()
	if _, ok, err := usField(req, "a", "a"); !ok || err != nil {
		t.Errorf("uint32 max refused: %v", err)
	}
	if _, _, err := usField(req, "b", "b"); err == nil {
		t.Error("an interval beyond uint32 accepted")
	}
}

// The base time takes every nanosecond below one second, also where int is 32 bits.
func TestQoSGatesBaseTime(t *testing.T) {
	table, err := gatesForm(t, url.Values{"base_s": {"1700000000"}, "base_ns": {"999999999"}})
	if err != nil {
		t.Fatal(err)
	}
	base := table["admin-base-time"].(map[string]any)
	if base["seconds"] != "1700000000" || base["nanoseconds"] != uint64(999999999) {
		t.Errorf("base time %v", base)
	}
	if _, err := gatesForm(t, url.Values{"base_ns": {"1000000000"}}); err == nil {
		t.Error("a full second of nanoseconds accepted")
	}
}

// Nodes of the gate parameter table that the form does not hold survive a save.
func TestSaveGatesKeepsUnknown(t *testing.T) {
	table := map[string]any{"gate-enabled": false}
	current := map[string]any{"gate-enabled": true, "admin-gate-states": 255.0, "admin-cycle-time-extension": 5.0}
	keepUnknown(table, current, qosGateFields)
	if table["admin-gate-states"] != 255.0 || table["gate-enabled"] != false {
		t.Errorf("table %v", table)
	}
	if _, ok := table["admin-cycle-time-extension"]; ok {
		t.Errorf("a cleared field came back: %v", table)
	}
}

// A rate that does not parse is refused before the candidate is touched.
func TestSaveQoSBadRateChangesNothing(t *testing.T) {
	rc := &recordingFetcher{MockFetcher: testutil.NewMockFetcher()}
	h := &ConfigureInterfacesHandler{RC: rc}
	postForm(t, h.SaveQoS, "/configure/interfaces/e1/qos", "e1", url.Values{"rate": {"fast"}})
	if len(rc.deletePaths) != 0 || rc.putCalls != 0 {
		t.Errorf("deletes %v puts %d", rc.deletePaths, rc.putCalls)
	}
}
