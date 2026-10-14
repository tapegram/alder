```alder-cli
# the rename test: the generator left a hole for the expected answer, so give it a real check
alder fill 'Profile."submitting Rename"' - <<'EOF'
{
  let me <- Db.insert User { email: Text.Email "email-1@example.com", name: Text.NonEmpty "name-1", passwordHash: "passwordhash-1" }
  let _ <- Test.login { user: Just me }
  let _ <- Test.get (Profile {})
  let r <- Test.submit Rename { name: Text.NonEmpty "name-2" }
  let after <- Db.get User me
  Task.succeed ({ answer: r.answer, name: Maybe.map (\u -> u.name) after } == { answer: Rerendered, name: Just (Text.NonEmpty "name-2") })
}
EOF

# the view: the name field starts with the current name; the errors come from al-input itself
alder fill --decl Profile.view - <<'EOF'
<Layout.Page title="Profile">
  <h1>{model.name}</h1>
  <p>{model.email}</p>
  <p>{Text.fromInt model.total} todos, {Text.fromInt model.done} done</p>
  <al-card>
    <h2 slot="header">Rename</h2>
    <form submit={SaveName}>
      <al-input field="name" label="Name" value={model.name} />
      <al-alert variant="destructive"><Errors form /></al-alert>
      <al-button type="submit">Rename</al-button>
    </form>
  </al-card>
  <al-button variant="outline" click={App.Logout}>Log out</al-button>
  <a href={Todos.Index {}}>Todos</a>
</Layout.Page>
EOF

# the load test: counts only the signed-in user's todos
alder new test Profile "the profile counts only my todos"
alder fill 'Profile."the profile counts only my todos"' - <<'EOF'
{
  let me <- Db.insert User { email: Text.Email "email-1@example.com", name: Text.NonEmpty "name-1", passwordHash: "passwordhash-1" }
  let other <- Db.insert User { email: Text.Email "email-2@example.com", name: Text.NonEmpty "name-2", passwordHash: "passwordhash-2" }
  let _ <- Db.insert Todo { title: Text.NonEmpty "Milk", due: Nothing, owner: me }
  let _ <- Db.insert Todo { title: Text.NonEmpty "Bread", due: Nothing, owner: me, done: True }
  let _ <- Db.insert Todo { title: Text.NonEmpty "Eggs", due: Nothing, owner: other, done: True }
  let _ <- Test.login { user: Just me }
  let page <- Test.get (Profile {})
  Task.succeed ({ total: page.model.total, done: page.model.done } == { total: 2, done: 1 })
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
