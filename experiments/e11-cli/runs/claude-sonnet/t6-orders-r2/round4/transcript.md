### Round 4

```text
$ alder fill Orders.Index.update.Create - <<'EOF'
{
  let id <- Db.insert Order { owner: session.user }
  Task.succeed (Redirect (Orders.Show { id: id }))
}
EOF
edited app/Orders/Index.alder
--- a/app/Orders/Index.alder
+++ b/app/Orders/Index.alder
@@ -20,7 +20,10 @@
 
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
$ alder fill 'Orders.Index."pressing Create"' --hole ?expected - <<'EOF'
Test.count "#orders li" after.doc == 1
EOF
edited app/Orders/Index.alder
--- a/app/Orders/Index.alder
+++ b/app/Orders/Index.alder
@@ -48,16 +48,11 @@
   Task.succeed (r.answer == Redirected (Auth.Login {}))
 }
 
--- Fill ?expected with what pressing Create must do, so that a test fails when the arm
--- does something else: its answer (`r.answer`: Rerendered, Redirected …) and what it
--- changed (`after`: the page opened again, `after.answer` Shown, or PageMissing when the arm
--- deleted the page's row, and `after.doc` its HTML), as
--- `{ answer: r.answer, … } == { answer: Rerendered, … }`.
 test "pressing Create" = {
   let me <- Db.insert User { email: Text.Email "email-1@example.com", name: Text.NonEmpty "name-1", passwordHash: "passwordhash-1" }
   let _ <- Test.login { user: Just me }
   let _ <- Test.get (Orders.Index {})
   let r <- Test.click Create
   let after <- Test.open (Orders.Index {})
-  Task.succeed ?expected
+  Task.succeed (Test.count "#orders li" after.doc == 1)
 }
[exit 0]
$ alder fill Orders.Show.update.AddLine - <<'EOF'
if model.order.status != Open then Task.succeed (Reject "This order is not open.")
else if not (List.any (\q -> q.id == p.product) model.products) then Task.succeed (Reject "This product is not available.")
else {
  let _ <- Db.insert Line { order: model.order.id, product: p.product, quantity: p.form.quantity }
  Task.succeed (Redirect (Orders.Show { id: model.order.id }))
}
EOF
edited app/Orders/Show.alder
--- a/app/Orders/Show.alder
+++ b/app/Orders/Show.alder
@@ -40,7 +40,12 @@
 
 update : Msg -> Model -> Session -> Task Nav
 update msg model session = match msg {
-  AddLine p -> ?addLine,
+  AddLine p -> if model.order.status != Open then Task.succeed (Reject "This order is not open.")
+  else if not (List.any (\q -> q.id == p.product) model.products) then Task.succeed (Reject "This product is not available.")
+  else {
+    let _ <- Db.insert Line { order: model.order.id, product: p.product, quantity: p.form.quantity }
+    Task.succeed (Redirect (Orders.Show { id: model.order.id }))
+  },
   Pay p -> ?pay,
   Cancel p -> ?cancel,
 }
app/Orders/Show.alder:49:12: warning[W0002]: hole ?pay has type Task Nav
    Pay p -> ?pay,
             ^^^^
app/Orders/Show.alder:50:15: warning[W0002]: hole ?cancel has type Task Nav
    Cancel p -> ?cancel,
                ^^^^^^^
app/Orders/Show.alder:60:38: warning[W0002]: hole ?addLineProduct has type Product.ProductId
      <form submit={AddLine { product: ?addLineProduct }}>
                                       ^^^^^^^^^^^^^^^
app/Orders/Show.alder:84:44: warning[W0002]: hole ?addLineProduct has type Product.ProductId
    let r <- Test.submit (AddLine { product: ?addLineProduct }) { quantity: Int.Positive 1 }
                                             ^^^^^^^^^^^^^^^
app/Orders/Show.alder:85:29: warning[W0002]: hole ?expected has type Answer
    Task.succeed (r.answer == ?expected)
                              ^^^^^^^^^
app/Orders/Show.alder:100:16: warning[W0002]: hole ?expected has type Bool
    Task.succeed ?expected
                 ^^^^^^^^^
app/Orders/Show.alder:115:16: warning[W0002]: hole ?expected has type Bool
    Task.succeed ?expected
                 ^^^^^^^^^
[exit 0]
$ alder fill Orders.Show.update.Pay - <<'EOF'
if model.order.status == Open then {
  let _ <- Db.update Order model.order.id (\o -> { o | status: Paid })
  Task.succeed (Redirect (Orders.Show { id: model.order.id }))
} else Task.succeed (Reject "This order is not open.")
EOF
edited app/Orders/Show.alder
--- a/app/Orders/Show.alder
+++ b/app/Orders/Show.alder
@@ -46,7 +46,10 @@
     let _ <- Db.insert Line { order: model.order.id, product: p.product, quantity: p.form.quantity }
     Task.succeed (Redirect (Orders.Show { id: model.order.id }))
   },
-  Pay p -> ?pay,
+  Pay p -> if model.order.status == Open then {
+    let _ <- Db.update Order model.order.id (\o -> { o | status: Paid })
+    Task.succeed (Redirect (Orders.Show { id: model.order.id }))
+  } else Task.succeed (Reject "This order is not open."),
   Cancel p -> ?cancel,
 }
 
app/Orders/Show.alder:53:15: warning[W0002]: hole ?cancel has type Task Nav
    Cancel p -> ?cancel,
                ^^^^^^^
app/Orders/Show.alder:63:38: warning[W0002]: hole ?addLineProduct has type Product.ProductId
      <form submit={AddLine { product: ?addLineProduct }}>
                                       ^^^^^^^^^^^^^^^
app/Orders/Show.alder:87:44: warning[W0002]: hole ?addLineProduct has type Product.ProductId
    let r <- Test.submit (AddLine { product: ?addLineProduct }) { quantity: Int.Positive 1 }
                                             ^^^^^^^^^^^^^^^
app/Orders/Show.alder:88:29: warning[W0002]: hole ?expected has type Answer
    Task.succeed (r.answer == ?expected)
                              ^^^^^^^^^
app/Orders/Show.alder:103:16: warning[W0002]: hole ?expected has type Bool
    Task.succeed ?expected
                 ^^^^^^^^^
app/Orders/Show.alder:118:16: warning[W0002]: hole ?expected has type Bool
    Task.succeed ?expected
                 ^^^^^^^^^
[exit 0]
$ alder fill Orders.Show.update.Cancel - <<'EOF'
if model.order.status == Open then {
  let _ <- Db.update Order model.order.id (\o -> { o | status: Cancelled })
  Task.succeed (Redirect (Orders.Show { id: model.order.id }))
} else Task.succeed (Reject "This order is not open.")
EOF
edited app/Orders/Show.alder
--- a/app/Orders/Show.alder
+++ b/app/Orders/Show.alder
@@ -50,7 +50,10 @@
     let _ <- Db.update Order model.order.id (\o -> { o | status: Paid })
     Task.succeed (Redirect (Orders.Show { id: model.order.id }))
   } else Task.succeed (Reject "This order is not open."),
-  Cancel p -> ?cancel,
+  Cancel p -> if model.order.status == Open then {
+    let _ <- Db.update Order model.order.id (\o -> { o | status: Cancelled })
+    Task.succeed (Redirect (Orders.Show { id: model.order.id }))
+  } else Task.succeed (Reject "This order is not open."),
 }
 
 view : Model -> Session -> Html Msg
app/Orders/Show.alder:66:38: warning[W0002]: hole ?addLineProduct has type Product.ProductId
      <form submit={AddLine { product: ?addLineProduct }}>
                                       ^^^^^^^^^^^^^^^
app/Orders/Show.alder:90:44: warning[W0002]: hole ?addLineProduct has type Product.ProductId
    let r <- Test.submit (AddLine { product: ?addLineProduct }) { quantity: Int.Positive 1 }
                                             ^^^^^^^^^^^^^^^
app/Orders/Show.alder:91:29: warning[W0002]: hole ?expected has type Answer
    Task.succeed (r.answer == ?expected)
                              ^^^^^^^^^
app/Orders/Show.alder:106:16: warning[W0002]: hole ?expected has type Bool
    Task.succeed ?expected
                 ^^^^^^^^^
app/Orders/Show.alder:121:16: warning[W0002]: hole ?expected has type Bool
    Task.succeed ?expected
                 ^^^^^^^^^
[exit 0]
$ alder fill --decl Orders.Show.view - <<'EOF'
<Layout.Page title="Order">
  <h1>Order</h1>
  <p id="status">{Order.statusText model.order.status}</p>
  <p id="total">{Text.fromInt model.total}</p>
  <ul id="lines">
    {for l in model.lines ->
      <li>
        <span class="name">{l.name}</span>
        <span class="quantity">{Text.fromInt l.quantity}</span>
        <span class="subtotal">{Text.fromInt l.subtotal}</span>
      </li>}
  </ul>
  {if model.order.status == Open then
    <div>
      {for p in model.products ->
        <form submit={AddLine { product: p.id }}>
          <h3>{p.name}</h3>
          <Errors form />
          <al-input field="quantity" label="Quantity" />
          <al-button type="submit">Add line</al-button>
        </form>}
      <al-button click={Pay}>Pay</al-button>
      <al-button click={Cancel}>Cancel</al-button>
    </div>
  else Html.none}
  <a href={Orders.Index {}}>Back</a>
</Layout.Page>
EOF
edited app/Orders/Show.alder
--- a/app/Orders/Show.alder
+++ b/app/Orders/Show.alder
@@ -57,23 +57,34 @@
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
+    <p id="status">{Order.statusText model.order.status}</p>
+    <p id="total">{Text.fromInt model.total}</p>
+    <ul id="lines">
+      {for l in model.lines ->
+        <li>
+          <span class="name">{l.name}</span>
+          <span class="quantity">{Text.fromInt l.quantity}</span>
+          <span class="subtotal">{Text.fromInt l.subtotal}</span>
+        </li>}
+    </ul>
+    {if model.order.status == Open then
+      <div>
+        {for p in model.products ->
+          <form submit={AddLine { product: p.id }}>
+            <h3>{p.name}</h3>
+            <Errors form />
+            <al-input field="quantity" label="Quantity" />
+            <al-button type="submit">Add line</al-button>
+          </form>}
+        <al-button click={Pay}>Pay</al-button>
+        <al-button click={Cancel}>Cancel</al-button>
+      </div>
+    else Html.none}
+    <a href={Orders.Index {}}>Back</a>
+  </Layout.Page>
 
 test "a visitor is sent to log in" = {
   let me <- Db.insert User { email: Text.Email "email-1@example.com", name: Text.NonEmpty "name-1", passwordHash: "passwordhash-1" }
app/Orders/Show.alder:101:44: warning[W0002]: hole ?addLineProduct has type Product.ProductId
    let r <- Test.submit (AddLine { product: ?addLineProduct }) { quantity: Int.Positive 1 }
                                             ^^^^^^^^^^^^^^^
app/Orders/Show.alder:102:29: warning[W0002]: hole ?expected has type Answer
    Task.succeed (r.answer == ?expected)
                              ^^^^^^^^^
app/Orders/Show.alder:117:16: warning[W0002]: hole ?expected has type Bool
    Task.succeed ?expected
                 ^^^^^^^^^
app/Orders/Show.alder:132:16: warning[W0002]: hole ?expected has type Bool
    Task.succeed ?expected
                 ^^^^^^^^^
[exit 0]
$ alder fill --decl 'Orders.Show."submitting NewLine"' - <<'EOF'
{
  let me <- Db.insert User { email: Text.Email "line@example.com", name: Text.NonEmpty "Line", passwordHash: "" }
  let o <- Db.insert Order { owner: me }
  let p1 <- Db.insert Product { name: Text.NonEmpty "Pen", price: Int.NonNegative 250 }
  let _ <- Test.login { user: Just me }
  let _ <- Test.get (Orders.Show { id: o })
  let r <- Test.submit (AddLine { product: p1 }) { quantity: Int.Positive 2 }
  let n <- Db.from Line |> Db.count
  Task.succeed ({ answer: r.answer, n: n } == { answer: Redirected (Orders.Show { id: o }), n: 1 })
}
EOF
edited app/Orders/Show.alder
--- a/app/Orders/Show.alder
+++ b/app/Orders/Show.alder
@@ -94,12 +94,14 @@
 }
 
 test "submitting NewLine" = {
-  let me <- Db.insert User { email: Text.Email "email-1@example.com", name: Text.NonEmpty "name-1", passwordHash: "passwordhash-1" }
+  let me <- Db.insert User { email: Text.Email "line@example.com", name: Text.NonEmpty "Line", passwordHash: "" }
+  let o <- Db.insert Order { owner: me }
+  let p1 <- Db.insert Product { name: Text.NonEmpty "Pen", price: Int.NonNegative 250 }
   let _ <- Test.login { user: Just me }
-  let order1 <- Db.insert Order { owner: me }
-  let _ <- Test.get (Orders.Show { id: order1 })
-  let r <- Test.submit (AddLine { product: ?addLineProduct }) { quantity: Int.Positive 1 }
-  Task.succeed (r.answer == ?expected)
+  let _ <- Test.get (Orders.Show { id: o })
+  let r <- Test.submit (AddLine { product: p1 }) { quantity: Int.Positive 2 }
+  let n <- Db.from Line |> Db.count
+  Task.succeed ({ answer: r.answer, n: n } == { answer: Redirected (Orders.Show { id: o }), n: 1 })
 }
 
 -- Fill ?expected with what pressing Pay must do, so that a test fails when the arm
app/Orders/Show.alder:119:16: warning[W0002]: hole ?expected has type Bool
    Task.succeed ?expected
                 ^^^^^^^^^
app/Orders/Show.alder:134:16: warning[W0002]: hole ?expected has type Bool
    Task.succeed ?expected
                 ^^^^^^^^^
rewriting a test: `alder test --mutate` is the check that it still tests something
[exit 0]
$ alder fill --decl 'Orders.Show."pressing Pay"' - <<'EOF'
{
  let me <- Db.insert User { email: Text.Email "pay@example.com", name: Text.NonEmpty "Pay", passwordHash: "" }
  let o <- Db.insert Order { owner: me }
  let _ <- Test.login { user: Just me }
  let _ <- Test.get (Orders.Show { id: o })
  let r <- Test.click Pay
  Task.succeed ({ answer: r.answer, status: Test.text "#status" r.doc } == { answer: Redirected (Orders.Show { id: o }), status: Just "Paid" })
}
EOF
edited app/Orders/Show.alder
--- a/app/Orders/Show.alder
+++ b/app/Orders/Show.alder
@@ -104,19 +104,13 @@
   Task.succeed ({ answer: r.answer, n: n } == { answer: Redirected (Orders.Show { id: o }), n: 1 })
 }
 
--- Fill ?expected with what pressing Pay must do, so that a test fails when the arm
--- does something else: its answer (`r.answer`: Rerendered, Redirected …) and what it
--- changed (`after`: the page opened again, `after.answer` Shown, or PageMissing when the arm
--- deleted the page's row, and `after.doc` its HTML), as
--- `{ answer: r.answer, … } == { answer: Rerendered, … }`.
 test "pressing Pay" = {
-  let me <- Db.insert User { email: Text.Email "email-1@example.com", name: Text.NonEmpty "name-1", passwordHash: "passwordhash-1" }
+  let me <- Db.insert User { email: Text.Email "pay@example.com", name: Text.NonEmpty "Pay", passwordHash: "" }
+  let o <- Db.insert Order { owner: me }
   let _ <- Test.login { user: Just me }
-  let order1 <- Db.insert Order { owner: me }
-  let _ <- Test.get (Orders.Show { id: order1 })
+  let _ <- Test.get (Orders.Show { id: o })
   let r <- Test.click Pay
-  let after <- Test.open (Orders.Show { id: order1 })
-  Task.succeed ?expected
+  Task.succeed ({ answer: r.answer, status: Test.text "#status" r.doc } == { answer: Redirected (Orders.Show { id: o }), status: Just "Paid" })
 }
 
 -- Fill ?expected with what pressing Cancel must do, so that a test fails when the arm
app/Orders/Show.alder:128:16: warning[W0002]: hole ?expected has type Bool
    Task.succeed ?expected
                 ^^^^^^^^^
rewriting a test: `alder test --mutate` is the check that it still tests something
[exit 0]
$ alder fill --decl 'Orders.Show."pressing Cancel"' - <<'EOF'
{
  let me <- Db.insert User { email: Text.Email "cancel@example.com", name: Text.NonEmpty "Cancel", passwordHash: "" }
  let o <- Db.insert Order { owner: me }
  let _ <- Test.login { user: Just me }
  let _ <- Test.get (Orders.Show { id: o })
  let r <- Test.click Cancel
  Task.succeed ({ answer: r.answer, status: Test.text "#status" r.doc } == { answer: Redirected (Orders.Show { id: o }), status: Just "Cancelled" })
}
EOF
edited app/Orders/Show.alder
--- a/app/Orders/Show.alder
+++ b/app/Orders/Show.alder
@@ -113,17 +113,11 @@
   Task.succeed ({ answer: r.answer, status: Test.text "#status" r.doc } == { answer: Redirected (Orders.Show { id: o }), status: Just "Paid" })
 }
 
--- Fill ?expected with what pressing Cancel must do, so that a test fails when the arm
--- does something else: its answer (`r.answer`: Rerendered, Redirected …) and what it
--- changed (`after`: the page opened again, `after.answer` Shown, or PageMissing when the arm
--- deleted the page's row, and `after.doc` its HTML), as
--- `{ answer: r.answer, … } == { answer: Rerendered, … }`.
 test "pressing Cancel" = {
-  let me <- Db.insert User { email: Text.Email "email-1@example.com", name: Text.NonEmpty "name-1", passwordHash: "passwordhash-1" }
+  let me <- Db.insert User { email: Text.Email "cancel@example.com", name: Text.NonEmpty "Cancel", passwordHash: "" }
+  let o <- Db.insert Order { owner: me }
   let _ <- Test.login { user: Just me }
-  let order1 <- Db.insert Order { owner: me }
-  let _ <- Test.get (Orders.Show { id: order1 })
+  let _ <- Test.get (Orders.Show { id: o })
   let r <- Test.click Cancel
-  let after <- Test.open (Orders.Show { id: order1 })
-  Task.succeed ?expected
+  Task.succeed ({ answer: r.answer, status: Test.text "#status" r.doc } == { answer: Redirected (Orders.Show { id: o }), status: Just "Cancelled" })
 }
rewriting a test: `alder test --mutate` is the check that it still tests something
[exit 0]
$ alder new test Orders.Show "an open order's total over two lines"
edited app/Orders/Show.alder
--- a/app/Orders/Show.alder
+++ b/app/Orders/Show.alder
@@ -121,3 +121,7 @@
   let r <- Test.click Cancel
   Task.succeed ({ answer: r.answer, status: Test.text "#status" r.doc } == { answer: Redirected (Orders.Show { id: o }), status: Just "Cancelled" })
 }
+
+test "an open order's total over two lines" = {
+  ?test
+}
app/Orders/Show.alder:126:3: warning[W0002]: hole ?test has type Task Bool
    ?test
    ^^^^^
[exit 0]
$ alder fill 'Orders.Show."an open order'"'"'s total over two lines"' - <<'EOF'
{
  let me <- Db.insert User { email: Text.Email "a@example.com", name: Text.NonEmpty "A", passwordHash: "" }
  let o <- Db.insert Order { owner: me }
  let p1 <- Db.insert Product { name: Text.NonEmpty "Pen", price: Int.NonNegative 250 }
  let p2 <- Db.insert Product { name: Text.NonEmpty "Ink", price: Int.NonNegative 100 }
  let _ <- Db.insert Line { order: o, product: p1, quantity: Int.Positive 2 }
  let _ <- Db.insert Line { order: o, product: p2, quantity: Int.Positive 3 }
  let _ <- Test.login { user: Just me }
  let page <- Test.get (Orders.Show { id: o })
  Task.succeed ({ total: Test.text "#total" page.doc, status: Test.text "#status" page.doc } == { total: Just "800", status: Just "Open" })
}
EOF
edited app/Orders/Show.alder
--- a/app/Orders/Show.alder
+++ b/app/Orders/Show.alder
@@ -123,5 +123,13 @@
 }
 
 test "an open order's total over two lines" = {
-  ?test
+  let me <- Db.insert User { email: Text.Email "a@example.com", name: Text.NonEmpty "A", passwordHash: "" }
+  let o <- Db.insert Order { owner: me }
+  let p1 <- Db.insert Product { name: Text.NonEmpty "Pen", price: Int.NonNegative 250 }
+  let p2 <- Db.insert Product { name: Text.NonEmpty "Ink", price: Int.NonNegative 100 }
+  let _ <- Db.insert Line { order: o, product: p1, quantity: Int.Positive 2 }
+  let _ <- Db.insert Line { order: o, product: p2, quantity: Int.Positive 3 }
+  let _ <- Test.login { user: Just me }
+  let page <- Test.get (Orders.Show { id: o })
+  Task.succeed ({ total: Test.text "#total" page.doc, status: Test.text "#status" page.doc } == { total: Just "800", status: Just "Open" })
 }
[exit 0]
$ alder new test Orders.Show "adding a line to a paid order is refused"
edited app/Orders/Show.alder
--- a/app/Orders/Show.alder
+++ b/app/Orders/Show.alder
@@ -133,3 +133,7 @@
   let page <- Test.get (Orders.Show { id: o })
   Task.succeed ({ total: Test.text "#total" page.doc, status: Test.text "#status" page.doc } == { total: Just "800", status: Just "Open" })
 }
+
+test "adding a line to a paid order is refused" = {
+  ?test
+}
app/Orders/Show.alder:138:3: warning[W0002]: hole ?test has type Task Bool
    ?test
    ^^^^^
[exit 0]
$ alder fill 'Orders.Show."adding a line to a paid order is refused"' - <<'EOF'
{
  let me <- Db.insert User { email: Text.Email "b@example.com", name: Text.NonEmpty "B", passwordHash: "" }
  let o <- Db.insert Order { owner: me }
  let p1 <- Db.insert Product { name: Text.NonEmpty "Pen", price: Int.NonNegative 250 }
  let _ <- Test.login { user: Just me }
  let _ <- Test.get (Orders.Show { id: o })
  let _ <- Db.update Order o (\x -> { x | status: Paid })
  let _ <- Test.submit (AddLine { product: p1 }) { quantity: Int.Positive 1 }
  let n <- Db.from Line |> Db.count
  Task.succeed (n == 0)
}
EOF
edited app/Orders/Show.alder
--- a/app/Orders/Show.alder
+++ b/app/Orders/Show.alder
@@ -135,5 +135,13 @@
 }
 
 test "adding a line to a paid order is refused" = {
-  ?test
+  let me <- Db.insert User { email: Text.Email "b@example.com", name: Text.NonEmpty "B", passwordHash: "" }
+  let o <- Db.insert Order { owner: me }
+  let p1 <- Db.insert Product { name: Text.NonEmpty "Pen", price: Int.NonNegative 250 }
+  let _ <- Test.login { user: Just me }
+  let _ <- Test.get (Orders.Show { id: o })
+  let _ <- Db.update Order o (\x -> { x | status: Paid })
+  let _ <- Test.submit (AddLine { product: p1 }) { quantity: Int.Positive 1 }
+  let n <- Db.from Line |> Db.count
+  Task.succeed (n == 0)
 }
[exit 0]
$ alder new test Orders.Show "a Staff user opening another user's order is not found"
edited app/Orders/Show.alder
--- a/app/Orders/Show.alder
+++ b/app/Orders/Show.alder
@@ -145,3 +145,7 @@
   let n <- Db.from Line |> Db.count
   Task.succeed (n == 0)
 }
+
+test "a Staff user opening another user's order is not found" = {
+  ?test
+}
app/Orders/Show.alder:150:3: warning[W0002]: hole ?test has type Task Bool
    ?test
    ^^^^^
[exit 0]
$ alder fill 'Orders.Show."a Staff user opening another user'"'"'s order is not found"' - <<'EOF'
{
  let a <- Db.insert User { email: Text.Email "c@example.com", name: Text.NonEmpty "C", passwordHash: "" }
  let b <- Db.insert User { email: Text.Email "d@example.com", name: Text.NonEmpty "D", passwordHash: "" }
  let o <- Db.insert Order { owner: a }
  let _ <- Test.login { user: Just b }
  let r <- Test.open (Orders.Show { id: o })
  Task.succeed (r.answer == PageMissing)
}
EOF
edited app/Orders/Show.alder
--- a/app/Orders/Show.alder
+++ b/app/Orders/Show.alder
@@ -147,5 +147,10 @@
 }
 
 test "a Staff user opening another user's order is not found" = {
-  ?test
+  let a <- Db.insert User { email: Text.Email "c@example.com", name: Text.NonEmpty "C", passwordHash: "" }
+  let b <- Db.insert User { email: Text.Email "d@example.com", name: Text.NonEmpty "D", passwordHash: "" }
+  let o <- Db.insert Order { owner: a }
+  let _ <- Test.login { user: Just b }
+  let r <- Test.open (Orders.Show { id: o })
+  Task.succeed (r.answer == PageMissing)
 }
[exit 0]
$ alder new test Orders.Show "a Manager opening another user's order sees it"
edited app/Orders/Show.alder
--- a/app/Orders/Show.alder
+++ b/app/Orders/Show.alder
@@ -154,3 +154,7 @@
   let r <- Test.open (Orders.Show { id: o })
   Task.succeed (r.answer == PageMissing)
 }
+
+test "a Manager opening another user's order sees it" = {
+  ?test
+}
app/Orders/Show.alder:159:3: warning[W0002]: hole ?test has type Task Bool
    ?test
    ^^^^^
[exit 0]
$ alder fill 'Orders.Show."a Manager opening another user'"'"'s order sees it"' - <<'EOF'
{
  let a <- Db.insert User { email: Text.Email "e@example.com", name: Text.NonEmpty "E", passwordHash: "" }
  let b <- Db.insert User { email: Text.Email "f@example.com", name: Text.NonEmpty "F", passwordHash: "", role: Manager }
  let o <- Db.insert Order { owner: a }
  let _ <- Test.login { user: Just b }
  let r <- Test.open (Orders.Show { id: o })
  Task.succeed (r.answer == Shown)
}
EOF
edited app/Orders/Show.alder
--- a/app/Orders/Show.alder
+++ b/app/Orders/Show.alder
@@ -156,5 +156,10 @@
 }
 
 test "a Manager opening another user's order sees it" = {
-  ?test
+  let a <- Db.insert User { email: Text.Email "e@example.com", name: Text.NonEmpty "E", passwordHash: "" }
+  let b <- Db.insert User { email: Text.Email "f@example.com", name: Text.NonEmpty "F", passwordHash: "", role: Manager }
+  let o <- Db.insert Order { owner: a }
+  let _ <- Test.login { user: Just b }
+  let r <- Test.open (Orders.Show { id: o })
+  Task.succeed (r.answer == Shown)
 }
[exit 0]
$ alder new page Products.Index /products --blank --requires user
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
$ alder fill Products.Index.load - <<'EOF'
{
  let mgr <- User.isManager session.user
  if mgr then {
    let prods <- Db.from Product |> Db.all
    Query.succeed { products: prods }
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
+    let prods <- Db.from Product |> Db.all
+    Query.succeed { products: prods }
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
$ alder fill Products.Index.update.Add - <<'EOF'
{
  let _ <- Db.insert Product { name: p.form.name, price: p.form.price }
  Task.succeed (Redirect (Products.Index {}))
}
EOF
alder fill: `Products.Index.update.Add` has no typed hole: a fill replaces a hole, and this arm has none; to rewrite it, give `alder fill --decl Products.Index.update -` the whole `match msg { … }` (`alder fill --list` lists the app's holes)
[exit 1]
```

The harness stopped at line 168 of your script: `alder fill Products.Index.update.Add - <<'EOF'`: it exited 1. The commands after it were not run.

Your script stopped at command 24; the app is as the commands before it left it; continue.

The program type-checks but `alder build` refused it; fix the cause below.

```text
app/Products/Index.alder:35:15: warning[W0002]: hole ?toggle has type Task Nav
    Toggle p -> ?toggle,
                ^^^^^^^
app/Products/Index.alder:53:36: warning[W0002]: hole ?toggleId has type Product.ProductId
      <al-button click={Toggle { id: ?toggleId }}>Toggle</al-button>
                                     ^^^^^^^^^
app/Products/Index.alder:89:16: warning[W0002]: hole ?expected has type Bool
    Task.succeed ?expected
                 ^^^^^^^^^
alder build: the program has 3 typed hole(s); fill them before building
```
