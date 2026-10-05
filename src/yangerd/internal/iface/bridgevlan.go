package iface

import (
	"encoding/json"
	"sort"
)

// VLANEntry is one row of `bridge -json vlan show` for an interface: a
// VID range with its flags.
type VLANEntry struct {
	VID      int
	End      int // last VID of the range, equal to VID for a single one
	PVID     bool
	Untagged bool
}

// VLANGlobal is one row of `bridge -json vlan global show`: the
// bridge-wide options of a VID range.
type VLANGlobal struct {
	VID         int
	End         int
	Snooping    bool
	RouterPorts []string
}

// BridgeInfo is what the bridge VLAN augment needs to know about a
// bridge from an `ip -json -d link` dump.
type BridgeInfo struct {
	Name          string
	Index         int
	VLANFiltering bool
	Protocol      string // as iproute2 prints it, "802.1Q" or "802.1ad"
	Snooping      bool
	AgingTime     int      // seconds a dynamic FDB entry is kept
	Ports         []string // bridge ports in dump order
}

// Bridges returns every bridge in an `ip -json -d link` dump by name,
// with its ports.  nil when the dump has no bridge.
func Bridges(linkData json.RawMessage) map[string]BridgeInfo {
	links := decodeObjects(linkData)
	bridges := make(map[string]BridgeInfo)
	for _, link := range links {
		name := getString(link, "ifname")
		linkinfo, _ := asMap(link["linkinfo"])
		if name == "" || getString(linkinfo, "info_kind") != "bridge" {
			continue
		}
		info, _ := asMap(linkinfo["info_data"])
		bridges[name] = BridgeInfo{
			Name:          name,
			Index:         getIntOrZero(link, "ifindex"),
			VLANFiltering: getBool(info, "vlan_filtering"),
			Protocol:      getString(info, "vlan_protocol"),
			Snooping:      getBool(info, "mcast_snooping"),
			AgingTime:     getIntOrZero(info, "ageing_time") / 100,
		}
	}
	if len(bridges) == 0 {
		return nil
	}

	for _, link := range links {
		linkinfo, _ := asMap(link["linkinfo"])
		if getString(linkinfo, "info_slave_kind") != "bridge" {
			continue
		}
		name := getString(link, "ifname")
		br, ok := bridges[getString(link, "master")]
		if !ok || name == "" {
			continue
		}
		br.Ports = append(br.Ports, name)
		bridges[br.Name] = br
	}
	return bridges
}

// ParseVLANTable decodes `bridge -json vlan show` into the VLAN entries
// of every interface that has any, the bridge itself included.
func ParseVLANTable(raw json.RawMessage) map[string][]VLANEntry {
	table := make(map[string][]VLANEntry)
	for _, row := range decodeObjects(raw) {
		name := getString(row, "ifname")
		if name == "" {
			continue
		}
		vlans, _ := asArray(row["vlans"])
		for _, v := range vlans {
			obj, ok := asMap(v)
			if !ok {
				continue
			}
			vid, end, ok := vidRange(obj)
			if !ok {
				continue
			}
			flags := getStrings(obj, "flags")
			table[name] = append(table[name], VLANEntry{
				VID:      vid,
				End:      end,
				PVID:     contains(flags, "PVID"),
				Untagged: contains(flags, "Egress Untagged"),
			})
		}
	}
	if len(table) == 0 {
		return nil
	}
	return table
}

// ParseVLANGlobals decodes `bridge -json vlan global show dev <bridge>`.
func ParseVLANGlobals(raw json.RawMessage) []VLANGlobal {
	var globals []VLANGlobal
	for _, row := range decodeObjects(raw) {
		vlans, _ := asArray(row["vlans"])
		for _, v := range vlans {
			obj, ok := asMap(v)
			if !ok {
				continue
			}
			vid, end, ok := vidRange(obj)
			if !ok {
				continue
			}
			globals = append(globals, VLANGlobal{
				VID:         vid,
				End:         end,
				Snooping:    getBool(obj, "mcast_snooping"),
				RouterPorts: RouterPorts(obj["router_ports"]),
			})
		}
	}
	return globals
}

// RouterPorts reads a list of router port objects as iproute2 prints
// them, `[{"port": "e1"}, ...]`.
func RouterPorts(v any) []string {
	list, _ := asArray(v)
	var ports []string
	for _, p := range list {
		obj, ok := asMap(p)
		if !ok {
			continue
		}
		if port := getString(obj, "port"); port != "" {
			ports = append(ports, port)
		}
	}
	return ports
}

// vidRange reads the "vlan" / "vlanEnd" pair iproute2 prints for a
// range, a single VID having no end.
func vidRange(obj map[string]any) (vid, end int, ok bool) {
	vid, ok = getInt(obj, "vlan")
	if !ok || vid <= 0 {
		return 0, 0, false
	}
	end, ok = getInt(obj, "vlanEnd")
	if !ok || end < vid {
		end = vid
	}
	return vid, end, true
}

// PVID returns the VID flagged PVID among entries.
func PVID(entries []VLANEntry) (int, bool) {
	for _, e := range entries {
		if e.PVID {
			return e.VID, true
		}
	}
	return 0, false
}

// BridgeVLANs builds the infix-interfaces:bridge/vlans container of a
// VLAN filtering bridge: the VLANs from the bridge's global table, each
// with the untagged and tagged members among the bridge itself and its
// ports, its snooping state and, when filters returns one, its
// multicast-filters container.  Members follow the order of br.Ports,
// the bridge first, like the Python yanger.  A port VLAN that is not
// in the global table is ignored.
func BridgeVLANs(br BridgeInfo, table map[string][]VLANEntry, globals []VLANGlobal, filters func(vid int) map[string]any) map[string]any {
	byVID := make(map[int]map[string]any)
	var vids []int
	for _, g := range globals {
		for vid := g.VID; vid <= g.End; vid++ {
			if _, seen := byVID[vid]; seen {
				continue
			}
			vids = append(vids, vid)
			mcast := map[string]any{"snooping": g.Snooping}
			if len(g.RouterPorts) > 0 {
				mcast["router-ports"] = g.RouterPorts
			}
			byVID[vid] = map[string]any{
				"vid":       vid,
				"multicast": mcast,
			}
		}
	}

	members := append([]string{br.Name}, br.Ports...)
	for _, member := range members {
		for _, e := range table[member] {
			for vid := e.VID; vid <= e.End; vid++ {
				vlan, ok := byVID[vid]
				if !ok {
					continue
				}
				key := "tagged"
				if e.Untagged {
					key = "untagged"
				}
				list, _ := vlan[key].([]string)
				vlan[key] = append(list, member)
			}
		}
	}

	sort.Ints(vids)
	list := make([]map[string]any, 0, len(vids))
	for _, vid := range vids {
		vlan := byVID[vid]
		if filters != nil {
			if mf := filters(vid); mf != nil {
				vlan["multicast-filters"] = mf
			}
		}
		list = append(list, vlan)
	}

	out := map[string]any{"proto": proto2yang(br.Protocol)}
	if len(list) > 0 {
		out["vlan"] = list
	}
	return out
}
