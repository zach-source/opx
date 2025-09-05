package session

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"
)

func TestNewManager(t *testing.T) {
	t.Run("with config", func(t *testing.T) {
		config := &Config{
			SessionIdleTimeout: 30 * time.Minute,
			EnableSessionLock:  true,
		}

		manager := NewManager(config)

		if manager.config != config {
			t.Error("Expected config to be set")
		}
		if manager.state != SessionUnknown {
			t.Errorf("Expected initial state to be Unknown, got %v", manager.state)
		}
	})

	t.Run("with nil config", func(t *testing.T) {
		manager := NewManager(nil)

		if manager.config == nil {
			t.Error("Expected default config to be set")
		}
		if manager.config.SessionIdleTimeout != DefaultIdleTimeout {
			t.Errorf("Expected default timeout %v, got %v", DefaultIdleTimeout, manager.config.SessionIdleTimeout)
		}
	})
}

func TestManager_UpdateActivity(t *testing.T) {
	manager := NewManager(DefaultConfig())

	// Initially unknown state - should not update activity
	initialTime := manager.lastActivity
	manager.UpdateActivity()
	if !manager.lastActivity.Equal(initialTime) {
		t.Error("Activity should not be updated when session is not authenticated")
	}

	// Mark as authenticated and update activity
	manager.MarkAuthenticated()
	time.Sleep(10 * time.Millisecond) // Ensure different timestamp
	manager.UpdateActivity()

	if manager.lastActivity.Equal(initialTime) {
		t.Error("Activity should be updated when session is authenticated")
	}
}

func TestManager_MarkLocked(t *testing.T) {
	manager := NewManager(DefaultConfig())
	lockCallbackCalled := false

	manager.SetCallbacks(func() error {
		lockCallbackCalled = true
		return nil
	}, nil)

	manager.MarkLocked()

	if manager.state != SessionLocked {
		t.Errorf("Expected state to be Locked, got %v", manager.state)
	}

	if manager.lockedAt.IsZero() {
		t.Error("Expected lockedAt to be set")
	}

	if !lockCallbackCalled {
		t.Error("Expected lock callback to be called")
	}
}

func TestManager_MarkAuthenticated(t *testing.T) {
	manager := NewManager(DefaultConfig())
	manager.MarkLocked() // Start in locked state

	manager.MarkAuthenticated()

	if manager.state != SessionAuthenticated {
		t.Errorf("Expected state to be Authenticated, got %v", manager.state)
	}

	if manager.lockedAt != (time.Time{}) {
		t.Error("Expected lockedAt to be cleared")
	}
}

func TestManager_ValidateSession(t *testing.T) {
	ctx := context.Background()

	t.Run("authenticated session", func(t *testing.T) {
		manager := NewManager(DefaultConfig())
		manager.MarkAuthenticated()

		err := manager.ValidateSession(ctx)
		if err != nil {
			t.Errorf("Expected no error for authenticated session, got %v", err)
		}
	})

	t.Run("locked session with unlock callback", func(t *testing.T) {
		manager := NewManager(DefaultConfig())
		unlockCalled := false

		manager.SetCallbacks(nil, func(ctx context.Context) error {
			unlockCalled = true
			return nil
		})

		manager.MarkLocked()

		err := manager.ValidateSession(ctx)
		if err != nil {
			t.Errorf("Expected no error when unlock succeeds, got %v", err)
		}

		if !unlockCalled {
			t.Error("Expected unlock callback to be called")
		}

		if manager.state != SessionAuthenticated {
			t.Errorf("Expected state to be Authenticated after unlock, got %v", manager.state)
		}
	})

	t.Run("locked session with failing unlock callback", func(t *testing.T) {
		manager := NewManager(DefaultConfig())
		expectedErr := errors.New("unlock failed")

		manager.SetCallbacks(nil, func(ctx context.Context) error {
			return expectedErr
		})

		manager.MarkLocked()

		err := manager.ValidateSession(ctx)
		if err == nil {
			t.Error("Expected error when unlock fails")
		}

		if manager.state == SessionAuthenticated {
			t.Error("State should not be authenticated when unlock fails")
		}
	})

	t.Run("locked session without unlock callback", func(t *testing.T) {
		manager := NewManager(DefaultConfig())
		manager.MarkLocked()

		err := manager.ValidateSession(ctx)
		if err == nil {
			t.Error("Expected error when no unlock callback is set")
		}
	})
}

func TestManager_GetInfo(t *testing.T) {
	config := &Config{
		SessionIdleTimeout: 2 * time.Hour,
		EnableSessionLock:  true,
	}
	manager := NewManager(config)
	manager.MarkAuthenticated()

	info := manager.GetInfo()

	if info.State != SessionAuthenticated {
		t.Errorf("Expected state Authenticated, got %v", info.State)
	}

	if info.IdleTimeout != 2*time.Hour {
		t.Errorf("Expected timeout 2h, got %v", info.IdleTimeout)
	}

	if info.LastActivity.IsZero() {
		t.Error("Expected LastActivity to be set")
	}

	timeUntilLock := info.TimeUntilLock()
	if timeUntilLock <= 0 || timeUntilLock > 2*time.Hour {
		t.Errorf("Expected reasonable time until lock, got %v", timeUntilLock)
	}
}

func TestManager_IdleTimeout(t *testing.T) {
	// Use short timeout for test
	config := &Config{
		SessionIdleTimeout: 50 * time.Millisecond,
		EnableSessionLock:  true,
		CheckInterval:      10 * time.Millisecond,
	}

	manager := NewManager(config)
	lockCallbackCalled := make(chan bool, 1)

	manager.SetCallbacks(func() error {
		select {
		case lockCallbackCalled <- true:
		default:
		}
		return nil
	}, nil)

	manager.MarkAuthenticated()

	// Start monitoring
	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()

	manager.Start(ctx)
	defer manager.Stop()

	// Wait for idle timeout to trigger
	select {
	case <-lockCallbackCalled:
		// Success - session was locked due to idle timeout
	case <-ctx.Done():
		t.Error("Expected session to be locked due to idle timeout")
	}

	// Verify session is actually locked
	info := manager.GetInfo()
	if info.State != SessionLocked {
		t.Errorf("Expected session to be locked, got state %v", info.State)
	}
}

func TestManager_ConcurrentAccess(t *testing.T) {
	manager := NewManager(DefaultConfig())
	manager.MarkAuthenticated()

	// Test concurrent access to manager methods
	var wg sync.WaitGroup
	ctx := context.Background()

	// Multiple goroutines updating activity
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 100; j++ {
				manager.UpdateActivity()
			}
		}()
	}

	// Multiple goroutines getting info
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 100; j++ {
				_ = manager.GetInfo()
			}
		}()
	}

	// Multiple goroutines validating session
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 100; j++ {
				_ = manager.ValidateSession(ctx)
			}
		}()
	}

	wg.Wait()

	// If we reach here without deadlock, the test passes
}

func TestManager_DisabledSessionLock(t *testing.T) {
	config := &Config{
		EnableSessionLock: false,
	}

	manager := NewManager(config)
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	// Start should return immediately when session lock is disabled
	manager.Start(ctx)
	manager.Stop()

	// Session should remain in unknown state
	info := manager.GetInfo()
	if info.State != SessionUnknown {
		t.Errorf("Expected state to remain Unknown when session lock disabled, got %v", info.State)
	}
}
