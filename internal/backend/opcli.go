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

type OpCLI struct{}

func (OpCLI) Name() string { return "opcli" }

// ReadRef shells out to `op read <ref>` and trims trailing newlines.
func (OpCLI) ReadRef(ctx context.Context, ref string) (string, error) {
	return OpCLI{}.ReadRefWithFlags(ctx, ref, nil)
}

// ReadRefWithFlags shells out to `op read` with additional flags and trims trailing newlines.
func (OpCLI) ReadRefWithFlags(ctx context.Context, ref string, flags []string) (string, error) {
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

	cmd := exec.CommandContext(ctx, "op", args...)

	// Set up account context isolation for multi-account support in single daemon
	cmd.Env = createAccountIsolatedEnv(flags)

	var out, errb bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &errb
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("op read failed: %w; stderr=%s", err, strings.TrimSpace(errb.String()))
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
