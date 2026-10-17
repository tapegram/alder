### Round 5

```text
$ alder fill 'Auth.Login."logging in through the form"' - --hole ?afterLogin <<'EOF'
Notes.Index {}
EOF
edited app/Auth/Login.alder
--- a/app/Auth/Login.alder
+++ b/app/Auth/Login.alder
@@ -82,7 +82,7 @@
   let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "name-1", passwordHash: "passwordhash-1" }
   let _ <- Test.get (Auth.Login {})
   let r <- Test.submit Credentials { email: Text.Email "me@example.com", password: Text.Secret "secret" }
-  Task.succeed ({ answer: r.answer, user: r.session.user } == { answer: Redirected (?afterLogin), user: Just me })
+  Task.succeed ({ answer: r.answer, user: r.session.user } == { answer: Redirected (Notes.Index {}), user: Just me })
 }
 
 test "a signed-in user logs out here" = {
[exit 0]
$ alder fill 'Notes.Show."pressing Delete"' - --hole ?expected <<'EOF'
{ answer: r.answer, after: after.answer } == { answer: Redirected (Notes.Index {}), after: PageMissing }
EOF
edited app/Notes/Show.alder
--- a/app/Notes/Show.alder
+++ b/app/Notes/Show.alder
@@ -81,11 +81,6 @@
   Task.succeed ({ answer: r.answer, rows: rows } == { answer: FormRejected [{ field: "title", message: "must not be empty" }], rows: 1 })
 }
 
--- Fill ?expected with what pressing Delete must do, so that a test fails when the arm
--- does something else: its answer (`r.answer`: Rerendered, Redirected …) and what it
--- changed (`after`: the page opened again, `after.answer` Shown, or PageMissing when the arm
--- deleted the page's row, and `after.doc` its HTML), as
--- `{ answer: r.answer, … } == { answer: Rerendered, … }`.
 test "pressing Delete" = {
   let me <- Db.insert User { email: Text.Email "email-1@example.com", name: Text.NonEmpty "name-1", passwordHash: "passwordhash-1" }
   let _ <- Test.login { user: Just me }
@@ -93,7 +88,7 @@
   let _ <- Test.get (Notes.Show { id: note1 })
   let r <- Test.click Delete
   let after <- Test.open (Notes.Show { id: note1 })
-  Task.succeed ?expected
+  Task.succeed ({ answer: r.answer, after: after.answer } == { answer: Redirected (Notes.Index {}), after: PageMissing })
 }
 
 test "another user's note is not found" = {
[exit 0]
$ alder fill --decl 'Notes.Show."submitting EditNote adds a row"' - <<'EOF'
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
edited app/Notes/Show.alder
--- a/app/Notes/Show.alder
+++ b/app/Notes/Show.alder
@@ -66,9 +66,10 @@
   let _ <- Test.login { user: Just me }
   let note1 <- Db.insert Note { title: Text.NonEmpty "title-1", body: "body-1", owner: me }
   let _ <- Test.get (Notes.Show { id: note1 })
-  let r <- Test.submit EditNote { title: Text.NonEmpty "title-1", body: "body-1" }
+  let r <- Test.submit EditNote { title: Text.NonEmpty "title-2", body: "body-2" }
+  let after <- Db.get Note note1
   let rows <- Db.from Note |> Db.count
-  Task.succeed ({ answer: r.answer, rows: rows } == { answer: Rerendered, rows: 2 })
+  Task.succeed ({ answer: r.answer, rows: rows, note: Maybe.map (\n -> { title: n.title, body: n.body }) after } == { answer: Redirected (Notes.Show { id: note1 }), rows: 1, note: Just { title: Text.NonEmpty "title-2", body: "body-2" } })
 }
 
 test "EditNote refuses an empty title" = {
rewriting a test: `alder test --mutate` is the check that it still tests something
[exit 0]
$ alder check
[exit 0]
$ alder test
ok   Theme "flip swaps the theme"
ok   App "Logout clears the user and goes to log in"
ok   App "pressing SwitchTheme"
ok   Auth.Login "an unknown email is rejected"
ok   Auth.Login "logging in through the form"
ok   Auth.Login "a signed-in user logs out here"
ok   Note "a Note keeps its owner from being deleted"
ok   Notes.Index "a visitor is sent to log in"
ok   Notes.Index "submitting NewNote adds a row"
ok   Notes.Index "NewNote refuses an empty title"
ok   Notes.Show "a visitor is sent to log in"
ok   Notes.Show "submitting EditNote adds a row"
ok   Notes.Show "EditNote refuses an empty title"
ok   Notes.Show "pressing Delete"
ok   Notes.Show "another user's note is not found"
15 passed, 0 failed
[exit 0]
```

The harness ran `alder check`, `alder build` and `alder test` on the app: all pass.
