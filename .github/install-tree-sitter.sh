#!/bin/sh
# Installs the tree-sitter CLI that tree-sitter-alder/src is generated with, pinned so the
# regeneration check compares like with like. Puts it on PATH for later steps (GITHUB_PATH).
set -eu
# The pin is in three places; change all three together: here, scripts/ts.sh (`want`) and
# tree-sitter-alder/drift_test.go (`pinnedCLI`).
version=0.27.1
case "$(uname -s)-$(uname -m)" in
  Linux-x86_64) asset=linux-x64 ;;
  Linux-aarch64) asset=linux-arm64 ;;
  Darwin-arm64) asset=macos-arm64 ;;
  Darwin-x86_64) asset=macos-x64 ;;
  *) echo "no tree-sitter $version build for $(uname -sm)" >&2; exit 1 ;;
esac
dir="${RUNNER_TEMP:-/tmp}/tree-sitter-bin"
mkdir -p "$dir"
curl -fsSL "https://github.com/tree-sitter/tree-sitter/releases/download/v$version/tree-sitter-$asset.gz" | gunzip > "$dir/tree-sitter"
chmod +x "$dir/tree-sitter"
if [ -n "${GITHUB_PATH:-}" ]; then echo "$dir" >> "$GITHUB_PATH"; fi
"$dir/tree-sitter" --version
