### Round 5

```text
$ alder fill --decl Products.Index.view - <<'EOF'
<Layout.Page title="Products">
    <h1>Products</h1>
    <al-card>
      <form submit={Add}>
        <Errors form />
        <al-input field="name" label="Name" />
        <al-input field="price" label="Price in cents" />
        <al-button type="submit">Add product</al-button>
      </form>
    </al-card>
    <ul id="products">
      {for p in model.products ->
        <li>
          <span class="name">{p.name}</span>
          <span class="price">{Text.fromInt p.price}</span>
          <span class="state">{if p.active then "active" else "inactive"}</span>
          <al-button variant="secondary" size="sm" click={Toggle { id: p.id }}>{if p.active then "Deactivate" else "Activate"}</al-button>
        </li>}
    </ul>
  </Layout.Page>
EOF
edited app/Products/Index.alder
--- a/app/Products/Index.alder
+++ b/app/Products/Index.alder
@@ -36,21 +36,27 @@
 }
 
 view : Model -> Session -> Html Msg
-view model session = <Layout.Page title="Products">
-    <h1>Products</h1>
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
-  </Layout.Page>
+view model session =
+  <Layout.Page title="Products">
+      <h1>Products</h1>
+      <al-card>
+        <form submit={Add}>
+          <Errors form />
+          <al-input field="name" label="Name" />
+          <al-input field="price" label="Price in cents" />
+          <al-button type="submit">Add product</al-button>
+        </form>
+      </al-card>
+      <ul id="products">
+        {for p in model.products ->
+          <li>
+            <span class="name">{p.name}</span>
+            <span class="price">{Text.fromInt p.price}</span>
+            <span class="state">{if p.active then "active" else "inactive"}</span>
+            <al-button variant="secondary" size="sm" click={Toggle { id: p.id }}>{if p.active then "Deactivate" else "Activate"}</al-button>
+          </li>}
+      </ul>
+    </Layout.Page>
 
 test "a visitor is sent to log in" = {
   let r <- Test.open (Products.Index {})
app/Products/Index.alder:35:15: warning[W0002]: hole ?toggle has type Task Nav
    Toggle p -> ?toggle,
                ^^^^^^^
app/Products/Index.alder:94:16: warning[W0002]: hole ?expected has type Bool
    Task.succeed ?expected
                 ^^^^^^^^^
[exit 0]
$ alder fill Products.Index.update.Toggle - --hole ?toggle <<'EOF'
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
app/Products/Index.alder:97:16: warning[W0002]: hole ?expected has type Bool
    Task.succeed ?expected
                 ^^^^^^^^^
[exit 0]
$ alder fill --decl 'Products.Index."submitting NewProduct adds a row"' - <<'EOF'
{
  let me <- Db.insert User { email: Text.Email "email-1@example.com", name: Text.NonEmpty "name-1", passwordHash: "passwordhash-1", role: Manager }
  let _ <- Test.login { user: Just me }
  let _ <- Test.get (Products.Index {})
  let r <- Test.submit NewProduct { name: Text.NonEmpty "name-1", price: Int.NonNegative 1 }
  let rows <- Db.from Product |> Db.all
  Task.succeed ({ answer: r.answer, rows: List.map (\row -> { name: row.name, price: row.price, active: row.active }) rows } == { answer: Redirected (Products.Index {}), rows: [{ name: Text.NonEmpty "name-1", price: Int.NonNegative 1, active: True }] })
}
EOF
edited app/Products/Index.alder
--- a/app/Products/Index.alder
+++ b/app/Products/Index.alder
@@ -67,7 +67,7 @@
 }
 
 test "submitting NewProduct adds a row" = {
-  let me <- Db.insert User { email: Text.Email "email-1@example.com", name: Text.NonEmpty "name-1", passwordHash: "passwordhash-1" }
+  let me <- Db.insert User { email: Text.Email "email-1@example.com", name: Text.NonEmpty "name-1", passwordHash: "passwordhash-1", role: Manager }
   let _ <- Test.login { user: Just me }
   let _ <- Test.get (Products.Index {})
   let r <- Test.submit NewProduct { name: Text.NonEmpty "name-1", price: Int.NonNegative 1 }
app/Products/Index.alder:97:16: warning[W0002]: hole ?expected has type Bool
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
  Task.succeed ({ answer: r.answer, rows: rows } == { answer: FormRejected [{ field: "name", message: "must not be empty" }], rows: 0 })
}
EOF
edited app/Products/Index.alder
--- a/app/Products/Index.alder
+++ b/app/Products/Index.alder
@@ -76,7 +76,7 @@
 }
 
 test "NewProduct refuses an empty name" = {
-  let me <- Db.insert User { email: Text.Email "email-1@example.com", name: Text.NonEmpty "name-1", passwordHash: "passwordhash-1" }
+  let me <- Db.insert User { email: Text.Email "email-1@example.com", name: Text.NonEmpty "name-1", passwordHash: "passwordhash-1", role: Manager }
   let _ <- Test.login { user: Just me }
   let _ <- Test.get (Products.Index {})
   let r <- Test.submitRaw NewProduct [{ name: "name", value: "" }, { name: "price", value: "1" }]
app/Products/Index.alder:97:16: warning[W0002]: hole ?expected has type Bool
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
  Task.succeed (r.answer == Redirected (Products.Index {}) && Maybe.map (\p -> p.active) after == Just False)
}
EOF
edited app/Products/Index.alder
--- a/app/Products/Index.alder
+++ b/app/Products/Index.alder
@@ -84,15 +84,12 @@
   Task.succeed ({ answer: r.answer, rows: rows } == { answer: FormRejected [{ field: "name", message: "must not be empty" }], rows: 0 })
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
+  Task.succeed (r.answer == Redirected (Products.Index {}) && Maybe.map (\p -> p.active) after == Just False)
 }
rewriting a test: `alder test --mutate` is the check that it still tests something
[exit 0]
$ alder fill --decl Orders.Index.view - <<'EOF'
<Layout.Page title="Orders">
    <h1>Orders</h1>
    <ul id="orders">
      {for o in model.orders ->
        <li>
          <a href={Orders.Show { id: o.id }}>Details</a>
          <span class="status">{Order.statusText o.status}</span>
          <span class="total">{Text.fromInt (Line.orderTotal model.products (List.filter (\l -> l.order == o.id) model.lines))}</span>
        </li>}
    </ul>
    <al-button click={Create}>New order</al-button>
  </Layout.Page>
EOF
edited app/Orders/Index.alder
--- a/app/Orders/Index.alder
+++ b/app/Orders/Index.alder
@@ -29,19 +29,19 @@
 }
 
 view : Model -> Session -> Html Msg
-view model session = <Layout.Page title="Orders">
-    <h1>Orders</h1>
-    <ul id="orders">
-      {for o in model.orders ->
-        <li>
-          <a href={Orders.Show { id: o.id }}>Open order</a>
-          <span class="status">{Order.statusText o.status}</span>
-          <span class="total">{Text.fromInt (Line.orderTotal model.products (List.filter (\l -> l.order == o.id) model.lines))}</span>
-        </li>}
-    </ul>
-
-    <al-button click={Create}>Create</al-button>
-  </Layout.Page>
+view model session =
+  <Layout.Page title="Orders">
+      <h1>Orders</h1>
+      <ul id="orders">
+        {for o in model.orders ->
+          <li>
+            <a href={Orders.Show { id: o.id }}>Details</a>
+            <span class="status">{Order.statusText o.status}</span>
+            <span class="total">{Text.fromInt (Line.orderTotal model.products (List.filter (\l -> l.order == o.id) model.lines))}</span>
+          </li>}
+      </ul>
+      <al-button click={Create}>New order</al-button>
+    </Layout.Page>
 
 test "a visitor is sent to log in" = {
   let r <- Test.open (Orders.Index {})
app/Orders/Index.alder:62:16: warning[W0002]: hole ?expected has type Bool
    Task.succeed ?expected
                 ^^^^^^^^^
[exit 0]
$ alder fill 'Orders.Index."pressing Create"' - --hole ?expected <<'EOF'
r.answer != PageMissing && Test.count "#orders li" after.doc == 1 && Test.text ".status" after.doc == Just "Open"
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
+  Task.succeed (r.answer != PageMissing && Test.count "#orders li" after.doc == 1 && Test.text ".status" after.doc == Just "Open")
 }
[exit 0]
```

The harness ran `alder check`, `alder build` and `alder test` on the app: all pass.
