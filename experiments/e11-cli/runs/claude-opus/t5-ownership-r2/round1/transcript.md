### Round 1

```text
$ alder new page Notes.Index / --requires user --blank
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
{
  let notes <- Db.mine Note |> Db.orderBy Note.created.desc |> Db.all
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
+  let notes <- Db.mine Note |> Db.orderBy Note.created.desc |> Db.all
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
 
@@ -13,12 +14,55 @@
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
+  Add p -> {
+    let _ <- Db.insertMine Note { title: p.form.title, body: p.form.body }
+    Task.succeed (Redirect (Notes.Index {}))
+  },
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
[exit 0]
$ alder new msg Notes.Index Delete id:Note.NoteId
edited app/Notes/Index.alder
--- a/app/Notes/Index.alder
+++ b/app/Notes/Index.alder
@@ -21,12 +21,15 @@
 
 msg Add form NewNote
 
+msg Delete { id: Note.NoteId }
+
 update : Msg -> Model -> Session -> Task Nav
 update msg model session = match msg {
   Add p -> {
     let _ <- Db.insertMine Note { title: p.form.title, body: p.form.body }
     Task.succeed (Redirect (Notes.Index {}))
   },
+  Delete p -> ?delete,
 }
 
 view : Model -> Session -> Html Msg
@@ -42,6 +45,8 @@
       <al-button type="submit">New note</al-button>
     </form>
   </al-card>
+
+  <al-button click={Delete { id: ?deleteId }}>Delete</al-button>
 </Layout.Page>
 
 test "a visitor is sent to log in" = {
@@ -66,3 +71,16 @@
   let rows <- Db.from Note |> Db.count
   Task.succeed ({ answer: r.answer, rows: rows } == { answer: FormRejected [{ field: "title", message: "must not be empty" }], rows: 0 })
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
app/Notes/Index.alder:32:15: warning[W0002]: hole ?delete has type Task Nav
    Delete p -> ?delete,
                ^^^^^^^
app/Notes/Index.alder:49:34: warning[W0002]: hole ?deleteId has type Note.NoteId
    <al-button click={Delete { id: ?deleteId }}>Delete</al-button>
                                   ^^^^^^^^^
app/Notes/Index.alder:85:16: warning[W0002]: hole ?expected has type Bool
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
@@ -29,7 +29,10 @@
     let _ <- Db.insertMine Note { title: p.form.title, body: p.form.body }
     Task.succeed (Redirect (Notes.Index {}))
   },
-  Delete p -> ?delete,
+  Delete p -> {
+    let _ <- Db.deleteMine Note p.id
+    Task.succeed (Redirect (Notes.Index {}))
+  },
 }
 
 view : Model -> Session -> Html Msg
app/Notes/Index.alder:52:34: warning[W0002]: hole ?deleteId has type Note.NoteId
    <al-button click={Delete { id: ?deleteId }}>Delete</al-button>
                                   ^^^^^^^^^
app/Notes/Index.alder:88:16: warning[W0002]: hole ?expected has type Bool
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
      <al-button type="submit">Add</al-button>
    </form>
  </al-card>
  <ul id="notes">
    {for n in model.notes ->
      <li>
        <a class="title" href={Notes.Show { id: n.id }}>{n.title}</a>
        <al-button variant="ghost" size="sm" click={Delete { id: n.id }}>Delete</al-button>
      </li>}
  </ul>
  <al-button variant="outline" click={App.Logout}>Log out</al-button>
</Layout.Page>
EOF
edited app/Notes/Index.alder
--- a/app/Notes/Index.alder
+++ b/app/Notes/Index.alder
@@ -36,21 +36,26 @@
 }
 
 view : Model -> Session -> Html Msg
-view model session = <Layout.Page title="Notes">
-  <h1>Notes</h1>
-
-  <al-card>
-    <h2 slot="header">New note</h2>
-    <form submit={Add}>
-      <al-input field="title" label="Title" />
-      <al-textarea field="body" label="Body" />
-      <al-alert variant="destructive"><Errors form /></al-alert>
-      <al-button type="submit">New note</al-button>
-    </form>
-  </al-card>
-
-  <al-button click={Delete { id: ?deleteId }}>Delete</al-button>
-</Layout.Page>
+view model session =
+  <Layout.Page title="Notes">
+    <h1>Notes</h1>
+    <al-card>
+      <form submit={Add}>
+        <Errors form />
+        <al-input field="title" label="Title" />
+        <al-textarea field="body" label="Body" />
+        <al-button type="submit">Add</al-button>
+      </form>
+    </al-card>
+    <ul id="notes">
+      {for n in model.notes ->
+        <li>
+          <a class="title" href={Notes.Show { id: n.id }}>{n.title}</a>
+          <al-button variant="ghost" size="sm" click={Delete { id: n.id }}>Delete</al-button>
+        </li>}
+    </ul>
+    <al-button variant="outline" click={App.Logout}>Log out</al-button>
+  </Layout.Page>
 
 test "a visitor is sent to log in" = {
   let r <- Test.open (Notes.Index {})
app/Notes/Index.alder:93:16: warning[W0002]: hole ?expected has type Bool
    Task.succeed ?expected
                 ^^^^^^^^^
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
 
@@ -14,7 +15,11 @@
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
$ alder new test Notes.Index "deleting another user's note is not found"
edited app/Notes/Index.alder
--- a/app/Notes/Index.alder
+++ b/app/Notes/Index.alder
@@ -92,3 +92,7 @@
   let after <- Db.get Note note1
   Task.succeed ?expected
 }
+
+test "deleting another user's note is not found" = {
+  ?test
+}
app/Notes/Index.alder:93:16: warning[W0002]: hole ?expected has type Bool
    Task.succeed ?expected
                 ^^^^^^^^^
app/Notes/Index.alder:97:3: warning[W0002]: hole ?test has type Task Bool
    ?test
    ^^^^^
[exit 0]
$ alder fill 'Notes.Index."deleting another user'"'"'s note is not found"' - <<'EOF'
{
  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
  let other <- Db.insert User { email: Text.Email "other@example.com", name: Text.NonEmpty "Other", passwordHash: "" }
  let id <- Db.insert Note { title: Text.NonEmpty "Theirs", body: "b", owner: other }
  let _ <- Test.login { user: Just me }
  let r <- Test.run (update (Delete { id: id }) { notes: [] } { user: me })
  let after <- Db.get Note id
  Task.succeed ({ value: r.value, kept: Maybe.isJust after } == { value: Err "not found", kept: True })
}
EOF
edited app/Notes/Index.alder
--- a/app/Notes/Index.alder
+++ b/app/Notes/Index.alder
@@ -94,5 +94,11 @@
 }
 
 test "deleting another user's note is not found" = {
-  ?test
+  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
+  let other <- Db.insert User { email: Text.Email "other@example.com", name: Text.NonEmpty "Other", passwordHash: "" }
+  let id <- Db.insert Note { title: Text.NonEmpty "Theirs", body: "b", owner: other }
+  let _ <- Test.login { user: Just me }
+  let r <- Test.run (update (Delete { id: id }) { notes: [] } { user: me })
+  let after <- Db.get Note id
+  Task.succeed ({ value: r.value, kept: Maybe.isJust after } == { value: Err "not found", kept: True })
 }
app/Notes/Index.alder:93:16: warning[W0002]: hole ?expected has type Bool
    Task.succeed ?expected
                 ^^^^^^^^^
[exit 0]
```

The program type-checks but `alder build` refused it; fix the cause below.

```text
app/Auth/Login.alder:47:33: warning[W0002]: hole ?afterLogin has type Page
          Task.succeed (Redirect (?afterLogin))
                                  ^^^^^^^^^^^
app/Auth/Login.alder:86:85: warning[W0002]: hole ?afterLogin has type Page
    Task.succeed ({ answer: r.answer, user: r.session.user } == { answer: Redirected (?afterLogin), user: Just me })
                                                                                      ^^^^^^^^^^^
app/Notes/Index.alder:93:16: warning[W0002]: hole ?expected has type Bool
    Task.succeed ?expected
                 ^^^^^^^^^
alder build: the program has 3 typed hole(s); fill them before building
```
