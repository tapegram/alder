### Round 2

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
$ alder new test Line "an open order totals its lines"
edited app/Line.alder
--- a/app/Line.alder
+++ b/app/Line.alder
@@ -38,3 +38,7 @@
   let left <- Db.from Line |> Db.count
   Task.succeed ({ deleted: Result.toMaybe r.value, left: left } == { deleted: Nothing, left: 1 })
 }
+
+test "an open order totals its lines" = {
+  ?test
+}
app/Line.alder:43:3: warning[W0002]: hole ?test has type Task Bool
    ?test
    ^^^^^
[exit 0]
$ alder fill 'Line."an open order totals its lines"' - <<'EOF'
{
  let ann <- Db.insert User { email: Text.Email "ann@example.com", name: Text.NonEmpty "Ann", passwordHash: "" }
  let tea <- Db.insert Product { name: Text.NonEmpty "Tea", price: Int.NonNegative 250 }
  let mug <- Db.insert Product { name: Text.NonEmpty "Mug", price: Int.NonNegative 1000 }
  let o <- Db.insert Order { owner: ann }
  let _ <- Db.insert Line { order: o, product: tea, quantity: Int.Positive 2 }
  let _ <- Db.insert Line { order: o, product: mug, quantity: Int.Positive 1 }
  let products <- Db.from Product |> Db.all
  let lines <- Db.from Line |> Db.all
  let row <- Db.get Order o
  Task.succeed ({ total: orderTotal products lines, status: Maybe.map (\x -> x.status) row } == { total: 1500, status: Just Open })
}
EOF
edited app/Line.alder
--- a/app/Line.alder
+++ b/app/Line.alder
@@ -40,5 +40,14 @@
 }
 
 test "an open order totals its lines" = {
-  ?test
+  let ann <- Db.insert User { email: Text.Email "ann@example.com", name: Text.NonEmpty "Ann", passwordHash: "" }
+  let tea <- Db.insert Product { name: Text.NonEmpty "Tea", price: Int.NonNegative 250 }
+  let mug <- Db.insert Product { name: Text.NonEmpty "Mug", price: Int.NonNegative 1000 }
+  let o <- Db.insert Order { owner: ann }
+  let _ <- Db.insert Line { order: o, product: tea, quantity: Int.Positive 2 }
+  let _ <- Db.insert Line { order: o, product: mug, quantity: Int.Positive 1 }
+  let products <- Db.from Product |> Db.all
+  let lines <- Db.from Line |> Db.all
+  let row <- Db.get Order o
+  Task.succeed ({ total: orderTotal products lines, status: Maybe.map (\x -> x.status) row } == { total: 1500, status: Just Open })
 }
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
$ alder new msg Orders.Show Pay
edited app/Orders/Show.alder
--- a/app/Orders/Show.alder
+++ b/app/Orders/Show.alder
@@ -24,9 +24,18 @@
   }
 }
 
+msg Pay
+
+update : Msg -> Model -> Session -> Task Nav
+update msg model session = match msg {
+  Pay p -> ?pay,
+}
+
 view : Model -> Session -> Html Msg
 view model session = <Layout.Page title="Order">
     <h1>Order</h1>
+
+    <al-button click={Pay}>Pay</al-button>
   </Layout.Page>
 
 test "a visitor is sent to log in" = {
@@ -35,3 +44,18 @@
   let r <- Test.open (Orders.Show { id: order1 })
   Task.succeed (r.answer == Redirected (Auth.Login {}))
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
app/Orders/Show.alder:31:12: warning[W0002]: hole ?pay has type Task Nav
    Pay p -> ?pay,
             ^^^^
app/Orders/Show.alder:60:16: warning[W0002]: hole ?expected has type Bool
    Task.succeed ?expected
                 ^^^^^^^^^
[exit 0]
$ alder new msg Orders.Show Cancel
edited app/Orders/Show.alder
--- a/app/Orders/Show.alder
+++ b/app/Orders/Show.alder
@@ -26,9 +26,12 @@
 
 msg Pay
 
+msg Cancel
+
 update : Msg -> Model -> Session -> Task Nav
 update msg model session = match msg {
   Pay p -> ?pay,
+  Cancel p -> ?cancel,
 }
 
 view : Model -> Session -> Html Msg
@@ -36,6 +39,8 @@
     <h1>Order</h1>
 
     <al-button click={Pay}>Pay</al-button>
+
+    <al-button click={Cancel}>Cancel</al-button>
   </Layout.Page>
 
 test "a visitor is sent to log in" = {
@@ -59,3 +64,18 @@
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
app/Orders/Show.alder:33:12: warning[W0002]: hole ?pay has type Task Nav
    Pay p -> ?pay,
             ^^^^
app/Orders/Show.alder:34:15: warning[W0002]: hole ?cancel has type Task Nav
    Cancel p -> ?cancel,
                ^^^^^^^
app/Orders/Show.alder:65:16: warning[W0002]: hole ?expected has type Bool
    Task.succeed ?expected
                 ^^^^^^^^^
app/Orders/Show.alder:80:16: warning[W0002]: hole ?expected has type Bool
    Task.succeed ?expected
                 ^^^^^^^^^
[exit 0]
$ alder new msg Orders.Show AddLine product:Product.ProductId
edited app/Orders/Show.alder
--- a/app/Orders/Show.alder
+++ b/app/Orders/Show.alder
@@ -28,10 +28,13 @@
 
 msg Cancel
 
+msg AddLine { product: Product.ProductId }
+
 update : Msg -> Model -> Session -> Task Nav
 update msg model session = match msg {
   Pay p -> ?pay,
   Cancel p -> ?cancel,
+  AddLine p -> ?addLine,
 }
 
 view : Model -> Session -> Html Msg
@@ -41,6 +44,8 @@
     <al-button click={Pay}>Pay</al-button>
 
     <al-button click={Cancel}>Cancel</al-button>
+
+    <al-button click={AddLine { product: ?addLineProduct }}>Add line</al-button>
   </Layout.Page>
 
 test "a visitor is sent to log in" = {
@@ -79,3 +84,17 @@
   let after <- Test.open (Orders.Show { id: order1 })
   Task.succeed ?expected
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
app/Orders/Show.alder:35:12: warning[W0002]: hole ?pay has type Task Nav
    Pay p -> ?pay,
             ^^^^
app/Orders/Show.alder:36:15: warning[W0002]: hole ?cancel has type Task Nav
    Cancel p -> ?cancel,
                ^^^^^^^
app/Orders/Show.alder:37:16: warning[W0002]: hole ?addLine has type Task Nav
    AddLine p -> ?addLine,
                 ^^^^^^^^
app/Orders/Show.alder:48:42: warning[W0002]: hole ?addLineProduct has type Product.ProductId
      <al-button click={AddLine { product: ?addLineProduct }}>Add line</al-button>
                                           ^^^^^^^^^^^^^^^
app/Orders/Show.alder:70:16: warning[W0002]: hole ?expected has type Bool
    Task.succeed ?expected
                 ^^^^^^^^^
app/Orders/Show.alder:85:16: warning[W0002]: hole ?expected has type Bool
    Task.succeed ?expected
                 ^^^^^^^^^
app/Orders/Show.alder:99:16: warning[W0002]: hole ?expected has type Bool
    Task.succeed ?expected
                 ^^^^^^^^^
[exit 0]
$ alder new form Orders.Show NewLine quantity:Int.Positive --msg AddLine
alder new form: `Orders.Show` already declares `AddLine`: choose another name
[exit 1]
```

The harness stopped at line 50 of your script: `alder new form Orders.Show NewLine quantity:Int.Positive --msg AddLine`: it exited 1. The commands after it were not run.

Your script stopped at command 11; the app is as the commands before it left it; continue.

The program type-checks but `alder build` refused it; fix the cause below.

```text
app/Auth/Login.alder:86:85: warning[W0002]: hole ?afterLogin has type Page
    Task.succeed ({ answer: r.answer, user: r.session.user } == { answer: Redirected (?afterLogin), user: Just me })
                                                                                      ^^^^^^^^^^^
app/Orders/Index.alder:12:23: warning[W0002]: hole ?load has type Query Orders.Index.Model
  load params session = ?load
                        ^^^^^
app/Orders/Index.alder:15:22: warning[W0002]: hole ?view has type Html Orders.Index.Msg
  view model session = ?view
                       ^^^^^
app/Orders/Show.alder:35:12: warning[W0002]: hole ?pay has type Task Nav
    Pay p -> ?pay,
             ^^^^
app/Orders/Show.alder:36:15: warning[W0002]: hole ?cancel has type Task Nav
    Cancel p -> ?cancel,
                ^^^^^^^
app/Orders/Show.alder:37:16: warning[W0002]: hole ?addLine has type Task Nav
    AddLine p -> ?addLine,
                 ^^^^^^^^
app/Orders/Show.alder:48:42: warning[W0002]: hole ?addLineProduct has type Product.ProductId
      <al-button click={AddLine { product: ?addLineProduct }}>Add line</al-button>
                                           ^^^^^^^^^^^^^^^
app/Orders/Show.alder:70:16: warning[W0002]: hole ?expected has type Bool
    Task.succeed ?expected
                 ^^^^^^^^^
app/Orders/Show.alder:85:16: warning[W0002]: hole ?expected has type Bool
    Task.succeed ?expected
                 ^^^^^^^^^
app/Orders/Show.alder:99:16: warning[W0002]: hole ?expected has type Bool
    Task.succeed ?expected
                 ^^^^^^^^^
alder build: the program has 10 typed hole(s); fill them before building
```
