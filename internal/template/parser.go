package template

import (
	"fmt"
	"net/url"
	"strings"
)

// ParseReferenceWithTemplate extracts template from reference query parameter
func ParseReferenceWithTemplate(ref string) (baseRef, templateStr string, err error) {
	// Check if reference contains template query parameter
	if !strings.Contains(ref, "?template=") {
		return ref, "", nil // No template
	}

	// Parse as URL to extract query parameters
	u, err := url.Parse(ref)
	if err != nil {
		return "", "", fmt.Errorf("invalid reference URL format: %w", err)
	}

	// Extract template parameter
	templateStr = u.Query().Get("template")
	if templateStr == "" {
		// Handle empty template parameter case
		q := u.Query()
		q.Del("template")
		u.RawQuery = q.Encode()
		baseRef = u.String()
		if strings.HasSuffix(baseRef, "?") {
			baseRef = strings.TrimSuffix(baseRef, "?")
		}
		return baseRef, "", nil
	}

	// Remove template query parameter to get base reference
	q := u.Query()
	q.Del("template")
	u.RawQuery = q.Encode()

	// Clean up URL if no query parameters remain
	baseRef = u.String()
	if strings.HasSuffix(baseRef, "?") {
		baseRef = strings.TrimSuffix(baseRef, "?")
	}

	return baseRef, templateStr, nil
}

// ValidateTemplateString performs basic validation on template string
func ValidateTemplateString(templateStr string) error {
	if templateStr == "" {
		return fmt.Errorf("empty template not allowed")
	}

	if len(templateStr) > 1024 { // 1KB limit
		return fmt.Errorf("template too large: %d bytes (max 1024)", len(templateStr))
	}

	// Basic syntax validation - check for balanced braces
	braceCount := 0
	for _, char := range templateStr {
		if char == '{' {
			braceCount++
		} else if char == '}' {
			braceCount--
			if braceCount < 0 {
				return fmt.Errorf("unmatched closing brace in template")
			}
		}
	}

	if braceCount != 0 {
		return fmt.Errorf("unmatched opening brace in template")
	}

	return nil
}
