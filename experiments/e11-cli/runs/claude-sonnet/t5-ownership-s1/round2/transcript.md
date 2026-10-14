### Round 2

```text
$ alder new page Notes.Index / --blank --requires user
wrote app/Notes/Index.alder
app/Notes/Index.alder:8:23: warning[W0002]: hole ?load has type Query Notes.Index.Model
  load params session = ?load
                        ^^^^^
app/Notes/Index.alder:11:22: warning[W0002]: hole ?view has type Html Notes.Index.Msg
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
  let found <- Db.get Note params.id
  match found {
    Just n -> if n.owner == session.user then Query.succeed { note: n } else Query.notFound,
    Nothing -> Query.notFound,
  }
}
EOF
edited app/Notes/Show.alder
--- a/app/Notes/Show.alder
+++ b/app/Notes/Show.alder
@@ -8,7 +8,13 @@
 type Model = { note: Note }
 
 load : Params -> Session -> Query Model
-load params session = ?load
+load params session = {
+  let found <- Db.get Note params.id
+  match found {
+    Just n -> if n.owner == session.user then Query.succeed { note: n } else Query.notFound,
+    Nothing -> Query.notFound,
+  }
+}
 
 view : Model -> Session -> Html Msg
 view model session = ?view
app/Notes/Show.alder:20:22: warning[W0002]: hole ?view has type Html Notes.Show.Msg
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
edited app/Notes/Show.alder
--- a/app/Notes/Show.alder
+++ b/app/Notes/Show.alder
@@ -1,5 +1,6 @@
 module Notes.Show
 
+import Layout
 import Note
 import User
 
@@ -17,7 +18,11 @@
 }
 
 view : Model -> Session -> Html Msg
-view model session = ?view
+view model session = <Layout.Page title={model.note.title}>
+  <h1 id="title">{model.note.title}</h1>
+  <p id="body">{model.note.body}</p>
+  <a href={Notes.Index {}}>Back</a>
+</Layout.Page>
 
 test "a visitor is sent to log in" = {
   let me <- Db.insert User { email: Text.Email "email-1@example.com", name: Text.NonEmpty "name-1", passwordHash: "passwordhash-1" }
added import Layout to app/Notes/Show.alder
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
{
  let notes <- Db.from Note |> Db.where (Note.owner.eq session.user) |> Db.orderBy Note.created.desc |> Db.all
  Query.succeed { notes: notes }
}
EOF
edited app/Notes/Index.alder
--- a/app/Notes/Index.alder
+++ b/app/Notes/Index.alder
@@ -7,7 +7,10 @@
 type Model = { notes: List Note }
 
 load : Params -> Session -> Query Model
-load params session = ?load
+load params session = {
+  let notes <- Db.from Note |> Db.where (Note.owner.eq session.user) |> Db.orderBy Note.created.desc |> Db.all
+  Query.succeed { notes: notes }
+}
 
 view : Model -> Session -> Html Msg
 view model session = ?view
app/Notes/Index.alder:16:22: warning[W0002]: hole ?view has type Html Notes.Index.Msg
  view model session = ?view
                       ^^^^^
[exit 0]
$ alder fill Notes.Index.view - <<'EOF'
<Layout.Page title="Notes">
  <h1>Notes</h1>
</Layout.Page>
EOF
edited app/Notes/Index.alder
--- a/app/Notes/Index.alder
+++ b/app/Notes/Index.alder
@@ -1,5 +1,6 @@
 module Notes.Index
 
+import Layout
 import Note
 
 page "/" { } requires user
@@ -13,7 +14,9 @@
 }
 
 view : Model -> Session -> Html Msg
-view model session = ?view
+view model session = <Layout.Page title="Notes">
+  <h1>Notes</h1>
+</Layout.Page>
 
 test "a visitor is sent to log in" = {
   let r <- Test.open (Notes.Index {})
added import Layout to app/Notes/Index.alder
[exit 0]
$ alder new form Notes.Index NewNote title:Text.NonEmpty body:Text --msg Add
edited app/Notes/Index.alder
--- a/app/Notes/Index.alder
+++ b/app/Notes/Index.alder
@@ -2,6 +2,7 @@
 
 import Layout
 import Note
+import User
 
 page "/" { } requires user
 
@@ -13,12 +14,50 @@
   Query.succeed { notes: notes }
 }
 
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
app/Notes/Index.alder:26:12: warning[W0002]: hole ?add has type Task Nav
    Add p -> ?add,
             ^^^^
app/Notes/Index.alder:54:29: warning[W0002]: hole ?expected has type Answer
    Task.succeed (r.answer == ?expected)
                              ^^^^^^^^^
[exit 0]
$ alder new msg Notes.Index Delete id:Note.NoteId
edited app/Notes/Index.alder
--- a/app/Notes/Index.alder
+++ b/app/Notes/Index.alder
@@ -21,9 +21,12 @@
 
 msg Add form NewNote
 
+msg Delete { id: Note.NoteId }
+
 update : Msg -> Model -> Session -> Task Nav
 update msg model session = match msg {
   Add p -> ?add,
+  Delete p -> ?delete,
 }
 
 view : Model -> Session -> Html Msg
@@ -39,6 +42,8 @@
       <al-button type="submit">New note</al-button>
     </form>
   </al-card>
+
+  <al-button click={Delete { id: ?deleteId }}>Delete</al-button>
 </Layout.Page>
 
 test "a visitor is sent to log in" = {
@@ -61,3 +66,16 @@
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
app/Notes/Index.alder:28:12: warning[W0002]: hole ?add has type Task Nav
    Add p -> ?add,
             ^^^^
app/Notes/Index.alder:29:15: warning[W0002]: hole ?delete has type Task Nav
    Delete p -> ?delete,
                ^^^^^^^
app/Notes/Index.alder:46:34: warning[W0002]: hole ?deleteId has type Note.NoteId
    <al-button click={Delete { id: ?deleteId }}>Delete</al-button>
                                   ^^^^^^^^^
app/Notes/Index.alder:59:29: warning[W0002]: hole ?expected has type Answer
    Task.succeed (r.answer == ?expected)
                              ^^^^^^^^^
app/Notes/Index.alder:80:16: warning[W0002]: hole ?expected has type Bool
    Task.succeed ?expected
                 ^^^^^^^^^
[exit 0]
$ alder fill Notes.Index.update.Add - <<'EOF'
{
  let _ <- Db.insert Note { title: p.form.title, body: p.form.body, owner: session.user }
  Task.succeed (Redirect (Notes.Index {}))
}
EOF
edited app/Notes/Index.alder
--- a/app/Notes/Index.alder
+++ b/app/Notes/Index.alder
@@ -25,7 +25,10 @@
 
 update : Msg -> Model -> Session -> Task Nav
 update msg model session = match msg {
-  Add p -> ?add,
+  Add p -> {
+    let _ <- Db.insert Note { title: p.form.title, body: p.form.body, owner: session.user }
+    Task.succeed (Redirect (Notes.Index {}))
+  },
   Delete p -> ?delete,
 }
 
app/Notes/Index.alder:32:15: warning[W0002]: hole ?delete has type Task Nav
    Delete p -> ?delete,
                ^^^^^^^
app/Notes/Index.alder:49:34: warning[W0002]: hole ?deleteId has type Note.NoteId
    <al-button click={Delete { id: ?deleteId }}>Delete</al-button>
                                   ^^^^^^^^^
app/Notes/Index.alder:62:29: warning[W0002]: hole ?expected has type Answer
    Task.succeed (r.answer == ?expected)
                              ^^^^^^^^^
app/Notes/Index.alder:83:16: warning[W0002]: hole ?expected has type Bool
    Task.succeed ?expected
                 ^^^^^^^^^
[exit 0]
$ alder fill Notes.Index.update.Delete - <<'EOF'
{
  let found <- Db.get Note p.id
  match found {
    Just n -> if n.owner == session.user then {
      let _ <- Db.delete Note p.id
      Task.succeed (Redirect (Notes.Index {}))
    } else Task.notFound,
    Nothing -> Task.notFound,
  }
}
EOF
edited app/Notes/Index.alder
--- a/app/Notes/Index.alder
+++ b/app/Notes/Index.alder
@@ -29,7 +29,16 @@
     let _ <- Db.insert Note { title: p.form.title, body: p.form.body, owner: session.user }
     Task.succeed (Redirect (Notes.Index {}))
   },
-  Delete p -> ?delete,
+  Delete p -> {
+    let found <- Db.get Note p.id
+    match found {
+      Just n -> if n.owner == session.user then {
+        let _ <- Db.delete Note p.id
+        Task.succeed (Redirect (Notes.Index {}))
+      } else Task.notFound,
+      Nothing -> Task.notFound,
+    }
+  },
 }
 
 view : Model -> Session -> Html Msg
app/Notes/Index.alder:58:34: warning[W0002]: hole ?deleteId has type Note.NoteId
    <al-button click={Delete { id: ?deleteId }}>Delete</al-button>
                                   ^^^^^^^^^
app/Notes/Index.alder:71:29: warning[W0002]: hole ?expected has type Answer
    Task.succeed (r.answer == ?expected)
                              ^^^^^^^^^
app/Notes/Index.alder:92:16: warning[W0002]: hole ?expected has type Bool
    Task.succeed ?expected
                 ^^^^^^^^^
[exit 0]
$ alder fill --decl Notes.Index.view - <<'EOF'
\model session ->
  <Layout.Page title="Notes">
    <h1>Notes</h1>
    <form submit={Add}>
      <Errors form />
      <Input field="title" label="Title" />
      <Textarea field="body" label="Body" />
      <button>Add</button>
    </form>
    <ul id="notes">
      {for n in model.notes -> <li><a class="title" href={Notes.Show { id: n.id }}>{n.title}</a><button click={Delete { id: n.id }}>Delete</button></li>}
    </ul>
    <button click={App.Logout}>Log out</button>
  </Layout.Page>
EOF
app/Notes/Index.alder:45:22: error[E0003]: expected Html Notes.Index.Msg, got _ -> _ -> Html Notes.Index.Msg
  view model session = \model session ->
                       ^
app/Notes/Index.alder:55:17: error[E0003]: cannot infer the record type of `model`; add a type annotation
        {for n in model.notes -> <li><a class="title" href={Notes.Show { id: n.id }}>{n.title}</a><button click={Delete { id: n.id }}>Delete</button></li>}
                  ^^^^^^^^^^^
app/Notes/Index.alder:55:76: error[E0003]: cannot infer the record type of `n`; add a type annotation
        {for n in model.notes -> <li><a class="title" href={Notes.Show { id: n.id }}>{n.title}</a><button click={Delete { id: n.id }}>Delete</button></li>}
                                                                             ^^^^
app/Notes/Index.alder:55:85: error[E0003]: cannot infer the record type of `n`; add a type annotation
        {for n in model.notes -> <li><a class="title" href={Notes.Show { id: n.id }}>{n.title}</a><button click={Delete { id: n.id }}>Delete</button></li>}
                                                                                      ^^^^^^^
app/Notes/Index.alder:55:125: error[E0003]: cannot infer the record type of `n`; add a type annotation
        {for n in model.notes -> <li><a class="title" href={Notes.Show { id: n.id }}>{n.title}</a><button click={Delete { id: n.id }}>Delete</button></li>}
                                                                                                                              ^^^^
app/Notes/Index.alder:70:29: warning[W0002]: hole ?expected has type Answer
    Task.succeed (r.answer == ?expected)
                              ^^^^^^^^^
app/Notes/Index.alder:91:16: warning[W0002]: hole ?expected has type Bool
    Task.succeed ?expected
                 ^^^^^^^^^
alder fill: the result does not check (the errors above): nothing was written
[exit 1]
```

The harness stopped at line 60 of your script: `alder fill --decl Notes.Index.view - <<'EOF'`: it exited 1. The commands after it were not run.

Your script stopped at command 12; the app is as the commands before it left it; continue.

The program type-checks but `alder build` refused it; fix the cause below.

```text
app/Auth/Login.alder:47:33: warning[W0002]: hole ?afterLogin has type Page
          Task.succeed (Redirect (?afterLogin))
                                  ^^^^^^^^^^^
app/Auth/Login.alder:86:85: warning[W0002]: hole ?afterLogin has type Page
    Task.succeed ({ answer: r.answer, user: r.session.user } == { answer: Redirected (?afterLogin), user: Just me })
                                                                                      ^^^^^^^^^^^
app/Notes/Index.alder:58:34: warning[W0002]: hole ?deleteId has type Note.NoteId
    <al-button click={Delete { id: ?deleteId }}>Delete</al-button>
                                   ^^^^^^^^^
app/Notes/Index.alder:71:29: warning[W0002]: hole ?expected has type Answer
    Task.succeed (r.answer == ?expected)
                              ^^^^^^^^^
app/Notes/Index.alder:92:16: warning[W0002]: hole ?expected has type Bool
    Task.succeed ?expected
                 ^^^^^^^^^
alder build: the program has 5 typed hole(s); fill them before building
```
