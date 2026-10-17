```alder-cli
# Notes.Index: the add arm, the delete arm, and the list
alder fill Notes.Index.update.Add - --hole ?add <<'EOF'
{
  let _ <- Db.insert Note { title: p.form.title, body: p.form.body, owner: session.user }
  Task.succeed (Redirect (Notes.Index {}))
}
EOF
alder fill Notes.Index.update.Delete - --hole ?delete <<'EOF'
{
  let found <- Db.from Note |> Db.where (Note.owner.eq session.user) |> Db.where (Note.id.eq p.id) |> Db.first
  match found {
    Just n -> {
      let _ <- Db.delete Note n.id
      Task.succeed (Redirect (Notes.Index {}))
    },
    Nothing -> Task.notFound,
  }
}
EOF
alder fill --decl Notes.Index.view - <<'EOF'
<Layout.Page title="Notes">
  <h1>Notes</h1>

  <al-card>
    <form submit={Add}>
      <al-input field="title" label="Title" />
      <Errors field="title" />
      <al-textarea field="body" label="Body" />
      <Errors form />
      <al-button type="submit">Add</al-button>
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

# Notes.Index tests: fill the holes left by the generated tests
alder fill 'Notes.Index."submitting NewNote"' - --hole ?expected <<'EOF'
Redirected (Notes.Index {})
EOF
alder fill 'Notes.Index."pressing Delete"' - --hole ?expected <<'EOF'
({ answer: r.answer, note: Maybe.map (\n -> n.title) after } == { answer: Redirected (Notes.Index {}), note: Nothing })
EOF

# Notes.Index: a stranger cannot delete, and a missing id is not found
alder new test Notes.Index "deleting a note of another user is not found"
alder fill 'Notes.Index."deleting a note of another user is not found"' - <<'EOF'
{
  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "passwordhash-1" }
  let other <- Db.insert User { email: Text.Email "other@example.com", name: Text.NonEmpty "Other", passwordHash: "passwordhash-2" }
  let note <- Db.insert Note { title: Text.NonEmpty "Secret", body: "b", owner: other }
  let r <- Test.run (update (Delete { id: note }) { notes: [] } { user: me })
  let after <- Db.get Note note
  Task.succeed ({ value: r.value, title: Maybe.map (\n -> n.title) after } == { value: Err "not found", title: Just (Text.NonEmpty "Secret") })
}
EOF
alder new test Notes.Index "deleting a missing note is not found"
alder fill 'Notes.Index."deleting a missing note is not found"' - <<'EOF'
{
  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "passwordhash-1" }
  let r <- Test.run (update (Delete { id: Note.NoteId "missing" }) { notes: [] } { user: me })
  Task.succeed (r.value == Err "not found")
}
EOF

# Notes.Show: the load checks the owner, the view names the title and body
alder fill --decl Notes.Show.load - <<'EOF'
{
  let found <- Db.from Note |> Db.where (Note.owner.eq session.user) |> Db.where (Note.id.eq params.id) |> Db.first
  match found {
    Just row -> Query.succeed { row: row },
    Nothing -> Query.notFound,
  }
}
EOF
alder fill --decl Notes.Show.view - <<'EOF'
<Layout.Page title={model.row.title}>
  <h1 id="title">{model.row.title}</h1>
  <p id="body">{model.row.body}</p>
  <p><a href={Notes.Index {}}>Back to notes</a></p>
  <al-button variant="outline" click={App.Logout}>Log out</al-button>
</Layout.Page>
EOF

# Notes.Show tests
alder new test Notes.Show "a note of another user is not found"
alder fill 'Notes.Show."a note of another user is not found"' - <<'EOF'
{
  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "passwordhash-1" }
  let other <- Db.insert User { email: Text.Email "other@example.com", name: Text.NonEmpty "Other", passwordHash: "passwordhash-2" }
  let note <- Db.insert Note { title: Text.NonEmpty "Secret", body: "b", owner: other }
  let _ <- Test.login { user: Just me }
  let r <- Test.open (Notes.Show { id: note })
  Task.succeed (r.answer == PageMissing)
}
EOF
alder new test Notes.Show "the owner sees the note"
alder fill 'Notes.Show."the owner sees the note"' - <<'EOF'
{
  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "passwordhash-1" }
  let note <- Db.insert Note { title: Text.NonEmpty "Milk", body: "Two litres", owner: me }
  let _ <- Test.login { user: Just me }
  let page <- Test.get (Notes.Show { id: note })
  Task.succeed ({ title: Test.text "#title" page.doc, body: Test.text "#body" page.doc } == { title: Just "Milk", body: Just "Two litres" })
}
EOF
```
