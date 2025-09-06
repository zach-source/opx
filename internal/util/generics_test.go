package util

import (
	"strconv"
	"testing"
)

func TestContains(t *testing.T) {
	// Test with strings
	strings := []string{"apple", "banana", "cherry"}
	if !Contains(strings, "banana") {
		t.Error("Expected Contains to find 'banana'")
	}
	if Contains(strings, "orange") {
		t.Error("Expected Contains to not find 'orange'")
	}

	// Test with integers
	ints := []int{1, 2, 3, 4, 5}
	if !Contains(ints, 3) {
		t.Error("Expected Contains to find 3")
	}
	if Contains(ints, 10) {
		t.Error("Expected Contains to not find 10")
	}
}

func TestMap(t *testing.T) {
	// Test mapping integers to strings
	ints := []int{1, 2, 3, 4}
	strings := Map(ints, func(i int) string {
		return strconv.Itoa(i * 2)
	})

	expected := []string{"2", "4", "6", "8"}
	if len(strings) != len(expected) {
		t.Fatalf("Expected %d strings, got %d", len(expected), len(strings))
	}

	for i, s := range strings {
		if s != expected[i] {
			t.Errorf("Expected %s at index %d, got %s", expected[i], i, s)
		}
	}
}

func TestFilter(t *testing.T) {
	// Test filtering even numbers
	numbers := []int{1, 2, 3, 4, 5, 6, 7, 8}
	evens := Filter(numbers, func(n int) bool {
		return n%2 == 0
	})

	expected := []int{2, 4, 6, 8}
	if len(evens) != len(expected) {
		t.Fatalf("Expected %d even numbers, got %d", len(expected), len(evens))
	}

	for i, n := range evens {
		if n != expected[i] {
			t.Errorf("Expected %d at index %d, got %d", expected[i], i, n)
		}
	}
}

func TestFindFirst(t *testing.T) {
	// Test finding first element > 5
	numbers := []int{1, 2, 3, 7, 8, 9}
	result, found := FindFirst(numbers, func(n int) bool {
		return n > 5
	})

	if !found {
		t.Error("Expected to find element > 5")
	}
	if result != 7 {
		t.Errorf("Expected first element > 5 to be 7, got %d", result)
	}

	// Test not finding element
	_, found = FindFirst(numbers, func(n int) bool {
		return n > 10
	})

	if found {
		t.Error("Expected not to find element > 10")
	}
}

func TestGroupBy(t *testing.T) {
	// Test grouping strings by length
	words := []string{"cat", "dog", "elephant", "fly", "butterfly"}
	grouped := GroupBy(words, func(s string) int {
		return len(s)
	})

	if len(grouped[3]) != 3 { // "cat", "dog", "fly"
		t.Errorf("Expected 3 words of length 3, got %d", len(grouped[3]))
	}
	if len(grouped[8]) != 1 { // "elephant"
		t.Errorf("Expected 1 word of length 8, got %d", len(grouped[8]))
	}
	if len(grouped[9]) != 1 { // "butterfly"
		t.Errorf("Expected 1 word of length 9, got %d", len(grouped[9]))
	}
}
