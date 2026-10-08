--- file: app/Todo.alder ---
module Todo

import User

type TodoId = brand Text

type Filter = All | Active | Completed

type Priority = Low | Normal | High

table Todo {
  id: TodoId @id,
  title: Text.NonEmpty,
  done: Bool = False,
  priority: Priority = Normal,
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

priorityLabel : Priority -> Text
priorityLabel priority = match priority {
  Low -> "Low",
  Normal -> "Normal",
  High -> "High",
}

-- Lower rank sorts first, so High comes first.
priorityRank : Priority -> Int
priorityRank priority = match priority {
  High -> 0,
  Normal -> 1,
  Low -> 2,
}

-- Expects todos newest first; the sort is stable, so newest stays first within a priority.
sortByPriority : List Todo -> List Todo
sortByPriority todos = List.sortBy (\t -> priorityRank t.priority) todos

countOpen : List Todo -> Int
countOpen todos = todos |> List.filter (\t -> not t.done) |> List.length

test "countOpen ignores done todos" =
  countOpen [
    { id: TodoId "a", title: "x", done: False, priority: Normal, due: Nothing, owner: User.UserId "u", created: Time.epoch },
    { id: TodoId "b", title: "y", done: True, priority: Normal, due: Nothing, owner: User.UserId "u", created: Time.epoch },
  ] == 1

test "sortByPriority puts High first, then Normal, then Low" =
  sortByPriority [
    { id: TodoId "a", title: "x", done: False, priority: Low, due: Nothing, owner: User.UserId "u", created: Time.epoch },
    { id: TodoId "b", title: "y", done: False, priority: High, due: Nothing, owner: User.UserId "u", created: Time.epoch },
    { id: TodoId "c", title: "z", done: False, priority: Normal, due: Nothing, owner: User.UserId "u", created: Time.epoch },
  ] |> List.map (\t -> t.id) == [TodoId "b", TodoId "c", TodoId "a"]

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
  Query.succeed {
    filter: params.filter,
    todos: todos |> List.filter (Todo.matches params.filter) |> Todo.sortByPriority,
  }
}

form NewTodo = {
  title: Text.NonEmpty,
  priority: Todo.Priority = Normal,
  due: Maybe Date,
}

msg Submit = "/todos" form NewTodo
msg Toggle = "/todos/{id}/toggle" { id: Todo.TodoId }
msg Delete = "/todos/{id}/delete" { id: Todo.TodoId }

update : Msg -> Model -> Session -> Task Nav
update msg model session = match msg {
  Submit p -> {
    let _ <- Db.insert Todo { title: p.form.title, priority: p.form.priority, due: p.form.due, owner: session.user }
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
      <Select field="priority" label="Priority" />
      <Errors field="priority" />
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
  </Layout.Page>

Item : { todo: Todo } -> Html Msg
Item props =
  <li class={if props.todo.done then "done" else ""}>
    <button click={Toggle { id: props.todo.id }}>{if props.todo.done then "Undo" else "Done"}</button>
    <a href={Todos.Show { id: props.todo.id }}>{props.todo.title}</a>
    <span class="priority">{Todo.priorityLabel props.todo.priority}</span>
    {match props.todo.due {
      Just d -> <span class="due">{Date.format "Jan 2" d}</span>,
      Nothing -> Html.none,
    }}
    <button click={Delete { id: props.todo.id }}>Delete</button>
  </li>

--- file: app/Todos/Show.alder ---
module Todos.Show

import Todo
import Layout
import Todos.Index

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
  priority: Todo.Priority = Normal,
  due: Maybe Date,
}

msg Save = "/todos/{id}/save" { id: Todo.TodoId } form EditTodo
msg Delete = "/todos/{id}/delete" { id: Todo.TodoId }

update : Msg -> Model -> Session -> Task Nav
update msg model session = match msg {
  Save p -> {
    let _ <- Db.update Todo p.id (\t -> { t | title: p.form.title, priority: p.form.priority, due: p.form.due })
    Task.succeed (Redirect (Todos.Index {}))
  },
  Delete p -> {
    let _ <- Db.delete Todo p.id
    Task.succeed (Redirect (Todos.Index {}))
  },
}

view : Model -> Html Msg
view model =
  <Layout.Page title={model.todo.title}>
    <h1>{model.todo.title}</h1>
    <form submit={Save { id: model.todo.id }}>
      <Input field="title" label="Title" value={model.todo.title} />
      <Errors field="title" />
      <Select field="priority" label="Priority" value={Todo.priorityLabel model.todo.priority} />
      <Errors field="priority" />
      <Input field="due" label="Due" value={Maybe.map Date.toText model.todo.due} />
      <Errors field="due" />
      <button>Save</button>
    </form>
    <button click={Delete { id: model.todo.id }}>Delete</button>
    <a href={Todos.Index {}}>Back</a>
  </Layout.Page>
