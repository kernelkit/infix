// SPDX-License-Identifier: MIT

package handlers

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"strings"

	"infix/webui/internal/restconf"
)

// timeSchedule is a window for one traffic class at the start of every cycle.
type timeSchedule struct {
	Ports    []string
	Class    int
	CycleNs  int64
	WindowNs int64
	Guard    bool // close all gates before the window
}

type gateEntry struct {
	Mask int
	Ns   int64
}

// planSchedule returns each port's gate control list; nothing is written.
func planSchedule(s timeSchedule, oper []ifaceJSON) (map[string][]gateEntry, error) {
	operByName := map[string]*ifaceJSON{}
	for i := range oper {
		operByName[oper[i].Name] = &oper[i]
	}
	plan := map[string][]gateEntry{}
	for _, name := range s.Ports {
		o := operByName[name]
		if o == nil {
			return nil, fmt.Errorf("no interface %s", name)
		}
		n := qosClassCount(o)
		if s.Class >= n {
			return nil, fmt.Errorf("%s has %d traffic classes, 0 to %d", name, n, n-1)
		}
		var guard int64
		if s.Guard {
			bps, _ := qosPortSpeed(o)
			if bps == 0 {
				return nil, fmt.Errorf("%s reports no speed to size a guard band from; create its schedule without one", name)
			}
			guard = guardBandNs(bps)
		}
		rest := s.CycleNs - s.WindowNs - guard
		if rest <= 0 {
			what := fmt.Sprintf("a %s µs window", usText(s.WindowNs))
			if guard > 0 {
				what += fmt.Sprintf(" and a %s µs guard band", usText(guard))
			}
			return nil, fmt.Errorf("%s: %s leave nothing of a %s µs cycle for the other classes", name, what, usText(s.CycleNs))
		}
		window := 1 << s.Class
		entries := []gateEntry{{window, s.WindowNs}, {(1<<n - 1) &^ window, rest}}
		if guard > 0 {
			entries = append(entries, gateEntry{0, guard})
		}
		plan[name] = entries
	}
	return plan, nil
}

func timeScheduleFromForm(r *http.Request) (timeSchedule, error) {
	s := timeSchedule{Guard: r.FormValue("guard") == "on"}
	var err error
	if s.Ports, s.Class, err = wizardPortsClass(r); err != nil {
		return s, err
	}
	var ok bool
	if s.CycleNs, ok, err = usField(r, "cycle", "the cycle"); err != nil {
		return s, err
	} else if !ok || s.CycleNs == 0 {
		return s, fmt.Errorf("the cycle must be above zero")
	}
	if s.WindowNs, ok, err = usField(r, "window", "the window"); err != nil {
		return s, err
	} else if !ok || s.WindowNs == 0 {
		return s, fmt.Errorf("the window must be above zero")
	}
	return s, nil
}

// TimeSchedule replaces the gate control list of the ports chosen.
// POST /configure/tsn/schedule
func (h *ConfigureInterfacesHandler) TimeSchedule(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	s, err := timeScheduleFromForm(r)
	if err != nil {
		renderSaveError(w, err)
		return
	}
	ctx := context.WithoutCancel(r.Context())
	fail := func(err error) {
		log.Printf("configure tsn schedule: %v", err)
		renderSaveError(w, err)
	}
	var oper interfacesWrapper
	if err := h.RC.Get(ctx, "/data/ietf-interfaces:interfaces", &oper); err != nil {
		fail(err)
		return
	}
	plan, err := planSchedule(s, oper.Interfaces.Interface)
	if err != nil {
		renderSaveError(w, err)
		return
	}

	patch := restconf.NewYangPatch(candidatePath)
	for _, name := range s.Ports {
		entries := plan[name]
		list := make([]map[string]any, len(entries))
		for i, e := range entries {
			list[i] = gateEntryBody(i, e.Mask, e.Ns)
		}
		target := ifaceTarget(name) + gatesNode
		patch.Remove(target + "/admin-control-list")
		patch.Merge(target, map[string]any{gatesContainer: map[string]any{
			"gate-enabled":       true,
			"admin-cycle-time":   cycleTimeBody(s.CycleNs),
			"admin-control-list": map[string]any{"gate-control-entry": list},
		}})
	}
	if err := h.RC.YangPatch(ctx, patch); err != nil {
		fail(err)
		return
	}

	msg := fmt.Sprintf("Schedule created on %s: class %d has the first %s µs of every %s µs",
		strings.Join(s.Ports, ", "), s.Class, usText(s.WindowNs), usText(s.CycleNs))
	renderSavedRedirect(w, msg, "/configure/tsn?open="+url.QueryEscape(s.Ports[0]))
}
