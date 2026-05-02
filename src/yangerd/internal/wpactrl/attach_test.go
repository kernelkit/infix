package wpactrl

import (
	"context"
	"net"
	"os"
	"testing"
	"time"
)

func TestParseEvent(t *testing.T) {
	tests := []struct {
		line     string
		wantOK   bool
		wantPri  int
		wantName string
		wantData string
	}{
		{"<3>CTRL-EVENT-SIGNAL-CHANGE above=0 signal=-88 noise=-92 txrate=6000", true, 3, "CTRL-EVENT-SIGNAL-CHANGE", "above=0 signal=-88 noise=-92 txrate=6000"},
		{"<3>CTRL-EVENT-CONNECTED - Connection to 02:00:00:00:01:00 completed", true, 3, "CTRL-EVENT-CONNECTED", "- Connection to 02:00:00:00:01:00 completed"},
		{"<3>CTRL-EVENT-SCAN-RESULTS ", true, 3, "CTRL-EVENT-SCAN-RESULTS", ""},
		{"<3>CTRL-EVENT-DISCONNECTED bssid=02:00:00:00:01:00 reason=3", true, 3, "CTRL-EVENT-DISCONNECTED", "bssid=02:00:00:00:01:00 reason=3"},
		{"<2>AP-STA-CONNECTED 9e:61:6b:cf:d8:15", true, 2, "AP-STA-CONNECTED", "9e:61:6b:cf:d8:15"},
		{"<2>AP-STA-DISCONNECTED 9e:61:6b:cf:d8:15", true, 2, "AP-STA-DISCONNECTED", "9e:61:6b:cf:d8:15"},
		{"AP-STA-CONNECTED 9e:61:6b:cf:d8:15", true, 0, "AP-STA-CONNECTED", "9e:61:6b:cf:d8:15"},
		{"<3>CTRL-EVENT-TERMINATING", true, 3, "CTRL-EVENT-TERMINATING", ""},
		{"", false, 0, "", ""},
		{"   ", false, 0, "", ""},
	}

	for _, tt := range tests {
		ev, ok := ParseEvent(tt.line)
		if ok != tt.wantOK {
			t.Errorf("ParseEvent(%q): ok=%v, want %v", tt.line, ok, tt.wantOK)
			continue
		}
		if !ok {
			continue
		}
		if ev.Priority != tt.wantPri {
			t.Errorf("ParseEvent(%q): priority=%d, want %d", tt.line, ev.Priority, tt.wantPri)
		}
		if ev.Name != tt.wantName {
			t.Errorf("ParseEvent(%q): name=%q, want %q", tt.line, ev.Name, tt.wantName)
		}
		if ev.Data != tt.wantData {
			t.Errorf("ParseEvent(%q): data=%q, want %q", tt.line, ev.Data, tt.wantData)
		}
	}
}

func TestAttachAndReceiveEvents(t *testing.T) {
	dir := t.TempDir()
	serverPath := dir + "/hostapd_test"

	serverAddr := &net.UnixAddr{Name: serverPath, Net: "unixgram"}
	server, err := net.ListenUnixgram("unixgram", serverAddr)
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer server.Close()

	go func() {
		buf := make([]byte, 4096)
		n, raddr, err := server.ReadFromUnix(buf)
		if err != nil {
			return
		}
		if string(buf[:n]) == "ATTACH" {
			server.WriteToUnix([]byte("OK\n"), raddr)
		}
		time.Sleep(50 * time.Millisecond)
		server.WriteToUnix([]byte("<2>AP-STA-CONNECTED 9e:61:6b:cf:d8:15"), raddr)
		time.Sleep(50 * time.Millisecond)
		server.WriteToUnix([]byte("<3>CTRL-EVENT-SIGNAL-CHANGE above=0 signal=-55"), raddr)

		n, _, err = server.ReadFromUnix(buf)
		if err != nil {
			return
		}
		if string(buf[:n]) == "DETACH" {
			server.WriteToUnix([]byte("OK\n"), raddr)
		}
	}()

	ac, err := Attach(serverPath)
	if err != nil {
		t.Fatalf("Attach: %v", err)
	}
	defer ac.Close()

	var events []Event
	ac.SetHandler(func(ev Event) {
		events = append(events, ev)
	})

	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()

	ac.Run(ctx)

	if len(events) < 2 {
		t.Fatalf("got %d events, want >= 2", len(events))
	}
	if events[0].Name != "AP-STA-CONNECTED" {
		t.Errorf("events[0].Name = %q, want AP-STA-CONNECTED", events[0].Name)
	}
	if events[0].Data != "9e:61:6b:cf:d8:15" {
		t.Errorf("events[0].Data = %q", events[0].Data)
	}
	if events[1].Name != "CTRL-EVENT-SIGNAL-CHANGE" {
		t.Errorf("events[1].Name = %q, want CTRL-EVENT-SIGNAL-CHANGE", events[1].Name)
	}

	os.Remove(ac.local)
}

func TestAttachContextCancel(t *testing.T) {
	dir := t.TempDir()
	serverPath := dir + "/wpa_test"

	serverAddr := &net.UnixAddr{Name: serverPath, Net: "unixgram"}
	server, err := net.ListenUnixgram("unixgram", serverAddr)
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer server.Close()

	go func() {
		buf := make([]byte, 4096)
		n, raddr, err := server.ReadFromUnix(buf)
		if err != nil {
			return
		}
		if string(buf[:n]) == "ATTACH" {
			server.WriteToUnix([]byte("OK\n"), raddr)
		}
		n, _, _ = server.ReadFromUnix(buf)
	}()

	ac, err := Attach(serverPath)
	if err != nil {
		t.Fatalf("Attach: %v", err)
	}
	defer ac.Close()

	ac.SetHandler(func(ev Event) {})

	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()

	err = ac.Run(ctx)
	if err != nil {
		t.Errorf("expected nil on context cancel, got %v", err)
	}
}
