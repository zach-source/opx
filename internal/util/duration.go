package util

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// ParseDuration parses a duration string with support for days, weeks, months, years
// Supports: 1h, 30m, 45s, 2d, 1w, 3M, 1y, or combinations like "1d12h30m"
func ParseDuration(s string) (time.Duration, error) {
	// First try standard Go parsing for simple cases
	if d, err := time.ParseDuration(s); err == nil {
		return d, nil
	}

	// Enhanced parsing for d/w/M/y units
	return parseExtendedDuration(s)
}

// parseExtendedDuration handles extended duration formats
func parseExtendedDuration(s string) (time.Duration, error) {
	// Regex to match duration components (including negative numbers)
	re := regexp.MustCompile(`(-?\d+(?:\.\d+)?)\s*([a-zA-Z]+)`)
	matches := re.FindAllStringSubmatch(s, -1)

	if len(matches) == 0 {
		return 0, fmt.Errorf("invalid duration format: %s", s)
	}

	var totalDuration time.Duration

	for _, match := range matches {
		if len(match) != 3 {
			continue
		}

		valueStr := match[1]
		unit := match[2] // Preserve case for M/m distinction

		value, err := strconv.ParseFloat(valueStr, 64)
		if err != nil {
			return 0, fmt.Errorf("invalid number in duration: %s", valueStr)
		}

		// Reject negative values
		if value < 0 {
			return 0, fmt.Errorf("duration must be positive: %s", valueStr)
		}

		var unitDuration time.Duration
		switch unit {
		case "ns", "nanosecond", "nanoseconds":
			unitDuration = time.Nanosecond
		case "us", "μs", "microsecond", "microseconds":
			unitDuration = time.Microsecond
		case "ms", "millisecond", "milliseconds":
			unitDuration = time.Millisecond
		case "s", "sec", "second", "seconds":
			unitDuration = time.Second
		case "m", "min", "minute", "minutes":
			unitDuration = time.Minute
		case "h", "hr", "hour", "hours":
			unitDuration = time.Hour
		case "d", "day", "days":
			unitDuration = 24 * time.Hour
		case "w", "week", "weeks":
			unitDuration = 7 * 24 * time.Hour
		case "M", "month", "months":
			unitDuration = 30 * 24 * time.Hour // Approximate
		case "y", "year", "years":
			unitDuration = 365 * 24 * time.Hour // Approximate
		default:
			return 0, fmt.Errorf("unknown duration unit: %s", unit)
		}

		totalDuration += time.Duration(value * float64(unitDuration))
	}

	if totalDuration == 0 {
		return 0, fmt.Errorf("duration must be positive: %s", s)
	}

	return totalDuration, nil
}

// FormatDuration formats a duration with appropriate units
func FormatDuration(d time.Duration) string {
	if d == 0 {
		return "0s"
	}

	// Convert to seconds for easier calculation
	totalSeconds := int64(d.Seconds())

	// Calculate components
	years := totalSeconds / (365 * 24 * 3600)
	totalSeconds %= (365 * 24 * 3600)

	months := totalSeconds / (30 * 24 * 3600)
	totalSeconds %= (30 * 24 * 3600)

	weeks := totalSeconds / (7 * 24 * 3600)
	totalSeconds %= (7 * 24 * 3600)

	days := totalSeconds / (24 * 3600)
	totalSeconds %= (24 * 3600)

	hours := totalSeconds / 3600
	totalSeconds %= 3600

	minutes := totalSeconds / 60
	seconds := totalSeconds % 60

	var parts []string

	if years > 0 {
		parts = append(parts, fmt.Sprintf("%dy", years))
	}
	if months > 0 {
		parts = append(parts, fmt.Sprintf("%dM", months))
	}
	if weeks > 0 {
		parts = append(parts, fmt.Sprintf("%dw", weeks))
	}
	if days > 0 {
		parts = append(parts, fmt.Sprintf("%dd", days))
	}
	if hours > 0 {
		parts = append(parts, fmt.Sprintf("%dh", hours))
	}
	if minutes > 0 {
		parts = append(parts, fmt.Sprintf("%dm", minutes))
	}
	if seconds > 0 || len(parts) == 0 {
		parts = append(parts, fmt.Sprintf("%ds", seconds))
	}

	return strings.Join(parts, "")
}

// ValidateDuration checks if a duration string is valid
func ValidateDuration(s string) error {
	_, err := ParseDuration(s)
	return err
}
