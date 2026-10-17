```alder-cli
# the profile page
alder new page Profile /profile --requires user --blank
alder fill --decl Profile.Model - <<'EOF'
{ name: Text.NonEmpty, email: Text.Email, total: Int, done: Int }
EOF
alder fill Profile.load - <<'EOF'
{
  let me <- Db.get User session.user
  let total <- Db.mine Todo |> Db.count
  let done <- Db.mine Todo |> Db.where (Todo.done.eq True) |> Db.count
  match me {
    Just u -> Query.succeed { name: u.name, email: u.email, total: total, done: done },
    Nothing -> Query.notFound,
  }
}
EOF
alder fill Profile.view - <<'EOF'
<Layout.Page title="Profile">
  <h1>Profile</h1>
</Layout.Page>
EOF
alder new form Profile NewName name:Text.NonEmpty --msg Rename
alder fill Profile.update.Rename - <<'EOF'
{
  let _ <- Db.update User session.user (\u -> { u | name: p.form.name })
  Task.succeed Rerender
}
EOF
alder fill --decl Profile.view - <<'EOF'
<Layout.Page title="Profile">
  <h1>Profile</h1>
  <al-card>
    <dl>
      <dt>Name</dt>
      <dd id="name">{model.name}</dd>
      <dt>Email</dt>
      <dd id="email">{model.email}</dd>
      <dt>Todos</dt>
      <dd id="total">{Text.fromInt model.total}</dd>
      <dt>Done</dt>
      <dd id="done">{Text.fromInt model.done}</dd>
    </dl>
  </al-card>
  <al-card>
    <form submit={Rename}>
      <al-input field="name" label="Name" value={model.name} />
      <al-button type="submit">Save</al-button>
    </form>
  </al-card>
  -- Logging out is the app-level message App.Logout, which any page can send: no second one here.
  <al-button variant="outline" click={App.Logout}>Log out</al-button>
</Layout.Page>
EOF

# the tests of the profile page
alder new test Profile "load counts only the user's todos"
alder fill 'Profile."load counts only the user'"'"'s todos"' - <<'EOF'
{
  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
  let other <- Db.insert User { email: Text.Email "other@example.com", name: Text.NonEmpty "Other", passwordHash: "" }
  let _ <- Db.insert Todo { title: Text.NonEmpty "Milk", due: Nothing, owner: me }
  let _ <- Db.insert Todo { title: Text.NonEmpty "Bread", done: True, due: Nothing, owner: me }
  let _ <- Db.insert Todo { title: Text.NonEmpty "Eggs", due: Nothing, owner: other }
  let _ <- Db.insert Todo { title: Text.NonEmpty "Jam", done: True, due: Nothing, owner: other }
  let _ <- Test.login { user: Just me }
  let m <- load {} { user: me, role: Member }
  Task.succeed ({ name: m.name, total: m.total, done: m.done } == { name: Text.NonEmpty "Me", total: 2, done: 1 })
}
EOF
alder new test Profile "renaming to an empty name is refused"
alder fill 'Profile."renaming to an empty name is refused"' - <<'EOF'
{
  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
  let _ <- Test.login { user: Just me }
  let _ <- Test.get (Profile {})
  let r <- Test.submitRaw NewName [{ name: "name", value: "" }]
  let after <- Db.get User me
  Task.succeed ({ answer: r.answer, name: Maybe.map (\u -> u.name) after } == { answer: FormRejected [{ field: "name", message: "must not be empty" }], name: Just (Text.NonEmpty "Me") })
}
EOF
alder new test Profile "renaming changes the name"
alder fill 'Profile."renaming changes the name"' - <<'EOF'
{
  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
  let _ <- Test.login { user: Just me }
  let _ <- Test.get (Profile {})
  let r <- Test.submit NewName { name: Text.NonEmpty "Ann" }
  let after <- Db.get User me
  Task.succeed ({ answer: r.answer, name: Maybe.map (\u -> u.name) after, shown: Test.text "#name" r.doc } == { answer: Rerendered, name: Just (Text.NonEmpty "Ann"), shown: Just "Ann" })
}
EOF

# a link to the profile from the to-do list
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

  <p><a id="profile" href={Profile {}}>Profile</a></p>

  <al-button variant="outline" click={App.Logout}>Log out</al-button>
</Layout.Page>
EOF
alder new test Todos.Index "the list links to the profile"
alder fill 'Todos.Index."the list links to the profile"' - <<'EOF'
{
  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
  let _ <- Test.login { user: Just me }
  let page <- Test.get (Todos.Index {})
  Task.succeed (Test.attr "href" "#profile" page.doc == Just "/profile")
}
EOF

alder check
alder test
```
