// SPDX-License-Identifier: MIT

package handlers

import (
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"infix/webui/internal/restconf"
)

// Locate says how long the LEDs blink and offers to stop them, and a
// stop clears the result.
func TestLocateStartStop(t *testing.T) {
	var body string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		body = string(b)
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()
	h := &HardwareHandler{RC: restconf.NewClient(srv.URL+"/restconf", false)}

	post := func(form url.Values) string {
		req := httptest.NewRequest(http.MethodPost, "/hardware/locate", strings.NewReader(form.Encode()))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		req = req.WithContext(restconf.ContextWithCredentials(req.Context(),
			restconf.Credentials{Username: "admin", Password: "admin"}))
		w := httptest.NewRecorder()
		h.Locate(w, req)
		return w.Body.String()
	}

	out := post(url.Values{})
	if !strings.Contains(body, `"duration":60`) || !strings.Contains(body, `"enable":true`) {
		t.Errorf("start sent %s", body)
	}
	if !strings.Contains(out, "60 seconds") || !strings.Contains(out, ">Stop</button>") {
		t.Errorf("start answered %s", out)
	}
	// Stop inherits the result's outerHTML swap, so it must target the
	// result and not its container, which Locate needs for next time
	if strings.Contains(out, `hx-target="#hw-locate-result"`) {
		t.Errorf("Stop replaces the Locate target: %s", out)
	}

	out = post(url.Values{"enable": {"false"}})
	if !strings.Contains(body, `"enable":false`) || strings.Contains(body, "duration") {
		t.Errorf("stop sent %s", body)
	}
	if out != "" {
		t.Errorf("stop answered %q, want nothing", out)
	}
}
