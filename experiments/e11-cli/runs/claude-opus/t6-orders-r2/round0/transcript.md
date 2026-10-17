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
$ alder fill --list --json
{"holes":[{"module":"Auth.Login","decl":"update","arm":"Submit","name":"afterLogin","type":"Page","file":"app/Auth/Login.alder","line":47,"col":33},{"module":"Auth.Login","decl":"\"logging in through the form\"","name":"afterLogin","type":"Page","file":"app/Auth/Login.alder","line":86,"col":85}]}
[exit 0]
$ alder new decl User isManager "UserId -> Query Bool"
edited app/User.alder
--- a/app/User.alder
+++ b/app/User.alder
@@ -12,3 +12,6 @@
   role: Role = Staff,
   created: Time @created,
 }
+
+isManager : UserId -> Query Bool
+isManager = ?isManager
app/User.alder:17:13: warning[W0002]: hole ?isManager has type User.UserId -> Query Bool
  isManager = ?isManager
              ^^^^^^^^^^
[exit 0]
$ alder fill User.isManager - <<'EOF'
\id -> {
  let me <- Db.get User id
  Query.succeed (Maybe.map (\u -> u.role) me == Just Manager)
}
EOF
edited app/User.alder
--- a/app/User.alder
+++ b/app/User.alder
@@ -14,4 +14,7 @@
 }
 
 isManager : UserId -> Query Bool
-isManager = ?isManager
+isManager = \id -> {
+  let me <- Db.get User id
+  Query.succeed (Maybe.map (\u -> u.role) me == Just Manager)
+}
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
$ alder new decl Line priceOf "List Product -> Product.ProductId -> Int"
edited app/Line.alder
--- a/app/Line.alder
+++ b/app/Line.alder
@@ -13,6 +13,9 @@
   quantity: Int.Positive,
 }
 
+priceOf : List Product -> Product.ProductId -> Int
+priceOf = ?priceOf
+
 test "deleting its order deletes a Line" = {
   let user1 <- Db.insert User { email: Text.Email "email-1@example.com", name: Text.NonEmpty "name-1", passwordHash: "passwordhash-1" }
   let target <- Db.insert Order { owner: user1 }
app/Line.alder:17:11: warning[W0002]: hole ?priceOf has type List Product -> Product.ProductId -> Int
  priceOf = ?priceOf
            ^^^^^^^^
[exit 0]
$ alder fill Line.priceOf - <<'EOF'
\products id -> match List.find (\p -> p.id == id) products {
  Just p -> Int.max 0 p.price,
  Nothing -> 0,
}
EOF
edited app/Line.alder
--- a/app/Line.alder
+++ b/app/Line.alder
@@ -14,7 +14,10 @@
 }
 
 priceOf : List Product -> Product.ProductId -> Int
-priceOf = ?priceOf
+priceOf = \products id -> match List.find (\p -> p.id == id) products {
+  Just p -> Int.max 0 p.price,
+  Nothing -> 0,
+}
 
 test "deleting its order deletes a Line" = {
   let user1 <- Db.insert User { email: Text.Email "email-1@example.com", name: Text.NonEmpty "name-1", passwordHash: "passwordhash-1" }
[exit 0]
$ alder new decl Line total "List Product -> List Line -> Int"
edited app/Line.alder
--- a/app/Line.alder
+++ b/app/Line.alder
@@ -19,6 +19,9 @@
   Nothing -> 0,
 }
 
+total : List Product -> List Line -> Int
+total = ?total
+
 test "deleting its order deletes a Line" = {
   let user1 <- Db.insert User { email: Text.Email "email-1@example.com", name: Text.NonEmpty "name-1", passwordHash: "passwordhash-1" }
   let target <- Db.insert Order { owner: user1 }
app/Line.alder:23:9: warning[W0002]: hole ?total has type List Product -> List Line -> Int
  total = ?total
          ^^^^^^
[exit 0]
$ alder fill Line.total - <<'EOF'
\products lines -> lines |> List.map (\l -> l.quantity * priceOf products l.product) |> List.sum
EOF
edited app/Line.alder
--- a/app/Line.alder
+++ b/app/Line.alder
@@ -20,7 +20,7 @@
 }
 
 total : List Product -> List Line -> Int
-total = ?total
+total = \products lines -> lines |> List.map (\l -> l.quantity * priceOf products l.product) |> List.sum
 
 test "deleting its order deletes a Line" = {
   let user1 <- Db.insert User { email: Text.Email "email-1@example.com", name: Text.NonEmpty "name-1", passwordHash: "passwordhash-1" }
[exit 0]
$ alder new page Orders.Index / --requires user --blank
wrote app/Orders/Index.alder
app/Orders/Index.alder:8:23: warning[W0002]: hole ?load has type Query Orders.Index.Model
  load params session = ?load
                        ^^^^^
app/Orders/Index.alder:11:22: warning[W0002]: hole ?view has type Html Orders.Index.Msg
  view model session = ?view
                       ^^^^^
[exit 0]
$ alder fill --decl Orders.Index.Model - <<'EOF'
{ orders: List Order.Order, lines: List Line.Line, products: List Product.Product }
EOF
edited app/Orders/Index.alder
--- a/app/Orders/Index.alder
+++ b/app/Orders/Index.alder
@@ -1,8 +1,12 @@
 module Orders.Index
 
+import Line
+import Order
+import Product
+
 page "/" { } requires user
 
-type Model = { }
+type Model = { orders: List Order.Order, lines: List Line.Line, products: List Product.Product }
 
 load : Params -> Session -> Query Model
 load params session = ?load
app/Orders/Index.alder:12:23: warning[W0002]: hole ?load has type Query Orders.Index.Model
  load params session = ?load
                        ^^^^^
app/Orders/Index.alder:15:22: warning[W0002]: hole ?view has type Html Orders.Index.Msg
  view model session = ?view
                       ^^^^^
added import Order to app/Orders/Index.alder
added import Line to app/Orders/Index.alder
added import Product to app/Orders/Index.alder
[exit 0]
$ alder fill Orders.Index.load - <<'EOF'
{
  let orders <- Db.from Order |> Db.where (Order.owner.eq session.user) |> Db.orderBy Order.created.desc |> Db.all
  let lines <- Db.from Line |> Db.where (Line.order.in (List.map (\o -> o.id) orders)) |> Db.all
  let products <- Db.from Product |> Db.all
  Query.succeed { orders: orders, lines: lines, products: products }
}
EOF
edited app/Orders/Index.alder
--- a/app/Orders/Index.alder
+++ b/app/Orders/Index.alder
@@ -9,7 +9,12 @@
 type Model = { orders: List Order.Order, lines: List Line.Line, products: List Product.Product }
 
 load : Params -> Session -> Query Model
-load params session = ?load
+load params session = {
+  let orders <- Db.from Order |> Db.where (Order.owner.eq session.user) |> Db.orderBy Order.created.desc |> Db.all
+  let lines <- Db.from Line |> Db.where (Line.order.in (List.map (\o -> o.id) orders)) |> Db.all
+  let products <- Db.from Product |> Db.all
+  Query.succeed { orders: orders, lines: lines, products: products }
+}
 
 view : Model -> Session -> Html Msg
 view model session = ?view
app/Orders/Index.alder:20:22: warning[W0002]: hole ?view has type Html Orders.Index.Msg
  view model session = ?view
                       ^^^^^
[exit 0]
$ alder fill Orders.Index.view - <<'EOF'
<p>Orders</p>
EOF
edited app/Orders/Index.alder
--- a/app/Orders/Index.alder
+++ b/app/Orders/Index.alder
@@ -17,7 +17,7 @@
 }
 
 view : Model -> Session -> Html Msg
-view model session = ?view
+view model session = <p>Orders</p>
 
 test "a visitor is sent to log in" = {
   let r <- Test.open (Orders.Index {})
[exit 0]
$ alder new msg Orders.Index Create
alder new msg: `Orders.Index`'s `view` is not one element over several lines (write it as `<Layout.Page …>` … `</Layout.Page>` on several lines, with `alder fill --decl Orders.Index.view -`), or add the markup yourself
[exit 1]
```

The harness stopped at line 58 of your script: `alder new msg Orders.Index Create`: it exited 1. The commands after it were not run.

Your script stopped at command 21; the app is as the commands before it left it; continue.

The program type-checks but `alder build` refused it; fix the cause below.

```text
app/Auth/Login.alder:47:33: warning[W0002]: hole ?afterLogin has type Page
          Task.succeed (Redirect (?afterLogin))
                                  ^^^^^^^^^^^
app/Auth/Login.alder:86:85: warning[W0002]: hole ?afterLogin has type Page
    Task.succeed ({ answer: r.answer, user: r.session.user } == { answer: Redirected (?afterLogin), user: Just me })
                                                                                      ^^^^^^^^^^^
alder build: the program has 2 typed hole(s); fill them before building
```
