package handlers

import (
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"infix/webui/internal/restconf"
)

// fakeSupportRPC serves infix-system:support-collect, recording the input
// it received and answering with reply.
func fakeSupportRPC(t *testing.T, reply string, status int) (*httptest.Server, *map[string]any) {
	t.Helper()
	var got map[string]any
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/operations/infix-system:support-collect" || r.Method != http.MethodPost {
			t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
			http.Error(w, "unexpected", http.StatusNotFound)
			return
		}
		if user, pass, ok := r.BasicAuth(); !ok || user != "admin" || pass != "secret" {
			t.Errorf("credentials not forwarded: %q %q %v", user, pass, ok)
		}
		body, _ := io.ReadAll(r.Body)
		got = nil
		if len(body) > 0 {
			if err := json.Unmarshal(body, &got); err != nil {
				t.Errorf("input is not JSON: %v", err)
			}
		}
		w.Header().Set("Content-Type", "application/yang-data+json")
		w.WriteHeader(status)
		io.WriteString(w, reply) //nolint:errcheck
	}))
	t.Cleanup(srv.Close)
	return srv, &got
}

func supportRequest(srv *httptest.Server, form string) *httptest.ResponseRecorder {
	h := &SystemHandler{RC: restconf.NewClient(srv.URL, true)}
	req := httptest.NewRequest(http.MethodPost, "/maintenance/support-bundle", strings.NewReader(form))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req = req.WithContext(restconf.ContextWithCredentials(req.Context(),
		restconf.Credentials{Username: "admin", Password: "secret"}))
	rec := httptest.NewRecorder()
	h.SupportBundle(rec, req)
	return rec
}

func TestSupportBundleDownloadsArchive(t *testing.T) {
	archive := []byte("\x1f\x8b\x08not really gzip")
	reply := `{"infix-system:output":{"size":` + strconv.Itoa(len(archive)) + `,"data":"` +
		base64.StdEncoding.EncodeToString(archive) + `"}}`
	srv, got := fakeSupportRPC(t, reply, http.StatusOK)

	rec := supportRequest(srv, "")

	if rec.Code != http.StatusOK {
		t.Fatalf("status %d: %s", rec.Code, rec.Body.String())
	}
	if *got != nil {
		t.Errorf("no password given, but input sent: %v", *got)
	}
	if rec.Body.String() != string(archive) {
		t.Errorf("body is not the decoded archive: %q", rec.Body.String())
	}
	if ct := rec.Header().Get("Content-Type"); ct != "application/gzip" {
		t.Errorf("Content-Type %q", ct)
	}
	cd := rec.Header().Get("Content-Disposition")
	if !strings.HasPrefix(cd, `attachment; filename="support-`) || !strings.HasSuffix(cd, `.tar.gz"`) {
		t.Errorf("Content-Disposition %q", cd)
	}
}

func TestSupportBundlePasswordEncrypts(t *testing.T) {
	reply := `{"infix-system:output":{"size":3,"data":"` + base64.StdEncoding.EncodeToString([]byte("gpg")) + `"}}`
	srv, got := fakeSupportRPC(t, reply, http.StatusOK)

	rec := supportRequest(srv, "password=hunter2")

	if rec.Code != http.StatusOK {
		t.Fatalf("status %d: %s", rec.Code, rec.Body.String())
	}
	input, _ := (*got)["infix-system:input"].(map[string]any)
	if input["password"] != "hunter2" {
		t.Errorf("password not passed to the RPC: %v", *got)
	}
	if ct := rec.Header().Get("Content-Type"); ct != "application/pgp-encrypted" {
		t.Errorf("Content-Type %q", ct)
	}
	if !strings.HasSuffix(rec.Header().Get("Content-Disposition"), `.tar.gz.gpg"`) {
		t.Errorf("Content-Disposition %q", rec.Header().Get("Content-Disposition"))
	}
}

func TestSupportBundleTooLarge(t *testing.T) {
	reply := `{"infix-system:output":{"size":20000000,"filename":"/var/lib/support/support-host-x.tar.gz"}}`
	srv, _ := fakeSupportRPC(t, reply, http.StatusOK)

	rec := supportRequest(srv, "")

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "19 MB") ||
		!strings.Contains(rec.Body.String(), "/var/lib/support/support-host-x.tar.gz") {
		t.Errorf("message does not say where the archive is: %q", rec.Body.String())
	}
}

func TestSupportBundleReportsRPCError(t *testing.T) {
	reply := `{"ietf-restconf:errors":{"error":[{"error-type":"application","error-tag":"operation-failed",` +
		`"error-message":"gpg is not available on this device"}]}}`
	srv, _ := fakeSupportRPC(t, reply, http.StatusInternalServerError)

	rec := supportRequest(srv, "password=x")

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "gpg is not available on this device") {
		t.Errorf("server's reason not surfaced: %q", rec.Body.String())
	}
}

func TestSupportBundleDeniedIsForbidden(t *testing.T) {
	reply := `{"ietf-restconf:errors":{"error":[{"error-type":"application","error-tag":"access-denied",` +
		`"error-message":"Access denied."}]}}`
	srv, _ := fakeSupportRPC(t, reply, http.StatusForbidden)

	rec := supportRequest(srv, "")

	if rec.Code != http.StatusForbidden {
		t.Fatalf("status %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "Access denied") {
		t.Errorf("reason not surfaced: %q", rec.Body.String())
	}
}
