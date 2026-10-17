### Round 5

```text
$ alder fill 'Notes.Show."the page shows the row"' - <<'EOF'
{
  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
  let id <- Db.insert Note { title: Text.NonEmpty "Milk", body: "x", owner: me }
  let _ <- Test.login { user: Just me }
  let page <- Test.get (Notes.Show { id: id })
  Task.succeed ({ title: Test.text "#title" page.doc, body: Test.text "#body" page.doc } == { title: Just "Milk", body: Just "x" })
}
EOF
alder fill: `Notes.Show."the page shows the row"` has no typed hole; to rewrite it whole, `alder fill --decl 'Notes.Show."the page shows the row"' -` (`alder fill --list` lists the app's holes)
[exit 1]
```

The harness stopped at line 2 of your script: `alder fill 'Notes.Show."the page shows the row"' - <<'EOF'`: it exited 1. The commands after it were not run.

Your script stopped at command 1; the app is as the commands before it left it; continue.

The program builds but its tests fail; fix the program or the tests so every test passes.

```text
ok   App "Logout clears the user and goes to log in"
ok   Auth.Login "an unknown email is rejected"
ok   Auth.Login "logging in through the form"
ok   Auth.Login "a signed-in user logs out here"
ok   Note "a Note keeps its owner from being deleted"
ok   Notes.Index "a visitor is sent to log in"
ok   Notes.Index "submitting NewNote"
ok   Notes.Index "NewNote refuses an empty title"
ok   Notes.Index "pressing Delete"
ok   Notes.Index "deleting another users note is not found"
ok   Notes.Index "deleting a missing note is not found"
ok   Notes.Index "the owner can delete a note"
FAIL Notes.Show "the page shows the row"
  left:  { answer: Shown, title: False }
  right: { answer: Shown, title: True }
  trace: ["Db.insert User", "Test.login", "Db.insert Note", "Test.open Notes.Show", "Db.get Note"]
ok   Notes.Show "a missing row is not found"
ok   Notes.Show "a visitor is sent to log in"
14 passed, 1 failed
```
