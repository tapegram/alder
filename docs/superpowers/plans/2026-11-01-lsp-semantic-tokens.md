# LSP Semantic Tokens, Folding and Selection (alder#144, milestone 1) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** `alder lsp` answers `textDocument/semanticTokens/full`, `…/range`, `textDocument/foldingRange` and `textDocument/selectionRange` from the document's current text, with a token legend that colours every module the way nvim's `highlights.scm` does, checked by a parity test.

**Architecture:** One parse (`syntax.ParseTokens`) gives the module and the tokens the parser consumed. A new pure package, `internal/semtok`, classifies those tokens: a lexical layer (keywords, operators, literals) and a structural layer (a post-order walk of the AST that says what an identifier is by where it sits, keyed by the token's start position). `internal/lsp/tokens.go` encodes the result in the LSP's five-integer form in the negotiated position unit. `folding.go` and `selection.go` walk the same AST. `tree-sitter-alder/highlights_test.go` compares the producer with `tree-sitter query queries/neovim/highlights.scm` byte by byte over the corpus.

**Tech Stack:** Go (module `alder`), the existing LSP server (`internal/lsp`), the tree-sitter CLI 0.27.1 for the parity test.

**Spec:** `docs/superpowers/specs/2026-11-01-vscode-extension-design.md` (sections "Changes: the server", "Server tests", milestone 1). Read it with this plan; the rulings below override it where they differ.

## Rulings (where this plan departs from the spec, and why)

- **R1. The producer lives in a new package, `internal/semtok`**, not in `internal/lsp/tokens.go`. The parity test in `tree-sitter-alder` must call it, and it should not have to start a language server to do that. `tokens.go` keeps the wire side: the capability, the two requests and the encoder.
- **R2. Interpolated strings differ from nvim on purpose.** The spec says splitting `"a {x.y} b"` colours `x` and `.y` "as in nvim". It does not: `tree-sitter-alder/README.md` says "Interpolations inside strings are not parsed as Alder yet", so nvim colours the whole literal as a string. The server still splits it (the spec's call, and better in VS Code). The parity test skips the bytes *inside* an interpolation's braces. The braces themselves stay `string`.
- **R3. A dotted module name is one `namespace` token per segment** (`Posts`, `Show`; the dot gets nothing), as tree-sitter captures it. This plan reads the spec's "one token, as written" as "as written, segment by segment".
- **R4. Three query edits so nvim matches the spec's legend table.** nvim applies the *last* matching pattern (`highlights.scm` line 2 says so, and `tree-sitter query` prints the pattern index that proves it). With today's order, the `-` of `-1` is `@operator` (pattern 18 beats pattern 6) and both parens of `()` are `@punctuation.bracket`. The spec's table says `-1` is one `number` and `()` is `enumMember`. Task 6 moves `(negative_number "-" @number)` below the operators and `(unit) @constant.builtin` below the brackets. It also adds `(import_decl alias: (constructor) @module)`, because the spec makes import names `namespace` and the alias otherwise falls to `@constructor`. This changes nvim's look in those three spots only.
- **R5. Parity is per byte, under nvim's rule** (the highest pattern index covering a byte wins). It skips whitespace and interpolation interiors (R2) and compares legend types only, not modifiers. It skips `stdlib/`, which parses in signature mode, and scratch files, whose watch `>` is `operator` by the spec's call but `@punctuation.special` (no colour) in nvim.
- **R6. No multi-line token splitting.** The spec says "only a string can span lines". In fact none can: `scanString` stops at a newline (unterminated) and comments end at the line's end. So the producer guarantees single-line tokens, and an invariant test over the whole corpus checks it (Task 5) instead of a splitter.
- **R7. Selection and folding work on scratch files too**, through the same position map the token producer needs (`Map.Shift`, one inserted `module` line). The spec names watches for folding only. Selection on scratch files is about five more lines.

## Global Constraints

- Legend types, in this order, exactly: `namespace type enumMember function method parameter variable property keyword operator string number comment decorator tag attribute boolean hole`.
- Legend modifiers, in this order, exactly: `declaration` (bit 0), `documentation` (bit 1), `defaultLibrary` (bit 2, reserved, never set in this milestone). No custom modifiers.
- Capabilities: `"semanticTokensProvider": {"legend": {...}, "full": true, "range": true}`, `"foldingRangeProvider": true`, `"selectionRangeProvider": true`. No delta tokens.
- The four methods are in `fastMethods` and read `s.text(rel)` (the current text), never an analysis.
- No token for brackets, commas, colons, dots, `_`, markup text, or a tag's `<`, `>`, `/`, `</`.
- Columns and lengths go through `s.enc.toWire` (encoding.go); internally everything is 1-based lines and 1-based byte columns, as the compiler counts.
- A token request for a URI outside the root, an unknown URI, a non-`.alder` file or an empty document answers `{"data": []}`, never `null` and never an error. Folding answers `[]` in those cases.
- Parity test gating is the drift test's: `tsCLI(t)` (fails without the CLI unless `ALDER_SKIP_TS=1`), the pinned CLI is 0.27.1, and a `minParity` floor guards a shrinking corpus.
- alder.nvim is not touched in this PR (milestone 2). Neovim 0.11 uses server tokens as soon as the server offers them, so milestone 2 should follow this one closely. Say so in the PR.
- Every commit message ends with the executing session's attribution lines.

## Review Focus

1. **Half-typed text.** An unterminated string, an unclosed `<div`, a lone `module`, a `match x {` at end of file or an empty document must still get tokens. Expect no panic, sorted non-overlapping single-line tokens, and full colours on the declarations before and after the broken one. Pinned in Task 4 (`TestBroken`) and Task 7 (empty document).
2. **The whole corpus, broken files included.** Every `.alder` under `examples/`, `testdata/` and `stdlib/` (scratch files through `Scratch`) must give tokens that are sorted, non-overlapping, single-line, inside their line and inside the legend. Pinned in Task 5 (`TestInvariants_Corpus`).
3. **Astral characters under UTF-16.** On a line holding `é` and `😀` before and inside tokens, starts and lengths must be in UTF-16 units (a surrogate pair counts 2). Pinned in Task 7 (`TestSemanticTokens_UTF16`).
4. **Scratch watches with odd spacing.** For `>    model` (several spaces) and continuation lines, the generated `itK =` must never leak a token, `>` must be `operator` at column 1, and the expression's tokens must sit where the file has them. Pinned in Task 5 (`TestScratch`).
5. **Positions the AST does not cover.** A selection request at a leading comment, at end of file, or on a line of a non-module file must answer one entry per position (a zero-width range when no node holds it). Folding must answer `[]` for a non-`.alder` file. Pinned in Task 9 (`TestSelectionRange_Edges`) and Task 8 (`TestFoldingRange_Edges`).

---

## File Structure

| File | Responsibility |
|---|---|
| `internal/syntax/parser.go` (modify) | `ParseTokens`; `Tokens` delegates to it |
| `internal/syntax/interp.go` (create) | `Interpolation`, `Interpolations`, `interpParser` (shared with `parseString`) |
| `internal/syntax/parser_expr.go` (modify) | `parseString` uses `interpParser` |
| `internal/syntax/interp_test.go` (create) | tests of the two |
| `internal/semtok/legend.go` (create) | `Kind`, `Types`, `Modifiers`, `Token` |
| `internal/semtok/produce.go` (create) | `Module`, the walker, lexical classes, emission, comments, string pieces |
| `internal/semtok/rules.go` (create) | the structural rules (declarations, types, expressions, patterns, markup) |
| `internal/semtok/scratch.go` (create) | `Scratch`: a scratch file mapped back to its lines |
| `internal/semtok/*_test.go` (create) | `helpers_test.go`, `lexical_test.go`, `decls_test.go`, `exprs_test.go`, `scratch_test.go`, `corpus_test.go` |
| `tree-sitter-alder/highlights_test.go` (create) | parity with `queries/neovim/highlights.scm` |
| `tree-sitter-alder/queries/neovim/highlights.scm` (modify) | R4's three edits |
| `internal/lsp/tokens.go` (create) | provider, `semanticTokensFullReq`, `semanticTokensRangeReq`, `encodeTokens` |
| `internal/lsp/folding.go` (create) | `syntaxDoc`/`parseSyntaxDoc` (module or scratch, position map), `foldingRangeReq` |
| `internal/lsp/selection.go` (create) | `selectionRangeReq` |
| `internal/lsp/protocol.go`, `server.go` (modify) | capabilities, `fastMethods` |
| `internal/lsp/tokens_test.go`, `folding_test.go`, `selection_test.go` (create); `server_test.go` (modify) | LSP tests |
| `book/11-tooling.md`, `book/13-compiler.md` (modify) | what an editor gets; how the server does it |

Work on a branch from `main`: `git switch -c lsp-semantic-tokens origin/main`.

---

### Task 1: `syntax.ParseTokens` and `syntax.Interpolations`

**Files:**
- Modify: `internal/syntax/parser.go:53-58` (`Tokens`)
- Create: `internal/syntax/interp.go`
- Modify: `internal/syntax/parser_expr.go` (`parseString`, the four lines that build `sub`)
- Test: `internal/syntax/interp_test.go`

**Interfaces:**
- Produces: `func ParseTokens(file string, src []byte) (*Module, []Token, []diag.Diagnostic)`. The module is `ParseFile`'s; the tokens are `Tokens`' (NL and EOF included, as today).
- Produces: `type Interpolation struct { From, To int; Tokens []Token }` and `func Interpolations(file string, t Token) []Interpolation`. `t.Text[From:To]` is the expression text between the braces. `Tokens` are what a parse of it consumes (EOF included), positioned in the file exactly as `parseString` positions the expression's nodes. For a non-`String` token the result is nil.

- [ ] **Step 1: Write the failing tests**

```go
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
	_ = Interpolations("t.alder", stringTok(t, toks, `"a {b"`))
}
```

- [ ] **Step 2: Run them to see them fail**

Run: `go test ./internal/syntax -run 'ParseTokens|Interpolations'`
Expected: compile errors, `undefined: ParseTokens`, `undefined: Interpolations`.

- [ ] **Step 3: Implement**

In `internal/syntax/parser.go` replace `Tokens` with:

```go
// ParseTokens parses a source file and returns its module (ParseFile's) and the tokens the
// parser consumed (Tokens'), from one parse.
func ParseTokens(file string, src []byte) (*Module, []Token, []diag.Diagnostic) {
	var trace []Token
	p := newParser(file, src, false, &trace)
	m := p.parseModule()
	return m, trace, p.finish()
}

// Tokens parses a source file and returns the tokens the parser consumed, in source order: the
// lexer is driven by the parser, so HTML text runs appear as Text tokens and speculative
// lookahead is not repeated. String literals are single tokens (interpolations included; see
// Interpolations).
func Tokens(file string, src []byte) ([]Token, []diag.Diagnostic) {
	_, toks, ds := ParseTokens(file, src)
	return toks, ds
}
```

Create `internal/syntax/interp.go`:

```go
package syntax

// Interpolation is one `{…}` of a string literal token: the expression's bytes in the token's
// text (t.Text[From:To], between the braces) and the tokens a parse of it consumes, positioned
// in the file as parseString positions the expression's nodes. Their Offset fields count from
// the expression's start, not the file's.
type Interpolation struct {
	From, To int
	Tokens   []Token
}

// Interpolations are the interpolations of t, a String token, in order, found as parseString
// finds them: escapes skipped, nested braces and strings matched, and an unterminated `{` ends
// the search. A string inside an interpolation is one of its tokens; ask again for its own. A
// token that is not a String has none.
func Interpolations(file string, t Token) []Interpolation {
	if t.Kind != String {
		return nil
	}
	raw := t.Text
	if len(raw) >= 2 && raw[len(raw)-1] == '"' {
		raw = raw[1 : len(raw)-1]
	} else if len(raw) >= 1 {
		raw = raw[1:]
	}
	var out []Interpolation
	for i := 0; i < len(raw); i++ {
		switch {
		case raw[i] == '\\' && i+1 < len(raw):
			i++
		case raw[i] == '{':
			j, ok := scanInterp([]byte(raw), i+1) // j is just past the matching `}`
			if !ok {
				return out
			}
			var trace []Token
			interpParser(file, t, raw[i+1:j-1], i, false, nil, &trace).parseExpr()
			// raw[k] is t.Text[k+1]: the expression raw[i+1:j-1] is t.Text[i+2:j]
			out = append(out, Interpolation{From: i + 2, To: j, Tokens: trace})
			i = j - 1
		}
	}
	return out
}

// interpParser is a parser of inner, the expression of the interpolation whose `{` is raw[i] of
// the string token t (raw being t's text without its opening quote), positioned in the file.
// trace, when non-nil, records the tokens it consumes.
func interpParser(file string, t Token, inner string, i int, sigMode bool, ctx []string, trace *[]Token) *Parser {
	sub := &Parser{lx: NewLexer(file, []byte(inner)), file: file, sigMode: sigMode, ctx: ctx}
	sub.lx.trace = trace
	sub.lx.line, sub.lx.col = t.Line, t.Col+1+i+1
	sub.advance()
	return sub
}
```

In `parseString` (`internal/syntax/parser_expr.go`), replace

```go
			sub := &Parser{lx: NewLexer(p.file, []byte(inner)), file: p.file, sigMode: p.sigMode, ctx: append([]string(nil), p.ctx...)}
			// position the sub-lexer at the interpolation inside the string literal
			sub.lx.line, sub.lx.col = t.Line, t.Col+1+i+1
			sub.advance()
```

with

```go
			sub := interpParser(p.file, t, inner, i, p.sigMode, append([]string(nil), p.ctx...), nil)
```

- [ ] **Step 4: Run the tests**

Run: `go test ./internal/syntax`
Expected: PASS (the whole package: `parseString` changed shape, not behaviour).

- [ ] **Step 5: Commit**

```bash
git add internal/syntax/parser.go internal/syntax/parser_expr.go internal/syntax/interp.go internal/syntax/interp_test.go
git commit -m "syntax: ParseTokens and Interpolations for semantic tokens (#144)"
```

---

### Task 2: `internal/semtok`: legend, lexical layer, emission

**Files:**
- Create: `internal/semtok/legend.go`, `internal/semtok/produce.go`, `internal/semtok/rules.go` (empty rule switch for now)
- Test: `internal/semtok/helpers_test.go`, `internal/semtok/lexical_test.go`

**Interfaces:**
- Consumes: `syntax.ParseTokens`, `syntax.Interpolations`, `syntax.Children` (Task 1 and existing).
- Produces, used by every later task:
  - `type Kind int` with constants `TNamespace, TType, TEnumMember, TFunction, TMethod, TParameter, TVariable, TProperty, TKeyword, TOperator, TString, TNumber, TComment, TDecorator, TTag, TAttribute, TBoolean, THole` (legend order, 0–17); unexported `none Kind = -1`.
  - `var Types []string`, `var Modifiers []string`; `const Declaration = 1, Documentation = 2, DefaultLibrary = 4`.
  - `type Token struct { Line, Col, Len int; Kind Kind; Mods int }`: 1-based line, 1-based byte column, byte length, on one line.
  - `func Module(file string, src []byte) []Token`: sorted, non-overlapping.
  - Inside the package, for Tasks 3–5: `walker` with `toks []syntax.Token`, `cls map[diag.Pos]class`, `in(sp diag.Span) []syntax.Token`, `set(t syntax.Token, k Kind, mods int)`, `setAt(p diag.Pos, k Kind, mods int)`, `rule(n syntax.Spanned)`; `class{kind Kind; mods int; end diag.Pos}`; `func produce(m *syntax.Module, trace []syntax.Token) []Token`; `pos(t syntax.Token) diag.Pos`; `before(a, b diag.Pos) bool`.

- [ ] **Step 1: Write the test helpers and failing tests**

```go
// internal/semtok/helpers_test.go
package semtok

import (
	"strings"
	"testing"
)

// where is the 1-based line and byte column of needle's first occurrence in src, plus off bytes.
func where(t *testing.T, src, needle string, off int) (line, col int) {
	t.Helper()
	i := strings.Index(src, needle)
	if i < 0 {
		t.Fatalf("%q is not in the source", needle)
	}
	i += off
	return strings.Count(src[:i], "\n") + 1, i - strings.LastIndex(src[:i], "\n")
}

// tokAt is the token starting at needle+off, and whether there is one.
func tokAt(t *testing.T, src string, toks []Token, needle string, off int) (Token, bool) {
	t.Helper()
	line, col := where(t, src, needle, off)
	for _, k := range toks {
		if k.Line == line && k.Col == col {
			return k, true
		}
	}
	return Token{}, false
}

// want checks the kind and modifiers of the token starting at needle+off.
func want(t *testing.T, src string, toks []Token, needle string, off int, kind Kind, mods int) {
	t.Helper()
	k, ok := tokAt(t, src, toks, needle, off)
	if !ok {
		t.Errorf("%q+%d: no token", needle, off)
		return
	}
	if k.Kind != kind || k.Mods != mods {
		t.Errorf("%q+%d: %s/%d, want %s/%d", needle, off, Types[k.Kind], k.Mods, Types[kind], mods)
	}
}

// wantLen is want plus the token's length in bytes.
func wantLen(t *testing.T, src string, toks []Token, needle string, off int, kind Kind, n int) {
	t.Helper()
	want(t, src, toks, needle, off, kind, 0)
	if k, ok := tokAt(t, src, toks, needle, off); ok && k.Len != n {
		t.Errorf("%q+%d: length %d, want %d", needle, off, k.Len, n)
	}
}

// wantNone checks that no token starts at needle+off.
func wantNone(t *testing.T, src string, toks []Token, needle string, off int) {
	t.Helper()
	if k, ok := tokAt(t, src, toks, needle, off); ok {
		t.Errorf("%q+%d: token %s, want none", needle, off, Types[k.Kind])
	}
}

// checkInvariants: sorted, non-overlapping, one line each and inside it, kinds and modifiers in the legend.
func checkInvariants(t *testing.T, label, src string, toks []Token) {
	t.Helper()
	lines := strings.Split(strings.ReplaceAll(src, "\r\n", "\n"), "\n")
	for i, k := range toks {
		switch {
		case k.Len <= 0, k.Line < 1, k.Line > len(lines), k.Col < 1, k.Col-1+k.Len > len(lines[k.Line-1]):
			t.Errorf("%s: token %d %+v is not inside its line", label, i, k)
		case k.Kind < 0 || int(k.Kind) >= len(Types) || k.Mods < 0 || k.Mods >= 1<<len(Modifiers):
			t.Errorf("%s: token %d %+v is outside the legend", label, i, k)
		case i > 0 && (toks[i-1].Line > k.Line || toks[i-1].Line == k.Line && toks[i-1].Col+toks[i-1].Len > k.Col):
			t.Errorf("%s: token %d %+v overlaps or precedes %+v", label, i, k, toks[i-1])
		}
	}
}
```

```go
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
```

- [ ] **Step 2: Run them to see them fail**

Run: `go test ./internal/semtok`
Expected: build failure: `undefined: Module`, `undefined: Token`.

- [ ] **Step 3: Implement**

```go
// internal/semtok/legend.go

// Package semtok classifies the tokens of an Alder source for highlighting: the language
// server's semantic tokens (internal/lsp/tokens.go) and the parity test against the tree-sitter
// queries (tree-sitter-alder/highlights_test.go). It reads one parse (syntax.ParseTokens) and
// no analysis, so it can run on every keystroke's text. The legend is the transcription of
// tree-sitter-alder/queries/neovim/highlights.scm (spec 2026-11-01-vscode-extension-design.md).
package semtok

// Kind is a token type: its index in Types, the legend the server sends.
type Kind int

const (
	TNamespace Kind = iota
	TType
	TEnumMember
	TFunction
	TMethod
	TParameter
	TVariable
	TProperty
	TKeyword
	TOperator
	TString
	TNumber
	TComment
	TDecorator
	TTag
	TAttribute
	TBoolean
	THole
)

// none marks a token that gets no colour (a tag's `<`, the digits of a `-1` the `-` already covers).
const none Kind = -1

// Types are the legend's token types, in Kind order. The VS Code extension pins the same list.
var Types = []string{"namespace", "type", "enumMember", "function", "method", "parameter", "variable",
	"property", "keyword", "operator", "string", "number", "comment", "decorator", "tag", "attribute",
	"boolean", "hole"}

// The modifiers, as bits.
const (
	Declaration    = 1 << iota // a name where it is bound
	Documentation              // a comment block that ends on the line above a declaration
	DefaultLibrary             // reserved for phase 2 (standard-library references); never set yet
)

// Modifiers are the legend's token modifiers, in bit order.
var Modifiers = []string{"declaration", "documentation", "defaultLibrary"}

// Token is one coloured span: 1-based line, 1-based byte column and byte length, on one line.
type Token struct {
	Line, Col, Len int
	Kind           Kind
	Mods           int
}
```

```go
// internal/semtok/produce.go
package semtok

import (
	"sort"
	"strings"

	"alder/internal/diag"
	"alder/internal/syntax"
)

// Module classifies src, a module's text. The tokens are sorted and do not overlap.
func Module(file string, src []byte) []Token {
	m, trace, _ := syntax.ParseTokens(file, src)
	return produce(m, trace)
}

// class is what the tree says a token is: its kind (none: no colour), its modifiers, and, when
// set, the end of the span it colours when that is longer than the token (`-1`, `()`).
type class struct {
	kind Kind
	mods int
	end  diag.Pos
}

// walker holds one parse: the tokens to colour, in source order, and the classes the tree gives
// them, keyed by each token's start.
type walker struct {
	m      *syntax.Module
	toks   []syntax.Token
	cls    map[diag.Pos]class
	interp map[diag.Pos]bool // the String tokens of literals with an interpolation
}

func pos(t syntax.Token) diag.Pos  { return diag.Pos{Line: t.Line, Col: t.Col} }
func before(a, b diag.Pos) bool    { return a.Line < b.Line || a.Line == b.Line && a.Col < b.Col }
func (w *walker) set(t syntax.Token, k Kind, mods int) { w.cls[pos(t)] = class{kind: k, mods: mods} }
func (w *walker) setAt(p diag.Pos, k Kind, mods int)   { w.cls[p] = class{kind: k, mods: mods} }

// produce classifies m's tokens: the trace of its parse, plus the tokens of each interpolation
// (which the parser reads with a lexer of its own, so the trace does not hold them).
func produce(m *syntax.Module, trace []syntax.Token) []Token {
	w := &walker{m: m, cls: map[diag.Pos]class{}, interp: map[diag.Pos]bool{}}
	var strs func(n syntax.Spanned)
	strs = func(n syntax.Spanned) {
		if s, ok := n.(*syntax.StrLit); ok {
			for _, p := range s.Parts {
				if p.Expr != nil {
					w.interp[s.Span.Start] = true
					break
				}
			}
		}
		for _, c := range syntax.Children(n) {
			strs(c)
		}
	}
	strs(m)
	add := func(ts []syntax.Token) {
		for _, t := range ts {
			if t.Kind != syntax.NL && t.Kind != syntax.EOF {
				w.toks = append(w.toks, t)
			}
		}
	}
	add(trace)
	for i := 0; i < len(w.toks); i++ { // grows: a string inside an interpolation may have its own
		if t := w.toks[i]; t.Kind == syntax.String && w.interp[pos(t)] {
			for _, in := range syntax.Interpolations(m.File, t) {
				add(in.Tokens)
			}
		}
	}
	sort.SliceStable(w.toks, func(i, j int) bool { return before(pos(w.toks[i]), pos(w.toks[j])) })
	w.walk(m)
	return w.emit()
}

// in is the tokens that start inside sp, in order.
func (w *walker) in(sp diag.Span) []syntax.Token {
	i := sort.Search(len(w.toks), func(i int) bool { return !before(pos(w.toks[i]), sp.Start) })
	j := i
	for j < len(w.toks) && before(pos(w.toks[j]), sp.End) {
		j++
	}
	return w.toks[i:j]
}

// walk applies the rules after the children's, so the rule of the larger construct wins (an
// application's function field is a method, though an access makes its field a property).
func (w *walker) walk(n syntax.Spanned) {
	for _, c := range syntax.Children(n) {
		w.walk(c)
	}
	w.rule(n)
}

var operators = map[syntax.Kind]bool{syntax.Pipe: true, syntax.Arrow: true, syntax.LeftArrow: true,
	syntax.Equal: true, syntax.EqEq: true, syntax.NotEq: true, syntax.Lt: true, syntax.Le: true,
	syntax.Gt: true, syntax.Ge: true, syntax.Plus: true, syntax.Minus: true, syntax.Star: true,
	syntax.Slash: true, syntax.Percent: true, syntax.PlusPlus: true, syntax.AndAnd: true,
	syntax.OrOr: true, syntax.Question2: true, syntax.PipeGt: true, syntax.DotDot: true}

// lexical is a token's class when the tree says nothing of it: by its kind, and for a name,
// the default `(identifier) @variable` and `(constructor) @constructor` give.
func lexical(t syntax.Token) class {
	switch t.Kind {
	case syntax.Keyword, syntax.Backslash:
		return class{kind: TKeyword}
	case syntax.String:
		return class{kind: TString}
	case syntax.Int, syntax.Float:
		return class{kind: TNumber}
	case syntax.Hole:
		return class{kind: THole}
	case syntax.At:
		return class{kind: TDecorator}
	case syntax.LIdent:
		return class{kind: TVariable}
	case syntax.UIdent:
		return class{kind: TEnumMember}
	}
	if operators[t.Kind] {
		return class{kind: TOperator}
	}
	return class{kind: none}
}

// emit turns the classified tokens and the comments into the sorted list; a token overlapping
// the one before it is dropped (none should).
func (w *walker) emit() []Token {
	var out []Token
	for _, t := range w.toks {
		c, ok := w.cls[pos(t)]
		if !ok {
			c = lexical(t)
		}
		switch {
		case c.kind == none:
		case t.Kind == syntax.String && w.interp[pos(t)]:
			out = append(out, pieces(w.m.File, t)...)
		default:
			n := len(t.Text)
			if t.Kind == syntax.Hole {
				n++ // the token's text is the name, without its `?`
			}
			if c.end.Line == t.Line && c.end.Col > t.Col {
				n = c.end.Col - t.Col
			}
			out = append(out, Token{Line: t.Line, Col: t.Col, Len: n, Kind: c.kind, Mods: c.mods})
		}
	}
	out = append(out, w.comments()...)
	sort.SliceStable(out, func(i, j int) bool {
		return out[i].Line < out[j].Line || out[i].Line == out[j].Line && out[i].Col < out[j].Col
	})
	kept := out[:0]
	for _, k := range out {
		if n := len(kept); k.Len <= 0 || n > 0 && kept[n-1].Line == k.Line && kept[n-1].Col+kept[n-1].Len > k.Col {
			continue
		}
		kept = append(kept, k)
	}
	return kept
}

// pieces are the string parts of t, a literal with interpolations: its text less each
// interpolation's expression (the braces stay string). The expressions' tokens are emitted on
// their own.
func pieces(file string, t syntax.Token) []Token {
	var out []Token
	at := 0
	for _, in := range syntax.Interpolations(file, t) {
		if in.From > at {
			out = append(out, Token{Line: t.Line, Col: t.Col + at, Len: in.From - at, Kind: TString})
		}
		at = in.To
	}
	if len(t.Text) > at {
		out = append(out, Token{Line: t.Line, Col: t.Col + at, Len: len(t.Text) - at, Kind: TString})
	}
	return out
}

// comments are the module's comments; one in the block of `--` lines ending on the line above a
// declaration (syntax.DocOf's rule) documents it.
func (w *walker) comments() []Token {
	doc := map[int]bool{}
	for _, d := range w.m.Decls {
		for l := d.GetSpan().Start.Line - 1; l >= 1 && l <= len(w.m.Lines) && strings.HasPrefix(strings.TrimSpace(w.m.Lines[l-1]), "--"); l-- {
			doc[l] = true
		}
	}
	var out []Token
	for _, c := range w.m.Comments {
		k := Token{Line: c.Line, Col: c.Col, Len: len(c.Text), Kind: TComment}
		if doc[c.Line] {
			k.Mods = Documentation
		}
		out = append(out, k)
	}
	return out
}
```

```go
// internal/semtok/rules.go
package semtok

import "alder/internal/syntax"

// rule records what n says of its own tokens (Tasks 3 and 4 fill it in).
func (w *walker) rule(n syntax.Spanned) {
	switch n.(type) {
	}
}
```

The empty `switch n.(type) {}` compiles. Tasks 3 and 4 replace it.

- [ ] **Step 4: Run the tests**

Run: `go test ./internal/semtok`
Expected: PASS. `f` and `x` show as `variable` for now; no test asserts them yet.

- [ ] **Step 5: Commit**

```bash
git add internal/semtok
git commit -m "semtok: legend, lexical classes and emission (#144)"
```

---

### Task 3: structural rules: module, declarations, types

**Files:**
- Modify: `internal/semtok/rules.go`
- Test: `internal/semtok/decls_test.go`

**Interfaces:**
- Consumes: `walker.in`, `set`, `setAt`, `class`, `pos` (Task 2).
- Produces: the helpers `run(ts []syntax.Token) []syntax.Token`, `(w *walker) dotted(name []syntax.Token, last Kind)`, `(w *walker) negative(sp diag.Span)` and the `rule` cases below. Task 4 adds cases to the same switch.

- [ ] **Step 1: Write the failing test**

```go
// internal/semtok/decls_test.go
package semtok

import (
	"testing"

	"alder/internal/syntax"
)

const declSrc = `module M

import Text as T

type Shape a = Circle Int | Square a
type Id = brand Text where Text.isEmpty
type Rec = { name: Text, tags: List Tag.Tag }

table Post @renamed "Article" {
  id: Int @id,
  author: User.UserId @ref User,
  org: Text @partition org,
}

page "/p/{slug}" { slug: Text, q: Text = "" as "Q" } requires user, org, role Admin | Editor, silo org

session { user: User.UserId, org: Maybe Org.OrgId via Membership }

silo org { Project, Task }

choose org = Orgs.Index

login = Auth.Login

deploy { prod: Ssh "host" { domain: "d", floor: -1 }, requires: ["x"] }

msg Save { title: Text } form NewPost

fake Mailer {
  table Sent { to: Text }
  send m = Db.insert Sent m
}

-- docs for f
f : Int -> Int
f n = n
`

func TestDecls(t *testing.T) {
	if _, ds := syntax.ParseFile("t.alder", []byte(declSrc)); len(ds) > 0 {
		t.Fatalf("the fixture does not parse: %v", ds)
	}
	toks := Module("t.alder", []byte(declSrc))
	checkInvariants(t, "declSrc", declSrc, toks)
	s := declSrc
	for _, c := range []struct {
		needle string
		off    int
		kind   Kind
		mods   int
	}{
		{"module M", 7, TNamespace, 0},
		{"import Text as T", 7, TNamespace, 0},
		{"import Text as T", 12, TKeyword, 0},
		{"import Text as T", 15, TNamespace, 0},
		{"type Shape a", 5, TType, Declaration},
		{"type Shape a", 11, TType, 0},
		{"Circle Int", 0, TEnumMember, Declaration},
		{"Circle Int", 7, TType, 0},
		{"| Square a", 2, TEnumMember, Declaration},
		{"| Square a", 9, TType, 0},
		{"brand Text where", 0, TKeyword, 0},
		{"brand Text where", 6, TType, 0},
		{"brand Text where", 11, TKeyword, 0},
		{"name: Text", 0, TProperty, 0},
		{"name: Text", 6, TType, 0},
		{"List Tag.Tag", 0, TType, 0},
		{"List Tag.Tag", 5, TNamespace, 0},
		{"List Tag.Tag", 9, TType, 0},
		{"table Post @renamed", 0, TKeyword, 0},
		{"table Post @renamed", 6, TEnumMember, 0},
		{"table Post @renamed", 11, TDecorator, 0},
		{"table Post @renamed", 12, TDecorator, 0},
		{"id: Int @id", 0, TProperty, 0},
		{"id: Int @id", 9, TDecorator, 0},
		{"@ref User", 5, TType, 0},
		{"@partition org", 11, TProperty, 0},
		{`as "Q"`, 0, TKeyword, 0},
		{"requires user, org", 0, TKeyword, 0},
		{"requires user, org", 9, TProperty, 0},
		{"requires user, org", 15, TProperty, 0},
		{"role Admin | Editor", 0, TKeyword, 0},
		{"role Admin | Editor", 5, TEnumMember, 0},
		{"role Admin | Editor", 11, TOperator, 0},
		{", silo org", 2, TKeyword, 0},
		{", silo org", 7, TProperty, 0},
		{"via Membership", 0, TKeyword, 0},
		{"via Membership", 4, TEnumMember, 0},
		{"silo org {", 0, TKeyword, 0},
		{"silo org {", 5, TProperty, 0},
		{"{ Project, Task }", 2, TType, 0},
		{"{ Project, Task }", 11, TType, 0},
		{"choose org = Orgs.Index", 0, TKeyword, 0},
		{"choose org = Orgs.Index", 7, TProperty, 0},
		{"choose org = Orgs.Index", 13, TNamespace, 0},
		{"choose org = Orgs.Index", 18, TNamespace, 0},
		{"login = Auth.Login", 0, TKeyword, 0},
		{"login = Auth.Login", 8, TNamespace, 0},
		{"login = Auth.Login", 13, TNamespace, 0},
		{"deploy { prod", 0, TKeyword, 0},
		{"deploy { prod", 9, TProperty, 0},
		{`Ssh "host"`, 0, TEnumMember, 0},
		{"domain:", 0, TProperty, 0},
		{"requires: [", 0, TProperty, 0},
		{"msg Save", 0, TKeyword, 0},
		{"msg Save", 4, TEnumMember, 0},
		{"title: Text }", 0, TProperty, 0},
		{"} form NewPost", 2, TKeyword, 0},
		{"} form NewPost", 7, TEnumMember, 0},
		{"fake Mailer", 0, TKeyword, 0},
		{"  table Sent", 2, TKeyword, 0},
		{"send m =", 0, TFunction, Declaration},
		{"send m =", 5, TParameter, Declaration},
		{"-- docs for f", 0, TComment, Documentation},
		{"f : Int", 0, TFunction, Declaration},
		{"f n = n", 0, TFunction, Declaration},
		{"f n = n", 2, TParameter, Declaration},
		{"f n = n", 6, TVariable, 0},
	} {
		want(t, s, toks, c.needle, c.off, c.kind, c.mods)
	}
	wantLen(t, s, toks, "floor: -1", 7, TNumber, 2) // `-1` is one number
	wantNone(t, s, toks, "floor: -1", 8)
	wantLen(t, s, toks, `"/p/{slug}"`, 0, TString, 11)
}
```

If the fixture fails to parse (the first check), fix the fixture with the smallest change that keeps every needle, not the parser. Each construct here is one the parser already accepts in other test files; `grep` the corpus for its shape.

- [ ] **Step 2: Run it to see it fail**

Run: `go test ./internal/semtok -run TestDecls`
Expected: FAIL with many `variable, want function`-style errors.

- [ ] **Step 3: Implement the rules**

Replace `internal/semtok/rules.go` with:

```go
package semtok

import (
	"alder/internal/diag"
	"alder/internal/syntax"
)

// run is the dotted name at the start of ts, `A` or `A.B.C`: its tokens, dots included.
func run(ts []syntax.Token) []syntax.Token {
	n := 0
	for n < len(ts) && ts[n].Kind == syntax.UIdent {
		n++
		if n+1 < len(ts) && ts[n].Kind == syntax.Dot && ts[n+1].Kind == syntax.UIdent {
			n++
			continue
		}
		break
	}
	return ts[:n]
}

// dotted colours a dotted name: each segment but the last is a namespace, the last is last.
func (w *walker) dotted(name []syntax.Token, last Kind) {
	for i, t := range name {
		switch {
		case t.Kind == syntax.Dot:
		case i == len(name)-1:
			w.set(t, last, 0)
		default:
			w.set(t, TNamespace, 0)
		}
	}
}

// negative makes `-1` (a `-` then a number, the literal's span sp) one number token.
func (w *walker) negative(sp diag.Span) {
	if ts := w.in(sp); len(ts) == 2 && ts[0].Kind == syntax.Minus {
		w.cls[pos(ts[0])] = class{kind: TNumber, end: sp.End}
		w.set(ts[1], none, 0)
	}
}

// first is the first token of sp, if any.
func (w *walker) first(sp diag.Span) (syntax.Token, bool) {
	if ts := w.in(sp); len(ts) > 0 {
		return ts[0], true
	}
	return syntax.Token{}, false
}

// keywordAt colours the first token of sp as a keyword (a contextual one the lexer does not reserve).
func (w *walker) keywordAt(sp diag.Span) {
	if t, ok := w.first(sp); ok {
		w.set(t, TKeyword, 0)
	}
}

// rule records what n says of its own tokens: the transcription of highlights.scm's patterns.
func (w *walker) rule(n syntax.Spanned) {
	switch n := n.(type) {
	case *syntax.Module: // `module A.B`
		if ts := w.in(n.Span); len(ts) > 0 && ts[0].Kind == syntax.Keyword && ts[0].Text == "module" {
			w.dotted(run(ts[1:]), TNamespace)
		}
	case *syntax.Import: // `import A.B [as C]`
		ts := w.in(n.Span)
		if len(ts) < 2 {
			break
		}
		name := run(ts[1:])
		w.dotted(name, TNamespace)
		if rest := ts[1+len(name):]; len(rest) > 0 && rest[0].Kind == syntax.LIdent && rest[0].Text == "as" {
			w.set(rest[0], TKeyword, 0)
			if len(rest) > 1 && rest[1].Kind == syntax.UIdent {
				w.set(rest[1], TNamespace, 0)
			}
		}
	case *syntax.ValueDecl: // `f : T` / `f x y = …`, and a fake's entries
		ts := w.in(n.Span)
		if len(ts) == 0 {
			break
		}
		w.set(ts[0], TFunction, Declaration)
		if n.Sig != nil {
			end := n.Sig.GetSpan().End
			for _, t := range ts {
				if !before(pos(t), end) && t.Kind == ts[0].Kind && t.Text == ts[0].Text {
					w.set(t, TFunction, Declaration) // the definition line's name
					break
				}
			}
		}
		for _, p := range n.Params {
			w.setAt(p.Span.Start, TParameter, Declaration)
		}
	case *syntax.TypeDecl: // `type T a b = …`
		ts := w.in(n.Span)
		for i, t := range ts {
			if t.Kind != syntax.UIdent {
				continue
			}
			w.set(t, TType, Declaration)
			for _, p := range ts[i+1:] {
				if p.Kind != syntax.LIdent {
					break
				}
				w.set(p, TType, 0) // a type parameter: (type_var) @type
			}
			break
		}
	case *syntax.Ctor:
		if t, ok := w.first(n.Span); ok {
			w.set(t, TEnumMember, Declaration)
		}
	case *syntax.BrandType: // `brand Base [where …]`
		if ts := w.in(n.Span); len(ts) > 1 && ts[1].Kind == syntax.UIdent {
			w.set(ts[1], TType, 0)
		}
	case *syntax.TypeName:
		w.dotted(run(w.in(n.Span)), TType)
	case *syntax.TypeVar:
		if t, ok := w.first(n.Span); ok {
			w.set(t, TType, 0)
		}
	case *syntax.Field: // `name: T [via U] [= d] [as "K"] [@attr…]`
		if t, ok := w.first(n.Span); ok {
			w.set(t, TProperty, 0)
		}
		if n.Via != "" {
			w.keywordAt(n.ViaSpan)
		}
		if n.Wire != "" {
			w.keywordAt(n.WireSpan)
		}
	case *syntax.Attr: // `@name [arg]`
		ts := w.in(n.Span)
		if len(ts) > 1 {
			w.set(ts[1], TDecorator, 0)
		}
		if len(ts) > 2 {
			switch ts[2].Kind {
			case syntax.UIdent:
				w.set(ts[2], TType, 0)
			case syntax.LIdent:
				w.set(ts[2], TProperty, 0)
			}
		}
	case *syntax.PageDecl: // `requires user, org, role R | S, silo k`
		for i, sp := range n.FieldSpans {
			ts := w.in(sp)
			if i == 0 && len(ts) > 1 { // FieldSpans[0] is RequiresSpan, from `requires`
				w.set(ts[1], TProperty, 0)
			} else if i > 0 && len(ts) > 0 {
				w.set(ts[0], TProperty, 0)
			}
		}
		if n.Roles != nil {
			w.keywordAt(n.RolesSpan)
		}
		if ts := w.in(n.SiloSpan); n.Silo != "" && len(ts) > 1 {
			w.set(ts[0], TKeyword, 0)
			w.set(ts[1], TProperty, 0)
		}
	case *syntax.SiloDecl: // `silo k { T, U }`
		w.keywordAt(n.Span)
		if t, ok := w.first(n.KeySpan); ok {
			w.set(t, TProperty, 0)
		}
		for _, sp := range n.TableSpans {
			if t, ok := w.first(sp); ok {
				w.set(t, TType, 0)
			}
		}
	case *syntax.ChooseDecl: // `choose k = A.B`
		w.keywordAt(n.Span)
		if t, ok := w.first(n.KeySpan); ok {
			w.set(t, TProperty, 0)
		}
		w.pageAfterEqual(n.Span)
	case *syntax.LoginDecl: // `login = A.B`
		w.pageAfterEqual(n.Span)
	case *syntax.MsgDecl: // `msg M [{ … }] [form F]`
		ts := w.in(n.Span)
		from := diag.Pos{}
		if n.Params != nil {
			from = n.Params.Span.End
		}
		for _, t := range ts[min(2, len(ts)):] {
			if t.Kind == syntax.LIdent && t.Text == "form" && !before(pos(t), from) {
				w.set(t, TKeyword, 0)
			}
		}
	case *syntax.TableDecl: // `table` is a keyword token only at the top level; in a fake it is a name
		w.keywordAt(n.Span)
	case *syntax.DeployDecl:
		w.keywordAt(n.Span)
		w.deployFields(n.Fields)
	case *syntax.ThemeDecl:
		w.keywordAt(n.Span)
		w.deployFields(n.Fields)
	}
}

// pageAfterEqual colours the dotted module name after the `=` of sp's tokens as namespaces.
func (w *walker) pageAfterEqual(sp diag.Span) {
	ts := w.in(sp)
	for i, t := range ts {
		if t.Kind == syntax.Equal {
			w.dotted(run(ts[i+1:]), TNamespace)
			return
		}
	}
}

// deployFields colours a deploy or theme record: names are properties, a target's kind a
// constructor, and the values inside.
func (w *walker) deployFields(fs []*syntax.DeployField) {
	for _, f := range fs {
		if t, ok := w.first(f.NameSpan); ok {
			w.set(t, TProperty, 0)
		}
		w.deployValue(f.Value)
	}
}

func (w *walker) deployValue(v *syntax.DeployValue) {
	if v == nil {
		return
	}
	switch v.Kind {
	case "int", "float":
		w.negative(v.Span)
	case "ctor":
		if t, ok := w.first(v.Span); ok {
			w.set(t, TEnumMember, 0)
		}
		w.deployValue(v.Arg)
		w.deployFields(v.Rec)
	case "record":
		w.deployFields(v.Rec)
	case "list":
		for _, e := range v.List {
			w.deployValue(e)
		}
	}
}
```

`min` is the Go 1.21 builtin; check `go.mod`'s `go` line is 1.21 or later (it is if `max(1, width)` in `spanTok` compiles).

- [ ] **Step 4: Run the tests**

Run: `go test ./internal/semtok`
Expected: PASS (TestLexical still passes: none of its tokens changed class except `f`, `x`, which it does not assert).

- [ ] **Step 5: Commit**

```bash
git add internal/semtok
git commit -m "semtok: rules for the module, declarations and types (#144)"
```

---

### Task 4: structural rules: expressions, patterns, markup

**Files:**
- Modify: `internal/semtok/rules.go` (add cases to `rule`, add `own` and `tagName`)
- Test: `internal/semtok/exprs_test.go`

**Interfaces:**
- Consumes: Task 3's `run`, `dotted`, `negative`, `first`, `keywordAt`.
- Produces: nothing new for other tasks (complete `Module` behaviour).

- [ ] **Step 1: Write the failing tests**

```go
// internal/semtok/exprs_test.go
package semtok

import (
	"testing"

	"alder/internal/syntax"
)

const exprSrc = `module M

view : Model -> Html Msg
view model =
  <Layout.Page title={model.post.title}>
    <h1 class="t">{model.post.title}</h1>
    {for c in model.items ->
      <li>{c.name}</li>}
    <Errors form />
    <form submit={Save}>x</form>
  </Layout.Page>

calc : Int -> Int
calc x = {
  let y = Post.bySlug x
  let z <- Db.from Comment |> Db.where (Comment.post.eq x)
  match y {
    Just { title: t } -> -2,
    [a, ..rest] -> 0,
    True -> 1,
    _ -> calc (x - 1),
  }
}

lam = \a b -> a.f b
rec = { r | name: "n", n: () }
neg = -3
str = "a {x.y} b"
ctor = Posts.Show { slug: "s" }
`

func TestExprs(t *testing.T) {
	if _, ds := syntax.ParseFile("t.alder", []byte(exprSrc)); len(ds) > 0 {
		t.Fatalf("the fixture does not parse: %v", ds)
	}
	toks := Module("t.alder", []byte(exprSrc))
	checkInvariants(t, "exprSrc", exprSrc, toks)
	s := exprSrc
	for _, c := range []struct {
		needle string
		off    int
		kind   Kind
		mods   int
	}{
		{"<Layout.Page title", 1, TNamespace, 0},
		{"<Layout.Page title", 8, TTag, 0},
		{"<Layout.Page title", 13, TAttribute, 0},
		{"<Layout.Page title", 18, TOperator, 0},
		{"{model.post.title}>", 1, TVariable, 0},
		{"{model.post.title}>", 7, TProperty, 0},
		{"{model.post.title}>", 12, TProperty, 0},
		{`<h1 class="t">`, 1, TTag, 0},
		{`<h1 class="t">`, 4, TAttribute, 0},
		{`<h1 class="t">`, 10, TString, 0},
		{"</h1>", 2, TTag, 0},
		{"{for c in", 1, TKeyword, 0},
		{"{for c in", 5, TParameter, Declaration},
		{"{for c in", 7, TKeyword, 0},
		{"<Errors form />", 1, TTag, 0},
		{"<Errors form />", 8, TAttribute, 0},
		{"<form submit", 1, TTag, 0},
		{"<form submit", 6, TAttribute, 0},
		{"{Save}", 1, TEnumMember, 0},
		{"</Layout.Page>", 2, TNamespace, 0},
		{"</Layout.Page>", 9, TTag, 0},
		{"let y = Post.bySlug x", 4, TVariable, Declaration},
		{"let y = Post.bySlug x", 8, TNamespace, 0},
		{"let y = Post.bySlug x", 13, TFunction, 0},
		{"let y = Post.bySlug x", 20, TVariable, 0},
		{"Db.from Comment", 0, TNamespace, 0},
		{"Db.from Comment", 3, TFunction, 0},
		{"Db.from Comment", 8, TEnumMember, 0},
		{"|> Db.where", 0, TOperator, 0},
		{"|> Db.where", 6, TFunction, 0},
		{"Comment.post.eq x", 0, TNamespace, 0},
		{"Comment.post.eq x", 8, TFunction, 0},
		{"Comment.post.eq x", 13, TMethod, 0},
		{"Just { title: t }", 0, TEnumMember, 0},
		{"Just { title: t }", 7, TProperty, 0},
		{"Just { title: t }", 14, TVariable, 0},
		{"[a, ..rest]", 1, TVariable, 0},
		{"[a, ..rest]", 4, TOperator, 0},
		{"[a, ..rest]", 6, TVariable, 0},
		{"True -> 1", 0, TBoolean, 0},
		{"_ -> calc", 5, TFunction, 0},
		{`\a b -> a.f b`, 0, TKeyword, 0},
		{`\a b -> a.f b`, 1, TParameter, Declaration},
		{`\a b -> a.f b`, 3, TParameter, Declaration},
		{`\a b -> a.f b`, 10, TMethod, 0},
		{`\a b -> a.f b`, 12, TVariable, 0},
		{"{ r | name", 2, TVariable, 0},
		{"{ r | name", 4, TOperator, 0},
		{"{ r | name", 6, TProperty, 0},
		{"n: ()", 0, TProperty, 0},
		{`"a {x.y} b"`, 4, TVariable, 0},
		{`"a {x.y} b"`, 6, TProperty, 0},
		{"Posts.Show { slug", 0, TNamespace, 0},
		{"Posts.Show { slug", 6, TEnumMember, 0},
		{"Posts.Show { slug", 13, TProperty, 0},
	} {
		want(t, s, toks, c.needle, c.off, c.kind, c.mods)
	}
	for _, c := range []struct {
		needle string
		off    int
	}{
		{"<Layout.Page title", 0}, {"{model.post.title}>", 18}, {`<h1 class="t">`, 0}, {`<h1 class="t">`, 13},
		{"</h1>", 0}, {"</h1>", 1}, {"<Errors form />", 13}, {"_ -> calc", 0}, {"-> -2", 4},
	} {
		wantNone(t, s, toks, c.needle, c.off)
	}
	wantLen(t, s, toks, "-> -2", 3, TNumber, 2)
	wantLen(t, s, toks, "neg = -3", 6, TNumber, 2)
	wantLen(t, s, toks, "n: ()", 3, TEnumMember, 2)
}

// TestBroken: text that does not parse still gets tokens, in order and inside their lines, and
// the declarations around the broken one keep their colours (Review Focus 1).
func TestBroken(t *testing.T) {
	for _, src := range []string{
		"module M\n\nok = 1\n\nbad = \"abc\n\nalso = 2\n",
		"module M\n\nv = <div class=\"x\">\n",
		"module",
		"",
		"module M\n\nf x = match x {\n",
		"module M\n\nf = \"a {b\"\n",
		"module M\n\nf = \"a {\"b {c}\"} d\"\n",
		"module M\n\nf = <p>{\n",
	} {
		checkInvariants(t, src, src, Module("t.alder", []byte(src)))
	}
	src := "module M\n\nok = 1\n\nbad = \"abc\n\nalso = 2\n"
	toks := Module("t.alder", []byte(src))
	want(t, src, toks, "ok = 1", 0, TFunction, Declaration)
	want(t, src, toks, "also = 2", 0, TFunction, Declaration)
}
```

- [ ] **Step 2: Run them to see them fail**

Run: `go test ./internal/semtok -run 'TestExprs|TestBroken'`
Expected: TestExprs FAILS (no tags, no methods); TestBroken may already pass, which is fine.

- [ ] **Step 3: Implement**

Add these cases to the `switch n := n.(type)` in `rule`:

```go
	case *syntax.Var: // `A.B.f`: the module segments, then the value (qualified_name name: @function.call)
		if n.Module != "" {
			ts := w.in(n.Span)
			for i, t := range ts {
				if t.Kind == syntax.UIdent {
					w.set(t, TNamespace, 0)
				} else if t.Kind == syntax.LIdent && i == len(ts)-1 {
					w.set(t, TFunction, 0)
				}
			}
		}
	case *syntax.CtorRef: // `A.B.C`
		if n.Module != "" {
			w.dotted(run(w.in(n.Span)), TEnumMember)
		}
	case *syntax.BoolLit:
		w.setAt(n.Span.Start, TBoolean, 0)
	case *syntax.PLit:
		if _, ok := n.Lit.(*syntax.BoolLit); ok { // the pattern's BoolLit has no span of its own
			w.setAt(n.Span.Start, TBoolean, 0)
		}
	case *syntax.UnitLit: // `()`: one token over both parens
		if ts := w.in(n.Span); len(ts) == 2 && ts[0].Kind == syntax.LParen && n.Span.End.Line == ts[0].Line {
			w.cls[pos(ts[0])] = class{kind: TEnumMember, end: n.Span.End}
		}
	case *syntax.IntLit:
		w.negative(n.Span)
	case *syntax.FloatLit:
		w.negative(n.Span)
	case *syntax.Access: // `e.f`: the field is the span's last token
		if ts := w.in(n.Span); len(ts) > 0 && ts[len(ts)-1].Kind == syntax.LIdent {
			w.set(ts[len(ts)-1], TProperty, 0)
		}
	case *syntax.App: // after its children: the function's name wins over what they made it
		switch f := n.Fn.(type) {
		case *syntax.Var:
			if ts := w.in(f.Span); len(ts) > 0 {
				w.set(ts[len(ts)-1], TFunction, 0)
			}
		case *syntax.Access:
			if ts := w.in(f.Span); len(ts) > 0 {
				w.set(ts[len(ts)-1], TMethod, 0)
			}
		}
	case *syntax.Lambda:
		for _, p := range n.Params {
			w.setAt(p.Span.Start, TParameter, Declaration)
		}
	case *syntax.LetStmt: // `let x [: T] = e` / `let x <- e`
		if ts := w.in(n.Span); len(ts) > 1 && ts[1].Kind == syntax.LIdent {
			w.set(ts[1], TVariable, Declaration)
		}
	case *syntax.FieldInit:
		if t, ok := w.first(n.Span); ok {
			w.set(t, TProperty, 0)
		}
	case *syntax.PField:
		if t, ok := w.first(n.Span); ok {
			w.set(t, TProperty, 0)
		}
	case *syntax.Element:
		w.element(n)
	case *syntax.HtmlAttr:
		if t, ok := w.first(n.Span); ok {
			w.set(t, TAttribute, 0)
		}
	case *syntax.HtmlFor: // `{for x in xs -> …}`: x is bound here
		ts := w.in(n.Span)
		for i, t := range ts {
			if t.Kind == syntax.Keyword && t.Text == "for" && i+1 < len(ts) && ts[i+1].Kind == syntax.LIdent {
				w.set(ts[i+1], TParameter, Declaration)
				break
			}
		}
```

Add after `rule`:

```go
// element colours an element's tags: the tokens of its span outside its attributes and children
// are `<`, `>`, `/` (no colour) and the tag names after `<` and `</`.
func (w *walker) element(el *syntax.Element) {
	var inner []diag.Span
	for _, a := range el.Attrs {
		inner = append(inner, a.Span)
	}
	for _, c := range el.Children {
		inner = append(inner, c.GetSpan())
	}
	ts := w.own(el.Span, inner)
	for i := 0; i < len(ts); i++ {
		switch ts[i].Kind {
		case syntax.Lt, syntax.Gt, syntax.Slash:
			w.set(ts[i], none, 0)
			if ts[i].Kind != syntax.Gt {
				if name := tagName(ts[i+1:]); len(name) > 0 {
					w.dotted(name, TTag) // `Layout.` is a namespace, `Page` the tag
					i += len(name)
				}
			}
		}
	}
}

// tagName is the tag name at the start of ts: a dotted component name, or one lower-case or
// keyword token (`div`, `al-alert`, `form`).
func tagName(ts []syntax.Token) []syntax.Token {
	if name := run(ts); len(name) > 0 {
		return name
	}
	if len(ts) > 0 && (ts[0].Kind == syntax.LIdent || ts[0].Kind == syntax.Keyword) {
		return ts[:1]
	}
	return nil
}

// own is the tokens of sp that are inside none of inner.
func (w *walker) own(sp diag.Span, inner []diag.Span) []syntax.Token {
	var out []syntax.Token
	for _, t := range w.in(sp) {
		inside := false
		for _, s := range inner {
			if !before(pos(t), s.Start) && before(pos(t), s.End) {
				inside = true
				break
			}
		}
		if !inside {
			out = append(out, t)
		}
	}
	return out
}
```

- [ ] **Step 4: Run the tests**

Run: `go test ./internal/semtok`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/semtok
git commit -m "semtok: rules for expressions, patterns and markup (#144)"
```

---

### Task 5: scratch files, and the corpus invariants

**Files:**
- Create: `internal/semtok/scratch.go`
- Test: `internal/semtok/scratch_test.go`, `internal/semtok/corpus_test.go`

**Interfaces:**
- Consumes: `produce` (Task 2), `scratch.ParseFile` (`File.Lines`, `File.Source`, `File.Watches[i].Line/End/Name`).
- Produces: `func Scratch(file, text string) []Token`, used by `internal/lsp/tokens.go` (Task 7).

- [ ] **Step 1: Write the failing tests**

```go
// internal/semtok/scratch_test.go
package semtok

import "testing"

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
```

```go
// internal/semtok/corpus_test.go
package semtok

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"alder/internal/scratch"
	"alder/internal/syntax"
)

// TestInvariants_Corpus: every .alder in the repo, broken ones included, gives sorted,
// non-overlapping tokens inside their lines and the legend (Review Focus 2).
func TestInvariants_Corpus(t *testing.T) {
	n := 0
	for _, root := range []string{"../../examples", "../../testdata", "../../stdlib"} {
		err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
			if err != nil || d.IsDir() || !strings.HasSuffix(path, ".alder") {
				return err
			}
			b, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			n++
			rel := filepath.ToSlash(path)
			if syntax.IsScratchName(rel) && !strings.Contains(rel, "/app/") {
				checkInvariants(t, path, string(b), Scratch(path, string(b)))
				return nil
			}
			checkInvariants(t, path, string(b), Module(path, b))
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	if n < 500 {
		t.Fatalf("only %d sources", n)
	}
	_ = scratch.IsScratchFile // the rule the server uses; IsScratchName plus "not under app/"
}
```

Drop the last line if the linter objects to the unused import trick. Then remove the `scratch` import too. It documents which rule the condition mirrors and adds nothing else.

- [ ] **Step 2: Run them to see them fail**

Run: `go test ./internal/semtok -run 'TestScratch|TestInvariants'`
Expected: build failure, `undefined: Scratch`.

- [ ] **Step 3: Implement**

```go
// internal/semtok/scratch.go
package semtok

import (
	"regexp"
	"sort"
	"strings"

	"alder/internal/scratch"
	"alder/internal/syntax"
)

var segmentRE = regexp.MustCompile(`[A-Z][A-Za-z0-9_]*`)

// Scratch classifies a scratch file's text: its module source (scratch.ParseFile) parsed behind
// a `module Scratch` line, as the run parses it, mapped back to the file's lines and columns
// (each watch's `itK = ` is the file's `> `, and the generated name gets no token), plus the
// `in` header and each watch's `>`.
func Scratch(file, text string) []Token {
	f, _ := scratch.ParseFile(file, text)
	m, trace, _ := syntax.ParseTokens(file, []byte("module Scratch\n"+f.Source))
	gen := map[int]int{} // a watch's first line → the width of its `itK = ` in the source
	for _, w := range f.Watches {
		gen[w.Line] = len(w.Name) + len(" = ")
	}
	var out []Token
	for _, k := range produce(m, trace) {
		if k.Line == 1 { // the `module Scratch` line
			continue
		}
		k.Line--
		if g, ok := gen[k.Line]; ok {
			if k.Col <= g {
				continue // `itK` and its `=`
			}
			k.Col += f.Map.Shift[k.Line-1]
		}
		out = append(out, k)
	}
	for i, l := range f.Lines { // the header: the first line that is not blank or a comment
		s := strings.TrimSpace(l)
		if s == "" || strings.HasPrefix(s, "--") {
			continue
		}
		if l == "in" || strings.HasPrefix(l, "in ") || strings.HasPrefix(l, "in\t") {
			out = append(out, Token{Line: i + 1, Col: 1, Len: 2, Kind: TKeyword})
			for _, seg := range segmentRE.FindAllStringIndex(l[2:], -1) {
				out = append(out, Token{Line: i + 1, Col: 3 + seg[0], Len: seg[1] - seg[0], Kind: TNamespace})
			}
		}
		break
	}
	for _, w := range f.Watches {
		out = append(out, Token{Line: w.Line, Col: 1, Len: 1, Kind: TOperator})
	}
	sort.SliceStable(out, func(i, j int) bool {
		return out[i].Line < out[j].Line || out[i].Line == out[j].Line && out[i].Col < out[j].Col
	})
	return out
}
```

`Map.Shift` on a watch's first line is `1 + len(l[1:]) - len(w.Expr) - len(prefix)`. A source column `c` past the prefix is file column `c + Shift`. That puts `model` of `>    model` (source column 7) at file column 6.

- [ ] **Step 4: Run the tests**

Run: `go test ./internal/semtok`
Expected: PASS. If `TestInvariants_Corpus` reports an overlap or out-of-line token, that is a real producer bug. Fix it in the producer and add the smallest reproducing source to `TestBroken`.

- [ ] **Step 5: Commit**

```bash
git add internal/semtok
git commit -m "semtok: scratch files; invariants over the whole corpus (#144)"
```

---

### Task 6: parity with `highlights.scm`

**Files:**
- Create: `tree-sitter-alder/highlights_test.go`
- Modify: `tree-sitter-alder/queries/neovim/highlights.scm` (R4)
- Modify (as the diffs require): `internal/semtok/rules.go`, `internal/semtok/*_test.go`

**Interfaces:**
- Consumes: `semtok.Module`, `semtok.Types`, `syntax.ParseTokens`, `syntax.Interpolations`; in package `grammar`: `tsCLI`, `libArgs`, `sources`, `writePaths`, `grammarDir` (drift_test.go).
- Produces: `TestHighlightParity`, `TestParityDiffs`; `parityExceptions` (empty unless step 5 adds entries).

- [ ] **Step 1: Write the harness and its own unit test**

```go
// tree-sitter-alder/highlights_test.go
package grammar

import (
	"bufio"
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"

	"alder/internal/semtok"
	"alder/internal/syntax"
)

// minParity guards the parity test against a shrinking corpus, as minSources guards the drift test.
const minParity = 0 // Step 4 sets it

// tsTypes maps each capture of queries/neovim/highlights.scm to the legend type the server gives
// the same bytes (internal/semtok), "" for a capture the server leaves uncoloured. A capture
// missing here fails the test: a new query capture needs a decision.
var tsTypes = map[string]string{
	"variable": "variable", "constructor": "enumMember", "comment": "comment",
	"string": "string", "string.special.url": "string", "number": "number", "number.float": "number",
	"boolean": "boolean", "constant.builtin": "enumMember", "variable.builtin": "hole", "character.special": "",
	"keyword": "keyword", "keyword.import": "keyword", "keyword.type": "keyword",
	"keyword.conditional": "keyword", "keyword.repeat": "keyword", "keyword.function": "keyword",
	"operator": "operator", "punctuation.bracket": "", "punctuation.delimiter": "", "punctuation.special": "",
	"function": "function", "function.call": "function", "function.method.call": "method",
	"variable.parameter": "parameter", "type": "type", "type.definition": "type", "module": "namespace",
	"variable.member": "property", "property": "property", "attribute": "decorator",
	"tag": "tag", "tag.attribute": "attribute", "tag.delimiter": "",
}

// hlCapture is one capture `tree-sitter query` prints: the pattern it belongs to, its name and
// its span (0-based rows, byte columns).
type hlCapture struct {
	pattern        int
	name           string
	sr, sc, er, ec int
}

var (
	patternRE   = regexp.MustCompile(`^\s+pattern: (\d+)$`)
	hlCaptureRE = regexp.MustCompile(`^\s+capture: (?:\d+ - )?([a-z.]+), start: \((\d+), (\d+)\), end: \((\d+), (\d+)\)`)
)

// queryHighlights runs highlights.scm over the files and returns each file's captures.
func queryHighlights(t *testing.T, cli, paths string) map[string][]hlCapture {
	t.Helper()
	args := append([]string{"query"}, libArgs(t, cli)...)
	args = append(args, filepath.Join(grammarDir(t), "queries", "neovim", "highlights.scm"), "--paths", paths)
	cmd := exec.Command(cli, args...)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("tree-sitter query: %v\n%s", err, stderr.String())
	}
	res := map[string][]hlCapture{}
	file, pattern := "", -1
	sc := bufio.NewScanner(bytes.NewReader(out))
	sc.Buffer(make([]byte, 1<<20), 1<<20)
	for sc.Scan() {
		line := sc.Text()
		if line != "" && !strings.HasPrefix(line, " ") {
			file, pattern = line, -1
			continue
		}
		if m := patternRE.FindStringSubmatch(line); m != nil {
			pattern, _ = strconv.Atoi(m[1])
			continue
		}
		m := hlCaptureRE.FindStringSubmatch(line)
		if m == nil {
			continue
		}
		n := func(i int) int { v, _ := strconv.Atoi(m[i]); return v }
		res[file] = append(res[file], hlCapture{pattern, m[1], n(2), n(3), n(4), n(5)})
	}
	return res
}

// parityDiff is a run of bytes on one line that the server and nvim colour differently.
type parityDiff struct {
	label        string
	line, col    int // 1-based
	text         string
	server, nvim string // legend types, "none" for no colour
}

func (d parityDiff) String() string {
	return fmt.Sprintf("%s:%d:%d `%s`: server %s, nvim %s", d.label, d.line, d.col, d.text, d.server, d.nvim)
}

// parityExceptions are differences the test accepts, each with its reason: a construct the
// server and nvim cannot agree on without a grammar change. Every entry is listed in the PR that
// adds it.
var parityExceptions = []struct {
	server, nvim string
	text         *regexp.Regexp
	why          string
}{}

func excepted(d parityDiff) bool {
	for _, e := range parityExceptions {
		if e.server == d.server && e.nvim == d.nvim && e.text.MatchString(d.text) {
			return true
		}
	}
	return false
}

func orNone(s string) string {
	if s == "" {
		return "none"
	}
	return s
}

// parityDiffs compares, byte by byte, the server's tokens with the captures under nvim's rule
// (the highest pattern covering a byte wins). Whitespace is skipped, and so are the bytes
// inside a string's interpolation braces, which tree-sitter does not parse (README).
func parityDiffs(label string, src []byte, toks []semtok.Token, caps []hlCapture) ([]parityDiff, error) {
	lines := strings.Split(string(src), "\n")
	goT := make([][]string, len(lines))
	tsT := make([][]string, len(lines))
	tsP := make([][]int, len(lines))
	mask := make([][]bool, len(lines))
	for i, l := range lines {
		goT[i], tsT[i], tsP[i], mask[i] = make([]string, len(l)), make([]string, len(l)), make([]int, len(l)), make([]bool, len(l))
		for b := range tsP[i] {
			tsP[i][b] = -1
		}
	}
	for _, k := range toks {
		for b := k.Col - 1; k.Line-1 < len(lines) && b < k.Col-1+k.Len && b < len(lines[k.Line-1]); b++ {
			goT[k.Line-1][b] = semtok.Types[k.Kind]
		}
	}
	for _, c := range caps {
		if c.name == "spell" { // a spell-checking region, not a colour
			continue
		}
		typ, ok := tsTypes[c.name]
		if !ok {
			return nil, fmt.Errorf("capture @%s has no entry in tsTypes", c.name)
		}
		for r := c.sr; r <= c.er && r < len(lines); r++ {
			from, to := 0, len(lines[r])
			if r == c.sr {
				from = c.sc
			}
			if r == c.er {
				to = min(c.ec, to)
			}
			for b := from; b < to; b++ {
				if c.pattern > tsP[r][b] {
					tsP[r][b], tsT[r][b] = c.pattern, typ
				}
			}
		}
	}
	_, all, _ := syntax.ParseTokens(label, src)
	for _, tk := range all {
		for _, in := range syntax.Interpolations(label, tk) {
			for b := tk.Col - 1 + in.From; b < tk.Col-1+in.To && b < len(mask[tk.Line-1]); b++ {
				mask[tk.Line-1][b] = true
			}
		}
	}
	var out []parityDiff
	skip := func(r, b int) bool { c := lines[r][b]; return c == ' ' || c == '\t' || c == '\r' || mask[r][b] }
	for r, l := range lines {
		for b := 0; b < len(l); {
			if skip(r, b) || goT[r][b] == tsT[r][b] {
				b++
				continue
			}
			e := b
			for e < len(l) && !skip(r, e) && goT[r][e] == goT[r][b] && tsT[r][e] == tsT[r][b] {
				e++
			}
			out = append(out, parityDiff{label, r + 1, b + 1, l[b:e], orNone(goT[r][b]), orNone(tsT[r][b])})
			b = e
		}
	}
	return out, nil
}

// TestParityDiffs checks the comparison itself, without the CLI: the later pattern wins, spell
// is ignored, whitespace and interpolations are skipped.
func TestParityDiffs(t *testing.T) {
	src := []byte("module M\n\nx = f \"a {y} b\"\n")
	toks := semtok.Module("t.alder", src)
	caps := []hlCapture{
		{11, "keyword.import", 0, 0, 0, 6}, {35, "module", 0, 7, 0, 8}, {1, "constructor", 0, 7, 0, 8},
		{21, "function", 2, 0, 2, 1}, {0, "variable", 2, 0, 2, 1}, {18, "operator", 2, 2, 2, 3},
		{38, "function.call", 2, 4, 2, 5}, {0, "variable", 2, 4, 2, 5}, {3, "string", 2, 6, 2, 15},
		{2, "spell", 2, 6, 2, 15},
	}
	ds, err := parityDiffs("t.alder", src, toks, caps)
	if err != nil || len(ds) != 0 {
		t.Fatalf("diffs %v, err %v", ds, err)
	}
	caps[0].name = "operator" // `module` now differs
	if ds, _ := parityDiffs("t.alder", src, toks, caps); len(ds) != 1 || ds[0].text != "module" || ds[0].nvim != "operator" {
		t.Fatalf("diffs %v", ds)
	}
	if _, err := parityDiffs("t.alder", src, toks, []hlCapture{{1, "made.up", 0, 0, 0, 1}}); err == nil {
		t.Fatal("an unknown capture must fail")
	}
}

// TestHighlightParity: for every module the drift test reads (stdlib, which parses in signature
// mode, and scratch files aside), the server's semantic tokens colour each byte as nvim colours
// it with queries/neovim/highlights.scm. This is the guard that the two editors look the same.
func TestHighlightParity(t *testing.T) {
	cli := tsCLI(t)
	dir := t.TempDir()
	var srcs []source
	for _, s := range sources(t, dir) {
		if !strings.HasPrefix(filepath.ToSlash(s.Label), "../stdlib") {
			srcs = append(srcs, s)
		}
	}
	if len(srcs) < minParity {
		t.Fatalf("only %d sources; expected at least %d", len(srcs), minParity)
	}
	caps := queryHighlights(t, cli, writePaths(t, dir, srcs))
	var bad []string
	files := 0
	for _, s := range srcs {
		src, err := os.ReadFile(s.Path)
		if err != nil {
			t.Fatal(err)
		}
		ds, err := parityDiffs(s.Label, src, semtok.Module(s.Path, src), caps[s.Path])
		if err != nil {
			t.Fatal(err)
		}
		n := len(bad)
		for _, d := range ds {
			if !excepted(d) {
				bad = append(bad, d.String())
			}
		}
		if len(bad) > n {
			files++
		}
	}
	if len(bad) > 0 {
		shown := bad
		if len(shown) > 20 {
			shown = shown[:20]
		}
		t.Fatalf("%d differences in %d of %d sources:\n%s", len(bad), files, len(srcs), strings.Join(shown, "\n"))
	}
	t.Logf("%d sources agree", len(srcs))
}
```

- [ ] **Step 2: Apply R4's query edits**

In `tree-sitter-alder/queries/neovim/highlights.scm`:

1. Delete `(negative_number "-" @number)` from the literals block (line 10) and `(unit) @constant.builtin` (line 12).
2. Directly after the `["(" ")" "[" "]" "{" "}"] @punctuation.bracket` line, add:

```scheme
; after the operators and brackets, so these win on their bytes (a later pattern wins)
(negative_number "-" @number)
(unit) @constant.builtin
```

3. In the "modules and qualified names" block, after `(module_name (constructor) @module)`, add:

```scheme
(import_decl alias: (constructor) @module)
```

Run: `go test ./tree-sitter-alder -run 'TestQueriesCompile|TestParityDiffs'` and `scripts/ts.sh test`.
Expected: PASS. If a `test/highlight/*.alder` assertion covers `-` or `()`, update it to the new capture in the same commit.

- [ ] **Step 3: Run the parity test and record the starting point**

Run: `go test ./tree-sitter-alder -run TestHighlightParity -v 2>&1 | tail -40`
Expected: either PASS with "N sources agree", or FAIL listing differences. Note N (the number of sources) and the number of differences.

- [ ] **Step 4: Set `minParity`**

Set `minParity` to N rounded down to the nearest hundred (with N = 1234, 1200).

- [ ] **Step 5: Drive the differences to zero**

Repeat until the test passes. Take the first difference and find its class (all the differences with the same server/nvim pair and the same construct). Decide in this order:

1. **The producer is wrong** (the query is clear and matches the spec's legend table). Fix the rule in `internal/semtok/rules.go`. Add the smallest construct showing it to `TestDecls` or `TestExprs` as a `want` line, with a source line added to that fixture if needed.
2. **The query contradicts the spec's legend table** and a query edit of at most a few lines makes nvim follow the table. Edit `highlights.scm`, run `scripts/ts.sh test` and `TestQueriesCompile`, and note the edit for the PR description.
3. **Neither** (a shape tree-sitter's grammar gives that the Go parser cannot see, or the reverse). Add a `parityExceptions` entry with the narrowest `text` regexp and a one-line `why`, and note it for the PR.

If class 3 reaches five entries, stop and ask the user before adding more. That many means the grammar and the parser disagree somewhere worth fixing first.

After each class: `go test ./internal/semtok ./tree-sitter-alder` (all of it: the drift tests must stay green).

- [ ] **Step 6: Commit**

Commit after each class is fixed (smaller commits, easier review), or once at the end:

```bash
git add tree-sitter-alder internal/semtok
git commit -m "tree-sitter-alder: highlight parity with the server's semantic tokens (#144)"
```

---

### Task 7: `textDocument/semanticTokens/full` and `/range`

**Files:**
- Create: `internal/lsp/tokens.go`
- Modify: `internal/lsp/protocol.go` (`capabilities`), `internal/lsp/server.go` (`fastMethods`)
- Test: `internal/lsp/tokens_test.go`; modify `internal/lsp/server_test.go` (`TestInitialize_Capabilities`)

**Interfaces:**
- Consumes: `semtok.Module`, `semtok.Scratch`, `semtok.Types`, `semtok.Modifiers`, `semtok.Token`; `s.rel`, `s.text`, `s.enc.toWire`, `scratch.IsScratchFile`.
- Produces: `semanticTokensFullReq`, `semanticTokensRangeReq` (handlers), `encodeTokens(enc posEncoding, lines []string, toks []semtok.Token) []int`, `type SemanticTokens struct{ Data []int }`. In tests: `semanticTokens(t, c, method, rel string, rng *Range) []semTok`, `decodeTokens`, `wantTok`, `wantNoTok`, used by nothing else.

- [ ] **Step 1: Write the failing tests**

Add to `TestInitialize_Capabilities` in `server_test.go`, before the `serverInfo` check:

```go
	stp, _ := caps["semanticTokensProvider"].(map[string]any)
	legend, _ := stp["legend"].(map[string]any)
	if stp["full"] != true || stp["range"] != true ||
		fmt.Sprint(legend["tokenTypes"]) != "[namespace type enumMember function method parameter variable property keyword operator string number comment decorator tag attribute boolean hole]" ||
		fmt.Sprint(legend["tokenModifiers"]) != "[declaration documentation defaultLibrary]" {
		t.Fatalf("semanticTokensProvider %v", caps["semanticTokensProvider"])
	}
```

(add `"fmt"` to the imports if absent).

```go
// internal/lsp/tokens_test.go
package lsp

import (
	"strings"
	"testing"

	"alder/internal/semtok"
)

type semTok struct {
	Line, Char, Len int
	Type            string
	Mods            []string
}

// semanticTokens asks for rel's tokens (method: …/full or …/range) and decodes them.
func semanticTokens(t *testing.T, c *client, method, rel string, rng *Range) []semTok {
	t.Helper()
	params := map[string]any{"textDocument": map[string]any{"uri": c.uri(rel)}}
	if rng != nil {
		params["range"] = rng
	}
	var out struct {
		Data []int `json:"data"`
	}
	if e := c.call(method, params, &out); e != nil {
		t.Fatalf("%s: %v", method, e)
	}
	return decodeTokens(t, out.Data)
}

// decodeTokens turns the five-integer stream back into absolute tokens with names.
func decodeTokens(t *testing.T, data []int) []semTok {
	t.Helper()
	if len(data)%5 != 0 {
		t.Fatalf("%d integers: not a multiple of 5", len(data))
	}
	var out []semTok
	line, char := 0, 0
	for i := 0; i < len(data); i += 5 {
		if data[i] > 0 {
			line, char = line+data[i], data[i+1]
		} else {
			char += data[i+1]
		}
		k := semTok{Line: line, Char: char, Len: data[i+2], Type: semtok.Types[data[i+3]]}
		for b, name := range semtok.Modifiers {
			if data[i+4]&(1<<b) != 0 {
				k.Mods = append(k.Mods, name)
			}
		}
		out = append(out, k)
	}
	return out
}

// wantTok checks the token starting at needle+off in rel's open text (utf-8: characters are bytes).
func wantTok(t *testing.T, c *client, toks []semTok, rel, needle string, off int, typ string, mods ...string) {
	t.Helper()
	p := c.at(rel, needle, off)
	for _, k := range toks {
		if k.Line == p.Line && k.Char == p.Character {
			if k.Type != typ || strings.Join(k.Mods, ",") != strings.Join(mods, ",") {
				t.Errorf("%q+%d: %s %v, want %s %v", needle, off, k.Type, k.Mods, typ, mods)
			}
			return
		}
	}
	t.Errorf("%q+%d: no token at %d:%d", needle, off, p.Line, p.Character)
}

func wantNoTok(t *testing.T, c *client, toks []semTok, rel, needle string, off int) {
	t.Helper()
	p := c.at(rel, needle, off)
	for _, k := range toks {
		if k.Line == p.Line && k.Char == p.Character {
			t.Errorf("%q+%d: token %s, want none", needle, off, k.Type)
		}
	}
}

const showRel = "app/Posts/Show.alder"

func TestSemanticTokens_Show(t *testing.T) {
	c := start(t, blogCopy(t))
	c.open(showRel, c.disk(showRel))
	toks := semanticTokens(t, c, "textDocument/semanticTokens/full", showRel, nil)
	for _, w := range []struct {
		needle string
		off    int
		typ    string
		mods   []string
	}{
		{"module Posts.Show", 0, "keyword", nil},
		{"module Posts.Show", 7, "namespace", nil},
		{"module Posts.Show", 13, "namespace", nil},
		{"import Layout", 7, "namespace", nil},
		{`page "/posts/{slug}"`, 0, "keyword", nil},
		{`page "/posts/{slug}"`, 5, "string", nil},
		{"-- load finds", 0, "comment", []string{"documentation"}},
		{"load : Params", 0, "function", []string{"declaration"}},
		{"load params session", 0, "function", []string{"declaration"}},
		{"load params session", 5, "parameter", []string{"declaration"}},
		{"load params session", 12, "parameter", []string{"declaration"}},
		{"Post.bySlug", 0, "namespace", nil},
		{"Post.bySlug", 5, "function", nil},
		{"let found", 4, "variable", []string{"declaration"}},
		{"Just post ->", 0, "enumMember", nil},
		{"Just post ->", 5, "variable", nil},
		{"|> Db.where", 0, "operator", nil},
		{"type Model", 5, "type", []string{"declaration"}},
		{"<Layout.Page title", 1, "namespace", nil},
		{"<Layout.Page title", 8, "tag", nil},
		{"<Layout.Page title", 13, "attribute", nil},
		{"{model.post.title}>", 1, "variable", nil},
		{"{model.post.title}>", 7, "property", nil},
		{"{model.post.title}>", 12, "property", nil},
		{"</Layout.Page>", 9, "tag", nil},
		{`<al-alert heading="Draft"`, 1, "tag", nil},
		{`<al-alert heading="Draft"`, 10, "attribute", nil},
		{`<al-alert heading="Draft"`, 18, "string", nil},
		{"<h1>", 1, "tag", nil},
		{"</h1>", 2, "tag", nil},
		{"{for c in", 1, "keyword", nil},
		{"{for c in", 5, "parameter", []string{"declaration"}},
	} {
		wantTok(t, c, toks, showRel, w.needle, w.off, w.typ, w.mods...)
	}
	wantNoTok(t, c, toks, showRel, "<Layout.Page title", 0)
	for _, k := range toks { // the page path is one string: `{slug}` is not an interpolation there
		if p := c.at(showRel, `"/posts/{slug}"`, 0); k.Line == p.Line && k.Char == p.Character && k.Len != len(`"/posts/{slug}"`) {
			t.Errorf("page path length %d", k.Len)
		}
	}
}

const tokensRel = "app/Tokens.alder"

const tokensSrc = `module Tokens

answer : Int
answer = -1

flag : Bool
flag = True

pick : Maybe Int -> Int
pick m = match m {
  -- inside a body
  Just n -> n,
  Nothing -> ?h,
}

broken : Int
broken = )

after : Unit
after = {
  let u = ()
  u
}

greet : Text -> Text
greet x = "héllo 😀 {x}" ++ x
`

func TestSemanticTokens_Fixture(t *testing.T) {
	c := start(t, blogCopy(t))
	c.open(tokensRel, tokensSrc)
	toks := semanticTokens(t, c, "textDocument/semanticTokens/full", tokensRel, nil)
	wantTok(t, c, toks, tokensRel, "= -1", 2, "number")
	wantTok(t, c, toks, tokensRel, "= True", 2, "boolean")
	wantTok(t, c, toks, tokensRel, "-- inside a body", 0, "comment")
	wantTok(t, c, toks, tokensRel, "?h", 0, "hole")
	wantTok(t, c, toks, tokensRel, "= ()", 2, "enumMember")
	wantTok(t, c, toks, tokensRel, "pick m = match", 0, "function", "declaration")
	wantTok(t, c, toks, tokensRel, "broken = )", 0, "function", "declaration")
	wantTok(t, c, toks, tokensRel, "after = {", 0, "function", "declaration") // after the broken one
	wantTok(t, c, toks, tokensRel, "let u", 4, "variable", "declaration")
	wantTok(t, c, toks, tokensRel, "{x}", 1, "variable")
	for _, k := range toks {
		p, q, r := c.at(tokensRel, "= -1", 2), c.at(tokensRel, "?h", 0), c.at(tokensRel, "= ()", 2)
		switch {
		case k.Line == p.Line && k.Char == p.Character && k.Len != 2,
			k.Line == q.Line && k.Char == q.Character && k.Len != 2,
			k.Line == r.Line && k.Char == r.Character && k.Len != 2:
			t.Errorf("token %+v: length %d, want 2", k, k.Len)
		}
	}
}

// TestSemanticTokens_UTF16: with a client that offers only utf-16, columns and lengths after
// and over é (one unit, two bytes) and 😀 (two units, four bytes) are in UTF-16 (Review Focus 3).
func TestSemanticTokens_UTF16(t *testing.T) {
	c := startOffering(t, blogCopy(t), Options{}, []string{"utf-16"})
	c.open(tokensRel, tokensSrc)
	toks := semanticTokens(t, c, "textDocument/semanticTokens/full", tokensRel, nil)
	line := strings.Split(tokensSrc, "\n")
	ln := 0
	for !strings.HasPrefix(line[ln], "greet x =") {
		ln++
	}
	byteX := strings.LastIndex(line[ln], "x")
	var str, last *semTok
	for i := range toks {
		if toks[i].Line == ln && toks[i].Type == "string" && str == nil {
			str = &toks[i]
		}
		if toks[i].Line == ln {
			last = &toks[i]
		}
	}
	if str == nil || last == nil {
		t.Fatalf("no tokens on line %d: %+v", ln, toks)
	}
	// `"héllo 😀 {` is 14 bytes, 11 UTF-16 units; the final x is 3 units left of its byte column
	if str.Len != 11 || last.Char != byteX-3 || last.Type != "variable" {
		t.Fatalf("string %+v, last %+v (x at byte %d)", *str, *last, byteX)
	}
}

func TestSemanticTokens_Range(t *testing.T) {
	c := start(t, blogCopy(t))
	c.open(showRel, c.disk(showRel))
	full := semanticTokens(t, c, "textDocument/semanticTokens/full", showRel, nil)
	got := semanticTokens(t, c, "textDocument/semanticTokens/range", showRel, &Range{Start: Position{18, 0}, End: Position{23, 0}})
	var want []semTok
	for _, k := range full {
		if k.Line >= 18 && k.Line <= 23 {
			want = append(want, k)
		}
	}
	if len(got) == 0 || len(got) != len(want) {
		t.Fatalf("range gave %d tokens, want %d", len(got), len(want))
	}
	for i := range got {
		if got[i].Line != want[i].Line || got[i].Char != want[i].Char || got[i].Len != want[i].Len || got[i].Type != want[i].Type {
			t.Fatalf("token %d: %+v, want %+v", i, got[i], want[i])
		}
	}
}

func TestSemanticTokens_Scratch(t *testing.T) {
	c := start(t, blogCopy(t))
	const rel = "scratch.alder"
	c.open(rel, "in Posts.Show\n\n> 1 + 1\n>    model\n")
	toks := semanticTokens(t, c, "textDocument/semanticTokens/full", rel, nil)
	wantTok(t, c, toks, rel, "in Posts.Show", 0, "keyword")
	wantTok(t, c, toks, rel, "in Posts.Show", 3, "namespace")
	wantTok(t, c, toks, rel, "> 1 + 1", 0, "operator")
	wantTok(t, c, toks, rel, ">    model", 5, "variable")
}

// TestSemanticTokens_Empty: an unknown URI, a file outside the root, a non-.alder file and an
// empty document answer `{"data": []}`, never null (Review Focus 5).
func TestSemanticTokens_Empty(t *testing.T) {
	c := start(t, blogCopy(t))
	c.open("app/Empty.alder", "")
	for _, uri := range []string{"file:///nowhere/x.alder", "untitled:Untitled-1", c.uri("README.md"), c.uri("app/Empty.alder")} {
		var out map[string]any
		if e := c.call("textDocument/semanticTokens/full", map[string]any{"textDocument": map[string]any{"uri": uri}}, &out); e != nil {
			t.Fatalf("%s: %v", uri, e)
		}
		if d, ok := out["data"].([]any); !ok || len(d) != 0 {
			t.Fatalf("%s: data %#v, want []", uri, out["data"])
		}
	}
}
```

`README.md` is the blog's own (`examples/blog/README.md`): a real file under the root that is not Alder.

- [ ] **Step 2: Run them to see them fail**

Run: `go test ./internal/lsp -run 'SemanticTokens|TestInitialize_Capabilities'`
Expected: FAIL: `unknown method textDocument/semanticTokens/full` (codeMethodNotFound) and the capabilities check.

- [ ] **Step 3: Implement**

```go
// internal/lsp/tokens.go
package lsp

import (
	"encoding/json"
	"strings"

	"alder/internal/scratch"
	"alder/internal/semtok"
)

// semanticTokensProvider is the capability: the legend (internal/semtok, which the VS Code
// extension pins too), full and range answers, no deltas (a module's token pass costs about
// what a hover costs; spec 2026-11-01).
func semanticTokensProvider() map[string]any {
	return map[string]any{
		"legend": map[string]any{"tokenTypes": semtok.Types, "tokenModifiers": semtok.Modifiers},
		"full":   true,
		"range":  true,
	}
}

type semanticTokensParams struct {
	TextDocument struct {
		URI string `json:"uri"`
	} `json:"textDocument"`
	Range *Range `json:"range"`
}

// SemanticTokens answers both requests: five integers per token (LSP 3.17), never null.
type SemanticTokens struct {
	Data []int `json:"data"`
}

// semanticTokensFullReq answers textDocument/semanticTokens/full from the document's current
// text: the tokens must match what is on screen, so no analysis is read.
func semanticTokensFullReq(s *server, params json.RawMessage) (any, error) {
	return s.semanticTokens(params, false)
}

// semanticTokensRangeReq answers textDocument/semanticTokens/range: the full answer cut to the
// range's lines, encoded from the first kept token.
func semanticTokensRangeReq(s *server, params json.RawMessage) (any, error) {
	return s.semanticTokens(params, true)
}

func (s *server) semanticTokens(params json.RawMessage, ranged bool) (any, error) {
	var p semanticTokensParams
	if err := json.Unmarshal(params, &p); err != nil {
		return nil, &rpcError{codeInvalidParams, err.Error()}
	}
	out := SemanticTokens{Data: []int{}}
	rel, ok := s.rel(p.TextDocument.URI)
	if !ok || !strings.HasSuffix(rel, ".alder") {
		return out, nil
	}
	text := s.text(rel)
	var toks []semtok.Token
	if scratch.IsScratchFile(rel) {
		toks = semtok.Scratch(rel, text)
	} else {
		toks = semtok.Module(rel, []byte(text))
	}
	if ranged && p.Range != nil {
		lo, hi := p.Range.Start.Line+1, p.Range.End.Line+1
		kept := toks[:0]
		for _, t := range toks {
			if t.Line >= lo && t.Line <= hi {
				kept = append(kept, t)
			}
		}
		toks = kept
	}
	out.Data = encodeTokens(s.enc, strings.Split(text, "\n"), toks)
	return out, nil
}

// encodeTokens is toks (sorted, one line each, byte columns) as the LSP's relative integers: line
// delta, start delta (from the previous token's start when on the same line), length, type,
// modifiers, with starts and lengths in enc's unit.
func encodeTokens(enc posEncoding, lines []string, toks []semtok.Token) []int {
	data := make([]int, 0, 5*len(toks))
	prevLine, prevChar := 0, 0
	for _, t := range toks {
		line := ""
		if t.Line-1 < len(lines) {
			line = lines[t.Line-1]
		}
		start := enc.toWire(line, t.Col-1)
		length := enc.toWire(line, t.Col-1+t.Len) - start
		dl, ds := t.Line-1-prevLine, start
		if dl == 0 {
			ds = start - prevChar
		}
		data = append(data, dl, ds, length, int(t.Kind), t.Mods)
		prevLine, prevChar = t.Line-1, start
	}
	return data
}
```

In `capabilities` (protocol.go) add:

```go
		"semanticTokensProvider":     semanticTokensProvider(),
```

In `fastMethods` (server.go) add:

```go
		"textDocument/semanticTokens/full":  semanticTokensFullReq,
		"textDocument/semanticTokens/range": semanticTokensRangeReq,
```

- [ ] **Step 4: Run the tests**

Run: `go test ./internal/lsp -run 'SemanticTokens|TestInitialize|TestEncoding'`
Expected: PASS. Then `go test ./internal/lsp` (the whole package; the new capability must not break other tests).

- [ ] **Step 5: Commit**

```bash
git add internal/lsp/tokens.go internal/lsp/tokens_test.go internal/lsp/protocol.go internal/lsp/server.go internal/lsp/server_test.go
git commit -m "lsp: semantic tokens, full and range, from the current text (#144)"
```

---

### Task 8: `textDocument/foldingRange`

**Files:**
- Create: `internal/lsp/folding.go`
- Modify: `internal/lsp/protocol.go`, `internal/lsp/server.go`, `internal/lsp/server_test.go`
- Test: `internal/lsp/folding_test.go`

**Interfaces:**
- Consumes: `syntax.ParseFile`, `syntax.Children`, `scratch.ParseFile`, `s.rel`, `s.text`.
- Produces, for Task 9: `type syntaxDoc struct { m *syntax.Module; lines []string; watches []scratch.Watch; shift []int }`, `func parseSyntaxDoc(rel, text string) syntaxDoc`, `(d syntaxDoc) toFile(p diag.Pos) diag.Pos`, `(d syntaxDoc) toModule(p diag.Pos) diag.Pos`, `type textDocumentParams struct{ TextDocument struct{ URI string } }`. Also `type FoldingRange`, `foldingRangeReq`.

- [ ] **Step 1: Write the failing tests**

Add to `TestInitialize_Capabilities`:

```go
	if caps["foldingRangeProvider"] != true {
		t.Fatalf("foldingRangeProvider %v", caps["foldingRangeProvider"])
	}
```

```go
// internal/lsp/folding_test.go
package lsp

import "testing"

func foldingRanges(t *testing.T, c *client, uri string) []FoldingRange {
	t.Helper()
	var out []FoldingRange
	if e := c.call("textDocument/foldingRange", map[string]any{"textDocument": map[string]any{"uri": uri}}, &out); e != nil {
		t.Fatalf("foldingRange: %v", e)
	}
	return out
}

func TestFoldingRange_Show(t *testing.T) {
	c := start(t, blogCopy(t))
	c.open(showRel, c.disk(showRel))
	got := foldingRanges(t, c, c.uri(showRel))
	has := func(start, end int, kind string) bool {
		for _, r := range got {
			if r.StartLine == start && r.EndLine == end && r.Kind == kind {
				return true
			}
		}
		return false
	}
	for _, w := range []struct {
		start, end int
		kind, what string
	}{
		{2, 6, "imports", "the imports"},
		{10, 14, "", "type Model"},
		{16, 17, "comment", "the comment above load"},
		{18, 39, "", "load"},
		{21, 38, "", "the match in load"},
		{43, 44, "comment", "the comment above msg"},
		{64, 71, "", "<article>"},
	} {
		if !has(w.start, w.end, w.kind) {
			t.Errorf("no %q range %d–%d (%s)", w.kind, w.start, w.end, w.what)
		}
	}
	for i, r := range got {
		if r.StartLine == 41 {
			t.Errorf("the one-line form declaration folds: %+v", r)
		}
		if r.EndLine <= r.StartLine {
			t.Errorf("single-line range %+v", r)
		}
		if i > 0 && got[i-1].StartLine > r.StartLine {
			t.Errorf("not in source order at %d: %+v after %+v", i, r, got[i-1])
		}
	}
}

func TestFoldingRange_Scratch(t *testing.T) {
	c := start(t, blogCopy(t))
	c.open("scratch.alder", "> 1\n  + 2\n\nsq x =\n  x * x\n")
	got := foldingRanges(t, c, c.uri("scratch.alder"))
	want := map[[2]int]bool{{0, 1}: false, {3, 4}: false}
	for _, r := range got {
		if _, ok := want[[2]int{r.StartLine, r.EndLine}]; ok {
			want[[2]int{r.StartLine, r.EndLine}] = true
		}
	}
	for k, ok := range want {
		if !ok {
			t.Errorf("no range %v in %+v", k, got)
		}
	}
}

// TestFoldingRange_Edges: unknown and non-Alder documents answer [] (Review Focus 5).
func TestFoldingRange_Edges(t *testing.T) {
	c := start(t, blogCopy(t))
	for _, uri := range []string{"file:///nowhere/x.alder", c.uri("README.md")} {
		var out []FoldingRange
		if e := c.call("textDocument/foldingRange", map[string]any{"textDocument": map[string]any{"uri": uri}}, &out); e != nil || out == nil || len(out) != 0 {
			t.Fatalf("%s: %v, %v", uri, out, e)
		}
	}
}
```

Before running, check the line numbers against the file: `awk 'NR==3||NR==7||NR==11||NR==15||NR==17||NR==19||NR==40||NR==22||NR==39||NR==42||NR==44||NR==45||NR==65||NR==72' examples/blog/app/Posts/Show.alder`. Each expected range is (first line − 1, last line − 1). If the blog changed since this plan was written, adjust the numbers to the same constructs.

- [ ] **Step 2: Run them to see them fail**

Run: `go test ./internal/lsp -run 'FoldingRange|TestInitialize_Capabilities'`
Expected: build failure, `undefined: FoldingRange`.

- [ ] **Step 3: Implement**

```go
// internal/lsp/folding.go
package lsp

import (
	"encoding/json"
	"sort"
	"strings"

	"alder/internal/diag"
	"alder/internal/scratch"
	"alder/internal/syntax"
)

// textDocumentParams are the params of a request about a whole document.
type textDocumentParams struct {
	TextDocument struct {
		URI string `json:"uri"`
	} `json:"textDocument"`
}

// syntaxDoc is a document's current text parsed for the syntax requests (folding, selection): an app
// module as is; a scratch file as its module source behind a `module Scratch` line, as the run
// parses it, with the map back to the file's positions (each watch's `itK = ` is the file's `> `).
type syntaxDoc struct {
	m       *syntax.Module
	lines   []string        // the document's lines
	watches []scratch.Watch // a scratch file's
	shift   []int           // a scratch file's Map.Shift; nil for a module
}

func parseSyntaxDoc(rel, text string) syntaxDoc {
	lines := strings.Split(text, "\n")
	if !scratch.IsScratchFile(rel) {
		m, _ := syntax.ParseFile(rel, []byte(text))
		return syntaxDoc{m: m, lines: lines}
	}
	f, _ := scratch.ParseFile(rel, text)
	m, _ := syntax.ParseFile(rel, []byte("module Scratch\n"+f.Source))
	return syntaxDoc{m: m, lines: lines, watches: f.Watches, shift: f.Map.Shift}
}

// toFile is p, a position in d.m, in the document (both 1-based); toModule is its inverse.
func (d syntaxDoc) toFile(p diag.Pos) diag.Pos {
	if d.shift == nil {
		return p
	}
	line, col := p.Line-1, p.Col
	if line >= 1 && line <= len(d.shift) {
		col += d.shift[line-1]
	}
	return diag.Pos{Line: max(line, 1), Col: max(col, 1)}
}

func (d syntaxDoc) toModule(p diag.Pos) diag.Pos {
	if d.shift == nil {
		return p
	}
	col := p.Col
	if p.Line >= 1 && p.Line <= len(d.shift) {
		col -= d.shift[p.Line-1]
	}
	return diag.Pos{Line: p.Line + 1, Col: col}
}

// FoldingRange is an LSP folding range: 0-based first and last lines, and a kind for comment and
// import runs.
type FoldingRange struct {
	StartLine int    `json:"startLine"`
	EndLine   int    `json:"endLine"`
	Kind      string `json:"kind,omitempty"`
}

// foldingRangeReq answers textDocument/foldingRange from the current text: one range per
// multi-line node of the kinds folds.scm lists, each scratch watch, each run of two or more
// `--` comment lines (kind comment) and the run of imports (kind imports), outermost first in
// source order.
func foldingRangeReq(s *server, params json.RawMessage) (any, error) {
	var p textDocumentParams
	if err := json.Unmarshal(params, &p); err != nil {
		return nil, &rpcError{codeInvalidParams, err.Error()}
	}
	out := []FoldingRange{}
	rel, ok := s.rel(p.TextDocument.URI)
	if !ok || !strings.HasSuffix(rel, ".alder") {
		return out, nil
	}
	d := parseSyntaxDoc(rel, s.text(rel))
	seen := map[[2]int]bool{}
	add := func(start, end int, kind string) { // 1-based document lines
		if end > start && !seen[[2]int{start, end}] {
			seen[[2]int{start, end}] = true
			out = append(out, FoldingRange{StartLine: start - 1, EndLine: end - 1, Kind: kind})
		}
	}
	span := func(sp diag.Span) {
		end := d.toFile(sp.End).Line
		if sp.End.Col == 1 { // nothing of the node is on its end line
			end--
		}
		add(d.toFile(sp.Start).Line, end, "")
	}
	var walk func(n syntax.Spanned)
	walk = func(n syntax.Spanned) {
		switch n.(type) {
		case *syntax.ValueDecl, *syntax.TypeDecl, *syntax.TableDecl, *syntax.CapDecl, *syntax.FakeDecl,
			*syntax.SessionDecl, *syntax.DeployDecl, *syntax.ThemeDecl, *syntax.TestDecl, *syntax.FormDecl,
			*syntax.Block, *syntax.Match, *syntax.Element, *syntax.RecordLit, *syntax.ListLit:
			span(n.GetSpan())
		}
		for _, c := range syntax.Children(n) {
			walk(c)
		}
	}
	walk(d.m)
	for _, w := range d.watches {
		add(w.Line, w.End, "")
	}
	if n := len(d.m.Imports); n > 0 {
		add(d.toFile(d.m.Imports[0].Span.Start).Line, d.toFile(d.m.Imports[n-1].Span.End).Line, "imports")
	}
	runStart, runEnd := 0, 0
	for _, c := range d.m.Comments {
		l := d.toFile(diag.Pos{Line: c.Line, Col: c.Col}).Line
		if l < 1 || l > len(d.lines) || !strings.HasPrefix(strings.TrimSpace(d.lines[l-1]), "--") {
			continue // a comment after code on its line
		}
		if runEnd > 0 && l == runEnd+1 {
			runEnd = l
			continue
		}
		add(runStart, runEnd, "comment")
		runStart, runEnd = l, l
	}
	add(runStart, runEnd, "comment")
	sort.SliceStable(out, func(i, j int) bool {
		return out[i].StartLine < out[j].StartLine || out[i].StartLine == out[j].StartLine && out[i].EndLine > out[j].EndLine
	})
	return out, nil
}
```

Capabilities: `"foldingRangeProvider": true,`. fastMethods: `"textDocument/foldingRange": foldingRangeReq,`.

- [ ] **Step 4: Run the tests**

Run: `go test ./internal/lsp -run 'FoldingRange|TestInitialize'`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/lsp/folding.go internal/lsp/folding_test.go internal/lsp/protocol.go internal/lsp/server.go internal/lsp/server_test.go
git commit -m "lsp: folding ranges from the current text (#144)"
```

---

### Task 9: `textDocument/selectionRange`

**Files:**
- Create: `internal/lsp/selection.go`
- Modify: `internal/lsp/protocol.go`, `internal/lsp/server.go`, `internal/lsp/server_test.go`
- Test: `internal/lsp/selection_test.go`

**Interfaces:**
- Consumes: `parseSyntaxDoc`, `syntaxDoc.toFile`, `syntaxDoc.toModule` (Task 8), `syntax.NodeAt`, `syntax.Contains`, `s.byteAtLines`, `s.enc.wireRange`.
- Produces: `type SelectionRange struct { Range Range; Parent *SelectionRange }`, `selectionRangeReq`.

- [ ] **Step 1: Write the failing tests**

Add to `TestInitialize_Capabilities`:

```go
	if caps["selectionRangeProvider"] != true {
		t.Fatalf("selectionRangeProvider %v", caps["selectionRangeProvider"])
	}
```

```go
// internal/lsp/selection_test.go
package lsp

import (
	"strings"
	"testing"
)

func selectionRanges(t *testing.T, c *client, rel string, ps ...Position) []SelectionRange {
	t.Helper()
	var out []SelectionRange
	if e := c.call("textDocument/selectionRange", map[string]any{
		"textDocument": map[string]any{"uri": c.uri(rel)}, "positions": ps}, &out); e != nil {
		t.Fatalf("selectionRange: %v", e)
	}
	return out
}

// rangeText is the text of r in rel's open text (utf-8: characters are bytes).
func rangeText(c *client, rel string, r Range) string {
	lines := strings.Split(c.texts[rel], "\n")
	if r.Start.Line == r.End.Line {
		return lines[r.Start.Line][r.Start.Character:r.End.Character]
	}
	out := []string{lines[r.Start.Line][r.Start.Character:]}
	out = append(out, lines[r.Start.Line+1:r.End.Line]...)
	return strings.Join(append(out, lines[r.End.Line][:r.End.Character]), "\n")
}

// TestSelectionRange_Show: at `title` of `{model.post.title}` the chain grows through the
// access, the attribute, the element, the declaration and the module.
func TestSelectionRange_Show(t *testing.T) {
	c := start(t, blogCopy(t))
	c.open(showRel, c.disk(showRel))
	got := selectionRanges(t, c, showRel, c.at(showRel, "{model.post.title}>", 12))
	if len(got) != 1 {
		t.Fatalf("%d answers", len(got))
	}
	var texts []string
	for r := &got[0]; r != nil; r = r.Parent {
		texts = append(texts, rangeText(c, showRel, r.Range))
	}
	checks := []func(string) bool{
		func(s string) bool { return s == "model.post.title" },
		func(s string) bool { return s == "title={model.post.title}" },
		func(s string) bool { return strings.HasPrefix(s, "<Layout.Page") && strings.HasSuffix(s, "</Layout.Page>") },
		func(s string) bool { return strings.HasPrefix(s, "view : Model") },
		func(s string) bool { return strings.HasPrefix(s, "module Posts.Show") },
	}
	if len(texts) != len(checks) {
		t.Fatalf("chain of %d: %q", len(texts), texts)
	}
	for i, ok := range checks {
		if !ok(texts[i]) {
			t.Errorf("level %d: %q", i, texts[i])
		}
	}
}

// TestSelectionRange_Edges: one answer per position, a zero-width range where no node holds
// the position (a comment before `module`, the end of the file) (Review Focus 5).
func TestSelectionRange_Edges(t *testing.T) {
	c := start(t, blogCopy(t))
	const rel = "app/Edge.alder"
	c.open(rel, "-- lead\nmodule Edge\n\nx = 1\n")
	got := selectionRanges(t, c, rel, Position{0, 2}, Position{3, 4}, Position{4, 0})
	if len(got) != 3 {
		t.Fatalf("%d answers for 3 positions", len(got))
	}
	if r := got[0].Range; r.Start != (Position{0, 2}) || r.End != r.Start {
		t.Errorf("in the leading comment: %+v", got[0])
	}
	if rangeText(c, rel, got[1].Range) != "1" {
		t.Errorf("at 1: %q", rangeText(c, rel, got[1].Range))
	}
	if r := got[2].Range; r.Start != (Position{4, 0}) || r.End != r.Start {
		t.Errorf("at the end: %+v", got[2])
	}
}
```

- [ ] **Step 2: Run them to see them fail**

Run: `go test ./internal/lsp -run 'SelectionRange|TestInitialize_Capabilities'`
Expected: build failure, `undefined: SelectionRange`.

- [ ] **Step 3: Implement**

```go
// internal/lsp/selection.go
package lsp

import (
	"encoding/json"
	"strings"

	"alder/internal/diag"
	"alder/internal/syntax"
)

// SelectionRange is one step of an expand-selection chain: a range and the next larger one.
type SelectionRange struct {
	Range  Range           `json:"range"`
	Parent *SelectionRange `json:"parent,omitempty"`
}

type selectionRangeParams struct {
	TextDocument struct {
		URI string `json:"uri"`
	} `json:"textDocument"`
	Positions []Position `json:"positions"`
}

// selectionRangeReq answers textDocument/selectionRange from the current text: for each
// position, the nodes from the innermost holding it out to the module (syntax.NodeAt), equal
// spans collapsed. Where no node holds the position, the chain is a zero-width range there.
func selectionRangeReq(s *server, params json.RawMessage) (any, error) {
	var p selectionRangeParams
	if err := json.Unmarshal(params, &p); err != nil {
		return nil, &rpcError{codeInvalidParams, err.Error()}
	}
	out := []SelectionRange{}
	rel, ok := s.rel(p.TextDocument.URI)
	if !ok || !strings.HasSuffix(rel, ".alder") {
		return out, nil
	}
	d := parseSyntaxDoc(rel, s.text(rel))
	for _, wp := range p.Positions {
		bp := s.byteAtLines(d.lines, wp)
		at := d.toModule(diag.Pos{Line: bp.Line + 1, Col: bp.Character + 1})
		var chain []Range // innermost first, in bytes
		path := syntax.NodeAt(d.m, at)
		for i := len(path) - 1; i >= 0; i-- {
			sp := path[i].GetSpan()
			if !syntax.Contains(sp, at) {
				continue
			}
			a, b := d.toFile(sp.Start), d.toFile(sp.End)
			r := Range{Start: Position{a.Line - 1, a.Col - 1}, End: Position{b.Line - 1, b.Col - 1}}
			if n := len(chain); n > 0 && chain[n-1] == r {
				continue
			}
			chain = append(chain, r)
		}
		if len(chain) == 0 {
			chain = []Range{{Start: bp, End: bp}}
		}
		var parent *SelectionRange
		for i := len(chain) - 1; i >= 0; i-- {
			parent = &SelectionRange{Range: s.enc.wireRange(d.lines, chain[i]), Parent: parent}
		}
		out = append(out, *parent)
	}
	return out, nil
}
```

Check `byteAtLines`' signature in `encoding.go:113` (`func (s *server) byteAtLines(lines []string, p Position) Position`). It returns a byte `Position`, and the zero-width fallback reuses it, so that range goes through `wireRange` too.

Capabilities: `"selectionRangeProvider": true,`. fastMethods: `"textDocument/selectionRange": selectionRangeReq,`.

- [ ] **Step 4: Run the tests**

Run: `go test ./internal/lsp -run 'SelectionRange|TestInitialize'`, then `go test ./internal/lsp`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/lsp/selection.go internal/lsp/selection_test.go internal/lsp/protocol.go internal/lsp/server.go internal/lsp/server_test.go
git commit -m "lsp: selection ranges from the current text (#144)"
```

---

### Task 10: the book, verification, pull request

**Files:**
- Modify: `book/11-tooling.md` ("What you get", after the colour swatches bullet), `book/13-compiler.md` (section 10, a new subsection before "Scratch files in the language server")

- [ ] **Step 1: Add the tooling bullets**

After the **Colour swatches** bullet in `book/11-tooling.md`, add:

```markdown
- **Highlighting, folding and expand selection.** The server colours the
  file itself: it sends each name's kind (a function where it is defined, a
  parameter, a field, a tag, a constructor) as *semantic tokens*, read from
  the text as it is now, saved or not. An editor without an Alder grammar
  (VS Code, Zed, Helix) highlights Alder from these alone. It can also fold
  each declaration, block, `match`, element, record, list, comment block and
  the imports, and grow a selection outward one construct at a time (in VS
  Code, Shift+Alt+→). Neovim keeps colouring with tree-sitter; the two agree
  by construction (chapter 13, section 10).
```

- [ ] **Step 2: Add the compiler section**

In `book/13-compiler.md`, section 10, before `### Scratch files in the language server`, add a subsection. It is for a beginner, as the rest of the book is, and uses `text` fences for any partial Alder (the book verifier compiles `alder` fences):

```markdown
### Highlighting from the server

An editor colours code by sorting its words into kinds: keyword, string,
function, parameter, field. Most editors learn the kinds from a grammar of
their own. Alder's server can tell them instead, through the protocol's
**semantic tokens**: a list in which each entry is a place in the file (line,
column, length) and a kind from a fixed list the server announced at
`initialize`, its *legend*. The legend is in `internal/semtok/legend.go`:
eighteen kinds (`namespace`, `type`, … `tag`, `attribute`, `boolean`,
`hole`) and three modifiers (`declaration` on a name where it is bound,
`documentation` on a comment block above a declaration, and one reserved).

The kinds come from one parse of the text on screen
(`syntax.ParseTokens`), in two layers:

- the **lexical** layer looks at each token alone: a keyword, an operator,
  a number, a string, a hole;
- the **structural** layer walks the syntax tree and says what a name is by
  where it sits. The first name of a value declaration is a function being
  declared; the last name of `post.title` is a field; in
  `<Layout.Page title={…}>` `Layout` is a module, `Page` a tag and `title`
  an attribute.

The walk visits a node's children before the node, so the larger construct
has the last word: in `xs.map f`, the access makes `map` a field, and then
the application makes it a method. Nothing here needs the type checker,
which is why the server can answer on every keystroke without waiting for
a rebuild.

On the wire the list is compressed into five integers per token: lines
apart from the previous token, columns apart (on the same line), length,
kind, modifiers. Columns and lengths are counted in the unit the editor
asked for (section "Position encodings"): VS Code counts UTF-16 units, so
`é` counts one and `😀` two, though they are two and four bytes.

Neovim colours Alder with tree-sitter (`tree-sitter-alder`). To keep the two
editors alike, a test (`tree-sitter-alder/highlights_test.go`) runs nvim's
highlight queries and the server's tokens over every module in the
repository and compares the kind of every byte. A rule changed on one side
and not the other fails it. The one place they differ on purpose is inside
`{…}` in a string: tree-sitter sees one string there, while the server
colours the expression inside.

Folding and expand selection come from the same parse. A folding range is
the first and last line of a declaration, block, `match`, element, record,
list or comment run. A selection range is the chain of nodes around the
cursor, from the innermost outward (`syntax.NodeAt`).
```

- [ ] **Step 3: Verify**

Run each and confirm the expected output:

```bash
go vet ./...
go test ./internal/syntax ./internal/semtok ./internal/lsp ./tree-sitter-alder
go test ./book/...
```

Expected: all PASS. `go test ./book/...` checks the book's snippets and links. Then run `go test ./...` once, in the foreground with a 20-minute timeout (`go test -timeout 20m ./...`). Report any failure outside the packages above as such: it is not evidence about this change, but it must be named in the PR, not hidden.

- [ ] **Step 4: Commit and open the pull request**

```bash
git add book/11-tooling.md book/13-compiler.md
git commit -m "Book: highlighting, folding and selection from the server (#144)"
git push -u origin lsp-semantic-tokens
gh pr create --title "LSP: semantic tokens, folding and selection ranges (#144, milestone 1)" --body-file <file>
```

The PR body says, in this order:
- what the server now answers (the four methods, the legend, current text only);
- the rulings R1–R7 in one line each;
- every `highlights.scm` edit (R4's three plus any from Task 6 step 5), and every `parityExceptions` entry with its why;
- that Neovim 0.11 picks these tokens up as soon as this ships, so milestone 2 (alder.nvim keeps tree-sitter) should follow promptly;
- the test commands run and their results;
- `Refs #144` (the issue stays open for milestones 2–8);
- the PR attribution footer.

After merge: reinstall `alder` (`go install ./cmd/alder`) and rebuild the parser if the queries changed. The user restarts nvim.

---

## Self-review notes

- **Spec coverage:** capabilities and `fastMethods` (Tasks 7–9); `ParseTokens` (1); lexical and structural layers, legend, modifiers, `_` and punctuation without tokens, interpolations without overlap, contextual keywords (2–4); encoding through `toWire` with a UTF-16 test (7); range = full cut to lines (7); broken text coloured (4, 7); unknown URI empty (7); folding kinds, comment and import runs, single-line nodes excluded (8); selection chain (9); `TestInitialize_Capabilities` with the exact legend (7–9); parity test with the drift test's gating (6). Phase 2, alder.nvim and the extension are later milestones. Out of scope.
- **Departures** are the rulings R1–R7, each with its reason.
