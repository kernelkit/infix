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
					"profile":    "tsn-ring",
					"rate-limit": map[string]any{"rate": "100000000"},
					"capabilities": map[string]any{
						"max-traffic-classes": 4,
						"offload":             []string{"classification", "remarking", "transmission-selection", "rate-limit"},
					},
				}},
				{"name": "e2", "type": "infix-if-type:ethernet", "infix-interfaces:qos": map[string]any{
					"capabilities": map[string]any{"offload": []string{"classification"}},
				}},
				{"name": "lo", "type": "infix-if-type:loopback"},
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
	for _, want := range []string{"tsn-ring", "100 Mbit/s", "<td>4</td>", "<td>classification, remarking, transmission-selection, rate-limit</td>", "<td>classification</td>", "<td>8</td>"} {
		if !strings.Contains(body, want) {
			t.Errorf("missing %q in:\n%s", want, body)
		}
	}
	if strings.Contains(body, ">lo<") {
		t.Errorf("loopback without qos state listed")
	}
}
