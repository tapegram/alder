```alder-cli
# App: the SwitchTheme test's expectation
alder fill 'App."pressing SwitchTheme"' - --hole ?expected <<'EOF'
{ value: r.value, trace: r.trace } == { value: Ok Rerender, trace: ["Session.put"] }
EOF

# Notes.Index: the list, the form, the theme and logout buttons
alder fill --decl Notes.Index.view - <<'EOF'
Layout.shell {
  title: "Notes",
  dark: Theme.isDark session.theme,
  children: [
    <h1>Notes</h1>,
    <form submit={Add}>
      <Errors form />
      <al-input field="title" label="Title" />
      <al-textarea field="body" label="Body" />
      <al-button type="submit">Add note</al-button>
    </form>,
    <ul id="notes">
      {for n in model.notes -> <li><a class="title" href={Notes.Show { id: n.id }}>{n.title}</a></li>}
    </ul>
  ]
}
EOF

# Notes.Show
alder new page Notes.Show '/notes/{id}' --param id:Note.NoteId --requires user --blank
alder fill --decl Notes.Show.Model - <<'EOF'
{ note: Note }
EOF
alder fill Notes.Show.load - <<'EOF'
{
  let note <- Db.getMine Note params.id
  Query.succeed { note: note }
}
EOF
alder fill --decl Notes.Show.view - <<'EOF'
<Layout.Page title="Note">
  <h1>Note</h1>
</Layout.Page>
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
    <h1 id="title">{model.note.title}</h1>,
    <div id="body">{model.note.body}</div>,
    <form submit={Save}>
      <Errors form />
      <al-input field="title" label="Title" value={model.note.title} />
      <al-textarea field="body" label="Body" value={model.note.body} />
      <al-button type="submit">Save</al-button>
    </form>,
    <button click={Delete}>Delete</button>,
    <a href={Notes.Index {}}>Back</a>
  ]
}
EOF
alder new test Notes.Show "a user cannot see another user's note"
alder fill 'Notes.Show."a user cannot see another user'"'"'s note"' - <<'EOF'
{
  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
  let other <- Db.insert User { email: Text.Email "other@example.com", name: Text.NonEmpty "Other", passwordHash: "" }
  let id <- Db.insert Note { title: Text.NonEmpty "Secret", body: "mine", owner: me }
  let _ <- Test.login { user: Just other }
  let r <- Test.open (Notes.Show { id: id })
  Task.succeed (r.answer == PageMissing)
}
EOF

# App: the theme button flips the theme through the browser
alder new test App "the theme button flips the theme"
alder fill 'App."the theme button flips the theme"' - <<'EOF'
{
  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
  let _ <- Test.login { user: Just me }
  let _ <- Test.get (Notes.Index {})
  let r <- Test.click SwitchTheme
  Task.succeed ({ theme: r.session.theme, label: Test.text "#theme" r.doc, dark: Test.attr "class" "body" r.doc } == { theme: Dark, label: Just "Switch to light", dark: Just "dark" })
}
EOF

# Login: the theme and logout buttons, and where it goes after login
alder fill --decl Auth.Login.view - <<'EOF'
Layout.shell {
  title: "Log in",
  dark: Theme.isDark session.theme,
  children: [
    <h1>Log in</h1>,
    <form submit={Submit}>
      <Errors form />
      <al-input field="email" label="Email" />
      <al-input field="password" label="Password" />
      <al-button type="submit">Log in</al-button>
    </form>
  ]
}
EOF
alder fill Auth.Login.update - --hole ?afterLogin <<'EOF'
Notes.Index {}
EOF
alder fill --list
alder fill 'Auth.Login."logging in through the form"' - --hole ?afterLogin <<'EOF'
Notes.Index {}
EOF
alder check
alder test
```
