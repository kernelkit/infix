// SPDX-License-Identifier: MIT

package handlers

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"html/template"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"infix/webui/internal/restconf"
	"infix/webui/internal/schema"
	"infix/webui/internal/security"
	"infix/webui/internal/testutil"
)

var minimalCfgAdvTmpl = template.Must(template.New("configure-advanced.html").Parse(
	`{{define "configure-advanced.html"}}{{template "content" .}}{{end}}` +
		`{{define "content"}}n={{len .Scripts}}` +
		`{{range .Scripts}};{{.Name}}:{{.Enabled}}:{{.Content}}{{end}}` +
		`|files={{len .Files}}{{range .Files}};{{.Name}}:{{.Enabled}}:{{.Content}}{{end}}{{end}}`,
))

// advRecorder extends recordingFetcher with PATCH recording.
type advRecorder struct {
	*recordingFetcher
	patchCalls int
	patchPath  string
	patchBody  any
}

func (r *advRecorder) Patch(_ context.Context, path string, body any) error {
	r.patchCalls++
	r.patchPath = path
	r.patchBody = body
	return nil
}

func newAdvRecorder() *advRecorder {
	return &advRecorder{recordingFetcher: &recordingFetcher{MockFetcher: testutil.NewMockFetcher()}}
}

func advSystemResponse(scripts, files []map[string]any) map[string]any {
	return map[string]any{
		"ietf-system:system": map[string]any{
			"hostname": "test",
			"infix-system:advanced": map[string]any{
				"rc.ds":    map[string]any{"rc.d": scripts},
				"defaults": map[string]any{"default": files},
			},
		},
	}
}

func advEntry(name, content string) map[string]any {
	return map[string]any{
		"name":    name,
		"content": base64.StdEncoding.EncodeToString([]byte(content)),
	}
}

func newAdvHandler(t *testing.T, rc restconf.Fetcher) *ConfigureAdvancedHandler {
	t.Helper()
	return &ConfigureAdvancedHandler{
		Template: minimalCfgAdvTmpl,
		RC:       rc,
		Schema:   schema.NewCache(rc, t.TempDir()),
	}
}

func newAdvRequest(t *testing.T, method, target string, form url.Values) *http.Request {
	t.Helper()
	var req *http.Request
	if form != nil {
		req = httptest.NewRequest(method, target, strings.NewReader(form.Encode()))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	} else {
		req = httptest.NewRequest(method, target, nil)
	}
	ctx := restconf.ContextWithCredentials(req.Context(), restconf.Credentials{
		Username: "admin",
		Password: "admin",
	})
	ctx = security.WithToken(ctx, "test-csrf-token")
	return req.WithContext(ctx)
}

// wireJSON re-encodes a recorded request body so tests can inspect the
// exact JSON the datastore would have received.
func wireJSON(t *testing.T, body any) string {
	t.Helper()
	raw, err := json.Marshal(body)
	if err != nil {
		t.Fatalf("marshal body: %v", err)
	}
	return string(raw)
}

func putRCD(t *testing.T, mock *recordingFetcher) cfgRCDJSON {
	t.Helper()
	var body struct {
		RCD cfgRCDJSON `json:"infix-system:rc.ds"`
	}
	raw := wireJSON(t, mock.lastBody)
	if err := json.Unmarshal([]byte(raw), &body); err != nil {
		t.Fatalf("unmarshal PUT body: %v; raw: %s", err, raw)
	}
	return body.RCD
}

// ─── rc.d scripts ─────────────────────────────────────────────────────────────

func TestConfigureAdvancedOverview_ScriptsInOrder(t *testing.T) {
	mock := testutil.NewMockFetcher()
	mock.SetResponse(candidatePath+"/ietf-system:system", advSystemResponse(
		[]map[string]any{
			advEntry("10-first", "#!/bin/sh\necho one\n"),
			{
				"name":    "20-second",
				"enabled": false,
				"content": base64.StdEncoding.EncodeToString([]byte("echo two\n")),
			},
		}, nil))

	w := httptest.NewRecorder()
	newAdvHandler(t, mock).Overview(w, newAdvRequest(t, http.MethodGet, "/configure/advanced", nil))

	if w.Code != http.StatusOK {
		t.Fatalf("want 200 got %d; body: %s", w.Code, w.Body.String())
	}
	want := "n=2;10-first:true:#!/bin/sh\necho one\n;20-second:false:echo two\n|files=0"
	if got := w.Body.String(); got != want {
		t.Fatalf("body mismatch\nwant: %q\n got: %q", want, got)
	}
}

func TestConfigureAdvancedAddScript_PutsBase64Content(t *testing.T) {
	mock := &recordingFetcher{MockFetcher: testutil.NewMockFetcher()}
	mock.SetResponse(candidatePath+"/ietf-system:system", advSystemResponse(
		[]map[string]any{advEntry("10-first", "echo one\n")}, nil))

	const script = "#!/bin/sh\r\nlogger -t rc.d hello\r\n"
	form := url.Values{
		"name":        {"20-hello"},
		"description": {"Say hello"},
		"enabled":     {"true"},
		"content":     {script},
	}
	w := httptest.NewRecorder()
	newAdvHandler(t, mock).AddScript(w, newAdvRequest(t, http.MethodPost, "/configure/advanced/rc.d/scripts", form))

	if w.Code != http.StatusNoContent {
		t.Fatalf("want 204 got %d; body: %s", w.Code, w.Body.String())
	}
	if mock.putCalls != 1 || mock.lastPath != rcdCandPath {
		t.Fatalf("want 1 PUT to %q, got %d call(s) to %q", rcdCandPath, mock.putCalls, mock.lastPath)
	}
	raw := wireJSON(t, mock.lastBody)
	wantB64 := base64.StdEncoding.EncodeToString([]byte("#!/bin/sh\nlogger -t rc.d hello\n"))
	if !strings.Contains(raw, `"content":"`+wantB64+`"`) {
		t.Fatalf("PUT body missing base64 content %q; raw: %s", wantB64, raw)
	}
	if strings.Contains(raw, `"rc.ds":{"enabled"`) || strings.Contains(raw, `"infix-system:rc.ds":{"enabled"`) {
		t.Fatalf("rc.d container must not carry an enabled leaf; raw: %s", raw)
	}
	rcd := putRCD(t, mock)
	if len(rcd.Scripts) != 2 || rcd.Scripts[0].Name != "10-first" || rcd.Scripts[1].Name != "20-hello" {
		t.Fatalf("new script must be appended after existing ones, got %+v", rcd.Scripts)
	}
	if rcd.Scripts[1].Description != "Say hello" || rcd.Scripts[1].Enabled == nil || !*rcd.Scripts[1].Enabled {
		t.Fatalf("unexpected new script fields %+v", rcd.Scripts[1])
	}
	if got := w.Header().Get("HX-Location"); !strings.Contains(got, `"/configure/advanced"`) {
		t.Fatalf("want redirect back to page, got HX-Location %q", got)
	}
}

func TestConfigureAdvancedAddScript_RejectsDuplicate(t *testing.T) {
	mock := &recordingFetcher{MockFetcher: testutil.NewMockFetcher()}
	mock.SetResponse(candidatePath+"/ietf-system:system", advSystemResponse(
		[]map[string]any{advEntry("10-first", "echo one\n")}, nil))

	form := url.Values{"name": {"10-first"}, "content": {"echo again"}}
	w := httptest.NewRecorder()
	newAdvHandler(t, mock).AddScript(w, newAdvRequest(t, http.MethodPost, "/configure/advanced/rc.d/scripts", form))

	if w.Code != http.StatusUnprocessableEntity {
		t.Fatalf("want 422 got %d", w.Code)
	}
	if mock.putCalls != 0 {
		t.Fatalf("duplicate must not be written, got %d PUT call(s)", mock.putCalls)
	}
}

func TestConfigureAdvancedReorder_RewritesListOrder(t *testing.T) {
	mock := &recordingFetcher{MockFetcher: testutil.NewMockFetcher()}
	mock.SetResponse(candidatePath+"/ietf-system:system", advSystemResponse(
		[]map[string]any{advEntry("a", "echo a"), advEntry("b", "echo b"), advEntry("c", "echo c")}, nil))

	form := url.Values{"order": {"c", "a", "b"}}
	w := httptest.NewRecorder()
	newAdvHandler(t, mock).ReorderScripts(w, newAdvRequest(t, http.MethodPost, "/configure/advanced/rc.d/order", form))

	if w.Code != http.StatusOK {
		t.Fatalf("want 200 got %d; body: %s", w.Code, w.Body.String())
	}
	if mock.putCalls != 1 || mock.lastPath != rcdCandPath {
		t.Fatalf("want 1 PUT to %q, got %d call(s) to %q", rcdCandPath, mock.putCalls, mock.lastPath)
	}
	var got []string
	for _, s := range putRCD(t, mock).Scripts {
		got = append(got, s.Name)
		if len(s.Content) == 0 {
			t.Fatalf("script %q lost its content on reorder", s.Name)
		}
	}
	if strings.Join(got, ",") != "c,a,b" {
		t.Fatalf("want order c,a,b got %v", got)
	}
	var trig map[string]string
	if err := json.Unmarshal([]byte(w.Header().Get("HX-Trigger")), &trig); err != nil {
		t.Fatalf("unmarshal HX-Trigger: %v", err)
	}
	if !strings.Contains(trig["cfgSaved"], "order saved") {
		t.Fatalf("unexpected success message %q", trig["cfgSaved"])
	}
}

func TestConfigureAdvancedReorder_RejectsMismatchedSet(t *testing.T) {
	mock := &recordingFetcher{MockFetcher: testutil.NewMockFetcher()}
	mock.SetResponse(candidatePath+"/ietf-system:system", advSystemResponse(
		[]map[string]any{advEntry("a", "echo a"), advEntry("b", "echo b")}, nil))
	h := newAdvHandler(t, mock)

	for _, order := range [][]string{{"a"}, {"a", "x"}, {"a", "a"}} {
		w := httptest.NewRecorder()
		h.ReorderScripts(w, newAdvRequest(t, http.MethodPost, "/configure/advanced/rc.d/order", url.Values{"order": order}))
		if w.Code != http.StatusUnprocessableEntity {
			t.Fatalf("order %v: want 422 got %d", order, w.Code)
		}
	}
	if mock.putCalls != 0 {
		t.Fatalf("mismatched order must not be written, got %d PUT call(s)", mock.putCalls)
	}
}

func TestConfigureAdvancedSaveScript_KeepsPosition(t *testing.T) {
	mock := &recordingFetcher{MockFetcher: testutil.NewMockFetcher()}
	mock.SetResponse(candidatePath+"/ietf-system:system", advSystemResponse(
		[]map[string]any{advEntry("a", "echo a"), advEntry("b", "echo b"), advEntry("c", "echo c")}, nil))

	form := url.Values{"description": {"middle"}, "content": {"echo B"}}
	req := newAdvRequest(t, http.MethodPost, "/configure/advanced/rc.d/scripts/b", form)
	req.SetPathValue("name", "b")
	w := httptest.NewRecorder()
	newAdvHandler(t, mock).SaveScript(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("want 200 got %d; body: %s", w.Code, w.Body.String())
	}
	rcd := putRCD(t, mock)
	if len(rcd.Scripts) != 3 || rcd.Scripts[1].Name != "b" {
		t.Fatalf("edited script must keep position 2, got %+v", rcd.Scripts)
	}
	b := rcd.Scripts[1]
	if string(b.Content) != "echo B" || b.Description != "middle" || b.Enabled == nil || *b.Enabled {
		t.Fatalf("unexpected edited script %+v (content %q)", b, b.Content)
	}
}

func TestConfigureAdvancedDeleteScript(t *testing.T) {
	mock := &recordingFetcher{MockFetcher: testutil.NewMockFetcher()}

	req := newAdvRequest(t, http.MethodDelete, "/configure/advanced/rc.d/scripts/10-first", nil)
	req.SetPathValue("name", "10-first")
	w := httptest.NewRecorder()
	newAdvHandler(t, mock).DeleteScript(w, req)

	if w.Code != http.StatusNoContent {
		t.Fatalf("want 204 got %d", w.Code)
	}
	want := rcdCandPath + "/rc.d=10-first"
	if len(mock.deletePaths) != 1 || mock.deletePaths[0] != want {
		t.Fatalf("want DELETE %q got %v", want, mock.deletePaths)
	}
}

// ─── /etc/default files ───────────────────────────────────────────────────────

func TestConfigureAdvancedOverview_DefaultFiles(t *testing.T) {
	mock := testutil.NewMockFetcher()
	mock.SetResponse(candidatePath+"/ietf-system:system", advSystemResponse(nil,
		[]map[string]any{
			// Quote-free content: the minimal template renders it in a text
			// context where html/template would escape double quotes.
			advEntry("ptp4l", "PTP4L_ARGS=--ptp_minor_version=0\n"),
			{
				"name":        "chronyd",
				"description": "off",
				"enabled":     false,
				"content":     base64.StdEncoding.EncodeToString([]byte("CHRONYD_ARGS=-x\n")),
			},
		}))

	w := httptest.NewRecorder()
	newAdvHandler(t, mock).Overview(w, newAdvRequest(t, http.MethodGet, "/configure/advanced", nil))

	if w.Code != http.StatusOK {
		t.Fatalf("want 200 got %d; body: %s", w.Code, w.Body.String())
	}
	want := "n=0|files=2;ptp4l:true:PTP4L_ARGS=--ptp_minor_version=0\n;chronyd:false:CHRONYD_ARGS=-x\n"
	if got := w.Body.String(); got != want {
		t.Fatalf("body mismatch\nwant: %q\n got: %q", want, got)
	}
}

func TestConfigureAdvancedAddFile_PutsEntryWithBase64Content(t *testing.T) {
	mock := newAdvRecorder()
	mock.SetResponse(candidatePath+"/ietf-system:system", advSystemResponse(nil,
		[]map[string]any{advEntry("chronyd", "CHRONYD_ARGS=\"-x\"")}))

	form := url.Values{
		"name":        {"ptp4l"},
		"description": {"Force PTPv2.0"},
		"enabled":     {"true"},
		"content":     {"PTP4L_ARGS=\"--ptp_minor_version 0\"\r\n"},
	}
	w := httptest.NewRecorder()
	newAdvHandler(t, mock).AddFile(w, newAdvRequest(t, http.MethodPost, "/configure/advanced/default/files", form))

	if w.Code != http.StatusNoContent {
		t.Fatalf("want 204 got %d; body: %s", w.Code, w.Body.String())
	}
	wantPath := defaultCandPath + "/default=ptp4l"
	if mock.putCalls != 1 || mock.lastPath != wantPath {
		t.Fatalf("want 1 PUT to %q, got %d call(s) to %q", wantPath, mock.putCalls, mock.lastPath)
	}
	if mock.patchCalls != 0 {
		t.Fatalf("add must not PATCH, got %d call(s)", mock.patchCalls)
	}
	raw := wireJSON(t, mock.lastBody)
	wantB64 := base64.StdEncoding.EncodeToString([]byte("PTP4L_ARGS=\"--ptp_minor_version 0\"\n"))
	for _, want := range []string{
		`"infix-system:default":[{`,
		`"name":"ptp4l"`,
		`"description":"Force PTPv2.0"`,
		`"enabled":true`,
		`"content":"` + wantB64 + `"`,
	} {
		if !strings.Contains(raw, want) {
			t.Fatalf("PUT body missing %s; raw: %s", want, raw)
		}
	}
	if got := w.Header().Get("HX-Location"); !strings.Contains(got, `"/configure/advanced"`) {
		t.Fatalf("want redirect back to page, got HX-Location %q", got)
	}
}

func TestConfigureAdvancedAddFile_RejectsDuplicate(t *testing.T) {
	mock := newAdvRecorder()
	mock.SetResponse(candidatePath+"/ietf-system:system", advSystemResponse(nil,
		[]map[string]any{advEntry("ptp4l", "PTP4L_ARGS=\"\"")}))

	form := url.Values{"name": {"ptp4l"}, "content": {"PTP4L_ARGS=\"-x\""}}
	w := httptest.NewRecorder()
	newAdvHandler(t, mock).AddFile(w, newAdvRequest(t, http.MethodPost, "/configure/advanced/default/files", form))

	if w.Code != http.StatusUnprocessableEntity {
		t.Fatalf("want 422 got %d", w.Code)
	}
	if mock.putCalls != 0 || mock.patchCalls != 0 {
		t.Fatalf("duplicate must not be written, got %d PUT / %d PATCH call(s)", mock.putCalls, mock.patchCalls)
	}
}

func TestConfigureAdvancedToggleFile_PatchesEnabledLeaf(t *testing.T) {
	mock := newAdvRecorder()

	// Unchecked checkbox: htmx sends no "enabled" value at all.
	req := newAdvRequest(t, http.MethodPost, "/configure/advanced/default/files/ptp4l/enabled", url.Values{})
	req.SetPathValue("name", "ptp4l")
	w := httptest.NewRecorder()
	newAdvHandler(t, mock).ToggleFile(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("want 200 got %d; body: %s", w.Code, w.Body.String())
	}
	wantPath := defaultCandPath + "/default=ptp4l"
	if mock.patchCalls != 1 || mock.patchPath != wantPath {
		t.Fatalf("want 1 PATCH to %q, got %d call(s) to %q", wantPath, mock.patchCalls, mock.patchPath)
	}
	if mock.putCalls != 0 {
		t.Fatalf("toggle must not PUT, got %d call(s)", mock.putCalls)
	}
	raw := wireJSON(t, mock.patchBody)
	if want := `{"infix-system:default":[{"enabled":false,"name":"ptp4l"}]}`; raw != want {
		t.Fatalf("PATCH body\nwant: %s\n got: %s", want, raw)
	}
	if !strings.Contains(w.Header().Get("HX-Trigger"), "File disabled") {
		t.Fatalf("unexpected HX-Trigger %q", w.Header().Get("HX-Trigger"))
	}

	// Checked: "enabled=true".
	req = newAdvRequest(t, http.MethodPost, "/configure/advanced/default/files/ptp4l/enabled", url.Values{"enabled": {"true"}})
	req.SetPathValue("name", "ptp4l")
	w = httptest.NewRecorder()
	newAdvHandler(t, mock).ToggleFile(w, req)
	if raw := wireJSON(t, mock.patchBody); !strings.Contains(raw, `"enabled":true`) {
		t.Fatalf("want enabled:true in PATCH body, got %s", raw)
	}
}

func TestConfigureAdvancedSaveFile_PutsEntry(t *testing.T) {
	mock := newAdvRecorder()

	form := url.Values{"description": {"tuned"}, "content": {"PTP4L_ARGS=\"-l 7\""}}
	req := newAdvRequest(t, http.MethodPost, "/configure/advanced/default/files/ptp4l", form)
	req.SetPathValue("name", "ptp4l")
	w := httptest.NewRecorder()
	newAdvHandler(t, mock).SaveFile(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("want 200 got %d; body: %s", w.Code, w.Body.String())
	}
	if mock.putCalls != 1 || mock.lastPath != defaultCandPath+"/default=ptp4l" {
		t.Fatalf("want 1 PUT to default=ptp4l, got %d call(s) to %q", mock.putCalls, mock.lastPath)
	}
	raw := wireJSON(t, mock.lastBody)
	wantB64 := base64.StdEncoding.EncodeToString([]byte("PTP4L_ARGS=\"-l 7\""))
	if !strings.Contains(raw, `"content":"`+wantB64+`"`) || !strings.Contains(raw, `"enabled":false`) {
		t.Fatalf("unexpected PUT body %s", raw)
	}
}

func TestConfigureAdvancedDeleteFile(t *testing.T) {
	mock := &recordingFetcher{MockFetcher: testutil.NewMockFetcher()}

	req := newAdvRequest(t, http.MethodDelete, "/configure/advanced/default/files/ptp4l", nil)
	req.SetPathValue("name", "ptp4l")
	w := httptest.NewRecorder()
	newAdvHandler(t, mock).DeleteFile(w, req)

	if w.Code != http.StatusNoContent {
		t.Fatalf("want 204 got %d", w.Code)
	}
	want := defaultCandPath + "/default=ptp4l"
	if len(mock.deletePaths) != 1 || mock.deletePaths[0] != want {
		t.Fatalf("want DELETE %q got %v", want, mock.deletePaths)
	}
	if got := w.Header().Get("HX-Location"); !strings.Contains(got, `"/configure/advanced"`) {
		t.Fatalf("want redirect back to page, got HX-Location %q", got)
	}
}
