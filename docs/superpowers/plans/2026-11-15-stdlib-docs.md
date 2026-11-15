# Standard Library Docs Implementation Plan (#119, #227)

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Every declaration in `stdlib/*.alder`, every operator and every keyword has a doc with
checked examples, shown by hover, completion and `alder search`, and the book gets a generated
reference built from the same comments.

**Architecture:** A doc stays an ordinary `--` comment read by `syntax.DocOf`. A new package
`internal/doc` parses that text into summary, details, examples and a book reference, and renders
it as markdown; hover, completion, search, the fill prompt, the tests and the reference generator
all go through it. Tests in the `book` package hold the stdlib to the format and run every example
in one batched `alder eval`. Operators and keywords get the same treatment from a Go table
(`internal/langdoc`).

**Tech Stack:** Go (language server, CLI, tests), Alder (`stdlib/*.alder` comments), markdown
(`book/`).

**Spec:** `docs/superpowers/specs/2026-11-15-stdlib-docs-design.md`. Read it first; this plan
argues from it and does not repeat its format section.

## Global Constraints

- Six PRs, each from a short branch off main and merged before the next starts: PR 1 = Tasks 1–8,
  PR 2 = Task 9, PR 3 = Task 10, PR 4 = Task 11, PR 5 = Task 12, PR 6 = Tasks 13–14.
- No new comment syntax. The tree-sitter grammar, the formatter and semantic tokens are not
  touched.
- No stdlib signature or behaviour changes. After any edit under `stdlib/`, this prints nothing:

  ```bash
  for f in stdlib/*.alder; do diff <(git show main:"$f" | grep -vE '^\s*(--|$)') <(grep -vE '^\s*(--|$)' "$f") >/dev/null || echo "CHANGED $f"; done
  ```
- The Go code in `rt/std/` decides what a doc says. `internal/gen/stdcall.go` maps a name to its
  function (`"List.take": "std.ListTake"` → `rt/std/list.go`). Document what the code does; where
  it looks wrong, document it as it is and file an issue.
- Docs are full sentences in plain words for a beginner. No doc restates the signature.
- Examples are never run one process each: `alder eval` builds a binary per run (#54).
- Scoped tests per task; `go test ./...` once before each PR is opened. Each PR adds a CHANGELOG
  line under the unreleased heading.
- After each merge: `go install ./cmd/alder`.

## Review Focus

Inputs the spec implies and a user will hit. Each has a test in the task named.

1. **An app comment that is not in the format** (prose, a code-looking line, more prose; a
   markdown list). Hover shows all of it, in the order written, with nothing dropped. Task 1.
2. **An example whose text would break the markdown fence** (it holds three backticks, as a
   `Text` example can). The rendered block uses a longer fence. Task 1.
3. **A wrong example in the middle of a file**: one that does not type-check and one that is
   false. Both are reported in one run, each against its own `stdlib/X.alder:LINE`, and a
   multi-line example is reported at its first line. Task 2.
4. **A stdlib declaration with no `Book:` line and an app declaration with none.** The first shows
   its module's book line; the second shows none. A test helper's "test helper (uses …)" line
   still appears, above the doc. Task 4.
5. **`alder search --doc` together with `--json`, and with no results.** With `--json` the output
   is the same JSON as without `--doc`; with no results nothing is printed to stdout. Task 6.

## Writing a module's docs (Tasks 3, 9, 10, 11)

The same procedure for every module. A module is one `stdlib/<Name>.alder` file.

1. Read the file, the module's section of the book (table below), and the Go function behind
   every declaration (`internal/gen/stdcall.go` → `rt/std/`). Types and special forms have no
   entry in `stdcall.go`; find their handling by grepping the name in `internal/check` and
   `internal/gen`.
2. Above the `module` line, write the module doc: a one-line summary of what the module is for,
   optional details, and a `Book:` line. The table gives the default; choose a narrower heading
   if one fits better. The heading text must match the book exactly.
3. For each declaration write the doc in the spec's format. Keep every fact the existing comment
   states. For a pure function, each edge case named in the details has a checked example. For an
   effect or a value with no equality, write a usage example.
4. A section header comment keeps a blank line under it. A comment that sits directly above a
   declaration but describes a group is moved up, and what it says goes into the docs of the
   declarations it covers.
5. Remove the module from `pendingDocs` (`book/stdlibdocs_test.go`).
6. Run `go test ./book -run 'TestStdlibDocs'` until it passes, then the signatures check under
   Global Constraints.
7. A function that cannot have even a type-checked example goes in `noExample` with a one-line
   reason. Behaviour that looks wrong gets an issue, listed in the PR body.

| Module | Default `Book:` line |
|---|---|
| List, Maybe, Result, Text, Bool, Dict, Html, Json, Random, Test, Fuzz | `chapter 9, "<Module>"` |
| Int, Float | `chapter 9, "Int, Float"` |
| Date, Time | `chapter 9, "Date, Time"` |
| File, Bytes | `chapter 9, "File, Bytes"` |
| Query, Task, Db, Session, Log, Http | `chapter 9, "Query, Task, Db, Session, Log, Time, Random, Http"` |
| Response | `chapter 9, "Response, Route"` |
| Notice | `chapter 6, "Notices and in-flight state"` |
| Url, Prelude | find the section that teaches it (`grep -n 'Url\.' book/*.md`) |

Modules in one task are independent files; they can be written in parallel. Only `pendingDocs`
is shared.

---

## PR 1: format and plumbing

### Task 1: The doc parser and renderer

**Files:**
- Create: `internal/doc/doc.go`
- Test: `internal/doc/doc_test.go`

**Interfaces:**
- Consumes: text as `syntax.DocOf` returns it (lines joined by `\n`, `--` and one space removed).
- Produces:

  ```go
  package doc

  type Example struct {
      Source string // its lines joined by "\n", four spaces removed from the front of each
      Line   int    // 0-based index of its first line in the text given to Parse
  }
  type Doc struct {
      Summary  string   // the first paragraph, its lines joined by a space; "" when the text starts with an example or a Book line
      Details  []string // the other prose paragraphs, in order, as written
      Examples []Example
      Book     string   // the text after "Book: " (`chapter 9, "List"`); "" when absent
  }
  func Parse(text string) Doc
  // Markdown is the paragraphs in the order written, each run of examples as one fenced `alder`
  // block, then the book line in italics (d.Book, or book when d.Book is ""). "" for an empty doc.
  func (d Doc) Markdown(book string) string
  // ParseBook splits `chapter 9, "List"` into ("chapter", "9", "List") and `appendix B, "X"` into ("appendix", "B", "X").
  func ParseBook(ref string) (kind, number, heading string, ok bool)
  type Strict struct{ Example, Book bool }
  // Problems are the ways text breaks the stdlib format; nil when it follows it.
  func Problems(text string, need Strict) []string
  ```

Parsing rules: paragraphs are separated by blank lines (a line of only spaces is blank). A
paragraph whose every line starts with four or more spaces is an example block; in it a line with
exactly four spaces starts an example and a deeper line continues it. A last paragraph that is one
line starting `Book: ` is the book line. Everything else is prose. Nothing is an error.

- [ ] **Step 1: Write the failing tests**

```go
const take = "The first n elements of a list.\n\nA list shorter than n comes back whole. An n of zero or less gives the empty list.\n\n    List.take 2 [1, 2, 3] == [1, 2]\n    List.take 0 [1, 2] == []\n\nBook: chapter 9, \"List\""

func TestParse(t *testing.T) {
	d := Parse(take)
	if d.Summary != "The first n elements of a list." || len(d.Details) != 1 || d.Book != `chapter 9, "List"` {
		t.Fatalf("%+v", d)
	}
	if len(d.Examples) != 2 || d.Examples[0] != (Example{"List.take 2 [1, 2, 3] == [1, 2]", 4}) || d.Examples[1].Line != 5 {
		t.Fatalf("%+v", d.Examples)
	}
}

func TestParse_Continuation(t *testing.T) {
	d := Parse("    List.map\n        (\\x -> x * 2)\n        [1, 2] == [2, 4]\n    List.length [] == 0")
	if len(d.Examples) != 2 || d.Examples[0].Source != "List.map\n    (\\x -> x * 2)\n    [1, 2] == [2, 4]" || d.Examples[1].Line != 3 || d.Summary != "" {
		t.Fatalf("%+v", d)
	}
}

func TestMarkdown(t *testing.T) {
	want := "The first n elements of a list.\n\nA list shorter than n comes back whole. An n of zero or less gives the empty list.\n\n```alder\nList.take 2 [1, 2, 3] == [1, 2]\nList.take 0 [1, 2] == []\n```\n\n*Book: chapter 9, \"List\"*"
	if got := Parse(take).Markdown(""); got != want {
		t.Fatalf("got %q", got)
	}
	if got := Parse("Sum.").Markdown(`chapter 9, "List"`); got != "Sum.\n\n*Book: chapter 9, \"List\"*" {
		t.Fatalf("fallback: %q", got)
	}
	if got := Parse("").Markdown(`chapter 9, "List"`); got != "" {
		t.Fatalf("empty: %q", got)
	}
}

// Review Focus 1: an app comment written before the format keeps every line, in order.
func TestMarkdown_Lenient(t *testing.T) {
	got := Parse("lastPage bounds `page`\nto the last page\n\n    page 3\n\n- one\n- two").Markdown("")
	want := "lastPage bounds `page`\nto the last page\n\n```alder\npage 3\n```\n\n- one\n- two"
	if got != want {
		t.Fatalf("got %q", got)
	}
}

// Review Focus 2: an example holding a fence gets a longer one.
func TestMarkdown_FenceInExample(t *testing.T) {
	got := Parse("Sum.\n\n    Text.length \"```\" == 3").Markdown("")
	if !strings.Contains(got, "````alder\nText.length \"```\" == 3\n````") {
		t.Fatalf("got %q", got)
	}
}

func TestProblems(t *testing.T) {
	const oneLine = "the summary is not one line ending in a full stop"
	cases := []struct {
		text string
		need Strict
		want []string
	}{
		{"", Strict{}, []string{"no doc comment"}},
		{"the first n code points", Strict{}, []string{oneLine}},
		{"Two lines\nof summary.", Strict{}, []string{oneLine}},
		{"Sum.", Strict{Example: true}, []string{"no example"}},
		{"Sum.", Strict{Book: true}, []string{"no Book line"}},
		{"Sum.\n\nBook: ch 9", Strict{}, []string{"the Book line is not `Book: chapter N, \"Heading\"` or `Book: appendix X, \"Heading\"`"}},
		{"Sum.\n\n    1 == 1\n\nMore prose.", Strict{}, []string{"prose follows the examples"}},
		{"Sum.\n\n    \t1 == 1", Strict{}, []string{"a tab in an example"}},
		{take, Strict{Example: true, Book: true}, nil},
	}
	for _, c := range cases {
		if got := Problems(c.text, c.need); !slices.Equal(got, c.want) {
			t.Errorf("%q: got %q, want %q", c.text, got, c.want)
		}
	}
}

func TestParseBook(t *testing.T) {
	if k, n, h, ok := ParseBook(`appendix B, "Common mistakes"`); !ok || k != "appendix" || n != "B" || h != "Common mistakes" {
		t.Fatal(k, n, h, ok)
	}
	if _, _, _, ok := ParseBook("ch 9"); ok {
		t.Fatal("ch 9 parsed")
	}
}
```

- [ ] **Step 2: Run them and see them fail**

Run: `go test ./internal/doc`
Expected: FAIL, the package does not compile (`Parse` undefined).

- [ ] **Step 3: Implement `internal/doc/doc.go`** to the interface and rules above. `Parse` keeps
  the paragraphs in order internally so `Markdown` can render them as written.

- [ ] **Step 4: Run the tests**

Run: `go test ./internal/doc`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/doc && git commit -m "doc: parse and render doc comments (#119)"
```

### Task 2: The stdlib doc tests

**Files:**
- Create: `book/stdlibdocs_test.go`
- Create: `book/testdata/stdlibdocs/Bad.alder`

**Interfaces:**
- Consumes: `doc.Parse`, `doc.Problems`, `doc.ParseBook` (Task 1); `syntax.ParseSignatures`,
  `syntax.DocOf`, `syntax.ParseExpr`; `stdlib.FS`; the `book` package's `alder(t)` (the built
  binary), `exampleApp(DefaultExample)` and `tmpRoot`.
- Produces (all in package `book`, test files):

  ```go
  // pendingDocs are the modules not yet documented; TestStdlibDocs_Complete skips them.
  var pendingDocs = map[string]bool{ /* all 27 module names */ }
  // noExample are the values that cannot have an example, each with the reason.
  var noExample = map[string]string{}
  // stdlibDocProblems lists findings as "stdlib/List.alder:7: List.take: no example".
  func stdlibDocProblems(fsys fs.FS, pending map[string]bool, noExample map[string]string, headingExists func(ref string) bool) []string
  // bookHeadingExists: "chapter 9" is 09-*.md, "appendix B" is B-*.md; the heading is a
  // heading line's exact text after its #s.
  func bookHeadingExists(ref string) bool
  type docExample struct{ Where, Source string } // Where: "stdlib/List.alder:14"
  func stdlibExamples(fsys fs.FS) []docExample
  // runExamples checks every example in one batch and returns the failures.
  func runExamples(t *testing.T, exs []docExample) []string
  ```

What `stdlibDocProblems` requires, per file not in `pending`: a module doc passing
`Problems(…, Strict{Book: true})`; every `type` passing `Problems(…, Strict{})`; every value
passing `Problems(…, Strict{Example: true})` unless it is in `noExample`; every `Book:` reference
passing `headingExists`, else `Book: <ref>: no such heading in the book`. A `noExample` key that
names no declaration is a finding too.

How `runExamples` works: one `alder eval <todos> --json -` whose stdin is, per example,
`let ex<i> =` followed by the example's lines indented four spaces, then a last line that is the
list of the checked examples (`[ex1, ex4]`, or `0` when there are none). An example is checked
when `syntax.ParseExpr` gives a `*syntax.BinOp` with `Op == "=="` at the root; the others are only
type-checked by being in a `let`. A diagnostic at `input:L` maps back to the example that holds
line L. When there are diagnostics, report them, drop those examples and run once more, so a false
example is still reported beside an ill-typed one. For each `False`, evaluate the left operand
alone (one more `alder eval`) and report `want <right operand text>, got <value>`.

- [ ] **Step 1: Write the fixture** `book/testdata/stdlibdocs/Bad.alder`:

```alder
module Bad

undocumented : Int -> Int
-- lowercase fragment
fragment : Int -> Int
-- No example here.
noExample : Int -> Int
-- A false example.
--
--     1 + 1 == 3
wrong : Int -> Int
-- An ill-typed example, on two lines.
--
--     1 +
--         "a" == 2
illTyped : Int -> Int
-- A book line that names no heading.
--
--     1 + 1 == 2
--
-- Book: chapter 9, "No such heading"
badBook : Int -> Int
```

- [ ] **Step 2: Write the failing tests**

```go
func TestStdlibDocs_FixtureFails(t *testing.T) {
	fsys := os.DirFS("testdata/stdlibdocs")
	got := stdlibDocProblems(fsys, nil, nil, bookHeadingExists)
	want := []string{
		"stdlib/Bad.alder:1: Bad: no doc comment",
		"stdlib/Bad.alder:3: Bad.undocumented: no doc comment",
		"stdlib/Bad.alder:5: Bad.fragment: the summary is not one line ending in a full stop",
		"stdlib/Bad.alder:5: Bad.fragment: no example",
		"stdlib/Bad.alder:7: Bad.noExample: no example",
		`stdlib/Bad.alder:22: Bad.badBook: Book: chapter 9, "No such heading": no such heading in the book`,
	}
	if !slices.Equal(got, want) {
		t.Fatalf("got:\n%s", strings.Join(got, "\n"))
	}
	// Review Focus 3: both wrong examples in one run, each at its own first line.
	fails := strings.Join(runExamples(t, stdlibExamples(fsys)), "\n")
	for _, w := range []string{"stdlib/Bad.alder:10: 1 + 1 == 3: want 3, got 2", "stdlib/Bad.alder:14: "} {
		if !strings.Contains(fails, w) {
			t.Errorf("missing %q in:\n%s", w, fails)
		}
	}
	if strings.Contains(fails, "Bad.alder:19") {
		t.Errorf("the true example failed:\n%s", fails)
	}
}

func TestStdlibDocs_Complete(t *testing.T) {
	for _, p := range stdlibDocProblems(stdlib.FS, pendingDocs, noExample, bookHeadingExists) {
		t.Error(p)
	}
}

func TestStdlibDocs_Examples(t *testing.T) {
	for _, f := range runExamples(t, stdlibExamples(stdlib.FS)) {
		t.Error(f)
	}
}

func TestBookHeadingExists(t *testing.T) {
	if !bookHeadingExists(`chapter 9, "List"`) || !bookHeadingExists(`chapter 6, "Notices and in-flight state"`) || bookHeadingExists(`chapter 9, "Nope"`) {
		t.Fatal("heading lookup")
	}
}
```

- [ ] **Step 3: Run them and see them fail**

Run: `go test ./book -run 'TestStdlibDocs|TestBookHeadingExists'`
Expected: FAIL, does not compile.

- [ ] **Step 4: Implement the helpers** in `book/stdlibdocs_test.go`, with `pendingDocs` holding
  all 27 module names. If a comment above the `module` line makes `ParseSignatures` or `ParseFile`
  report a diagnostic, stop and report it: the spec needs module docs and the parser must accept
  them.

- [ ] **Step 5: Run the tests**

Run: `go test ./book -run 'TestStdlibDocs|TestBookHeadingExists'`
Expected: PASS (`_Complete` passes because every module is pending).

- [ ] **Step 6: Commit**

```bash
git add book/stdlibdocs_test.go book/testdata/stdlibdocs && git commit -m "book: hold stdlib docs to the format and run their examples (#119)"
```

### Task 3: Document `List`, `Maybe`, `Result`

**Files:**
- Modify: `stdlib/List.alder`, `stdlib/Maybe.alder`, `stdlib/Result.alder`
- Modify: `book/stdlibdocs_test.go` (`pendingDocs`)

**Interfaces:**
- Consumes: the tests of Task 2; the procedure in "Writing a module's docs".
- Produces: the pattern the other modules copy. `List.take`'s doc is the spec's example block,
  word for word (later tasks' tests pin `List.take 2 [1, 2, 3] == [1, 2]` and
  `*Book: chapter 9, "List"*`). If one of its examples is false against `rt/std/list.go`, fix the
  example and the sentence it proves, and keep the first example.

- [ ] **Step 1:** Remove `List`, `Maybe`, `Result` from `pendingDocs`. Run
  `go test ./book -run TestStdlibDocs_Complete`. Expected: FAIL, one finding per undocumented
  declaration (49 declarations and 3 modules).
- [ ] **Step 2:** Write the docs, following the procedure.
- [ ] **Step 3:** Run `go test ./book -run TestStdlibDocs`. Expected: PASS. Run the signatures
  check from Global Constraints. Expected: no output.
- [ ] **Step 4:** Run `go test ./internal/check ./internal/format ./internal/semtok ./internal/search ./internal/fill ./internal/names ./internal/types ./tree-sitter-alder/...`.
  Expected: PASS (these read the stdlib files; `internal/format`'s corpus test runs the formatter
  over them, so an indented example line must survive formatting unchanged). If the formatter
  rewrites an example's indentation, stop and report it: the format depends on those four spaces.
- [ ] **Step 5: Commit**

```bash
git add stdlib book/stdlibdocs_test.go && git commit -m "stdlib: docs and examples for List, Maybe and Result (#119)"
```

### Task 4: Hover renders the doc

**Files:**
- Modify: `internal/lsp/hover.go` (`declDoc` at 230, `moduleHover` at 256, the callers of `withDoc`)
- Test: `internal/lsp/hover_test.go`

**Interfaces:**
- Consumes: `doc.Parse`, `Doc.Markdown` (Task 1); `List.take`'s doc (Task 3).
- Produces: `declDoc(a, ref)` returns rendered markdown; for a `syntax.RefStd` reference the
  fallback book line is the module doc's `Book`. `moduleHover` appends the module doc's markdown
  as its own paragraph. `func moduleDoc(mod *syntax.Module) doc.Doc` (the comment above the
  `module` line).

- [ ] **Step 1: Write the failing tests** in `hover_test.go`, using the existing `start`,
  `blogCopy`, `openClean`, `hover` and `c.at` helpers. Each writes a small module into the blog
  copy or edits one of its files before opening it.

  - `TestHover_StdDoc`: on `List.take` in an expression. The hover contains
    "```alder\nList.take 2 [1, 2, 3] == [1, 2]", contains `*Book: chapter 9, "List"*`, and the
    text `The first n elements of a list.` comes after the signature block.
  - `TestHover_StdModuleBook` (Review Focus 4): on a `List` function whose own doc has no `Book:`
    line. The hover contains `*Book: chapter 9, "List"*`.
  - `TestHover_AppDocNoBook` (Review Focus 4): the existing `lastPage` hover does not contain
    `Book:`.
  - `TestHover_HelperLineAboveDoc` (Review Focus 4): on a test helper that has a doc comment,
    `test helper (uses ` appears before the doc's text.
  - `TestHover_Qualifier`: extend it. The hover on the `List` qualifier still starts
    "`List` · stdlib/List.alder" and contains the module doc's summary.
  - `TestHover_AppExamples`: an app function with a doc holding an indented example line shows
    it in a fenced `alder` block.

- [ ] **Step 2:** Run `go test ./internal/lsp -run TestHover`. Expected: the new tests FAIL.
- [ ] **Step 3:** Implement. `TestHover_DeclWithDoc` must pass unchanged.
- [ ] **Step 4:** Run `go test ./internal/lsp -run TestHover`. Expected: PASS.
- [ ] **Step 5: Commit** `lsp: hover renders doc comments with examples and the book line (#119)`.

### Task 5: Completion carries the doc

**Files:**
- Modify: `internal/lsp/complete.go` (`declItems` 291, `valueItem` 316, `ctorItem` 329)
- Test: `internal/lsp/complete_test.go`

**Interfaces:**
- Consumes: `doc.Parse`, `Doc.Markdown`; `markdown(text) *markupContent` (complete.go:52).
- Produces: `func (cx *completion) docOf(mod *syntax.Module, line int) *markupContent`: the
  hover markdown of the declaration at that line without the signature, nil when there is no
  doc. Items built from a module's declarations (values, types, constructors; stdlib and app) set
  `Documentation` from it.

- [ ] **Step 1: Write the failing test** `TestComplete_StdDoc`: completing after `List.` in a
  blog module, the `take` item has `Detail == "take : Int -> List a -> List a"`, a non-nil
  `Documentation` of kind markdown whose value starts `The first n elements of a list.`, contains
  "```alder\n" and does not contain `take :`. An item for an undocumented app declaration has a
  nil `Documentation`.
- [ ] **Step 2:** Run `go test ./internal/lsp -run TestComplete`. Expected: FAIL.
- [ ] **Step 3:** Implement.
- [ ] **Step 4:** Run `go test ./internal/lsp -run TestComplete`. Expected: PASS.
- [ ] **Step 5: Measure.** In a throwaway test or with a logged byte count, record the JSON size
  of the completion response for an unqualified position in `examples/blog` on main and on this
  branch. Put both numbers in the PR body. Above about 200 KB: stop and report instead of
  committing.
- [ ] **Step 6: Commit** `lsp: completion items carry the doc (#119)`.

### Task 6: Search fields and `alder search --doc`

**Files:**
- Modify: `internal/search/index.go` (`Entry`, the `where` closure at 70)
- Modify: `internal/search/index_test.go:66` (the `Text.left` assertion)
- Modify: `cmd/alder/search.go` (flag parsing near 57, `searchRowJSON`, the table output near 127), `cmd/alder/helpjson.go` (the `search` flags)
- Modify: `internal/lsp/search.go` (the row struct near 40)
- Test: `internal/search/index_test.go`, `cmd/alder/search_test.go`, `internal/lsp/search_test.go`

**Interfaces:**
- Consumes: `doc.Parse`.
- Produces: `search.Entry` gains `Summary string`, `Examples []string`, `Book string` (`Doc`
  stays the raw text). The CLI JSON row and the `alder/search` row gain `summary`, `examples`
  (always an array), `book`. Usage becomes
  `usage: alder search <query> [--json] [--limit N] [--doc]`.

`--doc` text output: the table as today (aligned across all rows, the DOC column is the summary);
under each result's row, the doc's details, examples and book line, each line indented four
spaces, then a blank line. A result with no doc prints only its row.

- [ ] **Step 1: Write the failing tests**
  - `internal/search`: the `List.take` entry has `Summary == "The first n elements of a list."`,
    `Examples[0] == "List.take 2 [1, 2, 3] == [1, 2]"` and `Book == `chapter 9, "List"``. Change
    the `Text.left` assertion to `e.Doc != ""` (its wording changes in PR 2).
  - `cmd/alder`: `alder search List.take --limit 1 --doc` prints a row holding
    `The first n elements of a list.` and a later line equal to
    `    List.take 2 [1, 2, 3] == [1, 2]`. Without `--doc` no line starts with four spaces.
  - `cmd/alder` (Review Focus 5): `--json --doc` output is byte-equal to `--json` alone, and its
    first result has `"summary"`, `"examples"`, `"book"`. A query with no results and `--doc`
    prints nothing on stdout and exits 0.
  - `internal/lsp`: the `alder/search` row for `List.take` has the three fields.
- [ ] **Step 2:** Run `go test ./internal/search ./cmd/alder -run 'Search' ./internal/lsp -run 'Search'`
  (one `go test` per package). Expected: the new tests FAIL.
- [ ] **Step 3:** Implement.
- [ ] **Step 4:** Run the same commands. Expected: PASS. Run `go test ./cmd/alder -run 'Help'`.
  Expected: PASS (the help tree lists `--doc`).
- [ ] **Step 5: Commit** `search: summary, examples and book fields; alder search --doc (#119)`.

### Task 7: The fill prompt gets summaries only

**Files:**
- Modify: `internal/fill/prompt.go` (`stdFiles` 280–313, the comment at 269, `commentAbove` 89–103)
- Test: `internal/fill/prompt_test.go`

**Interfaces:**
- Consumes: `doc.Parse`.
- Produces: `func summarized(src string) string`: a stdlib file's source with every doc comment
  (a `--` run directly above a declaration or the `module` line) replaced by one
  `-- <summary>` line, or removed when it has no summary, and every other comment and the blank
  line after it removed. `stdFiles` embeds `summarized(src)`.

- [ ] **Step 1: Write the failing test** `TestSummarized`:

```go
src := "-- Lists.\n--\n-- Book: chapter 9, \"List\"\nmodule List\n\n-- Slices\n\n-- The first n elements of a list.\n--\n--     List.take 2 [1, 2, 3] == [1, 2]\ntake : Int -> List a -> List a\ndrop : Int -> List a -> List a\n"
want := "-- Lists.\nmodule List\n\n-- The first n elements of a list.\ntake : Int -> List a -> List a\ndrop : Int -> List a -> List a\n"
```

  and in the existing prompt test: the prompt for a hole contains
  `-- The first n elements of a list.\ntake : Int -> List a -> List a` and does not contain
  `List.take 2 [1, 2, 3]`.
- [ ] **Step 2:** Run `go test ./internal/fill`. Expected: FAIL.
- [ ] **Step 3:** Implement. Make `commentAbove` call `syntax.DocOf` if the change is a few lines;
  otherwise leave it.
- [ ] **Step 4:** Run `go test ./internal/fill`. Expected: PASS.
- [ ] **Step 5: Measure** the total bytes `stdFiles` produces for `coreStd` on main and on this
  branch (a logged number in the test is enough), put both in the PR body, and correct "together
  about 6 KB" at line 269 to the new figure.
- [ ] **Step 6: Commit** `fill: the prompt carries each stdlib summary, not the whole doc (#119)`.

### Task 8: The book, the changelog, PR 1

**Files:**
- Modify: `book/03-syntax.md` (`## Comments`, line 258), `book/11-tooling.md` (the hover,
  completion and `alder search` sections), `CHANGELOG.md`

- [ ] **Step 1:** In chapter 3's "Comments", add the doc comment convention: the four parts, the
  `List.take` block, and that the tools read it. In chapter 11, say what hover and completion now
  show and document `alder search --doc` with a transcript. Use the fence kinds the book tests
  understand (`book/snippets.go`, `Expectation`); a doc comment block above a signature is not a
  whole module, so check how `TestBook` classifies it and mark it `fragment` if it does not check.
- [ ] **Step 2:** Run `go test ./book`. Expected: PASS.
- [ ] **Step 3:** Add the CHANGELOG entry. Run `go test ./...`. Expected: PASS.
- [ ] **Step 4:** Commit, push the branch, open PR 1 with the two measurements (Tasks 5 and 7) in
  the body. After merge: `go install ./cmd/alder`, then ask the user to check in nvim and VS Code
  that hover on `List.take` shows the highlighted example block and that the completion popup
  shows the doc.

---

## PRs 2–4: the remaining modules

Each task follows "Writing a module's docs" for its modules and ends with the same checks.
Declaration counts are from the survey (`^type ` and `^name :` lines).

### Task 9 (PR 2): `Text` (43), `Int` (8), `Float` (25), `Bool` (1), `Dict` (20), `Bytes` (9)

**Files:** Modify `stdlib/{Text,Int,Float,Bool,Dict,Bytes}.alder`, `book/stdlibdocs_test.go`,
`CHANGELOG.md`.

- [ ] **Step 1:** Remove the six modules from `pendingDocs`; run
  `go test ./book -run TestStdlibDocs_Complete`; expected FAIL.
- [ ] **Step 2:** Write the docs. `Float` examples use values that are exact in binary
  (`0.5`, `2.25`) or go through the rounding functions.
- [ ] **Step 3:** Run `go test ./book -run TestStdlibDocs`, the signatures check, and
  `go test ./internal/check ./internal/format ./internal/semtok ./internal/search ./internal/fill ./internal/lsp ./tree-sitter-alder/...`.
  Expected: PASS, no output from the signatures check.
- [ ] **Step 4:** Commit per module (`stdlib: docs and examples for Text (#119)`), CHANGELOG, PR.

### Task 10 (PR 3): `Task` (6), `Query` (6), `Db` (25), `Session` (1), `Notice` (4), `Response` (8), `Html` (5), `Http` (3), `Url` (2), `Log` (4), `Prelude` (31)

**Files:** Modify those eleven `stdlib/*.alder` files, `book/stdlibdocs_test.go`, `CHANGELOG.md`.

Most examples here are usage examples (type-checked, not run). They may use the tables and
routes of `examples/todos`. For `Db`, keep each existing comment's SQL facts (`Db.updateMine`:
one `UPDATE` with `id = ? AND owner = ?`; no row changed is `Task.notFound`). `Notice.with`
says in its summary that it is how a route shows a success message, and its `Book:` line is
`chapter 6, "Notices and in-flight state"`. `Prelude` holds mostly types: each gets a summary and
needs no example.

- [ ] Steps 1–4 as in Task 9, for these modules.

### Task 11 (PR 4): `Date` (15), `Time` (10), `Random` (16), `Json` (10), `File` (9), `Test` (23), `Fuzz` (27)

**Files:** Modify those seven `stdlib/*.alder` files, `book/stdlibdocs_test.go`, `CHANGELOG.md`.

`Test.alder`'s group comments ("The browser steps (special forms…)", "Every step's result but
fetch's holds `url`…") are split per the procedure, step 4. Special forms that cannot be
type-checked in a `let` go in `noExample` with the reason. `Time.now` and `Date.today` get usage
examples; `Random` examples use a fixed seed.

- [ ] Steps 1–3 as in Task 9, for these modules.
- [ ] **Step 4:** `pendingDocs` is now empty: delete it and the `pending` parameter of
  `stdlibDocProblems`, and update `TestStdlibDocs_FixtureFails`'s call.
- [ ] **Step 5:** Run `go test ./book -run TestStdlibDocs`. Expected: PASS. Commit, CHANGELOG, PR.

---

## PR 5: operators and keywords

### Task 12: `internal/langdoc` and its hover

**Files:**
- Create: `internal/langdoc/langdoc.go`, `internal/langdoc/langdoc_test.go`
- Modify: `internal/lsp/hover.go` (`hoverText`, 64–133), `internal/lsp/hover_test.go`
- Modify: `book/stdlibdocs_test.go` (include the table), `book/11-tooling.md`, `CHANGELOG.md`

**Interfaces:**
- Consumes: `doc.Parse`, `doc.Problems`, `Doc.Markdown`; `syntax.Tokens`, `syntax.Keywords()`,
  `syntax.Kind.String()`; `stdlibExamples`/`runExamples` and `bookHeadingExists` (Task 2).
- Produces:

  ```go
  package langdoc

  type Entry struct {
      Token     string // "|>", "match", "<-", "?" for a typed hole
      Signature string // "(|>) : a -> (a -> b) -> b"; "" for a keyword
      Doc       string // the doc comment format, without the `-- ` prefixes
  }
  func Lookup(token string) (Entry, bool)
  func All() []Entry // in a fixed order: operators, then `<-` and `?`, then keywords
  // Markdown is the signature in a fenced block when there is one, then the rendered doc.
  func (e Entry) Markdown() string
  ```

Entries: `|>`, `??`, `&&`, `||`, `==`, `!=`, `<`, `<=`, `>`, `>=`, `+`, `-`, `*`, `/`, `%`, `++`,
`<-`, `?`, and every word of `syntax.Keywords()` except those in an `oldKeywords` map in the test
(`page`, `msg` if the checker now only rejects them, each with a reason). Facts come from
`internal/check/expr.go` (the `Op` switch near 1627) and book chapter 3 (`## Operators` line 396,
`## Match` 448, `## Blocks` 512), chapter 7 (`## Building effects`), chapter 11 (`## Holes`).
Each operator entry has a checked example; each entry has a `Book:` line.

Hover: when the token under the cursor is an operator, `<-` or a keyword, the hover is that
entry's `Markdown()`. On a typed hole it is today's `?name : T` block followed by the `?` entry's
rendered doc. Everything else is unchanged.

- [ ] **Step 1: Write the failing tests**
  - `langdoc.TestCovers`: lex `"== != < <= > >= + - * / % ++ && || ?? |> <-"` with
    `syntax.Tokens`; every token but the end markers has an entry. Every `syntax.Keywords()` word
    has an entry or is in `oldKeywords`. Every `syntax.Kind` from 0 up to `Invalid` is either
    produced by that sample string, or named in a `notDocumented` set of kind names in the test
    (identifiers, literals, brackets, punctuation, `Hole`, `Keyword`), so a token kind added to
    the lexer later fails until someone decides.
  - `langdoc.TestWellFormed`: every entry's `Doc` has no `doc.Problems` under
    `Strict{Book: true}`, and operator entries none under `Strict{Example: true, Book: true}`.
  - `book`: `TestStdlibDocs_Complete` also checks each entry's `Book:` reference with
    `bookHeadingExists`; `TestStdlibDocs_Examples` also runs each entry's examples, reported as
    `langdoc |>: …`.
  - `lsp.TestHover_Operator`: the cursor on `|>` in a blog module gives a hover containing
    "```alder\n(|>) : a -> (a -> b) -> b\n```" and `*Book: chapter 3, "Operators"*`.
  - `lsp.TestHover_Keyword`: on `match`, the hover contains `*Book: chapter 3, "Match"*`.
  - `lsp.TestHover_HoleDoc`: on a typed hole, the hover starts with the `?name : T` block and
    contains `alder fill`.
  - `lsp.TestHover_OperatorInString`: on `|>` inside a text literal or a comment there is no
    operator hover.
- [ ] **Step 2:** Run `go test ./internal/langdoc ./internal/lsp -run 'TestHover' ./book -run TestStdlibDocs`
  (one `go test` per package). Expected: FAIL.
- [ ] **Step 3:** Implement the table, then the hover.
- [ ] **Step 4:** Run the same commands. Expected: PASS.
- [ ] **Step 5:** Add a paragraph to chapter 11's hover section; CHANGELOG; `go test ./...`;
  commit; PR.

---

## PR 6: the generated reference

### Task 13: Appendix D (#227)

**Files:**
- Create: `internal/stdref/stdref.go`, `internal/stdref/stdref_test.go`
- Create: `book/D-stdlib.md` (generated)
- Modify: `book/appendix_test.go`, `book/README.md` (the appendix table near line 31),
  `book/09-stdlib.md` (the lead sentence), `book/13-compiler.md` (where hover or the search index
  is described), `CHANGELOG.md`

**Interfaces:**
- Consumes: `stdlib.FS`, `syntax.ParseSignatures`, `syntax.DocOf`, `doc.Parse`.
- Produces: `func stdref.Markdown() (string, error)`, the whole appendix.

Layout: a title `# Appendix D: Standard library reference` and a line saying the file is generated
from the comments in `stdlib/` and how to regenerate it. Then per module in alphabetical order
`## <Module>` with the module doc; per declaration in file order `### `<Module>.<name>`` (for
Prelude, the bare name), the signature in a fenced block, then summary, details and examples. The
examples go in fences marked `alder fragment`, so `TestBook` does not run each as its own
`alder eval`; Task 2's test already runs them. Book lines are left out (they point back into the
book).

- [ ] **Step 1: Write the failing tests**
  - `stdref.TestMarkdown`: the output contains "## List\n", "### `List.take`\n",
    "```alder fragment\nList.take 2 [1, 2, 3] == [1, 2]", and `## Dict` comes before `## List`.
  - `book.TestAppendixD_Current` in `appendix_test.go`: `D-stdlib.md` equals `stdref.Markdown()`;
    with `-update` (add `var update = flag.Bool("update", false, "rewrite book/D-stdlib.md")`)
    it writes the file. The failure message is
    `book/D-stdlib.md is stale: run go test ./book -run TestAppendixD_Current -update`.
- [ ] **Step 2:** Run `go test ./internal/stdref ./book -run TestAppendixD`. Expected: FAIL.
- [ ] **Step 3:** Implement `stdref`, then generate:
  `go test ./book -run TestAppendixD_Current -update`.
- [ ] **Step 4:** Add the row to `book/README.md`:
  `| [D. Standard library reference](D-stdlib.md) | Every standard library declaration with its description and examples, generated from the comments in `stdlib/` |`.
  Change chapter 9's "This is the complete list; a function not listed here does not exist." to
  say the complete list is appendix D and this chapter is the guide. In chapter 13 add the doc
  parser, the stdlib doc tests and the generated appendix where the chapter covers hover or
  search.
- [ ] **Step 5:** Run `go test ./book` and `python3 book/site/test_site.py`. Expected: PASS, and
  the built site has `D-stdlib.html`. If `TestBook` takes noticeably longer than on main because
  of the appendix's signature blocks, mark those `fragment` too and say so in the PR.
- [ ] **Step 6:** CHANGELOG; `go test ./...`; commit; PR with `Closes #119` and `Closes #227`.

### Task 14: Close out

- [ ] File the issues the spec defers: doctests for app code in `alder test`; searching doc text
  in `alder search`; an experiment on putting examples in the fill prompt. File one issue per
  behaviour found wrong while documenting, if not already filed.
- [ ] Comment on #228 with what is done (the fields, `--doc`) and what is left (the pickers in
  alder.nvim and alder-vscode).
- [ ] Tick item 8 in roadmap #52 and add the new issues there.
- [ ] `go install ./cmd/alder`.
