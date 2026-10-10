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
    <p>Name: {model.name}</p>
    <p>Email: {model.email}</p>
    <p>Todos: {Text.fromInt model.todoCount} ({Text.fromInt model.doneCount} done)</p>

    <form submit={Save}>
      <Input field="name" label="Name" value={model.name} />
      <Errors field="name" />
      <button>Update Name</button>
    </form>

    <button click={App.Logout}>Log out</button>
  </Layout.Page>

test "load counts todos correctly" = {
  let user <- Db.insert User { email: Text.Email "user@example.com", name: Text.NonEmpty "User", passwordHash: "" }
  let _ <- Db.insert Todo { title: Text.NonEmpty "Todo 1", due: Nothing, owner: user }
  let _ <- Db.insert Todo { title: Text.NonEmpty "Todo 2", due: Nothing, owner: user }
  let _ <- Db.insert Todo { title: Text.NonEmpty "Todo 3", due: Nothing, owner: user }
  let r <- Test.run (load {} { user: user, theme: Light })
  let result = r.value
  match result {
    Ok m -> Task.succeed (m.todoCount == 3 && m.doneCount == 0)
    Err _ -> Task.succeed False,
  }
}

test "Save with empty name is refused" = {
  let user <- Db.insert User { email: Text.Email "user@example.com", name: Text.NonEmpty "User", passwordHash: "" }
  let r <- Test.run (update (Save { form: { name: Text.NonEmpty "" } }) { name: Text.NonEmpty "User", email: Text.Email "user@example.com", todoCount: 0, doneCount: 0 } { user: user, theme: Light })
  Task.succeed (r.value == Err "not found")
}