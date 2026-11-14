# `alder new resource`: user-scoped CRUD routes for a table

Design for #194. Decided with the user 2026-11-14 (six questions ruled on from the draft; the
rest is marked **(my call)** where it was chosen while writing and not discussed). Builds on
`2026-11-09-rest-write-routes-design.md` (the path convention it inherits) and on `alder new
admin`, whose generator it shares.

## Intent

`alder new admin` writes full CRUD for every table, but for an Admin role, with the unscoped
database calls and the admin layout. The users of the app have no equivalent: "users manage their
own posts" is assembled from `new table`, `new route --list`, `new route --show`, `new form --into`,
a hand-written edit, a hand-written delete, the scoping calls, the links between the pages and the
tests. `alder new resource Post` should write all of that in one command, the way admin does for
the Admin, and its pieces (`new form --edit T`, `new route --list T --search a,b`) should also work
on their own.

Success: after `alder new resource Post` the app has `/posts`, `/posts/new`, `/posts/{id}`,
`/posts/{id}/edit` and `/posts/{id}/delete`, each user sees and changes only their own rows, another
user's id is a 404, and `alder test` is green with the cross-user cases covered. With
`--read all` the same command gives a blog: anyone reads, the owner writes.

## Goals

- One command writes list (search, filters, sort, paging), show, new/create, edit/save,
  delete/destroy for a table, following admin's layout of one module per page, with tests.
- Ownership is enforced by the scoped calls (`Db.mine`, `Db.getMine`, `Db.insertMine`,
  `Db.updateMine`, `Db.deleteMine`; the `Scoped` family for `@partition k`). No ownership code is
  hand-rolled by the generator; another user's row is a missing row.
- Reads can be wider than writes: `--read owner|signed-in|all` opens Index and Show to every
  signed-in user or to everyone, while writes stay the owner's. The blog shape is one flag.
- Paths follow the REST convention: `GET /posts`, `POST /posts`, `PUT /posts/{id}`,
  `DELETE /posts/{id}`; the route name never appears in the URL.
- The generator is admin's generator with a different *access*: the page bodies, forms, secret
  rules, labels, blockers and tests are shared code, not a copy.
- Owned regions and `alder regen resource`, so that `alder edit table` adding a column can update the
  forms and lists.
- The standalone pieces exist: `alder new form … --edit T` and `alder new route … --list T --search`.

## Non-goals

- No `resource` declaration in the language (#213, deferred). This is a generator.
- No child resources (`--under Parent`): the issue's "Later". The shared table struct keeps a seam
  for it, nothing more.
- No siloed resources. A table in a `silo` is refused with a sentence; admin's picker/dashboard
  model does not fit a user-facing page.
- No edit to the app's `Layout` (admin writes its own layout with a nav; a resource lives in the
  app's pages and the person links its Index where they want).
- No `--only`/`--no-delete`: a page the person does not want is a file they delete, and `regen` does
  not recreate it without `--all`, as admin's.
- No sort flag on plain lists (`--search` only, as the issue says); `--sort` is a natural follow-up
  on the same extraction.

## Anti-goals (what would make this a failure)

- A page that reads or writes a row the user does not own because the generator chose
  `Db.get`/`Db.update` where `getMine`/`updateMine` existed. On a keyed table the unscoped calls
  appear only where they must (blocker counts, `@unique` counts, reference selects), each with a
  comment saying why.
- A second CRUD generator. If `new resource` and `new admin` drift, both lose the tests and fixes the
  other gets. The refactor comes first and admin's goldens must not change by a byte.
- Silent danger: a keyless table that becomes "anyone edits everything" with no word said.
- A list of every user's email on a user-facing form.

## Constraints

- Ownership is the `@owner` attribute on a `@ref User` column (`Table.Owner`,
  `Table.PartKey == "user"`, `Table.PartCol`); `@partition k` generalises it. The checker refuses a
  scoped call on a table without the key (E0025) and in a module whose route does not `requires` the
  key (E0026). `insertMine` fills the owner and refuses an explicit one (E0003); `updateMine` never
  writes it.
- `Db.getMine : Table t -> Id t -> Query t`, no `Maybe`: a missing or foreign id is
  `Query.notFound`. `updateMine`/`deleteMine` give `Task.notFound` (whole task rolled back). `Db.get`
  is `Query (Maybe t)` and needs a `match`. The shared generator must emit either shape.
- **Delete rules and `deleteMine`** (checked for this design): `DeleteMineRow` is one
  `DELETE … WHERE id = ? AND owner = ?` through the same `Store.DeleteWhere` as `Db.delete`
  (`rt/db/store.go:474-508`, `rt/db/sqlite.go:369-396`). Every connection runs with
  `foreign_keys=ON`, and `@cascade`/`@setNull` are `ON DELETE` clauses in the schema
  (`rt/db/sqlite.go:62,563-565`), so the database applies them to any DELETE: **`deleteMine` cascades
  and nulls exactly like `Db.delete`.** A `@restrict` referrer makes it an `eff.ErrFail`
  ("Post 7 is referenced by a row of Comment (`Comment.post`)"), not a 404. So the generated Destroy
  must do what admin's does: count referrers first, show them, and refuse with `Response.invalid`
  before deleting. The counts are unscoped because a referrer may be another user's row.
- `requires user` sends a visitor to login and makes a wrong role a 404; every write route carries
  its own `requires`. Inside the module `session.user` is a plain `UserId`.
- The REST spec's rules 1–3 and 8: collection path = `names.Collection(T)` (`Post`→`/posts`,
  `BlogPost`→`/blog-posts`); create/save redirect to Show, destroy to Index; a program-wide
  method+path clash is refused before writing (`routeClash`, `clashRefusal` in `route_write.go`).
- Generators are `Spec`s (`specs.go`): one declaration feeds the CLI, `alder help --json`, the LSP
  `alder/plan` and the editor modals. `runEdits` → `Load` → `[]Edit` → `Apply` (copy, apply, format,
  check, write); `Probe(appDir, edits)` gives the checked program of a hypothetical edit set.
  Output and `--json` are the shared `genReport`/`genJSON` (`files, diff, diagnostics, holes,
  migration, written, error, next`; regen adds `skipped, upToDate`). Warnings print on stderr as
  `alder new resource: warning: …` (`Result.Warnings`).
- Admin's code is Go string building around `adminGen`/`adminTable` (`internal/generate/admin.go`),
  with every file split into owned regions (`regions.go`) and `RegenAdmin` replacing only regions
  whose fresh body differs; a region the author removed is never re-added. Its tests come from
  `testBuilder` (`samples.go`), which already has `otherUser()`.
- `new route --list/--show` already choose `Db.from`/`Db.mine`/`Db.scoped` from the table's key and
  the route's `requires` (`selectIn`, `route.go:753`); `new form --into` already emits `insertMine`
  and omits the partition column (`fieldsFit`, `form.go:261`). Neither has search, edit or delete.

## Decided

### 1. Command surface

```
alder new resource <Table> [<field>:<Type>[=default][@attr…] …]
                   [--read owner|signed-in|all] [--requires user|<key>[,…]]
                   [--module <M>] [--path /p] [--dir app] [--json] [--dry-run]
alder regen resource [<Table> …] [--all] [--dir app] [--json] [--dry-run]
```

- `alder new resource Post`: the table exists; write its routes.
- `alder new resource Post title:Text body:Text owner:User.UserId@ref=User@owner`: the table does
  not exist; create it exactly as `alder new table` would (same field grammar, attributes,
  timestamps, migration), then write the routes, in one `Apply`. Shape: `edits1 := NewTable(…)`;
  `p2 := Probe(app, edits1)`; `edits2 := resourceFiles(p2, opts)`; `Apply(edits1 ++ edits2,
  Options{Migrate: true})`. Fields given for a table that exists are refused, pointing at
  `alder edit table`.
- `--module` defaults to `PluralConcern(T)` (`Posts`); dotted names nest (`Mine.Posts` →
  `app/Mine/Posts/`). `--path` defaults to `"/" + names.Collection(T)` (`/posts`) and is checked with
  `check.PagePathRule` like admin's.
- `--read` says who may read (Index and Show); writes are never widened by it.
  - `owner` (default on an `@owner` table): every read scoped, the design of §3's first row.
  - `signed-in`: Index and Show `requires user` and read unscoped (`Db.from`, `Db.get` with the
    not-found arm); every row is visible to every signed-in user.
  - `all`: Index and Show carry no `requires` and read unscoped; a visitor reads them.
  - On an `@owner` table the writes stay `requires user` with `insertMine`/`updateMine`/`deleteMine`
    whatever `--read` says: New, Edit and Delete are the owner's pages, and another user's id on
    Edit, Save, Delete or Destroy is still a 404.
  - On a keyless table `--read` means `all` or `signed-in` only; the default is `signed-in` when the
    session has a `user`, else `all`. `--read owner` is refused: "`Note` has no `@owner` column to
    read by; add one (`alder edit table Note add-field owner:User.UserId@ref=User@owner`) or use
    `--read signed-in`".
  - On a `@partition k` table only `owner` is accepted in v1 **(my call)**: `signed-in` and `all`
    would read across partitions, which is what `@partition` exists to prevent. The refusal says
    "rows of `Project` are partitioned by `org` and read within it; `--read` applies to `@owner`
    tables". A cross-tenant public read is a later question, not a default.
- `--requires` is the write routes' guard (and Index/Show's under `--read owner|signed-in`). It
  defaults to the table's partition key, through `routeGuard`: `user` for `@owner`, `user, org` for
  `@partition org` with `via`. For a table with no key it defaults to `user` when the session has a
  `user` field, else to nothing. An explicit `--requires` that omits the table's key is refused:
  "`Post` is owned (`@owner`): its pages need `--requires user`". `--read all` on an `@owner` table
  with no session `user` is refused the same way, since the writes need one.
- No `--shared`. The draft had it as the opt-in for a keyless table; with the ruling that a keyless
  table warns and generates, the flag would only silence one stderr line, which is not worth a flag
  **(my call)**. If an unguarded resource on an app that has a session user is ever wanted, that is
  a later `--requires none`, not this issue.
- Refusals (one sentence, exit 1, nothing written): no such table and no fields; fields on an
  existing table; `app/<Module>/` already exists; a method+path clash (`clashRefusal` with `--path`
  as the way out; `--msg` does not apply); `--requires user` with no `user` in the session
  (`routeGuard`'s text); a silo table; a required secret column with no default that is neither
  `Maybe` nor Text (admin's rule); a bad `--path`.
- `--json`/`--dry-run`: the shared report, nothing added. Dry-run shows the five files (seven with
  table and migration) in full. No `next:` line **(my call)**: nothing has to run first, and the
  Index path is visible in the diff.
- Wiring: a `Spec{ID: "new resource"}` in `specs.go`, `newResourceUsage`, entries in `specUsage`,
  `newUsage`'s kind list and `newGeneratorUsages`; `Here` nil. The editor modals get the generator
  from `help --json`; alder.nvim and alder-vscode kind lists need a one-line check.

### 2. Generated modules

`alder new resource Post` on an `@owner` table, module `Posts`, path `/posts`, five files under
`app/Posts/`:

| File | Routes | Body |
|---|---|---|
| `Index.alder` | `Index = GET "/posts" { q: Text = "", <filter cols>, sort: Text = "<order col>", dir: Text = "desc", page: Int = 1 } requires user` | `Db.mine Post \|> Db.where (Db.and [search q, filtered …]) \|> Db.orderBy … \|> Db.limit perPage \|> Db.offset …`, a count, search form, filter selects, sortable headers, `al-pagination`; link to New; rows link to Show |
| `Show.alder` | `Show = GET "/posts/{id}" { id: Post.PostId } requires user` | `let row <- Db.getMine Post params.id`; fields in a `<dl>`; links Edit and Back; a delete dialog posting `Posts.Delete.Destroy` with blocker counts (admin's shape) |
| `New.alder` | `New = GET "/posts/new" { <finder params> } requires user`; `Create = POST "/posts" { } form PostForm requires user` | `Db.insertMine Post { … }` → `Response.redirect (Posts.Show.Show { id }) \|> Response.success "Saved"` |
| `Edit.alder` | `Edit = GET "/posts/{id}/edit" { id, <finder params> } requires user`; `Save = PUT "/posts/{id}" { id } form PostForm requires user` | load with `getMine`, inputs filled from `model.row`; `Db.updateMine Post params.id (\r -> { r \| … })` → redirect Show |
| `Delete.alder` | `Delete = GET "/posts/{id}/delete" { id } requires user`; `Destroy = DELETE "/posts/{id}" { id } form Confirm requires user` | load with `getMine` plus referrer counts; blocked → `Response.invalid`; else `Db.deleteMine Post params.id` → redirect Index `\|> Response.success "Deleted"` |

- Under `--read signed-in` or `all`, Index reads `Db.from Post |> …` and Show reads `Db.get Post
  params.id` with the `Nothing -> Query.notFound` arm; under `all` the two routes have no
  `requires`, so `session.user` is a `Maybe` there. Show's Edit link, Delete link and delete dialog
  render only for the owner: `if session.user == row.author then … else Html.none` (`signed-in`)
  or `if session.user == Just row.author` (`all`); Index's New link and per-row Edit links follow
  the same test. Edit, Delete, Create, Save and Destroy are unchanged from the table above, so a
  non-owner who types `/posts/7/edit` gets a 404 from `getMine`.
- Layout: the app's `<Layout.Page>`, as `new route` writes; no role check, no `adminOnly`.
- Index richness is admin's: search over the Text columns, Bool/union filters, sort on every shown
  column, paging. `perPage` is 20 **(my call)**, matching `new route --list` (admin's 25 comes from
  `bulkMax`, and there is no bulk here). A person who wants a plain list deletes the `search`,
  `filters` or `sorts` regions; regen leaves them out.
- Delete is both the confirm page and the dialog on Show, as admin.
- Form fields: admin's `editable` set minus the partition column (`insertMine` fills it;
  `updateMine` never writes it), secrets (a required secret is set to `Nothing`/`""` on insert),
  `nowCols` (`Time.now` on insert), ids and timestamps. Ref columns render as an `al-select` over the
  referenced table with admin's 200-row cap and finder; the options query is `Db.mine Ref` when the
  referenced table is itself owned (my categories), else `Db.from Ref`.
- A `@ref User` column that is not the owner (an `assignee`) is left out of the form, and the
  command prints a notice: "`assignee` refers to users and was left out of the form; add its input
  by hand". The column is still shown on Show and Index. Insert writes nothing for it, so it must be
  `Maybe` or have a default; otherwise the refusal is the secret-column one reworded ("`assignee` is
  required and refers to users: give it a default or make it `Maybe`").
- `@unique` keeps admin's count-and-`Response.invalid` guard. The count is unscoped, since the
  constraint is table-wide; the generated code says so in a comment.
- Header line: ``-- generated by `alder new resource Post --module Posts --path /posts`; regions
  between `-- @resource begin <name>` / `-- @resource end` are rewritten by `alder regen resource` ``.
  The header records the options regen needs (§5).
- Pre-write clash check: `routeClash` for each of the eight method+path pairs. (Admin does not call
  it today and relies on the checker's E0043; giving it the same call is a one-line follow-up.)
- Admin's `/admin/tag` (singular, `strings.ToLower`) does not follow REST rule 2; the resource uses
  `names.Collection`. Admin's path is a #216-class leftover and is not changed here.

### 3. Ownership scoping

| Table | `--read` | Index/Show `requires` and reads | Write routes (`requires`, calls) | Owner in form |
|---|---|---|---|---|
| `@owner` | `owner` (default) | `user`; `Db.mine`, `Db.getMine` | `user`; `insertMine`, `updateMine`, `deleteMine` | left out |
| `@owner` | `signed-in` | `user`; `Db.from`, `Db.get` + not-found arm; Edit/Delete shown to the owner only | same as above | left out |
| `@owner` | `all` | none; `Db.from`, `Db.get` + not-found arm; New/Edit/Delete shown to the owner only (`session.user == Just row.owner`) | same as above | left out |
| `@partition k` (no silo) | `owner` only (others refused) | `k` or `user, k`; `Db.scoped`, `getScoped` | same; `insertScoped`, `updateScoped`, `deleteScoped` | left out |
| no key | `signed-in` (default with a session user) or `all`; `owner` refused | `user` or none; `Db.from`, `Db.get` + not-found arm | `user` if the session has one, else none; `insert`, `update`, `delete` behind the same `match`, `Nothing -> Task.notFound` | n/a |
| silo | refused | | | |

- **Another user's id is a 404** on every write route without a line of generated code:
  `getMine`, `updateMine` and `deleteMine` say not found, and under `--read owner` `mine` does not
  contain the row either. The tests assert it on every member route; under `signed-in`/`all` the
  read routes show the row and the write routes still 404.
- **Hiding is not guarding.** The owner test around the Edit/Delete links is presentation; the
  write routes' `requires user` plus the `Mine` calls are the guard. The generated code says so in
  a comment above the `if`, so nobody "fixes" the 404 by widening `Save`.
- **A keyless table warns and generates** (the user's ruling). One warning on stderr, in `--json`
  under the existing warnings channel, naming the field to add:

  ```
  alder new resource: warning: `Note` has no `@owner`: every signed-in user can read and change
  every row. To scope it: alder edit table Note add-field owner:User.UserId@ref=User@owner, then
  alder regen resource Note
  ```

  When the app has no session user the first clause reads "any visitor can read and change every
  row" and the remedy names `alder new auth` first. The generated tests then say what the pages do
  ("any signed-in user sees the row", "any signed-in user can save it") rather than hide it.
- After `alder edit table Note add-field owner:…@owner`, `alder regen resource Note` recomputes
  from the table's current state: the `routes` region gains `requires user`, and `load`, `insert`,
  `update`, `remove` and `tests` switch to the scoped family. Regen rewrites the header's recorded
  `--requires` too when it was the default.
- The only unscoped reads on an owned table's pages: blocker counts (a referrer may be anyone's
  row), `@unique` counts (table-wide), and reference selects over unowned tables. Each carries a
  one-line comment.

### 4. Sharing admin's generator

Composition the other way, a resource made by calling `new route --list`, `--show` and `new form`,
was weighed and rejected: those pieces have no search, edit, delete, regions, page links or
cross-user tests, and it would leave two list generators to keep in step. Admin's page bodies are
the source; the standalone pieces reuse the extracted fragments.

PR 1 extracts, with no behaviour change, from `admin.go` into `internal/generate/crud.go`:

```go
// access says whose rows a set of pages reach and how a route is guarded.
type access struct {
    marker   string // "@admin" | "@resource"               (region markers, header)
    gate     string // "let _ <- Layout.adminOnly session.user" | ""  (first line of load/update)
    layout   string // "Admin.Layout.Page" | "Layout.Page"
    // reads: Index and Show
    readRequires string // " requires user, role Admin" | " requires user" | ""
    sel, get     string // "Db.from"/"Db.get" (Maybe, match) | "Db.mine"/"Db.getMine" | "Db.scoped"/"Db.getScoped"
    // writes: New/Create, Edit/Save, Delete/Destroy (and their loads)
    writeRequires string
    writeGet      string // the row load before Save/Destroy and on Edit/Delete pages
    insert, update, delete string
    ownerOnly string // "" | "session.user == row.owner" | "session.user == Just row.owner": wraps Edit/Delete/New links
    perPage  int    // admin 25 (bulkMax), resource 20
}
type crudTable struct { /* today's adminTable: t, mod, path, plural, title, noun, label,
                           editable, refs, nowCols, secrets, referrer; parent *Column (unused seam) */ }
type crudGen struct { p *Program; a access; names, files map[string]string; order []string; tb *testBuilder }
```

Moved and parameterised by `access`: `index`, `show`, `newPage`, `edit`, `delete`, `formParts`,
`unique`, `labels`, `filters`/`filterColumns`/`searchCols`/`sortColumn`, `shown`, `secretValue`,
`submitValue`, `changedValue`, `optionRows`, `deletePlan`/`closure`/`referrersOf`, `file` (header,
imports region, body), `regionAt`, `routeLine`, `pathLit`, and the test writers (`testsText`,
`missingTest`, `deleteTests`, `notOffered`, the sort/search/page tests). One helper,
`found(idExpr, body)`, emits the `match`-on-`Maybe` shape for `Db.get` and the direct `let` for
`getMine`/`getScoped`. Stays in `admin.go`: `adminCheck`, `layout`, `dashboard`, `bulk`, the silo
picker and dashboard, `gateTests`, `writeGate`'s role variants, `ownRole`/`isMe`, and `RegenAdmin`'s
old-layout refusals. `regions.go`: `RegionBegin`/`RegionEnd` become functions of the marker, the
`@admin` constants kept for existing callers.

Acceptance for PR 1: the `fixture-admin` goldens are byte-identical, `TestAdmin_Mutate` holds, and
`go test ./internal/generate ./cmd/alder` is green.

### 5. Owned regions and `alder regen resource`

- Markers `-- @resource begin <name>` … `-- @resource end`; region names as admin's (every file
  `imports`; Index `routes, search, filters, sorts, labels, headers, cells, tests`; Show `routes,
  model, load, labels, blockers, fields, counts, tests`; New/Edit `routes, model, load, form, search,
  insert|update, finder, inputs, tests`; Delete `routes, model, load, blockers, remove, row, counts,
  tests`). Outside a region is the author's forever; inside is rewritten when the fresh body
  differs; a region whose markers the author removed is never re-added.
- `alder regen resource [Post …] [--all]`: no table names means every resource in the app. Discovery
  and options come from the header line (`alder new resource Post --module Posts --path /posts
  --read owner [--requires …]`; `--read` is always written, so regen never re-derives it); regen
  re-runs that option set against the current program, loading with
  `LoadExcept` on the resource's directory so its own broken files do not refuse the run, as admin
  does. `--all` recreates deleted files. Output and JSON as `regen admin` (`skipped`, `upToDate`).
  Admin's old-layout refusals do not apply. A file whose header was edited away is refused with the
  explicit form as the way out: "`app/Posts/Index.alder` has no `alder new resource` header; pass
  the table and its options: `alder regen resource Post --module Posts --path /posts`"
  (`--module`/`--path` are accepted by regen for that case only).
- `alder edit table T …` stays table-only, but when the app has a resource for `T` its `Result.Next`
  is `alder regen resource T` (and `alder regen admin` when an admin exists), so the text output ends
  with the right `next:` line and `--json` carries it **(my call)**.
- `new form` and `new action` on a resource module stay allowed (they splice outside regions); only
  admin silo routes are refused today.

### 6. Standalone pieces

**`alder new form <Module> <Name> [fields…] --edit T [--method PUT] [--path /p]`**

- Target: a route module whose GET route has an `{id: T.TId}` param and whose `Model` has a field
  of type `T` (what `--show T` writes as `row: T`). Otherwise: "`M` has no `T` row to edit; make its
  page with `alder new route M /… --show T`". `--edit` excludes `--into` and `--hole`.
- Adds `form <Name> = { … }` (the given fields, else every editable column minus partition,
  secrets, `nowCols`, ids, timestamps, non-owner user refs), `Save<Name> = PUT <member path>
  { <page params> } form <Name>` with the page's `requires`, the `update` (`updateMine`,
  `updateScoped` or `update` behind a `match`, by the same `access` logic), the `<form
  submit={Save<Name> { id: model.row.id }}>` with `value={model.row.<col>}` inputs (admin's
  `formParts(edit=true)`), and tests "saving changes every field", "a required field left empty is
  rejected" and, when scoped, "another user's row is not found".
- Path: `formPath` gains a member form, `"/" + names.Collection(T) + "/{id}"`, nested under the
  page's partition segment as rule 3 does for collections. `--method` defaults to `PUT`.

**`alder new route <M> <path> --list T --search [a,b] [--requires …]`**

- `--search` with no columns is admin's `searchCols` (required Text columns that are not ids, refs
  or secrets); named columns are checked to exist and be Text (the refusal names the first that is
  not). Adds the `q: Text = ""` param (cut to 200 with `Text.slice`), `search q = Db.or […]`, the
  `Db.where (search q)` on both the rows query and the count, admin's GET search form above the
  table, and the tests "searching finds matching rows" and "a long query is cut".
- Implementation: `searchParts(t, cols, sel)` in `crud.go` feeds a `Search` field of `RouteData`
  in `route_list.tmpl`. The two list generators share only this fragment; full convergence is not
  attempted.

### 7. Tests generated

In each file's `tests` region, with the mutation gate at or above 0.95 as admin's:

- Index: shows my rows newest first, with labels; does not show another user's rows (rows inserted
  with `{Owner: tb.otherUser()}`); sorting both ways; searching finds; filters; a 20,000-character
  query is cut; 20 a page and a page past the last is the last; a visitor is sent to log in.
- Show: shows the row's fields; a missing row is not found; another user's row is not found; the
  delete dialog (blocked and allowed); visitor redirect.
- New/Create: creating adds a row that is mine (`Db.mine T |> Db.count` from me is 1, from
  `otherUser` 0); an empty required field is rejected (`FormRejected`); a value already used in a
  `@unique` column is refused; a ref id not offered is refused; a visitor cannot create and the
  count is unchanged.
- Edit/Save: saving changes every field; another user's row: GET not found and PUT changes nothing;
  missing row; unique; not offered; visitor.
- Delete/Destroy: deleting removes the row and redirects to Index; another user's row: not found
  and still there; `@restrict` blocks with the referrer named, `@cascade` counts shown; visitor.
- `--read signed-in` on an owned table: Index "shows every user's rows"; Show "shows another
  user's row without Edit or Delete" (`Test.texts` has no Edit/Delete link, the dialog is absent)
  and "shows my row with Edit and Delete"; Edit "another user's edit page is not found"; Save and
  Destroy "another user cannot save/delete it" (404, row unchanged, as under `owner`); visitor
  redirect on Index and Show.
- `--read all`: the above, plus Index and Show "a visitor sees the list / the row" (`Test.get`
  signed out is `Shown`) and "a visitor sees no New, Edit or Delete"; the write routes keep "a
  visitor is sent to log in".
- Keyless table: the cross-user tests read "any signed-in user sees the row", "any signed-in user
  can save it", "any signed-in user can delete it"; under `--read all`, "a visitor sees the row".

CLI tests: `fixture-resource` in `TestGenerate` (base `fixture`, `new resource Post`, `test: true`
so the generated tests run), `fixture-resource-create` (fields, table and migration),
`fixture-resource-keyless` (the warning), `fixture-resource-read-all` (the blog shape, `test:
true`), refusal cases (no table, fields on an existing table, silo, clash, no `user` in the
session, bad `--path`, `--read owner` on a keyless table, `--read all` on a partitioned table),
`--json` and `--dry-run` shape, regen
up-to-date / skipped / `--all` / header missing, `edit table` → `next:`. Book: a `## A resource:
alder new resource` section in `book/15-generators.md` after "A page for a row, and a write route",
an intro bullet, `#### alder new resource` and `#### alder regen resource` in `book/11-tooling.md`,
and `--edit`/`--search` in their existing sections. Snippets are tested, so every example must check.

## Rollout

1. **Extract `crudGen`/`access` from admin.** No behaviour change; goldens byte-identical; marker
   parameterised; `found` helper for the two get shapes. Reviewed on Opus: it reshapes a
   2,600-line file.
2. **`new resource` and `regen resource`**: owner, partition and keyless together, with
   `--read owner|signed-in|all`; silo refused; the warning; the non-owner user-ref notice; goldens;
   `edit table`'s `next:`; the book section (the blog shape as its second example).
3. **`new form --edit T`.**
4. **`new route --list --search`.** (3 and 4 depend on 1 only; either order.)
5. Follow-ups to file, not build: admin's missing `routeClash` call; admin's singular
   `/admin/<table>` path against rule 2 (add to #216); `--sort` on plain lists; `--under Parent`;
   siloed resources; `--requires none` if an unguarded resource is ever wanted; `--read` on
   partitioned tables (a public read across tenants) if a case appears; a draft/published
   column that narrows `--read all` to published rows (the blog example does this by hand).

## Left to the builder

- Exact wording of the generated comments on the unscoped reads, the keyless warning and the
  user-ref notice, within the shapes above.
- Whether `found` is one helper or two small ones; whether `crudTable` keeps admin's name
  internally for a smaller diff in PR 1.
- The header's option order and quoting; regen must parse what `new resource` writes and nothing
  more clever.
- How `edit table` finds a resource for a table: scanning for the header line in `app/` is enough.
- Check that alder.nvim and alder-vscode take the generator kind list from `help --json` rather
  than a hardcoded list; if hardcoded, one-line issues in those repos.
