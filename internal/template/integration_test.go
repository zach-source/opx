package template

import (
	"context"
	"testing"
)

func TestEndToEndTemplateIntegration(t *testing.T) {
	// Test basic template processing flow
	processor := NewProcessor()

	tests := []struct {
		name     string
		ref      string
		value    string
		expected string
	}{
		{
			name:     "base64 encoding integration",
			ref:      "op://vault/item/field?template={{.Value | base64encode}}",
			value:    "mysecret",
			expected: "bXlzZWNyZXQ=",
		},
		{
			name:     "default value integration",
			ref:      "op://vault/item/field?template={{.Value | default \"fallback\"}}",
			value:    "",
			expected: "fallback",
		},
		{
			name:     "complex transformation",
			ref:      "op://vault/item/field?template={{.Value | upper | trim}}",
			value:    " secret ",
			expected: "SECRET",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Parse reference
			baseRef, templateStr, err := ParseReferenceWithTemplate(tt.ref)
			if err != nil {
				t.Fatalf("Parse error: %v", err)
			}

			// Verify base reference
			expectedBase := "op://vault/item/field"
			if baseRef != expectedBase {
				t.Errorf("Expected base ref %q, got %q", expectedBase, baseRef)
			}

			// Process template
			if templateStr != "" {
				result, err := processor.ProcessTemplate(context.Background(), templateStr, tt.value)
				if err != nil {
					t.Fatalf("Template processing error: %v", err)
				}
				if result != tt.expected {
					t.Errorf("Expected %q, got %q", tt.expected, result)
				}
			}
		})
	}
}

func TestTemplateErrorHandling(t *testing.T) {
	processor := NewProcessor()

	errorTests := []struct {
		name    string
		ref     string
		value   string
		wantErr bool
	}{
		{
			name:    "invalid template syntax",
			ref:     "op://vault/item/field?template={{.Value | invalidfunc}}",
			value:   "test",
			wantErr: true,
		},
		{
			name:    "blocked function",
			ref:     "op://vault/item/field?template={{env \"HOME\"}}",
			value:   "test",
			wantErr: true,
		},
		{
			name:    "valid template",
			ref:     "op://vault/item/field?template={{.Value | upper}}",
			value:   "test",
			wantErr: false,
		},
	}

	for _, tt := range errorTests {
		t.Run(tt.name, func(t *testing.T) {
			_, templateStr, err := ParseReferenceWithTemplate(tt.ref)
			if err != nil {
				if !tt.wantErr {
					t.Fatalf("Unexpected parse error: %v", err)
				}
				return
			}

			if templateStr != "" {
				_, err = processor.ProcessTemplate(context.Background(), templateStr, tt.value)
				if tt.wantErr && err == nil {
					t.Error("Expected template processing error but got none")
				}
				if !tt.wantErr && err != nil {
					t.Errorf("Unexpected template processing error: %v", err)
				}
			}
		})
	}
}
