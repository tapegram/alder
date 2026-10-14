```alder-cli
# the profile page: logged in, no form yet (the view must be filled before a form can be made)
alder new page Profile /profile --blank --requires user

# its Model, the name and email shown, and the todo counts
alder fill --decl Profile.Model - <<'EOF'
{ name: Text, email: Text, total: Int, done: Int }
EOF

# load reads the user's row and counts the signed-in user's todos
alder fill --decl Profile.load - <<'EOF'
\params session -> {
  let who : User.UserId = session.user
  let found <- Db.get User who
  let total <- Db.mine Todo |> Db.count
  let done <- Db.mine Todo |> Db.where (Todo.done.eq True) |> Db.count
  match found {
    Just u -> Query.succeed { name: u.name, email: u.email, total: total, done: done },
    Nothing -> Query.notFound,
  }
}
EOF

# a first view, without the form, so that the form can be made
alder fill --decl Profile.view - <<'EOF'
\model session -> <Layout.Page title="Profile">
  <h1>Profile</h1>
  <p>{model.name} ({model.email})</p>
  <p id="todos">{Text.fromInt model.total} todos, {Text.fromInt model.done} done</p>
  <al-button variant="outline" click={App.Logout}>Log out</al-button>
  <a href={Todos.Index {}}>Back to todos</a>
</Layout.Page>
EOF

# the name form, with its message SubmitRename
alder new form Profile Rename name:Text.NonEmpty

# renaming: write the new name to the signed-in user's row, then show the page again
alder fill Profile.update.SubmitRename - <<'EOF'
{
  let _ <- Db.update User session.user (\u -> { u | name: p.form.name })
  Task.succeed Rerender
}
EOF

# the final view, with the form and its errors
alder fill --decl Profile.view - <<'EOF'
\model session -> <Layout.Page title="Profile">
  <h1>Profile</h1>
  <al-card>
    <p>{model.name} ({model.email})</p>
    <p id="todos">{Text.fromInt model.total} todos, {Text.fromInt model.done} done</p>
    <form submit={SubmitRename}>
      <al-input field="name" label="Name" value={model.name} />
      <al-alert variant="destructive"><Errors form /></al-alert>
      <al-button type="submit">Save name</al-button>
    </form>
  </al-card>
  <al-button variant="outline" click={App.Logout}>Log out</al-button>
  <a href={Todos.Index {}}>Back to todos</a>
</Layout.Page>
EOF

# the link from the to-do list to the profile page
alder fill --decl Todos.Index.view - <<'EOF'
\model session -> <Layout.Page title="Todos">
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
      <a href={Profile {}}>Profile</a>
    </nav>

    <ul id="todos">
      {for todo in model.todos -> <Item todo={todo} />}
    </ul>

    <p id="summary">{Text.fromInt (Todo.countOpen model.todos)} open</p>

    <al-button variant="outline" click={App.Logout}>Log out</al-button>
  </Layout.Page>
EOF

# test: load counts only the signed-in user's todos
alder new test Profile "load counts only the todos of the signed-in user"
alder fill 'Profile."load counts only the todos of the signed-in user"' - <<'EOF'
{
  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
  let other <- Db.insert User { email: Text.Email "other@example.com", name: Text.NonEmpty "Other", passwordHash: "" }
  let _ <- Db.insert Todo { title: Text.NonEmpty "Milk", due: Nothing, owner: me, done: False }
  let _ <- Db.insert Todo { title: Text.NonEmpty "Eggs", due: Nothing, owner: me, done: True }
  let _ <- Db.insert Todo { title: Text.NonEmpty "Bread", due: Nothing, owner: other, done: True }
  let _ <- Test.login { user: Just me }
  let page <- Test.get (Profile {})
  Task.succeed ({ total: page.model.total, done: page.model.done } == { total: 2, done: 1 })
}
EOF

# test: renaming to an empty name is refused, and the name is kept
alder new test Profile "renaming to an empty name is refused"
alder fill 'Profile."renaming to an empty name is refused"' - <<'EOF'
{
  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
  let _ <- Test.login { user: Just me }
  let _ <- Test.get (Profile {})
  let r <- Test.submitRaw SubmitRename [{ name: "name", value: "" }]
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
```
