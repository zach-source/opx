#!/bin/bash
set -euo pipefail

# Release script using GoReleaser for opx
# Handles versioning, building, signing, and GitHub release creation

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[0;33m'
BLUE='\033[0;34m'
NC='\033[0m'

info() { echo -e "${BLUE}ℹ ${1}${NC}"; }
success() { echo -e "${GREEN}✅ ${1}${NC}"; }
warn() { echo -e "${YELLOW}⚠ ${1}${NC}"; }
error() { echo -e "${RED}❌ ${1}${NC}"; exit 1; }

# Use svu to determine next version based on conventional commits
SVU_BIN=$(go env GOPATH)/bin/svu
if [[ ! -x "$SVU_BIN" ]]; then
    error "svu is required. Install: go install github.com/caarlos0/svu@latest"
fi

VERSION=$($SVU_BIN next)

# Check prerequisites
GORELEASER_BIN=$(go env GOPATH)/bin/goreleaser
if [[ ! -x "$GORELEASER_BIN" ]]; then
    error "GoReleaser is required. Install: go install github.com/goreleaser/goreleaser@latest"
fi

if ! command -v gh &> /dev/null; then
    error "GitHub CLI is required. Install: https://cli.github.com/"
fi

if ! git rev-parse --is-inside-work-tree &> /dev/null; then
    error "Not inside a git repository"
fi

if git tag -l | grep -q "^${VERSION}$"; then
    error "Version tag ${VERSION} already exists"
fi

if ! git diff-index --quiet HEAD --; then
    error "Repository has uncommitted changes. Please commit or stash them."
fi

info "Preparing release ${VERSION}..."

# Create git tag
info "Creating git tag ${VERSION}..."
git tag -a "${VERSION}" -m "Release ${VERSION}

🎉 opx ${VERSION} - Enterprise Production Release

Features:
- Session idle timeout with 8-hour default and full configurability  
- XDG Base Directory specification compliance with backward compatibility
- TLS encryption over Unix domain sockets with self-signed certificates
- Peer credential validation and policy-based access control
- Comprehensive audit logging for security compliance
- Full API with status and session unlock endpoints
- CLI integration with security and session management flags

Security:
- All HIGH priority security items resolved
- Command injection protection with comprehensive validation
- Race condition mitigation using atomic file operations
- Automatic cache clearing on session lock for security
- Input validation and sanitization throughout

Architecture:
- Clean layered design with dependency injection
- Thread-safe implementation with proper concurrency handling
- Zero external dependencies for session management
- Pluggable backend architecture (opcli production + fake testing)
- XDG-compliant file organization with legacy compatibility"

success "Git tag ${VERSION} created"

# Check for signing credentials (using GoReleaser standard env vars)
if [[ -n "${MACOS_SIGN_P12:-}" ]] && [[ -n "${MACOS_SIGN_PASSWORD:-}" ]]; then
    info "Apple Developer credentials detected - macOS binaries will be signed"
    if [[ -n "${MACOS_NOTARY_ISSUER_ID:-}" ]] && [[ -n "${MACOS_NOTARY_KEY_ID:-}" ]] && [[ -n "${MACOS_NOTARY_KEY:-}" ]]; then
        info "Apple notarization credentials detected - binaries will be notarized"
    else
        warn "Notarization credentials missing - binaries will be signed but not notarized"
    fi
else
    warn "Apple Developer credentials not set - macOS binaries will not be signed"
    warn "To enable signing, set these environment variables:"
    warn "  export MACOS_SIGN_P12='base64-encoded-certificate.p12'"
    warn "  export MACOS_SIGN_PASSWORD='certificate-password'"
    warn ""
    warn "For notarization, also set:"
    warn "  export MACOS_NOTARY_ISSUER_ID='your-issuer-uuid'"
    warn "  export MACOS_NOTARY_KEY_ID='your-key-id'"
    warn "  export MACOS_NOTARY_KEY='base64-encoded-api-key.p8'"
fi

SKIP_SIGN=""
if [[ -n "${GPG_FINGERPRINT:-}" ]]; then
    info "GPG fingerprint detected - checksums will be signed"
else
    # The signs: block templates .Env.GPG_FINGERPRINT unconditionally, so without
    # this the whole release aborts after building rather than shipping unsigned.
    warn "GPG_FINGERPRINT not set - checksums will not be signed"
    SKIP_SIGN="--skip=sign"
fi

# GoReleaser needs a forge token. Outside CI there is no GITHUB_TOKEN in the
# environment, so fall back to the one the GitHub CLI is already logged in with.
if [[ -z "${GITHUB_TOKEN:-}" ]] && [[ -z "${GITLAB_TOKEN:-}" ]] && [[ -z "${GITEA_TOKEN:-}" ]]; then
    if GITHUB_TOKEN=$(gh auth token 2>/dev/null) && [[ -n "$GITHUB_TOKEN" ]]; then
        export GITHUB_TOKEN
        info "Using the GitHub CLI's token"
    else
        error "No GITHUB_TOKEN set and 'gh auth token' returned nothing. Run: gh auth login"
    fi
fi

# Run GoReleaser
info "Building and releasing with GoReleaser..."

# Check if this is a dry run
if [[ "${DRY_RUN:-false}" == "true" ]]; then
    info "DRY RUN: Building local snapshot..."
    "$GORELEASER_BIN" build --snapshot --clean
    success "Dry run completed - binaries in ./dist/"
    exit 0
fi

# Actual release. The tag is created above, before GoReleaser runs, so a failure
# here would otherwise leave it behind and the retry would abort on
# "Version tag already exists" - with nothing published.
if ! "$GORELEASER_BIN" release --clean ${SKIP_SIGN}; then
    warn "Release failed - removing local tag ${VERSION} so this can be retried"
    git tag -d "${VERSION}" >/dev/null
    error "GoReleaser failed"
fi

success "🎉 Release ${VERSION} completed successfully!"
success "Release URL: https://github.com/zach-source/opx/releases/tag/${VERSION}"

echo
echo "📋 Release Summary:"
echo "  • Version: ${VERSION}"
echo "  • Platforms: Linux & macOS (amd64 & arm64)"  
echo "  • Binaries: opx-authd (server) + opx (client)"
if [[ -n "${APPLE_DEVELOPER_ID:-}" ]]; then
    echo "  • Apple Signed: ✅ Yes"
else
    echo "  • Apple Signed: ⚠ No (credentials not configured)"
fi
if [[ -n "${GPG_FINGERPRINT:-}" ]]; then
    echo "  • GPG Signed: ✅ Yes (checksums)"
else
    echo "  • GPG Signed: ⚠ No (GPG_FINGERPRINT not set)"
fi
echo "  • GitHub Release: ✅ Published"
echo
echo "🚀 Next steps:"
echo "  1. Test download and installation: gh release download ${VERSION}"
echo "  2. Update package managers if desired (Homebrew, etc.)"
echo "  3. Announce release to users"