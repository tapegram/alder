# 2. A tour

This chapter tours two apps. The first is the guestbook that `alder new
guestbook` writes (chapter 1, "Your first app"): small enough to read every
file of it. The second is the to-do app in `examples/todos/`, with a login,
three pages and the tests a real app needs. Every construct they use is
explained in the chapters that follow; the aim here is to see how the
pieces fit.

## The guestbook

A visitor signs the guestbook with a name and a message, and the page
lists every entry, newest first. `alder new guestbook` writes these files
(the book's verifier runs `alder new` and compares each file with this
chapter, so what you read here is exactly what you get):

```
guestbook/
  .gitignore                    keeps build/ and the databases out of git
  README.md                     the commands to try
  adapters/README.md            where Go code would go (none yet)
  app/App.alder                 the session and the error pages
  app/Entry.alder               the table
  app/Home.alder                the page at /, with its test
  app/Layout.alder              the HTML around every page
  migrations/0001_initial.sql   the SQL that creates the table
  migrations/schema.json        the tables that SQL leads to
  static/app.css                the stylesheet
```

An Alder app is a directory like this one: `app/` holds the program, one
module per file, and the module `Home` is the file `app/Home.alder`
(`Todos.Index` would be `app/Todos/Index.alder`).

### The table

`app/Entry.alder` declares the one table, and the type of its ids:

```alder part=guestbook/Entry
module Entry

type EntryId = brand Text

table Entry {
  id: EntryId @id,
  name: Text.NonEmpty,
  message: Text.NonEmpty,
  created: Time @created,
}
```

A `table` is a record type and a database table at once. Each field is a
column: `name` and `message` are `Text.NonEmpty`, a text that cannot be
empty, so no row and no form can hold an empty one. `@id` marks the id,
which the runtime makes for each new row; `@created` marks a time the
runtime fills in when the row is inserted. You never write either.

`EntryId` is a **brand**: at runtime a `Text`, but a type of its own, so an
entry's id cannot be mixed up with any other text (or another table's id).

### The page

`app/Home.alder` is the whole page: its address, what it reads, the form
it accepts, what it does with the form, and its HTML. Here it is from the
top, piece by piece.

```alder part=guestbook/Home
module Home

import Entry
import Layout

page "/" { }
```

`import` makes another module's names available: `Entry` for the table,
`Layout` for the HTML around the page. `page "/" { }` makes this module a
page, served at `/`; the braces would list the address's parameters, and
this page has none.

```alder part=guestbook/Home
type Model = { entries: List Entry }

load : Params -> Session -> Query Model
load params session = {
  let entries <- Db.from Entry |> Db.orderBy Entry.created.desc |> Db.all
  Query.succeed { entries: entries }
}
```

`Model` is what the page shows. `load` builds it for every request: it
reads every entry, newest first (`Entry.created.desc`), and succeeds with
them. It returns a `Query`, a description of reads that the runtime
carries out, so `load` cannot write anything. `let entries <- …` names
the result of a read. (`params` would be the page's parameters and
`session` the visitor's session; this page uses neither.)

```alder part=guestbook/Home
form Sign = {
  name: Text.NonEmpty,
  message: Text.NonEmpty,
}

msg Submit form Sign
```

`form Sign` is the form's fields and their types. `msg Submit form Sign`
declares a **message**: a POST that this page's HTML can send, carrying a
`Sign`. Before `Submit` reaches any of your code, the runtime checks the
posted fields against the types: an empty name sends the page back with
the error next to the field.

```alder part=guestbook/Home
update : Msg -> Model -> Session -> Task Nav
update msg model session = match msg {
  Submit p -> {
    let _ <- Db.insert Entry { name: p.form.name, message: p.form.message }
    Task.succeed (Redirect (Home {}))
  },
}
```

`update` handles each message. It returns a `Task`, a description of
writes ending in where the browser goes next. `Submit` inserts the entry
(`id` and `created` are filled in, so they are not written here) and
redirects to the page itself, `Home {}`, so that reloading the page does
not post the form twice. The `match` must handle every message of the
page, or the module does not compile.

```alder part=guestbook/Home
view : Model -> Session -> Html Msg
view model session =
  <Layout.Page title="Guestbook">
    <h1>Guestbook</h1>

    <al-card>
      <h2 slot="header">Sign the guestbook</h2>
      <form submit={Submit}>
        <al-input field="name" label="Your name" />
        <al-input field="message" label="Message" />
        <al-button type="submit">Sign</al-button>
      </form>
    </al-card>

    <ul id="entries">
      {for e in model.entries -> <li><b>{e.name}</b>: {e.message}</li>}
    </ul>
  </Layout.Page>
```

`view` turns the model into HTML, with expressions in braces.
`<form submit={Submit}>` ties the form to its message; `<al-input
field="name">` draws the labelled input for the field `name` of `Sign`
(a field the form does not have is a compile error), with the list where
its error appears. `<al-card>`, `<al-input>` and `<al-button>` are
elements of `alder-ui`, the design system every Alder app has (Appendix
C): a card with a header, a field, and a button, which Alder renders as a
real `<button type="submit">` inside the element, so the form posts
without JavaScript. `{for e in model.entries -> …}` makes one `<li>` per
entry. `<Layout.Page title="Guestbook">` is a component from `Layout`:
everything inside it becomes its `children`.

```alder part=guestbook/Home
test "signing adds an entry" = {
  let _ <- Test.get (Home {})
  let signed <- Test.submit Sign { name: Text.NonEmpty "Ada", message: Text.NonEmpty "Hello" }
  let page <- Test.get (Home {})
  Task.succeed ({ answer: signed.answer, entries: Test.count "#entries li" page.doc } == { answer: Redirected (Home {}), entries: 1 })
}
```

The page ends with its two tests. The first goes through the page the way a person
would, in an in-memory browser: it opens `/` (`Test.get`), fills in and
posts the form (`Test.submit Sign { … }`), and opens the page again. The
answer to the post must be the redirect `update` makes, and the page must
then list one entry: `Test.count "#entries li"` counts the `<li>` inside
`<ul id="entries">`. Each test runs against a fresh, empty database, so
the one entry is the one the test signed. Chapter 10 explains every
`Test.` function.

```alder part=guestbook/Home
test "entries are listed newest first" = {
  let _ <- Test.get (Home {})
  let _ <- Test.submit Sign { name: Text.NonEmpty "Ada", message: Text.NonEmpty "First" }
  let _ <- Test.submit Sign { name: Text.NonEmpty "Grace", message: Text.NonEmpty "Second" }
  let page <- Test.get (Home {})
  Task.succeed (Test.texts "#entries b" page.doc == ["Grace", "Ada"])
}
```

The second checks the order: it signs twice and reads the names in the
order the page lists them (`Test.texts "#entries b"` is the text of every
`<b>` inside the list), newest first. The first test cannot tell: one
entry is in order either way. "Running it" below shows how you find out
that such a test is needed.

### The layout and the app

`app/Layout.alder` is the HTML every page shares, as a component: a
function from a record of properties to HTML.

```alder part=guestbook/Layout
module Layout

Page : { title: Text, children: List (Html a) } -> Html a
Page props =
  <html>
    <head>
      <meta charset="utf-8" />
      <title>{props.title}</title>
      <link rel="stylesheet" href={Url.external "/static/al/al.css"} />
      <link rel="stylesheet" href={Url.external "/static/app.css"} />
    </head>
    <body>
      <al-page width="narrow">
        <main>{props.children}</main>
      </al-page>
    </body>
  </html>
```

`children` is the HTML written between `<Layout.Page …>` and
`</Layout.Page>`. `Url.external` marks a link that is not one of the
app's pages: a link to a page is a page value (`Home {}`), checked by the
compiler, never a string. The first stylesheet is the design system's,
which every build serves at `/static/al/al.css`; `<al-page
width="narrow">` is its centred column.

`app/App.alder` is the one module every app has: it declares the session,
what the server keeps for a visitor between requests (this app keeps
nothing: `session { }`; the to-do app below keeps who is logged in), and
the two pages the runtime shows when something goes wrong, which use
`Layout.Page` too, so they look like the rest of the app.

```alder part=guestbook/App
module App

import Layout

session { }

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

`error` is shown when a request fails, and `notFound` for an address no
page has. Their type, `Html Never`, says that they send no messages: they
have no buttons and no forms.

### The other files

`static/app.css` is served at `/static/app.css`, as are any other files
you put in `static/`. It holds the app's own rules; the design system's
stylesheet does the rest:

```css file=guestbook/static/app.css
/* The guestbook's own rules, served at /static/app.css. The design system,
   /static/al/al.css, styles the page, the card, the fields and the button.
   form._alder-click is the form Alder wraps round a click button: not a grid. */
form:not(._alder-click) { display: grid; gap: 1rem; }
#entries { padding-left: 1.25rem; }
li { margin: 0.25rem 0; }
```

`adapters/` is for Go code. A **capability** is something Alder cannot do
by itself, such as sending an email or checking a password; the app
declares it, and a Go **adapter** implements it (chapter 7). The guestbook
needs none, so the directory holds only this README:

```markdown file=guestbook/adapters/README.md
# adapters

The Go that implements the app's capabilities goes here, one package per capability's module.
A capability `X` declared in module `A.B` is implemented by the package in `adapters/a/` (the
module's first segment, lower-cased), which exports `func BX() app.Cap__A__B__X`; a module of
one segment drops the `B`, so `Markdown`'s `Markdown` is `func Markdown()` in
`adapters/markdown/`. This app declares no capability yet, so nothing goes here; when one is
missing, `alder build` prints the exact directory and function to write (the Alder book,
chapter 7, "Implementing a capability").
```

`migrations/` is how the database gets its tables. A table never creates
itself in production: a **migration**, a file of SQL, does, and the app
applies the ones it has not applied yet when it starts. `alder new` ran
the migration generator, `alder migrate` (chapter 8), on the new app, so
the first migration is already there:

```sql file=guestbook/migrations/0001_initial.sql
-- alder migration 0001_initial
-- from e3b0c44298fc
-- to 2e82b15d5f85
-- step: create table Entry

CREATE TABLE "Entry__Entry" ("id" TEXT NOT NULL PRIMARY KEY, "name" TEXT NOT NULL, "message" TEXT NOT NULL, "created" TEXT NOT NULL) STRICT;
```

The header says which tables the database had before (`from`, here none)
and has after (`to`), each as the start of a hash of the tables' `CREATE
TABLE` statements, sorted. `schema.json` records the tables as the last
migration left them, so the next migration's `from` can be worked out
from it:

```json file=guestbook/migrations/schema.json
{
  "format": "1",
  "tables": [
    {
      "name": "Entry.Entry",
      "id": "id",
      "created": "created",
      "columns": [
        {
          "name": "id",
          "type": "Text"
        },
        {
          "name": "name",
          "type": "Text"
        },
        {
          "name": "message",
          "type": "Text"
        },
        {
          "name": "created",
          "type": "Time"
        }
      ]
    }
  ]
}
```

When you change a table, `alder check` says that the tables no longer
match the last migration, and `alder migrate` writes the next one.

`.gitignore` keeps out of git what is not source: `build/`, where
`alder build` writes the server and `alder run` its development database,
and the SQLite files a server started from the app's directory keeps
there:

```text file=guestbook/.gitignore
# What alder writes, not source: build/ (the server, and alder run's database build/<name>/dev.db)
# and the SQLite files a server started from this directory keeps here (<name>.db and its
# -wal and -shm files).
build/
*.db
*.db-wal
*.db-shm
```

The README repeats the commands, and names the next ones, the
generators of chapter 15:

```markdown file=guestbook/README.md
# guestbook

A guestbook: a visitor signs it with a name and a message, and the page lists every entry,
newest first. It was made by `alder new guestbook`, and chapter 2 of the Alder book walks
through every file.

Run these from this directory:

    alder check            # type-check the app
    alder test             # run its tests
    alder test --mutate    # measure the tests: which changes to the app do they miss?
    alder build            # build the server, build/guestbook/guestbook
    alder run              # build, serve on http://localhost:8080, rebuild on every save

Then grow it from the command line. Each of these writes plain Alder with its tests, and only
if the app still checks (chapter 15):

    alder new page About /about                    # a page
    alder new table Reply entry:Entry.EntryId@ref=Entry@cascade message:Text.NonEmpty
    alder new page Replies /replies --list Reply   # a page of a table's rows
    alder new admin                                # an admin of every table, at /admin

`alder new admin` needs someone to let in: a session with a logged-in `user` and a `role` whose
type has an `Admin` constructor (`role: User.Role = Member`, with `type Role = Member | Admin`),
set at login. The guestbook has no login yet; chapter 6, "Rules", shows what to add.

- `app/`: the Alder modules, one per file (`Home` is the page at `/`, `Entry` its table).
- `migrations/`: the database's migrations, written by `alder migrate` after a change to a table.
- `static/`: files served as they are, at `/static/…`.
- `adapters/`: the Go behind the app's capabilities (none yet).
- `build/`: what `alder build` writes; not source (`.gitignore` keeps it, and the databases, out of git).
```

### Running it

From the app's directory, `alder check` type-checks every module, and
says nothing when there is nothing to say:

```
$ cd guestbook
$ alder check
$ alder test
ok   Home "signing adds an entry"
ok   Home "entries are listed newest first"
2 passed, 0 failed
```

On a machine that has never built an Alder app, the first `alder test`
starts with the line `fetching modernc.org/sqlite (first build on this
machine)` and takes longer, around 15 seconds (chapter 1, "Installing");
after that it takes a second or two.

`alder test --mutate` measures the tests. It makes small changes to the
program, called **mutants**, each one a plausible mistake (here: the
insert replaced by nothing, the redirect turned into a re-render, the
order reversed), and runs the
tests against each. A mutant the tests notice is killed; one they do not
notice survived, and each survivor names a test that is missing:

```
$ alder test --mutate
ok   Home "signing adds an entry"
ok   Home "entries are listed newest first"
2 passed, 0 failed
3 mutants: 3 killed, 0 survived  (score 1.00)
```

Without the second test, the order reversed (`Entry.created.desc`
changed to `.asc`) would survive, and the run would end `3 mutants: 2
killed, 1 survived  (score 0.67)` with that survivor named under
`survived:`. Chapter 10, "Mutation testing", explains the rest.

`alder build` writes the server, one executable with SQLite inside it, and
`alder run` builds it, starts it in development mode on
`http://localhost:8080`, and rebuilds it each time you save a file:

```
$ alder build
wrote build/guestbook/guestbook
$ alder run
alder run: building guestbook
info: migrated 0001_initial
warn: ALDER_SECRET is not set (or shorter than 16 bytes); dev mode signs sessions with the public development cookie key
dev server on 127.0.0.1:8080 → app on 127.0.0.1:56970
```

(The server's log lines also carry the date and time; they are left out
here.) The first start applies `0001_initial` to the development database,
`build/guestbook/dev.db`. Sign the guestbook in the browser and the log
shows each effect the requests ran: `Db.all Entry` for the page; for the
post, `Db.all Entry` again (the runtime loads the page's model before it
calls `update`) and `Db.insert Entry`; then `Db.all Entry` for the page
the post redirects to. Chapter 11 explains each command; the warning is about
production, where `ALDER_SECRET` must be set (chapter 6).

## The to-do app

The full source of this app is in `examples/todos/`. It has what the
guestbook does not: a login, a table that refers to another, a page with
parameters, and messages that change or delete a row.

### The data

`app/Todo.alder` declares the table and its id type:

```alder
module Todo

import User

type TodoId = brand Text

type Filter = All | Active | Completed

-- `owner` is the table's `@owner`: `Db.mine Todo`, `Db.getMine`, `Db.updateMine`, `Db.deleteMine`
-- and `Db.insertMine` read and write only the signed-in user's todos (book 06, "Rules").
table Todo {
  id: TodoId @id,
  title: Text.NonEmpty,
  done: Bool = False,
  due: Maybe Date,
  owner: User.UserId @ref User @owner,
  created: Time @created,
}

matches : Filter -> Todo -> Bool
matches filter todo = match filter {
  All -> True,
  Active -> not todo.done,
  Completed -> todo.done,
}

filterLabel : Filter -> Text
filterLabel filter = match filter {
  All -> "All",
  Active -> "Active",
  Completed -> "Completed",
}

countOpen : List Todo -> Int
countOpen todos = todos |> List.filter (\t -> not t.done) |> List.length
```

(The file ends with a test of `countOpen`; chapter 10.)

`TodoId` is a brand: at runtime it is a `Text`, and it can be used as one,
but only this module can make one from a plain `Text`. Everywhere else, a
`TodoId` comes from a route, a form, or a database row, so it is always a
real id. `title: Text.NonEmpty` means an empty title can't exist in the
table, and the form that creates todos will refuse one before any code runs.
`owner: User.UserId @ref User` says each todo belongs to a user, and
`@owner` makes that the rule of the table: the pages read and write todos
through `Db`'s scoped operations, which act only on the signed-in user's
rows (below; chapter 6, "Rules").

### The page

`app/Todos/Index.alder` is the list page.

```alder part=Todos.Index
module Todos.Index

import Todo
import Layout
import User

page "/" { filter: Todo.Filter = All } requires user
```

`User` is imported for the tests at the end of the module.
`page` declares the route. `filter` is a query parameter with a default, so
both `/` and `/?filter=Active` work and `filter` is never missing.
`requires user` means the `user` field of the session must be `Just`;
otherwise the runtime sends the visitor to the login page, and inside this
module `session.user` is a `UserId`, not a `Maybe UserId`.

```alder part=Todos.Index
type Model = {
  filter: Todo.Filter,
  todos: List Todo,
}

load : Params -> Session -> Query Model
load params session = {
  let todos <-
    Db.mine Todo
      |> Db.orderBy Todo.created.desc
      |> Db.all
  Query.succeed { filter: params.filter, todos: List.filter (Todo.matches params.filter) todos }
}
```

`load` builds the Model for one request. It returns a `Query`: a description
of reads that the runtime performs. `let todos <- ...` binds the result of a
read. Because it is a `Query`, nothing in `load` can write. `Db.mine Todo`
is the todos whose `owner` is the session's user, newest first here: the
page can only ever list the user's own.

```alder part=Todos.Index
form NewTodo = {
  title: Text.NonEmpty,
  due: Maybe Date,
}

msg Submit form NewTodo
msg Toggle { id: Todo.TodoId }
msg Delete { id: Todo.TodoId }
```

Each `msg` is an action this page can receive; the compiler gives it a POST
URL. `Submit` carries a form; the other two carry an id. Together they make up this page's `Msg` type.

```alder part=Todos.Index
update : Msg -> Model -> Session -> Task Nav
update msg model session = match msg {
  Submit p -> {
    let _ <- Db.insertMine Todo { title: p.form.title, due: p.form.due }
    Task.succeed (Redirect (Todos.Index { filter: model.filter }))
  },
  -- A message's id comes from the request: anyone can post `Toggle` with another user's id.
  -- `Db.updateMine` and `Db.deleteMine` change the row only when it is the session's user's;
  -- any other id, like a missing one, is a 404 (book 06, "Rules").
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

`update` returns a `Task`: a description of writes, ending in where to go
next. `Rerender` re-runs `load` and `view` and patches the page in place.
`Redirect` is a real HTTP redirect, used after a create so a refresh doesn't
resubmit. The `match` must handle every `msg`, or the module won't compile.
`Db.insertMine` stores the new todo with the session's user as its
`owner` (the record does not name `owner`; it may not). `Db.updateMine` and
`Db.deleteMine` act on the todo a message names only when it is the
user's: a message's id comes from the request, so `update` must not trust
it, and a todo that is someone else's answers exactly as a missing one
does, with a 404 (chapter 6, "Rules").

```alder part=Todos.Index
view : Model -> Session -> Html Msg
view model session =
  <Layout.Page title="Todos">
    <h1>Todos</h1>

    <al-card>
      <form submit={Submit}>
        <al-input field="title" label="What needs doing?" />
        <al-input field="due" label="Due" />
        <al-button type="submit">Add</al-button>
      </form>
    </al-card>

    <nav>
      {for f in [All, Active, Completed] ->
        <a href={Todos.Index { filter: f }} aria-current={if model.filter == f then Just "page" else Nothing}>
          {Todo.filterLabel f}
        </a>}
    </nav>

    <ul id="todos">
      {for todo in model.todos -> <Item todo={todo} />}
    </ul>

    <p id="summary">{Text.fromInt (Todo.countOpen model.todos)} open</p>

    <al-button variant="outline" click={App.Logout}>Log out</al-button>
  </Layout.Page>
```

`view` is HTML with expressions in braces. It also receives the session, which this page does not need (a page that shows a per-user setting kept in the session would; chapter 6's theme is one). `App.Logout` is an app-level message, which any page may send (chapter 6). `href` takes a page value, never
a string, so a link to a page that doesn't exist, or with the wrong
parameters, is a compile error. `submit={Submit}` ties the form to its
message, and `<al-input field="title">` is checked against `NewTodo`. If the
submitted form is invalid, the runtime re-renders this page with the errors
in place, and `update` is never called. The `al-` elements are the design
system's, as in the guestbook (Appendix C).

```alder part=Todos.Index
Item : { todo: Todo } -> Html Msg
Item props =
  <li class={if props.todo.done then "done" else ""}>
    <al-button variant="secondary" size="sm" click={Toggle { id: props.todo.id }}>{if props.todo.done then "Undo" else "Done"}</al-button>
    <a href={Todos.Show { id: props.todo.id }}>{props.todo.title}</a>
    {match props.todo.due {
      Just d -> <al-badge variant="outline" class="due">{Date.format "Jan 2" d}</al-badge>,
      Nothing -> Html.none,
    }}
    <al-button variant="ghost" size="sm" click={Delete { id: props.todo.id }}>Delete</al-button>
  </li>
```

`Item` is a component: a function from a record to HTML. A todo without a
due date shows nothing there: `Html.none` renders nothing; one with a date
shows it in an `<al-badge>`. `click={Toggle { id: ... }}` sends a message;
it compiles to a POST (and, with Datastar, an in-place patch of the page;
chapter 6, "How a request runs"). On an `<al-button>` it is a real
`<button>` inside the element, which posts without JavaScript too.

The module ends with its tests. The first inserts a user and a todo into
a fresh, empty database, logs in as that user, sends `update` a `Toggle`,
and checks what happened: the trace is the one scoped write, and the todo
is done. The next two are a stranger's attempt: my list is still open in
the browser when a stranger logs in on it and presses my todo's buttons.
Each answer is a 404 (`PageMissing`), and the todo is as it was. Chapter
10 explains them line by line.

```alder part=Todos.Index
test "Toggle flips done" = {
  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
  let id <- Db.insert Todo { title: Text.NonEmpty "Milk", due: Nothing, owner: me }
  let _ <- Test.login { user: Just me }
  let r <- Test.run (update (Toggle { id: id }) { filter: All, todos: [] } { user: me, role: Member })
  let after <- Db.get Todo id
  let done = Maybe.map (\t -> t.done) after
  Task.succeed ({ trace: r.trace, done: done } == { trace: ["Db.updateMine Todo"], done: Just True })
}

-- A stranger's own page lists none of my todos, but a message's id comes from the request.
-- Here my page is still open in the browser when a stranger logs in on it and presses my todo's
-- Toggle: `Db.updateMine` finds no todo of the stranger's with that id, so the answer is a 404
-- and nothing changes.
test "Toggle by another user is not found" = {
  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
  let id <- Db.insert Todo { title: Text.NonEmpty "Milk", due: Nothing, owner: me }
  let _ <- Test.login { user: Just me }
  let _ <- Test.get (Todos.Index {})
  let _ <- Test.login { user: Just (User.UserId "someone-else") }
  let r <- Test.click (Toggle { id: id })
  let after <- Db.get Todo id
  Task.succeed ({ answer: r.answer, done: Maybe.map (\t -> t.done) after } == { answer: PageMissing, done: Just False })
}

test "Delete by another user is not found" = {
  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
  let id <- Db.insert Todo { title: Text.NonEmpty "Milk", due: Nothing, owner: me }
  let _ <- Test.login { user: Just me }
  let _ <- Test.get (Todos.Index {})
  let _ <- Test.login { user: Just (User.UserId "someone-else") }
  let r <- Test.click (Delete { id: id })
  let after <- Db.get Todo id
  Task.succeed ({ answer: r.answer, todo: Maybe.map (\t -> t.title) after } == { answer: PageMissing, todo: Just (Text.NonEmpty "Milk") })
}
```

The next three tests go through the page itself, as a user would: they log
in, open the page, post its form, click its buttons, and read what the page
shows (`#summary` is the `<p id="summary">` above). The form posted by
`Test.submit NewTodo` and the button clicked by `Test.click (Toggle { id:
id })` must be on the page the test holds, so a stranger who opens the list
afresh, whose page lists none of my todos, has nothing to click; the todo's
own page is a 404 for them (`PageMissing`). Chapter 10, "Testing through the
browser", explains each step.

```alder part=Todos.Index
-- The same page through the browser: the test logs in, opens the page, and uses the form and
-- the buttons the page renders, as a user would (book 10, "Testing through the browser").
test "adding a todo through the page" = {
  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
  let _ <- Test.login { user: Just me }
  let _ <- Test.get (Todos.Index {})
  let added <- Test.submit NewTodo { title: Text.NonEmpty "Milk", due: Nothing }
  let page <- Test.get (Todos.Index {})
  Task.succeed ({ answer: added.answer, todos: List.map (\t -> t.title) page.model.todos, summary: Test.text "#summary" page.doc } == { answer: Redirected (Todos.Index {}), todos: [Text.NonEmpty "Milk"], summary: Just "1 open" })
}

test "toggling a todo through the page" = {
  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
  let id <- Db.insert Todo { title: Text.NonEmpty "Milk", due: Nothing, owner: me }
  let _ <- Test.login { user: Just me }
  let _ <- Test.get (Todos.Index {})
  let r <- Test.click (Toggle { id: id })
  Task.succeed ({ answer: r.answer, done: Test.has "li.done" r.doc, summary: Test.text "#summary" r.doc } == { answer: Rerendered, done: True, summary: Just "0 open" })
}

-- A stranger's page lists none of my todos, so it has no Toggle button to click for them; the
-- todo's own page is a 404 for the stranger.
test "a stranger cannot see my todo" = {
  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
  let id <- Db.insert Todo { title: Text.NonEmpty "Milk", due: Nothing, owner: me }
  let _ <- Test.login { user: Just (User.UserId "someone-else") }
  let page <- Test.get (Todos.Index {})
  let show <- Test.open (Todos.Show { id: id })
  Task.succeed ({ items: Test.count "li" page.doc, show: show.answer } == { items: 0, show: PageMissing })
}
```

The next two were written after `alder test --mutate` (chapter 10,
"Mutation testing") showed that nothing checked them: that `Delete`
really deletes, and that the newest todo comes first. `Test.texts "li a"`
is the text of every link inside a list item, in page order: the titles.
The last shows the scoped read on its own: a test may use `Db.mine` too,
and it acts for the user `Test.login` signed in, so it counts Me's two
todos and not the other user's one.

```alder part=Todos.Index
test "Delete removes the todo" = {
  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
  let id <- Db.insert Todo { title: Text.NonEmpty "Milk", due: Nothing, owner: me }
  let _ <- Test.login { user: Just me }
  let r <- Test.run (update (Delete { id: id }) { filter: All, todos: [] } { user: me, role: Member })
  let after <- Db.get Todo id
  Task.succeed ({ value: r.value, todo: Maybe.map (\t -> t.title) after } == { value: Ok Rerender, todo: Nothing })
}

test "the newest todo is listed first" = {
  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
  let _ <- Db.insert Todo { title: Text.NonEmpty "First", due: Nothing, owner: me }
  let _ <- Db.insert Todo { title: Text.NonEmpty "Second", due: Nothing, owner: me }
  let _ <- Test.login { user: Just me }
  let page <- Test.get (Todos.Index {})
  Task.succeed (Test.texts "li a" page.doc == ["Second", "First"])
}

-- A test may use the scoped operations itself: they act for the test's session, which
-- `Test.login` sets. `Db.mine Todo` is what Me sees, and not the other user's todo.
test "Db.mine is what Me sees" = {
  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
  let other <- Db.insert User { email: Text.Email "other@example.com", name: Text.NonEmpty "Other", passwordHash: "" }
  let _ <- Db.insert Todo { title: Text.NonEmpty "Milk", due: Nothing, owner: me }
  let _ <- Db.insert Todo { title: Text.NonEmpty "Bread", due: Nothing, owner: me }
  let _ <- Db.insert Todo { title: Text.NonEmpty "Eggs", due: Nothing, owner: other }
  let _ <- Test.login { user: Just me }
  let r <- Test.run {
    let n <- Db.mine Todo |> Db.count
    Task.succeed n
  }
  Task.succeed ({ value: r.value, trace: r.trace } == { value: Ok 2, trace: ["Db.mine Todo", "Db.count Todo"] })
}
```

### What the compiler checked

Reading this module, the compiler verified that: every `href` points at a
real page with the right parameters; every `click` and `submit` is a message
of this page; every message has an `update` arm; the form's fields match
the `NewTodo` type; `load` performs no writes; `session.user` is always
present; `title` is never empty; every `match` is complete; and the
scoped operations are used only on a table with an `@owner`, in a page
that `requires user`. None of that needed a test.

### The rest of the app

- `app/Todos/Show.alder`: a detail page with an edit form. Its `load`
  reads the todo with `Db.getMine`, which answers 404 when the todo
  doesn't exist or belongs to someone else.
  Its tests send a visitor to log in, save the form and delete the todo
  through the page.
- `app/Auth/Login.alder`: the login page. It declares a `Password`
  capability, implemented in Go, and uses `Session.put` to sign the user
  in, with the role their row has. A wrong password returns `Reject`,
  which re-renders the form with a message. Beside the capability is a
  `fake Password` for tests, and four tests that log in and out through
  the form and the button.
- `app/App.alder`: the session record (the user, and their role: a
  `Member`, or an `Admin`), the error pages, and the app-level `Logout`
  message, which any page can send with `click={App.Logout}`, with two
  tests of it.
- `app/User.alder`, `app/Layout.alder`: the user table, with its `role`
  column, and a page layout component.
- `app/Admin/`: an admin for every table, for a user whose role is
  `Admin`, which one command wrote: `alder new admin` (chapter 15).

A second example, `examples/blog`, adds what this app never needed: a
second table that refers to the first, a user role, a capability called
from `load`, pages of results and a search. Chapter 14 tours it the same
way, after the reference chapters.
