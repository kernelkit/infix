package containermonitor

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
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

// A state file appearing in a new container directory must trigger a
// re-read; here the re-read clears a previously-present container,
// proving the watch drives reconciliation.
func TestStateFileTriggersRefresh(t *testing.T) {
	StateDir = t.TempDir()

	calls := 0
	m, tr := newTestMonitor(t, func() json.RawMessage {
		calls++
		return nil // container is gone
	})
	tr.Set(treeKey, json.RawMessage(`{"container":[{"name":"gone"}]}`))

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go m.refreshLoop(ctx)
	go m.runOnce(ctx)

	// Give runOnce time to arm the watch, then do what ixcont does.
	time.Sleep(100 * time.Millisecond)
	dir := filepath.Join(StateDir, "gone")
	if err := os.Mkdir(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	time.Sleep(100 * time.Millisecond)
	if err := os.WriteFile(filepath.Join(dir, "state.json"), []byte(`{"name":"gone"}`), 0o644); err != nil {
		t.Fatal(err)
	}

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

// The update hook must run after every re-read, with and without
// containers, since the hook's own state depends on both.
func TestUpdateTreeRunsHook(t *testing.T) {
	have := true
	m, _ := newTestMonitor(t, func() json.RawMessage {
		if have {
			return json.RawMessage(`{"container":[{"name":"web"}]}`)
		}
		return nil
	})
	calls := 0
	m.SetOnUpdate(func() { calls++ })

	m.updateTree()
	have = false
	m.updateTree()

	if calls != 2 {
		t.Fatalf("hook ran %d times, want 2", calls)
	}
}
