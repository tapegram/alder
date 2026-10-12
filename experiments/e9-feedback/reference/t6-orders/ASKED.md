# Tests asked only: t6-orders

The calibration line for H1 (`findings.md` reads the baseline scores against it): the
reference's app code with only the tests the task's tests paragraph names, scored by
`alder test --mutate`.

The paragraph:

> Also write tests (`test "…" = …`, chapter 10) in the modules they test: an
> Open order's total over two lines; adding a line to a Paid order is
> refused; a Staff user opening another user's order is not found; a Manager
> opening it is. `alder test` must pass. Do not write tests that call the
> `Password` capability.

## How to reproduce

1. Copy `reference/t6-orders/` to a scratch directory (drop this file and `SURVIVORS.md`).
2. Delete every `test` declaration in `app/` except the ones below, and add the ones below that are missing (same module, same text).
3. Run `alder test --mutate .` there.

## Score: 8/46 = 0.174

| Class | Survivors |
|---|---|
| authorization | 5 |
| data | 12 |
| flow | 11 |
| logic | 5 |
| schema | 5 |

By operator: redirect → rerender 7, write replaced 5, cascade ↔ restrict 3, arm swapped 3, where dropped 3, requires dropped 3, bool flipped 2, session put dropped 2, default changed 2, orderBy reversed 2, guard dropped 2, condition negated 2, reject → rerender 1, not dropped 1.

```text
3 passed, 0 failed
46 mutants: 8 killed, 38 survived  (score 0.17)
survived:
  User             isManager:22     bool flipped          False → True
  App              update:15        session put dropped   let _ <- Session.put (\s -> { s | user: Nothing }) → let _ <- Task.succeed ()
  App              update:16        redirect → rerender   Redirect (Auth.Login {}) → Rerender
  Auth.Login       update:40        session put dropped   let _ <- Session.put (\s -> { s | user: Just id }) → let _ <- Task.succeed ()
  Auth.Login       update:41        redirect → rerender   Redirect (Orders.Index {}) → Rerender
  Auth.Login       update:43        reject → rerender     Reject "Email or password is incorrect." → Rerender
  Order            Order:11         cascade ↔ restrict    owner @ref User @restrict → owner @ref User @cascade
  Order            Order:12         default changed       status = Open → status = Paid
  Order            statusLabel:17   arm swapped           match status { Open -> "Op… → the Open and Paid bodies swapped
  Order            statusLabel:17   arm swapped           match status { Open -… → the Paid and Cancelled bodies swapped
  Product          Product:9        default changed       active = True → active = False
  Line             Line:12          cascade ↔ restrict    order @ref Order @cascade → order @ref Order @restrict
  Line             Line:13          cascade ↔ restrict    product @ref Product @restrict → product @ref Product @cascade
  Line             priced:34        where dropped         Db.where (Line.order.eq order) → the condition dropped
  Orders.Index     requires:8       requires dropped      requires user → served without user (user = User.UserId "mutant")
  Orders.Index     load:18          where dropped         Db.where (Order.owner.eq session.user) → the condition dropped
  Orders.Index     load:19          orderBy reversed      Db.orderBy Order.created.desc → Db.orderBy Order.created.asc
  Orders.Index     update:36        write replaced        let id <- Db.insert Order { owner: session.user } → let id <- Task.succeed (Order.OrderId "mutant")
  Orders.Index     update:37        redirect → rerender   Redirect (Orders.Show { id: id }) → Rerender
  Orders.Show      requires:9       requires dropped      requires user → served without user (user = User.UserId "mutant")
  Orders.Show      load:27          where dropped         Db.where (Product.active.eq True) → the condition dropped
  Orders.Show      load:27          bool flipped          True → False
  Orders.Show      update:45        arm swapped           match msg { AddLine _… → the Pay _ and Cancel _ bodies swapped
  Orders.Show      update:51        guard dropped         if product.active then … else Task.notFound → the notFound arm dropped
  Orders.Show      update:51        condition negated     if product.active → if not product.active
  Orders.Show      update:52        write replaced        let _ <- Db.insert Line {…} → let _ <- Task.succeed (Line.LineId "mutant")
  Orders.Show      update:53        redirect → rerender   Redirect (Orders.Show { id: order.id }) → Rerender
  Orders.Show      setStatus:71     write replaced        let _ <- Db.update Order open.id (\o -> { o | status: status }) → let _ <- Task.succeed ()
  Orders.Show      setStatus:72     redirect → rerender   Redirect (Orders.Show { id: open.id }) → Rerender
  Products.Index   requires:7       requires dropped      requires user → served without user (user = User.UserId "mutant")
  Products.Index   load:16          guard dropped         if manager then … else Query.notFound → the notFound arm dropped
  Products.Index   load:16          condition negated     if manager → if not manager
  Products.Index   load:17          orderBy reversed      Db.orderBy Product.name.asc → Db.orderBy Product.name.desc
  Products.Index   update:33        write replaced        let _ <- Db.insert Product { name: p.form.name, price: p.form.price } → let _ <- Task.succeed (Product.ProductId "mutant")
  Products.Index   update:34        redirect → rerender   Redirect (Products.Index {}) → Rerender
  Products.Index   update:40        write replaced        let _ <- Db.update Product product.id (\x -> { x | active: not x.active }) → let _ <- Task.succeed ()
  Products.Index   update:40        not dropped           not x.active → x.active
  Products.Index   update:41        redirect → rerender   Redirect (Products.Index {}) → Rerender
```

## The tests

`app/Line.alder`:

```alder
test "an Open order's total over two lines" = {
  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
  let pen <- Db.insert Product { name: Text.NonEmpty "Pen", price: Int.NonNegative 150 }
  let ink <- Db.insert Product { name: Text.NonEmpty "Ink", price: Int.NonNegative 400 }
  let order <- Db.insert Order { owner: me }
  let _ <- Db.insert Line { order: order, product: pen, quantity: Int.Positive 3 }
  let _ <- Db.insert Line { order: order, product: ink, quantity: Int.Positive 2 }
  let lines <- priced order
  Task.succeed (total lines == 1250)
```

`app/Orders/Show.alder`:

```alder
test "adding a line to a Paid order is refused" = {
  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
  let pen <- Db.insert Product { name: Text.NonEmpty "Pen", price: Int.NonNegative 150 }
  let id <- Db.insert Order { owner: me, status: Paid }
  let model <- load { id: id } { user: me }
  let r <- Test.run (update (AddLine { product: pen, form: { quantity: Int.Positive 1 } }) model { user: me })
  let lines <- Db.from Line |> Db.count
  Task.succeed ({ value: r.value, lines: lines } == { value: Err "the order is not open", lines: 0 })

test "a Staff user opening another user's order is not found; a Manager sees it" = {
  let owner <- Db.insert User { email: Text.Email "owner@example.com", name: Text.NonEmpty "Owner", passwordHash: "" }
  let staff <- Db.insert User { email: Text.Email "staff@example.com", name: Text.NonEmpty "Staff", passwordHash: "" }
  let boss <- Db.insert User { email: Text.Email "boss@example.com", name: Text.NonEmpty "Boss", passwordHash: "", role: Manager }
  let id <- Db.insert Order { owner: owner }
  let _ <- Test.login { user: Just staff }
  let asStaff <- Test.open (Orders.Show { id: id })
  let _ <- Test.login { user: Just boss }
  let asManager <- Test.open (Orders.Show { id: id })
  Task.succeed ({ staff: asStaff.answer, manager: asManager.answer } == { staff: PageMissing, manager: Shown })
```
