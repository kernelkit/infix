package iface

import (
	"encoding/json"
	"reflect"
	"testing"
)

// Shapes as iproute2 6.14 prints them (bridge/vlan.c print_vlan_opts,
// print_vlan_global_opts): a range gets "vlanEnd", a single VID does
// not, flags are absent when none is set, and "router_ports" only when
// the VLAN has a multicast router port.
const vlanShowJSON = `[
 {"ifname":"br0","vlans":[{"vlan":1,"flags":["PVID","Egress Untagged"]},{"vlan":10,"vlanEnd":11}]},
 {"ifname":"e1","vlans":[{"vlan":1,"flags":["PVID","Egress Untagged"]},{"vlan":10}]},
 {"ifname":"e2","vlans":[{"vlan":10,"flags":["PVID"]},{"vlan":11,"flags":["Egress Untagged"]},{"vlan":99}]},
 {"ifname":"br1","vlans":[{"vlan":1,"flags":["PVID","Egress Untagged"]}]},
 {"ifname":"e9","vlans":[{"vlan":1,"flags":["PVID","Egress Untagged"]}]}
]`

const vlanGlobalJSON = `[{"ifname":"br0","vlans":[
 {"vlan":1,"mcast_snooping":1,"mcast_querier":0,"mcast_igmp_version":2,"mcast_mld_version":1,"mcast_query_interval":12500,"router_ports":[{"port":"e1"}]},
 {"vlan":10,"vlanEnd":11,"mcast_snooping":0,"mcast_querier":0,"mcast_igmp_version":2,"mcast_mld_version":1}
]}]`

const bridgeLinksJSON = `[
 {"ifindex":1,"ifname":"lo","link_type":"loopback"},
 {"ifindex":5,"ifname":"br0","linkinfo":{"info_kind":"bridge","info_data":{"vlan_filtering":1,"vlan_protocol":"802.1Q","mcast_snooping":1}}},
 {"ifindex":6,"ifname":"br1","linkinfo":{"info_kind":"bridge","info_data":{"vlan_filtering":0,"vlan_protocol":"802.1ad","mcast_snooping":0}}},
 {"ifindex":2,"ifname":"e1","master":"br0","linkinfo":{"info_kind":"veth","info_slave_kind":"bridge","info_slave_data":{"state":"forwarding"}}},
 {"ifindex":3,"ifname":"e2","master":"br0","linkinfo":{"info_slave_kind":"bridge","info_slave_data":{}}},
 {"ifindex":4,"ifname":"e9","master":"br1","linkinfo":{"info_slave_kind":"bridge","info_slave_data":{}}},
 {"ifindex":7,"ifname":"bond0","linkinfo":{"info_kind":"bond"}},
 {"ifindex":8,"ifname":"e8","master":"bond0","linkinfo":{"info_slave_kind":"bond"}}
]`

func TestBridges(t *testing.T) {
	got := Bridges(json.RawMessage(bridgeLinksJSON))
	want := map[string]BridgeInfo{
		"br0": {Name: "br0", Index: 5, VLANFiltering: true, Protocol: "802.1Q", Snooping: true, Ports: []string{"e1", "e2"}},
		"br1": {Name: "br1", Index: 6, VLANFiltering: false, Protocol: "802.1ad", Snooping: false, Ports: []string{"e9"}},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("Bridges = %+v, want %+v", got, want)
	}
	if Bridges(json.RawMessage(`[{"ifindex":1,"ifname":"lo"}]`)) != nil {
		t.Fatal("no bridge in dump must give nil")
	}
	if Bridges(nil) != nil {
		t.Fatal("empty dump must give nil")
	}
}

func TestParseVLANTable(t *testing.T) {
	got := ParseVLANTable(json.RawMessage(vlanShowJSON))
	want := map[string][]VLANEntry{
		"br0": {{VID: 1, End: 1, PVID: true, Untagged: true}, {VID: 10, End: 11}},
		"e1":  {{VID: 1, End: 1, PVID: true, Untagged: true}, {VID: 10, End: 10}},
		"e2":  {{VID: 10, End: 10, PVID: true}, {VID: 11, End: 11, Untagged: true}, {VID: 99, End: 99}},
		"br1": {{VID: 1, End: 1, PVID: true, Untagged: true}},
		"e9":  {{VID: 1, End: 1, PVID: true, Untagged: true}},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("ParseVLANTable = %+v, want %+v", got, want)
	}
	if ParseVLANTable(json.RawMessage(`[]`)) != nil {
		t.Fatal("empty table must give nil")
	}
	if ParseVLANTable(json.RawMessage(`not json`)) != nil {
		t.Fatal("garbage must give nil")
	}
}

func TestParseVLANGlobals(t *testing.T) {
	got := ParseVLANGlobals(json.RawMessage(vlanGlobalJSON))
	want := []VLANGlobal{{VID: 1, End: 1, Snooping: true, RouterPorts: []string{"e1"}}, {VID: 10, End: 11, Snooping: false}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("ParseVLANGlobals = %+v, want %+v", got, want)
	}
	if ParseVLANGlobals(json.RawMessage(`[]`)) != nil {
		t.Fatal("empty output must give nil")
	}
}

func TestPVID(t *testing.T) {
	table := ParseVLANTable(json.RawMessage(vlanShowJSON))
	if vid, ok := PVID(table["e2"]); !ok || vid != 10 {
		t.Fatalf("PVID(e2) = %d,%v, want 10,true", vid, ok)
	}
	if _, ok := PVID([]VLANEntry{{VID: 5, End: 5}}); ok {
		t.Fatal("entry without the PVID flag must not be a PVID")
	}
	if _, ok := PVID(nil); ok {
		t.Fatal("no entries must not be a PVID")
	}
}

func TestBridgeVLANs(t *testing.T) {
	bridges := Bridges(json.RawMessage(bridgeLinksJSON))
	table := ParseVLANTable(json.RawMessage(vlanShowJSON))
	globals := ParseVLANGlobals(json.RawMessage(vlanGlobalJSON))

	filters := func(vid int) map[string]any {
		if vid != 10 {
			return nil
		}
		return map[string]any{"multicast-filter": []map[string]any{{"group": "224.1.1.1"}}}
	}

	got := BridgeVLANs(bridges["br0"], table, globals, filters)
	want := map[string]any{
		"proto": "ieee802-dot1q-types:c-vlan",
		"vlan": []map[string]any{
			{
				"vid":       1,
				"multicast": map[string]any{"snooping": true, "router-ports": []string{"e1"}},
				"untagged":  []string{"br0", "e1"},
			},
			{
				"vid":               10,
				"multicast":         map[string]any{"snooping": false},
				"tagged":            []string{"br0", "e1", "e2"},
				"multicast-filters": map[string]any{"multicast-filter": []map[string]any{{"group": "224.1.1.1"}}},
			},
			{
				"vid":       11,
				"multicast": map[string]any{"snooping": false},
				"tagged":    []string{"br0"},
				"untagged":  []string{"e2"},
			},
		},
	}
	if !reflect.DeepEqual(got, want) {
		gotJSON, _ := json.Marshal(got)
		wantJSON, _ := json.Marshal(want)
		t.Fatalf("BridgeVLANs =\n%s\nwant\n%s", gotJSON, wantJSON)
	}
	// VLAN 99 is on e2 but not in the bridge's global table: not listed.
	for _, v := range got["vlan"].([]map[string]any) {
		if v["vid"] == 99 {
			t.Fatal("port-only VLAN 99 must not be listed")
		}
	}
}

func TestBridgeVLANsSVLANNoGlobals(t *testing.T) {
	br := BridgeInfo{Name: "br1", Protocol: "802.1ad", VLANFiltering: true, Ports: []string{"e9"}}
	got := BridgeVLANs(br, ParseVLANTable(json.RawMessage(vlanShowJSON)), nil, nil)
	want := map[string]any{"proto": "ieee802-dot1q-types:s-vlan"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("BridgeVLANs without globals = %+v, want %+v", got, want)
	}
}

// A port can hold a VID range the global table lists as single VIDs
// and the other way round; membership is per VID either way.
func TestBridgeVLANsRanges(t *testing.T) {
	br := BridgeInfo{Name: "br0", Protocol: "802.1Q", Ports: []string{"e1"}}
	table := ParseVLANTable(json.RawMessage(`[{"ifname":"e1","vlans":[{"vlan":20,"vlanEnd":22}]}]`))
	globals := ParseVLANGlobals(json.RawMessage(`[{"ifname":"br0","vlans":[{"vlan":20,"mcast_snooping":1},{"vlan":21,"vlanEnd":22,"mcast_snooping":1}]}]`))

	got := BridgeVLANs(br, table, globals, nil)
	list := got["vlan"].([]map[string]any)
	if len(list) != 3 {
		t.Fatalf("got %d vlans, want 3", len(list))
	}
	for i, vid := range []int{20, 21, 22} {
		if list[i]["vid"] != vid {
			t.Fatalf("vlan[%d].vid = %v, want %d", i, list[i]["vid"], vid)
		}
		if tagged, _ := list[i]["tagged"].([]string); !reflect.DeepEqual(tagged, []string{"e1"}) {
			t.Fatalf("vlan %d tagged = %v, want [e1]", vid, tagged)
		}
	}
}
