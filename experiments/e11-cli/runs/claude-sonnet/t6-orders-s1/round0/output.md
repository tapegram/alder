```alder-cli
# users, with a role
alder new table User email:Text.Email@unique name:Text.NonEmpty passwordHash:Text created:Time@created --no-timestamps
alder new type User "Role = Staff | Manager"
alder edit table User add-field role:Role=Staff

# tables
alder new table Product name:Text.NonEmpty price:Int.NonNegative active:Bool=True --no-timestamps
alder new table Order owner:User.UserId@ref=User created:Time@created --no-timestamps
alder new type Order "Status = Open | Paid | Cancelled"
alder edit table Order add-field status:Status=Open
alder new table Line order:Order.OrderId@ref=Order@cascade product:Product.ProductId@ref=Product quantity:Int.Positive --no-timestamps

# login
alder new auth --users User

# layout component
alder new decl Layout Shell "{ title: Text, children: List (Html a) } -> Html a"
alder fill Layout.Shell - <<'EOF'
\props ->
  <html>
    <head>
      <meta charset="utf-8" />
      <title>{props.title}</title>
    </head>
    <body>
      <main>{props.children}</main>
    </body>
  </html>
EOF

# helpers
alder new decl Order statusLabel "Status -> Text"
alder fill Order.statusLabel - <<'EOF'
\s -> match s {
  Open -> "Open",
  Paid -> "Paid",
  Cancelled -> "Cancelled",
}
EOF

alder new decl Line priceOf "Product.ProductId -> List Product -> Int"
alder fill Line.priceOf - <<'EOF'
\pid products -> match List.find (\p -> p.id == pid) products {
  Just p -> Int.abs p.price,
  Nothing -> 0,
}
EOF
alder new decl Line nameOf "Product.ProductId -> List Product -> Text"
alder fill Line.nameOf - <<'EOF'
\pid products -> match List.find (\p -> p.id == pid) products {
  Just p -> Text.trim p.name,
  Nothing -> "?",
}
EOF
alder new decl Line sumLines "List Line -> List Product -> Int"
alder fill Line.sumLines - <<'EOF'
\lines products -> List.sum (List.map (\l -> l.quantity * priceOf l.product products) lines)
EOF
alder new decl Line total "Order.OrderId -> Query Int"
alder fill Line.total - <<'EOF'
\id -> {
  let lines <- Db.from Line |> Db.where (Line.order.eq id) |> Db.all
  let products <- Db.from Product |> Db.all
  Query.succeed (sumLines lines products)
}
EOF

# pages
alder new page Orders.Index / --requires user --blank
alder new page Orders.Show '/orders/{id}' --param id:Order.OrderId --requires user --blank
alder new page Products.Index /products --requires user --blank

# Orders.Index
alder new type Orders.Index "Row = { id: Order.OrderId, status: Order.Status, total: Int }"
alder fill --decl Orders.Index.Model - <<'EOF'
{ rows: List Row }
EOF
alder new decl Orders.Index mkRow "Order -> Int -> Row"
alder fill Orders.Index.mkRow - <<'EOF'
\o t -> { id: o.id, status: o.status, total: t }
EOF
alder fill Orders.Index.load - <<'EOF'
{
  let orders <- Db.from Order |> Db.where (Order.owner.eq session.user) |> Db.orderBy Order.created.desc |> Db.all
  let rows <- Query.all (List.map (\o -> Query.map (\t -> mkRow o t) (Line.total o.id)) orders)
  Query.succeed { rows: rows }
}
EOF
alder fill Orders.Index.view - <<'EOF'
<Layout.Shell title="Orders">
  <h1>Orders</h1>
</Layout.Shell>
EOF
alder new msg Orders.Index Create
alder fill --decl Orders.Index.view - <<'EOF'
<Layout.Shell title="Orders">
  <h1>Orders</h1>
  <button click={Create}>New order</button>
  <ul id="orders">
    {for r in model.rows ->
      <li><a href={Orders.Show { id: r.id }}>Order</a> <span class="status">{Order.statusLabel r.status}</span> <span class="total">{Text.fromInt r.total}</span></li>}
  </ul>
  <button click={App.Logout}>Log out</button>
</Layout.Shell>
EOF
alder fill Orders.Index.update.Create - <<'EOF'
{
  let id <- Db.insert Order { owner: session.user }
  Task.succeed (Redirect (Orders.Show { id: id }))
}
EOF

# Orders.Show
alder fill --decl Orders.Show.Model - <<'EOF'
{ order: Order, lines: List Line, products: List Product, total: Int }
EOF
alder fill Orders.Show.load - <<'EOF'
{
  let found <- Db.get Order params.id
  match found {
    Nothing -> Query.notFound,
    Just o -> {
      let me <- Db.get User session.user
      let isManager = Maybe.map (\u -> u.role) me == Just Manager
      if o.owner == session.user || isManager then {
        let lines <- Db.from Line |> Db.where (Line.order.eq o.id) |> Db.all
        let products <- Db.from Product |> Db.all
        Query.succeed { order: o, lines: lines, products: products, total: Line.sumLines lines products }
      } else Query.notFound
    },
  }
}
EOF
alder fill Orders.Show.view - <<'EOF'
<Layout.Shell title="Order">
  <h1>Order</h1>
</Layout.Shell>
EOF
alder new msg Orders.Show Pay
alder new msg Orders.Show Cancel
alder new msg Orders.Show AddLine product:Product.ProductId
alder new form Orders.Show NewLine quantity:Int.Positive --msg AddLine
alder fill --decl Orders.Show.view - <<'EOF'
<Layout.Shell title="Order">
  <h1>Order</h1>
  <p>Status: <span id="status">{Order.statusLabel model.order.status}</span></p>
  <p>Total: <span id="total">{Text.fromInt model.total}</span></p>
  <ul id="lines">
    {for l in model.lines ->
      <li><span class="name">{Line.nameOf l.product model.products}</span> x <span class="quantity">{Text.fromInt l.quantity}</span></li>}
  </ul>
  {if model.order.status == Open then
    <div>
      {for p in List.filter (\x -> x.active) model.products ->
        <form submit={AddLine { product: p.id }}>
          <Errors form />
          <Input field="quantity" label={p.name} />
          <button>Add</button>
        </form>}
      <button click={Pay}>Mark paid</button>
      <button click={Cancel}>Cancel order</button>
    </div>
  else Html.none}
  <a href={Orders.Index {}}>Back</a>
</Layout.Shell>
EOF
alder fill Orders.Show.update.Pay - <<'EOF'
if model.order.status == Open then {
  let _ <- Db.update Order model.order.id (\o -> { o | status: Paid })
  Task.succeed (Redirect (Orders.Show { id: model.order.id }))
} else Task.succeed (Reject "This order is not open.")
EOF
alder fill Orders.Show.update.Cancel - <<'EOF'
if model.order.status == Open then {
  let _ <- Db.update Order model.order.id (\o -> { o | status: Cancelled })
  Task.succeed (Redirect (Orders.Show { id: model.order.id }))
} else Task.succeed (Reject "This order is not open.")
EOF
alder fill Orders.Show.update.AddLine - <<'EOF'
if model.order.status == Open && List.any (\x -> x.id == p.product && x.active) model.products then {
  let _ <- Db.insert Line { order: model.order.id, product: p.product, quantity: p.form.quantity }
  Task.succeed (Redirect (Orders.Show { id: model.order.id }))
} else Task.succeed (Reject "This order is not open.")
EOF

# Products.Index
alder fill --decl Products.Index.Model - <<'EOF'
{ products: List Product }
EOF
alder fill Products.Index.load - <<'EOF'
{
  let me <- Db.get User session.user
  if Maybe.map (\u -> u.role) me == Just Manager then {
    let products <- Db.from Product |> Db.orderBy Product.name.asc |> Db.all
    Query.succeed { products: products }
  } else Query.notFound
}
EOF
alder fill Products.Index.view - <<'EOF'
<Layout.Shell title="Products">
  <h1>Products</h1>
</Layout.Shell>
EOF
alder new msg Products.Index Toggle id:Product.ProductId
alder new form Products.Index NewProduct name:Text.NonEmpty price:Int.NonNegative --msg Add
alder fill --decl Products.Index.view - <<'EOF'
<Layout.Shell title="Products">
  <h1>Products</h1>
  <form submit={Add}>
    <Errors form />
    <Input field="name" label="Name" />
    <Input field="price" label="Price (cents)" />
    <button>Add</button>
  </form>
  <ul id="products">
    {for p in model.products ->
      <li><span class="name">{p.name}</span> <span class="price">{Text.fromInt p.price}</span> <span class="active">{if p.active then "active" else "inactive"}</span> <button click={Toggle { id: p.id }}>Toggle active</button></li>}
  </ul>
  <a href={Orders.Index {}}>Orders</a>
</Layout.Shell>
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

# tests
alder new test Orders.Show "an open order total over two lines"
alder fill 'Orders.Show."an open order total over two lines"' - <<'EOF'
{
  let u <- Db.insert User { email: Text.Email "a@example.com", name: Text.NonEmpty "A", passwordHash: "" }
  let p1 <- Db.insert Product { name: Text.NonEmpty "Pen", price: Int.NonNegative 250 }
  let p2 <- Db.insert Product { name: Text.NonEmpty "Ink", price: Int.NonNegative 500 }
  let o <- Db.insert Order { owner: u }
  let _ <- Db.insert Line { order: o, product: p1, quantity: Int.Positive 2 }
  let _ <- Db.insert Line { order: o, product: p2, quantity: Int.Positive 1 }
  let _ <- Test.login { user: Just u }
  let page <- Test.get (Orders.Show { id: o })
  let t <- Line.total o
  Task.succeed ({ total: t, shown: Test.text "#total" page.doc, status: Test.text "#status" page.doc } == { total: 1000, shown: Just "1000", status: Just "Open" })
}
EOF
alder new test Orders.Show "adding a line to a paid order is refused"
alder fill 'Orders.Show."adding a line to a paid order is refused"' - <<'EOF'
{
  let u <- Db.insert User { email: Text.Email "a@example.com", name: Text.NonEmpty "A", passwordHash: "" }
  let prod <- Db.insert Product { name: Text.NonEmpty "Pen", price: Int.NonNegative 250 }
  let o <- Db.insert Order { owner: u }
  let _ <- Test.login { user: Just u }
  let _ <- Test.get (Orders.Show { id: o })
  let _ <- Db.update Order o (\x -> { x | status: Paid })
  let r <- Test.submit (AddLine { product: prod }) { quantity: Int.Positive 1 }
  let n <- Db.from Line |> Db.count
  Task.succeed (n == 0)
}
EOF
alder new test Orders.Show "a Staff user opening another user order is not found"
alder fill 'Orders.Show."a Staff user opening another user order is not found"' - <<'EOF'
{
  let owner <- Db.insert User { email: Text.Email "a@example.com", name: Text.NonEmpty "A", passwordHash: "" }
  let staff <- Db.insert User { email: Text.Email "b@example.com", name: Text.NonEmpty "B", passwordHash: "", role: Staff }
  let o <- Db.insert Order { owner: owner }
  let _ <- Test.login { user: Just staff }
  let s <- Test.open (Orders.Show { id: o })
  Task.succeed (s.answer == PageMissing)
}
EOF
alder new test Orders.Show "a Manager opening another user order sees it"
alder fill 'Orders.Show."a Manager opening another user order sees it"' - <<'EOF'
{
  let owner <- Db.insert User { email: Text.Email "a@example.com", name: Text.NonEmpty "A", passwordHash: "" }
  let mgr <- Db.insert User { email: Text.Email "c@example.com", name: Text.NonEmpty "C", passwordHash: "", role: Manager }
  let o <- Db.insert Order { owner: owner }
  let _ <- Test.login { user: Just mgr }
  let m <- Test.open (Orders.Show { id: o })
  Task.succeed (m.answer == Shown)
}
EOF

# where login goes afterwards
alder fill --list
alder fill Auth.Login.update.Submit - --hole ?afterLogin <<'EOF'
Orders.Index {}
EOF
```
