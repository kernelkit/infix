// Package wpactrl provides a native Go client for wpa_supplicant and
// hostapd control sockets.  It speaks the same text-based protocol as
// wpa_cli/hostapd_cli — Unix datagram sockets with ASCII
// command/response framing.  No subprocess, no CGo.
//
// wpa_supplicant listens at /var/run/wpa_supplicant/<ifname>
// hostapd      listens at /var/run/hostapd/<ifname>
//
// The client binds its own socket, sends a command string, and reads
// back the text response.
package wpactrl

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"time"

	"github.com/kernelkit/infix/src/yangerd/internal/unixgram"
)

const (
	DefaultTimeout = 5 * time.Second

	maxResponse = 64 * 1024
)

// WPADirs lists directories where wpa_supplicant control sockets may live.
var WPADirs = []string{"/run/wpa_supplicant", "/var/run/wpa_supplicant"}

// HostapdDirs lists directories where hostapd control sockets may live.
var HostapdDirs = []string{"/run/hostapd", "/var/run/hostapd"}

var clientSeq atomic.Uint64

// localDir holds our client sockets, cleaned before first use so ones
// left by a killed yangerd do not pile up.
var localDir = "/run/yangerd/wpactrl"

// dial binds a fresh client socket and connects it to serverPath.
func dial(kind, serverPath string) (*unixgram.Conn, error) {
	unixgram.CleanDir(localDir)
	local := fmt.Sprintf("%s/%s%d", localDir, kind, clientSeq.Add(1))
	return unixgram.Dial(local, serverPath, 0)
}

// SocketInfo describes a discovered control socket.
type SocketInfo struct {
	Path   string
	Iface  string
	Daemon string // "wpa_supplicant" or "hostapd"
}

// ScanSockets discovers wpa_supplicant and hostapd control sockets by
// listing the well-known directories.  Returns a map from interface
// name to SocketInfo.
func ScanSockets() map[string]SocketInfo {
	result := make(map[string]SocketInfo)
	for _, dir := range HostapdDirs {
		scanDir(dir, "hostapd", result)
	}
	for _, dir := range WPADirs {
		scanDir(dir, "wpa_supplicant", result)
	}
	return result
}

func scanDir(dir, daemon string, out map[string]SocketInfo) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}
	for _, e := range entries {
		name := e.Name()
		if _, exists := out[name]; exists {
			continue
		}
		path := filepath.Join(dir, name)
		fi, err := os.Stat(path)
		if err != nil {
			continue
		}
		if fi.Mode()&os.ModeSocket != 0 {
			out[name] = SocketInfo{
				Path:   path,
				Iface:  name,
				Daemon: daemon,
			}
		}
	}
}

// Conn is a connection to a wpa_supplicant or hostapd control socket.
type Conn struct {
	conn    *unixgram.Conn
	timeout time.Duration
}

// Dial connects to a wpa_supplicant or hostapd control socket at the
// given path (e.g. "/var/run/wpa_supplicant/wlan0").  The caller must
// call Close when done.
func Dial(serverPath string) (*Conn, error) {
	return DialTimeout(serverPath, DefaultTimeout)
}

// DialTimeout connects with a custom timeout.
func DialTimeout(serverPath string, timeout time.Duration) (*Conn, error) {
	conn, err := dial("c", serverPath)
	if err != nil {
		return nil, err
	}
	return &Conn{conn: conn, timeout: timeout}, nil
}

// Close closes the connection and removes the client socket file.
func (c *Conn) Close() error {
	return c.conn.Close()
}

// Command sends a command string and returns the response.
func (c *Conn) Command(cmd string) (string, error) {
	c.conn.SetDeadline(time.Now().Add(c.timeout))

	_, err := c.conn.Write([]byte(cmd))
	if err != nil {
		return "", fmt.Errorf("write %q: %w", cmd, err)
	}

	buf := make([]byte, maxResponse)
	n, err := c.conn.Read(buf)
	if err != nil {
		return "", fmt.Errorf("read response to %q: %w", cmd, err)
	}

	return string(buf[:n]), nil
}

// Status sends the STATUS command and returns the parsed key=value pairs.
func (c *Conn) Status() (map[string]string, error) {
	resp, err := c.Command("STATUS")
	if err != nil {
		return nil, err
	}
	return ParseKV(resp), nil
}

// SignalPoll sends SIGNAL_POLL and returns parsed key=value pairs.
// Returns RSSI, LINKSPEED, NOISE, FREQUENCY, etc.
// Only meaningful for wpa_supplicant (station mode).
func (c *Conn) SignalPoll() (map[string]string, error) {
	resp, err := c.Command("SIGNAL_POLL")
	if err != nil {
		return nil, err
	}
	return ParseKV(resp), nil
}

// ScanResults sends SCAN_RESULTS and returns parsed results.
// This is only meaningful for wpa_supplicant (station mode).
func (c *Conn) ScanResults() ([]ScanResult, error) {
	resp, err := c.Command("SCAN_RESULTS")
	if err != nil {
		return nil, err
	}
	return ParseScanResults(resp), nil
}

// AllStations enumerates all associated stations via STA-FIRST/STA-NEXT.
// Only meaningful for hostapd.
func (c *Conn) AllStations() ([]map[string]string, error) {
	resp, err := c.Command("STA-FIRST")
	if err != nil {
		return nil, fmt.Errorf("STA-FIRST: %w", err)
	}
	if resp == "" || resp == "\n" || resp == "FAIL\n" {
		return nil, nil
	}
	if strings.HasPrefix(resp, "UNKNOWN") {
		return nil, fmt.Errorf("STA-FIRST not supported: %q", strings.TrimSpace(resp))
	}

	var stations []map[string]string
	st := ParseStationResp(resp)
	for st != nil {
		stations = append(stations, st)
		addr := st["addr"]
		if addr == "" {
			break
		}
		resp, err = c.Command("STA-NEXT " + addr)
		if err != nil || resp == "" || resp == "\n" || resp == "FAIL\n" || strings.HasPrefix(resp, "UNKNOWN") {
			break
		}
		st = ParseStationResp(resp)
	}
	return stations, nil
}
