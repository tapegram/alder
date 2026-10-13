# 10. Testing

A test is a piece of the program that checks another piece and says yes or
no. In Alder a test is a declaration, written in the module it tests, after
the code, and `alder test` runs every test in the app. This chapter starts
with the simplest kind, a test of a pure function, then reads the report
`alder test` prints, then tests code that reads and writes the database,
replaces a capability with a fake written in Alder, drives the app
through an in-memory browser as a user would, measures the tests with
mutants (`alder test --mutate`), and ends with what cannot be tested
yet.

## 1. A test is a `Bool`

`test "name" = expression` declares a test. When its body is not a block,
the body must be a `Bool`: `True` means the test passed. The to-do app's
`Todo` module (`examples/todos/app/Todo.alder`) ends with one, which
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

Because `load`, `view` and `update` are functions that return
descriptions, much of a page can be tested this way. `view` returns HTML,
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
and prints one line per test. In the repository:

```text
$ alder test examples/todos
ok   App "Logout clears the user and goes to log in"
ok   Auth.Login "an unknown email is rejected"
ok   Auth.Login "logging in through the form"
ok   Auth.Login "a visitor is sent to log in, and can log out"
ok   Todo "countOpen ignores done todos"
ok   Todo "Active keeps only open todos"
ok   Todo "each filter has its label"
ok   Todo "a user with todos cannot be deleted"
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
20 passed, 0 failed
```

Each line names the module and the test, and is printed as soon as that
test finishes, so a slow test shows which tests ran before it. Tests run
in module order (a module after the modules it imports), and within a
module in the order they are written. The last line counts them.
`alder test` exits with 0 when every test passed and 1 when one failed,
so a script or CI job can use it directly. Two tests of one module may
not have the same name (E0009).

Here is the same app with two wrong tests added: one at the end of
`Todo`,

```alder
test "an empty list has one open todo" = Todo.countOpen [] == 1
```

and one at the end of `Todos.Index` that expects `Toggle` to read the
todo and then write it, in two steps (sections 3 to 6 explain effect
tests and traces):

```alder
test "Toggle reads, then writes" = {
  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
  let id <- Db.insert Todo { title: Text.NonEmpty "Milk", due: Nothing, owner: me }
  let _ <- Test.login { user: Just me }
  let r <- Test.run (Todos.Index.update (Toggle { id: id }) { filter: All, todos: [] } { user: me })
  Task.succeed (r.trace == ["Db.get Todo", "Db.update Todo"])
}
```

In their modules they are written without the `Todo.` and `Todos.Index.`
prefixes. Both compile; both are wrong:

```text
$ alder test examples/todos
ok   App "Logout clears the user and goes to log in"
ok   Auth.Login "an unknown email is rejected"
ok   Auth.Login "logging in through the form"
ok   Auth.Login "a visitor is sent to log in, and can log out"
ok   Todo "countOpen ignores done todos"
ok   Todo "Active keeps only open todos"
ok   Todo "each filter has its label"
ok   Todo "a user with todos cannot be deleted"
FAIL Todo "an empty list has one open todo"
  left:  0
  right: 1
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
20 passed, 2 failed
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

A test that runs for ten seconds is announced on standard error,
`still running Todo "slow" (10s)`, and again every ten seconds, so a
test that hangs is not mistaken for a quiet one.

If the app does not compile, no test runs: `alder test` prints the
diagnostics as `alder check` does and exits 1. A typed hole (`?todo`,
chapter 11) compiles to nothing that can run either: its warning W0002 is
followed by `alder test: no tests ran (fill the hole)`, exit 1. Here the app has gained a
module `Probe` that uses `Test.trace` outside a test (section 5) and
declares `test "one" = { Task.succeed 1 }`, a block whose verdict is not
a `Bool`:

```text
$ alder test examples/todos
app/Probe.alder:4:9: error[E0003]: `Test.trace` is only available inside a `test`
  steps = Test.trace
          ^^^^^^^^^^
app/Probe.alder:6:29: error[E0003]: expected Bool, got Int
  test "one" = { Task.succeed 1 }
                              ^
```

## 3. Testing effects

A `Task` cannot be compared with `==` (section 12 says why), so a test of
`update` cannot just compare its result. Instead it runs the task and
looks at what happened. A test whose body is a block, `{ … }`, is an
**effect test**: the block is a `Task Bool`, with `let x <-` lines like
any `Task` (chapter 7), and it ends with `Task.succeed` of the verdict. A
block that ends in a plain `Bool` is an error; the compiler's hint says
to end it with `Task.succeed (…)`.

The to-do page's tests are the last thing in `app/Todos/Index.alder`
(chapter 2 shows them in place). Written in another module, as the
blocks of this chapter are, the page's names are qualified:

```alder
test "Toggle flips done" = {
  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
  let id <- Db.insert Todo { title: Text.NonEmpty "Milk", due: Nothing, owner: me }
  let _ <- Test.login { user: Just me }
  let r <- Test.run (Todos.Index.update (Toggle { id: id }) { filter: All, todos: [] } { user: me })
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
4. Run `update` under `Test.run` (section 4) and call the outcome `r`.
   `update` takes the message (`Toggle` for that todo), the page's model
   (`load` is not called; `update` only needs a value of the type), and
   the session. `Todos.Index` `requires user`, so its `Session` is a type
   of its own whose `user` is a `UserId`, not a `Maybe` (chapter 6). The
   record literal `{ user: me }` is that type because it is
   written where `update` expects one. Bound to a name first by a `let`
   without an annotation, it would be a plain record, which `update`
   does not accept. This record is `update`'s `session` argument, which
   the page's own code may read; the scoped operations do not read it
   (step 3, and "Tests that use the rules", below).
5. Read the todo again, and take its `done` (`after` is a `Maybe Todo`:
   `Nothing` if the row were gone).
6. The verdict, as one `==` between two records: `update` made one
   step, the scoped write, and the stored todo is now done. Putting everything
   the test checks into one record, rather than joining several `==`
   with `&&`, means a failure prints both records whole (section 2).

The second test is the ownership rule of chapter 6, "Rules": a user who
posts `Toggle` with someone else's todo id gets a 404 and changes
nothing. The example tests it through the browser (section 9, "Who may
toggle a todo"); here it is through `update` itself, the stranger signed
in with `Test.login`:

```alder
test "Toggle by a stranger is not found" = {
  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
  let id <- Db.insert Todo { title: Text.NonEmpty "Milk", due: Nothing, owner: me }
  let stranger = User.UserId "someone-else"
  let _ <- Test.login { user: Just stranger }
  let r <- Test.run (Todos.Index.update (Toggle { id: id }) { filter: All, todos: [] } { user: stranger })
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
  left:  { value: Ok Rerender, trace: ["Db.update Todo"], done: Just True }
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
- **A login through the login page's form** (`Test.submit` of its
  `Credentials`) signs the user into the **browser** only: the page's
  `Session.put` ran in the request, and the router stored the result in
  the cookie. The browser's next requests act for that user; the test's
  own steps (a `Test.run (update …)`, a `Db.mine` in the test body) do
  not.

So a test that calls `update` or `load` itself, with a session record,
signs in with `Test.login` first; the record is still `update`'s
argument, and must have the page's `Session` type, but it does not choose
the user. Without a signed-in user a scoped step fails, and says why.
Here is "Toggle flips done" without its `Test.login` line, expecting
`Ok Rerender`:

```text
$ alder test examples/todos -run 'without a login'
FAIL Todos.Index "Toggle without a login"
  left:  { value: Err "Db.updateMine Todo: no signed-in user", trace: ["Db.updateMine Todo"] }
  right: { value: Ok Rerender, trace: ["Db.updateMine Todo"] }
  trace: ["Db.insert User", "Db.insert Todo", "Test.run", "Db.updateMine Todo"]
0 passed, 1 failed
```

It never answers as if the user had no rows.

A test may also use the scoped operations itself. The to-do page's last
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
  test are ordered, and a test gets the same times on every run. Time is
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
  session record as `update`'s argument, which the page's own code
  reads.
- **A browser.** A page held and a session cookie, both empty at the
  start, for the steps of section 9.

A pure test (section 1) touches none of this: it is an expression,
evaluated once.

## 4. `Test.run`: running a task that may fail

`update` for another user's todo ends in `Task.notFound`. If the test
simply wrote `let nav <- Todos.Index.update …`, that failure would be the
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
  error, the call and the error: `"Password.verify: connection refused"`. `update`'s `value` is a `Result Text Nav`, and
  `Nav` values compare with `==`, so `r.value == Ok Rerender` works too.
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

Like the page's tests, this one ends with one `==` between two records.

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

`Test.run` and `Test.trace` exist only inside a `test`. Anywhere else
they are an error, so no page can depend on them:

```alder error=E0003 "is only available inside a `test`"
steps : Task (List Text)
steps = Test.trace
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
  `"Test.get Todos.Index"`, `"Test.submit NewTodo"`,
  `"Test.click Todos.Index.Toggle"`, followed by the steps of the
  requests it made (section 9).

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
{"module":"Todo","name":"countOpen ignores done todos","ok":true,"left":"2","right":"2","trace":[],"ms":0.01}
1 passed, 0 failed
```

Each object has `module`, `name`, `ok`, `trace` (empty for a pure test)
and `ms`, the time it took in milliseconds (it differs from run to run);
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
  exempt from E0018 (a table in its own module).
- Only a `test`, in any module, and the fake's own entries may use it,
  its type or its columns. Anywhere else (a value, a signature, a page's
  `load`, an app table's `@ref`) it is E0021: a value
  `charges = Db.from Payments.Charge |> Db.count` in `Checkout` gets
  "`Charge` is a fake's table: only tests and `fake Stripe` may use it".

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

- `Test.fake Stripe` is a `Task Unit`, available only inside a `test`
  like the rest of `Test` (E0003 "`Test.fake` is only available inside
  a `test`"). Its argument is a capability's name, `Stripe` or
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
what the session then holds, through a browser that runs in memory.

### Why through the browser

The tests so far call `load` and `update` directly. That leaves out what
happens between them in the running app: the route that reads `/todos/x`
into a page value, the decoding of a submitted form, `requires` sending a
visitor to the login page, the session a login stores, the page a
`Redirect` leads to, the buttons a page actually shows. Most of the bugs a
server-rendered app ships live at those seams, and they show only across
one request and the next.

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

Six steps act through it. Each is a `Task` (a step of the test's block,
written with `let x <-`), and each works only inside a `test`:

| Step | What it does | It gives |
|---|---|---|
| `Test.login s` | puts the session `s` in the cookie, as a login would | `Unit` |
| `Test.get p` | requests the page `p`; the test fails unless the page is shown | `{ model: Model, session: Session, doc: Doc }` |
| `Test.open p` | requests the page `p`, whatever the answer | `{ answer: Answer, session: Session, doc: Doc }` |
| `Test.submit F r` | posts the record `r` through the page's form `F` | `{ answer: Answer, session: Session, doc: Doc }` |
| `Test.submitRaw F fields` | posts text fields through the page's form `F` | `{ answer: Answer, session: Session, doc: Doc }` |
| `Test.click m` | presses the page's button that sends the message `m` | `{ answer: Answer, session: Session, doc: Doc }` |

In the results, `model` is the page's `Model`, exactly the one its
`view` was given, and typed (`page.model.todos` is a `List Todo`);
`answer` says what the request came to (`Answer`, below); `session` is
`App`'s session after the step, what the cookie now carries
(`r.session.user == Just me`); and `doc` is the page the browser now
holds, a `Doc`: a parsed HTML document, read with the queries below and
compared with `==`.

### A first browser test

The to-do page's tests end with three through the browser (chapter 2
shows them in place). The first adds a todo by its form and reads the
summary line the page shows:

```alder
test "adding a todo through the page" = {
  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
  let _ <- Test.login { user: Just me }
  let _ <- Test.get (Todos.Index {})
  let added <- Test.submit Todos.Index.NewTodo { title: Text.NonEmpty "Milk", due: Nothing }
  let page <- Test.get (Todos.Index {})
  Task.succeed ({ answer: added.answer, todos: List.map (\t -> t.title) page.model.todos, summary: Test.text "#summary" page.doc } == { answer: Redirected (Todos.Index {}), todos: [Text.NonEmpty "Milk"], summary: Just "1 open" })
}
```

In `app/Todos/Index.alder` the form is written `NewTodo`; from another
module, as here, it is `Todos.Index.NewTodo` (no import is needed for it).

### Step by step

Line by line:

1. Insert a user, as any effect test does.
2. `Test.login { user: Just me }` puts that session in the browser's
   cookie. The record is `App`'s session, so its `user` is a
   `Maybe UserId`: `Just me`, not `me`. Nothing is requested.
3. `Test.get (Todos.Index {})` requests `/` with the cookie. The page
   `requires user`, and the session has one, so `load` runs, `view`
   renders, and the browser now holds the page. Its result is not needed
   here (`let _ <-`).
4. `Test.submit Todos.Index.NewTodo { … }` fills the page's `NewTodo`
   form with the record and posts it. The page must show that form: the
   browser posts only what a user could. The page's `update` inserts the
   todo and answers `Redirect (Todos.Index { filter: model.filter })`; the
   browser follows the redirect, as a real one does, and now holds the
   new page. `added.answer` is `Redirected (Todos.Index {})`.
5. `Test.get` again, for the page's model: `page.model.todos` is the list
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
  trace: ["Db.insert User", "Test.login", "Test.get Todos.Index", "Db.mine Todo", "Db.all Todo", "Test.submit NewTodo", "Db.mine Todo", "Db.all Todo", "Db.insertMine Todo", "Db.mine Todo", "Db.all Todo", "Test.get Todos.Index", "Db.mine Todo", "Db.all Todo"]
0 passed, 1 failed
```

A page value prints as its address: `Redirected /` is
`Redirected (Todos.Index {})`, whose `filter` is the default and so not
in the URL. Two page values are equal when their addresses are. The
trace is each step's name, then the effects of the requests it made
(section 10 explains why `Db.all Todo` comes before the insert; each
`Db.mine Todo` is the note `load`'s scoped read leaves before its
`Db.all`, section 3).

### `Test.get` and `Test.open`: requesting a page

`Test.get p` and `Test.open p` take a page value, written in place:
`Todos.Index {}`, `Todos.Index { filter: Active }`,
`Todos.Show { id: id }`. A variable holding a page is not accepted.
Each sends a plain `GET` of the page's address with the cookie.

`Test.get` is for a page the test expects to see. When the page is not
shown, the test fails and says why, with the address requested. Here,
from a test added to `Todos.Index` that gets the list without logging
in:

```text
FAIL Todos.Index "a visitor's list"
  error: GET /: redirected to /login (requires user)
  trace: ["Test.get Todos.Index"]
```

The other two reasons are a 404 (here `Todos.Show`'s `load` found no
todo `x` and said `Query.notFound`) and a failed request, a 500. Because
it fails otherwise, `Test.get` can give the page's `model`, typed: the
`Model` of the page module requested.

`Test.open` is the same request, but it never fails the test on the
answer: its result's `answer` says what happened, and the test checks it.
This is how a test checks a redirect or a 404:

```alder
test "a visitor is sent to the login page" = {
  let r <- Test.open (Todos.Index {})
  Task.succeed ({ answer: r.answer, heading: Test.text "h1" r.doc, user: r.session.user } == { answer: Redirected (Auth.Login {}), heading: Just "Log in", user: Nothing })
}
```

After `Redirected p` the browser already holds `p`, so the test goes on
from there without asking for it. (It could not write `Test.open p` with
the `p` it matched anyway: a step takes a page value written in place.)

### `Answer`: what a request came to

`Answer` is a union of the Prelude, which only tests can use:

```text
type Answer = Shown
            | Rerendered
            | Redirected Page
            | FormRejected (List { field: Text, message: Text })
            | PageMissing
            | RequestFailed Text
```

| Answer | When | The page the browser then holds |
|---|---|---|
| `Shown` | `get`/`open`: the page was shown | that page |
| `Rerendered` | `submit`/`click`: `update` answered `Rerender`; or `Reject` when the re-rendered page shows no errors (its form has no `<Errors form />`, or the page no longer draws that form) | the page re-rendered, with the new data |
| `Redirected p` | a `Redirect p`, or a `requires` sending a visitor to the login page | `p`, which the browser requested in turn |
| `FormRejected errors` | `submit`: the form did not decode, or `update` answered `Reject` and the re-rendered form shows the text in its `<Errors form />` | the page with the form re-rendered: the values typed and the errors |
| `PageMissing` | a 404: `Query.notFound`, `Task.notFound`, no such page | `App.notFound` |
| `RequestFailed text` | the request failed (a `Task.fail`, a Go panic): a 500 | `App.error` |

A redirect to another site (`Url.external`, chapter 6) is reported,
`Redirected` with that address, but not followed: the browser keeps the
page it had. A redirect to a page that is then a 404 is `PageMissing`.
`RequestFailed`'s text is the failure's message: `"nope"` for a
`Task.fail "nope"`, `"panic: …"` for a Go panic.

The 404 answer is `PageMissing`, not `NotFound`, so that an app keeps the
name `NotFound` for a constructor of its own. The six names are kept for
`Answer` everywhere: a union or `msg` of the app may not use one
(E0012), nor may a type be named `Doc` or `Answer`.

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
test about logging in itself goes through the login page with its real
form. That page calls the `Password` capability, whose adapter is Go; a
test replaces it with the fake `app/Auth/Login.alder` declares beside it
(section 8), which accepts any user who has a row, whatever the
password:

```alder
test "logging in through the form" = {
  let _ <- Test.fake Password
  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
  let _ <- Test.get (Auth.Login {})
  let r <- Test.submit Auth.Login.Credentials { email: Text.Email "me@example.com", password: Text.Secret "secret" }
  Task.succeed ({ answer: r.answer, user: r.session.user, title: Test.text "h1" r.doc } == { answer: Redirected (Todos.Index {}), user: Just me, title: Just "Todos" })
}
```

The page's `update` called the fake and `Session.put`; the browser kept
the cookie, followed the redirect with it, and holds the list. (That
login is the browser's: the test's own session is still the one it
started with, so a `Db.mine` in the test body would have no user.) With an
email that has no row the fake answers `Nothing`, `update` answers
`Reject "Email or password is incorrect."`, and the answer is
`FormRejected [{ field: "", message: "Email or password is incorrect." }]`.
`Test.click App.Logout` presses the list's "Log out" button: the answer
is `Redirected (Auth.Login {})` and `session.user` is `Nothing`.

### `Test.submit`: posting a form

`Test.submit F r` posts the form `F` with the record `r`. `F` is a form a
page or `App` declares: `NewTodo` in its own module, `Todos.Index.NewTodo`
from any other. `r` is checked as the form's record (a field with a
default may be left out). The browser sends it as the page's inputs
would:

| The field's type | Sent as |
|---|---|
| `Text`, `Text.NonEmpty`, `Text.Email`, `Text.Url`, `Text.Secret`, a brand of `Text` | the text |
| `Int`, a brand of `Int` | its digits: `42`, `-3` |
| `Float` | the shortest decimal that reads back the same: `1.5`, `1e-07` |
| `Bool` | `True` when `True` (a ticked checkbox); not sent when `False` (an unticked one) |
| `Date` | `2026-03-01` |
| a union without payloads | the constructor's name: `Active` |
| `Maybe a` | blank for `Nothing` (an empty input); `a`'s text for `Just` |

Decoding it gives back `r`, except that a `Maybe Text` that is
`Just ""` is sent blank and decodes as `Nothing`: neither a test nor a
user can send `Just ""`.

The form must be on the page the browser holds. The browser posts it to
the address it posts to, with the page's parameters (a page opened as
`Todos.Index { filter: Active }` posts with its filter), and sends only
the fields the form has an input for (an `<Input>`, `<Select>` or
`<Textarea>` with the field's name), as a real browser does. A field
without one fails the step, "the page's form NewTodo has no input
`due`", unless its value is what the page makes of a field that is not
sent: `Nothing` for a `Maybe` without a default, the default for a field
that has one, `False` for a `Bool`; those are left out, and the page
decodes the same record.

A `Date` in a test record: `Date.fromText "2026-03-01"` is a
`Maybe Date`, the type of a `due: Maybe Date` field. For a `Date` field,
take one from the test's clock: `let today <- Date.today` gives
`2026-01-01`, `Date.addDays 3 today` a later day.

**A form each row renders** (`<form submit={Rename { id: n.id }}>` in a
loop), or one that two messages post: the test writes the message with
its parameters instead of the form's name,
`Test.submit (Rename { id: b }) { title: Text.NonEmpty "B2" }`, which
posts the form of row `b`.

### More forms

Here is a todo with a due date, sent through the form:

```alder
test "a todo with a due date" = {
  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
  let _ <- Test.login { user: Just me }
  let _ <- Test.get (Todos.Index {})
  let r <- Test.submit Todos.Index.NewTodo { title: Text.NonEmpty "Milk", due: Date.fromText "2026-03-01" }
  Task.succeed ({ answer: r.answer, due: Test.text "span.due" r.doc } == { answer: Redirected (Todos.Index {}), due: Just "Mar 1" })
}
```

When two messages post the same form, the form's name cannot say which message is
meant, so the compiler asks for the message:

```alder error=E0003 "`Title` is posted by `Save` and `Draft`: write `Test.submit (Save { id: … }) { … }`"
module Drafts

page "/drafts" { }

type Model = { ids: List Text }

load : Params -> Session -> Query Model
load params session = Query.succeed { ids: ["a", "b"] }

form Title = {
  title: Text,
}

msg Save { id: Text } form Title
msg Draft { id: Text } form Title

update : Msg -> Model -> Session -> Task Nav
update msg model session = Task.succeed Rerender

view : Model -> Session -> Html Msg
view model session =
  <html>
    <body>
      <ul>
        {for id in model.ids ->
          <li>
            <form submit={Save { id: id }}><Errors form /><Input field="title" label="Title" /><button>Save</button></form>
            <form submit={Draft { id: id }}><Errors form /><Input field="title" label="Title" /><button>Keep as draft</button></form>
          </li>}
      </ul>
    </body>
  </html>

test "renaming one draft" = {
  let _ <- Test.get (Drafts {})
  let r <- Test.submit Title { title: "x" }
  Task.succeed (r.answer == Rerendered)
}
```

Written `Test.submit (Save { id: "b" }) { title: "x" }`, the test
compiles, and posts row `b`'s Save form. A test that opens the login
page and then submits `NewTodo` gets:

```text
FAIL Todos.Index "posting a form the page lacks"
  error: the page /login has no form NewTodo (Todos.Index.Submit)
  trace: ["Db.insert User", "Test.login", "Test.get Auth.Login", "Test.submit NewTodo"]
```

### `Test.submitRaw`: input the types forbid

`Test.submit`'s record can only hold valid values: a `Text.NonEmpty` is
never empty, a `Date` is always a date. A user's browser has no such
rule, and the page must answer bad input with the form and its errors.
`Test.submitRaw F fields` posts text, field by field, as typed:
`Test.submitRaw NewTodo [{ name: "title", value: "" }]`. A field not
sent is a field left out of the form. Every name sent must be a field of
the form and have an input on the page.

**`FormRejected errors`** is the answer when the page shows its form again
with errors: the form did not decode (chapter 6: `update` never ran), or
`update` answered `Reject "…"`. The browser holds the page re-rendered,
with what was typed back in each input (except a `Text.Secret`, which is
never sent back) and each error in its `<Errors>` list. `errors` is what
those lists show:

- in the order the lists come in the page (the order of the form's
  `<Errors field="…">`, usually its fields' order);
- `field` is the field's name, and `""` for the form's own list,
  `<Errors form />`, which holds `Reject`'s text;
- a field error the form has no `<Errors field>` for is shown in
  `<Errors form />`, after `Reject`'s text, prefixed with the field's
  name, so it comes back as `{ field: "", message: "title must not be
  empty" }`;
- a form with no `<Errors form />` shows no `Reject` text at all; the
  page looks unchanged, and the answer is `Rerendered`. The compiler
  warns about such a form when its message's arm of `update` can answer
  `Reject` (W0005).
- a form the re-rendered page no longer draws shows no `Reject` text
  either. A page held while an order was Open, the order paid meanwhile,
  and its add-line form posted: `update` answers `Reject`, but the Paid
  order's page has no add-line form, so the text has nowhere to go; the
  answer is `Rerendered`, and a user sees the page change with no
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
  let _ <- Test.get (Todos.Index {})
  let r <- Test.submitRaw Todos.Index.NewTodo [{ name: "title", value: "" }, { name: "due", value: "tomorrow" }]
  let rows <- Db.from Todo |> Db.count
  Task.succeed ({ answer: r.answer, due: Test.attr "value" "input[name=due]" r.doc, rows: rows } == { answer: FormRejected [{ field: "title", message: "must not be empty" }, { field: "due", message: "must be a date (YYYY-MM-DD)" }], due: Just "tomorrow", rows: 0 })
}
```

The title and the date come back as typed (`Test.attr "value"` reads
the input), with their errors, and no row was written.

### `Test.click`: pressing a button

`Test.click m` sends the message `m` as the page's button for it would:
`Test.click (Toggle { id: id })`, `Test.click App.Logout`. `m` is a
message of a page or of `App`, with its parameters, written as in a
`click={…}`. A message that carries a form is posted with `Test.submit`,
not clicked; a link is followed with `Test.get` or `Test.open`.

The page must show a button (or form) that sends exactly that message,
with the same parameters: a test can only do what the user it is logged
in as can see to do. The second to-do test through the page:

```alder
test "toggling a todo through the page" = {
  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
  let id <- Db.insert Todo { title: Text.NonEmpty "Milk", due: Nothing, owner: me }
  let _ <- Test.login { user: Just me }
  let _ <- Test.get (Todos.Index {})
  let r <- Test.click (Toggle { id: id })
  Task.succeed ({ answer: r.answer, done: Test.has "li.done" r.doc, summary: Test.text "#summary" r.doc } == { answer: Rerendered, done: True, summary: Just "0 open" })
}
```

`update` answered `Rerender`, so the browser holds the page rendered
again: the item now has `class="done"`, and nothing is open.

### Clicks that fail

The compiler refuses a click of a message that carries a form:

```alder error=E0003 "`Submit` carries form `NewTodo`: use `Test.submit`"
test "clicking a form's message" = {
  let _ <- Test.get (Todos.Index {})
  let r <- Test.click (Todos.Index.Submit { form: { title: Text.NonEmpty "Milk", due: Nothing } })
  Task.succeed (r.answer == Rerendered)
}
```

### A stranger's todo

**Who may toggle a todo.** Chapter 6's rule, "Rules", says that a user
who posts `Toggle` with someone else's todo id gets a 404. A stranger
who logs in and gets the list has no way to do that through the page (a
test that logs in as the stranger, gets the list, and clicks `Toggle {
id: id }` expecting `PageMissing` does not get that far):

```text
FAIL Todos.Index "a stranger clicks my todo"
  error: the page / has no button or form for Todos.Index.Toggle { id: "vTJh-0JAjgrsF9WiOyhe" }
  trace: ["Db.insert User", "Db.insert Todo", "Test.login", "Test.get Todos.Index", "Db.mine Todo", "Db.all Todo", "Test.click Todos.Index.Toggle"]
```

The stranger's list shows only the stranger's todos, so there is no
Toggle button for mine to press; the browser sends nothing, and the test
fails. (The id is random, and differs on each run. Had the stranger's
list shown todos of their own, the text would end with the ones it
offers: " (the page has Toggle for id: …)".) That is the right answer:
the page does not offer the action.

But a page can be out of date. My list is open in the browser; then
someone else logs in on it (the cookie changes, the page held does not)
and presses my todo's button. The example tests exactly that:

```alder
test "Toggle by another user is not found" = {
  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
  let id <- Db.insert Todo { title: Text.NonEmpty "Milk", due: Nothing, owner: me }
  let _ <- Test.login { user: Just me }
  let _ <- Test.get (Todos.Index {})
  let _ <- Test.login { user: Just (User.UserId "someone-else") }
  let r <- Test.click (Todos.Index.Toggle { id: id })
  let after <- Db.get Todo id
  Task.succeed ({ answer: r.answer, done: Maybe.map (\t -> t.done) after } == { answer: PageMissing, done: Just False })
}
```

The click posts my page's Toggle button with the stranger's cookie. The
request runs `load` for the stranger, then `update`, where
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
  let page <- Test.get (Todos.Index {})
  let show <- Test.open (Todos.Show { id: id })
  Task.succeed ({ items: Test.count "li" page.doc, show: show.answer } == { items: 0, show: PageMissing })
}
```

The stranger's list has no `<li>` (`Db.mine Todo` has none of mine), and
`Todos.Show`'s `load` reads the todo with `Db.getMine`, which answers
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
and text, leaving out the page's parameters the router adds to each
message's address (`?_p.filter=Active`, section 10), which `view` alone
does not know. `Test.doc` renders without a request, so a form in it has
no typed values and no errors: compare a `FormRejected` page by its queries,
not with `Test.doc`.

### A selector the compiler refuses

Here the `x` after a space would be an element inside `#sumary`, but
there is no element `x`:

```alder error=E0023 "`#sumary x` is not a selector: unexpected `x` after `#sumary`; `x` is not an HTML element (a custom element has a hyphen)"
test "a misspelt selector" = {
  let page <- Test.get (Todos.Index {})
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
  let page <- Test.get (Todos.Index { filter: Active })
  Task.succeed ({ same: page.doc == Test.doc (Todos.Index.view page.model { user: me }), links: Test.texts "nav a" page.doc, titles: Test.texts "#todos a" page.doc, items: Test.count "#todos > li" page.doc } == { same: True, links: ["All", "Active", "Completed"], titles: ["Milk"], items: 1 })
}
```

When a test's last `==` compares two `Doc`s that differ, the report does
not print the two documents. It names the first element whose content
differs, its path from the body, and shows that element on each side.
Had the test compared the page with a view of an empty model,
`page.doc == Test.doc (Todos.Index.view { page.model | todos: [] } { user: me })`,
it would print:

```text
FAIL Todos.Index "the page and an empty view"
  left:  <ul id="todos"><li class=""><form method="post" action="/_m/Todos.Index/Toggle?id=ZjFUNWta6zynlqJZb2jc" class="_alder-c…
  right: <ul id="todos"></ul>
  note:  the documents differ at body > main > ul#todos: left 1 child <li>, right 0
  trace: ["Db.insert User", "Db.insert Todo", "Test.login", "Test.get Todos.Index", "Db.mine Todo", "Db.all Todo"]
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
  page renders no form for that message.
- "the page / has no button or form for Todos.Index.Toggle { id: "x" }",
  followed by " (the page has Toggle for id: a, b)" when the page
  renders the message with other parameters.
- "the page's form NewTodo has no input `due`": a field the form renders
  no input for (`Test.submit`, `Test.submitRaw`).
- "the page has 2 forms for Notes.Index.Rename: write the message with
  its parameters".
- "no page is open: `Test.get` one first".
- "form NewTodo has no field `colour`": a `Test.submitRaw` name the form
  does not declare.

And when the test compiles (E0003 unless named):

- "`Test.get` is only available inside a `test`"; "`Test.get` needs an
  `App` module: the browser requests the app's routes, which an app
  without `App` does not serve".
- "`Test.submit` takes its arguments directly, not through `|>`: …";
  E0008 "`Test.submit` takes 2 arguments, got 1: …".
- "`Test.get` wants a page value such as `Todos.Index {}`, got `p`";
  "`Test.get` needs a record `Model`: … use `Test.open`".
- "`Test.login` wants the app's session `{ user: Maybe User.UserId }`:
  expected Maybe User.UserId, got User.UserId".
- E0002 "`NewTodo` is a form of `Todos.Index`: write
  `Todos.Index.NewTodo`"; "`Test.submit` wants a form declared by a
  page: `Contact` is a form of `Shared`, which is not a page";
  "`Test.submit` wants a form a message posts: no `msg … form Orphan` in
  `Todos.Index`"; "`Title` is posted by `Save` and `Draft`: write
  `Test.submit (Save { id: … }) { … }`"; "`Toggle` posts no form: use
  `Test.click`".
- "`Submit` carries form `NewTodo`: use `Test.submit`"; "`Test.click`
  wants a message of a page or of `App`, got `Todos.Index {}` of type
  `Page`: follow a link with `Test.get` or `Test.open`".
- E0002 "unknown constructor `NotFound`; the browser's 404 answer is
  `PageMissing`"; E0012 "`PageMissing` is a constructor of the Prelude's test
  type `Answer`; …"; "`Doc` is a test type: it exists only inside a
  `test`".
- E0023 "`#sumary x` is not a selector: unexpected `x` after `#sumary`;
  `x` is not an HTML element (a custom element has a hyphen)"; its other
  hints are "tags are lower case: `h1`" and "quote the value:
  `[href="/x"]`".
- W0005 "`Submit` can answer `Reject`, but this form has no
  `<Errors form />`: the user never sees the rejection text".

### The rules

- The steps exist only inside a `test`, each applied directly to all its
  arguments (not piped into, not passed as a value); all but
  `Test.login` need an `App` module.
- **Only what the page shows.** `Test.submit` and `Test.click` act on a
  form or button the held page renders, posting it exactly as rendered
  (the page's parameters included), and a submit sends only the fields
  the form has inputs for.
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
  `Test.get Todos.Index`, `Test.open Todos.Show`,
  `Test.submit NewTodo`, `Test.submitRaw NewTodo`,
  `Test.click Todos.Index.Toggle` or `Test.click App.Logout`, and the
  effects of the requests it made follow it.
- **What it does not test.** No CSS, no layout, no JavaScript: whether a
  button is visible, whether a web component works, how the page looks.
  The browser is Datastar's client (section 10), so the path of a
  browser with JavaScript off, a plain form post answered by a
  redirect, is not tested either (section 12).

## 10. How the browser works

Section 9 can be used without this one. This section explains what
happens when a step runs, so that a surprising trace or answer can be
read.

**The real routes, in memory.** The test program is built with the
app's routes, the same list the app's binary serves, and the same router
(`rt/web`). A step does not open a network connection: it builds an HTTP
request in memory and hands it to the router, which calls the route's
handler and writes the response into memory, where the browser reads it.
Everything between, from reading `/todos/x` into a page value to
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
  │ Test.submit                            POST /_m/Todos.Index/Submit
  │   Db.mine Todo, Db.all Todo            load, for update's model
  │   SAVEPOINT alder_t1                   update is a Task
  │   │ Db.insertMine Todo
  │   └ RELEASE alder_t1                   it succeeded: the insert is the test's now
  │   Db.mine Todo, Db.all Todo            GET /: the redirect, followed
  │ Test.get                               GET /
  │   Db.mine Todo, Db.all Todo
  COMMIT                                   the verdict
```

When an `update` fails, its savepoint is rolled back (`ROLLBACK TO`),
so its writes are gone and the test's are not. This is also why the
trace shows `Db.all Todo` before `Db.insertMine Todo`: a message's request
runs the page's `load` first, to give `update` its model (chapter 6,
"How a request runs").

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
  `Rerender` and `Reject` come back this way. The browser then looks at
  the page: an `<Errors>` list with items makes it `FormRejected`; a 404 or
  a 500, which come back as a patch too (with the status in a header,
  `Alder-Status`), are `PageMissing` and `RequestFailed`; anything else is
  `Rerendered`. A page whose `view` has no `<body>` (every page built
  on a layout such as `Layout.Page` has one) cannot be patched, and the
  step fails "the held page has no <body> to patch".
- an instruction to go to another address is a `Redirect`: the browser
  requests that page with a plain `GET`, as Datastar has the real
  browser do, and the answer is `Redirected`. An address on another site
  is not requested.

`Test.get` and `Test.open` are plain `GET`s, like typing an address. A
`303` answer, such as `requires` sending a visitor to `/login`, is
followed (at most ten in a row).

**Controls.** The page's buttons and forms post to message addresses,
`/_m/<Module>/<Msg>` with the message's parameters and the page's own
(`/_m/Todos.Index/Toggle?id=…&_p.filter=Active`: chapter 6). Before it
posts, the browser looks through the held page for a form or button
whose address names the same message with the same parameters, ignoring
the page's `_p.` ones, and posts that control's own address, so the
page's parameters go with it as they would from a real page.

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
stranger cannot see my todo" runs `Todos.Index.load`, `Db.orderBy`
included, and nothing in it notices if the order is backwards.

A **mutant** is a copy of the app with one small, deliberate bug: a
`requires` removed, a write that does nothing, an order reversed, a
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
again once per mutant. The to-do app before the tests this section adds
(its eleven `ok` lines and `11 passed, 0 failed` first):

```text
32 mutants: 18 killed, 14 survived  (score 0.56)
survived:
  App           update:16        redirect → rerender    Redirect (Auth.Login {}) → Rerender
  Todo          Todo:16          cascade ↔ restrict     owner @ref User @restrict → owner @ref User @cascade
  Todo          matches:21       arm swapped            match filter { All ->… → the Active and Completed bodies swapped
  Todo          matches:23       not dropped            not todo.done → todo.done
  Todo          filterLabel:28   arm swapped            match filter { All -> "All"… → the All and Active bodies swapped
  Todo          filterLabel:28   arm swapped            match filter { All ->… → the Active and Completed bodies swapped
  Todos.Index   load:18          orderBy reversed       Db.orderBy Todo.created.desc → Db.orderBy Todo.created.asc
  Todos.Show    requires:7       requires dropped       requires user → served without user (user = User.UserId "mutant")
  Todos.Show    update:32        write replaced         let _ <- Db.update Todo model.todo.id … → let _ <- Task.succeed ()
  Todos.Show    update:32        update field dropped   { t | title: p.form.title, due: p.form.due } → { t | due: p.form.due }
  Todos.Show    update:32        update field dropped   { t | title: p.form.title, due: p.form.due } → { t | title: p.form.title }
  Todos.Show    update:33        redirect → rerender    Redirect (Todos.Index {}) → Rerender
  Todos.Show    update:36        write replaced         let _ <- Db.delete Todo model.todo.id → let _ <- Task.succeed ()
  Todos.Show    update:37        redirect → rerender    Redirect (Todos.Index {}) → Rerender
```

A test failed for 18 of the 32 mutants: the score is 18 / 32. A
survivor's line gives its module; `declaration:line` (`load:18` is line
18 of `app/Todos/Index.alder`; `load:30:17` adds the column when the
line has mutants of that kind in two places); the operator; and the code
before `→` after, shortened with `…` (an arm swap names its arms).

### The operators

Each mutant comes from one of twenty **operators**, named by what it
breaks, in five classes; every mutant is well-typed:

| Class | Operator | What it changes |
|---|---|---|
| authorization | `requires dropped` | A page's `requires user` no longer redirects a visitor, who is served as `User.UserId "mutant"` (for a field that is a brand over `Text`) |
| authorization | `role dropped` | `requires user, role R` becomes `requires user` |
| authorization | `scope dropped` | `Db.mine T` becomes `Db.from T`; the other scoped operations act on anyone's row |
| authorization | `guard dropped` | An `if` or `match` with one arm ending in `Task.notFound`, `Query.notFound` or `Task.fail` becomes its other arm: the check is gone |
| data | `where dropped` | One `Db.where` of a query removed |
| data | `orderBy reversed` | A `Db.orderBy`'s `asc` and `desc` swapped |
| data | `limit dropped` | A query's `Db.limit` removed |
| data | `offset dropped` | A query's `Db.offset` removed |
| data | `write replaced` | A `let … <-` insert, update or delete becomes `Task.succeed` of a placeholder (`()`, or an id `"mutant"`) |
| data | `update field dropped` | One field of a record update `{ t \| a: …, b: … }` that sets two or more left out, wherever it is |
| data | `session put dropped` | A `Session.put` step becomes `Task.succeed ()` |
| flow | `redirect → rerender` | `Redirect page` becomes `Rerender` |
| flow | `reject → rerender` | `Reject "message"` becomes `Rerender` |
| flow | `arm swapped` | Two neighbouring arms of a `match` trade bodies (one mutant per pair) |
| logic | `condition negated` | `if c` becomes `if not c` |
| logic | `operator flipped` | `==`↔`!=`, `<`↔`<=`, `>`↔`>=`, `&&`↔`\|\|` |
| logic | `bool flipped` | `True`↔`False` |
| logic | `not dropped` | `not e` becomes `e` |
| schema | `default changed` | A table field's default: `False`↔`True`, a number → one more, `""`→`"x"`, a union's constructor → the next one |
| schema | `cascade ↔ restrict` | A reference's delete rule, `@cascade`↔`@restrict` |

### The rules' mutants in a report

The two operators for the declared rules of chapter 6 ("Rules") read
like this in a report. From `alder test --mutate -run '^Todo '
examples/todos`, whose four tests open no page, the to-do app's four
`scope dropped` survivors:

```text
  Todos.Index   load:17      scope dropped          Db.mine Todo → Db.from Todo
  Todos.Index   update:42    scope dropped          Db.updateMine Todo p.id (\t -> { t | done: not t.done }) → Db.update Todo p.id (\t -> { t | done: not t.done }), not found when missing
  Todos.Index   update:46    scope dropped          Db.deleteMine Todo p.id → Db.delete Todo p.id, not found when missing
  Todos.Show    load:13      scope dropped          Db.getMine Todo params.id → Db.get Todo params.id, not found when missing
```

and from the blog's (chapter 14), with only `Post`'s tests run:

```text
  Admin         requires:10   role dropped           requires user, role Author → requires user
  Admin.Edit    requires:10   role dropped           requires user, role Author → requires user
```

"Not found when missing" is the replacement's whole behaviour: the
mutant still answers 404 for an id that names no row, so only a test
with someone else's row tells it from the app. `Db.insertMine` gets no
`scope dropped` mutant (it is skipped, "the owner comes from the session
either way"), but its write can be replaced. Each of these is killed by
a test that acts as a stranger or as the wrong role: in the to-do app,
"a stranger cannot see my todo" (the list and the todo's page),
"Toggle by another user is not found" and "Delete by another user is not
found"; in the blog, "a Reader cannot open the admin page", "a demoted
author can edit until they log in again", "the admin page lists the
author's own posts, newest first" and "another author's post is not
found".

### A surviving mutant is a test you have not written

Each survivor is a behaviour no test checks: write the test that would
notice. `Todos.Show requires:7` says the todo page could be served to a
visitor; nothing opened it as one. The test, in `app/Todos/Show.alder`:

```alder
test "a visitor is sent to log in from a todo" = {
  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
  let id <- Db.insert Todo { title: Text.NonEmpty "Milk", due: Nothing, owner: me }
  let visitor <- Test.open (Todos.Show { id: id })
  Task.succeed (visitor.answer == Redirected (Auth.Login {}))
}
```

`Todos.Show update:32` and `update:33` say saving could store nothing,
drop a field, or stay on the page. One test that saves the form and
checks all three kills all four:

```alder
test "saving changes the title and the due date" = {
  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
  let id <- Db.insert Todo { title: Text.NonEmpty "Milk", due: Nothing, owner: me }
  let _ <- Test.login { user: Just me }
  let _ <- Test.get (Todos.Show { id: id })
  let r <- Test.submit Todos.Show.EditTodo { title: Text.NonEmpty "Bread", due: Date.fromText "2026-10-12" }
  let after <- Db.get Todo id
  Task.succeed ({ answer: r.answer, title: Maybe.map (\t -> t.title) after, due: Maybe.andThen (\t -> t.due) after } == { answer: Redirected (Todos.Index {}), title: Just (Text.NonEmpty "Bread"), due: Date.fromText "2026-10-12" })
}
```

Nine tests, now in the example, kill all fourteen: the report then ends
`32 mutants: 32 killed, 0 survived  (score 1.00)`.

### How the two tests kill them

Against the app the visitor is `Redirected (Auth.Login {})`. Against the
`requires dropped` mutant the page is served as the user `"mutant"`,
`load`'s `Db.getMine` finds no todo of that user's with the id, and the
answer is `PageMissing`: the test fails, and the mutant is killed.
`Todos.Index` has the same `requires`, and its mutant was killed
already, by "a visitor is sent to log in, and can log out".

The second test reads the row back with `Db.get`, so a save that wrote
nothing, or left a field out, shows in `title` or `due`; and it checks
the answer, so a `Rerender` shows too. (`Date.fromText` gives a `Maybe
Date`, which is the field's type.) In the module the form is written
`EditTodo`; this chapter's blocks are checked as a module of their own,
so it is qualified here.

### All fourteen, and one killed by luck

`examples/todos/README.md` lists each of the nine tests with the
survivors it kills. Two are worth a word. `Todo countOpen:35 not
dropped` (`not t.done → t.done`) was killed even before, but only by
luck: the old "countOpen ignores done todos" counted one open todo and
one done one, so counting the done ones instead also gave 1; only
"adding a todo through the page", whose `#summary` must say `"1 open"`,
noticed. The test now has two open todos and one done, so the mutant
gives 1 where the test wants 2. And the `App` survivor, the logout's
redirect, is the subject of "The logout redirect" below. After the nine
tests:

```text
$ alder test --mutate examples/todos
ok   App "Logout clears the user and goes to log in"
ok   Auth.Login "an unknown email is rejected"
ok   Auth.Login "logging in through the form"
ok   Auth.Login "a visitor is sent to log in, and can log out"
ok   Todo "countOpen ignores done todos"
ok   Todo "Active keeps only open todos"
ok   Todo "each filter has its label"
ok   Todo "a user with todos cannot be deleted"
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
20 passed, 0 failed
32 mutants: 32 killed, 0 survived  (score 1.00)
```

### What is never a mutant

Mutants are made only in the app's own code: the `load`, `update`,
helpers and plain values of every module, and its tables. Never in a
`test`, a `fake`, a `view` or a component (anything that makes `Html`),
or the standard library.

Some changes are never made, and are not counted in the score. Two kinds
are reported apart, in `--json`'s last object (below):

- **Suppressed** (`suppressed`): the compiler can prove the mutant is
  the same program as the original. The rules: an `arm swapped` of two
  identical bodies; an `orderBy reversed` under `Db.first` on a query by
  id; a `condition negated` of a literal `True` or `False`; an `operator
  flipped` whose two sides are the same expression; and a `default
  changed` on a field that no record of the app's code leaves out (no
  `Db.insert` row, no `Insert T` a helper builds), so the default never
  applies (the blog's `User.role`, which only tests rely on).
- **Skipped** (`skipped`): the operator does not apply, or another
  operator already makes the same program. A `condition negated` of `if
  a == b` (or `!=`) is skipped, because `operator flipped` already makes
  `if a != b`; so is one of `if not e`, which `not dropped` makes. (An
  ordering condition is negated: `<` flips to `<=`, while its negation is
  `>=`.) A `guard dropped` or an `arm swapped` is skipped where an arm
  uses what its pattern binds (in `Todos.Index.update`, the `Toggle p`
  arm uses `p`, so it cannot trade bodies with the `Delete p` arm, whose
  `p` is another record); a `requires` on a field that is not a brand
  over `Text`, a `Maybe` default and an `@setNull` rule have no
  well-typed change; and `Db.insertMine` has no `scope dropped`, since
  the owner comes from the session either way.

The to-do app has none suppressed and 5 skipped (4 arm swaps of `msg`
and `found` matches whose arms use what they bind, and the `scope
dropped` of `Db.insertMine`); the blog 1 and 3.

### Survivors that are not gaps

Sometimes no test can tell a survivor from the app: then say why in a
comment or a README; otherwise write the test. No annotation silences a
survivor: it stays until a test kills it or the code changes.

### The logout redirect

Before the new tests, `App update:16 redirect → rerender` survived, although "a visitor is sent to log in,
and can log out" clicks Log out and checks that the answer is
`Redirected (Auth.Login {})`. Every Log out button of the to-do app is
on a page that `requires user`. Under the mutant the click answers
`Rerender`: the browser asks for the same page again, with the user now
gone from the session, and `requires user` redirects it to the login
page. Either way the user ends up there, so no browser test can tell
`Redirect` from `Rerender` here. A test of `App.update` itself can,
since it sees the `Nav` value before any browser follows it. The example
has one. It must be in `app/App.alder`: `App.update` is called by the
runtime, and a test anywhere else that calls it is E0002. (This block is
not checked on its own, since it belongs in that module; it is the
file's own text.)

```alder fragment
test "Logout clears the user and goes to log in" = {
  let r <- Test.run (update Logout { user: Just (User.UserId "me") })
  Task.succeed ({ value: r.value, trace: r.trace } == { value: Ok (Redirect (Auth.Login {})), trace: ["Session.put"] })
}
```

(The blog has a Log out button on its front page, which a visitor may
open, so there a browser test kills the same mutant.)

### Mutants that are the same program

After its ten new tests, the blog keeps three survivors (here and
below, `…` stands for the 39 `ok` lines):

```text
$ alder test --mutate examples/blog
…
39 passed, 0 failed
58 mutants: 55 killed, 3 survived  (score 0.95)
survived:
  Posts.Index   load:30:17   operator flipped   params.page < 1 → params.page <= 1
  Posts.Index   load:30:48   operator flipped   params.page > lastPage → params.page >= lastPage
  Posts.Index   load:39      limit dropped      Db.limit (perPage + 1) → no limit
```

The first line of `Posts.Index.load` clamps the page number:

```alder fragment
  let page = if params.page < 1 then 1 else if params.page > lastPage then lastPage else params.page
```

Two `operator flipped` mutants sit on this line, so their lines carry
the column: `load:30:17` is `params.page < 1`, `load:30:48` is `params.page
> lastPage`. `<` and `<=` differ only when `params.page` is 1, and then
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

### The exact score

With no survivor the report ends at the score line. The comparison
uses the exact score, not the two decimals printed: the blog's is 55 /
58 = 0.948…, printed as 0.95, so

```text
$ alder test --mutate --min 0.95 examples/blog
…
39 passed, 0 failed
58 mutants: 55 killed, 3 survived  (score 0.95)
survived:
  Posts.Index   load:30:17   operator flipped   params.page < 1 → params.page <= 1
  Posts.Index   load:30:48   operator flipped   params.page > lastPage → params.page >= lastPage
  Posts.Index   load:39      limit dropped      Db.limit (perPage + 1) → no limit
$ echo $?
1
```

while `--min 0.94` passes. A CI job runs `alder test --mutate --min R`
with `R` at the score the app has reached, so a change that lets a
mutant survive fails the build. The examples do this in the Go test
suite: `TestExample_TodosMutation` and `TestExample_Blog` require at
least 1.00 and 0.94, and the number of mutants their READMEs record.
Without `--min`, the exit code is the tests'.

### `-run`, `-timeout` and `--json`

`-run re` selects tests as it does without `--mutate`, and the mutants
are judged by the selected tests only. With only `Todo`'s four tests,
most mutants survive:

```text
$ alder test --mutate -run '^Todo ' examples/todos
ok   Todo "countOpen ignores done todos"
ok   Todo "Active keeps only open todos"
ok   Todo "each filter has its label"
ok   Todo "a user with todos cannot be deleted"
4 passed, 0 failed
32 mutants: 7 killed, 25 survived  (score 0.22)
survived:
…
```

(25 survivor lines follow, the four `scope dropped` of "The operators"
among them.) This is useful while writing the tests of
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
per test; the to-do app's 32 take under a second once the build is made,
because each mutant stops at the first test that fails.

`--json` prints the per-test objects as usual, then one object listing
every mutant, then one with the totals. A mutant's object has its `id`,
`module`, `file`, `decl`, `line`, `col` (the column), `kind`, `before`,
`after`, whether it was `killed`, `by` which test (the first that
failed), and a `note` (`"crashed"`, `"timed out"`, or empty). From the
to-do app's run, two of the 32:

```text
{"id":16,"module":"Todos.Index","file":"app/Todos/Index.alder","decl":"load","line":18,"col":10,"kind":"orderBy reversed","before":"Db.orderBy Todo.created.desc","after":"Db.orderBy Todo.created.asc","killed":true,"by":"Todos.Index \"the newest todo is listed first\"","note":""}
{"id":7,"module":"Todo","file":"app/Todo.alder","decl":"Todo","line":16,"col":3,"kind":"cascade ↔ restrict","before":"owner @ref User @restrict","after":"owner @ref User @cascade","killed":true,"by":"Todo \"a user with todos cannot be deleted\"","note":"","tables":"ensure"}
```

and the last line:

```text
{"score":1,"killed":32,"survived":0,"suppressed":0,"skipped":5}
```

`score` is exact here (1 when every mutant was killed). `"tables":
"ensure"` marks a **schema mutant** (`default changed`, `cascade ↔
restrict`): its tests run on tables made from the mutated table
declarations by `Ensure`, as for an app without `migrations/`
(chapter 8, "Migrations"), not from the app's migrations, since the question is
whether a test would notice the changed rule, not whether a migration
exists for it.

### Ids and lines change with the code

Mutants are numbered in a fixed order: module by module, and within a
module its page's `requires`, then its tables field by field, then its
declarations in order, each from the outside in. The same source always
gives the same numbers, so two runs can be compared. An edit renumbers
every mutant after it, and moves the lines. Modules come in import order
(a module after those it imports, otherwise alphabetical), so adding an
import can move a whole module and renumber mutants in code the edit
never touched. When the to-do app's `Todos.Show` gained `import User`
for its new tests, its mutants kept their ids (then 25 to 33), since
`Todos.Show` already came after `User`, but each moved down one line:
`requires:6` became `requires:7`, `update:32` became `update:33`. When
the app later adopted the rules of chapter 6, the edits to `Todos.Index`
renumbered everything after them again: `Todos.Show`'s mutants are now
25 to 32. Ids
and lines are for reading one report; to compare runs, compare what
survived.

## 12. What cannot be tested yet

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
