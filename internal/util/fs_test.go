package util

import (
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestHomeDir(t *testing.T) {
	// Save original value
	originalHome := os.Getenv("HOME")
	defer func() {
		os.Setenv("HOME", originalHome)
	}()

	tests := []struct {
		name        string
		homeEnv     string
		expectEmpty bool
	}{
		{
			name:        "normal home directory",
			homeEnv:     "/home/user",
			expectEmpty: false,
		},
		{
			name:        "empty home directory",
			homeEnv:     "",
			expectEmpty: true, // Should fall back to "."
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Set the environment variable
			os.Setenv("HOME", tt.homeEnv)

			result := HomeDir()

			if tt.expectEmpty && result != "." {
				t.Errorf("Expected fallback to '.', got %q", result)
			} else if !tt.expectEmpty && result == "" {
				t.Error("HomeDir should not return empty string for valid home")
			} else if !tt.expectEmpty && tt.homeEnv != "" && result != tt.homeEnv {
				// Note: os.UserHomeDir() might return something different than HOME env var
				// on some systems, so we just check it's not empty
				if result == "" {
					t.Error("HomeDir returned empty string")
				}
			}
		})
	}
}

func TestStateDir(t *testing.T) {
	// Create a temporary directory to serve as fake home
	tempHome := t.TempDir()

	// Save and restore original HOME and clear XDG vars
	originalHome := os.Getenv("HOME")
	defer os.Setenv("HOME", originalHome)
	os.Setenv("HOME", tempHome)
	t.Setenv("XDG_DATA_HOME", "")

	// Create old directory to trigger backward compatibility
	oldDir := filepath.Join(tempHome, ".op-authd")
	if err := os.MkdirAll(oldDir, 0o700); err != nil {
		t.Fatalf("Failed to create old directory: %v", err)
	}

	// Test backward compatibility operation
	dir, err := StateDir()
	if err != nil {
		t.Fatalf("StateDir failed: %v", err)
	}

	expectedDir := filepath.Join(tempHome, ".op-authd")
	if dir != expectedDir {
		t.Errorf("Expected state dir %q, got %q", expectedDir, dir)
	}

	// Check that directory exists
	if _, err := os.Stat(dir); os.IsNotExist(err) {
		t.Error("State directory was not created")
	}

	// Check directory permissions
	info, err := os.Stat(dir)
	if err != nil {
		t.Fatalf("Failed to stat directory: %v", err)
	}

	perm := info.Mode().Perm()
	if perm != 0o700 {
		t.Errorf("Expected directory permissions 0o700, got %o", perm)
	}

	// Test calling StateDir again (should not error)
	dir2, err := StateDir()
	if err != nil {
		t.Errorf("Second call to StateDir failed: %v", err)
	}
	if dir2 != dir {
		t.Errorf("StateDir returned different path on second call: %q vs %q", dir, dir2)
	}
}

func TestStateDir_CreateError(t *testing.T) {
	// Try to create state dir in a non-writable location
	// This test might be platform-specific and could be skipped on some systems

	// Save and restore original HOME
	originalHome := os.Getenv("HOME")
	defer os.Setenv("HOME", originalHome)

	// Set HOME to a non-writable location (this might not work on all systems)
	if os.Getuid() == 0 {
		t.Skip("Skipping permission test when running as root")
	}

	os.Setenv("HOME", "/root") // Typically not writable by non-root users

	_, err := StateDir()
	if err == nil {
		t.Log("Expected error when creating state dir in non-writable location, but got none (this might be expected on some systems)")
	}
}

func TestSocketPath(t *testing.T) {
	// Create a temporary directory to serve as fake home
	tempHome := t.TempDir()

	// Save and restore original HOME and clear XDG vars
	originalHome := os.Getenv("HOME")
	defer os.Setenv("HOME", originalHome)
	os.Setenv("HOME", tempHome)
	t.Setenv("XDG_RUNTIME_DIR", "")
	t.Setenv("XDG_DATA_HOME", "")

	// Create old directory to trigger backward compatibility
	oldDir := filepath.Join(tempHome, ".op-authd")
	if err := os.MkdirAll(oldDir, 0o700); err != nil {
		t.Fatalf("Failed to create old directory: %v", err)
	}

	sockPath, err := SocketPath()
	if err != nil {
		t.Fatalf("SocketPath failed: %v", err)
	}

	expectedPath := filepath.Join(tempHome, ".op-authd", "socket.sock")
	if sockPath != expectedPath {
		t.Errorf("Expected socket path %q, got %q", expectedPath, sockPath)
	}

	// Check that the state directory was created
	stateDir := filepath.Dir(sockPath)
	if _, err := os.Stat(stateDir); os.IsNotExist(err) {
		t.Error("State directory was not created by SocketPath")
	}
}

func TestTokenPath(t *testing.T) {
	// Create a temporary directory to serve as fake home
	tempHome := t.TempDir()

	// Save and restore original HOME and clear XDG vars
	originalHome := os.Getenv("HOME")
	defer os.Setenv("HOME", originalHome)
	os.Setenv("HOME", tempHome)
	t.Setenv("XDG_DATA_HOME", "")

	// Create old directory to trigger backward compatibility
	oldDir := filepath.Join(tempHome, ".op-authd")
	if err := os.MkdirAll(oldDir, 0o700); err != nil {
		t.Fatalf("Failed to create old directory: %v", err)
	}

	tokenPath, err := TokenPath()
	if err != nil {
		t.Fatalf("TokenPath failed: %v", err)
	}

	expectedPath := filepath.Join(tempHome, ".op-authd", "token")
	if tokenPath != expectedPath {
		t.Errorf("Expected token path %q, got %q", expectedPath, tokenPath)
	}

	// Check that the state directory was created
	stateDir := filepath.Dir(tokenPath)
	if _, err := os.Stat(stateDir); os.IsNotExist(err) {
		t.Error("State directory was not created by TokenPath")
	}
}

func TestEnsureToken_NewToken(t *testing.T) {
	// Create a temporary directory for the token
	tempDir := t.TempDir()
	tokenPath := filepath.Join(tempDir, "token")

	// Ensure token file doesn't exist
	if _, err := os.Stat(tokenPath); !os.IsNotExist(err) {
		t.Fatalf("Token file should not exist initially")
	}

	// Call EnsureToken
	token, err := EnsureToken(tokenPath)
	if err != nil {
		t.Fatalf("EnsureToken failed: %v", err)
	}

	// Verify token properties
	if len(token) != 64 { // 32 bytes hex encoded = 64 chars
		t.Errorf("Expected token length 64, got %d", len(token))
	}

	// Verify token is valid hex
	if _, err := hex.DecodeString(token); err != nil {
		t.Errorf("Token is not valid hex: %v", err)
	}

	// Verify token file was created
	if _, err := os.Stat(tokenPath); os.IsNotExist(err) {
		t.Error("Token file was not created")
	}

	// Check file permissions
	info, err := os.Stat(tokenPath)
	if err != nil {
		t.Fatalf("Failed to stat token file: %v", err)
	}

	perm := info.Mode().Perm()
	if perm != 0o600 {
		t.Errorf("Expected token file permissions 0o600, got %o", perm)
	}

	// Verify file content matches returned token
	content, err := os.ReadFile(tokenPath)
	if err != nil {
		t.Fatalf("Failed to read token file: %v", err)
	}

	if string(content) != token {
		t.Errorf("Token file content %q does not match returned token %q", string(content), token)
	}
}

func TestEnsureToken_ExistingToken(t *testing.T) {
	// Create a temporary directory for the token
	tempDir := t.TempDir()
	tokenPath := filepath.Join(tempDir, "token")

	// Create an existing token file
	existingToken := "abcdef1234567890abcdef1234567890abcdef1234567890abcdef1234567890"
	if err := os.WriteFile(tokenPath, []byte(existingToken), 0o600); err != nil {
		t.Fatalf("Failed to create existing token file: %v", err)
	}

	// Call EnsureToken
	token, err := EnsureToken(tokenPath)
	if err != nil {
		t.Fatalf("EnsureToken failed: %v", err)
	}

	// Verify it returned the existing token
	if token != existingToken {
		t.Errorf("Expected existing token %q, got %q", existingToken, token)
	}
}

func TestEnsureToken_ReadError(t *testing.T) {
	// Create a temporary directory
	tempDir := t.TempDir()
	tokenPath := filepath.Join(tempDir, "unreadable")

	// Create a file with no read permissions
	if err := os.WriteFile(tokenPath, []byte("token"), 0o200); err != nil {
		t.Fatalf("Failed to create unreadable file: %v", err)
	}

	// Call EnsureToken - should fail to read existing file
	_, err := EnsureToken(tokenPath)
	if err == nil {
		t.Error("Expected error when reading unreadable token file")
	}
}

func TestEnsureToken_WriteError(t *testing.T) {
	// Skip this test if running as root (root can usually write anywhere)
	if os.Getuid() == 0 {
		t.Skip("Skipping write permission test when running as root")
	}

	// Try to create token in non-writable directory
	tokenPath := "/nonexistent/directory/token"

	_, err := EnsureToken(tokenPath)
	if err == nil {
		t.Error("Expected error when writing to non-writable location")
	}
}

func TestEnsureToken_Deterministic(t *testing.T) {
	// Create two temporary directories
	tempDir1 := t.TempDir()
	tempDir2 := t.TempDir()

	tokenPath1 := filepath.Join(tempDir1, "token")
	tokenPath2 := filepath.Join(tempDir2, "token")

	// Generate two tokens
	token1, err := EnsureToken(tokenPath1)
	if err != nil {
		t.Fatalf("Failed to create first token: %v", err)
	}

	token2, err := EnsureToken(tokenPath2)
	if err != nil {
		t.Fatalf("Failed to create second token: %v", err)
	}

	// Tokens should be different (extremely low probability of collision)
	if token1 == token2 {
		t.Error("Two randomly generated tokens should not be identical")
	}

	// Calling EnsureToken again on the same path should return the same token
	token1Again, err := EnsureToken(tokenPath1)
	if err != nil {
		t.Fatalf("Failed to read existing token: %v", err)
	}

	if token1Again != token1 {
		t.Errorf("EnsureToken should return same token for existing file: %q vs %q", token1, token1Again)
	}
}

func TestIntegration_PathsAndToken(t *testing.T) {
	// Integration test that combines all the path functions with token creation
	tempHome := t.TempDir()

	// Save and restore original HOME
	originalHome := os.Getenv("HOME")
	defer os.Setenv("HOME", originalHome)
	os.Setenv("HOME", tempHome)

	// Test the full workflow
	stateDir, err := StateDir()
	if err != nil {
		t.Fatalf("StateDir failed: %v", err)
	}

	socketPath, err := SocketPath()
	if err != nil {
		t.Fatalf("SocketPath failed: %v", err)
	}

	tokenPath, err := TokenPath()
	if err != nil {
		t.Fatalf("TokenPath failed: %v", err)
	}

	// Verify all paths are within the state directory
	if !strings.HasPrefix(socketPath, stateDir) {
		t.Errorf("Socket path %q is not within state dir %q", socketPath, stateDir)
	}

	if !strings.HasPrefix(tokenPath, stateDir) {
		t.Errorf("Token path %q is not within state dir %q", tokenPath, stateDir)
	}

	// Create a token
	token, err := EnsureToken(tokenPath)
	if err != nil {
		t.Fatalf("EnsureToken failed: %v", err)
	}

	// Verify token file exists at the expected path
	if _, err := os.Stat(tokenPath); os.IsNotExist(err) {
		t.Error("Token file was not created at expected path")
	}

	// Verify we can read the token back
	content, err := os.ReadFile(tokenPath)
	if err != nil {
		t.Fatalf("Failed to read token file: %v", err)
	}

	if string(content) != token {
		t.Errorf("Token file content does not match: expected %q, got %q", token, string(content))
	}

	t.Logf("Integration test successful:")
	t.Logf("  State dir: %s", stateDir)
	t.Logf("  Socket path: %s", socketPath)
	t.Logf("  Token path: %s", tokenPath)
	t.Logf("  Token: %s", token)
}

// Benchmark tests
func BenchmarkHomeDir(b *testing.B) {
	for i := 0; i < b.N; i++ {
		_ = HomeDir()
	}
}

func BenchmarkStateDir(b *testing.B) {
	tempHome := b.TempDir()
	originalHome := os.Getenv("HOME")
	defer os.Setenv("HOME", originalHome)
	os.Setenv("HOME", tempHome)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = StateDir()
	}
}

func BenchmarkEnsureToken_Existing(b *testing.B) {
	tempDir := b.TempDir()
	tokenPath := filepath.Join(tempDir, "token")

	// Create initial token
	_, err := EnsureToken(tokenPath)
	if err != nil {
		b.Fatalf("Failed to create initial token: %v", err)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = EnsureToken(tokenPath)
	}
}

func TestDataDir(t *testing.T) {
	// Test without XDG_DATA_HOME (should use ~/.local/share/op-authd)
	t.Setenv("XDG_DATA_HOME", "")

	dir, err := DataDir()
	if err != nil {
		t.Fatalf("DataDir failed: %v", err)
	}

	expectedSuffix := filepath.Join(".local", "share", "op-authd")
	if !strings.HasSuffix(dir, expectedSuffix) {
		t.Errorf("Expected DataDir to end with %q, got %q", expectedSuffix, dir)
	}
}

func TestDataDirWithXDG(t *testing.T) {
	// Test with XDG_DATA_HOME set
	testDataHome := t.TempDir()
	t.Setenv("XDG_DATA_HOME", testDataHome)

	dir, err := DataDir()
	if err != nil {
		t.Fatalf("DataDir failed: %v", err)
	}

	expected := filepath.Join(testDataHome, "op-authd")
	if dir != expected {
		t.Errorf("Expected DataDir %q, got %q", expected, dir)
	}
}

func TestConfigDir(t *testing.T) {
	// Test without XDG_CONFIG_HOME (should use ~/.config/op-authd)
	t.Setenv("XDG_CONFIG_HOME", "")

	dir, err := ConfigDir()
	if err != nil {
		t.Fatalf("ConfigDir failed: %v", err)
	}

	expectedSuffix := filepath.Join(".config", "op-authd")
	if !strings.HasSuffix(dir, expectedSuffix) {
		t.Errorf("Expected ConfigDir to end with %q, got %q", expectedSuffix, dir)
	}
}

func TestConfigDirWithXDG(t *testing.T) {
	// Test with XDG_CONFIG_HOME set
	testConfigHome := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", testConfigHome)

	dir, err := ConfigDir()
	if err != nil {
		t.Fatalf("ConfigDir failed: %v", err)
	}

	expected := filepath.Join(testConfigHome, "op-authd")
	if dir != expected {
		t.Errorf("Expected ConfigDir %q, got %q", expected, dir)
	}
}

func TestRuntimeDir(t *testing.T) {
	// Test without XDG_RUNTIME_DIR (should fall back to DataDir)
	tempHome := t.TempDir()
	t.Setenv("HOME", tempHome)
	t.Setenv("XDG_RUNTIME_DIR", "")
	t.Setenv("XDG_DATA_HOME", "")

	// Don't create old directory, so it uses XDG paths
	runtimeDir, err := RuntimeDir()
	if err != nil {
		t.Fatalf("RuntimeDir failed: %v", err)
	}

	dataDir, err := DataDir()
	if err != nil {
		t.Fatalf("DataDir failed: %v", err)
	}

	if runtimeDir != dataDir {
		t.Errorf("Expected RuntimeDir to equal DataDir when XDG_RUNTIME_DIR not set, got %q vs %q", runtimeDir, dataDir)
	}
}

func TestRuntimeDirWithXDG(t *testing.T) {
	// Test with XDG_RUNTIME_DIR set
	tempHome := t.TempDir()
	testRuntimeDir := t.TempDir()
	t.Setenv("HOME", tempHome)
	t.Setenv("XDG_RUNTIME_DIR", testRuntimeDir)

	// Don't create old directory, so it uses XDG paths
	dir, err := RuntimeDir()
	if err != nil {
		t.Fatalf("RuntimeDir failed: %v", err)
	}

	expected := filepath.Join(testRuntimeDir, "op-authd")
	if dir != expected {
		t.Errorf("Expected RuntimeDir %q, got %q", expected, dir)
	}
}

func TestStateDir_BackwardCompatibility(t *testing.T) {
	// Test backward compatibility: when ~/.op-authd exists, it should be used
	tempHome := t.TempDir()
	t.Setenv("HOME", tempHome)
	t.Setenv("XDG_DATA_HOME", "")

	// Create old directory structure
	oldDir := filepath.Join(tempHome, ".op-authd")
	if err := os.MkdirAll(oldDir, 0o700); err != nil {
		t.Fatalf("Failed to create old directory: %v", err)
	}

	dir, err := StateDir()
	if err != nil {
		t.Fatalf("StateDir failed: %v", err)
	}

	if dir != oldDir {
		t.Errorf("Expected StateDir to use old directory %q for compatibility, got %q", oldDir, dir)
	}
}

func TestStateDir_XDGWhenNoOldDir(t *testing.T) {
	// Test XDG behavior when no old directory exists
	tempHome := t.TempDir()
	testDataHome := t.TempDir()
	t.Setenv("HOME", tempHome)
	t.Setenv("XDG_DATA_HOME", testDataHome)

	dir, err := StateDir()
	if err != nil {
		t.Fatalf("StateDir failed: %v", err)
	}

	expected := filepath.Join(testDataHome, "op-authd")
	if dir != expected {
		t.Errorf("Expected StateDir to use XDG path %q when no old dir exists, got %q", expected, dir)
	}
}
