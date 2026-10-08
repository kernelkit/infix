// Package unixgram dials AF_UNIX datagram servers that reply to the
// client's own bound address: ptp4l, wpa_supplicant and hostapd.  The
// client socket file is created and removed here, so every caller
// cleans up the same way on every error path.
package unixgram

import (
	"fmt"
	"net"
	"os"
	"path/filepath"
	"sync"
)

// Conn is a connected datagram socket that owns its local socket file.
type Conn struct {
	*net.UnixConn
	local string
}

// Dial binds local, replacing a stale file left by a killed process,
// and connects it to remote.  A non-zero mode is applied to the local
// file, for servers that run unprivileged and must be able to reply.
func Dial(local, remote string, mode os.FileMode) (*Conn, error) {
	if err := os.MkdirAll(filepath.Dir(local), 0755); err != nil {
		return nil, err
	}
	os.Remove(local)

	laddr := &net.UnixAddr{Name: local, Net: "unixgram"}
	raddr := &net.UnixAddr{Name: remote, Net: "unixgram"}
	conn, err := net.DialUnix("unixgram", laddr, raddr)
	if err != nil {
		os.Remove(local)
		return nil, fmt.Errorf("dial %s: %w", remote, err)
	}

	if mode != 0 {
		if err := os.Chmod(local, mode); err != nil {
			conn.Close()
			os.Remove(local)
			return nil, err
		}
	}

	return &Conn{UnixConn: conn, local: local}, nil
}

// Close closes the socket and removes the local socket file.
func (c *Conn) Close() error {
	err := c.UnixConn.Close()
	os.Remove(c.local)
	return err
}

var cleaned sync.Map

// CleanDir removes every file in dir, once per process.  Callers keep
// their client sockets in a directory of their own and clean it before
// first use, so sockets left behind by a killed yangerd do not pile up.
func CleanDir(dir string) {
	if _, done := cleaned.LoadOrStore(dir, true); done {
		return
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}
	for _, e := range entries {
		os.Remove(filepath.Join(dir, e.Name()))
	}
}
