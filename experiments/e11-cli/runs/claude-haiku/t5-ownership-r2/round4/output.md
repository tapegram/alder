```alder-cli
# the note page: its load checks the owner, its view shows the title and body
alder fill --decl Notes.Show.load - <<'EOF'
{
  let found <- Db.get Note params.id
  let mine = Maybe.andThen (\n -> if n.owner == session.user then Just n else Nothing) found
  match mine {
    Just row -> Query.succeed { row: row },
    Nothing -> Query.notFound,
  }
}
EOF
alder fill --decl Notes.Show.view - <<'EOF'
<Layout.Page title={model.row.title}>
  <h1 id="title">{model.row.title}</h1>
  <p id="body">{model.row.body}</p>
  <a href={Notes.Index {}}>Back</a>
</Layout.Page>
EOF

# the holes left in the tests of the notes page
alder fill 'Notes.Index."submitting NewNote"' - --hole ?expected <<'EOF'
Redirected (Notes.Index {})
EOF
alder fill 'Notes.Index."pressing Delete"' - --hole ?expected <<'EOF'
({ answer: r.answer, note: Maybe.map (\n -> n.title) after } == { answer: Redirected (Notes.Index {}), note: Nothing })
EOF

# deleting another user's note, or a missing one, is not found; the owner can delete
alder new test Notes.Index "deleting another users note is not found"
alder fill 'Notes.Index."deleting another users note is not found"' - <<'EOF'
{
  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
  let other <- Db.insert User { email: Text.Email "other@example.com", name: Text.NonEmpty "Other", passwordHash: "" }
  let note1 <- Db.insert Note { title: Text.NonEmpty "Milk", body: "x", owner: other }
  let r <- Test.run (update (Delete { id: note1 }) { notes: [] } { user: me })
  let after <- Db.get Note note1
  Task.succeed ({ value: r.value, note: Maybe.map (\n -> n.title) after } == { value: Err "not found", note: Just (Text.NonEmpty "Milk") })
}
EOF
alder new test Notes.Index "deleting a missing note is not found"
alder fill 'Notes.Index."deleting a missing note is not found"' - <<'EOF'
{
  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
  let r <- Test.run (update (Delete { id: Note.NoteId "missing" }) { notes: [] } { user: me })
  Task.succeed ({ value: r.value } == { value: Err "not found" })
}
EOF
alder new test Notes.Index "the owner can delete a note"
alder fill 'Notes.Index."the owner can delete a note"' - <<'EOF'
{
  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
  let note1 <- Db.insert Note { title: Text.NonEmpty "Milk", body: "x", owner: me }
  let r <- Test.run (update (Delete { id: note1 }) { notes: [] } { user: me })
  let after <- Db.get Note note1
  Task.succeed ({ value: r.value, note: Maybe.map (\n -> n.title) after } == { value: Ok (Redirect (Notes.Index {})), note: Nothing })
}
EOF

alder test
```
