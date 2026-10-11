# 10. Testing

A test is a piece of the program that checks another piece and says yes or
no. In Alder a test is a declaration, written in the module it tests, after
the code, and `alder test` runs every test in the app. This chapter starts
with the simplest kind, a test of a pure function, then reads the report
`alder test` prints, then tests code that reads and writes the database,
and ends with what cannot be tested yet.

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
  ] == 1
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
ok   Todo "countOpen ignores done todos"
ok   Todos.Index "Toggle flips done"
ok   Todos.Index "Toggle by another user is not found"
3 passed, 0 failed
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

and one at the end of `Todos.Index` that expects `Toggle` to make a
single step (sections 3 to 6 explain effect tests and traces):

```alder
test "Toggle only updates" = {
  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
  let id <- Db.insert Todo { title: Text.NonEmpty "Milk", due: Nothing, owner: me }
  let r <- Test.run (Todos.Index.update (Toggle { id: id }) { filter: All, todos: [] } { user: me })
  Task.succeed (r.trace == ["Db.update Todo"])
}
```

In their modules they are written without the `Todo.` and `Todos.Index.`
prefixes. Both compile; both are wrong:

```text
$ alder test examples/todos
ok   Todo "countOpen ignores done todos"
FAIL Todo "an empty list has one open todo"
  left:  0
  right: 1
ok   Todos.Index "Toggle flips done"
ok   Todos.Index "Toggle by another user is not found"
FAIL Todos.Index "Toggle only updates"
  left:  ["Db.get Todo", "Db.update Todo"]
  right: ["Db.update Todo"]
  trace: ["Db.insert User", "Db.insert Todo", "Test.run", "Db.get Todo", "Db.update Todo"]
3 passed, 2 failed
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

A `Task` cannot be compared with `==` (section 8 says why), so a test of
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
  let r <- Test.run (Todos.Index.update (Toggle { id: id }) { filter: All, todos: [] } { user: me })
  let after <- Db.get Todo id
  let done = Maybe.map (\t -> t.done) after
  Task.succeed ({ trace: r.trace, done: done } == { trace: ["Db.get Todo", "Db.update Todo"], done: Just True })
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
3. Run `update` under `Test.run` (section 4) and call the outcome `r`.
   `update` takes the message (`Toggle` for that todo), the page's model
   (`load` is not called; `update` only needs a value of the type), and
   the session. `Todos.Index` `requires user`, so its `Session` is a type
   of its own whose `user` is a `UserId`, not a `Maybe` (chapter 6). The
   record literal `{ user: me }` is that type because it is
   written where `update` expects one. Bound to a name first by a `let`
   without an annotation, it would be a plain record, which `update`
   does not accept.
4. Read the todo again, and take its `done` (`after` is a `Maybe Todo`:
   `Nothing` if the row were gone).
5. The verdict, as one `==` between two records: `update` read the todo
   and updated it, and the stored todo is now done. Putting everything
   the test checks into one record, rather than joining several `==`
   with `&&`, means a failure prints both records whole (section 2).

The second test is the authorization rule of chapter 6, "Authorize in
`update` too": a user who posts `Toggle` with someone else's todo id gets
a 404 and changes nothing.

```alder
test "Toggle by another user is not found" = {
  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
  let id <- Db.insert Todo { title: Text.NonEmpty "Milk", due: Nothing, owner: me }
  let stranger = User.UserId "someone-else"
  let r <- Test.run (Todos.Index.update (Toggle { id: id }) { filter: All, todos: [] } { user: stranger })
  let after <- Db.get Todo id
  let done = Maybe.map (\t -> t.done) after
  Task.succeed ({ value: r.value, trace: r.trace, done: done } == { value: Err "not found", trace: ["Db.get Todo"], done: Just False })
}
```

The session's user, `stranger`, is not a row and need not be: a session
holds an id, not a reference the database checks. `owned` reads
the todo, sees another owner and stops with `Task.notFound`; the update
never runs. If someone deleted the owner check from `owned`, this test
would say exactly what went wrong:

```text
$ alder test examples/todos -run another
FAIL Todos.Index "Toggle by another user is not found"
  left:  { value: Ok Rerender, trace: ["Db.get Todo", "Db.update Todo"], done: Just True }
  right: { value: Err "not found", trace: ["Db.get Todo"], done: Just False }
  trace: ["Db.insert User", "Db.insert Todo", "Test.run", "Db.get Todo", "Db.update Todo", "Db.get Todo"]
0 passed, 1 failed
```

The stranger's `Toggle` succeeded, the update ran, and the todo is done.

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
- **The app's capabilities.** When the app has an `adapters/` package
  (chapter 7, "Implementing a capability"), its `Caps()` are wired, so a
  test calls the real Go. A capability's error is a failure the test can
  observe: under `Test.run` (section 4) it is `Err "Password.verify: …"`.
  When the app has no `adapters/`, a capability call fails the test,
  saying so:

  ```text
  FAIL Auth.Login "a stranger is not verified"
    error: capability Auth.Login.Password is not wired in tests
    trace: ["Password.verify"]
  ```

- **The app's default session.** A test's `Session.put` starts from
  `App`'s session with every field at its default (here
  `{ user: Nothing }`). A test of `update` passes the
  session it wants as an argument, as above.

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
test "Toggle by another user is not found"   (a fresh database in memory)

  BEGIN                                       first database step: the transaction opens
  │ Db.insert User
  │ Db.insert Todo
  │ Test.run ── SAVEPOINT alder_t1
  │             │ Db.get Todo                 owned: the todo is someone else's
  │             │ Task.notFound               update fails
  │             └ ROLLBACK TO alder_t1        undo update's writes (it made none)
  │               RELEASE alder_t1            r.value = Err "not found"
  │ Db.get Todo                               the todo is still there, not done
  COMMIT                                      the verdict was produced
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
  `"Db.count Todo"`;
- a capability call is the capability and its function, as the calling
  module writes them: `"Password.verify"`;
- `Test.run` is a step itself, `"Test.run"`, in the test's trace.

The steps inside a `Test.run` appear twice: in `r.trace`, which holds
only them, and in the test's own trace, after `"Test.run"`, even when
the run failed and its writes were undone. That is why the report of
"Toggle only updates" above has the trace
`["Db.insert User", "Db.insert Todo", "Test.run", "Db.get Todo", "Db.update Todo"]`
while its `r.trace` was `["Db.get Todo", "Db.update Todo"]`. Compare
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
{"module":"Todo","name":"countOpen ignores done todos","ok":true,"left":"1","right":"1","trace":[],"ms":0.009}
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

## 8. What cannot be tested yet

- **A request's round trip.** A test calls `load` and `update` directly.
  Nothing yet sends an HTTP request to the app and reads the response,
  so routing, the decoding of a submitted form, `requires` redirecting
  to the login page and the page that a `Nav` turns into are checked by
  the compiler and by the CLI's own Go tests, not by `alder test`.
- **`view` in a browser.** A test can compare `view`'s `Html` with
  another `Html` (section 1). What a browser does with it, and the
  in-place updates of chapter 6, are not testable from Alder.
- **Replacing a capability with a fake.** A test runs with the app's own
  adapters (section 3), or with none. There is no way yet to give one
  test a stand-in `Password` that, say, accepts everyone.
- **Observing the session after `Test.run`.** `Test.run` gives the run's
  value and trace, not the session it left; a test of a login cannot yet
  check that `Session.put` stored the user.

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
