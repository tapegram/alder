--- file: app/Profile.alder ---
module Profile

import Todo
import Layout
import User

page "/profile" { } requires user

type Model = {
  user: User,
  total: Int,
  done: Int,
}

-- The user's row is read again on every request: the session holds only the id. The counts use
-- the scoped `Db.mine Todo`, so they are only the signed-in user's todos (book 06, "Rules").
load : Params -> Session -> Query Model
load params session = {
  let found <- Db.get User session.user
  let total <- Db.mine Todo |> Db.count
  let done <- Db.mine Todo |> Db.where (Todo.done.eq True) |> Db.count
  match found {
    Just u -> Query.succeed { user: u, total: total, done: done },
    Nothing -> Query.notFound,
  }
}

form Rename = {
  name: Text.NonEmpty,
}

msg Save form Rename

-- `User` has no `@owner` column, so the write is unscoped; the row is the one `load` read for the
-- session's own user, so it can only ever be the signed-in user's.
update : Msg -> Model -> Session -> Task Nav
update msg model session = match msg {
  Save p -> {
    let _ <- Db.update User model.user.id (\u -> { u | name: p.form.name })
    Task.succeed (Redirect (Profile {}))
  },
}

-- Logging out is the app-level message `App.Logout` (declared in App, not on the login page):
-- any page may send it, so the button posts it rather than declaring a second Logout here.
view : Model -> Session -> Html Msg
view model session =
  <Layout.Page title="Profile">
    <h1>Profile</h1>

    <al-card>
      <dl>
        <dt>Name</dt>
        <dd id="name">{model.user.name}</dd>
        <dt>Email</dt>
        <dd id="email">{model.user.email}</dd>
        <dt>Todos</dt>
        <dd id="total">{Text.fromInt model.total}</dd>
        <dt>Done</dt>
        <dd id="done">{Text.fromInt model.done}</dd>
      </dl>
    </al-card>

    <al-card>
      <form submit={Save}>
        <al-input field="name" label="Name" value={model.user.name} />
        <al-button type="submit">Save</al-button>
      </form>
    </al-card>

    <p><a href={Todos.Index {}}>Back to the todos</a></p>

    <al-button variant="outline" click={App.Logout}>Log out</al-button>
  </Layout.Page>

test "load counts only the user's todos" = {
  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
  let other <- Db.insert User { email: Text.Email "other@example.com", name: Text.NonEmpty "Other", passwordHash: "" }
  let _ <- Db.insert Todo { title: Text.NonEmpty "Milk", done: True, due: Nothing, owner: me }
  let _ <- Db.insert Todo { title: Text.NonEmpty "Bread", due: Nothing, owner: me }
  let _ <- Db.insert Todo { title: Text.NonEmpty "Eggs", done: True, due: Nothing, owner: other }
  let _ <- Db.insert Todo { title: Text.NonEmpty "Jam", due: Nothing, owner: other }
  let _ <- Test.login { user: Just me }
  let m <- load {} { user: me, role: Member }
  Task.succeed ({ name: m.user.name, total: m.total, done: m.done } == { name: Text.NonEmpty "Me", total: 2, done: 1 })
}

test "the page shows the name, the email and the counts" = {
  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
  let _ <- Db.insert Todo { title: Text.NonEmpty "Milk", done: True, due: Nothing, owner: me }
  let _ <- Test.login { user: Just me }
  let page <- Test.get (Profile {})
  Task.succeed ({
    name: Test.text "#name" page.doc,
    email: Test.text "#email" page.doc,
    total: Test.text "#total" page.doc,
    done: Test.text "#done" page.doc,
  } == {
    name: Just "Me",
    email: Just "me@example.com",
    total: Just "1",
    done: Just "1",
  })
}

test "renaming changes the name" = {
  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
  let _ <- Test.login { user: Just me }
  let _ <- Test.get (Profile {})
  let r <- Test.submit Rename { name: Text.NonEmpty "New Me" }
  let after <- Db.get User me
  Task.succeed ({ answer: r.answer, name: Maybe.map (\u -> u.name) after } == { answer: Redirected (Profile {}), name: Just (Text.NonEmpty "New Me") })
}

test "renaming to an empty name is refused" = {
  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
  let _ <- Test.login { user: Just me }
  let _ <- Test.get (Profile {})
  let r <- Test.submitRaw Rename [{ name: "name", value: "" }]
  let after <- Db.get User me
  Task.succeed ({ answer: r.answer, name: Maybe.map (\u -> u.name) after } == { answer: FormRejected [{ field: "name", message: "must not be empty" }], name: Just (Text.NonEmpty "Me") })
}

test "a visitor is sent to log in from the profile" = {
  let r <- Test.open (Profile {})
  Task.succeed (r.answer == Redirected (Auth.Login {}))
}

test "logging out from the profile" = {
  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
  let _ <- Test.login { user: Just me }
  let _ <- Test.get (Profile {})
  let out <- Test.click App.Logout
  Task.succeed ({ answer: out.answer, user: out.session.user } == { answer: Redirected (Auth.Login {}), user: Nothing })
}

--- file: app/Todos/Index.alder ---
module Todos.Index

import Todo
import Layout
import User

page "/" { filter: Todo.Filter = All } requires user

type Model = {
  filter: Todo.Filter,
  todos: List Todo,
}

load : Params -> Session -> Query Model
load params session = {
  let todos <-
    Db.mine Todo
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
    let _ <- Db.insertMine Todo { title: p.form.title, due: p.form.due }
    Task.succeed (Redirect (Todos.Index { filter: model.filter }))
  },
  -- A message's id comes from the request: anyone can post `Toggle` with another user's id.
  -- `Db.updateMine` and `Db.deleteMine` change the row only when it is the session's user's;
  -- any other id, like a missing one, is a 404 (book 06, "Rules").
  Toggle p -> {
    let _ <- Db.updateMine Todo p.id (\t -> { t | done: not t.done })
    Task.succeed Rerender
  },
  Delete p -> {
    let _ <- Db.deleteMine Todo p.id
    Task.succeed Rerender
  },
}

view : Model -> Session -> Html Msg
view model session =
  <Layout.Page title="Todos">
    <h1>Todos</h1>

    <al-card>
      <form submit={Submit}>
        <al-input field="title" label="What needs doing?" />
        <al-input field="due" label="Due" />
        <al-button type="submit">Add</al-button>
      </form>
    </al-card>

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

    <p id="profile"><a href={Profile {}}>Profile</a></p>

    <al-button variant="outline" click={App.Logout}>Log out</al-button>
  </Layout.Page>

Item : { todo: Todo } -> Html Msg
Item props =
  <li class={if props.todo.done then "done" else ""}>
    <al-button variant="secondary" size="sm" click={Toggle { id: props.todo.id }}>{if props.todo.done then "Undo" else "Done"}</al-button>
    <a href={Todos.Show { id: props.todo.id }}>{props.todo.title}</a>
    {match props.todo.due {
      Just d -> <al-badge variant="outline" class="due">{Date.format "Jan 2" d}</al-badge>,
      Nothing -> Html.none,
    }}
    <al-button variant="ghost" size="sm" click={Delete { id: props.todo.id }}>Delete</al-button>
  </li>

test "Toggle flips done" = {
  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
  let id <- Db.insert Todo { title: Text.NonEmpty "Milk", due: Nothing, owner: me }
  let _ <- Test.login { user: Just me }
  let r <- Test.run (update (Toggle { id: id }) { filter: All, todos: [] } { user: me, role: Member })
  let after <- Db.get Todo id
  let done = Maybe.map (\t -> t.done) after
  Task.succeed ({ trace: r.trace, done: done } == { trace: ["Db.updateMine Todo"], done: Just True })
}

-- A stranger's own page lists none of my todos, but a message's id comes from the request.
-- Here my page is still open in the browser when a stranger logs in on it and presses my todo's
-- Toggle: `Db.updateMine` finds no todo of the stranger's with that id, so the answer is a 404
-- and nothing changes.
test "Toggle by another user is not found" = {
  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
  let id <- Db.insert Todo { title: Text.NonEmpty "Milk", due: Nothing, owner: me }
  let _ <- Test.login { user: Just me }
  let _ <- Test.get (Todos.Index {})
  let _ <- Test.login { user: Just (User.UserId "someone-else") }
  let r <- Test.click (Toggle { id: id })
  let after <- Db.get Todo id
  Task.succeed ({ answer: r.answer, done: Maybe.map (\t -> t.done) after } == { answer: PageMissing, done: Just False })
}

test "Delete by another user is not found" = {
  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
  let id <- Db.insert Todo { title: Text.NonEmpty "Milk", due: Nothing, owner: me }
  let _ <- Test.login { user: Just me }
  let _ <- Test.get (Todos.Index {})
  let _ <- Test.login { user: Just (User.UserId "someone-else") }
  let r <- Test.click (Delete { id: id })
  let after <- Db.get Todo id
  Task.succeed ({ answer: r.answer, todo: Maybe.map (\t -> t.title) after } == { answer: PageMissing, todo: Just (Text.NonEmpty "Milk") })
}

-- The same page through the browser: the test logs in, opens the page, and uses the form and
-- the buttons the page renders, as a user would (book 10, "Testing through the browser").
test "adding a todo through the page" = {
  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
  let _ <- Test.login { user: Just me }
  let _ <- Test.get (Todos.Index {})
  let added <- Test.submit NewTodo { title: Text.NonEmpty "Milk", due: Nothing }
  let page <- Test.get (Todos.Index {})
  Task.succeed ({ answer: added.answer, todos: List.map (\t -> t.title) page.model.todos, summary: Test.text "#summary" page.doc } == { answer: Redirected (Todos.Index {}), todos: [Text.NonEmpty "Milk"], summary: Just "1 open" })
}

test "toggling a todo through the page" = {
  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
  let id <- Db.insert Todo { title: Text.NonEmpty "Milk", due: Nothing, owner: me }
  let _ <- Test.login { user: Just me }
  let _ <- Test.get (Todos.Index {})
  let r <- Test.click (Toggle { id: id })
  Task.succeed ({ answer: r.answer, done: Test.has "li.done" r.doc, summary: Test.text "#summary" r.doc } == { answer: Rerendered, done: True, summary: Just "0 open" })
}

-- A stranger's page lists none of my todos, so it has no Toggle button to click for them; the
-- todo's own page is a 404 for the stranger.
test "a stranger cannot see my todo" = {
  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
  let id <- Db.insert Todo { title: Text.NonEmpty "Milk", due: Nothing, owner: me }
  let _ <- Test.login { user: Just (User.UserId "someone-else") }
  let page <- Test.get (Todos.Index {})
  let show <- Test.open (Todos.Show { id: id })
  Task.succeed ({ items: Test.count "li" page.doc, show: show.answer } == { items: 0, show: PageMissing })
}

test "Delete removes the todo" = {
  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
  let id <- Db.insert Todo { title: Text.NonEmpty "Milk", due: Nothing, owner: me }
  let _ <- Test.login { user: Just me }
  let r <- Test.run (update (Delete { id: id }) { filter: All, todos: [] } { user: me, role: Member })
  let after <- Db.get Todo id
  Task.succeed ({ value: r.value, todo: Maybe.map (\t -> t.title) after } == { value: Ok Rerender, todo: Nothing })
}

test "the newest todo is listed first" = {
  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
  let _ <- Db.insert Todo { title: Text.NonEmpty "First", due: Nothing, owner: me }
  let _ <- Db.insert Todo { title: Text.NonEmpty "Second", due: Nothing, owner: me }
  let _ <- Test.login { user: Just me }
  let page <- Test.get (Todos.Index {})
  Task.succeed (Test.texts "li a" page.doc == ["Second", "First"])
}

-- A test may use the scoped operations itself: they act for the test's session, which
-- `Test.login` sets. `Db.mine Todo` is what Me sees, and not the other user's todo.
test "Db.mine is what Me sees" = {
  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
  let other <- Db.insert User { email: Text.Email "other@example.com", name: Text.NonEmpty "Other", passwordHash: "" }
  let _ <- Db.insert Todo { title: Text.NonEmpty "Milk", due: Nothing, owner: me }
  let _ <- Db.insert Todo { title: Text.NonEmpty "Bread", due: Nothing, owner: me }
  let _ <- Db.insert Todo { title: Text.NonEmpty "Eggs", due: Nothing, owner: other }
  let _ <- Test.login { user: Just me }
  let r <- Test.run {
    let n <- Db.mine Todo |> Db.count
    Task.succeed n
  }
  Task.succeed ({ value: r.value, trace: r.trace } == { value: Ok 2, trace: ["Db.mine Todo", "Db.count Todo"] })
}

test "the list links to the profile" = {
  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
  let _ <- Test.login { user: Just me }
  let page <- Test.get (Todos.Index {})
  let profile <- Test.get (Profile {})
  Task.succeed ({ link: Test.text "#profile a" page.doc, title: Test.text "h1" profile.doc } == { link: Just "Profile", title: Just "Profile" })
}
