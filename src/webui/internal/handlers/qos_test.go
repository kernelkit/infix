// SPDX-License-Identifier: MIT

package handlers

import (
	"html/template"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"infix/webui/internal/restconf"
	"infix/webui/internal/security"
	"infix/webui/internal/testutil"
)

// pageTemplate parses a page with the layouts and fragments, as the
// server does.
func pageTemplate(t *testing.T, page string) *template.Template {
	t.Helper()
	return realTemplates(t, IfaceTemplateFuncs(), "layouts/*.html", "fragments/*.html", "pages/"+page)
}

func testRequest(method, target string) *http.Request {
	req := httptest.NewRequest(method, target, nil)
	req.Header.Set("HX-Request", "true")
	ctx := restconf.ContextWithCredentials(req.Context(), restconf.Credentials{Username: "admin", Password: "admin"})
	ctx = security.WithToken(ctx, "test-csrf-token")
	ctx = ContextWithCapabilities(ctx, NewCapabilities(map[string]bool{"qos": true}))
	return req.WithContext(ctx)
}

func TestQoSOverview(t *testing.T) {
	mock := testutil.NewMockFetcher()
	mock.SetResponse("/data/ietf-interfaces:interfaces", map[string]any{
		"ietf-interfaces:interfaces": map[string]any{
			"interface": []map[string]any{
				{"name": "e1", "type": "infix-if-type:ethernet", "infix-interfaces:qos": map[string]any{
					"active-profile": "tsn-ring",
					"ingress":        map[string]any{"trust": "pcp"},
					"egress-shaper":  map[string]any{"rate": "100000000"},
					"capabilities": map[string]any{
						"max-traffic-classes": 4,
						"offload":             []string{"classification", "remarking", "transmission-selection", "egress-shaper"},
					},
				}},
				{"name": "e2", "type": "infix-if-type:ethernet", "infix-interfaces:qos": map[string]any{
					"capabilities": map[string]any{"offload": []string{"classification"}},
				}},
				{"name": "e10", "type": "infix-if-type:ethernet",
					"infix-interfaces:qos": map[string]any{"capabilities": map[string]any{"max-traffic-classes": 8}},
					"ieee802-dot1dc-cbsa-if:cbsa": map[string]any{"cbsa-parameter-table": []map[string]any{
						{"traffic-class": 6, "oper-idle-slope": "20000000"},
						{"traffic-class": 7, "oper-idle-slope": "50000000"},
						{"traffic-class": 5, "admin-idle-slope": "1000000"}}},
					"ieee802-dot1dc-sched-if:gate-parameter-table": map[string]any{
						"oper-cycle-time": map[string]any{"numerator": 1, "denominator": 1000},
						"oper-control-list": map[string]any{"gate-control-entry": []map[string]any{
							{"index": 0, "gate-states-value": 128, "time-interval-value": 200000},
							{"index": 1, "gate-states-value": 127, "time-interval-value": 800000}}}}},
				{"name": "lo", "type": "infix-if-type:loopback",
					"infix-interfaces:qos": map[string]any{"capabilities": map[string]any{"max-traffic-classes": 8}}},
			},
		},
	})
	h := &QoSHandler{Template: pageTemplate(t, "qos.html"), RC: mock}
	w := httptest.NewRecorder()
	h.Overview(w, testRequest(http.MethodGet, "/qos"))
	if w.Code != http.StatusOK {
		t.Fatalf("want 200 got %d: %s", w.Code, w.Body.String())
	}
	body := w.Body.String()
	for _, want := range []string{"tsn-ring", "100 Mbps", "<td>4</td>", "<td>classification, remarking, transmission-selection, egress-shaper</td>", "<td>classification</td>", "<td>8</td>",
		"<td>pcp</td>", "<td>7: 50 Mbps, 6: 20 Mbps</td>", "<td>1000 µs cycle, 2 entries</td>"} {
		if !strings.Contains(body, want) {
			t.Errorf("missing %q in:\n%s", want, body)
		}
	}
	if strings.Contains(body, ">lo<") {
		t.Errorf("loopback listed")
	}
	if e2, e10 := strings.Index(body, ">e2<"), strings.Index(body, ">e10<"); e2 < 0 || e10 < e2 {
		t.Errorf("e10 sorted before e2")
	}
}
