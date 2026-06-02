#!/usr/bin/env bash
set -euo pipefail

repo="${DEX_REPO:-desenyon/dex}"
version="${DEX_VERSION:-latest}"
install_dir="${DEX_INSTALL_DIR:-/usr/local/bin}"

os="$(uname -s)"
arch="$(uname -m)"

case "$os" in
  Darwin) os="Darwin" ;;
  Linux) os="Linux" ;;
  *) echo "unsupported OS: $os" >&2; exit 1 ;;
esac

case "$arch" in
  arm64|aarch64) arch="arm64" ;;
  x86_64|amd64) arch="amd64" ;;
  *) echo "unsupported architecture: $arch" >&2; exit 1 ;;
esac

asset="dex_${os}_${arch}.tar.gz"
base_url="https://github.com/${repo}/releases"
if [[ "$version" == "latest" ]]; then
  url="${base_url}/latest/download/${asset}"
else
  url="${base_url}/download/${version}/${asset}"
fi

tmp="$(mktemp -d)"
trap 'rm -rf "$tmp"' EXIT

echo "Downloading ${asset} from ${repo}..."
curl -fsSL "$url" -o "$tmp/$asset"
tar -xzf "$tmp/$asset" -C "$tmp"

if [[ ! -w "$install_dir" ]]; then
  mkdir -p "$HOME/.local/bin"
  install_dir="$HOME/.local/bin"
fi

install -m 755 "$tmp/dex" "$install_dir/dex"
echo "Installed dex to ${install_dir}/dex"
echo "Run: dex --help"
