# Plan: `Text.hash` and a pure seeded `Random` (#7)

Spec: `docs/superpowers/specs/2026-10-29-pure-random-design.md` (binding). Branch
`issue-7-pure-random`. Each task is test-first (red, code, green) and one commit.

Before task 1: every vector in the spec was re-derived with a scratch Go program (SplitMix64
rows for seeds 0, 1, 42; the three `Text.hash` values; `intRange 1..6` from 42 with its next
seed; the five dice from 1; `floatRange` from 42). All match. The `normal` vector is checked
in task 3 against the implementation.

## Task 1: `Text.hash`

- Test `rt/std/text_hash_test.go` (new file; `text_test.go` is shared with other branches):
  - `TestTextHash_Vectors`: `""`, `"a"`, `"héllo"` give the spec's three values.
  - `TestTextHash_NonNegative`: a thousand short texts (`Text.fromInt i` and a suffix) all `>= 0`.
  - `TestTextHash_UTF8`: `"héllo" != "hello"`, and equals FNV-1a over `[]byte("héllo")`.
- Code: `rt/std/text.go` `TextHash` (`hash/fnv` `New64a`, `& 0x7FFF…`); `stdlib/Text.alder`
  `hash : Text -> Int` with its comment; `internal/gen/stdcall.go` `"Text.hash": "std.TextHash"`.
- `TestStdCallsCoverStdlib` stays green.

## Task 2: `Random` core

- Test `rt/std/random_test.go`:
  - `TestSplitMix64_Vectors`: the three rows (first three outputs from seed 0, 1, 42).
  - `TestRandomSeed_Show`: `rt.Show(RandomSeed(-5))` is `Random.seed -5`, as an argument
    `Just (Random.seed -5)`; the state of `RandomSeed(n)` is `uint64(n)`.
  - `TestRandomSeed_Eq`: two seeds with equal states are `rt.Eq`; different are not.
  - `TestRandomIntRange_Vectors`: 1..6 from 42 is `{2, Random.seed -7046029254386353089}`;
    full range from 0 is `-2152535657050944081`; `list 5 (1..6)` from 1 is `[6,2,1,6,4]`.
  - `TestRandomFloatRange_Vector`: 0..1 from 42 is `0.74156487877182342`.
  - `TestRandomDegenerate`: `intRange {5,5}`, `{5,1}`, `floatRange {2,2}`, `{2,1}`, NaN bounds
    return `min` and the seed advanced by one step.
  - `TestRandomConstantNoStep`, `TestRandomListNonPositive` (`0`, `-3`: `[]`, seed unchanged).
  - `TestRandomMap2AndThenOrder`: `map2 pair a b` equals stepping `a` then `b` by hand;
    `andThen` passes the first's seed to the second.
  - `TestRandomDice`: 60 000 draws of 1..6 from seed 3 within 3% of 10 000 per face.
- Code: new `rt/std/random.go`: `Seed{State uint64}` with `AlderShow`, `splitmix64`,
  `Generator[A] func(Seed) (A, Seed)`, `unit`, `RandomSeed`, `RandomStep`, `RandomConstant`,
  `RandomIntRange`, `RandomFloatRange`, `RandomList`, `RandomMap`, `RandomMap2`,
  `RandomAndThen`. `stdlib/Random.alder` types and signatures; `stdcall.go` entries;
  `internal/gen/types.go` `GoType`: `Con{Seed, Random}` → `std.Seed`, `App{Generator}` →
  `std.Generator[A]` (and the doc table).

## Task 3: `normal`, `uniform`, `weighted`

- Tests in `rt/std/random_test.go`:
  - `TestAcklam`: `q 0.5 == 0`, `q 0.975` within 1e-6 of 1.959964, `q 0.01` of -2.326348.
  - `TestRandomNormal_Vector`: standard normal from 42 is `0.6481773615835269`.
  - `TestRandomNormal_Moments`: 100 000 from seed 7: mean within 0.01 of 0, sd within 0.01 of 1.
  - `TestRandomUniform`: `uniform x []` is `x` and steps once; picks index `intRange 0..len xs`.
  - `TestRandomWeighted`: negative weight counts as its absolute value; all-zero gives the
    first; a total-overshooting `t` (rounding) gives the last (tested on `pickWeighted` with
    `unit` 1.0 against weights whose sum rounds low); one step.
- Code: `RandomNormal`, `acklam`, `RandomUniform`, `RandomWeighted`; stdlib lines; stdcall.

## Task 4: checker — generators are functions

- Test `internal/check/random_test.go` (new file, `checkWith` + `InferModule`):
  - `TestRandomTypesCheck`: `g : Random.Generator Int` and `s : Random.Seed` signatures check;
    `Random.step g s` has type `{ value: Int, seed: Random.Seed }`.
  - `TestRandomBareSeedIsE0202`: `s : Seed` is E0202 with fix `Random.Seed`.
  - `TestGeneratorEqRefused`: `g == g` on a `Random.Generator Int` and a `Fuzz.Fuzzer Int`
    is "functions cannot be compared".
  - `TestFuzzAnyAtSeedIsE0038`: `test "t" = \(s : Random.Seed) -> True` is E0038.
- Code: `internal/check/expr.go` `containsFunIn`: `Generator`, `Fuzzer` like `Query`/`Task`
  (they are not effects, so the "functions" message is the one given).
- Corpus: `testdata/corpus/random-ok` (a page whose `load` uses `Random.step` and
  `Text.hash` in a `Query`; empty `expected.diag`), and `internal/gen/random_test.go`
  `TestGen_RandomCorpus` emits and vets (builds) it.

## Task 5: `Fuzz.fromRandom`

- Tests `rt/fuzz/random_test.go`:
  - `TestFromRandom_ZeroIsSeedZero`: `Replay([]uint64{0})` gives the generator's value at
    `Random.seed 0`, one choice recorded.
  - `TestFromRandom_Shrinks`: `fromRandom (intRange 0..100)` with `n < 50` fails and shrinks
    to a seed whose value is `>= 50`; compared with a direct scan of seeds from 0.
- Code: `rt/fuzz/combinators.go` `FromRandom` (`c.Choice(math.MaxUint64)` as the state);
  `stdlib/Fuzz.alder` `fromRandom : Random.Generator a -> Fuzzer a`; stdcall.
- Checker test in `internal/check/random_test.go`: `Fuzz.fromRandom (Random.intRange {…})`
  has type `Fuzz.Fuzzer Int`.

## Task 6: book and site

- `book/09-stdlib.md`: `Text.hash` in the Text block plus a sentence; new `## Random` section
  before "Query, Task, …": the pure block, the step rule, SplitMix64 / FNV-1a / Acklam named,
  the seed idiom, an `eval` fence of `Random.step (Random.intRange { min: 1, max: 6 })
  (Random.seed 42)`; `Fuzz.fromRandom` in the Fuzz block with its shrinking caveat.
- `book/07-effects.md`: one sentence after the capability block pointing at chapter 9.
- `python3 book/site/build.py`; `go test ./book/`; `cd experiments && python3 -m unittest`
  (raise the token cap with a `#7` comment if exceeded).

## Finish

- `go vet ./...`, `go test ./...` (foreground, 10-minute timeout).
- Scratch check of `Utils.Rng`/`Utils.Curve` rewritten on the new API (outside the repo).
- Rebase on origin/main, push, open the PR.
