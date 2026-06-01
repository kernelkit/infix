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

var minimalDHCPTmpl = template.Must(template.New("dhcp.html").Parse(
	`{{define "dhcp.html"}}dhcp={{.DHCP}}{{end}}` +
		`{{define "content"}}{{.DHCP}}{{end}}`,
))

var minimalNTPTmpl = template.Must(template.New("ntp.html").Parse(
	`{{define "ntp.html"}}ntp={{.NTP}}{{end}}` +
		`{{define "content"}}{{.NTP}}{{end}}`,
))

var minimalLLDPTmpl = template.Must(template.New("lldp.html").Parse(
	`{{define "lldp.html"}}lldp={{.Neighbors}}{{end}}` +
		`{{define "content"}}{{.Neighbors}}{{end}}`,
))

func testCtx(req *http.Request) *http.Request {
	ctx := restconf.ContextWithCredentials(req.Context(), restconf.Credentials{
		Username: "admin",
		Password: "admin",
	})
	ctx = security.WithToken(ctx, "test-csrf-token")
	return req.WithContext(ctx)
}

func TestDHCPOverview_ReturnsOK(t *testing.T) {
	rc := restconf.NewClient("http://127.0.0.1:19999/restconf", false)
	h := &DHCPHandler{Template: minimalDHCPTmpl, RC: rc}

	req := httptest.NewRequest(http.MethodGet, "/dhcp", nil)
	req = testCtx(req)

	w := httptest.NewRecorder()
	h.Overview(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("want 200 got %d; body: %s", w.Code, w.Body.String())
	}
	if w.Body.String() == "" {
		t.Error("expected non-empty response body")
	}
}

func TestDHCPOverview_HTMXPartial(t *testing.T) {
	rc := restconf.NewClient("http://127.0.0.1:19999/restconf", false)
	h := &DHCPHandler{Template: minimalDHCPTmpl, RC: rc}

	req := httptest.NewRequest(http.MethodGet, "/dhcp", nil)
	req.Header.Set("HX-Request", "true")
	req = testCtx(req)

	w := httptest.NewRecorder()
	h.Overview(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("want 200 got %d; body: %s", w.Code, w.Body.String())
	}
	if w.Body.String() == "" {
		t.Error("expected non-empty response body for htmx partial")
	}
}

func TestNTPOverview_ReturnsOK(t *testing.T) {
	rc := restconf.NewClient("http://127.0.0.1:19999/restconf", false)
	h := &NTPHandler{Template: minimalNTPTmpl, RC: rc}

	req := httptest.NewRequest(http.MethodGet, "/ntp", nil)
	req = testCtx(req)

	w := httptest.NewRecorder()
	h.Overview(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("want 200 got %d; body: %s", w.Code, w.Body.String())
	}
	if w.Body.String() == "" {
		t.Error("expected non-empty response body")
	}
}

func TestLLDPOverview_ReturnsOK(t *testing.T) {
	rc := restconf.NewClient("http://127.0.0.1:19999/restconf", false)
	h := &LLDPHandler{Template: minimalLLDPTmpl, RC: rc}

	req := httptest.NewRequest(http.MethodGet, "/lldp", nil)
	req = testCtx(req)

	w := httptest.NewRecorder()
	h.Overview(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("want 200 got %d; body: %s", w.Code, w.Body.String())
	}
	if w.Body.String() == "" {
		t.Error("expected non-empty response body")
	}
}
