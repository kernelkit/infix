package iwmonitor

import (
	"encoding/json"
	"log/slog"
	"testing"

	"github.com/kernelkit/infix/src/yangerd/internal/nl80211"
	"github.com/kernelkit/infix/src/yangerd/internal/wpactrl"
)

func TestDetectMode(t *testing.T) {
	sup := wpactrl.SocketInfo{Iface: "wifi0", Daemon: "wpa_supplicant"}
	ap := wpactrl.SocketInfo{Iface: "wifi0", Daemon: "hostapd"}

	tests := []struct {
		name   string
		si     wpactrl.SocketInfo
		status map[string]string
		iftype string
		want   string
	}{
		{"hostapd", ap, nil, "AP", "ap"},
		{"station joined", sup, map[string]string{"mode": "station"}, "station", "station"},
		{"station idle", sup, map[string]string{}, "station", "station"},
		{"mesh joined", sup, map[string]string{"mode": "mesh"}, "mesh_point", "mesh"},
		{"mesh not yet joined", sup, map[string]string{"wpa_state": "SCANNING"}, "mesh_point", "mesh"},
		{"mesh, kernel unreadable", sup, map[string]string{"mode": "mesh"}, "", "mesh"},
	}
	for _, tt := range tests {
		if got := detectMode(tt.si, tt.status, tt.iftype); got != tt.want {
			t.Errorf("%s: detectMode = %q, want %q", tt.name, got, tt.want)
		}
	}
}

func TestDecodeWPASSID(t *testing.T) {
	tests := []struct{ in, want string }{
		{"plain", "plain"},
		{"caf\\xc3\\xa9", "café"},
		{"tab\\there", "tabhere"},
		{"esc\\e[31mred", "esc[31mred"},
		{"quote\\\"d", "quote\"d"},
		{"back\\\\slash", "back\\slash"},
		{"bad\\xff\\xfeutf8", "bad\\xff\\xfeutf8"},
		{"trailing\\", "trailing\\"},
		{"", ""},
	}
	for _, tt := range tests {
		if got := decodeWPASSID(tt.in); got != tt.want {
			t.Errorf("decodeWPASSID(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestBuildMeshData(t *testing.T) {
	m := New(slog.Default())
	si := wpactrl.SocketInfo{Iface: "wifi0", Daemon: "wpa_supplicant"}
	fwd := false
	sig := int8(-51)
	ms := meshState{
		iftype:     "mesh_point",
		forwarding: &fwd,
		peers: []nl80211.Station{{
			MAC: "02:00:00:00:00:02", Signal: sig, HasSignal: true,
			ConnectedTime: 42, RxBytes: 1000, TxBytes: 2000,
			RxPackets: 10, TxPackets: 20, RxBitrate: 650, TxBitrate: 1200,
		}, {
			MAC: "02:00:00:00:00:03",
		}},
	}
	status := map[string]string{"mode": "mesh", "ssid": "backhaul\\x2d1"}

	raw, err := json.Marshal(m.buildMeshData("wifi0", si, status, ms))
	if err != nil {
		t.Fatal(err)
	}

	var got struct {
		MeshID     string `json:"mesh-id"`
		Forwarding *bool  `json:"forwarding"`
		Peers      struct {
			Peer []map[string]any `json:"peer"`
		} `json:"peers"`
	}
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatal(err)
	}
	if got.MeshID != "backhaul-1" {
		t.Errorf("mesh-id = %q", got.MeshID)
	}
	if got.Forwarding == nil || *got.Forwarding {
		t.Errorf("forwarding = %v, want false", got.Forwarding)
	}
	if len(got.Peers.Peer) != 2 {
		t.Fatalf("peers = %v", got.Peers.Peer)
	}
	p := got.Peers.Peer[0]
	if p["mac-address"] != "02:00:00:00:00:02" || p["signal-strength"] != float64(-51) ||
		p["connected-time"] != float64(42) || p["rx-bytes"] != "1000" || p["tx-packets"] != "20" ||
		p["rx-speed"] != float64(650) || p["tx-speed"] != float64(1200) {
		t.Errorf("peer = %v", p)
	}
	if _, has := got.Peers.Peer[1]["signal-strength"]; has {
		t.Errorf("peer without signal must omit signal-strength: %v", got.Peers.Peer[1])
	}
}

// Before wpa_supplicant has joined, the mesh interface has no mesh-id and
// no peers, but is still reported as a mesh point.
func TestBuildMeshDataNotJoined(t *testing.T) {
	m := New(slog.Default())
	si := wpactrl.SocketInfo{Iface: "wifi0", Daemon: "wpa_supplicant"}
	fwd := true

	data := m.buildMeshData("wifi0", si, map[string]string{"wpa_state": "SCANNING"},
		meshState{iftype: "mesh_point", forwarding: &fwd})

	if _, has := data["mesh-id"]; has {
		t.Errorf("mesh-id must be absent until joined: %v", data)
	}
	if _, has := data["peers"]; has {
		t.Errorf("peers must be absent with no peers: %v", data)
	}
	if data["forwarding"] != true {
		t.Errorf("forwarding = %v", data["forwarding"])
	}
}

func TestMeshEventsRefresh(t *testing.T) {
	for _, line := range []string{
		"<3>MESH-PEER-CONNECTED 02:00:00:00:00:02",
		"<3>MESH-PEER-DISCONNECTED 02:00:00:00:00:02",
		"<3>MESH-GROUP-STARTED ssid=\"backhaul\" id=0",
		"<3>MESH-GROUP-REMOVED wifi0",
	} {
		ev, ok := wpactrl.ParseEvent(line)
		if !ok {
			t.Fatalf("ParseEvent(%q) failed", line)
		}
		if got := meshEvent(ev); !got {
			t.Errorf("%q must trigger a refresh", ev.Name)
		}
	}
	ev, _ := wpactrl.ParseEvent("<3>CTRL-EVENT-SCAN-RESULTS ")
	if meshEvent(ev) {
		t.Errorf("%q is not a mesh event", ev.Name)
	}
}
