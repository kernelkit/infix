package iwmonitor

import (
	"encoding/json"
	"strconv"
	"strings"

	"github.com/kernelkit/infix/src/yangerd/internal/wpactrl"
)

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

func parseIWLink(output string) map[string]string {
	m := make(map[string]string)
	for _, line := range strings.Split(output, "\n") {
		if k, v, ok := parseKV(strings.TrimSpace(line)); ok {
			m[k] = v
		}
	}
	return m
}

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

// parseBitrate extracts the speed in 100kbps units from iw/hostapd rate info.
// iw link: "866.7 MBit/s VHT-MCS 9 ..."
// hostapd: "1560 vhtmcs 8 vhtnss 2" (value in 100kbps)
func parseBitrate(s string) uint32 {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0
	}
	fields := strings.Fields(s)
	if len(fields) == 0 {
		return 0
	}
	if strings.Contains(s, "MBit/s") {
		val, err := strconv.ParseFloat(fields[0], 64)
		if err != nil {
			return 0
		}
		return uint32(val * 10)
	}
	val, err := strconv.ParseUint(fields[0], 10, 32)
	if err != nil {
		return 0
	}
	return uint32(val)
}

func extractEncryption(flags string) []string {
	flags = strings.ToUpper(flags)
	var result []string
	if strings.Contains(flags, "WPA3") || strings.Contains(flags, "SAE") {
		result = append(result, "WPA3-Personal")
	}
	if strings.Contains(flags, "WPA2") {
		if strings.Contains(flags, "EAP") {
			result = append(result, "WPA2-Enterprise")
		} else {
			result = append(result, "WPA2-Personal")
		}
	}
	if strings.Contains(flags, "WEP") {
		return []string{"WEP"}
	}
	if len(result) == 0 && strings.Contains(flags, "ESS") {
		return []string{"Open"}
	}
	if len(result) == 0 {
		return []string{"Unknown"}
	}
	return result
}

func formatScanResults(results []wpactrl.ScanResult) []map[string]any {
	seen := make(map[string]int)
	var out []map[string]any

	for _, r := range results {
		if r.SSID == "" {
			continue
		}
		entry := map[string]any{
			"ssid":            r.SSID,
			"bssid":           r.BSSID,
			"signal-strength": r.Signal,
			"channel":         wpactrl.FrequencyToChannel(r.Frequency),
		}
		if enc := extractEncryption(r.Flags); len(enc) > 0 {
			entry["encryption"] = enc
		}

		if idx, dup := seen[r.SSID]; dup {
			prev := out[idx]["signal-strength"].(int)
			if r.Signal > prev {
				out[idx] = entry
			}
			continue
		}
		seen[r.SSID] = len(out)
		out = append(out, entry)
	}
	return out
}

// ParseIWEvent parses a single line from `iw event -t` output.
// Retained for tests; no longer used in the main event loop.
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

// resolveSSID extracts the SSID for an interface.
// wpa_supplicant STATUS has "ssid=<value>".
// hostapd STATUS has "bss[N]=<ifname>" / "ssid[N]=<value>" pairs.
func resolveSSID(iface string, si wpactrl.SocketInfo, status map[string]string) string {
	if v := status["ssid"]; v != "" {
		return v
	}
	for i := 0; i < 16; i++ {
		idx := strconv.Itoa(i)
		if status["bss["+idx+"]"] == iface {
			if v := status["ssid["+idx+"]"]; v != "" {
				return v
			}
			break
		}
	}
	return ""
}
