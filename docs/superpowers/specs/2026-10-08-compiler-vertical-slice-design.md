# Alder compiler: vertical slice design

Date: 2026-10-08. Status: approved in conversation; awaiting written review.

## Purpose

Build the smallest compiler and runtime that takes `examples/todos` from
source to a running server, with a type checker good enough to catch the
errors the spike (book chapter 12, experiments E1–E4) showed models make.
The checker ships first, because the experiments found that prose stops
reducing errors (E3) and only compiler feedback can take the weakest useful
model (Qwen3-Coder 30B) from 3 errors to 0.

Success for the slice:

1. `alder check examples/todos` reports no errors, and reports the right
   error with a fix for each case in a corpus mined from E1–E4 outputs.
2. The experiment harness can run generate → check → retry, and the
   resulting error counts are recorded in the book.
3. `alder run examples/todos` serves the to-do app in a browser, with
   Datastar patching and full-page fallback.

## Scope

In the slice: everything `examples/todos` uses. Specifically:

- Core language: literals, records (nominal), unions, brands with `where`,
  functions, lambdas, `if`, `match` (one level, plus record patterns and
  list patterns), blocks with `let` and `let <-`, `|>`, operators, string
  interpolation.
- Declarations: `module`, `import`, value, `type`, `page`, `msg`, `form`,
  `table`, `capability`, `session`, `login`, `test`.
- HTML expressions, components, `for`/`if`/`match` in HTML, attribute
  typing, `<Input>`, `<Textarea>`, `<Select>`, `<Errors>`.
- Effects: `Query`, `Task`, their functions in chapter 7, `Query.notFound`,
  `Task.fail`, `Reject`.
- `Db`: `get`, `from`/`where`/`orderBy`/`limit`/`offset`/`all`/`first`/
  `count`, column conditions, `insert`/`update`/`delete`, transactions.
- Standard library: the subset the example and the four tasks use
  (`List`, `Maybe`, `Text`, `Int`, `Bool`, `Date`, `Time`, `Html`, `Url`).
  The rest of chapter 9 is added as tests demand.
- Runtime: router, cookie session, form decoding with refinements,
  `requires`, Datastar SSE patching, SQLite adapter, capability wiring.

Deferred (not in any slice milestone): migrations (the dev database is
recreated from the schema on start), `alder fmt`, typed holes, `alder
query`, `Dict`, `Http`, `Random`, `Email`, `Log`, multiple database
adapters, `Html.unsafe` auditing, the open questions in chapter 12.

## Architecture

One Go module, `alder`, producing one binary. Packages:

```
cmd/alder/           CLI: check, build, run, test
internal/syntax/     lexer, tokens, AST, parser
internal/names/      module graph, imports, resolution, globals
internal/types/      type representation, environment, unification
internal/check/      the checker: declarations, expressions, HTML, pages
internal/ir/         typed, desugared intermediate form
internal/gen/        Go emitter
internal/diag/       diagnostics: codes, spans, fixes, text and JSON output
rt/                  runtime library linked into generated apps
stdlib/              Alder-side standard library signatures (embedded)
testdata/            golden tests
```

Pipeline, per `alder check`:

```
files → lexer → parser → AST (per module)
      → names: build module graph, reject cycles, resolve every name
      → check: declarations first (types, tables, pages, msgs, forms),
               then bodies; collect diagnostics, never stop at the first
      → diagnostics (text or --json)
```

`alder build` continues: `→ ir → gen → build/<app>/*.go → go build`.
`alder run` builds, then runs the binary with `ALDER_DEV=1`, restarting on
file change. `alder test` builds a test binary from `test` declarations and
runs it.

The checker never emits IR for a program with errors. The emitter never
sees an ill-typed program, so it has no error paths of its own beyond
`go build` failing, which is a compiler bug.

## Components

### Lexer and parser (`internal/syntax`)

- Hand-written lexer. Tokens carry byte spans. Newlines are tokens, and
  the parser decides where they matter (block statements, declaration
  boundaries).
- Recursive-descent parser producing one `*ast.Module` per file. Errors
  are recovered at declaration boundaries so one bad declaration doesn't
  hide the rest of the file.
- HTML is lexed in a sub-mode entered at `<` in expression position
  (where `<` cannot be the comparison operator: start of an expression,
  after `=`, `->`, `(`, `{`, `,`, `then`, `else`, `in`). Inside an element,
  text runs until `<` or `{`; `{` re-enters expression mode.
- String interpolation: the lexer splits `"a {e} b"` into parts, and the
  parser builds a concatenation.
- The grammar in book appendix A is the reference. Where the appendix and
  the parser disagree, the appendix is fixed in the same change.

### Names (`internal/names`)

- Discovers modules under `app/` from file paths; the `module` line must
  match the path (diagnostic otherwise).
- Builds the import graph; an import cycle is a diagnostic naming the
  cycle.
- Global namespace: every `page` module name and every `msg` declared in
  `App` is visible everywhere without import. Standard library modules are
  always visible.
- Resolution rules from book chapter 3: qualified types and values; a
  declaration named like its module is referred to by the module name;
  constructors are unqualified and must be unique across a module's
  imports plus the standard library; `Params`, `Msg`, `Session`, `Model`
  are per-module generated or declared names.

### Types (`internal/types`)

Representation:

```
Int | Float | Bool | Text | Unit | Date | Time
Brand{Name, Base, Check}            -- nominal over a built-in base
Record{Name, Fields}                -- nominal; anonymous records have Name=""
Union{Name, Params, Ctors}
App{Con, Args}                      -- List a, Maybe a, Result e a, Html a,
                                       Query a, Task a, Form f, Table t, ...
Fun{Params, Result}
Var{id}                             -- inference variable
Never
```

Rules:

- Bidirectional: `check(expr, expected)` and `infer(expr)`. Every top-level
  value has a signature, so bodies are always checked against a known type
  and inference variables appear only inside expressions (lambda
  parameters, `let` without annotation, polymorphic stdlib calls).
- Unification is standard first-order; brands unify only with themselves
  except in `check` mode, where a `Brand` is accepted where its `Base` is
  expected (widening). `==` and `!=` require the two sides to unify
  without widening.
- Numeric operators on two values of the same numeric brand produce that
  brand. All other operations on brands produce the base.
- Anonymous record literals take the expected nominal type in `check`
  mode; in `infer` mode they are anonymous and can only be used where an
  anonymous record type is expected (inline signatures).
- Payload-less unions are ordered by declaration; the comparison operators
  accept them.
- `Html Never` and any `Html a` unify; app-level message values have type
  `Msg'` which unifies with every page's `Msg` inside `Html`.
- No typeclasses, no let-polymorphism beyond top-level signatures.

### Checker (`internal/check`)

Two passes over each module: declarations, then bodies. The declaration
pass is what makes pages, messages, forms, and tables language constructs:

- `page`: path segments and parameter record agree; parameter types are
  URL-representable; `requires` names a `Maybe` session field; the module
  has `Model`, `load`, `view`, and `update` if it has any `msg`; `Params`
  and `Msg` are generated. Inside the module, `Session` has the required
  field narrowed.
- `msg`: parameter types are URL-representable; `form F` names a `form`;
  `Msg` constructors carry the parameter record plus `form` when present.
  App-level messages live in `App` and `App.update : Msg -> Session ->
  Task Nav` must match them.
- `form`: field types are form-representable; defaults are well-typed.
- `table`: exactly one `@id` whose type is a `brand Text`; `@ref T` has
  `T`'s id type (or `Maybe` of it); field types are storable; the table
  record type and `Insert T`, `Id T`, column values (`T.field.eq`, …) are
  generated.
- `capability`: each entry's result is `Query` or `Task`.
- `session`: field types are form-representable; `login` names a page.

The body pass checks expressions against signatures and additionally:

- `load` is checked against `Params -> Session -> Query Model`; any `Task`
  inside is an error ("writes are not allowed in load").
- `update` arms are exhaustive over `Msg`; each arm's result is `Task Nav`.
- HTML: tag names from the HTML5 set; attribute expression types per
  chapter 5 (`href` wants a page value, `click`/`submit` want a message of
  this page or an app-level message, booleans, `Maybe Text`); children are
  `Text`, `Html`, or `List Html`; `<Input field="x">` and friends are inside
  a `<form submit={M}>` where `M` has a form containing `x`; a plain
  `<input>` inside such a form is an error.
- Effect blocks: `let x <- e` only in a block whose result type is `Query`
  or `Task`; a `Task` inside a `Query` block is an error.
- `match` exhaustiveness over unions, `Maybe`, `Result`, `Bool`, and list
  patterns; a `_` arm only last.

Every diagnostic the spike motivated, with its code:

| Code | Trigger | Fix offered |
|---|---|---|
| E0101 | `Db.get T x` where `x` is not `Id T` | `Db.from T |> Db.where (T.<col>.eq x) |> Db.first` when a column of `x`'s type exists |
| E0102 | field access on a `Maybe` | `match` skeleton |
| E0201 | name from a module that isn't imported | `import M` |
| E0202 | unqualified type from another module | `M.T` |
| E0203 | qualified constructor | unqualified name |
| E0301 | message sent without its parameters | `M { field: ?field }` |
| E0302 | `update` missing an arm | arm skeleton |
| E0303 | `href` with a `Text` | — |
| E0304 | form field not in the form type | list of fields |
| E0401 | `Task` inside `load` or a `Query` block | — |
| E0402 | `let x <-` outside an effect block | — |
| E0501 | `@id`, `@created`, `@updated` supplied on insert | remove the field |
| E0502 | `@ref` with the wrong id type | — |
| E0601 | `Html Msg` in a non-page module | `Html a` |
| E0602 | `case ... of`, `let ... in`, tuple, alias | the Alder form |

Codes are stable once published; `alder explain <code>` prints the table
row and an example.

### IR and emitter (`internal/ir`, `internal/gen`)

The IR is the checked AST with: names fully resolved, types on every node,
HTML desugared to a builder tree, interpolation to concatenation, `let <-`
to explicit bind nodes, match compiled to a decision tree with
exhaustiveness already proven, brands erased.

The emitter writes one Go file per Alder module plus `main.go`:

- Records become structs (value semantics match Alder's immutability).
- Unions become a struct with a tag and one field per payload; `match`
  becomes a `switch` on the tag.
- Functions are curried at the Alder level and emitted uncurried where
  fully applied, closures otherwise.
- Polymorphic stdlib calls are emitted against `rt` generics
  (`rt.List[T]`, `rt.Maybe[T]`), which Go 1.18+ supports; no monomorphizing
  in the slice.
- `Query` and `Task` values are data: trees of `rt.Query`/`rt.Task` nodes
  that the runtime interprets. `load`, `update`, and `App.update` compile
  to functions returning those trees.
- Pages and messages compile to entries in a route table: method, path
  pattern, parameter decoders, the page's `load`/`view`/`update`.
- Capabilities compile to Go interfaces; `main.go` constructs the app with
  one argument per capability and fails to compile if one is missing.
- HTML compiles to `rt.Html` builder calls with escaping at the builder
  level, so no generated code can emit unescaped text except via
  `Html.unsafe`.

The generated Go is formatted with `go/format` and is meant to be read.

### Runtime (`rt`)

- `rt.Serve(app)`: `net/http` server, router over the route table.
- Session: a signed cookie (HMAC, key from `ALDER_SECRET` or a dev
  default) holding the encoded `session` record; `Session.put` writes
  back on response.
- Request lifecycle as in book chapter 6: decode params → `requires` →
  `load` → `view` for GET; decode params and form → `requires` → `load` →
  `update` → run → `Rerender`/`Redirect`/`Reject` for POST.
- Form decoding runs the field type's checks (brands' `where`, `Maybe`,
  defaults, `Bool` checkboxes); failure re-renders the page with the form
  state attached, without calling `update`.
- Query interpreter: runs `Db` reads against SQLite via `database/sql`;
  `Query.notFound` short-circuits to 404 with `App.notFound`.
- Task interpreter: one transaction per `update`; `Task.fail` or a
  capability error rolls back and renders `App.error` with status 500.
- Datastar: when the request carries the Datastar header, `Rerender`
  responds with an SSE patch containing the elements whose `id` exists in
  both old and new render (morph); otherwise a full page. `Redirect` is a
  303 either way. Messages render as `data-on:click="@post(url)"` and
  forms as `data-on:submit` with a plain `method="post" action` fallback.
- SQLite adapter: creates tables from the schema at start in dev mode
  (drops and recreates if the schema changed; no migrations in the slice).

### CLI and diagnostics (`cmd/alder`, `internal/diag`)

```
alder check [dir] [--json]
alder build [dir]
alder run   [dir]
alder test  [dir]
alder explain <code>
```

Text diagnostics follow the Rust/Elm style: file:line:col, message, the
source line with a caret, the fix. `--json` emits one object per line:
`{code, severity, file, line, col, endLine, endCol, message, fix?}` with
`fix = {line, col, replace}`. Exit code 1 on any error.

## Data flow in a generated app

```
GET /?filter=active
  router → Todos.Index entry → decode Params{filter: Active}
  → session from cookie → requires user: Just → narrowed Session
  → load(params, session) : rt.Query → interpreter runs Db reads
  → view(model, session) : rt.Html → render → 200 text/html

POST /_m/Todos.Index/Toggle?id=42&filter=active        (generated URL)
  router → Toggle entry → decode {id: TodoId "42"} and page params
  → requires → load → update(Toggle{id}, model, session) : rt.Task
  → interpreter: begin tx, Db.update, commit
  → Rerender: load → view → diff by id → SSE patch (or full page)
```

Message URLs are generated: `/_m/<Module>/<Msg>` with the message's own
parameters and the page's parameters as query string. They are never
written by hand and never appear in the book as something to choose.

## Error handling

Compiler: every phase collects diagnostics into one list and continues;
the parser recovers at declaration boundaries; the checker uses an error
type that unifies with anything to avoid cascades. `go build` failure on
generated code is reported as an internal error with the Go output, since
it means a compiler bug.

Runtime: 404 for route or parameter decode failure and `Query.notFound`;
form decode failure re-renders with errors; task failure → rollback →
`App.error`, 500; panics in the runtime are recovered per request and
treated as task failure. Nothing in application code can perform an
effect, so there is no application-level error path besides `Task.fail`.

## Testing

- **Golden tests** under `testdata/`: each case is an `.alder` file (or a
  directory for multi-module cases) with an expected `.diag` file (empty
  for success) and, for codegen cases, an expected `.go`. `go test
  -update` rewrites expectations.
- **Error corpus**: `testdata/corpus/` holds minimal reproductions of
  every distinct error scored in E1–E4, each with its expected code. The
  checker must flag all of them. This is the acceptance test for M1.
- **Example apps** must check and build in CI: `examples/todos` from M1,
  the blog from M4 (now M5; see the M3 spec).
- **Runtime tests** use `net/http/httptest` against the generated to-do
  app: GET renders, POST toggles, invalid form re-renders with errors,
  `requires` redirects, Datastar request gets an SSE patch.
- **Harness**: `experiments/run_loop.py` runs generate → `alder check
  --json` → feed diagnostics back → one retry, and records errors before
  and after. Milestone M2 is this measurement.
- Unit tests for the lexer and for unification, written first (TDD), since
  they're where subtle bugs live.

## Milestones

**M1 — `alder check`.** Lexer, parser, names, types, checker, diagnostics.
Acceptance: `examples/todos` checks clean; every corpus case produces its
expected code; `--json` output validates against the schema above.

**M2 — Checker in the loop.** `run_loop.py`; T2–T4 on Qwen3-Coder and
Haiku with one retry. Acceptance: results recorded in chapter 12 as E5.

**M3 — Codegen and runtime.** IR, emitter, `rt`, SQLite, Datastar.
Acceptance: `alder run examples/todos` serves the app; runtime tests pass.

**M4 — Second app and migrations** (now M5; see the M3 spec). The blog from task T3 as
`examples/blog`, built end to end; then schema diffing and migration
files, moving migrations out of the deferred list.

Each milestone gets its own implementation plan.

## Decisions recorded after M1

- **Fix contract.** A diagnostic's `fix` is `{line, col, endLine, endCol,
  replace}`: replace the half-open span `[start, end)` with `replace`. An
  insertion has `start == end`. Text output prints `fix: <replace>` as
  before. Every producer states its end explicitly (M1 fixes had no end).
- **Phase gating is per module.** Parse errors still stop the pipeline
  (there is nothing to resolve). After the names pass, modules whose
  resolution produced errors are excluded from the body-checking pass;
  all other modules are checked. The declaration pass still runs for every
  module, because other modules depend on their types.
- **Positional message arguments** (`click={Toggle todo.id}`) are E0301
  with the record-form fix, the same shape as a bare constructor.
- **The harness drives Claude through the local `claude -p` CLI** (headless
  prompt mode under the user's own login) and local models through Ollama.
  Scoring is automatic: error counts and codes come from `alder check
  --json`.

## Decisions recorded

- `@unique` violations fail the task (book chapter 8). Revisit with data.
- Derived `toText` for unions, brace-less `let` in function bodies, and
  user-chosen `@id` stay out of the slice; the checker loop will show
  whether they matter.
- Message URLs are compiler-generated (`/_m/...`); the format is an
  implementation detail and may change.
- Polymorphism in generated Go uses Go generics in `rt`; no
  monomorphization until performance says otherwise.
- The compiler is Go, the target is Go, per the design conversation; no
  second target in the slice, but the IR has no Go-specific nodes.

## Project layout after M3

```
alder/
  book/  examples/  experiments/  docs/
  cmd/alder/  internal/{syntax,names,types,check,ir,gen,diag}/  rt/  stdlib/
  testdata/{golden,corpus}/
  go.mod
```
