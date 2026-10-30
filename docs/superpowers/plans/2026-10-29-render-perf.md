# Plan: large-output rendering (#37)

Spec: `docs/superpowers/specs/2026-10-29-render-perf-design.md` (binding). TDD per task: failing
test, code, green, commit. New unit tests go in new files (PR #71 also edits `rt/std/text.go`,
`rt/std/dict.go`, `rt/std/list_test.go`, `rt/std/text_test.go`), and edits to `rt/std/text.go` are
new functions at the end of the file.

## Task 0: baseline benchmarks

Write the benchmarks first (files below) against today's code and record
`go test ./rt/... -run XXX -bench 'DictGet|ListTakeDrop|TextJoinNested|ListUnique' -benchtime=200ms`
in the scratchpad, for the PR body (`TextBuild` has no "before": it is new).

## Task 1: list views (`rt/std/list.go`, `rt/core.go`)

Tests, new file `rt/std/views_test.go`:
- `TestListViews_Nil`: take/drop of nil are empty (`rt.Eq` to `[]int64{}`), tail of nil is Nothing,
  tail of `[1]` is `Just []`.
- `TestListViews_NoAlias`: append to each of take/drop/tail results leaves the input unchanged
  (input built with spare capacity, so a view with spare capacity would be caught).
- `TestListViews_SourceAppend`: append to the source (spare capacity) leaves earlier views
  unchanged; each view has `cap == len`.
- `TestAppend_EmptySide` (new file `rt/views_test.go`): `[] ++ xs`, `xs ++ []`, `nil ++ xs` Eq xs,
  append to the result leaves xs unchanged, `cap == len`; `[1] ++ [2]` fresh.
- `BenchmarkListTakeDrop`: 100k 48-byte records, take 50k + drop 50k.
Code: `ListTake`/`ListDrop` return three-index views; `ListTail` = `Just(ListDrop(1, xs))`;
`rt.Append` returns the capped other side when one is empty. Doc comments state the invariant.

## Task 2: the "never written" audit (`rt/noalias_test.go`)

`TestNoWritesToParameters`: parses every non-test `.go` file under `rt/` with `go/ast` and fails on
any function that, on one of its own parameters (or a reslice of one), does `p[i] = …`,
`append(p, …)`, `copy(p, …)`, `slices.Sort/SortFunc/SortStableFunc/Reverse(p)`, or
`clear(p)`. Known safe exceptions are listed with a reason. This is what keeps views (task 1) and
aliased File bodies (task 5) sound as the runtime grows.

## Task 3: Dict key fast path (`rt/core.go`)

Tests, new file `rt/dictkeys_test.go`:
- `TestDictIntKeys`: insert/get/member/remove/keys order with 0, -1, 1, MinInt64, MaxInt64; -1 and
  1 distinct; re-insert keeps position.
- `TestDictTextKeys`: `""` is a key; order kept.
- `TestDictAnyKeyType`: a `Dict[any, int64]` holding `"aaaaaaaa"` and an int64 with the same 8
  bytes keeps them distinct (the fast path is chosen by the static K).
- `TestDictAsKey_OrderInsensitive`: Key of two `Dict[string,int64]` built in different orders equal.
- `TestKey_Unchanged`: `Key("a") == "s1:a"`, `Key(int64(5)) == "i5;"`, `Key(int64(-3)) == "i-3;"`.
- `TestDictGet_Allocs`: `testing.AllocsPerRun` of Get/Has on Text and Int keys is 0.
- `BenchmarkDictGet_Text`, `_Int`, `_Record`.
Code: `dictEnc[K](k)` switching on `any(&k).(type)` (`*string`, `*int64`, default `Key`), inline
switch in Get/Has with a stack `[8]byte`; Put/Remove use `dictEnc`; `Key` fast path for `string`
and `int64` writing today's forms.

## Task 4: Text.Builder

- `rt/builder.go`: `TextBuilder{n *tbNode}`, `Add`, `Build` (one allocation, fill from the end,
  `unsafe.String`), `AlderEqual`, `AlderKey`, `AlderShow`.
- `rt/std/text.go` (appended at end): `TextBuilder()`, `TextAdd`, `TextBuild`.
- `stdlib/Text.alder`: `type Builder`, `builder`, `add`, `build`.
- `internal/gen/stdcall.go`: three rows; `internal/gen/types.go`: `Builder` of `Text` →
  `rt.TextBuilder` + doc table row.
Tests, new file `rt/std/builder_test.go`: empty build; `add ""` is b; three adds in order; build
twice; fork; a million adds; Eq true/false; Key equal for equal text; Show of empty and `"ab"`;
`TextBuilder()` is the zero value. `BenchmarkTextJoinNested` and `BenchmarkTextBuild` (10 items ×
10 options × 100 rows of ~335 B). Corpus `testdata/corpus/text-builder-ok/` (a File page whose body
is built with `List.foldl Text.add Text.builder` and `addRow : Row -> Text.Builder ->
Text.Builder`); `TestStdCallsCoverStdlib` green.

## Task 5: File bodies alias (`rt/std/file.go`, `rt/file.go`)

Tests, new file `rt/std/filealias_test.go`: `&FileText(..).Body[0] == unsafe.StringData(body)`,
same for `FileJson`, `FileHtml`; `""` gives an empty non-nil body; `BytesSlice`/`BytesAppend` of the
body do not share storage. Code: a `bytesOf(s)` helper with `unsafe.Slice`; doc comment on
`rt.File.Body` stating the invariant.

## Task 6: book, site, verification

`book/09-stdlib.md`: Text.Builder lines + paragraph (separate block after the Text block, away
from #71's edits); one sentence under List on `++` in a loop. `python3 book/site/build.py`;
`cd experiments && python3 -m unittest`. `go vet ./...`, `go test ./...`. Benchmarks after; PR.
