// SPDX-License-Identifier: MIT

package server

import (
	"bytes"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"infix/webui/internal/handlers"
	"infix/webui/internal/security"
)

func TestSecurityHeadersCacheControl(t *testing.T) {
	h := securityHeadersMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	for path, want := range map[string]string{"/dashboard": "no-store", "/assets/css/style.css": ""} {
		w := httptest.NewRecorder()
		h.ServeHTTP(w, httptest.NewRequest(http.MethodGet, path, nil))
		if got := w.Header().Get("Cache-Control"); got != want {
			t.Errorf("%s: Cache-Control %q, want %q", path, got, want)
		}
	}
}

// app.js posts FormData (multipart); htmx forms post urlencoded.
func TestValidCSRFFormBodies(t *testing.T) {
	for sent, want := range map[string]bool{"tok": true, "stale": false} {
		var mp bytes.Buffer
		mw := multipart.NewWriter(&mp)
		mw.WriteField("csrf", sent)
		mw.Close()

		bodies := map[string]string{
			mw.FormDataContentType():            mp.String(),
			"application/x-www-form-urlencoded": url.Values{"csrf": {sent}}.Encode(),
		}
		for ct, body := range bodies {
			r := httptest.NewRequest(http.MethodPost, "/session/timeout", strings.NewReader(body))
			r.Header.Set("Content-Type", ct)
			if got := validCSRF(r, "tok"); got != want {
				t.Errorf("csrf %q, %s: got %v, want %v", sent, ct, got, want)
			}
		}
	}
}

// An auto-refresh of a status page is background polling, it must not
// keep an idle session alive.
func TestIsPolling(t *testing.T) {
	for _, tc := range []struct {
		path, header string
		want         bool
	}{
		{"/device-status", "", true},
		{"/interfaces/eth0/counters", "", true},
		{"/interfaces", "", false},
		{"/interfaces", "1", true},
	} {
		r := httptest.NewRequest(http.MethodGet, tc.path, nil)
		if tc.header != "" {
			r.Header.Set("X-Auto-Refresh", tc.header)
		}
		if got := isPolling(r); got != tc.want {
			t.Errorf("%s (X-Auto-Refresh %q): polling %v, want %v", tc.path, tc.header, got, tc.want)
		}
	}
}

func TestAdminOnly(t *testing.T) {
	h := adminOnly(func(w http.ResponseWriter, r *http.Request) {})
	for nacm, want := range map[bool]int{true: http.StatusOK, false: http.StatusForbidden} {
		caps := handlers.NewCapabilities(map[string]bool{"nacm": nacm})
		r := httptest.NewRequest(http.MethodGet, "/maintenance/logs", nil)
		r = r.WithContext(handlers.ContextWithCapabilities(r.Context(), caps))
		w := httptest.NewRecorder()
		h(w, r)
		if w.Code != want {
			t.Errorf("nacm %v: status %d, want %d", nacm, w.Code, want)
		}
	}
}

func TestWithToken(t *testing.T) {
	h := withToken(func(w http.ResponseWriter, r *http.Request) {})
	for query, want := range map[string]int{
		"?tool=ping&target=x&csrf=tok":   http.StatusOK,
		"?tool=ping&target=x&csrf=stale": http.StatusForbidden,
		"?tool=ping&target=x":            http.StatusForbidden,
	} {
		r := httptest.NewRequest(http.MethodGet, "/maintenance/diagnostics/run"+query, nil)
		r = r.WithContext(security.WithToken(r.Context(), "tok"))
		w := httptest.NewRecorder()
		h(w, r)
		if w.Code != want {
			t.Errorf("%s: status %d, want %d", query, w.Code, want)
		}
	}
}
