```alder-cli
# the notes list page
alder new page Notes.Index / --requires user --blank
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
<Layout.Page title="Notes">
  <h1>Notes</h1>
</Layout.Page>
EOF
alder new form Notes.Index NewNote title:Text.NonEmpty body:Text --msg Add
alder new msg Notes.Index Delete id:Note.NoteId
alder fill --decl Notes.Index.update - <<'EOF'
match msg {
  Add p -> {
    let _ <- Db.insertMine Note { title: p.form.title, body: p.form.body }
    Task.succeed (Redirect (Notes.Index {}))
  },
  Delete p -> {
    let _ <- Db.deleteMine Note p.id
    Task.succeed (Redirect (Notes.Index {}))
  },
}
EOF
alder fill --decl Notes.Index.view - <<'EOF'
<Layout.Page title="Notes">
  <h1>Notes</h1>
  <al-card>
    <form submit={Add}>
      <Errors form />
      <al-input field="title" label="Title" />
      <al-textarea field="body" label="Body" />
      <al-button type="submit">Add</al-button>
    </form>
  </al-card>
  <ul id="notes">
    {for n in model.notes ->
      <li>
        <a class="title" href={Notes.Show { id: n.id }}>{n.title}</a>
        <al-button variant="ghost" size="sm" click={Delete { id: n.id }}>Delete</al-button>
      </li>}
  </ul>
  <al-button variant="outline" click={App.Logout}>Log out</al-button>
</Layout.Page>
EOF

# the note's page
alder fill Notes.Show.view - <<'EOF'
<Layout.Page title={model.note.title}>
  <h1 id="title">{model.note.title}</h1>
  <p id="body">{model.note.body}</p>
  <a href={Notes.Index {}}>Back</a>
</Layout.Page>
EOF

# tests of deleting
alder new test Notes.Index "deleting another user's note is not found"
alder fill 'Notes.Index."deleting another user'"'"'s note is not found"' - <<'EOF'
{
  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
  let other <- Db.insert User { email: Text.Email "other@example.com", name: Text.NonEmpty "Other", passwordHash: "" }
  let id <- Db.insert Note { title: Text.NonEmpty "Theirs", body: "b", owner: other }
  let _ <- Test.login { user: Just me }
  let r <- Test.run (update (Delete { id: id }) { notes: [] } { user: me })
  let after <- Db.get Note id
  Task.succeed ({ value: r.value, kept: Maybe.isJust after } == { value: Err "not found", kept: True })
}
EOF
```
