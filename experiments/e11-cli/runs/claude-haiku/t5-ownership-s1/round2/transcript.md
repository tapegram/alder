### Round 2

```text
$ alder remove page Notes.Index
removed app/Notes/Index.alder
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
$ alder fill --decl Notes.Index.view - <<'EOF'
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
@@ -8,7 +10,10 @@
 load params session = ?load
 
 view : Model -> Session -> Html Msg
-view model session = ?view
+view model session =
+  <Layout.Page title="Notes">
+    <h1>Notes</h1>
+  </Layout.Page>
 
 test "a visitor is sent to log in" = {
   let r <- Test.open (Notes.Index {})
app/Notes/Index.alder:10:23: warning[W0002]: hole ?load has type Query Notes.Index.Model
  load params session = ?load
                        ^^^^^
added import Layout to app/Notes/Index.alder
[exit 0]
$ alder new msg Notes.Index Delete id:Note.NoteId
edited app/Notes/Index.alder
--- a/app/Notes/Index.alder
+++ b/app/Notes/Index.alder
@@ -1,6 +1,8 @@
 module Notes.Index
 
 import Layout
+import Note
+import User
 
 page "/" { } requires user
 
@@ -9,13 +11,35 @@
 load : Params -> Session -> Query Model
 load params session = ?load
 
+msg Delete { id: Note.NoteId }
+
+update : Msg -> Model -> Session -> Task Nav
+update msg model session = match msg {
+  Delete p -> ?delete,
+}
+
 view : Model -> Session -> Html Msg
 view model session =
   <Layout.Page title="Notes">
     <h1>Notes</h1>
+
+    <al-button click={Delete { id: ?deleteId }}>Delete</al-button>
   </Layout.Page>
 
 test "a visitor is sent to log in" = {
   let r <- Test.open (Notes.Index {})
   Task.succeed (r.answer == Redirected (Auth.Login {}))
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
app/Notes/Index.alder:18:15: warning[W0002]: hole ?delete has type Task Nav
    Delete p -> ?delete,
                ^^^^^^^
app/Notes/Index.alder:26:36: warning[W0002]: hole ?deleteId has type Note.NoteId
      <al-button click={Delete { id: ?deleteId }}>Delete</al-button>
                                     ^^^^^^^^^
app/Notes/Index.alder:44:16: warning[W0002]: hole ?expected has type Bool
    Task.succeed ?expected
                 ^^^^^^^^^
[exit 0]
$ alder new form Notes.Index NewNote title:Text.NonEmpty body:Text --msg Add
edited app/Notes/Index.alder
--- a/app/Notes/Index.alder
+++ b/app/Notes/Index.alder
@@ -13,9 +13,20 @@
 
 msg Delete { id: Note.NoteId }
 
+form NewNote = {
+  title: Text.NonEmpty,
+  body: Text,
+}
+
+msg Add form NewNote
+
 update : Msg -> Model -> Session -> Task Nav
 update msg model session = match msg {
   Delete p -> ?delete,
+  Add p -> {
+    let _ <- Db.insertMine Note { title: p.form.title, body: p.form.body }
+    Task.succeed (Redirect (Notes.Index {}))
+  },
 }
 
 view : Model -> Session -> Html Msg
@@ -24,6 +35,16 @@
     <h1>Notes</h1>
 
     <al-button click={Delete { id: ?deleteId }}>Delete</al-button>
+
+    <al-card>
+      <h2 slot="header">New note</h2>
+      <form submit={Add}>
+        <al-input field="title" label="Title" />
+        <al-textarea field="body" label="Body" />
+        <al-alert variant="destructive"><Errors form /></al-alert>
+        <al-button type="submit">New note</al-button>
+      </form>
+    </al-card>
   </Layout.Page>
 
 test "a visitor is sent to log in" = {
@@ -43,3 +64,21 @@
   let after <- Db.get Note note1
   Task.succeed ?expected
 }
+
+test "submitting NewNote adds a row" = {
+  let me <- Db.insert User { email: Text.Email "email-1@example.com", name: Text.NonEmpty "name-1", passwordHash: "passwordhash-1" }
+  let _ <- Test.login { user: Just me }
+  let _ <- Test.get (Notes.Index {})
+  let r <- Test.submit NewNote { title: Text.NonEmpty "title-1", body: "body-1" }
+  let rows <- Db.from Note |> Db.all
+  Task.succeed ({ answer: r.answer, rows: List.map (\row -> { title: row.title, body: row.body }) rows } == { answer: Redirected (Notes.Index {}), rows: [{ title: Text.NonEmpty "title-1", body: "body-1" }] })
+}
+
+test "NewNote refuses an empty title" = {
+  let me <- Db.insert User { email: Text.Email "email-1@example.com", name: Text.NonEmpty "name-1", passwordHash: "passwordhash-1" }
+  let _ <- Test.login { user: Just me }
+  let _ <- Test.get (Notes.Index {})
+  let r <- Test.submitRaw NewNote [{ name: "title", value: "" }, { name: "body", value: "x" }]
+  let rows <- Db.from Note |> Db.count
+  Task.succeed ({ answer: r.answer, rows: rows } == { answer: FormRejected [{ field: "title", message: "must not be empty" }], rows: 0 })
+}
app/Notes/Index.alder:12:23: warning[W0002]: hole ?load has type Query Notes.Index.Model
  load params session = ?load
                        ^^^^^
app/Notes/Index.alder:25:15: warning[W0002]: hole ?delete has type Task Nav
    Delete p -> ?delete,
                ^^^^^^^
app/Notes/Index.alder:37:36: warning[W0002]: hole ?deleteId has type Note.NoteId
      <al-button click={Delete { id: ?deleteId }}>Delete</al-button>
                                     ^^^^^^^^^
app/Notes/Index.alder:65:16: warning[W0002]: hole ?expected has type Bool
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
app/Notes/Index.alder:25:15: warning[W0002]: hole ?delete has type Task Nav
    Delete p -> ?delete,
                ^^^^^^^
app/Notes/Index.alder:37:36: warning[W0002]: hole ?deleteId has type Note.NoteId
      <al-button click={Delete { id: ?deleteId }}>Delete</al-button>
                                     ^^^^^^^^^
app/Notes/Index.alder:65:16: warning[W0002]: hole ?expected has type Bool
    Task.succeed ?expected
                 ^^^^^^^^^
[exit 0]
$ alder fill --decl Notes.Index.load - <<'EOF'
{
  let notes <- Db.mine Note |> Db.orderBy Note.created.desc |> Db.all
  Query.succeed { notes: notes }
}
EOF
edited app/Notes/Index.alder
--- a/app/Notes/Index.alder
+++ b/app/Notes/Index.alder
@@ -9,7 +9,10 @@
 type Model = { notes: List Note }
 
 load : Params -> Session -> Query Model
-load params session = ?load
+load params session = {
+  let notes <- Db.mine Note |> Db.orderBy Note.created.desc |> Db.all
+  Query.succeed { notes: notes }
+}
 
 msg Delete { id: Note.NoteId }
 
app/Notes/Index.alder:28:15: warning[W0002]: hole ?delete has type Task Nav
    Delete p -> ?delete,
                ^^^^^^^
app/Notes/Index.alder:40:36: warning[W0002]: hole ?deleteId has type Note.NoteId
      <al-button click={Delete { id: ?deleteId }}>Delete</al-button>
                                     ^^^^^^^^^
app/Notes/Index.alder:68:16: warning[W0002]: hole ?expected has type Bool
    Task.succeed ?expected
                 ^^^^^^^^^
[exit 0]
$ alder fill --decl Notes.Index.update - <<'EOF'
match msg {
  Add p -> {
    let _ <- Db.insertMine Note { title: p.form.title, body: p.form.body }
    Task.succeed (Redirect (Notes.Index {}))
  },
  Delete p -> {
    let _ <- Db.deleteMine Note p.id
    Task.succeed (Redirect (Notes.Index {}))
  },
}
EOF
edited app/Notes/Index.alder
--- a/app/Notes/Index.alder
+++ b/app/Notes/Index.alder
@@ -25,11 +25,14 @@
 
 update : Msg -> Model -> Session -> Task Nav
 update msg model session = match msg {
-  Delete p -> ?delete,
   Add p -> {
     let _ <- Db.insertMine Note { title: p.form.title, body: p.form.body }
     Task.succeed (Redirect (Notes.Index {}))
   },
+  Delete p -> {
+    let _ <- Db.deleteMine Note p.id
+    Task.succeed (Redirect (Notes.Index {}))
+  },
 }
 
 view : Model -> Session -> Html Msg
app/Notes/Index.alder:43:36: warning[W0002]: hole ?deleteId has type Note.NoteId
      <al-button click={Delete { id: ?deleteId }}>Delete</al-button>
                                     ^^^^^^^^^
app/Notes/Index.alder:71:16: warning[W0002]: hole ?expected has type Bool
    Task.succeed ?expected
                 ^^^^^^^^^
[exit 0]
$ alder fill --decl Notes.Index.view - <<'EOF'
<Layout.Page title="Notes">
  <h1>Notes</h1>

  <al-card>
    <form submit={Add}>
      <Errors form />
      <al-input field="title" label="Title" />
      <al-textarea field="body" label="Body" />
      <al-button type="submit">Add note</al-button>
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
app/Notes/Index.alder:55:34: error[E0002]: unknown `Notes.Show`
            <a class="title" href={Notes.Show { id: note.id }}>{note.title}</a>
                                   ^^^^^^^^^^
alder fill: the result does not check (the errors above): nothing was written
[exit 1]
```

The harness stopped at line 36 of your script: `alder fill --decl Notes.Index.view - <<'EOF'`: it exited 1. The commands after it were not run.

Your script stopped at command 9; the app is as the commands before it left it; continue.

The program type-checks but `alder build` refused it; fix the cause below.

```text
app/Auth/Login.alder:47:33: warning[W0002]: hole ?afterLogin has type Page
          Task.succeed (Redirect (?afterLogin))
                                  ^^^^^^^^^^^
app/Auth/Login.alder:86:85: warning[W0002]: hole ?afterLogin has type Page
    Task.succeed ({ answer: r.answer, user: r.session.user } == { answer: Redirected (?afterLogin), user: Just me })
                                                                                      ^^^^^^^^^^^
app/Notes/Index.alder:43:36: warning[W0002]: hole ?deleteId has type Note.NoteId
      <al-button click={Delete { id: ?deleteId }}>Delete</al-button>
                                     ^^^^^^^^^
app/Notes/Index.alder:71:16: warning[W0002]: hole ?expected has type Bool
    Task.succeed ?expected
                 ^^^^^^^^^
alder build: the program has 4 typed hole(s); fill them before building
```
