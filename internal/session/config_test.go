package session

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestDefaultConfig(t *testing.T) {
	config := DefaultConfig()

	if config.SessionIdleTimeout != DefaultIdleTimeout {
		t.Errorf("Expected default timeout %v, got %v", DefaultIdleTimeout, config.SessionIdleTimeout)
	}

	if !config.EnableSessionLock {
		t.Error("Expected session lock to be enabled by default")
	}

	if !config.LockOnAuthFailure {
		t.Error("Expected lock on auth failure to be enabled by default")
	}

	if config.CheckInterval != time.Minute {
		t.Errorf("Expected check interval %v, got %v", time.Minute, config.CheckInterval)
	}
}

func TestConfig_validate(t *testing.T) {
	tests := []struct {
		name      string
		config    *Config
		expectErr bool
	}{
		{
			name:      "valid config",
			config:    DefaultConfig(),
			expectErr: false,
		},
		{
			name: "negative timeout",
			config: &Config{
				SessionIdleTimeout: -1 * time.Hour,
				EnableSessionLock:  true,
			},
			expectErr: true,
		},
		{
			name: "session lock enabled but zero timeout",
			config: &Config{
				SessionIdleTimeout: 0,
				EnableSessionLock:  true,
			},
			expectErr: true,
		},
		{
			name: "session lock disabled with zero timeout",
			config: &Config{
				SessionIdleTimeout: 0,
				EnableSessionLock:  false,
			},
			expectErr: false,
		},
		{
			name: "zero check interval gets default",
			config: &Config{
				SessionIdleTimeout: 1 * time.Hour,
				EnableSessionLock:  true,
				CheckInterval:      0,
			},
			expectErr: false,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := test.config.validate()
			if test.expectErr && err == nil {
				t.Error("Expected validation error but got none")
			}
			if !test.expectErr && err != nil {
				t.Errorf("Expected no validation error but got: %v", err)
			}

			// Check that zero check interval gets set to default
			if test.config.CheckInterval == 0 && err == nil {
				if test.config.CheckInterval != time.Minute {
					t.Errorf("Expected check interval to be set to %v, got %v", time.Minute, test.config.CheckInterval)
				}
			}
		})
	}
}

func TestConfig_loadFromEnv(t *testing.T) {
	// Save original env vars
	originalTimeout := os.Getenv("OPX_SESSION_IDLE_TIMEOUT")
	originalEnable := os.Getenv("OPX_ENABLE_SESSION_LOCK")
	originalLockOnFail := os.Getenv("OPX_LOCK_ON_AUTH_FAILURE")

	defer func() {
		// Restore original env vars
		setEnv("OPX_SESSION_IDLE_TIMEOUT", originalTimeout)
		setEnv("OPX_ENABLE_SESSION_LOCK", originalEnable)
		setEnv("OPX_LOCK_ON_AUTH_FAILURE", originalLockOnFail)
	}()

	tests := []struct {
		name             string
		envTimeout       string
		envEnable        string
		envLockOnFail    string
		expectedTimeout  time.Duration
		expectedEnable   bool
		expectedLockFail bool
	}{
		{
			name:             "valid timeout",
			envTimeout:       "2h",
			envEnable:        "true",
			envLockOnFail:    "false",
			expectedTimeout:  2 * time.Hour,
			expectedEnable:   true,
			expectedLockFail: false,
		},
		{
			name:             "enable with 1",
			envTimeout:       "",
			envEnable:        "1",
			envLockOnFail:    "1",
			expectedTimeout:  DefaultIdleTimeout,
			expectedEnable:   true,
			expectedLockFail: true,
		},
		{
			name:             "disable with false",
			envTimeout:       "",
			envEnable:        "false",
			envLockOnFail:    "false",
			expectedTimeout:  DefaultIdleTimeout,
			expectedEnable:   false,
			expectedLockFail: false,
		},
		{
			name:             "invalid timeout ignored",
			envTimeout:       "invalid",
			envEnable:        "",
			envLockOnFail:    "",
			expectedTimeout:  DefaultIdleTimeout,
			expectedEnable:   true,
			expectedLockFail: true,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			// Set environment variables
			setEnv("OPX_SESSION_IDLE_TIMEOUT", test.envTimeout)
			setEnv("OPX_ENABLE_SESSION_LOCK", test.envEnable)
			setEnv("OPX_LOCK_ON_AUTH_FAILURE", test.envLockOnFail)

			config := DefaultConfig()
			config.loadFromEnv()

			if config.SessionIdleTimeout != test.expectedTimeout {
				t.Errorf("Expected timeout %v, got %v", test.expectedTimeout, config.SessionIdleTimeout)
			}

			if config.EnableSessionLock != test.expectedEnable {
				t.Errorf("Expected enable %t, got %t", test.expectedEnable, config.EnableSessionLock)
			}

			if config.LockOnAuthFailure != test.expectedLockFail {
				t.Errorf("Expected lock on failure %t, got %t", test.expectedLockFail, config.LockOnAuthFailure)
			}
		})
	}
}

func TestConfig_SaveAndLoadFromFile(t *testing.T) {
	// Create a temporary directory
	tmpDir := t.TempDir()

	// Create subdirectories to mimic StateDir behavior
	if err := os.MkdirAll(tmpDir, 0o700); err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}

	// We'll manually construct the config path since we can't easily mock util.StateDir
	configPath := filepath.Join(tmpDir, "config.json")

	// Create a config to save
	config := &Config{
		SessionIdleTimeout: 4 * time.Hour,
		EnableSessionLock:  false,
		LockOnAuthFailure:  true,
		CheckInterval:      30 * time.Second,
	}

	// Manually save the config to our test path
	data, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		t.Fatalf("Failed to marshal config: %v", err)
	}

	if err := os.WriteFile(configPath, data, 0o600); err != nil {
		t.Fatalf("Failed to write config file: %v", err)
	}

	// Verify file was created
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		t.Fatal("Config file was not created")
	}

	// Load config from file by reading directly (since we can't mock util.StateDir easily)
	data, err = os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("Failed to read config file: %v", err)
	}

	newConfig := DefaultConfig()
	if err := json.Unmarshal(data, newConfig); err != nil {
		t.Fatalf("Failed to unmarshal config: %v", err)
	}

	// Verify loaded config matches saved config
	if newConfig.SessionIdleTimeout != config.SessionIdleTimeout {
		t.Errorf("Expected timeout %v, got %v", config.SessionIdleTimeout, newConfig.SessionIdleTimeout)
	}

	if newConfig.EnableSessionLock != config.EnableSessionLock {
		t.Errorf("Expected enable %t, got %t", config.EnableSessionLock, newConfig.EnableSessionLock)
	}

	if newConfig.LockOnAuthFailure != config.LockOnAuthFailure {
		t.Errorf("Expected lock on failure %t, got %t", config.LockOnAuthFailure, newConfig.LockOnAuthFailure)
	}
}

func TestLoadConfig(t *testing.T) {
	// Test that LoadConfig doesn't fail even when no config file exists
	config, err := LoadConfig()
	if err != nil {
		t.Errorf("LoadConfig failed: %v", err)
	}

	if config == nil {
		t.Error("Expected config to be returned")
	}

	// Should have default values
	if config.SessionIdleTimeout != DefaultIdleTimeout {
		t.Errorf("Expected default timeout, got %v", config.SessionIdleTimeout)
	}
}

// Helper function to set environment variables
func setEnv(key, value string) {
	if value == "" {
		os.Unsetenv(key)
	} else {
		os.Setenv(key, value)
	}
}
