
# op-authd-proto

A prototype **1Password op CLI batching daemon** (`op-authd`) with a companion client (`opx`).
It coalesces concurrent secret reads across processes, caches results briefly, and provides a simple
local API over a Unix domain socket.

> **Status:** Prototype. Linux/macOS (Unix socket). Windows named-pipe support would be a follow-up.

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
```bash
make build
# Binaries in ./bin: op-authd, opx
```

## Run Daemon
```bash
# Default configuration (8-hour session timeout)
./bin/op-authd --ttl 120 --verbose

# Custom session timeout (4 hours)
./bin/op-authd --ttl 120 --session-timeout 4 --verbose

# Disable session management 
./bin/op-authd --ttl 120 --enable-session-lock=false --verbose

# All session options
./bin/op-authd \
  --ttl 120 \
  --session-timeout 8 \
  --enable-session-lock=true \
  --lock-on-auth-failure=true \
  --verbose
```

### Session Management Options
- `--session-timeout=8` - Idle timeout in hours (0 to disable, default: 8)
- `--enable-session-lock=true` - Enable session idle timeout and locking 
- `--lock-on-auth-failure=true` - Lock session on authentication failures

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
```

The client will attempt to autostart the daemon if it can't connect. You can disable this via `OPX_AUTOSTART=0`.

## Security Notes
- **TLS encryption** over Unix domain socket protects all client-server communication
- **XDG Base Directory compliant**: Respects `XDG_CONFIG_HOME`, `XDG_DATA_HOME`, `XDG_RUNTIME_DIR` 
- **Backward compatibility**: Existing `~/.op-authd/` installations continue to work
- The socket directory is `0700`, token is `0600`. Only your user should be able to talk to the daemon.
- **Session idle timeout** automatically locks sessions after configurable period (default: 8 hours)
- **Automatic cache clearing** when sessions lock for security
- Values are kept in-memory only and zeroized on replacement/eviction to the extent Go allows
- **Command injection protection** with comprehensive input validation
- **Race condition protection** with atomic file operations
- This is a prototype: **do not** expose the socket to other users or mount it across trust boundaries.

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

## Systemd (user) example
```ini
# ~/.config/systemd/user/op-authd.service
[Unit]
Description=op-authd prototype
After=default.target

[Service]
ExecStart=%h/op-authd-proto/bin/op-authd --ttl 120 --verbose
Restart=on-failure

[Install]
WantedBy=default.target
```
```bash
systemctl --user daemon-reload
systemctl --user enable --now op-authd
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

## Caveats
- Assumes the `op` CLI is installed and signed-in (for `opcli` backend).
- Windows not yet implemented (named pipes TBD).

## License
MIT
