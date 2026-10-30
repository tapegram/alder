# Nested patterns (#2) implementation plan

Spec: `docs/superpowers/specs/2026-10-29-nested-patterns-design.md` (binding). Every task is
test-first: write or change the test, see it fail, write the code, see it pass, commit.

## Task 1: parser

- Test: add to `testdata/golden/parse_exprs.alder` a value `jn` whose match has the arms
  `[Just m, Just s] ->`, `{ a: Just x, b: [_, ..r] } ->`, `Just [a, ..rest] ->`,
  `Just (Just x) ->`, `[[a], b] ->`, `_ ->`. `go test ./internal/syntax -run Golden` fails (parse
  errors in the dump); regenerate with `-update` after the fix and read the dump.
- Code: `internal/syntax/parser_expr.go`: `parsePattern`'s constructor case also takes `LBracket`
  as the start of an argument; the list-element and record-field loops call `parsePattern`.
- Commit "Parser: nested patterns in lists, records and constructor arguments (#2)".

## Task 2: formatter

- Test: `testdata/golden/fmt_match.alder` gains `[Just m, Just s]`, `{ a: Just x, b: [_, ..r] }`
  record-subject and `Just [a, ..rest]` arms; `fmt_match.golden` expects
  `Pair { left: Just a, right: _ }`, `Many [first, Nothing, ..rest]`, `Wrapped []`, and the new arms
  bare. `go test ./internal/format` fails.
- Code: `internal/format/print_expr.go` `subPattern`: parenthesise an applied constructor and a
  negative literal only when `ctorArg`; never a list.
- Commit "Formatter: print nested patterns bare where the grammar allows (#2)".

## Task 3: usefulness (Maranget) core

- Test: new `internal/check/pattern_test.go` on hand-built normalised rows (no source):
  `TestUseful_FlatUnion`, `TestUseful_ListOfMaybe` (the issue's three-element match lists the seven
  witnesses of the spec, in order), `TestUseful_Literals` (never complete), `TestUseful_Record`,
  `TestUseful_Redundant`, `TestWitnessCap` (more than 8 missing → 8 and `, …`).
- Code: `internal/check/pattern.go`: a `pat` struct (`kind` wild/ctor, `tag`, `args`, `src`), a
  `sig` per column type (union, Bool, List, record, literal = infinite, other = none),
  `normalise(syntax.Pattern, type) pat`, `useful(rows, q) bool`, `witness(rows, n) []pat`,
  `printWitness(pat, top bool)`.
- Commit "Check: pattern usefulness on matrices (#2)".

## Task 4: wire it into match checking, goldens byte-identical

- Test: `go test ./internal/check ./internal/corpus...` with the existing `.diag` goldens and
  `testdata/corpus/*/expected.diag` unchanged must stay green after the swap.
- Code: delete `exhaustiveness`, `cover`, `refresh`, `report`'s table and the
  "patterns nest one level" error; `checkMatch` and `html.go` `HtmlMatch` check all arms, then call
  `mc.checkArms(subject, match)` which reports E0310 per arm (text: "every case is already handled"
  when the rows above are exhaustive, else "`<pattern>` is already handled" printed by
  `format`'s pattern printer) and E0004/E0302 from the witnesses with today's fix placement.
- Commit "Check: exhaustiveness and redundancy by usefulness (#2)".

## Task 5: nested diagnostics

- Test: `testdata/golden/check_exprs/app/Todo.alder` gains `nest1` (the issue's match: E0004 with
  the spec's exact text), `nest2` (an unreachable nested arm `[Just _] ` after `[Just x]`, E0310
  with its pattern), `nest3` (record subject, refutable arms only: E0004 now reported); update
  `check_exprs.diag` (only new lines appear). New corpus case `testdata/corpus/E0004-nested-list/`
  (`input`/`expected.diag` like `E0004-missing-case`).
- Code: as needed in the witness printer; `internal/diag/explain.go` E0004 and E0310 get one
  sentence each on nested cases and `_`.
- Commit "Diagnostics: nested missing cases and unreachable arms (#2)".

## Task 6: lowering, gen_vet golden, runtime cases

- Test: `testdata/golden/ir_patterns/app/P.alder` gains `sum3 : List (Maybe Int) -> Int`
  (the issue's match, `_ -> -1` fallback), `inner : Maybe (Maybe Int) -> Int` (`Just (Just x)`),
  `heads : Maybe (List Int) -> Int` (`Just [a, ..r]`); `TestGen_Run`'s `TestPatterns` asserts
  `sum3 [Just 1, Just 2, Just 3]`, `[Just 1, Nothing, Just 3]`, `[]`, `[Just 1]`, and the pair.
  `ir_patterns.ir` regenerated with `-update` and read. New
  `testdata/golden/gen_vet/n1-nested-patterns/app/M.alder` (nested list/record/ctor matches,
  HTML match) for `TestGenVetCorpus`.
- Code: `internal/ir/lower_match.go` header comment only (if the run passes; otherwise fix).
- Commit "Gen: nested pattern programs run and vet (#2)".

## Task 7: tree-sitter

- Test: `tree-sitter-alder/test/corpus/match.txt` gains a case with `[Just m, Just s, Just p]`,
  `{ left: Just a, right: _ }` and `Just [a, ..rest]`; `scripts/ts.sh test` fails.
- Code: `grammar.js` `_pattern_argument` gains `$.list_pattern`; `scripts/ts.sh generate`
  (CLI 0.27.1 only); `go test ./tree-sitter-alder/...` (drift test) green.
- Commit "tree-sitter: list pattern as a constructor argument (#2)".

## Task 8: book

- `book/A-grammar.md` pattern rules replaced with the spec's four rules; `book/03-syntax.md` the
  "do not nest" paragraph becomes the nesting rule with the issue's example and the parenthesis
  rule (the book's code blocks are checked by its tests). Run `python3 book/site/test_site.py`
  and `cd experiments && python3 -m unittest`; if the reference budget is exceeded, raise the cap in
  `experiments/test_build_context.py` with a comment naming #2.
- Commit "Book: nested patterns (#2)".

## Finish

`go vet ./...`, `go test ./...`, `scripts/ts.sh test`, `cd experiments && python3 -m unittest`;
the scratch check of the issue's `"1 2 3"`, `"1 x 3"`, `""`, `"1 2 3 4"`; push; PR.
