package semtok

import (
	"strings"
	"testing"

	"alder/internal/diag"
)

// TestScratch: the `in` header, each watch's `>`, the watches' expressions where the file has
// them (however many spaces follow `>`), and nothing of the generated `itK =` (Review Focus 4).
func TestScratch(t *testing.T) {
	src := "in Posts.Show\n\n> 1 + 1\n>    model\n|> f\nsq x = x * x\n"
	toks := Scratch("scratch.alder", src)
	checkInvariants(t, "scratch", src, toks)
	want(t, src, toks, "in Posts.Show", 0, TKeyword, 0)
	want(t, src, toks, "in Posts.Show", 3, TNamespace, 0)
	want(t, src, toks, "in Posts.Show", 9, TNamespace, 0)
	want(t, src, toks, "> 1 + 1", 0, TOperator, 0)
	want(t, src, toks, "> 1 + 1", 2, TNumber, 0)
	want(t, src, toks, "> 1 + 1", 4, TOperator, 0)
	want(t, src, toks, ">    model", 0, TOperator, 0)
	want(t, src, toks, ">    model", 5, TVariable, 0)
	want(t, src, toks, "|> f", 0, TOperator, 0)
	want(t, src, toks, "sq x =", 0, TFunction, Declaration)
	want(t, src, toks, "sq x =", 3, TParameter, Declaration)
	n := 0
	for _, k := range toks {
		if k.Line == 4 {
			n++
		}
	}
	if n != 2 { // `>` and `model`
		t.Fatalf("line 4 has %d tokens, want 2: %+v", n, toks)
	}
}

// TestParseScratch_RoundTrip: ToFile and ToModule are inverses on the file's text and on the
// module's text past each watch's generated `itK = `, on watch lines (one or several spaces
// after `>`), continuation lines and plain lines alike.
func TestParseScratch_RoundTrip(t *testing.T) {
	src := "in Posts.Show\n\n> 1 + 1\n>    model\n|> f\nsq x = x * x\n"
	d := ParseScratch("scratch.alder", src)
	if got := d.File.Source; got != "\n\nit1 = 1 + 1\nit2 = model\n|> f\nsq x = x * x\n" {
		t.Fatalf("source %q", got)
	}
	// Known points: a module position and the file position it is.
	for _, c := range []struct{ mod, file diag.Pos }{
		{diag.Pos{Line: 4, Col: 7}, diag.Pos{Line: 3, Col: 3}},   // `1` of `> 1 + 1`
		{diag.Pos{Line: 4, Col: 11}, diag.Pos{Line: 3, Col: 7}},  // the second `1`
		{diag.Pos{Line: 5, Col: 7}, diag.Pos{Line: 4, Col: 6}},   // `model` of `>    model`
		{diag.Pos{Line: 5, Col: 12}, diag.Pos{Line: 4, Col: 11}}, // just past `model`
		{diag.Pos{Line: 6, Col: 4}, diag.Pos{Line: 5, Col: 4}},   // `f` of the continuation `|> f`
		{diag.Pos{Line: 7, Col: 4}, diag.Pos{Line: 6, Col: 4}},   // `x` of `sq x`
	} {
		if got := d.ToFile(c.mod); got != c.file {
			t.Errorf("ToFile(%v) = %v, want %v", c.mod, got, c.file)
		}
		if got := d.ToModule(c.file); got != c.mod {
			t.Errorf("ToModule(%v) = %v, want %v", c.file, got, c.mod)
		}
	}
	// Every file position (one past each line's end included) survives the round trip.
	for i, l := range d.File.Lines {
		for c := 1; c <= len(l)+1; c++ {
			p := diag.Pos{Line: i + 1, Col: c}
			if got := d.ToFile(d.ToModule(p)); got != p {
				t.Errorf("ToFile(ToModule(%v)) = %v", p, got)
			}
		}
	}
	// Every module position past the generated prefix survives it too.
	gen := map[int]int{}
	for _, w := range d.File.Watches {
		gen[w.Line+1] = len(w.Name) + len(" = ")
	}
	for i, l := range strings.Split(d.File.Source, "\n") { // module line i+2: the `module Scratch` line is not in the file
		for c := gen[i+2] + 1; c <= len(l)+1; c++ {
			p := diag.Pos{Line: i + 2, Col: c}
			if got := d.ToModule(d.ToFile(p)); got != p {
				t.Errorf("ToModule(ToFile(%v)) = %v", p, got)
			}
		}
	}
	// The generated `itK =` and the `module Scratch` line clamp into the file.
	if got := d.ToFile(diag.Pos{Line: 5, Col: 1}); got != (diag.Pos{Line: 4, Col: 1}) {
		t.Errorf("ToFile(it2) = %v, want 4:1", got)
	}
	if got := d.ToFile(diag.Pos{Line: 1, Col: 8}); got != (diag.Pos{Line: 1, Col: 8}) {
		t.Errorf("ToFile(module line) = %v, want 1:8", got)
	}
}
