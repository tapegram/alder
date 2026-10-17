### Round 1

```text
$ alder fill Auth.Login.update.Submit - <<'EOF'
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
$ alder fill 'Auth.Login."logging in through the form"' - <<'EOF'
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
$ alder new page Orders.Show /orders/{id} --param id:Order.OrderId --requires user --blank
wrote app/Orders/Show.alder
app/Orders/Show.alder:11:23: warning[W0002]: hole ?load has type Query Orders.Show.Model
  load params session = ?load
                        ^^^^^
app/Orders/Show.alder:14:22: warning[W0002]: hole ?view has type Html Orders.Show.Msg
  view model session = ?view
                       ^^^^^
[exit 0]
$ alder fill --decl Orders.Show.Model - <<'EOF'
{ order: Order.Order, lines: List Line.Line, products: List Product.Product }
EOF
edited app/Orders/Show.alder
--- a/app/Orders/Show.alder
+++ b/app/Orders/Show.alder
@@ -1,11 +1,13 @@
 module Orders.Show
 
+import Line
 import Order
+import Product
 import User
 
 page "/orders/{id}" { id: Order.OrderId } requires user
 
-type Model = { }
+type Model = { order: Order.Order, lines: List Line.Line, products: List Product.Product }
 
 load : Params -> Session -> Query Model
 load params session = ?load
app/Orders/Show.alder:13:23: warning[W0002]: hole ?load has type Query Orders.Show.Model
  load params session = ?load
                        ^^^^^
app/Orders/Show.alder:16:22: warning[W0002]: hole ?view has type Html Orders.Show.Msg
  view model session = ?view
                       ^^^^^
added import Line to app/Orders/Show.alder
added import Product to app/Orders/Show.alder
[exit 0]
$ alder fill Orders.Show.load - <<'EOF'
{
  let found <- Db.get Order params.id
  match found {
    Nothing -> Query.notFound,
    Just o -> {
      let manager <- User.isManager session.user
      if o.owner == session.user || manager then {
        let lines <- Db.from Line |> Db.where (Line.order.eq o.id) |> Db.all
        let products <- Db.from Product |> Db.all
        Query.succeed { order: o, lines: lines, products: products }
      } else Query.notFound
    },
  }
}
EOF
edited app/Orders/Show.alder
--- a/app/Orders/Show.alder
+++ b/app/Orders/Show.alder
@@ -10,7 +10,20 @@
 type Model = { order: Order.Order, lines: List Line.Line, products: List Product.Product }
 
 load : Params -> Session -> Query Model
-load params session = ?load
+load params session = {
+  let found <- Db.get Order params.id
+  match found {
+    Nothing -> Query.notFound,
+    Just o -> {
+      let manager <- User.isManager session.user
+      if o.owner == session.user || manager then {
+        let lines <- Db.from Line |> Db.where (Line.order.eq o.id) |> Db.all
+        let products <- Db.from Product |> Db.all
+        Query.succeed { order: o, lines: lines, products: products }
+      } else Query.notFound
+    },
+  }
+}
 
 view : Model -> Session -> Html Msg
 view model session = ?view
app/Orders/Show.alder:29:22: warning[W0002]: hole ?view has type Html Orders.Show.Msg
  view model session = ?view
                       ^^^^^
[exit 0]
$ alder fill --decl Orders.Show.view - <<'EOF'
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
@@ -26,7 +27,10 @@
 }
 
 view : Model -> Session -> Html Msg
-view model session = ?view
+view model session =
+  <Layout.Page title="Order">
+    <h1>Order</h1>
+  </Layout.Page>
 
 test "a visitor is sent to log in" = {
   let me <- Db.insert User { email: Text.Email "email-1@example.com", name: Text.NonEmpty "name-1", passwordHash: "passwordhash-1" }
added import Layout to app/Orders/Show.alder
[exit 0]
$ alder fill --decl Orders.Index.view - <<'EOF'
<Layout.Page title="Orders">
  <h1>Orders</h1>
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
@@ -17,7 +18,10 @@
 }
 
 view : Model -> Session -> Html Msg
-view model session = <p>Orders</p>
+view model session =
+  <Layout.Page title="Orders">
+    <h1>Orders</h1>
+  </Layout.Page>
 
 test "a visitor is sent to log in" = {
   let r <- Test.open (Orders.Index {})
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
 
@@ -17,13 +18,36 @@
   Query.succeed { orders: orders, lines: lines, products: products }
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
     <h1>Orders</h1>
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
app/Orders/Index.alder:25:15: warning[W0002]: hole ?create has type Task Nav
    Create p -> ?create,
                ^^^^^^^
app/Orders/Index.alder:52:16: warning[W0002]: hole ?expected has type Bool
    Task.succeed ?expected
                 ^^^^^^^^^
[exit 0]
$ alder fill --decl Orders.Index.update - <<'EOF'
match msg {
  Create p -> {
    let id <- Db.insert Order { owner: session.user }
    Task.succeed (Redirect (Orders.Show { id: id }))
  },
}
EOF
edited app/Orders/Index.alder
--- a/app/Orders/Index.alder
+++ b/app/Orders/Index.alder
@@ -22,7 +22,10 @@
 
 update : Msg -> Model -> Session -> Task Nav
 update msg model session = match msg {
-  Create p -> ?create,
+  Create p -> {
+    let id <- Db.insert Order { owner: session.user }
+    Task.succeed (Redirect (Orders.Show { id: id }))
+  },
 }
 
 view : Model -> Session -> Html Msg
app/Orders/Index.alder:55:16: warning[W0002]: hole ?expected has type Bool
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
        <span class="status">{Order.statusLabel o.status}</span>
        <span class="total">{Text.fromInt (Line.total model.products (List.filter (\l -> l.order == o.id) model.lines))}</span>
      </li>}
  </ul>
  <button click={App.Logout}>Log out</button>
</Layout.Page>
EOF
edited app/Orders/Index.alder
--- a/app/Orders/Index.alder
+++ b/app/Orders/Index.alder
@@ -32,8 +32,16 @@
 view model session =
   <Layout.Page title="Orders">
     <h1>Orders</h1>
-
-    <al-button click={Create}>Create</al-button>
+    <button click={Create}>New order</button>
+    <ul id="orders">
+      {for o in model.orders ->
+        <li>
+          <a href={Orders.Show { id: o.id }}>Order</a>
+          <span class="status">{Order.statusLabel o.status}</span>
+          <span class="total">{Text.fromInt (Line.total model.products (List.filter (\l -> l.order == o.id) model.lines))}</span>
+        </li>}
+    </ul>
+    <button click={App.Logout}>Log out</button>
   </Layout.Page>
 
 test "a visitor is sent to log in" = {
app/Orders/Index.alder:63:16: warning[W0002]: hole ?expected has type Bool
    Task.succeed ?expected
                 ^^^^^^^^^
[exit 0]
$ alder new msg Orders.Show AddLine product:Product.ProductId
edited app/Orders/Show.alder
--- a/app/Orders/Show.alder
+++ b/app/Orders/Show.alder
@@ -26,10 +26,19 @@
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
 view model session =
   <Layout.Page title="Order">
     <h1>Order</h1>
+
+    <al-button click={AddLine { product: ?addLineProduct }}>Add line</al-button>
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
app/Orders/Show.alder:33:16: warning[W0002]: hole ?addLine has type Task Nav
    AddLine p -> ?addLine,
                 ^^^^^^^^
app/Orders/Show.alder:41:42: warning[W0002]: hole ?addLineProduct has type Product.ProductId
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
@@ -26,8 +26,13 @@
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
   <Layout.Page title="Order">
     <h1>Order</h1>
 
-    <al-button click={AddLine { product: ?addLineProduct }}>Add line</al-button>
+
+    <al-card>
+      <h2 slot="header">New line</h2>
+      <form submit={AddLine { product: ?addLineProduct }}>
+        <al-input field="quantity" label="Quantity" />
+        <al-alert variant="destructive"><Errors form /></al-alert>
+        <al-button type="submit">New line</al-button>
+      </form>
+    </al-card>
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
app/Orders/Show.alder:38:16: warning[W0002]: hole ?addLine has type Task Nav
    AddLine p -> ?addLine,
                 ^^^^^^^^
app/Orders/Show.alder:49:40: warning[W0002]: hole ?addLineProduct has type Product.ProductId
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
@@ -33,9 +33,12 @@
 
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
+    <al-button click={Pay}>Pay</al-button>
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
app/Orders/Show.alder:40:16: warning[W0002]: hole ?addLine has type Task Nav
    AddLine p -> ?addLine,
                 ^^^^^^^^
app/Orders/Show.alder:41:12: warning[W0002]: hole ?pay has type Task Nav
    Pay p -> ?pay,
             ^^^^
app/Orders/Show.alder:52:40: warning[W0002]: hole ?addLineProduct has type Product.ProductId
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
@@ -35,10 +35,13 @@
 
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
+    <al-button click={Cancel}>Cancel</al-button>
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
app/Orders/Show.alder:42:16: warning[W0002]: hole ?addLine has type Task Nav
    AddLine p -> ?addLine,
                 ^^^^^^^^
app/Orders/Show.alder:43:12: warning[W0002]: hole ?pay has type Task Nav
    Pay p -> ?pay,
             ^^^^
app/Orders/Show.alder:44:15: warning[W0002]: hole ?cancel has type Task Nav
    Cancel p -> ?cancel,
                ^^^^^^^
app/Orders/Show.alder:55:40: warning[W0002]: hole ?addLineProduct has type Product.ProductId
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
$ alder fill --decl Orders.Show.update - <<'EOF'
match msg {
  AddLine p -> if model.order.status != Open then Task.succeed (Reject "This order is not open.") else if List.any (\x -> x.id == p.product && x.active) model.products then {
    let _ <- Db.insert Line { order: model.order.id, product: p.product, quantity: p.form.quantity }
    Task.succeed (Redirect (Orders.Show { id: model.order.id }))
  } else Task.succeed (Reject "This product is not available."),
  Pay p -> if model.order.status != Open then Task.succeed (Reject "This order is not open.") else {
    let _ <- Db.update Order model.order.id (\o -> { o | status: Paid })
    Task.succeed (Redirect (Orders.Show { id: model.order.id }))
  },
  Cancel p -> if model.order.status != Open then Task.succeed (Reject "This order is not open.") else {
    let _ <- Db.update Order model.order.id (\o -> { o | status: Cancelled })
    Task.succeed (Redirect (Orders.Show { id: model.order.id }))
  },
}
EOF
edited app/Orders/Show.alder
--- a/app/Orders/Show.alder
+++ b/app/Orders/Show.alder
@@ -39,9 +39,18 @@
 
 update : Msg -> Model -> Session -> Task Nav
 update msg model session = match msg {
-  AddLine p -> ?addLine,
-  Pay p -> ?pay,
-  Cancel p -> ?cancel,
+  AddLine p -> if model.order.status != Open then Task.succeed (Reject "This order is not open.") else if List.any (\x -> x.id == p.product && x.active) model.products then {
+    let _ <- Db.insert Line { order: model.order.id, product: p.product, quantity: p.form.quantity }
+    Task.succeed (Redirect (Orders.Show { id: model.order.id }))
+  } else Task.succeed (Reject "This product is not available."),
+  Pay p -> if model.order.status != Open then Task.succeed (Reject "This order is not open.") else {
+    let _ <- Db.update Order model.order.id (\o -> { o | status: Paid })
+    Task.succeed (Redirect (Orders.Show { id: model.order.id }))
+  },
+  Cancel p -> if model.order.status != Open then Task.succeed (Reject "This order is not open.") else {
+    let _ <- Db.update Order model.order.id (\o -> { o | status: Cancelled })
+    Task.succeed (Redirect (Orders.Show { id: model.order.id }))
+  },
 }
 
 view : Model -> Session -> Html Msg
app/Orders/Show.alder:64:40: warning[W0002]: hole ?addLineProduct has type Product.ProductId
        <form submit={AddLine { product: ?addLineProduct }}>
                                         ^^^^^^^^^^^^^^^
app/Orders/Show.alder:88:44: warning[W0002]: hole ?addLineProduct has type Product.ProductId
    let r <- Test.submit (AddLine { product: ?addLineProduct }) { quantity: Int.Positive 1 }
                                             ^^^^^^^^^^^^^^^
app/Orders/Show.alder:89:29: warning[W0002]: hole ?expected has type Answer
    Task.succeed (r.answer == ?expected)
                              ^^^^^^^^^
app/Orders/Show.alder:104:16: warning[W0002]: hole ?expected has type Bool
    Task.succeed ?expected
                 ^^^^^^^^^
app/Orders/Show.alder:119:16: warning[W0002]: hole ?expected has type Bool
    Task.succeed ?expected
                 ^^^^^^^^^
[exit 0]
$ alder fill --decl Orders.Show.view - <<'EOF'
<Layout.Page title="Order">
  <h1>Order</h1>
  <p id="status">{Order.statusLabel model.order.status}</p>
  <ul id="lines">
    {for l in model.lines ->
      <li>
        {match List.find (\x -> x.id == l.product) model.products {
          Just x -> <span class="product">{x.name}</span>,
          Nothing -> Html.none,
        }}
        <span class="quantity">{Text.fromInt l.quantity}</span>
      </li>}
  </ul>
  <p id="total">{Text.fromInt (Line.total model.products model.lines)}</p>
  {if model.order.status == Open then
    <div id="actions">
      {for prod in List.filter (\x -> x.active) model.products ->
        <form submit={AddLine { product: prod.id }}>
          <Errors form />
          <span>{prod.name}</span>
          <Input field="quantity" label="Quantity" />
          <button>Add</button>
        </form>}
      <button click={Pay}>Mark paid</button>
      <button click={Cancel}>Cancel order</button>
    </div>
  else Html.none}
  <a href={Orders.Index {}}>Back</a>
</Layout.Page>
EOF
edited app/Orders/Show.alder
--- a/app/Orders/Show.alder
+++ b/app/Orders/Show.alder
@@ -57,20 +57,32 @@
 view model session =
   <Layout.Page title="Order">
     <h1>Order</h1>
-
-
-    <al-card>
-      <h2 slot="header">New line</h2>
-      <form submit={AddLine { product: ?addLineProduct }}>
-        <al-input field="quantity" label="Quantity" />
-        <al-alert variant="destructive"><Errors form /></al-alert>
-        <al-button type="submit">New line</al-button>
-      </form>
-    </al-card>
-
-    <al-button click={Pay}>Pay</al-button>
-
-    <al-button click={Cancel}>Cancel</al-button>
+    <p id="status">{Order.statusLabel model.order.status}</p>
+    <ul id="lines">
+      {for l in model.lines ->
+        <li>
+          {match List.find (\x -> x.id == l.product) model.products {
+            Just x -> <span class="product">{x.name}</span>,
+            Nothing -> Html.none,
+          }}
+          <span class="quantity">{Text.fromInt l.quantity}</span>
+        </li>}
+    </ul>
+    <p id="total">{Text.fromInt (Line.total model.products model.lines)}</p>
+    {if model.order.status == Open then
+      <div id="actions">
+        {for prod in List.filter (\x -> x.active) model.products ->
+          <form submit={AddLine { product: prod.id }}>
+            <Errors form />
+            <span>{prod.name}</span>
+            <Input field="quantity" label="Quantity" />
+            <button>Add</button>
+          </form>}
+        <button click={Pay}>Mark paid</button>
+        <button click={Cancel}>Cancel order</button>
+      </div>
+    else Html.none}
+    <a href={Orders.Index {}}>Back</a>
   </Layout.Page>
 
 test "a visitor is sent to log in" = {
app/Orders/Show.alder:100:44: warning[W0002]: hole ?addLineProduct has type Product.ProductId
    let r <- Test.submit (AddLine { product: ?addLineProduct }) { quantity: Int.Positive 1 }
                                             ^^^^^^^^^^^^^^^
app/Orders/Show.alder:101:29: warning[W0002]: hole ?expected has type Answer
    Task.succeed (r.answer == ?expected)
                              ^^^^^^^^^
app/Orders/Show.alder:116:16: warning[W0002]: hole ?expected has type Bool
    Task.succeed ?expected
                 ^^^^^^^^^
app/Orders/Show.alder:131:16: warning[W0002]: hole ?expected has type Bool
    Task.succeed ?expected
                 ^^^^^^^^^
[exit 0]
$ alder new page Products.Index /products --requires user --blank
wrote app/Products/Index.alder
app/Products/Index.alder:8:23: warning[W0002]: hole ?load has type Query Products.Index.Model
  load params session = ?load
                        ^^^^^
app/Products/Index.alder:11:22: warning[W0002]: hole ?view has type Html Products.Index.Msg
  view model session = ?view
                       ^^^^^
[exit 0]
$ alder fill --decl Products.Index.Model - <<'EOF'
{ products: List Product.Product }
EOF
edited app/Products/Index.alder
--- a/app/Products/Index.alder
+++ b/app/Products/Index.alder
@@ -1,8 +1,10 @@
 module Products.Index
 
+import Product
+
 page "/products" { } requires user
 
-type Model = { }
+type Model = { products: List Product.Product }
 
 load : Params -> Session -> Query Model
 load params session = ?load
app/Products/Index.alder:10:23: warning[W0002]: hole ?load has type Query Products.Index.Model
  load params session = ?load
                        ^^^^^
app/Products/Index.alder:13:22: warning[W0002]: hole ?view has type Html Products.Index.Msg
  view model session = ?view
                       ^^^^^
added import Product to app/Products/Index.alder
[exit 0]
$ alder fill Products.Index.load - <<'EOF'
{
  let manager <- User.isManager session.user
  if manager then {
    let products <- Db.from Product |> Db.orderBy Product.name.asc |> Db.all
    Query.succeed { products: products }
  } else Query.notFound
}
EOF
edited app/Products/Index.alder
--- a/app/Products/Index.alder
+++ b/app/Products/Index.alder
@@ -1,13 +1,20 @@
 module Products.Index
 
 import Product
+import User
 
 page "/products" { } requires user
 
 type Model = { products: List Product.Product }
 
 load : Params -> Session -> Query Model
-load params session = ?load
+load params session = {
+  let manager <- User.isManager session.user
+  if manager then {
+    let products <- Db.from Product |> Db.orderBy Product.name.asc |> Db.all
+    Query.succeed { products: products }
+  } else Query.notFound
+}
 
 view : Model -> Session -> Html Msg
 view model session = ?view
app/Products/Index.alder:20:22: warning[W0002]: hole ?view has type Html Products.Index.Msg
  view model session = ?view
                       ^^^^^
added import User to app/Products/Index.alder
[exit 0]
$ alder fill --decl Products.Index.view - <<'EOF'
<Layout.Page title="Products">
  <h1>Products</h1>
</Layout.Page>
EOF
edited app/Products/Index.alder
--- a/app/Products/Index.alder
+++ b/app/Products/Index.alder
@@ -1,5 +1,6 @@
 module Products.Index
 
+import Layout
 import Product
 import User
 
@@ -17,7 +18,10 @@
 }
 
 view : Model -> Session -> Html Msg
-view model session = ?view
+view model session =
+  <Layout.Page title="Products">
+    <h1>Products</h1>
+  </Layout.Page>
 
 test "a visitor is sent to log in" = {
   let r <- Test.open (Products.Index {})
added import Layout to app/Products/Index.alder
[exit 0]
$ alder new form Products.Index NewProduct name:Text.NonEmpty price:Int.NonNegative --msg Add
edited app/Products/Index.alder
--- a/app/Products/Index.alder
+++ b/app/Products/Index.alder
@@ -17,13 +17,56 @@
   } else Query.notFound
 }
 
+form NewProduct = {
+  name: Text.NonEmpty,
+  price: Int.NonNegative,
+}
+
+msg Add form NewProduct
+
+update : Msg -> Model -> Session -> Task Nav
+update msg model session = match msg {
+  Add p -> {
+    let _ <- Db.insert Product { name: p.form.name, price: p.form.price }
+    Task.succeed (Redirect (Products.Index {}))
+  },
+}
+
 view : Model -> Session -> Html Msg
 view model session =
   <Layout.Page title="Products">
     <h1>Products</h1>
+
+    <al-card>
+      <h2 slot="header">New product</h2>
+      <form submit={Add}>
+        <al-input field="name" label="Name" />
+        <al-input field="price" label="Price" />
+        <al-alert variant="destructive"><Errors form /></al-alert>
+        <al-button type="submit">New product</al-button>
+      </form>
+    </al-card>
   </Layout.Page>
 
 test "a visitor is sent to log in" = {
   let r <- Test.open (Products.Index {})
   Task.succeed (r.answer == Redirected (Auth.Login {}))
 }
+
+test "submitting NewProduct adds a row" = {
+  let me <- Db.insert User { email: Text.Email "email-1@example.com", name: Text.NonEmpty "name-1", passwordHash: "passwordhash-1" }
+  let _ <- Test.login { user: Just me }
+  let _ <- Test.get (Products.Index {})
+  let r <- Test.submit NewProduct { name: Text.NonEmpty "name-1", price: Int.NonNegative 1 }
+  let rows <- Db.from Product |> Db.all
+  Task.succeed ({ answer: r.answer, rows: List.map (\row -> { name: row.name, price: row.price, active: row.active }) rows } == { answer: Redirected (Products.Index {}), rows: [{ name: Text.NonEmpty "name-1", price: Int.NonNegative 1, active: True }] })
+}
+
+test "NewProduct refuses an empty name" = {
+  let me <- Db.insert User { email: Text.Email "email-1@example.com", name: Text.NonEmpty "name-1", passwordHash: "passwordhash-1" }
+  let _ <- Test.login { user: Just me }
+  let _ <- Test.get (Products.Index {})
+  let r <- Test.submitRaw NewProduct [{ name: "name", value: "" }, { name: "price", value: "1" }]
+  let rows <- Db.from Product |> Db.count
+  Task.succeed ({ answer: r.answer, rows: rows } == { answer: FormRejected [{ field: "name", message: "must not be empty" }], rows: 0 })
+}
[exit 0]
$ alder new msg Products.Index Toggle id:Product.ProductId
edited app/Products/Index.alder
--- a/app/Products/Index.alder
+++ b/app/Products/Index.alder
@@ -24,12 +24,15 @@
 
 msg Add form NewProduct
 
+msg Toggle { id: Product.ProductId }
+
 update : Msg -> Model -> Session -> Task Nav
 update msg model session = match msg {
   Add p -> {
     let _ <- Db.insert Product { name: p.form.name, price: p.form.price }
     Task.succeed (Redirect (Products.Index {}))
   },
+  Toggle p -> ?toggle,
 }
 
 view : Model -> Session -> Html Msg
@@ -46,6 +49,8 @@
         <al-button type="submit">New product</al-button>
       </form>
     </al-card>
+
+    <al-button click={Toggle { id: ?toggleId }}>Toggle</al-button>
   </Layout.Page>
 
 test "a visitor is sent to log in" = {
@@ -70,3 +75,16 @@
   let rows <- Db.from Product |> Db.count
   Task.succeed ({ answer: r.answer, rows: rows } == { answer: FormRejected [{ field: "name", message: "must not be empty" }], rows: 0 })
 }
+
+-- Fill ?expected with what pressing Toggle must do, so that a test fails when the arm
+-- does something else: its answer (`r.answer`: Rerendered, Redirected …) and what it
+-- changed (`after`, the row read again), as `{ answer: r.answer, … } == { answer: Rerendered, … }`.
+test "pressing Toggle" = {
+  let me <- Db.insert User { email: Text.Email "email-1@example.com", name: Text.NonEmpty "name-1", passwordHash: "passwordhash-1" }
+  let _ <- Test.login { user: Just me }
+  let product1 <- Db.insert Product { name: Text.NonEmpty "name-1", price: Int.NonNegative 1 }
+  let _ <- Test.get (Products.Index {})
+  let r <- Test.click (Toggle { id: product1 })
+  let after <- Db.get Product product1
+  Task.succeed ?expected
+}
app/Products/Index.alder:35:15: warning[W0002]: hole ?toggle has type Task Nav
    Toggle p -> ?toggle,
                ^^^^^^^
app/Products/Index.alder:53:36: warning[W0002]: hole ?toggleId has type Product.ProductId
      <al-button click={Toggle { id: ?toggleId }}>Toggle</al-button>
                                     ^^^^^^^^^
app/Products/Index.alder:89:16: warning[W0002]: hole ?expected has type Bool
    Task.succeed ?expected
                 ^^^^^^^^^
[exit 0]
$ alder fill --decl Products.Index.update - <<'EOF'
match msg {
  Add p -> {
    let _ <- Db.insert Product { name: p.form.name, price: p.form.price }
    Task.succeed (Redirect (Products.Index {}))
  },
  Toggle p -> {
    let _ <- Db.update Product p.id (\x -> { x | active: not x.active })
    Task.succeed (Redirect (Products.Index {}))
  },
}
EOF
edited app/Products/Index.alder
--- a/app/Products/Index.alder
+++ b/app/Products/Index.alder
@@ -32,7 +32,10 @@
     let _ <- Db.insert Product { name: p.form.name, price: p.form.price }
     Task.succeed (Redirect (Products.Index {}))
   },
-  Toggle p -> ?toggle,
+  Toggle p -> {
+    let _ <- Db.update Product p.id (\x -> { x | active: not x.active })
+    Task.succeed (Redirect (Products.Index {}))
+  },
 }
 
 view : Model -> Session -> Html Msg
app/Products/Index.alder:56:36: warning[W0002]: hole ?toggleId has type Product.ProductId
      <al-button click={Toggle { id: ?toggleId }}>Toggle</al-button>
                                     ^^^^^^^^^
app/Products/Index.alder:92:16: warning[W0002]: hole ?expected has type Bool
    Task.succeed ?expected
                 ^^^^^^^^^
[exit 0]
$ alder fill --decl Products.Index.view - <<'EOF'
<Layout.Page title="Products">
  <h1>Products</h1>
  <form submit={Add}>
    <Errors form />
    <Input field="name" label="Name" />
    <Input field="price" label="Price (cents)" />
    <button>Add product</button>
  </form>
  <ul id="products">
    {for prod in model.products ->
      <li>
        <span class="name">{prod.name}</span>
        <span class="price">{Text.fromInt prod.price}</span>
        <button click={Toggle { id: prod.id }}>{if prod.active then "Deactivate" else "Activate"}</button>
      </li>}
  </ul>
  <a href={Orders.Index {}}>Orders</a>
</Layout.Page>
EOF
edited app/Products/Index.alder
--- a/app/Products/Index.alder
+++ b/app/Products/Index.alder
@@ -42,18 +42,21 @@
 view model session =
   <Layout.Page title="Products">
     <h1>Products</h1>
-
-    <al-card>
-      <h2 slot="header">New product</h2>
-      <form submit={Add}>
-        <al-input field="name" label="Name" />
-        <al-input field="price" label="Price" />
-        <al-alert variant="destructive"><Errors form /></al-alert>
-        <al-button type="submit">New product</al-button>
-      </form>
-    </al-card>
-
-    <al-button click={Toggle { id: ?toggleId }}>Toggle</al-button>
+    <form submit={Add}>
+      <Errors form />
+      <Input field="name" label="Name" />
+      <Input field="price" label="Price (cents)" />
+      <button>Add product</button>
+    </form>
+    <ul id="products">
+      {for prod in model.products ->
+        <li>
+          <span class="name">{prod.name}</span>
+          <span class="price">{Text.fromInt prod.price}</span>
+          <button click={Toggle { id: prod.id }}>{if prod.active then "Deactivate" else "Activate"}</button>
+        </li>}
+    </ul>
+    <a href={Orders.Index {}}>Orders</a>
   </Layout.Page>
 
 test "a visitor is sent to log in" = {
app/Products/Index.alder:95:16: warning[W0002]: hole ?expected has type Bool
    Task.succeed ?expected
                 ^^^^^^^^^
[exit 0]
$ alder new test Orders.Show "an Open order's total is the sum of its lines"
edited app/Orders/Show.alder
--- a/app/Orders/Show.alder
+++ b/app/Orders/Show.alder
@@ -130,3 +130,7 @@
   let after <- Test.open (Orders.Show { id: order1 })
   Task.succeed ?expected
 }
+
+test "an Open order's total is the sum of its lines" = {
+  ?test
+}
app/Orders/Show.alder:100:44: warning[W0002]: hole ?addLineProduct has type Product.ProductId
    let r <- Test.submit (AddLine { product: ?addLineProduct }) { quantity: Int.Positive 1 }
                                             ^^^^^^^^^^^^^^^
app/Orders/Show.alder:101:29: warning[W0002]: hole ?expected has type Answer
    Task.succeed (r.answer == ?expected)
                              ^^^^^^^^^
app/Orders/Show.alder:116:16: warning[W0002]: hole ?expected has type Bool
    Task.succeed ?expected
                 ^^^^^^^^^
app/Orders/Show.alder:131:16: warning[W0002]: hole ?expected has type Bool
    Task.succeed ?expected
                 ^^^^^^^^^
app/Orders/Show.alder:135:3: warning[W0002]: hole ?test has type Task Bool
    ?test
    ^^^^^
[exit 0]
$ alder fill 'Orders.Show."an Open order'"'"'s total is the sum of its lines"' - <<'EOF'
{
  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
  let tea <- Db.insert Product { name: Text.NonEmpty "Tea", price: Int.NonNegative 250 }
  let cake <- Db.insert Product { name: Text.NonEmpty "Cake", price: Int.NonNegative 100 }
  let o <- Db.insert Order { owner: me }
  let _ <- Db.insert Line { order: o, product: tea, quantity: Int.Positive 2 }
  let _ <- Db.insert Line { order: o, product: cake, quantity: Int.Positive 3 }
  let _ <- Test.login { user: Just me }
  let page <- Test.get (Orders.Show { id: o })
  Task.succeed ({ status: Test.text "#status" page.doc, total: Test.text "#total" page.doc } == { status: Just "Open", total: Just "800" })
}
EOF
edited app/Orders/Show.alder
--- a/app/Orders/Show.alder
+++ b/app/Orders/Show.alder
@@ -132,5 +132,13 @@
 }
 
 test "an Open order's total is the sum of its lines" = {
-  ?test
+  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
+  let tea <- Db.insert Product { name: Text.NonEmpty "Tea", price: Int.NonNegative 250 }
+  let cake <- Db.insert Product { name: Text.NonEmpty "Cake", price: Int.NonNegative 100 }
+  let o <- Db.insert Order { owner: me }
+  let _ <- Db.insert Line { order: o, product: tea, quantity: Int.Positive 2 }
+  let _ <- Db.insert Line { order: o, product: cake, quantity: Int.Positive 3 }
+  let _ <- Test.login { user: Just me }
+  let page <- Test.get (Orders.Show { id: o })
+  Task.succeed ({ status: Test.text "#status" page.doc, total: Test.text "#total" page.doc } == { status: Just "Open", total: Just "800" })
 }
app/Orders/Show.alder:100:44: warning[W0002]: hole ?addLineProduct has type Product.ProductId
    let r <- Test.submit (AddLine { product: ?addLineProduct }) { quantity: Int.Positive 1 }
                                             ^^^^^^^^^^^^^^^
app/Orders/Show.alder:101:29: warning[W0002]: hole ?expected has type Answer
    Task.succeed (r.answer == ?expected)
                              ^^^^^^^^^
app/Orders/Show.alder:116:16: warning[W0002]: hole ?expected has type Bool
    Task.succeed ?expected
                 ^^^^^^^^^
app/Orders/Show.alder:131:16: warning[W0002]: hole ?expected has type Bool
    Task.succeed ?expected
                 ^^^^^^^^^
[exit 0]
$ alder new test Orders.Show "adding a line to a Paid order is refused"
edited app/Orders/Show.alder
--- a/app/Orders/Show.alder
+++ b/app/Orders/Show.alder
@@ -142,3 +142,7 @@
   let page <- Test.get (Orders.Show { id: o })
   Task.succeed ({ status: Test.text "#status" page.doc, total: Test.text "#total" page.doc } == { status: Just "Open", total: Just "800" })
 }
+
+test "adding a line to a Paid order is refused" = {
+  ?test
+}
app/Orders/Show.alder:100:44: warning[W0002]: hole ?addLineProduct has type Product.ProductId
    let r <- Test.submit (AddLine { product: ?addLineProduct }) { quantity: Int.Positive 1 }
                                             ^^^^^^^^^^^^^^^
app/Orders/Show.alder:101:29: warning[W0002]: hole ?expected has type Answer
    Task.succeed (r.answer == ?expected)
                              ^^^^^^^^^
app/Orders/Show.alder:116:16: warning[W0002]: hole ?expected has type Bool
    Task.succeed ?expected
                 ^^^^^^^^^
app/Orders/Show.alder:131:16: warning[W0002]: hole ?expected has type Bool
    Task.succeed ?expected
                 ^^^^^^^^^
app/Orders/Show.alder:147:3: warning[W0002]: hole ?test has type Task Bool
    ?test
    ^^^^^
[exit 0]
$ alder fill 'Orders.Show."adding a line to a Paid order is refused"' - <<'EOF'
{
  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
  let tea <- Db.insert Product { name: Text.NonEmpty "Tea", price: Int.NonNegative 250 }
  let o <- Db.insert Order { owner: me }
  let _ <- Test.login { user: Just me }
  let _ <- Test.get (Orders.Show { id: o })
  let _ <- Db.update Order o (\x -> { x | status: Paid })
  let r <- Test.submit (AddLine { product: tea }) { quantity: Int.Positive 2 }
  let n <- Db.from Line |> Db.where (Line.order.eq o) |> Db.count
  Task.succeed (n == 0)
}
EOF
edited app/Orders/Show.alder
--- a/app/Orders/Show.alder
+++ b/app/Orders/Show.alder
@@ -144,5 +144,13 @@
 }
 
 test "adding a line to a Paid order is refused" = {
-  ?test
+  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
+  let tea <- Db.insert Product { name: Text.NonEmpty "Tea", price: Int.NonNegative 250 }
+  let o <- Db.insert Order { owner: me }
+  let _ <- Test.login { user: Just me }
+  let _ <- Test.get (Orders.Show { id: o })
+  let _ <- Db.update Order o (\x -> { x | status: Paid })
+  let r <- Test.submit (AddLine { product: tea }) { quantity: Int.Positive 2 }
+  let n <- Db.from Line |> Db.where (Line.order.eq o) |> Db.count
+  Task.succeed (n == 0)
 }
app/Orders/Show.alder:100:44: warning[W0002]: hole ?addLineProduct has type Product.ProductId
    let r <- Test.submit (AddLine { product: ?addLineProduct }) { quantity: Int.Positive 1 }
                                             ^^^^^^^^^^^^^^^
app/Orders/Show.alder:101:29: warning[W0002]: hole ?expected has type Answer
    Task.succeed (r.answer == ?expected)
                              ^^^^^^^^^
app/Orders/Show.alder:116:16: warning[W0002]: hole ?expected has type Bool
    Task.succeed ?expected
                 ^^^^^^^^^
app/Orders/Show.alder:131:16: warning[W0002]: hole ?expected has type Bool
    Task.succeed ?expected
                 ^^^^^^^^^
[exit 0]
$ alder new test Orders.Show "a Staff user opening another user's order is not found"
edited app/Orders/Show.alder
--- a/app/Orders/Show.alder
+++ b/app/Orders/Show.alder
@@ -154,3 +154,7 @@
   let n <- Db.from Line |> Db.where (Line.order.eq o) |> Db.count
   Task.succeed (n == 0)
 }
+
+test "a Staff user opening another user's order is not found" = {
+  ?test
+}
app/Orders/Show.alder:100:44: warning[W0002]: hole ?addLineProduct has type Product.ProductId
    let r <- Test.submit (AddLine { product: ?addLineProduct }) { quantity: Int.Positive 1 }
                                             ^^^^^^^^^^^^^^^
app/Orders/Show.alder:101:29: warning[W0002]: hole ?expected has type Answer
    Task.succeed (r.answer == ?expected)
                              ^^^^^^^^^
app/Orders/Show.alder:116:16: warning[W0002]: hole ?expected has type Bool
    Task.succeed ?expected
                 ^^^^^^^^^
app/Orders/Show.alder:131:16: warning[W0002]: hole ?expected has type Bool
    Task.succeed ?expected
                 ^^^^^^^^^
app/Orders/Show.alder:159:3: warning[W0002]: hole ?test has type Task Bool
    ?test
    ^^^^^
[exit 0]
$ alder fill 'Orders.Show."a Staff user opening another user'"'"'s order is not found"' - <<'EOF'
{
  let owner <- Db.insert User { email: Text.Email "owner@example.com", name: Text.NonEmpty "Owner", passwordHash: "" }
  let staff <- Db.insert User { email: Text.Email "staff@example.com", name: Text.NonEmpty "Staff", passwordHash: "" }
  let o <- Db.insert Order { owner: owner }
  let _ <- Test.login { user: Just staff }
  let r <- Test.open (Orders.Show { id: o })
  Task.succeed (r.answer == PageMissing)
}
EOF
edited app/Orders/Show.alder
--- a/app/Orders/Show.alder
+++ b/app/Orders/Show.alder
@@ -156,5 +156,10 @@
 }
 
 test "a Staff user opening another user's order is not found" = {
-  ?test
+  let owner <- Db.insert User { email: Text.Email "owner@example.com", name: Text.NonEmpty "Owner", passwordHash: "" }
+  let staff <- Db.insert User { email: Text.Email "staff@example.com", name: Text.NonEmpty "Staff", passwordHash: "" }
+  let o <- Db.insert Order { owner: owner }
+  let _ <- Test.login { user: Just staff }
+  let r <- Test.open (Orders.Show { id: o })
+  Task.succeed (r.answer == PageMissing)
 }
app/Orders/Show.alder:100:44: warning[W0002]: hole ?addLineProduct has type Product.ProductId
    let r <- Test.submit (AddLine { product: ?addLineProduct }) { quantity: Int.Positive 1 }
                                             ^^^^^^^^^^^^^^^
app/Orders/Show.alder:101:29: warning[W0002]: hole ?expected has type Answer
    Task.succeed (r.answer == ?expected)
                              ^^^^^^^^^
app/Orders/Show.alder:116:16: warning[W0002]: hole ?expected has type Bool
    Task.succeed ?expected
                 ^^^^^^^^^
app/Orders/Show.alder:131:16: warning[W0002]: hole ?expected has type Bool
    Task.succeed ?expected
                 ^^^^^^^^^
[exit 0]
$ alder new test Orders.Show "a Manager opening another user's order sees it"
edited app/Orders/Show.alder
--- a/app/Orders/Show.alder
+++ b/app/Orders/Show.alder
@@ -163,3 +163,7 @@
   let r <- Test.open (Orders.Show { id: o })
   Task.succeed (r.answer == PageMissing)
 }
+
+test "a Manager opening another user's order sees it" = {
+  ?test
+}
app/Orders/Show.alder:100:44: warning[W0002]: hole ?addLineProduct has type Product.ProductId
    let r <- Test.submit (AddLine { product: ?addLineProduct }) { quantity: Int.Positive 1 }
                                             ^^^^^^^^^^^^^^^
app/Orders/Show.alder:101:29: warning[W0002]: hole ?expected has type Answer
    Task.succeed (r.answer == ?expected)
                              ^^^^^^^^^
app/Orders/Show.alder:116:16: warning[W0002]: hole ?expected has type Bool
    Task.succeed ?expected
                 ^^^^^^^^^
app/Orders/Show.alder:131:16: warning[W0002]: hole ?expected has type Bool
    Task.succeed ?expected
                 ^^^^^^^^^
app/Orders/Show.alder:168:3: warning[W0002]: hole ?test has type Task Bool
    ?test
    ^^^^^
[exit 0]
$ alder fill 'Orders.Show."a Manager opening another user'"'"'s order sees it"' - <<'EOF'
{
  let owner <- Db.insert User { email: Text.Email "owner@example.com", name: Text.NonEmpty "Owner", passwordHash: "" }
  let boss <- Db.insert User { email: Text.Email "boss@example.com", name: Text.NonEmpty "Boss", passwordHash: "", role: Manager }
  let o <- Db.insert Order { owner: owner }
  let _ <- Test.login { user: Just boss }
  let r <- Test.open (Orders.Show { id: o })
  Task.succeed (r.answer == Shown)
}
EOF
edited app/Orders/Show.alder
--- a/app/Orders/Show.alder
+++ b/app/Orders/Show.alder
@@ -165,5 +165,10 @@
 }
 
 test "a Manager opening another user's order sees it" = {
-  ?test
+  let owner <- Db.insert User { email: Text.Email "owner@example.com", name: Text.NonEmpty "Owner", passwordHash: "" }
+  let boss <- Db.insert User { email: Text.Email "boss@example.com", name: Text.NonEmpty "Boss", passwordHash: "", role: Manager }
+  let o <- Db.insert Order { owner: owner }
+  let _ <- Test.login { user: Just boss }
+  let r <- Test.open (Orders.Show { id: o })
+  Task.succeed (r.answer == Shown)
 }
app/Orders/Show.alder:100:44: warning[W0002]: hole ?addLineProduct has type Product.ProductId
    let r <- Test.submit (AddLine { product: ?addLineProduct }) { quantity: Int.Positive 1 }
                                             ^^^^^^^^^^^^^^^
app/Orders/Show.alder:101:29: warning[W0002]: hole ?expected has type Answer
    Task.succeed (r.answer == ?expected)
                              ^^^^^^^^^
app/Orders/Show.alder:116:16: warning[W0002]: hole ?expected has type Bool
    Task.succeed ?expected
                 ^^^^^^^^^
app/Orders/Show.alder:131:16: warning[W0002]: hole ?expected has type Bool
    Task.succeed ?expected
                 ^^^^^^^^^
[exit 0]
$ alder new test Products.Index "a Staff user gets 404 on products"
edited app/Products/Index.alder
--- a/app/Products/Index.alder
+++ b/app/Products/Index.alder
@@ -94,3 +94,7 @@
   let after <- Db.get Product product1
   Task.succeed ?expected
 }
+
+test "a Staff user gets 404 on products" = {
+  ?test
+}
app/Products/Index.alder:95:16: warning[W0002]: hole ?expected has type Bool
    Task.succeed ?expected
                 ^^^^^^^^^
app/Products/Index.alder:99:3: warning[W0002]: hole ?test has type Task Bool
    ?test
    ^^^^^
[exit 0]
$ alder fill 'Products.Index."a Staff user gets 404 on products"' - <<'EOF'
{
  let staff <- Db.insert User { email: Text.Email "staff@example.com", name: Text.NonEmpty "Staff", passwordHash: "" }
  let _ <- Test.login { user: Just staff }
  let r <- Test.open (Products.Index {})
  Task.succeed (r.answer == PageMissing)
}
EOF
edited app/Products/Index.alder
--- a/app/Products/Index.alder
+++ b/app/Products/Index.alder
@@ -96,5 +96,8 @@
 }
 
 test "a Staff user gets 404 on products" = {
-  ?test
+  let staff <- Db.insert User { email: Text.Email "staff@example.com", name: Text.NonEmpty "Staff", passwordHash: "" }
+  let _ <- Test.login { user: Just staff }
+  let r <- Test.open (Products.Index {})
+  Task.succeed (r.answer == PageMissing)
 }
app/Products/Index.alder:95:16: warning[W0002]: hole ?expected has type Bool
    Task.succeed ?expected
                 ^^^^^^^^^
[exit 0]
$ alder fill --list --json
{"holes":[{"module":"Orders.Index","decl":"\"pressing Create\"","name":"expected","type":"Bool","file":"app/Orders/Index.alder","line":63,"col":16},{"module":"Orders.Show","decl":"\"submitting NewLine\"","name":"addLineProduct","type":"Product.ProductId","file":"app/Orders/Show.alder","line":100,"col":44},{"module":"Orders.Show","decl":"\"submitting NewLine\"","name":"expected","type":"Answer","file":"app/Orders/Show.alder","line":101,"col":29},{"module":"Orders.Show","decl":"\"pressing Pay\"","name":"expected","type":"Bool","file":"app/Orders/Show.alder","line":116,"col":16},{"module":"Orders.Show","decl":"\"pressing Cancel\"","name":"expected","type":"Bool","file":"app/Orders/Show.alder","line":131,"col":16},{"module":"Products.Index","decl":"\"pressing Toggle\"","name":"expected","type":"Bool","file":"app/Products/Index.alder","line":95,"col":16}]}
[exit 0]
$ alder check
app/Orders/Index.alder:63:16: warning[W0002]: hole ?expected has type Bool
    Task.succeed ?expected
                 ^^^^^^^^^
app/Orders/Show.alder:100:44: warning[W0002]: hole ?addLineProduct has type Product.ProductId
    let r <- Test.submit (AddLine { product: ?addLineProduct }) { quantity: Int.Positive 1 }
                                             ^^^^^^^^^^^^^^^
app/Orders/Show.alder:101:29: warning[W0002]: hole ?expected has type Answer
    Task.succeed (r.answer == ?expected)
                              ^^^^^^^^^
app/Orders/Show.alder:116:16: warning[W0002]: hole ?expected has type Bool
    Task.succeed ?expected
                 ^^^^^^^^^
app/Orders/Show.alder:131:16: warning[W0002]: hole ?expected has type Bool
    Task.succeed ?expected
                 ^^^^^^^^^
app/Products/Index.alder:95:16: warning[W0002]: hole ?expected has type Bool
    Task.succeed ?expected
                 ^^^^^^^^^
alder check: 6 typed holes left
[exit 0]
```

The program type-checks but `alder build` refused it; fix the cause below.

```text
app/Orders/Index.alder:63:16: warning[W0002]: hole ?expected has type Bool
    Task.succeed ?expected
                 ^^^^^^^^^
app/Orders/Show.alder:100:44: warning[W0002]: hole ?addLineProduct has type Product.ProductId
    let r <- Test.submit (AddLine { product: ?addLineProduct }) { quantity: Int.Positive 1 }
                                             ^^^^^^^^^^^^^^^
app/Orders/Show.alder:101:29: warning[W0002]: hole ?expected has type Answer
    Task.succeed (r.answer == ?expected)
                              ^^^^^^^^^
app/Orders/Show.alder:116:16: warning[W0002]: hole ?expected has type Bool
    Task.succeed ?expected
                 ^^^^^^^^^
app/Orders/Show.alder:131:16: warning[W0002]: hole ?expected has type Bool
    Task.succeed ?expected
                 ^^^^^^^^^
app/Products/Index.alder:95:16: warning[W0002]: hole ?expected has type Bool
    Task.succeed ?expected
                 ^^^^^^^^^
alder build: the program has 6 typed hole(s); fill them before building
```
