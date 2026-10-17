```alder-cli
# 1. Log in goes to the order list (the page is created below; a fill may name a page by its value)
alder fill Auth.Login.update.Submit - --hole ?afterLogin <<'EOF'
Orders.Index {}
EOF
alder fill 'Auth.Login."logging in through the form"' - --hole ?afterLogin <<'EOF'
Orders.Index {}
EOF

# 2. The order list: a Model, a load, and a view
alder fill --decl Orders.Index.Model - <<'EOF'
{ rows: List { id: Order.OrderId, status: Order.Status, total: Int } }
EOF
alder fill Orders.Index.load - --hole ?load <<'EOF'
{
  let orders <- Db.from Order |> Db.where (Order.owner.eq session.user) |> Db.orderBy Order.created.desc |> Db.all
  let rows <- Query.all (List.map (\o -> {
    let total <- Line.orderTotal o.id
    Query.succeed { id: o.id, status: o.status, total: total }
  }) orders)
  Query.succeed { rows: rows }
}
EOF
alder fill Orders.Index.view - --hole ?view <<'EOF'
<Layout.Page title="Orders">
  <h1>Orders</h1>
</Layout.Page>
EOF
alder new msg Orders.Index Create

# 3. The order page exists before the list's update refers to it
alder new page Orders.Show '/orders/{id}' --blank --show Order --requires user
alder fill --decl Orders.Index.update - <<'EOF'
match msg {
  Create p -> {
    let id <- Db.insert Order { owner: session.user }
    Task.succeed (Redirect (Orders.Show { id: id }))
  },
}
EOF
alder fill --decl Orders.Index.view - <<'EOF'
<Layout.Page title="Orders">
  <h1>Orders</h1>
  <form submit={Create}>
    <al-button type="submit">New order</al-button>
  </form>
  <ul id="orders">
    {for row in model.rows ->
      <li>
        <a href={Orders.Show { id: row.id }}>Open order</a>
        <span class="status">{Order.statusLabel row.status}</span>
        <span class="total">{Text.fromInt row.total}</span>
      </li>}
  </ul>
  <al-button variant="outline" click={App.Logout}>Log out</al-button>
</Layout.Page>
EOF

# 4. One order: its model, load, and the messages
alder fill --decl Orders.Show.Model - <<'EOF'
{ id: Order.OrderId, status: Order.Status, lines: List Line.LineRow, total: Int, products: List Line.Choice }
EOF
alder fill Orders.Show.load - --hole ?load <<'EOF'
{
  let found <- Db.get Order params.id
  let mgr <- User.isManager session.user
  let mine = Maybe.withDefault False (Maybe.map (\o -> o.owner == session.user) found)
  let status = Maybe.withDefault Open (Maybe.map (\o -> o.status) found)
  if Maybe.isJust found && (mine || mgr) then {
    let lines <- Line.orderLines params.id
    let total <- Line.orderTotal params.id
    let products <- Line.choices
    Query.succeed { id: params.id, status: status, lines: lines, total: total, products: products }
  } else Query.notFound
}
EOF
alder fill Orders.Show.view - --hole ?view <<'EOF'
<Layout.Page title="Order">
  <h1>Order</h1>
</Layout.Page>
EOF
alder new msg Orders.Show AddLine product:Product.ProductId
alder new form Orders.Show NewLine quantity:Int.Positive --msg AddLine
alder new msg Orders.Show Pay
alder new msg Orders.Show Cancel
alder fill --decl Orders.Show.update - <<'EOF'
match msg {
  AddLine p -> {
    if model.status == Open then {
      let _ <- Db.insert Line { order: model.id, product: p.product, quantity: p.form.quantity }
      Task.succeed (Redirect (Orders.Show { id: model.id }))
    } else Task.fail "the order is not open"
  },
  Pay p -> {
    if model.status == Open then {
      let _ <- Db.update Order model.id (\o -> { o | status: Paid })
      Task.succeed (Redirect (Orders.Show { id: model.id }))
    } else Task.fail "the order is not open"
  },
  Cancel p -> {
    if model.status == Open then {
      let _ <- Db.update Order model.id (\o -> { o | status: Cancelled })
      Task.succeed (Redirect (Orders.Show { id: model.id }))
    } else Task.fail "the order is not open"
  },
}
EOF
alder fill --decl Orders.Show.view - <<'EOF'
<Layout.Page title="Order">
  <h1>Order</h1>
  <p>Status: <span id="status">{Order.statusLabel model.status}</span></p>
  <ul id="lines">
    {for l in model.lines ->
      <li>{l.name} x {Text.fromInt l.quantity} = {Text.fromInt l.cost}</li>}
  </ul>
  <p>Total: <span id="total">{Text.fromInt model.total}</span></p>
  {if model.status == Open then
    <div>
      {for p in model.products ->
        <form submit={AddLine { product: p.id }}>
          <al-input field="quantity" label="Quantity" />
          <al-button type="submit">Add {p.name}</al-button>
        </form>}
      <al-button click={Pay}>Mark paid</al-button>
      <al-button variant="destructive" click={Cancel}>Cancel order</al-button>
    </div> else Html.none}
  <a href={Orders.Index {}}>Back to orders</a>
</Layout.Page>
EOF

# 5. Products, for managers only
alder new page Products.Index '/products' --blank --requires user
alder fill --decl Products.Index.Model - <<'EOF'
{ products: List { id: Product.ProductId, name: Text.NonEmpty, price: Int.NonNegative, active: Bool } }
EOF
alder fill Products.Index.load - --hole ?load <<'EOF'
{
  let mgr <- User.isManager session.user
  if mgr then {
    let products <- Db.from Product |> Db.orderBy Product.name.asc |> Db.all
    Query.succeed { products: List.map (\p -> { id: p.id, name: p.name, price: p.price, active: p.active }) products }
  } else Query.notFound
}
EOF
alder fill Products.Index.view - --hole ?view <<'EOF'
<Layout.Page title="Products">
  <h1>Products</h1>
</Layout.Page>
EOF
alder new form Products.Index NewProduct name:Text.NonEmpty price:Int.NonNegative --msg Add
alder new msg Products.Index Toggle id:Product.ProductId
alder fill --decl Products.Index.update - <<'EOF'
match msg {
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
alder fill --decl Products.Index.view - <<'EOF'
<Layout.Page title="Products">
  <h1>Products</h1>
  <al-card>
    <form submit={Add}>
      <Errors form />
      <al-input field="name" label="Name" />
      <al-input field="price" label="Price in cents" />
      <al-button type="submit">Add</al-button>
    </form>
  </al-card>
  <ul id="products">
    {for p in model.products ->
      <li>
        <span class="name">{p.name}</span>
        <span class="price">{Text.fromInt p.price}</span>
        <span class="active">{if p.active then "active" else "inactive"}</span>
        <al-button variant="secondary" size="sm" click={Toggle { id: p.id }}>{if p.active then "Deactivate" else "Activate"}</al-button>
      </li>}
  </ul>
</Layout.Page>
EOF

# 6. Tests in the modules they test
alder new test Orders.Show "adding a line to a Paid order is refused"
alder fill 'Orders.Show."adding a line to a Paid order is refused"' - <<'EOF'
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
alder new test Orders.Show "a Staff user is not found on another order"
alder fill 'Orders.Show."a Staff user is not found on another order"' - <<'EOF'
{
  let ann <- Db.insert User { email: Text.Email "ann@example.com", name: Text.NonEmpty "Ann", passwordHash: "" }
  let bob <- Db.insert User { email: Text.Email "bob@example.com", name: Text.NonEmpty "Bob", passwordHash: "" }
  let o <- Db.insert Order { owner: ann }
  let _ <- Test.login { user: Just bob }
  let r <- Test.open (Orders.Show { id: o })
  Task.succeed (r.answer == PageMissing)
}
EOF
alder new test Orders.Show "a Manager sees another order"
alder fill 'Orders.Show."a Manager sees another order"' - <<'EOF'
{
  let ann <- Db.insert User { email: Text.Email "ann@example.com", name: Text.NonEmpty "Ann", passwordHash: "" }
  let boss <- Db.insert User { email: Text.Email "boss@example.com", name: Text.NonEmpty "Boss", passwordHash: "", role: Manager }
  let o <- Db.insert Order { owner: ann }
  let _ <- Test.login { user: Just boss }
  let r <- Test.open (Orders.Show { id: o })
  Task.succeed (r.answer == Shown)
}
EOF
alder new test Products.Index "a Staff user gets not found on products"
alder fill 'Products.Index."a Staff user gets not found on products"' - <<'EOF'
{
  let ann <- Db.insert User { email: Text.Email "ann@example.com", name: Text.NonEmpty "Ann", passwordHash: "" }
  let _ <- Test.login { user: Just ann }
  let r <- Test.open (Products.Index {})
  Task.succeed (r.answer == PageMissing)
}
EOF
```
