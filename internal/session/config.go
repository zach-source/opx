package session

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/zach-source/opx/internal/util"
)

// DefaultIdleTimeout is the default session idle timeout (8 hours)
const DefaultIdleTimeout = 8 * time.Hour

// Config holds session management configuration
type Config struct {
	// SessionIdleTimeout is the duration after which an idle session will be locked
	SessionIdleTimeout time.Duration `json:"session_idle_timeout"`
	// EnableSessionLock enables/disables the session locking feature
	EnableSessionLock bool `json:"enable_session_lock"`
	// LockOnAuthFailure locks the session when authentication failures occur
	LockOnAuthFailure bool `json:"lock_on_auth_failure"`
	// CheckInterval is how often to check for idle timeout (internal use)
	CheckInterval time.Duration `json:"check_interval,omitempty"`
}

// DefaultConfig returns a configuration with sensible defaults
func DefaultConfig() *Config {
	return &Config{
		SessionIdleTimeout: DefaultIdleTimeout,
		EnableSessionLock:  true,
		LockOnAuthFailure:  true,
		CheckInterval:      time.Minute, // Check every minute
	}
}

// LoadConfig loads configuration from environment variables, config file, and defaults
func LoadConfig() (*Config, error) {
	config := DefaultConfig()

	// Try to load from config file
	if err := config.loadFromFile(); err != nil {
		// Config file errors are not fatal, we'll use defaults/env vars
		// Only log if it's not a "file not found" error
		if !os.IsNotExist(err) {
			// Could log warning here if logger was available
		}
	}

	// Override with environment variables
	config.loadFromEnv()

	// Validate the configuration
	if err := config.validate(); err != nil {
		return nil, err
	}

	return config, nil
}

// LoadConfigFromFile loads configuration from a specific file path
func LoadConfigFromFile(filePath string) (*Config, error) {
	config := DefaultConfig()

	data, err := os.ReadFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}

	if err := json.Unmarshal(data, config); err != nil {
		return nil, fmt.Errorf("failed to parse config file: %w", err)
	}

	// Still apply environment variable overrides
	config.loadFromEnv()

	// Validate the configuration
	if err := config.validate(); err != nil {
		return nil, err
	}

	return config, nil
}

// loadFromFile loads configuration from XDG config directory
func (c *Config) loadFromFile() error {
	configDir, err := util.ConfigDir()
	if err != nil {
		return err
	}

	configPath := filepath.Join(configDir, "config.json")
	data, err := os.ReadFile(configPath)
	if err != nil {
		return err
	}

	return json.Unmarshal(data, c)
}

// loadFromEnv loads configuration from environment variables
func (c *Config) loadFromEnv() {
	if timeout := os.Getenv("OPX_SESSION_IDLE_TIMEOUT"); timeout != "" {
		if d, err := time.ParseDuration(timeout); err == nil {
			c.SessionIdleTimeout = d
		}
	}

	if lock := os.Getenv("OPX_ENABLE_SESSION_LOCK"); lock != "" {
		c.EnableSessionLock = lock == "true" || lock == "1"
	}

	if lockOnFail := os.Getenv("OPX_LOCK_ON_AUTH_FAILURE"); lockOnFail != "" {
		c.LockOnAuthFailure = lockOnFail == "true" || lockOnFail == "1"
	}
}

// ClampCacheTTL lowers a cache TTL that would outlive the session lock window,
// and returns the TTL to actually use. Nothing should stay hot past the point
// the session would have locked and cleared it.
//
// The lock window is the operator's control, so it wins: a short
// --session-timeout shortens the cache rather than the cache lengthening the
// lock. At the defaults (4h TTL, 8h idle timeout) nothing is clamped, so one
// unlock still covers a full 4h block.
//
// A timeout of 0 means "never lock" (see Manager.checkIdleTimeout), so there is
// no window to stay inside and the TTL is left alone.
func (c *Config) ClampCacheTTL(ttl time.Duration) time.Duration {
	if !c.EnableSessionLock || c.SessionIdleTimeout == 0 || ttl <= c.SessionIdleTimeout {
		return ttl
	}
	return c.SessionIdleTimeout
}

// validate ensures the configuration is valid
func (c *Config) validate() error {
	if c.SessionIdleTimeout < 0 {
		return errors.New("session idle timeout cannot be negative")
	}

	if c.EnableSessionLock && c.SessionIdleTimeout == 0 {
		return errors.New("session idle timeout must be greater than 0 when session lock is enabled")
	}

	if c.CheckInterval <= 0 {
		c.CheckInterval = time.Minute // Default to 1 minute
	}

	return nil
}

// SaveConfig saves the configuration to XDG config directory
func (c *Config) SaveConfig() error {
	configDir, err := util.ConfigDir()
	if err != nil {
		return err
	}

	configPath := filepath.Join(configDir, "config.json")
	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(configPath, data, 0o600)
}
