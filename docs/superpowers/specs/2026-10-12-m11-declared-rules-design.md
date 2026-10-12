# M11: Declared rules — ownership and roles — Design

Date: 2026-10-12. Status: approved in conversation; the M11 plan argues from
this. Related: chapter 6 ("Authorize in update"), the examples' `owned`
helpers and `author` lookups, E9 (authorization mutants are the ones tests
most often miss), the Turso-ready note in the M3 spec.

## Goal

The two authorization checks every Alder app writes by hand, "this row is
mine" and "this user has role R", become declarations the checker
enforces, the shortest spelling is the correct one, the mutation
operators target them precisely, and the generators (M12) emit them.

## Decisions recorded (2026-10-12)

- **Ownership lives on the table, used by explicit scoped operations**
  (user chose A): one `@owner` column per table; `Db.mine`, `Db.getMine`,
  `Db.updateMine`, `Db.deleteMine`. No page-level `owns` sugar (maybe
  later).
- **Roles live in the session** (C): `session { …, role: Role = Reader }`
  set at login; `requires user, role Author` is a cookie check; a role
  change applies at the next login; the re-read idiom covers the rare
  page where that matters.
- **A scoped read or write of a row that is not yours answers not-found**
  (B): one line, no arm to drop. A page wanting a different outcome uses
  the unscoped operation and writes its check.
- **Turso**: scoped reads are predicates (replica-friendly); role checks
  never read the database; scoped writes carry the predicate in the
  UPDATE/DELETE (atomic on the primary, closing the read-then-write race
  of hand-written checks). Tenant selection is a later, separate layer.

## The surface

### `@owner`

```alder
table Todo {
  id: TodoId @id,
  title: Text.NonEmpty,
  owner: User.UserId @ref User @owner,
  created: Time @created,
}
```

- `@owner` marks exactly one column of a table; it must be a `@ref` to the
  table whose id type is the session's user id type (`session { user:
  Maybe User.UserId }` → the column's type is `User.UserId`). E0024
  otherwise: "`@owner` must mark a `@ref` column of the session's user
  type `User.UserId`; `price` is `Int`" / "a table has one `@owner`
  column: `Todo` marks `owner` and `editor`" / "`@owner` needs a session
  with a `user` field: this app's session has none".
- A table without `@owner` has no scoped operations (E0025 "`Todo` has no
  `@owner` column, so `Db.mine Todo` has nothing to scope by").

### Scoped operations (stdlib `Db`)

```
mine       : Table t -> Select t            -- Db.from t |> where (owner.eq session.user)
getMine    : Table t -> Id t -> Query t      -- the row, when mine; else Query.notFound
updateMine : Table t -> Id t -> (t -> t) -> Task Unit   -- WHERE id = ? AND owner = ?; 0 rows → Task.notFound
deleteMine : Table t -> Id t -> Task Unit   -- likewise
insertMine : Table t -> Insert t -> Task (Id t)  -- the owner column filled from the session (an Insert that names `owner` is E0003: "`owner` is set by `Db.insertMine`")
```

- Each compiles only where `session.user` is narrowed to `User.UserId`:
  inside a page module whose page `requires user` (its `load`, `update`
  and helper values). Elsewhere (a page without `requires user`, `App`'s
  `update`, a table module) it is E0026: "`Db.mine` needs a page that
  `requires user`: `Posts.Index` does not".
- `getMine` is a Query (usable in `load` and, as a Query inside a Task,
  in `update`); `updateMine`/`deleteMine`/`insertMine` are Tasks.
- Traces name them as written: `Db.mine Todo`, `Db.getMine Todo`,
  `Db.updateMine Todo`.
- `Db.mine T` is a `Select` so it composes: `Db.mine Todo |> Db.orderBy …
  |> Db.all`.

### Roles

```alder
module App
session { user: Maybe User.UserId, role: User.Role = Reader }
```

```alder
page "/admin" { } requires user, role Author
```

- `requires user, role R`: `R` is a constructor of the session's `role`
  field's union; a request whose session lacks `user` is redirected to
  the login page as today; one whose role is not `R` answers 404 (the
  not-found document), like a missing row. `requires role R` without
  `user` is E0027 ("`role` needs `user` first"). Several roles:
  `requires user, role Author | Admin`.
- The session field is named `role` by convention; `requires … role`
  looks for a session field of a union type named `role` (E0027 "this
  session has no `role` field").
- Login sets it: `Session.put (\s -> { s | user: Just id, role: u.role })`;
  chapter 6 shows the idiom and states that a change of role applies at
  the next login, with the re-read idiom (`let u <- Db.get User …` in
  `load`) for pages that must not be stale.
- The page's narrowed `session.role` is the declared role's type (the
  union), as `session.user` is narrowed today.

### Mutation operators (M9 extension)

- `scope dropped`: `Db.mine T` → `Db.from T`; `Db.getMine` → `Db.get` +
  the Maybe unwrapped (replacement: the row whatever its owner, not
  found only when missing); `updateMine`/`deleteMine` → the unscoped
  write; `insertMine` → an insert with the session's user (same; skipped).
- `role dropped`: `requires user, role R` → `requires user`.
- Both named in the survivors list like the others.

### Compiler

- Parser: `@owner` attribute; `requires user, role R [| S]`.
- Checker: E0024–E0027; the narrowing of `session.role`; the scoped
  operations' typing (`Table t` with an owner → the session's user type
  matches); `insertMine`'s forbidden field.
- IR: `Table.Owner string`; `Page.Roles []string`; `DbMine`, `DbGetMine`,
  `DbUpdateMine`, `DbDeleteMine`, `DbInsertMine` (or flags on the
  existing nodes — implementer's call, said in the report).
- gen: the predicate in SQL (`WHERE owner = ?` with the session's user);
  `updateMine`/`deleteMine` as one statement with `AND owner = ?` and a
  rows-affected check → `eff.ErrNotFound`; the route's role check
  (before `load`, after the `requires user` redirect).
- Runtime: `db.UpdateWhere`/`DeleteWhere` with an owner predicate
  returning rows affected (or the existing update path extended).
- Migrations: `@owner` changes nothing in the schema (it is a marker on a
  `@ref` column that already exists); `schema.json` records it? No: the
  marker is not part of the stored schema; `migrate` ignores it.

### Examples and book

- todos: `owner @owner`; `Index.load` uses `Db.mine Todo`; `owned`
  disappears: `Toggle`/`Delete` use `Db.updateMine`/`deleteMine`;
  `Show.load` uses `Db.getMine`; tests unchanged in meaning (the
  stranger's click is still 404), mutation score stays 1.00 (the
  `scope dropped` mutants replace the guard-dropped ones).
- blog: `author @owner` on `Post`; `role: User.Role = Reader` in the
  session; `Admin` and `Admin.Edit` `requires user, role Author`; the
  `author` lookup disappears; `Admin.Edit.load` uses `Db.getMine Post`.
- Chapter 6 ("Authorize in update" becomes "Rules": ownership, roles, the
  not-found answer, when to use the unscoped forms, the re-read idiom,
  Turso note), chapter 8 (`@owner`), chapter 9 (`Db` additions), chapter
  10 (the two new operators; the todos survivors list re-recorded),
  chapter 11 (E0024–E0027), chapter 13 (the SQL, the rows-affected
  check), chapter 12 (why declared; why not-found; why the session for
  roles; the replica argument), chapter 14 (the blog's pages), the
  reference (the rules and the five operations).
- The E8/E9 hidden tests and references are historical; `experiments/
  tasks/` texts may mention the operations in the Names block for E10.

## Testing

- Corpus: E0024 (three texts), E0025, E0026, E0027 (two texts),
  `insertMine` naming `owner`; `rules-ok` with every operation and both
  roles forms.
- rt/db: `UpdateWhere`/`DeleteWhere` rows-affected; a scoped update of
  another user's row changes nothing and reports 0.
- gen golden `gen_rules`; e2e: a fixture app with ownership + roles
  under `alder test` (browser tests: the stranger's `Test.open` is
  `PageMissing`; a `Reader` on an Author page is `PageMissing`; a
  visitor is `Redirected` to login; a stale-page click on another's row
  is not found and changes nothing).
- Mutation: `scope dropped` and `role dropped` goldens; the examples'
  scores re-recorded.

## Non-goals

- Page-level `owns` sugar; membership rules through a join table
  (hand-written, or `Db.mine` on the join table); database-read roles;
  tenant selection; row-level rules beyond one owner column; the admin.
