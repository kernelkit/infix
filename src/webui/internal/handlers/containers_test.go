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
)

var minimalContainersTmpl = template.Must(template.New("containers.html").Parse(
	`{{define "containers.html"}}count={{len .Containers}}{{end}}` +
		`{{define "content"}}{{len .Containers}}{{end}}`,
))

func TestContainersOverview_ReturnsOK(t *testing.T) {
	rc := restconf.NewClient("http://127.0.0.1:19999/restconf", false)
	h := &ContainersHandler{Template: minimalContainersTmpl, RC: rc}

	req := httptest.NewRequest(http.MethodGet, "/containers", nil)
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

func TestContainersOverview_HTMXPartial(t *testing.T) {
	rc := restconf.NewClient("http://127.0.0.1:19999/restconf", false)
	h := &ContainersHandler{Template: minimalContainersTmpl, RC: rc}

	req := httptest.NewRequest(http.MethodGet, "/containers", nil)
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

// Without containers there is no data, and RESTCONF answers 404.  That
// is the empty list, not an error.
func TestContainersOverview_NoneConfigured(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/yang-data+json")
		w.WriteHeader(http.StatusNotFound)
		w.Write([]byte(`{"ietf-restconf:errors":{"error":[{"error-type":"application",` +
			`"error-tag":"invalid-value","error-message":"No data from sysrepo."}]}}`))
	}))
	defer srv.Close()

	tmpl := realTemplates(t, nil, "layouts/*.html", "pages/containers.html")
	h := &ContainersHandler{Template: tmpl, RC: restconf.NewClient(srv.URL+"/restconf", false)}

	req := httptest.NewRequest(http.MethodGet, "/containers", nil)
	req.Header.Set("HX-Request", "true")
	ctx := restconf.ContextWithCredentials(req.Context(), restconf.Credentials{
		Username: "admin",
		Password: "admin",
	})
	req = req.WithContext(security.WithToken(ctx, "test-csrf-token"))

	w := httptest.NewRecorder()
	h.Overview(w, req)

	body := w.Body.String()
	if strings.Contains(body, "Could not fetch") {
		t.Errorf("404 reported as an error: %s", body)
	}
	if !strings.Contains(body, "No containers configured.") {
		t.Errorf("empty list not shown: %s", body)
	}
}
