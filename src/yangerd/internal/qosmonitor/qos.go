package qosmonitor

import (
	"encoding/json"
	"strings"
)

// trustOrders are the orders a driver's dcb apptrust accepts, by driver
// name; the kernel has no query for it.  Same table as confd's qos
// validation.
var trustOrders = map[string][]string{
	"sparx5-switch":  {"pcp", "dscp", "dscp-pcp"},
	"lan966x-switch": {"pcp", "dscp", "dscp-pcp"},
}

// port is what the kernel says about one port's QoS.
type port struct {
	TxQueues int
	Driver   string
	App      json.RawMessage // dcb app show, nil when not asked
	Rewr     json.RawMessage // dcb rewr show, nil when not asked
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

	// Stages the driver runs.  The root qdisc is only ever mqprio with
	// hw offload, which the kernel refuses without driver support, so
	// its presence means the driver schedules the classes; ets means the
	// kernel does.  mqprio never sets the offloaded flag.
	var offload []string
	if trust != nil {
		var app struct {
			DSCP json.RawMessage `json:"dscp_prio"`
			PCP  json.RawMessage `json:"pcp_prio"`
		}
		if json.Unmarshal(p.App, &app) == nil && (nonEmpty(app.DSCP) || nonEmpty(app.PCP)) {
			offload = append(offload, "classification")
		}
		var rewr struct {
			PCP  json.RawMessage `json:"prio_pcp"`
			DSCP json.RawMessage `json:"prio_dscp"`
		}
		if json.Unmarshal(p.Rewr, &rewr) == nil && (nonEmpty(rewr.PCP) || nonEmpty(rewr.DSCP)) {
			offload = append(offload, "remarking")
		}
	}

	for _, q := range qdiscs {
		switch q.Kind {
		case "mqprio":
			if q.Root {
				offload = append(offload, "transmission-selection")
			}
		}
	}
	if len(offload) > 0 {
		caps["offload"] = offload
	}
	return caps
}

func nonEmpty(raw json.RawMessage) bool {
	s := strings.TrimSpace(string(raw))
	return s != "" && s != "null" && s != "[]" && s != "{}"
}
