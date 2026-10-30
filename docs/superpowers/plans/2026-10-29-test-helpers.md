# Test helpers (#4): implementation plan

Spec: `docs/superpowers/specs/2026-10-29-test-helpers-design.md` (binding). TDD: each task starts
with a failing test, then the code, then `go test` of the package, then a commit.

## Task 1: E0040 `TestOnly`

- `internal/diag/diag.go`: `TestOnly = "E0040"`; `internal/diag/explain.go`: the entry (what a
  test helper is, the four inducing things, the never-a-helper list, the fix).
- Test: `internal/diag/diag_test.go`'s code list gains `E0040` (fails until explain has it).

## Task 2: the checker infers test helpers

`internal/check`:

- `check.go`: `testDirect map[string]string` (value key → first inducing thing), `testEdges
  []testEdge{from, never, to string; span}`, `testOnly` (the fixpoint), `testReported`; `modCtx.never`
  (why the code being checked is never a helper: a brand check, a field default). After pass 4,
  `c.markTestHelpers()` runs the fixpoint and reports E0040 at each edge into a helper from a
  never-a-helper context.
- `helpers.go` (new): `helperBan()` (the reason the context is never a helper: a fake's entry, a
  page's `load`/`view`/`update`, any value of `App`, `mc.never`); `testUse(what, span) bool`
  (in a test: ok; in a plain value: record `testDirect`, ok; else E0040 second text, once per
  site); `helperRef(module, name, span)` (records the edge; none from tests); the fixpoint and the
  report (first text, reason chained: "it uses `fetchText`, a test helper").
- `expr.go`: the `Test.*` reference, `Test.*` application (`checkSelectorArg`), and `Answer`
  constructor sites call `testUse` instead of E0003; value references call `helperRef`;
  brand checks set `mc.never`. `browser.go`: `inferTestStep`, `testTypeUse` (a value's signature:
  `declareValues` sets `mc.value`); `fakes.go`: `inferTestFake`, `fakeTableUse` (E0021 inducing in a
  plain value; elsewhere E0021 as today). `html.go`: a component element calls `helperRef`.
  `decls.go`: field defaults set `mc.never`.
- `result.go`: `Result.TestOnly map[string]string`.
- Tests (`internal/check/check_test.go`, `TestTestHelpers`): helper accepted; production value
  using it refused with E0040 and the reason; a two-step chain (`at400` → `fetchText`); a page
  `view` using `Test.fetch` refused at the use; a fake entry and a field default refused at the
  inducing thing; cross-module use from a test accepted; a `Doc -> Text` helper; `Answer`
  constructor and fake table in a helper; `Result.TestOnly` contents. `TestTestBodies`' "outside"
  cases become accepted helpers.
- Corpus: `E0003-test-run-outside`, `E0003-test-fake-outside-test`, `E0003-doc-outside-test` are
  rewritten as E0040 cases (renamed `E0040-…`): the inducing thing in a wired value, a fake entry,
  a page's `view` reaching a helper. New `test-helpers-ok` (a helper in one module used by tests
  in another).

## Task 3: IR flag and gen omission

- `internal/ir/ir.go`: `Value.TestOnly bool`; `lower.go` copies it from `Result.TestOnly`.
- `internal/gen/gen.go`: `module` skips a `TestOnly` value (and component) unless `g.tests`.
- Golden `testdata/golden/gen_helpers` (in `testBuildGoldens`): `want/` (test build) has the
  helper, `want_prod/` has not. Test: `TestGenGolden` (regenerate with `-update`, read the diff).

## Task 4: mutation

- `internal/mutate/mutate.go` `Collect`: skip `v.TestOnly` beside `isFuzzer`.
- Test (`internal/mutate`): Collect on an app with a helper makes no mutant in it.

## Task 5: LSP hover

- `internal/lsp/hover.go`: hover on a helper's name or a reference to one adds
  "test helper (uses `Test.fetch`)".
- Test: `internal/lsp/hover_test.go` case.

## Task 6: book

- `book/10-testing.md`: "### Test helpers" after section 9's "The rules" (the spec's example
  verbatim, the four inducing things, the never-a-helper list, the error text); section 11 "What
  is never a mutant" adds a test helper. `book/09-stdlib.md`: "only in tests and test helpers".
  `book/13-compiler.md`: one sentence. `book/B-pitfalls.md` if it lists the old limit.
- `python3 book/site/build.py`; `cd experiments && python3 -m unittest`.

## Task 7: verification

- `go vet ./...`, `go test ./...`.
- One-off: a scratch copy of nightvote-replay's `Sites.MnSos` with the prologues replaced by a
  `fetchText` helper: `alder test` passes with the same names, `alder build` succeeds, the mutation
  score does not fall.
