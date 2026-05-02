package wpactrl

import (
	"context"
	"fmt"
	"net"
	"os"
	"strings"
	"time"
)

const attachBufSize = 4096

// Event is an unsolicited event from wpa_supplicant or hostapd,
// received after sending the ATTACH command.
type Event struct {
	Priority int
	Name     string
	Data     string
	Raw      string
}

// EventHandler is called for each unsolicited event.
type EventHandler func(Event)

// AttachConn is a persistent event listener on a wpa_supplicant or
// hostapd control socket.  After sending ATTACH, the daemon pushes
// unsolicited events like CTRL-EVENT-SIGNAL-CHANGE, AP-STA-CONNECTED,
// etc.  The connection reads these in a loop and dispatches them to a
// handler.
type AttachConn struct {
	conn    *net.UnixConn
	local   string
	handler EventHandler
}

// Attach connects to the control socket at serverPath and sends the
// ATTACH command.  On success, the daemon will send unsolicited events
// to this connection.  Call Run to start reading them.
func Attach(serverPath string) (*AttachConn, error) {
	seq := clientSeq.Add(1)
	localPath := fmt.Sprintf("/tmp/wpactrl_attach_%d_%d", os.Getpid(), seq)
	os.Remove(localPath)

	laddr := &net.UnixAddr{Name: localPath, Net: "unixgram"}
	raddr := &net.UnixAddr{Name: serverPath, Net: "unixgram"}

	conn, err := net.DialUnix("unixgram", laddr, raddr)
	if err != nil {
		os.Remove(localPath)
		return nil, fmt.Errorf("dial %s: %w", serverPath, err)
	}

	conn.SetDeadline(time.Now().Add(DefaultTimeout))
	if _, err := conn.Write([]byte("ATTACH")); err != nil {
		conn.Close()
		os.Remove(localPath)
		return nil, fmt.Errorf("send ATTACH: %w", err)
	}

	buf := make([]byte, 64)
	n, err := conn.Read(buf)
	if err != nil {
		conn.Close()
		os.Remove(localPath)
		return nil, fmt.Errorf("read ATTACH response: %w", err)
	}
	resp := strings.TrimSpace(string(buf[:n]))
	if resp != "OK" {
		conn.Close()
		os.Remove(localPath)
		return nil, fmt.Errorf("ATTACH rejected: %q", resp)
	}

	conn.SetDeadline(time.Time{})
	return &AttachConn{conn: conn, local: localPath}, nil
}

// SetHandler sets the callback for received events.
func (a *AttachConn) SetHandler(fn EventHandler) {
	a.handler = fn
}

// Run reads events until ctx is cancelled or the socket errors (daemon
// died).  Returns nil on context cancellation, error on socket failure.
func (a *AttachConn) Run(ctx context.Context) error {
	done := make(chan struct{})
	go func() {
		select {
		case <-ctx.Done():
			a.conn.SetReadDeadline(time.Now())
		case <-done:
		}
	}()
	defer close(done)

	buf := make([]byte, attachBufSize)
	for {
		n, err := a.conn.Read(buf)
		if err != nil {
			if ctx.Err() != nil {
				return nil
			}
			return fmt.Errorf("read: %w", err)
		}
		if a.handler == nil {
			continue
		}
		ev, ok := ParseEvent(string(buf[:n]))
		if ok {
			a.handler(ev)
		}
	}
}

// Close sends DETACH and closes the connection.
func (a *AttachConn) Close() error {
	a.conn.SetDeadline(time.Now().Add(DefaultTimeout))
	a.conn.Write([]byte("DETACH"))
	err := a.conn.Close()
	os.Remove(a.local)
	return err
}

// ParseEvent parses a single unsolicited event line.  Format:
// <N>EVENT-NAME optional-data
// where N is a priority digit (0-4).  Some events like
// AP-STA-CONNECTED have no priority prefix.
func ParseEvent(line string) (Event, bool) {
	line = strings.TrimSpace(line)
	if line == "" {
		return Event{}, false
	}

	ev := Event{Raw: line}

	if len(line) >= 3 && line[0] == '<' {
		end := strings.IndexByte(line, '>')
		if end > 1 {
			for _, c := range line[1:end] {
				if c < '0' || c > '9' {
					goto noPriority
				}
			}
			fmt.Sscanf(line[1:end], "%d", &ev.Priority)
			line = line[end+1:]
		}
	}
noPriority:

	if idx := strings.IndexByte(line, ' '); idx > 0 {
		ev.Name = line[:idx]
		ev.Data = line[idx+1:]
	} else {
		ev.Name = line
	}

	return ev, ev.Name != ""
}
