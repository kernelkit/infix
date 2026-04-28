package zapiwatcher

import (
	"encoding/json"
	"net"
	"testing"
	"time"

	"github.com/kernelkit/infix/src/yangerd/internal/tree"
	"github.com/kernelkit/infix/src/yangerd/internal/zapi"
)

func TestRouteProtocol(t *testing.T) {
	tests := []struct {
		name     string
		rt       zapi.RouteType
		expected string
	}{
		{"kernel", zapi.RouteKernel, "infix-routing:kernel"},
		{"connect", zapi.RouteConnect, "ietf-routing:direct"},
		{"static", zapi.RouteStatic, "ietf-routing:static"},
		{"ospf", zapi.RouteOSPF, "ietf-ospf:ospfv2"},
		{"rip", zapi.RouteRIP, "ietf-rip:rip"},
		{"unknown defaults to kernel", zapi.RouteType(99), "infix-routing:kernel"},
		{"zero defaults to kernel", zapi.RouteType(0), "infix-routing:kernel"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := routeProtocol(tc.rt)
			if got != tc.expected {
				t.Errorf("routeProtocol(%d) = %q, want %q", tc.rt, got, tc.expected)
			}
		})
	}
}

func TestRibName(t *testing.T) {
	tests := []struct {
		name     string
		prefix   net.IPNet
		expected string
	}{
		{
			"ipv4",
			net.IPNet{IP: net.ParseIP("10.0.0.0").To4(), Mask: net.CIDRMask(24, 32)},
			"ipv4",
		},
		{
			"ipv6",
			net.IPNet{IP: net.ParseIP("2001:db8::"), Mask: net.CIDRMask(64, 128)},
			"ipv6",
		},
		{
			"loopback v4",
			net.IPNet{IP: net.ParseIP("127.0.0.0").To4(), Mask: net.CIDRMask(8, 32)},
			"ipv4",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := ribName(tc.prefix)
			if got != tc.expected {
				t.Errorf("ribName(%v) = %q, want %q", tc.prefix, got, tc.expected)
			}
		})
	}
}

func TestTransformRoute(t *testing.T) {
	route := &zapi.Route{
		Type:     zapi.RouteStatic,
		Distance: 1,
		Metric:   100,
		Prefix: net.IPNet{
			IP:   net.ParseIP("10.0.0.0").To4(),
			Mask: net.CIDRMask(24, 32),
		},
	}

	received := time.Date(2025, 6, 15, 10, 30, 0, 0, time.UTC)
	result := transformRoute(route, false, received)

	var parsed map[string]any
	if err := json.Unmarshal(result, &parsed); err != nil {
		t.Fatalf("unmarshal transformRoute result: %v", err)
	}

	if got := parsed["ietf-ipv4-unicast-routing:destination-prefix"]; got != "10.0.0.0/24" {
		t.Errorf("destination-prefix = %v, want %q", got, "10.0.0.0/24")
	}

	if got := parsed["source-protocol"]; got != "ietf-routing:static" {
		t.Errorf("source-protocol = %v, want %q", got, "ietf-routing:static")
	}

	if got, ok := parsed["route-preference"].(float64); !ok || int(got) != 1 {
		t.Errorf("route-preference = %v, want 1 (admin distance)", parsed["route-preference"])
	}

	nhContainer, ok := parsed["next-hop"].(map[string]any)
	if !ok {
		t.Fatalf("next-hop not a map: %T", parsed["next-hop"])
	}
	nhList, ok := nhContainer["next-hop-list"].(map[string]any)
	if !ok {
		t.Fatalf("next-hop-list not a map: %T", nhContainer["next-hop-list"])
	}
	hops, ok := nhList["next-hop"].([]any)
	if !ok {
		t.Fatalf("next-hop not an array: %T", nhList["next-hop"])
	}
	if len(hops) != 0 {
		t.Errorf("expected 0 next-hops, got %d", len(hops))
	}

	if _, ok := parsed["active"]; ok {
		t.Error("non-selected route should not have 'active' leaf")
	}

	lastUpdated, ok := parsed["last-updated"].(string)
	if !ok {
		t.Fatal("last-updated missing or not a string")
	}
	if lastUpdated != "2025-06-15T10:30:00Z" {
		t.Errorf("last-updated = %q, want %q", lastUpdated, "2025-06-15T10:30:00Z")
	}
}

func TestTransformRouteActiveParam(t *testing.T) {
	route := &zapi.Route{
		Type:     zapi.RouteStatic,
		Distance: 5,
		Prefix: net.IPNet{
			IP:   net.ParseIP("0.0.0.0").To4(),
			Mask: net.CIDRMask(0, 32),
		},
	}

	result := transformRoute(route, true, time.Now())

	var parsed map[string]any
	if err := json.Unmarshal(result, &parsed); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	active, ok := parsed["active"].([]any)
	if !ok {
		t.Fatalf("active leaf missing or wrong type: %v", parsed["active"])
	}
	if len(active) != 1 || active[0] != nil {
		t.Errorf("active = %v, want [null]", active)
	}
}

func TestRouteKeyDifferentProtocols(t *testing.T) {
	ospf := &zapi.Route{
		Type:     zapi.RouteOSPF,
		Distance: 110,
		Prefix: net.IPNet{
			IP:   net.IPv4(10, 0, 0, 0),
			Mask: net.CIDRMask(24, 32),
		},
		Nexthops: []zapi.Nexthop{{
			Gate: net.ParseIP("192.168.10.3").To4(),
		}},
	}
	static := &zapi.Route{
		Type:     zapi.RouteStatic,
		Distance: 120,
		Prefix: net.IPNet{
			IP:   net.IPv4(10, 0, 0, 0),
			Mask: net.CIDRMask(24, 32),
		},
		Nexthops: []zapi.Nexthop{{
			Gate: net.ParseIP("192.168.50.2").To4(),
		}},
	}

	keyOSPF := routeKey(ospf)
	keyStatic := routeKey(static)

	if keyOSPF == keyStatic {
		t.Errorf("routes with different protocols must have different keys: %q == %q", keyOSPF, keyStatic)
	}
}

func TestRouteKeySamePrefixProtoIgnoresDistance(t *testing.T) {
	old := &zapi.Route{
		Type:     zapi.RouteStatic,
		Distance: 120,
		Prefix: net.IPNet{
			IP:   net.IPv4(0, 0, 0, 0),
			Mask: net.CIDRMask(0, 32),
		},
	}
	updated := &zapi.Route{
		Type:     zapi.RouteStatic,
		Distance: 1,
		Prefix: net.IPNet{
			IP:   net.IPv4(0, 0, 0, 0),
			Mask: net.CIDRMask(0, 32),
		},
	}

	if routeKey(old) != routeKey(updated) {
		t.Errorf("same prefix+proto must produce same key regardless of distance: %q vs %q", routeKey(old), routeKey(updated))
	}
}

func TestAddRouteSamePrefixDifferentProtocol(t *testing.T) {
	tr := tree.New()
	w := New(tr, nil)

	ospfRoute := &zapi.Route{
		Type:     zapi.RouteOSPF,
		Distance: 110,
		Prefix: net.IPNet{
			IP:   net.IPv4(0, 0, 0, 0),
			Mask: net.CIDRMask(0, 32),
		},
		Nexthops: []zapi.Nexthop{{
			Type: zapi.NHIPv4IFIndex,
			Gate: net.ParseIP("192.168.10.3").To4(),
		}},
	}
	staticRoute := &zapi.Route{
		Type:     zapi.RouteStatic,
		Distance: 120,
		Prefix: net.IPNet{
			IP:   net.IPv4(0, 0, 0, 0),
			Mask: net.CIDRMask(0, 32),
		},
		Nexthops: []zapi.Nexthop{{
			Type: zapi.NHIPv4IFIndex,
			Gate: net.ParseIP("192.168.50.2").To4(),
		}},
	}

	w.addRoute(ospfRoute)
	w.addRoute(staticRoute)

	w.mu.Lock()
	count := len(w.routes)
	w.mu.Unlock()

	if count != 2 {
		t.Errorf("expected 2 routes (different protocols), got %d", count)
	}

	data := tr.Get(routingTreeKey)
	if data == nil {
		t.Fatal("routing tree key not set")
	}

	var routing map[string]any
	if err := json.Unmarshal(data, &routing); err != nil {
		t.Fatalf("unmarshal routing: %v", err)
	}

	ribs, ok := routing["ribs"].(map[string]any)
	if !ok {
		t.Fatal("ribs not found")
	}
	ribList, ok := ribs["rib"].([]any)
	if !ok {
		t.Fatal("rib list not found")
	}

	var ipv4Routes []any
	for _, rib := range ribList {
		ribMap := rib.(map[string]any)
		if ribMap["name"] == "ipv4" {
			routes := ribMap["routes"].(map[string]any)
			ipv4Routes = routes["route"].([]any)
			break
		}
	}

	if len(ipv4Routes) != 2 {
		t.Fatalf("expected 2 IPv4 routes, got %d", len(ipv4Routes))
	}

	var activeCount int
	for _, r := range ipv4Routes {
		rm := r.(map[string]any)
		pref := int(rm["route-preference"].(float64))
		_, hasActive := rm["active"]
		if hasActive {
			activeCount++
			if pref != 110 {
				t.Errorf("active route has preference %d, want 110", pref)
			}
		}
		if pref == 120 && hasActive {
			t.Error("route with preference 120 should not be active")
		}
		if _, ok := rm["last-updated"].(string); !ok {
			t.Errorf("route with preference %d missing last-updated", pref)
		}
	}
	if activeCount != 1 {
		t.Errorf("expected exactly 1 active route, got %d", activeCount)
	}
}

func TestTransformRouteIPv6(t *testing.T) {
	route := &zapi.Route{
		Type:     zapi.RouteOSPF,
		Distance: 110,
		Metric:   10,
		Prefix: net.IPNet{
			IP:   net.ParseIP("2001:db8::").To16(),
			Mask: net.CIDRMask(48, 128),
		},
	}

	result := transformRoute(route, false, time.Now())

	var parsed map[string]any
	if err := json.Unmarshal(result, &parsed); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if got := parsed["ietf-ipv6-unicast-routing:destination-prefix"]; got != "2001:db8::/48" {
		t.Errorf("destination-prefix = %v, want %q", got, "2001:db8::/48")
	}
	if got := parsed["source-protocol"]; got != "ietf-ospf:ospfv2" {
		t.Errorf("source-protocol = %v, want %q", got, "ietf-ospf:ospfv2")
	}
}

func TestAddRouteSamePrefixProtoOverwrites(t *testing.T) {
	tr := tree.New()
	w := New(tr, nil)

	gateway := net.ParseIP("192.168.50.2").To4()
	prefix := net.IPNet{IP: net.IPv4(0, 0, 0, 0), Mask: net.CIDRMask(0, 32)}

	w.addRoute(&zapi.Route{
		Type:     zapi.RouteStatic,
		Distance: 120,
		Prefix:   prefix,
		Nexthops: []zapi.Nexthop{{Type: zapi.NHIPv4IFIndex, Gate: gateway}},
	})

	newGateway := net.ParseIP("10.0.0.1").To4()
	w.addRoute(&zapi.Route{
		Type:     zapi.RouteStatic,
		Distance: 1,
		Prefix:   prefix,
		Nexthops: []zapi.Nexthop{{Type: zapi.NHIPv4IFIndex, Gate: newGateway}},
	})

	w.mu.Lock()
	count := len(w.routes)
	w.mu.Unlock()

	if count != 1 {
		t.Errorf("same prefix+proto should overwrite: got %d routes, want 1", count)
	}

	data := tr.Get(routingTreeKey)
	if data == nil {
		t.Fatal("routing tree key not set")
	}

	var routing map[string]any
	if err := json.Unmarshal(data, &routing); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	ribs := routing["ribs"].(map[string]any)
	ribList := ribs["rib"].([]any)
	var ipv4Routes []any
	for _, rib := range ribList {
		ribMap := rib.(map[string]any)
		if ribMap["name"] == "ipv4" {
			ipv4Routes = ribMap["routes"].(map[string]any)["route"].([]any)
			break
		}
	}

	if len(ipv4Routes) != 1 {
		t.Fatalf("expected 1 route, got %d", len(ipv4Routes))
	}

	rm := ipv4Routes[0].(map[string]any)
	if pref := int(rm["route-preference"].(float64)); pref != 1 {
		t.Errorf("route-preference = %d, want 1 (latest add wins)", pref)
	}
}

func TestDeleteRouteWithoutNexthops(t *testing.T) {
	tr := tree.New()
	w := New(tr, nil)

	gateway := net.ParseIP("192.168.10.3").To4()
	prefix := net.IPNet{IP: net.IPv4(0, 0, 0, 0), Mask: net.CIDRMask(0, 32)}

	w.addRoute(&zapi.Route{
		Type:     zapi.RouteStatic,
		Distance: 5,
		Prefix:   prefix,
		Nexthops: []zapi.Nexthop{{Type: zapi.NHIPv4IFIndex, Gate: gateway}},
	})

	w.mu.Lock()
	count := len(w.routes)
	w.mu.Unlock()
	if count != 1 {
		t.Fatalf("expected 1 route after add, got %d", count)
	}

	w.deleteRoute(&zapi.Route{
		Type:   zapi.RouteStatic,
		Prefix: prefix,
	})

	w.mu.Lock()
	count = len(w.routes)
	w.mu.Unlock()
	if count != 0 {
		t.Errorf("expected 0 routes after delete without nexthops, got %d", count)
	}
}
