Continuing from the stopped fill: this round fills `load` (body only) and a placeholder `view`, adds the form and messages, then lists the remaining holes.

```alder-cli
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
alder fill --list
```
