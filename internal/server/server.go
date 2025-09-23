package server

import (
	"context"
	"crypto/subtle"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"golang.org/x/sync/singleflight"
	"golang.org/x/time/rate"

	"github.com/zach-source/opx/internal/audit"
	"github.com/zach-source/opx/internal/backend"
	"github.com/zach-source/opx/internal/cache"
	"github.com/zach-source/opx/internal/policy"
	"github.com/zach-source/opx/internal/protocol"
	"github.com/zach-source/opx/internal/security"
	"github.com/zach-source/opx/internal/session"
	"github.com/zach-source/opx/internal/util"
)

// Context key for peer information
type contextKey string

const peerInfoKey = contextKey("peerInfo")

// PeerAwareTLSListener wraps a Unix listener to extract peer info before TLS
type PeerAwareTLSListener struct {
	listener  net.Listener
	tlsConfig *tls.Config
	server    *Server
}

// Accept accepts a connection and extracts peer info before TLS wrapping
func (l *PeerAwareTLSListener) Accept() (net.Conn, error) {
	// Accept raw Unix socket connection
	rawConn, err := l.listener.Accept()
	if err != nil {
		return nil, err
	}

	// Extract peer info from Unix socket before TLS wrapping
	var peerInfo *security.PeerInfo
	if unixConn, ok := rawConn.(*net.UnixConn); ok {
		if info, err := security.PeerFromUnixConn(unixConn); err == nil {
			peerInfo = info
			if l.server.Verbose {
				log.Printf("[security] peer connection: %s", info.String())
			}
		} else if l.server.Verbose {
			log.Printf("[security] failed to get peer info: %v", err)
		}
	}

	// Apply TLS to the connection
	tlsConn := tls.Server(rawConn, l.tlsConfig)

	// Create wrapper that carries peer info
	return &PeerAwareConn{
		Conn:     tlsConn,
		peerInfo: peerInfo,
	}, nil
}

// Close closes the underlying listener
func (l *PeerAwareTLSListener) Close() error {
	return l.listener.Close()
}

// Addr returns the listener's network address
func (l *PeerAwareTLSListener) Addr() net.Addr {
	return l.listener.Addr()
}

// PeerAwareConn wraps a connection with extracted peer information
type PeerAwareConn struct {
	net.Conn
	peerInfo *security.PeerInfo
}

// GetPeerInfo returns the extracted peer information
func (c *PeerAwareConn) GetPeerInfo() *security.PeerInfo {
	return c.peerInfo
}

type Server struct {
	SockPath    string
	Token       string
	Cache       *cache.Cache
	Backend     backend.Backend
	Session     *session.Manager
	Policy      policy.Policy
	PolicyPath  string
	AuditLogger *audit.Logger
	RateLimiter *rate.Limiter
	Verbose     bool

	sf singleflight.Group
	mu sync.Mutex
}

func (s *Server) Serve(ctx context.Context) error {
	if s.SockPath == "" {
		p, err := util.SocketPath()
		if err != nil {
			return err
		}
		s.SockPath = p
	}
	// Prepare socket
	if err := os.MkdirAll(filepath.Dir(s.SockPath), 0o700); err != nil {
		return err
	}
	_ = os.Remove(s.SockPath) // remove stale

	// Setup TLS configuration
	tlsConfig, err := util.TLSConfig()
	if err != nil {
		return fmt.Errorf("failed to setup TLS: %w", err)
	}

	l, err := net.Listen("unix", s.SockPath)
	if err != nil {
		return fmt.Errorf("listen unix %s: %w", s.SockPath, err)
	}
	if err := os.Chmod(s.SockPath, 0o700); err != nil {
		return err
	}

	// Create peer-aware TLS listener
	peerListener := &PeerAwareTLSListener{
		listener:  l,
		tlsConfig: tlsConfig,
		server:    s,
	}

	// Secure token management
	tok, err := util.EnsureSecureToken()
	if err != nil {
		return err
	}
	s.Token = tok

	mux := http.NewServeMux()
	mux.HandleFunc("/v1/status", s.rateLimit(s.auth(s.handleStatus)))
	mux.HandleFunc("/v1/read", s.rateLimit(s.authWithPolicy(s.limitRequestSize(s.handleRead))))
	mux.HandleFunc("/v1/reads", s.rateLimit(s.authWithPolicy(s.limitRequestSize(s.handleReads))))
	mux.HandleFunc("/v1/resolve", s.rateLimit(s.authWithPolicy(s.limitRequestSize(s.handleResolve))))
	mux.HandleFunc("/v1/session/unlock", s.rateLimit(s.auth(s.limitRequestSize(s.handleSessionUnlock))))

	srv := &http.Server{
		Handler:     mux,
		ConnContext: s.peerConnContext,
	}

	// Start periodic cache cleanup
	go s.startCacheCleanup(ctx)

	// Session management
	if s.Session != nil {
		// Set up cache clearing callback for security
		s.setupSessionLockCallback()
		s.Session.Start(ctx)
		defer s.Session.Stop()
	}

	go func() {
		<-ctx.Done()
		_ = srv.Close()
		_ = peerListener.Close()
		_ = l.Close()
		_ = os.Remove(s.SockPath)
	}()

	if s.Verbose {
		log.Printf("op-authd listening on unix+tls://%s backend=%s ttl=%s", s.SockPath, s.Backend.Name(), s.CacheTTL())
	}

	return srv.Serve(peerListener)
}

// setupSessionLockCallback configures the session manager to clear cache on lock
func (s *Server) setupSessionLockCallback() {
	// Create lock callback that clears cache for security
	lockCallback := func() error {
		if s.Verbose {
			log.Printf("[session] clearing cache on session lock for security")
		}
		// Clear the cache for security when session locks
		s.Cache.Clear()
		return nil
	}

	// Set up session validation callback
	unlockCallback := func(ctx context.Context) error {
		// Validate session using CLI directly
		return backend.ValidateCurrentSession(ctx)
	}

	s.Session.SetCallbacks(lockCallback, unlockCallback)
}

// peerConnContext extracts peer information from PeerAwareConn
func (s *Server) peerConnContext(ctx context.Context, conn net.Conn) context.Context {
	if peerConn, ok := conn.(*PeerAwareConn); ok {
		if peerInfo := peerConn.GetPeerInfo(); peerInfo != nil {
			ctx = context.WithValue(ctx, peerInfoKey, *peerInfo)
			if s.Verbose {
				log.Printf("[security] using peer info: %s", peerInfo.String())
			}
		}
	}
	return ctx
}

func (s *Server) CacheTTL() time.Duration {
	return s.Cache.TTL()
}

func (s *Server) startCacheCleanup(ctx context.Context) {
	// Clean up expired entries every TTL/2 or every 30 seconds, whichever is longer
	interval := s.Cache.TTL() / 2
	if interval < 30*time.Second {
		interval = 30 * time.Second
	}

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			removed := s.Cache.CleanupExpired()
			if s.Verbose && removed > 0 {
				log.Printf("cache cleanup: removed %d expired entries", removed)
			}
		}
	}
}

func (s *Server) auth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		tok := r.Header.Get("X-OpAuthd-Token")
		if tok == "" || subtle.ConstantTimeCompare([]byte(tok), []byte(s.Token)) != 1 {
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = w.Write([]byte("unauthorized"))
			return
		}
		next(w, r)
	}
}

// authWithPolicy combines token auth with policy-based access control
func (s *Server) authWithPolicy(next http.HandlerFunc) http.HandlerFunc {
	return s.auth(func(w http.ResponseWriter, r *http.Request) {
		// Extract peer information from context
		peerInfo, hasPeer := r.Context().Value(peerInfoKey).(security.PeerInfo)
		if !hasPeer {
			if s.Verbose {
				log.Printf("[security] no peer information available for policy check")
			}
			// If we can't get peer info, fall back to basic auth (for backward compatibility)
			next(w, r)
			return
		}

		// Store peer info in request context for use by handlers
		ctx := context.WithValue(r.Context(), peerInfoKey, peerInfo)
		r = r.WithContext(ctx)
		next(w, r)
	})
}

// validateAccess checks if peer is allowed to access the given reference
func (s *Server) validateAccess(peerInfo security.PeerInfo, ref string) bool {
	subject := policy.Subject{
		PeerInfo: peerInfo,
	}

	allowed := policy.Allowed(s.Policy, subject, ref)

	// Audit log the access decision
	if s.AuditLogger != nil {
		details := map[string]string{
			"subject_pid":  fmt.Sprintf("%d", subject.PeerInfo.PID),
			"subject_path": subject.PeerInfo.ExecutablePath,
		}
		s.AuditLogger.LogAccessDecision(peerInfo, ref, allowed, s.PolicyPath, details)
	}

	if s.Verbose {
		if allowed {
			log.Printf("[security] access granted: %s -> %s", peerInfo.String(), ref)
		} else {
			log.Printf("[security] access denied: %s -> %s", peerInfo.String(), ref)
		}
	}

	return allowed
}

// limitRequestSize middleware limits HTTP request body size to prevent DoS attacks
func (s *Server) limitRequestSize(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Limit request body to 1MB
		const maxRequestSize = 1 << 20 // 1MB
		r.Body = http.MaxBytesReader(w, r.Body, maxRequestSize)
		next(w, r)
	}
}

// rateLimit middleware applies rate limiting to prevent brute force attacks
func (s *Server) rateLimit(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if s.RateLimiter != nil && !s.RateLimiter.Allow() {
			http.Error(w, "rate limit exceeded", http.StatusTooManyRequests)
			if s.Verbose {
				log.Printf("[security] rate limit exceeded from peer")
			}
			return
		}
		next(w, r)
	}
}

func (s *Server) handleStatus(w http.ResponseWriter, r *http.Request) {
	size, hits, misses, inflight := s.Cache.Stats()
	resp := protocol.Status{
		Backend:    s.Backend.Name(),
		CacheSize:  size,
		Hits:       hits,
		Misses:     misses,
		InFlight:   inflight,
		TTLSeconds: int(s.CacheTTL().Seconds()),
		SocketPath: s.SockPath,
	}

	// Add session information if session manager is available
	if s.Session != nil {
		sessionInfo := s.Session.GetInfo()
		resp.Session = &protocol.SessionStatus{
			State:         sessionInfo.State.String(),
			IdleTimeout:   int(sessionInfo.IdleTimeout.Seconds()),
			TimeUntilLock: int(sessionInfo.TimeUntilLock().Seconds()),
			Enabled:       sessionInfo.IdleTimeout > 0,
		}
	}

	_ = json.NewEncoder(w).Encode(resp)
}

func (s *Server) handleSessionUnlock(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if s.Session == nil {
		resp := protocol.SessionUnlockResponse{
			Success: false,
			State:   "disabled",
			Message: "Session management is disabled",
		}
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(resp)
		return
	}

	// Attempt to validate/unlock the session
	err := s.Session.ValidateSession(r.Context())
	sessionInfo := s.Session.GetInfo()

	resp := protocol.SessionUnlockResponse{
		Success: err == nil,
		State:   sessionInfo.State.String(),
	}

	if err != nil {
		resp.Message = fmt.Sprintf("Session unlock failed: %v", err)
		w.WriteHeader(http.StatusUnauthorized)
	} else {
		resp.Message = "Session unlocked successfully"
	}

	_ = json.NewEncoder(w).Encode(resp)
}

func (s *Server) handleRead(w http.ResponseWriter, r *http.Request) {
	var req protocol.ReadRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "bad json", http.StatusBadRequest)
		return
	}
	ref := strings.TrimSpace(req.Ref)
	if ref == "" {
		http.Error(w, "ref required", http.StatusBadRequest)
		return
	}
	rr, err := s.readOneWithFlags(r.Context(), ref, req.Flags)
	if err != nil {
		if s.Verbose {
			log.Printf("read error for ref %q: %v", ref, err)
		}
		// Return the actual error message instead of generic "failed to read secret"
		http.Error(w, fmt.Sprintf("failed to read secret: %v", err), http.StatusBadGateway)
		return
	}
	_ = json.NewEncoder(w).Encode(rr)
}

func (s *Server) handleReads(w http.ResponseWriter, r *http.Request) {
	var req protocol.ReadsRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "bad json", http.StatusBadRequest)
		return
	}
	result := make(map[string]protocol.ReadResponse, len(req.Refs))
	for _, ref := range req.Refs {
		ref = strings.TrimSpace(ref)
		if ref == "" {
			continue
		}
		rr, err := s.readOneWithFlags(r.Context(), ref, req.Flags)
		if err != nil {
			if s.Verbose {
				log.Printf("batch read error for ref %q: %v", ref, err)
			}
			// record the detailed error in Value to return something; caller decides
			result[ref] = protocol.ReadResponse{Ref: ref, Value: fmt.Sprintf("ERROR: %v", err), FromCache: false, ExpiresIn: 0, ResolvedAt: time.Now().Unix()}
			continue
		}
		result[ref] = rr
	}
	_ = json.NewEncoder(w).Encode(protocol.ReadsResponse{Results: result})
}

func (s *Server) handleResolve(w http.ResponseWriter, r *http.Request) {
	var req protocol.ResolveRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "bad json", http.StatusBadRequest)
		return
	}
	out := make(map[string]string, len(req.Env))
	for name, ref := range req.Env {
		rr, err := s.readOneWithFlags(r.Context(), ref, req.Flags)
		if err != nil {
			if s.Verbose {
				log.Printf("resolve error for %s (ref %q): %v", name, ref, err)
			}
			http.Error(w, fmt.Sprintf("resolve %s: %v", name, err), http.StatusBadGateway)
			return
		}
		out[name] = rr.Value
	}
	_ = json.NewEncoder(w).Encode(protocol.ResolveResponse{Env: out})
}

func (s *Server) readOne(ctx context.Context, ref string) (protocol.ReadResponse, error) {
	return s.readOneWithFlags(ctx, ref, nil)
}

func (s *Server) readOneWithFlags(ctx context.Context, ref string, flags []string) (protocol.ReadResponse, error) {
	// Check access policy if peer information is available
	if peerInfo, hasPeer := ctx.Value(peerInfoKey).(security.PeerInfo); hasPeer {
		if !s.validateAccess(peerInfo, ref) {
			return protocol.ReadResponse{}, fmt.Errorf("access denied by policy")
		}
	}

	// Create cache key that includes flags for proper cache isolation
	cacheKey := ref
	if len(flags) > 0 {
		cacheKey = ref + "|flags:" + strings.Join(flags, ",")
	}

	// Cache check
	if v, ok, exp, cached := s.Cache.Get(cacheKey); ok {
		s.Cache.IncHit()
		return protocol.ReadResponse{Ref: ref, Value: v, FromCache: true, ExpiresIn: int(time.Until(exp).Seconds()), ResolvedAt: cached.Unix()}, nil
	}
	s.Cache.IncMiss()
	s.Cache.IncInFlight()
	defer s.Cache.DecInFlight()

	vIF, err, _ := s.sf.Do(cacheKey, func() (interface{}, error) {
		// Re-check inside singleflight to avoid thundering herd
		if v, ok, exp, cached := s.Cache.Get(cacheKey); ok {
			s.Cache.IncHit()
			return protocol.ReadResponse{Ref: ref, Value: v, FromCache: true, ExpiresIn: int(time.Until(exp).Seconds()), ResolvedAt: cached.Unix()}, nil
		}
		// Read via backend
		ctx2, cancel := context.WithTimeout(ctx, 20*time.Second)
		defer cancel()
		v, err := s.Backend.ReadRefWithFlags(ctx2, ref, flags)
		if err != nil {
			return nil, err
		}
		s.Cache.Set(cacheKey, v)
		return protocol.ReadResponse{Ref: ref, Value: v, FromCache: false, ExpiresIn: int(s.CacheTTL().Seconds()), ResolvedAt: time.Now().Unix()}, nil
	})
	if err != nil {
		return protocol.ReadResponse{}, err
	}
	rr, ok := vIF.(protocol.ReadResponse)
	if !ok {
		return protocol.ReadResponse{}, errors.New("internal type assertion failed")
	}
	return rr, nil
}
