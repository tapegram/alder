The Show page has to be regenerated: `msg AddLine` was created without its form, and no command can add a form to an existing message. The form now carries the product as a select field, not as a message parameter, so the spec's `msg AddLine { product } form NewLine` shape isn't reachable with these commands. Some generated test names are guesses, so the script may stop there and I'll correct them from the transcript.

```alder-cli
# the login test that needs the page value
alder fill 'Auth.Login."logging in through the form"' - --hole ?afterLogin <<'EOF'
Orders.Index {}
EOF

# orders: the list page (its Model and load are set; the view and Create follow)
alder fill Orders.Index.load - --hole ?load <<'EOF'
{
  let orders <- Db.from Order |> Db.where (Order.owner.eq session.user) |> Db.orderBy Order.created.desc |> Db.all
  let lines <- Db.from Line |> Db.all
  let products <- Db.from Product |> Db.all
  Query.succeed { orders: orders, lines: lines, products: products }
}
EOF

# the order page: regenerated so its AddLine message carries its form
alder remove page Orders.Show
alder new page Orders.Show "/orders/{id}" --param id:Order.OrderId --requires user --blank
alder fill --decl Orders.Show.Model - <<'EOF'
{ order: Order, lines: List Line, products: List Product }
EOF
alder fill Orders.Show.load - --hole ?load <<'EOF'
{
  let mgr <- User.isManager session.user
  let found <- Db.get Order params.id
  match found {
    Nothing -> Query.notFound,
    Just o -> if o.owner == session.user || mgr then {
      let lines <- Db.from Line |> Db.where (Line.order.eq (o.id)) |> Db.all
      let products <- Db.from Product |> Db.all
      Query.succeed { order: o, lines: lines, products: products }
    } else Query.notFound,
  }
}
EOF
alder fill Orders.Show.view - --hole ?view <<'EOF'
<Layout.Page title="Order">
    <h1>Order</h1>
  </Layout.Page>
EOF
alder new form Orders.Show NewLine product:Product.ProductId quantity:Int.Positive --msg AddLine
alder new msg Orders.Show Pay
alder new msg Orders.Show Cancel

# the order page's view and update, written whole
alder fill --decl Orders.Show.view - <<'EOF'
<Layout.Page title="Order">
    <h1>Order</h1>
    <p id="status">{Order.statusText model.order.status}</p>
    <p id="total">{Text.fromInt (Line.orderTotal model.products model.lines)}</p>
    <ul id="lines">
      {for l in model.lines -> <li>{Text.fromInt l.quantity} × {Text.fromInt (Line.lineCost model.products l)}</li>}
    </ul>
    {if model.order.status == Open then
      <al-card>
        <form submit={AddLine}>
          <Errors form />
          <Select field="product" label="Product" options={List.map (\p -> { value: p.id, label: p.name }) (List.filter (\p -> p.active) model.products)} />
          <Input field="quantity" label="Quantity" />
          <al-button type="submit">Add line</al-button>
        </form>
        <al-button click={Pay}>Pay</al-button>
        <al-button click={Cancel}>Cancel</al-button>
      </al-card>
    else Html.none}
  </Layout.Page>
EOF
alder fill --decl Orders.Show.update - <<'EOF'
match msg {
  Pay p -> if model.order.status == Open then {
    let _ <- Db.update Order model.order.id (\o -> { o | status: Paid })
    Task.succeed (Redirect (Orders.Show { id: model.order.id }))
  } else Task.succeed Rerender,
  Cancel p -> if model.order.status == Open then {
    let _ <- Db.update Order model.order.id (\o -> { o | status: Cancelled })
    Task.succeed (Redirect (Orders.Show { id: model.order.id }))
  } else Task.succeed Rerender,
  AddLine p -> if model.order.status == Open then {
    let _ <- Db.insert Line { order: model.order.id, product: p.form.product, quantity: p.form.quantity }
    Task.succeed (Redirect (Orders.Show { id: model.order.id }))
  } else Task.succeed (Reject "This order is no longer open."),
}
EOF

# the order page's generated tests for the buttons
alder fill 'Orders.Show."pressing Pay"' - <<'EOF'
{
  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
  let _ <- Test.login { user: Just me }
  let order1 <- Db.insert Order { owner: me }
  let _ <- Test.get (Orders.Show { id: order1 })
  let r <- Test.click Pay
  let row <- Db.get Order order1
  Task.succeed ({ answer: r.answer, status: Maybe.map (\x -> x.status) row } == { answer: Redirected (Orders.Show { id: order1 }), status: Just Paid })
}
EOF
alder fill 'Orders.Show."pressing Cancel"' - <<'EOF'
{
  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
  let _ <- Test.login { user: Just me }
  let order1 <- Db.insert Order { owner: me }
  let _ <- Test.get (Orders.Show { id: order1 })
  let r <- Test.click Cancel
  let row <- Db.get Order order1
  Task.succeed ({ answer: r.answer, status: Maybe.map (\x -> x.status) row } == { answer: Redirected (Orders.Show { id: order1 }), status: Just Cancelled })
}
EOF

# the order page's own tests
alder new test Orders.Show "adding a line to an open order"
alder fill 'Orders.Show."adding a line to an open order"' - <<'EOF'
{
  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
  let tea <- Db.insert Product { name: Text.NonEmpty "Tea", price: Int.NonNegative 250 }
  let o <- Db.insert Order { owner: me }
  let _ <- Test.login { user: Just me }
  let _ <- Test.get (Orders.Show { id: o })
  let r <- Test.submit NewLine { product: tea, quantity: Int.Positive 2 }
  let n <- Db.from Line |> Db.count
  Task.succeed ({ answer: r.answer, lines: n } == { answer: Redirected (Orders.Show { id: o }), lines: 1 })
}
EOF
alder new test Orders.Show "adding a line to a Paid order is refused"
alder fill 'Orders.Show."adding a line to a Paid order is refused"' - <<'EOF'
{
  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
  let tea <- Db.insert Product { name: Text.NonEmpty "Tea", price: Int.NonNegative 250 }
  let o <- Db.insert Order { owner: me }
  let _ <- Test.login { user: Just me }
  let _ <- Test.get (Orders.Show { id: o })
  let _ <- Db.update Order o (\x -> { x | status: Paid })
  let _ <- Test.submit NewLine { product: tea, quantity: Int.Positive 2 }
  let n <- Db.from Line |> Db.count
  let row <- Db.get Order o
  Task.succeed ({ lines: n, status: Maybe.map (\x -> x.status) row } == { lines: 0, status: Just Paid })
}
EOF
alder new test Orders.Show "a staff user cannot open another user's order"
alder fill 'Orders.Show."a staff user cannot open another user\'s order"' - <<'EOF'
{
  let owner <- Db.insert User { email: Text.Email "owner@example.com", name: Text.NonEmpty "Owner", passwordHash: "" }
  let staff <- Db.insert User { email: Text.Email "staff@example.com", name: Text.NonEmpty "Staff", passwordHash: "" }
  let o <- Db.insert Order { owner: owner }
  let _ <- Test.login { user: Just staff }
  let r <- Test.open (Orders.Show { id: o })
  Task.succeed (r.answer == PageMissing)
}
EOF
alder new test Orders.Show "a manager can open another user's order"
alder fill 'Orders.Show."a manager can open another user\'s order"' - <<'EOF'
{
  let owner <- Db.insert User { email: Text.Email "owner@example.com", name: Text.NonEmpty "Owner", passwordHash: "" }
  let boss <- Db.insert User { email: Text.Email "boss@example.com", name: Text.NonEmpty "Boss", passwordHash: "", role: Manager }
  let o <- Db.insert Order { owner: owner }
  let _ <- Test.login { user: Just boss }
  let r <- Test.open (Orders.Show { id: o })
  Task.succeed ({ answer: r.answer, status: Test.text "#status" r.doc } == { answer: Shown, status: Just "Open" })
}
EOF

# the list page: its create message, and the view
alder fill Orders.Index.view - --hole ?view <<'EOF'
<Layout.Page title="Orders">
    <h1>Orders</h1>
  </Layout.Page>
EOF
alder new msg Orders.Index Create
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
    <al-button click={Create}>New order</al-button>
    <ul id="orders">
      {for o in model.orders ->
        <li>
          <a href={Orders.Show { id: o.id }}>Open</a>
          <span class="status">{Order.statusText o.status}</span>
          <span class="total">{Text.fromInt (Line.orderTotal model.products (List.filter (\l -> l.order == o.id) model.lines))}</span>
        </li>}
    </ul>
  </Layout.Page>
EOF

# products: a manager's page, nothing for staff
alder new page Products.Index "/products" --requires user --blank
alder fill --decl Products.Index.Model - <<'EOF'
{ products: List Product }
EOF
alder fill Products.Index.load - --hole ?load <<'EOF'
{
  let mgr <- User.isManager session.user
  if mgr then {
    let products <- Db.from Product |> Db.orderBy Product.name.asc |> Db.all
    Query.succeed { products: products }
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
        <al-input field="price" label="Price (cents)" />
        <al-button type="submit">Add</al-button>
      </form>
    </al-card>
    <ul id="products">
      {for p in model.products ->
        <li>
          <span class="name">{p.name}</span>
          <span>{Text.fromInt p.price}</span>
          <span>{if p.active then "active" else "inactive"}</span>
          <al-button click={Toggle { id: p.id }}>Toggle</al-button>
        </li>}
    </ul>
  </Layout.Page>
EOF
alder new test Products.Index "a staff user gets 404"
alder fill 'Products.Index."a staff user gets 404"' - <<'EOF'
{
  let staff <- Db.insert User { email: Text.Email "staff@example.com", name: Text.NonEmpty "Staff", passwordHash: "" }
  let _ <- Test.login { user: Just staff }
  let r <- Test.open (Products.Index {})
  Task.succeed (r.answer == PageMissing)
}
EOF
```
