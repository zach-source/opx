package backend

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/zach-source/opx/internal/session"
)

// mockBackend implements Backend interface for testing
type mockBackend struct {
	name          string
	readRefError  error
	readRefResult string
}

func (m *mockBackend) Name() string {
	return m.name
}

func (m *mockBackend) ReadRef(ctx context.Context, ref string) (string, error) {
	return m.ReadRefWithFlags(ctx, ref, nil)
}

func (m *mockBackend) ReadRefWithFlags(ctx context.Context, ref string, flags []string) (string, error) {
	if m.readRefError != nil {
		return "", m.readRefError
	}
	return m.readRefResult, nil
}

func TestNewSessionAwareBackend(t *testing.T) {
	backend := &mockBackend{name: "test"}
	sessionManager := session.NewManager(session.DefaultConfig())

	sessionAware := NewSessionAwareBackend(backend, sessionManager)

	if sessionAware.backend != backend {
		t.Error("Expected backend to be set")
	}
	if sessionAware.session != sessionManager {
		t.Error("Expected session manager to be set")
	}
}

func TestSessionAwareBackend_Name(t *testing.T) {
	backend := &mockBackend{name: "test"}
	sessionManager := session.NewManager(session.DefaultConfig())
	sessionAware := NewSessionAwareBackend(backend, sessionManager)

	expected := "test+session"
	if got := sessionAware.Name(); got != expected {
		t.Errorf("Expected name %q, got %q", expected, got)
	}
}

func TestSessionAwareBackend_ReadRef_SessionValidation(t *testing.T) {
	ctx := context.Background()

	t.Run("authenticated session succeeds", func(t *testing.T) {
		backend := &mockBackend{
			name:          "test",
			readRefResult: "secret-value",
		}
		sessionManager := session.NewManager(session.DefaultConfig())
		sessionManager.MarkAuthenticated()

		sessionAware := NewSessionAwareBackend(backend, sessionManager)

		result, err := sessionAware.ReadRef(ctx, "op://vault/item/field")
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
		if result != "secret-value" {
			t.Errorf("Expected 'secret-value', got %q", result)
		}
	})

	t.Run("locked session fails without unlock callback", func(t *testing.T) {
		backend := &mockBackend{
			name:          "test",
			readRefResult: "secret-value",
		}
		sessionManager := session.NewManager(session.DefaultConfig())
		sessionManager.MarkLocked()

		sessionAware := NewSessionAwareBackend(backend, sessionManager)

		_, err := sessionAware.ReadRef(ctx, "op://vault/item/field")
		if err == nil {
			t.Error("Expected error for locked session")
		}
	})

	t.Run("locked session succeeds with unlock callback", func(t *testing.T) {
		backend := &mockBackend{
			name:          "test",
			readRefResult: "secret-value",
		}
		sessionManager := session.NewManager(session.DefaultConfig())
		sessionManager.SetCallbacks(nil, func(ctx context.Context) error {
			return nil // Successful unlock
		})
		sessionManager.MarkLocked()

		sessionAware := NewSessionAwareBackend(backend, sessionManager)

		result, err := sessionAware.ReadRef(ctx, "op://vault/item/field")
		if err != nil {
			t.Errorf("Expected no error with successful unlock, got %v", err)
		}
		if result != "secret-value" {
			t.Errorf("Expected 'secret-value', got %q", result)
		}
	})

	t.Run("backend error is propagated", func(t *testing.T) {
		expectedError := errors.New("backend failure")
		backend := &mockBackend{
			name:         "test",
			readRefError: expectedError,
		}
		sessionManager := session.NewManager(session.DefaultConfig())
		sessionManager.MarkAuthenticated()

		sessionAware := NewSessionAwareBackend(backend, sessionManager)

		_, err := sessionAware.ReadRef(ctx, "op://vault/item/field")
		if err == nil {
			t.Error("Expected backend error to be propagated")
		}
		if !errors.Is(err, expectedError) {
			t.Errorf("Expected backend error, got %v", err)
		}
	})
}

func TestSessionAwareBackend_ReadRefWithFlags(t *testing.T) {
	ctx := context.Background()
	backend := &mockBackend{
		name:          "test",
		readRefResult: "secret-value",
	}
	sessionManager := session.NewManager(session.DefaultConfig())
	sessionManager.MarkAuthenticated()

	sessionAware := NewSessionAwareBackend(backend, sessionManager)

	result, err := sessionAware.ReadRefWithFlags(ctx, "op://vault/item/field", []string{"--account", "test"})
	if err != nil {
		t.Errorf("Expected no error, got %v", err)
	}
	if result != "secret-value" {
		t.Errorf("Expected 'secret-value', got %q", result)
	}
}

func TestSessionAwareBackend_ActivityTracking(t *testing.T) {
	ctx := context.Background()
	backend := &mockBackend{
		name:          "test",
		readRefResult: "secret-value",
	}
	sessionManager := session.NewManager(session.DefaultConfig())
	sessionManager.MarkAuthenticated()

	sessionAware := NewSessionAwareBackend(backend, sessionManager)

	// Get initial activity time
	initialInfo := sessionManager.GetInfo()
	initialActivity := initialInfo.LastActivity

	// Sleep to ensure different timestamp
	time.Sleep(10 * time.Millisecond)

	// Perform read operation
	_, err := sessionAware.ReadRef(ctx, "op://vault/item/field")
	if err != nil {
		t.Errorf("Expected no error, got %v", err)
	}

	// Check that activity was updated
	updatedInfo := sessionManager.GetInfo()
	if !updatedInfo.LastActivity.After(initialActivity) {
		t.Error("Expected activity timestamp to be updated after successful read")
	}
}

func TestSessionAwareBackend_NoActivityOnError(t *testing.T) {
	ctx := context.Background()
	expectedError := errors.New("backend failure")
	backend := &mockBackend{
		name:         "test",
		readRefError: expectedError,
	}
	sessionManager := session.NewManager(session.DefaultConfig())
	sessionManager.MarkAuthenticated()

	sessionAware := NewSessionAwareBackend(backend, sessionManager)

	// Get initial activity time
	initialInfo := sessionManager.GetInfo()
	initialActivity := initialInfo.LastActivity

	// Sleep to ensure different timestamp would be noticeable
	time.Sleep(10 * time.Millisecond)

	// Perform failed read operation
	_, err := sessionAware.ReadRef(ctx, "op://vault/item/field")
	if err == nil {
		t.Error("Expected error from backend")
	}

	// Check that activity was NOT updated on error
	updatedInfo := sessionManager.GetInfo()
	if updatedInfo.LastActivity.After(initialActivity) {
		t.Error("Activity should not be updated on backend error")
	}
}

// Integration test for ValidateCurrentSession (skipped by default since it requires op CLI)
func TestValidateCurrentSession_Integration(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// This test will pass if op CLI is installed and authenticated, fail otherwise
	// We don't assert specific behavior since it depends on external state
	err := ValidateCurrentSession(ctx)
	t.Logf("ValidateCurrentSession result: %v", err)
}

// Integration test for ClearCLISession (skipped by default since it affects real session)
func TestClearCLISession_Integration(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	// This test clears the actual CLI session, so we only run it in integration mode
	err := ClearCLISession()
	if err != nil {
		t.Errorf("ClearCLISession should not return error even if signout fails, got %v", err)
	}
}

func TestNewSessionAwareOpCLI(t *testing.T) {
	sessionManager := session.NewManager(session.DefaultConfig())

	backend := NewSessionAwareOpCLI(sessionManager)

	if backend.Name() != "opcli+session" {
		t.Errorf("Expected name 'opcli+session', got %q", backend.Name())
	}

	// Verify it's a SessionAwareBackend wrapping OpCLI
	sessionAware, ok := backend.(*SessionAwareBackend)
	if !ok {
		t.Error("Expected SessionAwareBackend type")
	}

	if sessionAware.backend.(OpCLI).Name() != "opcli" {
		t.Error("Expected wrapped backend to be OpCLI")
	}
}

func TestNewSessionAwareFake(t *testing.T) {
	sessionManager := session.NewManager(session.DefaultConfig())

	backend := NewSessionAwareFake(sessionManager)

	if backend.Name() != "fake+session" {
		t.Errorf("Expected name 'fake+session', got %q", backend.Name())
	}

	// Verify it's a SessionAwareBackend wrapping Fake
	sessionAware, ok := backend.(*SessionAwareBackend)
	if !ok {
		t.Error("Expected SessionAwareBackend type")
	}

	if sessionAware.backend.(Fake).Name() != "fake" {
		t.Error("Expected wrapped backend to be Fake")
	}
}
