// Package tree provides a concurrent in-memory store for per-module
// YANG operational data, keyed by module-qualified names like
// "ietf-system:system-state".
//
// Each module has its own read-write mutex, so writers for different
// modules never block each other.  All methods are safe for concurrent
// use.
package tree

import (
	"bytes"
	"encoding/json"
	"sync"
	"time"
)

// OnDemandFunc returns a JSON blob computed at call time.
// Registered providers are invoked on every Get/GetMulti to supply
// fields that must always be fresh (e.g. uptime, current-datetime).
type OnDemandFunc func() json.RawMessage

// modelEntry holds a single YANG module's pre-serialized JSON blob
// and its own read-write mutex.
type modelEntry struct {
	mu      sync.RWMutex
	data    json.RawMessage
	updated time.Time
}

// Tree holds the operational YANG data in per-module JSON blobs.
type Tree struct {
	mu        sync.RWMutex // protects the models map itself
	models    map[string]*modelEntry
	providers map[string]OnDemandFunc // on-demand overlay providers

	wmu      sync.RWMutex
	watchers map[*Watcher]struct{}
}

// Watcher collects the keys whose cached data changed.  C gets a
// token when there is something to Take.
type Watcher struct {
	C     chan struct{}
	keys  map[string]struct{} // nil for every key
	mu    sync.Mutex
	dirty map[string]struct{}
}

// Watch registers a Watcher for the given keys, or for every key when
// none is given.  It starts out with all of them changed.
func (t *Tree) Watch(keys ...string) *Watcher {
	w := &Watcher{C: make(chan struct{}, 1), dirty: make(map[string]struct{})}
	if len(keys) > 0 {
		w.keys = make(map[string]struct{}, len(keys))
		for _, key := range keys {
			w.keys[key] = struct{}{}
		}
	} else {
		keys = t.Keys()
	}
	t.wmu.Lock()
	t.watchers[w] = struct{}{}
	t.wmu.Unlock()
	for _, key := range keys {
		w.mark(key)
	}
	return w
}

// Unwatch removes a Watcher.
func (t *Tree) Unwatch(w *Watcher) {
	t.wmu.Lock()
	delete(t.watchers, w)
	t.wmu.Unlock()
}

// Take returns the keys changed since the last call.
func (w *Watcher) Take() []string {
	w.mu.Lock()
	defer w.mu.Unlock()
	keys := make([]string, 0, len(w.dirty))
	for key := range w.dirty {
		keys = append(keys, key)
	}
	clear(w.dirty)
	return keys
}

func (w *Watcher) mark(key string) {
	if _, ok := w.keys[key]; w.keys != nil && !ok {
		return
	}
	w.mu.Lock()
	w.dirty[key] = struct{}{}
	w.mu.Unlock()
	select {
	case w.C <- struct{}{}:
	default:
	}
}

func (t *Tree) changed(key string) {
	t.wmu.RLock()
	defer t.wmu.RUnlock()
	for w := range t.watchers {
		w.mark(key)
	}
}

// New creates an empty Tree.
func New() *Tree {
	return &Tree{
		models:    make(map[string]*modelEntry),
		providers: make(map[string]OnDemandFunc),
		watchers:  make(map[*Watcher]struct{}),
	}
}

// RegisterProvider adds an on-demand overlay for the given key.
// When Get or GetMulti reads this key the provider is called and its
// result is shallow-merged on top of the cached data.  The cached
// entry is never mutated — a merged copy is returned.
func (t *Tree) RegisterProvider(key string, fn OnDemandFunc) {
	t.mu.Lock()
	t.providers[key] = fn
	t.mu.Unlock()
}

// entry returns the model entry for key, creating it if absent.
func (t *Tree) entry(key string) *modelEntry {
	t.mu.RLock()
	entry, ok := t.models[key]
	t.mu.RUnlock()
	if ok {
		return entry
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	if entry, ok = t.models[key]; !ok {
		entry = &modelEntry{}
		t.models[key] = entry
	}
	return entry
}

// Set replaces the entire subtree at the given YANG module key.
// Only the target module's write lock is held; other modules remain
// readable and writable.
func (t *Tree) Set(key string, v json.RawMessage) {
	entry := t.entry(key)
	entry.mu.Lock()
	same := bytes.Equal(entry.data, v)
	entry.data = v
	entry.updated = time.Now()
	entry.mu.Unlock()
	if !same {
		t.changed(key)
	}
}

// Get returns the raw JSON for the given module key.
// If a provider is registered for key its output is shallow-merged on
// top of the cached data without mutating the cache.
func (t *Tree) Get(key string) json.RawMessage {
	t.mu.RLock()
	entry, ok := t.models[key]
	provider := t.providers[key]
	t.mu.RUnlock()
	if !ok {
		return nil
	}
	entry.mu.RLock()
	data := entry.data
	entry.mu.RUnlock()

	if provider == nil {
		return data
	}
	return ShallowMerge(data, provider())
}

// GetMulti returns the raw JSON for multiple module keys.
// Each module's read lock is acquired and released individually --
// the result is eventually consistent, not a snapshot.
// Providers are applied per-key, same as Get, and run without any
// tree lock held: a provider may read the tree itself.
func (t *Tree) GetMulti(keys []string) []json.RawMessage {
	type pick struct {
		entry    *modelEntry
		provider OnDemandFunc
	}
	picks := make([]pick, 0, len(keys))
	t.mu.RLock()
	for _, key := range keys {
		if entry, ok := t.models[key]; ok {
			picks = append(picks, pick{entry, t.providers[key]})
		}
	}
	t.mu.RUnlock()

	result := make([]json.RawMessage, 0, len(picks))
	for _, p := range picks {
		p.entry.mu.RLock()
		data := p.entry.data
		p.entry.mu.RUnlock()

		if p.provider != nil {
			data = ShallowMerge(data, p.provider())
		}
		result = append(result, data)
	}
	return result
}

// Keys returns all registered module keys.
func (t *Tree) Keys() []string {
	t.mu.RLock()
	defer t.mu.RUnlock()
	keys := make([]string, 0, len(t.models))
	for k := range t.models {
		keys = append(keys, k)
	}
	return keys
}

// ModelInfo holds metadata for a single model key.
type ModelInfo struct {
	LastUpdated time.Time
	SizeBytes   int
}

// Merge performs a shallow first-level JSON merge of partial into
// the existing blob at key.  If the key does not exist yet, partial
// becomes the entire value.  Each top-level field in partial
// overwrites the corresponding field in the existing object; fields
// not mentioned in partial are preserved.
//
// Both the existing data and partial must be JSON objects (maps).
// If either is not a valid JSON object, partial replaces the value.
func (t *Tree) Merge(key string, partial json.RawMessage) {
	entry := t.entry(key)
	entry.mu.Lock()
	old := entry.data
	entry.data = merge(old, partial)
	entry.updated = time.Now()
	same := bytes.Equal(old, entry.data)
	entry.mu.Unlock()
	if !same {
		t.changed(key)
	}
}

func merge(data, partial json.RawMessage) json.RawMessage {
	var base map[string]json.RawMessage
	if len(data) == 0 || json.Unmarshal(data, &base) != nil {
		base = make(map[string]json.RawMessage)
	}

	var overlay map[string]json.RawMessage
	if json.Unmarshal(partial, &overlay) != nil {
		return partial
	}

	for k, v := range overlay {
		base[k] = v
	}

	merged, err := json.Marshal(base)
	if err != nil {
		return partial
	}
	return merged
}

// Delete removes a key from the tree entirely.
func (t *Tree) Delete(key string) {
	t.mu.Lock()
	_, ok := t.models[key]
	delete(t.models, key)
	t.mu.Unlock()
	if ok {
		t.changed(key)
	}
}

// ShallowMerge overlays the top-level fields of overlay onto base and
// returns a new JSON blob.  Neither input is modified.  If either
// is not a valid JSON object the overlay wins outright.
func ShallowMerge(base, overlay json.RawMessage) json.RawMessage {
	if len(overlay) == 0 {
		return base
	}
	if len(base) == 0 {
		return overlay
	}

	var bm map[string]json.RawMessage
	if json.Unmarshal(base, &bm) != nil {
		return overlay
	}
	var om map[string]json.RawMessage
	if json.Unmarshal(overlay, &om) != nil {
		return overlay
	}

	for k, v := range om {
		bm[k] = v
	}

	merged, err := json.Marshal(bm)
	if err != nil {
		return overlay
	}
	return merged
}

// GetCached returns the raw cached JSON for the given module key
// WITHOUT invoking any registered provider.  This is safe to call
// from inside a provider closure (no recursion risk).
func (t *Tree) GetCached(key string) json.RawMessage {
	t.mu.RLock()
	entry, ok := t.models[key]
	t.mu.RUnlock()
	if !ok {
		return nil
	}
	entry.mu.RLock()
	defer entry.mu.RUnlock()
	return entry.data
}

// Info returns metadata for the given module key.
func (t *Tree) Info(key string) (ModelInfo, bool) {
	t.mu.RLock()
	entry, ok := t.models[key]
	t.mu.RUnlock()
	if !ok {
		return ModelInfo{}, false
	}
	entry.mu.RLock()
	defer entry.mu.RUnlock()
	return ModelInfo{
		LastUpdated: entry.updated,
		SizeBytes:   len(entry.data),
	}, true
}
