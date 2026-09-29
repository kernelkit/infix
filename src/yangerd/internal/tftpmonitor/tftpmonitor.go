// Package tftpmonitor keeps the infix-services:tftp subtree in sync with
// what dnsmasq serves.  confd writes /etc/dnsmasq.d/tftp.conf when the
// TFTP server is enabled and removes it when disabled, so that file is
// the source of truth for whether there is anything to report and where
// the root is.  The root and every directory below it are then watched
// with inotify, and the file list is rebuilt whenever something changes:
// a file is uploaded, removed, renamed or has its mode changed, a
// directory appears or disappears, or the root itself moves.
//
// Only world-readable regular files are listed, following symlinks the
// way dnsmasq does when it opens them.
package tftpmonitor

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/fsnotify/fsnotify"
	"github.com/kernelkit/infix/src/yangerd/internal/tree"
)

const (
	treeKey  = "infix-services:tftp"
	confPath = "/etc/dnsmasq.d/tftp.conf"
	rootKey  = "tftp-root="

	// debounceDelay coalesces bursts of events into one rescan.  A file
	// upload is many writes, and confd rewrites the snippet in place
	// (create, several writes) rather than renaming a temp file over it.
	debounceDelay = 300 * time.Millisecond

	// maxDepth bounds the walk below the root, symlink loops are caught
	// separately but a deep tree is not something a TFTP server needs.
	maxDepth = 16
)

// TFTPMonitor watches the dnsmasq TFTP snippet and the TFTP root.
type TFTPMonitor struct {
	tree *tree.Tree
	log  *slog.Logger

	// conf is the dnsmasq snippet to read the root from; overridable in
	// tests.
	conf string

	watcher *fsnotify.Watcher
	root    string          // current TFTP root, "" when disabled
	watched map[string]bool // directories currently watched below root
}

// New creates a TFTPMonitor.
func New(t *tree.Tree, log *slog.Logger) *TFTPMonitor {
	if log == nil {
		log = slog.Default()
	}
	return &TFTPMonitor{
		tree:    t,
		log:     log,
		conf:    confPath,
		watched: make(map[string]bool),
	}
}

// Run watches the snippet and the root until ctx is cancelled.  All
// state is owned by this goroutine; events only schedule a rescan.
func (m *TFTPMonitor) Run(ctx context.Context) error {
	w, err := fsnotify.NewWatcher()
	if err != nil {
		return fmt.Errorf("fsnotify: %w", err)
	}
	defer w.Close()
	m.watcher = w

	// confd creates and removes the snippet, so watch its directory.
	confDir := filepath.Dir(m.conf)
	if err := w.Add(confDir); err != nil {
		return fmt.Errorf("watch %s: %w", confDir, err)
	}

	m.reconcile()

	timer := time.NewTimer(time.Hour)
	timer.Stop()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()

		case ev, ok := <-w.Events:
			if !ok {
				return fmt.Errorf("watcher closed")
			}
			if filepath.Dir(ev.Name) == confDir && ev.Name != m.conf {
				continue // another dnsmasq snippet
			}
			timer.Reset(debounceDelay)

		case <-timer.C:
			m.reconcile()

		case err, ok := <-w.Errors:
			if !ok {
				return fmt.Errorf("watcher error channel closed")
			}
			m.log.Warn("tftp monitor: fsnotify error", "err", err)
		}
	}
}

// reconcile re-reads the snippet, re-syncs the directory watches to the
// current root and rewrites the subtree.  Called on start and after every
// settled burst of events, and cheap enough to be a full rebuild.
func (m *TFTPMonitor) reconcile() {
	root := m.readRoot()
	if root != m.root {
		if root == "" {
			m.log.Info("tftp monitor: server disabled")
		} else {
			m.log.Info("tftp monitor: serving from", "root", root)
		}
		m.root = root
	}

	if root == "" {
		m.syncWatches(nil)
		m.tree.Delete(treeKey)
		return
	}

	files, dirs := scan(root, m.log)
	m.syncWatches(dirs)

	data, err := json.Marshal(map[string]any{
		"files": map[string]any{"file": files},
	})
	if err != nil {
		m.log.Warn("tftp monitor: marshal", "err", err)
		return
	}
	m.tree.Set(treeKey, data)
	m.log.Debug("tftp monitor: tree updated", "root", root, "files", len(files))
}

// readRoot returns the tftp-root from the snippet, or "" when the server
// is disabled, i.e. the snippet is gone or names no root.
func (m *TFTPMonitor) readRoot() string {
	data, err := os.ReadFile(m.conf)
	if err != nil {
		return ""
	}
	for _, line := range strings.Split(string(data), "\n") {
		if strings.HasPrefix(line, rootKey) {
			return strings.TrimSuffix(strings.TrimPrefix(line, rootKey), "/")
		}
	}
	return ""
}

// syncWatches makes the set of watched directories equal to dirs.  The
// snippet directory is not in the set and is never touched.
func (m *TFTPMonitor) syncWatches(dirs []string) {
	want := make(map[string]bool, len(dirs))
	for _, dir := range dirs {
		want[dir] = true
	}

	for dir := range m.watched {
		if want[dir] {
			continue
		}
		if m.watcher != nil {
			// A removed directory already dropped its watch, ignore.
			_ = m.watcher.Remove(dir)
		}
		delete(m.watched, dir)
	}

	// Add even what is already watched: the kernel drops the watch of a
	// deleted directory, so one removed and recreated between two scans
	// needs it again, and Add on a live watch is a no-op.
	for dir := range want {
		if m.watcher != nil {
			if err := m.watcher.Add(dir); err != nil {
				m.log.Warn("tftp monitor: watch failed", "dir", dir, "err", err)
				continue
			}
		}
		m.watched[dir] = true
	}
}

// fileEntry is one entry of the files/file list.  Size is a uint64 in
// the model, hence a string per RFC 7951.
type fileEntry struct {
	Name     string `json:"name"`
	Size     string `json:"size"`
	Modified string `json:"modified"`
}

// scan lists the world-readable regular files below root, sorted by
// name, and the directories that must be watched to notice a change.
//
// A missing root is a legitimate state: the server is enabled and dnsmasq
// runs with tftp-no-fail, so the list is empty and the nearest existing
// ancestor is watched to catch the root, or a directory on the way to
// it, being created.
func scan(root string, log *slog.Logger) ([]fileEntry, []string) {
	files := []fileEntry{}

	if fi, err := os.Stat(root); err != nil || !fi.IsDir() {
		for dir := filepath.Dir(root); ; dir = filepath.Dir(dir) {
			if fi, err := os.Stat(dir); err == nil && fi.IsDir() {
				return files, []string{dir}
			}
			if dir == filepath.Dir(dir) {
				return files, nil
			}
		}
	}

	var dirs []string
	seen := make(map[string]bool) // real paths, to break symlink loops

	var walk func(dir string, depth int)
	walk = func(dir string, depth int) {
		real, err := filepath.EvalSymlinks(dir)
		if err != nil || seen[real] {
			return
		}
		seen[real] = true
		dirs = append(dirs, dir)

		entries, err := os.ReadDir(dir)
		if err != nil {
			log.Debug("tftp monitor: read dir", "dir", dir, "err", err)
			return
		}
		for _, entry := range entries {
			path := filepath.Join(dir, entry.Name())
			fi, err := os.Stat(path) // follows symlinks, like dnsmasq
			if err != nil {
				continue
			}
			switch {
			case fi.IsDir():
				if depth < maxDepth {
					walk(path, depth+1)
				}
			case fi.Mode().IsRegular() && fi.Mode().Perm()&0004 != 0:
				name, err := filepath.Rel(root, path)
				if err != nil {
					continue
				}
				files = append(files, fileEntry{
					Name:     filepath.ToSlash(name),
					Size:     strconv.FormatInt(fi.Size(), 10),
					Modified: fi.ModTime().UTC().Format(time.RFC3339),
				})
			}
		}
	}
	walk(root, 0)

	sort.Slice(files, func(i, j int) bool { return files[i].Name < files[j].Name })
	return files, dirs
}
