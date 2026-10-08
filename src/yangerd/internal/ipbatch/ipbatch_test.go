package ipbatch

import (
	"context"
	"errors"
	"log/slog"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// fakeBatch behaves like `ip -force -batch -`: one JSON line per good
// command, "Command failed -:N" on stderr for a bad one (after a stray
// "[]" for "junk"), a stall for "hang", and exit for "die".
const fakeBatch = `#!/bin/sh
n=0
while read -r cmd; do
	n=$((n+1))
	case "$cmd" in
	sentinel) echo "Object \"sentinel\" is unknown" >&2; echo "Command failed -:$n" >&2 ;;
	fail*) echo "Cannot find device" >&2; echo "Command failed -:$n" >&2 ;;
	junk*) echo "[]"; echo "Cannot find device" >&2; echo "Command failed -:$n" >&2 ;;
	hang*) sleep 30 ;;
	die*)  exit 1 ;;
	*)     echo "[\"$cmd\"]" ;;
	esac
done
`

func startFake(t *testing.T) *Batch {
	t.Helper()
	script := filepath.Join(t.TempDir(), "fake")
	if err := os.WriteFile(script, []byte(fakeBatch), 0755); err != nil {
		t.Fatal(err)
	}
	b, err := Start(context.Background(), slog.Default(), []string{script}, "canary")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(b.Close)
	return b
}

func TestQueryAnswers(t *testing.T) {
	b := startFake(t)
	got, err := b.Query("link show dev eth0")
	if err != nil || string(got) != `["link show dev eth0"]` {
		t.Fatalf("Query = %s, %v", got, err)
	}
}

// A rejected command is reported at once, and the stream stays in step.
func TestQueryFailedCommandIsImmediate(t *testing.T) {
	b := startFake(t)
	start := time.Now()
	if _, err := b.Query("fail dev gone0"); !errors.Is(err, ErrCommandFailed) {
		t.Fatalf("err = %v, want ErrCommandFailed", err)
	}
	if time.Since(start) > time.Second {
		t.Fatal("failed command waited for the timeout")
	}
	got, err := b.Query("next")
	if err != nil || string(got) != `["next"]` {
		t.Fatalf("follow-up Query = %s, %v", got, err)
	}
}

// A dead subprocess fails the next query fast and is restarted.
func TestQueryDeadThenRestart(t *testing.T) {
	b := startFake(t)
	start := time.Now()
	if _, err := b.Query("die"); !errors.Is(err, ErrBatchDead) {
		t.Fatalf("err = %v, want ErrBatchDead", err)
	}
	if time.Since(start) > time.Second {
		t.Fatal("dead subprocess waited for the timeout")
	}

	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if got, err := b.Query("again"); err == nil {
			if string(got) != `["again"]` {
				t.Fatalf("after restart got %s", got)
			}
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatal("subprocess was not restarted")
}

// A command that gets no answer at all times out and kills the process.
func TestQueryTimeout(t *testing.T) {
	if testing.Short() {
		t.Skip("waits for the query timeout")
	}
	b := startFake(t)
	if _, err := b.Query("hang"); err == nil || errors.Is(err, ErrCommandFailed) {
		t.Fatalf("err = %v, want a timeout", err)
	}
	if b.alive.Load() {
		t.Fatal("process still marked alive after timeout")
	}
}

// Refresh swaps in a new subprocess, which starts its line count over,
// and the old one's exit must not mark the new one dead.
func TestRefreshReplacesProcess(t *testing.T) {
	b := startFake(t)
	if _, err := b.Query("link show dev eth0"); err != nil {
		t.Fatal(err)
	}
	b.mu.Lock()
	oldPid := b.cmd.Process.Pid
	b.mu.Unlock()

	if err := b.Refresh(); err != nil {
		t.Fatalf("Refresh: %v", err)
	}

	b.mu.Lock()
	newPid := b.cmd.Process.Pid
	b.mu.Unlock()
	if newPid == oldPid {
		t.Fatal("Refresh kept the old process")
	}

	time.Sleep(100 * time.Millisecond)
	if _, err := b.Query("fail dev gone0"); !errors.Is(err, ErrCommandFailed) {
		t.Fatalf("line numbering out of step after Refresh: %v", err)
	}
	if got, err := b.Query("link show dev eth1"); err != nil || string(got) != `["link show dev eth1"]` {
		t.Fatalf("Query after Refresh = %s, %v", got, err)
	}
}

// A failing command that also prints JSON, as some iproute2 versions do,
// is still a failure, and its stray line is not handed to the next query.
func TestQueryJunkBeforeFailure(t *testing.T) {
	b := startFake(t)
	if _, err := b.Query("junk dev gone0"); !errors.Is(err, ErrCommandFailed) {
		t.Fatalf("err = %v, want ErrCommandFailed", err)
	}
	got, err := b.Query("next")
	if err != nil || string(got) != `["next"]` {
		t.Fatalf("follow-up Query = %s, %v", got, err)
	}
}

// Refresh and the restart loop must not kill each other's process.
func TestRefreshDuringRestart(t *testing.T) {
	b := startFake(t)
	if _, err := b.Query("die"); !errors.Is(err, ErrBatchDead) {
		t.Fatalf("err = %v, want ErrBatchDead", err)
	}
	for i := 0; i < 20; i++ {
		b.Refresh()
		time.Sleep(10 * time.Millisecond)
	}
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if got, err := b.Query("again"); err == nil && string(got) == `["again"]` {
			time.Sleep(300 * time.Millisecond)
			if !b.alive.Load() {
				t.Fatal("process killed after it was restarted")
			}
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatal("no live subprocess after refreshes")
}
