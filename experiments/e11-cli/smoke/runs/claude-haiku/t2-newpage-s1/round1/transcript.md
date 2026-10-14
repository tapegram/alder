### Round 1

```text
$ alder fill Profile.load - --hole ?load <<'EOF'
{
  let found <- Db.get User session.user
  let todos <- Db.mine Todo |> Db.all
  match found {
    Just me -> Query.succeed {
      name: me.name,
      email: me.email,
      total: List.length todos,
      done: List.length (List.filter (\t -> t.done) todos),
    },
    Nothing -> Query.notFound,
  }
}
EOF
edited app/Profile.alder
--- a/app/Profile.alder
+++ b/app/Profile.alder
@@ -1,11 +1,26 @@
 module Profile
 
+import Todo
+import User
+
 page "/profile" { } requires user
 
 type Model = { name: Text.NonEmpty, email: Text.Email, total: Int, done: Int }
 
 load : Params -> Session -> Query Model
-load params session = ?load
+load params session = {
+  let found <- Db.get User session.user
+  let todos <- Db.mine Todo |> Db.all
+  match found {
+    Just me -> Query.succeed {
+      name: me.name,
+      email: me.email,
+      total: List.length todos,
+      done: List.length (List.filter (\t -> t.done) todos),
+    },
+    Nothing -> Query.notFound,
+  }
+}
 
 view : Model -> Session -> Html Msg
 view model session = ?view
app/Profile.alder:26:22: warning[W0002]: hole ?view has type Html Profile.Msg
  view model session = ?view
                       ^^^^^
added import User to app/Profile.alder
added import Todo to app/Profile.alder
[exit 0]
$ alder fill Profile.view - --hole ?view <<'EOF'
<Layout.Page title="Profile">
    <h1>Profile</h1>
  </Layout.Page>
EOF
edited app/Profile.alder
--- a/app/Profile.alder
+++ b/app/Profile.alder
@@ -1,5 +1,6 @@
 module Profile
 
+import Layout
 import Todo
 import User
 
@@ -23,7 +24,9 @@
 }
 
 view : Model -> Session -> Html Msg
-view model session = ?view
+view model session = <Layout.Page title="Profile">
+    <h1>Profile</h1>
+  </Layout.Page>
 
 test "a visitor is sent to log in" = {
   let r <- Test.open (Profile {})
added import Layout to app/Profile.alder
[exit 0]
$ alder new form Profile Rename name:Text.NonEmpty
edited app/Profile.alder
--- a/app/Profile.alder
+++ b/app/Profile.alder
@@ -23,12 +23,48 @@
   }
 }
 
+form Rename = {
+  name: Text.NonEmpty,
+}
+
+msg SubmitRename form Rename
+
+update : Msg -> Model -> Session -> Task Nav
+update msg model session = match msg {
+  SubmitRename p -> ?submitRename,
+}
+
 view : Model -> Session -> Html Msg
 view model session = <Layout.Page title="Profile">
     <h1>Profile</h1>
+
+    <al-card>
+      <h2 slot="header">Rename</h2>
+      <form submit={SubmitRename}>
+        <al-input field="name" label="Name" />
+        <al-alert variant="destructive"><Errors form /></al-alert>
+        <al-button type="submit">Rename</al-button>
+      </form>
+    </al-card>
   </Layout.Page>
 
 test "a visitor is sent to log in" = {
   let r <- Test.open (Profile {})
   Task.succeed (r.answer == Redirected (Auth.Login {}))
 }
+
+test "submitting Rename" = {
+  let me <- Db.insert User { email: Text.Email "email-1@example.com", name: Text.NonEmpty "name-1", passwordHash: "passwordhash-1" }
+  let _ <- Test.login { user: Just me }
+  let _ <- Test.get (Profile {})
+  let r <- Test.submit Rename { name: Text.NonEmpty "name-1" }
+  Task.succeed (r.answer == ?expected)
+}
+
+test "Rename refuses an empty name" = {
+  let me <- Db.insert User { email: Text.Email "email-1@example.com", name: Text.NonEmpty "name-1", passwordHash: "passwordhash-1" }
+  let _ <- Test.login { user: Just me }
+  let _ <- Test.get (Profile {})
+  let r <- Test.submitRaw Rename [{ name: "name", value: "" }]
+  Task.succeed (r.answer == FormRejected [{ field: "name", message: "must not be empty" }])
+}
app/Profile.alder:34:21: warning[W0002]: hole ?submitRename has type Task Nav
    SubmitRename p -> ?submitRename,
                      ^^^^^^^^^^^^^
app/Profile.alder:61:29: warning[W0002]: hole ?expected has type Answer
    Task.succeed (r.answer == ?expected)
                              ^^^^^^^^^
[exit 0]
$ alder fill --decl Profile.view - <<'EOF'
<Layout.Page title="Profile">
    <h1>Profile</h1>

    <al-card>
      <dl>
        <dt>Name</dt>
        <dd>{model.name}</dd>
        <dt>Email</dt>
        <dd>{model.email}</dd>
        <dt>Todos</dt>
        <dd id="total">{Text.fromInt model.total}</dd>
        <dt>Done</dt>
        <dd id="done">{Text.fromInt model.done}</dd>
      </dl>
    </al-card>

    <al-card>
      <form submit={SubmitRename}>
        <Errors form />
        <al-input field="name" label="Name" value={model.name} />
        <Errors field="name" />
        <al-button type="submit">Save name</al-button>
      </form>
    </al-card>

    <al-button variant="outline" click={App.Logout}>Log out</al-button>
    <a href={Todos.Index {}}>Back to todos</a>
  </Layout.Page>
EOF
app/Profile.alder:34:21: warning[W0002]: hole ?submitRename has type Task Nav
    SubmitRename p -> ?submitRename,
                      ^^^^^^^^^^^^^
app/Profile.alder:59:11: error[E0028]: `<al-input field="name">` already shows the errors of `name`; remove `<Errors field="name" />`
            <Errors field="name" />
            ^^^^^^^^^^^^^^^^^^^^^^
app/Profile.alder:78:29: warning[W0002]: hole ?expected has type Answer
    Task.succeed (r.answer == ?expected)
                              ^^^^^^^^^
alder fill: the result does not check (the errors above): nothing was written
[exit 1]
```

The harness stopped at line 26 of your script: `alder fill --decl Profile.view - <<'EOF'`: it exited 1. The commands after it were not run.

Your script stopped at command 4; the app is as the commands before it left it; continue.

The program type-checks but `alder build` refused it; fix the cause below.

```text
app/Profile.alder:34:21: warning[W0002]: hole ?submitRename has type Task Nav
    SubmitRename p -> ?submitRename,
                      ^^^^^^^^^^^^^
app/Profile.alder:61:29: warning[W0002]: hole ?expected has type Answer
    Task.succeed (r.answer == ?expected)
                              ^^^^^^^^^
alder build: the program has 2 typed hole(s); fill them before building
```
