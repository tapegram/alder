# 8. Database

## Tables

A table is declared with `table`. It defines a record type of the same name
and the storage for it.

```alder
type TodoId = brand Text

table Todo {
  id: TodoId @id,
  title: Text.NonEmpty,
  done: Bool = False,
  due: Maybe Date,
  owner: User.UserId @ref User,
  created: Time @created,
}
```

- Exactly one field has `@id`. Its type is a brand of `Text`; the runtime
  generates the value on insert.
- `@ref T` marks a foreign key to table `T`'s id. The field's type must be
  `T`'s id type, or `Maybe` of it. A delete rule may follow it:
  `@restrict` (the default), `@cascade` or `@setNull` (below, under
  Writing).
- `@created` is set to the current time on insert. `@updated` is set on
  every write.
- A field with a default may be omitted on insert.
- `@unique` marks a field that must be unique.
- Field types: `Int`, `Float`, `Bool`, `Text`, `Date`, `Time`, brands of
  those, unions with no payloads, and `Maybe` of any of those. Nothing
  nested; model nesting with a second table and `@ref`.

A table is declared in its own module named after it (`Todo` in
`app/Todo.alder`), together with the brand for its id and any functions
that belong to it. A table declared in any other module is an error at
its declaration (E0018): `table Comment` in `app/Post.alder` belongs in
`app/Comment.alder`, even if only `Post` uses it.

A table field's default must be a **literal**: `True` or `False`, a
number, a string without `{…}`, a constructor without a payload
(`Reader`, `Nothing`), or `Just` of one of those. The default is not only
used by `Db.insert`: migrations (`alder migrate`) write it into the
database as SQL, to fill a new field in the rows that already exist, or
to replace `Nothing` when a field stops being a `Maybe`. The compiler
turns a literal into SQL text when it lowers the table (`True` becomes
`1`, `Reader` becomes `'Reader'`, `Nothing` becomes `NULL`); an
expression would have to be run by the program, so it is refused (E0022):

```alder error=E0022 "a table field's default must be a literal"
module Note

type NoteId = brand Text

defaultBody : Text
defaultBody = "(empty)"

table Note {
  id: NoteId @id,
  body: Text = defaultBody,
}
```

Defaults of forms, records, the session and page parameters are never
stored, and may be any expression.

When you rename a table or a field, say so with **`@renamed "old"`**,
after the table's name or after the field's type (and default). The next
migration then renames the table or column, keeping its data, instead of
dropping the old one and creating an empty new one; without the
attribute a rename looks exactly like a removed field and an added one.
The attribute may stay once the migration is made (the migration's
snapshot already has the new name):

```alder
module Memo

type MemoId = brand Text

table Memo @renamed "Note" {
  id: MemoId @id,
  text: Text @renamed "body",
  pinned: Bool = False,
}
```

The old name must be a name (`"body"`, not `"the body"`), not the field's
or table's own name, and not a field the table still has; a field takes
one `@renamed`, and two fields cannot both be renamed from one name (all
E0011).

## Reading

Reads are `Query` values.

```alder fragment
-- Generic over tables: a program cannot state these (a function of `Table t` is E0003, below).
Db.get   : Table t -> Id t -> Query (Maybe t)
Db.from  : Table t -> Select t
Db.where : Condition t -> Select t -> Select t
Db.orderBy : Order t -> Select t -> Select t
Db.limit : Int -> Select t -> Select t
Db.offset : Int -> Select t -> Select t
Db.all   : Select t -> Query (List t)
Db.first : Select t -> Query (Maybe t)
Db.count : Select t -> Query Int
```

`Table t` is the table's name used as a value: `Db.get Todo id`. Conditions
and orders are built from the table's columns, which are values named
`Table.field`:

```alder
Db.from Todo
  |> Db.where (Todo.owner.eq session.user)
  |> Db.where (Todo.done.eq False)
  |> Db.orderBy Todo.created.desc
  |> Db.limit 50
  |> Db.all
```

Column operations:

```alder
Todo.title.eq   : Text.NonEmpty -> Condition Todo   -- the column's type; also .ne
Todo.created.lt : Time -> Condition Todo         -- also .le .gt .ge
Todo.title.contains : Text -> Condition Todo     -- substring, case-insensitive
Todo.title.startsWith : Text -> Condition Todo   -- prefix, case-insensitive
Todo.owner.in   : List User.UserId -> Condition Todo
Todo.due.isNothing : Condition Todo              -- for Maybe fields; also .isJust
Todo.created.asc, Todo.created.desc : Order Todo   -- unions order by declaration

Db.and : List (Condition t) -> Condition t
Db.or  : List (Condition t) -> Condition t
Db.not : Condition t -> Condition t
```

A `Select` is a description that the steps add to, not a pipeline that
runs in order: where a step sits does not change what it means.

- Several `Db.where` calls are combined with `and`.
- `Db.orderBy` keys accumulate in the order written: the first is the
  primary sort key, a later one breaks its ties.
- `Db.limit` and `Db.offset` are settings: the last one written wins, and
  they apply after every condition and ordering wherever they appear, so
  `Db.limit 10 |> Db.where c` is ten rows that satisfy `c`.
- `Db.first` is the first row: a `Select` with limit 1, or with its own
  limit when that is smaller (`Db.limit 0 |> Db.first` is `Nothing`).

A column must be written in place, `Todo.done.eq False`: a column
method on a variable or record field holding a column (`let c = Todo.done`,
then `c.eq False`) is E0003. And the database functions need to know their
table where they are used: `countAll : Table t -> Query Int` calling
`Db.count (Db.from t)` is E0003 (`` `Db.count` needs a concrete table; `t`
is a type parameter``); write one function per table, `Table Todo`.

**`Maybe` columns.** A condition on a `Maybe` column is true or false for
every row, never unknown. `Nothing` is matched by `.isNothing`,
`.eq Nothing` and an `.in` list that contains `Nothing`
(`Todo.due.in [Just d, Nothing]`), and `.eq (Just d)` and `.ne Nothing`
never match it. `Db.not c` matches exactly the rows `c`
does not, so `Db.not (Todo.due.eq (Just d))` includes the rows with no due
date. Ordering by a `Maybe` column puts `Nothing` first with `.asc` and
last with `.desc`.

**Search.** `.contains` matches a column whose text has the argument
somewhere in it, and `.startsWith` one whose text begins with it. Both
work on `Text` columns and brands of `Text` only (on a brand, the
argument is still plain `Text`), and both ignore the case of the letters
A–Z. The argument is matched literally: `%`, `_` and `\` have no special
meaning, so `Todo.title.startsWith "100%"` finds `100% done` and not
`1000 ways`. An empty argument matches every row. To search two columns,
combine the conditions with `Db.or`:

```alder
module Article

type ArticleId = brand Text

table Article {
  id: ArticleId @id,
  title: Text.NonEmpty,
  body: Text,
  created: Time @created,
}

search : Text -> Query (List Article)
search q =
  Db.from Article
    |> Db.where (Db.or [Article.title.contains q, Article.body.contains q])
    |> Db.orderBy Article.created.desc
    |> Db.limit 10
    |> Db.all
```

A search costs the column's length times the argument's for every row,
and SQLite refuses a pattern much over 50 KB, so a search text over
10,000 bytes fails the query with `search text too long`. Cut a query a
visitor typed before searching with it, as the blog does with
`Text.slice 0 100` (chapter 14).

On any other column the method is an error (E0003):

```alder error=E0003 "`.startsWith` needs a Text column, got `Bool`"
Db.from Todo |> Db.where (Todo.done.startsWith "T")
```

## Writing

Writes are `Task` values.

```alder fragment
-- Generic over tables, like the reads above.
Db.insert : Table t -> Insert t -> Task (Id t)
Db.update : Table t -> Id t -> (t -> t) -> Task Unit
Db.delete : Table t -> Id t -> Task Unit
```

`Insert t` is the table record without `@id`, `@created`, and `@updated`
fields, and with defaulted fields optional. `Db.update` reads the row,
applies the function, and writes it back; updating a row that doesn't exist
fails the task. The function may not change the `@id` field: if it
returns a record with a different id, the task fails with
`Db.update Todo: update may not change the id` and nothing is written
(`UpdateRow` in `rt/db/store.go`). Deleting a row that doesn't exist is
not an error.

```alder fragment
let id <- Db.insert Todo { title: p.form.title, due: p.form.due, owner: session.user }
let _ <- Db.update Todo id (\t -> { t | done: True })
let _ <- Db.delete Todo id
```

Writes in one `update` run in a single transaction. If the task fails,
nothing is written. Inserting or updating a row that violates `@unique`
fails the task with a message such as `User.email must be unique`, and
everything the task wrote before is rolled back; check first with a query
and `Reject` if the user should see a message. `Reject` is a successful
result, so writes made before it are kept (chapter 6): decide first, then
write.

`@ref` is checked the same way. Inserting or updating a row whose `@ref`
field names a row that does not exist fails the task with
`Todo.owner: no User with that id`. What deleting a row that other rows
still refer to does is the referring field's **delete rule**, written
after `@ref T`:

- **`@restrict`**, the default, refuses: `Db.delete` fails with
  `User QXZMRYnkC2vuE_TefkuR is referenced by another row, directly or
  through rows that `@cascade` would delete with it` (the table and the
  id). The blocking row can be further down a chain: if comments cascade
  from posts, a `@restrict` reference to a comment refuses the post's
  delete. Delete or re-point the referring rows first.
- **`@cascade`** deletes the referring rows too, and theirs in turn if
  other tables cascade from them.
- **`@setNull`** sets the referring field to `Nothing`; the field must be
  a `Maybe`.

Either way a failure rolls back the task's earlier writes. A table of
notes attached to to-dos, which go when their to-do goes, and which keep
an optional link to another to-do that is cleared when that one goes:

```alder
module Note

import Todo

type NoteId = brand Text

table Note {
  id: NoteId @id,
  todo: Todo.TodoId @ref Todo @cascade,
  seeAlso: Maybe Todo.TodoId @ref Todo @setNull,
  body: Text,
}
```

The rules are carried out by SQLite inside the delete statement, not by
the runtime: `Db.delete Todo id` is one step in a test's trace
(chapter 10) however many notes go with it. A field takes at most one
rule, and only a `@ref` field takes one, after `@ref T` (`@cascade` alone,
or before `@ref T`, is E0011).
`@setNull` on a field that is not a `Maybe` is E0003, since the field
could not hold the `Nothing` it would be set to:

```alder error=E0003 "`@setNull` needs a `Maybe` reference, got `Todo.TodoId`"
module Note

import Todo

type NoteId = brand Text

table Note {
  id: NoteId @id,
  todo: Todo.TodoId @ref Todo @setNull,
  body: Text,
}
```

## How tables are stored

The database is SQLite (`rt/db/sqlite.go`). Each `table` is one SQL
table, named by its module-qualified name with `.` replaced by `__`:
`table Todo` in module `Todo` is `Todo__Todo`. Each field is one column
with the field's name, in declaration order. Values are stored like this
(the package comment of `rt/db/table.go`):

| Alder | SQL column |
|---|---|
| `Text`, a brand of `Text` | `TEXT` |
| `Int` | `INTEGER` |
| `Float` | `REAL` |
| `Bool` | `INTEGER`, `0` or `1` |
| `Date` | `TEXT`, `YYYY-MM-DD` |
| `Time` | `TEXT`, RFC 3339 in UTC with a fixed nine-digit fraction (`2026-10-09T12:00:00.000000000Z`), so text order equals time order |
| a union with no payloads | `TEXT`, the constructor's name (`Active`) |
| `Maybe a` | `a`'s column, `NULL` for `Nothing` |

A `Time` is always written with nine digits after the second, so every
stored time has the same width, and comparing the text compares the
times. `EncodeTime` in `rt/db/encode.go` does it:

```go
// EncodeTime is t as stored: RFC 3339, fixed 9-digit fraction, UTC
// ("2026-10-09T12:00:00.500000000Z"). Every stored time has the same width and the same zone,
// so comparing and ordering the text (Time conditions, Db.orderBy) compares the times.
func EncodeTime(t time.Time) string { return t.UTC().Format(timeLayout) }
```

Every column except a `Maybe` is `NOT NULL`. The `@id` is the primary key,
`@unique` adds `UNIQUE`, `@ref` adds `REFERENCES`, followed by
`ON DELETE CASCADE` for `@cascade` and `ON DELETE SET NULL` for
`@setNull` (`@restrict` adds nothing: SQLite refuses such a delete by
default), and a `Bool` or union column gets a `CHECK` listing its
allowed values. For the to-do example's
`Todo` table the runtime creates:

```
CREATE TABLE "Todo__Todo" ("id" TEXT NOT NULL PRIMARY KEY, "title" TEXT NOT NULL, "done" INTEGER NOT NULL CHECK ("done" IN (0, 1)), "due" TEXT, "owner" TEXT NOT NULL REFERENCES "User__User"("id"), "created" TEXT NOT NULL) STRICT
```

A few rules follow from the mapping:

- **Ids** are 20 random characters from `A–Z a–z 0–9 - _`, safe in a URL
  (`NewID` in `rt/db/store.go`), such as `QXZMRYnkC2vuE_TefkuR`.
- **`@created` and `@updated`** are set from the runtime's clock when the
  row is written. `Db.update` never rewrites the id or `@created`.
- **Unions order by declaration.** `orderBy` and `.lt`/`.gt` on a union
  column follow the order the constructors are declared in, not
  alphabetical order.
- **`.contains` and `.startsWith`** are SQL's `LIKE` with `%` around
  the argument (after it, for `.startsWith`), and with `%`, `_` and `\`
  in the argument escaped, so they match themselves. `LIKE` ignores case
  for the letters A–Z only; `É` and `é` differ.
- **Empty lists.** `Db.and []` matches every row, `Db.or []` matches no
  row, and `.in []` matches no row.

Every value reaches SQLite as a parameter, never as part of the SQL text,
so no input can change what a query does.

## Migrations

Your users' data has to survive when the tables change. A **migration**
is a file of SQL statements that turns a database with the old tables
into one with the new tables, keeping the rows. Alder writes them: you
change a `table`, run `alder migrate`, read the file, and commit it. The
app applies the files it has not had yet every time it starts.

### The files

An app with migrations has a `migrations/` directory beside `app/`. Both
examples have one:

```text
examples/blog/migrations/
  0001_initial.sql    -- creates every table
  schema.json         -- the tables as of the last migration
```

Each `NNNN_<name>.sql` is one migration, numbered in the order they
apply. `schema.json`, the **snapshot**, records every table, column,
type, default and attribute as of the last migration; `alder migrate`
compares your `table` declarations with it. Commit the whole directory:
the executable carries the `.sql` files inside it (chapter 13).

For your own app, run `alder migrate` once, from the first table on:
with no `migrations/` it creates one holding `0001_initial.sql`, which
creates every table (an app without migrations loses its data on every
change: "Without migrations", below).

### A change, step by step

The steps below were run on a copy of the blog (chapter 14):
`cp -R examples/blog /tmp/blog`, then `cd /tmp/blog`. First, give posts a
summary. Add the field to `table Post` in `app/Post.alder`:

```alder fragment
  created: Time @created,
  summary: Maybe Text = Nothing,
}
```

The default `= Nothing` lets every `Db.insert Post` in the blog leave the
field out. The one test that builds a whole `Post` by hand needs
`summary: Nothing` too (the checker points at it). Now the tables are
ahead of the migrations, and the compiler says so:

```text
$ alder check
migrations/schema.json:1:1: error[E0019]: the schema changed since migration 0001 (table Post: column summary): run `alder migrate`
```

`build`, `run`, `test` and `eval` refuse the app with the same error.
Run the command it names:

```text
$ alder migrate
wrote migrations/0002_add_summary.sql
  add column Post.summary
updated migrations/schema.json
```

```text
$ cat migrations/0002_add_summary.sql
-- alder migration 0002_add_summary
-- from 3fa31c8ee016
-- to c709adc8b82b
-- step: add column Post.summary

ALTER TABLE "Post__Post" ADD COLUMN "summary" TEXT;
```

The `--` lines are the header: `from` and `to` are short fingerprints
(*hashes*) of the tables before and after the file, and the `step:`
lines say in words what the statements below them do, one per line.
The runtime runs each file in a transaction of its own. Read the file,
then commit it with `schema.json`.

Next, rename the field. A rename looks exactly like a removed field and
an added one, so say it is a rename with `@renamed "old"` (above, under
Tables):

```alder fragment
  excerpt: Maybe Text = Nothing @renamed "summary",
```

```text
$ alder migrate
wrote migrations/0003_rename_summary.sql
  rename column Post.summary to excerpt
updated migrations/schema.json
```

The file's statement is
`ALTER TABLE "Post__Post" RENAME COLUMN "summary" TO "excerpt";`, which
keeps every post's summary. Without `@renamed`, the migration would drop
`summary`, with its data, and add an empty `excerpt`. Once the migration
exists you may delete `@renamed "summary"`: `alder check` stays clean,
because the snapshot already says `excerpt`.

Last, remove the field. `--dry-run` prints the file without writing it:

```text
$ alder migrate --dry-run
-- alder migration 0004_drop_excerpt
-- from 7af9d614a3af
-- to 3fa31c8ee016
-- step: drop column Post.excerpt
-- DROPS DATA: column Post.excerpt

ALTER TABLE "Post__Post" DROP COLUMN "excerpt";

dry run: migrations/0004_drop_excerpt.sql not written
```

```text
$ alder migrate
wrote migrations/0004_drop_excerpt.sql
  drop column Post.excerpt
!! DROPS DATA: column Post.excerpt
updated migrations/schema.json
```

**A migration never loses data silently.** Every step that can lose data
is marked `DROPS DATA`, in the output (`!!`) and in the file's header.
Read those lines before you commit. A migration you have not shared yet
can be thrown away: delete the file, restore the snapshot with
`git checkout migrations/schema.json`, change the tables, and run
`alder migrate` again. Once a migration has run against a database you
keep, never edit or delete it; write the next one instead.

The file's name comes from its steps (`add_summary`, `rename_summary`,
`drop_excerpt`, `create_comment`, `change_post` for a rebuild, below), at
most 40 characters; `--name some_name` chooses it instead. Run with
nothing changed, `alder migrate` writes nothing and prints
`no changes since 0004_drop_excerpt`.

### What a change becomes

- **A new `Maybe` field** with no default (or `= Nothing`) is
  `ADD COLUMN`; existing rows get `Nothing`.
- **A new field that is not a `Maybe`** needs a default to fill the rows
  that already exist. Without one, `alder migrate` writes nothing and
  says ``Post.views: a new non-null column needs a default (`= 0`) so
  existing rows can be filled`` (if no `Db.insert` has refused to
  compile first). A new `@ref` field has no possible
  default, so add it as a `Maybe` first.
- **A removed field or table** is `DROP COLUMN` or `DROP TABLE`, marked
  `DROPS DATA`.
- **A rename** declared with `@renamed` is `RENAME COLUMN` or
  `RENAME TO`.
- **Anything else is a rebuild.** SQLite's `ALTER TABLE` can only add,
  drop and rename columns. To change a type, make a field a `Maybe` or
  not, change a union's constructors, add or remove `@unique`, `@ref` or
  a delete rule, or add a field that is not a `Maybe`, the migration
  builds the table again: it creates the new table under a temporary
  name, copies every row into it, converting each value, drops the old
  table and renames the new one. Adding `views: Int = 0` to `Post`:

```text
-- step: rebuild table Post: Post.views: new column filled with 0

CREATE TABLE "Post__Post__alder__new" ("id" TEXT NOT NULL PRIMARY KEY, "slug" TEXT NOT NULL UNIQUE, "title" TEXT NOT NULL, "body" TEXT NOT NULL, "published" INTEGER NOT NULL CHECK ("published" IN (0, 1)), "author" TEXT NOT NULL REFERENCES "User__User"("id"), "created" TEXT NOT NULL, "views" INTEGER NOT NULL) STRICT;
INSERT INTO "Post__Post__alder__new" ("id", "slug", "title", "body", "published", "author", "created", "views") SELECT "id", "slug", "title", "body", "published", "author", "created", 0 FROM "Post__Post";
DROP TABLE "Post__Post";
ALTER TABLE "Post__Post__alder__new" RENAME TO "Post__Post";
CREATE INDEX IF NOT EXISTS "Post__Post__author" ON "Post__Post"("author");
```

A conversion that cannot keep every value is marked `DROPS DATA` and
says what is lost: `Float` to `Int` cuts off fractions; `Text` to `Int`
keeps the leading whole number (`'12abc'` becomes `12`) and turns other
text into `0`; a removed constructor deletes the rows that hold it; a
`Maybe` field made required replaces `Nothing` with the default, or,
without one, deletes those rows. A change with no sensible conversion
(`Date` to `Int`, say) is refused: remove the field and add one under a
new name. Adding `@unique` to a column that holds duplicates fails when
the migration runs, and nothing is changed.

A default lives in the program, not the database, so changing only a
default needs no migration. Every `@ref` column gets an index (the
`CREATE INDEX` line above), so finding a post's comments does not read
every comment.

### When the app starts

Before it serves anything, the executable brings its database up to
date (`db.Migrate`, in `rt/db/migrate.go`):

1. It reads `_alder_migrations`, the table listing the files applied.
2. It applies every other file, in order, each in its own transaction:
   the statements, a check that every `@ref` still points at a row, and
   the file's name into `_alder_migrations`.
3. It checks that the tables are exactly the app's.

The first start on an empty database logs each file (with a date and
time in front of each line); the next start applies nothing:

```text
$ ALDER_DB=blog.db ALDER_ADDR=127.0.0.1:8080 ALDER_SECRET=… ./build/blog/blog
info: migrated 0001_initial
info: migrated 0002_add_summary
info: migrated 0003_rename_summary
info: migrated 0004_drop_excerpt
listening on 127.0.0.1:8080
```

Before changing anything it checks that each file's `from` is where the
database, or the file before it, ends. Otherwise it prints `alder: ` and
the reason, and exits 1. The reasons, word for word:

- A statement fails: the file's transaction is undone, the files before
  it stay applied, and the app stops with
  `migration 0003_rename_summary failed: <SQLite's error>`.
- A file deletes rows that other rows still point at:
  `migration 0005_x: rows in Comment refer to rows this migration deletes: delete or re-point them first`.
- A file the database has had is gone:
  `migration 0003_rename_summary is recorded as applied but its file is missing`.
- A new file is numbered before one the database has had (two branches
  each added a `0005`):
  `migration 0005_a is not applied but 0005_b, which comes after it, is: migrations apply in order, so give it a number after 0005_b`.
- The files do not lead from the database's tables to the app's:
  `migration 0003_add_views expects the schema <hash> but the database is at <hash> (is a file missing, or was the database changed outside migrations?)`,
  or `db: the database's tables are not the app's schema: 0004_drop_excerpt leads to the schema <hash>, but the app's is <hash>: the migrations do not produce the app's schema`.
- Someone changed the tables by hand (here, added a column with the
  `sqlite3` tool):

```text
alder: db: the database's tables are not the app's schema: the database was changed outside migrations since 0004_drop_excerpt (changed tables: Post.Post); back it up and restore it, or run in dev mode to recreate it
```

`alder check` catches most of these first: a missing or edited file is
E0019 (`migration 0004_drop_excerpt does not follow
0002_add_summary: a file is missing or edited; regenerate it`), and so is
a malformed one (`migration 0002_x: line 6: a statement must end in `;``).

**A database from before migrations.** An app that ran before it had
`migrations/` has a database `Ensure` made (below), with tables but no
`_alder_migrations`. If its tables are exactly what one of the files
leads to, usually `0001_initial`, the executable *adopts* it: it records
that file and the ones before it as applied without running them, logs
`adopted the database at 0001_initial`, and applies the rest. If they
match no file, it refuses:
`this database was not created by migrations and matches none of them: back it up, then recreate it (or add a migration that produces its schema)`.

**Dev mode** (`alder run`, or `ALDER_DEV=1`) treats the database as
disposable. Where production refuses because the database is not where
the files say (made outside migrations and matching none, changed by
hand, not what the files produce, or of another storage format), dev
mode drops every table, creates the app's tables again, empty, records
every file as applied, and logs why:

```text
info: recreated the database (dev mode): it was changed outside migrations
```

A file that is missing or out of order is refused in dev mode too: the
files, not the database, are wrong. `alder eval --run` and the REPL never
recreate the database; they add where to delete it to start again.

### Tests run through migrations

`alder test` builds each test's fresh in-memory database by applying the
migrations, as a production start would. So a passing run also shows
that the migrations make the tables the app expects. When they do not
(a hand-edited file, say), every test fails with
``the migrations do not produce the app's schema: run `alder migrate` ``.

### Without migrations

An app without `migrations/` gets its tables from `Ensure`
(`rt/db/sqlite.go`), which records their hash and the storage format
(`db.Format`, now `1`) in `_alder_meta`. If either differs at start,
dev mode recreates the tables, empty; otherwise the app refuses:

```
db: schema mismatch: the database's schema (hash 5de59b25abca) differs from the app's (hash 40d037c4462a); changed tables: Todo.Todo; run in dev mode to recreate the database
```

or, for a database from a runtime with an older format:

```
db: database format v0 is older than this runtime (v1); run in dev mode to recreate the database
```


## Beyond CRUD

Joins, aggregates, and full-text search are not in the language. Declare a
capability and implement it in Go against the same database, through
`db.StoreFrom(ctx)` and `db.TxFrom(ctx)` so that it reads and writes in
the running task's transaction (chapter 7):

```alder
capability Reports {
  todosPerUser : Query (List { user: User.UserId, count: Int }),
}
```
