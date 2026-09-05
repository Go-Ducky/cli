#!/usr/bin/env bash
set -euo pipefail

REPO="${DUCKY_REPO:-Go-Ducky/cli}"
INSTALL_DIR="${DUCKY_INSTALL_DIR:-$HOME/.goducky/bin}"
BIN_DIR="${INSTALL_DIR/#\~/$HOME}"

info()  { printf "  \033[1;32m>\033[0m %s\n" "$*"; }
warn()  { printf "  \033[1;33m!\033[0m %s\n" "$*"; }

info "Uninstalling GoDucky CLI"

# Check whether goducky is anywhere on PATH (or at the install dir).
FOUND=""
if command -v goducky >/dev/null 2>&1; then
  FOUND="$(command -v goducky)"
elif [[ -f "$BIN_DIR/goducky" ]]; then
  FOUND="$BIN_DIR/goducky"
fi

# 1. Remove the PATH line(s) the installer added from the user's shell files.
clean_file() {
  local f="$1"
  [[ -f "$f" ]] || return 0
  local tmp
  tmp="$(mktemp)"
  grep -vF "$BIN_DIR" "$f" > "$tmp" || true
  grep -vx '# GoDucky CLI' "$tmp" > "$f" || true
  rm -f "$tmp"
}

for f in \
  "${XDG_CONFIG_HOME:-$HOME/.config}/fish/config.fish" \
  "$HOME/.zshrc" "$HOME/.zprofile" \
  "$HOME/.bashrc" "$HOME/.bash_profile" "$HOME/.profile"; do
  clean_file "$f" || true
done

if [[ -z "$FOUND" ]]; then
  info "GoDucky CLI isn't installed — nothing to do."
  exit 0
fi
info "Removed $BIN_DIR from shell PATH files (if present)"

# 2. Remove the binary and the folders the installer created.
rm -f "$BIN_DIR/goducky"
rmdir "$BIN_DIR" 2>/dev/null || true
rmdir "$HOME/.goducky" 2>/dev/null || true
if [[ -e "$BIN_DIR/goducky" ]]; then
  warn "Could not remove $BIN_DIR/goducky (permissions?). Delete it manually."
else
  info "Removed $BIN_DIR/goducky"
fi

# 3. Offer to remove saved chats and config.
remove_config() {
  local dir=""
  case "$(uname -s)" in
    Darwin) dir="$HOME/Library/Application Support/goducky" ;;
    *)      dir="${XDG_CONFIG_HOME:-$HOME/.config}/goducky" ;;
  esac
  [[ -e "$dir" ]] || return 0
  printf "Remove saved chats and config (%s)? [y/N] " "$dir"
  local ans
  read -r ans
  case "$ans" in
    y|Y|yes) rm -rf "$dir"; info "Removed $dir" ;;
    *)       info "Kept $dir" ;;
  esac
}

if [[ -t 0 ]]; then
  remove_config
else
  info "Tip: to also delete saved chats and config, run this again in a terminal."
fi

info "GoDucky CLI uninstalled. See it again someday: curl -fsSL https://raw.githubusercontent.com/$REPO/main/scripts/install.sh | bash"