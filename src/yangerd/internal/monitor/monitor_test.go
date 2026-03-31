package monitor

import (
	"encoding/json"
	"reflect"
	"syscall"
	"testing"

	"github.com/vishvananda/netlink"
)

func TestExtractOperStatus(t *testing.T) {
	tests := []struct {
		name   string
		raw    json.RawMessage
		want   string
		wantOK bool
	}{
		{
			name:   "valid single entry",
			raw:    json.RawMessage(`[{"operstate":"UP"}]`),
			want:   "UP",
			wantOK: true,
		},
		{
			name:   "multiple entries first wins",
			raw:    json.RawMessage(`[{"operstate":"DOWN"},{"operstate":"UP"}]`),
			want:   "DOWN",
			wantOK: true,
		},
		{
			name:   "missing operstate",
			raw:    json.RawMessage(`[{"ifname":"eth0"}]`),
			want:   "",
			wantOK: false,
		},
		{
			name:   "empty array",
			raw:    json.RawMessage(`[]`),
			want:   "",
			wantOK: false,
		},
		{
			name:   "invalid json",
			raw:    json.RawMessage(`{`),
			want:   "",
			wantOK: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := extractOperStatus(tt.raw)
			if got != tt.want || ok != tt.wantOK {
				t.Fatalf("extractOperStatus() = (%q, %v), want (%q, %v)", got, ok, tt.want, tt.wantOK)
			}
		})
	}
}

func TestInterfaceNames(t *testing.T) {
	tests := []struct {
		name string
		raw  json.RawMessage
		want []string
	}{
		{
			name: "deduplicate and keep order",
			raw: json.RawMessage(`[
				{"ifname":"eth0"},
				{"ifname":"eth1"},
				{"ifname":"eth0"},
				{"x":"y"}
			]`),
			want: []string{"eth0", "eth1"},
		},
		{
			name: "empty array",
			raw:  json.RawMessage(`[]`),
			want: []string{},
		},
		{
			name: "objects without ifname skipped",
			raw:  json.RawMessage(`[{"name":"eth0"},{"foo":"bar"}]`),
			want: []string{},
		},
		{
			name: "invalid json",
			raw:  json.RawMessage(`{`),
			want: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := interfaceNames(tt.raw)
			if !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("interfaceNames() = %#v, want %#v", got, tt.want)
			}
		})
	}
}

func TestFilterByIfName(t *testing.T) {
	tests := []struct {
		name      string
		raw       json.RawMessage
		ifname    string
		wantCount int
		wantEmpty bool
	}{
		{
			name: "filters correctly",
			raw: json.RawMessage(`[
				{"ifname":"eth0","x":1},
				{"ifname":"eth1","x":2},
				{"ifname":"eth0","x":3}
			]`),
			ifname:    "eth0",
			wantCount: 2,
		},
		{
			name:      "no matches returns empty array",
			raw:       json.RawMessage(`[{"ifname":"eth1"}]`),
			ifname:    "eth0",
			wantCount: 0,
			wantEmpty: true,
		},
		{
			name:      "invalid json",
			raw:       json.RawMessage(`{`),
			ifname:    "eth0",
			wantCount: 0,
			wantEmpty: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := filterByIfName(tt.raw, tt.ifname)

			var rows []map[string]json.RawMessage
			if err := json.Unmarshal(got, &rows); err != nil {
				t.Fatalf("unmarshal filtered rows: %v", err)
			}
			if len(rows) != tt.wantCount {
				t.Fatalf("row count = %d, want %d", len(rows), tt.wantCount)
			}
			if tt.wantEmpty && string(got) != "[]" {
				t.Fatalf("expected [] got %s", string(got))
			}
		})
	}
}

func TestBridgeNames(t *testing.T) {
	raw := json.RawMessage(`[
		{"br":"br0"},
		{"br":"br1"},
		{"br":"br0"},
		{"ifname":"eth0"}
	]`)

	got := bridgeNames(raw)
	want := []string{"br0", "br1"}

	if !reflect.DeepEqual(got, want) {
		t.Fatalf("bridgeNames() = %#v, want %#v", got, want)
	}
}

func TestFilterByBridge(t *testing.T) {
	tests := []struct {
		name      string
		raw       json.RawMessage
		bridge    string
		wantCount int
		wantEmpty bool
	}{
		{
			name: "filters correctly",
			raw: json.RawMessage(`[
				{"br":"br0","grp":"239.1.1.1"},
				{"br":"br1","grp":"239.1.1.2"},
				{"br":"br0","grp":"239.1.1.3"}
			]`),
			bridge:    "br0",
			wantCount: 2,
		},
		{
			name:      "no matches returns empty array",
			raw:       json.RawMessage(`[{"br":"br1"}]`),
			bridge:    "br0",
			wantCount: 0,
			wantEmpty: true,
		},
		{
			name:      "invalid json",
			raw:       json.RawMessage(`{`),
			bridge:    "br0",
			wantCount: 0,
			wantEmpty: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := filterByBridge(tt.raw, tt.bridge)

			var rows []map[string]json.RawMessage
			if err := json.Unmarshal(got, &rows); err != nil {
				t.Fatalf("unmarshal filtered rows: %v", err)
			}
			if len(rows) != tt.wantCount {
				t.Fatalf("row count = %d, want %d", len(rows), tt.wantCount)
			}
			if tt.wantEmpty && string(got) != "[]" {
				t.Fatalf("expected [] got %s", string(got))
			}
		})
	}
}

func TestIsBridgeFDB(t *testing.T) {
	tests := []struct {
		name   string
		update netlink.NeighUpdate
		want   bool
	}{
		{
			name:   "bridge family",
			update: netlink.NeighUpdate{Neigh: netlink.Neigh{Family: syscall.AF_BRIDGE}},
			want:   true,
		},
		{
			name:   "master index set",
			update: netlink.NeighUpdate{Neigh: netlink.Neigh{MasterIndex: 10}},
			want:   true,
		},
		{
			name:   "master flag set",
			update: netlink.NeighUpdate{Neigh: netlink.Neigh{Flags: netlink.NTF_MASTER}},
			want:   true,
		},
		{
			name:   "non-bridge",
			update: netlink.NeighUpdate{},
			want:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isBridgeFDB(tt.update); got != tt.want {
				t.Fatalf("isBridgeFDB() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestReplaceByIfName(t *testing.T) {
	tests := []struct {
		name     string
		bulk     json.RawMessage
		ifname   string
		perIface json.RawMessage
		want     int
	}{
		{
			name:     "replace existing",
			bulk:     json.RawMessage(`[{"ifname":"eth0","x":1},{"ifname":"eth1","x":2}]`),
			ifname:   "eth0",
			perIface: json.RawMessage(`[{"ifname":"eth0","x":99}]`),
			want:     2,
		},
		{
			name:     "add new",
			bulk:     json.RawMessage(`[{"ifname":"eth0","x":1}]`),
			ifname:   "eth1",
			perIface: json.RawMessage(`[{"ifname":"eth1","x":2}]`),
			want:     2,
		},
		{
			name:     "empty bulk",
			bulk:     json.RawMessage(`[]`),
			ifname:   "eth0",
			perIface: json.RawMessage(`[{"ifname":"eth0","x":1}]`),
			want:     1,
		},
		{
			name:     "invalid bulk",
			bulk:     json.RawMessage(`{`),
			ifname:   "eth0",
			perIface: json.RawMessage(`[{"ifname":"eth0","x":1}]`),
			want:     1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := replaceByIfName(tt.bulk, tt.ifname, tt.perIface)
			var rows []json.RawMessage
			if err := json.Unmarshal(got, &rows); err != nil {
				t.Fatalf("unmarshal: %v", err)
			}
			if len(rows) != tt.want {
				t.Fatalf("row count = %d, want %d (raw: %s)", len(rows), tt.want, string(got))
			}
		})
	}
}

func TestReplaceByIfNamePreservesUpdatedData(t *testing.T) {
	bulk := json.RawMessage(`[{"ifname":"eth0","x":1},{"ifname":"eth1","x":2}]`)
	updated := replaceByIfName(bulk, "eth0", json.RawMessage(`[{"ifname":"eth0","x":99}]`))

	var rows []map[string]json.RawMessage
	if err := json.Unmarshal(updated, &rows); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	for _, row := range rows {
		var name string
		json.Unmarshal(row["ifname"], &name)
		if name == "eth0" {
			var x int
			json.Unmarshal(row["x"], &x)
			if x != 99 {
				t.Fatalf("eth0.x = %d, want 99", x)
			}
			return
		}
	}
	t.Fatal("eth0 not found in result")
}

func TestMergeAugments(t *testing.T) {
	doc := json.RawMessage(`{"interface":[{"name":"eth0","type":"infix-if-type:ethernet"},{"name":"br0","type":"infix-if-type:bridge"}]}`)

	eth := map[string]json.RawMessage{
		"eth0": json.RawMessage(`{"speed":1000,"duplex":"full"}`),
	}
	fdb := map[string]json.RawMessage{
		"br0": json.RawMessage(`[{"mac":"00:11:22:33:44:55"}]`),
	}

	got := mergeAugments(doc, eth, nil, fdb, nil)

	var root map[string]any
	if err := json.Unmarshal(got, &root); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	ifaces := root["interface"].([]any)
	eth0 := ifaces[0].(map[string]any)
	if _, ok := eth0["ieee802-ethernet-interface:ethernet"]; !ok {
		t.Fatal("ethernet augment not merged into eth0")
	}

	br0 := ifaces[1].(map[string]any)
	bridge, ok := br0["infix-interfaces:bridge"]
	if !ok {
		t.Fatal("bridge augment not created for br0")
	}
	bridgeMap := bridge.(map[string]any)
	if _, ok := bridgeMap["fdb"]; !ok {
		t.Fatal("fdb not merged into bridge augment")
	}
}

func TestMergeAugmentsNoOp(t *testing.T) {
	doc := json.RawMessage(`{"interface":[{"name":"lo"}]}`)
	got := mergeAugments(doc, nil, nil, nil, nil)
	if string(got) != string(doc) {
		t.Fatalf("expected no-op, got %s", string(got))
	}
}

func TestMergeAugmentsInvalidDoc(t *testing.T) {
	doc := json.RawMessage(`{invalid`)
	eth := map[string]json.RawMessage{"eth0": json.RawMessage(`{}`)}
	got := mergeAugments(doc, eth, nil, nil, nil)
	if string(got) != string(doc) {
		t.Fatalf("expected passthrough on invalid doc, got %s", string(got))
	}
}

func TestTreeKey(t *testing.T) {
	if treeKey != "ietf-interfaces:interfaces" {
		t.Fatalf("treeKey = %q, want %q", treeKey, "ietf-interfaces:interfaces")
	}
}
