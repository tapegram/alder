# Default App Layout (#20) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Tables live in `app/Table/<Name>.alder` (module `Table.<Name>`, written `<Name>` at use sites), page folders are plural concern folders, and generators, scaffold, examples and the book follow that layout. These are conventions only, with no config file.

**Architecture:** The checker's table rule relaxes from "module == table name" to "module's last segment == table name", and table names become unique app-wide. Table identity stays `Module.Name`, so existing apps are unchanged. The migration diff pairs a table that moved between modules by its bare name, so it becomes a rename and no data is dropped. Generators look tables up by bare name and import the table's real module. `new table`/`new auth`/scaffold write into `app/Table/`. Admin folders become plural, and `alder new page --list/--show/--from T` can derive the module.

**Tech Stack:** Go (internal/check, internal/names, internal/migrate, internal/generate, internal/scaffold, cmd/alder), Markdown book, Lua (alder.nvim, verify only).

**Spec:** the decision comments on tapegram/alder#20 (`gh issue view 20 --comments`): the 2026-10-22 decisions, the "Revised: all tables in a top-level `Table` folder" comment, and the proposal's open-question recommendations. Rulings from this session (2026-10-24):
- **R1 Table rule:** a table's module must *end* in the table's name (`Todo`, `Table.Todo`, `Table.Billing.Invoice`). One table per module stays. Subfolders of `Table/` group related tables. Root tables (`app/Todo.alder`) remain legal.
- **R2 Identity:** a table's key stays `Module.Name` (SQL `Table__Todo__Todo`). A move is paired by bare name in the migration diff, with no `@renamed` needed.
- **R3 Old admin:** `regen admin` on an admin in the old singular layout refuses with a hint (delete `Admin/`, rerun `alder new admin`). It does not migrate.

## Global Constraints

- Conventions only: no `alder.toml`, no `[layout]` config.
- The root holds `App`, `Layout`, `Home` (and `Robots`). Pages live in plural concern folders (`Todos/Index`). Capability module is `<Concern>.Data`. Shared views go in `Ui/`.
- All tables go in `app/Table/`, module `Table.<Name>`. Use sites write `<Name>` after `import Table.<Name>` (#23's last-segment qualifier).
- Plural of table `T` for folders/modules: `UpperFirst(Plural(LowerFirst(T)))`: `Todo`→`Todos`, `Activity`→`Activities`, `LineItem`→`LineItems`.
- Admin URLs are unchanged (`/admin/todo`). Only folders and modules go plural.
- Every generator still honours `--dir`, `--json`, `--dry-run`.
- New diagnostic code: `E0037` (`DuplicateTable`), "two tables named X".
- After merge: `go install ./cmd/alder`. The user runs `:LspRestart`.

## Review Focus

1. **Moving a table with data:** `Todo.Todo` → `Table.Todo.Todo` in an app with rows must migrate as a rename (rows kept). Other tables' `@ref` columns pointing at it must follow (Task 2 test `TestDiffMoveTablePairsByName`).
2. **Existing root-table apps:** every `testdata/golden/gen_*` and `check_*` golden stays byte-identical except E0018/E0202 message text (Task 1 step 5).
3. **`alder new table Todo` when a root module `Todo` already exists:** refuse with a hint instead of creating a `Table.Todo` that clashes on the qualifier (Task 3 test `TestNewTableRefusesRootModule`).
4. **One positional to `new page`:** `alder new page /todos --list Todo` must not read `/todos` as the module. `alder new page Todos.Index` with no path is still an arity error (Task 5 tests).
5. **`regen admin` on the old layout:** writes nothing and exits non-zero with the hint (Task 4 test `TestRegenAdminRefusesSingularLayout`).

---

### Task 1: Checker accepts tables in `Table.*` (model: Opus)

**Files:**
- Modify: `internal/check/decls.go:396-400` (defineTable E0018)
- Modify: `internal/check/partitions.go:44-54` (`appTable`)
- Modify: `internal/names/resolve.go:~1005-1015` (E0202 condition/text)
- Modify: `internal/diag/diag.go` (add `DuplicateTable = "E0037"`), `internal/diag/explain.go` (E0018 text, E0037 entry)
- Test: `internal/check/table_layout_test.go` (new), `cmd/alder/layout_test.go` (end-to-end build)

**Interfaces:**
- Produces: `(*Checker).appTable(name string) string` returns the key (`"Table.Todo.Todo"`) of the unique non-fake app table whose `Name == name`, or `""`. Every existing caller (`@ref`, session `via`, partitions :162/:246) keeps working with the bare name.
- Produces: E0018 fires iff the module's last segment != table name (fakes still exempt). New message: ``a table is declared in a module named after it: `table Comment` belongs in `app/Table/Comment.alder` ``.
- Produces: E0037 on the second declaration of a table name already declared by another app module: ``two tables named Invoice: `Table.Invoice` and `Table.Billing.Invoice`; table names are app-wide``.

- [ ] **Step 1: Write failing tests** in `internal/check/table_layout_test.go`, using the same in-memory app helper the neighbouring check tests use:
  - `TestTableInTableFolder`: `app/Table/Todo.alder` = `module Table.Todo` + `type TodoId = Text` brand + `table Todo { id : TodoId @id, title : Text }`. `app/Home.alder` page imports `Table.Todo` and uses `Db.from Todo`, `Todo.title`. Assert no diagnostics.
  - `TestTableNested`: `Table.Billing.Invoice` with `table Invoice`, and `Table.Billing.LineItem` with `invoice : InvoiceId @ref=Invoice`. No diagnostics.
  - `TestTableWrongLastSegment`: `module Table.Todos` declaring `table Todo` gives exactly one E0018 whose message contains `app/Table/Todo.alder`.
  - `TestTableRootStillLegal`: `module Todo` + `table Todo` gives no diagnostics.
  - `TestDuplicateTableName`: `Table.Invoice` and `Table.Billing.Invoice` both `table Invoice` gives one E0037.
  - `TestSessionViaTableFolder`: `App` session `via User` with `app/Table/User.alder` gives no diagnostics.
- [ ] **Step 2:** `go test ./internal/check -run 'Table|Session' -v` should FAIL (E0018 on the Table.* cases).
- [ ] **Step 3: Implement.** E0018: compare `d.Name` with the last segment of `mc.m.Name`. `appTable`: scan `c.tables` for a non-fake key whose table name is `name`. Precompute a `map[name]key` once if `c.tables` is large. E0037: detect in the same pass that builds tables; report at the later module in sorted module order, so it is deterministic. In `names/resolve.go`, the "belongs to module" E0202 must compare the last segment, not the module, and its text should name `app/Table/<T>.alder`. Update `explain.go` E0018 and add E0037.
- [ ] **Step 4: End-to-end:** add `TestBuildTableFolder` in `cmd/alder/layout_test.go`. It copies the layout of `examples/todos` in miniature (one table in `app/Table/Todo.alder`, one page inserting and listing), then runs `alder check` and `alder build`, and asserts success. Use the helpers that file already uses.
- [ ] **Step 5:** `go test ./internal/check ./internal/names ./internal/diag ./internal/gen ./internal/ir ./cmd/alder -run 'Table|Layout|Golden|Check'` should PASS. If a `check_*` golden changes, only E0018/E0202 wording may differ. Rerun with `-update`, and `git diff testdata` must show message lines only.
- [ ] **Step 6: Commit** `feat(check): a table's module ends in its name; E0037 duplicate table names (#20)`

### Task 2: Migration diff pairs a moved table by name (model: Sonnet)

**Files:**
- Modify: `internal/migrate/diff.go:108-141` (`pairTables`)
- Test: `internal/migrate/migrate_test.go` (or a new `move_test.go` beside it)

**Interfaces:**
- Consumes: Schema table names are keys like `"Todo.Todo"`. `short(name)` gives the bare name.
- Produces: after the by-name and `@renamed` passes, a new table that is still unpaired is paired with the unique unclaimed old table having the same `short` name. That pairing counts as a rename in `p.renamed`. Ambiguity (two unclaimed old tables with that short name) means no pairing, which is the current behaviour.

- [ ] **Step 1: Write failing tests:**
  - `TestDiffMoveTablePairsByName`: old schema `Todo.Todo` (cols id, title) plus `User.User`, with `Todo.owner` `ref: "User.User"`. New schema `Table.Todo.Todo`, `Table.User.User`, with ref `"Table.User.User"`. The generated SQL renames both tables (`ALTER TABLE "Todo__Todo" RENAME TO "Table__Todo__Todo"` or whatever form the existing rename path emits; copy it from an existing `@renamed` test). It contains no `DROP TABLE` and no `CREATE TABLE` for them.
  - `TestDiffMoveTableKeepsRows`: same move, applied through the sqlite path the existing tests use, with a row inserted before. The row survives.
  - `TestDiffMoveAmbiguousNoPair`: old `A.Item` and `B.Item`, new `Table.Item` gives today's output (drop + create, or the existing error). Assert it equals the output without the new pass.
- [ ] **Step 2:** `go test ./internal/migrate -run DiffMove -v` should FAIL.
- [ ] **Step 3: Implement** the third pass in `pairTables`. Check that the ref-following code already maps through `p.renamed`. If refs compare raw names, map them through `p.renamed` too.
- [ ] **Step 4:** `go test ./internal/migrate ./internal/buildkit` should PASS.
- [ ] **Step 5: Commit** `feat(migrate): a table moved to another module is a rename (#20)`

### Task 3: Generators write tables into `app/Table/` (model: Opus)

**Files:**
- Modify: `internal/generate/program.go:391-398` (`TableNamed`), plus a new `ModuleOfTable`
- Modify: `internal/generate/table.go` (file/module, `@ref` imports :80-82, `typeImports` :220, `moduleOfQualifier` :231, `next:` line)
- Modify: `internal/generate/auth.go:56,130-144` (user table into `Table.<Users>`)
- Modify: every `imports.add(t.Module)` caller already passes the module, so check they still do. Also `admin.go`, `page.go`, `silo.go`, `remove.go`, `session_edit.go` callers of `TableNamed` (list in the survey: admin.go ~20 sites, admin_bulk.go 272/338/356, silo.go 52/105/139, auth.go 101/178, remove.go 31, page.go 273, session_edit.go 69).
- Modify: `internal/scaffold/files/app/Entry.alder` → `internal/scaffold/files/app/Table/Entry.alder` (`module Table.Entry`), `internal/scaffold/files/app/Home.alder` (`import Table.Entry`), `internal/scaffold/scaffold.go:77`, `internal/scaffold/files/README.md:24,32`
- Modify: `testdata/generate/fixture/app/{Comment,Post,Tag,User}.alder` → `testdata/generate/fixture/app/Table/*.alder`, with imports across the fixture updated
- Test: `internal/generate/*_test.go`, `cmd/alder/generate_test.go` goldens (`-update`), `cmd/alder/new_test.go`

**Interfaces:**
- Produces: `(*Program).TableNamed(name string) *Table` matches `t.Name == name` (bare table name, the form `--list T` and `@ref=T` use).
- Produces: `TableModule(name string) string` returns `"Table." + name`, the module a *new* table gets.
- Produces: `alder new table T` writes `app/Table/T.alder`, `module Table.T`, and its Result text ends with `next: alder new page /<plural-lower> --list T` (a module-less form, which works after Task 5. Until then the test asserts only the text).

- [ ] **Step 1: Write failing tests:**
  - `TestNewTableWritesTableFolder` (internal/generate): `NewTable(p, "Todo", …)` creates `app/Table/Todo.alder` starting `module Table.Todo`. The result text contains `next: alder new page /todos --list Todo`.
  - `TestNewTableRefImportsModule`: on a program with `Table.User`, `alder new table Todo owner:UserId@ref=User` emits `import Table.User`, not `import User`.
  - `TestNewTableRefusesRootModule`: program with root `module Todo` (types only) gives an error mentioning `app/Todo.alder` and `app/Table/Todo.alder`.
  - `TestTableNamedByName`: `TableNamed("Todo")` finds the table of module `Table.Todo`, and also the root module `Todo` (legacy).
  - `TestNewAuthUserInTableFolder` (auth_test.go): default auth writes `app/Table/User.alder` with `module Table.User`. `Auth/Login.alder` imports `Table.User`.
  - `TestNewAppScaffoldTable` (cmd/alder/new_test.go): `alder new demo` creates `app/Table/Entry.alder`, has no `app/Entry.alder`, and the new app passes `alder check`.
- [ ] **Step 2:** run them and they should FAIL.
- [ ] **Step 3: Implement.** Change `TableNamed` to match by name. Every place that turns a table *name* into an import (`@ref` args, `typeImports`, `moduleOfQualifier`) resolves through `p.TableNamed(name).Module`, falling back to `TableModule(name)` for a table being created in the same command. Write qualifiers through `names` qualify helpers (`qualFor`/`NeedsAlias` in internal/names/qualify.go), never raw module paths. Move the scaffold Entry and update `scaffold.go`'s path special-case. Move the fixture tables and fix fixture imports.
- [ ] **Step 4:** `go test ./internal/generate ./internal/scaffold` passes. Then `go test ./cmd/alder -run 'Generate|New' -update`, and review `git diff testdata/generate`: the only changes should be paths `app/X.alder`→`app/Table/X.alder`, `import X`→`import Table.X`, `module Table.X` and the `next:` line. Rerun without `-update` and it should PASS.
- [ ] **Step 5: Commit** `feat(generate): tables are written to app/Table; lookups by table name (#20)`

### Task 4: Admin uses plural folders; regen refuses the old layout (model: Sonnet)

**Files:**
- Modify: `internal/generate/admin.go:147-151,276,748` and every `g.o.Module + "." + t.Module` / `rt.Module` folder build; `admin_bulk.go`, `admin_silo.go` likewise
- Modify: `internal/generate/admin.go` `RegenAdmin` (~125-250): legacy detection
- Test: `internal/generate/admin_test.go`, `cmd/alder/admin_test.go`, `testdata/generate/want/fixture-admin`, `fixture-silo-admin` (`-update`)

**Interfaces:**
- Consumes: `TableNamed` by name (Task 3). `t.Module` may be `Table.Todo`.
- Produces: `adminTableModule(o AdminOptions, t *Table) string` returns `o.Module + "." + UpperFirst(Plural(LowerFirst(t.Name)))`, e.g. `Admin.Todos`. All admin page modules are `Admin.Todos.Index|Show|New|Edit|Delete|Bulk`.
- Produces: `RegenAdmin` error when, for some table T, `app/<Admin>/<T>/` exists or the layout nav links `<Admin>.<T>.Index`. The message is exactly: ``admin is in the old layout (Admin/Todo/): folders are plural now (Admin/Todos/). Delete app/Admin and run `alder new admin` again; move any code you added outside `-- @admin` regions first`` (folder names substituted, joined with `, ` when several).

- [ ] **Step 1: Write failing tests:**
  - `TestAdminPluralFolders`: `AdminFiles` for tables `Todo` (in `Table.Todo`) and `Activity` yields paths `app/Admin/Todos/Index.alder`… and `app/Admin/Activities/Index.alder`. No path contains `Admin/Table` or `Admin/Todo/`.
  - `TestRegenAdminRefusesSingularLayout` (cmd/alder/admin_test.go): an app with `app/Admin/Todo/Index.alder` from the old generator gives `alder regen admin` non-zero, stderr contains `old layout (Admin/Todo/)`, and no file changed (compare a tree hash before and after).
  - Existing admin tests: update expected paths to plural.
- [ ] **Step 2:** run them and they should FAIL.
- [ ] **Step 3: Implement** `adminTableModule` and use it at every folder build. The nav link check that decides "new table" uses the plural module. Add legacy detection at the start of `RegenAdmin`, before any write.
- [ ] **Step 4:** `go test ./internal/generate ./cmd/alder -run Admin -update`, then review the `git diff` (folder/module renames only), then rerun without `-update` and it should PASS.
- [ ] **Step 5: Commit** `feat(admin): plural folders (Admin/Todos); regen refuses the singular layout (#20)`

### Task 5: `alder new page` derives the module from `--list/--show/--from` (model: Sonnet)

**Files:**
- Modify: `internal/generate/spec.go:178-200` (optional positional before a required one)
- Modify: `internal/generate/specs.go:88` (`module` not Required), `internal/generate/page.go:~130-140` (derive)
- Modify: `cmd/alder/generate.go:240` (usage `alder new page [<Module>] <path> …`), `testdata/help.json` (golden)
- Test: `internal/generate/spec_test.go`, `internal/generate/page_test.go` or `specs_test.go`, `cmd/alder/generate_test.go`

**Interfaces:**
- Produces: in `ParseArgs`, when fewer positionals are given than declared args, non-Required args are skipped from the left, so the Required ones get the values. With `[module?, path]` and one positional, that positional is `path`. With zero positionals, the result is `ErrArity`.
- Produces: `DerivedPageModule(t, kind string) string`: `--list` gives `Ts.Index`, `--show` gives `Ts.Show`, `--from` (first table) gives `Ts.New`, with `Ts` as in Global Constraints. `NewPage` with `Module == ""` uses it. Otherwise it fails with ``a page needs a module: `alder new page <Module> <path>`, or give --list/--show/--from T to name it Ts.Index/Ts.Show/Ts.New``.
- The Result's `wrote app/Todos/Index.alder` line is unchanged (it already names the file).

- [ ] **Step 1: Write failing tests:**
  - `TestParseArgsOptionalLeading` (spec_test.go): spec args `[a optional, b required]`. Argv `["/x"]` gives `b=/x` and `a` unset. `["M", "/x"]` gives both. `[]` gives `ErrArity`. `["M","/x","extra"]` gives `ErrArity`.
  - `TestNewPageDerivesModule`: `--list Todo` gives `app/Todos/Index.alder` declaring `page "/todos"`, module `Todos.Index`. `--show Activity` gives `Activities.Show`. `--from Post Tag` gives `Posts.New`.
  - `TestNewPageNoModuleNoTable`: `alder new page /x --blank` gives the error text above.
  - `TestNewPageMissingPath`: `alder new page Todos.Index` gives an arity error (unchanged).
- [ ] **Step 2:** run them and they should FAIL.
- [ ] **Step 3: Implement**, then update usage and `go test ./cmd/alder -run Help -update` (help.json). Review the diff: `module` becomes `required:false`, and the usage string changes.
- [ ] **Step 4:** `go test ./internal/generate ./cmd/alder -run 'Page|Help|Args'` should PASS.
- [ ] **Step 5: Verify alder.nvim:** `cd ~/projects/alder.nvim && make test` (or the command its README gives) should pass. Then check by reading `lua/alder/form.lua:40` that an empty non-required `module` field is left out of fargs rather than passed as `""`. If it is passed, fix it there with a spec test, and commit in alder.nvim on branch `layout-20`.
- [ ] **Step 6: Commit** `feat(generate): new page derives Ts.Index/Show/New from the table (#20)`

### Task 6: Examples follow the layout (model: Sonnet)

**Files:**
- Move: `examples/todos/app/{Todo,User}.alder`, `examples/blog/app/{Comment,Post,User}.alder`, `examples/workspace/app/{Activity,Membership,Todo,Org,Project,User}.alder` → `examples/*/app/Table/`. Change each `module X` to `module Table.X`, and each `import X` to `import Table.X` across the example (~200 lines).
- Regenerate: `examples/*/app/Admin/` (plural)
- Modify: `examples/*/migrations/` (new rename migration via `alder migrate`), `examples/*/README.md`
- Test: whatever pins examples (`book/book_test.go`, `cmd/alder/*_test.go` that build examples, `internal/lsp` tests reading examples). Find them with `grep -rln 'examples/' --include='*_test.go' .`

- [ ] **Step 1: Snapshot custom admin code:** for each example, run `alder regen admin --dry-run` on the *old* tree (before the move), or diff `Admin/` against a fresh `alder new admin` into a temp copy of main. List any lines outside `-- @admin` regions. Record them in the commit message, and carry them over to the regenerated files.
- [ ] **Step 2: Move tables and fix imports** (`git mv`, then sed `^import X$` → `import Table.X` per table, then `alder fmt` per example to settle aliases). `alder check` passes in each example.
- [ ] **Step 3: Regenerate admin:** delete `app/Admin`, run `alder new admin`, reapply step 1's custom lines, and fix any links to `Admin.<T>.*` in non-admin pages. `alder check` passes.
- [ ] **Step 4: Migrations:** `alder migrate` in each example creates one new migration. Inspect it: it holds only `RENAME` statements, with no DROP and no CREATE of a moved table. Confirm `schema.json` names are `Table.X.X`.
- [ ] **Step 5:** `alder test` in each example, and `go test ./book ./cmd/alder ./internal/lsp`, should PASS. Fix pinned paths in tests to `app/Table/…`.
- [ ] **Step 6: Commit** `refactor(examples): tables in app/Table, plural admin folders (#20)`

### Task 7: The book and docs teach the layout (model: Sonnet)

**Files:**
- Modify: `book/02-tour.md:25-40,404-412,489,970`, `book/08-database.md:43-47`, `book/11-tooling.md:94,139-155,234-281,317,1579`, `book/01-why.md:193-196`, `book/06-pages.md:553,876,976,1129,1174`, `book/10-testing.md:632`, `book/14-second-app.md`, `book/15-generators.md` (check for admin trees and `new table` output), `book/12-rationale.md` (new entry)
- Test: `go test ./book` (snippets and pinned paths)

- [ ] **Step 1: Layout section:** in `book/02-tour.md`, after the scaffold tree, add a short "Where files go" section (the beginner explanation the user asked for in book chapters). It says: the root holds `App`, `Layout`, `Home`. Tables go in `Table/` (`app/Table/Todo.alder`, written `Todo` after `import Table.Todo`, why: tables are global state shared by every concern; one place for the schema). Pages of a concern go in a plural folder (`Todos/Index`, `Todos/Show`). A capability goes in `<Concern>/Data`. Shared views go in `Ui/`. It notes that these are conventions the generators follow, and the compiler requires only that a table's module end in its name.
- [ ] **Step 2: Fix every path and tree** listed above to the new layout. `book/08-database.md:43-47` states R1 (last segment, one table per module, subfolders group related tables, E0018, E0037). `book/11-tooling.md` documents the optional page module, the `next:` line, plural admin folders and the regen refusal.
- [ ] **Step 3: Rationale entry** in `book/12-rationale.md`: why tables go in `Table/`, why the last-segment rule rather than several tables per module (bare names, and the E5–E7 mistake class), and why a move is paired by name.
- [ ] **Step 4:** `go test ./book` should PASS. Run `grep -rn "app/[A-Z][a-z]*\.alder" book/*.md` and check each remaining hit is a root module (App/Layout/Home) or a deliberate legacy mention.
- [ ] **Step 5: Commit** `docs(book): the default app layout (#20)`

### Final

- [ ] Full `go vet ./... && go test ./...` (foreground, timeout 15 min) passes.
- [ ] Whole-branch review (Opus; Fable if the migration pairing diff is larger than ~50 lines, because it is data-critical).
- [ ] After merge: `go install ./cmd/alder`, comment on and close #20, update memory. nightvote-replay has no tables, so it needs no migration.
