package qosmonitor

import (
	"encoding/json"
)

// trustOrders are the orders a driver's dcb apptrust accepts, by driver
// name; the kernel has no query for it.  Same table as confd's qos
// validation.
var trustOrders = map[string][]string{
	"sparx5-switch":  {"pcp", "dscp", "dscp-pcp"},
	"lan966x-switch": {"pcp", "dscp", "dscp-pcp"},
	"mv88e6085":      {"pcp", "dscp", "pcp-dscp", "dscp-pcp"},
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

// build returns the members the port adds to its interface entry, keyed
// by module-prefixed node name, or nil when it has none.
func build(p port) map[string]any {
	var qdiscs []qdisc
	json.Unmarshal(p.Qdiscs, &qdiscs)

	qos := map[string]any{}
	if caps := capabilities(p, qdiscs); len(caps) > 0 {
		qos["capabilities"] = caps
	}

	out := map[string]any{}
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
		}
	}
	if len(offload) > 0 {
		caps["offload"] = offload
	}
	return caps
}
