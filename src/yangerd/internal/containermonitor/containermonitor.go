// Package containermonitor keeps the infix-containers subtree in the tree
// in sync with podman.  A persistent `podman events` subprocess is used
// purely as a change trigger; on every event the full container table is
// re-read with `podman ps` (via collector.CollectContainers) and the
// subtree replaced, so removed containers disappear and containers present
// before yangerd started are picked up.
//
// This replaces an earlier inotify watch on /run/libpod/events, which was
// reactive-only and silently went stale whenever an event was missed
// (debounce coalescing, inotify overflow, a removal racing the re-read, or
// yangerd starting after the container).  `podman events` reads whichever
// events backend podman is configured for (file or journald), so it does
// not depend on a specific on-disk layout.
package containermonitor

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"os/exec"
	"time"

	"github.com/kernelkit/infix/src/yangerd/internal/backoff"
	"github.com/kernelkit/infix/src/yangerd/internal/collector"
	"github.com/kernelkit/infix/src/yangerd/internal/tree"
)

const (
	treeKey = "infix-containers:containers"

	// debounceDelay coalesces bursts of events into one re-read.  It is
	// deliberately generous: container lifecycle events fire while confd
	// is still running its own `podman` start/stop/rm operations, so
	// re-reading too eagerly makes yangerd's `podman ps/inspect/stats`
	// contend with confd for the libpod lock on a CPU-starved guest.
	// Waiting for the churn to settle keeps yangerd off confd's back
	// during config apply/reset; a couple of seconds of staleness in
	// operational data is harmless.
	debounceDelay = 2 * time.Second
)

// ContainerMonitor subscribes to container lifecycle events via a
// persistent `podman events` subprocess and re-reads the full container
// table on every event.
type ContainerMonitor struct {
	tree    *tree.Tree
	log     *slog.Logger
	refresh chan struct{}

	// collect returns the current container subtree, or nil when there are
	// no containers; overridable in tests.
	collect func() json.RawMessage
}

// New creates a ContainerMonitor.
func New(t *tree.Tree, cmd collector.CommandRunner, fs collector.FileReader, log *slog.Logger) *ContainerMonitor {
	if log == nil {
		log = slog.Default()
	}
	return &ContainerMonitor{
		tree:    t,
		log:     log,
		refresh: make(chan struct{}, 1),
		collect: func() json.RawMessage { return collector.CollectContainers(cmd, fs) },
	}
}

// Run starts the container monitor.  It blocks until ctx is cancelled,
// restarting the events subprocess with backoff if it exits.
func (m *ContainerMonitor) Run(ctx context.Context) error {
	go m.refreshLoop(ctx)

	bo := backoff.Default()
	delay := bo.Initial

	for {
		err := m.runOnce(ctx)
		if ctx.Err() != nil {
			return ctx.Err()
		}

		m.log.Warn("container monitor: subprocess exited, restarting",
			"err", err, "delay", delay)
		if err := backoff.Sleep(ctx, delay); err != nil {
			return err
		}
		delay = bo.Next(delay)
	}
}

func (m *ContainerMonitor) runOnce(ctx context.Context) error {
	cmd := exec.CommandContext(ctx, "podman", "events", "--filter", "type=container", "--format", "json")
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return fmt.Errorf("stdout pipe: %w", err)
	}
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("start podman events: %w", err)
	}
	defer cmd.Wait()

	// Pick up containers that existed before we attached.
	m.triggerRefresh()

	return m.readEvents(stdout)
}

// readEvents consumes the newline-delimited JSON event stream.  Each event
// is only a trigger; the payload is never used to build state.
func (m *ContainerMonitor) readEvents(r io.Reader) error {
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 0, 64*1024), 1*1024*1024)

	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}
		if status := eventStatus(line); status != "" {
			m.log.Debug("container monitor: event", "status", status)
		}
		m.triggerRefresh()
	}
	if err := scanner.Err(); err != nil {
		return fmt.Errorf("read podman events: %w", err)
	}
	return fmt.Errorf("podman events process exited")
}

// eventStatus extracts the event status for logging; best-effort only.
func eventStatus(line []byte) string {
	var ev struct {
		Status string `json:"Status"`
	}
	if json.Unmarshal(line, &ev) != nil {
		return ""
	}
	return ev.Status
}

// triggerRefresh requests a table re-read; the buffered channel collapses
// pending requests into one.
func (m *ContainerMonitor) triggerRefresh() {
	select {
	case m.refresh <- struct{}{}:
	default:
	}
}

func (m *ContainerMonitor) refreshLoop(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case <-m.refresh:
		}

		// Let a burst of events settle before reading.
		select {
		case <-ctx.Done():
			return
		case <-time.After(debounceDelay):
		}
		select {
		case <-m.refresh:
		default:
		}

		m.updateTree()
	}
}

// updateTree re-reads the full container table and replaces the subtree.
// With no containers the key is deleted rather than left as an empty node,
// so an idle-but-enabled container feature reads as absent.
func (m *ContainerMonitor) updateTree() {
	data := m.collect()
	if len(data) == 0 {
		m.tree.Delete(treeKey)
		m.log.Debug("container monitor: no containers, key removed")
		return
	}
	m.tree.Set(treeKey, data)
	m.log.Debug("container monitor: tree updated")
}
