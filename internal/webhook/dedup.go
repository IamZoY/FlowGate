package webhook

import (
	"sync"
	"time"
)

type deduper struct {
	mu      sync.Mutex
	entries map[string]time.Time
	window  time.Duration
}

func newDeduper(window time.Duration) *deduper {
	d := &deduper{
		entries: make(map[string]time.Time),
		window:  window,
	}
	go d.cleanup()
	return d
}

// IsDuplicate returns true if the key was seen within the dedup window.
// If not a duplicate, it atomically marks the key as seen.
func (d *deduper) IsDuplicate(key string) bool {
	d.mu.Lock()
	defer d.mu.Unlock()
	if t, ok := d.entries[key]; ok && time.Since(t) < d.window {
		return true
	}
	d.entries[key] = time.Now()
	return false
}

func (d *deduper) cleanup() {
	ticker := time.NewTicker(d.window)
	defer ticker.Stop()
	for range ticker.C {
		d.mu.Lock()
		now := time.Now()
		for k, t := range d.entries {
			if now.Sub(t) >= d.window {
				delete(d.entries, k)
			}
		}
		d.mu.Unlock()
	}
}
