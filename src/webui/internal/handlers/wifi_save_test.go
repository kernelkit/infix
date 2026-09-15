// SPDX-License-Identifier: MIT

package handlers

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"infix/webui/internal/testutil"
)

func postSaveWifi(t *testing.T, form url.Values) (*httptest.ResponseRecorder, *testutil.MockFetcher) {
	t.Helper()
	rc := testutil.NewMockFetcher()
	h := &ConfigureInterfacesHandler{RC: rc}
	req := httptest.NewRequest(http.MethodPost, "/configure/interfaces/wlan0/wifi", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.SetPathValue("name", "wlan0")
	w := httptest.NewRecorder()
	h.SaveWifi(w, req)
	return w, rc
}

func TestSaveWifiOnePatchForBothHalves(t *testing.T) {
	w, rc := postSaveWifi(t, url.Values{
		"mode": {"access-point"}, "radio": {"radio0"}, "ssid": {"lab"},
		"sec-mode": {"wpa2-wpa3-personal"}, "secret": {"psk"},
		"country-code": {"SE"}, "band": {"5GHz"}, "channel": {"36"},
	})
	if w.Code != http.StatusOK {
		t.Fatalf("status %d: %s", w.Code, w.Body.String())
	}
	if len(rc.Patches) != 1 {
		t.Fatalf("patches %d", len(rc.Patches))
	}
	p := rc.Patches[0]
	if p.Target != candidatePath || len(p.Edits) != 2 {
		t.Fatalf("patch %+v", p)
	}
	if p.Edits[0].Op != "merge" || p.Edits[0].Target != "/ietf-hardware:hardware" {
		t.Errorf("radio edit %+v", p.Edits[0])
	}
	if p.Edits[1].Op != "replace" || p.Edits[1].Target != "/ietf-interfaces:interfaces/interface=wlan0/infix-interfaces:wifi" {
		t.Errorf("wifi edit %+v", p.Edits[1])
	}
	wifi := p.Edits[1].Value.(map[string]any)["infix-interfaces:wifi"].(map[string]any)
	ap, _ := wifi["access-point"].(map[string]any)
	if wifi["radio"] != "radio0" || ap["ssid"] != "lab" {
		t.Errorf("wifi value %v", wifi)
	}
}

func TestSaveWifiWithoutRadioFieldsPatchesWifiOnly(t *testing.T) {
	w, rc := postSaveWifi(t, url.Values{
		"mode": {"station"}, "radio": {"radio0"}, "ssid": {"lab"},
	})
	if w.Code != http.StatusOK {
		t.Fatalf("status %d: %s", w.Code, w.Body.String())
	}
	if len(rc.Patches) != 1 || len(rc.Patches[0].Edits) != 1 || rc.Patches[0].Edits[0].Op != "replace" {
		t.Fatalf("patches %+v", rc.Patches)
	}
}

func TestSaveWifiBadRadioWritesNothing(t *testing.T) {
	w, rc := postSaveWifi(t, url.Values{
		"mode": {"access-point"}, "radio": {"radio0"}, "ssid": {"lab"},
		"country-code": {"SE"}, "channel": {"999"},
	})
	if w.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status %d", w.Code)
	}
	if len(rc.Patches) != 0 {
		t.Fatalf("candidate written despite form error: %+v", rc.Patches)
	}
}
