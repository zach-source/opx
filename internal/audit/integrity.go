package audit

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/zach-source/opx/internal/util"
	"github.com/zalando/go-keyring"
)

const (
	integrityService = "opx-authd-integrity"
	keyAccount       = "hmac-key"
)

// SecureAuditEvent wraps audit events with integrity protection
type SecureAuditEvent struct {
	Event     AuditEvent `json:"event"`
	Signature string     `json:"hmac_signature"`
	Counter   uint64     `json:"event_counter"`
	KeyID     string     `json:"key_id"` // For key rotation support
}

// IntegrityManager handles HMAC signing and verification of audit events
type IntegrityManager struct {
	currentKey []byte
	keyID      string
	counter    uint64
}

// NewIntegrityManager creates a new integrity manager
func NewIntegrityManager() (*IntegrityManager, error) {
	key, keyID, err := ensureHMACKey()
	if err != nil {
		return nil, fmt.Errorf("failed to ensure HMAC key: %w", err)
	}

	return &IntegrityManager{
		currentKey: key,
		keyID:      keyID,
		counter:    0,
	}, nil
}

// SignEvent signs an audit event with HMAC
func (im *IntegrityManager) SignEvent(event AuditEvent) SecureAuditEvent {
	im.counter++

	// Create canonical JSON representation for signing
	eventData, _ := json.Marshal(event)
	counterData := fmt.Sprintf("%d", im.counter)

	// Sign: HMAC(event_json + counter + key_id)
	h := hmac.New(sha256.New, im.currentKey)
	h.Write(eventData)
	h.Write([]byte(counterData))
	h.Write([]byte(im.keyID))
	signature := hex.EncodeToString(h.Sum(nil))

	return SecureAuditEvent{
		Event:     event,
		Signature: signature,
		Counter:   im.counter,
		KeyID:     im.keyID,
	}
}

// VerifyEvent verifies the HMAC signature of a secure audit event
func (im *IntegrityManager) VerifyEvent(secureEvent SecureAuditEvent) (bool, error) {
	// Get the key for this event's key ID
	key, err := getHMACKeyByID(secureEvent.KeyID)
	if err != nil {
		return false, fmt.Errorf("failed to get key for ID %s: %w", secureEvent.KeyID, err)
	}

	// Recreate signature
	eventData, _ := json.Marshal(secureEvent.Event)
	counterData := fmt.Sprintf("%d", secureEvent.Counter)

	h := hmac.New(sha256.New, key)
	h.Write(eventData)
	h.Write([]byte(counterData))
	h.Write([]byte(secureEvent.KeyID))
	expectedSignature := hex.EncodeToString(h.Sum(nil))

	// Constant-time comparison
	return hmac.Equal([]byte(secureEvent.Signature), []byte(expectedSignature)), nil
}

// VerifyLogFile verifies the integrity of an entire audit log file
func (im *IntegrityManager) VerifyLogFile(logPath string) (bool, []string, error) {
	file, err := os.Open(logPath)
	if err != nil {
		return false, nil, fmt.Errorf("failed to open log file: %w", err)
	}
	defer file.Close()

	var errors []string
	decoder := json.NewDecoder(file)
	lineNum := 0

	for decoder.More() {
		lineNum++
		var secureEvent SecureAuditEvent
		if err := decoder.Decode(&secureEvent); err != nil {
			errors = append(errors, fmt.Sprintf("line %d: malformed JSON: %v", lineNum, err))
			continue
		}

		valid, err := im.VerifyEvent(secureEvent)
		if err != nil {
			errors = append(errors, fmt.Sprintf("line %d: verification error: %v", lineNum, err))
			continue
		}

		if !valid {
			errors = append(errors, fmt.Sprintf("line %d: invalid HMAC signature", lineNum))
		}
	}

	return len(errors) == 0, errors, nil
}

// ensureHMACKey ensures an HMAC key exists in secure storage.
//
// Both stores are read before minting a new key. Reading the file back is what
// makes the fallback a fallback: on a host with no keyring (headless Linux, CI)
// the previous version wrote the key and never looked at it again, so every call
// generated a fresh random key and no signature could ever be verified against
// the key that produced it.
func ensureHMACKey() ([]byte, string, error) {
	if keyData, err := keyring.Get(integrityService, keyAccount); err == nil {
		if key, keyID, ok := parseHMACKeyData(keyData); ok {
			return key, keyID, nil
		}
	}
	if keyData, err := loadHMACKeyFile(); err == nil {
		if key, keyID, ok := parseHMACKeyData(keyData); ok {
			return key, keyID, nil
		}
	}

	// Generate new key
	key := make([]byte, 32) // 256-bit key
	if _, err := rand.Read(key); err != nil {
		return nil, "", fmt.Errorf("failed to generate HMAC key: %w", err)
	}

	// Nanoseconds, not seconds: two distinct keys minted in the same second would
	// otherwise share an ID, turning "unknown key" into a silent bad signature.
	keyID := fmt.Sprintf("key-%d", time.Now().UnixNano())
	keyData := hex.EncodeToString(key) + ":" + keyID

	// Store in keyring
	if err := keyring.Set(integrityService, keyAccount, keyData); err != nil {
		// Fall back to file storage if keyring unavailable
		if err := storeHMACKeyFile(keyData); err != nil {
			return nil, "", fmt.Errorf("failed to store HMAC key: %w", err)
		}
	}

	return key, keyID, nil
}

// parseHMACKeyData splits the stored "<64 hex chars>:<key id>" form.
func parseHMACKeyData(keyData string) ([]byte, string, bool) {
	if len(keyData) <= 65 || keyData[64] != ':' {
		return nil, "", false
	}
	key, err := hex.DecodeString(keyData[:64])
	if err != nil || len(key) != 32 {
		return nil, "", false
	}
	return key, keyData[65:], true
}

// loadHMACKeyFile reads the fallback key file written by storeHMACKeyFile.
func loadHMACKeyFile() (string, error) {
	dataDir, err := util.DataDir()
	if err != nil {
		return "", err
	}

	data, err := os.ReadFile(filepath.Join(dataDir, "integrity.key"))
	if err != nil {
		return "", err
	}
	return string(data), nil
}

// getHMACKeyByID retrieves an HMAC key by its ID (for verification)
func getHMACKeyByID(keyID string) ([]byte, error) {
	// For now, only support current key
	// TODO: Implement key rotation with historical key storage
	key, currentKeyID, err := ensureHMACKey()
	if err != nil {
		return nil, err
	}

	if keyID == currentKeyID {
		return key, nil
	}

	return nil, fmt.Errorf("unknown key ID: %s", keyID)
}

// storeHMACKeyFile stores HMAC key in file (fallback)
func storeHMACKeyFile(keyData string) error {
	dataDir, err := util.DataDir()
	if err != nil {
		return err
	}

	keyPath := filepath.Join(dataDir, "integrity.key")
	return util.AtomicWriteFile(keyPath, []byte(keyData), 0o600)
}
