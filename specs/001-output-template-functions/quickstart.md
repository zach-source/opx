# Quickstart: Template Functions Testing

## Test Scenarios

### Scenario 1: Basic Base64 Encoding
```bash
# Store test secret
op item create --category=password --title="template-test" --vault=Private password="mysecret123"

# Test base64 template
opx read "op://Private/template-test/password?template={{.Value | base64encode}}"
# Expected: bXlzZWNyZXQxMjM=

# Verify decoding
echo "bXlzZWNyZXQxMjM=" | base64 -d
# Expected: mysecret123
```

### Scenario 2: Default Value Handling
```bash
# Test with non-existent secret
opx read "op://Private/nonexistent/field?template={{.Value | default \"fallback-value\"}}"
# Expected: fallback-value

# Test with empty secret
op item create --category=password --title="empty-test" --vault=Private
opx read "op://Private/empty-test/password?template={{.Value | default \"empty-fallback\"}}"
# Expected: empty-fallback
```

### Scenario 3: String Manipulation
```bash
# Test string functions
opx read "op://Private/template-test/password?template={{.Value | upper | trim}}"
# Expected: MYSECRET123

# Test replace function
opx read "op://Private/template-test/password?template={{.Value | replace \"secret\" \"hidden\"}}"
# Expected: myhidden123
```

### Scenario 4: Batch Operations
```bash
# Create multiple test items
op item create --category=password --title="test1" --vault=Private password="value1"
op item create --category=password --title="test2" --vault=Private password="value2"

# Test batch with templates
opx reads \
  "op://Private/test1/password?template={{.Value | base64encode}}" \
  "op://Private/test2/password?template={{.Value | upper}}"
# Expected:
# dmFsdWUx
# VALUE2
```

### Scenario 5: Environment Variable Resolution
```bash
# Test resolve with templates
opx resolve \
  DB_PASSWORD="op://Private/template-test/password?template={{.Value | base64encode}}" \
  API_KEY="op://Private/test1/password?template={{.Value | upper}}"
# Expected:
# DB_PASSWORD=bXlzZWNyZXQxMjM=
# API_KEY=VALUE1
```

### Scenario 6: Error Handling
```bash
# Test invalid template syntax
opx read "op://Private/template-test/password?template={{.Value | }}"
# Expected: Error: Invalid template syntax

# Test disallowed function
opx read "op://Private/template-test/password?template={{.Value | env}}"
# Expected: Error: Template function not allowed: env

# Test timeout (create complex template)
opx read "op://Private/template-test/password?template={{range 1000000}}{{.Value}}{{end}}"
# Expected: Error: Template execution timeout
```

## Development Test Commands

### Quick Validation
```bash
# Build and start daemon
make build
./bin/opx-authd --backend=fake --verbose &

# Test with fake backend (no 1Password needed)
./bin/opx read "op://test/item/field?template={{.Value | base64encode}}"
# Expected: fake value encoded in base64

# Test invalid template
./bin/opx read "op://test/item/field?template={{.Value | invalid}}"
# Expected: Clear error message

# Cleanup
pkill opx-authd
```

### Performance Testing
```bash
# Test template compilation caching
time ./bin/opx read "op://test/item/field?template={{.Value | base64encode}}"  # First call
time ./bin/opx read "op://test/item/field?template={{.Value | base64encode}}"  # Cached

# Test complex templates
./bin/opx read "op://test/item/field?template={{.Value | base64encode | upper | trim}}"

# Test batch performance
./bin/opx reads $(for i in {1..10}; do echo "op://test/item$i/field?template={{.Value | base64encode}}"; done)
```

### Security Testing
```bash
# Test function restrictions
./bin/opx read "op://test/item/field?template={{env \"HOME\"}}"  # Should fail
./bin/opx read "op://test/item/field?template={{readFile \"/etc/passwd\"}}"  # Should fail

# Test safe functions
./bin/opx read "op://test/item/field?template={{.Value | base64encode}}"  # Should work
./bin/opx read "op://test/item/field?template={{.Value | default \"safe\"}}"  # Should work
```

## Integration Test Setup

### Prerequisites
```bash
# Ensure 1Password CLI authenticated
./bin/opx login 1password --account=YOUR_ACCOUNT

# Or use fake backend for isolated testing
export OP_AUTHD_BACKEND=fake
```

### Test Data Setup
```bash
# Create test vault items (only needed for real 1Password testing)
op item create --category=password --title="template-test-1" --vault=Private password="test123"
op item create --category=password --title="template-test-2" --vault=Private password=""
op item create --category=password --title="template-test-3" --vault=Private password="special!@#$%"
```

### Cleanup
```bash
# Remove test items
op item delete "template-test-1" "template-test-2" "template-test-3" --vault=Private
```