### Round 3

```text
$ alder fill 'Auth.Login."logging in through the form"' - --hole ?afterLogin <<'EOF'
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
$ alder fill Orders.Index.load - --hole ?load <<'EOF'
{
  let orders <- Db.from Order |> Db.where (Order.owner.eq session.user) |> Db.orderBy Order.created.desc |> Db.all
  let lines <- Db.from Line |> Db.all
  let products <- Db.from Product |> Db.all
  Query.succeed { orders: orders, lines: lines, products: products }
}
EOF
edited app/Orders/Index.alder
--- a/app/Orders/Index.alder
+++ b/app/Orders/Index.alder
@@ -9,7 +9,12 @@
 type Model = { orders: List Order, lines: List Line, products: List Product }
 
 load : Params -> Session -> Query Model
-load params session = ?load
+load params session = {
+  let orders <- Db.from Order |> Db.where (Order.owner.eq session.user) |> Db.orderBy Order.created.desc |> Db.all
+  let lines <- Db.from Line |> Db.all
+  let products <- Db.from Product |> Db.all
+  Query.succeed { orders: orders, lines: lines, products: products }
+}
 
 view : Model -> Session -> Html Msg
 view model session = ?view
app/Orders/Index.alder:20:22: warning[W0002]: hole ?view has type Html Orders.Index.Msg
  view model session = ?view
                       ^^^^^
[exit 0]
$ alder remove page Orders.Show
removed app/Orders/Show.alder
[exit 0]
$ alder new page Orders.Show "/orders/{id}" --param id:Order.OrderId --requires user --blank
wrote app/Orders/Show.alder
app/Orders/Show.alder:11:23: warning[W0002]: hole ?load has type Query Orders.Show.Model
  load params session = ?load
                        ^^^^^
app/Orders/Show.alder:14:22: warning[W0002]: hole ?view has type Html Orders.Show.Msg
  view model session = ?view
                       ^^^^^
[exit 0]
$ alder fill --decl Orders.Show.Model - <<'EOF'
{ order: Order, lines: List Line, products: List Product }
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
+type Model = { order: Order, lines: List Line, products: List Product }
 
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
$ alder fill Orders.Show.load - --hole ?load <<'EOF'
{
  let mgr <- User.isManager session.user
  let found <- Db.get Order params.id
  match found {
    Nothing -> Query.notFound,
    Just o -> if o.owner == session.user || mgr then {
      let lines <- Db.from Line |> Db.where (Line.order.eq (o.id)) |> Db.all
      let products <- Db.from Product |> Db.all
      Query.succeed { order: o, lines: lines, products: products }
    } else Query.notFound,
  }
}
EOF
edited app/Orders/Show.alder
--- a/app/Orders/Show.alder
+++ b/app/Orders/Show.alder
@@ -10,7 +10,18 @@
 type Model = { order: Order, lines: List Line, products: List Product }
 
 load : Params -> Session -> Query Model
-load params session = ?load
+load params session = {
+  let mgr <- User.isManager session.user
+  let found <- Db.get Order params.id
+  match found {
+    Nothing -> Query.notFound,
+    Just o -> if o.owner == session.user || mgr then {
+      let lines <- Db.from Line |> Db.where (Line.order.eq (o.id)) |> Db.all
+      let products <- Db.from Product |> Db.all
+      Query.succeed { order: o, lines: lines, products: products }
+    } else Query.notFound,
+  }
+}
 
 view : Model -> Session -> Html Msg
 view model session = ?view
app/Orders/Show.alder:27:22: warning[W0002]: hole ?view has type Html Orders.Show.Msg
  view model session = ?view
                       ^^^^^
[exit 0]
$ alder fill Orders.Show.view - --hole ?view <<'EOF'
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
@@ -24,7 +25,9 @@
 }
 
 view : Model -> Session -> Html Msg
-view model session = ?view
+view model session = <Layout.Page title="Order">
+    <h1>Order</h1>
+  </Layout.Page>
 
 test "a visitor is sent to log in" = {
   let me <- Db.insert User { email: Text.Email "email-1@example.com", name: Text.NonEmpty "name-1", passwordHash: "passwordhash-1" }
added import Layout to app/Orders/Show.alder
[exit 0]
$ alder new form Orders.Show NewLine product:Product.ProductId quantity:Int.Positive --msg AddLine
edited app/Orders/Show.alder
--- a/app/Orders/Show.alder
+++ b/app/Orders/Show.alder
@@ -24,9 +24,31 @@
   }
 }
 
+form NewLine = {
+  product: Product.ProductId,
+  quantity: Int.Positive,
+}
+
+msg AddLine form NewLine
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
+    <al-card>
+      <h2 slot="header">New line</h2>
+      <form submit={AddLine}>
+        <al-input field="product" label="Product" />
+        <al-input field="quantity" label="Quantity" />
+        <al-alert variant="destructive"><Errors form /></al-alert>
+        <al-button type="submit">New line</al-button>
+      </form>
+    </al-card>
   </Layout.Page>
 
 test "a visitor is sent to log in" = {
@@ -35,3 +57,13 @@
   let r <- Test.open (Orders.Show { id: order1 })
   Task.succeed (r.answer == Redirected (Auth.Login {}))
 }
+
+test "submitting NewLine" = {
+  let me <- Db.insert User { email: Text.Email "email-1@example.com", name: Text.NonEmpty "name-1", passwordHash: "passwordhash-1" }
+  let _ <- Test.login { user: Just me }
+  let order1 <- Db.insert Order { owner: me }
+  let product1 <- Db.insert Product { name: Text.NonEmpty "name-1", price: Int.NonNegative 1 }
+  let _ <- Test.get (Orders.Show { id: order1 })
+  let r <- Test.submit NewLine { product: product1, quantity: Int.Positive 1 }
+  Task.succeed (r.answer == ?expected)
+}
app/Orders/Show.alder:36:16: warning[W0002]: hole ?addLine has type Task Nav
    AddLine p -> ?addLine,
                 ^^^^^^^^
app/Orders/Show.alder:68:29: warning[W0002]: hole ?expected has type Answer
    Task.succeed (r.answer == ?expected)
                              ^^^^^^^^^
[exit 0]
$ alder new msg Orders.Show Pay
edited app/Orders/Show.alder
--- a/app/Orders/Show.alder
+++ b/app/Orders/Show.alder
@@ -31,9 +31,12 @@
 
 msg AddLine form NewLine
 
+msg Pay
+
 update : Msg -> Model -> Session -> Task Nav
 update msg model session = match msg {
   AddLine p -> ?addLine,
+  Pay p -> ?pay,
 }
 
 view : Model -> Session -> Html Msg
@@ -49,6 +52,8 @@
         <al-button type="submit">New line</al-button>
       </form>
     </al-card>
+
+    <al-button click={Pay}>Pay</al-button>
   </Layout.Page>
 
 test "a visitor is sent to log in" = {
@@ -67,3 +72,18 @@
   let r <- Test.submit NewLine { product: product1, quantity: Int.Positive 1 }
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
app/Orders/Show.alder:38:16: warning[W0002]: hole ?addLine has type Task Nav
    AddLine p -> ?addLine,
                 ^^^^^^^^
app/Orders/Show.alder:39:12: warning[W0002]: hole ?pay has type Task Nav
    Pay p -> ?pay,
             ^^^^
app/Orders/Show.alder:73:29: warning[W0002]: hole ?expected has type Answer
    Task.succeed (r.answer == ?expected)
                              ^^^^^^^^^
app/Orders/Show.alder:88:16: warning[W0002]: hole ?expected has type Bool
    Task.succeed ?expected
                 ^^^^^^^^^
[exit 0]
$ alder new msg Orders.Show Cancel
edited app/Orders/Show.alder
--- a/app/Orders/Show.alder
+++ b/app/Orders/Show.alder
@@ -33,10 +33,13 @@
 
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
@@ -54,6 +57,8 @@
     </al-card>
 
     <al-button click={Pay}>Pay</al-button>
+
+    <al-button click={Cancel}>Cancel</al-button>
   </Layout.Page>
 
 test "a visitor is sent to log in" = {
@@ -87,3 +92,18 @@
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
app/Orders/Show.alder:40:16: warning[W0002]: hole ?addLine has type Task Nav
    AddLine p -> ?addLine,
                 ^^^^^^^^
app/Orders/Show.alder:41:12: warning[W0002]: hole ?pay has type Task Nav
    Pay p -> ?pay,
             ^^^^
app/Orders/Show.alder:42:15: warning[W0002]: hole ?cancel has type Task Nav
    Cancel p -> ?cancel,
                ^^^^^^^
app/Orders/Show.alder:78:29: warning[W0002]: hole ?expected has type Answer
    Task.succeed (r.answer == ?expected)
                              ^^^^^^^^^
app/Orders/Show.alder:93:16: warning[W0002]: hole ?expected has type Bool
    Task.succeed ?expected
                 ^^^^^^^^^
app/Orders/Show.alder:108:16: warning[W0002]: hole ?expected has type Bool
    Task.succeed ?expected
                 ^^^^^^^^^
[exit 0]
$ alder fill --decl Orders.Show.view - <<'EOF'
<Layout.Page title="Order">
    <h1>Order</h1>
    <p id="status">{Order.statusText model.order.status}</p>
    <p id="total">{Text.fromInt (Line.orderTotal model.products model.lines)}</p>
    <ul id="lines">
      {for l in model.lines -> <li>{Text.fromInt l.quantity} × {Text.fromInt (Line.lineCost model.products l)}</li>}
    </ul>
    {if model.order.status == Open then
      <al-card>
        <form submit={AddLine}>
          <Errors form />
          <Select field="product" label="Product" options={List.map (\p -> { value: p.id, label: p.name }) (List.filter (\p -> p.active) model.products)} />
          <Input field="quantity" label="Quantity" />
          <al-button type="submit">Add line</al-button>
        </form>
        <al-button click={Pay}>Pay</al-button>
        <al-button click={Cancel}>Cancel</al-button>
      </al-card>
    else Html.none}
  </Layout.Page>
EOF

(the harness stopped this command after 600 s)
[exit —]
```

The harness stopped at line 46 of your script: `alder fill --decl Orders.Show.view - <<'EOF'`: it exited None (timed out). The commands after it were not run.

Your script stopped at command 11; the app is as the commands before it left it; continue.

The program type-checks but `alder build` refused it; fix the cause below.

```text
app/Orders/Index.alder:20:22: warning[W0002]: hole ?view has type Html Orders.Index.Msg
  view model session = ?view
                       ^^^^^
app/Orders/Show.alder:40:16: warning[W0002]: hole ?addLine has type Task Nav
    AddLine p -> ?addLine,
                 ^^^^^^^^
app/Orders/Show.alder:41:12: warning[W0002]: hole ?pay has type Task Nav
    Pay p -> ?pay,
             ^^^^
app/Orders/Show.alder:42:15: warning[W0002]: hole ?cancel has type Task Nav
    Cancel p -> ?cancel,
                ^^^^^^^
app/Orders/Show.alder:78:29: warning[W0002]: hole ?expected has type Answer
    Task.succeed (r.answer == ?expected)
                              ^^^^^^^^^
app/Orders/Show.alder:93:16: warning[W0002]: hole ?expected has type Bool
    Task.succeed ?expected
                 ^^^^^^^^^
app/Orders/Show.alder:108:16: warning[W0002]: hole ?expected has type Bool
    Task.succeed ?expected
                 ^^^^^^^^^
alder build: the program has 7 typed hole(s); fill them before building
```
