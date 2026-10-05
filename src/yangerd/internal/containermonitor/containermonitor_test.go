package containermonitor

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/kernelkit/infix/src/yangerd/internal/tree"
)

// newTestMonitor builds a monitor whose collect() is driven by the test.
// cmd/fs are nil since collect is overridden, so the default closure that
// would use them is never called.
func newTestMonitor(t *testing.T, collect func() json.RawMessage) (*ContainerMonitor, *tree.Tree) {
	t.Helper()
	tr := tree.New()
	m := New(tr, nil, nil, nil)
	m.collect = collect
	return m, tr
}

func TestUpdateTreeSetsContainers(t *testing.T) {
	m, tr := newTestMonitor(t, func() json.RawMessage {
		return json.RawMessage(`{"container":[{"name":"web"}]}`)
	})

	m.updateTree()

	got := tr.Get(treeKey)
	if got == nil || !strings.Contains(string(got), "web") {
		t.Fatalf("expected container data, got %s", got)
	}
}

// With no containers the key must be deleted, not left as an empty node,
// so an idle-but-enabled container feature reads as absent.
func TestUpdateTreeDeletesWhenEmpty(t *testing.T) {
	m, tr := newTestMonitor(t, func() json.RawMessage { return nil })

	tr.Set(treeKey, json.RawMessage(`{"container":[{"name":"old"}]}`))
	m.updateTree()

	if got := tr.Get(treeKey); got != nil {
		t.Fatalf("expected key removed when no containers, got %s", got)
	}
}

// An event in the stream must trigger a re-read; here the re-read clears a
// previously-present container, proving the stream drives reconciliation.
func TestEventTriggersRefresh(t *testing.T) {
	calls := 0
	m, tr := newTestMonitor(t, func() json.RawMessage {
		calls++
		return nil // container is gone
	})
	tr.Set(treeKey, json.RawMessage(`{"container":[{"name":"gone"}]}`))

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go m.refreshLoop(ctx)

	// A container "died" event, newline-framed as podman emits it.
	go m.readEvents(strings.NewReader(`{"Type":"container","Status":"died","Name":"gone"}` + "\n"))

	// Must comfortably exceed debounceDelay, or this races the re-read.
	deadline := time.After(debounceDelay + 3*time.Second)
	for {
		if tr.Get(treeKey) == nil && calls > 0 {
			break
		}
		select {
		case <-deadline:
			t.Fatalf("event did not trigger reconcile; calls=%d tree=%s", calls, tr.Get(treeKey))
		default:
			time.Sleep(10 * time.Millisecond)
		}
	}
}

func TestEventStatus(t *testing.T) {
	if s := eventStatus([]byte(`{"Status":"start"}`)); s != "start" {
		t.Errorf("eventStatus = %q, want start", s)
	}
	if s := eventStatus([]byte(`not json`)); s != "" {
		t.Errorf("eventStatus on garbage = %q, want empty", s)
	}
}
