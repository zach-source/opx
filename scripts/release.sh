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

# Check for signing credentials
if [[ -n "${APPLE_DEVELOPER_ID:-}" ]] && [[ -n "${MACOS_SIGN_P12:-}" ]]; then
    info "Apple Developer credentials detected - macOS binaries will be signed"
else
    warn "Apple Developer credentials not set - macOS binaries will not be signed"
    warn "To enable signing, set these environment variables:"
    warn "  export APPLE_DEVELOPER_ID='Developer ID Application: Your Name (TEAMID)'"
    warn "  export MACOS_SIGN_P12='path/to/certificate.p12'"
    warn "  export MACOS_SIGN_PASSWORD='certificate-password'"
fi

if [[ -n "${GPG_FINGERPRINT:-}" ]]; then
    info "GPG fingerprint detected - checksums will be signed"
else
    warn "GPG_FINGERPRINT not set - checksums will not be signed"
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

# Actual release
"$GORELEASER_BIN" release --clean

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