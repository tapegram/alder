# 6. Routes, responses, forms

## A route module

Every URL of an Alder app is a **route**: one `route` line with an HTTP
method and a path. A route that reads (`GET`) has a `load` and a `view`. A
route that changes something (`POST`, `PUT`, `PATCH`, `DELETE`) has an
`update`, which does the change and says what to send back. A module may
declare any number of routes. This is the whole shape of an Alder app.

```alder
module Todos.Index

import Layout
import Table.Todo

route Index  = GET   "/"                 { filter: Todo.Filter = All } requires user
route Row    = GET   "/rows/{id}"        { id: Todo.TodoId }           requires user
route Toggle = PATCH "/rows/{id}/toggle" { id: Todo.TodoId }           requires user

type Model = { filter: Todo.Filter, todos: List Todo }

Index.load : Index.Params -> Session -> Query Model
Index.load params session = {
  let todos <- Db.mine Todo |> Db.orderBy Todo.created.desc |> Db.all
  Query.succeed { filter: params.filter, todos: List.filter (Todo.matches params.filter) todos }
}

Index.view : Model -> Session -> Html
Index.view model session =
  <Layout.Page title="Todos">
    <ul id="todos">{for todo in model.todos -> Row.view todo session}</ul>
  </Layout.Page>

Row.load : Row.Params -> Session -> Query Todo
Row.load params session = Db.getMine Todo params.id

Row.view : Todo -> Session -> Html
Row.view todo session =
  <li id={"todo-" ++ todo.id}>
    <button click={Toggle { id: todo.id }}>{todo.title}</button>
  </li>

Toggle.update : Toggle.Params -> Session -> Task Response
Toggle.update params session = {
  -- The id comes from the request: `Db.updateMine` changes the todo only when it is the
  -- signed-in user's, and answers 404 otherwise ("Rules", below).
  let _ <- Db.updateMine Todo params.id (\t -> { t | done: not t.done })
  Task.succeed (Response.render (Row { id: params.id }))
}
```

Three routes. `Index` is a page: its view includes the layout. `Row` is a
GET route whose view is one `<li>`; the page's view uses it for every todo,
and `Toggle` uses it to answer a click. `Toggle` is a `PATCH`: it flips one
todo and answers `Response.render (Row { id: … })`, which sends that one
`<li>` to the browser and nothing else.

The to-do app in `examples/todos` is this with a form to add a todo and a
button to delete one; its `Toggle` is a `POST` that answers
`Response.refresh`, the other way to answer ("`Response`", below).

`Db.mine Todo`, `Db.getMine` and `Db.updateMine` are the to-do table's
**scoped** operations: they read and write only the todos of the signed-in
user, because the table marks its `owner` column `@owner` ("Rules", below).

The compiler checks each route module against the shape of its routes.
A GET route has exactly one `load` and one `view`; a write route has
exactly one `update` and neither of those. There is no `Model` slot and no
`Msg`: the model is whatever `load` returns, and a button names a route.

`R.Params` is generated from the route line and must not be written by hand.
`view` receives the session so that per-user presentation (theme, "logged in
as") needs no model field.

## `route`

```alder fragment
route Show   = GET    "/todos/{id}"        { id: Todo.TodoId }
route Index  = GET    "/"                  { filter: Todo.Filter = All, q: Maybe Text }
route Toggle = PATCH  "/todos/{id}/toggle" { id: Todo.TodoId }           requires user
route Create = POST   "/todos"             { } form NewTodo              requires user
route Delete = DELETE "/todos/{id}"        { id: Todo.TodoId }           requires user
route Write  = GET    "/write"             { }                           requires user, role Author
```

A route line has this shape:

```
route NAME = METHOD "path" { params } [form F] [requires …]
```

### Name and method

- **`NAME`** is a constructor. A **route value** is the name applied to its
  parameter record: `Index { filter: Done }`.

  A route value is what `href`, `click` and `submit` take, what
  `Response.redirect` and `Response.render` take, and what a test passes to
  `Test.get`.

- **The method** is `GET`, `POST`, `PUT`, `PATCH` or `DELETE`.

- **`form F`** is for write routes only: a GET route has no body to decode.

### The path

The path is a literal with `{name}` segments.

- Each segment must appear in the parameter record.
- Parameters not in the path ride in the query string, for every method:
  `POST "/like" { postId: PostId }` posts to `/like?postId=…`.
- A segment is letters, digits, `.`, `_`, `~`, `-`, or a whole `{param}`.
  Anything else (`//x`, a space, `?`) is E0029.
- A query parameter whose pair does not parse is a `400` naming the
  parameter, never a parameter quietly at its default. That covers a raw `;`
  (write `%3B`) and a bad `%` escape.

### Parameters

| Rule | Example |
|---|---|
| Allowed types: `Int`, `Text`, `Bool`, a brand of those, a union with no payloads, `Maybe` or `List` of those | `{ id: Todo.TodoId }` |
| A parameter with a default is optional in links and in the URL | `{ filter: Todo.Filter = All }` |
| A `List` parameter repeats its key; `[]` is no key at all | `{ tag: List Text }` with `["a", "b"]` → `?tag=a&tag=b` |

#### Query keys: `as "Key"`

A query parameter's key is its name, unless the field gives another with
`as "Key"` after its type or default. Use it for URLs whose shape is not
yours to choose, such as a capitalised key (`ElectionId`) no field name can
spell:

```alder fragment
route Results = GET "/r/{cfg}/wood/Results" { cfg: Text, electionId: Text = "" as "ElectionId", contestId: Maybe Int as "ContestId" }
```

The code still says `params.electionId` and
`Sites.Wood.Results { cfg: c, electionId: "17" }`. The request decoder reads
`?ElectionId=17`, and every URL of the route (links, redirects, a test's
`Test.get`) writes `ElectionId=17`.

Rules for the key:

- It is letters, digits, `-`, `_`, `.` and `~`.
- Two parameters may not read one key.
- A `{segment}` parameter takes no `as` (E0011).
- The `as` may start the line after the type or default.

> **`as` inside a default.** Inside parentheses, a list's brackets, and the
> braces of a record, a record update or a block, `as` is an ordinary name:
> `f (g as) as "A"` passes a value named `as` to `g`. Anywhere else (a
> `match` arm and an element's `{…}` included), `as` starts the key; wrap a
> value named `as` in parentheses.

**Hand-written GET forms** name their inputs by the key, not the field:

```html
<form method="get" action="/r/c/wood/Results"><input name="ElectionId" /></form>
```

The browser sends the input's `name` as the key, so `name="electionId"`
would set nothing. `alder check` warns about it (W0008) when the form's
`action` and the input's `name` are literal text, and its fix writes the key.

### `R.Params`

`R.Params` is the parameter record type of route `R`. On a write route with
`form F` it has one more field, `form : F`, which the runtime fills in.

### Shared paths and matching

- Two routes may share a path when their methods differ (`GET "/todos/new"`
  and `POST "/todos/new"`).
- They may not share a method and a path. `/x/{id}` and `/x/{slug}` are the
  same pattern, so the second is an error (E0043).
- Patterns that merely overlap are fine, whatever the module order. A
  literal segment is matched before a parameter, left to right, so
  `/todos/new` is served before `/todos/{id}`.
- A request whose path some route declares, but not for its method, is a
  `405` with an `Allow` header.

### `requires`

`requires` names a field of `Session` whose type is `Maybe a`. The route is
served only when that field is `Just`; otherwise the runtime redirects to
the route named in `App.login`.

`requires user, role Author` adds a role: the route is also a 404 for a
signed-in user whose session's `role` is not `Author` ("Rules", below).

Inside the route's members, `session.<field>` has type `a`, not `Maybe a`:

- The route's `Session` is a type of its own (`R.Session` names it).
- It can be passed wherever the app's `Session` is expected (a function in a
  shared module, say), where the field is `Just` its value again.
- The other direction is a type error (E0003): a function of the route that
  takes its `Session` cannot be given the app's, whose field may be
  `Nothing`.

A route value is the only thing `href` accepts, and it must be a GET
route's:

```alder
<a href={Todos.Show.Show { id: todo.id }}>{todo.title}</a>
<a href={Todos.Index.Index { filter: Active }}>Active</a>   -- defaulted fields may be omitted
```

Two route values are `==` when they lead to the same address: a field left
at its default is not in the URL, so `Todos.Index.Index {} == Todos.Index.Index
{ filter: All }`:

```alder eval=True
Todos.Index.Index {} == Todos.Index.Index { filter: All }
```

A GET route may also be asked for by `click`, which fetches it and puts the
answer where the route's root `id` stands ("Delivery", below): its view needs a
root element with an `id`, or nothing is patched. `href` to a
write route, and `submit` to a GET route, are errors (E0045).

## Qualified declarations

Everything that belongs to a route is written `Route.name`: `Index.load`,
`Index.view`, `Toggle.update`, and the type `Index.Params`. The order of the
declarations in the file is free, and anything unqualified (a `type`, a
`form`, a helper, a component) belongs to the module. The signatures are
fixed, and, as everywhere in Alder, written out:

| declaration | on | type |
|---|---|---|
| `R.load` | a GET route | `R.Params -> Session -> Query m` |
| `R.view` | a GET route | `m -> Session -> Html` |
| `R.update` | a write route | `R.Params -> Session -> Task Response` |

`m` is one type: whatever `load` returns, and what `view` takes. The checker
ties the two together. Name it (`type Model = …`), use a table's row type
(`Todo`, as `Row` does above), or write the record type inline. A `view` may
return `File` instead of `Html` ("File pages", below). A write route has no
`view`, because it answers with a `Response`, which names the GET route to
show.

`Session` in the signature is the route's own session, narrowed by its
`requires`. Where a route's `Session` is named from outside its members (a
helper that takes it), it is `R.Session`.

A module with one GET route is a page file without a `Model` slot, which is
most files. A module with a `route` line is a route module. The old form, a
module with a `page` line and its messages, is E0046, and `alder fix --routes`
rewrites it as a route module.

## Names

A route's name is a namespace inside its module: `Row.view`, `Row.load`,
`Row.Params`. Inside the module you write `Row.view todo session`. From
another module you reach the route through its module:

```alder fragment
import Todos.Index

Index.Row.view todo session         -- a member: needs the import, named by its qualifier
Todos.Index.Row { id: todo.id }     -- the route value: no import needed, the full path
```

An imported module is named by its qualifier (chapter 3), so the member is
`Index.Row.view`; the full path in a member's name is E0205. A route value is
global and keeps its full path.

A dotted name is read as the longest module path the program knows, then a
route of that module, then a member. That is why a route's module and its
name often repeat: the to-do list is the module `Todos.Index`, its page is
the route `Index`, and the page's value is **`Todos.Index.Index {}`**. Read
it as module, route, parameters.

The login page is `Auth.Login.Login {}`, and `App` names it with
`login = Auth.Login.Login`. A route that shares its module's last name is the
usual one-route file; nothing shortens it.

Two things would make a name ambiguous, and both are E0044:

- a module `Todos.Row` beside a route `Row` in the module `Todos`: `Todos.Row.view`
  could be either;
- a route named like something already in scope in its module: an import's
  qualifier (`route Todo` beside `import Table.Todo`, because `Todo.view` would
  name the table module), a standard-library module (`route List`), a
  constructor or a table.

```alder error=E0044 "named like the qualifier of `import Table.Todo`"
module Board

import Table.Todo

route Todo = GET "/board" { }

Todo.load : Todo.Params -> Session -> Query Int
Todo.load params session = Query.succeed 0

Todo.view : Int -> Session -> Html
Todo.view n session = <p>{Text.fromInt n}</p>
```

Rename the route, or the module.

## The state rule

A request carries no memory of the last one. The server keeps nothing
between requests for you: a value you need later lives in one of four homes,
and which home decides what survives.

| home | survives | for |
|---|---|---|
| the URL (route parameters) | refresh, back, bookmark, share | filter, sort, page, wizard step |
| the HTML (form values, button payloads) | the next interaction only | drafts in flight, "the thing this button acts on" |
| the session | refresh, not sharing | identity, secrets, theme |
| the database | everything | durable facts |

The rule is: **If you would want it back after a refresh, it goes in the
URL. If it is secret, it goes in the session. Everything else rides in the
HTML or is a row.**

So the to-do list's filter is a parameter of `Index`
(`href={Index { filter: Done }}` is a link, and Back works); a half-typed
title is in the form until the form is sent; who you are is in the session;
the todos are rows. There is no fifth place: no model that the server holds
for a tab, and none that `update` hands to `view`. An `update` changes data,
and a GET route renders data.

## `Response`

A write route's `update` is `Params -> Session -> Task Response`. `Response`
is a type of the standard library, built with functions named after the HTTP
they produce (chapter 9 lists them with the rest):

```alder fragment
Response.redirect : Route -> Response                 -- 303, to the route's URL
Response.render   : Route -> Response                 -- 200, the route's view, as one element
Response.refresh  : Response                          -- 200, the page the browser is on, again
Response.invalid  : Text -> Response                  -- 422, the page, form kept, error shown
Response.toast    : Text -> Response -> Response      -- a toast on any response
Response.success  : Text -> Response -> Response      -- a toast of kind success
Response.warning  : Text -> Response -> Response
Response.error    : Text -> Response -> Response
```

The status code is the signal, as it is for a browser: a `3xx` is followed,
`422` is the same page again with a complaint, `200` is content. A response
can be used with `|>`: `Response.redirect (Index {}) |> Response.toast "Saved"`.

**`redirect r`** goes to the GET route `r`. After creating something, use it,
so that a browser refresh does not send the form twice. It goes to `r` even if
`r` is the page the browser is on.

**`render r`** shows the GET route `r` again, in place: the runtime runs
`r`'s `load`, renders its `view`, and sends that one element, which replaces the
element with the same `id` on the page (so that view needs a root element with
an `id`, or nothing is patched). Nothing else on the page is loaded or
sent.

**`refresh`** loads the page the browser is on again, and sends the regions of
it that changed.

**`invalid t`** refuses a well-formed submission for a reason only `update`
can know (wrong password, name already taken). It is the page the browser is
on again, with the values that were posted, and `t` shown in the form's
`<Errors form />`.

The rule a developer needs, in one sentence: **`render` is for something
already on the page; `refresh` is for things that appear or disappear, or when
more than one place changes.**

The reason is cost. `render` costs the `load` of the route it shows (one read
by key, for a row) and sends one element. `refresh` costs what loading the
page costs (twice: the runtime renders the page before the `update`, to know
what changed) and sends the regions that differ.

A toggle edits a row that is there: `render`. A delete removes one, and the
count in the page's footer changes too: `refresh`.

`invalid` is a successful result, not a failure: the `Task` that produced it
commits, so anything it wrote and any `Session.put` it ran are kept. To
refuse without changing anything, decide before writing (`let taken <-` a
query, then `invalid` or the writes). Only a failed `Task` (`Task.fail`,
`Task.notFound`, a `@unique` violation, a capability error) undoes its
writes and its session changes (chapter 7).

`toast` adds a toast to any of the four ("Notices and in-flight state",
below). The other `Response` builders a web framework has (`status`,
`header`, `json`, `file`, `remove`, `patch`, `stream`) are not in Alder yet;
the type is shaped for them, and a file answer is a GET route that returns a
`File` ("File pages", below).

## `form`

```alder
type Priority = Low | Normal | High

form NewTodo = {
  title: Text.NonEmpty,
  due: Maybe Date,
  priority: Priority = Normal,
}
```

Field types must be one of: `Text`, `Int`, `Float`, `Bool`, `Date`, a brand
of those, a union with no payloads, or `Maybe` of those. A field with a
default is optional in the request. A `Bool` field is a checkbox and is
`False` when absent.

A write route that declares `form NewTodo` gets the decoded form as
`params.form`. Here is a page to add a todo, with its two routes; the pair
share one path:

```alder
module Todos.New

import Layout
import Table.Todo

route New    = GET  "/todos/new" { }              requires user
route Create = POST "/todos/new" { } form NewTodo requires user

form NewTodo = { title: Text.NonEmpty, due: Maybe Date }

New.load : New.Params -> Session -> Query Unit
New.load params session = Query.succeed ()

New.view : Unit -> Session -> Html
New.view model session =
  <Layout.Page title="New todo">
    <form submit={Create}>
      <al-input field="title" label="Title" />
      <al-input field="due" label="Due" />
      <Errors form />
      <al-button type="submit">Add</al-button>
    </form>
  </Layout.Page>

Create.update : Create.Params -> Session -> Task Response
Create.update p session = {
  let same <- Db.mine Todo |> Db.where (Todo.title.eq p.form.title) |> Db.count
  if same > 0 then
    Task.succeed (Response.invalid "You have a todo with that title.")
  else {
    let _ <- Db.insertMine Todo { title: p.form.title, due: p.form.due }
    Task.succeed (Response.redirect (Todos.Index.Index {}) |> Response.toast "Added")
  }
}
```

There are two ways for this form to fail, and they look the same to the
user.

When a submitted form fails to decode (missing field, failed refinement,
wrong type), the runtime does **not** call `update`. It answers `422` with
the page the browser is on, rendered again with the submitted values and
errors attached to that form; `<Input>` and `<Errors>` show them. The program
never sees an invalid form.

Only the form that posted shows them: on a page with one form per row, the
other rows keep their own values.

A field's errors go to its `<Errors field="…" />`; the errors of a field
without one are listed in the form's `<Errors form />`, after its name ("due
must be a date (YYYY-MM-DD)"). A form with neither for a field gets warning
W0003.

When the form decodes and `update` still refuses it, `update` answers
`Response.invalid "…"`: the same `422`, the same page, with the text in
`<Errors form />`. A form that can be refused this way and has no
`<Errors form />` gets warning W0005, because the user would see the page come
back unchanged and not know why.

## App-level routes

Actions that belong to no page (log out, switch theme) are write routes
declared in the `App` module. Any page can send them, as
`click={App.Logout}`, and a route of `App` is like any other route except
that it has no page of its own.

It answers `redirect r`, to go to the route `r`, or `refresh`, which reloads
the page the browser is on, because that is where the request came from (the
page's address is sent with every write, "Delivery", below).

```alder fragment
-- in App
route Logout = POST "/_/Logout" { }

Logout.update : Logout.Params -> Session -> Task Response
Logout.update p session = {
  let _ <- Session.put (\s -> { s | user: Nothing })
  Task.succeed (Response.redirect (Auth.Login.Login {}))
}

-- in any page, or any component
<button click={App.Logout}>Log out</button>
```

An `App` route's value can be written in any markup, so a layout component
typed `Html` can contain `click={App.Logout}`.

### Example: a theme switch

The to-do app has one app-level route, `Logout`. A switch between a light
and a dark theme is the other usual one, and it uses every piece an app-level
route can involve: a type for the state, a session field to keep it in, the
route and its `update`, and a button that every page shows. Here is how to
add it.

> **Not in the example app.** The example app leaves it out, so that its
> `App`, `Layout` and login page are a skeleton a new app can copy and check
> with nothing to write except its own home page.

The state is a union, in a module of its own, `app/Theme.alder`:

```alder
module Theme

type Theme = Light | Dark

flip : Theme -> Theme
flip theme = match theme {
  Light -> Dark,
  Dark -> Light,
}

test "flip alternates" = flip (flip Light) == Light
```

`App` imports it, gives the session a field for it, and declares the route.
This block is an excerpt, and the verifier does not check it: an app declares
its `session` once, and the example's `App` (in "The `App` module", below)
already does. The new lines are the `import`, the `theme` field, the route and
its `update`:

```alder fragment
-- in App
import Theme

session { user: Maybe User.UserId, theme: Theme = Light }

route SwitchTheme = POST "/_/SwitchTheme" { }

SwitchTheme.update : SwitchTheme.Params -> Session -> Task Response
SwitchTheme.update p session = {
  let _ <- Session.put (\s -> { s | theme: Theme.flip s.theme })
  Task.succeed Response.refresh
}
```

`theme: Theme = Light` has a default, so a visitor's first request, and a
cookie from before the field existed, get `Light` ("The session cookie",
below). `Response.refresh` loads the page the button was on again, with the new
session.

A `refresh` or a `redirect` under Datastar updates only the page's `<body>`: a
class on `<html>`, such as `dark` here, shows after the next full page load, and
at once only without JavaScript (the `303` loads the whole page). That is a
known gap ([#189](https://github.com/tapegram/alder/issues/189)); the example
is still the way to keep the choice in the session.

The layout cannot read the session; it takes the theme as a prop, sets a
class on `<html>` for the stylesheet, and shows the button. Also an
excerpt, of `app/Layout.alder` (the examples write their layouts as
`Html`, like any markup):

```alder fragment
-- in Layout
import Theme

Page : { title: Text, theme: Theme, children: List Html } -> Html
Page props =
  <html class={if props.theme == Dark then "dark" else ""}>
    ...
    <body>
      <nav>
        <button click={App.SwitchTheme}>{if props.theme == Dark then "Light mode" else "Dark mode"}</button>
      </nav>
      <main><al-stack gap="lg">{props.children}</al-stack></main>
    </body>
  </html>
```

One thing to check first. A Layout from `alder new` carries a small theme
script in its `<head>`: on every full page load it reads the visitor's
choice from the browser (`localStorage`) or their operating system's
preference, and sets or removes the `dark` class on `<html>` itself.

That would overwrite the class the server just rendered from the session, and
your button would seem to do nothing whenever the two disagree. So a Layout
whose theme comes from the session drops the theme script: the session owns
the class.

> **The other way.** A button in the browser that remembers the choice in
> `localStorage` keeps the script; it is described under "Light and dark" in
> Appendix C.

The `dark` class does something without any rules of yours, because
alder-ui follows `html.dark` itself: its own colours switch to the dark
values (Appendix C, "Light and dark"). Rules in `static/app.css` (the to-do
app's, served at `/static/app.css`) are only for your own colours, the ones
alder-ui does not draw. Add them after the rules they override:

```css
html.dark body {
  color: #e6edf3;
  background: #0d1117;
}

html.dark a {
  color: #4493f8;
}
```

Every call of `Layout.Page` then passes a theme: a route passes its
session's, and `App.error` and `App.notFound`, which have no session,
pass `Light`:

```alder fragment
<Layout.Page title="Todos" theme={session.theme}>
<Layout.Page title="Error" theme={Light}>
```

That is the cost of the feature, and why the example does without it:
every route's `view` passes the theme on, and a test that builds a session
by hand names the field too, `{ user: me, theme: Light }` (chapter 10,
"Testing effects"), and the test's module imports `Theme` for `Light`.

The reason is that a record literal lists every field, defaults or not:
defaults apply at the boundaries, the cookie, a route parameter or an insert
(chapter 12, "No defaulted record literals").

A literal that leaves `theme` out is refused, for App's `Session` and a
`requires` route's alike:

```
error[E0003]: record literal for `Session` is missing `theme` (a default applies only to an insert, the session cookie, a page or message parameter, or a form, not to a literal)
```

## File pages

Every route so far answers with a web page, or a piece of one. Some
addresses should answer with something else: a spreadsheet to download, a
zip, a JSON document, a copy of another site's page kept exactly as it was. A
**file page** is a GET route whose `view` returns a `File` instead of `Html`.
It has an address, a `load` and parameters like any GET route, and the router
sends the file's bytes as the whole answer.

### Declaring a file page

Two small ideas first, because a file has to say both:

- A **content type** is the label that tells the browser what the bytes are
  (`text/csv` is a spreadsheet-like table of text, `application/zip` an
  archive, `text/html` a web page), and the browser chooses what to do with
  the answer by it.
- A **status** is the three-digit number that says how the request went
  (`200` it worked, `404` there is nothing here, `503` try again later).

A `File` carries both, plus its body and any extra headers.

The compiler decides a route is a file page by the type of its `view`. Here
is one that downloads the signed-in user's todos as a CSV file:

```alder
module Todos.Export

import Table.Todo

route Export = GET "/todos.csv" { } requires user

type Model = { todos: List Todo }

Export.load : Export.Params -> Session -> Query Model
Export.load params session = {
  let todos <-
    Db.mine Todo
      |> Db.orderBy Todo.created.desc
      |> Db.all
  Query.succeed { todos: todos }
}

row : Todo -> Text
row todo = Text.join "," [todo.title, if todo.done then "done" else "open"]

Export.view : Model -> Session -> File
Export.view model session =
  File.text { contentType: "text/csv", body: Text.join "\n" (List.map row model.todos) }
    |> File.withHeader { name: "Content-Disposition", value: "attachment; filename=todos.csv" }
```

`File.text` answers `200` with the text as the body, `File.bytes` does the
same for `Bytes` (chapter 7, "A capability returning bytes"), and
`File.json` takes a `Json.Value` built with the `Json` module (chapter 9,
"Json"), encodes it and labels it `application/json`; text that is already
JSON goes through `File.text` with that content type.

The `Content-Disposition` header, set with `File.withHeader`, asks the browser
to save the answer as `todos.csv` instead of showing it.

A route's path is made of whole segments: `"/todos.csv"` is one segment,
and so is `"/reports/{id}/report.csv"` (a parameter, then a fixed segment).
A parameter inside a segment, such as `"/reports/{id}.csv"`, is not a valid
path, so put the parameter in a segment of its own.

`File.html` is the way to answer a web page, byte for byte. Markup in a
`view` has one root element and the renderer writes no doctype, so to
reproduce a page that starts with `<!doctype html>` (a copy of another
site's page, say) put the doctype in a `Html.fragment`, which lays its
nodes side by side with no wrapper:

```alder
module Todos.Plain

route Plain = GET "/plain" { }

type Model = { }

Plain.load : Plain.Params -> Session -> Query Model
Plain.load params session = Query.succeed { }

Plain.view : Model -> Session -> File
Plain.view model session =
  File.html (Html.fragment [Html.unsafe "<!doctype html>\n", <html><body><p id="n">42</p></body></html>])
```

The answer is exactly `<!doctype html>`, a newline, then the `<html>`
element, with the content type `text/html; charset=utf-8`. The text
inside the markup is still escaped; only the `Html.unsafe` piece is not
(chapter 5).

`File.status`, `File.contentType` and `File.body` read a file back; they
are for tests and helpers, since the router is the one that sends it.

### Status and headers

`File.withStatus` changes the answer's status. It takes a number from 200
to 599: a `1xx` status is an interim "keep going" message that the Go
server cannot send as a final answer, so it is refused. A literal outside
the range is E0003, and a number computed at run time that falls outside
it is answered with the app's `500` page.

```alder
module Todos.Busy

route Busy = GET "/busy" { }

type Model = { }

Busy.load : Busy.Params -> Session -> Query Model
Busy.load params session = Query.succeed { }

Busy.view : Model -> Session -> File
Busy.view model session =
  File.text { contentType: "text/plain", body: "busy, try again" }
    |> File.withStatus 503
    |> File.withHeader { name: "Retry-After", value: "20" }
```

A header **name** is one of `Content-Disposition`, `Cache-Control`,
`ETag`, `Last-Modified`, `Retry-After`, `Content-Language`, or any name
that starts with `X-`, in any case (`etag` and `ETag` are the same). Set
twice, a named header keeps the last value; `X-` names may repeat.

The router owns every other header, because each is part of something the
router already does:

- `Content-Type` comes from the file.
- `Set-Cookie` comes from the session.
- `Location` comes from a redirect.
- `Content-Length` and the connection headers come from the server.

A literal name outside the list is E0035:

```alder error=E0035 "`Set-Cookie` cannot be set from a file route"
module Todos.Cookie

route Cookie = GET "/cookie" { }

type Model = { }

Cookie.load : Cookie.Params -> Session -> Query Model
Cookie.load params session = Query.succeed { }

Cookie.view : Model -> Session -> File
Cookie.view model session =
  File.text { contentType: "text/plain", body: "hi" }
    |> File.withHeader { name: "Set-Cookie", value: "a=b" }
```

A name that is not a literal (one built from a parameter) cannot be checked
before the page runs; at run time a refused header is left out and the
server logs a `warn` line naming it.

### Links and the boost

A link to a file page is a download or a document of its own, not a page
to patch into the one on screen, so the compiler marks it
`data-alder-reload` itself (see "Links and navigation", below):
`<a href={Todos.Export.Export {}}>Download</a>` needs no attribute. A redirect
into a file page navigates the browser to it, as a real load, and the
router answers a boosted request for a file page with a real navigation too.

### What stays HTML

A file page changes only the answer of a route that is shown. The answers
that belong to the app stay the app's HTML pages: a `404` from
`Query.notFound` or a parameter that does not decode (`App.notFound`), a
failed `Task` or `load` (`App.error`), the maintenance `503`, and the
redirect to the login page when the route `requires` something the session
lacks.

A file page's `load` that ends in `Query.redirect r` is a plain `303` to `r`,
as its `Query.notFound` is the app's `404`.

> **Its own `503`.** A file page that answers its own `503` (above) is
> different: that `503` is the page's own file.

A file page is not a document, so nothing on it can post a form or send a
click; put the button on an HTML route that links to the file. A test reads a
file page with `Test.fetch` (chapter 10, "`Test.fetch`: requesting a file");
`Test.get` and `Test.open` on one are checker errors.

## Rules

Most apps with a login have two rules about who may do what. A row
belongs to a user, and only that user may see or change it: a todo, a
draft post. And some routes are only for users with a role: an author's
desk, an admin page. Alder lets you **declare** both rules, and then the
shortest thing to write is the correct one: no check to remember, so no
check to forget.

### Ownership: `@owner`

A table says which of its columns names the row's owner. In the to-do
app (`examples/todos/app/Table/Todo.alder`):

```alder fragment
table Todo {
  id: TodoId @id,
  title: Text.NonEmpty,
  done: Bool = False,
  due: Maybe Date,
  owner: User.UserId @ref User @owner,
  created: Time @created,
}
```

`owner: User.UserId @ref User` was already there: each todo refers to a
user. `@owner` adds one thing: "this column is whose row it is". The
rules for it:

- A table has at most one `@owner` column.
- It must be a `@ref` to the user table, of the type of the session's
  `user` field without its `Maybe` (`session { user: Maybe User.UserId }`
  means `User.UserId`), so the column and the session can be compared.
- The app's session must have a `user` field.

Anything else is E0024 (the exact texts are in chapter 11). `@owner`
changes nothing in the database: the column and its
reference are what they were, and no migration is needed (chapter 8).

### Reading and writing your own rows

A table with an `@owner` column has five **scoped** operations in `Db`
(chapter 9). Each one does what its unscoped twin does, but only to rows
whose owner is the signed-in user:

| Operation | Type | What it does |
|---|---|---|
| `Db.mine Todo` | `Select Todo` | The rows whose `owner` is the signed-in user |
| `Db.getMine Todo id` | `Query Todo` | The row with that id, when it is the user's; otherwise not found |
| `Db.updateMine Todo id f` | `Task Unit` | Changes the row with `f`, when it is the user's; otherwise not found |
| `Db.deleteMine Todo id` | `Task Unit` | Deletes the row, when it is the user's; otherwise not found |
| `Db.insertMine Todo { … }` | `Task TodoId` | Inserts a row whose owner is the user |

`Db.mine` is a `Select`, like `Db.from Todo`, so `Db.where`, `Db.orderBy`,
`Db.limit`, `Db.all`, `Db.count` and the rest apply to it.

The to-do list uses four of them
(`examples/todos/app/Todos/Index.alder`):

```alder fragment
Index.load params session = {
  let todos <-
    Db.mine Todo
      |> Db.orderBy Todo.created.desc
      |> Db.all
  Query.succeed { filter: params.filter, todos: List.filter (Todo.matches params.filter) todos }
}

Submit.update p session = {
  let _ <- Db.insertMine Todo { title: p.form.title, due: p.form.due }
  Task.succeed (Response.redirect (Index { filter: p.filter }))
}

Toggle.update p session = {
  let _ <- Db.updateMine Todo p.id (\t -> { t | done: not t.done })
  Task.succeed Response.refresh
}

Delete.update p session = {
  let _ <- Db.deleteMine Todo p.id
  Task.succeed Response.refresh
}
```

and the todo's own page, `Todos.Show`, reads its todo with the fifth:

```alder fragment
Show.load params session = {
  let todo <- Db.getMine Todo params.id
  Query.succeed { todo: todo }
}
```

Why a write route needs them as much as a `load` does: `load` decides what
the page shows, but a write route carries its own parameters. `Toggle { id }`
puts the id in the request's URL (`/_/Toggle?id=…`), and anyone signed in can
send that request with an id that is not theirs. No `load` runs before a write
route to have checked it. With `Db.updateMine` the id is only ever one of the
user's own todos.

Two details:

**`Db.insertMine` takes its row as a record literal**, written in the
call, with every field but the owner: the owner is the session's user.
Naming it is an error:

```alder error=E0003 "`owner` is set by `Db.insertMine`"
module Quick

import Table.Todo

route Add = POST "/quick" { title: Text.NonEmpty } requires user

Add.update : Add.Params -> Session -> Task Response
Add.update p session = {
  let _ <- Db.insertMine Todo { title: p.title, due: Nothing, owner: session.user }
  Task.succeed Response.refresh
}
```

and so is passing a row built elsewhere: "`Db.insertMine` takes its row
as a record literal: write the record literal in the `Db.insertMine`
call (its row is `Insert Todo` without `owner`)". Fields with a default
(`done`) may be left out, as with `Db.insert`.

**`Db.updateMine` never changes the owner.** If `f` returns the row with
another `owner`, the rest of its changes are written and the owner stays
as it was: a scoped write cannot give a row away. A page that transfers
rows writes that with `Db.update` ("When to use the unscoped forms",
below).

### What a stranger gets

A row that exists but belongs to someone else is answered exactly as a
row that does not exist: **not found**.

- In `load`, `Db.getMine` of another user's id is `Query.notFound`: the
  page is a 404, `App.notFound`.
- In `update`, `Db.updateMine` or `Db.deleteMine` of another user's id
  is `Task.notFound`. The whole `Task` fails, so everything it did before
  is rolled back too, writes and `Session.put` alike (chapter 7), and the
  answer is a 404.
- `Db.mine` simply has no such row in it.

There is no arm to write for the stranger's case and so no arm to get
wrong, and a stranger cannot tell someone else's todo from an id that
was never used: the answer gives nothing away. A page that wants to
answer differently (a 403 page, say) uses the unscoped operation and
writes its own check.

### Where they may be used

A scoped operation needs a signed-in user, so it is allowed only where
the compiler knows there is one: in the members of a route that `requires
user`, its `load`, `view` or `update`, and in the helpers they call, such as

```alder fragment
recent : Select Todo
recent = Db.mine Todo |> Db.orderBy Todo.created.desc |> Db.limit 5
```

Anywhere else, it is E0026: in a route without `requires user`,

```alder error=E0026 "`Db.mine` needs a route that `requires user`: `Index` does not"
module Public

import Table.Todo

route Index = GET "/public" { }

type Model = { open: Int }

Index.load : Index.Params -> Session -> Query Model
Index.load params session = {
  let n <- Db.mine Todo |> Db.count
  Query.succeed { open: n }
}

Index.view : Model -> Session -> Html
Index.view model session = <p>{Text.fromInt model.open}</p>
```

The rule follows a helper: a value that uses a scoped operation is checked
where it is used, not where it is written, so a route that does not `requires
user` and calls `recent` is E0026 at that call ("`recent` uses `Db.mine`,
which needs a route that `requires user`: `Index` does not").

A `fake` (chapter 10) has no session either: "`Db.mine` cannot be used in a
fake: a fake has no session" (an excerpt: the message begins with where the
use is, such as "in `fake Counter`'s `count`:"). Tests may use them (below).

A table without `@owner` has nothing to scope by (E0025):

```alder error=E0025 "`User` has no `@owner` column, so `Db.mine User` has nothing to scope by"
module Account

import Table.User

route Account = GET "/account" { } requires user

type Model = { users: Int }

Account.load : Account.Params -> Session -> Query Model
Account.load params session = {
  let n <- Db.mine User |> Db.count
  Query.succeed { users: n }
}

Account.view : Model -> Session -> Html
Account.view model session = <p>{Text.fromInt model.users}</p>
```

### Whose session

A scoped operation acts for the user of the **running** request or
test, the session the runtime holds while it runs the effect. It does
not read the `session` argument of `load` or `update`. In a served
request the two are the same session, so the difference never shows.
It shows in two places:

- **A test** that calls `update` (or `load`) itself, with a session
  record it wrote, does not choose the user that way: the scoped
  operations act for the test's own session, which `Test.login` sets
  (chapter 10, "Tests that use the rules").

  A test that never logged in has no user, and a scoped operation fails with
  `Db.mine Todo: no signed-in user` (or `Db.getMine Todo: …`, and so on).
  It never quietly returns nothing.

- **`Session.put`** in the middle of an `update` changes the user for
  every scoped operation after it in the same `Task`: they act for the
  session as it now is.

### Roles: `requires user, role R`

A role lives in the session, beside the user. The blog
(`examples/blog`) has two, declared in `User`:

```alder fragment
type Role = Reader | Author
```

The user table has a column `role: Role = Reader`, and `App`'s session
has a field for it:

```alder fragment
session { user: Maybe User.UserId, role: User.Role = Reader }
```

The login page copies the role from the user's row when it signs them
in (`examples/blog/app/Auth/Login.alder`):

```alder fragment
Just id -> {
  let found <- Db.get User id
  let role = Maybe.withDefault Reader (Maybe.map (\u -> u.role) found)
  let _ <- Session.put (\s -> { s | user: Just id, role: role })
  Task.succeed (Response.redirect (Posts.Index.Index {}))
},
```

and logging out puts it back: `{ s | user: Nothing, role: Reader }`.

Adding `role` to the session changes the tests. A session record a test
writes for a route's `load` or `update` is a record literal, and names every
field: `{ user: author }` becomes `{ user: author, role: Author }`, or
E0003 says `role` is missing (a default applies at the boundaries, not
to a literal, as the theme switch earlier in this chapter shows).

`Test.login`'s record is a boundary, the cookie's value, so
`Test.login { user: Just me }` may leave `role` out and gets its default,
`Reader`.

A route for authors only says so on its `route` line:

```alder fragment
route Write  = GET "/write"        { } requires user, role Author
route Drafts = GET "/write/drafts" { } requires user, role Author | Admin
```

- `requires user` comes first: a visitor is sent to log in, as always.
- Then the role: a signed-in user whose session's `role` is not one of
  those named gets a 404, the same `App.notFound` page as a missing row,
  so a Reader cannot even tell the route exists. This is checked before
  the route's `load` or `update` runs, so neither needs a check of its own.
- `role Author | Admin` lets either in.
- Inside the route's members, `session.role` has the union's type, as
  everywhere.
- The router checks `requires` and the role, whatever the method. A test of
  them goes through the browser (`Test.open`, `Test.click`, chapter 10); a
  direct `load` or `update` call is not checked.

The rules for the clause (E0027): `role` needs `user` first (`requires
role Author` alone is an error); the session must have a field `role`
(else: "this session has no `role` field"); it must be a union, not
`Maybe`; and each name must be one of its constructors. In the to-do
app, whose roles are `Member` and `Admin`:

```alder error=E0027 "`Author` is not a constructor of this session's `role` type"
module Reports

route Reports = GET "/reports" { } requires user, role Author

type Model = { }

Reports.load : Reports.Params -> Session -> Query Model
Reports.load params session = Query.succeed { }

Reports.view : Model -> Session -> Html
Reports.view model session = <p>reports</p>
```

**Generated routes say the same.** `alder new route … --requires user`
writes the clause, and `--role R` adds `, role R` (and implies
`--requires user`), each with its test (chapter 15).

Every route of `alder new admin`'s admin `requires user, role Admin`: so it
needs a session `role` with an `Admin` constructor; and when the user table
holds the role, each of its routes also re-reads the user's row (in `load`,
and at the start of each write route), the idiom below, so a demoted Admin is
turned away at once (chapter 15, "The dashboard and the layout").

**A role is read at login.** The check reads the session cookie and
nothing else, so it costs no query.

A role in the cookie lasts until logout, or the session's 30-day lifetime from
login (visiting does not renew the cookie: "The session cookie", below). Until
then an author demoted to Reader still passes `requires user, role Author`,
and a Reader promoted to Author must log in again.

A route where a demotion must apply at once also reads the user's row, the
**re-read idiom**, as the blog's Write routes do with helpers in `User`
(`examples/blog/app/Table/User.alder`):

```alder fragment
isAuthor : UserId -> Query Bool
isAuthor id = {
  let me <- Db.get User id
  Query.succeed (Maybe.map (\u -> u.role) me == Just Author)
}

authorOnly : UserId -> Query Unit
authorOnly id = {
  let author <- isAuthor id
  if author then Query.succeed () else Query.notFound
}
```

A GET route asks in its `load`, as the blog's `Write.Edit` does,

```alder fragment
Edit.load params session = {
  let author <- User.isAuthor session.user
  if author then {
    let post <- Db.getMine Post params.id
    Query.succeed { post: post }
  } else Query.notFound
}
```

and a write route begins with `authorOnly`, because no `load` runs before it:

```alder fragment
Save.update p session = {
  let _ <- User.authorOnly session.user
  let _ <- Db.updateMine Post p.id (\post -> { post | title: p.form.title, body: p.form.body })
  Task.succeed (Response.refresh |> Response.toast "Saved")
}
```

That is the blog's test "a demoted author is turned away at once"
(chapter 14). The role clause stays: it turns away a Reader promoted
since their login, and keeps a Reader's request from reaching a `load`
or an `update` at all.

### When to use the unscoped forms

`Db.from`, `Db.get`, `Db.update`, `Db.delete` and `Db.insert` are still
there, and still right in these places:

- **Rows that are anyone's to read.** The blog's front page lists
  everyone's published posts with `Db.from Post`.
- **Routes for a role that acts on everyone's rows**, such as an admin
  route `requires user, role Admin` that deletes any post. The role is
  the rule there, and the route says so on its `route` line.
- **Giving a row away**, which `Db.updateMine` never does.

On an owned table outside those cases, an unscoped operation means the
check is yours to write again, and a test is yours to write for it.

A row a page has already shown is not one of the cases either: write the
scoped form; it costs nothing.

A write route runs no `load` before it, so it
has not read the row: it takes the id from its path (`Todos.Show`'s `Save` and
`Delete` are `POST "/todos/{id}/_/Save"` and `POST "/todos/{id}/_/Delete"`),
and the scoped write is what makes that id safe.

`Todos.Show` writes
`Db.updateMine Todo p.id …` and `Db.deleteMine Todo p.id`, and so does the
blog's `Write.Edit`; the test that sends a stranger's id and gets a 404
(`Todos.Show`'s "saving or deleting another user's todo is not found") is what
pins it.

### The rules and a replicated database

An app may one day run on a database with read replicas near its users
and one primary that takes the writes, as Turso offers. The rules are
built to fit:

- **Scoped reads are predicates.** `Db.mine` and `Db.getMine` are plain
  `SELECT`s with `"owner" = ?` in the `WHERE` clause, so any replica can
  answer them.
- **Role checks never read the database.** The role is in the signed
  cookie, so a role page costs no query anywhere.
- **Scoped writes carry the predicate.** `Db.deleteMine` is one
  `DELETE … WHERE "id" = ? AND "owner" = ?`. `Db.updateMine` is a
  predicated read then a predicated write (`UPDATE … WHERE "id" = ? AND
  "owner" = ?`), both in the `Task`'s transaction, on the primary. Each
  counts the rows it changed. Chapter 13 shows the SQL.

  A hand-written check that reads on a replica and writes on the primary can
  approve a write the primary would refuse, when the replica is behind or
  another request changed the row in between. With the predicate in the
  write, the primary decides ownership where it writes.

  > **A gap the examples do not have.** A `Db.getMine` in `load` and then a
  > plain write of that row, in a later request, would have that gap if the
  > row changed owner between the two; the examples' write routes carry the
  > predicate themselves, so they have none.

### Partitions

`@owner` says that a row belongs to one user. Many apps have a second
kind of owner: a row belongs to an **org** (a team, a workspace, a
company), every member of the org may see it, and a user works in one
org at a time.

The rule is the same shape as ownership, with another session field in place
of `user`, so Alder declares it the same way. This section follows
`examples/workspace` (chapter 14), whose orgs have projects, and whose users
belong to orgs through a `Membership` table.

#### A key in the session

The org a user works in is a session field, a
**key**, beside the user (`examples/workspace/app/App.alder`):

```alder fragment
session { user: Maybe User.UserId, org: Maybe Org.OrgId via Membership, role: User.Role = Member }
```

It is a `Maybe` of a table's id, like `user`: a new session has no org
yet. `via Membership` is the rule for setting it, below.

#### `@partition org`

A table says which of its columns holds the key
(`examples/workspace/app/Table/Project.alder`):

```alder fragment
table Project {
  id: ProjectId @id,
  name: Text.NonEmpty,
  org: Org.OrgId @ref Org @partition org,
  created: Time @created,
}
```

`@partition k` marks one `@ref` column whose type is the session field
`k` without its `Maybe`: `org: Maybe Org.OrgId` in the session means
`Org.OrgId` here. `@owner` is exactly `@partition user`, kept as the
short way to write it.

The rules, E0032 when broken:

- The session has a field `k` of type `Maybe` of the column's type
  ("`@partition org` needs a session field `org` of type
  `Maybe Org.OrgId`").
- The column is a `@ref` ("`@partition org` must mark a `@ref` column; `name`
  is `Text` without `@ref`").
- A table has one partition column ("a table has one partition column:
  `Board` marks `org` and `owner`").

Like `@owner`, it changes nothing in the database by itself: chapter 8's
`silo` is what gives each org a database of its own.

#### The scoped operations, for any key

A partitioned table has five
generic scoped operations, which do for its key what the `mine` family
does for the user (chapter 9 has their types):

| Operation | What it does |
|---|---|
| `Db.scoped Project` | the rows of the session's org: a `Select`, like `Db.from Project` |
| `Db.getScoped Project id` | the row with that id when it is the org's; otherwise not found |
| `Db.updateScoped Project id f` | changes the row when it is the org's (never its `org`); otherwise not found |
| `Db.deleteScoped Project id` | deletes the row when it is the org's; otherwise not found |
| `Db.insertScoped Project { … }` | inserts a row whose `org` is the session's; the row is a record literal without `org` |

The `mine` family is these five on a table partitioned by `user`, and
the same code: `Db.mine Todo` and `Db.scoped Todo` are one operation.

On a table partitioned by another key `Db.mine` is refused, naming the
fix (E0025): "`Project` is partitioned by `org`, not by the session's
`user`: `Db.mine` is for `@owner` tables; write `Db.scoped Project`".

What a stranger gets is what it is for `@owner`: a row of another org is
answered as a row that does not exist, not found, so there is no arm to
write for it and nothing to give away.

#### `requires user, org`

A scoped operation on `Project` needs a
session whose `org` is set, so it is allowed only in the members of a route
that `requires org` (E0026 otherwise: "`Db.scoped Project` needs a route
that `requires org`: `Index` does not"). The workspace's projects page and
the write routes beside it (`examples/workspace/app/Projects/Index.alder`):

```alder fragment
route Index  = GET  "/projects"          { }                       requires user, org
route Create = POST "/projects/_/Create" { } form NewProject       requires user, org
route Remove = POST "/projects/_/Remove" { id: Project.ProjectId } requires user, org
```

A request is answered in this order:

1. No `user`: the visitor is sent to log in, as always.
2. No `org`: the user is sent to the page `App` names with `choose org =
   …`, below (a key without one answers a 404: there is nothing to pick).
3. Otherwise the route runs, and inside its members `session.org` is an
   `Org.OrgId`, not a `Maybe`: the route is **narrowed** to it, as
   `requires user` narrows `session.user`.

A key with `via` must come after `user` (E0027: "`org` needs `user`
first: `org` is `via Membership`, a membership of the signed-in user;
write `requires user, org`"): a membership is a user's, so a session with
an org but no user cannot happen. The route can then read and write its
org's rows with the scoped operations, as `Projects.Index` does
(`Membership.memberOnly` is the re-read idiom, below):

```alder fragment
Create.update p session = {
  let _ <- Membership.memberOnly session.user session.org
  let id <- Db.insertScoped Project { name: p.form.name }
  Task.succeed (Response.redirect (Projects.Show.Show { id: id }))
}

Remove.update p session = {
  let _ <- Membership.memberOnly session.user session.org
  let _ <- Db.deleteScoped Project p.id
  Task.succeed Response.refresh
}
```

`Remove { id }` carries an id in its request, which anyone signed in can
send with another org's project's id; `Db.deleteScoped` looks for it in
the session's org only, finds nothing, and the `Task` fails with not
found (the workspace's test "another org's project cannot be removed").

#### `via`: who may set the key

The session's `org` is set as any field
is, with `Session.put`. `via Membership` adds one rule: a `Session.put`
that sets `org` to `Just v` fails, the whole `Task` with it, unless the
`Membership` table has a row whose user is the session's user and whose
org is `v`. The workspace's org picker (`examples/workspace/app/Orgs/Index.alder`)
sets it from a button:

```alder fragment
Open.update p session = {
  let _ <- Session.put (\s -> { s | org: Just p.org })
  Task.succeed (Response.redirect (Projects.Index.Index {}))
}
```

The page shows a button only for the user's own orgs, but `Open { org }`
is a URL like any other, and a crafted one can name any org. The check
does not trust the page: for an org the user is no member of, the
`Session.put` fails with "Session.put org: no Membership for this user
and org", the `Task` is rolled back, and the session keeps the org it
had (`Orgs.Index "a non-member's switch is refused"`). The details:

- The check runs **inside the `Task`'s transaction**, when the
  `Session.put` runs. A `Membership` row inserted earlier in the same
  `Task` counts, which is how creating an org, adding its creator as a
  member and opening it is one `Task` (`Orgs.Index`'s `Create`); a row
  deleted earlier in the same `Task` counts as gone.
- Setting it to `Nothing` is always allowed: leaving an org needs no
  membership. Logging out (a `Session.put` whose `user` becomes
  `Nothing`) clears every `via` key with it.
- The `via` table must have exactly one `@ref` to the user table and one
  to the key's table (E0032: "`via Seat`: `Seat` needs exactly one `@ref
  User` column and one `@ref Org` column; it has 0 and 1").
- The session must have a `user` ("`org` is `via Membership`, which needs a
  session field `user`: a membership is a user's").
- For now, a session has one `via` field ("`other` is `via Membership`: one
  `via` field per session (for now)").
- A key without `via` (a `region` chosen from a fixed list, say) is set
  freely; the pages that use it still `requires region`.

#### `choose`: where a session without the key goes

`App` names the page that picks the key (`examples/workspace/app/App.alder`):

```alder fragment
choose org = Orgs.Index.Index
```

A route that `requires user, org` sends a session without an org there.

The rules:

- A `choose` is required as soon as a route requires a `via` key (E0032:
  "`org` is `via Membership` and `Projects` `requires org`: `App` needs
  `choose org = Page`, the page a request without `org` is sent to (it picks
  one)").
- The route it names must not require the key itself, or a request would be
  sent back to it for ever ("`choose org = Picker`: `Picker` `requires org`
  itself, so a request without `org` would be sent back to it").
- The user is chosen by `login`, never by `choose` ("`choose user`: the
  signed-in user is chosen by `login`; `choose` names the page for another
  key").

The workspace's login puts the first org the user joined in the session as it
signs them in, through the same rule (one `Db.first` and one `Session.put`);
a user in no org gets none, and the projects page sends them to
`Orgs.Index.Index` to make one.

#### The re-read idiom, for a membership

Like a role, the key is checked
when it is put, and the cookie keeps it: it is **not** checked again on
every request. A member removed from the org keeps the org in their
cookie until they log out, pick another, or the cookie's 30 days end.

A route that must notice a removal at once reads the membership again, the
re-read idiom of "Roles" above, with helpers in `Membership`
(`examples/workspace/app/Table/Membership.alder`):

```alder fragment
isMember : User.UserId -> Org.OrgId -> Query Bool
isMember user org = {
  let n <- Db.from Membership |> Db.where (Membership.user.eq user) |> Db.where (Membership.org.eq org) |> Db.count
  Query.succeed (n > 0)
}

memberOnly : User.UserId -> Org.OrgId -> Query Unit
memberOnly user org = {
  let member <- isMember user org
  if member then Query.succeed () else Query.notFound
}
```

A GET route asks in its `load`, as each of the workspace's org pages does,

```alder fragment
Index.load params session = {
  let member <- Membership.isMember session.user session.org
  if member then {
    let org <- Db.get Org session.org
    let projects <- Db.scoped Project |> Db.orderBy Project.name.asc |> Db.all
    Query.succeed { org: Maybe.withDefault "" (Maybe.map (\o -> o.name) org), projects: projects }
  } else Query.notFound
}
```

and a write route begins with `memberOnly`, because no `load` runs before
it (`Create.update` and `Remove.update` above do). A removed member's
requests are refused either way: the tests "a member removed from the org
loses access at once".

It costs one query a request, on the shared database; an app that can wait
for the next login leaves it out. It is the recommended way for routes whose
data matter, and the generated admin's routes, which check the Admin's role
the same way, use it.

#### The admin form: `requires user, role Admin, silo org`

An admin
manages every org's rows, a member or not, so `via` cannot give it the
org. Its routes take the org from their own parameter instead, and the
role check is what guards them:

```alder fragment
route Index = GET "/admin/orgs/{org}/project" { org: Org.OrgId, … } requires user, role Admin, silo org
```

Inside, `session.org` is the parameter's `Org.OrgId`, and the route reads
and writes that org's rows as a member's route would. `alder new admin`
writes these routes for the tables of a silo (chapter 15).

The rules (E0033):

- The clause needs the role ("`silo org` needs a role check: the route takes
  `org` from its parameter, which only the role check guards; write
  `requires user, role Admin, silo org`").
- The route needs the parameter, of the key's type ("`silo org` takes `org`
  from the route's parameter: add `org: Org.OrgId` to the route's
  parameters").

> **The role is the only guard.** It must be a role only some users have.
> `requires user, role Admin, silo org` opens every org to an Admin; with
> `role Member | Admin`, where every signed-in user is a `Member` (the
> workspace's default role), it opens every org to every user, a
> non-member included, since the route takes the org from its address, not
> from a membership.
>
> The checker cannot tell a role everyone has from one few have: give the
> admin form a role that is granted, never a default.

#### Where the rules are kept

Only where the key is narrowed: in the
members of a route that `requires` it (or the admin form) and the helpers
they call, and in tests, which run with the test's session.

- A fake has no session ("`Db.scoped Project` cannot be used in a fake: a
  fake has no session").
- A helper that uses a scoped operation, called from a route that does not
  narrow the key, is refused at the call, as for `@owner` ("`recent` uses
  `Db.scoped Project`, which needs a route that `requires org`: `Public` does
  not").
- Rows that are anyone's (an org's name, a user's) stay unscoped: the
  workspace reads `Db.get Org session.org` beside its scoped projects.

#### One org's database

With `silo org { Project, Todo }` in `App`
(chapter 8, "Silos") each org's projects and to-dos are in a database of
the org's own, and the scoped operations read and write that file: a
row of another org is not merely filtered out but absent. A `Task`
then writes to one database, the shared one or one org's (E0031); the
pages above are written that way already.

## Delivery, decided by the request

A route does not say whether it is a page or a piece of one, and a write
route does not say what the browser should do with its answer. The request
decides, and the runtime delivers. Three kinds of request reach a GET route:

| request | a GET route answers with |
|---|---|
| plain: the address bar, no JavaScript | a document: `<html>`, the head tags, the dev panel |
| a boosted link (a Datastar `GET`) | the body morphed in, the title, a history step |
| from an element (a Datastar `GET` with `Alder-Fetch: 1`) | the view, as one `datastar-patch-elements` event, by its root element's `id` |

A GET route whose view includes the layout is a page; one that does not is
a partial. The runtime does not know or care: opening a partial's address in
the address bar shows it as a bare document. That is the first kind of
request. A `click={Row { id: todo.id }}` on a button is the third: it asks
for the `Row` route and puts the answer where the `<li id="todo-…">` stands.

A view delivered **from an element** needs a root element with an `id`, since
that is where the answer goes. A view whose first root element has none is a
runtime error: it is logged, shown in the dev panel, and the page is left as
it was.

> **Several root elements.** If a view has several root elements, all of them
> are morphed in and the first one's `id` is the target.

### Request headers

Three request headers carry what the server does not remember.

#### `Alder-Page`

It is the path and query of the page the browser is on
(`location.pathname + location.search`). Every write sent from the page
carries it, because a write route does not know which page it was
clicked on (`Toggle` may be used from the index and from a detail page),
and `refresh` and `invalid` need it. The router resolves it to a GET route
with the matcher a GET uses.

Without JavaScript there is no such header, and the router reads the
`Referer` instead. Both go through `rt.BackURL`, which keeps the path and
query, drops the scheme and host, and refuses a path that starts with `//`,
so the answer never leads to another site.

#### `Alder-Build`

It is the build id of the page, on every Datastar request. A page from an
older build has its `<head>` (stylesheet, scripts), and a morph would keep
it. So a mismatch answers a full reload, never a patch:

- A link loads its address.
- An element fetch reloads the page it was made from.
- A write still runs (its decoders guard it) and then reloads: `redirect r`
  goes to `r`, anything else reloads `Alder-Page`.

#### `Alder-Shown`

It is the hash of the body as the server last rendered it for this page (the
whole body, or the hash sent after region patches). A write sent without it
(before the page's script ran) is answered with the whole body.

The server compares its own render before the `update` with the one after it:
when they are equal, no element event is sent.

### A write's response, with JavaScript

- **`redirect r` (303)**: the browser goes to `r` as a boosted link would:
  `load`, view, body morphed in, title, a history entry for `r`'s URL. It is
  never diffed against the page, even when `r` is the page the browser is on,
  and no before-view is rendered for it.

  A `redirect` to another site (`Url.external`) is a real navigation.

- **`render r` (200)**: the runtime runs `r`'s `load` and `view` with the
  session the write left, and sends one patch by the root `id`, mode outer.
  No before-view, no page load.

  If `r` cannot answer its view (the row was deleted meanwhile), nothing is
  patched and an error toast says so.

- **`refresh` (200)**: the page the browser is on (`Alder-Page`) is loaded
  again and diffed against the before-view; the regions that differ are
  patched, and the URL stays. A region is an element with an `id`
  (chapter 5, "Partial updates"); when the diff cannot be cut into regions
  safely, the whole body is morphed.

- **`invalid t` (422)**: the same, with the posted form's values and `t`
  attached to that form.

#### The before-view

The before-view is the page rendered before `update` runs, so there is
something to diff against.

The compiler works out per write route whether it can be needed: a route
renders one only when its `update` mentions `Response.refresh` or
`Response.invalid`, or calls an app function whose result type contains
`Response`. A route that only `redirect`s or `render`s renders none, which is
why a toggle by `render` costs one read by key.

#### What the browser keeps

Regions the server did not change are left as the browser has them.
`render` and an element fetch change the DOM without the server rendering
the page, so the hash `Alder-Shown` is out of date after them by design; the
guarantee is the one a partial page gives you: every region the server sent
is right.

Rows a click fetched, or an inline edit that is open, survive a `refresh`
that did not touch them.

### A write's response, without JavaScript

A plain `POST` runs `update` and answers with the response's own HTTP:

| response | answer |
|---|---|
| `redirect r` | `303` to `r` |
| `refresh` | `303` to the referring page (`Referer`, else `/`) |
| `render r` | `303` to the referring page (the partial has nowhere to go without a page) |
| `invalid t` | `422` with the referring page rendered, the form kept, `t` shown |

A button that sends a `PUT`, `PATCH` or `DELETE` route is a `<form
method="post">` with a hidden `_method` field, and the router honours it on
`POST` only. A `click` on a GET route is a `<form method="get">`. A `click`
on an element other than a button needs JavaScript.

## Patterns

What each of these costs and how it is delivered follows from the routes it
uses. Each is a few lines of routes and uses nothing else:

| pattern | routes | delivery | loads |
|---|---|---|---|
| list page | `Index = GET` | document, or the body morph for a link | 1 |
| filter, sort, tab, page number | none: `href={Index { filter: Done }}` | a boosted link, the URL moves | 1 |
| create with a form | `Create = POST … form` answering `redirect (Index {}) \|> toast "Saved"` | the body morph and a toast; `303` without JavaScript | 1 |
| refuse a form | `Response.invalid "name taken"` | `422`, the form kept, the error shown | before-view and after |
| toggle a row | `Toggle = PATCH` answering `render (Row { id })` | one `<li>`, by its `id` | 1 row |
| delete a row | `Delete = DELETE` answering `refresh` | `#todos` patched | 2 |
| add a comment | `Comment = POST … form` answering `refresh` | `#comments` patched | 2 |
| a like and a count elsewhere | `Like = POST` answering `refresh` | both regions | 2 |
| inline edit | `EditRow = GET`, asked for by `click`; `Save = PUT` answering `render (Row …)`; cancel is `click={Row …}` | the row becomes a form and back, in place | 1 row each |
| modal form | `NewDialog = GET`, asked for by `click` into `#dialog`; `Create` answering `redirect (Index {})` | the dialog, then the page | 1 |
| wizard | `Step2 = GET { name }`; `Next1 = POST` answering `redirect (Step2 { name: … })` | one URL per step; Back works | 1 |
| login, logout, theme | a `POST` that runs `Session.put` and answers `redirect` or `refresh` | as above | 1 or 2 |
| CSV export | `Export = GET "/export.csv"` whose view returns a `File` | the file ("File pages", above) | 1 |

"Loads" counts the `load`s of GET routes a click costs; `refresh` loads the
page twice, once for the before-view.

> **Theme switch.** A class on `<html>` changes only on a full load
> ([#189](https://github.com/tapegram/alder/issues/189)).

> **Not yet:** search as you type, "load more" on scroll, a widget that fills
> in when it appears and a badge that polls. They need elements that request
> a route on `input`, `intersect`, `load` or a timer, and `click` is the only
> element attribute that requests a route today.

## The request lifecycle

```
GET  /path
  1. decode route parameters      → 404 on failure
  2. check `requires`             → redirect to App.login on failure
     check `role R`, if declared  → 404 when the session's role is not R
  3. load                         → Query, run by the runtime: 404 on
                                    Query.notFound, 303 on Query.redirect
  4. view (with the session)      → HTML (a File, for a file page)

POST, PUT, PATCH, DELETE (the route's own path)
  0. a POST carrying `_method` is routed by it; a method no route
     declares for the path → 405 with `Allow`
  1. decode the route's parameters (path and query) → 404 on failure
  2. check `requires`             → redirect to App.login on failure
     check `role R`, if declared  → 404 when the session's role is not R
  3. decode the form, if any → on failure: 422, the page the browser is
     on rendered again with the errors, and stop (update is not called)
  4. render the page the browser is on, if update can answer
     `refresh` or `invalid` (the before-view)
  5. update                       → Task, run by the runtime (no `load`)
  6. redirect: 303 (under Datastar: the target page, morphed in, and a
     history push; to another site, an event that navigates)
     render: the route's load and view, one element patched in
     refresh: the shown page loaded again, the regions that differ patched
     invalid: the shown page with the text on the form, 422
     Task failure: App.error → HTML, status 500 (404 for Query.notFound
     and Task.notFound, 303 for Query.redirect)
```

## The `App` module

Every app has an `App` module:

```alder
module App

import Table.User
import Layout

session { user: Maybe User.UserId, role: User.Role = Member }

login = Auth.Login.Login

route Logout = POST "/_/Logout" { }

Logout.update : Logout.Params -> Session -> Task Response
Logout.update p session = {
  let _ <- Session.put (\s -> { s | user: Nothing, role: Member })
  Task.succeed (Response.redirect (Auth.Login.Login {}))
}

error : Error -> Html
error err =
  <Layout.Page title="Error">
    <h1>Something went wrong</h1>
    <p>Please try again.</p>
  </Layout.Page>

notFound : Html
notFound =
  <Layout.Page title="Not found">
    <h1>Not found</h1>
  </Layout.Page>
```

- `session` declares the session record. The runtime stores it in a signed
  cookie. Field types follow the same rules as form fields. `Session.put`
  (chapter 7) changes it; a `Task` that fails leaves it as it was.
- `login` names the route to redirect to when `requires` fails.
- `error` and `notFound` render failure pages; an app with a `silo` may
  add `maintenance : Html Never`, the 503 page of a silo not served.
- `route` lines and their `update`s declare app-level routes (above). They
  are optional.

## Notices and in-flight state

### Toasts

A write route can answer with a toast. `Response.refresh |> Response.toast
"Saved"` is the `refresh` it wraps, plus a notice: after the page's regions are
patched in, the runtime appends `<al-toast variant="info" timeout="5000">Saved</al-toast>` to
the page's `<al-toasts>` region as an event of its own, so a later morph does
not show it again.

Put `<al-toasts>` in the layout, outside anything a route re-renders; a page
without one gets a region the runtime adds to the body of the answer.

`Response` has four toast functions, each `Text -> Response -> Response`, one
per kind of toast:

- `Response.toast`, an `info` toast: news that is neither good nor bad
  ("Showing archived posts").
- `Response.success`: what the person asked for was done ("Saved", "Deleted 2
  of 3").
- `Response.warning`: it was done, with something to look at ("Saved, but the
  image was too large and was left out").
- `Response.error`: it was not done ("Error: try again"). An error toast
  stays for ten seconds (`timeout="10000"`); the others close after
  five seconds. A form's own problems are better told with `invalid`, next
  to the form.

> **Old names.** `Notice.with`, `Notice.success`, `Notice.warning` and
> `Notice.error` are the old names of these, for the page-and-message syntax;
> `alder fix --routes` rewrites them.

### Notices without JavaScript

Without JavaScript there is no toast (the region is hidden), so a request
made without the script (a form posted with JavaScript off) gets its notice
as an `<al-alert>` of the same kind at the top of the page that answers it.
The alert has `role="alert"` for a warning or an error, so a screen reader
reads it out at once, else `role="status"`.

The page that answers it is:

- the page itself, on an `invalid`;
- the next page, after a `303` (a `redirect`, or a `refresh` or `render` that
  went back to the referring page).

The kind and text ride a cookie signed with the app's key, `alder_notice`, for
that one page. Appendix C lists the exact markup of each kind.

A browser test reads the toasts of a step as `r.notices`, their texts, and
`r.toasts`, their kinds and texts (chapter 10).

### In-flight state

While a write is pending, the form (or the form around a `click` button)
that sent it is `inert` and `aria-busy="true"`, and a two-pixel bar runs along
the top of the page; both go when the answer arrives. A boosted link shows the
bar too, and Back during a pending GET still ends on the page Back returned
to.

An inert form loses the keyboard focus, so when the answer arrives the runtime
puts it back on the field that had it (or the element that now carries its
`id`), unless the focus has gone somewhere else meanwhile: a refused form
leaves the cursor where the user was typing.

The bar is a rule the runtime puts in the head (`html[data-alder-pending]`),
not part of the design library.


## How a request runs

You can write a whole app without reading this section. It explains what
the server that `alder build` makes (chapter 11) does with each request, so
that nothing in the lifecycle above is a mystery. Chapter 13 follows one
request through it byte by byte.

Four pieces of Go do the work:

- `rt/web` is the router and the session cookie.
- `build/<app>/routes__alder.go`, which the compiler writes for your app,
  holds one handler for each route.
- `rt/html` renders the page.
- `rt/datastar` writes the answer a Datastar request gets.

### The router

`web.NewRouter` in `rt/web/router.go` is the app's HTTP handler. For each
request it does these things, in order:

1. `GET /_alder/datastar-v1.0.0.js` is answered with the Datastar script,
   which is built into the binary (`rt/datastar/datastar.js`, version in
   `rt/datastar/VERSION`). The version is in the address, so the browser
   may keep the script for good: a new Datastar is a new address.
2. `GET /static/<file>` is answered with that file from your app's
   `static/` directory. Directories are never listed, and a path with `..`
   in it is refused.
3. For a `POST`, the body is read (for a write: every method but `GET` and `HEAD`): a
   url-encoded form is parsed (at most 1 MiB, else status 413), a multipart
   body is refused (415), and any other body is an empty form. Datastar
   sends a button's request with a JSON body, which Alder ignores.

   A `POST` whose form carries `_method` is routed by it: `PUT`, `PATCH` or
   `DELETE`, exactly one value, else `400`. On any other method `_method` is
   ignored, and in the query string it is never read, so a link from another
   site can never become a write.

4. The method and path are matched against the generated list `Routes`.
   A route is its method at its pattern, such as `POST /_/Toggle` or
   `PATCH /todos/Em2Gv4v9BaA0Wf9ujESN/toggle`. The most specific pattern is
   tried first: segment by segment from the left, a literal before a
   parameter.

   A path that some route declares, but not for this method, is a `405` with
   an `Allow` header listing the methods it has. Anything else is a 404
   rendered with `App.notFound`.

5. The session cookie is decoded (below).
6. The route's handler runs, and its answer is written.

A Datastar request from a tab of another build (its `Alder-Build` is not this
build's) ends in a reload instead of a patch ("Delivery", above).

A Go panic anywhere in the request becomes a 500 page rendered with
`App.error`.

#### Tags the router adds to the head

The router also adds tags to the `<head>` of every page it writes, so your
layout stays plain Alder. Right after `<head>`:

- `<meta name="color-scheme" content="light dark" />`, which tells the
  browser the page has a light and a dark theme, so even its first frame,
  painted before the stylesheet arrives, is dark in dark mode.
- `<meta name="alder-build" content="…">`, the id of this build, which the page
  sends back as `Alder-Build`.
- The rule for the progress bar ("Notices and in-flight state", above).
- The Datastar script tag,
  `<script type="module" src="/_alder/datastar-v1.0.0.js"></script>`.

Just before `</head>`: the *boost*, a `<meta name="alder-boost">` whose
attributes make the links of the page work without reloading it ("Links
and navigation", below). The boost is about a kilobyte, so it goes last:
a `<meta charset>` at the top of your `<head>` stays within the first
1024 bytes of the page, where the browser looks for it.

On a page without `<head>`, all of them go where the browser still reads them
as the head: right after `<html>`, or else before `<body>`.

### One handler per route

This is the handler for `Toggle`, a write route of the to-do app, from
`build/todos/routes__alder.go`:

```go
func Route__Todos__Index__Toggle() web.Route {
	return web.Route{Method: "POST", Pattern: "/_/Toggle", Handle: func(rq *web.Request) web.Response {
		params, errs := decode__Todos__Index__Toggle__Params(rq.Params, rq.Query)
		if errs != nil {
			return web.NotFoundResponse(html.None())
		}
		session, stop := guard__Todos__Index__Toggle(rq, params)
		if stop != nil {
			return *stop
		}
		var before *web.Shown
		if rq.Datastar {
			before, _ = rq.RenderShown(nil)
		}
		resp, err := eff.Run(rq.Env, eff.KTask, Todos__Index__Toggle__update(params, session))
		if err != nil {
			return route__fail(err)
		}
		return web.AnswerResponse(resp, before, rq.Env.Session)
	}}
}
```

It runs these steps:

- It decodes the route's parameters and answers 404 if that fails
  (`html.None()` is an empty document, which the router replaces with
  `App.notFound`).
- `guard__…` is `requires` and the role: a 303 to `App.login` or a 404 when
  the session falls short, otherwise the narrowed `Session` that `update`
  takes.
- Because `Toggle.update` mentions `Response.refresh`, the handler also
  renders the page the browser is on, before the update, for a Datastar
  request (`rq.RenderShown`); a route that only redirects or renders has no
  such lines.
- Then the runtime runs the `update` `Task`, and `web.AnswerResponse` hands
  the `Response` and the before-view to the router, which delivers them
  ("Delivery", above).

The names are the route's qualified name with each dot written as `__`: the
route `Toggle` of module `Todos.Index` is `Todos__Index__Toggle`, and its
member `update` is `Todos__Index__Toggle__update`.

A GET route has its own handler, `Route__Todos__Index__Index`: it decodes the
path and query, checks `requires`, runs `load`, and renders `view`.

### The page a write came from

A write route does not know which page sent it. `Toggle` on `/?filter=Active`
has to load the Active list again, not the full list. The page's own address
is in the request: the page's post helper (`window._alderO`, defined once in
the page's `<head>`) sends it as `Alder-Page`, and the router resolves it to a
GET route and its parameters with the matcher a GET uses. This is the Done
button of `GET /?filter=Active`, as the running app sends it:

```html
<form method="post" action="/_/Toggle?id=Vggu4dUM2D0o1KQ-UzWf" class="_alder-click" data-on:submit="(window._alderBusy||((e,p)=&gt;p))(el, @post(&#39;/_/Toggle?id=Vggu4dUM2D0o1KQ-UzWf&#39;, window._alderO?.(&#39;form&#39;)))"><al-button variant="secondary" size="sm"><button type="submit">Done</button></al-button></form>
```

(A `click` button is wrapped in a form of its own so that it works without
JavaScript too; "With and without JavaScript", below, explains it.)

`id` is the route's own parameter, in the query string because it is not in
the route's path (`/_/Toggle`). The page's parameters are not in the URL at
all: the filter is in `Alder-Page: /?filter=Active`, which the post carries as a
header.

A route whose parameters are in its path puts them there, as `Todos.Show`'s
`Save` does (`/todos/Vggu4dUM2D0o1KQ-UzWf/_/Save`).

A `List` parameter repeats its key, as in links (`rt.QueryKVs` in
`rt/core.go`): a route parameter `tag` holding `["a", "b"]` is
`tag=a&tag=b`. The handler collects every value of the key.

Without JavaScript there is no `Alder-Page`, and the router reads the
`Referer` header instead. Both go through `rt.BackURL` (`rt/core.go`), which
keeps only the path and query, so the answer never leads to another site.

A write from a page the router cannot match to a GET route (another app, a
stale build) is answered by going to that address: a 303, or a navigation
under Datastar.

### The session cookie

#### What is in it

The session lives in one cookie, `alder_session`. Its value is two parts
joined by a dot: a small JSON document, then an HMAC-SHA256 signature of
it, both in unpadded base64url (`Sign` in `rt/web/session.go`). After
logging in to the to-do app, the first part decodes to

```json
{"iss":1791573032,"session":{"User":{"Ok":true,"Val":"oV5Et5NZUPszOeLnOSSB"}}}
```

`session` is the session. `iss` is when it was issued, in seconds since
1970: the moment a response last changed the session, such as the login.
The cookie stops counting 30 days after that. The server checks it
itself, so a copied cookie is no good after that, whatever the browser
keeps, and a cookie without `iss` is no session at all.

A union without payloads, such as the `Theme` of "App-level routes"
above, is stored by its constructor's name (`"Theme":"Dark"`), so
reordering the constructors doesn't change what an old cookie means.

A field the cookie lacks, or whose value no longer decodes, gets its default.

#### Signed, not encrypted

The cookie is **signed, not encrypted**. Anyone holding it can read it, so
never put a `Text.Secret`, or anything the user must not see, in the
session. What the signature does is stop anyone from changing it. A cookie
whose signature doesn't match is ignored, and the request sees the default
session: every field at its default, so `user` is `Nothing`.

#### The signing key

The signing key comes from the environment variable `ALDER_SECRET`, which
must be at least 16 bytes. Without it (unset, **or shorter than 16
bytes**) the server refuses to start:

```
alder: ALDER_SECRET must be set to at least 16 bytes outside dev mode (ALDER_DEV=1)
```

and exits with status 1. Only in development mode (`ALDER_DEV=1`, chapter
11) does it use a fixed development key instead, which is public (it is in
`rt/web/session.go`), and log this once at startup:

```
warn: ALDER_SECRET is not set (or shorter than 16 bytes); dev mode signs sessions with the public development cookie key
```

Anyone can forge a session signed with that key, which is why production
needs its own.

#### Attributes

The cookie is `HttpOnly` (page scripts cannot read it), `SameSite=Lax`,
`Path=/`, and lasts 30 days from the time it was issued (its
`Max-Age`, and the `iss` signed into it). It is
`Secure` when the request came over HTTPS, or carries the header
`X-Forwarded-Proto: https` (set by a proxy in front of the server).

#### When it is stored

The cookie is stored only when the answer **changes** the session:

- After an `update` whose `Session.put` ran (even one that wrote the value
  the session already had, such as logging in again as the same user) or that
  left a different session, whether the answer is a page, a 303 or a
  navigation event.
- On a 404 or 500 when such an `update` succeeded and the page then failed to
  load again (what the `update` committed is kept).

A stored cookie is issued then, for a fresh 30 days. Any other answer sends no
cookie: a page view, a form that fails to decode, a failed `Task`.

The browser keeps the cookie it has, with its issued time, so visiting does
not renew a session: a user who keeps using the app still logs in again 30
days after logging in.

The lifetime is absolute on purpose. Whatever the login wrote into the
session, such as a role (above), lasts until logout or 30 days after it was
written, and no longer.

### Form errors

When a form fails to decode, the handler does not call `update`. It runs
`requires`, renders the page the browser is on with the form's state (what
was submitted, and a list of messages per field), and answers `422`. Posting
`title=` to the to-do page renders

```html
<ul class="errors" data-errors="title"><li>must not be empty</li></ul>
```

where `<Errors field="title" />` stands. The values and errors go only to
the form that posted: the form state names the request the form makes, its
route with its own parameters (`rt.RouteKey`), and the renderer matches it
against each `<form>`'s route, so a per-row form of another row keeps its
initial values.

The value of a `Text.Secret` field is never sent back: its input is a
password input, and it comes back empty.

`Response.invalid t` renders the same way, with `t` in `<Errors form />` of
the form that posted. Unlike a form error, the `update` that answered
`invalid` ran and committed.

A form the re-rendered page no longer draws (a stale page: the state changed
after it was loaded) shows nothing, and so does a form with no
`<Errors form />`: the page comes back unchanged (W0005).

### With and without JavaScript

The browser either runs Datastar (the usual case) or does not. The answers
differ.

#### With Datastar

A write is sent with the header `Datastar-Request: true`,
and the headers of "Delivery" above. A `refresh`, an `invalid`, a form
error, a `render`, a 404 or a 500 is answered with status 200 and
server-sent events:

- For `refresh` and `invalid`, the regions that differ
  (`datastar.PatchRegion`; the whole `<body>`, with `datastar.PatchBody`, when
  the page cannot be cut into regions safely).
- For `render`, one element.
- For a 404 or a 500, the failure page's `<body>`.

Datastar morphs what it is sent into the page, so only what changed is
touched. A 404 or 500 is sent as an event too, because Datastar ignores
answers that are not 2xx; the real status of an `invalid` or a form error
(422), a 404 or a 500 is in the header `Alder-Status`.

#### A redirect under Datastar

A `redirect` (and a `requires` that sends the user to the login page) is
not a 303 under Datastar. Datastar posts with `fetch`, which would follow a
303 by itself: the new page would be morphed into the old one while the
address bar and the history stayed at the old URL.

So when the target is a page of this app (a path starting with `/`), the
router fetches it itself, in the same request: it serves a `GET` of the
target, with the session the write just stored, and answers with that page
the way it answers a link ("Links and navigation", below): the target's
`<body>` morphed in, its `<title>`, and a script that records the new URL in
the browser's history. Logging out of the blog answers (bodies shortened)

```
event: datastar-patch-elements
data: selector body
data: mode outer
data: elements <body>…the login page's body…</body>

event: datastar-patch-elements
data: selector head > title
data: mode outer
data: elements <title>Log in</title>

event: datastar-patch-elements
data: selector body
data: mode append
data: elements <script data-effect="el.remove()">(function(){var u="/login";…history.pushState(null,"",u);…})()</script>

```

with the session cookie in its headers, as a 303 would carry it. The
browser makes one request instead of two, and nothing reloads.

#### A redirect to another site

A `redirect` to another site (an `Url.external` URL) cannot be fetched
that way, so its answer is a 200 event that appends a script to the
`<body>`; the script removes itself and tells the browser to go to the
URL (`datastar.Navigate` with `window.location.assign`, what the official
Datastar SDKs do):

```
event: datastar-patch-elements
data: selector body
data: mode append
data: elements <script data-effect="el.remove()">window.location.assign("https://example.com/")</script>

```

The URL is a JSON string in both scripts, so `<`, `>` and `&` in it are
escaped and it can never end the script. Without Datastar, `redirect` is
a plain 303.

Either way the target must be a path of this site or an `http:`/`https:` URL:
`Url.external` turns anything else (a `javascript:` URL from user input, say)
into `#` (chapter 9), and the router answers a redirect to such a URL with a
500 rather than send it.

#### The event format

Every line of an event's elements is a `data: elements` line, and an event
ends at a blank line. A carriage return in your data could end a line
early and start a field of its own, so the renderer writes it as `&#13;`
and `PatchBody` turns any line end into a plain newline first.

#### Forms and `click` buttons under Datastar

A form and a `click` button travel differently under Datastar.

A form posts
with `data-on:submit="@post('<url>', window._alderO?.('form')||{contentType: 'form'})"`, so
its fields arrive url-encoded and are decoded as the form. The
`window._alderO` helper (defined once in the page's `<head>`) gives the post
its options: the content type, the `Alder-Shown` header (chapter 5,
"Partial updates"), `Alder-Page` and `Alder-Build`, and the page's one
navigation signal, shared with boosted links ("Links and navigation" below).

That signal is why a new post or click aborts the request still in flight, so
a slow answer never lands over a newer page.

A `click` button's request carries its parameters in the URL, and its body is
ignored. The method it uses depends on the route:

- A `PUT` or `PATCH` route is sent with its own method (`@put`, `@patch`).
- A `DELETE` is posted (`@post`) with `_method=DELETE` in the body whenever it
  sends a form: a form, and a `click` button outside a form, which is
  rendered in a form of its own.
- Only a `click` button inside another form (or another element with
  `click`) sends `@delete`, because Datastar puts the fields of a `DELETE` in
  its query, where the router never reads `_method`.
- A `click` on a GET route is not a post at all: it is a Datastar `GET` with
  `Alder-Fetch: 1` (`window._alderF`), and the answer is the route's view as
  one element.

#### A `click` button without JavaScript

A `click` button works without JavaScript too. Outside a form, it is
rendered inside a form of its own, as the Done button above shows: without
Datastar the browser posts that form to the route's URL (a hidden `_method`
field carries `PUT`, `PATCH` or `DELETE`); with it, Datastar posts it on
submit.

The wrapper has the class `_alder-click`; the design system's
stylesheet gives it `display: contents`, so it takes no part in the
layout and the button stays in line (without `al.css`, write that rule
yourself, and keep your own `form` rules off it with
`form:not(._alder-click)`, as the to-do app's `static/app.css` does).

The Done button is
an `<al-button>` (Appendix C), which the compiler renders with a real
`<button>` inside it; the wrapping form then goes around the whole
`<al-button>`, so the design system's stylesheet still finds the button
as the element's child.

Inside a
`<form submit>`, where forms cannot nest, a `click` button on a write route
is a submit button with its own target instead:

```html
<button type="submit" formaction="/_/Logout" formmethod="post" formnovalidate data-on:click__prevent="@post(&#39;/_/Logout&#39;)">Log out</button>
```

Datastar sends the request on click and stops the form's own submission.
Without JavaScript the browser posts the enclosing form's fields to the
button's URL; a route without a form ignores them.

> **`formnovalidate`.** It keeps an empty `required` field of the form from
> blocking the button, and so does leaving the fields out of the Datastar
> post: the browser would otherwise refuse to send a form with an empty
> required field.

A `click` on a GET route inside a form, and a `click` on another element,
such as a `<li>`, need JavaScript.

#### Answers without JavaScript

Without JavaScript, the answer to a write is a whole page for a form
error or an `invalid` (they must show what was submitted: `422`), and a 303
for everything else: `redirect` goes to its route, and `refresh` and `render`
go back to the page that sent the write, rebuilt from its address in the
`Referer` header (`rt.BackURL`), so the address bar never shows a write URL
and a refresh never posts again (Post/Redirect/Get).

Toggling Milk on `/?filter=Active` without JavaScript answers

```
HTTP/1.1 303 See Other
Location: /?filter=Active
```

and the browser loads the list again.

### App routes

The handler for an `App` route is an ordinary write route's: there is no page
of its own to show, so what it answers is read like any other, with the page
the write came from taken from `Alder-Page` (or `Referer`). A `redirect`
goes to its route; a `refresh` reloads the page the button was on, with the
new session.

Clicking "Log out" on `/?filter=Active` answers a cookie whose `User` is
`Nothing` and a redirect to `/login`.

With the theme switch of "App-level routes" (its `update` answers `refresh`),
clicking "Dark mode" there answers a cookie whose `Theme` is `"Dark"` and,
under Datastar, the `<body>` changes that the theme causes. Without
JavaScript it answers `303 /?filter=Active`, which loads the whole page.

> **Not yet:** under Datastar the class on `<html>` does not change; it
> arrives with the next full load
> ([#189](https://github.com/tapegram/alder/issues/189)).

### Links and navigation

#### The boost

A link in an Alder page is a plain `<a href>`, and without JavaScript a
click on it loads the page it names, as on any site.

With Datastar, a
click on a link to a page of this app does not load anything. The boost
in the page's `<head>` catches the click, cancels the browser's own
navigation and asks for the page with Datastar instead, as
`GET /posts/42` with the header `Datastar-Request: true` (and without
`Alder-Fetch`, which marks a `click` on a GET route and is answered with one
element instead, "Delivery" above).

The router answers that request with three events, in this order:

1. the page's `<body>`, which Datastar morphs into the current one, so
   what is the same on both pages (the header, the layout) is not
   touched;
2. the page's `<title>`, so the tab shows the new page's name;
3. a small script that adds the URL to the browser's history
   (`history.pushState`), scrolls to the top, and moves the keyboard
   focus to the new `<body>`, so a screen reader starts reading the new
   page.

The `<head>` is never replaced, so the stylesheet is not loaded again and
the page does not go blank in between: no white flash between two dark
pages.

#### Back and Forward

Back and Forward work as usual. The boost listens for them too
(the browser's `popstate` event): it asks for the page of the URL the
browser went back or forward to, and the answer replaces the history
entry instead of adding one, and scrolls to where you were on that page.
The boost keeps each entry's scroll position in the entry's history
state as you scroll, so both directions find it.

Back and Forward fetch the page again rather than restoring it from a cache:
what you typed into a form on that page is gone, and the page shows the data
as it is now.

#### While a page is on its way

While a boosted page is on its way, the old page stays as it is: nothing
says a click is pending, and if the server is down Datastar retries the
request quietly instead of showing an error.

> **Not yet:** a loading indicator is for a later version (M7b).

Only the newest navigation's answer is applied. The boosted GETs and
every write a page sends (a form's submit, a `click`) share one
`AbortController`, `window._alderAC`: each starts by aborting the
request still in flight. So a slow post followed by a click on a link
ends on the link's page, never on the post's late answer.

> **An aborted post.** The post itself may still have run on the server
> (aborting stops the browser waiting, not the server), and a form whose post
> was aborted keeps what was typed into it, with nothing to say it was not
> sent (also M7b's: inert forms while a post is pending).

#### Failing pages

The answer to a link whose page fails is the same three events with the
`App.notFound` or `App.error` page as the body, and the status in the
header `Alder-Status` (`404` or `500`): the address bar shows the URL you
asked for, as after a page load.

A link to a page whose `requires` sends the visitor to the login page ends on
the login page, and its URL is the one recorded.

#### Clicks the boost leaves alone

The boost leaves these clicks to the browser:

- a link to another site, or to a URL that is not `http:` or `https:`
  (`mailto:`, `tel:`);
- a link to a `#fragment` of the page you are on (a jump within the
  page); a fragment of another page is boosted like any link, and the
  browser then scrolls to it;
- a link with a `target` (`target="_blank"` opens a new tab) or a
  `download` attribute;
- a click with Ctrl, Cmd, Shift or Alt held (open in a new tab or
  window), or with a button other than the main one;
- a click that something else on the page already handled (an event
  handler that called `preventDefault`);
- a link with the attribute `data-alder-reload`;
- a link to a file page (chapter 6, "File pages"): the compiler marks it
  `data-alder-reload` itself, since a file is a document of its own;
- a link to a file under `/static/` or `/_alder/` (a stylesheet, a
  script, a PDF): a document of its own, not a page to morph in. The
  router answers a Datastar `GET` of such a path (from an older page's
  boost) with a real navigation to it, too.

#### `data-alder-reload`

The last one is the way out. Give a link the attribute
`data-alder-reload` when it must load its page for real, such as a link
to a page that needs a fresh `<head>`. A `data-*` attribute takes a
`Text` (chapter 5), and the boost only looks at whether the attribute is
there, so any value will do:

```alder fragment
<a href={Todos.Index.Index {}} data-alder-reload="true">Start over</a>
```

A link to a file page (a download, say) needs no such attribute: the
compiler adds it (chapter 6, "File pages").

A form that submits with `GET` (a search box, say) is not a link, and is
not boosted: it loads its page as usual.

### Failure pages

A path no route matches, a route parameter that doesn't decode,
`Query.notFound` and `Task.notFound` are answered with `App.notFound`,
status 404. `Query.redirect p` is not a failure page: it is a 303 to `p`,
answered as a `redirect` is (under Datastar, the target morphed in).

A failed `Task` is answered with `App.error`, status 500, and logged as an
`error` line naming the request.

With `ALDER_DEV=1` (chapter 11), the error's text is added at the end of the
page's `<body>` as an HTML comment, such as
`<!-- error: Db.update Todo: no row with id "nope" -->`. Without it, the
page never shows the text.

### Deploying

An app also says where it ships. A `deploy { … }` declaration in `App`
names the environments (a server reached by SSH, a Fly or Render app, or a
script of your own) and what must hold before a build goes out:

- the least mutation score,
- the commands to run,
- the environment variables the target must provide.

It is data that the `alder deploy` command reads; it adds no page, no route
and no code to the running program, and it never holds a secret, only the
names of the variables that do. Chapter 16 writes the declaration, the
command and the gate in full.
