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
$ alder new type Order "Status = Open | Paid | Cancelled"
wrote app/Order.alder
[exit 0]
$ alder new table Order owner:User.UserId@ref=User status:Status=Open created:Time@created --no-timestamps
edited app/Order.alder
wrote migrations/0002_create_order.sql
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
$ alder new table Product name:Text.NonEmpty price:Int.NonNegative active:Bool=True --no-timestamps
wrote app/Product.alder
wrote migrations/0003_create_product.sql
  create table Product
edited migrations/schema.json
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
$ alder new decl Line lineCost "List Product -> Line -> Int"
edited app/Line.alder
--- a/app/Line.alder
+++ b/app/Line.alder
@@ -13,6 +13,9 @@
   quantity: Int.Positive,
 }
 
+lineCost : List Product -> Line -> Int
+lineCost = ?lineCost
+
 test "deleting its order deletes a Line" = {
   let user1 <- Db.insert User { email: Text.Email "email-1@example.com", name: Text.NonEmpty "name-1", passwordHash: "passwordhash-1" }
   let target <- Db.insert Order { owner: user1 }
app/Line.alder:17:12: warning[W0002]: hole ?lineCost has type List Product -> Line -> Int
  lineCost = ?lineCost
             ^^^^^^^^^
[exit 0]
$ alder fill Line.lineCost - <<'EOF'
\products line -> List.foldl (\p acc -> if p.id == line.product then line.quantity * p.price else acc) 0 products
EOF
edited app/Line.alder
--- a/app/Line.alder
+++ b/app/Line.alder
@@ -14,7 +14,7 @@
 }
 
 lineCost : List Product -> Line -> Int
-lineCost = ?lineCost
+lineCost = \products line -> List.foldl (\p acc -> if p.id == line.product then line.quantity * p.price else acc) 0 products
 
 test "deleting its order deletes a Line" = {
   let user1 <- Db.insert User { email: Text.Email "email-1@example.com", name: Text.NonEmpty "name-1", passwordHash: "passwordhash-1" }
[exit 0]
$ alder new decl Line orderTotal "List Product -> List Line -> Int"
edited app/Line.alder
--- a/app/Line.alder
+++ b/app/Line.alder
@@ -16,6 +16,9 @@
 lineCost : List Product -> Line -> Int
 lineCost = \products line -> List.foldl (\p acc -> if p.id == line.product then line.quantity * p.price else acc) 0 products
 
+orderTotal : List Product -> List Line -> Int
+orderTotal = ?orderTotal
+
 test "deleting its order deletes a Line" = {
   let user1 <- Db.insert User { email: Text.Email "email-1@example.com", name: Text.NonEmpty "name-1", passwordHash: "passwordhash-1" }
   let target <- Db.insert Order { owner: user1 }
app/Line.alder:20:14: warning[W0002]: hole ?orderTotal has type List Product -> List Line -> Int
  orderTotal = ?orderTotal
               ^^^^^^^^^^^
[exit 0]
$ alder fill Line.orderTotal - <<'EOF'
\products lines -> List.foldl (\l acc -> acc + lineCost products l) 0 lines
EOF
edited app/Line.alder
--- a/app/Line.alder
+++ b/app/Line.alder
@@ -17,7 +17,7 @@
 lineCost = \products line -> List.foldl (\p acc -> if p.id == line.product then line.quantity * p.price else acc) 0 products
 
 orderTotal : List Product -> List Line -> Int
-orderTotal = ?orderTotal
+orderTotal = \products lines -> List.foldl (\l acc -> acc + lineCost products l) 0 lines
 
 test "deleting its order deletes a Line" = {
   let user1 <- Db.insert User { email: Text.Email "email-1@example.com", name: Text.NonEmpty "name-1", passwordHash: "passwordhash-1" }
[exit 0]
$ alder new page Orders.Index "/" --requires user --blank
alder new page: `--requires user` needs a session with a `user` field (`session { user: Maybe User.UserId }` in `App`): this app's session has none
[exit 1]
```

The harness stopped at line 32 of your script: `alder new page Orders.Index "/" --requires user --blank`: it exited 1. The commands after it were not run.

Your script stopped at command 13; the app is as the commands before it left it; continue.

The harness ran `alder check`, `alder build` and `alder test` on the app as the commands left it: all pass, but the round does not: the script stopped before its end.
