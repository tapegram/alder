# todos

The to-do list the book tours in chapter 2: log in, add, toggle, edit and delete your own
todos; and, for a user whose role is Admin, an admin of every table at `/admin` that `alder new
admin` generated (book 15). `alder check`, `alder build` and `alder test` take this directory;
chapter 11 explains each command.

To try it: `alder run`, then log in at `/login` with the password `password` as `a@b.c` (a
Member) or `admin@b.c` (an Admin: `/admin`). The accounts are the development stub's
(`adapters/auth/password.go`), which accepts them only under `alder run`.

## Mutation score

`alder test --mutate examples/todos` (book 10, "Mutation testing"): 188 mutants, 107 tests, 185
killed, 3 survived (score 0.98). `TestExample_TodosMutation` fails if the score drops below
0.98 or the number of mutants is not 188.

- The app's own code: 36 mutants, all killed by its 22 tests (score 1.00).
- The generated admin (`app/Admin/`, "Replaying the admin" below): 152 mutants, 149 killed by
  its generated tests, kept as `alder new admin` wrote them. The three survivors are equivalent:
  `ownersTotal > 200` flipped to `>= 200` in the `where` of the Todo form's owner list (New and
  Edit), where a search of "" matches every row anyway, and `Todo.owner.in params.ids` dropped from
  the user bulk page's load, whose `refused` is only asked of the chosen rows.

The history of the app's own code:

- Before: 11 tests, 18 killed, 14 survived of 32 (score 0.56).
- After: 20 tests, 32 killed, 0 survived (score 1.00).
- With the role (M12): the session's `role` added four mutants. Two of them, the `update field
  dropped` of the role in logout's and login's `Session.put`, survived the 20 tests; two more
  tests kill them: `App "Logout makes the role Member"` (an Admin who logs out is a Member) and
  `Auth.Login "logging in copies the role from the user's row"`.

The ownership rules are declared (book 06, "Rules"): `Todo.owner` is the table's `@owner`, and
the pages use `Db.mine`, `Db.getMine`, `Db.updateMine`, `Db.deleteMine` and `Db.insertMine`.
Their four `scope dropped` mutants (each scoped operation made unscoped) are killed by the
ownership tests among the eleven: `Todos.Index "a stranger cannot see my todo"` (the list's
`Db.mine` and `Todos.Show`'s `Db.getMine`), `Todos.Index "Toggle by another user is not found"`
and `Todos.Index "Delete by another user is not found"` (a stranger presses my todo's button on
a page still open in the browser). `Db.insertMine` has no `scope dropped` mutant: the owner
comes from the session either way.

The nine tests that closed the gaps, each named for what it kills:

- `App "Logout clears the user and goes to log in"`: logout's `Redirect → Rerender`. Every Log
  out button of this app is on a page that `requires user`, so in the browser a Rerender of that
  page also ends at the login page. Only a test of `App.update` itself tells the two apart.
- `Todo "countOpen ignores done todos"` (now two open todos and one done, so `not` dropped gives
  1, not 2), `Todo "Active keeps only open todos"` and `Todo "each filter has its label"`: the
  swapped `match` arms and the dropped `not` of `matches` and `filterLabel`.
- `Todo "a user with todos cannot be deleted"`: `owner @ref User` changed to `@cascade`.
- `Todos.Index "Delete removes the todo"`: the delete replaced by `Task.succeed ()` (now also
  killed by `"Delete by another user is not found"`, which expects a 404 and gets a Rerender).
- `Todos.Index "the newest todo is listed first"`: `Db.orderBy` reversed.
- `Todos.Show "a visitor is sent to log in from a todo"`: `requires user` dropped.
- `Todos.Show "saving changes the title and the due date"`: the save replaced, each updated
  field dropped, and the redirect after it.
- `Todos.Show "deleting from the todo's page"`: that page's delete, and its redirect.

No mutant survives. The run also refuses 3 mutants as provably equivalent (the `scope dropped`
of `Todos.Show`'s `Db.updateMine` and `Db.deleteMine` of `model.todo.id`, which its `load` read
with `Db.getMine`, and `User.role`'s default, which no row of the app's code leaves out) and
skips 25 that do not apply (5 in the app's own code, 20 in the admin); neither count is part of
the score.

## Replaying the admin

Every file under `app/Admin/` is exactly what `alder new admin` wrote; none was edited by hand.
The steps, from the app before the admin:

1. `User` gains `type Role = Member | Admin` and a column `role: Role = Member`; `App`'s
   session gains `role: User.Role = Member`; `Auth.Login` copies the row's role into the
   session, and `App.Logout` puts `Member` back.
2. `migrations/0001_initial.sql` is written again from the tables (`rm migrations/*` and
   `alder migrate`): an example keeps one migration, since it has no data to keep. An app with
   data runs `alder migrate` instead, which rebuilds `User` with the new column. A `todos.db`
   that `alder run` made before this step no longer matches `0001`, and the app refuses to
   start on it: delete it, or run once with `ALDER_DEV=1`, which recreates the database empty
   (book 8).
3. `alder new admin` writes the admin:

   ```
   $ alder new admin
   wrote app/Admin/Index.alder
   wrote app/Admin/Layout.alder
   wrote app/Admin/Todo/Bulk.alder
   wrote app/Admin/Todo/Delete.alder
   wrote app/Admin/Todo/Edit.alder
   wrote app/Admin/Todo/Index.alder
   wrote app/Admin/Todo/New.alder
   wrote app/Admin/Todo/Show.alder
   wrote app/Admin/User/Bulk.alder
   wrote app/Admin/User/Delete.alder
   wrote app/Admin/User/Edit.alder
   wrote app/Admin/User/Index.alder
   wrote app/Admin/User/New.alder
   wrote app/Admin/User/Show.alder
   ```

4. The development login (`adapters/auth/password.go`) gains `admin@b.c`, an Admin.

An Admin cannot delete their own account or change their own role there, and each list filters
by its `Bool` and union columns (`/admin/todo?done=yes`, `/admin/user?role=Admin`; book 15).

To check that the committed admin is still what the generator writes, in a copy of the app:
`rm -r app/Admin && alder new admin`, then `git diff --stat` shows nothing. After a change to a
table, `alder regen admin` rewrites only the admin's owned regions (book 15).
