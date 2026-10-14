```alder-cli
# the profile's name is a Text.NonEmpty, as the user's row holds it, so the form and the input agree
alder fill --decl Profile.Model - <<'EOF'
{ name: Text.NonEmpty, email: Text.Email, total: Int, done: Int }
EOF
# the view: the name input is the current name, and an empty name shows its error under the input
alder fill --decl Profile.view - <<'EOF'
<Layout.Page title="Profile">
  <h1>{model.name}</h1>
  <p>{model.email}</p>
  <p>{Text.fromInt model.total} todos, {Text.fromInt model.done} done</p>
  <al-card>
    <h2 slot="header">Rename</h2>
    <form submit={SaveName}>
      <Errors form />
      <al-input field="name" label="Name" value={model.name} />
      <Errors field="name" />
      <al-button type="submit">Save name</al-button>
    </form>
  </al-card>
  <al-button variant="outline" click={App.Logout}>Log out</al-button>
  <a href={Todos.Index {}}>Todos</a>
</Layout.Page>
EOF
# the rename test: a valid name is saved and the page is re-rendered
alder fill 'Profile."submitting Rename"' - <<'EOF'
{
  let me <- Db.insert User { email: Text.Email "email-1@example.com", name: Text.NonEmpty "name-1", passwordHash: "passwordhash-1" }
  let _ <- Test.login { user: Just me }
  let _ <- Test.get (Profile {})
  let r <- Test.submit Rename { name: Text.NonEmpty "Ann" }
  let after <- Db.get User me
  Task.succeed ({ answer: r.answer, name: Maybe.map (\u -> u.name) after } == { answer: Rerendered, name: Just (Text.NonEmpty "Ann") })
}
EOF
# the load test: only the signed-in user's todos are counted
alder new test Profile "counts only the todos of the signed-in user"
alder fill 'Profile."counts only the todos of the signed-in user"' - <<'EOF'
{
  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
  let other <- Db.insert User { email: Text.Email "other@example.com", name: Text.NonEmpty "Other", passwordHash: "" }
  let _ <- Db.insert Todo { title: Text.NonEmpty "Milk", due: Nothing, owner: me }
  let _ <- Db.insert Todo { title: Text.NonEmpty "Bread", done: True, due: Nothing, owner: me }
  let _ <- Db.insert Todo { title: Text.NonEmpty "Eggs", done: True, due: Nothing, owner: other }
  let _ <- Test.login { user: Just me }
  let page <- Test.get (Profile {})
  Task.succeed ({ total: page.model.total, done: page.model.done } == { total: 2, done: 1 })
}
EOF
# the link from the to-do list to the profile page (the whole view, with one link added)
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
