#!/usr/bin/env bash
set -euo pipefail

DEFAULT_VERSION="latest"
REPO="${DUCKY_REPO:-Go-Ducky/cli}"
INSTALL_DIR="${DUCKY_INSTALL_DIR:-$HOME/.goducky/bin}"
VERSION="${1:-$DEFAULT_VERSION}"

info()  { printf "  \033[1;32m>\033[0m %s\n" "$*"; }
warn()  { printf "  \033[1;33m!\033[0m %s\n" "$*"; }
error() { printf "  \033[1;31mX\033[0m %s\n" "$*" >&2; exit 1; }

OS="$(uname -s)"
ARCH="$(uname -m)"

case "$OS" in
  Linux)  GOOS="linux" ;;
  Darwin) GOOS="darwin" ;;
  *) error "Unsupported OS: $OS (supported: linux, macos)" ;;
esac

case "$ARCH" in
  x86_64|amd64) GOARCH="amd64" ;;
  aarch64|arm64) GOARCH="arm64" ;;
  *) error "Unsupported architecture: $ARCH" ;;
esac

TARGET_OS="$GOOS"
TARGET_ARCH="$GOARCH"
if [[ "$GOOS" == "darwin" && "$GOARCH" == "arm64" ]]; then
  TARGET_OS="darwin"
  TARGET_ARCH="arm64"
fi

ASSET="goducky-${TARGET_OS}-${TARGET_ARCH}"

if [[ "$VERSION" == "latest" ]]; then
  RELEASE_JSON="$(curl -fsSL "https://api.github.com/repos/$REPO/releases?per_page=1" || true)"
  TAG="$(printf '%s' "$RELEASE_JSON" | grep -o '"tag_name": *"[^"]*"' | head -1 | sed 's/.*"tag_name": *"//; s/"$//')"
  if [[ -z "$TAG" ]]; then
    error "Could not determine latest version"
  fi
  info "Latest release: $TAG"
else
  TAG="v${VERSION}"
fi

BINARY_URL="https://github.com/$REPO/releases/download/${TAG}/${ASSET}"
DOWNLOAD_URL="$BINARY_URL"
EXT=""

if ! curl -fsSL -o /dev/null --range 0-0 "$BINARY_URL" 2>/dev/null; then
	DOWNLOAD_URL="${BINARY_URL}.zip"
  EXT=".zip"
fi

info "Downloading $ASSET v$VERSION"
TMP_DIR="$(mktemp -d)"
trap 'rm -rf "$TMP_DIR"' EXIT

if ! curl -fsSL -o "$TMP_DIR/goducky" "$DOWNLOAD_URL"; then
  error "Failed to download $DOWNLOAD_URL"
fi

if [[ "$EXT" == ".zip" ]]; then
  (cd "$TMP_DIR" && unzip -o goducky >/dev/null && rm -f goducky)
  BIN="$(find "$TMP_DIR" -type f -name 'goducky*' | head -1)"
  mv "$BIN" "$TMP_DIR/goducky"
fi

install_dir="$(cd "$HOME" && pwd)/.goducky"
BIN_DIR="${DUCKY_INSTALL_DIR:-$INSTALL_DIR}"
BIN_DIR="${BIN_DIR/#\~/$HOME}"
mkdir -p "$BIN_DIR"

info "Installing to $BIN_DIR/goducky"
install -m 755 "$TMP_DIR/goducky" "$BIN_DIR/goducky"

# macOS Gatekeeper: downloaded binaries are quarantined and flagged as unsigned,
# which blocks the first run. Clear the quarantine flag and ad-hoc sign so the
# binary runs on both Apple Silicon (arm64) and Intel (amd64).
if [[ "$OS" == "Darwin" ]]; then
  xattr -d com.apple.quarantine "$BIN_DIR/goducky" 2>/dev/null || true
  if command -v codesign >/dev/null 2>&1; then
    codesign -s - --force "$BIN_DIR/goducky" 2>/dev/null || true
  fi
fi

# append_line <file> <line> -- appends a marker line at most once.
append_line() {
  local file="$1" line="$2"
  [ -f "$file" ] || touch "$file"
  grep -qF "$line" "$file" && return 0
  printf '\n# GoDucky CLI\n%s\n' "$line" >> "$file"
  info "Added $BIN_DIR to PATH in $file"
}

# auto-add the install dir to PATH for the user's shell (idempotent).
if [[ ":$PATH:" != *":$BIN_DIR:"* ]]; then
  SHELL_NAME="$(basename "${SHELL:-}")"
  case "$SHELL_NAME" in
    fish)
      append_line "${XDG_CONFIG_HOME:-$HOME/.config}/fish/config.fish" "set -gx PATH \"$BIN_DIR\" \$PATH" ;;
    zsh)
      append_line "$HOME/.zshrc" "export PATH=\"$BIN_DIR:\$PATH\""
      [ -f "$HOME/.zprofile" ] && append_line "$HOME/.zprofile" "export PATH=\"$BIN_DIR:\$PATH\"" ;;
    *)
      append_line "$HOME/.bashrc" "export PATH=\"$BIN_DIR:\$PATH\""
      [ -f "$HOME/.bash_profile" ] && append_line "$HOME/.bash_profile" "export PATH=\"$BIN_DIR:\$PATH\""
      append_line "$HOME/.profile" "export PATH=\"$BIN_DIR:\$PATH\"" ;;
  esac
else
  info "$BIN_DIR already on PATH"
fi

info "Verifying install..."
if "$BIN_DIR/goducky" --version; then
  info "GoDucky CLI installed successfully! Run 'goducky' to start."
else
  error "Installation verification failed"
fi
