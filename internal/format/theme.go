package format

import (
	"strings"

	"alder/internal/syntax"
)

// Theme prints `theme { … }` for fs as the formatter lays the declaration out, with no
// trailing newline; "" for no fields. The fields are printed in the order given.
func Theme(fs []*syntax.DeployField) string {
	if len(fs) == 0 {
		return ""
	}
	p := newPrinter(nil)
	p.c = &attached{} // no comments: braced asks it for dangling ones
	return strings.TrimRight(Render(Concat(Text("theme "), p.deployRecord(nil, fs)), Width), "\n")
}
