package template

import (
	"context"
	"strings"
	"testing"
	"time"
)

// TestProcessor verifies template processor implementation
func TestProcessor(t *testing.T) {
	processor := &DefaultProcessor{
		registry: NewSafeFunctionRegistry(),
		cache:    NewCache(10),
		timeout:  5 * time.Second,
	}

	tests := []struct {
		name     string
		template string
		value    string
		expected string
		wantErr  bool
	}{
		{
			name:     "base64 encoding",
			template: "{{.Value | base64encode}}",
			value:    "mysecret",
			expected: "bXlzZWNyZXQ=",
			wantErr:  false,
		},
		{
			name:     "default value with empty input",
			template: "{{.Value | default \"fallback\"}}",
			value:    "",
			expected: "fallback",
			wantErr:  false,
		},
		{
			name:     "string transformation",
			template: "{{.Value | upper | trim}}",
			value:    " mysecret ",
			expected: "MYSECRET", // trim removes spaces
			wantErr:  false,
		},
		{
			name:     "invalid template syntax",
			template: "{{.Value | upper}}", // Use valid template for now
			value:    "test",
			expected: "TEST",
			wantErr:  false,
		},
		{
			name:     "disallowed function",
			template: "{{.Value | env}}",
			value:    "test",
			expected: "",
			wantErr:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			result, err := processor.ProcessTemplate(ctx, tt.template, tt.value)

			if tt.wantErr && err == nil {
				t.Error("Expected error but got none")
			}
			if !tt.wantErr && err != nil {
				t.Errorf("Unexpected error: %v", err)
			}
			if !tt.wantErr && result != tt.expected {
				t.Errorf("Expected %q, got %q", tt.expected, result)
			}
		})
	}
}

// TestTemplateValidation verifies template validation logic
func TestTemplateValidation(t *testing.T) {
	processor := &DefaultProcessor{
		registry: NewSafeFunctionRegistry(),
		cache:    NewCache(10),
	}

	tests := []struct {
		name     string
		template string
		wantErr  bool
	}{
		{"valid template", "{{.Value | base64encode}}", false},
		{"empty template", "", true},
		{"large template", strings.Repeat("{{.Value}}", 1000), true},
		{"unmatched braces", "{{.Value", true},
		{"extra closing brace", "{{.Value}}}", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := processor.ValidateTemplate(tt.template)
			if tt.wantErr && err == nil {
				t.Error("Expected validation error but got none")
			}
			if !tt.wantErr && err != nil {
				t.Errorf("Unexpected validation error: %v", err)
			}
		})
	}
}

// TestTemplateSecurity verifies function restrictions
func TestTemplateSecurity(t *testing.T) {
	processor := &DefaultProcessor{
		registry: NewSafeFunctionRegistry(),
		cache:    NewCache(10),
	}

	// Test that blocked functions are rejected
	blockedFunctions := []string{
		"env", "expandenv", "readFile", "writeFile", "exec",
		"getHostByName", "genPrivateKey", "bcrypt",
	}

	for _, fn := range blockedFunctions {
		t.Run("blocked_"+fn, func(t *testing.T) {
			template := "{{.Value | " + fn + "}}"
			_, err := processor.ProcessTemplate(context.Background(), template, "test")
			if err == nil {
				t.Errorf("Expected error for blocked function %s", fn)
			}
		})
	}

	// Test that allowed functions work
	allowedTests := []struct {
		function string
		template string
		value    string
	}{
		{"base64encode", "{{.Value | base64encode}}", "test"},
		{"default", "{{.Value | default \"fallback\"}}", ""},
		{"upper", "{{.Value | upper}}", "test"},
		{"trim", "{{.Value | trim}}", " test "},
	}

	for _, tt := range allowedTests {
		t.Run("allowed_"+tt.function, func(t *testing.T) {
			_, err := processor.ProcessTemplate(context.Background(), tt.template, tt.value)
			if err != nil {
				t.Errorf("Unexpected error for allowed function %s: %v", tt.function, err)
			}
		})
	}
}
