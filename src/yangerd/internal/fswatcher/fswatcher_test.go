package fswatcher

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/kernelkit/infix/src/yangerd/internal/tree"
)

func newTestFSWatcher(t *testing.T) (*FSWatcher, *tree.Tree) {
	t.Helper()
	tr := tree.New()
	fw, err := New(tr, slog.Default())
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	t.Cleanup(func() { fw.Close() })
	return fw, tr
}

func TestNew(t *testing.T) {
	tr := tree.New()
	fw, err := New(tr, slog.Default())
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer fw.Close()

	if fw.tree != tr {
		t.Error("tree not stored")
	}
	if fw.handlers == nil {
		t.Error("handlers map nil")
	}
	if fw.debounce == nil {
		t.Error("debounce map nil")
	}
}

func TestWatch(t *testing.T) {
	fw, _ := newTestFSWatcher(t)

	tmp := t.TempDir()
	path := filepath.Join(tmp, "test.txt")
	if err := os.WriteFile(path, []byte("hello"), 0644); err != nil {
		t.Fatal(err)
	}

	handler := WatchHandler{
		TreeKey:  "test/key",
		ReadFunc: func(p string) (json.RawMessage, error) { return json.RawMessage(`"ok"`), nil },
	}

	if err := fw.Watch(path, handler); err != nil {
		t.Fatalf("Watch: %v", err)
	}

	fw.mu.Lock()
	_, ok := fw.handlers[path]
	fw.mu.Unlock()
	if !ok {
		t.Error("handler not registered")
	}
}

func TestInitialRead(t *testing.T) {
	fw, tr := newTestFSWatcher(t)

	tmp := t.TempDir()
	p1 := filepath.Join(tmp, "a.txt")
	p2 := filepath.Join(tmp, "b.txt")
	os.WriteFile(p1, []byte("1"), 0644)
	os.WriteFile(p2, []byte("2"), 0644)

	fw.Watch(p1, WatchHandler{
		TreeKey: "key/a",
		ReadFunc: func(path string) (json.RawMessage, error) {
			return json.RawMessage(`"value-a"`), nil
		},
	})
	fw.Watch(p2, WatchHandler{
		TreeKey: "key/b",
		ReadFunc: func(path string) (json.RawMessage, error) {
			return json.RawMessage(`"value-b"`), nil
		},
	})

	fw.InitialRead()

	if got := tr.Get("key/a"); string(got) != `"value-a"` {
		t.Errorf("key/a = %s, want %q", got, `"value-a"`)
	}
	if got := tr.Get("key/b"); string(got) != `"value-b"` {
		t.Errorf("key/b = %s, want %q", got, `"value-b"`)
	}
}

func TestInitialReadError(t *testing.T) {
	fw, tr := newTestFSWatcher(t)

	tmp := t.TempDir()
	p := filepath.Join(tmp, "fail.txt")
	os.WriteFile(p, []byte("x"), 0644)

	fw.Watch(p, WatchHandler{
		TreeKey: "key/fail",
		ReadFunc: func(path string) (json.RawMessage, error) {
			return nil, fmt.Errorf("read error")
		},
	})

	fw.InitialRead()

	if got := tr.Get("key/fail"); got != nil {
		t.Errorf("expected nil for failed read, got %s", got)
	}
}

func TestWatchGlob(t *testing.T) {
	fw, _ := newTestFSWatcher(t)

	tmp := t.TempDir()
	for _, name := range []string{"x1.conf", "x2.conf", "x3.conf"} {
		os.WriteFile(filepath.Join(tmp, name), []byte("data"), 0644)
	}
	os.WriteFile(filepath.Join(tmp, "y.txt"), []byte("data"), 0644)

	handler := WatchHandler{
		TreeKey:  "glob/test",
		ReadFunc: func(p string) (json.RawMessage, error) { return json.RawMessage(`"g"`), nil },
	}

	n, err := fw.WatchGlob(filepath.Join(tmp, "x*.conf"), handler)
	if err != nil {
		t.Fatalf("WatchGlob: %v", err)
	}
	if n != 3 {
		t.Errorf("WatchGlob matched %d, want 3", n)
	}

	fw.mu.Lock()
	count := len(fw.handlers)
	fw.mu.Unlock()
	if count != 3 {
		t.Errorf("handlers count = %d, want 3", count)
	}
}

func TestRunWriteEvent(t *testing.T) {
	fw, tr := newTestFSWatcher(t)

	tmp := t.TempDir()
	path := filepath.Join(tmp, "watched.txt")
	os.WriteFile(path, []byte("initial"), 0644)

	callCount := 0
	fw.Watch(path, WatchHandler{
		TreeKey: "run/test",
		ReadFunc: func(p string) (json.RawMessage, error) {
			callCount++
			return json.RawMessage(`"updated"`), nil
		},
	})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	errCh := make(chan error, 1)
	go func() { errCh <- fw.Run(ctx) }()

	time.Sleep(50 * time.Millisecond)

	os.WriteFile(path, []byte("changed"), 0644)

	deadline := time.After(2 * time.Second)
	for {
		if got := tr.Get("run/test"); string(got) == `"updated"` {
			break
		}
		select {
		case <-deadline:
			t.Fatal("timed out waiting for tree update after write event")
		default:
			time.Sleep(10 * time.Millisecond)
		}
	}

	cancel()
	err := <-errCh
	if err != nil && err != context.Canceled {
		t.Errorf("Run returned unexpected error: %v", err)
	}
}

func TestFireHandler(t *testing.T) {
	fw, tr := newTestFSWatcher(t)

	handler := WatchHandler{
		TreeKey: "fire/test",
		ReadFunc: func(path string) (json.RawMessage, error) {
			return json.RawMessage(`{"fired":true}`), nil
		},
	}

	fw.fireHandler("/fake/path", handler)

	if got := tr.Get("fire/test"); string(got) != `{"fired":true}` {
		t.Errorf("tree value = %s, want %s", got, `{"fired":true}`)
	}
}

func TestFireHandlerReadError(t *testing.T) {
	fw, tr := newTestFSWatcher(t)

	handler := WatchHandler{
		TreeKey: "fire/err",
		ReadFunc: func(path string) (json.RawMessage, error) {
			return nil, fmt.Errorf("broken")
		},
	}

	fw.fireHandler("/fake/path", handler)

	if got := tr.Get("fire/err"); got != nil {
		t.Errorf("expected nil for errored handler, got %s", got)
	}
}

func TestDebounce(t *testing.T) {
	fw, tr := newTestFSWatcher(t)

	tmp := t.TempDir()
	path := filepath.Join(tmp, "debounce.txt")
	os.WriteFile(path, []byte("init"), 0644)

	callCount := 0
	fw.Watch(path, WatchHandler{
		TreeKey:  "debounce/test",
		Debounce: 100 * time.Millisecond,
		ReadFunc: func(p string) (json.RawMessage, error) {
			callCount++
			return json.RawMessage(fmt.Sprintf(`"call-%d"`, callCount)), nil
		},
	})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go fw.Run(ctx)
	time.Sleep(50 * time.Millisecond)

	for i := 0; i < 5; i++ {
		os.WriteFile(path, []byte(fmt.Sprintf("data-%d", i)), 0644)
		time.Sleep(10 * time.Millisecond)
	}

	time.Sleep(300 * time.Millisecond)

	got := tr.Get("debounce/test")
	if got == nil {
		t.Fatal("tree not updated after debounced writes")
	}

	if callCount > 3 {
		t.Errorf("expected debounce to coalesce writes, but handler called %d times", callCount)
	}

	cancel()
}

func TestRunContextCancellation(t *testing.T) {
	fw, _ := newTestFSWatcher(t)

	ctx, cancel := context.WithCancel(context.Background())

	errCh := make(chan error, 1)
	go func() { errCh <- fw.Run(ctx) }()

	time.Sleep(20 * time.Millisecond)
	cancel()

	err := <-errCh
	if err != context.Canceled {
		t.Errorf("Run error = %v, want context.Canceled", err)
	}
}

func TestClose(t *testing.T) {
	tr := tree.New()
	fw, err := New(tr, slog.Default())
	if err != nil {
		t.Fatal(err)
	}

	tmp := t.TempDir()
	path := filepath.Join(tmp, "close.txt")
	os.WriteFile(path, []byte("x"), 0644)

	fw.Watch(path, WatchHandler{
		TreeKey:  "close/test",
		Debounce: time.Second,
		ReadFunc: func(p string) (json.RawMessage, error) { return json.RawMessage(`"x"`), nil },
	})

	fw.handleEvent(path)

	fw.mu.Lock()
	timerCount := len(fw.debounce)
	fw.mu.Unlock()
	if timerCount != 1 {
		t.Errorf("expected 1 debounce timer, got %d", timerCount)
	}

	fw.Close()
}

func TestHandleRemoveMergeHandler(t *testing.T) {
	fw, tr := newTestFSWatcher(t)

	tmp := t.TempDir()
	path := filepath.Join(tmp, "forwarding")
	os.WriteFile(path, []byte("1"), 0644)

	fw.Watch(path, WatchHandler{
		TreeKey: "routing",
		ReadFunc: func(_ string) (json.RawMessage, error) {
			return json.RawMessage(`{"interfaces":{"interface":["eth0"]}}`), nil
		},
		UseMerge: true,
	})

	fw.InitialRead()

	got := tr.Get("routing")
	if got == nil {
		t.Fatal("tree not populated after InitialRead")
	}

	os.Remove(path)
	fw.handleRemove(path)

	got = tr.Get("routing")
	if got == nil {
		t.Fatal("tree entry should still exist after merge-remove")
	}
	if string(got) != `{"interfaces":{"interface":["eth0"]}}` {
		t.Errorf("got %s, want updated merge data", got)
	}

	fw.mu.Lock()
	_, handlerExists := fw.handlers[path]
	fw.mu.Unlock()
	if handlerExists {
		t.Error("handler should be cleaned up after permanent removal")
	}
}

func TestHandleRemovePlainHandler(t *testing.T) {
	fw, tr := newTestFSWatcher(t)

	tmp := t.TempDir()
	path := filepath.Join(tmp, "value.txt")
	os.WriteFile(path, []byte("data"), 0644)

	fw.Watch(path, WatchHandler{
		TreeKey: "plain/key",
		ReadFunc: func(p string) (json.RawMessage, error) {
			return json.RawMessage(`"hello"`), nil
		},
	})

	fw.InitialRead()

	if got := tr.Get("plain/key"); string(got) != `"hello"` {
		t.Fatalf("initial = %s, want %q", got, `"hello"`)
	}

	os.Remove(path)
	fw.handleRemove(path)

	if got := tr.Get("plain/key"); got != nil {
		t.Errorf("tree entry should be deleted after remove, got %s", got)
	}

	fw.mu.Lock()
	_, handlerExists := fw.handlers[path]
	fw.mu.Unlock()
	if handlerExists {
		t.Error("handler should be cleaned up after permanent removal")
	}
}

func TestHandleRemoveUnknownPath(t *testing.T) {
	fw, _ := newTestFSWatcher(t)
	fw.handleRemove("/nonexistent/path")
}

func TestHandleRemoveRewatchSuccess(t *testing.T) {
	fw, tr := newTestFSWatcher(t)

	tmp := t.TempDir()
	path := filepath.Join(tmp, "ephemeral.txt")
	os.WriteFile(path, []byte("1"), 0644)

	calls := 0
	fw.Watch(path, WatchHandler{
		TreeKey: "ephem",
		ReadFunc: func(_ string) (json.RawMessage, error) {
			calls++
			return json.RawMessage(fmt.Sprintf(`"v%d"`, calls)), nil
		},
		UseMerge: true,
	})

	fw.InitialRead()

	fw.handleRemove(path)

	fw.mu.Lock()
	_, handlerExists := fw.handlers[path]
	fw.mu.Unlock()
	if !handlerExists {
		t.Error("handler should still exist when file still exists (rewatch succeeds)")
	}

	got := tr.Get("ephem")
	if string(got) != `"v2"` {
		t.Errorf("got %s, want %q (handler should have been called again)", got, `"v2"`)
	}
}

func TestWatchSymlink(t *testing.T) {
	fw, _ := newTestFSWatcher(t)

	tmp := t.TempDir()
	targetA := filepath.Join(tmp, "target-a")
	targetB := filepath.Join(tmp, "target-b")
	link := filepath.Join(tmp, "link")
	os.WriteFile(targetA, []byte("a"), 0644)
	os.WriteFile(targetB, []byte("b"), 0644)
	os.Symlink(targetA, link)

	handler := WatchHandler{
		TreeKey:  "sym/test",
		ReadFunc: func(p string) (json.RawMessage, error) { return json.RawMessage(`"sym"`), nil },
	}

	if err := fw.WatchSymlink(link, handler); err != nil {
		t.Fatalf("WatchSymlink: %v", err)
	}

	fw.mu.Lock()
	_, ok := fw.handlers[link]
	fw.mu.Unlock()
	if !ok {
		t.Error("handler not registered under symlink path")
	}
}

func TestWatchSymlinkReplace(t *testing.T) {
	fw, tr := newTestFSWatcher(t)

	tmp := t.TempDir()
	targetA := filepath.Join(tmp, "zone-a")
	targetB := filepath.Join(tmp, "zone-b")
	link := filepath.Join(tmp, "current")
	os.WriteFile(targetA, []byte("a"), 0644)
	os.WriteFile(targetB, []byte("b"), 0644)
	os.Symlink(targetA, link)

	calls := 0
	fw.WatchSymlink(link, WatchHandler{
		TreeKey: "sym/replace",
		ReadFunc: func(p string) (json.RawMessage, error) {
			calls++
			target, _ := os.Readlink(p)
			return json.RawMessage(fmt.Sprintf(`"target-%d-%s"`, calls, filepath.Base(target))), nil
		},
	})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go fw.Run(ctx)
	time.Sleep(50 * time.Millisecond)

	os.Remove(link)
	os.Symlink(targetB, link)

	deadline := time.After(2 * time.Second)
	for {
		got := tr.Get("sym/replace")
		if got != nil && strings.Contains(string(got), "zone-b") {
			break
		}
		select {
		case <-deadline:
			t.Fatalf("timed out waiting for symlink replace event; tree = %s", tr.Get("sym/replace"))
		default:
			time.Sleep(10 * time.Millisecond)
		}
	}

	cancel()
}

func TestWatchDir(t *testing.T) {
	fw, tr := newTestFSWatcher(t)

	tmp := t.TempDir()
	os.WriteFile(filepath.Join(tmp, "a.keys"), []byte("key-a"), 0644)

	fw.WatchDir(tmp, WatchHandler{
		TreeKey: "dir/test",
		ReadFunc: func(dir string) (json.RawMessage, error) {
			entries, _ := os.ReadDir(dir)
			names := make([]string, 0, len(entries))
			for _, e := range entries {
				names = append(names, e.Name())
			}
			return json.Marshal(map[string]interface{}{"files": names})
		},
		Debounce: 50 * time.Millisecond,
		UseMerge: true,
	})

	fw.InitialRead()
	got := tr.Get("dir/test")
	if got == nil {
		t.Fatal("tree not populated after InitialRead for dir handler")
	}
	if !strings.Contains(string(got), "a.keys") {
		t.Fatalf("initial read missing a.keys: %s", got)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go fw.Run(ctx)
	time.Sleep(50 * time.Millisecond)

	os.WriteFile(filepath.Join(tmp, "b.keys"), []byte("key-b"), 0644)

	deadline := time.After(2 * time.Second)
	for {
		got = tr.Get("dir/test")
		if got != nil && strings.Contains(string(got), "b.keys") {
			break
		}
		select {
		case <-deadline:
			t.Fatalf("timed out waiting for dir event; tree = %s", tr.Get("dir/test"))
		default:
			time.Sleep(10 * time.Millisecond)
		}
	}

	cancel()
}

func TestWatchDirRemoveFile(t *testing.T) {
	fw, tr := newTestFSWatcher(t)

	tmp := t.TempDir()
	os.WriteFile(filepath.Join(tmp, "x.keys"), []byte("data"), 0644)
	os.WriteFile(filepath.Join(tmp, "y.keys"), []byte("data"), 0644)

	fw.WatchDir(tmp, WatchHandler{
		TreeKey: "dir/rm",
		ReadFunc: func(dir string) (json.RawMessage, error) {
			entries, _ := os.ReadDir(dir)
			names := make([]string, 0, len(entries))
			for _, e := range entries {
				names = append(names, e.Name())
			}
			return json.Marshal(map[string]interface{}{"files": names})
		},
		UseMerge: true,
	})

	fw.InitialRead()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go fw.Run(ctx)
	time.Sleep(50 * time.Millisecond)

	os.Remove(filepath.Join(tmp, "x.keys"))

	deadline := time.After(2 * time.Second)
	for {
		got := tr.Get("dir/rm")
		if got != nil && !strings.Contains(string(got), "x.keys") && strings.Contains(string(got), "y.keys") {
			break
		}
		select {
		case <-deadline:
			t.Fatalf("timed out waiting for dir remove event; tree = %s", tr.Get("dir/rm"))
		default:
			time.Sleep(10 * time.Millisecond)
		}
	}

	cancel()
}
