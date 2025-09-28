package backend

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"
)

// Mock backend for testing server integration
type Mock struct {
	responses map[string]string
	errors    map[string]error
	calls     []string
	name      string
}

func NewMock(name string) *Mock {
	return &Mock{
		responses: make(map[string]string),
		errors:    make(map[string]error),
		calls:     []string{},
		name:      name,
	}
}

func (m *Mock) SetResponse(ref, value string) {
	m.responses[ref] = value
}

func (m *Mock) SetError(ref string, err error) {
	m.errors[ref] = err
}

func (m *Mock) GetCalls() []string {
	return append([]string{}, m.calls...)
}

func (m *Mock) ClearCalls() {
	m.calls = []string{}
}

func (m *Mock) Name() string {
	if m.name == "" {
		return "mock"
	}
	return m.name
}

func (m *Mock) ReadRef(ctx context.Context, ref string) (string, error) {
	return m.ReadRefWithFlags(ctx, ref, nil)
}

func (m *Mock) ReadRefWithFlags(ctx context.Context, ref string, flags []string) (string, error) {
	// Track call with flags
	call := ref
	if len(flags) > 0 {
		call += "|flags:" + strings.Join(flags, ",")
	}
	m.calls = append(m.calls, call)

	// Check for timeout
	select {
	case <-ctx.Done():
		return "", ctx.Err()
	default:
	}

	if err, ok := m.errors[ref]; ok {
		return "", err
	}
	if response, ok := m.responses[ref]; ok {
		return response, nil
	}
	return "", fmt.Errorf("ref not found: %s", ref)
}

// TestBackendInterface ensures both implementations satisfy the Backend interface
func TestBackendInterface(t *testing.T) {
	var _ Backend = &Fake{}
	var _ Backend = &OpCLI{}
	var _ Backend = NewMock("test")
}

// TestFake tests the Fake backend implementation
func TestFake(t *testing.T) {
	fake := &Fake{}
	ctx := context.Background()

	tests := []struct {
		name     string
		ref      string
		expected string
	}{
		{
			name:     "simple ref",
			ref:      "op://vault/item/field",
			expected: "fake_" + hashFirst8("op://vault/item/field"),
		},
		{
			name:     "different ref produces different hash",
			ref:      "op://vault/item/password",
			expected: "fake_" + hashFirst8("op://vault/item/password"),
		},
		{
			name:     "empty ref",
			ref:      "",
			expected: "fake_" + hashFirst8(""),
		},
		{
			name:     "same ref produces same hash",
			ref:      "op://vault/item/field",
			expected: "fake_" + hashFirst8("op://vault/item/field"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := fake.ReadRef(ctx, tt.ref)
			if err != nil {
				t.Errorf("Unexpected error: %v", err)
			}
			if result != tt.expected {
				t.Errorf("Expected %q, got %q", tt.expected, result)
			}
		})
	}
}

func TestFake_Name(t *testing.T) {
	fake := &Fake{}
	if name := fake.Name(); name != "fake" {
		t.Errorf("Expected name 'fake', got %q", name)
	}
}

func TestFake_Deterministic(t *testing.T) {
	fake := &Fake{}
	ctx := context.Background()
	ref := "op://vault/item/password"

	// Call multiple times and ensure consistent results
	var results []string
	for i := 0; i < 5; i++ {
		result, err := fake.ReadRef(ctx, ref)
		if err != nil {
			t.Fatalf("Unexpected error on iteration %d: %v", i, err)
		}
		results = append(results, result)
	}

	// All results should be identical
	for i, result := range results {
		if result != results[0] {
			t.Errorf("Result %d differs: %q vs %q", i, result, results[0])
		}
	}
}

func TestFake_ContextCancellation(t *testing.T) {
	fake := &Fake{}
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	// Should still work because Fake doesn't respect context cancellation
	_, err := fake.ReadRef(ctx, "op://vault/item/field")
	if err != nil {
		t.Errorf("Fake backend should not respect context cancellation, got error: %v", err)
	}
}

func TestFake_WithFlags(t *testing.T) {
	fake := &Fake{}
	ctx := context.Background()

	t.Run("no flags matches base result", func(t *testing.T) {
		baseResult, _ := fake.ReadRef(ctx, "op://vault/item/field")
		result, err := fake.ReadRefWithFlags(ctx, "op://vault/item/field", nil)
		if err != nil {
			t.Errorf("Unexpected error: %v", err)
		}
		if result != baseResult {
			t.Errorf("Expected same result without flags: %s vs %s", result, baseResult)
		}
	})

	t.Run("with account flag differs from base", func(t *testing.T) {
		baseResult, _ := fake.ReadRef(ctx, "op://vault/item/field")
		result, err := fake.ReadRefWithFlags(ctx, "op://vault/item/field", []string{"--account=ACC123"})
		if err != nil {
			t.Errorf("Unexpected error: %v", err)
		}
		if result == baseResult {
			t.Error("Expected different result with account flag")
		}
	})

	t.Run("multiple flags differs from base", func(t *testing.T) {
		baseResult, _ := fake.ReadRef(ctx, "op://vault/item/field")
		result, err := fake.ReadRefWithFlags(ctx, "op://vault/item/field", []string{"--account=ACC123", "--session=xyz"})
		if err != nil {
			t.Errorf("Unexpected error: %v", err)
		}
		if result == baseResult {
			t.Error("Expected different result with multiple flags")
		}
		if !strings.HasPrefix(result, "fake_") {
			t.Errorf("Expected result to start with 'fake_', got: %s", result)
		}
	})

	t.Run("same flags produce same result repeatedly", func(t *testing.T) {
		result1, _ := fake.ReadRefWithFlags(ctx, "op://vault/item/field", []string{"--account=ACC123"})
		result2, _ := fake.ReadRefWithFlags(ctx, "op://vault/item/field", []string{"--account=ACC123"})
		if result1 != result2 {
			t.Errorf("Expected same result with identical flags: %s vs %s", result1, result2)
		}
	})
}

func TestFake_AccountFlagExtraction(t *testing.T) {
	fake := &Fake{}
	ctx := context.Background()

	ref := "op://vault/item/field"

	result1, _ := fake.ReadRefWithFlags(ctx, ref, []string{"--account=ACC1"})
	result2, _ := fake.ReadRefWithFlags(ctx, ref, []string{"--account=ACC2"})
	result3, _ := fake.ReadRefWithFlags(ctx, ref, []string{"--account=ACC1"})

	if result1 == result2 {
		t.Error("Different account flags should produce different results")
	}

	if result1 != result3 {
		t.Error("Same account flag should produce same result")
	}
}

func TestMock_WithFlags(t *testing.T) {
	mock := NewMock("test-backend")
	ctx := context.Background()

	mock.SetResponse("op://vault/item/field", "secret-value")

	result, err := mock.ReadRefWithFlags(ctx, "op://vault/item/field", []string{"--account=ACC123"})
	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}
	if result != "secret-value" {
		t.Errorf("Expected 'secret-value', got %s", result)
	}

	calls := mock.GetCalls()
	if len(calls) != 1 {
		t.Fatalf("Expected 1 call, got %d", len(calls))
	}

	expectedCall := "op://vault/item/field|flags:--account=ACC123"
	if calls[0] != expectedCall {
		t.Errorf("Expected call %q, got %q", expectedCall, calls[0])
	}
}

// TestOpCLI tests the OpCLI backend implementation
func TestOpCLI_Name(t *testing.T) {
	opcli := &OpCLI{}
	if name := opcli.Name(); name != "opcli" {
		t.Errorf("Expected name 'opcli', got %q", name)
	}
}

func TestOpCLI_EmptyRef(t *testing.T) {
	opcli := &OpCLI{}
	ctx := context.Background()

	tests := []string{
		"",
		"   ",
		"\n\t  \n",
	}

	for _, ref := range tests {
		t.Run(fmt.Sprintf("empty_ref_%q", ref), func(t *testing.T) {
			_, err := opcli.ReadRef(ctx, ref)
			if err == nil {
				t.Error("Expected error for empty ref")
			}
			if !strings.Contains(err.Error(), "empty ref") {
				t.Errorf("Expected 'empty ref' error, got: %v", err)
			}
		})
	}
}

// TestOpCLI_CommandExecution tests the command execution without actually calling op
func TestOpCLI_CommandExecution(t *testing.T) {
	opcli := &OpCLI{}
	ctx := context.Background()

	_, err := opcli.ReadRef(ctx, "op://nonexistent-vault/item/field")
	if err == nil {
		t.Error("Expected error when op command fails")
	}

	errStr := err.Error()
	if !strings.Contains(errStr, "not signed in") &&
		!strings.Contains(errStr, "session validation") &&
		!strings.Contains(errStr, "op read failed") &&
		!strings.Contains(errStr, "executable file not found") {
		t.Errorf("Error should contain session/auth error or command error, got: %v", err)
	}
}

func TestOpCLI_ContextTimeout(t *testing.T) {
	opcli := &OpCLI{}

	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Nanosecond)
	defer cancel()

	time.Sleep(1 * time.Millisecond)

	_, err := opcli.ReadRef(ctx, "op://vault/item/field")
	if err == nil {
		t.Error("Expected timeout error")
	}

	errStr := err.Error()
	if !strings.Contains(errStr, "session validation timeout") &&
		!strings.Contains(errStr, "op read failed") {
		t.Errorf("Expected timeout error, got: %v", err)
	}
}

// TestWithTimeout tests the WithTimeout utility function
func TestWithTimeout(t *testing.T) {
	parent := context.Background()

	t.Run("positive duration", func(t *testing.T) {
		ctx, cancel := WithTimeout(parent, 1*time.Second)
		defer cancel()

		// Should have a deadline
		deadline, ok := ctx.Deadline()
		if !ok {
			t.Error("Context should have a deadline")
		}

		// Deadline should be in the future
		if time.Until(deadline) <= 0 {
			t.Error("Deadline should be in the future")
		}
	})

	t.Run("zero duration", func(t *testing.T) {
		ctx, cancel := WithTimeout(parent, 0)
		defer cancel()

		// Should return the parent context without timeout
		if ctx != parent {
			t.Error("Should return parent context for zero duration")
		}

		// Should not have a deadline
		if _, ok := ctx.Deadline(); ok {
			t.Error("Context should not have a deadline for zero duration")
		}
	})

	t.Run("negative duration", func(t *testing.T) {
		ctx, cancel := WithTimeout(parent, -1*time.Second)
		defer cancel()

		// Should return the parent context without timeout
		if ctx != parent {
			t.Error("Should return parent context for negative duration")
		}
	})
}

// TestMock tests our mock backend implementation
func TestMock(t *testing.T) {
	mock := NewMock("test-mock")
	ctx := context.Background()

	t.Run("name", func(t *testing.T) {
		if name := mock.Name(); name != "test-mock" {
			t.Errorf("Expected name 'test-mock', got %q", name)
		}
	})

	t.Run("default name", func(t *testing.T) {
		defaultMock := NewMock("")
		if name := defaultMock.Name(); name != "mock" {
			t.Errorf("Expected default name 'mock', got %q", name)
		}
	})

	t.Run("set and get response", func(t *testing.T) {
		mock.SetResponse("ref1", "value1")
		mock.SetResponse("ref2", "value2")

		result1, err1 := mock.ReadRef(ctx, "ref1")
		if err1 != nil {
			t.Errorf("Unexpected error for ref1: %v", err1)
		}
		if result1 != "value1" {
			t.Errorf("Expected 'value1', got %q", result1)
		}

		result2, err2 := mock.ReadRef(ctx, "ref2")
		if err2 != nil {
			t.Errorf("Unexpected error for ref2: %v", err2)
		}
		if result2 != "value2" {
			t.Errorf("Expected 'value2', got %q", result2)
		}
	})

	t.Run("set and get error", func(t *testing.T) {
		expectedErr := errors.New("test error")
		mock.SetError("error-ref", expectedErr)

		_, err := mock.ReadRef(ctx, "error-ref")
		if err != expectedErr {
			t.Errorf("Expected specific error, got: %v", err)
		}
	})

	t.Run("unknown ref", func(t *testing.T) {
		_, err := mock.ReadRef(ctx, "unknown-ref")
		if err == nil {
			t.Error("Expected error for unknown ref")
		}
		if !strings.Contains(err.Error(), "ref not found") {
			t.Errorf("Expected 'ref not found' error, got: %v", err)
		}
	})

	t.Run("call tracking", func(t *testing.T) {
		mock.ClearCalls()
		mock.SetResponse("track1", "value1")
		mock.SetResponse("track2", "value2")

		mock.ReadRef(ctx, "track1")
		mock.ReadRef(ctx, "track2")
		mock.ReadRef(ctx, "track1") // duplicate call

		calls := mock.GetCalls()
		expectedCalls := []string{"track1", "track2", "track1"}

		if len(calls) != len(expectedCalls) {
			t.Errorf("Expected %d calls, got %d", len(expectedCalls), len(calls))
		}

		for i, call := range calls {
			if call != expectedCalls[i] {
				t.Errorf("Call %d: expected %q, got %q", i, expectedCalls[i], call)
			}
		}
	})

	t.Run("context cancellation", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()

		mock.SetResponse("cancelled", "value")
		_, err := mock.ReadRef(ctx, "cancelled")
		if err == nil {
			t.Error("Expected context cancellation error")
		}
		if err != context.Canceled {
			t.Errorf("Expected context.Canceled, got: %v", err)
		}
	})
}

// Helper function to compute the first 8 bytes of SHA256 hash as hex
func hashFirst8(s string) string {
	sum := sha256.Sum256([]byte(s))
	return hex.EncodeToString(sum[:8])
}

// Benchmark tests for performance comparison
func BenchmarkFake_ReadRef(b *testing.B) {
	fake := &Fake{}
	ctx := context.Background()
	ref := "op://vault/item/password"

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = fake.ReadRef(ctx, ref)
	}
}

func BenchmarkMock_ReadRef(b *testing.B) {
	mock := NewMock("benchmark")
	mock.SetResponse("op://vault/item/password", "test-value")
	ctx := context.Background()
	ref := "op://vault/item/password"

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = mock.ReadRef(ctx, ref)
	}
}

// TestOpCLI_Integration is an integration test that can be enabled if op CLI is available
func TestOpCLI_Integration(t *testing.T) {
	// Skip by default since it requires op CLI to be installed and configured
	if os.Getenv("ENABLE_OP_INTEGRATION_TESTS") != "1" {
		t.Skip("Skipping op CLI integration test. Set ENABLE_OP_INTEGRATION_TESTS=1 to enable.")
	}

	// Check if op command is available
	if _, err := exec.LookPath("op"); err != nil {
		t.Skipf("op command not found: %v", err)
	}

	opcli := &OpCLI{}
	ctx := context.Background()

	// Test with a simple command that should work if op is configured
	// This is just a basic smoke test - but now --help will be rejected by validation
	_, err := opcli.ReadRef(ctx, "--help")
	// We expect this to fail due to validation (cannot start with dash)
	if err == nil {
		t.Error("Expected error for --help flag")
	}
	if !strings.Contains(err.Error(), "cannot start with dash") {
		t.Errorf("Expected validation error message, got: %v", err)
	}
}

func TestOpx_EndToEndIntegration(t *testing.T) {
	if os.Getenv("ENABLE_OP_INTEGRATION_TESTS") != "1" {
		t.Skip("Skipping end-to-end integration test. Set ENABLE_OP_INTEGRATION_TESTS=1 to enable.")
	}

	if _, err := exec.LookPath("op"); err != nil {
		t.Skipf("op command not found: %v", err)
	}

	ctx := context.Background()
	testValue := "opx-integration-test-value-" + fmt.Sprint(time.Now().Unix())
	testItemTitle := "opx-integration-test-item-" + fmt.Sprint(time.Now().Unix())
	accountID := os.Getenv("OP_ACCOUNT_ID")
	if accountID == "" {
		t.Skip("OP_ACCOUNT_ID not set, skipping multi-account test")
	}

	t.Logf("Creating test item with value: %s", testValue)
	createCmd := exec.CommandContext(ctx, "op", "item", "create",
		"--category=password",
		"--title="+testItemTitle,
		"--vault=Private",
		"--account="+accountID,
		"password="+testValue)
	if out, err := createCmd.CombinedOutput(); err != nil {
		t.Fatalf("Failed to create test item: %v\nOutput: %s", err, string(out))
	}

	defer func() {
		t.Logf("Cleaning up test item: %s", testItemTitle)
		delCmd := exec.Command("op", "item", "delete", testItemTitle, "--vault=Private", "--account="+accountID)
		_ = delCmd.Run()
	}()

	socketPath := "/tmp/opx-integration-test-" + fmt.Sprint(time.Now().Unix()) + ".sock"
	policyPath := "/tmp/opx-integration-policy-" + fmt.Sprint(time.Now().Unix()) + ".json"

	policy := `{"allow":[{"path":"` + os.Args[0] + `","refs":["op://*"],"require_signed":false}],"default_deny":false}`
	if err := os.WriteFile(policyPath, []byte(policy), 0600); err != nil {
		t.Fatalf("Failed to create policy file: %v", err)
	}
	defer os.Remove(policyPath)

	t.Logf("Starting opx-authd with socket: %s", socketPath)
	daemonCmd := exec.CommandContext(ctx, "../../bin/opx-authd",
		"--sock="+socketPath,
		"--backend=opcli",
		"--policy="+policyPath)
	if err := daemonCmd.Start(); err != nil {
		t.Fatalf("Failed to start daemon: %v", err)
	}
	defer func() {
		_ = daemonCmd.Process.Kill()
		_ = os.Remove(socketPath)
	}()

	time.Sleep(2 * time.Second)

	os.Setenv("OPX_SOCKET_PATH", socketPath)
	os.Setenv("OPX_AUTOSTART", "0")
	defer func() {
		os.Unsetenv("OPX_SOCKET_PATH")
		os.Unsetenv("OPX_AUTOSTART")
	}()

	refPath := "op://Private/" + testItemTitle + "/password"

	t.Run("flag after command", func(t *testing.T) {
		readCmd := exec.CommandContext(ctx, "../../bin/opx", "read", refPath, "--account="+accountID)
		out, err := readCmd.CombinedOutput()
		if err != nil {
			t.Fatalf("opx read failed: %v\nOutput: %s", err, string(out))
		}
		result := strings.TrimSpace(string(out))
		if result != testValue {
			t.Errorf("Expected %q, got %q", testValue, result)
		}
	})

	t.Run("flag before command", func(t *testing.T) {
		readCmd := exec.CommandContext(ctx, "../../bin/opx", "--account="+accountID, "read", refPath)
		out, err := readCmd.CombinedOutput()
		if err != nil {
			t.Fatalf("opx read failed: %v\nOutput: %s", err, string(out))
		}
		result := strings.TrimSpace(string(out))
		if result != testValue {
			t.Errorf("Expected %q, got %q", testValue, result)
		}
	})

	t.Logf("✅ End-to-end integration test passed")
}

// TestOpCLI_ValidationSecurity tests the new security validations
func TestOpCLI_ValidationSecurity(t *testing.T) {
	opcli := &OpCLI{}
	ctx := context.Background()

	tests := []struct {
		name        string
		ref         string
		flags       []string
		wantErrText string
	}{
		{
			name:        "ref starting with dash",
			ref:         "--help",
			flags:       nil,
			wantErrText: "cannot start with dash",
		},
		{
			name:        "ref not starting with op://",
			ref:         "not-op-url",
			flags:       nil,
			wantErrText: "must start with op://",
		},
		{
			name:        "flag without dash",
			ref:         "op://vault/item/field",
			flags:       []string{"account=test"},
			wantErrText: "must start with dash",
		},
		{
			name:        "flag with command injection",
			ref:         "op://vault/item/field",
			flags:       []string{"--account=test; rm -rf /"},
			wantErrText: "unsafe characters",
		},
		{
			name:        "flag with pipe injection",
			ref:         "op://vault/item/field",
			flags:       []string{"--account=test|whoami"},
			wantErrText: "unsafe characters",
		},
		{
			name:        "flag with backtick injection",
			ref:         "op://vault/item/field",
			flags:       []string{"--account=`whoami`"},
			wantErrText: "unsafe characters",
		},
		{
			name:        "valid ref and flags should pass validation",
			ref:         "op://vault/item/field",
			flags:       []string{"--account=ABCD1234", "--session=xyz"},
			wantErrText: "", // Should pass validation and fail only on op command execution
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := opcli.ReadRefWithFlags(ctx, tt.ref, tt.flags)

			if tt.wantErrText == "" {
				// Should pass validation but fail on command execution (op not found)
				if err == nil {
					t.Error("Expected command execution error")
				}
				// Should not be a validation error
				if strings.Contains(err.Error(), "invalid") {
					t.Errorf("Should pass validation but got validation error: %v", err)
				}
			} else {
				// Should fail validation
				if err == nil {
					t.Errorf("Expected validation error containing %q", tt.wantErrText)
				} else if !strings.Contains(err.Error(), tt.wantErrText) {
					t.Errorf("Expected error containing %q, got: %v", tt.wantErrText, err)
				}
			}
		})
	}
}
