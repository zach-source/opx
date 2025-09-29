package template

import (
	"testing"
	"text/template"
)

func TestNewCache(t *testing.T) {
	cache := NewCache(50)
	if cache == nil {
		t.Fatal("NewCache returned nil")
	}

	if cache.Size() != 0 {
		t.Errorf("Expected empty cache, got size %d", cache.Size())
	}

	// Test default size when invalid size provided
	cache = NewCache(0)
	if cache.maxSize != 100 {
		t.Errorf("Expected default maxSize 100, got %d", cache.maxSize)
	}
}

func TestCacheGetOrCompile(t *testing.T) {
	cache := NewCache(5)
	funcMap := template.FuncMap{
		"upper": func(s string) string { return s },
	}

	// Test compilation and caching
	templateStr := "{{.Value | upper}}"
	tmpl1, err := cache.GetOrCompile(templateStr, funcMap)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	if tmpl1 == nil {
		t.Fatal("GetOrCompile returned nil template")
	}

	if cache.Size() != 1 {
		t.Errorf("Expected cache size 1, got %d", cache.Size())
	}

	// Test cache hit (should return same template instance)
	tmpl2, err := cache.GetOrCompile(templateStr, funcMap)
	if err != nil {
		t.Fatalf("Unexpected error on cache hit: %v", err)
	}

	if tmpl1 != tmpl2 {
		t.Error("Expected same template instance from cache")
	}

	if cache.Size() != 1 {
		t.Errorf("Expected cache size still 1, got %d", cache.Size())
	}
}

func TestCacheLRUEviction(t *testing.T) {
	cache := NewCache(2) // Small cache for testing eviction
	funcMap := template.FuncMap{}

	// Add templates to fill cache
	_, err := cache.GetOrCompile("{{.Value1}}", funcMap)
	if err != nil {
		t.Fatalf("Error adding template 1: %v", err)
	}

	_, err = cache.GetOrCompile("{{.Value2}}", funcMap)
	if err != nil {
		t.Fatalf("Error adding template 2: %v", err)
	}

	if cache.Size() != 2 {
		t.Errorf("Expected cache size 2, got %d", cache.Size())
	}

	// Add third template, should evict oldest
	_, err = cache.GetOrCompile("{{.Value3}}", funcMap)
	if err != nil {
		t.Fatalf("Error adding template 3: %v", err)
	}

	if cache.Size() != 2 {
		t.Errorf("Expected cache size still 2 after eviction, got %d", cache.Size())
	}
}

func TestCacheInvalidTemplate(t *testing.T) {
	cache := NewCache(5)
	funcMap := template.FuncMap{}

	// Test compilation error
	_, err := cache.GetOrCompile("{{.Value | invalid}}", funcMap)
	if err == nil {
		t.Error("Expected compilation error for invalid template")
	}

	// Cache should not store failed compilations
	if cache.Size() != 0 {
		t.Errorf("Expected cache size 0 after failed compilation, got %d", cache.Size())
	}
}

func TestCacheClear(t *testing.T) {
	cache := NewCache(5)
	funcMap := template.FuncMap{}

	// Add some templates
	_, _ = cache.GetOrCompile("{{.Value1}}", funcMap)
	_, _ = cache.GetOrCompile("{{.Value2}}", funcMap)

	if cache.Size() != 2 {
		t.Errorf("Expected cache size 2, got %d", cache.Size())
	}

	// Clear cache
	cache.Clear()

	if cache.Size() != 0 {
		t.Errorf("Expected cache size 0 after clear, got %d", cache.Size())
	}
}

func TestCacheConcurrency(t *testing.T) {
	cache := NewCache(10)
	funcMap := template.FuncMap{}

	// Test concurrent access
	templateStr := "{{.Value}}"

	done := make(chan bool, 10)
	for i := 0; i < 10; i++ {
		go func() {
			_, err := cache.GetOrCompile(templateStr, funcMap)
			if err != nil {
				t.Errorf("Concurrent access error: %v", err)
			}
			done <- true
		}()
	}

	// Wait for all goroutines
	for i := 0; i < 10; i++ {
		<-done
	}

	// Should only have one template cached
	if cache.Size() != 1 {
		t.Errorf("Expected cache size 1 after concurrent access, got %d", cache.Size())
	}
}
