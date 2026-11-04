# Routes: every URL is a route, state lives in the hypermedia

Design for [tapegram/alder#160](https://github.com/tapegram/alder/issues/160), read with #49, #165,
#161 and #152. Brainstormed with the user on 2026-11-02 to 04. Everything marked **(call)** is
the author's decision, not the user's; everything else was decided in the conversation.

## Intent

Today an Alder module is a page: one `page` line, one `Model`, one `load`, one `view`, some `msg`
lines and one `update` that answers with `Rerender`, `Redirect` or `Reject`. Every message pays for
the page's `load` twice (before and after `update`), the only way to keep a computed result is to
write it somewhere or put it in a redirect's params, a row's buttons can't be shared between two
pages without two `update`s, and nothing lets a piece of a page be fetched on its own (search
results, "load more", a slow widget).

The user's position, which this design builds out:

- **Push HATEOAS as far as it goes.** The server remembers nothing between requests. State lives in
  the URL, in the HTML, in the session or in the database, and nowhere else. Which home a value is
  in decides what survives (refresh, back, share), so "does a refresh keep it" is a choice the
  developer makes by where they put it, not a runtime feature.
- **Cheap partial updates must be the easy path.** Toggling one row must not load the list.
- **Routes, not pages.** Every URL is a route with an HTTP method. A GET route has a `load` and a
  `view`; a write route has an `update` that changes data and answers with an HTTP response. A page
  is a GET route whose view happens to include the layout; the language has no word for it.
- **Composition is routes rendering routes.** A write route answers by naming the GET route whose
  representation it wants shown. Views are plain functions, so a page's view renders a row with the
  row route's view, and the row that a `PATCH` patches in is byte-for-byte the row the page renders.
- **The request says how to deliver, not the route.** As HTMX and Datastar do it: a plain request
  gets a document, a boosted link gets a body morph, a fetch from an element gets the view patched
  by its root `id`. No route declares whether it is a page or a partial.
- **The answer is an HTTP response, named as HTTP names it.** Rails/Turbo, HTMX, Remix and
  LiveView all have the same three outcomes of a write (redirect, re-render in place, partial) and
  spell them as status codes and headers. Alder returns a `Response` built by functions named after
  the HTTP they produce, instead of a union of invented words.
- **Fewer concepts, flat files.** `page`, `msg`, the module-level `update`, `type Model`, `Nav`,
  `Notice` and the type parameter of `Html` all go. Declarations are qualified by the route they
  belong to (`Index.view`) so a file with several routes stays unindented and order-free.

## Goals

1. A row toggle costs one write and one read by key, and sends one element on the wire. No page
   load, no before-view.
2. One `route` keyword covers pages, partials and writes; a module may hold any number.
3. Search-as-you-type, infinite scroll, lazy regions, inline edit, modals, wizards, login, toasts,
   CSV export and polling badges each fall out of routes plus existing attributes, with no new
   runtime concept (the walk-through is below).
4. Without JavaScript every write still works: a plain POST, the response's own status code, a GET.
5. `alder test` keeps a guarantee it can honour on a page with partials: every region the server
   patched equals what a fresh render of that region gives, checked on every click and submit; the
   whole-page equality is a test's opt-in (`Test.refresh`).
6. Existing apps move mechanically: a rewrite command converts a page module to routes.
7. `Response` is where later work lands (status codes, headers, several patches, JSON, files,
   streams) without a language change.

## Non-goals

- Client-side state, Datastar signals, `data-show`, `data-bind` (#49, deferred by the user).
- Streaming answers and server push (#165): this design gives them their unit (`Response.render`
  over time, a watched GET route) and stops there.
- JSON answers, webhooks (#152): `Response.json`, `Response.status`, `Response.header` are
  accounted for below and not built here (user: hold back until needed).
- `Response.remove`/`Response.patch` and keyed child diffs: delete and insert go through
  `Response.refresh` for now.
- Forms that submit to a GET route (a `<form method="get">` with typed fields): forms post.

## Anti-goals

- Any per-route flag or type that says "page" or "partial". The user tried `Page` as a view type
  and a `fragment` keyword and rejected both: the request decides, as in HTMX.
- A model that `update` returns to `view`. Tried and rejected: an answer that carried both a model
  and params could disagree, and a refresh is what exposes it. `update` changes data; a GET route
  renders data.
- A hidden cost model (loading the model only in arms that read it). Tried and rejected in favour
  of structure: a write route has no model because it has no view.
- A raw `{ status, headers, body }` record as the answer: a `Location` that is text loses the typed
  `Route`, and the checker could no longer warn about a rejected form with no `<Errors form />`.
  `Response` is opaque; its builders are typed; the raw status and header are escape hatches.
- Server-side per-tab state, a cache of the last body, sticky sessions.

## The rule for state

| home | survives | for |
|---|---|---|
| URL (route params) | refresh, back, bookmark, share | filter, sort, page, wizard step, calculator inputs |
| HTML (form values, button payloads) | the next interaction only | drafts in flight, "the thing this button acts on" |
| session | refresh, not sharing | identity, secrets, theme |
| database | everything | durable facts |

Book wording **(call)**: "If you would want it back after a refresh, it goes in the URL. If it
is secret, it goes in the session. Everything else rides in the HTML or is a row."

## The language

### `route`

```alder
route Index  = GET    "/"                  { filter: Todo.Filter = All } requires user
route Row    = GET    "/todos/{id}"        { id: Todo.TodoId }           requires user
route Toggle = PATCH  "/todos/{id}/toggle" { id: Todo.TodoId }           requires user
route Create = POST   "/todos"             { } form NewTodo              requires user
route Delete = DELETE "/todos/{id}"        { id: Todo.TodoId }           requires user
route Export = GET    "/export.csv"        { }                           requires user
```

`route NAME = METHOD "path" { params } [form F] [requires …]`. `NAME` is a constructor:
`href={Index { filter: Done }}`, `click={Toggle { id: t.id }}`, `submit={Create}`,
`Response.redirect (Index {})`, `Test.get (Row { id })`. Methods: `GET`, `POST`, `PUT`, `PATCH`,
`DELETE`. `form F` is allowed on write routes only; the decoded form is `params.form` as today.
`requires` and the ownership rules (`@owner`, `Db.mine`, roles) apply per route, unchanged. The
`Page` type of today (`Redirect Page`) becomes `Route`.

Path rules are today's (`/x/{id}` vs `/x/{slug}` is E0016, literal segments before parameters); two
routes may share a path if their methods differ **(call)**. Params not in the path ride where they
do today: in the query string of the request URL for every method, with form fields in the body
**(call)**; so `Like = POST "/like" { postId: PostId }` posts to `/like?postId=…`, and a no-JS
form's `action` carries them.

### Names

A route's name is a namespace inside its module **(call)**: `Row.view`, `Row.load`, `Row.Params`.
From another module it is reached through the module: `Todos.Row.view`, `Todos.Row { id }`,
`Todos.Index {}`. The resolver reads a dotted name as the longest module path it knows, then a
route, then a member; so a module `Todos.Row` and a route `Row` in module `Todos` cannot both
exist (E-code), and a route may not take the name of an import alias in its module (E-code). The
migration names a route after its module's last segment and stops with that error if it collides,
asking for `--name`.

### Qualified declarations

Every declaration that belongs to a route is written `Route.name`; order in the file is free, and
anything unqualified is the module's own (shared types, helpers, components).

```alder
module Todos

import Table.Todo
import Layout

route Index  = GET   "/"                  { filter: Todo.Filter = All } requires user
route Row    = GET   "/todos/{id}"        { id: Todo.TodoId }           requires user
route Toggle = PATCH "/todos/{id}/toggle" { id: Todo.TodoId }           requires user
route Create = POST  "/todos"             { } form NewTodo              requires user

type TodoList = { filter: Todo.Filter, todos: List Todo }

Index.load : Index.Params -> Session -> Query TodoList
Index.load params session = {
  let todos <- Db.mine Todo |> Db.where (Todo.matches params.filter) |> Db.all
  Query.succeed { filter: params.filter, todos: todos }
}

Index.view : TodoList -> Session -> Html
Index.view model session =
  <Layout.Page title="Todos" theme={session.theme}>
    <nav>
      <a href={Index { filter: All }}>All</a>
      <a href={Index { filter: Done }}>Done</a>
    </nav>
    <ul id="todos">{for t in model.todos -> Row.view t session}</ul>
    <form submit={Create}><Input field="title" /><Errors form /><button>Add</button></form>
  </Layout.Page>

Row.load : Row.Params -> Session -> Query Todo
Row.load params session = Db.getMine Todo params.id

Row.view : Todo -> Session -> Html
Row.view t session =
  <li id={"todo-" ++ t.id}>
    <button click={Toggle { id: t.id }}>{t.title}</button>
  </li>

Toggle.update : Toggle.Params -> Session -> Task Response
Toggle.update params session = {
  let _ <- Db.updateMine Todo params.id (\t -> { t | done: not t.done })
  Task.succeed (Response.render (Row { id: params.id }))
}

Create.update : Create.Params -> Session -> Task Response
Create.update params session = {
  let _ <- Db.insertMine Todo { title: params.form.title, due: Nothing }
  Task.succeed (Response.redirect (Index { filter: All }) |> Response.toast "Added")
}
```

- A GET route has exactly `R.load : R.Params -> Session -> Query m` and `R.view : m -> Session -> Html`
  for one type `m`; a write route has exactly `R.update : R.Params -> Session -> Task Response`.
  The checker ties `R.view`'s argument to `R.load`'s result (E-code, new).
- **The model is whatever `load` returns (call).** There is no `type Model` slot. Name it
  (`TodoList`), use a table's row type (`Todo`), or write the record type inline.
- **`R.Params` is generated per route (call)**, as `Params` was per page; `R.Params` on a write
  route with `form F` has the field `form : F`.
- Signatures stay required, as elsewhere in Alder **(call)**; the LSP can insert them since the
  route fixes the shape.
- `Html` has no type parameter: there is no per-module `Msg`, a `click` names a route. `Layout`
  components become `… -> Html`. App-level messages (`App.Logout`) become write routes in whatever
  module declares them.
- `alder fmt` keeps the `route` lines together where the first one is and keeps the file's order
  otherwise **(call)**; a file that opens with its routes reads as its own table of contents.
- A module with one GET route is today's page file minus `page` and `Model`, which is most files.

### `Response`

`Response` is an opaque type in the standard library, built by functions that are named after the
HTTP they produce. The status code is the signal, as Turbo reads it: 3xx follow, 422 render in
place, 200 content.

```alder
update : Params -> Session -> Task Response

-- Shipped by this design
Response.redirect : Route -> Response                 -- 303, Location: the route's URL
Response.render   : Route -> Response                 -- 200, the route's view
Response.refresh  : Response                          -- 200, the page the browser is on, again
Response.invalid  : Text -> Response                  -- 422, the page, form kept, error shown on it
Response.toast    : Text -> Response -> Response      -- a toast on any response (was Notice.with)
Response.success, Response.warning, Response.error    -- the #162 variants, same shape as toast

-- Accounted for, not built (user, 2026-11-04: hold back until needed)
Response.status   : Int -> Response -> Response       -- override the code (201 Created, 204)
Response.header   : Text -> Text -> Response -> Response
Response.json     : Json -> Response                  -- #152: 200, application/json
Response.file     : File -> Response                  -- folds file pages in (today's `File` answer)
Response.remove   : Text -> Response                  -- Datastar remove by id; a delete with no reload
Response.patch    : List Html -> Response             -- several elements in one answer, by their ids
Response.stream   : …                                 -- #165: render over time
```

**The one rule a developer needs:** `render` is for something already on the page; `refresh` is
for things that appear or disappear, or when more than one place changes. `render` costs the
rendered route's `load` (one read by key for a row) and sends one element; `refresh` costs what
`Rerender` costs today (the page's `load` before and after) and sends the regions that differ.

`invalid` is a committed success, as today's `Reject`: its `Task` commits, writes and
`Session.put` are kept. The checker's W0005 (a rejectable form with no `<Errors form />`) moves to
`Response.invalid`.

**What the type must be shaped for (call):** a status, a list of headers, a list of Datastar events
(element patches with selector and mode, a toast, a location), and a body for the no-JS answer.
`redirect`, `render`, `refresh`, `invalid` are each one value of it. `status` and `header` are
plain record updates on it. `toast` appends an event. Later builders add events. The runtime
consumes the value; nothing in the compiler knows the builders beyond their types.

`Notice.with` and the #162 variants stay for one release as aliases of `Response.toast` and
friends, with a deprecation warning that names the replacement **(call)**.

### Attributes that request a route

An element can ask for any route; the answer is patched where its root `id` stands.

| attribute | fires | exists today |
|---|---|---|
| `href={R …}` on `<a>` | boosted navigation to a GET route | yes (`Page` → `Route`) |
| `click={R …}` | request `R` (GET or write) | yes, for messages; GET is new |
| `submit={R …}` on `<form>` | post the form to a write route | yes |
| `input={R …}` on a field | GET `R` as the field changes, debounced **(call: 300 ms)** | new |
| `load={R …}` | GET `R` when the element appears | new |
| `intersect={R …}` | GET `R` when the element scrolls into view | new |
| `every={N s} … ={R …}` | GET `R` on a timer | new; shape per the fragments spec's note |

Checker rules **(call)**: `href` to a write route is an error; `submit` to a GET route is an
error; `load`, `intersect`, `input` and `every` to a write route are errors (a timer must not
write). `click` to a GET route is allowed: it is "fetch and patch".

The new attributes compile to Datastar `data-on-*` expressions carrying the same request helper
`click` uses, so the runtime grows no new protocol. The builder may land them as a second slice
after `click`/`submit`/`href` work against routes.

**Without JavaScript (call).** `click` keeps today's rendering: a wrapping `<form method="post">`.
For `PUT`/`PATCH`/`DELETE` it carries a hidden `_method` field, the Rails convention, which the
router honours on POST only. `click` to a GET route renders `<form method="get" action=…>`, so
without JS the button navigates to the partial's bare document. `submit` is a form already.
`input`, `load`, `intersect` and `every` have no no-JS path; the page shows what the server
rendered.

## The runtime

### Delivery, decided by the request

| request | GET route | write route |
|---|---|---|
| plain (address bar, no JS, a `<form>` post) | document (`<html>`, shell, dev panel) | run `update`, answer the response's own HTTP |
| boosted link (Alder's boost marker) | body morph, title, history, as today | — |
| from an element (`Datastar-Request: true`, no boost marker) | the view, one `datastar-patch-elements` by its root `id`, mode outer | run `update`, deliver per the response (below) |

A view delivered "from an element" must have a single root element with an `id`. A root without
one, or several roots, is a runtime error: logged, shown in the dev panel, DOM untouched
**(call)**. (Whether Datastar v1 morphs several top-level elements onto one selector is for the
builder to verify; if it does, several roots are allowed and the first root's `id` is the target,
which is what "load more" wants: the next rows and a new sentinel replacing the old sentinel.)

A GET route's view that includes `<Layout.Page>` is a page; one that does not is a partial. The
runtime does not know or care. Visiting a partial's URL in the address bar renders it as a bare
document, as hitting an HTMX partial does. Accepted in the brainstorm; no error **(call)**.

### A write's response, with JavaScript

- `redirect r` (303): deliver `r` as a boosted navigation would (`load`, view, body morph, title,
  history entry for `r`'s URL). Never diffed, even when `r` is the page the browser is on
  **(call)**: a redirect means "go there", and a before-view would have to be rendered before
  `update` without knowing where `update` will go.
- `render r` (200): run `r.load`, render `r.view`, one patch by the root `id`, mode outer. No
  before-view, no page load. Toast events follow.
- `refresh` (200): the page the browser is on, loaded again, diffed against the before-view, URL
  unchanged.
- `invalid t` (422): the page the browser is on with the posted form's values and `t` attached to
  that form, diffed against the before-view.

**Which page the browser is on.** A write route does not know (a `Toggle` may be used from the
index and from a detail page), and `refresh`, `invalid` and the before-view need it. `ClickPost`
and `SubmitPost` already send `Alder-Shown` (the body hash); they also send the page's URL
**(call: header `Alder-Page`, path and query)**. The router resolves it to a GET route and params
with the same matcher as a GET; a URL it cannot match (another app, a stale build) answers the
response's plain HTTP (a 303 to that URL for `refresh`). The before-view is rendered from that
route **before `update` runs**, as the generated update route already does for `Rerender`.
`alder check` computes per write route whether `refresh` or `invalid` is reachable in its `update`
**(call)**; only those routes render a before-view. `render` and `redirect` never do.

### The shown hash, with partials on the page

User decision, 2026-11-04: **a stale hash no longer forces a full body morph.** `render`, `load=`,
`intersect=`, `input=` and a GET `click=` change the DOM without the server rendering the page, so
after any of them `al-shown` is stale by design. On `refresh` and `invalid` the server diffs the
before-view against the after-view and patches the regions *it* changed; regions it did not change
are left as the browser has them, so loaded "more" rows, search results and an open inline edit
survive a delete elsewhere. The guarantee is HTMX's: every region the server sent is right.

- The hash keeps one job: before equals after → no element event is sent.
- The one hazard the hash also caught, a tab from an older build, moves to a build id: every
  Datastar request sends `Alder-Build` **(call)**; a mismatch answers a full reload (Datastar
  script event or `HX-Refresh`-style), never a patch.
- A rejected form's regions and a `load` that reads the clock are no longer special: their regions
  differ between before and after and are patched.
- `al-shown` stays in the page and is refreshed by every full-body delivery; `render` and element
  fetches do not touch it.

### A write's response, without JavaScript

A plain POST runs `update` and answers the response's HTTP: `redirect r` → 303 to `r`'s URL;
`refresh` → 303 to the referring page (`Referer`, else `/`) **(call)**; `render r` → 303 to the
referring page **(call)** (the partial has nowhere to go without a page); `invalid t` → 422 with
the referring page rendered, form kept, error shown, as today's no-JS `Reject` does. `_method`
override as above.

### Routing

Message URLs (`/_m/<Module>/<Msg>`) go away: a write route has the path it declares. Two routes
with one path and different methods are distinct handlers; a request with a method no route
declares for the path is 405 with `Allow` **(call)**. The request matcher is the one GETs use.
CSRF and origin handling stay what messages have today; the builder confirms the same checks apply
to every non-GET method.

### Tests

- `Test.get (R …)`, `Test.click (R …)`, `Test.submit R { … }`, `Test.submitRaw R fields`,
  `Test.open (R …)` — the same functions, taking routes. `Test.click` of a GET route fetches it
  and patches the held document as the browser would.
- The held document is kept up to date through every delivery. The automatic check (goal 5):
  after `refresh`, `invalid` and `redirect` the server rendered the body, and the browser compares
  every region it patched against that render, as the fragments spec's check does today; after
  `render` and element fetches it compares the patched element against `R.view` of `R.load`
  rendered on its own **(call)** — equal by construction, which is what the check proves.
- `Test.refresh` **(call)**: GET the URL the held document is on and fail if the body differs from
  what is held. This is the developer's whole-page honesty check: a toggle test ends with it; a
  test of "load more" or a draft preview does not call it.
- `answer.url` **(call)**: the URL the held document is on after a step, so a wizard test can
  assert the step moved. `answer.status` carries the response's code.

## Migration

- **`alder fix --routes`** (name is the builder's) rewrites a page module in place **(call)**:
  `page "/x" { … } requires u` → `route <ModuleLastSegment> = GET "/x" { … } requires u`;
  `type Model`, `load`, `view` → qualified, `Model` kept as a named type; each `msg M …` → a
  write route `route M = POST "/x/_/m" { …payload } [form F]` (path is the builder's; a readable
  default under the page's path) with `M.update` holding that arm's body and `Rerender` →
  `Response.refresh`, `Redirect p` → `Response.redirect p`, `Reject t` → `Response.invalid t`,
  `Notice.with t n` → `n |> Response.toast t`; `Html Msg`/`Html a` → `Html`. Arms that read `model`
  are rewritten with a hole and a comment (the model is gone; the arm must `render`/`refresh` or
  query), and `alder check` lists them. Every fixture, the book's snippets, `alder-test`,
  `alder-dx-demo` and the bench fixtures go through it; nightvote-replay and alder-app are the
  user's to run.
- The compiler accepts only the new syntax after this lands (E-code naming the rewrite command on
  `page`, `msg`, an unqualified `load`/`view`/`update` in a module with a `route`, `Nav`).
- Generators: `alder new page` → `alder new route` (GET: `load`+`view`; with `--method POST` and
  `--form F`: `update`); `alder new form` adds a `form`, a POST route and the form markup to an
  existing GET route's view, and takes `--hole` to leave the `update` body a hole instead of
  picking a table (#160's smaller idea); `alder new app` scaffolds routes. The generator modal in
  alder-tui and the LSP's generator commands follow.
- Book: ch. 2 tour, 5 HTML (attributes, `Html`), 6 (becomes "Routes": `route`, qualified
  declarations, names, the state rule, `Response` and the render/refresh rule, forms, rules), 7
  effects, 9 stdlib (`Response`), 10 testing, 11 tooling (dev panel wording), 13 compiler (the
  request walk-through, generated routes, how the runtime consumes a `Response`), 14 and 15
  (second app, generators), A-grammar, B-pitfalls, C-design-system where `Html a` shows. Recipes
  for #161 use `Response` from the start.
- Tooling: tree-sitter-alder (route line, qualified declarations, methods), alder.nvim and
  alder-vscode highlighting and outline (routes as the top level, their declarations grouped),
  the LSP (hover on a route shows method, path, params; go-to-definition from `Row.view` and from
  `click={Toggle …}` to the route line; completion of `R.` members; the elements list of
  2026-10-30-lsp-elements-design gains routes), `alder fmt` (route lines grouped, qualified
  declarations), the dev panel (requests list method and route name and status; the Patches tab
  shows `#todo-7 (render Row)`, `body (redirect Index)`, regions for `refresh`).

## Patterns, validated

| pattern | routes | delivery | loads |
|---|---|---|---|
| list page | `Index = GET` | document / body morph | 1 |
| filter, sort, tab, pagination | none: `href={Index { filter: Done }}` | boosted link, URL moves | 1 |
| create via form | `Create = POST … form` → `redirect (Index {}) \|> toast "Saved"` | body morph, toast; 303 without JS | 1 |
| refuse a form | `invalid "name taken"` | 422, form kept, error shown | before-view + after |
| toggle a row | `Toggle = PATCH` → `render (Row { id })` | one `<li>` by id | 1 row |
| delete a row | `Delete = DELETE` → `refresh` | `#todos` patched | 2 |
| add a comment | `Comment = POST … form` → `refresh` | `#comments` patched | 2 |
| like + count elsewhere | `Like = POST` → `refresh` | both regions | 2 |
| inline edit | `EditRow = GET` via `click=`; `Save = PUT` → `render (Row …)`; cancel `click={Row …}` | row ↔ form in place | 1 row each |
| search as you type | `Results = GET { q }` via `input=` | `#results` replaced | 1 |
| infinite scroll | `More = GET { after }` via `intersect=` on a sentinel | sentinel → rows + sentinel | 1 |
| slow widget | `Stats = GET` via `load=` on a placeholder | placeholder replaced | 1 |
| modal form | `NewDialog = GET` via `click=` into `#dialog`; `Create` → `redirect (Index {})` | dialog, then page | 1 |
| wizard | `Step2 = GET { name }`; `Next1 = POST` → `redirect (Step2 { name: … })` | URL per step; back works | 1 |
| login, logout, theme | `POST` → `Session.put` → `redirect (Home {})` / `refresh` | as today | 1–2 |
| polling badge | `Badge = GET` via `every=` | partial | 1 |
| CSV export | `Export = GET "/export.csv"` with a file answer | as file pages today | 1 |

**Draft preview** is the one pattern the walk-through did not close: `update` sees the posted
draft but has no view; a GET route cannot see a form. The honest HATEOAS shapes are a draft row
(`Preview` writes it, `render (Rendered { id })` reads it) or the draft in the URL when it is
short. Recorded as open for #161's recipes; not solved here **(call)**.

## What this gives later work

- #165 streaming: a write route whose `Task` yields `render r` several times before its final
  response (`Response.stream`), and a GET route a page can `watch` so the runtime re-delivers it
  when its rows change.
- #152 endpoints: `Response.json`, `Response.status`, `Response.header`; a POST route whose
  `params.form` is a JSON body.
- #161 recipes: every row of the table above is a recipe; the wizard recipe is the URL home.
- #49 client state: unchanged position; `data-preserve-attr="open"` on `<details>` still welcome.
- `Response.remove`/`Response.patch` and keyed child diffs: measured against `refresh` on the
  bench rows fixture.

## Left to the builder

- The exact E/W codes and messages; the migration command's name and the default path for a
  message-turned-route; where `_method` override lives in the request pipeline.
- `Alder-Page` versus reusing `Referer` for the shown page; `Alder-Build`'s value (the binary's
  build hash); the Go/Datastar helper names; the Go representation of `Response`.
- Whether `load=`/`intersect=`/`input=`/`every=` land in the same plan or a second one.
- Datastar's behaviour for several roots on one selector, and the shape of `More` accordingly.
- Debounce value for `input=` and how it is spelled if it needs to be tunable.
- The order of the book rewrite; `book/site/build.py` regenerates.

## Tests the builder adds (by behaviour)

- Parser/checker: `route` lines with each method; `form F` only on write routes; qualified
  declarations resolve to their route; cross-module `Todos.Row.view` and `Todos.Row { id }`; a
  route named like an import alias rejected; a module `Todos.Row` beside a route `Row` in `Todos`
  rejected; a GET route missing `load` or `view`, a write route missing `update`, an unmatched
  `view` argument type, a `load` or `view` on a write route, an `href` to a write route, a
  `submit` to a GET route, a `load=` to a write route, `click` to a GET route accepted; two routes
  with one path and different methods accepted, same method rejected; `Html` with a type argument
  rejected with the migration hint; `page`/`msg`/`Nav` rejected with the hint; W0005 on
  `Response.invalid` without `<Errors form />`; `Notice.with` deprecated with the replacement named;
  `refresh`/`invalid` reachability per write route.
- Generated routes: a GET route's handler; a write route's handler with and without a before-view
  (by reachability); `_method` override only on POST; 405 with `Allow`.
- Runtime delivery: plain/boost/element for one GET route; element delivery of a view without a
  root id (error, DOM untouched); `redirect` to another route (body morph, history) and to the
  shown route (body morph, not diffed); `render` (one patch by id, no page load: assert `loads`);
  `refresh` (regions; regions the server did not change untouched, with a stale `al-shown`);
  `invalid` (422, form kept); a stale `Alder-Build` → full reload; the plain-HTTP answer for each
  without `Datastar-Request`; toasts after each; `Response.toast` on a 303 survives the redirect
  as `Notice.with` does today.
- Test browser: `Test.click` on a GET route patches the held doc; the region check after
  `refresh`; the element check after `render`; `Test.refresh` passes on a toggle and fails on a
  route whose `render` lies; `answer.url` after `redirect`; `answer.status` 422 after `invalid`.
- Migration: goldens for a page module with messages, a page with `model` in an arm (hole emitted),
  a file page, the layout (`Html a` → `Html`), `Notice.with` → `toast`; every fixture rewritten
  and checking.
- Bench: `toggle 1000` via `render` — `loads` 0 for the page, `sql` 2 (one update, one read),
  `bytes` ≈ one `<li>`; `delete` via `refresh` matches today's `Rerender` numbers, with a stale
  hash too. Recorded in `docs/perf/2026-10-bench-baseline.md` as a "#160" section.

## Verification

- `go vet ./...`, `go test ./...` green; `cd experiments && python3 -m unittest`; the book's
  snippet tests; tree-sitter, alder.nvim and alder-vscode suites green on the grammar change.
- `alder bench testdata/bench/rows --count 10` idle, before and after; the keep rule: toggle via
  `render` sends ≤ the fragments spec's 2% and makes 0 page loads; delete via `refresh` after a
  toggle (stale hash) stays on the regions path.
- `alder-test`, `alder-dx-demo` and the bench fixtures migrated by the command and checking;
  nightvote-replay migrated by the user with the command, every suite passing.
