package qosmonitor

import (
	"encoding/json"
	"sort"
	"strconv"
	"strings"
)

// trustOrders are the orders a driver's dcb apptrust accepts, by driver
// name; the kernel has no query for it.  Same table as confd's qos
// validation.
var trustOrders = map[string][]string{
	"sparx5-switch":  {"pcp", "dscp", "dscp-pcp"},
	"lan966x-switch": {"pcp", "dscp", "dscp-pcp"},
	"mv88e6085":      {"pcp", "dscp", "pcp-dscp", "dscp-pcp"},
}

// Modules tells which optional YANG modules are installed, so their
// nodes may be reported.
type Modules struct {
	Sched bool // ieee802-dot1dc-sched-if
	CBSA  bool // ieee802-dot1dc-cbsa-if
}

// port is what the kernel says about one port's QoS.
type port struct {
	TxQueues int
	Driver   string
	App      bool            // PCP or DSCP entries in the DCB app table
	Rewr     bool            // PCP or DSCP entries in the DCB rewrite table
	Qdiscs   json.RawMessage // tc qdisc show
}

type qdisc struct {
	Kind      string          `json:"kind"`
	Handle    string          `json:"handle"`
	Parent    string          `json:"parent"`
	Root      bool            `json:"root"`
	Offloaded bool            `json:"offloaded"`
	Options   json.RawMessage `json:"options"`
}

type cbsOpts struct {
	IdleSlope int64 `json:"idleslope"`
	Offload   bool  `json:"offload"`
}

type taprioEntry struct {
	Index    int             `json:"index"`
	GateMask json.RawMessage `json:"gatemask"`
	Interval int64           `json:"interval"`
}

type taprioOpts struct {
	Bands     int             `json:"bands"` // ets
	Flags     json.RawMessage `json:"flags"`
	BaseTime  int64           `json:"base_time"`
	CycleTime int64           `json:"cycle_time"`
	Extension int64           `json:"cycle_time_extension"`
	Schedule  []taprioEntry   `json:"schedule"`
	Admin     json.RawMessage `json:"admin"`
}

// build returns the members the port adds to its interface entry, keyed
// by module-prefixed node name, or nil when it has none.
func build(p port, mods Modules) map[string]any {
	var qdiscs []qdisc
	json.Unmarshal(p.Qdiscs, &qdiscs)

	qos := map[string]any{}
	if caps := capabilities(p, qdiscs); len(caps) > 0 {
		qos["capabilities"] = caps
	}

	out := shapers(qdiscs, mods)
	if len(qos) > 0 {
		out["infix-interfaces:qos"] = qos
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func capabilities(p port, qdiscs []qdisc) map[string]any {
	caps := map[string]any{}

	// One class per transmit queue, at most eight.  A single queue has
	// no queue structure to respect, so the kernel's eight classes apply,
	// which is the model's default and left implicit.
	if p.TxQueues > 1 && p.TxQueues < 8 {
		caps["max-traffic-classes"] = p.TxQueues
	}

	trust := trustOrders[p.Driver]
	if trust != nil {
		caps["supported-trust-order"] = trust
	}

	// Stages the driver runs.  ets and tbf report offloaded when the
	// driver took them.
	var offload []string
	if trust != nil {
		if p.App {
			offload = append(offload, "classification")
		}
		if p.Rewr {
			offload = append(offload, "remarking")
		}
	}

	cbs := false
	for _, q := range qdiscs {
		switch q.Kind {
		case "ets":
			if q.Offloaded {
				offload = append(offload, "transmission-selection")
			}
		case "tbf":
			if q.Root && q.Offloaded {
				offload = append(offload, "rate-limit")
			}
		case "cbs":
			var o cbsOpts
			if !cbs && json.Unmarshal(q.Options, &o) == nil && o.Offload {
				offload = append(offload, "credit-based-shaper")
				cbs = true
			}
		case "taprio":
			var o taprioOpts
			if json.Unmarshal(q.Options, &o) == nil && number(o.Flags)&0x2 != 0 {
				offload = append(offload, "time-aware-shaper")
			}
		}
	}
	if len(offload) > 0 {
		caps["offload"] = offload
	}
	return caps
}

// shapers is the IEEE 802.1DC credit-based and time-aware shaper state
// of a port, read back from its qdisc tree.
//
// The sched entries of a taprio root are the operational gate control
// list.  A cbs child reports the idle slope the kernel runs; under gates
// that slope was scaled up by the cycle over the class's open time, so
// it is scaled back to the reservation it came from.
func shapers(qdiscs []qdisc, mods Modules) map[string]any {
	out := map[string]any{}

	var root, sched *qdisc
	for i := range qdiscs {
		if qdiscs[i].Root {
			root = &qdiscs[i]
			break
		}
	}
	sched = root
	if root != nil && root.Kind == "tbf" {
		sched = nil
		for i := range qdiscs {
			if qdiscs[i].Parent == root.Handle+"1" {
				sched = &qdiscs[i]
				break
			}
		}
	}
	if sched == nil || (sched.Kind != "ets" && sched.Kind != "taprio") {
		return out
	}

	var o taprioOpts
	json.Unmarshal(sched.Options, &o)
	gated := sched.Kind == "taprio"

	type gate struct {
		mask     uint64
		interval int64
	}
	var schedule []gate
	if gated {
		entries := append([]taprioEntry(nil), o.Schedule...)
		sort.SliceStable(entries, func(i, j int) bool { return entries[i].Index < entries[j].Index })
		for _, e := range entries {
			schedule = append(schedule, gate{number(e.GateMask), e.Interval})
		}
	}
	cycle := o.CycleTime
	if cycle == 0 {
		for _, g := range schedule {
			cycle += g.interval
		}
	}

	if gated && mods.Sched {
		const ns = 1_000_000_000
		div := gcd(cycle, ns)
		list := make([]any, 0, len(schedule))
		for i, g := range schedule {
			list = append(list, map[string]any{
				"index":               i,
				"operation-name":      "ieee802-dot1q-sched:set-gate-states",
				"gate-states-value":   g.mask,
				"time-interval-value": g.interval,
			})
		}
		table := map[string]any{
			"oper-control-list": map[string]any{"gate-control-entry": list},
			"oper-cycle-time":   map[string]any{"numerator": cycle / div, "denominator": ns / div},
			"oper-base-time": map[string]any{
				"seconds":     strconv.FormatInt(o.BaseTime/ns, 10),
				"nanoseconds": o.BaseTime % ns,
			},
			"config-pending": len(o.Admin) > 0,
		}
		if o.Extension != 0 {
			table["oper-cycle-time-extension"] = o.Extension
		}
		out["ieee802-dot1dc-sched-if:gate-parameter-table"] = table
	}

	if !mods.CBSA {
		return out
	}

	bands := o.Bands
	if bands == 0 {
		bands = 8
	}
	var classes []any
	for _, q := range qdiscs {
		if q.Kind != "cbs" || !strings.HasPrefix(q.Parent, sched.Handle) {
			continue
		}
		_, minorHex, _ := strings.Cut(q.Parent, ":")
		minor, err := strconv.ParseInt(minorHex, 16, 64)
		if err != nil {
			continue
		}
		tc := bands - int(minor)
		if gated {
			tc = int(minor) - 1
		}
		var c cbsOpts
		json.Unmarshal(q.Options, &c)
		idle := c.IdleSlope * 1000
		if gated && cycle > 0 {
			var open int64
			for _, g := range schedule {
				if g.mask&(1<<tc) != 0 {
					open += g.interval
				}
			}
			idle = idle * open / cycle
		}
		classes = append(classes, map[string]any{
			"traffic-class":   tc,
			"oper-idle-slope": strconv.FormatInt(idle, 10),
		})
	}
	if len(classes) > 0 {
		out["ieee802-dot1dc-cbsa-if:cbsa"] = map[string]any{"cbsa-parameter-table": classes}
	}
	return out
}

// number reads a JSON value tc prints either as a number or as a hex
// or decimal string.
func number(raw json.RawMessage) uint64 {
	s := strings.Trim(string(raw), `"`)
	n, err := strconv.ParseUint(s, 0, 64)
	if err != nil {
		return 0
	}
	return n
}

func gcd(a, b int64) int64 {
	for b != 0 {
		a, b = b, a%b
	}
	return a
}
