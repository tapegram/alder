# 2. A tour

The full source of this app is in `examples/todos/`. This chapter walks
through it. Every construct it uses is explained in the chapters that
follow; the aim here is to see how the pieces fit.

## The data

`app/Todo.alder` declares the table and its id type:

```alder
module Todo

import User

type TodoId = brand Text

type Filter = All | Active | Completed

table Todo {
  id: TodoId @id,
  title: Text.NonEmpty,
  done: Bool = False,
  due: Maybe Date,
  owner: User.UserId @ref User,
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

## The page

`app/Todos/Index.alder` is the list page.

```alder part=Todos.Index
module Todos.Index

import Todo
import Layout

page "/" { filter: Todo.Filter = All } requires user
```

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
    Db.from Todo
      |> Db.where (Todo.owner.eq session.user)
      |> Db.orderBy Todo.created.desc
      |> Db.all
  Query.succeed { filter: params.filter, todos: List.filter (Todo.matches params.filter) todos }
}
```

`load` builds the Model for one request. It returns a `Query`: a description
of reads that the runtime performs. `let todos <- ...` binds the result of a
read. Because it is a `Query`, nothing in `load` can write.

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
    let _ <- Db.insert Todo { title: p.form.title, due: p.form.due, owner: session.user }
    Task.succeed (Redirect (Todos.Index { filter: model.filter }))
  },
  Toggle p -> {
    let todo <- owned p.id session
    let _ <- Db.update Todo todo.id (\t -> { t | done: not t.done })
    Task.succeed Rerender
  },
  Delete p -> {
    let todo <- owned p.id session
    let _ <- Db.delete Todo todo.id
    Task.succeed Rerender
  },
}

-- owned is the todo a message names, when the session's user owns it; otherwise the message is
-- a 404. `load` only lists the user's todos, but a message's id comes from the request: anyone
-- can post `Toggle` with another user's id, so `update` checks it again.
owned : Todo.TodoId -> Session -> Task Todo
owned id session = {
  let found <- Db.get Todo id
  match found {
    Just todo -> if todo.owner == session.user then Task.succeed todo else Task.notFound,
    Nothing -> Task.notFound,
  }
}
```

`update` returns a `Task`: a description of writes, ending in where to go
next. `Rerender` re-runs `load` and `view` and patches the page in place.
`Redirect` is a real HTTP redirect, used after a create so a refresh doesn't
resubmit. The `match` must handle every `msg`, or the module won't compile.
`owned` checks that the todo a message names belongs to the user before
`Toggle` or `Delete` touches it: a message's id comes from the request, so
`update` must not trust it (`Task.notFound` answers 404; chapter 6,
"Authorize in `update` too").

```alder part=Todos.Index
view : Model -> Session -> Html Msg
view model session =
  <Layout.Page title="Todos" theme={session.theme}>
    <h1>Todos</h1>

    <form submit={Submit}>
      <Input field="title" label="What needs doing?" />
      <Errors field="title" />
      <Input field="due" label="Due" />
      <Errors field="due" />
      <button>Add</button>
    </form>

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

    <button click={App.Logout}>Log out</button>
  </Layout.Page>
```

`view` is HTML with expressions in braces. It also receives the session, used here for the theme. `App.Logout` is an app-level message, which any page may send (chapter 6). `href` takes a page value, never
a string, so a link to a page that doesn't exist, or with the wrong
parameters, is a compile error. `submit={Submit}` ties the form to its
message, and `<Input field="title">` is checked against `NewTodo`. If the
submitted form is invalid, the runtime re-renders this page with the errors
in place, and `update` is never called.

```alder part=Todos.Index
Item : { todo: Todo } -> Html Msg
Item props =
  <li class={if props.todo.done then "done" else ""}>
    <button click={Toggle { id: props.todo.id }}>{if props.todo.done then "Undo" else "Done"}</button>
    <a href={Todos.Show { id: props.todo.id }}>{props.todo.title}</a>
    {match props.todo.due {
      Just d -> <span class="due">{Date.format "Jan 2" d}</span>,
      Nothing -> Html.none,
    }}
    <button click={Delete { id: props.todo.id }}>Delete</button>
  </li>
```

`Item` is a component: a function from a record to HTML. A todo without a
due date shows nothing there: `Html.none` renders nothing. `click={Toggle
{ id: ... }}` sends a message; it compiles to a POST (and, with Datastar, an
in-place patch of the page; chapter 6, "How a request runs").

## What the compiler checked

Reading this module, the compiler verified that: every `href` points at a
real page with the right parameters; every `click` and `submit` is a message
of this page; every message has an `update` arm; the form's fields match
the `NewTodo` type; `load` performs no writes; `session.user` is always
present; `title` is never empty; and every `match` is complete. None of
that needed a test.

## The rest of the app

- `app/Todos/Show.alder`: a detail page with an edit form. It uses
  `Query.notFound` when the todo doesn't exist or belongs to someone else.
- `app/Auth/Login.alder`: the login page. It declares a `Password`
  capability, implemented in Go, and uses `Session.put` to sign the user
  in. A wrong password returns `Reject`, which re-renders the form with a
  message.
- `app/App.alder`: the session record, the error pages, and the app-level
  `Logout` message, which any page can send with `click={App.Logout}`.
- `app/User.alder`, `app/Layout.alder`: the user table and a page layout
  component.
