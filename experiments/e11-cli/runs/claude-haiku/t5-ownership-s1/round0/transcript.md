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
$ alder new page Notes.Index / --list Note --requires user
wrote app/Notes/Index.alder
[exit 0]
$ alder new form Notes.Index NewNote title:Text.NonEmpty body:Text --msg Add
edited app/Notes/Index.alder
--- a/app/Notes/Index.alder
+++ b/app/Notes/Index.alder
@@ -29,6 +29,21 @@
   Query.succeed { rows: rows, page: page, pages: Int.max 1 ((count + perPage - 1) / perPage) }
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
@@ -56,6 +71,16 @@
       <span>Page {Text.fromInt model.page} of {Text.fromInt model.pages}</span>
       {if model.page < model.pages then <a slot="next" href={Notes.Index { page: model.page + 1 }} rel="next">Next</a> else <span slot="next" aria-disabled="true">Next</span>}
     </al-pagination>
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
 
 test "the list shows my rows, newest first" = {
@@ -84,3 +109,21 @@
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
@@ -36,12 +36,15 @@
 
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
@@ -81,6 +84,8 @@
         <al-button type="submit">New note</al-button>
       </form>
     </al-card>
+
+    <al-button click={Delete { id: ?deleteId }}>Delete</al-button>
   </Layout.Page>
 
 test "the list shows my rows, newest first" = {
@@ -127,3 +132,16 @@
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
app/Notes/Index.alder:47:15: warning[W0002]: hole ?delete has type Task Nav
    Delete p -> ?delete,
                ^^^^^^^
app/Notes/Index.alder:88:36: warning[W0002]: hole ?deleteId has type Note.NoteId
      <al-button click={Delete { id: ?deleteId }}>Delete</al-button>
                                     ^^^^^^^^^
app/Notes/Index.alder:146:16: warning[W0002]: hole ?expected has type Bool
    Task.succeed ?expected
                 ^^^^^^^^^
[exit 0]
$ alder fill --decl Notes.Index.Model - <<'EOF'
{ notes: List Note }
EOF
app/Notes/Index.alder:29:17: error[E0003]: record `Notes.Index.Model` is missing field(s): notes
    Query.succeed { rows: rows, page: page, pages: Int.max 1 ((count + perPage - 1) / perPage) }
                  ^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^
app/Notes/Index.alder:29:19: error[E0002]: no field `rows` on `Notes.Index.Model`
    Query.succeed { rows: rows, page: page, pages: Int.max 1 ((count + perPage - 1) / perPage) }
                    ^^^^^^^^^^
app/Notes/Index.alder:29:31: error[E0002]: no field `page` on `Notes.Index.Model`
    Query.succeed { rows: rows, page: page, pages: Int.max 1 ((count + perPage - 1) / perPage) }
                                ^^^^^^^^^^
app/Notes/Index.alder:29:43: error[E0002]: no field `pages` on `Notes.Index.Model`
    Query.succeed { rows: rows, page: page, pages: Int.max 1 ((count + perPage - 1) / perPage) }
                                            ^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^
app/Notes/Index.alder:47:15: warning[W0002]: hole ?delete has type Task Nav
    Delete p -> ?delete,
                ^^^^^^^
app/Notes/Index.alder:61:23: error[E0002]: no field `rows` on `Notes.Index.Model`
            {for row in model.rows ->
                        ^^^^^^^^^^
app/Notes/Index.alder:63:20: error[E0003]: cannot infer the record type of `row`; add a type annotation
                <td>{row.title}</td>
                     ^^^^^^^^^
app/Notes/Index.alder:64:20: error[E0003]: cannot infer the record type of `row`; add a type annotation
                <td>{row.body}</td>
                     ^^^^^^^^
app/Notes/Index.alder:65:20: error[E0003]: cannot infer the record type of `row`; add a type annotation
                <td>{row.owner}</td>
                     ^^^^^^^^^
app/Notes/Index.alder:66:46: error[E0003]: cannot infer the record type of `row`; add a type annotation
                <td>{Time.format "Jan 2, 2006" row.created}</td>
                                               ^^^^^^^^^^^
app/Notes/Index.alder:73:11: error[E0002]: no field `page` on `Notes.Index.Model`
        {if model.page > 1 then <a slot="prev" href={Notes.Index { page: model.page - 1 }} rel="prev">Previous</a> else <span slot="prev" aria-disabled="true">Previous</span>}
            ^^^^^^^^^^
app/Notes/Index.alder:73:72: error[E0002]: no field `page` on `Notes.Index.Model`
        {if model.page > 1 then <a slot="prev" href={Notes.Index { page: model.page - 1 }} rel="prev">Previous</a> else <span slot="prev" aria-disabled="true">Previous</span>}
                                                                         ^^^^^^^^^^
app/Notes/Index.alder:74:32: error[E0002]: no field `page` on `Notes.Index.Model`
        <span>Page {Text.fromInt model.page} of {Text.fromInt model.pages}</span>
                                 ^^^^^^^^^^
app/Notes/Index.alder:74:61: error[E0002]: no field `pages` on `Notes.Index.Model`
        <span>Page {Text.fromInt model.page} of {Text.fromInt model.pages}</span>
                                                              ^^^^^^^^^^^
app/Notes/Index.alder:75:11: error[E0002]: no field `page` on `Notes.Index.Model`
        {if model.page < model.pages then <a slot="next" href={Notes.Index { page: model.page + 1 }} rel="next">Next</a> else <span slot="next" aria-disabled="true">Next</span>}
            ^^^^^^^^^^
app/Notes/Index.alder:75:24: error[E0002]: no field `pages` on `Notes.Index.Model`
        {if model.page < model.pages then <a slot="next" href={Notes.Index { page: model.page + 1 }} rel="next">Next</a> else <span slot="next" aria-disabled="true">Next</span>}
                         ^^^^^^^^^^^
app/Notes/Index.alder:75:82: error[E0002]: no field `page` on `Notes.Index.Model`
        {if model.page < model.pages then <a slot="next" href={Notes.Index { page: model.page + 1 }} rel="next">Next</a> else <span slot="next" aria-disabled="true">Next</span>}
                                                                     
… (1616 more characters cut)
[exit 1]
```

The harness stopped at line 13 of your script: `alder fill --decl Notes.Index.Model - <<'EOF'`: it exited 1. The commands after it were not run.

Your script stopped at command 7; the app is as the commands before it left it; continue.

The program type-checks but `alder build` refused it; fix the cause below.

```text
app/Auth/Login.alder:47:33: warning[W0002]: hole ?afterLogin has type Page
          Task.succeed (Redirect (?afterLogin))
                                  ^^^^^^^^^^^
app/Auth/Login.alder:86:85: warning[W0002]: hole ?afterLogin has type Page
    Task.succeed ({ answer: r.answer, user: r.session.user } == { answer: Redirected (?afterLogin), user: Just me })
                                                                                      ^^^^^^^^^^^
app/Notes/Index.alder:47:15: warning[W0002]: hole ?delete has type Task Nav
    Delete p -> ?delete,
                ^^^^^^^
app/Notes/Index.alder:88:36: warning[W0002]: hole ?deleteId has type Note.NoteId
      <al-button click={Delete { id: ?deleteId }}>Delete</al-button>
                                     ^^^^^^^^^
app/Notes/Index.alder:146:16: warning[W0002]: hole ?expected has type Bool
    Task.succeed ?expected
                 ^^^^^^^^^
alder build: the program has 5 typed hole(s); fill them before building
```
