package util

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

func HomeDir() string {
	h, _ := os.UserHomeDir()
	if h == "" {
		h = "."
	}
	return h
}

// DataDir returns the XDG-compliant data directory for op-authd
func DataDir() (string, error) {
	var dir string

	// Check XDG_DATA_HOME first
	if xdgDataHome := os.Getenv("XDG_DATA_HOME"); xdgDataHome != "" {
		dir = filepath.Join(xdgDataHome, "op-authd")
	} else {
		// Fallback to ~/.local/share/op-authd
		dir = filepath.Join(HomeDir(), ".local", "share", "op-authd")
	}

	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", fmt.Errorf("mkdir %s: %w", dir, err)
	}
	if err := os.Chmod(dir, 0o700); err != nil {
		return "", err
	}
	return dir, nil
}

// ConfigDir returns the XDG-compliant config directory for op-authd
func ConfigDir() (string, error) {
	var dir string

	// Check XDG_CONFIG_HOME first
	if xdgConfigHome := os.Getenv("XDG_CONFIG_HOME"); xdgConfigHome != "" {
		dir = filepath.Join(xdgConfigHome, "op-authd")
	} else {
		// Fallback to ~/.config/op-authd
		dir = filepath.Join(HomeDir(), ".config", "op-authd")
	}

	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", fmt.Errorf("mkdir %s: %w", dir, err)
	}
	if err := os.Chmod(dir, 0o700); err != nil {
		return "", err
	}
	return dir, nil
}

// RuntimeDir returns the XDG-compliant runtime directory for op-authd
func RuntimeDir() (string, error) {
	// For backward compatibility, check if old ~/.op-authd directory exists
	oldDir := filepath.Join(HomeDir(), ".op-authd")
	if _, err := os.Stat(oldDir); err == nil {
		// Old directory exists, use it for runtime files too
		if err := os.Chmod(oldDir, 0o700); err != nil {
			return "", err
		}
		return oldDir, nil
	}

	var dir string

	// Check XDG_RUNTIME_DIR first
	if xdgRuntimeDir := os.Getenv("XDG_RUNTIME_DIR"); xdgRuntimeDir != "" {
		dir = filepath.Join(xdgRuntimeDir, "op-authd")
	} else {
		// Fallback to data directory for runtime files
		return DataDir()
	}

	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", fmt.Errorf("mkdir %s: %w", dir, err)
	}
	if err := os.Chmod(dir, 0o700); err != nil {
		return "", err
	}
	return dir, nil
}

// StateDir maintains backward compatibility (now an alias for DataDir)
func StateDir() (string, error) {
	// For backward compatibility, check if old ~/.op-authd directory exists
	oldDir := filepath.Join(HomeDir(), ".op-authd")
	if _, err := os.Stat(oldDir); err == nil {
		// Old directory exists, continue using it for backward compatibility
		if err := os.Chmod(oldDir, 0o700); err != nil {
			return "", err
		}
		return oldDir, nil
	}

	// No old directory, use XDG-compliant path
	return DataDir()
}

func SocketPath() (string, error) {
	dir, err := StateDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "socket.sock"), nil
}

func TokenPath() (string, error) {
	dir, err := StateDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "token"), nil
}

func EnsureToken(path string) (string, error) {
	// Try to read existing token first
	if b, err := os.ReadFile(path); err == nil {
		return string(b), nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return "", err
	}

	// Generate new token
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	tok := hex.EncodeToString(b)

	// Use atomic file creation: write to temp file, then rename
	tempPath := path + ".tmp"

	// Create temp file with exclusive creation (O_EXCL prevents race condition)
	f, err := os.OpenFile(tempPath, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return "", fmt.Errorf("failed to create temp token file: %w", err)
	}

	// Write token to temp file
	_, writeErr := f.Write([]byte(tok))
	closeErr := f.Close()

	if writeErr != nil {
		os.Remove(tempPath) // Clean up temp file on write error
		return "", fmt.Errorf("failed to write token: %w", writeErr)
	}
	if closeErr != nil {
		os.Remove(tempPath) // Clean up temp file on close error
		return "", fmt.Errorf("failed to close token file: %w", closeErr)
	}

	// Atomically rename temp file to final location
	if err := os.Rename(tempPath, path); err != nil {
		os.Remove(tempPath) // Clean up temp file on rename error

		// If rename failed due to existing file, try to read it (race condition recovery)
		if b, readErr := os.ReadFile(path); readErr == nil {
			return string(b), nil
		}

		return "", fmt.Errorf("failed to rename token file: %w", err)
	}

	return tok, nil
}
