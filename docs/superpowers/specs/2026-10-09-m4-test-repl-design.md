# Alder M4: `alder test`, the REPL, `alder eval`, and a verified book

Date: 2026-10-09. Status: approved in conversation; awaiting written review.
Builds on `2026-10-09-m3-codegen-runtime-design.md` (M4 as recorded there).

## Purpose

Give programs a way to be checked beyond "it compiles": `alder test` runs
the `test` declarations chapter 10 describes, including tests of effects
against a fresh database; `alder repl` and `alder eval` evaluate
expressions through the real pipeline for humans and agents; and the book
is verified automatically so it cannot drift from the compiler again.
Everything runs the *compiled* program — there is no second interpreter.

Decisions made in conversation and binding here:

- One mechanism underneath all of it: a **scratch-module runner** that
  compiles the app plus one extra module and runs a generated `main`.
- Effect tests are ordinary effect blocks that use a stdlib function
  `Test.run`, which runs the inner task under a **savepoint** and reports
  its result and trace; no new syntax.
- A test body is inferred as `Bool` or `Task Bool`; no signature line.
- Values print as Alder syntax through `rt.Show`.
- The book's fenced Alder blocks carry their expectations in the fence
  info string and are checked by `go test`.

## Scope

In: `internal/scratch`, `rt.Show`, the `Test` stdlib module with
`run`/`trace`, savepoints in `rt/db`, the checker rules for tests,
`alder test` (text and `--json`, `-run`), `alder repl`, `alder eval`, the
book verifier and the book fixes it forces, chapter 10 rewritten, chapters
11/13 updated, and the M3d deferrals: the `alder run` child exits on stdin
EOF, a `harness_error` outcome, an output-directory parameter for
`buildApp`/`buildTo`, `--base` from a clean export.

Out: editor integration, multi-line REPL editing beyond a trailing `\`,
coverage, parallel test execution, property-based testing, `alder fmt`,
migrations (M5), E7 (its own plan after M4).

## Architecture

```
alder test  → scratch.Build(app, Tests module?, test main) → binary → runs every test in one process → report
alder eval  → scratch.Build(app, Scratch{it = expr}, print main) → binary → prints rt.Show(it) : Type
alder repl  → the eval loop with a persistent Scratch module, re-checked per line, rebuilt per evaluation
book_test   → extracts fenced blocks → alder check / alder eval per block → expectations from the fence
```

Packages added: `internal/scratch`, `rt/testing`, `stdlib/Test.alder`;
`cmd/alder` gains `test`, `repl`, `eval`.

### The scratch-module runner (`internal/scratch`)

```go
type Module struct{ Name, Source string; Infer bool } // an in-memory .alder module
type Build struct{ Dir string; Binary string; Diags []diag.Diagnostic; Err error; … }
func Check(appDir string, extra *Module) Checked
func Compile(appDir string, extra *Module, mainGo, purpose, out string) Build // Check + CompileChecked
func CompileChecked(c Checked, mainGo, purpose, out string) Build
func RunContext(ctx context.Context, b Build, env []string, stdin string) (stdout, stderr string, exit int)
```

(As built in M4a.) `Infer` sets `check.Options{InferModule: Name}`: that
module's sig-less values are inferred (functions may call themselves) and
it may use any brand constructor, as a `test` may. An app that already
has a module of that name is a `ModuleExistsError`. `CompileChecked`
takes a flock on `<out>.lock` and a runnable `Build` holds it until
`Close`. `RunContext` runs the binary in its own process group and kills
the group when the context is done (a timeout, or the caller's Ctrl-C).

`Compile` loads the app from `appDir` (as `alder check` does), adds `extra`
to the module set (if non-nil) with an implicit import of every app module
so names resolve as they do from a page module, checks, lowers, emits into
`out` (default `build/<app>/scratch/<purpose>/`), writes `cmd/main.go`
from `mainGo`, copies adapters and static, vets and builds. Diagnostics
are returned (never printed) with positions in the extra module reported
against the module name the caller gave, so the REPL can map them to the
line typed. `buildApp`/`buildTo` in `cmd/alder/build.go` gain the `out`
parameter and `Compile` reuses them. The scratch directory is kept between
runs so Go's build cache keeps the app package compiled; an evaluation
costs ~0.3–0.6 s on a warm cache.

### `rt.Show`

`func Show(v any) string` prints a Go value as Alder syntax, deterministic:
ints `3`, floats `1.5` (always with a point), strings Alder-escaped,
`True`/`False`, `()`, lists `[1, 2]`, records `{ title: "Milk", done:
False }` (field names from the Go struct, declaration order), unions
`Nothing` / `Just 3` / `Rect { w: 1.0, h: 2.0 }` (constructor names from
the generated tag constants via a registration the emitter writes into
`app__alder.go`: `rt.RegisterUnion(tagType, names)`), dates
`2026-10-09`, times `2026-10-09T12:00:00Z`, dicts `Dict.fromList [{ key:
"a", value: 1 }]`, HTML as its rendered markup, effects as `<Query>` /
`<Task>`, pointers (boxed Maybe) transparently. Used by `alder test`'s
failure output, `alder eval`, and the REPL.

### The `Test` stdlib module

```alder
module Test

run   : Task a -> Task { value: Result Text a, trace: List Text }
trace : Task (List Text)
```

`Test.run e`: snapshots `Env.Session`, opens `SAVEPOINT alder_t<n>`,
starts a trace window, runs `e`'s tree with the same `Env` (no nested
`Run`; the interpreter gains a reentrant `runInner`), then on success
`RELEASE`s and returns `{ value: Ok v, trace: window }`; on `*ErrFail`,
`ErrNotFound` or a capability error, `ROLLBACK TO` the savepoint, restores
the session, and returns `{ value: Err msg, trace: window }` where `msg`
is the failure text ("not found" for `ErrNotFound`). Any other error (a
store error) fails the enclosing test. `Test.trace` returns the enclosing
test's trace so far. `rt/db.Store` gains `Savepoint(ctx, tx, name)`,
`Release`, `RollbackTo` (SQLite nests savepoints; the `:memory:` store
uses the single connection).

Checker rules: `Test.*` is usable only inside a `test` body (E0003
"`Test.run` is only available inside a `test`"); a test body is checked
against `Bool` first and, if that fails, against `Task Bool`; any other
type is E0003 "a test must be `Bool` or `Task Bool`". Inside a `test`,
brand constructors are usable from any module (existing rule).

### `alder test`

- **Discovery:** every `TestDecl` in every module of the app. The IR gets
  `Module.Tests []*Test{Name, Body Expr, Effect bool}`; the emitter writes
  `func test__<Module>__<n>() bool` or `… eff.Task[bool]` plus a table
  `var Tests = []rtesting.Case{{Module, Name, Pure: fn} | {Module, Name,
  Task: fn}}` in `app__alder.go`.
- **Isolation:** one process; per test a fresh `db.OpenSQLite(":memory:")`
  + `Ensure(Tables, true)`, a fresh `eff.Env` (session = `App`'s defaults
  via the generated codec's `Default()`, `Now` fixed to
  `2026-01-01T00:00:00Z`, `Caps` = the app's adapters when the scratch
  build found `adapters/`, else a stub per capability that fails with
  "capability X is not wired in tests"), the whole test inside one
  transaction that is always rolled back at the end.
- **Running:** `rt/testing.Run(cases, opts) Report` runs each case: a pure
  case is called directly; a task case is `eff.Run` under the fresh env. A
  panic in a test is recovered and reported as a failure with the message.
- **Reporting:** `ok   Todo "matches Active excludes done"` / `FAIL
  Todos.Index "toggle flips done"`; when the body (or the task's final
  expression) is `a == b`, the emitter also captures both sides and the
  report prints `left:  <Show a>` / `right: <Show b>`; a failed task prints
  the error and the trace; a summary line `N passed, M failed`; exit 1 on
  any failure, 2 on a build problem. `--json` emits one object per test
  `{module, name, ok, left, right, error, trace, ms}` for the harness.
  `-run <regex>` filters by `Module "name"`.
- **CLI:** `alder test [dir] [-run re] [--json]` in `cmd/alder/test.go`,
  built through `scratch.Compile` with a ten-line generated main that
  calls `rtesting.Main(Tests, flags)`.

### `alder repl`

- `alder repl [dir]`; prompt `alder> `; the session is an in-memory
  module `Scratch` whose declarations accumulate: `let x = e` becomes a
  top-level `x = e` (its signature inferred with the E0007 machinery and
  shown as `x : T`); a bare expression becomes `it = e` and is evaluated;
  `:type e` checks only and prints `e : T`; `:run e` evaluates a `Query`
  against the dev database (`build/<app>/dev.db`, the same one `alder
  run` uses); `:run! e` evaluates a `Task` (writes commit); `:html e`
  renders an `Html a` value; `:reload` re-reads the app; `:reset` clears
  the session; `:quit`. Lines ending in `\` continue.
- Each line re-checks the whole session module (milliseconds); errors are
  the normal diagnostics with the line mapped to the input ("line 3 of
  your input"). A `let` that stops type-checking after `:reload` is
  dropped with a notice.
- Evaluation compiles the session through `scratch.Compile` into
  `build/<app>/scratch/repl/` and runs the binary, which prints
  `rt.Show(it)`; the REPL prints `value : Type`. Effects print `<Query>`
  unless `:run`.
- Auto-reload: the REPL polls `app/` like `alder run` (300 ms) and
  re-checks the session on change.

### `alder eval`

`alder eval [dir] 'expr'` or `-` for stdin (several lines allowed; `let`
lines then a final expression; a line at column 1 starts an item unless
it starts with a closer, `}`, `)`, `]` or `</`). Prints `value : Type` or
the diagnostics; `--run` allows a `Query` against the dev database;
`--json` prints `{"value": "...", "type": "...", "diagnostics": [...]}`,
plus `"error"` and `"hint"` when present; `--module <Name>` names the
scratch module for an app that has a `Scratch` module of its own. Exit
0/1/2 as `check`. This is the form agents and the book verifier use.

### Book verification

`book/book_test.go` (package `book`, run by `go test ./...`): extracts
every fenced block whose info string starts with `alder` from every
chapter and appendix. The info string carries the expectation:

- ```` ```alder ```` — the block must check clean: a block starting with
  `module` is checked as a whole module (in a temp app with the to-do
  example's modules available for imports); a block of declarations is
  wrapped in `module Snippet` with the example modules imported; a
  **signatures** block (every line `Name.x : T` or a list of names, at
  least one qualified) is a reference table, each typed name checked to
  have that type (`sig : T` / `sig = Name.x`). A block of expressions (or
  `let`s + expression) is only **parsed** (`alder dump`), not
  type-checked: prose expressions often use names the example does not
  define; give one `eval=` to have it evaluated.
- ```` ```alder eval=<value> ```` — must evaluate (`alder eval`) to
  exactly that `Show` text.
- ```` ```alder error=E0304 "message part" ```` — must produce that
  diagnostic code in the block itself (not in an example module it
  breaks), its message containing the optional quoted text; an expression
  is evaluated, anything else checked.
- ```` ```alder part=<Module> ```` — one piece of a module shown piece by
  piece: a chapter's `part=` blocks of one module, consecutive among its
  `part=` blocks, are joined in order (the first starts with the `module`
  line) and checked as that module, replacing the example's.
- ```` ```alder fragment ```` — a deliberate excerpt: not checked,
  but counted and listed in the test output so fragments stay rare.

The test reports every drift with chapter:line. The first run will find
real drift; fixing it is part of the milestone. The verifier is the
reason the book can keep being the single source of truth.

### Deferrals folded in

- The `alder run` child exits when its stdin reaches EOF (the dev server
  holds the pipe), so a SIGKILLed `alder run` leaves no orphan.
- Harness: build timeouts and unknown exit codes become `harness_error`,
  not `compiler_bug`; `--base` copies from `git archive HEAD` of the base
  directory, never the live tree.
- `buildApp`/`buildTo` output-directory parameter.

## Error handling

Scratch builds never print; they return diagnostics or a Go error (a vet
failure in generated code is an internal error, exit 2). `alder test`:
a failing test is exit 1; a build problem exit 2; a panic inside a test
is that test's failure, not the runner's. The REPL never exits on an
error. `Test.run` turns the inner task's failure into `Err`; a store
error is a test failure with the error text.

## Testing

- `internal/scratch`: builds the todos app plus a scratch module that
  references a page, a table and a capability; diagnostics mapped to the
  extra module; cache reuse (second build faster).
- `rt`: `Show` table over every value shape, incl. nested unions,
  boxed Maybe, dicts, HTML, escapes.
- `rt/db`: savepoint nesting, rollback-to leaves outer writes.
- `rt/testing` + `alder test`: a fixture app with pure passing/failing
  tests, an effect test with `Test.run` success and failure (rollback
  verified by a subsequent `Db.count`), a test that panics, a test whose
  capability is unwired, the `==` left/right capture, `--json`, `-run`.
- checker corpus: `Test.run` outside a test; a test of type `Int`.
- REPL/eval: `alder eval` cases (value, type-only error, `--run` Query,
  `--json`); REPL driven through a pipe: `let`, expression, `:type`,
  `:reset`, a diagnostic line mapping, `\` continuation.
- Book: `go test ./book` green after the drift is fixed; one deliberately
  wrong `eval=` in a test fixture proves it fails.

## Milestones

**M4a — foundations:** `buildTo` out-dir; `internal/scratch`; `rt.Show`
(+ union registration); `alder eval`; the book verifier and the book
fixes it forces.
**M4b — tests:** `Test` stdlib + savepoints + `runInner`; checker rules;
IR/emit of tests; `rt/testing`; `alder test`; chapter 10 rewritten; tests
added to `examples/todos`.
**M4c — REPL and deferrals:** `alder repl`; stdin-EOF child exit;
`harness_error`; `--base` export; chapters 11/13.

Each milestone gets its own plan.

## Decisions recorded

- No interpreter: every evaluation is compiled code; the REPL's latency
  is the price and it is acceptable (~0.5 s).
- `Test.run` is a savepoint, so "a failed task undoes its writes" holds
  inside tests exactly as it does in the server.
- `Test.*` only inside `test` bodies; test bodies are the one place a
  failure message becomes a value.
- Book expectations live in fence info strings so readers see them.
- `Now` is fixed in tests (`2026-01-01T00:00:00Z`); `Random.id` is real.
- Departure (M4b): each effect test gets a fresh `:memory:` database, and
  its transaction is committed when the task succeeds (not always rolled
  back, as "Isolation" above says); the database is then closed and
  gone, so nothing can leak between tests either way, and the test runs
  exactly as the server runs a Task.
- Departure (M4b): a test's body is a `Task Bool` exactly when it is a
  block `{ … }`, and a `Bool` otherwise (not "`Bool` first, then
  `Task Bool`"); the hints point at the other form ("a test of a Task is
  written as a block", "end it with `Task.succeed (…)`"), and a body that
  is neither names the test (`test "x": expected Bool, got Int`).
- A capability's error is a failure a test can observe: generated
  capability calls wrap an adapter's error as `*eff.ErrCapability`
  (`"Password.verify: <msg>"`), which `Test.run` turns into `Err`; the
  router still answers it with a 500.
- `alder test` streams each result as the test finishes, bounds the run
  with `-timeout` (default 5m) passed to the program as a deadline, names
  the test that crashed the program (exit 1, never the Go dump), and the
  program exits when its stdin (a pipe `alder test` holds) closes, so a
  killed `alder test` leaves nothing running.
