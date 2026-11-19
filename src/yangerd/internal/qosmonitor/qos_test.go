package qosmonitor

import (
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/mdlayher/netlink"
	"golang.org/x/sys/unix"
)

func buildJSON(t *testing.T, p port, mods Modules) map[string]any {
	t.Helper()
	out := build(p, mods)
	raw, err := json.Marshal(out)
	if err != nil {
		t.Fatal(err)
	}
	var got map[string]any
	json.Unmarshal(raw, &got)
	return got
}

func TestBuildNothing(t *testing.T) {
	p := port{TxQueues: 1, Qdiscs: json.RawMessage(`[{"kind":"noqueue","handle":"0:","root":true,"options":{}}]`)}
	if out := build(p, Modules{}); out != nil {
		t.Fatalf("want nil, got %v", out)
	}
}

func TestBuildSwitchPort(t *testing.T) {
	p := port{
		TxQueues:  4,
		Trust:     []string{"pcp", "dscp", "pcp-dscp", "dscp-pcp"},
		Supported: []string{"classification", "remarking", "transmission-selection", "egress-shaper"},
		DCB: dcbTables{
			HasTrust: true,
			Trust:    []uint8{dcbSelectorPCP},
			App:      []dcbApp{{dcbSelectorEther, 2, 0}, {dcbSelectorPCP, 3, 11}},
		},
		Qdiscs: json.RawMessage(`[
			{"kind":"tbf","handle":"8001:","root":true,"offloaded":true,"options":{"rate":12500000,"burst":15000}},
			{"kind":"ets","handle":"8002:","parent":"8001:1","offloaded":true,"options":{"bands":4}}]`),
		Profile: "edge",
	}
	got := buildJSON(t, p, Modules{})
	want := map[string]any{"infix-interfaces:qos": map[string]any{
		"capabilities": map[string]any{
			"max-traffic-classes":   4.0,
			"supported-trust-order": []any{"pcp", "dscp", "pcp-dscp", "dscp-pcp"},
			"supported-offload":     []any{"classification", "remarking", "transmission-selection", "egress-shaper"},
			"offload":               []any{"classification", "egress-shaper", "transmission-selection"},
		},
		"active-profile": "edge",
		"ingress": map[string]any{
			"trust":            "pcp",
			"default-priority": 2.0,
			"pcp-map": map[string]any{"entry": []any{
				map[string]any{"pcp": 3.0, "dei": true, "priority": 3.0},
			}},
		},
		"egress-shaper": map[string]any{"rate": "100000000", "burst": 15000.0},
	}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got  %v\nwant %v", got, want)
	}
}

func TestBuildCBSUnderETS(t *testing.T) {
	p := port{
		TxQueues: 8,
		Qdiscs: json.RawMessage(`[
			{"kind":"ets","handle":"1:","root":true,"options":{"bands":8}},
			{"kind":"cbs","handle":"8003:","parent":"1:2","options":{"idleslope":20000,"offload":0}}]`),
	}
	got := buildJSON(t, p, Modules{CBSA: true})
	want := map[string]any{"ieee802-dot1dc-cbsa-if:cbsa": map[string]any{
		"cbsa-parameter-table": []any{map[string]any{"traffic-class": 6.0, "oper-idle-slope": "20000000"}},
	}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got  %v\nwant %v", got, want)
	}
	if out := build(p, Modules{}); out != nil {
		t.Fatalf("cbsa reported without its module: %v", out)
	}
}

func TestBuildCBSUnderMqprio(t *testing.T) {
	p := port{
		TxQueues: 8,
		Qdiscs: json.RawMessage(`[
			{"kind":"mqprio","handle":"1:","root":true,"options":{"mode":"dcb"}},
			{"kind":"cbs","handle":"17:","parent":"1:8","options":{"idleslope":10000,"offload":1}}]`),
	}
	got := buildJSON(t, p, Modules{CBSA: true})
	want := map[string]any{
		"infix-interfaces:qos": map[string]any{"capabilities": map[string]any{
			"offload": []any{"transmission-selection", "credit-based-shaper"},
		}},
		"ieee802-dot1dc-cbsa-if:cbsa": map[string]any{
			"cbsa-parameter-table": []any{map[string]any{"traffic-class": 7.0, "oper-idle-slope": "10000000"}},
		},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got  %v\nwant %v", got, want)
	}
}

func TestScale(t *testing.T) {
	// A 10 Gbps slope over a cycle of a second overflows 64 bits
	if got := scale(10_000_000_000, 999_999_999, 999_999_999); got != 10_000_000_000 {
		t.Fatalf("got %d", got)
	}
}

func TestBuildGates(t *testing.T) {
	p := port{
		TxQueues: 8,
		Qdiscs: json.RawMessage(`[
			{"kind":"taprio","handle":"100:","root":true,"options":{"flags":"0x2","base_time":1500000000,
			 "cycle_time":1000000,"schedule":[
				{"index":1,"cmd":"S","gatemask":"0xfb","interval":750000},
				{"index":0,"cmd":"S","gatemask":"0x4","interval":250000}]}},
			{"kind":"cbs","handle":"8004:","parent":"100:3","options":{"idleslope":40000,"offload":1}}]`),
	}
	got := buildJSON(t, p, Modules{Sched: true, CBSA: true})
	want := map[string]any{
		"infix-interfaces:qos": map[string]any{"capabilities": map[string]any{
			"offload": []any{"transmission-selection", "time-aware-shaper", "credit-based-shaper"},
		}},
		"ieee802-dot1dc-sched-if:gate-parameter-table": map[string]any{
			"oper-control-list": map[string]any{"gate-control-entry": []any{
				map[string]any{"index": 0.0, "operation-name": "ieee802-dot1q-sched:set-gate-states",
					"gate-states-value": 4.0, "time-interval-value": 250000.0},
				map[string]any{"index": 1.0, "operation-name": "ieee802-dot1q-sched:set-gate-states",
					"gate-states-value": 251.0, "time-interval-value": 750000.0},
			}},
			"oper-cycle-time": map[string]any{"numerator": 1.0, "denominator": 1000.0},
			"oper-base-time":  map[string]any{"seconds": "1", "nanoseconds": 500000000.0},
			"config-pending":  false,
		},
		// Class 2 is open a quarter of the cycle, so the kernel runs
		// four times the reserved slope.
		"ieee802-dot1dc-cbsa-if:cbsa": map[string]any{"cbsa-parameter-table": []any{
			map[string]any{"traffic-class": 2.0, "oper-idle-slope": "10000000"},
		}},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got  %v\nwant %v", got, want)
	}
}

func TestBuildSchedLimits(t *testing.T) {
	p := port{
		TxQueues: 8,
		Sched:    schedLimits{ListMax: 3000, CycleMax: 999999999, IntervalMax: 999999999, Tick: 10},
		Qdiscs:   json.RawMessage(`[{"kind":"ets","handle":"1:","root":true,"options":{"bands":8}}]`),
	}
	got := buildJSON(t, p, Modules{Sched: true})
	want := map[string]any{
		"ieee802-dot1dc-sched-if:gate-parameter-table": map[string]any{
			"supported-list-max":     3000.0,
			"supported-cycle-max":    map[string]any{"numerator": 999999999.0, "denominator": 1000000000.0},
			"supported-interval-max": 999999999.0,
			"tick-granularity":       10.0,
		},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got  %v\nwant %v", got, want)
	}
	if out := build(p, Modules{}); out != nil {
		t.Fatalf("limits reported without the sched module: %v", out)
	}
}

type fakeDCB struct{ tables dcbTables }

func (f fakeDCB) Tables(string) (dcbTables, error) { return f.tables, nil }

type fakeBatch map[string]string

func (f fakeBatch) Query(cmd string) (json.RawMessage, error) {
	if out, ok := f[cmd]; ok {
		return json.RawMessage(out), nil
	}
	return nil, errors.New("command failed")
}

func TestDrainOnlyOnChange(t *testing.T) {
	dir := t.TempDir()
	tc := fakeBatch{"qdisc show dev if7": `[{"kind":"ets","handle":"1:","root":true,"offloaded":true,"options":{"bands":8}}]`}
	links := `[{"ifname":"e1","ifindex":7,"num_tx_queues":8}]`
	m := New(tc, fakeDCB{}, Modules{}, nil, func() json.RawMessage { return json.RawMessage(links) },
		slog.New(slog.NewTextHandler(io.Discard, nil)))
	m.runDir = dir
	m.sysfs = dir

	var calls []map[string]json.RawMessage
	m.SetOnUpdate(func(c map[string]json.RawMessage) { calls = append(calls, c) })

	m.refreshIndex(7)
	m.drain()
	m.Refresh("e1")
	m.drain()
	if len(calls) != 1 {
		t.Fatalf("want one update for unchanged state, got %v", calls)
	}

	os.WriteFile(filepath.Join(dir, "e1"), []byte("edge\n"), 0o644)
	m.Refresh("e1")
	m.drain()
	if len(calls) != 2 || string(calls[1]["e1"]) != `{"infix-interfaces:qos":{"active-profile":"edge","capabilities":{"offload":["transmission-selection"]}}}` {
		t.Fatalf("profile not picked up: %v", calls)
	}

	links = `[]`
	m.Refresh("e1")
	m.drain()
	if got, ok := calls[len(calls)-1]["e1"]; len(calls) != 3 || !ok || got != nil {
		t.Fatalf("want removal, got %v", calls)
	}
}

func TestDispatch(t *testing.T) {
	m := New(fakeBatch{}, fakeDCB{}, Modules{}, nil, func() json.RawMessage { return nil },
		slog.New(slog.NewTextHandler(io.Discard, nil)))

	ae := netlink.NewAttributeEncoder()
	ae.String(dcbAttrIfname, "e3")
	attrs, _ := ae.Encode()
	m.dispatch(netlink.Message{
		Header: netlink.Header{Type: unix.RTM_SETDCB},
		Data:   append([]byte{0, 0, 0, 0}, attrs...),
	})
	// A filter notification on the TC group is not about qdiscs.
	m.dispatch(netlink.Message{Header: netlink.Header{Type: unix.RTM_NEWTFILTER}, Data: make([]byte, sizeofTcmsg)})

	if len(m.pending) != 1 {
		t.Fatalf("want e3 pending, got %v", m.pending)
	}
	if _, ok := m.pending["e3"]; !ok {
		t.Fatalf("want e3 pending, got %v", m.pending)
	}
}

func TestIngressFilters(t *testing.T) {
	p := port{Ingress: json.RawMessage(`[
		{"protocol":"ip","pref":100,"kind":"flower","chain":0},
		{"protocol":"ip","pref":100,"kind":"flower","options":{"keys":{"eth_type":"ipv4","ip_tos":0,"ip_tos_mask":"fc"},
		 "actions":[{"kind":"skbedit","priority":"none"}]}},
		{"protocol":"ip","pref":100,"kind":"flower","options":{"keys":{"eth_type":"ipv4","ip_tos":184,"ip_tos_mask":"fc"},
		 "actions":[{"kind":"skbedit","priority":":5"}]}},
		{"protocol":"ipv6","pref":101,"kind":"flower","options":{"keys":{"eth_type":"ipv6","ip_tos":184,"ip_tos_mask":"fc"},
		 "actions":[{"kind":"skbedit","priority":":5"}]}},
		{"protocol":"802.1Q","pref":200,"kind":"flower","options":{"keys":{"vlan_prio":7},
		 "actions":[{"kind":"skbedit","priority":":7"}]}},
		{"protocol":"all","pref":900,"kind":"matchall","options":{"actions":[{"kind":"skbedit","priority":":2"}]}}]`)}
	raw, _ := json.Marshal(ingress(p))
	var got map[string]any
	json.Unmarshal(raw, &got)
	want := map[string]any{
		"trust":            "dscp-pcp",
		"default-priority": 2.0,
		"dscp-map": map[string]any{"entry": []any{
			map[string]any{"dscp": 0.0, "priority": 0.0},
			map[string]any{"dscp": 46.0, "priority": 5.0},
		}},
		"pcp-map": map[string]any{"entry": []any{
			map[string]any{"pcp": 7.0, "dei": false, "priority": 7.0},
			map[string]any{"pcp": 7.0, "dei": true, "priority": 7.0},
		}},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got  %v\nwant %v", got, want)
	}

	// The catch-all alone: nothing is trusted
	p.Ingress = json.RawMessage(`[{"pref":900,"kind":"matchall","options":{"actions":[{"kind":"skbedit","priority":"none"}]}}]`)
	if got := ingress(p); got["trust"] != "none" || got["default-priority"] != 0 {
		t.Fatalf("got %v", got)
	}
	if got := ingress(port{}); got != nil {
		t.Fatalf("want nil, got %v", got)
	}
}

func egressJSON(t *testing.T, p port) map[string]any {
	t.Helper()
	var qdiscs []qdisc
	json.Unmarshal(p.Qdiscs, &qdiscs)
	raw, err := json.Marshal(egress(p, qdiscs))
	if err != nil {
		t.Fatal(err)
	}
	var got map[string]any
	json.Unmarshal(raw, &got)
	return got
}

func class(id float64, algo string, share ...float64) map[string]any {
	c := map[string]any{"id": id, "algorithm": algo}
	if len(share) > 0 {
		c["bandwidth"] = share[0]
	}
	return c
}

func TestEgressETS(t *testing.T) {
	// Four classes under an egress shaper: 3 shaped, 2 strict, 1 and 0 share 70/30
	p := port{
		Remarks: true,
		Qdiscs: json.RawMessage(`[
			{"kind":"tbf","handle":"1:","root":true,"options":{"rate":12500000,"burst":15000}},
			{"kind":"ets","handle":"2:","parent":"1:1","options":{"bands":4,"strict":2,"quanta":[106260,45540],
			 "priomap":[3,3,2,2,1,1,0,0,3,3,3,3,3,3,3,3]}},
			{"kind":"cbs","handle":"8003:","parent":"2:1","options":{"idleslope":20000,"offload":0}}]`),
	}
	want := map[string]any{
		"traffic-class-table": map[string]any{
			"priority0": 0.0, "priority1": 0.0, "priority2": 1.0, "priority3": 1.0,
			"priority4": 2.0, "priority5": 2.0, "priority6": 3.0, "priority7": 3.0,
		},
		"traffic-class": []any{
			class(3, algoCBS), class(2, algoStrict), class(1, algoETS, 70), class(0, algoETS, 30),
		},
		"remark": map[string]any{"dscp": "from-priority"},
	}
	if got := egressJSON(t, p); !reflect.DeepEqual(got, want) {
		t.Fatalf("got  %v\nwant %v", got, want)
	}
}

func TestEgressQueues(t *testing.T) {
	// mqprio and taprio number the classes as the model does
	p := port{
		PCPEncoded: true,
		DCB:        dcbTables{Rewr: []dcbApp{{dcbSelectorDSCP, 1, 8}}},
		Qdiscs: json.RawMessage(`[
			{"kind":"mqprio","handle":"1:","root":true,"options":{"tc":2,"map":[0,0,0,0,1,1,1,1,0,0,0,0,0,0,0,0],"mode":"dcb"}},
			{"kind":"cbs","handle":"8003:","parent":"1:2","options":{"idleslope":20000,"offload":1}}]`),
	}
	want := map[string]any{
		"traffic-class-table": map[string]any{
			"priority0": 0.0, "priority1": 0.0, "priority2": 0.0, "priority3": 0.0,
			"priority4": 1.0, "priority5": 1.0, "priority6": 1.0, "priority7": 1.0,
		},
		"traffic-class": []any{class(1, algoCBS), class(0, algoStrict)},
		"remark":        map[string]any{"pcp": "from-priority", "dscp": "from-priority"},
	}
	if got := egressJSON(t, p); !reflect.DeepEqual(got, want) {
		t.Fatalf("got  %v\nwant %v", got, want)
	}
	if got := egress(port{}, nil); got != nil {
		t.Fatalf("want nil, got %v", got)
	}
}
