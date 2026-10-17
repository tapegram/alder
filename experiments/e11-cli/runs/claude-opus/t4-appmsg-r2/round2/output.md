```alder-cli
# Notes.Index: a placeholder view the generators can extend, then the form
alder fill --decl Notes.Index.view - <<'EOF'
<Layout.Page title="Notes">
  <h1>Notes</h1>
</Layout.Page>
EOF
alder new form Notes.Index NewNote title:Text.NonEmpty body:Text --msg Add
alder fill Notes.Index.update.Add - <<'EOF'
{
  let _ <- Db.insertMine Note { title: p.form.title, body: p.form.body }
  Task.succeed (Redirect (Notes.Index {}))
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
    <Errors field="body" />
    <button>Add</button>
  </form>,
  <ul id="notes">
    {for n in model.notes -> <li><a class="title" href={Notes.Show { id: n.id }}>{n.title}</a></li>}
  </ul>
] }
EOF
alder new test Notes.Index "switching the theme"
alder fill 'Notes.Index."switching the theme"' - <<'EOF'
{
  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
  let _ <- Test.login { user: Just me }
  let before <- Test.get (Notes.Index {})
  let r <- Test.click App.SwitchTheme
  let after <- Test.get (Notes.Index {})
  Task.succeed ({ before: Test.text "#theme" before.doc, light: Test.has "body.dark" before.doc, theme: r.session.theme, label: Test.text "#theme" after.doc, dark: Test.has "body.dark" after.doc } == { before: Just "Switch to dark", light: False, theme: Dark, label: Just "Switch to light", dark: True })
}
EOF

# Notes.Show
alder new page Notes.Show "/notes/{id}" --blank --param id:Note.NoteId --requires user
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
alder fill Notes.Show.update.Save - <<'EOF'
{
  let _ <- Db.updateMine Note model.note.id (\n -> { n | title: p.form.title, body: p.form.body })
  Task.succeed (Redirect (Notes.Show { id: model.note.id }))
}
EOF
alder fill Notes.Show.update.Delete - <<'EOF'
{
  let _ <- Db.deleteMine Note model.note.id
  Task.succeed (Redirect (Notes.Index {}))
}
EOF
alder fill --decl Notes.Show.view - <<'EOF'
Layout.shell { title: model.note.title, dark: Theme.isDark session.theme, children: [
  <h1 id="title">{model.note.title}</h1>,
  <p id="body">{model.note.body}</p>,
  <form submit={Save}>
    <Errors form />
    <Input field="title" label="Title" value={model.note.title} />
    <Errors field="title" />
    <Textarea field="body" label="Body" value={model.note.body} />
    <Errors field="body" />
    <button>Save</button>
  </form>,
  <button click={Delete}>Delete</button>
] }
EOF
alder new test Notes.Show "another user's note is not found"
alder fill 'Notes.Show."another user's note is not found"' - <<'EOF'
{
  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
  let other <- Db.insert User { email: Text.Email "other@example.com", name: Text.NonEmpty "Other", passwordHash: "" }
  let id <- Db.insert Note { title: Text.NonEmpty "Secret", body: "x", owner: me }
  let _ <- Test.login { user: Just other }
  let r <- Test.open (Notes.Show { id: id })
  Task.succeed (r.answer == PageMissing)
}
EOF

# App's SwitchTheme test
alder fill 'App."pressing SwitchTheme"' - <<'EOF'
{ value: r.value, trace: r.trace } == { value: Ok Rerender, trace: ["Session.put"] }
EOF

# Login page: themed view, and where to go after login
alder fill --decl Auth.Login.view - <<'EOF'
Layout.shell { title: "Log in", dark: Theme.isDark session.theme, children: [
  <h1>Log in</h1>,
  <form submit={Submit}>
    <Errors form />
    <Input field="email" label="Email" />
    <Errors field="email" />
    <Input field="password" label="Password" />
    <Errors field="password" />
    <button>Log in</button>
  </form>
] }
EOF
alder fill Auth.Login.update - --hole ?afterLogin <<'EOF'
Notes.Index {}
EOF
alder fill --list
alder fill 'Auth.Login."logging in through the form"' - --hole ?afterLogin <<'EOF'
Notes.Index {}
EOF
alder fill --list
alder check
```
