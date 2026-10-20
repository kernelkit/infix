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
