package qosmonitor

import (
	"encoding/json"
	"errors"
	"io"
	"log/slog"
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
		TxQueues: 4,
		Driver:   "sparx5-switch",
		App:      true,
		Qdiscs:   json.RawMessage(`[{"kind":"ets","handle":"8001:","root":true,"offloaded":true,"options":{"bands":4}}]`),
	}
	got := buildJSON(t, p, Modules{})
	want := map[string]any{"infix-interfaces:qos": map[string]any{
		"capabilities": map[string]any{
			"max-traffic-classes":   4.0,
			"supported-trust-order": []any{"pcp", "dscp", "dscp-pcp"},
			"offload":               []any{"classification", "transmission-selection"},
		},
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
			{"kind":"cbs","handle":"8003:","parent":"1:2","options":{"idleslope":20000,"offload":false}}]`),
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

func TestBuildGates(t *testing.T) {
	p := port{
		TxQueues: 8,
		Qdiscs: json.RawMessage(`[
			{"kind":"taprio","handle":"100:","root":true,"options":{"flags":"0x2","base_time":1500000000,
			 "cycle_time":1000000,"schedule":[
				{"index":1,"cmd":"S","gatemask":"0xfb","interval":750000},
				{"index":0,"cmd":"S","gatemask":"0x4","interval":250000}]}},
			{"kind":"cbs","handle":"8004:","parent":"100:3","options":{"idleslope":40000,"offload":true}}]`),
	}
	got := buildJSON(t, p, Modules{Sched: true, CBSA: true})
	want := map[string]any{
		"infix-interfaces:qos": map[string]any{"capabilities": map[string]any{
			"offload": []any{"time-aware-shaper", "credit-based-shaper"},
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

type fakeDCB struct{ app, rewr bool }

func (f fakeDCB) Tables(string) (bool, bool, error) { return f.app, f.rewr, nil }

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
	links := `[{"ifname":"e1","ifindex":7,"num_tx_queues":4}]`
	m := New(tc, fakeDCB{}, Modules{}, func() json.RawMessage { return json.RawMessage(links) },
		slog.New(slog.NewTextHandler(io.Discard, nil)))
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

	links = `[]`
	m.Refresh("e1")
	m.drain()
	if got, ok := calls[len(calls)-1]["e1"]; len(calls) != 2 || !ok || got != nil {
		t.Fatalf("want removal, got %v", calls)
	}
}

func TestDispatch(t *testing.T) {
	m := New(fakeBatch{}, fakeDCB{}, Modules{}, func() json.RawMessage { return nil },
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
