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
	"github.com/zach-source/opx/internal/config"
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

// createOpCLIBackend creates a 1Password CLI backend with optional session management
func createOpCLIBackend(cfg *config.Config, multiAccountSession *session.MultiAccountManager, sessionManager *session.Manager) backend.Backend {
	opPath, err := cfg.GetOpPath()
	if err != nil {
		log.Printf("[ERROR] Failed to find 1Password CLI: %v", err)
		log.Printf("[INFO] Falling back to fake backend for testing")
		return backend.Fake{}
	}

	opcli, err := backend.NewOpCLI(opPath)
	if err != nil {
		log.Printf("[ERROR] Failed to initialize 1Password CLI backend: %v", err)
		log.Printf("[INFO] Falling back to fake backend for testing")
		return backend.Fake{}
	}

	return wrapWithSessionManagement(opcli, multiAccountSession, sessionManager, opPath)
}

// createFakeBackend creates a fake backend with optional session management
func createFakeBackend(multiAccountSession *session.MultiAccountManager, sessionManager *session.Manager) backend.Backend {
	fake := backend.Fake{}
	return wrapWithSessionManagement(fake, multiAccountSession, sessionManager, "")
}

// createVaultBackend creates a Vault backend with optional session management
func createVaultBackend(cfg *config.Config, multiAccountSession *session.MultiAccountManager, sessionManager *session.Manager) backend.Backend {
	// TODO: Load vault config from file and use cfg.GetVaultPath()
	vaultConfig := backend.VaultConfig{
		Address:    "http://localhost:8200",
		AuthMethod: "token",
	}
	vault := backend.NewVault(vaultConfig)
	return wrapWithSessionManagement(vault, multiAccountSession, sessionManager, "")
}

// createBaoBackend creates a Bao backend with optional session management
func createBaoBackend(cfg *config.Config, multiAccountSession *session.MultiAccountManager, sessionManager *session.Manager) backend.Backend {
	// TODO: Load bao config from file and use cfg.GetBaoPath()
	baoConfig := backend.VaultConfig{
		Address:    "http://localhost:8300",
		AuthMethod: "token",
	}
	bao := backend.NewBao(baoConfig)
	return wrapWithSessionManagement(bao, multiAccountSession, sessionManager, "")
}

// createMultiBackend creates a multi-backend that routes by URI scheme
func createMultiBackend(cfg *config.Config, multiAccountSession *session.MultiAccountManager, sessionManager *session.Manager) backend.Backend {
	opBe := createOpCLIBackend(cfg, multiAccountSession, sessionManager)
	vaultBe := createVaultBackend(cfg, multiAccountSession, sessionManager)
	baoBe := createBaoBackend(cfg, multiAccountSession, sessionManager)

	return backend.NewMultiBackend(opBe, vaultBe, baoBe, "op")
}

// wrapWithSessionManagement wraps a backend with appropriate session management
func wrapWithSessionManagement(be backend.Backend, multiAccountSession *session.MultiAccountManager, sessionManager *session.Manager, opPath string) backend.Backend {
	if multiAccountSession != nil {
		return backend.NewMultiAccountSessionAwareBackend(be, multiAccountSession)
	}

	if sessionManager != nil {
		// For OpCLI backends, use the specialized session-aware wrapper
		if opPath != "" {
			sessionAware, err := backend.NewSessionAwareOpCLI(sessionManager, opPath)
			if err != nil {
				log.Printf("[ERROR] Failed to initialize session-aware OpCLI: %v", err)
				return backend.Fake{}
			}
			return sessionAware
		}
		// For fake backend, use the specialized fake wrapper
		if _, isFake := be.(backend.Fake); isFake {
			return backend.NewSessionAwareFake(sessionManager)
		}
		// For other backends (Vault, Bao)
		return backend.NewSessionAwareBackend(be, sessionManager)
	}

	return be
}

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
	flag.BoolVar(&verbose, "verbose", false, "verbose logging")
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

	if sock == "" {
		sock = os.Getenv("OPX_SOCKET_PATH")
	}

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

	// Load daemon configuration
	daemonConfig, err := config.LoadConfig()
	if err != nil {
		log.Printf("Warning: failed to load daemon config: %v, using defaults", err)
		daemonConfig = config.DefaultConfig()
	}

	// Load session configuration from custom file or default locations
	var sessionConfig *session.Config
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
		be = createOpCLIBackend(daemonConfig, multiAccountSession, sessionManager)
	case "fake":
		be = createFakeBackend(multiAccountSession, sessionManager)
	case "vault":
		be = createVaultBackend(daemonConfig, multiAccountSession, sessionManager)
	case "bao":
		be = createBaoBackend(daemonConfig, multiAccountSession, sessionManager)
	case "multi":
		be = createMultiBackend(daemonConfig, multiAccountSession, sessionManager)
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
		Version:             version,
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if err := srv.Serve(ctx); err != nil {
		log.Fatalf("server error: %v", err)
	}
}
