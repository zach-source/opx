# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

`opx` is a 1Password CLI batching daemon that coalesces concurrent secret reads across processes, caches results briefly, and provides a local API over Unix domain socket. It consists of:

- `op-authd`: The daemon server that handles secret fetching and caching
- `opx`: The client CLI that communicates with the daemon

**📋 Current Status**: See [CURRENT_STATUS.md](./CURRENT_STATUS.md) for recent security review results, action items, and development priorities.

## Build and Development Commands

```bash
# Build both binaries
make build

# Clean build artifacts
make clean

# Run the daemon (verbose by default)
make run

# Test the entire project
go test ./...

# Test a specific package
go test ./internal/cache

# Build individual binaries manually
go build -o bin/op-authd ./cmd/op-authd
go build -o bin/opx ./cmd/opx

# Format code
go fmt ./...

# Run go vet for static analysis
go vet ./...
```

## Architecture Overview

The project follows a clean layered architecture with clear separation of concerns:

### Core Components

**Server Layer (`internal/server/`)**
- HTTP server over Unix domain socket with TLS encryption
- JWT-like token authentication via `X-OpAuthd-Token` header
- Request routing and middleware (auth, JSON handling)
- Session management integration with lifecycle management
- Integrates cache, backend, and session layers via dependency injection

**Client Layer (`internal/client/`)**
- HTTP client with Unix socket transport
- Auto-daemon startup capability (controlled by `OPX_AUTOSTART` env var)
- Clean API wrapper around HTTP endpoints

**Backend Abstraction (`internal/backend/`)**
- Interface: `Backend` with `ReadRef(ctx, ref) (string, error)` and `Name() string`
- `OpCLI`: Production backend that shells out to `op read` command
- `Fake`: Test/dev backend that returns deterministic dummy values
- `SessionAwareBackend`: Wrapper that adds session validation to any backend
- Switch backends via `OP_AUTHD_BACKEND` or `--backend` flag

**Caching Layer (`internal/cache/`)**
- In-memory TTL cache with configurable expiration
- Thread-safe with RWMutex
- Hit/miss/inflight statistics tracking
- Best-effort string zeroization for security
- Automatic cache clearing on session lock

**Session Management (`internal/session/`)**
- Thread-safe session state management (Unknown, Authenticated, Locked, Expired)
- Configurable idle timeout with background monitoring (default: 8 hours)
- Callback architecture for lock/unlock mechanisms
- Environment variable and config file support
- Zero external dependencies (pure Go standard library)

**Protocol Layer (`internal/protocol/`)**
- JSON request/response structs for all API endpoints
- Clean separation between wire format and internal logic
- Session status and unlock request/response structs

### Key Architectural Patterns

1. **Single-flight coalescing**: Uses `golang.org/x/sync/singleflight` to prevent duplicate concurrent requests for the same secret reference

2. **Unix socket security**: Socket directory permissions (0700) and token-based auth ensure only the user can access the daemon

3. **Backend pluggability**: Interface-based design allows easy swapping between production (`opcli`) and test (`fake`) backends

4. **Cache-aside pattern**: Manual cache management with explicit cache checks and population

## API Endpoints

The daemon exposes these HTTP endpoints over TLS-encrypted Unix socket:

- `GET /v1/status` - Health check, statistics, and session information
- `POST /v1/read` - Read single secret reference  
- `POST /v1/reads` - Batch read multiple secret references
- `POST /v1/resolve` - Resolve environment variable mappings from refs to values
- `POST /v1/session/unlock` - Manually unlock locked sessions

## Configuration

### Command-Line Flags

- `--ttl=120` - Cache TTL in seconds
- `--backend=opcli` - Backend type (`opcli` or `fake`)
- `--verbose` - Enable verbose logging
- `--session-timeout=8` - Session idle timeout in hours (0 to disable)
- `--enable-session-lock=true` - Enable session management
- `--lock-on-auth-failure=true` - Lock session on authentication failures

### Environment Variables

#### Application Configuration
- `OP_AUTHD_BACKEND`: Set to `fake` for testing (default: `opcli`)
- `OPX_AUTOSTART`: Set to `0` to disable client auto-starting daemon
- `OP_AUTHD_SESSION_TIMEOUT`: Session timeout in duration format (e.g., `8h`)
- `OP_AUTHD_ENABLE_SESSION_LOCK`: Enable session management (`true`/`false`)

#### XDG Base Directory Specification
- `XDG_CONFIG_HOME`: Config directory base (default: `~/.config`)
- `XDG_DATA_HOME`: Data directory base (default: `~/.local/share`)
- `XDG_RUNTIME_DIR`: Runtime directory base (varies by system)

## Security Considerations

- **TLS Encryption**: All communication over Unix socket is TLS-encrypted with self-signed certificates
- **XDG Compliance**: Follows XDG Base Directory specification for config/data separation
- **Socket Security**: Socket path with 0700 directory permissions (XDG runtime dir or legacy ~/.op-authd/)
- **Authentication**: Token stored with 0600 permissions (XDG data dir or legacy ~/.op-authd/)  
- **Session Management**: Configurable idle timeout (default: 8 hours) with automatic locking
- **Cache Security**: Automatic cache clearing when sessions lock
- **Input Validation**: Command injection protection and reference format validation
- **Memory Security**: Values kept in-memory only with best-effort zeroization on eviction
- **Timeout Protection**: 20-second timeout on backend calls to prevent hanging
- **Race Condition Protection**: Atomic file operations for token management

## File System Layout

The application follows XDG Base Directory specification with backward compatibility:

### XDG-Compliant Paths (New Installations)
- **Config**: `$XDG_CONFIG_HOME/op-authd/config.json` (fallback: `~/.config/op-authd/config.json`)
- **Data**: `$XDG_DATA_HOME/op-authd/` (fallback: `~/.local/share/op-authd/`)
  - `token` - Authentication token
  - `cert.pem`, `key.pem` - TLS certificates
- **Runtime**: `$XDG_RUNTIME_DIR/op-authd/socket.sock` (fallback: same as data directory)

### Legacy Paths (Existing Installations)
- **All files**: `~/.op-authd/` (used when directory already exists)
  - `config.json`, `token`, `cert.pem`, `key.pem`, `socket.sock`

## Testing Strategy

Use the fake backend for testing:
```bash
export OP_AUTHD_BACKEND=fake
./bin/op-authd --backend fake
```

The fake backend returns predictable dummy values for any reference, enabling deterministic testing without requiring 1Password setup.