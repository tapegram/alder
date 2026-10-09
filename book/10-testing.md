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
descriptions, they can be tested without a database or a server:

```alder
test "delete removes the row and stays on the page" =
  update (Delete { id: TodoId "a" }) model session
    == { let _ <- Db.delete Todo (TodoId "a")
         Task.succeed Rerender }
```

Two effect values are equal when they describe the same operations with
the same arguments. An effect that contains a function (`Db.update`) cannot
be compared with `==`; test the function separately. HTML values are equal
when they render to the same markup.

Within a test, a brand constructor may be used from any module.
