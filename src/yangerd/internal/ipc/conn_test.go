package ipc

import (
	"context"
	"encoding/json"
	"net"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/kernelkit/infix/src/yangerd/internal/tree"
)

func startServer(t *testing.T, timeout time.Duration) (string, context.CancelFunc, chan error) {
	t.Helper()
	sockPath := filepath.Join(t.TempDir(), "conn.sock")
	ready := &atomic.Bool{}
	ready.Store(true)
	srv := NewServer(tree.New(), ready)
	srv.timeout = timeout
	if err := srv.Listen(sockPath); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- srv.Serve(ctx) }()
	return sockPath, cancel, done
}

// A client that connects and never sends is dropped at the deadline.
func TestServerIdleConnDeadline(t *testing.T) {
	sockPath, cancel, _ := startServer(t, 200*time.Millisecond)
	defer cancel()

	conn, err := net.Dial("unix", sockPath)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()

	conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	_, err = conn.Read(make([]byte, 1))
	if err == nil {
		t.Fatal("expected the server to close an idle connection")
	}
	if ne, ok := err.(net.Error); ok && ne.Timeout() {
		t.Fatal("server kept an idle connection open past its deadline")
	}
}

// Shutdown must not wait for a connection that is still open.
func TestServerShutdownClosesConns(t *testing.T) {
	sockPath, cancel, done := startServer(t, time.Hour)

	conn, err := net.Dial("unix", sockPath)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	time.Sleep(20 * time.Millisecond)

	cancel()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("Serve did not return with a connection still open")
	}
}

// Data travels in its own frame, byte for byte.
func TestServerGetRawDataFrame(t *testing.T) {
	tr := tree.New()
	tr.Set("ietf-system:system", json.RawMessage(`{"hostname":"r1"}`))

	resp := serverRoundTrip(t, tr, true, &Request{Method: "get", Path: "/ietf-system:system"})
	if !resp.Raw {
		t.Fatal("expected raw data frame")
	}
	if string(resp.Data) != `{"ietf-system:system":{"hostname":"r1"}}` {
		t.Fatalf("data = %s", resp.Data)
	}
}
