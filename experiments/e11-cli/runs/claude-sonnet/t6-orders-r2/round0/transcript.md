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
$ alder new decl Order statusText "Status -> Text"
edited app/Order.alder
--- a/app/Order.alder
+++ b/app/Order.alder
@@ -13,6 +13,9 @@
   created: Time @created,
 }
 
+statusText : Status -> Text
+statusText = ?statusText
+
 test "a Order keeps its owner from being deleted" = {
   let target <- Db.insert User { email: Text.Email "email-1@example.com", name: Text.NonEmpty "name-1", passwordHash: "passwordhash-1" }
   let _ <- Db.insert Order { owner: target }
app/Order.alder:17:14: warning[W0002]: hole ?statusText has type Order.Status -> Text
  statusText = ?statusText
               ^^^^^^^^^^^
[exit 0]
$ alder fill Order.statusText - <<'EOF'
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
 
 statusText : Status -> Text
-statusText = ?statusText
+statusText = \s -> match s {
+  Open -> "Open",
+  Paid -> "Paid",
+  Cancelled -> "Cancelled",
+}
 
 test "a Order keeps its owner from being deleted" = {
   let target <- Db.insert User { email: Text.Email "email-1@example.com", name: Text.NonEmpty "name-1", passwordHash: "passwordhash-1" }
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
$ alder new decl Line priceOf "List Product -> Line -> Int"
edited app/Line.alder
--- a/app/Line.alder
+++ b/app/Line.alder
@@ -13,6 +13,9 @@
   quantity: Int.Positive,
 }
 
+priceOf : List Product -> Line -> Int
+priceOf = ?priceOf
+
 test "deleting its order deletes a Line" = {
   let user1 <- Db.insert User { email: Text.Email "email-1@example.com", name: Text.NonEmpty "name-1", passwordHash: "passwordhash-1" }
   let target <- Db.insert Order { owner: user1 }
app/Line.alder:17:11: warning[W0002]: hole ?priceOf has type List Product -> Line -> Int
  priceOf = ?priceOf
            ^^^^^^^^
[exit 0]
$ alder fill Line.priceOf - <<'EOF'
\prods l -> Maybe.withDefault 0 (Maybe.map (\p -> l.quantity * p.price) (List.find (\p -> p.id == l.product) prods))
EOF
edited app/Line.alder
--- a/app/Line.alder
+++ b/app/Line.alder
@@ -14,7 +14,7 @@
 }
 
 priceOf : List Product -> Line -> Int
-priceOf = ?priceOf
+priceOf = \prods l -> Maybe.withDefault 0 (Maybe.map (\p -> l.quantity * p.price) (List.find (\p -> p.id == l.product) prods))
 
 test "deleting its order deletes a Line" = {
   let user1 <- Db.insert User { email: Text.Email "email-1@example.com", name: Text.NonEmpty "name-1", passwordHash: "passwordhash-1" }
[exit 0]
$ alder new decl Line total "Order.OrderId -> Query Int"
edited app/Line.alder
--- a/app/Line.alder
+++ b/app/Line.alder
@@ -16,6 +16,9 @@
 priceOf : List Product -> Line -> Int
 priceOf = \prods l -> Maybe.withDefault 0 (Maybe.map (\p -> l.quantity * p.price) (List.find (\p -> p.id == l.product) prods))
 
+total : Order.OrderId -> Query Int
+total = ?total
+
 test "deleting its order deletes a Line" = {
   let user1 <- Db.insert User { email: Text.Email "email-1@example.com", name: Text.NonEmpty "name-1", passwordHash: "passwordhash-1" }
   let target <- Db.insert Order { owner: user1 }
app/Line.alder:20:9: warning[W0002]: hole ?total has type Order.OrderId -> Query Int
  total = ?total
          ^^^^^^
[exit 0]
$ alder fill Line.total - <<'EOF'
\id -> {
  let lines <- Db.from Line |> Db.where (Line.order.eq id) |> Db.all
  let prods <- Db.from Product |> Db.all
  Query.succeed (List.sum (List.map (priceOf prods) lines))
}
EOF
edited app/Line.alder
--- a/app/Line.alder
+++ b/app/Line.alder
@@ -17,7 +17,11 @@
 priceOf = \prods l -> Maybe.withDefault 0 (Maybe.map (\p -> l.quantity * p.price) (List.find (\p -> p.id == l.product) prods))
 
 total : Order.OrderId -> Query Int
-total = ?total
+total = \id -> {
+  let lines <- Db.from Line |> Db.where (Line.order.eq id) |> Db.all
+  let prods <- Db.from Product |> Db.all
+  Query.succeed (List.sum (List.map (priceOf prods) lines))
+}
 
 test "deleting its order deletes a Line" = {
   let user1 <- Db.insert User { email: Text.Email "email-1@example.com", name: Text.NonEmpty "name-1", passwordHash: "passwordhash-1" }
[exit 0]
$ alder new page Orders.Index '/' --blank --requires user
alder new page: `--requires user` needs a session with a `user` field (`session { user: Maybe User.UserId }` in `App`): this app's session has none
[exit 1]
```

The harness stopped at line 39 of your script: `alder new page Orders.Index '/' --blank --requires user`: it exited 1. The commands after it were not run.

Your script stopped at command 15; the app is as the commands before it left it; continue.

The harness ran `alder check`, `alder build` and `alder test` on the app as the commands left it: all pass, but the round does not: the script stopped before its end.
