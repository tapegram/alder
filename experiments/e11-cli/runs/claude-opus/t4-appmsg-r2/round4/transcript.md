### Round 4

```text
$ alder new page Notes.Show /notes/{id} --param id:Note.NoteId --blank --requires user
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
$ alder fill --decl Notes.Show.view - <<'EOF'
<Layout.Page title="Note">
  <h1>Note</h1>
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
 
@@ -14,7 +15,10 @@
 }
 
 view : Model -> Session -> Html Msg
-view model session = ?view
+view model session =
+  <Layout.Page title="Note">
+    <h1>Note</h1>
+  </Layout.Page>
 
 test "a visitor is sent to log in" = {
   let me <- Db.insert User { email: Text.Email "email-1@example.com", name: Text.NonEmpty "name-1", passwordHash: "passwordhash-1" }
added import Layout to app/Notes/Show.alder
[exit 0]
$ alder new form Notes.Show EditNote title:Text.NonEmpty body:Text --msg Save
edited app/Notes/Show.alder
--- a/app/Notes/Show.alder
+++ b/app/Notes/Show.alder
@@ -14,10 +14,35 @@
   Query.succeed { note: note }
 }
 
+form EditNote = {
+  title: Text.NonEmpty,
+  body: Text,
+}
+
+msg Save form EditNote
+
+update : Msg -> Model -> Session -> Task Nav
+update msg model session = match msg {
+  Save p -> {
+    let _ <- Db.insertMine Note { title: p.form.title, body: p.form.body }
+    Task.succeed Rerender
+  },
+}
+
 view : Model -> Session -> Html Msg
 view model session =
   <Layout.Page title="Note">
     <h1>Note</h1>
+
+    <al-card>
+      <h2 slot="header">Edit note</h2>
+      <form submit={Save}>
+        <al-input field="title" label="Title" />
+        <al-textarea field="body" label="Body" />
+        <al-alert variant="destructive"><Errors form /></al-alert>
+        <al-button type="submit">Edit note</al-button>
+      </form>
+    </al-card>
   </Layout.Page>
 
 test "a visitor is sent to log in" = {
@@ -26,3 +51,23 @@
   let r <- Test.open (Notes.Show { id: note1 })
   Task.succeed (r.answer == Redirected (Auth.Login {}))
 }
+
+test "submitting EditNote adds a row" = {
+  let me <- Db.insert User { email: Text.Email "email-1@example.com", name: Text.NonEmpty "name-1", passwordHash: "passwordhash-1" }
+  let _ <- Test.login { user: Just me }
+  let note1 <- Db.insert Note { title: Text.NonEmpty "title-1", body: "body-1", owner: me }
+  let _ <- Test.get (Notes.Show { id: note1 })
+  let r <- Test.submit EditNote { title: Text.NonEmpty "title-1", body: "body-1" }
+  let rows <- Db.from Note |> Db.count
+  Task.succeed ({ answer: r.answer, rows: rows } == { answer: Rerendered, rows: 2 })
+}
+
+test "EditNote refuses an empty title" = {
+  let me <- Db.insert User { email: Text.Email "email-1@example.com", name: Text.NonEmpty "name-1", passwordHash: "passwordhash-1" }
+  let _ <- Test.login { user: Just me }
+  let note1 <- Db.insert Note { title: Text.NonEmpty "title-1", body: "body-1", owner: me }
+  let _ <- Test.get (Notes.Show { id: note1 })
+  let r <- Test.submitRaw EditNote [{ name: "title", value: "" }, { name: "body", value: "x" }]
+  let rows <- Db.from Note |> Db.count
+  Task.succeed ({ answer: r.answer, rows: rows } == { answer: FormRejected [{ field: "title", message: "must not be empty" }], rows: 1 })
+}
[exit 0]
$ alder new msg Notes.Show Delete
edited app/Notes/Show.alder
--- a/app/Notes/Show.alder
+++ b/app/Notes/Show.alder
@@ -21,12 +21,15 @@
 
 msg Save form EditNote
 
+msg Delete
+
 update : Msg -> Model -> Session -> Task Nav
 update msg model session = match msg {
   Save p -> {
     let _ <- Db.insertMine Note { title: p.form.title, body: p.form.body }
     Task.succeed Rerender
   },
+  Delete p -> ?delete,
 }
 
 view : Model -> Session -> Html Msg
@@ -43,6 +46,8 @@
         <al-button type="submit">Edit note</al-button>
       </form>
     </al-card>
+
+    <al-button click={Delete}>Delete</al-button>
   </Layout.Page>
 
 test "a visitor is sent to log in" = {
@@ -71,3 +76,18 @@
   let rows <- Db.from Note |> Db.count
   Task.succeed ({ answer: r.answer, rows: rows } == { answer: FormRejected [{ field: "title", message: "must not be empty" }], rows: 1 })
 }
+
+-- Fill ?expected with what pressing Delete must do, so that a test fails when the arm
+-- does something else: its answer (`r.answer`: Rerendered, Redirected …) and what it
+-- changed (`after`: the page opened again, `after.answer` Shown, or PageMissing when the arm
+-- deleted the page's row, and `after.doc` its HTML), as
+-- `{ answer: r.answer, … } == { answer: Rerendered, … }`.
+test "pressing Delete" = {
+  let me <- Db.insert User { email: Text.Email "email-1@example.com", name: Text.NonEmpty "name-1", passwordHash: "passwordhash-1" }
+  let _ <- Test.login { user: Just me }
+  let note1 <- Db.insert Note { title: Text.NonEmpty "title-1", body: "body-1", owner: me }
+  let _ <- Test.get (Notes.Show { id: note1 })
+  let r <- Test.click Delete
+  let after <- Test.open (Notes.Show { id: note1 })
+  Task.succeed ?expected
+}
app/Notes/Show.alder:32:15: warning[W0002]: hole ?delete has type Task Nav
    Delete p -> ?delete,
                ^^^^^^^
app/Notes/Show.alder:92:16: warning[W0002]: hole ?expected has type Bool
    Task.succeed ?expected
                 ^^^^^^^^^
[exit 0]
$ alder fill --decl Notes.Show.update - <<'EOF'
match msg {
  Save p -> {
    let _ <- Db.updateMine Note model.note.id (\n -> { n | title: p.form.title, body: p.form.body })
    Task.succeed (Redirect (Notes.Show { id: model.note.id }))
  },
  Delete p -> {
    let _ <- Db.deleteMine Note model.note.id
    Task.succeed (Redirect (Notes.Index {}))
  },
}
EOF
edited app/Notes/Show.alder
--- a/app/Notes/Show.alder
+++ b/app/Notes/Show.alder
@@ -26,10 +26,13 @@
 update : Msg -> Model -> Session -> Task Nav
 update msg model session = match msg {
   Save p -> {
-    let _ <- Db.insertMine Note { title: p.form.title, body: p.form.body }
-    Task.succeed Rerender
+    let _ <- Db.updateMine Note model.note.id (\n -> { n | title: p.form.title, body: p.form.body })
+    Task.succeed (Redirect (Notes.Show { id: model.note.id }))
   },
-  Delete p -> ?delete,
+  Delete p -> {
+    let _ <- Db.deleteMine Note model.note.id
+    Task.succeed (Redirect (Notes.Index {}))
+  },
 }
 
 view : Model -> Session -> Html Msg
app/Notes/Show.alder:95:16: warning[W0002]: hole ?expected has type Bool
    Task.succeed ?expected
                 ^^^^^^^^^
[exit 0]
$ alder fill --decl Notes.Show.view - <<'EOF'
Layout.shell {
  title: model.note.title,
  dark: Theme.isDark session.theme,
  children: [
    <h1 id="title">{model.note.title}</h1>,
    <p id="body">{model.note.body}</p>,
    <form submit={Save}>
      <Errors form />
      <al-input field="title" label="Title" value={model.note.title} />
      <al-textarea field="body" label="Body" value={model.note.body} />
      <al-button type="submit">Save</al-button>
    </form>,
    <button click={Delete}>Delete</button>
  ]
}
EOF
edited app/Notes/Show.alder
--- a/app/Notes/Show.alder
+++ b/app/Notes/Show.alder
@@ -2,6 +2,7 @@
 
 import Layout
 import Note
+import Theme
 import User
 
 page "/notes/{id}" { id: Note.NoteId } requires user
@@ -37,21 +38,21 @@
 
 view : Model -> Session -> Html Msg
 view model session =
-  <Layout.Page title="Note">
-    <h1>Note</h1>
-
-    <al-card>
-      <h2 slot="header">Edit note</h2>
+  Layout.shell {
+    title: model.note.title,
+    dark: Theme.isDark session.theme,
+    children: [
+      <h1 id="title">{model.note.title}</h1>,
+      <p id="body">{model.note.body}</p>,
       <form submit={Save}>
-        <al-input field="title" label="Title" />
-        <al-textarea field="body" label="Body" />
-        <al-alert variant="destructive"><Errors form /></al-alert>
-        <al-button type="submit">Edit note</al-button>
-      </form>
-    </al-card>
-
-    <al-button click={Delete}>Delete</al-button>
-  </Layout.Page>
+        <Errors form />
+        <al-input field="title" label="Title" value={model.note.title} />
+        <al-textarea field="body" label="Body" value={model.note.body} />
+        <al-button type="submit">Save</al-button>
+      </form>,
+      <button click={Delete}>Delete</button>
+    ]
+  }
 
 test "a visitor is sent to log in" = {
   let me <- Db.insert User { email: Text.Email "email-1@example.com", name: Text.NonEmpty "name-1", passwordHash: "passwordhash-1" }
app/Notes/Show.alder:96:16: warning[W0002]: hole ?expected has type Bool
    Task.succeed ?expected
                 ^^^^^^^^^
added import Theme to app/Notes/Show.alder
[exit 0]
$ alder new test Notes.Show "another user's note is not found"
edited app/Notes/Show.alder
--- a/app/Notes/Show.alder
+++ b/app/Notes/Show.alder
@@ -95,3 +95,7 @@
   let after <- Test.open (Notes.Show { id: note1 })
   Task.succeed ?expected
 }
+
+test "another user's note is not found" = {
+  ?test
+}
app/Notes/Show.alder:96:16: warning[W0002]: hole ?expected has type Bool
    Task.succeed ?expected
                 ^^^^^^^^^
app/Notes/Show.alder:100:3: warning[W0002]: hole ?test has type Task Bool
    ?test
    ^^^^^
[exit 0]
$ alder fill 'Notes.Show."another user'"'"'s note is not found"' - <<'EOF'
{
  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
  let other <- Db.insert User { email: Text.Email "other@example.com", name: Text.NonEmpty "Other", passwordHash: "" }
  let id <- Db.insert Note { title: Text.NonEmpty "Secret", body: "x", owner: other }
  let _ <- Test.login { user: Just me }
  let r <- Test.open (Notes.Show { id: id })
  Task.succeed (r.answer == PageMissing)
}
EOF
edited app/Notes/Show.alder
--- a/app/Notes/Show.alder
+++ b/app/Notes/Show.alder
@@ -97,5 +97,10 @@
 }
 
 test "another user's note is not found" = {
-  ?test
+  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
+  let other <- Db.insert User { email: Text.Email "other@example.com", name: Text.NonEmpty "Other", passwordHash: "" }
+  let id <- Db.insert Note { title: Text.NonEmpty "Secret", body: "x", owner: other }
+  let _ <- Test.login { user: Just me }
+  let r <- Test.open (Notes.Show { id: id })
+  Task.succeed (r.answer == PageMissing)
 }
app/Notes/Show.alder:96:16: warning[W0002]: hole ?expected has type Bool
    Task.succeed ?expected
                 ^^^^^^^^^
[exit 0]
$ alder fill --decl Notes.Index.view - <<'EOF'
Layout.shell {
  title: "Notes",
  dark: Theme.isDark session.theme,
  children: [
    <h1>Notes</h1>,
    <form submit={Add}>
      <Errors form />
      <al-input field="title" label="Title" />
      <al-textarea field="body" label="Body" />
      <al-button type="submit">Add note</al-button>
    </form>,
    <ul id="notes">
      {for n in model.notes -> <li><a class="title" href={Notes.Show { id: n.id }}>{n.title}</a></li>}
    </ul>
  ]
}
EOF
edited app/Notes/Index.alder
--- a/app/Notes/Index.alder
+++ b/app/Notes/Index.alder
@@ -32,19 +32,22 @@
 
 view : Model -> Session -> Html Msg
 view model session =
-  <Layout.Page title="Notes">
-    <h1>Notes</h1>
-
-    <al-card>
-      <h2 slot="header">New note</h2>
+  Layout.shell {
+    title: "Notes",
+    dark: Theme.isDark session.theme,
+    children: [
+      <h1>Notes</h1>,
       <form submit={Add}>
+        <Errors form />
         <al-input field="title" label="Title" />
         <al-textarea field="body" label="Body" />
-        <al-alert variant="destructive"><Errors form /></al-alert>
-        <al-button type="submit">New note</al-button>
-      </form>
-    </al-card>
-  </Layout.Page>
+        <al-button type="submit">Add note</al-button>
+      </form>,
+      <ul id="notes">
+        {for n in model.notes -> <li><a class="title" href={Notes.Show { id: n.id }}>{n.title}</a></li>}
+      </ul>
+    ]
+  }
 
 test "a visitor is sent to log in" = {
   let r <- Test.open (Notes.Index {})
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
$ alder fill --list
Auth.Login "logging in through the form" ?afterLogin : Page
Notes.Show "pressing Delete" ?expected : Bool
[exit 0]
$ alder fill --decl Auth.Login.view - <<'EOF'
Layout.shell {
  title: "Log in",
  dark: Theme.isDark session.theme,
  children: [
    <h1>Log in</h1>,
    <form submit={Submit}>
      <Errors form />
      <al-input field="email" label="Email" />
      <al-input field="password" label="Password" />
      <al-button type="submit">Log in</al-button>
    </form>
  ]
}
EOF
edited app/Auth/Login.alder
--- a/app/Auth/Login.alder
+++ b/app/Auth/Login.alder
@@ -1,6 +1,7 @@
 module Auth.Login
 
 import Layout
+import Theme
 import User
 
 -- Password checking is outside the language: a capability implemented in Go, by
@@ -53,21 +54,19 @@
 
 view : Model -> Session -> Html Msg
 view model session =
-  <Layout.Page title="Log in">
-    <h1>Log in</h1>
-    <al-card>
+  Layout.shell {
+    title: "Log in",
+    dark: Theme.isDark session.theme,
+    children: [
+      <h1>Log in</h1>,
       <form submit={Submit}>
-        <al-alert variant="destructive"><Errors form /></al-alert>
+        <Errors form />
         <al-input field="email" label="Email" />
         <al-input field="password" label="Password" />
         <al-button type="submit">Log in</al-button>
       </form>
-    </al-card>
-    {match session.user {
-      Just _ -> <al-button variant="outline" click={App.Logout}>Log out</al-button>,
-      Nothing -> Html.none,
-    }}
-  </Layout.Page>
+    ]
+  }
 
 -- Logging in through the form, with the fake: an unknown email is rejected with the form's
 -- message, and a known one puts the user in the session and goes to the page ?afterLogin names (fill it, here and in Submit's arm, once the page exists).
app/Auth/Login.alder:85:85: warning[W0002]: hole ?afterLogin has type Page
    Task.succeed ({ answer: r.answer, user: r.session.user } == { answer: Redirected (?afterLogin), user: Just me })
                                                                                      ^^^^^^^^^^^
added import Theme to app/Auth/Login.alder
[exit 0]
$ alder check
app/Auth/Login.alder:85:85: warning[W0002]: hole ?afterLogin has type Page
    Task.succeed ({ answer: r.answer, user: r.session.user } == { answer: Redirected (?afterLogin), user: Just me })
                                                                                      ^^^^^^^^^^^
app/Notes/Show.alder:96:16: warning[W0002]: hole ?expected has type Bool
    Task.succeed ?expected
                 ^^^^^^^^^
alder check: 2 typed holes left
[exit 0]
```

The program type-checks but `alder build` refused it; fix the cause below.

```text
app/Auth/Login.alder:85:85: warning[W0002]: hole ?afterLogin has type Page
    Task.succeed ({ answer: r.answer, user: r.session.user } == { answer: Redirected (?afterLogin), user: Just me })
                                                                                      ^^^^^^^^^^^
app/Notes/Show.alder:96:16: warning[W0002]: hole ?expected has type Bool
    Task.succeed ?expected
                 ^^^^^^^^^
alder build: the program has 2 typed hole(s); fill them before building
```
