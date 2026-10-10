--- file: app/App.alder ---
module App

import User
import Layout
import Theme

session { user: Maybe User.UserId, theme: Theme = Light }

login = Auth.Login

msg Logout
msg SwitchTheme

update : Msg -> Session -> Task Nav
update msg session = match msg {
  Logout p -> {
    let _ <- Session.put (\s -> { s | user: Nothing })
    Task.succeed (Redirect (Auth.Login {}))
  },
  SwitchTheme p -> {
    let _ <- Session.put (\s -> { s | theme: Theme.flip s.theme })
    Task.succeed Rerender
  },
}

error : Error -> Html Never
error err =
  <Layout.Page title="Error" theme={Light}>
    <h1>Something went wrong</h1>
    <p>Please try again.</p>
  </Layout.Page>

notFound : Html Never
notFound =
  <Layout.Page title="Not found" theme={Light}>
    <h1>Not found</h1>
  </Layout.Page>

--- file: app/Auth/Login.alder ---
module Auth.Login

import User
import Layout

-- Password checking is outside the language: a capability implemented in Go.
capability Password {
  verify : { email: Text.Email, password: Text } -> Task (Maybe User.UserId),
}

page "/login" { }

type Model = { }

load : Params -> Session -> Query Model
load params session = Query.succeed { }

form Credentials = {
  email: Text.Email,
  password: Text.Secret,
}

msg Submit form Credentials

update : Msg -> Model -> Session -> Task Nav
update msg model session = match msg {
  Submit p -> {
    let found <- Password.verify { email: p.form.email, password: p.form.password }
    match found {
      Just id -> {
        let _ <- Session.put (\s -> { s | user: Just id })
        Task.succeed (Redirect (Todos.Index {}))
      },
      Nothing -> Task.succeed (Reject "Email or password is incorrect."),
    }
  },
}

view : Model -> Session -> Html Msg
view model session =
  <Layout.Page title="Log in" theme={session.theme}>
    <h1>Log in</h1>
    <form submit={Submit}>
      <Errors form />
      <Input field="email" label="Email" />
      <Errors field="email" />
      <Input field="password" label="Password" />
      <Errors field="password" />
      <button>Log in</button>
    </form>
  </Layout.Page>

--- file: app/Layout.alder ---
module Layout

import Theme

Page : { title: Text, theme: Theme, children: List (Html a) } -> Html a
Page props =
  <html class={if props.theme == Dark then "dark" else ""}>
    <head>
      <meta charset="utf-8" />
      <title>{props.title}</title>
      <link rel="stylesheet" href={Url.external "/static/app.css"} />
    </head>
    <body>
      <nav>
        <button click={App.SwitchTheme}>{if props.theme == Dark then "Light mode" else "Dark mode"}</button>
      </nav>
      <main>{props.children}</main>
    </body>
  </html>

--- file: app/Profile.alder ---
module Profile

import Todo
import User
import Layout
import Theme

page "/profile" { } requires user

type Model = {
  user: User,
  total: Int,
  done: Int,
}

load : Params -> Session -> Query Model
load params session = {
  let found <- Db.get User session.user
  match found {
    Just user -> {
      let todos <- Db.from Todo |> Db.where (Todo.owner.eq session.user) |> Db.all
      Query.succeed {
        user: user,
        total: List.length todos,
        done: List.length (List.filter (\t -> t.done) todos),
      }
    },
    Nothing -> Query.notFound,
  }
}

form NewName = {
  name: Text.NonEmpty,
}

msg Rename form NewName

-- The decoder already refuses a blank name; `update` checks again so that the rule
-- holds (and can be tested) without going through a request.
update : Msg -> Model -> Session -> Task Nav
update msg model session = match msg {
  Rename p ->
    if Text.isEmpty (Text.trim p.form.name) then
      Task.succeed (Reject "Name must not be empty.")
    else {
      let _ <- Db.update User session.user (\u -> { u | name: p.form.name })
      Task.succeed Rerender
    },
}

view : Model -> Session -> Html Msg
view model session =
  <Layout.Page title="Profile" theme={session.theme}>
    <h1>Profile</h1>
    <p id="profile-name">Name: {model.user.name}</p>
    <p id="profile-email">Email: {model.user.email}</p>
    <p id="profile-counts">{Text.fromInt model.total} todos, {Text.fromInt model.done} done</p>

    <form submit={Rename}>
      <Errors form />
      <Input field="name" label="Name" value={model.user.name} />
      <Errors field="name" />
      <button>Change name</button>
    </form>

    <a href={Todos.Index {}}>Back to todos</a>

    -- Logout is the App-level message (App.Logout); any page may send it.
    <button click={App.Logout}>Log out</button>
  </Layout.Page>

test "load counts only the user's todos" = {
  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
  let other <- Db.insert User { email: Text.Email "other@example.com", name: Text.NonEmpty "Other", passwordHash: "" }
  let _ <- Db.insert Todo { title: Text.NonEmpty "a", due: Nothing, owner: me }
  let second <- Db.insert Todo { title: Text.NonEmpty "b", due: Nothing, owner: me }
  let _ <- Db.update Todo second (\t -> { t | done: True })
  let _ <- Db.insert Todo { title: Text.NonEmpty "c", due: Nothing, owner: other }
  let theirs <- Db.insert Todo { title: Text.NonEmpty "d", due: Nothing, owner: other }
  let _ <- Db.update Todo theirs (\t -> { t | done: True })
  let model <- load { } { user: me, theme: Light }
  Task.succeed ({ total: model.total, done: model.done } == { total: 2, done: 1 })
}

test "renaming to an empty name is refused" = {
  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
  let model <- load { } { user: me, theme: Light }
  let r <- Test.run (update (Rename { form: { name: Text.NonEmpty "  " } }) model { user: me, theme: Light })
  let after <- Db.get User me
  let name = Maybe.map (\u -> u.name) after
  Task.succeed ({ value: r.value, name: name } == { value: Ok (Reject "Name must not be empty."), name: Just (Text.NonEmpty "Me") })
}

--- file: app/Theme.alder ---
module Theme

type Theme = Light | Dark

flip : Theme -> Theme
flip theme = match theme {
  Light -> Dark,
  Dark -> Light,
}

test "flip alternates" = flip (flip Light) == Light

--- file: app/Todo.alder ---
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

test "countOpen ignores done todos" =
  countOpen [
    { id: TodoId "a", title: Text.NonEmpty "x", done: False, due: Nothing, owner: User.UserId "u", created: Time.epoch },
    { id: TodoId "b", title: Text.NonEmpty "y", done: True, due: Nothing, owner: User.UserId "u", created: Time.epoch },
  ] == 1

--- file: app/Todos/Index.alder ---
module Todos.Index

import Todo
import Layout
import Theme
import User

page "/" { filter: Todo.Filter = All } requires user

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

form NewTodo = {
  title: Text.NonEmpty,
  due: Maybe Date,
}

msg Submit form NewTodo
msg Toggle { id: Todo.TodoId }
msg Delete { id: Todo.TodoId }

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

    <a href={Profile {}}>Profile</a>

    <button click={App.Logout}>Log out</button>
  </Layout.Page>

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

test "Toggle flips done" = {
  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
  let id <- Db.insert Todo { title: Text.NonEmpty "Milk", due: Nothing, owner: me }
  let r <- Test.run (update (Toggle { id: id }) { filter: All, todos: [] } { user: me, theme: Light })
  let after <- Db.get Todo id
  let done = Maybe.map (\t -> t.done) after
  Task.succeed ({ trace: r.trace, done: done } == { trace: ["Db.get Todo", "Db.update Todo"], done: Just True })
}

test "Toggle by another user is not found" = {
  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
  let id <- Db.insert Todo { title: Text.NonEmpty "Milk", due: Nothing, owner: me }
  let stranger = User.UserId "someone-else"
  let r <- Test.run (update (Toggle { id: id }) { filter: All, todos: [] } { user: stranger, theme: Light })
  let after <- Db.get Todo id
  let done = Maybe.map (\t -> t.done) after
  Task.succeed ({ value: r.value, trace: r.trace, done: done } == { value: Err "not found", trace: ["Db.get Todo"], done: Just False })
}

--- file: app/Todos/Show.alder ---
module Todos.Show

import Todo
import Layout

page "/todos/{id}" { id: Todo.TodoId } requires user

type Model = { todo: Todo }

load : Params -> Session -> Query Model
load params session = {
  let found <- Db.get Todo params.id
  match found {
    Just todo -> if todo.owner == session.user then Query.succeed { todo: todo } else Query.notFound,
    Nothing -> Query.notFound,
  }
}

form EditTodo = {
  title: Text.NonEmpty,
  due: Maybe Date,
}

-- The messages carry no id: the page's parameter names the todo, and `load` has checked that
-- the session's user owns it, so `update` acts on `model.todo` (book 06, "Authorize in update").
msg Save form EditTodo
msg Delete

update : Msg -> Model -> Session -> Task Nav
update msg model session = match msg {
  Save p -> {
    let _ <- Db.update Todo model.todo.id (\t -> { t | title: p.form.title, due: p.form.due })
    Task.succeed (Redirect (Todos.Index {}))
  },
  Delete p -> {
    let _ <- Db.delete Todo model.todo.id
    Task.succeed (Redirect (Todos.Index {}))
  },
}

view : Model -> Session -> Html Msg
view model session =
  <Layout.Page title={model.todo.title} theme={session.theme}>
    <h1>{model.todo.title}</h1>
    <form submit={Save}>
      <Input field="title" label="Title" value={model.todo.title} />
      <Errors field="title" />
      <Input field="due" label="Due" value={model.todo.due} />
      <Errors field="due" />
      <button>Save</button>
    </form>
    <button click={Delete}>Delete</button>
    <a href={Todos.Index {}}>Back</a>
  </Layout.Page>

--- file: app/User.alder ---
module User

type UserId = brand Text

table User {
  id: UserId @id,
  email: Text.Email @unique,
  name: Text.NonEmpty,
  passwordHash: Text,
  created: Time @created,
}
