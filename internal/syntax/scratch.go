package syntax

import (
	"path"
	"strings"
)

// IsScratchName reports whether a file (a slash-separated path) is named as a scratch file: its
// base name ends in "scratch.alder" (scratch.alder, Todos.scratch.alder). Such a file is not a
// module. internal/scratch (IsScratchFile, which also requires it to be outside app/) and
// internal/names (E0036, one under app/) share the rule.
func IsScratchName(file string) bool {
	return strings.HasSuffix(path.Base(file), "scratch.alder")
}
