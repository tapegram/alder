```alder-cli
# the login goes to the order list
alder fill Auth.Login.update.Submit --hole ?afterLogin - <<'EOF'
Orders.Index {}
EOF
alder fill 'Auth.Login."logging in through the form"' --hole ?afterLogin - <<'EOF'
Orders.Index {}
EOF

# Orders.Index load
alder fill Orders.Index.load - <<'EOF'
{
  let orders <- Db.from Order |> Db.where (Order.owner.eq session.user) |> Db.orderBy Order.created.desc |> Db.all
  let rows <- Query.all (List.map (\o -> Query.map (\t -> { id: o.id, status: o.status, total: t }) (Line.total o.id)) orders)
  Query.succeed { orders: rows }
}
EOF
alder fill Orders.Index.view - <<'EOF'
Html.none
EOF
alder new msg Orders.Index Create

# Orders.Show
alder new page Orders.Show '/orders/{id}' --blank --param id:Order.OrderId --requires user
alder fill --decl Orders.Show.Model - <<'EOF'
{ order: Order, lines: List { name: Text, quantity: Int, subtotal: Int }, total: Int, products: List Product }
EOF
alder fill Orders.Show.view - <<'EOF'
Html.none
EOF
alder fill Orders.Show.load - <<'EOF'
{
  let manager <- User.isManager session.user
  let found <- Db.get Order params.id
  match found {
    Nothing -> Query.notFound,
    Just o -> if o.owner != session.user && not manager then Query.notFound else {
      let lines <- Db.from Line |> Db.where (Line.order.eq o.id) |> Db.all
      let prods <- Db.from Product |> Db.all
      let total <- Line.total o.id
      Query.succeed {
        order: o,
        lines: List.map (\l -> { name: Maybe.withDefault "?" (Maybe.map (\p -> Text.trim p.name) (List.find (\p -> p.id == l.product) prods)), quantity: l.quantity, subtotal: Line.priceOf prods l }) lines,
        total: total,
        products: List.filter (\p -> p.active) prods,
      }
    },
  }
}
EOF
alder new msg Orders.Show AddLine product:Product.ProductId
alder new form Orders.Show NewLine quantity:Int.Positive --msg AddLine
alder new msg Orders.Show Pay
alder new msg Orders.Show Cancel

alder fill Orders.Show.update.AddLine - <<'EOF'
if model.order.status != Open then Task.succeed (Reject "This order is not open.") else {
  let _ <- Db.insert Line { order: model.order.id, product: p.product, quantity: p.form.quantity }
  Task.succeed (Redirect (Orders.Show { id: model.order.id }))
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
alder fill --decl Orders.Show.view - <<'EOF'
<Layout.Page title="Order">
  <h1>Order</h1>
  <p>Status: <span id="status">{Order.statusText model.order.status}</span></p>
  <p>Total: <span id="total">{Text.fromInt model.total}</span></p>
  <ul id="lines">
    {for l in model.lines ->
      <li>{l.name} x {Text.fromInt l.quantity} = {Text.fromInt l.subtotal}</li>}
  </ul>
  {if model.order.status == Open then
    <div>
      {for p in model.products ->
        <form submit={AddLine { product: p.id }}>
          <Errors form />
          <al-input field="quantity" label={"Quantity of " ++ Text.trim p.name} />
          <al-button type="submit">Add</al-button>
        </form>}
      <al-button click={Pay}>Pay</al-button>
      <al-button click={Cancel}>Cancel</al-button>
    </div>
  else Html.none}
  <a href={Orders.Index {}}>Back</a>
</Layout.Page>
EOF

# Orders.Index create and view
alder fill Orders.Index.update.Create - <<'EOF'
{
  let id <- Db.insert Order { owner: session.user }
  Task.succeed (Redirect (Orders.Show { id: id }))
}
EOF
alder fill --decl Orders.Index.view - <<'EOF'
<Layout.Page title="Orders">
  <h1>Orders</h1>
  <al-button click={Create}>New order</al-button>
  <ul id="orders">
    {for o in model.orders ->
      <li>
        <a href={Orders.Show { id: o.id }}>Order</a>
        <span class="status">{Order.statusText o.status}</span>
        <span class="total">{Text.fromInt o.total}</span>
      </li>}
  </ul>
  <a href={Products.Index {}}>Products</a>
  <al-button variant="outline" click={App.Logout}>Log out</al-button>
</Layout.Page>
EOF

# Products
alder new page Products.Index /products --blank --requires user
alder fill --decl Products.Index.Model - <<'EOF'
{ products: List Product }
EOF
alder fill Products.Index.view - <<'EOF'
Html.none
EOF
alder fill Products.Index.load - <<'EOF'
{
  let manager <- User.isManager session.user
  if not manager then Query.notFound else {
    let prods <- Db.from Product |> Db.orderBy Product.name.asc |> Db.all
    Query.succeed { products: prods }
  }
}
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
alder fill --decl Products.Index.view - <<'EOF'
<Layout.Page title="Products">
  <h1>Products</h1>
  <ul id="products">
    {for p in model.products ->
      <li>
        <span class="name">{p.name}</span>
        <span class="price">{Text.fromInt p.price}</span>
        <span class="active">{if p.active then "active" else "inactive"}</span>
        <al-button click={Toggle { id: p.id }}>Toggle</al-button>
      </li>}
  </ul>
  <form submit={Add}>
    <Errors form />
    <al-input field="name" label="Name" />
    <al-input field="price" label="Price in cents" />
    <al-button type="submit">Add</al-button>
  </form>
  <a href={Orders.Index {}}>Orders</a>
</Layout.Page>
EOF

# tests
alder new test Orders.Show "an Open order shows its total over two lines"
alder fill 'Orders.Show."an Open order shows its total over two lines"' - <<'EOF'
{
  let u <- Db.insert User { email: Text.Email "a@example.com", name: Text.NonEmpty "A", passwordHash: "" }
  let p1 <- Db.insert Product { name: Text.NonEmpty "Pen", price: Int.NonNegative 250 }
  let p2 <- Db.insert Product { name: Text.NonEmpty "Cup", price: Int.NonNegative 400 }
  let o <- Db.insert Order { owner: u }
  let _ <- Db.insert Line { order: o, product: p1, quantity: Int.Positive 2 }
  let _ <- Db.insert Line { order: o, product: p2, quantity: Int.Positive 1 }
  let _ <- Test.login { user: Just u }
  let page <- Test.get (Orders.Show { id: o })
  Task.succeed ({ total: Test.text "#total" page.doc, status: Test.text "#status" page.doc } == { total: Just "900", status: Just "Open" })
}
EOF
alder new test Orders.Show "adding a line to a Paid order is refused"
alder fill 'Orders.Show."adding a line to a Paid order is refused"' - <<'EOF'
{
  let u <- Db.insert User { email: Text.Email "a@example.com", name: Text.NonEmpty "A", passwordHash: "" }
  let p <- Db.insert Product { name: Text.NonEmpty "Pen", price: Int.NonNegative 250 }
  let o <- Db.insert Order { owner: u }
  let _ <- Test.login { user: Just u }
  let _ <- Test.get (Orders.Show { id: o })
  let _ <- Db.update Order o (\x -> { x | status: Paid })
  let r <- Test.submit (AddLine { product: p }) { quantity: Int.Positive 1 }
  let n <- Db.from Line |> Db.count
  Task.succeed ({ answer: r.answer, lines: n } == { answer: Rerendered, lines: 0 })
}
EOF
alder new test Orders.Show "a Staff user opening another user's order is not found"
alder fill 'Orders.Show."a Staff user opening another user'"'"'s order is not found"' - <<'EOF'
{
  let u1 <- Db.insert User { email: Text.Email "a@example.com", name: Text.NonEmpty "A", passwordHash: "" }
  let u2 <- Db.insert User { email: Text.Email "b@example.com", name: Text.NonEmpty "B", passwordHash: "" }
  let o <- Db.insert Order { owner: u1 }
  let _ <- Test.login { user: Just u2 }
  let r <- Test.open (Orders.Show { id: o })
  Task.succeed (r.answer == PageMissing)
}
EOF
alder new test Orders.Show "a Manager opening another user's order sees it"
alder fill 'Orders.Show."a Manager opening another user'"'"'s order sees it"' - <<'EOF'
{
  let u1 <- Db.insert User { email: Text.Email "a@example.com", name: Text.NonEmpty "A", passwordHash: "" }
  let u2 <- Db.insert User { email: Text.Email "b@example.com", name: Text.NonEmpty "B", passwordHash: "", role: Manager }
  let o <- Db.insert Order { owner: u1 }
  let _ <- Test.login { user: Just u2 }
  let r <- Test.open (Orders.Show { id: o })
  Task.succeed (r.answer == Shown)
}
EOF
alder test
```
