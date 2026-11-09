// SPDX-License-Identifier: MIT

package handlers

import (
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"testing"

	"infix/webui/internal/restconf"
)

func TestRCStatus(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want int
	}{
		{"device refused", &restconf.Error{StatusCode: http.StatusBadRequest}, http.StatusBadRequest},
		{"device failed", &restconf.Error{StatusCode: http.StatusInternalServerError}, http.StatusInternalServerError},
		{"no answer", fmt.Errorf("restconf request failed: %w",
			&url.Error{Op: "Get", Err: errors.New("connection refused")}), http.StatusBadGateway},
		{"never sent", errors.New(`restconf: invalid path "/x?y"`), http.StatusInternalServerError},
	}
	for _, c := range cases {
		if got := rcStatus(c.err); got != c.want {
			t.Errorf("%s: %d, want %d", c.name, got, c.want)
		}
	}
}
