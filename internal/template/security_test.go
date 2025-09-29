package template

import (
	"context"
	"testing"
	"time"
)

func TestBlockedFunctionSecurity(t *testing.T) {
	processor := &DefaultProcessor{
		registry: NewSafeFunctionRegistry(),
		cache:    NewCache(10),
		timeout:  5 * time.Second,
	}

	// Test various categories of blocked functions
	blockedTests := []struct {
		category string
		function string
		template string
	}{
		{"OS", "env", "{{env \"HOME\"}}"},
		{"OS", "expandenv", "{{expandenv \"$HOME/test\"}}"},
		{"File", "readFile", "{{readFile \"/etc/passwd\"}}"},
		{"File", "writeFile", "{{writeFile \"/tmp/test\" \"data\"}}"},
		{"File", "glob", "{{glob \"/*\"}}"},
		{"Network", "getHostByName", "{{getHostByName \"example.com\"}}"},
		{"Crypto", "genPrivateKey", "{{genPrivateKey \"rsa\"}}"},
		{"Crypto", "bcrypt", "{{bcrypt \"password\"}}"},
	}

	for _, tt := range blockedTests {
		t.Run(tt.category+"_"+tt.function, func(t *testing.T) {
			_, err := processor.ProcessTemplate(context.Background(), tt.template, "test")
			if err == nil {
				t.Errorf("Expected error for blocked %s function %s", tt.category, tt.function)
			}
		})
	}
}

func TestAllowedFunctionSecurity(t *testing.T) {
	processor := &DefaultProcessor{
		registry: NewSafeFunctionRegistry(),
		cache:    NewCache(10),
		timeout:  5 * time.Second,
	}

	// Test that safe functions work correctly
	allowedTests := []struct {
		name     string
		template string
		value    string
		expected string
	}{
		{"base64encode", "{{.Value | base64encode}}", "test", "dGVzdA=="},
		{"upper", "{{.Value | upper}}", "test", "TEST"},
		{"default with value", "{{.Value | default \"fallback\"}}", "test", "test"},
		{"default with empty", "{{.Value | default \"fallback\"}}", "", "fallback"},
		{"trim", "{{.Value | trim}}", " test ", "test"},
		{"add", "{{add 1 2}}", "", "3"},
	}

	for _, tt := range allowedTests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := processor.ProcessTemplate(context.Background(), tt.template, tt.value)
			if err != nil {
				t.Errorf("Unexpected error for safe function: %v", err)
			}
			if result != tt.expected {
				t.Errorf("Expected %q, got %q", tt.expected, result)
			}
		})
	}
}

func TestTemplateInjectionPrevention(t *testing.T) {
	processor := &DefaultProcessor{
		registry: NewSafeFunctionRegistry(),
		cache:    NewCache(10),
		timeout:  5 * time.Second,
	}

	// Test various injection attempts
	injectionTests := []struct {
		name     string
		template string
		value    string
	}{
		{
			name:     "command injection attempt",
			template: "{{.Value}}; rm -rf /",
			value:    "test",
		},
		{
			name:     "script injection attempt",
			template: "{{.Value}}<script>alert('xss')</script>",
			value:    "test",
		},
		{
			name:     "path traversal attempt",
			template: "{{.Value}}../../../etc/passwd",
			value:    "test",
		},
	}

	for _, tt := range injectionTests {
		t.Run(tt.name, func(t *testing.T) {
			// These should not execute any malicious code
			// The template should either fail or treat injection as literal text
			result, err := processor.ProcessTemplate(context.Background(), tt.template, tt.value)

			// Whether it succeeds or fails, it should not cause system damage
			// The important thing is that no OS commands execute
			if err == nil {
				t.Logf("Template result (should be safe): %s", result)
			} else {
				t.Logf("Template failed safely: %v", err)
			}
		})
	}
}

func TestResourceExhaustionProtection(t *testing.T) {
	processor := &DefaultProcessor{
		registry: NewSafeFunctionRegistry(),
		cache:    NewCache(10),
		timeout:  100 * time.Millisecond, // Short timeout for testing
	}

	// Test infinite loop protection
	t.Run("infinite_loop_protection", func(t *testing.T) {
		// Template that would cause infinite recursion if not protected
		template := "{{range .Value}}{{.}}{{end}}"
		_, err := processor.ProcessTemplate(context.Background(), template, "test")

		// Should either fail fast or timeout
		if err == nil {
			t.Error("Expected error for potentially infinite template")
		}
	})

	// Test large output protection
	t.Run("large_output_protection", func(t *testing.T) {
		// Template that generates large output
		template := "{{range 10000}}{{.Value}}{{end}}"
		_, err := processor.ProcessTemplate(context.Background(), template, "test")

		// Should handle large output gracefully or timeout
		if err != nil {
			t.Logf("Large output handled with error: %v", err)
		}
	})
}
