### Round 3

```text
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
$ alder fill --decl Notes.Show.load - <<'EOF'
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
$ alder fill --decl Notes.Show.view - <<'EOF'
<Layout.Page title={model.note.title}>
  <h1 id="title">{model.note.title}</h1>
  <p id="body">{model.note.body}</p>
  <a href={Notes.Index {}}>Back to notes</a>
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
 
@@ -14,7 +15,12 @@
 }
 
 view : Model -> Session -> Html Msg
-view model session = ?view
+view model session =
+  <Layout.Page title={model.note.title}>
+    <h1 id="title">{model.note.title}</h1>
+    <p id="body">{model.note.body}</p>
+    <a href={Notes.Index {}}>Back to notes</a>
+  </Layout.Page>
 
 test "a visitor is sent to log in" = {
   let me <- Db.insert User { email: Text.Email "email-1@example.com", name: Text.NonEmpty "name-1", passwordHash: "passwordhash-1" }
added import Layout to app/Notes/Show.alder
[exit 0]
$ alder new test Notes.Show "the owner sees the note"
edited app/Notes/Show.alder
--- a/app/Notes/Show.alder
+++ b/app/Notes/Show.alder
@@ -28,3 +28,7 @@
   let r <- Test.open (Notes.Show { id: note1 })
   Task.succeed (r.answer == Redirected (Auth.Login {}))
 }
+
+test "the owner sees the note" = {
+  ?test
+}
app/Notes/Show.alder:33:3: warning[W0002]: hole ?test has type Task Bool
    ?test
    ^^^^^
[exit 0]
$ alder fill 'Notes.Show."the owner sees the note"' - <<'EOF'
{
  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
  let note1 <- Db.insert Note { title: Text.NonEmpty "Milk", body: "2 litres", owner: me }
  let _ <- Test.login { user: Just me }
  let page <- Test.get (Notes.Show { id: note1 })
  Task.succeed ({ title: Test.text "#title" page.doc, body: Test.text "#body" page.doc } == { title: Just "Milk", body: Just "2 litres" })
}
EOF
edited app/Notes/Show.alder
--- a/app/Notes/Show.alder
+++ b/app/Notes/Show.alder
@@ -30,5 +30,9 @@
 }
 
 test "the owner sees the note" = {
-  ?test
+  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
+  let note1 <- Db.insert Note { title: Text.NonEmpty "Milk", body: "2 litres", owner: me }
+  let _ <- Test.login { user: Just me }
+  let page <- Test.get (Notes.Show { id: note1 })
+  Task.succeed ({ title: Test.text "#title" page.doc, body: Test.text "#body" page.doc } == { title: Just "Milk", body: Just "2 litres" })
 }
[exit 0]
$ alder new test Notes.Show "a stranger gets not found"
edited app/Notes/Show.alder
--- a/app/Notes/Show.alder
+++ b/app/Notes/Show.alder
@@ -36,3 +36,7 @@
   let page <- Test.get (Notes.Show { id: note1 })
   Task.succeed ({ title: Test.text "#title" page.doc, body: Test.text "#body" page.doc } == { title: Just "Milk", body: Just "2 litres" })
 }
+
+test "a stranger gets not found" = {
+  ?test
+}
app/Notes/Show.alder:41:3: warning[W0002]: hole ?test has type Task Bool
    ?test
    ^^^^^
[exit 0]
$ alder fill 'Notes.Show."a stranger gets not found"' - <<'EOF'
{
  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
  let other <- Db.insert User { email: Text.Email "other@example.com", name: Text.NonEmpty "Other", passwordHash: "" }
  let note1 <- Db.insert Note { title: Text.NonEmpty "Milk", body: "", owner: me }
  let _ <- Test.login { user: Just other }
  let r <- Test.open (Notes.Show { id: note1 })
  Task.succeed (r.answer == PageMissing)
}
EOF
edited app/Notes/Show.alder
--- a/app/Notes/Show.alder
+++ b/app/Notes/Show.alder
@@ -38,5 +38,10 @@
 }
 
 test "a stranger gets not found" = {
-  ?test
+  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
+  let other <- Db.insert User { email: Text.Email "other@example.com", name: Text.NonEmpty "Other", passwordHash: "" }
+  let note1 <- Db.insert Note { title: Text.NonEmpty "Milk", body: "", owner: me }
+  let _ <- Test.login { user: Just other }
+  let r <- Test.open (Notes.Show { id: note1 })
+  Task.succeed (r.answer == PageMissing)
 }
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
edited app/Notes/Index.alder
--- a/app/Notes/Index.alder
+++ b/app/Notes/Index.alder
@@ -40,17 +40,24 @@
   <Layout.Page title="Notes">
     <h1>Notes</h1>
 
-    <al-button click={Delete { id: ?deleteId }}>Delete</al-button>
-
     <al-card>
-      <h2 slot="header">New note</h2>
       <form submit={Add}>
+        <Errors form />
         <al-input field="title" label="Title" />
         <al-textarea field="body" label="Body" />
-        <al-alert variant="destructive"><Errors form /></al-alert>
-        <al-button type="submit">New note</al-button>
+        <al-button type="submit">Add note</al-button>
       </form>
     </al-card>
+
+    <ul id="notes">
+      {for note in model.notes ->
+        <li>
+          <a class="title" href={Notes.Show { id: note.id }}>{note.title}</a>
+          <al-button variant="ghost" size="sm" click={Delete { id: note.id }}>Delete</al-button>
+        </li>}
+    </ul>
+
+    <al-button variant="outline" click={App.Logout}>Log out</al-button>
   </Layout.Page>
 
 test "a visitor is sent to log in" = {
app/Notes/Index.alder:78:16: warning[W0002]: hole ?expected has type Bool
    Task.succeed ?expected
                 ^^^^^^^^^
[exit 0]
$ alder new test Notes.Index "the list shows only my notes"
edited app/Notes/Index.alder
--- a/app/Notes/Index.alder
+++ b/app/Notes/Index.alder
@@ -95,3 +95,7 @@
   let rows <- Db.from Note |> Db.count
   Task.succeed ({ answer: r.answer, rows: rows } == { answer: FormRejected [{ field: "title", message: "must not be empty" }], rows: 0 })
 }
+
+test "the list shows only my notes" = {
+  ?test
+}
app/Notes/Index.alder:78:16: warning[W0002]: hole ?expected has type Bool
    Task.succeed ?expected
                 ^^^^^^^^^
app/Notes/Index.alder:100:3: warning[W0002]: hole ?test has type Task Bool
    ?test
    ^^^^^
[exit 0]
$ alder fill 'Notes.Index."the list shows only my notes"' - <<'EOF'
{
  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
  let other <- Db.insert User { email: Text.Email "other@example.com", name: Text.NonEmpty "Other", passwordHash: "" }
  let _ <- Db.insert Note { title: Text.NonEmpty "mine", body: "", owner: me }
  let _ <- Db.insert Note { title: Text.NonEmpty "theirs", body: "", owner: other }
  let _ <- Test.login { user: Just me }
  let page <- Test.get (Notes.Index {})
  Task.succeed (Test.texts "#notes a.title" page.doc == ["mine"])
}
EOF
edited app/Notes/Index.alder
--- a/app/Notes/Index.alder
+++ b/app/Notes/Index.alder
@@ -97,5 +97,11 @@
 }
 
 test "the list shows only my notes" = {
-  ?test
+  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
+  let other <- Db.insert User { email: Text.Email "other@example.com", name: Text.NonEmpty "Other", passwordHash: "" }
+  let _ <- Db.insert Note { title: Text.NonEmpty "mine", body: "", owner: me }
+  let _ <- Db.insert Note { title: Text.NonEmpty "theirs", body: "", owner: other }
+  let _ <- Test.login { user: Just me }
+  let page <- Test.get (Notes.Index {})
+  Task.succeed (Test.texts "#notes a.title" page.doc == ["mine"])
 }
app/Notes/Index.alder:78:16: warning[W0002]: hole ?expected has type Bool
    Task.succeed ?expected
                 ^^^^^^^^^
[exit 0]
$ alder new test Notes.Index "a stranger cannot delete a note"
edited app/Notes/Index.alder
--- a/app/Notes/Index.alder
+++ b/app/Notes/Index.alder
@@ -105,3 +105,7 @@
   let page <- Test.get (Notes.Index {})
   Task.succeed (Test.texts "#notes a.title" page.doc == ["mine"])
 }
+
+test "a stranger cannot delete a note" = {
+  ?test
+}
app/Notes/Index.alder:78:16: warning[W0002]: hole ?expected has type Bool
    Task.succeed ?expected
                 ^^^^^^^^^
app/Notes/Index.alder:110:3: warning[W0002]: hole ?test has type Task Bool
    ?test
    ^^^^^
[exit 0]
$ alder fill 'Notes.Index."a stranger cannot delete a note"' - <<'EOF'
{
  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
  let stranger <- Db.insert User { email: Text.Email "other@example.com", name: Text.NonEmpty "Other", passwordHash: "" }
  let note1 <- Db.insert Note { title: Text.NonEmpty "Milk", body: "2 litres", owner: me }
  let _ <- Test.login { user: Just stranger }
  let r <- Test.run (update (Delete { id: note1 }) { notes: [] } { user: stranger })
  let after <- Db.get Note note1
  Task.succeed ({ value: r.value, kept: Maybe.isJust after } == { value: Err "not found", kept: True })
}
EOF
edited app/Notes/Index.alder
--- a/app/Notes/Index.alder
+++ b/app/Notes/Index.alder
@@ -107,5 +107,11 @@
 }
 
 test "a stranger cannot delete a note" = {
-  ?test
+  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
+  let stranger <- Db.insert User { email: Text.Email "other@example.com", name: Text.NonEmpty "Other", passwordHash: "" }
+  let note1 <- Db.insert Note { title: Text.NonEmpty "Milk", body: "2 litres", owner: me }
+  let _ <- Test.login { user: Just stranger }
+  let r <- Test.run (update (Delete { id: note1 }) { notes: [] } { user: stranger })
+  let after <- Db.get Note note1
+  Task.succeed ({ value: r.value, kept: Maybe.isJust after } == { value: Err "not found", kept: True })
 }
app/Notes/Index.alder:78:16: warning[W0002]: hole ?expected has type Bool
    Task.succeed ?expected
                 ^^^^^^^^^
[exit 0]
$ alder new test Notes.Index "a missing id is not found"
edited app/Notes/Index.alder
--- a/app/Notes/Index.alder
+++ b/app/Notes/Index.alder
@@ -115,3 +115,7 @@
   let after <- Db.get Note note1
   Task.succeed ({ value: r.value, kept: Maybe.isJust after } == { value: Err "not found", kept: True })
 }
+
+test "a missing id is not found" = {
+  ?test
+}
app/Notes/Index.alder:78:16: warning[W0002]: hole ?expected has type Bool
    Task.succeed ?expected
                 ^^^^^^^^^
app/Notes/Index.alder:120:3: warning[W0002]: hole ?test has type Task Bool
    ?test
    ^^^^^
[exit 0]
$ alder fill 'Notes.Index."a missing id is not found"' - <<'EOF'
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
@@ -117,5 +117,8 @@
 }
 
 test "a missing id is not found" = {
-  ?test
+  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
+  let _ <- Test.login { user: Just me }
+  let r <- Test.run (update (Delete { id: Note.NoteId "missing" }) { notes: [] } { user: me })
+  Task.succeed (r.value == Err "not found")
 }
app/Notes/Index.alder:78:16: warning[W0002]: hole ?expected has type Bool
    Task.succeed ?expected
                 ^^^^^^^^^
[exit 0]
$ alder fill 'Notes.Index."pressing Delete"' - --hole ?expected <<'EOF'
r.answer == Redirected (Notes.Index {}) && after == Nothing
EOF
edited app/Notes/Index.alder
--- a/app/Notes/Index.alder
+++ b/app/Notes/Index.alder
@@ -65,9 +65,6 @@
   Task.succeed (r.answer == Redirected (Auth.Login {}))
 }
 
--- Fill ?expected with what pressing Delete must do, so that a test fails when the arm
--- does something else: its answer (`r.answer`: Rerendered, Redirected …) and what it
--- changed (`after`, the row read again), as `{ answer: r.answer, … } == { answer: Rerendered, … }`.
 test "pressing Delete" = {
   let me <- Db.insert User { email: Text.Email "email-1@example.com", name: Text.NonEmpty "name-1", passwordHash: "passwordhash-1" }
   let _ <- Test.login { user: Just me }
@@ -75,7 +72,7 @@
   let _ <- Test.get (Notes.Index {})
   let r <- Test.click (Delete { id: note1 })
   let after <- Db.get Note note1
-  Task.succeed ?expected
+  Task.succeed (r.answer == Redirected (Notes.Index {}) && after == Nothing)
 }
 
 test "submitting NewNote adds a row" = {
[exit 0]
$ alder fill --decl Auth.Login.update - <<'EOF'
match msg {
  Submit p -> {
    let found <- Password.verify { email: p.form.email, password: p.form.password }
    match found {
      Just id -> {
        let _ <- Session.put (\s -> { s | user: Just id })
        Task.succeed (Redirect (Notes.Index {}))
      },
      Nothing -> Task.succeed (Reject "Email or password is incorrect."),
    }
  },
}
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
$ alder fill 'Auth.Login."logging in through the form"' - --hole ?afterLogin <<'EOF'
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
```

The harness ran `alder check`, `alder build` and `alder test` on the app: all pass.
