package frrvty

import (
	"context"
	"net"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

// fakeZebra serves a single vty connection: it reads the NUL-terminated
// command, then writes the configured reply followed by the \0\0\0<ret>
// trailer.
func fakeZebra(t *testing.T, reply string, ret byte) string {
	t.Helper()

	sock := filepath.Join(t.TempDir(), "zebra.vty")
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

		// Read the NUL-terminated command.
		buf := make([]byte, 256)
		for {
			n, err := conn.Read(buf)
			if n > 0 && buf[n-1] == 0 {
				break
			}
			if err != nil {
				return
			}
		}

		out := append([]byte(reply), 0, 0, 0, ret)
		_, _ = conn.Write(out)
	}()

	t.Cleanup(func() {
		ln.Close()
		wg.Wait()
	})
	return sock
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
