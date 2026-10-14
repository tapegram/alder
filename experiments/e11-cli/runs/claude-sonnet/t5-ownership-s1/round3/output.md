```alder-cli
alder fill --decl Notes.Index.view - <<'EOF'
<Layout.Page title="Notes">
  <h1>Notes</h1>
  <form submit={Add}>
    <Errors form />
    <Input field="title" label="Title" />
    <Textarea field="body" label="Body" />
    <button>Add</button>
  </form>
  <ul id="notes">
    {for n in model.notes -> <li><a class="title" href={Notes.Show { id: n.id }}>{n.title}</a><button click={Delete { id: n.id }}>Delete</button></li>}
  </ul>
  <button click={App.Logout}>Log out</button>
</Layout.Page>
EOF
alder fill Notes.Index."submitting NewNote" - --hole ?expected <<'EOF'
Redirected (Notes.Index {})
EOF
alder fill Notes.Index."pressing Delete" - --hole ?expected <<'EOF'
{ answer: r.answer, gone: after == Nothing } == { answer: Redirected (Notes.Index {}), gone: True }
EOF
alder fill Auth.Login.update.Submit - --hole ?afterLogin <<'EOF'
Notes.Index {}
EOF
alder new test Notes.Index "deleting another user's note is not found"
alder fill Notes.Index."deleting another user's note is not found" - <<'EOF'
{
  let a <- Db.insert User { email: Text.Email "a@example.com", name: Text.NonEmpty "A", passwordHash: "" }
  let b <- Db.insert User { email: Text.Email "b@example.com", name: Text.NonEmpty "B", passwordHash: "" }
  let n <- Db.insert Note { title: Text.NonEmpty "T", body: "x", owner: a }
  let _ <- Test.login { user: Just b }
  let r <- Test.run (update (Delete { id: n }) { notes: [] } { user: b })
  let after <- Db.get Note n
  Task.succeed ({ value: r.value, kept: after != Nothing } == { value: Err "not found", kept: True })
}
EOF
alder new test Notes.Index "deleting a missing note is not found"
alder fill Notes.Index."deleting a missing note is not found" - <<'EOF'
{
  let a <- Db.insert User { email: Text.Email "a@example.com", name: Text.NonEmpty "A", passwordHash: "" }
  let _ <- Test.login { user: Just a }
  let r <- Test.run (update (Delete { id: Note.NoteId "missing" }) { notes: [] } { user: a })
  Task.succeed (r.value == Err "not found")
}
EOF
alder new test Notes.Index "the owner can delete a note"
alder fill Notes.Index."the owner can delete a note" - <<'EOF'
{
  let a <- Db.insert User { email: Text.Email "a@example.com", name: Text.NonEmpty "A", passwordHash: "" }
  let n <- Db.insert Note { title: Text.NonEmpty "T", body: "x", owner: a }
  let _ <- Test.login { user: Just a }
  let r <- Test.run (update (Delete { id: n }) { notes: [] } { user: a })
  let after <- Db.get Note n
  Task.succeed ({ value: r.value, gone: after == Nothing } == { value: Ok (Redirect (Notes.Index {})), gone: True })
}
EOF
alder fill --list
```
