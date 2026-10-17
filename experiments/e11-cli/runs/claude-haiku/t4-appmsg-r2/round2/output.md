```alder-cli
# the note table, owned by its user (@owner scopes Db.mine and friends to the signed-in user)
alder new table Note title:Text.NonEmpty body:Text owner:User.UserId@ref=User@owner created:Time@created --no-timestamps

# the theme: Theme.flip and Theme.isDark name Theme, so pages and App import it
alder new decl Theme flip "Theme -> Theme"
alder fill Theme.flip - <<'EOF'
\t -> match t {
  Light -> Dark,
  Dark -> Light,
}
EOF
alder new decl Theme isDark "Theme -> Bool"
alder fill Theme.isDark - <<'EOF'
\t -> match t {
  Light -> False,
  Dark -> True,
}
EOF

# a shared shell with the theme button and the log out button; it takes `dark` so
# Layout does not need to import Theme
alder new decl Layout Shell "{ title: Text, dark: Bool, children: List (Html a) } -> Html a"
alder fill Layout.Shell - <<'EOF'
\props ->
  <html>
    <head>
      <meta charset="utf-8" />
      <title>{props.title}</title>
      <link rel="stylesheet" href={Url.external "/static/al/al.css"} />
      <link rel="stylesheet" href={Url.external "/static/app.css"} />
    </head>
    <body class={if props.dark then "dark" else ""}>
      <al-page width="narrow">
        <main>
          <nav>
            <al-button id="theme" variant="outline" click={App.SwitchTheme}>{if props.dark then "Switch to light" else "Switch to dark"}</al-button>
            <al-button id="logout" variant="outline" click={App.Logout}>Log out</al-button>
          </nav>
          {props.children}
        </main>
      </al-page>
    </body>
  </html>
EOF

# App's error page uses the new shell; naming Theme.isDark imports Theme into App
alder fill --decl App.error - <<'EOF'
\err -> <Layout.Shell title="Error" dark={Theme.isDark Light}>
  <h1>Something went wrong</h1>
  <p>Please try again.</p>
</Layout.Shell>
EOF

# the old Logout test builds a session literal, which the new field would break: stub it first
alder fill --decl 'App."Logout clears the user and goes to log in"' - <<'EOF'
True
EOF

# the theme in the session
alder edit session add-field theme:Theme.Theme=Light

# the app's messages
alder new msg App SwitchTheme
alder fill --decl App.update - <<'EOF'
\msg session -> match msg {
  SwitchTheme p -> {
    let _ <- Session.put (\s -> { s | theme: Theme.flip s.theme })
    Task.succeed Rerender
  },
  Logout p -> {
    let _ <- Session.put (\s -> { s | user: Nothing })
    Task.succeed (Redirect (Auth.Login {}))
  },
}
EOF

alder fill --decl 'App."Logout clears the user and goes to log in"' - <<'EOF'
{
  let r <- Test.run (update Logout { user: Just (User.UserId "me"), theme: Light })
  Task.succeed ({ value: r.value, trace: r.trace } == { value: Ok (Redirect (Auth.Login {})), trace: ["Session.put"] })
}
EOF

alder new test App "the theme button flips the theme"
alder fill 'App."the theme button flips the theme"' - <<'EOF'
{
  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
  let _ <- Test.login { user: Just me, theme: Light }
  let _ <- Test.get (Notes.Index {})
  let r <- Test.click SwitchTheme
  Task.succeed ({ answer: r.answer, theme: r.session.theme, label: Test.text "#theme" r.doc } == { answer: Rerendered, theme: Dark, label: Just "Switch to light" })
}
EOF

# the notes list
alder new page Notes.Index "/" --list Note --requires user
alder new form Notes.Index NewNote title:Text.NonEmpty body:Text --msg Add
alder fill --decl Notes.Index.Model - <<'EOF'
{ notes: List Note }
EOF
alder fill --decl Notes.Index.load - <<'EOF'
\params session -> {
  let notes <- Db.mine Note |> Db.orderBy Note.created.desc |> Db.all
  Query.succeed { notes: notes }
}
EOF
alder fill --decl Notes.Index.update - <<'EOF'
\msg model session -> match msg {
  Add p -> {
    let _ <- Db.insertMine Note { title: p.form.title, body: p.form.body }
    Task.succeed (Redirect (Notes.Index {}))
  },
}
EOF
alder fill --decl Notes.Index.view - <<'EOF'
\model session ->
  <Layout.Shell title="Notes" dark={Theme.isDark session.theme}>
    <h1>Notes</h1>
    <al-card>
      <form submit={Add}>
        <al-input field="title" label="Title" />
        <al-textarea field="body" label="Body" />
        <al-button type="submit">Add</al-button>
      </form>
    </al-card>
    <ul id="notes">
      {for n in model.notes -> <li><a class="title" href={Notes.Show { id: n.id }}>{n.title}</a></li>}
    </ul>
  </Layout.Shell>
EOF

# one note: its page, its edit form, delete
alder new page Notes.Show "/notes/{id}" --show Note --param id:Note.NoteId --requires user
alder new form Notes.Show EditNote title:Text.NonEmpty body:Text --msg Save
alder new msg Notes.Show Delete
alder fill --decl Notes.Show.Model - <<'EOF'
{ note: Note }
EOF
alder fill --decl Notes.Show.load - <<'EOF'
\params session -> {
  let note <- Db.getMine Note params.id
  Query.succeed { note: note }
}
EOF
alder fill --decl Notes.Show.update - <<'EOF'
\msg model session -> match msg {
  Save p -> {
    let _ <- Db.updateMine Note model.note.id (\n -> { n | title: p.form.title, body: p.form.body })
    Task.succeed (Redirect (Notes.Show { id: model.note.id }))
  },
  Delete p -> {
    let _ <- Db.deleteMine Note model.note.id
    Task.succeed (Redirect (Notes.Index {}))
  },
}
EOF
alder fill --decl Notes.Show.view - <<'EOF'
\model session ->
  <Layout.Shell title={model.note.title} dark={Theme.isDark session.theme}>
    <h1 id="title">{model.note.title}</h1>
    <al-card>
      <form submit={Save}>
        <al-input field="title" label="Title" value={model.note.title} />
        <al-textarea field="body" label="Body" value={model.note.body} />
        <al-button type="submit">Save</al-button>
      </form>
      <div slot="actions">
        <al-button variant="ghost" href={Notes.Index {}}>Back</al-button>
        <al-button variant="destructive" click={Delete}>Delete</al-button>
      </div>
    </al-card>
    <p id="body">{model.note.body}</p>
  </Layout.Shell>
EOF

alder new test Notes.Show "a stranger gets not found for a note"
alder fill 'Notes.Show."a stranger gets not found for a note"' - <<'EOF'
{
  let ann <- Db.insert User { email: Text.Email "ann@example.com", name: Text.NonEmpty "Ann", passwordHash: "" }
  let bob <- Db.insert User { email: Text.Email "bob@example.com", name: Text.NonEmpty "Bob", passwordHash: "" }
  let id <- Db.insert Note { title: Text.NonEmpty "Secret", body: "", owner: bob }
  let _ <- Test.login { user: Just ann }
  let r <- Test.open (Notes.Show { id: id })
  Task.succeed (r.answer == PageMissing)
}
EOF

alder new test Notes.Show "the owner sees the title"
alder fill 'Notes.Show."the owner sees the title"' - <<'EOF'
{
  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
  let id <- Db.insert Note { title: Text.NonEmpty "Milk", body: "2 litres", owner: me }
  let _ <- Test.login { user: Just me }
  let page <- Test.get (Notes.Show { id: id })
  Task.succeed ({ title: Test.text "#title" page.doc, body: Test.text "#body" page.doc } == { title: Just "Milk", body: Just "2 litres" })
}
EOF

# the login page: its view uses the shell, and it goes to the notes list after logging in
alder fill --decl Auth.Login.view - <<'EOF'
\model session ->
  <Layout.Shell title="Log in" dark={Theme.isDark session.theme}>
    <h1>Log in</h1>
    <al-card>
      <form submit={Submit}>
        <Errors form />
        <al-input field="email" label="Email" />
        <al-input field="password" label="Password" />
        <al-button type="submit">Log in</al-button>
      </form>
    </al-card>
  </Layout.Shell>
EOF
alder fill Auth.Login.update.Submit - --hole ?afterLogin <<'EOF'
Notes.Index {}
EOF

alder migrate
alder test
```
