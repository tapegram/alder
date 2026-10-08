--- file: app/Profile.alder ---
module Profile

import Todo
import User
import Layout
import Auth.Login

page "/profile" { } requires user

type Model = {
  user: User,
  total: Int,
  done: Int,
}

load : Params -> Session -> Query Model
load params session = {
  let found <- Db.get User session.user
  let todos <- Db.from Todo |> Db.where (Todo.owner.eq session.user) |> Db.all
  match found {
    Just u -> Query.succeed {
      user: u,
      total: List.length todos,
      done: todos |> List.filter (\t -> t.done) |> List.length,
    },
    Nothing -> Query.notFound,
  }
}

form ChangeName = {
  name: Text.NonEmpty,
}

msg Rename = "/profile/name" form ChangeName

update : Msg -> Model -> Session -> Task Nav
update msg model session = match msg {
  Rename p -> {
    let _ <- Db.update User session.user (\u -> { u | name: p.form.name })
    Task.succeed Rerender
  },
}

view : Model -> Html Msg
view model =
  <Layout.Page title="Profile">
    <h1>Profile</h1>

    <dl id="details">
      <dt>Name</dt>
      <dd>{model.user.name}</dd>
      <dt>Email</dt>
      <dd>{model.user.email}</dd>
    </dl>

    <p id="stats">
      {Text.fromInt model.total} todos, {Text.fromInt model.done} done
    </p>

    <form submit={Rename}>
      <Errors form />
      <Input field="name" label="Name" value={model.user.name} />
      <Errors field="name" />
      <button>Change name</button>
    </form>

    -- NOTE: Logout is a msg owned by Auth.Login, and a msg can only be sent
    -- by the page that declares it (click/submit take that page's own Msg).
    -- So this cannot post to Auth.Login's Logout directly; it links to the
    -- Login page, where the Logout msg lives.
    <a href={Auth.Login {}}>Log out</a>

    <a href={Todos.Index {}}>Back</a>
  </Layout.Page>

--- file: app/Todos/Index.alder ---
module Todos.Index

import Todo
import User
import Layout
import Profile
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

    <a href={Profile {}}>Profile</a>

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
