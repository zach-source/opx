package server

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
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

// countingBackend records every backend invocation so tests can prove the cache
// and singleflight actually suppress duplicate reads.
type countingBackend struct {
	mu    sync.Mutex
	calls [][]string // flags passed on each call
}

func (c *countingBackend) Name() string { return "counting" }

func (c *countingBackend) ReadRef(ctx context.Context, ref string) (string, error) {
	return c.ReadRefWithFlags(ctx, ref, nil)
}

func (c *countingBackend) ReadRefWithFlags(ctx context.Context, ref string, flags []string) (string, error) {
	c.mu.Lock()
	c.calls = append(c.calls, flags)
	c.mu.Unlock()
	return "secret-for-" + ref, nil
}

func (c *countingBackend) count() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return len(c.calls)
}

// A ref with no explicit account must produce exactly one `op read`, no matter how
// many 1Password accounts are signed in. Fanning out per account would mean one
// unlock prompt per account, which is precisely what this daemon exists to avoid.
func TestReadOne_NoAccountFanout(t *testing.T) {
	be := &countingBackend{}
	srv := &Server{Backend: be, Cache: cache.New(time.Hour)}

	const ref = "op://Personal/example/password"

	// Concurrent readers: singleflight must collapse them into one backend call.
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, err := srv.readOne(context.Background(), ref); err != nil {
				t.Errorf("readOne: %v", err)
			}
		}()
	}
	wg.Wait()

	// Sequential re-read inside the TTL: must come from cache.
	rr, err := srv.readOne(context.Background(), ref)
	if err != nil {
		t.Fatalf("readOne: %v", err)
	}
	if !rr.FromCache {
		t.Error("second read should have been served from cache")
	}

	if got := be.count(); got != 1 {
		t.Fatalf("backend called %d times, want exactly 1 (one prompt, not one per account)", got)
	}
	if flags := be.calls[0]; len(flags) != 0 {
		t.Errorf("no --account flag should be synthesized for an account-less ref, got %v", flags)
	}
}

// Rotating a secret out of band must be able to take effect immediately rather
// than at the end of a 4h TTL.
func TestHandleInvalidate(t *testing.T) {
	const ref = "op://Test/item/password"

	newSrv := func() (*Server, *countingBackend) {
		be := &countingBackend{}
		srv := &Server{Backend: be, Cache: cache.New(time.Hour)}
		if _, err := srv.readOne(context.Background(), ref); err != nil {
			t.Fatalf("seed read: %v", err)
		}
		return srv, be
	}

	t.Run("by ref forces the next read back to the backend", func(t *testing.T) {
		srv, be := newSrv()

		body := strings.NewReader(`{"refs":["` + ref + `"]}`)
		w := httptest.NewRecorder()
		srv.handleInvalidate(w, httptest.NewRequest("POST", "/v1/invalidate", body))

		if w.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200", w.Code)
		}
		var resp protocol.InvalidateResponse
		if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
			t.Fatalf("decode: %v", err)
		}
		if resp.Removed != 1 {
			t.Errorf("removed = %d, want 1", resp.Removed)
		}

		rr, err := srv.readOne(context.Background(), ref)
		if err != nil {
			t.Fatalf("readOne: %v", err)
		}
		if rr.FromCache {
			t.Error("read after invalidate was served from cache")
		}
		if be.count() != 2 {
			t.Errorf("backend called %d times, want 2 (seed + re-read)", be.count())
		}
	})

	t.Run("--all clears everything", func(t *testing.T) {
		srv, _ := newSrv()

		w := httptest.NewRecorder()
		srv.handleInvalidate(w, httptest.NewRequest("POST", "/v1/invalidate", strings.NewReader(`{"all":true}`)))

		if w.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200", w.Code)
		}
		if size, _, _, _ := srv.Cache.Stats(); size != 0 {
			t.Errorf("cache still holds %d entries", size)
		}
	})

	t.Run("empty request is rejected", func(t *testing.T) {
		srv, _ := newSrv()

		w := httptest.NewRecorder()
		srv.handleInvalidate(w, httptest.NewRequest("POST", "/v1/invalidate", strings.NewReader(`{}`)))

		if w.Code != http.StatusBadRequest {
			t.Errorf("status = %d, want 400", w.Code)
		}
		if size, _, _, _ := srv.Cache.Stats(); size != 1 {
			t.Errorf("an empty request changed the cache (size=%d)", size)
		}
	})
}

// Flag variants (e.g. --account) are distinct cache keys for the same ref, so
// invalidating the ref has to take all of them.
func TestCacheInvalidate_CoversFlagVariants(t *testing.T) {
	const ref = "op://Test/item/password"

	c := cache.New(time.Hour)
	c.Set(ref, "plain")
	c.Set(ref+"|flags:--account=A", "acct-a")
	c.Set(ref+"|flags:--account=B", "acct-b")
	c.Set("op://Test/other/password", "untouched")

	if removed := c.Invalidate(ref); removed != 3 {
		t.Errorf("removed = %d, want 3", removed)
	}
	if _, ok, _, _ := c.Get("op://Test/other/password"); !ok {
		t.Error("Invalidate removed an unrelated ref")
	}
}

// flakyBackend can be flipped to start failing, simulating a transient outage.
type flakyBackend struct {
	value string
	fail  bool
}

func (f *flakyBackend) Name() string { return "flaky" }

func (f *flakyBackend) ReadRef(ctx context.Context, ref string) (string, error) {
	return f.ReadRefWithFlags(ctx, ref, nil)
}

func (f *flakyBackend) ReadRefWithFlags(ctx context.Context, ref string, flags []string) (string, error) {
	if f.fail {
		return "", errors.New("backend unavailable")
	}
	return f.value, nil
}
