// Package alarmmonitor keeps the ietf-alarms:alarms subtree in step
// with the marker files under /run/alarm.  A file in active/<type>/<name>
// is a raised alarm, written by a yangerd producer, confd or a script.
// The monitor keeps what the markers cannot say, when an alarm was
// first raised and when it cleared, in entry/<type>/<name>, so the list
// survives a restart of yangerd and confd can purge entries by removing
// files.  Operator actions arrive as state/<type>/<name>, appended by
// confd, and the shelves and severity assignments as config.json,
// rendered by confd from the running configuration.
package alarmmonitor

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/kernelkit/infix/src/yangerd/internal/alarm"
	"github.com/kernelkit/infix/src/yangerd/internal/inotify"
	"github.com/kernelkit/infix/src/yangerd/internal/tree"
)

const (
	treeKey    = "ietf-alarms:alarms"
	configFile = "config.json"

	// debounceDelay coalesces a burst of markers, and the write of a
	// marker followed by confd's state file, into one rebuild.
	debounceDelay = 300 * time.Millisecond
)

// Monitor watches the marker directory.
type Monitor struct {
	tree *tree.Tree
	log  *slog.Logger

	root   string           // alarm.Root at New, overridable in tests
	ledDir string           // fault LED markers, "" disables them
	now    func() time.Time // overridable in tests

	watcher *inotify.Watcher
	watched map[string]bool
	cfg     Config
	raised  map[key]bool // last known state, for the transition log
	started bool
	warned  map[string]bool
}

type key struct{ typ, name string }

// entry is what the monitor keeps per alarm, beside the marker.
type entry struct {
	Resource    string         `json:"resource,omitempty"`
	Severity    alarm.Severity `json:"severity"`
	Text        string         `json:"text"`
	TimeCreated string         `json:"time-created"`
	LastRaised  string         `json:"last-raised"`
	LastChanged string         `json:"last-changed"`
	IsCleared   bool           `json:"is-cleared"`
	Shelf       string         `json:"shelf,omitempty"`
	OpState     string         `json:"operator-state,omitempty"` // last operator state, for the login banner
}

// opChange is one operator-state-change entry, as confd writes it.
type opChange struct {
	Time     string `json:"time"`
	Operator string `json:"operator"`
	State    string `json:"state"`
	Text     string `json:"text,omitempty"`
}

type activeEntry struct {
	marker alarm.Marker
	mtime  time.Time
}

// New creates a Monitor.
func New(t *tree.Tree, log *slog.Logger) *Monitor {
	if log == nil {
		log = slog.Default()
	}
	return &Monitor{
		tree:    t,
		log:     log,
		root:    alarm.Root,
		ledDir:  "/run/led",
		now:     time.Now,
		watched: make(map[string]bool),
		raised:  make(map[key]bool),
		warned:  make(map[string]bool),
	}
}

// Run watches the marker directory until ctx is cancelled.  Events
// only schedule a rebuild, all state is owned by this goroutine.
func (m *Monitor) Run(ctx context.Context) error {
	w, err := inotify.NewWatcher()
	if err != nil {
		return fmt.Errorf("inotify: %w", err)
	}
	defer w.Close()
	m.watcher = w

	entryDir := filepath.Join(m.root, "entry")
	timer := time.NewTimer(0)
	defer timer.Stop()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()

		case ev, ok := <-w.Events:
			if !ok {
				return fmt.Errorf("watcher closed")
			}
			if strings.HasPrefix(ev.Name, entryDir+"/") {
				continue // our own writes
			}
			timer.Reset(debounceDelay)

		case <-timer.C:
			m.reconcile()

		case err, ok := <-w.Errors:
			if !ok {
				return fmt.Errorf("watcher error channel closed")
			}
			m.log.Warn("alarm monitor: inotify error", "err", err)
		}
	}
}

// reconcile rebuilds the subtree from the directory.  Cheap enough to
// be a full rebuild: a device has tens of alarms, not thousands.
func (m *Monitor) reconcile() {
	if cfg, err := loadConfig(filepath.Join(m.root, configFile)); err != nil {
		m.log.Warn("alarm monitor: keeping previous configuration", "err", err)
	} else {
		m.cfg = cfg
	}

	active, dirs := m.scanActive()
	entries := m.scanEntries()
	states, stateDirs := m.scanState()
	m.syncWatches(append(dirs, stateDirs...))

	now := m.now().UTC().Truncate(time.Second).Format(time.RFC3339)

	// Raised, re-raised or updated markers.
	for k, a := range active {
		at, ok := inventory[k.typ]
		if !ok {
			if !m.warned[k.typ] {
				m.log.Warn("alarm monitor: ignoring unknown alarm type", "type", k.typ)
				m.warned[k.typ] = true
			}
			continue
		}
		sev := a.marker.Severity
		if rank(sev) < 0 {
			sev = at.Levels[0]
		}
		text := a.marker.Text
		if text == "" {
			text = at.Description
		}
		stamp := a.mtime.UTC().Truncate(time.Second).Format(time.RFC3339)

		mt, exists := entries[k]
		switch {
		case !exists:
			mt = entry{Severity: sev, Text: text, TimeCreated: stamp, LastRaised: stamp, LastChanged: stamp}
			resource, _ := alarm.SplitName(k.name)
			mt.Resource = resourceOf(k.typ, resource)
		case mt.IsCleared:
			mt.IsCleared = false
			mt.Severity, mt.Text = sev, text
			mt.LastRaised, mt.LastChanged = stamp, stamp
		case mt.Severity != sev || mt.Text != text:
			mt.Severity, mt.Text = sev, text
			mt.LastChanged = stamp
		}
		entries[k] = mt
	}

	// Cleared: an entry whose marker is gone.
	for k, mt := range entries {
		if _, ok := active[k]; ok || mt.IsCleared {
			continue
		}
		mt.IsCleared = true
		mt.LastChanged = now
		entries[k] = mt
	}

	// Shelving follows the configuration, so it is re-evaluated every
	// time and persisted for confd's purge-shelved-alarms.  The last
	// operator state is persisted too, the state files are confd's.
	for k, mt := range entries {
		resource, qualifier := alarm.SplitName(k.name)
		mt.Shelf = m.cfg.shelfFor(k.typ, resource, qualifier)
		mt.OpState = ""
		if ops := states[k]; len(ops) > 0 {
			mt.OpState = ops[len(ops)-1].State
		}
		entries[k] = mt
	}

	m.persist(entries)
	m.logTransitions(entries)
	m.publish(entries, states)
}

// persist writes every entry that differs from the file.
func (m *Monitor) persist(entries map[key]entry) {
	for k, mt := range entries {
		path := filepath.Join(m.root, "entry", k.typ, k.name)
		data, err := json.Marshal(mt)
		if err != nil {
			continue
		}
		if old, err := os.ReadFile(path); err == nil && string(old) == string(data) {
			continue
		}
		if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
			m.log.Warn("alarm monitor: mkdir", "path", path, "err", err)
			continue
		}
		tmp := path + ".tmp"
		if err := os.WriteFile(tmp, data, 0644); err != nil || os.Rename(tmp, path) != nil {
			m.log.Warn("alarm monitor: write", "path", path, "err", err)
		}
	}
}

// logTransitions logs each raise and clear once, to syslog.  The first
// pass after start only records what is already there.
func (m *Monitor) logTransitions(entries map[key]entry) {
	seen := make(map[key]bool, len(entries))
	for k, mt := range entries {
		raised := !mt.IsCleared
		seen[k] = true
		if m.started && m.raised[k] != raised {
			resource, qualifier := alarm.SplitName(k.name)
			attrs := []any{"type", k.typ, "resource", resource, "severity", mt.Severity, "text", mt.Text}
			if qualifier != "" {
				attrs = append(attrs, "qualifier", qualifier)
			}
			if raised {
				m.log.Warn("alarm raised", attrs...)
			} else {
				m.log.Info("alarm cleared", attrs...)
			}
		}
		m.raised[k] = raised
	}
	for k := range m.raised {
		if !seen[k] {
			delete(m.raised, k)
		}
	}
	m.started = true
}

type alarmJSON struct {
	Resource    string         `json:"resource"`
	TypeID      string         `json:"alarm-type-id"`
	Qualifier   string         `json:"alarm-type-qualifier"`
	TimeCreated string         `json:"time-created,omitempty"`
	IsCleared   bool           `json:"is-cleared"`
	LastRaised  string         `json:"last-raised"`
	LastChanged string         `json:"last-changed"`
	Severity    alarm.Severity `json:"perceived-severity"`
	Text        string         `json:"alarm-text"`
	ShelfName   string         `json:"shelf-name,omitempty"`
	Ops         []opChange     `json:"operator-state-change,omitempty"`
}

type inventoryJSON struct {
	TypeID      string           `json:"alarm-type-id"`
	Qualifier   string           `json:"alarm-type-qualifier"`
	WillClear   bool             `json:"will-clear"`
	Levels      []alarm.Severity `json:"severity-level"`
	Description string           `json:"description"`
}

type summaryJSON struct {
	Severity            alarm.Severity `json:"severity"`
	Total               int            `json:"total"`
	NotCleared          int            `json:"not-cleared"`
	Cleared             int            `json:"cleared"`
	ClearedNotClosed    int            `json:"cleared-not-closed"`
	ClearedClosed       int            `json:"cleared-closed"`
	NotClearedClosed    int            `json:"not-cleared-closed"`
	NotClearedNotClosed int            `json:"not-cleared-not-closed"`
}

// publish builds the subtree and hands it to the tree, then follows
// up with the fault LED markers.
func (m *Monitor) publish(entries map[key]entry, states map[key][]opChange) {
	keys := make([]key, 0, len(entries))
	for k := range entries {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool {
		if keys[i].typ != keys[j].typ {
			return keys[i].typ < keys[j].typ
		}
		return keys[i].name < keys[j].name
	})

	var list, shelved []alarmJSON
	var listChanged, shelvedChanged string
	summary := make([]summaryJSON, len(severities))
	for i, s := range severities {
		summary[i].Severity = s
	}
	fault := ""

	for _, k := range keys {
		mt := entries[k]
		at := inventory[k.typ]
		resource, qualifier := alarm.SplitName(k.name)
		ops := states[k]
		closed := len(ops) > 0 && ops[len(ops)-1].State == "closed"
		sev := m.cfg.assign(k.typ, resource, qualifier, at, mt.Severity)

		row := alarmJSON{
			Resource:    resourceOf(k.typ, resource),
			TypeID:      typeID(k.typ),
			Qualifier:   qualifier,
			IsCleared:   mt.IsCleared,
			LastRaised:  mt.LastRaised,
			LastChanged: mt.LastChanged,
			Severity:    sev,
			Text:        mt.Text,
			Ops:         ops,
		}
		changed := mt.LastChanged
		for _, op := range ops {
			changed = later(changed, op.Time)
		}

		if mt.Shelf != "" {
			row.ShelfName = mt.Shelf
			shelved = append(shelved, row)
			shelvedChanged = later(shelvedChanged, changed)
			continue
		}

		row.TimeCreated = mt.TimeCreated
		list = append(list, row)
		listChanged = later(listChanged, changed)

		s := &summary[rank(sev)]
		s.Total++
		switch {
		case mt.IsCleared && closed:
			s.Cleared++
			s.ClearedClosed++
		case mt.IsCleared:
			s.Cleared++
			s.ClearedNotClosed++
		case closed:
			s.NotCleared++
			s.NotClearedClosed++
		default:
			s.NotCleared++
			s.NotClearedNotClosed++
		}
		// The LED nags about what nobody has dealt with.
		if !mt.IsCleared && !closed {
			switch {
			case sev == alarm.Critical:
				fault = "crit"
			case fault != "crit" && (sev == alarm.Major || sev == alarm.Minor):
				fault = "err"
			}
		}
	}

	inv := make([]inventoryJSON, 0, len(inventory))
	for typ, at := range inventory {
		inv = append(inv, inventoryJSON{
			TypeID: typeID(typ), WillClear: at.WillClear, Levels: at.Levels, Description: at.Description,
		})
	}
	sort.Slice(inv, func(i, j int) bool { return inv[i].TypeID < inv[j].TypeID })

	alarms := map[string]any{
		"alarm-inventory": map[string]any{"alarm-type": inv},
		"summary":         map[string]any{"alarm-summary": summary},
		"alarm-list":      map[string]any{"number-of-alarms": len(list)},
		"shelved-alarms":  map[string]any{"number-of-shelved-alarms": len(shelved)},
	}
	if len(list) > 0 {
		alarms["alarm-list"].(map[string]any)["alarm"] = list
		alarms["alarm-list"].(map[string]any)["last-changed"] = listChanged
	}
	if len(shelved) > 0 {
		alarms["shelved-alarms"].(map[string]any)["shelved-alarm"] = shelved
		alarms["shelved-alarms"].(map[string]any)["shelved-alarms-last-changed"] = shelvedChanged
		alarms["summary"].(map[string]any)["shelves-active"] = []any{nil}
	}

	// The IPC layer adds the module-prefixed top node.
	data, err := json.Marshal(alarms)
	if err != nil {
		m.log.Warn("alarm monitor: marshal", "err", err)
		return
	}
	m.tree.Set(treeKey, data)
	m.setLEDs(fault)
}

// setLEDs keeps the iitod fault markers in step: one of fault-ok,
// fault-err and fault-crit is set at a time, for the alarms that are
// neither cleared nor closed.  No LED directory, no LEDs.
func (m *Monitor) setLEDs(fault string) {
	if m.ledDir == "" {
		return
	}
	if _, err := os.Stat(m.ledDir); err != nil {
		return
	}
	if fault == "" {
		fault = "ok"
	}
	for _, name := range []string{"ok", "err", "crit"} {
		path := filepath.Join(m.ledDir, "fault-"+name)
		if name == fault {
			if f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY, 0644); err == nil {
				f.Close()
			}
			continue
		}
		if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
			m.log.Debug("alarm monitor: led marker", "path", path, "err", err)
		}
	}
}

// later returns the later of two RFC 3339 stamps, an empty one losing.
func later(a, b string) string {
	if a == "" {
		return b
	}
	if b == "" {
		return a
	}
	ta, erra := time.Parse(time.RFC3339, a)
	tb, errb := time.Parse(time.RFC3339, b)
	if erra != nil || errb != nil {
		if a > b {
			return a
		}
		return b
	}
	if tb.After(ta) {
		return b
	}
	return a
}

// scanActive reads every marker and returns the type directories to
// watch.  Temp files of a marker being written are skipped.
func (m *Monitor) scanActive() (map[key]activeEntry, []string) {
	base := filepath.Join(m.root, "active")
	out := make(map[key]activeEntry)
	dirs := []string{m.root, base}

	types, _ := os.ReadDir(base)
	for _, t := range types {
		if !t.IsDir() {
			continue
		}
		dir := filepath.Join(base, t.Name())
		dirs = append(dirs, dir)
		files, _ := os.ReadDir(dir)
		for _, f := range files {
			if f.IsDir() || strings.HasSuffix(f.Name(), ".tmp") {
				continue
			}
			fi, err := f.Info()
			if err != nil {
				continue
			}
			var marker alarm.Marker
			if data, err := os.ReadFile(filepath.Join(dir, f.Name())); err == nil && len(data) > 0 {
				if err := json.Unmarshal(data, &marker); err != nil {
					m.log.Warn("alarm monitor: bad marker, using defaults", "type", t.Name(), "name", f.Name(), "err", err)
				}
			}
			out[key{t.Name(), f.Name()}] = activeEntry{marker: marker, mtime: fi.ModTime()}
		}
	}
	return out, dirs
}

func (m *Monitor) scanEntries() map[key]entry {
	out := make(map[key]entry)
	m.scanJSON("entry", func(k key, data []byte) {
		var mt entry
		if err := json.Unmarshal(data, &mt); err != nil {
			m.log.Warn("alarm monitor: bad entry, dropping", "type", k.typ, "name", k.name, "err", err)
			os.Remove(filepath.Join(m.root, "entry", k.typ, k.name))
			return
		}
		out[k] = mt
	})
	return out
}

func (m *Monitor) scanState() (map[key][]opChange, []string) {
	base := filepath.Join(m.root, "state")
	out := make(map[key][]opChange)
	dirs := []string{base}
	types, _ := os.ReadDir(base)
	for _, t := range types {
		if t.IsDir() {
			dirs = append(dirs, filepath.Join(base, t.Name()))
		}
	}
	m.scanJSON("state", func(k key, data []byte) {
		var ops []opChange
		if err := json.Unmarshal(data, &ops); err != nil {
			m.log.Warn("alarm monitor: bad state entry", "type", k.typ, "name", k.name, "err", err)
			return
		}
		out[k] = ops
	})
	return out, dirs
}

// scanJSON calls fn for every regular file two levels below sub.
func (m *Monitor) scanJSON(sub string, fn func(key, []byte)) {
	base := filepath.Join(m.root, sub)
	types, _ := os.ReadDir(base)
	for _, t := range types {
		if !t.IsDir() {
			continue
		}
		files, _ := os.ReadDir(filepath.Join(base, t.Name()))
		for _, f := range files {
			if f.IsDir() || strings.HasSuffix(f.Name(), ".tmp") {
				continue
			}
			data, err := os.ReadFile(filepath.Join(base, t.Name(), f.Name()))
			if err != nil {
				continue
			}
			fn(key{t.Name(), f.Name()}, data)
		}
	}
}

// syncWatches makes the watched directories equal to dirs.  Add on a
// live watch is a no-op, and a deleted directory already lost its watch.
func (m *Monitor) syncWatches(dirs []string) {
	want := make(map[string]bool, len(dirs))
	for _, dir := range dirs {
		want[dir] = true
	}
	for dir := range m.watched {
		if !want[dir] {
			if m.watcher != nil {
				_ = m.watcher.Remove(dir)
			}
			delete(m.watched, dir)
		}
	}
	for dir := range want {
		if m.watcher != nil {
			if err := m.watcher.Add(dir); err != nil {
				if !m.warned[dir] {
					m.log.Warn("alarm monitor: watch failed", "dir", dir, "err", err)
					m.warned[dir] = true
				}
				continue
			}
		}
		m.watched[dir] = true
	}
}
