```alder-cli
# the login's test: a login goes to the notes list
alder fill 'Auth.Login."logging in through the form"' - --hole ?afterLogin <<'EOF'
Notes.Index {}
EOF

# the notes list: the add form (its errors shown by the field and the form), the notes, Log out
alder fill --decl Notes.Index.view - <<'EOF'
<Layout.Page title="Notes">
  <h1>Notes</h1>

  <al-card>
    <form submit={Add}>
      <al-input field="title" label="Title" />
      <al-textarea field="body" label="Body" />
      <Errors form />
      <al-button type="submit">New note</al-button>
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

# the note page: only the signed-in user's note is loaded; anyone else's is not found
alder fill --decl Notes.Show.load - <<'EOF'
\params session -> {
  let found <- Db.get Note params.id
  let mine = Maybe.andThen (\n -> if n.owner == session.user then Just n else Nothing) found
  match mine {
    Just row -> Query.succeed { row: row },
    Nothing -> Query.notFound,
  }
}
EOF

# the note page's view: the title and the body with their ids
alder fill --decl Notes.Show.view - <<'EOF'
<Layout.Page title={model.row.title}>
  <h1 id="title">{model.row.title}</h1>
  <p id="body">{model.row.body}</p>
  <a href={Notes.Index {}}>Back to notes</a>
</Layout.Page>
EOF

# the holes left in the tests of the notes list
alder fill 'Notes.Index."submitting NewNote"' - --hole ?expected <<'EOF'
Redirected (Notes.Index {})
EOF

alder fill 'Notes.Index."pressing Delete"' - --hole ?expected <<'EOF'
({ answer: r.answer, note: Maybe.map (\n -> n.title) after } == { answer: Redirected (Notes.Index {}), note: Nothing })
EOF

alder test
```
