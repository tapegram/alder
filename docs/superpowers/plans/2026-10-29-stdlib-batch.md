# Stdlib batch (#6) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Close tapegram/alder#6 by adding the eight missing stdlib forms and each touched module's Elm-core neighbours (~75 functions plus exponent float literals).

**Architecture:** Every stdlib function is wired the same way: a signature line in `stdlib/<M>.alder` (the type checker and LSP read it), a `"M.name": "std.MName"` row in `internal/gen/stdcall.go`, a Go function in `rt/std/`, unit tests, and a line in `book/09-stdlib.md`. Tasks 1–7 are one module each and touch disjoint code, so they run in parallel worktrees; Task 8 merges them, regenerates the site and verifies.

**Tech Stack:** Go (generics), Alder stdlib signature files, tree-sitter grammar (CLI 0.27.1 via `scripts/ts.sh`), Python site builder.

**Spec:** `docs/superpowers/specs/2026-10-29-stdlib-batch-design.md`. Every task implements the signatures and semantics of its module's section of the spec **verbatim**; the spec is the source of truth for exact behaviour and this plan does not repeat it. Read the spec's "Conventions the builder follows" before starting.

## Global Constraints

- Units are code points: every `Text` index and length works on `[]rune`.
- Slices clamp, never fail; negative indices count from the end; `start >= end` gives the empty value. `Bytes.slice` uses `Text.slice`'s index rule.
- Counts clamp to zero: `n <= 0` behaves as `0`.
- Records where Elm uses tuples map to Go anonymous structs with PascalCased fields in declaration order (`{ yes, no }` → `struct{ Yes …; No … }`), as `DictToList` and `ListIndexed` do.
- Dict order is key insertion order; `map`/`filter`/`partition`/`intersect`/`diff` preserve it.
- No function mutates its inputs. `rt.Dict.Put` is a pointer method: build a fresh Dict, never `Put` into the argument. Slices returned from `Bytes`/`List` functions never alias the input's backing array.
- Int/Float pairs: the `Int` version takes the plain name, the `Float` version adds `Float`.
- Callbacks with several parameters are uncurried Go funcs: `(Int -> a -> b)` is `func(int64, A) B` (see `ListFoldl`).
- A zero-parameter stdlib value (`Bytes.empty`, `Date.epoch`, `Float.pi`, `Float.e`) is a Go function of no arguments.
- Do not add anything the spec lists under Non-goals or Anti-goals (`zip`, `sort`, `toChars`, `Dict.foldr`, `Text.pad`, hex literals, …).
- Signature lines in `stdlib/*.alder` follow the file's style: a `-- comment` line above the signature when the behaviour is not obvious from the name.
- Book lines follow the module's code block style (`Module.name : type`, aligned), plus one sentence of prose where the spec says semantics are not obvious.

## Parallel-execution rules (Tasks 1–7)

- Each task runs in its own worktree branched from `stdlib-batch-6` and commits there; Task 8 merges.
- Tests go in a **new per-module test file** (`rt/std/<module>_test.go`, package `std`, reusing the `eq` helper from `std_test.go`) instead of appending to `std_test.go`, so merges do not conflict. This is the only deviation from the spec's file table.
- Add stdcall rows inside the module's existing comment group in `internal/gen/stdcall.go` (create the group for a module that has none, right after the nearest related group), then `gofmt -w`.
- Edit only your module's section of `book/09-stdlib.md`. **Do not** run `book/site/build.py` and do not commit anything under `site/`; Task 8 regenerates it once.
- Gate for every task: `go build ./... && go test ./rt/std/ ./internal/gen/ ./internal/types/ ./book/` green.

## Review Focus

1. **Non-ASCII text**: `Text.padLeft 3 "é" "a"`, `Text.left 2 "héllo"`, `Text.reverse "aé😀"`, `Text.indexes "é" "éaé"`, `Text.toCodePoints "😀"` must count code points, never bytes. Pinned in Task 1.
2. **Input mutation and aliasing**: `Bytes.append a b` must not write into `a`'s spare capacity; `Bytes.slice` results must not alias; `Dict.update/union/...` must leave the argument Dict unchanged. Pinned in Tasks 2, 3, 4.
3. **Empty inputs**: `List.maximum []`, `List.tail []`, `Text.lines ""`, `Text.words "   "`, `Bytes.concat []`, `Dict.union empty empty` give the documented empty/`Nothing` value and do not panic. Pinned in Tasks 1–4.
4. **Literal edge cases**: `1e` (no exponent digits), `1.5e`, `e1`, `x.e`, `Float.e` and `2e+` must not lex as a Float with a bad value; `1e` lexes as Int `1` then identifier `e`. Pinned in Task 7.
5. **NaN**: `List.sortByFloat` with NaN keys does not panic and keeps every element (NaN sorts first, as `cmp.Compare` orders it); `Float.clamp` with NaN returns NaN; `List.maximumFloat [nan, 1]` does not panic. Pinned in Tasks 2 and 7.

---

### Task 1: Text

**Files:**
- Modify: `stdlib/Text.alder`, `internal/gen/stdcall.go` (Text group), `rt/std/text.go`, `book/09-stdlib.md` (## Text)
- Create: `rt/std/text_test.go`

**Interfaces:**
- Produces: `TextPadLeft(n int64, pad, s string) string`, `TextPadRight(n int64, pad, s string) string`, `TextLeft/TextRight/TextDropLeft/TextDropRight(n int64, s string) string`, `TextRepeat(n int64, s string) string`, `TextReverse/TextTrimLeft/TextTrimRight(s string) string`, `TextConcat(xs []string) string`, `TextLines/TextWords(s string) []string`, `TextIndexes(needle, hay string) []int64`, `TextToCodePoints(s string) []int64`, `TextFromCodePoints(xs []int64) string`.

- [ ] **Step 1: Write the failing tests** in `rt/std/text_test.go`: `TestText_Pad`, `TestText_LeftRight`, `TestText_RepeatReverseTrim`, `TestText_LinesWords`, `TestText_Indexes`, `TestText_CodePoints`. Required `eq` cases (add one per spec bullet):

```go
eq(t, "padLeft", TextPadLeft(2, "0", "7"), "07")
eq(t, "padRight", TextPadRight(5, ".", "ab"), "ab...")
eq(t, "padLeft long enough", TextPadLeft(2, "0", "123"), "123")
eq(t, "padLeft empty pad", TextPadLeft(5, "", "a"), "a")
eq(t, "padLeft multi pad cut at outer edge", TextPadLeft(5, "ab", "x"), "babax")
eq(t, "padRight multi pad", TextPadRight(5, "ab", "x"), "xabab")
eq(t, "padLeft code points", TextPadLeft(3, "é", "a"), "ééa")
eq(t, "left", TextLeft(2, "héllo"), "hé")
eq(t, "left negative", TextLeft(-1, "abc"), "")
eq(t, "left past end", TextLeft(9, "abc"), "abc")
eq(t, "right", TextRight(2, "abc"), "bc")
eq(t, "dropLeft", TextDropLeft(1, "abc"), "bc")
eq(t, "dropRight", TextDropRight(1, "abc"), "ab")
eq(t, "dropRight past end", TextDropRight(9, "abc"), "")
eq(t, "repeat", TextRepeat(3, "ab"), "ababab")
eq(t, "repeat zero", TextRepeat(0, "ab"), "")
eq(t, "repeat negative", TextRepeat(-2, "ab"), "")
eq(t, "reverse", TextReverse("aé😀"), "😀éa")
eq(t, "trimLeft", TextTrimLeft("  a "), "a ")
eq(t, "trimRight", TextTrimRight(" a \n"), " a")
eq(t, "concat", TextConcat([]string{"a", "", "bc"}), "abc")
eq(t, "lines", TextLines("a\nb\n"), []string{"a", "b"})
eq(t, "lines crlf", TextLines("a\r\nb"), []string{"a", "b"})
eq(t, "lines empty", TextLines(""), []string{})
eq(t, "lines blank middle", TextLines("a\n\nb"), []string{"a", "", "b"})
eq(t, "words", TextWords("  a  b "), []string{"a", "b"})
eq(t, "words blank", TextWords("   "), []string{})
eq(t, "indexes overlap", TextIndexes("aa", "aaa"), []int64{0, 1})
eq(t, "indexes code points", TextIndexes("é", "éaé"), []int64{0, 2})
eq(t, "indexes empty needle", TextIndexes("", "abc"), []int64{})
eq(t, "indexes none", TextIndexes("z", "abc"), []int64{})
eq(t, "toCodePoints", TextToCodePoints("A😀"), []int64{65, 0x1F600})
eq(t, "fromCodePoints", TextFromCodePoints([]int64{104, 105}), "hi")
eq(t, "fromCodePoints invalid", TextFromCodePoints([]int64{-1, 0xD800, 0x110000}), "���")
```

Empty results are non-nil empty slices (`[]string{}`), matching `ListFilter`.

- [ ] **Step 2: Run** `go test ./rt/std/ -run TestText_` — expected: build failure, functions undefined.
- [ ] **Step 3: Implement** the functions in `rt/std/text.go` (on `[]rune`; reuse the existing `trim` whitespace rule `unicode.IsSpace`).
- [ ] **Step 4: Add** the 16 signature lines from the spec to `stdlib/Text.alder`, the 16 `stdcall.go` rows, and the book lines plus one sentence each on `padLeft` with a multi-character pad, `lines` (`\r\n`, trailing newline), `indexes` (overlapping), and that `Text.split "" s` gives the characters.
- [ ] **Step 5: Run** the gate (see Parallel-execution rules) — expected: PASS (`TestStdCallsCoverStdlib` confirms every signature has a row).
- [ ] **Step 6: Commit** `git commit -m "stdlib: Text padLeft/padRight, left/right, lines, words, indexes, code points (#6)"`.

### Task 2: List

**Files:**
- Modify: `stdlib/List.alder`, `internal/gen/stdcall.go` (List group), `rt/std/list.go`, `book/09-stdlib.md` (## List)
- Create: `rt/std/list_test.go`

**Interfaces:**
- Produces: `ListIndexedMap[A, B any](f func(int64, A) B, xs []A) []B`, `ListMap2[A, B, C any](f func(A, B) C, xs []A, ys []B) []C`, `ListMap3[A, B, C, D any](f func(A, B, C) D, xs []A, ys []B, zs []C) []D`, `ListFoldr[A, B any](f func(A, B) B, acc B, xs []A) B`, `ListConcatMap[A, B any](f func(A) []B, xs []A) []B`, `ListIntersperse[A any](sep A, xs []A) []A`, `ListPartition[A any](f func(A) bool, xs []A) struct{ Yes []A; No []A }`, `ListTail[A any](xs []A) rt.Maybe[[]A]`, `ListRepeat[A any](n int64, x A) []A`, `ListMaximum/ListMinimum(xs []int64) rt.Maybe[int64]`, `ListMaximumFloat/ListMinimumFloat(xs []float64) rt.Maybe[float64]`, `ListSortByFloat[A any](key func(A) float64, xs []A) []A`.

- [ ] **Step 1: Write the failing tests** in `rt/std/list_test.go` (`TestList_Batch6*`). Required cases:

```go
eq(t, "indexedMap", ListIndexedMap(func(i int64, s string) string { return fmt.Sprint(i) + s }, []string{"a", "b"}), []string{"0a", "1b"})
eq(t, "map2 shortest", ListMap2(func(a, b int64) int64 { return a + b }, []int64{1, 2, 3}, []int64{10, 20}), []int64{11, 22})
eq(t, "map3 shortest", ListMap3(func(a, b, c int64) int64 { return a + b + c }, []int64{1, 2}, []int64{10, 20}, []int64{100}), []int64{111})
eq(t, "foldr order", ListFoldr(func(x string, acc string) string { return acc + x }, "", []string{"a", "b", "c"}), "cba")
eq(t, "concatMap", ListConcatMap(func(x int64) []int64 { return []int64{x, x} }, []int64{1, 2}), []int64{1, 1, 2, 2})
eq(t, "intersperse", ListIntersperse(int64(0), []int64{1, 2, 3}), []int64{1, 0, 2, 0, 3})
eq(t, "intersperse one", ListIntersperse(int64(0), []int64{1}), []int64{1})
eq(t, "intersperse empty", ListIntersperse(int64(0), []int64{}), []int64{})
// partition: Yes = [2,4], No = [1,3], both in input order (compare fields)
eq(t, "tail", ListTail([]int64{1, 2}), rt.Just([]int64{2}))
eq(t, "tail empty", ListTail([]int64{}), rt.Nothing[[]int64]())
eq(t, "repeat", ListRepeat(3, "x"), []string{"x", "x", "x"})
eq(t, "repeat negative", ListRepeat(-1, "x"), []string{})
eq(t, "maximum", ListMaximum([]int64{3, 9, 2}), rt.Just[int64](9))
eq(t, "minimum", ListMinimum([]int64{3, 9, 2}), rt.Just[int64](2))
eq(t, "maximum empty", ListMaximum([]int64{}), rt.Nothing[int64]())
eq(t, "maximumFloat", ListMaximumFloat([]float64{0.5, 2.5}), rt.Just(2.5))
eq(t, "minimumFloat empty", ListMinimumFloat(nil), rt.Nothing[float64]())
// sortByFloat stable: items with equal keys keep input order
// sortByFloat NaN: keys {1, NaN, 0} → NaN item first, then 0, then 1; length preserved
// map2/tail/concatMap leave inputs unchanged (check the input slice after the call)
```

`ListTail` returns a copy, not `xs[1:]`. `maximumFloat` with a NaN element must not panic (any deterministic result).

- [ ] **Step 2: Run** `go test ./rt/std/ -run TestList_Batch6` — expected: build failure.
- [ ] **Step 3: Implement** in `rt/std/list.go`; `ListSortByFloat` reuses `sortByKey`. Separate Int/Float functions, matching `ListSum`/`ListSumFloat`.
- [ ] **Step 4: Add** the 14 signatures, stdcall rows and book lines; one book sentence on `partition`'s `yes`/`no` fields and that `map2` stops at the shorter list.
- [ ] **Step 5: Run** the gate — expected: PASS.
- [ ] **Step 6: Commit** `"stdlib: List indexedMap, map2/map3, foldr, partition, maximum/minimum, sortByFloat (#6)"`.

### Task 3: Dict

**Files:**
- Modify: `stdlib/Dict.alder`, `internal/gen/stdcall.go` (Dict group), `rt/std/dict.go`, `book/09-stdlib.md` (## Dict)
- Create: `rt/std/dict_test.go`

**Interfaces:**
- Consumes: `rt.Dict` methods `Get`, `Has`, `Keys`, `Len`, `Insert`, `Remove`, `Put` (`rt/core.go`); existing `DictMember` (rewrite its comment, it is now in the stdlib).
- Produces: `DictSingleton[K, V any](k K, v V) rt.Dict[K, V]`, `DictIsEmpty`, `DictMember(k K, d) bool`, `DictUpdate[K, V any](k K, f func(rt.Maybe[V]) rt.Maybe[V], d rt.Dict[K, V]) rt.Dict[K, V]`, `DictMap[K, V, W any](f func(K, V) W, d rt.Dict[K, V]) rt.Dict[K, W]`, `DictFilter(f func(K, V) bool, d) rt.Dict[K, V]`, `DictFoldl[K, V, B any](f func(K, V, B) B, acc B, d rt.Dict[K, V]) B`, `DictPartition(f func(K, V) bool, d) struct{ Yes rt.Dict[K, V]; No rt.Dict[K, V] }`, `DictUnion/DictIntersect/DictDiff(a, b rt.Dict[K, V]) rt.Dict[K, V]`.

- [ ] **Step 1: Write the failing tests** in `rt/std/dict_test.go`. Compare order with `DictToList(...)`, since `Equal` ignores order. Required cases, from `d := DictFromList` of `a:1, b:2, c:3`:
  - `singleton` → toList `[{a 1}]`; `isEmpty` on `DictEmpty` true, on `d` false; `member "b"` true, `"z"` false.
  - `update "b" (Just 20)` → order `a, b, c` with `b = 20`; `update "z" (Just 9)` → `z` appended last; `update "b" (always Nothing)` → `a, c`; `update "z" (always Nothing)` → unchanged; `f` receives `Just 2` for `"b"` and `Nothing` for `"z"`.
  - `map` (k, v → k ++ show v) keeps order; `filter` (v odd) → `a, c`; `foldl` (k, v, acc → acc ++ k) → `"abc"`.
  - `partition` (v odd) → Yes `a, c`, No `b`.
  - `union a b` with a = `{x:1, y:2}`, b = `{y:20, z:30}` → `x:1, y:2, z:30` in that order; `intersect a b` → `y:2`; `diff a b` → `x:1`; `union` of two empties is empty.
  - **No mutation:** after every call above, `DictToList(d)` still equals `a:1, b:2, c:3`.
- [ ] **Step 2: Run** `go test ./rt/std/ -run TestDict_` — expected: build failure.
- [ ] **Step 3: Implement** in `rt/std/dict.go`, building results with a fresh `var out rt.Dict[...]` and `out.Put`; `DictUpdate` uses `d.Insert`/`d.Remove` (which already keep position and copy).
- [ ] **Step 4: Add** the 11 signatures, stdcall rows and book lines; book sentences on `update` (position kept, new key appended, `Nothing` removes) and `union` preferring the first dict's value with its order.
- [ ] **Step 5: Run** the gate — expected: PASS.
- [ ] **Step 6: Commit** `"stdlib: Dict update, map, filter, foldl, partition, union/intersect/diff (#6)"`.

### Task 4: Bytes

**Files:**
- Modify: `stdlib/Bytes.alder`, `internal/gen/stdcall.go` (`// File, Bytes` group), `book/09-stdlib.md` (## File, Bytes)
- Create: `rt/std/bytes.go` (move `BytesLength` and `BytesToText` here from `file.go`), `rt/std/bytes_test.go`

**Interfaces:**
- Consumes: the index rule of `TextSlice` in `rt/std/text.go`. Factor its index normalisation into a shared unexported helper `sliceBounds(start, end int64, length int) (int, int)` used by both, without changing `TextSlice` behaviour.
- Produces: `BytesEmpty() []byte`, `BytesIsEmpty(b []byte) bool`, `BytesSlice(start, end int64, b []byte) []byte`, `BytesAppend(a, b []byte) []byte`, `BytesConcat(bs [][]byte) []byte`, `BytesToList(b []byte) []int64`, `BytesFromList(xs []int64) []byte`.

- [ ] **Step 1: Write the failing tests** in `rt/std/bytes_test.go`. Required cases (b = `[]byte{1,2,3,4}`):

```go
eq(t, "empty", BytesEmpty(), []byte{})
eq(t, "isEmpty", BytesIsEmpty(BytesEmpty()), true)
eq(t, "slice", BytesSlice(1, 3, b), []byte{2, 3})
eq(t, "slice negative", BytesSlice(-2, 4, b), []byte{3, 4})
eq(t, "slice clamps", BytesSlice(-9, 99, b), []byte{1, 2, 3, 4})
eq(t, "slice empty when start >= end", BytesSlice(3, 1, b), []byte{})
eq(t, "append", BytesAppend([]byte{1}, []byte{2}), []byte{1, 2})
eq(t, "concat", BytesConcat([][]byte{{1}, {}, {2, 3}}), []byte{1, 2, 3})
eq(t, "concat empty", BytesConcat(nil), []byte{})
eq(t, "toList", BytesToList([]byte{0, 255}), []int64{0, 255})
eq(t, "fromList masks", BytesFromList([]int64{256, -1, 65}), []byte{0, 255, 65})
```

Plus the aliasing cases: `a := make([]byte, 1, 8); a[0] = 1; x := BytesAppend(a, []byte{2}); y := BytesAppend(a, []byte{3})` leaves `x == {1,2}`; mutating `BytesSlice(0, 2, b)[0]` leaves `b[0] == 1`. Add one `TextSlice` regression case per existing index rule if `text_test.go`/`std_test.go` lacks one, since its helper is refactored.

- [ ] **Step 2: Run** `go test ./rt/std/ -run 'TestBytes_|TestText'` — expected: build failure.
- [ ] **Step 3: Implement** in `rt/std/bytes.go`; every result is a fresh slice.
- [ ] **Step 4: Add** the 7 signatures, stdcall rows and book lines; book sentences on `slice` using `Text.slice`'s rule in bytes and `fromList` keeping the low eight bits.
- [ ] **Step 5: Run** the gate — expected: PASS.
- [ ] **Step 6: Commit** `"stdlib: Bytes empty, slice, append, concat, toList/fromList (#6)"`.

### Task 5: Date, Time

**Files:**
- Modify: `stdlib/Date.alder`, `stdlib/Time.alder`, `internal/gen/stdcall.go` (Date and Time groups), `rt/std/date.go`, `book/09-stdlib.md` (## Date, Time)
- Create: `rt/std/date_test.go`

**Interfaces:**
- Consumes: `rt.Date` (`Y`, `M`, `D`, `Time()`), `rt.DateOf(time.Time)`.
- Produces: `DateFromParts(p struct{ Year int64; Month int64; Day int64 }) rt.Maybe[rt.Date]`, `DateToParts(d rt.Date) struct{ Year int64; Month int64; Day int64 }`, `DateEpoch() rt.Date`, `DateWeekday(d rt.Date) int64`, `DateAddMonths(n int64, d rt.Date) rt.Date`, `DateAddYears(n int64, d rt.Date) rt.Date`, `TimeFromDate(d rt.Date) time.Time`.

- [ ] **Step 1: Write the failing tests** in `rt/std/date_test.go`. Required cases (build dates with `DateFromText`):
  - `fromParts {2026,10,29}` = `Just 2026-10-29`; `{2026,13,1}`, `{2026,0,1}`, `{2026,2,29}`, `{2026,4,31}`, `{2026,1,0}` = `Nothing`; `{2024,2,29}` = `Just`.
  - `toParts 2026-10-29` = `{2026, 10, 29}`; `toParts` ∘ `fromParts` round-trips.
  - `epoch` = `1970-01-01`.
  - `weekday`: `2026-10-26` (Monday) = 1, `2026-11-01` (Sunday) = 7, `1970-01-01` = 4.
  - `addMonths 1 2026-01-31` = `2026-02-28`; `addMonths 1 2024-01-31` = `2024-02-29`; `addMonths -1 2026-03-31` = `2026-02-28`; `addMonths 12 2026-05-15` = `2027-05-15`; `addMonths -13 2026-01-15` = `2024-12-15`.
  - `addYears 1 2024-02-29` = `2025-02-28`; `addYears 4 2024-02-29` = `2028-02-29`; `addYears -1 2026-06-01` = `2025-06-01`.
  - `TimeFromDate 1970-01-02` = `TimeFromMillis(86400000)`, location UTC.
- [ ] **Step 2: Run** `go test ./rt/std/ -run TestDate_` — expected: build failure.
- [ ] **Step 3: Implement** in `rt/std/date.go`. `fromParts` rejects by round-trip (build with `time.Date`, `Nothing` unless year/month/day come back unchanged). `addMonths` computes the target year/month with floor division, then clamps the day to that month's length; do **not** use `AddDate` (it normalises Jan 31 + 1 month to March). `addYears n` is `addMonths (12 * n)`.
- [ ] **Step 4: Add** the 6 Date and 1 Time signatures, stdcall rows and book lines; book sentences on `weekday` numbering (1 = Monday) and `addMonths` clamping.
- [ ] **Step 5: Run** the gate — expected: PASS.
- [ ] **Step 6: Commit** `"stdlib: Date fromParts/toParts, epoch, weekday, addMonths/addYears; Time.fromDate (#6)"`.

### Task 6: Html

**Files:**
- Modify: `stdlib/Html.alder`, `internal/gen/stdcall.go` (Html group), `rt/std/html.go`, `book/09-stdlib.md` (## Html)
- Create: `rt/std/html_test.go`

**Interfaces:**
- Consumes: `html.Render(n html.Node) string` (used by `FileHtml`).
- Produces: `HtmlToText(n html.Node) string`.

- [ ] **Step 1: Write the failing test** `TestHtml_ToText`: `HtmlToText(HtmlText("a<b"))` = `"a&lt;b"`; `HtmlToText(HtmlFragment([]html.Node{HtmlUnsafe("<p>"), HtmlText("x")}))` = `"<p>x"`; and `HtmlToText(n)` equals `string(FileHtml(n).Body)` for one element node.
- [ ] **Step 2: Run** `go test ./rt/std/ -run TestHtml_` — expected: build failure.
- [ ] **Step 3: Implement** `HtmlToText` as `html.Render(n)`.
- [ ] **Step 4: Add** the signature `toText : Html Never -> Text` with a comment line (the markup `File.html` serves), the stdcall row and the book line plus one sentence on why it takes `Html Never`.
- [ ] **Step 5: Run** the gate, plus a type-check test proving `Html.toText` rejects a view with messages: add a `book/09-stdlib.md` snippet or an `internal/types` test case in the style of nearby tests, whichever the package already does for `File.html`. Expected: PASS.
- [ ] **Step 6: Commit** `"stdlib: Html.toText (#6)"`.

### Task 7: Float, Int and exponent literals

**Files:**
- Modify: `stdlib/Float.alder`, `stdlib/Int.alder`, `internal/gen/stdcall.go` (Float, Int groups), `rt/std/num.go`, `book/09-stdlib.md` (## Int, Float), `internal/syntax/lexer.go:202-212`, the lexer test file in `internal/syntax/`, `tree-sitter-alder/grammar.js:432` and its generated `src/`, `tree-sitter-alder/test/corpus/expressions.txt`
- Create: `rt/std/num_test.go`, `testdata/corpus/float-exponent-ok/` (an `app/` that uses `1.5e-3`, `1e6`, `2E+3` and an `expected.diag` following the other `*-ok` cases)

**Interfaces:**
- Produces: `FloatTruncate(f float64) int64`, `FloatClamp(r struct{ Min float64; Max float64 }, f float64) float64`, `FloatPi() float64`, `FloatE() float64`, `FloatIsNaN/FloatIsInfinite(f float64) bool`, `FloatLogBase(base, x float64) float64`, `FloatSin/Cos/Tan/Asin/Acos/Atan(f float64) float64`, `FloatAtan2(y, x float64) float64`, `IntPow(base, exp int64) int64`.

- [ ] **Step 1: Write the failing tests.** In `rt/std/num_test.go`:

```go
eq(t, "truncate pos", FloatTruncate(2.7), int64(2))
eq(t, "truncate neg", FloatTruncate(-2.7), int64(-2))
eq(t, "clamp below", FloatClamp(struct{ Min, Max float64 }{0, 1}, -0.5), 0.0)
eq(t, "clamp above", FloatClamp(struct{ Min, Max float64 }{0, 1}, 1.5), 1.0)
eq(t, "clamp inside", FloatClamp(struct{ Min, Max float64 }{0, 1}, 0.25), 0.25)
eq(t, "clamp min > max like Int.clamp", FloatClamp(struct{ Min, Max float64 }{5, 1}, 3), 5.0)
eq(t, "pi", FloatPi(), math.Pi)
eq(t, "e", FloatE(), math.E)
eq(t, "isNaN", FloatIsNaN(math.NaN()), true)
eq(t, "isInfinite", FloatIsInfinite(math.Inf(-1)), true)
eq(t, "isInfinite finite", FloatIsInfinite(1), false)
eq(t, "logBase", FloatLogBase(10, 1000), 3.0) // allow 1e-12 tolerance
eq(t, "atan2", FloatAtan2(1, 1), math.Pi/4)
eq(t, "pow", IntPow(2, 10), int64(1024))
eq(t, "pow zero exp", IntPow(0, 0), int64(1))
eq(t, "pow negative exp", IntPow(2, -1), int64(0))
eq(t, "pow negative base", IntPow(-3, 3), int64(-27))
eq(t, "pow wraps like *", IntPow(2, 64), int64(0))
```

`FloatClamp` with a NaN argument returns NaN (check with `math.IsNaN`). The `struct{ Min, Max float64 }` literal must match the generated anonymous struct type; if `IntClamp`'s tests use a different literal form, follow that. In the lexer test: `1.5e-3` → one `Float` token `"1.5e-3"`; `1e6` → `Float "1e6"`; `2E+3` → `Float "2E+3"`; `1e` → `Int "1"` then `LIdent "e"`; `1.5e` → `Float "1.5"` then `LIdent "e"`; `2e+` → `Int "2"`, `LIdent "e"`, then `+`; `e1` → `LIdent`. In `internal/types` or the parser tests: `1e6` type-checks as `Float`. In `text_test.go` or `num_test.go`: `TextToFloat("1.5e-3")` = `Just 0.0015`.

- [ ] **Step 2: Run** `go test ./rt/std/ ./internal/syntax/` — expected: FAIL.
- [ ] **Step 3: Implement** the num functions (`math` package; `IntPow` by repeated squaring with wrapping `*`). In the lexer, after the digits and optional fraction, consume an exponent only when `e|E` is followed by a digit, or by `+|-` and then a digit; a literal with an exponent is a `Float`. The parser's `strconv.ParseFloat` already reads it.
- [ ] **Step 4: Update the grammar:** `float: ($) => /[0-9]+(\.[0-9]+)?[eE][+-]?[0-9]+|[0-9]+\.[0-9]+/` (keep `integer` from matching the prefix: tree-sitter prefers the longer token). Run `scripts/ts.sh generate` (CLI 0.27.1 only), add a corpus case for `1.5e-3`, `1e6`, `2E+3` to `test/corpus/expressions.txt`, then `scripts/ts.sh test` and `go test ./tree-sitter-alder`. Expected: PASS.
- [ ] **Step 5: Add** the Float (13) and Int (1) signatures, stdcall rows, the `float-exponent-ok` corpus case, and the book lines plus sentences on `Int.pow` with a negative exponent, `Float.clamp` matching `Int.clamp`, and exponent literals (always `Float`; `Text.toFloat` reads them; `Text.fromFloat` output unchanged).
- [ ] **Step 6: Run** the gate plus `go test ./internal/syntax/ ./tree-sitter-alder/ ./internal/format/` and the corpus test package (find it with `grep -rl testdata/corpus --include=*_test.go`). Expected: PASS.
- [ ] **Step 7: Commit** `"stdlib: Float trig, clamp, truncate, pi/e; Int.pow; exponent float literals (#6)"`.

### Task 8: Integrate, regenerate, verify (main session)

**Files:**
- Modify: `site/09-stdlib.html`, `site/reference.md`, `site/reference-full.md` (generated)

- [ ] **Step 1:** Merge each task branch into `stdlib-batch-6` in task order; resolve `stdcall.go` and book conflicts by keeping both sides, then `gofmt -w internal/gen/stdcall.go`.
- [ ] **Step 2:** `python3 book/site/build.py`; commit the regenerated `site/` files.
- [ ] **Step 3:** `go vet ./... && go test ./...` — expected: all green.
- [ ] **Step 4:** Scratch check in the scratchpad (not in nightvote-replay): a small Alder program, built with this branch's `go run ./cmd/alder`, whose tests assert `Text.padLeft 2 "0" (Text.fromInt 7) == "07"`, `Text.toCodePoints "A" == [65]`, `1.5e-3 == 0.0015`, `Date.toText Date.epoch == "1970-01-01"`, `Bytes.length (Bytes.slice 0 2 (Text.toBytes "abcd")) == 2`, and `Html.toText (Html.text "x") == "x"`. Expected: tests pass.
- [ ] **Step 5:** Final whole-branch review (Fable, per the user's model policy), fix findings, push, open the PR closing #6, tick #6 on roadmap #52, and tell the user to `go install ./cmd/alder` and `:LspRestart` after merging.
