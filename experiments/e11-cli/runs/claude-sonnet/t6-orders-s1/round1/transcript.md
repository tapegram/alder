### Round 1

```text
$ alder new decl Order statusText "Status -> Text"
edited app/Order.alder
--- a/app/Order.alder
+++ b/app/Order.alder
@@ -13,6 +13,9 @@
   status: Status = Open,
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
$ alder new decl Order isOpenName "Text -> Bool"
edited app/Order.alder
--- a/app/Order.alder
+++ b/app/Order.alder
@@ -20,6 +20,9 @@
   Cancelled -> "Cancelled",
 }
 
+isOpenName : Text -> Bool
+isOpenName = ?isOpenName
+
 test "a Order keeps its owner from being deleted" = {
   let target <- Db.insert User { email: Text.Email "email-1@example.com", name: Text.NonEmpty "name-1", passwordHash: "passwordhash-1" }
   let _ <- Db.insert Order { owner: target }
app/Order.alder:24:14: warning[W0002]: hole ?isOpenName has type Text -> Bool
  isOpenName = ?isOpenName
               ^^^^^^^^^^^
[exit 0]
$ alder fill Order.isOpenName - <<'EOF'
\t -> t == "Open"
EOF
edited app/Order.alder
--- a/app/Order.alder
+++ b/app/Order.alder
@@ -21,7 +21,7 @@
 }
 
 isOpenName : Text -> Bool
-isOpenName = ?isOpenName
+isOpenName = \t -> t == "Open"
 
 test "a Order keeps its owner from being deleted" = {
   let target <- Db.insert User { email: Text.Email "email-1@example.com", name: Text.NonEmpty "name-1", passwordHash: "passwordhash-1" }
[exit 0]
$ alder new decl Product isFree "Int -> Bool"
edited app/Product.alder
--- a/app/Product.alder
+++ b/app/Product.alder
@@ -8,3 +8,6 @@
   price: Int.NonNegative,
   active: Bool = True,
 }
+
+isFree : Int -> Bool
+isFree = ?isFree
app/Product.alder:13:10: warning[W0002]: hole ?isFree has type Int -> Bool
  isFree = ?isFree
           ^^^^^^^
[exit 0]
$ alder fill Product.isFree - <<'EOF'
\n -> n == 0
EOF
edited app/Product.alder
--- a/app/Product.alder
+++ b/app/Product.alder
@@ -10,4 +10,4 @@
 }
 
 isFree : Int -> Bool
-isFree = ?isFree
+isFree = \n -> n == 0
[exit 0]
$ alder new type Line "Row = { name: Text, quantity: Int, price: Int }"
edited app/Line.alder
--- a/app/Line.alder
+++ b/app/Line.alder
@@ -6,6 +6,8 @@
 
 type LineId = brand Text
 
+type Row = { name: Text, quantity: Int, price: Int }
+
 table Line {
   id: LineId @id,
   order: Order.OrderId @ref Order @cascade,
[exit 0]
$ alder new type Line "Choice = { id: Product.ProductId, name: Text }"
edited app/Line.alder
--- a/app/Line.alder
+++ b/app/Line.alder
@@ -8,6 +8,8 @@
 
 type Row = { name: Text, quantity: Int, price: Int }
 
+type Choice = { id: Product.ProductId, name: Text }
+
 table Line {
   id: LineId @id,
   order: Order.OrderId @ref Order @cascade,
[exit 0]
$ alder new decl Line rows "List Line -> List Product -> List Row"
edited app/Line.alder
--- a/app/Line.alder
+++ b/app/Line.alder
@@ -17,6 +17,9 @@
   quantity: Int.Positive,
 }
 
+rows : List Line -> List Product -> List Row
+rows = ?rows
+
 test "deleting its order deletes a Line" = {
   let user1 <- Db.insert User { email: Text.Email "email-1@example.com", name: Text.NonEmpty "name-1", passwordHash: "passwordhash-1" }
   let target <- Db.insert Order { owner: user1 }
app/Line.alder:21:8: warning[W0002]: hole ?rows has type List Line -> List Product -> List Line.Row
  rows = ?rows
         ^^^^^
[exit 0]
$ alder fill Line.rows - <<'EOF'
\lines products -> List.filterMap (\l -> Maybe.map (\p -> { name: p.name, quantity: l.quantity, price: p.price }) (List.find (\p -> p.id == l.product) products)) lines
EOF
edited app/Line.alder
--- a/app/Line.alder
+++ b/app/Line.alder
@@ -18,7 +18,7 @@
 }
 
 rows : List Line -> List Product -> List Row
-rows = ?rows
+rows = \lines products -> List.filterMap (\l -> Maybe.map (\p -> { name: p.name, quantity: l.quantity, price: p.price }) (List.find (\p -> p.id == l.product) products)) lines
 
 test "deleting its order deletes a Line" = {
   let user1 <- Db.insert User { email: Text.Email "email-1@example.com", name: Text.NonEmpty "name-1", passwordHash: "passwordhash-1" }
[exit 0]
$ alder new decl Line total "List Line -> List Product -> Int"
edited app/Line.alder
--- a/app/Line.alder
+++ b/app/Line.alder
@@ -20,6 +20,9 @@
 rows : List Line -> List Product -> List Row
 rows = \lines products -> List.filterMap (\l -> Maybe.map (\p -> { name: p.name, quantity: l.quantity, price: p.price }) (List.find (\p -> p.id == l.product) products)) lines
 
+total : List Line -> List Product -> Int
+total = ?total
+
 test "deleting its order deletes a Line" = {
   let user1 <- Db.insert User { email: Text.Email "email-1@example.com", name: Text.NonEmpty "name-1", passwordHash: "passwordhash-1" }
   let target <- Db.insert Order { owner: user1 }
app/Line.alder:24:9: warning[W0002]: hole ?total has type List Line -> List Product -> Int
  total = ?total
          ^^^^^^
[exit 0]
$ alder fill Line.total - <<'EOF'
\lines products -> List.sum (List.map (\r -> r.quantity * r.price) (rows lines products))
EOF
edited app/Line.alder
--- a/app/Line.alder
+++ b/app/Line.alder
@@ -21,7 +21,7 @@
 rows = \lines products -> List.filterMap (\l -> Maybe.map (\p -> { name: p.name, quantity: l.quantity, price: p.price }) (List.find (\p -> p.id == l.product) products)) lines
 
 total : List Line -> List Product -> Int
-total = ?total
+total = \lines products -> List.sum (List.map (\r -> r.quantity * r.price) (rows lines products))
 
 test "deleting its order deletes a Line" = {
   let user1 <- Db.insert User { email: Text.Email "email-1@example.com", name: Text.NonEmpty "name-1", passwordHash: "passwordhash-1" }
[exit 0]
$ alder new decl Line choices "List Product -> List Choice"
edited app/Line.alder
--- a/app/Line.alder
+++ b/app/Line.alder
@@ -23,6 +23,9 @@
 total : List Line -> List Product -> Int
 total = \lines products -> List.sum (List.map (\r -> r.quantity * r.price) (rows lines products))
 
+choices : List Product -> List Choice
+choices = ?choices
+
 test "deleting its order deletes a Line" = {
   let user1 <- Db.insert User { email: Text.Email "email-1@example.com", name: Text.NonEmpty "name-1", passwordHash: "passwordhash-1" }
   let target <- Db.insert Order { owner: user1 }
app/Line.alder:27:11: warning[W0002]: hole ?choices has type List Product -> List Line.Choice
  choices = ?choices
            ^^^^^^^^
[exit 0]
$ alder fill Line.choices - <<'EOF'
\products -> List.map (\p -> { id: p.id, name: p.name }) (List.filter (\p -> p.active) products)
EOF
edited app/Line.alder
--- a/app/Line.alder
+++ b/app/Line.alder
@@ -24,7 +24,7 @@
 total = \lines products -> List.sum (List.map (\r -> r.quantity * r.price) (rows lines products))
 
 choices : List Product -> List Choice
-choices = ?choices
+choices = \products -> List.map (\p -> { id: p.id, name: p.name }) (List.filter (\p -> p.active) products)
 
 test "deleting its order deletes a Line" = {
   let user1 <- Db.insert User { email: Text.Email "email-1@example.com", name: Text.NonEmpty "name-1", passwordHash: "passwordhash-1" }
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
$ alder new decl Orders.Index imports "Bool"
edited app/Orders/Index.alder
--- a/app/Orders/Index.alder
+++ b/app/Orders/Index.alder
@@ -10,6 +10,9 @@
 view : Model -> Session -> Html Msg
 view model session = ?view
 
+imports : Bool
+imports = ?imports
+
 test "a visitor is sent to log in" = {
   let r <- Test.open (Orders.Index {})
   Task.succeed (r.answer == Redirected (Auth.Login {}))
app/Orders/Index.alder:8:23: warning[W0002]: hole ?load has type Query Orders.Index.Model
  load params session = ?load
                        ^^^^^
app/Orders/Index.alder:11:22: warning[W0002]: hole ?view has type Html Orders.Index.Msg
  view model session = ?view
                       ^^^^^
app/Orders/Index.alder:14:11: warning[W0002]: hole ?imports has type Bool
  imports = ?imports
            ^^^^^^^^
[exit 0]
$ alder fill Orders.Index.imports - <<'EOF'
Order.isOpenName "x" && Line.total [] [] == 0 && Product.isFree 0
EOF
edited app/Orders/Index.alder
--- a/app/Orders/Index.alder
+++ b/app/Orders/Index.alder
@@ -1,5 +1,9 @@
 module Orders.Index
 
+import Line
+import Order
+import Product
+
 page "/" { } requires user
 
 type Model = { }
@@ -11,7 +15,7 @@
 view model session = ?view
 
 imports : Bool
-imports = ?imports
+imports = Order.isOpenName "x" && Line.total [] [] == 0 && Product.isFree 0
 
 test "a visitor is sent to log in" = {
   let r <- Test.open (Orders.Index {})
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
$ alder new type Orders.Index "Row = { id: Order.OrderId, status: Order.Status, total: Int }"
edited app/Orders/Index.alder
--- a/app/Orders/Index.alder
+++ b/app/Orders/Index.alder
@@ -8,6 +8,8 @@
 
 type Model = { }
 
+type Row = { id: Order.OrderId, status: Order.Status, total: Int }
+
 load : Params -> Session -> Query Model
 load params session = ?load
 
app/Orders/Index.alder:14:23: warning[W0002]: hole ?load has type Query Orders.Index.Model
  load params session = ?load
                        ^^^^^
app/Orders/Index.alder:17:22: warning[W0002]: hole ?view has type Html Orders.Index.Msg
  view model session = ?view
                       ^^^^^
[exit 0]
$ alder fill --decl Orders.Index.Model - <<'EOF'
{ orders: List Row }
EOF
edited app/Orders/Index.alder
--- a/app/Orders/Index.alder
+++ b/app/Orders/Index.alder
@@ -6,7 +6,7 @@
 
 page "/" { } requires user
 
-type Model = { }
+type Model = { orders: List Row }
 
 type Row = { id: Order.OrderId, status: Order.Status, total: Int }
 
app/Orders/Index.alder:14:23: warning[W0002]: hole ?load has type Query Orders.Index.Model
  load params session = ?load
                        ^^^^^
app/Orders/Index.alder:17:22: warning[W0002]: hole ?view has type Html Orders.Index.Msg
  view model session = ?view
                       ^^^^^
[exit 0]
$ alder fill Orders.Index.load - <<'EOF'
\params session -> {
  let orders <- Db.from Order |> Db.where (Order.owner.eq session.user) |> Db.orderBy Order.created.desc |> Db.all
  let lines <- Db.from Line |> Db.where (Line.order.in (List.map (\o -> o.id) orders)) |> Db.all
  let products <- Db.from Product |> Db.all
  Query.succeed { orders: List.map (\o -> { id: o.id, status: o.status, total: Line.total (List.filter (\l -> l.order == o.id) lines) products }) orders }
}
EOF
app/Orders/Index.alder:14:23: error[E0003]: expected Query Orders.Index.Model, got _ -> _ -> Query { orders: List { id: Order.OrderId, status: Order.Status, total: Int } }
  load params session = \params session -> {
                        ^
app/Orders/Index.alder:15:59: error[E0003]: cannot infer the record type of `session`; add a type annotation
    let orders <- Db.from Order |> Db.where (Order.owner.eq session.user) |> Db.orderBy Order.created.desc |> Db.all
                                                            ^^^^^^^^^^^^
app/Orders/Index.alder:22:22: warning[W0002]: hole ?view has type Html Orders.Index.Msg
  view model session = ?view
                       ^^^^^
alder fill: the result does not check (the errors above): nothing was written
[exit 1]
```

The harness stopped at line 45 of your script: `alder fill Orders.Index.load - <<'EOF'`: it exited 1. The commands after it were not run.

Your script stopped at command 20; the app is as the commands before it left it; continue.

The program type-checks but `alder build` refused it; fix the cause below.

```text
app/Auth/Login.alder:47:33: warning[W0002]: hole ?afterLogin has type Page
          Task.succeed (Redirect (?afterLogin))
                                  ^^^^^^^^^^^
app/Auth/Login.alder:86:85: warning[W0002]: hole ?afterLogin has type Page
    Task.succeed ({ answer: r.answer, user: r.session.user } == { answer: Redirected (?afterLogin), user: Just me })
                                                                                      ^^^^^^^^^^^
app/Orders/Index.alder:14:23: warning[W0002]: hole ?load has type Query Orders.Index.Model
  load params session = ?load
                        ^^^^^
app/Orders/Index.alder:17:22: warning[W0002]: hole ?view has type Html Orders.Index.Msg
  view model session = ?view
                       ^^^^^
alder build: the program has 4 typed hole(s); fill them before building
```
