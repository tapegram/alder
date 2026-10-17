```alder-cli
# the profile page: /profile, for a signed-in user only
alder new page Profile /profile --requires user --blank
alder fill --decl Profile.Model - <<'EOF'
{ name: Text.NonEmpty, email: Text.Email, todos: Int, doneCount: Int }
EOF
alder fill Profile.load - <<'EOF'
\params session -> {
  let found <- Db.from User |> Db.where (User.id.eq session.user) |> Db.first
  let total <- Db.mine Todo |> Db.count
  let doneN <- Db.mine Todo |> Db.where (Todo.done.eq True) |> Db.count
  match found {
    Just me -> Query.succeed { name: me.name, email: me.email, todos: total, doneCount: doneN },
    Nothing -> Query.notFound,
  }
}
EOF
# the view must be filled before the form and the message
alder fill Profile.view - <<'EOF'
\model session -> <Layout.Page title="Profile">
  <h1>{model.name}</h1>
  <al-card>
    <dl>
      <dt>Email</dt>
      <dd>{model.email}</dd>
      <dt>Todos</dt>
      <dd id="todos">{Text.fromInt model.todos}</dd>
      <dt>Done</dt>
      <dd id="done">{Text.fromInt model.doneCount}</dd>
    </dl>
  </al-card>
  <al-button variant="outline" click={App.Logout}>Log out</al-button>
  <a href={Todos.Index {}}>Back to the list</a>
</Layout.Page>
EOF
alder new form Profile NameForm name:Text.NonEmpty --msg Save
alder fill --decl Profile.update - <<'EOF'
\msg model session -> match msg {
  Save p -> {
    let _ <- Db.update User session.user (\u -> { u | name: p.form.name })
    Task.succeed Rerender
  },
}
EOF
alder fill --decl Profile.view - <<'EOF'
\model session -> <Layout.Page title="Profile">
  <h1>{model.name}</h1>
  <al-card>
    <dl>
      <dt>Email</dt>
      <dd>{model.email}</dd>
      <dt>Todos</dt>
      <dd id="todos">{Text.fromInt model.todos}</dd>
      <dt>Done</dt>
      <dd id="done">{Text.fromInt model.doneCount}</dd>
    </dl>
  </al-card>
  <al-card>
    <form submit={Save}>
      <al-input field="name" label="Name" value={model.name} />
      <Errors field="name" />
      <al-button type="submit">Save name</al-button>
    </form>
  </al-card>
  <al-button variant="outline" click={App.Logout}>Log out</al-button>
  <a href={Todos.Index {}}>Back to the list</a>
</Layout.Page>
EOF
# tests in the profile module
alder new test Profile "the profile counts my todos and the done ones"
alder fill 'Profile."the profile counts my todos and the done ones"' - <<'EOF'
{
  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
  let other <- Db.insert User { email: Text.Email "other@example.com", name: Text.NonEmpty "Other", passwordHash: "" }
  let _ <- Db.insert Todo { title: Text.NonEmpty "Milk", due: Nothing, owner: me }
  let _ <- Db.insert Todo { title: Text.NonEmpty "Bread", done: True, due: Nothing, owner: me }
  let _ <- Db.insert Todo { title: Text.NonEmpty "Eggs", due: Nothing, owner: other }
  let _ <- Test.login { user: Just me }
  let page <- Test.get (Profile {})
  Task.succeed ({ todos: page.model.todos, done: page.model.doneCount } == { todos: 2, done: 1 })
}
EOF
alder new test Profile "renaming to an empty name is refused"
alder fill 'Profile."renaming to an empty name is refused"' - <<'EOF'
{
  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
  let _ <- Test.login { user: Just me }
  let _ <- Test.get (Profile {})
  let r <- Test.submitRaw NameForm [{ name: "name", value: "" }]
  let after <- Db.get User me
  Task.succeed ({
    answer: r.answer,
    name: Maybe.map (\u -> u.name) after,
  } == {
    answer: FormRejected [{ field: "name", message: "must not be empty" }],
    name: Just (Text.NonEmpty "Me"),
  })
}
EOF
# the link from the Index page
alder fill --decl Todos.Index.view - <<'EOF'
\model session -> <Layout.Page title="Todos">
    <h1>Todos</h1>

    <p><a href={Profile {}}>Your profile</a></p>

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

    <al-button variant="outline" click={App.Logout}>Log out</al-button>
  </Layout.Page>
EOF
alder test
```
