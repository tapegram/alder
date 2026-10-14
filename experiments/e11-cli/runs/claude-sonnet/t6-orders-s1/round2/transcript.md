### Round 2

```text
$ alder new decl User isManager "Maybe User -> Bool"
edited app/User.alder
--- a/app/User.alder
+++ b/app/User.alder
@@ -12,3 +12,6 @@
   created: Time @created,
   role: Role = Staff,
 }
+
+isManager : Maybe User -> Bool
+isManager = ?isManager
app/User.alder:17:13: warning[W0002]: hole ?isManager has type Maybe User -> Bool
  isManager = ?isManager
              ^^^^^^^^^^
[exit 0]
$ alder fill User.isManager - <<'EOF'
\m -> Maybe.map (\u -> u.role) m == Just Manager
EOF
edited app/User.alder
--- a/app/User.alder
+++ b/app/User.alder
@@ -14,4 +14,4 @@
 }
 
 isManager : Maybe User -> Bool
-isManager = ?isManager
+isManager = \m -> Maybe.map (\u -> u.role) m == Just Manager
[exit 0]
$ alder new page Orders.Show '/orders/{id}' --blank --param id:Order.OrderId --requires user
wrote app/Orders/Show.alder
app/Orders/Show.alder:11:23: warning[W0002]: hole ?load has type Query Orders.Show.Model
  load params session = ?load
                        ^^^^^
app/Orders/Show.alder:14:22: warning[W0002]: hole ?view has type Html Orders.Show.Msg
  view model session = ?view
                       ^^^^^
[exit 0]
$ alder new decl Orders.Show imports "Bool"
edited app/Orders/Show.alder
--- a/app/Orders/Show.alder
+++ b/app/Orders/Show.alder
@@ -13,6 +13,9 @@
 view : Model -> Session -> Html Msg
 view model session = ?view
 
+imports : Bool
+imports = ?imports
+
 test "a visitor is sent to log in" = {
   let me <- Db.insert User { email: Text.Email "email-1@example.com", name: Text.NonEmpty "name-1", passwordHash: "passwordhash-1" }
   let order1 <- Db.insert Order { owner: me }
app/Orders/Show.alder:11:23: warning[W0002]: hole ?load has type Query Orders.Show.Model
  load params session = ?load
                        ^^^^^
app/Orders/Show.alder:14:22: warning[W0002]: hole ?view has type Html Orders.Show.Msg
  view model session = ?view
                       ^^^^^
app/Orders/Show.alder:17:11: warning[W0002]: hole ?imports has type Bool
  imports = ?imports
            ^^^^^^^^
[exit 0]
$ alder fill Orders.Show.imports - <<'EOF'
Order.isOpenName "x" && Line.total [] [] == 0 && Product.isFree 0 && User.isManager Nothing
EOF
edited app/Orders/Show.alder
--- a/app/Orders/Show.alder
+++ b/app/Orders/Show.alder
@@ -1,6 +1,8 @@
 module Orders.Show
 
+import Line
 import Order
+import Product
 import User
 
 page "/orders/{id}" { id: Order.OrderId } requires user
@@ -14,7 +16,7 @@
 view model session = ?view
 
 imports : Bool
-imports = ?imports
+imports = Order.isOpenName "x" && Line.total [] [] == 0 && Product.isFree 0 && User.isManager Nothing
 
 test "a visitor is sent to log in" = {
   let me <- Db.insert User { email: Text.Email "email-1@example.com", name: Text.NonEmpty "name-1", passwordHash: "passwordhash-1" }
app/Orders/Show.alder:13:23: warning[W0002]: hole ?load has type Query Orders.Show.Model
  load params session = ?load
                        ^^^^^
app/Orders/Show.alder:16:22: warning[W0002]: hole ?view has type Html Orders.Show.Msg
  view model session = ?view
                       ^^^^^
added import Line to app/Orders/Show.alder
added import Product to app/Orders/Show.alder
[exit 0]
$ alder fill Orders.Index.load - <<'EOF'
{
  let orders <- Db.from Order |> Db.where (Order.owner.eq session.user) |> Db.orderBy Order.created.desc |> Db.all
  let lines <- Db.from Line |> Db.where (Line.order.in (List.map (\o -> o.id) orders)) |> Db.all
  let products <- Db.from Product |> Db.all
  Query.succeed { orders: List.map (\o -> { id: o.id, status: o.status, total: Line.total (List.filter (\l -> l.order == o.id) lines) products }) orders }
}
EOF
edited app/Orders/Index.alder
--- a/app/Orders/Index.alder
+++ b/app/Orders/Index.alder
@@ -11,7 +11,12 @@
 type Row = { id: Order.OrderId, status: Order.Status, total: Int }
 
 load : Params -> Session -> Query Model
-load params session = ?load
+load params session = {
+  let orders <- Db.from Order |> Db.where (Order.owner.eq session.user) |> Db.orderBy Order.created.desc |> Db.all
+  let lines <- Db.from Line |> Db.where (Line.order.in (List.map (\o -> o.id) orders)) |> Db.all
+  let products <- Db.from Product |> Db.all
+  Query.succeed { orders: List.map (\o -> { id: o.id, status: o.status, total: Line.total (List.filter (\l -> l.order == o.id) lines) products }) orders }
+}
 
 view : Model -> Session -> Html Msg
 view model session = ?view
app/Orders/Index.alder:22:22: warning[W0002]: hole ?view has type Html Orders.Index.Msg
  view model session = ?view
                       ^^^^^
[exit 0]
$ alder fill Orders.Index.view - <<'EOF'
<Layout.Page title="Orders">
  <h1>Orders</h1>
  <ul id="orders">
    {for o in model.orders ->
      <li>
        <a href={Orders.Show { id: o.id }}>Order</a>
        <span class="status">{Order.statusText o.status}</span>
        <span class="total">{Text.fromInt o.total}</span>
      </li>}
  </ul>
</Layout.Page>
EOF
edited app/Orders/Index.alder
--- a/app/Orders/Index.alder
+++ b/app/Orders/Index.alder
@@ -1,5 +1,6 @@
 module Orders.Index
 
+import Layout
 import Line
 import Order
 import Product
@@ -19,7 +20,17 @@
 }
 
 view : Model -> Session -> Html Msg
-view model session = ?view
+view model session = <Layout.Page title="Orders">
+  <h1>Orders</h1>
+  <ul id="orders">
+    {for o in model.orders ->
+      <li>
+        <a href={Orders.Show { id: o.id }}>Order</a>
+        <span class="status">{Order.statusText o.status}</span>
+        <span class="total">{Text.fromInt o.total}</span>
+      </li>}
+  </ul>
+</Layout.Page>
 
 imports : Bool
 imports = Order.isOpenName "x" && Line.total [] [] == 0 && Product.isFree 0
added import Layout to app/Orders/Index.alder
[exit 0]
$ alder new msg Orders.Index Create
edited app/Orders/Index.alder
--- a/app/Orders/Index.alder
+++ b/app/Orders/Index.alder
@@ -4,6 +4,7 @@
 import Line
 import Order
 import Product
+import User
 
 page "/" { } requires user
 
@@ -19,6 +20,13 @@
   Query.succeed { orders: List.map (\o -> { id: o.id, status: o.status, total: Line.total (List.filter (\l -> l.order == o.id) lines) products }) orders }
 }
 
+msg Create
+
+update : Msg -> Model -> Session -> Task Nav
+update msg model session = match msg {
+  Create p -> ?create,
+}
+
 view : Model -> Session -> Html Msg
 view model session = <Layout.Page title="Orders">
   <h1>Orders</h1>
@@ -30,6 +38,8 @@
         <span class="total">{Text.fromInt o.total}</span>
       </li>}
   </ul>
+
+  <al-button click={Create}>Create</al-button>
 </Layout.Page>
 
 imports : Bool
@@ -39,3 +49,17 @@
   let r <- Test.open (Orders.Index {})
   Task.succeed (r.answer == Redirected (Auth.Login {}))
 }
+
+-- Fill ?expected with what pressing Create must do, so that a test fails when the arm
+-- does something else: its answer (`r.answer`: Rerendered, Redirected …) and what it
+-- changed (`after`: the page opened again, `after.answer` Shown, or PageMissing when the arm
+-- deleted the page's row, and `after.doc` its HTML), as
+-- `{ answer: r.answer, … } == { answer: Rerendered, … }`.
+test "pressing Create" = {
+  let me <- Db.insert User { email: Text.Email "email-1@example.com", name: Text.NonEmpty "name-1", passwordHash: "passwordhash-1" }
+  let _ <- Test.login { user: Just me }
+  let _ <- Test.get (Orders.Index {})
+  let r <- Test.click Create
+  let after <- Test.open (Orders.Index {})
+  Task.succeed ?expected
+}
app/Orders/Index.alder:27:15: warning[W0002]: hole ?create has type Task Nav
    Create p -> ?create,
                ^^^^^^^
app/Orders/Index.alder:64:16: warning[W0002]: hole ?expected has type Bool
    Task.succeed ?expected
                 ^^^^^^^^^
[exit 0]
$ alder fill Orders.Index.update.Create - <<'EOF'
{
  let id <- Db.insert Order { owner: session.user }
  Task.succeed (Redirect (Orders.Show { id: id }))
}
EOF
edited app/Orders/Index.alder
--- a/app/Orders/Index.alder
+++ b/app/Orders/Index.alder
@@ -24,7 +24,10 @@
 
 update : Msg -> Model -> Session -> Task Nav
 update msg model session = match msg {
-  Create p -> ?create,
+  Create p -> {
+    let id <- Db.insert Order { owner: session.user }
+    Task.succeed (Redirect (Orders.Show { id: id }))
+  },
 }
 
 view : Model -> Session -> Html Msg
app/Orders/Index.alder:67:16: warning[W0002]: hole ?expected has type Bool
    Task.succeed ?expected
                 ^^^^^^^^^
[exit 0]
$ alder fill --decl Orders.Index.view - <<'EOF'
<Layout.Page title="Orders">
  <h1>Orders</h1>
  <button click={Create}>New order</button>
  <ul id="orders">
    {for o in model.orders ->
      <li>
        <a href={Orders.Show { id: o.id }}>Order</a>
        <span class="status">{Order.statusText o.status}</span>
        <span class="total">{Text.fromInt o.total}</span>
      </li>}
  </ul>
</Layout.Page>
EOF
edited app/Orders/Index.alder
--- a/app/Orders/Index.alder
+++ b/app/Orders/Index.alder
@@ -31,19 +31,19 @@
 }
 
 view : Model -> Session -> Html Msg
-view model session = <Layout.Page title="Orders">
-  <h1>Orders</h1>
-  <ul id="orders">
-    {for o in model.orders ->
-      <li>
-        <a href={Orders.Show { id: o.id }}>Order</a>
-        <span class="status">{Order.statusText o.status}</span>
-        <span class="total">{Text.fromInt o.total}</span>
-      </li>}
-  </ul>
-
-  <al-button click={Create}>Create</al-button>
-</Layout.Page>
+view model session =
+  <Layout.Page title="Orders">
+    <h1>Orders</h1>
+    <button click={Create}>New order</button>
+    <ul id="orders">
+      {for o in model.orders ->
+        <li>
+          <a href={Orders.Show { id: o.id }}>Order</a>
+          <span class="status">{Order.statusText o.status}</span>
+          <span class="total">{Text.fromInt o.total}</span>
+        </li>}
+    </ul>
+  </Layout.Page>
 
 imports : Bool
 imports = Order.isOpenName "x" && Line.total [] [] == 0 && Product.isFree 0
app/Orders/Index.alder:67:16: warning[W0002]: hole ?expected has type Bool
    Task.succeed ?expected
                 ^^^^^^^^^
[exit 0]
$ alder fill --decl Orders.Show.Model - <<'EOF'
{ id: Order.OrderId, status: Order.Status, rows: List Line.Row, total: Int, choices: List Line.Choice }
EOF
edited app/Orders/Show.alder
--- a/app/Orders/Show.alder
+++ b/app/Orders/Show.alder
@@ -7,7 +7,7 @@
 
 page "/orders/{id}" { id: Order.OrderId } requires user
 
-type Model = { }
+type Model = { id: Order.OrderId, status: Order.Status, rows: List Line.Row, total: Int, choices: List Line.Choice }
 
 load : Params -> Session -> Query Model
 load params session = ?load
app/Orders/Show.alder:13:23: warning[W0002]: hole ?load has type Query Orders.Show.Model
  load params session = ?load
                        ^^^^^
app/Orders/Show.alder:16:22: warning[W0002]: hole ?view has type Html Orders.Show.Msg
  view model session = ?view
                       ^^^^^
[exit 0]
$ alder fill Orders.Show.load - <<'EOF'
{
  let found <- Db.get Order params.id
  let me <- Db.get User session.user
  match found {
    Nothing -> Query.notFound,
    Just o -> if o.owner == session.user || User.isManager me then {
      let lines <- Db.from Line |> Db.where (Line.order.eq o.id) |> Db.all
      let products <- Db.from Product |> Db.all
      Query.succeed { id: o.id, status: o.status, rows: Line.rows lines products, total: Line.total lines products, choices: Line.choices products }
    } else Query.notFound,
  }
}
EOF
edited app/Orders/Show.alder
--- a/app/Orders/Show.alder
+++ b/app/Orders/Show.alder
@@ -10,7 +10,18 @@
 type Model = { id: Order.OrderId, status: Order.Status, rows: List Line.Row, total: Int, choices: List Line.Choice }
 
 load : Params -> Session -> Query Model
-load params session = ?load
+load params session = {
+  let found <- Db.get Order params.id
+  let me <- Db.get User session.user
+  match found {
+    Nothing -> Query.notFound,
+    Just o -> if o.owner == session.user || User.isManager me then {
+      let lines <- Db.from Line |> Db.where (Line.order.eq o.id) |> Db.all
+      let products <- Db.from Product |> Db.all
+      Query.succeed { id: o.id, status: o.status, rows: Line.rows lines products, total: Line.total lines products, choices: Line.choices products }
+    } else Query.notFound,
+  }
+}
 
 view : Model -> Session -> Html Msg
 view model session = ?view
app/Orders/Show.alder:27:22: warning[W0002]: hole ?view has type Html Orders.Show.Msg
  view model session = ?view
                       ^^^^^
[exit 0]
$ alder fill Orders.Show.view - <<'EOF'
<Layout.Page title="Order">
  <h1>Order</h1>
  <p id="status">{Order.statusText model.status}</p>
  <p id="total">{Text.fromInt model.total}</p>
</Layout.Page>
EOF
edited app/Orders/Show.alder
--- a/app/Orders/Show.alder
+++ b/app/Orders/Show.alder
@@ -1,5 +1,6 @@
 module Orders.Show
 
+import Layout
 import Line
 import Order
 import Product
@@ -24,7 +25,11 @@
 }
 
 view : Model -> Session -> Html Msg
-view model session = ?view
+view model session = <Layout.Page title="Order">
+  <h1>Order</h1>
+  <p id="status">{Order.statusText model.status}</p>
+  <p id="total">{Text.fromInt model.total}</p>
+</Layout.Page>
 
 imports : Bool
 imports = Order.isOpenName "x" && Line.total [] [] == 0 && Product.isFree 0 && User.isManager Nothing
added import Layout to app/Orders/Show.alder
[exit 0]
$ alder new msg Orders.Show AddLine product:Product.ProductId
edited app/Orders/Show.alder
--- a/app/Orders/Show.alder
+++ b/app/Orders/Show.alder
@@ -24,11 +24,20 @@
   }
 }
 
+msg AddLine { product: Product.ProductId }
+
+update : Msg -> Model -> Session -> Task Nav
+update msg model session = match msg {
+  AddLine p -> ?addLine,
+}
+
 view : Model -> Session -> Html Msg
 view model session = <Layout.Page title="Order">
   <h1>Order</h1>
   <p id="status">{Order.statusText model.status}</p>
   <p id="total">{Text.fromInt model.total}</p>
+
+  <al-button click={AddLine { product: ?addLineProduct }}>Add line</al-button>
 </Layout.Page>
 
 imports : Bool
@@ -40,3 +49,17 @@
   let r <- Test.open (Orders.Show { id: order1 })
   Task.succeed (r.answer == Redirected (Auth.Login {}))
 }
+
+-- Fill ?expected with what pressing AddLine must do, so that a test fails when the arm
+-- does something else: its answer (`r.answer`: Rerendered, Redirected …) and what it
+-- changed (`after`, the row read again), as `{ answer: r.answer, … } == { answer: Rerendered, … }`.
+test "pressing AddLine" = {
+  let me <- Db.insert User { email: Text.Email "email-1@example.com", name: Text.NonEmpty "name-1", passwordHash: "passwordhash-1" }
+  let _ <- Test.login { user: Just me }
+  let order1 <- Db.insert Order { owner: me }
+  let product1 <- Db.insert Product { name: Text.NonEmpty "name-1", price: Int.NonNegative 1 }
+  let _ <- Test.get (Orders.Show { id: order1 })
+  let r <- Test.click (AddLine { product: product1 })
+  let after <- Db.get Product product1
+  Task.succeed ?expected
+}
app/Orders/Show.alder:31:16: warning[W0002]: hole ?addLine has type Task Nav
    AddLine p -> ?addLine,
                 ^^^^^^^^
app/Orders/Show.alder:40:40: warning[W0002]: hole ?addLineProduct has type Product.ProductId
    <al-button click={AddLine { product: ?addLineProduct }}>Add line</al-button>
                                         ^^^^^^^^^^^^^^^
app/Orders/Show.alder:64:16: warning[W0002]: hole ?expected has type Bool
    Task.succeed ?expected
                 ^^^^^^^^^
[exit 0]
$ alder new form Orders.Show NewLine quantity:Int.Positive --msg AddLine
alder new form: `Orders.Show` already declares `AddLine`: choose another name
[exit 1]
```

The harness stopped at line 84 of your script: `alder new form Orders.Show NewLine quantity:Int.Positive --msg AddLine`: it exited 1. The commands after it were not run.

Your script stopped at command 15; the app is as the commands before it left it; continue.

The program type-checks but `alder build` refused it; fix the cause below.

```text
app/Auth/Login.alder:47:33: warning[W0002]: hole ?afterLogin has type Page
          Task.succeed (Redirect (?afterLogin))
                                  ^^^^^^^^^^^
app/Auth/Login.alder:86:85: warning[W0002]: hole ?afterLogin has type Page
    Task.succeed ({ answer: r.answer, user: r.session.user } == { answer: Redirected (?afterLogin), user: Just me })
                                                                                      ^^^^^^^^^^^
app/Orders/Index.alder:67:16: warning[W0002]: hole ?expected has type Bool
    Task.succeed ?expected
                 ^^^^^^^^^
app/Orders/Show.alder:31:16: warning[W0002]: hole ?addLine has type Task Nav
    AddLine p -> ?addLine,
                 ^^^^^^^^
app/Orders/Show.alder:40:40: warning[W0002]: hole ?addLineProduct has type Product.ProductId
    <al-button click={AddLine { product: ?addLineProduct }}>Add line</al-button>
                                         ^^^^^^^^^^^^^^^
app/Orders/Show.alder:64:16: warning[W0002]: hole ?expected has type Bool
    Task.succeed ?expected
                 ^^^^^^^^^
alder build: the program has 6 typed hole(s); fill them before building
```
