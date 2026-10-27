# Design: property-based testing (alder #22)

Status: design approved in conversation, 2026-10-26. Read-only survey of `~/projects/alder`; nothing else edited. Builder check done 2026-10-26 evening; its decisions are in §0a.

## 0. Decisions (2026-10-26)

- **Both forms.** A `test` whose body is a function is a property with generators **derived from the argument types** (the headline); `Fuzz.forAll gen f` is the explicit form for when the types don't say enough. `Fuzz.any : Fuzzer a` is the derived generator as a value, so the two compose.
- **Shrinking is internal** (Hypothesis / minithesis / elm-test 2.0): the runner minimises the recorded sequence of random choices. No shrinkers in user code, ever.
- **Failures are remembered locally**: shrunk choice sequences are saved in `.alder/fuzz/` (gitignored) and replayed first on the next run. The permanent record of a found bug is the **paste-ready plain `test`** printed in the failure report. No committed regressions file.
- **Effectful properties are in v1**: a function returning `Task Bool` runs each case in its own rolled-back transaction, like today's effect tests.
- **#7 (pure seeded `Random`) is no longer a blocker.** The engine has its own PRNG in Go. `Fuzz.fromRandom` may come later with weak shrinking.
- v1 non-goals: model-based testing of pages, `Fuzz.cover` / statistics, regex-driven brand generation, per-test run counts.

## 0a. Builder-check decisions (2026-10-26, evening)

- **Lambda parameter annotations, in every lambda** (user decision). `\(x : T) y (z : U) -> …` is new grammar: a parameter is a bare name or a parenthesised `name : Type`. Parser, AST, checker, formatter, tree-sitter and `book/A-grammar.md` change. §8's "Grammar: no change" is superseded.
- **`Fuzzer` values may live anywhere** (user decision): a top-level `slugs : Fuzz.Fuzzer Slug` is legal in any module, so generators are shared and typed by a signature. `Fuzz.forAll`, `Fuzz.forAllTask` and `Property` are legal only as a test body (new E-code). Fuzzer-typed values get no mutants.
- Saved failures live in `build/<app>/fuzz/<Module>/<sha1(test name)[:16]>` (already gitignored), not `.alder/fuzz/`.
- Effect properties repeat today's per-case isolation per attempt (fresh in-memory SQLite, migrations, fakes); there is no rolled-back transaction today. Fall back to a savepoint per attempt only if measurement says so.
- The six stdlib brands (`Text.NonEmpty`, `Text.Email`, `Text.Url`, `Text.Secret`, `Int.Positive`, `Int.NonNegative`) get dedicated generators; filtering random text would trip the health check.
- The "too slow" check is new (no per-test timeout exists): an attempt over 5 s fails the property.
- Argument lines print `xs: [0, -1]`; the type is added only when it was defaulted.
- Generated Text is printable code points plus `\n \t \r`, so every value prints as a literal. Values print with a typed printer (brands spelled out, `Time.fromMillis N`), not `rt.Show`.
- Under `--mutate`: fixed seed, no shrinking, no saved failures.
- A fatal crash (stack overflow) prints the seed and run, not the arguments.
- New codes: E0038 (argument or `Fuzz.any` type not derivable or not concrete) and E0039 (`Property` outside a test body).

## 1. Intent

The user is a property-testing enthusiast; there are no concrete nightvote-replay cases yet, the ask is "easy, native property testing" built on what the beloved frameworks got right. "Native" means: no new runner, no new declaration form, no library to install — a property is a `test` and `alder test` runs it. The AI-optimised angle: the shortest possible way to state a property (`\xs -> ...`) and a failure report that contains the fix-it-forever test verbatim.

Prior art surveyed: Hypothesis (internal shrinking, example database, `from_type`, health checks), elm-test 2.0 (the same engine in an Elm-like language without type classes), QuickCheck (type-directed generators; separate `shrink` is the hated part), Hedgehog (state-machine testing), proptest (committed regressions file), Go fuzzing (committed corpus), fast-check (seed/path replay).

## 2. Goals, non-goals, anti-goals

Goals
- A property is as short as its statement. Derived generators for every ordinary type, brands included.
- Shrunk, minimal counterexamples, printed as Alder literals.
- Reproducible: seed printed, `--seed` replays, failures replayed automatically until fixed.
- Properties over `Db` code, with the same isolation as effect tests.
- Explicit generators that compose with derived ones.

Non-goals (v1): see §0. Also: coverage-guided fuzzing; generating functions as test arguments; a `Property` type that is a first-class value anywhere other than a test body.

Anti-goals
- Users writing shrinkers. (QuickCheck's mistake.)
- A machine-written file in the repo that rots when tests are renamed. (Why no committed regressions file.)
- "Unfair" failures that teach people to distrust the tool: the health checks (§5) exist so an over-tight brand fails with an explanation, not a mystery.

## 3. Surface

### 3.1 Three kinds of test body

| Body type | Kind | Runs |
|---|---|---|
| `Bool` / `Task Bool` | plain (today) | once |
| `a -> ... -> Bool` or `a -> ... -> Task Bool` | property, derived generators | 100 × |
| `Property` | property, explicit generators | 100 × |

```alder
test "reverse twice" = \xs -> List.reverse (List.reverse xs) == xs
-- xs : List Int by default when unconstrained (§4.4); annotate to choose
test "a slug round-trips" = \(s : Slug) -> Slug.parse (Slug.toText s) == Just s
test "matches never crashes" = \(f : Filter) (t : Todo) -> Todo.matches f t || True

test "insert then get" = \(title : Text.NonEmpty) -> {
  let me <- Db.insert User { name: "Me" }
  let id <- Db.insert Todo { title: title, due: Nothing, owner: me }
  let t <- Db.get Todo id
  Task.succeed (t.title == title)
}

test "sorted lists stay sorted" =
  Fuzz.forAll (Fuzz.map List.sort (Fuzz.list Fuzz.int)) (\xs -> List.sort xs == xs)
```

Rules
- Every argument's type must be known after inference and derivable (§4). Otherwise E-code (§6) with the suggestion to annotate or use `Fuzz.forAll`.
- The function's arguments are the only inputs; a property may still use any brand constructor, as tests may.
- A property in a block uses the block's `Task` as today; `Fuzz` is not a monad you bind in a block (no `let x <- Fuzz.int` — keeps blocks single-effect).

### 3.2 The `Fuzz` module (`stdlib/Fuzz.alder`)

```alder
module Fuzz

-- a description of how to draw a value of type a from random choices
type Fuzzer a

-- the generator alder test derives from the type (§4); the explicit form of a derived argument
any : Fuzzer a                 -- special form: resolved by the compiler at the use site's type
forAll : Fuzzer a -> (a -> Bool) -> Property
forAllTask : Fuzzer a -> (a -> Task Bool) -> Property

constant : a -> Fuzzer a
bool : Fuzzer Bool
int : Fuzzer Int                -- mostly small, sometimes extreme
intRange : { min: Int, max: Int } -> Fuzzer Int
float : Fuzzer Float            -- finite; 0, -0, tiny, huge included; no NaN/Inf
floatRange : { min: Float, max: Float } -> Fuzzer Float
text : Fuzzer Text              -- mostly short ASCII, sometimes Unicode, sometimes empty
textOf : { min: Int, max: Int } -> Fuzzer Text   -- length
char : Fuzzer Text              -- one code point
list : Fuzzer a -> Fuzzer (List a)               -- size biased small
listOf : { min: Int, max: Int } -> Fuzzer a -> Fuzzer (List a)
maybe : Fuzzer a -> Fuzzer (Maybe a)
result : Fuzzer e -> Fuzzer a -> Fuzzer (Result e a)
pair : Fuzzer a -> Fuzzer b -> Fuzzer { first: a, second: b }
dict : Fuzzer k -> Fuzzer v -> Fuzzer (Dict k v)
oneOf : List (Fuzzer a) -> Fuzzer a
oneOfValues : List a -> Fuzzer a
frequency : List { weight: Int, fuzzer: Fuzzer a } -> Fuzzer a
map : (a -> b) -> Fuzzer a -> Fuzzer b
map2 : (a -> b -> c) -> Fuzzer a -> Fuzzer b -> Fuzzer c
andThen : (a -> Fuzzer b) -> Fuzzer a -> Fuzzer b
filter : (a -> Bool) -> Fuzzer a -> Fuzzer a     -- subject to the health check (§5)
```

Multiple explicit arguments: `Fuzz.pair` / `Fuzz.map2` into a record, or nest `forAll`. No `forAll2`/`forAll3` family (Elm needs it because it has no derivation; here `Fuzz.any` and records cover it).

`Property` is a type the test checker accepts as a body and nothing else can do anything with. `Fuzzer a` is opaque; all values come from this module. Both live in `Fuzz` (`Fuzz.Property`); `Test.alder` is left as the browser-step module.

### 3.3 CLI

- `alder test --fuzz N` — runs per property (default 100).
- `alder test --seed S` — the PRNG seed for fresh draws (default: random, printed on failure).
- `-run`, `--mutate`: unchanged; a property counts as one test. Under `--mutate` a property that kills a mutant counts the same as a plain test.

## 4. Derivation (the compiler's part)

`Fuzz.any` at type `T`, and each argument of a function-bodied test, is resolved at compile time to a generator for `T`:

| Type | Generator |
|---|---|
| `Int` | `Fuzz.int` |
| `Float` | `Fuzz.float` |
| `Bool` | `Fuzz.bool` |
| `Text` | `Fuzz.text` |
| `Unit` | `constant ()` |
| `Bytes` | short byte strings, biased small |
| `Date`, `Time` | within a few years of the epoch, biased near it; plus epoch itself |
| `Maybe a`, `Result e a`, `List a`, `Dict k v` | the combinators over derived parts |
| record | each field derived |
| union | `oneOf` of the constructors, each payload derived; recursive unions get a depth budget (my call: default depth 4, each level halves the chance of a recursive constructor) |
| brand `T where p` | draw `T`, keep values where `p` holds (§5 health check). Brands without `where`: draw `T` and wrap. |
| `Html`, `Doc`, `Page`, `Form f`, capabilities, functions, `Task`, `Query` | **not derivable**: E-code, suggest `Fuzz.forAll` |
| a type variable left unconstrained | **defaults to `Int`** (§4.4) |

4.1 Brands are derived even where the module forbids calling the constructor; tests may already.

4.2 `Fuzz.any` is a special form like `Test.fake`: the checker records the instantiated type at each use and the compiler emits the generator. Inside a polymorphic helper it is an error (the type must be concrete at the use site).

4.3 Distributions are chosen to find bugs, not to be uniform: small ints and short lists most of the time, extremes some of the time. Exact shapes are the builder's, within these intents. Document them in the book, not in the stdlib signatures.

4.4 An unannotated `\xs -> List.reverse (List.reverse xs) == xs` leaves the element type free. Rather than refuse, the checker defaults free type variables in a property's arguments to `Int` and the report prints the argument as `xs : List Int` so the choice is visible. Annotate to pick another.

## 5. Health checks

Done per test, during the run, failing the test with an explanation rather than passing vacuously or hanging:

- **Filter too tight**: a `Fuzz.filter` or brand predicate rejecting more than 90% of draws over the first 50 attempts → FAIL, "`Slug`'s predicate rejects 97% of generated Text; write a `Fuzz.forAll` with a generator that produces slugs". Same for a derived brand.
- **Too slow**: a single case over 5 s (reuse the existing test timeout if there is one) → FAIL naming the case.
- **Generator too large**: a derived recursive type hitting the depth budget is fine (it stops), a `Fuzz.andThen` chain producing more than 64 KiB of choices → FAIL, "generator draws too much".

## 6. Reporting

Pass line as today: `ok   Todo "reverse twice"`. On failure:

```text
FAIL Todo "reverse twice"  (failed on run 37 of 100, shrunk in 12 steps)
  xs: [0, -1]
  left:  [-1, 0]
  right: [0, -1]
  seed: 8812634501
  to keep this case:
    test "reverse twice (case)" = List.reverse (List.reverse [0, -1]) == [0, -1]
```

- One line per argument, named as in the lambda, as an Alder literal (brand constructors spelled out, `Time.epoch`-relative times as the stdlib prints them).
- `left:`/`right:` as the plain-test reporter already does for `==`; a non-`==` body prints nothing more.
- A crash inside a case reports as a crash does today, plus the arguments and the seed.
- The paste-ready test is the property's body with the lambda removed and each argument substituted; for a `Fuzz.forAll` body the same, with `forAll` dropped. For a block body the substitution is on the block. If the literal would be very long (my call: over 20 lines), print the arguments only and say "paste the values above into a plain test".
- Diagnostics (E-codes, numbers assigned by the builder in sequence after the current last): property argument type not derivable; `Fuzz.any` at a non-concrete type; function-bodied test with zero arguments is just a plain test whose body is a function — E-code "a test body may not be a function of no arguments" is unnecessary since that can't be written; skip.

## 7. Engine (runtime, Go)

- `Fuzzer a` compiles to a Go value: a function `func(*choices) a`. `choices` holds the PRNG, the recorded draw list, and in replay/shrink mode the sequence being replayed (drawing past the end of a replayed sequence ends that attempt as "overrun", which counts as neither pass nor fail).
- Primitive draws: `drawBits(n)`, `drawInt(min,max)`, `drawFloat`, `drawBool(p)`. Everything in `Fuzz` and every derived generator is built from these. Choices are ordered so that *smaller is simpler*: lists draw a continue/stop bit per element (stop = 0), ints draw magnitude then sign, unions draw the constructor index with the non-recursive/first constructors lowest.
- Run loop per property: replay each saved sequence from `.alder/fuzz/<module>/<test-name-hash>` first; then up to N fresh draws from the seed; stop at the first failure.
- Shrink loop (minithesis): repeat until no pass changes the sequence — delete a block of choices, zero a block, lower a choice toward 0, swap adjacent to sort, binary-search a single choice. Each candidate re-runs the test; a candidate counts only if it still fails. Bound by a step budget (my call: 1000 attempts) so shrinking cannot hang.
- Effectful property: each attempt (fresh, replay, shrink candidate) opens its own transaction and rolls it back; the test's `Test.fake`s and session apply to every attempt.
- Saved failures: after shrinking, write the sequence as bytes keyed by module + test name; delete the key when the property passes a full run. Location: the survey found no `.alder/` convention — the build cache is `build/<app>/` (see `internal/scratch`). Builder's call: put the database beside the build cache (`build/<app>/fuzz/`, already ignored) and keep the `.alder/fuzz/` name in this doc as the user-facing description only if a per-project directory is chosen instead; either way it is gitignored by default.
- Determinism: with `--seed S` and no saved failures the run is fully reproducible. Saved failures are tried regardless of seed.

## 8. Book and tooling

- Chapter 10 gains a section "Properties" between pure tests and the report, teaching: the function form, what derived types mean, brands as constraints, `Fuzz.forAll`, reading a failure, pasting the case, `--seed`/`--fuzz`, and why no shrinkers. Chapter 9 (stdlib) lists `Fuzz`. Chapter 13 (compiler) explains `Fuzz.any` resolution. Chapter 12 (rationale) gets the "why both forms, why internal shrinking, why not a committed file" paragraph.
- Grammar: lambda parameters may be annotated, `\(x : T)` (§0a).
- LSP/nvim: nothing new required; test running already works through `alder test`.
- `alder.nvim`: none in v1.

## 9. Left to the builder

- Exact distributions (§4.3), depth budget, shrink step budget, health-check thresholds — all marked "my call" above and may move.
- Where in `internal/` the engine lives and how `Fuzzer` values are represented in generated Go.
- E-code numbers and wording, matching the house style in `book/B-pitfalls.md`.
- Whether `Property` needs to be in `Prelude` (probably not: `Fuzz.forAll` returns it, the checker recognises it).

Builder check (a fresh reviewer reading this as the implementer) is scheduled for after 17:00 ET on 2026-10-26 per the working-hours rule; its questions, if any, come back to the user in one message before the plan is written.

## 10. Later (not v1, leave room)

- Model-based testing of pages: derive `Fuzzer Msg`, generate message sequences, run them through `update` and the in-memory browser, check invariants on the model/page. Alder's `update`-is-a-function shape makes this unusually cheap; the engine's choice sequence already shrinks sequences.
- `Fuzz.cover` / `Fuzz.label` statistics in the report.
- Regex-driven generation for `brand Text where Text.matches "..."` and range-driven for Int predicates, replacing filter for those shapes.
- Per-test run counts (`Fuzz.runs 1000 property`).
- `Fuzz.fromRandom : Random.Generator a -> Fuzzer a` once #7 lands (shrinks only the seed).
- Committing `.alder/fuzz/` as a project choice, documented.
