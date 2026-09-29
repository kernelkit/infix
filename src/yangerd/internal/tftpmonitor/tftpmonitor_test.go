package tftpmonitor

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/kernelkit/infix/src/yangerd/internal/tree"
)

func newTestMonitor(t *testing.T) (*TFTPMonitor, *tree.Tree, string) {
	t.Helper()
	tr := tree.New()
	m := New(tr, nil)
	m.conf = filepath.Join(t.TempDir(), "tftp.conf")
	return m, tr, m.conf
}

func enable(t *testing.T, conf, root string) {
	t.Helper()
	snippet := "enable-tftp\ntftp-root=" + root + "\ntftp-no-fail\n"
	if err := os.WriteFile(conf, []byte(snippet), 0644); err != nil {
		t.Fatal(err)
	}
}

func disable(t *testing.T, conf string) {
	t.Helper()
	if err := os.Remove(conf); err != nil {
		t.Fatal(err)
	}
}

func put(t *testing.T, path, content string, mode os.FileMode) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), mode); err != nil {
		t.Fatal(err)
	}
	// umask must not decide the outcome
	if err := os.Chmod(path, mode); err != nil {
		t.Fatal(err)
	}
}

func names(t *testing.T, tr *tree.Tree) []string {
	t.Helper()
	raw := tr.Get(treeKey)
	if raw == nil {
		return nil
	}
	var data struct {
		Files struct {
			File []fileEntry `json:"file"`
		} `json:"files"`
	}
	if err := json.Unmarshal(raw, &data); err != nil {
		t.Fatalf("bad tree data %s: %v", raw, err)
	}
	out := []string{}
	for _, f := range data.Files.File {
		out = append(out, f.Name)
	}
	return out
}

func equal(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func waitFor(t *testing.T, tr *tree.Tree, want []string) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if got := names(t, tr); equal(got, want) {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("timeout waiting for %v, have %v", want, names(t, tr))
}

func waitForAbsent(t *testing.T, tr *tree.Tree) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if tr.Get(treeKey) == nil {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("timeout waiting for key to go away, have %s", tr.Get(treeKey))
}

// Without the snippet the server is disabled and the key must not exist.
func TestDisabledDeletesKey(t *testing.T) {
	m, tr, _ := newTestMonitor(t)
	tr.Set(treeKey, json.RawMessage(`{"files":{"file":[{"name":"stale"}]}}`))

	m.reconcile()

	if got := tr.Get(treeKey); got != nil {
		t.Fatalf("expected key removed, got %s", got)
	}
}

// Only world-readable regular files are listed, recursively and through
// symlinks, sorted by name.
func TestListsWorldReadableFiles(t *testing.T) {
	m, tr, conf := newTestMonitor(t)
	root := filepath.Join(t.TempDir(), "tftpboot")
	put(t, filepath.Join(root, "zimage"), "kernel", 0644)
	put(t, filepath.Join(root, "private.bin"), "secret", 0600)
	put(t, filepath.Join(root, "sub", "dtb"), "tree", 0444)
	outside := filepath.Join(t.TempDir(), "outside.bin")
	put(t, outside, "linked", 0644)
	if err := os.Symlink(outside, filepath.Join(root, "link.bin")); err != nil {
		t.Fatal(err)
	}
	enable(t, conf, root)

	m.reconcile()

	want := []string{"link.bin", "sub/dtb", "zimage"}
	if got := names(t, tr); !equal(got, want) {
		t.Fatalf("want %v, got %v", want, got)
	}

	var data struct {
		Files struct {
			File []fileEntry `json:"file"`
		} `json:"files"`
	}
	if err := json.Unmarshal(tr.Get(treeKey), &data); err != nil {
		t.Fatal(err)
	}
	if data.Files.File[0].Size != "6" {
		t.Fatalf("size must be a decimal string, got %q", data.Files.File[0].Size)
	}
	if _, err := time.Parse(time.RFC3339, data.Files.File[0].Modified); err != nil {
		t.Fatalf("modified not RFC 3339: %v", err)
	}
}

// A symlink loop below the root must not hang the scan.
func TestSymlinkLoop(t *testing.T) {
	m, tr, conf := newTestMonitor(t)
	root := filepath.Join(t.TempDir(), "tftpboot")
	put(t, filepath.Join(root, "a", "file"), "x", 0644)
	if err := os.Symlink(root, filepath.Join(root, "a", "loop")); err != nil {
		t.Fatal(err)
	}
	enable(t, conf, root)

	m.reconcile()

	if got := names(t, tr); !equal(got, []string{"a/file"}) {
		t.Fatalf("got %v", got)
	}
}

// The running monitor follows uploads, mode changes, a moved root, and
// the server being disabled.
func TestRunFollowsChanges(t *testing.T) {
	m, tr, conf := newTestMonitor(t)
	base := t.TempDir()
	rootA := filepath.Join(base, "a")
	rootB := filepath.Join(base, "b")
	put(t, filepath.Join(rootA, "one"), "1", 0644)
	put(t, filepath.Join(rootB, "two"), "2", 0644)
	enable(t, conf, rootA)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan struct{})
	go func() {
		defer close(done)
		_ = m.Run(ctx)
	}()

	waitFor(t, tr, []string{"one"})

	put(t, filepath.Join(rootA, "sub", "three"), "3", 0644)
	waitFor(t, tr, []string{"one", "sub/three"})

	// A new directory must be watched too, not just the ones at start
	put(t, filepath.Join(rootA, "sub", "four"), "4", 0644)
	waitFor(t, tr, []string{"one", "sub/four", "sub/three"})

	if err := os.Chmod(filepath.Join(rootA, "one"), 0600); err != nil {
		t.Fatal(err)
	}
	waitFor(t, tr, []string{"sub/four", "sub/three"})

	enable(t, conf, rootB)
	waitFor(t, tr, []string{"two"})

	// The old root is no longer watched, changes there must not show
	put(t, filepath.Join(rootA, "five"), "5", 0644)
	put(t, filepath.Join(rootB, "six"), "6", 0644)
	waitFor(t, tr, []string{"six", "two"})

	disable(t, conf)
	waitForAbsent(t, tr)

	put(t, filepath.Join(rootB, "seven"), "7", 0644)
	time.Sleep(2 * debounceDelay)
	if got := tr.Get(treeKey); got != nil {
		t.Fatalf("disabled server must stay absent, got %s", got)
	}

	cancel()
	<-done
}

// An enabled server whose root does not exist yet lists nothing, and
// picks the root up once it is created.
func TestRootCreatedLater(t *testing.T) {
	m, tr, conf := newTestMonitor(t)
	root := filepath.Join(t.TempDir(), "tftpboot")
	enable(t, conf, root)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan struct{})
	go func() {
		defer close(done)
		_ = m.Run(ctx)
	}()

	waitFor(t, tr, []string{})

	put(t, filepath.Join(root, "late"), "x", 0644)
	waitFor(t, tr, []string{"late"})

	cancel()
	<-done
}

// A directory removed and recreated within one debounce window must
// still be watched afterwards.
func TestRecreatedDirStaysWatched(t *testing.T) {
	m, tr, conf := newTestMonitor(t)
	root := filepath.Join(t.TempDir(), "tftpboot")
	put(t, filepath.Join(root, "fw", "old"), "x", 0644)
	enable(t, conf, root)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan struct{})
	go func() {
		defer close(done)
		_ = m.Run(ctx)
	}()

	waitFor(t, tr, []string{"fw/old"})

	if err := os.RemoveAll(filepath.Join(root, "fw")); err != nil {
		t.Fatal(err)
	}
	put(t, filepath.Join(root, "fw", "new"), "y", 0644)
	waitFor(t, tr, []string{"fw/new"})

	put(t, filepath.Join(root, "fw", "later"), "z", 0644)
	waitFor(t, tr, []string{"fw/later", "fw/new"})

	cancel()
	<-done
}

// With neither the root nor its parent present, the nearest existing
// ancestor is watched so the whole path can appear later.
func TestRootWithMissingParent(t *testing.T) {
	m, tr, conf := newTestMonitor(t)
	root := filepath.Join(t.TempDir(), "media", "usb")
	enable(t, conf, root)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan struct{})
	go func() {
		defer close(done)
		_ = m.Run(ctx)
	}()

	waitFor(t, tr, []string{})

	put(t, filepath.Join(root, "late"), "x", 0644)
	waitFor(t, tr, []string{"late"})

	cancel()
	<-done
}
