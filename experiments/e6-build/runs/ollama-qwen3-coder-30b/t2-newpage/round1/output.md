--- file: app/Profile/Show.alder ---
module Profile.Show

import User
import Todo
import Layout

page "/profile" { } requires user

type Model = {
  user: User,
  todoCount: Int,
  doneCount: Int,
}

load : Params -> Session -> Query Model
load params session = {
  let found <- Db.get User session.user
  match found {
    Just user -> {
      let todos <-
        Db.from Todo
          |> Db.where (Todo.owner.eq session.user)
          |> Db.all
      let todoCount = List.length todos
      let doneCount = List.length (List.filter (\t -> t.done) todos)
      Query.succeed { user: user, todoCount: todoCount, doneCount: doneCount }
    },
    Nothing -> Query.notFound,
  }
}

form EditName = {
  name: Text.NonEmpty,
}

msg Save form EditName

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
    <p>Name: {model.user.name}</p>
    <p>Email: {model.user.email}</p>
    <p>Todos: {Text.fromInt model.todoCount} ({Text.fromInt model.doneCount} done)</p>
    <form submit={Save}>
      <Input field="name" label="Name" value={model.user.name} />
      <Errors field="name" />
      <button>Save</button>
    </form>
    <button click={App.Logout}>Log out</button>
  </Layout.Page>

--- file: app/Todos/Index.alder ---
module Todos.Index

import Todo
import Layout

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

    <a href={Profile.Show {}}>Profile</a>
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