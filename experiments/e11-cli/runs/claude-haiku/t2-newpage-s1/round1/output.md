```alder-cli
# the load: the signed-in user's name and email, and the counts of their own todos
alder fill Profile.load - --hole ?load <<'EOF'
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
# a first view, without the form (its message does not exist yet)
alder fill Profile.view - --hole ?view <<'EOF'
<Layout.Page title="Profile">
  <h1>{model.name}</h1>
  <p>{model.email}</p>
  <p>{Text.fromInt model.total} todos, {Text.fromInt model.done} done</p>
  <al-button variant="outline" click={App.Logout}>Log out</al-button>
  <a href={Todos.Index {}}>Todos</a>
</Layout.Page>
EOF
# the form to change the name; the name must not be empty (Text.NonEmpty)
alder new form Profile Rename name:Text.NonEmpty --msg SaveName
# the update: change the user's name, then show the page again
alder fill --decl Profile.update - <<'EOF'
match msg {
  SaveName p -> {
    let _ <- Db.update User session.user (\u -> { u | name: p.form.name })
    Task.succeed Rerender
  },
}
EOF
# the view with the form
alder fill --decl Profile.view - <<'EOF'
<Layout.Page title="Profile">
  <h1>{model.name}</h1>
  <al-card>
    <p>{model.email}</p>
    <p>{Text.fromInt model.total} todos, {Text.fromInt model.done} done</p>
    <form submit={SaveName}>
      <Errors form />
      <al-input field="name" label="Name" value={model.name} />
      <al-button type="submit">Save name</al-button>
    </form>
  </al-card>
  <al-button variant="outline" click={App.Logout}>Log out</al-button>
  <a href={Todos.Index {}}>Todos</a>
</Layout.Page>
EOF
# tests of the profile page
alder new test Profile "the profile counts only my todos"
alder fill 'Profile."the profile counts only my todos"' - <<'EOF'
{
  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
  let other <- Db.insert User { email: Text.Email "other@example.com", name: Text.NonEmpty "Other", passwordHash: "" }
  let _ <- Db.insert Todo { title: Text.NonEmpty "Milk", due: Nothing, owner: me }
  let _ <- Db.insert Todo { title: Text.NonEmpty "Eggs", done: True, due: Nothing, owner: me }
  let _ <- Db.insert Todo { title: Text.NonEmpty "Bread", done: True, due: Nothing, owner: other }
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
  let refused = match r.answer {
    FormRejected errors -> True,
    _ -> False,
  }
  Task.succeed ({ refused: refused, name: Maybe.map (\u -> u.name) after } == { refused: True, name: Just (Text.NonEmpty "Me") })
}
EOF
alder new test Profile "renaming changes the name"
alder fill 'Profile."renaming changes the name"' - <<'EOF'
{
  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
  let _ <- Test.login { user: Just me }
  let _ <- Test.get (Profile {})
  let r <- Test.submit Rename { name: Text.NonEmpty "Ann" }
  let after <- Db.get User me
  Task.succeed ({ answer: r.answer, name: Maybe.map (\u -> u.name) after } == { answer: Rerendered, name: Just (Text.NonEmpty "Ann") })
}
EOF
# the link from the to-do list to the profile page
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

    <a href={Profile {}}>Profile</a>
    <al-button variant="outline" click={App.Logout}>Log out</al-button>
  </Layout.Page>
EOF
```
