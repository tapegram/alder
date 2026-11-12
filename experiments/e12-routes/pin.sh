#!/usr/bin/env bash
# pin.sh: builds from, or exports from, one commit of this repository.
#   pin.sh binary COMMIT           build experiments/e12-routes/alder from `git archive COMMIT`
#                                  (go build ./cmd/alder); print its sha256 and `alder version`
#   pin.sh prompt COMMIT OUT_DIR   in a fresh export of COMMIT run experiments/build_context.py
#                                  --reference > OUT_DIR/prompt-reference.md and
#                                  --reference --size > OUT_DIR/sizes.md
# Each removes its export.
set -eu
HERE="$(cd "$(dirname "$0")" && pwd)"
ROOT="$(cd "$HERE/../.." && pwd)"
usage() { echo "usage: pin.sh binary COMMIT | pin.sh prompt COMMIT OUT_DIR" >&2; exit 2; }
[ $# -ge 2 ] || usage
mode="$1"; commit="$2"
EXPORT="$(mktemp -d "${TMPDIR:-/tmp}/e12-pin.XXXXXX")"
trap 'rm -rf "$EXPORT"' EXIT
(cd "$ROOT" && git archive "$commit") | tar -x -C "$EXPORT"
case "$mode" in
  binary)
    (cd "$EXPORT" && go build -o "$HERE/alder" ./cmd/alder)
    echo "sha256 $(shasum -a 256 "$HERE/alder" | awk '{print $1}')"
    "$HERE/alder" version | head -1
    ;;
  prompt)
    [ $# -eq 3 ] || usage
    out="$3"; mkdir -p "$out"; out="$(cd "$out" && pwd)"
    (cd "$EXPORT" && python3 experiments/build_context.py --reference > "$out/prompt-reference.md")
    (cd "$EXPORT" && python3 experiments/build_context.py --reference --size > "$out/sizes.md")
    ;;
  *) usage ;;
esac
