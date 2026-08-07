package server

import (
	"context"
	"strings"
	"time"

	"github.com/zach-source/opx/internal/backend"
)

// MinRevalidateInterval floors the revalidation period. Each pass shells out to
// the backend once per cached entry, so a short interval would hammer the CLI
// (and the 1Password service) for no benefit.
const MinRevalidateInterval = time.Minute

const cacheKeyFlagSep = "|flags:"

// cacheKeyFor builds the cache key for a ref. Flags are part of the key because
// the same ref read under different --account flags is a different secret.
func cacheKeyFor(ref string, flags []string) string {
	if len(flags) == 0 {
		return ref
	}
	return ref + cacheKeyFlagSep + strings.Join(flags, ",")
}

// splitCacheKey reverses cacheKeyFor.
func splitCacheKey(key string) (ref string, flags []string) {
	ref, rest, found := strings.Cut(key, cacheKeyFlagSep)
	if !found || rest == "" {
		return ref, nil
	}
	return ref, strings.Split(rest, ",")
}

// revalidateBackend returns the backend with any session wrapper peeled off.
//
// Revalidation is strictly read-only and must not touch session state: going
// through the wrapper would call UpdateActivity on every pass, so an idle daemon
// would keep renewing its own session and never lock.
func (s *Server) revalidateBackend() backend.Backend {
	if u, ok := s.Backend.(interface{ Unwrap() backend.Backend }); ok {
		return u.Unwrap()
	}
	return s.Backend
}

// revalidationSafe reports whether it is a good moment to talk to the backend.
// A locked or expired session means an `op read` would raise an interactive
// prompt, and prompting from a background timer is exactly what this daemon
// exists to avoid.
//
// Only RequiresUnlock blocks a pass. SessionUnknown is the ordinary steady state
// for a daemon nobody has explicitly authenticated - gating on IsActive() here
// would mean the revalidator silently never ran.
func (s *Server) revalidationSafe() bool {
	if s.Session == nil {
		return true
	}
	return !s.Session.GetInfo().State.RequiresUnlock()
}

// startRevalidator periodically re-reads cached secrets and corrects any that
// changed upstream. Opt-in: callers pass 0 to disable it.
//
// This closes the gap that `opx invalidate` cannot: a rotation performed on
// another machine, by a scheduled job, or in the 1Password web UI is invisible
// to this daemon until something re-reads the secret.
func (s *Server) startRevalidator(ctx context.Context, interval time.Duration) {
	if interval < MinRevalidateInterval {
		interval = MinRevalidateInterval
	}

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.revalidateOnce(ctx)
		}
	}
}

// revalidateOnce walks the cache once. Read-only by construction: the Backend
// interface has no write method, so this can only ever call ReadRefWithFlags.
func (s *Server) revalidateOnce(ctx context.Context) (checked, changed int) {
	if !s.revalidationSafe() {
		if s.Verbose && s.Logger != nil {
			s.Logger.Debugw("skipping revalidation, session is not active")
		}
		return 0, 0
	}

	be := s.revalidateBackend()

	// Snapshot first: the backend calls are slow and must not hold the cache lock.
	for _, rec := range s.Cache.Snapshot() {
		if ctx.Err() != nil {
			return checked, changed
		}

		ref, flags := splitCacheKey(rec.Key)

		readCtx, cancel := context.WithTimeout(ctx, 20*time.Second)
		val, err := be.ReadRefWithFlags(readCtx, ref, flags)
		cancel()

		if err != nil {
			// A transient failure must not evict a good entry: the cached value is
			// still the best answer we have, and dropping it would force a prompt.
			if s.Logger != nil {
				s.Logger.Warnw("revalidation read failed, keeping cached value", "ref", ref, "error", err)
			}
			continue
		}

		checked++
		if val == rec.Value {
			continue
		}

		// Refresh keeps the original expiry, so a rotated secret is corrected
		// without the entry living any longer than it otherwise would have.
		if s.Cache.Refresh(rec.Key, val) {
			changed++
			if s.Logger != nil {
				s.Logger.Infow("secret changed upstream, cache refreshed", "ref", ref)
			}
		}
	}

	if s.Verbose && s.Logger != nil {
		s.Logger.Debugw("revalidation pass complete", "checked", checked, "changed", changed)
	}
	return checked, changed
}
