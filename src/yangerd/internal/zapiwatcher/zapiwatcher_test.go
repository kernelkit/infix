package zapiwatcher

import (
	"encoding/json"
	"net"
	"testing"

	"github.com/osrg/gobgp/v3/pkg/zebra"
)

func TestRouteProtocol(t *testing.T) {
	tests := []struct {
		name     string
		rt       zebra.RouteType
		expected string
	}{
		{"kernel", RouteKernel, "infix-routing:kernel"},
		{"connect", RouteConnect, "ietf-routing:direct"},
		{"static", RouteStatic, "ietf-routing:static"},
		{"ospf", RouteOSPF, "ietf-ospf:ospfv2"},
		{"rip", RouteRIP, "ietf-rip:ripv2"},
		{"unknown defaults to kernel", zebra.RouteType(99), "infix-routing:kernel"},
		{"zero defaults to kernel", zebra.RouteType(0), "infix-routing:kernel"},
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
			"ipv4-master",
		},
		{
			"ipv6",
			net.IPNet{IP: net.ParseIP("2001:db8::"), Mask: net.CIDRMask(64, 128)},
			"ipv6-master",
		},
		{
			"loopback v4",
			net.IPNet{IP: net.ParseIP("127.0.0.0").To4(), Mask: net.CIDRMask(8, 32)},
			"ipv4-master",
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
	route := &zebra.IPRouteBody{
		Type:   RouteStatic,
		Metric: 100,
	}
	route.Prefix.Prefix = net.ParseIP("10.0.0.0")
	route.Prefix.PrefixLen = 24

	result := transformRoute(route)

	var parsed map[string]any
	if err := json.Unmarshal(result, &parsed); err != nil {
		t.Fatalf("unmarshal transformRoute result: %v", err)
	}

	if got := parsed["destination-prefix"]; got != "10.0.0.0/24" {
		t.Errorf("destination-prefix = %v, want %q", got, "10.0.0.0/24")
	}

	if got := parsed["source-protocol"]; got != "ietf-routing:static" {
		t.Errorf("source-protocol = %v, want %q", got, "ietf-routing:static")
	}

	if got, ok := parsed["metric"].(float64); !ok || int(got) != 100 {
		t.Errorf("metric = %v, want 100", parsed["metric"])
	}

	nhList, ok := parsed["next-hop-list"].(map[string]any)
	if !ok {
		t.Fatalf("next-hop-list not a map: %T", parsed["next-hop-list"])
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
	route := &zebra.IPRouteBody{
		Type:   RouteOSPF,
		Metric: 10,
	}
	route.Prefix.Prefix = net.ParseIP("2001:db8::")
	route.Prefix.PrefixLen = 48

	result := transformRoute(route)

	var parsed map[string]any
	if err := json.Unmarshal(result, &parsed); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if got := parsed["destination-prefix"]; got != "2001:db8::/48" {
		t.Errorf("destination-prefix = %v, want %q", got, "2001:db8::/48")
	}
	if got := parsed["source-protocol"]; got != "ietf-ospf:ospfv2" {
		t.Errorf("source-protocol = %v, want %q", got, "ietf-ospf:ospfv2")
	}
}

func TestIpNetFromPrefix(t *testing.T) {
	tests := []struct {
		name     string
		prefix   zebra.Prefix
		wantCIDR string
		wantOK   bool
	}{
		{
			"valid ipv4",
			zebra.Prefix{Prefix: net.ParseIP("192.168.1.0"), PrefixLen: 24},
			"192.168.1.0/24",
			true,
		},
		{
			"valid ipv6",
			zebra.Prefix{Prefix: net.ParseIP("2001:db8::"), PrefixLen: 64},
			"2001:db8::/64",
			true,
		},
		{
			"host route ipv4",
			zebra.Prefix{Prefix: net.ParseIP("10.0.0.1"), PrefixLen: 32},
			"10.0.0.1/32",
			true,
		},
		{
			"zero prefix length",
			zebra.Prefix{Prefix: net.ParseIP("0.0.0.0"), PrefixLen: 0},
			"0.0.0.0/0",
			true,
		},
		{
			"prefix masking applied",
			zebra.Prefix{Prefix: net.ParseIP("10.0.0.5"), PrefixLen: 24},
			"10.0.0.0/24",
			true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := ipNetFromPrefix(tc.prefix)
			if ok != tc.wantOK {
				t.Fatalf("ipNetFromPrefix ok = %v, want %v", ok, tc.wantOK)
			}
			if !ok {
				return
			}
			if got.String() != tc.wantCIDR {
				t.Errorf("ipNetFromPrefix = %q, want %q", got.String(), tc.wantCIDR)
			}
		})
	}
}
