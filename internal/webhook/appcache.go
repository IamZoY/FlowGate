package webhook

import (
	"sync"
	"time"

	"github.com/ali/flowgate/internal/group"
)

type cachedApp struct {
	app     *group.App
	expires time.Time
}

type appCache struct {
	mu      sync.RWMutex
	entries map[string]cachedApp
	ttl     time.Duration
}

func newAppCache(ttl time.Duration) *appCache {
	c := &appCache{
		entries: make(map[string]cachedApp),
		ttl:     ttl,
	}
	go c.cleanup()
	return c
}

// Get returns the cached decrypted app, or nil if not found / expired.
func (c *appCache) Get(key string) *group.App {
	c.mu.RLock()
	entry, ok := c.entries[key]
	c.mu.RUnlock()
	if !ok || time.Now().After(entry.expires) {
		return nil
	}
	return entry.app
}

// Set stores a decrypted app with the configured TTL.
func (c *appCache) Set(key string, app *group.App) {
	c.mu.Lock()
	c.entries[key] = cachedApp{app: app, expires: time.Now().Add(c.ttl)}
	c.mu.Unlock()
}

func (c *appCache) cleanup() {
	ticker := time.NewTicker(c.ttl)
	defer ticker.Stop()
	for range ticker.C {
		c.mu.Lock()
		now := time.Now()
		for k, v := range c.entries {
			if now.After(v.expires) {
				delete(c.entries, k)
			}
		}
		c.mu.Unlock()
	}
}
