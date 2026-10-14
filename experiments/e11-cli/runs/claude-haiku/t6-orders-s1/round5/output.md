```alder-cli
# Orders.Show keeps the product in its form (`msg AddLine form NewLine`): the commands cannot
# attach a form to a message that already has parameters, so the shape stays as it is.

# Products: a form to add a product, and a list with a toggle button per product
alder fill --decl Products.Index.view - <<'EOF'
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

# Toggle flips the product's active flag, then the page is shown again
alder fill Products.Index.update.Toggle - --hole ?toggle <<'EOF'
{
  let _ <- Db.update Product p.id (\x -> { x | active: not x.active })
  Task.succeed (Redirect (Products.Index {}))
}
EOF

# The three Products tests need a Manager: a Staff user gets 404 from load
alder fill --decl 'Products.Index."submitting NewProduct adds a row"' - <<'EOF'
{
  let me <- Db.insert User { email: Text.Email "email-1@example.com", name: Text.NonEmpty "name-1", passwordHash: "passwordhash-1", role: Manager }
  let _ <- Test.login { user: Just me }
  let _ <- Test.get (Products.Index {})
  let r <- Test.submit NewProduct { name: Text.NonEmpty "name-1", price: Int.NonNegative 1 }
  let rows <- Db.from Product |> Db.all
  Task.succeed ({ answer: r.answer, rows: List.map (\row -> { name: row.name, price: row.price, active: row.active }) rows } == { answer: Redirected (Products.Index {}), rows: [{ name: Text.NonEmpty "name-1", price: Int.NonNegative 1, active: True }] })
}
EOF

alder fill --decl 'Products.Index."NewProduct refuses an empty name"' - <<'EOF'
{
  let me <- Db.insert User { email: Text.Email "email-1@example.com", name: Text.NonEmpty "name-1", passwordHash: "passwordhash-1", role: Manager }
  let _ <- Test.login { user: Just me }
  let _ <- Test.get (Products.Index {})
  let r <- Test.submitRaw NewProduct [{ name: "name", value: "" }, { name: "price", value: "1" }]
  let rows <- Db.from Product |> Db.count
  Task.succeed ({ answer: r.answer, rows: rows } == { answer: FormRejected [{ field: "name", message: "must not be empty" }], rows: 0 })
}
EOF

alder fill --decl 'Products.Index."pressing Toggle"' - <<'EOF'
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

# Orders: list each order with its status and total; link text fits any status
alder fill --decl Orders.Index.view - <<'EOF'
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

# Creating an order shows one new Open order in the list
alder fill 'Orders.Index."pressing Create"' - --hole ?expected <<'EOF'
r.answer != PageMissing && Test.count "#orders li" after.doc == 1 && Test.text ".status" after.doc == Just "Open"
EOF
```
