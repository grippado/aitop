#!/bin/sh
set -eu

# ---------------------------------------------------------------------------
# aitop installer
#
# Downloads the latest release tarball for this OS/arch from GitHub
# Releases, verifies its checksum, and installs the binary to
# $HOME/.local/bin/aitop.
#
# Usage:
#   curl -fsSL https://raw.githubusercontent.com/grippado/aitop/main/install.sh | sh
# ---------------------------------------------------------------------------

REPO="grippado/aitop"
BIN_DIR="${AITOP_INSTALL_DIR:-$HOME/.local/bin}"

err() { printf 'ERROR: %s\n' "$*" >&2; exit 1; }

command -v curl >/dev/null 2>&1 || err "curl is required"
command -v tar >/dev/null 2>&1 || err "tar is required"

os=$(uname -s | tr '[:upper:]' '[:lower:]')
arch=$(uname -m)
case "$arch" in
  x86_64|amd64) arch=amd64 ;;
  arm64|aarch64) arch=arm64 ;;
  *) err "unsupported architecture: $arch" ;;
esac
case "$os" in
  darwin|linux) : ;;
  *) err "unsupported OS: $os (aitop supports macOS and Linux)" ;;
esac

version=$(curl -fsSL "https://api.github.com/repos/${REPO}/releases/latest" | grep -m1 '"tag_name"' | sed -E 's/.*"([^"]+)".*/\1/')
[ -n "$version" ] || err "could not determine latest release version"
version_num="${version#v}"

archive="aitop_${version_num}_${os}_${arch}.tar.gz"
base_url="https://github.com/${REPO}/releases/download/${version}"

work_dir=$(mktemp -d)
trap 'rm -rf "$work_dir"' EXIT

echo "Downloading ${archive} (${version})..."
curl -fsSL "${base_url}/${archive}" -o "${work_dir}/${archive}"
curl -fsSL "${base_url}/checksums.txt" -o "${work_dir}/checksums.txt"

( cd "$work_dir" && grep " ${archive}\$" checksums.txt | shasum -a 256 -c - ) \
  || err "checksum verification failed for ${archive}"

tar -xzf "${work_dir}/${archive}" -C "$work_dir" aitop
mkdir -p "$BIN_DIR"
mv "${work_dir}/aitop" "${BIN_DIR}/aitop"
chmod +x "${BIN_DIR}/aitop"

echo "Installed aitop to ${BIN_DIR}/aitop"
case ":$PATH:" in
  *":${BIN_DIR}:"*) ;;
  *) echo "Note: ${BIN_DIR} is not on your PATH — add it to your shell profile." ;;
esac
