package main

import (
	"strings"
	"testing"
)

// MinInt64 is writable as `-9223372036854775808` and prints back the same way; an Int literal
// that does not fit is an error, not MaxInt64 (#81).
func TestEval_Int64Literals(t *testing.T) {
	t.Parallel()
	needGoTool(t)
	evalOK(t, "-9223372036854775808 : Int\n", "-9223372036854775808")
	evalOK(t, "9223372036854775807 : Int\n", "9223372036854775807")
	code, _, stderr := runEval(t, "", todosDir(t), "9223372036854775808")
	if code == 0 || !strings.Contains(stderr, "does not fit in an Int") {
		t.Errorf("oversized literal: exit %d, stderr %q", code, stderr)
	}
}
