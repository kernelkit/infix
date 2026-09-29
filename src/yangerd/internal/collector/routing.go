package collector

import (
	"context"
	"encoding/json"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/kernelkit/infix/src/yangerd/internal/tree"
)

// RoutingCollector gathers ietf-routing operational data by merging
// OSPF, RIP, and BFD control-plane protocols into a single tree key.
// Each protocol contributes entries to the control-plane-protocol list
// under ietf-routing:routing.
type RoutingCollector struct {
	cmd      CommandRunner
	interval time.Duration
}

// NewRoutingCollector creates a RoutingCollector with the given dependencies.
func NewRoutingCollector(cmd CommandRunner, interval time.Duration) *RoutingCollector {
	return &RoutingCollector{cmd: cmd, interval: interval}
}

// Name implements Collector.
func (c *RoutingCollector) Name() string { return "routing" }

// Interval implements Collector.
func (c *RoutingCollector) Interval() time.Duration { return c.interval }

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

func (c *RoutingCollector) collectOSPF(ctx context.Context) interface{} {
	out, err := c.cmd.Run(ctx, "/usr/libexec/statd/ospf-status")
	if err != nil {
		return nil
	}

	var data map[string]interface{}
	if json.Unmarshal(out, &data) != nil || len(data) == 0 {
		return nil
	}

	ospf := map[string]interface{}{
		"ietf-ospf:areas": map[string]interface{}{},
	}

	if rid, ok := data["routerId"]; ok {
		ospf["ietf-ospf:router-id"] = rid
	}
	ospf["ietf-ospf:address-family"] = "ipv4"

	areas := make([]interface{}, 0)
	areasRaw, _ := data["areas"].(map[string]interface{})
	for areaID, valRaw := range areasRaw {
		values, ok := valRaw.(map[string]interface{})
		if !ok {
			continue
		}

		area := map[string]interface{}{
			"ietf-ospf:area-id":    areaID,
			"ietf-ospf:interfaces": map[string]interface{}{},
		}
		if at, ok := values["area-type"]; ok && at != nil {
			area["ietf-ospf:area-type"] = at
		}

		interfaces := make([]interface{}, 0)
		ifacesRaw, _ := values["interfaces"].([]interface{})
		for _, ifaceRaw := range ifacesRaw {
			iface, ok := ifaceRaw.(map[string]interface{})
			if !ok {
				continue
			}

			intf := map[string]interface{}{
				"name":                iface["name"],
				"ietf-ospf:neighbors": map[string]interface{}{},
			}

			setIfPresent(intf, "dr-router-id", iface, "drId")
			setIfPresent(intf, "dr-ip-addr", iface, "drAddress")
			setIfPresent(intf, "bdr-router-id", iface, "bdrId")
			setIfPresent(intf, "bdr-ip-addr", iface, "bdrAddress")

			if v, ok := iface["timerPassiveIface"]; ok && v != nil {
				intf["passive"] = true
			} else {
				intf["passive"] = false
			}

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

			// Hello interval: milliseconds to seconds
			if v := iface["timerMsecs"]; v != nil {
				helloSec := toInt(v) / 1000
				if helloSec >= 1 {
					intf["hello-interval"] = helloSec
				}
			}

			// Hello timer: remaining time in ms to seconds
			if v := iface["timerHelloInMsecs"]; v != nil {
				helloTimerSec := toInt(v) / 1000
				if helloTimerSec >= 1 {
					intf["hello-timer"] = helloTimerSec
				}
			}

			// Wait timer
			if v := iface["timerWaitSecs"]; v != nil {
				waitSec := toInt(v)
				if waitSec >= 1 {
					intf["wait-timer"] = waitSec
				}
			}

			neighbors := make([]interface{}, 0)
			neighsRaw, _ := iface["neighbors"].([]interface{})
			for _, neighRaw := range neighsRaw {
				neigh, ok := neighRaw.(map[string]interface{})
				if !ok {
					continue
				}

				neighbor := map[string]interface{}{
					"neighbor-router-id": neigh["neighborIp"],
					"address":            neigh["ifaceAddress"],
				}

				setIfPresentInt(neighbor, "priority", neigh, "nbrPriority")

				// Uptime: ms to seconds (infix augmentation)
				if v := neigh["lastPrgrsvChangeMsec"]; v != nil {
					neighbor["infix-routing:uptime"] = toInt(v) / 1000
				}

				// Dead timer: ms to seconds
				if v := neigh["routerDeadIntervalTimerDueMsec"]; v != nil {
					deadSec := toInt(v) / 1000
					if deadSec >= 1 {
						neighbor["dead-timer"] = deadSec
					}
				}

				if s, ok := neigh["nbrState"].(string); ok {
					neighbor["state"] = frrToIETFNeighborState(s)
				}

				if role, ok := neigh["role"].(string); ok && role != "" {
					neighbor["infix-routing:role"] = frrToIETFNeighborRole(role)
				}

				// Interface name (infix augmentation)
				ifName, _ := neigh["ifaceName"].(string)
				localAddr, _ := neigh["localIfaceAddress"].(string)
				if ifName != "" && localAddr != "" {
					neighbor["infix-routing:interface-name"] = ifName + ":" + localAddr
				} else if ifName != "" {
					neighbor["infix-routing:interface-name"] = ifName
				}

				setIfPresent(neighbor, "dr-router-id", neigh, "routerDesignatedId")
				setIfPresent(neighbor, "bdr-router-id", neigh, "routerDesignatedBackupId")

				neighbors = append(neighbors, neighbor)
			}

			intf["ietf-ospf:neighbors"] = map[string]interface{}{
				"ietf-ospf:neighbor": neighbors,
			}
			interfaces = append(interfaces, intf)
		}

		area["ietf-ospf:interfaces"] = map[string]interface{}{
			"ietf-ospf:interface": interfaces,
		}
		areas = append(areas, area)
	}

	// Add routes
	c.addOSPFRoutes(ctx, ospf)

	ospf["ietf-ospf:areas"] = map[string]interface{}{
		"ietf-ospf:area": areas,
	}

	return map[string]interface{}{
		"type":           "infix-routing:ospfv2",
		"name":           "default",
		"ietf-ospf:ospf": ospf,
	}
}

func (c *RoutingCollector) addOSPFRoutes(ctx context.Context, ospf map[string]interface{}) {
	out, err := c.cmd.Run(ctx, "vtysh", "-c", "show ip ospf route json")
	if err != nil {
		return
	}

	var data map[string]interface{}
	if json.Unmarshal(out, &data) != nil {
		return
	}

	var routes []interface{}
	for prefix, infoRaw := range data {
		if !strings.Contains(prefix, "/") {
			continue
		}

		info, ok := infoRaw.(map[string]interface{})
		if !ok {
			continue
		}

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
		routes = append(routes, route)
	}

	if len(routes) > 0 {
		ospf["ietf-ospf:local-rib"] = map[string]interface{}{
			"ietf-ospf:route": routes,
		}
	}
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
	statusOut, err := c.cmd.Run(ctx, "vtysh", "-c", "show ip rip status")
	if err != nil {
		return nil
	}
	statusText := string(statusOut)
	if statusText == "" {
		return nil
	}

	status := parseRIPStatus(statusText)
	if len(status) == 0 {
		return nil
	}

	rip := make(map[string]interface{})

	if v, ok := status["distance"]; ok {
		rip["distance"] = v
	}
	if v, ok := status["default-metric"]; ok {
		rip["default-metric"] = v
	}

	timers := make(map[string]interface{})
	if v, ok := status["update-interval"]; ok {
		timers["update-interval"] = v
	}
	if v, ok := status["invalid-interval"]; ok {
		timers["invalid-interval"] = v
	}
	if v, ok := status["flush-interval"]; ok {
		timers["flush-interval"] = v
	}
	if len(timers) > 0 {
		rip["timers"] = timers
	}

	if ifaces, ok := status["interfaces"].([]interface{}); ok && len(ifaces) > 0 {
		var ifaceList []interface{}
		for _, ifRaw := range ifaces {
			ifData, ok := ifRaw.(map[string]interface{})
			if !ok {
				continue
			}
			entry := map[string]interface{}{
				"interface":   ifData["name"],
				"oper-status": "up",
			}
			if sv, ok := ifData["send-version"].(string); ok {
				entry["send-version"] = sv
			}
			if rv, ok := ifData["recv-version"].(string); ok {
				entry["receive-version"] = rv
			}
			ifaceList = append(ifaceList, entry)
		}
		if len(ifaceList) > 0 {
			rip["interfaces"] = map[string]interface{}{
				"interface": ifaceList,
			}
		}
	}

	routeOut, err := c.cmd.Run(ctx, "vtysh", "-c", "show ip route rip json")
	if err == nil {
		var routeData map[string]interface{}
		if json.Unmarshal(routeOut, &routeData) == nil {
			var routes []interface{}
			for prefix, entriesRaw := range routeData {
				if !strings.Contains(prefix, "/") {
					continue
				}
				entries, ok := entriesRaw.([]interface{})
				if !ok || len(entries) == 0 {
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

			if len(routes) > 0 {
				if _, ok := rip["ipv4"]; !ok {
					rip["ipv4"] = make(map[string]interface{})
				}
				rip["ipv4"].(map[string]interface{})["routes"] = map[string]interface{}{
					"route": routes,
				}
				rip["num-of-routes"] = len(routes)
			}
		}
	}

	if neighs, ok := status["neighbors"].([]interface{}); ok && len(neighs) > 0 {
		var neighborList []interface{}
		for _, nRaw := range neighs {
			nd, ok := nRaw.(map[string]interface{})
			if !ok {
				continue
			}
			entry := map[string]interface{}{
				"ipv4-address": nd["address"],
			}
			if v, ok := nd["bad-packets"].(int); ok {
				entry["bad-packets-rcvd"] = v
			}
			if v, ok := nd["bad-routes"].(int); ok {
				entry["bad-routes-rcvd"] = v
			}
			neighborList = append(neighborList, entry)
		}
		if len(neighborList) > 0 {
			if _, ok := rip["ipv4"]; !ok {
				rip["ipv4"] = make(map[string]interface{})
			}
			rip["ipv4"].(map[string]interface{})["neighbors"] = map[string]interface{}{
				"neighbor": neighborList,
			}
		}
	}

	return map[string]interface{}{
		"type":         "infix-routing:ripv2",
		"name":         "default",
		"ietf-rip:rip": rip,
	}
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
	out, err := c.cmd.Run(ctx, "vtysh", "-c", "show bfd peers json")
	if err != nil {
		return nil
	}

	var data []interface{}
	if json.Unmarshal(out, &data) != nil || len(data) == 0 {
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
