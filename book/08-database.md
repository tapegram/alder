# 8. Database

## Tables

A table is declared with `table`. It defines a record type of the same name
and the storage for it.

```alder
table Todo {
  id: TodoId @id,
  title: Text.NonEmpty,
  done: Bool = False,
  owner: UserId @ref User,
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

```alder
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
Todo.title.eq   : Text -> Condition Todo         -- also .ne
Todo.created.lt : Time -> Condition Todo         -- also .le .gt .ge
Todo.title.contains : Text -> Condition Todo     -- substring, case-insensitive
Todo.owner.in   : List UserId -> Condition Todo
Todo.due.isNothing : Condition Todo              -- for Maybe fields; also .isJust
Todo.created.asc, Todo.created.desc : Order Todo

Db.and : List (Condition t) -> Condition t
Db.or  : List (Condition t) -> Condition t
Db.not : Condition t -> Condition t
```

Several `Db.where` calls are combined with `and`.

## Writing

Writes are `Task` values.

```alder
Db.insert : Table t -> Insert t -> Task (Id t)
Db.update : Table t -> Id t -> (t -> t) -> Task Unit
Db.delete : Table t -> Id t -> Task Unit
```

`Insert t` is the table record without `@id`, `@created`, and `@updated`
fields, and with defaulted fields optional. `Db.update` reads the row,
applies the function, and writes it back; updating a row that doesn't exist
fails the task. The function may not change the `@id` field.

```alder
let id <- Db.insert Todo { title: p.form.title, owner: session.user }
let _ <- Db.update Todo id (\t -> { t | done: True })
let _ <- Db.delete Todo id
```

Writes in one `update` run in a single transaction. If the task fails,
nothing is written.

## Migrations

The compiler keeps a snapshot of the schema in `db/schema.snapshot`. When
the tables change, `alder build` writes a migration to `db/migrations/` with
the SQL to go from the snapshot to the new schema, and updates the snapshot.
Migrations run on startup. A change the compiler cannot migrate safely
(changing a column's type, removing a `Maybe`) is a build error until the
migration is edited by hand.

## Beyond CRUD

Joins, aggregates, and full-text search are not in the language. Declare a
capability and implement it in Go against the same database:

```alder
capability Reports {
  todosPerUser : Query (List { user: UserId, count: Int }),
}
```
