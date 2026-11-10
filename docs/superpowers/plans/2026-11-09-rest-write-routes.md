# REST paths for generated write routes (#184) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** No generator writes a `<page>/_/<Name>` path any more. Write routes get resource paths
(`POST /entries`, `PUT /admin/posts/{id}`, `DELETE /session`). `alder fix --routes` is removed.
The book, examples and fixtures agree.

**Architecture:** Two shared helpers come first: a collection-path rule (`names.Collection`) and a
program-wide method+path clash check that mirrors the checker's E0043 (`routeClash`). Each
generator (msg, form, route --example, auth, admin) then swaps its `/_/` path for the convention
and runs the clash check before writing. The removal of `alder fix` and the mechanical pass over
fixtures, book and examples follow.

**Tech Stack:** Go (`internal/generate`, `internal/names`, `internal/check`, `internal/diag`,
`cmd/alder`), Alder sources and goldens, the book's tested snippets.

**Spec:** `docs/superpowers/specs/2026-11-09-rest-write-routes-design.md`. Read it with this plan.
Its decisions are settled; do not reopen them.

## Global Constraints

- No language change: no new syntax, no new checker rule. The checker's E0043 message stays
  byte-for-byte as it is.
- Hand-written routes may have any path the language accepts. Nothing refuses a typed `--path`
  except E0029 (bad path) and the clash check.
- No generator writes `/_/` or a verb segment (`/delete`, `/save`). Non-CRUD actions are a
  person's `--path`.
- Plural rule exactly: lowercase, hyphenate word boundaries, then `y`→`ies` (after a consonant),
  `s`/`x`/`z`/`ch`/`sh`→`+es`, else `+s`. `Entry`→`entries`, `Reply`→`replies`,
  `BlogPost`→`blog-posts`, `Status`→`statuses`. No inflector.
- Generator refusal on a clash, after the E0043 sentence: "give the new route its own resource
  with `--path`, or attach the form to `A` with `--msg A`". `A` is the route already there,
  written `Module.Name`.
- Response defaults stay as today: a form redirects back to its page, admin create/save redirect
  to `Show`, destroy to `Index`, everything else `Response.refresh`.
- Every task ends with `go build ./...` and `go vet ./...` clean, plus `go test` for the packages
  it touches. Goldens are rewritten with `go test ./cmd/alder -run TestGenerate -update` (and
  `-update` on the package whose golden test changed). Review the diff of every rewritten golden.
- When a generator's output changes, the book passage that shows that output changes in the same
  task. Run `go test ./book/...` when the book changes.
- No timing estimates. Comments and messages follow the house style: plain sentences, as in the
  surrounding code. Commit messages end with the session's attribution lines.
- Work on main (no side branch). Commit after each task. Push at the end.

## Decisions this plan makes (the spec left them to the builder)

- **Nesting (rule 3).** A partition key is bound by the path only when a `{param}` of the page's
  path has the key's name (`/admin/orgs/{org}/…` with `@partition org`). Then the collection
  nests after that segment. Otherwise the key comes from the session, and the collection is
  top level: the workspace's `NewProject` form on `/projects` posts to `/projects`. The page's
  path parameters stay the route's parameters, so those not in the new path ride the query
  string and the redirect back to the page still has them.
- **Editor prefill (rule 5).** No new prefill mechanism. Left empty, `--path` means the page's
  path, and the editor's preview shows the derived route line and any clash refusal. That makes
  the clash visible before generating, as the spec asks.
- **Admin search.** The spec's `GET <path>/search` route does not fit the code: the searches are
  on New and Edit, and the narrowed list is already a page parameter (`findParam`). The search
  becomes a GET form with no `action` and no `submit`, which the browser sends to the page's own
  URL with the query key `findParam(c)`. The `Search<Col>` POST routes, their forms and updates
  go away. This keeps the spec's intent (a search is a read, a GET with query parameters) and
  does not change any page's `load`.
- **Admin bulk.** `DELETE <path>/bulk` (BulkDelete) and `PUT <path>/bulk/<col>` (BulkSet<Col>),
  `<col>` the column name lowercased and hyphenated (`status`, `due-date`).
- **E0046 hint.** "see the book's chapter 6, Routes, for the route syntax that replaced it".
- **Pluralisation helper home.** `internal/names/plural.go`, `func Collection(name string) string`.
- **`new form --method PUT`.** The flag exists and is passed through; there is no member-path
  derivation (that is #194's `--edit T`). A PUT form with no `--path` gets the page's path.

## Review Focus

- A page at `/` (root): `new msg Home Like` must write `POST "/"`, never `""` or `"//"`. Pinned
  in Task 3 (`TestRouteMsg_RootPagePath`).
- A second `new form … --into Entry` on another page must refuse with the `--msg` hint, not
  write a second `POST /entries`. Pinned in Task 4 (`TestRouteForm_TableClashRefused`).
- A clash with a route that has another parameter name (`/replies/{id}` vs `/replies/{rid}`)
  must still refuse, as E0043 would. Pinned in Task 2 (`TestRouteClash_ParamNamesIgnored`).
- `--msg A` attaching a form to an existing write route must not run the clash check against
  `A` itself. Pinned in Task 4 (`TestRouteForm_MsgAttachNoClash`).
- `DELETE /session` logout pressed from a page must still work without JavaScript (hidden
  `_method`). Pinned in Task 6 (`TestAuth_LogoutIsDelete` presses it through `Test.click`).

---

### Task 1: The collection path of a table

**Files:**
- Create: `internal/names/plural.go`
- Test: `internal/names/plural_test.go`

**Interfaces:**
- Produces: `names.Collection(table string) string`: `"BlogPost"` → `"blog-posts"`.

- [ ] **Step 1: Write the failing test** `TestCollection`, table-driven:
  `Entry→entries`, `Reply→replies`, `BlogPost→blog-posts`, `Status→statuses`, `Box→boxes`,
  `Quiz→quizes` (the rule is `+es`, no doubling), `Match→matches`,
  `Wish→wishes`, `Day→days` (vowel before `y`), `Post→posts`, `HTTPLog→http-logs`
  (an acronym run is one word), `Person→persons` (no inflector).
- [ ] **Step 2: Run** `go test ./internal/names -run TestCollection`. Expected: FAIL, undefined.
- [ ] **Step 3: Implement** `Collection`. Split on lower→upper and on an upper followed by
  upper+lower (acronym end), join lowercased with `-`, pluralise the last word only.
- [ ] **Step 4: Run** the test. Expected: PASS.
- [ ] **Step 5: Commit** `names: Collection, a table's collection path segment (#184)`.

### Task 2: The program-wide route clash check

**Files:**
- Modify: `internal/check/decls.go`: export `PathShape` (rename `pathShape`, update callers).
- Create: `internal/generate/route_write.go` already exists; add `routeClash` and `clashRefusal`
  there.
- Test: `internal/generate/route_write_test.go` (create if absent).

**Interfaces:**
- Produces: `routeClash(p *Program, method, path, except string) *Route`: the first route of the
  program (any module, `p.Order`-independent) whose method equals `method` and whose
  `check.PathShape` equals `PathShape(path)`, skipping the route whose `Module.Name` is `except`
  (`""` skips nothing). Nil when none.
- Produces: `clashRefusal(have *Route, newName, method, path string) error`: a `Refusef` whose
  text is `routes `<have.Module>.<have.Name>` and `<newName>` both answer `<method> <path>`;
  give the new route its own resource with `--path`, or attach the form to
  `<have.Module>.<have.Name>` with `--msg <have.Name>``. Generators other than `new form` pass
  a newName and use the same function; the `--msg` clause names the route already there.

- [ ] **Step 1: Write failing tests** on a small program built the way the existing generate tests
  build one (copy the helper used in `route_form_test.go`):
  - `TestRouteClash_Found`: program with `route Delete = DELETE "/replies/{id}"`;
    `routeClash(p, "DELETE", "/replies/{id}", "")` returns it.
  - `TestRouteClash_ParamNamesIgnored`: same program, path `/replies/{rid}` → found.
  - `TestRouteClash_MethodDiffers`: `PUT /replies/{id}` → nil.
  - `TestRouteClash_TrailingSlash`: `/replies/{id}/` → found.
  - `TestRouteClash_Except`: except `"Replies.Show.Delete"` → nil.
  - `TestClashRefusal_Text`: the message equals the text above for `Replies.Show.Delete`.
- [ ] **Step 2: Run** `go test ./internal/generate -run 'TestRouteClash|TestClashRefusal'`.
  Expected: FAIL, undefined.
- [ ] **Step 3: Implement** both, iterating `p.Routes` (or the per-module route lists the
  Program already holds). Use `check.PathShape`; do not copy it.
- [ ] **Step 4: Run** the tests and `go test ./internal/check`. Expected: PASS.
- [ ] **Step 5: Commit** `generate: routeClash, the pre-write E0043 check (#184)`.

### Task 3: `new msg` writes the page's path, with `--method` and `--path`

**Files:**
- Modify: `internal/generate/msg.go` (`MsgOptions` gets `Method, Path string`; App branch)
- Modify: `internal/generate/route_msg.go:28-110` (doc comment, path, method, clash)
- Modify: `internal/generate/appmsg.go` (signature takes `path string`, doc comment)
- Modify: `internal/generate/specs.go` (`new msg` flags)
- Modify: `book/15-generators.md` (the `new msg` section and its `/_/` examples)
- Test: `internal/generate/route_msg` tests (find the existing `new msg` tests; add beside them),
  `cmd/alder` generate goldens for `new msg`.

**Interfaces:**
- Consumes: `routeClash`, `clashRefusal` (Task 2).
- Produces: `MsgOptions{InFor, When, ID, Method, Path string}`;
  `newAppRouteMsg(p *Program, name string, fields []FieldSpec, path string) ([]Edit, error)`.
- Spec flags: `{Name: "method", Kind: ArgChoice, Choices: POST PUT PATCH DELETE, Value:
  "POST|PUT|PATCH|DELETE", Summary: "the write route's method (POST by default)", Example:
  "DELETE"}` and `{Name: "path", Kind: ArgRoute, Value: "/path", Summary: "the write route's
  path (the page's own path by default; required on App)", Example: "/replies/{id}/pin"}`.

- [ ] **Step 1: Write failing tests:**
  - `TestRouteMsg_PagePath`: page `Replies.Show` at `/replies/{id}`; `new msg Replies.Show Delete
    --method DELETE` writes `route Delete = DELETE "/replies/{id}" { id: Reply.ReplyId }`.
  - `TestRouteMsg_DefaultPost`: no `--method` → `POST "/replies/{id}"`.
  - `TestRouteMsg_RootPagePath`: page at `/` → `POST "/"`.
  - `TestRouteMsg_ExplicitPath`: `--path /replies/{id}/pin --method DELETE` → that path. A
    `{param}` of `--path` that is not a page path parameter or a field is refused with the
    checker's own "each segment must appear in the parameter record" error (from the probe).
  - `TestRouteMsg_SecondOnPageClashes`: a second `new msg Replies.Show Archive` (POST, page path)
    when `Like` already answers `POST /replies/{id}` refuses with `clashRefusal`'s text.
  - `TestAppMsg_PathRequired`: `new msg App Ping` refuses: "`alder new msg App` needs `--path`:
    a route of App has no page to take a path from, such as `--path /theme`".
  - `TestAppMsg_Path`: `new msg App ToggleTheme --path /theme --method PUT` writes
    `route ToggleTheme = PUT "/theme" { }`.
  - `TestAppMsg_BadPath`: `--path theme` refuses with `check.PagePathRule`.
- [ ] **Step 2: Run** `go test ./internal/generate -run 'TestRouteMsg|TestAppMsg'`. Expected: FAIL.
- [ ] **Step 3: Implement.** Path = `o.Path`, else `r.Path`. Method = `o.Method`, else `POST`;
  `GET` is not a choice. Validate `o.Path` with `check.PagePathProblem` before anything else.
  Call `routeClash(p, method, path, "")` before building edits. Update the doc comments
  (route_msg.go:28, appmsg.go:11) to the new shape. Wire the flags in `specs.go` and refuse
  `--path`/`--method`-free App in `NewMsg`.
- [ ] **Step 4: Rewrite the `new msg` goldens** (`-update`), review the diff: only paths and
  methods changed. Update the book's `new msg` passages in 15-generators.md.
- [ ] **Step 5: Run** `go test ./internal/generate ./cmd/alder -run 'Msg|TestGenerate'` and
  `go test ./book/...`. Expected: PASS. Also `go test ./cmd/alder -run Help` (help.json may need
  `-update` for the new flags).
- [ ] **Step 6: Commit** `new msg: the page's own path, --method and --path (#184)`.

### Task 4: `new form` posts to the table's collection

**Files:**
- Modify: `internal/generate/form.go` (`FormOptions` gets `Method, Path string`)
- Modify: `internal/generate/route_form.go:12-20` (doc), `:155-170` (path, method, clash)
- Modify: `internal/generate/specs.go` (`new form` flags, same two as Task 3 with Summary
  "the write route's path (the table's collection, else the page's own path)")
- Modify: `book/15-generators.md` (the `new form` section and `:663-664`, the convention)
- Test: route_form tests and `new form` goldens.

**Interfaces:**
- Consumes: `names.Collection` (Task 1), `routeClash`, `clashRefusal` (Task 2).
- Produces: `formPath(r *Route, into *Table) string` in route_form.go: `"/" +
  names.Collection(into.Name)`, nested after the page-path segment `{k}` when `into.PartKey ==
  k` (the prefix of `r.Path` through that segment, then `/` + collection); `r.Path` when `into`
  is nil.

- [ ] **Step 1: Write failing tests:**
  - `TestRouteForm_IntoCollection`: guestbook page `/` with `--into Entry` → `route SubmitEntry =
    POST "/entries" { } form Entry`.
  - `TestRouteForm_InferredTableCollection`: no `--into`, fields fit `Reply` → `POST "/replies"`.
  - `TestRouteForm_UnscopedFromNestedPage`: page `/projects/{pid}/edit`, unscoped table `Note` →
    `POST "/notes" { pid: … }` (the page param rides the query; redirect back still compiles).
  - `TestRouteForm_NestedByPathKey`: page `/a/{org}/b/{y}`, table `@partition org` → path
    `/a/{org}/tasks`. Use a silo-free fixture where the route `requires org` and has `{org}`.
  - `TestRouteForm_NoTablePagePath`: `--hole` on page `/contact` → `POST "/contact"`.
  - `TestRouteForm_PathAndMethodFlags`: `--path /contact/messages --method PUT` → `PUT
    "/contact/messages"`.
  - `TestRouteForm_TableClashRefused`: two pages each `new form … --into Entry`; the second
    refuses with `clashRefusal` naming the first's `SubmitEntry`.
  - `TestRouteForm_MsgAttachNoClash`: `--msg Existing` attaching to a form-less write route
    writes no route line and runs no clash check.
- [ ] **Step 2: Run** `go test ./internal/generate -run TestRouteForm`. Expected: FAIL.
- [ ] **Step 3: Implement.** Compute `into` before the path (it is already computed above :157;
  move the path line after the table switch if needed). Path: `o.Path`, else `formPath(r, into)`.
  Method: `o.Method`, else `POST`. Skip the clash check when `attach != nil`. Update the doc
  comment at :12.
- [ ] **Step 4: Rewrite goldens** (`-update`), review. Rewrite 15-generators.md:663-664 with the
  convention: a URL names a resource; the method says what happens; the route's name stays in
  code; no table means the page's own path; a second such write is refused with the `--path` /
  `--msg` way out.
- [ ] **Step 5: Run** `go test ./internal/generate ./cmd/alder -run 'Form|TestGenerate|Help'`,
  `go test ./book/...`. Expected: PASS.
- [ ] **Step 6: Commit** `new form: the table's collection path, --method and --path (#184)`.

### Task 5: `new route --example` Bump answers on the page's path

**Files:**
- Modify: `internal/generate/route.go:40,114,317`
- Test: `new route` goldens; book passages that show the example (`grep -rn 'Bump' book`).

- [ ] **Step 1:** Change the expected golden/test text first: `route Bump = POST "/archive"
  { count: Int, by: Int }`; for `/`, `POST "/"`.
- [ ] **Step 2: Run** the example tests. Expected: FAIL.
- [ ] **Step 3: Implement** `d.BumpPath = o.Path` (trailing `/` trimmed except for `/` itself).
  Fix the comments at :40 and :114.
- [ ] **Step 4: Rewrite goldens**, update the book's example snippets, run
  `go test ./internal/generate ./cmd/alder -run 'Route|TestGenerate' ./book/...`. Expected: PASS.
- [ ] **Step 5: Commit** `new route --example: Bump posts to the page's own path (#184)`.

### Task 6: `new auth`: the session is the resource

**Files:**
- Modify: `internal/generate/templates/auth_login.tmpl:26` → `route SignIn = POST "/session" { }
  form Credentials`
- Modify: `internal/generate/auth.go:45,48,267,270,284` → `route Logout = DELETE "/session" { }`;
  comments say "the session is the resource: POST /session signs in, DELETE /session signs out".
- Test: `internal/generate/auth_test.go`, auth goldens, book auth passages.

- [ ] **Step 1: Write failing test** `TestAuth_LogoutIsDelete`: the generated App has
  `route Logout = DELETE "/session" { }`, Login has `route SignIn = POST "/session"`, and the
  generated app's own tests pass (run the generated app's `alder test` as the existing auth tests
  do; its Logout test presses `click={App.Logout}`).
- [ ] **Step 2: Run** it. Expected: FAIL.
- [ ] **Step 3: Implement** the template and auth.go changes. Run `routeClash` for both routes
  before writing (an app with a hand-written `POST /session` must refuse).
- [ ] **Step 4: Rewrite goldens**, update book passages showing auth routes, run
  `go test ./internal/generate ./cmd/alder -run 'Auth|TestGenerate' ./book/...`. Expected: PASS.
- [ ] **Step 5: Commit** `new auth: POST and DELETE /session (#184)`.

### Task 7: `new admin`: CRUD, bulk and search paths

Model: Opus (the admin generator is large and its tests are generated).

**Files:**
- Modify: `internal/generate/admin.go:593-595` (delete `writePath`), `:1876` Create →
  `POST path`, `:2207` Save → `PUT path + "/{id}"` (the Show path; use the id param's name the
  Edit route already uses), `:2546` Destroy → `DELETE path + "/{id}"`.
- Modify: `internal/generate/admin.go:1711-1740` (`finders`) and the finder tests in
  `admin_bulk.go:350-400`: no Search routes; the view's form becomes
  `<form method="get"><al-input><label>Find … <input type="search" name="<findParam(c)>"
  value={…} /></label></al-input><al-button type="submit" …>Find</al-button></form>`; the tests
  `Test.get` the page with the find parameter instead of `Test.submit Search…`. The "visitor's
  search goes to log in" test is dropped (no search route), and its comment with it.
- Modify: `internal/generate/admin_bulk.go:88-115`: BulkDelete → `DELETE path`; BulkSet<Col> →
  `PUT path + "/" + kebab(c.Name)`.
- Modify: `book/15-generators.md` admin listings, `book/06-routes.md` if it shows admin routes.
- Test: `internal/generate/admin_test.go`, `cmd/alder/admin_test.go`, admin goldens
  (including `fixture-silo`).

- [ ] **Step 1: Write failing test** `TestAdmin_RestPaths`: for table `Post` at
  `/admin/posts`, the generated modules contain `route Create = POST "/admin/posts"`,
  `route Save = PUT "/admin/posts/{id}"`, `route Destroy = DELETE "/admin/posts/{id}"`,
  `route BulkDelete = DELETE "/admin/posts/bulk"`, `route BulkSetStatus = PUT
  "/admin/posts/bulk/status"`, and no line containing `/_/` or `Search`.
- [ ] **Step 2: Run** `go test ./internal/generate -run TestAdmin_RestPaths`. Expected: FAIL.
- [ ] **Step 3: Implement** the path changes and the finder rewrite. Check that `PUT
  /admin/posts/{id}` and `GET /admin/posts/{id}` (Show) differ by method only (allowed), and
  that `GET /admin/posts/bulk` beats `GET /admin/posts/{id}` (literal first, already true).
- [ ] **Step 4: Run** the generated admin apps' own tests the way `cmd/alder/admin_test.go`
  does (silo and non-silo). Expected: PASS, including the finder tests on >200 rows.
- [ ] **Step 5: Rewrite goldens**, review: only paths, the finder forms and tests changed.
  Update the book listings. Run `go test ./internal/generate ./cmd/alder -run 'Admin|TestGenerate'
  ./book/...`. Expected: PASS.
- [ ] **Step 6: Commit** `new admin: resource paths; search is a GET of the page (#184)`.

### Task 8: Remove `alder fix --routes`

**Files:**
- Delete: `cmd/alder/fix.go`, `cmd/alder/fix_test.go`, `internal/routefix/`, `testdata/fix/`.
- Modify: `cmd/alder/main.go`, `cmd/alder/help.go`, `cmd/alder/helpjson.go` (command list),
  `testdata/help.json`, `cmd/alder/test_test.go` (any fix reference).
- Modify: `internal/diag/diag.go:98` `RemovedHint = "see the book's chapter 6, Routes, for the
  route syntax that replaced it"`; `internal/diag/explain.go`, `internal/diag/diag_test.go`.
- Modify: `internal/names/graph.go`, `rt/web/method.go` (comments that mention fix), the LSP's
  fix offer (`grep -rn 'fix --routes\|routefix' internal/lsp`).
- Modify: `tree-sitter-alder/drift_test.go:154` (drop the fix fixtures from the excluded list).
- Modify: book: delete `### alder fix` (11-tooling.md:583-680) and its tool-list entry
  (:32-33); rewrite 03-syntax.md:699, 05-html.md:9, 06-routes.md:249 and :2087,
  10-testing.md:1337, 12-rationale.md:90, 13-compiler.md:121, A-grammar.md:115,
  B-pitfalls.md:125, README.md:101, and 11-tooling.md:2704-2706. Line numbers are from the
  spec; find each by grep.

- [ ] **Step 1:** Update `TestRemovedSyntax`-style tests (diag_test.go and the checker goldens
  that print E0046) to the new hint; run them, expect FAIL.
- [ ] **Step 2:** Change `RemovedHint`, delete the files, remove the command. `go build ./...`.
- [ ] **Step 3:** `go test ./internal/diag ./internal/check ./cmd/alder -run
  'Help|Removed|Check' ./tree-sitter-alder/... -update` where goldens print the hint; review.
- [ ] **Step 4:** Book edits. Verify with
  `grep -rn 'fix --routes\|alder fix\|routefix' --include='*.go' --include='*.md' . | grep -v
  docs/superpowers | grep -v /build/` → no output.
- [ ] **Step 5:** `go test ./book/... ./internal/... ./cmd/alder -run 'Help|Book'`. Expected: PASS.
- [ ] **Step 6: Commit** `remove alder fix --routes; E0046 points at the book (#184, #188)`.

### Task 9: Fixtures and remaining goldens

**Files:** `testdata/corpus`, `testdata/golden` (incl. generated-Go goldens embedding
`Route.Path`), `testdata/generate`, `testdata/mutate`, and every `_test.go` with a `/_/` path
(`grep -rln '/_/' --include='*_test.go' .`).

- [ ] **Step 1:** List every remaining `/_/`: `grep -rn '/_/' testdata internal cmd rt
  tree-sitter-alder --include='*' | grep -v /build/`.
- [ ] **Step 2:** Hand-edit corpus and mutate inputs and `_test.go` literals to resource paths
  using the convention (collection for creates, member for PUT/DELETE, the page's own path for a
  lone POST). Where two routes of one module would then clash, give the second a named
  sub-resource (`/replies/{id}/pin`), never a verb. Where the test's point is the `/_/` text
  itself, keep the case's meaning and change only the path.
- [ ] **Step 3:** Regenerate goldens with `-update` in each package whose test reads them
  (`internal/check`, `internal/gen`, `internal/ir`, `internal/mutate`, `internal/format`,
  `cmd/alder`). Review the diffs: only paths changed.
- [ ] **Step 4:** Run `scripts/test.sh -short`. Expected: every part PASS.
- [ ] **Step 5:** Step 1's grep returns nothing outside the book and examples. Commit
  `fixtures: resource paths for write routes (#184)`.

### Task 10: The book teaches the convention

**Files:** `book/06-routes.md` (`### The path`, line ~106), every `/_/` route line in book
chapters 02, 05, 06, 10, 11 (including the `alder routes` table at 11-tooling.md:540-556), 13,
14, 15, C.

- [ ] **Step 1:** Add "Paths name resources" under `### The path` in 06-routes.md: one
  paragraph (a URL names a resource; the method says what happens; the route's name never
  appears in the URL), the CRUD table (`POST /replies` create, `PUT /replies/{id}` replace,
  `DELETE /replies/{id}` remove, `GET /replies/{id}` read), the `/session` example (sign in is
  `POST /session`, sign out `DELETE /session`), named sub-resources for non-CRUD
  (`PUT /replies/{id}/archived`), and a sentence that hand-written routes may use any path.
- [ ] **Step 2:** Rewrite every remaining `/_/` route line and the prose around it. The book is
  for beginners (explain why, not only what). Then `grep -n '/_/' book/*.md` → nothing.
- [ ] **Step 3:** `go test ./book/...`. Expected: PASS.
- [ ] **Step 4: Commit** `book: paths name resources (#184)`.

### Task 11: Examples and alder-dx-demo

**Files:** `examples/blog`, `examples/todos`, `examples/workspace` (their `app/**/*.alder`,
tests, READMEs; `build/` is regenerated output), `~/projects/alder-dx-demo/app/Home.alder`,
`app/Books/Show.alder`.

- [ ] **Step 1:** Rewrite each `/_/` route to the convention, by hand. In workspace:
  `Projects.Index.Create` → `POST /projects`, `Remove` → `DELETE /projects/{id}` (make `id` a
  path segment), `Projects.Show.Add` → `POST /projects/{id}/todos`, `Toggle` → `PUT
  /todos/{id}/done` (named sub-resource), `Members.Index.Add` → `POST /members`, `Remove` →
  `DELETE /members/{user}`, App's `Logout` → `DELETE /session`, `Leave` → choose by what it
  does (read its update). Do the same reasoning per route in blog and todos.
- [ ] **Step 2:** For each example: `alder check` and `alder test` in its directory, and the
  Go tests that build the examples (`go test ./cmd/alder -run Example`). Expected: PASS.
- [ ] **Step 3:** alder-dx-demo: edit, `alder check`, `alder test`, commit and push in that repo
  (`git -C ~/projects/alder-dx-demo`).
- [ ] **Step 4:** `grep -rn '/_/' examples --include='*.alder' --include='*.md'` → nothing.
  Commit `examples: resource paths (#184)`.

### Task 12: Close out

- [ ] **Step 1:** `scripts/test.sh` (full). Expected: every part PASS. Paste the summary.
- [ ] **Step 2:** `grep -rn '/_/' --include='*.go' --include='*.alder' --include='*.md'
  --include='*.tmpl' . | grep -v /build/ | grep -v docs/superpowers` → nothing.
- [ ] **Step 3:** Whole-branch review (Opus): the diff from 776f98c1 against the spec.
- [ ] **Step 4:** Push main. Comment on #184 with the commits and close it. Close #188 as
  won't-do, pointing at the spec. Update roadmap #52 item 3.
- [ ] **Step 5:** Reinstall `alder` (`go install ./cmd/alder`), fast-forward alder.nvim (no
  grammar change, so no parser rebuild).
