### Round 1

```text
$ alder fill --decl Profile.Model - <<'EOF'
{ user: User, total: Int, done: Int }
EOF
edited app/Profile.alder
--- a/app/Profile.alder
+++ b/app/Profile.alder
@@ -1,8 +1,10 @@
 module Profile
 
+import User
+
 page "/profile" { } requires user
 
-type Model = { }
+type Model = { user: User, total: Int, done: Int }
 
 load : Params -> Session -> Query Model
 load params session = ?load
app/Profile.alder:10:23: warning[W0002]: hole ?load has type Query Profile.Model
  load params session = ?load
                        ^^^^^
app/Profile.alder:13:22: warning[W0002]: hole ?view has type Html Profile.Msg
  view model session = ?view
                       ^^^^^
added import User to app/Profile.alder
[exit 0]
$ alder fill Profile.load - --hole ?load <<'EOF'
{
  let found <- Db.get User session.user
  let total <- Db.mine Todo |> Db.count
  let done <- Db.mine Todo |> Db.where (Todo.done.eq True) |> Db.count
  match found {
    Just u -> Query.succeed { user: u, total: total, done: done },
    Nothing -> Query.notFound,
  }
}
EOF
edited app/Profile.alder
--- a/app/Profile.alder
+++ b/app/Profile.alder
@@ -1,5 +1,6 @@
 module Profile
 
+import Todo
 import User
 
 page "/profile" { } requires user
@@ -7,7 +8,15 @@
 type Model = { user: User, total: Int, done: Int }
 
 load : Params -> Session -> Query Model
-load params session = ?load
+load params session = {
+  let found <- Db.get User session.user
+  let total <- Db.mine Todo |> Db.count
+  let done <- Db.mine Todo |> Db.where (Todo.done.eq True) |> Db.count
+  match found {
+    Just u -> Query.succeed { user: u, total: total, done: done },
+    Nothing -> Query.notFound,
+  }
+}
 
 view : Model -> Session -> Html Msg
 view model session = ?view
app/Profile.alder:22:22: warning[W0002]: hole ?view has type Html Profile.Msg
  view model session = ?view
                       ^^^^^
added import Todo to app/Profile.alder
[exit 0]
$ alder fill Profile.view - --hole ?view <<'EOF'
<Layout.Page title="Profile">
  <h1>Profile</h1>
  <p id="name">{model.user.name}</p>
  <p id="email">{model.user.email}</p>
  <p id="todos">{Text.fromInt model.total} todos, {Text.fromInt model.done} done</p>
  <a href={Todos.Index {}}>Todos</a>
  <al-button variant="outline" click={App.Logout}>Log out</al-button>
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
 
@@ -19,7 +20,14 @@
 }
 
 view : Model -> Session -> Html Msg
-view model session = ?view
+view model session = <Layout.Page title="Profile">
+  <h1>Profile</h1>
+  <p id="name">{model.user.name}</p>
+  <p id="email">{model.user.email}</p>
+  <p id="todos">{Text.fromInt model.total} todos, {Text.fromInt model.done} done</p>
+  <a href={Todos.Index {}}>Todos</a>
+  <al-button variant="outline" click={App.Logout}>Log out</al-button>
+</Layout.Page>
 
 test "a visitor is sent to log in" = {
   let r <- Test.open (Profile {})
added import Layout to app/Profile.alder
[exit 0]
$ alder new test Profile "load counts only my todos"
edited app/Profile.alder
--- a/app/Profile.alder
+++ b/app/Profile.alder
@@ -33,3 +33,7 @@
   let r <- Test.open (Profile {})
   Task.succeed (r.answer == Redirected (Auth.Login {}))
 }
+
+test "load counts only my todos" = {
+  ?test
+}
app/Profile.alder:38:3: warning[W0002]: hole ?test has type Task Bool
    ?test
    ^^^^^
[exit 0]
$ alder fill 'Profile."load counts only my todos"' - <<'EOF'
{
  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
  let other <- Db.insert User { email: Text.Email "other@example.com", name: Text.NonEmpty "Other", passwordHash: "" }
  let _ <- Db.insert Todo { title: Text.NonEmpty "Milk", due: Nothing, owner: me }
  let _ <- Db.insert Todo { title: Text.NonEmpty "Bread", due: Nothing, owner: me, done: True }
  let _ <- Db.insert Todo { title: Text.NonEmpty "Eggs", due: Nothing, owner: other, done: True }
  let _ <- Test.login { user: Just me }
  let page <- Test.get (Profile {})
  Task.succeed ({ total: page.model.total, done: page.model.done } == { total: 2, done: 1 })
}
EOF
edited app/Profile.alder
--- a/app/Profile.alder
+++ b/app/Profile.alder
@@ -35,5 +35,12 @@
 }
 
 test "load counts only my todos" = {
-  ?test
+  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
+  let other <- Db.insert User { email: Text.Email "other@example.com", name: Text.NonEmpty "Other", passwordHash: "" }
+  let _ <- Db.insert Todo { title: Text.NonEmpty "Milk", due: Nothing, owner: me }
+  let _ <- Db.insert Todo { title: Text.NonEmpty "Bread", due: Nothing, owner: me, done: True }
+  let _ <- Db.insert Todo { title: Text.NonEmpty "Eggs", due: Nothing, owner: other, done: True }
+  let _ <- Test.login { user: Just me }
+  let page <- Test.get (Profile {})
+  Task.succeed ({ total: page.model.total, done: page.model.done } == { total: 2, done: 1 })
 }
[exit 0]
```

The harness ran `alder check`, `alder build` and `alder test` on the app: all pass.
