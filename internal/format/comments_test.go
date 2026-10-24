package format

import (
	"slices"
	"testing"

	"alder/internal/syntax"
)

func parse(t *testing.T, src string) *syntax.Module {
	t.Helper()
	m, diags := syntax.ParseFile("m.alder", []byte(src))
	if len(diags) > 0 {
		t.Fatal(diags)
	}
	return m
}

// texts is the text of the comments attached to n in one of the maps.
func texts(cs map[any][]syntax.Comment, n any) []string {
	var out []string
	for _, c := range cs[n] {
		out = append(out, c.Text)
	}
	return out
}

func wantTexts(t *testing.T, what string, got []string, want ...string) {
	t.Helper()
	if !slices.Equal(got, want) {
		t.Errorf("%s = %q, want %q", what, got, want)
	}
}

func TestAttach_Classifies(t *testing.T) {
	m := parse(t, `-- header
module M -- module line

-- leads x
x : Int
x = 1 -- trails x

r : { a: Int, b: Int }
r = {
  -- leads a
  a: 1, -- trails a
  b: 2,
  -- dangles in r
}
-- at the end
`)
	a := attach(m)
	x, r := m.Decls[0], m.Decls[1].(*syntax.ValueDecl)
	rec := r.Body.(*syntax.RecordLit)
	wantTexts(t, "leading module", texts(a.leading, m), "-- header")
	wantTexts(t, "trailing module", texts(a.trailing, m), "-- module line")
	wantTexts(t, "leading x", texts(a.leading, x), "-- leads x")
	wantTexts(t, "trailing x", texts(a.trailing, x), "-- trails x")
	wantTexts(t, "leading a", texts(a.leading, rec.Fields[0]), "-- leads a")
	wantTexts(t, "trailing a", texts(a.trailing, rec.Fields[0]), "-- trails a")
	wantTexts(t, "dangling r", texts(a.dangling, rec), "-- dangles in r")
	wantTexts(t, "dangling module", texts(a.dangling, m), "-- at the end")
}

// A comment between an operator and its operand leads the body around it, and the comments
// placed inside that body before it move with it, keeping their order.
func TestAttach_UnplaceableLeadsTheEnclosingBody(t *testing.T) {
	m := parse(t, `module M

ok : Bool
ok =
  check [
    a, -- trails a
  ] && -- between operands
    b
`)
	a := attach(m)
	body := m.Decls[0].(*syntax.ValueDecl).Body
	wantTexts(t, "leading body", texts(a.leading, body), "-- trails a", "-- between operands")
	if len(a.trailing) != 0 {
		t.Errorf("trailing = %v, want none", a.trailing)
	}
}

// Comment lines of markup text: inside a text child they are printed with the text (not
// attached); between other children they lead the next one, or dangle at the end.
func TestAttach_Markup(t *testing.T) {
	m := parse(t, `module M

v : Html Never
v =
  <div>
    -- leads b
    <b>x</b>
    some text
    -- inside the text
    <i>y</i>
    -- dangles
  </div>
`)
	a := attach(m)
	div := m.Decls[0].(*syntax.ValueDecl).Body.(*syntax.Element)
	wantTexts(t, "leading <b>", texts(a.leading, div.Children[0]), "-- leads b")
	wantTexts(t, "dangling <div>", texts(a.dangling, div), "-- dangles")
	n := 0
	for _, cs := range a.leading {
		n += len(cs)
	}
	for _, cs := range a.trailing {
		n += len(cs)
	}
	for _, cs := range a.dangling {
		n += len(cs)
	}
	if n != 2 {
		t.Errorf("%d comments attached, want 2 (the text's own is printed with it)", n)
	}
}

// Comments between a signature and its definition stay there.
func TestAttach_BetweenSignatureAndDefinition(t *testing.T) {
	m := parse(t, "module M\n\nx : Int -- the type\n-- the value\nx =\n  -- the body\n  1\n")
	a := attach(m)
	d := m.Decls[0].(*syntax.ValueDecl)
	wantTexts(t, "trailing sig", texts(a.trailing, d.Sig), "-- the type")
	wantTexts(t, "dangling sig", texts(a.dangling, d.Sig), "-- the value")
	wantTexts(t, "leading body", texts(a.leading, d.Body), "-- the body")
}

// A comment hoisted into an inner item moves again, with it, when a later comment leads the
// outer body: both lead the body, in order.
func TestAttach_HoistAcrossNestedFrames(t *testing.T) {
	m := parse(t, `module M

ok : Bool
ok =
  [
    a && -- inner
      b,
  ] == -- outer
    c
`)
	a := attach(m)
	body := m.Decls[0].(*syntax.ValueDecl).Body
	elem := body.(*syntax.BinOp).L.(*syntax.ListLit).Elems[0]
	wantTexts(t, "leading body", texts(a.leading, body), "-- inner", "-- outer")
	wantTexts(t, "leading element", texts(a.leading, elem))
}

// Comments inside a signature, before its type, lead the declaration.
func TestAttach_InsideSignatureLeadsDeclaration(t *testing.T) {
	m := parse(t, "module M\n\nx :\n  -- c\n  Int\nx = 1\n")
	a := attach(m)
	wantTexts(t, "leading x", texts(a.leading, m.Decls[0]), "-- c")
}

// A region's end marker stays with the item before it.
func TestAttach_RegionEndFollowsItem(t *testing.T) {
	m := parse(t, "module M\n\nq : Int\nq = 1\n-- @admin end\nr : Int\nr = 2\n")
	a := attach(m)
	wantTexts(t, "after q", texts(a.after, m.Decls[0]), "-- @admin end")
	wantTexts(t, "leading r", texts(a.leading, m.Decls[1]))
}
