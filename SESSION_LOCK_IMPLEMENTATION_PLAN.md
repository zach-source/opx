# Session Lock Implementation Plan

## Overview

Implement configurable idle timeout functionality for opx to automatically lock the 1Password CLI session after periods of inactivity (default: 8 hours). This adds a security layer by requiring re-authentication when the daemon has been idle.

## Current State Analysis

### Authentication Flow
- **Token-based auth**: Uses `X-OpAuthd-Token` header for daemon authentication
- **No session management**: Currently no tracking of 1Password CLI session state
- **Direct CLI calls**: Backend shells out to `op read` commands without session validation
- **Cache-only timeouts**: Only cache entries expire, not the underlying session

### Architecture Components
- **Server**: HTTP server over Unix socket with token auth
- **Backend**: OpCLI backend shells out to `op` commands
- **Cache**: In-memory TTL cache for secrets
- **Client**: HTTP client with auto-daemon startup

## Implementation Architecture

### 1. Session State Management

**New Components:**
- `internal/session/manager.go` - Session state tracking and timeout management
- `internal/session/state.go` - Session state types and validation
- `internal/session/config.go` - Configuration for idle timeout settings

**Session State:**
```go
type SessionState int

const (
    SessionUnknown SessionState = iota
    SessionAuthenticated 
    SessionLocked
    SessionExpired
)

type SessionManager struct {
    mu           sync.RWMutex
    state        SessionState
    lastActivity time.Time
    idleTimeout  time.Duration
    lockCallback func() error
}
```

### 2. Idle Timeout Detection

**Tracking Strategy:**
- Track `lastActivity` timestamp on every successful secret read
- Run background goroutine to check for idle timeout periodically
- Lock session when `time.Since(lastActivity) > idleTimeout`

**Configuration:**
- Default idle timeout: 8 hours
- Configurable via CLI flag `--session-idle-timeout=8h`
- Environment variable `OPX_SESSION_IDLE_TIMEOUT`
- Configuration file support in `~/.op-authd/config.json`

### 3. Lock/Unlock Mechanism

**Lock Process:**
1. Detect idle timeout exceeded
2. Set session state to `SessionLocked`
3. Clear in-memory cache (security measure)
4. Log session lock event
5. Return authentication error for subsequent requests

**Unlock Process:**
1. Validate 1Password CLI session with `op whoami`
2. If valid, set state to `SessionAuthenticated` 
3. If invalid, require manual `op signin` and retry
4. Update `lastActivity` timestamp
5. Log session unlock event

### 4. Backend Integration

**Enhanced OpCLI Backend:**
```go
type OpCLI struct {
    sessionManager *session.Manager
}

func (o *OpCLI) ReadRefWithFlags(ctx context.Context, ref string, flags []string) (string, error) {
    // Check session state before executing
    if err := o.sessionManager.ValidateSession(ctx); err != nil {
        return "", fmt.Errorf("session validation failed: %w", err)
    }
    
    // Execute op command
    result, err := o.executeOpCommand(ctx, ref, flags)
    if err != nil {
        // Check if error indicates authentication failure
        if isAuthError(err) {
            o.sessionManager.MarkLocked()
            return "", fmt.Errorf("session locked due to authentication failure: %w", err)
        }
        return "", err
    }
    
    // Update activity timestamp on successful read
    o.sessionManager.UpdateActivity()
    return result, nil
}
```

### 5. Configuration System

**Configuration Structure:**
```go
type Config struct {
    SessionIdleTimeout time.Duration `json:"session_idle_timeout"`
    EnableSessionLock  bool          `json:"enable_session_lock"`
    LockOnAuthFailure  bool          `json:"lock_on_auth_failure"`
}
```

**Configuration Sources (priority order):**
1. CLI flags
2. Environment variables  
3. Configuration file `~/.op-authd/config.json`
4. Defaults

### 6. API Enhancements

**Status Endpoint Enhancement:**
```go
type StatusResponse struct {
    // ... existing fields
    SessionState     string    `json:"session_state"`
    LastActivity     time.Time `json:"last_activity,omitempty"`
    IdleTimeout      string    `json:"idle_timeout"`
    TimeUntilLock    string    `json:"time_until_lock,omitempty"`
}
```

**New Endpoint:**
- `POST /v1/session/unlock` - Manual session unlock endpoint

## Implementation Stages

### Stage 1: Core Session Management
- [ ] Create `internal/session` package with manager, state, and config
- [ ] Implement session state tracking with idle timeout detection
- [ ] Add background goroutine for periodic timeout checks
- [ ] Unit tests for session manager functionality

### Stage 2: Backend Integration  
- [ ] Enhance OpCLI backend with session validation
- [ ] Implement session unlock mechanism with `op whoami` validation
- [ ] Add activity timestamp updates on successful operations
- [ ] Integration tests for backend session validation

### Stage 3: Configuration System
- [ ] Add CLI flags for session timeout configuration
- [ ] Implement environment variable support
- [ ] Add configuration file parsing (`~/.op-authd/config.json`)
- [ ] Configuration validation and default handling

### Stage 4: Server Integration
- [ ] Integrate session manager into server struct
- [ ] Enhance status endpoint with session information
- [ ] Add manual session unlock endpoint
- [ ] Update authentication middleware with session validation

### Stage 5: Security & Error Handling
- [ ] Clear cache on session lock for security
- [ ] Add comprehensive error handling for auth failures
- [ ] Implement proper logging for session events
- [ ] Add metrics for session lock/unlock events

### Stage 6: Testing & Documentation
- [ ] Comprehensive test suite covering all scenarios
- [ ] End-to-end tests with real `op` CLI integration
- [ ] Update documentation with session lock feature
- [ ] Performance testing with session validation overhead

## Security Considerations

### Threat Model
- **Idle workstation**: Prevent access to secrets when user is away
- **Process hijacking**: Clear cached secrets when session locks
- **Authentication bypass**: Validate session state before secret access
- **Configuration tampering**: Secure configuration file permissions

### Security Measures
1. **Cache clearing**: Clear in-memory cache when session locks
2. **Activity tracking**: Only update on successful secret reads (not failed attempts)
3. **Validation frequency**: Balance security vs performance for session checks
4. **Secure defaults**: 8-hour timeout provides reasonable security/usability balance
5. **Audit logging**: Log all session state transitions for security monitoring

## Migration Strategy

### Backward Compatibility
- Feature disabled by default initially
- Graceful degradation when session management unavailable
- Existing API contracts preserved
- Optional configuration maintains current behavior

### Rollout Plan
1. **Phase 1**: Internal testing with feature flag
2. **Phase 2**: Opt-in beta with conservative defaults
3. **Phase 3**: Enabled by default with 8-hour timeout
4. **Phase 4**: Full production deployment

## Testing Strategy

### Unit Tests
- Session state transitions
- Idle timeout detection
- Configuration parsing and validation
- Backend integration points

### Integration Tests  
- End-to-end session lock/unlock workflows
- Real `op` CLI authentication testing
- Configuration file handling
- Error scenarios and recovery

### Security Tests
- Session lock bypasses
- Cache clearing verification
- Authentication failure handling
- Configuration security

## Success Metrics

### Functionality
- [ ] Session automatically locks after configured idle period
- [ ] Manual unlock works with valid 1Password session
- [ ] Cache clears on session lock
- [ ] No performance regression for active usage

### Security
- [ ] No cached secrets accessible when session locked
- [ ] Authentication failures trigger session lock
- [ ] Audit trail for all session events
- [ ] Secure configuration handling

### Usability
- [ ] Transparent operation during active usage
- [ ] Clear error messages for locked sessions
- [ ] Simple configuration for different timeout needs
- [ ] Minimal impact on existing workflows

---

**Status**: Planning Complete - Ready for Implementation
**Priority**: High Security Enhancement
**Estimated Effort**: 3-5 days (6 stages)
**Dependencies**: None (uses existing architecture)