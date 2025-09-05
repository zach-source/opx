package session

import (
	"context"
	"errors"
	"log"
	"sync"
	"time"
)

// LockCallback is called when the session needs to be locked
type LockCallback func() error

// UnlockCallback is called to validate and potentially unlock a session
type UnlockCallback func(ctx context.Context) error

// Manager manages session state and idle timeout functionality
type Manager struct {
	mu             sync.RWMutex
	config         *Config
	state          SessionState
	lastActivity   time.Time
	lockedAt       time.Time
	lockCallback   LockCallback
	unlockCallback UnlockCallback
	stopCh         chan struct{}
	doneCh         chan struct{}
	verbose        bool
}

// NewManager creates a new session manager with the given configuration
func NewManager(config *Config) *Manager {
	if config == nil {
		config = DefaultConfig()
	}

	return &Manager{
		config:       config,
		state:        SessionUnknown,
		lastActivity: time.Now(),
		stopCh:       make(chan struct{}),
		doneCh:       make(chan struct{}),
	}
}

// SetCallbacks sets the lock and unlock callback functions
func (m *Manager) SetCallbacks(lockFn LockCallback, unlockFn UnlockCallback) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.lockCallback = lockFn
	m.unlockCallback = unlockFn
}

// SetVerbose enables or disables verbose logging
func (m *Manager) SetVerbose(verbose bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.verbose = verbose
}

// Start begins the session manager's background monitoring
func (m *Manager) Start(ctx context.Context) {
	if !m.config.EnableSessionLock {
		// Close doneCh immediately since we're not starting monitoring
		close(m.doneCh)
		return // Session locking is disabled
	}

	go m.monitor(ctx)
}

// Stop stops the session manager's background monitoring
func (m *Manager) Stop() {
	close(m.stopCh)
	<-m.doneCh
}

// GetInfo returns current session information
func (m *Manager) GetInfo() SessionInfo {
	m.mu.RLock()
	defer m.mu.RUnlock()

	return SessionInfo{
		State:        m.state,
		LastActivity: m.lastActivity,
		IdleTimeout:  m.config.SessionIdleTimeout,
		LockedAt:     m.lockedAt,
	}
}

// UpdateActivity updates the last activity timestamp if session is active
func (m *Manager) UpdateActivity() {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.state == SessionAuthenticated {
		m.lastActivity = time.Now()
		if m.verbose {
			log.Printf("[session] activity updated")
		}
	}
}

// ValidateSession checks if the session is in a valid state for operations
func (m *Manager) ValidateSession(ctx context.Context) error {
	m.mu.RLock()
	currentState := m.state
	requiresUnlock := currentState.RequiresUnlock()
	isActive := currentState.IsActive()
	m.mu.RUnlock()

	// If session is active, we're good
	if isActive {
		return nil
	}

	// If session requires unlock, attempt to unlock it
	if requiresUnlock {
		return m.attemptUnlock(ctx)
	}

	// Unknown state - attempt to determine current state
	if currentState == SessionUnknown {
		return m.determineInitialState(ctx)
	}

	return errors.New("session validation failed")
}

// MarkLocked manually locks the session (e.g., on auth failure)
func (m *Manager) MarkLocked() {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.state != SessionLocked {
		m.state = SessionLocked
		m.lockedAt = time.Now()
		if m.verbose {
			log.Printf("[session] marked as locked")
		}
		m.executeLockCallback()
	}
}

// MarkAuthenticated marks the session as authenticated
func (m *Manager) MarkAuthenticated() {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.state = SessionAuthenticated
	m.lastActivity = time.Now()
	m.lockedAt = time.Time{} // Clear lock time
	if m.verbose {
		log.Printf("[session] marked as authenticated")
	}
}

// monitor runs the background idle timeout checking
func (m *Manager) monitor(ctx context.Context) {
	defer close(m.doneCh)

	ticker := time.NewTicker(m.config.CheckInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-m.stopCh:
			return
		case <-ticker.C:
			m.checkIdleTimeout()
		}
	}
}

// checkIdleTimeout checks if the session should be locked due to idle timeout
func (m *Manager) checkIdleTimeout() {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Only check if session is currently authenticated
	if m.state != SessionAuthenticated {
		return
	}

	// Check if idle timeout has been exceeded
	if m.config.SessionIdleTimeout > 0 && time.Since(m.lastActivity) > m.config.SessionIdleTimeout {
		if m.verbose {
			log.Printf("[session] idle timeout exceeded, locking session")
		}
		m.state = SessionLocked
		m.lockedAt = time.Now()
		m.executeLockCallback()
	}
}

// executeLockCallback executes the lock callback if set
func (m *Manager) executeLockCallback() {
	if m.lockCallback != nil {
		if err := m.lockCallback(); err != nil && m.verbose {
			log.Printf("[session] lock callback failed: %v", err)
		}
	}
}

// attemptUnlock attempts to unlock the session using the unlock callback
func (m *Manager) attemptUnlock(ctx context.Context) error {
	if m.unlockCallback == nil {
		return errors.New("session locked and no unlock callback configured")
	}

	m.mu.Lock()
	currentState := m.state
	m.mu.Unlock()

	if m.verbose {
		log.Printf("[session] attempting to unlock session (current state: %s)", currentState)
	}

	if err := m.unlockCallback(ctx); err != nil {
		if m.verbose {
			log.Printf("[session] unlock failed: %v", err)
		}
		return err
	}

	// Unlock succeeded
	m.MarkAuthenticated()
	if m.verbose {
		log.Printf("[session] session unlocked successfully")
	}
	return nil
}

// determineInitialState attempts to determine the initial session state
func (m *Manager) determineInitialState(ctx context.Context) error {
	if m.unlockCallback == nil {
		// No way to determine state, assume locked
		m.mu.Lock()
		m.state = SessionLocked
		m.lockedAt = time.Now()
		m.mu.Unlock()
		return errors.New("session state unknown and no unlock callback configured")
	}

	// Try to validate current session state
	if err := m.unlockCallback(ctx); err != nil {
		// Validation failed, session is locked/expired
		m.mu.Lock()
		m.state = SessionLocked
		m.lockedAt = time.Now()
		m.mu.Unlock()
		return err
	}

	// Validation succeeded, session is authenticated
	m.MarkAuthenticated()
	return nil
}
