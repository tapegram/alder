# 8. Database

## Tables

A table is declared with `table`. It defines a record type of the same name
and the storage for it. This is the to-do app's, an excerpt of
`app/Todo.alder` (chapter 2 shows, and the verifier checks, the whole
module):

```alder fragment
-- in Todo
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
  `T`'s id type, or `Maybe` of it.
- `@created` is set to the current time on insert. `@updated` is set on
  every write.
- A field with a default may be omitted on insert.
- `@unique` marks a field that must be unique.
- Field types: `Int`, `Float`, `Bool`, `Text`, `Date`, `Time`, brands of
  those, unions with no payloads, and `Maybe` of any of those. Nothing
  nested; model nesting with a second table and `@ref`.

A table is declared in its own module named after it (`Todo` in
`app/Todo.alder`), together with the brand for its id and any functions
that belong to it.

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
`Todo.owner: no User with that id`; deleting a row that another row still
refers to fails with `User QXZMRYnkC2vuE_TefkuR is referenced by another
row` (the table and the id). Either way the task's earlier writes are
rolled back.

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
`@unique` adds `UNIQUE`, `@ref` adds `REFERENCES`, and a `Bool` or union
column gets a `CHECK` listing its allowed values. For the to-do example's
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
- **`.contains`** ignores case for the letters A–Z only; `É` and `é`
  differ.
- **Empty lists.** `Db.and []` matches every row, `Db.or []` matches no
  row, and `.in []` matches no row.

Every value reaches SQLite as a parameter, never as part of the SQL text,
so no input can change what a query does.

## Schema changes

When an app starts, the runtime creates the tables (`Ensure` in
`rt/db/sqlite.go`): the generated `Serve` calls it before it serves the
first request, and so do tests (`TestGen_RunTodos` in
`internal/gen/gen_test.go`). It keeps two things in a table of its own, `_alder_meta`: a
hash of the `CREATE TABLE` statements, sorted so that the order of your
`table` declarations does not matter, and the storage format version of
the runtime that created the database (`db.Format`, now `1`, raised
whenever the runtime changes how a table is laid out). If both match,
nothing happens. If your tables changed since the database was created, or
it was created by a runtime with another format:

- **In dev mode** every table in the database is dropped, except
  SQLite's own (`sqlite_*`) and `_alder_meta`, and the app's tables are
  created again, empty. That includes tables your Go capability code
  created itself. Your dev data is lost on every schema change; that is
  the price of not writing migrations while the schema is still moving.
- **Otherwise** the app refuses to start and changes none of your tables
  (only `_alder_meta` is created, if it was missing). The error names the
  tables that changed:

```
db: schema mismatch: the database's schema (hash 5de59b25abca) differs from the app's (hash 40d037c4462a); changed tables: Todo.Todo; run in dev mode to recreate the database
```

or, for a database from a runtime with an older format:

```
db: database format v0 is older than this runtime (v1); run in dev mode to recreate the database
```

Migrations, which would keep the data across a change, are planned.

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
