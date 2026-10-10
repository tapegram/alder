# Alder M3: codegen and runtime

Date: 2026-10-09. Status: approved in conversation; awaiting written review.
Supersedes the "IR and emitter", "Runtime", and "Milestones: M3" sections of
`2026-10-08-compiler-vertical-slice-design.md` where they differ.

## Purpose

Make `alder run examples/todos` serve the to-do app in a browser: links,
forms and buttons work; Datastar patches the page in place; without
JavaScript the same app works as full page loads; `alder run` rebuilds on
change and reloads the browser. From M3 on, the harness can count "compiles
and runs" instead of only "type-checks".

Decisions made in conversation and binding here:

- Approach 1 from the design discussion: a thin typed IR and a Go emitter,
  with effects compiled to a bind tree that the runtime interprets.
- SQLite via `modernc.org/sqlite` (pure Go) — the only dependency of the
  runtime; the compiler stays stdlib-only.
- Datastar vendored into `rt` and served by the app; version pinned by the
  compiler.
- Dev loop option C: rebuild on change and browser live-reload.

## Scope

In: everything `examples/todos` needs to run (see the vertical-slice spec's
"Scope"), the dev loop, the harness `--build` round, and the compiler items
E5 justified: parse-error reasons for attributes and inline brands, a poison
type for unresolved names (no cascades from a failed declaration), E0201
dedupe per module, an E0007 fix that writes the inferred type.

Out: migrations (the dev database is recreated from the schema), `alder
test`, `fmt`, holes beyond W0002, `query`, non-SQLite stores, production
configuration (TLS, secrets management beyond `ALDER_SECRET`), the blog
example (M5; see Milestones).

## Architecture

```
alder check  → (no errors) → ir.Lower → gen.Emit → build/<app>/*.go → go build → binary
alder run    → the above in a loop: watch → rebuild → start new child, swap proxy → push reload
binary       → rt: router, session, forms, effect interpreter, SQLite, Datastar
```

Packages added: `internal/ir`, `internal/gen`, `rt/` (sub-packages `html`,
`eff`, `db`, `web`, `datastar`), `cmd/alder` gains `build` and `run`.

### IR (`internal/ir`)

`ir.Lower(prog *names.Program, c *check.Result) *ir.Program` runs only when
the checker reported no errors. It carries types on every node (the
checker's `types.Type`, brands erased to their base) and nothing
Go-specific.

Program-level nodes: `Table{Module, Name, Fields []Field, IDField,
InsertFields}`, `Form{Module, Name, Fields}`, `Page{Module, Path, Params,
Requires, Msgs []Msg, Model, Load, View, Update}`, `Msg{Name, Params,
Form}`, `AppMsgs`, `AppUpdate`, `Capability{Module, Name, Entries}`,
`Session{Fields}`, `Value{Module, Name, Type, Params, Body}` for every other
top-level function.

Expressions: `Lit`, `Var{Kind: Local|Global|Param, Ref}`, `Call{Fn, Args}`
(always saturated or explicitly partial: `Partial{Fn, Given}`), `Lambda`,
`Record{Type, Fields}`, `Update{Base, Fields}`, `Field{Expr, Name}`,
`Ctor{Union, Name, Payload}`, `Match{Subject, Cases []Case{Tag, Binds,
Body}, Default}` (a decision tree with exhaustiveness already proven),
`If`, `Let{Name, Value, Body}`, `Op{Kind, L, R}`, `Str{Parts}`
(concatenation), `Hole` (never reaches gen: the checker rejects programs
with holes at `build`).

HTML: `Elem{Tag, Attrs []Attr{Name, Kind: Text|Bool|Href|Click|Submit|Maybe,
Value}, Children}`, `Comp{Fn, Props, Children}`, `Text`, `HtmlExpr{Expr,
Kind: Text|Html|List}`, `For{Var, List, Body}`, `HtmlIf`, `HtmlMatch`,
`Input{Kind: Input|Textarea|Select|Errors, Field, Label, Value}`, `Form{Msg,
Children}`.

Effects (shared by `Query` and `Task`, the IR carries which):
`Pure{Value}`, `Bind{Effect, Param, Body}`, `Op{Kind, Args}` with kinds
`DbGet, DbSelect{From, Where, Order, Limit, Offset, Terminal: All|First|Count},
DbInsert, DbUpdate{Table, ID, Fn}, DbDelete, SessionPut{Fn}, Cap{Name,
Entry, Args}, NotFound, Fail{Message}, Now`. `Nav` values are ordinary
`Ctor`s of the `Nav` union.

### Emitter (`internal/gen`)

`gen.Emit(p *ir.Program, opts) (files map[string][]byte, error)` writes one Go
file per Alder module, `routes.go`, `main.go`, and `go.mod`; `error` is only
for I/O. Output is formatted with `go/format`. Mapping:

| Alder | Go |
|---|---|
| record type | `struct` (value semantics) |
| union | `struct{ Tag tag__<M>__<Union>; <Ctor> *<payload> … }` with `<payload>` the named record or the generated `payload__<M>__<Union>__<Ctor>`: one pointer field per constructor with a payload (nil unless that constructor built the value), tag constants `is__<M>__<Union>__<Ctor>`, constructors `<M>__<Union>__<Ctor>__` (a function that allocates the payload, or a value) |
| brand | erased to its base type |
| `Maybe a`, `Result e a`, `List a`, `Dict k v` | `rt.Maybe[T]`, `rt.Result[E, T]`, `[]T`, `rt.Dict[K, V]` |
| function | `func` with all parameters; `Partial` and `Lambda` → closures |
| `match` | `switch x.Tag` with bindings from payload fields |
| `Text` interpolation | `rt.Concat(...)` |
| operators | Go operators; `++` on lists → `rt.Append`; `??` → `rt.OrElse`; comparisons on payload-less unions → tag comparison |
| HTML | `html.El("tag", attrs..., children...)`, `html.Text(s)`, component → function call; `click`/`submit` → `html.OnPost(url)` which renders `data-on:click="@post('url')"` and, for `submit`, `method="post" action="url"` as the no-JS fallback |
| `<Input field>` etc. | `html.Input(form, "field", label, value)` reading the request's form state |
| `Query`/`Task` | `eff.Pure`, `eff.Bind`, `eff.Get[T]`, `eff.Select[T]`, … (generic constructors) |
| page | an entry in `routes.go`: `rt.Page{Path, Decode, Requires, Load, View, Msgs}` with generated param/form decoders |
| message URL | `/_m/<Module>/<Msg>`; message params and the page's params as query string; the form as the POST body |
| capability | `type <Cap> interface{ <Entry>(ctx, args) (T, error) }`; `main.go` takes one per capability |

Generics: `rt` uses Go generics for containers and effect constructors; no
monomorphization.

The generated module: `build/<app>/go.mod` with `module app`, `require
alder v0.0.0`, `replace alder => "<absolute repo path>"` (quoted); `go build` runs
with `-mod=mod` so the `replace` resolves offline. `modernc.org/sqlite` is
required by `rt/db` and therefore by the generated module; `go.sum` is
written by the first build.

### Runtime (`rt`)

- `rt/html`: builder (`El`, `Text`, `Raw`, `Attr`), attribute rendering,
  escaping of text and attribute values; `Raw` is reachable only from
  `Html.unsafe`.
- `rt/eff`: `Query[T]` and `Task[T]` as `Pure | Bind | Op`; `Run(ctx,
  store, q)` interprets: reads for `Query`, reads+writes for `Task`; one
  transaction per `Task` run, begun lazily (see "Decisions recorded");
  `NotFound` → `ErrNotFound`; `Fail` → `ErrFail`;
  every op is logged in dev mode.
- `rt/db`: `Store` interface (`Get`, `Select`, `Count`, `Insert`, `Update`,
  `Delete`, `Begin/Commit/Rollback`) and its SQLite implementation; schema
  creation from the generated table descriptors; a schema hash in a
  `_alder_meta` table — dev mode drops and recreates when it changes;
  `@unique` violation → `ErrFail`; ids generated as 20-char URL-safe random
  strings; `Time` stored as RFC 3339 text; `Date` as `YYYY-MM-DD`;
  payload-less unions as their constructor name.
- `rt/web`: router from the page table (exact paths with `{param}`
  segments, GET for pages, POST for messages); session as a signed cookie
  (HMAC-SHA256, key from `ALDER_SECRET`, a fixed dev key with a startup
  warning otherwise; values JSON); form decoding (field types, defaults,
  `Maybe`, `Bool` checkbox, refinements run here and at route params) with
  re-render-with-errors when decoding fails; `requires` redirect to
  `App.login`; lifecycle exactly as book chapter 6; `App.notFound`/`App.error`
  rendering; per-request panic recovery → 500.
- `rt/datastar`: the vendored `datastar.js` (served at
  `/_alder/datastar.js`), the SSE writer, and `PatchBody(html)` /
  `ExecuteScript(js)` event helpers.

### Datastar patching

Every page is rendered as a full document. The layout component is
ordinary Alder; the generated `<head>` gets the Datastar script tag and, in
dev mode, the live-reload subscriber. After a message:

- `Rerender`: if the request carries Datastar's `Datastar-Request: true`
  header, run `load` and `view` again and respond with one SSE event
  patching the whole `<body>` in morph mode; otherwise respond with a 303
  to the page (Post/Redirect/Get; see "Decisions recorded").
- `Redirect`: without Datastar, HTTP 303. Under Datastar a 303 would be
  followed inside the client's `fetch` and the new page morphed into the
  old URL, so the answer is a 200 SSE event that navigates: a
  `datastar-patch-elements` event appending to `body` a self-removing
  `<script data-effect="el.remove()">window.location.assign("<url>")</script>`
  (see "Decisions recorded").
- `Reject text`: same as `Rerender` with the form-level error attached.
- form decode failure: same as `Rerender` with field errors attached;
  `update` is not called.

The book's `id` guidance becomes "stable ids keep the morph minimal"; it
is no longer required for correctness.

### Dev loop (`alder run`)

As built in M3d (`cmd/alder/run.go`; book chapter 11 "`alder run`" and
chapter 13 "The dev server"). `alder run [dir]` is a dev server on
`ALDER_ADDR` (default `localhost:8080`: the app behind it is in dev mode) that reverse-proxies (`httputil.ReverseProxy`,
`FlushInterval: -1`, `Host` kept) every request except `/_dev/reload` to a
child process: the executable `buildApp` produces (the `alder build`
pipeline: check → lower → emit → copy adapters/static → `go vet` → `go
build`), started with `ALDER_ADDR=127.0.0.1:0 ALDER_DEV=1 ALDER_RELOAD=<gen>
ALDER_DB=<abs build/<app>/dev.db> ALDER_STATIC=<abs dir/static>` and the
rest of the environment; its `listening on <addr>` line gives the port.
`<gen>`, the build generation, is `<nonce>-<n>`: 8 random hex digits per
dev server, and the count of children started.

- Watcher: polls every 300 ms the (mtime, size) of `app/**/*.alder`,
  `adapters/*.go` and `static/**`; a burst is coalesced and reported once
  a poll finds nothing new. A source change rebuilds; a `static/`-only
  change only reloads.
- Worker: one goroutine is the only caller of `rebuild`, so rebuilds are
  single-flight; changes queued during a rebuild are merged into one.
- `rebuild`: `alder check` (errors → overlay + stderr, old child kept) →
  `buildApp` (errors → overlay, old child kept) → start the new child
  beside the old one (10 s for `listening on`, else the last 40 lines of its stderr → overlay,
  old child kept) → swap the proxy target → SIGTERM the old child → clear
  the overlay, then reload. The old child answers until the new one is up.
- A child that exits unasked is reported in the overlay (its exit status,
  "save a source file to rebuild it", its last 40 stderr lines); the proxy answers
  502 with a page holding the overlay and subscriber (and the dev server
  serves the Datastar script itself) until the next good build.
- `/_dev/reload?b=<gen>`: SSE, `: ready`, then the reload at once when
  `<gen>` is not the generation of the last child that came up (a page that
  missed a broadcast, or one of an earlier dev server), else the current
  overlay if any; then every broadcast, `: ping` every 15 s. Events are `datastar-patch-elements`
  (`datastar.PatchEvent`): the overlay (`#_alder_errors`, mode `inner`, the
  message HTML-escaped in `<pre class="_alder-errors">`), the clear (same
  target, empty elements), and the reload (`body`, mode `append`, `<script
  data-effect="el.remove()">location.reload()</script>`; Datastar 1.0 has
  no `datastar-execute-script`).
- Stop (SIGINT/SIGTERM): end the watcher, worker, monitors and streams,
  shut the server down (5 s), let a rebuild in progress finish, stop the
  child (SIGTERM, SIGKILL after 5 s), exit 0. Exit 2 on a usage error, a
  missing `app/`, or an address that cannot be bound.

`alder build` is the same pipeline without the loop.

### Compiler items (from E5)

- Parse-error reasons: every `expect` failure names what was being parsed
  ("in a form field: attributes such as `@unique` are only allowed on table
  fields"; "in a type: `brand` is only allowed in a `type` declaration").
- Poison type: `typeOf` returns `types.Error` for an unresolved reference;
  `Check`/`Unify` with `Error` always succeed and never report, so one
  missing name produces one diagnostic.
- E0201 dedupe: one diagnostic per (module, missing import), with the
  first use's span.
- E0007 fix: when the body's type can be inferred without the signature
  (it always can for M3's purposes: infer the body in a fresh scope), the
  fix writes it; `?type` only when inference fails.

### Harness

`run_loop.py --build`: after a round whose `alder check --json` is clean,
the harness runs `alder build --emit` (lower, emit, `go vet` the generated
app package; adapters are not copied and nothing is linked), and the round
passes only if that exits 0. The result is the round field
`build: {ok, exit, stderr}` (the full output also in
`round<N>/build.stderr.txt`). Exit 1 (holes, or anything else the build
refuses with diagnostics) is an ordinary failed round: the retry prompt
carries the build output. Exit 2 (an internal error, or generated Go that
does not vet) after a clean check is a compiler bug: the summary gets
`compiler_bug: true`, the task stops with no retry, and the bug is filed in
the experiment's findings. A build timeout is also exit 2 for now (a
separate `harness_error` outcome is deferred to M4). The deferred harness
items (multi-retry report, `claude -p` isolation, `prompt_eval_count`) are
done in M3d before E6; `claude -p` isolation was tightened after E6 (see
`experiments/README.md`, "Models").

## Error handling

Compiler: `lower` and `gen` have no user-facing errors; a `go build`
failure is an internal error with the Go output. Runtime: as the
vertical-slice spec, plus dev-mode error pages show the message and the
effect log. Dev loop: a failed check, build or child start is reported in
the terminal and the overlay, and the last good child keeps serving; a
child that crashes is reported the same way and the proxy answers 502
until the next successful rebuild.

## Testing

- `internal/ir`: lower `examples/todos` and golden-dump the IR; unit tests
  for match-tree construction and effect lowering.
- `internal/gen`: golden `.go` per module for a small app; each golden
  compiled with `go vet` in the test; `examples/todos` builds.
- `rt/html`: escaping tables; `rt/eff`: bind order, rollback on `Fail`,
  `NotFound` short-circuit; `rt/db`: schema creation, CRUD round-trips,
  unique violation, recreate-on-schema-change; `rt/web`: cookie sign/verify,
  form decoding matrix (each field type × valid/invalid/missing), route
  matching, `requires` redirect, 404/500.
- End-to-end (`rt/web` with `httptest` on the generated to-do app): login
  via a stub `Password` adapter, create/toggle/delete, invalid form
  re-render, Datastar-header POST returns an SSE body patch, no-JS POST
  returns a full page or 303.
- Dev loop: a test that edits a file, waits for the rebuild, and sees the
  reload event on `/_dev/reload`; plus a check error and its fix, coalesced
  saves, a build failure, a child that does not start, a child that is
  killed and the recovery, a stale-generation subscriber, and stop.
- E6: the harness with `--build` on T2–T5; results in chapter 12.

## Milestones

**M3a — IR and emitter for the pure language.** `ir.Lower`, `gen` for
types, values, expressions, HTML (no pages/effects yet), golden `.go` that
vets. Includes the four compiler items.

**M3b — runtime core.** `rt/html`, `rt/eff`, `rt/db` with their tests;
effect lowering and emission.

**M3c — pages, routing, session, forms, Datastar.** `rt/web`,
`rt/datastar`, `routes.go`/`main.go` emission; `alder build`; the to-do app
serves; the end-to-end test.

**M3d — dev loop and E6.** `alder run` with live reload; harness
`--build` and the three deferred harness items; E6 recorded.

**M4 — `alder test`, the REPL, `alder eval`, book verification.**
`alder test`; the REPL (`alder repl`: compile-and-run through the real
pipeline with a persistent scratch module); `alder eval` for agents; and
automatic verification of every book example. Shared machinery: the
`rt.Show` pretty-printer and the scratch-module runner. This replaces the
vertical-slice spec's M4 (the blog example and migrations), which moves to
M5.

**M5 — blog example and migrations** (formerly the vertical-slice spec's
M4): the blog example app end to end; schema migrations replacing
dev-mode recreate.

**M6 — editor support** (added 2026-10-09): a tree-sitter grammar for
Alder (generated from, and tested against, the same corpus
`internal/syntax` parses; chapter 3 is its prose form) and a Neovim
plugin: file type, highlighting and indentation from the grammar,
`alder check --json` as diagnostics, `alder eval` of the expression under
the cursor; a language server comes later, once the checker exposes
positions for hover and go-to-definition. To be brainstormed and specced
after M5.

Each milestone gets its own implementation plan.

## Decisions recorded

- Effect values are bind trees with Go closures as continuations; they are
  interpretable and loggable but not comparable with `==`. Book chapter 10
  changes in M4 to compare effect *traces*; until then `alder test` stays
  deferred.
- Whole-`<body>` morph on `Rerender`; ids are advisory.
- `go build` runs on the generated module with a `replace` to the local
  repo; publishing `rt` as a module is out of scope.
- The dev watcher polls (no fsnotify dependency in the compiler).
- The dev key for cookies is fixed and printed as a warning; `ALDER_SECRET`
  overrides it. (Superseded in the M3c final fix wave: the dev key is used
  only in dev mode; production requires `ALDER_SECRET`, below.)
- `Dict k v` takes keys of any type (M3a fix wave, C2): `rt.Dict[K, V any]`
  keys its map by the canonical encoding `rt.Key(k)` (a `Time` as its UTC
  instant, lists and records structurally, a nil list as the empty list),
  keeps insertion order, and generated code has no `comparable` constraint.
- `Int` is 64-bit two's complement and wraps on overflow; `/` truncates
  toward zero and `%` has the dividend's sign; `Int` division by zero is a
  runtime failure (a Go panic in M3a; M3b makes it a task failure). `Float`
  is IEEE. So that Go never folds constants at compile time, the emitter
  routes a literal through the identity `rt.Int`/`rt.Float` when both
  operands of an arithmetic operator are constant expressions, and always
  for the literal divisor of `/` or `%` (M3a fix wave, C8).
- Every union constructor payload is held by pointer (M3a fix wave, C1):
  `type M__Tree[A any] struct{ Tag …; Node *payload__M__Tree__Node[A] }`. Without
  the pointer a recursive union (`Node { left: Tree a, … }`) is an invalid
  recursive Go type. The rule is uniform (every payload, recursive or not)
  so the emitter needs no recursion analysis; the constructor function
  allocates, a match reads through the pointer, and `rt.Eq`/`rt.Key`
  follow pointers. Prelude's `Maybe` and `Result` keep their value
  representation.
- Generated Go names are injective (M3a fix wave, C3). Alder identifiers
  may not contain `__` and module name segments may not end with `_`
  (E0001). A top-level declaration is `<segments joined by __>__<name>`
  (`Todos__Index__view`); a constructor is `<M>__<Union>__<Ctor>__`; the
  other generated names start with a lowercase kind word that no module
  segment can be: `payload__…`, `tag__…`, `is__…`, `widen__…`; a reserved
  local gets `__` appended (`type__`); files are `<segments joined by
  __>.go`. The proof is in `internal/gen/names.go`, checked by
  `TestGoNameInjective`; `gen.Emit` also rejects a duplicate file or
  top-level name. Page modules and App reserve `Params`, `Msg`, `Session`
  and `<Msg>Params` for the generated types (E0012). The final review's
  suggested `M__Shape__Rect` payload and `Tag__M__Shape` tag forms were not
  used: module `M.Shape` declaring `Rect`, or module `Tag.M` declaring
  `Shape`, produces the same strings.
- A `requires f` page's session is a distinct nominal type in the checker,
  `<Page>.Session`, with `f : T` instead of `Maybe T` (M3a fix wave, C4). It
  widens implicitly to `App.Session` where that is expected, at the top of
  a checked expression or a piped value (as a brand widens to its base),
  and never narrows back (E0003). `check.Result.Widened` records each widened expression;
  `ir.Lower` wraps it in `ir.Widen`, and gen emits a per-page
  `widen__<Page>__Session(s) App__Session` that re-wraps `f` in `rt.Just`.
  A function type is not widened (`App.Session -> X` does not stand for
  `<Page>.Session -> X`), so no closure conversion is ever needed.
- Effects are interpreted eagerly, node by node (M3b). `rt/eff` represents
  an effect as `Pure | Bind | Call | Fail | NotFound` (the "Op" above is
  `Call`: a name for the trace, a write flag, and a Go function of the
  `Env`); `eff.Run` is a loop over a heap continuation stack, runs one step
  at a time in source order, never batches or parallelises, and starts no
  goroutines. A `Task` runs in at most one transaction, rolled back on
  `Fail`, `NotFound`, any error or a panic; a `Query` runs with no
  transaction and a write `Call` reached from one is `ErrInternal`. Every
  `Call` name is appended to `Env.Trace` and logged at level `effect`;
  tests will compare traces (book chapter 10 now says so; `alder test`
  stays deferred). Book chapter 12 records the reasoning.
- Storage mapping (M3b, `rt/db`): SQL table = module-qualified name with
  `.` → `__`; Text and brands TEXT, Int INTEGER, Float REAL, Bool INTEGER
  0/1, Date TEXT `YYYY-MM-DD`, Time TEXT RFC 3339 UTC with a fixed nine-digit
  fraction (text order is time order), payload-less unions
  TEXT constructor name (ordered and compared by declaration), `Maybe` →
  NULL; ids are 20-char URL-safe random strings; `STRICT` tables with
  `CHECK`s for Bool and unions. `Db.and []` is true, `Db.or []` and
  `.in []` are false. `Db.update` that changes the id fails the task
  (`UpdateRow`). Without dev mode a schema-hash mismatch refuses to start
  and names the changed tables.
- Transactions are lazy (M3b final fix wave, I2). `Run` begins none up
  front: `Env.Transaction` (used by `db.TxOf`/`db.TxFrom`) begins the
  `Task`'s transaction on its first store access and records it in
  `Env.Tx`; `Run` commits or rolls back only a transaction that was begun.
  So a `Task` that calls a capability or `Http` before touching the
  database (or never touches it) holds no SQLite write lock (file
  databases use `BEGIN IMMEDIATE`), and a session-only `Task` begins
  nothing. A `Begin` error fails the `Task` at that first store step.
- A failed `Task` restores the session (I1). `Run` snapshots `Env.Session`
  at start and restores it on every failure path (`Fail`, `NotFound`, an
  error, a panic, a failed commit); a successful `Task`, including one that
  produces `Reject`, keeps its `Session.put` changes and its writes.
- The context carries the `Env` (I3). While `Run` runs, `Env.Ctx` carries
  the `*eff.Env` (`eff.EnvFrom(ctx)`), and generated code passes that `Ctx`
  to every store operation and capability method; `rt/db` adds
  `TxFrom(ctx)` (lazy, as `TxOf`) and `StoreFrom(ctx)`, so a Go capability
  writes in the running `Task`'s transaction. Capability method signatures
  stay `(ctx, args) (T, error)`.
- Constraint failures are `Task` failures (I4): `@unique` →
  `"<Table>.<col> must be unique"`; a dangling `@ref` on insert or update
  (`SQLITE_CONSTRAINT_FOREIGNKEY`) → `"<Table>.<col>: no <RefTable> with
  that id"` (the column found by looking each `@ref` value up); deleting a
  referenced row → `"<Table> <id> is referenced by another row"`; all
  `*eff.ErrFail`.
- Schema identity (I5): `Ensure` hashes the sorted `CREATE TABLE`
  statements (declaration order is no change) and records a separate
  storage format version, `db.Format` (`"1"`), under `format` in
  `_alder_meta`; rt/db bumps it whenever `createStmt` changes. A different
  format recreates in dev and, otherwise, refuses with "database format v0
  is older than this runtime (v1)".
- Conditions on `Maybe` columns are two-valued (C1): `Eq`/`Ne` render as
  `IS`/`IS NOT`, `In` with `Nothing` in the list adds `OR col IS NULL`,
  and every leaf SQL could make NULL (`<`, `<=`, `>`, `>=`, `.contains`,
  `IN`) is `COALESCE((leaf), 0)` (an `IS`/`IS NULL` form never is NULL),
  so `Db.not` matches exactly the rows its operand does not. Ordering by a
  `Maybe` column puts `Nothing` first ascending.
- Generated files are `<Module segments joined by __>_alder.go` (M3b,
  superseding "files are `<…>.go`" above and the plan's `.alder.go`). Go
  truncates a file name at its first `.` before looking for a `_GOOS` or
  `_GOARCH` suffix, so module `Bar_linux` as `Bar_linux.alder.go` would
  build only on Linux; `Bar_linux_alder.go` (and `Foo_test_alder.go`,
  which does not end in `_test.go`) are ordinary files. The app-wide lists, `Tables` (for `db.Ensure`) and
  `Capabilities` (the `Env.Caps` keys that must be wired), are in
  `app__alder.go`: no module can produce that name, and `app_alder.go`
  would clash with `App_alder.go` on a case-insensitive file system.
- A record field whose type leads back to its own record through `Maybe`,
  `Result` or an anonymous record is held by pointer in Go
  (`Next *rt.Maybe[M__Node]`); lists need no box (M3b). Every record
  construction site, including hand-written Go in capability adapters,
  must allocate the box.
- `Nav` lives in `rt` (`rt/nav.go`: `rt.Rerender()`, `rt.Redirect(page)`,
  `rt.Reject(text)`), not in generated code, so `rt/web` (M3c) can act on
  an `update`'s result without importing the app.
- Capabilities are implemented by an adapters package (M3c, option B).
  `<app>/adapters/` is one Go package, `package adapters`, importing the
  generated package as `"app"` and declaring `func Caps() app.Caps`;
  `alder build` copies its `.go` files into `build/<app>/adapters/` and
  writes `build/<app>/cmd/main.go`, which calls `app.Serve(adapters.Caps(),
  …)`. The generated surface is exported: `Cap__<M>__<C>` (one method per
  entry, `(ctx, args Args__<M>__<C>__<Entry>)` for one argument, one Go
  parameter per curried argument, `ctx` only for an entry that is itself a
  `Query`/`Task`), `Args__…`, `Route__…`, and the fixed `Caps`, `Serve`,
  `Options`, `Routes`, `Tables`, `Capabilities`. E0015 refuses a module
  whose first segment is exactly `Route`, `Cap` or `Args`. `Serve` refuses
  to start when a `Caps` field is nil (before opening the database); a build
  of an app with capabilities and no `adapters/` exits 1 naming each
  interface. An app inside another Go module carries its own `go.mod`
  (`examples/todos/go.mod`), so the outer module's `./...` skips
  `adapters/`, whose `import "app"` resolves only in `build/<app>/`.
- Page parameters travel in message URLs (M3c fix round). The renderer
  appends the rendering page's parameters to every message URL as
  `_p.<name>` keys (`rt.MsgRef.URL`, deferred `html.MsgAttr` values
  resolved with `FormState.PageParams`); a parameter at its default is left
  out, as in links. A message route decodes the page's parameters only from
  the `_p.` keys (`rt.PageParams`): no `Referer` dependence, and message
  and page parameters never shadow each other (an Alder name contains no
  `.`). A missing required page parameter is a 404. `List` parameters
  repeat the key.
- `Rerender` (and `Reject`) from `App.update` is a 303 back to the
  `Referer`'s path and query (`rt.BackURL`, `/` without one); App has no
  page to render. A `Reject` text and the errors of a form on an App
  message are not shown (dropped on the redirect). `Redirect` is a 303 to
  its page. Under Datastar each of these 303s is the navigation event
  below.
- The server's address is `ALDER_ADDR` (default `:8080`; it supersedes
  the `ALDER_PORT` of the first "Dev loop" draft); with `ALDER_DB` (default `<app>.db`), `ALDER_SECRET`,
  `ALDER_DEV=1` and `ALDER_STATIC` (default: `static/` beside the
  executable) these are the environment of the generated `cmd/main.go`,
  which prints `listening on <addr>` once bound. It serves with an
  `http.Server` whose `ReadHeaderTimeout` is 10 s and, on SIGINT or
  SIGTERM, shuts down gracefully (5 s for requests in flight, exit 0).
- Session cookie format: `alder_session` = base64url(P) "." base64url(
  HMAC-SHA256(key, P)), unpadded, where the signed payload P is
  `{"exp":<unix seconds>,"session":<session JSON>}` (M3c final fix wave,
  I6); signed, not encrypted (a `Text.Secret` must never be stored in the
  session). The session JSON is an object keyed by Go field name; a
  payload-less union is stored by constructor name; decode starts from the
  defaults and a missing or undecodable field keeps its default; a cookie
  that does not verify, whose `exp` has passed, or that has no `exp` is
  the default session. `exp` is the time of the store plus 30 days,
  renewed on every store, so the server revokes a copied cookie after 30
  days whatever the browser does. Key: `ALDER_SECRET` of at least 16
  bytes. Outside dev mode a missing or shorter secret is refused:
  `web.CheckSecret`, called first by the generated `Serve`, returns
  "ALDER_SECRET must be set to at least 16 bytes outside dev mode
  (ALDER_DEV=1)", `cmd/main.go` prints it and exits 1, and `web.NewRouter`
  panics on it (M3c final fix wave, C2). Only in dev mode is the public dev
  key used, with a warning logged once at startup. Attributes: `HttpOnly`, `SameSite=Lax`,
  `Path=/`, `Max-Age` 30 days, `Secure` under TLS or
  `X-Forwarded-Proto: https`. Stored on every rendered `GET` and every
  answer after a successful `update`; on a 404/500 only when a successful
  `Task` left a session before the re-load failed; never after a form
  decode error or a failed `Task`.
- Datastar error patches: under `Datastar-Request: true`, a 404 or 500
  (including an unknown route and a recovered panic) is answered like a
  `Rerender`, status 200 with one `datastar-patch-elements` event of the
  failure page's `<body>`, because the Datastar client ignores non-2xx
  answers. The router injects the Datastar script tag after the first
  `<head>` of every document it writes (else after `<html>`, else before
  the last `</body>`, else first), so layouts stay plain Alder.
- Datastar redirect = navigation event (M3c final fix wave, C1). Under
  `Datastar-Request: true` every 303 the router would write (`Redirect`,
  `requires` → `App.login`, App messages) is instead a 200
  `datastar-patch-elements` event, `selector body`, `mode append`,
  `elements <script data-effect="el.remove()">window.location.assign(<url
  as a JSON string>)</script>` (`datastar.Navigate`; what the official SDKs
  do; `data-effect` and the `el` binding are in the vendored v1.0.0
  bundle, and appended scripts are executed). The session is stored on
  that response as on the 303. The generated handlers are unchanged: the
  router decides.
- Navigation targets are sanitized (re-review residual). `rt.SafeURL`
  accepts a site path (`/…`, not `//…` or `/\…`), a fragment (`#…`) and an
  absolute `http:`/`https:` URL with a host (scheme in any case), and
  refuses any URL containing a byte < 0x20 or 0x7f (browsers strip them:
  `/\t/evil` is `//evil`); `Url.external` (`std.UrlExternal`) maps anything
  it refuses to `#` (pure code: nothing is logged). Defense in depth: `datastar.Navigate`
  refuses an unsafe URL with an error and writes nothing; `rt/web`'s
  redirect path answers one with a 500 (`eff.ErrInternal`: "refusing to
  navigate to <scheme>:"; under Datastar the usual 200 patch of the error
  page); `rt/html` renders an unsafe `href` as `#`.
- SSE line ends (C3). `PatchBody`/`Navigate` turn `\r\n` and a lone `\r`
  into `\n` before splitting the elements into `data:` lines, and the HTML
  renderer writes `\r` in text and attribute values as `&#13;`, so no user
  text can end a data line and inject an SSE field.
- Post/Redirect/Get for a no-JS `Rerender` (I2). Without Datastar, a page
  message's `Rerender` (and a `Reject` of a message without a form)
  answers 303 to the page URL rebuilt from the decoded page parameters
  (`rt.PageAt(pattern, params__M(params))`); form errors and a form's
  `Reject` stay 200 re-renders, since they must show state.
- Click buttons without JavaScript (I3). The renderer (not the emitter, so
  components and computed HTML are covered too) renders a `<button>` with
  a `data-on:click` message attribute outside any `<form>` as
  `<form method="post" action="<url>" class="_alder-click"
  data-on:submit="@post('<url>', {contentType: 'form'})"><button
  type="submit" …>…</button></form>`, and inside a `<form>` as `<button
  type="submit" formaction="<url>" formmethod="post" formnovalidate
  data-on:click__prevent="@post('<url>')" …>`. Deviation from the review
  ruling: inside a form the Datastar action has no `{contentType: 'form'}`
  and the button has `formnovalidate`, because Datastar's form mode runs
  the enclosing form's `checkValidity()` (and the browser's native
  submission validates too), which would block a click button whenever a
  `required` field of the surrounding form is empty; the enclosing form's
  fields are therefore posted only by the no-JS path, and a message without
  a form ignores them. Other elements with `click` keep
  `data-on:click="@post('<url>')"` (JavaScript only).
- Form state belongs to one form (I1). `html.FormState` carries `Msg`, the
  posting message's key (`rt.MsgKey(path, query)`: path plus its own
  parameters, sorted, `_p.` keys dropped); a control shows the state only
  inside a `<form>` whose message key (`rt.MsgRef.Key`, or the literal
  action's) equals it, and only for its own form type; a state without
  `Form` or `Msg` matches nothing. Errors of a field without an `<Errors
  field>` in that form are listed by `<Errors form />` after the field's
  name (M2); the checker warns W0003 for a field with neither.
- Route precedence (I4). `web.NewRouter` sorts routes by specificity —
  segment by segment from the left, a literal before a parameter; equally
  specific patterns keep their declaration order — so `/x/new` is matched
  before `/x/{id}` whatever the module order. The checker rejects two
  pages answering the same pattern (parameter names and a trailing slash
  ignored) with E0016 "pages `A` and `B` both answer `/x/{id}`".
- Browser validation follows the decoder (I5): `Float` inputs get
  `step="any"`, `Text.Email`/`Text.Url` render `type="email"`/`"url"`, and
  `required` is set only when the decoder refuses a blank value (not for a
  `Maybe`, a field with a default, a checkbox, plain `Text`, or an app
  brand of `Text`).
- `Task.notFound : Task a` (C4) mirrors `Query.notFound`: it lowers to
  `ir.EffNotFound` and runs as `eff.NotFound`, so the `Task` rolls back and
  the route answers 404. The to-do example authorizes in `update`
  (`owned`), and `Todos.Show`'s messages carry no id: they act on the
  `load`-authorized `model.todo`.
- A `Bool` URL parameter is strict (M3, `web.BoolParam`): `True`/`False`,
  `true`/`false`, `1`/`0` or absent; anything else is a 404. A checkbox
  keeps the lenient `web.Bool`.
- `alder build` treats an `adapters/` without top-level `.go` files as
  absent (M1); only top-level `.go` files are copied.
- Dev loop as a proxy (M3d), superseding the restart-on-the-same-port
  design: the dev server owns `ALDER_ADDR` and each child listens on
  `127.0.0.1:0` (the port read from its `listening on` line), so the new
  child starts while the old one still serves and the swap is atomic for
  the browser.
- The reload subscriber is injected by the router, not by the proxy:
  `ALDER_RELOAD=<gen>` (set only by `alder run`) → generated `main` sets
  `web.Options.Reload` (the generation; "" = off) → `withReload` inserts
  `<div id="_alder_errors" data-ignore-morph></div><div
  data-init="@get('/_dev/reload?b=<gen>', {openWhenHidden: true})"></div>`
  right after `<body…>` in full documents, 404/500 pages and Datastar body
  patches (so a morphed body keeps them). The proxy stays a pass-through:
  it would otherwise have to buffer and rewrite HTML and SSE answers.
- The subscriber attribute is `data-init`, not `data-on-load` as first
  specified: Datastar v1.0.0 has no `on-load` plugin and a `<div>` fires no
  `load` event.
- No reload is lost (M3d final review): the box carries `data-ignore-morph`
  (Datastar 1.0.0 skips an element when old and new both carry it, so an
  app `Rerender` keeps the error; the dev server's `inner` patches still
  apply); `@get` gets `openWhenHidden: true` (its default for GET is to
  close the stream while the tab is hidden); and the generation in `?b=`
  makes a reconnecting page of an older build or dev server reload at once.
- The dev database is `build/<app>/dev.db` (absolute `ALDER_DB`); dev mode
  recreates the tables on a schema change, and old and new child briefly
  share the file during a swap.
