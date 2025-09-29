# Research: Template Function Implementation

## Technical Decisions

### Template Engine Choice: Go text/template + Sprig
**Decision**: Use Go's built-in `text/template` package with Masterminds/sprig v3 functions
**Rationale**:
- Native Go integration with existing codebase
- Sprig provides battle-tested template functions
- Security: Can selectively enable/disable function categories
- Performance: Compiled templates with caching support

**Alternatives Considered**:
- Custom template language: Too much development overhead
- Full Sprig: Security risk (OS/network functions)
- JSON transformation: Limited functionality

### Reference Syntax: Query Parameter
**Decision**: `op://vault/item/field?template={{.Value | base64}}`
**Rationale**:
- Standard URL query parameter syntax (familiar to users)
- Easy to parse with Go's `url.Parse()`
- Clear separation between reference and template
- Doesn't interfere with existing `op://` parsing

### Security: Function Allowlisting
**Decision**: Explicit allowlist of safe Sprig functions
**Safe Categories**:
- String functions: `trim`, `title`, `upper`, `lower`, `replace`
- Encoding functions: `base64encode`, `base64decode`, `urlquery`
- Math functions: `add`, `sub`, `mul`, `div`
- Logic functions: `default`, `empty`, `coalesce`
- Date functions: `now`, `date`

**Excluded Categories**:
- OS functions: `env`, `expandenv`
- File functions: `readFile`, `writeFile`
- Network functions: `getHostByName`
- Crypto functions: `genPrivateKey`, `genSelfSignedCert`

### Template Context Structure
```go
type TemplateContext struct {
    Value string    // The secret value from backend
    Ref   string    // The original reference (for debugging)
    Meta  struct {  // Optional metadata
        Backend string
        Cached  bool
    }
}
```

### Performance Strategy
- **Template Compilation Caching**: Cache compiled templates by template string
- **Timeout**: 5-second context deadline for template execution
- **Memory Limits**: Templates process individual values (not batch)

## Architecture Integration

### Processing Pipeline
```
1. Client sends reference with template query parameter
2. Server parses reference and extracts template
3. Backend retrieves secret value
4. Template processor applies template to value
5. Transformed result returned to client
```

### Module Structure
```
internal/
  template/           # New package
    processor.go      # Main template processing logic
    safe_functions.go # Sprig function allowlist
    parser.go         # Reference parsing with template extraction
    cache.go          # Template compilation cache
    processor_test.go # Comprehensive tests
```

### Error Handling Strategy
- Template syntax errors: Return clear user-facing error
- Template execution errors: Log details, return generic error
- Timeout errors: "Template execution timeout"
- Security violations: "Template function not allowed"

## Implementation Phases

### Phase 1: Core Template Processing
1. Create `internal/template` package
2. Implement safe Sprig function filtering
3. Template compilation and caching
4. Basic template execution with timeout

### Phase 2: Reference Integration
1. Extend reference parsing to handle query parameters
2. Integrate template processor into read pipeline
3. Update protocol types for template support

### Phase 3: Command Integration
1. Update `read` command to support templates
2. Update `reads` batch command
3. Update `resolve` command for env var templates

### Phase 4: Testing & Polish
1. Comprehensive unit tests
2. Integration tests with real templates
3. Security tests for function restrictions
4. Performance benchmarks