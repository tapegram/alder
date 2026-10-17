### Round 4

```text
$ alder fill Auth.Login.update.Submit - --hole ?afterLogin <<'EOF'
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
$ alder fill --decl Orders.Index.Model - <<'EOF'
{ rows: List { id: Order.OrderId, status: Order.Status, total: Int } }
EOF
edited app/Orders/Index.alder
--- a/app/Orders/Index.alder
+++ b/app/Orders/Index.alder
@@ -1,8 +1,10 @@
 module Orders.Index
 
+import Order
+
 page "/" { } requires user
 
-type Model = { }
+type Model = { rows: List { id: Order.OrderId, status: Order.Status, total: Int } }
 
 load : Params -> Session -> Query Model
 load params session = ?load
app/Orders/Index.alder:10:23: warning[W0002]: hole ?load has type Query Orders.Index.Model
  load params session = ?load
                        ^^^^^
app/Orders/Index.alder:13:22: warning[W0002]: hole ?view has type Html Orders.Index.Msg
  view model session = ?view
                       ^^^^^
added import Order to app/Orders/Index.alder
[exit 0]
$ alder fill Orders.Index.load - --hole ?load <<'EOF'
{
  let orders <- Db.from Order |> Db.where (Order.owner.eq session.user) |> Db.orderBy Order.created.desc |> Db.all
  let rows <- Query.all (List.map (\o -> {
    let total <- Line.orderTotal o.id
    Query.succeed { id: o.id, status: o.status, total: total }
  }) orders)
  Query.succeed { rows: rows }
}
EOF
edited app/Orders/Index.alder
--- a/app/Orders/Index.alder
+++ b/app/Orders/Index.alder
@@ -1,5 +1,6 @@
 module Orders.Index
 
+import Line
 import Order
 
 page "/" { } requires user
@@ -7,7 +8,14 @@
 type Model = { rows: List { id: Order.OrderId, status: Order.Status, total: Int } }
 
 load : Params -> Session -> Query Model
-load params session = ?load
+load params session = {
+  let orders <- Db.from Order |> Db.where (Order.owner.eq session.user) |> Db.orderBy Order.created.desc |> Db.all
+  let rows <- Query.all (List.map (\o -> {
+    let total <- Line.orderTotal o.id
+    Query.succeed { id: o.id, status: o.status, total: total }
+  }) orders)
+  Query.succeed { rows: rows }
+}
 
 view : Model -> Session -> Html Msg
 view model session = ?view
app/Orders/Index.alder:21:22: warning[W0002]: hole ?view has type Html Orders.Index.Msg
  view model session = ?view
                       ^^^^^
added import Line to app/Orders/Index.alder
[exit 0]
$ alder fill Orders.Index.view - --hole ?view <<'EOF'
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
 
@@ -18,7 +19,9 @@
 }
 
 view : Model -> Session -> Html Msg
-view model session = ?view
+view model session = <Layout.Page title="Orders">
+  <h1>Orders</h1>
+</Layout.Page>
 
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
 
@@ -18,12 +19,35 @@
   Query.succeed { rows: rows }
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
+
+  <al-button click={Create}>Create</al-button>
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
app/Orders/Index.alder:26:15: warning[W0002]: hole ?create has type Task Nav
    Create p -> ?create,
                ^^^^^^^
app/Orders/Index.alder:52:16: warning[W0002]: hole ?expected has type Bool
    Task.succeed ?expected
                 ^^^^^^^^^
[exit 0]
$ alder new page Orders.Show '/orders/{id}' --blank --show Order --requires user
wrote app/Orders/Show.alder
app/Orders/Show.alder:11:23: warning[W0002]: hole ?load has type Query Orders.Show.Model
  load params session = ?load
                        ^^^^^
app/Orders/Show.alder:14:22: warning[W0002]: hole ?view has type Html Orders.Show.Msg
  view model session = ?view
                       ^^^^^
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
@@ -23,7 +23,10 @@
 
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
  <form submit={Create}>
    <al-button type="submit">New order</al-button>
  </form>
  <ul id="orders">
    {for row in model.rows ->
      <li>
        <a href={Orders.Show { id: row.id }}>Open order</a>
        <span class="status">{Order.statusLabel row.status}</span>
        <span class="total">{Text.fromInt row.total}</span>
      </li>}
  </ul>
  <al-button variant="outline" click={App.Logout}>Log out</al-button>
</Layout.Page>
EOF
edited app/Orders/Index.alder
--- a/app/Orders/Index.alder
+++ b/app/Orders/Index.alder
@@ -30,11 +30,22 @@
 }
 
 view : Model -> Session -> Html Msg
-view model session = <Layout.Page title="Orders">
-  <h1>Orders</h1>
-
-  <al-button click={Create}>Create</al-button>
-</Layout.Page>
+view model session =
+  <Layout.Page title="Orders">
+    <h1>Orders</h1>
+    <form submit={Create}>
+      <al-button type="submit">New order</al-button>
+    </form>
+    <ul id="orders">
+      {for row in model.rows ->
+        <li>
+          <a href={Orders.Show { id: row.id }}>Open order</a>
+          <span class="status">{Order.statusLabel row.status}</span>
+          <span class="total">{Text.fromInt row.total}</span>
+        </li>}
+    </ul>
+    <al-button variant="outline" click={App.Logout}>Log out</al-button>
+  </Layout.Page>
 
 test "a visitor is sent to log in" = {
   let r <- Test.open (Orders.Index {})
app/Orders/Index.alder:66:16: warning[W0002]: hole ?expected has type Bool
    Task.succeed ?expected
                 ^^^^^^^^^
[exit 0]
$ alder fill --decl Orders.Show.Model - <<'EOF'
{ id: Order.OrderId, status: Order.Status, lines: List Line.LineRow, total: Int, products: List Line.Choice }
EOF
edited app/Orders/Show.alder
--- a/app/Orders/Show.alder
+++ b/app/Orders/Show.alder
@@ -1,11 +1,12 @@
 module Orders.Show
 
+import Line
 import Order
 import User
 
 page "/orders/{id}" { id: Order.OrderId } requires user
 
-type Model = { }
+type Model = { id: Order.OrderId, status: Order.Status, lines: List Line.LineRow, total: Int, products: List Line.Choice }
 
 load : Params -> Session -> Query Model
 load params session = ?load
app/Orders/Show.alder:12:23: warning[W0002]: hole ?load has type Query Orders.Show.Model
  load params session = ?load
                        ^^^^^
app/Orders/Show.alder:15:22: warning[W0002]: hole ?view has type Html Orders.Show.Msg
  view model session = ?view
                       ^^^^^
added import Line to app/Orders/Show.alder
[exit 0]
$ alder fill Orders.Show.load - --hole ?load <<'EOF'
{
  let found <- Db.get Order params.id
  let mgr <- User.isManager session.user
  let mine = Maybe.withDefault False (Maybe.map (\o -> o.owner == session.user) found)
  let status = Maybe.withDefault Open (Maybe.map (\o -> o.status) found)
  if Maybe.isJust found && (mine || mgr) then {
    let lines <- Line.orderLines params.id
    let total <- Line.orderTotal params.id
    let products <- Line.choices
    Query.succeed { id: params.id, status: status, lines: lines, total: total, products: products }
  } else Query.notFound
}
EOF
edited app/Orders/Show.alder
--- a/app/Orders/Show.alder
+++ b/app/Orders/Show.alder
@@ -9,7 +9,18 @@
 type Model = { id: Order.OrderId, status: Order.Status, lines: List Line.LineRow, total: Int, products: List Line.Choice }
 
 load : Params -> Session -> Query Model
-load params session = ?load
+load params session = {
+  let found <- Db.get Order params.id
+  let mgr <- User.isManager session.user
+  let mine = Maybe.withDefault False (Maybe.map (\o -> o.owner == session.user) found)
+  let status = Maybe.withDefault Open (Maybe.map (\o -> o.status) found)
+  if Maybe.isJust found && (mine || mgr) then {
+    let lines <- Line.orderLines params.id
+    let total <- Line.orderTotal params.id
+    let products <- Line.choices
+    Query.succeed { id: params.id, status: status, lines: lines, total: total, products: products }
+  } else Query.notFound
+}
 
 view : Model -> Session -> Html Msg
 view model session = ?view
app/Orders/Show.alder:26:22: warning[W0002]: hole ?view has type Html Orders.Show.Msg
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
 import User
@@ -23,7 +24,9 @@
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
$ alder new msg Orders.Show AddLine product:Product.ProductId
edited app/Orders/Show.alder
--- a/app/Orders/Show.alder
+++ b/app/Orders/Show.alder
@@ -3,6 +3,7 @@
 import Layout
 import Line
 import Order
+import Product
 import User
 
 page "/orders/{id}" { id: Order.OrderId } requires user
@@ -23,9 +24,18 @@
   } else Query.notFound
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
@@ -34,3 +44,17 @@
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
app/Orders/Show.alder:38:40: warning[W0002]: hole ?addLineProduct has type Product.ProductId
    <al-button click={AddLine { product: ?addLineProduct }}>Add line</al-button>
                                         ^^^^^^^^^^^^^^^
app/Orders/Show.alder:59:16: warning[W0002]: hole ?expected has type Bool
    Task.succeed ?expected
                 ^^^^^^^^^
[exit 0]
$ alder new form Orders.Show NewLine quantity:Int.Positive --msg AddLine
edited app/Orders/Show.alder
--- a/app/Orders/Show.alder
+++ b/app/Orders/Show.alder
@@ -24,8 +24,13 @@
   } else Query.notFound
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
@@ -35,7 +40,15 @@
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
@@ -45,16 +58,11 @@
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
app/Orders/Show.alder:36:16: warning[W0002]: hole ?addLine has type Task Nav
    AddLine p -> ?addLine,
                 ^^^^^^^^
app/Orders/Show.alder:46:38: warning[W0002]: hole ?addLineProduct has type Product.ProductId
      <form submit={AddLine { product: ?addLineProduct }}>
                                       ^^^^^^^^^^^^^^^
app/Orders/Show.alder:66:44: warning[W0002]: hole ?addLineProduct has type Product.ProductId
    let r <- Test.submit (AddLine { product: ?addLineProduct }) { quantity: Int.Positive 1 }
                                             ^^^^^^^^^^^^^^^
app/Orders/Show.alder:67:29: warning[W0002]: hole ?expected has type Answer
    Task.succeed (r.answer == ?expected)
                              ^^^^^^^^^
[exit 0]
$ alder new msg Orders.Show Pay
edited app/Orders/Show.alder
--- a/app/Orders/Show.alder
+++ b/app/Orders/Show.alder
@@ -31,9 +31,12 @@
 
 msg AddLine { product: Product.ProductId } form NewLine
 
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
+  <al-button click={Pay}>Pay</al-button>
 </Layout.Page>
 
 test "a visitor is sent to log in" = {
@@ -66,3 +71,18 @@
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
app/Orders/Show.alder:38:16: warning[W0002]: hole ?addLine has type Task Nav
    AddLine p -> ?addLine,
                 ^^^^^^^^
app/Orders/Show.alder:39:12: warning[W0002]: hole ?pay has type Task Nav
    Pay p -> ?pay,
             ^^^^
app/Orders/Show.alder:49:38: warning[W0002]: hole ?addLineProduct has type Product.ProductId
      <form submit={AddLine { product: ?addLineProduct }}>
                                       ^^^^^^^^^^^^^^^
app/Orders/Show.alder:71:44: warning[W0002]: hole ?addLineProduct has type Product.ProductId
    let r <- Test.submit (AddLine { product: ?addLineProduct }) { quantity: Int.Positive 1 }
                                             ^^^^^^^^^^^^^^^
app/Orders/Show.alder:72:29: warning[W0002]: hole ?expected has type Answer
    Task.succeed (r.answer == ?expected)
                              ^^^^^^^^^
app/Orders/Show.alder:87:16: warning[W0002]: hole ?expected has type Bool
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
+  <al-button click={Cancel}>Cancel</al-button>
 </Layout.Page>
 
 test "a visitor is sent to log in" = {
@@ -86,3 +91,18 @@
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
app/Orders/Show.alder:52:38: warning[W0002]: hole ?addLineProduct has type Product.ProductId
      <form submit={AddLine { product: ?addLineProduct }}>
                                       ^^^^^^^^^^^^^^^
app/Orders/Show.alder:76:44: warning[W0002]: hole ?addLineProduct has type Product.ProductId
    let r <- Test.submit (AddLine { product: ?addLineProduct }) { quantity: Int.Positive 1 }
                                             ^^^^^^^^^^^^^^^
app/Orders/Show.alder:77:29: warning[W0002]: hole ?expected has type Answer
    Task.succeed (r.answer == ?expected)
                              ^^^^^^^^^
app/Orders/Show.alder:92:16: warning[W0002]: hole ?expected has type Bool
    Task.succeed ?expected
                 ^^^^^^^^^
app/Orders/Show.alder:107:16: warning[W0002]: hole ?expected has type Bool
    Task.succeed ?expected
                 ^^^^^^^^^
[exit 0]
$ alder fill --decl Orders.Show.update - <<'EOF'
match msg {
  AddLine p -> {
    if model.status == Open then {
      let _ <- Db.insert Line { order: model.id, product: p.product, quantity: p.form.quantity }
      Task.succeed (Redirect (Orders.Show { id: model.id }))
    } else Task.fail "the order is not open"
  },
  Pay p -> {
    if model.status == Open then {
      let _ <- Db.update Order model.id (\o -> { o | status: Paid })
      Task.succeed (Redirect (Orders.Show { id: model.id }))
    } else Task.fail "the order is not open"
  },
  Cancel p -> {
    if model.status == Open then {
      let _ <- Db.update Order model.id (\o -> { o | status: Cancelled })
      Task.succeed (Redirect (Orders.Show { id: model.id }))
    } else Task.fail "the order is not open"
  },
}
EOF
edited app/Orders/Show.alder
--- a/app/Orders/Show.alder
+++ b/app/Orders/Show.alder
@@ -37,9 +37,24 @@
 
 update : Msg -> Model -> Session -> Task Nav
 update msg model session = match msg {
-  AddLine p -> ?addLine,
-  Pay p -> ?pay,
-  Cancel p -> ?cancel,
+  AddLine p -> {
+    if model.status == Open then {
+      let _ <- Db.insert Line { order: model.id, product: p.product, quantity: p.form.quantity }
+      Task.succeed (Redirect (Orders.Show { id: model.id }))
+    } else Task.fail "the order is not open"
+  },
+  Pay p -> {
+    if model.status == Open then {
+      let _ <- Db.update Order model.id (\o -> { o | status: Paid })
+      Task.succeed (Redirect (Orders.Show { id: model.id }))
+    } else Task.fail "the order is not open"
+  },
+  Cancel p -> {
+    if model.status == Open then {
+      let _ <- Db.update Order model.id (\o -> { o | status: Cancelled })
+      Task.succeed (Redirect (Orders.Show { id: model.id }))
+    } else Task.fail "the order is not open"
+  },
 }
 
 view : Model -> Session -> Html Msg
app/Orders/Show.alder:67:38: warning[W0002]: hole ?addLineProduct has type Product.ProductId
      <form submit={AddLine { product: ?addLineProduct }}>
                                       ^^^^^^^^^^^^^^^
app/Orders/Show.alder:91:44: warning[W0002]: hole ?addLineProduct has type Product.ProductId
    let r <- Test.submit (AddLine { product: ?addLineProduct }) { quantity: Int.Positive 1 }
                                             ^^^^^^^^^^^^^^^
app/Orders/Show.alder:92:29: warning[W0002]: hole ?expected has type Answer
    Task.succeed (r.answer == ?expected)
                              ^^^^^^^^^
app/Orders/Show.alder:107:16: warning[W0002]: hole ?expected has type Bool
    Task.succeed ?expected
                 ^^^^^^^^^
app/Orders/Show.alder:122:16: warning[W0002]: hole ?expected has type Bool
    Task.succeed ?expected
                 ^^^^^^^^^
[exit 0]
$ alder fill --decl Orders.Show.view - <<'EOF'
<Layout.Page title="Order">
  <h1>Order</h1>
  <p>Status: <span id="status">{Order.statusLabel model.status}</span></p>
  <ul id="lines">
    {for l in model.lines ->
      <li>{l.name} x {Text.fromInt l.quantity} = {Text.fromInt l.cost}</li>}
  </ul>
  <p>Total: <span id="total">{Text.fromInt model.total}</span></p>
  {if model.status == Open then
    <div>
      {for p in model.products ->
        <form submit={AddLine { product: p.id }}>
          <al-input field="quantity" label="Quantity" />
          <al-button type="submit">Add {p.name}</al-button>
        </form>}
      <al-button click={Pay}>Mark paid</al-button>
      <al-button variant="destructive" click={Cancel}>Cancel order</al-button>
    </div> else Html.none}
  <a href={Orders.Index {}}>Back to orders</a>
</Layout.Page>
EOF
edited app/Orders/Show.alder
--- a/app/Orders/Show.alder
+++ b/app/Orders/Show.alder
@@ -58,23 +58,27 @@
 }
 
 view : Model -> Session -> Html Msg
-view model session = <Layout.Page title="Order">
-  <h1>Order</h1>
-
-
-  <al-card>
-    <h2 slot="header">New line</h2>
-    <form submit={AddLine { product: ?addLineProduct }}>
-      <al-input field="quantity" label="Quantity" />
-      <al-alert variant="destructive"><Errors form /></al-alert>
-      <al-button type="submit">New line</al-button>
-    </form>
-  </al-card>
-
-  <al-button click={Pay}>Pay</al-button>
-
-  <al-button click={Cancel}>Cancel</al-button>
-</Layout.Page>
+view model session =
+  <Layout.Page title="Order">
+    <h1>Order</h1>
+    <p>Status: <span id="status">{Order.statusLabel model.status}</span></p>
+    <ul id="lines">
+      {for l in model.lines ->
+        <li>{l.name} x {Text.fromInt l.quantity} = {Text.fromInt l.cost}</li>}
+    </ul>
+    <p>Total: <span id="total">{Text.fromInt model.total}</span></p>
+    {if model.status == Open then
+      <div>
+        {for p in model.products ->
+          <form submit={AddLine { product: p.id }}>
+            <al-input field="quantity" label="Quantity" />
+            <al-button type="submit">Add {p.name}</al-button>
+          </form>}
+        <al-button click={Pay}>Mark paid</al-button>
+        <al-button variant="destructive" click={Cancel}>Cancel order</al-button>
+      </div> else Html.none}
+    <a href={Orders.Index {}}>Back to orders</a>
+  </Layout.Page>
 
 test "a visitor is sent to log in" = {
   let me <- Db.insert User { email: Text.Email "email-1@example.com", name: Text.NonEmpty "name-1", passwordHash: "passwordhash-1" }
app/Orders/Show.alder:95:44: warning[W0002]: hole ?addLineProduct has type Product.ProductId
    let r <- Test.submit (AddLine { product: ?addLineProduct }) { quantity: Int.Positive 1 }
                                             ^^^^^^^^^^^^^^^
app/Orders/Show.alder:96:29: warning[W0002]: hole ?expected has type Answer
    Task.succeed (r.answer == ?expected)
                              ^^^^^^^^^
app/Orders/Show.alder:111:16: warning[W0002]: hole ?expected has type Bool
    Task.succeed ?expected
                 ^^^^^^^^^
app/Orders/Show.alder:126:16: warning[W0002]: hole ?expected has type Bool
    Task.succeed ?expected
                 ^^^^^^^^^
[exit 0]
$ alder new page Products.Index '/products' --blank --requires user
wrote app/Products/Index.alder
app/Products/Index.alder:8:23: warning[W0002]: hole ?load has type Query Products.Index.Model
  load params session = ?load
                        ^^^^^
app/Products/Index.alder:11:22: warning[W0002]: hole ?view has type Html Products.Index.Msg
  view model session = ?view
                       ^^^^^
[exit 0]
$ alder fill --decl Products.Index.Model - <<'EOF'
{ products: List { id: Product.ProductId, name: Text.NonEmpty, price: Int.NonNegative, active: Bool } }
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
+type Model = { products: List { id: Product.ProductId, name: Text.NonEmpty, price: Int.NonNegative, active: Bool } }
 
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
    Query.succeed { products: List.map (\p -> { id: p.id, name: p.name, price: p.price, active: p.active }) products }
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
 
 type Model = { products: List { id: Product.ProductId, name: Text.NonEmpty, price: Int.NonNegative, active: Bool } }
 
 load : Params -> Session -> Query Model
-load params session = ?load
+load params session = {
+  let mgr <- User.isManager session.user
+  if mgr then {
+    let products <- Db.from Product |> Db.orderBy Product.name.asc |> Db.all
+    Query.succeed { products: List.map (\p -> { id: p.id, name: p.name, price: p.price, active: p.active }) products }
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
+  <h1>Products</h1>
+</Layout.Page>
 
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
+  <al-card>
+    <h2 slot="header">New product</h2>
+    <form submit={Add}>
+      <al-input field="name" label="Name" />
+      <al-input field="price" label="Price" />
+      <al-alert variant="destructive"><Errors form /></al-alert>
+      <al-button type="submit">New product</al-button>
+    </form>
+  </al-card>
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
+  <al-button click={Toggle { id: ?toggleId }}>Toggle</al-button>
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
app/Products/Index.alder:52:34: warning[W0002]: hole ?toggleId has type Product.ProductId
    <al-button click={Toggle { id: ?toggleId }}>Toggle</al-button>
                                   ^^^^^^^^^
app/Products/Index.alder:88:16: warning[W0002]: hole ?expected has type Bool
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
app/Products/Index.alder:55:34: warning[W0002]: hole ?toggleId has type Product.ProductId
    <al-button click={Toggle { id: ?toggleId }}>Toggle</al-button>
                                   ^^^^^^^^^
app/Products/Index.alder:91:16: warning[W0002]: hole ?expected has type Bool
    Task.succeed ?expected
                 ^^^^^^^^^
[exit 0]
$ alder fill --decl Products.Index.view - <<'EOF'
<Layout.Page title="Products">
  <h1>Products</h1>
  <al-card>
    <form submit={Add}>
      <Errors form />
      <al-input field="name" label="Name" />
      <al-input field="price" label="Price in cents" />
      <al-button type="submit">Add</al-button>
    </form>
  </al-card>
  <ul id="products">
    {for p in model.products ->
      <li>
        <span class="name">{p.name}</span>
        <span class="price">{Text.fromInt p.price}</span>
        <span class="active">{if p.active then "active" else "inactive"}</span>
        <al-button variant="secondary" size="sm" click={Toggle { id: p.id }}>{if p.active then "Deactivate" else "Activate"}</al-button>
      </li>}
  </ul>
</Layout.Page>
EOF
edited app/Products/Index.alder
--- a/app/Products/Index.alder
+++ b/app/Products/Index.alder
@@ -39,21 +39,27 @@
 }
 
 view : Model -> Session -> Html Msg
-view model session = <Layout.Page title="Products">
-  <h1>Products</h1>
-
-  <al-card>
-    <h2 slot="header">New product</h2>
-    <form submit={Add}>
-      <al-input field="name" label="Name" />
-      <al-input field="price" label="Price" />
-      <al-alert variant="destructive"><Errors form /></al-alert>
-      <al-button type="submit">New product</al-button>
-    </form>
-  </al-card>
-
-  <al-button click={Toggle { id: ?toggleId }}>Toggle</al-button>
-</Layout.Page>
+view model session =
+  <Layout.Page title="Products">
+    <h1>Products</h1>
+    <al-card>
+      <form submit={Add}>
+        <Errors form />
+        <al-input field="name" label="Name" />
+        <al-input field="price" label="Price in cents" />
+        <al-button type="submit">Add</al-button>
+      </form>
+    </al-card>
+    <ul id="products">
+      {for p in model.products ->
+        <li>
+          <span class="name">{p.name}</span>
+          <span class="price">{Text.fromInt p.price}</span>
+          <span class="active">{if p.active then "active" else "inactive"}</span>
+          <al-button variant="secondary" size="sm" click={Toggle { id: p.id }}>{if p.active then "Deactivate" else "Activate"}</al-button>
+        </li>}
+    </ul>
+  </Layout.Page>
 
 test "a visitor is sent to log in" = {
   let r <- Test.open (Products.Index {})
app/Products/Index.alder:97:16: warning[W0002]: hole ?expected has type Bool
    Task.succeed ?expected
                 ^^^^^^^^^
[exit 0]
$ alder new test Orders.Show "adding a line to a Paid order is refused"
edited app/Orders/Show.alder
--- a/app/Orders/Show.alder
+++ b/app/Orders/Show.alder
@@ -125,3 +125,7 @@
   let after <- Test.open (Orders.Show { id: order1 })
   Task.succeed ?expected
 }
+
+test "adding a line to a Paid order is refused" = {
+  ?test
+}
app/Orders/Show.alder:95:44: warning[W0002]: hole ?addLineProduct has type Product.ProductId
    let r <- Test.submit (AddLine { product: ?addLineProduct }) { quantity: Int.Positive 1 }
                                             ^^^^^^^^^^^^^^^
app/Orders/Show.alder:96:29: warning[W0002]: hole ?expected has type Answer
    Task.succeed (r.answer == ?expected)
                              ^^^^^^^^^
app/Orders/Show.alder:111:16: warning[W0002]: hole ?expected has type Bool
    Task.succeed ?expected
                 ^^^^^^^^^
app/Orders/Show.alder:126:16: warning[W0002]: hole ?expected has type Bool
    Task.succeed ?expected
                 ^^^^^^^^^
app/Orders/Show.alder:130:3: warning[W0002]: hole ?test has type Task Bool
    ?test
    ^^^^^
[exit 0]
$ alder fill 'Orders.Show."adding a line to a Paid order is refused"' - <<'EOF'
{
  let ann <- Db.insert User { email: Text.Email "ann@example.com", name: Text.NonEmpty "Ann", passwordHash: "" }
  let tea <- Db.insert Product { name: Text.NonEmpty "Tea", price: Int.NonNegative 250 }
  let o <- Db.insert Order { owner: ann }
  let _ <- Test.login { user: Just ann }
  let _ <- Test.get (Orders.Show { id: o })
  let _ <- Db.update Order o (\x -> { x | status: Paid })
  let r <- Test.submit (AddLine { product: tea }) { quantity: Int.Positive 2 }
  let lines <- Db.from Line |> Db.count
  Task.succeed ({ answer: r.answer, lines: lines } == { answer: RequestFailed "the order is not open", lines: 0 })
}
EOF
edited app/Orders/Show.alder
--- a/app/Orders/Show.alder
+++ b/app/Orders/Show.alder
@@ -127,5 +127,13 @@
 }
 
 test "adding a line to a Paid order is refused" = {
-  ?test
+  let ann <- Db.insert User { email: Text.Email "ann@example.com", name: Text.NonEmpty "Ann", passwordHash: "" }
+  let tea <- Db.insert Product { name: Text.NonEmpty "Tea", price: Int.NonNegative 250 }
+  let o <- Db.insert Order { owner: ann }
+  let _ <- Test.login { user: Just ann }
+  let _ <- Test.get (Orders.Show { id: o })
+  let _ <- Db.update Order o (\x -> { x | status: Paid })
+  let r <- Test.submit (AddLine { product: tea }) { quantity: Int.Positive 2 }
+  let lines <- Db.from Line |> Db.count
+  Task.succeed ({ answer: r.answer, lines: lines } == { answer: RequestFailed "the order is not open", lines: 0 })
 }
app/Orders/Show.alder:95:44: warning[W0002]: hole ?addLineProduct has type Product.ProductId
    let r <- Test.submit (AddLine { product: ?addLineProduct }) { quantity: Int.Positive 1 }
                                             ^^^^^^^^^^^^^^^
app/Orders/Show.alder:96:29: warning[W0002]: hole ?expected has type Answer
    Task.succeed (r.answer == ?expected)
                              ^^^^^^^^^
app/Orders/Show.alder:111:16: warning[W0002]: hole ?expected has type Bool
    Task.succeed ?expected
                 ^^^^^^^^^
app/Orders/Show.alder:126:16: warning[W0002]: hole ?expected has type Bool
    Task.succeed ?expected
                 ^^^^^^^^^
[exit 0]
$ alder new test Orders.Show "a Staff user is not found on another order"
edited app/Orders/Show.alder
--- a/app/Orders/Show.alder
+++ b/app/Orders/Show.alder
@@ -137,3 +137,7 @@
   let lines <- Db.from Line |> Db.count
   Task.succeed ({ answer: r.answer, lines: lines } == { answer: RequestFailed "the order is not open", lines: 0 })
 }
+
+test "a Staff user is not found on another order" = {
+  ?test
+}
app/Orders/Show.alder:95:44: warning[W0002]: hole ?addLineProduct has type Product.ProductId
    let r <- Test.submit (AddLine { product: ?addLineProduct }) { quantity: Int.Positive 1 }
                                             ^^^^^^^^^^^^^^^
app/Orders/Show.alder:96:29: warning[W0002]: hole ?expected has type Answer
    Task.succeed (r.answer == ?expected)
                              ^^^^^^^^^
app/Orders/Show.alder:111:16: warning[W0002]: hole ?expected has type Bool
    Task.succeed ?expected
                 ^^^^^^^^^
app/Orders/Show.alder:126:16: warning[W0002]: hole ?expected has type Bool
    Task.succeed ?expected
                 ^^^^^^^^^
app/Orders/Show.alder:142:3: warning[W0002]: hole ?test has type Task Bool
    ?test
    ^^^^^
[exit 0]
$ alder fill 'Orders.Show."a Staff user is not found on another order"' - <<'EOF'
{
  let ann <- Db.insert User { email: Text.Email "ann@example.com", name: Text.NonEmpty "Ann", passwordHash: "" }
  let bob <- Db.insert User { email: Text.Email "bob@example.com", name: Text.NonEmpty "Bob", passwordHash: "" }
  let o <- Db.insert Order { owner: ann }
  let _ <- Test.login { user: Just bob }
  let r <- Test.open (Orders.Show { id: o })
  Task.succeed (r.answer == PageMissing)
}
EOF
edited app/Orders/Show.alder
--- a/app/Orders/Show.alder
+++ b/app/Orders/Show.alder
@@ -139,5 +139,10 @@
 }
 
 test "a Staff user is not found on another order" = {
-  ?test
+  let ann <- Db.insert User { email: Text.Email "ann@example.com", name: Text.NonEmpty "Ann", passwordHash: "" }
+  let bob <- Db.insert User { email: Text.Email "bob@example.com", name: Text.NonEmpty "Bob", passwordHash: "" }
+  let o <- Db.insert Order { owner: ann }
+  let _ <- Test.login { user: Just bob }
+  let r <- Test.open (Orders.Show { id: o })
+  Task.succeed (r.answer == PageMissing)
 }
app/Orders/Show.alder:95:44: warning[W0002]: hole ?addLineProduct has type Product.ProductId
    let r <- Test.submit (AddLine { product: ?addLineProduct }) { quantity: Int.Positive 1 }
                                             ^^^^^^^^^^^^^^^
app/Orders/Show.alder:96:29: warning[W0002]: hole ?expected has type Answer
    Task.succeed (r.answer == ?expected)
                              ^^^^^^^^^
app/Orders/Show.alder:111:16: warning[W0002]: hole ?expected has type Bool
    Task.succeed ?expected
                 ^^^^^^^^^
app/Orders/Show.alder:126:16: warning[W0002]: hole ?expected has type Bool
    Task.succeed ?expected
                 ^^^^^^^^^
[exit 0]
$ alder new test Orders.Show "a Manager sees another order"
edited app/Orders/Show.alder
--- a/app/Orders/Show.alder
+++ b/app/Orders/Show.alder
@@ -146,3 +146,7 @@
   let r <- Test.open (Orders.Show { id: o })
   Task.succeed (r.answer == PageMissing)
 }
+
+test "a Manager sees another order" = {
+  ?test
+}
app/Orders/Show.alder:95:44: warning[W0002]: hole ?addLineProduct has type Product.ProductId
    let r <- Test.submit (AddLine { product: ?addLineProduct }) { quantity: Int.Positive 1 }
                                             ^^^^^^^^^^^^^^^
app/Orders/Show.alder:96:29: warning[W0002]: hole ?expected has type Answer
    Task.succeed (r.answer == ?expected)
                              ^^^^^^^^^
app/Orders/Show.alder:111:16: warning[W0002]: hole ?expected has type Bool
    Task.succeed ?expected
                 ^^^^^^^^^
app/Orders/Show.alder:126:16: warning[W0002]: hole ?expected has type Bool
    Task.succeed ?expected
                 ^^^^^^^^^
app/Orders/Show.alder:151:3: warning[W0002]: hole ?test has type Task Bool
    ?test
    ^^^^^
[exit 0]
$ alder fill 'Orders.Show."a Manager sees another order"' - <<'EOF'
{
  let ann <- Db.insert User { email: Text.Email "ann@example.com", name: Text.NonEmpty "Ann", passwordHash: "" }
  let boss <- Db.insert User { email: Text.Email "boss@example.com", name: Text.NonEmpty "Boss", passwordHash: "", role: Manager }
  let o <- Db.insert Order { owner: ann }
  let _ <- Test.login { user: Just boss }
  let r <- Test.open (Orders.Show { id: o })
  Task.succeed (r.answer == Shown)
}
EOF
edited app/Orders/Show.alder
--- a/app/Orders/Show.alder
+++ b/app/Orders/Show.alder
@@ -148,5 +148,10 @@
 }
 
 test "a Manager sees another order" = {
-  ?test
+  let ann <- Db.insert User { email: Text.Email "ann@example.com", name: Text.NonEmpty "Ann", passwordHash: "" }
+  let boss <- Db.insert User { email: Text.Email "boss@example.com", name: Text.NonEmpty "Boss", passwordHash: "", role: Manager }
+  let o <- Db.insert Order { owner: ann }
+  let _ <- Test.login { user: Just boss }
+  let r <- Test.open (Orders.Show { id: o })
+  Task.succeed (r.answer == Shown)
 }
app/Orders/Show.alder:95:44: warning[W0002]: hole ?addLineProduct has type Product.ProductId
    let r <- Test.submit (AddLine { product: ?addLineProduct }) { quantity: Int.Positive 1 }
                                             ^^^^^^^^^^^^^^^
app/Orders/Show.alder:96:29: warning[W0002]: hole ?expected has type Answer
    Task.succeed (r.answer == ?expected)
                              ^^^^^^^^^
app/Orders/Show.alder:111:16: warning[W0002]: hole ?expected has type Bool
    Task.succeed ?expected
                 ^^^^^^^^^
app/Orders/Show.alder:126:16: warning[W0002]: hole ?expected has type Bool
    Task.succeed ?expected
                 ^^^^^^^^^
[exit 0]
$ alder new test Products.Index "a Staff user gets not found on products"
edited app/Products/Index.alder
--- a/app/Products/Index.alder
+++ b/app/Products/Index.alder
@@ -96,3 +96,7 @@
   let after <- Db.get Product product1
   Task.succeed ?expected
 }
+
+test "a Staff user gets not found on products" = {
+  ?test
+}
app/Products/Index.alder:97:16: warning[W0002]: hole ?expected has type Bool
    Task.succeed ?expected
                 ^^^^^^^^^
app/Products/Index.alder:101:3: warning[W0002]: hole ?test has type Task Bool
    ?test
    ^^^^^
[exit 0]
$ alder fill 'Products.Index."a Staff user gets not found on products"' - <<'EOF'
{
  let ann <- Db.insert User { email: Text.Email "ann@example.com", name: Text.NonEmpty "Ann", passwordHash: "" }
  let _ <- Test.login { user: Just ann }
  let r <- Test.open (Products.Index {})
  Task.succeed (r.answer == PageMissing)
}
EOF
edited app/Products/Index.alder
--- a/app/Products/Index.alder
+++ b/app/Products/Index.alder
@@ -98,5 +98,8 @@
 }
 
 test "a Staff user gets not found on products" = {
-  ?test
+  let ann <- Db.insert User { email: Text.Email "ann@example.com", name: Text.NonEmpty "Ann", passwordHash: "" }
+  let _ <- Test.login { user: Just ann }
+  let r <- Test.open (Products.Index {})
+  Task.succeed (r.answer == PageMissing)
 }
app/Products/Index.alder:97:16: warning[W0002]: hole ?expected has type Bool
    Task.succeed ?expected
                 ^^^^^^^^^
[exit 0]
```

The program type-checks but `alder build` refused it; fix the cause below.

```text
app/Orders/Index.alder:66:16: warning[W0002]: hole ?expected has type Bool
    Task.succeed ?expected
                 ^^^^^^^^^
app/Orders/Show.alder:95:44: warning[W0002]: hole ?addLineProduct has type Product.ProductId
    let r <- Test.submit (AddLine { product: ?addLineProduct }) { quantity: Int.Positive 1 }
                                             ^^^^^^^^^^^^^^^
app/Orders/Show.alder:96:29: warning[W0002]: hole ?expected has type Answer
    Task.succeed (r.answer == ?expected)
                              ^^^^^^^^^
app/Orders/Show.alder:111:16: warning[W0002]: hole ?expected has type Bool
    Task.succeed ?expected
                 ^^^^^^^^^
app/Orders/Show.alder:126:16: warning[W0002]: hole ?expected has type Bool
    Task.succeed ?expected
                 ^^^^^^^^^
app/Products/Index.alder:97:16: warning[W0002]: hole ?expected has type Bool
    Task.succeed ?expected
                 ^^^^^^^^^
alder build: the program has 6 typed hole(s); fill them before building
```
