```alder-cli
# fix the order link in the list view
alder fill --decl Orders.Index.view - <<'EOF'
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
</Layout.Page>
EOF
alder fill Auth.Login - --hole ?afterLogin <<'EOF'
Orders.Index {}
EOF
alder fill 'Orders.Index."pressing Create"' - --hole ?expected <<'EOF'
Test.count "#orders li" after.doc == 1
EOF

# the products page
alder new page Products.Index /products --blank --requires user
alder fill --decl Products.Index.Model - <<'EOF'
{ products: List Product }
EOF
alder fill Products.Index.load - <<'EOF'
{
  let me <- Db.get User session.user
  if User.isManager me then {
    let ps <- Db.from Product |> Db.orderBy Product.name.asc |> Db.all
    Query.succeed { products: ps }
  } else Query.notFound
}
EOF
alder fill Products.Index.view - <<'EOF'
<Layout.Page title="Products">
  <h1>Products</h1>
  <ul id="products">
    {for p in model.products -> <li><span class="name">{p.name}</span></li>}
  </ul>
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
  <form submit={Add}>
    <Errors form />
    <Input field="name" label="Name" />
    <Input field="price" label="Price" />
    <button>Add</button>
  </form>
  <ul id="products">
    {for p in model.products ->
      <li>
        <span class="name">{p.name}</span>
        <span class="price">{Text.fromInt p.price}</span>
        <span class="active">{if p.active then "active" else "inactive"}</span>
        <button click={Toggle { id: p.id }}>Toggle</button>
      </li>}
  </ul>
</Layout.Page>
EOF
alder fill --list
```
