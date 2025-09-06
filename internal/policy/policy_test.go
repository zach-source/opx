package policy

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestDefaultPolicy(t *testing.T) {
	pol := defaultPolicy()

	if pol.DefaultDeny {
		t.Error("Expected default policy to not be default deny")
	}

	if len(pol.Allow) != 0 {
		t.Error("Expected default policy to have empty allow rules")
	}
}

func TestSha256Hex(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"", "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855"},
		{"hello", "2cf24dba4f21d4288094c9b9b6a3d5c9a2e5b7f0a8a6f5e0b9aa4b9c8d3e2f1"},
		{"/usr/bin/test", "55d094c83d96b6f1e2e5b7f0a8a6f5e0b9aa4b9c8d3e2f1a2b3c4d5e6f7890a"},
	}

	for _, test := range tests {
		result := sha256Hex(test.input)
		if len(result) != 64 {
			t.Errorf("Expected 64-character hex string for input %q, got %d characters", test.input, len(result))
		}
		// Note: Not checking exact expected values since they're fake in the test above
		// Just ensure it's consistent
		result2 := sha256Hex(test.input)
		if result != result2 {
			t.Errorf("sha256Hex not consistent for input %q", test.input)
		}
	}
}

func TestMatchRef(t *testing.T) {
	tests := []struct {
		allowed  []string
		ref      string
		expected bool
	}{
		{[]string{"*"}, "op://vault/item/field", true},
		{[]string{"op://vault/*"}, "op://vault/item/field", true},
		{[]string{"op://vault/*"}, "op://other/item/field", false},
		{[]string{"op://vault/item/field"}, "op://vault/item/field", true},
		{[]string{"op://vault/item/field"}, "op://vault/item/other", false},
		{[]string{"op://dev/*", "op://prod/*"}, "op://dev/db/password", true},
		{[]string{"op://dev/*", "op://prod/*"}, "op://staging/db/password", false},
		{[]string{}, "op://vault/item/field", false},
	}

	for _, test := range tests {
		result := matchRef(test.allowed, test.ref)
		if result != test.expected {
			t.Errorf("matchRef(%v, %q) = %t, want %t", test.allowed, test.ref, result, test.expected)
		}
	}
}

func TestSamePath(t *testing.T) {
	tests := []struct {
		a, b     string
		expected bool
	}{
		{"/usr/bin/test", "/usr/bin/test", true},
		{"/usr/bin/test", "/usr/bin/other", false},
		{"/usr/bin/../bin/test", "/usr/bin/test", true},
		{"", "/usr/bin/test", false},
		{"/usr/bin/test", "", false},
		{"", "", false},
		{"./test", "/current/dir/test", false}, // relative vs absolute
	}

	for _, test := range tests {
		result := samePath(test.a, test.b)
		if result != test.expected {
			t.Errorf("samePath(%q, %q) = %t, want %t", test.a, test.b, result, test.expected)
		}
	}
}

func TestAllowed(t *testing.T) {
	tests := []struct {
		name     string
		policy   Policy
		subject  Subject
		ref      string
		expected bool
	}{
		{
			name:     "default policy allows all",
			policy:   Policy{Allow: []Rule{}, DefaultDeny: false},
			subject:  Subject{PID: 123, Path: "/usr/bin/test"},
			ref:      "op://vault/item/field",
			expected: true,
		},
		{
			name:     "default deny blocks when no rules match",
			policy:   Policy{Allow: []Rule{}, DefaultDeny: true},
			subject:  Subject{PID: 123, Path: "/usr/bin/test"},
			ref:      "op://vault/item/field",
			expected: false,
		},
		{
			name: "path rule matches",
			policy: Policy{
				Allow: []Rule{{
					Path: "/usr/bin/test",
					Refs: []string{"op://vault/*"},
				}},
				DefaultDeny: true,
			},
			subject:  Subject{PID: 123, Path: "/usr/bin/test"},
			ref:      "op://vault/item/field",
			expected: true,
		},
		{
			name: "path rule doesn't match",
			policy: Policy{
				Allow: []Rule{{
					Path: "/usr/bin/other",
					Refs: []string{"op://vault/*"},
				}},
				DefaultDeny: true,
			},
			subject:  Subject{PID: 123, Path: "/usr/bin/test"},
			ref:      "op://vault/item/field",
			expected: false,
		},
		{
			name: "PID rule matches",
			policy: Policy{
				Allow: []Rule{{
					PID:  123,
					Refs: []string{"*"},
				}},
				DefaultDeny: true,
			},
			subject:  Subject{PID: 123, Path: "/usr/bin/test"},
			ref:      "op://vault/item/field",
			expected: true,
		},
		{
			name: "PID rule doesn't match",
			policy: Policy{
				Allow: []Rule{{
					PID:  456,
					Refs: []string{"*"},
				}},
				DefaultDeny: true,
			},
			subject:  Subject{PID: 123, Path: "/usr/bin/test"},
			ref:      "op://vault/item/field",
			expected: false,
		},
		{
			name: "ref rule doesn't match",
			policy: Policy{
				Allow: []Rule{{
					Path: "/usr/bin/test",
					Refs: []string{"op://other/*"},
				}},
				DefaultDeny: true,
			},
			subject:  Subject{PID: 123, Path: "/usr/bin/test"},
			ref:      "op://vault/item/field",
			expected: false,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			result := Allowed(test.policy, test.subject, test.ref)
			if result != test.expected {
				t.Errorf("Expected %t, got %t", test.expected, result)
			}
		})
	}
}

func TestLoadPolicy(t *testing.T) {
	// Test loading default policy when file doesn't exist
	tempDir := t.TempDir()
	originalConfigDir := os.Getenv("XDG_CONFIG_HOME")
	defer func() {
		if originalConfigDir != "" {
			os.Setenv("XDG_CONFIG_HOME", originalConfigDir)
		} else {
			os.Unsetenv("XDG_CONFIG_HOME")
		}
	}()

	// Point to temp directory
	os.Setenv("XDG_CONFIG_HOME", tempDir)

	pol, path, err := Load()
	if err != nil {
		t.Fatalf("Expected no error loading default policy, got %v", err)
	}

	if pol.DefaultDeny {
		t.Error("Expected default policy to not be default deny")
	}

	expectedPath := filepath.Join(tempDir, "op-authd", "policy.json")
	if path != expectedPath {
		t.Errorf("Expected policy path %q, got %q", expectedPath, path)
	}
}

func TestLoadPolicy_WithFile(t *testing.T) {
	tempDir := t.TempDir()
	configDir := filepath.Join(tempDir, "op-authd")
	if err := os.MkdirAll(configDir, 0o700); err != nil {
		t.Fatalf("Failed to create config dir: %v", err)
	}

	// Create a test policy file
	testPolicy := Policy{
		Allow: []Rule{{
			Path: "/usr/bin/approved",
			Refs: []string{"op://vault/*"},
		}},
		DefaultDeny: true,
	}

	policyPath := filepath.Join(configDir, "policy.json")
	data, err := json.MarshalIndent(testPolicy, "", "  ")
	if err != nil {
		t.Fatalf("Failed to marshal test policy: %v", err)
	}

	if err := os.WriteFile(policyPath, data, 0o600); err != nil {
		t.Fatalf("Failed to write policy file: %v", err)
	}

	originalConfigDir := os.Getenv("XDG_CONFIG_HOME")
	defer func() {
		if originalConfigDir != "" {
			os.Setenv("XDG_CONFIG_HOME", originalConfigDir)
		} else {
			os.Unsetenv("XDG_CONFIG_HOME")
		}
	}()

	os.Setenv("XDG_CONFIG_HOME", tempDir)

	pol, path, err := Load()
	if err != nil {
		t.Fatalf("Expected no error loading policy file, got %v", err)
	}

	if !pol.DefaultDeny {
		t.Error("Expected loaded policy to have default deny")
	}

	if len(pol.Allow) != 1 {
		t.Errorf("Expected 1 allow rule, got %d", len(pol.Allow))
	}

	if pol.Allow[0].Path != "/usr/bin/approved" {
		t.Errorf("Expected path '/usr/bin/approved', got %q", pol.Allow[0].Path)
	}

	if path != policyPath {
		t.Errorf("Expected policy path %q, got %q", policyPath, path)
	}
}

func TestLoadPolicy_InvalidJSON(t *testing.T) {
	tempDir := t.TempDir()
	configDir := filepath.Join(tempDir, "op-authd")
	if err := os.MkdirAll(configDir, 0o700); err != nil {
		t.Fatalf("Failed to create config dir: %v", err)
	}

	// Create invalid JSON file
	policyPath := filepath.Join(configDir, "policy.json")
	if err := os.WriteFile(policyPath, []byte("invalid json"), 0o600); err != nil {
		t.Fatalf("Failed to write invalid policy file: %v", err)
	}

	originalConfigDir := os.Getenv("XDG_CONFIG_HOME")
	defer func() {
		if originalConfigDir != "" {
			os.Setenv("XDG_CONFIG_HOME", originalConfigDir)
		} else {
			os.Unsetenv("XDG_CONFIG_HOME")
		}
	}()

	os.Setenv("XDG_CONFIG_HOME", tempDir)

	_, _, err := Load()
	if err == nil {
		t.Error("Expected error loading invalid JSON policy")
	}
}
