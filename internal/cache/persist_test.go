package cache

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func testStore(t *testing.T) *Store {
	t.Helper()
	key := bytes.Repeat([]byte{7}, 32)
	s, err := NewStore(filepath.Join(t.TempDir(), storeFileName), key)
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	return s
}

// The whole point of the feature: a restart must come back warm.
func TestCache_SurvivesRestart(t *testing.T) {
	store := testStore(t)

	before := New(time.Hour)
	if _, err := before.Persist(store, nil); err != nil {
		t.Fatalf("Persist: %v", err)
	}
	before.Set("op://Test/item/password", "hunter2")

	// A fresh Cache, as a restarted daemon would build.
	after := New(time.Hour)
	restored, err := after.Persist(store, nil)
	if err != nil {
		t.Fatalf("Persist: %v", err)
	}
	if restored != 1 {
		t.Fatalf("restored %d entries, want 1", restored)
	}

	got, ok, _, _ := after.Get("op://Test/item/password")
	if !ok {
		t.Fatal("entry did not survive the restart")
	}
	if got != "hunter2" {
		t.Errorf("value = %q, want %q", got, "hunter2")
	}
}

func TestCache_ExpiredEntriesAreNotRestored(t *testing.T) {
	store := testStore(t)

	// A TTL this short means the entry is already stale by the time we reload.
	before := New(time.Nanosecond)
	if _, err := before.Persist(store, nil); err != nil {
		t.Fatalf("Persist: %v", err)
	}
	before.Set("op://Test/item/password", "hunter2")
	time.Sleep(time.Millisecond)

	after := New(time.Hour)
	restored, err := after.Persist(store, nil)
	if err != nil {
		t.Fatalf("Persist: %v", err)
	}
	if restored != 0 {
		t.Errorf("restored %d expired entries, want 0", restored)
	}
}

// Clearing on session lock must take the disk copy with it, or the next restart
// resurrects secrets the lock deliberately discarded.
func TestCache_ClearRemovesStoreFile(t *testing.T) {
	store := testStore(t)

	c := New(time.Hour)
	if _, err := c.Persist(store, nil); err != nil {
		t.Fatalf("Persist: %v", err)
	}
	c.Set("op://Test/item/password", "hunter2")

	if _, err := os.Stat(store.Path()); err != nil {
		t.Fatalf("expected a store file after Set: %v", err)
	}

	c.Clear()

	if _, err := os.Stat(store.Path()); !os.IsNotExist(err) {
		t.Errorf("store file still present after Clear (err=%v)", err)
	}
}

func TestStore_FileIsEncryptedAndPrivate(t *testing.T) {
	store := testStore(t)

	const secret = "correct-horse-battery-staple"
	if err := store.Save([]Record{{Key: "op://Test/item/password", Value: secret, Exp: time.Now().Add(time.Hour)}}); err != nil {
		t.Fatalf("Save: %v", err)
	}

	blob, err := os.ReadFile(store.Path())
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if bytes.Contains(blob, []byte(secret)) {
		t.Error("secret is readable in the store file")
	}
	if bytes.Contains(blob, []byte("op://Test/item/password")) {
		t.Error("ref is readable in the store file")
	}

	info, err := os.Stat(store.Path())
	if err != nil {
		t.Fatalf("Stat: %v", err)
	}
	if perm := info.Mode().Perm(); perm != 0o600 {
		t.Errorf("store file mode = %o, want 600", perm)
	}
}

// A tampered or wrong-key file must be a loud error, never a silent empty cache.
func TestStore_RejectsTamperedFile(t *testing.T) {
	store := testStore(t)

	if err := store.Save([]Record{{Key: "k", Value: "v", Exp: time.Now().Add(time.Hour)}}); err != nil {
		t.Fatalf("Save: %v", err)
	}

	blob, err := os.ReadFile(store.Path())
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	blob[len(blob)-1] ^= 0xff
	if err := os.WriteFile(store.Path(), blob, 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	if _, err := store.Load(); err == nil {
		t.Error("Load accepted a tampered file")
	}
}

func TestStore_MissingFileLoadsEmpty(t *testing.T) {
	records, err := testStore(t).Load()
	if err != nil {
		t.Errorf("Load on a missing file returned %v, want nil", err)
	}
	if len(records) != 0 {
		t.Errorf("got %d records, want 0", len(records))
	}
}

func TestStore_SaveEmptyRemovesFile(t *testing.T) {
	store := testStore(t)

	if err := store.Save([]Record{{Key: "k", Value: "v", Exp: time.Now().Add(time.Hour)}}); err != nil {
		t.Fatalf("Save: %v", err)
	}
	if err := store.Save(nil); err != nil {
		t.Fatalf("Save(nil): %v", err)
	}
	if _, err := os.Stat(store.Path()); !os.IsNotExist(err) {
		t.Errorf("empty save left a file behind (err=%v)", err)
	}
}
