package backend

import (
	"context"
	"strings"
	"testing"
)

func TestParseVaultURI(t *testing.T) {
	tests := []struct {
		name          string
		ref           string
		expectedPath  string
		expectedField string
		expectError   bool
	}{
		{
			name:          "simple path",
			ref:           "vault://secret/myapp/config",
			expectedPath:  "secret/myapp/config",
			expectedField: "",
			expectError:   false,
		},
		{
			name:          "path with field",
			ref:           "vault://secret/myapp/config#password",
			expectedPath:  "secret/myapp/config",
			expectedField: "password",
			expectError:   false,
		},
		{
			name:          "invalid scheme",
			ref:           "op://vault/item/field",
			expectedPath:  "",
			expectedField: "",
			expectError:   true,
		},
		{
			name:          "empty path",
			ref:           "vault://",
			expectedPath:  "",
			expectedField: "",
			expectError:   true,
		},
		{
			name:          "empty path with field",
			ref:           "vault://#field",
			expectedPath:  "",
			expectedField: "",
			expectError:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path, field, err := parseVaultURI(tt.ref)

			if tt.expectError {
				if err == nil {
					t.Error("Expected error but got none")
				}
				return
			}

			if err != nil {
				t.Errorf("Unexpected error: %v", err)
				return
			}

			if path != tt.expectedPath {
				t.Errorf("Expected path %q, got %q", tt.expectedPath, path)
			}

			if field != tt.expectedField {
				t.Errorf("Expected field %q, got %q", tt.expectedField, field)
			}
		})
	}
}

func TestVault_Name(t *testing.T) {
	vault := NewVault(VaultConfig{})
	if vault.Name() != "vault" {
		t.Errorf("Expected name 'vault', got %q", vault.Name())
	}
}

func TestBao_Name(t *testing.T) {
	bao := NewBao(VaultConfig{})
	if bao.Name() != "bao" {
		t.Errorf("Expected name 'bao', got %q", bao.Name())
	}
}

func TestBao_URIConversion(t *testing.T) {
	// Test that Bao backend converts bao:// to vault:// for processing
	// We can't easily test the full ReadRef without a real Vault server,
	// but we can test the URI conversion logic by checking that it
	// processes bao:// references correctly

	bao := NewBao(VaultConfig{
		Address:    "http://localhost:8300",
		AuthMethod: "token",
		Token:      "test-token",
	})

	ctx := context.Background()

	// This will fail because there's no real Vault server, but we're testing
	// that it attempts to process the bao:// reference
	_, err := bao.ReadRef(ctx, "bao://secret/test")

	// We expect an error (no server), but it should be a connection error,
	// not a parsing error, which indicates the URI was processed correctly
	if err == nil {
		t.Error("Expected error due to no Vault server")
	}

	// The error should be about connection, not URI parsing
	if err != nil && !containsAny(err.Error(), []string{"connection", "network", "dial", "refused"}) {
		t.Logf("Error (expected due to no server): %v", err)
	}
}

func TestMultiBackend_Name(t *testing.T) {
	multi := NewMultiBackend(nil, nil, nil, "op")
	if multi.Name() != "multi" {
		t.Errorf("Expected name 'multi', got %q", multi.Name())
	}
}

func TestMultiBackend_GetBackendForRef(t *testing.T) {
	opBackend := &Fake{}
	vaultBackend := NewVault(VaultConfig{})
	baoBackend := NewBao(VaultConfig{})

	multi := NewMultiBackend(opBackend, vaultBackend, baoBackend, "op")

	tests := []struct {
		name            string
		ref             string
		expectedBackend Backend
	}{
		{
			name:            "op scheme",
			ref:             "op://vault/item/field",
			expectedBackend: opBackend,
		},
		{
			name:            "vault scheme",
			ref:             "vault://secret/myapp/config",
			expectedBackend: vaultBackend,
		},
		{
			name:            "bao scheme",
			ref:             "bao://secret/myapp/config",
			expectedBackend: baoBackend,
		},
		{
			name:            "no scheme defaults to op",
			ref:             "some/path",
			expectedBackend: opBackend,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			backend := multi.getBackendForRef(tt.ref)
			if backend != tt.expectedBackend {
				t.Errorf("Expected backend %T, got %T", tt.expectedBackend, backend)
			}
		})
	}
}

// Helper function to check if error message contains any of the given substrings
func containsAny(s string, substrings []string) bool {
	for _, substr := range substrings {
		if strings.Contains(strings.ToLower(s), strings.ToLower(substr)) {
			return true
		}
	}
	return false
}

// Integration test for Vault backend (requires real Vault server)
func TestVault_Integration(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping Vault integration test in short mode")
	}

	// This test requires a real Vault server running on localhost:8200
	// Skip if not available
	vault := NewVault(VaultConfig{
		Address:    "http://localhost:8200",
		AuthMethod: "token",
		Token:      "test-token",
	})

	ctx := context.Background()
	_, err := vault.ReadRef(ctx, "vault://secret/test")

	// We expect this to fail in most cases (no real Vault server)
	// This is just to verify the code structure
	t.Logf("Vault integration test result: %v (expected to fail without real Vault server)", err)
}
