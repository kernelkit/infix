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

const surveyActionPath = "/data/ietf-hardware:hardware/component=radio0/infix-hardware:wifi-radio/channel-survey"

// fakeSurveyAction serves the channel-survey action on radio0 with reply.
func fakeSurveyAction(t *testing.T, reply string, status int) *httptest.Server {
	t.Helper()
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != surveyActionPath || r.Method != http.MethodPost {
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

func surveyRequest(t *testing.T, srv *httptest.Server) string {
	t.Helper()
	h := &WiFiHandler{
		Template: realTemplates(t, nil, "layouts/*.html", "pages/wifi.html"),
		RC:       restconf.NewClient(srv.URL, true),
	}
	req := httptest.NewRequest(http.MethodPost, "/wifi/radio0/survey", nil)
	req.SetPathValue("name", "radio0")
	req = req.WithContext(restconf.ContextWithCredentials(req.Context(),
		restconf.Credentials{Username: "admin", Password: "secret"}))
	rec := httptest.NewRecorder()
	h.Survey(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d, want 200", rec.Code)
	}
	return rec.Body.String()
}

func TestWiFiSurveyRendersChart(t *testing.T) {
	reply := `{"infix-hardware:output":{"channel":[
		{"frequency":2412,"in-use":false,"noise":-92,"active-time":120,"busy-time":15},
		{"frequency":2437,"in-use":true,"noise":-92,"active-time":1000,"busy-time":125,
		 "receive-time":60,"transmit-time":30}]}}`
	out := surveyRequest(t, fakeSurveyAction(t, reply, http.StatusOK))

	for _, want := range []string{`class="survey-chart"`, "Scan again", `hx-post="/wifi/radio0/survey"`} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in\n%s", want, out)
		}
	}
	if strings.Contains(out, "alert-error") {
		t.Errorf("unexpected error in\n%s", out)
	}
}

func TestWiFiSurveyShowsDeviceError(t *testing.T) {
	reply := `{"ietf-restconf:errors":{"error":[{"error-type":"application",
		"error-tag":"operation-failed",
		"error-message":"Channel survey failed: no interface on radio0 to scan with"}]}}`
	out := surveyRequest(t, fakeSurveyAction(t, reply, http.StatusInternalServerError))

	for _, want := range []string{"alert-error", "no interface on radio0", "Scan channels"} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in\n%s", want, out)
		}
	}
	if strings.Contains(out, "survey-chart") {
		t.Errorf("chart rendered on error:\n%s", out)
	}
}

func TestWiFiPageOffersSurveyScan(t *testing.T) {
	tmpl := realTemplates(t, nil, "layouts/*.html", "pages/wifi.html")
	comps := []hwComponentWiFiJSON{{Name: "radio0", WiFiRadio: &wifiRadioHWJSON{Band: "2.4GHz"}}}
	data := wifiData{Radios: buildWiFiRadios(comps, nil)}

	var buf strings.Builder
	if err := tmpl.ExecuteTemplate(&buf, "content", data); err != nil {
		t.Fatalf("render: %v", err)
	}
	out := buf.String()
	for _, want := range []string{`id="wifi-survey-radio0"`, `hx-post="/wifi/radio0/survey"`, "Scan channels"} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in\n%s", want, out)
		}
	}
	if strings.Contains(out, "survey-chart") {
		t.Errorf("chart rendered before any scan:\n%s", out)
	}
}
