# 10. Testing

A test is a piece of the program that checks another piece and says yes or
no. In Alder a test is a declaration, written in the module it tests, after
the code, and `alder test` runs every test in the app. This chapter starts
with the simplest kind, a test of a pure function, then reads the report
`alder test` prints, then tests code that reads and writes the database,
replaces a capability with a fake written in Alder, drives the app
through an in-memory browser as a user would, measures the tests with
mutants (`alder test --mutate`), checks statements about all inputs
instead of examples (properties, section 12), and ends with what cannot
be tested yet.

## 1. A test is a `Bool`

`test "name" = expression` declares a test. When its body is not a block,
the body must be a `Bool`: `True` means the test passed. The to-do app's
`Todo` module (`examples/todos/app/Table/Todo.alder`) ends with one, which
checks that `countOpen` counts only the todos that are not done:

```alder
test "countOpen ignores done todos" =
  Todo.countOpen [
    { id: Todo.TodoId "a", title: Text.NonEmpty "x", done: False, due: Nothing, owner: User.UserId "u", created: Time.epoch },
    { id: Todo.TodoId "b", title: Text.NonEmpty "y", done: True, due: Nothing, owner: User.UserId "u", created: Time.epoch },
    { id: Todo.TodoId "c", title: Text.NonEmpty "z", done: False, due: Nothing, owner: User.UserId "u", created: Time.epoch },
  ] == 2
```

Inside `Todo` the names are written unqualified (`countOpen`,
`TodoId "a"`), and that is how the file has them; the blocks of this
chapter are checked as a module of their own, so they qualify them as any
other module must. A test of another function looks the same. This one,
not in the example, checks that the `Active` filter leaves out a done
todo:

```alder
test "matches Active excludes done" =
  not (Todo.matches Active { id: Todo.TodoId "a", title: Text.NonEmpty "x", done: True, due: Nothing, owner: User.UserId "u", created: Time.epoch })
```

Two rules are particular to tests:

- A test may use any module's brand constructors. Outside `Todo`, nothing
  else may write `Todo.TodoId "a"`: a `TodoId` must come from a route, a
  form or a row (chapter 4). A test needs to make up values, so it may.
- A test has no name you can call. It is not a value; only `alder test`
  runs it.

Because a route's `load`, `view` and `update` are functions that return
descriptions, much of a route can be tested this way. `view` returns HTML,
and two HTML values are equal when they render to the same markup:

```alder eval=True
<p>{Html.text "a"}</p> == <p>a</p>
```

The comparison is of the markup, not of how the values were built: two
text pieces side by side are the same markup as one piece holding both,

```alder eval=True
<p>{Html.text "a"}{Html.text "b"}</p> == <p>ab</p>
```

so a test of `view` passes exactly when the two sides it prints are the
same text.

## 2. Running the tests

`alder test` checks the app, compiles it with its tests, runs every test
and prints one line per test. In the repository, the to-do app has 126:
its own 23, and the 103 of the admin that `alder new admin` generated for
it (chapter 15). `-run` (section 7) picks out the app's own, by module:

```text
$ alder test examples/todos -run '^(App|Auth|Table\.Todo|Todos)'
ok   Table.Todo "countOpen ignores done todos"
ok   Table.Todo "Active keeps only open todos"
ok   Table.Todo "each filter has its label"
ok   Table.Todo "a user with todos cannot be deleted"
ok   App "Logout clears the user and goes to log in"
ok   App "Logout makes the role Member"
ok   Auth.Login "an unknown email is rejected"
ok   Auth.Login "logging in through the form"
ok   Auth.Login "logging in copies the role from the user's row"
ok   Auth.Login "a visitor is sent to log in, and can log out"
ok   Todos.Index "Toggle flips done"
ok   Todos.Index "Toggle by another user is not found"
ok   Todos.Index "Delete by another user is not found"
ok   Todos.Index "adding a todo through the page"
ok   Todos.Index "toggling a todo through the page"
ok   Todos.Index "a stranger cannot see my todo"
ok   Todos.Index "Delete removes the todo"
ok   Todos.Index "the newest todo is listed first"
ok   Todos.Index "Db.mine is what Me sees"
ok   Todos.Show "a visitor is sent to log in from a todo"
ok   Todos.Show "saving changes the title and the due date"
ok   Todos.Show "deleting from the todo's page"
ok   Todos.Show "saving or deleting another user's todo is not found"
23 passed, 0 failed
```

Each line names the module and the test, and is printed as soon as that
test finishes, so a slow test shows which tests ran before it. Tests run
in module order (a module after the modules it imports), and within a
module in the order they are written. The last line counts them.
`alder test` exits with 0 when every test passed and 1 when one failed,
so a script or CI job can use it directly. Two tests of one module may
not have the same name (E0009).

Here is the same app with two wrong tests added: one at the end of
`Table.Todo`,

```alder
test "an empty list has one open todo" = Todo.countOpen [] == 1
```

and one at the end of `Todos.Index` that expects the `Toggle` route's
`update` to read the todo and then write it, in two steps (sections 3 to 6
explain effect tests and traces):

```alder
test "Toggle reads, then writes" = {
  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
  let id <- Db.insert Todo { title: Text.NonEmpty "Milk", due: Nothing, owner: me }
  let _ <- Test.login { user: Just me }
  let r <- Test.run (Index.Toggle.update { id: id } { user: me, role: Member })
  Task.succeed (r.trace == ["Db.get Todo", "Db.update Todo"])
}
```

In their modules they are written without the `Todo.` and `Index.`
prefixes (`Toggle.update`). Both compile; both are wrong:

```text
$ alder test examples/todos -run '^(App|Auth|Table\.Todo|Todos)'
ok   Table.Todo "countOpen ignores done todos"
ok   Table.Todo "Active keeps only open todos"
ok   Table.Todo "each filter has its label"
ok   Table.Todo "a user with todos cannot be deleted"
FAIL Table.Todo "an empty list has one open todo"
  left:  0
  right: 1
ok   App "Logout clears the user and goes to log in"
ok   App "Logout makes the role Member"
ok   Auth.Login "an unknown email is rejected"
ok   Auth.Login "logging in through the form"
ok   Auth.Login "logging in copies the role from the user's row"
ok   Auth.Login "a visitor is sent to log in, and can log out"
ok   Todos.Index "Toggle flips done"
ok   Todos.Index "Toggle by another user is not found"
ok   Todos.Index "Delete by another user is not found"
ok   Todos.Index "adding a todo through the page"
ok   Todos.Index "toggling a todo through the page"
ok   Todos.Index "a stranger cannot see my todo"
ok   Todos.Index "Delete removes the todo"
ok   Todos.Index "the newest todo is listed first"
ok   Todos.Index "Db.mine is what Me sees"
FAIL Todos.Index "Toggle reads, then writes"
  left:  ["Db.updateMine Todo"]
  right: ["Db.get Todo", "Db.update Todo"]
  trace: ["Db.insert User", "Db.insert Todo", "Test.login", "Test.run", "Db.updateMine Todo"]
ok   Todos.Show "a visitor is sent to log in from a todo"
ok   Todos.Show "saving changes the title and the due date"
ok   Todos.Show "deleting from the todo's page"
ok   Todos.Show "saving or deleting another user's todo is not found"
23 passed, 2 failed
```

A failing test is `FAIL`, followed by indented lines that say why:

- `left:` and `right:` are the two sides of the test's `==`, written as
  Alder values (as `alder eval` writes them). They appear when the test's
  body is an `==`, or, for a block (section 3), when the value its final
  `Task.succeed` returns is an `==`. A body such as `a == b && c == d`
  has no single `==` to show, so its failure has no sides: when you
  want the sides, end a test with one `==`, comparing records if there
  is more than one thing to check.
- `note:` appears under the sides when they print the same text but
  `==` still said `False`: `the sides print the same but are different
  values`. A `Float` that is not a number (`0.0 / 0.0` prints as `NaN`
  and is not equal to itself) is one way to get there.
- `error:` is why the test stopped before it produced `True` or `False`:
  a task that failed, a capability with no implementation, a Go panic.
  Two more stop the whole run at that test, which then fails with the
  reason and no later test runs (exit 1): a test that ran past the time
  limit (`error: timed out after 5m0s`; `-timeout`, section 7), and a
  test that crashed the program, such as a function that recurses
  forever and overflows the stack: the error is the first lines of the
  crash, `fatal error: stack overflow` and the Go function and line it
  happened in, not the whole Go trace.
- `trace:` is, for a block test, every step it performed (section 6).

A test that passes or fails can be followed by `warn:` lines, which never
fail it. Each is a statement a page ran in a loop during one of the test's
browser steps (section 9): five or more runs of one SQL statement in one
request, the mark of a `Db.get` per row (an "N+1": one query for the list,
then N more, one per row):

```text
ok   Rows.Bench "slow 100"
  warn: GET /slow: N+1: SELECT "id", "title", … FROM "Table__Row__Row" WHERE "id" = ? ran 100 times (100 argument sets, 0.29 ms): one Db.get per row; read them together with Db.where (Row.id.in ids), or in the page's first query
```

On a file database the loop is cheap; on a remote one (Turso, chapter 15)
every statement is a round trip over the network, so the page that answers
in a millisecond here takes a second there. The rule is the one the
developer panel's SQL tab applies (chapter 11, "Developer mode", says how
it is decided), run over each request a browser step made; the request
line in front names which. With `--json` the test's object carries
`"warnings": […]`. A warning is not a failure because the page may be
right as it is (a handful of rows, a lookup you accept); the line is there
so the cost is seen before it is paid in production.

A test that runs for ten seconds is announced on standard error,
`still running Todo "slow" (10s)`, and again every ten seconds, so a
test that hangs is not mistaken for a quiet one.

If the app does not compile, no test runs: `alder test` prints the
diagnostics as `alder check` does and exits 1. A typed hole (`?todo`,
chapter 11) compiles to nothing that can run either: its warning W0002 is
followed by `alder test: no tests ran (fill the hole)` and the holes with their types, exit 1. Here the app has gained a
module `Probe` that declares `count : Int` as `count = "none"` and
`test "one" = { Task.succeed 1 }`, a block whose verdict is not a
`Bool`:

```text
$ alder test examples/todos
app/Probe.alder:4:9: error[E0003]: expected Int, got Text
  count = "none"
          ^^^^^^
app/Probe.alder:6:29: error[E0003]: expected Bool, got Int
  test "one" = { Task.succeed 1 }
                              ^
```

## 3. Testing effects

A `Task` cannot be compared with `==` (section 13 says why), so a test of
`update` cannot just compare its result. Instead it runs the task and
looks at what happened. A test whose body is a block, `{ … }`, is an
**effect test**: the block is a `Task Bool`, with `let x <-` lines like
any `Task` (chapter 7), and it ends with `Task.succeed` of the verdict. A
block that ends in a plain `Bool` is an error; the compiler's hint says
to end it with `Task.succeed (…)`.

The to-do list's tests are the last thing in `app/Todos/Index.alder`
(chapter 2 shows them in place). Written in another module, as the
blocks of this chapter are, the route module's declarations are named
through its import (`Index.Toggle.update`; chapter 6, "Names"):

```alder
test "Toggle flips done" = {
  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
  let id <- Db.insert Todo { title: Text.NonEmpty "Milk", due: Nothing, owner: me }
  let _ <- Test.login { user: Just me }
  let r <- Test.run (Index.Toggle.update { id: id } { user: me, role: Member })
  let after <- Db.get Todo id
  let done = Maybe.map (\t -> t.done) after
  Task.succeed ({ trace: r.trace, done: done } == { trace: ["Db.updateMine Todo"], done: Just True })
}
```

Line by line:

1. Insert a user. The database starts empty, and `Todo.owner` is a
   reference to a `User` row (`@ref User`, chapter 8), so a todo whose
   owner does not exist cannot be inserted: the test would fail with
   `error: Todo.owner: no User with that id`. A test builds every row it
   needs, the rows they refer to included. `Db.insert` returns the new
   row's id, `me`.
2. Insert a todo owned by `me`; its id is `id`.
3. Sign `me` in. `Toggle` uses `Db.updateMine` (chapter 6, "Rules"),
   which acts for the user of the session the test is running with, and
   `Test.login` sets that session (and the browser's cookie, section 9).
   Without this line the step would fail with `Db.updateMine Todo: no
   signed-in user`.
4. Run the `Toggle` route's `update` under `Test.run` (section 4) and
   call the outcome `r`. `Toggle.update` takes the route's parameters
   (`{ id: id }`, a `Toggle.Params`, the id the URL would carry) and the
   session; no `load` runs before it, and no form is decoded. The `Toggle`
   route `requires user`, so its `Session` is a type of its own whose
   `user` is a `UserId`, not a `Maybe` (chapter 6). The record literal
   `{ user: me, role: Member }` names every field, the role too (a
   literal gets no defaults: chapter 6, "Rules"), and is that type because
   it is written where `update` expects one. Bound to a name first by a
   `let` without an annotation, it would be a plain record, which `update`
   does not accept. This record is `update`'s `session` argument, which
   the route's own code may read; the scoped operations do not read it
   (step 3, and "Tests that use the rules", below).
5. Read the todo again, and take its `done` (`after` is a `Maybe Todo`:
   `Nothing` if the row were gone).
6. The verdict, as one `==` between two records: `update` made one
   step, the scoped write, and the stored todo is now done. Putting everything
   the test checks into one record, rather than joining several `==`
   with `&&`, means a failure prints both records whole (section 2).

The second test is the ownership rule of chapter 6, "Rules": a user who
posts `Toggle` with someone else's todo id gets a 404 and changes
nothing. The example tests it through the browser (section 9, "A
stranger's todo"); here it is through `update` itself, the stranger signed
in with `Test.login`:

```alder
test "Toggle by a stranger is not found" = {
  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
  let id <- Db.insert Todo { title: Text.NonEmpty "Milk", due: Nothing, owner: me }
  let stranger = User.UserId "someone-else"
  let _ <- Test.login { user: Just stranger }
  let r <- Test.run (Index.Toggle.update { id: id } { user: stranger, role: Member })
  let after <- Db.get Todo id
  let done = Maybe.map (\t -> t.done) after
  Task.succeed ({ value: r.value, trace: r.trace, done: done } == { value: Err "not found", trace: ["Db.updateMine Todo"], done: Just False })
}
```

The session's user, `stranger`, is not a row and need not be: a session
holds an id, not a reference the database checks. `Db.updateMine` finds
no todo with that id whose owner is `stranger`, and stops with
`Task.notFound`; nothing is written. If `Toggle` used the unscoped
`Db.update` instead, this test would say exactly what went wrong:

```text
$ alder test examples/todos -run 'by a stranger'
FAIL Todos.Index "Toggle by a stranger is not found"
  left:  { value: Ok Response.refresh, trace: ["Db.update Todo"], done: Just True }
  right: { value: Err "not found", trace: ["Db.updateMine Todo"], done: Just False }
  trace: ["Db.insert User", "Db.insert Todo", "Test.login", "Test.run", "Db.update Todo", "Db.get Todo"]
0 passed, 1 failed
```

The stranger's `Toggle` succeeded, the update ran, and the todo is done.

### Tests that use the rules

The scoped operations of chapter 6 (`Db.mine`, `Db.getMine`,
`Db.updateMine`, `Db.deleteMine`, `Db.insertMine`) act for the session
the test is running with, never for a session record the test passes
to a function. What sets that session:

- **`Test.login s`** sets it to `s`, as well as the browser's cookie
  (section 9). This is the way to choose the user.
- **`Session.put`**, in the test or inside an `update` it runs, changes
  it for every later step: a scoped operation after it acts for the
  new user.
- **A login through the login route's form** (`Test.submit` of its
  `Credentials`) signs the user into the **browser** only: the route's
  `Session.put` ran in the request, and the router stored the result in
  the cookie. The browser's next requests act for that user; the test's
  own steps (a `Test.run (Toggle.update …)`, a `Db.mine` in the test body)
  do not.

The route checks `requires` and the role (chapter 6); a test of them
goes through the browser (`Test.open`, `Test.click`, section 9); a
direct `update` call is not checked.

So a test that calls `update` or `load` itself, with a session record,
signs in with `Test.login` first; the record is still `update`'s
argument, and must have the route's `Session` type, but it does not choose
the user. Without a signed-in user a scoped step fails, and says why.
Here is "Toggle flips done" without its `Test.login` line, expecting
`Ok Response.refresh`:

```text
$ alder test examples/todos -run 'without a login'
FAIL Todos.Index "Toggle without a login"
  left:  { value: Err "Db.updateMine Todo: no signed-in user", trace: ["Db.updateMine Todo"] }
  right: { value: Ok Response.refresh, trace: ["Db.updateMine Todo"] }
  trace: ["Db.insert User", "Db.insert Todo", "Test.run", "Db.updateMine Todo"]
0 passed, 1 failed
```

It never answers as if the user had no rows.

A test may also use the scoped operations itself. The to-do list's last
test counts what `Me` sees:

```alder
test "Db.mine is what Me sees" = {
  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
  let other <- Db.insert User { email: Text.Email "other@example.com", name: Text.NonEmpty "Other", passwordHash: "" }
  let _ <- Db.insert Todo { title: Text.NonEmpty "Milk", due: Nothing, owner: me }
  let _ <- Db.insert Todo { title: Text.NonEmpty "Bread", due: Nothing, owner: me }
  let _ <- Db.insert Todo { title: Text.NonEmpty "Eggs", due: Nothing, owner: other }
  let _ <- Test.login { user: Just me }
  let r <- Test.run {
    let n <- Db.mine Todo |> Db.count
    Task.succeed n
  }
  Task.succeed ({ value: r.value, trace: r.trace } == { value: Ok 2, trace: ["Db.mine Todo", "Db.count Todo"] })
}
```

Its trace shows how `Db.mine` is recorded. `Db.mine Todo` is a
`Select`, a description that runs nothing (chapter 8), so it is not a
step of its own; when the `Db.count` that runs it starts, the runtime
notes `Db.mine Todo` just before the count's own name. The other four
are steps, named as written: `Db.getMine Todo`, `Db.updateMine Todo`,
`Db.deleteMine Todo`, `Db.insertMine Todo`.

### Two orgs

A partition by another key (chapter 6, "Partitions") is tested the same
way, with the key in the session. `Test.login` takes it as a served
request's cookie would hold it, and a key `via Membership` must pass the
same check a `Session.put` does: the user must have a membership row for
it first. `Test.member user org` inserts that row (the user and key
columns set, the others `Nothing` or their default; chapter 9). A test
of the workspace's project list (`examples/workspace/app/Projects/Index.alder`)
with two orgs, Ann a member of Acme, Bob of Globex, and Cy of both:

```alder fragment
test "a project is created in the org's database, and another org does not see it" = {
  …
  let _ <- Test.login { user: Just s.cy, org: Just s.acme }
  let _ <- Test.get (Projects.Index.Index {})
  let r <- Test.submit Projects.Index.Create { name: Text.NonEmpty "Zephyr" }
  let inAcme <- Db.scoped Project |> Db.where (Project.name.eq (Text.NonEmpty "Zephyr")) |> Db.all
  let acme <- Test.get (Projects.Index.Index {})
  let _ <- Test.login { user: Just s.cy, org: Just s.globex }
  let globex <- Test.get (Projects.Index.Index {})
  let inGlobex <- Db.scoped Project |> Db.where (Project.name.eq (Text.NonEmpty "Zephyr")) |> Db.count
  Task.succeed ({ answer: List.map (\p -> Redirected (Projects.Show.Show { id: p.id })) inAcme, acme: Test.texts "#projects .name" acme.doc, globex: Test.texts "#projects .name" globex.doc, inGlobex: inGlobex } == { answer: [r.answer], acme: ["Rocket", "Zephyr"], globex: ["Widget"], inGlobex: 0 })
}
```

The test's own `Db.scoped Project` reads the org of the test's session,
like the page's. Things to know:

- **A database per org.** With `silo org` (chapter 8, "Silos") each org a
  test touches has its own database in memory, made when the test first
  reads or writes it, and thrown away with the test. A row inserted with
  `Db.insert Project { …, org: acme }` goes to Acme's, whatever the
  session says. The test's savepoints (`Test.run`, each browser request)
  cover every one of them, so a failed `Test.run` undoes its writes in
  each.
- **Setup before the login.** A test whose session has an org may not
  insert a siloed row of another org ("Db.insert Project: the row's org
  is not this request's"): production refuses the same write. Insert the
  rows of both orgs first, then `Test.login`; or log in with `org:
  Nothing`, as the generated admin's tests do, whose pages take the org
  from their parameter.
- **A switch refused.** A request crafted by hand is a direct `update`
  call (the page offers no such button): `Test.run (Open.update { org:
  globex } { user: ann, org: Nothing, role: Member })` gives `Err
  "Session.put org: no Membership for this user and org"`
  (`Orgs.Index "a non-member's switch is refused"`). `Test.login` itself
  is refused the same way for an org the user is no member of. Through
  the browser the same refusal is an answer, not an error: the route
  answers the not-found page (a 404 in production, not a 500), which a
  test sees as `PageMissing`.
- **One store per `Test.run`.** A `Test.run` runs its `Task` as a request
  runs one, on one store: the first one it writes, or for `Test.run
  (Create.update p session)` the store the route's `update` is bound to,
  as the request's route binds it. A `Test.run` whose `Task` writes the shared
  database and a silo fails the test with the internal error production
  answers 500 with ("Db.insert Project writes silo org in a Task on the
  shared store (one store per Task)"). The test's own body is not one
  request: it may set up rows in the shared database and in several
  orgs' databases.
- **The re-read.** Deleting the membership in the test body, then
  `Test.open`ing the route, shows the re-read idiom at work: the cookie
  still has the org, and the route answers `PageMissing` (`Projects.Index
  "a member removed from the org loses access at once"`).

**Where an effect test runs.** Every effect test gets a world of its own:

- **A fresh database.** An empty SQLite database in memory, with the
  app's tables and nothing in them. Each test has its own, so no test
  sees another's rows, and the order tests run in never matters. When the
  test ends, the database is thrown away.
- **One transaction.** The block runs as one `Task` (chapter 7, "How
  effects run"): its first database step begins a transaction, every
  later step uses it, and the block's own reads see its own writes.
- **A ticking clock.** `Time.now`, and the `@created` column a
  `Db.insert` fills, read a clock that starts at `2026-01-01T00:00:00Z`
  and advances one second each time it is read, so rows created in a
  test are ordered, and a test gets the same times on every run, until
  the test sets it: `Test.setClock`, section 9 ("Setting the clock"). Time is
  not transactional: a `Test.run` that fails rolls back its writes, but
  the clock keeps the ticks that run read.
- **The app's capabilities.** When the app has adapters (Go under
  `adapters/`, chapter 7, "Implementing a capability"), they are wired as
  in `alder build`, by the generated `caps` package, so a test calls the
  real Go. A capability's error is a failure the test can observe: under
  `Test.run` (section 4) it is `Err "Password.verify: …"`. A test can
  replace a capability with a fake instead (section 8). A capability
  whose adapter is missing is not wired: `alder test` prints the list
  `alder build` would refuse, as a warning (`warning: 1 capability
  without an adapter runs unwired in tests:`, then one line per
  capability), and runs the tests anyway, so you can test with fakes
  before writing any Go. When the app has no `adapters/` at all, no
  capability is wired. Either way, a call to an unwired capability that
  no `Test.fake` replaced fails the test, saying so:

  ```text
  FAIL Auth.Login "a stranger is not verified"
    error: capability Auth.Login.Password is not wired in tests
    trace: ["Password.verify"]
  ```

- **The app's default session.** A test's session starts as `App`'s
  session with every field at its default (here `{ user: Nothing }`).
  `Test.login` and `Session.put` change it; it is the session the
  scoped operations act for (above). A test of `update` also passes a
  session record as `update`'s argument, which the route's own code
  reads.
- **A browser.** A page held and a session cookie, both empty at the
  start, for the steps of section 9.

A pure test (section 1) touches none of this: it is an expression,
evaluated once.

## 4. `Test.run`: running a task that may fail

`Toggle.update` for another user's todo ends in `Task.notFound`. If the
test simply wrote `let response <- Index.Toggle.update …`, that failure would be the
test's failure, as a failing step is any task's failure, and the test
could never check that the 404 happened. `Test.run` catches it. Its type,
from the standard module `Test` (`stdlib/Test.alder`), is
`Test.run : Task a -> Task { value: Result Text a, trace: List Text }`.

`Test.run t` runs the task `t` inside the test and always succeeds, with
a record:

- `value` is `Ok v` when `t` succeeded with `v`, and `Err msg` when it
  failed: `msg` is the text of a `Task.fail msg`, `"not found"` for
  `Task.notFound` (and `Query.notFound`) — the failures that become a 404
  page in the running app — or, when a capability's Go code returned an
  error, the call and the error: `"Password.verify: connection refused"`.
  A write route's `update` gives a `Result Text Response`, and `Response`
  values compare with `==`, so `r.value == Ok Response.refresh` works too.
- `trace` is the list of steps `t` performed (section 6).

When `t` fails, what it did is undone, and only that: its writes are
rolled back and the session is put back as it was before `Test.run`; the
writes the test made before it stay. The database can do this because of
a **savepoint**: a named mark inside the open transaction. `Test.run`
sets one before `t` starts; on failure `ROLLBACK TO` the mark undoes
exactly the writes after it, and on success `RELEASE` forgets the mark,
and `t`'s writes become part of the test's. Here is the second test as
the database sees it:

```text
test "Toggle by a stranger is not found"   (a fresh database in memory)

  BEGIN                                     first database step: the transaction opens
  │ Db.insert User
  │ Db.insert Todo
  │ Test.login                              the test's session: the stranger
  │ Test.run ── SAVEPOINT alder_t1
  │             │ Db.updateMine Todo        no todo with that id is the stranger's
  │             │ Task.notFound             update fails
  │             └ ROLLBACK TO alder_t1      undo update's writes (it made none)
  │               RELEASE alder_t1          r.value = Err "not found"
  │ Db.get Todo                             the todo is still there, not done
  COMMIT                                    the verdict was produced
                                            then the database is thrown away
```

Savepoints nest, so `Test.run` inside `Test.run` works the same way. A
test that inserts inside a failing `Test.run` can see the insert undone:

```alder
test "a failed run is undone" = {
  let r <- Test.run {
    let _ <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
    Task.fail "changed my mind"
  }
  let n <- Db.from User |> Db.count
  Task.succeed ({ value: r.value, users: n } == { value: Err "changed my mind", users: 0 })
}
```

Like the route's tests, this one ends with one `==` between two records.

## 5. `Test.trace`: the steps so far

`Test.trace : Task (List Text)` is the list of steps the test itself has
performed so far, without the `Test.trace` step:

```alder
test "the steps so far" = {
  let _ <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
  let _ <- Db.from Todo |> Db.count
  let steps <- Test.trace
  Task.succeed (steps == ["Db.insert User", "Db.count Todo"])
}
```

`Test.run` and `Test.trace` are for tests only. A value that uses one
is a *test helper* (section 9, "Test helpers"), which tests may call and
production code may not:

```alder
steps : Task (List Text)
steps = Test.trace
```

Where no test can reach (a route's `load`, `view` or `update`, `App`,
a fake's entry, a brand's check, a field's default) it is E0040, so no
route can depend on them:

```alder error=E0040 "`Test.trace` is only available in a `test` or a test helper: a field default is not a helper"
type Probe = { steps: Task (List Text) = Test.trace }
```

## 6. What a trace says

Every step a `Task` performs (every `Db.*` operation, capability call,
`Time.now`, `Log.info`, `Session.put`) is recorded by name, in order,
before it runs (chapter 7, "Every step is traced"). The names:

- a database step is its function and table: `"Db.insert User"`,
  `"Db.get Todo"`, `"Db.update Todo"`, `"Db.delete Todo"`,
  `"Db.count Todo"`, and the scoped ones as written,
  `"Db.updateMine Todo"`; `Db.mine Todo`, which is not a step, is noted
  as `"Db.mine Todo"` just before the step that runs it;
- a capability call is the capability and its function, as the calling
  module writes them: `"Password.verify"`;
- `Test.run` is a step itself, `"Test.run"`, in the test's trace;
- a browser step is its name and what it names: `"Test.login"`,
  `"Test.get Todos.Index.Index"` (the route's full path),
  `"Test.submit NewTodo"` (the form's name),
  `"Test.click Todos.Index.Toggle"` and `"Test.refresh"`, followed by the
  steps of the requests it made (section 9).

The steps inside a `Test.run` appear twice: in `r.trace`, which holds
only them, and in the test's own trace, after `"Test.run"`, even when
the run failed and its writes were undone. That is why the report of
"Toggle reads, then writes" above has the trace
`["Db.insert User", "Db.insert Todo", "Test.login", "Test.run", "Db.updateMine Todo"]`
while its `r.trace` was `["Db.updateMine Todo"]`. Compare
`r.trace` when the question is what `update` did; the test's own trace
is what the report prints to help you find out why a test failed.

A trace says what was attempted, not what the data became: it is the
same whether `Toggle` set `done` to `True` or to `False`. A test that
cares about the data reads it back, as both tests above do.

## 7. Choosing tests, and output for programs

`-run` takes a regular expression and runs only the tests whose
`Module "name"` matches it:

```text
$ alder test examples/todos -run Toggle
ok   Todos.Index "Toggle flips done"
ok   Todos.Index "Toggle by another user is not found"
2 passed, 0 failed
```

A pattern that matches nothing runs nothing and passes:
`0 passed, 0 failed`.

`-timeout` bounds the whole run (default `5m`; any Go duration, such as
`30s` or `2m`). When the time is up, the test running then fails with
`error: timed out after 30s`, the summary counts the tests that ran, and
`alder test` exits 1. The test program also stops if `alder test` itself
is killed, even with `kill -9`: it reads its standard input, which
`alder test` holds open, and exits when that closes.

`--json` is for programs, an agent among them: standard output has one
JSON object per test and nothing else, and the summary goes to standard
error.

```text
$ alder test examples/todos --json -run countOpen
{"module":"Table.Todo","name":"countOpen ignores done todos","file":"app/Table/Todo.alder","line":40,"col":1,"ok":true,"left":"2","right":"2","trace":[],"ms":0.01}
1 passed, 0 failed
```

Each object has `module`, `name`, `file`, `line` and `col` (where the
test's `test` keyword is, so an editor can jump to it), `ok`, `trace`
(empty for a pure test) and `ms`, the time it took in milliseconds (it differs from run to run);
and `left`, `right`, `note` and `error` when they have something to say.
Unlike the text report, it gives the sides of a passing test's `==` too.
A test that crashed the program or timed out is an object like any
failing test's, with the reason in `error`. If the app does not compile,
the output is one object, `{"diagnostics":[…]}`, holding what
`alder check --json` would print; any other problem after the arguments
are read (a build failure, a bad `-run` pattern) is one object
`{"error":"…"}`. So a program always gets JSON, except for a usage
error such as an unknown flag, which prints the usage line. Chapter 11
lists the exit codes.

## 8. Faking a capability

A capability's adapter is Go code that reaches outside the program
(chapter 7). A test of a checkout should not charge a real card, and a
test of a page should not depend on how Markdown is rendered. For
those, a module can declare a **fake**: a second implementation of a
capability, written in Alder beside the capability, that only tests
use. It answers every entry of the capability, and it can keep state in
tables of its own, so a charge and then its refund behave as they would
against the real service.

### A fake without state: `Markdown`

The blog's `app/Markdown.alder` declares the capability and, after it,
its fake:

```alder
module Markdown

-- Rendering a post's body is outside the language: a capability implemented in Go
-- (adapters/markdown/markdown.go), which builds the HTML node by node, so a post cannot inject markup.
capability Markdown {
  render : Text -> Query (Html Never),
}

-- In a test, `let _ <- Test.fake Markdown` replaces the adapter with this: the body as plain
-- text, so a test about a page asserts on the page, not on how Markdown is rendered.
fake Markdown {
  render text = Query.succeed (Html.text text)
}
```

`fake Markdown { … }` names the capability and holds one definition for
each of its entries, here `render`, without a signature: its type is
the entry's, so `text` is a `Text` and the body a `Query (Html Never)`.

A test installs the fake with a step, `let _ <- Test.fake Markdown`.
From then until the test ends, every call of `Markdown.render` (in
`load`, in a function it calls, anywhere) runs the fake instead of the
adapter. These two tests call the entry with the fake and without it:

```alder
module Preview

import Markdown

test "the fake leaves the text as it is" = {
  let _ <- Test.fake Markdown
  let body <- Markdown.render "Not *yet*."
  let steps <- Test.trace
  Task.succeed ({ body: body, steps: steps } == { body: Html.text "Not *yet*.", steps: ["Test.fake Markdown", "Markdown.render"] })
}

test "the adapter renders the Markdown" = {
  let body <- Markdown.render "Not *yet*."
  let steps <- Test.trace
  Task.succeed ({ body: body, steps: steps } == { body: <p>Not <em>yet</em>.</p>, steps: ["Markdown.render"] })
}
```

Added to a copy of the blog, both pass. Their traces show the shape of
every faked trace: installing is a step, `"Test.fake Markdown"`; each
call is traced by the entry's usual name, as a call to the adapter is;
and the steps a fake takes inside an entry follow that entry's step.

The blog's `Posts.Show` has one test of each kind: one through the real
adapter, so the wiring is tested; one with the fake, checking that
`load` puts the body in its Model and that `"Markdown.render"` is in its
trace. The rendering itself is tested in Go. That is the usual split:
fake a capability in the tests of the code that uses it, and keep one
test through the real adapter.

### A fake with state: a card processor

A card processor remembers: a refund must name a charge it made. This
module, `Payments`, and `Checkout` after it were run as an app of their
own, `shop`. The fake keeps each charge as a row:

```alder
module Payments

type ChargeId = brand Text

-- The card processor. In production, Go in adapters/payments/ implements it, func Stripe (chapter 7).
capability Stripe {
  charge : { cents: Int.Positive } -> Task (Result Text ChargeId),
  refund : { id: ChargeId } -> Task (Result Text Unit),
}

-- In a test, `Test.fake Stripe` puts this in the adapter's place: a processor in memory that
-- keeps each charge as a row of its own table.
fake Stripe {
  table Charge {
    id: ChargeId @id,
    cents: Int,
    refunded: Bool = False,
  }

  -- A charge over 1000.00 fails like a processor that answers with an error.
  charge args = {
    let id <- Db.insert Charge { cents: args.cents }
    if args.cents > 100000 then Task.fail "card declined" else Task.succeed (Ok id)
  }

  refund args = {
    let found <- Db.get Charge args.id
    match found {
      Just c -> {
        let _ <- Db.update Charge c.id (\x -> { x | refunded: True })
        Task.succeed (Ok ())
      },
      Nothing -> Task.succeed (Err "no such charge"),
    }
  }
}

test "a refunded charge is marked refunded" = {
  let _ <- Test.fake Stripe
  let charged <- Stripe.charge { cents: Int.Positive 250 }
  let refunded <- match charged {
    Ok id -> Stripe.refund { id: id },
    Err e -> Task.succeed (Err e),
  }
  let found <- match charged {
    Ok id -> Db.get Charge id,
    Err e -> Query.succeed Nothing,
  }
  let row = Maybe.map (\c -> { cents: c.cents, refunded: c.refunded }) found
  Task.succeed ({ refunded: refunded, row: row } == { refunded: Ok (), row: Just { cents: 250, refunded: True } })
}
```

`table Charge { … }` is a table like any other (chapter 8), the record
type `Payments.Charge`, used with `Db`. The entries are ordinary Alder
typed by the capability (`args` is `{ cents: Int.Positive }` in
`charge`); `charge` inserts before it decides, so the insert is there to
be undone when it fails. The test's trace is
`["Test.fake Stripe", "Stripe.charge", "Db.insert Charge", "Stripe.refund", "Db.get Charge", "Db.update Charge", "Db.get Charge"]`.

Code that uses the capability does not change; its tests install the
fake by the capability's name:

```alder
module Checkout

import Payments

-- The app's code calls the capability as always; it never names the fake.
pay : Int.Positive -> Task Text
pay cents = {
  let charged <- Stripe.charge { cents: cents }
  match charged {
    Ok id -> Task.succeed "paid",
    Err e -> Task.succeed ("refused: " ++ e),
  }
}

test "an order is paid" = {
  let _ <- Test.fake Stripe
  let said <- pay (Int.Positive 250)
  Task.succeed (said == "paid")
}

test "a declined charge leaves no row" = {
  let _ <- Test.fake Stripe
  let small <- pay (Int.Positive 250)
  let large <- Test.run (pay (Int.Positive 500000))
  let rows <- Db.from Payments.Charge |> Db.where (Payments.Charge.cents.eq 250) |> Db.count
  let all <- Db.from Payments.Charge |> Db.count
  Task.succeed ({ small: small, large: large.value, trace: large.trace, rows: rows, all: all } == { small: "paid", large: Err "card declined", trace: ["Stripe.charge", "Db.insert Charge"], rows: 1, all: 1 })
}
```

From `Checkout` the table is `Payments.Charge`, its columns
`Payments.Charge.cents`. The second test charges 500000: `charge`
inserts, then `Task.fail "card declined"`, which under `Test.run`
(section 4) is `Err "card declined"`; the savepoint undid the insert, so
one row is left. (A `Result` `Err` is an answer `pay` handles;
`Task.fail` imitates a processor that does not answer.) All three tests
pass. Outside `Test.run`, that charge fails the test.

### The rules

**The declaration.**

- A fake is declared in the module that declares its capability. A fake
  elsewhere is E0020. The to-do app's login tests (section 9) use a
  `Password` that accepts any user who has a row, whatever the password;
  that fake is in `app/Auth/Login.alder`, beside `capability Password`.
  Written in the module that wants it instead, it is refused:

```alder error=E0020 "`fake Password` must be declared in the module that declares `capability Password`"
fake Password {
  verify args = {
    let found <- Db.from User |> Db.where (User.email.eq args.email) |> Db.first
    Task.succeed (Maybe.map (\u -> u.id) found)
  }
}
```

- One fake per capability: a second is E0009, "`fake Stripe` is already
  declared at line 7" (the line of the first). Inside a fake, a second
  definition of one entry is E0009 too: "`charge` is already defined in
  `fake Stripe` at line 8" (the first definition's line).
- It defines every entry of the capability, and nothing else. A missing
  entry is E0003, at the fake, with the type it must have; an entry the
  capability does not have is E0003 "`fake Stripe` defines `cancel`,
  which `capability Stripe` has no entry for":

```alder error=E0003 "`fake Stripe` lacks `refund` (`{ id: ChargeId } -> Task (Result Text Unit)`)"
type ChargeId = brand Text

capability Stripe {
  charge : { cents: Int } -> Task (Result Text ChargeId),
  refund : { id: ChargeId } -> Task (Result Text Unit),
}

fake Stripe {
  charge args = Task.succeed (Ok (ChargeId "ch_1"))
}
```

- An entry is written `name params = body`, with no signature (a
  signature inside a fake is a parse error): its type is the entry's.
  Every error inside an entry is named after it: "in `fake Stripe`'s
  `charge`: expected Result Text Payments.ChargeId, got Text".
- A fake holds only `table` declarations and entry definitions, in any
  order. Types the fake needs (`ChargeId`) are declared in the module.
  A top-level value cannot be named `fake`: E0001 "`fake` starts a fake
  declaration at column 1: rename the value".
- Entry bodies are ordinary Alder: `Db` on the fake's tables and the
  app's, values of their module and its imports, and the capability's
  other entries (inside `fake Stripe`, `Stripe.refund` is the fake's,
  never the adapter's; an entry that calls itself unconditionally
  recurses until the run stops with `fatal error: stack overflow`). Not
  `Session`, which an adapter does not have: E0003 "in `fake Stripe`'s
  `charge`: a fake cannot use `Session`: an adapter has no session".

**The fake's tables.**

- A table of `fake Stripe` in module `Payments` is the record type
  `Payments.Charge`, as an app table of that module would be. It is
  exempt from E0018 (a table in a module named after it).
- Only a `test`, in any module, a test helper (section 9, "Test
  helpers") and the fake's own entries may use it, its type or its
  columns. A value `charges = Db.from Payments.Charge |> Db.count` in
  `Checkout` becomes a test helper. Anywhere else (a page's `load`,
  another fake's entry, a type declaration, an app table's `@ref`) it is
  E0021: "`Charge` is a fake's table: only tests and `fake Stripe` may use
  it".

- In another module, the table and its columns carry the module's name,
  `Payments.Charge` and `Payments.Charge.cents`, and that module must
  import `Payments`. Written bare, `Charge.cents.eq 250` is E0002
  "`Charge` is a table of `fake Stripe` in module `Payments`: write
  `Payments.Charge.cents`"; in a module without `import Payments`,
  `Payments.Charge` is E0002 "`Charge` is a table of `fake Stripe` in
  module `Payments`: add `import Payments`, then write `Payments.Charge`
  (columns: `Payments.Charge.column`)", with the import as its fix.
- A fake's table may `@ref` an app table (`user: User.UserId @ref User`):
  a fake can keep, say, which user each charge was for. Give that ref
  `@cascade` (or `@setNull`), so the app's deletes behave as they do in
  production, where the fake's table does not exist: with the default
  `@restrict`, deleting a user with a charge fails with
  ``User … is referenced by a row of Charge (`Charge.user`)``. An app
  table may not `@ref` a fake's (E0021).
- The tables exist only in test databases, created beside the app's
  tables (section 3). They are never in `migrations/schema.json` or a
  migration, so adding or changing a fake's table needs no migration.

**`Test.fake`.**

- `Test.fake Stripe` is a `Task Unit`, available only in a `test` or a
  test helper like the rest of `Test` (section 9, "Test helpers"; in a
  fake's entry, E0040 "`Test.fake` is only available in a `test` or a
  test helper: a fake's entry is not a helper"). Its argument is a capability's name, `Stripe` or
  `Payments.Stripe`, not an expression (E0003 "`Test.fake` wants a
  capability name, got an expression"), and the capability must have a
  fake (E0003 "`Test.fake` wants a capability with a `fake`: `Stripe` has
  none"). When two imported modules both declare `Clock`, the bare name
  is E0002 "`Clock` is a capability of both `A` and `B`: write `A.Clock`
  or `B.Clock`", here and in a call (`A.Clock.now`).
- It replaces the capability for the rest of the test, in every module.
  Installed inside a `Test.run`, it stays after the run, even a failed
  one (installing is not a database write). Installing again gives a
  new copy of the fake; the rows in its tables stay.
- Without it, a test calls the app's adapter, if any (section 3).
- Its trace has the shape shown with `Markdown` above. A failing entry
  is a failing step: under `Test.run` it is `Err` with the entry's
  message, its writes undone; outside it, the test fails.
- Each test starts with a fresh database and its own capabilities, so a
  fake installed in one test, and its rows, are never seen by the next.

**Where fakes exist.** `alder test` compiles them, and so do `alder eval`
and `alder repl`, whose builds are test builds too (chapter 11). A
production build (`alder build`, `alder run`) contains no tests at all:
no `test`, no fake, no fake's table and no test runner. (Chapter 13
shows the generated Go.)

## 9. Testing through the browser

A test can also use the app the way a user does: open a page, fill in and
post its form, click its buttons, and read what the page then shows and
what the session then holds, through a browser that runs in memory. Each
step names a route (chapter 6), as a link, a form or a button does.

### Why through the browser

The tests so far call `load` and `update` directly. That leaves out what
happens between them in the running app: the router that reads `/todos/x`
into a route's parameters, the decoding of a submitted form, `requires`
sending a visitor to the login page, the session a login stores, the page
a `Response.redirect` leads to, the regions a `Response.refresh` patches,
the buttons a page actually shows. Most of the bugs a server-rendered app
ships live at those seams, and they show only across one request and the
next.

So a test can also go through those seams. No browser program runs. The test drives the
app's real routes in memory, inside the test's own transaction, and each
step takes a fraction of a millisecond, so an app can have hundreds of
these tests and they give the same answer on every run. (Chapter 12,
"Tests drive the app in memory", says why this and not a real browser;
section 10 below shows how it works inside.)

### The browser a test holds

Every effect test (section 3) has, beside its fresh database, its clock
and its capabilities, a **browser** of its own. It holds **the page**,
the document of the last page it was given (none before the first step),
and **a cookie**, the signed cookie that carries the session (chapter 6),
sent back with every request. Before the first step there is no cookie,
so the session is `App`'s default, `{ user: Nothing }`.

Eight steps act through it. Each is a `Task` (a step of the test's block,
written with `let x <-`), and each works only inside a `test`. `r` is a
route value written in place, with its parameters (`Todos.Index.Index {}`,
`Todos.Index.Toggle { id: id }`):

| Step | What it does | It gives |
|---|---|---|
| `Test.login s` | puts the session `s` in the cookie, as a login would | `Unit` |
| `Test.get r` | requests the GET route `r`; the test fails unless its page is shown | `{ model: Model, session: Session, doc: Doc, url: Text, status: Int }` |
| `Test.open r` | requests the GET route `r`, whatever the answer | `{ answer: Answer, session: Session, doc: Doc, url: Text, status: Int }` |
| `Test.fetch r` | requests the file route `r`; the raw answer, redirects not followed, headers sorted by name | `{ status: Int, contentType: Text, body: Bytes, headers: … }` |
| `Test.submit r f` | posts the record `f` through the form of the write route `r` | `{ answer: Answer, session: Session, doc: Doc, url: Text, status: Int }` |
| `Test.submitRaw r fields` | posts text fields through the form of the write route `r` | the same |
| `Test.click r` | presses the page's button for the route `r`: a write route is sent, a GET route is fetched into the page | the same |
| `Test.refresh` | loads the page held again, and fails unless it is the page held | the same |

In the results, `model` is what the route's `load` gave, exactly the one
its `view` was given, and typed (`page.model.todos` is a `List Todo`);
`answer` says what the request came to (`Answer`, below); `session` is
`App`'s session after the step, what the cookie now carries
(`r.session.user == Just me`); `doc` is the page the browser now holds, a
`Doc`: a parsed HTML document, read with the queries below and compared
with `==`; `url` is the address of that page, path and query
(`"/?filter=Active"`); and `status` is the code of the step's last
response. After `Response.invalid` it is 422, after a missing page 404,
and after a redirect it is the status of the page the browser then
fetched (200). `Test.fetch` holds no page, so its result has no `session`,
`doc` or `url`; its `status` is the raw answer's.

### A first browser test

The to-do list's tests end with three through the browser (chapter 2
shows them in place). The first adds a todo by its form and reads the
summary line the page shows:

```alder
test "adding a todo through the page" = {
  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
  let _ <- Test.login { user: Just me }
  let _ <- Test.get (Todos.Index.Index {})
  let added <- Test.submit (Todos.Index.Submit {}) { title: Text.NonEmpty "Milk", due: Nothing }
  let page <- Test.get (Todos.Index.Index {})
  Task.succeed ({ answer: added.answer, todos: List.map (\t -> t.title) page.model.todos, summary: Test.text "#summary" page.doc } == { answer: Redirected (Todos.Index.Index {}), todos: [Text.NonEmpty "Milk"], summary: Just "1 open" })
}
```

In `app/Todos/Index.alder` the routes are written `Index` and `Submit`;
from another module, as here, a route value keeps its full path,
`Todos.Index.Index {}` (no import is needed for it; chapter 6, "Names").

### Step by step

Line by line:

1. Insert a user, as any effect test does.
2. `Test.login { user: Just me }` puts that session in the browser's
   cookie. The record is `App`'s session, so its `user` is a
   `Maybe UserId`: `Just me`, not `me`. Nothing is requested.
3. `Test.get (Todos.Index.Index {})` requests `/` with the cookie. The
   route `requires user`, and the session has one, so `load` runs, `view`
   renders, and the browser now holds the page. Its result is not needed
   here (`let _ <-`).
4. `Test.submit (Todos.Index.Submit {}) { … }` fills in the form of the
   `Submit` route with the record and posts it. A route value is a global
   address, so another module writes it with the full path. Even when
   the test file imports `Todos.Index`, the short `Index.Submit` is an
   error (E0205), and its fix writes `Todos.Index.Submit`. The page held
   must show that form: the browser posts only what a user could. The
   route's `update` inserts the todo and answers `Response.redirect
   (Index { filter: p.filter })`; the browser follows the redirect, as a
   real one does, and now holds the new page. `added.answer` is
   `Redirected (Todos.Index.Index {})`.
5. `Test.get` again, for the route's model: `page.model.todos` is the list
   `view` was given.
6. The verdict: the answer, the titles in the model, and the text of
   `<p id="summary">`, which `Test.text "#summary"` finds.

When the test is wrong, here with `"2 open"` expected, the report shows
what the page said and every step the requests took:

```text
$ alder test examples/todos -run "adding a todo"
FAIL Todos.Index "adding a todo through the page"
  left:  { answer: Redirected /, todos: ["Milk"], summary: Just "1 open" }
  right: { answer: Redirected /, todos: ["Milk"], summary: Just "2 open" }
  trace: ["Db.insert User", "Test.login", "Test.get Todos.Index.Index", "Db.mine Todo", "Db.all Todo", "Test.submit NewTodo", "Db.insertMine Todo", "Db.mine Todo", "Db.all Todo", "Test.get Todos.Index.Index", "Db.mine Todo", "Db.all Todo"]
0 passed, 1 failed
```

A route value prints as its address: `Redirected /` is
`Redirected (Todos.Index.Index {})`, whose `filter` is the default and so
not in the URL. Two route values are equal when their addresses are. The
trace is each step's name (`Test.submit` names the form), then the effects
of the requests it made. The submit made one request, the `Submit` route's
`update`, whose `Db.insertMine Todo` is its only write, and then fetched
the page the redirect led to (`Db.mine Todo`, `Db.all Todo`): a `redirect`
renders no page before the write, so nothing is read ahead of the insert
(section 10). Each `Db.mine Todo` is the note `load`'s scoped read leaves
before its `Db.all` (section 3).

### `Test.get` and `Test.open`: requesting a page

`Test.get r` and `Test.open r` take a GET route value, written in place:
`Todos.Index.Index {}`, `Todos.Index.Index { filter: Active }`,
`Todos.Show.Show { id: id }`. A variable holding a route is not accepted,
and neither is a write route (E0045: a write route is sent with
`Test.click` or `Test.submit`). Each sends a plain `GET` of the route's
address with the cookie.

`Test.get` is for a page the test expects to see. When the page is not
shown, the test fails and says why, with the address requested. Here,
from a test added to `Todos.Index` that gets the list without logging
in:

```text
FAIL Todos.Index "a visitor's list"
  error: GET /: redirected to /login (requires user)
  trace: ["Test.get Todos.Index.Index"]
```

The other two reasons are a 404 (the route's `load` found nothing and
said `Query.notFound`, as `Todos.Show.Show { id: … }` does for a todo
that is not the user's) and a failed request, a 500. Because it fails
otherwise, `Test.get` can give the route's `model`, typed: what its
`load` returned, a record.

`Test.open` is the same request, but it never fails the test on the
answer: its result's `answer` says what happened, and the test checks it.
This is how a test checks a redirect or a 404:

```alder
test "a visitor is sent to the login page" = {
  let r <- Test.open (Todos.Index.Index {})
  Task.succeed ({ answer: r.answer, heading: Test.text "h1" r.doc, user: r.session.user, url: r.url } == { answer: Redirected (Auth.Login.Login {}), heading: Just "Log in", user: Nothing, url: "/login" })
}
```

After `Redirected r` the browser already holds `r`, so the test goes on
from there without asking for it; `r.url` says where it is. (It could
not write `Test.open r` with the `r` it matched anyway: a step takes a
route value written in place.)

### `Answer`: what a request came to

`Answer` is a union of the Prelude, which only tests can use:

```text
type Answer = Shown
            | Rerendered
            | Rendered Route
            | Refreshed
            | Redirected Route
            | FormRejected (List { field: Text, message: Text })
            | PageMissing
            | RequestFailed Text
```

| Answer | When | The page the browser then holds |
|---|---|---|
| `Shown` | `get`/`open`: the page was shown | that page |
| `Rendered r` | `click` of the GET route `r`, or of a write route whose `update` answered `Response.render r` | the page it held, with `r`'s view patched in as the element with the view's root `id` |
| `Refreshed` | `click`/`submit` of a write route whose `update` answered `Response.refresh`; or `Test.refresh`, when the page loaded again is the page held | after a write, the page re-rendered, the regions the server changed patched in; after `Test.refresh`, the page loaded again |
| `Redirected r` | a `Response.redirect r`, a `load` ending in `Query.redirect r`, or a `requires` sending a visitor to the login page | `r`, which the browser requested in turn |
| `FormRejected errors` | `submit`/`click`: the form did not decode, or `update` answered `Response.invalid t` | the page with the form re-rendered: the values typed and the errors (status 422) |
| `PageMissing` | a 404: `Query.notFound`, `Task.notFound`, no such route | `App.notFound` (when only an element was asked for, a `Response.render` or a click on a GET route: the page it held, with an error toast) |
| `RequestFailed text` | the request failed (a `Task.fail`, a Go panic): a 500 | `App.error` (for an element, as above: the page it held, with an error toast) |

`Rendered` and `Refreshed` are the names of `Response.render` and
`Response.refresh` as the browser sees them: the first patches one
element, the second the regions that changed (section 10). `Rerendered`
is the older name of a re-render, kept in the union until the old
page-and-message syntax is removed; a route's answer is one of the other
seven, and a new test never writes it.

A redirect to another site (`Url.external`, chapter 6) is reported,
`Redirected` with that address, but not followed: the browser keeps the
page it had. A redirect to a page that is then a 404 is `PageMissing`.
`RequestFailed`'s text is the failure's message: `"nope"` for a
`Task.fail "nope"`, `"panic: …"` for a Go panic.

The 404 answer is `PageMissing`, not `NotFound`, so that an app keeps the
name `NotFound` for a constructor of its own. The eight names are kept for
`Answer` everywhere: a union of the app may not use one (E0012), nor may
a type be named `Doc` or `Answer`.

### `Test.fetch`: requesting a file

A file page (chapter 6, "File pages") answers a file, not a document, so
there is no page for the browser to hold. `Test.fetch r` takes a GET route
value written in place, like `Test.get`, sends the same plain `GET` with
the cookie, and gives the answer as the server would send it:

```text
Test.fetch : Route -> Task { status: Int, contentType: Text, body: Bytes, headers: List { name: Text, value: Text } }
```

`status` is the status number, `contentType` the label of the body (both
explained in chapter 6), `body` the bytes, and `headers` every header of
the answer, `Content-Type` included, sorted by name. It does not follow a redirect: a route that
`requires` a user the cookie lacks answers `status: 303` and a
`Location` header, and the test reads that; so does a route whose `load`
ends in `Query.redirect r` (chapter 7), with `r`'s address:

```alder fragment
test "start sends the builder its seed" = {
  let r <- Test.fetch (Start { seed: "7" })
  Task.succeed (r.status == 303 && List.member { name: "Location", value: "/builder/7" } r.headers)
}
```

It does not change the page the browser holds, either, so it can be used
between `Test.get`s.

`Bytes` has almost no operations (chapter 9, "File, Bytes"), so a test
reads a text body with `Bytes.toText`, which gives `Just text` when the
bytes are valid UTF-8, and compares the `Maybe`:

```alder
module Todos.Slow

route Slow = GET "/slow" { }

type Model = { waited: Int }

Slow.load : Slow.Params -> Session -> Query Model
Slow.load params session = {
  let before <- Time.now
  let _ <- Query.delay 20000
  let after <- Time.now
  Query.succeed { waited: Time.diffMillis before after }
}

Slow.view : Model -> Session -> File
Slow.view model session = File.json (Json.int model.waited)

-- The test clock moves one second at each Time.now reading, and the delay moves it 20 s
-- without waiting, so the two readings are 21000 ms apart.
test "slow waits twenty virtual seconds" = {
  let r <- Test.fetch (Slow {})
  Task.succeed ({ status: r.status, type: r.contentType, body: Bytes.toText r.body } == { status: 200, type: "application/json", body: Just "21000" })
}

test "the delay is on the trace" = {
  let _ <- Test.fetch (Slow {})
  let trace <- Test.trace
  Task.succeed (List.member "Query.delay 20000ms" trace)
}
```

The delay is virtual in a test: `Query.delay 20000` does not wait 20
seconds. The test's clock moves forward by the delay at once, so a
`Time.now` after it shows the time that passed, and the test stays fast.
The trace names the step with its clamped number (`Query.delay 20000ms`).

`Test.open` and `Test.get` on a file route are errors, because their result
is a page to read: the checker refuses them (E0003, "`Slow` is a file route:
use `Test.fetch`"). This is a check of the `view`'s type, so it covers a
`File.html` route too. As a second guard, a `Test.open` whose answer at run
time is not HTML fails with `RequestFailed` and the same advice.

A route that shows its own full URL (`Url.absolute`, chapter 9) sees the
same address in every test: `http://localhost:8080`. The test runner never
reads `ALDER_PUBLIC_URL`, so a test can compare the text exactly and passes
on every machine and in CI: `Url.absolute (Todos.Index.Index {}) ==
"http://localhost:8080/"`. The requests `Test.get` and `Test.fetch` send
are unchanged; only the text `Url.absolute` gives is fixed.

### Setting the clock

`Slow` above works out its answer from the ticking clock: 20000 ms
of delay plus the one second the second reading adds. That is fine for
one route, but a route whose output depends on the time (a countdown, a
"closes in 3 hours", today's schedule) would make every expectation a
sum of ticks, and one more `Time.now` on the route's path would shift them
all by a second. Two steps let a test say what time it is and when time
passes:

```text
Test.setClock : Time -> Task Unit
Test.advance  : Int -> Task Unit
```

- `Test.setClock t` sets the test's clock to `t`, and from then on the
  clock **stands still**: every `Time.now` and `Date.today` reads `t`
  (the one-second tick is off for the rest of the test). A second
  `setClock` moves it again, backwards too; the clock is the test's.
- `Test.advance ms` moves the clock `ms` milliseconds forward, the unit
  of `Time.toMillis` and `Query.delay`. A negative `ms` counts as 0 (to
  go back, set the clock again), and there is no practical upper limit
  (advancing a day is the point; anything past about 292 years stops
  there rather than wrapping into the past). A clock that ticks keeps ticking after an advance,
  and a set one stays still.
- A `Query.delay` or `Task.sleep` still moves the clock by its delay,
  set or not, and the routes a test requests through the browser read the
  same clock, so a set applies to them too.
- `@created` reads the clock as well, so rows inserted after a set all
  get the same time. A test that sets the clock and then wants a list in
  creation order calls `Test.advance 1000` between the inserts, or
  inserts its rows before the set.
- The session cookie's own clock does not move: a test that logged in
  stays logged in however far it advances time.
- Like the tick, a set or an advance is not undone by a `Test.run` that
  fails. Both are on the trace, as `Test.setClock 2026-03-04T00:00:00Z`
  (the time in UTC) and `Test.advance 86400000ms`.

A test that never sets the clock is unchanged: it starts at
`2026-01-01T00:00:00Z` and ticks. Here a page prints today's date, and
the test sets the clock to a date, fetches it, advances a day and
fetches again:

```alder
module Todos.Today

route Today = GET "/today" { }

type Model = { today: Date }

Today.load : Today.Params -> Session -> Query Model
Today.load params session = {
  let today <- Date.today
  Query.succeed { today: today }
}

Today.view : Model -> Session -> File
Today.view model session = File.text { contentType: "text/plain", body: Date.toText model.today }

-- 1772582400000 is 2026-03-04T00:00:00Z.
test "today follows the test's clock" = {
  let _ <- Test.setClock (Time.fromMillis 1772582400000)
  let before <- Test.fetch (Today {})
  let _ <- Test.advance (24 * 60 * 60 * 1000)
  let after <- Test.fetch (Today {})
  Task.succeed ({ before: Bytes.toText before.body, after: Bytes.toText after.body } == { before: Just "2026-03-04", after: Just "2026-03-05" })
}
```

### `Test.login`, `requires`, and the session

`Test.login s` signs `s` into the browser's cookie, as the router would
after a `Session.put`, and makes `s` the test's own session too, the one
the scoped operations of chapter 6 act for (section 3, "Tests that use
the rules"); `s` is checked against `App`'s `session` record
when the test compiles (`{ }` in an app without a session). It is the
cookie's value, so a field with a default may be left out: with
`session { user: Maybe UserId, visits: Int = 0 }`,
`Test.login { user: Just me }` stores `visits: 0`.

`Test.login` is the short way in, for a test about something else. A
test about logging in itself goes through the login route with its real
form. That route calls the `Password` capability, whose adapter is Go; a
test replaces it with the fake `app/Auth/Login.alder` declares beside it
(section 8), which accepts any user who has a row, whatever the
password. The test file imports `Auth.Login`, so the capability is
written through that import's qualifier, `Login.Password`; the routes
are route values, written with their full paths, `Auth.Login.Login {}`
for the page and `Auth.Login.Submit` for the one that posts its form:

```alder
test "logging in through the form" = {
  let _ <- Test.fake Login.Password
  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
  let _ <- Test.get (Auth.Login.Login {})
  let r <- Test.submit Auth.Login.Submit { email: Text.Email "me@example.com", password: Text.Secret "secret" }
  Task.succeed ({ answer: r.answer, user: r.session.user, title: Test.text "h1" r.doc } == { answer: Redirected (Todos.Index.Index {}), user: Just me, title: Just "Todos" })
}
```

The `Submit` route's `update` called the fake and `Session.put`; the
browser kept the cookie, followed the redirect with it, and holds the
list. (That login is the browser's: the test's own session is still the
one it started with, so a `Db.mine` in the test body would have no
user.) With an email that has no row the fake answers `Nothing`, `update`
answers `Response.invalid "Email or password is incorrect."`, and the
answer is `FormRejected [{ field: "", message: "Email or password is
incorrect." }]`, with `status` 422. `Test.click App.Logout` presses the
list's "Log out" button: the answer is `Redirected (Auth.Login.Login {})`
and `session.user` is `Nothing`.

### `Test.submit`: posting a form

`Test.submit r f` posts the form of the write route `r` with the record
`f`. `r` is a route with `form F`, written in place with its parameters
if it has any: `Todos.Index.Submit {}` here, `Todos.Show.Save { id: id }`
for the route that saves one todo. `f` is checked as the form's record (a
field with a default may be left out). The browser sends it as the page's
inputs would:

| The field's type | Sent as |
|---|---|
| `Text`, `Text.NonEmpty`, `Text.Email`, `Text.Url`, `Text.Secret`, a brand of `Text` | the text |
| `Int`, a brand of `Int` | its digits: `42`, `-3` |
| `Float` | the shortest decimal that reads back the same: `1.5`, `1e-07` |
| `Bool` | `True` when `True` (a ticked checkbox); not sent when `False` (an unticked one) |
| `Date` | `2026-03-01` |
| a union without payloads | the constructor's name: `Active` |
| `Maybe a` | blank for `Nothing` (an empty input); `a`'s text for `Just` |

Decoding it gives back `f`, except that a `Maybe Text` that is
`Just ""` is sent blank and decodes as `Nothing`: neither a test nor a
user can send `Just ""`.

The form must be on the page the browser holds, and the route the test
writes must be the route that form posts to, parameters included. The
browser sends it to the address the form carries, and sends only the
fields the form has an input for (an `<Input>`, `<Select>` or
`<Textarea>` with the field's name), as a real browser does. A field
without one fails the step, "the page's form NewTodo has no input
`due`", unless its value is what the page makes of a field that is not
sent: `Nothing` for a `Maybe` without a default, the default for a field
that has one, `False` for a `Bool`; those are left out, and the route
decodes the same record.

A page opened as `Todos.Index.Index { filter: Active }` renders its form
as `Submit { filter: Active }`, whose address is `/_/Submit?filter=Active`;
that is the route to write. `Submit {}` is `/_/Submit`, another address,
and the step fails "the page /?filter=Active has no form NewTodo
(Todos.Index.Submit)". A form that each row renders
(`<form submit={Rename { id: n.id }}>` in a loop) is told apart the same
way: the test writes the route with the row's parameters,
`Test.submit (Rename { id: b }) { title: Text.NonEmpty "B2" }`, which
posts the form of row `b`.

A `Date` in a test record: `Date.fromText "2026-03-01"` is a
`Maybe Date`, the type of a `due: Maybe Date` field. For a `Date` field,
take one from the test's clock: `let today <- Date.today` gives
`2026-01-01`, `Date.addDays 3 today` a later day.

### More forms

Here is a todo with a due date, sent through the form:

```alder
test "a todo with a due date" = {
  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
  let _ <- Test.login { user: Just me }
  let _ <- Test.get (Todos.Index.Index {})
  let r <- Test.submit (Todos.Index.Submit {}) { title: Text.NonEmpty "Milk", due: Date.fromText "2026-03-01" }
  Task.succeed ({ answer: r.answer, due: Test.text ".due" r.doc } == { answer: Redirected (Todos.Index.Index {}), due: Just "Mar 1" })
}
```

A route posts a form; the form's name does not say which route. The
compiler asks for the route:

```alder error=E0003 "`Test.submit` names the route that posts a form: write `Test.submit (Submit { filter: … }) …`"
test "naming the form" = {
  let _ <- Test.get (Todos.Index.Index {})
  let r <- Test.submit Todos.Index.NewTodo { title: Text.NonEmpty "Milk", due: Nothing }
  Task.succeed (r.answer == Shown)
}
```

and it refuses a route that cannot be posted or fetched that way, with
E0045: `Test.submit` of a route without a `form` ("`Test.submit` posts a
route's form; `Toggle` has no `form`: use `Test.click (Toggle { id: … })`")
or of a GET route ("… `Index` is GET: use `Test.get`").
A test that opens the login page and then submits `Submit` gets:

```text
FAIL Todos.Index "posting a form the page lacks"
  error: the page /login has no form NewTodo (Todos.Index.Submit)
  trace: ["Db.insert User", "Test.get Auth.Login.Login", "Test.submit NewTodo"]
```

### `Test.submitRaw`: input the types forbid

`Test.submit`'s record can only hold valid values: a `Text.NonEmpty` is
never empty, a `Date` is always a date. A user's browser has no such
rule, and the route must answer bad input with the form and its errors.
`Test.submitRaw r fields` posts text, field by field, as typed:
`Test.submitRaw (Todos.Index.Submit {}) [{ name: "title", value: "" }]`. A
field not sent is a field left out of the form. Every name sent must be a
field of the form and have an input on the page.

**`FormRejected errors`** is the answer when the page shows its form again
with errors: the form did not decode (chapter 6: `update` never ran), or
`update` answered `Response.invalid "…"`. The status is 422. The browser
holds the page re-rendered, with what was typed back in each input
(except a `Text.Secret`, which is never sent back) and each error in its
`<Errors>` list. `errors` is what those lists show:

- in the order the lists come in the page (the order of the form's
  `<Errors field="…">`, usually its fields' order);
- `field` is the field's name, and `""` for the form's own list,
  `<Errors form />`, which holds `invalid`'s text;
- a field error the form has no `<Errors field>` for is shown in
  `<Errors form />`, after `invalid`'s text, prefixed with the field's
  name, so it comes back as `{ field: "", message: "title must not be
  empty" }`;
- a form with no `<Errors form />` shows no `invalid` text at all, and
  `errors` is `[]`: the answer to an `invalid` is still `FormRejected` and
  the status 422, but a user sees the page unchanged. The compiler warns about such a form
  when the `update` of its route can answer `Response.invalid` (W0005).
- a form the re-rendered page no longer draws shows no `invalid` text
  either. A page held while an order was Open, the order paid meanwhile,
  and its add-line form posted: `update` answers `invalid`, but the Paid
  order's page has no add-line form, so the text has nowhere to go; the
  answer is `FormRejected []`, and a user sees the page change with no
  message. W0005 cannot see this (it depends on the data). When such a
  refusal must be visible, answer `Task.notFound` or `Task.fail` instead.

The messages are the decoders' (chapter 6): "must not be empty" for a
`Text.NonEmpty`, "must be a date (YYYY-MM-DD)", "must be a whole
number", and "is not valid" for a brand whose `where` refuses the value
(the blog's comment `Body`, chapter 14).

### Bad input, an example

The to-do form with an empty title and a date that is not one:

```alder
test "an empty title is rejected" = {
  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
  let _ <- Test.login { user: Just me }
  let _ <- Test.get (Todos.Index.Index {})
  let r <- Test.submitRaw (Todos.Index.Submit {}) [{ name: "title", value: "" }, { name: "due", value: "tomorrow" }]
  let rows <- Db.from Todo |> Db.count
  Task.succeed ({ answer: r.answer, due: Test.attr "value" "input[name=due]" r.doc, rows: rows, status: r.status } == { answer: FormRejected [{ field: "title", message: "must not be empty" }, { field: "due", message: "must be a date (YYYY-MM-DD)" }], due: Just "tomorrow", rows: 0, status: 422 })
}
```

The title and the date come back as typed (`Test.attr "value"` reads
the input), with their errors, and no row was written. `update` never
ran: the form did not decode.

### `Test.click`: pressing a button

`Test.click r` sends the route `r` as the page's button for it would:
`Test.click (Todos.Index.Toggle { id: id })`, `Test.click App.Logout`. `r`
is a route without a `form`, with its parameters, written as in a
`click={…}`. A write route is sent, with the headers the page's script
sends (`Alder-Page`, the address the browser holds, and `Alder-Build`;
chapter 6, "Delivery, decided by the request"). A GET route is fetched
the way a button fetches an element, and its view is patched into the page
held as the element with the view's root `id`. A route that carries a
form is posted with `Test.submit`; a link is followed with `Test.get` or
`Test.open`.

The page must show a button (or form) that sends exactly that route,
with the same parameters: a test can only do what the user it is logged
in as can see to do. The second to-do test through the page:

```alder
test "toggling a todo through the page" = {
  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
  let id <- Db.insert Todo { title: Text.NonEmpty "Milk", due: Nothing, owner: me }
  let _ <- Test.login { user: Just me }
  let _ <- Test.get (Todos.Index.Index {})
  let r <- Test.click (Todos.Index.Toggle { id: id })
  Task.succeed ({ answer: r.answer, done: Test.has "li.done" r.doc, summary: Test.text "#summary" r.doc } == { answer: Refreshed, done: True, summary: Just "0 open" })
}
```

`Toggle.update` answered `Response.refresh`, so the browser loaded the
page again and patched in the regions that changed: the item now has
`class="done"`, and nothing is open. The answer is `Refreshed`. Had the
route answered `Response.render (Row { id: id })`, the answer would be
`Rendered` with the address of `Row`, and only that one item would have
changed (chapter 6, "`Response`").

### Clicks that fail

The compiler refuses a click of a route that carries a form (E0045):

```alder error=E0045 "`Test.click` sends a route without a form; `Submit` posts form `NewTodo`: use `Test.submit (Submit { filter: … }) { … }`"
test "clicking a form's route" = {
  let _ <- Test.get (Todos.Index.Index {})
  let r <- Test.click (Todos.Index.Submit {})
  Task.succeed (r.answer == Shown)
}
```

### What the browser checks on every step

A click or a submit changes the page the browser holds by patching it, as
the page's script does. Two checks run on what was patched, and a patch
with nowhere to go is refused. A step that fails one fails the test.
Section 10 shows how each is made.

- **The region check.** After a `refresh`, an `invalid` or a followed
  `redirect` the server rendered the page, and the browser compares each
  region the answer patched with the same region of the full render. A
  difference fails the step: "POST /_/Toggle: the patched page differs
  from a full render at" the path of the first element that differs (named
  as a failed `Doc` comparison names it, below), with both markups, as the
  browser's parser builds them. A region the answer did not send is not
  compared. A difference means the page the user holds is not the page the
  server rendered.
- **The element check.** After `Rendered r`, the browser asks for `r` again
  on its own, as a button does, and the element it holds must equal the
  view that answers. It is the same `load` and `view`, so they are equal
  unless the view shows something that changes from one request to the
  next. The step fails: "the answer patched #todo-1 unlike
  Todos.Index.Row's view rendered on its own (GET /rows/…) at li#todo-1".
  The second request runs in a savepoint that is undone, and is not a step
  of the trace.
- **A patch with nowhere to go.** A `render` whose root `id` the held page
  lacks patches nothing in a real browser, silently. The test fails the
  step: "POST /_/Toggle: the answer patched `#todo-9`, which the page does
  not have". A view with no root `id` is not patched either: the answer is
  `RequestFailed` ("route GET /rows/\{id} answered an element fetch with a
  view whose first root element has no id").

A `render` patches one element, and a page can have other places that
should have changed: these checks do not see that. `Test.refresh` does.

### `Test.refresh`: is the page the one a fresh load would give?

`Test.refresh` loads the address the browser holds again, with a plain
`GET` as a reload does (a redirect followed), and fails unless the body
of the page it gets is the body of the page the browser held. It then
holds the new page, and the answer is `Refreshed`. The `<al-toasts>`
region is left out of the comparison, since a reload may fill it from a
notice that the step's toasts already delivered. A test of a toggle ends
with it:

```alder
test "a toggle leaves the page as a fresh load would" = {
  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
  let id <- Db.insert Todo { title: Text.NonEmpty "Milk", due: Nothing, owner: me }
  let _ <- Test.login { user: Just me }
  let _ <- Test.get (Todos.Index.Index {})
  let clicked <- Test.click (Todos.Index.Toggle { id: id })
  let again <- Test.refresh
  Task.succeed ({ clicked: clicked.answer, again: again.answer, url: again.url } == { clicked: Refreshed, again: Refreshed, url: "/" })
}
```

`Toggle` answers `refresh`, which patches the regions that changed, so
the page after the click is the page a fresh load gives, and this
passes. `Test.refresh` is the developer's check, not part of every step,
because some pages are right without it. A test of "load more", of a draft
preview, or of an inline edit that is open leaves the page different from a
fresh load on purpose: do not end those with `Test.refresh`.

### When `Test.refresh` fails

The test earns its place when a route answers with less than it
should. Here is the same app with `Toggle` changed as chapter 6 does
(`Row` is a GET route for one item, and `Toggle` answers
`Response.render (Row { id: p.id })`). The click passes both checks above,
since the item it patched is right, and `Test.refresh` fails:

```text
FAIL Todos.Index "toggling, then refreshing"
  error: Test.refresh: GET /: the page held differs from the page loaded again at body > al-page > main > al-stack > p#summary: child 1: left text "1 open", right text "0 open"
      held:   <p id="summary">1 open</p>
      loaded: <p id="summary">0 open</p>
  trace: ["Db.insert User", "Db.insert Todo", "Test.login", "Test.get Todos.Index.Index", "Db.mine Todo", "Db.all Todo", "Test.click Todos.Index.Toggle", "Db.updateMine Todo", "Db.getMine Todo", "Test.refresh", "Db.mine Todo", "Db.all Todo"]
0 passed, 1 failed
```

The item changed and the count of open todos below the list did not: the
toggle should have been a `refresh`. The trace shows the other price of
`render`: one `Db.getMine Todo` for the item, where `refresh` loads the
whole list. The failure names where the two bodies first differ and shows
each side. The other failures are "Test.refresh: GET /rows/x: not found",
when the address is no longer shown (the text of `Test.get`'s failure for
that address), and "Test.refresh: the browser holds no page yet".

### A stranger's todo

**Who may toggle a todo.** Chapter 6's rule, "Rules", says that a user
who posts `Toggle` with someone else's todo id gets a 404. A stranger
who logs in and gets the list has no way to do that through the page (a
test that logs in as the stranger, gets the list, and clicks `Toggle {
id: id }` expecting `PageMissing` does not get that far):

```text
FAIL Todos.Index "a stranger clicks my todo"
  error: the page / has no button or form for Todos.Index.Toggle { id: "Q7QLW1cLZ2muCysr_EhO" }
  trace: ["Db.insert User", "Db.insert Todo", "Test.login", "Test.get Todos.Index.Index", "Db.mine Todo", "Db.all Todo", "Test.click Todos.Index.Toggle"]
```

The stranger's list shows only the stranger's todos, so there is no
Toggle button for mine to press; the browser sends nothing, and the test
fails. (The id is random, and differs on each run.) That is the right
answer: the page does not offer the action.

But a page can be out of date. My list is open in the browser; then
someone else logs in on it (the cookie changes, the page held does not)
and presses my todo's button. The example tests exactly that:

```alder
test "Toggle by another user is not found" = {
  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
  let id <- Db.insert Todo { title: Text.NonEmpty "Milk", due: Nothing, owner: me }
  let _ <- Test.login { user: Just me }
  let _ <- Test.get (Todos.Index.Index {})
  let _ <- Test.login { user: Just (User.UserId "someone-else") }
  let r <- Test.click (Todos.Index.Toggle { id: id })
  let after <- Db.get Todo id
  Task.succeed ({ answer: r.answer, done: Maybe.map (\t -> t.done) after } == { answer: PageMissing, done: Just False })
}
```

The click posts my page's Toggle button with the stranger's cookie. The
router renders the page the stranger is on first (the before-view of a
route that can answer `refresh`; section 10), then runs `update`, where
`Db.updateMine` finds no todo of the stranger's with that id:
`PageMissing`, and the todo is not done. "Delete by another user is not
found" does the same with the Delete button. Section 3's "Toggle by a
stranger is not found" asks the same question of `update` directly.

The ownership test is also written with what a stranger can do on a
fresh page, open the todo's address:

```alder
test "a stranger cannot see my todo" = {
  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
  let id <- Db.insert Todo { title: Text.NonEmpty "Milk", due: Nothing, owner: me }
  let _ <- Test.login { user: Just (User.UserId "someone-else") }
  let page <- Test.get (Todos.Index.Index {})
  let show <- Test.open (Todos.Show.Show { id: id })
  Task.succeed ({ items: Test.count "li" page.doc, show: show.answer } == { items: 0, show: PageMissing })
}
```

The stranger's list has no `<li>` (`Db.mine Todo` has none of mine), and
`Todos.Show.Show`'s `load` reads the todo with `Db.getMine`, which answers
not found for a todo that is someone else's: `PageMissing`.

### Reading the page: queries

Five functions read a `Doc`, and one makes a `Doc` from a view. They are
plain functions (not steps), so they are used in expressions, as above:

```text
Test.text  : Text -> Doc -> Maybe Text           -- the first match's text
Test.texts : Text -> Doc -> List Text            -- every match's text, in page order
Test.has   : Text -> Doc -> Bool                 -- whether anything matches
Test.count : Text -> Doc -> Int                  -- how many elements match
Test.attr  : Text -> Text -> Doc -> Maybe Text   -- an attribute of the first match
Test.doc   : Html a -> Doc                       -- a view, as a document
```

The first `Text` is a **selector**, as in CSS; `Test.attr`'s first is
the attribute's name, its second the selector:
`Test.attr "href" "a[rel=next]" doc`. Alder accepts a small part of
CSS's selectors:

| Selector | Matches |
|---|---|
| `li` | every `<li>` |
| `#summary` | the element with `id="summary"` |
| `.done` | every element whose `class` has `done` |
| `[href]` | every element with an `href` attribute |
| `[rel=next]`, `[name="title"]` | every element whose `rel` is `next` (quote a value that is not a plain word: `[href="/x"]`) |
| `li.done`, `input[name=due]` | both at once: an `<li>` with the class `done` |
| `ul li` | an `<li>` anywhere inside a `<ul>` |
| `ul > li` | an `<li>` directly inside a `<ul>` |

A tag is an HTML element's name in lower case, or a custom element's (a
name with a hyphen). Nothing else is accepted (no `*`, `:first-child`
or `,`). A selector written as a literal is checked when the test
compiles (E0023). A selector that is not a literal (in a variable, or
built with `++`) is read when the test runs, and if it is not a selector
it matches nothing: `Nothing`, `[]`, `False`, `0`. A wrong one then
looks like a page that lacks the element, so write selectors as
literals.

An element's text is the text inside it (not counting a `<script>` or
`<style>`), with every run of spaces and line breaks made one space and
none at either end. No space is put between elements: the to-do item
`<li><button>Done</button><a …>Milk</a><button>Delete</button></li>` has
the text `"DoneMilkDelete"`. Select the element that holds just the text
wanted, `Test.texts "#todos a"`, not the whole item.

**`Test.doc` and `view`.** `Test.doc (view model session)` renders a view
the way the page renders it and parses it, so a test can compare the
page the browser holds with what `view` makes of the model it was given.
Two `Doc`s are equal when they are the same tree of elements, attributes
and text. `Test.doc` renders without a request, so a form in it has
no typed values and no errors: compare a `FormRejected` page by its queries,
not with `Test.doc`.

### A selector the compiler refuses

Here the `x` after a space would be an element inside `#sumary`, but
there is no element `x`:

```alder error=E0023 "`#sumary x` is not a selector: unexpected `x` after `#sumary`; `x` is not an HTML element (a custom element has a hyphen)"
test "a misspelt selector" = {
  let page <- Test.get (Todos.Index.Index {})
  Task.succeed (Test.text "#sumary x" page.doc == Nothing)
}
```

The full text ends with the list above: "(a selector is a tag, `#id`,
`.class`, `[attr]`, `[attr=value]`, combined with spaces or `>`)".

### Comparing a page with its view

```alder
test "the page and its view agree" = {
  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
  let _ <- Db.insert Todo { title: Text.NonEmpty "Milk", due: Nothing, owner: me }
  let _ <- Test.login { user: Just me }
  let page <- Test.get (Todos.Index.Index { filter: Active })
  Task.succeed ({ same: page.doc == Test.doc (Index.Index.view page.model { user: me, role: Member }), links: Test.texts "nav a" page.doc, titles: Test.texts "#todos a" page.doc, items: Test.count "#todos > li" page.doc } == { same: True, links: ["All", "Active", "Completed"], titles: ["Milk"], items: 1 })
}
```

When a test's last `==` compares two `Doc`s that differ, the report does
not print the two documents. It names the first element whose content
differs, its path from the body, and shows that element on each side.
Had the test compared the page with a view of an empty model,
`page.doc == Test.doc (Index.Index.view { page.model | todos: [] } { user: me, role: Member })`,
it would print:

```text
FAIL Todos.Index "the page and an empty view"
  left:  <ul id="todos"><li class=""><form method="post" action="/_/Toggle?id=gt7bDOoKtnkV0adE5DOj" class="_alder-click" data-on…
  right: <ul id="todos"></ul>
  note:  the documents differ at body > al-page > main > al-stack > ul#todos: left 1 child <li>, right 0
  trace: ["Db.insert User", "Db.insert Todo", "Test.login", "Test.get Todos.Index.Index", "Db.mine Todo", "Db.all Todo"]
```

Each side is cut to 120 characters. The path names each element by its
tag and its `id` (or its classes), so it can be pasted into
`Test.count` to look closer.

When to use which: compare `page.model` when the question is what `load`
computed; query `doc` when it is what the user sees (a summary line, a
link's address, a button's presence); compare with `Test.doc (view …)`
to check that a page shows nothing but what `view` makes of its model.

### What a failed step says

When a step cannot do what the test asks, the test fails with one of
these texts, and nothing is sent:

- `GET /: redirected to /login (requires user)`, `GET /todos/x: not
  found`, `GET /: failed: <message>`: `Test.get` of a page not shown.
- "the page /login has no form NewTodo (Todos.Index.Submit)": the held
  page renders no form for that route, with those parameters.
- "the page / has no button or form for Todos.Index.Toggle { id: "x" }":
  the held page renders no control for that route, with those
  parameters.
- "the page's form NewTodo has no input `due`": a field the form renders
  no input for (`Test.submit`, `Test.submitRaw`).
- "no page is open: `Test.get` one first".
- "form NewTodo has no field `colour`": a `Test.submitRaw` name the form
  does not declare.
- "POST /_/Toggle: the patched page differs from a full render at …",
  "… the answer patched #todo-1 unlike Todos.Index.Row's view rendered on
  its own …" and "… the answer patched `#todo-9`, which the page does not
  have": the checks of "What the browser checks on every step".
- "Test.refresh: GET /: the page held differs from the page loaded again
  at …", "Test.refresh: GET /rows/x: not found" and "Test.refresh: the
  browser holds no page yet": `Test.refresh`.

And when the test compiles (E0003 unless named):

- E0040 "`Test.get` is only available in a `test` or a test helper:
  `Toggle.update` is the route's update, not a helper"; "`Test.get`
  needs an `App` module: the browser requests the app's routes, which an
  app without `App` does not serve".
- "`Test.submit` takes its arguments directly, not through `|>`: …";
  E0008 "`Test.submit` takes 2 arguments, got 1: …".
- "`Test.get` wants a page value such as `Todos.Index {}`, got `p`" (a
  route held in a variable: write the route in place, `Todos.Index.Index
  {}`); "`Test.get` needs a record `Model`: … use `Test.open`".
- "`Test.login` wants the app's session `{ user: Maybe User.UserId }`:
  expected Maybe User.UserId, got User.UserId".
- E0045, a route of the wrong kind for the step: "`Test.get` requests a
  GET route; `Toggle` is POST: use `Test.click`"; "`Test.submit` posts a
  route's form; `Toggle` has no `form`: use `Test.click (Toggle { id: … })`";
  "`Test.submit` posts a route's form; `Index` is GET: use `Test.get`";
  "`Test.click` sends a route without a form; `Submit` posts form
  `NewTodo`: use `Test.submit (Submit { filter: … }) { … }`".
- E0003 "`Test.submit` names the route that posts a form: write
  `Test.submit (Submit { filter: … }) …`"; E0002 "`NewTodo` is a form of
  `Todos.Index`: write `Todos.Index.NewTodo`" and "write
  `Todos.Index.Submit`: a route is written with its module's full path".
- E0002 "unknown constructor `NotFound`; the browser's 404 answer is
  `PageMissing`"; E0012 "`PageMissing` is a constructor of the Prelude's test
  type `Answer`; …"; in a type declaration, "`Doc` is a test type: it
  exists only inside a `test`".
- E0023 "`#sumary x` is not a selector: unexpected `x` after `#sumary`;
  `x` is not an HTML element (a custom element has a hyphen)"; its other
  hints are "tags are lower case: `h1`" and "quote the value:
  `[href="/x"]`".
- W0005 "`Submit` can answer `Response.invalid`, but this form has no
  `<Errors form />`: the user never sees its text".

### The rules

- The steps exist only inside a `test` or a test helper (below), each
  applied directly to all its arguments (not piped into, not passed as a value); all but
  `Test.login` need an `App` module.
- **Only what the page shows.** `Test.submit` and `Test.click` act on a
  form or button the held page renders, posting it exactly as rendered,
  and a submit sends only the fields the form has inputs for.
- **One transaction.** Every request runs inside the test's own
  transaction, its `update` under a savepoint of its own: a page sees
  the rows the test inserted, the test sees the rows a page wrote, and a
  request that fails (`RequestFailed`, `PageMissing`) undoes only its own writes.
- **The browser is not rolled back.** A `Test.run` that fails undoes
  its database writes, the writes of the steps inside it included, but
  not what happened to the browser: the cookie and the page held stay as
  the run left them. Steps inside `Test.run` work as anywhere else.
- **One browser per test.** Each test starts with no page and no
  cookie; nothing of one test's browser reaches the next.
- **Traces.** A step is a step of the trace, named `Test.login`,
  `Test.get Todos.Index.Index`, `Test.open Todos.Show.Show`,
  `Test.submit NewTodo`, `Test.submitRaw NewTodo`,
  `Test.click Todos.Index.Toggle`, `Test.click App.Logout` or
  `Test.refresh`, and the effects of the requests it made follow it.
- **What it does not test.** No CSS, no layout, no JavaScript: whether a
  button is visible, whether a web component works, how the page looks.
  The browser is Datastar's client (section 10), so the path of a
  browser with JavaScript off, a plain form post answered by a
  redirect, is not tested either (section 13).

A step's result also has `notices`, the list of toast texts the answer showed
(`Response.toast "Saved" Response.refresh` gives `["Saved"]`), empty when there were none:

    { answer: r.answer, notices: r.notices } == { answer: Refreshed, notices: ["Saved"] }

It also has `toasts`, the same toasts in the same order with their kind
(`"info"`, `"success"`, `"warning"` or `"error"`, chapter 6), for a test
that checks which kind the answer showed. `Response.success "Saved"
Response.refresh` gives:

    r.toasts == [{ kind: "success", text: "Saved" }]

### Test helpers

Tests of one page often begin the same way: the same fakes, the same
request, the same decoding of the body. That prologue can be written
once, as an ordinary top-level value, in any module, and called from any
test:

```alder fragment
fetchText : Text -> Task { status: Int, body: Text }
fetchText cfg = {
  let _ <- Test.fake Gate
  let _ <- Test.fake MnSos
  let r <- Test.fetch (Sites.MnSos.MnSos { cfg: cfg })
  Task.succeed { status: r.status, body: Bytes.toText r.body ?? "" }
}

test "at minute 400 the file is the final" = {
  let r <- fetchText (at 400 "")
  Task.succeed (r.status == 200 && Text.contains "final" r.body)
}
```

`fetchText` is a *test helper*: a value that only tests may use. There
is no keyword for it. A top-level value is a test helper when its
signature or body uses something only a test may use:

- anything of `Test`: the steps, `Test.run`, `Test.trace`, `Test.fake`,
  `Test.login`, `Test.member`, and the queries `Test.text`, `Test.has`, …;
- the Prelude's test types `Doc` and `Answer` (a `Doc -> Text` value is
  a helper), or a constructor of `Answer`;
- a fake's table (section 8);
- another test helper.

A `test`, or another test helper, in any module, may use one. Production
code may not, and some values can never be helpers, because the compiler
or the runtime calls them: a route's `load`, `view` and `update`, every
value of `App`, a fake's entries, a brand's `where` check and a field's
default. A reference from one of those to a helper is E0040, naming the
helper and the first thing that makes it one:

    `fetchText` is a test helper (it uses `Test.fetch`): only a `test` or another test helper may use it

and along a chain, `` `at400` is a test helper (it uses `fetchText`, a
test helper) ``. Using a test-only thing directly in one of them is E0040
at the use: "`Test.fetch` is only available in a `test` or a test
helper: `Index.view` is the route's view, not a helper" (likewise "a fake's
entry is not a helper", "a field default is not a helper", "a brand
check is not a helper", "`App.update` is not a helper").

`alder build` leaves every test helper out of the program, and `alder
test --mutate` makes no mutant in one (section 11). The browser's rules
still hold inside a helper: `Test.fetch (Sites.MnSos.MnSos { cfg: cfg })` is a
route written in place with the helper's parameter inside it, while
`Test.fetch p`, with `p` a parameter, is refused as in a test. A fake is
still declared in its capability's module (E0020); a helper shares the
`Test.fake` call, not the fake. The editor's hover on a helper says
which it is: "test helper (uses `Test.fetch`)".

## 10. How the browser works

Section 9 can be used without this one. This section explains what
happens when a step runs, so that a surprising trace or answer can be
read.

**The real routes, in memory.** The test program is built with the
app's routes, the same list the app's binary serves, and the same router
(`rt/web`). A step does not open a network connection: it builds an HTTP
request in memory and hands it to the router, which calls the route's
handler and writes the response into memory, where the browser reads it.
Everything between, from reading `/todos/x` into a route's parameters to
decoding the form, checking `requires`, running `load` and `update`,
rendering and setting the cookie, is the code that runs in production.

**Inside the test's transaction.** A test's database exists in memory,
with one connection; the test's block holds a transaction on it from its
first database step (section 3). In the running app, each request begins
a transaction of its own. In a test, that would wait for ever for the
one connection, and would not see the test's uncommitted rows anyway. So
the browser gives the router each request's environment itself, one that
runs inside the test's transaction. A `load` is a `Query`, which cannot
write, so it simply reads through that transaction. An `update` is a
`Task`, and runs under a savepoint (section 4) instead of a transaction
of its own:

```text
test "adding a todo through the page"   (a fresh database in memory)

  BEGIN                                    first database step
  │ Db.insert User
  │ Test.login                             the cookie; no request
  │ Test.get                               GET /
  │   Db.mine Todo, Db.all Todo            load reads in the test's transaction
  │ Test.submit                            POST /_/Submit
  │   SAVEPOINT alder_t1                   update is a Task
  │   │ Db.insertMine Todo
  │   └ RELEASE alder_t1                   it succeeded: the insert is the test's now
  │   Db.mine Todo, Db.all Todo            GET /: the redirect, followed
  │ Test.get                               GET /
  │   Db.mine Todo, Db.all Todo
  COMMIT                                   the verdict
```

When an `update` fails, its savepoint is rolled back (`ROLLBACK TO`),
so its writes are gone and the test's are not.

**The before-view.** `Submit` answers `redirect`, so its request reads
nothing before the insert: there is no `Db.all Todo` ahead of
`Db.insertMine Todo` in the trace above. A write route whose `update` can
answer `Response.refresh` or `Response.invalid` is different. The router
renders the page the browser is on first (the address in the `Alder-Page`
header, run as a `GET` of its route), so that after the `update` it can
compare the two renders and send only the regions that changed (chapter
6, "A write's response, with JavaScript"). A click on `Toggle`, which
answers `refresh`, shows it in the trace: the page's `Db.mine Todo` and
`Db.all Todo` come before `Db.updateMine Todo`, and again after it. A
route that only `redirect`s or `render`s reads nothing it does not need.

**The model.** `Test.get`'s `model` is not computed again: the page's
route stores the model it rendered on the request (production does too;
nothing reads it there), and the router hands each answered request to
the browser, which keeps that model. So `model` is exactly what `view`
was given, even when `load` read the clock.

**A Datastar client.** In the running app, a click or a form post is
sent by Datastar (chapter 6), which marks the request with a
`Datastar-Request: true` header and gets back events instead of a page.
The browser sends `Test.submit` and `Test.click` the same way and
applies the events as Datastar would:

- a patch of the page's `<body>` replaces the body it holds (Datastar
  merges the new body into the old one; the result is the same page).
  A `refresh` or `invalid` that the router cannot cut into regions
  safely comes back this way, and so does a followed redirect. A page
  whose `view` has no `<body>` (every page built on a layout such as
  `Layout.Page` has one) cannot be patched, and the step fails "the held
  page has no <body> to patch".
- a patch of one element, selected by its `id` (mode outer), replaces
  that element in the page held. These are the regions of a `refresh` or
  an `invalid`, and the one element of a `render` or of an element fetch.
  The held page without an element of that id cannot take it, and the
  step fails ("… which the page does not have").
- a toast is appended to the page's `<al-toasts>` and read into the
  step's `notices` and `toasts`.
- an instruction to go to another address is a `Redirect`: the browser
  requests that page with a plain `GET`, as Datastar has the real
  browser do, and the answer is `Redirected`. An address on another site
  is not requested.

Which answer a step gives is read from the route's `Response`, as the
router handed it to the browser: a `render r` is `Rendered r` (once the
element check has passed), a `refresh` is `Refreshed`, an `invalid` is
`FormRejected` with the errors the page now shows in its `<Errors>`
lists. A 404 or a 500, which come back as a patch too (with the status
in a header, `Alder-Status`), are `PageMissing` and `RequestFailed`.
`Rerendered` is what the old page-and-message form's re-render came to.

`Test.get` and `Test.open` are plain `GET`s, like typing an address. A
`303` answer, such as `requires` sending a visitor to `/login`, is
followed (at most ten in a row).

**Controls.** The page's buttons and forms request routes by the
routes' own addresses (`/_/Toggle?id=…`, `/_/Submit?filter=Active`:
chapter 6). Before it sends anything, the browser looks through the held
page for a form or button whose method and address name the same route
with the same parameters, and sends that control's request, with the
headers the page's script adds: `Alder-Page`, the address of the page held
(the `url` of the last step), which `refresh` and `invalid` need to know which page to
render again; `Alder-Build`; `Alder-Fetch: 1` for a GET route fetched
into the page, and for a write the `Alder-Shown` hash of the body held. A
form sends the fields the held form has, as a browser would; a button
that is not in a form sends an empty payload.

**The checks.** The router hands the browser the full render it made of
the page to answer a request (`refresh`, `invalid`, a followed redirect).
After applying the patches, the browser parses that render the way its
HTML parser builds a page and compares it with the page it now holds: the
whole body when the answer replaced the body, else only the regions the
answer patched, each by its `id`. That is the region check. For a
`render` or an element fetch it asks the router for the route's own
address once more, as an element fetch, inside a savepoint that it rolls
back, and compares the element in the page held with the element that
comes back. That is the element check. `Test.refresh` sends a plain `GET`
for the address held, as a reload does, and compares the body it gets
with the body held, leaving out `<al-toasts>`.

**The document.** The router's HTML is parsed into a tree, and the
browser holds that, without the `<!doctype html>` line and the Datastar
`<script>` tag the router adds to every page, so the `Doc` is the page as
`view` renders it. The queries walk that tree.

**The cookie.** The browser keeps the session cookie the router sets and
sends it back with each request; `Test.login` writes the cookie itself,
signed with the router's key. A cookie the router clears (a logout)
is dropped. The cookie's clock stands still, so it never expires
during a test.

## 11. Mutation testing

Passing tests say the code does what they check, not that they check
what matters. This section measures that, and turns it into tests.

### Why a mutation score

Many languages measure tests by **line coverage**: the share of the
program's lines that ran while the tests ran. In Alder that number says
almost nothing. The checker already makes every `match` complete and
every function total, and one browser test that opens a page runs all of
its `load` and its `view`. A line can run and still go unchecked: "a
stranger cannot see my todo" runs `Index.load` of `Todos.Index`, `Db.orderBy`
included, and nothing in it notices if the order is backwards.

A **mutant** is a copy of the app with one small, deliberate bug: a
check removed, a write that does nothing, an order reversed, a
scoped operation made unscoped. If some
test fails when it runs against the mutant, the mutant is **killed**.
If every test still passes, it **survived**, and that is a bug the tests
would have let through. The **mutation score** is the share of mutants
killed. Unlike line coverage, it asks the question that matters: if this
line were wrong, would a test fail?

The mutants are not random edits. Each operator below is a change that
breaks something a server-rendered app must get right. Every mutant is
well-typed: it compiles, so a test that fails against it fails because
of what it does, not because it is broken code.

### The command

```text
alder test --mutate [--min R] [--json] [-run re] [-timeout d] [dir]
```

It runs the tests as `alder test` does; if one fails, it stops with
`tests fail: fix them before measuring mutants`. Otherwise it runs them
again once per mutant. Here is the to-do app's own code (the generated
admin of chapter 15 adds 94 mutants of its own) with the eleven tests it
had before this section's work, the others left out with `-run`:

```text
11 passed, 0 failed
30 mutants: 16 killed, 14 survived  (score 0.53)
survived:
  App           Logout.update:16   update field dropped   { s | user: Nothing, role: Member } → { s | user: Nothing }
  Auth.Login    Submit.update:42   update field dropped   { s | user: Just id, role: role } → { s | user: Just id }
  Table.Todo    Todo:16            cascade ↔ restrict     owner @ref User @restrict → owner @ref User @cascade
  Table.Todo    matches:21         arm swapped            match filter { All -> True, Active -> not todo.done, Complete…
  Table.Todo    matches:23         not dropped            not todo.done → todo.done
  Table.Todo    filterLabel:28     arm swapped            match filter { All -> "Al… → the All and Active bodies swapped
  Table.Todo    filterLabel:28     arm swapped            match filter { All -> "All", Active -> "Active", Completed ->…
  Todos.Index   Index.load:18      orderBy reversed       Db.orderBy Todo.created.desc → Db.orderBy Todo.created.asc
  Todos.Show    Save.update:27     write replaced         let _ <- Db.updateMine Todo p.id … → let _ <- Task.succeed ()
  Todos.Show    Save.update:27     scope dropped          Db.updateMine Todo p.id (\t -> { t | title: p.form.title, due: p.form.due }) → Db.update Todo p.id …, not found when missing
  Todos.Show    Save.update:27     update field dropped   { t | title: p.form.title, due: p.form.due } → { t | due: p.form.due }
  Todos.Show    Save.update:27     update field dropped   { t | title: p.form.title, due: p.form.due } → { t | title: p.form.title }
  Todos.Show    Delete.update:33   write replaced         let _ <- Db.deleteMine Todo p.id → let _ <- Task.succeed ()
  Todos.Show    Delete.update:33   scope dropped          Db.deleteMine Todo p.id → Db.delete Todo p.id, not found when missing
```

A test failed for 16 of the 30 mutants: the score is 16 / 30. A
survivor's line gives its module; `declaration:line` (`Index.load:18` is
line 18 of `app/Todos/Index.alder`; a route's members are named as in the
source, `Save.update:27`; a column is added, `load:33:17`, when the line
has two mutants of that kind); the operator; and the code before `→`
after, shortened with `…` (an arm swap names its arms).

### The operators

Each mutant comes from one of twenty-one **operators**, named by what it
breaks, in five classes; every mutant is well-typed:

| Class | Operator | What it changes |
|---|---|---|
| authorization | `requires dropped` | A page's `requires user` no longer redirects a visitor, who is served as `User.UserId "mutant"` (for a field that is a brand over `Text`); old `page` form only |
| authorization | `role dropped` | `requires user, role R` becomes `requires user`; old `page` form only |
| authorization | `scope dropped` | `Db.mine T` becomes `Db.from T`; the other scoped operations act on anyone's row; likewise `Db.scoped` and its family (not on a siloed table) |
| authorization | `membership dropped` | A key's `via M` check removed: a `Session.put` of any org succeeds |
| authorization | `guard dropped` | An `if` or `match` with one arm ending in `Task.notFound`, `Query.notFound` or `Task.fail` becomes its other arm: the check is gone |
| data | `where dropped` | One `Db.where` of a query removed |
| data | `orderBy reversed` | A `Db.orderBy`'s `asc` and `desc` swapped |
| data | `limit dropped` | A query's `Db.limit` removed |
| data | `offset dropped` | A query's `Db.offset` removed |
| data | `write replaced` | A `let … <-` insert, update or delete becomes `Task.succeed` of a placeholder (`()`, or an id `"mutant"`) |
| data | `update field dropped` | One field of a record update `{ t \| a: …, b: … }` that sets two or more left out, wherever it is |
| data | `session put dropped` | A `Session.put` step becomes `Task.succeed ()` |
| flow | `redirect → rerender` | `Redirect page` becomes `Rerender`; old `Nav` only |
| flow | `reject → rerender` | `Reject "message"` becomes `Rerender`; old `Nav` only |
| flow | `arm swapped` | Two neighbouring arms of a `match` trade bodies (one mutant per pair) |
| logic | `condition negated` | `if c` becomes `if not c` |
| logic | `operator flipped` | `==`↔`!=`, `<`↔`<=`, `>`↔`>=`, `&&`↔`\|\|` |
| logic | `bool flipped` | `True`↔`False` |
| logic | `not dropped` | `not e` becomes `e` |
| schema | `default changed` | A table field's default: `False`↔`True`, a number → one more, `""`→`"x"`, a union's constructor → the next one |
| schema | `cascade ↔ restrict` | A reference's delete rule, `@cascade`↔`@restrict` |

Four operators are for what chapter 6 replaced: a page's `requires` and
`Nav`. A route's `requires` and `Response` are not mutated yet (alder#187),
so the browser tests alone hold them (section 9): a visitor sent to log
in, the wrong role, the answer a click comes to.

**The partition mutants.** `scope dropped` is made for every key: in
the workspace, `Db.scoped Activity` → `Db.from Activity` is killed by
`Members.Index "the members, by name, and the activity of this org
only"`, whose page would show both orgs' lines. On a table in a `silo`
the run does not make it: the org's database holds that org's rows only,
so `Db.from Project` reads the same rows as `Db.scoped Project` (the
report's equivalence line says "the silo's database holds this org's
rows only"). The one mutant of a `via` field reads

```text
  App                      session:13      membership dropped     Session.put org → Session.put org without the Membership check
```

and is killed only by a test that tries a key the user is no member of:
`Orgs.Index "a non-member's switch is refused"` in the workspace. An app
whose tests never try is told so by its score.

### The rules' mutants in a report

The two operators for the declared rules of chapter 6 ("Rules") that a
route has are `scope dropped` and `membership dropped`. `scope dropped`
reads like this in a report. From `alder test --mutate -run
'^Table\.Todo ' examples/todos`, whose four tests open no page, the to-do
app's six:

```text
  Todos.Index   Index.load:17      scope dropped          Db.mine Todo → Db.from Todo
  Todos.Index   Toggle.update:37   scope dropped          Db.updateMine Todo p.id (\t -> { t | done: not t.done }) → Db.update Todo p.id (\t -> { t | done: not t.done }), not found when missing
  Todos.Index   Delete.update:43   scope dropped          Db.deleteMine Todo p.id → Db.delete Todo p.id, not found when missing
  Todos.Show    Show.load:19       scope dropped          Db.getMine Todo params.id → Db.get Todo params.id, not found when missing
  Todos.Show    Save.update:27     scope dropped          Db.updateMine Todo p.id (\t -> { t | title: p.form.title, due: p.form.due }) → Db.update Todo p.id …, not found when missing
  Todos.Show    Delete.update:33   scope dropped          Db.deleteMine Todo p.id → Db.delete Todo p.id, not found when missing
```

"Not found when missing" is the replacement's whole behaviour: the
mutant still answers 404 for an id that names no row, so only a test
with someone else's row tells it from the app. `Db.insertMine` gets no
`scope dropped` mutant (it is skipped, "the owner comes from the session
either way"), but its write can be replaced. Each of the six is killed by
a test that acts as a stranger: "a stranger cannot see my todo" (the
list's `Db.mine` and `Todos.Show`'s `Db.getMine`), "Toggle by another
user is not found" and "Delete by another user is not found" (a stranger
presses my todo's button on a page still open in the browser), and
"saving or deleting another user's todo is not found" (the same for the
todo's own page). Of the first two, "Toggle flips done" kills the
`Toggle` mutant before them, since its trace says `Db.updateMine Todo`.

### A surviving mutant is a test you have not written

Each survivor is a behaviour no test checks: write the test that would
notice. `Todos.Show Save.update:27` has four of them in the report above:
the save could store nothing, drop the title, drop the due date, or touch
anyone's todo. One test that saves the form and reads the row back kills
the first three:

```alder
test "saving changes the title and the due date" = {
  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
  let id <- Db.insert Todo { title: Text.NonEmpty "Milk", due: Nothing, owner: me }
  let _ <- Test.login { user: Just me }
  let _ <- Test.get (Todos.Show.Show { id: id })
  let r <- Test.submit (Todos.Show.Save { id: id }) { title: Text.NonEmpty "Bread", due: Date.fromText "2026-10-12" }
  let after <- Db.get Todo id
  Task.succeed ({ answer: r.answer, title: Maybe.map (\t -> t.title) after, due: Maybe.andThen (\t -> t.due) after } == { answer: Redirected (Todos.Index.Index {}), title: Just (Text.NonEmpty "Bread"), due: Date.fromText "2026-10-12" })
}
```

Nine tests of this kind kill all fourteen: the app's own code then ends
`30 mutants: 30 killed, 0 survived  (score 1.00)`.

### The mutant that needs a stranger

The fourth, `scope dropped`, needs a stranger. A write route takes its id
from the request and no `load` runs before it, so the check that the todo
is the user's is `Db.updateMine`'s alone. The test keeps my page open, logs
in as someone else, and saves and deletes my todo:

```alder
test "saving or deleting another user's todo is not found" = {
  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
  let id <- Db.insert Todo { title: Text.NonEmpty "Milk", due: Nothing, owner: me }
  let _ <- Test.login { user: Just me }
  let _ <- Test.get (Todos.Show.Show { id: id })
  let _ <- Test.login { user: Just (User.UserId "someone-else") }
  let saved <- Test.submit (Todos.Show.Save { id: id }) { title: Text.NonEmpty "Bread", due: Nothing }
  let _ <- Test.login { user: Just me }
  let _ <- Test.get (Todos.Show.Show { id: id })
  let _ <- Test.login { user: Just (User.UserId "someone-else") }
  let deleted <- Test.click (Todos.Show.Delete { id: id })
  let after <- Db.get Todo id
  Task.succeed ({ saved: saved.answer, deleted: deleted.answer, todo: Maybe.map (\t -> t.title) after } == { saved: PageMissing, deleted: PageMissing, todo: Just (Text.NonEmpty "Milk") })
}
```

### How the two tests kill them

The second test reads the row back with `Db.get`, so a save that wrote
nothing, or left a field out, shows in `title` or `due`; and it checks
the answer, so a redirect that stayed on the page shows too. (`Date.fromText`
gives a `Maybe Date`, which is the field's type.) In the module the form is
written `EditTodo` and the routes `Show` and `Save`; this chapter's blocks
are checked as a module of their own, so they are qualified here.

The stranger's test kills the `scope dropped` mutant by what it does to
the answer. Against the app, `Db.updateMine` finds no todo of the
stranger's with that id and the save ends in `Task.notFound`: the answer
is `PageMissing` and the todo is as it was. Against the mutant, `Db.update`
finds my todo, writes it, and the answer is a redirect: the test fails, and
the mutant is killed. The same test kills `Delete`'s `scope dropped` and
its `write replaced`.

### All fourteen, and one killed by luck

Nine tests kill the fourteen. "Logout makes the role Member" and
"logging in copies the role from the user's row" kill the two `update
field dropped` of the role; "a user with todos cannot be deleted" the
`cascade ↔ restrict`; "Active keeps only open todos" and "each filter has
its label" the swapped arms and the dropped `not` of `matches` and
`filterLabel`; "the newest todo is listed first" the reversed
`orderBy`; and the three of `Todos.Show` above. `examples/todos/README.md`
lists the example's tests with what each kills. The report had no
`requires dropped` or `redirect → rerender` line, because a route has
neither mutant, so "a visitor is sent to log in from a todo" and "Logout
clears the user and goes to log in" are in the example for the rules
they check (section 9, and "The logout redirect" below), not because a
survivor asked for them.

One kill is worth a word. `Table.Todo countOpen:37 not dropped` (`not
t.done → t.done`) counts the done todos instead of the open ones. A test
with one open todo and one done one gives 1 either way and notices
nothing; "countOpen ignores done todos" has two open todos and one done,
so the mutant gives 1 where the test wants 2. After the tests, the whole
example, admin included, reads:

```text
$ alder test --mutate examples/todos
…
126 passed, 0 failed
124 mutants: 122 killed, 2 survived  (score 0.98)
survived:
  Admin.Todos.New    New.load:33   operator flipped   ownersTotal > 200 → ownersTotal >= 200
  Admin.Users.Bulk   selected:44   where dropped      Db.where (Todo.owner.in taken) → the condition dropped
```

(`…` stands for the 126 `ok` lines.) The app's own code has 30 mutants
and all are killed by its 23 tests: the nine above; two that check rules and kill
no mutant ("Logout clears the user…", "a visitor is sent to log in from a todo");
and twelve older ones (`Todos.Index`'s eight, `Auth.Login`'s three, "countOpen…"). Two of the nine exist for the
session's `role`, which the admin of chapter 15 needed: `App "Logout makes
the role Member"` and `Auth.Login "logging in copies the role from the
user's row"` kill the `update field dropped` of `role` in logout's and
login's `Session.put`. The generated admin brings 94 mutants and 103 tests
of its own, which kill all but two. Those two are equivalent mutants
(below), and the example's README says why.

### What is never a mutant

Mutants are made only in the app's own code: the `load`, `update`,
helpers and plain values of every module, and its tables. Never in a
`test`, a `fake`, a test helper (section 9, "Test helpers"), a `view` or
a component (anything that makes `Html`), or the standard library.

Some changes are never made, and are not counted in the score. Two kinds
are reported apart, in `--json`'s last object (below):

- **Suppressed** (`suppressed`): the compiler can prove the mutant is
  the same program as the original. The rules: an `arm swapped` of two
  identical bodies; an `orderBy reversed` under `Db.first` on a query by
  id; a `condition negated` of a literal `True` or `False`; an `operator
  flipped` whose two sides are the same expression; and a `default
  changed` on a field that no record of the app's code leaves out (no
  `Db.insert` row, no `Insert T` a helper builds), so the default never
  applies (the blog's `User.role`, which only tests rely on); and, in
  the page-and-message form, a `scope dropped` of a `Db.updateMine` or
  `Db.deleteMine` in a page's `update` whose id is `model.f.id`, where
  the page's `load` filled `f` with a row of the same table read by
  `Db.getMine` ("load fetched it with getMine": every message runs
  `load` first, so the id is always the user's own row). A write route
  has no such `load`: its id comes from the request, and its `scope
  dropped` mutants are never suppressed (the six of the report above).
- **Skipped** (`skipped`): the operator does not apply, or another
  operator already makes the same program. A `condition negated` of `if
  a == b` (or `!=`) is skipped, because `operator flipped` already makes
  `if a != b`; so is one of `if not e`, which `not dropped` makes. (An
  ordering condition is negated: `<` flips to `<=`, while its negation is
  `>=`.) A `guard dropped` or an `arm swapped` is skipped where an arm
  uses what its pattern binds (a `match` on a `Maybe` whose `Just x` arm
  uses `x` cannot trade bodies with the `Nothing` arm); a `requires` on a field that is not a brand
  over `Text`, a `Maybe` default and an `@setNull` rule have no
  well-typed change; and `Db.insertMine` has no `scope dropped`, since
  the owner comes from the session either way.

The to-do app has 1 suppressed (`User.role`'s default, which no row of
the app's code leaves out) and 27 skipped (2 in its own code, the `scope
dropped` of `Db.insertMine` among them, and 25 in its generated admin);
the blog 1 (the same) and 41.

### Survivors that are not gaps

Sometimes no test can tell a survivor from the app: then say why in a
comment or a README; otherwise write the test. No annotation silences a
survivor: it stays until a test kills it or the code changes.

### The logout redirect

Every Log out button of the to-do app is on a page that `requires user`.
"a visitor is sent to log in, and can log out" clicks it and checks that
the answer is `Redirected (Auth.Login.Login {})`. The logout would end at
the login page even if `Logout.update` answered `Response.refresh`: the
browser asks for the same page again, with the user now gone from the
session, and `requires user` redirects it to the login page. Either way
the user ends up there, so no browser test can tell `redirect` from
`refresh` here. A test of `Logout.update` itself can, since it sees the
`Response` before any browser follows it. (With the page-and-message
form this was a survivor, `redirect → rerender`; a route has no such
mutant, but the test still checks something a browser cannot.) The example
has one. It must be in `app/App.alder`: `Logout.update` is called by the
runtime, and a test anywhere else that calls it is E0002. (This block is
not checked on its own, since it belongs in that module; it is the
file's own text.)

```alder fragment
test "Logout clears the user and goes to log in" = {
  let r <- Test.run (Logout.update {} { user: Just (User.UserId "me"), role: Admin })
  Task.succeed ({ value: r.value, trace: r.trace } == { value: Ok (Response.redirect (Auth.Login.Login {})), trace: ["Session.put"] })
}
```

(The blog has a Log out button on its front page, which a visitor may
open, so there a browser test tells the two apart.)

### Mutants that are the same program

With all its tests (those of its rules, and those of its generated admin),
the blog keeps six survivors (here and below, `…` stands for the 196 `ok`
lines):

```text
$ alder test --mutate examples/blog
…
196 passed, 0 failed
202 mutants: 196 killed, 6 survived  (score 0.97)
survived:
  Admin.Comments.New   New.load:34        operator flipped   postsTotal > 200 → postsTotal >= 200
  Admin.Posts.New      New.load:33        operator flipped   authorsTotal > 200 → authorsTotal >= 200
  Admin.Users.Bulk     selected:44        where dropped      Db.where (Post.author.in taken) → the condition dropped
  Posts.Index          Index.load:33:17   operator flipped   params.page < 1 → params.page <= 1
  Posts.Index          Index.load:33:48   operator flipped   params.page > lastPage → params.page >= lastPage
  Posts.Index          Index.load:42      limit dropped      Db.limit (perPage + 1) → no limit
```

The first three are the generated admin's, equivalent as the to-do
admin's two are (the blog's README says why). The other three are in
the blog's own code. The first line of `Posts.Index`'s `load` clamps the
page number:

```alder fragment
  let page = if params.page < 1 then 1 else if params.page > lastPage then lastPage else params.page
```

Two `operator flipped` mutants sit on this line, so their lines carry
the column: `Index.load:33:17` is `params.page < 1`, `Index.load:33:48` is
`params.page > lastPage`. `<` and `<=` differ only when `params.page` is 1, and then
both branches give 1; `>` and `>=` differ only at `lastPage`, where both
give `lastPage`. No input tells the mutant from the app. (The line's two
`condition negated` mutants, `if not (params.page < 1)` and `if not
(params.page > lastPage)`, are killed: negating either clamp sends an
ordinary page number to the wrong branch, and "page 99 is empty" and
the front page's test notice.) The third is `limit
dropped`: `load` asks for `perPage + 1` posts, keeps `List.take perPage`
of them, and sets `hasMore` when more than `perPage` came back. Without
the limit, the query reads every matching post and the page is the same,
only slower. These three are **equivalent mutants**: the compiler cannot
prove them equivalent with its few structural rules, so it makes them,
and they survive honestly. `examples/blog/README.md` records them with
the reason for each.

### `--min` in CI

`--min R` (0 to 1) exits 1 when the exact score is below `R`; without
it the exit code is the tests'.

### Properties under `--mutate`

A property (section 12) is one test here: a mutant is killed when it
fails on any of the property's cases. A mutated run uses a fixed seed
(0, or the one `--seed` gives), does not shrink, and neither reads nor
writes saved failures, so the same code gives the same score. A `Fuzzer`
value (a generator, such as `slugs` in section 12) is not part of the
app's behaviour and gets no mutants.

### The exact score

With no survivor the report ends at the score line. The comparison
uses the exact score, not the two decimals printed: the blog's is 196 /
202 = 0.9703…, printed as 0.97, so

```text
$ alder test --mutate --min 0.971 examples/blog
…
196 passed, 0 failed
202 mutants: 196 killed, 6 survived  (score 0.97)
survived:
  Admin.Comments.New   New.load:34        operator flipped   postsTotal > 200 → postsTotal >= 200
  Admin.Posts.New      New.load:33        operator flipped   authorsTotal > 200 → authorsTotal >= 200
  Admin.Users.Bulk     selected:44        where dropped      Db.where (Post.author.in taken) → the condition dropped
  Posts.Index          Index.load:33:17   operator flipped   params.page < 1 → params.page <= 1
  Posts.Index          Index.load:33:48   operator flipped   params.page > lastPage → params.page >= lastPage
  Posts.Index          Index.load:42      limit dropped      Db.limit (perPage + 1) → no limit
$ echo $?
1
```

while `--min 0.97` passes. A CI job runs `alder test --mutate --min R`
with `R` at the score the app has reached, so a change that lets a
mutant survive fails the build. The examples do this in the Go test
suite: `TestExample_TodosMutation` and `TestExample_Blog` require at
least 0.98 and 0.97, and the number of mutants their READMEs record.
Without `--min`, the exit code is the tests'.

### `-run`, `-timeout` and `--json`

`-run re` selects tests as it does without `--mutate`, and the mutants
are judged by the selected tests only. With only `Table.Todo`'s four tests,
most mutants survive:

```text
$ alder test --mutate -run '^Table\.Todo ' examples/todos
ok   Table.Todo "countOpen ignores done todos"
ok   Table.Todo "Active keeps only open todos"
ok   Table.Todo "each filter has its label"
ok   Table.Todo "a user with todos cannot be deleted"
4 passed, 0 failed
124 mutants: 7 killed, 117 survived  (score 0.06)
survived:
…
```

(117 survivor lines follow: the six `scope dropped` of "The rules' mutants
in a report" among them, and all 94 of the generated admin's, which only
its own tests kill.) This is useful while writing the tests of
one module, but the score that counts is the whole suite's.

`-timeout d` bounds each run of the tests: the plain run first, then each
mutant's (default 10 s with `--mutate`, rather than 5 m, since it applies
once per mutant). A mutant's run gets at least three times as long as
the plain run took, so a suite that takes 7 s under the default 10 s
gives each mutant 21 s, and a mutant is not killed by a slow moment of
the machine; when the plain run takes more than half of `d`, a line on
standard error says so (`alder test: the tests took 7.2s, over half the
-timeout 10s: each mutant gets 21.6s (3 × the tests' time)`). A mutant
that makes a test loop for ever is killed when its time is up (`timed
out`; the score line then says `K killed (of which T timed out)`), and
one that crashes the test program, a stack overflow say, is killed too
(`crashed`): `alder test` starts the program again from the next
mutant. If the plain run itself runs out of time, the message says
`(--mutate's default 10s; raise it with -timeout)`. A `-run` that
selects no test ends with `no test matches -run: nothing can kill a
mutant`. Mutants run one after another, each with a fresh database
per test; the to-do app's 124 take a few seconds once the build is made,
because each mutant stops at the first test that fails, and the tests of
the mutant's own module run first, where the killing test almost always is.

`--json` prints the per-test objects as usual, then one object listing
every mutant, then one with the totals. A mutant's object has its `id`,
`module`, `file`, `decl`, `line`, `col` (the column), `kind`, `before`,
`after`, whether it was `killed`, `by` which test (the first that
failed), and a `note` (`"crashed"`, `"timed out"`, or empty). From the
to-do app's run, two of the 124:

```text
{"id":110,"module":"Todos.Index","file":"app/Todos/Index.alder","decl":"Index.load","line":18,"col":10,"kind":"orderBy reversed","before":"Db.orderBy Todo.created.desc","after":"Db.orderBy Todo.created.asc","killed":true,"by":"Todos.Index \"the newest todo is listed first\"","note":""}
{"id":5,"module":"Table.Todo","file":"app/Table/Todo.alder","decl":"Todo","line":16,"col":3,"kind":"cascade ↔ restrict","before":"owner @ref User @restrict","after":"owner @ref User @cascade","killed":true,"by":"Table.Todo \"a user with todos cannot be deleted\"","note":"","tables":"ensure"}
```

and the last line:

```text
{"score":0.9838709677419355,"killed":122,"survived":2,"suppressed":1,"skipped":27}
```

`score` is exact here (122 / 124). `"tables":
"ensure"` marks a **schema mutant** (`default changed`, `cascade ↔
restrict`): its tests run on tables made from the mutated table
declarations by `Ensure`, as for an app without `migrations/`
(chapter 8, "Migrations"), not from the app's migrations, since the question is
whether a test would notice the changed rule, not whether a migration
exists for it.

### Ids and lines change with the code

Mutants are numbered in a fixed order: module by module, and within a
module its tables field by field, then its declarations in order, each
from the outside in. The same source always gives the same numbers, so
two runs can be compared. An edit renumbers every mutant after it, and
moves the lines. Modules come in import order (a module after those it
imports, otherwise alphabetical), so adding an import can move a whole
module and renumber mutants in code the edit never touched. The generated
admin's modules come before `Todos.Index` in that order, so a mutant
of the to-do list's `load` is number 110 here. Ids and lines are for
reading one report; to compare runs, compare what survived.

## 12. Properties

A test checks one example: `countOpen` of three todos is 2. It passes,
and says nothing about four todos, or none, or a thousand. A **property**
is a statement that should hold for every input of some type, and a
property test checks it against many inputs the tool makes up. You write
the statement; `alder test` writes the examples, a hundred of them by
default, and when one fails it shrinks the example to the smallest one
that still fails and prints it as a test you can keep.

### The function form

A test whose body is a function is a property. Its parameters are the
inputs, and its body, as in section 1, is a `Bool`:

```alder
-- moves the first element to the end: a rotation, not a reverse
rotate : List a -> List a
rotate xs = List.append (List.drop 1 xs) (List.take 1 xs)

test "rotate keeps the length" = \(xs : List Int) -> List.length (rotate xs) == List.length xs

test "rotate twice" = \xs -> rotate (rotate xs) == xs
```

`\(xs : List Int) -> …` is a lambda (chapter 3) whose parameter is
written with its type, in parentheses; a bare `\xs ->` is the same
without the type. There is no new declaration and no library: the
property is a `test`, it is listed in the report like any other, and
`-run` (section 7) picks it by name. The first one passes. The second is
wrong, as the report below will show: rotating a list twice gives it
back only when it has at most two elements.

### Where the inputs come from

`alder test` has to make a `List Int` out of nothing, and it does it from
the **type**. The checker works out the type of each parameter (the
annotation, or what the body forces), and the compiler writes, for that
type, a **generator**: a function that builds one value of the type from
a stream of random choices. Every ordinary type has one:

- `Int`, `Float`, `Bool`, `Text`, `Unit`, `Bytes`, `Date` and `Time`.
  The distributions are chosen to find bugs, not to be uniform: an `Int`
  is mostly small (over half the draws are 0 to 10), sometimes up to a
  thousand, sometimes large, and now and then a boundary such as 255,
  2^31 - 1 or the largest `Int`; a `Float` is finite (no `NaN`, no
  infinity) and includes 0, tiny and huge values; a `Text` is mostly
  short ASCII, sometimes Unicode, sometimes empty, and made only of
  printable characters and `\n`, `\t` and `\r`, so that every value can
  be written back as a literal; a `Date` or `Time` is within a few years
  of the epoch or of 2026.
- `Maybe a`, `Result e a`, `List a` and `Dict k v` are built from the
  generators of their parts, and a list is short most of the time.
- A record is each field's generator in turn.
- A union is a choice of one constructor, then a generator for its
  payload. A union that contains itself (a tree) has a depth budget of four
  levels: each level down halves the chance of a constructor that
  recurses, and once the budget is spent only the others are drawn, so the
  values are finite. (A union whose every constructor needs the union
  again has no value to stop at: E0038, below.)
- A brand draws its base type and, when it has a `where` predicate,
  keeps the value only if the predicate holds. Below.

Some types have no generator: `Html`, `Route`, a function, a `Task`, a
`Query`, a capability, a `Form`. A property over one is E0038 (below),
and you write the generator yourself with `Fuzz.forAll`.

### `Int` when the type is open

In `\xs -> rotate (rotate xs) == xs` nothing says what the elements are:
`rotate` works on a list of anything, and `==` on anything comparable, so
the type of `xs` is `List a` for an `a` that nothing decides. Rather than
refuse, the checker **defaults** a type variable that is left open in a
property's parameter to `Int`, and the report says so (below), so that
the choice is not hidden. `Int` is the simplest type that shows most
bugs. When the property is about something else, annotate the parameter:

```alder
test "unique never grows a list of text" = \(xs : List Text) -> List.length (List.unique xs) <= List.length xs
```

An annotation is how you pick any type, and the only way to get a type
the body does not force, such as a brand. Each lambda parameter is a bare
name or `(name : Type)`; the annotated form works in every lambda, not
only in a test.

### Brands as constraints

A brand with a `where` (chapter 4) is a constraint, and a property over
it draws only values that satisfy it:

```alder
type Age = brand Int where (\n -> n >= 0 && n < 150)

test "ages are under 100" = \(a : Age) -> a < 100
```

The generator for `Age` draws an `Int` and throws away the ones the
predicate refuses. For this brand that wastes few draws. For a brand like
`Slug = brand Text where Text.matches "^[a-z0-9-]+$"` it wastes nearly
all of them: a random `Text` is almost never a slug. A generator that
rejected 94% of its draws would run the property on a handful of values,
or on none and pass, so `alder test` checks, and fails the property with
the reason instead:

```text
FAIL Probe "a slug is trimmed"
  error: `Slug`'s predicate rejects 94% of generated values; write a `Fuzz.forAll` with a generator that produces `Slug` values
  seed: 1061676061
```

The check looks after the first 50 attempts: more than 90% rejected is a
failure. The six brands of the standard library (`Text.NonEmpty`,
`Text.Email`, `Text.Url`, `Text.Secret`, `Int.Positive`,
`Int.NonNegative`) are not made by filtering; each has a generator that
draws values of its own shape directly, so they never trip it. For your
own tight brand, write the generator.

### `Fuzz.forAll` and shared `Fuzzer`s

`Fuzz.forAll gen f` is the explicit form: the test's body is the
property, `gen` is a `Fuzzer a` that you build, and `f` takes one `a`.
The `Fuzz` module (chapter 9) has the pieces: `Fuzz.int`,
`Fuzz.intRange { min: 0, max: 5 }`, `Fuzz.text`, `Fuzz.list`,
`Fuzz.listOf { min: 1, max: 5 }`, `Fuzz.maybe`, `Fuzz.oneOfValues`,
`Fuzz.map`, `Fuzz.filter` and the rest. A `Fuzzer` is a value like any
other and may be a top-level declaration of any module, typed by a
signature, so a generator is written once and shared by many tests:

```alder
type Slug = brand Text where Text.matches "^[a-z0-9-]+$"

slugs : Fuzz.Fuzzer Slug
slugs = Fuzz.map Slug (Fuzz.oneOfValues ["a", "b-c", "2024"])

test "a slug is not empty" = Fuzz.forAll slugs (\s -> Text.length s > 0)
```

`Fuzz.map Slug` is allowed because the brand's constructor is: the
module that declares a brand, and any test, may call it. A property over
several inputs takes `Fuzz.pair` or `Fuzz.map2` into a record, or nests
two `forAll`s; there is no `forAll2`. `Fuzz.any : Fuzzer a` is the
generator that the function form derives from the type, as a value, so
the two forms mix: `Fuzz.pair slugs Fuzz.any` is a record of a slug and a derived
value of whatever type the context asks for. Its type must be known where
it is written: inside a function that is itself generic, it has none,
which is E0038.

`Fuzz.forAll` and `Fuzz.forAllTask` make a `Fuzz.Property`, a value that
only a `test` body can be. Anywhere else it is E0039 (below).

### Properties over the database

A property whose body is a block is a property of effects, and, as in
section 3, the block ends in `Task.succeed` of the verdict. Each attempt
gets a database of its own, empty and migrated, with the test's fakes and
session set up afresh, so one attempt never sees another's rows:

```alder
test "a saved title reads back" = \(title : Text.NonEmpty) -> {
  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
  let id <- Db.insert Todo { title: title, due: Nothing, owner: me }
  let after <- Db.get Todo id
  Task.succeed (Maybe.map (\t -> t.title) after == Just title)
}
```

The same holds for each attempt the shrinker makes, which is why an
effect property is slower than a pure one: each attempt builds a database
(in memory, section 3). `Fuzz.forAllTask` is the explicit form, with a
function that returns a `Task Bool`.

### Reading a failure

Here is `rotate`, in a module `Lists`, run:

```text
$ alder test
ok   Lists "rotate keeps the length"
FAIL Lists "rotate twice"  (failed on run 1 of 100, shrunk in 146 steps)
  xs: [0, 0, 1]  (xs : List Int)
  left:  [1, 0, 0]
  right: [0, 0, 1]
  seed: 953983626
  to keep this case:
    test "rotate twice (case)" = rotate (rotate [0, 0, 1]) == [0, 0, 1]
1 passed, 1 failed
```

Line by line:

- The header says which run failed first (`run 1` of the `100`) and how
  many attempts the shrinker spent getting from that case to this one.
  The first failing list was longer and noisier; this one is the
  smallest the shrinker could find.
- `xs:` is the shrunk argument, one line for each, named as in the
  lambda and written as an Alder literal that you could paste into code:
  brands are spelled out (`Text.NonEmpty "a"`), a `Time` is
  `Time.fromMillis 0`. When the type was a defaulted one, the line ends
  with the type the checker chose, `(xs : List Int)`, so a missing
  annotation shows. An annotated argument has no such suffix.
- `left:` and `right:` are the sides of the body's `==`, as in section 2
  (a body that is not an `==` has none).
- `seed:` is what to give `--seed` to draw the same cases.
- `to keep this case:` is the failing case as a **plain test**.

### Keeping the case

The pasted test is the property's body with the lambda removed and each
parameter replaced by its literal. Copy it into the module. It fails
today, and once `rotate` is fixed it passes for good, without random
inputs and without the saved failure below: a bug found once stays
found. For a `Fuzz.forAll` the same is done with `forAll` removed, and
for a block the substitution is made inside the block:

```text
FAIL Notes "titles stay short"  (failed on run 2 of 100, shrunk in 73 steps)
  title: Text.NonEmpty "aaa"
  trace: ["Db.insert Note", "Db.get Note"]
  seed: 9744707179
  to keep this case:
    test "titles stay short (case)" = {
      let id <- Db.insert Note { title: Text.NonEmpty "aaa" }
      let n <- Db.get Note id
      Task.succeed (match n {
        Just x -> Text.length x.title < 3,
        Nothing -> False,
      })
    }
```

(An effect property also shows the steps the failing case took, as
section 6 does.) Two things to know before you paste.

- A literal may name a module the test's module does not import. The
  test is printed with names as the test's own module would write them,
  but the *value* can reach into types the module never mentions. A
  property over `Box.Box`, whose `id` is an `Ids.Id`, prints
  `{ id: Ids.Id "", size: 10 }`, and a module that imports `Box` and not
  `Ids` needs an `import Ids` (chapter 3) before the pasted test
  compiles. Add the import.
- Sometimes there is no test to paste, and the report says `paste the
  values above into a plain test`: when the case is longer than 20 lines,
  when a value has no Alder literal (a `Text` with non-printing
  characters, or the value of a `Fuzz.forAll` generator whose type has no
  printer), or when a parameter is used inside text interpolation or
  markup (`"n is {Text.fromInt n}"`), where a literal cannot be put in
  without changing what it means. The argument lines are still printed,
  and you write the test from them:

```text
FAIL Notes "interpolated"  (failed on run 1 of 100, shrunk in 79 steps)
  n: 100
  seed: 1118386523
  paste the values above into a plain test
```

### `--fuzz`, `--seed` and saved failures

```text
alder test --fuzz 1000      # 1000 cases for each property (the default is 100)
alder test --seed 5         # draw the cases from seed 5
```

`--fuzz N` is the number of cases for each property. `--seed S` fixes the
random choices, so the run is the same every time; without it each run
draws a new seed, and prints it when a property fails. With the same
seed and the same code, the same cases are drawn, so a failure of CI is
reproduced on your machine with the seed of its report.

Finding a bug and fixing it take two different runs, and the next fresh
run may not draw the same case. So a failing case is **saved**: `alder
test` keeps the shrunk choices in `build/<app>/fuzz/<Module>/<hash>`,
the hash being that of the test's name, and replays them before it draws
anything new. The saved case fails again until the bug is fixed; the
first run in which the property passes all its cases deletes the file.
`--seed` does not override a saved failure: the saved case is replayed
first whatever the seed. When it is the saved case that fails, the report
says so (`failed on the saved case`) and gives no seed, since the seed did
not find it; the case is in the report and in the file. `build/` is not source (`.gitignore` keeps it
out of git, chapter 2), so this is a local memory and not a record: the
permanent record of a bug is the plain test that the report printed. A
saved case is also lost if its test is renamed, which is another reason
not to commit it.

### Health checks

A property can pass for a bad reason, or hang, so each property is
checked as it runs, and the check fails the property with a reason
instead:

- **A filter or brand that is too tight.** More than 90% of the first 50
  attempts rejected by a `Fuzz.filter` or a brand's predicate (above).
- **An attempt that is too slow.** One case took over 5 seconds. The
  report says `stopped on run K` and keeps the case like a failure, so the
  next run tries it first; the shrinker gives up on a case that is slow
  as well.
- **A generator that draws too much.** One case needed more than 8192
  random choices (a `Fuzz.andThen` that builds lists of lists of lists
  with no bound): the report says `stopped before run K`, because that
  run never got its values, and `generator draws too much`. Bound the size
  with `Fuzz.listOf { min: 0, max: 10 }`.

A program that crashes outright (a function that recurses without end
overflows the stack) takes `alder test` down with it, as in section 2.
The report then gives what reproduces the crash instead of the arguments,
which the crashed program could not print: the seed and the run, or, when
the crash was in the replay of a saved case, a line that says so.

### How the shrinking works, and why you do not write it

The case `[0, 0, 1]` was not the first one found. The first failing list
had more elements and bigger numbers, and the report shows what 146
re-runs made of it.

Every generator builds its value out of **choices**: numbers drawn from a
random stream, each recorded in order. A list draws, for each element,
one choice that says "another element" or "stop", and then the element's
own choices; an `Int` draws a magnitude and then a sign; a union draws a
constructor number. All generators are written so that a **smaller choice
means a simpler value**: zero is the empty list, the first constructor,
the number 0.

The shrinker never sees a list. It sees the recorded sequence of choices
of the failing case, and tries smaller sequences: delete a block of
choices, set a block to zero, lower a choice by binary search, sort
neighbours, move a value from one choice onto the next. Each candidate is
*replayed*: the same generator runs, reading its choices from the
candidate instead of the random stream, so what it builds is always a
value the generator could have drawn, and the property runs on it. A
candidate that still fails and is smaller replaces the sequence. The
shrinker goes on until a full round changes nothing or it has spent its
**1000 attempts**, so it cannot hang. That is how a bug in a record of a
union of lists shrinks to a short list in the first constructor, without
anyone saying how to shrink a record.

This is the design of Hypothesis, of its small model `minithesis`, and of
elm-test 2.0 (the test library of Elm, the language Alder's syntax is
closest to). It is *internal* shrinking, as against QuickCheck's, where
each type has a separate `shrink` function that the programmer writes
beside its generator. The two must agree, and nothing makes them: a
shrinker that produces a value its generator never could, such as an
`Age` of 200, reports a counterexample that cannot happen. With choices
there is no second function to get wrong. That is why Alder has no way to
write a shrinker, and no need.

### Properties and `--mutate`

Under `--mutate` a property counts as one test, with a fixed seed and no
shrinking or saved failures; section 11 has the details.

### Two errors

A property needs a generator for every parameter. When a type has none,
the checker says so at the parameter:

```alder error=E0038 "`Int -> Int` cannot be generated; use `Fuzz.forAll` with a generator you write"
test "a function is the same twice" = \(f : Int -> Int) -> f 1 == f 1
```

A function type, an effect type (`Task`, `Query`), a type parameter and a
value only the runtime can make have no generator, and neither has a
union whose every constructor holds the union again (`type S = S { next:
S }`: nothing ends the recursion; let one constructor hold a `Maybe` or a
`List` of it, or nothing). Annotate with a type
that has one, or write the generator with the `Fuzz` functions and give
it to `Fuzz.forAll`.

A `Fuzz.Property` is what a test body is, and nothing else may be one:

```alder error=E0039 "`Fuzz.forAll` makes a Property, which only a test body can use"
pending : Fuzz.Property
pending = Fuzz.forAll Fuzz.int (\n -> n == n)
```

Move the `Fuzz.forAll` into a `test`, and keep the generator in a value.

## 13. What cannot be tested yet

The browser of section 9 runs the app's real routes, so a request's round
trip, the page a step leaves, and the session after a step are all
tested. What it does not run is a browser's own machinery:

- **CSS, layout and JavaScript.** The `Doc` is the page's HTML. Whether
  an element is visible, where it is, how it looks, and what the
  JavaScript of a web component does with it are not modelled. A small
  suite in a real browser, planned for later, will cover them.
- **The page without JavaScript.** The browser is Datastar's client, as
  every visitor's browser is when its JavaScript runs. With JavaScript
  off, a form is a plain `POST` answered by a `303` redirect (chapter 6);
  that path is not exercised.

And one thing will not change: effects are tested by what they do, not
by `==`. A `Query` or `Task` cannot be compared; `==` or `!=` on one is a
type error:

```alder error=E0003 "effect values cannot be compared"
Query.succeed 1 == Query.succeed 1
```

An effect holds functions (each `let x <-` continues with a function of
`x`; chapter 7, "How effects run"), and two functions cannot be compared:
there is no way to tell whether they would always do the same thing. So
a test runs the effect, against a fresh database, and compares what it
did (its trace, its value) and what it left behind (the rows it reads
back).
