```alder-cli
# the theme: App's helpers, used by every page and by Layout
alder new decl App isDark "Theme -> Bool"
alder fill --decl App.isDark - <<'EOF'
\t -> match t { Light -> False, Dark -> True }
EOF
alder new decl App toggled "Theme -> Theme"
alder fill --decl App.toggled - <<'EOF'
\t -> match t { Light -> Dark, Dark -> Light }
EOF
alder new test App "the theme flips"
alder fill 'App."the theme flips"' - <<'EOF'
isDark (toggled Light) && not (isDark (toggled Dark))
EOF

# the layout: the body gets class="dark" when the theme is Dark
alder new decl Layout themed "{ dark: Bool, title: Text, children: List (Html a) } -> Html a"
alder fill --decl Layout.themed - <<'EOF'
\props -> <html>
  <head>
    <meta charset="utf-8" />
    <title>{props.title}</title>
    <link rel="stylesheet" href={Url.external "/static/al/al.css"} />
    <link rel="stylesheet" href={Url.external "/static/app.css"} />
  </head>
  <body class={if props.dark then Just "dark" else Nothing}>
    <al-page width="narrow">
      <main>{props.children}</main>
    </al-page>
  </body>
</html>
EOF

# the notes list (/), requires login
alder new form Notes.Index NewNote title:Text.NonEmpty body:Text
alder new msg Notes.Index ToggleTheme
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
  SubmitNewNote p -> {
    let _ <- Db.insertMine Note { title: p.form.title, body: p.form.body }
    Task.succeed (Redirect (Notes.Index {}))
  },
  ToggleTheme p -> {
    let _ <- Session.put (\s -> { s | theme: App.toggled s.theme })
    Task.succeed Rerender
  },
}
EOF
alder fill --decl Notes.Index.view - <<'EOF'
\model session -> Layout.themed { dark: App.isDark session.theme, title: "Notes", children: [<div>
  <h1>Notes</h1>
  <al-card>
    <form submit={SubmitNewNote}>
      <Errors form />
      <al-input field="title" label="Title" />
      <al-textarea field="body" label="Body" />
      <al-button type="submit">Add note</al-button>
    </form>
  </al-card>
  <ul id="notes">
    {for n in model.notes -> <li><a href={Notes.Show { id: n.id }}>{n.title}</a></li>}
  </ul>
  <al-button variant="outline" click={ToggleTheme}>{if App.isDark session.theme then "Switch to light" else "Switch to dark"}</al-button>
  <al-button variant="outline" click={App.Logout}>Log out</al-button>
</div>] }
EOF
alder fill 'Notes.Index."the switch flips the theme"' - <<'EOF' 2>/dev/null
EOF
alder new test Notes.Index "the switch flips the theme"
alder fill 'Notes.Index."the switch flips the theme"' - <<'EOF'
{
  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
  let _ <- Test.login { user: Just me }
  let _ <- Test.get (Notes.Index {})
  let r <- Test.click ToggleTheme
  let back <- Test.click ToggleTheme
  Task.succeed ({ dark: Test.has "body.dark" r.doc, theme: App.isDark r.session.theme, light: not (Test.has "body.dark" back.doc) } == { dark: True, theme: True, light: True })
}
EOF

# a single note (/notes/{id}), requires login
alder new page Notes.Show '/notes/{id}' --show Note --requires user
alder new form Notes.Show EditNote title:Text.NonEmpty body:Text --msg SaveNote
alder new msg Notes.Show Delete
alder new msg Notes.Show ToggleTheme
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
  SaveNote p -> {
    let _ <- Db.updateMine Note model.note.id (\n -> { n | title: p.form.title, body: p.form.body })
    Task.succeed (Redirect (Notes.Index {}))
  },
  Delete p -> {
    let _ <- Db.deleteMine Note model.note.id
    Task.succeed (Redirect (Notes.Index {}))
  },
  ToggleTheme p -> {
    let _ <- Session.put (\s -> { s | theme: App.toggled s.theme })
    Task.succeed Rerender
  },
}
EOF
alder fill --decl Notes.Show.view - <<'EOF'
\model session -> Layout.themed { dark: App.isDark session.theme, title: model.note.title, children: [<div>
  <h1>{model.note.title}</h1>
  <al-card>
    <form submit={SaveNote}>
      <Errors form />
      <al-input field="title" label="Title" value={model.note.title} />
      <al-textarea field="body" label="Body" value={model.note.body} />
      <al-button type="submit">Save</al-button>
    </form>
  </al-card>
  <al-button variant="destructive" click={Delete}>Delete</al-button>
  <a href={Notes.Index {}}>Back to notes</a>
  <al-button variant="outline" click={ToggleTheme}>{if App.isDark session.theme then "Switch to light" else "Switch to dark"}</al-button>
  <al-button variant="outline" click={App.Logout}>Log out</al-button>
</div>] }
EOF
alder new test Notes.Show "a stranger cannot open a note"
alder fill 'Notes.Show."a stranger cannot open a note"' - <<'EOF'
{
  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
  let id <- Db.insert Note { title: Text.NonEmpty "Secret", body: "", owner: me }
  let _ <- Test.login { user: Just (User.UserId "someone-else") }
  let r <- Test.open (Notes.Show { id: id })
  Task.succeed (r.answer == PageMissing)
}
EOF

# the login page: theme switch and log out too
alder new msg Auth.Login ToggleTheme
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
    let _ <- Session.put (\s -> { s | theme: App.toggled s.theme })
    Task.succeed Rerender
  },
}
EOF
alder fill --decl Auth.Login.view - <<'EOF'
\model session -> Layout.themed { dark: App.isDark session.theme, title: "Log in", children: [<div>
  <h1>Log in</h1>
  <al-card>
    <form submit={Submit}>
      <Errors form />
      <al-input field="email" label="Email" />
      <al-input field="password" label="Password" />
      <al-button type="submit">Log in</al-button>
    </form>
  </al-card>
  <al-button variant="outline" click={ToggleTheme}>{if App.isDark session.theme then "Switch to light" else "Switch to dark"}</al-button>
  <al-button variant="outline" click={App.Logout}>Log out</al-button>
</div>] }
EOF
```
