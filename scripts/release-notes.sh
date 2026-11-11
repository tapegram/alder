#!/bin/sh
# Prints the body of CHANGELOG.md's section for a tag: the lines under the heading that
# starts "## vX.Y.Z" (followed by a space or the end of the line) up to the next "## "
# heading. Exits 1 when the section is missing or blank. Usage: release-notes.sh vX.Y.Z
set -eu
if [ $# -ne 1 ]; then echo "usage: release-notes.sh vX.Y.Z" >&2; exit 2; fi
root=$(cd "$(dirname "$0")/.." && pwd)
body=$(awk -v tag="$1" '
	/^## / {
		if (on) exit
		if (index($0, "## " tag) == 1) {
			rest = substr($0, length("## " tag) + 1)
			if (rest == "" || substr(rest, 1, 1) == " ") on = 1
		}
		next
	}
	on { print }
' "$root/CHANGELOG.md")
if [ -z "$(printf '%s' "$body" | tr -d '[:space:]')" ]; then
	echo "release-notes: CHANGELOG.md has no section for $1" >&2
	exit 1
fi
printf '%s\n' "$body"
