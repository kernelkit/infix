// Package iwmonitor manages a persistent `iw event -t` subprocess
// for reactive 802.11 wireless monitoring.  Events are parsed from
// the human-readable text output and dispatched to re-query handlers.
// Started only when YANGERD_ENABLE_WIFI=true.
package iwmonitor

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"math"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

const (
	reconnectInitial = 100 * time.Millisecond
	reconnectMax     = 30 * time.Second
	reconnectFactor  = 2.0
	queryTimeout     = 5 * time.Second
)

// IWEvent represents a single parsed line from `iw event -t`.
type IWEvent struct {
	Timestamp float64
	Interface string
	Phy       string
	Type      string
	Addr      string
}

// IWMonitor subscribes to WiFi events via `iw event -t` and updates
// the tree when stations associate/disassociate, links connect/
// disconnect, or regulatory domains change.
type IWMonitor struct {
	log      *slog.Logger
	onUpdate func(ifname string, data json.RawMessage)
}

// New creates an IWMonitor.
func New(log *slog.Logger) *IWMonitor {
	return &IWMonitor{log: log}
}

// SetOnUpdate sets the callback invoked when wifi data changes for
// an interface. The data is a JSON object with station/info fields.
func (m *IWMonitor) SetOnUpdate(fn func(string, json.RawMessage)) {
	m.onUpdate = fn
}

// Run starts the iw event monitor.  It blocks until ctx is cancelled.
// On subprocess exit it reconnects with exponential backoff.
func (m *IWMonitor) Run(ctx context.Context) error {
	delay := reconnectInitial

	for {
		err := m.runOnce(ctx)
		if ctx.Err() != nil {
			return ctx.Err()
		}

		m.log.Warn("iw event: subprocess exited, restarting",
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

func (m *IWMonitor) runOnce(ctx context.Context) error {
	cmd := exec.CommandContext(ctx, "iw", "event", "-t")
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return fmt.Errorf("stdout pipe: %w", err)
	}
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("start iw event: %w", err)
	}
	defer cmd.Wait()

	m.refreshAllInterfaces(ctx)

	scanner := bufio.NewScanner(stdout)
	for scanner.Scan() {
		ev, ok := ParseIWEvent(scanner.Text())
		if !ok {
			continue
		}
		m.handleEvent(ctx, ev)
	}
	if err := scanner.Err(); err != nil {
		return fmt.Errorf("read iw event: %w", err)
	}
	return fmt.Errorf("iw event process exited")
}

func (m *IWMonitor) handleEvent(ctx context.Context, ev IWEvent) {
	switch ev.Type {
	case "new station", "del station":
		m.refreshStationList(ctx, ev.Interface)
	case "connected", "ch_switch_started_notify":
		m.refreshInterfaceInfo(ctx, ev.Interface)
	case "disconnected":
		m.publishWifi(ev.Interface, json.RawMessage(`{}`), nil)
	case "reg_change":
		m.refreshAllInterfaces(ctx)
	default:
		m.log.Debug("iw event: unhandled", "type", ev.Type, "iface", ev.Interface)
	}
}

func (m *IWMonitor) refreshStationList(ctx context.Context, iface string) {
	ctx, cancel := context.WithTimeout(ctx, queryTimeout)
	defer cancel()
	out, err := exec.CommandContext(ctx, "iw", "dev", iface, "station", "dump").Output()
	if err != nil {
		m.log.Warn("iw station dump failed", "iface", iface, "err", err)
		return
	}
	stations := parseStationDump(string(out))
	m.publishWifi(iface, nil, stations)
}

func (m *IWMonitor) refreshInterfaceInfo(ctx context.Context, iface string) {
	ctx, cancel := context.WithTimeout(ctx, queryTimeout)
	defer cancel()
	out, err := exec.CommandContext(ctx, "iw", "dev", iface, "info").Output()
	if err != nil {
		m.log.Warn("iw dev info failed", "iface", iface, "err", err)
		return
	}
	info := parseIWInfo(string(out))
	m.publishWifi(iface, info, nil)
}

func (m *IWMonitor) publishWifi(iface string, info, stations json.RawMessage) {
	if m.onUpdate == nil {
		return
	}

	wifi := make(map[string]json.RawMessage)
	if info != nil {
		wifi["info"] = info
	}
	if stations != nil {
		wifi["stations"] = stations
	}

	raw, err := json.Marshal(wifi)
	if err != nil {
		m.log.Warn("iwmonitor: marshal wifi data", "iface", iface, "err", err)
		return
	}
	m.onUpdate(iface, json.RawMessage(raw))
}

func (m *IWMonitor) refreshAllInterfaces(ctx context.Context) {
	ctx, cancel := context.WithTimeout(ctx, queryTimeout)
	defer cancel()
	out, err := exec.CommandContext(ctx, "iw", "dev").Output()
	if err != nil {
		m.log.Warn("iw dev list failed", "err", err)
		return
	}
	for _, iface := range parseIWDevList(string(out)) {
		m.refreshInterfaceInfo(ctx, iface)
		m.refreshStationList(ctx, iface)
	}
}

// ParseIWEvent parses a single line from `iw event -t` output.
func ParseIWEvent(line string) (IWEvent, bool) {
	parts := strings.SplitN(line, ": ", 3)
	if len(parts) < 3 {
		return IWEvent{}, false
	}

	ts, err := strconv.ParseFloat(parts[0], 64)
	if err != nil {
		return IWEvent{}, false
	}

	ifacePhy := parts[1]
	parenIdx := strings.Index(ifacePhy, " (")
	if parenIdx < 0 {
		return IWEvent{}, false
	}
	iface := ifacePhy[:parenIdx]
	phy := strings.Trim(ifacePhy[parenIdx+2:], ")")

	eventStr := parts[2]
	ev := IWEvent{Timestamp: ts, Interface: iface, Phy: phy}

	switch {
	case strings.HasPrefix(eventStr, "new station "):
		ev.Type = "new station"
		ev.Addr = strings.TrimPrefix(eventStr, "new station ")
	case strings.HasPrefix(eventStr, "del station "):
		ev.Type = "del station"
		ev.Addr = strings.TrimPrefix(eventStr, "del station ")
	case strings.HasPrefix(eventStr, "connected to "):
		ev.Type = "connected"
		ev.Addr = strings.TrimPrefix(eventStr, "connected to ")
	case eventStr == "disconnected":
		ev.Type = "disconnected"
	case strings.HasPrefix(eventStr, "ch_switch_started_notify"):
		ev.Type = "ch_switch_started_notify"
	case eventStr == "scan started":
		ev.Type = "scan started"
	case eventStr == "scan aborted":
		ev.Type = "scan aborted"
	case strings.HasPrefix(eventStr, "reg_change"):
		ev.Type = "reg_change"
	case strings.HasPrefix(eventStr, "auth"):
		ev.Type = "auth"
	default:
		ev.Type = eventStr
	}

	return ev, true
}

// parseStationDump parses `iw dev <iface> station dump` output into JSON.
func parseStationDump(output string) json.RawMessage {
	type station struct {
		MAC        string `json:"mac"`
		Signal     string `json:"signal,omitempty"`
		RxBytes    string `json:"rx-bytes,omitempty"`
		TxBytes    string `json:"tx-bytes,omitempty"`
		Connected  string `json:"connected-time,omitempty"`
		Inactive   string `json:"inactive-time,omitempty"`
		RxBitrate  string `json:"rx-bitrate,omitempty"`
		TxBitrate  string `json:"tx-bitrate,omitempty"`
		Authorized string `json:"authorized,omitempty"`
	}
	var stations []station
	var current *station

	for _, line := range strings.Split(output, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "Station ") {
			parts := strings.Fields(line)
			if len(parts) >= 2 {
				s := station{MAC: parts[1]}
				stations = append(stations, s)
				current = &stations[len(stations)-1]
			}
			continue
		}
		if current == nil {
			continue
		}
		if k, v, ok := parseKV(line); ok {
			switch k {
			case "signal":
				current.Signal = v
			case "rx bytes":
				current.RxBytes = v
			case "tx bytes":
				current.TxBytes = v
			case "connected time":
				current.Connected = v
			case "inactive time":
				current.Inactive = v
			case "rx bitrate":
				current.RxBitrate = v
			case "tx bitrate":
				current.TxBitrate = v
			case "authorized":
				current.Authorized = v
			}
		}
	}

	data, _ := json.Marshal(stations)
	return json.RawMessage(data)
}

// parseIWInfo parses `iw dev <iface> info` output into JSON.
func parseIWInfo(output string) json.RawMessage {
	info := make(map[string]string)
	for _, line := range strings.Split(output, "\n") {
		if k, v, ok := parseKV(strings.TrimSpace(line)); ok {
			switch k {
			case "ssid":
				info["ssid"] = v
			case "type":
				info["type"] = v
			case "channel":
				info["channel"] = v
			case "txpower":
				info["tx-power"] = v
			}
		}
	}
	data, _ := json.Marshal(info)
	return json.RawMessage(data)
}

// parseIWDevList extracts interface names from `iw dev` output.
func parseIWDevList(output string) []string {
	var ifaces []string
	for _, line := range strings.Split(output, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "Interface ") {
			if name := strings.TrimPrefix(line, "Interface "); name != "" {
				ifaces = append(ifaces, name)
			}
		}
	}
	return ifaces
}

func parseKV(line string) (string, string, bool) {
	idx := strings.Index(line, ":")
	if idx < 0 {
		return "", "", false
	}
	k := strings.TrimSpace(line[:idx])
	v := strings.TrimSpace(line[idx+1:])
	return k, v, k != ""
}
