// Package lldpmonitor keeps the LLDP neighbor table in the tree in sync
// with lldpd.  A persistent `lldpcli -f json0 watch` subprocess is used
// purely as a change trigger -- its events carry only the changed
// neighbor, so they cannot be used to rebuild state (a delete event
// would re-add the neighbor, and an event for one port would wipe the
// others).  On every event the full table is re-read with
// `lldpcli -f json0 show neighbors` and the subtree replaced, so removed
// neighbors disappear and neighbors present before yangerd started are
// picked up.
package lldpmonitor

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/kernelkit/infix/src/yangerd/internal/backoff"
	"github.com/kernelkit/infix/src/yangerd/internal/tree"
)

const (
	lldpMulticastMAC = "01:80:C2:00:00:0E"
	treeKey          = "ieee802-dot1ab-lldp:lldp"

	// debounceDelay coalesces bursts of watch events into one re-read.
	debounceDelay = 200 * time.Millisecond
	queryTimeout  = 5 * time.Second
)

// LLDPMonitor subscribes to LLDP neighbor events via a persistent
// lldpcli watch subprocess and re-reads the full neighbor table on
// every event.
type LLDPMonitor struct {
	tree    *tree.Tree
	log     *slog.Logger
	refresh chan struct{}

	// query returns the current full neighbor table; overridable in tests.
	query func(ctx context.Context) ([]byte, error)
}

// New creates an LLDPMonitor.
func New(t *tree.Tree, log *slog.Logger) *LLDPMonitor {
	if log == nil {
		log = slog.Default()
	}
	return &LLDPMonitor{
		tree:    t,
		log:     log,
		refresh: make(chan struct{}, 1),
		query:   queryNeighbors,
	}
}

func queryNeighbors(ctx context.Context) ([]byte, error) {
	ctx, cancel := context.WithTimeout(ctx, queryTimeout)
	defer cancel()
	return exec.CommandContext(ctx, "lldpcli", "-f", "json0", "show", "neighbors").Output()
}

// Run starts the LLDP monitor.  It blocks until ctx is cancelled.
func (m *LLDPMonitor) Run(ctx context.Context) error {
	go m.refreshLoop(ctx)

	bo := backoff.Default()
	delay := bo.Initial

	for {
		err := m.runOnce(ctx)
		if ctx.Err() != nil {
			return ctx.Err()
		}

		m.log.Warn("lldp monitor: subprocess exited, restarting",
			"err", err, "delay", delay)
		if err := backoff.Sleep(ctx, delay); err != nil {
			return err
		}
		delay = bo.Next(delay)
	}
}

func (m *LLDPMonitor) runOnce(ctx context.Context) error {
	cmd := exec.CommandContext(ctx, "lldpcli", "-f", "json0", "watch")
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return fmt.Errorf("stdout pipe: %w", err)
	}
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("start lldpcli watch: %w", err)
	}
	defer cmd.Wait()

	// Pick up neighbors that existed before we attached.
	m.triggerRefresh()

	scanner := bufio.NewScanner(stdout)
	scanner.Buffer(make([]byte, 0, 1*1024*1024), 1*1024*1024)

	var buf bytes.Buffer
	braceDepth := 0

	for scanner.Scan() {
		line := scanner.Text()

		// Blank-line framing: object separator.
		if strings.TrimSpace(line) == "" && braceDepth == 0 {
			if buf.Len() > 0 {
				m.processEvent(buf.Bytes())
				buf.Reset()
			}
			continue
		}

		buf.WriteString(line)
		buf.WriteByte('\n')

		for _, ch := range line {
			switch ch {
			case '{':
				braceDepth++
			case '}':
				braceDepth--
			}
		}

		if braceDepth == 0 && buf.Len() > 0 {
			m.processEvent(buf.Bytes())
			buf.Reset()
		}
	}
	if err := scanner.Err(); err != nil {
		return fmt.Errorf("read lldpcli: %w", err)
	}
	return fmt.Errorf("lldpcli watch process exited")
}

// processEvent inspects a watch event and triggers a full table re-read.
// The event payload itself is never used to build state.
func (m *LLDPMonitor) processEvent(data []byte) {
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		m.log.Warn("lldp monitor: parse event", "err", err)
		return
	}

	for key := range raw {
		switch key {
		case "lldp-added", "lldp-updated", "lldp-deleted":
			m.log.Debug("lldp monitor: neighbor change", "event", key)
			m.triggerRefresh()
			return
		}
	}
	m.log.Debug("lldp monitor: unknown event keys", "keys", keysOf(raw))
}

// triggerRefresh requests a table re-read; the buffered channel collapses
// pending requests into one.
func (m *LLDPMonitor) triggerRefresh() {
	select {
	case m.refresh <- struct{}{}:
	default:
	}
}

func (m *LLDPMonitor) refreshLoop(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case <-m.refresh:
		}

		// Let a burst of events settle before reading.
		select {
		case <-ctx.Done():
			return
		case <-time.After(debounceDelay):
		}
		select {
		case <-m.refresh:
		default:
		}

		m.updateTree(ctx)
	}
}

// updateTree reads the full neighbor table and replaces the subtree.
// On a query error the previous data is left untouched.
func (m *LLDPMonitor) updateTree(ctx context.Context) {
	out, err := m.query(ctx)
	if err != nil {
		m.log.Warn("lldp monitor: show neighbors", "err", err)
		return
	}

	m.tree.Set(treeKey, transformNeighbors(out))
	m.log.Debug("lldp monitor: tree updated")
}

// j0ID is a chassis/port id element: {"type": "mac", "value": "..."}.
type j0ID struct {
	Type  string `json:"type"`
	Value string `json:"value"`
}

// j0Iface is one neighbor entry on an interface.  In json0 format the
// interface name, rid and age are plain string fields and chassis/port
// are arrays.  In the older keyed json format chassis/port are objects;
// the custom unmarshallers accept both.
type j0Iface struct {
	Name    string      `json:"name"`
	RID     interface{} `json:"rid"`
	Age     string      `json:"age"`
	Chassis j0IDHolder  `json:"chassis"`
	Port    j0IDHolder  `json:"port"`
}

// j0IDHolder extracts the first id from a chassis/port node in either
// json0 (array) or json (object) form.
type j0IDHolder struct {
	ID j0ID
}

func (h *j0IDHolder) UnmarshalJSON(data []byte) error {
	var asArray []struct {
		ID json.RawMessage `json:"id"`
	}
	if err := json.Unmarshal(data, &asArray); err == nil {
		for _, e := range asArray {
			if id, ok := parseID(e.ID); ok {
				h.ID = id
				return nil
			}
		}
		return nil
	}

	var asObject struct {
		ID json.RawMessage `json:"id"`
	}
	if err := json.Unmarshal(data, &asObject); err != nil {
		return nil // tolerate unknown shapes
	}
	if id, ok := parseID(asObject.ID); ok {
		h.ID = id
	}
	return nil
}

// parseID accepts an id as object {"type","value"} or array of such.
func parseID(raw json.RawMessage) (j0ID, bool) {
	if len(raw) == 0 {
		return j0ID{}, false
	}
	var one j0ID
	if err := json.Unmarshal(raw, &one); err == nil && (one.Type != "" || one.Value != "") {
		return one, true
	}
	var many []j0ID
	if err := json.Unmarshal(raw, &many); err == nil && len(many) > 0 {
		return many[0], true
	}
	return j0ID{}, false
}

// collectIfaces extracts all neighbor interface entries from a show
// neighbors document, accepting both json0 ("lldp" is an array, entries
// carry a "name" field) and json ("lldp" is an object, entries are keyed
// by interface name) output formats.
func collectIfaces(data []byte) []j0Iface {
	var ifaces []j0Iface

	addRaw := func(name string, raw json.RawMessage) {
		var iface j0Iface
		if err := json.Unmarshal(raw, &iface); err != nil {
			return
		}
		if iface.Name == "" {
			iface.Name = name
		}
		if iface.Name != "" {
			ifaces = append(ifaces, iface)
		}
	}

	collectInterfaceNode := func(raw json.RawMessage) {
		// json0: array of {"name": "eth0", ...}
		var asArray []json.RawMessage
		if err := json.Unmarshal(raw, &asArray); err == nil {
			for _, e := range asArray {
				// Either a direct entry with "name", or a keyed map
				// {"eth0": {...}} from the older json format.
				var iface j0Iface
				if err := json.Unmarshal(e, &iface); err == nil && iface.Name != "" {
					ifaces = append(ifaces, iface)
					continue
				}
				var keyed map[string]json.RawMessage
				if err := json.Unmarshal(e, &keyed); err == nil {
					for name, v := range keyed {
						addRaw(name, v)
					}
				}
			}
			return
		}
		// json: single keyed map {"eth0": {...}}
		var keyed map[string]json.RawMessage
		if err := json.Unmarshal(raw, &keyed); err == nil {
			for name, v := range keyed {
				addRaw(name, v)
			}
		}
	}

	var doc map[string]json.RawMessage
	if err := json.Unmarshal(data, &doc); err != nil {
		return nil
	}
	lldpRaw, ok := doc["lldp"]
	if !ok {
		return nil
	}

	// json0: "lldp" is an array of {"interface": [...]}; json: an object.
	var lldpArray []map[string]json.RawMessage
	if err := json.Unmarshal(lldpRaw, &lldpArray); err == nil {
		for _, entry := range lldpArray {
			if ifRaw, ok := entry["interface"]; ok {
				collectInterfaceNode(ifRaw)
			}
		}
		return ifaces
	}
	var lldpObject map[string]json.RawMessage
	if err := json.Unmarshal(lldpRaw, &lldpObject); err == nil {
		if ifRaw, ok := lldpObject["interface"]; ok {
			collectInterfaceNode(ifRaw)
		}
	}
	return ifaces
}

// transformNeighbors converts `lldpcli show neighbors` output to the
// YANG ieee802-dot1ab-lldp subtree (unwrapped -- the IPC layer adds the
// module envelope when serving the key).
func transformNeighbors(data []byte) json.RawMessage {
	type remoteEntry struct {
		TimeMark         int    `json:"time-mark"`
		RemoteIndex      int    `json:"remote-index"`
		ChassisIDSubtype string `json:"chassis-id-subtype"`
		ChassisID        string `json:"chassis-id"`
		PortIDSubtype    string `json:"port-id-subtype"`
		PortID           string `json:"port-id"`
	}
	type portEntry struct {
		Name           string        `json:"name"`
		DestMACAddress string        `json:"dest-mac-address"`
		RemoteSystems  []remoteEntry `json:"remote-systems-data"`
	}

	portMap := make(map[string]*portEntry)
	var order []string

	for _, iface := range collectIfaces(data) {
		port, ok := portMap[iface.Name]
		if !ok {
			port = &portEntry{
				Name:           iface.Name,
				DestMACAddress: lldpMulticastMAC,
			}
			portMap[iface.Name] = port
			order = append(order, iface.Name)
		}

		rid := 0
		switch v := iface.RID.(type) {
		case float64:
			rid = int(v)
		case string:
			rid, _ = strconv.Atoi(v)
		}

		port.RemoteSystems = append(port.RemoteSystems, remoteEntry{
			TimeMark:         parseAge(iface.Age),
			RemoteIndex:      rid,
			ChassisIDSubtype: chassisIDSubtype(iface.Chassis.ID.Type),
			ChassisID:        iface.Chassis.ID.Value,
			PortIDSubtype:    portIDSubtype(iface.Port.ID.Type),
			PortID:           iface.Port.ID.Value,
		})
	}

	ports := make([]portEntry, 0, len(order))
	for _, name := range order {
		ports = append(ports, *portMap[name])
	}

	if len(ports) == 0 {
		return json.RawMessage(`{}`)
	}

	out, _ := json.Marshal(map[string]interface{}{"port": ports})
	return json.RawMessage(out)
}

var idSubtypeMap = map[string]string{
	"ifalias": "interface-alias",
	"mac":     "mac-address",
	"ip":      "network-address",
	"ifname":  "interface-name",
	"local":   "local",
}

func chassisIDSubtype(t string) string {
	if v, ok := idSubtypeMap[t]; ok {
		return v
	}
	return "unknown"
}

func portIDSubtype(t string) string {
	if v, ok := idSubtypeMap[t]; ok {
		return v
	}
	return "unknown"
}

var ageRe = regexp.MustCompile(`(\d+)\s*day[s]*,\s*(\d+):(\d+):(\d+)`)

func parseAge(s string) int {
	m := ageRe.FindStringSubmatch(s)
	if m == nil {
		return 0
	}
	days, _ := strconv.Atoi(m[1])
	hours, _ := strconv.Atoi(m[2])
	mins, _ := strconv.Atoi(m[3])
	secs, _ := strconv.Atoi(m[4])
	return days*86400 + hours*3600 + mins*60 + secs
}

func keysOf(m map[string]json.RawMessage) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	return keys
}
