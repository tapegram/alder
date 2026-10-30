package gen

import (
	"os"
	"path/filepath"
	"testing"
)

// A Go adapter that writes into the Lists and Bytes it is handed cannot change the caller's
// values (#37). The runtime shares storage (File.text's body is the Text's own bytes, List.take
// is a view), so without rt.Fresh at the capability boundary the adapter below would turn a heap
// Text "hello world" into "HELLO WORLD", fault (SIGBUS) writing a literal's read-only bytes, and
// reorder the list a List.take was cut from.
func TestGen_CapabilityArgsAreFresh(t *testing.T) {
	dir := emitApp(t, "../../testdata/corpus/capability-args-fresh-ok")
	if err := os.WriteFile(filepath.Join(dir, "zz_fresh_test.go"), []byte(capFreshTest), 0o644); err != nil {
		t.Fatal(err)
	}
	goRun(t, dir, "test", "-count=1", "-run", "TestFresh", ".")
}

const capFreshTest = `package app

import (
	"context"
	"slices"
	"sort"
	"testing"

	"alder/rt/eff"
)

// scribble writes into every argument it gets, as an adapter is free to.
type scribble struct{}

func upper(b []byte) {
	for i, c := range b {
		if 'a' <= c && c <= 'z' {
			b[i] = c - 32
		}
	}
}

func (scribble) Shout(ctx context.Context, args Args__Mut__Scribble__Shout) (int64, error) {
	upper(args.Value)
	return int64(len(args.Value)), nil
}

func (scribble) SortAll(ctx context.Context, args Args__Mut__Scribble__SortAll) (int64, error) {
	sort.Strings(args.Value)
	full := args.Value[:cap(args.Value)]
	for i := range full {
		full[i] = "zz"
	}
	return int64(len(args.Value)), nil
}

func (scribble) ShoutRow(ctx context.Context, args Args__Mut__Scribble__ShoutRow) (int64, error) {
	upper(args.Raw)
	slices.Reverse(args.Tags)
	return 0, nil
}

var _ Cap__Mut__Scribble = scribble{}

func run[T any](t *testing.T, task eff.Task[T]) T {
	t.Helper()
	env := &eff.Env{Ctx: context.Background(), Caps: map[string]any{"Mut.Scribble": scribble{}}}
	v, err := eff.Run(env, eff.KTask, task)
	if err != nil {
		t.Fatal(err)
	}
	return v
}

func TestFresh(t *testing.T) {
	got := run(t, Mut__shoutHeap("world"))
	keys := map[string]bool{got: true}
	if got != "hello world" || !keys["hello world"] {
		t.Errorf("the adapter changed the caller's Text: %q", got)
	}
	src := []string{"d", "c", "b", "a", "e"}
	if out := run(t, Mut__sortTake(src)); !slices.Equal(out, []string{"d", "c", "b", "a", "e"}) || !slices.Equal(src, out) {
		t.Errorf("the adapter reordered the source of List.take: %v", src)
	}
	tags := []string{"x", "y"}
	if out := run(t, Mut__shoutRecord("abc", tags)); !slices.Equal(out, []string{"x", "y"}) {
		t.Errorf("the adapter changed a list inside a record: %v", out)
	}
	// last: without the copy this faults (SIGBUS) and ends the process
	if n := run(t, Mut__shoutLiteral); n != 7 {
		t.Errorf("shoutLiteral = %d", n)
	}
}
`
