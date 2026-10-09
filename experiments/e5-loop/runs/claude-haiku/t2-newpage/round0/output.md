--- file: app/Profile.alder ---
module Profile

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
  let todoCount <-
    Db.from Todo
      |> Db.where (Todo.owner.eq session.user)
      |> Db.count
  let doneCount <-
    Db.from Todo
      |> Db.where (Todo.owner.eq session.user)
      |> Db.where (Todo.done.eq True)
      |> Db.count
  match found {
    Just user -> Query.succeed { user: user, todoCount: todoCount, doneCount: doneCount },
    Nothing -> Query.notFound,
  }
}

form EditName = {
  name: Text.NonEmpty,
}

msg Rename form EditName

update : Msg -> Model -> Session -> Task Nav
update msg model session = match msg {
  Rename p -> {
    let _ <- Db.update User session.user (\u -> { u | name: p.form.name })
    Task.succeed (Redirect (Profile {}))
  },
}

view : Model -> Session -> Html Msg
view model session =
  <Layout.Page title="Profile" theme={session.theme}>
    <h1>Profile</h1>

    <dl>
      <dt>Name</dt>
      <dd id="name">{model.user.name}</dd>
      <dt>Email</dt>
      <dd id="email">{model.user.email}</dd>
    </dl>

    <p id="todo-counts">{Text.fromInt model.todoCount} todos, {Text.fromInt model.doneCount} done</p>

    <h2>Change name</h2>
    <form submit={Rename}>
      <Errors form />
      <Input field="name" label="Name" value={model.user.name} />
      <Errors field="name" />
      <button>Save</button>
    </form>

    <a href={Todos.Index {}}>Back</a>
    -- Log out is the app-level App.Logout message, declared in App, not in this page.
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
    let _ <- Db.update Todo p.id (\t -> { t | done: not t.done })
    Task.succeed Rerender
  },
  Delete p -> {
    let _ <- Db.delete Todo p.id
    Task.succeed Rerender
  },
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
