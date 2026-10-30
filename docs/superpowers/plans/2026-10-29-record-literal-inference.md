# Record literal inference (#3): implementation plan

Spec: `docs/superpowers/specs/2026-10-29-record-literal-inference-design.md` (binding). TDD per task:
write the failing test, run it red, implement, run green, commit.

## Task 1: the handle in the unifier

Files: `internal/types/types.go`, `internal/types/unify.go`, new `internal/types/record_flex_test.go`.

Tests (all in `record_flex_test.go`, a literal is `&Record{Anonymous: true, Fields: …, Flex: u.Fresh()}`):

- `TestRecordFlex_BindsNominal`: literal ~ nominal `Curve` succeeds; `Apply(lit)` is `Curve`.
- `TestRecordFlex_Symmetric`: nominal on the left, literal on the right: same result.
- `TestRecordFlex_FieldOrderIrrelevant`: literal fields in another order still bind.
- `TestRecordFlex_DeclinesOnNames`: extra or missing field: error, handle unbound, `Apply(lit)` anonymous.
- `TestRecordFlex_DeclinesOnFieldType`: `median: Int` vs `Float`: error, handle unbound.
- `TestRecordFlex_CheckAndUnify`: both `Check` and `Unify` bind.
- `TestRecordFlex_LinksTwoLiterals`: two undecided literals unify, then one meets `Curve`: both are `Curve`.
- `TestRecordFlex_DecidesStructural`: literal ~ declared structural record (no handle): bound to it;
  a later `Curve` fails and `Apply(lit)` stays anonymous without a handle.
- `TestRecordFlex_GenericNominal`: `{ item: 1 }` ~ `Box a` (instance with a fresh var arg): binds,
  `Apply` is `Box Int`.
- `TestRecordFlex_CarriedBySubstAndInstantiate`: `Apply`/`Instantiate` copies keep `Flex`.
- `TestRecordFlex_SelfUnify`: a literal unified with itself does not bind its handle to itself.

Code:

- `Record.Flex *Var` with a doc comment.
- `Apply`: anonymous with bound handle → `Apply(Subst[Flex.ID])`; else the copy carries `Flex`.
- `Instantiate` and `subst` copies carry `Flex`.
- `unify` `Record` case: one side anonymous with a handle, the other nominal → `bindFlex` (name sets
  equal, fieldwise unify by name, then bind). Anonymous/anonymous: today's check, then bind a handle
  (both handles: bind a's to b unless same ID; one handle: bind it to the other).

## Task 2: the checker gives literals a handle

Files: `internal/check/expr.go` (`inferExpr` `RecordLit`), new `internal/check/record_infer_test.go`,
corpus `testdata/corpus/record-literal-inferred-ok/` (`app/Curve.alder`, `expected.diag` empty).

Tests (`record_infer_test.go`, via `checkModule`, each must check clean):

- `TestRecordLitInference_Shapes` subtests:
  - `eq`: `r == Ok { … }` with `r : Result Text Summary`.
  - `pipeline`: `rows |> List.map (\r -> { … })` in a function returning `List Shown`.
  - `chain`: `rows |> List.map (\r -> { … }) |> List.filter (\s -> s.shown)` returning `List Shown`.
  - `let`: `let wide = { … }` then `reportTime wide` where `reportTime : Curve -> …`.
  - `ctor-let`: `let m = Just { … }` then `m` returned as `Maybe Curve`.
  - `update`: `let w = { … }`, `let v = { w | sigma: 1.0 }`, `reportTime v`.
  - `infer-module`: an unsigned top-level literal (Options.InferModule) used at `Curve`.
- `TestRecordLitInference_PointVec`: one literal used as `Point` then `Vec`: `expected Vec, got Point`, no hint.
- Corpus `record-literal-inferred-ok` holds the four nightvote shapes as one module.

## Task 3: IR golden

Files: `testdata/golden/ir_record_infer/app/R.alder`, `testdata/golden/ir_record_infer.ir`
(generated with `go test ./internal/ir -run TestIRGolden -update`, reviewed: the `let wide` literal
lowers as the `R.Curve` record, not an anonymous one). Plus a one-off scratch `alder build` of the four
shapes to prove the Go compiles.

## Task 4: diagnostics (`recordDiff`)

Files: `internal/check/expr.go` (`recordDiff` beside `sameFields`; used by `mismatch` and the `==`
branch of `inferBinOp`). Corpus cases (each `app/M.alder` + `expected.diag`):

- `E0003-record-literal-field-type`: `; field `median` is Int, Curve's is Float`.
- `E0003-record-literal-missing-field`: `; the record is missing Curve's `perDoubling``.
- `E0003-record-literal-extra-field`: `; `extra` is not a field of Curve` (and missing+extra together).
- `E0003-record-literal-structural`: a literal that met a signature's inline record, then `Curve`:
  the reworded hint (`declare it as Curve where it is made …`).
- `E0003-record-literal-compare`: `==` mismatch gains the suffix.

`TestRecordLitInference_Diagnostics` in `record_infer_test.go` asserts each suffix substring too, and
that `annotate the let` never appears. Hint audit: grep `annotate the let` in `internal/lsp`,
`internal/diag/explain.go`, `book/`.

## Task 5: book

- `book/04-types.md` (Records): "… the type the context expects, even when the context is seen later
  (a `let`, one side of `==`, a lambda in a pipeline)".
- `book/13-compiler.md` after the `??` walkthrough: one paragraph on the handle.
- Regenerate `book/site` (the repo's generator), run `cd experiments && python3 -m unittest`.

## Finish

`go vet ./...`, `go test ./...` (10 min timeout each), push, `gh pr create --base main`.
