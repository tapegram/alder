# B. Common mistakes

The errors models make most often when writing Alder, from the experiments
in chapter 12, and the mistakes routes make easy. Each is a compile error
unless the item says what happens instead.


1. `Db.get T x` takes `T`'s `@id` value only. To look up by another column:
   `Db.from T |> Db.where (T.col.eq x) |> Db.first`.

2. `Db.get` and `Db.first` return a `Maybe`. Match it; there is no `.field`
   on a `Maybe`.

3. Types from another module are qualified (`User.UserId`, `Todo.Priority`).
   Constructors are not (`Normal`, `Just`). The qualifier is the last
   segment of the import: `import Table.Todo` gives `Todo.find`, and
   writing the full path (`Table.Todo.find`) is E0205.

   Where two imports or a declared name would clash, the import carries the
   canonical alias (`import Admin.Layout as AdminLayout`); copy it from the
   E0204 fix or run `alder fmt`.

   Route values stay full path (`Todos.Show.Show { id: x }`, and
   `Todos.Index.Create` as the first argument of `Test.submit`), but a
   route's members go through the qualifier: after `import Todos.Index`, its
   `Row` route's view is `Index.Row.view`, and `Todos.Index.Row.view` is
   E0205.

4. Every module whose types, functions, columns, or components you use must
   be imported. Only route values (`Todos.Show.Show { id: x }`) and `App`'s
   routes (`App.Logout`) need no import.

5. A route with parameters is named with them: `click={Delete { id: x }}`,
   `submit={Comment { post: id }}`. Only a route declared with `{ }` is
   written bare; `click={Delete}` for a route that takes an `id` is E0003
   ("expected Route, got Delete.Params -> Route").

6. Actions shared by every page (log out, theme) are write routes declared
   once, in `App`, and sent as `App.Logout` from any page.

7. A route module's markup is `Html`. A shared component outside a route
   module (a layout, a card) is typed `Html a`, so that any route can use it.
   `Html Msg` is E0601.

8. `Date.format` takes a `Date`; `Time.format` takes a `Time`. Convert with
   `Time.toDate`.

9. `++` joins `Text` and lists. `+` is for numbers only.

10. `Db.insert` never takes `@id`, `@created`, or `@updated` fields.

11. `let` statements live inside a `{ }` block. A function body with a
    `let` is a block.

12. There is no `case ... of`, no `let ... in`, no `andThen`, no tuples,
    and no type aliases.

13. Names may not contain `__` (`max__len`) and module names may not end
    with `_`; in any module a Prelude type's name is taken (`form Query`,
    `type Route`, `form Response`: E0012).

14. A `<script>` or `<style>` body may not contain `</script` or `</style`,
    nor be wholly one `{ … }` that is not JSON
    (`<script>{model.code}</script>`) (E0001): the body is raw text, not
    Alder, so `{x}` in it is never a value. Pass data through a `data-*`
    attribute (`<div data-clip={model.clip}>`) and read it from the script.

15. A property argument with no annotation whose type nothing in the body
    decides is generated as an `Int` (`\xs -> List.reverse (List.reverse xs) == xs`
    tests `List Int`), and passes without ever drawing the type you meant. The
    failure report marks the choice, `xs: [0, 1]  (xs : List Int)`; write
    `\(xs : List Text) -> …` to pick the type.

16. `??` binds looser than `&&` and `||`, and `|>` looser than `==` and the
    other comparisons: `a && m ?? True` is `(a && m) ?? True`, and
    `x |> f == y` is `x |> (f == y)`. Write `a && (m ?? True)` and
    `(x |> f) == y`; the E0003 hint shows the form with your expressions.

17. A route named like another name in its module hides one of them, and
    the compiler refuses it (E0044):

    - a route named like an import's qualifier (`route Todo` beside
      `import Table.Todo`, because `Todo.view` would name the table's
      module);
    - a route named like a standard-library module (`route List`);
    - a route named like a constructor, brand or table in scope
      (`route Active` beside `type Filter = All | Active`);
    - a module `Todos.Row` beside a route `Row` in `Todos`.

    Rename the route or the module.

    A module whose qualifier is a capability's name (`import Clarity` beside
    `capability Clarity`) makes `Clarity.final` a name of the module: E0002
    with a fix when the module lacks `final`, W0010 when it has one (the
    capability is `DataClarity.final`). Rename one, as the message suggests.

18. `Todos.Index.Index {}` is not a typo. A dotted name is read as a module
    path, then a route of that module, then a member, so the to-do list is
    the module `Todos.Index`, its route `Index`, and its parameters `{}`.
    A route named like its module's last segment is the usual one-route
    file, and nothing shortens it.

19. `Response.render (Row { id: x })` patches the element that `Row`'s view
    draws, found by its root `id`, into the page the browser holds. When that
    page has no element with that `id` (the row is not on this page, or a
    filter hides it), a browser patches nothing and says nothing; `alder
    test` fails the step ("the answer patched `#todo-9`, which the page does
    not have").

    `render` is for something already on the page. For something that
    appears or disappears, or more than one place, answer `Response.refresh`.
    The view needs one root element with an `id`, or nothing is patched.

20. `href` takes a GET route and `submit` a write route that has a `form`;
    `Response.redirect` and `Response.render` take a GET route. The other way
    round is E0045. A write route has no `view`: it answers with a
    `Response`, which names the GET route to show.

21. A write route runs no `load` before its `update`, so its parameters come
    from the request and anyone can send any id. Use the scoped operations
    (`Db.updateMine`, `Db.deleteMine`) on an owned table, as chapter 6,
    "Rules", shows, rather than checking in `load` and trusting the id.

22. A module is a route module or an old `page`/`msg` module, never both
    (E0043). `alder fix --routes` rewrites the second into the first.
