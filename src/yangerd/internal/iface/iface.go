// Package iface transforms raw `ip -json` data into YANG-shaped
// ietf-interfaces JSON.
package iface

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
)

// FileChecker abstracts filesystem probes needed during interface transformation.
type FileChecker interface {
	Exists(path string) bool
	ReadFile(path string) (string, error)
}

// Transform converts raw `ip -json` link/address/statistics arrays into
// `{"interface":[...]}`.  The caller (NLMonitor) stores this at tree key
// "ietf-interfaces:interfaces"; the IPC server adds the module-qualified
// wrapper when responding to clients.
func Transform(linkData, addrData, statsData json.RawMessage, fc FileChecker) json.RawMessage {
	links := dedup(decodeObjects(linkData))
	addrs := decodeObjects(addrData)
	stats := decodeObjects(statsData)

	addrByName := make(map[string]map[string]any, len(addrs))
	for _, addr := range addrs {
		ifname := getString(addr, "ifname")
		if ifname == "" {
			continue
		}
		addrByName[ifname] = addr
	}

	statsByName := make(map[string]map[string]any, len(stats))
	for _, st := range stats {
		ifname := getString(st, "ifname")
		if ifname == "" {
			continue
		}
		statsByName[ifname] = st
	}

	interfaces := make([]map[string]any, 0, len(links))
	for _, iplink := range links {
		if skipInterface(iplink) {
			continue
		}

		ifname := getString(iplink, "ifname")
		ipaddr, ok := addrByName[ifname]
		if !ok {
			ipaddr = map[string]any{}
		}

		if st, ok := statsByName[ifname]; ok {
			if stat64, ok := st["stats64"]; ok {
				iplink["stats64"] = stat64
			}
		}

		iface := interfaceCommon(iplink, ipaddr, fc)
		yangType := getString(iface, "type")

		switch yangType {
		case "infix-if-type:vlan":
			if v := vlanAugment(iplink); len(v) > 0 {
				iface["infix-interfaces:vlan"] = v
			}
		case "infix-if-type:veth":
			if v := vethAugment(iplink); len(v) > 0 {
				iface["infix-interfaces:veth"] = v
			}
		case "infix-if-type:gre", "infix-if-type:gretap":
			if v := greAugment(iplink); len(v) > 0 {
				iface["infix-interfaces:gre"] = v
			}
		case "infix-if-type:vxlan":
			if v := vxlanAugment(iplink); len(v) > 0 {
				iface["infix-interfaces:vxlan"] = v
			}
		case "infix-if-type:lag":
			if v := lagAugment(iplink); len(v) > 0 {
				iface["infix-interfaces:lag"] = v
			}
		}

		switch iplink2yangLower(iplink) {
		case "infix-interfaces:bridge-port":
			if lower := bridgePortLower(iplink); len(lower) > 0 {
				iface["infix-interfaces:bridge-port"] = lower
			}
		case "infix-interfaces:lag-port":
			if lower := lagPortLower(iplink); len(lower) > 0 {
				iface["infix-interfaces:lag-port"] = lower
			}
		}

		interfaces = append(interfaces, iface)
	}

	out := map[string]any{
		"interface": interfaces,
	}

	raw, err := json.Marshal(out)
	if err != nil {
		return json.RawMessage(`{"interface":[]}`)
	}

	return raw
}

func decodeObjects(raw json.RawMessage) []map[string]any {
	if len(raw) == 0 {
		return nil
	}

	var entries []any
	if err := json.Unmarshal(raw, &entries); err != nil {
		return nil
	}

	out := make([]map[string]any, 0, len(entries))
	for _, entry := range entries {
		obj, ok := asMap(entry)
		if !ok {
			continue
		}
		out = append(out, obj)
	}

	return out
}

func skipInterface(iplink map[string]any) bool {
	if getString(iplink, "group") == "internal" {
		return true
	}

	switch getString(iplink, "link_type") {
	case "can", "vcan":
		return true
	default:
		return false
	}
}

// dedup removes duplicate link entries that share the same ifindex.
// When an interface is renamed (e.g. eth0 → e1), ip -json may report
// both the old and new names with the same ifindex.  We keep the entry
// whose operstate is "UP", or the last one seen if neither is up.
func dedup(links []map[string]any) []map[string]any {
	seen := make(map[int]int, len(links))
	out := make([]map[string]any, 0, len(links))
	for _, link := range links {
		idx := getIntOrZero(link, "ifindex")
		if idx == 0 {
			out = append(out, link)
			continue
		}
		if prev, ok := seen[idx]; ok {
			if getString(link, "operstate") == "UP" && getString(out[prev], "operstate") != "UP" {
				out[prev] = link
			}
		} else {
			seen[idx] = len(out)
			out = append(out, link)
		}
	}
	return out
}

func interfaceCommon(iplink, ipaddr map[string]any, fc FileChecker) map[string]any {
	flags := getStrings(iplink, "flags")

	iface := map[string]any{
		"type":         iplink2yangType(iplink, fc),
		"name":         getString(iplink, "ifname"),
		"if-index":     getIntOrZero(iplink, "ifindex"),
		"admin-status": boolToStatus(contains(flags, "UP"), "up", "down"),
		"oper-status":  iplink2yangOperstate(getString(iplink, "operstate")),
	}

	if _, ok := iplink["ifalias"]; ok {
		iface["description"] = getString(iplink, "ifalias")
	}

	if !contains(flags, "POINTOPOINT") {
		if address, ok := iplink["address"]; ok {
			iface["phys-address"] = fmt.Sprintf("%v", address)
		}
	}

	if stats := statistics(iplink); len(stats) > 0 {
		iface["statistics"] = stats
	}

	if ipv4 := ipv4Data(ipaddr); len(ipv4) > 0 {
		iface["ietf-ip:ipv4"] = ipv4
	}

	if ipv6 := ipv6Data(ipaddr, fc); len(ipv6) > 0 {
		iface["ietf-ip:ipv6"] = ipv6
	}

	return iface
}

func iplink2yangType(iplink map[string]any, fc FileChecker) string {
	ifname := getString(iplink, "ifname")

	switch getString(iplink, "link_type") {
	case "loopback":
		return "infix-if-type:loopback"
	case "gre", "gre6":
		return "infix-if-type:gre"
	case "ether":
		if fc != nil {
			if fc.Exists(fmt.Sprintf("/sys/class/net/%s/wireless/", ifname)) {
				return "infix-if-type:wifi"
			}
		}
	case "none":
	default:
		return "infix-if-type:other"
	}

	linkinfo, _ := asMap(iplink["linkinfo"])
	switch getString(linkinfo, "info_kind") {
	case "bond":
		return "infix-if-type:lag"
	case "bridge":
		return "infix-if-type:bridge"
	case "dummy":
		return "infix-if-type:dummy"
	case "gretap", "ip6gretap":
		return "infix-if-type:gretap"
	case "vxlan":
		return "infix-if-type:vxlan"
	case "veth":
		return "infix-if-type:veth"
	case "vlan":
		return "infix-if-type:vlan"
	case "wireguard":
		return "infix-if-type:wireguard"
	default:
		return "infix-if-type:ethernet"
	}
}

func iplink2yangLower(iplink map[string]any) string {
	linkinfo, _ := asMap(iplink["linkinfo"])
	switch getString(linkinfo, "info_slave_kind") {
	case "bridge":
		return "infix-interfaces:bridge-port"
	case "bond":
		return "infix-interfaces:lag-port"
	default:
		return ""
	}
}

func iplink2yangOperstate(oper string) string {
	switch oper {
	case "DOWN":
		return "down"
	case "UP":
		return "up"
	case "DORMANT":
		return "dormant"
	case "TESTING":
		return "testing"
	case "LOWERLAYERDOWN":
		return "lower-layer-down"
	case "NOTPRESENT":
		return "not-present"
	default:
		return "unknown"
	}
}

func statistics(iplink map[string]any) map[string]any {
	out := map[string]any{}

	stats64, _ := asMap(iplink["stats64"])
	rx, _ := asMap(stats64["rx"])
	tx, _ := asMap(stats64["tx"])

	if octets, ok := rx["bytes"]; ok && isTruthy(octets) {
		out["in-octets"] = toCounterString(octets)
	}

	if octets, ok := tx["bytes"]; ok && isTruthy(octets) {
		out["out-octets"] = toCounterString(octets)
	}

	return out
}

func ipv4Data(ipaddr map[string]any) map[string]any {
	if len(ipaddr) == 0 {
		return nil
	}

	out := map[string]any{}
	if mtu, ok := getInt(ipaddr, "mtu"); ok && mtu != 0 && getString(ipaddr, "ifname") != "lo" {
		out["mtu"] = mtu
	}

	if addr := addresses(ipaddr, "inet"); len(addr) > 0 {
		out["address"] = addr
	}

	return out
}

func ipv6Data(ipaddr map[string]any, fc FileChecker) map[string]any {
	if len(ipaddr) == 0 {
		return nil
	}

	out := map[string]any{}
	ifname := getString(ipaddr, "ifname")
	if ifname != "" && fc != nil {
		path := fmt.Sprintf("/proc/sys/net/ipv6/conf/%s/mtu", ifname)
		if raw, err := fc.ReadFile(path); err == nil {
			trimmed := strings.TrimSpace(raw)
			if mtu, err := strconv.Atoi(trimmed); err == nil {
				out["mtu"] = mtu
			}
		}
	}

	if addr := addresses(ipaddr, "inet6"); len(addr) > 0 {
		out["address"] = addr
	}

	return out
}

func addresses(ipaddr map[string]any, family string) []map[string]any {
	addrInfo, ok := ipaddr["addr_info"]
	if !ok {
		return nil
	}

	arr, ok := asArray(addrInfo)
	if !ok {
		return nil
	}

	out := make([]map[string]any, 0, len(arr))
	for _, entry := range arr {
		inet, ok := asMap(entry)
		if !ok {
			continue
		}

		if getString(inet, "family") != family {
			continue
		}

		address := map[string]any{
			"ip":            inet["local"],
			"prefix-length": getIntOrZero(inet, "prefixlen"),
			"origin":        inet2yangOrigin(inet),
		}
		out = append(out, address)
	}

	return out
}

func inet2yangOrigin(inet map[string]any) string {
	proto := getString(inet, "protocol")
	if proto == "kernel_ll" || proto == "kernel_ra" {
		if _, ok := inet["stable-privacy"]; ok {
			return "random"
		}
	}

	switch proto {
	case "kernel_ll", "kernel_ra":
		return "link-layer"
	case "static":
		return "static"
	case "dhcp":
		return "dhcp"
	case "random":
		return "random"
	default:
		return "other"
	}
}

func vlanAugment(iplink map[string]any) map[string]any {
	info := infoData(iplink)
	if len(info) == 0 {
		return nil
	}

	vlan := map[string]any{
		"tag-type": proto2yang(getString(info, "protocol")),
		"id":       getIntOrZero(info, "id"),
	}

	if lower := getString(iplink, "link"); lower != "" {
		vlan["lower-layer-if"] = lower
	}

	return vlan
}

func vethAugment(iplink map[string]any) map[string]any {
	peer := getString(iplink, "link")
	if peer == "" {
		return nil
	}

	return map[string]any{"peer": peer}
}

func greAugment(iplink map[string]any) map[string]any {
	info := infoData(iplink)
	if len(info) == 0 {
		return nil
	}

	return map[string]any{
		"local":  firstAny(info["local"], info["local6"]),
		"remote": firstAny(info["remote"], info["remote6"]),
	}
}

func vxlanAugment(iplink map[string]any) map[string]any {
	vxlan := greAugment(iplink)
	if len(vxlan) == 0 {
		return nil
	}

	info := infoData(iplink)
	if vni, ok := info["id"]; ok {
		vxlan["vni"] = vni
	}

	return vxlan
}

func lagAugment(iplink map[string]any) map[string]any {
	info := infoData(iplink)
	if len(info) == 0 {
		return nil
	}

	mode := lagMode(getString(info, "mode"))
	bond := map[string]any{
		"mode": mode,
		"link-monitor": map[string]any{
			"debounce": map[string]any{
				"up":   getIntOrZero(info, "updelay"),
				"down": getIntOrZero(info, "downdelay"),
			},
		},
	}

	if mode == "lacp" {
		lacp := map[string]any{
			"mode": boolToStatus(getString(info, "ad_lacp_active") == "on", "active", "passive"),
			"rate": getString(info, "ad_lacp_rate"),
			"hash": lagHash(getString(info, "xmit_hash_policy")),
		}

		adInfo, ok := asMap(info["ad_info"])
		if ok {
			if v, ok := adInfo["aggregator"]; ok {
				lacp["aggregator-id"] = v
			}
			if v, ok := adInfo["actor_key"]; ok {
				lacp["actor-key"] = v
			}
			if v, ok := adInfo["partner_key"]; ok {
				lacp["partner-key"] = v
			}
			if v, ok := adInfo["partner_mac"]; ok {
				lacp["partner-mac"] = v
			}
		}

		if v, ok := info["ad_actor_sys_prio"]; ok {
			lacp["system-priority"] = v
		}

		bond["lacp"] = lacp
	} else {
		bond["static"] = map[string]any{
			"mode": getString(info, "mode"),
			"hash": getString(info, "xmit_hash_policy"),
		}
	}

	return bond
}

func bridgePortLower(iplink map[string]any) map[string]any {
	master := getString(iplink, "master")
	if master == "" {
		return nil
	}

	linkinfo, _ := asMap(iplink["linkinfo"])
	info, _ := asMap(linkinfo["info_slave_data"])
	if len(info) == 0 {
		return nil
	}

	return map[string]any{
		"bridge": master,
		"flood": map[string]any{
			"broadcast": getBool(info, "bcast_flood"),
			"unicast":   getBool(info, "flood"),
			"multicast": getBool(info, "mcast_flood"),
		},
		"multicast": map[string]any{
			"fast-leave": getBool(info, "fastleave"),
			"router":     bridgeRouterMode(getIntOrZero(info, "multicast_router")),
		},
		"stp": map[string]any{},
	}
}

func lagPortLower(iplink map[string]any) map[string]any {
	master := getString(iplink, "master")
	if master == "" {
		return nil
	}

	port := map[string]any{"lag": master}

	linkinfo, _ := asMap(iplink["linkinfo"])
	info, _ := asMap(linkinfo["info_slave_data"])
	if len(info) == 0 {
		port["state"] = "backup"
		port["link-failures"] = 0
		return port
	}

	port["state"] = strings.ToLower(getString(info, "state"))
	port["link-failures"] = getIntOrZero(info, "link_failure_count")

	if _, ok := info["ad_aggregator_id"]; ok {
		port["lacp"] = map[string]any{
			"aggregator-id": info["ad_aggregator_id"],
			"actor-state":   getString(info, "ad_actor_oper_port_state_str"),
			"partner-state": getString(info, "ad_partner_oper_port_state_str"),
		}
	}

	return port
}

func infoData(iplink map[string]any) map[string]any {
	linkinfo, ok := asMap(iplink["linkinfo"])
	if !ok {
		return nil
	}

	data, ok := asMap(linkinfo["info_data"])
	if !ok {
		return nil
	}

	return data
}

func proto2yang(proto string) string {
	switch proto {
	case "802.1Q":
		return "ieee802-dot1q-types:c-vlan"
	case "802.1ad":
		return "ieee802-dot1q-types:s-vlan"
	default:
		return "other"
	}
}

func lagMode(mode string) string {
	switch mode {
	case "802.3ad":
		return "lacp"
	case "balance-xor":
		return "static"
	default:
		return "static"
	}
}

func lagHash(hash string) string {
	switch hash {
	case "layer2":
		return "layer2"
	case "layer3+4":
		return "layer3-4"
	case "layer2+3":
		return "layer2-3"
	case "encap2+3":
		return "encap2-3"
	case "encap3+4":
		return "encap3-4"
	case "vlan+srcmac":
		return "vlan-srcmac"
	default:
		return "layer2"
	}
}

func bridgeRouterMode(v int) string {
	switch v {
	case 0:
		return "off"
	case 1:
		return "auto"
	case 2:
		return "permanent"
	default:
		return "UNKNOWN"
	}
}

func getString(obj map[string]any, key string) string {
	v, ok := obj[key]
	if !ok || v == nil {
		return ""
	}

	s, ok := v.(string)
	if ok {
		return s
	}

	return fmt.Sprintf("%v", v)
}

func getInt(obj map[string]any, key string) (int, bool) {
	v, ok := obj[key]
	if !ok || v == nil {
		return 0, false
	}

	switch n := v.(type) {
	case int:
		return n, true
	case int8:
		return int(n), true
	case int16:
		return int(n), true
	case int32:
		return int(n), true
	case int64:
		return int(n), true
	case uint:
		return int(n), true
	case uint8:
		return int(n), true
	case uint16:
		return int(n), true
	case uint32:
		return int(n), true
	case uint64:
		return int(n), true
	case float64:
		return int(n), true
	case json.Number:
		i, err := n.Int64()
		if err != nil {
			return 0, false
		}
		return int(i), true
	case string:
		i, err := strconv.Atoi(strings.TrimSpace(n))
		if err != nil {
			return 0, false
		}
		return i, true
	default:
		return 0, false
	}
}

func getIntOrZero(obj map[string]any, key string) int {
	v, ok := getInt(obj, key)
	if !ok {
		return 0
	}
	return v
}

func getBool(obj map[string]any, key string) bool {
	v, ok := obj[key]
	if !ok || v == nil {
		return false
	}

	b, ok := v.(bool)
	if ok {
		return b
	}

	s := strings.ToLower(strings.TrimSpace(fmt.Sprintf("%v", v)))
	return s == "1" || s == "true" || s == "on" || s == "yes"
}

func getStrings(obj map[string]any, key string) []string {
	v, ok := obj[key]
	if !ok || v == nil {
		return nil
	}

	if direct, ok := v.([]string); ok {
		return direct
	}

	arr, ok := asArray(v)
	if !ok {
		return nil
	}

	out := make([]string, 0, len(arr))
	for _, item := range arr {
		out = append(out, fmt.Sprintf("%v", item))
	}
	return out
}

func asMap(v any) (map[string]any, bool) {
	if v == nil {
		return nil, false
	}

	m, ok := v.(map[string]any)
	if ok {
		return m, true
	}

	m2, ok := v.(map[string]interface{})
	if ok {
		return map[string]any(m2), true
	}

	return nil, false
}

func asArray(v any) ([]any, bool) {
	if v == nil {
		return nil, false
	}

	arr, ok := v.([]any)
	if ok {
		return arr, true
	}

	arr2, ok := v.([]interface{})
	if ok {
		return []any(arr2), true
	}

	return nil, false
}

func contains(values []string, needle string) bool {
	for _, value := range values {
		if value == needle {
			return true
		}
	}
	return false
}

func isTruthy(v any) bool {
	if v == nil {
		return false
	}

	s := strings.TrimSpace(fmt.Sprintf("%v", v))
	return s != "" && s != "0"
}

func toCounterString(v any) string {
	switch n := v.(type) {
	case int:
		return strconv.FormatInt(int64(n), 10)
	case int8:
		return strconv.FormatInt(int64(n), 10)
	case int16:
		return strconv.FormatInt(int64(n), 10)
	case int32:
		return strconv.FormatInt(int64(n), 10)
	case int64:
		return strconv.FormatInt(n, 10)
	case uint:
		return strconv.FormatUint(uint64(n), 10)
	case uint8:
		return strconv.FormatUint(uint64(n), 10)
	case uint16:
		return strconv.FormatUint(uint64(n), 10)
	case uint32:
		return strconv.FormatUint(uint64(n), 10)
	case uint64:
		return strconv.FormatUint(n, 10)
	case float64:
		return strconv.FormatInt(int64(n), 10)
	case json.Number:
		return n.String()
	case string:
		return strings.TrimSpace(n)
	default:
		return fmt.Sprintf("%v", v)
	}
}

func firstAny(a, b any) any {
	if a != nil {
		s := strings.TrimSpace(fmt.Sprintf("%v", a))
		if s != "" {
			return a
		}
	}
	return b
}

func boolToStatus(cond bool, yes, no string) string {
	if cond {
		return yes
	}
	return no
}
