# M12: Generators and the built-in admin — Design

Date: 2026-10-13. Status: approved in conversation; the M12 plan argues from
this. Related: memory notes on generators by purpose and the admin (user,
2026-10-12), M7a (the `alder-ui` elements every generator emits), M11 (the
rules every generator emits), E11 "CLI-only development" (consumes the
commands here), Olifant (the user's generator engine; an Olifant pack for
Alder is a thin wrapper over these commands).

## Goal

An agent or a person builds an Alder app from the command line: `alder new`
creates pages, tables, forms, messages and capabilities as plain, checked,
tested Alder source; `alder edit table` changes a table and its migration;
`alder fill` fills a typed hole from stdin and refuses anything that does
not check; `alder new admin` generates a Django/Rails-class admin for every
table, regenerable as the schema changes. Everything generated is ordinary
Alder a reader can open, the checker verifies, browser tests cover and
`--mutate` scores.

## Decisions recorded (2026-10-13)

- **Ordinary generators write plain files once** (A); **the admin uses owned
  regions** and `alder regen admin` rewrites them (B).
- **One verb per operation, kinds as subcommands** (A): `alder new <kind>`,
  `alder edit table`, `alder fill`, `alder regen admin`. Every command is
  idempotent and refuses rather than overwrites.
- **The admin needs a role**: `alder new admin` is refused when the session
  has no `role` field whose union has an `Admin` constructor, with the
  sentence that says what to add; it never edits `App.alder` or the login
  flow.
- **The admin has a dashboard index** (counts per table, recent rows).

## The command surface

Every command: reads the app as typed IR (`names.Load` + the checker, as
`alder check` does), computes the edit, applies it to a temporary copy of
the app, runs the checker on the copy, and only then writes the files
(created or edited) into the app. It prints the files written, the unified
diff of edited files, and any diagnostics; `--json` prints `{files:[…],
diff:"…", diagnostics:[…], holes:[{decl,type}]}`. A refusal (a target file
exists, a name is taken, the result does not check, a precondition fails)
exits 1 with one sentence naming the file or the declaration and nothing
is written. `--dry-run` prints what would be written. Names follow the
language's own rules (a module `Posts.Index`, a path `/posts`).

### `alder new page <Module> <path> [--list T] [--show T] [--from T …] [--requires user] [--role R]`

- Writes `app/<Module path>.alder`: `module`, imports, `page "<path>" {
  params }`, `Model`, `load`, `view` (an `al-page` with a heading), and one
  browser test (`Test.get` is `Shown` and the heading is present).
- `--list T`: `Model { rows: List T, page: Int, pages: Int }`, `page`
  params `{ page: Int = 1 }`, `load` with `Db.from T |> Db.orderBy
  T.<created or id>.desc |> Db.limit 20 |> Db.offset …`, `Db.count` for
  `pages`, an `al-table` whose columns are every column but the id
  (rendered by type: Bool and unions as `al-badge`, `Time` with
  `Time.format`, refs as the id text), `al-pagination` with `prev`/`next`
  links in the slots and "Page N of M" in the default slot; a test that
  inserts rows and counts `al-table tbody tr`.
- `--show T`: `page "<path>/:id"` with `{ id: T.<Id> }`, `load` via
  `Db.get` + `Query.notFound`, an `al-card` listing every column; a test
  for the row and one for `PageMissing`.
- `--from T …` (one or more): the model gets one `List T` per table loaded
  in full, the view one `al-card` per table with a `<ul>`; the agent then
  edits. (Multi-table pages are the common case the per-table shape does
  not cover; the generator gives the loads and the skeleton, not the
  design.)
- `--requires user`: `requires user` on the page; with `--list T` on a
  table with `@owner`, `Db.mine T` instead of `Db.from T`; `--role R` adds
  `, role R` (refused if the session has no such role).
- Refused when the module file exists or the path is taken by another
  page (the checker's own error, surfaced).

### `alder new table <Name> <field>:<Type>[@attr …] …`

- Writes `app/<Name>.alder` with `table <Name> { id: <Name>Id @id, …,
  created: Time @created, updated: Time @updated }` plus the id type, then
  runs the migration generator (`alder migrate`'s machinery) so
  `migrations/` gains the migration. Field syntax: `title:Text.NonEmpty`,
  `done:Bool=False`, `post:Post.PostId@ref=Post@cascade`,
  `owner:User.UserId@ref=User@owner`. Refused when the module exists, when
  a type is unknown (the checker's text), or when the migration generator
  reports a conflict.

### `alder new form <Page module> <Name> <field>:<Type> …`

- In the page module: `form <Name> { … }`, `msg Submit<Name> form <Name>`,
  an `update` arm that inserts when every field maps onto one table's
  columns by name and type (`--into T` names the table; otherwise the arm
  body is a typed hole `?submit<Name>` of type `Task Nav`), the `<form
  submit={Submit<Name>}>` in `view` with one wrapped input per field
  (`al-input`; `al-textarea` for `Text` fields named `body`/`message`/
  `description`; `al-select` for unions; `al-checkbox` for Bool) and an
  `al-alert variant="destructive"` with `<Errors form />`, an `al-button
  type="submit"`; a browser test that submits a valid form
  (`Test.submit`) and asserts the answer, and one `Test.submitRaw` with an
  empty required field asserting `FormRejected`. The form is appended
  where `view` already has an `al-card` (inside its last one) or at the
  end of the page's root element. Refused if the name is taken.

### `alder new msg <Page module> <Name> [<field>:<Type> …]`

- `msg <Name> { … }`, an `update` arm with a typed hole `?<name>` of type
  `Task Nav`, an `<al-button click={<Name> {…}}>` placed at the end of the
  root element (the fields' values are holes `?<name><Field>` in the
  click), a `Test.click` test asserting the answer is `Rerendered` or
  `Redirected` (a hole-typed assertion the agent completes: the test's
  body is written in full with `?expected` where the answer goes).

### `alder new capability <Module> <name> "<Type>"`

- `capability <name> : <Type>` in the module, `fake <Name>Fake { … }` in
  the module's test section returning a typed hole body, the adapter
  package under `adapters/<segment>/` with the exact constructor the book
  specifies (`func <Module><Name>() app.Cap__…`) and a stub body that
  returns an error "not implemented", an install test. Refused if the
  capability exists.

### `alder edit table <T> add-field <field>:<Type>[=default][@attr …]` | `rename-field <old> <new>` | `drop-field <field>`

- Edits the table declaration (`rename-field` writes `@renamed "old"` as
  chapter 8 specifies; `drop-field` removes the field), runs the migration
  generator, then reports every place the change breaks (the checker's
  diagnostics on the copy: inserts missing the new field without a
  default, uses of a dropped field) — those are printed, the edit is
  STILL written (the diagnostics are the agent's to-do list), unless
  `--strict`. `add-field` without a default on a table that is inserted
  somewhere therefore prints the inserts to fix.

### `alder fill <Module>.<decl>[.<arm>] -` and `alder fill --list`

- `--list`: every typed hole in the app with its module, declaration,
  name and expected type (W0002's data), as text or `--json`.
- `fill`: reads an expression from stdin, replaces the named hole (a
  declaration with exactly one hole, or `--hole ?name`), checks the copy,
  writes only if the app checks (the expression may itself contain new
  holes). Refused with the checker's diagnostics otherwise; nothing
  written. This is the one free-text channel E11 allows.

### `alder new admin [--module Admin] [--path /admin]` and `alder regen admin`

- Precondition: the session has `user` and a `role` field whose union has
  `Admin`; otherwise refused: "`alder new admin` needs a session with a
  `role` whose type has an `Admin` constructor: add `role: User.Role =
  Reader` to `session` in `App` and `Admin` to `User.Role`, and set it at
  login (chapter 6)".
- Generates, each `requires user, role Admin`:
  - `app/Admin/Index.alder` (`/admin`): the dashboard: an `al-card` per
    table with `Db.count` and the 5 most recent rows (by `@created`, else
    by id) linking to their show page; `al-stack`.
  - Per table `T` (module `Admin.<T>`): `Index` (`/admin/<t>`: `al-table`
    with every column, `q` search over every `Text`-typed column with
    `.contains`, `sort` by any column with `dir`, pagination 25/page with
    `Db.count`, badges for Bool/unions, refs as links to the referenced
    row's admin show page, a "New" `al-button href`), `Show`
    (`/admin/<t>/:id`: every column; `@created`/`@updated` read-only;
    Edit and Delete buttons), `New` (`/admin/<t>/new`) and `Edit`
    (`/admin/<t>/:id/edit`): typed forms from the columns with `al-select`
    for unions AND for refs (options = the referenced table's rows, label
    = its first `Text`-typed column or the id), `al-checkbox` for Bool,
    `al-textarea` for `Text` columns named `body`/`message`/`description`
    /`content`, `@owner` columns as a select of users; brand validation
    for free; `Delete` (`/admin/<t>/:id/delete`): a confirmation page that
    lists, per table with `@ref T @cascade`, how many rows go with it,
    and refuses (shows the count, no button) when a `@restrict` ref
    points at the row; the message deletes and redirects to the list.
  - Tests per table: list shows inserted rows; search filters; sort
    reverses; pagination; show; create; edit; delete; cascade count;
    restrict refusal; a `Reader` gets `PageMissing`; a visitor is
    `Redirected`. The generated tests must kill the generated code's
    mutants (the fixture's `--mutate` score ≥ 0.95 with the survivors
    documented as equivalent).
  - A `Layout.Admin` component (`app/Admin/Layout.alder`): `al-page` with
    a nav of every table.
- **Owned regions**: the generator writes each file whole the first time;
  every later `alder regen admin` rewrites only the regions between
  `-- @admin begin <name>` and `-- @admin end` (columns of the table, the
  form fields, the search/sort columns, the nav, the dashboard cards)
  and leaves everything else, including tests the author added, alone; a
  file the author deleted is not recreated unless `--all`. `regen` is
  idempotent (a second run is a no-op) and refuses when a region marker
  is missing its pair.
- The admin needs `<Select>` with dynamic options (`options={list}` of
  `(Id t, Text)` pairs) — added to the form machinery and `al-select` in
  this milestone if absent (the implementer confirms).

## Implementation

- New package `internal/generate`: `Program` facts from the IR (tables,
  columns with types/attrs/refs/cascade, pages, paths, session, roles,
  modules), templates (`text/template` files embedded; one per generated
  unit), the region machinery (`Regions.Replace(src, name, body)`), the
  copy-check-write pipeline (`Apply(app, edits) (Result, error)`), field
  syntax parsing, the hole index. `cmd/alder/new.go` grows the subcommands
  (`alder new <app>` keeps working as today); `edit.go`, `fill.go`,
  `regen.go`.
- Every generated file is formatted as the book's examples are (two-space
  indentation, the import order the examples use); the generated tests use
  the stdlib's `Test.*` as chapter 10 shows.
- `--json` is the contract for E11 and for the Olifant pack.

## Examples and book

- **blog**: the author's pages `Admin`/`Admin.Edit` are renamed `Write`/
  `Write.Edit` (`/write`, `/write/:id`); `User.Role` gains `Admin`;
  `alder new admin` generates the site admin for `Post`, `User`; its
  generated tests are kept verbatim (the showcase) and the examples'
  overall `--mutate` score stays ≥ 0.95; a committed `README` section
  shows the exact commands that produced the admin, replayable.
- **todos**: `role: User.Role = Member` with `Member | Admin` added to the
  session (set at login from the user row), `alder new admin` for
  `Todo`, `User`; score ≥ 0.95 overall (the todos' own 1.00 kept for its
  non-admin mutants: report both).
- **scaffold**: `alder new <app>` unchanged; the README it writes shows
  `alder new admin` as the next step and what it needs.
- **Book**: a new chapter `book/15-generators.md` "Generators and the
  admin": the architecture written down (users + tables + pages + forms/
  msgs + capabilities + rules), every command with a verified transcript
  (the book test runs each command on a scratch copy of the scaffold and
  compares the output and the written file with the chapter's blocks),
  the admin tour (the blog's generated pages quoted by `part=`), owned
  regions and `regen`, holes and `fill`, refuse-not-overwrite, `--json`,
  "AI only in small, typed, checked gaps"; chapter 11 (the commands'
  reference rows), chapter 12 (why generators in the compiler; why plain
  files; why regions only for the admin; why refuse), chapter 13 (the
  copy-check-write pipeline; regions), chapter 14 (the blog's `Write`
  rename), README; the reference gains a compact command table (≤ 46k).

## Testing

- `internal/generate`: unit tests for field parsing, regions (replace,
  idempotent, missing pair), the hole index.
- `cmd/alder`: a test per command on scratch apps (the scaffold and a
  fixture with refs/cascade/restrict/unions/owner): the written files
  equal goldens; the app checks and its tests pass after each command;
  refusals' texts; `--json` shape; `--dry-run` writes nothing; `fill` with
  an expression that does not check writes nothing.
- The admin fixture: every generated page under `alder test` and
  `--mutate` (score ≥ 0.95, survivors listed); `regen` after `alder edit
  table add-field` updates the table's regions and keeps an added test.
- Examples' gates as always; the book's transcripts verified.

## Non-goals

- Page-level edits beyond `new form`/`new msg` (`alder edit page`); a
  dialog for delete confirmation (M7b); file uploads; the admin's
  per-row bulk actions; a permissions model finer than `role Admin`;
  undo; generators for components or layouts; E11 itself (the next
  experiment); the Olifant pack (a follow-up in Olifant's repository).
