# API Contract: Template Processing

## Reference Parsing Contract

### Input Format
```
op://vault/item/field?template={{.Value | base64}}
vault://secret/data/myapp?template={{.Value | default "defaultval"}}
```

### Parsing Behavior
- Base reference: Everything before `?template=`
- Template string: Everything after `?template=`
- URL decode template parameter value
- Validate base reference follows existing format rules

### Error Cases
- Missing `?template=` → Process as normal reference (no template)
- Empty template string → Error: "Empty template not allowed"
- Invalid URL encoding → Error: "Invalid template encoding"

## Template Processing Contract

### Function: ProcessTemplate
```go
func ProcessTemplate(ctx context.Context, templateStr, value string) (string, error)
```

**Input**:
- `templateStr`: Template string with Go template syntax
- `value`: Secret value from backend
- `ctx`: Context with 5-second timeout

**Output**:
- `string`: Transformed value
- `error`: Parsing, execution, or timeout error

**Behavior**:
- Parse template with safe function map
- Create context with `.Value` field
- Execute template with timeout
- Return transformed result

### Function: ValidateTemplate
```go
func ValidateTemplate(templateStr string) error
```

**Input**:
- `templateStr`: Template string to validate

**Output**:
- `error`: Validation error or nil if valid

**Behavior**:
- Check template syntax
- Verify all functions are in allowlist
- Check for recursive calls
- Validate template size (<1KB)

## Integration Points

### Backend Integration
Templates are processed AFTER backend retrieval:
```go
// Existing flow
value, err := backend.ReadRef(ctx, baseRef)
if err != nil {
    return "", err
}

// New template processing
if hasTemplate {
    value, err = processor.ProcessTemplate(ctx, templateStr, value)
    if err != nil {
        return "", fmt.Errorf("template processing failed: %w", err)
    }
}
```

### Command Integration

#### Read Command
- Parse reference for template parameter
- Process template after secret retrieval
- Return transformed value

#### Reads Command (Batch)
- Parse each reference independently
- Process templates for each value
- Maintain original order in results

#### Resolve Command
- Parse reference in environment mapping
- Apply template to resolved value
- Format as `KEY=transformed_value`

## Error Response Format

### Template Syntax Error
```json
{
  "error": "template_parse_error",
  "message": "Invalid template syntax: unexpected token",
  "details": {
    "template": "{{.Value | invalidfunc}}",
    "position": 10
  }
}
```

### Function Not Allowed Error
```json
{
  "error": "template_function_not_allowed",
  "message": "Template function not allowed: env",
  "details": {
    "function": "env",
    "allowed_functions": ["base64encode", "default", "trim", "..."]
  }
}
```

### Execution Timeout Error
```json
{
  "error": "template_timeout",
  "message": "Template execution exceeded 5 second timeout",
  "details": {
    "timeout": "5s",
    "template": "{{.Value | complex_operation}}"
  }
}
```