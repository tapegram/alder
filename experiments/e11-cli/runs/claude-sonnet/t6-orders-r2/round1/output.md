```alder-cli
alder new auth --users User
alder fill --list

alder new page Orders.Index / --blank --requires user
alder fill --decl Orders.Index.Model - <<'EOF'
{ orders: List { id: Order.OrderId, status: Order.Status, total: Int } }
EOF
alder fill Orders.Index.load - <<'EOF'
\params session -> {
  let orders <- Db.from Order |> Db.where (Order.owner.eq session.user) |> Db.orderBy Order.created.desc |> Db.all
  let rows <- Query.all (List.map (\o -> Query.map (\t -> { id: o.id, status: o.status, total: t }) (Line.total o.id)) orders)
  Query.succeed { orders: rows }
}
EOF
alder fill Orders.Index.view - <<'EOF'
\model session ->
  <Layout.Page title="Orders">
    <h1>Orders</h1>
  </Layout.Page>
EOF

alder new page Orders.Show '/orders/{id}' --param id:Order.OrderId --blank --requires user
alder fill --decl Orders.Show.Model - <<'EOF'
{ order: Order, lines: List Line, products: List Product, total: Int }
EOF
alder fill Orders.Show.load - <<'EOF'
\params session -> {
  let found <- Db.get Order params.id
  let manager <- User.isManager session.user
  match found {
    Nothing -> Query.notFound,
    Just o -> if o.owner == session.user || manager then {
      let lines <- Db.from Line |> Db.where (Line.order.eq o.id) |> Db.all
      let products <- Db.from Product |> Db.all
      let total <- Line.total o.id
      Query.succeed { order: o, lines: lines, products: products, total: total }
    } else Query.notFound,
  }
}
EOF
alder fill Orders.Show.view - <<'EOF'
\model session ->
  <Layout.Page title="Order">
    <h1>Order</h1>
  </Layout.Page>
EOF

alder new page Products.Index /products --blank --requires user
alder fill --decl Products.Index.Model - <<'EOF'
{ products: List Product }
EOF
alder fill Products.Index.load - <<'EOF'
\params session -> {
  let manager <- User.isManager session.user
  if manager then {
    let products <- Db.from Product |> Db.orderBy Product.name.asc |> Db.all
    Query.succeed { products: products }
  } else Query.notFound
}
EOF
alder fill Products.Index.view - <<'EOF'
\model session ->
  <Layout.Page title="Products">
    <h1>Products</h1>
  </Layout.Page>
EOF

alder new msg Orders.Index Create
alder fill Orders.Index.update.Create - <<'EOF'
{
  let id <- Db.insert Order { owner: session.user }
  Task.succeed (Redirect (Orders.Show { id: id }))
}
EOF

alder new msg Orders.Show Pay
alder new msg Orders.Show Cancel
alder new msg Orders.Show AddLine product:Product.ProductId
alder new form Orders.Show NewLine quantity:Int.Positive --msg AddLine
alder fill Orders.Show.update.Pay - <<'EOF'
if model.order.status == Open then {
  let _ <- Db.update Order model.order.id (\o -> { o | status: Paid })
  Task.succeed (Redirect (Orders.Show { id: model.order.id }))
} else Task.succeed (Reject "Order is not open.")
EOF
alder fill Orders.Show.update.Cancel - <<'EOF'
if model.order.status == Open then {
  let _ <- Db.update Order model.order.id (\o -> { o | status: Cancelled })
  Task.succeed (Redirect (Orders.Show { id: model.order.id }))
} else Task.succeed (Reject "Order is not open.")
EOF
alder fill Orders.Show.update.AddLine - <<'EOF'
if model.order.status == Open then {
  let _ <- Db.insert Line { order: model.order.id, product: p.product, quantity: p.form.quantity }
  Task.succeed (Redirect (Orders.Show { id: model.order.id }))
} else Task.succeed (Reject "Order is not open.")
EOF

alder new form Products.Index NewProduct name:Text.NonEmpty price:Int.NonNegative --msg Add
alder new msg Products.Index Toggle id:Product.ProductId
alder fill Products.Index.update.Add - <<'EOF'
{
  let _ <- Db.insert Product { name: p.form.name, price: p.form.price }
  Task.succeed (Redirect (Products.Index {}))
}
EOF
alder fill Products.Index.update.Toggle - <<'EOF'
{
  let _ <- Db.update Product p.id (\x -> { x | active: not x.active })
  Task.succeed (Redirect (Products.Index {}))
}
EOF

alder fill --decl Orders.Index.view - <<'EOF'
\model session ->
  <Layout.Page title="Orders">
    <h1>Orders</h1>
    <button click={Create}>New order</button>
    <ul id="orders">
      {for o in model.orders ->
        <li>
          <a href={Orders.Show { id: o.id }}>Order</a>
          <span class="status">{Order.statusText o.status}</span>
          <span class="total">{Text.fromInt o.total}</span>
        </li>}
    </ul>
    <button click={App.Logout}>Log out</button>
  </Layout.Page>
EOF
alder fill --decl Orders.Show.view - <<'EOF'
\model session ->
  <Layout.Page title="Order">
    <h1>Order</h1>
    <p>Status: <span id="status">{Order.statusText model.order.status}</span></p>
    <p>Total: <span id="total">{Text.fromInt model.total}</span></p>
    <ul id="lines">
      {for l in model.lines ->
        <li>{Maybe.withDefault "?" (Maybe.map (\p -> Text.trim p.name) (List.find (\p -> p.id == l.product) model.products))} x {Text.fromInt l.quantity}</li>}
    </ul>
    {if model.order.status == Open then
      <div>
        {for p in List.filter (\p -> p.active) model.products ->
          <form submit={AddLine { product: p.id }}>
            <span>{p.name}</span>
            <Input field="quantity" label="Quantity" />
            <Errors form />
            <button>Add</button>
          </form>}
        <button click={Pay}>Mark paid</button>
        <button click={Cancel}>Mark cancelled</button>
      </div>
    else Html.none}
    <a href={Orders.Index {}}>Back</a>
  </Layout.Page>
EOF
alder fill --decl Products.Index.view - <<'EOF'
\model session ->
  <Layout.Page title="Products">
    <h1>Products</h1>
    <ul id="products">
      {for p in model.products ->
        <li>
          <span class="name">{p.name}</span>
          <span class="price">{Text.fromInt p.price}</span>
          <button click={Toggle { id: p.id }}>{if p.active then "Deactivate" else "Activate"}</button>
        </li>}
    </ul>
    <form submit={Add}>
      <Errors form />
      <Input field="name" label="Name" />
      <Input field="price" label="Price" />
      <button>Add</button>
    </form>
    <a href={Orders.Index {}}>Orders</a>
  </Layout.Page>
EOF
alder check
```
