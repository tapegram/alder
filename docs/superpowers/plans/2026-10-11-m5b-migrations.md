# M5b: Migrations — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Schema changes keep data. `alder migrate` diffs the declared `table`s against a committed snapshot and writes a numbered SQL migration (naming any step that drops data); renames are declared with `@renamed`; `alder check` refuses an app whose declarations are ahead of its snapshot; the built binary (and `alder test`, `alder eval --run`) applies pending migrations at start, records them, and refuses to start on a failed migration or a drifted database. Both examples get a `migrations/` directory. Apps without one keep today's behaviour.

**Architecture:** Three layers. (1) `internal/migrate`, pure: a canonical `Schema` built from the checked program, its JSON snapshot, a deterministic `Diff` into steps, SQLite SQL per step (including the documented table rebuild), a `DropsData` mark, and the rendered file; the parser/IR gain `@renamed` and the literal-default rule. (2) Runtime and build: `rt/db.Migrate` applies files in order inside transactions, records them in `_alder_migrations`, verifies the schema hash and detects drift; `buildkit` copies `migrations/` into the build and `gen` embeds it as `Migrations`; `Serve`, `rtesting.Run` and the eval main use `Migrate` when an app has migrations; `alder check` reports E0019 when the declarations differ from the snapshot. (3) The command, the examples' initial migrations, and chapter 8's "Migrations".

**Tech Stack:** Go stdlib, `rt/db` (SQLite via modernc), `encoding/json`, `embed`.

**Spec:** `docs/superpowers/specs/2026-10-10-m5-blog-migrations-fakes-design.md` — "Migrations", "Error handling", "Testing", "Decisions recorded". Read it first. Also the ledger items from M5a: index `@ref` columns; defaults must be in the schema description for `ADD COLUMN NOT NULL`; a new non-null `@ref` column is a rebuild.

## Global Constraints

- Every generated statement is in the SQLite/libSQL common subset (`CREATE TABLE`, `DROP TABLE`, `ALTER TABLE … ADD COLUMN | DROP COLUMN | RENAME COLUMN | RENAME TO`, `CREATE/DROP INDEX`, `INSERT INTO … SELECT`, `PRAGMA foreign_keys`, `PRAGMA foreign_key_check`). No `CREATE TABLE` text changes for existing tables (no `db.Format` bump; `createStmt` is untouched), so every existing database's hash is unchanged.
- Nothing is generated silently: `alder migrate` writes a file only when run, prints every step, and a step that drops data is marked `DROPS DATA` in the file header and on the terminal. It never rewrites an existing migration file.
- No defaulted record literals (M5 decision). Table field defaults must be literals so a migration can write them (E0022; today's examples already comply).
- Gate: `gofmt -l $(git ls-files '*.go')` empty; `go vet ./...`; `go test ./... -count=1`; both examples check/build/test (todos 3, blog 23); `(cd experiments && python3 -m unittest)` — the harness's generated apps have no `migrations/` and must behave exactly as today; the book verifier green.
- Every new diagnostic (E0019, E0022) has a corpus case and an `alder explain` text; chapter 11's codes table and chapter 8 state the rules.
- Commit messages end with `Co-Authored-By: Claude Fable 5.1 <noreply@anthropic.com>` and `Claude-Session: https://claude.ai/code/session_01VSwqPTccEdvyAkyJ2WtrhG`. Stage explicitly.

## Review Focus

1. **A rename declared with `@renamed`, then the attribute removed after `alder migrate`**: expected — the migration has `RENAME COLUMN`, the snapshot has the new name, and the next `alder check` is clean (no E0019), because the snapshot already knows the new name. Test in Task 1 (`TestDiff_RenameThenForget`) and Task 3 (CLI, end to end with data kept).
2. **A type change on a column with rows** (`Text.NonEmpty` → `Text` keeps values; `Int` → `Text` converts; `Text` → `Int` cannot keep every value): expected — the first two are rebuilds that keep every row; the third is a rebuild marked `DROPS DATA` whose `INSERT … SELECT` uses `CAST` and the file header says non-numeric text becomes 0 — never a silent loss. Test in Task 1 (`TestSQL_RebuildConversions`) and Task 2 (`rt/db` applies it with rows).
3. **The binary started with a migration file missing from the middle** (someone deleted `0002`): expected — refuse to start: "migration 0002_add_comment is recorded as applied but its file is missing"; never apply `0003` over a gap. Test in Task 2 (`TestMigrate_GapRefused`).
4. **A dev database created by `Ensure` before migrations existed** (today's `dev.db`), then the app gains `migrations/0001_initial.sql`: expected — in dev mode the database is recreated once (tables already exist but no `_alder_migrations`: with `ALDER_DEV=1` recreate and record; without it refuse with "this database was not created by migrations: run in dev mode once, or back it up and delete it"). Test in Task 2 (`TestMigrate_AdoptsEnsuredDB`).
5. **`alder migrate` run twice without a change**: expected — "no changes since 0003", exit 0, nothing written; and `--dry-run` with a change writes nothing and prints the file. Test in Task 3 (`TestMigrate_NoChange`, `TestMigrate_DryRun`).

---

### Task 1: `internal/migrate` — schema, snapshot, diff, SQL, file; `@renamed`; literal defaults

**Files:**
- Create: `internal/migrate/schema.go` (`Schema`, `Table`, `Column`, `FromProgram`, JSON), `internal/migrate/diff.go` (`Diff`, `Step`), `internal/migrate/sqlite.go` (`SQL`, the rebuild), `internal/migrate/render.go` (`Render`, `Parse` of a file header), `internal/migrate/migrate_test.go`, `internal/migrate/testdata/` (schema pairs → expected steps/SQL as goldens)
- Modify: `internal/syntax/ast.go` (`TableDecl.Attrs []*Attr`), `internal/syntax/parser.go` (`table Post @renamed "Article" {`; field `@renamed "old"` takes a string), `internal/syntax/parser_test.go`
- Modify: `internal/check/decls.go` (E0022: a table field default must be a literal: `True`/`False`, an integer, a float, a string, a payload-less constructor, `Nothing`; `@renamed`'s string must be an identifier; a `@renamed` naming the field's own name is E0011), `internal/check/check_test.go`
- Modify: `internal/ir/ir.go` (`Table.RenamedFrom string`, `FieldT.RenamedFrom string`, `FieldT.DefaultLit *string` — the SQL literal of the default when the field has one), `internal/ir/lower.go`
- Create: `testdata/corpus/E0022-table-default-not-literal/`, `E0011-renamed-self/`, `renamed-attrs-ok/` (clean), `internal/diag` explain text for E0022

**Interfaces:**
```go
package migrate

// Schema is the canonical description of an app's tables: what migrations are diffed over.
type Schema struct {
    Format string  `json:"format"` // db.Format
    Tables []Table `json:"tables"` // sorted by Name
}
type Table struct {
    Name        string   `json:"name"`         // "Post.Post" (module-qualified, as db.TableDesc.Name)
    ID, Created, Updated string
    Columns     []Column `json:"columns"`      // declaration order
    RenamedFrom string   `json:"-"`            // from @renamed; never stored
}
type Column struct {
    Name     string      `json:"name"`
    Type     db.ColType  `json:"type"`
    Nullable bool        `json:"nullable,omitempty"`
    Enum     []string    `json:"enum,omitempty"`
    Unique   bool        `json:"unique,omitempty"`
    Ref      string      `json:"ref,omitempty"`
    OnDelete db.OnDelete `json:"onDelete,omitempty"`
    Default  *string     `json:"default,omitempty"` // SQL literal: "0", "'Reader'", "NULL"
    RenamedFrom string   `json:"-"`
}
// FromProgram builds the Schema of a checked app (module order then declaration order, then sorted).
func FromProgram(p *ir.Program) Schema
// Hash is Ensure's hash of the schema (the sorted CREATE TABLE statements), so a snapshot and a live database can be compared.
func (s Schema) Hash() (string, error)
func ReadSnapshot(path string) (Schema, error)
func WriteSnapshot(path string, s Schema) error

type StepKind int // CreateTable, DropTable, RenameTable, AddColumn, DropColumn, RenameColumn, AddIndex, DropIndex, Rebuild
type Step struct {
    Kind      StepKind
    Table     string   // SQL table name
    Column    string   // for column steps
    NewName   string   // renames
    Reason    string   // Rebuild: "Post.body: Text.NonEmpty -> Text", "Comment.post: delete rule"
    DropsData string   // "" or what is lost: "column Post.summary", "table Draft", "Post.count: non-numeric text becomes 0"
}
// Diff plans the steps from old to new. Renames come from RenamedFrom; a column with
// RenamedFrom that does not exist in old is an error. Deterministic order: drops of tables,
// renames, creates, per-table column steps in declaration order, rebuilds, indexes.
func Diff(old, new Schema) ([]Step, error)
// SQL renders one step for SQLite; a Rebuild is the documented procedure (create T__new, INSERT … SELECT with conversions, DROP, RENAME, recreate indexes).
func SQL(step Step, old, new Schema) ([]string, error)

type Migration struct {
    Name     string   // "0003_add_comment"
    FromHash, ToHash string
    Steps    []Step
    SQL      []string
}
func Plan(old, new Schema, name string) (Migration, error)   // Diff + SQL + hashes
func Render(m Migration) string                              // the file: header comment + statements
func ParseHeader(text string) (name, fromHash, toHash string, dropsData []string, err error)
func NextName(dir string, slug string) (string, error)       // "0004_<slug>" from the files present
```

**Design (binding):**
- Default literals: the IR lowering turns a literal default into its SQL text (`True`→`1`, `False`→`0`, ints/floats as written, text `'…'` with `'` doubled, a constructor `'Reader'`, `Nothing`→`NULL`, `Just x`→x's literal). A table field whose default is not a literal → E0022 "a table field's default must be a literal (migrations write it into the database): got an expression". Session and page-parameter defaults are unaffected.
- Type comparison is on `db.ColType` + Nullable + Enum + Unique + Ref + OnDelete; a brand change over the same base (`Text.NonEmpty`→`Text`) is NOT a schema change (same `CText`) — note that in the file header? No: nothing to do. Changes of `Enum` members: adding a constructor is a rebuild (the CHECK constraint changes) with no data loss; removing one → `DROPS DATA` ("rows with `Archived` are deleted") — the rebuild's `INSERT … SELECT … WHERE col IN (…)`.
- Rebuild = SQLite's procedure: `CREATE TABLE "T__new" (…)` (the new `createStmt`), `INSERT INTO "T__new" (cols) SELECT <exprs> FROM "T"`, `DROP TABLE "T"`, `ALTER TABLE "T__new" RENAME TO "T"`, recreate unique indexes; executed by the runtime with foreign keys off and `PRAGMA foreign_key_check` after (Task 2). Conversions: `Int`↔`Float` numeric cast; `Int`/`Float`→`Text` `CAST(x AS TEXT)`; `Text`→`Int` `CAST(x AS INTEGER)` marked `DROPS DATA`; `Bool`→`Int` identity; nullable→non-nullable: `COALESCE(x, <default>)` when the new column has a default, else `DROPS DATA` ("rows with NULL are deleted") with a `WHERE x IS NOT NULL`; a new non-null column with a default: `ADD COLUMN … NOT NULL DEFAULT lit` (no rebuild); a new non-null column without a default: `DROPS DATA`? No — it cannot be filled: Diff returns an error "Post.count: a new non-null column needs a default (`= 0`) so existing rows can be filled" (E0022-shaped, reported by `alder migrate`); a new non-null `@ref` column: a rebuild (SQLite forbids `ADD COLUMN … REFERENCES` with a non-NULL default) and needs a default — which cannot exist for a reference → the same error: "add it as `Maybe` first". Unique indexes: `CREATE UNIQUE INDEX "T__col" ON "T"("col")` — wait: today `UNIQUE` is inline in `createStmt`; adding `@unique` to an existing column is a rebuild (inline constraint) — keep it inline (no `createStmt` change); so `@unique` changes are rebuilds; `AddIndex/DropIndex` are for the M5a ledger item "index `@ref` columns": `CREATE INDEX IF NOT EXISTS "T__col" ON "T"("col")` for every `@ref` column — in `0001_initial` for new snapshots and as a step when a `@ref` appears. (`createStmt` stays untouched: indexes are separate statements, also created by `Ensure` for apps without migrations — Task 2 adds that to `Ensure`; they don't enter the hash.)
- File format: first line `-- alder migration <name>`, then `-- from <hash>` / `-- to <hash>` (12 hex chars each), `-- step: <human line>` per step, `-- DROPS DATA: <what>` per such step, a blank line, then statements one per line ending in `;`. No `BEGIN`/`COMMIT` (the runtime wraps). `Render`/`ParseHeader` round-trip.
- Determinism: `Diff` output is a pure function of (old, new); the goldens pin it.

- [ ] **Step 1 (RED):** table-driven tests over schema pairs (every step kind; the rebuild cases above; renames incl. table rename; the `RenamedFrom` of a missing column → error; the new-non-null-without-default error; enum add/remove; `@unique` add → rebuild; `@ref` add → index + (if non-null) the error; determinism: `Diff(a,b)` twice equal); `Render`/`ParseHeader` round-trip; `NextName`; the PROPERTY test: for every pair, apply `old`'s create statements to a `:memory:` SQLite, insert a few rows, apply `SQL` of every step in order (foreign keys off), and assert `migrate.Schema` read back from `sqlite_master`… simpler: assert the live `CREATE TABLE` texts (from `sqlite_master`) equal `new`'s `createStmt`s — which is what `Hash` compares — and that row counts match except where `DropsData` says otherwise. Parser/checker/IR tests for `@renamed`, E0022, `DefaultLit`.
- [ ] **Step 2:** implement. **Step 3 (GREEN)** + corpus + explain text + chapter 8 gets only the `@renamed` and literal-default rules (the full "Migrations" section is Task 3's).
- [ ] **Step 4:** commit.

---

### Task 2: Runtime application, build embedding, the check-time rule

**Files:**
- Modify: `rt/db/sqlite.go` (`Migrate`, `_alder_migrations`, drift, index creation in `Ensure`), `rt/db/store.go`? (`Migration` type), `rt/db/db_test.go`
- Modify: `internal/buildkit/buildkit.go` (`CopyMigrations(src, dst)`, `HasMigrations(dir)`, `SchemaDrift(dir, prog) (*diag.Diagnostic)`), `internal/buildkit/buildkit_test.go`
- Modify: `internal/gen/gen.go`, `internal/gen/caps.go` (`gen.Options{Migrations bool}`; `app__alder.go` gains `//go:embed migrations/*.sql` + `var migrationsFS embed.FS` + `var Migrations = db.ReadMigrations(migrationsFS)` when set, else `var Migrations []db.Migration`; `Serve` calls `store.Migrate(ctx, Tables, Migrations, o.Dev)` when `len(Migrations) > 0`, else `Ensure`), goldens (a new golden app with migrations; existing goldens unchanged)
- Modify: `rt/testing/testing.go` (`Options.Migrations`; `runCase` applies them to the fresh store and fails every test with "the migrations do not produce the app's schema: run `alder migrate`" when the hash differs), `cmd/alder/scratchmain.go` (the Query/Task mains likewise), `cmd/alder/test.go` (pass `app.Migrations`)
- Modify: `cmd/alder/check.go`, `build.go`, `run.go`, `test.go`, `eval.go` → one helper `checkedApp(dir)` that appends `buildkit.SchemaDrift` (E0019) to the diagnostics; `internal/scratch/scratch.go` (`Check` likewise, so `test`/`eval`/`repl` refuse too)
- Create: `testdata/corpus/E0019-schema-changed/` (an app dir WITH a `migrations/schema.json` behind its tables — the corpus runner must copy the whole dir, confirm it does), `internal/diag` explain text for E0019

**Interfaces:**
```go
package db
type Migration struct{ Name, SQL string }           // Name "0003_add_comment"; SQL the file's statements (header stripped by ReadMigrations)
func ReadMigrations(fsys fs.FS) []Migration           // migrations/*.sql in name order (panics on a malformed file: a build-time bug)
// Migrate brings the database to the app's schema by applying every migration in ms not yet
// recorded in _alder_migrations, in order, each in its own transaction with foreign keys off
// and PRAGMA foreign_key_check after; then it checks that the recorded schema hash equals the
// app's (Ensure's recipe) and records it. Refusals (an error, nothing changed): a recorded
// migration whose file is missing (a gap); a database with app tables but no _alder_migrations
// (not created by migrations) unless dev, which recreates it; a hash mismatch after applying
// (drift: the database was changed by hand) unless dev, which recreates it. A failed statement
// rolls its migration back and returns "migration <name> failed: <sqlite error>".
func (s *SQLite) Migrate(ctx context.Context, tables []TableDesc, ms []Migration, dev bool) (applied []string, err error)
```
```go
package buildkit
func HasMigrations(dir string) bool                          // dir/migrations/schema.json exists
func CopyMigrations(src, dst string) error                   // dir/migrations/*.sql → build dir (nothing when absent)
// SchemaDrift compares the checked program's schema with dir/migrations/schema.json; nil when
// absent or equal; else an E0019 at migrations/schema.json:1:1 naming the first changed table.
func SchemaDrift(dir string, p *ir.Program) *diag.Diagnostic
```

**Design (binding):**
- `Migrate`'s dev recreate reuses `Ensure`'s drop-and-create path and then records every migration in `ms` as applied (the fresh tables ARE the end state) plus the hash — so a dev database stays usable without replaying.
- `Serve` logs `migrated 0002_add_comment` per applied file via `o.Log`.
- `Ensure` (apps without migrations) also creates the `@ref` indexes (idempotent `CREATE INDEX IF NOT EXISTS`) — hash unaffected.
- E0019 text: "the schema changed since migration 0003 (table Post: column body): run `alder migrate`"; file `migrations/schema.json`, line 1. `alder check --json` carries it like any diagnostic.
- The gen golden: `testdata/golden/gen_migrations/` with a `migrations/` dir and `want/app__alder.go` showing the embed; every other golden unchanged (no migrations → the plain `var Migrations []db.Migration`, which IS a one-line change in every `app__alder.go` golden — acceptable and explained; inspect).

- [ ] **Step 1 (RED):** `rt/db`: `TestMigrate_AppliesInOrder` (rows kept across an ADD COLUMN and a rebuild), `TestMigrate_RecordsAndSkips`, `TestMigrate_FailureRollsBack`, `TestMigrate_GapRefused`, `TestMigrate_AdoptsEnsuredDB` (dev recreates; prod refuses with the text), `TestMigrate_DriftRefused` (hand `ALTER TABLE` after migrating → prod refuses naming the table; dev recreates), `TestEnsure_RefIndexes`; `buildkit`: `SchemaDrift` nil/absent/equal/changed; gen golden; `rtesting`: a case with migrations whose hash mismatches → every test fails with the text; `cmd/alder`: `TestCheck_E0019` on a temp app.
- [ ] **Step 2:** implement; **Step 3 (GREEN)**; full gate (the harness's apps: no `migrations/` → unchanged outputs; E5–E7 `scores.md` byte-identical).
- [ ] **Step 4:** commit.

---

### Task 3: `alder migrate`, the examples' migrations, chapter 8 "Migrations"

**Files:**
- Create: `cmd/alder/migrate.go`, `cmd/alder/migrate_test.go`; `examples/todos/migrations/{schema.json,0001_initial.sql}`, `examples/blog/migrations/{schema.json,0001_initial.sql}`
- Modify: `cmd/alder/main.go` (dispatch + usage), `book/08-database.md` ("Migrations" replaces "Schema changes"), `book/11-tooling.md` (`alder migrate`; E0019/E0022 rows; the dev-mode note), `book/13-compiler.md` §8 ("Migrations": how `Diff` plans, how `Migrate` applies, the embed), `book/14-second-app.md` ("Run it": the migrations dir), `book/12-rationale.md` (decisions: generated + versioned; `@renamed`; `DROPS DATA`; E0019 — from the spec), `book/README.md` status, `docs/superpowers/specs/...` nothing (the spec is the design)

**Design (binding):**
- `alder migrate [dir] [--name slug] [--dry-run]`: check the app (diagnostics → exit 1; E0019 is NOT reported by `migrate` itself — it is what fixes it); `old := ReadSnapshot` (absent → `0001_initial` from an empty schema, creating the directory); `new := FromProgram`; `Plan`; no steps → "no changes since <last>", exit 0; else print each step (`DROPS DATA` lines in red? no colour: prefix `!! `) and, unless `--dry-run`, write `NNNN_<slug>.sql` (slug from `--name` or derived: `add_<col>`, `create_<table>`, `rename_<x>`, `change_<table>`, joined by `_`, ≤ 40 chars) and the new `schema.json`; exit 0. A snapshot whose hash doesn't match the last file's `to` hash → "migrations/schema.json does not match 0003_x.sql's result; restore the snapshot from git or regenerate the migration", exit 1. `--json`? No.
- The examples: run `alder migrate examples/todos` and `examples/blog` to create their `0001_initial`; `alder test` on both still passes (now through migrations); `alder run` on a fresh dir creates the dev database by migration; an existing `build/todos/dev.db` from `Ensure` adopts in dev mode (Review Focus 4).
- Chapter 8 "Migrations" (beginner-level, ≤ 150 prose lines + blocks): why (data survives), the workflow (change a `table` → `alder check` says E0019 → `alder migrate` → read the file → commit), a worked change on a copy of the blog (add `Post.summary: Maybe Text`, then rename it with `@renamed`, then drop it: three generated files quoted as ```` ```text ```` captured from the real command, the `DROPS DATA` line shown), the rules (literal defaults; a new non-null column needs a default; `@renamed`; what is a rebuild; `@unique`/enum changes; indexes on `@ref`), what the binary does at start (apply, record, verify; the refusals and their messages verbatim), dev mode (adopt/recreate), tests run through migrations, drift. Chapter 11: the command and E0019/E0022. Chapter 13 §8: `Diff`'s order, the rebuild procedure, `Migrate`'s transaction and pragma handling, the embed.
- CLI tests (`migrate_test.go`, temp copies of the blog): init; add a nullable column (ADD COLUMN); add a non-null column with a default (ADD COLUMN … DEFAULT); rename with `@renamed` then remove the attribute (clean check after); drop a column (`DROPS DATA` in file and output); a type change rebuild with rows kept (apply via `alder test`? better: build the binary, start it on a dev.db with rows inserted at the previous version, restart after migrating, assert rows via `alder eval --run`); no change; dry-run; E0019 from `alder check`/`build`/`test`/`eval` on the changed-but-unmigrated app; a snapshot/file mismatch; the slug derivation.

- [ ] **Step 1 (RED):** the CLI tests. **Step 2:** `migrate.go`, dispatch, the examples' initial migrations. **Step 3 (GREEN):** gate incl. both examples. **Step 4:** the book (every transcript captured from the binary; the verifier green). **Step 5:** commit (two commits: code + examples; book).

---

## Self-review notes

- Spec coverage: files and snapshot (T1), `alder migrate` (T3), diff and SQL incl. rebuild (T1), renames declared (T1), check-time rule E0019 (T2), application at start + `_alder_migrations` + drift + dev recreate (T2), tests via migrations (T2), chapter 8/11/13 (T3), both examples (T3). The M5a ledger: `@ref` indexes (T1 steps + T2 `Ensure`), defaults in the schema (T1 `DefaultLit`, E0022), non-null `@ref` ADD COLUMN = error/rebuild (T1).
- Interfaces: T2's `Migrate` consumes T1's file format via `ReadMigrations` (header stripping must match `Render`); T3's command consumes T1's `Plan/Render/NextName` and T2's `SchemaDrift` is what E0019 is.
- Review Focus 1–5 each have a named test.
- Not in M5b: data migrations in Alder; zero-downtime; a libSQL store.
