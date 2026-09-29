package collector

import (
	"context"
	"encoding/json"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/kernelkit/infix/src/yangerd/internal/frrvty"
	"github.com/kernelkit/infix/src/yangerd/internal/tree"
)

const (
	frrRunDir = "/var/run/frr"

	// vtyTimeout bounds one show command, so a wedged daemon cannot
	// stall the poll.
	vtyTimeout = 5 * time.Second
)

// VtyQuery runs a show command against one FRR daemon, named as its vty
// socket is ("ospfd", "ripd", "bfdd", "zebra"), and returns the output.
type VtyQuery func(ctx context.Context, daemon, command string) ([]byte, error)

// FRRVty is the production VtyQuery.  A daemon that is not running has
// no socket, so the dial fails and the protocol is skipped.
func FRRVty(ctx context.Context, daemon, command string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(ctx, vtyTimeout)
	defer cancel()
	return frrvty.New(filepath.Join(frrRunDir, daemon+".vty")).Query(ctx, command)
}

// RoutingCollector gathers ietf-routing operational data by merging
// OSPF, RIP, and BFD control-plane protocols into a single tree key.
// Each protocol contributes entries to the control-plane-protocol list
// under ietf-routing:routing.
type RoutingCollector struct {
	vty      VtyQuery
	interval time.Duration
}

// NewRoutingCollector creates a RoutingCollector querying FRR over vty.
// The runner is no longer used; it stays until the caller drops it.
func NewRoutingCollector(interval time.Duration) *RoutingCollector {
	return &RoutingCollector{vty: FRRVty, interval: interval}
}

// Name implements Collector.
func (c *RoutingCollector) Name() string { return "routing" }

// Interval implements Collector.
func (c *RoutingCollector) Interval() time.Duration { return c.interval }

// vtyJSON runs a show command and decodes its JSON output into dst.
func (c *RoutingCollector) vtyJSON(ctx context.Context, daemon, command string, dst interface{}) error {
	out, err := c.vty(ctx, daemon, command)
	if err != nil {
		return err
	}
	return json.Unmarshal(out, dst)
}

// Collect implements Collector.  It produces one tree key:
// "ietf-routing:routing" containing merged OSPF, RIP, and BFD data.
func (c *RoutingCollector) Collect(ctx context.Context, t *tree.Tree) error {
	protocols := []interface{}{}

	if p := c.collectOSPF(ctx); p != nil {
		protocols = append(protocols, p)
	}
	if p := c.collectRIP(ctx); p != nil {
		protocols = append(protocols, p)
	}
	if p := c.collectBFD(ctx); p != nil {
		protocols = append(protocols, p)
	}

	// Always written, also when empty, so a protocol that stops running
	// disappears instead of leaving its last state behind.
	routing := map[string]interface{}{
		"control-plane-protocols": map[string]interface{}{
			"control-plane-protocol": protocols,
		},
	}

	if data, err := json.Marshal(routing); err == nil {
		t.Merge("ietf-routing:routing", data)
	}
	return nil
}

// --- OSPF ---

var ospfIfaceStateMap = map[string]string{
	"DependUpon":     "down",
	"Down":           "down",
	"Waiting":        "waiting",
	"Loopback":       "loopback",
	"Point-To-Point": "point-to-point",
	"DROther":        "dr-other",
	"Backup":         "bdr",
	"DR":             "dr",
}

func frrToIETFNeighborState(state string) string {
	parts := strings.SplitN(state, "/", 2)
	s := parts[0]
	if s == "TwoWay" {
		return "2-way"
	}
	return strings.ToLower(s)
}

func frrToIETFNeighborRole(role string) string {
	if role == "Backup" {
		return "BDR"
	}
	return role
}

func ospfNetworkType(nt string, p2mpNonBroadcast bool) string {
	switch nt {
	case "POINTOPOINT":
		return "point-to-point"
	case "BROADCAST":
		return "broadcast"
	case "POINTOMULTIPOINT":
		if p2mpNonBroadcast {
			return "point-to-multipoint"
		}
		return "hybrid"
	case "NBMA":
		return "non-broadcast"
	default:
		return ""
	}
}

// ospfAreaSuffix is what ospfd appends to the area of a stub or NSSA
// area, e.g. "0.0.0.1 [Stub]".
var ospfAreaSuffix = map[string]string{
	" [Stub]": "stub-area",
	" [NSSA]": "nssa-area",
}

// ospfArea splits ospfd's area string into the bare area id and its
// area-type.
func ospfArea(area string) (string, string) {
	for suffix, areaType := range ospfAreaSuffix {
		if id, ok := strings.CutSuffix(area, suffix); ok {
			return id, areaType
		}
	}
	return area, "normal-area"
}

// ospfStatus merges ospfd's three views, areas, interfaces and
// neighbors, into areas holding their interfaces and each interface its
// neighbors, the way the ietf-ospf model nests them.  It is a port of
// the ospf-status helper yanger used.
func ospfStatus(ospf, ifaces, neighbors map[string]interface{}) map[string]interface{} {
	areas, _ := ospf["areas"].(map[string]interface{})
	ifaceMap, _ := ifaces["interfaces"].(map[string]interface{})
	nbrMap, _ := neighbors["neighbors"].(map[string]interface{})

	names := make([]string, 0, len(ifaceMap))
	for name := range ifaceMap {
		names = append(names, name)
	}
	sort.Strings(names)

	for _, name := range names {
		iface, ok := ifaceMap[name].(map[string]interface{})
		if !ok {
			continue
		}
		enabled, _ := iface["ospfEnabled"].(bool)
		areaStr, _ := iface["area"].(string)
		if !enabled || areaStr == "" {
			continue
		}

		areaID, areaType := ospfArea(areaStr)
		area, ok := areas[areaID].(map[string]interface{})
		if !ok {
			continue
		}
		area["area-type"] = areaType

		iface["name"] = name
		iface["area"] = areaID
		iface["neighbors"] = ospfIfaceNeighbors(nbrMap, name, areaID)

		list, _ := area["interfaces"].([]interface{})
		area["interfaces"] = append(list, iface)
	}

	return ospf
}

// ospfIfaceNeighbors picks the neighbors seen on one interface in one
// area, tagging each with the neighbor's router id.
func ospfIfaceNeighbors(nbrMap map[string]interface{}, ifname, areaID string) []interface{} {
	ids := make([]string, 0, len(nbrMap))
	for id := range nbrMap {
		ids = append(ids, id)
	}
	sort.Strings(ids)

	out := []interface{}{}
	for _, id := range ids {
		list, _ := nbrMap[id].([]interface{})
		for _, raw := range list {
			nbr, ok := raw.(map[string]interface{})
			if !ok {
				continue
			}
			nbrArea, _ := nbr["areaId"].(string)
			nbrArea, _ = ospfArea(nbrArea)
			if nbr["ifaceName"] != ifname || nbrArea != areaID {
				continue
			}
			nbr["areaId"] = nbrArea
			nbr["neighborIp"] = id
			out = append(out, nbr)
		}
	}
	return out
}

func (c *RoutingCollector) collectOSPF(ctx context.Context) interface{} {
	var ospfData, ifaces, neighbors map[string]interface{}
	if c.vtyJSON(ctx, "ospfd", "show ip ospf json", &ospfData) != nil ||
		c.vtyJSON(ctx, "ospfd", "show ip ospf interface json", &ifaces) != nil ||
		c.vtyJSON(ctx, "ospfd", "show ip ospf neighbor detail json", &neighbors) != nil {
		return nil
	}
	if len(ospfData) == 0 {
		return nil
	}
	data := ospfStatus(ospfData, ifaces, neighbors)

	ospf := map[string]interface{}{
		"ietf-ospf:address-family": "ipv4",
	}
	if rid, ok := data["routerId"]; ok {
		ospf["ietf-ospf:router-id"] = rid
	}

	areas := make([]interface{}, 0)
	areasRaw, _ := data["areas"].(map[string]interface{})
	for areaID, valRaw := range areasRaw {
		values, ok := valRaw.(map[string]interface{})
		if !ok {
			continue
		}

		area := map[string]interface{}{
			"ietf-ospf:area-id": areaID,
		}
		if at, ok := values["area-type"]; ok && at != nil {
			area["ietf-ospf:area-type"] = at
		}

		interfaces := make([]interface{}, 0)
		ifacesRaw, _ := values["interfaces"].([]interface{})
		for _, ifaceRaw := range ifacesRaw {
			if iface, ok := ifaceRaw.(map[string]interface{}); ok {
				interfaces = append(interfaces, ospfInterface(iface))
			}
		}

		area["ietf-ospf:interfaces"] = map[string]interface{}{
			"ietf-ospf:interface": interfaces,
		}
		areas = append(areas, area)
	}

	if routes := c.ospfRoutes(ctx); len(routes) > 0 {
		ospf["ietf-ospf:local-rib"] = map[string]interface{}{
			"ietf-ospf:route": routes,
		}
	}

	ospf["ietf-ospf:areas"] = map[string]interface{}{
		"ietf-ospf:area": areas,
	}

	return map[string]interface{}{
		"type":           "infix-routing:ospfv2",
		"name":           "default",
		"ietf-ospf:ospf": ospf,
	}
}

// secondsAtLeast1 converts a millisecond FRR timer to whole seconds,
// dropping values that round to zero.
func secondsAtLeast1(dst map[string]interface{}, key string, msec interface{}) {
	if msec == nil {
		return
	}
	if sec := toInt(msec) / 1000; sec >= 1 {
		dst[key] = sec
	}
}

func ospfInterface(iface map[string]interface{}) map[string]interface{} {
	intf := map[string]interface{}{
		"name": iface["name"],
	}

	setIfPresent(intf, "dr-router-id", iface, "drId")
	setIfPresent(intf, "dr-ip-addr", iface, "drAddress")
	setIfPresent(intf, "bdr-router-id", iface, "bdrId")
	setIfPresent(intf, "bdr-ip-addr", iface, "bdrAddress")

	passive, ok := iface["timerPassiveIface"]
	intf["passive"] = ok && passive != nil

	if v, ok := iface["ospfEnabled"]; ok {
		intf["enabled"] = v
	}

	if nt, ok := iface["networkType"].(string); ok {
		p2mpNB, _ := iface["p2mpNonBroadcast"].(bool)
		if it := ospfNetworkType(nt, p2mpNB); it != "" {
			intf["interface-type"] = it
		}
	}

	if s, ok := iface["state"].(string); ok {
		if mapped, ok := ospfIfaceStateMap[s]; ok {
			intf["state"] = mapped
		} else {
			intf["state"] = "unknown"
		}
	}

	setIfPresentInt(intf, "priority", iface, "priority")
	setIfPresentInt(intf, "cost", iface, "cost")
	setIfPresentInt(intf, "dead-interval", iface, "timerDeadSecs")
	setIfPresentInt(intf, "retransmit-interval", iface, "timerRetransmitSecs")
	setIfPresentInt(intf, "transmit-delay", iface, "transmitDelaySecs")

	secondsAtLeast1(intf, "hello-interval", iface["timerMsecs"])
	secondsAtLeast1(intf, "hello-timer", iface["timerHelloInMsecs"])
	if v := iface["timerWaitSecs"]; v != nil && toInt(v) >= 1 {
		intf["wait-timer"] = toInt(v)
	}

	neighbors := make([]interface{}, 0)
	neighsRaw, _ := iface["neighbors"].([]interface{})
	for _, neighRaw := range neighsRaw {
		if neigh, ok := neighRaw.(map[string]interface{}); ok {
			neighbors = append(neighbors, ospfNeighbor(neigh))
		}
	}
	intf["ietf-ospf:neighbors"] = map[string]interface{}{
		"ietf-ospf:neighbor": neighbors,
	}

	return intf
}

func ospfNeighbor(neigh map[string]interface{}) map[string]interface{} {
	neighbor := map[string]interface{}{
		"neighbor-router-id": neigh["neighborIp"],
		"address":            neigh["ifaceAddress"],
	}

	setIfPresentInt(neighbor, "priority", neigh, "nbrPriority")

	if v := neigh["lastPrgrsvChangeMsec"]; v != nil {
		neighbor["infix-routing:uptime"] = toInt(v) / 1000
	}
	secondsAtLeast1(neighbor, "dead-timer", neigh["routerDeadIntervalTimerDueMsec"])

	if s, ok := neigh["nbrState"].(string); ok {
		neighbor["state"] = frrToIETFNeighborState(s)
	}
	if role, ok := neigh["role"].(string); ok && role != "" {
		neighbor["infix-routing:role"] = frrToIETFNeighborRole(role)
	}

	ifName, _ := neigh["ifaceName"].(string)
	localAddr, _ := neigh["localIfaceAddress"].(string)
	if ifName != "" && localAddr != "" {
		neighbor["infix-routing:interface-name"] = ifName + ":" + localAddr
	} else if ifName != "" {
		neighbor["infix-routing:interface-name"] = ifName
	}

	setIfPresent(neighbor, "dr-router-id", neigh, "routerDesignatedId")
	setIfPresent(neighbor, "bdr-router-id", neigh, "routerDesignatedBackupId")

	return neighbor
}

func (c *RoutingCollector) ospfRoutes(ctx context.Context) []interface{} {
	var data map[string]interface{}
	if c.vtyJSON(ctx, "ospfd", "show ip ospf route json", &data) != nil {
		return nil
	}

	var routes []interface{}
	for prefix, infoRaw := range data {
		if !strings.Contains(prefix, "/") {
			continue
		}
		if info, ok := infoRaw.(map[string]interface{}); ok {
			routes = append(routes, ospfRoute(prefix, info))
		}
	}
	return routes
}

func ospfRoute(prefix string, info map[string]interface{}) map[string]interface{} {
	route := map[string]interface{}{
		"prefix": prefix,
	}

	if rt, ok := info["routeType"].(string); ok {
		parts := strings.Fields(rt)
		if len(parts) > 1 {
			switch parts[1] {
			case "E1":
				route["route-type"] = "external-1"
			case "E2":
				route["route-type"] = "external-2"
			case "IA":
				route["route-type"] = "inter-area"
			}
		} else if len(parts) > 0 && parts[0] == "N" {
			route["route-type"] = "intra-area"
		}
	}

	if v := info["area"]; v != nil {
		route["infix-routing:area-id"] = v
	}
	if v := info["cost"]; v != nil {
		route["metric"] = v
	} else if v := info["metric"]; v != nil {
		route["metric"] = v
	}
	if v := info["tag"]; v != nil {
		route["route-tag"] = v
	}

	nexthops := make([]interface{}, 0)
	hopsRaw, _ := info["nexthops"].([]interface{})
	for _, hopRaw := range hopsRaw {
		hop, ok := hopRaw.(map[string]interface{})
		if !ok {
			continue
		}
		nh := make(map[string]interface{})
		ip, _ := hop["ip"].(string)
		if ip != "" && ip != " " {
			nh["next-hop"] = ip
		} else if da, ok := hop["directlyAttachedTo"].(string); ok {
			nh["outgoing-interface"] = da
		}
		nexthops = append(nexthops, nh)
	}
	route["next-hops"] = map[string]interface{}{
		"next-hop": nexthops,
	}

	return route
}

// --- RIP ---

// ripStatusScalars are the single-value lines of 'show ip rip status'.
var ripStatusScalars = []struct {
	re  *regexp.Regexp
	key string
}{
	{regexp.MustCompile(`Sending updates every (\d+) seconds`), "update-interval"},
	{regexp.MustCompile(`Timeout after (\d+) seconds`), "invalid-interval"},
	{regexp.MustCompile(`garbage collect after (\d+) seconds`), "flush-interval"},
	{regexp.MustCompile(`Default redistribution metric is (\d+)`), "default-metric"},
	{regexp.MustCompile(`Distance: \(default is (\d+)\)`), "distance"},
}

// ripVersion maps FRR's ri_version_msg to the infix-routing enum.
var ripVersion = map[string]string{
	"1":   "1",
	"2":   "2",
	"1 2": "1-2",
}

func (c *RoutingCollector) collectRIP(ctx context.Context) interface{} {
	statusOut, err := c.vty(ctx, "ripd", "show ip rip status")
	if err != nil || len(statusOut) == 0 {
		return nil
	}

	status := parseRIPStatus(string(statusOut))
	if len(status) == 0 {
		return nil
	}

	rip := make(map[string]interface{})
	setIfPresent(rip, "distance", status, "distance")
	setIfPresent(rip, "default-metric", status, "default-metric")

	timers := make(map[string]interface{})
	for _, key := range []string{"update-interval", "invalid-interval", "flush-interval"} {
		setIfPresent(timers, key, status, key)
	}
	if len(timers) > 0 {
		rip["timers"] = timers
	}

	if ifaces := ripInterfaces(status); len(ifaces) > 0 {
		rip["interfaces"] = map[string]interface{}{
			"interface": ifaces,
		}
	}

	ipv4 := make(map[string]interface{})
	if routes := c.ripRoutes(ctx); len(routes) > 0 {
		ipv4["routes"] = map[string]interface{}{
			"route": routes,
		}
		rip["num-of-routes"] = len(routes)
	}
	if neighbors := ripNeighbors(status); len(neighbors) > 0 {
		ipv4["neighbors"] = map[string]interface{}{
			"neighbor": neighbors,
		}
	}
	if len(ipv4) > 0 {
		rip["ipv4"] = ipv4
	}

	return map[string]interface{}{
		"type":         "infix-routing:ripv2",
		"name":         "default",
		"ietf-rip:rip": rip,
	}
}

func ripInterfaces(status map[string]interface{}) []interface{} {
	var out []interface{}
	ifaces, _ := status["interfaces"].([]interface{})
	for _, ifRaw := range ifaces {
		ifData, ok := ifRaw.(map[string]interface{})
		if !ok {
			continue
		}
		entry := map[string]interface{}{
			"interface":   ifData["name"],
			"oper-status": "up",
		}
		setIfPresent(entry, "send-version", ifData, "send-version")
		setIfPresent(entry, "receive-version", ifData, "recv-version")
		out = append(out, entry)
	}
	return out
}

func ripNeighbors(status map[string]interface{}) []interface{} {
	var out []interface{}
	neighs, _ := status["neighbors"].([]interface{})
	for _, nRaw := range neighs {
		nd, ok := nRaw.(map[string]interface{})
		if !ok {
			continue
		}
		entry := map[string]interface{}{
			"ipv4-address": nd["address"],
		}
		setIfPresent(entry, "bad-packets-rcvd", nd, "bad-packets")
		setIfPresent(entry, "bad-routes-rcvd", nd, "bad-routes")
		out = append(out, entry)
	}
	return out
}

func (c *RoutingCollector) ripRoutes(ctx context.Context) []interface{} {
	var routeData map[string]interface{}
	if c.vtyJSON(ctx, "zebra", "show ip route rip json", &routeData) != nil {
		return nil
	}

	var routes []interface{}
	for prefix, entriesRaw := range routeData {
		if !strings.Contains(prefix, "/") {
			continue
		}
		entries, _ := entriesRaw.([]interface{})
		if len(entries) == 0 {
			continue
		}
		entry, ok := entries[0].(map[string]interface{})
		if !ok {
			continue
		}

		route := map[string]interface{}{
			"ipv4-prefix": prefix,
			"route-type":  "rip",
		}
		if m, ok := entry["metric"]; ok {
			route["metric"] = toInt(m)
		}

		nexthops, _ := entry["nexthops"].([]interface{})
		if len(nexthops) > 0 {
			firstHop, _ := nexthops[0].(map[string]interface{})
			if ip, ok := firstHop["ip"].(string); ok && ip != "" {
				route["next-hop"] = ip
			}
			if ifName, ok := firstHop["interfaceName"].(string); ok && ifName != "" {
				route["interface"] = ifName
			}
		}
		routes = append(routes, route)
	}
	return routes
}

// parseRIPStatus parses the text output of 'show ip rip status'.
func parseRIPStatus(text string) map[string]interface{} {
	status := make(map[string]interface{})

	for _, sc := range ripStatusScalars {
		if m := sc.re.FindStringSubmatch(text); m != nil {
			v, _ := strconv.Atoi(m[1])
			status[sc.key] = v
		}
	}

	lines := strings.Split(text, "\n")
	if interfaces := parseRIPInterfaces(lines); len(interfaces) > 0 {
		status["interfaces"] = interfaces
	}
	if neighbors := parseRIPNeighbors(lines); len(neighbors) > 0 {
		status["neighbors"] = neighbors
	}

	return status
}

// parseRIPInterfaces reads the interface table.  ripd prints it with
// "%-17s%-3s   %-3s", and a version can be "1 2", so the columns are
// taken by position rather than split on whitespace.
func parseRIPInterfaces(lines []string) []interface{} {
	var interfaces []interface{}
	inTable := false

	for _, raw := range lines {
		line := strings.TrimSpace(raw)
		if strings.HasPrefix(line, "Interface") && strings.Contains(line, "Send") && strings.Contains(line, "Recv") {
			inTable = true
			continue
		}
		if !inTable {
			continue
		}
		if line == "" || strings.HasPrefix(line, "Routing for Networks:") || strings.HasPrefix(line, "Routing Information Sources:") {
			break
		}

		row := strings.TrimLeft(raw, " ")
		name := strings.Fields(row)[0]
		col := len(name)
		if col < 17 {
			col = 17
		}
		send, ok1 := ripVersion[column(row, col, 3)]
		recv, ok2 := ripVersion[column(row, col+6, 3)]
		if !ok1 || !ok2 {
			continue
		}
		interfaces = append(interfaces, map[string]interface{}{
			"name":         name,
			"send-version": send,
			"recv-version": recv,
		})
	}

	return interfaces
}

// parseRIPNeighbors reads the "Routing Information Sources" table.
func parseRIPNeighbors(lines []string) []interface{} {
	var neighbors []interface{}
	inTable := false

	for _, raw := range lines {
		line := strings.TrimSpace(raw)
		if strings.HasPrefix(line, "Routing Information Sources:") {
			inTable = true
			continue
		}
		if !inTable || strings.HasPrefix(line, "Gateway") {
			continue
		}
		if strings.HasPrefix(line, "Distance:") || (line == "" && len(neighbors) > 0) {
			break
		}

		parts := strings.Fields(line)
		if len(parts) < 5 {
			continue
		}
		badPkts, err1 := strconv.Atoi(parts[1])
		badRoutes, err2 := strconv.Atoi(parts[2])
		if err1 != nil || err2 != nil {
			continue
		}
		neighbors = append(neighbors, map[string]interface{}{
			"address":     parts[0],
			"bad-packets": badPkts,
			"bad-routes":  badRoutes,
		})
	}

	return neighbors
}

// column returns the trimmed text in [start, start+width) of s.
func column(s string, start, width int) string {
	if start >= len(s) {
		return ""
	}
	end := start + width
	if end > len(s) {
		end = len(s)
	}
	return strings.TrimSpace(s[start:end])
}

// --- BFD ---

var bfdStateMap = map[string]string{
	"up":        "up",
	"down":      "down",
	"init":      "init",
	"adminDown": "adminDown",
}

func (c *RoutingCollector) collectBFD(ctx context.Context) interface{} {
	var data []interface{}
	if c.vtyJSON(ctx, "bfdd", "show bfd peers json", &data) != nil || len(data) == 0 {
		return nil
	}

	var sessions []interface{}
	for _, peerRaw := range data {
		peer, ok := peerRaw.(map[string]interface{})
		if !ok {
			continue
		}
		// Only process single-hop sessions (multihop == false)
		if mh, _ := peer["multihop"].(bool); mh {
			continue
		}

		session := map[string]interface{}{
			"interface": strDefault(peer["interface"], "unknown"),
			"dest-addr": strDefault(peer["peer"], "0.0.0.0"),
		}

		if v := peer["id"]; v != nil {
			session["local-discriminator"] = v
		}
		if v := peer["remote-id"]; v != nil {
			session["remote-discriminator"] = v
		}

		state := strDefault(peer["status"], "down")
		ietfState := bfdStateMap[state]
		if ietfState == "" {
			ietfState = "down"
		}

		sessionRunning := map[string]interface{}{
			"local-state":      ietfState,
			"remote-state":     ietfState,
			"local-diagnostic": "none",
			"detection-mode":   "async-without-echo",
		}

		if v := peer["receive-interval"]; v != nil {
			sessionRunning["negotiated-rx-interval"] = toInt(v) * 1000
		}
		if v := peer["transmit-interval"]; v != nil {
			sessionRunning["negotiated-tx-interval"] = toInt(v) * 1000
		}
		if dm := peer["detect-multiplier"]; dm != nil {
			if ri := peer["receive-interval"]; ri != nil {
				detectionTimeMs := toInt(dm) * toInt(ri)
				sessionRunning["detection-time"] = detectionTimeMs * 1000
			}
		}

		session["session-running"] = sessionRunning
		session["path-type"] = "ietf-bfd-types:path-ip-sh"
		session["ip-encapsulation"] = true

		sessions = append(sessions, session)
	}

	if len(sessions) == 0 {
		return nil
	}

	return map[string]interface{}{
		"type": "infix-routing:bfdv1",
		"name": "bfd",
		"ietf-bfd:bfd": map[string]interface{}{
			"ietf-bfd-ip-sh:ip-sh": map[string]interface{}{
				"sessions": map[string]interface{}{
					"session": sessions,
				},
			},
		},
	}
}

// --- Helpers ---

func setIfPresent(dst map[string]interface{}, dstKey string, src map[string]interface{}, srcKey string) {
	if v, ok := src[srcKey]; ok && v != nil {
		dst[dstKey] = v
	}
}

func setIfPresentInt(dst map[string]interface{}, dstKey string, src map[string]interface{}, srcKey string) {
	if v, ok := src[srcKey]; ok && v != nil {
		dst[dstKey] = toInt(v)
	}
}

func strDefault(v interface{}, def string) string {
	if s, ok := v.(string); ok && s != "" {
		return s
	}
	return def
}
