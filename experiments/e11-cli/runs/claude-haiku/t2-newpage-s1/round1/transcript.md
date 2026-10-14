### Round 1

```text
$ alder fill Profile.load - --hole ?load <<'EOF'
{
  let found <- Db.get User session.user
  let total <- Db.mine Todo |> Db.count
  let done <- Db.mine Todo |> Db.where (Todo.done.eq True) |> Db.count
  match found {
    Just u -> Query.succeed { name: u.name, email: u.email, total: total, done: done },
    Nothing -> Query.notFound,
  }
}
EOF
edited app/Profile.alder
--- a/app/Profile.alder
+++ b/app/Profile.alder
@@ -1,11 +1,22 @@
 module Profile
 
+import Todo
+import User
+
 page "/profile" { } requires user
 
 type Model = { name: Text, email: Text, total: Int, done: Int }
 
 load : Params -> Session -> Query Model
-load params session = ?load
+load params session = {
+  let found <- Db.get User session.user
+  let total <- Db.mine Todo |> Db.count
+  let done <- Db.mine Todo |> Db.where (Todo.done.eq True) |> Db.count
+  match found {
+    Just u -> Query.succeed { name: u.name, email: u.email, total: total, done: done },
+    Nothing -> Query.notFound,
+  }
+}
 
 view : Model -> Session -> Html Msg
 view model session = ?view
app/Profile.alder:22:22: warning[W0002]: hole ?view has type Html Profile.Msg
  view model session = ?view
                       ^^^^^
added import User to app/Profile.alder
added import Todo to app/Profile.alder
[exit 0]
$ alder fill Profile.view - --hole ?view <<'EOF'
<Layout.Page title="Profile">
  <h1>{model.name}</h1>
  <p>{model.email}</p>
  <p>{Text.fromInt model.total} todos, {Text.fromInt model.done} done</p>
  <al-button variant="outline" click={App.Logout}>Log out</al-button>
  <a href={Todos.Index {}}>Todos</a>
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
 
@@ -19,7 +20,13 @@
 }
 
 view : Model -> Session -> Html Msg
-view model session = ?view
+view model session = <Layout.Page title="Profile">
+  <h1>{model.name}</h1>
+  <p>{model.email}</p>
+  <p>{Text.fromInt model.total} todos, {Text.fromInt model.done} done</p>
+  <al-button variant="outline" click={App.Logout}>Log out</al-button>
+  <a href={Todos.Index {}}>Todos</a>
+</Layout.Page>
 
 test "a visitor is sent to log in" = {
   let r <- Test.open (Profile {})
added import Layout to app/Profile.alder
[exit 0]
$ alder new form Profile Rename name:Text.NonEmpty --msg SaveName
edited app/Profile.alder
--- a/app/Profile.alder
+++ b/app/Profile.alder
@@ -19,6 +19,17 @@
   }
 }
 
+form Rename = {
+  name: Text.NonEmpty,
+}
+
+msg SaveName form Rename
+
+update : Msg -> Model -> Session -> Task Nav
+update msg model session = match msg {
+  SaveName p -> ?saveName,
+}
+
 view : Model -> Session -> Html Msg
 view model session = <Layout.Page title="Profile">
   <h1>{model.name}</h1>
@@ -26,9 +37,34 @@
   <p>{Text.fromInt model.total} todos, {Text.fromInt model.done} done</p>
   <al-button variant="outline" click={App.Logout}>Log out</al-button>
   <a href={Todos.Index {}}>Todos</a>
+
+  <al-card>
+    <h2 slot="header">Rename</h2>
+    <form submit={SaveName}>
+      <al-input field="name" label="Name" />
+      <al-alert variant="destructive"><Errors form /></al-alert>
+      <al-button type="submit">Rename</al-button>
+    </form>
+  </al-card>
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
app/Profile.alder:30:17: warning[W0002]: hole ?saveName has type Task Nav
    SaveName p -> ?saveName,
                  ^^^^^^^^^
app/Profile.alder:61:29: warning[W0002]: hole ?expected has type Answer
    Task.succeed (r.answer == ?expected)
                              ^^^^^^^^^
[exit 0]
$ alder fill --decl Profile.update - <<'EOF'
match msg {
  SaveName p -> {
    let _ <- Db.update User session.user (\u -> { u | name: p.form.name })
    Task.succeed Rerender
  },
}
EOF
edited app/Profile.alder
--- a/app/Profile.alder
+++ b/app/Profile.alder
@@ -27,7 +27,10 @@
 
 update : Msg -> Model -> Session -> Task Nav
 update msg model session = match msg {
-  SaveName p -> ?saveName,
+  SaveName p -> {
+    let _ <- Db.update User session.user (\u -> { u | name: p.form.name })
+    Task.succeed Rerender
+  },
 }
 
 view : Model -> Session -> Html Msg
app/Profile.alder:64:29: warning[W0002]: hole ?expected has type Answer
    Task.succeed (r.answer == ?expected)
                              ^^^^^^^^^
[exit 0]
$ alder fill --decl Profile.view - <<'EOF'
<Layout.Page title="Profile">
  <h1>{model.name}</h1>
  <al-card>
    <p>{model.email}</p>
    <p>{Text.fromInt model.total} todos, {Text.fromInt model.done} done</p>
    <form submit={SaveName}>
      <Errors form />
      <al-input field="name" label="Name" value={model.name} />
      <al-button type="submit">Save name</al-button>
    </form>
  </al-card>
  <al-button variant="outline" click={App.Logout}>Log out</al-button>
  <a href={Todos.Index {}}>Todos</a>
</Layout.Page>
EOF
app/Profile.alder:45:52: error[E0003]: expected Text.NonEmpty, got Text
          <al-input field="name" label="Name" value={model.name} />
                                                     ^^^^^^^^^^
app/Profile.alder:63:29: warning[W0002]: hole ?expected has type Answer
    Task.succeed (r.answer == ?expected)
                              ^^^^^^^^^
alder fill: the result does not check (the errors above): nothing was written
[exit 1]
```

The harness stopped at line 35 of your script: `alder fill --decl Profile.view - <<'EOF'`: it exited 1. The commands after it were not run.

Your script stopped at command 5; the app is as the commands before it left it; continue.

The program type-checks but `alder build` refused it; fix the cause below.

```text
app/Profile.alder:64:29: warning[W0002]: hole ?expected has type Answer
    Task.succeed (r.answer == ?expected)
                              ^^^^^^^^^
alder build: the program has 1 typed hole(s); fill them before building
```
