# 10. Testing

A test is a piece of the program that checks another piece and says yes or
no. In Alder a test is a declaration, written in the module it tests, after
the code, and `alder test` runs every test in the app. This chapter starts
with the simplest kind, a test of a pure function, then reads the report
`alder test` prints, then tests code that reads and writes the database,
replaces a capability with a fake written in Alder, and ends with what
cannot be tested yet.

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

A `Task` cannot be compared with `==` (section 9 says why), so a test of
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
  A test can replace a capability with a fake instead (section 8).
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

## 8. Faking a capability

A capability's adapter is Go code that reaches outside the program
(chapter 7): the blog's `Markdown` renderer, a card processor, a mail
service. Section 3 runs a test against the app's adapters, or against
none. Often neither is what a test wants. A test of a checkout should
not charge a real card, and a test of a page should not depend on how
Markdown is rendered. For those, a module can declare a **fake**: a
second implementation of a capability, written in Alder beside the
capability, that only tests use. A fake acts as a real implementation
would, just in memory. It answers every entry of the capability, and it
can keep state in tables of its own, so a charge and then a refund of
that charge behave as they would against the real service.

### A fake without state: `Markdown`

The blog's `app/Markdown.alder` (chapter 14) declares the capability and,
after it, its fake:

```alder
module Markdown

-- Rendering a post's body is outside the language: a capability implemented in Go
-- (adapters/markdown.go), which builds the HTML node by node, so a post cannot inject markup.
capability Markdown {
  render : Text -> Query (Html Never),
}

-- In a test, `let _ <- Test.fake Markdown` replaces the adapter with this: the body as plain
-- text, so a test about a page asserts on the page, not on how Markdown is rendered.
fake Markdown {
  render text = Query.succeed (Html.text text)
}
```

`fake Markdown { … }` names the capability it implements and holds one
definition for each of its entries, here `render`. A definition has no
signature: its type is the entry's, `Text -> Query (Html Never)`, so
`text` is a `Text` and the body must be a `Query (Html Never)`. This
one wraps the text in a single text node, unrendered.

A test installs the fake with `Test.fake`, a step like `Test.run`:
`let _ <- Test.fake Markdown`. From that step on, until the test ends,
every call of `Markdown.render` in the test (in `load`, in a function
`load` calls, anywhere) runs the fake instead of the adapter. These two
tests, in a module of their own, call the entry once with the fake and
once without it:

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

Added to a copy of the blog as `app/Preview.alder`, both pass:

```text
$ alder test blog -run Preview
ok   Preview "the fake leaves the text as it is"
ok   Preview "the adapter renders the Markdown"
2 passed, 0 failed
```

The traces show what changed and what did not. Installing is a step,
`"Test.fake Markdown"`. The call is `"Markdown.render"` either way: a
fake's entry is traced by the entry's usual name, as a call to the
adapter is. So a test that compares only the steps of the code it tests
(a `Test.run`'s `r.trace`) gets the same list with this fake and with
the adapter, while a comparison of the test's whole trace sees the extra
first step. (A fake whose entries take steps of their own, such as
reading a table, adds those steps after the entry's; the next example
has one.)

The blog itself has one test of each kind in `Posts.Show`. "a draft is
shown to its author" goes through the real adapter and checks that
`Not *yet*.` becomes `<p>Not <em>yet</em>.</p>`: the wiring works. "a
post's body is rendered through the Markdown capability" installs the
fake and checks that `load` puts the body in its Model, whatever
Markdown does to it. It inserts an author and a published post whose
body is `Not *yet*.`, calls `load` for the post's slug, and checks two
things: the body is `Html.text "Not *yet*."`, and `"Markdown.render"` is
in its `Test.trace`, so the body went through the capability. Its
trace, from `alder test examples/blog --json`, is
`["Test.fake Markdown", "Db.insert User", "Db.insert Post", "Db.first Post", "Markdown.render", "Db.all Comment", "Test.trace"]`
(the report's trace includes the `Test.trace` step, which the list
`Test.trace` returns leaves out); the adapter's test has the same steps
from `Db.insert User` to `Db.all Comment`. The rendering itself is tested where it is written, in Go
(`adapters/markdown_test.go`). That is the usual split: fake a
capability in the tests of the code that uses it, and keep one test
through the real adapter, so the app's wiring is tested too.

### A fake with state: a card processor

A fake that only answers is enough for a renderer. A card processor
remembers: a refund must name a charge it made. This module, `Payments`,
is not part of an example; it and the module after it were run as an
app of their own, `shop`. Its capability charges and refunds, and its
fake keeps each charge as a row:

```alder
module Payments

type ChargeId = brand Text

-- The card processor. In production, Go code in adapters/ implements it (chapter 7).
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

Piece by piece:

- **The fake's table.** `table Charge { … }` inside the fake is a table
  like any other (chapter 8): a record type `Payments.Charge` with an
  `@id`, columns and a default, read and written with `Db`. Two things
  differ. It is declared inside the fake, not in a module named after
  it, so the rule that a table lives in its own module (E0018) does not
  apply. And it exists only in the databases of tests, so only tests and
  the fake itself may use it.
- **The entries.** `charge args = { … }` and `refund args = { … }` are
  ordinary Alder, typed by the capability: `args` is
  `{ cents: Int.Positive }` in `charge` and `{ id: ChargeId }` in
  `refund`, and each body is a `Task (Result Text …)`. They read and
  write the fake's table as a `Task` reads and writes any table.
  `charge` inserts before it decides, so its insert is there to be
  undone when it fails (below).
- **The test.** It installs the fake, charges 250, refunds that charge,
  reads the row back with `Db.get Charge id`, and compares. Its trace is
  `["Test.fake Stripe", "Stripe.charge", "Db.insert Charge", "Stripe.refund", "Db.get Charge", "Db.update Charge", "Db.get Charge"]`:
  each entry by its usual name, followed by the steps the fake took
  inside it, then the test's own `Db.get Charge`.

Code that uses the capability does not change. A checkout in another
module calls `Stripe.charge` as it always would, and its tests install
the fake by the capability's name:

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

From `Checkout`, the table is `Payments.Charge` and its columns are
`Payments.Charge.cents`, with the module's name in front. The second
test is the failing entry. Charging 500000 makes `charge` insert a row
and then `Task.fail "card declined"`. Under `Test.run` (section 4) that
is `Err "card declined"`, the message as `Task.fail` wrote it (an
adapter's Go error would read `Stripe.charge: <the error>`, section 4).
The run's trace holds the fake's own step, `Db.insert Charge`, and
the savepoint undid that insert, so one row is left, the 250 charge.
(A `Result` `Err` is a different thing: an answer `pay` handles, as
`"refused: …"`. `Task.fail` imitates a processor that does not answer.)

```text
$ alder test shop
ok   Payments "a refunded charge is marked refunded"
ok   Checkout "an order is paid"
ok   Checkout "a declined charge leaves no row"
3 passed, 0 failed
```

Two more tests, added at the end of `Checkout` to show the failures: one
charges 500000 outside `Test.run`, and one calls `pay` without
`Test.fake` in an app that has no `adapters/`:

```text
$ alder test shop -run Checkout
ok   Checkout "an order is paid"
ok   Checkout "a declined charge leaves no row"
FAIL Checkout "a large order is paid"
  error: card declined
  trace: ["Test.fake Stripe", "Stripe.charge", "Db.insert Charge"]
FAIL Checkout "an order without the fake"
  error: capability Payments.Stripe is not wired in tests
  trace: ["Stripe.charge"]
2 passed, 2 failed
```

Outside `Test.run`, the fake's failure is the test's failure, as any
failing step is. Without `Test.fake` the test gets the app's adapter,
and this app has none.

### The rules

**The declaration.**

- A fake is declared in the module that declares its capability. A fake
  elsewhere is E0020. To give the to-do app's login tests a `Password`
  that accepts any user who has a row, whatever the password, the fake
must go in `app/Auth/Login.alder`,
  beside `capability Password`, not in the module that wants it:

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
  A body that does not fit is an ordinary type error, named after the
  entry: "in `fake Stripe`'s `charge`: expected Result Text
  Payments.ChargeId, got Text".
- A fake holds only `table` declarations and entry definitions, in any
  order. Types the fake needs (`ChargeId`) are declared in the module.
- Entry bodies are ordinary Alder. They may use `Db` on the fake's
  tables and on the app's, values of their own module and of the
  modules it imports, and the capability's other entries. Inside
  `fake Stripe`, `Stripe.refund` calls the installed implementation,
  which is the fake (a fake runs only once installed), never the
  adapter. An entry that calls itself unconditionally (`tick = Clock.tick`)
  recurses forever, like any function that does; the test crashes with
  `fatal error: stack overflow` (section 2).

**The fake's tables.**

- A table of `fake Stripe` in module `Payments` is the record type
  `Payments.Charge`, as an app table of that module would be. It is
  exempt from E0018 (a table in its own module).
- Only a `test`, in any module, and the fake's own entries may use it,
  its type or its columns. Anywhere else (a value, a signature, a page's
  `load`, an app table's `@ref`) it is E0021. Here `Checkout` has gained
  a value that counts the charges:

  ```text
  $ alder check shop
  app/Checkout.alder:31:19: error[E0021]: `Charge` is a fake's table: only tests and `fake Stripe` may use it
    charges = Db.from Payments.Charge |> Db.count
                      ^^^^^^^^^^^^^^^
  ```

- In another module, the table and its columns carry the module's name:
  `Payments.Charge`, `Payments.Charge.cents`. Here the second `Checkout`
  test writes `Charge.cents.eq 250` instead:

  ```text
  $ alder check shop
  app/Checkout.alder:25:52: error[E0002]: `Charge` is a table of `fake Stripe` in module `Payments`: write `Payments.Charge.cents`
      let rows <- Db.from Payments.Charge |> Db.where (Charge.cents.eq 250) |> Db.count
                                                       ^^^^^^^^^^^^
  ```

- A fake's table may `@ref` an app table (`user: User.UserId @ref User`):
  a fake can keep, say, which user each charge was for. An app table may
  not `@ref` a fake's (E0021): the app's tables exist in production,
  where the fake's do not.
- The tables exist only in test databases. Each test's database is made
  as section 3 says, with the app's tables, by its migrations when it
  has them; then the fakes' tables are created beside them. They are
  never in `migrations/schema.json` or a migration, and `alder migrate`
  ignores them, so adding or changing a fake's table needs no
  migration.

**`Test.fake`.**

- `Test.fake Stripe` is a `Task Unit`, available only inside a `test`
  like the rest of `Test` (E0003 "`Test.fake` is only available inside
  a `test`"). Its argument is a capability's name, `Stripe` or
  `Payments.Stripe`, not an expression (E0003 "`Test.fake` wants a
  capability name, got an expression"), and the capability must have a
  fake (E0003 "`Test.fake` wants a capability with a `fake`: `Stripe` has
  none").
- It replaces the capability for the rest of the test: every later call,
  from any module, runs the fake. A `Test.fake` inside a `Test.run` stays
  installed after the run, even when the run fails: the savepoint undoes
  writes to the database, and installing is not one. Installing again
  replaces the fake with a new copy of itself; the rows in its tables
  stay, since they are in the database.
- Without it, a test calls the app's adapter, or fails with "capability
  … is not wired in tests" when the app has no `adapters/` (section 3).
- The trace shows `Test.fake Stripe`, then each call by the entry's
  name (`Stripe.charge`), then the steps the fake took inside it
  (`Db.insert Charge`). A trace of the code under test therefore has the
  same entry steps with the fake as with the adapter, plus the fake's
  own steps after each.
- A failing entry is a failing step: under `Test.run` it is `Err` with
  the entry's message, and the writes it made are undone; outside it,
  the test fails with that message.
- Each test starts with a fresh database and its own set of
  capabilities, so a fake installed in one test, and the rows it wrote,
  are never seen by the next.

**Where fakes exist.** Only `alder test` compiles them. `alder build`,
`alder run` and the programs of `alder eval` and `alder repl` contain no
fake and no fake's table. A `test` is compiled into a production build
too, but nothing there runs it: in that build, a test that uses a fake's
table becomes one that fails at once, and `Test.fake` is a step that
fails ("Test.fake is only available in `alder test`"). Chapter 13
("Fakes") shows the Go a fake becomes.

## 9. What cannot be tested yet

- **A request's round trip.** A test calls `load` and `update` directly.
  Nothing yet sends an HTTP request to the app and reads the response,
  so routing, the decoding of a submitted form, `requires` redirecting
  to the login page and the page that a `Nav` turns into are checked by
  the compiler and by the CLI's own Go tests, not by `alder test`.
- **`view` in a browser.** A test can compare `view`'s `Html` with
  another `Html` (section 1). What a browser does with it, and the
  in-place updates of chapter 6, are not testable from Alder.
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
