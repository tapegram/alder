```alder-cli
# the note's own page
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
    <p id="body">{model.note.body}</p>,
    <form submit={Save}>
      <Errors form />
      <al-input field="title" label="Title" value={model.note.title} />
      <al-textarea field="body" label="Body" value={model.note.body} />
      <al-button type="submit">Save</al-button>
    </form>,
    <button click={Delete}>Delete</button>
  ]
}
EOF
alder new test Notes.Show "another user's note is not found"
alder fill 'Notes.Show."another user'"'"'s note is not found"' - <<'EOF'
{
  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
  let other <- Db.insert User { email: Text.Email "other@example.com", name: Text.NonEmpty "Other", passwordHash: "" }
  let id <- Db.insert Note { title: Text.NonEmpty "Secret", body: "x", owner: other }
  let _ <- Test.login { user: Just me }
  let r <- Test.open (Notes.Show { id: id })
  Task.succeed (r.answer == PageMissing)
}
EOF

# the list page
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

# login
alder fill Auth.Login.update - --hole ?afterLogin <<'EOF'
Notes.Index {}
EOF
alder fill --list
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
alder check
```
