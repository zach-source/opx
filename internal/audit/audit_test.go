package audit

import (
	"bytes"
	"encoding/json"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/zach-source/opx/internal/security"
)

func TestAuditEvent_JSON(t *testing.T) {
	event := AuditEvent{
		Timestamp: time.Date(2025, 1, 15, 12, 30, 45, 0, time.UTC),
		Event:     "ACCESS_DECISION",
		PeerInfo:  security.PeerInfo{PID: 12345, ExecutablePath: "/usr/bin/test"},
		Reference: "op://vault/item/field",
		Decision:  "ALLOW",
		Details:   map[string]string{"reason": "policy match"},
	}

	data, err := json.Marshal(event)
	if err != nil {
		t.Fatalf("Failed to marshal audit event: %v", err)
	}

	var decoded AuditEvent
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("Failed to unmarshal audit event: %v", err)
	}

	if decoded.Event != event.Event {
		t.Errorf("Expected event %q, got %q", event.Event, decoded.Event)
	}

	if decoded.Decision != event.Decision {
		t.Errorf("Expected decision %q, got %q", event.Decision, decoded.Decision)
	}

	if decoded.Reference != event.Reference {
		t.Errorf("Expected reference %q, got %q", event.Reference, decoded.Reference)
	}
}

func TestLogger_LogEvent(t *testing.T) {
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
		FlushInterval: 0, // Disable for testing
	}

	logger, err := NewLoggerWithConfig(true, config)
	if err != nil {
		t.Fatalf("Failed to create logger: %v", err)
	}
	defer logger.Close()

	// Create test event
	event := AuditEvent{
		Event:     "TEST_EVENT",
		Decision:  "SUCCESS",
		PeerInfo:  security.PeerInfo{PID: 123, ExecutablePath: "/usr/bin/test"},
		Reference: "test://reference",
		Details:   map[string]string{"test": "value"},
	}

	// Log the event
	logger.LogEvent(event)

	// Verify log file was created and contains signed event
	logPath := logger.roller.GetCurrentLogPath()
	data, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("Failed to read log file: %v", err)
	}

	// Should contain a SecureAuditEvent (with HMAC signature)
	var secureEvent SecureAuditEvent
	if err := json.Unmarshal(data, &secureEvent); err != nil {
		t.Fatalf("Failed to parse secure audit event: %v", err)
	}

	if secureEvent.Event.Event != "TEST_EVENT" {
		t.Errorf("Expected event TEST_EVENT, got %q", secureEvent.Event.Event)
	}

	if secureEvent.Signature == "" {
		t.Error("Expected HMAC signature to be present")
	}

	if secureEvent.KeyID == "" {
		t.Error("Expected key ID to be present")
	}

	if secureEvent.Counter == 0 {
		t.Error("Expected counter to be non-zero")
	}
}

func TestLogger_LogAccessDecision(t *testing.T) {
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
		FlushInterval: 0,
	}

	logger, err := NewLoggerWithConfig(true, config)
	if err != nil {
		t.Fatalf("Failed to create logger: %v", err)
	}
	defer logger.Close()

	peerInfo := security.PeerInfo{PID: 456, ExecutablePath: "/usr/bin/kubectl"}
	reference := "op://Production/k8s/token"

	// Test both allow and deny decisions
	logger.LogAccessDecision(peerInfo, reference, true, "/config/policy.json", map[string]string{"rule": "match"})
	logger.LogAccessDecision(peerInfo, reference, false, "/config/policy.json", map[string]string{"rule": "nomatch"})

	// Verify both events were logged
	logPath := logger.roller.GetCurrentLogPath()
	data, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("Failed to read log file: %v", err)
	}

	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	if len(lines) != 2 {
		t.Errorf("Expected 2 log lines, got %d", len(lines))
	}

	// Parse first event (ALLOW)
	var allowEvent SecureAuditEvent
	if err := json.Unmarshal([]byte(lines[0]), &allowEvent); err != nil {
		t.Fatalf("Failed to parse allow event: %v", err)
	}

	if allowEvent.Event.Decision != "ALLOW" {
		t.Errorf("Expected ALLOW decision, got %q", allowEvent.Event.Decision)
	}

	// Parse second event (DENY)
	var denyEvent SecureAuditEvent
	if err := json.Unmarshal([]byte(lines[1]), &denyEvent); err != nil {
		t.Fatalf("Failed to parse deny event: %v", err)
	}

	if denyEvent.Event.Decision != "DENY" {
		t.Errorf("Expected DENY decision, got %q", denyEvent.Event.Decision)
	}
}

func TestIntegrityManager_SignAndVerify(t *testing.T) {
	// Set up temp environment for keyring fallback
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

	manager, err := NewIntegrityManager()
	if err != nil {
		t.Fatalf("Failed to create integrity manager: %v", err)
	}

	event := AuditEvent{
		Event:     "TEST_SIGNING",
		Decision:  "TEST",
		PeerInfo:  security.PeerInfo{PID: 789, ExecutablePath: "/usr/bin/test"},
		Reference: "vault://secret/test",
	}

	// Sign the event
	secureEvent := manager.SignEvent(event)

	// Verify basic structure
	if secureEvent.Signature == "" {
		t.Error("Expected signature to be generated")
	}

	if secureEvent.Counter == 0 {
		t.Error("Expected counter to be incremented")
	}

	if secureEvent.KeyID == "" {
		t.Error("Expected key ID to be set")
	}

	// Verify the signature
	valid, err := manager.VerifyEvent(secureEvent)
	if err != nil {
		t.Fatalf("Failed to verify event: %v", err)
	}

	if !valid {
		t.Error("Expected event signature to be valid")
	}

	// Test signature tampering detection
	secureEvent.Event.Decision = "TAMPERED"
	valid, err = manager.VerifyEvent(secureEvent)
	if err != nil {
		t.Fatalf("Verification should not error on tampered event: %v", err)
	}

	if valid {
		t.Error("Expected tampered event to fail verification")
	}
}

func TestIntegrityManager_CounterIncrement(t *testing.T) {
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

	manager, err := NewIntegrityManager()
	if err != nil {
		t.Fatalf("Failed to create integrity manager: %v", err)
	}

	event1 := AuditEvent{Event: "FIRST", Decision: "TEST"}
	event2 := AuditEvent{Event: "SECOND", Decision: "TEST"}

	secure1 := manager.SignEvent(event1)
	secure2 := manager.SignEvent(event2)

	if secure1.Counter >= secure2.Counter {
		t.Errorf("Expected counter to increment: %d >= %d", secure1.Counter, secure2.Counter)
	}

	if secure2.Counter != secure1.Counter+1 {
		t.Errorf("Expected consecutive counters: %d, %d", secure1.Counter, secure2.Counter)
	}
}

func TestAuditLogRotation(t *testing.T) {
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
		MaxDays:       3,
		FlushInterval: 0,
	}

	logger, err := NewLoggerWithConfig(true, config)
	if err != nil {
		t.Fatalf("Failed to create logger: %v", err)
	}
	defer logger.Close()

	// Log an event
	event := AuditEvent{
		Event:    "ROTATION_TEST",
		Decision: "SUCCESS",
		PeerInfo: security.PeerInfo{PID: 999, ExecutablePath: "/usr/bin/rotation-test"},
	}

	logger.LogEvent(event)

	// Check that log file was created with today's date
	expectedDate := time.Now().Format("2006-01-02")
	logPath := logger.roller.GetCurrentLogPath()

	if !strings.Contains(logPath, expectedDate) {
		t.Errorf("Expected log path to contain today's date %s, got %s", expectedDate, logPath)
	}

	// Verify file exists and contains data
	if _, err := os.Stat(logPath); os.IsNotExist(err) {
		t.Error("Expected log file to exist")
	}
}

func TestDisabledLogger(t *testing.T) {
	logger, err := NewLogger(false)
	if err != nil {
		t.Fatalf("Failed to create disabled logger: %v", err)
	}
	defer logger.Close()

	if logger.enabled {
		t.Error("Expected logger to be disabled")
	}

	if logger.roller != nil {
		t.Error("Expected disabled logger to have no roller")
	}

	// Logging should be no-op for disabled logger
	event := AuditEvent{Event: "TEST", Decision: "TEST"}
	logger.LogEvent(event) // Should not crash
}

// Benchmark tests for performance validation
func BenchmarkLogger_LogEvent(b *testing.B) {
	tempDir := b.TempDir()
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
		FlushInterval: 0, // Disable for benchmarking
	}

	logger, err := NewLoggerWithConfig(true, config)
	if err != nil {
		b.Fatalf("Failed to create logger: %v", err)
	}
	defer logger.Close()

	event := AuditEvent{
		Event:     "BENCHMARK_EVENT",
		Decision:  "ALLOW",
		PeerInfo:  security.PeerInfo{PID: 123, ExecutablePath: "/usr/bin/benchmark"},
		Reference: "op://vault/item/field",
		Details:   map[string]string{"test": "benchmark"},
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		logger.LogEvent(event)
	}
}

func BenchmarkIntegrityManager_SignEvent(b *testing.B) {
	tempDir := b.TempDir()
	originalDataHome := os.Getenv("XDG_DATA_HOME")
	defer func() {
		if originalDataHome != "" {
			os.Setenv("XDG_DATA_HOME", originalDataHome)
		} else {
			os.Unsetenv("XDG_DATA_HOME")
		}
	}()
	os.Setenv("XDG_DATA_HOME", tempDir)

	manager, err := NewIntegrityManager()
	if err != nil {
		b.Fatalf("Failed to create integrity manager: %v", err)
	}

	event := AuditEvent{
		Event:     "BENCHMARK_SIGNING",
		Decision:  "TEST",
		PeerInfo:  security.PeerInfo{PID: 123, ExecutablePath: "/usr/bin/benchmark"},
		Reference: "benchmark://reference",
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		manager.SignEvent(event)
	}
}

// The invariant that broke on hosts without a keyring: two calls must hand back
// the same key. When only the keyring was read, the unread file fallback meant a
// fresh random key every call and no event could verify against its own signature.
func TestEnsureHMACKey_StableAcrossCalls(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", t.TempDir())

	key1, id1, err := ensureHMACKey()
	if err != nil {
		t.Fatalf("first ensureHMACKey: %v", err)
	}

	key2, id2, err := ensureHMACKey()
	if err != nil {
		t.Fatalf("second ensureHMACKey: %v", err)
	}

	if id1 != id2 {
		t.Errorf("key ID changed between calls: %q then %q", id1, id2)
	}
	if !bytes.Equal(key1, key2) {
		t.Error("key material changed between calls")
	}
}
