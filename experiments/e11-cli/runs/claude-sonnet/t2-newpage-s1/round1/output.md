```alder-cli
alder fill Profile.load - <<'EOF'
{
  let found <- Db.get User session.user
  let total <- Db.mine Todo |> Db.count
  let finished <- Db.mine Todo |> Db.where (Todo.done.eq True) |> Db.count
  match found {
    Just u -> Query.succeed { name: u.name, email: u.email, total: total, finished: finished },
    Nothing -> Query.notFound,
  }
}
EOF
alder fill Profile.view - <<'EOF'
<Layout.Page title="Profile"><h1>Profile</h1></Layout.Page>
EOF
alder new form Profile Rename name:Text.NonEmpty --msg SaveName
alder fill Profile.update.SaveName - <<'EOF'
{
  let _ <- Db.update User session.user (\u -> { u | name: p.form.name })
  Task.succeed Rerender
}
EOF
alder fill --decl Profile.view - <<'EOF'
<Layout.Page title="Profile">
  <h1>Profile</h1>
  <p id="name">{model.name}</p>
  <p id="email">{model.email}</p>
  <p id="total">{Text.fromInt model.total}</p>
  <p id="finished">{Text.fromInt model.finished}</p>
  <al-card>
    <form submit={SaveName}>
      <al-input field="name" label="Name" value={model.name} />
      <Errors field="name" />
      <al-button type="submit">Save</al-button>
    </form>
  </al-card>
  <a href={Todos.Index {}}>Back to todos</a>
  <al-button variant="outline" click={App.Logout}>Log out</al-button>
</Layout.Page>
EOF
alder fill --decl Todos.Index.view - <<'EOF'
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

  <a id="profile" href={Profile {}}>Profile</a>
  <al-button variant="outline" click={App.Logout}>Log out</al-button>
</Layout.Page>
EOF
alder new test Profile "load counts only the user's todos"
alder fill 'Profile."load counts only the user'"'"'s todos"' - <<'EOF'
{
  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
  let other <- Db.insert User { email: Text.Email "other@example.com", name: Text.NonEmpty "Other", passwordHash: "" }
  let _ <- Db.insert Todo { title: Text.NonEmpty "A", done: True, due: Nothing, owner: me }
  let _ <- Db.insert Todo { title: Text.NonEmpty "B", done: False, due: Nothing, owner: me }
  let _ <- Db.insert Todo { title: Text.NonEmpty "C", done: True, due: Nothing, owner: other }
  let _ <- Db.insert Todo { title: Text.NonEmpty "D", done: False, due: Nothing, owner: other }
  let _ <- Db.insert Todo { title: Text.NonEmpty "E", done: False, due: Nothing, owner: other }
  let _ <- Test.login { user: Just me }
  let page <- Test.get (Profile {})
  Task.succeed ({ total: page.model.total, finished: page.model.finished } == { total: 2, finished: 1 })
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
alder test
```
