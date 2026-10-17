### Round 2

```text
$ alder fill Auth.Login.update.Submit --hole ?afterLogin - <<'EOF'
Orders.Index {}
EOF
edited app/Auth/Login.alder
--- a/app/Auth/Login.alder
+++ b/app/Auth/Login.alder
@@ -44,7 +44,7 @@
     match found {
       Just id -> {
         let _ <- Session.put (\s -> { s | user: Just id })
-        Task.succeed (Redirect (?afterLogin))
+        Task.succeed (Redirect (Orders.Index {}))
       },
       Nothing -> Task.succeed (Reject "Email or password is incorrect."),
     }
app/Auth/Login.alder:86:85: warning[W0002]: hole ?afterLogin has type Page
    Task.succeed ({ answer: r.answer, user: r.session.user } == { answer: Redirected (?afterLogin), user: Just me })
                                                                                      ^^^^^^^^^^^
[exit 0]
$ alder fill 'Auth.Login."logging in through the form"' --hole ?afterLogin - <<'EOF'
Orders.Index {}
EOF
edited app/Auth/Login.alder
--- a/app/Auth/Login.alder
+++ b/app/Auth/Login.alder
@@ -83,7 +83,7 @@
   let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "name-1", passwordHash: "passwordhash-1" }
   let _ <- Test.get (Auth.Login {})
   let r <- Test.submit Credentials { email: Text.Email "me@example.com", password: Text.Secret "secret" }
-  Task.succeed ({ answer: r.answer, user: r.session.user } == { answer: Redirected (?afterLogin), user: Just me })
+  Task.succeed ({ answer: r.answer, user: r.session.user } == { answer: Redirected (Orders.Index {}), user: Just me })
 }
 
 test "a signed-in user logs out here" = {
[exit 0]
$ alder fill Orders.Index.load - <<'EOF'
{
  let orders <- Db.from Order |> Db.where (Order.owner.eq session.user) |> Db.orderBy Order.created.desc |> Db.all
  let rows <- Query.all (List.map (\o -> Query.map (\t -> { id: o.id, status: o.status, total: t }) (Line.total o.id)) orders)
  Query.succeed { orders: rows }
}
EOF
edited app/Orders/Index.alder
--- a/app/Orders/Index.alder
+++ b/app/Orders/Index.alder
@@ -1,5 +1,6 @@
 module Orders.Index
 
+import Line
 import Order
 
 page "/" { } requires user
@@ -7,7 +8,11 @@
 type Model = { orders: List { id: Order.OrderId, status: Order.Status, total: Int } }
 
 load : Params -> Session -> Query Model
-load params session = ?load
+load params session = {
+  let orders <- Db.from Order |> Db.where (Order.owner.eq session.user) |> Db.orderBy Order.created.desc |> Db.all
+  let rows <- Query.all (List.map (\o -> Query.map (\t -> { id: o.id, status: o.status, total: t }) (Line.total o.id)) orders)
+  Query.succeed { orders: rows }
+}
 
 view : Model -> Session -> Html Msg
 view model session = ?view
app/Orders/Index.alder:18:22: warning[W0002]: hole ?view has type Html Orders.Index.Msg
  view model session = ?view
                       ^^^^^
added import Line to app/Orders/Index.alder
[exit 0]
$ alder fill Orders.Index.view - <<'EOF'
Html.none
EOF
edited app/Orders/Index.alder
--- a/app/Orders/Index.alder
+++ b/app/Orders/Index.alder
@@ -15,7 +15,7 @@
 }
 
 view : Model -> Session -> Html Msg
-view model session = ?view
+view model session = Html.none
 
 test "a visitor is sent to log in" = {
   let r <- Test.open (Orders.Index {})
[exit 0]
$ alder new msg Orders.Index Create
alder new msg: `Orders.Index`'s `view` is not one element over several lines (write it as `<Layout.Page …>` … `</Layout.Page>` on several lines, with `alder fill --decl Orders.Index.view -`), or add the markup yourself
[exit 1]
```

The harness stopped at line 20 of your script: `alder new msg Orders.Index Create`: it exited 1. The commands after it were not run.

Your script stopped at command 5; the app is as the commands before it left it; continue.

The harness ran `alder check`, `alder build` and `alder test` on the app as the commands left it: all pass, but the round does not: the script stopped before its end.
