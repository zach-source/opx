# Data Model: Template Functions

## Core Entities

### TemplateProcessor
**Purpose**: Processes templates with safe Sprig functions
**Attributes**:
- `allowedFunctions map[string]interface{}` - Allowlisted Sprig functions
- `cache map[string]*template.Template` - Compiled template cache
- `timeout time.Duration` - Execution timeout (5s)

**Methods**:
- `ProcessTemplate(ctx, templateStr, value string) (string, error)`
- `ParseReference(ref string) (baseRef, templateStr string, error)`
- `ValidateTemplate(templateStr string) error`

### TemplateContext
**Purpose**: Data structure passed to template execution
**Attributes**:
- `Value string` - Secret value from backend
- `Ref string` - Original reference for debugging
- `Meta TemplateMetadata` - Optional metadata

### TemplateMetadata
**Purpose**: Additional context for template processing
**Attributes**:
- `Backend string` - Backend name that provided the value
- `Cached bool` - Whether value came from cache
- `Timestamp time.Time` - When value was retrieved

### SafeFunctionRegistry
**Purpose**: Registry of allowed Sprig functions
**Attributes**:
- `stringFunctions map[string]interface{}` - String manipulation functions
- `encodingFunctions map[string]interface{}` - Encoding/decoding functions
- `mathFunctions map[string]interface{}` - Mathematical functions
- `logicFunctions map[string]interface{}` - Logic and conditional functions

## Data Flow

### Template Processing Pipeline
```
1. Reference Input: "op://vault/item/field?template={{.Value | base64}}"
2. Parse Reference: Extract base reference and template string
3. Validate Template: Check syntax and function usage
4. Retrieve Secret: Use existing backend to get value
5. Create Context: Build TemplateContext with value and metadata
6. Execute Template: Apply template with timeout
7. Return Result: Transformed value or error
```

### Template Cache Structure
```
Key: Template string (SHA256 hash)
Value: Compiled *template.Template
TTL: 1 hour (configurable)
Size Limit: 100 entries (LRU eviction)
```

## Security Model

### Function Categories (Allowed)
- **Strings**: `trim`, `title`, `upper`, `lower`, `replace`, `split`, `join`
- **Encoding**: `base64encode`, `base64decode`, `urlquery`, `htmlescape`
- **Math**: `add`, `sub`, `mul`, `div`, `mod`, `max`, `min`
- **Logic**: `default`, `empty`, `coalesce`, `ternary`
- **Date**: `now`, `date`, `dateInZone`

### Function Categories (Blocked)
- **OS**: `env`, `expandenv`, `exec`
- **File**: `readFile`, `writeFile`, `glob`
- **Network**: `getHostByName`, `httpGet`
- **Crypto**: `genPrivateKey`, `genCert`, `bcrypt`

### Validation Rules
1. Template syntax must be valid Go text/template
2. All functions must be in allowlisted set
3. No recursive template calls
4. Maximum template size: 1KB
5. Maximum execution time: 5 seconds

## Error Types

### TemplateParseError
**When**: Invalid template syntax
**Message**: "Invalid template syntax: {detail}"
**Action**: Return to user with syntax help

### TemplateFunctionError
**When**: Disallowed function used
**Message**: "Template function not allowed: {function}"
**Action**: Return to user with allowed function list

### TemplateExecutionError
**When**: Runtime template error
**Message**: "Template execution failed"
**Action**: Log details, return generic error (don't expose internals)

### TemplateTimeoutError
**When**: Template execution exceeds 5s
**Message**: "Template execution timeout"
**Action**: Kill template goroutine, return error