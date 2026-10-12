# 10. Testing

A test is a piece of the program that checks another piece and says yes or
no. In Alder a test is a declaration, written in the module it tests, after
the code, and `alder test` runs every test in the app. This chapter starts
with the simplest kind, a test of a pure function, then reads the report
`alder test` prints, then tests code that reads and writes the database,
replaces a capability with a fake written in Alder, drives the app
through an in-memory browser as a user would, and ends with what cannot
be tested yet.

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
ok   Todos.Index "adding a todo through the page"
ok   Todos.Index "toggling a todo through the page"
ok   Todos.Index "a stranger cannot see my todo"
ok   Todos.Index "Delete removes the todo"
ok   Todos.Index "the newest todo is listed first"
ok   Todos.Show "a visitor is sent to log in from a todo"
ok   Todos.Show "saving changes the title and the due date"
ok   Todos.Show "deleting from the todo's page"
18 passed, 0 failed
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
ok   Todos.Index "adding a todo through the page"
ok   Todos.Index "toggling a todo through the page"
ok   Todos.Index "a stranger cannot see my todo"
ok   Todos.Index "Delete removes the todo"
ok   Todos.Index "the newest todo is listed first"
FAIL Todos.Index "Toggle only updates"
  left:  ["Db.get Todo", "Db.update Todo"]
  right: ["Db.update Todo"]
  trace: ["Db.insert User", "Db.insert Todo", "Test.run", "Db.get Todo", "Db.update Todo"]
ok   Todos.Show "a visitor is sent to log in from a todo"
ok   Todos.Show "saving changes the title and the due date"
ok   Todos.Show "deleting from the todo's page"
18 passed, 2 failed
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

A `Task` cannot be compared with `==` (section 11 says why), so a test of
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

- **The app's default session.** A test's `Session.put` starts from
  `App`'s session with every field at its default (here
  `{ user: Nothing }`). A test of `update` passes the
  session it wants as an argument, as above.
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
- `Test.run` is a step itself, `"Test.run"`, in the test's trace;
- a browser step is its name and what it names: `"Test.login"`,
  `"Test.get Todos.Index"`, `"Test.submit NewTodo"`,
  `"Test.click Todos.Index.Toggle"`, followed by the steps of the
  requests it made (section 9).

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
  trace: ["Db.insert User", "Test.login", "Test.get Todos.Index", "Db.all Todo", "Test.submit NewTodo", "Db.all Todo", "Db.insert Todo", "Db.all Todo", "Test.get Todos.Index", "Db.all Todo"]
0 passed, 1 failed
```

A page value prints as its address: `Redirected /` is
`Redirected (Todos.Index {})`, whose `filter` is the default and so not
in the URL. Two page values are equal when their addresses are. The
trace is each step's name, then the effects of the requests it made
(section 10 explains why `Db.all Todo` comes before the insert).

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
after a `Session.put`; `s` is checked against `App`'s `session` record
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
the cookie, followed the redirect with it, and holds the list. With an
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

**Who may toggle a todo.** Chapter 6's rule, "Authorize in `update`
too", says that a user who posts `Toggle` with someone else's
todo id gets a 404. Section 3 tests that by calling `update` with a
stranger's session. Through the browser, the same attempt (a test that
logs in as the stranger, gets the list, and clicks `Toggle { id: id }`
expecting `PageMissing`) does not get that far:

```text
FAIL Todos.Index "a stranger clicks my todo"
  error: the page / has no button or form for Todos.Index.Toggle { id: "tZy4DmaadTQ3si86vA7B" }
  trace: ["Db.insert User", "Db.insert Todo", "Test.login", "Test.get Todos.Index", "Db.all Todo", "Test.click Todos.Index.Toggle"]
```

The stranger's list shows only the stranger's todos, so there is no
Toggle button for mine to press; the browser sends nothing, and the test
fails. (The id is random, and differs on each run. Had the stranger's
list shown todos of their own, the text would end with the ones it
offers: " (the page has Toggle for id: …)".) That is the right answer,
and it is why both tests are kept: the page does not offer the action,
and section 3's test shows that `update` refuses it to anyone who posts
it anyway, without a page.

### A stranger's todo

Through the browser, the ownership test is written with what a stranger
can do, open the todo's address:

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

The stranger's list has no `<li>`, and `Todos.Show`'s `load` answers the
todo's page with `Query.notFound`: `PageMissing`.

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
  left:  <ul id="todos"><li class=""><form method="post" action="/_m/Todos.Index/Toggle?id=YivaANfICyEevYmgeIK_" class="_alder-c…
  right: <ul id="todos"></ul>
  note:  the documents differ at body > main > ul#todos: left 1 child <li>, right 0
  trace: ["Db.insert User", "Db.insert Todo", "Test.login", "Test.get Todos.Index", "Db.all Todo"]
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
  redirect, is not tested either (section 11).

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
  │   Db.all Todo                          load reads in the test's transaction
  │ Test.submit                            POST /_m/Todos.Index/Submit
  │   Db.all Todo                          load, for update's model
  │   SAVEPOINT alder_t1                   update is a Task
  │   │ Db.insert Todo
  │   └ RELEASE alder_t1                   it succeeded: the insert is the test's now
  │   Db.all Todo                          GET /: the redirect, followed
  │ Test.get                               GET /
  │   Db.all Todo
  COMMIT                                   the verdict
```

When an `update` fails, its savepoint is rolled back (`ROLLBACK TO`),
so its writes are gone and the test's are not. This is also why the
trace shows `Db.all Todo` before `Db.insert Todo`: a message's request
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

## 11. What cannot be tested yet

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
