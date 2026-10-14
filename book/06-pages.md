# 6. Pages, messages, forms

## A page module

A page is a module that declares a route and the five things that go with
it. This is the whole shape of an Alder app.

```alder
module Todos.Index

import Todo

page "/" { filter: Todo.Filter = All } requires user

type Model = { filter: Todo.Filter, todos: List Todo }

load : Params -> Session -> Query Model
load params session = {
  let todos <- Db.mine Todo |> Db.all
  Query.succeed { filter: params.filter, todos: todos }
}

view : Model -> Session -> Html Msg
view model session =
  <ul>
    {for todo in model.todos ->
      <li><button click={Toggle { id: todo.id }}>{todo.title}</button></li>}
  </ul>

msg Toggle { id: Todo.TodoId }

update : Msg -> Model -> Session -> Task Nav
update msg model session = match msg {
  -- The id in a message comes from the request: `Db.updateMine` changes the todo only when it
  -- is the signed-in user's, and answers 404 otherwise ("Rules", below).
  Toggle p -> {
    let _ <- Db.updateMine Todo p.id (\t -> { t | done: not t.done })
    Task.succeed Rerender
  },
}
```

`Db.mine Todo` and `Db.updateMine` are the to-do table's **scoped**
operations: they read and write only the todos of the signed-in user,
because the table marks its `owner` column `@owner` ("Rules", below).

The compiler checks that each page module has exactly: one `page`, one
`Model`, one `load`, one `view`, and, if it has any `msg`, one `update`.
`Params` and `Msg` are generated from the declarations and must not be
written by hand. `view` is always `Model -> Session -> Html Msg`, even on
a page with no `msg` (its `Msg` is then empty; `Html Never` is not used for
pages). `view` receives the session so that per-user presentation (theme,
"logged in as") needs no Model field.

## `page`

```alder fragment
page "/todos/{id}" { id: Todo.TodoId }
page "/" { filter: Todo.Filter = All, q: Maybe Text }
page "/write" { } requires user
```

- The path is a literal with `{name}` segments. Each segment must appear in
  the parameter record; parameters not in the path are query parameters.
  Segments are letters, digits, `.`, `_`, `~`, `-`, or a whole `{param}`;
  anything else (`//x`, a space, `?`) is E0029.
- Parameter types must be one of: `Int`, `Text`, `Bool`, a brand of those,
  a union with no payloads, `Maybe` of those, or `List` of those.
- A parameter with a default is optional in links and in the URL.
- A `List` parameter repeats its key in the URL: `{ tag: List Text }` with
  `["a", "b"]` is `?tag=a&tag=b`, and `[]` is no key at all.
- `Params` is the parameter record type for this module.
- Two pages may not answer the same path: `/x/{id}` and `/x/{slug}` are the
  same pattern, so the second is an error (E0016). Patterns that merely
  overlap are fine, whatever the module order: a literal segment is matched
  before a parameter, left to right, so `/todos/new` is served before
  `/todos/{id}`.
- `requires` names a field of `Session` whose type is `Maybe a`. The page is
  served only when that field is `Just`; otherwise the runtime redirects to
  the page named in `App.login`. `requires user, role Author` adds a role:
  the page is also a 404 for a signed-in user whose session's `role` is
  not `Author` ("Rules", below). Inside this module `session.<field>` has type
  `a`, not `Maybe a`. The page's `Session` is then a type of its own, and it
  can be passed wherever the app's `Session` is expected (to a function in a
  shared module, say), where the field is `Just` its value again. The other
  direction is a type error (E0003): a function of the page that takes its
  `Session` cannot be given the app's, whose field may be `Nothing`.

A page value is the module name applied to its parameter record. It is the
only thing `href` accepts:

```alder
<a href={Todos.Show { id: todo.id }}>{todo.title}</a>
<a href={Todos.Index { filter: Active }}>Active</a>   -- defaulted fields may be omitted
```

Two page values are `==` when they lead to the same address: a field left
at its default is not in the URL, so `Todos.Index {} == Todos.Index
{ filter: All }`.

## `msg`

A message belongs to the page that declares it. It has no path: the
compiler generates the POST URL from the page and message name, so two
messages can never collide.

```alder fragment
msg Toggle { id: Todo.TodoId }
msg Submit form NewTodo
msg Comment { post: PostId } form NewComment
```

- Parameter types follow the same rules as `page` parameters. A message
  with no parameters is declared bare: `msg Logout`.
- `form F` means the request body is decoded as form type `F`.
- `Msg` is the union of the module's messages. Each constructor takes its
  parameter record. If the message has a form, the record has an extra
  field `form: F` that the runtime fills in.

A message value in HTML is the constructor applied to its parameters,
without `form`:

```alder
<button click={Toggle { id: todo.id }}>Done</button>
<form submit={Comment { post: model.post.id }}>…</form>
<form submit={Submit}>…</form>                 -- no parameters: bare constructor
```

In `update`, the same constructor carries the full record:

```alder fragment
update msg model session = match msg {
  Toggle p -> ...,                 -- p.id : Todo.TodoId
  Submit p -> ...,                 -- p.form : NewTodo
  Comment p -> ...,                -- p.post : PostId, p.form : NewComment
}
```

The page's own parameters travel with every message it sends, so `update`
always has the same `Model` that `view` had. They ride in the message's URL
under keys that start with `_p.` ("How a request runs", below).

A page can only send its own messages and the app-level messages below. It
cannot send another page's message.

## App-level messages

Actions that belong to no page (log out, switch theme) are declared as `msg`
in the `App` module. Any page can send them, and `App.update` handles them.
It has no `Model`, and no page of its own to render, so its `Nav` is
answered differently: `Redirect p` is a redirect to `p`, and `Rerender` and
`Reject` are a redirect back to the page that sent the message (its
address, from the browser's `Referer` header, or `/` without one). The page
is then loaded again with the new session. A `Reject` text from
`App.update` is not shown anywhere, and neither are the errors of a form on
an App message that fails to decode: both answer with the same redirect
back, and the errors are dropped. Give a form that needs its errors shown a
page message. ("How a request runs", below, shows the code.)

```alder fragment
-- in App
msg Logout

update : Msg -> Session -> Task Nav
update msg session = match msg {
  Logout p -> {
    let _ <- Session.put (\s -> { s | user: Nothing })
    Task.succeed (Redirect (Auth.Login {}))
  },
}

-- in any page, or any component
<button click={App.Logout}>Log out</button>
```

An app-level message value is accepted in `Html a` for every `a`, so a
layout component typed `Html a` can contain `click={App.Logout}`.

### Example: a theme switch

The to-do app has one app-level message, `Logout`. A switch between a
light and a dark theme is the other usual one, and it uses every piece an
app-level message can involve: a type for the state, a session field to
keep it in, the message and its arm of `App.update`, and a button that
every page shows. (The example app leaves it out, so that its `App`,
`Layout` and login page are a skeleton a new app can copy and check with
nothing to write except its own home page.) Here is how to add it.

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

`App` imports it, gives the session a field for it, and declares the
message. This block is an excerpt, and the verifier does not check it: an
app declares its `session` once, and the example's `App` (in "The `App`
module", below) already does. The new lines are the `import`, the
`theme` field, `msg SwitchTheme` and its arm:

```alder fragment
-- in App
import Theme

session { user: Maybe User.UserId, theme: Theme = Light }

msg Logout
msg SwitchTheme

update : Msg -> Session -> Task Nav
update msg session = match msg {
  Logout p -> ...,
  SwitchTheme p -> {
    let _ <- Session.put (\s -> { s | theme: Theme.flip s.theme })
    Task.succeed Rerender
  },
}
```

`theme: Theme = Light` has a default, so a visitor's first request, and a
cookie from before the field existed, get `Light` ("The session cookie",
below). `Rerender` sends the browser back to the page the button was on,
which loads again with the new session.

The layout cannot read the session; it takes the theme as a prop, sets a
class on `<html>` for the stylesheet, and shows the button. Also an
excerpt, of `app/Layout.alder`:

```alder fragment
-- in Layout
import Theme

Page : { title: Text, theme: Theme, children: List (Html a) } -> Html a
Page props =
  <html class={if props.theme == Dark then "dark" else ""}>
    ...
    <body>
      <nav>
        <button click={App.SwitchTheme}>{if props.theme == Dark then "Light mode" else "Dark mode"}</button>
      </nav>
      <main>{props.children}</main>
    </body>
  </html>
```

The class does nothing until the stylesheet has rules for it. Add them to
`static/app.css` (the to-do app's, served at `/static/app.css`), after the
rules they override:

```css
html.dark body {
  color: #e6edf3;
  background: #0d1117;
}

html.dark a {
  color: #4493f8;
}
```

Every call of `Layout.Page` then passes a theme: a page passes its
session's, and `App.error` and `App.notFound`, which have no session,
pass `Light`:

```alder fragment
<Layout.Page title="Todos" theme={session.theme}>
<Layout.Page title="Error" theme={Light}>
```

That is the cost of the feature, and why the example does without it:
every page's `view` passes the theme on, and a test that builds a session
by hand names the field too, `{ user: me, theme: Light }` (chapter 10,
"Testing effects"), and the test's module imports `Theme` for `Light`,
because a record literal lists every field, defaults or not: defaults
apply at the boundaries, the cookie, a page parameter or an insert
(chapter 12, "No defaulted record literals"). A literal that leaves `theme` out is refused, for
App's `Session` and a `requires` page's alike:

```
error[E0003]: record literal for `Session` is missing `theme` (a default applies only to an insert, the session cookie, a page or message parameter, or a form, not to a literal)
```

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

When a submitted form fails to decode (missing field, failed refinement,
wrong type), the runtime does **not** call `update`. It re-renders the page
with the submitted values and errors attached to that form; `<Input>` and
`<Errors>` show them. The program never sees an invalid form. Only the form
that posted the message shows them: on a page with one form per row, the
other rows keep their own values. A field's errors go to its
`<Errors field="…" />`; the errors of a field without one are listed in the
form's `<Errors form />`, after its name ("due must be a date
(YYYY-MM-DD)"). A form with neither for a field gets warning W0003.

## `update` and `Nav`

```alder fragment
update : Msg -> Model -> Session -> Task Nav
```

`update` returns a `Task` (chapter 7) that ends in a `Nav`:

```alder fragment
-- Built in; shown for reference.
type Nav
  = Rerender                 -- re-run load and view for this page, patch the result in
  | Redirect Page            -- go to another page (HTTP 303)
  | Reject Text              -- re-render with this error shown on the form that sent the msg
```

Use `Rerender` for in-place changes (toggle, delete). Use `Redirect` after
creating something, so a browser refresh doesn't resubmit. Use `Reject` when
a well-formed submission is refused for a reason only `update` can know
(wrong password, name already taken); `<Errors form />` shows the text,
and a form that can be rejected without one gets warning W0005.

`Reject` is a successful result, not a failure: the `Task` that produced it
commits, so anything it wrote and any `Session.put` it ran are kept. To
refuse without changing anything, decide before writing (`let taken <-` a
query, then `Reject` or the writes). Only a failed `Task` (`Task.fail`,
`Task.notFound`, a `@unique` violation, a capability error) undoes its
writes and its session changes (chapter 7).

## Rules

Most apps with a login have two rules about who may do what. A row
belongs to a user, and only that user may see or change it: a todo, a
draft post. And some pages are only for users with a role: an author's
desk, an admin page. Alder lets you **declare** both rules, and then the
shortest thing to write is the correct one: no check to remember, so no
check to forget.

### Ownership: `@owner`

A table says which of its columns names the row's owner. In the to-do
app (`examples/todos/app/Todo.alder`):

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
| `Db.mine Todo` | `Select Todo` | The rows whose `owner` is the signed-in user. It is a `Select`, like `Db.from Todo`, so `Db.where`, `Db.orderBy`, `Db.limit`, `Db.all`, `Db.count` and the rest apply to it |
| `Db.getMine Todo id` | `Query Todo` | The row with that id, when it is the user's; otherwise not found |
| `Db.updateMine Todo id f` | `Task Unit` | Changes the row with `f`, when it is the user's; otherwise not found |
| `Db.deleteMine Todo id` | `Task Unit` | Deletes the row, when it is the user's; otherwise not found |
| `Db.insertMine Todo { … }` | `Task TodoId` | Inserts a row whose owner is the user |

The to-do list's page uses four of them
(`examples/todos/app/Todos/Index.alder`):

```alder fragment
load params session = {
  let todos <-
    Db.mine Todo
      |> Db.orderBy Todo.created.desc
      |> Db.all
  Query.succeed { filter: params.filter, todos: List.filter (Todo.matches params.filter) todos }
}

update msg model session = match msg {
  Submit p -> {
    let _ <- Db.insertMine Todo { title: p.form.title, due: p.form.due }
    Task.succeed (Redirect (Todos.Index { filter: model.filter }))
  },
  Toggle p -> {
    let _ <- Db.updateMine Todo p.id (\t -> { t | done: not t.done })
    Task.succeed Rerender
  },
  Delete p -> {
    let _ <- Db.deleteMine Todo p.id
    Task.succeed Rerender
  },
}
```

and the todo's own page, `Todos.Show`, reads its todo with the fifth:

```alder fragment
load params session = {
  let todo <- Db.getMine Todo params.id
  Query.succeed { todo: todo }
}
```

Why `update` needs them as much as `load` does: `load` decides what the
page shows, but a message carries its own parameters. `Toggle { id }`
puts the id in the message's URL (`/_m/Todos.Index/Toggle?id=…`), and
anyone signed in can post that URL with an id that is not theirs. With
`Db.updateMine` the id is only ever one of the user's own todos.

Two details:

**`Db.insertMine` takes its row as a record literal**, written in the
call, with every field but the owner: the owner is the session's user.
Naming it is an error:

```alder error=E0003 "`owner` is set by `Db.insertMine`"
module Quick

import Todo

page "/quick" { } requires user

type Model = { }

load : Params -> Session -> Query Model
load params session = Query.succeed { }

msg Add { title: Text.NonEmpty }

update : Msg -> Model -> Session -> Task Nav
update msg model session = match msg {
  Add p -> {
    let _ <- Db.insertMine Todo { title: p.title, due: Nothing, owner: session.user }
    Task.succeed Rerender
  },
}

view : Model -> Session -> Html Msg
view model session = <p>quick</p>
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
the compiler knows there is one: in a page module whose page `requires
user`. That covers the module's `load`, `update` and `view`, and its
helper values, such as

```alder fragment
recent : Select Todo
recent = Db.mine Todo |> Db.orderBy Todo.created.desc |> Db.limit 5
```

Anywhere else, it is E0026: in a page without `requires user`,

```alder error=E0026 "`Db.mine` needs a page that `requires user`: `Public` does not"
module Public

import Todo

page "/public" { }

type Model = { open: Int }

load : Params -> Session -> Query Model
load params session = {
  let n <- Db.mine Todo |> Db.count
  Query.succeed { open: n }
}

view : Model -> Session -> Html Msg
view model session = <p>{Text.fromInt model.open}</p>
```

and in a module that is not a page at all, `App` included ("`Db.deleteMine`
needs a page that `requires user`: `App` is not a page"). The rule
follows a helper across modules: a value of a `requires user` page that
uses a scoped operation, used from somewhere the operation itself would
be refused, is E0026 at that use ("`Todos.Index.recent` uses `Db.mine`,
which needs a page that `requires user`: `Public` does not"). A `fake`
(chapter 10) has no session either: "`Db.mine` cannot be used in a fake:
a fake has no session" (an excerpt: the message begins with where the
use is, such as "in `fake Counter`'s `count`:"). Tests may use them (below).

A table without `@owner` has nothing to scope by (E0025):

```alder error=E0025 "`User` has no `@owner` column, so `Db.mine User` has nothing to scope by"
module Account

import User

page "/account" { } requires user

type Model = { users: Int }

load : Params -> Session -> Query Model
load params session = {
  let n <- Db.mine User |> Db.count
  Query.succeed { users: n }
}

view : Model -> Session -> Html Msg
view model session = <p>{Text.fromInt model.users}</p>
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
  (chapter 10, "Tests that use the rules"). A test that never logged in
  has no user, and a scoped operation fails with `Db.mine Todo: no
  signed-in user` (or `Db.getMine Todo: …`, and so on). It never quietly
  returns nothing.
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
  Task.succeed (Redirect (Posts.Index {}))
},
```

and logging out puts it back: `{ s | user: Nothing, role: Reader }`.

Adding `role` to the session changes the tests. A session record a test
writes for `load` or `update` is a record literal, and names every
field: `{ user: author }` becomes `{ user: author, role: Author }`, or
E0003 says `role` is missing (a default applies at the boundaries, not
to a literal, as the theme switch earlier in this chapter shows). `Test.login`'s record
is a boundary, the cookie's value, so `Test.login { user: Just me }` may
leave `role` out and gets its default, `Reader`.

A page for authors only says so on its `page` line:

```alder fragment
page "/write" { } requires user, role Author
page "/write/drafts" { } requires user, role Author | Admin
```

- `requires user` comes first: a visitor is sent to log in, as always.
- Then the role: a signed-in user whose session's `role` is not one of
  those named gets a 404, the same `App.notFound` page as a missing row,
  so a Reader cannot even tell the page exists. This is checked before
  `load` runs, and before every message of the page, so `load` and
  `update` need no check of their own.
- `role Author | Admin` lets either in.
- Inside the page, `session.role` has the union's type, as everywhere.
- The route checks `requires` and the role. A test of them goes through
  the browser (`Test.open`, `Test.click`, chapter 10); a direct `load` or
  `update` call is not checked.

The rules for the clause (E0027): `role` needs `user` first (`requires
role Author` alone is an error); the session must have a field `role`
(else: "this session has no `role` field"); it must be a union, not
`Maybe`; and each name must be one of its constructors. In the to-do
app, whose roles are `Member` and `Admin`:

```alder error=E0027 "`Author` is not a constructor of this session's `role` type"
module Reports

page "/reports" { } requires user, role Author

type Model = { }

load : Params -> Session -> Query Model
load params session = Query.succeed { }

view : Model -> Session -> Html Msg
view model session = <p>reports</p>
```

**Generated pages say the same.** `alder new page … --requires user`
writes the clause, and `--role R` adds `, role R` (and implies
`--requires user`), each with its test (chapter 15). Every page of
`alder new admin`'s admin `requires user, role Admin`: so it needs a
session `role` with an `Admin` constructor; and when the user table
holds the role, each of its pages also re-reads the user's row in
`load`, the idiom below, so a demoted Admin is turned away at once
(chapter 15, "The dashboard and the layout").

**A role is read at login.** The check reads the session cookie and
nothing else, so it costs no query. A role in the cookie lasts until
logout, or the session's 30-day lifetime from login (visiting does not
renew the cookie: "The session cookie", below). Until then an author
demoted to Reader still passes `requires user, role Author`, and a
Reader promoted to Author must log in again. A page where a demotion
must apply at once also reads the user's row in its `load`, the
**re-read idiom**, as the blog's Write pages do with a helper in `User`
(`examples/blog/app/User.alder`):

```alder fragment
isAuthor : UserId -> Query Bool
isAuthor id = {
  let me <- Db.get User id
  Query.succeed (Maybe.map (\u -> u.role) me == Just Author)
}
```

and, in `Write.Edit`,

```alder fragment
load params session = {
  let author <- User.isAuthor session.user
  if author then {
    let post <- Db.getMine Post params.id
    Query.succeed { post: post }
  } else Query.notFound
}
```

Every message of the page runs `load` first, so its messages are turned
away too: the blog's test "a demoted author is turned away at once"
(chapter 14). The role clause stays: it turns away a Reader promoted
since their login, and keeps a Reader's request from reaching `load`.

### When to use the unscoped forms

`Db.from`, `Db.get`, `Db.update`, `Db.delete` and `Db.insert` are still
there, and still right in these places:

- **Rows that are anyone's to read.** The blog's front page lists
  everyone's published posts with `Db.from Post`.
- **Pages for a role that acts on everyone's rows**, such as an admin
  page `requires user, role Admin` that deletes any post. The role is
  the rule there, and the page says so on its `page` line.
- **Giving a row away**, which `Db.updateMine` never does.

On an owned table outside those cases, an unscoped operation means the
check is yours to write again, and a test is yours to write for it.

A row `load` already found with `Db.getMine` is not one of the cases:
write the scoped form; it costs nothing. `Todos.Show`'s messages carry
no id: they act on `model.todo`, which its `load` read with
`Db.getMine`, and every message runs that `load` first. A plain
`Db.update Todo model.todo.id …` could reach no other row either, but
`Todos.Show` writes `Db.updateMine Todo model.todo.id …` and
`Db.deleteMine Todo model.todo.id`, and so does the blog's
`Write.Edit`. `alder test --mutate` knows that there the two forms are
the same program, and makes no `scope dropped` mutant of such a write
(chapter 10, "What is never a mutant"), so the scoped form asks for no
extra test.

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
  counts the rows it changed. A hand-written check that reads on a
  replica and writes on the primary can approve a write the primary
  would refuse, when the replica is behind or another request changed
  the row in between. With the predicate in the write, the primary
  decides ownership where it writes. (The pattern of `Todos.Show`, a
  `Db.getMine` in `load` and then a write of `model.todo.id`, has that
  gap only if the row changes owner between the two; written with the
  scoped form, as the examples do, it has none.) Chapter 13 shows the
  SQL.

## The request lifecycle

```
GET  /path
  1. decode route parameters      → 404 on failure
  2. check `requires`             → redirect to App.login on failure
     check `role R`, if declared  → 404 when the session's role is not R
  3. load                         → Query, run by the runtime
  4. view (with the session)      → HTML

POST (generated message URL)
  1. decode the page's parameters (`_p.` keys) → 404 on failure
  2. decode the message's parameters           → 404 on failure
  3. decode the form, if any → on failure: requires, load, view with the
     errors, and stop (update is not called)
  4. check `requires`             → redirect to App.login on failure
     check `role R`, if declared  → 404 when the session's role is not R
  5. load                         → Model
  6. update                       → Task, run by the runtime
  7. Rerender: load → view → patch (without JavaScript: 303 to the page)
     Redirect: 303 (under Datastar: the target page, morphed in, and a
     history push; to another site, an event that navigates)
     Reject: load → view with the text on the form → patch
     Task failure: App.error → HTML, status 500 (404 for Query.notFound
     and Task.notFound)
```

## The `App` module

Every app has an `App` module:

```alder
module App

import User
import Layout

session { user: Maybe User.UserId, role: User.Role = Member }

login = Auth.Login

msg Logout

update : Msg -> Session -> Task Nav
update msg session = match msg {
  Logout p -> {
    let _ <- Session.put (\s -> { s | user: Nothing, role: Member })
    Task.succeed (Redirect (Auth.Login {}))
  },
}

error : Error -> Html Never
error err =
  <Layout.Page title="Error">
    <h1>Something went wrong</h1>
    <p>Please try again.</p>
  </Layout.Page>

notFound : Html Never
notFound =
  <Layout.Page title="Not found">
    <h1>Not found</h1>
  </Layout.Page>
```

- `session` declares the session record. The runtime stores it in a signed
  cookie. Field types follow the same rules as form fields. `Session.put`
  (chapter 7) changes it; a `Task` that fails leaves it as it was.
- `login` names the page to redirect to when `requires` fails.
- `error` and `notFound` render failure pages.
- `msg` and `update` declare app-level messages (above). They are optional.

## How a request runs

You can write a whole app without reading this section. It explains what
the server that `alder build` makes (chapter 11) does with each request, so
that nothing in the lifecycle above is a mystery. Chapter 13 follows one
request through it byte by byte.

Four pieces of Go do the work. `rt/web` is the router and the session
cookie. `build/<app>/routes__alder.go`, which the compiler writes for your
app, holds one handler for each page and one for each message. `rt/html`
renders the page, and `rt/datastar` writes the answer a Datastar request
gets.

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
3. The method and path are matched against the generated list `Routes`. A
   page is a `GET` of its pattern; a message is a `POST` to
   `/_m/<Module>/<Msg>`, such as `/_m/Todos.Index/Toggle`. The most specific
   pattern is tried first: segment by segment from the left, a literal
   before a parameter. Anything else is a 404 rendered with `App.notFound`.
4. For a `POST`, the body is read: a url-encoded form is parsed (at most
   1 MiB, else status 413), a multipart body is refused (415), and any
   other body is an empty form. Datastar sends a button's message with a
   JSON body, which Alder ignores.
5. The session cookie is decoded (below).
6. The route's handler runs, and its answer is written.

A Go panic anywhere in the request becomes a 500 page rendered with
`App.error`. The router also puts three tags right after the `<head>` of
every page it writes (on a page without `<head>`, right after `<html>`;
without that, before `</body>`), so your layout stays plain Alder:
`<meta name="color-scheme" content="light dark" />`, which tells the
browser the page has a light and a dark theme, so even its first frame,
painted before the stylesheet arrives, is dark in dark mode; the Datastar
script tag,
`<script type="module" src="/_alder/datastar-v1.0.0.js"></script>`; and
the *boost*, a `<meta name="alder-boost">` whose attributes make the links
of the page work without reloading it ("Links and navigation", below).

### One handler per message

This is the handler for `Toggle`, from `build/todos/routes__alder.go`:

```go
func Route__Todos__Index__Toggle() web.Route {
	return web.Route{Method: "POST", Pattern: "/_m/Todos.Index/Toggle", Handle: func(rq *web.Request) web.Response {
		params, errs := decode__Todos__Index__Params(rt.PageParams(rq.Query))
		if errs != nil {
			return web.NotFoundResponse(html.None())
		}
		p, errs := decode__Todos__Index__ToggleParams(rq.Query)
		if errs != nil {
			return web.NotFoundResponse(html.None())
		}
		return update__Todos__Index(rq, params, Todos__Index__Msg__Toggle__(p), nil)
	}}
}
```

It decodes the page's parameters, then the message's, and answers 404 if
either fails. (`rt.PageParams` picks the page's parameters out of the URL,
as the next part explains; `html.None()` is an empty document, which the
router replaces with `App.notFound`.) Then `update__Todos__Index` does the rest of the lifecycle:
`requires`, `load`, `update`, and the answer to the `Nav`. Every page gets
one `update__<Page>` and one handler per message. A page's `GET` has its
own handler, `Route__<Page>__page`, which decodes the path and query,
checks `requires`, runs `load` and renders `view`.

### Page parameters travel in the message URL

A message must know which page sent it. `Toggle` on `/?filter=Active` has
to load the Active list again, not the full list. So the renderer adds the
page's parameters to every message URL it writes. This is the Done button
of `GET /?filter=Active`, as the running app sends it:

```html
<form method="post" action="/_m/Todos.Index/Toggle?id=Em2Gv4v9BaA0Wf9ujESN&amp;_p.filter=Active" class="_alder-click" data-on:submit="@post(&#39;/_m/Todos.Index/Toggle?id=Em2Gv4v9BaA0Wf9ujESN&amp;_p.filter=Active&#39;, {contentType: &#39;form&#39;})"><al-button variant="secondary" size="sm"><button type="submit">Done</button></al-button></form>
```

(A `click` button is wrapped in a form of its own so that it works without
JavaScript too; "With and without JavaScript", below, explains it.)

`id` is the message's own parameter. `_p.filter` is the page's parameter
`filter`, with `_p.` in front. Your names never contain a `.`, so a page
parameter can never be mistaken for a message parameter of the same name.
A parameter at its default is left out, as in links: on `/`, the same
button posts to `/_m/Todos.Index/Toggle?id=Em2Gv4v9BaA0Wf9ujESN`, and the
handler decodes `filter` as `All`.

A `List` parameter repeats its key, as in links (`rt.QueryKVs` in
`rt/core.go`): a page parameter `tag` holding `["a", "b"]` is
`_p.tag=a&_p.tag=b` in a message URL, and a message parameter is
`tag=a&tag=b`. The handler collects every value of the key.

The handler reads the page's parameters only from the `_p.` keys
(`rt.PageParams` in `rt/core.go`). It does not look at the `Referer`
header. A message URL without the keys a page needs, such as a `Save` for
`/todos/{id}` without `_p.id`, gets a 404.

The URLs of App messages carry the `_p.` keys too
(`/_m/App/Logout?_p.filter=Active`), but `App.update` has no page, so
its handler ignores them.

### The session cookie

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

A union without payloads, such as the `Theme` of "App-level messages"
above, is stored by its constructor's name (`"Theme":"Dark"`), so
reordering the constructors doesn't change what an old cookie means. A field the cookie lacks, or whose value no longer decodes, gets its
default.

The cookie is **signed, not encrypted**. Anyone holding it can read it, so
never put a `Text.Secret`, or anything the user must not see, in the
session. What the signature does is stop anyone from changing it. A cookie
whose signature doesn't match is ignored, and the request sees the default
session: every field at its default, so `user` is `Nothing`.

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

The cookie is `HttpOnly` (page scripts cannot read it), `SameSite=Lax`,
`Path=/`, and lasts 30 days from the time it was issued (its
`Max-Age`, and the `iss` signed into it). It is
`Secure` when the request came over HTTPS, or carries the header
`X-Forwarded-Proto: https` (set by a proxy in front of the server).

The cookie is stored only when the answer **changes** the session: after
an `update` whose `Session.put` ran (even one that wrote the value the
session already had, such as logging in again as the same user) or that
left a different session, whether the answer is a page, a 303 or a
navigation event; and on a 404 or 500 when such an `update` succeeded
and the page then failed to load again (what the `update` committed is
kept). A stored cookie is issued then, for a fresh 30 days. Any other
answer sends no cookie: a page view, a form that fails to decode, a
failed `Task`. The browser keeps the cookie it has, with its issued
time, so visiting does not renew a session: a user who keeps using the
app still logs in again 30 days after logging in. The lifetime is
absolute on purpose. Whatever the login wrote into the session, such as
a role (above), lasts until logout or 30 days after it was written, and
no longer.

### Form errors

When a form fails to decode, the handler does not call `update`. It runs
`requires` and `load`, and renders `view` with the form's state: what was
submitted, and a list of messages per field. The status is 200. Posting
`title=` to the to-do page renders

```html
<ul class="errors" data-errors="title"><li>must not be empty</li></ul>
```

where `<Errors field="title" />` stands. The values and errors go only to
the form that posted the message: the form state names the message, with
its own parameters (`rt.MsgKey`), and the renderer matches it against each
`<form>`'s message, so a per-row form of another row keeps its initial
values. The value of a `Text.Secret` field is never sent back: its input is
a password input, and it comes back empty.

`Reject t` renders the same way, with `t` in `<Errors form />` of the form
that sent the message. Unlike a form error, the `update` that rejected ran
and committed. A message without a form that ends in `Reject` is answered
like `Rerender`, and the text is not shown. So is a `Reject` whose form
the re-rendered page no longer draws (a stale page: the state changed
after it was loaded).

### With and without JavaScript

The browser either runs Datastar (the usual case) or does not. The answers
differ.

With Datastar, a message is sent with the header `Datastar-Request: true`.
For `Rerender`, `Reject`, a form error, a 404 or a 500, the answer is
status 200 with one server-sent event that replaces the page's `<body>`
(`datastar.PatchBody` in `rt/datastar/datastar.go`). Datastar morphs the
new body into the old one, so only what changed is touched. A 404 or 500
is sent that way too, because Datastar ignores answers that are not 2xx.

A `Redirect` (and a `requires` that sends the user to the login page) is
not a 303 under Datastar. Datastar posts with `fetch`, which would follow a
303 by itself: the new page would be morphed into the old one while the
address bar and the history stayed at the old URL. So when the target is
a page of this app (a path starting with `/`), the router fetches it
itself, in the same request: it serves a `GET` of the target, with the
session the message just stored, and answers with that page the way it
answers a link ("Links and navigation", below): the target's `<body>`
morphed in, its `<title>`, and a script that records the new URL in the
browser's history. Logging out of the blog answers (bodies shortened)

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
data: elements <script data-effect="el.remove()">(function(){history.pushState(null,"","/login");…})()</script>

```

with the session cookie in its headers, as a 303 would carry it. The
browser makes one request instead of two, and nothing reloads.

A `Redirect` to another site (an `Url.external` URL) cannot be fetched
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
escaped and it can never end the script. Without Datastar, `Redirect` is
a plain 303. Either way the target must be a path of this site or an
`http:`/`https:` URL: `Url.external` turns anything else (a `javascript:`
URL from user input, say) into `#` (chapter 9), and the router answers a
redirect to such a URL with a 500 rather than send it.

Every line of an event's elements is a `data: elements` line, and an event
ends at a blank line. A carriage return in your data could end a line
early and start a field of its own, so the renderer writes it as `&#13;`
and `PatchBody` turns any line end into a plain newline first.

The two kinds of message travel differently under Datastar. A form posts
with `data-on:submit="@post('<url>', {contentType: 'form'})"`, so its
fields arrive url-encoded and are decoded as the form. A `click` button's
message carries its parameters in the URL, and its body is ignored.

A `click` button works without JavaScript too. Outside a form, it is
rendered inside a form of its own, as the Done button above shows: without
Datastar the browser posts that form to the message URL; with it, Datastar
posts it on submit. The wrapper has the class `_alder-click`; the design system's
stylesheet gives it `display: contents`, so it takes no part in the
layout and the button stays in line (without `al.css`, write that rule
yourself, and keep your own `form` rules off it with
`form:not(._alder-click)`, as the to-do app's `static/app.css` does). The Done button is
an `<al-button>` (Appendix C), which the compiler renders with a real
`<button>` inside it; the wrapping form then goes around the whole
`<al-button>`, so the design system's stylesheet still finds the button
as the element's child. Inside a
`<form submit>`, where forms cannot nest, a `click` button is a submit
button with its own target instead:

```html
<button type="submit" formaction="/_m/App/Reset" formmethod="post" formnovalidate data-on:click__prevent="@post(&#39;/_m/App/Reset&#39;)">Reset</button>
```

Datastar posts the message on click and stops the form's own submission.
Without JavaScript the browser posts the enclosing form's fields to the
button's URL; a message without a form ignores them. (`formnovalidate`
keeps an empty `required` field of the form from blocking the button,
and so does leaving the fields out of the Datastar post: the browser would
otherwise refuse to send a form with an empty required field.) A `click` on another element, such as a `<li>`, needs
JavaScript.

Without JavaScript, the answer to a message is a whole page for a form
error or a `Reject` (they must show what was submitted), and a 303 for
everything else: `Redirect` goes to its page, and `Rerender` goes back to
the page that sent the message, rebuilt from its parameters (`rt.PageAt`
in `rt/core.go`), so the address bar never shows a message URL and a
refresh never posts again (Post/Redirect/Get). Toggling Milk on
`/?filter=Active` without JavaScript answers

```
HTTP/1.1 303 See Other
Location: /?filter=Active
```

and the browser loads the list again.

### App messages

The handler for an App message runs `App.update` with the session and
answers with a redirect (`update__App` in `build/todos/routes__alder.go`),
which the router sends as a 303, or under Datastar as the target page
morphed in, as above:

```go
	if nav.Tag == rt.NavRedirect {
		return web.RedirectResponse(rt.PageURL(nav.Page), rq.Env.Session)
	}
	return web.RedirectResponse(rt.BackURL(rq.HTTP.Referer()), rq.Env.Session)
```

`rt.BackURL` keeps only the path and query of the `Referer`, so the 303
never leads to another site. Clicking "Log out" on `/?filter=Active`
answers a cookie whose `User` is `Nothing` and a redirect to `/login`.
With the theme switch of "App-level messages" (its `update` answers
`Rerender`), clicking "Dark mode" there answers a cookie whose `Theme` is
`"Dark"` and, under Datastar, the list page rendered in the dark theme
with a history push of `/?filter=Active` (without it,
`303 /?filter=Active`).

### Links and navigation

A link in an Alder page is a plain `<a href>`, and without JavaScript a
click on it loads the page it names, as on any site. With Datastar, a
click on a link to a page of this app does not load anything. The boost
in the page's `<head>` catches the click, cancels the browser's own
navigation and asks for the page with Datastar instead, as
`GET /posts/42` with the header `Datastar-Request: true`. The router
answers that request with three events, in this order:

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
pages. Back and Forward work as usual. The boost listens for them too
(the browser's `popstate` event): it asks for the page of the URL the
browser went back to, and the answer replaces the history entry instead
of adding one, and scrolls to where you were on that page.

The answer to a link whose page fails is the same three events with the
`App.notFound` or `App.error` page as the body, and the status in the
header `Alder-Status` (`404` or `500`): the address bar shows the URL you
asked for, as after a page load. A link to a page whose `requires` sends
the visitor to the login page ends on the login page, and its URL is the
one recorded.

The boost leaves these clicks to the browser:

- a link to another site, or to a URL that is not `http:` or `https:`
  (`mailto:`, `tel:`);
- a link with a `#fragment` (a jump within a page);
- a link with a `target` (`target="_blank"` opens a new tab) or a
  `download` attribute;
- a click with Ctrl, Cmd, Shift or Alt held (open in a new tab or
  window), or with a button other than the main one;
- a click that something else on the page already handled (an event
  handler that called `preventDefault`);
- a link with the attribute `data-alder-reload`.

The last one is the way out. Give a link the attribute
`data-alder-reload` when it must load its page for real, such as a link
to a page that needs a fresh `<head>`. A `data-*` attribute takes a
`Text` (chapter 5), and the boost only looks at whether the attribute is
there, so any value will do:

```alder fragment
<a href={Export {}} data-alder-reload="true">Download everything</a>
```

A form that submits with `GET` (a search box, say) is not a link, and is
not boosted: it loads its page as usual.

### Failure pages

A path no route matches, a page parameter that doesn't decode,
`Query.notFound` and `Task.notFound` are answered with `App.notFound`,
status 404. A failed
`Task` is answered with `App.error`, status 500, and logged as an `error`
line naming the request. With `ALDER_DEV=1` (chapter 11), the error's text
is added at the end of the page's `<body>` as an HTML comment, such as
`<!-- error: Db.update Todo: no row with id "nope" -->`. Without it, the
page never shows the text.
