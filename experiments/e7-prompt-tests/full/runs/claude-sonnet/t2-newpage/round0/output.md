--- file: app/Profile.alder ---
module Profile

import Todo
import User
import Layout

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
