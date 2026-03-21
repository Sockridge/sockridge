#!/bin/sh
set -e

# Sockridge CLI installer
# Usage: curl -fsSL https://sockridge.com/install.sh | sh

REPO="Sockridge/sockridge"
BINARY="sockridge"
INSTALL_DIR="/usr/local/bin"

# colors
RED='\033[0;31m'
GREEN='\033[0;32m'
CYAN='\033[0;36m'
DIM='\033[2m'
RESET='\033[0m'

log()     { printf "${CYAN}=>${RESET} %s\n" "$1"; }
success() { printf "${GREEN}✓${RESET} %s\n" "$1"; }
error()   { printf "${RED}✗ error:${RESET} %s\n" "$1" >&2; exit 1; }

# detect OS
OS=$(uname -s | tr '[:upper:]' '[:lower:]')
case "$OS" in
  linux)  OS="linux" ;;
  darwin) OS="macos" ;;
  *)      error "Unsupported OS: $OS" ;;
esac

# detect arch
ARCH=$(uname -m)
case "$ARCH" in
  x86_64 | amd64) ARCH="amd64" ;;
  arm64 | aarch64) ARCH="arm64" ;;
  *) error "Unsupported architecture: $ARCH" ;;
esac

# get latest release tag
log "Fetching latest release..."
LATEST=$(curl -fsSL "https://api.github.com/repos/${REPO}/releases/latest" \
  | grep '"tag_name"' \
  | sed 's/.*"tag_name": *"\([^"]*\)".*/\1/')

if [ -z "$LATEST" ]; then
  error "Could not fetch latest release. Check https://github.com/${REPO}/releases"
fi

log "Installing sockridge ${LATEST} (${OS}/${ARCH})..."

# download binary
FILENAME="${BINARY}-${OS}-${ARCH}"
URL="https://github.com/${REPO}/releases/download/${LATEST}/${FILENAME}"
TMP=$(mktemp)

curl -fsSL "$URL" -o "$TMP" || error "Download failed from ${URL}"
chmod +x "$TMP"

# install
if [ -w "$INSTALL_DIR" ]; then
  mv "$TMP" "${INSTALL_DIR}/${BINARY}"
else
  sudo mv "$TMP" "${INSTALL_DIR}/${BINARY}"
fi

success "sockridge installed to ${INSTALL_DIR}/${BINARY}"
echo ""
echo "${DIM}Get started:${RESET}"
echo "  sockridge auth keygen"
echo "  sockridge auth register --handle yourname --server https://sockridge.com:9000"
echo "  sockridge auth login --server https://sockridge.com:9000"
echo "  sockridge publish --file agent.json"
echo ""
echo "${DIM}Docs: https://sockridge.com${RESET}"
