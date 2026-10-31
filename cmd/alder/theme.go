package main

import "io"

const themeUsage = "usage: alder theme import <file.css | -> [--print] [--dir app] [--json] [--dry-run]"

// runThemeTo is `alder theme import` (#38 phase 2): a shadcn or tweakcn CSS export becomes
// App's `theme` declaration, and what alder-ui has no token for comes back as a block for
// static/app.css. With --print the declaration goes to standard output and nothing is written.
func runThemeTo(args []string, stdout, stderr io.Writer) int {
	return runGroup("theme", themeUsage, args, stdout, stderr)
}
