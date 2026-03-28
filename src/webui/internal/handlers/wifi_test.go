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

var minimalWiFiTmpl = template.Must(template.New("wifi.html").Parse(
	`{{define "wifi.html"}}radios={{len .Radios}}{{end}}` +
		`{{define "content"}}{{len .Radios}}{{end}}`,
))

func TestWiFiOverview_ReturnsOK(t *testing.T) {
	rc := restconf.NewClient("http://127.0.0.1:19999/restconf", false)
	h := &WiFiHandler{Template: minimalWiFiTmpl, RC: rc}

	req := httptest.NewRequest(http.MethodGet, "/wifi", nil)
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

func TestWiFiOverview_HTMXPartial(t *testing.T) {
	rc := restconf.NewClient("http://127.0.0.1:19999/restconf", false)
	h := &WiFiHandler{Template: minimalWiFiTmpl, RC: rc}

	req := httptest.NewRequest(http.MethodGet, "/wifi", nil)
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
