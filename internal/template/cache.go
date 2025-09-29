package template

import (
	"sync"
	"text/template"
)

// Cache provides thread-safe template compilation caching
type Cache struct {
	templates map[string]*template.Template
	mu        sync.RWMutex
	maxSize   int
}

// NewCache creates a new template cache with specified maximum size
func NewCache(maxSize int) *Cache {
	if maxSize <= 0 {
		maxSize = 100 // Default size
	}
	return &Cache{
		templates: make(map[string]*template.Template),
		maxSize:   maxSize,
	}
}

// GetOrCompile retrieves a compiled template from cache or compiles it
func (c *Cache) GetOrCompile(templateStr string, funcMap template.FuncMap) (*template.Template, error) {
	// Try to get from cache first (read lock)
	c.mu.RLock()
	if tmpl, exists := c.templates[templateStr]; exists {
		c.mu.RUnlock()
		return tmpl, nil
	}
	c.mu.RUnlock()

	// Not in cache, compile with write lock
	c.mu.Lock()
	defer c.mu.Unlock()

	// Double-check after acquiring write lock (prevent race condition)
	if tmpl, exists := c.templates[templateStr]; exists {
		return tmpl, nil
	}

	// Compile the template
	tmpl, err := template.New("").Funcs(funcMap).Parse(templateStr)
	if err != nil {
		return nil, err
	}

	// Add to cache with size limit (simple LRU)
	if len(c.templates) >= c.maxSize {
		// Remove first entry (oldest)
		for key := range c.templates {
			delete(c.templates, key)
			break
		}
	}

	c.templates[templateStr] = tmpl
	return tmpl, nil
}

// Size returns the current number of cached templates
func (c *Cache) Size() int {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return len(c.templates)
}

// Clear removes all cached templates
func (c *Cache) Clear() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.templates = make(map[string]*template.Template)
}
