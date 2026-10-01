package schema

import (
	"context"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"infix/webui/internal/restconf"
)

// Cache holds a lazily-loaded schema Manager and refreshes it at startup.
// All methods are safe for concurrent use.
type Cache struct {
	mu      sync.RWMutex
	manager *Manager
	syncing bool // guarded by mu
	dir     string
	version string // image version the cached files belong to
	rc      restconf.Fetcher
}

// NewCache creates a Cache for the YANG files of the given image version,
// empty when unknown.
// Call LoadFromCacheBackground at startup, then RefreshBackground after login.
func NewCache(rc restconf.Fetcher, dir, version string) *Cache {
	return &Cache{rc: rc, dir: dir, version: version}
}

// dropStale empties the cache when another image version wrote it.  The
// YANG files ship with the image, and a module's revision does not always
// change when its text does, so the version is the only reliable key.
func (c *Cache) dropStale() error {
	if c.version == "" {
		return nil
	}
	stamp := filepath.Join(c.dir, ".version")
	if b, err := os.ReadFile(stamp); err == nil && strings.TrimSpace(string(b)) == c.version {
		return nil
	}
	if err := os.RemoveAll(c.dir); err != nil {
		return err
	}
	if err := os.MkdirAll(c.dir, 0750); err != nil {
		return err
	}
	log.Printf("schema: cache in %s is for another image, dropped", c.dir)
	return os.WriteFile(stamp, []byte(c.version+"\n"), 0640)
}

// LoadFromCache parses whatever .yang files are already in the cache
// directory.  It makes no HTTP requests and needs no credentials.
// This is fast — suitable for server startup.  If the directory is empty
// or has too few files to form a useful schema, the Manager is left nil.
func (c *Cache) LoadFromCache() error {
	if err := c.dropStale(); err != nil {
		return fmt.Errorf("schema: cache version check: %w", err)
	}
	entries, err := os.ReadDir(c.dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil // nothing cached yet — that is fine
		}
		return err
	}
	var count int
	for _, e := range entries {
		if !e.IsDir() && len(e.Name()) > 5 { // len(".yang") == 5
			if e.Name()[len(e.Name())-5:] == ".yang" {
				count++
			}
		}
	}
	if count == 0 {
		return nil // empty cache — wait for first Refresh
	}

	mgr, err := Load(c.dir)
	if err != nil {
		return fmt.Errorf("schema: load from cache: %w", err)
	}
	c.mu.Lock()
	c.manager = mgr
	c.mu.Unlock()
	log.Printf("schema: loaded %d cached YANG file(s) from %s", count, c.dir)
	return nil
}

// LoadFromCacheBackground calls LoadFromCache in a goroutine. Errors are logged.
func (c *Cache) LoadFromCacheBackground() {
	go func() {
		if err := c.LoadFromCache(); err != nil {
			log.Printf("schema: cache load failed: %v", err)
		}
	}()
}

// Refresh downloads any missing YANG files from the device (credentials must
// be present in ctx) and then reloads the schema Manager.
// Only one refresh runs at a time; concurrent calls return immediately.
func (c *Cache) Refresh(ctx context.Context) error {
	c.mu.Lock()
	if c.syncing {
		c.mu.Unlock()
		return nil
	}
	c.syncing = true
	c.mu.Unlock()

	defer func() {
		c.mu.Lock()
		c.syncing = false
		c.mu.Unlock()
	}()

	if _, err := FetchModules(ctx, c.rc, c.dir); err != nil {
		return fmt.Errorf("schema refresh: fetch: %w", err)
	}

	mgr, err := Load(c.dir)
	if err != nil {
		return fmt.Errorf("schema refresh: load: %w", err)
	}

	c.mu.Lock()
	c.manager = mgr
	c.mu.Unlock()

	log.Printf("schema: refreshed successfully from %s", c.dir)
	return nil
}

// RefreshBackground calls Refresh in a goroutine. Errors are logged.
// The context's values (credentials) are preserved but its cancellation is
// detached so the goroutine is not killed when the originating HTTP request
// completes.
func (c *Cache) RefreshBackground(ctx context.Context) {
	detached := context.WithoutCancel(ctx)
	go func() {
		if err := c.Refresh(detached); err != nil {
			log.Printf("schema: background refresh failed: %v", err)
		}
	}()
}

// Manager returns the current Manager, or nil if not yet loaded.
func (c *Cache) Manager() *Manager {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.manager
}
