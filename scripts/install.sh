#!/usr/bin/env bash
# Install agenterm native binary from GitHub Releases (no container).
# Usage:
#   curl -fsSL https://raw.githubusercontent.com/saurabhahuja71/agenterm/main/scripts/install.sh | bash
#   INSTALL_DIR=~/bin ./scripts/install.sh
set -euo pipefail

REPO="${AGENTERM_REPO:-saurabhahuja71/agenterm}"
VERSION="${AGENTERM_VERSION:-latest}"
INSTALL_DIR="${INSTALL_DIR:-${HOME}/.local/bin}"

os="$(uname -s | tr '[:upper:]' '[:lower:]')"
arch="$(uname -m)"
case "$arch" in
  x86_64|amd64) arch=amd64 ;;
  aarch64|arm64) arch=arm64 ;;
  *) echo "unsupported arch: $arch" >&2; exit 1 ;;
esac
case "$os" in
  linux|darwin) ;;
  msys*|mingw*|cygwin*) os=windows ;;
  *) echo "unsupported OS: $os" >&2; exit 1 ;;
esac

ext=""
[[ "$os" == windows ]] && ext=".exe"
asset="agenterm-${os}-${arch}${ext}"

if [[ "$VERSION" == latest ]]; then
  url="https://github.com/${REPO}/releases/latest/download/${asset}"
else
  url="https://github.com/${REPO}/releases/download/${VERSION}/${asset}"
fi

mkdir -p "$INSTALL_DIR"
tmp="$(mktemp)"
echo "Downloading ${url}"
curl -fsSL -o "$tmp" "$url"
chmod +x "$tmp"
mv "$tmp" "${INSTALL_DIR}/agenterm${ext}"
echo "Installed ${INSTALL_DIR}/agenterm${ext}"
echo "Ensure ${INSTALL_DIR} is on your PATH, then run: agenterm --ping && agenterm"
