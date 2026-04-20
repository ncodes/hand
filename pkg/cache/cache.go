package cache

import (
	"sync"
	"time"
)

type Options[K comparable, V any] struct {
	TTL   time.Duration
	Now   func() time.Time
	Clone func(V) V
}

type Cache[K comparable, V any] struct {
	ttl     time.Duration
	now     func() time.Time
	clone   func(V) V
	mu      sync.RWMutex
	entries map[K]entry[V]
}

type entry[V any] struct {
	expiresAt time.Time
	value     V
}

func New[K comparable, V any](opts Options[K, V]) *Cache[K, V] {
	if opts.TTL <= 0 {
		return nil
	}

	now := opts.Now
	if now == nil {
		now = time.Now
	}

	clone := opts.Clone
	if clone == nil {
		clone = func(value V) V { return value }
	}

	c := &Cache[K, V]{
		ttl:     opts.TTL,
		now:     now,
		clone:   clone,
		entries: make(map[K]entry[V]),
	}

	return c
}

func (c *Cache[K, V]) Get(key K) (V, bool) {
	var zero V
	if c == nil {
		return zero, false
	}

	c.mu.RLock()
	entry, ok := c.entries[key]
	c.mu.RUnlock()
	if !ok {
		return zero, false
	}

	if !c.now().Before(entry.expiresAt) {
		c.mu.Lock()
		entry, ok = c.entries[key]
		if ok && !c.now().Before(entry.expiresAt) {
			delete(c.entries, key)
			ok = false
		}
		c.mu.Unlock()
		if !ok {
			return zero, false
		}
	}

	return c.clone(entry.value), true
}

func (c *Cache[K, V]) Set(key K, value V) {
	if c == nil {
		return
	}

	c.mu.Lock()
	c.entries[key] = entry[V]{
		expiresAt: c.now().Add(c.ttl),
		value:     c.clone(value),
	}
	c.mu.Unlock()

	time.AfterFunc(c.ttl, c.cleanupExpired)
}

func (c *Cache[K, V]) Delete(key K) {
	if c == nil {
		return
	}

	c.mu.Lock()
	delete(c.entries, key)
	c.mu.Unlock()
}

func (c *Cache[K, V]) Len() int {
	if c == nil {
		return 0
	}

	c.cleanupExpired()

	c.mu.RLock()
	defer c.mu.RUnlock()

	return len(c.entries)
}

func (c *Cache[K, V]) cleanupExpired() {
	if c == nil {
		return
	}

	now := c.now()

	c.mu.Lock()
	for key, entry := range c.entries {
		if !now.Before(entry.expiresAt) {
			delete(c.entries, key)
		}
	}
	c.mu.Unlock()
}
