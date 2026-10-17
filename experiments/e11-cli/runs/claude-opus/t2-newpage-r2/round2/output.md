The app already has errors in the generated admin files, and those errors block every other command. So the script first regenerates the admin and checks the app before building the profile page.

```alder-cli
# the generated admin has errors (Notice.with, al-dialog) that block every command: regenerate it
alder regen admin --all
alder check

# the profile page
alder new page Profile /profile --requires user --blank
alder fill --decl Profile.Model - <<'EOF'
{ name: Text.NonEmpty, email: Text.Email, total: Int, done: Int }
EOF
alder fill Profile.load - <<'EOF'
{
  let found <- Db.get User session.user
  let total <- Db.mine Todo |> Db.count
  let done <- Db.mine Todo |> Db.where (Todo.done.eq True) |> Db.count
  match found {
    Just u -> Query.succeed { name: u.name, email: u.email, total: total, done: done },
    Nothing -> Query.notFound,
  }
}
EOF
alder fill Profile.view - <<'EOF'
<Layout.Page title="Profile">
  <h1>Profile</h1>
  <dl>
    <dt>Name</dt>
    <dd id="name">{model.name}</dd>
    <dt>Email</dt>
    <dd id="email">{model.email}</dd>
  </dl>
  <p id="counts">{Text.fromInt model.done} of {Text.fromInt model.total} todos done</p>
  <al-button variant="outline" click={App.Logout}>Log out</al-button>
</Layout.Page>
EOF
alder new form Profile ChangeName name:Text.NonEmpty --msg SaveName
alder fill --decl Profile.view - <<'EOF'
<Layout.Page title="Profile">
  <h1>Profile</h1>
  <dl>
    <dt>Name</dt>
    <dd id="name">{model.name}</dd>
    <dt>Email</dt>
    <dd id="email">{model.email}</dd>
  </dl>
  <p id="counts">{Text.fromInt model.done} of {Text.fromInt model.total} todos done</p>
  <al-card>
    <form submit={SaveName}>
      <al-input field="name" label="Name" value={model.name} />
      <al-button type="submit">Save</al-button>
    </form>
  </al-card>
  <al-button variant="ghost" href={Todos.Index {}}>Back</al-button>
  -- Logging out is App's message (App.Logout); any page may send it, so it is not duplicated here.
  <al-button variant="outline" click={App.Logout}>Log out</al-button>
</Layout.Page>
EOF
alder fill Profile.update.SaveName - <<'EOF'
{
  let _ <- Db.update User session.user (\u -> { u | name: p.form.name })
  Task.succeed Rerender
}
EOF
alder check

# the link from the to-do list
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

# tests
alder new test Profile "load counts only the user's todos"
alder fill 'Profile."load counts only the user'"'"'s todos"' - <<'EOF'
{
  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
  let other <- Db.insert User { email: Text.Email "other@example.com", name: Text.NonEmpty "Other", passwordHash: "" }
  let _ <- Db.insert Todo { title: Text.NonEmpty "Milk", done: True, due: Nothing, owner: me }
  let _ <- Db.insert Todo { title: Text.NonEmpty "Bread", due: Nothing, owner: me }
  let _ <- Db.insert Todo { title: Text.NonEmpty "Eggs", done: True, due: Nothing, owner: other }
  let _ <- Db.insert Todo { title: Text.NonEmpty "Jam", due: Nothing, owner: other }
  let _ <- Test.login { user: Just me }
  let m <- load {} { user: me, role: Member }
  Task.succeed ({ name: m.name, total: m.total, done: m.done } == { name: Text.NonEmpty "Me", total: 2, done: 1 })
}
EOF
```
