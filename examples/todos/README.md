# todos

The to-do list the book tours in chapter 2: log in, add, toggle, edit and delete your own
todos; and, for a user whose role is Admin, an admin of every table at `/admin` that `alder new
admin` generated (book 15). `alder check`, `alder build` and `alder test` take this directory;
chapter 11 explains each command.

To try it: `alder run`, then log in at `/login` with the password `password` as `a@b.c` (a
Member) or `admin@b.c` (an Admin: `/admin`). The accounts are the development stub's
(`adapters/auth/password.go`), which accepts them only under `alder run`.

## Layout

The tables (Todo and User) live under `app/Table/`, one module each (`module Table.Todo` and `Table.User`), and route modules `import Table.<Name>`. The admin's folders are plural (`app/Admin/Todos/` and so on); its URLs are not (`/admin/todo`). `migrations/0002_rename_todo_rename_user.sql` renames the tables from the old root modules to these; it moves no data.

## Deploying

`App` declares one environment, `local`, a `Script` target (book 16, "The Script hatch"):

```
alder deploy local
```

runs the whole gate (check, build, the tests, the mutation score), writes the artifact
`build/todos/deploy/<build id>/` (the executable, `static/`, `migrations/`,
`alder-deploy.json`) and runs `scripts/deploy-local.sh` with it. The script only records what
it was given, in `build/todos/deploy/<build id>.deploy-local.txt`, so the example deploys with
nothing installed; `go test ./book` runs this deploy for chapter 16.

To put it on a server, add an environment to the declaration in `app/App.alder`, such as
`production: Ssh "deploy@box.example.com" { domain: "todos.example.com", path: "/srv/todos" }`,
write `ALDER_SECRET=…` (16 bytes or more) into `/srv/todos/env` on the box, and run `alder
deploy production`;

`alder ci init` writes the GitHub workflow that does it on every push to
`main`.

The example's login is the development stub, which refuses every password outside
`alder run`: a real deployment needs a real `Password` adapter (book 7).

## Mutation score

`alder test --mutate examples/todos` (book 10, "Mutation testing"): 236 mutants, 147 tests, 230
killed, 6 survived (score 0.97). `TestExample_TodosMutation` fails if the score drops below
0.97, the exact 0.974 rounded down, or the number of mutants is not 236.

- **The app's own code:** 45 mutants, all killed by its 25 tests (score 1.00).

- **The generated admin** (`app/Admin/`, "Replaying the admin" below): 191 mutants, 185 killed by
  its 122 generated tests, kept as `alder new admin` wrote them.

  Two survivors are equivalent:

  - `ownersTotal > 200` flipped to `>= 200` in the `where` of the New route's owner list, where a
    search of "" matches every row anyway.
  - `Todo.owner.in ids` dropped from the user bulk route's `load`, whose `refused` is only asked
    of the chosen rows.

  > **The Edit route.** The Edit route's list no longer has the first: its row's own owner is
  > read by itself and comes first, and the search leaves it out, so the generated test of
  > exactly 200 users tells `>` from `>=`.

  The four others are the `step dropped` of the `adminOnly` that begins `Create` and `Save`, and
  the `role dropped` of the New and Edit pages' `SearchOwner` routes:

The `step dropped` of the `adminOnly` that begins `Create` and `Save` on a form with a select of
another table's rows is not a gap: a demoted Admin's post of that form is refused (400) by the
options check before the update runs, since the page it was posted from is rendered again to
check the options, and that page's `load` begins with `adminOnly` too. Nor is the `role dropped`
of a `Search…` route, which only redirects to the page it was posted from: another role's search
is redirected there, or, when it does not decode, that page is rendered again with the errors,
and either way that page's own role check answers the 404 the search's would. (The search's
`requires dropped` is killed: the generated test of a long list sends a visitor's search, which
must go to log in, where the mutant's role check answers 404.)

The history of the app's own code:

- Before: 11 tests, 18 killed, 14 survived of 32 (score 0.56).
- After: 20 tests, 32 killed, 0 survived (score 1.00).
- With the role (M12): the session's `role` added four mutants.

  Two of them, the `update field dropped` of the role in logout's and login's `Session.put`,
  survived the 20 tests; two more tests kill them: `App "Logout makes the role Member"` (an Admin who logs out is a Member) and
  `Auth.Login "logging in copies the role from the user's row"`.
- With the route operators (alder#187): a route's `requires`, its `Response` answers and its
  `let _ <-` steps added 15 mutants. The five `requires dropped` of the write routes survived (no
  `load` runs before a write, so its own `requires user` is the check, and no test sent a write
  without a session); two tests kill them: `Todos.Index "a write after logging out is sent to log
  in"` and `Todos.Show "saving or deleting after logging out is sent to log in"`, which open the
  page, log out (`Test.login { user: Nothing }`) and send each write from the page still held.
  The other ten were killed by tests that were there: the GET routes' `requires dropped` by the
  visitor tests, and the answers (`redirect → refresh`, `refresh → invalid`, `invalid → refresh`)
  by the tests that check what a write answers.

The ownership rules are declared (book 06, "Rules"): `Todo.owner` is the table's `@owner`, and
the routes use `Db.mine`, `Db.getMine`, `Db.updateMine`, `Db.deleteMine` and `Db.insertMine`.

Their four `scope dropped` mutants (each scoped operation made unscoped) are killed by the
ownership tests among the eleven: `Todos.Index "a stranger cannot see my todo"` (the list's
`Db.mine` and `Todos.Show`'s `Db.getMine`), `Todos.Index "Toggle by another user is not found"`
and `Todos.Index "Delete by another user is not found"` (a stranger presses my todo's button on
a page still open in the browser).

> **`Db.insertMine`.** It has no `scope dropped` mutant: the owner comes from the session either way.

On routes (#160) no `load` runs before a write, so the
`scope dropped` of `Todos.Show`'s Save and Delete (`Db.updateMine` and `Db.deleteMine` of the
id in their path) are no longer equivalent: `Todos.Show "saving or deleting another user's todo
is not found"` kills them.

The nine tests that closed the gaps, each named for what it kills:

- `App "Logout clears the user and goes to log in"`: the `redirect → refresh` of
  `Logout.update`, which no browser test can kill (every Log out button is on a page that
  `requires user`, so a refresh ends at the login page too).
- `Todo "countOpen ignores done todos"` (now two open todos and one done, so `not` dropped gives
  1, not 2), `Todo "Active keeps only open todos"` and `Todo "each filter has its label"`: the
  swapped `match` arms and the dropped `not` of `matches` and `filterLabel`.
- `Todo "a user with todos cannot be deleted"`: `owner @ref User` changed to `@cascade`.
- `Todos.Index "Delete removes the todo"`: the delete replaced by `Task.succeed ()` (now also
  killed by `"Delete by another user is not found"`, which expects a 404).
- `Todos.Index "the newest todo is listed first"`: `Db.orderBy` reversed.
- `Todos.Show "a visitor is sent to log in from a todo"`: the `requires dropped` of the `Show`
  route.
- `Todos.Show "saving changes the title and the due date"`: the save replaced, and each updated
  field dropped.
- `Todos.Show "deleting from the todo's page"`: that route's delete.

No mutant of the app's own code survives. The run also refuses 1 mutant as provably equivalent
(`User.role`'s default, which no row of the app's code leaves out) and skips 38 that do not
apply (most of them a `step dropped` of a write or a `Session.put`, which `write replaced` and
`session put dropped` make); neither count is part of the score.

## Replaying the admin

Every file under `app/Admin/` is exactly what `alder new admin` wrote; none was edited by hand.
The steps, from the app before the admin:

1. `User` gains `type Role = Member | Admin` and a column `role: Role = Member`; `App`'s
   session gains `role: User.Role = Member`; `Auth.Login` copies the row's role into the
   session, and `App.Logout` puts `Member` back.
2. `migrations/0001_initial.sql` was written again from the tables (`rm migrations/*` and
   `alder migrate`) at this step: the example had no data to keep, so it kept one migration.

   The tables have since moved into `app/Table/` (`0002_rename_*`, see Layout above), so do not
   `rm migrations/*` now: it would delete that rename. An app with
   data runs `alder migrate` instead, which rebuilds `User` with the new column.

   A `todos.db`
   that `alder run` made before this step no longer matches `0001`, and the app refuses to
   start on it: delete it, or run once with `ALDER_DEV=1`, which recreates the database empty
   (book 8).
3. `alder new admin` writes the admin:

   ```
   $ alder new admin
   wrote app/Admin/Index.alder
   wrote app/Admin/Layout.alder
   wrote app/Admin/Todos/Bulk.alder
   wrote app/Admin/Todos/Delete.alder
   wrote app/Admin/Todos/Edit.alder
   wrote app/Admin/Todos/Index.alder
   wrote app/Admin/Todos/New.alder
   wrote app/Admin/Todos/Show.alder
   wrote app/Admin/Users/Bulk.alder
   wrote app/Admin/Users/Delete.alder
   wrote app/Admin/Users/Edit.alder
   wrote app/Admin/Users/Index.alder
   wrote app/Admin/Users/New.alder
   wrote app/Admin/Users/Show.alder
   ```

4. The development login (`adapters/auth/password.go`) gains `admin@b.c`, an Admin.

An Admin cannot delete their own account or change their own role there, and each list filters
by its `Bool` and union columns (`/admin/todo?done=yes`, `/admin/user?role=Admin`; book 15).

To check that the committed admin is still what the generator writes, in a copy of the app:
`rm -r app/Admin && alder new admin`, then `git diff --stat` shows nothing. After a change to a
table, `alder regen admin` rewrites only the admin's owned regions (book 15).
