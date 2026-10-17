### Round 2

```text
$ alder fill --decl 'Orders.Index."pressing Create"' - <<'EOF'
{
  let me <- Db.insert User { email: Text.Email "email-1@example.com", name: Text.NonEmpty "name-1", passwordHash: "passwordhash-1" }
  let _ <- Test.login { user: Just me }
  let _ <- Test.get (Orders.Index {})
  let r <- Test.click Create
  let made <- Db.from Order |> Db.first
  let want = match made {
    Just o -> Just (Redirected (Orders.Show { id: o.id })),
    Nothing -> Nothing,
  }
  let after <- Test.get (Orders.Index {})
  Task.succeed ({ answer: Just r.answer, status: Test.text "#status" r.doc, listed: Test.texts "#orders .status" after.doc } == { answer: want, status: Just "Open", listed: ["Open"] })
}
EOF
edited app/Orders/Index.alder
--- a/app/Orders/Index.alder
+++ b/app/Orders/Index.alder
@@ -49,16 +49,16 @@
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
-  let after <- Test.open (Orders.Index {})
-  Task.succeed ?expected
+  let made <- Db.from Order |> Db.first
+  let want = match made {
+    Just o -> Just (Redirected (Orders.Show { id: o.id })),
+    Nothing -> Nothing,
+  }
+  let after <- Test.get (Orders.Index {})
+  Task.succeed ({ answer: Just r.answer, status: Test.text "#status" r.doc, listed: Test.texts "#orders .status" after.doc } == { answer: want, status: Just "Open", listed: ["Open"] })
 }
rewriting a test: `alder test --mutate` is the check that it still tests something
[exit 0]
$ alder fill --decl 'Orders.Show."submitting NewLine"' - <<'EOF'
{
  let me <- Db.insert User { email: Text.Email "email-1@example.com", name: Text.NonEmpty "name-1", passwordHash: "passwordhash-1" }
  let tea <- Db.insert Product { name: Text.NonEmpty "Tea", price: Int.NonNegative 250 }
  let _ <- Test.login { user: Just me }
  let o <- Db.insert Order { owner: me }
  let _ <- Test.get (Orders.Show { id: o })
  let r <- Test.submit (AddLine { product: tea }) { quantity: Int.Positive 2 }
  let lines <- Db.from Line |> Db.where (Line.order.eq o) |> Db.all
  Task.succeed ({ answer: r.answer, q: List.map (\l -> l.quantity) lines, total: Test.text "#total" r.doc } == { answer: Redirected (Orders.Show { id: o }), q: [Int.Positive 2], total: Just "500" })
}
EOF
edited app/Orders/Show.alder
--- a/app/Orders/Show.alder
+++ b/app/Orders/Show.alder
@@ -94,11 +94,13 @@
 
 test "submitting NewLine" = {
   let me <- Db.insert User { email: Text.Email "email-1@example.com", name: Text.NonEmpty "name-1", passwordHash: "passwordhash-1" }
+  let tea <- Db.insert Product { name: Text.NonEmpty "Tea", price: Int.NonNegative 250 }
   let _ <- Test.login { user: Just me }
-  let order1 <- Db.insert Order { owner: me }
-  let _ <- Test.get (Orders.Show { id: order1 })
-  let r <- Test.submit (AddLine { product: ?addLineProduct }) { quantity: Int.Positive 1 }
-  Task.succeed (r.answer == ?expected)
+  let o <- Db.insert Order { owner: me }
+  let _ <- Test.get (Orders.Show { id: o })
+  let r <- Test.submit (AddLine { product: tea }) { quantity: Int.Positive 2 }
+  let lines <- Db.from Line |> Db.where (Line.order.eq o) |> Db.all
+  Task.succeed ({ answer: r.answer, q: List.map (\l -> l.quantity) lines, total: Test.text "#total" r.doc } == { answer: Redirected (Orders.Show { id: o }), q: [Int.Positive 2], total: Just "500" })
 }
 
 -- Fill ?expected with what pressing Pay must do, so that a test fails when the arm
app/Orders/Show.alder:118:16: warning[W0002]: hole ?expected has type Bool
    Task.succeed ?expected
                 ^^^^^^^^^
app/Orders/Show.alder:133:16: warning[W0002]: hole ?expected has type Bool
    Task.succeed ?expected
                 ^^^^^^^^^
rewriting a test: `alder test --mutate` is the check that it still tests something
[exit 0]
$ alder fill 'Orders.Show."pressing Pay"' - <<'EOF'
{ answer: r.answer, status: Test.text "#status" after.doc, actions: Test.has "#actions" after.doc } == { answer: Redirected (Orders.Show { id: order1 }), status: Just "Paid", actions: False }
EOF
edited app/Orders/Show.alder
--- a/app/Orders/Show.alder
+++ b/app/Orders/Show.alder
@@ -103,11 +103,6 @@
   Task.succeed ({ answer: r.answer, q: List.map (\l -> l.quantity) lines, total: Test.text "#total" r.doc } == { answer: Redirected (Orders.Show { id: o }), q: [Int.Positive 2], total: Just "500" })
 }
 
--- Fill ?expected with what pressing Pay must do, so that a test fails when the arm
--- does something else: its answer (`r.answer`: Rerendered, Redirected …) and what it
--- changed (`after`: the page opened again, `after.answer` Shown, or PageMissing when the arm
--- deleted the page's row, and `after.doc` its HTML), as
--- `{ answer: r.answer, … } == { answer: Rerendered, … }`.
 test "pressing Pay" = {
   let me <- Db.insert User { email: Text.Email "email-1@example.com", name: Text.NonEmpty "name-1", passwordHash: "passwordhash-1" }
   let _ <- Test.login { user: Just me }
@@ -115,7 +110,7 @@
   let _ <- Test.get (Orders.Show { id: order1 })
   let r <- Test.click Pay
   let after <- Test.open (Orders.Show { id: order1 })
-  Task.succeed ?expected
+  Task.succeed ({ answer: r.answer, status: Test.text "#status" after.doc, actions: Test.has "#actions" after.doc } == { answer: Redirected (Orders.Show { id: order1 }), status: Just "Paid", actions: False })
 }
 
 -- Fill ?expected with what pressing Cancel must do, so that a test fails when the arm
app/Orders/Show.alder:128:16: warning[W0002]: hole ?expected has type Bool
    Task.succeed ?expected
                 ^^^^^^^^^
[exit 0]
$ alder fill 'Orders.Show."pressing Cancel"' - <<'EOF'
{ answer: r.answer, status: Test.text "#status" after.doc, actions: Test.has "#actions" after.doc } == { answer: Redirected (Orders.Show { id: order1 }), status: Just "Cancelled", actions: False }
EOF
edited app/Orders/Show.alder
--- a/app/Orders/Show.alder
+++ b/app/Orders/Show.alder
@@ -113,11 +113,6 @@
   Task.succeed ({ answer: r.answer, status: Test.text "#status" after.doc, actions: Test.has "#actions" after.doc } == { answer: Redirected (Orders.Show { id: order1 }), status: Just "Paid", actions: False })
 }
 
--- Fill ?expected with what pressing Cancel must do, so that a test fails when the arm
--- does something else: its answer (`r.answer`: Rerendered, Redirected …) and what it
--- changed (`after`: the page opened again, `after.answer` Shown, or PageMissing when the arm
--- deleted the page's row, and `after.doc` its HTML), as
--- `{ answer: r.answer, … } == { answer: Rerendered, … }`.
 test "pressing Cancel" = {
   let me <- Db.insert User { email: Text.Email "email-1@example.com", name: Text.NonEmpty "name-1", passwordHash: "passwordhash-1" }
   let _ <- Test.login { user: Just me }
@@ -125,7 +120,7 @@
   let _ <- Test.get (Orders.Show { id: order1 })
   let r <- Test.click Cancel
   let after <- Test.open (Orders.Show { id: order1 })
-  Task.succeed ?expected
+  Task.succeed ({ answer: r.answer, status: Test.text "#status" after.doc, actions: Test.has "#actions" after.doc } == { answer: Redirected (Orders.Show { id: order1 }), status: Just "Cancelled", actions: False })
 }
 
 test "an Open order's total is the sum of its lines" = {
[exit 0]
$ alder fill --decl 'Products.Index."submitting NewProduct adds a row"' - <<'EOF'
{
  let me <- Db.insert User { email: Text.Email "email-1@example.com", name: Text.NonEmpty "name-1", passwordHash: "passwordhash-1", role: Manager }
  let _ <- Test.login { user: Just me }
  let _ <- Test.get (Products.Index {})
  let r <- Test.submit NewProduct { name: Text.NonEmpty "name-1", price: Int.NonNegative 1 }
  let rows <- Db.from Product |> Db.all
  Task.succeed ({ answer: r.answer, rows: List.map (\row -> { name: row.name, price: row.price, active: row.active }) rows, names: Test.texts "#products .name" r.doc } == { answer: Redirected (Products.Index {}), rows: [{ name: Text.NonEmpty "name-1", price: Int.NonNegative 1, active: True }], names: ["name-1"] })
}
EOF
edited app/Products/Index.alder
--- a/app/Products/Index.alder
+++ b/app/Products/Index.alder
@@ -65,12 +65,12 @@
 }
 
 test "submitting NewProduct adds a row" = {
-  let me <- Db.insert User { email: Text.Email "email-1@example.com", name: Text.NonEmpty "name-1", passwordHash: "passwordhash-1" }
+  let me <- Db.insert User { email: Text.Email "email-1@example.com", name: Text.NonEmpty "name-1", passwordHash: "passwordhash-1", role: Manager }
   let _ <- Test.login { user: Just me }
   let _ <- Test.get (Products.Index {})
   let r <- Test.submit NewProduct { name: Text.NonEmpty "name-1", price: Int.NonNegative 1 }
   let rows <- Db.from Product |> Db.all
-  Task.succeed ({ answer: r.answer, rows: List.map (\row -> { name: row.name, price: row.price, active: row.active }) rows } == { answer: Redirected (Products.Index {}), rows: [{ name: Text.NonEmpty "name-1", price: Int.NonNegative 1, active: True }] })
+  Task.succeed ({ answer: r.answer, rows: List.map (\row -> { name: row.name, price: row.price, active: row.active }) rows, names: Test.texts "#products .name" r.doc } == { answer: Redirected (Products.Index {}), rows: [{ name: Text.NonEmpty "name-1", price: Int.NonNegative 1, active: True }], names: ["name-1"] })
 }
 
 test "NewProduct refuses an empty name" = {
app/Products/Index.alder:95:16: warning[W0002]: hole ?expected has type Bool
    Task.succeed ?expected
                 ^^^^^^^^^
rewriting a test: `alder test --mutate` is the check that it still tests something
[exit 0]
$ alder fill --decl 'Products.Index."NewProduct refuses an empty name"' - <<'EOF'
{
  let me <- Db.insert User { email: Text.Email "email-1@example.com", name: Text.NonEmpty "name-1", passwordHash: "passwordhash-1", role: Manager }
  let _ <- Test.login { user: Just me }
  let _ <- Test.get (Products.Index {})
  let r <- Test.submitRaw NewProduct [{ name: "name", value: "" }, { name: "price", value: "1" }]
  let rows <- Db.from Product |> Db.count
  Task.succeed ({ answer: r.answer, rows: rows } == { answer: FormRejected [{ field: "", message: "name must not be empty" }], rows: 0 })
}
EOF
edited app/Products/Index.alder
--- a/app/Products/Index.alder
+++ b/app/Products/Index.alder
@@ -74,12 +74,12 @@
 }
 
 test "NewProduct refuses an empty name" = {
-  let me <- Db.insert User { email: Text.Email "email-1@example.com", name: Text.NonEmpty "name-1", passwordHash: "passwordhash-1" }
+  let me <- Db.insert User { email: Text.Email "email-1@example.com", name: Text.NonEmpty "name-1", passwordHash: "passwordhash-1", role: Manager }
   let _ <- Test.login { user: Just me }
   let _ <- Test.get (Products.Index {})
   let r <- Test.submitRaw NewProduct [{ name: "name", value: "" }, { name: "price", value: "1" }]
   let rows <- Db.from Product |> Db.count
-  Task.succeed ({ answer: r.answer, rows: rows } == { answer: FormRejected [{ field: "name", message: "must not be empty" }], rows: 0 })
+  Task.succeed ({ answer: r.answer, rows: rows } == { answer: FormRejected [{ field: "", message: "name must not be empty" }], rows: 0 })
 }
 
 -- Fill ?expected with what pressing Toggle must do, so that a test fails when the arm
app/Products/Index.alder:95:16: warning[W0002]: hole ?expected has type Bool
    Task.succeed ?expected
                 ^^^^^^^^^
rewriting a test: `alder test --mutate` is the check that it still tests something
[exit 0]
$ alder fill --decl 'Products.Index."pressing Toggle"' - <<'EOF'
{
  let me <- Db.insert User { email: Text.Email "email-1@example.com", name: Text.NonEmpty "name-1", passwordHash: "passwordhash-1", role: Manager }
  let _ <- Test.login { user: Just me }
  let product1 <- Db.insert Product { name: Text.NonEmpty "name-1", price: Int.NonNegative 1 }
  let _ <- Test.get (Products.Index {})
  let r <- Test.click (Toggle { id: product1 })
  let after <- Db.get Product product1
  Task.succeed ({ answer: r.answer, active: Maybe.map (\x -> x.active) after } == { answer: Redirected (Products.Index {}), active: Just False })
}
EOF
edited app/Products/Index.alder
--- a/app/Products/Index.alder
+++ b/app/Products/Index.alder
@@ -82,17 +82,14 @@
   Task.succeed ({ answer: r.answer, rows: rows } == { answer: FormRejected [{ field: "", message: "name must not be empty" }], rows: 0 })
 }
 
--- Fill ?expected with what pressing Toggle must do, so that a test fails when the arm
--- does something else: its answer (`r.answer`: Rerendered, Redirected …) and what it
--- changed (`after`, the row read again), as `{ answer: r.answer, … } == { answer: Rerendered, … }`.
 test "pressing Toggle" = {
-  let me <- Db.insert User { email: Text.Email "email-1@example.com", name: Text.NonEmpty "name-1", passwordHash: "passwordhash-1" }
+  let me <- Db.insert User { email: Text.Email "email-1@example.com", name: Text.NonEmpty "name-1", passwordHash: "passwordhash-1", role: Manager }
   let _ <- Test.login { user: Just me }
   let product1 <- Db.insert Product { name: Text.NonEmpty "name-1", price: Int.NonNegative 1 }
   let _ <- Test.get (Products.Index {})
   let r <- Test.click (Toggle { id: product1 })
   let after <- Db.get Product product1
-  Task.succeed ?expected
+  Task.succeed ({ answer: r.answer, active: Maybe.map (\x -> x.active) after } == { answer: Redirected (Products.Index {}), active: Just False })
 }
 
 test "a Staff user gets 404 on products" = {
rewriting a test: `alder test --mutate` is the check that it still tests something
[exit 0]
$ alder check
[exit 0]
$ alder test
ok   App "Logout clears the user and goes to log in"
ok   Auth.Login "an unknown email is rejected"
ok   Auth.Login "logging in through the form"
ok   Auth.Login "a signed-in user logs out here"
ok   Order "a Order keeps its owner from being deleted"
ok   Line "deleting its order deletes a Line"
ok   Line "a Line keeps its product from being deleted"
ok   Orders.Index "a visitor is sent to log in"
ok   Orders.Index "pressing Create"
ok   Orders.Show "a visitor is sent to log in"
ok   Orders.Show "submitting NewLine"
ok   Orders.Show "pressing Pay"
ok   Orders.Show "pressing Cancel"
ok   Orders.Show "an Open order's total is the sum of its lines"
ok   Orders.Show "adding a line to a Paid order is refused"
ok   Orders.Show "a Staff user opening another user's order is not found"
ok   Orders.Show "a Manager opening another user's order sees it"
ok   Products.Index "a visitor is sent to log in"
ok   Products.Index "submitting NewProduct adds a row"
ok   Products.Index "NewProduct refuses an empty name"
ok   Products.Index "pressing Toggle"
ok   Products.Index "a Staff user gets 404 on products"
22 passed, 0 failed
[exit 0]
```

The harness ran `alder check`, `alder build` and `alder test` on the app: all pass.
