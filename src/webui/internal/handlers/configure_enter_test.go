// SPDX-License-Identifier: MIT

package handlers

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"infix/webui/internal/restconf"
	"infix/webui/internal/testutil"
)

// A failed copy says why in the body, which the page shows in place of
// the Configure content.
func TestConfigureEnterAnswers(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want int
		body string
	}{
		{"ok", nil, http.StatusNoContent, ""},
		{"denied", &restconf.Error{StatusCode: http.StatusForbidden}, http.StatusForbidden, "permission"},
		{"failed", errors.New(`restconf: invalid path "/ds/x?y"`), http.StatusBadGateway, "invalid path"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			rc := testutil.NewMockFetcher()
			rc.CopyError = c.err
			h := &ConfigureHandler{RC: rc}
			w := httptest.NewRecorder()
			h.Enter(w, httptest.NewRequest(http.MethodPost, "/configure/enter", nil))
			if w.Code != c.want || !strings.Contains(w.Body.String(), c.body) {
				t.Errorf("status %d, want %d; body %q", w.Code, c.want, w.Body.String())
			}
		})
	}
}
