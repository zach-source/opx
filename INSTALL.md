# Installation and Setup Guide

This guide provides step-by-step instructions for installing, configuring, and running `opx`, a 1Password CLI batching daemon that coalesces concurrent secret reads across processes.

## Prerequisites

### System Requirements
- **Operating System**: macOS, Linux, or WSL on Windows
- **Go**: Version 1.20 or later
- **1Password CLI**: Required for production use (optional for development/testing)
- **Unix Domain Sockets**: Supported by the operating system

### Required Tools
```bash
# Verify Go installation
go version  # Should show 1.20+

# Check if 1Password CLI is installed (for production)
op --version  # Optional for development
```

## Installation

### Method 1: Build from Source (Recommended)

1. **Clone the repository**
```bash
git clone https://github.com/zach-source/opx.git
cd opx
```

2. **Build the binaries**
```bash
# Build both daemon and client
make build

# Or build individually
go build -o bin/op-authd ./cmd/op-authd
go build -o bin/opx ./cmd/opx
```

3. **Verify installation**
```bash
./bin/op-authd --help
./bin/opx --help
```

### Method 2: Direct Go Install
```bash
# Install directly from source
go install github.com/zach-source/opx/cmd/op-authd@latest
go install github.com/zach-source/opx/cmd/opx@latest
```

## Configuration

### Environment Variables

| Variable | Description | Default | Required |
|----------|-------------|---------|----------|
| `OP_AUTHD_BACKEND` | Backend type (`opcli` or `fake`) | `opcli` | No |
| `OPX_AUTOSTART` | Auto-start daemon if not running | `1` | No |
| `HOME` | User home directory for state storage | System default | Yes |

### Backend Configuration

#### Production Backend (`opcli`)
Uses the 1Password CLI for actual secret retrieval:

```bash
# Set to use production backend (default)
export OP_AUTHD_BACKEND=opcli

# Ensure 1Password CLI is configured
op signin  # Follow 1Password setup instructions
```

#### Development/Testing Backend (`fake`)
Uses deterministic fake values for testing:

```bash
# Set to use fake backend for testing
export OP_AUTHD_BACKEND=fake
```

### File System Layout

The daemon creates the following directory structure:

```
~/.op-authd/
├── socket.sock    # Unix domain socket (created at runtime)
└── token          # Authentication token (auto-generated)
```

**Permissions:**
- Directory: `0700` (owner read/write/execute only)
- Token file: `0600` (owner read/write only)
- Socket: `0700` (owner read/write/execute only)

## Usage

### Starting the Daemon

#### Method 1: Direct Start
```bash
# Start with verbose logging
./bin/op-authd --verbose

# Start in background
./bin/op-authd &

# Start with specific backend
./bin/op-authd --backend=fake --verbose
```

#### Method 2: Auto-start via Client
The client can automatically start the daemon if it's not running:

```bash
# Client will auto-start daemon if needed
./bin/opx read "op://vault/item/password"
```

To disable auto-start:
```bash
export OPX_AUTOSTART=0
./bin/opx read "op://vault/item/password"  # Will fail if daemon not running
```

### Using the Client

#### Single Secret Read
```bash
# Read a single secret
./bin/opx read "op://vault/MyItem/password"

# Read with specific 1Password account
./bin/opx --account=YOPUYSOQIRHYVGIV3IQ5CS627Y read "op://Private/ClaudeCodeLongLiveCreds/credential"

# Account flag also works with separated syntax
./bin/opx --account YOPUYSOQIRHYVGIV3IQ5CS627Y read "op://vault/item/password"
```

#### Batch Secret Reads
```bash
# Read multiple secrets at once
./bin/opx reads "op://vault/item1/password" "op://vault/item2/apikey"

# Read multiple secrets with account flag
./bin/opx --account=YOPUYSOQIRHYVGIV3IQ5CS627Y reads "op://Private/item1/password" "op://Private/item2/apikey"
```

#### Environment Variable Resolution
```bash
# Resolve environment variables
./bin/opx resolve DB_PASSWORD=op://vault/database/password API_KEY=op://vault/service/apikey

# Resolve with account flag
./bin/opx --account=YOPUYSOQIRHYVGIV3IQ5CS627Y resolve DB_PASSWORD=op://Private/database/password

# Use in run command with account
./bin/opx --account=YOPUYSOQIRHYVGIV3IQ5CS627Y run --env DB_PASSWORD=op://Private/db/password -- psql
```

### Daemon Management

#### Check Status
```bash
# View daemon status and statistics
curl -H "X-OpAuthd-Token: $(cat ~/.op-authd/token)" \
     --unix-socket ~/.op-authd/socket.sock \
     http://localhost/v1/status
```

#### Stop Daemon
```bash
# Find and stop the daemon process
pkill op-authd

# Or use process management
ps aux | grep op-authd
kill <pid>
```

## Development Setup

### Running Tests
```bash
# Run all tests
make test
# Or: go test ./...

# Run tests with coverage
go test -cover ./...

# Run specific package tests
go test ./internal/cache -v
go test ./internal/backend -v

# Run with fake backend
export OP_AUTHD_BACKEND=fake
go test ./...
```

### Development Workflow
```bash
# Clean build artifacts
make clean

# Format code
go fmt ./...

# Run static analysis
go vet ./...

# Build and test in one command
make build && go test ./...
```

### Integration Testing
```bash
# Enable 1Password CLI integration tests (requires op setup)
export ENABLE_OP_INTEGRATION_TESTS=1
go test ./internal/backend -v

# Test with fake backend (no 1Password required)
export OP_AUTHD_BACKEND=fake
./bin/op-authd --verbose &
./bin/opx read "op://vault/test/password"
```

## Troubleshooting

### Common Issues

#### 1. Daemon Won't Start
**Symptoms**: Connection refused errors, socket not found

**Solutions**:
```bash
# Check if daemon is already running
ps aux | grep op-authd

# Check socket permissions
ls -la ~/.op-authd/

# Try starting with verbose logging
./bin/op-authd --verbose

# Check for port conflicts (shouldn't happen with Unix sockets)
lsof ~/.op-authd/socket.sock
```

#### 2. Permission Denied Errors
**Symptoms**: "Permission denied" when accessing socket or token

**Solutions**:
```bash
# Check directory permissions
ls -la ~/.op-authd/
# Should show: drwx------ (700)

# Fix permissions if needed
chmod 700 ~/.op-authd/
chmod 600 ~/.op-authd/token

# Recreate state directory
rm -rf ~/.op-authd/
./bin/op-authd  # Will recreate with correct permissions
```

#### 3. 1Password CLI Issues
**Symptoms**: "op read failed" errors in production mode

**Solutions**:
```bash
# Verify 1Password CLI is working
op --version
op vault list

# Re-authenticate if needed
op signin

# Test with fake backend temporarily
export OP_AUTHD_BACKEND=fake
./bin/opx read "test-ref"
```

#### 4. Client Connection Issues
**Symptoms**: "connection refused", "no such file or directory"

**Solutions**:
```bash
# Check if daemon is running
ps aux | grep op-authd

# Verify socket exists
ls -la ~/.op-authd/socket.sock

# Check token file
cat ~/.op-authd/token

# Enable auto-start
export OPX_AUTOSTART=1
./bin/opx read "test"
```

### Debug Mode

Enable verbose logging for troubleshooting:

```bash
# Start daemon with verbose output
./bin/op-authd --verbose

# Check logs for:
# - Socket creation
# - Token generation
# - Client connections
# - Backend calls
# - Cache operations
```

### Log Analysis

The daemon logs important events:
```
op-authd listening on unix:///Users/user/.op-authd/socket.sock backend=opcli ttl=5m0s
cache cleanup: removed 3 expired entries
```

Look for:
- Socket path and backend confirmation
- Cache statistics and cleanup events
- Error messages with context

## Security Considerations

### Token Security
- Authentication tokens are randomly generated (256-bit)
- Stored with restrictive permissions (600)
- Unique per daemon instance
- Not transmitted over network (Unix socket only)

### Socket Security
- Unix domain sockets provide local-only access
- Directory permissions prevent other users from accessing
- No network exposure by design

### Memory Security
- Best-effort string zeroization after use
- Limited cache TTL to reduce exposure time
- No persistence of secrets to disk

### Process Security
```bash
# Run daemon as non-root user
./bin/op-authd  # Never run as root

# Monitor process permissions
ps -eo user,pid,cmd | grep op-authd

# Check file permissions regularly
ls -la ~/.op-authd/
```

## Performance Tuning

### Cache Configuration
The cache TTL is currently fixed but can be modified in code:
```go
// In cmd/op-authd/main.go, modify:
cache := cache.New(5 * time.Minute)  // Adjust TTL as needed
```

### Concurrent Access
The daemon handles concurrent requests efficiently:
- Single-flight coalescing prevents duplicate requests
- Thread-safe cache operations
- Connection pooling for backend calls

### Resource Monitoring
```bash
# Monitor memory usage
ps -o pid,rss,vsz,comm -p $(pgrep op-authd)

# Monitor file descriptors
lsof -p $(pgrep op-authd)

# Monitor cache statistics via status endpoint
curl -s -H "X-OpAuthd-Token: $(cat ~/.op-authd/token)" \
     --unix-socket ~/.op-authd/socket.sock \
     http://localhost/v1/status | jq .
```

## Production Deployment

### Systemd Service (Linux)
Create `/etc/systemd/system/op-authd.service`:
```ini
[Unit]
Description=1Password Auth Daemon
After=network.target

[Service]
Type=simple
User=%i
ExecStart=/usr/local/bin/op-authd
Restart=always
RestartSec=5
Environment=OP_AUTHD_BACKEND=opcli

[Install]
WantedBy=multi-user.target
```

### Process Management
```bash
# Start service
sudo systemctl start op-authd@username

# Enable auto-start
sudo systemctl enable op-authd@username

# Check status
sudo systemctl status op-authd@username
```

### Monitoring
- Monitor daemon process health
- Check socket accessibility
- Verify cache hit rates via status endpoint
- Monitor 1Password CLI authentication status

## API Reference

### HTTP Endpoints

All requests require the `X-OpAuthd-Token` header with the token from `~/.op-authd/token`.

#### GET /v1/status
Returns daemon status and statistics.

#### POST /v1/read
Read a single secret reference.

#### POST /v1/reads  
Read multiple secret references in batch.

#### POST /v1/resolve
Resolve environment variable mappings.

See the protocol package documentation for detailed request/response formats.

---

For additional help or issues, please refer to the project documentation or submit an issue to the repository.