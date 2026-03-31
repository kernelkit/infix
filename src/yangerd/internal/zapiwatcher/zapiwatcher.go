package zapiwatcher

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"math"
	"net"
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
	zapi.RouteStatic,
	zapi.RouteRIP,
	zapi.RouteOSPF,
}

var routeTypeToProtocol = map[zapi.RouteType]string{
	zapi.RouteKernel:  "infix-routing:kernel",
	zapi.RouteConnect: "ietf-routing:direct",
	zapi.RouteStatic:  "ietf-routing:static",
	zapi.RouteOSPF:    "ietf-ospf:ospfv2",
	zapi.RouteRIP:     "ietf-rip:ripv2",
}

type ZAPIWatcher struct {
	tree   *tree.Tree
	log    *slog.Logger
	mu     sync.Mutex
	routes map[string]json.RawMessage
}

const routingTreeKey = "ietf-routing:routing"

func New(t *tree.Tree, log *slog.Logger) *ZAPIWatcher {
	if log == nil {
		log = slog.Default()
	}
	return &ZAPIWatcher{tree: t, log: log, routes: make(map[string]json.RawMessage)}
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
	switch hdr.Command {
	case zapi.CmdRedistRouteAdd:
		route, err := zapi.DecodeRoute(body)
		if err != nil {
			w.log.Warn("zapi watcher: decode route add", "err", err)
			return
		}
		w.addRoute(route)
	case zapi.CmdRedistRouteDel:
		route, err := zapi.DecodeRoute(body)
		if err != nil {
			w.log.Warn("zapi watcher: decode route del", "err", err)
			return
		}
		w.deleteRoute(route)
	}
}

func (w *ZAPIWatcher) addRoute(route *zapi.Route) {
	rib := ribName(route.Prefix)
	key := rib + ":" + route.Prefix.String() + ":" + routeProtocol(route.Type)

	w.mu.Lock()
	w.routes[key] = transformRoute(route)
	w.mu.Unlock()

	w.writeRibs()
}

func (w *ZAPIWatcher) deleteRoute(route *zapi.Route) {
	rib := ribName(route.Prefix)
	key := rib + ":" + route.Prefix.String() + ":" + routeProtocol(route.Type)

	w.mu.Lock()
	delete(w.routes, key)
	w.mu.Unlock()

	w.writeRibs()
}

func (w *ZAPIWatcher) clearAllRoutes() {
	w.mu.Lock()
	w.routes = make(map[string]json.RawMessage)
	w.mu.Unlock()

	w.writeRibs()
}

func (w *ZAPIWatcher) writeRibs() {
	w.mu.Lock()

	ipv4Routes := make([]json.RawMessage, 0)
	ipv6Routes := make([]json.RawMessage, 0)

	for key, routeData := range w.routes {
		if strings.HasPrefix(key, "ipv4:") {
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

func transformRoute(route *zapi.Route) json.RawMessage {
	isIPv4 := route.Prefix.IP.To4() != nil

	addrKey := "ietf-ipv6-unicast-routing:address"
	destKey := "ietf-ipv6-unicast-routing:destination-prefix"
	if isIPv4 {
		addrKey = "ietf-ipv4-unicast-routing:address"
		destKey = "ietf-ipv4-unicast-routing:destination-prefix"
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
		destKey:            route.Prefix.String(),
		"source-protocol":  routeProtocol(route.Type),
		"route-preference": route.Metric,
		"next-hop": map[string]any{
			"next-hop-list": map[string]any{
				"next-hop": nextHops,
			},
		},
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
