### Round 0

```text
$ alder new type User "Role = Staff | Manager"
wrote app/User.alder
[exit 0]
$ alder new table User email:Text.Email@unique name:Text.NonEmpty passwordHash:Text role:Role=Staff created:Time@created --no-timestamps
edited app/User.alder
wrote migrations/0001_initial.sql
  create table User
wrote migrations/schema.json
--- a/app/User.alder
+++ b/app/User.alder
@@ -1,3 +1,14 @@
 module User
 
 type Role = Staff | Manager
+
+type UserId = brand Text
+
+table User {
+  id: UserId @id,
+  email: Text.Email @unique,
+  name: Text.NonEmpty,
+  passwordHash: Text,
+  role: Role = Staff,
+  created: Time @created,
+}
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
$ alder new table Product name:Text.NonEmpty price:Int.NonNegative active:Bool=True --no-timestamps
wrote app/Product.alder
wrote migrations/0002_create_product.sql
  create table Product
edited migrations/schema.json
[exit 0]
$ alder new type Order "Status = Open | Paid | Cancelled"
wrote app/Order.alder
[exit 0]
$ alder new table Order owner:User.UserId@ref=User status:Status=Open created:Time@created --no-timestamps
edited app/Order.alder
wrote migrations/0003_create_order.sql
  create table Order
  add index on Order.owner
edited migrations/schema.json
--- a/app/Order.alder
+++ b/app/Order.alder
@@ -1,3 +1,22 @@
 module Order
 
+import User
+
 type Status = Open | Paid | Cancelled
+
+type OrderId = brand Text
+
+table Order {
+  id: OrderId @id,
+  owner: User.UserId @ref User,
+  status: Status = Open,
+  created: Time @created,
+}
+
+test "a Order keeps its owner from being deleted" = {
+  let target <- Db.insert User { email: Text.Email "email-1@example.com", name: Text.NonEmpty "name-1", passwordHash: "passwordhash-1" }
+  let _ <- Db.insert Order { owner: target }
+  let r <- Test.run (Db.delete User target)
+  let left <- Db.from Order |> Db.count
+  Task.succeed ({ deleted: Result.toMaybe r.value, left: left } == { deleted: Nothing, left: 1 })
+}
[exit 0]
$ alder new table Line order:Order.OrderId@ref=Order@cascade product:Product.ProductId@ref=Product quantity:Int.Positive --no-timestamps
wrote app/Line.alder
wrote migrations/0004_create_line.sql
  create table Line
  add index on Line.order
  add index on Line.product
edited migrations/schema.json
[exit 0]
$ alder new decl Order statusLabel "Status -> Text"
edited app/Order.alder
--- a/app/Order.alder
+++ b/app/Order.alder
@@ -13,6 +13,9 @@
   created: Time @created,
 }
 
+statusLabel : Status -> Text
+statusLabel = ?statusLabel
+
 test "a Order keeps its owner from being deleted" = {
   let target <- Db.insert User { email: Text.Email "email-1@example.com", name: Text.NonEmpty "name-1", passwordHash: "passwordhash-1" }
   let _ <- Db.insert Order { owner: target }
app/Order.alder:17:15: warning[W0002]: hole ?statusLabel has type Order.Status -> Text
  statusLabel = ?statusLabel
                ^^^^^^^^^^^^
[exit 0]
$ alder fill Order.statusLabel - <<'EOF'
\s -> match s {
  Open -> "Open",
  Paid -> "Paid",
  Cancelled -> "Cancelled",
}
EOF
edited app/Order.alder
--- a/app/Order.alder
+++ b/app/Order.alder
@@ -14,7 +14,11 @@
 }
 
 statusLabel : Status -> Text
-statusLabel = ?statusLabel
+statusLabel = \s -> match s {
+  Open -> "Open",
+  Paid -> "Paid",
+  Cancelled -> "Cancelled",
+}
 
 test "a Order keeps its owner from being deleted" = {
   let target <- Db.insert User { email: Text.Email "email-1@example.com", name: Text.NonEmpty "name-1", passwordHash: "passwordhash-1" }
[exit 0]
$ alder new decl Line lineCost "Line -> Query Int"
edited app/Line.alder
--- a/app/Line.alder
+++ b/app/Line.alder
@@ -13,6 +13,9 @@
   quantity: Int.Positive,
 }
 
+lineCost : Line -> Query Int
+lineCost = ?lineCost
+
 test "deleting its order deletes a Line" = {
   let me <- Db.insert User { email: Text.Email "email-1@example.com", name: Text.NonEmpty "name-1", passwordHash: "passwordhash-1" }
   let target <- Db.insert Order { owner: me }
app/Line.alder:17:12: warning[W0002]: hole ?lineCost has type Line -> Query Int
  lineCost = ?lineCost
             ^^^^^^^^^
[exit 0]
$ alder fill Line.lineCost - <<'EOF'
\line -> {
  let found <- Db.get Product line.product
  Query.succeed (Maybe.withDefault 0 (Maybe.map (\p -> p.price * line.quantity) found))
}
EOF
edited app/Line.alder
--- a/app/Line.alder
+++ b/app/Line.alder
@@ -14,7 +14,10 @@
 }
 
 lineCost : Line -> Query Int
-lineCost = ?lineCost
+lineCost = \line -> {
+  let found <- Db.get Product line.product
+  Query.succeed (Maybe.withDefault 0 (Maybe.map (\p -> p.price * line.quantity) found))
+}
 
 test "deleting its order deletes a Line" = {
   let me <- Db.insert User { email: Text.Email "email-1@example.com", name: Text.NonEmpty "name-1", passwordHash: "passwordhash-1" }
[exit 0]
$ alder new decl Line orderTotal "Order.OrderId -> Query Int"
edited app/Line.alder
--- a/app/Line.alder
+++ b/app/Line.alder
@@ -19,6 +19,9 @@
   Query.succeed (Maybe.withDefault 0 (Maybe.map (\p -> p.price * line.quantity) found))
 }
 
+orderTotal : Order.OrderId -> Query Int
+orderTotal = ?orderTotal
+
 test "deleting its order deletes a Line" = {
   let me <- Db.insert User { email: Text.Email "email-1@example.com", name: Text.NonEmpty "name-1", passwordHash: "passwordhash-1" }
   let target <- Db.insert Order { owner: me }
app/Line.alder:23:14: warning[W0002]: hole ?orderTotal has type Order.OrderId -> Query Int
  orderTotal = ?orderTotal
               ^^^^^^^^^^^
[exit 0]
$ alder fill Line.orderTotal - <<'EOF'
\order -> {
  let lines <- Db.from Line |> Db.where (Line.order.eq order) |> Db.all
  let costs <- Query.all (List.map lineCost lines)
  Query.succeed (List.sum costs)
}
EOF
edited app/Line.alder
--- a/app/Line.alder
+++ b/app/Line.alder
@@ -20,7 +20,11 @@
 }
 
 orderTotal : Order.OrderId -> Query Int
-orderTotal = ?orderTotal
+orderTotal = \order -> {
+  let lines <- Db.from Line |> Db.where (Line.order.eq order) |> Db.all
+  let costs <- Query.all (List.map lineCost lines)
+  Query.succeed (List.sum costs)
+}
 
 test "deleting its order deletes a Line" = {
   let me <- Db.insert User { email: Text.Email "email-1@example.com", name: Text.NonEmpty "name-1", passwordHash: "passwordhash-1" }
[exit 0]
$ alder new test Line "an order total sums its lines"
edited app/Line.alder
--- a/app/Line.alder
+++ b/app/Line.alder
@@ -45,3 +45,7 @@
   let left <- Db.from Line |> Db.count
   Task.succeed ({ deleted: Result.toMaybe r.value, left: left } == { deleted: Nothing, left: 1 })
 }
+
+test "an order total sums its lines" = {
+  ?test
+}
app/Line.alder:50:3: warning[W0002]: hole ?test has type Task Bool
    ?test
    ^^^^^
[exit 0]
$ alder fill 'Line."an order total sums its lines"' - <<'EOF'
{
  let ann <- Db.insert User { email: Text.Email "ann@example.com", name: Text.NonEmpty "Ann", passwordHash: "" }
  let tea <- Db.insert Product { name: Text.NonEmpty "Tea", price: Int.NonNegative 250 }
  let cake <- Db.insert Product { name: Text.NonEmpty "Cake", price: Int.NonNegative 400 }
  let o <- Db.insert Order { owner: ann }
  let _ <- Db.insert Line { order: o, product: tea, quantity: Int.Positive 2 }
  let _ <- Db.insert Line { order: o, product: cake, quantity: Int.Positive 1 }
  let total <- Line.orderTotal o
  Task.succeed (total == 900)
}
EOF
app/Line.alder:56:16: error[E0002]: inside `Line`, its own names are written unqualified: `orderTotal`
    let total <- Line.orderTotal o
                 ^^^^^^^^^^^^^^^
  fix: orderTotal
alder fill: the result does not check (the errors above): nothing was written
[exit 1]
```

The harness stopped at line 43 of your script: `alder fill 'Line."an order total sums its lines"' - <<'EOF'`: it exited 1. The commands after it were not run.

Your script stopped at command 15; the app is as the commands before it left it; continue.

The program type-checks but `alder build` refused it; fix the cause below.

```text
app/Auth/Login.alder:47:33: warning[W0002]: hole ?afterLogin has type Page
          Task.succeed (Redirect (?afterLogin))
                                  ^^^^^^^^^^^
app/Auth/Login.alder:86:85: warning[W0002]: hole ?afterLogin has type Page
    Task.succeed ({ answer: r.answer, user: r.session.user } == { answer: Redirected (?afterLogin), user: Just me })
                                                                                      ^^^^^^^^^^^
app/Line.alder:50:3: warning[W0002]: hole ?test has type Task Bool
    ?test
    ^^^^^
alder build: the program has 3 typed hole(s); fill them before building
```
