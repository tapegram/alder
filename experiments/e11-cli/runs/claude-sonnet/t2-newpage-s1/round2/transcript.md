### Round 2

```text
$ alder fill --decl Profile.view - <<'EOF'
<Layout.Page title="Profile">
  <h1>Profile</h1>
  <p id="name">{model.name}</p>
  <p id="email">{model.email}</p>
  <p id="total">{Text.fromInt model.total}</p>
  <p id="finished">{Text.fromInt model.finished}</p>
  <a href={Todos.Index {}}>Back to todos</a>
  <al-button variant="outline" click={App.Logout}>Log out</al-button>
</Layout.Page>
EOF
edited app/Profile.alder
--- a/app/Profile.alder
+++ b/app/Profile.alder
@@ -20,7 +20,16 @@
 }
 
 view : Model -> Session -> Html Msg
-view model session = <Layout.Page title="Profile"><h1>Profile</h1></Layout.Page>
+view model session =
+  <Layout.Page title="Profile">
+    <h1>Profile</h1>
+    <p id="name">{model.name}</p>
+    <p id="email">{model.email}</p>
+    <p id="total">{Text.fromInt model.total}</p>
+    <p id="finished">{Text.fromInt model.finished}</p>
+    <a href={Todos.Index {}}>Back to todos</a>
+    <al-button variant="outline" click={App.Logout}>Log out</al-button>
+  </Layout.Page>
 
 test "a visitor is sent to log in" = {
   let r <- Test.open (Profile {})
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
 view model session =
   <Layout.Page title="Profile">
@@ -29,9 +40,34 @@
     <p id="finished">{Text.fromInt model.finished}</p>
     <a href={Todos.Index {}}>Back to todos</a>
     <al-button variant="outline" click={App.Logout}>Log out</al-button>
+
+    <al-card>
+      <h2 slot="header">Rename</h2>
+      <form submit={SaveName}>
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
app/Profile.alder:30:17: warning[W0002]: hole ?saveName has type Task Nav
    SaveName p -> ?saveName,
                  ^^^^^^^^^
app/Profile.alder:64:29: warning[W0002]: hole ?expected has type Answer
    Task.succeed (r.answer == ?expected)
                              ^^^^^^^^^
[exit 0]
$ alder fill Profile.update.SaveName - <<'EOF'
{
  let _ <- Db.update User session.user (\u -> { u | name: p.form.name })
  Task.succeed Rerender
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
app/Profile.alder:67:29: warning[W0002]: hole ?expected has type Answer
    Task.succeed (r.answer == ?expected)
                              ^^^^^^^^^
[exit 0]
$ alder fill --decl Todos.Index.view - <<'EOF'
<Layout.Page title="Todos">
  <h1>Todos</h1>

  <al-card>
    <form submit={Submit}>
      <al-input field="title" label="What needs doing?" />
      <al-input field="due" label="Due" />
      <al-button type="submit">Add</al-button>
    </form>
  </al-card>

  <nav>
    {for f in [All, Active, Completed] ->
      <a href={Todos.Index { filter: f }} aria-current={if model.filter == f then Just "page" else Nothing}>
        {Todo.filterLabel f}
      </a>}
  </nav>

  <ul id="todos">
    {for todo in model.todos -> <Item todo={todo} />}
  </ul>

  <p id="summary">{Text.fromInt (Todo.countOpen model.todos)} open</p>

  <a href={Profile {}}>Profile</a>

  <al-button variant="outline" click={App.Logout}>Log out</al-button>
</Layout.Page>
EOF
edited app/Todos/Index.alder
--- a/app/Todos/Index.alder
+++ b/app/Todos/Index.alder
@@ -74,6 +74,8 @@
 
     <p id="summary">{Text.fromInt (Todo.countOpen model.todos)} open</p>
 
+    <a href={Profile {}}>Profile</a>
+
     <al-button variant="outline" click={App.Logout}>Log out</al-button>
   </Layout.Page>
 
[exit 0]
$ alder new test Profile "load counts only the user's todos"
edited app/Profile.alder
--- a/app/Profile.alder
+++ b/app/Profile.alder
@@ -74,3 +74,7 @@
   let r <- Test.submitRaw Rename [{ name: "name", value: "" }]
   Task.succeed (r.answer == FormRejected [{ field: "name", message: "must not be empty" }])
 }
+
+test "load counts only the user's todos" = {
+  ?test
+}
app/Profile.alder:67:29: warning[W0002]: hole ?expected has type Answer
    Task.succeed (r.answer == ?expected)
                              ^^^^^^^^^
app/Profile.alder:79:3: warning[W0002]: hole ?test has type Task Bool
    ?test
    ^^^^^
[exit 0]
$ alder fill 'Profile."load counts only the user'"'"'s todos"' - <<'EOF'
{
  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
  let other <- Db.insert User { email: Text.Email "other@example.com", name: Text.NonEmpty "Other", passwordHash: "" }
  let _ <- Db.insert Todo { title: Text.NonEmpty "A", done: True, due: Nothing, owner: me }
  let _ <- Db.insert Todo { title: Text.NonEmpty "B", due: Nothing, owner: me }
  let _ <- Db.insert Todo { title: Text.NonEmpty "C", done: True, due: Nothing, owner: other }
  let _ <- Test.login { user: Just me }
  let page <- Test.get (Profile {})
  Task.succeed ({ total: page.model.total, finished: page.model.finished } == { total: 2, finished: 1 })
}
EOF
edited app/Profile.alder
--- a/app/Profile.alder
+++ b/app/Profile.alder
@@ -76,5 +76,12 @@
 }
 
 test "load counts only the user's todos" = {
-  ?test
+  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
+  let other <- Db.insert User { email: Text.Email "other@example.com", name: Text.NonEmpty "Other", passwordHash: "" }
+  let _ <- Db.insert Todo { title: Text.NonEmpty "A", done: True, due: Nothing, owner: me }
+  let _ <- Db.insert Todo { title: Text.NonEmpty "B", due: Nothing, owner: me }
+  let _ <- Db.insert Todo { title: Text.NonEmpty "C", done: True, due: Nothing, owner: other }
+  let _ <- Test.login { user: Just me }
+  let page <- Test.get (Profile {})
+  Task.succeed ({ total: page.model.total, finished: page.model.finished } == { total: 2, finished: 1 })
 }
app/Profile.alder:67:29: warning[W0002]: hole ?expected has type Answer
    Task.succeed (r.answer == ?expected)
                              ^^^^^^^^^
[exit 0]
$ alder new test Profile "renaming to an empty name is refused"
edited app/Profile.alder
--- a/app/Profile.alder
+++ b/app/Profile.alder
@@ -85,3 +85,7 @@
   let page <- Test.get (Profile {})
   Task.succeed ({ total: page.model.total, finished: page.model.finished } == { total: 2, finished: 1 })
 }
+
+test "renaming to an empty name is refused" = {
+  ?test
+}
app/Profile.alder:67:29: warning[W0002]: hole ?expected has type Answer
    Task.succeed (r.answer == ?expected)
                              ^^^^^^^^^
app/Profile.alder:90:3: warning[W0002]: hole ?test has type Task Bool
    ?test
    ^^^^^
[exit 0]
$ alder fill 'Profile."renaming to an empty name is refused"' - <<'EOF'
{
  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
  let _ <- Test.login { user: Just me }
  let _ <- Test.get (Profile {})
  let r <- Test.submitRaw Rename [{ name: "name", value: "" }]
  let after <- Db.get User me
  let rejected = match r.answer {
    FormRejected es -> True,
    _ -> False,
  }
  Task.succeed ({ rejected: rejected, name: Maybe.map (\u -> u.name) after } == { rejected: True, name: Just (Text.NonEmpty "Me") })
}
EOF
edited app/Profile.alder
--- a/app/Profile.alder
+++ b/app/Profile.alder
@@ -87,5 +87,14 @@
 }
 
 test "renaming to an empty name is refused" = {
-  ?test
+  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
+  let _ <- Test.login { user: Just me }
+  let _ <- Test.get (Profile {})
+  let r <- Test.submitRaw Rename [{ name: "name", value: "" }]
+  let after <- Db.get User me
+  let rejected = match r.answer {
+    FormRejected es -> True,
+    _ -> False,
+  }
+  Task.succeed ({ rejected: rejected, name: Maybe.map (\u -> u.name) after } == { rejected: True, name: Just (Text.NonEmpty "Me") })
 }
app/Profile.alder:67:29: warning[W0002]: hole ?expected has type Answer
    Task.succeed (r.answer == ?expected)
                              ^^^^^^^^^
[exit 0]
$ alder test
app/Profile.alder:67:29: warning[W0002]: hole ?expected has type Answer
    Task.succeed (r.answer == ?expected)
                              ^^^^^^^^^
alder test: no tests ran (fill the hole)
[exit 1]
```

The harness stopped at line 78 of your script: `alder test`: it exited 1. The commands after it were not run.

Your script stopped at command 9; the app is as the commands before it left it; continue.

The program type-checks but `alder build` refused it; fix the cause below.

```text
app/Profile.alder:67:29: warning[W0002]: hole ?expected has type Answer
    Task.succeed (r.answer == ?expected)
                              ^^^^^^^^^
alder build: the program has 1 typed hole(s); fill them before building
```
