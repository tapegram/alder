# Record literals take the named type they meet

Design for [tapegram/alder#3](https://github.com/tapegram/alder/issues/3). Found four times in
nightvote-replay (`docs/alder-dev-log.md` there, and the issue). The user was not available for this
design; everything marked **(call)** is the author's decision, not the user's.

## Intent

The book already promises it (`book/04-types.md`, "Records"): *an anonymous record literal
`{ x: 1.0, y: 2.0 }` is given the type the context expects*. Today that is true only when the
expectation arrives before the literal is typed: a literal written directly as an argument, a return
value, an annotated `let`, a list element under a known `List T`, or a lambda body under a known
`Fun`. The checker's `check` pushes a nominal `Record` into `checkRecordLit` in all of those. Where the
expectation arrives *after* the literal has been typed, the literal is a structural record for good
and nominal typing rejects it:

1. `==` against a named type: `r == Ok { t0: …, … }` with `r : Result Text Cfg.Summary` →
   `cannot compare `Result Text Cfg.Summary` with `Result _ { t0: Int, … }``. `inferBinOp` infers both
   sides, then unifies.
2. A lambda in a pipeline: `rows |> List.map (\r -> { name: …, shown: … })` as the body of a function
   returning `List Clarity.Shown` → `expected List Clarity.Shown, got List { name: Text, shown: Curve.Shown }`.
   `applyFn` would bind `b` from `want` before checking the lambda, but `|>` calls `inferApp` with
   `want = nil`, and a chain (`|> List.map … |> List.filter …`) would defeat `want` anyway: the inner
   call is inferred first.
3. An unannotated `let`: `let wide = { median: …, sigma: …, perDoubling: … }` then `reportTime wide …`
   where `Curve` is expected. The literal was typed when nothing was expected.
4. A constructor payload met late: `Just { median: m, … }` is fine when `Maybe Curve` is the expected
   type of the whole `if`/`match`; the same literal in a `let` or on one side of `==` is case 1 or 3.

The workarounds (`let x : T = …`, a helper whose return type names `T`, a constructor function) all
say the same thing: tell the checker the name it would have found on its own. The hint in
`mismatch` ("the record has Curve's fields: annotate the let …") shows the checker already knows.

## Goals

1. All four shapes above type-check with no annotation, and lower to the nominal Go struct.
2. One rule, stated in one place (the unifier), so every position gets it: `==`, pipelines, lets,
   constructor payloads, list elements, record updates, and any position added later.
3. Records stay nominal: two named types with the same fields are still distinct, a value typed as
   `Point` is never a `Vec`, and a *declared* structural type (a signature's inline record) never
   silently becomes a named one.
4. Existing programs keep compiling and behaving identically. Every diagnostic that still fires keeps
   its code (E0003) and gets more specific, not less.

## Non-goals

- Structural subtyping or width subtyping: a literal with extra fields never fits a named type.
- Extensible or open record types (`{ r | x: Int }` as a *type*). Alder has record update syntax
  `{ r | f: v }` but no row types (`syntax.RecordType` has only `Fields`); this design adds none.
- Fixing `||` inside a `List.any` lambda ("cannot infer the record type of `x`", dev log 2026-10-27,
  Wood). That is field access on a lambda parameter whose type is still a variable, not a literal;
  it needs its own issue **(call)**.
- Propagating `want` through `|>` in `applyFn`. Unnecessary once the rule below exists, and it would
  not cover chains or lets; not added, to keep one mechanism **(call)**.
- Searching the declared types for a record with matching fields. No position ever asks "which named
  type has these fields?"; the expected type at the position decides (see "several").

## Anti-goals

- A literal that changes meaning depending on which use the checker happens to visit first *and*
  keeps compiling. Order-dependence is accepted only where the second use is an error either way.
- Lowering a literal as an anonymous Go struct into a slot typed as a named struct (a Go compile
  error the user cannot read). The literal's recorded type must be the type Lower emits.
- A new error code or a new hint for a situation the old hint already described.

## The rule

> A record literal typed without a record expectation gets an **undecided** structural type. The first
> record type it is unified with that has **exactly its field names** decides it: a nominal record
> makes the literal that nominal type; a structural record makes it structural. A literal never
> decided stays structural, as today.

Mechanics **(call)**:

- `types.Record` gains `Flex *Var` ("handle"), nil for every record except one made by `inferExpr`'s
  `RecordLit` case (`Record{Anonymous: true, Fields: …, Flex: u.Fresh()}`). `checkRecordLit` is
  unchanged: a literal checked against a known record never gets a handle.
- The handle is the literal's substitution slot. `Apply` on an anonymous record with a *bound* handle
  returns `Apply(Subst[Flex.ID])` (the decided type); with an unbound handle it returns the structural
  copy it builds today, **carrying `Flex` over** (today's copy would drop it). `subst` and
  `Instantiate` carry it over the same way; `prune` and `occurs` are unchanged.
- In `unify`'s `Record` case, after both sides are `Apply`ed, when exactly one side is anonymous with
  an unbound handle and the other is nominal:
  1. if the field *names* are not exactly equal as sets (order irrelevant, as `sameFields`): fall to
     `mismatch` as today, handle left unbound;
  2. unify each literal field's type with the nominal field's type (the nominal side is an instance:
     `Box a`'s fields already mention its `Args`, so a generic record binds its arguments here);
     on failure return that error, handle left unbound **(call)**: binding on a bad field would print
     "expected Curve, got Curve" at the use site;
  3. bind the handle: `Subst[Flex.ID] = nominal`.
  The rule is symmetric (either operand may be the literal) and applies in both `Unify` and `Check`
  **(call)**: `==` uses `Unify`, and a decision made in `Check` must be the same decision.
- Anonymous vs anonymous keeps today's check (same names in the same order, fieldwise unify). Then:
  both handles unbound → bind one to the other record (so two literals in one list decide together
  when either meets a name); one handle, the other a declared structural type → bind the handle to it
  (the literal is now structural for good; a later nominal use is a plain mismatch with the hint).
- The record-update case in `inferExpr` returns the base's type as today, so `{ wide | sigma: 1.0 }`
  shares `wide`'s handle; deciding either decides both. Field access on a handle-carrying record is
  field access on a structural record, unchanged.
- `Result` already `Apply`s every recorded type (`result.go:132`), so `res.Types[lit]` is the nominal
  type when decided and Lower's `l.record(x, l.recordType(t), t)` emits the named struct. Lower does
  not change. A side effect worth having: LSP hover on `wide` shows `Curve`.

### Zero, one, several

- **Zero** named types ever meet the literal: it stays structural, as today. No error, no warning.
- **One**: it becomes that type, wherever and however late the meeting happens, including across
  module boundaries (one `Unifier` per `Checker`).
- **Several** named types with the same fields (`Point`, `Vec`): nothing to choose; the literal becomes
  the first one it meets. A later use at the other name is `expected Vec, got Point`, with no hint
  (the record is nominal and the message says so; the fix is a second literal or a conversion the
  user writes). The order is source order of the uses, which is what a reader expects.
- A top-level value without a signature exists only under `Options.InferModule` (eval/REPL); its
  type is generalised by binding free *variables* to `Param`s. A handle is not a free variable (it
  is not a `*Var` in the type; `freeVars` does not see it), so the literal stays undecided across the
  generalisation and is decided by its first use, like a `let` **(call)**. Local `let`s are not
  generalised.

### Extra, missing, wrong-typed fields

Names must match exactly. A literal with a field the named type lacks, or lacking one it has
(including a field with a default: a literal of a declared record names every field, as
`checkRecordLit` already rules), does not bind and is the mismatch it is today. What changes is the
message (next section). Boundary records (an `Insert T`, a form, a page's or message's parameters,
the session cookie) only ever meet a literal in a checked position, so the exact-name rule costs
nothing there; it is stated so a builder does not add a defaults case to the unifier.

## Diagnostics

All E0003. `mismatch` (`expr.go`) and the `==` branch of `inferBinOp` share one new helper,
`recordDiff(got, expected) string`, that returns a suffix when `got` is an anonymous record and
`expected` a nominal one:

| situation | suffix |
|---|---|
| same names, a field's type differs | `; field `median` is Int, Curve's is Float` (first differing field) |
| names differ | `; the record is missing Curve's `perDoubling`` and/or `; `extra` is not a field of Curve` (all names, sorted) |
| same names and types, `got` carries no handle (a declared structural type, or a literal already decided structural) | `; the record has Curve's fields: declare it as Curve where it is made (a signature's inline record type, or a let that already met a structural type)` |

The third row replaces today's hint ("annotate the let (`let x : T = …`), or give the value a
signature"): once the rule exists, annotating a let is never the fix for a literal, so the hint must
not say it. `sameFields` stays as the name-set test the helper uses. The `==` message keeps its
shape (`cannot compare `A` with `B`: `==` needs both sides to have the same type`) and gains the
same suffix when either side's mismatch is a record one **(call)**: the first real case was an `==`.

The `Maybe` hint and the session widening messages are untouched. Printing is untouched: an
undecided literal prints as the structural record it is.

## Where things change

| layer | files |
|---|---|
| handle | `internal/types/types.go`: `Record.Flex`; `internal/types/unify.go`: `Apply` (bound/unbound handle, copy carries `Flex`), `subst`, `Instantiate`, the `Record` case of `unify`; `types_test`/`unify_test.go`: bind, decline (names, field type), link two literals, decide structural, symmetric, `Check` and `Unify`, generic nominal record |
| checker | `internal/check/expr.go`: `inferExpr` `RecordLit` sets the handle; `mismatch` and `inferBinOp` `==` use `recordDiff`; `sameFields` kept; `check_test.go`: the four shapes from this document as passing programs, plus `Point`/`Vec` order, extra/missing/wrong-typed field messages, the reworded hint, a literal that meets a signature's inline record then a named type |
| lowering | none; an IR golden (`internal/ir`) for the `let wide` shape proves the literal lowers as `Curve` |
| book | `book/04-types.md` line 57: one clause, "… the type the context expects, even when the context is seen later (a `let`, one side of `==`, a lambda in a pipeline)"; `book/13-compiler.md` after the `??` walkthrough: one paragraph on the handle, since that chapter teaches the unifier; regenerate `book/site` and check the reference prompt cap (`[tests]`, dev log 2026-10-20) |
| hint audit | grep `annotate the let` in `internal/lsp` and `internal/diag/explain.go`; no test covers the old hint today, so the new rows each get one |

Also: tick #3 in roadmap #52, and the user reinstalls (`go install ./cmd/alder`) after merge.

## Left to the builder

- Whether `recordDiff` lives in `expr.go` or beside `sameFields`; whether `Flex` is a `*Var` or an
  `int` ID (a `*Var` matches `Subst`'s key and `Fresh`).
- The exact wording of the two book sentences.
- Task order: unifier first (its tests pass with no checker change), then `inferExpr`, then
  diagnostics, then book.

## Verification

- `go test ./...` green, including the new `types` and `check` cases and the IR golden.
- The four shapes, written as checker tests exactly as the issue and dev log give them (an `Ok {…}`
  under `==` against `Result Text T`; `xs |> List.map (\s -> {…})` returning `List T`, and a two-step
  chain; `let wide = {…}` passed to a `T` parameter; `Just {…}` bound by a `let` then returned as
  `Maybe T`), each also lowered and built with `alder build` in a scratch app to prove Go compiles.
- A scratch build of nightvote-replay with this Alder, with the `: Curve` annotation on `wide`
  (`app/Utils/Curve.alder`, the `reportTime is clamped` test) removed, passes its tests. One-off; the
  project pins its own Alder and is not changed here.
