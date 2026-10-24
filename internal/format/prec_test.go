package format

import (
	"strings"
	"testing"

	"alder/internal/syntax"
)

var binaryOps = []string{"|>", "??", "||", "&&", "==", "!=", "<", "<=", ">", ">=", "+", "-", "++", "*", "/", "%"}

// parsesAs reports whether body, as a value's body, parses without errors to the same program
// as want's.
func parsesAs(body, want string) bool {
	src := func(b string) []byte { return []byte("module M\n\nv : Int\nv = " + b + "\n") }
	m1, d1 := syntax.ParseFile("m.alder", src(body))
	m2, _ := syntax.ParseFile("m.alder", src(want))
	return len(d1) == 0 && syntax.Dump(m1) == syntax.Dump(m2)
}

// bodyOf is the body of the value v in a formatted module, on one line.
func bodyOf(out []byte) string {
	_, body, _ := strings.Cut(string(out), "v =")
	return strings.Join(strings.Fields(body), " ")
}

// For every pair of operators, the formatter keeps the parentheses in `(x a y) b z` and
// `x a (y b z)` exactly when removing them changes the program.
func TestPrec_RoundTrip(t *testing.T) {
	for _, a := range binaryOps {
		for _, b := range binaryOps {
			for _, in := range []string{"(x " + a + " y) " + b + " z", "x " + a + " (y " + b + " z)"} {
				bare := strings.NewReplacer("(", "", ")", "").Replace(in)
				want := in
				if parsesAs(bare, in) {
					want = bare
				}
				src := "module M\n\nv : Int\nv = " + in + "\n"
				out, diags, err := Source("m.alder", []byte(src))
				if err != nil || len(diags) > 0 {
					t.Errorf("%s: err=%v diags=%v", in, err, diags)
					continue
				}
				if got := bodyOf(out); got != want {
					t.Errorf("%s: got %q, want %q", in, got, want)
				}
			}
		}
	}
}

func TestPrec_Levels(t *testing.T) {
	order := [][]string{{"|>"}, {"??"}, {"||"}, {"&&"}, {"==", "!=", "<", "<=", ">", ">="}, {"+", "-", "++"}, {"*", "/", "%"}}
	for i, ops := range order {
		for _, op := range ops {
			l, _ := prec(op)
			if l != levelPipe+i {
				t.Errorf("%s: level %d, want %d", op, l, levelPipe+i)
			}
		}
	}
	if l, a := prec("??"); a != assocRight || l != levelDefault {
		t.Errorf("?? is (%d, %v), want right-associative", l, a)
	}
	if _, a := prec("<"); a != assocNone {
		t.Errorf("< associativity %v, want none", a)
	}
}

// Parentheses the grammar needs around non-operator expressions are kept; redundant ones go.
func TestExpr_Parens(t *testing.T) {
	cases := map[string]string{
		"((1 + 2))":                "1 + 2",
		"f (g x)":                  "f (g x)",
		"(f x) y":                  "(f x) y",
		"f (x.y)":                  "f x.y",
		"(f x).y":                  "(f x).y",
		"f (-1)":                   "f (-1)",
		"(-1) + 2":                 "-1 + 2",
		"f (\\x -> x) y":           "f (\\x -> x) y",
		"(\\x -> x) |> f":          "(\\x -> x) |> f",
		"(if a then b else c) + 1": "(if a then b else c) + 1",
		"x |> (\\y -> y)":          "x |> (\\y -> y)",
		"(a ?? b) ?? c":            "(a ?? b) ?? c",
		"a ?? (b ?? c)":            "a ?? b ?? c",
		"a - (b - c)":              "a - (b - c)",
		"(a - b) - c":              "a - b - c",
		"f { a: 1 } (g x)":         "f { a: 1 } (g x)",
		"{ (x) }":                  "{ x }",
		"f ({ x })":                "f { x }",
		"f (Just)":                 "f Just",
		"(Foo).x":                  "(Foo).x",
		"x == (y == z)":            "x == (y == z)",
	}
	for in, want := range cases {
		src := "module M\n\nv : Int\nv = " + in + "\n"
		out, diags, err := Source("m.alder", []byte(src))
		if err != nil || len(diags) > 0 {
			t.Errorf("%s: err=%v diags=%v", in, err, diags)
			continue
		}
		if got := bodyOf(out); got != want {
			t.Errorf("%s: got %q, want %q", in, got, want)
		}
	}
}

// In a match subject an argument cannot start with `{`: it would open the arms.
func TestExpr_MatchSubjectBraceArgument(t *testing.T) {
	src := "module M\n\nv : Int\nv = match f ({ a: 1 }) {\n  _ -> 1,\n}\n"
	out, _, err := Source("m.alder", []byte(src))
	if err != nil || string(out) != src {
		t.Fatalf("got %q, %v", out, err)
	}
}

// A block's result that starts with `-` keeps its parentheses: on a line of its own it would
// continue the statement before.
func TestExpr_BlockResultStartingWithMinus(t *testing.T) {
	src := "module M\n\nv : Int\nv = {\n  let x = 1\n  (-1 + x)\n}\n"
	out, _, err := Source("m.alder", []byte(src))
	if err != nil || string(out) != src {
		t.Fatalf("got %q, %v", out, err)
	}
}

// A block holding only a result stays a block: the checker treats some blocks differently from
// their result (a test's body makes an effect test; an `update` that is a match splits its
// arms). It is printed on one line when it fits.
func TestExpr_ResultOnlyBlockKept(t *testing.T) {
	for _, src := range []string{
		"module M\n\ntest \"t\" = { Task.succeed True }\n",
		"module M\n\nupdate : Msg -> Int\nupdate msg = {\n  match msg {\n    _ -> 1,\n  }\n}\n",
	} {
		out, _, err := Source("m.alder", []byte(src))
		if err != nil || string(out) != src {
			t.Errorf("got %q, %v; want %q", out, err, src)
		}
	}
}
