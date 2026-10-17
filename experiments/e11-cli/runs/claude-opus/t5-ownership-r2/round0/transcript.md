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
$ alder new table Note title:Text.NonEmpty body:Text owner:User.UserId@ref=User@owner created:Time@created --no-timestamps
wrote app/Note.alder
wrote migrations/0002_create_note.sql
  create table Note
  add index on Note.owner
edited migrations/schema.json
[exit 0]
$ alder new page Notes.Show '/notes/{id}' --param id:Note.NoteId --requires user --blank
wrote app/Notes/Show.alder
app/Notes/Show.alder:11:23: warning[W0002]: hole ?load has type Query Notes.Show.Model
  load params session = ?load
                        ^^^^^
app/Notes/Show.alder:14:22: warning[W0002]: hole ?view has type Html Notes.Show.Msg
  view model session = ?view
                       ^^^^^
[exit 0]
$ alder fill --decl Notes.Show.Model - <<'EOF'
{ note: Note }
EOF
edited app/Notes/Show.alder
--- a/app/Notes/Show.alder
+++ b/app/Notes/Show.alder
@@ -5,7 +5,7 @@
 
 page "/notes/{id}" { id: Note.NoteId } requires user
 
-type Model = { }
+type Model = { note: Note }
 
 load : Params -> Session -> Query Model
 load params session = ?load
app/Notes/Show.alder:11:23: warning[W0002]: hole ?load has type Query Notes.Show.Model
  load params session = ?load
                        ^^^^^
app/Notes/Show.alder:14:22: warning[W0002]: hole ?view has type Html Notes.Show.Msg
  view model session = ?view
                       ^^^^^
[exit 0]
$ alder fill Notes.Show.load - <<'EOF'
{
  let note <- Db.getMine Note params.id
  Query.succeed { note: note }
}
EOF
edited app/Notes/Show.alder
--- a/app/Notes/Show.alder
+++ b/app/Notes/Show.alder
@@ -8,7 +8,10 @@
 type Model = { note: Note }
 
 load : Params -> Session -> Query Model
-load params session = ?load
+load params session = {
+  let note <- Db.getMine Note params.id
+  Query.succeed { note: note }
+}
 
 view : Model -> Session -> Html Msg
 view model session = ?view
app/Notes/Show.alder:17:22: warning[W0002]: hole ?view has type Html Notes.Show.Msg
  view model session = ?view
                       ^^^^^
[exit 0]
$ alder fill Notes.Show.view - <<'EOF'
<Layout.Page title={model.note.title}>
  <h1 id="title">{model.note.title}</h1>
  <p id="body">{model.note.body}</p>
  <a href={Notes.Index {}}>Back</a>
</Layout.Page>
EOF
app/Notes/Show.alder:21:12: error[E0002]: unknown `Notes.Index`
    <a href={Notes.Index {}}>Back</a>
             ^^^^^^^^^^^
alder fill: the result does not check (the errors above): nothing was written
[exit 1]
```

The harness stopped at line 19 of your script: `alder fill Notes.Show.view - <<'EOF'`: it exited 1. The commands after it were not run.

Your script stopped at command 7; the app is as the commands before it left it; continue.

The program type-checks but `alder build` refused it; fix the cause below.

```text
app/Auth/Login.alder:47:33: warning[W0002]: hole ?afterLogin has type Page
          Task.succeed (Redirect (?afterLogin))
                                  ^^^^^^^^^^^
app/Auth/Login.alder:86:85: warning[W0002]: hole ?afterLogin has type Page
    Task.succeed ({ answer: r.answer, user: r.session.user } == { answer: Redirected (?afterLogin), user: Just me })
                                                                                      ^^^^^^^^^^^
app/Notes/Show.alder:17:22: warning[W0002]: hole ?view has type Html Notes.Show.Msg
  view model session = ?view
                       ^^^^^
alder build: the program has 3 typed hole(s); fill them before building
```
