package main

import (
	"context"
	"flag"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/zach-source/opx/internal/backend"
	"github.com/zach-source/opx/internal/cache"
	"github.com/zach-source/opx/internal/server"
	"github.com/zach-source/opx/internal/session"
)

func main() {
	var ttlSec int
	var sock string
	var verbose bool
	var backendName string
	var sessionTimeout int
	var enableSessionLock bool
	var lockOnAuthFailure bool

	flag.IntVar(&ttlSec, "ttl", 120, "cache TTL seconds")
	flag.StringVar(&sock, "sock", "", "unix socket path (default: XDG data dir or ~/.op-authd/socket.sock)")
	flag.BoolVar(&verbose, "verbose", true, "verbose logging")
	flag.StringVar(&backendName, "backend", "opcli", "backend: opcli|fake")
	flag.IntVar(&sessionTimeout, "session-timeout", int(session.DefaultIdleTimeout.Hours()), "session idle timeout in hours (0 to disable)")
	flag.BoolVar(&enableSessionLock, "enable-session-lock", true, "enable session idle timeout and locking")
	flag.BoolVar(&lockOnAuthFailure, "lock-on-auth-failure", true, "lock session on authentication failures")
	flag.Parse()

	// Load session configuration from environment/file, then override with flags
	sessionConfig, err := session.LoadConfig()
	if err != nil {
		log.Printf("Warning: failed to load session config: %v, using defaults", err)
		sessionConfig = session.DefaultConfig()
	}

	// Override config with command-line flags
	sessionConfig.SessionIdleTimeout = time.Duration(sessionTimeout) * time.Hour
	sessionConfig.EnableSessionLock = enableSessionLock
	sessionConfig.LockOnAuthFailure = lockOnAuthFailure

	// Create session manager
	var sessionManager *session.Manager
	if enableSessionLock {
		sessionManager = session.NewManager(sessionConfig)
		if verbose {
			sessionManager.SetVerbose(true)
		}
	}

	// Create backend (potentially session-aware)
	var be backend.Backend
	switch backendName {
	case "opcli":
		if sessionManager != nil {
			be = backend.NewSessionAwareOpCLI(sessionManager)
		} else {
			be = backend.OpCLI{}
		}
	case "fake":
		if sessionManager != nil {
			be = backend.NewSessionAwareFake(sessionManager)
		} else {
			be = backend.Fake{}
		}
	default:
		log.Fatalf("unknown backend: %s", backendName)
	}

	srv := &server.Server{
		SockPath: sock,
		Backend:  be,
		Cache:    cache.New(time.Duration(ttlSec) * time.Second),
		Session:  sessionManager,
		Verbose:  verbose,
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if err := srv.Serve(ctx); err != nil {
		log.Fatalf("server error: %v", err)
	}
}
