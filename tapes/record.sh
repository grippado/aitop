#!/usr/bin/env bash
# Render every VHS tape for aitop into its declared Output path.
#
# Renders:
#   vhs.tape          -> demo.gif              (README hero)
#   tapes/social.tape -> docs/assets/aitop-social.gif  (PR / social clip)
#
# VHS needs a real TTY toolchain (ttyd + ffmpeg) and the monospace/Nerd Font
# named in each tape's `Set FontFamily`. It runs fine headless (no X needed),
# which is why a Linux box/VPS is a great place to run this.
#
# Usage:  ./tapes/record.sh
set -euo pipefail

cd "$(dirname "$0")/.."   # repo root

need() { command -v "$1" >/dev/null 2>&1; }

missing=()
for t in vhs ttyd ffmpeg; do need "$t" || missing+=("$t"); done
if ((${#missing[@]})); then
  echo "⚠️  Missing: ${missing[*]}"
  echo "   macOS:  brew install vhs        # pulls ttyd + ffmpeg"
  echo "   Debian/Ubuntu:"
  echo "     sudo apt-get update && sudo apt-get install -y ffmpeg"
  echo "     # ttyd + vhs: see https://github.com/charmbracelet/vhs#installation"
  echo "     go install github.com/charmbracelet/vhs@latest   # if Go is present"
  exit 1
fi

# The tapes name a Nerd Font; without it, box-drawing/bars silently misalign.
FONT='JetBrainsMono Nerd Font'
if need fc-list && ! fc-list | grep -qi "JetBrainsMono Nerd Font"; then
  echo "ℹ️  '$FONT' not found — installing it for a faithful render…"
  dir="${XDG_DATA_HOME:-$HOME/.local/share}/fonts"
  mkdir -p "$dir"
  url="https://github.com/ryanoasis/nerd-fonts/releases/latest/download/JetBrainsMono.zip"
  tmp="$(mktemp -d)"
  if need curl; then curl -fsSL "$url" -o "$tmp/f.zip"; else wget -qO "$tmp/f.zip" "$url"; fi
  (cd "$tmp" && unzip -oq f.zip -d jbm) && cp "$tmp"/jbm/*.ttf "$dir/" 2>/dev/null || true
  fc-cache -f "$dir" >/dev/null 2>&1 || true
  rm -rf "$tmp"
fi

echo "🔨 Building aitop…"
go build -o aitop ./cmd/aitop/

for tape in vhs.tape tapes/social.tape; do
  [ -f "$tape" ] || { echo "skip (missing): $tape"; continue; }
  echo "🎬 vhs $tape"
  vhs "$tape"
done

echo "✅ Done. Outputs:"
ls -lh demo.gif docs/assets/aitop-social.gif 2>/dev/null || true
echo "   Commit the gif(s) to the branch and drop aitop-social.gif into the PR's 'show it off' slot."
