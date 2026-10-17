### Round 3

```text
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

  <al-card>
    <form submit={Add}>
      <al-input field="title" label="Title" />
      <al-textarea field="body" label="Body" />
      <Errors form />
      <al-button type="submit">New note</al-button>
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
@@ -46,22 +46,30 @@
 }
 
 view : Model -> Session -> Html Msg
-view model session = <Layout.Page title="Notes">
-  <h1>Notes</h1>
+view model session =
+  <Layout.Page title="Notes">
+    <h1>Notes</h1>
 
-  <al-card>
-    <h2 slot="header">New note</h2>
-    <form submit={Add}>
-      <al-input field="title" label="Title" />
-      <al-textarea field="body" label="Body" />
-      <al-alert variant="destructive"><Errors form /></al-alert>
-      <al-button type="submit">New note</al-button>
-    </form>
-  </al-card>
+    <al-card>
+      <form submit={Add}>
+        <al-input field="title" label="Title" />
+        <al-textarea field="body" label="Body" />
+        <Errors form />
+        <al-button type="submit">New note</al-button>
+      </form>
+    </al-card>
 
-  <al-button click={Delete { id: ?deleteId }}>Delete</al-button>
-</Layout.Page>
+    <ul id="notes">
+      {for note in model.notes ->
+        <li>
+          <a class="title" href={Notes.Show { id: note.id }}>{note.title}</a>
+          <al-button variant="ghost" size="sm" click={Delete { id: note.id }}>Delete</al-button>
+        </li>}
+    </ul>
 
+    <al-button variant="outline" click={App.Logout}>Log out</al-button>
+  </Layout.Page>
+
 test "a visitor is sent to log in" = {
   let r <- Test.open (Notes.Index {})
   Task.succeed (r.answer == Redirected (Auth.Login {}))
app/Notes/Index.alder:83:29: warning[W0002]: hole ?expected has type Answer
    Task.succeed (r.answer == ?expected)
                              ^^^^^^^^^
app/Notes/Index.alder:104:16: warning[W0002]: hole ?expected has type Bool
    Task.succeed ?expected
                 ^^^^^^^^^
[exit 0]
$ alder fill --decl Notes.Show.load - <<'EOF'
\params session -> {
  let found <- Db.get Note params.id
  let mine = Maybe.andThen (\n -> if n.owner == session.user then Just n else Nothing) found
  match mine {
    Just row -> Query.succeed { row: row },
    Nothing -> Query.notFound,
  }
}
EOF
alder fill: the parameters `params session` are bound already: send the body without `\params session ->`
[exit 1]
```

The harness stopped at line 33 of your script: `alder fill --decl Notes.Show.load - <<'EOF'`: it exited 1. The commands after it were not run.

Your script stopped at command 3; the app is as the commands before it left it; continue.

The program type-checks but `alder build` refused it; fix the cause below.

```text
app/Notes/Index.alder:83:29: warning[W0002]: hole ?expected has type Answer
    Task.succeed (r.answer == ?expected)
                              ^^^^^^^^^
app/Notes/Index.alder:104:16: warning[W0002]: hole ?expected has type Bool
    Task.succeed ?expected
                 ^^^^^^^^^
alder build: the program has 2 typed hole(s); fill them before building
```
