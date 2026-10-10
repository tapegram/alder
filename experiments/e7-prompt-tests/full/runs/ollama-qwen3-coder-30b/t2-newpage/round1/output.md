--- file: app/Profile.alder ---
module Profile

import Todo
import Layout
import User

page "/profile" { } requires user

type Model = {
  name: Text.NonEmpty,
  email: Text.Email,
  todoCount: Int,
  doneCount: Int,
}

load : Params -> Session -> Query Model
load params session = {
  let todos <- Db.from Todo |> Db.where (Todo.owner.eq session.user) |> Db.all
  let todoCount = List.length todos
  let doneCount = List.length (List.filter (\t -> t.done) todos)
  let user <- Db.get User session.user
  match user {
    Just u -> Query.succeed { name: u.name, email: u.email, todoCount: todoCount, doneCount: doneCount }
    Nothing -> Query.notFound,
  }
}

form UpdateName = {
  name: Text.NonEmpty,
}

msg Save form UpdateName

update : Msg -> Model -> Session -> Task Nav
update msg model session = match msg {
  Save p -> {
    let _ <- Db.update User session.user (\u -> { u | name: p.form.name })
    Task.succeed Rerender
  },
}

view : Model -> Session -> Html Msg
view model session =
  <Layout.Page title="Profile" theme={session.theme}>
    <h1>Profile</h1>
    <p><strong>Name:</strong> {model.name}</p>
    <p><strong>Email:</strong> {model.email}</p>
    <p><strong>Todos:</strong> {Text.fromInt model.todoCount} ({Text.fromInt model.doneCount} done)</p>
    <form submit={Save}>
      <Input field="name" label="Name" />
      <Errors field="name" />
      <button>Update Name</button>
    </form>
    <button click={App.Logout}>Log out</button>
  </Layout.Page>

test "load counts user's todos" = {
  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
  let id1 <- Db.insert Todo { title: Text.NonEmpty "Milk", due: Nothing, owner: me }
  let id2 <- Db.insert Todo { title: Text.NonEmpty "Bread", due: Nothing, owner: me }
  let r <- Test.run (Profile.load {} { user: me, theme: Light })
  Task.succeed (r.value == Ok { name: Text.NonEmpty "Me", email: Text.Email "me@example.com", todoCount: 2, doneCount: 0 })
}

test "renaming to empty is refused" = {
  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
  let r <- Test.run (Profile.update (Save { name: Text.NonEmpty "" }) { name: Text.NonEmpty "Me", email: Text.Email "me@example.com", todoCount: 0, doneCount: 0 } { user: me, theme: Light })
  Task.succeed (r.value == Err "not found")
}

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