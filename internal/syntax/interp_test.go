// internal/syntax/interp_test.go
package syntax

import (
	"fmt"
	"os"
	"reflect"
	"testing"
)

func stringTok(t *testing.T, toks []Token, text string) Token {
	t.Helper()
	for _, k := range toks {
		if k.Kind == String && k.Text == text {
			return k
		}
	}
	t.Fatalf("no String token %s", text)
	return Token{}
}

func TestParseTokens_IsTokensAndParseFile(t *testing.T) {
	src, err := os.ReadFile("../../examples/blog/app/Posts/Show.alder")
	if err != nil {
		t.Fatal(err)
	}
	m, toks, ds := ParseTokens("Show.alder", src)
	want, wantDs := Tokens("Show.alder", src)
	if !reflect.DeepEqual(toks, want) || len(ds) != len(wantDs) {
		t.Fatalf("ParseTokens' tokens differ from Tokens'")
	}
	pm, _ := ParseFile("Show.alder", src)
	if len(m.Decls) != len(pm.Decls) || len(m.Imports) != len(pm.Imports) || m.Name != pm.Name || len(m.Comments) != len(pm.Comments) {
		t.Fatalf("module %s with %d decls, want %s with %d", m.Name, len(m.Decls), pm.Name, len(pm.Decls))
	}
}

func TestParserKind(t *testing.T) {
	for _, c := range []struct {
		tok  Token
		want Kind
	}{
		{Token{Kind: Keyword, Text: "msg", Col: 1}, Keyword},
		{Token{Kind: Keyword, Text: "msg", Col: 5}, LIdent},
		{Token{Kind: Keyword, Text: "match", Col: 5}, Keyword},
		{Token{Kind: LIdent, Text: "msg", Col: 5}, LIdent},
	} {
		if got := ParserKind(c.tok); got != c.want {
			t.Errorf("ParserKind(%s %q col %d) = %s, want %s", c.tok.Kind, c.tok.Text, c.tok.Col, got, c.want)
		}
	}
}

// TestInterpolations: the expression of each `{…}` and its tokens, at the columns the parser
// gives the expression's nodes; an escaped `\{` is not one.
func TestInterpolations(t *testing.T) {
	src := "module M\n\nx = \"a {b.c} \\{d} e\"\n"
	m, toks, _ := ParseTokens("t.alder", []byte(src))
	tok := stringTok(t, toks, `"a {b.c} \{d} e"`)
	ins := Interpolations("t.alder", tok)
	if len(ins) != 1 || tok.Text[ins[0].From:ins[0].To] != "b.c" {
		t.Fatalf("interpolations %+v", ins)
	}
	var got []string
	for _, k := range ins[0].Tokens {
		if k.Kind != EOF {
			got = append(got, fmt.Sprintf("%s@%d:%d", k.Kind, k.Line, k.Col))
		}
	}
	// `"` is column 5 of line 3, so `{` is 8 and `b` 9, `.` 10, `c` 11
	if want := []string{"LIdent@3:9", "Dot@3:10", "LIdent@3:11"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("tokens %v, want %v", got, want)
	}
	// the parser's node for the same expression starts where the token does
	str := m.Decls[0].(*ValueDecl).Body.(*StrLit)
	if acc := str.Parts[1].Expr.(*Access); acc.Span.Start.Line != 3 || acc.Span.Start.Col != 9 {
		t.Fatalf("Access at %v", acc.Span.Start)
	}
}

// TestInterpolations_Nested: a string inside an interpolation is one token of it; its own
// interpolations come from asking again, at the right columns.
func TestInterpolations_Nested(t *testing.T) {
	src := "module M\n\nx = \"o {f \"i {z}\"} w\"\n"
	_, toks, _ := ParseTokens("t.alder", []byte(src))
	outer := stringTok(t, toks, `"o {f "i {z}"} w"`)
	ins := Interpolations("t.alder", outer)
	if len(ins) != 1 {
		t.Fatalf("outer %+v", ins)
	}
	inner := stringTok(t, ins[0].Tokens, `"i {z}"`)
	in2 := Interpolations("t.alder", inner)
	if len(in2) != 1 || in2[0].Tokens[0].Text != "z" || in2[0].Tokens[0].Line != 3 || in2[0].Tokens[0].Col != 15 {
		t.Fatalf("inner %+v", in2)
	}
}

func TestInterpolations_NotAString(t *testing.T) {
	if got := Interpolations("t.alder", Token{Kind: LIdent, Text: "x"}); got != nil {
		t.Fatalf("got %+v", got)
	}
	// an unterminated `{` gives none, and no panic
	_, toks, _ := ParseTokens("t.alder", []byte("module M\n\nx = \"a {b\"\n"))
	if got := Interpolations("t.alder", stringTok(t, toks, `"a {b"`)); len(got) != 0 {
		t.Fatalf("got %+v", got)
	}
}
