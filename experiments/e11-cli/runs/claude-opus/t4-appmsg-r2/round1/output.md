```alder-cli
# theme helpers
alder new decl Theme isDark "Theme -> Bool"
alder fill Theme.isDark - <<'EOF'
\t -> t == Dark
EOF

# the app-level theme switch
alder new msg App SwitchTheme
alder fill App.update.SwitchTheme - <<'EOF'
{
  let _ <- Session.put (\s -> { s | theme: Theme.flip s.theme })
  Task.succeed Rerender
}
EOF

# the layout every page uses: theme button, log out, body class
alder new decl Layout shell "{ title: Text, dark: Bool, children: List (Html a) } -> Html a"
alder fill Layout.shell - <<'EOF'
\props -> <html>
  <head>
    <meta charset="utf-8" />
    <title>{props.title}</title>
  </head>
  <body class={if props.dark then Just "dark" else Nothing}>
    <header>
      <button id="theme" click={App.SwitchTheme}>{if props.dark then "Switch to light" else "Switch to dark"}</button>
      <button click={App.Logout}>Log out</button>
    </header>
    <main>{props.children}</main>
  </body>
</html>
EOF

# notes
alder new table Note title:Text.NonEmpty body:Text owner:User.UserId@ref=User@owner created:Time@created --no-timestamps

# the list page
alder new page Notes.Index / --blank --requires user
alder fill --decl Notes.Index.Model - <<'EOF'
{ notes: List Note }
EOF
alder fill Notes.Index.load - <<'EOF'
{
  let notes <- Db.mine Note |> Db.orderBy Note.created.desc |> Db.all
  Query.succeed { notes: notes }
}
EOF
alder fill Notes.Index.view - <<'EOF'
Layout.shell { title: "Notes", dark: Theme.isDark session.theme, children: [] }
EOF
alder new form Notes.Index NewNote title:Text.NonEmpty body:Text --msg Add
alder fill --decl Notes.Index.update - <<'EOF'
match msg {
  Add p -> {
    let _ <- Db.insertMine Note { title: p.form.title, body: p.form.body }
    Task.succeed (Redirect (Notes.Index {}))
  },
}
EOF
alder fill --decl Notes.Index.view - <<'EOF'
Layout.shell { title: "Notes", dark: Theme.isDark session.theme, children: [
  <h1>Notes</h1>,
  <form submit={Add}>
    <Errors form />
    <Input field="title" label="Title" />
    <Errors field="title" />
    <Textarea field="body" label="Body" />
    <button>Add</button>
  </form>,
  <ul id="notes">
    {for n in model.notes -> <li><a class="title" href={Notes.Show { id: n.id }}>{n.title}</a></li>}
  </ul>,
] }
EOF

# the note page
alder new page Notes.Show /notes/{id} --param id:Note.NoteId --blank --requires user
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
Layout.shell { title: model.note.title, dark: Theme.isDark session.theme, children: [] }
EOF
alder new form Notes.Show EditNote title:Text.NonEmpty body:Text --msg Save
alder new msg Notes.Show Delete
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
}
EOF
alder fill --decl Notes.Show.view - <<'EOF'
Layout.shell { title: model.note.title, dark: Theme.isDark session.theme, children: [
  <h1 id="title">{model.note.title}</h1>,
  <div id="body">{model.note.body}</div>,
  <form submit={Save}>
    <Errors form />
    <Input field="title" label="Title" value={model.note.title} />
    <Errors field="title" />
    <Textarea field="body" label="Body" value={model.note.body} />
    <button>Save</button>
  </form>,
  <button click={Delete}>Delete</button>,
  <a href={Notes.Index {}}>All notes</a>,
] }
EOF
alder new test Notes.Show "a stranger cannot see my note"
alder fill 'Notes.Show."a stranger cannot see my note"' - <<'EOF'
{
  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
  let other <- Db.insert User { email: Text.Email "other@example.com", name: Text.NonEmpty "Other", passwordHash: "" }
  let id <- Db.insert Note { title: Text.NonEmpty "Secret", body: "", owner: me }
  let _ <- Test.login { user: Just other }
  let r <- Test.open (Notes.Show { id: id })
  Task.succeed (r.answer == PageMissing)
}
EOF

# the login page
alder fill --list
alder fill Auth.Login.update.Submit - --hole ?afterLogin <<'EOF'
Notes.Index {}
EOF
alder fill 'Auth.Login."logging in through the form"' - --hole ?afterLogin <<'EOF'
Notes.Index {}
EOF
alder fill --decl Auth.Login.view - <<'EOF'
Layout.shell { title: "Log in", dark: Theme.isDark session.theme, children: [
  <h1>Log in</h1>,
  <form submit={Submit}>
    <Errors form />
    <Input field="email" label="Email" />
    <Input field="password" label="Password" />
    <button>Log in</button>
  </form>,
] }
EOF

# the theme switch through the browser
alder new test App "the theme button flips the theme"
alder fill 'App."the theme button flips the theme"' - <<'EOF'
{
  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
  let _ <- Test.login { user: Just me }
  let before <- Test.get (Notes.Index {})
  let dark <- Test.click SwitchTheme
  let light <- Test.click SwitchTheme
  Task.succeed ({ before: Test.text "#theme" before.doc, theme: dark.session.theme, label: Test.text "#theme" dark.doc, isDark: Test.has "body.dark" dark.doc, back: light.session.theme, backDark: Test.has "body.dark" light.doc } == { before: Just "Switch to dark", theme: Dark, label: Just "Switch to light", isDark: True, back: Light, backDark: False })
}
EOF
alder check
alder test
```
