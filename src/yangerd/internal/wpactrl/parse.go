package wpactrl

import (
	"strconv"
	"strings"
)

// ScanResult is a single entry from SCAN_RESULTS.
type ScanResult struct {
	BSSID     string
	Frequency int
	Signal    int
	Flags     string
	SSID      string
}

// ParseKV parses a wpa_supplicant/hostapd key=value response.
func ParseKV(resp string) map[string]string {
	m := make(map[string]string)
	for _, line := range strings.Split(resp, "\n") {
		line = strings.TrimSpace(line)
		if idx := strings.IndexByte(line, '='); idx > 0 {
			m[line[:idx]] = line[idx+1:]
		}
	}
	return m
}

// ParseScanResults parses wpa_supplicant SCAN_RESULTS output.
// Format: bssid / frequency / signal level / flags / ssid
// First line is a header, subsequent lines are tab-separated.
func ParseScanResults(resp string) []ScanResult {
	var results []ScanResult
	for _, line := range strings.Split(resp, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "bssid") {
			continue
		}
		fields := strings.SplitN(line, "\t", 5)
		if len(fields) < 4 {
			continue
		}
		freq, _ := strconv.Atoi(fields[1])
		sig, _ := strconv.Atoi(fields[2])
		ssid := ""
		if len(fields) >= 5 {
			ssid = fields[4]
		}
		results = append(results, ScanResult{
			BSSID:     fields[0],
			Frequency: freq,
			Signal:    sig,
			Flags:     fields[3],
			SSID:      ssid,
		})
	}
	return results
}

// ParseStationResp parses a hostapd STA-FIRST/STA-NEXT response.
// First line is the station MAC, subsequent lines are key=value pairs.
func ParseStationResp(resp string) map[string]string {
	lines := strings.Split(resp, "\n")
	if len(lines) == 0 {
		return nil
	}
	m := make(map[string]string)
	addr := strings.TrimSpace(lines[0])
	if addr != "" {
		m["addr"] = addr
	}
	for _, line := range lines[1:] {
		line = strings.TrimSpace(line)
		if idx := strings.IndexByte(line, '='); idx > 0 {
			m[line[:idx]] = line[idx+1:]
		}
	}
	return m
}

// ParseAllStations parses hostapd ALL_STA response containing multiple
// stations.  Each station block starts with a MAC address line (xx:xx:xx:xx:xx:xx)
// followed by key=value lines.
func ParseAllStations(resp string) []map[string]string {
	var stations []map[string]string
	var current map[string]string

	for _, line := range strings.Split(resp, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if isMACAddress(line) {
			if current != nil {
				stations = append(stations, current)
			}
			current = map[string]string{"addr": line}
			continue
		}
		if current != nil {
			if idx := strings.IndexByte(line, '='); idx > 0 {
				current[line[:idx]] = line[idx+1:]
			}
		}
	}
	if current != nil {
		stations = append(stations, current)
	}
	return stations
}

func isMACAddress(s string) bool {
	if len(s) != 17 {
		return false
	}
	for i, c := range s {
		if i%3 == 2 {
			if c != ':' {
				return false
			}
		} else {
			if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F')) {
				return false
			}
		}
	}
	return true
}

// FrequencyToChannel converts a WiFi frequency in MHz to a channel number.
func FrequencyToChannel(freq int) int {
	switch {
	case freq == 2484:
		return 14
	case freq >= 2412 && freq <= 2472:
		return (freq-2412)/5 + 1
	case freq >= 5170 && freq <= 5825:
		return (freq - 5000) / 5
	case freq >= 5955 && freq <= 7115:
		return (freq - 5950) / 5
	}
	return 0
}
