package session

import (
	"context"
	"fmt"
	"sync"
	"time"
)

// AccountSession represents session state for a specific account
type AccountSession struct {
	AccountID    string        `json:"account_id"`
	State        SessionState  `json:"state"`
	LastActivity time.Time     `json:"last_activity"`
	IdleTimeout  time.Duration `json:"idle_timeout"`
}

// MultiAccountManager manages sessions for multiple accounts
type MultiAccountManager struct {
	mu             sync.RWMutex
	sessions       map[string]*AccountSession
	config         *Config
	lockCallback   LockCallback
	unlockCallback UnlockCallback
	stopCh         chan struct{}
	doneCh         chan struct{}
	verbose        bool
}

// NewMultiAccountManager creates a new multi-account session manager
func NewMultiAccountManager(config *Config) *MultiAccountManager {
	return &MultiAccountManager{
		sessions: make(map[string]*AccountSession),
		config:   config,
		stopCh:   make(chan struct{}),
		doneCh:   make(chan struct{}),
	}
}

// Start begins monitoring all account sessions
func (mam *MultiAccountManager) Start(ctx context.Context) {
	if !mam.config.EnableSessionLock {
		close(mam.doneCh)
		return
	}

	go mam.monitorSessions(ctx)
}

// Stop halts session monitoring
func (mam *MultiAccountManager) Stop() {
	close(mam.stopCh)
	<-mam.doneCh
}

// SetCallbacks sets global callbacks for all accounts
func (mam *MultiAccountManager) SetCallbacks(lock LockCallback, unlock UnlockCallback) {
	mam.mu.Lock()
	defer mam.mu.Unlock()
	mam.lockCallback = lock
	mam.unlockCallback = unlock
}

// SetVerbose enables verbose logging
func (mam *MultiAccountManager) SetVerbose(verbose bool) {
	mam.verbose = verbose
}

// GetAccountSession returns session info for a specific account
func (mam *MultiAccountManager) GetAccountSession(accountID string) *AccountSession {
	mam.mu.RLock()
	defer mam.mu.RUnlock()

	session, exists := mam.sessions[accountID]
	if !exists {
		// Create new session for this account
		session = &AccountSession{
			AccountID:    accountID,
			State:        SessionAuthenticated, // Assume authenticated when first accessed
			LastActivity: time.Now(),
			IdleTimeout:  mam.config.SessionIdleTimeout,
		}
		mam.sessions[accountID] = session
	}

	return session
}

// ValidateAccountSession validates session for a specific account
func (mam *MultiAccountManager) ValidateAccountSession(ctx context.Context, accountID string) error {
	mam.mu.Lock()
	defer mam.mu.Unlock()

	session := mam.GetAccountSession(accountID)

	// Check if session is locked or expired
	if session.State == SessionLocked || session.State == SessionExpired {
		// Try to unlock using account-specific validation
		if mam.unlockCallback != nil {
			if err := mam.unlockCallback(ctx); err != nil {
				return fmt.Errorf("session locked for account %s: %w", accountID, err)
			}
			session.State = SessionAuthenticated
			session.LastActivity = time.Now()
		} else {
			return fmt.Errorf("session locked for account %s and no unlock callback configured", accountID)
		}
	}

	// Check idle timeout
	if mam.config.SessionIdleTimeout > 0 && time.Since(session.LastActivity) > mam.config.SessionIdleTimeout {
		session.State = SessionExpired
		return fmt.Errorf("session expired for account %s after %v idle", accountID, mam.config.SessionIdleTimeout)
	}

	return nil
}

// UpdateAccountActivity updates last activity for a specific account
func (mam *MultiAccountManager) UpdateAccountActivity(accountID string) {
	mam.mu.Lock()
	defer mam.mu.Unlock()

	session := mam.GetAccountSession(accountID)
	session.LastActivity = time.Now()
	session.State = SessionAuthenticated
}

// LockAccountSession locks a specific account session
func (mam *MultiAccountManager) LockAccountSession(accountID string) error {
	mam.mu.Lock()
	defer mam.mu.Unlock()

	session := mam.GetAccountSession(accountID)
	session.State = SessionLocked

	if mam.lockCallback != nil {
		return mam.lockCallback()
	}

	return nil
}

// GetAllSessions returns information about all account sessions
func (mam *MultiAccountManager) GetAllSessions() map[string]*AccountSession {
	mam.mu.RLock()
	defer mam.mu.RUnlock()

	result := make(map[string]*AccountSession)
	for accountID, session := range mam.sessions {
		// Return a copy to prevent external modification
		sessionCopy := *session
		result[accountID] = &sessionCopy
	}

	return result
}

// monitorSessions periodically checks all account sessions for expiration
func (mam *MultiAccountManager) monitorSessions(ctx context.Context) {
	defer close(mam.doneCh)

	if mam.config.CheckInterval <= 0 {
		return
	}

	ticker := time.NewTicker(mam.config.CheckInterval)
	defer ticker.Stop()

	for {
		select {
		case <-mam.stopCh:
			return
		case <-ctx.Done():
			return
		case <-ticker.C:
			mam.checkAllSessionTimeouts()
		}
	}
}

// checkAllSessionTimeouts checks all account sessions for idle timeout
func (mam *MultiAccountManager) checkAllSessionTimeouts() {
	mam.mu.Lock()
	defer mam.mu.Unlock()

	if mam.config.SessionIdleTimeout <= 0 {
		return
	}

	now := time.Now()
	for accountID, session := range mam.sessions {
		if session.State == SessionAuthenticated &&
			now.Sub(session.LastActivity) > mam.config.SessionIdleTimeout {

			if mam.verbose {
				fmt.Printf("[session] account %s idle timeout reached, locking session\n", accountID)
			}

			session.State = SessionLocked

			if mam.lockCallback != nil {
				mam.lockCallback() // Call global lock callback
			}
		}
	}
}
