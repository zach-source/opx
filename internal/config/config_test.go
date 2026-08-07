package config

import (
	"os"
	"testing"
)

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()
	if cfg == nil {
		t.Fatal("DefaultConfig() returned nil")
	}

	// Executable paths should be empty by default (auto-discovered)
	if cfg.ExecutablePaths.Op != "" {
		t.Errorf("Expected empty Op path, got %s", cfg.ExecutablePaths.Op)
	}
	if cfg.ExecutablePaths.Vault != "" {
		t.Errorf("Expected empty Vault path, got %s", cfg.ExecutablePaths.Vault)
	}
	if cfg.ExecutablePaths.Bao != "" {
		t.Errorf("Expected empty Bao path, got %s", cfg.ExecutablePaths.Bao)
	}
}

func TestLoadConfigFromEnv(t *testing.T) {
	// Save original env vars
	origOp := os.Getenv("OPX_OP_PATH")
	origVault := os.Getenv("OPX_VAULT_PATH")
	origBao := os.Getenv("OPX_BAO_PATH")
	defer func() {
		os.Setenv("OPX_OP_PATH", origOp)
		os.Setenv("OPX_VAULT_PATH", origVault)
		os.Setenv("OPX_BAO_PATH", origBao)
	}()

	// Set test values
	os.Setenv("OPX_OP_PATH", "/custom/op")
	os.Setenv("OPX_VAULT_PATH", "/custom/vault")
	os.Setenv("OPX_BAO_PATH", "/custom/bao")

	cfg := DefaultConfig()
	cfg.loadFromEnv()

	if cfg.ExecutablePaths.Op != "/custom/op" {
		t.Errorf("Expected Op path '/custom/op', got %s", cfg.ExecutablePaths.Op)
	}
	if cfg.ExecutablePaths.Vault != "/custom/vault" {
		t.Errorf("Expected Vault path '/custom/vault', got %s", cfg.ExecutablePaths.Vault)
	}
	if cfg.ExecutablePaths.Bao != "/custom/bao" {
		t.Errorf("Expected Bao path '/custom/bao', got %s", cfg.ExecutablePaths.Bao)
	}
}

func TestGetOpPath(t *testing.T) {
	cfg := DefaultConfig()

	// Test with empty path (should error)
	_, err := cfg.GetOpPath()
	if err == nil {
		t.Error("Expected error when op path is empty")
	}

	// Test with configured path
	cfg.ExecutablePaths.Op = "/test/op"
	path, err := cfg.GetOpPath()
	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}
	if path != "/test/op" {
		t.Errorf("Expected '/test/op', got %s", path)
	}
}

func TestSaveAndLoadConfig(t *testing.T) {
	// SaveConfig writes to the XDG config dir. Point that at a temp dir, or the
	// test overwrites the developer's real daemon.json with /test/op.
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	cfg := DefaultConfig()
	cfg.ExecutablePaths.Op = "/test/op"
	cfg.ExecutablePaths.Vault = "/test/vault"

	if err := cfg.SaveConfig(); err != nil {
		t.Fatalf("SaveConfig failed: %v", err)
	}

	loaded, err := LoadConfig()
	if err != nil {
		t.Fatalf("LoadConfig failed: %v", err)
	}
	if loaded.ExecutablePaths.Op != "/test/op" {
		t.Errorf("Op = %q, want /test/op", loaded.ExecutablePaths.Op)
	}
	if loaded.ExecutablePaths.Vault != "/test/vault" {
		t.Errorf("Vault = %q, want /test/vault", loaded.ExecutablePaths.Vault)
	}
}

func TestResolveExecutablePaths(t *testing.T) {
	cfg := DefaultConfig()

	// Before resolution, paths should be empty
	if cfg.ExecutablePaths.Op != "" {
		t.Errorf("Expected empty Op path before resolution, got %s", cfg.ExecutablePaths.Op)
	}

	// After resolution, paths should be set if executables exist
	err := cfg.resolveExecutablePaths()
	if err != nil {
		t.Errorf("resolveExecutablePaths failed: %v", err)
	}

	// We can't assert specific paths since they depend on the test environment
	// But we can verify the function doesn't error
	t.Logf("Resolved paths: op=%s, vault=%s, bao=%s",
		cfg.ExecutablePaths.Op, cfg.ExecutablePaths.Vault, cfg.ExecutablePaths.Bao)
}
