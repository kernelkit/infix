// SPDX-License-Identifier: MIT

package handlers

import (
	"sort"
	"strconv"
	"strings"
)

// ifaceShaperStatus is one class the port shapes, with the slope it runs.
type ifaceShaperStatus struct {
	Class    int
	IdleMbit string
}

type ifaceGateStatus struct {
	Index      int
	Open       string // the classes whose gates the entry opens, top down
	IntervalUs string
}

// ifaceTSNStatus is what the port's shapers run, from the operational
// datastore; nil when it runs neither.
type ifaceTSNStatus struct {
	Shapers []ifaceShaperStatus
	Gates   []ifaceGateStatus
	CycleUs string
	Base    string
	Pending bool
}

func tsnStatusOf(iface *ifaceJSON) *ifaceTSNStatus {
	st := &ifaceTSNStatus{}
	if iface.CBSA != nil {
		for _, e := range iface.CBSA.Table {
			if e.OperIdleSlope > 0 {
				st.Shapers = append(st.Shapers, ifaceShaperStatus{e.TrafficClass, mbitText(int64(e.OperIdleSlope))})
			}
		}
		sort.Slice(st.Shapers, func(i, j int) bool { return st.Shapers[i].Class > st.Shapers[j].Class })
	}
	if g := iface.Gates; g != nil {
		var cycle int64
		for _, row := range qosGateEntryRows(g.OperList) {
			var open []string
			for c := qosNumClasses - 1; c >= 0; c-- {
				if row.Open[c] {
					open = append(open, strconv.Itoa(c))
				}
			}
			st.Gates = append(st.Gates, ifaceGateStatus{row.Index, strings.Join(open, ", "), row.IntervalUs})
		}
		if g.OperList != nil {
			for _, e := range g.OperList.Entries {
				cycle += int64(e.TimeInterval)
			}
		}
		if ns := rationalNs(g.OperCycleTime); ns > 0 {
			cycle = ns
		}
		if len(st.Gates) > 0 {
			st.CycleUs = usText(cycle)
		}
		if b := g.OperBaseTime; b != nil {
			st.Base = strconv.FormatInt(int64(b.Seconds), 10) + " s " + strconv.Itoa(b.Nanoseconds) + " ns"
		}
		st.Pending = g.ConfigPending
	}
	if len(st.Shapers) == 0 && len(st.Gates) == 0 && !st.Pending {
		return nil
	}
	return st
}
