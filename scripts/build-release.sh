#!/usr/bin/env bash
set -euo pipefail

root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$root"

go_bin="${GO:-.tools/go-root/bin/go}"
version="${VERSION:-dev}"
commit="$(git rev-parse --short HEAD 2>/dev/null || echo local)"
dist="$root/dist"

rm -rf "$dist"
mkdir -p "$dist"

targets=(
  "Darwin arm64"
  "Darwin amd64"
  "Linux arm64"
  "Linux amd64"
)

for target in "${targets[@]}"; do
  read -r goos goarch <<<"$target"
  out_dir="$dist/dex_${goos}_${goarch}"
  mkdir -p "$out_dir"
  GOOS="$(tr '[:upper:]' '[:lower:]' <<<"$goos")" GOARCH="$goarch" \
    "$go_bin" build -ldflags "-s -w -X main.version=${version} -X main.commit=${commit}" -o "$out_dir/dex" ./cmd/dex
  cp README.md LICENSE "$out_dir/"
  tar -C "$out_dir" -czf "$dist/dex_${goos}_${goarch}.tar.gz" dex README.md LICENSE
done

(cd "$dist" && shasum -a 256 *.tar.gz > checksums.txt)
echo "Release artifacts written to $dist"
