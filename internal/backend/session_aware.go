package backend

import (
	"context"
	"fmt"
	"os/exec"
	"strings"

	"github.com/zach-source/opx/internal/session"
)

// SessionAwareBackend wraps another backend and adds session validation
type SessionAwareBackend struct {
	backend             Backend
	session             *session.Manager
	multiAccountSession *session.MultiAccountManager
}

// NewSessionAwareBackend creates a new session-aware backend wrapper
func NewSessionAwareBackend(backend Backend, sessionManager *session.Manager) *SessionAwareBackend {
	return &SessionAwareBackend{
		backend: backend,
		session: sessionManager,
	}
}

// NewMultiAccountSessionAwareBackend creates a backend with multi-account session support
func NewMultiAccountSessionAwareBackend(backend Backend, multiAccountSession *session.MultiAccountManager) *SessionAwareBackend {
	return &SessionAwareBackend{
		backend:             backend,
		multiAccountSession: multiAccountSession,
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
	// Extract account ID from flags for multi-account session management
	accountID := extractAccountFromFlags(flags)

	// Use multi-account session validation if available
	if s.multiAccountSession != nil && accountID != "" {
		if err := s.multiAccountSession.ValidateAccountSession(ctx, accountID); err != nil {
			return "", fmt.Errorf("account session validation failed: %w", err)
		}
	} else if s.session != nil {
		// Fall back to global session validation
		if err := s.session.ValidateSession(ctx); err != nil {
			return "", fmt.Errorf("session validation failed: %w", err)
		}
	}

	// Perform the actual read operation
	value, err := s.backend.ReadRefWithFlags(ctx, ref, flags)
	if err != nil {
		return "", err
	}

	// Update activity timestamp on successful operation
	if s.multiAccountSession != nil && accountID != "" {
		s.multiAccountSession.UpdateAccountActivity(accountID)
	} else if s.session != nil {
		s.session.UpdateActivity()
	}

	return value, nil
}

// extractAccountFromFlags extracts the account ID from command flags
func extractAccountFromFlags(flags []string) string {
	for _, flag := range flags {
		if strings.HasPrefix(flag, "--account=") {
			return strings.TrimPrefix(flag, "--account=")
		}
	}
	return "" // No account specified
}

// ValidateCurrentSession validates daemon access session (not 1Password sessions)
// This is used as the unlock callback for session validation
func ValidateCurrentSession(ctx context.Context) error {
	// For daemon session validation, we just need user to approve daemon access
	// We don't validate underlying 1Password sessions here since they're per-request
	// This represents: "Is the user approved to access the daemon for the next 8 hours?"

	// For now, always return nil since daemon access approval is implicit
	// TODO: Add interactive approval prompt for first-time access
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
