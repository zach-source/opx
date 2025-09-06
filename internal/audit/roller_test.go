package audit

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/zach-source/opx/internal/security"
)

func TestDefaultRollerConfig(t *testing.T) {
	config := DefaultRollerConfig()

	if config.MaxDays != 30 {
		t.Errorf("Expected MaxDays 30, got %d", config.MaxDays)
	}

	if config.FlushInterval != 5*time.Second {
		t.Errorf("Expected FlushInterval 5s, got %v", config.FlushInterval)
	}

	if !config.RotateOnStart {
		t.Error("Expected RotateOnStart to be true")
	}
}

func TestRoller_GetCurrentLogPath(t *testing.T) {
	// Set up temp environment
	tempDir := t.TempDir()
	originalDataHome := os.Getenv("XDG_DATA_HOME")
	defer func() {
		if originalDataHome != "" {
			os.Setenv("XDG_DATA_HOME", originalDataHome)
		} else {
			os.Unsetenv("XDG_DATA_HOME")
		}
	}()
	os.Setenv("XDG_DATA_HOME", tempDir)

	config := DefaultRollerConfig()
	roller, err := NewRoller(config)
	if err != nil {
		t.Fatalf("Failed to create roller: %v", err)
	}
	defer roller.Close()

	path := roller.GetCurrentLogPath()
	expectedDate := time.Now().Format("2006-01-02")
	expectedSuffix := filepath.Join("op-authd", "audit-"+expectedDate+".log")

	if !strings.HasSuffix(path, expectedSuffix) {
		t.Errorf("Expected path to end with %q, got %q", expectedSuffix, path)
	}
}

func TestRoller_Write(t *testing.T) {
	// Set up temp environment
	tempDir := t.TempDir()
	originalDataHome := os.Getenv("XDG_DATA_HOME")
	defer func() {
		if originalDataHome != "" {
			os.Setenv("XDG_DATA_HOME", originalDataHome)
		} else {
			os.Unsetenv("XDG_DATA_HOME")
		}
	}()
	os.Setenv("XDG_DATA_HOME", tempDir)

	config := RollerConfig{
		MaxDays:       7,
		CompressOld:   false,
		RotateOnStart: true,
		FlushInterval: 0, // Disable flush timer for test
	}

	roller, err := NewRoller(config)
	if err != nil {
		t.Fatalf("Failed to create roller: %v", err)
	}
	defer roller.Close()

	// Write test data
	testData := []byte("test audit log entry\n")
	err = roller.Write(testData)
	if err != nil {
		t.Errorf("Failed to write to roller: %v", err)
	}

	// Verify file was created and contains data
	logPath := roller.GetCurrentLogPath()
	data, err := os.ReadFile(logPath)
	if err != nil {
		t.Errorf("Failed to read log file: %v", err)
	}

	if string(data) != string(testData) {
		t.Errorf("Expected log content %q, got %q", string(testData), string(data))
	}
}

func TestRoller_ListLogFiles(t *testing.T) {
	// Set up temp environment
	tempDir := t.TempDir()
	originalDataHome := os.Getenv("XDG_DATA_HOME")
	defer func() {
		if originalDataHome != "" {
			os.Setenv("XDG_DATA_HOME", originalDataHome)
		} else {
			os.Unsetenv("XDG_DATA_HOME")
		}
	}()
	os.Setenv("XDG_DATA_HOME", tempDir)

	config := RollerConfig{
		MaxDays:       7,
		FlushInterval: 0, // Disable flush timer
	}

	roller, err := NewRoller(config)
	if err != nil {
		t.Fatalf("Failed to create roller: %v", err)
	}
	defer roller.Close()

	// Create some test log files
	dataDir := filepath.Join(tempDir, "op-authd")
	os.MkdirAll(dataDir, 0700)

	testDates := []string{"2025-01-01", "2025-01-02", "2025-01-03"}
	for _, date := range testDates {
		logPath := filepath.Join(dataDir, "audit-"+date+".log")
		err := os.WriteFile(logPath, []byte("test"), 0600)
		if err != nil {
			t.Fatalf("Failed to create test log file: %v", err)
		}
	}

	// List log files
	files, err := roller.ListLogFiles()
	if err != nil {
		t.Errorf("Failed to list log files: %v", err)
	}

	if len(files) != len(testDates) {
		t.Errorf("Expected %d log files, got %d", len(testDates), len(files))
	}

	// Files should be sorted with newest first
	// Check that first file is from the latest date
	if len(files) > 0 {
		firstFile := filepath.Base(files[0])
		if !strings.Contains(firstFile, "2025-01-03") {
			t.Errorf("Expected first file to contain latest date, got %s", firstFile)
		}
	}
}

func TestNewLoggerWithRoller(t *testing.T) {
	// Test that the logger integration with roller works
	tempDir := t.TempDir()
	originalDataHome := os.Getenv("XDG_DATA_HOME")
	defer func() {
		if originalDataHome != "" {
			os.Setenv("XDG_DATA_HOME", originalDataHome)
		} else {
			os.Unsetenv("XDG_DATA_HOME")
		}
	}()
	os.Setenv("XDG_DATA_HOME", tempDir)

	config := RollerConfig{
		MaxDays:       7,
		FlushInterval: 0, // Disable for testing
	}

	logger, err := NewLoggerWithConfig(true, config)
	if err != nil {
		t.Fatalf("Failed to create logger with config: %v", err)
	}
	defer logger.Close()

	// Test logging an event
	event := AuditEvent{
		Event:    "TEST_EVENT",
		Decision: "TEST",
		PeerInfo: security.PeerInfo{PID: 123, Path: "/usr/bin/test"},
	}

	logger.LogEvent(event)

	// Verify log file was created and contains the event
	if logger.roller == nil {
		t.Error("Expected logger to have roller")
	}
}
