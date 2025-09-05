package session

import "time"

// SessionState represents the current state of the 1Password CLI session
type SessionState int

const (
	// SessionUnknown indicates the session state hasn't been determined yet
	SessionUnknown SessionState = iota
	// SessionAuthenticated indicates the session is active and authenticated
	SessionAuthenticated
	// SessionLocked indicates the session has been locked due to idle timeout
	SessionLocked
	// SessionExpired indicates the session has expired and needs re-authentication
	SessionExpired
)

// String returns a human-readable string representation of the session state
func (s SessionState) String() string {
	switch s {
	case SessionUnknown:
		return "unknown"
	case SessionAuthenticated:
		return "authenticated"
	case SessionLocked:
		return "locked"
	case SessionExpired:
		return "expired"
	default:
		return "invalid"
	}
}

// IsActive returns true if the session state allows secret operations
func (s SessionState) IsActive() bool {
	return s == SessionAuthenticated
}

// RequiresUnlock returns true if the session state requires unlocking
func (s SessionState) RequiresUnlock() bool {
	return s == SessionLocked || s == SessionExpired
}

// SessionInfo holds information about the current session
type SessionInfo struct {
	State        SessionState  `json:"state"`
	LastActivity time.Time     `json:"last_activity,omitempty"`
	IdleTimeout  time.Duration `json:"idle_timeout"`
	LockedAt     time.Time     `json:"locked_at,omitempty"`
}

// TimeUntilLock returns the duration until the session will be locked
// Returns 0 if already locked or if idle timeout is disabled
func (si *SessionInfo) TimeUntilLock() time.Duration {
	if si.State != SessionAuthenticated || si.IdleTimeout <= 0 {
		return 0
	}

	elapsed := time.Since(si.LastActivity)
	remaining := si.IdleTimeout - elapsed
	if remaining < 0 {
		return 0
	}
	return remaining
}

// IsIdle returns true if the session has been idle longer than the timeout
func (si *SessionInfo) IsIdle() bool {
	if si.IdleTimeout <= 0 {
		return false
	}
	return time.Since(si.LastActivity) > si.IdleTimeout
}
