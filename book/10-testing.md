# 10. Testing

A test is a declaration whose body is a `Bool`:

```alder
test "summary counts open todos" =
  summary [{ id: TodoId "a", title: Text.NonEmpty "x", done: False, due: Nothing, owner: UserId "u", created: Time.epoch }] == "1 open"

test "matches Active excludes done" =
  not (matches Active { id: TodoId "a", title: Text.NonEmpty "x", done: True, due: Nothing, owner: UserId "u", created: Time.epoch })
```

Tests live in the module they test, after the code. `alder test` runs every
test in the project and reports each failing test with both sides of the
final `==` if the body is an equality.

Because `load`, `view`, and `update` are pure functions that return
descriptions, they can be tested without a server. `view` returns HTML,
and two HTML values are equal when they render to the same markup.

Effects are tested by what they do, not by `==`. An effect value holds Go
functions (each `let x <-` continues with a function of `x`; chapter 7,
"How effects run"), and functions can't be compared, so `==` on two
`Query` or `Task` values is not offered. Instead a test will run the
effect against a fresh database and compare its **trace**: the names of
the steps it performed, in order, which the runtime records in
`Env.Trace` (`rt/eff/eff.go`). The `Toggle` message of the to-do page
traces `["Db.update Todo"]`; the test can then also read the table to
check that `done` flipped. `alder test` is not built yet; the trace is
what it will compare.

Within a test, a brand constructor may be used from any module.
