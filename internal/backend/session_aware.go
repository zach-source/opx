package backend

import (
	"context"
	"fmt"
	"os/exec"

	"github.com/zach-source/opx/internal/session"
)

// SessionAwareBackend wraps another backend and adds session validation
type SessionAwareBackend struct {
	backend Backend
	session *session.Manager
}

// NewSessionAwareBackend creates a new session-aware backend wrapper
func NewSessionAwareBackend(backend Backend, sessionManager *session.Manager) *SessionAwareBackend {
	return &SessionAwareBackend{
		backend: backend,
		session: sessionManager,
	}
}

// Name returns the wrapped backend's name with session awareness indicator
func (s *SessionAwareBackend) Name() string {
	return s.backend.Name() + "+session"
}

// ReadRef reads a secret reference with session validation
func (s *SessionAwareBackend) ReadRef(ctx context.Context, ref string) (string, error) {
	return s.ReadRefWithFlags(ctx, ref, nil)
}

// ReadRefWithFlags reads a secret reference with flags and session validation
func (s *SessionAwareBackend) ReadRefWithFlags(ctx context.Context, ref string, flags []string) (string, error) {
	// Validate session state before attempting to read secrets
	if err := s.session.ValidateSession(ctx); err != nil {
		return "", fmt.Errorf("session validation failed: %w", err)
	}

	// Perform the actual read operation
	value, err := s.backend.ReadRefWithFlags(ctx, ref, flags)
	if err != nil {
		return "", err
	}

	// Update activity timestamp on successful operation
	s.session.UpdateActivity()

	return value, nil
}

// ValidateCurrentSession checks if the current 1Password CLI session is valid
// This is used as the unlock callback for session validation
func ValidateCurrentSession(ctx context.Context) error {
	// Use `op whoami` to check if there's an active session
	cmd := exec.CommandContext(ctx, "op", "whoami")
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("1Password CLI session invalid or expired: %w", err)
	}
	return nil
}

// ClearCLISession clears the current 1Password CLI session
// This is used as the lock callback to secure secrets when session locks
func ClearCLISession() error {
	// Use `op signout --forget` to clear the session
	cmd := exec.Command("op", "signout", "--forget")
	if err := cmd.Run(); err != nil {
		// Don't return error if signout fails - session might already be cleared
		// Just log that we attempted to clear it
		return nil
	}
	return nil
}

// NewSessionAwareOpCLI creates a new OpCLI backend with session management
func NewSessionAwareOpCLI(sessionManager *session.Manager) Backend {
	// Set up session callbacks
	sessionManager.SetCallbacks(ClearCLISession, ValidateCurrentSession)

	return NewSessionAwareBackend(OpCLI{}, sessionManager)
}

// NewSessionAwareFake creates a new Fake backend with session management for testing
func NewSessionAwareFake(sessionManager *session.Manager) Backend {
	// For fake backend, we don't need to clear anything, just use no-op callbacks
	sessionManager.SetCallbacks(func() error { return nil }, func(ctx context.Context) error { return nil })

	return NewSessionAwareBackend(Fake{}, sessionManager)
}
