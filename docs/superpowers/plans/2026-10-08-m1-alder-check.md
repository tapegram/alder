# M1 `alder check` Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** A Go binary `alder` whose `check` command parses, resolves and type-checks an Alder app, passes `examples/todos` clean, and reports every error in the E1–E4 corpus with the code and fix the spec lists.

**Architecture:** A lazy hand-written lexer with an HTML sub-mode feeds a recursive-descent parser into an AST. A names pass builds the module graph, resolves every identifier to a `Ref`, and exposes the global pages and app messages. A bidirectional checker (check-against-expected plus infer) with first-order unification types declarations first (generating `Params`, `Msg`, table record/insert/column types), then bodies, HTML, and page rules, collecting diagnostics into one bag that renders as text or JSON lines.

**Tech Stack:** Go 1.27 (stdlib only; no third-party dependencies in M1), `go test` with golden files under `testdata/`.

**Spec:** `docs/superpowers/specs/2026-10-08-compiler-vertical-slice-design.md` — read it first; the plan argues from it. The language itself is specified by `book/03-syntax.md` through `book/09-stdlib.md` and `book/A-grammar.md`; the checker implements those chapters.

## Global Constraints

- Go `1.27`, module path `alder`, one binary `cmd/alder`. No third-party modules.
- Package layout exactly as the spec: `cmd/alder`, `internal/{syntax,names,types,check,diag}`, `stdlib/` (embedded `.alder` signature files), `testdata/{golden,corpus}`.
- Diagnostics: every error has a stable code from the spec table (E0101…E0602) or the general codes defined in Task 1; JSON lines use the field names `code, severity, file, line, col, endLine, endCol, message, fix` with `fix = {line, col, replace}`. Exit code 1 on any error, 0 otherwise. Text format: `file:line:col: message` then the source line, a caret line, and `fix: ...` when present.
- The checker never stops at the first error: every phase appends and continues.
- `examples/todos` must check clean at the end of Task 12. Any disagreement between the book and the checker is resolved by changing whichever is wrong **and saying so in the commit message**; the book is updated in the same commit.
- Positions are 1-based line and column, columns in bytes.
- Tests: every task adds `go test ./...`-runnable tests before implementation (TDD); golden expectations are regenerated only with `go test ./... -update` and the diff is read before committing.

## Review Focus

Inputs the spec implies but no task's tests exercise directly; each line's test is added to the owning task.

1. A source file with a UTF-8 BOM or CRLF line endings must lex and parse as if plain LF without a BOM (Task 2 adds `TestLexer_CRLFAndBOM`).
2. An unterminated string or HTML element at end of file must produce one diagnostic at the opening position, not a panic or a cascade (Task 2 `TestLexer_UnterminatedString`, Task 5 `TestParse_UnclosedElement`).
3. Two modules that each import the other (`Todo` ↔ `User`) must produce exactly one E0006 naming both modules, and checking must still proceed for all other modules (Task 6 `TestResolve_ImportCycle`).
4. A `match` with duplicate arms (`Low -> …, Low -> …`) must report E0310 "unreachable arm" rather than silently accept (Task 9 `TestCheck_DuplicateArm`).
5. A `--json` run on a directory with no `.alder` files must print nothing and exit 1 with a text message on stderr, not exit 0 (Task 12 `TestCLI_NoFiles`).

---

## File structure

```
go.mod                                  module alder, go 1.27
cmd/alder/main.go                       CLI entry: subcommand dispatch, exit codes
internal/diag/diag.go                   Pos, Span, Fix, Diagnostic, Bag, codes
internal/diag/render.go                 text and JSON rendering
internal/diag/diag_test.go
internal/syntax/token.go                Kind, Token
internal/syntax/lexer.go                Lexer (lazy, position-based, HTML sub-mode)
internal/syntax/lexer_test.go
internal/syntax/ast.go                  AST node types, Ref
internal/syntax/dump.go                 Dump(node) → S-expression text for golden tests
internal/syntax/parser.go               Parser core: module, declarations, types
internal/syntax/parser_expr.go          expressions, patterns, blocks, strings
internal/syntax/parser_html.go          HTML
internal/syntax/parser_test.go          golden-driven
internal/names/graph.go                 module discovery, import graph, cycles
internal/names/resolve.go               scope building and identifier resolution
internal/names/names_test.go
internal/types/types.go                 Type representation, printing
internal/types/unify.go                 substitution, unify, normalize (Id T, Insert T)
internal/types/unify_test.go
internal/types/stdlib.go                load embedded stdlib signatures into an Env
stdlib/*.alder                          Prelude, List, Maybe, Result, Text, Int, Float, Bool,
                                        Date, Time, Html, Url, Query, Task, Db, Session, Form
internal/check/check.go                 entry point Program(), per-module driver
internal/check/decls.go                 declaration pass: types, tables, pages, msgs, forms, caps
internal/check/expr.go                  expression checking
internal/check/pattern.go               patterns and exhaustiveness
internal/check/html.go                  HTML checking, attribute table
internal/check/pages.go                 load/view/update rules, page checks
internal/check/fixes.go                 fix text builders for the spec's codes
internal/check/check_test.go            golden-driven over testdata/golden and testdata/corpus
testdata/golden/<case>.alder + .diag    parser/checker goldens
testdata/corpus/<code>-<n>/app/*.alder + expected.diag
```

Each `internal/*` package has one responsibility and depends only on packages above it in the list (`check` → `types`, `names`, `syntax`, `diag`).

---

### Task 1: Module scaffold and diagnostics

**Files:**
- Create: `go.mod`, `cmd/alder/main.go`, `internal/diag/diag.go`, `internal/diag/render.go`, `internal/diag/diag_test.go`

**Interfaces:**
- Produces:
  ```go
  package diag
  type Pos struct{ Line, Col int }
  type Span struct{ File string; Start, End Pos }
  type Fix struct{ Line, Col int; Replace string }
  type Diagnostic struct{ Code, Severity string; Span Span; Message string; Fix *Fix }
  type Bag struct{ Items []Diagnostic }
  func (b *Bag) Error(code string, span Span, format string, args ...any) *Diagnostic
  func (b *Bag) Warn(code string, span Span, format string, args ...any) *Diagnostic
  func (b *Bag) HasErrors() bool
  func (b *Bag) Sort()                       // by file, line, col
  func RenderText(d Diagnostic, lines []string) string   // lines = source lines of d.Span.File
  func RenderJSON(d Diagnostic) string                   // one line, no trailing newline
  ```
- General codes (constants in `diag.go`): `E0001` parse error, `E0002` unknown name, `E0003` type mismatch, `E0004` non-exhaustive match, `E0005` module name/path mismatch, `E0006` import cycle, `E0007` missing signature, `E0008` wrong number of arguments, `E0009` duplicate declaration, `E0010` missing required declaration (Model/load/view/update), `E0011` invalid declaration (bad page param type, bad table attr, …), `E0310` unreachable match arm, `W0001` unused import.

- [ ] **Step 1: Write the failing tests**

```go
// internal/diag/diag_test.go
package diag

import "testing"

func TestBag_ErrorAndSort(t *testing.T) {
	var b Bag
	b.Error("E0003", Span{File: "b.alder", Start: Pos{2, 1}, End: Pos{2, 4}}, "expected %s", "Int")
	b.Error("E0002", Span{File: "a.alder", Start: Pos{1, 5}, End: Pos{1, 6}}, "unknown name")
	if !b.HasErrors() {
		t.Fatal("expected errors")
	}
	b.Sort()
	if b.Items[0].Span.File != "a.alder" || b.Items[0].Code != "E0002" {
		t.Fatalf("sort order wrong: %+v", b.Items)
	}
	if b.Items[1].Message != "expected Int" {
		t.Fatalf("message not formatted: %q", b.Items[1].Message)
	}
}

func TestRenderJSON(t *testing.T) {
	d := Diagnostic{Code: "E0101", Severity: "error",
		Span:    Span{File: "app/X.alder", Start: Pos{3, 7}, End: Pos{3, 12}},
		Message: "Db.get takes an id", Fix: &Fix{Line: 3, Col: 7, Replace: "Db.first"}}
	got := RenderJSON(d)
	want := `{"code":"E0101","severity":"error","file":"app/X.alder","line":3,"col":7,"endLine":3,"endCol":12,"message":"Db.get takes an id","fix":{"line":3,"col":7,"replace":"Db.first"}}`
	if got != want {
		t.Fatalf("\n got %s\nwant %s", got, want)
	}
	d.Fix = nil
	if got := RenderJSON(d); got[len(got)-1] != '}' || contains(got, `"fix"`) {
		t.Fatalf("fix should be omitted when nil: %s", got)
	}
}

func TestRenderText(t *testing.T) {
	d := Diagnostic{Code: "E0003", Severity: "error",
		Span:    Span{File: "app/X.alder", Start: Pos{2, 5}, End: Pos{2, 8}},
		Message: "expected Int, got Text", Fix: &Fix{Line: 2, Col: 5, Replace: "Text.fromInt n"}}
	lines := []string{"module X", "x = foo + 1"}
	got := RenderText(d, lines)
	want := "app/X.alder:2:5: error[E0003]: expected Int, got Text\n" +
		"  x = foo + 1\n" +
		"      ^^^\n" +
		"  fix: Text.fromInt n\n"
	if got != want {
		t.Fatalf("\n got %q\nwant %q", got, want)
	}
}

func contains(s, sub string) bool { return len(s) >= len(sub) && (func() bool { for i := 0; i+len(sub) <= len(s); i++ { if s[i:i+len(sub)] == sub { return true } }; return false })() }
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd /Users/tavishpegram/projects/alder && go mod init alder 2>/dev/null; go test ./internal/diag/`
Expected: FAIL — `undefined: Bag` etc.

- [ ] **Step 3: Implement diag**

```go
// go.mod
module alder

go 1.27
```

```go
// internal/diag/diag.go
package diag

import (
	"fmt"
	"sort"
)

type Pos struct{ Line, Col int }
type Span struct {
	File       string
	Start, End Pos
}
type Fix struct {
	Line, Col int
	Replace   string
}
type Diagnostic struct {
	Code     string
	Severity string // "error" | "warning"
	Span     Span
	Message  string
	Fix      *Fix
}

// General codes. Spec codes (E0101..E0602) are declared where they are raised.
const (
	ParseError      = "E0001"
	UnknownName     = "E0002"
	TypeMismatch    = "E0003"
	NonExhaustive   = "E0004"
	ModulePath      = "E0005"
	ImportCycle     = "E0006"
	MissingSig      = "E0007"
	Arity           = "E0008"
	Duplicate       = "E0009"
	MissingDecl     = "E0010"
	InvalidDecl     = "E0011"
	UnreachableArm  = "E0310"
	UnusedImport    = "W0001"
)

type Bag struct{ Items []Diagnostic }

func (b *Bag) add(sev, code string, span Span, format string, args ...any) *Diagnostic {
	b.Items = append(b.Items, Diagnostic{Code: code, Severity: sev, Span: span, Message: fmt.Sprintf(format, args...)})
	return &b.Items[len(b.Items)-1]
}
func (b *Bag) Error(code string, span Span, format string, args ...any) *Diagnostic {
	return b.add("error", code, span, format, args...)
}
func (b *Bag) Warn(code string, span Span, format string, args ...any) *Diagnostic {
	return b.add("warning", code, span, format, args...)
}
func (b *Bag) HasErrors() bool {
	for _, d := range b.Items {
		if d.Severity == "error" {
			return true
		}
	}
	return false
}
func (b *Bag) Sort() {
	sort.SliceStable(b.Items, func(i, j int) bool {
		a, c := b.Items[i].Span, b.Items[j].Span
		if a.File != c.File {
			return a.File < c.File
		}
		if a.Start.Line != c.Start.Line {
			return a.Start.Line < c.Start.Line
		}
		return a.Start.Col < c.Start.Col
	})
}
```

Note: `add` returns a pointer into the slice; callers set `.Fix` immediately, before the next append (append may reallocate). Document this on `Error`.

```go
// internal/diag/render.go
package diag

import (
	"encoding/json"
	"fmt"
	"strings"
)

func RenderText(d Diagnostic, lines []string) string {
	var sb strings.Builder
	fmt.Fprintf(&sb, "%s:%d:%d: %s[%s]: %s\n", d.Span.File, d.Span.Start.Line, d.Span.Start.Col, d.Severity, d.Code, d.Message)
	if l := d.Span.Start.Line; l >= 1 && l <= len(lines) {
		src := lines[l-1]
		fmt.Fprintf(&sb, "  %s\n", src)
		width := 1
		if d.Span.End.Line == l && d.Span.End.Col > d.Span.Start.Col {
			width = d.Span.End.Col - d.Span.Start.Col
		}
		fmt.Fprintf(&sb, "  %s%s\n", strings.Repeat(" ", d.Span.Start.Col-1), strings.Repeat("^", width))
	}
	if d.Fix != nil {
		fmt.Fprintf(&sb, "  fix: %s\n", d.Fix.Replace)
	}
	return sb.String()
}

type jsonFix struct {
	Line    int    `json:"line"`
	Col     int    `json:"col"`
	Replace string `json:"replace"`
}
type jsonDiag struct {
	Code     string   `json:"code"`
	Severity string   `json:"severity"`
	File     string   `json:"file"`
	Line     int      `json:"line"`
	Col      int      `json:"col"`
	EndLine  int      `json:"endLine"`
	EndCol   int      `json:"endCol"`
	Message  string   `json:"message"`
	Fix      *jsonFix `json:"fix,omitempty"`
}

func RenderJSON(d Diagnostic) string {
	j := jsonDiag{d.Code, d.Severity, d.Span.File, d.Span.Start.Line, d.Span.Start.Col, d.Span.End.Line, d.Span.End.Col, d.Message, nil}
	if d.Fix != nil {
		j.Fix = &jsonFix{d.Fix.Line, d.Fix.Col, d.Fix.Replace}
	}
	b, _ := json.Marshal(j)
	return string(b)
}
```

```go
// cmd/alder/main.go
package main

import (
	"fmt"
	"os"
)

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(2)
	}
	switch os.Args[1] {
	case "check":
		os.Exit(runCheck(os.Args[2:]))
	default:
		usage()
		os.Exit(2)
	}
}

func usage() { fmt.Fprintln(os.Stderr, "usage: alder check [dir] [--json]") }

// runCheck is completed in Task 12.
func runCheck(args []string) int {
	fmt.Fprintln(os.Stderr, "alder check: not implemented")
	return 2
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/diag/ && go build ./cmd/alder && ./alder; echo exit=$?`
Expected: `ok`, usage text, `exit=2`.

- [ ] **Step 5: Commit**

```bash
git add go.mod cmd internal/diag && git commit -m "m1: module scaffold and diagnostics package"
```

---

### Task 2: Lexer

**Files:**
- Create: `internal/syntax/token.go`, `internal/syntax/lexer.go`, `internal/syntax/lexer_test.go`

**Interfaces:**
- Produces:
  ```go
  package syntax
  type Kind int   // see constants below
  type Token struct{ Kind Kind; Text string; Line, Col int; Offset int }
  type Lexer struct{ /* src []byte, file string, pos, line, col int, htmlTag bool */ }
  func NewLexer(file string, src []byte) *Lexer
  func (l *Lexer) Next() Token            // next token in code mode; skips spaces, tabs, comments; emits NL for newlines
  func (l *Lexer) Pos() int               // byte offset after the last returned token
  func (l *Lexer) Reset(offset int)       // rewind (recomputes line/col)
  func (l *Lexer) ScanText() Token        // HTML mode: raw text up to '<' or '{' (Kind Text); "" allowed
  func (l *Lexer) SetHTMLTag(on bool)     // inside a tag: '-' is an identifier char (aria-current)
  func (l *Lexer) Errors() []LexError     // {Line, Col, Msg}
  ```
- Kinds: `EOF, NL, LIdent, UIdent, Int, Float, String, Text, LParen, RParen, LBrace, RBrace, LBracket, RBracket, Comma, Colon, Dot, DotDot, Pipe, Arrow, LeftArrow, Equal, EqEq, NotEq, Lt, Le, Gt, Ge, Plus, Minus, Star, Slash, Percent, PlusPlus, AndAnd, OrOr, Question2, PipeGt, Backslash, Underscore, At, Hole, Keyword`. A `Keyword` token has `Text` in {module import type brand where page msg form table capability session login test let match if then else for in requires}. `Hole` is `?name`. `String` carries the raw literal including quotes; interpolation is split by the parser (Task 4).
- `Text` tokens (HTML mode) carry raw text with `\n` normalized; leading/trailing whitespace is preserved (the parser trims per HTML rules).

- [ ] **Step 1: Write the failing tests**

```go
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
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/syntax/`
Expected: FAIL — undefined NewLexer.

- [ ] **Step 3: Implement token.go and lexer.go**

```go
// internal/syntax/token.go
package syntax

type Kind int

const (
	EOF Kind = iota
	NL
	LIdent
	UIdent
	Int
	Float
	String
	Text
	LParen
	RParen
	LBrace
	RBrace
	LBracket
	RBracket
	Comma
	Colon
	Dot
	DotDot
	Pipe
	Arrow
	LeftArrow
	Equal
	EqEq
	NotEq
	Lt
	Le
	Gt
	Ge
	Plus
	Minus
	Star
	Slash
	Percent
	PlusPlus
	AndAnd
	OrOr
	Question2
	PipeGt
	Backslash
	Underscore
	At
	Hole
	Keyword
	Invalid
)

var kindNames = [...]string{"EOF", "NL", "LIdent", "UIdent", "Int", "Float", "String", "Text", "LParen", "RParen", "LBrace", "RBrace", "LBracket", "RBracket", "Comma", "Colon", "Dot", "DotDot", "Pipe", "Arrow", "LeftArrow", "Equal", "EqEq", "NotEq", "Lt", "Le", "Gt", "Ge", "Plus", "Minus", "Star", "Slash", "Percent", "PlusPlus", "AndAnd", "OrOr", "Question2", "PipeGt", "Backslash", "Underscore", "At", "Hole", "Keyword", "Invalid"}

func (k Kind) String() string { return kindNames[k] }

type Token struct {
	Kind   Kind
	Text   string
	Line   int
	Col    int
	Offset int
}

var keywords = map[string]bool{"module": true, "import": true, "type": true, "brand": true, "where": true, "page": true, "msg": true, "form": true, "table": true, "capability": true, "session": true, "login": true, "test": true, "let": true, "match": true, "if": true, "then": true, "else": true, "for": true, "in": true, "requires": true}
```

```go
// internal/syntax/lexer.go
package syntax

import "bytes"

type LexError struct {
	Line, Col int
	Msg       string
}

type Lexer struct {
	file    string
	src     []byte
	pos     int
	line    int
	col     int
	htmlTag bool
	errs    []LexError
}

func NewLexer(file string, src []byte) *Lexer {
	src = bytes.TrimPrefix(src, []byte("\xEF\xBB\xBF"))
	src = bytes.ReplaceAll(src, []byte("\r\n"), []byte("\n"))
	return &Lexer{file: file, src: src, line: 1, col: 1}
}

func (l *Lexer) Errors() []LexError   { return l.errs }
func (l *Lexer) Pos() int             { return l.pos }
func (l *Lexer) SetHTMLTag(on bool)   { l.htmlTag = on }
func (l *Lexer) Reset(offset int) {
	l.pos, l.line, l.col = 0, 1, 1
	for l.pos < offset {
		l.advance()
	}
}

func (l *Lexer) advance() byte {
	c := l.src[l.pos]
	l.pos++
	if c == '\n' {
		l.line++
		l.col = 1
	} else {
		l.col++
	}
	return c
}
func (l *Lexer) peek() byte {
	if l.pos >= len(l.src) {
		return 0
	}
	return l.src[l.pos]
}
func (l *Lexer) peek2() byte {
	if l.pos+1 >= len(l.src) {
		return 0
	}
	return l.src[l.pos+1]
}
func (l *Lexer) tok(k Kind, text string, line, col, off int) Token {
	return Token{Kind: k, Text: text, Line: line, Col: col, Offset: off}
}

func isLower(c byte) bool { return c >= 'a' && c <= 'z' }
func isUpper(c byte) bool { return c >= 'A' && c <= 'Z' }
func isDigit(c byte) bool { return c >= '0' && c <= '9' }
func (l *Lexer) isIdent(c byte) bool {
	return isLower(c) || isUpper(c) || isDigit(c) || c == '_' || (l.htmlTag && c == '-')
}

// Next returns the next code-mode token.
func (l *Lexer) Next() Token {
	for l.pos < len(l.src) {
		c := l.peek()
		if c == ' ' || c == '\t' {
			l.advance()
			continue
		}
		if c == '-' && l.peek2() == '-' {
			for l.pos < len(l.src) && l.peek() != '\n' {
				l.advance()
			}
			continue
		}
		break
	}
	line, col, off := l.line, l.col, l.pos
	if l.pos >= len(l.src) {
		return l.tok(EOF, "", line, col, off)
	}
	c := l.advance()
	switch {
	case c == '\n':
		return l.tok(NL, "\n", line, col, off)
	case isLower(c) || isUpper(c):
		for l.pos < len(l.src) && l.isIdent(l.peek()) {
			l.advance()
		}
		text := string(l.src[off:l.pos])
		if keywords[text] {
			return l.tok(Keyword, text, line, col, off)
		}
		if isUpper(c) {
			return l.tok(UIdent, text, line, col, off)
		}
		return l.tok(LIdent, text, line, col, off)
	case c == '_':
		if l.isIdent(l.peek()) {
			for l.pos < len(l.src) && l.isIdent(l.peek()) {
				l.advance()
			}
			return l.tok(LIdent, string(l.src[off:l.pos]), line, col, off)
		}
		return l.tok(Underscore, "_", line, col, off)
	case isDigit(c):
		for isDigit(l.peek()) {
			l.advance()
		}
		if l.peek() == '.' && isDigit(l.peek2()) {
			l.advance()
			for isDigit(l.peek()) {
				l.advance()
			}
			return l.tok(Float, string(l.src[off:l.pos]), line, col, off)
		}
		return l.tok(Int, string(l.src[off:l.pos]), line, col, off)
	case c == '"':
		for {
			if l.pos >= len(l.src) || l.peek() == '\n' {
				l.errs = append(l.errs, LexError{line, col, "unterminated string"})
				return l.tok(String, string(l.src[off:l.pos]), line, col, off)
			}
			d := l.advance()
			if d == '\\' && l.pos < len(l.src) {
				l.advance()
				continue
			}
			if d == '"' {
				return l.tok(String, string(l.src[off:l.pos]), line, col, off)
			}
		}
	case c == '?':
		if l.peek() == '?' {
			l.advance()
			return l.tok(Question2, "??", line, col, off)
		}
		for l.pos < len(l.src) && l.isIdent(l.peek()) {
			l.advance()
		}
		return l.tok(Hole, string(l.src[off+1:l.pos]), line, col, off)
	}
	two := func(k Kind, s string) Token { l.advance(); return l.tok(k, s, line, col, off) }
	switch c {
	case '(':
		return l.tok(LParen, "(", line, col, off)
	case ')':
		return l.tok(RParen, ")", line, col, off)
	case '{':
		return l.tok(LBrace, "{", line, col, off)
	case '}':
		return l.tok(RBrace, "}", line, col, off)
	case '[':
		return l.tok(LBracket, "[", line, col, off)
	case ']':
		return l.tok(RBracket, "]", line, col, off)
	case ',':
		return l.tok(Comma, ",", line, col, off)
	case ':':
		return l.tok(Colon, ":", line, col, off)
	case '@':
		return l.tok(At, "@", line, col, off)
	case '\\':
		return l.tok(Backslash, "\\", line, col, off)
	case '.':
		if l.peek() == '.' {
			return two(DotDot, "..")
		}
		return l.tok(Dot, ".", line, col, off)
	case '|':
		if l.peek() == '>' {
			return two(PipeGt, "|>")
		}
		if l.peek() == '|' {
			return two(OrOr, "||")
		}
		return l.tok(Pipe, "|", line, col, off)
	case '-':
		if l.peek() == '>' {
			return two(Arrow, "->")
		}
		return l.tok(Minus, "-", line, col, off)
	case '<':
		if l.peek() == '-' {
			return two(LeftArrow, "<-")
		}
		if l.peek() == '=' {
			return two(Le, "<=")
		}
		return l.tok(Lt, "<", line, col, off)
	case '>':
		if l.peek() == '=' {
			return two(Ge, ">=")
		}
		return l.tok(Gt, ">", line, col, off)
	case '=':
		if l.peek() == '=' {
			return two(EqEq, "==")
		}
		return l.tok(Equal, "=", line, col, off)
	case '!':
		if l.peek() == '=' {
			return two(NotEq, "!=")
		}
	case '+':
		if l.peek() == '+' {
			return two(PlusPlus, "++")
		}
		return l.tok(Plus, "+", line, col, off)
	case '*':
		return l.tok(Star, "*", line, col, off)
	case '/':
		return l.tok(Slash, "/", line, col, off)
	case '%':
		return l.tok(Percent, "%", line, col, off)
	case '&':
		if l.peek() == '&' {
			return two(AndAnd, "&&")
		}
	}
	l.errs = append(l.errs, LexError{line, col, "unexpected character " + string(c)})
	return l.tok(Invalid, string(c), line, col, off)
}

// ScanText reads raw HTML text from the current position up to '<' or '{'.
func (l *Lexer) ScanText() Token {
	line, col, off := l.line, l.col, l.pos
	for l.pos < len(l.src) && l.peek() != '<' && l.peek() != '{' {
		l.advance()
	}
	return l.tok(Text, string(l.src[off:l.pos]), line, col, off)
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/syntax/`
Expected: PASS (8 tests).

- [ ] **Step 5: Commit**

```bash
git add internal/syntax && git commit -m "m1: lexer with HTML text mode and positions"
```

---

### Task 3: AST and dump

**Files:**
- Create: `internal/syntax/ast.go`, `internal/syntax/dump.go`, `internal/syntax/dump_test.go`

**Interfaces:**
- Produces the node types below. Every node embeds `Span diag.Span` via `Node` and later passes attach resolution to `Ref` fields. `Dump(any) string` prints a canonical S-expression used by the parser goldens.

- [ ] **Step 1: Write the failing test**

```go
// internal/syntax/dump_test.go
package syntax

import "testing"

func TestDump(t *testing.T) {
	e := &App{Fn: &Var{Name: "add"}, Args: []Expr{&IntLit{Value: 1}, &Var{Module: "M", Name: "x"}}}
	got := Dump(e)
	want := "(app (var add) (int 1) (var M.x))"
	if got != want {
		t.Fatalf("got %s", got)
	}
}
```

- [ ] **Step 2: Run to verify it fails**

Run: `go test ./internal/syntax/ -run TestDump`
Expected: FAIL — undefined App.

- [ ] **Step 3: Write ast.go and dump.go**

```go
// internal/syntax/ast.go
package syntax

import "alder/internal/diag"

type Node struct{ Span diag.Span }

func (n *Node) GetSpan() diag.Span { return n.Span }

// Ref is filled in by the names pass.
type RefKind int

const (
	RefNone      RefKind = iota
	RefValue             // top-level value/function
	RefLocal             // parameter, let, lambda param, pattern var
	RefType              // type/table/form/brand declaration
	RefCtor              // union constructor (incl. Msg ctors)
	RefPage              // page module used as a value
	RefTable             // table used as a value (Db.get Todo)
	RefColumn            // Todo.done
	RefComponent         // capitalized function used as element
	RefCapability        // capability module (Password.verify)
	RefStd               // standard library value
)

type Ref struct {
	Kind   RefKind
	Module string // declaring module ("" for locals)
	Name   string // declaration name
}

// ---- Module and declarations

type Module struct {
	Node
	Name    string
	File    string
	Imports []*Import
	Decls   []Decl
	Lines   []string // source lines, for rendering
}
type Import struct {
	Node
	Module string
}

type Decl interface{ GetSpan() diag.Span }

type ValueDecl struct {
	Node
	Name   string
	Sig    Type   // nil only in stdlib signature files (body nil) or when missing (E0007)
	Params []*Ident
	Body   Expr   // nil in stdlib signature files
}
type Ident struct {
	Node
	Name string
}
type TypeDecl struct {
	Node
	Name   string
	Params []string
	Union  []*Ctor     // one of Union/Record/Brand set; all nil = opaque (stdlib only)
	Record *RecordType
	Brand  *BrandType
}
type Ctor struct {
	Node
	Name string
	Arg  Type // nil for no payload
}
type BrandType struct {
	Node
	Base  string // Int | Float | Bool | Text
	Check Expr   // nil if no `where`
}
type Field struct {
	Node
	Name    string
	Type    Type
	Default Expr   // nil if none
	Attrs   []*Attr
}
type Attr struct {
	Node
	Name string // id | ref | created | updated | unique
	Arg  string // table name for ref
}
type PageDecl struct {
	Node
	Path     string
	Params   *RecordType
	Requires string
}
type MsgDecl struct {
	Node
	Name   string
	Params *RecordType // nil = no params
	Form   string      // "" = no form
}
type FormDecl struct {
	Node
	Name   string
	Fields []*Field
}
type TableDecl struct {
	Node
	Name   string
	Fields []*Field
}
type CapDecl struct {
	Node
	Name    string
	Entries []*Field // Type is a FunType or effect type
}
type SessionDecl struct {
	Node
	Fields []*Field
}
type LoginDecl struct {
	Node
	Page string
}
type TestDecl struct {
	Node
	Name string
	Body Expr
}

// ---- Types

type Type interface{ GetSpan() diag.Span }

type TypeName struct {
	Node
	Module string // "" if unqualified
	Name   string
	Args   []Type
	Ref    Ref
}
type TypeVar struct {
	Node
	Name string
}
type RecordType struct {
	Node
	Fields []*Field
}
type FunType struct {
	Node
	Params []Type
	Result Type
}

// ---- Expressions

type Expr interface{ GetSpan() diag.Span }

type IntLit struct {
	Node
	Value int64
}
type FloatLit struct {
	Node
	Value float64
}
type BoolLit struct {
	Node
	Value bool
}
type UnitLit struct{ Node }
type StrLit struct {
	Node
	Parts []StrPart // literal text and interpolated expressions, in order
}
type StrPart struct {
	Text string
	Expr Expr // nil for text parts
}
type ListLit struct {
	Node
	Elems []Expr
}
type RecordLit struct {
	Node
	Fields []*FieldInit
}
type FieldInit struct {
	Node
	Name  string
	Value Expr
}
type RecordUpdate struct {
	Node
	Base   Expr
	Fields []*FieldInit
}
type Var struct {
	Node
	Module string
	Name   string
	Ref    Ref
}
type CtorRef struct { // capitalized name in expression position: Just, Normal, Todos.Show, App.Logout
	Node
	Module string
	Name   string
	Ref    Ref
}
type Access struct {
	Node
	Expr  Expr
	Field string
}
type App struct {
	Node
	Fn   Expr
	Args []Expr
}
type Lambda struct {
	Node
	Params []*Ident
	Body   Expr
}
type If struct {
	Node
	Cond, Then, Else Expr
}
type Match struct {
	Node
	Subject Expr
	Arms    []*Arm
}
type Arm struct {
	Node
	Pat  Pattern
	Body Expr
}
type Block struct {
	Node
	Stmts  []*LetStmt
	Result Expr
}
type LetStmt struct {
	Node
	Name string
	Type Type // optional annotation
	Bind bool // true for `let x <- e`
	Expr Expr
}
type BinOp struct {
	Node
	Op   string // "+", "++", "==", "|>", "??", ...
	L, R Expr
}
type HoleExpr struct {
	Node
	Name string
}

// ---- HTML

type Element struct {
	Node
	Module   string // for <Layout.Page>
	Tag      string // lowercase html tag or capitalized component name
	Attrs    []*HtmlAttr
	Children []Expr // Element, HtmlText, HtmlExpr, HtmlFor, HtmlIf, HtmlMatch
	Ref      Ref    // component resolution
}
type HtmlAttr struct {
	Node
	Name  string
	Value Expr // StrLit for literal, any Expr for {…}, nil for bare (`<Errors form />`)
}
type HtmlText struct {
	Node
	Text string
}
type HtmlExpr struct {
	Node
	Expr Expr
}
type HtmlFor struct {
	Node
	Var  string
	List Expr
	Body Expr
}
type HtmlIf struct {
	Node
	Cond, Then, Else Expr
}
type HtmlMatch struct {
	Node
	Subject Expr
	Arms    []*Arm
}

// ---- Patterns

type Pattern interface{ GetSpan() diag.Span }

type PWild struct{ Node }
type PVar struct {
	Node
	Name string
}
type PLit struct {
	Node
	Lit Expr // IntLit, StrLit (no interpolation), BoolLit
}
type PCtor struct {
	Node
	Name string
	Arg  Pattern // nil if none
	Ref  Ref
}
type PRecord struct {
	Node
	Fields []*PField
}
type PField struct {
	Node
	Name string
	Pat  Pattern
}
type PList struct {
	Node
	Elems []Pattern
	Rest  string // "" if none
}
```

```go
// internal/syntax/dump.go
package syntax

import (
	"fmt"
	"strings"
)

// Dump renders any node as a compact S-expression. Used by golden tests.
func Dump(n any) string {
	var sb strings.Builder
	dump(&sb, n)
	return sb.String()
}

func dumpList[T any](sb *strings.Builder, items []T) {
	for _, it := range items {
		sb.WriteByte(' ')
		dump(sb, it)
	}
}

func dump(sb *strings.Builder, n any) {
	switch x := n.(type) {
	case nil:
		sb.WriteString("nil")
	case *Module:
		fmt.Fprintf(sb, "(module %s", x.Name)
		for _, im := range x.Imports {
			fmt.Fprintf(sb, " (import %s)", im.Module)
		}
		dumpList(sb, x.Decls)
		sb.WriteByte(')')
	case *ValueDecl:
		fmt.Fprintf(sb, "(val %s", x.Name)
		if x.Sig != nil {
			sb.WriteString(" : ")
			dump(sb, x.Sig)
		}
		for _, p := range x.Params {
			sb.WriteString(" " + p.Name)
		}
		if x.Body != nil {
			sb.WriteString(" = ")
			dump(sb, x.Body)
		}
		sb.WriteByte(')')
	case *TypeDecl:
		fmt.Fprintf(sb, "(type %s", x.Name)
		for _, p := range x.Params {
			sb.WriteString(" " + p)
		}
		switch {
		case x.Union != nil:
			for _, c := range x.Union {
				fmt.Fprintf(sb, " (ctor %s", c.Name)
				if c.Arg != nil {
					sb.WriteByte(' ')
					dump(sb, c.Arg)
				}
				sb.WriteByte(')')
			}
		case x.Record != nil:
			sb.WriteByte(' ')
			dump(sb, x.Record)
		case x.Brand != nil:
			fmt.Fprintf(sb, " (brand %s", x.Brand.Base)
			if x.Brand.Check != nil {
				sb.WriteString(" where ")
				dump(sb, x.Brand.Check)
			}
			sb.WriteByte(')')
		default:
			sb.WriteString(" opaque")
		}
		sb.WriteByte(')')
	case *PageDecl:
		fmt.Fprintf(sb, "(page %q ", x.Path)
		dump(sb, x.Params)
		if x.Requires != "" {
			sb.WriteString(" requires " + x.Requires)
		}
		sb.WriteByte(')')
	case *MsgDecl:
		fmt.Fprintf(sb, "(msg %s", x.Name)
		if x.Params != nil {
			sb.WriteByte(' ')
			dump(sb, x.Params)
		}
		if x.Form != "" {
			sb.WriteString(" form " + x.Form)
		}
		sb.WriteByte(')')
	case *FormDecl:
		fmt.Fprintf(sb, "(form %s", x.Name)
		dumpList(sb, x.Fields)
		sb.WriteByte(')')
	case *TableDecl:
		fmt.Fprintf(sb, "(table %s", x.Name)
		dumpList(sb, x.Fields)
		sb.WriteByte(')')
	case *CapDecl:
		fmt.Fprintf(sb, "(capability %s", x.Name)
		dumpList(sb, x.Entries)
		sb.WriteByte(')')
	case *SessionDecl:
		sb.WriteString("(session")
		dumpList(sb, x.Fields)
		sb.WriteByte(')')
	case *LoginDecl:
		fmt.Fprintf(sb, "(login %s)", x.Page)
	case *TestDecl:
		fmt.Fprintf(sb, "(test %q ", x.Name)
		dump(sb, x.Body)
		sb.WriteByte(')')
	case *Field:
		fmt.Fprintf(sb, "(field %s ", x.Name)
		dump(sb, x.Type)
		if x.Default != nil {
			sb.WriteString(" = ")
			dump(sb, x.Default)
		}
		for _, a := range x.Attrs {
			fmt.Fprintf(sb, " @%s", a.Name)
			if a.Arg != "" {
				sb.WriteString(" " + a.Arg)
			}
		}
		sb.WriteByte(')')
	case *TypeName:
		if len(x.Args) == 0 {
			sb.WriteString(qual(x.Module, x.Name))
			return
		}
		fmt.Fprintf(sb, "(%s", qual(x.Module, x.Name))
		dumpList(sb, x.Args)
		sb.WriteByte(')')
	case *TypeVar:
		sb.WriteString(x.Name)
	case *RecordType:
		sb.WriteString("{")
		for i, f := range x.Fields {
			if i > 0 {
				sb.WriteString(", ")
			}
			sb.WriteString(f.Name + ": ")
			dump(sb, f.Type)
			if f.Default != nil {
				sb.WriteString(" = ")
				dump(sb, f.Default)
			}
		}
		sb.WriteString("}")
	case *FunType:
		sb.WriteString("(fn")
		dumpList(sb, x.Params)
		sb.WriteString(" -> ")
		dump(sb, x.Result)
		sb.WriteByte(')')
	case *IntLit:
		fmt.Fprintf(sb, "(int %d)", x.Value)
	case *FloatLit:
		fmt.Fprintf(sb, "(float %g)", x.Value)
	case *BoolLit:
		fmt.Fprintf(sb, "(bool %v)", x.Value)
	case *UnitLit:
		sb.WriteString("(unit)")
	case *StrLit:
		sb.WriteString("(str")
		for _, p := range x.Parts {
			if p.Expr == nil {
				fmt.Fprintf(sb, " %q", p.Text)
			} else {
				sb.WriteByte(' ')
				dump(sb, p.Expr)
			}
		}
		sb.WriteByte(')')
	case *ListLit:
		sb.WriteString("(list")
		dumpList(sb, x.Elems)
		sb.WriteByte(')')
	case *RecordLit:
		sb.WriteString("(record")
		for _, f := range x.Fields {
			fmt.Fprintf(sb, " (%s ", f.Name)
			dump(sb, f.Value)
			sb.WriteByte(')')
		}
		sb.WriteByte(')')
	case *RecordUpdate:
		sb.WriteString("(update ")
		dump(sb, x.Base)
		for _, f := range x.Fields {
			fmt.Fprintf(sb, " (%s ", f.Name)
			dump(sb, f.Value)
			sb.WriteByte(')')
		}
		sb.WriteByte(')')
	case *Var:
		fmt.Fprintf(sb, "(var %s)", qual(x.Module, x.Name))
	case *CtorRef:
		fmt.Fprintf(sb, "(ctor %s)", qual(x.Module, x.Name))
	case *Access:
		sb.WriteString("(access ")
		dump(sb, x.Expr)
		sb.WriteString(" " + x.Field + ")")
	case *App:
		sb.WriteString("(app ")
		dump(sb, x.Fn)
		dumpList(sb, x.Args)
		sb.WriteByte(')')
	case *Lambda:
		sb.WriteString("(lambda")
		for _, p := range x.Params {
			sb.WriteString(" " + p.Name)
		}
		sb.WriteString(" -> ")
		dump(sb, x.Body)
		sb.WriteByte(')')
	case *If:
		sb.WriteString("(if ")
		dump(sb, x.Cond)
		sb.WriteByte(' ')
		dump(sb, x.Then)
		sb.WriteByte(' ')
		dump(sb, x.Else)
		sb.WriteByte(')')
	case *Match:
		sb.WriteString("(match ")
		dump(sb, x.Subject)
		dumpList(sb, x.Arms)
		sb.WriteByte(')')
	case *Arm:
		sb.WriteString("(arm ")
		dump(sb, x.Pat)
		sb.WriteByte(' ')
		dump(sb, x.Body)
		sb.WriteByte(')')
	case *Block:
		sb.WriteString("(block")
		dumpList(sb, x.Stmts)
		sb.WriteByte(' ')
		dump(sb, x.Result)
		sb.WriteByte(')')
	case *LetStmt:
		op := "="
		if x.Bind {
			op = "<-"
		}
		fmt.Fprintf(sb, "(let %s", x.Name)
		if x.Type != nil {
			sb.WriteString(" : ")
			dump(sb, x.Type)
		}
		sb.WriteString(" " + op + " ")
		dump(sb, x.Expr)
		sb.WriteByte(')')
	case *BinOp:
		fmt.Fprintf(sb, "(%s ", x.Op)
		dump(sb, x.L)
		sb.WriteByte(' ')
		dump(sb, x.R)
		sb.WriteByte(')')
	case *HoleExpr:
		fmt.Fprintf(sb, "(hole %s)", x.Name)
	case *Element:
		fmt.Fprintf(sb, "(el %s", qual(x.Module, x.Tag))
		for _, a := range x.Attrs {
			fmt.Fprintf(sb, " (attr %s", a.Name)
			if a.Value != nil {
				sb.WriteByte(' ')
				dump(sb, a.Value)
			}
			sb.WriteByte(')')
		}
		dumpList(sb, x.Children)
		sb.WriteByte(')')
	case *HtmlText:
		fmt.Fprintf(sb, "(text %q)", x.Text)
	case *HtmlExpr:
		sb.WriteString("(expr ")
		dump(sb, x.Expr)
		sb.WriteByte(')')
	case *HtmlFor:
		fmt.Fprintf(sb, "(for %s ", x.Var)
		dump(sb, x.List)
		sb.WriteByte(' ')
		dump(sb, x.Body)
		sb.WriteByte(')')
	case *HtmlIf:
		sb.WriteString("(hif ")
		dump(sb, x.Cond)
		sb.WriteByte(' ')
		dump(sb, x.Then)
		sb.WriteByte(' ')
		dump(sb, x.Else)
		sb.WriteByte(')')
	case *HtmlMatch:
		sb.WriteString("(hmatch ")
		dump(sb, x.Subject)
		dumpList(sb, x.Arms)
		sb.WriteByte(')')
	case *PWild:
		sb.WriteString("_")
	case *PVar:
		sb.WriteString(x.Name)
	case *PLit:
		dump(sb, x.Lit)
	case *PCtor:
		if x.Arg == nil {
			fmt.Fprintf(sb, "(pctor %s)", x.Name)
			return
		}
		fmt.Fprintf(sb, "(pctor %s ", x.Name)
		dump(sb, x.Arg)
		sb.WriteByte(')')
	case *PRecord:
		sb.WriteString("(prec")
		for _, f := range x.Fields {
			fmt.Fprintf(sb, " (%s ", f.Name)
			dump(sb, f.Pat)
			sb.WriteByte(')')
		}
		sb.WriteByte(')')
	case *PList:
		sb.WriteString("(plist")
		dumpList(sb, x.Elems)
		if x.Rest != "" {
			sb.WriteString(" .." + x.Rest)
		}
		sb.WriteByte(')')
	default:
		fmt.Fprintf(sb, "<%T>", n)
	}
}

func qual(m, n string) string {
	if m == "" {
		return n
	}
	return m + "." + n
}
```

- [ ] **Step 4: Run tests**

Run: `go test ./internal/syntax/`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/syntax && git commit -m "m1: AST node types and S-expression dump"
```

---

### Task 4: Parser — modules, declarations, types

**Files:**
- Create: `internal/syntax/parser.go`, `internal/syntax/parser_test.go`, `testdata/golden/parse_decls.alder`, `testdata/golden/parse_decls.dump`

**Interfaces:**
- Produces:
  ```go
  func ParseFile(file string, src []byte) (*Module, []diag.Diagnostic)   // never panics; recovers at declaration boundaries
  func ParseSignatures(file string, src []byte) (*Module, []diag.Diagnostic) // stdlib mode: bodies optional, `type X a` opaque allowed
  ```
- Parser internals used by Task 5: `p.tok Token` (current), `p.advance()`, `p.expect(Kind, string) Token`, `p.at(Kind) bool`, `p.atKeyword(string) bool`, `p.skipNL()`, `p.span(start Token) diag.Span`, `p.errorAt(tok, msg)`, `p.parseExpr() Expr` (Task 5), `p.parseType() Type`, `p.parseRecordType() *RecordType`, `p.parseFieldList(closing Kind, attrs bool) []*Field`.
- Golden test harness in `parser_test.go` reads every `testdata/golden/parse_*.alder`, parses it, and compares `Dump(module)` (plus any diagnostics as `! code line:col message` lines) to the sibling `.dump` file; `-update` rewrites.

- [ ] **Step 1: Write the golden input and expected output**

```alder
-- testdata/golden/parse_decls.alder
module Todos.Index

import Todo
import Layout

page "/" { filter: Todo.Filter = All } requires user

type Model = { filter: Todo.Filter, todos: List Todo }

type Shape = Circle Float | Rect { w: Float, h: Float }

type TodoId = brand Text

type Slug = brand Text where Text.matches "^[a-z0-9-]+$"

type Tree a = Leaf | Node { left: Tree a, value: a, right: Tree a }

table Todo {
  id: TodoId @id,
  title: Text.NonEmpty,
  done: Bool = False,
  owner: User.UserId @ref User,
  created: Time @created,
}

form NewTodo = {
  title: Text.NonEmpty,
  due: Maybe Date,
}

msg Submit form NewTodo
msg Toggle { id: Todo.TodoId }
msg Logout

capability Password {
  verify : { email: Text.Email, password: Text } -> Task (Maybe User.UserId),
}

session { user: Maybe User.UserId, theme: Theme = Light }

login = Auth.Login

add : Int -> Int -> Int
add a b = a

first : List a -> Maybe a
first xs = Nothing

area : { w: Float, h: Float } -> Float
area r = 1.5

test "adds" = True
```

```
(module Todos.Index (import Todo) (import Layout) (page "/" {filter: Todo.Filter = (ctor All)} requires user) (type Model {filter: Todo.Filter, todos: (List Todo)}) (type Shape (ctor Circle Float) (ctor Rect {w: Float, h: Float})) (type TodoId (brand Text)) (type Slug (brand Text where (app (var Text.matches) (str "^[a-z0-9-]+$")))) (type Tree a (ctor Leaf) (ctor Node {left: (Tree a), value: a, right: (Tree a)})) (table Todo (field id TodoId @id) (field title Text.NonEmpty) (field done Bool = (bool false)) (field owner User.UserId @ref User) (field created Time @created)) (form NewTodo (field title Text.NonEmpty) (field due (Maybe Date))) (msg Submit form NewTodo) (msg Toggle {id: Todo.TodoId}) (msg Logout) (capability Password (field verify (fn {email: Text.Email, password: Text} -> (Task (Maybe User.UserId))))) (session (field user (Maybe User.UserId)) (field theme Theme = (ctor Light))) (login Auth.Login) (val add : (fn Int Int -> Int) a b = (var a)) (val first : (fn (List a) -> (Maybe a)) xs = (ctor Nothing)) (val area : (fn {w: Float, h: Float} -> Float) r = (float 1.5)) (test "adds" (bool true)))
```

Save the second block as `testdata/golden/parse_decls.dump` (one line, no trailing newline beyond one `\n`).

- [ ] **Step 2: Write the golden harness**

```go
// internal/syntax/parser_test.go
package syntax

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

var update = flag.Bool("update", false, "rewrite golden files")

func TestParseGolden(t *testing.T) {
	files, _ := filepath.Glob("../../testdata/golden/parse_*.alder")
	if len(files) == 0 {
		t.Fatal("no golden files")
	}
	for _, f := range files {
		t.Run(filepath.Base(f), func(t *testing.T) {
			src, err := os.ReadFile(f)
			if err != nil {
				t.Fatal(err)
			}
			m, diags := ParseFile(filepath.Base(f), src)
			var sb strings.Builder
			sb.WriteString(Dump(m))
			sb.WriteString("\n")
			for _, d := range diags {
				fmt.Fprintf(&sb, "! %s %d:%d %s\n", d.Code, d.Span.Start.Line, d.Span.Start.Col, d.Message)
			}
			got := sb.String()
			goldenPath := strings.TrimSuffix(f, ".alder") + ".dump"
			if *update {
				os.WriteFile(goldenPath, []byte(got), 0o644)
				return
			}
			want, err := os.ReadFile(goldenPath)
			if err != nil {
				t.Fatalf("missing golden %s (run with -update)", goldenPath)
			}
			if got != string(want) {
				t.Fatalf("golden mismatch for %s\n got: %s\nwant: %s", f, got, want)
			}
		})
	}
}
```

- [ ] **Step 3: Run to verify it fails**

Run: `go test ./internal/syntax/ -run TestParseGolden`
Expected: FAIL — undefined ParseFile.

- [ ] **Step 4: Implement parser.go**

```go
// internal/syntax/parser.go
package syntax

import (
	"strings"

	"alder/internal/diag"
)

type Parser struct {
	lx      *Lexer
	file    string
	tok     Token
	diags   []diag.Diagnostic
	sigMode bool
	lines   []string
}

func newParser(file string, src []byte, sigMode bool) *Parser {
	p := &Parser{lx: NewLexer(file, src), file: file, sigMode: sigMode}
	p.lines = strings.Split(string(p.lx.src), "\n")
	p.advance()
	return p
}

func ParseFile(file string, src []byte) (*Module, []diag.Diagnostic) {
	p := newParser(file, src, false)
	m := p.parseModule()
	return m, p.finish()
}

func ParseSignatures(file string, src []byte) (*Module, []diag.Diagnostic) {
	p := newParser(file, src, true)
	m := p.parseModule()
	return m, p.finish()
}

func (p *Parser) finish() []diag.Diagnostic {
	for _, e := range p.lx.Errors() {
		p.diags = append(p.diags, diag.Diagnostic{Code: diag.ParseError, Severity: "error",
			Span: diag.Span{File: p.file, Start: diag.Pos{e.Line, e.Col}, End: diag.Pos{e.Line, e.Col + 1}}, Message: e.Msg})
	}
	return p.diags
}

// ---- token helpers

func (p *Parser) advance() Token {
	prev := p.tok
	p.tok = p.lx.Next()
	return prev
}
func (p *Parser) at(k Kind) bool               { return p.tok.Kind == k }
func (p *Parser) atKeyword(s string) bool      { return p.tok.Kind == Keyword && p.tok.Text == s }
func (p *Parser) skipNL() {
	for p.tok.Kind == NL {
		p.advance()
	}
}
func (p *Parser) posOf(t Token) diag.Pos { return diag.Pos{t.Line, t.Col} }
func (p *Parser) spanFrom(start Token) diag.Span {
	return diag.Span{File: p.file, Start: p.posOf(start), End: diag.Pos{p.tok.Line, p.tok.Col}}
}
func (p *Parser) spanTok(t Token) diag.Span {
	return diag.Span{File: p.file, Start: p.posOf(t), End: diag.Pos{t.Line, t.Col + max(1, len(t.Text))}}
}
func (p *Parser) errorAt(t Token, msg string) {
	p.diags = append(p.diags, diag.Diagnostic{Code: diag.ParseError, Severity: "error", Span: p.spanTok(t), Message: msg})
}
func (p *Parser) expect(k Kind, what string) Token {
	if p.tok.Kind != k {
		p.errorAt(p.tok, "expected "+what+", got "+describe(p.tok))
		return p.tok
	}
	return p.advance()
}
func (p *Parser) expectKeyword(s string) Token {
	if !p.atKeyword(s) {
		p.errorAt(p.tok, "expected `"+s+"`, got "+describe(p.tok))
		return p.tok
	}
	return p.advance()
}
func describe(t Token) string {
	switch t.Kind {
	case EOF:
		return "end of file"
	case NL:
		return "end of line"
	default:
		return "`" + t.Text + "`"
	}
}

// skipToDecl recovers after a parse error: advance to the next line that starts a declaration.
func (p *Parser) skipToDecl() {
	for !p.at(EOF) {
		if p.at(NL) {
			p.advance()
			if p.tok.Col == 1 && (p.tok.Kind == Keyword || p.tok.Kind == LIdent || p.tok.Kind == UIdent) {
				return
			}
			continue
		}
		p.advance()
	}
}

// ---- module

func (p *Parser) parseModule() *Module {
	start := p.tok
	m := &Module{Lines: p.lines, File: p.file}
	p.skipNL()
	p.expectKeyword("module")
	m.Name = p.parseModName()
	for !p.at(EOF) {
		p.skipNL()
		if p.at(EOF) {
			break
		}
		before := p.tok
		if p.atKeyword("import") {
			t := p.advance()
			im := &Import{Module: p.parseModName()}
			im.Span = p.spanFrom(t)
			m.Imports = append(m.Imports, im)
		} else if d := p.parseDecl(); d != nil {
			m.Decls = append(m.Decls, d)
		}
		if !p.at(NL) && !p.at(EOF) {
			p.errorAt(p.tok, "expected end of line after declaration, got "+describe(p.tok))
			p.skipToDecl()
		}
		if p.tok == before { // no progress: avoid an infinite loop
			p.advance()
		}
	}
	m.Span = p.spanFrom(start)
	return m
}

func (p *Parser) parseModName() string {
	t := p.expect(UIdent, "module name")
	name := t.Text
	for p.at(Dot) {
		p.advance()
		name += "." + p.expect(UIdent, "module name segment").Text
	}
	return name
}

// ---- declarations

func (p *Parser) parseDecl() Decl {
	start := p.tok
	switch {
	case p.atKeyword("type"):
		return p.parseTypeDecl()
	case p.atKeyword("page"):
		p.advance()
		d := &PageDecl{Path: p.parseStringText()}
		d.Params = p.parseRecordType()
		if p.atKeyword("requires") {
			p.advance()
			d.Requires = p.expect(LIdent, "session field").Text
		}
		d.Span = p.spanFrom(start)
		return d
	case p.atKeyword("msg"):
		p.advance()
		d := &MsgDecl{Name: p.expect(UIdent, "message name").Text}
		if p.at(LBrace) {
			d.Params = p.parseRecordType()
		}
		if p.atKeyword("form") {
			p.advance()
			d.Form = p.expect(UIdent, "form name").Text
		}
		d.Span = p.spanFrom(start)
		return d
	case p.atKeyword("form"):
		p.advance()
		d := &FormDecl{Name: p.expect(UIdent, "form name").Text}
		p.expect(Equal, "`=`")
		p.expect(LBrace, "`{`")
		d.Fields = p.parseFieldList(RBrace, false)
		d.Span = p.spanFrom(start)
		return d
	case p.atKeyword("table"):
		p.advance()
		d := &TableDecl{Name: p.expect(UIdent, "table name").Text}
		p.expect(LBrace, "`{`")
		d.Fields = p.parseFieldList(RBrace, true)
		d.Span = p.spanFrom(start)
		return d
	case p.atKeyword("capability"):
		p.advance()
		d := &CapDecl{Name: p.expect(UIdent, "capability name").Text}
		p.expect(LBrace, "`{`")
		d.Entries = p.parseFieldList(RBrace, false)
		d.Span = p.spanFrom(start)
		return d
	case p.atKeyword("session"):
		p.advance()
		d := &SessionDecl{}
		p.expect(LBrace, "`{`")
		d.Fields = p.parseFieldList(RBrace, false)
		d.Span = p.spanFrom(start)
		return d
	case p.atKeyword("login"):
		p.advance()
		p.expect(Equal, "`=`")
		d := &LoginDecl{Page: p.parseModName()}
		d.Span = p.spanFrom(start)
		return d
	case p.atKeyword("test"):
		p.advance()
		d := &TestDecl{Name: p.parseStringText()}
		p.expect(Equal, "`=`")
		p.skipNL()
		d.Body = p.parseExpr()
		d.Span = p.spanFrom(start)
		return d
	case p.at(LIdent) || p.at(UIdent):
		return p.parseValueDecl()
	}
	p.errorAt(p.tok, "expected a declaration, got "+describe(p.tok))
	p.skipToDecl()
	return nil
}

// parseValueDecl handles `name : Type NL name params = expr`, and in sigMode a bare signature.
func (p *Parser) parseValueDecl() Decl {
	start := p.tok
	nameTok := p.advance()
	d := &ValueDecl{Name: nameTok.Text}
	if p.at(Colon) {
		p.advance()
		d.Sig = p.parseType()
		if p.sigMode && (p.at(NL) || p.at(EOF)) {
			// peek: is the next non-blank line this name's definition?
			save := p.lx.Pos()
			saveTok := p.tok
			p.skipNL()
			if !(p.tok.Kind == nameTok.Kind && p.tok.Text == nameTok.Text) {
				p.lx.Reset(save)
				p.tok = saveTok
				d.Span = p.spanFrom(start)
				return d
			}
			p.advance()
		} else {
			p.skipNL()
			if !(p.tok.Kind == nameTok.Kind && p.tok.Text == nameTok.Text) {
				p.errorAt(p.tok, "expected definition of `"+nameTok.Text+"` after its signature")
				d.Span = p.spanFrom(start)
				return d
			}
			p.advance()
		}
	}
	for p.at(LIdent) {
		t := p.advance()
		id := &Ident{Name: t.Text}
		id.Span = p.spanTok(t)
		d.Params = append(d.Params, id)
	}
	p.expect(Equal, "`=`")
	p.skipNL()
	d.Body = p.parseExpr()
	d.Span = p.spanFrom(start)
	return d
}

func (p *Parser) parseTypeDecl() Decl {
	start := p.tok
	p.advance()
	d := &TypeDecl{Name: p.expect(UIdent, "type name").Text}
	for p.at(LIdent) {
		d.Params = append(d.Params, p.advance().Text)
	}
	if p.sigMode && (p.at(NL) || p.at(EOF)) { // opaque
		d.Span = p.spanFrom(start)
		return d
	}
	p.expect(Equal, "`=`")
	p.skipNL()
	switch {
	case p.atKeyword("brand"):
		bt := p.tok
		p.advance()
		b := &BrandType{Base: p.expect(UIdent, "base type").Text}
		if p.atKeyword("where") {
			p.advance()
			b.Check = p.parseExpr()
		}
		b.Span = p.spanFrom(bt)
		d.Brand = b
	case p.at(LBrace):
		d.Record = p.parseRecordType()
	default:
		if p.at(Pipe) {
			p.advance()
		}
		for {
			ct := p.expect(UIdent, "constructor")
			c := &Ctor{Name: ct.Text}
			if p.at(UIdent) || p.at(LIdent) || p.at(LBrace) || p.at(LParen) {
				c.Arg = p.parseAType()
			}
			c.Span = p.spanFrom(ct)
			d.Union = append(d.Union, c)
			save, saveTok := p.lx.Pos(), p.tok
			p.skipNL()
			if p.at(Pipe) {
				p.advance()
				continue
			}
			p.lx.Reset(save)
			p.tok = saveTok
			break
		}
	}
	d.Span = p.spanFrom(start)
	return d
}

// parseFieldList parses `name: Type [= default] [@attr…] ,` … up to `closing`; the opening brace is consumed.
func (p *Parser) parseFieldList(closing Kind, attrs bool) []*Field {
	var fields []*Field
	for {
		p.skipNL()
		if p.at(closing) {
			p.advance()
			return fields
		}
		if p.at(EOF) {
			p.errorAt(p.tok, "unterminated field list")
			return fields
		}
		ft := p.expect(LIdent, "field name")
		f := &Field{Name: ft.Text}
		p.expect(Colon, "`:`")
		f.Type = p.parseType()
		if p.at(Equal) {
			p.advance()
			f.Default = p.parseExpr()
		}
		for attrs && p.at(At) {
			at := p.advance()
			a := &Attr{Name: p.expect(LIdent, "attribute name").Text}
			if a.Name == "ref" {
				a.Arg = p.expect(UIdent, "table name").Text
			}
			a.Span = p.spanFrom(at)
			f.Attrs = append(f.Attrs, a)
		}
		f.Span = p.spanFrom(ft)
		fields = append(fields, f)
		p.skipNL()
		if p.at(Comma) {
			p.advance()
			continue
		}
		p.skipNL()
		if !p.at(closing) {
			p.errorAt(p.tok, "expected `,` or closing brace")
			return fields
		}
	}
}

func (p *Parser) parseStringText() string {
	t := p.expect(String, "string literal")
	s := t.Text
	if len(s) >= 2 && s[0] == '"' && s[len(s)-1] == '"' {
		return s[1 : len(s)-1]
	}
	return s
}

// ---- types

// parseType: atype {atype} ["->" type]
func (p *Parser) parseType() Type {
	start := p.tok
	first := p.parseAType()
	var args []Type
	for p.at(UIdent) || p.at(LIdent) || p.at(LBrace) || p.at(LParen) {
		args = append(args, p.parseAType())
	}
	var head Type = first
	if len(args) > 0 {
		tn, ok := first.(*TypeName)
		if !ok {
			p.errorAt(start, "only a named type can take type arguments")
		} else {
			tn.Args = args
			tn.Span = p.spanFrom(start)
		}
	}
	if p.at(Arrow) {
		p.advance()
		res := p.parseType()
		ft := &FunType{Params: []Type{head}, Result: res}
		if inner, ok := res.(*FunType); ok { // a -> b -> c flattens to params [a, b]
			ft.Params = append(ft.Params, inner.Params...)
			ft.Result = inner.Result
		}
		ft.Span = p.spanFrom(start)
		return ft
	}
	return head
}

func (p *Parser) parseAType() Type {
	start := p.tok
	switch {
	case p.at(UIdent):
		t := p.advance()
		tn := &TypeName{Name: t.Text}
		for p.at(Dot) {
			p.advance()
			tn.Module = qual(tn.Module, tn.Name)
			tn.Name = p.expect(UIdent, "type name").Text
		}
		tn.Span = p.spanFrom(start)
		return tn
	case p.at(LIdent):
		t := p.advance()
		tv := &TypeVar{Name: t.Text}
		tv.Span = p.spanTok(t)
		return tv
	case p.at(LBrace):
		return p.parseRecordType()
	case p.at(LParen):
		p.advance()
		if p.at(RParen) {
			p.advance()
			tn := &TypeName{Name: "Unit"}
			tn.Span = p.spanFrom(start)
			return tn
		}
		inner := p.parseType()
		p.expect(RParen, "`)`")
		return inner
	}
	p.errorAt(p.tok, "expected a type, got "+describe(p.tok))
	tn := &TypeName{Name: "?"}
	tn.Span = p.spanTok(p.tok)
	return tn
}

func (p *Parser) parseRecordType() *RecordType {
	start := p.tok
	p.expect(LBrace, "`{`")
	rt := &RecordType{Fields: p.parseFieldList(RBrace, false)}
	rt.Span = p.spanFrom(start)
	return rt
}
```

`parseExpr` is defined in Task 5. To compile Task 4 alone, add a temporary `parser_expr.go` containing only:

```go
package syntax
func (p *Parser) parseExpr() Expr { p.errorAt(p.tok, "expressions not implemented"); p.advance(); return &UnitLit{} }
```

and replace it in Task 5. For Task 4's golden, the expected `.dump` above includes expressions; run Task 4's test with `-update` only after Task 5, and in Task 4 verify by running the test and checking that only expression-related diagnostics appear (`! E0001 ... expressions not implemented`).

- [ ] **Step 5: Run the golden test**

Run: `go test ./internal/syntax/ -run TestParseGolden`
Expected: FAIL on mismatch only because of the expression stub; the declaration structure in the `got:` output must match the `want:` output everywhere except expression positions. If a declaration is misparsed, fix the parser now.

- [ ] **Step 6: Commit**

```bash
git add internal/syntax testdata/golden && git commit -m "m1: parser for modules, declarations and types"
```

---

### Task 5: Parser — expressions, patterns, blocks, strings, HTML

**Files:**
- Replace: `internal/syntax/parser_expr.go`
- Create: `internal/syntax/parser_html.go`, `testdata/golden/parse_exprs.alder` + `.dump`, `testdata/golden/parse_html.alder` + `.dump`, `testdata/golden/parse_errors.alder` + `.dump`

**Interfaces:**
- Produces: `p.parseExpr() Expr`, `p.parsePattern() Pattern`, `p.parseBlock() *Block`, `p.parseHTML() Expr`.
- Precedence, lowest to highest: `|>` (left), `??` (right), `||`, `&&`, comparison (non-assoc), `+ - ++`, `* / %`, application, postfix `.field`.
- Record literal vs block vs record update after `{`: look at the first tokens: `}` → empty record; `let` → block; `LIdent :` → record literal; otherwise parse an expression; if followed by `|` → record update; otherwise it's a block with that expression as result.
- Unary minus: only before a numeric literal (`-1`) — `-x` is not in the language; report E0001 "negation is not an operator; use `0 - x`".

- [ ] **Step 1: Write the golden inputs**

```alder
-- testdata/golden/parse_exprs.alder
module E

a : Int
a = 1 + 2 * 3 - 4 % 2

b : Bool
b = x < 3 && y >= 4 || not z

c : Text
c = "Hi {name}, {Text.fromInt n}!" ++ "\{x} \"q\""

d : List Int
d = [1, 2, 3] |> List.map (\x -> x + 1) |> List.filter (\x y -> x == y)

e : Int
e = m ?? 0

f : Point
f = { x: 1, y: 2 }

g : Point
g = { f | x: 10 }

h : Int
h = f.x

i : Int
i = if a then 1 else 2

j : Text
j = match s {
  Circle r -> "c",
  Rect { w: w, h: h } -> "r",
  Just (Just x) -> "nested",
  0 -> "zero",
  "s" -> "str",
  [] -> "empty",
  [x] -> "one",
  [x, ..rest] -> "many",
  _ -> "other",
}

k : Query Model
k = {
  let todos <- Db.from Todo |> Db.all
  let n : Int = List.length todos
  Query.succeed { todos: todos }
}

l : Int
l = ?count

m : Unit
m = ()

n : Todos.Show
n = Todos.Show { id: id }

o : Msg
o = Toggle { id: props.todo.id }

q : Int
q = -1
```

```
(module E (val a : Int = (- (+ (int 1) (* (int 2) (int 3))) (% (int 4) (int 2)))) (val b : Bool = (|| (&& (< (var x) (int 3)) (>= (var y) (int 4))) (app (var not) (var z)))) (val c : Text = (++ (str "Hi " (var name) ", " (app (var Text.fromInt) (var n)) "!") (str "{x} \"q\""))) (val d : (List Int) = (|> (|> (list (int 1) (int 2) (int 3)) (app (var List.map) (lambda x -> (+ (var x) (int 1))))) (app (var List.filter) (lambda x y -> (== (var x) (var y)))))) (val e : Int = (?? (var m) (int 0))) (val f : Point = (record (x (int 1)) (y (int 2)))) (val g : Point = (update (var f) (x (int 10)))) (val h : Int = (access (var f) x)) (val i : Int = (if (var a) (int 1) (int 2))) (val j : Text = (match (var s) (arm (pctor Circle r) (str "c")) (arm (pctor Rect (prec (w w) (h h))) (str "r")) (arm (pctor Just (pctor Just x)) (str "nested")) (arm (int 0) (str "zero")) (arm (str "s") (str "str")) (arm (plist) (str "empty")) (arm (plist x) (str "one")) (arm (plist x ..rest) (str "many")) (arm _ (str "other")))) (val k : (Query Model) = (block (let todos <- (|> (app (var Db.from) (ctor Todo)) (var Db.all))) (let n : Int = (app (var List.length) (var todos))) (app (var Query.succeed) (record (todos (var todos)))))) (val l : Int = (hole count)) (val m : Unit = (unit)) (val n : Todos.Show = (app (ctor Todos.Show) (record (id (var id))))) (val o : Msg = (app (ctor Toggle) (record (id (access (access (var props) todo) id))))) (val q : Int = (int -1)))
```

Note `(str "{x} \"q\"")`: `\{` unescapes to `{` in the part text. The `Just (Just x)` pattern is accepted by the parser; the checker rejects nesting deeper than one level (chapter 3) — keeping the parser permissive gives better error messages.

```alder
-- testdata/golden/parse_html.alder
module H

view : Model -> Session -> Html Msg
view model session =
  <Layout.Page title="Todos" theme={session.theme}>
    <h1>Todos</h1>
    -- a comment between elements
    <form submit={Submit}>
      <Errors form />
      <Input field="title" label="What?" />
      <button disabled={model.saving}>Add</button>
    </form>
    <ul id="todos">
      {for todo in model.todos -> <Item todo={todo} />}
      {if model.todos == [] then <p>Nothing</p> else Html.none}
      {match model.user {
        Just u -> <span>{u.name}</span>,
        Nothing -> <a href={Auth.Login {}}>Log in</a>,
      }}
    </ul>
    <p class="n">{Text.fromInt n} open, a < b</p>
    <br />
  </Layout.Page>
```

```
(module H (val view : (fn Model Session -> (Html Msg)) model session = (el Layout.Page (attr title (str "Todos")) (attr theme (access (var session) theme)) (el h1 (text "Todos")) (el form (attr submit (ctor Submit)) (el Errors (attr form)) (el Input (attr field (str "title")) (attr label (str "What?"))) (el button (attr disabled (access (var model) saving)) (text "Add"))) (el ul (attr id (str "todos")) (for todo (access (var model) todos) (el Item (attr todo (var todo)))) (hif (== (access (var model) todos) (list)) (el p (text "Nothing")) (expr (var Html.none))) (hmatch (access (var model) user) (arm (pctor Just u) (el span (expr (access (var u) name)))) (arm (pctor Nothing) (el a (attr href (app (ctor Auth.Login) (record))) (text "Log in"))))) (el p (attr class (str "n")) (expr (app (var Text.fromInt) (var n))) (text " open, a < b")) (el br)))
```

HTML text rules for the parser: text between elements is trimmed of leading/trailing whitespace that touches a newline, collapsed internally to single spaces, and dropped if empty; a line whose first non-space characters are `--` inside an element is a comment. Text containing `<` followed by a space (` a < b`) is text: in `ScanText` mode `<` ends text only when followed by a letter, `/`, or `!`. **Amend the lexer**: `ScanText` stops at `{`, or at `<` when the following byte is a letter, `/` or `!`; otherwise `<` is text. Add `TestLexer_ScanTextLessThan` covering `"a < b<p>"` → text `"a < b"`.

```alder
-- testdata/golden/parse_errors.alder
module Err

a : Int
a = case x of

b : Int
b = let y = 1 in y

c : Int
c = 2

d : Int
d = (1, 2)
```

```
(module Err (val a : Int = (unit)) (val b : Int = (unit)) (val c : Int = (int 2)) (val d : Int = (unit)))
! E0602 4:5 `case ... of` is not Alder; use `match x { ... }`
! E0602 7:5 `let ... in` is not Alder; use a block: `{ let y = 1  ...  }`
! E0602 13:5 tuples are not Alder; use a record `{ a: 1, b: 2 }`
```

`c` must still parse correctly after the errors above (recovery).

- [ ] **Step 2: Run to verify failures**

Run: `go test ./internal/syntax/ -run TestParseGolden`
Expected: FAIL for all three new goldens (stub parser).

- [ ] **Step 3: Implement parser_expr.go**

```go
// internal/syntax/parser_expr.go
package syntax

import (
	"strconv"
	"strings"

	"alder/internal/diag"
)

func (p *Parser) parseExpr() Expr { return p.parsePipe() }

func (p *Parser) binary(start Token, op string, l, r Expr) Expr {
	b := &BinOp{Op: op, L: l, R: r}
	b.Span = p.spanFrom(start)
	return b
}

// continuation: an operator may begin the next line.
func (p *Parser) atOpAfterNL(kinds ...Kind) bool {
	save, saveTok := p.lx.Pos(), p.tok
	p.skipNL()
	for _, k := range kinds {
		if p.at(k) {
			return true
		}
	}
	p.lx.Reset(save)
	p.tok = saveTok
	return false
}

func (p *Parser) parsePipe() Expr {
	start := p.tok
	l := p.parseDefault()
	for p.at(PipeGt) || (p.at(NL) && p.atOpAfterNL(PipeGt)) {
		p.advance()
		p.skipNL()
		l = p.binary(start, "|>", l, p.parseDefault())
	}
	return l
}

func (p *Parser) parseDefault() Expr {
	start := p.tok
	l := p.parseOr()
	if p.at(Question2) {
		p.advance()
		p.skipNL()
		return p.binary(start, "??", l, p.parseDefault())
	}
	return l
}

func (p *Parser) parseOr() Expr {
	start := p.tok
	l := p.parseAnd()
	for p.at(OrOr) || (p.at(NL) && p.atOpAfterNL(OrOr)) {
		p.advance()
		p.skipNL()
		l = p.binary(start, "||", l, p.parseAnd())
	}
	return l
}

func (p *Parser) parseAnd() Expr {
	start := p.tok
	l := p.parseCmp()
	for p.at(AndAnd) || (p.at(NL) && p.atOpAfterNL(AndAnd)) {
		p.advance()
		p.skipNL()
		l = p.binary(start, "&&", l, p.parseCmp())
	}
	return l
}

var cmpOps = map[Kind]string{EqEq: "==", NotEq: "!=", Lt: "<", Le: "<=", Gt: ">", Ge: ">="}

func (p *Parser) parseCmp() Expr {
	start := p.tok
	l := p.parseAdd()
	if op, ok := cmpOps[p.tok.Kind]; ok {
		p.advance()
		p.skipNL()
		r := p.parseAdd()
		if _, again := cmpOps[p.tok.Kind]; again {
			p.errorAt(p.tok, "comparisons do not chain; use `&&`")
		}
		return p.binary(start, op, l, r)
	}
	return l
}

func (p *Parser) parseAdd() Expr {
	start := p.tok
	l := p.parseMul()
	for p.at(Plus) || p.at(Minus) || p.at(PlusPlus) || (p.at(NL) && p.atOpAfterNL(Plus, Minus, PlusPlus)) {
		op := p.advance().Text
		p.skipNL()
		l = p.binary(start, op, l, p.parseMul())
	}
	return l
}

func (p *Parser) parseMul() Expr {
	start := p.tok
	l := p.parseApp()
	for p.at(Star) || p.at(Slash) || p.at(Percent) {
		op := p.advance().Text
		p.skipNL()
		l = p.binary(start, op, l, p.parseApp())
	}
	return l
}

// parseApp: primary {primary}; arguments continue while the next token can start a primary on the same line.
func (p *Parser) parseApp() Expr {
	start := p.tok
	fn := p.parsePostfix()
	var args []Expr
	for p.canStartArg() {
		args = append(args, p.parsePostfix())
	}
	if len(args) == 0 {
		return fn
	}
	a := &App{Fn: fn, Args: args}
	a.Span = p.spanFrom(start)
	return a
}

func (p *Parser) canStartArg() bool {
	switch p.tok.Kind {
	case LIdent, UIdent, Int, Float, String, LParen, LBracket, LBrace, Hole:
		return true
	case Backslash:
		return true
	case Keyword:
		return false
	}
	return false
}

func (p *Parser) parsePostfix() Expr {
	start := p.tok
	e := p.parsePrimary()
	for p.at(Dot) {
		p.advance()
		f := p.expect(LIdent, "field name")
		a := &Access{Expr: e, Field: f.Text}
		a.Span = p.spanFrom(start)
		e = a
	}
	return e
}

func (p *Parser) parsePrimary() Expr {
	start := p.tok
	switch p.tok.Kind {
	case Int:
		t := p.advance()
		v, _ := strconv.ParseInt(t.Text, 10, 64)
		e := &IntLit{Value: v}
		e.Span = p.spanTok(t)
		return e
	case Float:
		t := p.advance()
		v, _ := strconv.ParseFloat(t.Text, 64)
		e := &FloatLit{Value: v}
		e.Span = p.spanTok(t)
		return e
	case Minus: // -1 only
		p.advance()
		if p.at(Int) || p.at(Float) {
			lit := p.parsePrimary()
			switch l := lit.(type) {
			case *IntLit:
				l.Value = -l.Value
			case *FloatLit:
				l.Value = -l.Value
			}
			return lit
		}
		p.errorAt(start, "negation is not an operator; use `0 - x`")
		return p.parsePrimary()
	case String:
		return p.parseString(p.advance())
	case Hole:
		t := p.advance()
		e := &HoleExpr{Name: t.Text}
		e.Span = p.spanTok(t)
		return e
	case LIdent:
		t := p.advance()
		if t.Text == "not" { // `not` is a function; nothing special
		}
		e := &Var{Name: t.Text}
		e.Span = p.spanTok(t)
		return e
	case UIdent:
		return p.parseQualified()
	case LParen:
		p.advance()
		if p.at(RParen) {
			p.advance()
			e := &UnitLit{}
			e.Span = p.spanFrom(start)
			return e
		}
		p.skipNL()
		inner := p.parseExpr()
		p.skipNL()
		if p.at(Comma) {
			p.errorAt(start, "tuples are not Alder; use a record `{ a: 1, b: 2 }`")
			for !p.at(RParen) && !p.at(EOF) && !p.at(NL) {
				p.advance()
			}
			if p.at(RParen) {
				p.advance()
			}
			e := &UnitLit{}
			e.Span = p.spanFrom(start)
			p.diags[len(p.diags)-1].Code = "E0602"
			return e
		}
		p.expect(RParen, "`)`")
		return inner
	case LBracket:
		p.advance()
		e := &ListLit{}
		p.skipNL()
		for !p.at(RBracket) && !p.at(EOF) {
			e.Elems = append(e.Elems, p.parseExpr())
			p.skipNL()
			if p.at(Comma) {
				p.advance()
				p.skipNL()
			} else {
				break
			}
		}
		p.expect(RBracket, "`]`")
		e.Span = p.spanFrom(start)
		return e
	case LBrace:
		return p.parseBraces()
	case Backslash:
		p.advance()
		l := &Lambda{}
		for p.at(LIdent) {
			t := p.advance()
			id := &Ident{Name: t.Text}
			id.Span = p.spanTok(t)
			l.Params = append(l.Params, id)
		}
		p.expect(Arrow, "`->`")
		p.skipNL()
		l.Body = p.parseExpr()
		l.Span = p.spanFrom(start)
		return l
	case Lt:
		return p.parseHTML()
	case Keyword:
		switch p.tok.Text {
		case "if":
			p.advance()
			e := &If{Cond: p.parseExpr()}
			p.skipNL()
			p.expectKeyword("then")
			p.skipNL()
			e.Then = p.parseExpr()
			p.skipNL()
			p.expectKeyword("else")
			p.skipNL()
			e.Else = p.parseExpr()
			e.Span = p.spanFrom(start)
			return e
		case "match":
			p.advance()
			e := &Match{Subject: p.parseExpr()}
			p.skipNL()
			p.expect(LBrace, "`{`")
			e.Arms = p.parseArms()
			e.Span = p.spanFrom(start)
			return e
		case "let":
			p.errorAt(start, "`let ... in` is not Alder; use a block: `{ let y = 1  ...  }`")
			p.diags[len(p.diags)-1].Code = "E0602"
			p.skipToDeclStayingOnLine()
			e := &UnitLit{}
			e.Span = p.spanFrom(start)
			return e
		}
	}
	if p.at(LIdent) && p.tok.Text == "case" { // unreachable (LIdent handled above) but kept for clarity
	}
	p.errorAt(p.tok, "expected an expression, got "+describe(p.tok))
	p.advance()
	e := &UnitLit{}
	e.Span = p.spanFrom(start)
	return e
}

// `case` is not a keyword, so `case x of` lexes as LIdent(case). Detect it in parseApp's caller:
// parseValueDecl/parseBlock call parseExpr; we intercept here by checking Var name "case" followed by ... "of".
// Implemented as a post-check in parseApp:
func (p *Parser) checkCaseOf(e Expr, start Token) Expr {
	if a, ok := e.(*App); ok {
		if v, ok := a.Fn.(*Var); ok && v.Name == "case" {
			if last, ok := a.Args[len(a.Args)-1].(*Var); ok && last.Name == "of" {
				p.errorAt(start, "`case ... of` is not Alder; use `match x { ... }`")
				p.diags[len(p.diags)-1].Code = "E0602"
				p.skipToDeclStayingOnLine()
				u := &UnitLit{}
				u.Span = p.spanFrom(start)
				return u
			}
		}
	}
	return e
}

// skipToDeclStayingOnLine consumes the rest of the current logical expression (to the next blank line or EOF).
func (p *Parser) skipToDeclStayingOnLine() {
	for !p.at(EOF) {
		if p.at(NL) {
			save, saveTok := p.lx.Pos(), p.tok
			p.advance()
			if p.at(NL) || p.at(EOF) {
				p.lx.Reset(save)
				p.tok = saveTok
				return
			}
			continue
		}
		p.advance()
	}
}

func (p *Parser) parseQualified() Expr {
	start := p.tok
	t := p.advance()
	mod, name := "", t.Text
	for p.at(Dot) {
		save, saveTok := p.lx.Pos(), p.tok
		p.advance()
		if p.at(UIdent) {
			mod = qual(mod, name)
			name = p.advance().Text
			continue
		}
		if p.at(LIdent) { // Module.value
			v := &Var{Module: qual(mod, name), Name: p.advance().Text}
			v.Span = p.spanFrom(start)
			return v
		}
		p.lx.Reset(save)
		p.tok = saveTok
		break
	}
	c := &CtorRef{Module: mod, Name: name}
	c.Span = p.spanFrom(start)
	return c
}

// parseBraces: record literal, record update, or block.
func (p *Parser) parseBraces() Expr {
	start := p.tok
	p.expect(LBrace, "`{`")
	p.skipNL()
	if p.at(RBrace) {
		p.advance()
		e := &RecordLit{}
		e.Span = p.spanFrom(start)
		return e
	}
	if p.atKeyword("let") {
		return p.parseBlockBody(start)
	}
	if p.at(LIdent) {
		save, saveTok := p.lx.Pos(), p.tok
		p.advance()
		isField := p.at(Colon)
		p.lx.Reset(save)
		p.tok = saveTok
		if isField {
			e := &RecordLit{Fields: p.parseFieldInits()}
			e.Span = p.spanFrom(start)
			return e
		}
	}
	first := p.parseExpr()
	p.skipNL()
	if p.at(Pipe) {
		p.advance()
		p.skipNL()
		e := &RecordUpdate{Base: first, Fields: p.parseFieldInits()}
		e.Span = p.spanFrom(start)
		return e
	}
	// block with a single result expression
	p.expect(RBrace, "`}`")
	b := &Block{Result: first}
	b.Span = p.spanFrom(start)
	return b
}

// parseFieldInits parses `name: expr, …}` and consumes the closing brace.
func (p *Parser) parseFieldInits() []*FieldInit {
	var fs []*FieldInit
	for {
		p.skipNL()
		if p.at(RBrace) {
			p.advance()
			return fs
		}
		ft := p.expect(LIdent, "field name")
		if !p.at(Colon) {
			p.errorAt(p.tok, "record fields are written `name: value`; there is no punning")
		} else {
			p.advance()
		}
		p.skipNL()
		fi := &FieldInit{Name: ft.Text, Value: p.parseExpr()}
		fi.Span = p.spanFrom(ft)
		fs = append(fs, fi)
		p.skipNL()
		if p.at(Comma) {
			p.advance()
			continue
		}
		p.skipNL()
		p.expect(RBrace, "`}` or `,`")
		return fs
	}
}

func (p *Parser) parseBlockBody(start Token) Expr {
	b := &Block{}
	for p.atKeyword("let") {
		lt := p.advance()
		s := &LetStmt{Name: p.expectNameOrWild()}
		if p.at(Colon) {
			p.advance()
			s.Type = p.parseType()
		}
		if p.at(LeftArrow) {
			s.Bind = true
			p.advance()
		} else {
			p.expect(Equal, "`=` or `<-`")
		}
		p.skipNL()
		s.Expr = p.parseExpr()
		s.Span = p.spanFrom(lt)
		b.Stmts = append(b.Stmts, s)
		if !p.at(NL) && !p.at(RBrace) {
			p.errorAt(p.tok, "expected end of line after `let` statement")
		}
		p.skipNL()
	}
	if p.at(RBrace) {
		p.errorAt(p.tok, "a block must end with an expression")
	} else {
		b.Result = p.parseExpr()
	}
	p.skipNL()
	p.expect(RBrace, "`}`")
	b.Span = p.spanFrom(start)
	return b
}

func (p *Parser) expectNameOrWild() string {
	if p.at(Underscore) {
		p.advance()
		return "_"
	}
	return p.expect(LIdent, "name").Text
}

func (p *Parser) parseArms() []*Arm {
	var arms []*Arm
	for {
		p.skipNL()
		if p.at(RBrace) {
			p.advance()
			return arms
		}
		if p.at(EOF) {
			p.errorAt(p.tok, "unterminated match")
			return arms
		}
		start := p.tok
		a := &Arm{Pat: p.parsePattern()}
		p.expect(Arrow, "`->`")
		p.skipNL()
		a.Body = p.parseExpr()
		a.Span = p.spanFrom(start)
		arms = append(arms, a)
		p.skipNL()
		if p.at(Comma) {
			p.advance()
			continue
		}
		p.skipNL()
		p.expect(RBrace, "`,` or `}`")
		return arms
	}
}

// ---- patterns

func (p *Parser) parsePattern() Pattern {
	start := p.tok
	switch p.tok.Kind {
	case Underscore:
		p.advance()
		w := &PWild{}
		w.Span = p.spanTok(start)
		return w
	case LIdent:
		t := p.advance()
		v := &PVar{Name: t.Text}
		v.Span = p.spanTok(t)
		return v
	case Int, String, Minus:
		lit := p.parsePrimary()
		if b, ok := lit.(*Var); ok { // True/False lex as UIdent, handled below; this catches nothing else
			_ = b
		}
		l := &PLit{Lit: lit}
		l.Span = p.spanFrom(start)
		return l
	case UIdent:
		t := p.advance()
		if t.Text == "True" || t.Text == "False" {
			l := &PLit{Lit: &BoolLit{Value: t.Text == "True"}}
			l.Span = p.spanTok(t)
			return l
		}
		c := &PCtor{Name: t.Text}
		if p.at(LIdent) || p.at(Underscore) || p.at(LBrace) || p.at(LParen) || p.at(UIdent) || p.at(Int) || p.at(String) {
			c.Arg = p.parsePattern1()
		}
		c.Span = p.spanFrom(start)
		return c
	case LBrace:
		return p.parseRecordPattern()
	case LBracket:
		p.advance()
		l := &PList{}
		for !p.at(RBracket) && !p.at(EOF) {
			if p.at(DotDot) {
				p.advance()
				l.Rest = p.expect(LIdent, "rest name").Text
				break
			}
			l.Elems = append(l.Elems, p.parsePattern1())
			if p.at(Comma) {
				p.advance()
			} else {
				break
			}
		}
		p.expect(RBracket, "`]`")
		l.Span = p.spanFrom(start)
		return l
	case LParen:
		p.advance()
		inner := p.parsePattern()
		p.expect(RParen, "`)`")
		return inner
	}
	p.errorAt(p.tok, "expected a pattern, got "+describe(p.tok))
	p.advance()
	w := &PWild{}
	w.Span = p.spanTok(start)
	return w
}

// parsePattern1: a constructor argument — no bare constructor application (needs parens).
func (p *Parser) parsePattern1() Pattern {
	if p.at(UIdent) {
		t := p.advance()
		if t.Text == "True" || t.Text == "False" {
			l := &PLit{Lit: &BoolLit{Value: t.Text == "True"}}
			l.Span = p.spanTok(t)
			return l
		}
		c := &PCtor{Name: t.Text}
		c.Span = p.spanTok(t)
		return c
	}
	return p.parsePattern()
}

func (p *Parser) parseRecordPattern() Pattern {
	start := p.tok
	p.expect(LBrace, "`{`")
	r := &PRecord{}
	for {
		p.skipNL()
		if p.at(RBrace) {
			p.advance()
			break
		}
		ft := p.expect(LIdent, "field name")
		f := &PField{Name: ft.Text}
		if p.at(Colon) {
			p.advance()
			f.Pat = p.parsePattern1()
		} else {
			p.errorAt(p.tok, "record patterns are written `{ field: pattern }`")
			f.Pat = &PWild{}
		}
		f.Span = p.spanFrom(ft)
		r.Fields = append(r.Fields, f)
		p.skipNL()
		if p.at(Comma) {
			p.advance()
			continue
		}
		p.skipNL()
		p.expect(RBrace, "`}`")
		break
	}
	r.Span = p.spanFrom(start)
	return r
}

// ---- strings

// parseString splits a raw literal (with quotes) into literal and interpolated parts.
func (p *Parser) parseString(t Token) Expr {
	raw := t.Text
	if len(raw) >= 2 && raw[len(raw)-1] == '"' {
		raw = raw[1 : len(raw)-1]
	} else if len(raw) >= 1 {
		raw = raw[1:]
	}
	s := &StrLit{}
	s.Span = p.spanTok(t)
	var buf strings.Builder
	flush := func() {
		if buf.Len() > 0 {
			s.Parts = append(s.Parts, StrPart{Text: buf.String()})
			buf.Reset()
		}
	}
	for i := 0; i < len(raw); i++ {
		c := raw[i]
		switch {
		case c == '\\' && i+1 < len(raw):
			i++
			switch raw[i] {
			case 'n':
				buf.WriteByte('\n')
			case 't':
				buf.WriteByte('\t')
			case '"', '\\', '{':
				buf.WriteByte(raw[i])
			default:
				p.errorAt(t, "unknown escape \\"+string(raw[i]))
			}
		case c == '{':
			depth, j := 1, i+1
			for j < len(raw) && depth > 0 {
				if raw[j] == '{' {
					depth++
				} else if raw[j] == '}' {
					depth--
				}
				j++
			}
			if depth != 0 {
				p.errorAt(t, "unterminated `{` in string")
				return s
			}
			flush()
			inner := raw[i+1 : j-1]
			sub := newParser(p.file, []byte(inner), p.sigMode)
			// position the sub-parser's lexer positions relative to the string start
			sub.lx.line, sub.lx.col = t.Line, t.Col+1+i+1
			sub.advance()
			e := sub.parseExpr()
			if !sub.at(EOF) {
				sub.errorAt(sub.tok, "unexpected token in interpolation")
			}
			p.diags = append(p.diags, sub.diags...)
			s.Parts = append(s.Parts, StrPart{Expr: e})
			i = j - 1
		default:
			buf.WriteByte(c)
		}
	}
	flush()
	if len(s.Parts) == 0 {
		s.Parts = []StrPart{{Text: ""}}
	}
	return s
}
```

Then wire `checkCaseOf` into `parseApp`: replace `return a` at the end of `parseApp` with `return p.checkCaseOf(a, start)`.

Note on `sub.lx.line/col`: the sub-lexer is constructed with `newParser`, which calls `advance()` once before positions are adjusted; set `sub.lx.line, sub.lx.col` **before** the first advance by constructing with `&Parser{lx: NewLexer(...)}` directly and then calling `sub.lx.line = …; sub.advance()`. Write it that way.

- [ ] **Step 4: Implement parser_html.go**

```go
// internal/syntax/parser_html.go
package syntax

import "strings"

// parseHTML is entered with p.tok == Lt at expression start.
func (p *Parser) parseHTML() Expr {
	start := p.tok
	p.expect(Lt, "`<`")
	el := &Element{}
	p.lx.SetHTMLTag(true)
	defer p.lx.SetHTMLTag(false)
	tagTok := p.tok
	if p.at(UIdent) {
		p.advance()
		el.Tag = tagTok.Text
		for p.at(Dot) {
			p.advance()
			el.Module = qual(el.Module, el.Tag)
			el.Tag = p.expect(UIdent, "component name").Text
		}
	} else {
		el.Tag = p.expect(LIdent, "tag name").Text
	}
	// attributes
	for p.at(LIdent) {
		at := p.advance()
		a := &HtmlAttr{Name: at.Text}
		if p.at(Equal) {
			p.advance()
			if p.at(String) {
				a.Value = p.parseString(p.advance())
			} else {
				p.expect(LBrace, "`{` or a string")
				p.lx.SetHTMLTag(false)
				p.skipNL()
				a.Value = p.parseExpr()
				p.skipNL()
				p.expect(RBrace, "`}`")
				p.lx.SetHTMLTag(true)
			}
		}
		a.Span = p.spanFrom(at)
		el.Attrs = append(el.Attrs, a)
	}
	if p.at(Slash) { // self-closing
		p.advance()
		p.expect(Gt, "`>`")
		el.Span = p.spanFrom(start)
		return el
	}
	p.expect(Gt, "`>`")
	p.lx.SetHTMLTag(false)
	// children: raw text until `<` or `{`
	for {
		tx := p.lx.ScanText()
		p.addText(el, tx)
		p.advance() // now p.tok is `<` or `{` or EOF
		switch {
		case p.at(EOF):
			p.errorAt(start, "unclosed element <"+el.Tag+">")
			el.Span = p.spanFrom(start)
			return el
		case p.at(LBrace):
			ltok := p.tok
			p.advance()
			p.skipNL()
			child := p.parseHTMLBrace(ltok)
			el.Children = append(el.Children, child)
			p.skipNL()
			p.expect(RBrace, "`}`")
		case p.at(Lt):
			// closing tag?
			save, saveTok := p.lx.Pos(), p.tok
			p.advance()
			if p.at(Slash) {
				p.advance()
				p.lx.SetHTMLTag(true)
				name := p.tok.Text
				p.advance()
				for p.at(Dot) {
					p.advance()
					name += "." + p.advance().Text
				}
				p.lx.SetHTMLTag(false)
				p.expect(Gt, "`>`")
				if name != qual(el.Module, el.Tag) {
					p.errorAt(saveTok, "closing tag </"+name+"> does not match <"+qual(el.Module, el.Tag)+">")
				}
				el.Span = p.spanFrom(start)
				return el
			}
			p.lx.Reset(save)
			p.tok = saveTok
			el.Children = append(el.Children, p.parseHTML())
		}
	}
}

// addText splits raw text into lines, drops `--` comment lines, collapses whitespace, and appends an HtmlText if non-empty.
func (p *Parser) addText(el *Element, tx Token) {
	lines := strings.Split(tx.Text, "\n")
	var kept []string
	for _, ln := range lines {
		if strings.HasPrefix(strings.TrimSpace(ln), "--") {
			continue
		}
		kept = append(kept, ln)
	}
	text := strings.Join(kept, "\n")
	if strings.TrimSpace(text) == "" {
		return
	}
	// trim whitespace that touches a newline at either end; collapse inner whitespace runs
	if i := strings.IndexByte(text, '\n'); i >= 0 && strings.TrimSpace(text[:i]) == "" {
		text = strings.TrimLeft(text, " \t\n")
	}
	if i := strings.LastIndexByte(text, '\n'); i >= 0 && strings.TrimSpace(text[i:]) == "" {
		text = strings.TrimRight(text, " \t\n")
	}
	text = strings.Join(strings.Fields(text), " ")
	if strings.HasPrefix(tx.Text, " ") && !strings.HasPrefix(text, " ") && !strings.ContainsRune(tx.Text[:1], '\n') {
		text = " " + text
	}
	if strings.HasSuffix(tx.Text, " ") && !strings.HasSuffix(text, " ") {
		text = text + " "
	}
	t := &HtmlText{Text: text}
	t.Span = p.spanTok(tx)
	el.Children = append(el.Children, t)
}

// parseHTMLBrace is entered after `{`; p.tok is the first token inside.
func (p *Parser) parseHTMLBrace(ltok Token) Expr {
	switch {
	case p.atKeyword("for"):
		p.advance()
		f := &HtmlFor{Var: p.expect(LIdent, "loop variable").Text}
		p.expectKeyword("in")
		f.List = p.parseExpr()
		p.expect(Arrow, "`->`")
		p.skipNL()
		f.Body = p.parseHTMLChild()
		f.Span = p.spanFrom(ltok)
		return f
	case p.atKeyword("if"):
		p.advance()
		h := &HtmlIf{Cond: p.parseExpr()}
		p.skipNL()
		p.expectKeyword("then")
		p.skipNL()
		h.Then = p.parseHTMLChild()
		p.skipNL()
		p.expectKeyword("else")
		p.skipNL()
		h.Else = p.parseHTMLChild()
		h.Span = p.spanFrom(ltok)
		return h
	case p.atKeyword("match"):
		p.advance()
		m := &HtmlMatch{Subject: p.parseExpr()}
		p.skipNL()
		p.expect(LBrace, "`{`")
		for _, a := range p.parseArmsHTML() {
			m.Arms = append(m.Arms, a)
		}
		m.Span = p.spanFrom(ltok)
		return m
	default:
		e := &HtmlExpr{Expr: p.parseExpr()}
		e.Span = p.spanFrom(ltok)
		return e
	}
}

// parseHTMLChild: an element, or an expression (wrapped in HtmlExpr).
func (p *Parser) parseHTMLChild() Expr {
	if p.at(Lt) {
		return p.parseHTML()
	}
	start := p.tok
	e := &HtmlExpr{Expr: p.parseExpr()}
	e.Span = p.spanFrom(start)
	return e
}

func (p *Parser) parseArmsHTML() []*Arm {
	var arms []*Arm
	for {
		p.skipNL()
		if p.at(RBrace) {
			p.advance()
			return arms
		}
		if p.at(EOF) {
			p.errorAt(p.tok, "unterminated match")
			return arms
		}
		start := p.tok
		a := &Arm{Pat: p.parsePattern()}
		p.expect(Arrow, "`->`")
		p.skipNL()
		a.Body = p.parseHTMLChild()
		a.Span = p.spanFrom(start)
		arms = append(arms, a)
		p.skipNL()
		if p.at(Comma) {
			p.advance()
			continue
		}
		p.skipNL()
		p.expect(RBrace, "`,` or `}`")
		return arms
	}
}
```

Also amend the lexer's `ScanText` per the note above (stop at `<` only when followed by a letter, `/`, or `!`), and add `TestLexer_ScanTextLessThan` and `TestParse_UnclosedElement` (source `module U\nv : Html a\nv = <div><p>x</div>` must yield exactly one E0001 "closing tag </div> does not match <p>" plus one "unclosed element <div>", and no panic).

- [ ] **Step 5: Run the goldens, iterate until they match exactly**

Run: `go test ./internal/syntax/`
Expected: PASS for all four `parse_*` goldens and the unit tests. Do not use `-update` to make a failing golden pass: the `.dump` files above are the specification. If an output differs and the parser is right and the golden is wrong, fix the golden by hand and say which in the commit.

- [ ] **Step 6: Commit**

```bash
git add internal/syntax testdata/golden && git commit -m "m1: parser for expressions, patterns, blocks, strings and HTML"
```

---

### Task 6: Names — module graph and resolution

**Files:**
- Create: `internal/names/graph.go`, `internal/names/resolve.go`, `internal/names/names_test.go`

**Interfaces:**
- Produces:
  ```go
  package names
  type Program struct {
      Modules  map[string]*syntax.Module   // by module name
      Order    []string                    // topological (dependencies first)
      Pages    map[string]*syntax.PageDecl // page module name → decl
      AppMsgs  map[string]*syntax.MsgDecl  // "Logout" → decl (from App)
      Std      map[string]*syntax.Module   // stdlib modules by name (from Task 7's embedded files)
  }
  func Load(dir string, std map[string]*syntax.Module) (*Program, *diag.Bag)      // reads app/**/*.alder, parses, builds graph
  func Build(mods []*syntax.Module, std map[string]*syntax.Module) (*Program, *diag.Bag) // from already-parsed modules (tests)
  func (p *Program) Resolve(bag *diag.Bag)   // fills every Ref in every module; reports E0002/E0202/E0203/E0201
  func (p *Program) IsPageModule(name string) bool
  type Scope struct{ /* parent *Scope; names map[string]syntax.Ref */ }
  ```
- Resolution rules implemented here (chapter 3):
  - Module `M` declares its top-level names. `import X` makes `X.name` and `X.Type` resolvable. Stdlib modules are always importable without `import`.
  - A `TypeName{Module:"", Name:N}`: if `N` is declared in this module → local; else if `N` is a module name imported or stdlib and that module declares a type/table `N` → that type (module-name rule); else if a stdlib prelude type (`Int`, `Text`, `List`, `Maybe`, `Result`, `Html`, `Query`, `Task`, `Nav`, `Error`, `Never`, `Unit`, `Bool`, `Float`, `Date`, `Time`, `Form`, `Table`, `Id`, `Insert`, `Select`, `Condition`, `Order`) → std; else if generated for this module (`Params`, `Msg`, `Model`, `Session`) → `RefType{Module: thisModule}`; else E0002 — and if some *imported* module declares `N`, emit **E0202** "type `N` belongs to module `X`; write `X.N`" with fix `X.N` instead.
  - A `TypeName{Module:"X", Name:N}`: `X` must be imported (or stdlib) → else **E0201** "module `X` is not imported" with fix `import X` at the end of the import block; then `N` must be declared in `X`.
  - A `Var{Module:"", Name:n}`: innermost scope (locals) → this module's values → stdlib prelude values (`not`) → E0002.
  - A `Var{Module:"X", Name:n}`: `X` imported or stdlib or a table module's column (`Todo.done` where `Todo` is a table in module `Todo`: `RefColumn`) or a capability (`Password.verify`: `RefCapability`) → else E0201.
  - A `CtorRef{Module:"", Name:C}`: this module's union ctors (including generated `Msg` ctors and `Just/Nothing/Ok/Err/True/False` from the prelude) → ctors of imported modules' unions (must be unique: duplicates are E0009 at the import) → if `C` is a page module name (global) → `RefPage` → else if `C` is a table name in scope → `RefTable` → E0002.
  - A `CtorRef{Module:"X", Name:C}`: if `X.C` is a page module (`Todos.Show`) → `RefPage`; if `X == "App"` and `C` is an app msg → `RefCtor{Module:"App"}`; if `X` is imported and `C` is a ctor there → **E0203** "constructors are not qualified; write `C`" with fix `C`; else E0002.
  - Element tags: lowercase → html; `Element{Module:"",Tag:C}` → component `C` in this module, else an imported module's component named like the module (`<Layout>`), else E0002; `Element{Module:"X",Tag:C}` → `X.C` with `X` imported.
  - Patterns: `PCtor` resolved like `CtorRef` without the page/table cases. `PVar`, lambda params, `let` names, `for` vars, function params bind locals in nested scopes.
  - Locals shadow top-level names; a top-level name shadowing a stdlib prelude name is allowed.
  - Import cycle: **E0006** at the first import in the cycle, message lists the cycle `A -> B -> A`. Unused import: **W0001**.
  - Module name must equal the path relative to `app/` with `/` → `.`, else **E0005**.

- [ ] **Step 1: Write the failing tests**

```go
// internal/names/names_test.go
package names

import (
	"strings"
	"testing"

	"alder/internal/diag"
	"alder/internal/syntax"
)

func parse(t *testing.T, file, src string) *syntax.Module {
	m, ds := syntax.ParseFile(file, []byte(src))
	if len(ds) > 0 {
		t.Fatalf("parse %s: %+v", file, ds)
	}
	return m
}

func codes(b *diag.Bag) string {
	var s []string
	for _, d := range b.Items {
		s = append(s, d.Code+":"+d.Message)
	}
	return strings.Join(s, "\n")
}

var prelude = map[string]*syntax.Module{} // filled by Task 7; tests here use syntax.ParseSignatures directly

func std(t *testing.T) map[string]*syntax.Module {
	m, _ := syntax.ParseSignatures("Prelude.alder", []byte("module Prelude\ntype Int\ntype Text\ntype Bool = True | False\ntype List a\ntype Maybe a = Just a | Nothing\ntype Html a\ntype Query a\ntype Task a\nnot : Bool -> Bool\n"))
	l, _ := syntax.ParseSignatures("List.alder", []byte("module List\nmap : (a -> b) -> List a -> List b\n"))
	return map[string]*syntax.Module{"Prelude": m, "List": l}
}

func TestResolve_Basics(t *testing.T) {
	todo := parse(t, "app/Todo.alder", "module Todo\n\ntype TodoId = brand Text\n\ntype Filter = All | Active\n\ntable Todo {\n  id: TodoId @id,\n  done: Bool = False,\n}\n")
	idx := parse(t, "app/Todos/Index.alder", "module Todos.Index\n\nimport Todo\n\npage \"/\" { filter: Todo.Filter = All }\n\ntype Model = { todos: List Todo }\n\nload : Params -> Session -> Query Model\nload params session = Query.succeed { todos: [] }\n\nview : Model -> Session -> Html Msg\nview model session = <ul>{for t in model.todos -> <li>{Todo.filterLabel All}</li>}</ul>\n\nf : Todo.Filter -> Bool\nf x = match x { All -> True, Active -> not False }\n")
	prog, bag := Build([]*syntax.Module{todo, idx}, std(t))
	prog.Resolve(bag)
	if bag.HasErrors() {
		t.Fatalf("unexpected: %s", codes(bag))
	}
	// Todo.filterLabel is unknown: must be reported
	if !strings.Contains(codes(bag), "E0002") {
		t.Fatalf("expected E0002 for Todo.filterLabel, got: %s", codes(bag))
	}
}

func TestResolve_QualificationErrors(t *testing.T) {
	todo := parse(t, "app/Todo.alder", "module Todo\n\ntype Priority = Low | High\n")
	idx := parse(t, "app/Todos/Index.alder", "module Todos.Index\n\nimport Todo\n\npage \"/\" { }\n\ntype Model = { p: Priority }\n\nx : Todo.Priority\nx = Todo.Low\n\ny : User.UserId\ny = ?y\n")
	prog, bag := Build([]*syntax.Module{todo, idx}, std(t))
	prog.Resolve(bag)
	got := codes(bag)
	for _, want := range []string{"E0202:type `Priority` belongs to module `Todo`; write `Todo.Priority`", "E0203:constructors are not qualified; write `Low`", "E0201:module `User` is not imported"} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in:\n%s", want, got)
		}
	}
	var fixes []string
	for _, d := range bag.Items {
		if d.Fix != nil {
			fixes = append(fixes, d.Fix.Replace)
		}
	}
	if strings.Join(fixes, "|") != "Todo.Priority|Low|import User" {
		t.Fatalf("fixes: %v", fixes)
	}
}

func TestResolve_ImportCycle(t *testing.T) {
	a := parse(t, "app/A.alder", "module A\n\nimport B\n\nx : Int\nx = 1\n")
	b := parse(t, "app/B.alder", "module B\n\nimport A\n\ny : Int\ny = A.x\n")
	c := parse(t, "app/C.alder", "module C\n\nz : Int\nz = 1\n")
	prog, bag := Build([]*syntax.Module{a, b, c}, std(t))
	prog.Resolve(bag)
	n := 0
	for _, d := range bag.Items {
		if d.Code == "E0006" {
			n++
			if !strings.Contains(d.Message, "A -> B -> A") && !strings.Contains(d.Message, "B -> A -> B") {
				t.Fatalf("cycle message: %s", d.Message)
			}
		}
	}
	if n != 1 {
		t.Fatalf("expected exactly one E0006, got %d: %s", n, codes(bag))
	}
	if _, ok := prog.Modules["C"]; !ok || len(prog.Order) == 0 {
		t.Fatal("other modules must still be processed")
	}
}

func TestResolve_GlobalsAndModulePath(t *testing.T) {
	app := parse(t, "app/App.alder", "module App\n\nsession { user: Maybe Text }\n\nlogin = Auth.Login\n\nmsg Logout\n\nupdate : Msg -> Session -> Task Nav\nupdate msg session = Task.succeed Rerender\n")
	login := parse(t, "app/Auth/Login.alder", "module Auth.Login\n\npage \"/login\" { }\n\ntype Model = { }\n\nload : Params -> Session -> Query Model\nload params session = Query.succeed { }\n\nview : Model -> Session -> Html Msg\nview model session = <a href={Auth.Login {}}><button click={App.Logout}>x</button></a>\n")
	wrong := parse(t, "app/Misc/Wrong.alder", "module Wrong\n\nx : Int\nx = 1\n")
	prog, bag := Build([]*syntax.Module{app, login, wrong}, std(t))
	prog.Resolve(bag)
	got := codes(bag)
	if !strings.Contains(got, "E0005") {
		t.Fatalf("expected E0005 for module/path mismatch: %s", got)
	}
	for _, d := range bag.Items {
		if d.Code != "E0005" && d.Severity == "error" {
			t.Fatalf("unexpected error: %s", codes(bag))
		}
	}
	if prog.AppMsgs["Logout"] == nil || prog.Pages["Auth.Login"] == nil {
		t.Fatal("globals not collected")
	}
}
```

- [ ] **Step 2: Run to verify failure**

Run: `go test ./internal/names/`
Expected: FAIL — undefined Build.

- [ ] **Step 3: Implement graph.go**

```go
// internal/names/graph.go
package names

import (
	"os"
	"path/filepath"
	"strings"

	"alder/internal/diag"
	"alder/internal/syntax"
)

type Program struct {
	Modules map[string]*syntax.Module
	Order   []string
	Pages   map[string]*syntax.PageDecl
	AppMsgs map[string]*syntax.MsgDecl
	Std     map[string]*syntax.Module
	scopes  map[string]*moduleScope
}

func Load(dir string, std map[string]*syntax.Module) (*Program, *diag.Bag) {
	bag := &diag.Bag{}
	var mods []*syntax.Module
	root := filepath.Join(dir, "app")
	filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.HasSuffix(path, ".alder") {
			return nil
		}
		src, err := os.ReadFile(path)
		if err != nil {
			return nil
		}
		rel, _ := filepath.Rel(dir, path)
		m, ds := syntax.ParseFile(filepath.ToSlash(rel), src)
		bag.Items = append(bag.Items, ds...)
		mods = append(mods, m)
		return nil
	})
	prog, b2 := Build(mods, std)
	bag.Items = append(bag.Items, b2.Items...)
	return prog, bag
}

func Build(mods []*syntax.Module, std map[string]*syntax.Module) (*Program, *diag.Bag) {
	bag := &diag.Bag{}
	p := &Program{Modules: map[string]*syntax.Module{}, Pages: map[string]*syntax.PageDecl{}, AppMsgs: map[string]*syntax.MsgDecl{}, Std: std, scopes: map[string]*moduleScope{}}
	for _, m := range mods {
		if prev, dup := p.Modules[m.Name]; dup {
			bag.Error(diag.Duplicate, m.Span, "module `%s` is also defined in %s", m.Name, prev.File)
			continue
		}
		p.Modules[m.Name] = m
		if want := expectedName(m.File); want != "" && want != m.Name {
			bag.Error(diag.ModulePath, m.Span, "module `%s` is in file %s; expected `module %s`", m.Name, m.File, want)
		}
		for _, d := range m.Decls {
			switch d := d.(type) {
			case *syntax.PageDecl:
				p.Pages[m.Name] = d
			case *syntax.MsgDecl:
				if m.Name == "App" {
					p.AppMsgs[d.Name] = d
				}
			}
		}
	}
	p.Order = p.topoSort(bag)
	return p, bag
}

// expectedName: "app/Todos/Index.alder" → "Todos.Index"; "" if the file is not under app/.
func expectedName(file string) string {
	file = filepath.ToSlash(file)
	i := strings.Index(file, "app/")
	if i < 0 {
		return ""
	}
	rest := strings.TrimSuffix(file[i+4:], ".alder")
	return strings.ReplaceAll(rest, "/", ".")
}

func (p *Program) IsPageModule(name string) bool { _, ok := p.Pages[name]; return ok }

// topoSort orders modules dependencies-first; reports one E0006 per cycle and still returns every module.
func (p *Program) topoSort(bag *diag.Bag) []string {
	const (white, grey, black = 0, 1, 2 )
	color := map[string]int{}
	var order []string
	var stack []string
	reported := map[string]bool{}
	var visit func(name string)
	visit = func(name string) {
		switch color[name] {
		case black:
			return
		case grey:
			// cycle: stack from name to end
			start := 0
			for i, s := range stack {
				if s == name {
					start = i
				}
			}
			cyc := append(append([]string{}, stack[start:]...), name)
			key := strings.Join(cyc, ">")
			if !reported[key] {
				reported[key] = true
				m := p.Modules[cyc[0]]
				var sp diag.Span = m.Span
				for _, im := range m.Imports {
					if im.Module == cyc[1] {
						sp = im.Span
					}
				}
				bag.Error(diag.ImportCycle, sp, "import cycle: %s", strings.Join(cyc, " -> "))
			}
			return
		}
		color[name] = grey
		stack = append(stack, name)
		if m := p.Modules[name]; m != nil {
			for _, im := range m.Imports {
				if _, ok := p.Modules[im.Module]; ok {
					visit(im.Module)
				}
			}
		}
		stack = stack[:len(stack)-1]
		color[name] = black
		order = append(order, name)
	}
	names := make([]string, 0, len(p.Modules))
	for n := range p.Modules {
		names = append(names, n)
	}
	sortStrings(names)
	for _, n := range names {
		visit(n)
	}
	return order
}

func sortStrings(s []string) {
	for i := 1; i < len(s); i++ {
		for j := i; j > 0 && s[j] < s[j-1]; j-- {
			s[j], s[j-1] = s[j-1], s[j]
		}
	}
}
```

- [ ] **Step 4: Implement resolve.go**

```go
// internal/names/resolve.go
package names

import (
	"alder/internal/diag"
	"alder/internal/syntax"
)

var preludeTypes = map[string]bool{"Int": true, "Float": true, "Bool": true, "Text": true, "Unit": true, "Date": true, "Time": true, "List": true, "Maybe": true, "Result": true, "Dict": true, "Html": true, "Never": true, "Query": true, "Task": true, "Nav": true, "Error": true, "Form": true, "Table": true, "Id": true, "Insert": true, "Select": true, "Condition": true, "Order": true, "Page": true}
var generatedTypes = map[string]bool{"Params": true, "Msg": true, "Session": true}

// moduleScope indexes one module's declarations.
type moduleScope struct {
	name    string
	values  map[string]bool
	types   map[string]bool   // type, table, form, brand names
	ctors   map[string]string // ctor → owning type name
	tables  map[string]bool
	caps    map[string]bool
	comps   map[string]bool   // capitalized value names
	columns map[string]map[string]bool // table → field set
	imports map[string]*syntax.Import
	used    map[string]bool
}

func (p *Program) index(m *syntax.Module) *moduleScope {
	s := &moduleScope{name: m.Name, values: map[string]bool{}, types: map[string]bool{}, ctors: map[string]string{}, tables: map[string]bool{}, caps: map[string]bool{}, comps: map[string]bool{}, columns: map[string]map[string]bool{}, imports: map[string]*syntax.Import{}, used: map[string]bool{}}
	for _, im := range m.Imports {
		s.imports[im.Module] = im
	}
	for _, d := range m.Decls {
		switch d := d.(type) {
		case *syntax.ValueDecl:
			if isUpper(d.Name) {
				s.comps[d.Name] = true
			}
			s.values[d.Name] = true
		case *syntax.TypeDecl:
			s.types[d.Name] = true
			for _, c := range d.Union {
				s.ctors[c.Name] = d.Name
			}
		case *syntax.TableDecl:
			s.types[d.Name] = true
			s.tables[d.Name] = true
			cols := map[string]bool{}
			for _, f := range d.Fields {
				cols[f.Name] = true
			}
			s.columns[d.Name] = cols
		case *syntax.FormDecl:
			s.types[d.Name] = true
		case *syntax.CapDecl:
			s.caps[d.Name] = true
		case *syntax.MsgDecl:
			s.ctors[d.Name] = "Msg"
		case *syntax.PageDecl:
			s.types["Params"] = true
		}
	}
	if m.Name == "App" {
		s.types["Session"] = true
	}
	return s
}

func isUpper(s string) bool { return len(s) > 0 && s[0] >= 'A' && s[0] <= 'Z' }

type Scope struct {
	parent *Scope
	names  map[string]bool
}

func (s *Scope) child() *Scope { return &Scope{parent: s, names: map[string]bool{}} }
func (s *Scope) bind(n string) { s.names[n] = true }
func (s *Scope) has(n string) bool {
	for c := s; c != nil; c = c.parent {
		if c.names[n] {
			return true
		}
	}
	return false
}

type resolver struct {
	p   *Program
	bag *diag.Bag
	m   *syntax.Module
	ms  *moduleScope
}

func (p *Program) Resolve(bag *diag.Bag) {
	for _, name := range p.Order {
		p.scopes[name] = p.index(p.Modules[name])
	}
	for _, name := range p.Std {
		p.scopes[name.Name] = p.index(name)
	}
	for _, name := range p.Order {
		r := &resolver{p: p, bag: bag, m: p.Modules[name], ms: p.scopes[name]}
		r.module()
	}
}

func (r *resolver) module() {
	for _, d := range r.m.Decls {
		switch d := d.(type) {
		case *syntax.ValueDecl:
			if d.Sig != nil {
				r.typ(d.Sig)
			}
			sc := (&Scope{names: map[string]bool{}}).child()
			for _, prm := range d.Params {
				sc.bind(prm.Name)
			}
			if d.Body != nil {
				r.expr(d.Body, sc)
			}
		case *syntax.TypeDecl:
			for _, c := range d.Union {
				if c.Arg != nil {
					r.typ(c.Arg)
				}
			}
			if d.Record != nil {
				r.recordType(d.Record)
			}
			if d.Brand != nil && d.Brand.Check != nil {
				r.expr(d.Brand.Check, &Scope{names: map[string]bool{}})
			}
		case *syntax.PageDecl:
			r.recordType(d.Params)
		case *syntax.MsgDecl:
			if d.Params != nil {
				r.recordType(d.Params)
			}
			if d.Form != "" && !r.ms.types[d.Form] {
				r.bag.Error(diag.UnknownName, d.Span, "unknown form `%s`", d.Form)
			}
		case *syntax.FormDecl:
			r.fields(d.Fields)
		case *syntax.TableDecl:
			r.fields(d.Fields)
		case *syntax.CapDecl:
			r.fields(d.Entries)
		case *syntax.SessionDecl:
			r.fields(d.Fields)
		case *syntax.LoginDecl:
			if !r.p.IsPageModule(d.Page) {
				r.bag.Error(diag.UnknownName, d.Span, "unknown page `%s`", d.Page)
			}
		case *syntax.TestDecl:
			r.expr(d.Body, &Scope{names: map[string]bool{}})
		}
	}
	for name, im := range r.ms.imports {
		if !r.ms.used[name] {
			r.bag.Warn(diag.UnusedImport, im.Span, "import `%s` is unused", name)
		}
	}
}

func (r *resolver) fields(fs []*syntax.Field) {
	for _, f := range fs {
		r.typ(f.Type)
		if f.Default != nil {
			r.expr(f.Default, &Scope{names: map[string]bool{}})
		}
	}
}
func (r *resolver) recordType(rt *syntax.RecordType) {
	if rt != nil {
		r.fields(rt.Fields)
	}
}

// importedScope returns the scope of X if X is imported or std, marking it used.
func (r *resolver) importedScope(x string) (*moduleScope, bool) {
	if _, ok := r.ms.imports[x]; ok {
		r.ms.used[x] = true
		return r.p.scopes[x], true
	}
	if _, ok := r.p.Std[x]; ok {
		return r.p.scopes[x], true
	}
	return nil, false
}

func (r *resolver) notImported(span diag.Span, x string) {
	d := r.bag.Error("E0201", span, "module `%s` is not imported", x)
	line := 2
	if n := len(r.m.Imports); n > 0 {
		line = r.m.Imports[n-1].Span.End.Line + 1
	}
	d.Fix = &diag.Fix{Line: line, Col: 1, Replace: "import " + x}
}

// ---- types

func (r *resolver) typ(t syntax.Type) {
	switch t := t.(type) {
	case *syntax.TypeName:
		for _, a := range t.Args {
			r.typ(a)
		}
		if t.Module != "" {
			if s, ok := r.importedScope(t.Module); ok {
				if s.types[t.Name] {
					t.Ref = syntax.Ref{Kind: syntax.RefType, Module: t.Module, Name: t.Name}
					return
				}
				r.bag.Error(diag.UnknownName, t.Span, "module `%s` has no type `%s`", t.Module, t.Name)
				return
			}
			if r.p.IsPageModule(t.Module+"."+t.Name) || r.p.IsPageModule(t.Module) {
				t.Ref = syntax.Ref{Kind: syntax.RefPage, Module: t.Module + "." + t.Name}
				return
			}
			r.notImported(t.Span, t.Module)
			return
		}
		switch {
		case r.ms.types[t.Name]:
			t.Ref = syntax.Ref{Kind: syntax.RefType, Module: r.m.Name, Name: t.Name}
		case generatedTypes[t.Name]:
			mod := r.m.Name
			if t.Name == "Session" {
				mod = "App"
			}
			t.Ref = syntax.Ref{Kind: syntax.RefType, Module: mod, Name: t.Name}
		case preludeTypes[t.Name]:
			t.Ref = syntax.Ref{Kind: syntax.RefType, Module: "Prelude", Name: t.Name}
		case r.p.IsPageModule(t.Name):
			t.Ref = syntax.Ref{Kind: syntax.RefPage, Module: t.Name}
		default:
			// module-name rule: imported module X declaring type X
			if s, ok := r.importedScope(t.Name); ok && s.types[t.Name] {
				t.Ref = syntax.Ref{Kind: syntax.RefType, Module: t.Name, Name: t.Name}
				return
			}
			// unqualified type that lives in an imported module → E0202 with fix
			for x := range r.ms.imports {
				if r.p.scopes[x] != nil && r.p.scopes[x].types[t.Name] {
					r.ms.used[x] = true
					d := r.bag.Error("E0202", t.Span, "type `%s` belongs to module `%s`; write `%s.%s`", t.Name, x, x, t.Name)
					d.Fix = &diag.Fix{Line: t.Span.Start.Line, Col: t.Span.Start.Col, Replace: x + "." + t.Name}
					return
				}
			}
			// any module at all declaring it → not imported
			for x, s := range r.p.scopes {
				if s.types[t.Name] && x != "Prelude" {
					r.notImported(t.Span, x)
					return
				}
			}
			r.bag.Error(diag.UnknownName, t.Span, "unknown type `%s`", t.Name)
		}
	case *syntax.RecordType:
		r.recordType(t)
	case *syntax.FunType:
		for _, a := range t.Params {
			r.typ(a)
		}
		r.typ(t.Result)
	case *syntax.TypeVar:
	}
}

// ---- expressions

func (r *resolver) expr(e syntax.Expr, sc *Scope) {
	switch e := e.(type) {
	case nil, *syntax.IntLit, *syntax.FloatLit, *syntax.BoolLit, *syntax.UnitLit, *syntax.HoleExpr:
	case *syntax.StrLit:
		for _, p := range e.Parts {
			if p.Expr != nil {
				r.expr(p.Expr, sc)
			}
		}
	case *syntax.ListLit:
		for _, x := range e.Elems {
			r.expr(x, sc)
		}
	case *syntax.RecordLit:
		for _, f := range e.Fields {
			r.expr(f.Value, sc)
		}
	case *syntax.RecordUpdate:
		r.expr(e.Base, sc)
		for _, f := range e.Fields {
			r.expr(f.Value, sc)
		}
	case *syntax.Var:
		r.variable(e, sc)
	case *syntax.CtorRef:
		r.ctor(e)
	case *syntax.Access:
		r.expr(e.Expr, sc)
	case *syntax.App:
		r.expr(e.Fn, sc)
		for _, a := range e.Args {
			r.expr(a, sc)
		}
	case *syntax.Lambda:
		c := sc.child()
		for _, p := range e.Params {
			c.bind(p.Name)
		}
		r.expr(e.Body, c)
	case *syntax.If:
		r.expr(e.Cond, sc)
		r.expr(e.Then, sc)
		r.expr(e.Else, sc)
	case *syntax.Match:
		r.expr(e.Subject, sc)
		for _, a := range e.Arms {
			c := sc.child()
			r.pattern(a.Pat, c)
			r.expr(a.Body, c)
		}
	case *syntax.Block:
		c := sc.child()
		for _, s := range e.Stmts {
			if s.Type != nil {
				r.typ(s.Type)
			}
			r.expr(s.Expr, c)
			c.bind(s.Name)
		}
		r.expr(e.Result, c)
	case *syntax.BinOp:
		r.expr(e.L, sc)
		r.expr(e.R, sc)
	case *syntax.Element:
		r.element(e, sc)
	case *syntax.HtmlText:
	case *syntax.HtmlExpr:
		r.expr(e.Expr, sc)
	case *syntax.HtmlFor:
		r.expr(e.List, sc)
		c := sc.child()
		c.bind(e.Var)
		r.expr(e.Body, c)
	case *syntax.HtmlIf:
		r.expr(e.Cond, sc)
		r.expr(e.Then, sc)
		r.expr(e.Else, sc)
	case *syntax.HtmlMatch:
		r.expr(e.Subject, sc)
		for _, a := range e.Arms {
			c := sc.child()
			r.pattern(a.Pat, c)
			r.expr(a.Body, c)
		}
	}
}

func (r *resolver) variable(v *syntax.Var, sc *Scope) {
	if v.Module == "" {
		switch {
		case sc.has(v.Name):
			v.Ref = syntax.Ref{Kind: syntax.RefLocal, Name: v.Name}
		case r.ms.values[v.Name]:
			v.Ref = syntax.Ref{Kind: syntax.RefValue, Module: r.m.Name, Name: v.Name}
		case r.p.scopes["Prelude"] != nil && r.p.scopes["Prelude"].values[v.Name]:
			v.Ref = syntax.Ref{Kind: syntax.RefStd, Module: "Prelude", Name: v.Name}
		default:
			r.bag.Error(diag.UnknownName, v.Span, "unknown name `%s`", v.Name)
		}
		return
	}
	// Module.value
	if s, ok := r.importedScope(v.Module); ok {
		switch {
		case s.values[v.Name]:
			kind := syntax.RefValue
			if _, std := r.p.Std[v.Module]; std {
				kind = syntax.RefStd
			}
			v.Ref = syntax.Ref{Kind: kind, Module: v.Module, Name: v.Name}
		case s.tables[v.Module] && s.columns[v.Module][v.Name]:
			v.Ref = syntax.Ref{Kind: syntax.RefColumn, Module: v.Module, Name: v.Name}
		default:
			r.bag.Error(diag.UnknownName, v.Span, "module `%s` has no `%s`", v.Module, v.Name)
		}
		return
	}
	// capability declared in this or an imported module: Password.verify
	if r.ms.caps[v.Module] {
		v.Ref = syntax.Ref{Kind: syntax.RefCapability, Module: r.m.Name, Name: v.Module + "." + v.Name}
		return
	}
	for x := range r.ms.imports {
		if r.p.scopes[x].caps[v.Module] {
			r.ms.used[x] = true
			v.Ref = syntax.Ref{Kind: syntax.RefCapability, Module: x, Name: v.Module + "." + v.Name}
			return
		}
	}
	// this module's own table columns: Todo.done inside module Todo
	if r.ms.tables[v.Module] && r.ms.columns[v.Module][v.Name] {
		v.Ref = syntax.Ref{Kind: syntax.RefColumn, Module: r.m.Name, Name: v.Name}
		return
	}
	r.notImported(v.Span, v.Module)
}

func (r *resolver) ctor(c *syntax.CtorRef) {
	full := c.Name
	if c.Module != "" {
		full = c.Module + "." + c.Name
	}
	if c.Module == "" {
		if _, ok := r.ms.ctors[c.Name]; ok {
			c.Ref = syntax.Ref{Kind: syntax.RefCtor, Module: r.m.Name, Name: c.Name}
			return
		}
		if r.p.scopes["Prelude"] != nil {
			if _, ok := r.p.scopes["Prelude"].ctors[c.Name]; ok {
				c.Ref = syntax.Ref{Kind: syntax.RefCtor, Module: "Prelude", Name: c.Name}
				return
			}
		}
		var found []string
		for x := range r.ms.imports {
			if _, ok := r.p.scopes[x].ctors[c.Name]; ok {
				found = append(found, x)
			}
		}
		if len(found) > 1 {
			r.bag.Error(diag.Duplicate, c.Span, "constructor `%s` is declared in both `%s` and `%s`", c.Name, found[0], found[1])
			return
		}
		if len(found) == 1 {
			r.ms.used[found[0]] = true
			c.Ref = syntax.Ref{Kind: syntax.RefCtor, Module: found[0], Name: c.Name}
			return
		}
		if r.p.IsPageModule(c.Name) {
			c.Ref = syntax.Ref{Kind: syntax.RefPage, Module: c.Name}
			return
		}
		if r.ms.tables[c.Name] {
			c.Ref = syntax.Ref{Kind: syntax.RefTable, Module: r.m.Name, Name: c.Name}
			return
		}
		if s, ok := r.importedScope(c.Name); ok && s.tables[c.Name] {
			c.Ref = syntax.Ref{Kind: syntax.RefTable, Module: c.Name, Name: c.Name}
			return
		}
		r.bag.Error(diag.UnknownName, c.Span, "unknown constructor `%s`", c.Name)
		return
	}
	if r.p.IsPageModule(full) {
		c.Ref = syntax.Ref{Kind: syntax.RefPage, Module: full}
		return
	}
	if c.Module == "App" {
		if _, ok := r.p.AppMsgs[c.Name]; ok {
			c.Ref = syntax.Ref{Kind: syntax.RefCtor, Module: "App", Name: c.Name}
			return
		}
	}
	if s, ok := r.importedScope(c.Module); ok {
		if _, isCtor := s.ctors[c.Name]; isCtor {
			d := r.bag.Error("E0203", c.Span, "constructors are not qualified; write `%s`", c.Name)
			d.Fix = &diag.Fix{Line: c.Span.Start.Line, Col: c.Span.Start.Col, Replace: c.Name}
			c.Ref = syntax.Ref{Kind: syntax.RefCtor, Module: c.Module, Name: c.Name}
			return
		}
		if s.tables[c.Name] {
			c.Ref = syntax.Ref{Kind: syntax.RefTable, Module: c.Module, Name: c.Name}
			return
		}
		r.bag.Error(diag.UnknownName, c.Span, "module `%s` has no `%s`", c.Module, c.Name)
		return
	}
	r.bag.Error(diag.UnknownName, c.Span, "unknown `%s`", full)
}

func (r *resolver) pattern(p syntax.Pattern, sc *Scope) {
	switch p := p.(type) {
	case *syntax.PVar:
		sc.bind(p.Name)
	case *syntax.PCtor:
		c := &syntax.CtorRef{Name: p.Name}
		c.Span = p.Span
		r.ctor(c)
		p.Ref = c.Ref
		if p.Arg != nil {
			r.pattern(p.Arg, sc)
		}
	case *syntax.PRecord:
		for _, f := range p.Fields {
			r.pattern(f.Pat, sc)
		}
	case *syntax.PList:
		for _, e := range p.Elems {
			r.pattern(e, sc)
		}
		if p.Rest != "" {
			sc.bind(p.Rest)
		}
	case *syntax.PWild, *syntax.PLit:
	}
}

var builtinComponents = map[string]bool{"Input": true, "Textarea": true, "Select": true, "Errors": true}

func (r *resolver) element(e *syntax.Element, sc *Scope) {
	for _, a := range e.Attrs {
		if a.Value != nil {
			r.expr(a.Value, sc)
		}
	}
	for _, c := range e.Children {
		r.expr(c, sc)
	}
	if !isUpper(e.Tag) {
		return
	}
	if e.Module == "" {
		switch {
		case builtinComponents[e.Tag]:
			e.Ref = syntax.Ref{Kind: syntax.RefComponent, Module: "Prelude", Name: e.Tag}
		case r.ms.comps[e.Tag]:
			e.Ref = syntax.Ref{Kind: syntax.RefComponent, Module: r.m.Name, Name: e.Tag}
		default:
			if s, ok := r.importedScope(e.Tag); ok && s.comps[e.Tag] {
				e.Ref = syntax.Ref{Kind: syntax.RefComponent, Module: e.Tag, Name: e.Tag}
				return
			}
			r.bag.Error(diag.UnknownName, e.Span, "unknown component `%s`", e.Tag)
		}
		return
	}
	if s, ok := r.importedScope(e.Module); ok {
		if s.comps[e.Tag] {
			e.Ref = syntax.Ref{Kind: syntax.RefComponent, Module: e.Module, Name: e.Tag}
			return
		}
		r.bag.Error(diag.UnknownName, e.Span, "module `%s` has no component `%s`", e.Module, e.Tag)
		return
	}
	r.notImported(e.Span, e.Module)
}
```

- [ ] **Step 5: Run tests**

Run: `go test ./internal/names/`
Expected: PASS (4 tests). `TestResolve_Basics` expects an E0002 for `Todo.filterLabel` and otherwise no errors — the `Query.succeed` reference resolves because `Query` is not in the test's std map; add `"Query": ParseSignatures("module Query\nsucceed : a -> Query a\n")` to `std(t)` so the test's "no errors besides E0002" assertion holds. Also give `Db`/`Html` stubs if the resolver reports them; the test's std map must contain every std module the test source references.

- [ ] **Step 6: Commit**

```bash
git add internal/names && git commit -m "m1: module graph, cycle detection and name resolution"
```

---

### Task 7: Types, unification and the embedded standard library

**Files:**
- Create: `internal/types/types.go`, `internal/types/unify.go`, `internal/types/unify_test.go`, `internal/types/stdlib.go`, `stdlib/*.alder`

**Interfaces:**
- Produces:
  ```go
  package types
  type Type interface{ isType() }
  type Con struct{ Name string }                       // Int, Text, Bool, Float, Unit, Date, Time, Never
  type Var struct{ ID int; Name string }               // inference variable; Name for printing (a, b)
  type Brand struct{ Module, Name string; Base Type; HasCheck bool }
  type Record struct{ Module, Name string; Fields []Field; Anonymous bool }  // Anonymous: Name == ""
  type Field struct{ Name string; Type Type; HasDefault bool }
  type Union struct{ Module, Name string; Args []Type; Ctors []Ctor }        // Ctors is shared per definition
  type Ctor struct{ Name string; Arg Type }            // Arg nil = no payload; may mention the union's params
  type App struct{ Con string; Args []Type }           // List, Maybe, Result, Html, Query, Task, Form, Table, Id, Insert, Select, Condition, Order, Dict, Page, Column
  type Fun struct{ Params []Type; Result Type }
  type Param struct{ Name string }                     // rigid type parameter inside a signature (a, b)
  func String(t Type) string
  type Subst map[int]Type
  type Unifier struct{ Subst Subst; next int; tables map[string]*TableInfo }
  func (u *Unifier) Fresh() *Var
  func (u *Unifier) Apply(t Type) Type                 // substitute and normalize (Id T → brand, Insert T → record)
  func (u *Unifier) Unify(a, b Type) error             // exact
  func (u *Unifier) Check(actual, expected Type) error // allows Brand→Base widening and Never→anything; otherwise Unify
  func (u *Unifier) Instantiate(t Type) Type           // replace Param with fresh Vars
  type TableInfo struct{ Module, Name string; Record *Record; IDField string; IDType Type; Insert *Record; Columns map[string]Type }
  func (u *Unifier) RegisterTable(info *TableInfo)
  ```
- Stdlib: `stdlib/*.alder` are signature files parsed with `syntax.ParseSignatures`. `LoadStd() (map[string]*syntax.Module, error)` embeds them with `embed.FS`. The checker converts their `ValueDecl.Sig` to `types.Type` (Task 8). Stdlib modules and what they must contain, exactly as chapter 9 and chapters 7–8 list them: `Prelude` (opaque `Int Float Bool Text Unit Date Time Never Html a Query a Task a Form f Table t Id t Insert t Select t Condition t Order t Column t a Dict k v Page`; unions `Bool = True | False`, `Maybe a = Just a | Nothing`, `Result e a = Ok a | Err e`, `Nav = Rerender | Redirect Page | Reject Text`; opaque `Error`; `not : Bool -> Bool`), `List`, `Maybe`, `Result`, `Text`, `Int`, `Float`, `Bool`, `Dict`, `Date`, `Time`, `Html` (`unsafe`, `none`, `text`), `Url` (`external : Text -> Page`), `Query` (`succeed, map, all, notFound`), `Task` (`succeed, fail, map, all`), `Db` (`get, from, where, orderBy, limit, offset, all, first, count, and, or, not, insert, update, delete`), `Session` (`put : (Session -> Session) -> Task Unit`), `Log`, `Time` includes `now : Query Time` and `epoch : Time`, `Date.today : Query Date`, `Random`, `Http`, `Form` (`values`), `Email` is not std (it's an app capability in the book's example; omit). Brands: `Text.NonEmpty`, `Text.Email`, `Text.Url`, `Text.Secret`, `Int.Positive`, `Int.NonNegative` declared as `type NonEmpty = brand Text where …` with a dummy check expression `True` (the runtime's check is in Go; the checker only needs `HasCheck`).
- `Column t a` is the type of `Todo.done`: a record-like builtin with methods; model it as `App{Con:"Column", Args:[t, a]}` and give `Access` on it special typing in Task 9: `.eq/.ne : a -> Condition t`, `.lt/.le/.gt/.ge` (numeric, Text, Date, Time, ordered unions), `.contains : Text -> Condition t` (a == Text or brand of Text), `.in : List a -> Condition t`, `.isNothing/.isJust : Condition t` (a is Maybe), `.asc/.desc : Order t`.

- [ ] **Step 1: Write the failing tests**

```go
// internal/types/unify_test.go
package types

import "testing"

var (
	tInt  = &Con{Name: "Int"}
	tText = &Con{Name: "Text"}
)

func TestUnify_Basics(t *testing.T) {
	u := &Unifier{Subst: Subst{}}
	a := u.Fresh()
	if err := u.Unify(&App{Con: "List", Args: []Type{a}}, &App{Con: "List", Args: []Type{tInt}}); err != nil {
		t.Fatal(err)
	}
	if String(u.Apply(a)) != "Int" {
		t.Fatalf("got %s", String(u.Apply(a)))
	}
	if err := u.Unify(tInt, tText); err == nil {
		t.Fatal("Int ~ Text must fail")
	}
}

func TestCheck_BrandWidening(t *testing.T) {
	u := &Unifier{Subst: Subst{}}
	id := &Brand{Module: "Todo", Name: "TodoId", Base: tText}
	if err := u.Check(id, tText); err != nil {
		t.Fatal("brand must widen to base in check mode:", err)
	}
	if err := u.Check(tText, id); err == nil {
		t.Fatal("base must not narrow to brand")
	}
	if err := u.Unify(id, tText); err == nil {
		t.Fatal("unify (exact) must not widen")
	}
	if err := u.Check(&Con{Name: "Never"}, tInt); err != nil {
		t.Fatal("Never fits anything")
	}
}

func TestNormalize_IdAndInsert(t *testing.T) {
	u := &Unifier{Subst: Subst{}}
	id := &Brand{Module: "Todo", Name: "TodoId", Base: tText}
	rec := &Record{Module: "Todo", Name: "Todo", Fields: []Field{{"id", id, false}, {"title", tText, false}, {"done", &Con{"Bool"}, true}}}
	u.RegisterTable(&TableInfo{Module: "Todo", Name: "Todo", Record: rec, IDField: "id", IDType: id,
		Insert: &Record{Module: "Todo", Name: "Insert Todo", Fields: rec.Fields[1:]}})
	got := u.Apply(&App{Con: "Id", Args: []Type{rec}})
	if String(got) != "Todo.TodoId" {
		t.Fatalf("Id Todo → %s", String(got))
	}
	ins := u.Apply(&App{Con: "Insert", Args: []Type{rec}}).(*Record)
	if len(ins.Fields) != 2 || ins.Fields[0].Name != "title" {
		t.Fatalf("Insert Todo → %s", String(ins))
	}
	// Id applied to a type variable stays symbolic until the variable is bound
	v := u.Fresh()
	sym := &App{Con: "Id", Args: []Type{v}}
	if _, ok := u.Apply(sym).(*App); !ok {
		t.Fatal("Id a must stay an App while a is unbound")
	}
	u.Unify(v, rec)
	if String(u.Apply(sym)) != "Todo.TodoId" {
		t.Fatalf("after binding: %s", String(u.Apply(sym)))
	}
}

func TestInstantiate(t *testing.T) {
	u := &Unifier{Subst: Subst{}}
	sig := &Fun{Params: []Type{&Fun{Params: []Type{&Param{"a"}}, Result: &Param{"b"}}, &App{"List", []Type{&Param{"a"}}}}, Result: &App{"List", []Type{&Param{"b"}}}}
	inst := u.Instantiate(sig).(*Fun)
	if _, ok := inst.Params[1].(*App).Args[0].(*Var); !ok {
		t.Fatal("params must become fresh vars")
	}
	if inst.Params[0].(*Fun).Params[0] != inst.Params[1].(*App).Args[0] {
		t.Fatal("same param must map to the same var")
	}
}

func TestString(t *testing.T) {
	got := String(&Fun{Params: []Type{tInt, &App{"Maybe", []Type{tText}}}, Result: &App{"List", []Type{&App{"Maybe", []Type{tInt}}}}})
	if got != "Int -> Maybe Text -> List (Maybe Int)" {
		t.Fatalf("got %s", got)
	}
}

func TestLoadStd(t *testing.T) {
	std, err := LoadStd()
	if err != nil {
		t.Fatal(err)
	}
	for _, m := range []string{"Prelude", "List", "Maybe", "Result", "Text", "Int", "Float", "Bool", "Dict", "Date", "Time", "Html", "Url", "Query", "Task", "Db", "Session", "Log", "Random", "Http", "Form"} {
		if std[m] == nil {
			t.Errorf("missing stdlib module %s", m)
		}
	}
}
```

- [ ] **Step 2: Run to verify failure**

Run: `go test ./internal/types/`
Expected: FAIL — undefined.

- [ ] **Step 3: Implement types.go**

```go
// internal/types/types.go
package types

import (
	"strings"
)

type Type interface{ isType() }

type Con struct{ Name string }
type Var struct {
	ID   int
	Name string
}
type Brand struct {
	Module, Name string
	Base         Type
	HasCheck     bool
}
type Field struct {
	Name       string
	Type       Type
	HasDefault bool
}
type Record struct {
	Module, Name string
	Fields       []Field
	Anonymous    bool
}
type Ctor struct {
	Name string
	Arg  Type
}
type Union struct {
	Module, Name string
	Args         []Type
	Ctors        []Ctor
	Params       []string // declared parameter names, same length as Args
}
type App struct {
	Con  string
	Args []Type
}
type Fun struct {
	Params []Type
	Result Type
}
type Param struct{ Name string }

func (*Con) isType()    {}
func (*Var) isType()    {}
func (*Brand) isType()  {}
func (*Record) isType() {}
func (*Union) isType()  {}
func (*App) isType()    {}
func (*Fun) isType()    {}
func (*Param) isType()  {}

var (
	Int   = &Con{"Int"}
	Float = &Con{"Float"}
	Bool  = &Con{"Bool"}
	Text  = &Con{"Text"}
	Unit  = &Con{"Unit"}
	Date  = &Con{"Date"}
	Time  = &Con{"Time"}
	Never = &Con{"Never"}
	Error = &Con{"Error"}
	Page  = &Con{"Page"}
)

func String(t Type) string { return str(t, false) }

func str(t Type, paren bool) string {
	switch t := t.(type) {
	case *Con:
		return t.Name
	case *Var:
		if t.Name != "" {
			return t.Name
		}
		return "?" + itoa(t.ID)
	case *Param:
		return t.Name
	case *Brand:
		return t.Module + "." + t.Name
	case *Record:
		if !t.Anonymous && t.Name != "" {
			if t.Module != "" && t.Module != t.Name {
				return t.Module + "." + t.Name
			}
			return t.Name
		}
		var parts []string
		for _, f := range t.Fields {
			parts = append(parts, f.Name+": "+str(f.Type, false))
		}
		return "{ " + strings.Join(parts, ", ") + " }"
	case *Union:
		s := t.Name
		if t.Module != "" && t.Module != "Prelude" && t.Module != t.Name {
			s = t.Module + "." + t.Name
		}
		if len(t.Args) == 0 {
			return s
		}
		for _, a := range t.Args {
			s += " " + str(a, true)
		}
		if paren {
			return "(" + s + ")"
		}
		return s
	case *App:
		s := t.Con
		for _, a := range t.Args {
			s += " " + str(a, true)
		}
		if paren && len(t.Args) > 0 {
			return "(" + s + ")"
		}
		return s
	case *Fun:
		var parts []string
		for _, p := range t.Params {
			parts = append(parts, str(p, true))
		}
		s := strings.Join(parts, " -> ") + " -> " + str(t.Result, false)
		if paren {
			return "(" + s + ")"
		}
		return s
	}
	return "?"
}

func itoa(i int) string {
	if i == 0 {
		return "0"
	}
	var b []byte
	for i > 0 {
		b = append([]byte{byte('0' + i%10)}, b...)
		i /= 10
	}
	return string(b)
}
```

- [ ] **Step 4: Implement unify.go**

```go
// internal/types/unify.go
package types

import "fmt"

type Subst map[int]Type

type TableInfo struct {
	Module, Name string
	Record       *Record
	IDField      string
	IDType       Type
	Insert       *Record
	Columns      map[string]Type // field → field type
}

type Unifier struct {
	Subst  Subst
	next   int
	tables map[string]*TableInfo // key: Module+"."+Name
}

func (u *Unifier) RegisterTable(info *TableInfo) {
	if u.tables == nil {
		u.tables = map[string]*TableInfo{}
	}
	u.tables[info.Module+"."+info.Name] = info
}

func (u *Unifier) Table(r *Record) *TableInfo {
	if u.tables == nil {
		return nil
	}
	return u.tables[r.Module+"."+r.Name]
}

func (u *Unifier) Fresh() *Var {
	u.next++
	return &Var{ID: u.next}
}

func (u *Unifier) prune(t Type) Type {
	for {
		v, ok := t.(*Var)
		if !ok {
			return t
		}
		b, bound := u.Subst[v.ID]
		if !bound {
			return t
		}
		t = b
	}
}

// Apply substitutes bound variables and normalizes Id/Insert over known tables.
func (u *Unifier) Apply(t Type) Type {
	t = u.prune(t)
	switch t := t.(type) {
	case *App:
		args := make([]Type, len(t.Args))
		for i, a := range t.Args {
			args[i] = u.Apply(a)
		}
		if (t.Con == "Id" || t.Con == "Insert") && len(args) == 1 {
			if r, ok := args[0].(*Record); ok {
				if info := u.Table(r); info != nil {
					if t.Con == "Id" {
						return info.IDType
					}
					return info.Insert
				}
			}
		}
		return &App{Con: t.Con, Args: args}
	case *Fun:
		ps := make([]Type, len(t.Params))
		for i, p := range t.Params {
			ps[i] = u.Apply(p)
		}
		return &Fun{Params: ps, Result: u.Apply(t.Result)}
	case *Union:
		if len(t.Args) == 0 {
			return t
		}
		args := make([]Type, len(t.Args))
		for i, a := range t.Args {
			args[i] = u.Apply(a)
		}
		return &Union{Module: t.Module, Name: t.Name, Args: args, Ctors: t.Ctors, Params: t.Params}
	case *Record:
		if !t.Anonymous {
			return t
		}
		fs := make([]Field, len(t.Fields))
		for i, f := range t.Fields {
			fs[i] = Field{f.Name, u.Apply(f.Type), f.HasDefault}
		}
		return &Record{Fields: fs, Anonymous: true}
	}
	return t
}

func (u *Unifier) occurs(v *Var, t Type) bool {
	t = u.prune(t)
	switch t := t.(type) {
	case *Var:
		return t.ID == v.ID
	case *App:
		for _, a := range t.Args {
			if u.occurs(v, a) {
				return true
			}
		}
	case *Fun:
		for _, p := range t.Params {
			if u.occurs(v, p) {
				return true
			}
		}
		return u.occurs(v, t.Result)
	case *Union:
		for _, a := range t.Args {
			if u.occurs(v, a) {
				return true
			}
		}
	case *Record:
		for _, f := range t.Fields {
			if u.occurs(v, f.Type) {
				return true
			}
		}
	}
	return false
}

func mismatch(a, b Type) error { return fmt.Errorf("expected %s, got %s", String(b), String(a)) }

// Unify: exact structural unification with nominal records/unions/brands.
func (u *Unifier) Unify(a, b Type) error { return u.unify(a, b, false) }

// Check: like Unify, but `actual` may be a brand where its base is expected, or Never anywhere.
func (u *Unifier) Check(actual, expected Type) error { return u.unify(actual, expected, true) }

func (u *Unifier) unify(a, b Type, widen bool) error {
	a, b = u.Apply(a), u.Apply(b)
	if va, ok := a.(*Var); ok {
		if vb, ok := b.(*Var); ok && vb.ID == va.ID {
			return nil
		}
		if u.occurs(va, b) {
			return fmt.Errorf("infinite type")
		}
		u.Subst[va.ID] = b
		return nil
	}
	if vb, ok := b.(*Var); ok {
		if u.occurs(vb, a) {
			return fmt.Errorf("infinite type")
		}
		u.Subst[vb.ID] = a
		return nil
	}
	if widen {
		if c, ok := a.(*Con); ok && c.Name == "Never" {
			return nil
		}
		if br, ok := a.(*Brand); ok {
			if _, bIsBrand := b.(*Brand); !bIsBrand {
				return u.unify(br.Base, b, true)
			}
		}
	}
	switch x := a.(type) {
	case *Con:
		if y, ok := b.(*Con); ok && x.Name == y.Name {
			return nil
		}
	case *Param:
		if y, ok := b.(*Param); ok && x.Name == y.Name {
			return nil
		}
	case *Brand:
		if y, ok := b.(*Brand); ok && x.Module == y.Module && x.Name == y.Name {
			return nil
		}
	case *Record:
		y, ok := b.(*Record)
		if !ok {
			break
		}
		if x.Anonymous && y.Anonymous {
			if len(x.Fields) != len(y.Fields) {
				return mismatch(a, b)
			}
			for i := range x.Fields {
				if x.Fields[i].Name != y.Fields[i].Name {
					return mismatch(a, b)
				}
				if err := u.unify(x.Fields[i].Type, y.Fields[i].Type, widen); err != nil {
					return err
				}
			}
			return nil
		}
		if x.Module == y.Module && x.Name == y.Name && !x.Anonymous && !y.Anonymous {
			return nil
		}
	case *Union:
		y, ok := b.(*Union)
		if !ok || x.Module != y.Module || x.Name != y.Name || len(x.Args) != len(y.Args) {
			break
		}
		for i := range x.Args {
			if err := u.unify(x.Args[i], y.Args[i], false); err != nil {
				return err
			}
		}
		return nil
	case *App:
		y, ok := b.(*App)
		if !ok || x.Con != y.Con || len(x.Args) != len(y.Args) {
			break
		}
		for i := range x.Args {
			// Html Never fits Html a: widen inside Html only
			w := widen && x.Con == "Html"
			if err := u.unify(x.Args[i], y.Args[i], w); err != nil {
				return err
			}
		}
		return nil
	case *Fun:
		y, ok := b.(*Fun)
		if !ok || len(x.Params) != len(y.Params) {
			break
		}
		for i := range x.Params {
			if err := u.unify(y.Params[i], x.Params[i], widen); err != nil {
				return err
			}
		}
		return u.unify(x.Result, y.Result, widen)
	}
	return mismatch(a, b)
}

// Instantiate replaces every Param with a fresh Var, consistently.
func (u *Unifier) Instantiate(t Type) Type {
	m := map[string]*Var{}
	var inst func(Type) Type
	inst = func(t Type) Type {
		switch t := t.(type) {
		case *Param:
			v, ok := m[t.Name]
			if !ok {
				v = u.Fresh()
				v.Name = ""
				m[t.Name] = v
			}
			return v
		case *App:
			args := make([]Type, len(t.Args))
			for i, a := range t.Args {
				args[i] = inst(a)
			}
			return &App{Con: t.Con, Args: args}
		case *Fun:
			ps := make([]Type, len(t.Params))
			for i, p := range t.Params {
				ps[i] = inst(p)
			}
			return &Fun{Params: ps, Result: inst(t.Result)}
		case *Union:
			if len(t.Args) == 0 {
				return t
			}
			args := make([]Type, len(t.Args))
			for i, a := range t.Args {
				args[i] = inst(a)
			}
			return &Union{Module: t.Module, Name: t.Name, Args: args, Ctors: t.Ctors, Params: t.Params}
		case *Record:
			if !t.Anonymous {
				return t
			}
			fs := make([]Field, len(t.Fields))
			for i, f := range t.Fields {
				fs[i] = Field{f.Name, inst(f.Type), f.HasDefault}
			}
			return &Record{Fields: fs, Anonymous: true}
		}
		return t
	}
	return inst(t)
}

// CtorArg returns the payload type of constructor `name` of union `un`, with the union's parameters substituted by un.Args.
func CtorArg(un *Union, name string) (Type, bool) {
	for _, c := range un.Ctors {
		if c.Name == name {
			if c.Arg == nil {
				return nil, true
			}
			return substParams(c.Arg, un.Params, un.Args), true
		}
	}
	return nil, false
}

func substParams(t Type, names []string, args []Type) Type {
	idx := map[string]Type{}
	for i, n := range names {
		if i < len(args) {
			idx[n] = args[i]
		}
	}
	var sub func(Type) Type
	sub = func(t Type) Type {
		switch t := t.(type) {
		case *Param:
			if r, ok := idx[t.Name]; ok {
				return r
			}
			return t
		case *App:
			args := make([]Type, len(t.Args))
			for i, a := range t.Args {
				args[i] = sub(a)
			}
			return &App{Con: t.Con, Args: args}
		case *Fun:
			ps := make([]Type, len(t.Params))
			for i, p := range t.Params {
				ps[i] = sub(p)
			}
			return &Fun{Params: ps, Result: sub(t.Result)}
		case *Union:
			if len(t.Args) == 0 {
				return t
			}
			as := make([]Type, len(t.Args))
			for i, a := range t.Args {
				as[i] = sub(a)
			}
			return &Union{Module: t.Module, Name: t.Name, Args: as, Ctors: t.Ctors, Params: t.Params}
		case *Record:
			fs := make([]Field, len(t.Fields))
			for i, f := range t.Fields {
				fs[i] = Field{f.Name, sub(f.Type), f.HasDefault}
			}
			return &Record{Module: t.Module, Name: t.Name, Fields: fs, Anonymous: t.Anonymous}
		}
		return t
	}
	return sub(t)
}
```

- [ ] **Step 5: Write the stdlib signature files and stdlib.go**

`stdlib/Prelude.alder`:

```alder
module Prelude

type Int
type Float
type Text
type Unit
type Date
type Time
type Never
type Error
type Page
type List a
type Dict k v
type Html a
type Query a
type Task a
type Form f
type Table t
type Id t
type Insert t
type Select t
type Condition t
type Order t
type Column t a

type Bool = True | False
type Maybe a = Just a | Nothing
type Result e a = Ok a | Err e
type Nav = Rerender | Redirect Page | Reject Text

not : Bool -> Bool
```

`stdlib/List.alder` — every signature from book chapter 9's List section, verbatim, as `name : Type` lines after `module List`. Do the same for `Maybe`, `Result`, `Text` (plus `type NonEmpty = brand Text where True`, `type Email = brand Text where True`, `type Url = brand Text where True`, `type Secret = brand Text where True`), `Int` (plus `type Positive = brand Int where True`, `type NonNegative = brand Int where True`), `Float`, `Bool` (`toText : Bool -> Text`), `Dict`, `Date`, `Time` (add `epoch : Time`), `Html` (`unsafe : Text -> Html Never`, `none : Html a`, `text : Text -> Html a`), `Url` (`external : Text -> Page`), `Query` (`succeed : a -> Query a`, `map : (a -> b) -> Query a -> Query b`, `all : List (Query a) -> Query (List a)`, `notFound : Query a`), `Task` (`succeed`, `fail : Text -> Task a`, `map`, `all`), `Db`:

```alder
module Db

get : Table t -> Id t -> Query (Maybe t)
from : Table t -> Select t
where : Condition t -> Select t -> Select t
orderBy : Order t -> Select t -> Select t
limit : Int -> Select t -> Select t
offset : Int -> Select t -> Select t
all : Select t -> Query (List t)
first : Select t -> Query (Maybe t)
count : Select t -> Query Int
and : List (Condition t) -> Condition t
or : List (Condition t) -> Condition t
not : Condition t -> Condition t
insert : Table t -> Insert t -> Task (Id t)
update : Table t -> Id t -> (t -> t) -> Task Unit
delete : Table t -> Id t -> Task Unit
```

`Session` (`put : (Session -> Session) -> Task Unit`), `Log` (`info`, `warn : Text -> Task Unit`), `Random` (`int : { min: Int, max: Int } -> Task Int`, `id : Task Text`), `Http` (`type HttpError`, `get : { url: Text } -> Task (Result HttpError Text)`, `post : { url: Text, body: Text } -> Task (Result HttpError Text)`), `Form` (`values : Form f -> f`).

Note `where` is a keyword in Alder; `Db.where` as a *name* must still lex. Amend the lexer: after a `.`, the next identifier is never a keyword (set a one-shot flag `afterDot` in `Next`). Add `TestLexer_KeywordAfterDot` (`Db.where x` → `UIdent(Db) Dot LIdent(where) LIdent(x)`). In `Db.alder` itself, the declaration line `where : …` starts a line; allow keyword-named declarations in `sigMode` by accepting `Keyword` tokens as names in `parseValueDecl` when `p.sigMode` (treat `p.tok.Kind == Keyword && next is Colon` as a value name).

```go
// internal/types/stdlib.go
package types

import (
	"embed"
	"fmt"
	"io/fs"
	"strings"

	"alder/internal/syntax"
)

//go:embed all:../../stdlib
var stdFS embed.FS

// LoadStd parses every embedded stdlib signature file.
func LoadStd() (map[string]*syntax.Module, error) {
	out := map[string]*syntax.Module{}
	entries, err := fs.ReadDir(stdFS, "stdlib")
	if err != nil {
		return nil, err
	}
	for _, e := range entries {
		if !strings.HasSuffix(e.Name(), ".alder") {
			continue
		}
		src, _ := stdFS.ReadFile("stdlib/" + e.Name())
		m, ds := syntax.ParseSignatures("stdlib/"+e.Name(), src)
		if len(ds) > 0 {
			return nil, fmt.Errorf("stdlib %s: %s", e.Name(), ds[0].Message)
		}
		out[m.Name] = m
	}
	return out, nil
}
```

`go:embed` cannot reference a parent directory. Move the embed to a package at the repo root instead: create `stdlib/embed.go` with `package stdlib; //go:embed *.alder; var FS embed.FS` and have `types.LoadStd` import `alder/stdlib`. Write it that way.

- [ ] **Step 6: Run tests**

Run: `go test ./internal/types/ ./internal/syntax/`
Expected: PASS, including `TestLoadStd` (which also proves every stdlib file parses).

- [ ] **Step 7: Commit**

```bash
git add internal/types stdlib internal/syntax && git commit -m "m1: type representation, unification with brand widening, embedded stdlib signatures"
```

---

### Task 8: Checker — scaffold and declaration pass

**Files:**
- Create: `internal/check/check.go`, `internal/check/decls.go`, `internal/check/check_test.go`, `testdata/golden/check_decls.alder` (+ sibling files as a multi-module case: `testdata/golden/check_decls/app/*.alder`) and `testdata/golden/check_decls.diag`

**Interfaces:**
- Produces:
  ```go
  package check
  func Program(prog *names.Program, bag *diag.Bag)   // runs the whole checker; appends to bag
  type Checker struct {
      prog   *names.Program
      bag    *diag.Bag
      u      *types.Unifier
      types  map[string]types.Type               // "Module.Name" → Record | Union | Brand (declared types, tables, forms)
      values map[string]types.Type               // "Module.name" → signature (uses Param for type variables)
      ctors  map[string]*ctorInfo                // "Module.Ctor" → union + payload
      tables map[string]*types.TableInfo         // "Module.Name"
      caps   map[string]map[string]types.Type    // "Module.Cap" → entry → type
      pages  map[string]*pageInfo                // page module → info
      session *types.Record                      // App's session record
      appMsg  *types.Union                       // App's Msg union (may be empty)
  }
  type ctorInfo struct{ Union *types.Union; Arg types.Type }
  type pageInfo struct{ Params *types.Record; Msg *types.Union; Requires string; Decl *syntax.PageDecl; Msgs map[string]*syntax.MsgDecl }
  type modCtx struct{ c *Checker; m *syntax.Module; page *pageInfo; session *types.Record; isApp bool }
  func (mc *modCtx) typeOf(t syntax.Type, params map[string]bool) types.Type   // AST type → types.Type; unknown → fresh Var (error already reported by names)
  ```
- Declaration pass order (so later references resolve): (1) all type/brand/record/union/table/form declarations in every module, by name only (placeholders); (2) their bodies; (3) tables → `TableInfo` (+ `RegisterTable`); (4) App session/msgs → `session`, `appMsg`; (5) pages → `pageInfo` with generated `Params` and `Msg`; (6) capabilities; (7) value signatures. All before any body is checked.
- Rules checked here (each a diagnostic): page param types URL-representable (`Int`, `Text`, `Bool`, brand of those, payload-less union, `Maybe`/`List` of those) → else **E0011**; `requires f` must name a `Maybe` session field → **E0011**; msg param types same rule; msg `form F` must be a form → E0002; form field types form-representable (`Text`, `Int`, `Float`, `Bool`, `Date`, brands, payload-less unions, `Maybe` of those) → **E0011**; table: exactly one `@id` and it's a `brand Text` → **E0011**; `@ref T`: `T` is a table and the field type is `T`'s id type or `Maybe` of it → **E0502**; `@created`/`@updated` on `Time` only → E0011; field types storable → E0011; capability entries end in `Query`/`Task` → E0011; `session` only in `App`, `login` only in `App` → E0011; a value without a signature → **E0007**; duplicate top-level names → **E0009**; a page module missing `Model`/`load`/`view` (or `update` when it has msgs), or a non-page module having `load`/`view`/`update`/`msg` → **E0010**.
- Generated types: `Params` = nominal `Record{Module: page, Name: "Params"}` from `page`'s record (defaults → `HasDefault`); `Msg` = `Union{Module: page, Name: "Msg"}` with one ctor per msg, payload `Record{Module: page, Name: "<Msg>Params", Fields: params… + form: FormRecord}`; `Session` = `Record{Module:"App", Name:"Session"}`; inside a page with `requires f`, `mc.session` is a copy with field `f` unwrapped from `Maybe a` to `a`.

- [ ] **Step 1: Write the golden case**

`testdata/golden/check_decls/app/App.alder`:
```alder
module App

import User
import Theme

session { user: Maybe User.UserId, theme: Theme = Light }

login = Todos.Index

msg Logout

update : Msg -> Session -> Task Nav
update msg session = Task.succeed Rerender

error : Error -> Html Never
error err = <h1>err</h1>

notFound : Html Never
notFound = <h1>nf</h1>
```

`testdata/golden/check_decls/app/Theme.alder`:
```alder
module Theme

type Theme = Light | Dark
```

`testdata/golden/check_decls/app/User.alder`:
```alder
module User

type UserId = brand Text

table User {
  id: UserId @id,
  email: Text.Email @unique,
}
```

`testdata/golden/check_decls/app/Todo.alder`:
```alder
module Todo

import User

type TodoId = brand Text

table Todo {
  id: TodoId @id,
  title: Text.NonEmpty,
  done: Bool = False,
  owner: User.UserId @ref User,
  bad: Text @ref User,
  created: Time @created,
  nested: List Int,
}

table Broken {
  id: Int @id,
  name: Text,
}
```

`testdata/golden/check_decls/app/Todos/Index.alder`:
```alder
module Todos.Index

import Todo

page "/" { filter: Int = 0, todos: List Todo } requires missing

type Model = { todos: List Todo }

load : Params -> Session -> Query Model
load params session = Query.succeed { todos: [] }

form NewTodo = {
  title: Text.NonEmpty,
  items: List Int,
}

msg Submit form NewTodo
msg Toggle { id: Todo.TodoId }

view : Model -> Session -> Html Msg
view model session = <ul></ul>

noSig = 1
```

`testdata/golden/check_decls.diag` (one line per diagnostic, sorted by file then position; format `file:line:col CODE message` — the harness prints exactly this):
```
app/Todo.alder:12:3 E0502 `@ref User` field `bad` must have type `User.UserId` or `Maybe User.UserId`, got `Text`
app/Todo.alder:14:3 E0011 table field `nested` has type `List Int`, which cannot be stored; model nesting with a second table and `@ref`
app/Todo.alder:18:3 E0011 `@id` field must be a `brand Text`, got `Int`
app/Todos/Index.alder:5:28 E0011 page parameter `todos` has type `List Todo`, which cannot appear in a URL
app/Todos/Index.alder:5:51 E0011 `requires missing`: the session has no field `missing`
app/Todos/Index.alder:14:3 E0011 form field `items` has type `List Int`, which cannot come from a form
app/Todos/Index.alder:20:1 E0010 page module `Todos.Index` declares messages but has no `update`
app/Todos/Index.alder:23:1 E0007 `noSig` has no type signature; write `noSig : Int` on the line before it
```

(Column numbers point at the field/parameter name or the declaration start; adjust the golden to the parser's actual spans only if the parser is right and the number above is a counting slip — never to hide a wrong span.)

- [ ] **Step 2: Write the golden harness**

```go
// internal/check/check_test.go
package check

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"alder/internal/diag"
	"alder/internal/names"
	"alder/internal/types"
)

var update = flag.Bool("update", false, "rewrite golden files")

func runDir(t *testing.T, dir string) *diag.Bag {
	std, err := types.LoadStd()
	if err != nil {
		t.Fatal(err)
	}
	prog, bag := names.Load(dir, std)
	if !bag.HasErrors() {
		prog.Resolve(bag)
	}
	if !bag.HasErrors() {
		Program(prog, bag)
	}
	bag.Sort()
	return bag
}

func render(bag *diag.Bag) string {
	var sb strings.Builder
	for _, d := range bag.Items {
		if d.Severity != "error" {
			continue
		}
		fmt.Fprintf(&sb, "%s:%d:%d %s %s\n", d.Span.File, d.Span.Start.Line, d.Span.Start.Col, d.Code, d.Message)
		if d.Fix != nil {
			fmt.Fprintf(&sb, "  fix %d:%d %s\n", d.Fix.Line, d.Fix.Col, d.Fix.Replace)
		}
	}
	return sb.String()
}

func TestCheckGolden(t *testing.T) {
	dirs, _ := filepath.Glob("../../testdata/golden/check_*")
	for _, dir := range dirs {
		if st, err := os.Stat(dir); err != nil || !st.IsDir() {
			continue
		}
		t.Run(filepath.Base(dir), func(t *testing.T) {
			got := render(runDir(t, dir))
			golden := dir + ".diag"
			if *update {
				os.WriteFile(golden, []byte(got), 0o644)
				return
			}
			want, _ := os.ReadFile(golden)
			if got != string(want) {
				t.Fatalf("golden mismatch\n got:\n%s\nwant:\n%s", got, want)
			}
		})
	}
}

func TestCorpus(t *testing.T) {
	dirs, _ := filepath.Glob("../../testdata/corpus/*")
	for _, dir := range dirs {
		t.Run(filepath.Base(dir), func(t *testing.T) {
			got := render(runDir(t, dir))
			want, err := os.ReadFile(filepath.Join(dir, "expected.diag"))
			if err != nil {
				t.Fatal(err)
			}
			if got != string(want) {
				t.Fatalf("corpus mismatch\n got:\n%s\nwant:\n%s", got, want)
			}
		})
	}
}

func TestExamplesCheckClean(t *testing.T) {
	for _, ex := range []string{"../../examples/todos"} {
		if got := render(runDir(t, ex)); got != "" {
			t.Fatalf("%s should check clean:\n%s", ex, got)
		}
	}
}
```

The multi-module golden harness ignores the `-update` note for a first run: write the `.diag` by hand (above) and make the checker produce it.

- [ ] **Step 3: Run to verify failure**

Run: `go test ./internal/check/ -run TestCheckGolden`
Expected: FAIL — undefined Program.

- [ ] **Step 4: Implement check.go**

```go
// internal/check/check.go
package check

import (
	"alder/internal/diag"
	"alder/internal/names"
	"alder/internal/syntax"
	"alder/internal/types"
)

type ctorInfo struct {
	Union *types.Union
	Arg   types.Type
}
type pageInfo struct {
	Params   *types.Record
	Msg      *types.Union
	Requires string
	Decl     *syntax.PageDecl
	Msgs     map[string]*syntax.MsgDecl
	Module   string
}

type Checker struct {
	prog    *names.Program
	bag     *diag.Bag
	u       *types.Unifier
	types   map[string]types.Type
	values  map[string]types.Type
	ctors   map[string]*ctorInfo
	tables  map[string]*types.TableInfo
	forms   map[string]*types.Record
	caps    map[string]map[string]types.Type
	pages   map[string]*pageInfo
	session *types.Record
	appMsg  *types.Union
}

type modCtx struct {
	c       *Checker
	m       *syntax.Module
	page    *pageInfo
	session *types.Record
	isApp   bool
}

func Program(prog *names.Program, bag *diag.Bag) {
	c := &Checker{prog: prog, bag: bag, u: &types.Unifier{Subst: types.Subst{}},
		types: map[string]types.Type{}, values: map[string]types.Type{}, ctors: map[string]*ctorInfo{},
		tables: map[string]*types.TableInfo{}, forms: map[string]*types.Record{}, caps: map[string]map[string]types.Type{}, pages: map[string]*pageInfo{}}
	c.declareStd()
	// pass 1: placeholders for every declared type in every module (app + std)
	for _, m := range c.allModules() {
		c.declareTypeNames(m)
	}
	// pass 2: bodies of types, tables, forms
	for _, m := range c.allModules() {
		c.ctx(m).defineTypes()
	}
	// pass 3: App session + app msgs, then pages (generated Params/Msg), capabilities, value signatures
	if app := prog.Modules["App"]; app != nil {
		c.ctx(app).defineApp()
	}
	if c.session == nil {
		c.session = &types.Record{Module: "App", Name: "Session"}
	}
	if c.appMsg == nil {
		c.appMsg = &types.Union{Module: "App", Name: "Msg"}
	}
	for _, name := range prog.Order {
		mc := c.ctx(prog.Modules[name])
		if prog.IsPageModule(name) {
			mc.definePage()
		}
		mc.defineCapabilities()
	}
	for _, m := range c.allModules() {
		c.ctx(m).declareValues()
	}
	// pass 4: bodies (Tasks 9–11)
	for _, name := range prog.Order {
		c.ctx(prog.Modules[name]).checkBodies()
	}
}

func (c *Checker) allModules() []*syntax.Module {
	var out []*syntax.Module
	for _, m := range c.prog.Std {
		out = append(out, m)
	}
	for _, name := range c.prog.Order {
		out = append(out, c.prog.Modules[name])
	}
	return out
}

func (c *Checker) ctx(m *syntax.Module) *modCtx {
	mc := &modCtx{c: c, m: m, session: c.session, isApp: m.Name == "App"}
	if p, ok := c.pages[m.Name]; ok {
		mc.page = p
		if p.Requires != "" && c.session != nil {
			mc.session = narrow(c.session, p.Requires)
		}
	}
	return mc
}

// narrow returns a copy of the session record with field f's `Maybe a` replaced by `a`.
func narrow(s *types.Record, f string) *types.Record {
	out := &types.Record{Module: s.Module, Name: s.Name}
	for _, fl := range s.Fields {
		if fl.Name == f {
			if un, ok := fl.Type.(*types.Union); ok && un.Name == "Maybe" && len(un.Args) == 1 {
				fl = types.Field{Name: fl.Name, Type: un.Args[0], HasDefault: fl.HasDefault}
			}
		}
		out.Fields = append(out.Fields, fl)
	}
	return out
}

func (c *Checker) key(mod, name string) string { return mod + "." + name }

func (c *Checker) errorf(code string, span diag.Span, format string, args ...any) *diag.Diagnostic {
	return c.bag.Error(code, span, format, args...)
}

// declareStd maps Prelude's opaque types to their canonical representation.
func (c *Checker) declareStd() {
	for _, n := range []string{"Int", "Float", "Bool", "Text", "Unit", "Date", "Time", "Never", "Error", "Page"} {
		c.types[c.key("Prelude", n)] = &types.Con{Name: n}
	}
}
```

- [ ] **Step 5: Implement decls.go**

```go
// internal/check/decls.go
package check

import (
	"strings"

	"alder/internal/diag"
	"alder/internal/syntax"
	"alder/internal/types"
)

// declareTypeNames creates placeholders so types can refer to each other in any order.
func (c *Checker) declareTypeNames(m *syntax.Module) {
	seen := map[string]diag.Span{}
	dup := func(name string, sp diag.Span) bool {
		if prev, ok := seen[name]; ok {
			c.errorf(diag.Duplicate, sp, "`%s` is already declared at line %d", name, prev.Start.Line)
			return true
		}
		seen[name] = sp
		return false
	}
	for _, d := range m.Decls {
		switch d := d.(type) {
		case *syntax.TypeDecl:
			if dup(d.Name, d.Span) {
				continue
			}
			k := c.key(m.Name, d.Name)
			if _, isStdCon := c.types[k]; isStdCon && m.Name == "Prelude" {
				continue // Int, Text… already canonical
			}
			switch {
			case d.Brand != nil:
				c.types[k] = &types.Brand{Module: m.Name, Name: d.Name, Base: baseType(d.Brand.Base), HasCheck: d.Brand.Check != nil}
			case d.Record != nil:
				c.types[k] = &types.Record{Module: m.Name, Name: d.Name}
			case d.Union != nil:
				c.types[k] = &types.Union{Module: m.Name, Name: d.Name, Params: d.Params}
			default: // opaque (stdlib): represented as App with Param args, or Con when no params
				if len(d.Params) == 0 {
					c.types[k] = &types.Con{Name: d.Name}
				} else {
					var args []types.Type
					for _, p := range d.Params {
						args = append(args, &types.Param{Name: p})
					}
					c.types[k] = &types.App{Con: d.Name, Args: args}
				}
			}
		case *syntax.TableDecl:
			if dup(d.Name, d.Span) {
				continue
			}
			c.types[c.key(m.Name, d.Name)] = &types.Record{Module: m.Name, Name: d.Name}
		case *syntax.FormDecl:
			if dup(d.Name, d.Span) {
				continue
			}
			c.types[c.key(m.Name, d.Name)] = &types.Record{Module: m.Name, Name: d.Name}
		case *syntax.ValueDecl:
			dup(d.Name, d.Span)
		case *syntax.MsgDecl:
			dup(d.Name, d.Span)
		}
	}
}

func baseType(name string) types.Type {
	switch name {
	case "Int":
		return types.Int
	case "Float":
		return types.Float
	case "Bool":
		return types.Bool
	default:
		return types.Text
	}
}

// typeOf converts an AST type. params are the type variables in scope (signature or type decl).
func (mc *modCtx) typeOf(t syntax.Type, params map[string]bool) types.Type {
	c := mc.c
	switch t := t.(type) {
	case *syntax.TypeName:
		var args []types.Type
		for _, a := range t.Args {
			args = append(args, mc.typeOf(a, params))
		}
		switch t.Ref.Kind {
		case syntax.RefPage:
			return types.Page
		case syntax.RefType:
			if t.Ref.Module == mc.m.Name || t.Ref.Module == "App" || c.prog.IsPageModule(t.Ref.Module) {
				switch t.Name {
				case "Params":
					if mc.page != nil && t.Ref.Module == mc.m.Name {
						return mc.page.Params
					}
				case "Msg":
					if t.Ref.Module == "App" {
						return c.appMsg
					}
					if mc.page != nil {
						return mc.page.Msg
					}
					if mc.isApp {
						return c.appMsg
					}
					c.errorf("E0601", t.Span, "`Msg` is only defined in page modules; a shared component is typed `Html a`").Fix = &diag.Fix{Line: t.Span.Start.Line, Col: t.Span.Start.Col, Replace: "a"}
					return c.u.Fresh()
				case "Session":
					return mc.session
				}
			}
			decl, ok := c.types[c.key(t.Ref.Module, t.Name)]
			if !ok {
				return c.u.Fresh()
			}
			return applyArgs(decl, args)
		}
		return c.u.Fresh()
	case *syntax.TypeVar:
		if params != nil && params[t.Name] {
			return &types.Param{Name: t.Name}
		}
		// an unknown lowercase type is a type variable in a signature (implicitly quantified)
		if params != nil {
			params[t.Name] = true
			return &types.Param{Name: t.Name}
		}
		return c.u.Fresh()
	case *syntax.RecordType:
		r := &types.Record{Anonymous: true}
		for _, f := range t.Fields {
			r.Fields = append(r.Fields, types.Field{Name: f.Name, Type: mc.typeOf(f.Type, params), HasDefault: f.Default != nil})
		}
		return r
	case *syntax.FunType:
		fn := &types.Fun{}
		for _, p := range t.Params {
			fn.Params = append(fn.Params, mc.typeOf(p, params))
		}
		fn.Result = mc.typeOf(t.Result, params)
		return fn
	}
	return c.u.Fresh()
}

// applyArgs instantiates a declared type with arguments: unions get Args; opaque Apps substitute their Params.
func applyArgs(decl types.Type, args []types.Type) types.Type {
	switch d := decl.(type) {
	case *types.Union:
		if len(args) == 0 {
			return d
		}
		return &types.Union{Module: d.Module, Name: d.Name, Args: args, Ctors: d.Ctors, Params: d.Params}
	case *types.App:
		if len(args) == 0 {
			return d
		}
		return &types.App{Con: d.Con, Args: args}
	}
	return decl
}

// defineTypes fills in placeholder bodies for this module's unions, records, tables, forms.
func (mc *modCtx) defineTypes() {
	c := mc.c
	for _, d := range mc.m.Decls {
		switch d := d.(type) {
		case *syntax.TypeDecl:
			params := map[string]bool{}
			for _, p := range d.Params {
				params[p] = true
			}
			k := c.key(mc.m.Name, d.Name)
			switch tt := c.types[k].(type) {
			case *types.Union:
				for _, ct := range d.Union {
					var arg types.Type
					if ct.Arg != nil {
						arg = mc.typeOf(ct.Arg, params)
					}
					tt.Ctors = append(tt.Ctors, types.Ctor{Name: ct.Name, Arg: arg})
					c.ctors[c.key(mc.m.Name, ct.Name)] = &ctorInfo{Union: tt, Arg: arg}
				}
			case *types.Record:
				for _, f := range d.Record.Fields {
					tt.Fields = append(tt.Fields, types.Field{Name: f.Name, Type: mc.typeOf(f.Type, params), HasDefault: f.Default != nil})
				}
			}
		case *syntax.TableDecl:
			mc.defineTable(d)
		case *syntax.FormDecl:
			rec := c.types[c.key(mc.m.Name, d.Name)].(*types.Record)
			for _, f := range d.Fields {
				ft := mc.typeOf(f.Type, nil)
				if !formRepresentable(ft) {
					c.errorf(diag.InvalidDecl, f.Span, "form field `%s` has type `%s`, which cannot come from a form", f.Name, types.String(ft))
				}
				rec.Fields = append(rec.Fields, types.Field{Name: f.Name, Type: ft, HasDefault: f.Default != nil})
			}
			c.forms[c.key(mc.m.Name, d.Name)] = rec
		}
	}
	if mc.m.Name == "Prelude" {
		// True/False are literals; Bool stays a Con. Register ctors Just/Nothing/Ok/Err/Rerender/Redirect/Reject only.
		delete(c.ctors, "Prelude.True")
		delete(c.ctors, "Prelude.False")
	}
}

func (mc *modCtx) defineTable(d *syntax.TableDecl) {
	c := mc.c
	rec := c.types[c.key(mc.m.Name, d.Name)].(*types.Record)
	info := &types.TableInfo{Module: mc.m.Name, Name: d.Name, Record: rec, Columns: map[string]types.Type{}, Insert: &types.Record{Module: mc.m.Name, Name: "Insert " + d.Name}}
	idCount := 0
	for _, f := range d.Fields {
		ft := mc.typeOf(f.Type, nil)
		if !storable(ft) {
			c.errorf(diag.InvalidDecl, f.Span, "table field `%s` has type `%s`, which cannot be stored; model nesting with a second table and `@ref`", f.Name, types.String(ft))
		}
		rec.Fields = append(rec.Fields, types.Field{Name: f.Name, Type: ft, HasDefault: f.Default != nil})
		info.Columns[f.Name] = ft
		generated := false
		for _, a := range f.Attrs {
			switch a.Name {
			case "id":
				idCount++
				generated = true
				info.IDField, info.IDType = f.Name, ft
				if b, ok := ft.(*types.Brand); !ok || b.Base != types.Text {
					c.errorf(diag.InvalidDecl, f.Span, "`@id` field must be a `brand Text`, got `%s`", types.String(ft))
				}
			case "created", "updated":
				generated = true
				if ft != types.Time {
					c.errorf(diag.InvalidDecl, f.Span, "`@%s` field must have type `Time`", a.Name)
				}
			case "ref":
				mc.checkRef(f, a, ft)
			case "unique":
			default:
				c.errorf(diag.InvalidDecl, a.Span, "unknown attribute `@%s`", a.Name)
			}
		}
		if !generated {
			info.Insert.Fields = append(info.Insert.Fields, types.Field{Name: f.Name, Type: ft, HasDefault: f.Default != nil})
		}
	}
	if idCount != 1 {
		c.errorf(diag.InvalidDecl, d.Span, "table `%s` must have exactly one `@id` field", d.Name)
	}
	c.tables[c.key(mc.m.Name, d.Name)] = info
	c.u.RegisterTable(info)
}

// checkRef validates `@ref T` after all tables have placeholders; the referenced table's id type is read from its decl.
func (mc *modCtx) checkRef(f *syntax.Field, a *syntax.Attr, ft types.Type) {
	c := mc.c
	// find the table decl named a.Arg in this module or an import
	var idType types.Type
	for _, m := range c.allModules() {
		if m.Name != mc.m.Name && !imports(mc.m, m.Name) {
			continue
		}
		for _, d := range m.Decls {
			if td, ok := d.(*syntax.TableDecl); ok && td.Name == a.Arg {
				for _, tf := range td.Fields {
					for _, ta := range tf.Attrs {
						if ta.Name == "id" {
							idType = c.ctx(m).typeOf(tf.Type, nil)
						}
					}
				}
			}
		}
	}
	if idType == nil {
		c.errorf(diag.UnknownName, a.Span, "`@ref %s`: no table named `%s` is in scope", a.Arg, a.Arg)
		return
	}
	ok := c.u.Unify(ft, idType) == nil
	if !ok {
		if un, isU := ft.(*types.Union); isU && un.Name == "Maybe" && len(un.Args) == 1 {
			ok = c.u.Unify(un.Args[0], idType) == nil
		}
	}
	if !ok {
		c.errorf("E0502", f.Span, "`@ref %s` field `%s` must have type `%s` or `Maybe %s`, got `%s`", a.Arg, f.Name, types.String(idType), types.String(idType), types.String(ft))
	}
}

func imports(m *syntax.Module, name string) bool {
	for _, im := range m.Imports {
		if im.Module == name {
			return true
		}
	}
	return false
}

func isScalar(t types.Type) bool {
	switch t := t.(type) {
	case *types.Con:
		switch t.Name {
		case "Int", "Float", "Bool", "Text", "Date", "Time":
			return true
		}
	case *types.Brand:
		return true
	case *types.Union:
		if t.Name == "Maybe" {
			return false
		}
		for _, ct := range t.Ctors {
			if ct.Arg != nil {
				return false
			}
		}
		return true
	}
	return false
}

func maybeOf(t types.Type) (types.Type, bool) {
	if un, ok := t.(*types.Union); ok && un.Name == "Maybe" && len(un.Args) == 1 {
		return un.Args[0], true
	}
	return nil, false
}

func storable(t types.Type) bool {
	if inner, ok := maybeOf(t); ok {
		return isScalar(inner)
	}
	return isScalar(t)
}

func formRepresentable(t types.Type) bool {
	if c, ok := t.(*types.Con); ok && c.Name == "Time" {
		return false
	}
	return storable(t)
}

func urlRepresentable(t types.Type) bool {
	if inner, ok := maybeOf(t); ok {
		return urlRepresentable(inner)
	}
	if app, ok := t.(*types.App); ok && app.Con == "List" && len(app.Args) == 1 {
		return urlRepresentable(app.Args[0])
	}
	if c, ok := t.(*types.Con); ok && (c.Name == "Float" || c.Name == "Date" || c.Name == "Time") {
		return false
	}
	return isScalar(t)
}

// defineApp reads session/login/msg from App.
func (mc *modCtx) defineApp() {
	c := mc.c
	c.appMsg = &types.Union{Module: "App", Name: "Msg"}
	for _, d := range mc.m.Decls {
		switch d := d.(type) {
		case *syntax.SessionDecl:
			rec := &types.Record{Module: "App", Name: "Session"}
			for _, f := range d.Fields {
				ft := mc.typeOf(f.Type, nil)
				if !formRepresentable(ft) {
					c.errorf(diag.InvalidDecl, f.Span, "session field `%s` has type `%s`, which cannot be stored in a session", f.Name, types.String(ft))
				}
				rec.Fields = append(rec.Fields, types.Field{Name: f.Name, Type: ft, HasDefault: f.Default != nil})
			}
			c.session = rec
		case *syntax.MsgDecl:
			mc.addMsg(c.appMsg, d, "App")
		}
	}
	mc.session = c.session
}

// addMsg appends a constructor for a msg declaration to union u; payload is a record of params (+ form).
func (mc *modCtx) addMsg(u *types.Union, d *syntax.MsgDecl, owner string) {
	c := mc.c
	payload := &types.Record{Module: owner, Name: d.Name + "Params"}
	if d.Params != nil {
		for _, f := range d.Params.Fields {
			ft := mc.typeOf(f.Type, nil)
			if !urlRepresentable(ft) {
				c.errorf(diag.InvalidDecl, f.Span, "message parameter `%s` has type `%s`, which cannot appear in a URL", f.Name, types.String(ft))
			}
			payload.Fields = append(payload.Fields, types.Field{Name: f.Name, Type: ft, HasDefault: f.Default != nil})
		}
	}
	if d.Form != "" {
		if rec, ok := c.forms[c.key(mc.m.Name, d.Form)]; ok {
			payload.Fields = append(payload.Fields, types.Field{Name: "form", Type: rec})
		} else {
			c.errorf(diag.UnknownName, d.Span, "unknown form `%s`", d.Form)
		}
	}
	u.Ctors = append(u.Ctors, types.Ctor{Name: d.Name, Arg: payload})
	c.ctors[c.key(owner, d.Name)] = &ctorInfo{Union: u, Arg: payload}
}

// definePage builds Params and Msg for a page module and validates the page declaration.
func (mc *modCtx) definePage() {
	c := mc.c
	pd := c.prog.Pages[mc.m.Name]
	info := &pageInfo{Decl: pd, Module: mc.m.Name, Msgs: map[string]*syntax.MsgDecl{}, Params: &types.Record{Module: mc.m.Name, Name: "Params"}, Msg: &types.Union{Module: mc.m.Name, Name: "Msg"}}
	c.pages[mc.m.Name] = info
	mc.page = info
	segs := pathParams(pd.Path)
	declared := map[string]bool{}
	for _, f := range pd.Params.Fields {
		ft := mc.typeOf(f.Type, nil)
		declared[f.Name] = true
		if !urlRepresentable(ft) {
			c.errorf(diag.InvalidDecl, f.Span, "page parameter `%s` has type `%s`, which cannot appear in a URL", f.Name, types.String(ft))
		}
		info.Params.Fields = append(info.Params.Fields, types.Field{Name: f.Name, Type: ft, HasDefault: f.Default != nil})
	}
	for _, s := range segs {
		if !declared[s] {
			c.errorf(diag.InvalidDecl, pd.Span, "path segment `{%s}` has no matching parameter", s)
		}
	}
	if pd.Requires != "" {
		info.Requires = pd.Requires
		found := false
		for _, f := range c.session.Fields {
			if f.Name == pd.Requires {
				found = true
				if _, ok := maybeOf(f.Type); !ok {
					c.errorf(diag.InvalidDecl, pd.Span, "`requires %s`: session field `%s` must be a `Maybe`", pd.Requires, pd.Requires)
				}
			}
		}
		if !found {
			sp := pd.Span
			sp.Start.Col = sp.Start.Col + strings.Index(lineOf(mc.m, pd.Span.Start.Line), "requires")
			c.errorf(diag.InvalidDecl, sp, "`requires %s`: the session has no field `%s`", pd.Requires, pd.Requires)
		}
		mc.session = narrow(c.session, pd.Requires)
	}
	for _, d := range mc.m.Decls {
		if md, ok := d.(*syntax.MsgDecl); ok {
			info.Msgs[md.Name] = md
			mc.addMsg(info.Msg, md, mc.m.Name)
		}
	}
}

func lineOf(m *syntax.Module, line int) string {
	if line >= 1 && line <= len(m.Lines) {
		return m.Lines[line-1]
	}
	return ""
}

// pathParams extracts {name} segments.
func pathParams(path string) []string {
	var out []string
	for i := 0; i < len(path); i++ {
		if path[i] == '{' {
			j := strings.IndexByte(path[i:], '}')
			if j > 0 {
				out = append(out, path[i+1:i+j])
				i += j
			}
		}
	}
	return out
}

func (mc *modCtx) defineCapabilities() {
	c := mc.c
	for _, d := range mc.m.Decls {
		cd, ok := d.(*syntax.CapDecl)
		if !ok {
			continue
		}
		entries := map[string]types.Type{}
		for _, e := range cd.Entries {
			params := map[string]bool{}
			et := mc.typeOf(e.Type, params)
			res := et
			if fn, isFn := et.(*types.Fun); isFn {
				res = fn.Result
			}
			if app, isApp := res.(*types.App); !isApp || (app.Con != "Query" && app.Con != "Task") {
				c.errorf(diag.InvalidDecl, e.Span, "capability entry `%s` must return a `Query` or `Task`", e.Name)
			}
			entries[e.Name] = et
		}
		c.caps[c.key(mc.m.Name, cd.Name)] = entries
	}
}

// declareValues records every value signature (E0007 when missing) and checks page/App shape (E0010).
func (mc *modCtx) declareValues() {
	c := mc.c
	has := map[string]bool{}
	for _, d := range mc.m.Decls {
		vd, ok := d.(*syntax.ValueDecl)
		if !ok {
			continue
		}
		has[vd.Name] = true
		if vd.Sig == nil {
			if mc.m.Name == "Prelude" || c.prog.Std[mc.m.Name] != nil {
				continue
			}
			d := c.errorf(diag.MissingSig, vd.Span, "`%s` has no type signature; write `%s : %s` on the line before it", vd.Name, vd.Name, "?type")
			_ = d
			continue
		}
		params := map[string]bool{}
		c.values[c.key(mc.m.Name, vd.Name)] = mc.typeOf(vd.Sig, params)
	}
	if mc.page != nil {
		for _, need := range []string{"load", "view"} {
			if !has[need] {
				c.errorf(diag.MissingDecl, mc.m.Span, "page module `%s` has no `%s`", mc.m.Name, need)
			}
		}
		if _, ok := c.types[c.key(mc.m.Name, "Model")]; !ok {
			c.errorf(diag.MissingDecl, mc.m.Span, "page module `%s` has no `Model`", mc.m.Name)
		}
		if len(mc.page.Msgs) > 0 && !has["update"] {
			sp := mc.m.Span
			sp.Start = diag.Pos{Line: lastDeclLine(mc.m), Col: 1}
			c.errorf(diag.MissingDecl, sp, "page module `%s` declares messages but has no `update`", mc.m.Name)
		}
	} else if !mc.isApp && c.prog.Std[mc.m.Name] == nil {
		for _, bad := range []string{"load", "view", "update"} {
			if has[bad] {
				c.errorf(diag.MissingDecl, mc.m.Span, "`%s` belongs in a page module; `%s` has no `page` declaration", bad, mc.m.Name)
			}
		}
		for _, d := range mc.m.Decls {
			switch d := d.(type) {
			case *syntax.MsgDecl:
				c.errorf(diag.InvalidDecl, d.Span, "`msg` is only allowed in a page module or `App`")
			case *syntax.SessionDecl:
				c.errorf(diag.InvalidDecl, d.Span, "`session` is only allowed in `App`")
			case *syntax.LoginDecl:
				c.errorf(diag.InvalidDecl, d.Span, "`login` is only allowed in `App`")
			}
		}
	}
}

func lastDeclLine(m *syntax.Module) int {
	line := 1
	for _, d := range m.Decls {
		if l := d.GetSpan().Start.Line; l > line {
			line = l
		}
	}
	return line
}

// checkBodies is completed in Tasks 9–11.
func (mc *modCtx) checkBodies() {}
```

The E0007 message in the golden says `noSig : Int`: infer the body's type when it's a literal or otherwise print `?type`. Implement: in `declareValues`, if `vd.Body` is `*syntax.IntLit` → `Int`, `*syntax.StrLit` → `Text`, `*syntax.BoolLit` → `Bool`, `*syntax.FloatLit` → `Float`, else `?type`; set `Fix{Line: vd.Span.Start.Line, Col: 1, Replace: name + " : " + ty + "\n"}`.

- [ ] **Step 6: Run the golden**

Run: `go test ./internal/check/ -run TestCheckGolden`
Expected: PASS once spans match. `TestExamplesCheckClean` will fail until Task 11; run only the golden test for now.

- [ ] **Step 7: Commit**

```bash
git add internal/check testdata/golden && git commit -m "m1: checker scaffold and declaration pass (types, tables, forms, pages, msgs, capabilities)"
```

---

### Task 9: Checker — expressions, blocks, patterns, exhaustiveness

**Files:**
- Create: `internal/check/expr.go`, `internal/check/pattern.go`, `testdata/golden/check_exprs/app/*.alder`, `testdata/golden/check_exprs.diag`
- Modify: `internal/check/decls.go` (replace the `checkBodies` stub)

**Interfaces:**
- Produces:
  ```go
  type env struct{ parent *env; vars map[string]types.Type }
  func (e *env) child() *env; func (e *env) bind(name string, t types.Type); func (e *env) lookup(name string) (types.Type, bool)
  type effect struct{ kind string /* "", "Query", "Task" */; inLoad bool }
  func (mc *modCtx) check(e syntax.Expr, expected types.Type, env *env, eff effect)      // reports E0003 on mismatch with "expected X, got Y"
  func (mc *modCtx) infer(e syntax.Expr, env *env, eff effect) types.Type
  func (mc *modCtx) checkBodies()      // values, tests, brand checks; pages/HTML are Tasks 10–11 but their hooks are called from here
  ```
- Rules (chapter 3–4, 7):
  - `check(e, expected)`: for `RecordLit`, `ListLit`, `Lambda`, `If`, `Match`, `Block`, `StrLit` with the expected type pushed in; everything else infers then `u.Check(got, expected)`; on failure **E0003** `expected <E>, got <G>` at `e`'s span. Special message when `got` is `Maybe X` and `expected` is `X`: append `; match it: match m { Just x -> …, Nothing -> … }`.
  - Literals: `Int`, `Float`, `Bool`, `Unit`; `StrLit` parts: each interpolated expr is checked against `Text` (so an `Int` interpolation is E0003 with fix `Text.fromInt (<expr>)`).
  - `Var` by `Ref.Kind`: local → env; value/std → `values[key]` instantiated; column → `App{Column, [tableRecord, fieldType]}`; capability → `caps[key][name]` instantiated.
  - `CtorRef` by `Ref.Kind`: ctor → if payload nil, the union type; else `Fun{[payload], union}` (instantiated); page → `Fun{[Params], Page}` where Params comes from `c.pages[module].Params` **with defaulted fields optional** (see record-literal rule) — or `Page` directly when the page has no fields and the ctor is bare; table → `App{Table,[record]}`; `App.X` ctor → `Fun{[payload], appMsg}` or `appMsg` when the payload is empty.
  - Bare msg constructor: if a ctor's payload record has **no fields**, the bare `CtorRef` has the union type itself (so `click={Logout}` and `submit={Submit}` type-check without `{}`); with fields, bare use is **E0301** "message `Toggle` needs its parameters: `Toggle { id: ?id }`" with that fix. The same applies when `Submit` has only a `form` field: `form` is never supplied in HTML, so a ctor whose only field is `form` is bare too; in `update` patterns the payload still has `form`.
  - `App` (application): infer `Fn`; must be `Fun`; check each arg against the param in order; fewer args → partial application yields the remaining `Fun`; more args than params → **E0008**. Applying a ctor to a record literal: the record literal is checked against the payload record with the "form field is implicit" rule (a payload record's `form` field is omitted from HTML-side literals; the checker treats `form` as `HasDefault` for literal checking).
  - `Access`: infer base; `Record` → field type or E0002 "no field `f` on `T`"; `Maybe _` → **E0102** "`.f` on a `Maybe`; match it first" with fix `(match <base> { Just x -> x.f, Nothing -> ?default })`; `App{Column}` → method types per Task 7's table; otherwise E0003.
  - `RecordLit` checked against `Record`: every non-default field present, no extras (E0003 listing missing/extra); fields checked. Inferred (no expectation): anonymous record.
  - `RecordUpdate`: infer base (must be a `Record`); each field must exist; values checked against field types; result is the base type.
  - `BinOp`: `+ - * / %` → both sides same numeric (`Int`, `Float`, or same numeric brand; result that type; `+`-family on brands returns the brand per chapter 4); `++` → both `Text` (brands widen) or both `List a`; comparisons `< <= > >=` → `Int`, `Float`, `Text`, `Date`, `Time`, or a payload-less union, both sides same; `== !=` → `u.Unify` (no widening) and the type must not contain `Fun` (E0003 "functions cannot be compared"); `&& ||` → `Bool`; `??` → L `Maybe a`, R `a`, result `a`; `|>` → R must be `Fun`, L checked against its first param, result is the rest. For `a + "x"` where one side is `Text`, add to the E0003: "`++` joins Text; `+` is for numbers".
  - `Lambda`: expected `Fun` → bind params to param types and check body; inferred → fresh vars.
  - `If`: cond `Bool`; both branches against expected (or infer then unify).
  - `Match`: subject inferred; each arm's pattern checked against the subject type (binds vars), body against expected; exhaustiveness via `pattern.go`; **E0310** for an arm after a `_`, or a duplicate constructor arm.
  - `Block`: expected `App{Query|Task}` → `eff.kind` set accordingly; else if any `let <-` present and expected is unknown → infer the first bound expr's kind; `let x <- e`: `e` must be `Query a` (if kind Query) or `Query a`/`Task a` (if kind Task) → else **E0401** "`Task` cannot run inside a `Query`; `load` can only read" (when `eff.kind == "Query"`) — and **E0402** "`let x <-` only inside a block that returns `Query` or `Task`" when kind is ""; result expression checked against expected.
  - `HoleExpr`: type is a fresh var; report **W0002** "hole ?name has type T" after the enclosing declaration is checked (resolve var then print).
  - `Element` and HTML nodes → Task 10 (`mc.checkHTML(e, expected, env, eff)`), stubbed here as "infer fresh".
- Patterns (`pattern.go`): `checkPattern(p, t types.Type, env)` binds variables; `PCtor` must belong to the subject's union (else E0003 "pattern `X` is not a constructor of `T`"); arg pattern checked against the ctor payload; nesting deeper than one level inside a ctor payload (`Just (Just x)`) is **E0001**-class error "patterns nest one level; match on `x` in a second `match`"; `PRecord` against a `Record`; `PList` against `List a`; `PLit` against `Int`/`Text`/`Bool`. Exhaustiveness: union → set of ctors covered or `_`; `Bool` → both literals or `_`; `List` → `[]` and `[x, ..rest]` (or `_`); `Int`/`Text` → `_` required; records → always exhaustive. **E0004** lists missing cases: "missing cases: `Completed`, `Active`" with fix appending arms `  Completed -> ?todo,`.

- [ ] **Step 1: Write the golden case**

`testdata/golden/check_exprs/app/Todo.alder`:
```alder
module Todo

type TodoId = brand Text
type Cents = brand Int
type Filter = All | Active | Completed

table Todo {
  id: TodoId @id,
  title: Text.NonEmpty,
  done: Bool = False,
  due: Maybe Date,
  price: Cents,
}

matches : Filter -> Todo -> Bool
matches filter todo = match filter {
  All -> True,
  Active -> not todo.done,
}

label : Filter -> Text
label f = match f {
  All -> "all",
  Active -> "active",
  Completed -> "done",
  _ -> "never",
}

total : List Todo -> Cents
total todos = List.foldl (\t acc -> t.price + acc) (Cents 0) todos

bad1 : Int
bad1 = "x" + 1

bad2 : Text
bad2 = "n = {List.length []}"

bad3 : Text
bad3 = Text.trim (TodoId "a") ++ (TodoId "b")

bad4 : Bool
bad4 = (TodoId "a") == "a"

bad5 : Todo -> Text
bad5 t = t.due.year

bad6 : Query (Maybe Todo)
bad6 = {
  let found <- Db.get Todo "not-an-id"
  Query.succeed found
}

bad7 : Query Unit
bad7 = {
  let _ <- Db.delete Todo (TodoId "a")
  Query.succeed ()
}

bad8 : Int
bad8 = {
  let x <- Query.succeed 1
  x
}

bad9 : Maybe Int -> Int
bad9 m = match m {
  Just x -> x,
}

bad10 : List Int -> Int
bad10 xs = match xs {
  [] -> 0,
}

ok1 : Maybe Int -> Int
ok1 m = m ?? 0

ok2 : Todo -> Text
ok2 t = t.title

ok3 : Filter -> Bool
ok3 f = f < Completed

ok4 : List Int -> List Int
ok4 xs = xs |> List.map (\x -> x + 1) |> List.filter (\x -> x > 2)

ok5 : { w: Float, h: Float } -> Float
ok5 r = r.w * r.h

ok6 : Todo -> Todo
ok6 t = { t | done: True }

bad11 : Todo -> Todo
bad11 t = { t | nope: True }

test "label all" = label All == "all"
```

`testdata/golden/check_exprs.diag`:
```
app/Todo.alder:15:23 E0004 missing cases: `Completed`
  fix 18:1   Completed -> ?todo,
app/Todo.alder:25:3 E0310 unreachable arm: every case is already handled
app/Todo.alder:31:8 E0003 expected Int, got Text; `++` joins Text, `+` is for numbers
app/Todo.alder:34:14 E0003 expected Text, got Int
  fix 34:14 Text.fromInt (List.length [])
app/Todo.alder:40:8 E0003 cannot compare `Todo.TodoId` with `Text`: `==` needs both sides to have the same type
app/Todo.alder:43:10 E0102 `.year` on a `Maybe Date`; match it first
  fix 43:10 (match t.due { Just x -> x.year, Nothing -> ?default })
app/Todo.alder:47:27 E0101 `Db.get Todo` takes a `Todo.TodoId`, got `Text`
app/Todo.alder:53:12 E0401 `Task` cannot run inside a `Query`; `load` and `Query` blocks can only read
app/Todo.alder:59:3 E0402 `let x <-` is only allowed in a block that returns `Query` or `Task`
app/Todo.alder:64:10 E0004 missing cases: `Nothing`
  fix 66:1   Nothing -> ?todo,
app/Todo.alder:69:12 E0004 missing cases: `[x, ..rest]`
  fix 71:1   [x, ..rest] -> ?todo,
app/Todo.alder:92:17 E0002 no field `nope` on `Todo`
```

`bad3` is fine (brand widens to `Text` for `Text.trim` and `++`), so no diagnostic. `ok3` relies on union ordering. The exact columns are the parser's spans for: the `match` keyword (E0004), the `_` arm, the `+` BinOp start (its L operand), the interpolated expr, the `==` BinOp, the `.year` Access (span starts at `t`), the argument `"not-an-id"`, the `Db.delete …` expression, the `let`, the match keyword again, the `nope` field init.

- [ ] **Step 2: Run to verify failure**

Run: `go test ./internal/check/ -run TestCheckGolden/check_exprs`
Expected: FAIL (empty output; bodies not checked).

- [ ] **Step 3: Implement expr.go**

```go
// internal/check/expr.go
package check

import (
	"fmt"
	"strings"

	"alder/internal/diag"
	"alder/internal/syntax"
	"alder/internal/types"
)

type env struct {
	parent *env
	vars   map[string]types.Type
}

func newEnv() *env        { return &env{vars: map[string]types.Type{}} }
func (e *env) child() *env { return &env{parent: e, vars: map[string]types.Type{}} }
func (e *env) bind(n string, t types.Type) {
	if n != "_" {
		e.vars[n] = t
	}
}
func (e *env) lookup(n string) (types.Type, bool) {
	for c := e; c != nil; c = c.parent {
		if t, ok := c.vars[n]; ok {
			return t, true
		}
	}
	return nil, false
}

type effect struct {
	kind   string // "", "Query", "Task"
	inLoad bool
}

type holeNote struct {
	name string
	t    types.Type
	span diag.Span
}

// checkBodies checks every value, test and brand check in the module (page/HTML rules hook in from Tasks 10–11).
func (mc *modCtx) checkBodies() {
	c := mc.c
	if c.prog.Std[mc.m.Name] != nil {
		return
	}
	for _, d := range mc.m.Decls {
		switch d := d.(type) {
		case *syntax.ValueDecl:
			sig, ok := c.values[c.key(mc.m.Name, d.Name)]
			if !ok || d.Body == nil {
				continue
			}
			mc.holes = nil
			env := newEnv()
			eff := effect{}
			if mc.page != nil && d.Name == "load" {
				eff.inLoad = true
			}
			body := sig
			if fn, isFn := sig.(*types.Fun); isFn {
				if len(d.Params) > len(fn.Params) {
					c.errorf(diag.Arity, d.Span, "`%s` takes %d parameters but %d are named", d.Name, len(fn.Params), len(d.Params))
					continue
				}
				for i, p := range d.Params {
					env.bind(p.Name, fn.Params[i])
				}
				if len(d.Params) < len(fn.Params) {
					body = &types.Fun{Params: fn.Params[len(d.Params):], Result: fn.Result}
				} else {
					body = fn.Result
				}
			} else if len(d.Params) > 0 {
				c.errorf(diag.Arity, d.Span, "`%s` has type `%s`, which takes no parameters", d.Name, types.String(sig))
				continue
			}
			mc.check(d.Body, body, env, eff)
			mc.reportHoles()
		case *syntax.TestDecl:
			mc.check(d.Body, types.Bool, newEnv(), effect{})
		case *syntax.TypeDecl:
			if d.Brand != nil && d.Brand.Check != nil {
				mc.check(d.Brand.Check, &types.Fun{Params: []types.Type{baseType(d.Brand.Base)}, Result: types.Bool}, newEnv(), effect{})
			}
		}
	}
	mc.checkPage() // Task 11; no-op for non-page modules
}

func (mc *modCtx) reportHoles() {
	for _, h := range mc.holes {
		mc.c.bag.Warn("W0002", h.span, "hole ?%s has type %s", h.name, types.String(mc.c.u.Apply(h.t)))
	}
}

func (mc *modCtx) mismatch(span diag.Span, got, expected types.Type) *diag.Diagnostic {
	c := mc.c
	g, e := c.u.Apply(got), c.u.Apply(expected)
	msg := fmt.Sprintf("expected %s, got %s", types.String(e), types.String(g))
	if inner, ok := maybeOf(g); ok && c.u.Unify(inner, e) == nil {
		msg += "; match it: match m { Just x -> …, Nothing -> … }"
	}
	return c.errorf(diag.TypeMismatch, span, "%s", msg)
}

// check verifies e against expected, pushing the expectation into literals, lambdas, branches and blocks.
func (mc *modCtx) check(e syntax.Expr, expected types.Type, env *env, eff effect) {
	c := mc.c
	expected = c.u.Apply(expected)
	switch x := e.(type) {
	case *syntax.RecordLit:
		if rec, ok := expected.(*types.Record); ok && !rec.Anonymous {
			mc.checkRecordLit(x, rec, env, eff)
			return
		}
		if rec, ok := expected.(*types.Record); ok && rec.Anonymous {
			mc.checkRecordLit(x, rec, env, eff)
			return
		}
	case *syntax.ListLit:
		if app, ok := expected.(*types.App); ok && app.Con == "List" {
			for _, el := range x.Elems {
				mc.check(el, app.Args[0], env, eff)
			}
			return
		}
	case *syntax.Lambda:
		if fn, ok := expected.(*types.Fun); ok {
			if len(x.Params) > len(fn.Params) {
				c.errorf(diag.Arity, x.Span, "lambda has %d parameters but %d are expected", len(x.Params), len(fn.Params))
				return
			}
			child := env.child()
			for i, p := range x.Params {
				child.bind(p.Name, fn.Params[i])
			}
			body := types.Type(fn.Result)
			if len(x.Params) < len(fn.Params) {
				body = &types.Fun{Params: fn.Params[len(x.Params):], Result: fn.Result}
			}
			mc.check(x.Body, body, child, eff)
			return
		}
	case *syntax.If:
		mc.check(x.Cond, types.Bool, env, eff)
		mc.check(x.Then, expected, env, eff)
		mc.check(x.Else, expected, env, eff)
		return
	case *syntax.Match:
		mc.checkMatch(x, expected, env, eff)
		return
	case *syntax.Block:
		mc.checkBlock(x, expected, env, eff)
		return
	case *syntax.Element, *syntax.HtmlFor, *syntax.HtmlIf, *syntax.HtmlMatch, *syntax.HtmlExpr, *syntax.HtmlText:
		mc.checkHTML(e, expected, env, eff)
		return
	case *syntax.HoleExpr:
		mc.holes = append(mc.holes, holeNote{x.Name, expected, x.Span})
		return
	}
	got := mc.infer(e, env, eff)
	if err := c.u.Check(got, expected); err != nil {
		d := mc.mismatch(e.GetSpan(), got, expected)
		mc.annotateMismatch(d, e, got, expected)
	}
}

// annotateMismatch adds the fixes the spec lists for common mismatches.
func (mc *modCtx) annotateMismatch(d *diag.Diagnostic, e syntax.Expr, got, expected types.Type) {
	c := mc.c
	g, ex := c.u.Apply(got), c.u.Apply(expected)
	if isCon(ex, "Text") {
		src := mc.source(e.GetSpan())
		switch {
		case isCon(g, "Int"):
			d.Fix = &diag.Fix{Line: e.GetSpan().Start.Line, Col: e.GetSpan().Start.Col, Replace: "Text.fromInt (" + src + ")"}
		case isCon(g, "Float"):
			d.Fix = &diag.Fix{Line: e.GetSpan().Start.Line, Col: e.GetSpan().Start.Col, Replace: "Text.fromFloat (" + src + ")"}
		}
	}
}

func isCon(t types.Type, name string) bool {
	c, ok := t.(*types.Con)
	return ok && c.Name == name
}

// source returns the text of a single-line span.
func (mc *modCtx) source(sp diag.Span) string {
	line := lineOf(mc.m, sp.Start.Line)
	if sp.Start.Line != sp.End.Line {
		if sp.Start.Col-1 <= len(line) {
			return strings.TrimSpace(line[sp.Start.Col-1:])
		}
		return ""
	}
	if sp.End.Col-1 > len(line) || sp.Start.Col-1 > len(line) {
		return ""
	}
	return line[sp.Start.Col-1 : sp.End.Col-1]
}

func (mc *modCtx) checkRecordLit(x *syntax.RecordLit, rec *types.Record, env *env, eff effect) {
	c := mc.c
	byName := map[string]types.Field{}
	for _, f := range rec.Fields {
		byName[f.Name] = f
	}
	seen := map[string]bool{}
	for _, fi := range x.Fields {
		f, ok := byName[fi.Name]
		if !ok {
			c.errorf(diag.UnknownName, fi.Span, "no field `%s` on `%s`", fi.Name, types.String(rec))
			mc.infer(fi.Value, env, eff)
			continue
		}
		if seen[fi.Name] {
			c.errorf(diag.Duplicate, fi.Span, "field `%s` is given twice", fi.Name)
		}
		seen[fi.Name] = true
		mc.check(fi.Value, f.Type, env, eff)
	}
	var missing []string
	for _, f := range rec.Fields {
		if !seen[f.Name] && !f.HasDefault && f.Name != "form" {
			missing = append(missing, f.Name)
		}
	}
	if len(missing) > 0 {
		c.errorf(diag.TypeMismatch, x.Span, "record `%s` is missing field(s): %s", types.String(rec), strings.Join(missing, ", "))
	}
}

func (mc *modCtx) checkMatch(x *syntax.Match, expected types.Type, env *env, eff effect) {
	subj := mc.c.u.Apply(mc.infer(x.Subject, env, eff))
	ex := newExhaustiveness(mc, subj)
	for _, a := range x.Arms {
		child := env.child()
		if ex.closed {
			mc.c.errorf(diag.UnreachableArm, a.Span, "unreachable arm: every case is already handled")
		}
		mc.checkPattern(a.Pat, subj, child)
		ex.cover(a.Pat)
		if expected != nil {
			mc.check(a.Body, expected, child, eff)
		} else {
			mc.infer(a.Body, child, eff)
		}
	}
	ex.report(x)
}

func (mc *modCtx) checkBlock(x *syntax.Block, expected types.Type, env *env, eff effect) {
	c := mc.c
	kind := ""
	if app, ok := c.u.Apply(expected).(*types.App); ok && (app.Con == "Query" || app.Con == "Task") {
		kind = app.Con
	}
	child := env.child()
	for _, s := range x.Stmts {
		var t types.Type
		if s.Type != nil {
			t = mc.typeOf(s.Type, nil)
		}
		if s.Bind {
			if kind == "" {
				c.errorf("E0402", s.Span, "`let %s <-` is only allowed in a block that returns `Query` or `Task`", s.Name)
				mc.infer(s.Expr, child, eff)
				continue
			}
			inner := t
			if inner == nil {
				inner = c.u.Fresh()
			}
			got := c.u.Apply(mc.infer(s.Expr, child, effect{kind: kind, inLoad: eff.inLoad}))
			app, isApp := got.(*types.App)
			switch {
			case isApp && app.Con == kind:
				c.u.Unify(app.Args[0], inner)
			case isApp && app.Con == "Query" && kind == "Task":
				c.u.Unify(app.Args[0], inner)
			case isApp && app.Con == "Task" && kind == "Query":
				c.errorf("E0401", s.Expr.GetSpan(), "`Task` cannot run inside a `Query`; `load` and `Query` blocks can only read")
			default:
				mc.mismatch(s.Expr.GetSpan(), got, &types.App{Con: kind, Args: []types.Type{inner}})
			}
			child.bind(s.Name, inner)
			continue
		}
		if t != nil {
			mc.check(s.Expr, t, child, effect{kind: kind, inLoad: eff.inLoad})
		} else {
			t = mc.infer(s.Expr, child, effect{kind: kind, inLoad: eff.inLoad})
		}
		child.bind(s.Name, t)
	}
	if x.Result != nil {
		mc.check(x.Result, expected, child, effect{kind: kind, inLoad: eff.inLoad})
	}
}

// infer returns e's type, reporting errors inside e.
func (mc *modCtx) infer(e syntax.Expr, env *env, eff effect) types.Type {
	c := mc.c
	switch x := e.(type) {
	case *syntax.IntLit:
		return types.Int
	case *syntax.FloatLit:
		return types.Float
	case *syntax.BoolLit:
		return types.Bool
	case *syntax.UnitLit:
		return types.Unit
	case *syntax.StrLit:
		for _, p := range x.Parts {
			if p.Expr != nil {
				mc.check(p.Expr, types.Text, env, eff)
			}
		}
		return types.Text
	case *syntax.ListLit:
		elem := c.u.Fresh()
		for _, el := range x.Elems {
			mc.check(el, elem, env, eff)
		}
		return &types.App{Con: "List", Args: []types.Type{elem}}
	case *syntax.RecordLit:
		rec := &types.Record{Anonymous: true}
		for _, f := range x.Fields {
			rec.Fields = append(rec.Fields, types.Field{Name: f.Name, Type: mc.infer(f.Value, env, eff)})
		}
		return rec
	case *syntax.RecordUpdate:
		base := c.u.Apply(mc.infer(x.Base, env, eff))
		rec, ok := base.(*types.Record)
		if !ok {
			c.errorf(diag.TypeMismatch, x.Span, "record update on a non-record `%s`", types.String(base))
			return base
		}
		for _, fi := range x.Fields {
			found := false
			for _, f := range rec.Fields {
				if f.Name == fi.Name {
					found = true
					mc.check(fi.Value, f.Type, env, eff)
				}
			}
			if !found {
				c.errorf(diag.UnknownName, fi.Span, "no field `%s` on `%s`", fi.Name, types.String(rec))
				mc.infer(fi.Value, env, eff)
			}
		}
		return rec
	case *syntax.Var:
		return mc.inferVar(x, env)
	case *syntax.CtorRef:
		return mc.inferCtor(x, false)
	case *syntax.Access:
		return mc.inferAccess(x, env, eff)
	case *syntax.App:
		return mc.inferApp(x, env, eff)
	case *syntax.Lambda:
		child := env.child()
		fn := &types.Fun{}
		for _, p := range x.Params {
			v := c.u.Fresh()
			child.bind(p.Name, v)
			fn.Params = append(fn.Params, v)
		}
		fn.Result = mc.infer(x.Body, child, eff)
		return fn
	case *syntax.If:
		mc.check(x.Cond, types.Bool, env, eff)
		t := mc.infer(x.Then, env, eff)
		mc.check(x.Else, t, env, eff)
		return t
	case *syntax.Match:
		v := c.u.Fresh()
		mc.checkMatch(x, v, env, eff)
		return v
	case *syntax.Block:
		v := c.u.Fresh()
		// a block with `let <-` and no expectation: decide the kind from the first bound expression
		for _, s := range x.Stmts {
			if s.Bind {
				probe := c.u.Apply(mc.infer(s.Expr, env, eff))
				if app, ok := probe.(*types.App); ok && (app.Con == "Query" || app.Con == "Task") {
					res := c.u.Fresh()
					mc.checkBlock(x, &types.App{Con: app.Con, Args: []types.Type{res}}, env, eff)
					return &types.App{Con: app.Con, Args: []types.Type{res}}
				}
				break
			}
		}
		mc.checkBlock(x, v, env, eff)
		return v
	case *syntax.BinOp:
		return mc.inferBinOp(x, env, eff)
	case *syntax.HoleExpr:
		v := c.u.Fresh()
		mc.holes = append(mc.holes, holeNote{x.Name, v, x.Span})
		return v
	case *syntax.Element, *syntax.HtmlFor, *syntax.HtmlIf, *syntax.HtmlMatch, *syntax.HtmlExpr, *syntax.HtmlText:
		v := &types.App{Con: "Html", Args: []types.Type{c.u.Fresh()}}
		mc.checkHTML(e, v, env, eff)
		return v
	}
	return c.u.Fresh()
}

func (mc *modCtx) inferVar(x *syntax.Var, env *env) types.Type {
	c := mc.c
	switch x.Ref.Kind {
	case syntax.RefLocal:
		if t, ok := env.lookup(x.Name); ok {
			return t
		}
	case syntax.RefValue, syntax.RefStd:
		if t, ok := c.values[c.key(x.Ref.Module, x.Ref.Name)]; ok {
			return c.u.Instantiate(t)
		}
	case syntax.RefColumn:
		if info, ok := c.tables[c.key(x.Ref.Module, x.Module)]; ok {
			return &types.App{Con: "Column", Args: []types.Type{info.Record, info.Columns[x.Name]}}
		}
	case syntax.RefCapability:
		capName, entry := x.Module, x.Name
		if entries, ok := c.caps[c.key(x.Ref.Module, capName)]; ok {
			if t, ok := entries[entry]; ok {
				return c.u.Instantiate(t)
			}
			c.errorf(diag.UnknownName, x.Span, "capability `%s` has no `%s`", capName, entry)
		}
	}
	return c.u.Fresh()
}

// inferCtor types a constructor reference. inPattern=false: bare ctor with a non-empty payload is E0301.
func (mc *modCtx) inferCtor(x *syntax.CtorRef, inPattern bool) types.Type {
	c := mc.c
	switch x.Ref.Kind {
	case syntax.RefPage:
		info, ok := c.pages[x.Ref.Module]
		if !ok {
			return types.Page
		}
		return &types.Fun{Params: []types.Type{info.Params}, Result: types.Page}
	case syntax.RefTable:
		if info, ok := c.tables[c.key(x.Ref.Module, x.Ref.Name)]; ok {
			return &types.App{Con: "Table", Args: []types.Type{info.Record}}
		}
	case syntax.RefCtor:
		if x.Name == "True" || x.Name == "False" {
			return types.Bool
		}
		info, ok := c.ctors[c.key(x.Ref.Module, x.Ref.Name)]
		if !ok {
			return c.u.Fresh()
		}
		un := types.Type(info.Union)
		if len(info.Union.Params) > 0 {
			un = c.u.Instantiate(&types.Union{Module: info.Union.Module, Name: info.Union.Name, Ctors: info.Union.Ctors, Params: info.Union.Params, Args: paramsOf(info.Union.Params)})
		}
		if info.Arg == nil {
			return un
		}
		arg := info.Arg
		if u2, ok := un.(*types.Union); ok && len(u2.Args) > 0 {
			arg, _ = types.CtorArg(u2, x.Name)
		}
		return &types.Fun{Params: []types.Type{arg}, Result: un}
	}
	return c.u.Fresh()
}

func paramsOf(names []string) []types.Type {
	var out []types.Type
	for _, n := range names {
		out = append(out, &types.Param{Name: n})
	}
	return out
}

// messageLike reports whether a ctor's payload record has no HTML-supplied fields (only `form` or none).
func messageLike(rec *types.Record) bool {
	for _, f := range rec.Fields {
		if f.Name != "form" {
			return false
		}
	}
	return true
}

func (mc *modCtx) inferAccess(x *syntax.Access, env *env, eff effect) types.Type {
	c := mc.c
	base := c.u.Apply(mc.infer(x.Expr, env, eff))
	switch b := base.(type) {
	case *types.Record:
		for _, f := range b.Fields {
			if f.Name == x.Field {
				return f.Type
			}
		}
		c.errorf(diag.UnknownName, x.Span, "no field `%s` on `%s`", x.Field, types.String(b))
	case *types.Union:
		if inner, ok := maybeOf(b); ok {
			src := mc.source(x.Expr.GetSpan())
			d := c.errorf("E0102", x.Span, "`.%s` on a `Maybe %s`; match it first", x.Field, types.String(inner))
			d.Fix = &diag.Fix{Line: x.Span.Start.Line, Col: x.Span.Start.Col, Replace: fmt.Sprintf("(match %s { Just x -> x.%s, Nothing -> ?default })", src, x.Field)}
			return c.u.Fresh()
		}
		c.errorf(diag.TypeMismatch, x.Span, "`%s` has no fields; it is a union", types.String(b))
	case *types.App:
		if b.Con == "Column" && len(b.Args) == 2 {
			return mc.columnMethod(x, b.Args[0], b.Args[1])
		}
		c.errorf(diag.TypeMismatch, x.Span, "`%s` has no field `%s`", types.String(b), x.Field)
	case *types.Var:
		c.errorf(diag.TypeMismatch, x.Span, "cannot infer the record type of `%s`; add a type annotation", mc.source(x.Expr.GetSpan()))
	default:
		c.errorf(diag.TypeMismatch, x.Span, "`%s` has no field `%s`", types.String(b), x.Field)
	}
	return c.u.Fresh()
}

func (mc *modCtx) columnMethod(x *syntax.Access, table, field types.Type) types.Type {
	c := mc.c
	cond := &types.App{Con: "Condition", Args: []types.Type{table}}
	order := &types.App{Con: "Order", Args: []types.Type{table}}
	ft := c.u.Apply(field)
	isTextLike := isCon(ft, "Text") || func() bool { b, ok := ft.(*types.Brand); return ok && b.Base == types.Text }()
	switch x.Field {
	case "eq", "ne":
		return &types.Fun{Params: []types.Type{ft}, Result: cond}
	case "lt", "le", "gt", "ge":
		if !comparable(ft) {
			c.errorf(diag.TypeMismatch, x.Span, "`.%s` needs an ordered column, got `%s`", x.Field, types.String(ft))
		}
		return &types.Fun{Params: []types.Type{ft}, Result: cond}
	case "contains":
		if !isTextLike {
			c.errorf(diag.TypeMismatch, x.Span, "`.contains` needs a Text column, got `%s`", types.String(ft))
		}
		return &types.Fun{Params: []types.Type{types.Text}, Result: cond}
	case "in":
		return &types.Fun{Params: []types.Type{&types.App{Con: "List", Args: []types.Type{ft}}}, Result: cond}
	case "isNothing", "isJust":
		if _, ok := maybeOf(ft); !ok {
			c.errorf(diag.TypeMismatch, x.Span, "`.%s` needs a Maybe column, got `%s`", x.Field, types.String(ft))
		}
		return cond
	case "asc", "desc":
		return order
	}
	c.errorf(diag.UnknownName, x.Span, "columns have no `.%s`; use .eq .ne .lt .le .gt .ge .contains .in .isNothing .isJust .asc .desc", x.Field)
	return c.u.Fresh()
}

func comparable(t types.Type) bool {
	switch t := t.(type) {
	case *types.Con:
		return t.Name == "Int" || t.Name == "Float" || t.Name == "Text" || t.Name == "Date" || t.Name == "Time"
	case *types.Brand:
		return true
	case *types.Union:
		return isScalar(t)
	}
	return false
}

func (mc *modCtx) inferApp(x *syntax.App, env *env, eff effect) types.Type {
	c := mc.c
	// constructor applied to a record literal: check against payload (form field implicit)
	if ct, ok := x.Fn.(*syntax.CtorRef); ok && len(x.Args) == 1 {
		if lit, isLit := x.Args[0].(*syntax.RecordLit); isLit {
			fnT := c.u.Apply(mc.inferCtor(ct, false))
			if fn, isFn := fnT.(*types.Fun); isFn && len(fn.Params) == 1 {
				if rec, isRec := c.u.Apply(fn.Params[0]).(*types.Record); isRec {
					mc.checkRecordLit(lit, rec, env, eff)
					return fn.Result
				}
			}
		}
	}
	// E0101: Db.get T x with x not an id
	if v, ok := x.Fn.(*syntax.Var); ok && v.Module == "Db" && v.Name == "get" && len(x.Args) == 2 {
		if r := mc.checkDbGet(x, env, eff); r != nil {
			return r
		}
	}
	fnT := c.u.Apply(mc.infer(x.Fn, env, eff))
	fn, ok := fnT.(*types.Fun)
	if !ok {
		if v, isVar := fnT.(*types.Var); isVar {
			// unknown function type: build one from the args
			var ps []types.Type
			for _, a := range x.Args {
				ps = append(ps, mc.infer(a, env, eff))
			}
			res := c.u.Fresh()
			c.u.Unify(v, &types.Fun{Params: ps, Result: res})
			return res
		}
		if ct, isCtor := x.Fn.(*syntax.CtorRef); isCtor {
			c.errorf(diag.Arity, x.Span, "`%s` takes no arguments", ct.Name)
		} else {
			c.errorf(diag.TypeMismatch, x.Fn.GetSpan(), "`%s` is not a function; it has type `%s`", mc.source(x.Fn.GetSpan()), types.String(fnT))
		}
		for _, a := range x.Args {
			mc.infer(a, env, eff)
		}
		return c.u.Fresh()
	}
	if len(x.Args) > len(fn.Params) {
		c.errorf(diag.Arity, x.Span, "too many arguments: `%s` takes %d, got %d", mc.source(x.Fn.GetSpan()), len(fn.Params), len(x.Args))
	}
	for i, a := range x.Args {
		if i < len(fn.Params) {
			mc.check(a, fn.Params[i], env, eff)
		} else {
			mc.infer(a, env, eff)
		}
	}
	if len(x.Args) < len(fn.Params) {
		return &types.Fun{Params: fn.Params[len(x.Args):], Result: fn.Result}
	}
	return fn.Result
}

func (mc *modCtx) inferBinOp(x *syntax.BinOp, env *env, eff effect) types.Type {
	c := mc.c
	switch x.Op {
	case "|>":
		fnT := c.u.Apply(mc.infer(x.R, env, eff))
		fn, ok := fnT.(*types.Fun)
		if !ok {
			c.errorf(diag.TypeMismatch, x.R.GetSpan(), "right side of `|>` must be a function, got `%s`", types.String(fnT))
			mc.infer(x.L, env, eff)
			return c.u.Fresh()
		}
		mc.check(x.L, fn.Params[0], env, eff)
		if len(fn.Params) > 1 {
			return &types.Fun{Params: fn.Params[1:], Result: fn.Result}
		}
		return fn.Result
	case "??":
		inner := c.u.Fresh()
		mc.check(x.L, &types.Union{Module: "Prelude", Name: "Maybe", Args: []types.Type{inner}, Ctors: mc.maybeCtors(), Params: []string{"a"}}, env, eff)
		mc.check(x.R, inner, env, eff)
		return inner
	case "&&", "||":
		mc.check(x.L, types.Bool, env, eff)
		mc.check(x.R, types.Bool, env, eff)
		return types.Bool
	case "==", "!=":
		l := c.u.Apply(mc.infer(x.L, env, eff))
		r := c.u.Apply(mc.infer(x.R, env, eff))
		if err := c.u.Unify(l, r); err != nil {
			c.errorf(diag.TypeMismatch, x.Span, "cannot compare `%s` with `%s`: `==` needs both sides to have the same type", types.String(l), types.String(r))
		} else if containsFun(c.u.Apply(l)) {
			c.errorf(diag.TypeMismatch, x.Span, "functions cannot be compared")
		}
		return types.Bool
	case "<", "<=", ">", ">=":
		l := c.u.Apply(mc.infer(x.L, env, eff))
		mc.check(x.R, l, env, eff)
		if l2 := c.u.Apply(l); !comparable(l2) {
			if _, isVar := l2.(*types.Var); !isVar {
				c.errorf(diag.TypeMismatch, x.Span, "`%s` is not ordered; comparison needs Int, Float, Text, Date, Time or a union without payloads", types.String(l2))
			}
		}
		return types.Bool
	case "++":
		l := c.u.Apply(mc.infer(x.L, env, eff))
		if app, ok := l.(*types.App); ok && app.Con == "List" {
			mc.check(x.R, l, env, eff)
			return l
		}
		if err := c.u.Check(l, types.Text); err != nil {
			mc.mismatch(x.L.GetSpan(), l, types.Text)
		}
		mc.check(x.R, types.Text, env, eff)
		return types.Text
	case "+", "-", "*", "/", "%":
		l := c.u.Apply(mc.infer(x.L, env, eff))
		r := c.u.Apply(mc.infer(x.R, env, eff))
		return mc.arith(x, l, r)
	}
	return c.u.Fresh()
}

func (mc *modCtx) maybeCtors() []types.Ctor {
	if u, ok := mc.c.types["Prelude.Maybe"].(*types.Union); ok {
		return u.Ctors
	}
	return nil
}

func containsFun(t types.Type) bool {
	switch t := t.(type) {
	case *types.Fun:
		return true
	case *types.App:
		for _, a := range t.Args {
			if containsFun(a) {
				return true
			}
		}
	case *types.Record:
		for _, f := range t.Fields {
			if containsFun(f.Type) {
				return true
			}
		}
	}
	return false
}

func numericBase(t types.Type) (types.Type, bool) {
	switch t := t.(type) {
	case *types.Con:
		if t.Name == "Int" || t.Name == "Float" {
			return t, true
		}
	case *types.Brand:
		if b, ok := t.Base.(*types.Con); ok && (b.Name == "Int" || b.Name == "Float") {
			return t, true
		}
	}
	return nil, false
}

func (mc *modCtx) arith(x *syntax.BinOp, l, r types.Type) types.Type {
	c := mc.c
	if v, ok := l.(*types.Var); ok {
		c.u.Unify(v, r)
		l = c.u.Apply(l)
	}
	if v, ok := r.(*types.Var); ok {
		c.u.Unify(v, l)
		r = c.u.Apply(r)
	}
	lt, lok := numericBase(l)
	_, rok := numericBase(r)
	if !lok || !rok {
		msg := fmt.Sprintf("expected Int, got %s", types.String(firstNonNumeric(l, r)))
		if isCon(l, "Text") || isCon(r, "Text") {
			msg += "; `++` joins Text, `+` is for numbers"
		}
		c.errorf(diag.TypeMismatch, x.Span, "%s", msg)
		return c.u.Fresh()
	}
	if err := c.u.Unify(l, r); err != nil {
		// Brand + base: allowed by widening, result is the base
		if err2 := c.u.Check(l, r); err2 == nil {
			return r
		}
		if err3 := c.u.Check(r, l); err3 == nil {
			return l
		}
		c.errorf(diag.TypeMismatch, x.Span, "cannot apply `%s` to `%s` and `%s`", x.Op, types.String(l), types.String(r))
		return c.u.Fresh()
	}
	return lt
}

func firstNonNumeric(l, r types.Type) types.Type {
	if _, ok := numericBase(l); !ok {
		return l
	}
	return r
}

// checkDbGet handles `Db.get T x`; returns the result type when the call is well-formed or diagnosed, nil to fall through.
func (mc *modCtx) checkDbGet(x *syntax.App, env *env, eff effect) types.Type {
	c := mc.c
	tableT := c.u.Apply(mc.infer(x.Args[0], env, eff))
	app, ok := tableT.(*types.App)
	if !ok || app.Con != "Table" {
		return nil
	}
	rec, ok := c.u.Apply(app.Args[0]).(*types.Record)
	if !ok {
		return nil
	}
	info := c.u.Table(rec)
	if info == nil {
		return nil
	}
	argT := c.u.Apply(mc.infer(x.Args[1], env, eff))
	res := &types.App{Con: "Query", Args: []types.Type{&types.Union{Module: "Prelude", Name: "Maybe", Args: []types.Type{rec}, Ctors: mc.maybeCtors(), Params: []string{"a"}}}}
	if err := c.u.Check(argT, info.IDType); err == nil {
		return res
	}
	d := c.errorf("E0101", x.Args[1].GetSpan(), "`Db.get %s` takes a `%s`, got `%s`", rec.Name, types.String(info.IDType), types.String(argT))
	// fix: a column of the argument's type, if exactly one exists
	var cols []string
	for name, ct := range info.Columns {
		if name != info.IDField && c.u.Check(argT, ct) == nil {
			cols = append(cols, name)
		}
	}
	if len(cols) == 1 {
		d.Fix = &diag.Fix{Line: x.Span.Start.Line, Col: x.Span.Start.Col,
			Replace: fmt.Sprintf("Db.from %s |> Db.where (%s.%s.eq %s) |> Db.first", rec.Name, rec.Name, cols[0], mc.source(x.Args[1].GetSpan()))}
	}
	return res
}
```

Add `holes []holeNote` to `modCtx` in `check.go`.

- [ ] **Step 4: Implement pattern.go**

```go
// internal/check/pattern.go
package check

import (
	"fmt"
	"strings"

	"alder/internal/diag"
	"alder/internal/syntax"
	"alder/internal/types"
)

// checkPattern checks p against t, binding variables in env.
func (mc *modCtx) checkPattern(p syntax.Pattern, t types.Type, env *env) {
	mc.checkPatternDepth(p, t, env, 0)
}

func (mc *modCtx) checkPatternDepth(p syntax.Pattern, t types.Type, env *env, depth int) {
	c := mc.c
	t = c.u.Apply(t)
	switch p := p.(type) {
	case *syntax.PWild:
	case *syntax.PVar:
		env.bind(p.Name, t)
	case *syntax.PLit:
		lt := mc.infer(p.Lit, env, effect{})
		if err := c.u.Unify(t, lt); err != nil {
			mc.mismatch(p.Span, lt, t)
		}
	case *syntax.PCtor:
		if p.Name == "True" || p.Name == "False" {
			if err := c.u.Unify(t, types.Bool); err != nil {
				mc.mismatch(p.Span, types.Bool, t)
			}
			return
		}
		un, ok := t.(*types.Union)
		if !ok {
			if v, isVar := t.(*types.Var); isVar {
				if info, found := c.ctors[c.key(p.Ref.Module, p.Ref.Name)]; found {
					inst := c.u.Instantiate(&types.Union{Module: info.Union.Module, Name: info.Union.Name, Ctors: info.Union.Ctors, Params: info.Union.Params, Args: paramsOf(info.Union.Params)})
					c.u.Unify(v, inst)
					mc.checkPatternDepth(p, inst, env, depth)
					return
				}
			}
			c.errorf(diag.TypeMismatch, p.Span, "pattern `%s` is a constructor, but the value has type `%s`", p.Name, types.String(t))
			return
		}
		arg, found := types.CtorArg(un, p.Name)
		if !found {
			c.errorf(diag.TypeMismatch, p.Span, "`%s` is not a constructor of `%s`", p.Name, types.String(un))
			return
		}
		if p.Arg == nil {
			if arg != nil {
				c.errorf(diag.TypeMismatch, p.Span, "constructor `%s` carries a value; write `%s x`", p.Name, p.Name)
			}
			return
		}
		if arg == nil {
			c.errorf(diag.TypeMismatch, p.Span, "constructor `%s` carries no value", p.Name)
			return
		}
		if inner, isCtor := p.Arg.(*syntax.PCtor); isCtor && inner.Arg != nil {
			c.errorf(diag.ParseError, inner.Span, "patterns nest one level; match on the inner value in a second `match`")
			return
		}
		mc.checkPatternDepth(p.Arg, arg, env, depth+1)
	case *syntax.PRecord:
		rec, ok := t.(*types.Record)
		if !ok {
			c.errorf(diag.TypeMismatch, p.Span, "record pattern against `%s`", types.String(t))
			return
		}
		for _, f := range p.Fields {
			var ft types.Type
			for _, rf := range rec.Fields {
				if rf.Name == f.Name {
					ft = rf.Type
				}
			}
			if ft == nil {
				c.errorf(diag.UnknownName, f.Span, "no field `%s` on `%s`", f.Name, types.String(rec))
				continue
			}
			mc.checkPatternDepth(f.Pat, ft, env, depth+1)
		}
	case *syntax.PList:
		elem := c.u.Fresh()
		if err := c.u.Unify(t, &types.App{Con: "List", Args: []types.Type{elem}}); err != nil {
			mc.mismatch(p.Span, &types.App{Con: "List", Args: []types.Type{elem}}, t)
			return
		}
		for _, e := range p.Elems {
			mc.checkPatternDepth(e, elem, env, depth+1)
		}
		if p.Rest != "" {
			env.bind(p.Rest, &types.App{Con: "List", Args: []types.Type{elem}})
		}
	}
}

// ---- exhaustiveness

type exhaustiveness struct {
	mc      *modCtx
	subject types.Type
	ctors   map[string]bool // for unions: covered ctor names
	bools   map[bool]bool
	listNil bool
	listCons bool
	closed  bool // a wildcard/var arm has been seen, or every case is covered
	kind    string // "union" | "bool" | "list" | "other"
	all     []string
}

func newExhaustiveness(mc *modCtx, subject types.Type) *exhaustiveness {
	ex := &exhaustiveness{mc: mc, subject: subject, ctors: map[string]bool{}, bools: map[bool]bool{}, kind: "other"}
	switch t := subject.(type) {
	case *types.Union:
		ex.kind = "union"
		for _, ct := range t.Ctors {
			ex.all = append(ex.all, ct.Name)
		}
	case *types.Con:
		if t.Name == "Bool" {
			ex.kind = "bool"
		}
	case *types.App:
		if t.Con == "List" {
			ex.kind = "list"
		}
	case *types.Record:
		ex.kind = "record"
	}
	return ex
}

func (ex *exhaustiveness) cover(p syntax.Pattern) {
	switch p := p.(type) {
	case *syntax.PWild, *syntax.PVar:
		ex.closed = true
	case *syntax.PRecord:
		ex.closed = true
	case *syntax.PCtor:
		if p.Name == "True" || p.Name == "False" {
			ex.bools[p.Name == "True"] = true
			if ex.bools[true] && ex.bools[false] {
				ex.closed = true
			}
			return
		}
		if ex.kind == "union" {
			if ex.ctors[p.Name] && irrefutable(p.Arg) {
				ex.mc.c.errorf(diag.UnreachableArm, p.Span, "unreachable arm: `%s` is already handled", p.Name)
			}
			if irrefutable(p.Arg) {
				ex.ctors[p.Name] = true
			}
			if len(ex.ctors) == len(ex.all) {
				ex.closed = true
			}
		}
	case *syntax.PLit:
		if b, ok := p.Lit.(*syntax.BoolLit); ok {
			ex.bools[b.Value] = true
			if ex.bools[true] && ex.bools[false] {
				ex.closed = true
			}
		}
	case *syntax.PList:
		if len(p.Elems) == 0 && p.Rest == "" {
			ex.listNil = true
		} else if len(p.Elems) == 1 && p.Rest != "" && irrefutable(p.Elems[0]) {
			ex.listCons = true
		}
		if ex.listNil && ex.listCons {
			ex.closed = true
		}
	}
}

func irrefutable(p syntax.Pattern) bool {
	switch p := p.(type) {
	case nil, *syntax.PWild, *syntax.PVar:
		return true
	case *syntax.PRecord:
		for _, f := range p.Fields {
			if !irrefutable(f.Pat) {
				return false
			}
		}
		return true
	}
	return false
}

func (ex *exhaustiveness) report(m *syntax.Match) {
	if ex.closed {
		return
	}
	var missing []string
	switch ex.kind {
	case "union":
		for _, n := range ex.all {
			if !ex.ctors[n] {
				missing = append(missing, "`"+n+"`")
			}
		}
	case "bool":
		for _, b := range []bool{true, false} {
			if !ex.bools[b] {
				missing = append(missing, fmt.Sprintf("`%v`", map[bool]string{true: "True", false: "False"}[b]))
			}
		}
	case "list":
		if !ex.listNil {
			missing = append(missing, "`[]`")
		}
		if !ex.listCons {
			missing = append(missing, "`[x, ..rest]`")
		}
	case "record":
		return
	default:
		missing = append(missing, "`_`")
	}
	if len(missing) == 0 {
		return
	}
	d := ex.mc.c.errorf(diag.NonExhaustive, m.Span, "missing cases: %s", strings.Join(missing, ", "))
	var arms []string
	for _, mcase := range missing {
		name := strings.Trim(mcase, "`")
		if ex.kind == "union" {
			if arg, _ := types.CtorArg(ex.subject.(*types.Union), name); arg != nil {
				name += " x"
			}
		}
		arms = append(arms, "  "+name+" -> ?todo,")
	}
	d.Fix = &diag.Fix{Line: m.Span.End.Line, Col: 1, Replace: strings.Join(arms, "\n")}
}
```

`m.Span.End.Line` for a match is the line of the closing `}`; the fix inserts the missing arms before it (the golden expects line 18 for a match whose `}` is on line 18).

- [ ] **Step 5: Add a `checkHTML` and `checkPage` stub so the package compiles**

In a new file `internal/check/html.go` (replaced in Task 10) and `pages.go` (replaced in Task 11):

```go
package check
func (mc *modCtx) checkHTML(e syntax.Expr, expected types.Type, env *env, eff effect) {}
func (mc *modCtx) checkPage() {}
```

- [ ] **Step 6: Run the golden and iterate**

Run: `go test ./internal/check/ -run 'TestCheckGolden'`
Expected: both `check_decls` and `check_exprs` PASS.

- [ ] **Step 7: Commit**

```bash
git add internal/check testdata/golden && git commit -m "m1: expression checker, effect blocks, patterns and exhaustiveness"
```

---

### Task 10: Checker — HTML

**Files:**
- Replace: `internal/check/html.go`
- Create: `testdata/golden/check_html/app/*.alder`, `testdata/golden/check_html.diag`

**Interfaces:**
- Produces: `func (mc *modCtx) checkHTML(e syntax.Expr, expected types.Type, env *env, eff effect)` where `expected` is `Html m` (m may be a Var). Also `func (mc *modCtx) msgParam() types.Type` returning the page's `Msg` union, or a fresh `Var` in non-page modules.
- Rules (chapter 5):
  - `expected` must be `App{Html,[m]}`; otherwise E0003 "expected X, got Html".
  - `Element` with lowercase tag: tag ∈ HTML5 set (`htmlTags` below) else **E0002** "unknown element `<x>`". Attributes: by name → expected type: boolean attrs (`disabled checked required hidden selected readonly autofocus multiple open`) → `Bool`; `href` → `Page` (a `Text` gives **E0303** "`href` takes a page value, not a string; write `href={Module.Page { … }}` or `Url.external \"…\"`"); `click`/`submit` → `m` (the message type: a page `Msg` ctor, or an app-level ctor); any other attribute (`class id value placeholder name type for alt src rel charset title aria-* data-* …`) → `Text` or `Maybe Text`: check against `Text`; if that fails, check against `Maybe Text`; if both fail, report the `Text` mismatch. `onclick`/`onsubmit`/`on*` → **E0001**-class error "`onclick` is not Alder; use `click={Msg}`". A literal string attribute value is `Text`; for `href` a literal is E0303.
  - Message-typed attributes (`click`, `submit`): the value is checked against `m`; additionally an **app-level** ctor (`Ref.Module == "App"`) is accepted for any `m` (`c.appMsg` unifies with nothing, so handle before checking: if the attribute expr is a `CtorRef`/`App` whose ctor resolves to module `App`, check it against `c.appMsg` instead). A bare `CtorRef` whose payload has HTML-side fields → **E0301** with fix `Name { field: ?field, … }`.
  - `<form submit={M}>`: `M`'s ctor payload may have a `form` field of record type `F`; push `F` as the current form (`mc.form = F`) while checking children. `<Input>`, `<Textarea>`, `<Select>`, `<Errors>` outside a form → **E0304** "`<Input>` must be inside `<form submit={…}>`". `field="x"` must be a literal string naming a field of `F` → else **E0304** "form `F` has no field `x`; fields: a, b". `<Select>` requires a payload-less union field; `<Input>` on a union field → E0304 "use `<Select>` for `Priority`". `value={…}` is checked against the field type or `Maybe` of it. `label` is `Text`. `<Errors form />` (bare `form` attr) or `<Errors field="x" />`. A plain `<input>`/`<textarea>`/`<select>` inside a bound form → **E0304** "plain `<input>` inside `<form submit>`; use `<Input field=\"…\">`".
  - Component element (`Ref.Kind == RefComponent`, non-builtin): its type is `values[key]` = `Fun{[Record props], Html r}`; build a record literal from attributes (`title="x"` → `StrLit`, `theme={e}` → e) plus `children` when the element has children (type `List (Html m)`); check that literal against the props record (**E0003** on missing/extra props, via `checkRecordLit`); unify the result `Html r` with `Html m` using `Check` (so `Html Never`/`Html a` components fit).
  - Children: `HtmlText` → fine; `HtmlExpr{e}` → infer `e`; accept if it checks against `Text` (brands widen), `Html m`, or `List (Html m)`; else **E0003** "expected Text, Html or List Html in `{…}`, got X" with the `Text.fromInt` fix when X is `Int`. `HtmlFor` → list expr against `List a`, bind var, body is an HTML child; `HtmlIf` → cond `Bool`, both branches HTML children; `HtmlMatch` → like `checkMatch` with HTML-child bodies.
  - Closing: `Element` typed `Html m`.

- [ ] **Step 1: Write the golden case**

`testdata/golden/check_html/app/App.alder`:
```alder
module App

session { user: Maybe Text }

login = Pg

msg Logout

update : Msg -> Session -> Task Nav
update msg session = Task.succeed Rerender

error : Error -> Html Never
error err = <h1>x</h1>

notFound : Html Never
notFound = <h1>x</h1>
```

`testdata/golden/check_html/app/Layout.alder`:
```alder
module Layout

Page : { title: Text, children: List (Html a) } -> Html a
Page props = <html><head><title>{props.title}</title></head><body>{props.children}<button click={App.Logout}>out</button></body></html>

Bad : { x: Int } -> Html Msg
Bad props = <span>{props.x}</span>
```

`testdata/golden/check_html/app/Pg.alder`:
```alder
module Pg

import Layout

page "/" { }

type Model = { n: Int, name: Text, saving: Bool, items: List Text }

load : Params -> Session -> Query Model
load params session = Query.succeed { n: 1, name: "x", saving: False, items: [] }

form F = { title: Text.NonEmpty, kind: Kind }

type Kind = A | B

msg Submit form F
msg Toggle { id: Int }

update : Msg -> Model -> Session -> Task Nav
update msg model session = match msg {
  Submit p -> Task.succeed Rerender,
  Toggle p -> Task.succeed Rerender,
}

view : Model -> Session -> Html Msg
view model session =
  <Layout.Page title="T">
    <p>{model.n}</p>
    <a href="/somewhere">x</a>
    <button onclick={Toggle { id: 1 }}>x</button>
    <button click={Toggle}>x</button>
    <button click={Toggle { id: 1 }} disabled={model.saving}>ok</button>
    <blink>x</blink>
    <Input field="title" label="T" />
    <form submit={Submit}>
      <Input field="nope" label="N" />
      <Input field="kind" label="K" />
      <Select field="kind" label="K" value={A} />
      <input name="raw" />
      <Errors form />
      <Errors field="title" />
      <button>Go</button>
    </form>
    <ul>{for it in model.items -> <li>{it}</li>}</ul>
    {if model.saving then <p>s</p> else Html.none}
    {match session.user {
      Just u -> <span>{u}</span>,
      Nothing -> <a href={Pg {}}>in</a>,
    }}
    <Layout.Page title={model.n}>x</Layout.Page>
  </Layout.Page>
```

`testdata/golden/check_html.diag`:
```
app/Layout.alder:6:30 E0601 `Msg` is only defined in page modules; a shared component is typed `Html a`
  fix 6:30 a
app/Pg.alder:27:9 E0003 expected Text, Html or List Html in `{…}`, got Int
  fix 27:9 Text.fromInt (model.n)
app/Pg.alder:28:8 E0303 `href` takes a page value, not a string; write `href={Module.Page { … }}` or `Url.external "…"`
app/Pg.alder:29:13 E0001 `onclick` is not Alder; use `click={Msg}`
app/Pg.alder:30:20 E0301 message `Toggle` needs its parameters: `Toggle { id: ?id }`
  fix 30:20 Toggle { id: ?id }
app/Pg.alder:32:5 E0002 unknown element `<blink>`
app/Pg.alder:33:5 E0304 `<Input>` must be inside `<form submit={…}>`
app/Pg.alder:35:7 E0304 form `F` has no field `nope`; fields: title, kind
app/Pg.alder:36:7 E0304 field `kind` is a union; use `<Select>` for `Pg.Kind`
app/Pg.alder:38:7 E0304 plain `<input>` inside `<form submit>`; use `<Input field="…">`
app/Pg.alder:48:25 E0003 expected Text, got Int
  fix 48:25 Text.fromInt (model.n)
```

- [ ] **Step 2: Run to verify failure**

Run: `go test ./internal/check/ -run TestCheckGolden/check_html`
Expected: FAIL (no HTML diagnostics yet).

- [ ] **Step 3: Implement html.go**

```go
// internal/check/html.go
package check

import (
	"strings"

	"alder/internal/diag"
	"alder/internal/syntax"
	"alder/internal/types"
)

var htmlTags = map[string]bool{}

func init() {
	for _, t := range strings.Fields(`a abbr address article aside audio b bdi bdo blockquote body br button canvas caption cite code col colgroup data datalist dd del details dfn dialog div dl dt em embed fieldset figcaption figure footer form h1 h2 h3 h4 h5 h6 head header hgroup hr html i iframe img input ins kbd label legend li link main map mark menu meta meter nav noscript object ol optgroup option output p picture pre progress q rp rt ruby s samp script section select slot small source span strong style sub summary sup table tbody td template textarea tfoot th thead time title tr track u ul var video wbr`) {
		htmlTags[t] = true
	}
}

var boolAttrs = map[string]bool{"disabled": true, "checked": true, "required": true, "hidden": true, "selected": true, "readonly": true, "autofocus": true, "multiple": true, "open": true}

type formCtx struct {
	name string
	rec  *types.Record
}

// msgParam is the message type HTML in this module may send.
func (mc *modCtx) msgParam() types.Type {
	if mc.page != nil {
		return mc.page.Msg
	}
	if mc.isApp {
		return mc.c.appMsg
	}
	return mc.c.u.Fresh()
}

func (mc *modCtx) checkHTML(e syntax.Expr, expected types.Type, env *env, eff effect) {
	c := mc.c
	ex := c.u.Apply(expected)
	app, ok := ex.(*types.App)
	if !ok || app.Con != "Html" {
		if v, isVar := ex.(*types.Var); isVar {
			m := c.u.Fresh()
			c.u.Unify(v, &types.App{Con: "Html", Args: []types.Type{m}})
			mc.htmlNode(e, m, env, eff)
			return
		}
		c.errorf(diag.TypeMismatch, e.GetSpan(), "expected %s, got Html", types.String(ex))
		mc.htmlNode(e, c.u.Fresh(), env, eff)
		return
	}
	mc.htmlNode(e, app.Args[0], env, eff)
}

// htmlNode checks one HTML child against message type m.
func (mc *modCtx) htmlNode(e syntax.Expr, m types.Type, env *env, eff effect) {
	c := mc.c
	htmlT := &types.App{Con: "Html", Args: []types.Type{m}}
	switch x := e.(type) {
	case *syntax.HtmlText:
	case *syntax.Element:
		mc.element(x, m, env, eff)
	case *syntax.HtmlExpr:
		got := c.u.Apply(mc.infer(x.Expr, env, eff))
		if c.u.Check(got, types.Text) == nil || c.u.Check(got, htmlT) == nil || c.u.Check(got, &types.App{Con: "List", Args: []types.Type{htmlT}}) == nil {
			return
		}
		d := c.errorf(diag.TypeMismatch, x.Expr.GetSpan(), "expected Text, Html or List Html in `{…}`, got %s", types.String(got))
		mc.annotateMismatch(d, x.Expr, got, types.Text)
	case *syntax.HtmlFor:
		elem := c.u.Fresh()
		mc.check(x.List, &types.App{Con: "List", Args: []types.Type{elem}}, env, eff)
		child := env.child()
		child.bind(x.Var, elem)
		mc.htmlNode(x.Body, m, child, eff)
	case *syntax.HtmlIf:
		mc.check(x.Cond, types.Bool, env, eff)
		mc.htmlNode(x.Then, m, env, eff)
		mc.htmlNode(x.Else, m, env, eff)
	case *syntax.HtmlMatch:
		subj := c.u.Apply(mc.infer(x.Subject, env, eff))
		exh := newExhaustiveness(mc, subj)
		for _, a := range x.Arms {
			child := env.child()
			if exh.closed {
				c.errorf(diag.UnreachableArm, a.Span, "unreachable arm: every case is already handled")
			}
			mc.checkPattern(a.Pat, subj, child)
			exh.cover(a.Pat)
			mc.htmlNode(a.Body, m, child, eff)
		}
		exh.report(&syntax.Match{Node: x.Node, Subject: x.Subject, Arms: x.Arms})
	default:
		mc.check(e, htmlT, env, eff)
	}
}

func (mc *modCtx) element(x *syntax.Element, m types.Type, env *env, eff effect) {
	c := mc.c
	if x.Ref.Kind == syntax.RefComponent {
		if x.Ref.Module == "Prelude" {
			mc.formComponent(x, env, eff)
			return
		}
		mc.component(x, m, env, eff)
		return
	}
	if isUpperName(x.Tag) { // unresolved component: names already reported
		return
	}
	if !htmlTags[x.Tag] {
		c.errorf(diag.UnknownName, x.Span, "unknown element `<%s>`", x.Tag)
	}
	if (x.Tag == "input" || x.Tag == "textarea" || x.Tag == "select") && mc.form != nil {
		c.errorf("E0304", x.Span, "plain `<%s>` inside `<form submit>`; use `<%s field=\"…\">`", x.Tag, map[string]string{"input": "Input", "textarea": "Textarea", "select": "Select"}[x.Tag])
	}
	var pushed *formCtx
	for _, a := range x.Attrs {
		mc.attribute(x, a, m, env, eff)
		if x.Tag == "form" && a.Name == "submit" && a.Value != nil {
			if f := mc.formOf(a.Value); f != nil {
				pushed = f
			} else {
				pushed = &formCtx{name: "", rec: &types.Record{Anonymous: true}}
			}
		}
	}
	saved := mc.form
	if pushed != nil {
		mc.form = pushed
	}
	for _, ch := range x.Children {
		mc.htmlNode(ch, m, env, eff)
	}
	mc.form = saved
}

func isUpperName(s string) bool { return len(s) > 0 && s[0] >= 'A' && s[0] <= 'Z' }

// formOf finds the form record carried by a message expression (bare ctor or ctor applied to a record).
func (mc *modCtx) formOf(e syntax.Expr) *formCtx {
	c := mc.c
	var ct *syntax.CtorRef
	switch v := e.(type) {
	case *syntax.CtorRef:
		ct = v
	case *syntax.App:
		if r, ok := v.Fn.(*syntax.CtorRef); ok {
			ct = r
		}
	}
	if ct == nil || ct.Ref.Kind != syntax.RefCtor {
		return nil
	}
	info, ok := c.ctors[c.key(ct.Ref.Module, ct.Ref.Name)]
	if !ok {
		return nil
	}
	if rec, ok := info.Arg.(*types.Record); ok {
		for _, f := range rec.Fields {
			if f.Name == "form" {
				if fr, ok := f.Type.(*types.Record); ok {
					return &formCtx{name: fr.Name, rec: fr}
				}
			}
		}
	}
	return &formCtx{name: "", rec: &types.Record{Anonymous: true}}
}

func (mc *modCtx) attribute(el *syntax.Element, a *syntax.HtmlAttr, m types.Type, env *env, eff effect) {
	c := mc.c
	if strings.HasPrefix(a.Name, "on") && len(a.Name) > 2 {
		c.errorf(diag.ParseError, a.Span, "`%s` is not Alder; use `click={Msg}`", a.Name)
		return
	}
	if a.Value == nil {
		if !boolAttrs[a.Name] {
			c.errorf(diag.TypeMismatch, a.Span, "attribute `%s` needs a value", a.Name)
		}
		return
	}
	switch {
	case a.Name == "href":
		if _, isStr := a.Value.(*syntax.StrLit); isStr {
			c.errorf("E0303", a.Span, "`href` takes a page value, not a string; write `href={Module.Page { … }}` or `Url.external \"…\"`")
			return
		}
		mc.check(a.Value, types.Page, env, eff)
	case a.Name == "click" || a.Name == "submit":
		mc.checkMessage(a.Value, m, env, eff)
	case boolAttrs[a.Name]:
		mc.check(a.Value, types.Bool, env, eff)
	default:
		got := c.u.Apply(mc.infer(a.Value, env, eff))
		if c.u.Check(got, types.Text) == nil {
			return
		}
		if c.u.Check(got, &types.Union{Module: "Prelude", Name: "Maybe", Args: []types.Type{types.Text}, Ctors: mc.maybeCtors(), Params: []string{"a"}}) == nil {
			return
		}
		d := mc.mismatch(a.Value.GetSpan(), got, types.Text)
		mc.annotateMismatch(d, a.Value, got, types.Text)
	}
}

// checkMessage checks a click/submit value: a page message (against m) or an app-level message.
func (mc *modCtx) checkMessage(e syntax.Expr, m types.Type, env *env, eff effect) {
	c := mc.c
	var ct *syntax.CtorRef
	switch v := e.(type) {
	case *syntax.CtorRef:
		ct = v
	case *syntax.App:
		if r, ok := v.Fn.(*syntax.CtorRef); ok {
			ct = r
		}
	}
	if ct != nil && ct.Ref.Kind == syntax.RefCtor {
		info, ok := c.ctors[c.key(ct.Ref.Module, ct.Ref.Name)]
		if ok {
			target := m
			if ct.Ref.Module == "App" {
				target = c.appMsg
			}
			if bare, isBare := e.(*syntax.CtorRef); isBare {
				if rec, isRec := info.Arg.(*types.Record); isRec && !messageLike(rec) {
					var parts []string
					for _, f := range rec.Fields {
						if f.Name != "form" {
							parts = append(parts, f.Name+": ?"+f.Name)
						}
					}
					fix := ct.Name + " { " + strings.Join(parts, ", ") + " }"
					d := c.errorf("E0301", bare.Span, "message `%s` needs its parameters: `%s`", ct.Name, fix)
					d.Fix = &diag.Fix{Line: bare.Span.Start.Line, Col: bare.Span.Start.Col, Replace: fix}
					return
				}
				if err := c.u.Unify(info.Union, target); err != nil {
					c.errorf(diag.TypeMismatch, e.GetSpan(), "`%s` is not a message of this page", ct.Name)
				}
				return
			}
			got := mc.infer(e, env, eff)
			if err := c.u.Unify(got, target); err != nil {
				mc.mismatch(e.GetSpan(), got, target)
			}
			return
		}
	}
	mc.check(e, m, env, eff)
}

// component checks <Mod.Name attrs>children</Mod.Name> as a call of Name with a props record.
func (mc *modCtx) component(x *syntax.Element, m types.Type, env *env, eff effect) {
	c := mc.c
	sig, ok := c.values[c.key(x.Ref.Module, x.Ref.Name)]
	if !ok {
		return
	}
	fn, isFn := c.u.Instantiate(sig).(*types.Fun)
	if !isFn || len(fn.Params) != 1 {
		c.errorf(diag.TypeMismatch, x.Span, "component `%s` must take exactly one record argument", x.Tag)
		return
	}
	props, isRec := c.u.Apply(fn.Params[0]).(*types.Record)
	if !isRec {
		c.errorf(diag.TypeMismatch, x.Span, "component `%s` must take a record, got `%s`", x.Tag, types.String(fn.Params[0]))
		return
	}
	lit := &syntax.RecordLit{Node: x.Node}
	for _, a := range x.Attrs {
		val := a.Value
		if val == nil {
			val = &syntax.BoolLit{Node: syntax.Node{Span: a.Span}, Value: true}
		}
		lit.Fields = append(lit.Fields, &syntax.FieldInit{Node: syntax.Node{Span: a.Span}, Name: a.Name, Value: val})
	}
	if len(x.Children) > 0 {
		// children: List (Html m) — check them directly, then mark the field as supplied
		for _, ch := range x.Children {
			mc.htmlNode(ch, m, env, eff)
		}
		lit.Fields = append(lit.Fields, &syntax.FieldInit{Node: x.Node, Name: "children", Value: &syntax.ListLit{Node: x.Node}})
	}
	mc.checkRecordLit(lit, props, env, eff)
	// unify the children field's element type with Html m, if present
	for _, f := range props.Fields {
		if f.Name == "children" {
			c.u.Check(&types.App{Con: "List", Args: []types.Type{&types.App{Con: "Html", Args: []types.Type{m}}}}, f.Type)
		}
	}
	if err := c.u.Check(fn.Result, &types.App{Con: "Html", Args: []types.Type{m}}); err != nil {
		mc.mismatch(x.Span, fn.Result, &types.App{Con: "Html", Args: []types.Type{m}})
	}
}

// formComponent checks <Input>, <Textarea>, <Select>, <Errors>.
func (mc *modCtx) formComponent(x *syntax.Element, env *env, eff effect) {
	c := mc.c
	if mc.form == nil {
		c.errorf("E0304", x.Span, "`<%s>` must be inside `<form submit={…}>`", x.Tag)
		return
	}
	var fieldName string
	var value syntax.Expr
	bareForm := false
	for _, a := range x.Attrs {
		switch a.Name {
		case "field":
			if s, ok := a.Value.(*syntax.StrLit); ok && len(s.Parts) == 1 && s.Parts[0].Expr == nil {
				fieldName = s.Parts[0].Text
			} else {
				c.errorf("E0304", a.Span, "`field` must be a literal field name")
			}
		case "label":
			mc.check(a.Value, types.Text, env, eff)
		case "value":
			value = a.Value
		case "form":
			bareForm = true
		default:
			c.errorf("E0304", a.Span, "`<%s>` has no attribute `%s`", x.Tag, a.Name)
		}
	}
	if x.Tag == "Errors" && bareForm {
		return
	}
	if fieldName == "" {
		c.errorf("E0304", x.Span, "`<%s>` needs `field=\"…\"`", x.Tag)
		return
	}
	var ft types.Type
	var names []string
	for _, f := range mc.form.rec.Fields {
		names = append(names, f.Name)
		if f.Name == fieldName {
			ft = f.Type
		}
	}
	if ft == nil {
		c.errorf("E0304", x.Span, "form `%s` has no field `%s`; fields: %s", mc.form.name, fieldName, strings.Join(names, ", "))
		return
	}
	inner := ft
	if in, ok := maybeOf(ft); ok {
		inner = in
	}
	isUnion := func() bool { u, ok := inner.(*types.Union); return ok && isScalar(u) }()
	switch x.Tag {
	case "Input", "Textarea":
		if isUnion {
			c.errorf("E0304", x.Span, "field `%s` is a union; use `<Select>` for `%s`", fieldName, types.String(inner))
		}
	case "Select":
		if !isUnion {
			c.errorf("E0304", x.Span, "`<Select>` needs a union field; `%s` has type `%s`", fieldName, types.String(ft))
		}
	}
	if value != nil {
		got := c.u.Apply(mc.infer(value, env, eff))
		if c.u.Check(got, ft) != nil && c.u.Check(got, &types.Union{Module: "Prelude", Name: "Maybe", Args: []types.Type{ft}, Ctors: mc.maybeCtors(), Params: []string{"a"}}) != nil {
			mc.mismatch(value.GetSpan(), got, ft)
		}
	}
}
```

Add `form *formCtx` to `modCtx`.

- [ ] **Step 4: Run the golden and iterate**

Run: `go test ./internal/check/ -run TestCheckGolden`
Expected: PASS for `check_decls`, `check_exprs`, `check_html`.

- [ ] **Step 5: Commit**

```bash
git add internal/check testdata/golden && git commit -m "m1: HTML checking: elements, attributes, messages, components, form inputs"
```

---

### Task 11: Checker — page rules, App rules, writes in load

**Files:**
- Replace: `internal/check/pages.go`
- Create: `testdata/golden/check_pages/app/*.alder`, `testdata/golden/check_pages.diag`

**Interfaces:**
- Produces: `func (mc *modCtx) checkPage()` called at the end of `checkBodies`.
- Rules (chapter 6):
  - Page module: `load`'s signature must be exactly `Params -> Session -> Query Model` (E0003 "`load` must have type `Params -> Session -> Query Model`" at the signature); `view` exactly `Model -> Session -> Html Msg`; `update` exactly `Msg -> Model -> Session -> Task Nav`. Comparison is by `u.Unify` against the generated types.
  - `update` must be a `match` over its first parameter with an arm per message — exhaustiveness already enforces this; additionally **E0302** is the code when the subject is `Msg` (override `E0004` → `E0302` with message "`update` does not handle `X`" and fix arm `  X p -> ?todo,`): implement by having `checkMatch` consult `mc.inUpdate` and the subject type's name `Msg`.
  - Writes inside `load`: `effect.inLoad` is set for `load`'s body; in `checkBlock`, when `eff.inLoad` and a bound expression is a `Task`, the E0401 message reads "`load` can only read: `Db.insert` is a write" — i.e. `load`'s signature forces `Query`, so this is already E0401 from Task 9; this task adds the test.
  - `App`: `update` (if present) must be `Msg -> Session -> Task Nav` with App's `Msg`; `error : Error -> Html Never` and `notFound : Html Never` are required (**E0010** if missing); `login` is required when any page has `requires` (E0010 "a page uses `requires`, but `App` has no `login`"); `session` must exist if any page uses `requires` or any module uses `Session.put`.
  - `Html Never` for `view` is **E0003** with message "`view` is `Html Msg` even when the page has no messages" (chapter 6).
  - Warn **W0003** "page `X` is never linked to" is out of scope for M1.

- [ ] **Step 1: Write the golden case**

`testdata/golden/check_pages/app/App.alder`:
```alder
module App

session { user: Maybe Text }

msg Logout

update : Msg -> Session -> Task Nav
update msg session = match msg {
  Logout p -> {
    let _ <- Session.put (\s -> { s | user: Nothing })
    Task.succeed Rerender
  },
}

notFound : Html Never
notFound = <h1>x</h1>
```

`testdata/golden/check_pages/app/T.alder`:
```alder
module T

type TId = brand Text

table T {
  id: TId @id,
  n: Int,
}
```

`testdata/golden/check_pages/app/P.alder`:
```alder
module P

import T

page "/p/{id}" { id: T.TId } requires user

type Model = { t: T }

load : Params -> Session -> Query Model
load params session = {
  let _ <- Db.delete T params.id
  let found <- Db.get T params.id
  match found {
    Just t -> Query.succeed { t: t },
    Nothing -> Query.notFound,
  }
}

msg Inc { id: T.TId }
msg Dec { id: T.TId }

update : Msg -> Model -> Session -> Task Nav
update msg model session = match msg {
  Inc p -> {
    let _ <- Db.update T p.id (\t -> { t | n: t.n + 1 })
    Task.succeed Rerender
  },
}

view : Model -> Session -> Html Never
view model session = <p>{Text.fromInt model.t.n}</p>
```

`testdata/golden/check_pages/app/Q.alder`:
```alder
module Q

page "/q" { }

type Model = { }

load : Params -> Query Model
load params = Query.succeed { }

view : Model -> Session -> Html Msg
view model session = <p>q</p>
```

`testdata/golden/check_pages.diag`:
```
app/App.alder:1:1 E0010 `App` has no `error : Error -> Html Never`
app/App.alder:1:1 E0010 a page uses `requires`, but `App` has no `login`
app/P.alder:11:12 E0401 `Task` cannot run inside a `Query`; `load` and `Query` blocks can only read
app/P.alder:23:36 E0302 `update` does not handle `Dec`
  fix 29:1   Dec p -> ?todo,
app/P.alder:31:8 E0003 `view` must have type `Model -> Session -> Html Msg`; it is `Html Msg` even when the page has no messages
app/Q.alder:7:8 E0003 `load` must have type `Params -> Session -> Query Model`
```

- [ ] **Step 2: Run to verify failure**

Run: `go test ./internal/check/ -run TestCheckGolden/check_pages`
Expected: FAIL.

- [ ] **Step 3: Implement pages.go**

```go
// internal/check/pages.go
package check

import (
	"alder/internal/diag"
	"alder/internal/syntax"
	"alder/internal/types"
)

func (mc *modCtx) checkPage() {
	c := mc.c
	if mc.isApp {
		mc.checkApp()
		return
	}
	if mc.page == nil {
		return
	}
	queryModel := &types.App{Con: "Query", Args: []types.Type{mc.modelType()}}
	mc.requireSig("load", &types.Fun{Params: []types.Type{mc.page.Params, mc.session}, Result: queryModel},
		"`load` must have type `Params -> Session -> Query Model`")
	mc.requireSig("view", &types.Fun{Params: []types.Type{mc.modelType(), mc.session}, Result: &types.App{Con: "Html", Args: []types.Type{mc.page.Msg}}},
		"`view` must have type `Model -> Session -> Html Msg`; it is `Html Msg` even when the page has no messages")
	if len(mc.page.Msgs) > 0 {
		mc.requireSig("update", &types.Fun{Params: []types.Type{mc.page.Msg, mc.modelType(), mc.session}, Result: &types.App{Con: "Task", Args: []types.Type{c.navType()}}},
			"`update` must have type `Msg -> Model -> Session -> Task Nav`")
	}
}

func (mc *modCtx) modelType() types.Type {
	if t, ok := mc.c.types[mc.c.key(mc.m.Name, "Model")]; ok {
		return t
	}
	return mc.c.u.Fresh()
}

func (c *Checker) navType() types.Type {
	if t, ok := c.types["Prelude.Nav"]; ok {
		return t
	}
	return c.u.Fresh()
}

// requireSig checks that value `name`'s declared signature unifies exactly with want.
func (mc *modCtx) requireSig(name string, want types.Type, msg string) {
	c := mc.c
	for _, d := range mc.m.Decls {
		vd, ok := d.(*syntax.ValueDecl)
		if !ok || vd.Name != name || vd.Sig == nil {
			continue
		}
		got := c.values[c.key(mc.m.Name, name)]
		// compare structurally without widening; Html Never vs Html Msg must fail
		if err := c.u.Unify(got, want); err != nil {
			c.errorf(diag.TypeMismatch, vd.Sig.GetSpan(), "%s", msg)
		}
	}
}

func (mc *modCtx) checkApp() {
	c := mc.c
	has := map[string]*syntax.ValueDecl{}
	hasLogin, hasSession := false, false
	for _, d := range mc.m.Decls {
		switch d := d.(type) {
		case *syntax.ValueDecl:
			has[d.Name] = d
		case *syntax.LoginDecl:
			hasLogin = true
		case *syntax.SessionDecl:
			hasSession = true
		}
	}
	if has["error"] == nil {
		c.errorf(diag.MissingDecl, mc.m.Span, "`App` has no `error : Error -> Html Never`")
	} else {
		mc.requireSig("error", &types.Fun{Params: []types.Type{types.Error}, Result: &types.App{Con: "Html", Args: []types.Type{types.Never}}}, "`error` must have type `Error -> Html Never`")
	}
	if has["notFound"] == nil {
		c.errorf(diag.MissingDecl, mc.m.Span, "`App` has no `notFound : Html Never`")
	}
	if len(c.appMsg.Ctors) > 0 {
		if has["update"] == nil {
			c.errorf(diag.MissingDecl, mc.m.Span, "`App` declares messages but has no `update`")
		} else {
			mc.requireSig("update", &types.Fun{Params: []types.Type{c.appMsg, c.session}, Result: &types.App{Con: "Task", Args: []types.Type{c.navType()}}}, "`App.update` must have type `Msg -> Session -> Task Nav`")
		}
	}
	anyRequires := false
	for _, p := range c.pages {
		if p.Requires != "" {
			anyRequires = true
		}
	}
	if anyRequires && !hasLogin {
		c.errorf(diag.MissingDecl, mc.m.Span, "a page uses `requires`, but `App` has no `login`")
	}
	if anyRequires && !hasSession {
		c.errorf(diag.MissingDecl, mc.m.Span, "a page uses `requires`, but `App` has no `session`")
	}
}
```

In `checkBodies` (Task 9), set `mc.inUpdate = (mc.page != nil && d.Name == "update") || (mc.isApp && d.Name == "update")` before checking the body, and in `exhaustiveness.report`, when `ex.mc.inUpdate` and the subject is a `Union` named `Msg`, emit **E0302** with message "`update` does not handle `X`" (one diagnostic listing each missing ctor, fix `  X p -> ?todo,` per ctor) instead of E0004. Add `inUpdate bool` to `modCtx`.

The `App` module's span for E0010 is the module's first line (`1:1`); the golden sorts those two App errors by message order of emission — emit "error" first, then "login".

- [ ] **Step 4: Run all check goldens**

Run: `go test ./internal/check/ -run TestCheckGolden`
Expected: all four PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/check testdata/golden && git commit -m "m1: page and App rules: required signatures, update coverage, writes in load"
```

---

### Task 12: CLI, the error corpus, and the example app

**Files:**
- Modify: `cmd/alder/main.go`
- Create: `cmd/alder/main_test.go`, `testdata/corpus/*/app/*.alder` + `expected.diag` (one directory per corpus case), `experiments/corpus_notes.md`
- Modify: `examples/todos/app/*.alder` and `book/*.md` only if the checker exposes a disagreement (see Global Constraints)

**Interfaces:**
- `alder check [dir] [--json]` → exit 0 (no errors; warnings printed to stderr in text mode), 1 (errors), 2 (usage / no `.alder` files / IO failure). Text diagnostics go to stdout, sorted, errors and warnings; JSON lines go to stdout, errors and warnings, one per line, nothing else on stdout.
- `alder explain <code>` prints the one-paragraph explanation from `internal/diag/explain.go` (a `map[string]string` with every code raised in M1) and exits 0; unknown code → exit 2.

- [ ] **Step 1: Write the CLI tests**

```go
// cmd/alder/main_test.go
package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func run(t *testing.T, args ...string) (int, string, string) {
	var out, errb bytes.Buffer
	code := runCheckTo(args, &out, &errb)
	return code, out.String(), errb.String()
}

func TestCLI_ExampleClean(t *testing.T) {
	code, out, _ := run(t, "../../examples/todos")
	if code != 0 || out != "" {
		t.Fatalf("exit %d, out: %s", code, out)
	}
}

func TestCLI_JSONAndExit(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, "app"), 0o755)
	os.WriteFile(filepath.Join(dir, "app", "X.alder"), []byte("module X\n\nx : Int\nx = \"s\"\n"), 0o644)
	code, out, _ := run(t, dir, "--json")
	if code != 1 {
		t.Fatalf("exit %d", code)
	}
	lines := strings.Split(strings.TrimSpace(out), "\n")
	if len(lines) != 1 || !strings.HasPrefix(lines[0], `{"code":"E0003"`) {
		t.Fatalf("json: %q", out)
	}
}

func TestCLI_NoFiles(t *testing.T) {
	dir := t.TempDir()
	code, out, errs := run(t, dir, "--json")
	if code != 2 || out != "" || !strings.Contains(errs, "no .alder files") {
		t.Fatalf("exit %d out %q err %q", code, out, errs)
	}
}

func TestCLI_TextFormat(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, "app"), 0o755)
	os.WriteFile(filepath.Join(dir, "app", "X.alder"), []byte("module X\n\nx : Text\nx = 1\n"), 0o644)
	code, out, _ := run(t, dir)
	if code != 1 || !strings.Contains(out, "app/X.alder:4:5: error[E0003]: expected Text, got Int") || !strings.Contains(out, "fix: Text.fromInt (1)") {
		t.Fatalf("text: %s", out)
	}
}
```

- [ ] **Step 2: Run to verify failure**

Run: `go test ./cmd/alder/`
Expected: FAIL — undefined runCheckTo.

- [ ] **Step 3: Implement the CLI**

```go
// cmd/alder/main.go
package main

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"alder/internal/check"
	"alder/internal/diag"
	"alder/internal/names"
	"alder/internal/types"
)

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(2)
	}
	switch os.Args[1] {
	case "check":
		os.Exit(runCheckTo(os.Args[2:], os.Stdout, os.Stderr))
	case "explain":
		if len(os.Args) < 3 {
			usage()
			os.Exit(2)
		}
		text, ok := diag.Explain[os.Args[2]]
		if !ok {
			fmt.Fprintf(os.Stderr, "unknown diagnostic code %s\n", os.Args[2])
			os.Exit(2)
		}
		fmt.Println(text)
	default:
		usage()
		os.Exit(2)
	}
}

func usage() { fmt.Fprintln(os.Stderr, "usage: alder check [dir] [--json] | alder explain <code>") }

func runCheckTo(args []string, stdout, stderr io.Writer) int {
	dir, jsonOut := ".", false
	for _, a := range args {
		if a == "--json" {
			jsonOut = true
		} else {
			dir = a
		}
	}
	matches, _ := filepath.Glob(filepath.Join(dir, "app", "*.alder"))
	more, _ := filepath.Glob(filepath.Join(dir, "app", "*", "*.alder"))
	deeper, _ := filepath.Glob(filepath.Join(dir, "app", "*", "*", "*.alder"))
	if len(matches)+len(more)+len(deeper) == 0 {
		fmt.Fprintf(stderr, "alder check: no .alder files under %s/app\n", dir)
		return 2
	}
	std, err := types.LoadStd()
	if err != nil {
		fmt.Fprintf(stderr, "alder: internal error loading stdlib: %v\n", err)
		return 2
	}
	prog, bag := names.Load(dir, std)
	if !bag.HasErrors() {
		prog.Resolve(bag)
	}
	if !bag.HasErrors() {
		check.Program(prog, bag)
	}
	bag.Sort()
	sources := map[string][]string{}
	for _, m := range prog.Modules {
		sources[m.File] = m.Lines
	}
	for _, d := range bag.Items {
		if jsonOut {
			fmt.Fprintln(stdout, diag.RenderJSON(d))
			continue
		}
		w := stdout
		if d.Severity == "warning" {
			w = stderr
		}
		fmt.Fprint(w, diag.RenderText(d, sources[d.Span.File]))
	}
	if bag.HasErrors() {
		return 1
	}
	return 0
}

var _ = strings.TrimSpace
```

Note the pipeline stops after the first phase that produces errors (parse → names → check), so a parse error never cascades into a hundred resolution errors. That is the intended behavior for M1; the spec's "never stop at the first error" is about *within* a phase.

Create `internal/diag/explain.go` with `var Explain = map[string]string{...}` holding one paragraph per code: E0001–E0011, E0101, E0102, E0201–E0203, E0301–E0304, E0310, E0401, E0402, E0501, E0502, E0601, E0602, W0001, W0002. Each paragraph: what the code means, the rule in the book (chapter number), and the usual fix. Write them from the book; no paragraph may be a placeholder.

- [ ] **Step 4: Build the error corpus from the experiments**

For each distinct error scored in `experiments/e1-baseline/scores.md` through `experiments/e4-msgpath/scores.md`, create `testdata/corpus/<code>-<n>/` with a **minimal** multi-file app reproducing it and an `expected.diag`. The cases, with the source to reproduce (write each as a complete tiny app with an `App` module; reuse one `User`/`Todo` pair where possible):

| Dir | Reproduces | Expected code |
|---|---|---|
| `E0101-get-by-slug` | `Db.get Post params.slug` (qwen, E1–E4) | E0101 with the `Db.from … \|> Db.first` fix |
| `E0102-maybe-field` | `let user <- Db.get User id` then `user.name` (qwen E1–E3) | E0102 with the match fix |
| `E0201-missing-import` | `Db.from Todo` in a module without `import Todo` (qwen3:8b E1) | E0201 with `import Todo` fix |
| `E0202-unqualified-type` | `priority: Priority` in Index for `Todo.Priority` (Haiku E1) | E0202 with fix |
| `E0203-qualified-ctor` | `Todo.Normal` (qwen3:8b E1) | E0203 with fix |
| `E0301-bare-msg` | `click={Toggle}` where Toggle has `{ id }` (Haiku E2 refonly) | E0301 with fix |
| `E0302-missing-arm` | `update` without an arm for `Delete` | E0302 with fix |
| `E0303-href-string` | `href="/profile"` | E0303 |
| `E0304-form-field` | `<Input field="nope">` | E0304 |
| `E0401-write-in-load` | `Db.delete` inside `load` | E0401 |
| `E0402-bind-outside` | `let x <- Query.succeed 1` in a plain Int block (qwen3:8b E1) | E0402 |
| `E0501-id-on-insert` | `Db.insert Post { id: p.form.id, … }` (Haiku E2 T3) | E0501 |
| `E0502-ref-type` | `post: Post.Slug @ref Post` (qwen E3) | E0502 |
| `E0601-msg-in-layout` | `Layout : … -> Html Msg` in a non-page module (Sonnet/Haiku E2) | E0601 with fix `a` |
| `E0602-case-of` | `case x of` (qwen E2 refonly) | E0602 |
| `E0602-let-in` | `let y = 1 in y` | E0602 |
| `E0602-tuple` | `(a, b)` | E0602 |
| `E0003-int-interp` | `"n = {List.length xs}"` (several) | E0003 with `Text.fromInt` fix |
| `E0003-plus-text` | `"a" + "b"` (qwen E2) | E0003 with the `++` hint |
| `E0003-date-on-time` | `Date.format "Jan 2" post.created` where `created : Time` (qwen E1–E3) | E0003 "expected Date, got Time" |
| `E0004-missing-case` | `matches` missing `Completed` | E0004 with fix |
| `E0007-no-signature` | `priorityRank p = …` without a signature | E0007 |
| `E0009-dup-ctor` | two imported modules both declaring `Submit`… use `Light` in `Theme` and `Theme2` | E0009 |
| `E0010-no-update` | page with msgs and no `update` (Haiku E2 T4 used `App.Logout` without declaring it: separate case `E0002-undeclared-app-msg` expecting E0002 "unknown `App.Logout`") | E0010 / E0002 |
| `E0011-list-param` | `page "/" { todos: List Todo }` | E0011 |

E0501 is raised in `checkRecordLit` when the expected record is a table's `Insert` record and a field named like the table's `@id`/`@created`/`@updated` is supplied: add to Task 9's `checkRecordLit`: if `rec.Name` starts with `"Insert "`, look up the table and report **E0501** "`Db.insert` never takes `id`; it is generated" with fix removing the field (replace the `FieldInit` span with empty text). Add that now, with the corpus case as its test.

`expected.diag` files use the same format as the goldens (`file:line:col CODE message` + optional `  fix line:col text`). Write each by hand from the book's rules; then run `go test ./internal/check/ -run TestCorpus` and fix the checker until each matches. If a case reveals the book is unclear, fix the book in the same commit.

Record each case's origin (experiment, model, task) in `experiments/corpus_notes.md` so the corpus stays traceable to the data.

- [ ] **Step 5: Make the example check clean**

Run: `go test ./internal/check/ -run TestExamplesCheckClean` and `go run ./cmd/alder check examples/todos`.
Expected: no output, exit 0. Every diagnostic that appears is either a checker bug (fix the checker) or a real inconsistency between `examples/todos` and the book (fix whichever is wrong and the other to match; note it in the commit message). Likely candidates, from reading the example against the rules: `Text.fromInt (Todo.countOpen model.todos)` is fine; `aria-current={if … then Just "page" else Nothing}` relies on `Maybe Text` attributes; `Date.format "Jan 2" d` needs `Date.format : Text -> Date -> Text` in the stdlib; `Time.epoch` must exist; `Layout.Page title={model.todo.title}` passes `Text.NonEmpty` where `Text` is expected (brand widening in a record literal field: `checkRecordLit` uses `check`, which widens — good).

- [ ] **Step 6: Run everything**

Run: `go vet ./... && go test ./...`
Expected: all packages PASS.

- [ ] **Step 7: Update the book's tooling chapter and commit**

In `book/11-tooling.md`, change the opening line to "`alder check` exists as of M1; `build`, `run`, `test`, `fmt`, holes and `query` are planned." and add the diagnostic-code table from the spec (Task 1's general codes plus the spec's) under "## Diagnostics".

```bash
git add -A && git commit -m "m1: alder check CLI, error corpus from E1-E4, examples/todos checks clean"
```

---

## Self-review notes

**Spec coverage.** Lexer/parser (spec "Lexer and parser") → Tasks 2–5; names (module graph, cycles, globals, resolution rules) → Task 6; types and unification with brand widening, `Id`/`Insert` normalization, union ordering → Tasks 7 and 9; checker declaration rules (pages, msgs, forms, tables, capabilities, session) → Task 8; expression rules, effect blocks, exhaustiveness → Task 9; HTML rules → Task 10; page/App rules → Task 11; every diagnostic in the spec's table (E0101, E0102, E0201–E0203, E0301–E0304, E0401, E0402, E0501, E0502, E0601, E0602) is raised by a named task and covered by a corpus case in Task 12; CLI and JSON format → Tasks 1 and 12; `alder explain` → Task 12; acceptance (`examples/todos` clean, corpus passes, JSON validates) → Task 12. Not in M1 by spec: IR, emitter, runtime, holes beyond W0002, `alder query`, `fmt`.

**Type consistency.** `diag.Bag.Error` returns `*Diagnostic` and callers set `.Fix` immediately (Tasks 1, 6, 9–11). `syntax.Ref{Kind, Module, Name}` is set by Task 6 and read by Tasks 8–10 with the kinds defined in Task 3. `types.Unifier.Check/Unify/Apply/Instantiate/RegisterTable/Table` (Task 7) are the only unifier methods used later. `modCtx` fields added across tasks: `holes` (9), `form` (10), `inUpdate` (11) — declare all three in `check.go` in Task 8 to avoid churn. `names.Program.Std` is read by `check.allModules`. `checkHTML`/`checkPage` stubs (Task 9) are replaced, not duplicated, in Tasks 10–11.

**Review Focus coverage.** (1) BOM/CRLF → Task 2 test. (2) unterminated string / unclosed element → Tasks 2 and 5 tests. (3) import cycle with other modules still processed → Task 6 test. (4) duplicate match arm → Task 9 golden (`_ -> "never"` after full coverage) raises E0310; add a second case `Low -> …, Low -> …` to `check_exprs` as `bad12` expecting E0310 "unreachable arm: `Low` is already handled". (5) `--json` with no files → Task 12 test.

**Known simplifications, deliberate:** no let-polymorphism for locals; anonymous records in `infer` mode are structural; `Html Never` handled by `Check` widening inside `Html` only; the pipeline stops after a phase with errors.
