# B. Common mistakes

The errors models make most often when writing Alder, from the experiments
in chapter 12. Each is a compile error.

1. `Db.get T x` takes `T`'s `@id` value only. To look up by another column:
   `Db.from T |> Db.where (T.col.eq x) |> Db.first`.
2. `Db.get` and `Db.first` return a `Maybe`. Match it; there is no `.field`
   on a `Maybe`.
3. Types from another module are qualified (`User.UserId`, `Todo.Priority`).
   Constructors are not (`Normal`, `Just`). The qualifier is the last
   segment of the import: `import Table.Todo` gives `Todo.find`, and
   writing the full path (`Table.Todo.find`) is E0205. Where two imports
   or a declared name would clash, the import carries the canonical alias
   (`import Admin.Layout as AdminLayout`); copy it from the E0204 fix or
   run `alder fmt`. Pages stay full path (`Todos.Show {}`), but their
   members go through the qualifier (`Index.helper` after `import
   Todos.Index`).
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
    `<Msg>Params` are taken by the compiler (E0012), and in any module a
    Prelude type's name (`form Query`) is too (E0012).
14. A `<script>` or `<style>` body may not contain `</script` or `</style`,
    nor be wholly one `{ … }` that is not JSON
    (`<script>{model.code}</script>`) (E0001): the body is raw text, not
    Alder, so `{x}` in it is never a value. Pass data through a `data-*`
    attribute (`<div data-clip={model.clip}>`) and read it from the script.
