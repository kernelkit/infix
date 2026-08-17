package zapiwatcher

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/kernelkit/infix/src/yangerd/internal/backoff"
	"github.com/kernelkit/infix/src/yangerd/internal/tree"
	"github.com/kernelkit/infix/src/yangerd/internal/zapi"
)

const (
	zapiSocketPath = "/var/run/frr/zserv.api"
	routingTreeKey = "ietf-routing:routing"

	// debounceDelay coalesces a burst of ZAPI route notifications into a
	// single RIB read.  FRR emits many RouteAdd/Del messages while a
	// protocol converges; without debouncing we would re-read the table
	// dozens of times in a few milliseconds.
	debounceDelay = 200 * time.Millisecond
)

// subscribeTypes are the route types we ask zebra to redistribute.  We do
// not use the route payloads themselves -- redistribution only ever
// delivers the selected route per destination and does not reliably send
// a delete when a route is superseded.  The subscription exists purely so
// zebra notifies us that *something* changed; the authoritative table is
// then read from zebra's vty socket (see RouteQuerier).
var subscribeTypes = []zapi.RouteType{
	zapi.RouteKernel,
	zapi.RouteConnect,
	zapi.RouteLocal,
	zapi.RouteStatic,
	zapi.RouteRIP,
	zapi.RouteOSPF,
}

// RouteQuerier runs a "show ... json" command against FRR and returns its
// raw output.  Production code uses an frrvty.Client (zebra's vty socket);
// tests inject a fake.
type RouteQuerier interface {
	Query(ctx context.Context, command string) ([]byte, error)
}

// ZAPIWatcher keeps the operational RIB (ietf-routing:routing/ribs) in
// sync with FRR.  It does NOT reconstruct routes from the ZAPI stream:
// the ZAPI socket is used only as a change trigger, and the full routing
// table -- every candidate per destination, with FRR's own
// selected/installed flags, exactly as "show ip route" renders it -- is
// read from zebra's vty socket on each change.  Because every refresh is
// a complete snapshot, a route removed from zebra simply disappears; we
// never depend on receiving a ZAPI delete.
type ZAPIWatcher struct {
	tree    *tree.Tree
	querier RouteQuerier
	log     *slog.Logger
	refresh chan struct{}
}

func New(t *tree.Tree, querier RouteQuerier, log *slog.Logger) *ZAPIWatcher {
	if log == nil {
		log = slog.Default()
	}
	return &ZAPIWatcher{
		tree:    t,
		querier: querier,
		log:     log,
		refresh: make(chan struct{}, 1),
	}
}

func (w *ZAPIWatcher) Run(ctx context.Context) error {
	// The refresh worker owns all writes to the tree and runs for the
	// lifetime of the watcher, independent of the ZAPI connection.
	go w.refreshLoop(ctx)

	bo := backoff.Default()
	delay := bo.Initial

	for {
		conn, err := w.connect(ctx)
		if err != nil {
			if ctx.Err() != nil {
				return ctx.Err()
			}

			w.log.Warn("zapi watcher: connect failed", "err", err, "delay", delay)
			if err := backoff.Sleep(ctx, delay); err != nil {
				return err
			}
			delay = bo.Next(delay)
			continue
		}

		delay = bo.Initial
		w.log.Info("zapi watcher: connected", "socket", zapiSocketPath)

		// Read the current table now that we are subscribed, so we have
		// data even if no further events arrive.
		w.triggerRefresh()

		err = w.processMessages(ctx, conn)
		_ = conn.Close()
		if ctx.Err() != nil {
			return ctx.Err()
		}

		w.log.Warn("zapi watcher: disconnected", "err", err)
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

// processMessages drains the ZAPI stream.  We only care *that* a route
// changed, not what changed -- each route add/delete triggers a debounced
// re-read of the full table.
func (w *ZAPIWatcher) processMessages(ctx context.Context, conn net.Conn) error {
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		hdr, _, err := zapi.ReadMessage(conn)
		if err != nil {
			return fmt.Errorf("read message: %w", err)
		}

		switch hdr.Command {
		case zapi.CmdRedistRouteAdd, zapi.CmdRedistRouteDel:
			w.log.Debug("zapi watcher: route change", "cmd", hdr.Command, "vrf", hdr.VrfID)
			w.triggerRefresh()
		}
	}
}

// triggerRefresh requests a table re-read.  The buffered channel collapses
// multiple pending requests into one.
func (w *ZAPIWatcher) triggerRefresh() {
	select {
	case w.refresh <- struct{}{}:
	default:
	}
}

func (w *ZAPIWatcher) refreshLoop(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case <-w.refresh:
		}

		// Let a burst of notifications settle before reading.
		select {
		case <-ctx.Done():
			return
		case <-time.After(debounceDelay):
		}
		// Drain a request that arrived during the debounce window; the
		// upcoming read already reflects it.
		select {
		case <-w.refresh:
		default:
		}

		w.writeRibs(ctx)
	}
}

// writeRibs reads the full IPv4 and IPv6 routing tables from zebra and
// replaces the ribs subtree.  On a query error it leaves the previous
// data untouched rather than blanking the table.
func (w *ZAPIWatcher) writeRibs(ctx context.Context) {
	ipv4, err := w.collectRoutes(ctx, "ipv4")
	if err != nil {
		w.log.Warn("zapi watcher: read ipv4 routes", "err", err)
		return
	}
	ipv6, err := w.collectRoutes(ctx, "ipv6")
	if err != nil {
		w.log.Warn("zapi watcher: read ipv6 routes", "err", err)
		return
	}

	ribs := map[string]any{
		"rib": []map[string]any{
			{
				"name":           "ipv4",
				"address-family": "ietf-routing:ipv4",
				"routes":         map[string]any{"route": ipv4},
			},
			{
				"name":           "ipv6",
				"address-family": "ietf-routing:ipv6",
				"routes":         map[string]any{"route": ipv6},
			},
		},
	}

	data, err := json.Marshal(map[string]any{"ribs": ribs})
	if err != nil {
		w.log.Error("zapi watcher: marshal ribs", "err", err)
		return
	}

	w.tree.Merge(routingTreeKey, data)
}

// collectRoutes runs "show ip route json" / "show ipv6 route json" and
// transforms every entry into an ietf-routing route node.
func (w *ZAPIWatcher) collectRoutes(ctx context.Context, family string) ([]json.RawMessage, error) {
	command := "show ip route json"
	if family == "ipv6" {
		command = "show ipv6 route json"
	}

	out, err := w.querier.Query(ctx, command)
	if err != nil {
		return nil, err
	}

	// FRR prints "{}" for an empty table; otherwise a map of
	// prefix -> [route, ...] (multiple candidates per prefix).
	var table map[string][]map[string]any
	if err := json.Unmarshal(out, &table); err != nil {
		return nil, fmt.Errorf("parse %q: %w", command, err)
	}

	now := time.Now()
	routes := make([]json.RawMessage, 0, len(table))
	for prefix, entries := range table {
		if !strings.Contains(prefix, "/") {
			continue
		}
		for _, entry := range entries {
			routes = append(routes, transformRoute(family, prefix, entry, now))
		}
	}
	return routes, nil
}

// protocolMap maps FRR's protocol names to IETF routing-protocol
// identities.  Unknown protocols fall back to kernel so they still
// validate against the model.
var protocolMap = map[string]string{
	"kernel":    "infix-routing:kernel",
	"connected": "ietf-routing:direct",
	"local":     "ietf-routing:direct",
	"static":    "ietf-routing:static",
	"ospf":      "ietf-ospf:ospfv2",
	"ospf6":     "ietf-ospf:ospfv3",
	"rip":       "ietf-rip:rip",
	"ripng":     "ietf-rip:rip",
}

func protocolName(frr string) string {
	if p, ok := protocolMap[frr]; ok {
		return p
	}
	return "infix-routing:kernel"
}

// transformRoute converts one FRR JSON route entry into an ietf-routing
// route node.  It mirrors the legacy yanger ietf_routing.py:add_protocol.
func transformRoute(family, prefixKey string, route map[string]any, now time.Time) json.RawMessage {
	addrKey := "ietf-ipv4-unicast-routing:address"
	dpKey := "ietf-ipv4-unicast-routing:destination-prefix"
	nhAddrKey := "ietf-ipv4-unicast-routing:next-hop-address"
	hostLen := "32"
	if family == "ipv6" {
		addrKey = "ietf-ipv6-unicast-routing:address"
		dpKey = "ietf-ipv6-unicast-routing:destination-prefix"
		nhAddrKey = "ietf-ipv6-unicast-routing:next-hop-address"
		hostLen = "128"
	}

	dst := stringField(route, "prefix")
	if dst == "" {
		dst = prefixKey
	}
	if !strings.Contains(dst, "/") {
		plen := hostLen
		if v, ok := route["prefixLen"]; ok {
			plen = strconv.Itoa(toInt(v))
		}
		dst = dst + "/" + plen
	}

	frr := stringField(route, "protocol")

	node := map[string]any{
		dpKey:              dst,
		"source-protocol":  protocolName(frr),
		"route-preference": toInt(route["distance"]),
		"last-updated":     now.Add(-parseUptime(stringField(route, "uptime"))).Format(time.RFC3339),
	}

	// Metric is modelled only for OSPF and RIP routes.
	switch {
	case strings.Contains(frr, "ospf"):
		node["ietf-ospf:metric"] = toInt(route["metric"])
	case strings.Contains(frr, "rip"):
		node["ietf-rip:metric"] = toInt(route["metric"])
	}

	// "selected" is FRR's own best-path decision -- the '>' in
	// "show ip route".  active is a presence leaf, encoded as [null].
	if boolField(route, "selected") {
		node["active"] = []any{nil}
	}

	installed := boolField(route, "installed")

	nextHops := make([]map[string]any, 0)
	if hops, ok := route["nexthops"].([]any); ok {
		for _, h := range hops {
			hop, ok := h.(map[string]any)
			if !ok {
				continue
			}
			nh := map[string]any{}
			if ip := stringField(hop, "ip"); ip != "" {
				nh[addrKey] = ip
			} else if ifn := stringField(hop, "interfaceName"); ifn != "" {
				nh["outgoing-interface"] = ifn
			}
			// zebra marks the nexthop programmed into the FIB with
			// "fib":true (see zebra/zebra_vty.c).
			if installed && boolField(hop, "fib") {
				nh["infix-routing:installed"] = []any{nil}
			}
			if len(nh) > 0 {
				nextHops = append(nextHops, nh)
			}
		}
	}

	if len(nextHops) > 0 {
		node["next-hop"] = map[string]any{
			"next-hop-list": map[string]any{
				"next-hop": nextHops,
			},
		}
	} else {
		nh := map[string]any{}
		switch frr {
		case "blackhole":
			nh["special-next-hop"] = "blackhole"
		case "unreachable":
			nh["special-next-hop"] = "unreachable"
		default:
			if ifn := stringField(route, "interfaceName"); ifn != "" {
				nh["outgoing-interface"] = ifn
			}
			if gw := stringField(route, "nexthop"); gw != "" {
				nh[nhAddrKey] = gw
			}
		}
		node["next-hop"] = nh
	}

	encoded, err := json.Marshal(node)
	if err != nil {
		return json.RawMessage(`{}`)
	}
	return encoded
}

func stringField(m map[string]any, key string) string {
	if s, ok := m[key].(string); ok {
		return s
	}
	return ""
}

func boolField(m map[string]any, key string) bool {
	b, _ := m[key].(bool)
	return b
}

func toInt(v any) int {
	switch n := v.(type) {
	case float64:
		return int(n)
	case int:
		return n
	case int64:
		return int(n)
	case json.Number:
		i, _ := n.Int64()
		return int(i)
	case string:
		i, _ := strconv.Atoi(n)
		return i
	}
	return 0
}

// FRR uptime string formats (frrtime), ported from yanger's
// uptime2datetime: "HH:MM:SS", "XdXXhXXm", "XXwXdXXh".
var (
	uptimeHMS = regexp.MustCompile(`^(\d{2}):(\d{2}):(\d{2})$`)
	uptimeDHM = regexp.MustCompile(`^(\d+)d(\d{2})h(\d{2})m$`)
	uptimeWDH = regexp.MustCompile(`^(\d{2})w(\d)d(\d{2})h$`)
)

// parseUptime converts an FRR uptime string into a duration.  The
// last-updated leaf is then computed as now-uptime.  Unrecognised input
// yields zero (i.e. last-updated == now).
func parseUptime(s string) time.Duration {
	atoi := func(x string) int { n, _ := strconv.Atoi(x); return n }

	if m := uptimeHMS.FindStringSubmatch(s); m != nil {
		return time.Duration(atoi(m[1]))*time.Hour +
			time.Duration(atoi(m[2]))*time.Minute +
			time.Duration(atoi(m[3]))*time.Second
	}
	if m := uptimeDHM.FindStringSubmatch(s); m != nil {
		return time.Duration(atoi(m[1]))*24*time.Hour +
			time.Duration(atoi(m[2]))*time.Hour +
			time.Duration(atoi(m[3]))*time.Minute
	}
	if m := uptimeWDH.FindStringSubmatch(s); m != nil {
		return time.Duration(atoi(m[1]))*7*24*time.Hour +
			time.Duration(atoi(m[2]))*24*time.Hour +
			time.Duration(atoi(m[3]))*time.Hour
	}
	return 0
}
