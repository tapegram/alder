### Round 1

```text
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
$ alder fill --list
Auth.Login update.Submit ?afterLogin : Page
Auth.Login "logging in through the form" ?afterLogin : Page
[exit 0]
$ alder new page Orders.Index / --blank --requires user
wrote app/Orders/Index.alder
app/Orders/Index.alder:8:23: warning[W0002]: hole ?load has type Query Orders.Index.Model
  load params session = ?load
                        ^^^^^
app/Orders/Index.alder:11:22: warning[W0002]: hole ?view has type Html Orders.Index.Msg
  view model session = ?view
                       ^^^^^
[exit 0]
$ alder fill --decl Orders.Index.Model - <<'EOF'
{ orders: List { id: Order.OrderId, status: Order.Status, total: Int } }
EOF
edited app/Orders/Index.alder
--- a/app/Orders/Index.alder
+++ b/app/Orders/Index.alder
@@ -1,8 +1,10 @@
 module Orders.Index
 
+import Order
+
 page "/" { } requires user
 
-type Model = { }
+type Model = { orders: List { id: Order.OrderId, status: Order.Status, total: Int } }
 
 load : Params -> Session -> Query Model
 load params session = ?load
app/Orders/Index.alder:10:23: warning[W0002]: hole ?load has type Query Orders.Index.Model
  load params session = ?load
                        ^^^^^
app/Orders/Index.alder:13:22: warning[W0002]: hole ?view has type Html Orders.Index.Msg
  view model session = ?view
                       ^^^^^
added import Order to app/Orders/Index.alder
[exit 0]
$ alder fill Orders.Index.load - <<'EOF'
\params session -> {
  let orders <- Db.from Order |> Db.where (Order.owner.eq session.user) |> Db.orderBy Order.created.desc |> Db.all
  let rows <- Query.all (List.map (\o -> Query.map (\t -> { id: o.id, status: o.status, total: t }) (Line.total o.id)) orders)
  Query.succeed { orders: rows }
}
EOF
alder fill: the parameters `params session` are bound already: send the body without `\params session ->`
[exit 1]
```

The harness stopped at line 8 of your script: `alder fill Orders.Index.load - <<'EOF'`: it exited 1. The commands after it were not run.

Your script stopped at command 5; the app is as the commands before it left it; continue.

The program type-checks but `alder build` refused it; fix the cause below.

```text
app/Auth/Login.alder:47:33: warning[W0002]: hole ?afterLogin has type Page
          Task.succeed (Redirect (?afterLogin))
                                  ^^^^^^^^^^^
app/Auth/Login.alder:86:85: warning[W0002]: hole ?afterLogin has type Page
    Task.succeed ({ answer: r.answer, user: r.session.user } == { answer: Redirected (?afterLogin), user: Just me })
                                                                                      ^^^^^^^^^^^
app/Orders/Index.alder:10:23: warning[W0002]: hole ?load has type Query Orders.Index.Model
  load params session = ?load
                        ^^^^^
app/Orders/Index.alder:13:22: warning[W0002]: hole ?view has type Html Orders.Index.Msg
  view model session = ?view
                       ^^^^^
alder build: the program has 4 typed hole(s); fill them before building
```
