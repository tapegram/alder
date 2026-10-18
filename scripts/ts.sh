#!/bin/sh
# scripts/ts.sh generate|test|parse [args…] — the tree-sitter CLI for tree-sitter-alder/.
# generate and test run inside tree-sitter-alder/; parse runs from the current directory with
# -p, so file arguments are relative to where you are. Only the CLI version CI pins may
# regenerate src/: another version writes a different parser.c and the regeneration check fails.
set -eu
want=0.27.1 # also pinned in tree-sitter-alder/drift_test.go (pinnedCLI) and .github/install-tree-sitter.sh; change all three together
grammar=$(cd "$(dirname "$0")/../tree-sitter-alder" && pwd)
command -v tree-sitter >/dev/null 2>&1 || { echo "tree-sitter CLI not found: brew install tree-sitter-cli" >&2; exit 2; }
cmd=${1:-}
[ $# -gt 0 ] && shift
case "$cmd" in
  generate)
    have=$(tree-sitter --version | awk '{print $2}')
    if [ "$have" != "$want" ]; then
      echo "tree-sitter $have: src/ must be generated with $want, the version CI pins (.github/install-tree-sitter.sh)" >&2
      exit 2
    fi
    (cd "$grammar" && TREE_SITTER_JS_RUNTIME=native tree-sitter generate "$@") ;;
  test)
    (cd "$grammar" && tree-sitter test "$@") ;;
  parse)
    tree-sitter parse -p "$grammar" "$@" ;;
  *)
    echo "usage: scripts/ts.sh generate|test|parse [args…]" >&2
    exit 2 ;;
esac
