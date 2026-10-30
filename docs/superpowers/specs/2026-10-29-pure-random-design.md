# Deterministic randomness: `Text.hash` and a pure seeded `Random`

Design for [tapegram/alder#7](https://github.com/tapegram/alder/issues/7). nightvote-replay needed
reproducible "random" values in `load` (a seeded election night: report times, partial counts,
fault injection) and hand-rolled them, because `Random.int` is a `Task` and the language has no
hash. Everything marked **(call)** is the author's decision, not the user's.

## Intent

`app/Utils/Rng.alder` in nightvote-replay is 60 lines of workaround: a printable-ASCII `code`
table, a polynomial hash modulo 2^31 − 1, a MINSTD mixer, a squaring `scramble` (because hash and
mixer are affine and neighbouring inputs stayed ordered), and Acklam's inverse normal CDF with its
coefficients spelled out. Every call site is the same shape: hash a list of text parts to an `Int`,
then draw a uniform float, a normal, or `scramble h % n`. After this change that is
`Text.hash` to make a seed and `Random.step` with a generator, and the numbers are the same on
every platform and in every future Alder version.

## Goals

1. `Text.hash : Text -> Int`, stable forever, documented as not cryptographic.
2. A pure, Elm-shaped `Random`: `Seed`, `Generator a`, `seed`, `step`, and the smallest set of
   generators and combinators that replaces `Utils.Rng` and `Utils.Curve`'s draws.
3. The algorithms are pinned by name and test vectors, so a seeded run reproduces across versions.
4. The existing `Random.int : { min, max } -> Task Int` and `Random.id : Task Text` keep working
   unchanged, as does every program that uses them.
5. `Fuzz.fromRandom`, the hook the property-testing spec left for #7, ships in the same change.

## Non-goals

- Bitwise operators, `Char`, or a cryptographic hash. `Text.hash` is for seeds, bucketing and
  cache keys, and the book says so.
- `Random.independentSeed`, `Random.lazy`, `map3`+, `pair`, `Random.bool`, a `Dict`/`Maybe`
  generator, shuffling. Each is one `map`/`andThen` away; add when a project asks.
- A `Task` that yields a `Seed` (`Random.int` already yields an `Int` to seed from).
- Renaming the effectful functions. They stay as they are; see "Coexistence".

## Anti-goals

- A name that means something different from its Elm namesake (the stdlib-batch rule). So no
  pure `Random.int`/`Random.float` beside the `Task` one: the generators take the names `Fuzz`
  already uses for the same shapes.
- A generator whose step count depends on the value drawn (a rejection loop): a seed's sequence
  must be the same every time, and the number of steps a generator takes must be statable.

## Coexistence **(call)**

One module. `Random` keeps `int` and `id` (effects) and gains the pure API below. The pure
generators for ranges are named `intRange`/`floatRange`, mirroring `Fuzz.intRange`/`Fuzz.floatRange`,
so `Random.int` stays the one `Task` and nothing is renamed. A separate module (`Seeded`, `Gen`)
was rejected: Elm readers look in `Random`, and the two halves share one book section.

## The API

Exact lines for `stdlib/Random.alder` (after `int` and `id`), `stdlib/Text.alder` and
`stdlib/Fuzz.alder`. Types of a stdlib module other than Prelude are written qualified
(`Random.Seed`, `Random.Generator Int`), as `Fuzz.Fuzzer` and `Http.HttpError` are.

```alder
-- Random: pure, seeded randomness (the Task-returning int and id are above)
type Seed
type Generator a
seed       : Int -> Seed
step       : Generator a -> Seed -> { value: a, seed: Seed }
constant   : a -> Generator a
intRange   : { min: Int, max: Int } -> Generator Int
floatRange : { min: Float, max: Float } -> Generator Float
normal     : { mean: Float, sd: Float } -> Generator Float
uniform    : a -> List a -> Generator a                    -- Random.uniform first rest
weighted   : { weight: Float, value: a } -> List { weight: Float, value: a } -> Generator a
list       : Int -> Generator a -> Generator (List a)
map        : (a -> b) -> Generator a -> Generator b
map2       : (a -> b -> c) -> Generator a -> Generator b -> Generator c
andThen    : (a -> Generator b) -> Generator a -> Generator b

-- Text
hash : Text -> Int   -- stable across versions and platforms; 0 <= hash; not cryptographic

-- Fuzz
fromRandom : Random.Generator a -> Fuzzer a   -- draws a seed; shrinks the seed toward 0
```

### Semantics

**`Text.hash`** is FNV-1a, 64-bit, over the text's UTF-8 bytes (offset basis
`14695981039346656037`, prime `1099511628211`), with the top bit cleared so the result is in
`[0, 2^63)` **(call)**: a non-negative hash makes `hash s % n` a bucket, never a negative
surprise. Vectors: `Text.hash "" == 5472609002491880229`, `Text.hash "a" == 3414815163700866188`,
`Text.hash "héllo" == 2549027629147767008`. Go: `hash/fnv` `New64a`, `& 0x7FFFFFFFFFFFFFFF`.

**`Seed`** is one 64-bit state; `Random.seed n` is the state `n` as an unsigned 64-bit value
(two's complement), so every `Int` is a valid seed, including `0` and negatives. A `Seed` prints
(`rt.Show`, the REPL, test failures) as `Random.seed N` with `N` the state as an `Int`, which
re-creates it; two seeds are `==` when their states are.

**The PRNG is SplitMix64** (Steele, Lea, Flood 2014) **(call)**: one step is
`state += 0x9E3779B97F4A7C15; z = state; z = (z ^ (z >> 30)) * 0xBF58476D1CE4E5B9;
z = (z ^ (z >> 27)) * 0x94D049BB133111EB; output = z ^ (z >> 31)`, all in `uint64`. Chosen over
Elm's PCG-32 because one step yields 64 bits (a `Float` needs 53), the seeding is the identity, it
is ten lines with no 128-bit arithmetic, and it is the standard splittable PRNG should
`independentSeed` come later. Vectors (seed = state, first three outputs, hex / as `Int`):

| seed | outputs |
|---|---|
| `0` | `e220a8397b1dcdaf` (−2152535657050944081), `6e789e6aa1b965f4`, `06c45d188009454f` |
| `1` | `910a2dec89025cc1`, `beeb8da1658eec67`, `f893a2eefb32555e` |
| `42` | `bdd732262feb6e95`, `28efe333b266f103`, `47526757130f9f52` |

The seed-0 row is the published reference sequence; the builder confirms all three rows against
a transcription of Vigna's `splitmix64.c` before trusting the table.

**Steps.** Every primitive generator (`intRange`, `floatRange`, `normal`, `uniform`, `weighted`)
takes exactly one step, degenerate inputs included; `constant` takes none; `list n g` takes
`n` steps of `g` in order (`n <= 0` gives `[]` and no step); `map` adds none; `map2 f a b` runs
`a` then `b`; `andThen f a` runs `a`, then the generator `f` returns. `step g s` returns the
value and the seed after those steps. So a value's position in the stream is a statement about
the program, not about the values drawn.

**The unit float** of an output `o` is `(Float (o >> 11) + 0.5) * 2^-53`: 53 bits, strictly in
`(0, 1)` **(call)** (never `0`, so a log of it is finite). `floatRange { min, max }` is
`min + (max - min) * unit`; when `max <= min` (or either is NaN) it is `min`.

**`intRange { min, max }`** is `min` when `max <= min`; else with `span = max - min` as
`uint64`, it is `output` reinterpreted as `Int` when `span` is `2^64 - 1`, otherwise
`min + output % (span + 1)` **(call)**: a modulo bias below 2^-40 for any range under 2^24 is
nothing against a fixed one step per draw. Vectors from the table: `Random.step (Random.intRange
{ min: 1, max: 6 }) (Random.seed 42)` is `{ value: 2, seed: Random.seed -7046029254386353089 }`;
`Random.list 5 (Random.intRange { min: 1, max: 6 })` from `Random.seed 1` is `[6, 2, 1, 6, 4]`;
`floatRange { min: 0.0, max: 1.0 }` from seed `42` is `0.74156487877182342`.

**`normal { mean, sd }`** is `mean + sd * q(unit)` where `q` is Acklam's rational
approximation of the inverse normal CDF (2003; central region for `0.02425 <= p <= 0.97575`,
tail formula outside, no refinement step; the coefficients are those in nightvote-replay's
`Utils.Rng`, which are the published ones) **(call)** over Box–Muller: one step, monotone in the
unit float, relative error 1.15e-9. Vectors: `q 0.5 == 0.0`, `q 0.975` within 1e-6 of
`1.959964`, `q 0.01` of `-2.326348`; `normal { mean: 0.0, sd: 1.0 }` from seed `42` is
`0.6481773615835267`. Every product in `q`, `normal` and `floatRange` is rounded before it is
added (no fused multiply-add, which Go uses on arm64 but not amd64), so the floats are the same
on every platform; this vector is the unfused IEEE value (a Python check agrees), corrected by
the builder from `…5269`, which was the fused arm64 result.

**`uniform x xs`** picks index `intRange { min: 0, max: List.length xs }` of `x :: xs`.
**`weighted w ws`** (Elm's) uses the absolute values of the weights: `t = unit * total`; walk
`w :: ws` in order and take the first item with `t <= |weight|`, else subtract `|weight|` from
`t`; the last item if none (rounding); all weights zero gives the first. Weights are `Float`
as in Elm; a negative weight counts as its absolute value, not an error.

**`Fuzz.fromRandom g`** draws one choice in `[0, 2^64 - 1]`, makes it the seed's state, and
returns `(step g seed).value`. Shrinking lowers that one choice, so the minimal case is the value
`g` gives at `Random.seed 0`; the engine cannot shrink inside `g` and the book says so. No health
check applies. The generator type is written `Random.Generator a` inside `stdlib/Fuzz.alder`;
the names resolver already resolves a stdlib module's qualified types in stdlib signatures
(`importedScope` falls through to `isStd`), which the builder confirms with the checker test below.

**Comparing generators.** `==` on a `Random.Generator a` or a `Fuzz.Fuzzer a` is refused by the
checker as "functions cannot be compared" (today `containsFun` does not see through either opaque
type, and `rt.Eq` on a Go `func` is silently `False`) **(call)**: add both `App` names to
`containsFunIn`.

## Where things change

| layer | files |
|---|---|
| signatures | `stdlib/Random.alder` (the two types and twelve functions), `stdlib/Text.alder` (`hash`), `stdlib/Fuzz.alder` (`fromRandom`) |
| name → Go | `internal/gen/stdcall.go`: `"Random.seed": "std.RandomSeed"`, …, `"Text.hash": "std.TextHash"`, `"Fuzz.fromRandom": "fuzz.FromRandom"` |
| Go types | `internal/gen/types.go` `GoType`: `Con{Seed, Module: Random}` → `std.Seed`; `App{Generator}` → `std.Generator[A]` (the `Fuzzer` precedent) |
| checker | `internal/check/expr.go` `containsFunIn`: `Generator` and `Fuzzer` count as functions |
| runtime | new `rt/std/random.go`: `type Seed struct{ State uint64 }` implementing `rt.Shower` (`Random.seed N`, an application), `type Generator[A any] func(Seed) (A, Seed)`, the step function and each stdlib function; `rt/std/text.go`: `TextHash`; `rt/fuzz/combinators.go`: `FromRandom` (rt/fuzz may import rt/std; nothing in rt/std imports rt/fuzz) |
| book | `book/09-stdlib.md`: `Text.hash` in the Text block; a new `## Random` section before "Query, Task, …" with the pure block, the step rule, the algorithm names, the seed idiom `Random.seed (Text.hash (Text.join "/" [county, key]))`, and one `eval` fence; `Fuzz.fromRandom` in the Fuzz block with its shrinking caveat. `book/07-effects.md`: one sentence after the capability block pointing reproducible randomness at chapter 9 |
| site | `book/site/build.py` regenerates `site/09-stdlib.html`, `site/reference.md`, `site/reference-full.md` |
| roadmap | tick #7 in the pinned roadmap issue #52; the user reinstalls (`go install ./cmd/alder`) after merge for LSP completion |

No change to the parser, IR, formatter, LSP, tree-sitter grammar or `cmd/alder`: an opaque
stdlib type with or without a parameter is already parsed (`Fuzz.Fuzzer a`, `Fuzz.Property`), and
pure stdlib calls are ordinary `VarStd` calls. `Fuzz.any` at `Random.Seed` or `Random.Generator a`
is E0038 (not derivable), as for `Html`; the builder confirms with a test rather than adding a case.

## Tests the builder adds (by behaviour)

- `rt/std/random_test.go`: the three SplitMix64 rows above; `Random.seed` of a negative `Int`
  round-trips through `Show`; every vector in "Semantics" (`intRange 1..6` from 42, the five
  dice from 1, `floatRange` and `normal` from 42, the full-range `intRange` from seed 0 is
  `-2152535657050944081`); degenerate `intRange`/`floatRange` return `min` and still advance the
  seed; `constant` does not advance; `list 0` and `list -3` give `[]` without advancing;
  `map2`/`andThen` step left to right (the second generator sees the first's seed); `uniform x []`
  is `x`; `weighted` picks by absolute weight, all-zero weights give the first, the last item
  catches rounding; Acklam's three quantiles; 60 000 dice from seed 3 land within 3% of uniform
  per face; 100 000 standard normals from seed 7 have mean within 0.01 of 0 and sd within 0.01
  of 1; two seeds with equal states are `rt.Eq`.
- `rt/std/text_test.go`: the three `Text.hash` vectors; the hash is never negative over a
  thousand short texts; `"héllo"` hashes its UTF-8 bytes (differs from `"hello"`).
- `internal/check` tests: a signature `Random.Generator Int` and `Random.Seed` type-checks; bare
  `Seed` is E0202 with the qualified fix; `g == g` on a `Random.Generator Int` and on a
  `Fuzz.Fuzzer Int` is "functions cannot be compared"; `Fuzz.fromRandom (Random.intRange { min:
  1, max: 6 })` has type `Fuzz.Fuzzer Int`; `Fuzz.any` at `Random.Seed` is E0038.
- `internal/gen`: `TestStdCallsCoverStdlib` passes unchanged; a corpus program whose `load` uses
  `Random.step` and `Text.hash` in a `Query` compiles and the generated Go builds.
- `rt/fuzz`: `FromRandom` on the all-zero sequence gives the generator's `Random.seed 0` value;
  a property `Fuzz.forAll (Fuzz.fromRandom (Random.intRange { min: 0, max: 100 })) (\n -> n < 50)`
  fails and shrinks to the smallest seed whose value is `>= 50` (checked against a direct scan).
- Book: the `eval` fence in the new section (`Random.step (Random.intRange { min: 1, max: 6 })
  (Random.seed 42)`) renders `{ value: 2, seed: Random.seed -7046029254386353089 }`.

## Left to the builder

- Plan tasks: (1) `Text.hash`; (2) `Random` types, `seed`, `step`, `intRange`, `floatRange`,
  `constant`, `map`, `map2`, `andThen`, `list`; (3) `normal`, `uniform`, `weighted`; (4) the
  `containsFun` change; (5) `Fuzz.fromRandom`; (6) book and site. Each is mergeable alone.
- Whether `Generator[A]` is a named func type or a struct wrapping one; the Alder-visible
  behaviour is the same. (`Choices.Choice(math.MaxUint64)` is already uniform over the full
  range: `uniform` special-cases it.)
- Exact book wording.

## Verification

- `go test ./...` green, including every test above.
- A scratch check (not a change to nightvote-replay, which pins its own Alder): `Utils.Rng` and
  `Utils.Curve` rewritten on the new API: `Rng.hash parts` → `Random.seed (Text.hash (Text.join
  "/" parts))`, `Rng.normal h` → `Random.normal { mean: 0.0, sd: 1.0 }`, `Rng.uniform h` →
  `Random.floatRange { min: 0.0, max: 1.0 }`, `Rng.scramble h % 360` → `Random.intRange { min: 0,
  max: 359 }`, each through `Random.step`. The `Curve` tests ("log-normal shape", "clamped to
  [5, 360]", "about 30% have a partial phase", "seed changes the night") pass on the rewrite,
  and the `Rng` module's `scramble` is no longer needed (SplitMix64's output is not affine in its
  state).
