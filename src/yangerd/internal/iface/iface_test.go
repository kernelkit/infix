package iface

import (
	"encoding/json"
	"errors"
	"testing"
)

type mockFileChecker struct {
	exists  map[string]bool
	files   map[string]string
	readErr map[string]error
}

func (m *mockFileChecker) Exists(path string) bool {
	if m == nil || m.exists == nil {
		return false
	}
	return m.exists[path]
}

func (m *mockFileChecker) ReadFile(path string) (string, error) {
	if m == nil {
		return "", errors.New("nil file checker")
	}
	if err, ok := m.readErr[path]; ok {
		return "", err
	}
	if v, ok := m.files[path]; ok {
		return v, nil
	}
	return "", errors.New("not found")
}

func mustRaw(t *testing.T, v any) json.RawMessage {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	return b
}

func mustInterfaces(t *testing.T, raw json.RawMessage) []map[string]any {
	t.Helper()

	var root map[string]any
	if err := json.Unmarshal(raw, &root); err != nil {
		t.Fatalf("unmarshal transform output: %v", err)
	}

	arr, ok := root["interface"].([]any)
	if !ok {
		t.Fatalf("missing interface list: %#v", root)
	}

	out := make([]map[string]any, 0, len(arr))
	for _, v := range arr {
		m, ok := v.(map[string]any)
		if !ok {
			t.Fatalf("interface entry not object: %T", v)
		}
		out = append(out, m)
	}

	return out
}

func mustIfaceByName(t *testing.T, ifaces []map[string]any, name string) map[string]any {
	t.Helper()
	for _, iface := range ifaces {
		if iface["name"] == name {
			return iface
		}
	}
	t.Fatalf("interface %q not found", name)
	return nil
}

func TestTransformEmptyInputs(t *testing.T) {
	tests := []struct {
		name     string
		linkData json.RawMessage
		addrData json.RawMessage
		stats    json.RawMessage
	}{
		{name: "nil raw messages"},
		{
			name:     "empty arrays",
			linkData: mustRaw(t, []map[string]any{}),
			addrData: mustRaw(t, []map[string]any{}),
			stats:    mustRaw(t, []map[string]any{}),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ifaces := mustInterfaces(t, Transform(tt.linkData, tt.addrData, tt.stats, nil))
			if len(ifaces) != 0 {
				t.Fatalf("expected empty interface list, got %d", len(ifaces))
			}
		})
	}
}

func TestTransformSingleLoopback(t *testing.T) {
	link := []map[string]any{{
		"ifindex":    1,
		"ifname":     "lo",
		"flags":      []any{"LOOPBACK", "UP"},
		"link_type":  "loopback",
		"operstate":  "UNKNOWN",
		"address":    "00:00:00:00:00:00",
		"statistics": map[string]any{},
	}}

	ifaces := mustInterfaces(t, Transform(mustRaw(t, link), nil, nil, nil))
	if len(ifaces) != 1 {
		t.Fatalf("expected 1 interface, got %d", len(ifaces))
	}

	lo := ifaces[0]
	if lo["name"] != "lo" {
		t.Fatalf("name = %v", lo["name"])
	}
	if lo["type"] != "infix-if-type:loopback" {
		t.Fatalf("type = %v", lo["type"])
	}
	if lo["admin-status"] != "up" || lo["oper-status"] != "unknown" {
		t.Fatalf("admin/oper mismatch: %v/%v", lo["admin-status"], lo["oper-status"])
	}
}

func TestTransformSingleEthernetWithIPv4IPv6(t *testing.T) {
	link := []map[string]any{{
		"ifindex":   2,
		"ifname":    "eth0",
		"flags":     []any{"UP"},
		"link_type": "ether",
		"operstate": "UP",
		"address":   "52:54:00:12:34:56",
	}}

	addr := []map[string]any{{
		"ifname": "eth0",
		"mtu":    1500,
		"addr_info": []map[string]any{
			{"family": "inet", "local": "192.0.2.10", "prefixlen": 24, "protocol": "static"},
			{"family": "inet6", "local": "2001:db8::10", "prefixlen": 64, "protocol": "kernel_ra"},
		},
	}}

	fc := &mockFileChecker{files: map[string]string{"/proc/sys/net/ipv6/conf/eth0/mtu": "1400\n"}}

	ifaces := mustInterfaces(t, Transform(mustRaw(t, link), mustRaw(t, addr), nil, fc))
	eth0 := mustIfaceByName(t, ifaces, "eth0")

	if eth0["type"] != "infix-if-type:ethernet" {
		t.Fatalf("unexpected type: %v", eth0["type"])
	}

	ipv4, ok := eth0["ietf-ip:ipv4"].(map[string]any)
	if !ok {
		t.Fatalf("missing ipv4 container: %#v", eth0)
	}
	if ipv4["mtu"] != float64(1500) {
		t.Fatalf("ipv4 mtu = %v", ipv4["mtu"])
	}
	v4addrs := ipv4["address"].([]any)
	v4 := v4addrs[0].(map[string]any)
	if v4["ip"] != "192.0.2.10" || v4["prefix-length"] != float64(24) || v4["origin"] != "static" {
		t.Fatalf("unexpected ipv4 address entry: %#v", v4)
	}

	ipv6, ok := eth0["ietf-ip:ipv6"].(map[string]any)
	if !ok {
		t.Fatalf("missing ipv6 container: %#v", eth0)
	}
	if ipv6["mtu"] != float64(1400) {
		t.Fatalf("ipv6 mtu = %v", ipv6["mtu"])
	}
	v6addrs := ipv6["address"].([]any)
	v6 := v6addrs[0].(map[string]any)
	if v6["ip"] != "2001:db8::10" || v6["prefix-length"] != float64(64) || v6["origin"] != "link-layer" {
		t.Fatalf("unexpected ipv6 address entry: %#v", v6)
	}
}

func TestTransformStatisticsCountersAsStrings(t *testing.T) {
	link := []map[string]any{{
		"ifindex":   3,
		"ifname":    "eth1",
		"flags":     []any{"UP"},
		"link_type": "ether",
		"operstate": "UP",
	}}

	stats := []map[string]any{{
		"ifname": "eth1",
		"stats64": map[string]any{
			"rx": map[string]any{"bytes": uint64(1234567890)},
			"tx": map[string]any{"bytes": uint64(9876543210)},
		},
	}}

	ifaces := mustInterfaces(t, Transform(mustRaw(t, link), nil, mustRaw(t, stats), nil))
	eth1 := mustIfaceByName(t, ifaces, "eth1")
	st, ok := eth1["statistics"].(map[string]any)
	if !ok {
		t.Fatalf("missing statistics: %#v", eth1)
	}

	if _, ok := st["in-octets"].(string); !ok {
		t.Fatalf("in-octets must be string, got %T", st["in-octets"])
	}
	if _, ok := st["out-octets"].(string); !ok {
		t.Fatalf("out-octets must be string, got %T", st["out-octets"])
	}
}

func TestTransformVLANAugment(t *testing.T) {
	link := []map[string]any{{
		"ifindex":   10,
		"ifname":    "eth0.100",
		"flags":     []any{"UP"},
		"link_type": "none",
		"operstate": "UP",
		"link":      "eth0",
		"linkinfo": map[string]any{
			"info_kind": "vlan",
			"info_data": map[string]any{"protocol": "802.1Q", "id": 100},
		},
	}}

	ifaces := mustInterfaces(t, Transform(mustRaw(t, link), nil, nil, nil))
	vlan := mustIfaceByName(t, ifaces, "eth0.100")

	if vlan["type"] != "infix-if-type:vlan" {
		t.Fatalf("type = %v", vlan["type"])
	}
	v, ok := vlan["infix-interfaces:vlan"].(map[string]any)
	if !ok {
		t.Fatalf("missing vlan augment: %#v", vlan)
	}
	if v["tag-type"] != "ieee802-dot1q-types:c-vlan" || v["id"] != float64(100) || v["lower-layer-if"] != "eth0" {
		t.Fatalf("unexpected vlan augment: %#v", v)
	}
}

func TestTransformVethAugment(t *testing.T) {
	link := []map[string]any{{
		"ifname":    "veth0",
		"ifindex":   11,
		"flags":     []any{"UP"},
		"link_type": "none",
		"operstate": "UP",
		"link":      "veth1",
		"linkinfo":  map[string]any{"info_kind": "veth"},
	}}

	ifaces := mustInterfaces(t, Transform(mustRaw(t, link), nil, nil, nil))
	veth := mustIfaceByName(t, ifaces, "veth0")
	v, ok := veth["infix-interfaces:veth"].(map[string]any)
	if !ok || v["peer"] != "veth1" {
		t.Fatalf("unexpected veth augment: %#v", veth)
	}
}

func TestTransformGREAndVXLANAugments(t *testing.T) {
	link := []map[string]any{
		{
			"ifname":    "gre1",
			"ifindex":   12,
			"flags":     []any{"UP"},
			"link_type": "gre",
			"operstate": "UP",
			"linkinfo": map[string]any{
				"info_data": map[string]any{"local": "192.0.2.1", "remote": "198.51.100.1"},
			},
		},
		{
			"ifname":    "vxlan10",
			"ifindex":   13,
			"flags":     []any{"UP"},
			"link_type": "none",
			"operstate": "UP",
			"linkinfo": map[string]any{
				"info_kind": "vxlan",
				"info_data": map[string]any{"local": "10.0.0.1", "remote": "10.0.0.2", "id": 10},
			},
		},
	}

	ifaces := mustInterfaces(t, Transform(mustRaw(t, link), nil, nil, nil))

	gre := mustIfaceByName(t, ifaces, "gre1")
	if gre["type"] != "infix-if-type:gre" {
		t.Fatalf("gre type = %v", gre["type"])
	}
	g, ok := gre["infix-interfaces:gre"].(map[string]any)
	if !ok || g["local"] != "192.0.2.1" || g["remote"] != "198.51.100.1" {
		t.Fatalf("unexpected gre augment: %#v", g)
	}

	vx := mustIfaceByName(t, ifaces, "vxlan10")
	if vx["type"] != "infix-if-type:vxlan" {
		t.Fatalf("vxlan type = %v", vx["type"])
	}
	v, ok := vx["infix-interfaces:vxlan"].(map[string]any)
	if !ok || v["local"] != "10.0.0.1" || v["remote"] != "10.0.0.2" || v["vni"] != float64(10) {
		t.Fatalf("unexpected vxlan augment: %#v", v)
	}
}

func TestTransformLAGAugmentModes(t *testing.T) {
	link := []map[string]any{
		{
			"ifname":    "bond0",
			"ifindex":   20,
			"flags":     []any{"UP"},
			"link_type": "none",
			"operstate": "UP",
			"linkinfo": map[string]any{
				"info_kind": "bond",
				"info_data": map[string]any{
					"mode":              "802.3ad",
					"updelay":           10,
					"downdelay":         20,
					"ad_lacp_active":    "on",
					"ad_lacp_rate":      "fast",
					"xmit_hash_policy":  "layer3+4",
					"ad_actor_sys_prio": 100,
					"ad_info": map[string]any{
						"aggregator":  7,
						"actor_key":   1000,
						"partner_key": 2000,
						"partner_mac": "02:00:00:00:00:01",
					},
				},
			},
		},
		{
			"ifname":    "bond1",
			"ifindex":   21,
			"flags":     []any{"UP"},
			"link_type": "none",
			"operstate": "UP",
			"linkinfo": map[string]any{
				"info_kind": "bond",
				"info_data": map[string]any{
					"mode":             "balance-xor",
					"xmit_hash_policy": "layer2",
				},
			},
		},
	}

	ifaces := mustInterfaces(t, Transform(mustRaw(t, link), nil, nil, nil))

	bond0 := mustIfaceByName(t, ifaces, "bond0")
	b0 := bond0["infix-interfaces:lag"].(map[string]any)
	if b0["mode"] != "lacp" {
		t.Fatalf("bond0 mode = %v", b0["mode"])
	}
	lacp := b0["lacp"].(map[string]any)
	if lacp["mode"] != "active" || lacp["rate"] != "fast" || lacp["hash"] != "layer3-4" {
		t.Fatalf("unexpected bond0 lacp: %#v", lacp)
	}

	bond1 := mustIfaceByName(t, ifaces, "bond1")
	b1 := bond1["infix-interfaces:lag"].(map[string]any)
	if b1["mode"] != "static" {
		t.Fatalf("bond1 mode = %v", b1["mode"])
	}
	static := b1["static"].(map[string]any)
	if static["mode"] != "balance-xor" || static["hash"] != "layer2" {
		t.Fatalf("unexpected bond1 static: %#v", static)
	}
}

func TestTransformBridgePortLowerLayer(t *testing.T) {
	link := []map[string]any{{
		"ifname":    "eth2",
		"ifindex":   30,
		"flags":     []any{"UP"},
		"link_type": "ether",
		"operstate": "UP",
		"master":    "br0",
		"linkinfo": map[string]any{
			"info_slave_kind": "bridge",
			"info_slave_data": map[string]any{
				"bcast_flood":      true,
				"flood":            false,
				"mcast_flood":      true,
				"fastleave":        true,
				"multicast_router": 2,
			},
		},
	}}

	ifaces := mustInterfaces(t, Transform(mustRaw(t, link), nil, nil, nil))
	eth2 := mustIfaceByName(t, ifaces, "eth2")
	lower := eth2["infix-interfaces:bridge-port"].(map[string]any)
	if lower["bridge"] != "br0" {
		t.Fatalf("bridge lower bridge = %v", lower["bridge"])
	}
	mcast := lower["multicast"].(map[string]any)
	if mcast["router"] != "permanent" {
		t.Fatalf("bridge router mode = %v", mcast["router"])
	}
}

func TestTransformLagPortLowerLayer(t *testing.T) {
	link := []map[string]any{{
		"ifname":    "eth3",
		"ifindex":   31,
		"flags":     []any{"UP"},
		"link_type": "ether",
		"operstate": "UP",
		"master":    "bond0",
		"linkinfo": map[string]any{
			"info_slave_kind": "bond",
			"info_slave_data": map[string]any{
				"state":                          "ACTIVE",
				"link_failure_count":             5,
				"ad_aggregator_id":               42,
				"ad_actor_oper_port_state_str":   "collecting_distributing",
				"ad_partner_oper_port_state_str": "collecting_distributing",
			},
		},
	}}

	ifaces := mustInterfaces(t, Transform(mustRaw(t, link), nil, nil, nil))
	eth3 := mustIfaceByName(t, ifaces, "eth3")
	lower := eth3["infix-interfaces:lag-port"].(map[string]any)
	if lower["lag"] != "bond0" || lower["state"] != "active" || lower["link-failures"] != float64(5) {
		t.Fatalf("unexpected lag-port lower-layer: %#v", lower)
	}
	lacp := lower["lacp"].(map[string]any)
	if lacp["aggregator-id"] != float64(42) {
		t.Fatalf("lag-port lacp aggregator-id = %v", lacp["aggregator-id"])
	}
}

func TestTransformFilteredInterfaces(t *testing.T) {
	link := []map[string]any{
		{"ifname": "dummy0", "group": "internal", "link_type": "none"},
		{"ifname": "can0", "link_type": "can"},
		{"ifname": "vcan0", "link_type": "vcan"},
		{"ifname": "eth9", "ifindex": 99, "flags": []any{"UP"}, "link_type": "ether", "operstate": "UP"},
	}

	ifaces := mustInterfaces(t, Transform(mustRaw(t, link), nil, nil, nil))
	if len(ifaces) != 1 {
		t.Fatalf("expected only one surviving interface, got %d", len(ifaces))
	}
	if ifaces[0]["name"] != "eth9" {
		t.Fatalf("surviving interface = %v", ifaces[0]["name"])
	}
}

func TestTransformWiFiType(t *testing.T) {
	link := []map[string]any{{
		"ifname":    "wlan0",
		"ifindex":   40,
		"flags":     []any{"UP"},
		"link_type": "ether",
		"operstate": "UP",
	}}

	fc := &mockFileChecker{exists: map[string]bool{"/sys/class/net/wlan0/wireless/": true}}
	ifaces := mustInterfaces(t, Transform(mustRaw(t, link), nil, nil, fc))
	wlan0 := mustIfaceByName(t, ifaces, "wlan0")
	if wlan0["type"] != "infix-if-type:wifi" {
		t.Fatalf("wlan0 type = %v", wlan0["type"])
	}
}

func TestIplink2yangTypeMappings(t *testing.T) {
	fc := &mockFileChecker{exists: map[string]bool{"/sys/class/net/wlan0/wireless/": true}}

	tests := []struct {
		name   string
		iplink map[string]any
		want   string
	}{
		{name: "loopback", iplink: map[string]any{"ifname": "lo", "link_type": "loopback"}, want: "infix-if-type:loopback"},
		{name: "gre", iplink: map[string]any{"ifname": "gre0", "link_type": "gre"}, want: "infix-if-type:gre"},
		{name: "gre6", iplink: map[string]any{"ifname": "gre6", "link_type": "gre6"}, want: "infix-if-type:gre"},
		{name: "wifi via ether", iplink: map[string]any{"ifname": "wlan0", "link_type": "ether"}, want: "infix-if-type:wifi"},
		{name: "bond", iplink: map[string]any{"ifname": "bond0", "link_type": "none", "linkinfo": map[string]any{"info_kind": "bond"}}, want: "infix-if-type:lag"},
		{name: "bridge", iplink: map[string]any{"ifname": "br0", "link_type": "none", "linkinfo": map[string]any{"info_kind": "bridge"}}, want: "infix-if-type:bridge"},
		{name: "dummy", iplink: map[string]any{"ifname": "dummy0", "link_type": "none", "linkinfo": map[string]any{"info_kind": "dummy"}}, want: "infix-if-type:dummy"},
		{name: "gretap", iplink: map[string]any{"ifname": "gretap0", "link_type": "none", "linkinfo": map[string]any{"info_kind": "gretap"}}, want: "infix-if-type:gretap"},
		{name: "vxlan", iplink: map[string]any{"ifname": "vxlan10", "link_type": "none", "linkinfo": map[string]any{"info_kind": "vxlan"}}, want: "infix-if-type:vxlan"},
		{name: "veth", iplink: map[string]any{"ifname": "veth0", "link_type": "none", "linkinfo": map[string]any{"info_kind": "veth"}}, want: "infix-if-type:veth"},
		{name: "vlan", iplink: map[string]any{"ifname": "eth0.10", "link_type": "none", "linkinfo": map[string]any{"info_kind": "vlan"}}, want: "infix-if-type:vlan"},
		{name: "wireguard", iplink: map[string]any{"ifname": "wg0", "link_type": "none", "linkinfo": map[string]any{"info_kind": "wireguard"}}, want: "infix-if-type:wireguard"},
		{name: "default ethernet", iplink: map[string]any{"ifname": "eth0", "link_type": "none", "linkinfo": map[string]any{"info_kind": "unknown"}}, want: "infix-if-type:ethernet"},
		{name: "unknown link type", iplink: map[string]any{"ifname": "x", "link_type": "strange"}, want: "infix-if-type:other"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := iplink2yangType(tt.iplink, fc)
			if got != tt.want {
				t.Fatalf("iplink2yangType() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestIplink2yangOperstateMappings(t *testing.T) {
	tests := []struct {
		in   string
		want string
	}{
		{"DOWN", "down"},
		{"UP", "up"},
		{"DORMANT", "dormant"},
		{"TESTING", "testing"},
		{"LOWERLAYERDOWN", "lower-layer-down"},
		{"NOTPRESENT", "not-present"},
		{"WHATEVER", "unknown"},
	}

	for _, tt := range tests {
		if got := iplink2yangOperstate(tt.in); got != tt.want {
			t.Fatalf("iplink2yangOperstate(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestSkipInterface(t *testing.T) {
	tests := []struct {
		name   string
		iplink map[string]any
		want   bool
	}{
		{name: "internal group", iplink: map[string]any{"group": "internal"}, want: true},
		{name: "can", iplink: map[string]any{"link_type": "can"}, want: true},
		{name: "vcan", iplink: map[string]any{"link_type": "vcan"}, want: true},
		{name: "normal", iplink: map[string]any{"link_type": "ether"}, want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := skipInterface(tt.iplink); got != tt.want {
				t.Fatalf("skipInterface() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestInet2yangOrigin(t *testing.T) {
	tests := []struct {
		name string
		inet map[string]any
		want string
	}{
		{name: "kernel_ll", inet: map[string]any{"protocol": "kernel_ll"}, want: "link-layer"},
		{name: "kernel_ra", inet: map[string]any{"protocol": "kernel_ra"}, want: "link-layer"},
		{name: "stable privacy kernel_ll", inet: map[string]any{"protocol": "kernel_ll", "stable-privacy": true}, want: "random"},
		{name: "static", inet: map[string]any{"protocol": "static"}, want: "static"},
		{name: "dhcp", inet: map[string]any{"protocol": "dhcp"}, want: "dhcp"},
		{name: "random", inet: map[string]any{"protocol": "random"}, want: "random"},
		{name: "other", inet: map[string]any{"protocol": "kernel_lo"}, want: "other"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := inet2yangOrigin(tt.inet); got != tt.want {
				t.Fatalf("inet2yangOrigin() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestProto2yang(t *testing.T) {
	tests := []struct {
		in   string
		want string
	}{
		{"802.1Q", "ieee802-dot1q-types:c-vlan"},
		{"802.1ad", "ieee802-dot1q-types:s-vlan"},
		{"something", "other"},
	}

	for _, tt := range tests {
		if got := proto2yang(tt.in); got != tt.want {
			t.Fatalf("proto2yang(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestLagMode(t *testing.T) {
	tests := []struct {
		in   string
		want string
	}{
		{"802.3ad", "lacp"},
		{"balance-xor", "static"},
		{"active-backup", "static"},
	}

	for _, tt := range tests {
		if got := lagMode(tt.in); got != tt.want {
			t.Fatalf("lagMode(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestLagHash(t *testing.T) {
	tests := []struct {
		in   string
		want string
	}{
		{"layer2", "layer2"},
		{"layer3+4", "layer3-4"},
		{"layer2+3", "layer2-3"},
		{"encap2+3", "encap2-3"},
		{"encap3+4", "encap3-4"},
		{"vlan+srcmac", "vlan-srcmac"},
		{"something-else", "layer2"},
	}

	for _, tt := range tests {
		if got := lagHash(tt.in); got != tt.want {
			t.Fatalf("lagHash(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestBridgeRouterMode(t *testing.T) {
	tests := []struct {
		in   int
		want string
	}{
		{0, "off"},
		{1, "auto"},
		{2, "permanent"},
		{9, "UNKNOWN"},
	}

	for _, tt := range tests {
		if got := bridgeRouterMode(tt.in); got != tt.want {
			t.Fatalf("bridgeRouterMode(%d) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestStatistics(t *testing.T) {
	t.Run("with stats64", func(t *testing.T) {
		st := statistics(map[string]any{
			"stats64": map[string]any{
				"rx": map[string]any{"bytes": json.Number("123")},
				"tx": map[string]any{"bytes": uint64(456)},
			},
		})
		if st["in-octets"] != "123" || st["out-octets"] != "456" {
			t.Fatalf("unexpected statistics map: %#v", st)
		}
	})

	t.Run("without stats64", func(t *testing.T) {
		st := statistics(map[string]any{})
		if len(st) != 0 {
			t.Fatalf("expected empty stats, got %#v", st)
		}
	})
}

func TestToCounterString(t *testing.T) {
	tests := []struct {
		name string
		in   any
		want string
	}{
		{name: "int", in: int(7), want: "7"},
		{name: "int64", in: int64(8), want: "8"},
		{name: "uint64", in: uint64(9), want: "9"},
		{name: "float64", in: float64(10.9), want: "10"},
		{name: "json number", in: json.Number("11"), want: "11"},
		{name: "string", in: " 12 ", want: "12"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := toCounterString(tt.in); got != tt.want {
				t.Fatalf("toCounterString(%v) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

func TestAddressesFamilyFilter(t *testing.T) {
	ipaddr := map[string]any{
		"addr_info": []any{
			map[string]any{"family": "inet", "local": "192.0.2.1", "prefixlen": 24, "protocol": "dhcp"},
			map[string]any{"family": "inet6", "local": "2001:db8::1", "prefixlen": 64, "protocol": "kernel_ra"},
		},
	}

	v4 := addresses(ipaddr, "inet")
	if len(v4) != 1 || v4[0]["ip"] != "192.0.2.1" || v4[0]["prefix-length"] != 24 || v4[0]["origin"] != "dhcp" {
		t.Fatalf("unexpected inet addresses: %#v", v4)
	}

	v6 := addresses(ipaddr, "inet6")
	if len(v6) != 1 || v6[0]["ip"] != "2001:db8::1" || v6[0]["prefix-length"] != 64 || v6[0]["origin"] != "link-layer" {
		t.Fatalf("unexpected inet6 addresses: %#v", v6)
	}
}

func TestIPv4Data(t *testing.T) {
	t.Run("with mtu and addresses", func(t *testing.T) {
		in := map[string]any{
			"ifname": "eth0",
			"mtu":    1500,
			"addr_info": []any{
				map[string]any{"family": "inet", "local": "10.0.0.1", "prefixlen": 24, "protocol": "static"},
			},
		}
		out := ipv4Data(in)
		if out["mtu"] != 1500 {
			t.Fatalf("unexpected mtu: %#v", out)
		}
		if _, ok := out["address"]; !ok {
			t.Fatalf("missing address list: %#v", out)
		}
	})

	t.Run("without mtu", func(t *testing.T) {
		in := map[string]any{
			"ifname": "eth0",
			"addr_info": []any{
				map[string]any{"family": "inet", "local": "10.0.0.2", "prefixlen": 24, "protocol": "static"},
			},
		}
		out := ipv4Data(in)
		if _, ok := out["mtu"]; ok {
			t.Fatalf("did not expect mtu in %#v", out)
		}
	})

	t.Run("loopback omits mtu", func(t *testing.T) {
		in := map[string]any{"ifname": "lo", "mtu": 65536}
		out := ipv4Data(in)
		if _, ok := out["mtu"]; ok {
			t.Fatalf("loopback must not include mtu: %#v", out)
		}
	})
}

func TestIPv6Data(t *testing.T) {
	t.Run("with mtu and addresses", func(t *testing.T) {
		in := map[string]any{
			"ifname": "eth0",
			"addr_info": []any{
				map[string]any{"family": "inet6", "local": "2001:db8::1", "prefixlen": 64, "protocol": "static"},
			},
		}
		fc := &mockFileChecker{files: map[string]string{"/proc/sys/net/ipv6/conf/eth0/mtu": "1280\n"}}
		out := ipv6Data(in, fc)
		if out["mtu"] != 1280 {
			t.Fatalf("unexpected mtu: %#v", out)
		}
		if _, ok := out["address"]; !ok {
			t.Fatalf("missing address list: %#v", out)
		}
	})

	t.Run("without mtu from filechecker", func(t *testing.T) {
		in := map[string]any{"ifname": "eth1"}
		fc := &mockFileChecker{readErr: map[string]error{"/proc/sys/net/ipv6/conf/eth1/mtu": errors.New("no file")}}
		out := ipv6Data(in, fc)
		if _, ok := out["mtu"]; ok {
			t.Fatalf("did not expect mtu in %#v", out)
		}
	})

	t.Run("without addresses", func(t *testing.T) {
		in := map[string]any{"ifname": "eth2"}
		out := ipv6Data(in, nil)
		if len(out) != 0 {
			t.Fatalf("expected empty ipv6 map, got %#v", out)
		}
	})
}

func TestDedupByIfindex(t *testing.T) {
	t.Run("keeps UP over DOWN for same ifindex", func(t *testing.T) {
		link := []map[string]any{
			{"ifindex": 2, "ifname": "eth0", "flags": []any{}, "link_type": "ether", "operstate": "DOWN", "address": "02:00:00:00:00:01"},
			{"ifindex": 2, "ifname": "e1", "flags": []any{"UP"}, "link_type": "ether", "operstate": "UP", "address": "02:00:00:00:00:01"},
		}
		ifaces := mustInterfaces(t, Transform(mustRaw(t, link), nil, nil, nil))
		if len(ifaces) != 1 {
			t.Fatalf("expected 1 interface after dedup, got %d", len(ifaces))
		}
		if ifaces[0]["name"] != "e1" {
			t.Fatalf("expected e1 to survive dedup, got %v", ifaces[0]["name"])
		}
	})

	t.Run("keeps first when both DOWN", func(t *testing.T) {
		link := []map[string]any{
			{"ifindex": 3, "ifname": "a0", "flags": []any{}, "link_type": "ether", "operstate": "DOWN"},
			{"ifindex": 3, "ifname": "a1", "flags": []any{}, "link_type": "ether", "operstate": "DOWN"},
		}
		ifaces := mustInterfaces(t, Transform(mustRaw(t, link), nil, nil, nil))
		if len(ifaces) != 1 {
			t.Fatalf("expected 1 interface after dedup, got %d", len(ifaces))
		}
		if ifaces[0]["name"] != "a0" {
			t.Fatalf("expected a0 to survive dedup, got %v", ifaces[0]["name"])
		}
	})

	t.Run("different ifindex not deduped", func(t *testing.T) {
		link := []map[string]any{
			{"ifindex": 1, "ifname": "lo", "flags": []any{"LOOPBACK", "UP"}, "link_type": "loopback", "operstate": "UNKNOWN"},
			{"ifindex": 2, "ifname": "e1", "flags": []any{"UP"}, "link_type": "ether", "operstate": "UP"},
		}
		ifaces := mustInterfaces(t, Transform(mustRaw(t, link), nil, nil, nil))
		if len(ifaces) != 2 {
			t.Fatalf("expected 2 interfaces, got %d", len(ifaces))
		}
	})

	t.Run("zero ifindex entries kept as-is", func(t *testing.T) {
		link := []map[string]any{
			{"ifname": "x0", "flags": []any{"UP"}, "link_type": "ether", "operstate": "UP"},
			{"ifname": "x1", "flags": []any{"UP"}, "link_type": "ether", "operstate": "UP"},
		}
		ifaces := mustInterfaces(t, Transform(mustRaw(t, link), nil, nil, nil))
		if len(ifaces) != 2 {
			t.Fatalf("expected 2 interfaces (zero ifindex not deduped), got %d", len(ifaces))
		}
	})
}
