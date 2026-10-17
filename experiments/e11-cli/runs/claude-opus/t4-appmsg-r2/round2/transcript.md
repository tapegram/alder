### Round 2

```text
$ alder fill --decl Notes.Index.view - <<'EOF'
<Layout.Page title="Notes">
  <h1>Notes</h1>
</Layout.Page>
EOF
edited app/Notes/Index.alder
--- a/app/Notes/Index.alder
+++ b/app/Notes/Index.alder
@@ -15,7 +15,10 @@
 }
 
 view : Model -> Session -> Html Msg
-view model session = Layout.shell { title: "Notes", dark: Theme.isDark session.theme, children: [] }
+view model session =
+  <Layout.Page title="Notes">
+    <h1>Notes</h1>
+  </Layout.Page>
 
 test "a visitor is sent to log in" = {
   let r <- Test.open (Notes.Index {})
app/Notes/Index.alder:5:1: warning[W0001]: import `Theme` is unused
  import Theme
  ^^^^^^^^^^^^
[exit 0]
$ alder new form Notes.Index NewNote title:Text.NonEmpty body:Text --msg Add
edited app/Notes/Index.alder
--- a/app/Notes/Index.alder
+++ b/app/Notes/Index.alder
@@ -3,6 +3,7 @@
 import Layout
 import Note
 import Theme
+import User
 
 page "/" { } requires user
 
@@ -14,13 +15,56 @@
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
app/Notes/Index.alder:5:1: warning[W0001]: import `Theme` is unused
  import Theme
  ^^^^^^^^^^^^
[exit 0]
$ alder fill Notes.Index.update.Add - <<'EOF'
{
  let _ <- Db.insertMine Note { title: p.form.title, body: p.form.body }
  Task.succeed (Redirect (Notes.Index {}))
}
EOF
alder fill: `Notes.Index.update.Add` has no typed hole: a fill replaces a hole, and this arm has none; to rewrite it, give `alder fill --decl Notes.Index.update -` the whole `match msg { … }` (`alder fill --list` lists the app's holes)
[exit 1]
```

The harness stopped at line 8 of your script: `alder fill Notes.Index.update.Add - <<'EOF'`: it exited 1. The commands after it were not run.

Your script stopped at command 3; the app is as the commands before it left it; continue.

The program type-checks but `alder build` refused it; fix the cause below.

```text
app/App.alder:52:16: warning[W0002]: hole ?expected has type Bool
    Task.succeed ?expected
                 ^^^^^^^^^
app/Auth/Login.alder:47:33: warning[W0002]: hole ?afterLogin has type Page
          Task.succeed (Redirect (?afterLogin))
                                  ^^^^^^^^^^^
app/Auth/Login.alder:86:85: warning[W0002]: hole ?afterLogin has type Page
    Task.succeed ({ answer: r.answer, user: r.session.user } == { answer: Redirected (?afterLogin), user: Just me })
                                                                                      ^^^^^^^^^^^
app/Notes/Index.alder:5:1: warning[W0001]: import `Theme` is unused
  import Theme
  ^^^^^^^^^^^^
alder build: the program has 3 typed hole(s); fill them before building
```
