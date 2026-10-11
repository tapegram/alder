# Alder M5: the blog example, migrations, and stateful fakes

*2026-10-10. Decided in conversation; this document records the design.
Builds on the M3 spec (`2026-10-09-m3-codegen-runtime-design.md`: the
runtime, `rt/db`, capabilities) and the M4 spec (`2026-10-09-m4-test-repl-
design.md`: `alder test`, `Test.run`, the per-test database). The
measurements behind it: E6/E7 (`experiments/e7-prompt-tests/findings.md`)
and the id probe of 2026-10-10 (Decisions, below).*

## Purpose

Three things the to-do app never needed, each with a real customer in a
second example app:

1. **A second example, the blog** (`examples/blog`): posts with a slug, a
   second table with a foreign key (comments), an author role, pagination,
   search, a second capability (`Markdown`). It is the first app with a
   relation and a role, the floor for the harder-use-case list, and the
   app E8's hidden acceptance tests will run against.
2. **Migrations.** Today a schema change drops every table in dev and
   refuses to start in production. After M5 the compiler generates
   migration files from the `table` declarations, keeps data across
   changes, and the built binary applies pending migrations at start.
3. **Stateful fakes for capabilities.** A test can install an in-memory
   implementation of a whole capability (every entry, with its own tables
   in the per-test database) and drive a lifecycle through it.

Everything stays explicit: no defaulted record literals, no guessed
renames, no automatic generation of files, no hidden state.

## Scope

In: the blog app with its tests; `contains`/`startsWith` conditions on
`Text` columns; `@cascade`/`@restrict`/`@setNull` on references; the
`Markdown` capability and its Go adapter; `alder migrate`, the snapshot,
the migration files, their application at start, the check-time rule; the
`@renamed` attribute; the `fake` declaration and `Test.fake`; chapters 8,
10, 11 and 13 extended, and a new chapter section on migrations; the
verifier covering the new blocks.

Out (ledgered): full-text search (FTS5; after the Turso question settles);
a user-chosen primary key (declined, below); adapter sub-packages per
domain (M5's second plan); zero-downtime/expand-contract migrations (one
binary, one file: a restart is the deploy); data migrations in Alder
(backfills are SQL in the migration file); a libSQL store (after M5,
"Turso-ready" in the M3 spec); roles as a language feature (the blog does
it with a column and a check in `load`, as E7's T6 apps did).

## Architecture

### The blog (`examples/blog`)

Modules: `App` (session `{ user: Maybe User.UserId }`), `Layout`, `User`
(`role: Role = Reader` with `type Role = Reader | Author`), `Auth.Login`
(the `Password` capability, as in the to-do example), `Post`, `Comment`,
`Markdown` (the capability), `Posts.Index` (`/`, paginated, searchable),
`Posts.Show` (`/posts/{slug}`, comments, a comment form), `Admin`
(`/admin`, `requires user`, authors only: a Reader gets 404 from `load`),
`Admin.Edit` (`/admin/{id}`: edit, publish/unpublish, delete).

```alder
module Post

import User

type PostId = brand Text
type Slug = brand Text where (\s -> Text.matches "^[a-z0-9-]+$" s)

table Post {
  id: PostId @id,
  slug: Slug @unique,
  title: Text.NonEmpty,
  body: Text,
  published: Bool = False,
  author: User.UserId @ref User,
  created: Time @created,
}

bySlug : Slug -> Query (Maybe Post)
bySlug slug = Db.from Post |> Db.where (Post.slug.eq slug) |> Db.first
```

```alder
module Comment

import Post

type CommentId = brand Text

table Comment {
  id: CommentId @id,
  post: Post.PostId @ref Post @cascade,
  author: Text.NonEmpty,
  body: Text.NonEmpty,
  created: Time @created,
}
```

The slug is a `@unique` column found with `Db.first`; ids stay generated
(Decisions). Rendering goes through the capability:

```alder
module Markdown

capability Markdown {
  render : Text -> Query Html,
}
```

Its adapter (`adapters/markdown.go`) implements a small Markdown subset
(paragraphs, emphasis, links, code spans) by building `rt/html` nodes,
never by emitting raw HTML, so the output is safe by construction and
`Html.unsafe` is not needed. No third-party dependency.

**Search.** Two new column conditions in `Db`, on `Text` columns (and
`Text` brands) only: `contains : Text -> Condition t` and `startsWith`,
compiled to `LIKE` with the argument escaped (`%`, `_`, `\`) so a user's
`%` is literal; case-insensitive for ASCII (SQLite's `LIKE`). The checker
rejects them on non-text columns (E0003 "`contains` wants a Text column").
`Posts.Index` has `page "/" { page: Int = 1, q: Text = "" }`, applies `q`
to title and body (`Db.or`), orders by `created.desc`, `Db.limit 10`,
`Db.offset ((page - 1) * 10)`, and shows next/previous links.

**References.** A `@ref T` column takes one of `@cascade`, `@restrict`
(the default, so existing tables are unchanged) or `@setNull` (only on a
`Maybe` reference; E0003 otherwise). They become `ON DELETE CASCADE /
RESTRICT / SET NULL` in the generated `CREATE TABLE`; the migration
generator emits them, and changing one is a table rebuild (below). The
trace of `Db.delete Post id` shows one step; the comments go with it in
SQL. Chapter 8 says so.

**Tests.** The blog ships tests in every module: pure (`Post.visible`,
slug validation), effect (`Posts.Show.load` 404s a draft for a reader and
shows it to its author; a comment on a draft; `Admin` 404s a Reader;
`Delete` cascades, asserted by `Db.count`), and fakes (`Markdown` faked
to a `Html.text` renderer, below).

### Migrations

**The source of truth is the `table` declarations.** The compiler already
computes the schema (`db.TableDesc` per table, hashed by `Ensure`); M5
makes the schema diffable and versioned.

**Files.** `migrations/schema.json` is the snapshot: every table, column,
type, default, attributes, references and their delete rule, in a
canonical order, plus the format version. `migrations/NNNN_<slug>.sql`
are the steps (`0001_initial.sql`, `0002_add_comment.sql`), each a header
comment and SQL statements:

```sql
-- alder migration 0002_add_comment
-- from 8f3a…  to 2c91…  (schema.json hashes)
-- creates: Comment
-- changes: Post.body (Text.NonEmpty -> Text)
-- DROPS DATA: Post.summary (column removed)
BEGIN;
CREATE TABLE "Comment" (…);
…
COMMIT;
```

**`alder migrate [dir] [--name slug] [--dry-run]`.** Checks the app,
diffs the declared schema against `schema.json`, and:

- with no `migrations/` directory, creates it with `0001_initial.sql`
  (every table) and the snapshot;
- with no difference, says so and exits 0;
- otherwise writes the next file and the new snapshot, and prints each
  step; a step that drops data (a removed table or column, a type change
  that cannot keep every value) is printed as `DROPS DATA` and written in
  the file's header — generated, never silently, never skipped;
- `--dry-run` prints without writing.

The diff is deterministic (`internal/migrate`): added/removed tables,
added/removed columns, changed type/default/nullability/attributes/
references, renames (below). SQL per step, for SQLite: `CREATE TABLE`,
`DROP TABLE`, `ALTER TABLE … ADD COLUMN` (with a default when the column is
not nullable), `ALTER TABLE … DROP COLUMN`, `ALTER TABLE … RENAME COLUMN`,
`ALTER TABLE … RENAME TO`; anything else (a type change, a changed
default, nullability, a changed reference rule, a new `@unique`) is the
documented rebuild: create `T__new`, `INSERT INTO T__new SELECT …` with
the value conversion, drop `T`, rename, inside the migration's
transaction with `PRAGMA foreign_keys` handled as SQLite's procedure says.
Indexes (`@unique`) are created/dropped as statements of their own. Every
statement is in the SQLite/libSQL common subset ("Turso-ready", M3 spec).

**Renames are declared, never guessed.** `title: Text.NonEmpty @renamed
"heading"` tells the generator the column was `heading`; `table Post
@renamed "Article"` likewise for a table (grammar: the attribute after the
table name). The generated step is a rename, not drop+add. The attribute
can be removed once the migration exists (the snapshot knows the new
name); leaving it is harmless. Without it, a removed column and an added
one are what they look like, and the removed one is `DROPS DATA`.

**The check-time rule.** When `migrations/` exists, `alder check` compares
the declared schema with `schema.json`; a difference is E0019 "the schema
changed since migration 0003: run `alder migrate`" (one error, at the
first changed table). So `build`, `run` and `test` refuse an app whose
migrations are behind. An app without `migrations/` keeps today's
behaviour (dev recreates, production refuses) — the harness's generated
apps keep working, and chapter 8 recommends `alder migrate` from the
first table.

**Application.** The generated `app__alder.go` embeds the migration files
and the snapshot (`Migrations []db.Migration`). `Serve` (and `alder
test`'s runner, and `alder eval --run`) calls `db.Migrate(ctx, store,
Migrations)` instead of `Ensure` when the app has migrations: it creates
`_alder_migrations` (name, hash, applied at), applies each pending file in
order inside a transaction, logs `migrated 0002_add_comment`, and verifies
afterwards that the live schema's hash equals the snapshot's (drift: a
hand-edited database → refuse to start, naming the table; in dev mode
with `ALDER_DEV=1`, recreate as today, since the dev database is
disposable). A failed migration rolls back and the app refuses to start
with the file name and SQLite's error. Production and dev behave the
same; the only dev-only behaviour is the drift recreate.

**Tests** get a fresh `:memory:` database per test as now; it is built by
applying the migrations (so a test proves the migrations produce the
schema the app expects), falling back to `Ensure` for an app without
them.

### Stateful fakes (`fake` and `Test.fake`)

```alder
module Markdown

capability Markdown {
  render : Text -> Query Html,
}

fake Markdown {
  render text = Query.succeed (Html.text text)
}
```

```alder
module Payments

capability Stripe {
  charge : { cents: Int.Positive } -> Task (Result Text ChargeId),
  refund : { id: ChargeId } -> Task (Result Text Unit),
}

fake Stripe {
  table Charge {
    id: ChargeId @id,
    cents: Int,
    refunded: Bool = False,
  }

  charge args = {
    let id <- Db.insert Charge { cents: args.cents }
    Task.succeed (Ok id)
  }
  refund args = {
    let found <- Db.get Charge args.id
    match found {
      Just c -> { let _ <- Db.update Charge c.id (\x -> { x | refunded: True }); Task.succeed (Ok ()) },
      Nothing -> Task.succeed (Err "no such charge"),
    }
  }
}
```

- **Declaration.** `fake X { … }` in the module that declares capability
  `X` (E0020 "a fake is declared beside its capability" elsewhere; one
  fake per capability, E0009 otherwise). It holds zero or more `table`
  declarations and exactly one definition per entry of the capability,
  each checked against the entry's type (a missing or extra entry is
  E0003 naming it). Entry bodies are ordinary Alder: they may use `Db`
  on the fake's tables and on the app's tables, `Task.fail`, and other
  modules. The fake's tables are nominal records of the module
  (`Payments.Charge`), exempt from E0018 (they belong to the fake), usable
  from tests (`Db.from Payments.Charge`) and from the fake; using one from
  non-test code is E0021 "`Charge` is a fake's table: only tests and the
  fake may use it".
- **Installing.** `Test.fake : Fake -> Task Unit`, test-only like the
  rest of `Test.*`: `let _ <- Test.fake Stripe` replaces the capability's
  implementation in the test's environment for the rest of the test (the
  trace records `Test.fake Stripe`, then the entries by their usual
  names, `Stripe.charge`). Without it, a test runs the real adapter or
  fails with "not wired in tests", as today.
- **Compilation.** The checker: a `fake` is checked like a module's
  values with the capability's entry types as signatures. The IR gets
  `Fake{Capability, Tables, Entries}`; gen emits, only in test builds
  (`gen.Options{Tests: true}`, set by `alder test`'s scratch main), a Go
  struct implementing the capability's `Cap__M__X` interface whose methods
  run the Alder entries through `eff.RunInner` against the test's `Env`,
  plus the fake's tables in the test schema; `app.Fakes` maps the
  capability key to a constructor; `Test.fake` lowers to a Call that sets
  `env.Caps[key]`. Production builds contain no fake code and no fake
  tables.

### Book

- Chapter 8: `contains`/`startsWith`; the delete rules; **"Migrations"**
  replaces "Schema changes" (the workflow, a worked change on the blog
  with the generated file shown, renames, `DROPS DATA`, drift, what the
  binary does at start, the check-time rule), written for a beginner.
- Chapter 10: "Faking a capability" (the `fake` declaration, `Test.fake`,
  a lifecycle test through the Stripe-shaped example, what the trace
  shows), replacing the "not yet possible" item.
- Chapter 11: `alder migrate`; E0019–E0021 in the codes table.
- Chapter 13 §8: how a migration is generated and applied; how a fake
  becomes a Go struct.
- Chapter 2 or a new chapter 14, "A second app": a tour of the blog
  (`part=` blocks verified against `examples/blog`), shorter than the
  to-do tour, pointing at what is new.
- Chapter 12: the decisions below; the open questions E7 raised that M5
  answers (fakes; row ownership gets the blog's second data point).

## Error handling

- `alder migrate` on an app that does not check: the diagnostics, exit 1;
  on a `migrations/` directory whose snapshot does not match its files'
  final state (someone edited a file): "migration 0002's result does not
  match schema.json; regenerate it or fix the snapshot", exit 1; it never
  rewrites an existing migration file.
- Application: a failing statement rolls the migration back; the binary
  exits 1 with `migration 0003_rename_title failed: <sqlite error>`;
  drift as above.
- `@setNull` on a non-`Maybe` column, `contains` on a non-text column, a
  `fake` with a wrong entry, `Test.fake` outside a test, a fake's table
  used from app code: check errors with the codes above, each with a
  corpus case.
- A fake entry that fails (`Task.fail`) behaves like a failing adapter:
  under `Test.run` it is `Err`; outside, the test fails.

## Testing

- `internal/migrate`: a table of schema pairs → expected statements
  (every step kind; the rebuild; renames; `DROPS DATA` detection);
  property: applying the generated migrations to an empty database
  yields the snapshot's schema (hash equality) for every pair.
- `rt/db`: `Migrate` applies in order, records, skips applied, rolls back
  a failing file, detects drift, dev-recreates.
- Checker corpus: E0019, E0020, E0021, `@setNull` misuse, `contains`
  misuse, a fake missing an entry, `Test.fake` outside a test, `@renamed`
  parsing.
- Gen: a test build with a fake builds and vets; a production build of
  the same app contains no fake symbols (golden).
- `cmd/alder`: `migrate` end to end on a temp copy of the blog (init,
  add a column, rename with `@renamed`, drop with `DROPS DATA`, dry-run,
  the E0019 refusal, application at start with data kept, drift).
- The blog's own tests pass under `alder test examples/blog`; its tour
  blocks join to its files; the Markdown adapter has Go unit tests for
  escaping.
- The book verifier: every new block.

## Milestones

**M5a — the blog and the small language additions:** `contains`/
`startsWith`; delete rules; the `Markdown` capability and adapter; the
blog with pagination and search; its tests; chapter 8 additions; the tour.
**M5b — migrations:** `internal/migrate`, `alder migrate`, `@renamed`,
E0019, `rt/db.Migrate`, application at start, tests via migrations;
chapter 8 "Migrations"; both examples get `migrations/`.
**M5c — fakes:** `fake`, `Test.fake`, E0020/E0021, test-only gen; the
blog's Markdown fake and a Stripe-shaped fake in the corpus; chapter 10.
**M5d — adapter sub-packages** (the second plan, after M5c): one Go
package per domain under `adapters/`, generated wiring.

## Decisions recorded

- **Ids stay generated; a slug is a `@unique` column.** A probe app
  (2026-10-10: create a post and its first comment in one task, redirect
  by slug, comment on the post `load` found, toggle, delete with
  children, a pure test with chosen ids, an effect test with returned
  ids) checked, built and passed with no friction: a row never exists
  before its insert, the id is the next step's value, URLs use the slug.
  A user-chosen `@id` is declined unless an experiment shows the lookup
  being fumbled.
- **Delete rules are declared on the referencing column** (`@cascade`,
  `@restrict` default, `@setNull`), not chosen at the call site.
- **Search is `LIKE`-based** (`contains`, `startsWith`); FTS5 later.
- **Migrations are versioned and generated by the compiler**; generation
  is a command (`alder migrate`), application is automatic at start;
  renames are declared with `@renamed`; a step that drops data is
  generated with a `DROPS DATA` header, never silently; an app whose
  declarations are ahead of its snapshot does not check (E0019).
- **No defaulted record literals** (the user's stance, 2026-10-10: implicit
  values cause mistakes; revisit only if required fields prove worse).
- **Fakes are whole implementations with state in the per-test database**
  (`fake X { tables; entries }`, `Test.fake X`), declared beside the
  capability, compiled only into test builds.
- **Turso-ready, not Turso-only** (M3 spec): the generated SQL, the
  migration statements and the store seam are kept engine-portable.
