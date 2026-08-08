package server

import (
	"context"
	"reflect"
	"testing"
	"time"

	"github.com/zach-source/opx/internal/backend"
	"github.com/zach-source/opx/internal/cache"
	"github.com/zach-source/opx/internal/session"
)

// rotatingBackend serves a value that can be changed under the daemon's feet,
// as an out-of-band rotation would.
type rotatingBackend struct {
	*countingBackend
	value string
}

func (r *rotatingBackend) ReadRefWithFlags(ctx context.Context, ref string, flags []string) (string, error) {
	if _, err := r.countingBackend.ReadRefWithFlags(ctx, ref, flags); err != nil {
		return "", err
	}
	return r.value, nil
}

func (r *rotatingBackend) ReadRef(ctx context.Context, ref string) (string, error) {
	return r.ReadRefWithFlags(ctx, ref, nil)
}

func TestCacheKeyRoundTrip(t *testing.T) {
	tests := []struct {
		ref   string
		flags []string
	}{
		{"op://Test/item/password", nil},
		{"op://Test/item/password", []string{"--account=A"}},
		{"op://Test/item/password", []string{"--account=A", "--other"}},
	}

	for _, tt := range tests {
		ref, flags := splitCacheKey(cacheKeyFor(tt.ref, tt.flags))
		if ref != tt.ref {
			t.Errorf("ref = %q, want %q", ref, tt.ref)
		}
		if len(flags) != len(tt.flags) || (len(flags) > 0 && !reflect.DeepEqual(flags, tt.flags)) {
			t.Errorf("flags = %v, want %v", flags, tt.flags)
		}
	}
}

// The point of the feature: a rotation the daemon never saw gets corrected.
func TestRevalidate_PicksUpRotation(t *testing.T) {
	const ref = "op://Test/item/password"

	be := &rotatingBackend{countingBackend: &countingBackend{}, value: "old"}
	srv := &Server{Backend: be, Cache: cache.New(time.Hour)}

	if _, err := srv.readOne(context.Background(), ref); err != nil {
		t.Fatalf("seed: %v", err)
	}
	if got, _, _, _ := srv.Cache.Get(ref); got != "old" {
		t.Fatalf("seeded value = %q, want old", got)
	}

	be.value = "rotated"
	checked, changed := srv.revalidateOnce(context.Background())
	if checked != 1 || changed != 1 {
		t.Errorf("checked=%d changed=%d, want 1/1", checked, changed)
	}

	got, ok, _, _ := srv.Cache.Get(ref)
	if !ok || got != "rotated" {
		t.Errorf("cached value = %q (ok=%v), want rotated", got, ok)
	}
}

// Revalidation must not extend how long a secret stays cached, or a
// periodically-refreshed entry would never age out.
func TestRevalidate_DoesNotExtendExpiry(t *testing.T) {
	const ref = "op://Test/item/password"

	be := &rotatingBackend{countingBackend: &countingBackend{}, value: "old"}
	srv := &Server{Backend: be, Cache: cache.New(time.Hour)}
	if _, err := srv.readOne(context.Background(), ref); err != nil {
		t.Fatalf("seed: %v", err)
	}
	_, _, expBefore, _ := srv.Cache.Get(ref)

	be.value = "rotated"
	srv.revalidateOnce(context.Background())

	_, _, expAfter, _ := srv.Cache.Get(ref)
	if !expAfter.Equal(expBefore) {
		t.Errorf("expiry moved from %v to %v; revalidation must not extend the TTL", expBefore, expAfter)
	}
}

// A transient backend failure must not evict a good entry - dropping it would
// force an interactive prompt on the next read.
func TestRevalidate_KeepsValueOnBackendError(t *testing.T) {
	const ref = "op://Test/item/password"

	be := &flakyBackend{value: "good"}
	srv := &Server{Backend: be, Cache: cache.New(time.Hour)}
	if _, err := srv.readOne(context.Background(), ref); err != nil {
		t.Fatalf("seed: %v", err)
	}

	be.fail = true
	srv.revalidateOnce(context.Background())

	got, ok, _, _ := srv.Cache.Get(ref)
	if !ok || got != "good" {
		t.Errorf("cached value = %q (ok=%v), want the entry kept", got, ok)
	}
}

// Revalidation must not count as user activity: going through the session
// wrapper would renew the session on every pass and the idle lock would never fire.
func TestRevalidate_DoesNotRenewSession(t *testing.T) {
	const ref = "op://Test/item/password"

	mgr := session.NewManager(&session.Config{
		SessionIdleTimeout: time.Hour,
		EnableSessionLock:  true,
		CheckInterval:      time.Hour,
	})
	mgr.MarkAuthenticated()

	wrapped := backend.NewSessionAwareBackend(&rotatingBackend{countingBackend: &countingBackend{}, value: "old"}, mgr)
	srv := &Server{Backend: wrapped, Cache: cache.New(time.Hour), Session: mgr}

	if _, err := srv.readOne(context.Background(), ref); err != nil {
		t.Fatalf("seed: %v", err)
	}

	before := mgr.GetInfo().LastActivity
	time.Sleep(10 * time.Millisecond)
	srv.revalidateOnce(context.Background())

	if after := mgr.GetInfo().LastActivity; !after.Equal(before) {
		t.Errorf("revalidation moved LastActivity %v -> %v; the idle lock would never fire", before, after)
	}
}

// A locked session must not be poked: an `op read` would raise an interactive
// prompt, from a background timer, which is what the daemon exists to prevent.
func TestRevalidate_SkipsWhenSessionLocked(t *testing.T) {
	const ref = "op://Test/item/password"

	mgr := session.NewManager(&session.Config{
		SessionIdleTimeout: time.Hour,
		EnableSessionLock:  true,
		CheckInterval:      time.Hour,
	})
	mgr.MarkAuthenticated()

	be := &rotatingBackend{countingBackend: &countingBackend{}, value: "old"}
	srv := &Server{Backend: be, Cache: cache.New(time.Hour), Session: mgr}
	if _, err := srv.readOne(context.Background(), ref); err != nil {
		t.Fatalf("seed: %v", err)
	}
	callsAfterSeed := be.count()

	mgr.MarkLocked()
	checked, changed := srv.revalidateOnce(context.Background())

	if checked != 0 || changed != 0 {
		t.Errorf("checked=%d changed=%d, want 0/0 while locked", checked, changed)
	}
	if be.count() != callsAfterSeed {
		t.Errorf("backend was called %d times while locked", be.count()-callsAfterSeed)
	}
}

// SessionUnknown is the ordinary state for a daemon nobody explicitly
// authenticated. Treating it as unsafe would silently disable the revalidator.
func TestRevalidate_RunsWhenSessionUnknown(t *testing.T) {
	const ref = "op://Test/item/password"

	mgr := session.NewManager(&session.Config{
		SessionIdleTimeout: time.Hour,
		EnableSessionLock:  true,
		CheckInterval:      time.Hour,
	})
	if state := mgr.GetInfo().State; state != session.SessionUnknown {
		t.Fatalf("expected a fresh manager to be unknown, got %v", state)
	}

	be := &rotatingBackend{countingBackend: &countingBackend{}, value: "old"}
	srv := &Server{Backend: be, Cache: cache.New(time.Hour), Session: mgr}
	if _, err := srv.readOne(context.Background(), ref); err != nil {
		t.Fatalf("seed: %v", err)
	}

	be.value = "rotated"
	checked, changed := srv.revalidateOnce(context.Background())
	if checked != 1 || changed != 1 {
		t.Errorf("checked=%d changed=%d, want 1/1 in the unknown state", checked, changed)
	}
}
