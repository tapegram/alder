# M9: Mutation testing — Design

Date: 2026-10-12. Status: approved in conversation; the M9 plan argues from
this. Related: M4 (tests), M5e (browser tests), E7's hand mutation probe,
E8 (hidden tests; "automated mutants should gate experiment material").

## Goal

`alder test --mutate` measures whether an app's tests would notice the
behaviours changing that matter in a server-rendered app, and prints every
surviving mutant as the test that is missing. Line coverage is meaningless in
a language whose checker already forces exhaustive matches and total
functions; a mutation score is the honest coverage number, and the survivors
list is the sharpest feedback an agent can get about its tests.

## Decisions recorded (2026-10-12)

- Amended after the Task 1 review (2026-10-12): `not dropped`, `session put dropped`, `arm swapped` per adjacent pair, a union default flips to the next constructor; `condition negated` skipped on a flippable operator; `suppressed` (equivalent) and `skipped` (not applicable) are reported apart.

- **A curated, domain-aware operator set** (user chose A): about fifteen
  operators on typed IR nodes, each producing a well-typed mutant by
  construction, each named by what it breaks. No generic expression
  mutation.
- **Opt-in, with a threshold** (A): `alder test --mutate [--min R]`; plain
  `alder test` is unchanged.
- **Structural suppression only** (C): the mutator refuses mutants it can
  prove equivalent; there is no annotation to silence a survivor.
- Tests, fakes and `view` bodies are never mutated.

## The command

```
alder test --mutate [--min R] [--json] [-run re] [-timeout d] [dir]
```

Runs the suite as today; if it passes, runs it once per mutant. Output:

```
30 tests passed
47 mutants: 44 killed, 3 survived  (score 0.94)
survived:
  Orders.Show   load:18    guard dropped       if order.owner == session.user then … else Task.notFound → the notFound arm dropped
  Todos.Index   update:31  write replaced      let _ <- Db.insert Todo {…} → Task.succeed
  Posts.Index   load:12    where dropped       Db.where (Post.published.eq True)
```

- A line per survivor: module, declaration and line, operator, the code
  before and after (one line each, elided to 80 columns).
- `score` = killed / total over the mutants that ran. `--min R` exits 1
  when the score is below `R` (printing the same report); without `--min`
  the exit code is the tests' as today.
- `--json` prints, after the per-test objects, one object
  `{"mutants": [{id, module, decl, line, kind, before, after, killed, by, note}]}`
  (`by`: the first test that failed; `note`: "crashed"/"timed out" when the
  mutant was killed that way) and a final `{"score", "killed", "survived",
  "suppressed"}`.
- `-run` filters the tests as usual; mutants are judged by the selected
  tests. A suite that does not pass unmutated stops before any mutant
  ("tests fail: fix them before measuring mutants").
- If the app has no tests: "no tests: nothing can kill a mutant", exit 1
  with `--min`.

## The operators

| Class | Kind | Node | Mutant |
|---|---|---|---|
| authorization | `requires dropped` | `Page.Requires` | the route no longer redirects a request without the session field: it serves the page with a placeholder value for that field (`UserId "mutant"`), so `load`/`update` keep their types; only for a field whose type is a brand over `Text` (otherwise the operator is skipped) |
| authorization | `guard dropped` | `If`/`Match` with exactly one arm whose body is `EffNotFound`/`EffFail` (`Task.notFound`, `Query.notFound`, `Task.fail _`) | the whole expression becomes the other arm |
| data | `where dropped` | `DbSelect.Where[i]` | that condition removed (one mutant per condition) |
| data | `orderBy reversed` | `DbSelect.Order[i]` | `asc`↔`desc` |
| data | `limit dropped` / `offset dropped` | `DbSelect.Limit`/`Offset` | the limit (or offset) removed |
| data | `write replaced` | `DbInsert`/`DbUpdate`/`DbDelete` as an `EffBind` step | the step becomes `Task.succeed v` where `v` is a typed placeholder: the inserted table's id brand of `"mutant"`, or `()` |
| data | `update field dropped` | `Update` with ≥ 2 fields | one field's update removed |
| flow | `redirect → rerender` | `Ctor Redirect` of `Nav` | `Rerender` |
| flow | `reject → rerender` | `Ctor Reject` of `Nav` | `Rerender` |
| flow | `arm swapped` | each adjacent pair of `Match` arms of the same type with different bodies (one mutant per pair) | the two bodies swapped |
| logic | `condition negated` | `If.Cond` | `not cond` |
| logic | `operator flipped` | `Op` with `==`/`!=`, `<`/`<=`, `>`/`>=`, `&&`/`\|\|` | the paired operator |
| logic | `bool flipped` | `Lit` of `Bool` in a non-test, non-view body | the other literal |
| logic | `not dropped` | `not e` (a `Call` of Prelude `not`) outside tests/view | `e` |
| data | `session put dropped` | a `Session.put` step | the step becomes `Task.succeed ()` |
| schema | `default changed` | a table field default | `False`↔`True`, `0`→`1`, `""`→`"x"`, a union default → the next constructor in declaration order (`Reader`→`Author`), `Nothing`→(skipped) |
| schema | `cascade ↔ restrict` | a `@ref` delete rule | the other rule |

Structural suppression (refused, counted as `suppressed` in the JSON, never
listed; operator-not-applicable cases are a separate `skipped` count): `condition negated` when the condition is an operator `operator flipped` already covers (the two mutants would be the same program); a `guard dropped` whose surviving arm uses a binding the guard
introduced (unscopable); `arm swapped` with identical bodies; `orderBy reversed` under a
`Db.get`/`Db.first` with a `where` on the id; `where dropped` when the
only condition is the id equality of a `Db.get` (`Db.get` has no `where`:
n/a); `condition negated` on a literal condition; `operator flipped` where
both operands are the same expression; `write replaced` on a write whose
result is never bound or used AND the test suite has no `Db` reads of that
table (not provable cheaply: skip this rule); `default changed` on a field
no `Db.insert` omits (the default never applies). Keep the list exactly
this; everything else survives honestly.

Scope: mutants are made in every non-test module's `load`, `update`, plain
values and helpers; never in `test` bodies, `fake` entries, `view` and
components, or the stdlib. Mutant ids are assigned in IR order (module,
declaration, node pre-order), so they are stable across runs; a source edit
renumbers later mutants (documented).

## How it runs: one binary, mutant schemata

`gen.Options{Mutate: true}` (set only by `alder test --mutate`) emits every
mutation point as a runtime switch on a package-level `var Mutant int`:

```go
if app.Mutant == 17 { <mutated> } else { <original> }
```

plus `var Mutants = []rtesting.Mutant{{ID, Module, Decl, Line, Kind, Before, After}, …}`
and, for schema mutants, a per-mutant `[]db.TableDesc` override. All
mutants compile once, in the same test build `alder test` makes; the
production build (`Mutate` unset) is byte-identical to today's.

`rtesting.Main --mutate`: runs the suite (unchanged) and, if it passed, for
each mutant sets `Mutant = id` (and the table override), runs the suite
again in the same process with a fresh database per test as today,
stopping at the first failing test (killed, `by` = that test). A test
panic or a stack overflow kills the mutant (`note: crashed`); a per-mutant
deadline (the suite's `-timeout`, default 10 s) kills it (`note: timed
out`). The suite's own timeout applies per mutant, not to the whole run.
Mutants run sequentially (the runtime's effect interpreter is single-threaded
per env; parallelism is a later optimisation).

Cost: one build plus (mutants × tests) executions, early-stopped; a
50-mutant app over a 30-test browser suite is seconds.

## Compiler pieces

- `internal/ir`: the mutated node kinds gain a `Pos` (line, column of the
  source expression) so a survivor can name its line; set by the lowerer.
- `internal/mutate`: `Collect(p *ir.Program) []Mutant` — the table, with
  the suppression rules; `Apply` is not needed (gen reads the table and the
  node annotations); `Mutant{ID int; Module, Decl, Kind string; Line int;
  Before, After string; Node ir.Expr; Replacement ir.Expr; Tables
  []db.TableDesc (schema mutants only)}`.
- `internal/gen`: with `Options{Mutate}`, `code(e)` consults a map
  `node → []mutant` and wraps; `requires dropped` is emitted in the route
  (the `requires` check skipped under the id); schema mutants are a
  `Mutants[i].Tables` value.
- `cmd/alder test`: `--mutate`, `--min`; passes through to the runner;
  `--json` shape above.
- Harness: a `--test` round of a passing program that also ran `--mutate`
  records `mutation: {score, killed, survived, suppressed, survivors:
  [{module, decl, line, kind}]}` (opt-in `--mutate` flag on `run_loop.py`;
  feedback of survivors to the model is E9's design).

## Book

- Chapter 10, a new section "Mutation testing": why (coverage that means
  something), the command, the operator table (as above, in prose + table),
  the output with a REAL survivors list from the todos example, the rule
  "a surviving mutant is a test you have not written", what is suppressed
  and what survives legitimately (with one real example), `--min` in CI,
  the fact that ids renumber on edits.
- Chapter 11: the flags. Chapter 13: schemata (one real generated switch
  quoted), how a kill is decided, the per-mutant timeout. Chapter 12: why
  not line coverage; why no annotation. README. The agent reference keeps
  the rule, the command and the output shape.
- The examples: `alder test --mutate` on todos and blog; survivors become
  new tests in the examples where they reveal a real gap, and the book
  quotes the before/after.

## Testing

- `internal/mutate`: a golden per operator (fixture program → mutant table
  text), the suppression rules each with a positive and a negative case,
  id stability (the same program twice; an edit above renumbers below).
- gen: a `gen_mutate` golden (the switches, `Mutants`, a schema override);
  the production golden unchanged with `Mutate` unset.
- rtesting: kill by failure, by crash, by timeout; `--min`; `-run`
  narrowing; "tests fail: fix them first"; no tests.
- e2e: a fixture app with one test per behaviour where every mutant is
  killed; the same app with one test removed where exactly the expected
  mutant survives (one case per operator class).
- Examples under `--mutate` in `TestExample_*` (score recorded, no
  threshold).

## Non-goals

- Generic/exhaustive mutation; mutation of `view`; a source annotation to
  skip mutants; parallel mutant execution; incremental/cached results;
  feeding survivors back to a model (E9).
