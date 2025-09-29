package template

import (
	"strings"
	"testing"
)

func TestParseReferenceWithTemplate(t *testing.T) {
	tests := []struct {
		name         string
		input        string
		expectedBase string
		expectedTmpl string
		wantErr      bool
	}{
		{
			name:         "reference without template",
			input:        "op://vault/item/field",
			expectedBase: "op://vault/item/field",
			expectedTmpl: "",
			wantErr:      false,
		},
		{
			name:         "reference with base64 template",
			input:        "op://vault/item/field?template={{.Value | base64encode}}",
			expectedBase: "op://vault/item/field",
			expectedTmpl: "{{.Value | base64encode}}",
			wantErr:      false,
		},
		{
			name:         "reference with default template",
			input:        "op://vault/item/field?template={{.Value | default \"fallback\"}}",
			expectedBase: "op://vault/item/field",
			expectedTmpl: "{{.Value | default \"fallback\"}}",
			wantErr:      false,
		},
		{
			name:         "reference with complex template",
			input:        "op://vault/item/field?template={{.Value | upper | trim | base64encode}}",
			expectedBase: "op://vault/item/field",
			expectedTmpl: "{{.Value | upper | trim | base64encode}}",
			wantErr:      false,
		},
		{
			name:         "reference with empty template parameter",
			input:        "op://vault/item/field?template=",
			expectedBase: "op://vault/item/field",
			expectedTmpl: "",
			wantErr:      false,
		},
		{
			name:         "reference with additional query parameters",
			input:        "op://vault/item/field?foo=bar&template={{.Value | base64encode}}&baz=qux",
			expectedBase: "op://vault/item/field?foo=bar&baz=qux",
			expectedTmpl: "{{.Value | base64encode}}",
			wantErr:      false,
		},
		{
			name:         "invalid URL format",
			input:        "op://vault/item/field?template={{.Value | base64encode}}&invalid%url",
			expectedBase: "",
			expectedTmpl: "",
			wantErr:      true,
		},
		{
			name:         "vault reference with template",
			input:        "vault://secret/data/myapp?template={{.Value | default \"config\"}}",
			expectedBase: "vault://secret/data/myapp",
			expectedTmpl: "{{.Value | default \"config\"}}",
			wantErr:      false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			baseRef, templateStr, err := ParseReferenceWithTemplate(tt.input)

			if tt.wantErr && err == nil {
				t.Error("Expected error but got none")
			}
			if !tt.wantErr && err != nil {
				t.Errorf("Unexpected error: %v", err)
			}

			if baseRef != tt.expectedBase {
				t.Errorf("Expected base ref %q, got %q", tt.expectedBase, baseRef)
			}
			if templateStr != tt.expectedTmpl {
				t.Errorf("Expected template %q, got %q", tt.expectedTmpl, templateStr)
			}
		})
	}
}

func TestValidateTemplateString(t *testing.T) {
	tests := []struct {
		name     string
		template string
		wantErr  bool
	}{
		{
			name:     "valid simple template",
			template: "{{.Value}}",
			wantErr:  false,
		},
		{
			name:     "valid template with function",
			template: "{{.Value | base64encode}}",
			wantErr:  false,
		},
		{
			name:     "empty template",
			template: "",
			wantErr:  true,
		},
		{
			name:     "template too large",
			template: strings.Repeat("{{.Value}}", 200), // > 1KB
			wantErr:  true,
		},
		{
			name:     "unmatched opening brace",
			template: "{{.Value",
			wantErr:  true,
		},
		{
			name:     "unmatched closing brace",
			template: "{{.Value}}}",
			wantErr:  true,
		},
		{
			name:     "multiple unmatched braces",
			template: "{{{.Value}}",
			wantErr:  true,
		},
		{
			name:     "complex valid template",
			template: "{{.Value | default \"test\" | upper | trim}}",
			wantErr:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateTemplateString(tt.template)
			if tt.wantErr && err == nil {
				t.Error("Expected validation error but got none")
			}
			if !tt.wantErr && err != nil {
				t.Errorf("Unexpected validation error: %v", err)
			}
		})
	}
}
