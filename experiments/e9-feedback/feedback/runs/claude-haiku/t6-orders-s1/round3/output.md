--- file: app/App.alder ---
module App

import User
import Layout

session { user: Maybe User.UserId }

login = Auth.Login

msg Logout

update : Msg -> Session -> Task Nav
update msg session = match msg {
  Logout p -> {
    let _ <- Session.put (\s -> { s | user: Nothing })
    Task.succeed (Redirect (Auth.Login {}))
  },
}

error : Error -> Html Never
error err =
  <Layout.Page title="Error">
    <h1>Something went wrong</h1>
    <p>Please try again.</p>
  </Layout.Page>

notFound : Html Never
notFound =
  <Layout.Page title="Not found">
    <h1>Not found</h1>
  </Layout.Page>

test "Logout clears the user and goes to log in" = {
  let r <- Test.run (update Logout { user: Just (User.UserId "me") })
  Task.succeed ({ value: r.value, trace: r.trace } == { value: Ok (Redirect (Auth.Login {})), trace: ["Session.put"] })
}

--- file: app/Auth/Login.alder ---
module Auth.Login

import User
import Layout

-- Password checking is outside the language: a capability implemented in Go.
capability Password {
  verify : { email: Text.Email, password: Text } -> Task (Maybe User.UserId),
}

-- In a test, `Test.fake Password` puts this in the adapter's place: it accepts any user who has
-- a row, whatever the password, so a test can log in through the form below.
fake Password {
  verify args = {
    let found <- Db.from User |> Db.where (User.email.eq args.email) |> Db.first
    Task.succeed (Maybe.map (\u -> u.id) found)
  }
}

page "/login" { }

type Model = { }

load : Params -> Session -> Query Model
load params session = Query.succeed { }

form Credentials = {
  email: Text.Email,
  password: Text.Secret,
}

msg Submit form Credentials

update : Msg -> Model -> Session -> Task Nav
update msg model session = match msg {
  Submit p -> {
    let found <- Password.verify { email: p.form.email, password: p.form.password }
    match found {
      Just id -> {
        let _ <- Session.put (\s -> { s | user: Just id })
        Task.succeed (Redirect (Orders.Index {}))
      },
      Nothing -> Task.succeed (Reject "Email or password is incorrect."),
    }
  },
}

view : Model -> Session -> Html Msg
view model session =
  <Layout.Page title="Log in">
    <h1>Log in</h1>
    <form submit={Submit}>
      <Errors form />
      <Input field="email" label="Email" />
      <Errors field="email" />
      <Input field="password" label="Password" />
      <Errors field="password" />
      <button>Log in</button>
    </form>
  </Layout.Page>

-- A correct login stores the user in the session and goes to the orders; the session is checked
-- after the redirect, so a login that stores nothing is seen.
test "logging in through the form stores the user and goes to the orders" = {
  let _ <- Test.fake Password
  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
  let _ <- Test.get (Auth.Login {})
  let r <- Test.submit Credentials { email: Text.Email "me@example.com", password: Text.Secret "secret" }
  Task.succeed ({ answer: r.answer, user: r.session.user } == { answer: Redirected (Orders.Index {}), user: Just me })
}

-- A wrong login is refused with the form's message, and the session stays empty.
test "an unknown email is rejected with the form's message" = {
  let _ <- Test.fake Password
  let _ <- Test.get (Auth.Login {})
  let r <- Test.submit Credentials { email: Text.Email "nobody@example.com", password: Text.Secret "secret" }
  Task.succeed ({ answer: r.answer, user: r.session.user } == { answer: FormRejected [{ field: "", message: "Email or password is incorrect." }], user: Nothing })
}

--- file: app/Layout.alder ---
module Layout

Page : { title: Text, children: List (Html a) } -> Html a
Page props =
  <html>
    <head>
      <meta charset="utf-8" />
      <title>{props.title}</title>
      <link rel="stylesheet" href={Url.external "/static/app.css"} />
    </head>
    <body>
      <main>{props.children}</main>
    </body>
  </html>

--- file: app/Line.alder ---
module Line

import Order
import Product
import User

type LineId = brand Text

table Line {
  id: LineId @id,
  order: Order.OrderId @ref Order @cascade,
  product: Product.ProductId @ref Product,
  quantity: Int.Positive,
}

-- lineAmount is what one line costs: its quantity times its product's price in cents. Nothing
-- when the product is not in the list.
lineAmount : Line -> List Product -> Maybe Int
lineAmount line products =
  Maybe.map (\p -> line.quantity * p.price) (List.find (\p -> p.id == line.product) products)

-- orderTotal sums, in cents, the lines that belong to one order.
orderTotal : Order.OrderId -> List Line -> List Product -> Int
orderTotal id lines products =
  lines
    |> List.filter (\l -> l.order == id)
    |> List.filterMap (\l -> lineAmount l products)
    |> List.sum

test "an order's total is the sum of its lines" =
  orderTotal (Order.OrderId "o1") [
    { id: LineId "l1", order: Order.OrderId "o1", product: Product.ProductId "p1", quantity: Int.Positive 2 },
    { id: LineId "l2", order: Order.OrderId "o1", product: Product.ProductId "p2", quantity: Int.Positive 1 },
    { id: LineId "l3", order: Order.OrderId "o2", product: Product.ProductId "p1", quantity: Int.Positive 9 },
  ] [
    { id: Product.ProductId "p1", name: Text.NonEmpty "Tea", price: Int.NonNegative 500, active: True },
    { id: Product.ProductId "p2", name: Text.NonEmpty "Cup", price: Int.NonNegative 250, active: True },
  ] == 1250

-- A product that an order's line names cannot be deleted: the reference's default rule refuses.
test "a product on an order cannot be deleted" = {
  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
  let tea <- Db.insert Product { name: Text.NonEmpty "Tea", price: Int.NonNegative 500 }
  let id <- Db.insert Order { owner: me }
  let _ <- Db.insert Line { order: id, product: tea, quantity: Int.Positive 1 }
  let r <- Test.run (Db.delete Product tea)
  let lines <- Db.from Line |> Db.count
  Task.succeed ({ refused: Result.toMaybe r.value == Nothing, lines: lines } == { refused: True, lines: 1 })
}

-- Deleting an order deletes its lines: the reference's rule is @cascade.
test "deleting an order deletes its lines" = {
  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
  let tea <- Db.insert Product { name: Text.NonEmpty "Tea", price: Int.NonNegative 500 }
  let id <- Db.insert Order { owner: me }
  let _ <- Db.insert Line { order: id, product: tea, quantity: Int.Positive 1 }
  let _ <- Db.delete Order id
  let lines <- Db.from Line |> Db.count
  Task.succeed (lines == 0)
}

--- file: app/Order.alder ---
module Order

import User

type OrderId = brand Text

type Status = Open | Paid | Cancelled

table Order {
  id: OrderId @id,
  owner: User.UserId @ref User,
  status: Status = Open,
  created: Time @created,
}

statusLabel : Status -> Text
statusLabel status = match status {
  Open -> "Open",
  Paid -> "Paid",
  Cancelled -> "Cancelled",
}

test "each status has its label" =
  List.map statusLabel [Open, Paid, Cancelled] == ["Open", "Paid", "Cancelled"]

test "a new order is open" = {
  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
  let id <- Db.insert Order { owner: me }
  let row <- Db.get Order id
  Task.succeed (Maybe.map (\o -> o.status) row == Just Open)
}

-- A user who owns an order cannot be deleted while the order exists (the default rule).
test "a user with orders cannot be deleted" = {
  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
  let _ <- Db.insert Order { owner: me }
  let r <- Test.run (Db.delete User me)
  let n <- Db.from Order |> Db.count
  Task.succeed ({ refused: Result.toMaybe r.value == Nothing, orders: n } == { refused: True, orders: 1 })
}

--- file: app/Orders/Index.alder ---
module Orders.Index

import Order
import Line
import Product
import Layout
import User

page "/" { } requires user

type OrderRow = { id: Order.OrderId, status: Order.Status, total: Int }

type Model = {
  rows: List OrderRow,
  manager: Bool,
}

toRow : List Line -> List Product -> Order -> OrderRow
toRow lines products o = { id: o.id, status: o.status, total: Line.orderTotal o.id lines products }

-- Equivalent mutant: the lines query's where only narrows which lines are read. toRow's orderTotal
-- keeps only each order's own lines, and every row's id is one of the listed orders, so no
-- input makes dropping this where change a total or a row.
load : Params -> Session -> Query Model
load params session = {
  let orders <-
    Db.from Order
      |> Db.where (Order.owner.eq session.user)
      |> Db.orderBy Order.created.desc
      |> Db.all
  let lines <- Db.from Line |> Db.where (Line.order.in (List.map (\o -> o.id) orders)) |> Db.all
  let products <- Db.from Product |> Db.all
  let role <- User.roleOf session.user
  Query.succeed { rows: List.map (toRow lines products) orders, manager: role == Manager }
}

msg Create

update : Msg -> Model -> Session -> Task Nav
update msg model session = match msg {
  Create p -> {
    let id <- Db.insert Order { owner: session.user }
    Task.succeed (Redirect (Orders.Show { id: id }))
  },
}

view : Model -> Session -> Html Msg
view model session =
  <Layout.Page title="Orders">
    <h1>Orders</h1>
    <form submit={Create}>
      <button>New order</button>
    </form>
    <ul id="orders">
      {for row in model.rows ->
        <li>
          <a href={Orders.Show { id: row.id }}>View</a>
          <span class="status">{Order.statusLabel row.status}</span>
          <span class="total">{Text.fromInt row.total}</span>
        </li>}
    </ul>
    {if model.manager then <a href={Products.Index {}}>Products</a> else Html.none}
    <button click={App.Logout}>Log out</button>
  </Layout.Page>

test "a visitor is sent to log in" = {
  let visitor <- Test.open (Orders.Index {})
  Task.succeed (visitor.answer == Redirected (Auth.Login {}))
}

test "a user sees only their own orders" = {
  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
  let other <- Db.insert User { email: Text.Email "other@example.com", name: Text.NonEmpty "Other", passwordHash: "" }
  let _ <- Db.insert Order { owner: me }
  let _ <- Db.insert Order { owner: other }
  let _ <- Test.login { user: Just me }
  let page <- Test.get (Orders.Index {})
  Task.succeed (Test.count "#orders li" page.doc == 1)
}

test "the newest order is listed first" = {
  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
  let older <- Db.insert Order { owner: me }
  let _ <- Db.update Order older (\o -> { o | status: Paid })
  let _ <- Db.insert Order { owner: me }
  let _ <- Test.login { user: Just me }
  let page <- Test.get (Orders.Index {})
  Task.succeed (Test.texts "#orders .status" page.doc == ["Open", "Paid"])
}

test "an order's total is listed" = {
  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
  let tea <- Db.insert Product { name: Text.NonEmpty "Tea", price: Int.NonNegative 500 }
  let id <- Db.insert Order { owner: me }
  let _ <- Db.insert Line { order: id, product: tea, quantity: Int.Positive 2 }
  let _ <- Test.login { user: Just me }
  let page <- Test.get (Orders.Index {})
  Task.succeed (Test.texts "#orders .total" page.doc == ["1000"])
}

test "New order opens a new open order" = {
  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
  let _ <- Test.login { user: Just me }
  let _ <- Test.get (Orders.Index {})
  let r <- Test.click Create
  let n <- Db.from Order |> Db.count
  Task.succeed ({ status: Test.text "#status" r.doc, orders: n } == { status: Just "Open", orders: 1 })
}

test "only a manager sees the products link" = {
  let sam <- Db.insert User { email: Text.Email "sam@example.com", name: Text.NonEmpty "Sam", passwordHash: "" }
  let boss <- Db.insert User { email: Text.Email "boss@example.com", name: Text.NonEmpty "Boss", passwordHash: "", role: Manager }
  let _ <- Test.login { user: Just sam }
  let staffPage <- Test.get (Orders.Index {})
  let _ <- Test.login { user: Just boss }
  let bossPage <- Test.get (Orders.Index {})
  Task.succeed ({ staff: Test.texts "main > a" staffPage.doc, boss: Test.texts "main > a" bossPage.doc } == { staff: [], boss: ["Products"] })
}

--- file: app/Orders/Show.alder ---
module Orders.Show

import Order
import Line
import Product
import Layout
import User

page "/orders/{id}" { id: Order.OrderId } requires user

type LineView = { name: Text, quantity: Int, amount: Int }

type Model = {
  order: Order,
  lines: List LineView,
  total: Int,
  products: List Product,
}

form NewLine = { quantity: Int.Positive }

msg AddLine { product: Product.ProductId } form NewLine
msg Pay
msg Cancel

-- lineView is one line as the page shows it.
lineView : Line -> Product -> LineView
lineView line p = { name: p.name, quantity: line.quantity, amount: line.quantity * p.price }

-- lineRow is the page's row for a line, when its product is known.
lineRow : List Product -> Line -> Maybe LineView
lineRow products line =
  Maybe.map (lineView line) (List.find (\p -> p.id == line.product) products)

-- pageFor is the model of an order the session may see.
pageFor : Order -> Query Model
pageFor order = {
  let lines <- Db.from Line |> Db.where (Line.order.eq order.id) |> Db.all
  let allProducts <- Db.from Product |> Db.orderBy Product.name.asc |> Db.all
  Query.succeed {
    order: order,
    lines: List.filterMap (lineRow allProducts) lines,
    total: Line.orderTotal order.id lines allProducts,
    products: List.filter (\p -> p.active) allProducts,
  }
}

load : Params -> Session -> Query Model
load params session = {
  let found <- Db.get Order params.id
  let role <- User.roleOf session.user
  match found {
    Just order -> if order.owner == session.user || role == Manager then pageFor order else Query.notFound,
    Nothing -> Query.notFound,
  }
}

-- visibleOrder is the order a message names, when the session may act on it: its owner, or a
-- Manager. Otherwise the message is a 404 (authorize in update too).
visibleOrder : Order.OrderId -> Session -> Task Order
visibleOrder id session = {
  let found <- Db.get Order id
  let role <- User.roleOf session.user
  match found {
    Just order -> if order.owner == session.user || role == Manager then Task.succeed order else Task.notFound,
    Nothing -> Task.notFound,
  }
}

-- openOrder is an order that may still change. A Paid or Cancelled order refuses the change,
-- even when the form came from a page loaded while it was Open.
openOrder : Order.OrderId -> Session -> Task Order
openOrder id session = {
  let order <- visibleOrder id session
  if order.status == Open then Task.succeed order else Task.fail "the order is not open"
}

activeProduct : Maybe Product -> Task Product
activeProduct found = match found {
  Just product -> if product.active then Task.succeed product else Task.fail "that product is not active",
  Nothing -> Task.notFound,
}

update : Msg -> Model -> Session -> Task Nav
update msg model session = match msg {
  AddLine p -> {
    let order <- openOrder model.order.id session
    let found <- Db.get Product p.product
    let product <- activeProduct found
    let _ <- Db.insert Line { order: order.id, product: product.id, quantity: p.form.quantity }
    Task.succeed (Redirect (Orders.Show { id: order.id }))
  },
  Pay p -> {
    let order <- openOrder model.order.id session
    let _ <- Db.update Order order.id (\o -> { o | status: Paid })
    Task.succeed (Redirect (Orders.Show { id: order.id }))
  },
  Cancel p -> {
    let order <- openOrder model.order.id session
    let _ <- Db.update Order order.id (\o -> { o | status: Cancelled })
    Task.succeed (Redirect (Orders.Show { id: order.id }))
  },
}

-- OpenControls are the forms and buttons of an Open order.
OpenControls : { products: List Product } -> Html Msg
OpenControls props =
  <div>
    {for p in props.products ->
      <form submit={AddLine { product: p.id }}>
        <Errors form />
        <Input field="quantity" label="Quantity" />
        <Errors field="quantity" />
        <button>Add {p.name}</button>
      </form>}
    <button click={Pay}>Mark paid</button>
    <button click={Cancel}>Cancel order</button>
  </div>

view : Model -> Session -> Html Msg
view model session =
  <Layout.Page title="Order">
    <h1>Order</h1>
    <p>Status: <span id="status">{Order.statusLabel model.order.status}</span></p>
    <ul id="lines">
      {for l in model.lines ->
        <li><span class="name">{l.name}</span> x <span class="quantity">{Text.fromInt l.quantity}</span> = <span class="amount">{Text.fromInt l.amount}</span></li>}
    </ul>
    <p>Total: <span id="total">{Text.fromInt model.total}</span></p>
    {if model.order.status == Open then <OpenControls products={model.products} /> else Html.none}
    <a href={Orders.Index {}}>Back</a>
    <button click={App.Logout}>Log out</button>
  </Layout.Page>

test "an open order's total is the sum of its lines" = {
  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
  let tea <- Db.insert Product { name: Text.NonEmpty "Tea", price: Int.NonNegative 500 }
  let cup <- Db.insert Product { name: Text.NonEmpty "Cup", price: Int.NonNegative 250 }
  let id <- Db.insert Order { owner: me }
  let _ <- Db.insert Line { order: id, product: tea, quantity: Int.Positive 2 }
  let _ <- Db.insert Line { order: id, product: cup, quantity: Int.Positive 1 }
  let _ <- Test.login { user: Just me }
  let page <- Test.get (Orders.Show { id: id })
  Task.succeed ({ total: Test.text "#total" page.doc, status: Test.text "#status" page.doc } == { total: Just "1250", status: Just "Open" })
}

test "adding a line to a paid order is refused" = {
  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
  let tea <- Db.insert Product { name: Text.NonEmpty "Tea", price: Int.NonNegative 500 }
  let id <- Db.insert Order { owner: me }
  let _ <- Test.login { user: Just me }
  let _ <- Test.get (Orders.Show { id: id })
  let _ <- Db.update Order id (\o -> { o | status: Paid })
  let r <- Test.submit (AddLine { product: tea }) { quantity: Int.Positive 2 }
  let lines <- Db.from Line |> Db.where (Line.order.eq id) |> Db.count
  Task.succeed ({ answer: r.answer, lines: lines } == { answer: RequestFailed "the order is not open", lines: 0 })
}

test "a staff user opening another user's order is not found" = {
  let owner <- Db.insert User { email: Text.Email "owner@example.com", name: Text.NonEmpty "Owner", passwordHash: "" }
  let staffer <- Db.insert User { email: Text.Email "sam@example.com", name: Text.NonEmpty "Sam", passwordHash: "" }
  let id <- Db.insert Order { owner: owner }
  let _ <- Test.login { user: Just staffer }
  let r <- Test.open (Orders.Show { id: id })
  Task.succeed (r.answer == PageMissing)
}

test "a manager opening another user's order is shown it" = {
  let owner <- Db.insert User { email: Text.Email "owner@example.com", name: Text.NonEmpty "Owner", passwordHash: "" }
  let boss <- Db.insert User { email: Text.Email "boss@example.com", name: Text.NonEmpty "Boss", passwordHash: "", role: Manager }
  let id <- Db.insert Order { owner: owner }
  let _ <- Test.login { user: Just boss }
  let r <- Test.open (Orders.Show { id: id })
  Task.succeed (r.answer == Shown)
}

test "a visitor is sent to log in from an order" = {
  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
  let id <- Db.insert Order { owner: me }
  let visitor <- Test.open (Orders.Show { id: id })
  Task.succeed (visitor.answer == Redirected (Auth.Login {}))
}

test "an order shows only its own lines, with their own products" = {
  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
  let tea <- Db.insert Product { name: Text.NonEmpty "Tea", price: Int.NonNegative 500 }
  let cup <- Db.insert Product { name: Text.NonEmpty "Cup", price: Int.NonNegative 250 }
  let id <- Db.insert Order { owner: me }
  let other <- Db.insert Order { owner: me }
  let _ <- Db.insert Line { order: id, product: tea, quantity: Int.Positive 2 }
  let _ <- Db.insert Line { order: other, product: cup, quantity: Int.Positive 1 }
  let _ <- Test.login { user: Just me }
  let page <- Test.get (Orders.Show { id: id })
  Task.succeed ({ names: Test.texts "#lines .name" page.doc, amounts: Test.texts "#lines .amount" page.doc } == { names: ["Tea"], amounts: ["1000"] })
}

test "an open order offers its active products, by name" = {
  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
  let _ <- Db.insert Product { name: Text.NonEmpty "Tea", price: Int.NonNegative 500 }
  let _ <- Db.insert Product { name: Text.NonEmpty "Cup", price: Int.NonNegative 250 }
  let _ <- Db.insert Product { name: Text.NonEmpty "Old", price: Int.NonNegative 100, active: False }
  let id <- Db.insert Order { owner: me }
  let _ <- Test.login { user: Just me }
  let page <- Test.get (Orders.Show { id: id })
  Task.succeed ({ adds: List.take 2 (Test.texts "form button" page.doc), forms: Test.count "input[name=quantity]" page.doc } == { adds: ["Add Cup", "Add Tea"], forms: 2 })
}

test "adding a line through the page" = {
  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
  let tea <- Db.insert Product { name: Text.NonEmpty "Tea", price: Int.NonNegative 500 }
  let id <- Db.insert Order { owner: me }
  let _ <- Test.login { user: Just me }
  let _ <- Test.get (Orders.Show { id: id })
  let r <- Test.submit (AddLine { product: tea }) { quantity: Int.Positive 2 }
  Task.succeed ({ answer: r.answer, total: Test.text "#total" r.doc } == { answer: Redirected (Orders.Show { id: id }), total: Just "1000" })
}

test "a line cannot add an inactive product" = {
  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
  let old <- Db.insert Product { name: Text.NonEmpty "Old", price: Int.NonNegative 100, active: False }
  let id <- Db.insert Order { owner: me }
  let model : Model = { order: { id: id, owner: me, status: Open, created: Time.epoch }, lines: [], total: 0, products: [] }
  let r <- Test.run (update (AddLine { product: old, form: { quantity: Int.Positive 1 } }) model { user: me })
  let lines <- Db.from Line |> Db.where (Line.order.eq id) |> Db.count
  Task.succeed ({ value: r.value, lines: lines } == { value: Err "that product is not active", lines: 0 })
}

test "paying marks the order paid" = {
  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
  let id <- Db.insert Order { owner: me }
  let _ <- Test.login { user: Just me }
  let _ <- Test.get (Orders.Show { id: id })
  let r <- Test.click Pay
  Task.succeed ({ answer: r.answer, status: Test.text "#status" r.doc } == { answer: Redirected (Orders.Show { id: id }), status: Just "Paid" })
}

test "cancelling marks the order cancelled" = {
  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
  let id <- Db.insert Order { owner: me }
  let _ <- Test.login { user: Just me }
  let _ <- Test.get (Orders.Show { id: id })
  let r <- Test.click Cancel
  Task.succeed ({ answer: r.answer, status: Test.text "#status" r.doc } == { answer: Redirected (Orders.Show { id: id }), status: Just "Cancelled" })
}

test "a staff user cannot change another user's order" = {
  let owner <- Db.insert User { email: Text.Email "owner@example.com", name: Text.NonEmpty "Owner", passwordHash: "" }
  let sam <- Db.insert User { email: Text.Email "sam@example.com", name: Text.NonEmpty "Sam", passwordHash: "" }
  let id <- Db.insert Order { owner: owner }
  let model : Model = { order: { id: id, owner: owner, status: Open, created: Time.epoch }, lines: [], total: 0, products: [] }
  let r <- Test.run (update Cancel model { user: sam })
  let after <- Db.get Order id
  Task.succeed ({ value: r.value, status: Maybe.map (\o -> o.status) after } == { value: Err "not found", status: Just Open })
}

test "a manager can pay another user's order" = {
  let owner <- Db.insert User { email: Text.Email "owner@example.com", name: Text.NonEmpty "Owner", passwordHash: "" }
  let boss <- Db.insert User { email: Text.Email "boss@example.com", name: Text.NonEmpty "Boss", passwordHash: "", role: Manager }
  let id <- Db.insert Order { owner: owner }
  let model : Model = { order: { id: id, owner: owner, status: Open, created: Time.epoch }, lines: [], total: 0, products: [] }
  let r <- Test.run (update Pay model { user: boss })
  let after <- Db.get Order id
  Task.succeed ({ value: r.value, status: Maybe.map (\o -> o.status) after } == { value: Ok (Redirect (Orders.Show { id: id })), status: Just Paid })
}

--- file: app/Product.alder ---
module Product

type ProductId = brand Text

table Product {
  id: ProductId @id,
  name: Text.NonEmpty,
  price: Int.NonNegative,
  active: Bool = True,
}

test "a new product is active" = {
  let id <- Db.insert Product { name: Text.NonEmpty "Tea", price: Int.NonNegative 500 }
  let row <- Db.get Product id
  Task.succeed (Maybe.map (\p -> p.active) row == Just True)
}

--- file: app/Products/Index.alder ---
module Products.Index

import Product
import Layout
import User

page "/products" { } requires user

type Model = { products: List Product }

load : Params -> Session -> Query Model
load params session = {
  let role <- User.roleOf session.user
  match role {
    Manager -> {
      let products <- Db.from Product |> Db.orderBy Product.name.asc |> Db.all
      Query.succeed { products: products }
    },
    Staff -> Query.notFound,
  }
}

form NewProduct = {
  name: Text.NonEmpty,
  price: Int.NonNegative,
}

msg Add form NewProduct
msg Toggle { id: Product.ProductId }

-- requireManager ends the request with a 404 for a Staff user.
requireManager : Session -> Task Unit
requireManager session = {
  let role <- User.roleOf session.user
  match role {
    Manager -> Task.succeed (),
    Staff -> Task.notFound,
  }
}

update : Msg -> Model -> Session -> Task Nav
update msg model session = {
  let _ <- requireManager session
  match msg {
    Add p -> {
      let _ <- Db.insert Product { name: p.form.name, price: p.form.price }
      Task.succeed (Redirect (Products.Index {}))
    },
    Toggle p -> {
      let found <- Db.get Product p.id
      match found {
        Just product -> {
          let _ <- Db.update Product product.id (\x -> { x | active: not x.active })
          Task.succeed (Redirect (Products.Index {}))
        },
        Nothing -> Task.notFound,
      }
    },
  }
}

view : Model -> Session -> Html Msg
view model session =
  <Layout.Page title="Products">
    <h1>Products</h1>
    <form submit={Add}>
      <Input field="name" label="Name" />
      <Errors field="name" />
      <Input field="price" label="Price in cents" />
      <Errors field="price" />
      <button>Add product</button>
    </form>
    <ul id="products">
      {for p in model.products ->
        <li>
          <span class="name">{p.name}</span>
          <span class="price">{Text.fromInt p.price}</span>
          <span class="state">{if p.active then "active" else "inactive"}</span>
          <button click={Toggle { id: p.id }}>{if p.active then "Deactivate" else "Activate"}</button>
        </li>}
    </ul>
    <a href={Orders.Index {}}>Orders</a>
    <button click={App.Logout}>Log out</button>
  </Layout.Page>

test "a staff user cannot open the product list" = {
  let sam <- Db.insert User { email: Text.Email "sam@example.com", name: Text.NonEmpty "Sam", passwordHash: "" }
  let _ <- Test.login { user: Just sam }
  let r <- Test.open (Products.Index {})
  Task.succeed (r.answer == PageMissing)
}

test "a visitor is sent to log in from the products" = {
  let visitor <- Test.open (Products.Index {})
  Task.succeed (visitor.answer == Redirected (Auth.Login {}))
}

test "a manager sees every product, by name" = {
  let boss <- Db.insert User { email: Text.Email "boss@example.com", name: Text.NonEmpty "Boss", passwordHash: "", role: Manager }
  let _ <- Db.insert Product { name: Text.NonEmpty "Tea", price: Int.NonNegative 500 }
  let _ <- Db.insert Product { name: Text.NonEmpty "Cup", price: Int.NonNegative 250 }
  let _ <- Db.insert Product { name: Text.NonEmpty "Old", price: Int.NonNegative 100, active: False }
  let _ <- Test.login { user: Just boss }
  let page <- Test.get (Products.Index {})
  Task.succeed (Test.texts "#products .name" page.doc == ["Cup", "Old", "Tea"])
}

test "a manager adds a product" = {
  let boss <- Db.insert User { email: Text.Email "boss@example.com", name: Text.NonEmpty "Boss", passwordHash: "", role: Manager }
  let _ <- Test.login { user: Just boss }
  let _ <- Test.get (Products.Index {})
  let r <- Test.submit NewProduct { name: Text.NonEmpty "Tea", price: Int.NonNegative 500 }
  let page <- Test.get (Products.Index {})
  Task.succeed ({ answer: r.answer, names: Test.texts "#products .name" page.doc, states: Test.texts "#products .state" page.doc } == { answer: Redirected (Products.Index {}), names: ["Tea"], states: ["active"] })
}

test "a manager deactivates a product" = {
  let boss <- Db.insert User { email: Text.Email "boss@example.com", name: Text.NonEmpty "Boss", passwordHash: "", role: Manager }
  let tea <- Db.insert Product { name: Text.NonEmpty "Tea", price: Int.NonNegative 500 }
  let _ <- Test.login { user: Just boss }
  let _ <- Test.get (Products.Index {})
  let r <- Test.click (Toggle { id: tea })
  let after <- Db.get Product tea
  Task.succeed ({ answer: r.answer, active: Maybe.map (\p -> p.active) after } == { answer: Redirected (Products.Index {}), active: Just False })
}

test "a staff user cannot toggle a product" = {
  let sam <- Db.insert User { email: Text.Email "sam@example.com", name: Text.NonEmpty "Sam", passwordHash: "" }
  let tea <- Db.insert Product { name: Text.NonEmpty "Tea", price: Int.NonNegative 500 }
  let r <- Test.run (update (Toggle { id: tea }) { products: [] } { user: sam })
  let after <- Db.get Product tea
  Task.succeed ({ value: r.value, active: Maybe.map (\p -> p.active) after } == { value: Err "not found", active: Just True })
}

--- file: app/User.alder ---
module User

type UserId = brand Text

type Role = Staff | Manager

table User {
  id: UserId @id,
  email: Text.Email @unique,
  name: Text.NonEmpty,
  passwordHash: Text,
  role: Role = Staff,
  created: Time @created,
}

-- roleOf is the role of the user with this id. A missing user gets the least power, Staff.
roleOf : UserId -> Query Role
roleOf id = {
  let found <- Db.get User id
  match found {
    Just u -> Query.succeed u.role,
    Nothing -> Query.succeed Staff,
  }
}

test "a new user is staff" = {
  let u <- Db.insert User { email: Text.Email "new@example.com", name: Text.NonEmpty "New", passwordHash: "" }
  let role <- roleOf u
  Task.succeed (role == Staff)
}
