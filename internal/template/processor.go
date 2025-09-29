package template

import (
	"bytes"
	"context"
	"fmt"
	"text/template"
	"time"
)

// Processor handles template execution with safe Sprig functions
type Processor interface {
	ProcessTemplate(ctx context.Context, templateStr, value string) (string, error)
	ValidateTemplate(templateStr string) error
}

// DefaultProcessor implements the Processor interface
type DefaultProcessor struct {
	registry *SafeFunctionRegistry
	cache    *Cache
	timeout  time.Duration
}

// NewProcessor creates a new template processor with safe functions
func NewProcessor() *DefaultProcessor {
	return &DefaultProcessor{
		registry: NewSafeFunctionRegistry(),
		cache:    NewCache(100),
		timeout:  5 * time.Second,
	}
}

// ProcessTemplate executes a template with the given value
func (p *DefaultProcessor) ProcessTemplate(ctx context.Context, templateStr, value string) (string, error) {
	// Validate template string first
	if err := p.ValidateTemplate(templateStr); err != nil {
		return "", err
	}

	// Apply timeout to context
	ctx, cancel := context.WithTimeout(ctx, p.timeout)
	defer cancel()

	// Get or compile template
	tmpl, err := p.cache.GetOrCompile(templateStr, p.registry.GetFunctions())
	if err != nil {
		return "", fmt.Errorf("template compilation failed: %w", err)
	}

	// Create template context
	templateCtx := Context{
		Value: value,
		Meta: Metadata{
			Timestamp: time.Now(),
		},
	}

	// Execute template with timeout protection
	var buf bytes.Buffer
	done := make(chan error, 1)

	go func() {
		done <- tmpl.Execute(&buf, templateCtx)
	}()

	select {
	case err := <-done:
		if err != nil {
			return "", fmt.Errorf("template execution failed: %w", err)
		}
		return buf.String(), nil
	case <-ctx.Done():
		return "", fmt.Errorf("template execution timeout")
	}
}

// ValidateTemplate validates template syntax and function usage
func (p *DefaultProcessor) ValidateTemplate(templateStr string) error {
	// Use the parser validation function
	return ValidateTemplateString(templateStr)
}

// Context represents the data structure passed to template execution
type Context struct {
	Value string   `json:"value"`
	Ref   string   `json:"ref"`
	Meta  Metadata `json:"meta"`
}

// Metadata contains additional context for template processing
type Metadata struct {
	Backend   string    `json:"backend"`
	Cached    bool      `json:"cached"`
	Timestamp time.Time `json:"timestamp"`
}
