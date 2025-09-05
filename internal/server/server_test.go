package server

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/zach-source/opx/internal/backend"
	"github.com/zach-source/opx/internal/cache"
	"github.com/zach-source/opx/internal/protocol"
	"github.com/zach-source/opx/internal/session"
)

func TestServer_StatusHandler(t *testing.T) {
	// Test status handler without session management
	srv := &Server{
		Backend: backend.Fake{},
		Cache:   cache.New(5 * time.Minute),
		Verbose: false,
	}

	req := httptest.NewRequest("GET", "/v1/status", nil)
	w := httptest.NewRecorder()

	srv.handleStatus(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", w.Code)
	}

	var status protocol.Status
	if err := json.NewDecoder(w.Body).Decode(&status); err != nil {
		t.Fatalf("Failed to decode status: %v", err)
	}

	if status.Backend != "fake" {
		t.Errorf("Expected backend 'fake', got %q", status.Backend)
	}

	if status.Session != nil {
		t.Error("Expected no session info when session manager is nil")
	}
}

func TestServer_StatusHandlerWithSessionManagement(t *testing.T) {
	// Create session manager with proper configuration
	sessionConfig := &session.Config{
		SessionIdleTimeout: 1 * time.Hour,
		EnableSessionLock:  true,
		LockOnAuthFailure:  true,
		CheckInterval:      1 * time.Minute,
	}
	sessionManager := session.NewManager(sessionConfig)

	// Create session-aware backend
	be := backend.NewSessionAwareFake(sessionManager)

	// Create server
	srv := &Server{
		Backend: be,
		Cache:   cache.New(5 * time.Minute),
		Session: sessionManager,
		Verbose: false,
	}

	req := httptest.NewRequest("GET", "/v1/status", nil)
	w := httptest.NewRecorder()

	srv.handleStatus(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", w.Code)
	}

	var status protocol.Status
	if err := json.NewDecoder(w.Body).Decode(&status); err != nil {
		t.Fatalf("Failed to decode status: %v", err)
	}

	// Check session information is present
	if status.Session == nil {
		t.Error("Expected session information in status")
	} else {
		if status.Session.State == "" {
			t.Error("Expected session state to be set")
		}
		if status.Session.IdleTimeout != 3600 {
			t.Errorf("Expected idle timeout 3600 seconds, got %d", status.Session.IdleTimeout)
		}
		if !status.Session.Enabled {
			t.Error("Expected session to be enabled")
		}
	}

	if status.Backend != "fake+session" {
		t.Errorf("Expected backend 'fake+session', got %q", status.Backend)
	}
}

func TestServer_SessionUnlockHandler(t *testing.T) {
	// Create session manager with proper configuration
	sessionConfig := &session.Config{
		SessionIdleTimeout: 1 * time.Hour,
		EnableSessionLock:  true,
		CheckInterval:      1 * time.Minute,
	}
	sessionManager := session.NewManager(sessionConfig)

	// Set up callbacks for testing (similar to session-aware backend)
	sessionManager.SetCallbacks(
		func() error { return nil },                    // Lock callback
		func(ctx context.Context) error { return nil }, // Unlock callback that always succeeds
	)

	// Create server
	srv := &Server{
		Backend: backend.Fake{},
		Cache:   cache.New(5 * time.Minute),
		Session: sessionManager,
		Verbose: false,
	}

	// Test session unlock endpoint directly (without auth middleware for now)
	req := httptest.NewRequest("POST", "/v1/session/unlock", strings.NewReader("{}"))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	srv.handleSessionUnlock(w, req)

	// Should succeed with fake backend
	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d. Body: %s", w.Code, w.Body.String())
	}

	var unlockResp protocol.SessionUnlockResponse
	if err := json.NewDecoder(w.Body).Decode(&unlockResp); err != nil {
		t.Fatalf("Failed to decode unlock response: %v", err)
	}

	if !unlockResp.Success {
		t.Error("Expected unlock to succeed")
	}
}

func TestServer_SessionUnlockHandlerWithoutSessionManager(t *testing.T) {
	// Test server behavior when no session manager is configured
	srv := &Server{
		Backend: backend.Fake{},
		Cache:   cache.New(5 * time.Minute),
		Session: nil, // No session manager
		Verbose: false,
	}

	// Test unlock endpoint - should return error
	req := httptest.NewRequest("POST", "/v1/session/unlock", strings.NewReader("{}"))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	srv.handleSessionUnlock(w, req)

	if w.Code == http.StatusOK {
		t.Error("Expected unlock endpoint to fail when session management is disabled")
	}

	var unlockResp protocol.SessionUnlockResponse
	if err := json.NewDecoder(w.Body).Decode(&unlockResp); err != nil {
		t.Fatalf("Failed to decode unlock response: %v", err)
	}

	if unlockResp.Success {
		t.Error("Expected unlock to fail when session management is disabled")
	}

	if unlockResp.State != "disabled" {
		t.Errorf("Expected state 'disabled', got %q", unlockResp.State)
	}
}
