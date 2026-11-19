// SPDX-License-Identifier: MIT

package handlers

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"reflect"
	"strings"
	"testing"

	"infix/webui/internal/testutil"
)

func TestConfigurePTPOverview(t *testing.T) {
	mock := testutil.NewMockFetcher()
	mock.SetResponse(candidatePath+ptpPath, map[string]any{
		"ieee1588-ptp-tt:ptp": map[string]any{"instances": map[string]any{"instance": []map[string]any{{
			"instance-index": 0,
			"default-ds":     map[string]any{"infix-ptp:profile": "ieee802-dot1as", "priority1": 1},
			"ports": map[string]any{"port": []map[string]any{
				{"port-index": 1, "underlying-interface": "e10"},
				{"port-index": 2, "underlying-interface": "e2"},
			}},
		}}}},
	})
	mock.SetResponse("/data/ietf-interfaces:interfaces", map[string]any{
		"ietf-interfaces:interfaces": map[string]any{"interface": []map[string]any{
			{"name": "e10", "infix-interfaces:ptp-capabilities": map[string]any{"phc-index": 0}},
			{"name": "e2", "infix-interfaces:ptp-capabilities": map[string]any{"phc-index": 0}},
			{"name": "e3", "infix-interfaces:ptp-capabilities": map[string]any{"phc-index": 0}},
			{"name": "lo", "type": "infix-if-type:loopback", "infix-interfaces:ptp-capabilities": map[string]any{}},
			{"name": "br0"},
		}},
	})
	h := &ConfigurePTPHandler{
		Template: pageTemplate(t, "configure-ptp.html"),
		RC:       mock,
		Schema:   repoModules(t, mock, "ieee1588-ptp-tt", "infix-ptp", "ieee802-dot1as-gptp", "ietf-interfaces", "ietf-yang-types"),
	}
	w := httptest.NewRecorder()
	h.Overview(w, testRequest(http.MethodGet, "/configure/ptp"))
	if w.Code != http.StatusOK {
		t.Fatalf("want 200 got %d: %s", w.Code, w.Body.String())
	}
	body := w.Body.String()
	for _, want := range []string{
		"<td>e2, e10</td>", // natural order
		`value="ieee802-dot1as" title=`,
		`name="priority1" step="1" style="width:6rem"` + "\n" + `       min="0" max="255" value="1"`,
		`name="port" value="e3" >`,
		`name="port" value="e2" checked>`,
		`name="index" min="0" step="1" required value="1"`,
		"Boundary Clock",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("missing %q", want)
		}
	}
	for _, name := range []string{"lo", "br0"} {
		if strings.Contains(body, `value="`+name+`"`) {
			t.Errorf("%s offered as a PTP port", name)
		}
	}
}

func TestPTPPortsFor(t *testing.T) {
	current := []ptpPortJSON{{Index: 1, Interface: "e1"}, {Index: 3, Interface: "e3"}}
	add, remove := ptpPortsFor(current, []string{"e3", "e5", "e6"})
	wantAdd := []map[string]any{
		{"port-index": 2, "underlying-interface": "e5"},
		{"port-index": 4, "underlying-interface": "e6"},
	}
	if !reflect.DeepEqual(add, wantAdd) || !reflect.DeepEqual(remove, []int{1}) {
		t.Fatalf("add %v remove %v", add, remove)
	}
}

func TestPTPDefaultDSFromForm(t *testing.T) {
	form := url.Values{"profile": {"ieee802-dot1as"}, "priority1": {"1"}, "domain-number": {""}, "time-receiver-only": {"on"}}
	req := httptest.NewRequest(http.MethodPost, "/configure/ptp/instances/0", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	if err := req.ParseForm(); err != nil {
		t.Fatal(err)
	}
	ds, err := ptpDefaultDSFromForm(req)
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]any{"infix-ptp:profile": "ieee802-dot1as", "priority1": uint64(1), "time-receiver-only": true}
	if !reflect.DeepEqual(ds, want) {
		t.Fatalf("got %v", ds)
	}

	req.Form.Set("priority1", "300")
	if _, err := ptpDefaultDSFromForm(req); err == nil {
		t.Fatal("priority1 300 accepted")
	}
}
