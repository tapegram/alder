### Round 4

```text
$ alder fill --decl Notes.Show.load - <<'EOF'
{
  let found <- Db.get Note params.id
  let mine = Maybe.andThen (\n -> if n.owner == session.user then Just n else Nothing) found
  match mine {
    Just row -> Query.succeed { row: row },
    Nothing -> Query.notFound,
  }
}
EOF
edited app/Notes/Show.alder
--- a/app/Notes/Show.alder
+++ b/app/Notes/Show.alder
@@ -11,7 +11,8 @@
 load : Params -> Session -> Query Model
 load params session = {
   let found <- Db.get Note params.id
-  match found {
+  let mine = Maybe.andThen (\n -> if n.owner == session.user then Just n else Nothing) found
+  match mine {
     Just row -> Query.succeed { row: row },
     Nothing -> Query.notFound,
   }
[exit 0]
$ alder fill --decl Notes.Show.view - <<'EOF'
<Layout.Page title={model.row.title}>
  <h1 id="title">{model.row.title}</h1>
  <p id="body">{model.row.body}</p>
  <a href={Notes.Index {}}>Back</a>
</Layout.Page>
EOF
edited app/Notes/Show.alder
--- a/app/Notes/Show.alder
+++ b/app/Notes/Show.alder
@@ -20,23 +20,10 @@
 
 view : Model -> Session -> Html Msg
 view model session =
-  <Layout.Page title="Note">
-    <h1>Note</h1>
-
-    <al-card>
-      <dl>
-        <dt>Id</dt>
-        <dd>{model.row.id}</dd>
-        <dt>Title</dt>
-        <dd>{model.row.title}</dd>
-        <dt>Body</dt>
-        <dd>{model.row.body}</dd>
-        <dt>Owner</dt>
-        <dd>{model.row.owner}</dd>
-        <dt>Created</dt>
-        <dd>{Time.format "Jan 2, 2006" model.row.created}</dd>
-      </dl>
-    </al-card>
+  <Layout.Page title={model.row.title}>
+    <h1 id="title">{model.row.title}</h1>
+    <p id="body">{model.row.body}</p>
+    <a href={Notes.Index {}}>Back</a>
   </Layout.Page>
 
 test "the page shows the row" = {
[exit 0]
$ alder fill 'Notes.Index."submitting NewNote"' - --hole ?expected <<'EOF'
Redirected (Notes.Index {})
EOF
edited app/Notes/Index.alder
--- a/app/Notes/Index.alder
+++ b/app/Notes/Index.alder
@@ -80,7 +80,7 @@
   let _ <- Test.login { user: Just me }
   let _ <- Test.get (Notes.Index {})
   let r <- Test.submit NewNote { title: Text.NonEmpty "title-1", body: "body-1" }
-  Task.succeed (r.answer == ?expected)
+  Task.succeed (r.answer == Redirected (Notes.Index {}))
 }
 
 test "NewNote refuses an empty title" = {
app/Notes/Index.alder:104:16: warning[W0002]: hole ?expected has type Bool
    Task.succeed ?expected
                 ^^^^^^^^^
[exit 0]
$ alder fill 'Notes.Index."pressing Delete"' - --hole ?expected <<'EOF'
({ answer: r.answer, note: Maybe.map (\n -> n.title) after } == { answer: Redirected (Notes.Index {}), note: Nothing })
EOF
edited app/Notes/Index.alder
--- a/app/Notes/Index.alder
+++ b/app/Notes/Index.alder
@@ -91,9 +91,6 @@
   Task.succeed (r.answer == FormRejected [{ field: "title", message: "must not be empty" }])
 }
 
--- Fill ?expected with what pressing Delete must do, so that a test fails when the arm
--- does something else: its answer (`r.answer`: Rerendered, Redirected …) and what it
--- changed (`after`, the row read again), as `{ answer: r.answer, … } == { answer: Rerendered, … }`.
 test "pressing Delete" = {
   let me <- Db.insert User { email: Text.Email "email-1@example.com", name: Text.NonEmpty "name-1", passwordHash: "passwordhash-1" }
   let _ <- Test.login { user: Just me }
@@ -101,5 +98,5 @@
   let _ <- Test.get (Notes.Index {})
   let r <- Test.click (Delete { id: note1 })
   let after <- Db.get Note note1
-  Task.succeed ?expected
+  Task.succeed ({ answer: r.answer, note: Maybe.map (\n -> n.title) after } == { answer: Redirected (Notes.Index {}), note: Nothing })
 }
[exit 0]
$ alder new test Notes.Index "deleting another users note is not found"
edited app/Notes/Index.alder
--- a/app/Notes/Index.alder
+++ b/app/Notes/Index.alder
@@ -100,3 +100,7 @@
   let after <- Db.get Note note1
   Task.succeed ({ answer: r.answer, note: Maybe.map (\n -> n.title) after } == { answer: Redirected (Notes.Index {}), note: Nothing })
 }
+
+test "deleting another users note is not found" = {
+  ?test
+}
app/Notes/Index.alder:105:3: warning[W0002]: hole ?test has type Task Bool
    ?test
    ^^^^^
[exit 0]
$ alder fill 'Notes.Index."deleting another users note is not found"' - <<'EOF'
{
  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
  let other <- Db.insert User { email: Text.Email "other@example.com", name: Text.NonEmpty "Other", passwordHash: "" }
  let note1 <- Db.insert Note { title: Text.NonEmpty "Milk", body: "x", owner: other }
  let r <- Test.run (update (Delete { id: note1 }) { notes: [] } { user: me })
  let after <- Db.get Note note1
  Task.succeed ({ value: r.value, note: Maybe.map (\n -> n.title) after } == { value: Err "not found", note: Just (Text.NonEmpty "Milk") })
}
EOF
edited app/Notes/Index.alder
--- a/app/Notes/Index.alder
+++ b/app/Notes/Index.alder
@@ -102,5 +102,10 @@
 }
 
 test "deleting another users note is not found" = {
-  ?test
+  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
+  let other <- Db.insert User { email: Text.Email "other@example.com", name: Text.NonEmpty "Other", passwordHash: "" }
+  let note1 <- Db.insert Note { title: Text.NonEmpty "Milk", body: "x", owner: other }
+  let r <- Test.run (update (Delete { id: note1 }) { notes: [] } { user: me })
+  let after <- Db.get Note note1
+  Task.succeed ({ value: r.value, note: Maybe.map (\n -> n.title) after } == { value: Err "not found", note: Just (Text.NonEmpty "Milk") })
 }
[exit 0]
$ alder new test Notes.Index "deleting a missing note is not found"
edited app/Notes/Index.alder
--- a/app/Notes/Index.alder
+++ b/app/Notes/Index.alder
@@ -109,3 +109,7 @@
   let after <- Db.get Note note1
   Task.succeed ({ value: r.value, note: Maybe.map (\n -> n.title) after } == { value: Err "not found", note: Just (Text.NonEmpty "Milk") })
 }
+
+test "deleting a missing note is not found" = {
+  ?test
+}
app/Notes/Index.alder:114:3: warning[W0002]: hole ?test has type Task Bool
    ?test
    ^^^^^
[exit 0]
$ alder fill 'Notes.Index."deleting a missing note is not found"' - <<'EOF'
{
  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
  let r <- Test.run (update (Delete { id: Note.NoteId "missing" }) { notes: [] } { user: me })
  Task.succeed ({ value: r.value } == { value: Err "not found" })
}
EOF
edited app/Notes/Index.alder
--- a/app/Notes/Index.alder
+++ b/app/Notes/Index.alder
@@ -111,5 +111,7 @@
 }
 
 test "deleting a missing note is not found" = {
-  ?test
+  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
+  let r <- Test.run (update (Delete { id: Note.NoteId "missing" }) { notes: [] } { user: me })
+  Task.succeed ({ value: r.value } == { value: Err "not found" })
 }
[exit 0]
$ alder new test Notes.Index "the owner can delete a note"
edited app/Notes/Index.alder
--- a/app/Notes/Index.alder
+++ b/app/Notes/Index.alder
@@ -115,3 +115,7 @@
   let r <- Test.run (update (Delete { id: Note.NoteId "missing" }) { notes: [] } { user: me })
   Task.succeed ({ value: r.value } == { value: Err "not found" })
 }
+
+test "the owner can delete a note" = {
+  ?test
+}
app/Notes/Index.alder:120:3: warning[W0002]: hole ?test has type Task Bool
    ?test
    ^^^^^
[exit 0]
$ alder fill 'Notes.Index."the owner can delete a note"' - <<'EOF'
{
  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
  let note1 <- Db.insert Note { title: Text.NonEmpty "Milk", body: "x", owner: me }
  let r <- Test.run (update (Delete { id: note1 }) { notes: [] } { user: me })
  let after <- Db.get Note note1
  Task.succeed ({ value: r.value, note: Maybe.map (\n -> n.title) after } == { value: Ok (Redirect (Notes.Index {})), note: Nothing })
}
EOF
edited app/Notes/Index.alder
--- a/app/Notes/Index.alder
+++ b/app/Notes/Index.alder
@@ -117,5 +117,9 @@
 }
 
 test "the owner can delete a note" = {
-  ?test
+  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
+  let note1 <- Db.insert Note { title: Text.NonEmpty "Milk", body: "x", owner: me }
+  let r <- Test.run (update (Delete { id: note1 }) { notes: [] } { user: me })
+  let after <- Db.get Note note1
+  Task.succeed ({ value: r.value, note: Maybe.map (\n -> n.title) after } == { value: Ok (Redirect (Notes.Index {})), note: Nothing })
 }
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
ok   Notes.Index "deleting another users note is not found"
ok   Notes.Index "deleting a missing note is not found"
ok   Notes.Index "the owner can delete a note"
FAIL Notes.Show "the page shows the row"
  left:  { answer: Shown, title: False }
  right: { answer: Shown, title: True }
  trace: ["Db.insert User", "Test.login", "Db.insert Note", "Test.open Notes.Show", "Db.get Note"]
ok   Notes.Show "a missing row is not found"
ok   Notes.Show "a visitor is sent to log in"
14 passed, 1 failed
[exit 1]
```

The harness stopped at line 59 of your script: `alder test`: it exited 1. The commands after it were not run.

Your script stopped at command 11; the app is as the commands before it left it; continue.

The program builds but its tests fail; fix the program or the tests so every test passes.

```text
ok   App "Logout clears the user and goes to log in"
ok   Auth.Login "an unknown email is rejected"
ok   Auth.Login "logging in through the form"
ok   Auth.Login "a signed-in user logs out here"
ok   Note "a Note keeps its owner from being deleted"
ok   Notes.Index "a visitor is sent to log in"
ok   Notes.Index "submitting NewNote"
ok   Notes.Index "NewNote refuses an empty title"
ok   Notes.Index "pressing Delete"
ok   Notes.Index "deleting another users note is not found"
ok   Notes.Index "deleting a missing note is not found"
ok   Notes.Index "the owner can delete a note"
FAIL Notes.Show "the page shows the row"
  left:  { answer: Shown, title: False }
  right: { answer: Shown, title: True }
  trace: ["Db.insert User", "Test.login", "Db.insert Note", "Test.open Notes.Show", "Db.get Note"]
ok   Notes.Show "a missing row is not found"
ok   Notes.Show "a visitor is sent to log in"
14 passed, 1 failed
```
