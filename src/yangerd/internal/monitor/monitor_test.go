package monitor

import (
	"encoding/json"
	"github.com/kernelkit/infix/src/yangerd/internal/iface"
	"github.com/kernelkit/infix/src/yangerd/internal/mcd"
	"github.com/kernelkit/infix/src/yangerd/internal/tree"
	"log/slog"
	"os"
	"reflect"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/vishvananda/netlink"
	"github.com/vishvananda/netlink/nl"
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
	bridges := map[string]iface.BridgeInfo{
		"br0": {Name: "br0", Index: 2, AgingTime: 300, Ports: []string{"eth0"}},
	}

	got := mergeAugments(doc, augments{ethernet: eth, bridges: bridges})

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
	if bridgeMap["aging-time"] != float64(300) {
		t.Fatalf("aging-time = %v, want 300", bridgeMap["aging-time"])
	}
}

func TestMergeAugmentsNoOp(t *testing.T) {
	doc := json.RawMessage(`{"interface":[{"name":"lo"}]}`)
	got := mergeAugments(doc, augments{})
	if string(got) != string(doc) {
		t.Fatalf("expected no-op, got %s", string(got))
	}
}

func TestMergeAugmentsInvalidDoc(t *testing.T) {
	doc := json.RawMessage(`{invalid`)
	eth := map[string]json.RawMessage{"eth0": json.RawMessage(`{}`)}
	got := mergeAugments(doc, augments{ethernet: eth})
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

type noFiles struct{}

func (noFiles) Exists(string) bool              { return false }
func (noFiles) ReadFile(string) (string, error) { return "", os.ErrNotExist }
func (noFiles) ListDir(string) []string         { return nil }

// The startup sweep asks ethtool for ethernet ports only, not for the
// loopback, bridges or VLANs in the same dump.
func TestEthernetNames(t *testing.T) {
	dump := json.RawMessage(`[
		{"ifindex": 1, "ifname": "lo", "link_type": "loopback"},
		{"ifindex": 2, "ifname": "eth0", "link_type": "ether"},
		{"ifindex": 3, "ifname": "br0", "link_type": "ether", "linkinfo": {"info_kind": "bridge"}},
		{"ifindex": 4, "ifname": "eth0.10", "link_type": "ether", "linkinfo": {"info_kind": "vlan"}},
		{"ifindex": 5, "ifname": "eth1", "link_type": "ether"}
	]`)

	got := ethernetNames(dump, noFiles{})
	want := []string{"eth0", "eth1"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("ethernetNames = %v, want %v", got, want)
	}
}

// A delete of the old wifi0 handled after the new wifi0 took the name
// must not drop the new interface's name-keyed state.
func TestLateDeleteKeepsReusedName(t *testing.T) {
	m := New(nil, nil, nil, nil, tree.New(), nil, slog.Default())
	m.links = json.RawMessage(`[{"ifindex":14,"ifname":"wifi0"},{"ifindex":16,"ifname":"wifi0"}]`)
	m.wifi["wifi0"] = json.RawMessage(`{"station":{"ssid":"campus"}}`)

	m.removeInterface(14)

	if m.wifi["wifi0"] == nil {
		t.Fatal("late delete of the old wifi0 dropped the new one's wifi data")
	}
	if got := nameByIndex(m.links, 16); got != "wifi0" {
		t.Fatalf("new wifi0 row lost, links %s", m.links)
	}

	m.removeInterface(16)
	if m.wifi["wifi0"] != nil {
		t.Fatal("deleting the last wifi0 kept its wifi data")
	}
}

func TestLinkRowFor(t *testing.T) {
	for _, tc := range []struct {
		raw  string
		want bool
	}{
		{`[{"ifindex":16,"ifname":"wifi0"}]`, true},
		{`[{}]`, false},
		{`[{"ifindex":14,"ifname":"wifi0"}]`, false},
		{`[{"ifindex":16}]`, false},
		{`[]`, false},
	} {
		if got := linkRowFor(json.RawMessage(tc.raw), 16); got != tc.want {
			t.Errorf("linkRowFor(%s) = %v, want %v", tc.raw, got, tc.want)
		}
	}
}

func TestLastChangeStampsTransitionsOnly(t *testing.T) {
	m := New(nil, nil, nil, nil, tree.New(), nil, slog.Default())
	m.tree.Set(treeKey, json.RawMessage(`{"interface":[{"name":"e1","oper-status":"up"},{"name":"e2","oper-status":"down"}]}`))

	if got := m.OnDemand(); got != nil {
		t.Fatalf("no transitions yet, want nil overlay, got %s", got)
	}

	m.mu.Lock()
	m.lastOperStatus["e1"] = "DOWN"
	m.lastOperStatus["e2"] = "DOWN"
	m.updateOperStatus("e1", json.RawMessage(`[{"ifname":"e1","operstate":"UP"}]`))
	m.updateOperStatus("e2", json.RawMessage(`[{"ifname":"e2","operstate":"DOWN"}]`))
	m.mu.Unlock()

	var out struct {
		Interface []map[string]any `json:"interface"`
	}
	if err := json.Unmarshal(m.OnDemand(), &out); err != nil {
		t.Fatal(err)
	}
	if len(out.Interface) != 2 {
		t.Fatalf("overlay must carry the whole list, got %d entries", len(out.Interface))
	}
	if _, ok := out.Interface[0]["last-change"].(string); !ok {
		t.Errorf("e1 changed state, want last-change, got %v", out.Interface[0])
	}
	if _, ok := out.Interface[1]["last-change"]; ok {
		t.Errorf("e2 kept its state, want no last-change, got %v", out.Interface[1])
	}

	m.mu.Lock()
	m.forgetName("e1")
	m.mu.Unlock()
	if got := m.OnDemand(); got != nil {
		t.Errorf("stamp must go with the interface, got %s", got)
	}
}

func TestOverlayStampsLastChange(t *testing.T) {
	doc := json.RawMessage(`{"interface":[{"name":"e1"},{"name":"e2"}]}`)
	changed := time.Now()
	stamps := map[string]time.Time{"e1": changed}

	out := overlay(doc, stamps, changed.Add(time.Minute), nil, nil, nil)
	want := changed.UTC().Format("2006-01-02T15:04:05+00:00")
	if !strings.Contains(string(out), `{"last-change":"`+want+`","name":"e1"}`) {
		t.Errorf("want %s on e1 in %s", want, out)
	}
	if !strings.Contains(string(out), `{"name":"e2"}`) {
		t.Errorf("e2 must be untouched in %s", out)
	}
	if got := overlay(json.RawMessage(`garbage`), stamps, changed, nil, nil, nil); got != nil {
		t.Errorf("bad doc must yield no overlay, got %s", got)
	}
}

func TestOverlayQueriers(t *testing.T) {
	doc := json.RawMessage(`{"interface":[
	 {"name":"br0","infix-interfaces:bridge":{"vlans":{"vlan":[
	   {"vid":10,"multicast":{"snooping":true}},
	   {"vid":20,"multicast":{"snooping":true}}]}}},
	 {"name":"br0.20","infix-interfaces:vlan":{"id":20,"lower-layer-if":"br0"}},
	 {"name":"br1","infix-interfaces:bridge":{"multicast":{"snooping":false}}},
	 {"name":"e1"}]}`)
	qs := []mcd.Querier{
		{Interface: "br0", Family: "ipv4", VID: 10, State: "up", Address: "10.0.0.1", Interval: 125, Version: 3},
		{Interface: "br0.20", Family: "ipv4", State: "proxy", Address: "0.0.0.0", Interval: 30, Version: 3},
	}

	var root struct {
		Interface []map[string]json.RawMessage `json:"interface"`
	}
	if err := json.Unmarshal(overlay(doc, nil, time.Now(), nil, func() []mcd.Querier { return qs }, nil), &root); err != nil {
		t.Fatal(err)
	}

	want := map[string]string{
		"br0": `{"vlans":{"vlan":[` +
			`{"multicast":{"active-querier":[{"address":"10.0.0.1","family":"ipv4","local":true,"query-interval":125,"version":3}],"querier":"auto","query-interval":125,"snooping":true},"vid":10},` +
			`{"multicast":{"active-querier":[{"address":"0.0.0.0","family":"ipv4","local":true,"query-interval":30,"version":3}],"querier":"proxy","query-interval":30,"snooping":true},"vid":20}]}}`,
		"br1": `{"multicast":{"querier":"off","snooping":false}}`,
	}
	for _, ifc := range root.Interface {
		var name string
		json.Unmarshal(ifc["name"], &name)
		if got, ok := ifc["infix-interfaces:bridge"]; ok && string(got) != want[name] {
			t.Errorf("%s:\n got  %s\n want %s", name, got, want[name])
		}
	}

	out := overlay(doc, nil, time.Now(), nil, nil, nil)
	if strings.Contains(string(out), "querier") {
		t.Errorf("no querier source must leave the bridges alone: %s", out)
	}
}

// sysfsFiles is a FileChecker serving ifalias files from a map.
type sysfsFiles map[string]string

func (f sysfsFiles) Exists(path string) bool { _, ok := f[path]; return ok }
func (f sysfsFiles) ListDir(string) []string { return nil }
func (f sysfsFiles) ReadFile(path string) (string, error) {
	if v, ok := f[path]; ok {
		return v, nil
	}
	return "", os.ErrNotExist
}

// A description set while the interface is down never reaches the link
// row, the provider must take it from sysfs on every read.
func TestOnDemandRefreshesDescriptionFromSysfs(t *testing.T) {
	fc := sysfsFiles{
		"/sys/class/net/wan/ifalias": "Uplink\n",
		"/sys/class/net/e2/ifalias":  "\n",
	}
	m := New(nil, nil, nil, nil, tree.New(), fc, slog.Default())
	m.tree.Set(treeKey, json.RawMessage(`{"interface":[`+
		`{"name":"wan"},`+
		`{"name":"e2","description":"stale"},`+
		`{"name":"eth0","description":"kept"}]}`))

	var out struct {
		Interface []map[string]any `json:"interface"`
	}
	if err := json.Unmarshal(m.OnDemand(), &out); err != nil {
		t.Fatal(err)
	}
	if got := out.Interface[0]["description"]; got != "Uplink" {
		t.Errorf("wan: want description from sysfs, got %v", got)
	}
	if got, ok := out.Interface[1]["description"]; ok {
		t.Errorf("e2: alias cleared in sysfs, want no description, got %v", got)
	}
	if got := out.Interface[2]["description"]; got != "kept" {
		t.Errorf("eth0: unreadable in sysfs, want link row value kept, got %v", got)
	}
}

const vlanBridgeLinks = `[
 {"ifindex":5,"ifname":"br0","operstate":"UP","linkinfo":{"info_kind":"bridge","info_data":{"vlan_filtering":1,"vlan_protocol":"802.1Q","mcast_snooping":1}}},
 {"ifindex":6,"ifname":"br1","operstate":"UP","linkinfo":{"info_kind":"bridge","info_data":{"vlan_filtering":0,"vlan_protocol":"802.1Q","mcast_snooping":0}}},
 {"ifindex":2,"ifname":"e1","operstate":"UP","master":"br0","linkinfo":{"info_slave_kind":"bridge","info_slave_data":{"state":"forwarding","fastleave":false,"multicast_router":1,"bcast_flood":true,"flood":true,"mcast_flood":true}}},
 {"ifindex":3,"ifname":"e2","operstate":"UP","master":"br0","linkinfo":{"info_slave_kind":"bridge","info_slave_data":{"state":"forwarding","fastleave":false,"multicast_router":1,"bcast_flood":true,"flood":true,"mcast_flood":true}}},
 {"ifindex":4,"ifname":"e9","operstate":"UP","master":"br1","linkinfo":{"info_slave_kind":"bridge","info_slave_data":{"state":"forwarding","fastleave":false,"multicast_router":1,"bcast_flood":true,"flood":true,"mcast_flood":true}}}
]`

func vlanAugments(t *testing.T) augments {
	t.Helper()
	return augments{
		bridges: iface.Bridges(json.RawMessage(vlanBridgeLinks)),
		vlans: iface.ParseVLANTable(json.RawMessage(`[
		 {"ifname":"br0","vlans":[{"vlan":1,"flags":["PVID","Egress Untagged"]},{"vlan":10}]},
		 {"ifname":"e1","vlans":[{"vlan":1,"flags":["PVID","Egress Untagged"]},{"vlan":10}]},
		 {"ifname":"e2","vlans":[{"vlan":10,"flags":["PVID"]}]},
		 {"ifname":"br1","vlans":[{"vlan":1,"flags":["PVID","Egress Untagged"]}]},
		 {"ifname":"e9","vlans":[{"vlan":1,"flags":["PVID","Egress Untagged"]}]}]`)),
		vlanGlobals: map[string][]iface.VLANGlobal{
			"br0": iface.ParseVLANGlobals(json.RawMessage(`[{"ifname":"br0","vlans":[{"vlan":1,"mcast_snooping":1},{"vlan":10,"mcast_snooping":1}]}]`)),
		},
		mdb: map[string]json.RawMessage{
			"br0": json.RawMessage(`[{"dev":"br0","port":"e1","grp":"224.1.1.1","vid":10,"state":"temp"},{"dev":"br0","port":"e2","grp":"224.1.1.1","vid":10,"state":"temp"},{"dev":"br0","port":"e1","grp":"ff02::6a","vid":1,"state":"temp"}]`),
			"br1": json.RawMessage(`[{"dev":"br1","port":"e9","grp":"224.2.2.2","state":"permanent"}]`),
		},
		rtrPorts: map[string][]string{"br1": {"e9"}},
	}
}

// A VLAN filtering bridge gets its VLANs with members and per-VLAN
// filters, and nothing at the bridge level (the YANG choice); it and
// its ports get their PVID.  A plain bridge keeps the bridge level
// multicast state and filters, and neither it nor its ports a PVID.
func TestMergeAugmentsBridgeVLANs(t *testing.T) {
	doc := iface.Transform(json.RawMessage(vlanBridgeLinks), nil, nil, noFiles{})
	got := mergeAugments(doc, vlanAugments(t))

	var root struct {
		Interface []map[string]json.RawMessage `json:"interface"`
	}
	if err := json.Unmarshal(got, &root); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	byName := make(map[string]map[string]json.RawMessage)
	for _, entry := range root.Interface {
		var name string
		json.Unmarshal(entry["name"], &name)
		byName[name] = entry
	}

	var br0 map[string]json.RawMessage
	if err := json.Unmarshal(byName["br0"]["infix-interfaces:bridge"], &br0); err != nil {
		t.Fatalf("br0 bridge augment: %v", err)
	}
	for _, key := range []string{"multicast", "multicast-filters"} {
		if _, ok := br0[key]; ok {
			t.Fatalf("VLAN filtering bridge must not carry bridge level %s: %s", key, br0[key])
		}
	}
	want := `{"proto":"ieee802-dot1q-types:c-vlan","vlan":[` +
		`{"multicast":{"snooping":true},"multicast-filters":{"multicast-filter":[{"group":"ff02::6a","ports":[{"port":"e1","state":"temporary"}]}]},"untagged":["br0","e1"],"vid":1},` +
		`{"multicast":{"snooping":true},"multicast-filters":{"multicast-filter":[{"group":"224.1.1.1","ports":[{"port":"e1","state":"temporary"},{"port":"e2","state":"temporary"}]}]},"tagged":["br0","e1","e2"],"vid":10}]}`
	if string(br0["vlans"]) != want {
		t.Fatalf("br0 vlans =\n%s\nwant\n%s", br0["vlans"], want)
	}

	var br1 map[string]json.RawMessage
	if err := json.Unmarshal(byName["br1"]["infix-interfaces:bridge"], &br1); err != nil {
		t.Fatalf("br1 bridge augment: %v", err)
	}
	if _, ok := br1["vlans"]; ok {
		t.Fatal("plain bridge must not carry vlans")
	}
	if string(br1["multicast"]) != `{"router-ports":["e9"],"snooping":false}` {
		t.Fatalf("br1 multicast = %s", br1["multicast"])
	}
	if !strings.Contains(string(br1["multicast-filters"]), `"224.2.2.2"`) {
		t.Fatalf("br1 multicast-filters = %s", br1["multicast-filters"])
	}

	bridgePort := func(name string) map[string]json.RawMessage {
		var port map[string]json.RawMessage
		if err := json.Unmarshal(byName[name]["infix-interfaces:bridge-port"], &port); err != nil {
			t.Fatalf("%s bridge-port: %v", name, err)
		}
		return port
	}
	if port := bridgePort("e2"); string(port["pvid"]) != "10" || string(port["bridge"]) != `"br0"` {
		t.Fatalf("e2 bridge-port = %s, want pvid 10 with the kernel fields kept", byName["e2"]["infix-interfaces:bridge-port"])
	}
	if port := bridgePort("e1"); string(port["pvid"]) != "1" {
		t.Fatalf("e1 pvid = %s, want 1", port["pvid"])
	}
	if _, ok := bridgePort("e9")["pvid"]; ok {
		t.Fatal("port of a plain bridge must not get a pvid")
	}
	if port := bridgePort("br0"); string(port["pvid"]) != "1" {
		t.Fatalf("br0 pvid = %s, want 1", port["pvid"])
	}
	if _, ok := byName["br1"]["infix-interfaces:bridge-port"]; ok {
		t.Fatal("plain bridge must not get a pvid")
	}
}

func TestRouterPorts(t *testing.T) {
	// `bridge -json -d mdb show`, iproute2 6.14 (mdb.c print_rtrs)
	raw := json.RawMessage(`[{"mdb":[{"index":5,"dev":"br0","port":"e1","grp":"224.1.1.1","state":"temp","flags":[],"vid":10}],` +
		`"router":{"br1":[{"port":"e9"},{"port":"e8"}],"br0":[]}}]`)
	want := map[string][]string{"br1": {"e9", "e8"}}
	if got := routerPorts(raw); !reflect.DeepEqual(got, want) {
		t.Fatalf("routerPorts = %v, want %v", got, want)
	}
	if got := routerPorts(json.RawMessage(`[{"mdb":[],"router":{}}]`)); len(got) != 0 {
		t.Fatalf("no router ports, got %v", got)
	}
}

func TestIsRouterMsg(t *testing.T) {
	brPortMsg := make([]byte, sizeofBrPortMsg)
	msg := func(attr uint16) syscall.NetlinkMessage {
		return syscall.NetlinkMessage{Data: append(brPortMsg, nl.NewRtAttr(int(attr), nil).Serialize()...)}
	}
	if !isRouterMsg(msg(mdbaRouter)) {
		t.Error("MDBA_ROUTER must be a router message")
	}
	if isRouterMsg(msg(1)) {
		t.Error("MDBA_MDB must not be a router message")
	}
	if isRouterMsg(syscall.NetlinkMessage{Data: []byte{0}}) {
		t.Error("a short message must not be a router message")
	}
}

func TestTransformMDBVID(t *testing.T) {
	raw := json.RawMessage(`[{"dev":"br0","port":"e1","grp":"224.1.1.1","vid":10,"state":"temp"},{"dev":"br0","port":"e2","grp":"ff02::6a","vid":20,"state":"temp"},{"dev":"br0","port":"e3","grp":"224.3.3.3","state":"permanent"}]`)

	got := transformMDBVID(raw, 10)
	filters := got["multicast-filter"].([]map[string]any)
	if len(filters) != 1 || filters[0]["group"] != "224.1.1.1" {
		t.Fatalf("vid 10 filters = %v", filters)
	}
	if transformMDBVID(raw, 30) != nil {
		t.Fatal("no entry for vid 30, want nil")
	}
	if all := transformMDB(raw)["multicast-filter"].([]map[string]any); len(all) != 3 {
		t.Fatalf("unfiltered got %d groups, want 3", len(all))
	}
}

func TestAppendInterfaces(t *testing.T) {
	doc := json.RawMessage(`{"interface":[{"name":"lo","if-index":1},{"name":"eth0","if-index":2}]}`)
	extra := map[string]json.RawMessage{
		"net1": json.RawMessage(`{"name":"net1","if-index":9,"statistics":{"in-octets":"18446744073709551615"}}`),
		"eth0": json.RawMessage(`{"name":"eth0","if-index":3}`),
		"net0": json.RawMessage(`{"name":"net0","if-index":8}`),
	}

	got := appendInterfaces(doc, extra)
	want := `{"interface":[{"name":"lo","if-index":1},{"name":"eth0","if-index":2},{"name":"net0","if-index":8},{"name":"net1","if-index":9,"statistics":{"in-octets":"18446744073709551615"}}]}`
	if string(got) != want {
		t.Fatalf("appendInterfaces =\n%s\nwant\n%s", got, want)
	}

	if got := appendInterfaces(doc, nil); string(got) != string(doc) {
		t.Fatalf("no extra must be a no-op, got %s", got)
	}
	if got := appendInterfaces(json.RawMessage(`{}`), map[string]json.RawMessage{"net0": json.RawMessage(`{"name":"net0"}`)}); string(got) != `{"interface":[{"name":"net0"}]}` {
		t.Fatalf("empty document got %s", got)
	}
}
