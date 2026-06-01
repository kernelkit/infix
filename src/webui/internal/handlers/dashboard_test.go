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

var minimalDashTmpl = template.Must(template.New("dashboard.html").Parse(
	`{{define "dashboard.html"}}hostname={{.Hostname}} error={{.Error}}{{end}}` +
		`{{define "content"}}{{.Hostname}}{{end}}`,
))

func TestDashboardIndex_ReturnsOK(t *testing.T) {
	rc := restconf.NewClient("http://127.0.0.1:19999/restconf", false)
	h := &DashboardHandler{Template: minimalDashTmpl, RC: rc}

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	ctx := restconf.ContextWithCredentials(req.Context(), restconf.Credentials{
		Username: "testuser",
		Password: "testpass",
	})
	ctx = security.WithToken(ctx, "test-csrf-token")
	req = req.WithContext(ctx)

	w := httptest.NewRecorder()
	h.Index(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("want 200 got %d", w.Code)
	}
}

func TestDashboardIndex_ShowsErrorOnRESTCONFFailure(t *testing.T) {
	rc := restconf.NewClient("http://127.0.0.1:19999/restconf", false)
	h := &DashboardHandler{Template: minimalDashTmpl, RC: rc}

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	ctx := restconf.ContextWithCredentials(req.Context(), restconf.Credentials{
		Username: "admin",
		Password: "admin",
	})
	ctx = security.WithToken(ctx, "tok")
	req = req.WithContext(ctx)

	w := httptest.NewRecorder()
	h.Index(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("want 200 got %d; body: %s", w.Code, w.Body.String())
	}

	body := w.Body.String()
	if body == "" {
		t.Error("expected non-empty response body")
	}
}

func TestDashboardIndex_HTMXPartial(t *testing.T) {
	rc := restconf.NewClient("http://127.0.0.1:19999/restconf", false)
	h := &DashboardHandler{Template: minimalDashTmpl, RC: rc}

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("HX-Request", "true")
	ctx := restconf.ContextWithCredentials(req.Context(), restconf.Credentials{
		Username: "admin",
		Password: "admin",
	})
	ctx = security.WithToken(ctx, "tok")
	req = req.WithContext(ctx)

	w := httptest.NewRecorder()
	h.Index(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("want 200 got %d; body: %s", w.Code, w.Body.String())
	}
}
