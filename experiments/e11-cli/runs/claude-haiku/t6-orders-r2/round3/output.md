```alder-cli
# Orders.Index and Orders.Show were generated from the wrong shape; regenerate them cleanly.
alder remove page Orders.Index
alder remove page Orders.Show
alder new page Orders.Index '/' --blank --requires user
alder fill --decl Orders.Index.view - <<'EOF'
\model session -> <Layout.Page title="Orders"><h1>Orders</h1></Layout.Page>
EOF
alder new msg Orders.Index Create
alder new type Orders.Index "Row = { id: Order.OrderId, status: Order.Status, total: Int }"
alder fill --decl Orders.Index.Model - <<'EOF'
{ rows: List Row }
EOF
alder fill --decl Orders.Index.load - <<'EOF'
\params session -> {
  let orders <- Db.from Order |> Db.where (Order.owner.eq session.user) |> Db.orderBy Order.created.desc |> Db.all
  let rows <- Query.all (List.map (\o -> {
    let total <- Line.orderTotal o.id
    Query.succeed { id: o.id, status: o.status, total: total }
  }) orders)
  Query.succeed { rows: rows }
}
EOF
alder fill --decl Orders.Index.view - <<'EOF'
\model session -> <Layout.Page title="Orders">
    <h1>Orders</h1>
    <form submit={Create}>
      <al-button type="submit">New order</al-button>
    </form>
    <ul id="orders">
      {for row in model.rows ->
        <li>
          <a href={Orders.Show { id: row.id }}>Open order</a>
          <span class="status">{Order.statusLabel (row.status)}</span>
          <span class="total">{Text.fromInt (row.total)}</span>
        </li>}
    </ul>
  </Layout.Page>
EOF
alder fill --decl Orders.Index.update - <<'EOF'
\msg model session -> match msg {
  Create p -> {
    let id <- Db.insert Order { owner: session.user }
    Task.succeed (Redirect (Orders.Show { id: id }))
  },
}
EOF

# the order page
alder new page Orders.Show '/orders/{id}' --param id:Order.OrderId --blank --requires user
alder fill --decl Orders.Show.view - <<'EOF'
\model session -> <Layout.Page title="Order"><h1>Order</h1></Layout.Page>
EOF
alder new msg Orders.Show AddLine product:Product.ProductId
alder new form Orders.Show NewLine quantity:Int.Positive --msg AddLine
alder new msg Orders.Show Pay
alder new msg Orders.Show Cancel
alder fill --decl Orders.Show.Model - <<'EOF'
{ id: Order.OrderId, status: Order.Status, total: Int, lines: List Line.LineRow, products: List Line.Choice }
EOF
alder new decl Orders.Show showOf "Order.OrderId -> Order.Status -> Query Model"
alder fill --decl Orders.Show.showOf - <<'EOF'
\id status -> {
  let lines <- Line.orderLines id
  let total <- Line.orderTotal id
  let products <- Line.choices
  Query.succeed { id: id, status: status, total: total, lines: lines, products: products }
}
EOF
alder fill --decl Orders.Show.load - <<'EOF'
\params session -> {
  let manager <- User.isManager session.user
  let found <- Db.get Order params.id
  match found {
    Nothing -> Query.notFound,
    Just o -> if manager || o.owner == session.user then showOf o.id o.status else Query.notFound,
  }
}
EOF
alder fill --decl Orders.Show.view - <<'EOF'
\model session -> <Layout.Page title="Order">
    <h1>Order</h1>
    <p>Status: <span id="status">{Order.statusLabel (model.status)}</span></p>
    <ul id="lines">
      {for l in model.lines -> <li>{l.name} x {Text.fromInt (l.quantity)} = {Text.fromInt (l.cost)}</li>}
    </ul>
    <p>Total: <span id="total">{Text.fromInt (model.total)}</span></p>
    {if model.status == Open then
      <div>
        {for p in model.products ->
          <form submit={AddLine { product: p.id }}>
            <al-input field="quantity" label={p.name} />
            <al-button type="submit">Add line</al-button>
          </form>}
        <al-button click={Pay}>Mark paid</al-button>
        <al-button variant="destructive" click={Cancel}>Cancel order</al-button>
      </div>
    else Html.none}
    <a href={Orders.Index {}}>Back to orders</a>
  </Layout.Page>
EOF
alder fill --decl Orders.Show.update - <<'EOF'
\msg model session -> match msg {
  AddLine p -> {
    if model.status == Open then {
      let _ <- Db.insert Line { order: model.id, product: p.product, quantity: p.form.quantity }
      Task.succeed (Redirect (Orders.Show { id: model.id }))
    } else Task.fail "the order is not open",
  },
  Pay p -> {
    if model.status == Open then {
      let _ <- Db.update Order model.id (\o -> { o | status: Paid })
      Task.succeed (Redirect (Orders.Show { id: model.id }))
    } else Task.fail "the order is not open",
  },
  Cancel p -> {
    if model.status == Open then {
      let _ <- Db.update Order model.id (\o -> { o | status: Cancelled })
      Task.succeed (Redirect (Orders.Show { id: model.id }))
    } else Task.fail "the order is not open",
  },
}
EOF
alder new test Orders.Show "a line cannot be added to a paid order"
alder fill 'Orders.Show."a line cannot be added to a paid order"' - <<'EOF'
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
alder new test Orders.Show "a Staff user is not shown the order of another user"
alder fill 'Orders.Show."a Staff user is not shown the order of another user"' - <<'EOF'
{
  let ann <- Db.insert User { email: Text.Email "ann@example.com", name: Text.NonEmpty "Ann", passwordHash: "" }
  let bob <- Db.insert User { email: Text.Email "bob@example.com", name: Text.NonEmpty "Bob", passwordHash: "" }
  let o <- Db.insert Order { owner: ann }
  let _ <- Test.login { user: Just bob }
  let r <- Test.open (Orders.Show { id: o })
  Task.succeed (r.answer == PageMissing)
}
EOF
alder new test Orders.Show "a Manager is shown the order of another user"
alder fill 'Orders.Show."a Manager is shown the order of another user"' - <<'EOF'
{
  let ann <- Db.insert User { email: Text.Email "ann@example.com", name: Text.NonEmpty "Ann", passwordHash: "" }
  let boss <- Db.insert User { email: Text.Email "boss@example.com", name: Text.NonEmpty "Boss", passwordHash: "", role: Manager }
  let o <- Db.insert Order { owner: ann }
  let _ <- Test.login { user: Just boss }
  let r <- Test.get (Orders.Show { id: o })
  Task.succeed (Test.text "#status" r.doc == Just "Open")
}
EOF

# products, for managers only
alder new page Products.Index '/products' --blank --requires user
alder fill --decl Products.Index.view - <<'EOF'
\model session -> <Layout.Page title="Products"><h1>Products</h1></Layout.Page>
EOF
alder new form Products.Index NewProduct name:Text.NonEmpty price:Int.NonNegative --msg Add
alder new msg Products.Index Toggle id:Product.ProductId
alder fill --decl Products.Index.load - <<'EOF'
\params session -> {
  let ok <- User.isManager session.user
  let products <- if ok then (Db.from Product |> Db.orderBy Product.name.asc |> Db.all) else Query.notFound
  Query.succeed { products: products }
}
EOF
alder fill --decl Products.Index.Model - <<'EOF'
{ products: List Product }
EOF
alder fill --decl Products.Index.view - <<'EOF'
\model session -> <Layout.Page title="Products">
    <h1>Products</h1>
    <al-card>
      <form submit={Add}>
        <al-input field="name" label="Name" />
        <al-input field="price" label="Price (cents)" />
        <al-button type="submit">Add product</al-button>
      </form>
    </al-card>
    <ul id="products">
      {for p in model.products ->
        <li>
          <span class="name">{p.name}</span>
          <span>{Text.fromInt (p.price)}</span>
          <al-badge variant="outline">{if p.active then "active" else "inactive"}</al-badge>
          <al-button variant="secondary" size="sm" click={Toggle { id: p.id }}>Toggle</al-button>
        </li>}
    </ul>
  </Layout.Page>
EOF
alder fill --decl Products.Index.update - <<'EOF'
\msg model session -> match msg {
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

# login sends users to the order list
alder fill Auth.Login.update.Submit - --hole ?afterLogin <<'EOF'
Orders.Index {}
EOF

# list the remaining holes, so the login test's and the old Index test's can be filled
alder fill --list
```
