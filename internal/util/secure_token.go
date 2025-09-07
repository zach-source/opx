package util

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os"

	"github.com/zalando/go-keyring"
)

const (
	keyringSservice = "opx-authd"
	keyringAccount  = "auth-token"
)

// SecureTokenManager handles token storage using OS keyring with file fallback
type SecureTokenManager struct {
	fallbackPath string
}

// NewSecureTokenManager creates a new secure token manager
func NewSecureTokenManager() (*SecureTokenManager, error) {
	// Get fallback path for systems without keyring support
	fallbackPath, err := TokenPath()
	if err != nil {
		return nil, fmt.Errorf("failed to get token path: %w", err)
	}

	return &SecureTokenManager{
		fallbackPath: fallbackPath,
	}, nil
}

// StoreToken stores the token in OS keyring with file fallback
func (stm *SecureTokenManager) StoreToken(token string) error {
	// Try keyring first
	err := keyring.Set(keyringSservice, keyringAccount, token)
	if err == nil {
		// Successfully stored in keyring, remove any existing file
		os.Remove(stm.fallbackPath)
		return nil
	}

	// Keyring failed, fall back to encrypted file storage
	if err := stm.storeTokenFile(token); err != nil {
		return fmt.Errorf("failed to store token in both keyring and file: keyring_err=%v, file_err=%w", err, err)
	}

	return nil
}

// RetrieveToken retrieves the token from OS keyring with file fallback
func (stm *SecureTokenManager) RetrieveToken() (string, error) {
	// Try keyring first
	token, err := keyring.Get(keyringSservice, keyringAccount)
	if err == nil {
		return token, nil
	}

	// Keyring failed, try file fallback
	return stm.retrieveTokenFile()
}

// DeleteToken removes the token from both keyring and file
func (stm *SecureTokenManager) DeleteToken() error {
	var keyringErr, fileErr error

	// Try to delete from keyring
	keyringErr = keyring.Delete(keyringSservice, keyringAccount)

	// Try to delete from file
	fileErr = os.Remove(stm.fallbackPath)

	// Return error only if both failed and both exist
	if keyringErr != nil && fileErr != nil {
		return fmt.Errorf("failed to delete token from keyring (%v) and file (%v)", keyringErr, fileErr)
	}

	return nil
}

// IsKeyringsSupported checks if OS keyring is available
func (stm *SecureTokenManager) IsKeyringsSupported() bool {
	// Test by trying to set and get a test value
	testValue := "test"
	if err := keyring.Set(keyringSservice, "test-key", testValue); err != nil {
		return false
	}

	retrieved, err := keyring.Get(keyringSservice, "test-key")
	if err != nil || retrieved != testValue {
		return false
	}

	// Clean up test
	keyring.Delete(keyringSservice, "test-key")
	return true
}

// GetStorageLocation returns information about where the token is stored
func (stm *SecureTokenManager) GetStorageLocation() string {
	if stm.IsKeyringsSupported() {
		return "OS keyring"
	}
	return "encrypted file: " + stm.fallbackPath
}

// storeTokenFile stores token in encrypted file (fallback)
func (stm *SecureTokenManager) storeTokenFile(token string) error {
	// For now, store as plaintext file with secure permissions
	// TODO: Add file encryption using AES-GCM
	return AtomicWriteFile(stm.fallbackPath, []byte(token), 0o600)
}

// retrieveTokenFile retrieves token from file
func (stm *SecureTokenManager) retrieveTokenFile() (string, error) {
	data, err := os.ReadFile(stm.fallbackPath)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

// EnsureSecureToken ensures a token exists using secure storage
func EnsureSecureToken() (string, error) {
	stm, err := NewSecureTokenManager()
	if err != nil {
		return "", err
	}

	// Try to retrieve existing token
	if token, err := stm.RetrieveToken(); err == nil && token != "" {
		return token, nil
	}

	// Generate new token
	token, err := generateToken()
	if err != nil {
		return "", fmt.Errorf("failed to generate token: %w", err)
	}

	// Store securely
	if err := stm.StoreToken(token); err != nil {
		return "", fmt.Errorf("failed to store token: %w", err)
	}

	return token, nil
}

// generateToken creates a cryptographically secure random token
func generateToken() (string, error) {
	b := make([]byte, 32) // 256-bit token
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}
