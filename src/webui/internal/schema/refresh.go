package schema

import (
	"context"
	"fmt"
	"log"
	"os"
	"sync"

	"github.com/kernelkit/webui/internal/restconf"
)

// Cache holds a lazily-loaded schema Manager and refreshes it at startup.
// All methods are safe for concurrent use.
type Cache struct {
	mu       sync.RWMutex
	manager  *Manager
	syncing  bool // guarded by mu
	dir      string
	rc       restconf.Fetcher
}

// NewCache creates a Cache.
// Call LoadFromCacheBackground at startup, then RefreshBackground after login.
func NewCache(rc restconf.Fetcher, dir string) *Cache {
	return &Cache{rc: rc, dir: dir}
}

// LoadFromCache parses whatever .yang files are already in the cache
// directory.  It makes no HTTP requests and needs no credentials.
// This is fast — suitable for server startup.  If the directory is empty
// or has too few files to form a useful schema, the Manager is left nil.
func (c *Cache) LoadFromCache() error {
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
