package main

import (
	"fmt"
	"io"

	"alder/internal/version"
)

// runVersionTo is `alder version`: it prints "alder <version> (go <go version>)", the version
// internal/version.String finds in the binary's build information (a release's or `go
// install`'s module version, "devel+<commit>" for a checkout build, "(devel)" otherwise) and
// the Go version the binary was built with. The generated app__alder.go records the same
// version in its header. Exit 0; 2 for any argument.
func runVersionTo(args []string, stdout, stderr io.Writer) int {
	if len(args) > 0 {
		usageTo(stderr)
		return 2
	}
	fmt.Fprintf(stdout, "alder %s (go %s)\n", version.String(), version.Go())
	return 0
}
