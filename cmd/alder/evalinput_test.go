package main

import (
	"errors"
	"fmt"
	"reflect"
	"strings"
	"testing"
)

func TestParseItems(t *testing.T) {
	t.Parallel()
	type it struct {
		Let    bool
		Name   string
		Lines  []string
		InLine []int
		Shift  []int
	}
	cases := []struct {
		name, in string
		want     []it
		err      string // a substring of the error; "" for none
		nothing  bool   // the error is errNothing
	}{
		{name: "one expression", in: "1 + 2", want: []it{
			{Lines: []string{"it = 1 + 2"}, InLine: []int{1}, Shift: []int{-5}},
		}},
		{name: "two lets and an expression", in: "let a = 1\nlet b = a + 1\nb * 10\n", want: []it{
			{Let: true, Name: "a", Lines: []string{"a = 1"}, InLine: []int{1}, Shift: []int{4}},
			{Let: true, Name: "b", Lines: []string{"b = a + 1"}, InLine: []int{2}, Shift: []int{4}},
			{Lines: []string{"it = b * 10"}, InLine: []int{3}, Shift: []int{-5}},
		}},
		{name: "annotated let", in: "let x : Text = 1\nx", want: []it{
			{Let: true, Name: "x", Lines: []string{"x : Text", "x = 1"}, InLine: []int{1, 1}, Shift: []int{4, 11}},
			{Lines: []string{"it = x"}, InLine: []int{2}, Shift: []int{-5}},
		}},
		{name: "indented first line", in: "   1 + 2", want: []it{
			{Lines: []string{"it = 1 + 2"}, InLine: []int{1}, Shift: []int{-2}},
		}},
		{name: "indented let and continuation", in: "  let a = 1\nlet b =\n    a + 1\nb", want: []it{
			{Let: true, Name: "a", Lines: []string{"a = 1"}, InLine: []int{1}, Shift: []int{6}},
			{Let: true, Name: "b", Lines: []string{"b =", "    a + 1"}, InLine: []int{2, 3}, Shift: []int{4, 0}},
			{Lines: []string{"it = b"}, InLine: []int{4}, Shift: []int{-5}},
		}},
		{name: "column-1 closer continues", in: "let r = {\n  a: 1,\n}\nr.a\n", want: []it{
			{Let: true, Name: "r", Lines: []string{"r = {", "  a: 1,", "}"}, InLine: []int{1, 2, 3}, Shift: []int{4, 0, 0}},
			{Lines: []string{"it = r.a"}, InLine: []int{4}, Shift: []int{-5}},
		}},
		{name: "leading comment goes with the first item", in: "-- first\nlet x = 1\nx", want: []it{
			{Let: true, Name: "x", Lines: []string{"-- first", "x = 1"}, InLine: []int{1, 2}, Shift: []int{0, 4}},
			{Lines: []string{"it = x"}, InLine: []int{3}, Shift: []int{-5}},
		}},
		{name: "let it", in: "let it = 1\nit", err: "input line 1: a let may not be named `it`, the name eval gives the expression"},
		// an uppercase name is a module's or a type's, which would shadow the let
		{name: "let Upper", in: "let Todo = 1\nTodo", err: "input line 1: a let name starts with a lowercase letter or `_` (`let todo = …`)"},
		// a value name may start with `_`
		{name: "let underscore", in: "let _x = 1\n_x", want: []it{
			{Let: true, Name: "_x", Lines: []string{"_x = 1"}, InLine: []int{1}, Shift: []int{4}},
			{Lines: []string{"it = _x"}, InLine: []int{2}, Shift: []int{-5}},
		}},
		// `'` is no identifier character: the name ends before it (the parser then refuses the rest)
		{name: "let prime", in: "let x' = 1\n1", want: []it{
			{Let: true, Name: "x", Lines: []string{"x' = 1"}, InLine: []int{1}, Shift: []int{4}},
			{Lines: []string{"it = 1"}, InLine: []int{2}, Shift: []int{-5}},
		}},
		{name: "let non-ASCII", in: "let ünï = 1\n1", err: "a let name starts with a lowercase letter"},
		{name: "let digit", in: "let x = 1\nlet 3 = 4\n1", err: "input line 2: a let name starts with a lowercase letter"},
		{name: "empty", in: "\n  \n-- c\n", nothing: true},
		{name: "lets alone", in: "let a = 1\nlet b = 2\n", nothing: true, want: []it{
			{Let: true, Name: "a", Lines: []string{"a = 1"}, InLine: []int{1}, Shift: []int{4}},
			{Let: true, Name: "b", Lines: []string{"b = 2"}, InLine: []int{2}, Shift: []int{4}},
		}},
		{name: "non-let before the expression", in: "1\n2", err: "input line 1: only `let name = expr` lines may come before the expression"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			items, err := parseItems(c.in, "eval")
			switch {
			case c.nothing:
				if !errors.Is(err, errNothing) {
					t.Fatalf("err %v, want errNothing", err)
				}
			case c.err != "":
				if err == nil || !strings.Contains(err.Error(), c.err) {
					t.Fatalf("err %v, want %q", err, c.err)
				}
				return
			case err != nil:
				t.Fatal(err)
			}
			var got []it
			for _, x := range items {
				got = append(got, it{x.Let, x.Name, x.Lines, x.InLine, x.Shift})
			}
			if !reflect.DeepEqual(got, c.want) {
				t.Errorf("got  %+v\nwant %+v", got, c.want)
			}
		})
	}
}

// assemble of lets alone is a module with no `it`.
func TestAssembleLetsOnly(t *testing.T) {
	t.Parallel()
	items, err := parseItems("let a = 1\nlet b : Int = a", "eval")
	if !errors.Is(err, errNothing) {
		t.Fatal(err)
	}
	es := assemble(items)
	if es.src != "a = 1\nb : Int\nb = a\n" || !reflect.DeepEqual(es.inLine, []int{1, 2, 2}) {
		t.Errorf("%q %v", es.src, es.inLine)
	}
}

// assemble(parseItems(x)) is the module evalModule builds for x, its position map included,
// whenever x ends in an expression. (Before evalModule was rewritten on parseItems and
// assemble, this test held the old evalModule verbatim and compared the two on every input
// below, errors included: byte-identical.)
func TestAssembleRoundTrip(t *testing.T) {
	t.Parallel()
	for _, in := range []string{
		"1 + 2",
		"let xs = [1, 2]\nList.length xs\n",
		"let a = 1\nlet b = a + \"x\"\nb\n",
		"   1 + 2",
		"  let a = 1\nlet b =\n    a + 1\nb * 10",
		"let x : Int = 1\nx + 1",
		"let none : List Int = []\nnone",
		"let x : Text = 1\nx",
		"let a = 1 + \"x\"\nlet b = a\na + 1",
		"let f = \\n -> g n\nlet g = \\n -> n\n1",
		"let v = v + 1\nv",
		"it + 1",
		"match Just 1 {\n  Just v -> v,\n  Nothing -> 0,\n}\n",
		"<ul>\n  <li>a</li>\n</ul>\n",
		"let r = {\n  a: 1,\n}\nr.a\n",
		"-- first\nlet x = 1\nlet x = 2\nx",
		"\n\n-- c\n\n  let a = 1\n\n-- mid\nlet b = 2\n\na + b\n\n-- end\n",
		"let fact n = if n <= 1 then 1 else n * fact (n - 1)\nfact 5",
		"let x\t:\tInt = 1\r\nx\r\n",
		"let it = 1\nit",
		"1\n2",
		"let x = 1",
		"let it = 1",
		"let x : Int\nx",
		"",
	} {
		old, oldErr := evalModule(in)
		items, err := parseItems(in, "eval")
		if oldErr != nil { // parseItems fails alike
			if fmt.Sprint(err) != fmt.Sprint(oldErr) {
				t.Errorf("%q: err %v, evalModule's %v", in, err, oldErr)
			}
			continue
		}
		if err != nil { // the input ends in a let: eval's `it = let …`, the REPL's lets alone
			continue
		}
		es := assemble(items)
		es.lines = inputLines(in)
		if es.src != old.src || !reflect.DeepEqual(es.inLine, old.inLine) || !reflect.DeepEqual(es.shift, old.shift) || !reflect.DeepEqual(es.lines, old.lines) {
			t.Errorf("%q:\n got %q %v %v\nwant %q %v %v", in, es.src, es.inLine, es.shift, old.src, old.inLine, old.shift)
		}
	}
	// and what the two agree on, for a few inputs: the module and its column shifts
	for in, want := range map[string]struct {
		src   string
		shift []int
	}{
		"1 + 2":                          {"it = 1 + 2\n", []int{-5}},
		"   1 + 2":                       {"it = 1 + 2\n", []int{-2}},
		"let x : Int = 1\nx + 1":         {"x : Int\nx = 1\nit = x + 1\n", []int{4, 10, -5}},
		"let r = {\n  a: 1,\n}\nr.a\n":   {"r = {\n  a: 1,\n}\nit = r.a\n", []int{4, 0, 0, -5}},
		"-- c\nlet a = 1\n\na\n-- end\n": {"-- c\na = 1\n\nit = a\n-- end\n", []int{0, 4, 0, -5, 0}},
	} {
		items, err := parseItems(in, "eval")
		if err != nil {
			t.Fatalf("%q: %v", in, err)
		}
		if es := assemble(items); es.src != want.src || !reflect.DeepEqual(es.shift, want.shift) {
			t.Errorf("%q: got %q %v, want %q %v", in, es.src, es.shift, want.src, want.shift)
		}
	}
}
