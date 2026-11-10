# REST paths for generated write routes

Design for #184. Decided with the user 2026-11-09; the `alder fix --routes` removal and the
follow-ups are part of it. Builds on `2026-11-03-routes-design.md`; settles the convention #194
(`alder new resource`) will inherit.

## Intent

Since #160 every generator that writes a write route gives it the path `<page path>/_/<RouteName>`
(`POST "/replies/{id}/_/Delete"`, `PUT "/admin/post/{id}/edit/_/Save"`, `POST "/_/Logout"`). The
convention was never chosen: it is the shape `alder fix --routes` needed when it turned a `msg`
arm, which has a name and no URL, into a route, and the generators copied it. It is RPC wearing a
URL: the path names a page and a procedure, the method and the name repeat each other, write URLs
are tied to the page that hosts the button, and `_` is an unexplained magic segment.

The user wants Alder's URLs to be as RESTful as hypermedia and Fielding ask: **a URL identifies a
resource, the method says what happens to it, and the route name is a code identifier that never
appears in the URL.** Generators should make that path the easy one and the default; custom paths
stay allowed everywhere.

This is a generator-level change. Nothing in the language changes; a `resource` declaration was
considered and deferred (see follow-ups). The principle throughout: **hand-written routes may have
any path and method the language accepts; the generators, the book and the error hints encourage
the RESTful one.** Nothing here refuses a URL a person typed.

## Goals

- Every generator writes resource paths: `POST /replies`, `PUT /admin/posts/{id}`,
  `DELETE /replies/{id}`, `DELETE /session`. No generator writes `/_/`.
- The person names the resource when there is one (a table, or an explicit `--path`); the
  generator derives the path and keeps the method it already chooses per route.
- Writes with no resource behind them default to the page's own path, and the second such write on
  a page gets a clear error that asks for a name, rather than an invented URL.
- The book, the examples and the test fixtures agree with the generators.
- `alder fix --routes` and `internal/routefix` are removed; no migration tool replaces them.

## Non-goals

- No new language syntax (`resource`), no new checker rule, no inflector.
- No `/_/` → resource migration tool. Existing apps keep working, since a path is a path; the one
  production app (nightvote-replay, a separate repo) is out of scope and the user's to rename by
  hand, or not. alder-dx-demo in ~/projects is in scope (three lines).
- Where a write lands afterwards stays the `update`'s decision (`Response.redirect`,
  `Response.refresh`); this design does not change the response side. Generators keep today's
  defaults: a form redirects back to its page, admin create/save redirect to `Show`, destroy to
  `Index`, everything else `refresh`.
- No "requesting page" value for `update` (HTMX's `HX-Current-URL`). `Response.refresh` already
  returns to the referring page through the `Referer`, which covers a write route shared by two
  pages. Datastar has the same gap (it sends only `Datastar-Request: true`). Follow-up issue.

## Anti-goals (what would make this a failure)

- A generated URL that lies about the resource: `POST /replies/{id}` for a delete. The method flag
  and the collision error exist so the generator never has to guess a verb into a path.
- A second generator convention that quietly replaces `/_/` (`/replies/{id}/delete`, lowercase
  verbs). Non-CRUD actions are named sub-resources or the person's own `--path`, not verb segments.
- Generators, book and fixtures disagreeing after the change; book snippets are tested, so the
  plan must regenerate and re-run everything.

## Constraints

- Route syntax is unchanged: `route NAME = METHOD "path" { params } [form F] [requires ...]`,
  methods `GET|POST|PUT|PATCH|DELETE` (internal/syntax/parser.go:380-411).
- The checker already refuses two routes with one method and one path shape (E0043,
  internal/check/decls.go:1003-1022, `pathShape` collapses every `{x}`); the runtime ranks a
  literal segment before a parameter (`/replies/new` beats `/replies/{id}`). No new rule is needed
  for the collision `_` was presumably dodging.
- No-JS forms carry a hidden `_method` for PUT/PATCH/DELETE (rt/web/method.go:32-46,
  rt/html/html.go:218-225); Datastar calls `@put`/`@delete` directly. A `DELETE /session` logout
  button works without JavaScript today.
- Generator inputs are `Spec` args and flags (internal/generate/specs.go); the CLI, `alder help
  --json` and the `alder/plan` LSP request (`sets` keyed by arg/flag names) all read the same Spec,
  so a new flag reaches the editors' forms without editor changes. The editors' e2e tests may
  still assert generated text; the builder greps alder.nvim and alder-vscode for `/_/`.
- `page` and `msg` remain recognised by the parser only so that E0046 can name them; E0046's hint
  (`diag.RemovedHint`, internal/diag/diag.go:98) currently says "run `alder fix --routes`".

## The convention, as generators apply it

1. **Resource first.** When the person gives a table (`--into T`, admin's and `new resource`'s
   table) or a `--path`, the path is that resource's. The method says what happens: `POST` on the
   collection creates, `PUT` on the member replaces, `DELETE` on the member removes.
2. **Collection path from a table name.** Lowercase the table's name, hyphenate word boundaries,
   pluralise with a small rule (`y`→`ies`; `s`, `x`, `z`, `ch`, `sh`→`+es`; else `+s`):
   `Entry`→`entries`, `Reply`→`replies`, `BlogPost`→`blog-posts`, `Status`→`statuses`. Odd
   plurals (`Person`) are a `--path` away; no inflector. *(my call)*
3. **Nesting.** The collection nests under the page path when the insert is scoped by one of the
   page's parameters, which the form generator already knows (route_form.go chooses
   `Db.insertScoped`/`Db.insertMine` when the table's partition key is among the page's keys):
   the page path is cut after the segment that binds that key, so a form on `/projects/{pid}` or
   on `/projects/{pid}/edit` inserting a project-scoped `Task` posts to `/projects/{pid}/tasks`,
   and on `/a/{x}/b/{y}` with the key bound by `{x}` it posts to `/a/{x}/tasks`. An unscoped
   insert gives a top-level collection whatever the page: the guestbook form on `/` posts to
   `/entries`. `--path` for anything else. *(my call)*
4. **A write-only path is fine.** `POST /entries` with no `GET /entries` page is correct REST; a
   GET there is a 405 with `Allow`, as the runtime already does. `alder routes` lists it as is.
5. **No resource: the page's own path.** A `new msg`, or a `new form` with no table, defaults to
   `<page path>` with the method given (`POST` by default). The first write on a page is almost
   always a submit-to-self and this is the Remix shape. A second write that would share a method
   and a path shape with any route in the program fails before anything is written; the error
   names the clash and the two ways out (`--path`, or `--msg` to attach a form to the existing
   route). The editor form pre-fills the page path, so the clash is visible before generating.
   *(user's choice)*
   - The clash is program-wide, as E0043 is (one `seen` map over every module, with `pathShape`
     collapsing `{x}`), not per page. The generators already load and resolve the program, so the
     pre-write check runs the checker's own rule over the parsed program plus the new route, never
     a text scan. Every generator that writes a write route runs it, `new msg App --path` included.
   - A table clash is refused, not reused: two pages each running `new form … --into Entry` both
     derive `POST /entries`; the second refuses with the `--msg SubmitEntry` hint, because it is
     one resource and should be one route. *(decided with the user)*
6. **`--path` everywhere, `--method` where it was missing.** Every generator that writes a write
   route accepts `--path`; `new msg` and `new form` gain `--method` so that
   `new msg Replies.Show Delete --method DELETE` writes `DELETE "/replies/{id}"` and the person
   never has to type the page's path to get the right verb.
7. **Non-CRUD actions** are named sub-resources, lowercase, no `_`: the state as a noun
   (`PUT /replies/{id}/archived`, `DELETE /replies/{id}/pin`) or the person's own `--path`. No
   generator writes a verb segment.
8. **The response is separate.** The path says what a write touches; the `update` says where you
   go next. Generators keep today's response defaults (see non-goals).

## Generator by generator

Where the `/_/` default is computed today, and what replaces it.

| Generator | Today | After |
|---|---|---|
| `new route <module> <path>` | explicit path, no default; `--example`'s Bump is `<path>/_/Bump` (route.go:317) | unchanged; Bump becomes `POST <path>` (the example page's own path, rule 5), keeping any `{param}` the path declares |
| `new form <page> <name> --into T` | `POST <page>/_/Submit<Name>` (route_form.go:157,169) | `POST <collection of T>` per rules 2–3; `--path`, `--method` override; `--msg` unchanged |
| `new form <page> <name>` (no table) | same | `POST <page path>` (rule 5); second one is E0043 with the hint |
| `new msg <page> <name>` | `POST <page>/_/<Name>` (route_msg.go:102) | `<method> <page path>`, `--method` default POST, `--path` override; second one is E0043 with the hint |
| `new msg App <name>` | `POST /_/<Name>` (appmsg.go:38,40) | `--path` required (there is no page to borrow a path from); the usage error says so |
| `new auth` | `POST /login/_/SignIn`, `POST /_/Logout` (auth.go, templates/auth_login.tmpl:26) | `GET /login` page unchanged; `POST /session` SignIn; `DELETE /session` Logout. Book and comments say "the session is the resource" |
| `new admin` (admin.go `writePath`, line 595) | `POST <path>/_/Create`, `PUT <path>/{id}/edit/_/Save`, `DELETE <path>/{id}/_/Destroy` | `POST <path>`, `PUT <path>/{id}`, `DELETE <path>/{id}`; `New`/`Edit`/`Delete` GET pages keep `/new`, `/{id}/edit`, `/{id}/delete` |
| `new admin` search (`Search<Col>`, admin.go:1722-1732) | `POST <path>/_/Search<Col>` with a form | a search is a read: one `GET <path>/search` route with a query parameter per searchable column, submitted by a hand-written GET form (book 06-routes.md:156). It sits beside `GET <path>/{id}` safely (literal before parameter). Merging it into `Index` as query parameters is the purer shape but changes `Index`'s load and tests; it is not part of this work *(my call)* |
| `new admin` bulk (admin_bulk.go:95-115) | `POST <path>/bulk/_/BulkDelete`, `POST <path>/bulk/_/BulkSet<Col>` | the selection (`GET <path>/bulk`, ids in the query) is the resource, and the generator writes only two actions, both states of it: `DELETE <path>/bulk` removes the selected rows and `PUT <path>/bulk/<col>` sets a column on them (`PUT /admin/posts/bulk/status`), ids in the form body as today. No verb segment anywhere *(my call)* |
| `new resource` (#194, not built yet) | — | inherits the admin table |

The E0043 message for a duplicate method+path today reads "routes `A` and `B` both answer `M
/path`". The generators check the clash before writing and refuse with that message plus: "give
the new route its own resource with `--path`, or attach the form to `A` with `--msg A`". The
checker's own message is unchanged.

## Removal of `alder fix --routes`

Removed outright, by the user's decision: there are no page-era apps left that anyone wants to
migrate, and a tool that must emit placeholder paths teaches the old convention.

- Delete `cmd/alder/fix.go`, `internal/routefix/` (code, tests, testdata), `testdata/fix/`, and the
  `fix` entry in the CLI's command list and `alder help`.
- E0046 stays (the parser still recognises `page`, `msg`, `Nav`, `Page`, an unqualified
  `load`/`view`/`update`) so page-era code gets a named error. Its hint changes from "run `alder fix
  --routes` to rewrite this module" to a pointer at the book's routes chapter; the exact wording is
  the builder's. Dropping page/msg recognition from the parser altogether is a follow-up issue.
- tree-sitter-alder/drift_test.go:154 mentions the fix fixtures among excluded sources; update.
- Book: remove `### alder fix` (11-tooling.md:583-680) and the fix mention in its tool list
  (11-tooling.md:32-33); rewrite the sentences that point at it in 03-syntax.md:699, 05-html.md:9,
  06-routes.md:249 and :2087, 10-testing.md:1337, 12-rationale.md:90, 13-compiler.md:121,
  A-grammar.md:115, B-pitfalls.md:125, README.md:101, and the LSP fix-offer text
  (11-tooling.md:2704-2706). Grep for `fix --routes` and `alder fix` after.
- Close #188 (`alder fix --routes` follow-ups) as won't-do, pointing here.

## Book, examples, fixtures

Generators, docs and tests must agree after the change, so the plan ends with a mechanical pass:

- **Book.** 15-generators.md:663-664 defines "the `<path>/_/<Name>` convention"; replace it with
  the convention above (resource, method, name stays in code, the no-resource default and its
  E0043 nudge). 06-routes.md gets a short "Paths name resources" passage under `### The path`
  (line 106) with the CRUD table and the `/session` example. Every `/_/` route line in the book
  (33 lines across 02-tour, 05-html, 06-routes, 10-testing, 11-tooling, 13-compiler,
  14-second-app, 15-generators, C-design-system) is rewritten to its resource form; the `alder
  routes` sample table (11-tooling.md:540-556) too. Book snippets are tested (book/book_test.go).
- **Examples.** examples/blog (26 `/_/` lines), examples/todos (18), examples/workspace (44), and
  alder-dx-demo in ~/projects (3) are regenerated or hand-edited to the new paths; they must
  check and their tests pass.
- **Fixtures.** testdata/corpus (102), golden (81, plus ~41 generated-Go goldens that embed
  `Route.Path`), generate (120), mutate (35), and the ~40 lines in 19 `_test.go` files. Goldens
  regenerate; corpus and mutate cases are edited by hand where a `/_/` path is incidental to what
  the case tests.
- **Editors.** alder.nvim and alder-vscode e2e suites assert generated text in places; grep both
  for `/_/` and update alongside.

## Follow-ups to file

1. **`resource` in the language**: a declaration pages and write routes attach to, so the checker
   knows `/replies/{id}` is a `Reply` and #194 can build on it. Deferred by the user; revisit much
   later, after `new resource` has shown what it needs.
2. **Expose the requesting page to `update`** (`Request.from`-style, fed by the `Referer` or by a
   signal the runtime adds), for an `update` that must branch on where the request came from.
   Neither Alder nor Datastar has it today; HTMX's `HX-Current-URL` is the precedent.
3. **Drop `page`/`msg` recognition from the parser** once E0046 has served its purpose.
4. **#194 `alder new resource`** inherits this convention; its issue already says so.

## Left to the builder

- The exact E0046 hint text and the exact generator refusal text.
- The admin search route's query-parameter names.
- The pluralisation helper's home (`internal/names` is the likely place) and its tests.
- Order of the mechanical pass (generators → goldens → examples → book) and how much of it is
  scripted.
- Whether `new form`'s `--method PUT` with a member path is wired now or left for #194's
  `--edit T`; either is fine as long as the flag exists and `--path` works.
