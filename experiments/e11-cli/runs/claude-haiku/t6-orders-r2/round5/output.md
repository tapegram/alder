```alder-cli
# the orders page: the Create test checks the new order's owner and status
alder fill 'Orders.Index."pressing Create"' - <<'EOF'
{
  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
  let _ <- Test.login { user: Just me }
  let _ <- Test.get (Orders.Index {})
  let r <- Test.click Create
  let made <- Db.from Order |> Db.first
  Task.succeed ({ answer: Maybe.map (\o -> Redirected (Orders.Show { id: o.id })) made, owner: Maybe.map (\o -> o.owner) made, status: Maybe.map (\o -> o.status) made } == { answer: Just r.answer, owner: Just me, status: Just Open })
}
EOF

# the order page: AddLine refuses a paused order and a product that is not active
alder fill --decl Orders.Show.update - <<'EOF'
match msg {
  AddLine p -> {
    if model.status == Open then {
      if List.any (\c -> c.id == p.product) model.products then {
        let _ <- Db.insert Line { order: model.id, product: p.product, quantity: p.form.quantity }
        Task.succeed (Redirect (Orders.Show { id: model.id }))
      } else Task.fail "the product is not active"
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

alder fill 'Orders.Show."submitting NewLine"' - <<'EOF'
{
  let ann <- Db.insert User { email: Text.Email "ann@example.com", name: Text.NonEmpty "Ann", passwordHash: "" }
  let tea <- Db.insert Product { name: Text.NonEmpty "Tea", price: Int.NonNegative 250 }
  let o <- Db.insert Order { owner: ann }
  let _ <- Test.login { user: Just ann }
  let _ <- Test.get (Orders.Show { id: o })
  let r <- Test.submit (AddLine { product: tea }) { quantity: Int.Positive 1 }
  let lines <- Db.from Line |> Db.count
  Task.succeed ({ answer: r.answer, lines: lines } == { answer: Redirected (Orders.Show { id: o }), lines: 1 })
}
EOF

alder fill 'Orders.Show."pressing Pay"' - <<'EOF'
{
  let ann <- Db.insert User { email: Text.Email "ann@example.com", name: Text.NonEmpty "Ann", passwordHash: "" }
  let o <- Db.insert Order { owner: ann }
  let _ <- Test.login { user: Just ann }
  let _ <- Test.get (Orders.Show { id: o })
  let r <- Test.click Pay
  let row <- Db.get Order o
  Task.succeed ({ answer: r.answer, status: Maybe.map (\x -> x.status) row } == { answer: Redirected (Orders.Show { id: o }), status: Just Paid })
}
EOF

alder fill 'Orders.Show."pressing Cancel"' - <<'EOF'
{
  let ann <- Db.insert User { email: Text.Email "ann@example.com", name: Text.NonEmpty "Ann", passwordHash: "" }
  let o <- Db.insert Order { owner: ann }
  let _ <- Test.login { user: Just ann }
  let _ <- Test.get (Orders.Show { id: o })
  let r <- Test.click Cancel
  let row <- Db.get Order o
  Task.succeed ({ answer: r.answer, status: Maybe.map (\x -> x.status) row } == { answer: Redirected (Orders.Show { id: o }), status: Just Cancelled })
}
EOF

# the products page: the tests use a Manager, who is the only one the page admits
alder fill 'Products.Index."submitting NewProduct adds a row"' - <<'EOF'
{
  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "", role: Manager }
  let _ <- Test.login { user: Just me }
  let _ <- Test.get (Products.Index {})
  let r <- Test.submit NewProduct { name: Text.NonEmpty "Tea", price: Int.NonNegative 250 }
  let rows <- Db.from Product |> Db.all
  Task.succeed ({ answer: r.answer, rows: List.map (\row -> { name: row.name, price: row.price, active: row.active }) rows } == { answer: Redirected (Products.Index {}), rows: [{ name: Text.NonEmpty "Tea", price: Int.NonNegative 250, active: True }] })
}
EOF

alder fill 'Products.Index."NewProduct refuses an empty name"' - <<'EOF'
{
  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "", role: Manager }
  let _ <- Test.login { user: Just me }
  let _ <- Test.get (Products.Index {})
  let r <- Test.submitRaw NewProduct [{ name: "name", value: "" }, { name: "price", value: "1" }]
  let rows <- Db.from Product |> Db.count
  let rejected = match r.answer {
    FormRejected errs -> List.length errs > 0,
    _ -> False,
  }
  Task.succeed ({ rejected: rejected, rows: rows } == { rejected: True, rows: 0 })
}
EOF

alder fill 'Products.Index."pressing Toggle"' - <<'EOF'
{
  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "", role: Manager }
  let _ <- Test.login { user: Just me }
  let product1 <- Db.insert Product { name: Text.NonEmpty "Tea", price: Int.NonNegative 250 }
  let _ <- Test.get (Products.Index {})
  let r <- Test.click (Toggle { id: product1 })
  let after <- Db.get Product product1
  Task.succeed ({ answer: r.answer, active: Maybe.map (\p -> p.active) after } == { answer: Redirected (Products.Index {}), active: Just False })
}
EOF
```
