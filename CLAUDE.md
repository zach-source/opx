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
- HTTP server over Unix domain socket 
- JWT-like token authentication via `X-OpAuthd-Token` header
- Request routing and middleware (auth, JSON handling)
- Integrates cache and backend layers via dependency injection

**Client Layer (`internal/client/`)**
- HTTP client with Unix socket transport
- Auto-daemon startup capability (controlled by `OPX_AUTOSTART` env var)
- Clean API wrapper around HTTP endpoints

**Backend Abstraction (`internal/backend/`)**
- Interface: `Backend` with `ReadRef(ctx, ref) (string, error)` and `Name() string`
- `OpCLI`: Production backend that shells out to `op read` command
- `Fake`: Test/dev backend that returns deterministic dummy values
- Switch backends via `OP_AUTHD_BACKEND` or `--backend` flag

**Caching Layer (`internal/cache/`)**
- In-memory TTL cache with configurable expiration
- Thread-safe with RWMutex
- Hit/miss/inflight statistics tracking
- Best-effort string zeroization for security

**Protocol Layer (`internal/protocol/`)**
- JSON request/response structs for all API endpoints
- Clean separation between wire format and internal logic

### Key Architectural Patterns

1. **Single-flight coalescing**: Uses `golang.org/x/sync/singleflight` to prevent duplicate concurrent requests for the same secret reference

2. **Unix socket security**: Socket directory permissions (0700) and token-based auth ensure only the user can access the daemon

3. **Backend pluggability**: Interface-based design allows easy swapping between production (`opcli`) and test (`fake`) backends

4. **Cache-aside pattern**: Manual cache management with explicit cache checks and population

## API Endpoints

The daemon exposes these HTTP endpoints over Unix socket:

- `GET /v1/status` - Health check and statistics
- `POST /v1/read` - Read single secret reference  
- `POST /v1/reads` - Batch read multiple secret references
- `POST /v1/resolve` - Resolve environment variable mappings from refs to values

## Environment Variables

- `OP_AUTHD_BACKEND`: Set to `fake` for testing (default: `opcli`)
- `OPX_AUTOSTART`: Set to `0` to disable client auto-starting daemon

## Security Considerations

- Socket path: `~/.op-authd/socket.sock` with 0700 directory permissions
- Authentication token stored in `~/.op-authd/token` with 0600 permissions  
- Values kept in-memory only with best-effort zeroization on eviction
- 20-second timeout on backend calls to prevent hanging

## Testing Strategy

Use the fake backend for testing:
```bash
export OP_AUTHD_BACKEND=fake
./bin/op-authd --backend fake
```

The fake backend returns predictable dummy values for any reference, enabling deterministic testing without requiring 1Password setup.