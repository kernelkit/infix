// SPDX-License-Identifier: MIT

package handlers

import (
	"context"
	"reflect"
	"strings"
	"testing"

	"infix/webui/internal/testutil"
)

func TestPlanSchedule(t *testing.T) {
	caps := func(n int) *qosIfaceJSON { return &qosIfaceJSON{Capabilities: &qosCapsJSON{MaxTrafficClasses: n}} }
	oper := []ifaceJSON{
		{Name: "e1", Speed: 1e9, QoS: caps(8)},
		{Name: "e2", Speed: 1e8, QoS: caps(4)},
		{Name: "e3", QoS: caps(8)},
	}
	s := timeSchedule{Ports: []string{"e1", "e2"}, Class: 3, CycleNs: 1e6, WindowNs: 200e3, Guard: true}

	plan, err := planSchedule(s, oper)
	if err != nil {
		t.Fatal(err)
	}
	want := map[string][]gateEntry{
		"e1": {{0x08, 200e3}, {0xf7, 787600}, {0, 12400}},
		"e2": {{0x08, 200e3}, {0x07, 676300}, {0, 123700}},
	}
	if !reflect.DeepEqual(plan, want) {
		t.Errorf("got %v, want %v", plan, want)
	}

	s.Guard = false
	s.Ports = []string{"e3"}
	plan, err = planSchedule(s, oper)
	if err != nil {
		t.Fatal(err)
	}
	if want := []gateEntry{{0x08, 200e3}, {0xf7, 800e3}}; !reflect.DeepEqual(plan["e3"], want) {
		t.Errorf("no guard band: got %v", plan["e3"])
	}

	for _, tc := range []struct {
		name string
		s    timeSchedule
		want string
	}{
		{"window fills the cycle", timeSchedule{Ports: []string{"e1"}, Class: 7, CycleNs: 1e6, WindowNs: 990e3, Guard: true},
			"a 990 µs window and a 12.4 µs guard band leave nothing of a 1000 µs cycle"},
		{"class the port lacks", timeSchedule{Ports: []string{"e2"}, Class: 7, CycleNs: 1e6, WindowNs: 1e3}, "e2 has 4 traffic classes"},
		{"no speed for the guard band", timeSchedule{Ports: []string{"e3"}, Class: 7, CycleNs: 1e6, WindowNs: 1e3, Guard: true}, "reports no speed"},
		{"unknown port", timeSchedule{Ports: []string{"e9"}, Class: 7, CycleNs: 1e6, WindowNs: 1e3}, "no interface e9"},
	} {
		if _, err := planSchedule(tc.s, oper); err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Errorf("%s: got %v", tc.name, err)
		}
	}
}

func TestConfigureTSNOffersTimeSchedule(t *testing.T) {
	out := renderTSN(t)
	for _, want := range []string{`hx-post="/configure/tsn/schedule"`, `data-show="tsn-schedule"`, `name="window"`} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %s", want)
		}
	}
}

func TestPTPClockHint(t *testing.T) {
	ptp := NewCapabilities(map[string]bool{"ptp": true})
	hint := func(resp map[string]any, caps *Capabilities, port string) string {
		mock := testutil.NewMockFetcher()
		if resp != nil {
			mock.SetResponse("/data/ieee1588-ptp-tt:ptp", resp)
		}
		h := &ConfigureInterfacesHandler{RC: mock}
		return h.ptpClockHint(context.Background(), caps)(port)
	}
	for _, tc := range []struct {
		name             string
		clock, gm, state string
		caps             *Capabilities
		port             string
		want             string
	}{
		{"follows a grandmaster", "aa", "bb", "time-receiver", ptp, "e5", ""},
		{"is grandmaster", "aa", "aa", "time-transmitter", ptp, "e5", ""},
		{"listening", "aa", "bb", "listening", ptp, "e5", "PTP instance 0 is not synchronized"},
		{"port without PTP", "aa", "aa", "time-transmitter", ptp, "e6", "No PTP instance runs on this port"},
		{"no PTP support", "aa", "bb", "listening", NewCapabilities(nil), "e5", ""},
	} {
		if got := hint(ptpOperJSON(tc.clock, tc.gm, tc.state), tc.caps, tc.port); got != tc.want {
			t.Errorf("%s: got %q", tc.name, got)
		}
	}
	if got := hint(nil, ptp, "e5"); got != "No PTP instance runs on this port" {
		t.Errorf("no instances: got %q", got)
	}
}
