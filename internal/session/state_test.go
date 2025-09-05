package session

import (
	"testing"
	"time"
)

func TestSessionState_String(t *testing.T) {
	tests := []struct {
		state    SessionState
		expected string
	}{
		{SessionUnknown, "unknown"},
		{SessionAuthenticated, "authenticated"},
		{SessionLocked, "locked"},
		{SessionExpired, "expired"},
		{SessionState(99), "invalid"}, // Invalid state
	}

	for _, test := range tests {
		if got := test.state.String(); got != test.expected {
			t.Errorf("SessionState(%d).String() = %q, want %q", test.state, got, test.expected)
		}
	}
}

func TestSessionState_IsActive(t *testing.T) {
	tests := []struct {
		state    SessionState
		expected bool
	}{
		{SessionUnknown, false},
		{SessionAuthenticated, true},
		{SessionLocked, false},
		{SessionExpired, false},
	}

	for _, test := range tests {
		if got := test.state.IsActive(); got != test.expected {
			t.Errorf("SessionState(%s).IsActive() = %t, want %t", test.state, got, test.expected)
		}
	}
}

func TestSessionState_RequiresUnlock(t *testing.T) {
	tests := []struct {
		state    SessionState
		expected bool
	}{
		{SessionUnknown, false},
		{SessionAuthenticated, false},
		{SessionLocked, true},
		{SessionExpired, true},
	}

	for _, test := range tests {
		if got := test.state.RequiresUnlock(); got != test.expected {
			t.Errorf("SessionState(%s).RequiresUnlock() = %t, want %t", test.state, got, test.expected)
		}
	}
}

func TestSessionInfo_TimeUntilLock(t *testing.T) {
	now := time.Now()

	tests := []struct {
		name     string
		info     SessionInfo
		expected time.Duration
	}{
		{
			name: "authenticated with time remaining",
			info: SessionInfo{
				State:        SessionAuthenticated,
				LastActivity: now.Add(-30 * time.Minute),
				IdleTimeout:  1 * time.Hour,
			},
			expected: 30 * time.Minute,
		},
		{
			name: "authenticated but already expired",
			info: SessionInfo{
				State:        SessionAuthenticated,
				LastActivity: now.Add(-2 * time.Hour),
				IdleTimeout:  1 * time.Hour,
			},
			expected: 0,
		},
		{
			name: "locked session",
			info: SessionInfo{
				State:        SessionLocked,
				LastActivity: now.Add(-30 * time.Minute),
				IdleTimeout:  1 * time.Hour,
			},
			expected: 0,
		},
		{
			name: "timeout disabled",
			info: SessionInfo{
				State:        SessionAuthenticated,
				LastActivity: now.Add(-30 * time.Minute),
				IdleTimeout:  0,
			},
			expected: 0,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got := test.info.TimeUntilLock()

			// For time-based tests, allow some tolerance
			if test.expected == 0 {
				if got != 0 {
					t.Errorf("Expected 0, got %v", got)
				}
			} else {
				tolerance := 1 * time.Second
				if got < test.expected-tolerance || got > test.expected+tolerance {
					t.Errorf("Expected ~%v, got %v", test.expected, got)
				}
			}
		})
	}
}

func TestSessionInfo_IsIdle(t *testing.T) {
	now := time.Now()

	tests := []struct {
		name     string
		info     SessionInfo
		expected bool
	}{
		{
			name: "not idle",
			info: SessionInfo{
				LastActivity: now.Add(-30 * time.Minute),
				IdleTimeout:  1 * time.Hour,
			},
			expected: false,
		},
		{
			name: "is idle",
			info: SessionInfo{
				LastActivity: now.Add(-2 * time.Hour),
				IdleTimeout:  1 * time.Hour,
			},
			expected: true,
		},
		{
			name: "timeout disabled",
			info: SessionInfo{
				LastActivity: now.Add(-2 * time.Hour),
				IdleTimeout:  0,
			},
			expected: false,
		},
		{
			name: "negative timeout",
			info: SessionInfo{
				LastActivity: now.Add(-2 * time.Hour),
				IdleTimeout:  -1 * time.Hour,
			},
			expected: false,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got := test.info.IsIdle()
			if got != test.expected {
				t.Errorf("Expected %t, got %t", test.expected, got)
			}
		})
	}
}
