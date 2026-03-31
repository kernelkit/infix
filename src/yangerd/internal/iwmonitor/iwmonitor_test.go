package iwmonitor

import (
	"encoding/json"
	"reflect"
	"testing"
)

func TestParseIWEvent(t *testing.T) {
	tests := []struct {
		name      string
		line      string
		wantOK    bool
		wantType  string
		wantIface string
		wantPhy   string
		wantAddr  string
	}{
		{
			name:      "new station",
			line:      "1234567890.123456: wlan0 (phy#0): new station aa:bb:cc:dd:ee:ff",
			wantOK:    true,
			wantType:  "new station",
			wantIface: "wlan0",
			wantPhy:   "phy#0",
			wantAddr:  "aa:bb:cc:dd:ee:ff",
		},
		{
			name:      "del station",
			line:      "1234567890.123456: wlan0 (phy#0): del station aa:bb:cc:dd:ee:ff",
			wantOK:    true,
			wantType:  "del station",
			wantIface: "wlan0",
			wantPhy:   "phy#0",
			wantAddr:  "aa:bb:cc:dd:ee:ff",
		},
		{
			name:      "connected",
			line:      "1234567890.123456: wlan0 (phy#0): connected to aa:bb:cc:dd:ee:ff",
			wantOK:    true,
			wantType:  "connected",
			wantIface: "wlan0",
			wantPhy:   "phy#0",
			wantAddr:  "aa:bb:cc:dd:ee:ff",
		},
		{
			name:      "disconnected",
			line:      "1234567890.123456: wlan0 (phy#0): disconnected",
			wantOK:    true,
			wantType:  "disconnected",
			wantIface: "wlan0",
			wantPhy:   "phy#0",
		},
		{
			name:      "channel switch",
			line:      "1234567890.123456: wlan0 (phy#0): ch_switch_started_notify",
			wantOK:    true,
			wantType:  "ch_switch_started_notify",
			wantIface: "wlan0",
			wantPhy:   "phy#0",
		},
		{
			name:      "scan started",
			line:      "1234567890.123456: wlan0 (phy#0): scan started",
			wantOK:    true,
			wantType:  "scan started",
			wantIface: "wlan0",
			wantPhy:   "phy#0",
		},
		{
			name:      "reg change",
			line:      "1234567890.123456: wlan0 (phy#0): reg_change",
			wantOK:    true,
			wantType:  "reg_change",
			wantIface: "wlan0",
			wantPhy:   "phy#0",
		},
		{
			name:   "malformed missing separators",
			line:   "1234567890.123456 wlan0 (phy#0) new station aa:bb:cc:dd:ee:ff",
			wantOK: false,
		},
		{
			name:   "malformed bad timestamp",
			line:   "not-a-float: wlan0 (phy#0): disconnected",
			wantOK: false,
		},
		{
			name:   "malformed missing phy",
			line:   "1234567890.123456: wlan0 phy#0: disconnected",
			wantOK: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := ParseIWEvent(tt.line)
			if ok != tt.wantOK {
				t.Fatalf("ok = %v, want %v", ok, tt.wantOK)
			}
			if !tt.wantOK {
				return
			}

			if got.Type != tt.wantType {
				t.Fatalf("Type = %q, want %q", got.Type, tt.wantType)
			}
			if got.Interface != tt.wantIface {
				t.Fatalf("Interface = %q, want %q", got.Interface, tt.wantIface)
			}
			if got.Phy != tt.wantPhy {
				t.Fatalf("Phy = %q, want %q", got.Phy, tt.wantPhy)
			}
			if got.Addr != tt.wantAddr {
				t.Fatalf("Addr = %q, want %q", got.Addr, tt.wantAddr)
			}
		})
	}
}

func TestParseStationDump(t *testing.T) {
	input := `Station aa:bb:cc:dd:ee:ff (on wlan0)
	inactive time: 10 ms
	rx bytes: 1234
	tx bytes: 5678
	connected time: 42 seconds
	signal: -40 dBm
	rx bitrate: 6.5 MBit/s
	tx bitrate: 130.0 MBit/s
	authorized: yes

Station 11:22:33:44:55:66 (on wlan0)
	inactive time: 20 ms
	rx bytes: 9876
	tx bytes: 5432
	connected time: 84 seconds
	signal: -55 dBm
	authorized: no`

	got := parseStationDump(input)

	var gotDecoded []map[string]string
	if err := json.Unmarshal(got, &gotDecoded); err != nil {
		t.Fatalf("unmarshal got: %v", err)
	}

	want := []map[string]string{
		{
			"mac":            "aa:bb:cc:dd:ee:ff",
			"inactive-time":  "10 ms",
			"rx-bytes":       "1234",
			"tx-bytes":       "5678",
			"connected-time": "42 seconds",
			"signal":         "-40 dBm",
			"rx-bitrate":     "6.5 MBit/s",
			"tx-bitrate":     "130.0 MBit/s",
			"authorized":     "yes",
		},
		{
			"mac":            "11:22:33:44:55:66",
			"inactive-time":  "20 ms",
			"rx-bytes":       "9876",
			"tx-bytes":       "5432",
			"connected-time": "84 seconds",
			"signal":         "-55 dBm",
			"authorized":     "no",
		},
	}

	if !reflect.DeepEqual(gotDecoded, want) {
		t.Fatalf("parseStationDump mismatch\n got: %#v\nwant: %#v", gotDecoded, want)
	}
}

func TestParseIWInfo(t *testing.T) {
	input := `Interface wlan0
	ifindex: 4
	wdev: 0x1
	addr: 12:34:56:78:9a:bc
	ssid: MyWiFi
	type: managed
	channel: 11 (2462 MHz), width: 20 MHz, center1: 2462 MHz
	txpower: 20.00 dBm`

	got := parseIWInfo(input)

	var gotDecoded map[string]string
	if err := json.Unmarshal(got, &gotDecoded); err != nil {
		t.Fatalf("unmarshal got: %v", err)
	}

	want := map[string]string{
		"ssid":     "MyWiFi",
		"type":     "managed",
		"channel":  "11 (2462 MHz), width: 20 MHz, center1: 2462 MHz",
		"tx-power": "20.00 dBm",
	}

	if !reflect.DeepEqual(gotDecoded, want) {
		t.Fatalf("parseIWInfo mismatch\n got: %#v\nwant: %#v", gotDecoded, want)
	}
}

func TestParseIWDevList(t *testing.T) {
	input := `phy#0
	Interface wlan0
		ifindex 4
		wdev 0x1

phy#1
	Interface wlan1
		ifindex 5
		wdev 0x2`

	got := parseIWDevList(input)
	want := []string{"wlan0", "wlan1"}

	if !reflect.DeepEqual(got, want) {
		t.Fatalf("parseIWDevList mismatch\n got: %#v\nwant: %#v", got, want)
	}
}
