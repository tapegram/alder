package generate

import (
	"reflect"
	"strings"
	"testing"
)

// TestLineEdits_HunksAndImport: an added import and a filled hole are two edits, each a run of
// whole lines; applied to before (from the last edit up) they give after.
func TestLineEdits_HunksAndImport(t *testing.T) {
	before := "module Home\n\nimport Layout\n\nx = 1\n\nupdate =\n  ?done\n"
	after := "module Home\n\nimport Layout\nimport Member\n\nx = 1\n\nupdate =\n  Member.go 1\n"
	got := LineEdits(before, after)
	want := []LineEdit{{Start: 3, End: 3, Text: "import Member\n"}, {Start: 7, End: 8, Text: "  Member.go 1\n"}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("LineEdits = %#v, want %#v", got, want)
	}
	lines := strings.SplitAfter(before, "\n")
	for i := len(got) - 1; i >= 0; i-- {
		e := got[i]
		lines = append(append(append([]string{}, lines[:e.Start]...), e.Text), lines[e.End:]...)
	}
	if s := strings.Join(lines, ""); s != after {
		t.Errorf("applied:\n%s\nwant:\n%s", s, after)
	}
	if got := LineEdits(before, before); len(got) != 0 {
		t.Errorf("equal texts: %#v", got)
	}
	if got := LineEdits("a\nb\n", "a\n"); !reflect.DeepEqual(got, []LineEdit{{Start: 1, End: 2, Text: ""}}) {
		t.Errorf("a removed line: %#v", got)
	}
}

// TestFindHole: the hole an address names, with Fill's refusals.
func TestFindHole(t *testing.T) {
	p := fillApp(t, nil)
	h, err := FindHole(p, "Home.update.Submit", "")
	if err != nil || h.Name != "done" || h.Type != "Task Nav" {
		t.Fatalf("FindHole = %+v, %v", h, err)
	}
	if _, err := FindHole(p, "Home.update.Submit", "?nope"); err == nil || !strings.Contains(err.Error(), "has no hole `?nope`") {
		t.Errorf("a hole it does not have: %v", err)
	}
	if _, err := FindHole(p, "Home.view", ""); err == nil || !strings.Contains(err.Error(), "has no typed hole") {
		t.Errorf("a declaration without a hole: %v", err)
	}
}

// TestUnifiedDiff: hunks with three lines of context, deletions before insertions, ranges as
// `diff -u` writes them.
func TestUnifiedDiff(t *testing.T) {
	a := "1\n2\n3\n4\n5\n6\n7\n8\n9\n10\n11\n12\n"
	b := "1\n2\nthree\n4\n5\n6\n7\n8\n9\n10\n11\n12\n13\n"
	want := "--- a/app/X.alder\n+++ b/app/X.alder\n" +
		"@@ -1,6 +1,6 @@\n 1\n 2\n-3\n+three\n 4\n 5\n 6\n" +
		"@@ -10,3 +10,4 @@\n 10\n 11\n 12\n+13\n"
	if got := UnifiedDiff("app/X.alder", a, b); got != want {
		t.Errorf("diff:\n%s\nwant:\n%s", got, want)
	}
	if got := UnifiedDiff("x", a, a); got != "" {
		t.Errorf("equal texts: %q", got)
	}
	if got := UnifiedDiff("x", "", "a\n"); got != "--- a/x\n+++ b/x\n@@ -0,0 +1 @@\n+a\n" {
		t.Errorf("from empty: %q", got)
	}
}
