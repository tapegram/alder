# Standard library docs: hover, completion, search, book

Design for #119, with #227 and the format half of #228. Written 2026-11-15. The user skipped the
design questions and gave the intent below; every choice under "Decided" is **my call** unless it
is marked as the user's. Read the "Calls to check" list at the end first.

## Intent

The user's words: "I just want to be able to get full context and descriptions and maybe even
example usage for all standard lib functions", done the way Elm, Haskell and other languages with
a good developer experience do it.

Today `K` on most standard library functions shows a signature and nothing else. Hover already
shows a declaration's comment (`hoverReq` → `declDoc` → `syntax.DocOf`), but only 89 of the 360
declarations in `stdlib/*.alder` have one, none has an example, and twelve of the 27 files have no
comment at all. `Notice.with`, the answer to "how do I show a success toast", is one of the
undocumented ones.

Success: on any standard library name, operator or keyword, the editor shows what it does, how it
behaves at the edges and a working example, without the user opening the book or the stdlib file.
An agent gets the same text from `alder search`.

## What the other languages do

| Language | Where docs live | Examples | Gaps | Reference |
|---|---|---|---|---|
| Elm | `{-\| -}` comment above the declaration, markdown | indented `expr == value` lines (checked by the third-party elm-verify-examples) | a package with an undocumented exposed value cannot be published | package site generated from the comments |
| Haskell | Haddock `-- \|` comment | `>>> expr` lines, checked by doctest | none enforced | Haddock pages; HLS shows the comment on hover |
| Rust | `///` comment, markdown | fenced blocks, run by `cargo test` | `missing_docs` lint | rustdoc; keywords have doc pages and rust-analyzer shows them on hover |
| Elixir | `@doc` string | `iex>` lines, run as doctests | none enforced | ExDoc; `h List.first` in the shell |
| Go | comment above the declaration | `Example` functions, run by `go test` | linters | pkg.go.dev; gopls hover |

What they share, and what this design takes:

1. The comment in the source is the only copy. Hover, the shell, and the reference pages are
   generated from it.
2. Examples are code that a test runs, so they cannot go stale.
3. A missing doc is a failure, not a style note.
4. The guide (a book) and the reference (generated) are separate documents.

## Goals

- Every declaration in `stdlib/*.alder`, and every module, has a doc comment: a summary, the edge
  cases, and for every function at least one example.
- Every example is checked by a test.
- A test fails when a stdlib declaration has no doc, or when a doc breaks the format.
- Hover, completion, `alder search` and the `alder fill --ai` prompt all read that one comment.
- Hover on operators and keywords shows a description and an example.
- The book gets a complete, generated standard library reference (#227), so nothing is written
  twice.
- App code gets the same rendering: a comment in the user's own module written in this format
  shows the same way on hover.

## Non-goals

- Doctests for app code (`alder test` running the examples in the user's comments). File an issue.
- Searching doc text in `alder search`. File an issue.
- Clickable links to the book. The published book is reachable only by repo members today; hover
  names the chapter and section in prose. Links come with the website (#209).
- Links to recipes (asked for on #119). There are no recipes until #161.
- Showing examples and book sections in the alder.nvim and alder-vscode search pickers. That stays
  in #228; this design gives it the fields.
- `completionItem/resolve`, signature help, docs on record fields or locals.
- New comment syntax. A doc comment stays an ordinary `--` comment.
- Any change to a stdlib signature or to what a function does.

## Anti-goals (what would make this a failure)

- A doc that restates the signature ("takes an Int and a List and returns a List").
- A doc that describes what a function should do, not what the code in `rt/` does.
- An example nobody ran.
- The same prose kept by hand in a comment and in the book.
- The `alder fill --ai` prompt growing several times over because every stdlib file now carries
  examples.
- The test suite getting much slower: `alder eval` builds a binary per run (#54), so one process
  per example is out.

## Constraints

- `syntax.DocOf` (`internal/syntax/walk.go:92`) defines a doc comment: the run of `--` lines
  directly above the declaration, with `--` and one space stripped. Text after that one space is
  kept, so indented example lines survive. A blank line ends the run.
- The stdlib files are signature-only (`module`, `type`, `name : Type`), embedded by
  `stdlib/embed.go` and read by `types.LoadStd`, the search index and the fill prompt.
- Operators are built into the checker (`internal/check/expr.go`), not declared in
  `stdlib/Prelude.alder`. Keywords are `syntax.keywords` (`internal/syntax/token.go:72`).
- Hover returns markdown and passes the doc through unescaped.
- The tree-sitter grammar, the formatter and semantic tokens must need no change.
- Tests that pin stdlib comment text today: `internal/search/index_test.go:66` (`Text.left`).
  `internal/fill/prompt.go:269` states an "about 6 KB" budget for the core modules.

## Decided

### The doc comment format

```alder
-- The first n elements of a list.
--
-- A list shorter than n comes back whole. An n of zero or less gives the empty list.
--
--     List.take 2 [1, 2, 3] == [1, 2]
--     List.take 5 [1, 2] == [1, 2]
--     List.take 0 [1, 2] == []
--
-- Book: chapter 9, "List"
take : Int -> List a -> List a
```

Paragraphs are separated by a bare `--` line. In order:

1. **Summary** (required). One sentence on one line, ending in a full stop. It says what the
   function gives back or does, for a reader who has never seen it. It does not repeat the name
   or the type. `alder search` and completion lists show this line alone, so it must stand alone.
2. **Details** (optional). Plain paragraphs, markdown inline code allowed. Edge cases (empty
   input, `Nothing`, negative or out-of-range numbers), what fails and how, anything surprising
   about cost or order, and how it differs from a sibling.
3. **Examples** (required for a function, optional for a type). Lines indented four spaces after
   the `-- `. A line indented exactly four spaces starts an example; a deeper line continues it.
   Names are qualified as a caller writes them (`List.take`, not `take`). Two kinds:
   - **Checked equality**: an expression whose top level is `==`. The test evaluates it and it
     must be `True`.
   - **Usage**: any other expression. The test type-checks it and does not run it. This is the
     form for effects (`Task`, `Query`, `Db`, `Test`, `Log`) and for values with no equality.
4. **Book** (optional on a declaration, required on a module). A last paragraph of the form
   `Book: chapter 9, "List"` or `Book: appendix B, "Heading"`. The quoted text is the exact text
   of a heading in that chapter. It points at the narrative chapter that teaches the function,
   never at the generated reference.

A module's doc is the comment above its `module` line: a summary of what the module is for,
optional details, and a `Book:` line.

Rules that follow:

- For a pure function, every edge case the details state has a checked example that shows it.
- A comment directly above a declaration is that declaration's doc. A section header
  (`-- Scoped operations: …`) is followed by a blank line. Group comments that sit directly above
  the first declaration of a group today (`Test.alder`) are moved up and their facts are written
  into each declaration they cover.
- Examples are checked in a copy of `examples/todos`, so an example may name that app's tables and
  routes, as the book's snippets do.

### Reading the format

One parser, in one place, turns `DocOf`'s text into `{Summary, Details, Examples, Book}`. Hover,
completion, search, the fill prompt, the tests and the reference generator all use it.

It is lenient. A comment that does not follow the format (any app comment written before this) has
its first paragraph as the summary and the rest as details, with no error. Strictness lives in the
stdlib test, not in the parser.

`internal/fill/prompt.go`'s `commentAbove` duplicates `DocOf`; fold it in if that is cheap.

### Hover

For a declaration, in order: the signature block as today; the summary; the details; the examples
in one fenced `alder` block (so the editor highlights them); the book line in italics. A stdlib
declaration with no `Book:` line of its own shows its module's.

````markdown
```alder
take : Int -> List a -> List a
```
The first n elements of a list.

A list shorter than n comes back whole. An n of zero or less gives the empty list.

```alder
List.take 2 [1, 2, 3] == [1, 2]
List.take 5 [1, 2] == [1, 2]
List.take 0 [1, 2] == []
```
*Book: chapter 9, "List"*
````

- A module name or qualifier: today's line (`` `Text` · stdlib/Text.alder · N declarations ``),
  then the module's doc.
- App declarations: the same rendering of the user's own comment.
- Types and constructors: unchanged signature, then the doc rendered the same way.

### Operators and keywords

A table in Go, in a small package of its own so completion and search can use it later. One entry
per operator and per keyword: the token, a signature line where one makes sense
(`(|>) : a -> (a -> b) -> b`), a summary, optional details, examples and a book reference. The
entries use the same four parts as a doc comment and go through the same parser, tests and
renderer.

- Covered: every operator the checker handles (`|>`, `??`, `&&`, `||`, `==`, `!=`, `++`, the
  comparison and arithmetic operators), every word in `syntax.keywords`, `<-` in an effect block,
  and the typed hole `?name` (which keeps its `?name : T` block and gains the description and a
  pointer to `alder fill`).
- Hover on one of these tokens shows the entry. Hover elsewhere in the expression is unchanged.
- A test fails when an operator or keyword has no entry. A keyword kept only for an old form
  (`page`, `msg`) may be excluded in that test with a reason.

### Completion

Value, type and constructor items, for the stdlib and for the app, set `documentation` to the
hover markdown without the signature block (`detail` already carries the signature). It is sent
inline; `completionItem/resolve` is not added. Measure the size of an unqualified completion
response in `examples/blog` before and after, and put both numbers in the PR. If it passes about
200 KB, stop and raise it instead of shipping.

### Search

- The index entry gains `Summary`, `Examples` and `Book`; `Doc` stays the full raw text.
- `alder search --json` and the `alder/search` request gain `summary`, `examples` and `book`.
  This is the format decision #228 was waiting for; the editor pickers stay in #228.
- Text output keeps one line per result and shows the summary in the fourth column.
- New flag `alder search <query> --doc`: under each result, the full doc (details, examples, book
  line), indented. This is how an agent reads the docs without an editor.
- Operators and keywords are not added to the index in this work.

### The `alder fill --ai` prompt

The prompt keeps embedding the stdlib modules it embeds today, but rendered: each declaration as
one `-- <summary>` line and its signature. Details, examples, book lines and section headers are
left out. Measure the core modules' size before and after, put both in the PR, and correct the
"about 6 KB" comment. Whether examples in the prompt make fills better is a separate experiment;
file an issue.

### Tests that enforce it

1. **Complete and well-formed.** For every stdlib file: the module has a doc with a `Book:` line;
   every `type` and every value has a doc; the summary is one line ending in a full stop; every
   value has at least one example or is on a waiver list (name and reason, in the test, expected
   to stay near empty: a special form that cannot be type-checked on its own); every `Book:`
   reference names a chapter file and a heading that exist in `book/`.
2. **Examples.** Every example in the stdlib comments and in the operator table is checked in a
   copy of `examples/todos`. Checked equalities are evaluated in one batch (one build, not one per
   example); usage examples are type-checked in one batch. A failure names the source line
   (`stdlib/List.alder:14`), and for a false equality shows the value the left side produced.
3. **Rollout allowlist.** During the rollout the first test skips modules on a list of
   not-yet-documented modules. Each batch removes its modules. The last PR deletes the list.
4. **Generated reference is current** (below).

All of these run in ordinary `go test`, in the CI group the book tests are in.

### Writing the docs

Sources, in this order of authority:

1. The Go implementation in `rt/` (bound by name in `internal/gen/stdcall.go`). It decides what is
   true.
2. The existing comment. Keep every fact in it (`Text.hash`'s FNV-1a note, `Db.updateMine`'s
   single `UPDATE`).
3. The book, chiefly `book/09-stdlib.md`, for wording that already exists.

House style: full sentences, plain words, written for a beginner. This replaces the terse
lowercase fragments the 89 existing comments use; they are rewritten to the format.

Document what the code does. Where the behaviour looks wrong (a crash on a negative count, an
off-by-one), document it as it is, file an issue, and list the issues in the PR body.

### The generated reference (#227)

A new appendix, `book/D-stdlib.md`, "Appendix D: Standard library reference", generated from the
stdlib comments and checked in. One section per module in alphabetical order with the module's
doc, then each declaration in file order: a heading with the qualified name, the signature, the
summary, details and examples. A test generates it in memory and fails when the checked-in file
differs; the failure says which command rewrites it.

- The examples are already checked by test 2. Mark the appendix's example fences so `TestBook`
  does not run each one as its own `alder eval`.
- Chapter 9 stays a hand-written guide, grouped by topic. Its sentence "This is the complete list;
  a function not listed here does not exist" now sends the reader to appendix D for the complete
  list. Its signature blocks stay type-checked against the stdlib as today.
- Add the appendix to the table of contents in `book/README.md` and check the site build
  (`book/site/build.py`) picks it up.

This closes #227 in a different shape from its title: the reference is generated from the comments
into an appendix, and chapter 9 is not generated.

### Book and changelog

- Chapter 3, "Comments": the doc comment convention, with the example above.
- Chapter 11 (tooling): what hover and completion show, `alder search --doc`.
- Chapter 13 (compiler), if it describes hover or the search index: the parser and the tests.
- A CHANGELOG entry.

## Rollout

Each PR goes to main and leaves it green.

1. **Format and plumbing.** The parser, hover and completion rendering, search fields and
   `--doc`, the fill prompt rendering, tests 1–3 with the allowlist, and three modules documented
   as the pattern for the rest: `List`, `Maybe`, `Result`. Book chapters 3 and 11.
2. **Core values.** `Text`, `Int`, `Float`, `Bool`, `Dict`, `Bytes`.
3. **App and effects.** `Task`, `Query`, `Db`, `Session`, `Notice`, `Response`, `Html`, `Http`,
   `Url`, `Log`, `Prelude`.
4. **The rest.** `Date`, `Time`, `Random`, `Json`, `File`, `Test`, `Fuzz`. The allowlist is
   deleted.
5. **Operators and keywords.**
6. **Appendix D** and the chapter 9 change. Closes #119 and #227; comment on #228 with what is
   left.

After each merge: reinstall `alder`. No change is expected in alder.nvim or alder-vscode. After
PR 1, check by hand in both editors that hover shows the example block highlighted and that the
completion popup shows the doc.

## Left to the builder

- Package and function names, test names, the generator's entry point.
- How the example test batches: one `alder eval -` input with a `let` per example and a list as
  the last expression, or a generated test module run by `alder test`. Whichever reports a failure
  against the stdlib line most clearly.
- The exact list of operator and keyword entries, and which old keywords are excluded.
- Whether section headers in a stdlib file become sub-headings in appendix D.
- Whether appendix D's signature blocks stay checked by `TestBook` or are marked as fragments, by
  what it costs in test time.
- The wording of every doc, within the format and the house style.

## Calls to check

Chosen while writing; none was discussed.

1. Examples are Elm-style `expr == value` lines, not Elixir `iex>` or Haskell `>>>` pairs.
2. Every function must have an example. Effects get a type-checked usage example, not a run one.
3. The house style changes from lowercase fragments to full sentences for a beginner.
4. Book pointers are prose (`Book: chapter 9, "List"`), required per module and optional per
   declaration, and checked against the book's headings. No URLs yet.
5. #227 is done as a generated appendix D; chapter 9 stays hand-written.
6. The fill prompt gets summaries only, so it does not grow with the examples.
7. `alder search --doc` is added for agents; the search JSON gains `summary`, `examples`, `book`.
8. Completion carries the doc inline, with no resolve request.
9. Operators and keywords get a Go table, since they are not declared in any `.alder` file.
10. Doctests for app code, doc-text search and clickable book links are left out and get issues.
