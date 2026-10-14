```alder-cli
# Notes.Index: load, then a placeholder view so the form and msgs can be added
alder fill Notes.Index.load - <<'EOF'
{
  let notes <- Db.mine Note |> Db.orderBy Note.created.desc |> Db.all
  Query.succeed { notes: notes }
}
EOF
alder fill Notes.Index.view - <<'EOF'
Html.none
EOF
alder new form Notes.Index NewNote title:Text.NonEmpty body:Text --msg Submit
alder new msg Notes.Index ToggleTheme
alder fill --decl Notes.Index.update - <<'EOF'
match msg {
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
alder fill --decl Notes.Index.view - <<'EOF'
<html>
  <head>
    <meta charset="utf-8" />
    <title>Notes</title>
    <link rel="stylesheet" href={Url.external "/static/al/al.css"} />
  </head>
  <body class={if Theme.isDark session.theme then Just "dark" else Nothing}>
    <main>
      <h1>Notes</h1>
      <form submit={Submit}>
        <Errors form />
        <al-input field="title" label="Title" />
        <al-textarea field="body" label="Body" />
        <al-button type="submit">Add note</al-button>
      </form>
      <ul id="notes">
        {for n in model.notes -> <li><a href={Notes.Show { id: n.id }}>{n.title}</a></li>}
      </ul>
      <al-button click={ToggleTheme}>{if Theme.isDark session.theme then "Switch to light" else "Switch to dark"}</al-button>
      <al-button variant="outline" click={App.Logout}>Log out</al-button>
    </main>
  </body>
</html>
EOF
alder new test Notes.Index "the theme flips"
alder fill 'Notes.Index."the theme flips"' - <<'EOF'
{
  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: Text.Secret "pw" }
  let _ <- Test.login { user: Just me }
  let _ <- Test.get (Notes.Index {})
  let r <- Test.click ToggleTheme
  let back <- Test.click ToggleTheme
  Task.succeed ({ theme: r.session.theme, dark: Test.has "body.dark" r.doc, after: back.session.theme } == { theme: Dark, dark: True, after: Light })
}
EOF

# Notes.Show
alder fill --decl Notes.Show.Model - <<'EOF'
{ note: Note }
EOF
alder fill Notes.Show.load - <<'EOF'
{
  let note <- Db.getMine Note params.id
  Query.succeed { note: note }
}
EOF
alder fill Notes.Show.view - <<'EOF'
Html.none
EOF
alder new form Notes.Show EditNote title:Text.NonEmpty body:Text --msg Save
alder new msg Notes.Show Delete
alder new msg Notes.Show ToggleTheme
alder fill --decl Notes.Show.update - <<'EOF'
match msg {
  Save p -> {
    let _ <- Db.updateMine Note model.note.id (\n -> { n | title: p.form.title, body: p.form.body })
    Task.succeed (Redirect (Notes.Show { id: model.note.id }))
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
alder fill --decl Notes.Show.view - <<'EOF'
<html>
  <head>
    <meta charset="utf-8" />
    <title>{model.note.title}</title>
    <link rel="stylesheet" href={Url.external "/static/al/al.css"} />
  </head>
  <body class={if Theme.isDark session.theme then Just "dark" else Nothing}>
    <main>
      <h1>{model.note.title}</h1>
      <form submit={Save}>
        <Errors form />
        <al-input field="title" label="Title" value={model.note.title} />
        <al-textarea field="body" label="Body" value={model.note.body} />
        <al-button type="submit">Save</al-button>
      </form>
      <al-button variant="destructive" click={Delete}>Delete</al-button>
      <a href={Notes.Index {}}>Back</a>
      <al-button click={ToggleTheme}>{if Theme.isDark session.theme then "Switch to light" else "Switch to dark"}</al-button>
      <al-button variant="outline" click={App.Logout}>Log out</al-button>
    </main>
  </body>
</html>
EOF
alder new test Notes.Show "a user cannot see another user's note"
alder fill 'Notes.Show."a user cannot see another user'"'"'s note"' - <<'EOF'
{
  let a <- Db.insert User { email: Text.Email "a@example.com", name: Text.NonEmpty "A", passwordHash: Text.Secret "pw" }
  let b <- Db.insert User { email: Text.Email "b@example.com", name: Text.NonEmpty "B", passwordHash: Text.Secret "pw" }
  let id <- Db.insert Note { title: Text.NonEmpty "Secret", body: "", owner: a }
  let _ <- Test.login { user: Just b }
  let r <- Test.open (Notes.Show { id: id })
  Task.succeed (r.answer == PageMissing)
}
EOF

# Login page: theme button, redirect to the notes
alder new msg Auth.Login ToggleTheme
alder fill --decl Auth.Login.update - <<'EOF'
match msg {
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
alder fill --decl Auth.Login.view - <<'EOF'
<html>
  <head>
    <meta charset="utf-8" />
    <title>Log in</title>
    <link rel="stylesheet" href={Url.external "/static/al/al.css"} />
  </head>
  <body class={if Theme.isDark session.theme then Just "dark" else Nothing}>
    <main>
      <h1>Log in</h1>
      <form submit={Submit}>
        <Errors form />
        <al-input field="email" label="Email" />
        <al-input field="password" label="Password" />
        <al-button type="submit">Log in</al-button>
      </form>
      <al-button click={ToggleTheme}>{if Theme.isDark session.theme then "Switch to light" else "Switch to dark"}</al-button>
      <al-button variant="outline" click={App.Logout}>Log out</al-button>
    </main>
  </body>
</html>
EOF
alder fill Auth.Login --hole ?afterLogin - <<'EOF'
Notes.Index {}
EOF
alder fill --list
alder check
```
