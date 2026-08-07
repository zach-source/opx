
# opx - Multi-Backend Secret Batching Daemon

A **multi-backend secret batching daemon** (`opx-authd`) with a companion client (`opx`).
It coalesces concurrent secret reads across processes from multiple sources (1Password, HashiCorp Vault, OpenBao), 
caches results briefly, and provides a secure local API over a TLS-encrypted Unix domain socket with comprehensive access controls.

> **Status:** Production-ready. Linux/macOS (Unix socket). Windows named-pipe support planned.

## Why?
Toolchains that shell out to secret management CLIs (`op read`, `vault kv get`, etc.) many times end up spamming auth prompts and duplicate API calls.
This daemon centralizes those reads from multiple sources, **coalesces identical in-flight requests**, and **short-caches** results.

## Features
- Unix domain socket server with TLS encryption (XDG Base Directory compliant)
- Bearer token with secure permissions (0600) and directory perms 0700
- **Session idle timeout** with automatic locking after configurable period (default: 8 hours)
- In-memory TTL cache (default 4h) with single-flight coalescing and security clearing
- **Multi-backend support**:
  - `opcli`: 1Password CLI integration with `op://` references
  - `vault`: HashiCorp Vault with `vault://` references  
  - `bao`: OpenBao with `bao://` references
  - `multi`: Route requests to appropriate backend based on URI scheme
  - `fake`: Deterministic dummy values for testing
- Endpoints:
  - `POST /v1/read` – read a single ref
  - `POST /v1/reads` – batch read multiple refs
  - `POST /v1/resolve` – resolve env var mapping `{ENV: ref}`
  - `GET  /v1/status` – health/counters and session information
  - `POST /v1/session/unlock` – manually unlock locked sessions

## Install

### Homebrew (Recommended for macOS)

```bash
# Add the tap
brew tap zach-source/homebrew-tap

# Install opx
brew install opx

# Start as a service (automatically starts on boot)
brew services start opx

# Or run manually
opx-authd --enable-audit-log --verbose
```

### Nix (Declarative Installation)

```bash
# Install directly
nix profile install github:zach-source/nix-packages#opx

# Or add to flake.nix
{
  inputs.zach-utils.url = "github:zach-source/nix-packages";
  # Then use: zach-utils.packages.${system}.opx
}
```

**Home Manager Integration (Recommended):**
```nix
{
  inputs = {
    nixpkgs.url = "github:NixOS/nixpkgs/nixpkgs-unstable";
    home-manager.url = "github:nix-community/home-manager";
    zach-utils.url = "github:zach-source/nix-packages";
  };

  outputs = { nixpkgs, home-manager, zach-utils, ... }: {
    homeConfigurations."your-username" = home-manager.lib.homeManagerConfiguration {
      modules = [
        zach-utils.homeManagerModules.opx
        {
          services.opx-authd = {
            enable = true;
            backend = "multi";
            enableAuditLog = true;
            sessionTimeout = 8;
            auditLogRetentionDays = 90;
            
            # Optional: Define access policies declaratively
            policy = {
              allow = [
                {
                  path = "/usr/bin/kubectl";
                  refs = [ "op://Production/k8s/*" ];
                  require_signed = true;
                }
              ];
              default_deny = true;
            };
          };
        }
      ];
    };
  };
}
```

**Service Configuration Options:**
- `backend`: opcli, vault, bao, multi, fake
- `sessionTimeout`: Hours before session lock (default: 8)
- `enableAuditLog`: Enable structured audit logging
- `auditLogRetentionDays`: Days to keep audit logs (default: 30)
- `policy`: Access control policy (JSON format)
- `environmentFile`: Path to file with VAULT_TOKEN, etc.

### From Source

```bash
git clone https://github.com/zach-source/opx.git
cd opx
make build
# Binaries in ./bin: opx-authd, opx
```

## Usage

### Quick Start (Any Installation Method)

```bash
# Login to 1Password
opx login 1password --account=YOUR_ACCOUNT

# Start daemon (if not using service)
opx-authd --backend=multi --enable-audit-log --verbose

# Read secrets from any backend
opx read "op://vault/item/field"              # 1Password
opx read "vault://secret/myapp/config#pass"   # Vault
opx read "bao://kv/prod/api#key"             # Bao
```

### Advanced Authentication

```bash
# Self-authentication using credential references
opx login vault --token-ref="op://vault/vault-token/value"
opx login vault --username-ref="op://vault/creds/user" --password-ref="op://vault/creds/pass"

# Multi-backend workflows
opx-authd --backend=multi --enable-audit-log --session-timeout=8 --verbose
```

### Security and Audit Options
- `--session-timeout=8` - Idle timeout in hours (0 to disable, default: 8)
- `--enable-session-lock=true` - Enable session idle timeout and locking 
- `--lock-on-auth-failure=true` - Lock session on authentication failures
- `--enable-audit-log` - Enable structured audit logging to file
- `--audit-log-retention-days=30` - Number of days to keep audit logs (0 = keep all)

## Environment Variables

### Application Configuration
- `OP_AUTHD_BACKEND=fake` - Set backend for testing (default: `opcli`)
- `OPX_AUTOSTART=0` - Disable client auto-starting daemon
- `OPX_AUTHD_PATH=/path/to/opx-authd` - Custom path to daemon binary
- `OPX_SOCKET_PATH=/tmp/custom.sock` - Custom socket path (both client and daemon)
- `OP_AUTHD_SESSION_TIMEOUT=8h` - Session timeout (duration format)
- `OP_AUTHD_ENABLE_SESSION_LOCK=true` - Enable session management

### XDG Base Directory Specification  
- `XDG_CONFIG_HOME` - Config directory base (default: `~/.config`)
- `XDG_DATA_HOME` - Data directory base (default: `~/.local/share`) 
- `XDG_RUNTIME_DIR` - Runtime directory base (system-specific)

## Client Usage

### Multi-Account Support
The `--account` flag can be placed before OR after the command:

```bash
# Both work identically:
opx read "op://vault/item/field" --account=ACCOUNT_ID
opx --account=ACCOUNT_ID read "op://vault/item/field"
```

### Basic Operations
```bash
# Read from different backends (uses 1Password app integration)
opx read "op://Engineering/DB/password"           # 1Password
opx read "vault://secret/myapp/config#password"   # HashiCorp Vault
opx read "bao://kv/production/api#key"           # OpenBao

# Multi-account 1Password
opx read "op://Private/SSH/key" --account=PERSONAL_ACCOUNT
opx read "op://Work/API/token" --account=WORK_ACCOUNT

# Batch read from multiple backends
opx read op://Vault/A/secret1 vault://secret/B/secret2

# Resolve env vars then run a command
opx run --env DB_PASS=op://Engineering/DB/password --env API_KEY=vault://secret/api#key -- bash -lc 'echo "db: $DB_PASS"'

# Check daemon status
opx status

# View recent access denials
opx audit --since=1h

# Interactive policy management
opx audit --interactive
```

### Authentication
No explicit login required if 1Password app is unlocked (biometric/touch integration).
Session starts automatically on first successful read and persists for the configured idle timeout (default: 8 hours).

The client will autostart the daemon if it can't connect. Disable via `OPX_AUTOSTART=0`.

## Supported URI Schemes

The daemon supports multiple secret backends with different URI schemes:

### 1Password (`op://`)
```bash
op://vault/item/field          # Standard 1Password reference
op://Private/SSH/private_key   # Private vault SSH key
op://Shared/API/token         # Shared vault API token
```

### HashiCorp Vault (`vault://`)
```bash
vault://secret/data/myapp#password    # KV v2 secret with field
vault://secret/database              # Entire secret as JSON
vault://auth/aws/config#access_key   # Auth backend configuration
```

### OpenBao (`bao://`)
```bash
bao://kv/data/production#api_key     # KV secret with field  
bao://database/config               # Database configuration
bao://pki/ca_chain                 # PKI certificate chain
```

**Note**: Vault and Bao backends require proper authentication and configuration. The daemon supports token-based and userpass authentication.

## Self-Authentication Chain

Use opx to provide credentials to itself for automated workflows:

### Vault Authentication Using 1Password

```bash
# Store Vault credentials in 1Password first
# op://vault/vault-creds/username -> "myuser"  
# op://vault/vault-creds/password -> "mypass"

# Use opx to authenticate to Vault using 1Password credentials
opx login vault --username-ref="op://vault/vault-creds/username" --password-ref="op://vault/vault-creds/password"

# Credentials are automatically stored for daemon usage
# Start Vault-enabled daemon
./bin/opx-authd --backend=vault --verbose
```

### Token-Based Self-Authentication

```bash
# Store Vault token in 1Password
# op://vault/vault-token/value -> "hvs.your-token-here"

# Use opx to set up Vault authentication
opx login vault --method=token --token-ref="op://vault/vault-token/value"

# Start daemon with stored credentials
./bin/opx-authd --backend=vault --verbose
```

### Cross-Backend Workflows

```bash
# 1. Login to 1Password
opx login 1password --account=YOUR_ACCOUNT

# 2. Use 1Password to authenticate to Vault
opx login vault --token-ref="op://vault/vault-token/value"

# 3. Start multi-backend daemon
./bin/opx-authd --backend=multi --verbose

# 4. Access secrets from all backends
opx read "op://vault/item/field"        # 1Password
opx read "vault://secret/app#key"       # Vault (authenticated via 1Password)
```

## Security Notes
- **TLS encryption** over Unix domain socket protects all client-server communication
- **Peer credential validation** extracts calling process information for access control
- **Policy-based access control** restricts secret access by process path/PID and reference patterns
- **XDG Base Directory compliant**: Respects `XDG_CONFIG_HOME`, `XDG_DATA_HOME`, `XDG_RUNTIME_DIR` 
- **Backward compatibility**: Existing `~/.op-authd/` installations continue to work
- The socket directory is `0700`, token is `0600`. Only your user should be able to talk to the daemon.
- **Session idle timeout** automatically locks sessions after configurable period (default: 8 hours)
- **Automatic cache clearing** when sessions lock for security
- Values are kept in-memory only and zeroized on replacement/eviction to the extent Go allows
- **Command injection protection** with comprehensive input validation
- **Race condition protection** with atomic file operations
- **Production-ready**: Comprehensive security with audit logging and access controls

## File Locations

The tool follows XDG Base Directory specification with backward compatibility:

### Data Files (tokens, certificates)
- **XDG**: `$XDG_DATA_HOME/op-authd/` (fallback: `~/.local/share/op-authd/`)
- **Legacy**: `~/.op-authd/` (used if directory already exists)

### Config Files
- **XDG**: `$XDG_CONFIG_HOME/op-authd/config.json` (fallback: `~/.config/op-authd/config.json`)  
- **Legacy**: `~/.op-authd/config.json` (used if `~/.op-authd/` directory exists)

### Runtime Files (socket)
- **XDG**: `$XDG_RUNTIME_DIR/op-authd/socket.sock` (fallback: same as data dir)
- **Legacy**: `~/.op-authd/socket.sock` (used if directory already exists)

## Access Control Policy

The daemon supports optional policy-based access control to restrict which processes can access which secrets.

### Policy Configuration

Create a policy file at `$XDG_CONFIG_HOME/op-authd/policy.json` (or `~/.config/op-authd/policy.json`):

```json
{
  "allow": [
    {
      "path": "/usr/local/bin/deployment-tool",
      "refs": ["op://Production/*"]
    },
    {
      "path": "/usr/bin/approved-app", 
      "refs": ["op://Development/*", "op://Testing/*"]
    }
  ],
  "default_deny": true
}
```

### Policy Rules

- **`path`**: Absolute path to executable (must match exactly)
- **`path_sha256`**: SHA256 hash of executable path (alternative to `path`)
- **`pid`**: Exact process ID (useful for temporary access)
- **`refs`**: Array of allowed reference patterns
  - `"*"` - Allow all references
  - `"op://vault/*"` - Allow all references in vault
  - `"op://vault/item/field"` - Allow exact reference

### Default Behavior

- **No policy file**: All processes allowed (current behavior)
- **Empty policy**: All processes allowed unless `default_deny: true`
- **Policy exists**: Only explicitly allowed processes can access matching references

## Audit Logging

Enable comprehensive security audit logging with `--enable-audit-log`:

```bash
./bin/opx-authd --enable-audit-log --verbose
```

### Audit Log Features

- **Structured JSON logging**: Each event recorded as structured JSON in `audit.log`
- **Access decisions**: Every policy decision logged with process and reference details
- **Authentication events**: Token validation attempts and outcomes
- **Session events**: Session lock/unlock operations
- **Process tracking**: Complete process information (PID, path, UID/GID where available)

### Audit Log Location and Rotation

**Daily Log Files:**
- **XDG**: `$XDG_DATA_HOME/op-authd/audit-YYYY-MM-DD.log` (fallback: `~/.local/share/op-authd/audit-YYYY-MM-DD.log`)
- **Legacy**: `~/.op-authd/audit-YYYY-MM-DD.log` (if legacy directory exists)

**Rotation Features:**
- **Daily rotation**: New log file created each day at midnight
- **Configurable retention**: Default 30 days, configurable via `--audit-log-retention-days`
- **Automatic cleanup**: Old logs automatically removed based on retention policy
- **Historical analysis**: `opx audit` scans across all available daily log files

### Example Audit Events

```json
{"timestamp":"2025-09-05T15:30:45Z","event":"ACCESS_DECISION","peer_info":{"PID":12345,"Path":"/usr/bin/kubectl"},"reference":"op://Production/k8s/token","decision":"ALLOW","policy_path":"~/.config/op-authd/policy.json"}
{"timestamp":"2025-09-05T15:31:02Z","event":"ACCESS_DECISION","peer_info":{"PID":12346,"Path":"/tmp/malicious"},"reference":"op://Production/admin/key","decision":"DENY","policy_path":"~/.config/op-authd/policy.json"}
```

## Audit Log Management

The `opx audit` command helps you analyze access denials and create policy rules:

### View Recent Denials

```bash
# Show denials from last 24 hours (default)
./opx audit

# Show denials from last hour
./opx audit --since=1h

# Show denials from last week
./opx audit --since=168h
```

### Interactive Policy Management

```bash
# Interactive mode for creating allow rules
./opx audit --interactive
```

**Example workflow:**
1. **View denials**: See which processes were denied access to which secrets
2. **Select denials**: Choose which ones should be allowed (comma-separated: `1,3,5`)
3. **Choose scope**: Select permission level (exact reference, vault-wide, or all secrets)
4. **Auto-update**: Policy file automatically updated with new rules

**Interactive Session Example:**
```
Scanning audit log for denials in the last 24h...
Found 2 unique access denials:

[1] Process: /usr/bin/kubectl
    Reference: op://Production/k8s/token
    Denied: 5 times, Last: 2025-09-05 15:31:02

[2] Process: /usr/local/bin/deploy
    Reference: op://Staging/api/key
    Denied: 2 times, Last: 2025-09-05 15:28:15

Select denials to create allow rules for: 1,2

Creating allow rule for: /usr/bin/kubectl -> op://Production/k8s/token
Select permission level:
  [1] op://Production/k8s/token (exact match)
  [2] op://Production/* (entire vault)
  [3] * (all secrets)
Choice: 2

✅ Added rule: /usr/bin/kubectl can access op://Production/*
```

## Systemd (user) example
```ini
# ~/.config/systemd/user/opx-authd.service
[Unit]
Description=opx-authd - 1Password CLI Batching Daemon
After=default.target

[Service]
ExecStart=%h/opx/bin/opx-authd --ttl 14400 --enable-audit-log --verbose
Restart=on-failure
RestartSec=5

[Install]
WantedBy=default.target
```
```bash
systemctl --user daemon-reload
systemctl --user enable --now opx-authd
```

## Implementation sketch
- HTTP over Unix socket with custom `http.Transport` dialing `unix` (client) and `http.Serve` (server)
- `singleflight.Group` to coalesce identical `ref` lookups
- Small TTL cache keyed by `ref`
- Backend interface:
  ```go
  type Backend interface {
    ReadRef(ctx context.Context, ref string) (string, error)
  }
  ```

## Supported Platforms

| Platform | Architecture | Status |
|----------|-------------|---------|
| Linux | x86_64 (amd64) | ✅ Supported |
| Linux | ARM64 | ✅ Supported |  
| macOS | Intel (amd64) | ✅ Supported |
| macOS | Apple Silicon (arm64) | ✅ Supported |
| Windows | x86_64 | ⏳ Planned (named pipes) |

## Requirements

- **1Password CLI** must be installed and authenticated
- **Go 1.22+** (if building from source)
- **Linux** or **macOS** operating system

## Caveats
- Assumes the `op` CLI is installed and signed-in (for `opcli` backend).
- Windows not yet implemented (named pipes TBD).

## License
MIT
