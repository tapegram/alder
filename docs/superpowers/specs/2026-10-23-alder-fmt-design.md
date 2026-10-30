# `alder fmt` — canonical formatter (design)

Issue: #18. Date: 2026-10-23.

## Intent

One canonical layout for Alder source, produced by `alder fmt`, applied on
save through the LSP, and emitted by every command that writes Alder
(`new`, `edit`, `fill`, `remove`). No options.

Why now: agents write most Alder. They don't choose line breaks
thoughtfully (the nightvote replay produced one-line record types with
nine fields), and a human reading agent-written code pays for every
inconsistency. A formatter that is fast, safe and deterministic means the
agent's starting point is always canonical, diffs show only real changes,
and "where does the next field go" has exactly one answer.

Priorities, in order: safe on save, extremely fast, readable output.

## Goals

- `alder fmt` rewrites files to the canonical style; `alder fmt --check`
  reports files that would change.
- LSP `textDocument/formatting` so editors format on save.
- Every Alder source the toolchain emits is already formatted.
- Comments survive, verbatim, in the position a reader expects.
- Deterministic: output depends on the program's structure, not on how
  the input happened to be laid out (blank lines are the one exception,
  below).

## Non-goals

- Configuration of any kind (width, indent, comma style, …).
- Range formatting, format-on-type, `--diff`.
- Reflowing or rewriting comment text.
- Semantic rewrites (gofmt -s style simplifications, pipeline rewriting,
  import pruning). Redundant parentheses are the one normalisation, since
  the AST has no paren node (see Parentheses).
- Formatting JS/CSS inside `<script>`/`<style>`: emitted verbatim.
- Formatting the expression inside a string interpolation `"…{expr}…"`:
  the whole string literal is emitted verbatim.
- Changes to alder.nvim beyond documenting the format-on-save autocmd.

## Anti-goals (failure even if it "works")

- Ever dropping a comment, or moving one so it reads as describing
  something else.
- Ever changing program meaning. Markup whitespace is semantic (JSX
  rule, book 05); a formatter that "tidies" text children is a bug.
- Turning a file that didn't parse into a different file.
- Format-on-save that is perceptible (>50 ms on a typical file) or that
  makes the cursor jump.
- A style that depends on the source's existing line breaks (Prettier's
  "keep objects expanded if the first key was on a new line", elm-format's
  "multiline if it was multiline"). Agents don't signal intent through
  layout; such rules just make output unpredictable.

## Constraints from the language (verified in the parser)

- `--` to end of line is the only comment form. Today the lexer drops
  them (`internal/syntax/lexer.go` ~143). `<script>`/`<style>` bodies are
  raw text; `--` inside them is not a comment.
- Not offside/indent-sensitive. Statements in a block are separated by
  newlines; top-level declarations start at column 1.
- Line continuation is allowed only: inside unclosed `( ) [ ] { }`; after
  a line ending in an operator (`=`, `->`, `then`, `else`, binary
  operators); when the next line begins with a binary operator (`|>`,
  `??`, `||`, `&&`, comparisons, `+ - ++ * / %`). `<` at line start
  continues a comparison only when followed by whitespace.
- **Function application cannot span lines**: `parseApp` stops at a
  newline. A long `f a b c` can only be shortened by breaking *inside* a
  bracketed or lambda argument. The formatter never inserts parentheses
  to create a break.
- Trailing commas are accepted in records, record types, lists, match
  arms and field lists; *not* in record update `{ r | a: 1, b: 2 }`.
- Markup children follow the JSX whitespace rule: whitespace runs that
  contain a newline are dropped; a space between text and a node on the
  same line is kept.
- The AST (`internal/syntax/ast.go`) has start+end spans (line/col) on
  every node, no paren node, and no comment storage. `Dump()` gives a
  cheap structural equality check.

## Decisions

### 1. `fmt` is separate from `check`

`alder check` stays format-agnostic — it answers "is the program
correct", and a cosmetic failure in that signal is noise for an agent
fixing a type error. Enforcement is `alder fmt --check` in CI. The
documented recipe is `alder fmt --check && alder check`. `check` never
mutates files.

### 2. Everything the toolchain emits is fmt-clean

`internal/generate` runs its output through the formatter;
`generate.Tidy()` is retired (today called from `auth.go`, `page.go`,
`typedecl.go`, `capability.go`, `admin.go`; the plan enumerates every
call site). Splice edits (`edit`, `fill`, `remove`) and `regen admin`
format the **whole file** after splicing — intended: a canonical file
stays unchanged elsewhere, and this is what format-on-save does anyway.
Template golden tests then enforce canonical output for free.

Runtime failure policy: if the formatter cannot format emitted text
(parse error in the generated output, or the safety check fails), the
command still writes the unformatted text and prints a one-line warning
to stderr (`alder new: warning: output not formatted: …; please file an
issue`). Aborting would strand the user; the tests are what catch the
bug. A splice into a file that doesn't parse *before* the edit is
written unformatted with the same warning.

Region markers (`-- @admin begin <name>` / `-- @admin end`,
`internal/generate/regions.go`) are ordinary own-line comments to the
formatter. They are always leading or dangling (never trailing, since
they sit alone on their line), and the comment rules below keep an
own-line comment on its own line, so `parseRegions` still finds them.
Code inside a region may be reflowed; `regen` runs the formatter after
replacing the region, so `regen` then `fmt` is a no-op.

### 3. Safety contract

- Parse errors → no output. CLI prints the diagnostics (same renderer as
  `check`) and skips the file; LSP returns no edits.
- After printing, re-parse the output and require `Dump` equality with
  the input (comments excluded from the dump) *and* that the sequence of
  comment texts is unchanged. One exception: imports are sorted and their
  comments move with them, so the comments of the import block (after the
  module line, up to the last import's line) are compared as a multiset,
  with a marker at the block's end so no comment can move into or out of
  the block. `Dump` omits spans (verified); the plan's
  first task confirms it covers every semantic field (`HtmlText.Raw`,
  attribute order, `HoleExpr`, literal values) and adds a normalised dump
  if anything is missing. On mismatch: don't write, report
  `alder fmt: internal error: <file>: formatting changed the program; please
  file an issue`, exit 2. This is Black's `--safe` check; there is no
  `--fast` flag.
- Idempotency (`fmt(fmt(x)) == fmt(x)`) is tested over the whole corpus,
  not checked at runtime.
- LSP returns minimal edits (line-based diff of old vs new, no
  intra-line diffing), not a whole-document replace, so cursor and marks
  survive.

### 4. Style

Where the book's snippets already fix a convention the formatter follows
it; the book is the reference corpus. Spacing as in the examples:
`name : Type` in signatures, `field: value` in records and record types,
`match x {` on one line, K&R braces.

- 2-space indent, LF, exactly one trailing newline, no trailing
  whitespace, tabs only inside string literals. CRLF input is normalised
  to LF and counts as a change under `--check`.
- Width budget **100 columns**, counted in runes, used only as the
  tiebreaker where no structural rule applies. Lines that cannot be
  broken legally (see Application) may exceed it. A string literal
  containing a newline is treated as containing hard line breaks: it
  forces its enclosing group vertical and width is measured per line.
- **Literals verbatim**: `IntLit`/`FloatLit` are printed from the source
  text through their span (so `1.50` stays `1.50`), as are strings. The
  AST gains no raw-text field; the printer has the source.
- **Trailing commas** on every item in a vertical group, except where the
  grammar forbids them (record update).
- Imports: sorted, de-duplicated, one blank line after the import block
  and after `module`.
- Top-level declarations separated by exactly one blank line. A signature
  sits directly above its value with no blank line.
- **Blank lines inside blocks and markup**: the one source-respecting
  rule. Runs collapse to one; zero stays zero. Never a blank line directly
  after an opening bracket or before a closing one.
- **Records, record types, forms, tables, page params, session, deploy**:
  3 or more fields → always vertical. 1–2 fields → one line if it fits,
  else vertical. Vertical form: opening bracket stays on the current line,
  one field per line indented, closing bracket alone on a line at the
  parent indent.
- **Record update** `{ r | a: x }`: the same field-count rule; a 1–2
  field update that fits stays on one line. Vertical form: `{ r |` on the
  opening line (the base always follows `{`), one field per line indented
  one level from the line that opened it, no trailing comma (the grammar
  forbids it), `}` alone at that line's indent. Hugged as a lambda body or
  an argument it breaks K&R exactly like a hugged record literal, the
  closing parens directly after the `}`:

  ```
  List.map (\row -> Db.update Post row.id (\r -> { r |
    published: p.form.value
  })) model.rows
  ```
- **Record types inside a function type** (signatures, capability and fake
  entries, `let` annotations) are width-based whatever their field count:
  one line if it fits, else K&R with the rest of the type on the closing
  line (`send : {` … `} -> Task Unit`). A function type holding a record,
  directly or nested (`Task { a: Int }`), first keeps its arrows inline and
  lets the record break; the record's fields indent one level from the
  line the type starts on, not from the arrows. If a line of that layout is
  still over the width (a long tail after `}`), the arrows break
  `->`-leading instead, each record on its own `->` line, one line if it
  fits, else K&R there (#27). A function type nested inside another stays
  on one line.
- **Capability and fake bodies**: always one entry per line.
- **Lists**: one line if it fits, else one element per line.
- **Sum types**: one line if it fits, else one constructor per line,
  `|`-leading. Preferred layout:

  ```
  type Msg =
    Inc
    | Dec
    | Set Int
  ```

  If the preferred layout is not accepted by the parser, the builder may
  relax newline tolerance in the parser (and tree-sitter, and A-grammar,
  keeping `drift_test.go` green) — never add tokens or change meaning.
  Same licence applies to any other layout in this section (arm body
  after `->`, `else if`). These relaxations land as the plan's first
  milestone, before the printer, so every layout the printer emits is
  already accepted.
- **match**: `match subject {` on one line; arms always one per line (even
  one arm), each with a trailing comma; an arm body goes on the arm line
  if it fits, else on the next line indented 2 after `->`.
- **if/then/else**: one line if it fits, else
  ```
  if cond then
    a
  else
    b
  ```
  with `else if` chains flattened at the same indent — in markup too,
  where `{if c then` … `else if d then` … `else` … `}` flattens an `else`
  whose body is an `if` expression. A comment above the inner `if` has no
  line to sit on in the flattened form, so an `else` led by a comment
  keeps the nested layout: `else`, the comment and the inner `if` each on
  a line of their own, indented under `else` (in code and in markup). A block branch opens
  on the `then`/`else` line, K&R: `if c then {` … `} else {` … `}`, and
  `} else if d then {`.
- **Lambdas**: `\x y -> body`; a body that doesn't fit breaks after `->`.
- **Pipelines** (`|>`): 2 or more pipes → always vertical, one `|>` per
  line, operator leading, indented 2 from the expression's start line.
  One pipe → width-based.
- **Other binary operator chains**: one line if they fit, else one
  operator per line, operator leading (the grammar's continuation form).
  A chain with an operand that always spans lines (a vertical record, a
  block) keeps its operators inline, `} == {`. The operands after the
  first share one indentation, that of the line the first one ends on:
  `} == {` keeps its fields one level in (not two), an operand after a
  first one that broke deeper is not left of it, and a long chain of
  multi-line operands does not step further in with each one (#27).
- **Application**: arguments stay on the function's line (grammar). If
  the line is too long, break inside the *last* argument when it is a
  record, list, lambda or parenthesised expression (Prettier's "hugging"),
  or inside an earlier argument that is a lambda, record, list or block
  (`List.map (\row -> {` … `}) rows`); otherwise leave the line long.
  Never wrap in parens to force a break. A body after `=`, `->` or `<-`
  that is a call hugging its last argument stays on that line unless the
  call's head (the line up to where the argument opens) is over the width
  there and fits on the next line: then the call starts on the next line,
  indented (#27).
- **Blocks** `{ let … ; result }`: a block with statements is always
  vertical. A block with only a result stays a block, `{ x }` on one line
  if it fits: blocks are shape-significant to the checker and lowering
  (a test whose body is a block is an effect test; an `update` whose body
  is a `match` splits its arms), so `{ x }` and `x` are not the same
  program.
- **Parentheses**: re-derived from precedence; the minimal set is
  emitted, so redundant parens disappear. The equivalence check guards
  the precedence table. One exception: a lambda argument is always
  parenthesised, `f (\x -> e)`, though `f \x -> e` parses (Alder code
  and Elm always write the parentheses).
- **Empty records**: `{}` as an expression or pattern; `{ }` for an empty
  record type, form or declaration body.
- **Markup**: an element with 2 or more element children — counting the
  elements directly inside its `{if}` and `{match}` children (a branch, an
  arm), and a `{for}` whose body is an element as 2 (it renders many) — is
  always vertical, one child per line. An
  element with 0–1 element children → one line if it fits, else children
  one per line, indented. A blank line between children keeps the element
  vertical. Attributes: on the tag line if they
  fit, else one per line indented 2, with `>` closing on the last
  attribute's line. Runs of inline children joined by significant spaces
  move as a unit; the formatter never splits a run across lines in a way
  that changes the JSX whitespace result — the Dump equality check is the
  oracle. `<script>`/`<style>` bodies verbatim, indentation included: the
  formatter does not re-indent them when the tags around them move (a
  body's whitespace is the script's or stylesheet's own). A self-closing
  element prints `<br />`, however the source spaced the `/ >`; an empty
  element written with an end tag keeps it (`<p></p>`).
- **Strings and holes**: verbatim.
- **Comments**: text after `--` verbatim (trailing spaces trimmed). A
  trailing comment on an item forces its group vertical (a line comment
  can't live mid-line).

### 5. Comments: representation and attachment

- The lexer collects comments into a side list
  (`Comment{Text, Line, Col, Offset}`) instead of discarding them.
  Comments are *not* tokens, so the parser is untouched. `ParseFile`
  exposes them on `Module` (e.g. `Module.Comments`).
- Attachment runs after parsing, by position:
  - **Leading**: a comment on its own line attaches to the next item at
    its level (declaration, statement, field, arm, list element, markup
    child) and prints on its own line(s) above it, at the item's indent.
    A blank line before the comment, and a blank line between the comment
    and its item, are each preserved (collapsed to one) — so a comment
    that closes a section (`-- @admin end`) stays separated from the next
    declaration.
  - **Trailing**: a comment on the same line as an item attaches to that
    item and prints after it, one space separated.
  - **Dangling**: a comment with no following item inside a bracketed
    group (before `}`/`]`/closing tag, or at end of file) prints at the
    end of that group at the items' indent.
  - A comment the attacher cannot place (e.g. between an operator and its
    operand) attaches as leading to the smallest enclosing statement or
    declaration. Never dropped — the comment-sequence check enforces it.

### 6. CLI

`alder fmt [paths…]` — paths are files or directories; a directory is
walked for `*.alder`, skipping `build/` and dot-directories. Default `.`.

- Default mode rewrites in place; prints one line per changed file,
  nothing when nothing changed.
- `--check`: writes nothing; prints `<file> would be reformatted` per
  file; exit 1 if any.
- `--stdin`: formats stdin to stdout (for editors and agents); errors to
  stderr.
- Exit codes follow `check`: 0 clean; 1 = would reformat (`--check`) or
  some file failed to parse; 2 = usage, IO, internal error. Messages are
  prefixed `alder fmt: `. Register in `help.go`, `helpjson.go` and the
  `testdata/help.json` golden.

### 7. LSP

Advertise `documentFormattingProvider`. The handler formats the current
buffer text from the document overlay — a standalone parse, not the full
app analysis — and returns minimal `TextEdit`s. Parse error → `null`
result (no edits, no error popup; diagnostics already show the error).

### 8. Performance

Targets, enforced by a benchmark in the plan: a 1000-line file formats in
under 5 ms after parsing on a laptop; all of `examples/` under 200 ms
total. Single pass over the AST; no regex over source; the width check is
the usual bounded look-ahead.

### 9. Tests

- Golden: `testdata/golden/fmt_*.alder` → `.fmt.alder`, with the
  existing `-update` flag convention, one file per rule group above.
- Corpus round trip over every file that parses cleanly in `examples/`,
  `testdata/corpus/`, `stdlib/` and the book snippets: Dump equality,
  comment-sequence equality, idempotency.
- Generator templates: output parses and is fmt-clean.
- LSP: formatting request on an open document returns edits that produce
  the formatter's output; parse error returns null.

### 10. Documentation

- Book `11-tooling.md`: `alder fmt` section (modes, exit codes, CI
  recipe, format-on-save snippet for alder.nvim).
- A short "Style" section (03-syntax or a new appendix) stating the rules
  in §4, so agents reading the book write canonical code the first time.
- Agent-facing instructions shipped by the scaffold: write → `alder fmt`
  → `alder check`.

## Architecture (recommendation; builder may deviate with reason)

New package `internal/format` (not `fmt`, to avoid shadowing Go's):

```
func Source(file string, src []byte) (out []byte, diags []diag.Diagnostic, err error)
```

Wadler/Prettier-style document IR (text, concat, nest, line, softline,
hardline, group) with a fits-in-width renderer; a printer that walks the
AST plus attached comments and emits the IR; structural forcing expressed
as hardlines inside a group. Comments and parens make the token trace
unnecessary.

## Left to the builder

- Exact attachment algorithm and its edge cases (comment between a
  signature and its value, comment after `=` before the body, comment
  inside an empty record, comment between `then` and the branch).
- Which layouts need parser newline relaxations, and the matching
  tree-sitter/A-grammar edits.
- Precedence/associativity table for paren re-derivation (derive from
  `parser_expr.go`).
- Markup inline-run detection details, within the Dump-equality oracle.
- How splice edits handle a file that didn't parse before the edit.
- Milestone split and ordering of the plan.
