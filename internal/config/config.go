package config

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/zach-source/opx/internal/util"
)

// Config holds daemon configuration
type Config struct {
	// ExecutablePaths specifies custom paths to external binaries
	ExecutablePaths ExecutablePaths `json:"executable_paths"`
}

// ExecutablePaths holds paths to external executables
type ExecutablePaths struct {
	Op    string `json:"op,omitempty"`    // Path to 1Password CLI
	Vault string `json:"vault,omitempty"` // Path to HashiCorp Vault CLI
	Bao   string `json:"bao,omitempty"`   // Path to OpenBao CLI
}

// DefaultConfig returns a configuration with sensible defaults
func DefaultConfig() *Config {
	return &Config{
		ExecutablePaths: ExecutablePaths{
			// Paths will be auto-discovered via exec.LookPath if not set
		},
	}
}

// LoadConfig loads configuration from environment variables, config file, and defaults
func LoadConfig() (*Config, error) {
	config := DefaultConfig()

	// Try to load from config file
	if err := config.loadFromFile(); err != nil {
		// Config file errors are not fatal, we'll use defaults/env vars
		if !os.IsNotExist(err) {
			// Could log warning here if logger was available
		}
	}

	// Override with environment variables
	config.loadFromEnv()

	// Validate and resolve paths
	if err := config.resolveExecutablePaths(); err != nil {
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

	// Validate and resolve paths
	if err := config.resolveExecutablePaths(); err != nil {
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

	configPath := filepath.Join(configDir, "daemon.json")
	data, err := os.ReadFile(configPath)
	if err != nil {
		return err
	}

	return json.Unmarshal(data, c)
}

// loadFromEnv loads configuration from environment variables
func (c *Config) loadFromEnv() {
	if opPath := os.Getenv("OPX_OP_PATH"); opPath != "" {
		c.ExecutablePaths.Op = opPath
	}
	if vaultPath := os.Getenv("OPX_VAULT_PATH"); vaultPath != "" {
		c.ExecutablePaths.Vault = vaultPath
	}
	if baoPath := os.Getenv("OPX_BAO_PATH"); baoPath != "" {
		c.ExecutablePaths.Bao = baoPath
	}
}

// resolveExecutablePaths finds executables in PATH if not explicitly configured
func (c *Config) resolveExecutablePaths() error {
	// Resolve op path
	if c.ExecutablePaths.Op == "" {
		if path, err := exec.LookPath("op"); err == nil {
			c.ExecutablePaths.Op = path
		}
		// Not an error if op is not found - user might not use 1Password backend
	}

	// Resolve vault path
	if c.ExecutablePaths.Vault == "" {
		if path, err := exec.LookPath("vault"); err == nil {
			c.ExecutablePaths.Vault = path
		}
	}

	// Resolve bao path
	if c.ExecutablePaths.Bao == "" {
		if path, err := exec.LookPath("bao"); err == nil {
			c.ExecutablePaths.Bao = path
		}
	}

	return nil
}

// GetOpPath returns the configured or discovered path to op CLI
func (c *Config) GetOpPath() (string, error) {
	if c.ExecutablePaths.Op == "" {
		return "", fmt.Errorf("1Password CLI (op) not found in PATH and no custom path configured")
	}
	return c.ExecutablePaths.Op, nil
}

// GetVaultPath returns the configured or discovered path to vault CLI
func (c *Config) GetVaultPath() (string, error) {
	if c.ExecutablePaths.Vault == "" {
		return "", fmt.Errorf("HashiCorp Vault CLI (vault) not found in PATH and no custom path configured")
	}
	return c.ExecutablePaths.Vault, nil
}

// GetBaoPath returns the configured or discovered path to bao CLI
func (c *Config) GetBaoPath() (string, error) {
	if c.ExecutablePaths.Bao == "" {
		return "", fmt.Errorf("OpenBao CLI (bao) not found in PATH and no custom path configured")
	}
	return c.ExecutablePaths.Bao, nil
}

// SaveConfig saves the configuration to XDG config directory
func (c *Config) SaveConfig() error {
	configDir, err := util.ConfigDir()
	if err != nil {
		return err
	}

	configPath := filepath.Join(configDir, "daemon.json")
	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(configPath, data, 0o600)
}
