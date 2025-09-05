package session

import (
	"encoding/json"
	"errors"
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

// loadFromFile loads configuration from ~/.op-authd/config.json
func (c *Config) loadFromFile() error {
	stateDir, err := util.StateDir()
	if err != nil {
		return err
	}

	configPath := filepath.Join(stateDir, "config.json")
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

// SaveConfig saves the configuration to ~/.op-authd/config.json
func (c *Config) SaveConfig() error {
	stateDir, err := util.StateDir()
	if err != nil {
		return err
	}

	configPath := filepath.Join(stateDir, "config.json")
	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(configPath, data, 0o600)
}
