#!/bin/bash
set -euo pipefail

RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[0;33m'
BLUE='\033[0;34m'
NC='\033[0m'

info() { echo -e "${BLUE}ℹ ${1}${NC}"; }
success() { echo -e "${GREEN}✅ ${1}${NC}"; }
warn() { echo -e "${YELLOW}⚠ ${1}${NC}"; }
error() { echo -e "${RED}❌ ${1}${NC}"; exit 1; }

SVU_BIN=$(go env GOPATH)/bin/svu
if [[ ! -x "$SVU_BIN" ]]; then
    error "svu is required. Install: go install github.com/caarlos0/svu@latest"
fi

if ! command -v gh &> /dev/null; then
    error "GitHub CLI is required. Install: https://cli.github.com/"
fi

if ! git rev-parse --is-inside-work-tree &> /dev/null; then
    error "Not inside a git repository"
fi

if ! git diff-index --quiet HEAD --; then
    error "Repository has uncommitted changes. Please commit or stash them."
fi

VERSION=$($SVU_BIN next)

if git tag -l | grep -q "^${VERSION}$"; then
    error "Version tag ${VERSION} already exists"
fi

info "Preparing release ${VERSION}..."

info "Running tests..."
go test ./... || error "Tests failed"
success "Tests passed"

info "Building binaries..."
make clean && make build || error "Build failed"
success "Binaries built"

ARCH=$(arch)
info "Signing binaries for darwin_${ARCH}..."
if security find-identity -v -p codesigning | grep -q "Apple Development"; then
    SIGNING_ID=$(security find-identity -v -p codesigning | grep "Apple Development" | head -1 | awk '{print $2}')
    codesign --sign "$SIGNING_ID" --force bin/opx bin/opx-authd
    success "Binaries signed"
else
    warn "No signing identity found - binaries will not be signed"
fi

info "Creating release archives..."
mkdir -p dist
cd bin
tar czf ../dist/opx-server_${VERSION}_darwin_${ARCH}_signed.tar.gz opx-authd
tar czf ../dist/opx-client_${VERSION}_darwin_${ARCH}_signed.tar.gz opx
cd ..

info "Generating checksums..."
cd dist
shasum -a 256 *.tar.gz > checksums.txt
cd ..
success "Archives and checksums created"

info "Creating git tag ${VERSION}..."
git tag "${VERSION}"
success "Git tag created"

info "Pushing tag to GitHub..."
git push origin "${VERSION}"
success "Tag pushed"

info "Creating GitHub release..."
gh release create "${VERSION}" \
  --title "opx ${VERSION}" \
  --notes "See commit history for details: https://github.com/zach-source/opx/compare/$(git describe --tags --abbrev=0 HEAD^)...${VERSION}" \
  dist/*_signed.tar.gz dist/checksums.txt
success "GitHub release created"

RELEASE_URL="https://github.com/zach-source/opx/releases/tag/${VERSION}"
info "Fetching checksums from release..."
SERVER_SHA=$(grep "opx-server_${VERSION}_darwin_${ARCH}_signed.tar.gz" dist/checksums.txt | awk '{print $1}')
CLIENT_SHA=$(grep "opx-client_${VERSION}_darwin_${ARCH}_signed.tar.gz" dist/checksums.txt | awk '{print $1}')

HOMEBREW_TAP_DIR="${HOME}/repos/workspaces/homebrew-tap"
if [[ -d "$HOMEBREW_TAP_DIR" ]]; then
    info "Updating Homebrew tap..."
    cd "$HOMEBREW_TAP_DIR"
    
    sed -i.bak "s|releases/download/v[^/]*/opx-server_v[^\"]*|releases/download/${VERSION}/opx-server_${VERSION}_darwin_${ARCH}_signed.tar.gz|g" Formula/opx.rb
    sed -i.bak "s|releases/download/v[^/]*/opx-client_v[^\"]*|releases/download/${VERSION}/opx-client_${VERSION}_darwin_${ARCH}_signed.tar.gz|g" Formula/opx.rb
    sed -i.bak "/opx-server.*sha256/s/sha256 \"[^\"]*\"/sha256 \"${SERVER_SHA}\"/" Formula/opx.rb
    sed -i.bak "/opx-client.*sha256/s/sha256 \"[^\"]*\"/sha256 \"${CLIENT_SHA}\"/" Formula/opx.rb
    rm -f Formula/opx.rb.bak
    
    git add Formula/opx.rb
    git commit -m "chore: update opx formula to ${VERSION}"
    git push origin main
    success "Homebrew tap updated"
    cd - > /dev/null
else
    warn "Homebrew tap directory not found at ${HOMEBREW_TAP_DIR}"
fi

NIX_PACKAGES_DIR="${HOME}/repos/workspaces/nix-packages"
if [[ -d "$NIX_PACKAGES_DIR" ]]; then
    info "Updating Nix packages..."
    cd "$NIX_PACKAGES_DIR"
    
    sed -i.bak "s/version = \"[^\"]*\"/version = \"${VERSION#v}\"/" flake.nix
    sed -i.bak "s|releases/download/v[^/]*/opx-server_[^\"]*|releases/download/${VERSION}/opx-server_${VERSION}_darwin_${ARCH}_signed.tar.gz|g" flake.nix
    sed -i.bak "s|releases/download/v[^/]*/opx-client_[^\"]*|releases/download/${VERSION}/opx-client_${VERSION}_darwin_${ARCH}_signed.tar.gz|g" flake.nix
    sed -i.bak "/opx-server.*sha256/s/sha256 = \"[^\"]*\"/sha256 = \"${SERVER_SHA}\"/" flake.nix
    sed -i.bak "/clientSrc.*sha256/s/sha256 = \"[^\"]*\"/sha256 = \"${CLIENT_SHA}\"/" flake.nix
    rm -f flake.nix.bak
    
    nix flake check || warn "Nix flake check failed (non-fatal)"
    
    git add flake.nix
    git commit -m "chore: update opx to ${VERSION}"
    git push origin main
    success "Nix packages updated"
    cd - > /dev/null
else
    warn "Nix packages directory not found at ${NIX_PACKAGES_DIR}"
fi

success "🎉 Release ${VERSION} completed successfully!"
echo
echo "📦 Release Details:"
echo "  • Version: ${VERSION}"
echo "  • GitHub: ${RELEASE_URL}"
echo "  • Homebrew: brew install zach-source/tap/opx"
echo "  • Nix: nix profile install github:zach-source/nix-packages#opx"
echo
echo "📋 Package Manager Status:"
[[ -d "$HOMEBREW_TAP_DIR" ]] && echo "  ✅ Homebrew tap updated" || echo "  ⚠ Homebrew tap skipped"
[[ -d "$NIX_PACKAGES_DIR" ]] && echo "  ✅ Nix packages updated" || echo "  ⚠ Nix packages skipped"