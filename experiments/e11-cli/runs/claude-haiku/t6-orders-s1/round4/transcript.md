### Round 4

```text
$ alder fill Orders.Index.view - --hole ?view <<'EOF'
<Layout.Page title="Orders">
    <h1>Orders</h1>
    <ul id="orders">
      {for o in model.orders ->
        <li>
          <a href={Orders.Show { id: o.id }}>Open order</a>
          <span class="status">{Order.statusText o.status}</span>
          <span class="total">{Text.fromInt (Line.orderTotal model.products (List.filter (\l -> l.order == o.id) model.lines))}</span>
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
@@ -17,7 +18,17 @@
 }
 
 view : Model -> Session -> Html Msg
-view model session = ?view
+view model session = <Layout.Page title="Orders">
+    <h1>Orders</h1>
+    <ul id="orders">
+      {for o in model.orders ->
+        <li>
+          <a href={Orders.Show { id: o.id }}>Open order</a>
+          <span class="status">{Order.statusText o.status}</span>
+          <span class="total">{Text.fromInt (Line.orderTotal model.products (List.filter (\l -> l.order == o.id) model.lines))}</span>
+        </li>}
+    </ul>
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
 
@@ -17,6 +18,13 @@
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
 view model session = <Layout.Page title="Orders">
     <h1>Orders</h1>
@@ -28,9 +36,25 @@
           <span class="total">{Text.fromInt (Line.orderTotal model.products (List.filter (\l -> l.order == o.id) model.lines))}</span>
         </li>}
     </ul>
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
app/Orders/Index.alder:59:16: warning[W0002]: hole ?expected has type Bool
    Task.succeed ?expected
                 ^^^^^^^^^
[exit 0]
$ alder fill Orders.Index.update.Create - --hole ?create <<'EOF'
{
  let id <- Db.insert Order { owner: session.user }
  Task.succeed (Redirect (Orders.Show { id: id }))
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
app/Orders/Index.alder:62:16: warning[W0002]: hole ?expected has type Bool
    Task.succeed ?expected
                 ^^^^^^^^^
[exit 0]
$ alder fill --decl Orders.Show.view - <<'EOF'
<Layout.Page title="Order">
    <h1>Order</h1>
    <p id="status">{Order.statusText model.order.status}</p>
    <p id="total">{Text.fromInt (Line.orderTotal model.products model.lines)}</p>
    <ul id="lines">
      {for l in model.lines -> <li>{Text.fromInt l.quantity} x {Text.fromInt (Line.lineCost model.products l)}</li>}
    </ul>
    {if model.order.status == Open then
      <div>
        <form submit={AddLine}>
          <Errors form />
          <Select field="product" label="Product" options={List.map (\p -> { value: p.id, label: p.name }) (List.filter (\p -> p.active) model.products)} />
          <Input field="quantity" label="Quantity" />
          <al-button type="submit">Add line</al-button>
        </form>
        <al-button click={Pay}>Pay</al-button>
        <al-button click={Cancel}>Cancel</al-button>
      </div>
    else Html.none}
  </Layout.Page>
EOF
edited app/Orders/Show.alder
--- a/app/Orders/Show.alder
+++ b/app/Orders/Show.alder
@@ -43,23 +43,27 @@
 }
 
 view : Model -> Session -> Html Msg
-view model session = <Layout.Page title="Order">
-    <h1>Order</h1>
-
-    <al-card>
-      <h2 slot="header">New line</h2>
-      <form submit={AddLine}>
-        <al-input field="product" label="Product" />
-        <al-input field="quantity" label="Quantity" />
-        <al-alert variant="destructive"><Errors form /></al-alert>
-        <al-button type="submit">New line</al-button>
-      </form>
-    </al-card>
-
-    <al-button click={Pay}>Pay</al-button>
-
-    <al-button click={Cancel}>Cancel</al-button>
-  </Layout.Page>
+view model session =
+  <Layout.Page title="Order">
+      <h1>Order</h1>
+      <p id="status">{Order.statusText model.order.status}</p>
+      <p id="total">{Text.fromInt (Line.orderTotal model.products model.lines)}</p>
+      <ul id="lines">
+        {for l in model.lines -> <li>{Text.fromInt l.quantity} x {Text.fromInt (Line.lineCost model.products l)}</li>}
+      </ul>
+      {if model.order.status == Open then
+        <div>
+          <form submit={AddLine}>
+            <Errors form />
+            <Select field="product" label="Product" options={List.map (\p -> { value: p.id, label: p.name }) (List.filter (\p -> p.active) model.products)} />
+            <Input field="quantity" label="Quantity" />
+            <al-button type="submit">Add line</al-button>
+          </form>
+          <al-button click={Pay}>Pay</al-button>
+          <al-button click={Cancel}>Cancel</al-button>
+        </div>
+      else Html.none}
+    </Layout.Page>
 
 test "a visitor is sent to log in" = {
   let me <- Db.insert User { email: Text.Email "email-1@example.com", name: Text.NonEmpty "name-1", passwordHash: "passwordhash-1" }
app/Orders/Show.alder:40:16: warning[W0002]: hole ?addLine has type Task Nav
    AddLine p -> ?addLine,
                 ^^^^^^^^
app/Orders/Show.alder:41:12: warning[W0002]: hole ?pay has type Task Nav
    Pay p -> ?pay,
             ^^^^
app/Orders/Show.alder:42:15: warning[W0002]: hole ?cancel has type Task Nav
    Cancel p -> ?cancel,
                ^^^^^^^
app/Orders/Show.alder:82:29: warning[W0002]: hole ?expected has type Answer
    Task.succeed (r.answer == ?expected)
                              ^^^^^^^^^
app/Orders/Show.alder:97:16: warning[W0002]: hole ?expected has type Bool
    Task.succeed ?expected
                 ^^^^^^^^^
app/Orders/Show.alder:112:16: warning[W0002]: hole ?expected has type Bool
    Task.succeed ?expected
                 ^^^^^^^^^
[exit 0]
$ alder fill Orders.Show.update.AddLine - --hole ?addLine <<'EOF'
{
  let prod <- Db.get Product p.form.product
  let active = Maybe.withDefault False (Maybe.map (\x -> x.active) prod)
  if model.order.status == Open && active then {
    let _ <- Db.insert Line { order: model.order.id, product: p.form.product, quantity: p.form.quantity }
    Task.succeed (Redirect (Orders.Show { id: model.order.id }))
  } else Task.succeed (Redirect (Orders.Show { id: model.order.id }))
}
EOF
edited app/Orders/Show.alder
--- a/app/Orders/Show.alder
+++ b/app/Orders/Show.alder
@@ -37,7 +37,14 @@
 
 update : Msg -> Model -> Session -> Task Nav
 update msg model session = match msg {
-  AddLine p -> ?addLine,
+  AddLine p -> {
+    let prod <- Db.get Product p.form.product
+    let active = Maybe.withDefault False (Maybe.map (\x -> x.active) prod)
+    if model.order.status == Open && active then {
+      let _ <- Db.insert Line { order: model.order.id, product: p.form.product, quantity: p.form.quantity }
+      Task.succeed (Redirect (Orders.Show { id: model.order.id }))
+    } else Task.succeed (Redirect (Orders.Show { id: model.order.id }))
+  },
   Pay p -> ?pay,
   Cancel p -> ?cancel,
 }
app/Orders/Show.alder:48:12: warning[W0002]: hole ?pay has type Task Nav
    Pay p -> ?pay,
             ^^^^
app/Orders/Show.alder:49:15: warning[W0002]: hole ?cancel has type Task Nav
    Cancel p -> ?cancel,
                ^^^^^^^
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
$ alder fill Orders.Show.update.Pay - --hole ?pay <<'EOF'
{
  if model.order.status == Open then {
    let _ <- Db.update Order model.order.id (\o -> { o | status: Paid })
    Task.succeed (Redirect (Orders.Show { id: model.order.id }))
  } else Task.succeed (Redirect (Orders.Show { id: model.order.id }))
}
EOF
edited app/Orders/Show.alder
--- a/app/Orders/Show.alder
+++ b/app/Orders/Show.alder
@@ -45,7 +45,12 @@
       Task.succeed (Redirect (Orders.Show { id: model.order.id }))
     } else Task.succeed (Redirect (Orders.Show { id: model.order.id }))
   },
-  Pay p -> ?pay,
+  Pay p -> {
+    if model.order.status == Open then {
+      let _ <- Db.update Order model.order.id (\o -> { o | status: Paid })
+      Task.succeed (Redirect (Orders.Show { id: model.order.id }))
+    } else Task.succeed (Redirect (Orders.Show { id: model.order.id }))
+  },
   Cancel p -> ?cancel,
 }
 
app/Orders/Show.alder:54:15: warning[W0002]: hole ?cancel has type Task Nav
    Cancel p -> ?cancel,
                ^^^^^^^
app/Orders/Show.alder:94:29: warning[W0002]: hole ?expected has type Answer
    Task.succeed (r.answer == ?expected)
                              ^^^^^^^^^
app/Orders/Show.alder:109:16: warning[W0002]: hole ?expected has type Bool
    Task.succeed ?expected
                 ^^^^^^^^^
app/Orders/Show.alder:124:16: warning[W0002]: hole ?expected has type Bool
    Task.succeed ?expected
                 ^^^^^^^^^
[exit 0]
$ alder fill Orders.Show.update.Cancel - --hole ?cancel <<'EOF'
{
  if model.order.status == Open then {
    let _ <- Db.update Order model.order.id (\o -> { o | status: Cancelled })
    Task.succeed (Redirect (Orders.Show { id: model.order.id }))
  } else Task.succeed (Redirect (Orders.Show { id: model.order.id }))
}
EOF
edited app/Orders/Show.alder
--- a/app/Orders/Show.alder
+++ b/app/Orders/Show.alder
@@ -51,7 +51,12 @@
       Task.succeed (Redirect (Orders.Show { id: model.order.id }))
     } else Task.succeed (Redirect (Orders.Show { id: model.order.id }))
   },
-  Cancel p -> ?cancel,
+  Cancel p -> {
+    if model.order.status == Open then {
+      let _ <- Db.update Order model.order.id (\o -> { o | status: Cancelled })
+      Task.succeed (Redirect (Orders.Show { id: model.order.id }))
+    } else Task.succeed (Redirect (Orders.Show { id: model.order.id }))
+  },
 }
 
 view : Model -> Session -> Html Msg
app/Orders/Show.alder:99:29: warning[W0002]: hole ?expected has type Answer
    Task.succeed (r.answer == ?expected)
                              ^^^^^^^^^
app/Orders/Show.alder:114:16: warning[W0002]: hole ?expected has type Bool
    Task.succeed ?expected
                 ^^^^^^^^^
app/Orders/Show.alder:129:16: warning[W0002]: hole ?expected has type Bool
    Task.succeed ?expected
                 ^^^^^^^^^
[exit 0]
$ alder fill 'Orders.Show."submitting NewLine"' - --hole ?expected <<'EOF'
Redirected (Orders.Show { id: order1 })
EOF
edited app/Orders/Show.alder
--- a/app/Orders/Show.alder
+++ b/app/Orders/Show.alder
@@ -96,7 +96,7 @@
   let product1 <- Db.insert Product { name: Text.NonEmpty "name-1", price: Int.NonNegative 1 }
   let _ <- Test.get (Orders.Show { id: order1 })
   let r <- Test.submit NewLine { product: product1, quantity: Int.Positive 1 }
-  Task.succeed (r.answer == ?expected)
+  Task.succeed (r.answer == Redirected (Orders.Show { id: order1 }))
 }
 
 -- Fill ?expected with what pressing Pay must do, so that a test fails when the arm
app/Orders/Show.alder:114:16: warning[W0002]: hole ?expected has type Bool
    Task.succeed ?expected
                 ^^^^^^^^^
app/Orders/Show.alder:129:16: warning[W0002]: hole ?expected has type Bool
    Task.succeed ?expected
                 ^^^^^^^^^
[exit 0]
$ alder fill 'Orders.Show."pressing Pay"' - --hole ?expected <<'EOF'
r.answer == Redirected (Orders.Show { id: order1 }) && Test.text "#status" after.doc == Just "Paid"
EOF
edited app/Orders/Show.alder
--- a/app/Orders/Show.alder
+++ b/app/Orders/Show.alder
@@ -99,11 +99,6 @@
   Task.succeed (r.answer == Redirected (Orders.Show { id: order1 }))
 }
 
--- Fill ?expected with what pressing Pay must do, so that a test fails when the arm
--- does something else: its answer (`r.answer`: Rerendered, Redirected …) and what it
--- changed (`after`: the page opened again, `after.answer` Shown, or PageMissing when the arm
--- deleted the page's row, and `after.doc` its HTML), as
--- `{ answer: r.answer, … } == { answer: Rerendered, … }`.
 test "pressing Pay" = {
   let me <- Db.insert User { email: Text.Email "email-1@example.com", name: Text.NonEmpty "name-1", passwordHash: "passwordhash-1" }
   let _ <- Test.login { user: Just me }
@@ -111,7 +106,7 @@
   let _ <- Test.get (Orders.Show { id: order1 })
   let r <- Test.click Pay
   let after <- Test.open (Orders.Show { id: order1 })
-  Task.succeed ?expected
+  Task.succeed (r.answer == Redirected (Orders.Show { id: order1 }) && Test.text "#status" after.doc == Just "Paid")
 }
 
 -- Fill ?expected with what pressing Cancel must do, so that a test fails when the arm
app/Orders/Show.alder:124:16: warning[W0002]: hole ?expected has type Bool
    Task.succeed ?expected
                 ^^^^^^^^^
[exit 0]
$ alder fill 'Orders.Show."pressing Cancel"' - --hole ?expected <<'EOF'
r.answer == Redirected (Orders.Show { id: order1 }) && Test.text "#status" after.doc == Just "Cancelled"
EOF
edited app/Orders/Show.alder
--- a/app/Orders/Show.alder
+++ b/app/Orders/Show.alder
@@ -109,11 +109,6 @@
   Task.succeed (r.answer == Redirected (Orders.Show { id: order1 }) && Test.text "#status" after.doc == Just "Paid")
 }
 
--- Fill ?expected with what pressing Cancel must do, so that a test fails when the arm
--- does something else: its answer (`r.answer`: Rerendered, Redirected …) and what it
--- changed (`after`: the page opened again, `after.answer` Shown, or PageMissing when the arm
--- deleted the page's row, and `after.doc` its HTML), as
--- `{ answer: r.answer, … } == { answer: Rerendered, … }`.
 test "pressing Cancel" = {
   let me <- Db.insert User { email: Text.Email "email-1@example.com", name: Text.NonEmpty "name-1", passwordHash: "passwordhash-1" }
   let _ <- Test.login { user: Just me }
@@ -121,5 +116,5 @@
   let _ <- Test.get (Orders.Show { id: order1 })
   let r <- Test.click Cancel
   let after <- Test.open (Orders.Show { id: order1 })
-  Task.succeed ?expected
+  Task.succeed (r.answer == Redirected (Orders.Show { id: order1 }) && Test.text "#status" after.doc == Just "Cancelled")
 }
[exit 0]
$ alder new test Orders.Show "adding a line to a paid order is refused"
edited app/Orders/Show.alder
--- a/app/Orders/Show.alder
+++ b/app/Orders/Show.alder
@@ -118,3 +118,7 @@
   let after <- Test.open (Orders.Show { id: order1 })
   Task.succeed (r.answer == Redirected (Orders.Show { id: order1 }) && Test.text "#status" after.doc == Just "Cancelled")
 }
+
+test "adding a line to a paid order is refused" = {
+  ?test
+}
app/Orders/Show.alder:123:3: warning[W0002]: hole ?test has type Task Bool
    ?test
    ^^^^^
[exit 0]
$ alder fill 'Orders.Show."adding a line to a paid order is refused"' - <<'EOF'
{
  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
  let tea <- Db.insert Product { name: Text.NonEmpty "Tea", price: Int.NonNegative 250 }
  let order1 <- Db.insert Order { owner: me }
  let _ <- Test.login { user: Just me }
  let _ <- Test.get (Orders.Show { id: order1 })
  let _ <- Db.update Order order1 (\o -> { o | status: Paid })
  let r <- Test.submit NewLine { product: tea, quantity: Int.Positive 1 }
  let lines <- Db.from Line |> Db.count
  Task.succeed ({ answer: r.answer, lines: lines } == { answer: Redirected (Orders.Show { id: order1 }), lines: 0 })
}
EOF
edited app/Orders/Show.alder
--- a/app/Orders/Show.alder
+++ b/app/Orders/Show.alder
@@ -120,5 +120,13 @@
 }
 
 test "adding a line to a paid order is refused" = {
-  ?test
+  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
+  let tea <- Db.insert Product { name: Text.NonEmpty "Tea", price: Int.NonNegative 250 }
+  let order1 <- Db.insert Order { owner: me }
+  let _ <- Test.login { user: Just me }
+  let _ <- Test.get (Orders.Show { id: order1 })
+  let _ <- Db.update Order order1 (\o -> { o | status: Paid })
+  let r <- Test.submit NewLine { product: tea, quantity: Int.Positive 1 }
+  let lines <- Db.from Line |> Db.count
+  Task.succeed ({ answer: r.answer, lines: lines } == { answer: Redirected (Orders.Show { id: order1 }), lines: 0 })
 }
[exit 0]
$ alder new test Orders.Show "a staff user is not found on another order"
edited app/Orders/Show.alder
--- a/app/Orders/Show.alder
+++ b/app/Orders/Show.alder
@@ -130,3 +130,7 @@
   let lines <- Db.from Line |> Db.count
   Task.succeed ({ answer: r.answer, lines: lines } == { answer: Redirected (Orders.Show { id: order1 }), lines: 0 })
 }
+
+test "a staff user is not found on another order" = {
+  ?test
+}
app/Orders/Show.alder:135:3: warning[W0002]: hole ?test has type Task Bool
    ?test
    ^^^^^
[exit 0]
$ alder fill 'Orders.Show."a staff user is not found on another order"' - <<'EOF'
{
  let owner <- Db.insert User { email: Text.Email "owner@example.com", name: Text.NonEmpty "Owner", passwordHash: "" }
  let other <- Db.insert User { email: Text.Email "other@example.com", name: Text.NonEmpty "Other", passwordHash: "" }
  let order1 <- Db.insert Order { owner: owner }
  let _ <- Test.login { user: Just other }
  let r <- Test.open (Orders.Show { id: order1 })
  Task.succeed (r.answer == PageMissing)
}
EOF
edited app/Orders/Show.alder
--- a/app/Orders/Show.alder
+++ b/app/Orders/Show.alder
@@ -132,5 +132,10 @@
 }
 
 test "a staff user is not found on another order" = {
-  ?test
+  let owner <- Db.insert User { email: Text.Email "owner@example.com", name: Text.NonEmpty "Owner", passwordHash: "" }
+  let other <- Db.insert User { email: Text.Email "other@example.com", name: Text.NonEmpty "Other", passwordHash: "" }
+  let order1 <- Db.insert Order { owner: owner }
+  let _ <- Test.login { user: Just other }
+  let r <- Test.open (Orders.Show { id: order1 })
+  Task.succeed (r.answer == PageMissing)
 }
[exit 0]
$ alder new test Orders.Show "a manager opens another order"
edited app/Orders/Show.alder
--- a/app/Orders/Show.alder
+++ b/app/Orders/Show.alder
@@ -139,3 +139,7 @@
   let r <- Test.open (Orders.Show { id: order1 })
   Task.succeed (r.answer == PageMissing)
 }
+
+test "a manager opens another order" = {
+  ?test
+}
app/Orders/Show.alder:144:3: warning[W0002]: hole ?test has type Task Bool
    ?test
    ^^^^^
[exit 0]
$ alder fill 'Orders.Show."a manager opens another order"' - <<'EOF'
{
  let owner <- Db.insert User { email: Text.Email "owner@example.com", name: Text.NonEmpty "Owner", passwordHash: "" }
  let boss <- Db.insert User { email: Text.Email "boss@example.com", name: Text.NonEmpty "Boss", passwordHash: "", role: Manager }
  let order1 <- Db.insert Order { owner: owner }
  let _ <- Test.login { user: Just boss }
  let r <- Test.open (Orders.Show { id: order1 })
  Task.succeed (r.answer == Shown)
}
EOF
edited app/Orders/Show.alder
--- a/app/Orders/Show.alder
+++ b/app/Orders/Show.alder
@@ -141,5 +141,10 @@
 }
 
 test "a manager opens another order" = {
-  ?test
+  let owner <- Db.insert User { email: Text.Email "owner@example.com", name: Text.NonEmpty "Owner", passwordHash: "" }
+  let boss <- Db.insert User { email: Text.Email "boss@example.com", name: Text.NonEmpty "Boss", passwordHash: "", role: Manager }
+  let order1 <- Db.insert Order { owner: owner }
+  let _ <- Test.login { user: Just boss }
+  let r <- Test.open (Orders.Show { id: order1 })
+  Task.succeed (r.answer == Shown)
 }
[exit 0]
$ alder new page Products.Index "/products" --requires user --blank
wrote app/Products/Index.alder
app/Products/Index.alder:8:23: warning[W0002]: hole ?load has type Query Products.Index.Model
  load params session = ?load
                        ^^^^^
app/Products/Index.alder:11:22: warning[W0002]: hole ?view has type Html Products.Index.Msg
  view model session = ?view
                       ^^^^^
[exit 0]
$ alder fill --decl Products.Index.Model - <<'EOF'
{ products: List Product }
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
+type Model = { products: List Product }
 
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
$ alder fill Products.Index.load - --hole ?load <<'EOF'
{
  let mgr <- User.isManager session.user
  if mgr then {
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
 
 type Model = { products: List Product }
 
 load : Params -> Session -> Query Model
-load params session = ?load
+load params session = {
+  let mgr <- User.isManager session.user
+  if mgr then {
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
$ alder fill Products.Index.view - --hole ?view <<'EOF'
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
 
@@ -17,7 +18,9 @@
 }
 
 view : Model -> Session -> Html Msg
-view model session = ?view
+view model session = <Layout.Page title="Products">
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
@@ -17,12 +17,55 @@
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
 view model session = <Layout.Page title="Products">
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
@@ -45,6 +48,8 @@
         <al-button type="submit">New product</al-button>
       </form>
     </al-card>
+
+    <al-button click={Toggle { id: ?toggleId }}>Toggle</al-button>
   </Layout.Page>
 
 test "a visitor is sent to log in" = {
@@ -69,3 +74,16 @@
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
app/Products/Index.alder:52:36: warning[W0002]: hole ?toggleId has type Product.ProductId
      <al-button click={Toggle { id: ?toggleId }}>Toggle</al-button>
                                     ^^^^^^^^^
app/Products/Index.alder:88:16: warning[W0002]: hole ?expected has type Bool
    Task.succeed ?expected
                 ^^^^^^^^^
[exit 0]
$ alder fill Products.Index.update.Add - --hole ?add <<'EOF'
{
  let mgr <- User.isManager session.user
  if mgr then {
    let _ <- Db.insert Product { name: p.form.name, price: p.form.price }
    Task.succeed (Redirect (Products.Index {}))
  } else Task.notFound
}
EOF
alder fill: `Products.Index.update.Add` has no typed hole: a fill replaces a hole, and this arm has none; to rewrite it, give `alder fill --decl Products.Index.update -` the whole `match msg { … }` (`alder fill --list` lists the app's holes)
[exit 1]
```

The harness stopped at line 143 of your script: `alder fill Products.Index.update.Add - --hole ?add <<'EOF'`: it exited 1. The commands after it were not run.

Your script stopped at command 23; the app is as the commands before it left it; continue.

The program type-checks but `alder build` refused it; fix the cause below.

```text
app/Orders/Index.alder:62:16: warning[W0002]: hole ?expected has type Bool
    Task.succeed ?expected
                 ^^^^^^^^^
app/Products/Index.alder:35:15: warning[W0002]: hole ?toggle has type Task Nav
    Toggle p -> ?toggle,
                ^^^^^^^
app/Products/Index.alder:52:36: warning[W0002]: hole ?toggleId has type Product.ProductId
      <al-button click={Toggle { id: ?toggleId }}>Toggle</al-button>
                                     ^^^^^^^^^
app/Products/Index.alder:88:16: warning[W0002]: hole ?expected has type Bool
    Task.succeed ?expected
                 ^^^^^^^^^
alder build: the program has 4 typed hole(s); fill them before building
```
