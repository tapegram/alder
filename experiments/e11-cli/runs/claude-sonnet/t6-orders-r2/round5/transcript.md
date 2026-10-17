### Round 5

```text
$ alder fill Products.Index.update.Toggle - <<'EOF'
{
  let _ <- Db.update Product p.id (\x -> { x | active: not x.active })
  Task.succeed (Redirect (Products.Index {}))
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
  <al-card>
    <h2 slot="header">New product</h2>
    <form submit={Add}>
      <al-input field="name" label="Name" />
      <al-input field="price" label="Price" />
      <al-alert variant="destructive"><Errors form /></al-alert>
      <al-button type="submit">New product</al-button>
    </form>
  </al-card>
  <ul id="products">
    {for p in model.products ->
      <li>
        <span class="name">{p.name}</span>
        <span class="price">{Text.fromInt p.price}</span>
        <span class="active">{if p.active then "Active" else "Inactive"}</span>
        <al-button variant="secondary" size="sm" click={Toggle { id: p.id }}>Toggle</al-button>
      </li>}
  </ul>
  <a href={Orders.Index {}}>Orders</a>
</Layout.Page>
EOF
edited app/Products/Index.alder
--- a/app/Products/Index.alder
+++ b/app/Products/Index.alder
@@ -42,7 +42,6 @@
 view model session =
   <Layout.Page title="Products">
     <h1>Products</h1>
-
     <al-card>
       <h2 slot="header">New product</h2>
       <form submit={Add}>
@@ -52,8 +51,16 @@
         <al-button type="submit">New product</al-button>
       </form>
     </al-card>
-
-    <al-button click={Toggle { id: ?toggleId }}>Toggle</al-button>
+    <ul id="products">
+      {for p in model.products ->
+        <li>
+          <span class="name">{p.name}</span>
+          <span class="price">{Text.fromInt p.price}</span>
+          <span class="active">{if p.active then "Active" else "Inactive"}</span>
+          <al-button variant="secondary" size="sm" click={Toggle { id: p.id }}>Toggle</al-button>
+        </li>}
+    </ul>
+    <a href={Orders.Index {}}>Orders</a>
   </Layout.Page>
 
 test "a visitor is sent to log in" = {
app/Products/Index.alder:99:16: warning[W0002]: hole ?expected has type Bool
    Task.succeed ?expected
                 ^^^^^^^^^
[exit 0]
$ alder fill --decl 'Products.Index."submitting NewProduct adds a row"' - <<'EOF'
{
  let me <- Db.insert User { email: Text.Email "m1@example.com", name: Text.NonEmpty "M", passwordHash: "", role: Manager }
  let _ <- Test.login { user: Just me }
  let _ <- Test.get (Products.Index {})
  let r <- Test.submit NewProduct { name: Text.NonEmpty "Pen", price: Int.NonNegative 250 }
  let rows <- Db.from Product |> Db.all
  Task.succeed ({ answer: r.answer, rows: List.map (\row -> { name: row.name, price: row.price, active: row.active }) rows } == { answer: Redirected (Products.Index {}), rows: [{ name: Text.NonEmpty "Pen", price: Int.NonNegative 250, active: True }] })
}
EOF
edited app/Products/Index.alder
--- a/app/Products/Index.alder
+++ b/app/Products/Index.alder
@@ -69,12 +69,12 @@
 }
 
 test "submitting NewProduct adds a row" = {
-  let me <- Db.insert User { email: Text.Email "email-1@example.com", name: Text.NonEmpty "name-1", passwordHash: "passwordhash-1" }
+  let me <- Db.insert User { email: Text.Email "m1@example.com", name: Text.NonEmpty "M", passwordHash: "", role: Manager }
   let _ <- Test.login { user: Just me }
   let _ <- Test.get (Products.Index {})
-  let r <- Test.submit NewProduct { name: Text.NonEmpty "name-1", price: Int.NonNegative 1 }
+  let r <- Test.submit NewProduct { name: Text.NonEmpty "Pen", price: Int.NonNegative 250 }
   let rows <- Db.from Product |> Db.all
-  Task.succeed ({ answer: r.answer, rows: List.map (\row -> { name: row.name, price: row.price, active: row.active }) rows } == { answer: Redirected (Products.Index {}), rows: [{ name: Text.NonEmpty "name-1", price: Int.NonNegative 1, active: True }] })
+  Task.succeed ({ answer: r.answer, rows: List.map (\row -> { name: row.name, price: row.price, active: row.active }) rows } == { answer: Redirected (Products.Index {}), rows: [{ name: Text.NonEmpty "Pen", price: Int.NonNegative 250, active: True }] })
 }
 
 test "NewProduct refuses an empty name" = {
app/Products/Index.alder:99:16: warning[W0002]: hole ?expected has type Bool
    Task.succeed ?expected
                 ^^^^^^^^^
rewriting a test: `alder test --mutate` is the check that it still tests something
[exit 0]
$ alder fill --decl 'Products.Index."NewProduct refuses an empty name"' - <<'EOF'
{
  let me <- Db.insert User { email: Text.Email "m2@example.com", name: Text.NonEmpty "M", passwordHash: "", role: Manager }
  let _ <- Test.login { user: Just me }
  let _ <- Test.get (Products.Index {})
  let r <- Test.submitRaw NewProduct [{ name: "name", value: "" }, { name: "price", value: "1" }]
  let rows <- Db.from Product |> Db.count
  Task.succeed ({ answer: r.answer, rows: rows } == { answer: FormRejected [{ field: "name", message: "must not be empty" }], rows: 0 })
}
EOF
edited app/Products/Index.alder
--- a/app/Products/Index.alder
+++ b/app/Products/Index.alder
@@ -78,7 +78,7 @@
 }
 
 test "NewProduct refuses an empty name" = {
-  let me <- Db.insert User { email: Text.Email "email-1@example.com", name: Text.NonEmpty "name-1", passwordHash: "passwordhash-1" }
+  let me <- Db.insert User { email: Text.Email "m2@example.com", name: Text.NonEmpty "M", passwordHash: "", role: Manager }
   let _ <- Test.login { user: Just me }
   let _ <- Test.get (Products.Index {})
   let r <- Test.submitRaw NewProduct [{ name: "name", value: "" }, { name: "price", value: "1" }]
app/Products/Index.alder:99:16: warning[W0002]: hole ?expected has type Bool
    Task.succeed ?expected
                 ^^^^^^^^^
rewriting a test: `alder test --mutate` is the check that it still tests something
[exit 0]
$ alder fill --decl 'Products.Index."pressing Toggle"' - <<'EOF'
{
  let me <- Db.insert User { email: Text.Email "m3@example.com", name: Text.NonEmpty "M", passwordHash: "", role: Manager }
  let p <- Db.insert Product { name: Text.NonEmpty "Pen", price: Int.NonNegative 250 }
  let _ <- Test.login { user: Just me }
  let _ <- Test.get (Products.Index {})
  let r <- Test.click (Toggle { id: p })
  let after <- Db.get Product p
  Task.succeed ({ answer: r.answer, active: Maybe.map (\x -> x.active) after } == { answer: Redirected (Products.Index {}), active: Just False })
}
EOF
edited app/Products/Index.alder
--- a/app/Products/Index.alder
+++ b/app/Products/Index.alder
@@ -86,15 +86,12 @@
   Task.succeed ({ answer: r.answer, rows: rows } == { answer: FormRejected [{ field: "name", message: "must not be empty" }], rows: 0 })
 }
 
--- Fill ?expected with what pressing Toggle must do, so that a test fails when the arm
--- does something else: its answer (`r.answer`: Rerendered, Redirected …) and what it
--- changed (`after`, the row read again), as `{ answer: r.answer, … } == { answer: Rerendered, … }`.
 test "pressing Toggle" = {
-  let me <- Db.insert User { email: Text.Email "email-1@example.com", name: Text.NonEmpty "name-1", passwordHash: "passwordhash-1" }
+  let me <- Db.insert User { email: Text.Email "m3@example.com", name: Text.NonEmpty "M", passwordHash: "", role: Manager }
+  let p <- Db.insert Product { name: Text.NonEmpty "Pen", price: Int.NonNegative 250 }
   let _ <- Test.login { user: Just me }
-  let product1 <- Db.insert Product { name: Text.NonEmpty "name-1", price: Int.NonNegative 1 }
   let _ <- Test.get (Products.Index {})
-  let r <- Test.click (Toggle { id: product1 })
-  let after <- Db.get Product product1
-  Task.succeed ?expected
+  let r <- Test.click (Toggle { id: p })
+  let after <- Db.get Product p
+  Task.succeed ({ answer: r.answer, active: Maybe.map (\x -> x.active) after } == { answer: Redirected (Products.Index {}), active: Just False })
 }
rewriting a test: `alder test --mutate` is the check that it still tests something
[exit 0]
$ alder new test Products.Index "a Staff user gets 404 on products"
edited app/Products/Index.alder
--- a/app/Products/Index.alder
+++ b/app/Products/Index.alder
@@ -95,3 +95,7 @@
   let after <- Db.get Product p
   Task.succeed ({ answer: r.answer, active: Maybe.map (\x -> x.active) after } == { answer: Redirected (Products.Index {}), active: Just False })
 }
+
+test "a Staff user gets 404 on products" = {
+  ?test
+}
app/Products/Index.alder:100:3: warning[W0002]: hole ?test has type Task Bool
    ?test
    ^^^^^
[exit 0]
$ alder fill 'Products.Index."a Staff user gets 404 on products"' - <<'EOF'
{
  let me <- Db.insert User { email: Text.Email "s1@example.com", name: Text.NonEmpty "S", passwordHash: "" }
  let _ <- Test.login { user: Just me }
  let r <- Test.open (Products.Index {})
  Task.succeed (r.answer == PageMissing)
}
EOF
edited app/Products/Index.alder
--- a/app/Products/Index.alder
+++ b/app/Products/Index.alder
@@ -97,5 +97,8 @@
 }
 
 test "a Staff user gets 404 on products" = {
-  ?test
+  let me <- Db.insert User { email: Text.Email "s1@example.com", name: Text.NonEmpty "S", passwordHash: "" }
+  let _ <- Test.login { user: Just me }
+  let r <- Test.open (Products.Index {})
+  Task.succeed (r.answer == PageMissing)
 }
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
ok   Orders.Show "an open order's total over two lines"
ok   Orders.Show "adding a line to a paid order is refused"
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
