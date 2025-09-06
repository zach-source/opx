
# opx - 1Password CLI Batching Daemon

A **1Password CLI batching daemon** (`opx-authd`) with a companion client (`opx`).
It coalesces concurrent secret reads across processes, caches results briefly, and provides a secure
local API over a TLS-encrypted Unix domain socket with comprehensive access controls.

> **Status:** Production-ready. Linux/macOS (Unix socket). Windows named-pipe support planned.

## Why?
Toolchains that shell out to `op read ...` many times end up spamming auth prompts and duplicate API calls.
This daemon centralizes those reads, **coalesces identical in-flight requests**, and **short-caches** results.

## Features
- Unix domain socket server with TLS encryption (XDG Base Directory compliant)
- Bearer token with secure permissions (0600) and directory perms 0700
- **Session idle timeout** with automatic locking after configurable period (default: 8 hours)
- In-memory TTL cache (default 120s) with single-flight coalescing and security clearing
- Backends:
  - `opcli` (default): shells out to `op read <ref>` and relies on 1Password's built-in auth/daemon
  - `fake`: returns deterministic dummy values for testing (`export OP_AUTHD_BACKEND=fake`)
- Endpoints:
  - `POST /v1/read` – read a single ref
  - `POST /v1/reads` – batch read multiple refs
  - `POST /v1/resolve` – resolve env var mapping `{ENV: ref}`
  - `GET  /v1/status` – health/counters and session information
  - `POST /v1/session/unlock` – manually unlock locked sessions

## Install

### From GitHub Releases (Recommended)

Download pre-built binaries for your platform:

```bash
# Download latest release
gh release download -R zach-source/opx

# Or download specific version
gh release download v1.0.0 -R zach-source/opx

# Make binaries executable
chmod +x opx-authd-* opx-*

# Rename for your platform (example for Linux x86_64)
mv opx-authd-linux-amd64 opx-authd
mv opx-linux-amd64 opx
```

### Verify Downloads

```bash
# Verify checksums (recommended)
sha256sum -c checksums.txt

# Verify GPG signature (if available)
gpg --verify checksums.txt.sig checksums.txt
```

### From Source

```bash
git clone https://github.com/zach-source/opx.git
cd opx
make build
# Binaries in ./bin: opx-authd, opx
```

## Run Daemon
```bash
# Default configuration (8-hour session timeout)
./bin/opx-authd --ttl 120 --verbose

# Custom session timeout (4 hours)
./bin/opx-authd --ttl 120 --session-timeout 4 --verbose

# Enable audit logging for security compliance
./bin/opx-authd --ttl 120 --enable-audit-log --verbose

# Disable session management 
./bin/opx-authd --ttl 120 --enable-session-lock=false --verbose

# All security options enabled
./bin/opx-authd \
  --ttl 120 \
  --session-timeout 8 \
  --enable-session-lock=true \
  --lock-on-auth-failure=true \
  --enable-audit-log \
  --verbose
```

### Security Options
- `--session-timeout=8` - Idle timeout in hours (0 to disable, default: 8)
- `--enable-session-lock=true` - Enable session idle timeout and locking 
- `--lock-on-auth-failure=true` - Lock session on authentication failures
- `--enable-audit-log` - Enable structured audit logging to file

## Environment Variables

### Application Configuration
- `OP_AUTHD_BACKEND=fake` - Set backend for testing (default: `opcli`)
- `OPX_AUTOSTART=0` - Disable client auto-starting daemon
- `OP_AUTHD_SESSION_TIMEOUT=8h` - Session timeout (duration format)
- `OP_AUTHD_ENABLE_SESSION_LOCK=true` - Enable session management

### XDG Base Directory Specification  
- `XDG_CONFIG_HOME` - Config directory base (default: `~/.config`)
- `XDG_DATA_HOME` - Data directory base (default: `~/.local/share`) 
- `XDG_RUNTIME_DIR` - Runtime directory base (system-specific)

## Client Usage
```bash
# Single read
./bin/opx read "op://Engineering/DB/password"

# Batch read (multiple args)
./bin/opx read op://Vault/A/secret1 op://Vault/B/secret2

# Resolve env vars then run a command locally
./bin/opx run --env DB_PASS=op://Engineering/DB/password -- -- bash -lc 'echo "db pass: $DB_PASS"'

# Check daemon status
./bin/opx status

# View recent access denials
./bin/opx audit --since=1h

# Interactive policy management
./bin/opx audit --interactive
```

The client will attempt to autostart the daemon if it can't connect. You can disable this via `OPX_AUTOSTART=0`.

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
- **Enterprise-ready**: Comprehensive security with audit logging and access controls

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

### Audit Log Location

- **XDG**: `$XDG_DATA_HOME/op-authd/audit.log` (fallback: `~/.local/share/op-authd/audit.log`)
- **Legacy**: `~/.op-authd/audit.log` (if legacy directory exists)

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
ExecStart=%h/opx/bin/opx-authd --ttl 120 --enable-audit-log --verbose
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
