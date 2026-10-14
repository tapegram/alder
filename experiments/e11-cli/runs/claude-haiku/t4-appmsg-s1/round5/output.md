```alder-cli
# the shell: Layout.shell sets the body's class from the theme
alder new decl Layout shell "{ title: Text, dark: Bool, body: Html a } -> Html a"
alder fill --decl Layout.shell - <<'EOF'
\props -> {
  <html>
    <head>
      <meta charset="utf-8" />
      <title>{props.title}</title>
      <link rel="stylesheet" href={Url.external "/static/al/al.css"} />
      <link rel="stylesheet" href={Url.external "/static/app.css"} />
    </head>
    <body class={if props.dark then "dark" else ""}>
      <al-page width="narrow">
        <main>{props.body}</main>
      </al-page>
    </body>
  </html>
}
EOF

# the theme test in App
alder fill 'App."the theme flips"' - --hole ?test <<'EOF'
Task.succeed ({ first: toggled Light, second: toggled (toggled Light) } == { first: Dark, second: Light })
EOF

# the notes list: the user's notes, newest first, with a form to add one
alder fill --decl Notes.Index.Model - <<'EOF'
{ notes: List Note }
EOF
alder fill --decl Notes.Index.load - <<'EOF'
\params session -> {
  let notes <- Db.mine Note |> Db.orderBy Note.created.desc |> Db.all
  Query.succeed { notes: notes }
}
EOF
# a placeholder view, so the form and the message can be made
alder fill --decl Notes.Index.view - <<'EOF'
\model session -> Layout.shell { title: "Notes", dark: App.isDark session.theme, body: <p>Notes</p> }
EOF
alder new form Notes.Index NewNote title:Text.NonEmpty body:Text --into Note
alder new msg Notes.Index ToggleTheme
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
\model session -> {
  let dark = App.isDark session.theme
  Layout.shell {
    title: "Notes",
    dark: dark,
    body: <div>
      <h1>Notes</h1>
      <al-button variant="outline" click={ToggleTheme}>{if dark then "Switch to light" else "Switch to dark"}</al-button>
      <al-button variant="outline" click={App.Logout}>Log out</al-button>
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
    </div>,
  }
}
EOF
alder new test Notes.Index "the newest note is listed first"
alder fill 'Notes.Index."the newest note is listed first"' - <<'EOF'
{
  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
  let _ <- Db.insert Note { title: Text.NonEmpty "First", body: "", owner: me }
  let _ <- Db.insert Note { title: Text.NonEmpty "Second", body: "", owner: me }
  let _ <- Test.login { user: Just me }
  let page <- Test.get (Notes.Index {})
  Task.succeed (Test.texts "#notes a" page.doc == ["Second", "First"])
}
EOF
alder new test Notes.Index "the theme button flips the theme"
alder fill 'Notes.Index."the theme button flips the theme"' - <<'EOF'
{
  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
  let _ <- Test.login { user: Just me }
  let _ <- Test.get (Notes.Index {})
  let r <- Test.click ToggleTheme
  Task.succeed ({ answer: r.answer, theme: r.session.theme, dark: Test.has "body.dark" r.doc } == { answer: Rerendered, theme: Dark, dark: True })
}
EOF

# one note: the user's own, edited or deleted here; another user's is not found
alder new page Notes.Show '/notes/{id}' --show Note --requires user
alder fill --decl Notes.Show.view - <<'EOF'
\model session -> Layout.shell { title: "Note", dark: App.isDark session.theme, body: <p>Note</p> }
EOF
alder new msg Notes.Show ToggleTheme
alder new msg Notes.Show Delete
alder new form Notes.Show EditNote title:Text.NonEmpty body:Text --msg Save
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
  ToggleTheme p -> {
    let _ <- Session.put (\s -> { s | theme: App.toggled s.theme })
    Task.succeed Rerender
  },
}
EOF
alder fill --decl Notes.Show.view - <<'EOF'
\model session -> {
  let dark = App.isDark session.theme
  Layout.shell {
    title: model.note.title,
    dark: dark,
    body: <div>
      <p><a href={Notes.Index {}}>All notes</a></p>
      <al-button variant="outline" click={ToggleTheme}>{if dark then "Switch to light" else "Switch to dark"}</al-button>
      <al-button variant="outline" click={App.Logout}>Log out</al-button>
      <al-card>
        <form submit={Save}>
          <Errors form />
          <al-input field="title" label="Title" value={model.note.title} />
          <al-textarea field="body" label="Body" value={model.note.body} />
          <al-button type="submit">Save</al-button>
        </form>
      </al-card>
      <al-button variant="destructive" click={Delete}>Delete</al-button>
    </div>,
  }
}
EOF
alder new test Notes.Show "a note of another user is not found"
alder fill 'Notes.Show."a note of another user is not found"' - <<'EOF'
{
  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
  let other <- Db.insert User { email: Text.Email "other@example.com", name: Text.NonEmpty "Other", passwordHash: "" }
  let id <- Db.insert Note { title: Text.NonEmpty "Private", body: "", owner: other }
  let _ <- Test.login { user: Just me }
  let r <- Test.open (Notes.Show { id: id })
  Task.succeed (r.answer == PageMissing)
}
EOF
alder new test Notes.Show "saving changes the title and body"
alder fill 'Notes.Show."saving changes the title and body"' - <<'EOF'
{
  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
  let id <- Db.insert Note { title: Text.NonEmpty "Milk", body: "", owner: me }
  let _ <- Test.login { user: Just me }
  let _ <- Test.get (Notes.Show { id: id })
  let r <- Test.submit EditNote { title: Text.NonEmpty "Bread", body: "Two loaves" }
  let after <- Db.get Note id
  Task.succeed ({ answer: r.answer, title: Maybe.map (\n -> n.title) after, body: Maybe.map (\n -> n.body) after } == { answer: Redirected (Notes.Show { id: id }), title: Just (Text.NonEmpty "Bread"), body: Just "Two loaves" })
}
EOF
alder new test Notes.Show "delete removes the note"
alder fill 'Notes.Show."delete removes the note"' - <<'EOF'
{
  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
  let id <- Db.insert Note { title: Text.NonEmpty "Milk", body: "", owner: me }
  let _ <- Test.login { user: Just me }
  let _ <- Test.get (Notes.Show { id: id })
  let r <- Test.click Delete
  let after <- Db.get Note id
  Task.succeed ({ answer: r.answer, note: Maybe.map (\n -> n.title) after } == { answer: Redirected (Notes.Index {}), note: Nothing })
}
EOF

# the login page: a theme button, and the shell
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
\model session -> {
  let dark = App.isDark session.theme
  Layout.shell {
    title: "Log in",
    dark: dark,
    body: <div>
      <h1>Log in</h1>
      <al-card>
        <form submit={Submit}>
          <Errors form />
          <al-input field="email" label="Email" />
          <al-input field="password" label="Password" />
          <al-button type="submit">Log in</al-button>
        </form>
      </al-card>
      <al-button variant="outline" click={ToggleTheme}>{if dark then "Switch to light" else "Switch to dark"}</al-button>
    </div>,
  }
}
EOF
```
