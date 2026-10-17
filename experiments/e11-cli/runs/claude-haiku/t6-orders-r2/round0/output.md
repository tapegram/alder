```alder-cli
# the users and login: User (with its role), then the login the auth generator writes
alder new type User "Role = Staff | Manager"
alder new table User email:Text.Email@unique name:Text.NonEmpty passwordHash:Text role:Role=Staff created:Time@created --no-timestamps
alder new auth --users User

# products
alder new table Product name:Text.NonEmpty price:Int.NonNegative active:Bool=True --no-timestamps

# orders, with their status
alder new type Order "Status = Open | Paid | Cancelled"
alder new table Order owner:User.UserId@ref=User status:Status=Open created:Time@created --no-timestamps

# line items
alder new table Line order:Order.OrderId@ref=Order@cascade product:Product.ProductId@ref=Product quantity:Int.Positive --no-timestamps

# the status as text
alder new decl Order statusLabel "Status -> Text"
alder fill Order.statusLabel - <<'EOF'
\s -> match s {
  Open -> "Open",
  Paid -> "Paid",
  Cancelled -> "Cancelled",
}
EOF

# the cost of a line, and the total of an order
alder new decl Line lineCost "Line -> Query Int"
alder fill Line.lineCost - <<'EOF'
\line -> {
  let found <- Db.get Product line.product
  Query.succeed (Maybe.withDefault 0 (Maybe.map (\p -> p.price * line.quantity) found))
}
EOF
alder new decl Line orderTotal "Order.OrderId -> Query Int"
alder fill Line.orderTotal - <<'EOF'
\order -> {
  let lines <- Db.from Line |> Db.where (Line.order.eq order) |> Db.all
  let costs <- Query.all (List.map lineCost lines)
  Query.succeed (List.sum costs)
}
EOF
alder new test Line "an order total sums its lines"
alder fill 'Line."an order total sums its lines"' - <<'EOF'
{
  let ann <- Db.insert User { email: Text.Email "ann@example.com", name: Text.NonEmpty "Ann", passwordHash: "" }
  let tea <- Db.insert Product { name: Text.NonEmpty "Tea", price: Int.NonNegative 250 }
  let cake <- Db.insert Product { name: Text.NonEmpty "Cake", price: Int.NonNegative 400 }
  let o <- Db.insert Order { owner: ann }
  let _ <- Db.insert Line { order: o, product: tea, quantity: Int.Positive 2 }
  let _ <- Db.insert Line { order: o, product: cake, quantity: Int.Positive 1 }
  let total <- Line.orderTotal o
  Task.succeed (total == 900)
}
EOF

# the orders list, at /
alder new page Orders.Index / --blank --requires user
alder fill Orders.Index.view - <<'EOF'
<Layout.Page title="Orders">
  <h1>Orders</h1>
</Layout.Page>
EOF
alder new type Orders.Index "Row = { order: Order.Order, total: Int }"
alder fill --decl Orders.Index.Model - <<'EOF'
{ rows: List Row }
EOF
alder new decl Orders.Index withTotal "Order.Order -> Query Row"
alder fill Orders.Index.withTotal - <<'EOF'
\o -> Query.map (\t -> { order: o, total: t }) (Line.orderTotal o.id)
EOF
alder fill Orders.Index.load - <<'EOF'
{
  let mine <- Db.from Order |> Db.where (Order.owner.eq session.user) |> Db.orderBy Order.created.desc |> Db.all
  let rows <- Query.all (List.map withTotal mine)
  Query.succeed { rows: rows }
}
EOF
alder new msg Orders.Index Create
alder fill --decl Orders.Index.view - <<'EOF'
\model session -> <Layout.Page title="Orders">
    <h1>Orders</h1>
    <form submit={Create}>
      <al-button type="submit">New order</al-button>
    </form>
    <ul id="orders">
      {for row in model.rows ->
        <li>
          <a href={Orders.Show { id: row.order.id }}>View</a>
          <span class="status">{Order.statusLabel row.order.status}</span>
          <span class="total">{Text.fromInt row.total}</span>
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

# one order, with its lines, at /orders/{id}
alder new page Orders.Show /orders/{id} --blank --param id:Order.OrderId --requires user
alder fill Orders.Show.view - <<'EOF'
<Layout.Page title="Order">
  <h1>Order</h1>
</Layout.Page>
EOF
alder new type Orders.Show "Row = { line: Line.Line, product: Maybe Product.Product, cost: Int }"
alder fill --decl Orders.Show.Model - <<'EOF'
{ order: Order.Order, lines: List Row, total: Int, products: List Product.Product }
EOF
alder new decl Orders.Show lineRow "Line.Line -> Query Row"
alder fill Orders.Show.lineRow - <<'EOF'
\line -> {
  let product <- Db.get Product line.product
  let cost <- Line.lineCost line
  Query.succeed { line: line, product: product, cost: cost }
}
EOF
alder new decl Orders.Show showModel "Order.Order -> Query Model"
alder fill Orders.Show.showModel - <<'EOF'
\order -> {
  let lines <- Db.from Line |> Db.where (Line.order.eq order.id) |> Db.all
  let rows <- Query.all (List.map lineRow lines)
  let products <- Db.from Product |> Db.where (Product.active.eq True) |> Db.orderBy Product.name.asc |> Db.all
  Query.succeed { order: order, lines: rows, total: List.sum (List.map (\r -> r.cost) rows), products: products }
}
EOF
alder fill Orders.Show.load - <<'EOF'
{
  let found <- Db.get Order params.id
  let me <- Db.get User session.user
  let manager = Maybe.map (\u -> u.role) me == Just Manager
  match found {
    Nothing -> Query.notFound,
    Just order -> if order.owner == session.user || manager then showModel order else Query.notFound,
  }
}
EOF
alder new msg Orders.Show Pay
alder new msg Orders.Show Cancel
alder new msg Orders.Show AddLine product:Product.ProductId
alder new form Orders.Show NewLine quantity:Int.Positive --msg AddLine
alder fill --decl Orders.Show.view - <<'EOF'
\model session -> <Layout.Page title="Order">
    <h1>Order</h1>
    <p id="status">{Order.statusLabel model.order.status}</p>
    <ul id="lines">
      {for row in model.lines ->
        <li>
          {match row.product {
            Just p -> <span>{p.name}</span>,
            Nothing -> Html.none,
          }}
          <span>{Text.fromInt row.line.quantity}</span>
          <span class="cost">{Text.fromInt row.cost}</span>
        </li>}
    </ul>
    <p id="total">{Text.fromInt model.total}</p>
    {if model.order.status == Open then
      <div>
        {for p in model.products ->
          <form submit={AddLine { product: p.id }}>
            <al-input field="quantity" label="Quantity" />
            <al-button type="submit">Add {p.name}</al-button>
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
    if model.order.status != Open then Task.fail "the order is not open" else {
      let _ <- Db.insert Line { order: model.order.id, product: p.product, quantity: p.form.quantity }
      Task.succeed (Redirect (Orders.Show { id: model.order.id }))
    }
  },
  Pay p -> {
    if model.order.status != Open then Task.fail "the order is not open" else {
      let _ <- Db.update Order model.order.id (\o -> { o | status: Paid })
      Task.succeed (Redirect (Orders.Show { id: model.order.id }))
    }
  },
  Cancel p -> {
    if model.order.status != Open then Task.fail "the order is not open" else {
      let _ <- Db.update Order model.order.id (\o -> { o | status: Cancelled })
      Task.succeed (Redirect (Orders.Show { id: model.order.id }))
    }
  },
}
EOF
alder new test Orders.Show "a line is refused on a paid order from a stale page"
alder fill 'Orders.Show."a line is refused on a paid order from a stale page"' - <<'EOF'
{
  let ann <- Db.insert User { email: Text.Email "ann@example.com", name: Text.NonEmpty "Ann", passwordHash: "" }
  let tea <- Db.insert Product { name: Text.NonEmpty "Tea", price: Int.NonNegative 250 }
  let o <- Db.insert Order { owner: ann }
  let _ <- Test.login { user: Just ann }
  let _ <- Test.get (Orders.Show { id: o })
  let _ <- Db.update Order o (\x -> { x | status: Paid })
  let r <- Test.submit (AddLine { product: tea }) { quantity: Int.Positive 1 }
  let lines <- Db.from Line |> Db.where (Line.order.eq o) |> Db.count
  Task.succeed ({ answer: r.answer, lines: lines } == { answer: RequestFailed "the order is not open", lines: 0 })
}
EOF
alder new test Orders.Show "a staff user is not found on another order"
alder fill 'Orders.Show."a staff user is not found on another order"' - <<'EOF'
{
  let ann <- Db.insert User { email: Text.Email "ann@example.com", name: Text.NonEmpty "Ann", passwordHash: "" }
  let bob <- Db.insert User { email: Text.Email "bob@example.com", name: Text.NonEmpty "Bob", passwordHash: "" }
  let o <- Db.insert Order { owner: ann }
  let _ <- Test.login { user: Just bob }
  let r <- Test.open (Orders.Show { id: o })
  Task.succeed (r.answer == PageMissing)
}
EOF
alder new test Orders.Show "a manager opens another order"
alder fill 'Orders.Show."a manager opens another order"' - <<'EOF'
{
  let ann <- Db.insert User { email: Text.Email "ann@example.com", name: Text.NonEmpty "Ann", passwordHash: "" }
  let boss <- Db.insert User { email: Text.Email "boss@example.com", name: Text.NonEmpty "Boss", passwordHash: "", role: Manager }
  let o <- Db.insert Order { owner: ann }
  let _ <- Test.login { user: Just boss }
  let r <- Test.open (Orders.Show { id: o })
  Task.succeed (r.answer == Shown)
}
EOF

# products, at /products (Managers only)
alder new page Products.Index /products --blank --requires user
alder fill Products.Index.view - <<'EOF'
<Layout.Page title="Products">
  <h1>Products</h1>
</Layout.Page>
EOF
alder fill --decl Products.Index.Model - <<'EOF'
{ products: List Product.Product }
EOF
alder fill Products.Index.load - <<'EOF'
{
  let me <- Db.get User session.user
  let manager = Maybe.map (\u -> u.role) me == Just Manager
  if manager then {
    let products <- Db.from Product |> Db.orderBy Product.name.asc |> Db.all
    Query.succeed { products: products }
  } else Query.notFound
}
EOF
alder new form Products.Index NewProduct name:Text.NonEmpty price:Int.NonNegative --msg Add
alder new msg Products.Index Toggle id:Product.ProductId
alder fill --decl Products.Index.view - <<'EOF'
\model session -> <Layout.Page title="Products">
    <h1>Products</h1>
    <form submit={Add}>
      <al-input field="name" label="Name" />
      <al-input field="price" label="Price (cents)" />
      <al-button type="submit">Add product</al-button>
    </form>
    <ul id="products">
      {for p in model.products ->
        <li>
          <span class="name">{p.name}</span>
          <span class="price">{Text.fromInt p.price}</span>
          <span class="active">{if p.active then "active" else "inactive"}</span>
          <al-button variant="outline" size="sm" click={Toggle { id: p.id }}>Toggle</al-button>
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

# after login, go to the orders list
alder fill Auth.Login.update.Submit - --hole ?afterLogin <<'EOF'
Orders.Index {}
EOF

# run the tests last
alder test
```
