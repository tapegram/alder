# B. Common mistakes

The errors models make most often when writing Alder, from the experiments
in chapter 12. Each is a compile error.

1. `Db.get T x` takes `T`'s `@id` value only. To look up by another column:
   `Db.from T |> Db.where (T.col.eq x) |> Db.first`.
2. `Db.get` and `Db.first` return a `Maybe`. Match it; there is no `.field`
   on a `Maybe`.
3. Types from another module are qualified (`User.UserId`, `Todo.Priority`).
   Constructors are not (`Normal`, `Just`).
4. Every module whose types, functions, columns, or components you use must
   be imported. Only pages (`Todos.Show {}`) and app-level messages
   (`App.Logout`) need no import.
5. A message with parameters is sent with them: `click={Delete { id: x }}`,
   `submit={Comment { post: id }}`. Only a message declared with `{ }` is
   written bare.
6. Actions shared by every page (log out, theme) are declared once, in
   `App`, and sent as `App.Logout` from any page.
7. A shared component outside a page module is typed `Html a`, never
   `Html Msg`.
8. `Date.format` takes a `Date`; `Time.format` takes a `Time`. Convert with
   `Time.toDate`.
9. `++` joins `Text` and lists. `+` is for numbers only.
10. `Db.insert` never takes `@id`, `@created`, or `@updated` fields.
11. `let` statements live inside a `{ }` block. A function body with a
    `let` is a block.
12. There is no `case ... of`, no `let ... in`, no `andThen`, no tuples,
    and no type aliases.
13. Names may not contain `__` (`max__len`) and module names may not end
    with `_`; in a page module or `App`, `Params`, `Msg`, `Session` and
    `<Msg>Params` are taken by the compiler (E0012).
