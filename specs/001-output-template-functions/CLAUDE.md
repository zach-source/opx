# Claude Code Instructions: Template Functions Implementation

## Implementation Guidelines

### Code Structure
Follow existing opx patterns when implementing template functionality:
- Use `internal/template/` package for template processing logic
- Follow existing error handling patterns from `internal/backend/`
- Use structured logging with Zap sugar logger
- Implement comprehensive tests following existing test patterns

### Security Requirements
CRITICAL: Template function security is paramount since templates process secrets.

**Function Allowlisting**:
```go
// Use explicit allowlist - DO NOT use Sprig's default function map
allowedFunctions := template.FuncMap{
    // String functions
    "trim":       sprig.TrimSpace,
    "title":      sprig.Title,
    "upper":      sprig.Upper,
    "lower":      sprig.Lower,

    // Encoding functions
    "base64encode": sprig.Base64Encode,
    "base64decode": sprig.Base64Decode,
    "urlquery":     sprig.URLQuery,

    // Logic functions
    "default":      sprig.Default,
    "empty":        sprig.Empty,
    "coalesce":     sprig.Coalesce,

    // Math functions
    "add":          sprig.Add,
    "sub":          sprig.Sub,
    // ... etc
}
```

**Security Validations**:
- Template size limit: 1KB
- Execution timeout: 5 seconds using context.WithTimeout
- No recursive template processing
- Input sanitization for template strings

### Integration Points

**Reference Parsing**:
```go
// Extend existing reference parsing in internal/util/ or internal/protocol/
func ParseReferenceWithTemplate(ref string) (baseRef, template string, error) {
    u, err := url.Parse(ref)
    if err != nil {
        return "", "", err
    }

    template := u.Query().Get("template")
    if template == "" {
        return ref, "", nil // No template
    }

    // Remove template query parameter for base reference
    q := u.Query()
    q.Del("template")
    u.RawQuery = q.Encode()

    return u.String(), template, nil
}
```

**Server Integration**:
```go
// In internal/server/server.go readOneWithFlags
value, err := s.Backend.ReadRefWithFlags(ctx, baseRef, flags)
if err != nil {
    return "", err
}

// Apply template if present
if templateStr != "" {
    value, err = s.TemplateProcessor.ProcessTemplate(ctx, templateStr, value)
    if err != nil {
        return "", fmt.Errorf("template processing failed: %w", err)
    }
}
```

### Testing Strategy

**Unit Tests**:
- Test each allowed function individually
- Test function restrictions (blocked functions should error)
- Test timeout behavior
- Test template compilation caching
- Test reference parsing with query parameters

**Integration Tests**:
- Test with fake backend (no 1Password dependency)
- Test all three commands: read, reads, resolve
- Test error propagation through the stack
- Test with malformed templates

**Security Tests**:
- Attempt to use blocked functions (env, readFile, etc.)
- Test injection attempts in template strings
- Test resource exhaustion (infinite loops, large output)
- Test timeout enforcement

### Performance Considerations

**Template Caching**:
```go
type TemplateCache struct {
    cache map[string]*template.Template
    mu    sync.RWMutex
    maxSize int
}

func (tc *TemplateCache) GetOrCompile(templateStr string, funcMap template.FuncMap) (*template.Template, error) {
    tc.mu.RLock()
    if tmpl, exists := tc.cache[templateStr]; exists {
        tc.mu.RUnlock()
        return tmpl, nil
    }
    tc.mu.RUnlock()

    // Compile template with write lock
    tc.mu.Lock()
    defer tc.mu.Unlock()

    // Double-check after acquiring write lock
    if tmpl, exists := tc.cache[templateStr]; exists {
        return tmpl, nil
    }

    tmpl, err := template.New("").Funcs(funcMap).Parse(templateStr)
    if err != nil {
        return nil, err
    }

    tc.cache[templateStr] = tmpl
    return tmpl, nil
}
```

### Error Handling Patterns

Follow existing opx error handling:
- Use structured errors with context
- Log detailed errors internally, return generic errors to users
- Preserve error chains with `fmt.Errorf("...: %w", err)`
- Use Zap logger for error logging

### Code Location Guidelines

**New Files**:
- `internal/template/processor.go` - Main template processing
- `internal/template/safe_functions.go` - Function allowlist
- `internal/template/parser.go` - Reference parsing with templates
- `internal/template/cache.go` - Template compilation cache
- `internal/template/processor_test.go` - Comprehensive tests

**Modified Files**:
- `internal/server/server.go` - Add template processing to read pipeline
- `internal/protocol/protocol.go` - Add template support to structs if needed
- `cmd/opx/main.go` - Update commands to handle template errors

### Dependencies
Add to go.mod:
```go
require (
    github.com/Masterminds/sprig/v3 v3.2.3
    // existing dependencies...
)
```