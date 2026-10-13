package generate

import "testing"

// TestNeedsParens: a filled hole is parenthesised only where the expression would otherwise
// read differently.
func TestNeedsParens(t *testing.T) {
	for _, c := range []struct {
		before, after, expr string
		want                bool
	}{
		{"  Clear p -> ", ",", "Task.succeed Rerender", false},
		{"  let x <- ", "", "Db.from Post |> Db.all", false},
		{"  Task.succeed (r.answer == ", ")", "Redirected (Home {})", false},
		{"  Task.succeed (r.answer == ", ")", "a || b", true},
		{"  Task.succeed (r.answer == ", ")", "if a then b else c", true},
		{"  f ", " y", "g x", true},
		{"  f ", " y", "(g x)", false},
		{"  f ", " y", "Rerendered", false},
		{"  f ", " y", `"a b"`, false},
		{"  f ", " y", "{ a: 1 }", false},
		{"  f ", " y", "(a) (b)", true},
		{"  { id: ", " }", "Post.PostId \"x\"", false},
		{"x = ", "", "{\n  let a <- b\n  c\n}", false},
	} {
		if got := needsParens(c.before, c.after, c.expr); got != c.want {
			t.Errorf("needsParens(%q, %q, %q) = %v, want %v", c.before, c.after, c.expr, got, c.want)
		}
	}
}
