package tftpmonitor

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// A regular file never signals POLLPRI, so nothing fires, and the
// watcher returns promptly when cancelled instead of blocking in poll.
func TestWatchMountsCancel(t *testing.T) {
	path := filepath.Join(t.TempDir(), "mountinfo")
	if err := os.WriteFile(path, []byte("22 1 0:21 / / rw - ext4 /dev/root rw\n"), 0644); err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	fired := make(chan struct{}, 1)
	done := make(chan error, 1)
	go func() {
		done <- watchMounts(ctx, path, func() { fired <- struct{}{} })
	}()

	select {
	case <-fired:
		t.Fatal("no mount change, nothing may fire")
	case <-time.After(100 * time.Millisecond):
	}

	cancel()
	select {
	case err := <-done:
		if err != context.Canceled {
			t.Fatalf("err = %v, want context.Canceled", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("watchMounts did not return after cancel")
	}
}

func TestWatchMountsMissingFile(t *testing.T) {
	err := watchMounts(context.Background(), filepath.Join(t.TempDir(), "none"), func() {})
	if err == nil {
		t.Fatal("expected an error for a missing mount table")
	}
}
