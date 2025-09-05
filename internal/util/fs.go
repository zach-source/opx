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

func StateDir() (string, error) {
	dir := filepath.Join(HomeDir(), ".op-authd")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", fmt.Errorf("mkdir %s: %w", dir, err)
	}
	if err := os.Chmod(dir, 0o700); err != nil {
		return "", err
	}
	return dir, nil
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
