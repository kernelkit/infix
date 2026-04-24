package zapiwatcher

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"math"
	"net"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/kernelkit/infix/src/yangerd/internal/tree"
	"github.com/kernelkit/infix/src/yangerd/internal/zapi"
)

const (
	zapiSocketPath = "/var/run/frr/zserv.api"

	reconnectInitial = 100 * time.Millisecond
	reconnectMax     = 30 * time.Second
	reconnectFactor  = 2.0
)

var subscribeTypes = []zapi.RouteType{
	zapi.RouteKernel,
	zapi.RouteConnect,
	zapi.RouteLocal,
	zapi.RouteStatic,
	zapi.RouteRIP,
	zapi.RouteOSPF,
}

var routeTypeToProtocol = map[zapi.RouteType]string{
	zapi.RouteKernel:  "infix-routing:kernel",
	zapi.RouteConnect: "ietf-routing:direct",
	zapi.RouteLocal:   "ietf-routing:direct",
	zapi.RouteStatic:  "ietf-routing:static",
	zapi.RouteOSPF:    "ietf-ospf:ospfv2",
	zapi.RouteRIP:     "ietf-rip:rip",
}

// routeEntry pairs a ZAPI route with the wall-clock time it was first
// received.  The timestamp is used for the YANG last-updated leaf.
type routeEntry struct {
	route      *zapi.Route
	receivedAt time.Time
}

type ZAPIWatcher struct {
	tree   *tree.Tree
	log    *slog.Logger
	mu     sync.Mutex
	routes map[string]*routeEntry
}

const routingTreeKey = "ietf-routing:routing"

func New(t *tree.Tree, log *slog.Logger) *ZAPIWatcher {
	if log == nil {
		log = slog.Default()
	}
	return &ZAPIWatcher{tree: t, log: log, routes: make(map[string]*routeEntry)}
}

func (w *ZAPIWatcher) Run(ctx context.Context) error {
	delay := reconnectInitial

	for {
		conn, err := w.connect(ctx)
		if err != nil {
			if ctx.Err() != nil {
				return ctx.Err()
			}

			w.log.Warn("zapi watcher: connect failed", "err", err, "delay", delay)
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(delay):
			}
			delay = time.Duration(math.Min(float64(delay)*reconnectFactor, float64(reconnectMax)))
			continue
		}

		delay = reconnectInitial
		w.log.Info("zapi watcher: connected", "socket", zapiSocketPath)

		err = w.processMessages(ctx, conn)
		_ = conn.Close()
		if ctx.Err() != nil {
			return ctx.Err()
		}

		w.log.Warn("zapi watcher: disconnected", "err", err)
		w.clearAllRoutes()
	}
}

func (w *ZAPIWatcher) connect(ctx context.Context) (net.Conn, error) {
	d := net.Dialer{}
	conn, err := d.DialContext(ctx, "unix", zapiSocketPath)
	if err != nil {
		return nil, fmt.Errorf("dial zserv: %w", err)
	}

	hello := zapi.BuildMessage(zapi.CmdHello, zapi.DefaultVrf, zapi.EncodeHello())
	if _, err := conn.Write(hello); err != nil {
		conn.Close()
		return nil, fmt.Errorf("send hello: %w", err)
	}

	for _, afi := range []uint8{zapi.AFIIPv4, zapi.AFIIPv6} {
		msg := zapi.BuildMessage(zapi.CmdRouterIDAdd, zapi.DefaultVrf, zapi.EncodeRouterIDAdd(afi))
		if _, err := conn.Write(msg); err != nil {
			conn.Close()
			return nil, fmt.Errorf("send router-id-add: %w", err)
		}
	}

	for _, rt := range subscribeTypes {
		for _, afi := range []uint8{zapi.AFIIPv4, zapi.AFIIPv6} {
			msg := zapi.BuildMessage(zapi.CmdRedistributeAdd, zapi.DefaultVrf, zapi.EncodeRedistributeAdd(afi, rt))
			if _, err := conn.Write(msg); err != nil {
				conn.Close()
				return nil, fmt.Errorf("send redistribute-add: %w", err)
			}
			w.log.Debug("zapi watcher: subscribed", "afi", afi, "routeType", rt)
		}
	}

	return conn, nil
}

func (w *ZAPIWatcher) processMessages(ctx context.Context, conn net.Conn) error {
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		hdr, body, err := zapi.ReadMessage(conn)
		if err != nil {
			return fmt.Errorf("read message: %w", err)
		}

		w.handleMessage(hdr, body)
	}
}

func (w *ZAPIWatcher) handleMessage(hdr zapi.Header, body []byte) {
	w.log.Debug("zapi watcher: message", "cmd", hdr.Command, "vrf", hdr.VrfID, "len", hdr.Length)

	switch hdr.Command {
	case zapi.CmdRedistRouteAdd:
		route, err := zapi.DecodeRouteLog(body, w.log)
		if err != nil {
			w.log.Warn("zapi watcher: decode route add", "err", err, "bodyLen", len(body))
			return
		}
		w.log.Debug("zapi watcher: route add",
			"type", route.Type,
			"prefix", route.Prefix.String(),
			"distance", route.Distance,
			"metric", route.Metric,
			"nexthops", len(route.Nexthops),
			"msg", route.Message,
		)
		w.addRoute(route)
	case zapi.CmdRedistRouteDel:
		route, err := zapi.DecodeRoute(body)
		if err != nil {
			w.log.Warn("zapi watcher: decode route del", "err", err, "bodyLen", len(body))
			return
		}
		w.log.Debug("zapi watcher: route del",
			"type", route.Type,
			"prefix", route.Prefix.String(),
		)
		w.deleteRoute(route)
	}
}

func routeKey(route *zapi.Route) string {
	rib := ribName(route.Prefix)
	proto := routeProtocol(route.Type)

	gates := make([]string, 0, len(route.Nexthops))
	for _, nh := range route.Nexthops {
		if len(nh.Gate) > 0 && !nh.Gate.IsUnspecified() {
			gates = append(gates, nh.Gate.String())
		}
	}
	sort.Strings(gates)

	return rib + ":" + route.Prefix.String() + ":" + proto + ":" + strings.Join(gates, ",")
}

func routeKeyPrefix(route *zapi.Route) string {
	return ribName(route.Prefix) + ":" + route.Prefix.String() + ":" + routeProtocol(route.Type) + ":"
}

func (w *ZAPIWatcher) addRoute(route *zapi.Route) {
	key := routeKey(route)
	now := time.Now()

	w.mu.Lock()
	if existing, ok := w.routes[key]; ok {
		now = existing.receivedAt
	}
	w.routes[key] = &routeEntry{route: route, receivedAt: now}
	routeCount := len(w.routes)
	w.mu.Unlock()

	w.log.Debug("zapi watcher: stored route", "key", key, "totalRoutes", routeCount)
	w.writeRibs()
}

func (w *ZAPIWatcher) deleteRoute(route *zapi.Route) {
	key := routeKey(route)

	w.mu.Lock()
	if _, ok := w.routes[key]; ok {
		delete(w.routes, key)
	} else {
		prefix := routeKeyPrefix(route)
		for k := range w.routes {
			if strings.HasPrefix(k, prefix) {
				delete(w.routes, k)
			}
		}
	}
	w.mu.Unlock()

	w.writeRibs()
}

func (w *ZAPIWatcher) clearAllRoutes() {
	w.mu.Lock()
	w.routes = make(map[string]*routeEntry)
	w.mu.Unlock()

	w.writeRibs()
}

// destKey returns the grouping key for active route computation:
// "rib:prefix" — all routes sharing the same destination compete.
func destKey(route *zapi.Route) string {
	return ribName(route.Prefix) + ":" + route.Prefix.String()
}

func (w *ZAPIWatcher) writeRibs() {
	w.mu.Lock()

	bestDist := make(map[string]uint8)
	for _, entry := range w.routes {
		dk := destKey(entry.route)
		if d, ok := bestDist[dk]; !ok || entry.route.Distance < d {
			bestDist[dk] = entry.route.Distance
		}
	}

	// Collect entries into a slice for deterministic ordering.
	allEntries := make([]*routeEntry, 0, len(w.routes))
	for _, entry := range w.routes {
		allEntries = append(allEntries, entry)
	}

	// Sort by destination prefix so routes for the same destination are
	// grouped together.  Within the same prefix, lowest distance first
	// (active route on top).
	sort.Slice(allEntries, func(i, j int) bool {
		a, b := allEntries[i].route, allEntries[j].route
		ap, bp := a.Prefix.String(), b.Prefix.String()
		if ap != bp {
			return ap < bp
		}
		return a.Distance < b.Distance
	})

	ipv4Routes := make([]json.RawMessage, 0, len(allEntries))
	ipv6Routes := make([]json.RawMessage, 0, len(allEntries))

	for _, entry := range allEntries {
		dk := destKey(entry.route)
		active := entry.route.Distance == bestDist[dk]
		routeData := transformRoute(entry.route, active, entry.receivedAt)

		if entry.route.Prefix.IP.To4() != nil {
			ipv4Routes = append(ipv4Routes, routeData)
		} else {
			ipv6Routes = append(ipv6Routes, routeData)
		}
	}
	w.mu.Unlock()

	ribs := map[string]any{
		"rib": []map[string]any{
			{
				"name":           "ipv4",
				"address-family": "ietf-routing:ipv4",
				"routes": map[string]any{
					"route": ipv4Routes,
				},
			},
			{
				"name":           "ipv6",
				"address-family": "ietf-routing:ipv6",
				"routes": map[string]any{
					"route": ipv6Routes,
				},
			},
		},
	}

	ribsJSON, err := json.Marshal(map[string]any{"ribs": ribs})
	if err != nil {
		w.log.Error("zapi watcher: marshal ribs", "err", err)
		return
	}

	w.tree.Merge(routingTreeKey, ribsJSON)
}

func ribName(prefix net.IPNet) string {
	if prefix.IP.To4() != nil {
		return "ipv4"
	}
	return "ipv6"
}

func transformRoute(route *zapi.Route, active bool, receivedAt time.Time) json.RawMessage {
	isIPv4 := route.Prefix.IP.To4() != nil

	addrKey := "ietf-ipv6-unicast-routing:address"
	dpKey := "ietf-ipv6-unicast-routing:destination-prefix"
	if isIPv4 {
		addrKey = "ietf-ipv4-unicast-routing:address"
		dpKey = "ietf-ipv4-unicast-routing:destination-prefix"
	}

	nextHops := make([]map[string]any, 0, len(route.Nexthops))
	for _, nh := range route.Nexthops {
		hop := make(map[string]any)

		if len(nh.Gate) > 0 && !nh.Gate.IsUnspecified() {
			hop[addrKey] = nh.Gate.String()
		}

		if nh.Ifindex > 0 {
			ifi, err := net.InterfaceByIndex(int(nh.Ifindex))
			if err == nil && ifi != nil && ifi.Name != "" {
				hop["outgoing-interface"] = ifi.Name
			} else {
				hop["outgoing-interface"] = strconv.FormatUint(uint64(nh.Ifindex), 10)
			}
		}

		if len(hop) > 0 {
			nextHops = append(nextHops, hop)
		}
	}

	routeNode := map[string]any{
		dpKey:              route.Prefix.String(),
		"source-protocol":  routeProtocol(route.Type),
		"route-preference": route.Distance,
		"last-updated":     receivedAt.Format(time.RFC3339),
		"next-hop": map[string]any{
			"next-hop-list": map[string]any{
				"next-hop": nextHops,
			},
		},
	}

	if active {
		routeNode["active"] = []any{nil}
	}

	encoded, err := json.Marshal(routeNode)
	if err != nil {
		return json.RawMessage(`{}`)
	}

	return json.RawMessage(encoded)
}

func routeProtocol(rt zapi.RouteType) string {
	if protocol, ok := routeTypeToProtocol[rt]; ok {
		return protocol
	}
	return "infix-routing:kernel"
}
