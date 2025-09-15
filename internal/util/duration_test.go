package util

import (
	"testing"
	"time"
)

func TestParseDuration(t *testing.T) {
	tests := []struct {
		input    string
		expected time.Duration
		hasError bool
	}{
		// Standard Go durations should still work
		{"1h", time.Hour, false},
		{"30m", 30 * time.Minute, false},
		{"45s", 45 * time.Second, false},

		// Enhanced formats
		{"1d", 24 * time.Hour, false},
		{"2d", 48 * time.Hour, false},
		{"1w", 7 * 24 * time.Hour, false},
		{"2w", 14 * 24 * time.Hour, false},
		{"1M", 30 * 24 * time.Hour, false},
		{"1y", 365 * 24 * time.Hour, false},

		// Combined formats
		{"1d12h", 36 * time.Hour, false},
		{"1w2d", (7 + 2) * 24 * time.Hour, false},

		// Invalid formats
		{"", 0, true},
		{"invalid", 0, true},
		{"1x", 0, true},
		{"-1d", 0, true},
	}

	for _, test := range tests {
		t.Run(test.input, func(t *testing.T) {
			result, err := ParseDuration(test.input)

			if test.hasError {
				if err == nil {
					t.Errorf("Expected error for input %q but got none", test.input)
				}
				return
			}

			if err != nil {
				t.Errorf("Unexpected error for input %q: %v", test.input, err)
				return
			}

			if result != test.expected {
				t.Errorf("For input %q: expected %v, got %v", test.input, test.expected, result)
			}
		})
	}
}

func TestFormatDuration(t *testing.T) {
	tests := []struct {
		input    time.Duration
		expected string
	}{
		{0, "0s"},
		{time.Second, "1s"},
		{time.Minute, "1m"},
		{time.Hour, "1h"},
		{24 * time.Hour, "1d"},
		{7 * 24 * time.Hour, "1w"},
		{36 * time.Hour, "1d12h"},
		{90 * time.Minute, "1h30m"},
	}

	for _, test := range tests {
		t.Run(test.expected, func(t *testing.T) {
			result := FormatDuration(test.input)
			if result != test.expected {
				t.Errorf("For input %v: expected %q, got %q", test.input, test.expected, result)
			}
		})
	}
}

func TestValidateDuration(t *testing.T) {
	validDurations := []string{"1h", "2d", "1w", "1M", "1y", "1d12h30m"}
	invalidDurations := []string{"", "invalid", "1x", "-1d"}

	for _, valid := range validDurations {
		if err := ValidateDuration(valid); err != nil {
			t.Errorf("Expected %q to be valid, got error: %v", valid, err)
		}
	}

	for _, invalid := range invalidDurations {
		if err := ValidateDuration(invalid); err == nil {
			t.Errorf("Expected %q to be invalid, but got no error", invalid)
		}
	}
}
