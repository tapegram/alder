package gen

import (
	"testing"
)

// pasteApp is the app of the paste-ready case tests: properties of every body form in Todo,
// and in Report and Sheet properties over other modules' constructors and brands.
var pasteApp = map[string]string{
	"Todo.alder": `module Todo

type Tag = brand Text

type Tree = Leaf | Node { left: Tree, value: Int, right: Tree }

size : Tree -> Int
size t = match t {
  Leaf -> 0,
  Node n -> size n.left + 1 + size n.right,
}

positive : Int -> Bool
positive n = n > 0

test "reverse twice" = \xs -> List.reverse (List.reverse xs) == xs

test "sorted" = Fuzz.forAll (Fuzz.list Fuzz.int) (\xs -> List.reverse xs == xs)

test "piped" = (\n -> n + 1 > n) |> Fuzz.forAll Fuzz.int

test "named" = Fuzz.forAll Fuzz.int positive

test "block" = \(n : Int) -> {
  let m = n + 1
  Task.succeed (m > n)
}

test "shadow" = \x -> (\x -> x) 1 == x

test "let shadow" = \(n : Int) -> {
  let n = 2
  Task.succeed (n > 1)
}

test "places" = \(m : Maybe Int) (t : Tree) -> Maybe.withDefault 0 m >= 0 && size t >= 0 && m == m

test "subject" = \(m : Maybe Int) -> match m {
  Just k -> k >= 0 || True,
  Nothing -> True,
}

test "interpolated" = \(s : Text) -> "a{s}" != ""

test "tags" = \(t : Tag) (tr : Tree) -> Text.length t >= 0

test "functions" = Fuzz.forAll (Fuzz.constant (\(x : Int) -> x)) (\f -> f 1 == 2)
`,
	"Data/Colors.alder": `module Data.Colors

type Color = Red | Green

type Hex = brand Text
`,
	"Pen.alder": `module Pen

import Data.Colors

type Pen = { color: Colors.Color }
`,
	"Report.alder": `module Report

import Data.Colors
import Todo

test "cross" = \(c : Colors.Color) (h : Colors.Hex) (t : Todo.Tag) (tr : Todo.Tree) -> c == c
`,
	"Sheet.alder": `module Sheet

import Pen

test "not imported" = \(p : Pen) -> p == p
`,
}

// The paste-ready case (spec §6): the property's body with the lambda (or Fuzz.forAll and its
// generator) removed and each argument replaced by its literal, parenthesised only where an
// application needs it; a shadowed name is left alone; an argument inside a string literal
// gets no case. Constructors and brands are written as the test's module writes them: its own
// bare, an imported module's through its canonical qualifier, a module it does not import
// through the qualifier an import of it would have.
func TestGen_PasteCase(t *testing.T) {
	files := emitSourceWith(t, pasteApp, true)
	runGo(t, files, `package app

import (
	"regexp"
	"strings"
	"testing"

	"alder/rt/fuzz"
	rtesting "alder/rt/testing"
)

func prop(t *testing.T, name string) *rtesting.Property {
	for _, c := range Tests {
		if c.Name == name {
			return c.Prop
		}
	}
	t.Fatalf("no test %q", name)
	return nil
}

func TestPasteCase(t *testing.T) {
	for _, c := range []struct {
		name string
		lits []string
		want string
	}{
		{"reverse twice", []string{"[0, -1]"}, "test \"reverse twice (case)\" = List.reverse (List.reverse [0, -1]) == [0, -1]"},
		{"sorted", []string{"[1, 0]"}, "test \"sorted (case)\" = List.reverse [1, 0] == [1, 0]"},
		{"piped", []string{"-1"}, "test \"piped (case)\" = -1 + 1 > -1"},
		{"named", []string{"-1"}, "test \"named (case)\" = positive (-1)"},
		{"block", []string{"5"}, "test \"block (case)\" = {\n  let m = 5 + 1\n  Task.succeed (m > 5)\n}"},
		{"shadow", []string{"5"}, "test \"shadow (case)\" = (\\x -> x) 1 == 5"},
		{"let shadow", []string{"5"}, "test \"let shadow (case)\" = {\n  let n = 2\n  Task.succeed (n > 1)\n}"},
		{"places", []string{"Just (-1)", "Leaf"}, "test \"places (case)\" = Maybe.withDefault 0 (Just (-1)) >= 0 && size Leaf >= 0 && Just (-1) == Just (-1)"},
		{"subject", []string{"Just 3"}, "test \"subject (case)\" = match Just 3 {\n  Just k -> k >= 0 || True,\n  Nothing -> True,\n}"},
	} {
		p := prop(t, c.name)
		if p.Case == nil {
			t.Errorf("%s: no Case", c.name)
			continue
		}
		if got := p.Case(c.lits); got != c.want {
			t.Errorf("%s:\n got %s\nwant %s", c.name, got, c.want)
		}
	}
	if prop(t, "interpolated").Case != nil {
		t.Error("interpolated: a Case")
	}
	// a forAll value with no printer is shown with rt.Show, which is no Alder: no case
	if prop(t, "functions").Case != nil {
		t.Error("functions: a Case")
	}
	for _, r := range rtesting.Run(Tests, rtesting.Options{Run: regexp.MustCompile("functions"), Seed: 1, SeedSet: true}) {
		var b strings.Builder
		rtesting.WriteResult(&b, r, false)
		if r.OK || strings.Contains(b.String(), "to keep this case") || !strings.Contains(b.String(), "  paste the values above into a plain test\n") {
			t.Errorf("functions report:\n%s", b.String())
		}
	}
	zeros := func() *fuzz.Choices { return fuzz.Replay(make([]uint64, 64)) }
	for _, c := range []struct {
		name  string
		lits  string
		types string
	}{
		{"tags", "Tag \"\" | Leaf", "Tag | Tree"},
		{"cross", "Colors.Red | Colors.Hex \"\" | Todo.Tag \"\" | Todo.Leaf", "Colors.Color | Colors.Hex | Todo.Tag | Todo.Tree"},
		{"not imported", "{ color: Colors.Red }", "Pen"},
	} {
		p := prop(t, c.name)
		lits := p.Lit(p.Draw(zeros()))
		var types []string
		for _, a := range p.Args {
			types = append(types, a.Type)
		}
		if got := strings.Join(lits, " | "); got != c.lits {
			t.Errorf("%s: literals %s, want %s", c.name, got, c.lits)
		}
		if got := strings.Join(types, " | "); got != c.types {
			t.Errorf("%s: types %s, want %s", c.name, got, c.types)
		}
	}
	if got := prop(t, "cross").Case([]string{"Colors.Red", "Colors.Hex \"\"", "Todo.Tag \"\"", "Todo.Leaf"}); got != "test \"cross (case)\" = Colors.Red == Colors.Red" {
		t.Errorf("cross: %s", got)
	}
}
`)
}
