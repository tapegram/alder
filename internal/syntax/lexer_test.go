// internal/syntax/lexer_test.go
package syntax

import (
	"strings"
	"testing"
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
