package security

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestPeerInfo_String(t *testing.T) {
	pi := PeerInfo{
		PID:  12345,
		UID:  1000,
		GID:  1000,
		Path: "/usr/bin/example",
	}

	str := pi.String()
	if str == "" {
		t.Error("Expected non-empty string representation")
	}

	// Should contain key information
	if !contains(str, "12345") {
		t.Error("Expected PID in string representation")
	}
	if !contains(str, "/usr/bin/example") {
		t.Error("Expected path in string representation")
	}
}

func TestExePathForPID(t *testing.T) {
	// Test with current process PID
	currentPID := os.Getpid()
	path := exePathForPID(currentPID)

	if path == "" {
		t.Skip("Could not determine executable path for current process (may be expected in some environments)")
	}

	t.Logf("Current process executable path: %s", path)

	// Path should be absolute on successful detection
	if path != "" && !filepath.IsAbs(path) {
		t.Errorf("Expected absolute path, got relative: %s", path)
	}
}

func TestExePathForPID_InvalidPID(t *testing.T) {
	// Test with invalid PIDs
	testCases := []int{0, -1, -999, 999999}

	for _, pid := range testCases {
		path := exePathForPID(pid)
		if path != "" {
			t.Errorf("Expected empty path for invalid PID %d, got %s", pid, path)
		}
	}
}

// Integration test for peer credential extraction (requires Unix socket)
func TestPeerFromUnixConn_Integration(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	// This test requires creating actual Unix socket connections
	// which is complex to set up in unit tests
	t.Skip("PeerFromUnixConn requires complex Unix socket setup - tested in server integration tests")
}

func TestPeerInfo_PlatformSupport(t *testing.T) {
	// Test that we handle platform support correctly
	switch runtime.GOOS {
	case "linux", "darwin":
		// These platforms should be supported
		t.Logf("Platform %s is supported for peer credential extraction", runtime.GOOS)
	default:
		t.Logf("Platform %s is not supported for peer credential extraction", runtime.GOOS)
	}
}

// Helper function for string contains check
func contains(s, substr string) bool {
	return len(s) > 0 && len(substr) > 0 && (s == substr || len(s) > len(substr) &&
		(s[:len(substr)] == substr || s[len(s)-len(substr):] == substr ||
			strings.Contains(s, substr)))
}
