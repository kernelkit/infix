// Package containermonitor keeps the infix-containers subtree in the tree
// in sync with ixcont.  ixcont writes /run/containers/<name>/state.json
// when it creates a container, when crun has started it (OCI poststart
// hook) and when it has stopped, so inotify on that directory is the
// event.  On every event the full container table is re-read from the
// state files, crun and the cgroup tree (collector.CollectContainers) and
// the subtree replaced, so removed containers disappear and containers
// present before yangerd started are picked up.
package containermonitor

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"time"

	"github.com/kernelkit/infix/src/yangerd/internal/backoff"
	"github.com/kernelkit/infix/src/yangerd/internal/collector"
	"github.com/kernelkit/infix/src/yangerd/internal/inotify"
	"github.com/kernelkit/infix/src/yangerd/internal/tree"
)

const (
	treeKey = "infix-containers:containers"

	// debounceDelay coalesces bursts of events into one re-read: setup
	// writes several files, and the poststart hook follows shortly.
	debounceDelay = 500 * time.Millisecond
)

// StateDir is where ixcont keeps one directory per container.
var StateDir = "/run/containers"

// ContainerMonitor follows the ixcont state directory and re-reads the
// full container table on every change in it.
type ContainerMonitor struct {
	tree    *tree.Tree
	log     *slog.Logger
	refresh chan struct{}

	// collect returns the current container subtree, or nil when there are
	// no containers; overridable in tests.
	collect func() json.RawMessage

	// onUpdate, when set, runs after every re-read of the container
	// table, whatever it found.
	onUpdate func()
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

// SetOnUpdate sets a callback run after every re-read of the container
// table, so other collectors can follow the container set without a
// watch of their own.  Must be called before Run.
func (m *ContainerMonitor) SetOnUpdate(fn func()) {
	m.onUpdate = fn
}

// Run starts the container monitor.  It blocks until ctx is cancelled,
// restarting the watch with backoff if it fails.
func (m *ContainerMonitor) Run(ctx context.Context) error {
	go m.refreshLoop(ctx)
	return backoff.Retry(ctx, m.log, "container monitor", m.runOnce)
}

func (m *ContainerMonitor) runOnce(ctx context.Context) error {
	if err := os.MkdirAll(StateDir, 0o755); err != nil {
		return fmt.Errorf("mkdir %s: %w", StateDir, err)
	}
	w, err := inotify.NewWatcher()
	if err != nil {
		return err
	}
	defer w.Close()

	if err := w.Add(StateDir); err != nil {
		return err
	}
	entries, _ := os.ReadDir(StateDir)
	for _, e := range entries {
		if e.IsDir() {
			_ = w.Add(filepath.Join(StateDir, e.Name()))
		}
	}

	// Pick up containers that existed before we attached.
	m.triggerRefresh()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case ev := <-w.Events:
			// A new container directory: follow its state file too.
			if filepath.Dir(ev.Name) == StateDir && ev.Has(inotify.Create) {
				if fi, err := os.Stat(ev.Name); err == nil && fi.IsDir() {
					_ = w.Add(ev.Name)
				}
			}
			m.log.Debug("container monitor: event", "path", ev.Name, "op", ev.Op.String())
			m.triggerRefresh()
		case err := <-w.Errors:
			return fmt.Errorf("inotify: %w", err)
		}
	}
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
	} else {
		m.tree.Set(treeKey, data)
		m.log.Debug("container monitor: tree updated")
	}
	if m.onUpdate != nil {
		m.onUpdate()
	}
}
