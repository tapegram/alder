```alder-cli
# the shared layout: a value `shell` beside Layout.Page, with the theme button, log out, and the body class
alder new decl Layout shell "{ title: Text, dark: Bool, children: List (Html a) } -> Html a"
alder fill Layout.shell - <<'EOF'
\props -> <html>
  <head>
    <meta charset="utf-8" />
    <title>{props.title}</title>
    <link rel="stylesheet" href={Url.external "/static/al/al.css"} />
    <link rel="stylesheet" href={Url.external "/static/app.css"} />
  </head>
  <body class={if props.dark then Just "dark" else Nothing}>
    <al-page width="narrow">
      <nav>
        <al-button id="theme" variant="outline" size="sm" click={App.SwitchTheme}>{if props.dark then "Switch to light" else "Switch to dark"}</al-button>
        <al-button id="logout" variant="ghost" size="sm" click={App.Logout}>Log out</al-button>
      </nav>
      <main>{props.children}</main>
    </al-page>
  </body>
</html>
EOF

# App: the theme flip and the theme test
alder fill App.update.SwitchTheme - --hole ?switchTheme <<'EOF'
{
  let _ <- Session.put (\s -> { s | theme: Theme.flip s.theme })
  Task.succeed Rerender
}
EOF
alder fill 'App."pressing SwitchTheme"' - --hole ?expected <<'EOF'
({ value: r.value, trace: r.trace } == { value: Ok Rerender, trace: ["Session.put"] })
EOF

# the two notes pages (blank), both created before either refers to the other
alder new page Notes.Index / --blank --requires user
alder new page Notes.Show '/notes/{id}' --show Note --blank --requires user

# Notes.Index: load first (naming Note imports it), then the model, then the real load
alder fill Notes.Index.load - --hole ?load <<'EOF'
{
  let _ <- Db.mine Note |> Db.orderBy Note.created.desc |> Db.all
  Query.succeed { }
}
EOF
alder fill --decl Notes.Index.Model - <<'EOF'
{ notes: List Note }
EOF
alder fill --decl Notes.Index.load - <<'EOF'
{
  let notes <-
    Db.mine Note
      |> Db.orderBy Note.created.desc
      |> Db.all
  Query.succeed { notes: notes }
}
EOF

# Notes.Index view, list only (a form cannot be added while view is a hole)
alder fill Notes.Index.view - --hole ?view <<'EOF'
Layout.shell {
  title: "Notes",
  dark: Theme.isDark session.theme,
  children: [
    <h1>Notes</h1>,
    <ul id="notes">
      {for note in model.notes -> <li><a class="title" href={Notes.Show { id: note.id }}>{note.title}</a></li>}
    </ul>,
  ],
}
EOF

# Notes.Index: the form, its message, its update, then the full view
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
Layout.shell {
  title: "Notes",
  dark: Theme.isDark session.theme,
  children: [
    <h1>Notes</h1>,
    <al-card>
      <form submit={Add}>
        <Errors form />
        <Input field="title" label="Title" />
        <Textarea field="body" label="Body" />
        <al-button type="submit">Add</al-button>
      </form>
    </al-card>,
    <ul id="notes">
      {for note in model.notes -> <li><a class="title" href={Notes.Show { id: note.id }}>{note.title}</a></li>}
    </ul>,
  ],
}
EOF

# Notes.Show: model, load, view (without the form), then form and message
alder fill --decl Notes.Show.Model - <<'EOF'
{ note: Note }
EOF
alder fill Notes.Show.load - --hole ?load <<'EOF'
{
  let note <- Db.getMine Note params.id
  Query.succeed { note: note }
}
EOF
alder fill Notes.Show.view - --hole ?view <<'EOF'
Layout.shell {
  title: model.note.title,
  dark: Theme.isDark session.theme,
  children: [
    <p><a href={Notes.Index {}}>Back to notes</a></p>,
    <h1 id="title">{model.note.title}</h1>,
    <p id="body">{model.note.body}</p>,
  ],
}
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
Layout.shell {
  title: model.note.title,
  dark: Theme.isDark session.theme,
  children: [
    <p><a href={Notes.Index {}}>Back to notes</a></p>,
    <h1 id="title">{model.note.title}</h1>,
    <p id="body">{model.note.body}</p>,
    <al-card>
      <form submit={Save}>
        <Errors form />
        <Input field="title" label="Title" value={model.note.title} />
        <Textarea field="body" label="Body" value={model.note.body} />
        <al-button type="submit">Save</al-button>
      </form>
    </al-card>,
    <al-button variant="destructive" click={Delete}>Delete</al-button>,
  ],
}
EOF

# Notes.Show: a user cannot see another user's note
alder new test Notes.Show "a note of another user is not found"
alder fill 'Notes.Show."a note of another user is not found"' - <<'EOF'
{
  let owner <- Db.insert User { email: Text.Email "owner@example.com", name: Text.NonEmpty "Owner", passwordHash: "" }
  let id <- Db.insert Note { title: Text.NonEmpty "Secret", body: "", owner: owner }
  let stranger = User.UserId "someone-else"
  let _ <- Test.login { user: Just stranger }
  let r <- Test.open (Notes.Show { id: id })
  Task.succeed (r.answer == PageMissing)
}
EOF

# Auth.Login: the redirect after login, and the page with the shared layout
alder fill Auth.Login.update.Submit - --hole ?afterLogin <<'EOF'
Notes.Index {}
EOF
alder fill --decl Auth.Login.view - <<'EOF'
Layout.shell {
  title: "Log in",
  dark: Theme.isDark session.theme,
  children: [
    <h1>Log in</h1>,
    <al-card>
      <form submit={Submit}>
        <Errors form />
        <al-input field="email" label="Email" />
        <al-input field="password" label="Password" />
        <al-button type="submit">Log in</al-button>
      </form>
    </al-card>,
  ],
}
EOF

# App: the theme flips, checked through the browser
alder fill --decl 'App."the theme flips"' - <<'EOF'
{
  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
  let _ <- Test.login { user: Just me }
  let _ <- Test.get (Notes.Index {})
  let dark <- Test.click SwitchTheme
  let light <- Test.click SwitchTheme
  Task.succeed ({ dark: dark.session.theme, darkBody: Test.has "body.dark" dark.doc, back: light.session.theme, lightBody: Test.has "body.dark" light.doc } == { dark: Dark, darkBody: True, back: Light, lightBody: False })
}
EOF

# the holes still left (the generated login test's ?afterLogin, by its name)
alder fill --list
```
