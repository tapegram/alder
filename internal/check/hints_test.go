package check

import (
	"strings"
	"testing"
)

// The precedence-trap hints (alder#10) suggest a rewrite: pasted in place of the expression, it
// must type-check, and every error the trap causes carries the hint.
func TestPrecedenceHintsPaste(t *testing.T) {
	const header = "module M\n\npositive : Int -> Bool\npositive n = n > 0\n\ndouble : Int -> Int\ndouble n = n * 2\n\n"
	cases := []struct{ sig, params, expr, want string }{
		{"Bool -> Maybe Int -> Bool", "ready m", "ready && (m |> Maybe.map positive) ?? True", "ready && ((m |> Maybe.map positive) ?? True)"},
		{"Bool -> Bool -> Maybe Bool -> Bool", "a b m", "a || b && m ?? False", "a || b && (m ?? False)"},
		{"Bool -> Bool -> Maybe Bool -> Bool", "a b m", "a && (b && m) ?? True", "a && (b && (m ?? True))"},
		{"Bool -> Bool -> Maybe Bool -> Bool", "a b m", "a && (b || (m)) ?? True", "a && (b || (m ?? True))"},
		{"Bool -> Maybe Bool -> Bool", "a m", "a && ((m)) ?? True", "a && (m ?? True)"},
		{"Int -> Bool", "x", "x |> double == 4", "(x |> double) == 4"},
		{"Int -> Bool", "x", "x |> double == 4 && True", "(x |> double) == 4 && True"},
		{"Int -> Bool", "x", "x |> (double) == 4", "(x |> double) == 4"},
		{"List Int -> Bool", "xs", "xs |> List.map double |> List.length < 3", "(xs |> List.map double |> List.length) < 3"},
	}
	for _, c := range cases {
		t.Run(c.expr, func(t *testing.T) {
			src := func(e string) string { return header + "f : " + c.sig + "\nf " + c.params + " = " + e + "\n" }
			ds := checkModule(t, src(c.expr))
			if len(ds) == 0 {
				t.Fatal("no errors")
			}
			// a `??` trap's errors (the left side and its last operand) all carry the hint; a
			// pipe's carries it on the `|>` error, not on the comparison's own
			hinted := 0
			for _, d := range ds {
				i := strings.LastIndex(d.Message, "; write `")
				if i < 0 {
					if strings.Contains(c.expr, "??") {
						t.Fatalf("%s has no rewrite: %s", d.Code, d.Message)
					}
					continue
				}
				hinted++
				got := strings.TrimSuffix(d.Message[i+len("; write `"):], "`")
				if got != c.want {
					t.Fatalf("rewrite %q, want %q (%s)", got, c.want, d.Message)
				}
			}
			if hinted == 0 {
				t.Fatalf("no error carries the hint: %v", ds)
			}
			if after := checkModule(t, src(c.want)); len(after) != 0 {
				t.Fatalf("the rewrite does not check: %v", after)
			}
		})
	}
}
