#!/usr/bin/env bash
# Download the latest terminal-games release into /tmp and run it.
# The binary is removed when the game exits — nothing is installed.

set -euo pipefail

REPO="BenjaminBenetti/terminal-games"

if [ "$(uname -s)" != "Linux" ]; then
  echo "terminal-games: Linux only (got $(uname -s))" >&2
  exit 1
fi

case "$(uname -m)" in
  x86_64|amd64)  arch=amd64 ;;
  aarch64|arm64) arch=arm64 ;;
  *) echo "terminal-games: unsupported architecture: $(uname -m)" >&2; exit 1 ;;
esac

asset="terminal-games-linux-${arch}"
url="https://github.com/${REPO}/releases/latest/download/${asset}"

dir=$(mktemp -d /tmp/terminal-games-XXXXXX)
bin="${dir}/terminal-games"
trap 'rm -rf "${dir}"' EXIT

echo "terminal-games: downloading ${asset}..." >&2
curl -fsSL --retry 3 "${url}" -o "${bin}"
chmod +x "${bin}"

# Reattach stdin to the terminal so `curl ... | bash` still gives the
# game a real TTY for input.
"${bin}" "$@" </dev/tty
