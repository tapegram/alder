```alder-cli
# the profile page: logged-in users only, at /profile
alder new page Profile /profile --blank --requires user

# the model: the user's name and email, and how many todos they have and how many are done
alder fill --decl Profile.Model - <<'EOF'
{ name: Text.NonEmpty, email: Text.Email, total: Int, done: Int }
EOF

# load reads the user's row and only their todos (Db.mine)
alder fill --decl Profile.load - <<'EOF'
\params session -> {
  let found <- Db.get User session.user
  let todos <- Db.mine Todo |> Db.all
  match found {
    Just me -> Query.succeed {
      name: me.name,
      email: me.email,
      total: List.length todos,
      done: List.length (List.filter (\t -> t.done) todos),
    },
    Nothing -> Query.notFound,
  }
}
EOF

# a placeholder view, so the form can be generated (form refuses while view is a hole)
alder fill --decl Profile.view - <<'EOF'
\model session -> <Layout.Page title="Profile"><h1>Profile</h1></Layout.Page>
EOF

# the name form: the field must not be empty (Text.NonEmpty refuses a blank name)
alder new form Profile Rename name:Text.NonEmpty

# the full view: the details, the rename form, and the app's logout button
alder fill --decl Profile.view - <<'EOF'
\model session -> <Layout.Page title="Profile">
    <h1>Profile</h1>
    <al-card>
      <dl>
        <dt>Name</dt>
        <dd>{model.name}</dd>
        <dt>Email</dt>
        <dd>{model.email}</dd>
        <dt>Todos</dt>
        <dd>{Text.fromInt model.total} ({Text.fromInt model.done} done)</dd>
      </dl>
    </al-card>
    <al-card>
      <form submit={SubmitRename}>
        <Errors form />
        <al-input field="name" label="Name" value={model.name} />
        <Errors field="name" />
        <al-button type="submit">Save name</al-button>
      </form>
    </al-card>
    <al-button variant="outline" click={App.Logout}>Log out</al-button>
    <a href={Todos.Index {}}>Back to todos</a>
  </Layout.Page>
EOF

# update: a rename writes the user's row, then goes back to the profile
alder fill --decl Profile.update - <<'EOF'
\msg model session -> match msg {
  SubmitRename p -> {
    let _ <- Db.update User session.user (\u -> { u | name: p.form.name })
    Task.succeed (Redirect (Profile {}))
  },
}
EOF

# tests of the profile page
alder new test Profile "the profile counts only my todos"
alder fill 'Profile."the profile counts only my todos"' - <<'EOF'
{
  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
  let other <- Db.insert User { email: Text.Email "other@example.com", name: Text.NonEmpty "Other", passwordHash: "" }
  let _ <- Db.insert Todo { title: Text.NonEmpty "Milk", done: True, due: Nothing, owner: me }
  let _ <- Db.insert Todo { title: Text.NonEmpty "Bread", due: Nothing, owner: me }
  let _ <- Db.insert Todo { title: Text.NonEmpty "Eggs", due: Nothing, owner: other }
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

alder new test Profile "renaming changes the name"
alder fill 'Profile."renaming changes the name"' - <<'EOF'
{
  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
  let _ <- Test.login { user: Just me }
  let _ <- Test.get (Profile {})
  let r <- Test.submit Rename { name: Text.NonEmpty "Ann" }
  let after <- Db.get User me
  Task.succeed ({ answer: r.answer, name: Maybe.map (\u -> u.name) after } == { answer: Redirected (Profile {}), name: Just (Text.NonEmpty "Ann") })
}
EOF

alder new test Profile "a visitor is sent to log in"
alder fill 'Profile."a visitor is sent to log in"' - <<'EOF'
{
  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
  let r <- Test.open (Profile {})
  Task.succeed (r.answer == Redirected (Auth.Login {}))
}
EOF

# the todo list links to the profile
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

alder new test Todos.Index "the todo list links to the profile"
alder fill 'Todos.Index."the todo list links to the profile"' - <<'EOF'
{
  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
  let _ <- Test.login { user: Just me }
  let page <- Test.get (Todos.Index {})
  Task.succeed (Test.texts "p a" page.doc == ["Your profile"])
}
EOF
```
