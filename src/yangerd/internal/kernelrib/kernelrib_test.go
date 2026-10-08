package kernelrib

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net"
	"testing"
	"time"

	"github.com/vishvananda/netlink"
	"golang.org/x/sys/unix"

	"github.com/kernelkit/infix/src/yangerd/internal/tree"
)

func cidr(t *testing.T, s string) *net.IPNet {
	t.Helper()
	_, n, err := net.ParseCIDR(s)
	if err != nil {
		t.Fatal(err)
	}
	return n
}

func fakeIfName(index int) string {
	switch index {
	case 2:
		return "e0"
	case 3:
		return "e1"
	}
	return ""
}

func newWatcher(t *testing.T, v4, v6 []netlink.Route, err error) (*Watcher, *tree.Tree) {
	t.Helper()
	tr := tree.New()
	w := New(tr, slog.New(slog.NewTextHandler(testWriter{t}, nil)))
	w.ifname = fakeIfName
	w.list = func(family int) ([]netlink.Route, error) {
		if err != nil {
			return nil, err
		}
		if family == netlink.FAMILY_V6 {
			return v6, nil
		}
		return v4, nil
	}
	return w, tr
}

type testWriter struct{ t *testing.T }

func (w testWriter) Write(p []byte) (int, error) { w.t.Log(string(p)); return len(p), nil }

func ribRoutes(t *testing.T, tr *tree.Tree, name string) []map[string]any {
	t.Helper()
	data := tr.Get(routingTreeKey)
	if data == nil {
		t.Fatal("routing tree key not set")
	}
	var routing map[string]any
	if err := json.Unmarshal(data, &routing); err != nil {
		t.Fatalf("unmarshal routing: %v", err)
	}
	for _, rib := range routing["ribs"].(map[string]any)["rib"].([]any) {
		rm := rib.(map[string]any)
		if rm["name"] != name {
			continue
		}
		out := []map[string]any{}
		for _, r := range rm["routes"].(map[string]any)["route"].([]any) {
			out = append(out, r.(map[string]any))
		}
		return out
	}
	t.Fatalf("rib %s missing", name)
	return nil
}

func findRoute(routes []map[string]any, family, dst string) map[string]any {
	for _, r := range routes {
		if r["ietf-"+family+"-unicast-routing:destination-prefix"] == dst {
			return r
		}
	}
	return nil
}

func hops(r map[string]any) []map[string]any {
	nh := r["next-hop"].(map[string]any)
	list := nh["next-hop-list"].(map[string]any)["next-hop"].([]any)
	out := make([]map[string]any, 0, len(list))
	for _, h := range list {
		out = append(out, h.(map[string]any))
	}
	return out
}

func TestStaticAndConnectedRoutes(t *testing.T) {
	v4 := []netlink.Route{
		{Dst: nil, Gw: net.ParseIP("192.168.1.1"), LinkIndex: 2, Protocol: unix.RTPROT_STATIC, Priority: 5},
		{Dst: cidr(t, "192.168.1.0/24"), LinkIndex: 2, Protocol: unix.RTPROT_KERNEL, Scope: unix.RT_SCOPE_LINK},
		{Dst: cidr(t, "10.0.0.0/8"), LinkIndex: 3, Protocol: unix.RTPROT_BOOT, Priority: 100},
	}
	w, tr := newWatcher(t, v4, nil, nil)
	w.writeRibs()

	routes := ribRoutes(t, tr, "ipv4")
	if len(routes) != 3 {
		t.Fatalf("want 3 routes, got %d", len(routes))
	}

	def := findRoute(routes, "ipv4", "0.0.0.0/0")
	if def == nil {
		t.Fatal("default route missing")
	}
	if def["source-protocol"] != "ietf-routing:static" {
		t.Errorf("default source-protocol = %v", def["source-protocol"])
	}
	if def["route-preference"] != float64(5) {
		t.Errorf("default route-preference = %v", def["route-preference"])
	}
	if _, ok := def["active"]; !ok {
		t.Error("default route not active")
	}
	h := hops(def)
	if len(h) != 1 || h[0]["ietf-ipv4-unicast-routing:address"] != "192.168.1.1" {
		t.Errorf("default next-hop = %v", h)
	}
	if _, ok := h[0]["infix-routing:installed"]; !ok {
		t.Error("default next-hop not installed")
	}
	if _, ok := h[0]["outgoing-interface"]; ok {
		t.Error("gateway hop must not also carry the interface")
	}

	conn := findRoute(routes, "ipv4", "192.168.1.0/24")
	if conn["source-protocol"] != "ietf-routing:direct" {
		t.Errorf("connected source-protocol = %v", conn["source-protocol"])
	}
	if h := hops(conn); len(h) != 1 || h[0]["outgoing-interface"] != "e0" {
		t.Errorf("connected next-hop = %v", h)
	}

	boot := findRoute(routes, "ipv4", "10.0.0.0/8")
	if boot["source-protocol"] != "infix-routing:kernel" {
		t.Errorf("boot source-protocol = %v", boot["source-protocol"])
	}

	if v6 := ribRoutes(t, tr, "ipv6"); len(v6) != 0 {
		t.Errorf("want empty ipv6 rib, got %v", v6)
	}
}

func TestLowestMetricIsActive(t *testing.T) {
	v4 := []netlink.Route{
		{Gw: net.ParseIP("192.168.1.1"), LinkIndex: 2, Protocol: unix.RTPROT_STATIC, Priority: 120},
		{Gw: net.ParseIP("192.168.2.1"), LinkIndex: 3, Protocol: unix.RTPROT_STATIC, Priority: 5},
	}
	w, tr := newWatcher(t, v4, nil, nil)
	w.writeRibs()

	active := 0
	for _, r := range ribRoutes(t, tr, "ipv4") {
		_, isActive := r["active"]
		if isActive {
			active++
			if r["route-preference"] != float64(5) {
				t.Errorf("active route has preference %v, want 5", r["route-preference"])
			}
		}
	}
	if active != 1 {
		t.Errorf("want exactly one active default route, got %d", active)
	}
}

func TestSpecialAndMultipathNextHops(t *testing.T) {
	v4 := []netlink.Route{
		{Dst: cidr(t, "10.1.0.0/16"), Type: unix.RTN_BLACKHOLE, Protocol: unix.RTPROT_STATIC},
		{Dst: cidr(t, "10.2.0.0/16"), Type: unix.RTN_UNREACHABLE, Protocol: unix.RTPROT_STATIC},
		{Dst: cidr(t, "10.3.0.0/16"), Type: unix.RTN_PROHIBIT, Protocol: unix.RTPROT_STATIC},
		{Dst: cidr(t, "10.4.0.0/16"), Protocol: unix.RTPROT_STATIC, MultiPath: []*netlink.NexthopInfo{
			{Gw: net.ParseIP("192.168.1.1"), LinkIndex: 2},
			{LinkIndex: 3},
		}},
	}
	w, tr := newWatcher(t, v4, nil, nil)
	w.writeRibs()
	routes := ribRoutes(t, tr, "ipv4")

	for dst, want := range map[string]string{
		"10.1.0.0/16": "blackhole",
		"10.2.0.0/16": "unreachable",
		"10.3.0.0/16": "prohibit",
	} {
		r := findRoute(routes, "ipv4", dst)
		if got := r["next-hop"].(map[string]any)["special-next-hop"]; got != want {
			t.Errorf("%s special-next-hop = %v, want %s", dst, got, want)
		}
	}

	h := hops(findRoute(routes, "ipv4", "10.4.0.0/16"))
	if len(h) != 2 {
		t.Fatalf("want 2 multipath hops, got %v", h)
	}
	if h[0]["ietf-ipv4-unicast-routing:address"] != "192.168.1.1" || h[1]["outgoing-interface"] != "e1" {
		t.Errorf("multipath hops = %v", h)
	}
}

func TestIPv6Routes(t *testing.T) {
	v6 := []netlink.Route{
		{Gw: net.ParseIP("fe80::1"), LinkIndex: 2, Protocol: unix.RTPROT_STATIC, Priority: 1},
		{Dst: cidr(t, "2001:db8::/64"), LinkIndex: 2, Protocol: unix.RTPROT_KERNEL, Priority: 256},
	}
	w, tr := newWatcher(t, nil, v6, nil)
	w.writeRibs()
	routes := ribRoutes(t, tr, "ipv6")

	def := findRoute(routes, "ipv6", "::/0")
	if def == nil {
		t.Fatal("ipv6 default route missing")
	}
	if h := hops(def); h[0]["ietf-ipv6-unicast-routing:address"] != "fe80::1" {
		t.Errorf("ipv6 default next-hop = %v", h)
	}
	if conn := findRoute(routes, "ipv6", "2001:db8::/64"); conn["source-protocol"] != "ietf-routing:direct" {
		t.Errorf("ipv6 connected source-protocol = %v", conn["source-protocol"])
	}
}

func TestReadErrorKeepsPreviousData(t *testing.T) {
	v4 := []netlink.Route{{Gw: net.ParseIP("192.168.1.1"), LinkIndex: 2, Protocol: unix.RTPROT_STATIC}}
	w, tr := newWatcher(t, v4, nil, nil)
	w.writeRibs()

	w.list = func(int) ([]netlink.Route, error) { return nil, errors.New("netlink down") }
	w.writeRibs()

	if routes := ribRoutes(t, tr, "ipv4"); len(routes) != 1 {
		t.Errorf("want previous route kept, got %v", routes)
	}
}

func TestLastUpdatedSurvivesRefresh(t *testing.T) {
	v4 := []netlink.Route{{Gw: net.ParseIP("192.168.1.1"), LinkIndex: 2, Protocol: unix.RTPROT_STATIC}}
	w, tr := newWatcher(t, v4, nil, nil)

	t0 := time.Date(2026, 10, 5, 8, 0, 0, 0, time.UTC)
	w.now = func() time.Time { return t0 }
	w.writeRibs()
	first := ribRoutes(t, tr, "ipv4")[0]["last-updated"]

	w.now = func() time.Time { return t0.Add(time.Hour) }
	w.writeRibs()
	if again := ribRoutes(t, tr, "ipv4")[0]["last-updated"]; again != first {
		t.Errorf("last-updated changed on refresh: %v -> %v", first, again)
	}

	// A changed next-hop is a new route and gets a new timestamp.
	w.list = func(int) ([]netlink.Route, error) {
		return []netlink.Route{{Gw: net.ParseIP("192.168.1.2"), LinkIndex: 2, Protocol: unix.RTPROT_STATIC}}, nil
	}
	w.writeRibs()
	if changed := ribRoutes(t, tr, "ipv4")[0]["last-updated"]; changed == first {
		t.Error("last-updated not refreshed for a replaced route")
	}
}

func TestUnknownInterfaceHopIsDropped(t *testing.T) {
	v4 := []netlink.Route{{Dst: cidr(t, "10.0.0.0/8"), LinkIndex: 99, Protocol: unix.RTPROT_KERNEL}}
	w, tr := newWatcher(t, v4, nil, nil)
	w.writeRibs()

	r := ribRoutes(t, tr, "ipv4")[0]
	if _, ok := r["next-hop"]; ok {
		t.Errorf("want no next-hop for unresolvable interface, got %v", r["next-hop"])
	}
}
