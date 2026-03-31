// Package fswatcher provides inotify-based reactive monitoring of
// filesystem paths.  It replaces polling for procfs files that support
// inotify (e.g. IP forwarding flags).  Each watched path has a
// handler that reads the file and updates the tree, with per-path
// debouncing to coalesce burst writes.
package fswatcher

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"path/filepath"
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"
	"github.com/kernelkit/infix/src/yangerd/internal/tree"
)

// WatchHandler defines the callback for a watched path.
type WatchHandler struct {
	TreeKey  string
	ReadFunc func(path string) (json.RawMessage, error)
	Debounce time.Duration
	// UseMerge causes the watcher to call tree.Merge instead of
	// tree.Set, performing a shallow first-level JSON merge into
	// the existing blob at TreeKey.
	UseMerge bool
}

// FSWatcher monitors filesystem paths via inotify and updates the
// tree when files change.
type FSWatcher struct {
	watcher     *fsnotify.Watcher
	tree        *tree.Tree
	handlers    map[string]WatchHandler
	dirHandlers map[string]WatchHandler // directory path → handler
	debounce    map[string]*time.Timer
	mu          sync.Mutex
	log         *slog.Logger
}

// New creates an FSWatcher backed by an inotify instance.
func New(t *tree.Tree, log *slog.Logger) (*FSWatcher, error) {
	w, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, fmt.Errorf("fsnotify: %w", err)
	}
	return &FSWatcher{
		watcher:     w,
		tree:        t,
		handlers:    make(map[string]WatchHandler),
		dirHandlers: make(map[string]WatchHandler),
		debounce:    make(map[string]*time.Timer),
		log:         log,
	}, nil
}

// Watch registers a handler for a specific filesystem path and adds
// the inotify watch.
func (fw *FSWatcher) Watch(path string, handler WatchHandler) error {
	fw.mu.Lock()
	fw.handlers[path] = handler
	fw.mu.Unlock()
	return fw.watcher.Add(path)
}

// WatchGlob expands a glob pattern and registers a handler for each
// matching path.  Returns the number of paths matched.
func (fw *FSWatcher) WatchGlob(pattern string, handler WatchHandler) (int, error) {
	matches, err := filepath.Glob(pattern)
	if err != nil {
		return 0, fmt.Errorf("glob %s: %w", pattern, err)
	}
	for _, path := range matches {
		if err := fw.Watch(path, handler); err != nil {
			fw.log.Warn("fswatcher: watch failed, skipping", "path", path, "err", err)
		}
	}
	return len(matches), nil
}

// WatchSymlink registers a handler for a symlink by watching its parent
// directory.  fsnotify follows symlinks to the target inode, so replacing
// a symlink (ln -sf) would not trigger events on a direct watch.  Watching
// the parent directory catches Create and Rename events for the symlink
// entry itself.
func (fw *FSWatcher) WatchSymlink(path string, handler WatchHandler) error {
	dir := filepath.Dir(path)
	fw.mu.Lock()
	fw.handlers[path] = handler
	fw.mu.Unlock()
	return fw.watcher.Add(dir)
}

// WatchDir registers a handler for an entire directory.  Any file
// create/write/remove event inside the directory triggers the handler
// with the directory path.  The handler's ReadFunc receives the directory
// path (not the individual file), so it can rescan all contents.
func (fw *FSWatcher) WatchDir(dir string, handler WatchHandler) error {
	fw.mu.Lock()
	fw.dirHandlers[dir] = handler
	fw.mu.Unlock()
	return fw.watcher.Add(dir)
}

// InitialRead reads the current value of every watched file and
// populates the tree.  Called once after all Watch() calls and glob
// expansion, before Run().
func (fw *FSWatcher) InitialRead() {
	fw.mu.Lock()
	defer fw.mu.Unlock()
	for path, handler := range fw.handlers {
		data, err := handler.ReadFunc(path)
		if err != nil {
			fw.log.Warn("fswatcher: initial read failed", "path", path, "err", err)
			continue
		}
		if handler.UseMerge {
			fw.tree.Merge(handler.TreeKey, data)
		} else {
			fw.tree.Set(handler.TreeKey, data)
		}
		fw.log.Debug("fswatcher: initial read", "path", path, "key", handler.TreeKey)
	}
	for dir, handler := range fw.dirHandlers {
		data, err := handler.ReadFunc(dir)
		if err != nil {
			fw.log.Warn("fswatcher: initial read failed", "path", dir, "err", err)
			continue
		}
		if handler.UseMerge {
			fw.tree.Merge(handler.TreeKey, data)
		} else {
			fw.tree.Set(handler.TreeKey, data)
		}
		fw.log.Debug("fswatcher: initial read", "path", dir, "key", handler.TreeKey)
	}
}

// Run processes inotify events until ctx is cancelled.
func (fw *FSWatcher) Run(ctx context.Context) error {
	defer fw.watcher.Close()
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case event, ok := <-fw.watcher.Events:
			if !ok {
				return fmt.Errorf("watcher closed")
			}
			if event.Has(fsnotify.Write) || event.Has(fsnotify.Create) {
				fw.handleEvent(event.Name)
			}
			if event.Has(fsnotify.Remove) {
				fw.handleRemove(event.Name)
			}
		case err, ok := <-fw.watcher.Errors:
			if !ok {
				return fmt.Errorf("watcher error channel closed")
			}
			fw.log.Warn("fsnotify error", "err", err)
		}
	}
}

// Close shuts down the inotify watcher and cancels pending timers.
func (fw *FSWatcher) Close() {
	fw.mu.Lock()
	defer fw.mu.Unlock()
	for _, timer := range fw.debounce {
		timer.Stop()
	}
	fw.watcher.Close()
}

func (fw *FSWatcher) handleEvent(path string) {
	fw.mu.Lock()
	handler, ok := fw.handlers[path]
	handlerPath := path
	if !ok {
		dir := filepath.Dir(path)
		handler, ok = fw.dirHandlers[dir]
		handlerPath = dir
		if !ok {
			fw.mu.Unlock()
			return
		}
	}

	if handler.Debounce > 0 {
		if timer, exists := fw.debounce[handlerPath]; exists {
			timer.Reset(handler.Debounce)
			fw.mu.Unlock()
			return
		}
		fw.debounce[handlerPath] = time.AfterFunc(handler.Debounce, func() {
			fw.fireHandler(handlerPath, handler)
		})
		fw.mu.Unlock()
		return
	}
	fw.mu.Unlock()
	fw.fireHandler(handlerPath, handler)
}

func (fw *FSWatcher) handleRemove(path string) {
	fw.mu.Lock()
	handler, ok := fw.handlers[path]
	if !ok {
		dir := filepath.Dir(path)
		handler, ok = fw.dirHandlers[dir]
		if ok {
			fw.mu.Unlock()
			fw.fireHandler(dir, handler)
			return
		}
		fw.mu.Unlock()
		return
	}
	fw.mu.Unlock()

	if handler.UseMerge {
		fw.fireHandler(path, handler)
	} else {
		fw.tree.Delete(handler.TreeKey)
		fw.log.Debug("fswatcher: removed", "path", path, "key", handler.TreeKey)
	}

	if err := fw.watcher.Add(path); err != nil {
		fw.mu.Lock()
		delete(fw.handlers, path)
		if timer, exists := fw.debounce[path]; exists {
			timer.Stop()
			delete(fw.debounce, path)
		}
		fw.mu.Unlock()
		fw.log.Debug("fswatcher: file gone, handler removed", "path", path)
	}
}

func (fw *FSWatcher) fireHandler(path string, handler WatchHandler) {
	data, err := handler.ReadFunc(path)
	if err != nil {
		fw.log.Warn("fswatcher: read failed", "path", path, "err", err)
		return
	}
	if handler.UseMerge {
		fw.tree.Merge(handler.TreeKey, data)
	} else {
		fw.tree.Set(handler.TreeKey, data)
	}
	fw.log.Debug("fswatcher: updated", "path", path, "key", handler.TreeKey)
}
