// SPDX-License-Identifier: MIT

package handlers

import (
	"reflect"
	"strings"
	"testing"
)

func reserveFixture() (ifaces, oper []ifaceJSON, profiles []qosProfileJSON) {
	caps := func(n int) *qosIfaceJSON { return &qosIfaceJSON{Capabilities: &qosCapsJSON{MaxTrafficClasses: n}} }
	ifaces = []ifaceJSON{
		{Name: "e1"},
		{Name: "e2", QoS: &qosIfaceJSON{Profile: "ring"}},
		{Name: "e3", QoS: &qosIfaceJSON{Profile: "ring"}},
		{Name: "e4", QoS: &qosIfaceJSON{Egress: &qosEgressJSON{
			Table:   &qosTableJSON{Preset: qosCustom},
			Classes: []qosClassJSON{{ID: 7, Algorithm: qosAlgoCBS}}}}},
		{Name: "e5"},
	}
	oper = []ifaceJSON{{Name: "e1", QoS: caps(8)}, {Name: "e2", QoS: caps(8)}, {Name: "e3", QoS: caps(8)},
		{Name: "e4", QoS: caps(8)}, {Name: "e5", QoS: caps(4)}}
	profiles = []qosProfileJSON{{Name: "ring"}}
	return
}

func TestPlanReservation(t *testing.T) {
	ifaces, oper, profiles := reserveFixture()
	shape7 := []any{map[string]any{"id": 7, "algorithm": qosAlgoCBS}}
	sr := map[string]any{"preset": qosTableSR}

	t.Run("own settings and a shared profile", func(t *testing.T) {
		plan, err := planReservation(reservation{Ports: []string{"e1", "e2", "e3"}, Class: 7, SRTable: true}, ifaces, oper, profiles)
		if err != nil {
			t.Fatal(err)
		}
		want := map[string]any{"traffic-class": shape7, "traffic-class-table": sr}
		if !reflect.DeepEqual(plan.Own, map[string]map[string]any{"e1": want}) {
			t.Errorf("own %v", plan.Own)
		}
		if !reflect.DeepEqual(plan.Profiles, map[string]map[string]any{"ring": want}) {
			t.Errorf("profiles %v", plan.Profiles)
		}
		if len(plan.Notes) != 1 || !strings.Contains(plan.Notes[0], "profile ring now shapes class 7") {
			t.Errorf("notes %v", plan.Notes)
		}
	})

	t.Run("already shaped, custom table kept", func(t *testing.T) {
		plan, err := planReservation(reservation{Ports: []string{"e4"}, Class: 7, SRTable: true}, ifaces, oper, profiles)
		if err != nil {
			t.Fatal(err)
		}
		if len(plan.Own) != 0 || len(plan.Profiles) != 0 {
			t.Errorf("writes %v %v", plan.Own, plan.Profiles)
		}
		if len(plan.Notes) != 1 || !strings.Contains(plan.Notes[0], "e4 keeps its custom class table") {
			t.Errorf("notes %v", plan.Notes)
		}
	})

	t.Run("next class down on a shaped port", func(t *testing.T) {
		plan, err := planReservation(reservation{Ports: []string{"e4"}, Class: 6}, ifaces, oper, profiles)
		if err != nil {
			t.Fatal(err)
		}
		want := map[string]any{"traffic-class": []any{map[string]any{"id": 6, "algorithm": qosAlgoCBS}}}
		if !reflect.DeepEqual(plan.Own["e4"], want) {
			t.Errorf("own %v", plan.Own)
		}
	})

	for name, c := range map[string]struct {
		res  reservation
		want string
	}{
		"class below an unshaped one": {reservation{Ports: []string{"e1"}, Class: 6}, "class 7 is above class 6 and not shaped"},
		"class the port lacks":        {reservation{Ports: []string{"e5"}, Class: 7}, "e5 has 4 traffic classes"},
		"unknown port":                {reservation{Ports: []string{"e9"}, Class: 7}, "no interface e9"},
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := planReservation(c.res, ifaces, oper, profiles); err == nil || !strings.Contains(err.Error(), c.want) {
				t.Errorf("got %v", err)
			}
		})
	}
}
