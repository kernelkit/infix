package iwmonitor

import (
	"encoding/json"
	"testing"

	"github.com/kernelkit/infix/src/yangerd/internal/wpactrl"
)

func TestFormatStations(t *testing.T) {
	m := &IWMonitor{}
	stas := []map[string]string{
		{
			"addr":           "02:00:00:00:00:01",
			"signal":         "-57",
			"connected_time": "120",
			"rx_packets":     "1500",
			"tx_packets":     "2500",
			"rx_bytes":       "4825331939",
			"tx_bytes":       "216392802676",
			"rx_rate_info":   "1560 vhtmcs 8 vhtnss 2",
			"tx_rate_info":   "1733 vhtmcs 9 vhtnss 2",
		},
	}

	raw, err := json.Marshal(m.formatStations(stas))
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var parsed struct {
		Station []struct {
			MAC           string `json:"mac-address"`
			Signal        int16  `json:"signal-strength"`
			ConnectedTime uint32 `json:"connected-time"`
			RxPackets     string `json:"rx-packets"`
			TxPackets     string `json:"tx-packets"`
			RxBytes       string `json:"rx-bytes"`
			TxBytes       string `json:"tx-bytes"`
			RxSpeed       uint32 `json:"rx-speed"`
			TxSpeed       uint32 `json:"tx-speed"`
		} `json:"station"`
	}
	if err := json.Unmarshal(raw, &parsed); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(parsed.Station) != 1 {
		t.Fatalf("got %d stations, want 1", len(parsed.Station))
	}

	s := parsed.Station[0]
	if s.MAC != "02:00:00:00:00:01" {
		t.Errorf("mac-address = %q", s.MAC)
	}
	if s.Signal != -57 {
		t.Errorf("signal-strength = %d, want -57", s.Signal)
	}
	if s.ConnectedTime != 120 {
		t.Errorf("connected-time = %d, want 120", s.ConnectedTime)
	}
	if s.RxBytes != "4825331939" || s.TxBytes != "216392802676" {
		t.Errorf("bytes = %q/%q", s.RxBytes, s.TxBytes)
	}
	if s.RxPackets != "1500" || s.TxPackets != "2500" {
		t.Errorf("packets = %q/%q", s.RxPackets, s.TxPackets)
	}
	// hostapd rate info is already in 100kbps units
	if s.RxSpeed != 1560 || s.TxSpeed != 1733 {
		t.Errorf("speed = %d/%d, want 1560/1733", s.RxSpeed, s.TxSpeed)
	}
}

func TestFilterAuthorized(t *testing.T) {
	stas := []map[string]string{
		{"addr": "02:00:00:00:00:01", "flags": "[AUTH][ASSOC][AUTHORIZED]"},
		{"addr": "02:00:00:00:00:02", "flags": "[AUTH][ASSOC]"}, // mid-handshake
		{"addr": "02:00:00:00:00:03", "flags": "[AUTH][ASSOC][AUTHORIZED][SHORT_PREAMBLE]"},
	}

	out := filterAuthorized(stas)
	if len(out) != 2 {
		t.Fatalf("got %d stations, want 2", len(out))
	}
	if out[0]["addr"] != "02:00:00:00:00:01" || out[1]["addr"] != "02:00:00:00:00:03" {
		t.Errorf("addrs = %q, %q", out[0]["addr"], out[1]["addr"])
	}
}

func TestResolveSSIDHostapd(t *testing.T) {
	// hostapd STATUS reports bss[N]=<ifname> / ssid[N]=<ssid> pairs;
	// multi-BSS setups must resolve by interface name.
	status := map[string]string{
		"state":   "ENABLED",
		"bss[0]":  "wlan0",
		"ssid[0]": "Lobby",
		"bss[1]":  "wlan0_1",
		"ssid[1]": "Office",
	}

	si := wpactrl.SocketInfo{Iface: "wlan0_1", Daemon: "hostapd"}
	if got := resolveSSID("wlan0_1", si, status); got != "Office" {
		t.Errorf("resolveSSID(wlan0_1) = %q, want Office", got)
	}
	si = wpactrl.SocketInfo{Iface: "wlan0", Daemon: "hostapd"}
	if got := resolveSSID("wlan0", si, status); got != "Lobby" {
		t.Errorf("resolveSSID(wlan0) = %q, want Lobby", got)
	}
	if got := resolveSSID("wlan9", si, status); got != "" {
		t.Errorf("resolveSSID(wlan9) = %q, want empty", got)
	}
}

func TestParseBitrate(t *testing.T) {
	cases := map[string]uint32{
		"1560 vhtmcs 8 vhtnss 2": 1560, // hostapd: 100kbps units
		"866.7 MBit/s VHT-MCS 9": 8667, // iw: MBit/s -> 100kbps
		"54.0 MBit/s":            540,
		"":                       0,
		"garbage rate":           0,
	}
	for in, want := range cases {
		if got := parseBitrate(in); got != want {
			t.Errorf("parseBitrate(%q) = %d, want %d", in, got, want)
		}
	}
}
