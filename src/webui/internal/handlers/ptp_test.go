// SPDX-License-Identifier: MIT

package handlers

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"infix/webui/internal/testutil"
)

func ptpOperJSON(clock, gm, state string) map[string]any {
	return map[string]any{
		"ieee1588-ptp-tt:ptp": map[string]any{"instances": map[string]any{"instance": []map[string]any{{
			"instance-index": 0,
			"default-ds": map[string]any{
				"clock-identity": clock, "priority1": 128, "priority2": 128,
				"domain-number": 0, "instance-type": "oc",
				"clock-quality": map[string]any{"clock-class": "ieee1588-ptp-tt:cc-default"},
			},
			"current-ds": map[string]any{"steps-removed": 1, "offset-from-time-transmitter": "-458752", "mean-delay": "0"},
			"parent-ds":  map[string]any{"grandmaster-identity": gm, "grandmaster-priority1": 1, "grandmaster-priority2": 128},
			"ports": map[string]any{"port": []map[string]any{{
				"port-index": 1, "underlying-interface": "e5",
				"port-ds":                                map[string]any{"port-state": state, "mean-link-delay": "98172928", "delay-mechanism": "p2p"},
				"ieee802-dot1as-gptp:port-statistics-ds": map[string]any{"rx-sync-count": 2371, "tx-sync-count": 0},
			}}},
		}}}},
	}
}

func ptpOverview(t *testing.T, resp map[string]any) string {
	t.Helper()
	mock := testutil.NewMockFetcher()
	if resp != nil {
		mock.SetResponse("/data/ieee1588-ptp-tt:ptp", resp)
	}
	h := &PTPHandler{Template: pageTemplate(t, "ptp.html"), RC: mock}
	w := httptest.NewRecorder()
	h.Overview(w, testRequest(http.MethodGet, "/ptp"))
	if w.Code != http.StatusOK {
		t.Fatalf("want 200 got %d: %s", w.Code, w.Body.String())
	}
	return w.Body.String()
}

func TestPTPOverview(t *testing.T) {
	const self, gm = "02-00-00-FF-FE-BA-03-05", "02-00-00-FF-FE-BA-02-07"
	cases := []struct {
		name string
		resp map[string]any
		want []string
		not  []string
	}{
		{"time receiver", ptpOperJSON(self, gm, "time-receiver"),
			[]string{"Follows grandmaster " + gm, "-7 ns", "1498 ns", "2371 / 0", "e5"},
			[]string{"Not synchronized"}},
		{"grandmaster", ptpOperJSON(self, self, "time-transmitter"),
			[]string{"Grandmaster, the other clocks follow this one"},
			[]string{"Offset", "Follows grandmaster"}},
		{"not synchronized", ptpOperJSON(self, gm, "listening"),
			[]string{"Not synchronized"},
			[]string{"Follows grandmaster"}},
		{"not configured", nil, []string{"PTP is not configured."}, nil},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			body := ptpOverview(t, c.resp)
			for _, want := range c.want {
				if !strings.Contains(body, want) {
					t.Errorf("missing %q in:\n%s", want, body)
				}
			}
			for _, not := range c.not {
				if strings.Contains(body, not) {
					t.Errorf("unexpected %q", not)
				}
			}
		})
	}
}
