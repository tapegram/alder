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
  owner: User.UserId @ref User @owner,
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
- A field with a default may be omitted on insert, but not from a
  literal of the row type: a `Todo` written out names every field.
- `@unique` marks a field that must be unique.
- `@owner` marks the one field that names the row's owner: a `@ref` to
  the user table, of the session's user type. It gives the table the
  scoped operations `Db.mine`, `Db.getMine`, `Db.updateMine`,
  `Db.deleteMine` and `Db.insertMine` ("Your own rows", under Writing; chapter
  6, "Rules"). It changes nothing in the database: the column is stored,
  indexed and referenced exactly as without it, so adding or removing
  `@owner` makes no migration.
- Field types: `Int`, `Float`, `Bool`, `Text`, `Date`, `Time`, brands of
  those, unions with no payloads, and `Maybe` of any of those. Nothing
  nested; model nesting with a second table and `@ref`.

A table is declared in a module named after it, together with the brand for
its id and any functions that belong to it. By convention that module is in
`Table/`: `Todo` in `app/Table/Todo.alder`, written `Todo` after
`import Table.Todo`. The compiler's rule is that the module's name **ends in
the table's name** (`Todo`, `Table.Todo` or `Table.Billing.Invoice` for
`Invoice`), so subfolders can group related tables, and a module holds one
table. A table declared in any other module is an error at its declaration
(E0018): `table Comment` in `app/Post.alder` belongs in
`app/Table/Comment.alder`, even if only `Post` uses it. Table names are
app-wide (a table is `Todo` everywhere, whichever folder it is in), so two
tables of one name, in two folders, are E0037.

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
module Table.Memo

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

Moving a table to another module, as when `app/Todo.alder` becomes
`app/Table/Todo.alder`, needs no `@renamed`: the migration sees the same
table name leave one module and arrive in another, and renames the table,
so its rows stay. (`@renamed "Note"` likewise finds a table of `Table/`
by its short name.)

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
  ``User QXZMRYnkC2vuE_TefkuR is referenced by a row of Todo (`Todo.owner`)``
  (the table and the id, then the table and the field that refer to it).
  The blocking row can be further down a chain: if comments cascade
  from posts, a `@restrict` reference to a comment refuses the post's
  delete, with ``Post … is referenced by another row, directly or
  through rows that `@cascade` would delete with it``. Delete or re-point
  the referring rows first.
- **`@cascade`** deletes the referring rows too, and theirs in turn if
  other tables cascade from them.
- **`@setNull`** sets the referring field to `Nothing`; the field must be
  a `Maybe`.

Either way a failure rolls back the task's earlier writes. A table of
notes attached to to-dos, which go when their to-do goes, and which keep
an optional link to another to-do that is cleared when that one goes:

```alder
module Table.Note

import Table.Todo

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
module Table.Note

import Table.Todo

type NoteId = brand Text

table Note {
  id: NoteId @id,
  todo: Todo.TodoId @ref Todo @setNull,
  body: Text,
}
```

### Your own rows

A table with an `@owner` column (above) also has five **scoped**
operations, for the pages of a signed-in user:

```alder fragment
Db.mine       : Table t -> Select t                  -- Db.from t, only the user's rows
Db.getMine    : Table t -> Id t -> Query t           -- the user's row, else not found
Db.updateMine : Table t -> Id t -> (t -> t) -> Task Unit
Db.deleteMine : Table t -> Id t -> Task Unit
Db.insertMine : Table t -> Insert t -> Task (Id t)   -- the row without its owner, as a literal
```

They act for the user of the running request (or test), and answer a
row that is someone else's as a missing one: not found, a 404.
`Db.getMine` is a `Query t`, not a `Query (Maybe t)`: there is nothing
to match, because a missing row ends the page. They are allowed only in
a page module whose page `requires user`. Chapter 6, "Rules", explains
them in full, and chapter 13 shows their SQL: each puts `"owner" = ?` in
its statement.

### Partitions

A table may be partitioned by any session key, not only the user: `org:
Org.OrgId @ref Org @partition org` keeps a table's rows to the session's
org as `@owner` keeps them to its user (chapter 6, "Partitions"), with the
generic scoped operations `Db.scoped`, `Db.getScoped`, `Db.updateScoped`,
`Db.deleteScoped` and `Db.insertScoped` (chapter 9). By itself a
partition is a predicate: the rows of every org are in one table of one
database, and each scoped statement carries `"org" = ?`. The next
section gives an org a database of its own.

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
so no input can change what a query does. It also means one query's text is
the same for every value, so the store prepares each text once (SQLite
parses and plans it into a program) and reuses the prepared statement on
later runs, which saves about 7 µs a statement. The cache holds at most 256
texts (a `.in` list makes one per list length) and is emptied whenever the
tables may change (at start-up's table check and migrations), so a
prepared statement never outlives the table it was planned for; a database
on Turso (chapter 15) prepares nothing, since each statement is sent over
the network anyway.

## Migrations

A **migration** is a file of SQL statements that turns a database with
the old tables into one with the new tables, keeping the rows. Alder
writes them: you change a `table`, run `alder migrate`, read the file,
and commit it. The app applies new files every time it starts.

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
summary. Add the field to `table Post` in `app/Table/Post.alder`:

```alder fragment
  created: Time @created,
  summary: Maybe Text = Nothing,
}
```

The default `= Nothing` lets every `Db.insert Post` leave the field out,
but not a `Post` written as a literal: the checker points at the one test
that builds a whole `Post` by hand, which needs `summary: Nothing`:

```text
app/Table/Post.alder:35:22: error[E0003]: record literal for `Post` is missing `summary` (a default applies only to an insert, the session cookie, a page or message parameter, or a form, not to a literal)
```

With that fixed, the tables are ahead of the migrations:

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
keep, never edit or delete it; write the next one instead. (A file
whose statements were edited is caught when the app applies it, at
startup: the tables do not come out as its header says, and the app
refuses to start; `alder test` runs every file, so it finds it first.)

The file's name comes from its steps (`add_summary`, `change_post` for
a rebuild), or from `--name some_name`. With nothing changed,
`alder migrate` prints `no changes since 0004_drop_excerpt`.

### What a change becomes

- **A new `Maybe` field, last in the table,** with no default or
  `= Nothing` is `ADD COLUMN`; existing rows get `Nothing`.
- **A new field that is not a `Maybe`** needs a default to fill the rows
  that already exist. Without one, `alder migrate` writes nothing and
  says ``Post.views: a new non-null column needs a default (`= 0`) so
  existing rows can be filled`` (if no `Db.insert` has refused to
  compile first). A new `@ref` field has no possible
  default, so add it as a `Maybe` first.
- **A removed field** is `DROP COLUMN`, and a removed table `DROP TABLE`,
  marked `DROPS DATA`. SQLite cannot drop a `@unique` or `@ref` column in
  place, so removing one of those is a rebuild (below), marked
  `DROPS DATA` all the same.
- **A rename** declared with `@renamed` is `RENAME COLUMN` or
  `RENAME TO`.
- **Anything else is a rebuild.** SQLite's `ALTER TABLE` can only
  append, drop and rename columns. To change a type, make a field a
  `Maybe` or not, change a union's constructors, add or remove `@unique`,
  `@ref` or a delete rule, add a field anywhere but last, reorder fields,
  or add a field that is not a `Maybe` or has a `Just` default (filled
  with the default), the migration builds the table again: it creates
  the new table under a temporary name, copies every row into it,
  converting each value, drops the old table and renames the new one.
  Adding `views: Int = 0` to `Post`:

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
text into `0`; a removed constructor deletes the rows that hold it (the
line adds `(to keep them, add an UPDATE first)`, a statement you write
at the top of the file); a `Maybe` field made required replaces `Nothing` with the
default, or, without one, deletes those rows. A change with no sensible conversion
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

1. In one transaction that holds the database's write lock, it reads
   `_alder_migrations` (the table listing the files applied), the
   tables, and the fingerprint recorded with the last file, and checks
   that they agree with the files: everything a refusal below names is
   found here, before anything changes.
2. It applies every other file, in order, each in its own transaction:
   a check that the tables are the file's `from`, the statements, a
   check that every `@ref` still points at a row, a check that the
   tables are now the file's `to`, and the file's name into
   `_alder_migrations`. Any check that fails undoes the file.
3. It checks that the tables are exactly the app's, and creates the
   index of every `@ref` column that lacks one.

Two copies of the app starting at once take turns: the second waits for
the first's lock, then finds the files recorded and applies nothing.

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

**There are no down-migrations.** A migration only goes forward; to undo
one, restore the database from a backup. Back up the database before
every deploy that brings a migration: stop the app and copy the file
(with its `-wal` file, if there is one), or, while it runs,
`sqlite3 blog.db ".backup blog-before-0005.db"`.

If the database cannot be brought up to date, the app prints `alder: `
and the reason, and exits 1. A file that fails is rolled back, the files
before it stay applied, and nothing else changes. Word for word:

- A statement fails: the app stops with
  `migration 0003_rename_summary failed: <SQLite's error>`.
- A file's statements do not make the tables its header names (someone
  edited the file after `alder migrate` wrote it):
  `migration 0004_drop_excerpt does not produce the schema its header names (to <hash>, got <hash>): the file was edited; restore it from version control`.
- A file deletes rows that other rows still point at:
  `migration 0003_change_user: rows in Post refer to rows this migration deletes: delete or re-point them first: add an UPDATE or DELETE at the top of 0003_change_user.sql (it was not applied)`
  (a statement you write at the top of the file).
- The database has had a file this executable does not have, newer than
  all of its own (an older build started on a newer database):
  `migration 0005_add_tags is recorded as applied but its file is missing; this executable predates the database (it has had 0005_add_tags): deploy a build that includes it, or restore a backup taken before it`.
- A file the database has had is gone from the middle:
  `migration 0003_rename_summary is recorded as applied but its file is missing: restore it from version control`.
- A new file is numbered before one the database has had:
  ``migration 0005_a is not applied but 0005_b, which comes after it, is: migrations apply in order; delete the file and run `alder migrate` again (only if it has not run against a database you keep)``.
- The files do not lead from the database's tables to the app's:
  `migration 0003_add_views expects the schema <hash> but the database is at <hash> (is a file missing, or was the database changed outside migrations?)`;
  `db: the database's tables are not the app's schema: 0004_drop_excerpt leads to the schema <hash>, but the app's is <hash>: the migrations do not produce the app's schema`;
  or, with no file left to apply,
  `db: the database's tables are not the app's schema: the database is at the schema <hash> (after 0004_drop_excerpt), but the app's is <hash>: the migrations do not produce the app's schema`.
- Another copy of the app held the database longer than five seconds
  (a long rebuild):
  `db: the database is locked: another instance is migrating or serving; wait and retry (<SQLite's error>)`.
- Someone changed the tables by hand (here, added a column with the
  `sqlite3` tool):

```text
alder: db: the database's tables are not the app's schema: the database was changed outside migrations since 0004_drop_excerpt (changed tables: Post): restore a backup; to start over, run in dev mode (ALDER_DEV=1 recreates the database empty: every row is deleted)
```

`alder check` catches the file problems first, as E0019: a missing or
edited header (``migration 0004_drop_excerpt does not follow
0002_add_summary: a file is missing or edited; restore migrations/ from
version control (`git checkout migrations/`)``), a malformed file
(``migration 0002_x: line 6: a statement must end in `;` ``), and two
files with one number, which happens when two branches each add the next
migration: ``two migrations are numbered 0005 (likely two branches:
0005_a.sql, 0005_b.sql): keep the one already merged or deployed, delete
the other, restore schema.json to its result, run `alder migrate` ``.
A file whose header is intact but whose statements were edited is found
only when it runs, by the `to` check above; `alder test` runs every
file, so it finds it before a deploy does.

### Statements you add

Some changes need a statement Alder cannot write: keeping the rows a
removed constructor would delete, or deleting the comments of posts a
migration deletes. You may add statements to a file `alder migrate`
wrote, under four rules:

- **Only in a file no database you keep has applied.** A database runs
  each file once; one that has run it never sees what you add.
- **One statement per line, ending in `;`.** A line starting with `--`
  is a comment; blank lines are skipped. Leave the header as it is.
- **Data only.** `UPDATE`, `DELETE` and `INSERT` change rows, not tables:
  the runtime checks that each file leaves the tables its `to` names,
  and refuses one that does not, as an edited file.
- **At the top, before the rebuild,** so that they run on the old table.

Removing a constructor, say `Editor` from `type Role = Reader | Author |
Editor` in the blog's `User`, rebuilds the table and deletes the editors:

```text
-- step: rebuild table User: User.role: Reader | Author | Editor -> Reader | Author
-- DROPS DATA: rows whose User.role is Editor are deleted (to keep them, add an UPDATE first)
```

and, since posts point at their author, it is refused when it runs if
an editor wrote one (the foreign-key refusal above). To make the
editors authors instead, add one line at the top of the file:

```text
-- keep the editors: they become authors
UPDATE "User__User" SET "role" = 'Author' WHERE "role" = 'Editor';
CREATE TABLE "User__User__alder__new" (…) STRICT;
…
```

A table's name in SQL is its module and its name joined by `__`
(`User__User`), and a column's is its field's. The `DROPS DATA` line
stays in the header; it says what the generated statements would lose,
and your `UPDATE` is why they no longer do.

**A database from before migrations.** An app that ran before it had
`migrations/` has a database `Ensure` made (below), with tables but no
`_alder_migrations`. If its tables are exactly what one of the files
leads to, usually `0001_initial`, the executable *adopts* it: it records
that file and the ones before it as applied without running them, logs
`adopted the database at 0001_initial`, applies the rest, and creates
any `@ref` index the database lacks. If they match no file, it refuses:

```text
alder: this database was not created by migrations and matches none of them: if the app made it before it had migrations, generate 0001 from the deployed version (check out that commit, run `alder migrate`), then migrate forward; otherwise back it up; to start over, run in dev mode (ALDER_DEV=1 recreates the database empty: every row is deleted)
```

That happens when the tables changed between the deploy that made the
database and the commit that added `migrations/`: `0001_initial`
creates the new tables, not the deployed ones. To keep the data, make
the first migration the deployed tables, and the second the change:

1. On your branch, delete `migrations/` and commit (only if its files
   have not run against a database you keep).
2. Check out the commit that is deployed and run `alder migrate` (the
   current `alder`): it writes `0001_initial`, which creates the
   deployed tables.
3. Check out your branch again (the new files come along, untracked) and
   run `alder migrate`: `0002` takes the deployed tables to yours.
   Commit `migrations/`.

The database then adopts at `0001_initial` and applies `0002`.

**Dev mode** (`alder run`, or `ALDER_DEV=1`) treats the database as
disposable. Where production refuses because the database is not where
the files say (made outside migrations and matching none, changed by
hand, not what the files produce, or of another storage format), dev
mode drops every table, creates the app's tables again, empty, records
every file as applied, and logs why:

```text
info: recreated the database (dev mode): it was changed outside migrations
```

Every refusal that dev mode would get past says what it costs:
`ALDER_DEV=1 recreates the database empty: every row is deleted`. A
file missing, out of order or edited is refused in dev mode too: the
files, not the database, are wrong. `alder eval --run` and the REPL
never recreate the database; they say ``to start over, start `alder run`
(it recreates build/<app>/dev.db empty: every row is deleted) or delete
it`` instead.

### Tests run through migrations

`alder test` builds each test's in-memory database by applying the
migrations, so a passing run shows they make the app's tables. When the
tables changed after the last migration, every test fails with
``the migrations do not produce the app's schema: run `alder migrate` ``;
when the files themselves are refused (an edited file, say), every test
fails with the runtime's refusal, which names the file.

### Without migrations

An app without `migrations/` gets its tables from `Ensure`
(`rt/db/sqlite.go`), which records their hash and the storage format
(`db.Format`, now `1`) in `_alder_meta`. If either differs at start,
dev mode recreates the tables, empty; otherwise the app refuses:

```
db: schema mismatch: the database's schema (hash 5de59b25abca) differs from the app's (hash 40d037c4462a); changed tables: Todo; to start over, run in dev mode (ALDER_DEV=1 recreates the database empty: every row is deleted)
```

or, for a database from a runtime with an older format:

```
db: database format v0 is older than this runtime (v1); to start over, run in dev mode (ALDER_DEV=1 recreates the database empty: every row is deleted)
```

An app with migrations meets a new storage format through `alder
check`: ``the storage format changed (v1 → v2): this needs a rebuild
migration of every table, which this `alder migrate` cannot write yet
(`alder migrate --format` is planned); build with the compiler that wrote
migrations/ until then`` (E0019).


## Silos

A partition keeps one org's rows from another's pages; a **silo** keeps
them in another file. `App` lists the tables that live in a database per
value of a key (`examples/workspace/app/App.alder`):

```alder fragment
silo org { Project, Todo }
```

Each org then has a database of its own holding its projects and its
to-dos, and nothing else. Everything else (the users, the orgs, the
memberships, here also the `Activity` log, which is partitioned by `org`
but in no silo) is in the **shared** database, the one an app without
silos has. Why an app wants that: one org's rows cannot appear in
another's answer by any query, a mistake included, since they are not in
the file the request reads; an org can be backed up, restored, exported
or deleted as one file; and one big org does not slow the others' small
ones.

### The rules

- Every table in `silo org` is partitioned by `org` (E0033: "`Note` is
  in `silo org` but is not partitioned by `org`"). A table is in at most
  one silo ("`Board` is in `silo org` and `silo team`: a table lives in
  one store"), and a key has one `silo` declaration.
- A siloed table may `@ref` a shared one (a to-do's assignee, a
  `User`), and another table of its silo (`Todo.project`). A shared table
  may not `@ref` a siloed one, since its row would point into one of many
  files: "`AuditLog.project` refers to `Project`, which lives in `silo
  org`: a shared table cannot point into a silo"; nor may one silo's
  table point into another's.
- Reading or writing a siloed table needs the key narrowed: the scoped
  operations, and the unscoped ones too (`Db.from Project` reads one
  org's file, so it needs to know which): "`Db.delete Project` (`Project`
  lives in `silo org`) needs a page that `requires org`: `App` is not a
  page". There is no query over every org's rows; the admin goes through
  the orgs one at a time (chapter 15).
- `Db.update` cannot move a row to another org: "Db.update Project: a row
  cannot move to another org (delete it and insert it there)". A row is
  inserted into the org its `org` column names, which in a request must
  be the request's ("Db.insert Project: the row's org is not this
  request's").

### One store per `Task`

A `Task` runs in one transaction, and a transaction is one database's.
So **a `Task` writes to one store**: the shared database, or one silo.
The checker works out what each arm of every `update` writes, and
refuses an arm whose writes fall in two stores (E0031):

```text
`Create` writes `Project` (silo `org`) and `AuditLog` (shared): a Task writes to one store. Write `AuditLog` inside the silo, or send it from its own message.
```

What it counts is every mention of a write function — `Db.insert`,
`Db.update`, `Db.delete` and their scoped forms — anywhere the arm can
reach: in the arm, in every helper it mentions (in any module, a helper
of a helper too), in a lambda, in a `Task.all` over a `List.map`. It
does not matter whether the function is called where it is written:
`let ins = Db.insert` and then `ins AuditLog { … }`, or a lambda that is
handed `Db.insert` and applies it, write `AuditLog` all the same. The
table is read from the type the function has at that mention (a `Db`
function always names its table there: `Table t` is refused, E0003).
The count errs on the side of too many: a helper that is mentioned and
never called still counts.

That is what the checker proves: an `update` arm it accepts mentions no
write of a second store. It does not look at a `test`'s own Tasks, nor
inside a capability's adapter (Go code). There the runtime holds the
rule: a request's `Task` is bound to its arm's store before it runs, a
`Test.run`'s to the store it writes first (to its arm's, for a page's
`update`), and a write of another store fails the `Task` with an
internal error: a 500 in production, a failed test under `alder test`.

Reads may mix: an arm, or a `load`, may read the shared database and a
silo, as the workspace's projects page reads its org's name (shared)
beside its projects (the org's file). A `Session.put` writes the cookie,
not a store, and a capability is outside both, so neither counts. The
workspace keeps its `Activity` log in the shared database for this
reason: a line of activity is written with the membership it is about
(shared), never with a project. A write that must follow another in a
different store is sent from its own message; a transaction across
databases (an outbox) is a later milestone's.

### Where the files live

The shared database is the file `ALDER_DB` names, as always. Each silo
is a file next to it:

```text
<dir of ALDER_DB>/app.db                     the shared database
<dir of ALDER_DB>/silos/org/<name>.db        one org's database
```

`<name>` is the org's id written in base32hex, lowercase, without
padding (RFC 4648): the id `A` is `84.db`, the id `a` is `c4.db`. The
id's own text is never the name, since ids are case-sensitive and a file
system may not be. Only an id can name a silo (letters, digits, `-` and
`_`, at most 64 characters, the alphabet of the ids Alder makes): any
other value of `org` is not found, and makes no file. Under `alder run`
the shared database is `build/<app>/dev.db`, so the silos are under
`build/<app>/silos/`; at a server they are under `<path>/db/silos/`
(chapter 16); with `ALDER_DB=:memory:` every silo is in memory too.

A silo's file is made the first time a request needs it, and only for an
org whose row exists in the shared database: a request for an org that
was never made is not found and leaves no file. The file holds the
silo's tables and nothing else. A reference to another table of the
silo is a real foreign key (a project's `@cascade` deletes its to-dos in
that file); a reference to a shared table (`org`, an assignee) is an
indexed column without one, since the row it names is in another file.
So **deleting an org does not delete its silo**: the file stays,
unreachable, an **orphan**; `alder silos prune <env>` lists and removes
orphans (chapter 11). The shared database has the siloed tables too,
created by the same migrations, and they stay empty.

### The open-time check

Each silo's file records the schema it is at: a table
`_alder_schema` with one row, a fingerprint of the tables (as
`_alder_meta` does for the shared database) and of which tables live in
which silo, and the name of the last migration applied. The silos'
tables are in the fingerprint so that a table moved into a silo (below)
makes every silo made before the move behind, not equal: equal would
serve it without the table. A deploy migrates the shared database at start-up
("When the app starts", above), but not the thousands of silos: each is
checked when the running app first opens it, and the answer is cached
for the process. Three outcomes, besides a new file, which is created at
the app's schema:

1. **Equal**: the row is the app's. The silo is served (when the row
   names an earlier migration that changed none of the tables, the row
   is brought to the last one's name).
2. **Behind**: the row names an earlier migration of this app. The
   migrations after it are applied to the file then and there (only
   their statements on the silo's tables), checked as at start-up, and
   the silo is served, unless one of them **drops data** (its file has a
   `DROPS DATA` line). That silo is then not migrated on its own: it
   answers the maintenance page until `alder migrate silos <env>
   --allow-data-loss` applies it on purpose (chapter 16).
3. **Ahead**: the row names a migration after this app's last, written
   by a newer build (an older build was rolled back to). It is never
   touched: the silo answers the maintenance page, and `alder status`
   names the restore (`alder restore <env> --silo org:<id> --to <time>`).

A silo that is not served answers **only its org's requests** with the
app's `maintenance` page and status 503; every other org is served as
usual. `App` may declare the page (`maintenance : Html Never`, like
`notFound`); without one, a plain page says "Down for maintenance": this
part of the app is being updated. The app checks such a silo again a minute later, so a silo
fixed by `alder migrate silos` is served without a restart. `alder
status <env>` lists each key's silos by state (chapter 11).

In tests, each org a test touches has its own database in memory, made
on first use, so a test with two orgs has two (chapter 10).

### Moving a table into a silo

A silo is declared before its tables hold data. A table that the
migrations already create in the shared database has its rows there;
listing it in a `silo` later would not move them (each org's file starts
without the table's rows, and the shared rows are never read again), and
a silo made before the change would lack the table. So `alder migrate`
refuses a table that moves into a silo, out of one, or between two:

```text
alder migrate: table Activity moves into `silo org`, but the migrations create it in the shared database, and a migration does not move its rows into a database per org (book 8, "Silos"): when no database holds rows of it yet, `alder new silo org Activity --empty` declares it, writes the migration that creates it in the silos made before and says what it leaves behind; otherwise keep Activity shared
```

`migrations/schema.json` records each table's silo for that comparison
(`"silo": "org"`). `alder new silo` (chapter 15) is the deliberate way,
and for a table the migrations already create it asks you to say that
no database holds its rows: without `--empty` it refuses. With it, it
writes the declaration, marks the table in `schema.json`, and writes a
migration of the move (`0002_silo_org.sql`):

```sql
-- alder migration 0002_silo_org
-- from 1e805e781e3e
-- to 1e805e781e3e
-- step: move table Activity into silo org
-- DROPS DATA: shared rows of Activity: it moves into silo org, and the rows the shared database holds of it stay there, unread

CREATE TABLE IF NOT EXISTS "Activity__Activity" (…) STRICT;
CREATE INDEX IF NOT EXISTS "Activity__Activity__org" ON "Activity__Activity"("org");
```

It changes no table, so it leads from the schema to the same schema.
In the shared database, which has the table, its statements do nothing.
In a silo made before the move, which is behind by it (the fingerprint
above), they create the table when the silo is next opened. Its `DROPS
DATA` line is the shared database's: rows it holds of the table stay
there, never read again. So a deploy that applies it needs
`--allow-data-loss` (chapter 16), which is the point: the deploy plan
shows the move, and you say once more that nothing is lost. The line
names no silo's data, so it does not stop a silo from migrating.

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
