package inotify

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"
)

func next(t *testing.T, w *Watcher) Event {
	t.Helper()
	select {
	case ev := <-w.Events:
		return ev
	case err := <-w.Errors:
		t.Fatalf("unexpected error: %v", err)
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for event")
	}
	return Event{}
}

func collect(t *testing.T, w *Watcher, until func(Event) bool) []Event {
	t.Helper()
	var evs []Event
	for {
		ev := next(t, w)
		evs = append(evs, ev)
		if until(ev) {
			return evs
		}
	}
}

func TestDirectoryEventsCarryFullPath(t *testing.T) {
	dir := t.TempDir()
	w, err := NewWatcher()
	if err != nil {
		t.Fatal(err)
	}
	defer w.Close()
	if err := w.Add(dir); err != nil {
		t.Fatal(err)
	}

	file := filepath.Join(dir, "f")
	if err := os.WriteFile(file, []byte("x"), 0644); err != nil {
		t.Fatal(err)
	}
	evs := collect(t, w, func(e Event) bool { return e.Has(Write) })
	if evs[0].Name != file || !evs[0].Has(Create) {
		t.Errorf("first event = %+v, want Create on %s", evs[0], file)
	}

	if err := os.Remove(file); err != nil {
		t.Fatal(err)
	}
	if ev := next(t, w); ev.Name != file || !ev.Has(Remove) {
		t.Errorf("event = %+v, want Remove on %s", ev, file)
	}
}

func TestFileWatchRemoveSelf(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "f")
	if err := os.WriteFile(file, nil, 0644); err != nil {
		t.Fatal(err)
	}
	w, err := NewWatcher()
	if err != nil {
		t.Fatal(err)
	}
	defer w.Close()
	if err := w.Add(file); err != nil {
		t.Fatal(err)
	}
	if err := w.Add(file); err != nil {
		t.Errorf("second Add of a live watch: %v", err)
	}

	if err := os.Remove(file); err != nil {
		t.Fatal(err)
	}
	evs := collect(t, w, func(e Event) bool { return e.Has(Remove) })
	if evs[len(evs)-1].Name != file {
		t.Errorf("remove event on %s, want %s", evs[len(evs)-1].Name, file)
	}
	if err := w.Remove(file); err == nil {
		t.Error("Remove after the kernel dropped the watch should report it unknown")
	}
}

func TestRenameIntoWatchedDirIsCreate(t *testing.T) {
	dir := t.TempDir()
	other := t.TempDir()
	src := filepath.Join(other, "f")
	if err := os.WriteFile(src, nil, 0644); err != nil {
		t.Fatal(err)
	}
	w, err := NewWatcher()
	if err != nil {
		t.Fatal(err)
	}
	defer w.Close()
	if err := w.Add(dir); err != nil {
		t.Fatal(err)
	}

	dst := filepath.Join(dir, "f")
	if err := os.Rename(src, dst); err != nil {
		t.Fatal(err)
	}
	if ev := next(t, w); ev.Name != dst || !ev.Has(Create) {
		t.Errorf("event = %+v, want Create on %s", ev, dst)
	}
}

func TestAddMissingPathFails(t *testing.T) {
	w, err := NewWatcher()
	if err != nil {
		t.Fatal(err)
	}
	defer w.Close()
	if err := w.Add(filepath.Join(t.TempDir(), "missing")); err == nil {
		t.Error("Add of a missing path should fail")
	}
	if err := w.Remove("/never/watched"); err == nil {
		t.Error("Remove of an unwatched path should fail")
	}
}

func TestCloseEndsChannels(t *testing.T) {
	w, err := NewWatcher()
	if err != nil {
		t.Fatal(err)
	}
	if err := w.Add(t.TempDir()); err != nil {
		t.Fatal(err)
	}
	w.Close()
	w.Close()
	if _, ok := <-w.Events; ok {
		t.Error("Events still open after Close")
	}
	if _, ok := <-w.Errors; ok {
		t.Error("Errors still open after Close")
	}
}

// Delivering events must not leave goroutines behind: each one used to
// spawn a poller that lived until Close, pinning an OS thread.
func TestEventsLeaveNoGoroutines(t *testing.T) {
	dir := t.TempDir()
	w, err := NewWatcher()
	if err != nil {
		t.Fatal(err)
	}
	defer w.Close()
	if err := w.Add(dir); err != nil {
		t.Fatal(err)
	}

	before := runtime.NumGoroutine()
	for i := 0; i < 200; i++ {
		name := filepath.Join(dir, "f")
		if err := os.WriteFile(name, []byte("x"), 0644); err != nil {
			t.Fatal(err)
		}
		os.Remove(name)
		for drained := false; !drained; {
			select {
			case <-w.Events:
			case <-time.After(20 * time.Millisecond):
				drained = true
			}
		}
	}
	runtime.GC()
	time.Sleep(50 * time.Millisecond)
	if after := runtime.NumGoroutine(); after > before+2 {
		t.Fatalf("goroutines grew from %d to %d over 200 events", before, after)
	}
}
