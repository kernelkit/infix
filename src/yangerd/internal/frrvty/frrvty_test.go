package frrvty

import (
	"bytes"
	"context"
	"net"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

// fakeZebra serves a single vty connection like an FRR daemon whose show
// commands live in the view node: "enable" and the command both work.
func fakeZebra(t *testing.T, reply string, ret byte) string {
	return fakeDaemon(t, reply, ret, false)
}

// fakeDaemon serves a single vty connection.  It answers each
// NUL-terminated command with a \0\0\0<ret> trailer: "enable" with an
// empty reply, anything else with the configured reply.  With
// enableOnly it behaves like bfdd, whose show commands exist only in
// the enable node, and rejects a command sent before "enable".
func fakeDaemon(t *testing.T, reply string, ret byte, enableOnly bool) string {
	t.Helper()

	sock := filepath.Join(t.TempDir(), "daemon.vty")
	ln, err := net.Listen("unix", sock)
	if err != nil {
		t.Fatalf("listen: %v", err)
	}

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		conn, err := ln.Accept()
		if err != nil {
			return
		}
		defer conn.Close()

		enabled := false
		var pending []byte
		buf := make([]byte, 256)
		for {
			n, err := conn.Read(buf)
			pending = append(pending, buf[:n]...)
			for {
				i := bytes.IndexByte(pending, 0)
				if i < 0 {
					break
				}
				cmd := string(pending[:i])
				pending = pending[i+1:]

				var out []byte
				switch {
				case cmd == "enable":
					enabled = true
					out = []byte{0, 0, 0, 0}
				case enableOnly && !enabled:
					out = append([]byte("% Unknown command: "+cmd+"\n"), 0, 0, 0, 2)
				default:
					out = append([]byte(reply), 0, 0, 0, ret)
				}
				if _, werr := conn.Write(out); werr != nil {
					return
				}
			}
			if err != nil {
				return
			}
		}
	}()

	t.Cleanup(func() {
		ln.Close()
		wg.Wait()
	})
	return sock
}

// bfdd installs "show bfd peers" in the enable node only, so the query
// must enter it first or the daemon answers "Unknown command".
func TestQueryEntersEnableNode(t *testing.T) {
	sock := fakeDaemon(t, `[{"peer":"192.168.100.2","status":"up"}]`, 0, true)

	out, err := New(sock).Query(context.Background(), "show bfd peers json")
	if err != nil {
		t.Fatalf("Query: %v", err)
	}
	if got := string(out); got != `[{"peer":"192.168.100.2","status":"up"}]` {
		t.Fatalf("output = %q", got)
	}
}

func TestQueryStripsTrailer(t *testing.T) {
	sock := fakeZebra(t, `{"a":1}`, 0)
	c := New(sock)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	out, err := c.Query(ctx, "show ip route json")
	if err != nil {
		t.Fatalf("Query: %v", err)
	}
	if string(out) != `{"a":1}` {
		t.Errorf("output = %q, want %q", out, `{"a":1}`)
	}
}

func TestQueryNonZeroStatus(t *testing.T) {
	sock := fakeZebra(t, "Unknown command", 1)
	c := New(sock)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	out, err := c.Query(ctx, "bogus")
	if err == nil {
		t.Fatal("expected error for non-zero status")
	}
	if string(out) != "Unknown command" {
		t.Errorf("partial output = %q, want %q", out, "Unknown command")
	}
}

func TestQueryDialError(t *testing.T) {
	c := New(filepath.Join(t.TempDir(), "does-not-exist.vty"))

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	if _, err := c.Query(ctx, "show ip route json"); err == nil {
		t.Fatal("expected dial error")
	}
}
