```alder-cli
# the index page first, so the other pages can link to it
alder new page Notes.Index / --blank --requires user

# Notes.Show
alder fill --decl Notes.Show.Model - <<'EOF'
{ note: Note }
EOF
alder fill Notes.Show.load - <<'EOF'
{
  let found <- Db.get Note params.id
  match found {
    Just n -> if n.owner == session.user then Query.succeed { note: n } else Query.notFound,
    Nothing -> Query.notFound,
  }
}
EOF
alder fill Notes.Show.view - <<'EOF'
<Layout.Page title={model.note.title}>
  <h1 id="title">{model.note.title}</h1>
  <p id="body">{model.note.body}</p>
  <a href={Notes.Index {}}>Back</a>
</Layout.Page>
EOF

# Notes.Index
alder fill --decl Notes.Index.Model - <<'EOF'
{ notes: List Note }
EOF
alder fill Notes.Index.load - <<'EOF'
{
  let notes <- Db.from Note |> Db.where (Note.owner.eq session.user) |> Db.orderBy Note.created.desc |> Db.all
  Query.succeed { notes: notes }
}
EOF
alder fill Notes.Index.view - <<'EOF'
<Layout.Page title="Notes">
  <h1>Notes</h1>
</Layout.Page>
EOF
alder new form Notes.Index NewNote title:Text.NonEmpty body:Text --msg Add
alder new msg Notes.Index Delete id:Note.NoteId
alder fill Notes.Index.update.Add - <<'EOF'
{
  let _ <- Db.insert Note { title: p.form.title, body: p.form.body, owner: session.user }
  Task.succeed (Redirect (Notes.Index {}))
}
EOF
alder fill Notes.Index.update.Delete - <<'EOF'
{
  let found <- Db.get Note p.id
  match found {
    Just n -> if n.owner == session.user then {
      let _ <- Db.delete Note p.id
      Task.succeed (Redirect (Notes.Index {}))
    } else Task.notFound,
    Nothing -> Task.notFound,
  }
}
EOF
alder fill --decl Notes.Index.view - <<'EOF'
\model session ->
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

# tests
alder new test Notes.Index "deleting another user's note is not found"
alder fill 'Notes.Index."deleting another user'"'"'s note is not found"' - <<'EOF'
{
  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
  let other <- Db.insert User { email: Text.Email "other@example.com", name: Text.NonEmpty "Other", passwordHash: "" }
  let id <- Db.insert Note { title: Text.NonEmpty "A", body: "x", owner: me }
  let r <- Test.run (update (Delete { id: id }) { notes: [] } { user: other })
  let after <- Db.get Note id
  Task.succeed ({ value: r.value, title: Maybe.map (\n -> n.title) after } == { value: Err "not found", title: Just (Text.NonEmpty "A") })
}
EOF
alder new test Notes.Index "deleting a missing note is not found"
alder fill 'Notes.Index."deleting a missing note is not found"' - <<'EOF'
{
  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
  let r <- Test.run (update (Delete { id: Note.NoteId "nope" }) { notes: [] } { user: me })
  Task.succeed (r.value == Err "not found")
}
EOF
alder new test Notes.Index "the owner can delete a note"
alder fill 'Notes.Index."the owner can delete a note"' - <<'EOF'
{
  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
  let id <- Db.insert Note { title: Text.NonEmpty "A", body: "x", owner: me }
  let r <- Test.run (update (Delete { id: id }) { notes: [] } { user: me })
  let after <- Db.get Note id
  Task.succeed ({ value: r.value, title: Maybe.map (\n -> n.title) after } == { value: Ok (Redirect (Notes.Index {})), title: Nothing })
}
EOF
alder new test Notes.Show "another user's note is not found"
alder fill 'Notes.Show."another user'"'"'s note is not found"' - <<'EOF'
{
  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
  let other <- Db.insert User { email: Text.Email "other@example.com", name: Text.NonEmpty "Other", passwordHash: "" }
  let id <- Db.insert Note { title: Text.NonEmpty "A", body: "x", owner: me }
  let _ <- Test.login { user: Just other }
  let r <- Test.open (Notes.Show { id: id })
  Task.succeed (r.answer == PageMissing)
}
EOF

# login target
alder fill Auth.Login.update.Submit - --hole ?afterLogin <<'EOF'
Notes.Index {}
EOF
alder fill --list
```
