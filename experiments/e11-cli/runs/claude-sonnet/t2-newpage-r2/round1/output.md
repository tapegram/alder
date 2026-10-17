```alder-cli
# the old Admin pages do not check (Notice, al-dialog): replace those bodies first
alder fill --decl Admin.Todo.Bulk.update - <<'EOF'
\msg model session -> match msg {
  BulkDelete p -> {
    let allowed = List.filter (\row -> refused model session row == False) model.rows
    let _ <- Task.all (List.map (\row -> Db.delete Todo row.id) allowed)
    Task.succeed (Redirect (Admin.Todo.Index {}))
  },
  BulkSetDone p -> {
    let _ <- Task.all (List.map (\row -> Db.update Todo row.id (\r -> { r | done: p.form.value })) model.rows)
    Task.succeed (Redirect (Admin.Todo.Index {}))
  },
}
EOF
alder fill --decl Admin.User.Bulk.update - <<'EOF'
\msg model session -> match msg {
  BulkDelete p -> {
    let allowed = List.filter (\row -> refused model session row == False) model.rows
    let _ <- Task.all (List.map (\row -> Db.delete User row.id) allowed)
    Task.succeed (Redirect (Admin.User.Index {}))
  },
}
EOF
alder fill --decl Admin.Todo.Show.view - <<'EOF'
\model session ->
  <Admin.Layout.Page title="Todo">
    <h1>Todo</h1>
    <al-card>
      <dl>
        <dt>Id</dt>
        <dd>{model.row.id}</dd>
        <dt>Title</dt>
        <dd>{model.row.title}</dd>
        <dt>Done</dt>
        <dd>{if model.row.done then <al-badge>Yes</al-badge> else <al-badge variant="secondary">No</al-badge>}</dd>
        <dt>Due</dt>
        <dd>{match model.row.due { Just v -> Date.format "Jan 2, 2006" v, Nothing -> "" }}</dd>
        <dt>Owner</dt>
        <dd><a href={Admin.User.Show { id: model.row.owner }}>{ownerLabel model.labels model.row.owner}</a></dd>
        <dt>Created</dt>
        <dd>{Time.format "Jan 2, 2006" model.row.created}</dd>
      </dl>
      <div slot="actions">
        <al-button href={Admin.Todo.Edit { id: model.row.id }}>Edit</al-button>
        <al-button variant="destructive" href={Admin.Todo.Delete { id: model.row.id }}>Delete</al-button>
      </div>
    </al-card>
    <a href={Admin.Todo.Index {}}>All todos</a>
  </Admin.Layout.Page>
EOF
alder fill --decl Admin.User.Show.view - <<'EOF'
\model session ->
  <Admin.Layout.Page title="User">
    <h1>User</h1>
    <al-card>
      <dl>
        <dt>Id</dt>
        <dd>{model.row.id}</dd>
        <dt>Email</dt>
        <dd>{model.row.email}</dd>
        <dt>Name</dt>
        <dd>{model.row.name}</dd>
        <dt>Created</dt>
        <dd>{Time.format "Jan 2, 2006" model.row.created}</dd>
        <dt>Role</dt>
        <dd><al-badge>{match model.row.role { Member -> "Member", Admin -> "Admin" }}</al-badge></dd>
      </dl>
      <div slot="actions">
        <al-button href={Admin.User.Edit { id: model.row.id }}>Edit</al-button>
        <al-button variant="destructive" href={Admin.User.Delete { id: model.row.id }}>Delete</al-button>
      </div>
    </al-card>
    <a href={Admin.User.Index {}}>All users</a>
  </Admin.Layout.Page>
EOF
alder check
# the profile page
alder new page Profile /profile --blank --requires user
alder fill --decl Profile.Model - <<'EOF'
{ name: Text.NonEmpty, email: Text.Email, total: Int, done: Int }
EOF
alder fill Profile.load - <<'EOF'
\params session -> {
  let me <- Db.get User session.user
  let total <- Db.mine Todo |> Db.count
  let doneCount <- Db.mine Todo |> Db.where (Todo.done.eq True) |> Db.count
  match me {
    Just u -> Query.succeed { name: u.name, email: u.email, total: total, done: doneCount },
    Nothing -> Query.notFound,
  }
}
EOF
alder fill Profile.view - <<'EOF'
\model session ->
  <Layout.Page title="Profile">
    <h1>Profile</h1>
    <p>{model.name}</p>
  </Layout.Page>
EOF
alder new form Profile ChangeName name:Text.NonEmpty
alder fill --decl Profile.update - <<'EOF'
\msg model session -> match msg {
  SubmitChangeName p -> {
    let _ <- Db.update User session.user (\u -> { u | name: p.form.name })
    Task.succeed (Redirect (Profile {}))
  },
}
EOF
alder fill --decl Profile.view - <<'EOF'
\model session ->
  <Layout.Page title="Profile">
    <h1>Profile</h1>
    <p id="name">{model.name}</p>
    <p id="email">{model.email}</p>
    <p id="total">{Text.fromInt model.total} todos</p>
    <p id="done">{Text.fromInt model.done} done</p>
    <al-card>
      <form submit={SubmitChangeName}>
        <Errors form />
        <al-input field="name" label="Name" value={model.name} />
        <al-button type="submit">Change name</al-button>
      </form>
    </al-card>
    <a href={Todos.Index {}}>Back to todos</a>
    <al-button variant="outline" click={App.Logout}>Log out</al-button>
  </Layout.Page>
EOF
# link from the Index page
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
# tests
alder new test Profile "load counts only the user's todos"
alder fill 'Profile."load counts only the user'"'"'s todos"' - <<'EOF'
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
alder new test Profile "renaming to an empty name is refused"
alder fill 'Profile."renaming to an empty name is refused"' - <<'EOF'
{
  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
  let _ <- Test.login { user: Just me }
  let _ <- Test.get (Profile {})
  let r <- Test.submitRaw ChangeName [{ name: "name", value: "" }]
  let after <- Db.get User me
  Task.succeed ({ answer: r.answer, name: Maybe.map (\u -> u.name) after } == { answer: FormRejected [{ field: "name", message: "must not be empty" }], name: Just (Text.NonEmpty "Me") })
}
EOF
alder test
```
