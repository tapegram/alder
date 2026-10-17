```alder-cli
alder fill 'Auth.Login."logging in through the form"' - --hole ?afterLogin <<'EOF'
Notes.Index {}
EOF
alder fill 'Notes.Show."pressing Delete"' - --hole ?expected <<'EOF'
{ answer: r.answer, after: after.answer } == { answer: Redirected (Notes.Index {}), after: PageMissing }
EOF
alder fill --decl 'Notes.Show."submitting EditNote adds a row"' - <<'EOF'
{
  let me <- Db.insert User { email: Text.Email "email-1@example.com", name: Text.NonEmpty "name-1", passwordHash: "passwordhash-1" }
  let _ <- Test.login { user: Just me }
  let note1 <- Db.insert Note { title: Text.NonEmpty "title-1", body: "body-1", owner: me }
  let _ <- Test.get (Notes.Show { id: note1 })
  let r <- Test.submit EditNote { title: Text.NonEmpty "title-2", body: "body-2" }
  let after <- Db.get Note note1
  let rows <- Db.from Note |> Db.count
  Task.succeed ({ answer: r.answer, rows: rows, note: Maybe.map (\n -> { title: n.title, body: n.body }) after } == { answer: Redirected (Notes.Show { id: note1 }), rows: 1, note: Just { title: Text.NonEmpty "title-2", body: "body-2" } })
}
EOF
alder check
alder test
```
