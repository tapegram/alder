package generate

import (
	"strings"
	"testing"

	"alder/internal/syntax"
)

// The splice of a fill (security review of #46): an expression placed between the text before
// and after a hole on its line must not change that text's meaning.

// calcApp is fillApp with a module Calc whose holes sit inside operators.
func calcApp(t *testing.T) *Program {
	return fillApp(t, map[string]string{"app/Calc.alder": "module Calc\n\nf : Int -> Int\nf x = x\n\nbump : Int -> Int\nbump n = ?a + 1 * n\n\ntwice : Int\ntwice = ?b * 2\n"})
}

// fillOutside fills address's hole with expr, checks that the bytes before and after the hole
// are unchanged in the edit and that no comment was spliced in, and returns the dry run's text of
// the file (formatted).
func fillOutside(t *testing.T, p *Program, address, expr string) string {
	t.Helper()
	h, err := FindHole(p, address, "")
	if err != nil {
		t.Fatal(err)
	}
	src := p.Modules[h.Module].Source
	off := h.Span.Start.Col - 1
	for _, l := range strings.SplitAfter(src, "\n")[:h.Span.Start.Line-1] {
		off += len(l)
	}
	edits, _, _, err := Fill(p, address, "", expr)
	if err != nil {
		t.Fatalf("%s %q: %v", address, expr, err)
	}
	body := strings.TrimRight(edits[0].Body, "\n")
	pre, post := src[:off], strings.TrimRight(src[off+len("?"+h.Name):], "\n")
	if !strings.HasPrefix(body, pre) || !strings.HasSuffix(body, post) || len(body) < len(pre)+len(post) {
		t.Fatalf("%s %q changed text outside the hole:\n%s", address, expr, body)
	}
	if inserted := body[len(pre) : len(body)-len(post)]; syntax.StripComments(inserted) != strings.TrimRight(inserted, " \t\n") {
		t.Errorf("%s %q: a comment was spliced in: %q", address, expr, inserted)
	}
	res, err := Apply(p.Dir, edits, Options{DryRun: true})
	if err != nil {
		t.Fatalf("%s %q: %v", address, expr, err)
	}
	for _, f := range res.Files {
		if f.Path == h.File {
			return *f.After
		}
	}
	t.Fatalf("%s not changed", h.File)
	return ""
}

// TestFill_CommentCannotSwallowTheLine: a trailing `--` comment is cut, so it never comments
// out the rest of the hole's line.
func TestFill_CommentCannotSwallowTheLine(t *testing.T) {
	if got := fillOutside(t, calcApp(t), "Calc.bump", "n -- "); !strings.Contains(got, "bump n = n + 1 * n\n") {
		t.Errorf("Calc:\n%s", got)
	}
	if got := fillOutside(t, calcApp(t), "Calc.bump", "n --+ 0"); !strings.Contains(got, "bump n = n + 1 * n\n") {
		t.Errorf("Calc:\n%s", got)
	}
}

// TestFill_OperatorKeepsItsPrecedence: an expression with an operator is parenthesised, so it
// binds as the hole did; the formatter drops parentheses that are not needed.
func TestFill_OperatorKeepsItsPrecedence(t *testing.T) {
	if got := fillOutside(t, calcApp(t), "Calc.twice", "1 + 1"); !strings.Contains(got, "twice = (1 + 1) * 2\n") {
		t.Errorf("Calc:\n%s", got)
	}
	if got := fillOutside(t, calcApp(t), "Calc.twice", "f 3 -- three"); !strings.Contains(got, "twice = f 3 * 2\n") {
		t.Errorf("Calc:\n%s", got)
	}
	if got := fillOutside(t, calcApp(t), "Calc.bump", "if n > 0 then n else 0"); !strings.Contains(got, "bump n = (if n > 0 then n else 0) + 1 * n\n") {
		t.Errorf("Calc:\n%s", got)
	}
}

// TestFill_MultiLineWithComments: a multi-line answer's comments are cut and its lines kept; an
// answer of comments only is refused.
func TestFill_MultiLineWithComments(t *testing.T) {
	expr := "{\n  -- first, nothing\n  let x <- Task.succeed Rerender -- the answer\n  -- then\n  Task.succeed x\n} -- done"
	got := fillOutside(t, fillApp(t, nil), "Home.update.Submit", expr)
	if strings.Contains(got, "first, nothing") || strings.Contains(got, "the answer") || !strings.Contains(got, "let x <- Task.succeed Rerender\n") || !strings.Contains(got, "Task.succeed x\n") {
		t.Errorf("Home:\n%s", got)
	}
	if _, _, _, err := Fill(fillApp(t, nil), "Home.update.Submit", "", "-- only a comment\n"); err == nil {
		t.Error("an expression of comments only was accepted")
	}
}

// TestFill_StringWithDashesKept: `--` inside a string is not a comment.
func TestFill_StringWithDashesKept(t *testing.T) {
	got := fillOutside(t, fillApp(t, nil), "Home.update.Submit", `Task.succeed (Reject "a -- b") -- why`)
	if !strings.Contains(got, `Reject "a -- b"`) || strings.Contains(got, "why") {
		t.Errorf("Home:\n%s", got)
	}
}
