package zapiwatcher

import (
	"encoding/json"
	"net"
	"testing"

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

	result := transformRoute(route)

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

	result := transformRoute(route)

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
