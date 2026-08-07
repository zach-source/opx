package cache

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/zalando/go-keyring"

	"github.com/zach-source/opx/internal/util"
)

const (
	keyringService = "opx-authd"
	keyringAccount = "cache-key"
	storeFileName  = "cache.enc"
)

// Record is one persisted cache entry.
type Record struct {
	Key    string    `json:"k"`
	Value  string    `json:"v"`
	Exp    time.Time `json:"e"`
	Cached time.Time `json:"c"`
}

// Store persists cache entries to disk as a single AES-256-GCM blob. The key
// lives in the OS keyring, never on disk beside the ciphertext.
type Store struct {
	path string
	key  []byte
}

// NewStore builds a store around an explicit key. The key must be 32 bytes.
func NewStore(path string, key []byte) (*Store, error) {
	if len(key) != 32 {
		return nil, fmt.Errorf("cache key must be 32 bytes, got %d", len(key))
	}
	return &Store{path: path, key: key}, nil
}

// OpenStore returns a store in the XDG data dir, keyed by a secret from the OS
// keyring (generated on first use). It fails when no keyring is available:
// stashing the key next to the ciphertext would make the encryption pointless.
func OpenStore() (*Store, error) {
	dir, err := util.DataDir()
	if err != nil {
		return nil, err
	}

	key, err := keyringKey()
	if err != nil {
		return nil, err
	}

	return NewStore(filepath.Join(dir, storeFileName), key)
}

// Path reports where the encrypted cache lives.
func (s *Store) Path() string { return s.path }

func keyringKey() ([]byte, error) {
	switch encoded, err := keyring.Get(keyringService, keyringAccount); {
	case err == nil:
		key, decErr := hex.DecodeString(encoded)
		if decErr != nil || len(key) != 32 {
			return nil, errors.New("stored cache key is corrupt; delete it from the keyring to regenerate")
		}
		return key, nil
	case !errors.Is(err, keyring.ErrNotFound):
		return nil, fmt.Errorf("keyring unavailable: %w", err)
	}

	key := make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		return nil, err
	}
	if err := keyring.Set(keyringService, keyringAccount, hex.EncodeToString(key)); err != nil {
		return nil, fmt.Errorf("keyring unavailable: %w", err)
	}
	return key, nil
}

func (s *Store) aead() (cipher.AEAD, error) {
	block, err := aes.NewCipher(s.key)
	if err != nil {
		return nil, err
	}
	return cipher.NewGCM(block)
}

// Save encrypts records and atomically replaces the store file. An empty record
// set removes the file rather than leaving a decryptable empty blob behind.
func (s *Store) Save(records []Record) error {
	if len(records) == 0 {
		return s.Delete()
	}

	plaintext, err := json.Marshal(records)
	if err != nil {
		return err
	}

	gcm, err := s.aead()
	if err != nil {
		return err
	}

	nonce := make([]byte, gcm.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return err
	}

	// Layout: nonce || ciphertext+tag
	return util.AtomicWriteFile(s.path, gcm.Seal(nonce, nonce, plaintext, nil), 0o600)
}

// Load returns the records on disk. A missing file is not an error; a file that
// fails to decrypt is, so a tampered or key-mismatched store is never silently
// treated as an empty cache.
func (s *Store) Load() ([]Record, error) {
	blob, err := os.ReadFile(s.path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	gcm, err := s.aead()
	if err != nil {
		return nil, err
	}
	if len(blob) < gcm.NonceSize() {
		return nil, errors.New("cache file is truncated")
	}

	nonce, ciphertext := blob[:gcm.NonceSize()], blob[gcm.NonceSize():]
	plaintext, err := gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return nil, fmt.Errorf("cache file failed authentication: %w", err)
	}

	var records []Record
	if err := json.Unmarshal(plaintext, &records); err != nil {
		return nil, err
	}
	return records, nil
}

// Delete removes the store file. A missing file is a no-op.
func (s *Store) Delete() error {
	if err := os.Remove(s.path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return nil
}
