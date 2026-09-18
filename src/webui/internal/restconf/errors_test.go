package restconf

import (
	"io"
	"net/http"
	"strings"
	"testing"
)

func TestParseErrorUnwrapsSysrepoFraming(t *testing.T) {
	body := `{"ietf-restconf:errors":{"error":[{"error-type":"application","error-tag":"operation-failed",` +
		`"error-message":"Internal server error due to sysrepo exception: Couldn't send RPC: ` +
		`SR_ERR_OPERATION_FAILED Support data collection failed: /mnt/cfg has 9 MB free, collection needs about 37 MB ` +
		`(SR_ERR_OPERATION_FAILED) NETCONF: application: operation-failed: ` +
		`Support data collection failed: /mnt/cfg has 9 MB free, collection needs about 37 MB"}]}}`
	resp := &http.Response{StatusCode: http.StatusInternalServerError, Body: io.NopCloser(strings.NewReader(body))}

	err := parseError(resp)

	re, ok := err.(*Error)
	if !ok {
		t.Fatalf("not a *Error: %T", err)
	}
	want := "Support data collection failed: /mnt/cfg has 9 MB free, collection needs about 37 MB"
	if re.Message != want {
		t.Errorf("message %q, want %q", re.Message, want)
	}
}

func TestParseErrorKeepsPlainMessage(t *testing.T) {
	body := `{"ietf-restconf:errors":{"error":[{"error-type":"application","error-tag":"access-denied",` +
		`"error-message":"Access denied."}]}}`
	resp := &http.Response{StatusCode: http.StatusForbidden, Body: io.NopCloser(strings.NewReader(body))}

	re := parseError(resp).(*Error)
	if re.Message != "Access denied." {
		t.Errorf("message %q", re.Message)
	}
}
