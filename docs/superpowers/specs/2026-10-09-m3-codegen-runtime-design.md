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
example (M4).

## Architecture

```
alder check  → (no errors) → ir.Lower → gen.Emit → build/<app>/*.go → go build → binary
alder run    → the above in a loop: watch app/ → rebuild → restart child → push reload
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
  transaction per `Task` run; `NotFound` → `ErrNotFound`; `Fail` → `ErrFail`;
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

- `Rerender`: run `load` and `view` again; if the request carries
  Datastar's `Datastar-Request: true` header, respond with one SSE event
  patching the whole `<body>` in morph mode; otherwise respond with the full
  page (200).
- `Redirect`: HTTP 303 in both cases (Datastar follows it).
- `Reject text`: same as `Rerender` with the form-level error attached.
- form decode failure: same as `Rerender` with field errors attached;
  `update` is not called.

The book's `id` guidance becomes "stable ids keep the morph minimal"; it
is no longer required for correctness.

### Dev loop (`alder run`)

`alder run [dir]`: check → lower → emit → `go build` → start the child on
`:8080` (`ALDER_PORT`); then poll `app/**/*.alder` mtimes every 300 ms. On
change: re-run check; on errors, push them to the browser; on success,
rebuild, restart the child (graceful: new child starts on the port after the
old one exits), then push a reload. Live reload: the dev server proxies the
child and adds `/_dev/reload` (SSE, held open); pages in dev mode include
`<div data-on-load="@get('/_dev/reload')"></div>`; a reload is a
`datastar-execute-script` event `location.reload()`; errors are a
`datastar-patch-elements` event replacing `#_alder_errors` (a fixed overlay
the dev server injects into every page) with the rendered diagnostics.
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

`run_loop.py --build`: a round passes only if `alder check --json` has no
errors **and** `alder build` succeeds; a Go build failure is recorded as a
new round field `build: {ok, stderr}` and is a compiler bug by definition
(the checker accepted the program), filed as such in the experiment's
findings. The deferred harness items (multi-retry report, `claude -p`
isolation, `prompt_eval_count`) are done in M3d before E6.

## Error handling

Compiler: `lower` and `gen` have no user-facing errors; a `go build`
failure is an internal error with the Go output. Runtime: as the
vertical-slice spec, plus dev-mode error pages show the message and the
effect log. Dev loop: a crashed child is reported in the terminal and the
overlay; the server keeps serving the last good binary until the next
successful rebuild.

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
  reload event on `/_dev/reload`.
- E6: the harness with `--build` on T2–T4; results in chapter 12.

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
  overrides it.
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
