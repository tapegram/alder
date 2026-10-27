package gen

import (
	"math"
	"testing"
)

// The gen_fuzz golden (a test build, TestGenGolden) builds and its derived generators and
// literal printers run: every property draws, prints one literal per argument and holds; the
// simplest draws print as expected; recursive types stay within the depth budget.
func TestGen_FuzzDeriveCompiles(t *testing.T) {
	files := emitWith(t, "../../testdata/golden/gen_fuzz", true)
	contains(t, files["fuzz__alder.go"], "func fuzz__Todo_Tree(c *fuzz.Choices, depth int) Todo__Tree {",
		"func lit__Todo_Tree(v Todo__Tree) string {", `panic(fuzz.Reject{What: "Slug"})`)
	contains(t, files["app__alder.go"], "Prop: test__Todo__1()")
	runGo(t, files, `package app

import (
	"strings"
	"testing"

	"alder/rt/fuzz"
	rtesting "alder/rt/testing"
)

// draw is p's values for seed, or ok false when a brand's check rejected them.
func draw(p *rtesting.Property, c *fuzz.Choices) (vs []any, ok bool) {
	defer func() {
		if r := recover(); r != nil {
			if _, rej := r.(fuzz.Reject); !rej {
				panic(r)
			}
			ok = false
		}
	}()
	return p.Draw(c), true
}

func depth(t Todo__Tree) int {
	if t.Tag == is__Todo__Tree__Leaf {
		return 0
	}
	return 1 + max(depth(t.Node.Left), depth(t.Node.Right))
}

func dirDepth(d Todo__Dir) int {
	n := 0
	for _, e := range d.Entries {
		n = max(n, 1+dirDepth(e))
	}
	return n
}

func TestFuzz(t *testing.T) {
	props := map[string]*rtesting.Property{}
	for _, c := range Tests {
		if c.Prop != nil {
			props[c.Name] = c.Prop
		} else if c.Name != "plain" {
			t.Errorf("%s has no Prop", c.Name)
		}
	}
	if len(props) != 11 {
		t.Fatalf("%d properties, want 11", len(props))
	}
	maxTree, maxDir := 0, 0
	for name, p := range props {
		if (p.Pure == nil) == (p.Task == nil) {
			t.Errorf("%s: want exactly one of Pure and Task", name)
		}
		drawn := 0
		for seed := range uint64(300) {
			vs, ok := draw(p, fuzz.Fresh(seed))
			if !ok {
				continue
			}
			drawn++
			lits := p.Lit(vs)
			if len(vs) != len(p.Args) || len(lits) != len(p.Args) {
				t.Fatalf("%s: %d values, %d literals, %d args", name, len(vs), len(lits), len(p.Args))
			}
			if p.Pure != nil && !p.Pure(vs) {
				t.Errorf("%s does not hold for %v", name, lits)
			}
			if p.Task != nil {
				p.Task(vs)
			}
			for _, v := range vs {
				switch x := v.(type) {
				case Todo__Tree:
					maxTree = max(maxTree, depth(x))
				case Todo__Dir:
					maxDir = max(maxDir, dirDepth(x))
				}
			}
		}
		if want := map[bool]int{true: 10, false: 300}[name == "slugs are not empty" || name == "explicit task" || name == "the same types from another module"]; drawn < want {
			t.Errorf("%s: only %d of 300 draws accepted", name, drawn)
		}
	}
	if maxTree < 2 || maxTree > fuzz.Depth || maxDir < 1 || maxDir > fuzz.Depth {
		t.Errorf("deepest Tree %d, Dir %d; want within 1..%d", maxTree, maxDir, fuzz.Depth)
	}

	simplest := map[string]string{
		"titles are not blank": "{ title: Text.NonEmpty \"a\", due: Nothing, tags: [] }",
		"trees have a size":    "Leaf",
		"reverse twice":        "[]",
		"everything else": "Dot | { item: Ok (Time.fromMillis 0) } | Dict.empty | () | Text.Email \"a@a\" | " +
			"{ name: \"\", entries: [] } | 0.0 | False",
		"positive in a task":                  "Int.Positive 1",
		"explicit":                            "[0]",
		"explicit with any":                   "[]",
		"the same types from another module": "{ title: Text.NonEmpty \"a\", due: Nothing, tags: [] } | [] | Nothing",
		"rows and generic unions":            "{ id: NoteId \"\", body: \"\" } | NoteId \"\" | Neither | Neither",
	}
	for name, want := range simplest {
		p := props[name]
		vs, ok := draw(p, fuzz.Replay(make([]uint64, 256)))
		if !ok {
			t.Errorf("%s: the simplest draw was rejected", name)
			continue
		}
		if got := strings.Join(p.Lit(vs), " | "); got != want {
			t.Errorf("%s: simplest %s, want %s", name, got, want)
		}
	}
	// the simplest Text is "", which Slug's check rejects
	if _, ok := draw(props["slugs are not empty"], fuzz.Replay(make([]uint64, 256))); ok {
		t.Error("the simplest Slug was not rejected")
	}
	// a Slug that passes prints with its constructor
	for seed := range uint64(1000) {
		p := props["slugs are not empty"]
		if vs, ok := draw(p, fuzz.Fresh(seed)); ok {
			if l := p.Lit(vs)[0]; !strings.HasPrefix(l, "Slug \"") {
				t.Errorf("a Slug prints as %s", l)
			}
			break
		}
	}

	args := map[string][]rtesting.Arg{
		"reverse twice":   {{Name: "xs", Type: "List Int", Defaulted: true}},
		"everything else": {{Name: "s", Type: "Shape"}, {Name: "b", Type: "Box (Result Text Time)"}, {Name: "d", Type: "Dict Text Bytes"}, {Name: "u", Type: "Unit"}, {Name: "e", Type: "Text.Email"}, {Name: "w", Type: "Dir"}, {Name: "f", Type: "Float"}, {Name: "ok", Type: "Bool"}},
		"explicit":        {{Name: "ns", Type: "List Int"}},
		"explicit task":   {{Name: "n", Type: "Int"}},
		"the same types from another module": {{Name: "t", Type: "Todo"}, {Name: "ss", Type: "List Todo.Slug"}, {Name: "k", Type: "Maybe Text.Email"}},
		"rows and generic unions":            {{Name: "n", Type: "Note"}, {Name: "i", Type: "NoteId"}, {Name: "p", Type: "Pick Text"}, {Name: "q", Type: "Pick Int"}},
	}
	// every constructor of a generic union is drawn and printed
	pick := map[string]bool{}
	for seed := range uint64(300) {
		p := props["rows and generic unions"]
		vs, _ := draw(p, fuzz.Fresh(seed))
		l := p.Lit(vs)[3]
		pick[strings.Fields(l)[0]] = true
		if strings.HasPrefix(l, "Two") && !strings.HasPrefix(l, "Two { first: ") {
			t.Errorf("Pick Int prints as %s", l)
		}
	}
	if len(pick) != 3 {
		t.Errorf("Pick Int constructors drawn: %v", pick)
	}
	for name, want := range args {
		got := props[name].Args
		if len(got) != len(want) {
			t.Errorf("%s: args %v, want %v", name, got, want)
			continue
		}
		for i := range want {
			if got[i] != want[i] {
				t.Errorf("%s: arg %d is %+v, want %+v", name, i, got[i], want[i])
			}
		}
	}

	// slugs (a top-level Fuzz.any) draws Slugs
	if _, ok := func() (s string, ok bool) {
		defer func() {
			if recover() != nil {
				ok = false
			}
		}()
		return Todo__slugs(fuzz.Replay([]uint64{0, 1, 0, 0})), true
	}(); !ok {
		t.Error("slugs rejected \"a\"")
	}
}
`)
}

// Review Focus 2: a property over a record with a Text.Email field compiles, and its derived
// generator never rejects (it draws emails directly, not by filtering Text). Stdlib brands
// widen to their base, so the property reads the field as Text (stdlib/Text.alder has no
// accessor). Each draw is one attempt, so each gets its own choices, as the runner gives it.
func TestGen_FuzzDeriveEmailRecord(t *testing.T) {
	files := emitSourceWith(t, map[string]string{"Users.alder": `module Users

type User = { name: Text, email: Text.Email }

test "emails are not empty" = \(u : User) -> Text.length u.email > 0
`}, true)
	runGo(t, files, `package app

import (
	"testing"

	"alder/rt/fuzz"
	"alder/rt/web"
)

func TestEmails(t *testing.T) {
	p := Tests[0].Prop
	for seed := range uint64(100) {
		vs := p.Draw(fuzz.Fresh(seed + 1)) // a Reject panics the test
		u := vs[0].(Users__User)
		if !web.Email(u.Email) || !p.Pure(vs) {
			t.Fatalf("drew %+v", u)
		}
	}
}
`)
}

// A negative zero Float literal is negative zero when the program runs (the parser keeps its
// sign; Go would fold the constant -0.0 to 0), so FloatLit's "-0.0" reads back exactly.
func TestGen_NegativeZeroLiteral(t *testing.T) {
	if got := floatLit(math.Copysign(0, -1)); got != "math.Copysign(0, -1)" {
		t.Errorf("floatLit(-0) = %s", got)
	}
	files := emitSource(t, map[string]string{"Z.alder": "module Z\n\nz : Float\nz = -0.0\n\nm : Float -> Int\nm f = match f {\n  -0.0 -> 1,\n  _ -> 2,\n}\n"})
	runGo(t, files, `package app

import (
	"math"
	"testing"
)

func TestZero(t *testing.T) {
	if !math.Signbit(Z__z) || Z__m(0) != 1 {
		t.Fatalf("z = %v, m 0 = %d", Z__z, Z__m(0))
	}
}
`)
}
