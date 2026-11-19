// SPDX-License-Identifier: MIT

package handlers

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"infix/webui/internal/restconf"
)

// The proxy relays method, path, query and body as the logged-in user,
// and hands back the device's own status and body.
func TestRestconfProxy(t *testing.T) {
	var got struct{ method, uri, user, body string }
	dev := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		got.method, got.uri, got.body = r.Method, r.URL.RequestURI(), string(b)
		got.user, _, _ = r.BasicAuth()
		if strings.Contains(r.URL.Path, "denied") {
			w.Header().Set("Content-Type", "application/yang-data+json")
			w.WriteHeader(http.StatusForbidden)
			io.WriteString(w, `{"ietf-restconf:errors":{}}`)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer dev.Close()
	h := &RestconfProxy{RC: restconf.NewClient(dev.URL+"/restconf", false)}

	for _, tc := range []struct {
		method, url, body string
		want              int
		uri               string
	}{
		{http.MethodGet, "/api/restconf/data/ietf-hardware:hardware?depth=2", "", http.StatusNoContent, "/restconf/data/ietf-hardware:hardware?depth=2"},
		{http.MethodPost, "/api/restconf/data/ietf-hardware:hardware/component=x/act", `{"in":1}`, http.StatusNoContent, "/restconf/data/ietf-hardware:hardware/component=x/act"},
		{http.MethodPatch, "/api/restconf/data/denied", `{}`, http.StatusForbidden, "/restconf/data/denied"},
		{http.MethodGet, "/api/restconf/data/ietf-routing:routing/route=10.0.0.0%2F24", "", http.StatusNoContent, "/restconf/data/ietf-routing:routing/route=10.0.0.0%2F24"},
		{http.MethodGet, "/api/restconf/yang/x.yang", "", http.StatusNotFound, ""},
		{http.MethodGet, "/api/restconf/database", "", http.StatusNotFound, ""},
		{http.MethodGet, "/api/restconf/data/../../x", "", http.StatusBadGateway, ""},
		{http.MethodOptions, "/api/restconf/data/x", "", http.StatusMethodNotAllowed, ""},
	} {
		got.uri, got.body = "", ""
		r := httptest.NewRequest(tc.method, tc.url, strings.NewReader(tc.body))
		r = r.WithContext(restconf.ContextWithCredentials(r.Context(), restconf.Credentials{Username: "guest", Password: "pw"}))
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		if w.Code != tc.want {
			t.Errorf("%s %s: status %d, want %d", tc.method, tc.url, w.Code, tc.want)
		}
		if got.uri != tc.uri {
			t.Errorf("%s %s: device got %q, want %q", tc.method, tc.url, got.uri, tc.uri)
		}
		if tc.uri != "" && (got.method != tc.method || got.body != tc.body || got.user != "guest") {
			t.Errorf("%s %s: device got %s as %q with body %q", tc.method, tc.url, got.method, got.user, got.body)
		}
	}
}
