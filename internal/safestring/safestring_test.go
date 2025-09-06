package safestring

import (
	"testing"
)

func TestNew(t *testing.T) {
	original := "test-secret-value"
	safe := New(original)

	if safe.String() != original {
		t.Errorf("Expected %q, got %q", original, safe.String())
	}

	if safe.Len() != len(original) {
		t.Errorf("Expected length %d, got %d", len(original), safe.Len())
	}
}

func TestFromBytes(t *testing.T) {
	original := []byte("test-bytes")
	safe := FromBytes(original)

	if string(safe.Bytes()) != string(original) {
		t.Errorf("Expected %q, got %q", string(original), string(safe.Bytes()))
	}

	// Verify it's a copy, not the same slice
	original[0] = 'X'
	if safe.String()[0] == 'X' {
		t.Error("SafeString should be a copy, not reference to original bytes")
	}
}

func TestZero(t *testing.T) {
	safe := New("secret-to-be-zeroed")
	originalLen := safe.Len()

	if originalLen == 0 {
		t.Fatal("SafeString should have non-zero length before zeroing")
	}

	safe.Zero()

	if safe.Len() != 0 {
		t.Error("SafeString length should be 0 after zeroing")
	}

	if safe.String() != "" {
		t.Error("SafeString should be empty after zeroing")
	}
}

func TestEqual(t *testing.T) {
	safe1 := New("same-value")
	safe2 := New("same-value")
	safe3 := New("different-value")

	if !safe1.Equal(safe2) {
		t.Error("Expected equal SafeStrings to be equal")
	}

	if safe1.Equal(safe3) {
		t.Error("Expected different SafeStrings to not be equal")
	}

	// Test nil cases
	var nilSafe *SafeString
	if safe1.Equal(nilSafe) {
		t.Error("SafeString should not equal nil")
	}

	if !nilSafe.Equal(nilSafe) {
		t.Error("Nil SafeStrings should equal each other")
	}
}

func TestEqualString(t *testing.T) {
	safe := New("test-value")

	if !safe.EqualString("test-value") {
		t.Error("SafeString should equal matching string")
	}

	if safe.EqualString("different-value") {
		t.Error("SafeString should not equal different string")
	}

	var nilSafe *SafeString
	if !nilSafe.EqualString("") {
		t.Error("Nil SafeString should equal empty string")
	}

	if nilSafe.EqualString("non-empty") {
		t.Error("Nil SafeString should not equal non-empty string")
	}
}

func TestClone(t *testing.T) {
	original := New("original-value")
	clone := original.Clone()

	if !original.Equal(clone) {
		t.Error("Clone should equal original")
	}

	// Modify original and ensure clone is unchanged
	original.AppendString("-modified")
	if clone.String() != "original-value" {
		t.Error("Clone should be independent of original")
	}
}

func TestAppend(t *testing.T) {
	safe := New("base")
	safe.Append([]byte("-append"))

	if safe.String() != "base-append" {
		t.Errorf("Expected 'base-append', got %q", safe.String())
	}

	safe.AppendString("-string")
	if safe.String() != "base-append-string" {
		t.Errorf("Expected 'base-append-string', got %q", safe.String())
	}
}

func TestTruncate(t *testing.T) {
	safe := New("long-test-string")
	originalData := safe.Bytes() // Get copy before truncation

	safe.Truncate(4)

	if safe.String() != "long" {
		t.Errorf("Expected 'long', got %q", safe.String())
	}

	if safe.Len() != 4 {
		t.Errorf("Expected length 4, got %d", safe.Len())
	}

	// Test that original data beyond truncation point was zeroed
	// Note: We can't directly verify this since we don't expose internals,
	// but we can verify the behavior is correct
	safe.Truncate(0)
	if safe.String() != "" {
		t.Error("Expected empty string after truncating to 0")
	}

	// Test negative length
	safe2 := New("test")
	safe2.Truncate(-1)
	if safe2.String() != "" {
		t.Error("Expected empty string after truncating to negative length")
	}

	_ = originalData // Use variable to avoid lint warning
}

func TestPool(t *testing.T) {
	pool := NewPool(2)

	// Get from empty pool
	safe1 := pool.Get()
	if safe1 == nil {
		t.Error("Expected non-nil SafeString from pool")
	}

	// Use the SafeString
	safe1.AppendString("pooled-value")
	if safe1.String() != "pooled-value" {
		t.Errorf("Expected 'pooled-value', got %q", safe1.String())
	}

	// Return to pool
	pool.Put(safe1)

	// Get again (should be zeroed)
	safe2 := pool.Get()
	if safe2.String() != "" {
		t.Error("Expected zeroed SafeString from pool")
	}
}

func TestIsEmpty(t *testing.T) {
	safe := New("")
	if !safe.IsEmpty() {
		t.Error("Expected empty SafeString to report as empty")
	}

	safe.AppendString("not-empty")
	if safe.IsEmpty() {
		t.Error("Expected non-empty SafeString to report as not empty")
	}

	safe.Zero()
	if !safe.IsEmpty() {
		t.Error("Expected zeroed SafeString to report as empty")
	}
}

func TestConstantTimeComparison(t *testing.T) {
	// This test ensures our comparison is actually constant-time
	// We can't easily test timing, but we can test correctness
	safe1 := New("password123")
	safe2 := New("password123")
	safe3 := New("password124") // One character different

	if !safe1.Equal(safe2) {
		t.Error("Expected identical SafeStrings to be equal")
	}

	if safe1.Equal(safe3) {
		t.Error("Expected different SafeStrings to not be equal")
	}

	// Test with strings of different lengths
	safe4 := New("short")
	safe5 := New("much-longer-string")

	if safe4.Equal(safe5) {
		t.Error("Expected strings of different lengths to not be equal")
	}
}

// Benchmark tests for performance validation
func BenchmarkNew(b *testing.B) {
	testString := "benchmark-test-string-value"
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		safe := New(testString)
		_ = safe
	}
}

func BenchmarkZero(b *testing.B) {
	testString := "benchmark-test-string-value"
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		b.StopTimer()
		safe := New(testString)
		b.StartTimer()
		safe.Zero()
	}
}

func BenchmarkEqual(b *testing.B) {
	safe1 := New("benchmark-test-string")
	safe2 := New("benchmark-test-string")
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		safe1.Equal(safe2)
	}
}
