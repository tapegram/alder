// internal/syntax/lexer_test.go
package syntax

import (
	"strings"
	"testing"

	"alder/internal/diag"
)

func kinds(src string) string {
	l := NewLexer("t.alder", []byte(src))
	var out []string
	for {
		t := l.Next()
		if t.Kind == EOF {
			break
		}
		if t.Kind == Keyword || t.Kind == LIdent || t.Kind == UIdent || t.Kind == Int || t.Kind == String {
			out = append(out, t.Kind.String()+"("+t.Text+")")
		} else {
			out = append(out, t.Kind.String())
		}
	}
	return strings.Join(out, " ")
}

func TestLexer_Basics(t *testing.T) {
	got := kinds("add : Int -> Int\nadd a b = a + b -- comment\n")
	want := "LIdent(add) Colon UIdent(Int) Arrow UIdent(Int) NL LIdent(add) LIdent(a) LIdent(b) Equal LIdent(a) Plus LIdent(b) NL"
	if got != want {
		t.Fatalf("\n got %s\nwant %s", got, want)
	}
}

func TestLexer_ExponentLiterals(t *testing.T) {
	cases := []struct{ src, want string }{
		{"1.5e-3", "Float(1.5e-3)"},
		{"1e6", "Float(1e6)"},
		{"2E+3", "Float(2E+3)"},
		{"1e", "Int(1) LIdent(e)"},
		{"1.5e", "Float(1.5) LIdent(e)"},
		{"2e+", "Int(2) LIdent(e) Plus"},
		{"e1", "LIdent(e1)"},
	}
	for _, c := range cases {
		l := NewLexer("t.alder", []byte(c.src))
		var out []string
		for {
			tk := l.Next()
			if tk.Kind == EOF {
				break
			}
			switch tk.Kind {
			case Float, Int, LIdent:
				out = append(out, tk.Kind.String()+"("+tk.Text+")")
			default:
				out = append(out, tk.Kind.String())
			}
		}
		if got := strings.Join(out, " "); got != c.want {
			t.Errorf("%q: got %s want %s", c.src, got, c.want)
		}
	}
}

func TestLexer_OperatorsAndKeywords(t *testing.T) {
	got := kinds(`match x { Just y -> y ?? 0, _ -> [1, ..rest] } |> f <- g == != <= >= && || ++ \x ?hole @id 3.5 "s"`)
	want := "Keyword(match) LIdent(x) LBrace UIdent(Just) LIdent(y) Arrow LIdent(y) Question2 Int(0) Comma Underscore Arrow LBracket Int(1) Comma DotDot LIdent(rest) RBracket RBrace PipeGt LIdent(f) LeftArrow LIdent(g) EqEq NotEq Le Ge AndAnd OrOr PlusPlus Backslash LIdent(x) Hole At LIdent(id) Float String(\"s\")"
	if got != want {
		t.Fatalf("\n got %s\nwant %s", got, want)
	}
}

func TestLexer_Positions(t *testing.T) {
	l := NewLexer("t.alder", []byte("ab\n  cd"))
	a := l.Next()
	l.Next() // NL
	c := l.Next()
	if a.Line != 1 || a.Col != 1 || c.Line != 2 || c.Col != 3 || c.Offset != 5 {
		t.Fatalf("positions: %+v %+v", a, c)
	}
}

func TestLexer_ResetAndScanText(t *testing.T) {
	l := NewLexer("t.alder", []byte("<p>Hello {name}!</p>"))
	l.Next() // Lt
	l.Next() // p
	l.Next() // Gt
	tx := l.ScanText()
	if tx.Kind != Text || tx.Text != "Hello " {
		t.Fatalf("text: %+v", tx)
	}
	save := l.Pos()
	if l.Next().Kind != LBrace {
		t.Fatal("expected LBrace after text")
	}
	l.Reset(save)
	if l.Next().Kind != LBrace {
		t.Fatal("reset failed")
	}
}

func TestLexer_HTMLTagIdent(t *testing.T) {
	l := NewLexer("t.alder", []byte("aria-current data-x"))
	l.SetHTMLTag(true)
	if tok := l.Next(); tok.Text != "aria-current" {
		t.Fatalf("got %q", tok.Text)
	}
	l.SetHTMLTag(false)
	if tok := l.Next(); tok.Text != "data" {
		t.Fatalf("got %q", tok.Text)
	}
}

func TestLexer_StringRaw(t *testing.T) {
	l := NewLexer("t.alder", []byte(`"a \"q\" {x} \{b}"`))
	tok := l.Next()
	if tok.Kind != String || tok.Text != `"a \"q\" {x} \{b}"` {
		t.Fatalf("got %+v", tok)
	}
}

func TestLexer_CRLFAndBOM(t *testing.T) {
	got := kinds("\xEF\xBB\xBFmodule X\r\nx : Int\r\n")
	want := "Keyword(module) UIdent(X) NL LIdent(x) Colon UIdent(Int) NL"
	if got != want {
		t.Fatalf("\n got %s\nwant %s", got, want)
	}
}

func TestLexer_UnterminatedString(t *testing.T) {
	l := NewLexer("t.alder", []byte("x = \"abc\ny = 1"))
	for l.Next().Kind != EOF {
	}
	errs := l.Errors()
	if len(errs) != 1 || errs[0].Line != 1 || errs[0].Col != 5 {
		t.Fatalf("errors: %+v", errs)
	}
}

func TestLexer_ResetDropsErrors(t *testing.T) {
	l := NewLexer("t.alder", []byte("x = \"abc\ny = 1"))
	for l.Next().Kind != EOF {
	}
	errs := l.Errors()
	if len(errs) != 1 {
		t.Fatalf("expected 1 error before reset, got %d: %+v", len(errs), errs)
	}
	// Reset to beginning should drop the error
	l.Reset(0)
	for l.Next().Kind != EOF {
	}
	errs = l.Errors()
	if len(errs) != 1 {
		t.Fatalf("expected 1 error after reset and re-lex, got %d: %+v", len(errs), errs)
	}
}

func TestLexer_ResetClampsOffset(t *testing.T) {
	l := NewLexer("t.alder", []byte("hello"))
	// Reset with offset beyond source length should not panic
	l.Reset(1000)
	tok := l.Next()
	if tok.Kind != EOF {
		t.Fatalf("expected EOF after clamped reset, got %+v", tok)
	}
}

func TestLexer_ScanTextLessThan(t *testing.T) {
	l := NewLexer("t.alder", []byte("a < b<p>"))
	tx := l.ScanText()
	if tx.Kind != Text || tx.Text != "a < b" {
		t.Fatalf("got %+v", tx)
	}
	if next := l.Next(); next.Kind != Lt {
		t.Fatalf("expected Lt after text, got %+v", next)
	}
}

func TestLexer_MarkRestore(t *testing.T) {
	l := NewLexer("t.alder", []byte("a \"abc\nb"))
	l.Next()
	m := l.Mark()
	if tok := l.Next(); tok.Kind != String || len(l.Errors()) != 1 {
		t.Fatalf("expected unterminated string with 1 error, got %+v, %d errors", tok, len(l.Errors()))
	}
	l.Restore(m)
	if l.Mark() != m || len(l.Errors()) != 0 {
		t.Fatalf("state not restored: %+v vs %+v, %d errors", l.Mark(), m, len(l.Errors()))
	}
}

func TestLexer_KeywordAfterDot(t *testing.T) {
	got := kinds("Db.where x")
	want := "UIdent(Db) Dot LIdent(where) LIdent(x)"
	if got != want {
		t.Fatalf("\n got %s\nwant %s", got, want)
	}
	if got := kinds("where x"); got != "Keyword(where) LIdent(x)" {
		t.Fatalf("plain where must stay a keyword, got %s", got)
	}
}

// ParseString is a test convenience wrapping ParseFile for string sources.
func ParseString(file, src string) (*Module, []diag.Diagnostic) { return ParseFile(file, []byte(src)) }

func TestLex_QuoteInsideInterpolation(t *testing.T) {
	src := "module M\n\nx : Text\nx = \"a {Text.join \"-\" [\"b\", \"c\"]} \\\"d\\\"\"\n"
	m, diags := ParseString("M.alder", src)
	if len(diags) != 0 {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}
	got := Dump(m.Decls[len(m.Decls)-1])
	want := `(val x : Text = (str "a " (app (var Text.join) (str "-") (list (str "b") (str "c"))) " \"d\""))`
	if got != want {
		t.Fatalf("\n got %s\nwant %s", got, want)
	}
}

// Braces nested inside an interpolation, and inside a nested string's own interpolation,
// are matched with the string rule: `}` inside a nested literal does not close the outer `{`.
func TestLex_NestedBracesInInterpolation(t *testing.T) {
	for _, s := range []string{
		`"x {f { a = "}" }} y"`,
		`"x {g "in {h "deep"} er"} y"`,
		`"a \{ b"`,
	} {
		l := NewLexer("t.alder", []byte(s+" z"))
		tok := l.Next()
		if tok.Kind != String || tok.Text != s || len(l.Errors()) != 0 {
			t.Errorf("%s: got %+v errs %v", s, tok, l.Errors())
		}
	}
}

// An unbalanced `{` still lexes as a string, so the parser reports it precisely.
func TestLex_UnbalancedBraceInString(t *testing.T) {
	_, diags := ParseString("M.alder", "module M\nx : Text\nx = \"a { b\"\n")
	if len(diags) != 1 || diags[0].Code != "E0001" || !strings.Contains(diags[0].Message, "unterminated `{` in string") {
		t.Fatalf("diags: %v", diags)
	}
}

// Tokens reports the stream the parser consumed: HTML text runs as Text tokens, lookahead once.
func TestTokens_ParserDriven(t *testing.T) {
	toks, diags := Tokens("V.alder", []byte("module V\nv = <p>it's {x}</p>\n"))
	if len(diags) != 0 {
		t.Fatalf("diags: %v", diags)
	}
	var got []string
	for _, tk := range toks {
		got = append(got, tk.Kind.String()+"("+tk.Text+")")
	}
	want := "Keyword(module) UIdent(V) NL(\n) LIdent(v) Equal(=) Lt(<) LIdent(p) Gt(>) Text(it's ) LBrace({) LIdent(x) RBrace(}) Text() Lt(<) Slash(/) LIdent(p) Gt(>) NL(\n) EOF()"
	if s := strings.Join(got, " "); s != want {
		t.Fatalf("\n got %s\nwant %s", s, want)
	}
}

const commentsSrc = "module M -- head\n-- own line\nf : Text\nf = \"a -- b\" -- tail\nv : Html Never\nv = <p>x -- y<script>a -- b</script></p>\n"

func TestLexer_CollectsComments(t *testing.T) {
	m, _ := ParseFile("m.alder", []byte(commentsSrc))
	wantText := []string{"-- head", "-- own line", "-- tail"}
	wantLine := []int{1, 2, 4}
	wantCol := []int{10, 1, 14}
	wantOffset := []int{9, 17, 51} // byte offsets of the `--`
	if len(m.Comments) != len(wantText) {
		t.Fatalf("comments = %+v, want %d", m.Comments, len(wantText))
	}
	for i, c := range m.Comments {
		if c.Text != wantText[i] || c.Line != wantLine[i] || c.Col != wantCol[i] || c.Offset != wantOffset[i] {
			t.Errorf("comment %d = %+v, want %q at %d:%d, offset %d", i, c, wantText[i], wantLine[i], wantCol[i], wantOffset[i])
		}
		if !strings.HasPrefix(commentsSrc[c.Offset:], c.Text) {
			t.Errorf("comment %d: the source at offset %d is %q, not the comment", i, c.Offset, commentsSrc[c.Offset:])
		}
	}
	sm, _ := ParseSignatures("m.alder", []byte("module M -- head\nf : Text -- sig\n"))
	if len(sm.Comments) != 2 {
		t.Errorf("signature comments = %+v, want 2", sm.Comments)
	}
}

func TestLexer_RestoreDropsComments(t *testing.T) {
	l := NewLexer("m.alder", []byte("a -- one\nb"))
	l.Next()
	s := l.Mark()
	l.Next()
	l.Next()
	if len(l.Comments()) != 1 {
		t.Fatalf("comments = %+v", l.Comments())
	}
	l.Restore(s)
	if len(l.Comments()) != 0 {
		t.Fatalf("after Restore: %+v", l.Comments())
	}
	l.Next()
	l.Reset(0)
	if len(l.Comments()) != 0 {
		t.Fatalf("after Reset: %+v", l.Comments())
	}
}

// A line of markup text after the first that begins with `--` is a comment (the parser drops it
// from the text): it is collected, marked Markup. `--` elsewhere in text is text.
func TestLexer_CollectsMarkupComments(t *testing.T) {
	src := "module M\nv = <p>a -- b\n  -- one  \n  c\n  -- two {x}</p>\n"
	m, diags := ParseFile("m.alder", []byte(src))
	if len(diags) > 0 {
		t.Fatal(diags)
	}
	want := []Comment{
		{Text: "-- one", Line: 3, Col: 3, Offset: strings.Index(src, "-- one"), Markup: true},
		{Text: "-- two", Line: 5, Col: 3, Offset: strings.Index(src, "-- two"), Markup: true},
	}
	if len(m.Comments) != len(want) {
		t.Fatalf("comments = %+v, want %+v", m.Comments, want)
	}
	for i := range want {
		if m.Comments[i] != want[i] {
			t.Errorf("comment %d = %+v, want %+v", i, m.Comments[i], want[i])
		}
	}
}
