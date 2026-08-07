package cache

import (
	"fmt"
	"strings"
	"sync"
	"time"
	"unsafe"

	"github.com/zach-source/opx/internal/safestring"
)

// DefaultTTL is how long a fetched secret stays hot. It is deliberately long:
// the whole point of the daemon is that one 1Password unlock covers a work
// block instead of prompting again every couple of minutes.
const DefaultTTL = 4 * time.Hour

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

	// store, when set, mirrors the entries to an encrypted file so a daemon
	// restart comes back warm instead of re-prompting.
	store *Store
	onErr func(error)
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
	c.persistLocked()
}

// Persist attaches an encrypted store and restores whatever unexpired entries it
// holds, so the daemon starts warm. Returns the number of entries restored.
//
// maxIdle, when non-zero, discards the whole file if it has sat untouched for
// longer than the session idle timeout. Without that, restarting the daemon
// would reset the idle clock and keep serving a cache that should have been
// wiped by an idle lock.
func (c *Cache) Persist(s *Store, maxIdle time.Duration, onErr func(error)) (int, error) {
	records, savedAt, err := s.Load()
	if err != nil {
		// The file is unreadable (wrong key, tampered, corrupt). Drop it and carry
		// on persisting, otherwise a single bad file disables persistence forever
		// and strands a stale blob on disk.
		delErr := s.Delete()
		c.attach(s, onErr)
		if delErr != nil {
			return 0, fmt.Errorf("%w (and could not remove it: %v)", err, delErr)
		}
		return 0, err
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	c.store = s
	c.onErr = onErr

	if maxIdle > 0 && !savedAt.IsZero() && time.Since(savedAt) > maxIdle {
		if delErr := s.Delete(); delErr != nil && onErr != nil {
			onErr(delErr)
		}
		return 0, nil
	}

	now := time.Now()
	restored := 0
	for _, r := range records {
		if now.After(r.Exp) {
			continue
		}
		c.data[r.Key] = entry{v: safestring.New(r.Value), exp: r.Exp, cached: r.Cached}
		restored++
	}

	// Expired entries were dropped, so rewrite the file to match.
	if restored != len(records) {
		c.persistLocked()
	}
	return restored, nil
}

func (c *Cache) attach(s *Store, onErr func(error)) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.store = s
	c.onErr = onErr
}

// persistLocked mirrors the current entries to disk. Caller must hold c.mu.
// ponytail: rewrites the whole file under the cache lock. Fine for the tens of
// secrets a daemon actually holds; batch behind a dirty flag if it ever holds thousands.
func (c *Cache) persistLocked() {
	if c.store == nil {
		return
	}

	records := make([]Record, 0, len(c.data))
	for k, e := range c.data {
		records = append(records, Record{Key: k, Value: e.v.String(), Exp: e.exp, Cached: e.cached})
	}

	if err := c.store.Save(records); err != nil && c.onErr != nil {
		c.onErr(err)
	}
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
	if removed > 0 {
		c.persistLocked()
	}
	return removed
}

// Invalidate drops every cached entry for a ref, across all flag variants
// (the cache key is "ref" or "ref|flags:..."), and returns how many it removed.
//
// Rotating a secret out of band leaves the daemon serving the old value until
// its TTL runs out; this is how a writer tells it otherwise.
func (c *Cache) Invalidate(ref string) int {
	c.mu.Lock()
	defer c.mu.Unlock()

	removed := 0
	for key, e := range c.data {
		if key != ref && !strings.HasPrefix(key, ref+"|flags:") {
			continue
		}
		e.v.Zero()
		delete(c.data, key)
		removed++
	}
	if removed > 0 {
		c.persistLocked()
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

	// A session lock clears the cache for security; the on-disk copy has to go
	// with it, or a restart would resurrect secrets the lock just discarded.
	if c.store != nil {
		if err := c.store.Delete(); err != nil && c.onErr != nil {
			c.onErr(err)
		}
	}
	return removed
}
