```alder-cli
alder new page Profile /profile --blank --requires user
alder fill --decl Profile.Model - <<'EOF'
{ user: User, total: Int, done: Int }
EOF
alder fill Profile.load - --hole ?load <<'EOF'
\params session -> {
  let me <- Db.get User session.user
  let todos <- Db.mine Todo |> Db.all
  match me {
    Just u -> Query.succeed { user: u, total: List.length todos, done: List.length (List.filter (\t -> t.done) todos) },
    Nothing -> Query.notFound,
  }
}
EOF
alder fill Profile.view - --hole ?view <<'EOF'
\model session ->
  <Layout.Page title="Profile">
    <h1>Profile</h1>
    <al-card>
      <p id="name">{model.user.name}</p>
      <p id="email">{model.user.email}</p>
      <p id="total">{Text.fromInt model.total} todos</p>
      <p id="done">{Text.fromInt model.done} done</p>
    </al-card>
    <al-button variant="outline" click={App.Logout}>Log out</al-button>
    <a href={Todos.Index {}}>Back to todos</a>
  </Layout.Page>
EOF
alder new form Profile Rename name:Text.NonEmpty --msg Rename
alder fill --decl Profile.view - <<'EOF'
\model session ->
  <Layout.Page title="Profile">
    <h1>Profile</h1>
    <al-card>
      <p id="name">{model.user.name}</p>
      <p id="email">{model.user.email}</p>
      <p id="total">{Text.fromInt model.total} todos</p>
      <p id="done">{Text.fromInt model.done} done</p>
      <form submit={Rename}>
        <al-input field="name" label="Name" value={model.user.name} />
        <al-button type="submit">Save</al-button>
      </form>
    </al-card>
    <al-button variant="outline" click={App.Logout}>Log out</al-button>
    <a href={Todos.Index {}}>Back to todos</a>
  </Layout.Page>
EOF
alder fill Profile.update.Rename - <<'EOF'
{
  let _ <- Db.update User session.user (\u -> { u | name: p.form.name })
  Task.succeed Rerender
}
EOF
alder fill --decl Todos.Index.view - <<'EOF'
\model session ->
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

    <a href={Profile {}}>Profile</a>
    <al-button variant="outline" click={App.Logout}>Log out</al-button>
  </Layout.Page>
EOF
alder new test Profile "load counts only the user's todos"
alder fill 'Profile."load counts only the user'"'"'s todos"' - <<'EOF'
{
  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
  let other <- Db.insert User { email: Text.Email "other@example.com", name: Text.NonEmpty "Other", passwordHash: "" }
  let _ <- Db.insert Todo { title: Text.NonEmpty "Milk", done: True, due: Nothing, owner: me }
  let _ <- Db.insert Todo { title: Text.NonEmpty "Bread", due: Nothing, owner: me }
  let _ <- Db.insert Todo { title: Text.NonEmpty "Eggs", done: True, due: Nothing, owner: other }
  let _ <- Test.login { user: Just me }
  let page <- Test.get (Profile {})
  Task.succeed ({ total: page.model.total, done: page.model.done } == { total: 2, done: 1 })
}
EOF
alder new test Profile "renaming to an empty name is refused"
alder fill 'Profile."renaming to an empty name is refused"' - <<'EOF'
{
  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
  let _ <- Test.login { user: Just me }
  let _ <- Test.get (Profile {})
  let r <- Test.submitRaw Rename [{ name: "name", value: "" }]
  let after <- Db.get User me
  Task.succeed ({ answer: r.answer, name: Maybe.map (\u -> u.name) after } == { answer: FormRejected [{ field: "name", message: "must not be empty" }], name: Just (Text.NonEmpty "Me") })
}
EOF
alder check
alder test
```
