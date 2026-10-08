// Package kernelrib fills the ietf-routing ribs from the kernel FIB over
// rtnetlink.  It is the RIB source for builds without FRR, where netd
// installs static and DHCP routes straight into the kernel.
//
// Route notifications are only a trigger: on each burst the main table
// is re-read in full and the ribs subtree replaced, the same shape the
// zapiwatcher uses with zebra.  Every route in the kernel table is in
// the FIB, so every next-hop is installed, and of the routes to one
// prefix the lowest metric is the active one.  netd installs static
// routes with the configured route preference as metric.
package kernelrib

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"strconv"
	"strings"
	"time"

	"github.com/vishvananda/netlink"
	"golang.org/x/sys/unix"

	"github.com/kernelkit/infix/src/yangerd/internal/backoff"
	"github.com/kernelkit/infix/src/yangerd/internal/tree"
)

const (
	routingTreeKey = "ietf-routing:routing"
	debounceDelay  = 100 * time.Millisecond
)

// Lister returns the routes of one address family from the main table.
type Lister func(family int) ([]netlink.Route, error)

// IfName resolves an interface index to its name, "" when unknown.
type IfName func(index int) string

// Watcher mirrors the kernel main routing table into the ribs subtree.
type Watcher struct {
	tree    *tree.Tree
	log     *slog.Logger
	list    Lister
	ifname  IfName
	now     func() time.Time
	refresh chan struct{}
	seen    map[string]time.Time // first sighting per route, for last-updated
}

// New creates a Watcher reading the kernel table over rtnetlink.
func New(t *tree.Tree, log *slog.Logger) *Watcher {
	if log == nil {
		log = slog.Default()
	}
	return &Watcher{
		tree:    t,
		log:     log,
		list:    listMainTable,
		ifname:  ifNameByIndex,
		now:     time.Now,
		refresh: make(chan struct{}, 1),
		seen:    map[string]time.Time{},
	}
}

func listMainTable(family int) ([]netlink.Route, error) {
	filter := &netlink.Route{Table: unix.RT_TABLE_MAIN}
	return netlink.RouteListFiltered(family, filter, netlink.RT_FILTER_TABLE)
}

func ifNameByIndex(index int) string {
	iface, err := net.InterfaceByIndex(index)
	if err != nil {
		return ""
	}
	return iface.Name
}

// Run subscribes to route changes and keeps the ribs current until ctx
// ends.  A broken subscription is re-established with backoff.
func (w *Watcher) Run(ctx context.Context) error {
	// The refresh worker owns all writes to the tree and runs for the
	// lifetime of the watcher, independent of the subscription.
	go w.refreshLoop(ctx)

	return backoff.Retry(ctx, w.log, "kernel rib", w.session)
}

// session runs one rtnetlink subscription until it fails or ctx ends.
func (w *Watcher) session(ctx context.Context) error {
	sessCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	done := make(chan struct{})
	defer close(done)

	var subErr error
	errorCallback := func(err error) {
		if err == nil {
			return
		}
		subErr = err
		cancel()
	}

	updates := make(chan netlink.RouteUpdate, 256)
	err := netlink.RouteSubscribeWithOptions(updates, done, netlink.RouteSubscribeOptions{
		ErrorCallback:          errorCallback,
		ReceiveBufferSize:      1 << 20,
		ReceiveBufferForceSize: true,
	})
	if err != nil {
		return fmt.Errorf("subscribe route updates: %w", err)
	}

	w.log.Info("kernel rib: subscribed to route changes")

	// Read the current table now that we are subscribed, so we have
	// data even if no further events arrive.
	w.triggerRefresh()

	for {
		select {
		case <-sessCtx.Done():
			if ctx.Err() != nil {
				return ctx.Err()
			}
			if subErr != nil {
				return fmt.Errorf("route subscription: %w", subErr)
			}
			return errors.New("route subscription ended")
		case _, ok := <-updates:
			if !ok {
				return errors.New("route subscription closed")
			}
			w.triggerRefresh()
		}
	}
}

func (w *Watcher) triggerRefresh() {
	select {
	case w.refresh <- struct{}{}:
	default:
	}
}

func (w *Watcher) refreshLoop(ctx context.Context) {
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

		w.writeRibs()
	}
}

// writeRibs reads the IPv4 and IPv6 main tables and replaces the ribs
// subtree.  On a read error it leaves the previous data untouched
// rather than blanking the table.
func (w *Watcher) writeRibs() {
	v4, err := w.list(netlink.FAMILY_V4)
	if err != nil {
		w.log.Warn("kernel rib: read ipv4 routes", "err", err)
		return
	}
	v6, err := w.list(netlink.FAMILY_V6)
	if err != nil {
		w.log.Warn("kernel rib: read ipv6 routes", "err", err)
		return
	}

	now := w.now()
	seen := make(map[string]time.Time, len(v4)+len(v6))
	ipv4 := w.routes("ipv4", v4, now, seen)
	ipv6 := w.routes("ipv6", v6, now, seen)
	w.seen = seen

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
		w.log.Error("kernel rib: marshal ribs", "err", err)
		return
	}

	w.tree.Merge(routingTreeKey, data)
}

// routes converts one family's kernel routes into ietf-routing route
// nodes.  seen collects the first-sighting time of every route kept.
func (w *Watcher) routes(family string, routes []netlink.Route, now time.Time, seen map[string]time.Time) []map[string]any {
	best := map[string]int{}
	for _, r := range routes {
		dst := prefix(family, r.Dst)
		if m, ok := best[dst]; !ok || r.Priority < m {
			best[dst] = r.Priority
		}
	}

	out := make([]map[string]any, 0, len(routes))
	for _, r := range routes {
		node := w.transform(family, r)
		dst := node[destinationKey(family)].(string)
		if r.Priority == best[dst] {
			node["active"] = []any{nil}
		}

		key := routeKey(family, r)
		first, ok := w.seen[key]
		if !ok {
			first = now
		}
		seen[key] = first
		node["last-updated"] = first.Format(time.RFC3339)

		out = append(out, node)
	}
	return out
}

func destinationKey(family string) string {
	return "ietf-" + family + "-unicast-routing:destination-prefix"
}

// prefix renders the destination; a nil destination is the default route.
func prefix(family string, dst *net.IPNet) string {
	if dst == nil {
		if family == "ipv6" {
			return "::/0"
		}
		return "0.0.0.0/0"
	}
	return dst.String()
}

// protocolName maps rtnetlink route protocols to IETF routing-protocol
// identities.  Connected routes are what the kernel installs itself;
// everything not modelled falls back to kernel so it still validates.
func protocolName(p netlink.RouteProtocol) string {
	switch int(p) {
	case unix.RTPROT_KERNEL:
		return "ietf-routing:direct"
	case unix.RTPROT_STATIC:
		return "ietf-routing:static"
	default:
		return "infix-routing:kernel"
	}
}

// transform converts one kernel route into an ietf-routing route node,
// without the active and last-updated leaves which need the whole table.
func (w *Watcher) transform(family string, r netlink.Route) map[string]any {
	addrKey := "ietf-" + family + "-unicast-routing:address"

	node := map[string]any{
		destinationKey(family): prefix(family, r.Dst),
		"source-protocol":      protocolName(r.Protocol),
		"route-preference":     r.Priority,
	}

	switch r.Type {
	case unix.RTN_BLACKHOLE:
		node["next-hop"] = map[string]any{"special-next-hop": "blackhole"}
		return node
	case unix.RTN_UNREACHABLE:
		node["next-hop"] = map[string]any{"special-next-hop": "unreachable"}
		return node
	case unix.RTN_PROHIBIT:
		node["next-hop"] = map[string]any{"special-next-hop": "prohibit"}
		return node
	}

	hops := make([]map[string]any, 0, 1)
	add := func(gw net.IP, index int) {
		hop := map[string]any{"infix-routing:installed": []any{nil}}
		if len(gw) > 0 {
			hop[addrKey] = gw.String()
		} else if name := w.ifname(index); name != "" {
			hop["outgoing-interface"] = name
		} else {
			return
		}
		hops = append(hops, hop)
	}

	if len(r.MultiPath) > 0 {
		for _, nh := range r.MultiPath {
			add(nh.Gw, nh.LinkIndex)
		}
	} else {
		add(r.Gw, r.LinkIndex)
	}

	if len(hops) > 0 {
		node["next-hop"] = map[string]any{
			"next-hop-list": map[string]any{"next-hop": hops},
		}
	}
	return node
}

// routeKey identifies a route across refreshes so its first sighting
// survives: destination, metric, protocol and the set of next-hops.
func routeKey(family string, r netlink.Route) string {
	var sb strings.Builder
	sb.WriteString(family)
	sb.WriteByte('|')
	sb.WriteString(prefix(family, r.Dst))
	sb.WriteByte('|')
	sb.WriteString(strconv.Itoa(r.Priority))
	sb.WriteByte('|')
	sb.WriteString(strconv.Itoa(int(r.Protocol)))
	sb.WriteByte('|')
	sb.WriteString(strconv.Itoa(r.Type))
	if len(r.MultiPath) > 0 {
		for _, nh := range r.MultiPath {
			fmt.Fprintf(&sb, "|%s@%d", nh.Gw, nh.LinkIndex)
		}
	} else {
		fmt.Fprintf(&sb, "|%s@%d", r.Gw, r.LinkIndex)
	}
	return sb.String()
}
