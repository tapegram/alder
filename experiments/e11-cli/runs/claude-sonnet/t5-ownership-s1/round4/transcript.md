### Round 4

```text
$ alder fill --list
Auth.Login update.Submit ?afterLogin : Page
Auth.Login "logging in through the form" ?afterLogin : Page
Notes.Index "submitting NewNote" ?expected : Answer
Notes.Index "pressing Delete" ?expected : Bool
[exit 0]
$ alder fill Auth.Login.update - --hole ?afterLogin <<'EOF'
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
$ alder fill --decl 'Notes.Index."submitting NewNote"' - <<'EOF'
{
  let me <- Db.insert User { email: Text.Email "email-1@example.com", name: Text.NonEmpty "name-1", passwordHash: "passwordhash-1" }
  let _ <- Test.login { user: Just me }
  let _ <- Test.get (Notes.Index {})
  let r <- Test.submit NewNote { title: Text.NonEmpty "title-1", body: "body-1" }
  Task.succeed (r.answer == Redirected (Notes.Index {}))
}
EOF
edited app/Notes/Index.alder
--- a/app/Notes/Index.alder
+++ b/app/Notes/Index.alder
@@ -67,7 +67,7 @@
   let _ <- Test.login { user: Just me }
   let _ <- Test.get (Notes.Index {})
   let r <- Test.submit NewNote { title: Text.NonEmpty "title-1", body: "body-1" }
-  Task.succeed (r.answer == ?expected)
+  Task.succeed (r.answer == Redirected (Notes.Index {}))
 }
 
 test "NewNote refuses an empty title" = {
app/Notes/Index.alder:91:16: warning[W0002]: hole ?expected has type Bool
    Task.succeed ?expected
                 ^^^^^^^^^
rewriting a test: `alder test --mutate` is the check that it still tests something
[exit 0]
$ alder fill --decl 'Notes.Index."pressing Delete"' - <<'EOF'
{
  let me <- Db.insert User { email: Text.Email "email-1@example.com", name: Text.NonEmpty "name-1", passwordHash: "passwordhash-1" }
  let _ <- Test.login { user: Just me }
  let note1 <- Db.insert Note { title: Text.NonEmpty "title-1", body: "body-1", owner: me }
  let _ <- Test.get (Notes.Index {})
  let r <- Test.click (Delete { id: note1 })
  let after <- Db.get Note note1
  Task.succeed ({ answer: r.answer, gone: after == Nothing } == { answer: Redirected (Notes.Index {}), gone: True })
}
EOF
edited app/Notes/Index.alder
--- a/app/Notes/Index.alder
+++ b/app/Notes/Index.alder
@@ -78,9 +78,6 @@
   Task.succeed (r.answer == FormRejected [{ field: "title", message: "must not be empty" }])
 }
 
--- Fill ?expected with what pressing Delete must do, so that a test fails when the arm
--- does something else: its answer (`r.answer`: Rerendered, Redirected …) and what it
--- changed (`after`, the row read again), as `{ answer: r.answer, … } == { answer: Rerendered, … }`.
 test "pressing Delete" = {
   let me <- Db.insert User { email: Text.Email "email-1@example.com", name: Text.NonEmpty "name-1", passwordHash: "passwordhash-1" }
   let _ <- Test.login { user: Just me }
@@ -88,5 +85,5 @@
   let _ <- Test.get (Notes.Index {})
   let r <- Test.click (Delete { id: note1 })
   let after <- Db.get Note note1
-  Task.succeed ?expected
+  Task.succeed ({ answer: r.answer, gone: after == Nothing } == { answer: Redirected (Notes.Index {}), gone: True })
 }
rewriting a test: `alder test --mutate` is the check that it still tests something
[exit 0]
$ alder check
app/Auth/Login.alder:86:85: warning[W0002]: hole ?afterLogin has type Page
    Task.succeed ({ answer: r.answer, user: r.session.user } == { answer: Redirected (?afterLogin), user: Just me })
                                                                                      ^^^^^^^^^^^
[exit 0]
```

The program type-checks but `alder build` refused it; fix the cause below.

```text
app/Auth/Login.alder:86:85: warning[W0002]: hole ?afterLogin has type Page
    Task.succeed ({ answer: r.answer, user: r.session.user } == { answer: Redirected (?afterLogin), user: Just me })
                                                                                      ^^^^^^^^^^^
alder build: the program has 1 typed hole(s); fill them before building
```
