package wpactrl

import (
	"net"
	"strings"
	"testing"
	"time"
)

// fakeHostapd serves the hostapd control protocol for station
// enumeration: STA-FIRST returns the first station block, STA-NEXT <addr>
// the one after it, and an empty datagram past the last station.
func fakeHostapd(t *testing.T, stations []string) string {
	t.Helper()

	dir := t.TempDir()
	serverPath := dir + "/wlan0"

	serverAddr := &net.UnixAddr{Name: serverPath, Net: "unixgram"}
	server, err := net.ListenUnixgram("unixgram", serverAddr)
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	t.Cleanup(func() { server.Close() })

	addrOf := func(block string) string {
		return strings.SplitN(block, "\n", 2)[0]
	}

	go func() {
		buf := make([]byte, 4096)
		for {
			n, raddr, err := server.ReadFromUnix(buf)
			if err != nil {
				return
			}
			cmd := string(buf[:n])

			var resp string
			switch {
			case cmd == "STA-FIRST":
				if len(stations) > 0 {
					resp = stations[0]
				}
			case strings.HasPrefix(cmd, "STA-NEXT "):
				prev := strings.TrimPrefix(cmd, "STA-NEXT ")
				for i, st := range stations {
					if addrOf(st) == prev && i+1 < len(stations) {
						resp = stations[i+1]
						break
					}
				}
			default:
				resp = "UNKNOWN COMMAND\n"
			}
			server.WriteToUnix([]byte(resp), raddr)
		}
	}()

	return serverPath
}

func TestAllStations(t *testing.T) {
	sta1 := "02:00:00:00:00:01\nflags=[AUTH][ASSOC][AUTHORIZED]\n" +
		"signal=-57\nconnected_time=120\nrx_bytes=1000\ntx_bytes=2000\n"
	sta2 := "02:00:00:00:00:02\nflags=[AUTH][ASSOC][AUTHORIZED]\n" +
		"signal=-78\nconnected_time=60\nrx_bytes=300\ntx_bytes=400\n"

	path := fakeHostapd(t, []string{sta1, sta2})

	conn, err := DialTimeout(path, 2*time.Second)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer conn.Close()

	stas, err := conn.AllStations()
	if err != nil {
		t.Fatalf("AllStations: %v", err)
	}
	if len(stas) != 2 {
		t.Fatalf("got %d stations, want 2", len(stas))
	}
	if stas[0]["addr"] != "02:00:00:00:00:01" || stas[1]["addr"] != "02:00:00:00:00:02" {
		t.Errorf("addrs = %q, %q", stas[0]["addr"], stas[1]["addr"])
	}
	if stas[0]["signal"] != "-57" {
		t.Errorf("sta[0] signal = %q", stas[0]["signal"])
	}
	if stas[1]["connected_time"] != "60" {
		t.Errorf("sta[1] connected_time = %q", stas[1]["connected_time"])
	}
}

func TestAllStationsNone(t *testing.T) {
	path := fakeHostapd(t, nil)

	conn, err := DialTimeout(path, 2*time.Second)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer conn.Close()

	stas, err := conn.AllStations()
	if err != nil {
		t.Fatalf("AllStations: %v", err)
	}
	if len(stas) != 0 {
		t.Fatalf("got %d stations, want 0", len(stas))
	}
}

func TestAllStationsUnsupported(t *testing.T) {
	dir := t.TempDir()
	serverPath := dir + "/wlan0"

	serverAddr := &net.UnixAddr{Name: serverPath, Net: "unixgram"}
	server, err := net.ListenUnixgram("unixgram", serverAddr)
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer server.Close()

	go func() {
		buf := make([]byte, 4096)
		n, raddr, err := server.ReadFromUnix(buf)
		if err != nil || n == 0 {
			return
		}
		server.WriteToUnix([]byte("UNKNOWN COMMAND\n"), raddr)
	}()

	conn, err := DialTimeout(serverPath, 2*time.Second)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer conn.Close()

	if _, err := conn.AllStations(); err == nil {
		t.Fatal("expected error for UNKNOWN COMMAND")
	}
}
