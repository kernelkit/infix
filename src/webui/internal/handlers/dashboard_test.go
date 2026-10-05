// SPDX-License-Identifier: MIT

package handlers

import (
	"encoding/json"
	"html/template"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"infix/webui/internal/restconf"
	"infix/webui/internal/security"
)

var minimalDashTmpl = template.Must(template.New("dashboard.html").Parse(
	`{{define "dashboard.html"}}hostname={{.Hostname}} error={{.Error}}{{end}}` +
		`{{define "content"}}{{.Hostname}}{{end}}` +
		`{{define "update-card"}}check={{.Update.Check}} error={{.UpdateError}}{{end}}`,
))

func TestDashboardIndex_ReturnsOK(t *testing.T) {
	rc := restconf.NewClient("http://127.0.0.1:19999/restconf", false)
	h := &DashboardHandler{Template: minimalDashTmpl, RC: rc}

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	ctx := restconf.ContextWithCredentials(req.Context(), restconf.Credentials{
		Username: "testuser",
		Password: "testpass",
	})
	ctx = security.WithToken(ctx, "test-csrf-token")
	req = req.WithContext(ctx)

	w := httptest.NewRecorder()
	h.Index(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("want 200 got %d", w.Code)
	}
}

func TestDashboardIndex_ShowsErrorOnRESTCONFFailure(t *testing.T) {
	rc := restconf.NewClient("http://127.0.0.1:19999/restconf", false)
	h := &DashboardHandler{Template: minimalDashTmpl, RC: rc}

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	ctx := restconf.ContextWithCredentials(req.Context(), restconf.Credentials{
		Username: "admin",
		Password: "admin",
	})
	ctx = security.WithToken(ctx, "tok")
	req = req.WithContext(ctx)

	w := httptest.NewRecorder()
	h.Index(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("want 200 got %d; body: %s", w.Code, w.Body.String())
	}

	body := w.Body.String()
	if body == "" {
		t.Error("expected non-empty response body")
	}
}

func TestDashboardIndex_HTMXPartial(t *testing.T) {
	rc := restconf.NewClient("http://127.0.0.1:19999/restconf", false)
	h := &DashboardHandler{Template: minimalDashTmpl, RC: rc}

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("HX-Request", "true")
	ctx := restconf.ContextWithCredentials(req.Context(), restconf.Credentials{
		Username: "admin",
		Password: "admin",
	})
	ctx = security.WithToken(ctx, "tok")
	req = req.WithContext(ctx)

	w := httptest.NewRecorder()
	h.Index(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("want 200 got %d; body: %s", w.Code, w.Body.String())
	}
}

func TestUpdateEntry(t *testing.T) {
	off := false
	cfg := swConfig{
		UpdateURL:   "https://github.com/kernelkit/infix/releases.atom",
		CheckUpdate: swScheduled{Schedule: "nightly"},
		Unattended:  swUnattended{swScheduled: swScheduled{Schedule: "nightly", Enabled: &off}},
	}
	state := swUpdateState{
		LastCheck: "2026-09-30T03:00:12Z", Latest: "v26.09.0", Available: true,
		LastInstall: "2026-09-28T03:02:41Z", Installed: "v26.08.1", RebootPending: true,
	}

	var nightly scheduleEntry
	nightly.Name = "nightly"
	nightly.Recurrence.Frequency = "ietf-schedule:daily"
	nightly.Recurrence.ByHour = []int{3}
	schedules := []scheduleEntry{nightly}

	e := newUpdateEntry(cfg, schedules, state)
	if e.Check != "nightly (daily at 03:00)" {
		t.Errorf("Check = %q", e.Check)
	}
	if e.Unattended != "nightly (paused)" || e.AutoReboot {
		t.Errorf("Unattended = %q auto-reboot %v", e.Unattended, e.AutoReboot)
	}
	if e.LastCheck != "2026-09-30 03:00" {
		t.Errorf("LastCheck = %q", e.LastCheck)
	}
	if !e.Available || !e.RebootPending {
		t.Errorf("flags lost: %+v", e)
	}

	e = newUpdateEntry(swConfig{}, nil, swUpdateState{})
	if e.Source != "" || e.Check != "not configured" || e.Unattended != "not configured" {
		t.Errorf("unconfigured entry = %+v", e)
	}
}

func TestDescribeRecurrence(t *testing.T) {
	var s scheduleEntry
	s.Recurrence.Frequency = "ietf-schedule:weekly"
	s.Recurrence.ByDay = append(s.Recurrence.ByDay, struct {
		Weekday string `json:"weekday"`
	}{"sunday"})
	s.Recurrence.ByHour = []int{3}
	if got := describeRecurrence(s); got != "sunday at 03:00" {
		t.Errorf("weekly = %q", got)
	}

	s.Recurrence.Interval = 2
	s.Recurrence.ByMinute = []int{15}
	if got := describeRecurrence(s); got != "every 2 weeks on sunday at 03:15" {
		t.Errorf("biweekly = %q", got)
	}

	var h scheduleEntry
	h.Recurrence.Frequency = "ietf-schedule:hourly"
	if got := describeRecurrence(h); got != "hourly" {
		t.Errorf("hourly = %q", got)
	}
}

func TestSystemConfigDecode(t *testing.T) {
	body := `{"ietf-system:system": {
	  "hostname": "example",
	  "infix-system:software": {
	    "update-url": "https://github.com/kernelkit/infix/releases.atom",
	    "check-update": {"schedule": "nightly"},
	    "unattended-update": {"schedule": "weekly", "reboot": "immediate"}
	  },
	  "infix-schedule:schedules": {"schedule": [
	    {"name": "nightly", "recurrence": {"frequency": "ietf-schedule:daily", "byhour": [3]}},
	    {"name": "weekly", "enabled": true, "recurrence": {"frequency": "ietf-schedule:weekly",
	      "byday": [{"weekday": "sunday"}]}}
	  ]}
	}}`
	var cfg systemConfigWrapper
	if err := json.Unmarshal([]byte(body), &cfg); err != nil {
		t.Fatal(err)
	}
	e := newUpdateEntry(cfg.System.Software, cfg.System.Schedules.Schedule, swUpdateState{})
	if e.Source != "https://github.com/kernelkit/infix/releases.atom" {
		t.Errorf("Source = %q", e.Source)
	}
	if e.Check != "nightly (daily at 03:00)" {
		t.Errorf("Check = %q", e.Check)
	}
	if e.Unattended != "weekly (sunday)" || !e.AutoReboot {
		t.Errorf("Unattended = %q auto-reboot %v", e.Unattended, e.AutoReboot)
	}
}

func TestShortURL(t *testing.T) {
	cases := map[string]string{
		"https://github.com/kernelkit/infix/releases.atom": "releases.atom",
		"http://releases.example.com/releases.atom":        "releases.atom",
		"http://10.0.0.1/": "10.0.0.1",
		"not a url":        "not a url",
	}
	for in, want := range cases {
		if got := shortURL(in); got != want {
			t.Errorf("shortURL(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestCheckUpdate_RendersCardOnFailure(t *testing.T) {
	rc := restconf.NewClient("http://127.0.0.1:19999/restconf", false)
	h := &DashboardHandler{Template: minimalDashTmpl, RC: rc}

	req := httptest.NewRequest(http.MethodPost, "/dashboard/check-update", nil)
	ctx := restconf.ContextWithCredentials(req.Context(), restconf.Credentials{
		Username: "testuser",
		Password: "testpass",
	})
	req = req.WithContext(ctx)

	w := httptest.NewRecorder()
	h.CheckUpdate(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d", w.Code)
	}
	body := w.Body.String()
	if !strings.Contains(body, "check=not configured") || !strings.Contains(body, "error=Update check failed") {
		t.Errorf("body = %q", body)
	}
}

func TestPorts(t *testing.T) {
	var iw interfacesWrapper
	for _, p := range []struct {
		name, typ, oper string
		speed           int64
	}{
		{"eth10", "iana-if-type:ethernetCsmacd", "up", 2500000000},
		{"eth2", "iana-if-type:ethernetCsmacd", "down", 0},
		{"br0", "iana-if-type:bridge", "up", 0},
		{"eth1", "iana-if-type:ethernetCsmacd", "up", 1000000000},
		{"lan1", "infix-if-type:ethernet", "lower-layer-down", 0},
	} {
		iw.Interfaces.Interface = append(iw.Interfaces.Interface, ifaceJSON{
			Name: p.name, Type: p.typ, OperStatus: p.oper, Speed: yangInt64(p.speed),
		})
	}
	got := ports(iw, map[string]string{"eth1": "core-sw"})
	want := []portEntry{
		{Name: "eth1", Up: true, Speed: "1G", Neighbor: "core-sw"},
		{Name: "eth2"},
		{Name: "eth10", Up: true, Speed: "2.5G"},
		{Name: "lan1"},
	}
	if len(got) != len(want) {
		t.Fatalf("ports = %+v", got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("port %d = %+v, want %+v", i, got[i], want[i])
		}
	}
	if s := portSpeed(100000000); s != "100M" {
		t.Errorf("100M = %q", s)
	}
}

func TestHealthItems(t *testing.T) {
	var sw software
	sw.Booted = "primary"
	sw.Slot = []softwareSlot{{Name: "rootfs.0", BootName: "primary", Class: "rootfs"}, {Name: "rootfs.1", BootName: "secondary", Class: "rootfs"}}
	sw.Slot[0].Bundle.Version = "v2"
	sw.Slot[1].Bundle.Version = "v1"

	items := healthItems([]serviceJSON{{Name: "hostapd", Status: "crashed"}, {Name: "sshd", Status: "running"}}, sw)
	if len(items) != 2 || items[0].Text != "hostapd crashed" || items[1].Text != "secondary partition is out of date (v1)" {
		t.Errorf("items = %+v", items)
	}

	sw.Update.RebootPending = true
	sw.Update.Installed = "v3"
	items = healthItems(nil, sw)
	if len(items) != 1 || items[0].Text != "Reboot to activate v3" || items[0].Class != "status-warn" {
		t.Errorf("reboot items = %+v", items)
	}

	sw.Update.RebootPending = false
	sw.Slot[1].Bundle.Version = "v2"
	items = healthItems(nil, sw)
	if len(items) != 1 || items[0].Class != "status-up" {
		t.Errorf("healthy items = %+v", items)
	}
	if softwareVersion(sw) != "v2" {
		t.Errorf("softwareVersion = %q", softwareVersion(sw))
	}
}
