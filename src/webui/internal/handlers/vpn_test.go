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

var minimalVPNTmpl = template.Must(template.New("vpn.html").Parse(
	`{{define "vpn.html"}}tunnels={{len .Tunnels}}{{end}}` +
		`{{define "content"}}{{len .Tunnels}}{{end}}`,
))

func TestVPNOverview_ReturnsOK(t *testing.T) {
	rc := restconf.NewClient("http://127.0.0.1:19999/restconf", false)
	h := &VPNHandler{Template: minimalVPNTmpl, RC: rc}

	req := httptest.NewRequest(http.MethodGet, "/vpn", nil)
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

func TestVPNOverview_HTMXPartial(t *testing.T) {
	rc := restconf.NewClient("http://127.0.0.1:19999/restconf", false)
	h := &VPNHandler{Template: minimalVPNTmpl, RC: rc}

	req := httptest.NewRequest(http.MethodGet, "/vpn", nil)
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
