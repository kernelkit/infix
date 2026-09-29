package monitor

import (
	"encoding/json"
	"github.com/kernelkit/infix/src/yangerd/internal/tree"
	"log/slog"
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

func TestMergeAugments(t *testing.T) {
	doc := json.RawMessage(`{"interface":[{"name":"eth0","type":"infix-if-type:ethernet"},{"name":"br0","type":"infix-if-type:bridge"}]}`)

	eth := map[string]json.RawMessage{
		"eth0": json.RawMessage(`{"ethernet":{"speed":"1.000","duplex":"full"},"speed":"1000000000"}`),
	}
	fdb := map[string]json.RawMessage{
		"br0": json.RawMessage(`[{"mac":"00:11:22:33:44:55"}]`),
	}

	got := mergeAugments(doc, eth, nil, fdb, nil, nil, nil, nil)

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
	got := mergeAugments(doc, nil, nil, nil, nil, nil, nil, nil)
	if string(got) != string(doc) {
		t.Fatalf("expected no-op, got %s", string(got))
	}
}

func TestMergeAugmentsInvalidDoc(t *testing.T) {
	doc := json.RawMessage(`{invalid`)
	eth := map[string]json.RawMessage{"eth0": json.RawMessage(`{}`)}
	got := mergeAugments(doc, eth, nil, nil, nil, nil, nil, nil)
	if string(got) != string(doc) {
		t.Fatalf("expected passthrough on invalid doc, got %s", string(got))
	}
}

func TestTreeKey(t *testing.T) {
	if treeKey != "ietf-interfaces:interfaces" {
		t.Fatalf("treeKey = %q, want %q", treeKey, "ietf-interfaces:interfaces")
	}
}

func TestTransformMDB(t *testing.T) {
	// transformMDB receives the output of filterByMDBBridge: a flat array of entries
	raw := json.RawMessage(`[{"dev":"br0","port":"e3","grp":"224.1.1.1","state":"temp"},{"dev":"br0","port":"e4","grp":"224.1.1.1","state":"permanent"},{"dev":"br0","port":"e3","grp":"ff02::6a","state":"temp"}]`)

	result := transformMDB(raw)
	if result == nil {
		t.Fatal("expected non-nil result")
	}

	filters, ok := result["multicast-filter"].([]map[string]any)
	if !ok {
		t.Fatalf("unexpected type: %T", result["multicast-filter"])
	}
	if len(filters) != 2 {
		t.Fatalf("expected 2 filters, got %d", len(filters))
	}

	if filters[0]["group"] != "224.1.1.1" {
		t.Fatalf("unexpected group: %v", filters[0]["group"])
	}

	out, _ := json.Marshal(result)
	if !json.Valid(out) {
		t.Fatalf("invalid JSON: %s", out)
	}
}

func TestTransformMDBEmpty(t *testing.T) {
	if transformMDB(json.RawMessage(`[]`)) != nil {
		t.Fatal("expected nil for empty")
	}
	if transformMDB(json.RawMessage(`[{"dev":"br0","port":"br0","grp":"ff02::6a","state":"temp"}]`)) == nil {
		t.Fatal("expected non-nil for router-only entry")
	}
}

func TestSTPFingerprintDeterministic(t *testing.T) {
	br1 := map[string]json.RawMessage{
		"br0": json.RawMessage(`{"root-id":"1.000.00:a0:85:00:01:00"}`),
		"br1": json.RawMessage(`{"root-id":"2.000.00:a0:85:00:02:00"}`),
	}
	br2 := map[string]json.RawMessage{
		"br1": json.RawMessage(`{"root-id":"2.000.00:a0:85:00:02:00"}`),
		"br0": json.RawMessage(`{"root-id":"1.000.00:a0:85:00:01:00"}`),
	}
	pt1 := map[string]json.RawMessage{"e1": json.RawMessage(`{"state":"forwarding"}`)}
	pt2 := map[string]json.RawMessage{"e1": json.RawMessage(`{"state":"forwarding"}`)}

	if stpFingerprint(br1, pt1) != stpFingerprint(br2, pt2) {
		t.Fatal("fingerprint must be independent of map iteration order")
	}

	br2["br0"] = json.RawMessage(`{"root-id":"8.000.00:a0:85:00:01:00"}`)
	if stpFingerprint(br1, pt1) == stpFingerprint(br2, pt2) {
		t.Fatal("fingerprint must change when STP data changes")
	}
}

func names(t *testing.T, raw json.RawMessage) []string {
	t.Helper()
	var out []string
	for _, row := range decodeRows(raw) {
		out = append(out, rowString(row, "ifname"))
	}
	return out
}

// Rows are keyed by ifindex: a rename replaces the old row instead of
// leaving it behind, and a later delete leaves nothing.
func TestReplaceRowsByIfindexRenameThenDelete(t *testing.T) {
	links := json.RawMessage(`[{"ifindex":1,"ifname":"lo"},{"ifindex":5,"ifname":"foo"}]`)

	if got := nameByIndex(links, 5); got != "foo" {
		t.Fatalf("nameByIndex = %q, want foo", got)
	}

	links = replaceRows(links, "ifindex", 5, json.RawMessage(`[{"ifindex":5,"ifname":"bar"}]`))
	if got := names(t, links); !reflect.DeepEqual(got, []string{"lo", "bar"}) {
		t.Fatalf("after rename = %v, want [lo bar]", got)
	}

	links = replaceRows(links, "ifindex", 5, nil)
	if got := names(t, links); !reflect.DeepEqual(got, []string{"lo"}) {
		t.Fatalf("after delete = %v, want [lo]", got)
	}

	if got := string(replaceRows(links, "ifindex", 1, nil)); got != "[]" {
		t.Fatalf("empty result = %s, want []", got)
	}
}

func TestReplaceRowsByDev(t *testing.T) {
	neighs := json.RawMessage(`[{"dst":"10.0.0.1","dev":"eth0"},{"dst":"10.0.0.2","dev":"eth1"}]`)
	fresh := withField(json.RawMessage(`[{"dst":"10.0.0.9"}]`), "dev", "eth0")
	got := replaceRows(neighs, "dev", "eth0", fresh)

	var rows []map[string]string
	if err := json.Unmarshal(got, &rows); err != nil {
		t.Fatal(err)
	}
	want := []map[string]string{{"dst": "10.0.0.2", "dev": "eth1"}, {"dst": "10.0.0.9", "dev": "eth0"}}
	if !reflect.DeepEqual(rows, want) {
		t.Fatalf("got %v, want %v", rows, want)
	}
}

// A bridge whose last group left has no entry, so its filters go away.
func TestMDBByBridge(t *testing.T) {
	raw := json.RawMessage(`[{"mdb":[{"dev":"br0","port":"e1","grp":"239.1.1.1","state":"temp"},` +
		`{"dev":"br1","port":"e2","grp":"239.2.2.2","state":"permanent"}],"router":{}}]`)
	got := mdbByBridge(raw)
	if len(got) != 2 || got["br0"] == nil || got["br1"] == nil {
		t.Fatalf("got %v", got)
	}
	if got := mdbByBridge(json.RawMessage(`[{"mdb":[],"router":{}}]`)); len(got) != 0 {
		t.Fatalf("empty dump gave %v", got)
	}
}

func TestSetWireguardAllClearsAndSkipsUnchanged(t *testing.T) {
	m := New(nil, nil, nil, nil, tree.New(), nil, slog.Default())
	peer := map[string]json.RawMessage{"wg0": json.RawMessage(`{"peer-status":{}}`)}

	m.SetWireguardAll(peer)
	if m.wireguard["wg0"] == nil {
		t.Fatal("wg0 not staged")
	}
	before, _ := m.tree.Info(treeKey)
	m.SetWireguardAll(peer)
	if after, _ := m.tree.Info(treeKey); !after.LastUpdated.Equal(before.LastUpdated) {
		t.Fatal("unchanged WireGuard data rebuilt the document")
	}

	m.SetWireguardAll(nil)
	if len(m.wireguard) != 0 {
		t.Fatalf("wireguard staging not cleared: %v", m.wireguard)
	}
}
