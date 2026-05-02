package wpactrl

import (
	"net"
	"os"
	"testing"
	"time"
)

func TestParseKV(t *testing.T) {
	resp := `bssid=02:00:00:00:01:00
freq=2412
ssid=TestNetwork
id=0
mode=station
pairwise_cipher=CCMP
group_cipher=CCMP
key_mgmt=WPA2-PSK
wpa_state=COMPLETED
address=02:00:00:00:00:01
`
	m := ParseKV(resp)
	if m["ssid"] != "TestNetwork" {
		t.Errorf("ssid = %q, want TestNetwork", m["ssid"])
	}
	if m["wpa_state"] != "COMPLETED" {
		t.Errorf("wpa_state = %q, want COMPLETED", m["wpa_state"])
	}
	if m["freq"] != "2412" {
		t.Errorf("freq = %q, want 2412", m["freq"])
	}
	if m["mode"] != "station" {
		t.Errorf("mode = %q, want station", m["mode"])
	}
}

func TestParseKVEmpty(t *testing.T) {
	m := ParseKV("")
	if len(m) != 0 {
		t.Errorf("expected empty map, got %v", m)
	}
}

func TestParseScanResults(t *testing.T) {
	resp := "bssid / frequency / signal level / flags / ssid\n" +
		"02:00:00:00:01:00\t2412\t-50\t[WPA2-PSK-CCMP][ESS]\tMyNetwork\n" +
		"02:00:00:00:02:00\t5180\t-70\t[WPA2-EAP-CCMP][ESS]\tOffice\n" +
		"02:00:00:00:03:00\t2437\t-85\t[ESS]\t\n"

	results := ParseScanResults(resp)
	if len(results) != 3 {
		t.Fatalf("got %d results, want 3", len(results))
	}

	r := results[0]
	if r.BSSID != "02:00:00:00:01:00" {
		t.Errorf("bssid = %q", r.BSSID)
	}
	if r.Frequency != 2412 {
		t.Errorf("freq = %d, want 2412", r.Frequency)
	}
	if r.Signal != -50 {
		t.Errorf("signal = %d, want -50", r.Signal)
	}
	if r.SSID != "MyNetwork" {
		t.Errorf("ssid = %q, want MyNetwork", r.SSID)
	}

	if results[1].Frequency != 5180 {
		t.Errorf("results[1].freq = %d, want 5180", results[1].Frequency)
	}
}

func TestParseScanResultsEmpty(t *testing.T) {
	results := ParseScanResults("bssid / frequency / signal level / flags / ssid\n")
	if len(results) != 0 {
		t.Errorf("expected empty, got %d", len(results))
	}
}

func TestParseStationResp(t *testing.T) {
	resp := "02:00:00:00:00:01\nflags=[AUTH][ASSOC][AUTHORIZED]\naid=1\n" +
		"rx_bytes=12345\ntx_bytes=67890\nconnected_time=120\n"

	m := ParseStationResp(resp)
	if m["addr"] != "02:00:00:00:00:01" {
		t.Errorf("addr = %q", m["addr"])
	}
	if m["rx_bytes"] != "12345" {
		t.Errorf("rx_bytes = %q", m["rx_bytes"])
	}
	if m["connected_time"] != "120" {
		t.Errorf("connected_time = %q", m["connected_time"])
	}
}

func TestParseStationRespEmpty(t *testing.T) {
	m := ParseStationResp("")
	if m["addr"] != "" {
		t.Errorf("expected empty addr, got %q", m["addr"])
	}
}

func TestParseAllStations(t *testing.T) {
	resp := "c8:69:cd:69:35:da\n" +
		"flags=[AUTH][ASSOC][AUTHORIZED]\n" +
		"rx_bytes=4825331939\n" +
		"tx_bytes=216392802676\n" +
		"signal=-57\n" +
		"connected_time=1846085\n" +
		"d8:3a:dd:72:8e:b1\n" +
		"flags=[AUTH][ASSOC][AUTHORIZED]\n" +
		"rx_bytes=237629088\n" +
		"tx_bytes=190760338\n" +
		"signal=-78\n" +
		"connected_time=3435639\n"

	stas := ParseAllStations(resp)
	if len(stas) != 2 {
		t.Fatalf("got %d stations, want 2", len(stas))
	}
	if stas[0]["addr"] != "c8:69:cd:69:35:da" {
		t.Errorf("sta[0] addr = %q", stas[0]["addr"])
	}
	if stas[0]["signal"] != "-57" {
		t.Errorf("sta[0] signal = %q", stas[0]["signal"])
	}
	if stas[1]["addr"] != "d8:3a:dd:72:8e:b1" {
		t.Errorf("sta[1] addr = %q", stas[1]["addr"])
	}
	if stas[1]["rx_bytes"] != "237629088" {
		t.Errorf("sta[1] rx_bytes = %q", stas[1]["rx_bytes"])
	}
}

func TestParseAllStationsEmpty(t *testing.T) {
	stas := ParseAllStations("")
	if len(stas) != 0 {
		t.Errorf("expected empty, got %d", len(stas))
	}
}

func TestIsMACAddress(t *testing.T) {
	if !isMACAddress("c8:69:cd:69:35:da") {
		t.Error("valid MAC rejected")
	}
	if isMACAddress("not-a-mac") {
		t.Error("invalid string accepted")
	}
	if isMACAddress("signal=-57") {
		t.Error("key=value accepted as MAC")
	}
}

func TestFrequencyToChannel(t *testing.T) {
	tests := []struct {
		freq int
		ch   int
	}{
		{2412, 1},
		{2437, 6},
		{2462, 11},
		{2484, 14},
		{5180, 36},
		{5240, 48},
		{5745, 149},
		{5825, 165},
		{5955, 1},
		{6115, 33},
		{1000, 0},
	}
	for _, tt := range tests {
		got := FrequencyToChannel(tt.freq)
		if got != tt.ch {
			t.Errorf("FrequencyToChannel(%d) = %d, want %d", tt.freq, got, tt.ch)
		}
	}
}

func TestDialAndCommand(t *testing.T) {
	dir := t.TempDir()
	serverPath := dir + "/test_server"
	clientDone := make(chan struct{})

	serverAddr := &net.UnixAddr{Name: serverPath, Net: "unixgram"}
	server, err := net.ListenUnixgram("unixgram", serverAddr)
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer server.Close()

	go func() {
		defer close(clientDone)
		buf := make([]byte, 4096)
		n, raddr, err := server.ReadFromUnix(buf)
		if err != nil {
			t.Errorf("server read: %v", err)
			return
		}
		cmd := string(buf[:n])
		var resp string
		switch cmd {
		case "PING":
			resp = "PONG\n"
		case "STATUS":
			resp = "wpa_state=COMPLETED\nssid=Test\n"
		default:
			resp = "UNKNOWN COMMAND\n"
		}
		server.WriteToUnix([]byte(resp), raddr)

		n, raddr, err = server.ReadFromUnix(buf)
		if err != nil {
			t.Errorf("server read 2: %v", err)
			return
		}
		if string(buf[:n]) == "STATUS" {
			server.WriteToUnix([]byte("wpa_state=COMPLETED\nssid=Test\n"), raddr)
		}
	}()

	conn, err := DialTimeout(serverPath, 2*time.Second)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer conn.Close()

	if !conn.Ping() {
		t.Error("Ping failed")
	}

	status, err := conn.Status()
	if err != nil {
		t.Fatalf("Status: %v", err)
	}
	if status["ssid"] != "Test" {
		t.Errorf("ssid = %q, want Test", status["ssid"])
	}

	<-clientDone
	os.Remove(conn.local)
}
