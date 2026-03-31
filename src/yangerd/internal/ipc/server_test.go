package ipc

import (
	"context"
	"encoding/json"
	"net"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/kernelkit/infix/src/yangerd/internal/tree"
)

func TestServerGetSingle(t *testing.T) {
	tr := tree.New()
	tr.Set("ietf-system:system-state", json.RawMessage(`{"platform":{"os-name":"Infix"}}`))

	resp := serverRoundTrip(t, tr, true, &Request{Method: "get", Path: "/ietf-system:system-state"})

	if resp.Status != "ok" {
		t.Fatalf("expected ok, got %s: %s", resp.Status, resp.Message)
	}
	var data map[string]json.RawMessage
	json.Unmarshal(resp.Data, &data)
	if _, ok := data["ietf-system:system-state"]; !ok {
		t.Fatalf("missing key in response data: %s", resp.Data)
	}
}

func TestServerGetNotFound(t *testing.T) {
	tr := tree.New()
	resp := serverRoundTrip(t, tr, true, &Request{Method: "get", Path: "/nonexistent"})

	if resp.Status != "error" || resp.Code != 404 {
		t.Fatalf("expected 404 error, got %+v", resp)
	}
}

func TestServerDump(t *testing.T) {
	tr := tree.New()
	tr.Set("a", json.RawMessage(`1`))
	tr.Set("b", json.RawMessage(`2`))

	resp := serverRoundTrip(t, tr, true, &Request{Method: "get", Path: "/"})

	if resp.Status != "ok" {
		t.Fatalf("expected ok, got %s: %s", resp.Status, resp.Message)
	}
	var data map[string]json.RawMessage
	json.Unmarshal(resp.Data, &data)
	if len(data) != 2 {
		t.Fatalf("expected 2 models in dump, got %d", len(data))
	}
}

func TestServerHealth(t *testing.T) {
	tr := tree.New()
	tr.Set("model-a", json.RawMessage(`{}`))

	resp := serverRoundTrip(t, tr, true, &Request{Method: "health"})

	if resp.Status != "ok" {
		t.Fatalf("expected ok, got %s", resp.Status)
	}
	if _, ok := resp.Models["model-a"]; !ok {
		t.Fatalf("expected model-a in health models, got %v", resp.Models)
	}
}

func TestServerNotReady(t *testing.T) {
	tr := tree.New()
	resp := serverRoundTrip(t, tr, false, &Request{Method: "get", Path: "/"})

	if resp.Status != "starting" || resp.Code != 503 {
		t.Fatalf("expected 503 starting, got %+v", resp)
	}
}

func TestServerUnknownMethod(t *testing.T) {
	tr := tree.New()
	resp := serverRoundTrip(t, tr, true, &Request{Method: "invalid"})

	if resp.Status != "error" || resp.Code != 400 {
		t.Fatalf("expected 400 error, got %+v", resp)
	}
}

func serverRoundTrip(t *testing.T, tr *tree.Tree, ready bool, req *Request) *Response {
	t.Helper()

	sockPath := filepath.Join(t.TempDir(), "test.sock")
	readyFlag := &atomic.Bool{}
	readyFlag.Store(ready)

	srv := NewServer(tr, readyFlag)
	if err := srv.Listen(sockPath); err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	done := make(chan error, 1)
	go func() {
		done <- srv.Serve(ctx)
	}()

	time.Sleep(10 * time.Millisecond)

	conn, err := net.Dial("unix", sockPath)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()

	payload, _ := json.Marshal(req)
	if err := WriteFrame(conn, payload); err != nil {
		t.Fatal(err)
	}

	resp, err := ReadResponse(conn)
	if err != nil {
		t.Fatal(err)
	}

	cancel()

	if _, err := os.Stat(sockPath); err == nil {
		os.Remove(sockPath)
	}

	return resp
}
