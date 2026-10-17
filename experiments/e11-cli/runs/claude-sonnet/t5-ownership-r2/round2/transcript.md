### Round 2

```text
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
$ alder new page Notes.Show '/notes/{id}' --blank --param id:Note.NoteId --requires user
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
  let found <- Db.from Note |> Db.where (Note.id.eq params.id) |> Db.where (Note.owner.eq session.user) |> Db.first
  match found {
    Just n -> Query.succeed { note: n },
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
+  let found <- Db.from Note |> Db.where (Note.id.eq params.id) |> Db.where (Note.owner.eq session.user) |> Db.first
+  match found {
+    Just n -> Query.succeed { note: n },
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
$ alder fill --decl Notes.Index.view - <<'EOF'
<Layout.Page title="Notes">
  <h1>Notes</h1>
  <al-card>
    <form submit={Add}>
      <al-input field="title" label="Title" />
      <al-textarea field="body" label="Body" />
      <Errors form />
      <al-button type="submit">New note</al-button>
    </form>
  </al-card>
  <ul id="notes">
    {for n in model.notes ->
      <li>
        <a class="title" href={Notes.Show { id: n.id }}>{n.title}</a>
        <al-button click={Delete { id: n.id }}>Delete</al-button>
      </li>}
  </ul>
</Layout.Page>
EOF
edited app/Notes/Index.alder
--- a/app/Notes/Index.alder
+++ b/app/Notes/Index.alder
@@ -30,21 +30,25 @@
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
+        <al-input field="title" label="Title" />
+        <al-textarea field="body" label="Body" />
+        <Errors form />
+        <al-button type="submit">New note</al-button>
+      </form>
+    </al-card>
+    <ul id="notes">
+      {for n in model.notes ->
+        <li>
+          <a class="title" href={Notes.Show { id: n.id }}>{n.title}</a>
+          <al-button click={Delete { id: n.id }}>Delete</al-button>
+        </li>}
+    </ul>
+  </Layout.Page>
 
 test "a visitor is sent to log in" = {
   let r <- Test.open (Notes.Index {})
app/Notes/Index.alder:28:12: warning[W0002]: hole ?add has type Task Nav
    Add p -> ?add,
             ^^^^
app/Notes/Index.alder:29:15: warning[W0002]: hole ?delete has type Task Nav
    Delete p -> ?delete,
                ^^^^^^^
app/Notes/Index.alder:63:29: warning[W0002]: hole ?expected has type Answer
    Task.succeed (r.answer == ?expected)
                              ^^^^^^^^^
app/Notes/Index.alder:84:16: warning[W0002]: hole ?expected has type Bool
    Task.succeed ?expected
                 ^^^^^^^^^
[exit 0]
$ alder fill Notes.Index.update.Add --hole ?add - <<'EOF'
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
app/Notes/Index.alder:66:29: warning[W0002]: hole ?expected has type Answer
    Task.succeed (r.answer == ?expected)
                              ^^^^^^^^^
app/Notes/Index.alder:87:16: warning[W0002]: hole ?expected has type Bool
    Task.succeed ?expected
                 ^^^^^^^^^
[exit 0]
$ alder fill Notes.Index.update.Delete --hole ?delete - <<'EOF'
{
  let found <- Db.from Note |> Db.where (Note.id.eq p.id) |> Db.where (Note.owner.eq session.user) |> Db.first
  match found {
    Just n -> {
      let _ <- Db.delete Note p.id
      Task.succeed (Redirect (Notes.Index {}))
    },
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
+    let found <- Db.from Note |> Db.where (Note.id.eq p.id) |> Db.where (Note.owner.eq session.user) |> Db.first
+    match found {
+      Just n -> {
+        let _ <- Db.delete Note p.id
+        Task.succeed (Redirect (Notes.Index {}))
+      },
+      Nothing -> Task.notFound,
+    }
+  },
 }
 
 view : Model -> Session -> Html Msg
app/Notes/Index.alder:75:29: warning[W0002]: hole ?expected has type Answer
    Task.succeed (r.answer == ?expected)
                              ^^^^^^^^^
app/Notes/Index.alder:96:16: warning[W0002]: hole ?expected has type Bool
    Task.succeed ?expected
                 ^^^^^^^^^
[exit 0]
$ alder fill 'Notes.Index."submitting NewNote"' --hole ?expected - <<'EOF'
Redirected (Notes.Index {})
EOF
edited app/Notes/Index.alder
--- a/app/Notes/Index.alder
+++ b/app/Notes/Index.alder
@@ -72,7 +72,7 @@
   let _ <- Test.login { user: Just me }
   let _ <- Test.get (Notes.Index {})
   let r <- Test.submit NewNote { title: Text.NonEmpty "title-1", body: "body-1" }
-  Task.succeed (r.answer == ?expected)
+  Task.succeed (r.answer == Redirected (Notes.Index {}))
 }
 
 test "NewNote refuses an empty title" = {
app/Notes/Index.alder:96:16: warning[W0002]: hole ?expected has type Bool
    Task.succeed ?expected
                 ^^^^^^^^^
[exit 0]
$ alder fill 'Notes.Index."pressing Delete"' --hole ?expected - <<'EOF'
{ answer: r.answer, gone: Maybe.map (\n -> n.title) after } == { answer: Redirected (Notes.Index {}), gone: Nothing }
EOF
edited app/Notes/Index.alder
--- a/app/Notes/Index.alder
+++ b/app/Notes/Index.alder
@@ -83,9 +83,6 @@
   Task.succeed (r.answer == FormRejected [{ field: "title", message: "must not be empty" }])
 }
 
--- Fill ?expected with what pressing Delete must do, so that a test fails when the arm
--- does something else: its answer (`r.answer`: Rerendered, Redirected …) and what it
--- changed (`after`, the row read again), as `{ answer: r.answer, … } == { answer: Rerendered, … }`.
 test "pressing Delete" = {
   let me <- Db.insert User { email: Text.Email "email-1@example.com", name: Text.NonEmpty "name-1", passwordHash: "passwordhash-1" }
   let _ <- Test.login { user: Just me }
@@ -93,5 +90,5 @@
   let _ <- Test.get (Notes.Index {})
   let r <- Test.click (Delete { id: note1 })
   let after <- Db.get Note note1
-  Task.succeed ?expected
+  Task.succeed ({ answer: r.answer, gone: Maybe.map (\n -> n.title) after } == { answer: Redirected (Notes.Index {}), gone: Nothing })
 }
[exit 0]
$ alder new test Notes.Index "deleting another user's note is not found"
edited app/Notes/Index.alder
--- a/app/Notes/Index.alder
+++ b/app/Notes/Index.alder
@@ -92,3 +92,7 @@
   let after <- Db.get Note note1
   Task.succeed ({ answer: r.answer, gone: Maybe.map (\n -> n.title) after } == { answer: Redirected (Notes.Index {}), gone: Nothing })
 }
+
+test "deleting another user's note is not found" = {
+  ?test
+}
app/Notes/Index.alder:97:3: warning[W0002]: hole ?test has type Task Bool
    ?test
    ^^^^^
[exit 0]
$ alder fill 'Notes.Index."deleting another user'"'"'s note is not found"' - <<'EOF'
{
  let a <- Db.insert User { email: Text.Email "a@example.com", name: Text.NonEmpty "A", passwordHash: "" }
  let b <- Db.insert User { email: Text.Email "b@example.com", name: Text.NonEmpty "B", passwordHash: "" }
  let n <- Db.insert Note { title: Text.NonEmpty "T", body: "B", owner: a }
  let r <- Test.run (update (Delete { id: n }) { notes: [] } { user: b })
  let after <- Db.get Note n
  Task.succeed ({ value: r.value, kept: Maybe.isJust after } == { value: Err "not found", kept: True })
}
EOF
edited app/Notes/Index.alder
--- a/app/Notes/Index.alder
+++ b/app/Notes/Index.alder
@@ -94,5 +94,10 @@
 }
 
 test "deleting another user's note is not found" = {
-  ?test
+  let a <- Db.insert User { email: Text.Email "a@example.com", name: Text.NonEmpty "A", passwordHash: "" }
+  let b <- Db.insert User { email: Text.Email "b@example.com", name: Text.NonEmpty "B", passwordHash: "" }
+  let n <- Db.insert Note { title: Text.NonEmpty "T", body: "B", owner: a }
+  let r <- Test.run (update (Delete { id: n }) { notes: [] } { user: b })
+  let after <- Db.get Note n
+  Task.succeed ({ value: r.value, kept: Maybe.isJust after } == { value: Err "not found", kept: True })
 }
[exit 0]
$ alder new test Notes.Index "deleting a missing note is not found"
edited app/Notes/Index.alder
--- a/app/Notes/Index.alder
+++ b/app/Notes/Index.alder
@@ -101,3 +101,7 @@
   let after <- Db.get Note n
   Task.succeed ({ value: r.value, kept: Maybe.isJust after } == { value: Err "not found", kept: True })
 }
+
+test "deleting a missing note is not found" = {
+  ?test
+}
app/Notes/Index.alder:106:3: warning[W0002]: hole ?test has type Task Bool
    ?test
    ^^^^^
[exit 0]
$ alder fill 'Notes.Index."deleting a missing note is not found"' - <<'EOF'
{
  let a <- Db.insert User { email: Text.Email "a@example.com", name: Text.NonEmpty "A", passwordHash: "" }
  let r <- Test.run (update (Delete { id: Note.NoteId "missing" }) { notes: [] } { user: a })
  Task.succeed (r.value == Err "not found")
}
EOF
edited app/Notes/Index.alder
--- a/app/Notes/Index.alder
+++ b/app/Notes/Index.alder
@@ -103,5 +103,7 @@
 }
 
 test "deleting a missing note is not found" = {
-  ?test
+  let a <- Db.insert User { email: Text.Email "a@example.com", name: Text.NonEmpty "A", passwordHash: "" }
+  let r <- Test.run (update (Delete { id: Note.NoteId "missing" }) { notes: [] } { user: a })
+  Task.succeed (r.value == Err "not found")
 }
[exit 0]
$ alder new test Notes.Index "the owner can delete a note"
edited app/Notes/Index.alder
--- a/app/Notes/Index.alder
+++ b/app/Notes/Index.alder
@@ -107,3 +107,7 @@
   let r <- Test.run (update (Delete { id: Note.NoteId "missing" }) { notes: [] } { user: a })
   Task.succeed (r.value == Err "not found")
 }
+
+test "the owner can delete a note" = {
+  ?test
+}
app/Notes/Index.alder:112:3: warning[W0002]: hole ?test has type Task Bool
    ?test
    ^^^^^
[exit 0]
$ alder fill 'Notes.Index."the owner can delete a note"' - <<'EOF'
{
  let a <- Db.insert User { email: Text.Email "a@example.com", name: Text.NonEmpty "A", passwordHash: "" }
  let n <- Db.insert Note { title: Text.NonEmpty "T", body: "B", owner: a }
  let r <- Test.run (update (Delete { id: n }) { notes: [] } { user: a })
  let after <- Db.get Note n
  Task.succeed ({ value: r.value, kept: Maybe.isJust after } == { value: Ok (Redirect (Notes.Index {})), kept: False })
}
EOF
edited app/Notes/Index.alder
--- a/app/Notes/Index.alder
+++ b/app/Notes/Index.alder
@@ -109,5 +109,9 @@
 }
 
 test "the owner can delete a note" = {
-  ?test
+  let a <- Db.insert User { email: Text.Email "a@example.com", name: Text.NonEmpty "A", passwordHash: "" }
+  let n <- Db.insert Note { title: Text.NonEmpty "T", body: "B", owner: a }
+  let r <- Test.run (update (Delete { id: n }) { notes: [] } { user: a })
+  let after <- Db.get Note n
+  Task.succeed ({ value: r.value, kept: Maybe.isJust after } == { value: Ok (Redirect (Notes.Index {})), kept: False })
 }
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
ok   Notes.Index "NewNote refuses an empty title"
ok   Notes.Index "pressing Delete"
ok   Notes.Index "deleting another user's note is not found"
ok   Notes.Index "deleting a missing note is not found"
ok   Notes.Index "the owner can delete a note"
ok   Notes.Show "a visitor is sent to log in"
13 passed, 0 failed
[exit 0]
```

The harness ran `alder check`, `alder build` and `alder test` on the app: all pass.
