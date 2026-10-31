// SPDX-License-Identifier: MIT

package handlers

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

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
		{ID: 7, Shaped: true, IdleMbit: "20", OperMbit: "20 Mbit/s"},
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
	if g.Running != "running 2 entries, cycle 100 µs, change pending" {
		t.Errorf("running: %q", g.Running)
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
	if len(e2.QoSShapers) != 0 || e2.QoSGates.Enabled || e2.QoSGates.Running != "not running" {
		t.Errorf("port without tsn settings: %+v %+v", e2.QoSShapers, e2.QoSGates)
	}
}

func TestConfigureInterfacesRendersTSN(t *testing.T) {
	tmpl := realTemplates(t, IfaceTemplateFuncs(), "layouts/*.html", "fragments/configure-toolbar.html",
		"fragments/wizard-psk-picker.html", "fragments/wizard-wgkey-picker.html",
		"fragments/wizard-radio-picker.html", "pages/configure-interfaces.html")
	rows := tsnRows(t)
	data := cfgIfacePageData{
		PageData:    PageData{Capabilities: NewCapabilities(map[string]bool{"qos": true, "tsn": true, "rate-limit": true})},
		Interfaces:  rows,
		Desc:        map[string]string{},
		QoSProfiles: []string{"tsn-ring"},
	}
	var buf bytes.Buffer
	if err := tmpl.ExecuteTemplate(&buf, "content", data); err != nil {
		t.Fatalf("render: %v", err)
	}
	out := buf.String()
	for _, want := range []string{
		`hx-post="/configure/interfaces/e1/qos"`, `<option value="tsn-ring" selected>`,
		`hx-post="/configure/interfaces/e1/cbs"`, `name="idle_7"`, `value="20"`, `(not shaped)`,
		`hx-post="/configure/interfaces/e1/gates"`, `name="gate_enabled" value="on" checked`,
		`name="cycle"`, `name="entry_open_1" value="5" checked`, `name="entry_interval_0"`,
		`id="gate-entry-tpl-e1"`, `data-template="gate-entry-tpl-e1"`, `name="entry_interval___N__"`, `name="maxsdu_5"`,
		"running 2 entries, cycle 100 µs, change pending",
		"No shaped class.", `hx-post="/configure/interfaces/e2/gates"`,
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
