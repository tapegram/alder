Stopping after the generators and a hole listing, since I can't confirm the hole names or what `new msg` leaves unfilled. I'll fill them next round.

```alder-cli
alder new table User email:Text.Email@unique name:Text.NonEmpty passwordHash:Text created:Time@created --no-timestamps
alder new auth --users User
alder new table Note title:Text.NonEmpty body:Text owner:User.UserId@ref=User created:Time@created --no-timestamps
alder new page Notes.Index / --blank --requires user
alder fill --decl Notes.Index.Model - <<'EOF'
{ notes: List Note }
EOF
alder fill Notes.Index.load - <<'EOF'
\params session -> {
  let notes <- Db.from Note |> Db.where (Note.owner.eq session.user) |> Db.orderBy Note.created.desc |> Db.all
  Query.succeed { notes: notes }
}
EOF
alder fill Notes.Index.view - <<'EOF'
\model session ->
  <Layout.Page title="Notes">
    <h1>Notes</h1>
    <ul id="notes">
      {for n in model.notes -> <li><a class="title" href={Notes.Show { id: n.id }}>{n.title}</a></li>}
    </ul>
  </Layout.Page>
EOF
alder new form Notes.Index NewNote title:Text.NonEmpty body:Text
alder new msg Notes.Index Delete id:Note.NoteId
alder new page Notes.Show "/notes/{id}" --blank --requires user --param id:Note.NoteId
alder fill --list
```
