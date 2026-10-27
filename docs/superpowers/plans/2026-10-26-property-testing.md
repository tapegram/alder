# Property-based testing (#22) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** A `test` whose body is a function (or a `Fuzz.forAll`) runs as a property: 100 generated cases, internal shrinking, a minimal counterexample printed as Alder literals with a paste-ready plain test, saved failures replayed first.

**Architecture:** The checker recognises property bodies and records the concrete argument types (defaulting free variables to `Int`). `ir.Lower`, which still sees brands, turns each argument type and each `Fuzz.any` into a generator tree; gen emits one Go generator and one literal printer per concrete type. A new runtime package `rt/fuzz` owns the choice sequence, primitive draws, combinators and the minithesis shrinker; `rt/testing` runs the property loop beside `runCase`.

**Tech Stack:** Go (compiler `internal/*`, runtime `rt/*`), Alder stdlib signatures (`stdlib/*.alder`), tree-sitter grammar (JS), the book (Markdown with checked examples).

**Spec:** `docs/superpowers/specs/2026-10-26-property-testing-design.md` (read §0 and §0a first; §0a overrides the body where they differ).
**Builder-check facts:** every `file:line` below comes from the builder check of 2026-10-26; line numbers are as of alder `56df4b69` and may drift by a few lines.

## Global Constraints

- Default runs per property: **100** (`--fuzz N`). Seed: random unless `--seed S`; printed on every property failure.
- Shrink step budget: **1000** attempts. Recursive union depth budget: **4**, each level halving the chance of a recursive constructor.
- Health checks: filter/brand rejects **> 90%** of draws over the first **50** attempts → FAIL; one attempt over **5 s** → FAIL naming the case; more than **8192 choices** (64 KiB at 8 bytes each) in one attempt → FAIL "generator draws too much".
- Paste-ready test over **20 lines** → print the arguments only and the line `paste the values above into a plain test`.
- Saved failures: `build/<app>/fuzz/<Module>/<first 16 hex of sha1(test name)>`, passed to the test binary as env `ALDER_TEST_FUZZ_DB`. Deleted when the property passes a full run. Tried before fresh draws, regardless of seed.
- Under `--mutate`: seed 0 (or `--seed`), no shrinking, no saved-failure reads or writes; a property counts as one test.
- New diagnostics: **E0038** (property argument / `Fuzz.any` type not derivable or not concrete), **E0039** (`Property` value outside a test body). Each needs `internal/diag/diag.go` const, `internal/diag/explain.go` paragraph and a row in `book/11-tooling.md` (~1583).
- Generated `Text` is printable code points plus `\n \t \r` only; generated `Bytes` is valid UTF-8 only (so every value has a literal).
- Names outside Prelude stay qualified: `Fuzz.Fuzzer`, `Fuzz.Property`. `Test.alder` is not touched.
- No new runner, no new declaration form; plain tests' output, JSON and goldens stay byte-identical.

## Review Focus

1. **A user brand whose predicate is tight** (e.g. `brand Text where Text.length t == 5`) must fail with the health-check message naming the brand, never hang or pass vacuously. Test added to Task 6.
2. **A record argument containing `Text.Email`** (the common case: a `User` row) must generate valid emails and pass 100 runs; filtering would fail every user. Test added to Task 5.
3. **A shadowed lambda parameter in the body** (`\x -> let x = 1 in x == x`, or a nested `\x ->`) must produce a paste-ready case that substitutes only the outer binding. Test added to Task 7.
4. **An effect property that inserts rows** must see an empty database on every attempt, including shrink candidates (no leakage between attempts). Test added to Task 6.
5. **A free type variable inside a record or Maybe** (`\m -> Maybe.withDefault 0 (Maybe.map identity m) >= 0 || True`) must default to `Int` and the report must say so. Test added to Task 3.

---

## File Structure

| File | Responsibility |
|---|---|
| `internal/syntax/{ast.go,parser_expr.go}` | `Lambda.Types []Type` (nil entry = unannotated) and parsing `(x : T)` |
| `internal/format/print_expr.go` | print annotations |
| `tree-sitter-alder/grammar.js` + `queries/` + `test/corpus/expressions.txt` | grammar for annotated params |
| `stdlib/Fuzz.alder` | signatures of §3.2 plus `type Fuzzer a`, `type Property` |
| `internal/check/fuzz.go` (new) | property-body recognition, `Fuzz.any`, defaulting, derivability, E0038/E0039 |
| `internal/check/expr.go` | hooks into `checkBodies`, the Var case, lambda check/infer |
| `rt/fuzz/` (new) | `choices.go`, `draw.go`, `combinators.go`, `shrink.go`, `brands.go`, `literal.go` |
| `internal/ir/fuzz.go` (new) | `GenTree`, `FuzzDerive`, `TestProp` lowering |
| `internal/gen/fuzz.go` (new) | emit generators and literal printers per type; `Fuzzer` GoType |
| `rt/testing/property.go` (new) | `Property`, property loop, health checks, saved-failure db |
| `rt/testing/testing.go`, `cmd/alder/test.go` | `Case.Prop`, report, flags, env, crash seed |
| `book/10-testing.md`, `09`, `12`, `13`, `11`, `A-grammar.md` | docs |

---

### Task 1: Annotated lambda parameters

**Files:**
- Modify: `internal/syntax/ast.go:319-323`, `internal/syntax/parser_expr.go:290-303`
- Modify: `internal/check/expr.go:521-536` (check against Fun) and `902-911` (infer)
- Modify: `internal/format/print_expr.go` (Lambda case)
- Modify: `tree-sitter-alder/grammar.js:118-119`, regenerate the parser, `tree-sitter-alder/test/corpus/expressions.txt`, highlight queries if they name lambda params
- Modify: `book/A-grammar.md` (lambda production), `book/03-syntax.md` (lambda section: one example)
- Test: `testdata/golden/parse_lambda_annot.alder`, `testdata/golden/fmt_lambda_annot.{alder,golden}`, `testdata/golden/check_lambda_annot/` + `.diag`

**Interfaces:**
- Produces: `syntax.Lambda{Params []*Ident; Types []Type; Body Expr}` where `len(Types) == len(Params)` and `Types[i] == nil` means unannotated. Later tasks read `Types[i]` to know the user chose a type.

- [ ] **Step 1: Write failing goldens.** Parse golden containing `f = \(x : Int) y (z : List Text) -> x`, `g = \(s : Text.NonEmpty) -> s`, `h = \(p : { a: Int }) -> p.a`. Fmt golden: input `\( x:Int )  y->x` formats to `\(x : Int) y -> x`. Check golden: `bad = \(x : Int) -> x ++ "a"` reports the usual E0003 at `x ++`; `ok = List.map (\(n : Int) -> n + 1) [1]` is clean; `amb = \(x : Text) -> x` used as `amb 1` reports E0003 at the argument.
- [ ] **Step 2: Run** `go test ./internal/syntax ./internal/format ./internal/check -run 'Golden'` — expect FAIL (parse error at `(`).
- [ ] **Step 3: Implement.** Parser: in the `Backslash` case loop while at `LIdent` or `LParen`; `(` must be followed by `LIdent`, `Colon`, a type (reuse the signature type parser), `RParen`. Checker: an annotated parameter's type is the resolved annotation (resolve like a signature's type, so qualification rules and E0202 apply); in check mode unify it with the expected param type, in infer mode use it instead of a fresh var. Formatter prints `(x : T)` with the existing type printer. tree-sitter: `seq('\\', repeat(choice(identifier, seq('(', identifier, ':', $._type, ')'))), '->', …)`.
- [ ] **Step 4: Run** the three goldens, `go test ./internal/format -run Corpus`, `go test ./tree-sitter-alder/...` (drift test) and `cd tree-sitter-alder && npx tree-sitter test` — expect PASS.
- [ ] **Step 5: Run** `go test ./book/...` — expect PASS with the new book example.
- [ ] **Step 6: Commit** `feat(syntax): annotated lambda parameters (#22)`.

---

### Task 2: `Fuzz` stdlib surface in the checker

**Files:**
- Create: `stdlib/Fuzz.alder`
- Create: `internal/check/fuzz.go`
- Modify: `internal/check/expr.go:950-960` (Var case), `internal/diag/diag.go:91-131`, `internal/diag/explain.go`, `book/11-tooling.md:1583-1601`
- Modify: `internal/gen/gen_test.go:792` exemption list (so `TestStdCallsCoverStdlib` stays green until Task 5 maps the names; exempt `Fuzz.*` for now with a `// Task 5` comment)
- Test: `testdata/corpus/E0039-forall-outside-test/`, `testdata/corpus/fuzz-helper-ok/`, `testdata/corpus/app-module-named-fuzz/`

**Interfaces:**
- Produces: `stdlib/Fuzz.alder` exactly the signatures of spec §3.2 (with `constant : a -> Fuzzer a`), plus `type Fuzzer a` and `type Property`. Types: `Fuzzer` is `types.App{Con:"Fuzzer"}`, `Property` is `types.Con{Name:"Property", Module:"Fuzz"}` (`internal/check/decls.go:45-57`).
- Produces: `diag.NotDerivable = "E0038"`, `diag.PropertyOutsideTest = "E0039"`.
- Produces: in `internal/check/fuzz.go`, `func isFuzzForAll(e syntax.Expr) bool` — true when `e` is an App (or last `|>` stage) whose head resolves to `Fuzz.forAll` or `Fuzz.forAllTask`.

- [ ] **Step 1: Write failing corpus cases.**
  - `E0039-forall-outside-test`: `p = Fuzz.forAll Fuzz.int (\n -> n == n)` at top level → `E0039` with message `` `Fuzz.forAll` makes a Property, which only a test body can use ``.
  - `fuzz-helper-ok`: a non-test module with `evens : Fuzz.Fuzzer Int` / `evens = Fuzz.map (\n -> n * 2) Fuzz.int` and a test `test "evens" = Fuzz.forAll evens (\n -> n % 2 == 0)` → no diagnostics.
  - `app-module-named-fuzz`: an app with `app/Fuzz.alder` — expected diagnostics are whatever today's rule for an app module shadowing a stdlib module produces (look up `internal/names/resolve.go` handling of e.g. an app `Http` module and copy that behaviour); record it in `expected.diag`.
- [ ] **Step 2: Run** `go test ./internal/check -run Corpus` — expect FAIL.
- [ ] **Step 3: Implement.** Add the stdlib file (it is embedded by `stdlib/embed.go`). Leave `Fuzz.any` as a `-- special form:` comment line; its Var case is Task 3. E0039: in the checker, an expression whose type is `Fuzz.Property` anywhere other than directly as a `TestDecl.Body` reports E0039 (check at the `forAll` App when not `mc.testBody` top). Explain text and book row follow the E0003 style at `explain.go:37-40`.
- [ ] **Step 4: Run** `go test ./internal/check ./internal/diag ./internal/gen -run 'Corpus|Explain|StdCalls'` — expect PASS.
- [ ] **Step 5: Commit** `feat(check): Fuzz stdlib signatures and E0039 (#22)`.

---

### Task 3: Property bodies, `Fuzz.any`, Int defaulting, E0038

**Files:**
- Modify: `internal/check/fuzz.go`, `internal/check/expr.go:54-63` (`checkBodies`), `expr.go:580-600` (`annotateMismatch`), Var case `950-960`
- Modify: `internal/check/result.go` (expose `Result.Props` and `Result.FuzzAny`)
- Test: `testdata/golden/check_props/` + `.diag`, `internal/check/result_test.go`, corpus `E0038-html-arg`, `E0038-any-polymorphic`, `E0038-fun-field`

**Interfaces:**
- Consumes: `syntax.Lambda.Types` (Task 1), `isFuzzForAll` (Task 2).
- Produces on `check.Result`:
  ```go
  type PropInfo struct {
      Kind      PropKind      // PropFun or PropForAll
      Effect    bool          // body result is Task Bool
      Params    []*syntax.Ident
      Types     []types.Type  // final, concrete (after defaulting)
      Defaulted []bool        // a free var in Types[i] was set to Int
  }
  Props   map[*syntax.TestDecl]PropInfo
  FuzzAny map[*syntax.Var]types.Type // concrete type at each Fuzz.any use
  ```
- Produces: `func derivable(t types.Type) (path string, ok bool)` in `fuzz.go`; `path` names the offending part, e.g. ``Html Msg` in field `view` of `Card``.

- [ ] **Step 1: Write failing tests.**
  - `check_props` golden, clean: `test "rev" = \xs -> List.reverse (List.reverse xs) == xs`; `test "slug" = \(s : Text.NonEmpty) -> Text.length (Text.NonEmpty.toText s) > 0` (use the real accessor name from `stdlib/Text.alder`); a block-bodied `\(t : Text.NonEmpty) -> { … Task.succeed True }`; `test "any" = Fuzz.forAll (Fuzz.list Fuzz.any) (\xs -> List.length xs >= 0)`.
  - Same golden, errors: `test "ret" = \x -> x + 1` → E0003 with the existing mismatch hint retargeted to the lambda body (`a property must return Bool or Task Bool`).
  - `result_test.go` `TestProps_Defaulting`: for `\xs -> List.reverse (List.reverse xs) == xs`, `Props[..].Types[0]` prints `List Int` and `Defaulted[0] == true`; for `\(xs : List Text) -> …` it prints `List Text`, `Defaulted[0] == false`; **Review Focus 5**: for `\m -> Maybe.withDefault 0 (Maybe.map identity m) >= 0 || True`, `Types[0]` is `Maybe Int`, `Defaulted[0] == true`.
  - `TestProps_FuzzAny`: `Fuzz.forAll (Fuzz.list Fuzz.any) (\(xs : List Bool) -> True)` records `Bool` for the `Fuzz.any` Var.
  - Corpus: `E0038-html-arg` (`\(h : Html Msg) -> True`) → `E0038` message `` `Html Msg` cannot be generated; use `Fuzz.forAll` with a generator you write ``; `E0038-fun-field` (a record with a function field) → path names the field; `E0038-any-polymorphic` (`gen : Fuzz.Fuzzer a` / `gen = Fuzz.any`) → E0038 `` `Fuzz.any` needs a concrete type here ``.
- [ ] **Step 2: Run** `go test ./internal/check` — expect FAIL.
- [ ] **Step 3: Implement.** In `checkBodies`: a `*syntax.Lambda` body is checked against `Fun{Params: annotations or fresh vars, Result: Task Bool if the lambda body is a *syntax.Block else Bool}`; `isFuzzForAll(body)` is checked against `Fuzz.Property`; anything else unchanged. A body that merely names a function stays a Bool mismatch (no new hint beyond the existing one). After the body is checked, apply the substitution to each param type and set every `freeVars` var to `types.Int` (mechanism of `inferTopLevel`, `expr.go:190-192`), then `derivable` each → E0038 at the parameter. `Fuzz.any`: a bare Var branch returning `App{Fuzzer, fresh}`, recorded via `c.record`; collected per declaration; after the declaration is checked, inside a test default free vars to Int, outside a test any remaining `Var`/`Param` is E0038. Derivable set is spec §4's table; the not-derivable list is spec §4 plus `Table t, Select t, Condition t, Order t, Column t a, Insert t, File, Error, Never, Answer, HttpError, Nav`. `Id t` is derivable.
- [ ] **Step 4: Run** `go test ./internal/check ./internal/diag` and `go test ./... -run 'Golden|Corpus|Examples'` — expect PASS, existing goldens unchanged.
- [ ] **Step 5: Commit** `feat(check): property test bodies and Fuzz.any (#22)`.

---

### Task 4: `rt/fuzz` engine

**Files:**
- Create: `rt/fuzz/choices.go`, `rt/fuzz/draw.go`, `rt/fuzz/combinators.go`, `rt/fuzz/shrink.go`
- Test: `rt/fuzz/choices_test.go`, `rt/fuzz/draw_test.go`, `rt/fuzz/shrink_test.go`

**Interfaces:**
- Produces (only import: `alder/rt`):
  ```go
  type Choices struct{ /* PRNG, recorded []uint64, replay []uint64, pos int */ }
  func Fresh(seed uint64) *Choices
  func Replay(seq []uint64) *Choices
  func (c *Choices) Choice(n uint64) uint64   // value in [0, n]; replay returns min(seq[pos], n)
  func (c *Choices) Recorded() []uint64
  type Overrun struct{}                         // panic value: replay ran past its end, or > MaxChoices
  type Reject struct{ What string }             // panic value: a filter/brand predicate said no
  const MaxChoices = 8192
  type Fuzzer[A any] func(*Choices) A

  // primitive draws
  func Bool(c *Choices, p float64) bool
  func IntRange(c *Choices, min, max int64) int64
  func Int(c *Choices) int64          // magnitude then sign; mostly small, sometimes extreme
  func Float(c *Choices) float64      // finite; 0, -0, tiny, huge included
  func Text(c *Choices) string        // printable + \n\t\r; mostly short ASCII
  // combinators mirror stdlib/Fuzz.alder 1:1, e.g.
  func List[A any](f Fuzzer[A]) Fuzzer[[]A]          // continue bit per element, stop = 0
  func Filter[A any](what string, p func(A) bool, f Fuzzer[A]) Fuzzer[A]  // panics Reject{what}
  func AndThen[A, B any](k func(A) Fuzzer[B], f Fuzzer[A]) Fuzzer[B]
  // ... Constant, BoolF, IntF, IntRangeF, FloatF, FloatRangeF, TextF, TextOf, Char, ListOf,
  //     Maybe, Result, Pair, Dict, OneOf, OneOfValues, Frequency, Map, Map2

  func Shrink(seq []uint64, fails func([]uint64) bool, budget int) (best []uint64, steps int)
  func Encode(seq []uint64) []byte   // uvarint stream
  func Decode(b []byte) ([]uint64, error)
  ```
  The Go representations of Alder values (`Maybe`, `Result`, `Dict`, records for `pair`) must match what `internal/gen/types.go` `GoType` emits; read it and use the same runtime types (`rt.Maybe[A]` etc.).

- [ ] **Step 1: Write failing tests.**
  - `TestReplayDeterministic`: drawing `List(IntF)` from `Fresh(42)` twice gives equal values; `Replay(c.Recorded())` reproduces the value.
  - `TestReplayOverrun`: `Replay([]uint64{1})` drawing a 3-element list panics `Overrun{}`.
  - `TestMaxChoices`: a generator drawing 9000 choices panics `Overrun{}`.
  - `TestTextPrintable`: 10 000 draws of `Text` contain no rune `< 0x20` except `\n \t \r`, no `0x7f–0x9f`, and are valid UTF-8.
  - `TestFloatFinite`: 10 000 draws, none NaN/Inf; at least one `0`, one negative zero, one `|x| > 1e300`.
  - `TestSmallerIsSimpler`: `Replay` of all-zero sequences gives `0`, `[]`, `""`, `False`, first `OneOf` alternative.
  - `TestShrinkList`: property "no list contains an element ≥ 10" — shrink from a random failing sequence yields the value `[10]`.
  - `TestShrinkPair`: property "a + b < 100" over two ints — shrinks to a pair summing to exactly 100 with one side 0.
  - `TestShrinkBudget`: `fails` that always returns true and a budget of 1000 → `steps <= 1000`.
  - `TestEncodeRoundTrip`.
- [ ] **Step 2: Run** `go test ./rt/fuzz` — expect FAIL (package missing).
- [ ] **Step 3: Implement.** PRNG: `math/rand/v2` PCG seeded from `seed`. Shrink passes, repeated until a full round changes nothing or the budget is spent: delete blocks of size 8,4,2,1; zero blocks of the same sizes; lower each choice by binary search toward 0; sort adjacent pairs ascending. A candidate is kept only if it is shortlex-smaller and `fails` returns true; `Overrun`/`Reject` during a candidate count as not failing.
- [ ] **Step 4: Run** `go test ./rt/fuzz -race` — expect PASS.
- [ ] **Step 5: Commit** `feat(rt/fuzz): choice sequence, draws, combinators, shrinker (#22)`.

---

### Task 5: Derived generators and literal printers (lowering + codegen)

**Files:**
- Create: `internal/ir/fuzz.go`, `internal/gen/fuzz.go`, `rt/fuzz/brands.go`, `rt/fuzz/literal.go`
- Modify: `internal/ir/ir.go` (new nodes), `internal/ir/lower.go:64-111` (`lowerTests`), `internal/gen/types.go` (`Fuzzer` GoType), `internal/gen/stdcall.go` (map every `Fuzz.*` to `fuzz.*`; drop Task 2's exemption except `Fuzz.any`, `Fuzz.forAll`, `Fuzz.forAllTask`), `internal/gen/names.go:20-60` (document `fuzz__…`, `lit__…`)
- Test: `internal/gen/gen_test.go` (`TestGen_FuzzDerive*`), `rt/fuzz/brands_test.go`, `rt/fuzz/literal_test.go`, `testdata/golden/gen_fuzz/`

**Interfaces:**
- Consumes: `check.Result.Props`, `check.Result.FuzzAny` (Task 3); `rt/fuzz` API (Task 4).
- Produces IR:
  ```go
  type GenTree struct {
      Kind   GenKind     // Prim, List, Maybe, Result, Dict, Record, Union, Brand, StdBrand
      Prim   string      // "Int","Float","Bool","Text","Unit","Bytes","Date","Time","Id"
      Elems  []*GenTree  // children in order (record fields, ctor payloads, k/v)
      Names  []string    // record field / ctor names
      Rec    []bool      // Union: ctor is recursive
      Brand  string      // "M.B" for Brand/StdBrand
      Key    string      // types.String of the type: the emitted function's identity
  }
  type FuzzDerive struct{ Gen *GenTree }       // expr: the value of Fuzz.any
  type TestProp struct {                        // replaces ir.Test.Body for a property
      Params    []string
      Gens      []*GenTree
      Defaulted []bool
      Fn        Expr      // the lambda, or forAll's function argument
      ForAllGen Expr      // non-nil for Fuzz.forAll: the explicit generator
      ForAllLit *GenTree  // the value type of forAll's generator, for printing
  }
  ```
- Produces generated Go: per distinct `Key`, `func fuzz__<mangled>(c *fuzz.Choices, depth int) T` and `func lit__<mangled>(v T) string`. A property test emits `rtesting.Case{…, Prop: &rtesting.Property{…}}` (field shapes defined in Task 6).
- Produces in `rt/fuzz/brands.go`: `NonEmpty, Email, URL, Secret func(*Choices) string`, `Positive, NonNegative func(*Choices) int64`, each post-checked with the predicate from `internal/gen/routes.go:936-957` `brandCheck` (move or share that predicate via `rt/web`, do not duplicate it).
- Produces in `rt/fuzz/literal.go`: `func TextLit(s string) string` (escapes `\" \\ \{ \n \r \t`), `IntLit(int64)` (`(-9223372036854775807 - 1)` for MinInt64), `FloatLit(float64)`, `TimeLit(ms int64)` → `Time.fromMillis N`, `DateLit(days int)` → `Date.addDays N (Time.toDate Time.epoch)`, `BytesLit([]byte)` → `Text.toBytes "…"`.

- [ ] **Step 1: Write failing tests.**
  - `literal_test.go`: table of value → literal for each function above, including `TextLit("a\"{\n")` → `"a\"\{\n"`. Decide `-0.0` by parsing: if the Alder parser keeps the sign of `-0.0`, `FloatLit(-0)` is `-0.0`; otherwise drop `-0` from `Float` (change Task 4's test) — record which in the test comment.
  - `brands_test.go`: 1000 draws each of `Email`, `URL`, `NonEmpty`, `Secret` pass `brandCheck`'s predicate; `Positive > 0`; `NonNegative >= 0`.
  - `gen_fuzz` golden: an app with a record `Todo { title: Text.NonEmpty, due: Maybe Date, tags: List Text }`, a recursive `type Tree = Leaf | Node Tree Int Tree`, a user brand `Slug = Text where …`, and properties over each; golden is the emitted Go for the `fuzz__`/`lit__` functions.
  - `TestGen_FuzzDeriveCompiles`: the golden app builds with `go build` (use the existing gen test harness that compiles).
  - **Review Focus 2**: `TestGen_FuzzDeriveEmailRecord`: a property `\(u : User) -> Text.Email.toText u.email /= ""` (real accessor from `stdlib/Text.alder`) over a record with an `Email` field compiles, and running the generator 100 times from `Fresh(1)` never panics `Reject`.
- [ ] **Step 2: Run** `go test ./internal/gen ./internal/ir ./rt/fuzz` — expect FAIL.
- [ ] **Step 3: Implement.** Lowering builds `GenTree` from `check.Result` types (brands are still present there; `types.Brand{HasCheck}` with `HasCheck` → `Brand` node calling `check__M__B`, without → plain base). Union: constructor index drawn with non-recursive constructors lowest; at `depth == 0` only non-recursive constructors; recursive constructor chance halves per level from the root budget 4. Generic records/unions are monomorphised by `Key`. `Fuzz.forAll`'s arguments lower to ordinary calls of `rtesting.ForAll`/`ForAllTask` (Task 6); `Fuzz.any` lowers to `FuzzDerive`. `Date`/`Time` draw near the epoch **and** near the test clock start 2026-01-01 (spec §4 + builder B24), with the epoch itself among the simplest choices.
- [ ] **Step 4: Run** `go test ./internal/... ./rt/fuzz` and `go test ./... -run 'Golden|StdCalls'` — expect PASS.
- [ ] **Step 5: Commit** `feat(gen): derived generators and literal printers (#22)`.

---

### Task 6: The property loop in `rt/testing`

**Files:**
- Create: `rt/testing/property.go`
- Modify: `rt/testing/testing.go` (`Case` 198-205, `CaseResult` 211-226, `runCase` 363-438, `WriteResult` 494-527, `jsonResult` 473-486, `Main` flags 576-585), `internal/gen/caps.go:298-316` (`testsTable` emits `Prop:`), `internal/ir/lower.go:64-111` (`testEq` looks through the lambda / block / forAll function; `Effect` from the Fun's result)
- Test: `rt/testing/property_test.go`, `cmd/alder/test_test.go`

**Interfaces:**
- Consumes: `rt/fuzz` (Task 4), generated `fuzz__`/`lit__` (Task 5).
- Produces:
  ```go
  type Arg struct{ Name, Type string; Defaulted bool }
  type Property struct {
      Args []Arg
      Draw func(*fuzz.Choices) []any            // one value per arg
      Lit  func([]any) []string                 // literal per arg
      Pure func([]any) bool                     // exactly one of Pure/Task
      Task func([]any) eff.Task[bool]
      Case func(lits []string) string           // paste-ready test text (Task 7 fills it; nil until then)
  }
  func ForAll[A any](g fuzz.Fuzzer[A], f func(A) bool) *Property
  func ForAllTask[A any](g fuzz.Fuzzer[A], f func(A) eff.Task[bool]) *Property
  // Case gains: Prop *Property
  // CaseResult gains: Args []ArgValue{Name, Type, Value string; Defaulted bool}; Seed uint64;
  //   Run, Runs, Shrinks int; Paste string
  // Options gains: Fuzz int (0 → 100); Seed uint64; SeedSet bool; FuzzDB string; Mutating bool
  ```
- Produces env/flags read by `Main`: `ALDER_TEST_FUZZ`, `ALDER_TEST_SEED`, `ALDER_TEST_FUZZ_DB`.

- [ ] **Step 1: Write failing tests** in `property_test.go` (hand-built `Property` values, no compiler):
  - `TestProp_Passes`: `xs reversed twice == xs` over `fuzz.List(fuzz.IntF)` → OK, `Runs == 100`.
  - `TestProp_ShrinksAndReports`: "every list has length < 2" → FAIL; `Args[0].Value == "[0, 0]"`; `WriteResult` output contains `(failed on run`, `shrunk in`, `  xs: [0, 0]`, `  seed: `.
  - `TestProp_LeftRight`: a property ending in `==` fills `left:`/`right:` for the shrunk case (re-run once after shrinking).
  - `TestProp_FilterTooTight` (**Review Focus 1**): `Filter("Slug", func(s string) bool { return len(s) == 5 }, Text)` → FAIL with `` `Slug`'s predicate rejects `` and `write a \`Fuzz.forAll\``; completes in < 2 s.
  - `TestProp_TooSlow`: an attempt sleeping 6 s on its 3rd run → FAIL naming run 3 (use an injectable clock or a `slowAfter` variable set to 50 ms in the test).
  - `TestProp_DrawsTooMuch`: a generator drawing 9000 choices → FAIL `generator draws too much`.
  - `TestProp_SeedReproducible`: two runs with `Seed: 7, SeedSet: true` and no db produce identical `Args`.
  - `TestProp_SavedFailureReplayedFirst`: run a failing property with `FuzzDB: t.TempDir()`; the file `<db>/M/<hash>` exists; a second run with a different seed fails on `Run == 1`; after the property is changed to pass, a full run deletes the file.
  - `TestProp_Mutating`: with `Mutating: true` no file is written, `Shrinks == 0`.
  - `TestProp_EffectIsolation` (**Review Focus 4**): a `Task` property that inserts a row into a test table and asserts the table had 0 rows before the insert passes 100 runs and its shrink of a deliberate later failure still sees 0 rows.
  - `cmd/alder/test_test.go` `TestTest_Property`: `tempTodos` with `test "rev" = \xs -> List.reverse xs == xs` → exit 1, stdout contains `FAIL Table.Todo "rev"  (failed on run`, `  xs: [0, 1]` (or the shrinker's actual minimal two-element list; assert the shape `xs: [` and two elements), `  seed: `; the JSON variant contains `"property":{`.
- [ ] **Step 2: Run** `go test ./rt/testing -run Prop` — expect FAIL.
- [ ] **Step 3: Implement.** Loop: saved sequence first (unless `Mutating`), then up to `Fuzz` fresh attempts from `Seed` (random when `!SeedSet`, using `crypto/rand`), stop at first failure; `Reject` attempts do not count as runs and total attempts are capped at `10 × Fuzz`; filter health check after 50 attempts. Shrink with `fuzz.Shrink(seq, fails, 1000)` unless `Mutating`. Each attempt of a `Task` property calls the existing `runCase` setup (fresh `:memory:` store, migrations, test tables, caps clone, clock, silo, browser) — factor that setup out of `runCase` into `newCaseEnv(o Options) (*eff.Env, func())` and call it per attempt. Time each attempt; > 5 s → FAIL. A Go panic inside an attempt is a failure and shrinks like one. JSON: add `property: {runs, seed, run, shrinks, args, case}` only when `Prop != nil`.
- [ ] **Step 4: Run** `go test ./rt/testing ./cmd/alder -run 'Prop|Test_'` — expect PASS; existing `TestTest_Todos` still reports `116 passed`.
- [ ] **Step 5: Measure.** Run `TestProp_EffectIsolation` with `-bench`-style timing (`go test -run EffectIsolation -v`); record ms per attempt in the commit message. If > 20 ms per attempt, switch effect attempts to one store per property with a savepoint per attempt (`Inner`, `testing.go:78-135`) and re-run Step 4.
- [ ] **Step 6: Commit** `feat(rt/testing): property loop, shrinking, health checks, saved failures (#22)`.

---

### Task 7: Paste-ready case, CLI flags, `--mutate`, crash seed

**Files:**
- Modify: `internal/ir/fuzz.go` + `internal/gen/fuzz.go` (emit `Property.Case`), `cmd/alder/test.go` (`testUsage` 26, `runTestTo` 103-140, env 255-266, crash 244-301), `cmd/alder/helpjson.go:133-137`, `cmd/alder/help.go`, `testdata/help.json`, `rt/testing/mutate.go` (pass `Mutating`), `internal/mutate/mutate.go` (skip Fuzzer-typed values)
- Test: `internal/gen/gen_test.go` `TestGen_PasteCase*`, `cmd/alder/test_test.go`, `rt/testing/mutate_test.go`

**Interfaces:**
- Consumes: `Property.Case` slot and `Options` fields (Task 6); `check.Result.Binds` (`result.go:11-15`); `internal/format`'s expression printer.

- [ ] **Step 1: Write failing tests.**
  - `TestGen_PasteCaseLambda`: for `test "reverse twice" = \xs -> List.reverse (List.reverse xs) == xs`, `Case([]string{"[0, -1]"})` returns `test "reverse twice (case)" = List.reverse (List.reverse [0, -1]) == [0, -1]` (spec §6 verbatim).
  - `TestGen_PasteCaseForAll`: for a `Fuzz.forAll g (\xs -> P xs)` body the `forAll` and generator are dropped; same substitution.
  - `TestGen_PasteCaseBlock`: a block body substitutes inside the block.
  - `TestGen_PasteCaseShadow` (**Review Focus 3**): `\x -> (\x -> x) 1 == x` with `x = 5` gives `(\x -> x) 1 == 5`.
  - `TestGen_PasteCaseLong`: a case over 20 lines makes the report print `paste the values above into a plain test` and no `to keep this case:` block.
  - `TestTest_PropertyFlags`: `alder test --fuzz 5 --seed 3` on a passing property app runs; a failing property's report contains `seed: 3`; `alder test --help` lists `--fuzz` and `--seed` and says `--seed` does not override saved failures.
  - `TestTest_PropertyDB`: after a failing run, `build/todos/fuzz/Table.Todo/` holds one file; it is under the scaffold's ignored `build/`.
  - `mutate_test.go` `TestMutate_Property`: a property that kills a mutant counts as a kill; two `--mutate` runs give the same score; no `fuzz/` directory is written.
  - `TestTest_PropertyFatalCrash`: a property whose body recurses without bound reports the crash plus `seed:` and `run:` lines.
- [ ] **Step 2: Run** `go test ./internal/gen ./cmd/alder ./rt/testing -run 'Paste|Property|Mutate_Property'` — expect FAIL.
- [ ] **Step 3: Implement.** Paste case: at lowering, print the body with `internal/format` after replacing every `*syntax.Var` whose binding (`Result.Binds`) is a property parameter with a placeholder `\x00N\x00`; gen emits a Go string template; `Case` fills the literals and prefixes `test "<name> (case)" = `. CLI: parse `--fuzz`/`--seed`, set the three env vars (`ALDER_TEST_FUZZ_DB` = `build/<app>/fuzz` via `appBuildDir`). Before each attempt write `Module\tName\tseed\trun` to the status file; `crashFilter` prints `seed:`/`run:` from it.
- [ ] **Step 4: Run** `go test ./...` (full suite, under a 10-minute timeout) — expect PASS.
- [ ] **Step 5: Commit** `feat(test): paste-ready cases, --fuzz/--seed, mutate and crash handling (#22)`.

---

### Task 8: Book and help

**Files:**
- Modify: `book/10-testing.md` (new `## Properties` section after the running-tests section; a note in the mutation section), `book/09-*.md` (stdlib list: `Fuzz`), `book/12-*.md` (rationale paragraph), `book/13-*.md` (`Fuzz.any` resolution, generators built in lowering, the shrinker), `book/B-pitfalls.md` (one entry: an unannotated argument the checker defaulted to `Int` when you meant another type)
- Test: `book/book_test.go` (existing harness)

- [ ] **Step 1: Write the sections** in the book's teaching style (memory: the book explains internals for a beginner). Chapter 10 teaches, in order: the function form, derived types and `Int` defaulting, annotations, brands as constraints, `Fuzz.forAll` and shared `Fuzzer` helpers, reading a failure (use the §6 report verbatim), pasting the case, `--seed`/`--fuzz`, saved failures in `build/<app>/fuzz/`, why no shrinkers. Each runnable example uses the book's checked fenced-block format; include one `alder error=E0038` and one `alder error=E0039` block.
- [ ] **Step 2: Run** `go test ./book/...` — expect PASS.
- [ ] **Step 3: Commit** `docs(book): properties (#22)`.
