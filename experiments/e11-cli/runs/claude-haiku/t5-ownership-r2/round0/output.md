```alder-cli
# the users, and logging in
alder new table User email:Text.Email@unique name:Text.NonEmpty passwordHash:Text created:Time@created --no-timestamps
alder new auth --users User

# the notes, each with its owner
alder new table Note title:Text.NonEmpty body:Text owner:User.UserId@ref=User created:Time@created --no-timestamps

# the list page: a placeholder view first, since a form cannot be added while the view is a hole
alder new page Notes.Index / --blank --requires user
alder fill Notes.Index.view - --hole ?view <<'EOF'
<Layout.Page title="Notes">
  <h1>Notes</h1>
</Layout.Page>
EOF
alder new form Notes.Index NewNote title:Text.NonEmpty body:Text --msg Add
alder new msg Notes.Index Delete id:Note.NoteId

# the model, the query, and the full view
alder fill --decl Notes.Index.Model - <<'EOF'
{ notes: List Note }
EOF
alder fill Notes.Index.load - --hole ?load <<'EOF'
{
  let notes <-
    Db.from Note
      |> Db.where (Note.owner.eq session.user)
      |> Db.orderBy Note.created.desc
      |> Db.all
  Query.succeed { notes: notes }
}
EOF
alder fill --decl Notes.Index.view - <<'EOF'
<Layout.Page title="Notes">
  <h1>Notes</h1>

  <al-card>
    <form submit={Add}>
      <al-input field="title" label="Title" />
      <al-textarea field="body" label="Body" />
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

# the messages: add a note, delete a note of the signed-in user only
alder fill Notes.Index.update.Add - <<'EOF'
{
  let _ <- Db.insert Note { title: p.form.title, body: p.form.body, owner: session.user }
  Task.succeed (Redirect (Notes.Index {}))
}
EOF
alder fill Notes.Index.update.Delete - <<'EOF'
{
  let found <- Db.get Note p.id
  let owned = Maybe.withDefault False (Maybe.map (\n -> n.owner == session.user) found)
  if owned then {
    let _ <- Db.delete Note p.id
    Task.succeed (Redirect (Notes.Index {}))
  } else Task.notFound
}
EOF

# the note page: requires login, and another user's note is not found
alder new page Notes.Show '/notes/{id}' --blank --param id:Note.NoteId --requires user
alder fill --decl Notes.Show.Model - <<'EOF'
{ note: Note }
EOF
alder fill Notes.Show.load - --hole ?load <<'EOF'
{
  let found <- Db.get Note params.id
  match found {
    Just n -> if n.owner == session.user then Query.succeed { note: n } else Query.notFound,
    Nothing -> Query.notFound,
  }
}
EOF
alder fill Notes.Show.view - --hole ?view <<'EOF'
<Layout.Page title={model.note.title}>
  <h1 id="title">{model.note.title}</h1>
  <p id="body">{model.note.body}</p>
  <a href={Notes.Index {}}>Back to notes</a>
</Layout.Page>
EOF

# after logging in, go to the list
alder fill Auth.Login.update.Submit - --hole ?afterLogin <<'EOF'
Notes.Index {}
EOF

# tests of the list page
alder new test Notes.Index "a stranger cannot delete a note"
alder fill 'Notes.Index."a stranger cannot delete a note"' - <<'EOF'
{
  let ann <- Db.insert User { email: Text.Email "ann@example.com", name: Text.NonEmpty "Ann", passwordHash: "" }
  let bob <- Db.insert User { email: Text.Email "bob@example.com", name: Text.NonEmpty "Bob", passwordHash: "" }
  let id <- Db.insert Note { title: Text.NonEmpty "Milk", body: "", owner: ann }
  let r <- Test.run (update (Delete { id: id }) { notes: [] } { user: bob })
  let after <- Db.get Note id
  Task.succeed ({ value: r.value, note: Maybe.map (\n -> n.title) after } == { value: Err "not found", note: Just (Text.NonEmpty "Milk") })
}
EOF
alder new test Notes.Index "a missing note cannot be deleted"
alder fill 'Notes.Index."a missing note cannot be deleted"' - <<'EOF'
{
  let ann <- Db.insert User { email: Text.Email "ann@example.com", name: Text.NonEmpty "Ann", passwordHash: "" }
  let r <- Test.run (update (Delete { id: Note.NoteId "missing" }) { notes: [] } { user: ann })
  Task.succeed (r.value == Err "not found")
}
EOF
alder new test Notes.Index "the owner can delete a note"
alder fill 'Notes.Index."the owner can delete a note"' - <<'EOF'
{
  let ann <- Db.insert User { email: Text.Email "ann@example.com", name: Text.NonEmpty "Ann", passwordHash: "" }
  let id <- Db.insert Note { title: Text.NonEmpty "Milk", body: "", owner: ann }
  let r <- Test.run (update (Delete { id: id }) { notes: [] } { user: ann })
  let after <- Db.get Note id
  Task.succeed ({ value: r.value, note: Maybe.map (\n -> n.title) after } == { value: Ok (Redirect (Notes.Index {})), note: Nothing })
}
EOF
alder new test Notes.Index "adding a note through the page"
alder fill 'Notes.Index."adding a note through the page"' - <<'EOF'
{
  let ann <- Db.insert User { email: Text.Email "ann@example.com", name: Text.NonEmpty "Ann", passwordHash: "" }
  let _ <- Test.login { user: Just ann }
  let _ <- Test.get (Notes.Index {})
  let added <- Test.submit NewNote { title: Text.NonEmpty "Milk", body: "" }
  let page <- Test.get (Notes.Index {})
  Task.succeed ({ answer: added.answer, titles: Test.texts "a.title" page.doc } == { answer: Redirected (Notes.Index {}), titles: ["Milk"] })
}
EOF

# tests of the note page
alder new test Notes.Show "a visitor is sent to log in from a note"
alder fill 'Notes.Show."a visitor is sent to log in from a note"' - <<'EOF'
{
  let ann <- Db.insert User { email: Text.Email "ann@example.com", name: Text.NonEmpty "Ann", passwordHash: "" }
  let id <- Db.insert Note { title: Text.NonEmpty "Milk", body: "", owner: ann }
  let visitor <- Test.open (Notes.Show { id: id })
  Task.succeed (visitor.answer == Redirected (Auth.Login {}))
}
EOF
alder new test Notes.Show "a stranger gets not found on a note"
alder fill 'Notes.Show."a stranger gets not found on a note"' - <<'EOF'
{
  let ann <- Db.insert User { email: Text.Email "ann@example.com", name: Text.NonEmpty "Ann", passwordHash: "" }
  let bob <- Db.insert User { email: Text.Email "bob@example.com", name: Text.NonEmpty "Bob", passwordHash: "" }
  let id <- Db.insert Note { title: Text.NonEmpty "Milk", body: "", owner: ann }
  let _ <- Test.login { user: Just bob }
  let r <- Test.open (Notes.Show { id: id })
  Task.succeed (r.answer == PageMissing)
}
EOF
alder new test Notes.Show "the owner sees the note"
alder fill 'Notes.Show."the owner sees the note"' - <<'EOF'
{
  let ann <- Db.insert User { email: Text.Email "ann@example.com", name: Text.NonEmpty "Ann", passwordHash: "" }
  let id <- Db.insert Note { title: Text.NonEmpty "Milk", body: "bread", owner: ann }
  let _ <- Test.login { user: Just ann }
  let page <- Test.get (Notes.Show { id: id })
  Task.succeed ({ title: Test.text "#title" page.doc, body: Test.text "#body" page.doc } == { title: Just "Milk", body: Just "bread" })
}
EOF

alder test
```
