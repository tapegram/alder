### Round 5

```text
$ alder fill --decl 'Auth.Login."logging in through the form"' - <<'EOF'
{
  let _ <- Test.fake Password
  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
  let _ <- Test.get (Auth.Login {})
  let r <- Test.submit Credentials { email: Text.Email "me@example.com", password: Text.Secret "secret" }
  Task.succeed ({ answer: r.answer, user: r.session.user } == { answer: Redirected (Notes.Index {}), user: Just me })
}
EOF
edited app/Auth/Login.alder
--- a/app/Auth/Login.alder
+++ b/app/Auth/Login.alder
@@ -80,10 +80,10 @@
 
 test "logging in through the form" = {
   let _ <- Test.fake Password
-  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "name-1", passwordHash: "passwordhash-1" }
+  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
   let _ <- Test.get (Auth.Login {})
   let r <- Test.submit Credentials { email: Text.Email "me@example.com", password: Text.Secret "secret" }
-  Task.succeed ({ answer: r.answer, user: r.session.user } == { answer: Redirected (?afterLogin), user: Just me })
+  Task.succeed ({ answer: r.answer, user: r.session.user } == { answer: Redirected (Notes.Index {}), user: Just me })
 }
 
 test "a signed-in user logs out here" = {
rewriting a test: `alder test --mutate` is the check that it still tests something
[exit 0]
$ alder new test Notes.Index "deleting another user's note is not found"
edited app/Notes/Index.alder
--- a/app/Notes/Index.alder
+++ b/app/Notes/Index.alder
@@ -87,3 +87,7 @@
   let after <- Db.get Note note1
   Task.succeed ({ answer: r.answer, gone: after == Nothing } == { answer: Redirected (Notes.Index {}), gone: True })
 }
+
+test "deleting another user's note is not found" = {
+  ?test
+}
app/Notes/Index.alder:92:3: warning[W0002]: hole ?test has type Task Bool
    ?test
    ^^^^^
[exit 0]
$ alder fill --decl 'Notes.Index."deleting another user'"'"'s note is not found"' - <<'EOF'
{
  let owner <- Db.insert User { email: Text.Email "owner@example.com", name: Text.NonEmpty "Owner", passwordHash: "" }
  let other <- Db.insert User { email: Text.Email "other@example.com", name: Text.NonEmpty "Other", passwordHash: "" }
  let id <- Db.insert Note { title: Text.NonEmpty "T", body: "b", owner: owner }
  let _ <- Test.login { user: Just other }
  let r <- Test.run (update (Delete { id: id }) { notes: [] } { user: other })
  let after <- Db.get Note id
  Task.succeed ({ value: r.value, kept: after != Nothing } == { value: Err "not found", kept: True })
}
EOF
edited app/Notes/Index.alder
--- a/app/Notes/Index.alder
+++ b/app/Notes/Index.alder
@@ -89,5 +89,11 @@
 }
 
 test "deleting another user's note is not found" = {
-  ?test
+  let owner <- Db.insert User { email: Text.Email "owner@example.com", name: Text.NonEmpty "Owner", passwordHash: "" }
+  let other <- Db.insert User { email: Text.Email "other@example.com", name: Text.NonEmpty "Other", passwordHash: "" }
+  let id <- Db.insert Note { title: Text.NonEmpty "T", body: "b", owner: owner }
+  let _ <- Test.login { user: Just other }
+  let r <- Test.run (update (Delete { id: id }) { notes: [] } { user: other })
+  let after <- Db.get Note id
+  Task.succeed ({ value: r.value, kept: after != Nothing } == { value: Err "not found", kept: True })
 }
rewriting a test: `alder test --mutate` is the check that it still tests something
[exit 0]
$ alder new test Notes.Index "deleting a missing note is not found"
edited app/Notes/Index.alder
--- a/app/Notes/Index.alder
+++ b/app/Notes/Index.alder
@@ -97,3 +97,7 @@
   let after <- Db.get Note id
   Task.succeed ({ value: r.value, kept: after != Nothing } == { value: Err "not found", kept: True })
 }
+
+test "deleting a missing note is not found" = {
+  ?test
+}
app/Notes/Index.alder:102:3: warning[W0002]: hole ?test has type Task Bool
    ?test
    ^^^^^
[exit 0]
$ alder fill --decl 'Notes.Index."deleting a missing note is not found"' - <<'EOF'
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
@@ -99,5 +99,8 @@
 }
 
 test "deleting a missing note is not found" = {
-  ?test
+  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
+  let _ <- Test.login { user: Just me }
+  let r <- Test.run (update (Delete { id: Note.NoteId "missing" }) { notes: [] } { user: me })
+  Task.succeed (r.value == Err "not found")
 }
rewriting a test: `alder test --mutate` is the check that it still tests something
[exit 0]
$ alder check
[exit 0]
$ alder test
ok   App "Logout clears the user and goes to log in"
ok   Auth.Login "an unknown email is rejected"
ok   Auth.Login "logging in through the form"
ok   Auth.Login "a signed-in user logs out here"
ok   Note "a Note keeps its owner from being deleted"
ok   Notes.Index "a visitor is sent to log in"
ok   Notes.Index "submitting NewNote"
FAIL Notes.Index "NewNote refuses an empty title"
  left:  FormRejected [{ field: "", message: "title must not be empty" }]
  right: FormRejected [{ field: "title", message: "must not be empty" }]
  trace: ["Db.insert User", "Test.login", "Test.get Notes.Index", "Db.all Note", "Test.submitRaw NewNote", "Db.all Note"]
ok   Notes.Index "pressing Delete"
ok   Notes.Index "deleting another user's note is not found"
ok   Notes.Index "deleting a missing note is not found"
ok   Notes.Show "a visitor is sent to log in"
11 passed, 1 failed
[exit 1]
```

The harness stopped at line 32 of your script: `alder test`: it exited 1. The commands after it were not run.

Your script stopped at command 7; the app is as the commands before it left it; continue.

The program builds but its tests fail; fix the program or the tests so every test passes.

```text
ok   App "Logout clears the user and goes to log in"
ok   Auth.Login "an unknown email is rejected"
ok   Auth.Login "logging in through the form"
ok   Auth.Login "a signed-in user logs out here"
ok   Note "a Note keeps its owner from being deleted"
ok   Notes.Index "a visitor is sent to log in"
ok   Notes.Index "submitting NewNote"
FAIL Notes.Index "NewNote refuses an empty title"
  left:  FormRejected [{ field: "", message: "title must not be empty" }]
  right: FormRejected [{ field: "title", message: "must not be empty" }]
  trace: ["Db.insert User", "Test.login", "Test.get Notes.Index", "Db.all Note", "Test.submitRaw NewNote", "Db.all Note"]
ok   Notes.Index "pressing Delete"
ok   Notes.Index "deleting another user's note is not found"
ok   Notes.Index "deleting a missing note is not found"
ok   Notes.Show "a visitor is sent to log in"
11 passed, 1 failed
```
