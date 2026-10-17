### Round 3

```text
$ alder new page Orders.Show '/orders/{id}' --blank --requires user --param id:Order.OrderId
wrote app/Orders/Show.alder
app/Orders/Show.alder:11:23: warning[W0002]: hole ?load has type Query Orders.Show.Model
  load params session = ?load
                        ^^^^^
app/Orders/Show.alder:14:22: warning[W0002]: hole ?view has type Html Orders.Show.Msg
  view model session = ?view
                       ^^^^^
[exit 0]
$ alder fill --decl Orders.Show.Model - <<'EOF'
{ order: Order, lines: List { name: Text, quantity: Int, subtotal: Int }, products: List Product, total: Int }
EOF
edited app/Orders/Show.alder
--- a/app/Orders/Show.alder
+++ b/app/Orders/Show.alder
@@ -1,11 +1,12 @@
 module Orders.Show
 
 import Order
+import Product
 import User
 
 page "/orders/{id}" { id: Order.OrderId } requires user
 
-type Model = { }
+type Model = { order: Order, lines: List { name: Text, quantity: Int, subtotal: Int }, products: List Product, total: Int }
 
 load : Params -> Session -> Query Model
 load params session = ?load
app/Orders/Show.alder:12:23: warning[W0002]: hole ?load has type Query Orders.Show.Model
  load params session = ?load
                        ^^^^^
app/Orders/Show.alder:15:22: warning[W0002]: hole ?view has type Html Orders.Show.Msg
  view model session = ?view
                       ^^^^^
added import Product to app/Orders/Show.alder
[exit 0]
$ alder fill Orders.Show.load - <<'EOF'
{
  let found <- Db.get Order params.id
  match found {
    Nothing -> Query.notFound,
    Just o -> {
      let mgr <- User.isManager session.user
      if o.owner == session.user || mgr then {
        let lines <- Db.from Line |> Db.where (Line.order.eq o.id) |> Db.all
        let prods <- Db.from Product |> Db.all
        let total <- Line.total o.id
        Query.succeed { order: o, lines: List.map (\l -> { name: Maybe.withDefault "" (Maybe.map (\p -> Text.trim p.name) (List.find (\p -> p.id == l.product) prods)), quantity: l.quantity, subtotal: Line.priceOf prods l }) lines, products: List.filter (\p -> p.active) prods, total: total }
      } else Query.notFound
    },
  }
}
EOF
edited app/Orders/Show.alder
--- a/app/Orders/Show.alder
+++ b/app/Orders/Show.alder
@@ -1,5 +1,6 @@
 module Orders.Show
 
+import Line
 import Order
 import Product
 import User
@@ -9,7 +10,21 @@
 type Model = { order: Order, lines: List { name: Text, quantity: Int, subtotal: Int }, products: List Product, total: Int }
 
 load : Params -> Session -> Query Model
-load params session = ?load
+load params session = {
+  let found <- Db.get Order params.id
+  match found {
+    Nothing -> Query.notFound,
+    Just o -> {
+      let mgr <- User.isManager session.user
+      if o.owner == session.user || mgr then {
+        let lines <- Db.from Line |> Db.where (Line.order.eq o.id) |> Db.all
+        let prods <- Db.from Product |> Db.all
+        let total <- Line.total o.id
+        Query.succeed { order: o, lines: List.map (\l -> { name: Maybe.withDefault "" (Maybe.map (\p -> Text.trim p.name) (List.find (\p -> p.id == l.product) prods)), quantity: l.quantity, subtotal: Line.priceOf prods l }) lines, products: List.filter (\p -> p.active) prods, total: total }
+      } else Query.notFound
+    },
+  }
+}
 
 view : Model -> Session -> Html Msg
 view model session = ?view
app/Orders/Show.alder:30:22: warning[W0002]: hole ?view has type Html Orders.Show.Msg
  view model session = ?view
                       ^^^^^
added import Line to app/Orders/Show.alder
[exit 0]
$ alder fill Orders.Show.view - <<'EOF'
<Layout.Page title="Order">
  <h1>Order</h1>
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
@@ -27,7 +28,9 @@
 }
 
 view : Model -> Session -> Html Msg
-view model session = ?view
+view model session = <Layout.Page title="Order">
+  <h1>Order</h1>
+</Layout.Page>
 
 test "a visitor is sent to log in" = {
   let me <- Db.insert User { email: Text.Email "email-1@example.com", name: Text.NonEmpty "name-1", passwordHash: "passwordhash-1" }
added import Layout to app/Orders/Show.alder
[exit 0]
$ alder fill --decl Orders.Index.view - <<'EOF'
<Layout.Page title="Orders">
  <h1>Orders</h1>
  <ul id="orders">
    {for o in model.orders ->
      <li>
        <a href={Orders.Show { id: o.id }}>{o.id}</a>
        <span class="status">{Order.statusText o.status}</span>
        <span class="total">{Text.fromInt o.total}</span>
      </li>}
  </ul>
  <al-button variant="outline" click={App.Logout}>Log out</al-button>
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
 
@@ -15,7 +16,19 @@
 }
 
 view : Model -> Session -> Html Msg
-view model session = Html.none
+view model session =
+  <Layout.Page title="Orders">
+    <h1>Orders</h1>
+    <ul id="orders">
+      {for o in model.orders ->
+        <li>
+          <a href={Orders.Show { id: o.id }}>{o.id}</a>
+          <span class="status">{Order.statusText o.status}</span>
+          <span class="total">{Text.fromInt o.total}</span>
+        </li>}
+    </ul>
+    <al-button variant="outline" click={App.Logout}>Log out</al-button>
+  </Layout.Page>
 
 test "a visitor is sent to log in" = {
   let r <- Test.open (Orders.Index {})
added import Layout to app/Orders/Index.alder
[exit 0]
$ alder new msg Orders.Index Create
edited app/Orders/Index.alder
--- a/app/Orders/Index.alder
+++ b/app/Orders/Index.alder
@@ -3,6 +3,7 @@
 import Layout
 import Line
 import Order
+import User
 
 page "/" { } requires user
 
@@ -15,6 +16,13 @@
   Query.succeed { orders: rows }
 }
 
+msg Create
+
+update : Msg -> Model -> Session -> Task Nav
+update msg model session = match msg {
+  Create p -> ?create,
+}
+
 view : Model -> Session -> Html Msg
 view model session =
   <Layout.Page title="Orders">
@@ -28,9 +36,25 @@
         </li>}
     </ul>
     <al-button variant="outline" click={App.Logout}>Log out</al-button>
+
+    <al-button click={Create}>Create</al-button>
   </Layout.Page>
 
 test "a visitor is sent to log in" = {
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
app/Orders/Index.alder:23:15: warning[W0002]: hole ?create has type Task Nav
    Create p -> ?create,
                ^^^^^^^
app/Orders/Index.alder:59:16: warning[W0002]: hole ?expected has type Bool
    Task.succeed ?expected
                 ^^^^^^^^^
[exit 0]
$ alder new msg Orders.Show AddLine product:Product.ProductId
edited app/Orders/Show.alder
--- a/app/Orders/Show.alder
+++ b/app/Orders/Show.alder
@@ -27,9 +27,18 @@
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
+
+  <al-button click={AddLine { product: ?addLineProduct }}>Add line</al-button>
 </Layout.Page>
 
 test "a visitor is sent to log in" = {
@@ -38,3 +47,17 @@
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
app/Orders/Show.alder:34:16: warning[W0002]: hole ?addLine has type Task Nav
    AddLine p -> ?addLine,
                 ^^^^^^^^
app/Orders/Show.alder:41:40: warning[W0002]: hole ?addLineProduct has type Product.ProductId
    <al-button click={AddLine { product: ?addLineProduct }}>Add line</al-button>
                                         ^^^^^^^^^^^^^^^
app/Orders/Show.alder:62:16: warning[W0002]: hole ?expected has type Bool
    Task.succeed ?expected
                 ^^^^^^^^^
[exit 0]
$ alder new form Orders.Show NewLine quantity:Int.Positive --msg AddLine
edited app/Orders/Show.alder
--- a/app/Orders/Show.alder
+++ b/app/Orders/Show.alder
@@ -27,8 +27,13 @@
   }
 }
 
-msg AddLine { product: Product.ProductId }
+-- AddLine's arm reads the posted fields as p.form (p.form : NewLine, such as p.form.quantity)
+form NewLine = {
+  quantity: Int.Positive,
+}
 
+msg AddLine { product: Product.ProductId } form NewLine
+
 update : Msg -> Model -> Session -> Task Nav
 update msg model session = match msg {
   AddLine p -> ?addLine,
@@ -38,7 +43,15 @@
 view model session = <Layout.Page title="Order">
   <h1>Order</h1>
 
-  <al-button click={AddLine { product: ?addLineProduct }}>Add line</al-button>
+
+  <al-card>
+    <h2 slot="header">New line</h2>
+    <form submit={AddLine { product: ?addLineProduct }}>
+      <al-input field="quantity" label="Quantity" />
+      <al-alert variant="destructive"><Errors form /></al-alert>
+      <al-button type="submit">New line</al-button>
+    </form>
+  </al-card>
 </Layout.Page>
 
 test "a visitor is sent to log in" = {
@@ -48,16 +61,11 @@
   Task.succeed (r.answer == Redirected (Auth.Login {}))
 }
 
--- Fill ?expected with what pressing AddLine must do, so that a test fails when the arm
--- does something else: its answer (`r.answer`: Rerendered, Redirected …) and what it
--- changed (`after`, the row read again), as `{ answer: r.answer, … } == { answer: Rerendered, … }`.
-test "pressing AddLine" = {
+test "submitting NewLine" = {
   let me <- Db.insert User { email: Text.Email "email-1@example.com", name: Text.NonEmpty "name-1", passwordHash: "passwordhash-1" }
   let _ <- Test.login { user: Just me }
   let order1 <- Db.insert Order { owner: me }
-  let product1 <- Db.insert Product { name: Text.NonEmpty "name-1", price: Int.NonNegative 1 }
   let _ <- Test.get (Orders.Show { id: order1 })
-  let r <- Test.click (AddLine { product: product1 })
-  let after <- Db.get Product product1
-  Task.succeed ?expected
+  let r <- Test.submit (AddLine { product: ?addLineProduct }) { quantity: Int.Positive 1 }
+  Task.succeed (r.answer == ?expected)
 }
app/Orders/Show.alder:39:16: warning[W0002]: hole ?addLine has type Task Nav
    AddLine p -> ?addLine,
                 ^^^^^^^^
app/Orders/Show.alder:49:38: warning[W0002]: hole ?addLineProduct has type Product.ProductId
      <form submit={AddLine { product: ?addLineProduct }}>
                                       ^^^^^^^^^^^^^^^
app/Orders/Show.alder:69:44: warning[W0002]: hole ?addLineProduct has type Product.ProductId
    let r <- Test.submit (AddLine { product: ?addLineProduct }) { quantity: Int.Positive 1 }
                                             ^^^^^^^^^^^^^^^
app/Orders/Show.alder:70:29: warning[W0002]: hole ?expected has type Answer
    Task.succeed (r.answer == ?expected)
                              ^^^^^^^^^
[exit 0]
$ alder new msg Orders.Show Pay
edited app/Orders/Show.alder
--- a/app/Orders/Show.alder
+++ b/app/Orders/Show.alder
@@ -34,9 +34,12 @@
 
 msg AddLine { product: Product.ProductId } form NewLine
 
+msg Pay
+
 update : Msg -> Model -> Session -> Task Nav
 update msg model session = match msg {
   AddLine p -> ?addLine,
+  Pay p -> ?pay,
 }
 
 view : Model -> Session -> Html Msg
@@ -52,6 +55,8 @@
       <al-button type="submit">New line</al-button>
     </form>
   </al-card>
+
+  <al-button click={Pay}>Pay</al-button>
 </Layout.Page>
 
 test "a visitor is sent to log in" = {
@@ -69,3 +74,18 @@
   let r <- Test.submit (AddLine { product: ?addLineProduct }) { quantity: Int.Positive 1 }
   Task.succeed (r.answer == ?expected)
 }
+
+-- Fill ?expected with what pressing Pay must do, so that a test fails when the arm
+-- does something else: its answer (`r.answer`: Rerendered, Redirected …) and what it
+-- changed (`after`: the page opened again, `after.answer` Shown, or PageMissing when the arm
+-- deleted the page's row, and `after.doc` its HTML), as
+-- `{ answer: r.answer, … } == { answer: Rerendered, … }`.
+test "pressing Pay" = {
+  let me <- Db.insert User { email: Text.Email "email-1@example.com", name: Text.NonEmpty "name-1", passwordHash: "passwordhash-1" }
+  let _ <- Test.login { user: Just me }
+  let order1 <- Db.insert Order { owner: me }
+  let _ <- Test.get (Orders.Show { id: order1 })
+  let r <- Test.click Pay
+  let after <- Test.open (Orders.Show { id: order1 })
+  Task.succeed ?expected
+}
app/Orders/Show.alder:41:16: warning[W0002]: hole ?addLine has type Task Nav
    AddLine p -> ?addLine,
                 ^^^^^^^^
app/Orders/Show.alder:42:12: warning[W0002]: hole ?pay has type Task Nav
    Pay p -> ?pay,
             ^^^^
app/Orders/Show.alder:52:38: warning[W0002]: hole ?addLineProduct has type Product.ProductId
      <form submit={AddLine { product: ?addLineProduct }}>
                                       ^^^^^^^^^^^^^^^
app/Orders/Show.alder:74:44: warning[W0002]: hole ?addLineProduct has type Product.ProductId
    let r <- Test.submit (AddLine { product: ?addLineProduct }) { quantity: Int.Positive 1 }
                                             ^^^^^^^^^^^^^^^
app/Orders/Show.alder:75:29: warning[W0002]: hole ?expected has type Answer
    Task.succeed (r.answer == ?expected)
                              ^^^^^^^^^
app/Orders/Show.alder:90:16: warning[W0002]: hole ?expected has type Bool
    Task.succeed ?expected
                 ^^^^^^^^^
[exit 0]
$ alder new msg Orders.Show Cancel
edited app/Orders/Show.alder
--- a/app/Orders/Show.alder
+++ b/app/Orders/Show.alder
@@ -36,10 +36,13 @@
 
 msg Pay
 
+msg Cancel
+
 update : Msg -> Model -> Session -> Task Nav
 update msg model session = match msg {
   AddLine p -> ?addLine,
   Pay p -> ?pay,
+  Cancel p -> ?cancel,
 }
 
 view : Model -> Session -> Html Msg
@@ -57,6 +60,8 @@
   </al-card>
 
   <al-button click={Pay}>Pay</al-button>
+
+  <al-button click={Cancel}>Cancel</al-button>
 </Layout.Page>
 
 test "a visitor is sent to log in" = {
@@ -89,3 +94,18 @@
   let after <- Test.open (Orders.Show { id: order1 })
   Task.succeed ?expected
 }
+
+-- Fill ?expected with what pressing Cancel must do, so that a test fails when the arm
+-- does something else: its answer (`r.answer`: Rerendered, Redirected …) and what it
+-- changed (`after`: the page opened again, `after.answer` Shown, or PageMissing when the arm
+-- deleted the page's row, and `after.doc` its HTML), as
+-- `{ answer: r.answer, … } == { answer: Rerendered, … }`.
+test "pressing Cancel" = {
+  let me <- Db.insert User { email: Text.Email "email-1@example.com", name: Text.NonEmpty "name-1", passwordHash: "passwordhash-1" }
+  let _ <- Test.login { user: Just me }
+  let order1 <- Db.insert Order { owner: me }
+  let _ <- Test.get (Orders.Show { id: order1 })
+  let r <- Test.click Cancel
+  let after <- Test.open (Orders.Show { id: order1 })
+  Task.succeed ?expected
+}
app/Orders/Show.alder:43:16: warning[W0002]: hole ?addLine has type Task Nav
    AddLine p -> ?addLine,
                 ^^^^^^^^
app/Orders/Show.alder:44:12: warning[W0002]: hole ?pay has type Task Nav
    Pay p -> ?pay,
             ^^^^
app/Orders/Show.alder:45:15: warning[W0002]: hole ?cancel has type Task Nav
    Cancel p -> ?cancel,
                ^^^^^^^
app/Orders/Show.alder:55:38: warning[W0002]: hole ?addLineProduct has type Product.ProductId
      <form submit={AddLine { product: ?addLineProduct }}>
                                       ^^^^^^^^^^^^^^^
app/Orders/Show.alder:79:44: warning[W0002]: hole ?addLineProduct has type Product.ProductId
    let r <- Test.submit (AddLine { product: ?addLineProduct }) { quantity: Int.Positive 1 }
                                             ^^^^^^^^^^^^^^^
app/Orders/Show.alder:80:29: warning[W0002]: hole ?expected has type Answer
    Task.succeed (r.answer == ?expected)
                              ^^^^^^^^^
app/Orders/Show.alder:95:16: warning[W0002]: hole ?expected has type Bool
    Task.succeed ?expected
                 ^^^^^^^^^
app/Orders/Show.alder:110:16: warning[W0002]: hole ?expected has type Bool
    Task.succeed ?expected
                 ^^^^^^^^^
[exit 0]
$ alder fill --list
Orders.Index update.Create ?create : Task Nav
Orders.Index "pressing Create" ?expected : Bool
Orders.Show update.AddLine ?addLine : Task Nav
Orders.Show update.Pay ?pay : Task Nav
Orders.Show update.Cancel ?cancel : Task Nav
Orders.Show view ?addLineProduct : Product.ProductId
Orders.Show "submitting NewLine" ?addLineProduct : Product.ProductId
Orders.Show "submitting NewLine" ?expected : Answer
Orders.Show "pressing Pay" ?expected : Bool
Orders.Show "pressing Cancel" ?expected : Bool
[exit 0]
```

The program type-checks but `alder build` refused it; fix the cause below.

```text
app/Orders/Index.alder:23:15: warning[W0002]: hole ?create has type Task Nav
    Create p -> ?create,
                ^^^^^^^
app/Orders/Index.alder:59:16: warning[W0002]: hole ?expected has type Bool
    Task.succeed ?expected
                 ^^^^^^^^^
app/Orders/Show.alder:43:16: warning[W0002]: hole ?addLine has type Task Nav
    AddLine p -> ?addLine,
                 ^^^^^^^^
app/Orders/Show.alder:44:12: warning[W0002]: hole ?pay has type Task Nav
    Pay p -> ?pay,
             ^^^^
app/Orders/Show.alder:45:15: warning[W0002]: hole ?cancel has type Task Nav
    Cancel p -> ?cancel,
                ^^^^^^^
app/Orders/Show.alder:55:38: warning[W0002]: hole ?addLineProduct has type Product.ProductId
      <form submit={AddLine { product: ?addLineProduct }}>
                                       ^^^^^^^^^^^^^^^
app/Orders/Show.alder:79:44: warning[W0002]: hole ?addLineProduct has type Product.ProductId
    let r <- Test.submit (AddLine { product: ?addLineProduct }) { quantity: Int.Positive 1 }
                                             ^^^^^^^^^^^^^^^
app/Orders/Show.alder:80:29: warning[W0002]: hole ?expected has type Answer
    Task.succeed (r.answer == ?expected)
                              ^^^^^^^^^
app/Orders/Show.alder:95:16: warning[W0002]: hole ?expected has type Bool
    Task.succeed ?expected
                 ^^^^^^^^^
app/Orders/Show.alder:110:16: warning[W0002]: hole ?expected has type Bool
    Task.succeed ?expected
                 ^^^^^^^^^
alder build: the program has 10 typed hole(s); fill them before building
```
