# Stdlib batch: the #6 gaps and their Elm-core neighbours

Design for [tapegram/alder#6](https://github.com/tapegram/alder/issues/6). The eight gaps were found
by Claude while building nightvote-replay; the user asked for each touched module to be rounded out
to its obvious Elm-core neighbours at the same time, so the next wave of that project does not file
#6 again. Everything marked **(call)** is the author's decision, not the user's.

## Intent

nightvote-replay still carries hand-written workarounds for every gap in #6: `pad2`, the
`Text.slice 1 (places + 1) (Text.fromInt (scale + n % scale))` zero-padding trick, a `code c` lookup
that splits a printable-ASCII alphabet, Acklam's constants spelled out as 16-digit decimals,
`Time.toDate Time.epoch` as the Date fallback in eight places, `Bytes.toText (File.body (File.html h))`,
and the zip "cut in half" living in a Go adapter because Bytes cannot be sliced. After this batch each
of those is one stdlib call, and a model writing Alder that reaches for an Elm core name in `Text`,
`List`, `Dict`, `Date` or `Float` finds it.

## Goals

1. Every item in #6 has a stdlib function or literal form.
2. The touched modules (`Text`, `List`, `Dict`, `Bytes`, `Date`, `Time`, `Html`, `Float`, `Int`) carry
   the Elm core functions that translate to Alder's types.
3. Each new function is documented in `book/09-stdlib.md` with the same one-line style as its
   neighbours, has a unit test in `rt/std/std_test.go`, and type-checks from its signature line alone.
4. Existing programs keep compiling and behaving identically.

## Non-goals

- A `Char` type, tuples, `Set`, an `Order` type or a comparable class. Where Elm leans on one of
  these the Alder shape uses records, `Text` of length one, `Int` or a `by` function (see below).
- `Maybe` and `Result` neighbours (`map2`, `fromMaybe`, …). Not touched by #6; a separate issue if
  wanted.
- Hex/binary/octal literals, `Text.fromFloat` emitting exponent form, `Dict.merge`, `List.sort`
  on a comparable, `List.scanl`, `Text.pad` (centre). Listed so a builder does not add them.
- Performance work. Implementations are the straightforward Go.

## Anti-goals

- A name that means something different from its Elm namesake. If the semantics must differ, the
  name must differ (`List.sortBy` already takes `a -> Int`, so there is no `List.sort`).
- Functions that return a record with invented field names where a `by`-function or `map2` would
  let the caller name things (so: `List.map2`, no `List.zip`).

## Conventions the builder follows

- **Units are code points.** Every `Text` index and length is in code points, as `Text.slice` and
  `Text.length` already are (`rt/std/text.go` works on `[]rune`).
- **Slices clamp, never fail.** Indices follow `Text.slice`: negative counts from the end, out of
  range clamps, `start >= end` gives the empty value. `Bytes.slice` reuses the same index rule.
- **Counts clamp to zero.** `n <= 0` in `left`, `right`, `repeat`, `List.repeat`, `padLeft`, … is
  treated as `0`.
- **Records where Elm uses tuples**, with the field names fixed below. On the Go side they are
  anonymous structs exactly as `Dict.toList`'s `{ key, value }` and `List.indexed`'s
  `{ index, item }` are today.
- **Dict order is key insertion order**, as `rt.Dict` already is; `foldl`, `toList`, `keys` and the
  new functions all iterate in it, and `map`/`filter` preserve it.
- **Int/Float pairs** follow the `List.sum`/`List.sumFloat` precedent: the `Int` version takes the
  plain name, the `Float` version adds `Float`.

## The API

Signatures are the exact lines to add to `stdlib/<Module>.alder`. Each needs a `"Module.name":
"std.ModuleName"` row in `internal/gen/stdcall.go` and a Go function in `rt/std/`.

### Text

```alder
padLeft        : Int -> Text -> Text -> Text   -- Text.padLeft 2 "0" "7" == "07"
padRight       : Int -> Text -> Text -> Text   -- Text.padRight 5 "." "ab" == "ab..."
left           : Int -> Text -> Text           -- first n code points
right          : Int -> Text -> Text           -- last n code points
dropLeft       : Int -> Text -> Text
dropRight      : Int -> Text -> Text
repeat         : Int -> Text -> Text
reverse        : Text -> Text
trimLeft       : Text -> Text
trimRight      : Text -> Text
concat         : List Text -> Text             -- Text.join ""
lines          : Text -> List Text
words          : Text -> List Text
indexes        : Text -> Text -> List Int      -- Text.indexes needle haystack
toCodePoints   : Text -> List Int
fromCodePoints : List Int -> Text
```

Semantics:

- `padLeft n pad s`: if `Text.length s >= n` or `pad == ""`, `s`; otherwise
  `Text.right n (Text.repeat n pad ++ s)`. `padRight n pad s` is `Text.left n (s ++ Text.repeat n pad)`
  under the same guard. (A multi-character `pad` is therefore cut at the outer edge; the string's own
  characters are never cut.)
- `left`/`right`/`dropLeft`/`dropRight` clamp `n` to `[0, length]`.
- `repeat n s`: `n <= 0` gives `""`.
- `trimLeft`/`trimRight` strip the same whitespace set as `trim` (Go `unicode.IsSpace`).
- `lines` splits on `\n` and removes a trailing `\r` from each line (so `\r\n` input works); a final
  empty line after a trailing `\n` is dropped, as in Elm (`Text.lines "a\nb\n" == ["a", "b"]`).
  `Text.lines "" == []`.
- `words` splits on runs of whitespace, dropping empty parts (`Text.words "  a  b " == ["a", "b"]`).
- `indexes needle haystack` returns every code-point offset where `needle` starts, including
  overlapping matches (`Text.indexes "aa" "aaa" == [0, 1]`); an empty needle gives `[]`.
- `toCodePoints` is `[]rune` as `List Int`. `fromCodePoints` turns each `Int` into a character;
  values that are not valid scalar values (negative, surrogates, above U+10FFFF) become U+FFFD, so
  there is no `Maybe` **(call)**.
- No `toChars`: `Text.split "" s` already yields the characters and the book says so.

### List

```alder
indexedMap   : (Int -> a -> b) -> List a -> List b
map2         : (a -> b -> c) -> List a -> List b -> List c
map3         : (a -> b -> c -> d) -> List a -> List b -> List c -> List d
foldr        : (a -> b -> b) -> b -> List a -> b
concatMap    : (a -> List b) -> List a -> List b
intersperse  : a -> List a -> List a
partition    : (a -> Bool) -> List a -> { yes: List a, no: List a }
tail         : List a -> Maybe (List a)
repeat       : Int -> a -> List a
maximum      : List Int -> Maybe Int
minimum      : List Int -> Maybe Int
maximumFloat : List Float -> Maybe Float
minimumFloat : List Float -> Maybe Float
sortByFloat  : (a -> Float) -> List a -> List a
```

Semantics:

- `map2`/`map3` stop at the shortest list.
- `partition` keeps each half in input order; field names `yes`/`no` **(call)** — the Elm tuple is
  `(satisfy, fail)`, and `yes`/`no` read naturally at the call site
  (`let parts = List.partition isOpen races` … `parts.yes`).
- `tail []` is `Nothing`.
- `sortByFloat` is stable, like `sortBy` and `sortByText`.
- No `zip`/`unzip` (anti-goal), no `sort`/`sortWith` (no comparable/Order), no `singleton`
  (`[x]`).

### Dict

```alder
singleton : k -> v -> Dict k v
isEmpty   : Dict k v -> Bool
member    : k -> Dict k v -> Bool
update    : k -> (Maybe v -> Maybe v) -> Dict k v -> Dict k v
map       : (k -> v -> w) -> Dict k v -> Dict k w
filter    : (k -> v -> Bool) -> Dict k v -> Dict k v
foldl     : (k -> v -> b -> b) -> b -> Dict k v -> b
partition : (k -> v -> Bool) -> Dict k v -> { yes: Dict k v, no: Dict k v }
union     : Dict k v -> Dict k v -> Dict k v
intersect : Dict k v -> Dict k v -> Dict k v
diff      : Dict k v -> Dict k v -> Dict k v
```

Semantics:

- `update k f d`: `f` sees `Dict.get k d`; `Just v` inserts/replaces (keeping the key's position if
  it existed, appending at the end if it did not), `Nothing` removes.
- `union a b` prefers `a`'s value on a shared key (Elm). Order: `a`'s keys in `a`'s order, then `b`'s
  new keys in `b`'s order.
- `intersect a b` keeps `a`'s entries whose key is in `b`; `diff a b` keeps `a`'s entries whose key
  is not in `b`. Both keep `a`'s values and order.
- `foldl` iterates in key insertion order; no `foldr` **(call)** — with insertion order there is no
  natural reverse, and `Dict.toList |> List.foldr` exists.

### Bytes

```alder
empty    : Bytes
isEmpty  : Bytes -> Bool
slice    : Int -> Int -> Bytes -> Bytes      -- same index rules as Text.slice, in bytes
append   : Bytes -> Bytes -> Bytes
concat   : List Bytes -> Bytes
toList   : Bytes -> List Int                 -- each 0..255
fromList : List Int -> Bytes                 -- the low eight bits of each Int
```

`fromList` masks each `Int` to its low eight bits (`Bytes.fromList [256] == Bytes.fromList [0]`,
`-1` becomes `255`) **(call)**: binary code rarely wants a `Maybe` here, and masking is what a
builder expects from a byte cast.

### Date, Time

```alder
-- Date
fromParts : { year: Int, month: Int, day: Int } -> Maybe Date
toParts   : Date -> { year: Int, month: Int, day: Int }
epoch     : Date                 -- 1970-01-01
weekday   : Date -> Int          -- ISO: 1 = Monday … 7 = Sunday
addMonths : Int -> Date -> Date  -- clamps the day to the target month's length
addYears  : Int -> Date -> Date  -- Feb 29 → Feb 28 in a non-leap year

-- Time
fromDate  : Date -> Time         -- midnight UTC
```

- `fromParts` returns `Nothing` for an impossible date (month outside 1–12, day outside the month,
  Feb 30, …). It does not normalise; `{ year: 2026, month: 13, day: 1 }` is `Nothing`, not January.
- `weekday` is an `Int` because Alder has no `Weekday` type and `Date.format` already covers names.
  ISO numbering **(call)**: it matches the `%u`-style convention most date libraries print and makes
  `weekday d >= 6` read as "weekend".
- `addMonths` clamps (Jan 31 + 1 month = Feb 28/29), matching justinmimbs/date and every mainstream
  date library.

### Html

```alder
toText : Html Never -> Text   -- the markup File.html serves
```

`Html Never` because a view with messages carries event wiring that has no text form; this matches
`File.html`.

### Float, Int, literals

```alder
-- Float
truncate   : Float -> Int
clamp      : { min: Float, max: Float } -> Float -> Float
pi         : Float
e          : Float
isNaN      : Float -> Bool
isInfinite : Float -> Bool
logBase    : Float -> Float -> Float   -- Float.logBase base x
sin, cos, tan, asin, acos, atan : Float -> Float
atan2      : Float -> Float -> Float   -- Float.atan2 y x

-- Int
pow        : Int -> Int -> Int         -- Int.pow base exponent; a negative exponent gives 0
```

- `truncate` rounds toward zero; `round`, `floor`, `ceil` already exist.
- `Float.clamp` takes the same record as `Int.clamp` and checks in the same order (below `min`
  gives `min`, else above `max` gives `max`, else `x`), so `min > max` behaves as `Int.clamp` does.
- `Int.pow` with a negative exponent returns `0` **(call)** — Elm's `^` is undefined there; `0` is
  the integer-division answer and never panics. Overflow wraps as `*` does today.

**Exponent literals.** The lexer accepts `digits [ "." digits ] ("e" | "E") [ "+" | "-" ] digits`
as a `Float` literal: `1.5e-3`, `1e6`, `2E+3`. Any literal with an exponent is a `Float`, including
`1e6` **(call)**, as in Elm. `Text.toFloat` accepts the same form (Go `strconv.ParseFloat` already
does; the builder confirms with a test). `Text.fromFloat` output is unchanged.

The tree-sitter grammar's `float` rule gets the same optional exponent, and the grammar's generated
files and the corpus test for literals are regenerated in the same change (see #63/#67 for the
regeneration steps).

## Where things change

| layer | files |
|---|---|
| signatures (the type checker reads these) | `stdlib/Text.alder`, `List.alder`, `Dict.alder`, `Bytes.alder`, `Date.alder`, `Time.alder`, `Html.alder`, `Float.alder`, `Int.alder` |
| name → Go mapping | `internal/gen/stdcall.go` |
| implementations | `rt/std/text.go`, `list.go`, `dict.go`, `date.go` (Date/Time), `html.go`, `num.go`; `Bytes` goes in a new `rt/std/bytes.go` |
| unit tests | `rt/std/std_test.go`, one `eq(...)` line per behaviour listed above, including every edge case this document names |
| lexer | `internal/syntax/lexer.go` (around the `Float` token at line ~210) plus a lexer/parser test, and a corpus case under `testdata/corpus/` |
| grammar | `tree-sitter-alder/grammar.js` `float` rule; regenerate |
| book | `book/09-stdlib.md`: each new line in the module's code block, plus a sentence where semantics are not obvious (`padLeft` with multi-char pad, `lines`, `indexes`, `Dict.union` bias, `Bytes.fromList` masking, `Date.addMonths` clamping, exponent literals) |
| site | `book/site/build.py` regenerates `site/09-stdlib.html`, `site/reference.md` and `site/reference-full.md` from the book; run it after the book edit |

Also update: the roadmap issue #52 tick for #6, and the user reinstalls (`go install ./cmd/alder`)
after merge since the LSP's completion reads the stdlib signatures.

## Left to the builder

- Grouping into plan tasks: one task per module is the natural split (Text, List, Dict, Bytes,
  Date/Time, Html, Float/Int, literals), each independently mergeable and testable.
- Whether a Go generic helper is shared between `List.maximum`/`maximumFloat` etc. Match the
  surrounding code (`list.go` has separate `ListSum`/`ListSumFloat`).
- Exact wording of the book sentences.

## Verification

- `go test ./...` green, including the new `rt/std` cases and the lexer/corpus cases.
- A scratch check: each #6 workaround in nightvote-replay rewritten with the new call gives the same
  output (`pad2` → `Text.padLeft 2 "0"`, `code` → `Text.toCodePoints`, `tail`'s constants →
  exponent literals, `Time.toDate Time.epoch` → `Date.epoch`, the zip cut → `Bytes.slice`). This is a
  one-off check in a scratch file, not a change to nightvote-replay, which pins its own Alder build.
