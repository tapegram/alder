```alder-cli
# theme helpers
alder new decl Theme flip "Theme -> Theme"
alder fill Theme.flip - <<'EOF'
\t -> if t == Dark then Light else Dark
EOF
alder new decl Theme light "Theme"
alder fill Theme.light - <<'EOF'
Light
EOF
alder new decl Theme dark "Theme"
alder fill Theme.dark - <<'EOF'
Dark
EOF

# layout with theme class on body
alder new decl Layout Themed "{ title: Text, dark: Bool, children: List (Html a) } -> Html a"
alder fill Layout.Themed - <<'EOF'
\props ->
  <html>
    <head>
      <meta charset="utf-8" />
      <title>{props.title}</title>
    </head>
    <body class={if props.dark then "dark" else ""}>
      <main>{props.children}</main>
    </body>
  </html>
EOF

# the notes list page
alder remove page Notes.Index
alder new page Notes.Index / --blank --requires user
alder fill --decl Notes.Index.Model - <<'EOF'
{ notes: List Note }
EOF
alder fill Notes.Index.load - <<'EOF'
\params session -> {
  let notes <- Db.mine Note |> Db.orderBy Note.created.desc |> Db.all
  Query.succeed { notes: notes }
}
EOF
alder fill Notes.Index.view - <<'EOF'
\model session -> <p>Notes</p>
EOF
alder new form Notes.Index NewNote title:Text.NonEmpty body:Text --msg Submit
alder new msg Notes.Index ToggleTheme
alder fill --decl Notes.Index.update - <<'EOF'
\msg model session -> match msg {
  Submit p -> {
    let _ <- Db.insertMine Note { title: p.form.title, body: p.form.body }
    Task.succeed (Redirect (Notes.Index {}))
  },
  ToggleTheme p -> {
    let _ <- Session.put (\s -> { s | theme: Theme.flip s.theme })
    Task.succeed Rerender
  },
}
EOF

# the note page
alder new page Notes.Show /notes/{id} --blank --requires user --param id:Note.NoteId
alder fill --decl Notes.Show.Model - <<'EOF'
{ note: Note }
EOF
alder fill Notes.Show.load - <<'EOF'
\params session -> {
  let note <- Db.getMine Note params.id
  Query.succeed { note: note }
}
EOF
alder fill Notes.Show.view - <<'EOF'
\model session -> <p>Note</p>
EOF
alder new form Notes.Show EditNote title:Text.NonEmpty body:Text --msg Save
alder new msg Notes.Show Delete
alder new msg Notes.Show ToggleTheme
alder fill --decl Notes.Show.update - <<'EOF'
\msg model session -> match msg {
  Save p -> {
    let _ <- Db.updateMine Note model.note.id (\n -> { n | title: p.form.title, body: p.form.body })
    Task.succeed (Redirect (Notes.Index {}))
  },
  Delete p -> {
    let _ <- Db.deleteMine Note model.note.id
    Task.succeed (Redirect (Notes.Index {}))
  },
  ToggleTheme p -> {
    let _ <- Session.put (\s -> { s | theme: Theme.flip s.theme })
    Task.succeed Rerender
  },
}
EOF

# login: after-login page, theme button
alder fill Auth.Login.update --hole ?afterLogin - <<'EOF'
Notes.Index {}
EOF
alder new msg Auth.Login ToggleTheme

# final views
alder fill --decl Notes.Index.view - <<'EOF'
\model session ->
  <Layout.Themed title="Notes" dark={Theme.isDark session.theme}>
    <h1>Notes</h1>
    <button click={ToggleTheme}>{if Theme.isDark session.theme then "Switch to light" else "Switch to dark"}</button>
    <button click={App.Logout}>Log out</button>
    <form submit={Submit}>
      <Errors form />
      <Input field="title" label="Title" />
      <Textarea field="body" label="Body" />
      <button>Add</button>
    </form>
    <ul id="notes">
      {for n in model.notes -> <li><a href={Notes.Show { id: n.id }}>{n.title}</a></li>}
    </ul>
  </Layout.Themed>
EOF
alder fill --decl Notes.Show.view - <<'EOF'
\model session ->
  <Layout.Themed title={model.note.title} dark={Theme.isDark session.theme}>
    <h1>{model.note.title}</h1>
    <a href={Notes.Index {}}>Back</a>
    <button click={ToggleTheme}>{if Theme.isDark session.theme then "Switch to light" else "Switch to dark"}</button>
    <button click={App.Logout}>Log out</button>
    <form submit={Save}>
      <Errors form />
      <Input field="title" label="Title" value={model.note.title} />
      <Textarea field="body" label="Body" value={model.note.body} />
      <button>Save</button>
    </form>
    <button click={Delete}>Delete</button>
  </Layout.Themed>
EOF
alder fill --decl Auth.Login.view - <<'EOF'
\model session ->
  <Layout.Themed title="Log in" dark={Theme.isDark session.theme}>
    <h1>Log in</h1>
    <button click={ToggleTheme}>{if Theme.isDark session.theme then "Switch to light" else "Switch to dark"}</button>
    <button click={App.Logout}>Log out</button>
    <form submit={Submit}>
      <Errors form />
      <Input field="email" label="Email" />
      <Input field="password" label="Password" />
      <button>Log in</button>
    </form>
  </Layout.Themed>
EOF
alder fill --decl Auth.Login.update - <<'EOF'
\msg model session -> match msg {
  Submit p -> {
    let found <- Password.verify { email: p.form.email, password: p.form.password }
    match found {
      Just id -> {
        let _ <- Session.put (\s -> { s | user: Just id })
        Task.succeed (Redirect (Notes.Index {}))
      },
      Nothing -> Task.succeed (Reject "Email or password is incorrect."),
    }
  },
  ToggleTheme p -> {
    let _ <- Session.put (\s -> { s | theme: Theme.flip s.theme })
    Task.succeed Rerender
  },
}
EOF

# tests
alder new test Notes.Show "a stranger gets not found"
alder fill 'Notes.Show."a stranger gets not found"' - <<'EOF'
{
  let a <- Db.insert User { email: Text.Email "a@example.com", name: Text.NonEmpty "A", passwordHash: Text.Secret "x" }
  let b <- Db.insert User { email: Text.Email "b@example.com", name: Text.NonEmpty "B", passwordHash: Text.Secret "x" }
  let id <- Db.insert Note { title: Text.NonEmpty "Secret", body: "", owner: a }
  let _ <- Test.login { user: Just b }
  let r <- Test.open (Notes.Show { id: id })
  Task.succeed (r.answer == PageMissing)
}
EOF
alder new test Notes.Index "the theme flips"
alder fill 'Notes.Index."the theme flips"' - <<'EOF'
{
  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: Text.Secret "x" }
  let _ <- Test.login { user: Just me }
  let _ <- Test.get (Notes.Index {})
  let r <- Test.click ToggleTheme
  let back <- Test.click ToggleTheme
  Task.succeed ({ theme: r.session.theme, dark: Test.has "body.dark" r.doc, back: back.session.theme } == { theme: Theme.dark, dark: True, back: Theme.light })
}
EOF
alder fill --list
alder check
```
