// internal/semtok/lexical_test.go
package semtok

import "testing"

const lexSrc = `module M

-- a doc line
f : Int -> Int
f x = if x == 0 then 1 else x * 2 |> g -- trailing

h = \y -> ?todo

k = match v { _ -> "s" }

s = "a {b.c} d"

page "/p/{slug}" { slug: Text }
`

func TestLexical(t *testing.T) {
	toks := Module("t.alder", []byte(lexSrc))
	checkInvariants(t, "lexSrc", lexSrc, toks)
	want(t, lexSrc, toks, "module M", 0, TKeyword, 0)
	want(t, lexSrc, toks, "-- a doc line", 0, TComment, Documentation)
	want(t, lexSrc, toks, "-- trailing", 0, TComment, 0)
	want(t, lexSrc, toks, "if x ==", 0, TKeyword, 0)
	want(t, lexSrc, toks, "x == 0", 0, TVariable, 0)
	want(t, lexSrc, toks, "== 0", 0, TOperator, 0)
	want(t, lexSrc, toks, "0 then", 0, TNumber, 0)
	want(t, lexSrc, toks, "|> g", 0, TOperator, 0)
	want(t, lexSrc, toks, "-> Int", 0, TOperator, 0)
	wantNone(t, lexSrc, toks, ": Int", 0)
	want(t, lexSrc, toks, `\y`, 0, TKeyword, 0)
	wantLen(t, lexSrc, toks, "?todo", 0, THole, 5)
	wantNone(t, lexSrc, toks, "_ ->", 0)
	wantNone(t, lexSrc, toks, "{ _", 0)
	wantLen(t, lexSrc, toks, `"s"`, 0, TString, 3)
}

// TestLexical_Interpolation: a literal with an interpolation is string pieces around the
// expression's own tokens (the braces stay string); a page path's `{slug}` is not one.
func TestLexical_Interpolation(t *testing.T) {
	toks := Module("t.alder", []byte(lexSrc))
	wantLen(t, lexSrc, toks, `"a {b.c} d"`, 0, TString, 4) // `"a {`
	want(t, lexSrc, toks, `"a {b.c} d"`, 4, TVariable, 0)  // b
	wantNone(t, lexSrc, toks, `"a {b.c} d"`, 5)            // .
	wantLen(t, lexSrc, toks, `"a {b.c} d"`, 7, TString, 4) // `} d"`
	wantLen(t, lexSrc, toks, `"/p/{slug}"`, 0, TString, 11)
}

// TestLexical_DeclKeywordInExpression: a declaration keyword is a keyword at column 1 only; in
// an expression the parser reads it as a name.
func TestLexical_DeclKeywordInExpression(t *testing.T) {
	const src = "k = match msg { _ -> session.user }\n\nmsg Go\n"
	toks := Module("t.alder", []byte(src))
	checkInvariants(t, "decl", src, toks)
	want(t, src, toks, "msg {", 0, TVariable, 0)
	want(t, src, toks, "session.", 0, TVariable, 0)
	want(t, src, toks, "msg Go", 0, TKeyword, 0)
}
