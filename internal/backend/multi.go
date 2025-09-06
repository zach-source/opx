package backend

import (
	"context"
	"fmt"
	"strings"
)

// MultiBackend routes requests to different backends based on URI scheme
type MultiBackend struct {
	opBackend     Backend
	vaultBackend  Backend
	baoBackend    Backend
	defaultScheme string
}

// NewMultiBackend creates a new multi-backend router
func NewMultiBackend(opBackend, vaultBackend, baoBackend Backend, defaultScheme string) *MultiBackend {
	return &MultiBackend{
		opBackend:     opBackend,
		vaultBackend:  vaultBackend,
		baoBackend:    baoBackend,
		defaultScheme: defaultScheme,
	}
}

func (m *MultiBackend) Name() string {
	return "multi"
}

// ReadRef routes the request to the appropriate backend based on URI scheme
func (m *MultiBackend) ReadRef(ctx context.Context, ref string) (string, error) {
	return m.ReadRefWithFlags(ctx, ref, nil)
}

// ReadRefWithFlags routes the request with flags to the appropriate backend
func (m *MultiBackend) ReadRefWithFlags(ctx context.Context, ref string, flags []string) (string, error) {
	backend := m.getBackendForRef(ref)
	if backend == nil {
		return "", fmt.Errorf("no backend available for reference: %s", ref)
	}

	return backend.ReadRefWithFlags(ctx, ref, flags)
}

// getBackendForRef determines which backend to use for a given reference
func (m *MultiBackend) getBackendForRef(ref string) Backend {
	switch {
	case strings.HasPrefix(ref, "op://"):
		return m.opBackend
	case strings.HasPrefix(ref, "vault://"):
		return m.vaultBackend
	case strings.HasPrefix(ref, "bao://"):
		return m.baoBackend
	default:
		// For references without scheme, use default
		switch m.defaultScheme {
		case "op":
			return m.opBackend
		case "vault":
			return m.vaultBackend
		case "bao":
			return m.baoBackend
		}
	}
	return nil
}
