```alder-cli
# the order page, so the list can link to it
alder new page Orders.Show '/orders/{id}' --blank --requires user --param id:Order.OrderId
alder fill --decl Orders.Show.Model - <<'EOF'
{ order: Order, lines: List { name: Text, quantity: Int, subtotal: Int }, products: List Product, total: Int }
EOF
alder fill Orders.Show.load - <<'EOF'
{
  let found <- Db.get Order params.id
  match found {
    Nothing -> Query.notFound,
    Just o -> {
      let mgr <- User.isManager session.user
      if o.owner == session.user || mgr then {
        let lines <- Db.from Line |> Db.where (Line.order.eq o.id) |> Db.all
        let prods <- Db.from Product |> Db.all
        let total <- Line.total o.id
        Query.succeed { order: o, lines: List.map (\l -> { name: Maybe.withDefault "" (Maybe.map (\p -> Text.trim p.name) (List.find (\p -> p.id == l.product) prods)), quantity: l.quantity, subtotal: Line.priceOf prods l }) lines, products: List.filter (\p -> p.active) prods, total: total }
      } else Query.notFound
    },
  }
}
EOF
alder fill Orders.Show.view - <<'EOF'
<Layout.Page title="Order">
  <h1>Order</h1>
</Layout.Page>
EOF

# the list page
alder fill --decl Orders.Index.view - <<'EOF'
<Layout.Page title="Orders">
  <h1>Orders</h1>
  <ul id="orders">
    {for o in model.orders ->
      <li>
        <a href={Orders.Show { id: o.id }}>{o.id}</a>
        <span class="status">{Order.statusText o.status}</span>
        <span class="total">{Text.fromInt o.total}</span>
      </li>}
  </ul>
  <al-button variant="outline" click={App.Logout}>Log out</al-button>
</Layout.Page>
EOF
alder new msg Orders.Index Create
alder new msg Orders.Show AddLine product:Product.ProductId
alder new form Orders.Show NewLine quantity:Int.Positive --msg AddLine
alder new msg Orders.Show Pay
alder new msg Orders.Show Cancel
alder fill --list
```
