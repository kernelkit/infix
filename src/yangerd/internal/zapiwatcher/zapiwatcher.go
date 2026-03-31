// Package zapiwatcher subscribes to FRR zebra zserv route redistribution
// events and mirrors routing data into the operational YANG tree.
package zapiwatcher

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"math"
	"net"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/kernelkit/infix/src/yangerd/internal/tree"
	"github.com/osrg/gobgp/v3/pkg/log"
	"github.com/osrg/gobgp/v3/pkg/zebra"
)

const (
	zapiSocketPath = "/var/run/frr/zserv.api"
	zapiVersion    = 6
	zapiSoftware   = "frr10.5"

	reconnectInitial = 100 * time.Millisecond
	reconnectMax     = 30 * time.Second
	reconnectFactor  = 2.0
)

const (
	RouteKernel  zebra.RouteType = 1
	RouteConnect zebra.RouteType = 2
	RouteStatic  zebra.RouteType = 3
	RouteRIP     zebra.RouteType = 4
	RouteOSPF    zebra.RouteType = 6
)

var subscribeTypes = []zebra.RouteType{
	RouteKernel,
	RouteConnect,
	RouteStatic,
	RouteRIP,
	RouteOSPF,
}

var routeTypeToProtocol = map[zebra.RouteType]string{
	RouteKernel:  "infix-routing:kernel",
	RouteConnect: "ietf-routing:direct",
	RouteStatic:  "ietf-routing:static",
	RouteOSPF:    "ietf-ospf:ospfv2",
	RouteRIP:     "ietf-rip:ripv2",
}

// ZAPIWatcher keeps the routing subtree in sync with zebra route updates.
type ZAPIWatcher struct {
	tree   *tree.Tree
	log    *slog.Logger
	mu     sync.Mutex
	routes map[string]json.RawMessage
}

const routingTreeKey = "ietf-routing:routing"

// slogAdapter wraps slog.Logger to implement gobgp v3 log.Logger interface.
type slogAdapter struct {
	l *slog.Logger
}

func (a *slogAdapter) Panic(msg string, fields log.Fields) { a.l.Error(msg, toAttrs(fields)...) }
func (a *slogAdapter) Fatal(msg string, fields log.Fields) { a.l.Error(msg, toAttrs(fields)...) }
func (a *slogAdapter) Error(msg string, fields log.Fields) { a.l.Error(msg, toAttrs(fields)...) }
func (a *slogAdapter) Warn(msg string, fields log.Fields)  { a.l.Warn(msg, toAttrs(fields)...) }
func (a *slogAdapter) Info(msg string, fields log.Fields)  { a.l.Info(msg, toAttrs(fields)...) }
func (a *slogAdapter) Debug(msg string, fields log.Fields) { a.l.Debug(msg, toAttrs(fields)...) }
func (a *slogAdapter) SetLevel(level log.LogLevel)         {}
func (a *slogAdapter) GetLevel() log.LogLevel              { return log.LogLevel(0) }

func toAttrs(fields log.Fields) []any {
	attrs := make([]any, 0, len(fields)*2)
	for k, v := range fields {
		attrs = append(attrs, k, v)
	}
	return attrs
}

// New creates a ZAPIWatcher bound to the provided operational tree and logger.
func New(t *tree.Tree, log *slog.Logger) *ZAPIWatcher {
	if log == nil {
		log = slog.Default()
	}
	return &ZAPIWatcher{tree: t, log: log, routes: make(map[string]json.RawMessage)}
}

// Run starts the zebra watcher loop with exponential reconnect backoff.
//
// On successful connection it processes incoming route messages until
// disconnect or context cancellation. On disconnect, cached routes are cleared.
func (w *ZAPIWatcher) Run(ctx context.Context) error {
	delay := reconnectInitial

	for {
		cli, err := w.connect(ctx)
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
		w.log.Info("zapi watcher: connected", "socket", zapiSocketPath, "version", zapiVersion, "software", zapiSoftware)

		err = w.processMessages(ctx, cli)
		if ctx.Err() != nil {
			return ctx.Err()
		}

		w.log.Warn("zapi watcher: disconnected", "err", err)
		w.clearAllRoutes()
	}
}

func (w *ZAPIWatcher) connect(ctx context.Context) (*zebra.Client, error) {
	d := net.Dialer{}
	conn, err := d.DialContext(ctx, "unix", zapiSocketPath)
	if err != nil {
		return nil, fmt.Errorf("dial zserv socket: %w", err)
	}
	_ = conn.Close()

	software := zebra.NewSoftware(zapiVersion, zapiSoftware)
	zebra.MaxSoftware = software

	cli, err := zebra.NewClient(&slogAdapter{l: w.log}, "unix", zapiSocketPath, zebra.RouteType(0), zapiVersion, software, 0)
	if err != nil {
		return nil, fmt.Errorf("new zapi client: %w", err)
	}

	cli.SendHello()
	cli.SendRouterIDAdd()
	for _, typ := range subscribeTypes {
		cli.SendRedistribute(typ, zebra.DefaultVrf)
	}

	return cli, nil
}

func (w *ZAPIWatcher) processMessages(ctx context.Context, cli *zebra.Client) error {
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case msg, ok := <-cli.Receive():
			if !ok {
				return errors.New("zapi receive channel closed")
			}
			if msg == nil {
				continue
			}
			w.handleMessage(msg)
		}
	}
}

func (w *ZAPIWatcher) handleMessage(msg *zebra.Message) {
	ipr, ok := msg.Body.(*zebra.IPRouteBody)
	if !ok || ipr == nil {
		return
	}

	cmd := msg.Header.Command.ToCommon(zapiVersion, zebra.NewSoftware(zapiVersion, zapiSoftware))
	switch cmd {
	case zebra.RedistributeRouteAdd:
		w.addRoute(ipr)
	case zebra.RedistributeRouteDel:
		w.deleteRoute(ipr)
	}
}

func (w *ZAPIWatcher) addRoute(route *zebra.IPRouteBody) {
	pfx, ok := ipNetFromPrefix(route.Prefix)
	if !ok {
		w.log.Debug("zapi watcher: ignore route add with invalid prefix")
		return
	}
	rib := ribName(pfx)
	key := rib + ":" + pfx.String() + ":" + routeProtocol(route.Type)

	w.mu.Lock()
	w.routes[key] = transformRoute(route)
	w.mu.Unlock()

	w.writeRibs()
}

func (w *ZAPIWatcher) deleteRoute(route *zebra.IPRouteBody) {
	pfx, ok := ipNetFromPrefix(route.Prefix)
	if !ok {
		return
	}
	rib := ribName(pfx)
	key := rib + ":" + pfx.String() + ":" + routeProtocol(route.Type)

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
		if strings.HasPrefix(key, "ipv4-master:") {
			ipv4Routes = append(ipv4Routes, routeData)
		} else {
			ipv6Routes = append(ipv6Routes, routeData)
		}
	}
	w.mu.Unlock()

	ribs := map[string]any{
		"rib": []map[string]any{
			{
				"name":           "ipv4-master",
				"address-family": "ietf-routing:ipv4",
				"routes": map[string]any{
					"route": ipv4Routes,
				},
			},
			{
				"name":           "ipv6-master",
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
		return "ipv4-master"
	}
	return "ipv6-master"
}

func transformRoute(route *zebra.IPRouteBody) json.RawMessage {
	pfx, ok := ipNetFromPrefix(route.Prefix)
	if !ok {
		return json.RawMessage(`{}`)
	}

	nextHops := make([]map[string]any, 0, len(route.Nexthops))
	for _, nh := range route.Nexthops {
		hop := make(map[string]any)

		if len(nh.Gate) > 0 && !nh.Gate.IsUnspecified() {
			hop["next-hop-address"] = nh.Gate.String()
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
		"destination-prefix": pfx.String(),
		"source-protocol":    routeProtocol(route.Type),
		"metric":             route.Metric,
		"next-hop-list": map[string]any{
			"next-hop": nextHops,
		},
	}

	encoded, err := json.Marshal(routeNode)
	if err != nil {
		return json.RawMessage(`{}`)
	}

	return json.RawMessage(encoded)
}

func routeProtocol(rt zebra.RouteType) string {
	if protocol, ok := routeTypeToProtocol[rt]; ok {
		return protocol
	}
	return "infix-routing:kernel"
}

func ipNetFromPrefix(prefix zebra.Prefix) (net.IPNet, bool) {
	ip := prefix.Prefix
	if len(ip) == 0 {
		return net.IPNet{}, false
	}

	bits := 128
	if v4 := ip.To4(); v4 != nil {
		ip = v4
		bits = 32
	}

	prefixLen := int(prefix.PrefixLen)
	if prefixLen < 0 {
		prefixLen = 0
	}
	if prefixLen > bits {
		prefixLen = bits
	}

	mask := net.CIDRMask(prefixLen, bits)
	return net.IPNet{IP: ip.Mask(mask), Mask: mask}, true
}
