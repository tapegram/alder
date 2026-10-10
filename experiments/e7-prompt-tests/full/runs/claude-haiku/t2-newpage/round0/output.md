--- file: app/Profile.alder ---
```alder
module Profile

import Todo
import User
import Layout
import Theme

page "/profile" { } requires user

type Model = {
  name: Text,
  email: Text,
  total: Int,
  done: Int,
}

load : Params -> Session -> Query Model
load params session = {
  let found <- Db.get User session.user
  match found {
    Just user -> {
      let total <-
        Db.from Todo
          |> Db.where (Todo.owner.eq session.user)
          |> Db.count
      let done <-
        Db.from Todo
          |> Db.where (Todo.owner.eq session.user)
          |> Db.where (Todo.done.eq True)
          |> Db.count
      Query.succeed { name: user.name, email: user.email, total: total, done: done }
    },
    Nothing -> Query.notFound,
  }
}

form NewName = {
  name: Text,
}

msg Save form NewName

-- cleanName is the name a user may choose: trimmed, and never empty.
cleanName : Text -> Result Text Text
cleanName raw = {
  let name = Text.trim raw
  if Text.isEmpty name then Err "Name must not be empty." else Ok name
}

update : Msg -> Model -> Session -> Task Nav
update msg model session = match msg {
  Save p -> match cleanName p.form.name {
    Ok name -> {
      let _ <- Db.update User session.user (\u -> { u | name: name })
      Task.succeed Rerender
    },
    Err e -> Task.succeed (Reject e),
  },
}

view : Model -> Session -> Html Msg
view model session =
  <Layout.Page title="Profile" theme={session.theme}>
    <h1>Profile</h1>
    <dl>
      <dt>Name</dt>
      <dd>{model.name}</dd>
      <dt>Email</dt>
      <dd>{model.email}</dd>
    </dl>
    <p id="counts">{Text.fromInt model.total} todos, {Text.fromInt model.done} done</p>

    <form submit={Save}>
      <Errors form />
      <Input field="name" label="Name" value={model.name} />
      <Errors field="name" />
      <button>Save name</button>
    </form>

    <button click={App.Logout}>Log out</button>
    <a href={Todos.Index {}}>Back to todos</a>
  </Layout.Page>

test "load counts only the user's todos" = {
  let me <- Db.insert User { email: Text.Email "me@example.com", name: "Me", passwordHash: "" }
  let other <- Db.insert User { email: Text.Email "other@example.com", name: "Other", passwordHash: "" }
  let _ <- Db.insert Todo { title: Text.NonEmpty "Milk", due: Nothing, owner: me }
  let _ <- Db.insert Todo { title: Text.NonEmpty "Bread", due: Nothing, owner: me, done: True }
  let _ <- Db.insert Todo { title: Text.NonEmpty "Not mine", due: Nothing, owner: other, done: True }
  let m <- load {} { user: me, theme: Light }
  Task.succeed ({ total: m.total, done: m.done } == { total: 2, done: 1 })
}

test "renaming to an empty name is refused" =
  cleanName "   " == Err "Name must not be empty."

test "renamed names are trimmed" =
  cleanName "  Ann " == Ok "Ann"
```

--- file: app/User.alder ---
```alder
module User

-- name is checked when it is changed: Profile.cleanName refuses an empty one.
type UserId = brand Text

table User {
  id: UserId @id,
  email: Text.Email @unique,
  name: Text,
  passwordHash: Text,
  created: Time @created,
}
```

--- file: app/Todos/Index.alder ---
```alder
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
  let me <- Db.insert User { email: Text.Email "me@example.com", name: "Me", passwordHash: "" }
  let id <- Db.insert Todo { title: Text.NonEmpty "Milk", due: Nothing, owner: me }
  let r <- Test.run (update (Toggle { id: id }) { filter: All, todos: [] } { user: me, theme: Light })
  let after <- Db.get Todo id
  let done = Maybe.map (\t -> t.done) after
  Task.succeed ({ trace: r.trace, done: done } == { trace: ["Db.get Todo", "Db.update Todo"], done: Just True })
}

test "Toggle by another user is not found" = {
  let me <- Db.insert User { email: Text.Email "me@example.com", name: "Me", passwordHash: "" }
  let id <- Db.insert Todo { title: Text.NonEmpty "Milk", due: Nothing, owner: me }
  let stranger = User.UserId "someone-else"
  let r <- Test.run (update (Toggle { id: id }) { filter: All, todos: [] } { user: stranger, theme: Light })
  let after <- Db.get Todo id
  let done = Maybe.map (\t -> t.done) after
  Task.succeed ({ value: r.value, trace: r.trace, done: done } == { value: Err "not found", trace: ["Db.get Todo"], done: Just False })
}
```
