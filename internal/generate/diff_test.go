package generate

import "testing"

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
