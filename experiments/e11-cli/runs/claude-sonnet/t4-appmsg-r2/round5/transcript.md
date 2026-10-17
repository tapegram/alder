### Round 5

```text
$ alder fill Auth.Login.update.Submit - --hole ?afterLogin <<'EOF'
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
$ alder fill --decl Notes.Index.view - <<'EOF'
<Layout.Page title="Notes">
  <h1>Notes</h1>
</Layout.Page>
EOF
edited app/Notes/Index.alder
--- a/app/Notes/Index.alder
+++ b/app/Notes/Index.alder
@@ -14,7 +14,10 @@
 }
 
 view : Model -> Session -> Html Msg
-view model session = Layout.shell { title: "Notes", theme: session.theme, children: [<h1>Notes</h1>] }
+view model session =
+  <Layout.Page title="Notes">
+    <h1>Notes</h1>
+  </Layout.Page>
 
 test "a visitor is sent to log in" = {
   let r <- Test.open (Notes.Index {})
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
 
@@ -13,13 +14,56 @@
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
 view model session =
   <Layout.Page title="Notes">
     <h1>Notes</h1>
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
$ alder fill --list
[exit 0]
```

The harness ran `alder check`, `alder build` and `alder test` on the app: all pass.
