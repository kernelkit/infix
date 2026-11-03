// SPDX-License-Identifier: MIT

package handlers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"infix/webui/internal/restconf"
	"infix/webui/internal/security"
	"infix/webui/internal/testutil"
)

const alarmsFixture = `{"ietf-alarms:alarms":{
  "alarm-list":{"number-of-alarms":2,"alarm":[
    {"resource":"backup","alarm-type-id":"infix-alarms:external","alarm-type-qualifier":"nightly",
     "time-created":"2026-11-03T06:12:40+00:00","is-cleared":false,"last-raised":"2026-11-03T06:12:40+00:00",
     "last-changed":"2026-11-03T06:12:40+00:00","perceived-severity":"critical","alarm-text":"Nightly backup failed"},
    {"resource":"/ietf-interfaces:interfaces/interface[name='e2']","alarm-type-id":"infix-alarms:link-down","alarm-type-qualifier":"",
     "time-created":"2026-11-03T05:00:00+00:00","is-cleared":true,"last-raised":"2026-11-03T05:00:00+00:00",
     "last-changed":"2026-11-03T07:00:00+00:00","perceived-severity":"major","alarm-text":"No link on e2",
     "operator-state-change":[{"time":"2026-11-03T06:30:00+00:00","operator":"admin","state":"ack","text":"Cable ordered"}]}]},
  "shelved-alarms":{"number-of-shelved-alarms":1,"shelved-alarm":[
    {"resource":"e7","alarm-type-id":"infix-alarms:link-down","alarm-type-qualifier":"","shelf-name":"spare-ports",
     "is-cleared":false,"last-raised":"2026-11-03T05:00:00+00:00","last-changed":"2026-11-03T05:00:00+00:00",
     "perceived-severity":"major","alarm-text":"No link on e7"}]}}}`

func alarmsRequest(t *testing.T, method, target string) *http.Request {
	t.Helper()
	req := httptest.NewRequest(method, target, nil)
	ctx := restconf.ContextWithCredentials(req.Context(), restconf.Credentials{Username: "admin", Password: "admin"})
	ctx = security.WithToken(ctx, "test-csrf-token")
	return req.WithContext(ctx)
}

func TestAlarmsPageRenders(t *testing.T) {
	mock := testutil.NewMockFetcher()
	var fixture map[string]any
	if err := json.Unmarshal([]byte(alarmsFixture), &fixture); err != nil {
		t.Fatal(err)
	}
	mock.SetResponse("/data/ietf-alarms:alarms", fixture)

	h := &AlarmsHandler{Template: realTemplates(t, nil, "layouts/*.html", "pages/alarms.html"), RC: mock}
	w := httptest.NewRecorder()
	h.Overview(w, alarmsRequest(t, http.MethodGet, "/alarms"))

	body := w.Body.String()
	for _, want := range []string{
		"badge-critical", "Nightly backup failed", "external (nightly)",
		"1 active", "Cable ordered", "spare-ports", "2026-11-03 06:12",
		`href="/interfaces/e2"`, `title="/ietf-interfaces:interfaces/interface[name=&#39;e2&#39;]"`,
		`hx-post="/alarms/clear"`, `hx-post="/alarms/state"`, `hx-post="/alarms/close-all"`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("page lacks %q", want)
		}
	}
	// The button values are JSON once the browser decodes the attribute.
	if !strings.Contains(body, `hx-vals="{&#34;qualifier&#34;:&#34;nightly&#34;,&#34;resource&#34;:&#34;backup&#34;,&#34;state&#34;:&#34;ack&#34;,&#34;type&#34;:&#34;infix-alarms:external&#34;}"`) {
		t.Error("ack button values not a quoted JSON object")
	}
	// A shelved alarm takes no operator state, its row has no buttons.
	if strings.Contains(body, `&#34;resource&#34;:&#34;e7&#34;`) {
		t.Error("shelved alarm has action buttons")
	}
	// Active alarms come first, the cleared one after.
	if strings.Index(body, "backup") > strings.Index(body, "No link on e2") {
		t.Error("cleared alarm listed before the active one")
	}
}

func TestAlarmsPageEmpty(t *testing.T) {
	mock := testutil.NewMockFetcher()
	mock.SetResponse("/data/ietf-alarms:alarms", map[string]any{"ietf-alarms:alarms": map[string]any{}})
	h := &AlarmsHandler{Template: realTemplates(t, nil, "layouts/*.html", "pages/alarms.html"), RC: mock}
	w := httptest.NewRecorder()
	h.Overview(w, alarmsRequest(t, http.MethodGet, "/alarms"))
	if !strings.Contains(w.Body.String(), "No alarms.") {
		t.Error("empty list not reported")
	}
}

// Close all is one action on the alarm list, the page reports its count.
func TestAlarmsCloseAll(t *testing.T) {
	mock := testutil.NewMockFetcher()
	var fixture map[string]any
	if err := json.Unmarshal([]byte(alarmsFixture), &fixture); err != nil {
		t.Fatal(err)
	}
	mock.SetResponse("/data/ietf-alarms:alarms", fixture)
	action := "/data/ietf-alarms:alarms/alarm-list/infix-alarms:set-operator-state"
	mock.SetResponse(action, map[string]any{"infix-alarms:output": map[string]any{"changed-alarms": 3}})

	h := &AlarmsHandler{Template: realTemplates(t, nil, "layouts/*.html", "pages/alarms.html"), RC: mock}
	w := httptest.NewRecorder()
	h.CloseAll(w, alarmsRequest(t, http.MethodPost, "/alarms/close-all"))

	if len(mock.Posts) != 1 || mock.Posts[0] != action {
		t.Fatalf("posts = %v", mock.Posts)
	}
	if !strings.Contains(w.Body.String(), "Closed 3 alarm(s)") {
		t.Error("notice missing")
	}
}

func TestLinkify(t *testing.T) {
	got := string(linkify("Release v26.10.0 is available, see https://example.com/tag/v26.10.0. <b>"))
	want := `Release v26.10.0 is available, see <a href="https://example.com/tag/v26.10.0" target="_blank" rel="noopener noreferrer">https://example.com/tag/v26.10.0</a>. &lt;b&gt;`
	if got != want {
		t.Fatalf("got %q\nwant %q", got, want)
	}
	if got := shortStamp("2026-11-03T22:09:59Z"); got != "2026-11-03 22:09" {
		t.Fatalf("shortStamp = %q", got)
	}
}

func TestResourceNameAndLink(t *testing.T) {
	iid := "/ietf-interfaces:interfaces/interface[name='eth0']"
	if resourceName(iid) != "eth0" || resourceLink(iid) != "/interfaces/eth0" {
		t.Fatalf("%q %q", resourceName(iid), resourceLink(iid))
	}
	if resourceName("backup") != "backup" || resourceLink("backup") != "" {
		t.Fatal("plain resource changed")
	}
}

func TestAlarmPathEscapesKeys(t *testing.T) {
	got := alarmPath("backup", "infix-alarms:external", "")
	want := "/data/ietf-alarms:alarms/alarm-list/alarm=backup,infix-alarms%3Aexternal,"
	if got != want {
		t.Fatalf("got %q want %q", got, want)
	}
	if got := alarmPath("a b", "infix-alarms:external", "q/1,2"); !strings.Contains(got, "a%20b") || !strings.Contains(got, "q%2F1%2C2") {
		t.Fatalf("keys not escaped: %q", got)
	}
}

// The Health card shows the alarms that are not cleared or closed,
// worst first, and caps the list.
func TestAlarmHealthItems(t *testing.T) {
	alarms := []Alarm{
		{Resource: "a", Severity: "warning", Text: "w"},
		{Resource: "b", Severity: "critical", Text: "c"},
		{Resource: "c", Severity: "major", Text: "m", Cleared: true},
		{Resource: "d", Severity: "major", Text: "closed", State: "closed"},
	}
	items := alarmHealthItems(alarms)
	if len(items) != 2 || items[0].Text != "b: c" || items[0].Class != "status-down" || items[1].Class != "status-warn" {
		t.Fatalf("items = %+v", items)
	}
	var many []Alarm
	for i := 0; i < 7; i++ {
		many = append(many, Alarm{Resource: "x", Severity: "minor"})
	}
	if items := alarmHealthItems(many); len(items) != 6 || items[5].Link != "/alarms" {
		t.Fatalf("capped items = %+v", items)
	}
}
