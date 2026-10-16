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

const scanActionPath = "/data/ietf-hardware:hardware/component=radio0/infix-hardware:wifi-radio/scan"

// fakeScanAction serves the scan action on radio0 with reply.
func fakeScanAction(t *testing.T, reply string, status int) *httptest.Server {
	t.Helper()
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != scanActionPath || r.Method != http.MethodPost {
			t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
			http.Error(w, "unexpected", http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "application/yang-data+json")
		w.WriteHeader(status)
		io.WriteString(w, reply) //nolint:errcheck
	}))
	t.Cleanup(srv.Close)
	return srv
}

func scanRequest(t *testing.T, srv *httptest.Server) string {
	t.Helper()
	h := &WiFiHandler{
		Template: realTemplates(t, nil, "layouts/*.html", "pages/wifi.html"),
		RC:       restconf.NewClient(srv.URL, true),
	}
	req := httptest.NewRequest(http.MethodPost, "/wifi/radio0/scan", nil)
	req.SetPathValue("name", "radio0")
	req = req.WithContext(restconf.ContextWithCredentials(req.Context(),
		restconf.Credentials{Username: "admin", Password: "secret"}))
	rec := httptest.NewRecorder()
	h.Scan(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d, want 200", rec.Code)
	}
	return rec.Body.String()
}

func TestWiFiScanListsNetworks(t *testing.T) {
	reply := `{"infix-hardware:output":{"network":[
		{"bssid":"b4:fb:e4:17:b6:a7","ssid":"MyNetwork","frequency":2437,"channel":6,
		 "signal-strength":-45,"encryption":["WPA2-Personal"]},
		{"bssid":"0a:a0:85:00:01:00","frequency":5180,"channel":36,"signal-strength":-70,
		 "encryption":["WPA3-Personal","WPA2-Personal"]}]}}`
	out := scanRequest(t, fakeScanAction(t, reply, http.StatusOK))

	for _, want := range []string{"MyNetwork", "b4:fb:e4:17:b6:a7", "-45 dBm", "WPA2-Personal",
		"<em>Hidden</em>", "WPA3-Personal, WPA2-Personal", "Scan again", `hx-post="/wifi/radio0/scan"`} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in\n%s", want, out)
		}
	}
	if strings.Contains(out, "alert-error") {
		t.Errorf("unexpected error in\n%s", out)
	}
}

func TestWiFiScanShowsDeviceError(t *testing.T) {
	reply := `{"ietf-restconf:errors":{"error":[{"error-type":"application",
		"error-tag":"operation-failed",
		"error-message":"Scan failed: no interface on radio0 to scan with"}]}}`
	out := scanRequest(t, fakeScanAction(t, reply, http.StatusInternalServerError))

	for _, want := range []string{"alert-error", "no interface on radio0", "Scan networks"} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in\n%s", want, out)
		}
	}
	if strings.Contains(out, "data-table") {
		t.Errorf("table rendered on error:\n%s", out)
	}
}

func TestWiFiPageOffersNetworkScan(t *testing.T) {
	tmpl := realTemplates(t, nil, "layouts/*.html", "pages/wifi.html")
	comps := []hwComponentWiFiJSON{{Name: "radio0", WiFiRadio: &wifiRadioHWJSON{Band: "2.4GHz"}}}
	data := wifiData{Radios: buildWiFiRadios(comps, nil)}

	var buf strings.Builder
	if err := tmpl.ExecuteTemplate(&buf, "content", data); err != nil {
		t.Fatalf("render: %v", err)
	}
	out := buf.String()
	for _, want := range []string{`id="wifi-scan-radio0"`, `hx-post="/wifi/radio0/scan"`, "Scan networks"} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in\n%s", want, out)
		}
	}
}
