package backend

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"
)

type OpCLI struct {
	opPath string
}

func (OpCLI) Name() string { return "opcli" }

func NewOpCLI(opPath string) (*OpCLI, error) {
	if opPath == "" {
		path, err := exec.LookPath("op")
		if err != nil {
			return nil, fmt.Errorf("1Password CLI not found in PATH: %w", err)
		}
		opPath = path
	}
	return &OpCLI{opPath: opPath}, nil
}

// ReadRef shells out to `op read <ref>` and trims trailing newlines.
func (o *OpCLI) ReadRef(ctx context.Context, ref string) (string, error) {
	return o.ReadRefWithFlags(ctx, ref, nil)
}

// ReadRefWithFlags shells out to `op read` with additional flags and trims trailing newlines.
func (o *OpCLI) ReadRefWithFlags(ctx context.Context, ref string, flags []string) (string, error) {
	if strings.TrimSpace(ref) == "" {
		return "", errors.New("empty ref")
	}

	// Prevent command injection: refs cannot start with dash (flag injection)
	if strings.HasPrefix(ref, "-") {
		return "", errors.New("invalid reference format: cannot start with dash")
	}

	// Validate reference format: must match op://[vault]/[item]/[field] pattern
	if !strings.HasPrefix(ref, "op://") {
		return "", errors.New("invalid reference format: must start with op://")
	}

	// Validate flags: each flag must start with dash and contain safe characters
	for _, flag := range flags {
		if flag == "" {
			continue
		}
		if !strings.HasPrefix(flag, "-") {
			return "", errors.New("invalid flag format: must start with dash")
		}
		// Check for command injection attempts in flags
		if strings.ContainsAny(flag, ";&|`$()") {
			return "", errors.New("invalid flag format: contains unsafe characters")
		}
	}

	// Build command args: op [global-flags] read --no-color ref
	args := []string{}

	// Add global flags first (like --account)
	for _, flag := range flags {
		if flag != "" {
			args = append(args, flag)
		}
	}

	// Add the read subcommand and its flags
	args = append(args, "read", "--no-color", ref)

	cmd := exec.CommandContext(ctx, o.opPath, args...)
	cmd.Stdin = nil

	var out, errb bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &errb
	if err := cmd.Run(); err != nil {
		stderr := strings.TrimSpace(errb.String())
		if strings.Contains(stderr, "not signed in") || strings.Contains(stderr, "not currently signed in") {
			accountID := extractAccountFromFlags(flags)
			if accountID != "" {
				return "", fmt.Errorf("account %s not signed in. Run: opx login 1password --account=%s", accountID, accountID)
			}
			return "", errors.New("not signed in to 1Password. Run: opx login 1password")
		}
		return "", fmt.Errorf("op read failed: %w; stderr=%s", err, stderr)
	}
	// Trim one trailing newline without nuking legitimate whitespace
	s := out.String()
	s = strings.TrimRight(s, "\n")
	return s, nil
}

// createAccountIsolatedEnv creates an isolated environment for account-specific op CLI calls
func createAccountIsolatedEnv(flags []string) []string {
	// Start with current process environment
	env := os.Environ()

	// Extract account ID from flags
	accountID := extractAccountFromFlags(flags)
	if accountID == "" {
		// No account specified, use current environment
		return env
	}

	// Create isolated environment with account override
	var newEnv []string
	opAccountSet := false

	for _, envVar := range env {
		if strings.HasPrefix(envVar, "OP_ACCOUNT=") {
			// Replace existing OP_ACCOUNT with account from flags
			newEnv = append(newEnv, "OP_ACCOUNT="+accountID)
			opAccountSet = true
		} else {
			newEnv = append(newEnv, envVar)
		}
	}

	// Add OP_ACCOUNT if it wasn't present in environment
	if !opAccountSet {
		newEnv = append(newEnv, "OP_ACCOUNT="+accountID)
	}

	return newEnv
}

// extractAccountFromFlags extracts account ID from command flags
func extractAccountFromFlags(flags []string) string {
	for _, flag := range flags {
		if strings.HasPrefix(flag, "--account=") {
			return strings.TrimPrefix(flag, "--account=")
		}
	}
	return ""
}

func WithTimeout(parent context.Context, d time.Duration) (context.Context, context.CancelFunc) {
	if d <= 0 {
		return parent, func() {}
	}
	return context.WithTimeout(parent, d)
}
