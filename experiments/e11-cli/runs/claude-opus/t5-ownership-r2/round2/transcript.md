### Round 2

```text
$ alder fill 'Notes.Index."pressing Delete"' --hole ?expected - <<'EOF'
{ answer: r.answer, gone: after == Nothing } == { answer: Redirected (Notes.Index {}), gone: True }
EOF
edited app/Notes/Index.alder
--- a/app/Notes/Index.alder
+++ b/app/Notes/Index.alder
@@ -80,9 +80,6 @@
   Task.succeed ({ answer: r.answer, rows: rows } == { answer: FormRejected [{ field: "title", message: "must not be empty" }], rows: 0 })
 }
 
--- Fill ?expected with what pressing Delete must do, so that a test fails when the arm
--- does something else: its answer (`r.answer`: Rerendered, Redirected …) and what it
--- changed (`after`, the row read again), as `{ answer: r.answer, … } == { answer: Rerendered, … }`.
 test "pressing Delete" = {
   let me <- Db.insert User { email: Text.Email "email-1@example.com", name: Text.NonEmpty "name-1", passwordHash: "passwordhash-1" }
   let _ <- Test.login { user: Just me }
@@ -90,7 +87,7 @@
   let _ <- Test.get (Notes.Index {})
   let r <- Test.click (Delete { id: note1 })
   let after <- Db.get Note note1
-  Task.succeed ?expected
+  Task.succeed ({ answer: r.answer, gone: after == Nothing } == { answer: Redirected (Notes.Index {}), gone: True })
 }
 
 test "deleting another user's note is not found" = {
[exit 0]
$ alder new test Notes.Index "deleting a missing note is not found"
edited app/Notes/Index.alder
--- a/app/Notes/Index.alder
+++ b/app/Notes/Index.alder
@@ -99,3 +99,7 @@
   let after <- Db.get Note id
   Task.succeed ({ value: r.value, kept: Maybe.isJust after } == { value: Err "not found", kept: True })
 }
+
+test "deleting a missing note is not found" = {
+  ?test
+}
app/Notes/Index.alder:104:3: warning[W0002]: hole ?test has type Task Bool
    ?test
    ^^^^^
[exit 0]
$ alder fill 'Notes.Index."deleting a missing note is not found"' - <<'EOF'
{
  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
  let _ <- Test.login { user: Just me }
  let r <- Test.run (update (Delete { id: Note.NoteId "missing" }) { notes: [] } { user: me })
  Task.succeed (r.value == Err "not found")
}
EOF
edited app/Notes/Index.alder
--- a/app/Notes/Index.alder
+++ b/app/Notes/Index.alder
@@ -101,5 +101,8 @@
 }
 
 test "deleting a missing note is not found" = {
-  ?test
+  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
+  let _ <- Test.login { user: Just me }
+  let r <- Test.run (update (Delete { id: Note.NoteId "missing" }) { notes: [] } { user: me })
+  Task.succeed (r.value == Err "not found")
 }
[exit 0]
$ alder new test Notes.Show "another user's note is not found"
edited app/Notes/Show.alder
--- a/app/Notes/Show.alder
+++ b/app/Notes/Show.alder
@@ -27,3 +27,7 @@
   let r <- Test.open (Notes.Show { id: note1 })
   Task.succeed (r.answer == Redirected (Auth.Login {}))
 }
+
+test "another user's note is not found" = {
+  ?test
+}
app/Notes/Show.alder:32:3: warning[W0002]: hole ?test has type Task Bool
    ?test
    ^^^^^
[exit 0]
$ alder fill 'Notes.Show."another user'"'"'s note is not found"' - <<'EOF'
{
  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
  let other <- Db.insert User { email: Text.Email "other@example.com", name: Text.NonEmpty "Other", passwordHash: "" }
  let id <- Db.insert Note { title: Text.NonEmpty "Theirs", body: "b", owner: other }
  let _ <- Test.login { user: Just me }
  let r <- Test.open (Notes.Show { id: id })
  Task.succeed (r.answer == PageMissing)
}
EOF
edited app/Notes/Show.alder
--- a/app/Notes/Show.alder
+++ b/app/Notes/Show.alder
@@ -29,5 +29,10 @@
 }
 
 test "another user's note is not found" = {
-  ?test
+  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
+  let other <- Db.insert User { email: Text.Email "other@example.com", name: Text.NonEmpty "Other", passwordHash: "" }
+  let id <- Db.insert Note { title: Text.NonEmpty "Theirs", body: "b", owner: other }
+  let _ <- Test.login { user: Just me }
+  let r <- Test.open (Notes.Show { id: id })
+  Task.succeed (r.answer == PageMissing)
 }
[exit 0]
$ alder fill --list
Auth.Login update.Submit ?afterLogin : Page
Auth.Login "logging in through the form" ?afterLogin : Page
[exit 0]
$ alder fill Auth.Login.update.Submit --hole ?afterLogin - <<'EOF'
Notes.Index {}
EOF
edited app/Auth/Login.alder
--- a/app/Auth/Login.alder
+++ b/app/Auth/Login.alder
@@ -44,7 +44,7 @@
     match found {
       Just id -> {
         let _ <- Session.put (\s -> { s | user: Just id })
-        Task.succeed (Redirect (?afterLogin))
+        Task.succeed (Redirect (Notes.Index {}))
       },
       Nothing -> Task.succeed (Reject "Email or password is incorrect."),
     }
app/Auth/Login.alder:86:85: warning[W0002]: hole ?afterLogin has type Page
    Task.succeed ({ answer: r.answer, user: r.session.user } == { answer: Redirected (?afterLogin), user: Just me })
                                                                                      ^^^^^^^^^^^
[exit 0]
$ alder fill --list
Auth.Login "logging in through the form" ?afterLogin : Page
[exit 0]
$ alder fill 'Auth.Login."logging in through the form"' --hole ?afterLogin - <<'EOF'
Notes.Index {}
EOF
edited app/Auth/Login.alder
--- a/app/Auth/Login.alder
+++ b/app/Auth/Login.alder
@@ -83,7 +83,7 @@
   let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "name-1", passwordHash: "passwordhash-1" }
   let _ <- Test.get (Auth.Login {})
   let r <- Test.submit Credentials { email: Text.Email "me@example.com", password: Text.Secret "secret" }
-  Task.succeed ({ answer: r.answer, user: r.session.user } == { answer: Redirected (?afterLogin), user: Just me })
+  Task.succeed ({ answer: r.answer, user: r.session.user } == { answer: Redirected (Notes.Index {}), user: Just me })
 }
 
 test "a signed-in user logs out here" = {
[exit 0]
$ alder test
ok   App "Logout clears the user and goes to log in"
ok   Auth.Login "an unknown email is rejected"
ok   Auth.Login "logging in through the form"
ok   Auth.Login "a signed-in user logs out here"
ok   Note "a Note keeps its owner from being deleted"
ok   Notes.Index "a visitor is sent to log in"
ok   Notes.Index "submitting NewNote adds a row"
ok   Notes.Index "NewNote refuses an empty title"
ok   Notes.Index "pressing Delete"
ok   Notes.Index "deleting another user's note is not found"
ok   Notes.Index "deleting a missing note is not found"
ok   Notes.Show "a visitor is sent to log in"
ok   Notes.Show "another user's note is not found"
13 passed, 0 failed
[exit 0]
```

The harness ran `alder check`, `alder build` and `alder test` on the app: all pass.
