<!-- model=qwen3:8b task=t2-newpage.md seconds=137 prompt_tokens=11187 output_tokens=5219 -->
--- file: app/Profile.alder ---
module Profile

import User
import Layout
import Auth.Login
import Todos.Index

page "/profile" { } requires user

type Model = {
  user: User,
  todoCount: Int,
  doneTodoCount: Int,
}

load : Params -> Session -> Query Model
load params session = {
  let user <- Db.get User session.user
  let todos <- Db.from Todo |> Db.where (Todo.owner.eq session.user) |> Db.all
  let todoCount = List.length todos
  let doneTodoCount = List.filter (\t -> t.done) todos |> List.length
  Query.succeed { user: user, todoCount: todoCount, doneTodoCount: doneTodoCount }
}

form UpdateName = {
  name: Text.NonEmpty,
}

msg UpdateName = "/profile" form UpdateName

update : Msg -> Model -> Session -> Task Nav
update msg model session = match msg {
  UpdateName p -> {
    let _ <- Db.update User model.user.id (\u -> { u | name: p.form.name })
    Task.succeed (Redirect (Profile {}))
  },
}

view : Model -> Html Msg
view model =
  <Layout.Page title="Profile">
    <h1>Profile</h1>
    <p>Name: {model.user.name}</p>
    <p>Email: {model.user.email}</p>
    <p>Todos: {Text.fromInt model.todoCount} ({Text.fromInt model.doneTodoCount} done)</p>
    <form submit={UpdateName}>
      <Input field="name" label="Name" value={model.user.name} />
      <Errors field="name" />
      <button>Update Name</button>
    </form>
    <button click={Auth.Login.Logout}>Log out</button>
  </Layout.Page>

--- file: app/Todos/Index.alder ---
module Todos.Index

import Todo
import User
import Layout
import Todos.Show

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

msg Submit = "/todos" form NewTodo
msg Toggle = "/todos/{id}/toggle" { id: Todo.TodoId }
msg Delete = "/todos/{id}/delete" { id: Todo.TodoId }

update : Msg -> Model -> Session -> Task Nav
update msg model session = match msg {
  Submit p -> {
    let _ <- Db.insert Todo { title: p.form.title, due: p.form.due, owner: session.user }
    Task.succeed (Redirect (Todos.Index { filter: model.filter }))
  },
  Toggle p -> {
    let _ <- Db.update Todo p.id (\t -> { t | done: not t.done })
    Task.succeed Rerender
  },
  Delete p -> {
    let _ <- Db.delete Todo p.id
    Task.succeed Rerender
  },
}

view : Model -> Html Msg
view model =
  <Layout.Page title="Todos">
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
      <a href={Profile { }}>Profile</a>
    </nav>

    <ul id="todos">
      {for todo in model.todos -> <Item todo={todo} />}
    </ul>

    <p id="summary">{Text.fromInt (Todo.countOpen model.todos)} open</p>
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