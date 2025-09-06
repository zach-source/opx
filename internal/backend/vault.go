package backend

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// VaultConfig holds Vault/Bao connection configuration
type VaultConfig struct {
	Address    string        `json:"address"`     // Vault server address
	Namespace  string        `json:"namespace"`   // Vault namespace (optional)
	AuthPath   string        `json:"auth_path"`   // Authentication path (e.g., "auth/userpass")
	AuthMethod string        `json:"auth_method"` // Authentication method ("userpass", "token", etc.)
	Token      string        `json:"-"`           // Current auth token (runtime only)
	TokenTTL   time.Duration `json:"-"`           // Token expiration (runtime only)
}

// Vault backend for HashiCorp Vault
type Vault struct {
	config VaultConfig
	client *http.Client
}

// NewVault creates a new Vault backend with the given configuration
func NewVault(config VaultConfig) *Vault {
	return &Vault{
		config: config,
		client: &http.Client{
			Timeout: 10 * time.Second,
		},
	}
}

func (v *Vault) Name() string {
	return "vault"
}

// ReadRef reads a secret from Vault using vault:// URI scheme
func (v *Vault) ReadRef(ctx context.Context, ref string) (string, error) {
	return v.ReadRefWithFlags(ctx, ref, nil)
}

// ReadRefWithFlags reads a secret from Vault with optional flags
func (v *Vault) ReadRefWithFlags(ctx context.Context, ref string, flags []string) (string, error) {
	// Parse vault:// URI
	vaultPath, field, err := parseVaultURI(ref)
	if err != nil {
		return "", fmt.Errorf("invalid vault reference %s: %w", ref, err)
	}

	// Ensure we have a valid authentication token
	if err := v.ensureAuthenticated(ctx); err != nil {
		return "", fmt.Errorf("vault authentication failed: %w", err)
	}

	// Read the secret from Vault
	secret, err := v.readSecret(ctx, vaultPath)
	if err != nil {
		return "", fmt.Errorf("failed to read vault secret: %w", err)
	}

	// Extract the specific field if specified
	if field != "" {
		if data, ok := secret.Data["data"].(map[string]interface{}); ok {
			if value, exists := data[field]; exists {
				if str, ok := value.(string); ok {
					return str, nil
				}
				return fmt.Sprintf("%v", value), nil
			}
			return "", fmt.Errorf("field %s not found in secret", field)
		}
		return "", fmt.Errorf("secret does not contain data field")
	}

	// If no specific field requested, return JSON representation
	data, err := json.Marshal(secret.Data)
	if err != nil {
		return "", fmt.Errorf("failed to marshal secret data: %w", err)
	}

	return string(data), nil
}

// VaultSecret represents a Vault secret response
type VaultSecret struct {
	Data     map[string]interface{} `json:"data"`
	Metadata map[string]interface{} `json:"metadata,omitempty"`
}

// parseVaultURI parses a vault:// URI into path and field components
func parseVaultURI(ref string) (path, field string, err error) {
	if !strings.HasPrefix(ref, "vault://") {
		return "", "", fmt.Errorf("reference must start with vault://")
	}

	// Remove vault:// prefix
	trimmed := strings.TrimPrefix(ref, "vault://")

	// Split on # to separate path from field
	parts := strings.SplitN(trimmed, "#", 2)
	path = parts[0]

	if len(parts) > 1 {
		field = parts[1]
	}

	if path == "" {
		return "", "", fmt.Errorf("vault path cannot be empty")
	}

	return path, field, nil
}

// ensureAuthenticated ensures we have a valid Vault token
func (v *Vault) ensureAuthenticated(ctx context.Context) error {
	// Check if current token is still valid
	if v.config.Token != "" && time.Now().Before(time.Now().Add(v.config.TokenTTL)) {
		return nil
	}

	// Token expired or missing, need to authenticate
	return v.authenticate(ctx)
}

// authenticate performs Vault authentication
func (v *Vault) authenticate(ctx context.Context) error {
	switch v.config.AuthMethod {
	case "token":
		// Token auth - just verify the token works
		return v.verifyToken(ctx)
	case "userpass":
		return v.authenticateUserpass(ctx)
	default:
		return fmt.Errorf("authentication method %s not yet implemented", v.config.AuthMethod)
	}
}

// authenticateUserpass performs username/password authentication
func (v *Vault) authenticateUserpass(ctx context.Context) error {
	// This would typically prompt for credentials or read from environment
	// For now, return an error with instructions
	return fmt.Errorf("userpass authentication requires environment variables VAULT_USERNAME and VAULT_PASSWORD")
}

// verifyToken checks if the current token is valid
func (v *Vault) verifyToken(ctx context.Context) error {
	req, err := http.NewRequestWithContext(ctx, "GET", v.config.Address+"/v1/auth/token/lookup-self", nil)
	if err != nil {
		return err
	}

	req.Header.Set("X-Vault-Token", v.config.Token)
	if v.config.Namespace != "" {
		req.Header.Set("X-Vault-Namespace", v.config.Namespace)
	}

	resp, err := v.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return fmt.Errorf("token verification failed with status %d", resp.StatusCode)
	}

	return nil
}

// readSecret reads a secret from the specified Vault path
func (v *Vault) readSecret(ctx context.Context, path string) (*VaultSecret, error) {
	// Construct Vault API URL
	apiPath := "/v1/" + path
	req, err := http.NewRequestWithContext(ctx, "GET", v.config.Address+apiPath, nil)
	if err != nil {
		return nil, err
	}

	req.Header.Set("X-Vault-Token", v.config.Token)
	if v.config.Namespace != "" {
		req.Header.Set("X-Vault-Namespace", v.config.Namespace)
	}

	resp, err := v.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode == 404 {
		return nil, fmt.Errorf("secret not found at path %s", path)
	}

	if resp.StatusCode != 200 {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("vault API returned status %d: %s", resp.StatusCode, string(body))
	}

	var vaultResp struct {
		Data *VaultSecret `json:"data"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&vaultResp); err != nil {
		return nil, fmt.Errorf("failed to decode vault response: %w", err)
	}

	if vaultResp.Data == nil {
		return nil, fmt.Errorf("vault response missing data field")
	}

	return vaultResp.Data, nil
}

// Bao backend for OpenBao (same as Vault but different name)
type Bao struct {
	*Vault
}

// NewBao creates a new Bao backend (same as Vault)
func NewBao(config VaultConfig) *Bao {
	return &Bao{
		Vault: NewVault(config),
	}
}

func (b *Bao) Name() string {
	return "bao"
}

// ReadRef reads a secret from Bao using bao:// URI scheme
func (b *Bao) ReadRef(ctx context.Context, ref string) (string, error) {
	// Convert bao:// to vault:// for processing
	if strings.HasPrefix(ref, "bao://") {
		ref = "vault://" + strings.TrimPrefix(ref, "bao://")
	}
	return b.Vault.ReadRef(ctx, ref)
}

func (b *Bao) ReadRefWithFlags(ctx context.Context, ref string, flags []string) (string, error) {
	// Convert bao:// to vault:// for processing
	if strings.HasPrefix(ref, "bao://") {
		ref = "vault://" + strings.TrimPrefix(ref, "bao://")
	}
	return b.Vault.ReadRefWithFlags(ctx, ref, flags)
}
