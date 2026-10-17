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
$ alder fill Notes.Index.view - --hole ?view <<'EOF'
<Layout.Page title="Notes">
  <h1>Notes</h1>
</Layout.Page>
EOF
edited app/Notes/Index.alder
--- a/app/Notes/Index.alder
+++ b/app/Notes/Index.alder
@@ -1,5 +1,7 @@
 module Notes.Index
 
+import Layout
+
 page "/" { } requires user
 
 type Model = { }
@@ -8,7 +10,9 @@
 load params session = ?load
 
 view : Model -> Session -> Html Msg
-view model session = ?view
+view model session = <Layout.Page title="Notes">
+  <h1>Notes</h1>
+</Layout.Page>
 
 test "a visitor is sent to log in" = {
   let r <- Test.open (Notes.Index {})
app/Notes/Index.alder:10:23: warning[W0002]: hole ?load has type Query Notes.Index.Model
  load params session = ?load
                        ^^^^^
added import Layout to app/Notes/Index.alder
[exit 0]
$ alder new form Notes.Index NewNote title:Text.NonEmpty body:Text --msg Add
edited app/Notes/Index.alder
--- a/app/Notes/Index.alder
+++ b/app/Notes/Index.alder
@@ -1,6 +1,7 @@
 module Notes.Index
 
 import Layout
+import User
 
 page "/" { } requires user
 
@@ -9,12 +10,50 @@
 load : Params -> Session -> Query Model
 load params session = ?load
 
+form NewNote = {
+  title: Text.NonEmpty,
+  body: Text,
+}
+
+msg Add form NewNote
+
+update : Msg -> Model -> Session -> Task Nav
+update msg model session = match msg {
+  Add p -> ?add,
+}
+
 view : Model -> Session -> Html Msg
 view model session = <Layout.Page title="Notes">
   <h1>Notes</h1>
+
+  <al-card>
+    <h2 slot="header">New note</h2>
+    <form submit={Add}>
+      <al-input field="title" label="Title" />
+      <al-textarea field="body" label="Body" />
+      <al-alert variant="destructive"><Errors form /></al-alert>
+      <al-button type="submit">New note</al-button>
+    </form>
+  </al-card>
 </Layout.Page>
 
 test "a visitor is sent to log in" = {
   let r <- Test.open (Notes.Index {})
   Task.succeed (r.answer == Redirected (Auth.Login {}))
 }
+
+test "submitting NewNote" = {
+  let me <- Db.insert User { email: Text.Email "email-1@example.com", name: Text.NonEmpty "name-1", passwordHash: "passwordhash-1" }
+  let _ <- Test.login { user: Just me }
+  let _ <- Test.get (Notes.Index {})
+  let r <- Test.submit NewNote { title: Text.NonEmpty "title-1", body: "body-1" }
+  Task.succeed (r.answer == ?expected)
+}
+
+test "NewNote refuses an empty title" = {
+  let me <- Db.insert User { email: Text.Email "email-1@example.com", name: Text.NonEmpty "name-1", passwordHash: "passwordhash-1" }
+  let _ <- Test.login { user: Just me }
+  let _ <- Test.get (Notes.Index {})
+  let r <- Test.submitRaw NewNote [{ name: "title", value: "" }, { name: "body", value: "x" }]
+  Task.succeed (r.answer == FormRejected [{ field: "title", message: "must not be empty" }])
+}
app/Notes/Index.alder:11:23: warning[W0002]: hole ?load has type Query Notes.Index.Model
  load params session = ?load
                        ^^^^^
app/Notes/Index.alder:22:12: warning[W0002]: hole ?add has type Task Nav
    Add p -> ?add,
             ^^^^
app/Notes/Index.alder:50:29: warning[W0002]: hole ?expected has type Answer
    Task.succeed (r.answer == ?expected)
                              ^^^^^^^^^
[exit 0]
$ alder new msg Notes.Index Delete id:Note.NoteId
edited app/Notes/Index.alder
--- a/app/Notes/Index.alder
+++ b/app/Notes/Index.alder
@@ -1,6 +1,7 @@
 module Notes.Index
 
 import Layout
+import Note
 import User
 
 page "/" { } requires user
@@ -17,9 +18,12 @@
 
 msg Add form NewNote
 
+msg Delete { id: Note.NoteId }
+
 update : Msg -> Model -> Session -> Task Nav
 update msg model session = match msg {
   Add p -> ?add,
+  Delete p -> ?delete,
 }
 
 view : Model -> Session -> Html Msg
@@ -35,6 +39,8 @@
       <al-button type="submit">New note</al-button>
     </form>
   </al-card>
+
+  <al-button click={Delete { id: ?deleteId }}>Delete</al-button>
 </Layout.Page>
 
 test "a visitor is sent to log in" = {
@@ -57,3 +63,16 @@
   let r <- Test.submitRaw NewNote [{ name: "title", value: "" }, { name: "body", value: "x" }]
   Task.succeed (r.answer == FormRejected [{ field: "title", message: "must not be empty" }])
 }
+
+-- Fill ?expected with what pressing Delete must do, so that a test fails when the arm
+-- does something else: its answer (`r.answer`: Rerendered, Redirected …) and what it
+-- changed (`after`, the row read again), as `{ answer: r.answer, … } == { answer: Rerendered, … }`.
+test "pressing Delete" = {
+  let me <- Db.insert User { email: Text.Email "email-1@example.com", name: Text.NonEmpty "name-1", passwordHash: "passwordhash-1" }
+  let _ <- Test.login { user: Just me }
+  let note1 <- Db.insert Note { title: Text.NonEmpty "title-1", body: "body-1", owner: me }
+  let _ <- Test.get (Notes.Index {})
+  let r <- Test.click (Delete { id: note1 })
+  let after <- Db.get Note note1
+  Task.succeed ?expected
+}
app/Notes/Index.alder:12:23: warning[W0002]: hole ?load has type Query Notes.Index.Model
  load params session = ?load
                        ^^^^^
app/Notes/Index.alder:25:12: warning[W0002]: hole ?add has type Task Nav
    Add p -> ?add,
             ^^^^
app/Notes/Index.alder:26:15: warning[W0002]: hole ?delete has type Task Nav
    Delete p -> ?delete,
                ^^^^^^^
app/Notes/Index.alder:43:34: warning[W0002]: hole ?deleteId has type Note.NoteId
    <al-button click={Delete { id: ?deleteId }}>Delete</al-button>
                                   ^^^^^^^^^
app/Notes/Index.alder:56:29: warning[W0002]: hole ?expected has type Answer
    Task.succeed (r.answer == ?expected)
                              ^^^^^^^^^
app/Notes/Index.alder:77:16: warning[W0002]: hole ?expected has type Bool
    Task.succeed ?expected
                 ^^^^^^^^^
[exit 0]
$ alder fill --decl Notes.Index.Model - <<'EOF'
{ notes: List Note }
EOF
edited app/Notes/Index.alder
--- a/app/Notes/Index.alder
+++ b/app/Notes/Index.alder
@@ -6,7 +6,7 @@
 
 page "/" { } requires user
 
-type Model = { }
+type Model = { notes: List Note }
 
 load : Params -> Session -> Query Model
 load params session = ?load
app/Notes/Index.alder:12:23: warning[W0002]: hole ?load has type Query Notes.Index.Model
  load params session = ?load
                        ^^^^^
app/Notes/Index.alder:25:12: warning[W0002]: hole ?add has type Task Nav
    Add p -> ?add,
             ^^^^
app/Notes/Index.alder:26:15: warning[W0002]: hole ?delete has type Task Nav
    Delete p -> ?delete,
                ^^^^^^^
app/Notes/Index.alder:43:34: warning[W0002]: hole ?deleteId has type Note.NoteId
    <al-button click={Delete { id: ?deleteId }}>Delete</al-button>
                                   ^^^^^^^^^
app/Notes/Index.alder:56:29: warning[W0002]: hole ?expected has type Answer
    Task.succeed (r.answer == ?expected)
                              ^^^^^^^^^
app/Notes/Index.alder:77:16: warning[W0002]: hole ?expected has type Bool
    Task.succeed ?expected
                 ^^^^^^^^^
[exit 0]
$ alder fill Notes.Index.load - --hole ?load <<'EOF'
{
  let notes <-
    Db.from Note
      |> Db.where (Note.owner.eq session.user)
      |> Db.orderBy Note.created.desc
      |> Db.all
  Query.succeed { notes: notes }
}
EOF
edited app/Notes/Index.alder
--- a/app/Notes/Index.alder
+++ b/app/Notes/Index.alder
@@ -9,7 +9,14 @@
 type Model = { notes: List Note }
 
 load : Params -> Session -> Query Model
-load params session = ?load
+load params session = {
+  let notes <-
+    Db.from Note
+      |> Db.where (Note.owner.eq session.user)
+      |> Db.orderBy Note.created.desc
+      |> Db.all
+  Query.succeed { notes: notes }
+}
 
 form NewNote = {
   title: Text.NonEmpty,
app/Notes/Index.alder:32:12: warning[W0002]: hole ?add has type Task Nav
    Add p -> ?add,
             ^^^^
app/Notes/Index.alder:33:15: warning[W0002]: hole ?delete has type Task Nav
    Delete p -> ?delete,
                ^^^^^^^
app/Notes/Index.alder:50:34: warning[W0002]: hole ?deleteId has type Note.NoteId
    <al-button click={Delete { id: ?deleteId }}>Delete</al-button>
                                   ^^^^^^^^^
app/Notes/Index.alder:63:29: warning[W0002]: hole ?expected has type Answer
    Task.succeed (r.answer == ?expected)
                              ^^^^^^^^^
app/Notes/Index.alder:84:16: warning[W0002]: hole ?expected has type Bool
    Task.succeed ?expected
                 ^^^^^^^^^
[exit 0]
$ alder fill --decl Notes.Index.view - <<'EOF'
<Layout.Page title="Notes">
  <h1>Notes</h1>

  <al-card>
    <form submit={Add}>
      <al-input field="title" label="Title" />
      <al-textarea field="body" label="Body" />
      <al-button type="submit">Add</al-button>
    </form>
  </al-card>

  <ul id="notes">
    {for note in model.notes ->
      <li>
        <a class="title" href={Notes.Show { id: note.id }}>{note.title}</a>
        <al-button variant="ghost" size="sm" click={Delete { id: note.id }}>Delete</al-button>
      </li>}
  </ul>

  <al-button variant="outline" click={App.Logout}>Log out</al-button>
</Layout.Page>
EOF
app/Notes/Index.alder:52:34: error[E0002]: unknown `Notes.Show`
            <a class="title" href={Notes.Show { id: note.id }}>{note.title}</a>
                                   ^^^^^^^^^^
alder fill: the result does not check (the errors above): nothing was written
[exit 1]
```

The harness stopped at line 32 of your script: `alder fill --decl Notes.Index.view - <<'EOF'`: it exited 1. The commands after it were not run.

Your script stopped at command 10; the app is as the commands before it left it; continue.

The program type-checks but `alder build` refused it; fix the cause below.

```text
app/Auth/Login.alder:47:33: warning[W0002]: hole ?afterLogin has type Page
          Task.succeed (Redirect (?afterLogin))
                                  ^^^^^^^^^^^
app/Auth/Login.alder:86:85: warning[W0002]: hole ?afterLogin has type Page
    Task.succeed ({ answer: r.answer, user: r.session.user } == { answer: Redirected (?afterLogin), user: Just me })
                                                                                      ^^^^^^^^^^^
app/Notes/Index.alder:32:12: warning[W0002]: hole ?add has type Task Nav
    Add p -> ?add,
             ^^^^
app/Notes/Index.alder:33:15: warning[W0002]: hole ?delete has type Task Nav
    Delete p -> ?delete,
                ^^^^^^^
app/Notes/Index.alder:50:34: warning[W0002]: hole ?deleteId has type Note.NoteId
    <al-button click={Delete { id: ?deleteId }}>Delete</al-button>
                                   ^^^^^^^^^
app/Notes/Index.alder:63:29: warning[W0002]: hole ?expected has type Answer
    Task.succeed (r.answer == ?expected)
                              ^^^^^^^^^
app/Notes/Index.alder:84:16: warning[W0002]: hole ?expected has type Bool
    Task.succeed ?expected
                 ^^^^^^^^^
alder build: the program has 7 typed hole(s); fill them before building
```
