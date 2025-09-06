package cache

import (
	"sync"
	"time"
	"unsafe"

	"github.com/zach-source/opx/internal/safestring"
)

type entry struct {
	v      *safestring.SafeString
	exp    time.Time
	cached time.Time
}

type Cache struct {
	mu       sync.RWMutex
	data     map[string]entry
	ttl      time.Duration
	hits     int64
	misses   int64
	inflight int
}

func New(ttl time.Duration) *Cache {
	return &Cache{
		data: make(map[string]entry),
		ttl:  ttl,
	}
}

func (c *Cache) Get(key string) (string, bool, time.Time, time.Time) {
	c.mu.RLock()
	e, ok := c.data[key]
	c.mu.RUnlock()
	if !ok || time.Now().After(e.exp) {
		if ok {
			// treat expired as miss
		}
		return "", false, time.Time{}, time.Time{}
	}
	return e.v.String(), true, e.exp, e.cached
}

func (c *Cache) Set(key, val string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	// Zero any existing entry before replacing
	if existing, exists := c.data[key]; exists {
		existing.v.Zero()
	}

	c.data[key] = entry{v: safestring.New(val), exp: time.Now().Add(c.ttl), cached: time.Now()}
}

func (c *Cache) Stats() (size int, hits, misses int64, inflight int) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return len(c.data), c.hits, c.misses, c.inflight
}

func (c *Cache) IncHit()      { c.mu.Lock(); c.hits++; c.mu.Unlock() }
func (c *Cache) IncMiss()     { c.mu.Lock(); c.misses++; c.mu.Unlock() }
func (c *Cache) IncInFlight() { c.mu.Lock(); c.inflight++; c.mu.Unlock() }
func (c *Cache) DecInFlight() {
	c.mu.Lock()
	if c.inflight > 0 {
		c.inflight--
	}
	c.mu.Unlock()
}

// Best-effort zeroize when replacing strings (Go GC caveats apply).
func ZeroizeString(s *string) {
	if s == nil {
		return
	}
	hdr := (*[2]uintptr)(unsafe.Pointer(s))
	p := (*byte)(unsafe.Pointer(hdr[0]))
	if p == nil {
		return
	}
	l := int(hdr[1])
	b := unsafe.Slice(p, l)
	for i := range b {
		b[i] = 0
	}
}

func (c *Cache) TTL() time.Duration {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.ttl
}

// CleanupExpired removes expired entries from the cache
func (c *Cache) CleanupExpired() int {
	c.mu.Lock()
	defer c.mu.Unlock()

	now := time.Now()
	removed := 0
	for key, entry := range c.data {
		if now.After(entry.exp) {
			// Securely zero the SafeString before removal
			entry.v.Zero()
			delete(c.data, key)
			removed++
		}
	}
	return removed
}

// Clear removes all entries from the cache with secure zeroization
func (c *Cache) Clear() int {
	c.mu.Lock()
	defer c.mu.Unlock()

	removed := len(c.data)
	for key, entry := range c.data {
		// Securely zero the SafeString before removal
		entry.v.Zero()
		delete(c.data, key)
	}
	return removed
}
