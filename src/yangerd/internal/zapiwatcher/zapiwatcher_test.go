package zapiwatcher

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/kernelkit/infix/src/yangerd/internal/tree"
)

// fakeQuerier returns canned vtysh JSON per command.
type fakeQuerier struct {
	ipv4 string
	ipv6 string
	err  error
}

func (f fakeQuerier) Query(_ context.Context, command string) ([]byte, error) {
	if f.err != nil {
		return nil, f.err
	}
	if command == "show ipv6 route json" {
		if f.ipv6 == "" {
			return []byte("{}"), nil
		}
		return []byte(f.ipv6), nil
	}
	if f.ipv4 == "" {
		return []byte("{}"), nil
	}
	return []byte(f.ipv4), nil
}

func ipv4Routes(t *testing.T, tr *tree.Tree) []map[string]any {
	t.Helper()
	data := tr.Get(routingTreeKey)
	if data == nil {
		t.Fatal("routing tree key not set")
	}
	var routing map[string]any
	if err := json.Unmarshal(data, &routing); err != nil {
		t.Fatalf("unmarshal routing: %v", err)
	}
	ribs := routing["ribs"].(map[string]any)
	for _, rib := range ribs["rib"].([]any) {
		rm := rib.(map[string]any)
		if rm["name"] == "ipv4" {
			out := []map[string]any{}
			for _, r := range rm["routes"].(map[string]any)["route"].([]any) {
				out = append(out, r.(map[string]any))
			}
			return out
		}
	}
	t.Fatal("ipv4 rib not found")
	return nil
}

func TestProtocolName(t *testing.T) {
	cases := map[string]string{
		"kernel":    "infix-routing:kernel",
		"connected": "ietf-routing:direct",
		"local":     "ietf-routing:direct",
		"static":    "ietf-routing:static",
		"ospf":      "ietf-ospf:ospfv2",
		"rip":       "ietf-rip:rip",
		"bgp":       "infix-routing:kernel", // unknown -> kernel
		"":          "infix-routing:kernel",
	}
	for in, want := range cases {
		if got := protocolName(in); got != want {
			t.Errorf("protocolName(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestParseUptime(t *testing.T) {
	cases := map[string]time.Duration{
		"02:09:02": 2*time.Hour + 9*time.Minute + 2*time.Second,
		"00:00:30": 30 * time.Second,
		"3d04h05m": 3*24*time.Hour + 4*time.Hour + 5*time.Minute,
		"02w3d04h": 2*7*24*time.Hour + 3*24*time.Hour + 4*time.Hour,
		"bogus":    0,
		"":         0,
	}
	for in, want := range cases {
		if got := parseUptime(in); got != want {
			t.Errorf("parseUptime(%q) = %v, want %v", in, got, want)
		}
	}
}

// The user's exact bug: a static route is in the FIB (selected, distance
// 120) while a stale OSPF entry (distance 110) is still listed but not
// selected.  active must follow FRR's "selected" flag, not the lowest
// admin distance.
func TestActiveFollowsSelectedNotDistance(t *testing.T) {
	const j = `{
	  "192.168.20.0/24":[
	    {"prefix":"192.168.20.0/24","protocol":"ospf","selected":false,"distance":110,"metric":100,"uptime":"02:11:49",
	     "nexthops":[{"ip":"192.168.60.2","interfaceName":"e3","active":true}]},
	    {"prefix":"192.168.20.0/24","protocol":"static","selected":true,"installed":true,"distance":120,"metric":0,"uptime":"02:09:02",
	     "nexthops":[{"ip":"192.168.50.2","interfaceName":"e7","fib":true,"active":true}]}
	  ]
	}`

	tr := tree.New()
	w := New(tr, fakeQuerier{ipv4: j}, nil)
	w.writeRibs(context.Background())

	routes := ipv4Routes(t, tr)
	if len(routes) != 2 {
		t.Fatalf("expected 2 candidate routes, got %d", len(routes))
	}

	for _, r := range routes {
		pref := toInt(r["route-preference"])
		_, active := r["active"]
		switch pref {
		case 120:
			if !active {
				t.Error("static route (pref 120, selected) must be active")
			}
		case 110:
			if active {
				t.Error("ospf route (pref 110, not selected) must NOT be active")
			}
		default:
			t.Errorf("unexpected route-preference %d", pref)
		}
	}
}

// A full snapshot read means a route removed from zebra disappears from
// the cache without any ZAPI delete.
func TestSnapshotPurgesRemovedRoutes(t *testing.T) {
	const before = `{
	  "192.168.20.0/24":[
	    {"prefix":"192.168.20.0/24","protocol":"ospf","selected":true,"distance":110,"uptime":"00:05:00","nexthops":[{"ip":"192.168.60.2"}]}
	  ]
	}`
	const after = `{
	  "192.168.20.0/24":[
	    {"prefix":"192.168.20.0/24","protocol":"static","selected":true,"installed":true,"distance":120,"uptime":"00:01:00","nexthops":[{"ip":"192.168.50.2","fib":true}]}
	  ]
	}`

	tr := tree.New()
	New(tr, fakeQuerier{ipv4: before}, nil).writeRibs(context.Background())
	if got := len(ipv4Routes(t, tr)); got != 1 {
		t.Fatalf("before: expected 1 route, got %d", got)
	}

	// zebra now has only the static route; the OSPF route is gone with no
	// delete event.  A fresh snapshot must not carry the corpse forward.
	New(tr, fakeQuerier{ipv4: after}, nil).writeRibs(context.Background())
	routes := ipv4Routes(t, tr)
	if len(routes) != 1 {
		t.Fatalf("after: expected 1 route, got %d", len(routes))
	}
	if got := protocolName("static"); routes[0]["source-protocol"] != got {
		t.Errorf("surviving route protocol = %v, want %v", routes[0]["source-protocol"], got)
	}
}

func TestTransformRouteFields(t *testing.T) {
	entry := map[string]any{
		"prefix":    "10.0.0.0/24",
		"protocol":  "ospf",
		"selected":  true,
		"installed": true,
		"distance":  float64(110),
		"metric":    float64(20),
		"uptime":    "01:00:00",
		"nexthops": []any{
			map[string]any{"ip": "192.168.1.1", "interfaceName": "e1", "fib": true},
		},
	}

	now := time.Date(2026, 6, 10, 12, 0, 0, 0, time.UTC)
	var parsed map[string]any
	if err := json.Unmarshal(transformRoute("ipv4", "10.0.0.0/24", entry, now), &parsed); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if parsed["ietf-ipv4-unicast-routing:destination-prefix"] != "10.0.0.0/24" {
		t.Errorf("destination-prefix = %v", parsed["ietf-ipv4-unicast-routing:destination-prefix"])
	}
	if parsed["source-protocol"] != "ietf-ospf:ospfv2" {
		t.Errorf("source-protocol = %v", parsed["source-protocol"])
	}
	if toInt(parsed["route-preference"]) != 110 {
		t.Errorf("route-preference = %v", parsed["route-preference"])
	}
	if toInt(parsed["ietf-ospf:metric"]) != 20 {
		t.Errorf("ietf-ospf:metric = %v", parsed["ietf-ospf:metric"])
	}
	if _, ok := parsed["active"]; !ok {
		t.Error("selected route must have active leaf")
	}
	// last-updated = now - 1h
	if parsed["last-updated"] != "2026-06-10T11:00:00Z" {
		t.Errorf("last-updated = %v, want 2026-06-10T11:00:00Z", parsed["last-updated"])
	}

	hops := parsed["next-hop"].(map[string]any)["next-hop-list"].(map[string]any)["next-hop"].([]any)
	if len(hops) != 1 {
		t.Fatalf("expected 1 nexthop, got %d", len(hops))
	}
	hop := hops[0].(map[string]any)
	if hop["ietf-ipv4-unicast-routing:address"] != "192.168.1.1" {
		t.Errorf("nexthop address = %v", hop["ietf-ipv4-unicast-routing:address"])
	}
	if _, ok := hop["infix-routing:installed"]; !ok {
		t.Error("fib nexthop must have infix-routing:installed")
	}
}

func TestTransformRouteBlackhole(t *testing.T) {
	entry := map[string]any{
		"prefix":   "10.1.0.0/24",
		"protocol": "blackhole",
		"distance": float64(0),
		"uptime":   "00:00:10",
	}
	var parsed map[string]any
	if err := json.Unmarshal(transformRoute("ipv4", "10.1.0.0/24", entry, time.Now()), &parsed); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	nh := parsed["next-hop"].(map[string]any)
	if nh["special-next-hop"] != "blackhole" {
		t.Errorf("special-next-hop = %v, want blackhole", nh["special-next-hop"])
	}
}

func TestWriteRibsQueryErrorKeepsData(t *testing.T) {
	tr := tree.New()
	// Seed with good data.
	New(tr, fakeQuerier{ipv4: `{"10.0.0.0/24":[{"prefix":"10.0.0.0/24","protocol":"static","selected":true,"distance":1,"uptime":"00:00:05","nexthops":[{"ip":"10.0.0.1"}]}]}`}, nil).
		writeRibs(context.Background())
	before := tr.Get(routingTreeKey)

	// A failing query must not blank the table.
	New(tr, fakeQuerier{err: context.DeadlineExceeded}, nil).writeRibs(context.Background())
	after := tr.Get(routingTreeKey)

	if string(before) != string(after) {
		t.Error("query error overwrote existing rib data")
	}
}

func TestWriteRibsBothFamilies(t *testing.T) {
	tr := tree.New()
	w := New(tr, fakeQuerier{
		ipv4: `{"10.0.0.0/24":[{"prefix":"10.0.0.0/24","protocol":"static","selected":true,"distance":1,"uptime":"00:00:05","nexthops":[{"ip":"10.0.0.1"}]}]}`,
		ipv6: `{"2001:db8::/64":[{"prefix":"2001:db8::/64","protocol":"connected","selected":true,"distance":0,"uptime":"00:00:05","nexthops":[{"interfaceName":"e1"}]}]}`,
	}, nil)
	w.writeRibs(context.Background())

	var routing map[string]any
	if err := json.Unmarshal(tr.Get(routingTreeKey), &routing); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	ribs := routing["ribs"].(map[string]any)["rib"].([]any)
	if len(ribs) != 2 {
		t.Fatalf("expected 2 ribs, got %d", len(ribs))
	}
}
