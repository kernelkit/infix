// SPDX-License-Identifier: MIT

package handlers

import (
	"context"
	"encoding/json"
	"html/template"
	"net/http"
	"net/http/httptest"
	"net/url"
	"reflect"
	"strings"
	"testing"

	"infix/webui/internal/restconf"
	"infix/webui/internal/schema"
	"infix/webui/internal/security"
	"infix/webui/internal/testutil"
)

var minimalCfgFwTmpl = template.Must(template.New("configure-firewall.html").Parse(
	`{{define "configure-firewall.html"}}{{template "content" .}}{{end}}` +
		`{{define "content"}}sets={{len .AddressSets}}` +
		`{{range .AddressSets}};{{.Name}}:{{.EntriesTxt}}:{{if .Timeout}}{{.Timeout}}{{end}}{{end}}` +
		`{{range .Zones}};zone-{{.Name}}:{{.AddrSetsTxt}}{{end}}{{end}}`,
))

func TestConfigureFirewallOverview_AddressSets(t *testing.T) {
	mock := testutil.NewMockFetcher()
	mock.SetResponse(candidatePath+"/infix-firewall:firewall", map[string]any{
		"infix-firewall:firewall": map[string]any{
			"default": "trusted",
			"zone": []map[string]any{{
				"name":        "trusted",
				"action":      "accept",
				"address-set": []string{"allowed"},
			}},
			"address-set": []map[string]any{{
				"name":  "allowed",
				"entry": []string{"192.168.1.40", "10.0.0.0/24"},
			}, {
				"name":    "banned",
				"timeout": 3600,
			}},
		},
	})

	h := &ConfigureFirewallHandler{
		Template: minimalCfgFwTmpl,
		RC:       mock,
		Schema:   schema.NewCache(mock, t.TempDir()),
	}

	req := httptest.NewRequest(http.MethodGet, "/configure/firewall", nil)
	ctx := restconf.ContextWithCredentials(req.Context(), restconf.Credentials{
		Username: "admin",
		Password: "admin",
	})
	ctx = security.WithToken(ctx, "test-csrf-token")
	req = req.WithContext(ctx)

	w := httptest.NewRecorder()
	h.Overview(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("want 200 got %d; body: %s", w.Code, w.Body.String())
	}

	body := w.Body.String()
	for _, want := range []string{
		"sets=2",
		";allowed:192.168.1.40\n10.0.0.0/24:",
		";banned::3600",
		";zone-trusted:allowed",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("body missing %q; body: %s", want, body)
		}
	}
}

type recordingFetcher struct {
	*testutil.MockFetcher
	putCalls    int
	lastPath    string
	lastBody    any
	deletePaths []string
}

func (r *recordingFetcher) Put(_ context.Context, path string, body any) error {
	r.putCalls++
	r.lastPath = path
	r.lastBody = body
	return nil
}

func (r *recordingFetcher) Delete(_ context.Context, path string) error {
	r.deletePaths = append(r.deletePaths, path)
	return nil
}

// zoneGetResponse mimics the server's response shape for a keyed zone GET:
// the zone is nested under its full parent path, not returned bare.
func zoneGetResponse(zone map[string]any) map[string]any {
	return map[string]any{
		"infix-firewall:firewall": map[string]any{
			"zone": []map[string]any{zone},
		},
	}
}

func TestConfigureFirewallSaveZoneAllowsInterfacesWithAddressSets(t *testing.T) {
	mock := &recordingFetcher{MockFetcher: testutil.NewMockFetcher()}
	mock.SetResponse(candidatePath+"/infix-firewall:firewall/zone=public", zoneGetResponse(map[string]any{
		"name":      "public",
		"action":    "drop",
		"interface": []string{"eth0"},
		"network":   []string{"10.0.0.0/24"},
	}))

	h := &ConfigureFirewallHandler{
		Template: minimalCfgFwTmpl,
		RC:       mock,
		Schema:   schema.NewCache(mock, t.TempDir()),
	}

	form := url.Values{
		"action":       {"drop"},
		"description":  {"Public zone"},
		"interfaces":   {"eth0"},
		"address-sets": {"allowed"},
	}
	req := httptest.NewRequest(http.MethodPost, "/configure/firewall/zones/public", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.SetPathValue("name", "public")
	ctx := restconf.ContextWithCredentials(req.Context(), restconf.Credentials{
		Username: "admin",
		Password: "admin",
	})
	ctx = security.WithToken(ctx, "test-csrf-token")
	req = req.WithContext(ctx)

	w := httptest.NewRecorder()
	h.SaveZone(w, req)

	if mock.putCalls != 1 {
		t.Fatalf("want 1 PUT call got %d", mock.putCalls)
	}
	if w.Code != http.StatusNoContent {
		t.Fatalf("want 204 got %d; body: %s", w.Code, w.Body.String())
	}
	if got, want := mock.lastPath, candidatePath+"/infix-firewall:firewall/zone=public"; got != want {
		t.Fatalf("want PUT path %q got %q", want, got)
	}
	body, ok := mock.lastBody.(map[string]any)
	if !ok {
		t.Fatalf("unexpected PUT body type %T", mock.lastBody)
	}
	zones, ok := body["infix-firewall:zone"].([]map[string]any)
	if !ok || len(zones) != 1 {
		t.Fatalf("unexpected PUT zone payload %#v", body["infix-firewall:zone"])
	}
	zone := zones[0]
	if got, want := zone["interface"], []string{"eth0"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("want interfaces %#v got %#v", want, got)
	}
	if got, want := zone["address-set"], []string{"allowed"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("want address-sets %#v got %#v", want, got)
	}
	if got, want := zone["network"], []string{"10.0.0.0/24"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("want networks preserved %#v got %#v", want, got)
	}
	var trig map[string]string
	if err := json.Unmarshal([]byte(w.Header().Get("HX-Trigger")), &trig); err != nil {
		t.Fatalf("unmarshal HX-Trigger: %v", err)
	}
	if got := trig["cfgSaved"]; !strings.Contains(got, "Zone saved") {
		t.Fatalf("unexpected success message %q", got)
	}
}

func TestConfigureFirewallSaveZoneClearsAllServices(t *testing.T) {
	mock := &recordingFetcher{MockFetcher: testutil.NewMockFetcher()}
	mock.SetResponse(candidatePath+"/infix-firewall:firewall/zone=public", zoneGetResponse(map[string]any{
		"name":      "public",
		"action":    "drop",
		"interface": []string{"eth0"},
		"service":   []string{"ssh", "http"},
	}))

	h := &ConfigureFirewallHandler{
		Template: minimalCfgFwTmpl,
		RC:       mock,
		Schema:   schema.NewCache(mock, t.TempDir()),
	}

	form := url.Values{
		"action":      {"drop"},
		"description": {"Public zone"},
		"interfaces":  {"eth0"},
	}
	req := httptest.NewRequest(http.MethodPost, "/configure/firewall/zones/public", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.SetPathValue("name", "public")
	ctx := restconf.ContextWithCredentials(req.Context(), restconf.Credentials{
		Username: "admin",
		Password: "admin",
	})
	ctx = security.WithToken(ctx, "test-csrf-token")
	req = req.WithContext(ctx)

	w := httptest.NewRecorder()
	h.SaveZone(w, req)

	if mock.putCalls != 1 {
		t.Fatalf("want 1 PUT call got %d", mock.putCalls)
	}
	body, ok := mock.lastBody.(map[string]any)
	if !ok {
		t.Fatalf("unexpected PUT body type %T", mock.lastBody)
	}
	zones, ok := body["infix-firewall:zone"].([]map[string]any)
	if !ok || len(zones) != 1 {
		t.Fatalf("unexpected PUT zone payload %#v", body["infix-firewall:zone"])
	}
	if _, ok := zones[0]["service"]; ok {
		t.Fatalf("expected cleared services to be omitted from payload, got %#v", zones[0]["service"])
	}
}

func TestConfigureFirewallResetZoneServicesOnlyDeletesServices(t *testing.T) {
	mock := &recordingFetcher{MockFetcher: testutil.NewMockFetcher()}
	mock.SetResponse(candidatePath+"/infix-firewall:firewall/zone=public", zoneGetResponse(map[string]any{
		"name":        "public",
		"action":      "drop",
		"interface":   []string{"eth0"},
		"address-set": []string{"allowed"},
		"service":     []string{"ssh", "dhcpv6-client"},
	}))

	h := &ConfigureFirewallHandler{
		Template: minimalCfgFwTmpl,
		RC:       mock,
		Schema:   schema.NewCache(mock, t.TempDir()),
	}

	req := httptest.NewRequest(http.MethodDelete, "/configure/firewall/zones/public/services", nil)
	req.SetPathValue("name", "public")
	ctx := restconf.ContextWithCredentials(req.Context(), restconf.Credentials{
		Username: "admin",
		Password: "admin",
	})
	ctx = security.WithToken(ctx, "test-csrf-token")
	req = req.WithContext(ctx)

	w := httptest.NewRecorder()
	h.ResetZoneServices(w, req)

	if w.Code != http.StatusNoContent {
		t.Fatalf("want 204 got %d; body: %s", w.Code, w.Body.String())
	}
	if got, want := w.Header().Get("HX-Location"), `{"path":"/configure/firewall","target":"#content"}`; got != want {
		t.Fatalf("want HX-Location %q got %q", want, got)
	}
	if mock.putCalls != 0 {
		t.Fatalf("reset must not rewrite the zone, got %d PUT call(s) with body %#v",
			mock.putCalls, mock.lastBody)
	}
	want := []string{
		candidatePath + "/infix-firewall:firewall/zone=public/service=ssh",
		candidatePath + "/infix-firewall:firewall/zone=public/service=dhcpv6-client",
	}
	if !reflect.DeepEqual(mock.deletePaths, want) {
		t.Fatalf("want DELETE paths %#v got %#v", want, mock.deletePaths)
	}
}
