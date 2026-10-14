```alder-cli
# helpers in Order and Product
alder new decl Order statusText "Status -> Text"
alder fill Order.statusText - <<'EOF'
\s -> match s {
  Open -> "Open",
  Paid -> "Paid",
  Cancelled -> "Cancelled",
}
EOF
alder new decl Order isOpenName "Text -> Bool"
alder fill Order.isOpenName - <<'EOF'
\t -> t == "Open"
EOF
alder new decl Product isFree "Int -> Bool"
alder fill Product.isFree - <<'EOF'
\n -> n == 0
EOF

# helpers in Line
alder new type Line "Row = { name: Text, quantity: Int, price: Int }"
alder new type Line "Choice = { id: Product.ProductId, name: Text }"
alder new decl Line rows "List Line -> List Product -> List Row"
alder fill Line.rows - <<'EOF'
\lines products -> List.filterMap (\l -> Maybe.map (\p -> { name: p.name, quantity: l.quantity, price: p.price }) (List.find (\p -> p.id == l.product) products)) lines
EOF
alder new decl Line total "List Line -> List Product -> Int"
alder fill Line.total - <<'EOF'
\lines products -> List.sum (List.map (\r -> r.quantity * r.price) (rows lines products))
EOF
alder new decl Line choices "List Product -> List Choice"
alder fill Line.choices - <<'EOF'
\products -> List.map (\p -> { id: p.id, name: p.name }) (List.filter (\p -> p.active) products)
EOF

# Orders.Index
alder new page Orders.Index / --blank --requires user
alder new decl Orders.Index imports "Bool"
alder fill Orders.Index.imports - <<'EOF'
Order.isOpenName "x" && Line.total [] [] == 0 && Product.isFree 0
EOF
alder new type Orders.Index "Row = { id: Order.OrderId, status: Order.Status, total: Int }"
alder fill --decl Orders.Index.Model - <<'EOF'
{ orders: List Row }
EOF
alder fill Orders.Index.load - <<'EOF'
\params session -> {
  let orders <- Db.from Order |> Db.where (Order.owner.eq session.user) |> Db.orderBy Order.created.desc |> Db.all
  let lines <- Db.from Line |> Db.where (Line.order.in (List.map (\o -> o.id) orders)) |> Db.all
  let products <- Db.from Product |> Db.all
  Query.succeed { orders: List.map (\o -> { id: o.id, status: o.status, total: Line.total (List.filter (\l -> l.order == o.id) lines) products }) orders }
}
EOF
alder fill Orders.Index.view - <<'EOF'
\model session -> <Layout.Page title="Orders">
  <h1>Orders</h1>
  <ul id="orders">
    {for o in model.orders -> <li><a href={Orders.Show { id: o.id }}>Order</a> <span class="status">{Order.statusText o.status}</span> <span class="total">{Text.fromInt o.total}</span></li>}
  </ul>
</Layout.Page>
EOF

# Orders.Show
alder new page Orders.Show '/orders/{id}' --blank --show Order --requires user
alder new decl Orders.Show imports "Bool"
alder fill Orders.Show.imports - <<'EOF'
Order.isOpenName "x" && Line.total [] [] == 0 && Product.isFree 0
EOF
alder fill --decl Orders.Show.Model - <<'EOF'
{ order: Order, rows: List Line.Row, choices: List Line.Choice, total: Int }
EOF
alder fill Orders.Show.load - <<'EOF'
\params session -> {
  let found <- Db.get Order params.id
  let me <- Db.get User session.user
  match found {
    Nothing -> Query.notFound,
    Just o -> {
      let manager = Maybe.map (\u -> u.role) me == Just Manager
      if o.owner != session.user && not manager then Query.notFound else {
        let lines <- Db.from Line |> Db.where (Line.order.eq o.id) |> Db.all
        let products <- Db.from Product |> Db.all
        Query.succeed { order: o, rows: Line.rows lines products, choices: Line.choices products, total: Line.total lines products }
      }
    },
  }
}
EOF
alder fill Orders.Show.view - <<'EOF'
\model session -> <Layout.Page title="Order">
  <h1>Order</h1>
  <p>Status: <span id="status">{Order.statusText model.order.status}</span></p>
  <ul id="lines">
    {for r in model.rows -> <li>{r.name} x {Text.fromInt r.quantity} at {Text.fromInt r.price}</li>}
  </ul>
  <p>Total: <span id="total">{Text.fromInt model.total}</span></p>
  <a href={Orders.Index {}}>Back</a>
</Layout.Page>
EOF
alder new msg Orders.Show AddLine product:Product.ProductId
alder new form Orders.Show NewLine quantity:Int.Positive --msg AddLine
alder new msg Orders.Show Pay
alder new msg Orders.Show Cancel
alder fill --decl Orders.Show.view - <<'EOF'
\model session -> <Layout.Page title="Order">
  <h1>Order</h1>
  <p>Status: <span id="status">{Order.statusText model.order.status}</span></p>
  <ul id="lines">
    {for r in model.rows -> <li>{r.name} x {Text.fromInt r.quantity} at {Text.fromInt r.price}</li>}
  </ul>
  <p>Total: <span id="total">{Text.fromInt model.total}</span></p>
  {if model.order.status == Open then <div id="actions">
    <al-button click={Pay}>Pay</al-button>
    <al-button click={Cancel}>Cancel</al-button>
    {for c in model.choices -> <form submit={AddLine { product: c.id }}>
      <h3>{c.name}</h3>
      <Errors form />
      <al-input field="quantity" label="Quantity" />
      <al-button type="submit">Add</al-button>
    </form>}
  </div> else Html.none}
  <a href={Orders.Index {}}>Back</a>
</Layout.Page>
EOF
alder fill Orders.Show.update.AddLine - <<'EOF'
{
  let prod <- Db.get Product p.product
  let ok = Maybe.map (\pr -> pr.active) prod == Just True
  if model.order.status != Open || not ok then Task.succeed (Reject "This order is not open.") else {
    let _ <- Db.insert Line { order: model.order.id, product: p.product, quantity: p.form.quantity }
    Task.succeed (Redirect (Orders.Show { id: model.order.id }))
  }
}
EOF
alder fill Orders.Show.update.Pay - <<'EOF'
if model.order.status != Open then Task.succeed (Reject "This order is not open.") else {
  let _ <- Db.update Order model.order.id (\o -> { o | status: Paid })
  Task.succeed (Redirect (Orders.Show { id: model.order.id }))
}
EOF
alder fill Orders.Show.update.Cancel - <<'EOF'
if model.order.status != Open then Task.succeed (Reject "This order is not open.") else {
  let _ <- Db.update Order model.order.id (\o -> { o | status: Cancelled })
  Task.succeed (Redirect (Orders.Show { id: model.order.id }))
}
EOF

# Orders.Index messages
alder new msg Orders.Index Create
alder fill --decl Orders.Index.view - <<'EOF'
\model session -> <Layout.Page title="Orders">
  <h1>Orders</h1>
  <al-button click={Create}>New order</al-button>
  <ul id="orders">
    {for o in model.orders -> <li><a href={Orders.Show { id: o.id }}>Order</a> <span class="status">{Order.statusText o.status}</span> <span class="total">{Text.fromInt o.total}</span></li>}
  </ul>
  <a href={Products.Index {}}>Products</a>
  <al-button variant="outline" click={App.Logout}>Log out</al-button>
</Layout.Page>
EOF
alder fill Orders.Index.update.Create - <<'EOF'
{
  let id <- Db.insert Order { owner: session.user }
  Task.succeed (Redirect (Orders.Show { id: id }))
}
EOF

# Products.Index
alder new page Products.Index /products --blank --requires user
alder new decl Products.Index imports "Bool"
alder fill Products.Index.imports - <<'EOF'
Product.isFree 0
EOF
alder fill --decl Products.Index.Model - <<'EOF'
{ products: List Product }
EOF
alder fill Products.Index.load - <<'EOF'
\params session -> {
  let me <- Db.get User session.user
  if Maybe.map (\u -> u.role) me == Just Manager then {
    let ps <- Db.from Product |> Db.orderBy Product.name.asc |> Db.all
    Query.succeed { products: ps }
  } else Query.notFound
}
EOF
alder fill Products.Index.view - <<'EOF'
\model session -> <Layout.Page title="Products">
  <h1>Products</h1>
  <ul id="products">
    {for p in model.products -> <li><span class="name">{p.name}</span> <span class="price">{Text.fromInt p.price}</span></li>}
  </ul>
</Layout.Page>
EOF
alder new msg Products.Index Toggle id:Product.ProductId
alder new form Products.Index NewProduct name:Text.NonEmpty price:Int.NonNegative --msg Add
alder fill --decl Products.Index.view - <<'EOF'
\model session -> <Layout.Page title="Products">
  <h1>Products</h1>
  <al-card>
    <form submit={Add}>
      <Errors form />
      <al-input field="name" label="Name" />
      <al-input field="price" label="Price (cents)" />
      <al-button type="submit">Add</al-button>
    </form>
  </al-card>
  <ul id="products">
    {for p in model.products -> <li><span class="name">{p.name}</span> <span class="price">{Text.fromInt p.price}</span> <span class="active">{if p.active then "active" else "inactive"}</span> <al-button click={Toggle { id: p.id }}>Toggle</al-button></li>}
  </ul>
  <a href={Orders.Index {}}>Orders</a>
</Layout.Page>
EOF
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

# login hole
alder fill Auth.Login.update - --hole ?afterLogin <<'EOF'
Orders.Index {}
EOF
alder fill --list

# tests
alder new test Orders.Show "an Open order total over two lines"
alder fill 'Orders.Show."an Open order total over two lines"' - <<'EOF'
{
  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
  let a <- Db.insert Product { name: Text.NonEmpty "A", price: Int.NonNegative 250 }
  let b <- Db.insert Product { name: Text.NonEmpty "B", price: Int.NonNegative 100 }
  let o <- Db.insert Order { owner: me }
  let _ <- Db.insert Line { order: o, product: a, quantity: Int.Positive 2 }
  let _ <- Db.insert Line { order: o, product: b, quantity: Int.Positive 3 }
  let lines <- Db.from Line |> Db.where (Line.order.eq o) |> Db.all
  let ps <- Db.from Product |> Db.all
  Task.succeed (Line.total lines ps == 800)
}
EOF
alder new test Orders.Show "adding a line to a Paid order is refused"
alder fill 'Orders.Show."adding a line to a Paid order is refused"' - <<'EOF'
{
  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
  let p <- Db.insert Product { name: Text.NonEmpty "A", price: Int.NonNegative 250 }
  let o <- Db.insert Order { owner: me, status: Paid }
  let _ <- Test.login { user: Just me }
  let r <- Test.run (update (AddLine { product: p, form: { quantity: Int.Positive 1 } }) { order: { id: o, owner: me, status: Paid, created: Time.epoch }, rows: [], choices: [], total: 0 } { user: me })
  let n <- Db.from Line |> Db.count
  Task.succeed ({ value: r.value, n: n } == { value: Ok (Reject "This order is not open."), n: 0 })
}
EOF
alder new test Orders.Show "a Staff user opening another order gets not found"
alder fill 'Orders.Show."a Staff user opening another order gets not found"' - <<'EOF'
{
  let owner <- Db.insert User { email: Text.Email "owner@example.com", name: Text.NonEmpty "Owner", passwordHash: "" }
  let staff <- Db.insert User { email: Text.Email "staff@example.com", name: Text.NonEmpty "Staff", passwordHash: "" }
  let o <- Db.insert Order { owner: owner }
  let _ <- Test.login { user: Just staff }
  let r <- Test.open (Orders.Show { id: o })
  Task.succeed (r.answer == PageMissing)
}
EOF
alder new test Orders.Show "a Manager opening another order sees it"
alder fill 'Orders.Show."a Manager opening another order sees it"' - <<'EOF'
{
  let owner <- Db.insert User { email: Text.Email "owner@example.com", name: Text.NonEmpty "Owner", passwordHash: "" }
  let boss <- Db.insert User { email: Text.Email "boss@example.com", name: Text.NonEmpty "Boss", passwordHash: "", role: Manager }
  let o <- Db.insert Order { owner: owner }
  let _ <- Test.login { user: Just boss }
  let r <- Test.open (Orders.Show { id: o })
  Task.succeed (r.answer == Shown)
}
EOF
alder check
```
