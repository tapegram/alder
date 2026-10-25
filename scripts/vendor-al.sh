#!/bin/sh
# vendor-al.sh <tag>: copy the alder-ui design system at <tag> into rt/al/.
#
# A plain copy at a tag: al.css and al.js (the bundles), custom-elements.json (the
# manifest the checker reads), the Geist fonts with their OFL.txt (fonts/), and the
# LICENSE and LICENSE-lucide files, with the tag written to rt/al/VERSION. Nothing
# fetches at build time; the copy is committed and embedded by rt/embed.go.
# Uses `gh repo clone` when gh is installed (the repository may be private),
# else `git clone`. Reads the manifest's version with python3's json module.
set -eu

if [ $# -ne 1 ]; then
	echo "usage: scripts/vendor-al.sh <tag>   (for example v0.1.2)" >&2
	exit 2
fi
tag=$1
repo=tapegram/alder-ui
here=$(cd "$(dirname "$0")/.." && pwd)
dest=$here/rt/al
tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT

if command -v gh >/dev/null 2>&1; then
	gh repo clone "$repo" "$tmp/alder-ui" -- -c advice.detachedHead=false --quiet --depth 1 --branch "$tag"
else
	git -c advice.detachedHead=false clone --quiet --depth 1 --branch "$tag" "https://github.com/$repo" "$tmp/alder-ui"
fi

version=$(python3 -c 'import json, sys; print(json.load(open(sys.argv[1])).get("version", ""))' "$tmp/alder-ui/custom-elements.json")
if [ "$version" != "$tag" ]; then
	echo "vendor-al: the manifest at $tag says version \"$version\"" >&2
	exit 1
fi

mkdir -p "$dest/fonts"
cp "$tmp/alder-ui/al.css" "$dest/al.css"
cp "$tmp/alder-ui/al.js" "$dest/al.js"
cp "$tmp/alder-ui/custom-elements.json" "$dest/custom-elements.json"
cp "$tmp/alder-ui/fonts/Geist-Variable.woff2" "$dest/fonts/Geist-Variable.woff2"
cp "$tmp/alder-ui/fonts/GeistMono-Variable.woff2" "$dest/fonts/GeistMono-Variable.woff2"
cp "$tmp/alder-ui/fonts/OFL.txt" "$dest/fonts/OFL.txt"
cp "$tmp/alder-ui/LICENSE" "$dest/LICENSE"
cp "$tmp/alder-ui/LICENSE-lucide" "$dest/LICENSE-lucide"
printf '%s\n' "$tag" >"$dest/VERSION"
echo "vendored alder-ui $tag into rt/al/"
