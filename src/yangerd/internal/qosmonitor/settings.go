package qosmonitor

import (
	"bytes"
	"encoding/json"
	"sort"
	"strconv"
	"strings"
)

const (
	algoStrict = "ieee802-dot1q-types:strict-priority"
	algoETS    = "ieee802-dot1q-types:enhanced-transmission-selection"
	algoCBS    = "ieee802-dot1q-types:credit-based-shaper"

	// prefDefault is the catch-all confd ends a port's software
	// classification with; the trusted fields come before it, one
	// hundred preferences apart.
	prefDefault = 900
)

// filter is the part of a `tc filter show` row used here.
type filter struct {
	Protocol string `json:"protocol"`
	Pref     int    `json:"pref"`
	Kind     string `json:"kind"`
	Options  *struct {
		Keys struct {
			VlanPrio *int `json:"vlan_prio"`
			IPTos    *int `json:"ip_tos"`
		} `json:"keys"`
		Actions []struct {
			Kind     string `json:"kind"`
			Priority string `json:"priority"`
		} `json:"actions"`
	} `json:"options"`
}

// priority is the skb priority a filter's skbedit action sets, which tc
// prints as a class id, "none" for zero.
func (f filter) priority() (int, bool) {
	if f.Options == nil {
		return 0, false
	}
	for _, a := range f.Options.Actions {
		if a.Kind != "skbedit" || a.Priority == "" {
			continue
		}
		if a.Priority == "none" {
			return 0, true
		}
		_, minor, _ := strings.Cut(a.Priority, ":")
		n, err := strconv.ParseInt(minor, 16, 32)
		return int(n), err == nil
	}
	return 0, false
}

// ingress is the classification a port runs: from the driver's DCB
// tables where it has them, else from the tc filters confd classifies
// with in software.  Nil when the port classifies nothing.
func ingress(p port) map[string]any {
	if p.DCB.HasTrust || len(p.DCB.App) > 0 {
		return ingressDCB(p.DCB)
	}
	return ingressFilters(p.Ingress)
}

func ingressDCB(t dcbTables) map[string]any {
	out := map[string]any{}
	if t.HasTrust {
		var order []string
		for _, sel := range t.Trust {
			switch sel {
			case dcbSelectorPCP:
				order = append(order, "pcp")
			case dcbSelectorDSCP:
				order = append(order, "dscp")
			}
		}
		out["trust"] = trustName(order)
	}

	var pcp, dscp []any
	for _, a := range t.App {
		switch a.Selector {
		case dcbSelectorEther:
			if a.Protocol == 0 {
				out["default-priority"] = a.Priority
			}
		case dcbSelectorPCP:
			pcp = append(pcp, map[string]any{
				"pcp": a.Protocol & 7, "dei": a.Protocol&8 != 0, "priority": a.Priority,
			})
		case dcbSelectorDSCP:
			dscp = append(dscp, map[string]any{"dscp": a.Protocol, "priority": a.Priority})
		}
	}
	return withMaps(out, pcp, dscp)
}

func ingressFilters(raw json.RawMessage) map[string]any {
	var filters []filter
	if json.Unmarshal(raw, &filters) != nil || len(filters) == 0 {
		return nil
	}

	out := map[string]any{}
	blocks := map[int]string{} // trusted field by its block of preferences
	seen := map[int]bool{}
	var pcp, dscp []any
	for _, f := range filters {
		prio, ok := f.priority()
		if !ok {
			continue
		}
		switch {
		case f.Kind == "matchall" && f.Pref == prefDefault:
			out["default-priority"] = prio
		case f.Kind != "flower":
		case f.Options.Keys.VlanPrio != nil:
			blocks[f.Pref/100] = "pcp"
			// flower cannot match the DEI, one rule serves both
			for _, dei := range []bool{false, true} {
				pcp = append(pcp, map[string]any{
					"pcp": *f.Options.Keys.VlanPrio, "dei": dei, "priority": prio,
				})
			}
		case f.Options.Keys.IPTos != nil:
			blocks[f.Pref/100] = "dscp"
			// One rule per IP version, tagged and untagged
			if code := *f.Options.Keys.IPTos >> 2; !seen[code] {
				seen[code] = true
				dscp = append(dscp, map[string]any{"dscp": code, "priority": prio})
			}
		}
	}
	if len(out) == 0 && len(blocks) == 0 {
		return nil
	}

	prefs := make([]int, 0, len(blocks))
	for pref := range blocks {
		prefs = append(prefs, pref)
	}
	sort.Ints(prefs)
	order := make([]string, 0, len(prefs))
	for _, pref := range prefs {
		order = append(order, blocks[pref])
	}
	out["trust"] = trustName(order)
	return withMaps(out, pcp, dscp)
}

// trustName is the trust leaf for the fields trusted, in order.
func trustName(order []string) string {
	if len(order) == 0 {
		return "none"
	}
	return strings.Join(order, "-")
}

func withMaps(out map[string]any, pcp, dscp []any) map[string]any {
	if len(pcp) > 0 {
		out["pcp-map"] = map[string]any{"entry": pcp}
	}
	if len(dscp) > 0 {
		out["dscp-map"] = map[string]any{"entry": dscp}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// scheduler finds the qdisc that selects between the traffic classes:
// the root, or the child of an egress shaper at the root.
func scheduler(qdiscs []qdisc) *qdisc {
	var root *qdisc
	for i := range qdiscs {
		if qdiscs[i].Root {
			root = &qdiscs[i]
			break
		}
	}
	sched := root
	if root != nil && root.Kind == "tbf" {
		sched = nil
		for i := range qdiscs {
			if qdiscs[i].Parent == root.Handle+"1" {
				sched = &qdiscs[i]
				break
			}
		}
	}
	if sched == nil || (sched.Kind != "ets" && sched.Kind != "mqprio" && sched.Kind != "taprio") {
		return nil
	}
	return sched
}

// schedOpts is the layout part of an ets, mqprio or taprio qdisc.
type schedOpts struct {
	Bands   int   `json:"bands"` // ets
	Strict  int   `json:"strict"`
	Quanta  []int `json:"quanta"`
	PrioMap []int `json:"priomap"`
	NumTC   int   `json:"tc"` // mqprio, taprio
	Map     []int `json:"map"`
}

// egress is the traffic class layout a port runs, from its scheduler
// qdisc and the shapers below it, and what it remarks.  The bands of an
// ets qdisc count down from the top class, strict ones first; mqprio
// and taprio number the classes as the model does.
func egress(p port, qdiscs []qdisc) map[string]any {
	out := map[string]any{}

	if sched := scheduler(qdiscs); sched != nil {
		var o schedOpts
		json.Unmarshal(sched.Options, &o)

		num, prioMap := o.NumTC, o.Map
		if sched.Kind == "ets" {
			num, prioMap = o.Bands, o.PrioMap
		}
		if len(prioMap) >= 8 && num > 0 {
			table := map[string]any{}
			for prio := 0; prio < 8; prio++ {
				tc := prioMap[prio]
				if sched.Kind == "ets" {
					tc = num - 1 - tc
				}
				table["priority"+strconv.Itoa(prio)] = tc
			}
			out["traffic-class-table"] = table
			out["traffic-class"] = classes(sched, o, num, qdiscs)
		}
	}

	remark := map[string]any{}
	for _, a := range p.DCB.Rewr {
		switch a.Selector {
		case dcbSelectorPCP:
			remark["pcp"] = "from-priority"
		case dcbSelectorDSCP:
			remark["dscp"] = "from-priority"
		}
	}
	if p.PCPEncoded {
		remark["pcp"] = "from-priority"
	}
	if p.Remarks {
		remark["dscp"] = "from-priority"
	}
	if len(remark) > 0 {
		out["remark"] = remark
	}

	if len(out) == 0 {
		return nil
	}
	return out
}

func classes(sched *qdisc, o schedOpts, num int, qdiscs []qdisc) []any {
	shaped := map[int]bool{}
	for _, q := range qdiscs {
		if tc, ok := classOf(q, sched, num); ok && q.Kind == "cbs" {
			shaped[tc] = true
		}
	}

	total := 0
	for _, quantum := range o.Quanta {
		total += quantum
	}

	list := make([]any, 0, num)
	for tc := num - 1; tc >= 0; tc-- {
		class := map[string]any{"id": tc, "algorithm": algoStrict}
		weighted := num - 1 - tc - o.Strict // index among the weighted bands
		switch {
		case shaped[tc]:
			class["algorithm"] = algoCBS
		case sched.Kind == "ets" && weighted >= 0 && weighted < len(o.Quanta) && total > 0:
			class["algorithm"] = algoETS
			class["bandwidth"] = (o.Quanta[weighted]*100 + total/2) / total
		}
		list = append(list, class)
	}
	return list
}

// classOf is the traffic class a qdisc below the scheduler serves.
func classOf(q qdisc, sched *qdisc, num int) (int, bool) {
	if !strings.HasPrefix(q.Parent, sched.Handle) {
		return 0, false
	}
	_, minorHex, _ := strings.Cut(q.Parent, ":")
	minor, err := strconv.ParseInt(minorHex, 16, 64)
	if err != nil || minor < 1 {
		return 0, false
	}
	if sched.Kind == "ets" {
		return num - int(minor), true
	}
	return int(minor) - 1, true
}

// hasClsact tells whether the port has the qdisc confd hangs its
// software classification and remarking on.
func hasClsact(raw json.RawMessage) bool {
	return bytes.Contains(raw, []byte(`"kind":"clsact"`))
}

// remarksInSoftware tells whether the port's egress has the filters
// confd remarks the DSCP with.  tc prints their match expression as
// text inside the JSON, so the answer is searched, not parsed.
func remarksInSoftware(raw json.RawMessage) bool {
	return bytes.Contains(raw, []byte(`"kind":"basic"`))
}
