// SPDX-License-Identifier: MIT

package handlers

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"sort"
	"strconv"
	"strings"
)

// qosTableSR is the class table preset that puts the stream reservation
// classes on the two highest traffic classes.
const qosTableSR = "ieee-sr"

// reservation is what the Reserve bandwidth form asks for.
type reservation struct {
	Ports   []string
	Class   int
	Bits    string // the idle slope, bits per second
	SRTable bool   // switch the class table to qosTableSR
}

// reservePlan is the writes a reservation takes: per profile and per
// port with its own settings the egress nodes to merge in, and the
// idle slope on every port.
type reservePlan struct {
	Profiles map[string]map[string]any
	Own      map[string]map[string]any
	Notes    []string
}

// qosClassCount is the number of traffic classes a port has.
func qosClassCount(oper *ifaceJSON) int {
	if oper != nil && oper.QoS != nil && oper.QoS.Capabilities != nil && oper.QoS.Capabilities.MaxTrafficClasses > 0 {
		return oper.QoS.Capabilities.MaxTrafficClasses
	}
	return qosNumClasses
}

// planReservation works out what a reservation changes, and refuses
// one the device would reject at commit: shaped classes are the
// highest on a port, so every class above the reserved one must be
// shaped already.  Nothing is written.
func planReservation(res reservation, ifaces, oper []ifaceJSON, profiles []qosProfileJSON) (reservePlan, error) {
	plan := reservePlan{Profiles: map[string]map[string]any{}, Own: map[string]map[string]any{}}
	profByName := map[string]qosProfileJSON{}
	for _, p := range profiles {
		profByName[p.Name] = p
	}
	byName := map[string]*ifaceJSON{}
	for i := range ifaces {
		byName[ifaces[i].Name] = &ifaces[i]
	}
	operByName := map[string]*ifaceJSON{}
	for i := range oper {
		operByName[oper[i].Name] = &oper[i]
	}

	for _, name := range res.Ports {
		iface := byName[name]
		if iface == nil {
			return plan, fmt.Errorf("no interface %s", name)
		}
		if n := qosClassCount(operByName[name]); res.Class >= n {
			return plan, fmt.Errorf("%s has %d traffic classes, 0 to %d", name, n, n-1)
		}
		profile := ""
		if iface.QoS != nil {
			profile = iface.QoS.Profile
		}
		eg := qosEgressOf(iface, profByName)
		shaped := qosShapedClasses(eg)
		for c := res.Class + 1; c < qosClassCount(operByName[name]); c++ {
			if !shaped[c] {
				return plan, fmt.Errorf("%s: class %d is above class %d and not shaped; shaped classes must be the highest, reserve for class %d first",
					name, c, res.Class, c)
			}
		}

		egress := map[string]any{}
		if !shaped[res.Class] {
			egress["traffic-class"] = []any{map[string]any{"id": res.Class, "algorithm": qosAlgoCBS}}
		}
		if res.SRTable {
			switch preset := qosTablePreset(eg); preset {
			case qosTableSR:
			case qosCustom:
				plan.Notes = append(plan.Notes, fmt.Sprintf("%s keeps its custom class table", qosOwner(name, profile)))
			default:
				egress["traffic-class-table"] = map[string]any{"preset": qosTableSR}
			}
		}
		if len(egress) == 0 {
			continue
		}
		if profile == "" {
			plan.Own[name] = egress
		} else if plan.Profiles[profile] == nil {
			plan.Profiles[profile] = egress
			if _, changed := egress["traffic-class"]; changed {
				plan.Notes = append(plan.Notes, fmt.Sprintf("profile %s now shapes class %d on all its ports", profile, res.Class))
			}
		}
	}
	return plan, nil
}

func qosOwner(port, profile string) string {
	if profile != "" {
		return "profile " + profile
	}
	return port
}

// qosTablePreset names the class table an egress uses; the model's
// default when it sets none.
func qosTablePreset(eg *qosEgressJSON) string {
	if eg == nil || eg.Table == nil {
		return ""
	}
	return eg.Table.Preset
}

func reservationFromForm(r *http.Request) (reservation, error) {
	res := reservation{Ports: r.Form["port"], SRTable: r.FormValue("sr-table") == "on"}
	if len(res.Ports) == 0 {
		return res, fmt.Errorf("pick at least one port")
	}
	class, err := strconv.Atoi(r.FormValue("class"))
	if err != nil || class < 0 || class >= qosNumClasses {
		return res, fmt.Errorf("pick a traffic class")
	}
	res.Class = class
	res.Bits, err = mbitField(strings.TrimSpace(r.FormValue("rate")), "the rate")
	return res, err
}

// ReserveBandwidth guarantees a traffic class a rate on the ports
// chosen: the class is given the credit-based shaper, in the port's
// profile or its own settings, and the rate as its idle slope.
// POST /configure/tsn/reserve
func (h *ConfigureInterfacesHandler) ReserveBandwidth(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	res, err := reservationFromForm(r)
	if err != nil {
		renderSaveError(w, err)
		return
	}
	ctx := context.WithoutCancel(r.Context())
	fail := func(err error) {
		log.Printf("configure tsn reserve: %v", err)
		renderSaveError(w, err)
	}
	ifaces, oper, profiles, err := h.qosPortData(ctx)
	if err != nil {
		fail(err)
		return
	}
	plan, err := planReservation(res, ifaces, oper, profiles)
	if err != nil {
		renderSaveError(w, err)
		return
	}

	for name, egress := range plan.Profiles {
		body := map[string]any{"ietf-system:system": map[string]any{
			"infix-qos:qos-profile": []any{map[string]any{"name": name, "egress": egress}}}}
		if err := h.RC.Patch(ctx, candidatePath+qosSystemPath, body); err != nil {
			fail(err)
			return
		}
	}
	for name, egress := range plan.Own {
		body := map[string]any{"infix-interfaces:qos": map[string]any{"egress": egress}}
		if err := h.RC.Patch(ctx, ifacePath(name)+"/infix-interfaces:qos", body); err != nil {
			fail(err)
			return
		}
	}
	for _, name := range res.Ports {
		body := map[string]any{"ieee802-dot1dc-cbsa-if:cbsa": map[string]any{"cbsa-parameter-table": []any{
			map[string]any{"traffic-class": res.Class, "admin-idle-slope": res.Bits}}}}
		if err := h.RC.Patch(ctx, ifacePath(name)+cbsaNode, body); err != nil {
			fail(err)
			return
		}
	}

	ports := append([]string(nil), res.Ports...)
	sort.Slice(ports, func(i, j int) bool { return naturalLess(ports[i], ports[j]) })
	msg := fmt.Sprintf("Reserved %s Mbps for class %d on %s", strings.TrimSpace(r.FormValue("rate")), res.Class, strings.Join(ports, ", "))
	if len(plan.Notes) > 0 {
		msg += "; " + strings.Join(plan.Notes, "; ")
	}
	renderSavedRedirect(w, msg, "/configure/tsn")
}
