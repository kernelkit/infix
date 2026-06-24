// Package frrvty is a minimal in-process client for an FRR daemon's vty
// Unix socket.
//
// It speaks the same protocol vtysh uses, so yangerd can run "show ..."
// commands (e.g. "show ip route json") against zebra without forking
// vtysh.  The command is written NUL-terminated; the daemon streams the
// command output followed by a four-byte trailer of three NUL bytes and a
// one-byte CLI return code (\0\0\0<ret>).
package frrvty

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net"
)

// ZebraVtySocket is the default path to zebra's vty socket.  It lives in
// the same runstatedir as the zserv API socket.
const ZebraVtySocket = "/var/run/frr/zebra.vty"

// Client runs commands against a single FRR daemon vty socket.  A fresh
// connection is opened per query, matching vtysh's behaviour.
type Client struct {
	socket string
}

// New returns a Client for the given vty socket path.  An empty path
// selects the zebra socket.
func New(socket string) *Client {
	if socket == "" {
		socket = ZebraVtySocket
	}
	return &Client{socket: socket}
}

// Query connects to the vty socket, runs one command, and returns its raw
// output with the protocol trailer stripped.  A non-zero CLI return code
// is reported as an error (the partial output is still returned).
func (c *Client) Query(ctx context.Context, command string) ([]byte, error) {
	var d net.Dialer
	conn, err := d.DialContext(ctx, "unix", c.socket)
	if err != nil {
		return nil, fmt.Errorf("dial %s: %w", c.socket, err)
	}
	defer conn.Close()

	if deadline, ok := ctx.Deadline(); ok {
		_ = conn.SetDeadline(deadline)
	}

	// vtysh writes the command including its trailing NUL terminator.
	if _, err := conn.Write(append([]byte(command), 0)); err != nil {
		return nil, fmt.Errorf("write %q: %w", command, err)
	}

	var buf bytes.Buffer
	tmp := make([]byte, 4096)
	for {
		n, rerr := conn.Read(tmp)
		if n > 0 {
			buf.Write(tmp[:n])
			// The response ends with \0\0\0<ret>.  The payload is
			// text/JSON and never contains NUL, so testing the last
			// four accumulated bytes is unambiguous.
			if b := buf.Bytes(); len(b) >= 4 &&
				b[len(b)-4] == 0 && b[len(b)-3] == 0 && b[len(b)-2] == 0 {
				payload := b[:len(b)-4]
				if ret := b[len(b)-1]; ret != 0 {
					return payload, fmt.Errorf("vty command %q: status %d", command, ret)
				}
				return payload, nil
			}
		}
		if rerr != nil {
			if rerr == io.EOF {
				return nil, fmt.Errorf("vty command %q: closed before trailer", command)
			}
			return nil, fmt.Errorf("vty command %q: read: %w", command, rerr)
		}
	}
}
