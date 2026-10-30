# Large-output rendering: list views, Dict key fast path, Text.Builder

Design for [tapegram/alder#37](https://github.com/tapegram/alder/issues/37), with the smallest part
of [#48](https://github.com/tapegram/alder/issues/48) (a benchmark harness) that proves each fix.
The evidence is nightvote-replay's `docs/perf/2026-10-ev-render.md`: a 118 MB JSON body from
329,523 rows took 0.38 s and allocated 1.04 GB in Alder 393f986, against 0.029 s and one
allocation in Go, so the app rendered that site in Go. Everything marked **(call)** is the author's
decision, not the user's.

## Intent

A render that writes a large `Text` body row by row should cost about what the same loop costs in
Go: O(1) list access, O(1) `Dict.get` on `Text`/`Int` keys, and about two bytes allocated per byte
of output instead of nine. After this change the EV render described in the report would fit the
report's own budgets (0.33 s, under 1 GB for two concurrent requests) without leaving Alder.

## Goals

1. `List.take`, `List.drop` and `List.tail` are O(1) and allocate nothing.
2. `Dict.get`/`Dict.member` with a `Text` or `Int` key allocate nothing and do not use reflection.
3. A `Text.Builder` whose `add` is O(1) and whose `build` copies each byte once.
4. `File.text`, `File.json` and `File.html` do not copy the body a second time.
5. Each goal has a Go benchmark under `rt/` with a target this document states; the four
   benchmarks are #48's harness for the rendering half of that issue, nothing more.
6. Existing programs compile and behave identically (`go test ./...` is the witness).

## Non-goals

- A persistent list (`++` in a loop stays O(n) per step; see "Append" below) and O(1) random
  access (`List.get`): #6 added `indexedMap`, `map2` and `map3`, which remove the walk the report
  measured; this document does not add an index.
- Streaming a response body (a `File` is still one body in memory).
- `not` on `Bool`: it already exists (`stdlib/Prelude.alder` line 44, `not : Bool -> Bool`, since
  2026-10-08, before the 393f986 build the report used; the book lists it under Bool) **(call)**.
  Nothing to design; the closing comment on #37 says so.
- #48's other items (N+1 detection, statement cache, the whole-page benchmark).
- A compiler change for the `[x, ..rest]` element copy (see cause 1 below).

## Anti-goals

- Any sharing of storage that a later append could observe. Every shared slice this design
  returns has `cap == len` (a three-index slice), so an append must reallocate.
- A builder with in-place mutation. Alder values are immutable and may be shared across requests
  (a top-level `let`); a builder that appends into a shared buffer is the aliasing bug this design
  exists to avoid.
- Encoding changes visible through `rt.Key`: `Key(any)` keeps today's encoding exactly.

## Cause 1: list access copies

**`List.take`, `List.drop`, `List.tail` return views** **(call)**. In `rt/std/list.go`:

```go
func ListTake[A any](n int64, xs []A) []A { k := clampLen(n, len(xs)); return xs[:k:k] }
func ListDrop[A any](n int64, xs []A) []A { k := clampLen(n, len(xs)); return xs[k:len(xs):len(xs)] }
```

and `ListTail` is `ListDrop(1, xs)` wrapped in `Just`. Clamping is unchanged (`n <= 0` takes
nothing / drops nothing; `n >= length` takes all / drops all). A nil input gives a nil view, which
is the empty list everywhere (`rt.Eq`, `rt.Show`, `List.isEmpty` all treat nil and empty alike;
`rt.Eq`'s doc already promises it).

Why sharing is safe (the builder's doc comment on `ListTake` states this): Alder has no
assignment, and `internal/gen` never emits an index store on a list; the runtime only appends to
slices it has just made (`out := []A{}` in `list.go`, `groups`/`r.Yes` locals) and never to a
parameter or a result that Alder code has seen; `rt.Append` builds a fresh slice. So the only way
to write into a shared backing array would be `append` into spare capacity, and the views above
have none. The existing `take does not alias` test (`std_test.go`, `_ = append(tk, 99)`) keeps
passing by construction, and a new one covers `drop`.

**`++` (`rt.Append`)**: when one operand is empty, the result is the other operand, capped
(`b[:len(b):len(b)]`) **(call)**; otherwise a fresh slice as today. The doc comment changes from
"never shares storage" to "shares storage only when one side is empty; the result's capacity is
its length". A recursive `acc ++ [x]` stays O(n) per step; the book's List section says to use
`List.foldl`, `List.concatMap` or `Text.Builder` for an accumulating loop.

**The `[x, ..rest]` element copy stays** **(call)**. `caseBody` binds `x := s[0]` before the arm's
body tests the index, so a 48-byte record is copied per step. Measured on this machine, a 1000-step
walk over 48-byte records is 580 ns with the copy and 380 ns indexing in place: 0.2 ns per step,
about 3 ms of the report's 110 ms. The cost was the O(rows²) walk, which `List.map2` (#6) removes;
binding lazily would complicate `caseBody` for nothing measurable.

## Cause 2: Dict keys by reflection

Today `Dict.Get` calls `rt.Key(k)`: a `strings.Builder`, a `reflect.Value` walk and the `any`
boxing — 43 ns and 3 allocations per lookup on a 1000-key `Dict Text Int` (measured here).

**A per-Dict encoding with a fast path for `Text` and `Int`** **(call)**. A `rt.Dict[K, V]` has one
key type, so its map only needs an encoding that is injective within `K`, not across all types:

| `K` (Go type) | map key | how `Get`/`Has` build it |
|---|---|---|
| `string` (Text, its brands, `Id t`) | the string itself | `d.m[x]` |
| `int64` (Int, its brands) | its 8 bytes, big-endian | `var buf [8]byte` on the stack, `d.m[string(buf[:])]` (the compiler's no-copy map index) |
| anything else | `Key(k)` as today | unchanged |

`Get` and `Has` switch on `any(k).(type)` inline (0 allocations in generic code; measured 7 ns).
`Put`, `Insert` and `Remove` use one helper `dictEnc[K](k) string` with the same three cases, so a
Dict never mixes schemes; `encs`, `Remove`'s comparison, `Equal`, `showDict` and `keyAny` keep
working unchanged because they only compare or sort encodings of one Dict. `keyAny` (a Dict as a key
of another Dict) still sorts `encs`, so insertion order is still ignored; the sort order changes,
the canonical form is still canonical.

`rt.Key(any)` gets a type switch for `string` and `int64` ahead of the reflect walk, writing
exactly today's `s<len>:…` and `i<n>;` forms **(call)**, so `List.unique` and `List.groupBy` stop
reflecting on the common key types while every `TestDict_AnyKey` pair stays distinct.

## Cause 3: Text building allocates 9× the output

Nested `Text.join`/interpolation copies every byte once per nesting level (row, option, item,
file). A 3-level render of 10k rows of ~335 bytes measured here allocates 7.5× its output (the
report's real file: 8.8×). Neither `Text.join` nor `rt.Concat` can get cheaper: each is already
one allocation. A `File` body built from `List Text` chunks would only remove the last level
(string headers still copy per level, 16 bytes × chunks × depth). The multiplier goes away only
when no level copies: a builder.

**`Text.Builder`** **(call)**. In `stdlib/Text.alder`:

```alder
-- an accumulator for a large Text: each add is O(1) and shares nothing; build copies once
type Builder
builder : Builder                          -- empty
add     : Text -> Builder -> Builder       -- Text.add piece b
build   : Builder -> Text
```

Pipe shape: `rows |> List.foldl (\r b -> b |> Text.add "," |> Text.add (rowText r)) Text.builder
|> Text.build`. There is no `addAll`: `List.foldl Text.add b parts` is that **(call)**. No
`File.fromBuilder`: `File.text { contentType: …, body: Text.build b }` is one line and (cause 4) no
longer copies.

Representation, `rt.TextBuilder` in a new `rt/builder.go`: a persistent cons list,
`struct{ n *tbNode }` with `tbNode{ s string; prev *tbNode; size int }` (`size` is the total bytes
up to and including this node). The zero value is `Text.builder`. `add` allocates one 32-byte node
(or returns `b` unchanged when the piece is `""`); nodes are never mutated, so two builders that
share a prefix are both correct — the fork test below proves it. `build` allocates `size` bytes
once, fills them from the end walking `prev` (a loop, no recursion, so a million adds is fine) and
returns them as a string with `unsafe.String`, the idiom `strings.Builder.String` uses (`unsafe` is
new to `rt`; it stays inside `rt/builder.go` and `rt/std/file.go`). Measured on the shape above:
2.2× output allocated instead of 7.5×, at 0.46× the time. With 30-byte pieces the node overhead
shows (3.7×, same time as the joins), which the book sentence mentions: build row text by
interpolation, add whole rows.

`TextBuilder` implements `rt.Equaler`, `rt.Keyer` and `rt.Shower`: two builders are `==` when
they build the same `Text`, a builder as a Dict key is keyed by its text, and `rt.Show` prints
`Text.builder` for the empty one and `Text.add "<text>" Text.builder` otherwise, an expression
that rebuilds an equal value **(call)**. `Fuzz.any` cannot derive a `Text.Builder` (E0038 by the
existing rule: an opaque type outside Prelude has no generator), as for `Html`.

Go names: `std.TextBuilder()` (a constant, so a Go function of no parameters, like `Dict.empty`),
`std.TextAdd(s string, b rt.TextBuilder)`, `std.TextBuild(b rt.TextBuilder)`; `GoType` maps the
`Con{Module: "Text", Name: "Builder"}` to `rt.TextBuilder` as `Fuzz.Property` is mapped by module.
The checker needs no change: `type Builder` in a stdlib module is already the opaque path
(`decls.go`, `Con` with the module set), and `Text.Builder` resolves in user annotations as
`Text.NonEmpty` and `Fuzz.Fuzzer a` do; the corpus case below is the proof.

## Cause 4: File.text's copy

`std.FileText` does `[]byte(r.Body)`: the request holds the body twice (the report measured 1.0 GB
against 0.84 GB without the copy). **`File.text`, `File.json` and `File.html` alias the Text's
bytes** with `unsafe.Slice(unsafe.StringData(s), len(s))` (empty text gives `[]byte{}`) **(call)**.
The invariant that makes this sound, stated in a doc comment on `rt.File.Body`: a `[]byte` that
Alder code can see is never written to. Today that holds — `BytesSlice`, `BytesAppend`,
`BytesConcat`, `BytesFromList` all build fresh slices, `BytesToText` copies into a string, the
router only `Write`s `f.Body`, `Test.fetch` reads a recorded response — and the test below pins
it.

## Tests the builder adds (by behaviour)

`rt/std/std_test.go`:
- take/drop/tail of a nil list give an empty list; `drop` result appended to does not change the
  input (as the existing `take does not alias`); `tail` of `[1]` is `Just []`.
- `++`: `[] ++ xs` and `xs ++ []` are `Eq` to `xs`, appending to the result does not change `xs`;
  `[1] ++ [2]` is a fresh `[1, 2]`.
- Builder: `build builder == ""`; `add "" b` is `b` (same text, no growth); build of three adds
  concatenates in order; building twice gives the same text and leaves the builder usable; fork:
  `b1 = add "a" b0`, `b2 = add "b" b0`, builds are `"a"` and `"b"`; a million adds build (no
  recursion); `rt.Eq` of two builders with the same text is true and with different text false;
  `rt.Key` of equal-text builders is equal; `rt.Show` prints `Text.builder` and
  `Text.add "ab" Text.builder`; `std.TextBuilder()` is the zero value.
- `FileText`: `&f.Body[0] == unsafe.StringData(body)`; `FileJson` and `FileHtml` likewise;
  `File.text` of `""` has an empty non-nil body; `BytesSlice`/`BytesAppend` of `File.body` do not
  share storage with it (`&out[0]` differs).

`rt/core_test.go`:
- `Dict[int64, …]`: insert, get, member, remove, keys order with `0`, `-1`, `1`,
  `math.MinInt64`, `math.MaxInt64`; `-1` and `1` are distinct keys; `Insert` of an existing Int key
  keeps its position.
- `Dict[string, …]`: `""` is a valid key; existing order tests pass unchanged.
- A `Dict Text Int` as the key of another Dict is one key regardless of insertion order (as the
  existing `unique dicts order-insensitive`).
- `Key("a") == "s1:a"` and `Key(int64(5)) == "i5;"` (the encoding is unchanged).

`testdata/corpus/text-builder-ok/`: a page whose view builds its `File.text` body with
`List.foldl Text.add Text.builder`, and a module function annotated
`addRow : Row -> Text.Builder -> Text.Builder`; the case compiles with no diagnostics and
`TestStdCallsCoverStdlib` passes with the three new rows.

## Benchmarks (the #48 slice)

Each is `b.ReportAllocs()`, runs under `go test ./rt/... -run XXX -bench 'DictGet|ListTakeDrop|TextBuild|TextJoinNested' -benchtime=200ms`, and the builder records before/after in the PR body.

| benchmark (file) | shape | before (measured here) | target |
|---|---|---|---|
| `BenchmarkListTakeDrop` (`rt/std`) | take 50k and drop 50k of 100k 48-byte records | 290 µs, 2.4 MB, 1 alloc each | 0 allocs/op, < 20 ns/op |
| `BenchmarkDictGet_Text`, `_Int` (`rt`) | 1000-key Dict, cycling keys | 43 ns, 56 B, 3 allocs | 0 allocs/op, ≤ 15 ns/op |
| `BenchmarkDictGet_Record` (`rt`) | key `{ a: Text, b: Int }` | not measured | within 10% of before (the reflect path is untouched) |
| `BenchmarkTextJoinNested` (`rt/std`) | 10 items × 10 options × 100 rows of ~335 B, joined per level | 3.5 ms, 25.5 MB (7.5× output) | kept as the comparison, not a target |
| `BenchmarkTextBuild` (`rt/std`) | same output through `TextAdd`/`TextBuild` | 1.6 ms, 7.5 MB (2.2×) in the scratch prototype | ≤ 2.5× output B/op, ≤ 0.6× the join's ns/op |

`BenchmarkListUnique` (existing) must not get slower. The EV render itself is not re-run here (its
data lives in nightvote-replay); the issue's 1.04 GB per 118 MB corresponds to the join row above.

## Where things change

| layer | files |
|---|---|
| runtime | `rt/std/list.go` (take, drop, tail), `rt/core.go` (Append, Dict Get/Has/Put/Remove, dictEnc, Key fast path), new `rt/builder.go` (TextBuilder, Equaler/Keyer/Shower), `rt/std/text.go` (TextBuilder, TextAdd, TextBuild), `rt/std/file.go` (aliasing), `rt/file.go` (Body doc) |
| signatures | `stdlib/Text.alder`: `type Builder` and the three lines |
| name → Go mapping | `internal/gen/stdcall.go`: `Text.builder`, `Text.add`, `Text.build` |
| Go types | `internal/gen/types.go`: `Builder` of module `Text` → `rt.TextBuilder`, and the doc table |
| checker, IR, parser, formatter, LSP, tree-sitter, `cmd/alder` | nothing: no syntax, and the LSP's completion reads the signature file |
| tests, benchmarks | `rt/std/std_test.go`, `rt/core_test.go`, `testdata/corpus/text-builder-ok/` |
| book | `book/09-stdlib.md`: the three lines in the Text block with a paragraph (what a builder is for, add whole rows, `List.foldl Text.add`); one sentence under List (`++` in a loop) |
| site | `book/site/build.py` regenerates the stdlib pages after the book edit |

Also: tick #37 on the roadmap issue #52; the closing comment on #37 notes that `not` exists; the
user reinstalls (`go install ./cmd/alder`) so the LSP completes `Text.add`.

## Left to the builder

- Whether `dictEnc` and the inline switches in `Get`/`Has` share a helper for the Int buffer.
- Node layout details (`size` may be dropped for a two-pass build if the benchmark prefers it).
- Exact wording of the book paragraph.

## Verification

- `go test ./...` green, including the corpus case and `TestStdCallsCoverStdlib`.
- The benchmark table above, before and after, in the PR body; every target met.
- A scratch check (not committed): the report's `Sites.EvJson.render` shape rewritten with
  `Text.Builder` and `List.map2` on Kent's 11 MB fixture gives byte-equal output; its B/op is
  within 3× of the output size. This is a one-off in a scratch clone of nightvote-replay, which
  pins its own Alder build.
