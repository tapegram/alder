### Round 0

```text
$ alder new table User email:Text.Email@unique name:Text.NonEmpty passwordHash:Text created:Time@created --no-timestamps
wrote app/User.alder
wrote migrations/0001_initial.sql
  create table User
wrote migrations/schema.json
[exit 0]
$ alder new auth --users User
wrote adapters/auth/login_password.go
wrote adapters/auth/login_password_test.go
edited app/App.alder
wrote app/Auth/Login.alder
--- a/app/App.alder
+++ b/app/App.alder
@@ -1,9 +1,22 @@
 module App
 
 import Layout
+import User
 
-session { }
+session { user: Maybe User.UserId }
 
+login = Auth.Login
+
+msg Logout
+
+update : Msg -> Session -> Task Nav
+update msg session = match msg {
+  Logout p -> {
+    let _ <- Session.put (\s -> { s | user: Nothing })
+    Task.succeed (Redirect (Auth.Login {}))
+  },
+}
+
 error : Error -> Html Never
 error err =
   <Layout.Page title="Error">
@@ -16,3 +29,10 @@
   <Layout.Page title="Not found">
     <h1>Not found</h1>
   </Layout.Page>
+
+-- Every Log out button is on a page that requires a user, so a browser test cannot tell this
+-- Redirect from a Rerender (book 10, "The logout redirect"): the test calls update itself.
+test "Logout clears the user and goes to log in" = {
+  let r <- Test.run (update Logout { user: Just (User.UserId "me") })
+  Task.succeed ({ value: r.value, trace: r.trace } == { value: Ok (Redirect (Auth.Login {})), trace: ["Session.put"] })
+}
app/Auth/Login.alder:47:33: warning[W0002]: hole ?afterLogin has type Page
          Task.succeed (Redirect (?afterLogin))
                                  ^^^^^^^^^^^
app/Auth/Login.alder:86:85: warning[W0002]: hole ?afterLogin has type Page
    Task.succeed ({ answer: r.answer, user: r.session.user } == { answer: Redirected (?afterLogin), user: Just me })
                                                                                      ^^^^^^^^^^^
[exit 0]
$ alder new table Note title:Text.NonEmpty body:Text owner:User.UserId@ref=User created:Time@created --no-timestamps
wrote app/Note.alder
wrote migrations/0002_create_note.sql
  create table Note
  add index on Note.owner
edited migrations/schema.json
[exit 0]
$ alder new page Notes.Index / --blank --requires user
wrote app/Notes/Index.alder
app/Notes/Index.alder:8:23: warning[W0002]: hole ?load has type Query Notes.Index.Model
  load params session = ?load
                        ^^^^^
app/Notes/Index.alder:11:22: warning[W0002]: hole ?view has type Html Notes.Index.Msg
  view model session = ?view
                       ^^^^^
[exit 0]
$ alder fill --decl Notes.Index.Model - <<'EOF'
{ notes: List Note }
EOF
edited app/Notes/Index.alder
--- a/app/Notes/Index.alder
+++ b/app/Notes/Index.alder
@@ -1,8 +1,10 @@
 module Notes.Index
 
+import Note
+
 page "/" { } requires user
 
-type Model = { }
+type Model = { notes: List Note }
 
 load : Params -> Session -> Query Model
 load params session = ?load
app/Notes/Index.alder:10:23: warning[W0002]: hole ?load has type Query Notes.Index.Model
  load params session = ?load
                        ^^^^^
app/Notes/Index.alder:13:22: warning[W0002]: hole ?view has type Html Notes.Index.Msg
  view model session = ?view
                       ^^^^^
added import Note to app/Notes/Index.alder
[exit 0]
$ alder fill Notes.Index.load - <<'EOF'
\params session -> {
  let notes <- Db.from Note |> Db.where (Note.owner.eq session.user) |> Db.orderBy Note.created.desc |> Db.all
  Query.succeed { notes: notes }
}
EOF
alder fill: the parameters `params session` are bound already: send the body without `\params session ->`
[exit 1]
```

The harness stopped at line 8 of your script: `alder fill Notes.Index.load - <<'EOF'`: it exited 1. The commands after it were not run.

Your script stopped at command 6; the app is as the commands before it left it; continue.

The program type-checks but `alder build` refused it; fix the cause below.

```text
app/Auth/Login.alder:47:33: warning[W0002]: hole ?afterLogin has type Page
          Task.succeed (Redirect (?afterLogin))
                                  ^^^^^^^^^^^
app/Auth/Login.alder:86:85: warning[W0002]: hole ?afterLogin has type Page
    Task.succeed ({ answer: r.answer, user: r.session.user } == { answer: Redirected (?afterLogin), user: Just me })
                                                                                      ^^^^^^^^^^^
app/Notes/Index.alder:10:23: warning[W0002]: hole ?load has type Query Notes.Index.Model
  load params session = ?load
                        ^^^^^
app/Notes/Index.alder:13:22: warning[W0002]: hole ?view has type Html Notes.Index.Msg
  view model session = ?view
                       ^^^^^
alder build: the program has 4 typed hole(s); fill them before building
```
