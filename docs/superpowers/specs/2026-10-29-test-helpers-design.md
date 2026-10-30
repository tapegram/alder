# Test helpers: values that only tests may use

Design for [tapegram/alder#4](https://github.com/tapegram/alder/issues/4). Found by Claude while
building nightvote-replay: `Test.fetch`, `Test.fake` and the rest of `Test` are legal only directly
inside a `test`, so every file-page test repeats the same prologue (fake, fake, fetch, `Bytes.toText
r.body ?? ""`), nine times in `Sites.MnSos` alone, and the dev log adds that `cfgOf`, `kent` and
`header` exist three times because test support cannot be shared between modules. The user was not
available for this design; everything marked **(call)** is the author's decision, not the user's.

## Intent

A test's prologue becomes one ordinary top-level value, written once, in any module, and called
from any test:

```alder
fetchText : Text -> Task { status: Int, body: Text }
fetchText cfg = {
  let _ <- Test.fake Gate
  let _ <- Test.fake MnSos
  let r <- Test.fetch (Sites.MnSos { cfg: cfg })
  Task.succeed { status: r.status, body: Bytes.toText r.body ?? "" }
}

test "at minute 400 the file is the final" = {
  let r <- fetchText (at 400 "")
  Task.succeed (r.status == 200 && Text.contains "final" r.body)
}
```

`fetchText` is a *test helper*: a value the checker knows only tests may use, that no production
build contains and no mutant is made in. The term is prose, not a keyword.

## Goals

1. A value may use everything a `test` body may use; it then becomes a test helper.
2. A test helper is usable from any `test` and any other test helper, in any module.
3. Production code that reaches a test helper is a checker error naming the helper and why it is one.
4. `alder build` emits no test helper; `--mutate` makes no mutant in one.
5. No new syntax: parser, formatter, tree-sitter, the grammar appendix and the LSP's syntax paths
   are untouched.

## Non-goals

- A `test helper` declaration keyword, a `Test` effect or capability type, or a test-module/file
  convention. See "The rule" for why each is bigger than needed.
- Sharing `fake` declarations or a fake's tables across modules (E0020 stands).
- A coverage tool. Alder has none today, so there is nothing to exclude; when one comes, a test
  helper counts as test code.
- Changing the shape rules of the browser steps: `Test.fetch (Sites.MnSos { cfg: cfg })` is still a
  page written in place with the helper's parameters inside it; `Test.fetch p` with `p` a parameter
  is refused as today.

## Anti-goals

- A production build that imports `alder/rt/testing` or carries a test node. gen already panics when
  one reaches the emitter (`testNode`); this design keeps that invariant.
- A mutation score that counts test support. The dev log's complaint about fakes (survivors in
  `fakeMeta`, `mask`) is a separate issue; helpers must not add to it.
- Silent demotion: a value becoming a test helper must be visible at the first production use, with
  the reason, so a model reading one error knows the fix.

## The rule

**A top-level value is a test helper when its signature or body uses something only a test may
use** **(call)**. The inducing things are exactly the four the checker gates on `inTest` today:

- a reference to `Test.*` (the steps, `Test.run`, `Test.trace`, `Test.fake`, `Test.login`,
  `Test.member`, and the pure `Doc` queries `Test.text`, `Test.has`, …);
- the Prelude's test types `Doc` and `Answer` (in a signature, or an `Answer` constructor in a body);
- a fake's table (E0021's subject);
- another test helper.

Not inducing **(call)**: a brand constructor used outside its module (keeps its own rule, since a
value silently becoming test-only through a brand would surprise), scoped `Db` operations (E0026
stands; a helper that needs `Db.mine` lives in a `requires user` page as today), and `Fuzzer` values
(§0a of the property-testing design: generators are ordinary values).

**Who may use a test helper**: a `test` body, or another test helper, in any module **(call)**.
Cross-module use needs no new visibility rule; everything a module defines is visible already.

**What can never be a helper**: a value the compiler wires by name (a page module's `load`, `view`,
`update` and the other names chapter 6 lists; `App`'s values), a fake's entries, a brand's `where`
check and a table field's default **(call)**. Using an inducing thing in one of those is an error at
the use, as today; a fake entry calling `Test.fetch` would run a browser step inside a capability
call, and nothing else on the list runs under a test.

Why this and not the alternatives **(call)**: a `test helper` keyword is a fourth declaration form to
parse, print, highlight and complete for something the checker can already see; a `Test` effect in the
type is the same information written twice (the body already says `Test.fetch`); a test
module/file convention conflicts with Alder's one-file-per-module layout and with the dev log's ask to
share helpers between modules. Inference is the smallest rule, and it is safe because every inducing
thing is already fenced by `inTest`: this design moves the fence from "the `test` keyword" to "a value
only tests reach", which the checker proves transitively and gen enforces by omission.

## The diagnostic

New code **E0040 `TestOnly`** **(call)**, replacing the three E0003 texts "`Test.%s` is only available
inside a `test`" (`expr.go`, `browser.go`, `fakes.go`). Two texts:

- At a reference to a helper from code that may not use it:
  ``fetchText` is a test helper (it uses `Test.fetch`): only a `test` or another test helper may use it``.
  The reason is the helper's first inducing thing, so a chain reads
  ``at400` is a test helper (it uses `fetchText`, a test helper)``.
- At an inducing thing inside a value that can never be a helper:
  ``Test.fetch` is only available in a `test` or a test helper: `view` is the page's view, not a helper``
  (likewise "a fake's entry is not a helper", "a field default is not a helper", "a brand check is not
  a helper", "`App.session` is not a helper").

Reported once per site. E0003 "`Doc` is a test type" in a signature becomes E0040 only when the
value is one that can never be a helper; in a plain value, `Doc` now just makes it a helper.

## How the checker does it

Mirrors `scopedDirect`/`scopedEdges`/`propagateScoped` (rules.go):

1. While checking a plain value's body (`mc.value != ""`, not `inTest`, not a fake), an inducing
   thing records `testDirect[key] = reason` instead of erroring; a reference to another top-level
   value records an edge `{from, to, span}` (every module, not only same-module: a dependency's
   helper is known already, but recording all edges keeps one report pass).
2. In a never-a-helper context, an inducing thing errors at once (second text).
3. After all modules' bodies: a fixpoint over the edges marks every value that reaches a helper,
   keeping the first reason. Then one pass over the edges reports E0040 (first text) at each edge
   whose `to` is a helper and whose `from` is not (a wired value, or a plain value the fixpoint did
   not mark; the latter cannot happen, so the report is effectively for wired values). Edges from
   tests are not recorded.
4. `Result.TestOnly map[string]string` ("Module.name" → reason) is exported for lower and the LSP.

Eval and the REPL: the scratch module is a test build, and `infersTopLevel` keeps working; a scratch
value using `Test.fetch` becomes a helper there too, which is what a user poking at a page wants.

## Builds, mutation, tooling

- **IR**: `ir.Value.TestOnly bool`, from `Result.TestOnly`. Lowering is unchanged (test nodes are
  lowered as in a test body).
- **gen**: `module` skips a `TestOnly` value unless `g.tests`, next to where fakes and tests are
  gated. A production build therefore never sees the value, so `testNode`'s panic stays a bug check,
  and nothing emitted can reference the skipped value: the checker refused every production use.
- **Mutation**: `mutate.Collect` skips `v.TestOnly` on the same line as `isFuzzer`. The score's
  denominator shrinks by exactly the helpers' nodes; the dev-log fake survivors are unaffected.
- **Formatter, parser, tree-sitter, `book/A-grammar.md`**: no change.
- **LSP** **(call)**: hover on a helper or a reference to one adds a line "test helper (uses
  `Test.fetch`)"; E0040 surfaces like any diagnostic. Completion is unchanged (helpers complete as
  values; the error says why when misused).
- **Scaffolding** **(call)**: `alder new page --file` keeps generating its inline `Test.fetch` test;
  a helper is the user's refactor once a second test appears.

## Where things change

| layer | files |
|---|---|
| code | `internal/diag/diag.go`: `TestOnly = "E0040"`; `internal/diag/explain.go` entry |
| checker | `internal/check/check.go` (fields, the post-bodies pass), `expr.go` (the `Test.*`, `Answer` and value-reference sites), `browser.go`, `fakes.go` (E0021 becomes inducing in a plain value), `result.go`; tests in `check_test.go` |
| IR / gen | `internal/ir/ir.go`, `lower.go` (copy the flag); `internal/gen/gen.go` module emission; a golden under `internal/gen/testdata` for both build kinds |
| mutation | `internal/mutate/mutate.go` `Collect` |
| LSP | `internal/lsp` hover |
| book | `book/10-testing.md`: new "### Test helpers" after section 9's "The rules" (the example above, the four inducing things, the never-a-helper list, the error text); section 11 "What is never a mutant" adds "a test helper"; `book/09-stdlib.md` "exist only in tests" → "only in tests and test helpers"; `book/13-compiler.md` one sentence where the production build is said to carry no test code; `book/B-pitfalls.md` if it lists the old limit |
| site | `book/site/build.py` after the book edit |

Also: tick #4 in roadmap #52; the user reinstalls (`go install ./cmd/alder`) after merge.

## Left to the builder

- The exact never-a-helper name list, read from `pages.go` (where page values are wired by name).
- Whether the edge list reuses `scopedEdges` with spans added or is its own slice.
- Book wording; keep the example verbatim so the site's reference pages show it.

## Verification

- `go test ./...` green, with new checker cases: a helper accepted; a production value using it
  refused with E0040 and the reason; a two-step chain; a page `view`, a fake entry and a field default
  each refused at the inducing thing; cross-module use from a test accepted; a `Doc -> Text` helper.
- A gen golden: the helper is absent from the production module and present in the test module.
- `alder test --mutate` on the golden app: zero mutants in the helper.
- A scratch copy of nightvote-replay's `Sites.MnSos` with the nine prologues replaced by `fetchText`:
  `alder test` passes with the same test names, `alder build` succeeds, and the mutation score does
  not fall. One-off; nightvote-replay pins its own Alder build.
