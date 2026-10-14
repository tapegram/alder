```alder-cli
# the detail page for one note, owned by the signed-in user
alder new page Notes.Show '/notes/{id}' --param id:Note.NoteId --requires user --blank
alder fill --decl Notes.Show.Model - <<'EOF'
{ note: Note }
EOF
alder fill --decl Notes.Show.load - <<'EOF'
{
  let note <- Db.getMine Note params.id
  Query.succeed { note: note }
}
EOF
alder fill --decl Notes.Show.view - <<'EOF'
<Layout.Page title={model.note.title}>
  <h1 id="title">{model.note.title}</h1>
  <p id="body">{model.note.body}</p>
  <a href={Notes.Index {}}>Back to notes</a>
</Layout.Page>
EOF
alder new test Notes.Show "the owner sees the note"
alder fill 'Notes.Show."the owner sees the note"' - <<'EOF'
{
  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
  let note1 <- Db.insert Note { title: Text.NonEmpty "Milk", body: "2 litres", owner: me }
  let _ <- Test.login { user: Just me }
  let page <- Test.get (Notes.Show { id: note1 })
  Task.succeed ({ title: Test.text "#title" page.doc, body: Test.text "#body" page.doc } == { title: Just "Milk", body: Just "2 litres" })
}
EOF
alder new test Notes.Show "a stranger gets not found"
alder fill 'Notes.Show."a stranger gets not found"' - <<'EOF'
{
  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
  let other <- Db.insert User { email: Text.Email "other@example.com", name: Text.NonEmpty "Other", passwordHash: "" }
  let note1 <- Db.insert Note { title: Text.NonEmpty "Milk", body: "", owner: me }
  let _ <- Test.login { user: Just other }
  let r <- Test.open (Notes.Show { id: note1 })
  Task.succeed (r.answer == PageMissing)
}
EOF

# the list page: my notes, the add form, a Delete button per note
alder fill --decl Notes.Index.view - <<'EOF'
<Layout.Page title="Notes">
  <h1>Notes</h1>

  <al-card>
    <form submit={Add}>
      <Errors form />
      <al-input field="title" label="Title" />
      <al-textarea field="body" label="Body" />
      <al-button type="submit">Add note</al-button>
    </form>
  </al-card>

  <ul id="notes">
    {for note in model.notes ->
      <li>
        <a class="title" href={Notes.Show { id: note.id }}>{note.title}</a>
        <al-button variant="ghost" size="sm" click={Delete { id: note.id }}>Delete</al-button>
      </li>}
  </ul>

  <al-button variant="outline" click={App.Logout}>Log out</al-button>
</Layout.Page>
EOF
alder new test Notes.Index "the list shows only my notes"
alder fill 'Notes.Index."the list shows only my notes"' - <<'EOF'
{
  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
  let other <- Db.insert User { email: Text.Email "other@example.com", name: Text.NonEmpty "Other", passwordHash: "" }
  let _ <- Db.insert Note { title: Text.NonEmpty "mine", body: "", owner: me }
  let _ <- Db.insert Note { title: Text.NonEmpty "theirs", body: "", owner: other }
  let _ <- Test.login { user: Just me }
  let page <- Test.get (Notes.Index {})
  Task.succeed (Test.texts "#notes a.title" page.doc == ["mine"])
}
EOF
alder new test Notes.Index "a stranger cannot delete a note"
alder fill 'Notes.Index."a stranger cannot delete a note"' - <<'EOF'
{
  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
  let stranger <- Db.insert User { email: Text.Email "other@example.com", name: Text.NonEmpty "Other", passwordHash: "" }
  let note1 <- Db.insert Note { title: Text.NonEmpty "Milk", body: "2 litres", owner: me }
  let _ <- Test.login { user: Just stranger }
  let r <- Test.run (update (Delete { id: note1 }) { notes: [] } { user: stranger })
  let after <- Db.get Note note1
  Task.succeed ({ value: r.value, kept: Maybe.isJust after } == { value: Err "not found", kept: True })
}
EOF
alder new test Notes.Index "a missing id is not found"
alder fill 'Notes.Index."a missing id is not found"' - <<'EOF'
{
  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
  let _ <- Test.login { user: Just me }
  let r <- Test.run (update (Delete { id: Note.NoteId "missing" }) { notes: [] } { user: me })
  Task.succeed (r.value == Err "not found")
}
EOF
alder fill 'Notes.Index."pressing Delete"' - --hole ?expected <<'EOF'
r.answer == Redirected (Notes.Index {}) && after == Nothing
EOF

# login: after logging in, go to the notes list
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
}
EOF
alder fill 'Auth.Login."logging in through the form"' - --hole ?afterLogin <<'EOF'
Notes.Index {}
EOF
```
