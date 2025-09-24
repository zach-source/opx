package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"golang.org/x/time/rate"

	"github.com/zach-source/opx/internal/audit"
	"github.com/zach-source/opx/internal/backend"
	"github.com/zach-source/opx/internal/cache"
	"github.com/zach-source/opx/internal/policy"
	"github.com/zach-source/opx/internal/server"
	"github.com/zach-source/opx/internal/session"
)

// Version information (set via ldflags during build)
var (
	version = "dev"
	commit  = "unknown"
	date    = "unknown"
)

func main() {
	var ttlSec int
	var sock string
	var verbose bool
	var backendName string
	var sessionTimeout int
	var enableSessionLock bool
	var lockOnAuthFailure bool
	var enableAuditLog bool
	var auditLogRetentionDays int
	var showVersion bool
	var configFile string
	var policyFile string

	flag.IntVar(&ttlSec, "ttl", 120, "cache TTL seconds")
	flag.StringVar(&sock, "sock", "", "unix socket path (default: XDG data dir or ~/.op-authd/socket.sock)")
	flag.BoolVar(&verbose, "verbose", true, "verbose logging")
	flag.StringVar(&backendName, "backend", "opcli", "backend: opcli|fake|vault|bao|multi")
	flag.IntVar(&sessionTimeout, "session-timeout", int(session.DefaultIdleTimeout.Hours()), "session idle timeout in hours (0 to disable)")
	flag.BoolVar(&enableSessionLock, "enable-session-lock", true, "enable session idle timeout and locking")
	flag.BoolVar(&lockOnAuthFailure, "lock-on-auth-failure", true, "lock session on authentication failures")
	flag.BoolVar(&enableAuditLog, "enable-audit-log", false, "enable structured audit logging to file")
	flag.IntVar(&auditLogRetentionDays, "audit-log-retention-days", 30, "number of days to keep audit logs (0 = keep all)")
	flag.BoolVar(&showVersion, "version", false, "show version information and exit")
	flag.StringVar(&configFile, "config", "", "path to configuration file (overrides default locations)")
	flag.StringVar(&policyFile, "policy", "", "path to policy file (overrides default policy.json)")
	flag.Parse()

	if showVersion {
		fmt.Printf("opx-authd version: %s\n", version)
		if commit != "unknown" {
			fmt.Printf("  commit: %s\n", commit)
		}
		if date != "unknown" {
			fmt.Printf("  built: %s\n", date)
		}
		os.Exit(0)
	}

	// Load session configuration from custom file or default locations
	var sessionConfig *session.Config
	var err error
	if configFile != "" {
		sessionConfig, err = session.LoadConfigFromFile(configFile)
		if err != nil {
			log.Fatalf("Failed to load config from %s: %v", configFile, err)
		}
		if verbose {
			log.Printf("Loaded session config from %s", configFile)
		}
	} else {
		sessionConfig, err = session.LoadConfig()
		if err != nil {
			log.Printf("Warning: failed to load session config: %v, using defaults", err)
			sessionConfig = session.DefaultConfig()
		}
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

	// Create multi-account session manager
	var multiAccountSession *session.MultiAccountManager
	if enableSessionLock {
		multiAccountSession = session.NewMultiAccountManager(sessionConfig)
		if verbose {
			multiAccountSession.SetVerbose(true)
		}
	}

	// Create backend (potentially session-aware)
	var be backend.Backend
	switch backendName {
	case "opcli":
		if multiAccountSession != nil {
			be = backend.NewMultiAccountSessionAwareBackend(backend.OpCLI{}, multiAccountSession)
		} else if sessionManager != nil {
			be = backend.NewSessionAwareOpCLI(sessionManager)
		} else {
			be = backend.OpCLI{}
		}
	case "fake":
		if multiAccountSession != nil {
			be = backend.NewMultiAccountSessionAwareBackend(backend.Fake{}, multiAccountSession)
		} else if sessionManager != nil {
			be = backend.NewSessionAwareFake(sessionManager)
		} else {
			be = backend.Fake{}
		}
	case "vault":
		// TODO: Load vault config from file
		vaultConfig := backend.VaultConfig{
			Address:    "http://localhost:8200", // Default local Vault
			AuthMethod: "token",
		}
		be = backend.NewVault(vaultConfig)
	case "bao":
		// TODO: Load bao config from file
		baoConfig := backend.VaultConfig{
			Address:    "http://localhost:8300", // Default local Bao
			AuthMethod: "token",
		}
		be = backend.NewBao(baoConfig)
	case "multi":
		// Create multi-backend with all backends available
		var opBe backend.Backend = backend.OpCLI{}
		if multiAccountSession != nil {
			opBe = backend.NewMultiAccountSessionAwareBackend(backend.OpCLI{}, multiAccountSession)
		} else if sessionManager != nil {
			opBe = backend.NewSessionAwareOpCLI(sessionManager)
		}

		vaultBe := backend.NewVault(backend.VaultConfig{
			Address:    "http://localhost:8200",
			AuthMethod: "token",
		})
		baoBe := backend.NewBao(backend.VaultConfig{
			Address:    "http://localhost:8300",
			AuthMethod: "token",
		})
		be = backend.NewMultiBackend(opBe, vaultBe, baoBe, "op")
	default:
		log.Fatalf("unknown backend: %s", backendName)
	}

	// Load access policy from custom file or default location
	var accessPolicy policy.Policy
	var policyPath string
	if policyFile != "" {
		accessPolicy, err = policy.LoadFromFile(policyFile)
		if err != nil {
			log.Fatalf("Failed to load policy from %s: %v", policyFile, err)
		}
		policyPath = policyFile
		if verbose {
			log.Printf("Loaded access policy from %s", policyFile)
		}
	} else {
		accessPolicy, policyPath, err = policy.Load()
		if err != nil {
			log.Printf("Warning: failed to load access policy from %s: %v, using defaults", policyPath, err)
			accessPolicy = policy.Policy{Allow: []policy.Rule{}, DefaultDeny: false}
		} else if verbose {
			log.Printf("Loaded access policy from %s", policyPath)
		}
	}

	// Create audit logger with rotation configuration
	var auditLogger *audit.Logger
	if enableAuditLog {
		rollerConfig := audit.RollerConfig{
			MaxDays:       auditLogRetentionDays,
			CompressOld:   false,
			RotateOnStart: true,
			FlushInterval: 5 * time.Second,
		}
		auditLogger, err = audit.NewLoggerWithConfig(true, rollerConfig)
		if err != nil {
			log.Fatalf("Failed to create audit logger: %v", err)
		}
		defer auditLogger.Close()
	} else {
		auditLogger, err = audit.NewLogger(false)
		if err != nil {
			log.Fatalf("Failed to create audit logger: %v", err)
		}
	}

	if enableAuditLog && verbose {
		log.Printf("Audit logging enabled")
	}

	// Create rate limiter: 10 requests per second with burst of 5
	rateLimiter := rate.NewLimiter(rate.Every(100*time.Millisecond), 5)

	srv := &server.Server{
		SockPath:            sock,
		Backend:             be,
		Cache:               cache.New(time.Duration(ttlSec) * time.Second),
		Session:             sessionManager,
		MultiAccountSession: multiAccountSession,
		Policy:              accessPolicy,
		PolicyPath:          policyPath,
		AuditLogger:         auditLogger,
		RateLimiter:         rateLimiter,
		Verbose:             verbose,
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if err := srv.Serve(ctx); err != nil {
		log.Fatalf("server error: %v", err)
	}
}
