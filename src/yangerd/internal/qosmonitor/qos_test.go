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

func buildJSON(t *testing.T, p port) map[string]any {
	t.Helper()
	out := build(p)
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
	if out := build(p); out != nil {
		t.Fatalf("want nil, got %v", out)
	}
}

func TestBuildSwitchPort(t *testing.T) {
	p := port{
		TxQueues: 4,
		Driver:   "sparx5-switch",
		App:      json.RawMessage(`{"dscp_prio":[[0,0],[46,6]],"pcp_prio":[]}`),
		Rewr:     json.RawMessage(`{"prio_pcp":[],"prio_dscp":[]}`),
		Qdiscs:   json.RawMessage(`[{"kind":"ets","handle":"8001:","root":true,"offloaded":true,"options":{"bands":4}}]`),
	}
	got := buildJSON(t, p)
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
	m := New(tc, fakeBatch{}, func() json.RawMessage { return json.RawMessage(links) },
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
	m := New(fakeBatch{}, fakeBatch{}, func() json.RawMessage { return nil },
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
