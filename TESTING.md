# Isolated Testing Guide

This document describes how to run isolated opx daemon instances for testing different configurations and multi-account scenarios.

## Custom Configuration Override

### Configuration Files

**Session Configuration (test-config.json):**
```json
{
  "session_idle_timeout": 28800000000000,
  "enable_session_lock": true,
  "lock_on_auth_failure": true,
  "check_interval": 60000000000
}
```

**Policy Configuration (test-policy.json):**
```json
{
  "allow": [
    {
      "path": "/nix/store/.../opx/bin/opx",
      "refs": ["op://Private/*", "op://Preview/*", "op://Employee/*"],
      "require_signed": true,
      "description": "Opx binary with multi-vault access"
    }
  ],
  "default_deny": true
}
```

### Daemon Configuration Flags

```bash
# Custom configuration and policy
opx-authd --config ./custom-config.json --policy ./custom-policy.json

# Custom socket path for isolation
opx-authd --sock /tmp/custom-opx.sock

# Combined custom configuration
opx-authd --sock /tmp/test.sock --config ./test-config.json --policy ./test-policy.json --backend=multi --verbose
```

## Multi-Account Testing Patterns

### Pattern 1: Multiple Isolated Daemons

**Work Account Daemon:**
```bash
OP_ACCOUNT=WORK_ACCOUNT_ID opx-authd \
  --sock /tmp/opx-work.sock \
  --policy ./work-policy.json \
  --backend=multi \
  --verbose &
```

**Personal Account Daemon:**
```bash
OP_ACCOUNT=PERSONAL_ACCOUNT_ID opx-authd \
  --sock /tmp/opx-personal.sock \
  --policy ./personal-policy.json \
  --backend=opcli \
  --verbose &
```

### Pattern 2: Client Socket Selection

**Connect to Work Daemon:**
```bash
OPX_SOCKET_PATH=/tmp/opx-work.sock opx read "op://Employee/secret" --account=WORK_ACCOUNT_ID
```

**Connect to Personal Daemon:**
```bash
OPX_SOCKET_PATH=/tmp/opx-personal.sock opx read "op://Private/secret" --account=PERSONAL_ACCOUNT_ID
```

### Pattern 3: Testing Different Backends

**1Password Only:**
```bash
opx-authd --sock /tmp/opx-1p.sock --backend=opcli --policy ./1p-policy.json &
OPX_SOCKET_PATH=/tmp/opx-1p.sock opx read "op://vault/secret"
```

**Multi-Backend:**
```bash
opx-authd --sock /tmp/opx-multi.sock --backend=multi --policy ./multi-policy.json &
OPX_SOCKET_PATH=/tmp/opx-multi.sock opx read "op://vault/secret"      # 1Password
OPX_SOCKET_PATH=/tmp/opx-multi.sock opx read "vault://secret/key"     # Vault
OPX_SOCKET_PATH=/tmp/opx-multi.sock opx read "bao://kv/secret"        # Bao
```

## Policy Testing Workflows

### Permissive Testing Policy

```json
{
  "allow": [
    {
      "path": "/nix/store/.../opx",
      "refs": ["*"],
      "require_signed": false,
      "description": "Allow all access for testing"
    }
  ],
  "default_deny": false
}
```

### Restrictive Testing Policy

```json
{
  "allow": [
    {
      "path": "/nix/store/.../opx",
      "refs": ["op://Development/*"],
      "require_signed": true,
      "required_parents": ["zsh"],
      "description": "Development environment only"
    }
  ],
  "default_deny": true
}
```

## Debugging Commands

### Daemon Inspection

```bash
# Check which daemon is running
opx version

# Check daemon with custom socket
OPX_SOCKET_PATH=/tmp/custom.sock opx version
```

### Policy Debugging

```bash
# Test policy with custom configuration
OPX_SOCKET_PATH=/tmp/test.sock opx policy debug "/path/to/binary" "op://vault/secret"

# Audit failures with isolated daemon
OPX_SOCKET_PATH=/tmp/test.sock opx audit failures --since=1h
```

### Process Management

```bash
# List all opx daemon processes
ps aux | grep opx-authd

# Stop specific daemon by socket
pkill -f "opx-test.sock"

# Clean up test sockets
rm -f /tmp/opx-*.sock
```

## Multi-Account Session Testing

### Verified Working Architecture

The multi-account session system provides:
- **Per-account session tracking**: Individual session state for each 1Password account
- **Account-specific validation**: Session validation tied to specific account IDs
- **Activity tracking**: Last activity timestamps per account
- **Isolated timeouts**: Each account can have different idle timeout behavior

### Testing Multi-Account Sessions

```bash
# Start daemon with multi-account session support
opx-authd --sock /tmp/multi-account.sock --backend=multi --verbose &

# Test different accounts (requires daemon environment per account)
OPX_SOCKET_PATH=/tmp/multi-account.sock opx read "op://vault/secret" --account=ACCOUNT_A
OPX_SOCKET_PATH=/tmp/multi-account.sock opx read "op://vault/secret" --account=ACCOUNT_B
```

**Note**: Full multi-account support requires 1Password CLI session management improvements beyond the current opx architecture.

## Best Practices

1. **Use isolated sockets** for testing to avoid conflicts with production daemons
2. **Custom policies** for testing without affecting global security configuration  
3. **Multiple daemon instances** for testing different account/backend combinations
4. **Environment isolation** to test different 1Password account contexts
5. **Cleanup test resources** (sockets, config files) after testing

This testing infrastructure provides **comprehensive isolation** for configuration experimentation and multi-account development.