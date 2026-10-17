### Round 1

```text
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
$ alder fill --list
Auth.Login update.Submit ?afterLogin : Page
Auth.Login "logging in through the form" ?afterLogin : Page
Notes.Index update.Add ?add : Task Nav
Notes.Index update.Delete ?delete : Task Nav
Notes.Index view ?deleteId : Note.NoteId
Notes.Index "submitting NewNote" ?expected : Answer
Notes.Index "pressing Delete" ?expected : Bool
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
alder build: the program has 7 typed hole(s); fill them before building
```
