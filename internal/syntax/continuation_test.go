package syntax

import (
	"strings"
	"testing"
)

// Application never spans lines: an argument on the next line is not one (#29). The formatter
// relies on it to keep arguments on the function's line.
func TestParse_ApplicationDoesNotSpanLines(t *testing.T) {
	for _, src := range []string{
		"module V\nv =\n  f 1\n    2\n",
		"module V\nv = f\n  x\n",
		"module V\nv = {\n  let a = f\n    x\n  a\n}\n",
	} {
		_, diags := ParseString("V.alder", src)
		if len(diags) == 0 || diags[0].Code != "E0001" {
			t.Errorf("%q: diags %v, want E0001", src, diags)
		}
	}
}

// A line-leading `->` continues only a type (parseType): in a match arm or after a lambda's
// parameters it is still E0001 (#29).
func TestParse_ArrowLeadingLineOnlyInTypes(t *testing.T) {
	for _, src := range []string{
		"module V\nv x = match x {\n  A\n    -> 1,\n  _ -> 2,\n}\n",
		"module V\nv = List.map (\\x\n  -> x) xs\n",
	} {
		_, diags := ParseString("V.alder", src)
		if len(diags) == 0 || diags[0].Code != "E0001" || !strings.Contains(diags[0].Message, "expected `->`") {
			t.Errorf("%q: diags %v, want E0001 expecting `->`", src, diags)
		}
	}
	if _, diags := ParseString("V.alder", "module V\nf : Int\n  -> Int\nf x = x\n"); len(diags) > 0 {
		t.Errorf("`->`-leading signature: %v", diags)
	}
}

// peekIsColon looks one token ahead and restores the lexer: a comment it lexes on the way is
// dropped with the lookahead and collected once when the parser reaches it (#29).
func TestParse_PeekIsColonAcrossComment(t *testing.T) {
	p := newParser("m.alder", []byte("send -- why\n  : Int\n"), false, nil)
	if p.peekIsColon() {
		t.Fatal("peekIsColon saw `:` past the end of the line")
	}
	if len(p.lx.Comments()) != 0 {
		t.Fatalf("the lookahead kept its comment: %+v", p.lx.Comments())
	}
	if !p.at(LIdent) || p.tok.Text != "send" {
		t.Fatalf("lookahead moved the parser to %v", p.tok)
	}
	p.advance()
	if !p.at(NL) || len(p.lx.Comments()) != 1 || p.lx.Comments()[0].Text != "-- why" {
		t.Fatalf("after advancing: tok %v, comments %+v", p.tok, p.lx.Comments())
	}
	// on one line the lookahead reaches the `:` before the comment
	p = newParser("m.alder", []byte("send : Int -- why\n"), false, nil)
	if !p.peekIsColon() || len(p.lx.Comments()) != 0 {
		t.Fatalf("peekIsColon on `send :`: comments %+v", p.lx.Comments())
	}
}
