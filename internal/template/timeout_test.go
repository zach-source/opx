package template

import (
	"context"
	"testing"
	"time"
)

func TestTemplateTimeout(t *testing.T) {
	// Short timeout for testing
	processor := &DefaultProcessor{
		registry: NewSafeFunctionRegistry(),
		cache:    NewCache(10),
		timeout:  100 * time.Millisecond,
	}

	tests := []struct {
		name        string
		template    string
		value       string
		wantTimeout bool
	}{
		{
			name:        "simple template (should not timeout)",
			template:    "{{.Value | base64encode}}",
			value:       "test",
			wantTimeout: false,
		},
		{
			name:        "complex template (may timeout)",
			template:    "{{range 1000000}}{{.Value}}{{end}}",
			value:       "test",
			wantTimeout: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			start := time.Now()
			_, err := processor.ProcessTemplate(context.Background(), tt.template, tt.value)
			duration := time.Since(start)

			if tt.wantTimeout {
				if err == nil {
					t.Error("Expected timeout error but got none")
				}
				if duration > 200*time.Millisecond {
					t.Errorf("Timeout took too long: %v (expected ~100ms)", duration)
				}
			} else {
				if err != nil {
					t.Errorf("Unexpected error for simple template: %v", err)
				}
			}
		})
	}
}

func TestContextCancellation(t *testing.T) {
	processor := &DefaultProcessor{
		registry: NewSafeFunctionRegistry(),
		cache:    NewCache(10),
		timeout:  5 * time.Second,
	}

	// Test with pre-cancelled context
	t.Run("cancelled_context", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		cancel() // Cancel immediately

		_, err := processor.ProcessTemplate(ctx, "{{.Value}}", "test")
		if err == nil {
			t.Error("Expected error for cancelled context")
		}
	})

	// Test context timeout
	t.Run("context_timeout", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
		defer cancel()

		// Template that might take longer than context timeout
		_, err := processor.ProcessTemplate(ctx, "{{range 100000}}{{.Value}}{{end}}", "test")
		if err == nil {
			t.Error("Expected timeout error")
		}
	})
}

func TestTimeoutRecovery(t *testing.T) {
	processor := &DefaultProcessor{
		registry: NewSafeFunctionRegistry(),
		cache:    NewCache(10),
		timeout:  50 * time.Millisecond,
	}

	// First template times out
	_, err := processor.ProcessTemplate(context.Background(), "{{range 100000}}{{.Value}}{{end}}", "test")
	if err == nil {
		t.Error("Expected timeout error for first template")
	}

	// Second template should work normally (processor should recover)
	result, err := processor.ProcessTemplate(context.Background(), "{{.Value | upper}}", "test")
	if err != nil {
		t.Errorf("Processor should recover after timeout: %v", err)
	}
	if result != "TEST" {
		t.Errorf("Expected TEST, got %s", result)
	}
}

func TestProcessorTimeoutConfiguration(t *testing.T) {
	// Test with different timeout values
	tests := []struct {
		name    string
		timeout time.Duration
	}{
		{"very_short", 1 * time.Millisecond},
		{"short", 100 * time.Millisecond},
		{"normal", 5 * time.Second},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			processor := &DefaultProcessor{
				registry: NewSafeFunctionRegistry(),
				cache:    NewCache(10),
				timeout:  tt.timeout,
			}

			start := time.Now()
			_, err := processor.ProcessTemplate(context.Background(), "{{range 10000}}{{.Value}}{{end}}", "x")
			duration := time.Since(start)

			// Should respect the configured timeout
			if duration > tt.timeout+50*time.Millisecond {
				t.Errorf("Template execution exceeded timeout: %v > %v", duration, tt.timeout)
			}

			if tt.timeout < 10*time.Millisecond && err == nil {
				t.Error("Expected timeout error for very short timeout")
			}
		})
	}
}
