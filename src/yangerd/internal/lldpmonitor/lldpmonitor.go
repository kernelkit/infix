// Package lldpmonitor manages a persistent `lldpcli -f json0 watch`
// subprocess for reactive LLDP neighbor updates.  Events are framed
// as pretty-printed JSON objects separated by blank lines.  Each
// event triggers full LLDP subtree regeneration.
package lldpmonitor

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"math"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/kernelkit/infix/src/yangerd/internal/tree"
)

const (
	reconnectInitial = 100 * time.Millisecond
	reconnectMax     = 30 * time.Second
	reconnectFactor  = 2.0

	lldpMulticastMAC = "01:80:C2:00:00:0E"
	treeKey          = "ieee802-dot1ab-lldp:lldp"
)

// LLDPMonitor subscribes to LLDP neighbor events via a persistent
// lldpcli watch subprocess and updates the tree on every event.
type LLDPMonitor struct {
	tree *tree.Tree
	log  *slog.Logger
}

// New creates an LLDPMonitor.
func New(t *tree.Tree, log *slog.Logger) *LLDPMonitor {
	return &LLDPMonitor{tree: t, log: log}
}

// Run starts the LLDP monitor.  It blocks until ctx is cancelled.
func (m *LLDPMonitor) Run(ctx context.Context) error {
	delay := reconnectInitial

	for {
		err := m.runOnce(ctx)
		if ctx.Err() != nil {
			return ctx.Err()
		}

		m.log.Warn("lldp monitor: subprocess exited, restarting",
			"err", err, "delay", delay)
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(delay):
		}
		delay = time.Duration(math.Min(
			float64(delay)*reconnectFactor,
			float64(reconnectMax)))
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

func (m *LLDPMonitor) processEvent(data []byte) {
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		m.log.Warn("lldp monitor: parse event", "err", err)
		return
	}

	// Dispatch by root key: lldp-added, lldp-updated, lldp-deleted.
	// All event types trigger full subtree rebuild from payload.
	for key := range raw {
		switch key {
		case "lldp-added", "lldp-updated", "lldp-deleted":
			m.rebuildTree(data)
			return
		}
	}
	m.log.Debug("lldp monitor: unknown event keys", "keys", keysOf(raw))
}

func (m *LLDPMonitor) rebuildTree(data []byte) {
	result := transformLLDPEvent(data)
	m.tree.Set(treeKey, result)
	m.log.Debug("lldp monitor: tree updated")
}

// transformLLDPEvent converts lldpcli json0 watch output to YANG
// ieee802-dot1ab-lldp:lldp format matching the Python infix_lldp.py.
func transformLLDPEvent(data []byte) json.RawMessage {
	var event map[string]json.RawMessage
	if err := json.Unmarshal(data, &event); err != nil {
		return json.RawMessage(`{}`)
	}

	// Extract the lldp payload from whichever event key is present.
	var lldpPayload json.RawMessage
	for _, key := range []string{"lldp-added", "lldp-updated", "lldp-deleted"} {
		if p, ok := event[key]; ok {
			lldpPayload = p
			break
		}
	}
	if lldpPayload == nil {
		return json.RawMessage(`{}`)
	}

	var lldpData struct {
		LLDP struct {
			Interface []map[string]json.RawMessage `json:"interface"`
		} `json:"lldp"`
	}
	if err := json.Unmarshal(lldpPayload, &lldpData); err != nil {
		// Try unwrapped format.
		var unwrapped struct {
			Interface []map[string]json.RawMessage `json:"interface"`
		}
		if err := json.Unmarshal(lldpPayload, &unwrapped); err != nil {
			return json.RawMessage(`{}`)
		}
		lldpData.LLDP.Interface = unwrapped.Interface
	}

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

	for _, ifEntry := range lldpData.LLDP.Interface {
		for ifName, ifData := range ifEntry {
			var iface struct {
				RID     interface{} `json:"rid"`
				Age     string      `json:"age"`
				Chassis struct {
					ID struct {
						Type  string `json:"type"`
						Value string `json:"value"`
					} `json:"id"`
				} `json:"chassis"`
				Port struct {
					ID struct {
						Type  string `json:"type"`
						Value string `json:"value"`
					} `json:"id"`
				} `json:"port"`
			}
			if err := json.Unmarshal(ifData, &iface); err != nil {
				continue
			}

			port, ok := portMap[ifName]
			if !ok {
				port = &portEntry{
					Name:           ifName,
					DestMACAddress: lldpMulticastMAC,
				}
				portMap[ifName] = port
			}

			rid := 0
			switch v := iface.RID.(type) {
			case float64:
				rid = int(v)
			case string:
				rid, _ = strconv.Atoi(v)
			}

			remote := remoteEntry{
				TimeMark:         parseAge(iface.Age),
				RemoteIndex:      rid,
				ChassisIDSubtype: chassisIDSubtype(iface.Chassis.ID.Type),
				ChassisID:        iface.Chassis.ID.Value,
				PortIDSubtype:    portIDSubtype(iface.Port.ID.Type),
				PortID:           iface.Port.ID.Value,
			}
			port.RemoteSystems = append(port.RemoteSystems, remote)
		}
	}

	var ports []portEntry
	for _, p := range portMap {
		if len(p.RemoteSystems) > 0 {
			ports = append(ports, *p)
		}
	}

	result := map[string]interface{}{
		"ieee802-dot1ab-lldp:lldp": map[string]interface{}{
			"port": ports,
		},
	}
	out, _ := json.Marshal(result)
	return json.RawMessage(out)
}

var chassisSubtypeMap = map[string]string{
	"ifalias": "interface-alias",
	"mac":     "mac-address",
	"ip":      "network-address",
	"ifname":  "interface-name",
	"local":   "local",
}

var portSubtypeMap = map[string]string{
	"ifalias": "interface-alias",
	"mac":     "mac-address",
	"ip":      "network-address",
	"ifname":  "interface-name",
	"local":   "local",
}

func chassisIDSubtype(t string) string {
	if v, ok := chassisSubtypeMap[t]; ok {
		return v
	}
	return "unknown"
}

func portIDSubtype(t string) string {
	if v, ok := portSubtypeMap[t]; ok {
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
