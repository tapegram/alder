# Nested patterns: constructors, lists and literals at any depth

Design for [tapegram/alder#2](https://github.com/tapegram/alder/issues/2). The gap was found by
Claude while building nightvote-replay: `match parts |> List.map Text.toFloat { [Just m, Just s,
Just p] -> … }` is a parse error, and the workaround (`List.filterMap`, a length check, then
`[m, s, p]`) hides the intent and gets the edge cases subtly different. Everything marked
**(call)** is the author's decision, not the user's.

## Intent

Patterns nest fully, as in Elm: a constructor, list, record, literal, wildcard or variable may
stand wherever a pattern is expected, at any depth. The checker proves every such match
exhaustive and flags unreachable arms with the standard usefulness algorithm, and the missing-case
message prints a real pattern the reader can paste.

**Full nesting, not the issue's fallback (`Just`/`Ok`/`Err` inside list patterns) (call).** Three
reasons. The IR already compiles arbitrary nesting: `internal/ir/lower_match.go` is a recursive
backtracking decision-tree compiler, and `testdata/golden/gen_vet/o2-maybe-recursion` already
nests a constructor inside a record inside a constructor through the record loophole. The parser
change is a few lines (`parsePattern1` takes a bare constructor without its argument, which is the
whole reason `[Just m]` fails). And the hard part, exhaustiveness, is no easier for the restricted
form: the issue's own example needs `[]`, `[Nothing, ..rest]`, `[Just _]`, `[Just _, Nothing, ..rest]`,
… as missing cases, which the current flat per-kind table cannot express, so a general algorithm is
needed either way. That algorithm (below) is shorter than the table it replaces.

## Goals

1. `[Just m, Just s, Just p] -> …` and the other forms in the grammar below parse, check, format,
   highlight and compile.
2. Exhaustiveness and redundancy are exact for every nesting, including record subjects, which
   today are never reported as non-exhaustive (`report` returns for kind `record`).
3. Every existing program keeps compiling, with the same diagnostics text where the match is flat:
   the golden `.diag` files and E0302 (`update` does not handle …) are unchanged.
4. Nested matches run with first-match-wins semantics and no extra allocation.

## Non-goals

- `x :: rest` cons syntax. Alder's spelling is `[x, ..rest]` and stays the only one.
- `as` patterns (`whole as Just x`), or-patterns, guards, `Float` literal patterns (the parser
  takes `Int`, `String` and `-Int` today and keeps to that), negative literals without parens as
  a constructor argument.
- Nested positions in the LSP's arm completion (`internal/lsp/complete.go` `patterns()` completes
  constructor names at the top of an arm only). Pattern binders, references and rename already
  recurse through every pattern form and need no change.
- A duplicate name inside one pattern (`[x, x]`). Whatever the checker does today (the second
  binding shadows the first) is unchanged; a lint is a separate issue.

## Anti-goals

- A depth limit of any kind, or a nesting rule that differs between list elements, record fields
  and constructor arguments beyond the parenthesisation rule below.
- Pattern compilation that duplicates arm bodies. Fallbacks are shared nodes, as today.

## Grammar

`book/A-grammar.md` replaces its pattern rules with:

```
pattern     = "_" | literal | lname | uname [pattern1] | recordPat | listPat | "(" pattern ")" ;
pattern1    = "_" | literal | lname | uname | recordPat | listPat | "(" pattern ")" ;
recordPat   = "{" {lname ":" pattern ","} [lname ":" pattern] "}" ;
listPat     = "[" [pattern {"," pattern}] ["," ".." lname] "]" | "[" ".." lname "]" ;
```

The rules **(call)**:

- A constructor's argument is `pattern1`: a bare `uname` is a nullary constructor, so an applied
  constructor needs parentheses, `Just (Just x)`, as in Elm. A list may be an argument without
  parentheses, `Just [a, ..rest]` (no ambiguity: `[` never starts anything else after a
  constructor in a pattern). A negative literal still needs them, `Just (-1)`, mirroring the
  expression grammar where `f -1` is subtraction.
- List elements and record fields take a full `pattern`, so `[Just m, Just s]` and
  `{ left: Just a, right: _ }` need no parentheses; the element ends at `,`, `]` or `}`.
- Parentheses are allowed around any pattern (they already are; the grammar now says so).

Parser: `parsePattern1` keeps returning a bare constructor for `uname`; `parsePattern`'s
constructor case adds `LBracket` to the tokens that start an argument; the list and record-field
loops call `parsePattern` instead of `parsePattern1`. `PCtor`, `PList`, `PRecord` already hold
`Pattern` children, so the AST, `dump.go`, `walk.go` and the LSP are untouched.

`tree-sitter-alder/grammar.js`: `list_pattern` and `field_pattern` already take `$._pattern`;
`_pattern_argument` gains `$.list_pattern`. Regenerate and add a corpus case to
`test/corpus/match.txt` with the three examples above.

Formatter (`internal/format/print_expr.go` `subPattern`): an applied constructor is parenthesised
only as a constructor argument; list elements and record fields print it bare, and a list as a
constructor argument prints bare. `testdata/golden/fmt_match.golden` changes accordingly
(`Pair { left: (Just a), right: _ }` becomes `Pair { left: Just a, right: _ }`). A list pattern
stays on one line at any depth **(call)**: patterns are short, and the "no line break inside a
list pattern" rule in the grammar is kept.

## Semantics

Unchanged and now stated for depth: arms are tried in order, the first whose whole pattern
matches runs; within a pattern, parts are tested left to right (list elements, then the rest;
record fields in pattern order; a constructor's payload after its tag). Variables bind in the
arm's scope exactly as flat patterns do. A value matched by a nested pattern is never copied:
the lowering binds parts to temporaries and names them with `Let`s, as today.

## Checking

`checkPattern` (`internal/check/pattern.go`) already types every form recursively; the only
change is deleting the `patterns nest one level; match on x in a second match` error. The
`exhaustiveness` struct, `cover`, `refresh` and `report` are replaced.

### Usefulness (Maranget) **(call)**

The replacement is the classic `useful(P, q)` on pattern matrices, run once per match after all
its arms are checked (so the subject's type is fully refined and `refresh` is no longer needed).
`checkMatch` and the `HtmlMatch` case in `html.go` both call it.

Patterns are first normalised into one shape, `(tag, args)` or wildcard, driven by the subject
type after `u.Apply`:

| type | constructors (signature) | arity |
|---|---|---|
| union | its declared constructors, declaration order | 1 if it carries a payload, else 0 |
| `Bool` | `True`, `False` | 0 |
| `List a` | `[]`; `cons` | 0; 2 (head `a`, tail `List a`) |
| record | one constructor, the record itself | its field count, in the type's field order |
| `Int`, `Text`, `Float`, brands | each literal is its own constructor; the signature is infinite | 0 |
| anything else (functions, `Dict`, effects, unresolved `Var`, poison) | none | — |

Normalisation: `[p1, …, pn]` is `cons(p1, cons(…, cons(pn, [])))`; `[p1, …, pn, ..r]` is the same
with `r` a wildcard as the innermost tail; `[..r]` is a wildcard. A record pattern is its single
constructor with a wildcard for every field it does not name. `PVar`, `PWild` and nil are
wildcards. The poison type short-circuits as today (nothing is reported).

`useful(P, q)` is the standard recursion: with no columns, useful iff `P` has no rows; if `q`'s
first column is a constructor `c`, recurse on the specialisation `S(c, P)`; if it is a wildcard,
either the column's constructors in `P` form a complete signature and `useful` must hold for some
`S(c, P)`, or they do not and the default matrix is used. A literal column is never complete.
Types "anything else" have no constructors, so only the default-matrix branch applies.

- **Redundancy (E0310).** Arm `i` is unreachable iff `useful(rows 0..i-1, p_i)` is false.
- **Exhaustiveness (E0004 / E0302).** The match is exhaustive iff `useful(P, _)` is false. When
  it is useful, the algorithm is run in witness-producing form (returning the pattern vector it
  built, choosing the first missing constructor in signature order). To list every missing case
  as today's messages do, the witness is appended to `P` as an extra row and the search repeats
  until nothing is useful, capped at 8 witnesses **(call)**; past the cap the message ends with
  `, …`. Each witness is a pattern that covers strictly more, so the loop terminates.

No complexity cap **(call)**: usefulness is exponential only in pathological matrices, and an
Alder match is one function's arms; the builder adds the issue's three-element case and a
20-arm flat `Msg` update to the existing check benchmarks to confirm nothing measurable changed.

### Diagnostics

The messages keep their shape; only what a case prints changes, and only when it nests:

- A witness prints as a pattern: constructor names, `[` `]`, `{ field: p }` with only the
  constrained fields (an all-wildcard record is `_`), `_` for an unconstrained part, `..rest` for
  an unconstrained list tail, literals as written. Sub-wildcards are never given names **(call)**:
  `[Nothing, ..rest]`, `[_, Nothing, ..rest]`, `Just [_, ..rest]`, `{ next: Just { next: Nothing } }`.
- The top-level spellings existing programs see are kept verbatim: a missing constructor with a
  payload prints `Name x` (`Name p` inside `update`, for E0302), a missing cons prints
  `[x, ..rest]`, a missing nil `[]`, Bools `True`/`False`, an uncovered literal column `_`. So for
  the issue's example `match xs { [Just m, Just s, Just p] -> … }` the message is

  ```
  E0004 missing cases: `[]`, `[Nothing, ..rest]`, `[Just _]`, `[Just _, Nothing, ..rest]`, `[Just _, Just _]`, `[Just _, Just _, Nothing, ..rest]`, `[Just _, Just _, Just _, _, ..rest]`
  ```

  (each witness names the constructor the complete-signature branch chose, so the cases are
  disjoint and paste in as arms without overlap), and the fix inserts one `<witness> -> ?todo,` arm per case, exactly as `report` does today
  (same placement rules, same E0302 path for a page's `update`).
- E0310 keeps both texts: `unreachable arm: every case is already handled` when the rows above
  the arm are already exhaustive, else `` unreachable arm: `<pattern>` is already handled `` with
  the arm's own pattern printed by the formatter's `pattern` printer (so the flat case still
  prints `` `All` ``).
- `internal/diag/explain.go` E0004 and E0310 gain one sentence each saying the listed case may be
  a nested pattern and `_` stands for any value there.

A match whose subject is a record and whose arms are all refutable now reports E0004, which it
never did. That is a bug fix; the corpus gets a case for it.

## Lowering and Go

`internal/ir/lower_match.go` needs no structural change: `pattern` already recurses into a
constructor's argument, a list's elements and a record's fields, builds a single-`Case` `Match`
for each refutable nested tag with the remaining rows as its shared fallback, and binds parts to
temporaries before the arm's `Let`s. The header comment's "patterns nest at most one level below a
constructor" sentence is replaced by the rule above. `internal/gen/match.go` emits only the
existing `Match` shapes (`switch` on tag, `if s.Ok`, `len(s)`, literal `switch`) and the shared
fallbacks become the closures `declareShared` already makes, so Go generation is unchanged.

The issue's example therefore lowers to: `len(s) > 0` → bind head/tail → `if h.Ok` → `len(t) > 0`
→ … → the body, with every failure jumping to the shared fallback (the next arm, or
`panic("unreachable")` when the checker proved there is none). The builder confirms with a
`gen_vet` golden (the generated Go must pass `go vet`) and a runtime case in `TestPatterns`.

## Where things change

| layer | files |
|---|---|
| grammar doc | `book/A-grammar.md` pattern rules |
| parser | `internal/syntax/parser_expr.go` (`parsePattern`, list/record loops); `testdata/golden/parse_exprs.alder` + `.golden`: `[Just m, Just s]`, `{ a: Just x, b: [_, ..r] }`, `Just [a, ..rest]`, `Just (Just x)`, `[[a], b]` |
| checker | `internal/check/pattern.go`: delete the nesting error; replace `exhaustiveness` with the usefulness matrix; `expr.go` `checkMatch` and `html.go` `HtmlMatch` call it after the arm loop; new `internal/check/pattern_test.go` unit tests on hand-built matrices; `testdata/golden/check_exprs` + `.diag` for nested E0004/E0310 and the record-subject case; a corpus case `testdata/corpus/E0004-nested-list/` |
| diagnostics | `internal/diag/explain.go` E0004, E0310 |
| IR | `internal/ir/lower_match.go` header comment; `ir_test.go` dump of the issue's match |
| gen | no code change; `testdata/golden/gen_vet/n1-nested-patterns/`; `internal/gen/gen_test.go` `TestPatterns` cases: `sum3 [Just 1, Just 2, Just 3]`, `[Just 1, Nothing, Just 3]`, `[]`, `[Just 1]`, and a `Just (Just x)` / `Just [a, ..r]` pair |
| formatter | `internal/format/print_expr.go` `subPattern`; `testdata/golden/fmt_match.alder` + `.golden` |
| tree-sitter | `tree-sitter-alder/grammar.js` `_pattern_argument`; regenerate; `test/corpus/match.txt` |
| book | `book/03-syntax.md`: the "Patterns do not nest more than one level deep" paragraph becomes the nesting rule with the issue's example and the parenthesis rule; `book/site/build.py` regenerates the site |

Also update: the roadmap issue #52 tick for #2; the user reinstalls (`go install ./cmd/alder`)
after merge and restarts the LSP.

## Left to the builder

- Task split: (1) parser + formatter + tree-sitter + grammar doc, (2) checker usefulness with its
  unit tests and goldens, (3) IR comment, gen goldens and runtime tests, (4) book and explain
  text. (1) and (2) are independent; (3) needs both.
- Representation of the normalised pattern (a small struct with tag, args and a back-pointer to
  the source pattern for spans) and of witnesses (the same struct, printed by one function that
  applies the top-level naming rule).
- Whether the per-column signature lookup caches `u.Apply` results. Match the surrounding code.

## Verification

- `go test ./...` green; the generated Go of the new `gen_vet` golden passes `go vet`.
- Every existing `.diag` golden and `testdata/corpus/*/expected.diag` is byte-identical, which
  proves goal 3.
- A scratch check (not a change to nightvote-replay, which pins its own Alder build): the
  `filterMap` + length-check workaround rewritten as the issue's three-element match gives the
  same results on `"1 2 3"`, `"1 x 3"`, `""` and `"1 2 3 4"`.
