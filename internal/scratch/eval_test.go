package scratch

import (
	"testing"

	"alder/internal/types"
)

func TestClassifyRun(t *testing.T) {
	t.Parallel()
	for _, c := range []struct {
		errOut  string
		exit    int
		msg     string
		outcome RunOutcome
	}{
		{"", 0, "", RunOK},
		{"scratch: exec: no such file\n", -1, "scratch: exec: no such file", RunInternal},
		{"runtime: goroutine stack exceeds 1000000000-byte limit\nfatal error: stack overflow\n\nruntime stack:\n…", 2, "stack overflow (unbounded recursion?)", RunCrashed},
		{"scratch: /tmp/x y/build/todos/scratch/eval/scratch timed out after 1m0s\n", -1, "the evaluation timed out after 1m0s", RunCrashed},
		{"panic: list index out of range\n\ngoroutine 1 [running]:\nmain.main()\n", 2, "panic: list index out of range", RunFailed},
		{"no such todo\n", 1, "no such todo", RunFailed},
	} {
		if msg, outcome := ClassifyRun(c.errOut, c.exit); msg != c.msg || outcome != c.outcome {
			t.Errorf("%q %d: got %q %d, want %q %d", c.errOut, c.exit, msg, outcome, c.msg, c.outcome)
		}
	}
}

func TestGoValue(t *testing.T) {
	t.Parallel()
	a := &types.Param{Name: "a"}
	for _, c := range []struct {
		name  string
		sig   types.Type
		thunk bool
		want  string
	}{
		{"plain", types.Int, false, "app.Scratch__it"},
		{"plain thunk", types.Int, true, "app.Scratch__it()"},
		{"generic value", &types.App{Con: "List", Args: []types.Type{a}}, false, "app.Scratch__it[any]()"},
		{"generic value thunk", &types.App{Con: "List", Args: []types.Type{a}}, true, "app.Scratch__it[any]()"},
		{"function", &types.Fun{Params: []types.Type{types.Int}, Result: types.Int}, true, "app.Scratch__it"},
		{"generic function", &types.Fun{Params: []types.Type{a}, Result: a}, true, "app.Scratch__it[any]"},
	} {
		if got := GoValue("Scratch", "it", c.sig, c.thunk); got != c.want {
			t.Errorf("%s: got %q, want %q", c.name, got, c.want)
		}
	}
	if got := GoValue("Scratch", "it3", types.Int, true); got != "app.Scratch__it3()" {
		t.Errorf("named thunk: %q", got)
	}
}
