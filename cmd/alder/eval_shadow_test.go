package main

import (
	"strings"
	"testing"
)

// A let that redeclares a name of the --in module: the diagnostic points at the let in the
// expression (line 1, the name's column), and names the module's file for the earlier
// declaration instead of a bare line number of it.
func TestEval_InModuleShadowNamesModule(t *testing.T) {
	t.Parallel()
	needGoTool(t)
	code, stdout, stderr := runEval(t, "", todosDir(t), "--in", "Table.Todo", "let countOpen = 1\ncountOpen")
	want := "input:1:5: error[E0009]: `countOpen` is already declared in app/Table/Todo.alder:34\n"
	if code != 1 || stdout != "" || !strings.Contains(stderr, want) || strings.Contains(stderr, "at line") {
		t.Errorf("exit %d, stdout %q\nstderr: %s\nwant %q", code, stdout, stderr, want)
	}
	// a long input must not have the module's line mapped as one of its own
	long := strings.Repeat("-- pad\n", 40) + "let countOpen = 1\ncountOpen"
	code, _, stderr = runEval(t, long, todosDir(t), "--in", "Table.Todo", "-")
	if code != 1 || !strings.Contains(stderr, "input:41:5: error[E0009]: `countOpen` is already declared in app/Table/Todo.alder:34\n") {
		t.Errorf("long input: exit %d\nstderr: %s", code, stderr)
	}
}
