// Package dbusmonitor watches D-Bus signals from dnsmasq and firewalld
// and keeps their operational YANG subtrees updated.
package dbusmonitor

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"math"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/godbus/dbus/v5"
	"github.com/kernelkit/infix/src/yangerd/internal/tree"
)

const (
	dnsmasqBusName   = "uk.org.thekelleys.dnsmasq"
	dnsmasqInterface = "uk.org.thekelleys.dnsmasq"
	dnsmasqPath      = "/uk/org/thekelleys/dnsmasq"

	firewalldBusName   = "org.fedoraproject.FirewallD1"
	firewalldInterface = "org.fedoraproject.FirewallD1"
	firewalldPath      = "/org/fedoraproject/FirewallD1"

	dbusInterface = "org.freedesktop.DBus"
	dbusPath      = "/org/freedesktop/DBus"

	dnsmasqLeaseFile = "/var/lib/misc/dnsmasq.leases"

	dhcpTreeKey     = "infix-dhcp-server:dhcp-server"
	firewallTreeKey = "infix-firewall:firewall"

	reconnectInitial = 100 * time.Millisecond
	reconnectMax     = 30 * time.Second
	reconnectFactor  = 2.0
)

// DBusMonitor subscribes to dnsmasq and firewalld D-Bus signals and
// updates the shared operational tree.
type DBusMonitor struct {
	tree *tree.Tree
	log  *slog.Logger
}

// New creates a DBusMonitor.
func New(t *tree.Tree, log *slog.Logger) *DBusMonitor {
	return &DBusMonitor{tree: t, log: log}
}

// Run starts the monitor loop. It connects to the system bus, subscribes
// to relevant signals, loads initial DHCP/firewall data, and reconnects
// with exponential backoff on failures until ctx is cancelled.
func (m *DBusMonitor) Run(ctx context.Context) error {
	delay := reconnectInitial

	for {
		if err := ctx.Err(); err != nil {
			return err
		}

		conn, err := dbus.ConnectSystemBus()
		if err != nil {
			m.log.Warn("dbus monitor: connect system bus failed", "err", err, "delay", delay)
			if err := sleepOrDone(ctx, delay); err != nil {
				return err
			}
			delay = nextDelay(delay)
			continue
		}

		if err := m.subscribe(conn); err != nil {
			m.log.Warn("dbus monitor: subscribe failed", "err", err, "delay", delay)
			_ = conn.Close()
			if err := sleepOrDone(ctx, delay); err != nil {
				return err
			}
			delay = nextDelay(delay)
			continue
		}

		delay = reconnectInitial

		if err := m.refreshDHCP(conn); err != nil {
			m.log.Warn("dbus monitor: initial dhcp refresh failed", "err", err)
		}
		if err := m.refreshFirewall(conn); err != nil {
			m.log.Warn("dbus monitor: initial firewall refresh failed", "err", err)
		}

		err = m.processSignals(ctx, conn)
		_ = conn.Close()
		if ctx.Err() != nil {
			return ctx.Err()
		}

		m.log.Warn("dbus monitor: signal loop ended, reconnecting", "err", err, "delay", delay)
		if err := sleepOrDone(ctx, delay); err != nil {
			return err
		}
		delay = nextDelay(delay)
	}
}

func (m *DBusMonitor) subscribe(conn *dbus.Conn) error {
	if err := conn.AddMatchSignal(
		dbus.WithMatchInterface(dnsmasqInterface),
		dbus.WithMatchMember("DHCPLeaseAdded"),
	); err != nil {
		return fmt.Errorf("add dnsmasq DHCPLeaseAdded match: %w", err)
	}

	if err := conn.AddMatchSignal(
		dbus.WithMatchInterface(dnsmasqInterface),
		dbus.WithMatchMember("DHCPLeaseDeleted"),
	); err != nil {
		return fmt.Errorf("add dnsmasq DHCPLeaseDeleted match: %w", err)
	}

	if err := conn.AddMatchSignal(
		dbus.WithMatchInterface(dnsmasqInterface),
		dbus.WithMatchMember("DHCPLeaseUpdated"),
	); err != nil {
		return fmt.Errorf("add dnsmasq DHCPLeaseUpdated match: %w", err)
	}

	if err := conn.AddMatchSignal(
		dbus.WithMatchInterface(firewalldInterface),
		dbus.WithMatchMember("Reloaded"),
	); err != nil {
		return fmt.Errorf("add firewalld Reloaded match: %w", err)
	}

	if err := conn.AddMatchSignal(
		dbus.WithMatchInterface(dbusInterface),
		dbus.WithMatchMember("NameOwnerChanged"),
		dbus.WithMatchArg(0, dnsmasqBusName),
	); err != nil {
		return fmt.Errorf("add NameOwnerChanged dnsmasq match: %w", err)
	}

	if err := conn.AddMatchSignal(
		dbus.WithMatchInterface(dbusInterface),
		dbus.WithMatchMember("NameOwnerChanged"),
		dbus.WithMatchArg(0, firewalldBusName),
	); err != nil {
		return fmt.Errorf("add NameOwnerChanged firewalld match: %w", err)
	}

	return nil
}

func (m *DBusMonitor) processSignals(ctx context.Context, conn *dbus.Conn) error {
	sigCh := make(chan *dbus.Signal, 128)
	conn.Signal(sigCh)
	defer conn.RemoveSignal(sigCh)

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case sig, ok := <-sigCh:
			if !ok {
				return fmt.Errorf("dbus signal channel closed")
			}
			if sig == nil {
				continue
			}
			if err := m.handleSignal(conn, sig); err != nil {
				m.log.Warn("dbus monitor: failed handling signal", "name", sig.Name, "path", sig.Path, "err", err)
			}
		}
	}
}

func (m *DBusMonitor) handleSignal(conn *dbus.Conn, sig *dbus.Signal) error {
	switch sig.Name {
	case dnsmasqInterface + ".DHCPLeaseAdded",
		dnsmasqInterface + ".DHCPLeaseDeleted",
		dnsmasqInterface + ".DHCPLeaseUpdated":
		if sig.Path != "" && string(sig.Path) != dnsmasqPath {
			return nil
		}
		return m.refreshDHCP(conn)

	case firewalldInterface + ".Reloaded":
		if sig.Path != "" && string(sig.Path) != firewalldPath {
			return nil
		}
		return m.refreshFirewall(conn)

	case dbusInterface + ".NameOwnerChanged":
		if sig.Path != "" && string(sig.Path) != dbusPath {
			return nil
		}
		if len(sig.Body) < 3 {
			return fmt.Errorf("NameOwnerChanged: expected 3 args, got %d", len(sig.Body))
		}

		name, ok1 := sig.Body[0].(string)
		oldOwner, ok2 := sig.Body[1].(string)
		newOwner, ok3 := sig.Body[2].(string)
		if !ok1 || !ok2 || !ok3 {
			return fmt.Errorf("NameOwnerChanged: unexpected arg types")
		}

		switch name {
		case dnsmasqBusName:
			if newOwner == "" {
				m.clearTreeKey(dhcpTreeKey)
				return nil
			}
			if oldOwner == "" {
				return m.refreshDHCP(conn)
			}
		case firewalldBusName:
			if newOwner == "" {
				m.clearTreeKey(firewallTreeKey)
				return nil
			}
			if oldOwner == "" {
				return m.refreshFirewall(conn)
			}
		}
	}

	return nil
}

func (m *DBusMonitor) refreshDHCP(conn *dbus.Conn) error {
	data, err := os.ReadFile(dnsmasqLeaseFile)
	if err != nil {
		m.log.Warn("dbus monitor: read dnsmasq leases failed", "file", dnsmasqLeaseFile, "err", err)
	}

	leases := parseDnsmasqLeases(string(data))
	stats := defaultDHCPStats()

	obj := conn.Object(dnsmasqBusName, dbus.ObjectPath(dnsmasqPath))
	call := obj.Call(dnsmasqInterface+".GetMetrics", 0)
	if call.Err != nil {
		m.log.Warn("dbus monitor: dnsmasq GetMetrics failed", "err", call.Err)
	} else if len(call.Body) > 0 {
		stats = mergeDHCPStats(stats, decodeDHCPMetrics(call.Body[0]))
	}

	m.tree.Set(dhcpTreeKey, buildDHCPTree(leases, stats))
	return nil
}

func (m *DBusMonitor) refreshFirewall(conn *dbus.Conn) error {
	obj := conn.Object(firewalldBusName, dbus.ObjectPath(firewalldPath))

	defaultZone := ""
	if call := obj.Call(firewalldInterface+".getDefaultZone", 0); call.Err != nil {
		m.log.Info("dbus monitor: firewalld not reachable, skipping", "err", call.Err)
		return nil
	} else if err := call.Store(&defaultZone); err != nil {
		m.log.Warn("dbus monitor: firewalld getDefaultZone decode failed", "err", err)
		return nil
	}

	logDenied := ""
	if call := obj.Call(firewalldInterface+".getLogDenied", 0); call.Err != nil {
		m.log.Warn("dbus monitor: firewalld getLogDenied failed", "err", call.Err)
	} else if err := call.Store(&logDenied); err != nil {
		m.log.Warn("dbus monitor: firewalld getLogDenied decode failed", "err", err)
	}

	lockdown := false
	if call := obj.Call(firewalldInterface+".queryPanicMode", 0); call.Err != nil {
		m.log.Warn("dbus monitor: firewalld queryPanicMode failed", "err", call.Err)
	} else if len(call.Body) > 0 {
		lockdown = asBool(call.Body[0])
	}

	zones := m.getFirewallZones(obj)
	policies := m.getFirewallPolicies(obj)
	services := m.getFirewallServices(obj, referencedServices(zones, policies))

	m.tree.Set(firewallTreeKey, buildFirewallTree(defaultZone, logDenied, lockdown, zones, policies, services))
	return nil
}

func (m *DBusMonitor) getFirewallZones(obj dbus.BusObject) []map[string]any {
	active := make(map[string]map[string]any)
	if call := obj.Call(firewalldInterface+".zone.getActiveZones", 0); call.Err != nil {
		m.log.Warn("dbus monitor: firewalld zone.getActiveZones failed", "err", call.Err)
		return nil
	} else if len(call.Body) > 0 {
		active = decodeActiveZones(call.Body[0])
	}

	zones := make([]map[string]any, 0, len(active))
	for name, zoneInfo := range active {
		settings := map[string]any{}
		if call := obj.Call(firewalldInterface+".zone.getZoneSettings2", 0, name); call.Err != nil {
			m.log.Warn("dbus monitor: firewalld zone.getZoneSettings2 failed", "zone", name, "err", call.Err)
			continue
		} else if len(call.Body) > 0 {
			settings = variantMap(call.Body[0])
		}

		zone := map[string]any{
			"name":      name,
			"immutable": hasImmutableTag(getString(settings, "short")),
			"action":    mapZoneTarget(getString(settings, "target")),
		}
		if ifaces := firstStringList(zoneInfo, "interfaces", getStringList(settings, "interfaces")); len(ifaces) > 0 {
			zone["interface"] = ifaces
		}
		if networks := firstStringList(zoneInfo, "sources", getStringList(settings, "sources")); len(networks) > 0 {
			zone["network"] = networks
		}
		if services := getStringList(settings, "services"); len(services) > 0 {
			zone["service"] = services
		}
		if desc := getString(settings, "description"); desc != "" {
			zone["description"] = desc
		}

		if forwards := getForwardPorts(settings); len(forwards) > 0 {
			zone["port-forward"] = forwards
		}

		zones = append(zones, zone)
	}

	return zones
}

func (m *DBusMonitor) getFirewallPolicies(obj dbus.BusObject) []map[string]any {
	var names []string
	if call := obj.Call(firewalldInterface+".policy.getPolicies", 0); call.Err != nil {
		m.log.Warn("dbus monitor: firewalld policy.getPolicies failed", "err", call.Err)
	} else if err := call.Store(&names); err != nil {
		m.log.Warn("dbus monitor: firewalld policy.getPolicies decode failed", "err", err)
	}

	policies := make([]map[string]any, 0, len(names)+1)
	for _, name := range names {
		settings := map[string]any{}
		if call := obj.Call(firewalldInterface+".policy.getPolicySettings", 0, name); call.Err != nil {
			m.log.Warn("dbus monitor: firewalld policy.getPolicySettings failed", "policy", name, "err", call.Err)
			continue
		} else if len(call.Body) > 0 {
			settings = variantMap(call.Body[0])
		}

		policy := map[string]any{
			"name":       name,
			"action":     mapPolicyTarget(getString(settings, "target")),
			"priority":   getInt(settings, "priority", 32767),
			"immutable":  hasImmutableTag(getString(settings, "short")),
			"masquerade": asBool(settings["masquerade"]),
		}
		if ingress := getStringList(settings, "ingress_zones"); len(ingress) > 0 {
			policy["ingress"] = ingress
		}
		if egress := getStringList(settings, "egress_zones"); len(egress) > 0 {
			policy["egress"] = egress
		}
		if desc := getString(settings, "description"); desc != "" {
			policy["description"] = desc
		}
		if services := getStringList(settings, "services"); len(services) > 0 {
			policy["service"] = services
		}
		if custom := parsePolicyCustomFilters(getStringList(settings, "rich_rules")); len(custom) > 0 {
			policy["custom"] = map[string]any{"filter": custom}
		}

		policies = append(policies, policy)
	}

	policies = append(policies, map[string]any{
		"name":        "default-drop",
		"description": "Default deny rule - drops all unmatched traffic",
		"action":      "drop",
		"priority":    32767,
		"ingress":     []string{"ANY"},
		"egress":      []string{"ANY"},
		"immutable":   true,
	})

	return policies
}

func referencedServices(zones, policies []map[string]any) map[string]bool {
	refs := map[string]bool{}
	for _, z := range zones {
		if svcs, ok := z["service"].([]string); ok {
			for _, s := range svcs {
				refs[s] = true
			}
		}
	}
	for _, p := range policies {
		if svcs, ok := p["service"].([]string); ok {
			for _, s := range svcs {
				refs[s] = true
			}
		}
	}
	return refs
}

func (m *DBusMonitor) getFirewallServices(obj dbus.BusObject, wanted map[string]bool) []map[string]any {
	var names []string
	if call := obj.Call(firewalldInterface+".listServices", 0); call.Err != nil {
		m.log.Warn("dbus monitor: firewalld listServices failed", "err", call.Err)
		return nil
	} else if err := call.Store(&names); err != nil {
		m.log.Warn("dbus monitor: firewalld listServices decode failed", "err", err)
		return nil
	}

	services := make([]map[string]any, 0, len(wanted))
	for _, name := range names {
		if !wanted[name] {
			continue
		}

		settings := map[string]any{}
		if call := obj.Call(firewalldInterface+".getServiceSettings2", 0, name); call.Err != nil {
			m.log.Warn("dbus monitor: firewalld getServiceSettings2 failed", "service", name, "err", call.Err)
			continue
		} else if len(call.Body) > 0 {
			settings = variantMap(call.Body[0])
		}

		ports := parseServicePorts(settings)
		if len(ports) == 0 {
			continue
		}

		service := map[string]any{
			"name": name,
			"port": ports,
		}
		if desc := getString(settings, "description"); desc != "" {
			service["description"] = desc
		}

		services = append(services, service)
	}

	return services
}

func (m *DBusMonitor) clearTreeKey(key string) {
	m.tree.Set(key, json.RawMessage(`{}`))
}

func parseDnsmasqLeases(data string) []map[string]any {
	leases := make([]map[string]any, 0)
	for _, line := range strings.Split(data, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		fields := strings.Fields(line)
		if len(fields) != 5 {
			continue
		}

		expires := "never"
		if fields[0] != "0" {
			ts, err := strconv.ParseInt(fields[0], 10, 64)
			if err != nil {
				continue
			}
			expires = time.Unix(ts, 0).UTC().Format(time.RFC3339)
		}

		hostname := ""
		if fields[3] != "*" {
			hostname = fields[3]
		}

		clientID := ""
		if fields[4] != "*" {
			clientID = fields[4]
		}

		leases = append(leases, map[string]any{
			"expires":      expires,
			"address":      fields[2],
			"phys-address": fields[1],
			"hostname":     hostname,
			"client-id":    clientID,
		})
	}

	return leases
}

func buildDHCPTree(leases []map[string]any, stats map[string]any) json.RawMessage {
	root := map[string]any{
		"statistics": stats,
		"leases": map[string]any{
			"lease": leases,
		},
	}
	raw, err := json.Marshal(root)
	if err != nil {
		return json.RawMessage(`{}`)
	}
	return raw
}

func buildFirewallTree(defaultZone, logDenied string, lockdown bool, zones, policies, services []map[string]any) json.RawMessage {
	fw := map[string]any{
		"default":  defaultZone,
		"logging":  logDenied,
		"lockdown": lockdown,
	}
	if len(zones) > 0 {
		fw["zone"] = zones
	}
	if len(policies) > 0 {
		fw["policy"] = policies
	}
	if len(services) > 0 {
		fw["service"] = services
	}

	raw, err := json.Marshal(fw)
	if err != nil {
		return json.RawMessage(`{}`)
	}
	return raw
}

func defaultDHCPStats() map[string]any {
	return map[string]any{
		"out-offers":   uint64(0),
		"out-acks":     uint64(0),
		"out-naks":     uint64(0),
		"in-declines":  uint64(0),
		"in-discovers": uint64(0),
		"in-requests":  uint64(0),
		"in-releases":  uint64(0),
		"in-informs":   uint64(0),
	}
}

func decodeDHCPMetrics(v any) map[string]any {
	metrics := map[string]any{}

	switch raw := v.(type) {
	case map[string]dbus.Variant:
		for k, val := range raw {
			metrics[k] = val.Value()
		}
	case map[string]any:
		for k, val := range raw {
			metrics[k] = val
		}
	}

	return map[string]any{
		"out-offers":   toUint64(metrics["dhcp_offer"]),
		"out-acks":     toUint64(metrics["dhcp_ack"]),
		"out-naks":     toUint64(metrics["dhcp_nak"]),
		"in-declines":  toUint64(metrics["dhcp_decline"]),
		"in-discovers": toUint64(metrics["dhcp_discover"]),
		"in-requests":  toUint64(metrics["dhcp_request"]),
		"in-releases":  toUint64(metrics["dhcp_release"]),
		"in-informs":   toUint64(metrics["dhcp_inform"]),
	}
}

func mergeDHCPStats(base, override map[string]any) map[string]any {
	out := map[string]any{}
	for k, v := range base {
		out[k] = v
	}
	for k, v := range override {
		out[k] = v
	}
	return out
}

func parseServicePorts(settings map[string]any) []map[string]any {
	rawPorts, ok := settings["ports"]
	if !ok {
		return []map[string]any{}
	}

	out := []map[string]any{}
	for _, entry := range toAnySlice(rawPorts) {
		pair := toAnySlice(entry)
		if len(pair) < 2 {
			continue
		}

		portSpec := fmt.Sprint(pair[0])
		proto := fmt.Sprint(pair[1])
		if portSpec == "" || proto == "" {
			continue
		}

		port := map[string]any{"proto": proto}
		if strings.Contains(portSpec, "-") {
			parts := strings.SplitN(portSpec, "-", 2)
			lower, err1 := strconv.Atoi(strings.TrimSpace(parts[0]))
			upper, err2 := strconv.Atoi(strings.TrimSpace(parts[1]))
			if err1 != nil || err2 != nil {
				continue
			}
			port["lower"] = lower
			port["upper"] = upper
		} else {
			lower, err := strconv.Atoi(strings.TrimSpace(portSpec))
			if err != nil {
				continue
			}
			port["lower"] = lower
		}

		out = append(out, port)
	}

	return out
}

func parsePolicyCustomFilters(rules []string) []map[string]any {
	filters := []map[string]any{}
	for _, rule := range rules {
		family := "both"
		if strings.Contains(rule, `family="ipv4"`) {
			family = "ipv4"
		} else if strings.Contains(rule, `family="ipv6"`) {
			family = "ipv6"
		}

		icmpType := ""
		action := ""
		prio := -1

		if idx := strings.Index(rule, "priority="); idx >= 0 {
			prio = parsePriority(rule[idx+len("priority="):])
		}

		if strings.Contains(rule, "icmp-type") && strings.Contains(rule, `name="`) {
			icmpType = parseQuotedName(rule)
			action = "accept"
			if strings.Contains(rule, " drop") {
				action = "drop"
			} else if strings.Contains(rule, " reject") {
				action = "reject"
			}
		} else if strings.Contains(rule, "icmp-block") && strings.Contains(rule, `name="`) {
			icmpType = parseQuotedName(rule)
			action = "reject"
		}

		if icmpType == "" || action == "" {
			continue
		}

		filters = append(filters, map[string]any{
			"name":     "icmp-" + icmpType,
			"priority": prio,
			"family":   family,
			"action":   action,
			"icmp": map[string]any{
				"type": icmpType,
			},
		})
	}

	return filters
}

func getForwardPorts(settings map[string]any) []map[string]any {
	raw, ok := settings["forward_ports"]
	if !ok {
		return nil
	}

	out := []map[string]any{}
	for _, item := range toAnySlice(raw) {
		vals := toAnySlice(item)
		if len(vals) < 4 {
			continue
		}

		portStr := fmt.Sprint(vals[0])
		proto := fmt.Sprint(vals[1])
		toPortStr := strings.TrimSpace(fmt.Sprint(vals[2]))
		toAddr := fmt.Sprint(vals[3])

		if portStr == "" || proto == "" {
			continue
		}

		entry := map[string]any{"proto": proto}
		if strings.Contains(portStr, "-") {
			parts := strings.SplitN(portStr, "-", 2)
			lower, err1 := strconv.Atoi(strings.TrimSpace(parts[0]))
			upper, err2 := strconv.Atoi(strings.TrimSpace(parts[1]))
			if err1 != nil || err2 != nil {
				continue
			}
			entry["lower"] = lower
			entry["upper"] = upper
		} else {
			lower, err := strconv.Atoi(strings.TrimSpace(portStr))
			if err != nil {
				continue
			}
			entry["lower"] = lower
		}

		to := map[string]any{"addr": toAddr}
		if toPortStr != "" && !strings.ContainsAny(toPortStr, ".:") {
			if p, err := strconv.Atoi(toPortStr); err == nil {
				to["port"] = p
			}
		}
		if _, ok := to["port"]; !ok {
			to["port"] = entry["lower"]
		}

		entry["to"] = to
		out = append(out, entry)
	}

	return out
}

func decodeActiveZones(v any) map[string]map[string]any {
	out := map[string]map[string]any{}

	switch m := v.(type) {
	case map[string]map[string]dbus.Variant:
		for zone, data := range m {
			inner := map[string]any{}
			for k, vv := range data {
				inner[k] = vv.Value()
			}
			out[zone] = inner
		}
	case map[string]map[string]any:
		for zone, data := range m {
			out[zone] = data
		}
	case map[string]map[string][]string:
		for zone, data := range m {
			inner := map[string]any{}
			for k, v := range data {
				inner[k] = v
			}
			out[zone] = inner
		}
	case map[string]any:
		for zone, raw := range m {
			if mm, ok := raw.(map[string]any); ok {
				out[zone] = mm
			}
		}
	}

	return out
}

func variantMap(v any) map[string]any {
	out := map[string]any{}
	switch m := v.(type) {
	case map[string]dbus.Variant:
		for k, vv := range m {
			out[k] = vv.Value()
		}
	case map[string]any:
		for k, vv := range m {
			if dv, ok := vv.(dbus.Variant); ok {
				out[k] = dv.Value()
			} else {
				out[k] = vv
			}
		}
	}
	return out
}

func getString(m map[string]any, key string) string {
	v, ok := m[key]
	if !ok || v == nil {
		return ""
	}
	return fmt.Sprint(v)
}

func getInt(m map[string]any, key string, def int) int {
	v, ok := m[key]
	if !ok {
		return def
	}
	switch n := v.(type) {
	case int:
		return n
	case int8:
		return int(n)
	case int16:
		return int(n)
	case int32:
		return int(n)
	case int64:
		return int(n)
	case uint:
		return int(n)
	case uint8:
		return int(n)
	case uint16:
		return int(n)
	case uint32:
		return int(n)
	case uint64:
		return int(n)
	case float32:
		return int(n)
	case float64:
		return int(n)
	case string:
		i, err := strconv.Atoi(strings.TrimSpace(n))
		if err == nil {
			return i
		}
	}
	return def
}

func getStringList(m map[string]any, key string) []string {
	v, ok := m[key]
	if !ok {
		return nil
	}
	return toStringSlice(v)
}

func toStringSlice(v any) []string {
	vals := toAnySlice(v)
	if len(vals) == 0 {
		if s, ok := v.(string); ok {
			if s == "" {
				return nil
			}
			return []string{s}
		}
		return nil
	}

	out := make([]string, 0, len(vals))
	for _, item := range vals {
		s := strings.TrimSpace(fmt.Sprint(item))
		if s != "" {
			out = append(out, s)
		}
	}
	return out
}

func toAnySlice(v any) []any {
	switch a := v.(type) {
	case []any:
		return a
	case []string:
		out := make([]any, 0, len(a))
		for _, item := range a {
			out = append(out, item)
		}
		return out
	case [][]any:
		out := make([]any, 0, len(a))
		for _, item := range a {
			out = append(out, any(item))
		}
		return out
	case [][]string:
		out := make([]any, 0, len(a))
		for _, item := range a {
			inner := make([]any, 0, len(item))
			for _, p := range item {
				inner = append(inner, p)
			}
			out = append(out, inner)
		}
		return out
	}
	return nil
}

func firstStringList(a map[string]any, key string, fallback []string) []string {
	if list := getStringList(a, key); len(list) > 0 {
		return list
	}
	return fallback
}

func hasImmutableTag(short string) bool {
	return strings.Contains(short, "(immutable)")
}

func mapZoneTarget(target string) string {
	switch strings.ToUpper(strings.TrimSpace(target)) {
	case "%%REJECT%%", "REJECT":
		return "reject"
	case "DROP":
		return "drop"
	case "ACCEPT", "DEFAULT", "":
		return "accept"
	default:
		return "accept"
	}
}

func mapPolicyTarget(target string) string {
	switch strings.ToUpper(strings.TrimSpace(target)) {
	case "CONTINUE":
		return "continue"
	case "ACCEPT":
		return "accept"
	case "DROP":
		return "drop"
	case "REJECT", "":
		return "reject"
	default:
		return "reject"
	}
}

func parseQuotedName(rule string) string {
	idx := strings.Index(rule, `name="`)
	if idx < 0 {
		return ""
	}
	start := idx + len(`name="`)
	end := strings.Index(rule[start:], `"`)
	if end < 0 {
		return ""
	}
	return rule[start : start+end]
}

func parsePriority(fragment string) int {
	fragment = strings.TrimSpace(fragment)
	if fragment == "" {
		return -1
	}
	fields := strings.Fields(fragment)
	if len(fields) == 0 {
		return -1
	}
	p, err := strconv.Atoi(strings.Trim(fields[0], `"`))
	if err != nil {
		return -1
	}
	return p
}

func asBool(v any) bool {
	switch x := v.(type) {
	case bool:
		return x
	case uint8:
		return x != 0
	case uint16:
		return x != 0
	case uint32:
		return x != 0
	case uint64:
		return x != 0
	case int8:
		return x != 0
	case int16:
		return x != 0
	case int32:
		return x != 0
	case int64:
		return x != 0
	case int:
		return x != 0
	case string:
		x = strings.TrimSpace(strings.ToLower(x))
		return x == "1" || x == "true" || x == "yes" || x == "on"
	default:
		return false
	}
}

func toUint64(v any) uint64 {
	switch x := v.(type) {
	case uint8:
		return uint64(x)
	case uint16:
		return uint64(x)
	case uint32:
		return uint64(x)
	case uint64:
		return x
	case uint:
		return uint64(x)
	case int8:
		if x < 0 {
			return 0
		}
		return uint64(x)
	case int16:
		if x < 0 {
			return 0
		}
		return uint64(x)
	case int32:
		if x < 0 {
			return 0
		}
		return uint64(x)
	case int64:
		if x < 0 {
			return 0
		}
		return uint64(x)
	case int:
		if x < 0 {
			return 0
		}
		return uint64(x)
	case float32:
		if x < 0 {
			return 0
		}
		return uint64(x)
	case float64:
		if x < 0 {
			return 0
		}
		return uint64(x)
	case string:
		u, err := strconv.ParseUint(strings.TrimSpace(x), 10, 64)
		if err == nil {
			return u
		}
	}
	return 0
}

func sleepOrDone(ctx context.Context, d time.Duration) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-time.After(d):
		return nil
	}
}

func nextDelay(delay time.Duration) time.Duration {
	next := time.Duration(math.Min(float64(delay)*reconnectFactor, float64(reconnectMax)))
	if next <= 0 {
		return reconnectInitial
	}
	return next
}
