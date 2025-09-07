package opaccount

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
)

// Account represents a 1Password account
type Account struct {
	URL    string `json:"url"`
	Email  string `json:"email"`
	UserID string `json:"user_id"`
}

// GetAvailableAccounts retrieves list of configured 1Password accounts
func GetAvailableAccounts(ctx context.Context) ([]Account, error) {
	cmd := exec.CommandContext(ctx, "op", "account", "list", "--format=json")
	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("failed to list accounts: %w", err)
	}

	var accounts []Account
	if err := json.Unmarshal(output, &accounts); err != nil {
		return nil, fmt.Errorf("failed to parse accounts: %w", err)
	}

	return accounts, nil
}

// SelectAccountInteractively prompts user to select an account
func SelectAccountInteractively(ctx context.Context) (*Account, error) {
	accounts, err := GetAvailableAccounts(ctx)
	if err != nil {
		return nil, err
	}

	if len(accounts) == 0 {
		return nil, fmt.Errorf("no 1Password accounts configured")
	}

	if len(accounts) == 1 {
		return &accounts[0], nil
	}

	// Multiple accounts - prompt user
	fmt.Println("Multiple 1Password accounts available:")
	for i, account := range accounts {
		fmt.Printf("  [%d] %s (%s)\n", i+1, account.Email, account.URL)
	}
	fmt.Print("Select account (1-" + fmt.Sprintf("%d", len(accounts)) + "): ")

	reader := bufio.NewReader(os.Stdin)
	input, err := reader.ReadString('\n')
	if err != nil {
		return nil, fmt.Errorf("failed to read input: %w", err)
	}

	choice, err := strconv.Atoi(strings.TrimSpace(input))
	if err != nil || choice < 1 || choice > len(accounts) {
		return nil, fmt.Errorf("invalid selection: %s", strings.TrimSpace(input))
	}

	return &accounts[choice-1], nil
}

// GetCurrentAccount gets the currently signed-in account
func GetCurrentAccount(ctx context.Context) (*Account, error) {
	cmd := exec.CommandContext(ctx, "op", "whoami", "--format=json")
	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("no active 1Password session")
	}

	var account Account
	if err := json.Unmarshal(output, &account); err != nil {
		return nil, fmt.Errorf("failed to parse current account: %w", err)
	}

	return &account, nil
}

// ValidateAccountSession checks if a specific account session is valid
func ValidateAccountSession(ctx context.Context, userID string) error {
	if userID == "" {
		// No specific account, try general whoami
		cmd := exec.CommandContext(ctx, "op", "whoami")
		return cmd.Run()
	}

	// Account-specific validation
	cmd := exec.CommandContext(ctx, "op", "whoami", "--account", userID)
	return cmd.Run()
}

// SignInToAccount performs interactive signin to a specific account
func SignInToAccount(ctx context.Context, account *Account) error {
	args := []string{"signin"}
	if account.UserID != "" {
		args = append(args, "--account", account.UserID)
	}

	cmd := exec.CommandContext(ctx, "op", args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Stdin = os.Stdin

	return cmd.Run()
}

// GetPreferredAccount gets account from environment or prompts for selection
func GetPreferredAccount(ctx context.Context) (*Account, error) {
	// Check if account specified via environment
	if accountID := os.Getenv("OP_ACCOUNT"); accountID != "" {
		accounts, err := GetAvailableAccounts(ctx)
		if err != nil {
			return nil, err
		}

		// Find account by ID
		for _, account := range accounts {
			if account.UserID == accountID {
				return &account, nil
			}
		}

		return nil, fmt.Errorf("account %s not found in configured accounts", accountID)
	}

	// No environment variable, check if already signed in
	if current, err := GetCurrentAccount(ctx); err == nil {
		return current, nil
	}

	// Multiple accounts and none selected - prompt user
	return SelectAccountInteractively(ctx)
}
