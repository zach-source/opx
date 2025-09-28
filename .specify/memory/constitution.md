<!--
Sync Impact Report:
- Version change: INITIAL → 1.0.0
- New constitution created from template
- Added principles: Security-First, Zero-Trust Architecture, Defense in Depth,
  Session Management, Testing Discipline, Backend Abstraction, Clean Architecture
- Templates status:
  ✅ plan-template.md - reviewed, constitution principles align
  ✅ spec-template.md - reviewed, security requirements compatible
  ✅ tasks-template.md - reviewed, testing discipline reflected
  ✅ agent-file-template.md - reviewed, no conflicts
- Follow-up: None - all placeholders resolved
-->

# opx Project Constitution

**Project**: opx - Multi-Backend Secret Batching Daemon
**Purpose**: Secure, performant, production-grade secret management daemon with multi-backend support

## Core Principles

### I. Security-First Architecture

Security is not negotiable. Every design decision MUST prioritize security over convenience.

**Non-Negotiable Rules:**
- All network communication MUST use TLS encryption (even Unix sockets)
- File permissions MUST be restrictive (0600 for tokens, 0700 for directories)
- Secrets MUST be stored in-memory only with best-effort zeroization
- Input validation MUST prevent command injection and path traversal
- Authentication MUST be required for all secret access operations
- Audit logging MUST track all access decisions when enabled

**Rationale**: The daemon handles production secrets. A single security vulnerability could compromise entire systems. Defense-in-depth provides multiple security layers even if one fails.

### II. Zero-Trust Architecture

Never trust input. Always validate. Assume compromise is possible.

**Non-Negotiable Rules:**
- ALL user input MUST be validated before processing
- Reference formats MUST be explicitly validated (cannot start with `-`)
- Flags MUST be checked for command injection patterns (`;&|` backticks, etc.)
- Peer credentials MUST be validated for Unix socket connections
- Process paths MUST be verified against expected values
- Policy checks MUST occur before secret access

**Rationale**: External processes can be spoofed, inputs can be crafted maliciously, and privilege escalation is a real threat. Zero-trust ensures security holds even when assumptions break.

### III. Defense in Depth (6-Layer Security Model)

Security through multiple independent layers. Failure of one layer must not compromise the system.

**Required Security Layers:**
1. **Transport Layer**: TLS encryption over Unix domain sockets
2. **Authentication Layer**: Bearer token validation (X-OpAuthd-Token header)
3. **Authorization Layer**: Peer credential validation (PID, UID, executable path)
4. **Policy Layer**: JSON-configurable access control rules
5. **Session Layer**: Idle timeout with automatic locking
6. **Audit Layer**: Comprehensive logging of access decisions

**Rationale**: Multiple security layers ensure that compromise of one mechanism (e.g., token theft) doesn't grant full access. Each layer provides independent protection.

### IV. Session Management & Cache Security

Sessions and caches MUST be tied together. When a session locks, caches MUST clear.

**Non-Negotiable Rules:**
- Session state MUST be tracked per-account for multi-account support
- Idle timeout MUST be configurable (default: 8 hours)
- Cache MUST be cleared automatically on session lock/expiry
- Session validation MUST NOT cause deadlocks (use internal locked methods)
- Lazy authentication MUST leverage 1Password app integration

**Rationale**: Cached secrets are as sensitive as live secrets. A locked session with cached data is a security vulnerability. Multi-account isolation prevents cross-account data leakage.

### V. Testing Discipline

Production code MUST be proven correct through comprehensive automated testing.

**Non-Negotiable Rules:**
- All business logic MUST have unit tests
- Critical paths (argument parsing, flag extraction) MUST have 100% coverage
- Integration tests MUST validate end-to-end workflows
- Tests MUST use fake backends to avoid external dependencies
- Tests MUST NOT be disabled - failures MUST be fixed
- Race detection MUST be enabled (`go test -race`)

**Coverage Requirements:**
- Core algorithms: 100% (parseArgs, extractAccountFromFlags)
- Business logic: 80%+ (backends, session management)
- HTTP handlers: 50%+ (integration tests supplement unit tests)

**Rationale**: Untested code is broken code. High coverage on critical paths prevents regressions. Fake backends enable fast, reliable testing without external services.

### VI. Backend Abstraction & Pluggability

Backends MUST be pluggable through clean interfaces. No backend-specific logic in server layer.

**Non-Negotiable Rules:**
- All backends MUST implement `Backend` interface
- Backends MUST support `ReadRef(ctx, ref)` and `ReadRefWithFlags(ctx, ref, flags)`
- Backend selection MUST be runtime configurable (`--backend` flag)
- Fake backend MUST be available for testing
- Session awareness MUST be composable via wrapper pattern

**Rationale**: Clean abstractions enable testing with fake backends, support multiple secret sources, and allow future backend additions without server changes.

### VII. Clean Architecture & Separation of Concerns

Layer boundaries MUST be clear and respected. Dependencies flow inward.

**Layer Structure:**
```
cmd/         → Entry points (main functions)
internal/
  server/    → HTTP routing, middleware, orchestration
  client/    → HTTP client, daemon management
  backend/   → Secret fetching abstraction
  cache/     → In-memory TTL cache
  session/   → Session state management
  security/  → Peer validation, process inspection
  policy/    → Access control rules
  audit/     → Structured logging
  util/      → File paths, TLS, helpers
```

**Non-Negotiable Rules:**
- Server MUST NOT know about backend implementation details
- Backends MUST NOT import server package
- Shared types MUST live in `internal/protocol`
- No circular dependencies
- Dependency injection over singletons

**Rationale**: Clean architecture enables independent testing, parallel development, and confident refactoring. Clear boundaries prevent tight coupling.

### VIII. Concurrency Safety

All concurrent access MUST be safe. Data races are bugs.

**Non-Negotiable Rules:**
- Shared state MUST be protected by mutexes
- Internal locked methods MUST NOT call public locking methods (prevents deadlock)
- Public API methods acquire locks, delegate to `*Locked()` internals
- Singleflight MUST be used for deduplication
- All packages MUST pass `go test -race`

**Example Pattern:**
```go
func (m *Manager) PublicMethod() {
    m.mu.Lock()
    defer m.mu.Unlock()
    return m.publicMethodLocked()  // Internal, assumes lock held
}
```

**Rationale**: The v0.4.0 deadlock (ValidateAccountSession calling GetAccountSession) demonstrated the critical importance of careful lock management. Recursive locking causes production hangs.

## Security Requirements

### Command Injection Prevention

ALL external command execution MUST be protected against injection attacks.

**Required Protections:**
- `cmd.Stdin = nil` to prevent interactive prompts
- Reference validation: MUST start with `op://`, `vault://`, or `bao://`
- Reference validation: MUST NOT start with `-` (flag injection)
- Flag validation: MUST start with `-`
- Flag validation: MUST NOT contain `;&|` backticks `$()` (shell metacharacters)
- Timeouts: External commands MUST have context deadlines
- Process killing: `cmd.Cancel` MUST be set to kill on timeout

### Memory Security

Secrets in memory MUST be handled with care.

**Requirements:**
- Secrets stored in `SafeString` or plain strings (in-memory only)
- Best-effort zeroization on cache eviction
- No secrets in logs (use `[REDACTED]` placeholders)
- No secrets in error messages returned to clients
- No secrets written to disk (temporary files, swap, etc.)

### File System Security

All persistent data MUST follow secure file permissions.

**Requirements:**
- Token files: 0600 permissions
- Config files: 0600 permissions
- Socket directories: 0700 permissions
- Audit logs: 0600 permissions
- XDG compliance with backward compatibility for `~/.op-authd/`

## Development Workflow

### Conventional Commits

All commits MUST follow Conventional Commits specification for automatic versioning.

**Commit Types:**
- `feat:` - New features (minor version bump)
- `fix:` - Bug fixes (patch version bump)
- `docs:` - Documentation only
- `test:` - Test additions/changes
- `refactor:` - Code refactoring without behavior change
- `security:` - Security improvements
- `perf:` - Performance improvements

**Breaking Changes:** Add `!` after type or `BREAKING CHANGE:` in footer (major version bump)

### Release Process

Releases MUST be automated and reproducible.

**Requirements:**
- Version determined automatically via `svu` (semantic version utility)
- All tests MUST pass before release
- Binaries MUST be code-signed when signing credentials available
- Release MUST update all distribution channels:
  - GitHub release with binaries and checksums
  - Homebrew tap formula
  - Nix flake package
- Release notes MUST reference commit history

**Release Command:**
```bash
./scripts/release-full.sh
```

### Testing Gates

Code MUST NOT be merged or released unless all tests pass.

**Required:**
- `go test ./...` - All unit tests pass
- `go test -race ./...` - No data races detected
- `go vet ./...` - Static analysis passes (except documented unsafe.Pointer)
- `go fmt ./...` - Code properly formatted
- Integration test available (manual/optional due to 1Password dependency)

## Governance

This constitution defines the foundational principles for the opx project. All code, architecture decisions, and processes MUST align with these principles.

### Amendment Process

1. Propose amendment via pull request to `.specify/memory/constitution.md`
2. Document rationale in PR description
3. Update `CONSTITUTION_VERSION` following semantic versioning
4. Update `LAST_AMENDED_DATE` to amendment date
5. Verify all dependent templates updated (plan, spec, tasks)
6. Require approval from project maintainer
7. Update runtime guidance (CLAUDE.md, README.md) if applicable

### Compliance Verification

All pull requests and code reviews MUST verify:
- Security principles followed (input validation, permissions, etc.)
- Testing discipline met (coverage requirements, race detection)
- Architecture boundaries respected (no layer violations)
- Concurrency safety maintained (proper lock usage)

### Version Semantics

- **MAJOR** (X.0.0): Principle removed or fundamentally redefined
- **MINOR** (x.Y.0): New principle added or existing principle materially expanded
- **PATCH** (x.y.Z): Clarifications, typo fixes, non-semantic refinements

**Version**: 1.0.0 | **Ratified**: 2025-09-28 | **Last Amended**: 2025-09-28