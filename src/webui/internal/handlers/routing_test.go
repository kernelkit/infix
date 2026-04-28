// SPDX-License-Identifier: MIT

package handlers

import (
	"html/template"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/kernelkit/webui/internal/restconf"
	"github.com/kernelkit/webui/internal/security"
)

var minimalRoutingTmpl = template.Must(template.New("routing.html").Parse(
	`{{define "routing.html"}}routes={{len .Routes}}{{end}}` +
		`{{define "content"}}{{len .Routes}}{{end}}`,
))

func TestRoutingOverview_ReturnsOK(t *testing.T) {
	rc := restconf.NewClient("http://127.0.0.1:19999/restconf", false)
	h := &RoutingHandler{Template: minimalRoutingTmpl, RC: rc}

	req := httptest.NewRequest(http.MethodGet, "/routing", nil)
	ctx := restconf.ContextWithCredentials(req.Context(), restconf.Credentials{
		Username: "admin",
		Password: "admin",
	})
	ctx = security.WithToken(ctx, "test-csrf-token")
	req = req.WithContext(ctx)

	w := httptest.NewRecorder()
	h.Overview(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("want 200 got %d; body: %s", w.Code, w.Body.String())
	}

	body := w.Body.String()
	if body == "" {
		t.Error("expected non-empty response body")
	}
}

func TestRoutingOverview_HTMXPartial(t *testing.T) {
	rc := restconf.NewClient("http://127.0.0.1:19999/restconf", false)
	h := &RoutingHandler{Template: minimalRoutingTmpl, RC: rc}

	req := httptest.NewRequest(http.MethodGet, "/routing", nil)
	req.Header.Set("HX-Request", "true")
	ctx := restconf.ContextWithCredentials(req.Context(), restconf.Credentials{
		Username: "admin",
		Password: "admin",
	})
	ctx = security.WithToken(ctx, "test-csrf-token")
	req = req.WithContext(ctx)

	w := httptest.NewRecorder()
	h.Overview(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("want 200 got %d; body: %s", w.Code, w.Body.String())
	}

	body := w.Body.String()
	if body == "" {
		t.Error("expected non-empty response body for htmx partial")
	}
}
