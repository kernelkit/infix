// Package tree provides a concurrent in-memory store for per-module
// YANG operational data, keyed by module-qualified names like
// "ietf-system:system-state".
//
// Each module has its own read-write mutex, so writers for different
// modules never block each other.  All methods are safe for concurrent
// use.
package tree

import (
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
}

// New creates an empty Tree.
func New() *Tree {
	return &Tree{
		models:    make(map[string]*modelEntry),
		providers: make(map[string]OnDemandFunc),
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

// Set replaces the entire subtree at the given YANG module key.
// Only the target module's write lock is held; other modules remain
// readable and writable.
func (t *Tree) Set(key string, v json.RawMessage) {
	t.mu.RLock()
	entry, ok := t.models[key]
	t.mu.RUnlock()
	if !ok {
		t.mu.Lock()
		entry, ok = t.models[key]
		if !ok {
			entry = &modelEntry{}
			t.models[key] = entry
		}
		t.mu.Unlock()
	}
	entry.mu.Lock()
	entry.data = v
	entry.updated = time.Now()
	entry.mu.Unlock()
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
	return shallowMerge(data, provider())
}

// GetMulti returns the raw JSON for multiple module keys.
// Each module's read lock is acquired and released individually —
// the result is eventually consistent, not a snapshot.
// Providers are applied per-key, same as Get.
func (t *Tree) GetMulti(keys []string) []json.RawMessage {
	result := make([]json.RawMessage, 0, len(keys))
	t.mu.RLock()
	defer t.mu.RUnlock()
	for _, key := range keys {
		entry, ok := t.models[key]
		if !ok {
			continue
		}
		entry.mu.RLock()
		data := entry.data
		entry.mu.RUnlock()

		if provider, has := t.providers[key]; has {
			data = shallowMerge(data, provider())
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
// If either is not a valid JSON object, Merge falls back to Set.
func (t *Tree) Merge(key string, partial json.RawMessage) {
	t.mu.RLock()
	entry, ok := t.models[key]
	t.mu.RUnlock()

	if !ok {
		// No existing entry — just set.
		t.Set(key, partial)
		return
	}

	entry.mu.Lock()
	defer entry.mu.Unlock()

	// Unmarshal existing data.
	var base map[string]json.RawMessage
	if len(entry.data) == 0 || json.Unmarshal(entry.data, &base) != nil {
		base = make(map[string]json.RawMessage)
	}

	// Unmarshal partial.
	var overlay map[string]json.RawMessage
	if json.Unmarshal(partial, &overlay) != nil {
		// partial is not a JSON object — fall back to full replace.
		entry.data = partial
		entry.updated = time.Now()
		return
	}

	for k, v := range overlay {
		base[k] = v
	}

	merged, err := json.Marshal(base)
	if err != nil {
		// Should never happen with valid JSON inputs.
		entry.data = partial
		entry.updated = time.Now()
		return
	}
	entry.data = merged
	entry.updated = time.Now()
}

// Delete removes a key from the tree entirely.
func (t *Tree) Delete(key string) {
	t.mu.Lock()
	delete(t.models, key)
	t.mu.Unlock()
}

// shallowMerge overlays the top-level fields of overlay onto base and
// returns a new JSON blob.  Neither input is modified.  If either
// is not a valid JSON object the overlay wins outright.
func shallowMerge(base, overlay json.RawMessage) json.RawMessage {
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
