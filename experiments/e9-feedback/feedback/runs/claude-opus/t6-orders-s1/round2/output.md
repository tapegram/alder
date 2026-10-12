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
-- a row, whatever the password.
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

-- These tests go through the login form with the fake above in the adapter's place: the real
-- `Password` adapter is never called.
test "an unknown email is rejected" = {
  let _ <- Test.fake Password
  let _ <- Test.get (Auth.Login {})
  let r <- Test.submit Credentials { email: Text.Email "nobody@example.com", password: Text.Secret "secret" }
  Task.succeed ({ answer: r.answer, user: r.session.user } == { answer: FormRejected [{ field: "", message: "Email or password is incorrect." }], user: Nothing })
}

test "logging in through the form" = {
  let _ <- Test.fake Password
  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
  let _ <- Test.get (Auth.Login {})
  let r <- Test.submit Credentials { email: Text.Email "me@example.com", password: Text.Secret "secret" }
  Task.succeed ({ answer: r.answer, user: r.session.user, title: Test.text "h1" r.doc } == { answer: Redirected (Orders.Index {}), user: Just me, title: Just "Orders" })
}

test "a visitor is sent to log in, and can log out" = {
  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
  let visitor <- Test.open (Orders.Index {})
  let _ <- Test.login { user: Just me }
  let _ <- Test.get (Orders.Index {})
  let out <- Test.click App.Logout
  Task.succeed ({ visitor: visitor.answer, out: out.answer, user: out.session.user } == { visitor: Redirected (Auth.Login {}), out: Redirected (Auth.Login {}), user: Nothing })
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

-- lineTotal is quantity × price in cents; a line whose product is not in the list counts 0.
lineTotal : List Product -> Line -> Int
lineTotal products line = {
  let found = List.find (\p -> p.id == line.product) products
  match found {
    Just p -> line.quantity * p.price,
    Nothing -> 0,
  }
}

total : List Product -> List Line -> Int
total products lines = List.sum (List.map (lineTotal products) lines)

productName : List Product -> Line -> Text
productName products line = {
  let found = List.find (\p -> p.id == line.product) products
  match found {
    Just p -> Text.trim p.name,
    Nothing -> "",
  }
}

test "total sums quantity times price" =
  total [
    { id: Product.ProductId "tea", name: Text.NonEmpty "Tea", price: Int.NonNegative 500, active: True },
    { id: Product.ProductId "cup", name: Text.NonEmpty "Cup", price: Int.NonNegative 250, active: True },
  ] [
    { id: LineId "a", order: Order.OrderId "o", product: Product.ProductId "tea", quantity: Int.Positive 2 },
    { id: LineId "b", order: Order.OrderId "o", product: Product.ProductId "cup", quantity: Int.Positive 1 },
  ] == 1250

test "an empty order totals zero" =
  total [] [] == 0

test "productName finds the line's product" =
  productName [
    { id: Product.ProductId "cup", name: Text.NonEmpty "Cup", price: Int.NonNegative 250, active: True },
    { id: Product.ProductId "tea", name: Text.NonEmpty "Tea", price: Int.NonNegative 500, active: True },
  ] { id: LineId "a", order: Order.OrderId "o", product: Product.ProductId "tea", quantity: Int.Positive 2 } == "Tea"

-- `order` is `@ref Order @cascade`: an order's lines go with it.
test "deleting an order deletes its lines" = {
  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
  let tea <- Db.insert Product { name: Text.NonEmpty "Tea", price: Int.NonNegative 500 }
  let id <- Db.insert Order { owner: me }
  let _ <- Db.insert Line { order: id, product: tea, quantity: Int.Positive 2 }
  let r <- Test.run (Db.delete Order id)
  let n <- Db.from Line |> Db.count
  Task.succeed ({ value: r.value, lines: n } == { value: Ok (), lines: 0 })
}

-- `product` is `@ref Product` with the default rule, `@restrict`: a product that a line names
-- cannot be deleted, and the line stays.
test "a product on a line cannot be deleted" = {
  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
  let tea <- Db.insert Product { name: Text.NonEmpty "Tea", price: Int.NonNegative 500 }
  let id <- Db.insert Order { owner: me }
  let _ <- Db.insert Line { order: id, product: tea, quantity: Int.Positive 2 }
  let r <- Test.run (Db.delete Product tea)
  let n <- Db.from Line |> Db.count
  Task.succeed ({ deleted: Result.toMaybe r.value, lines: n } == { deleted: Nothing, lines: 1 })
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

-- `owner` is `@ref User` with the default rule, `@restrict`: a user who still has orders cannot
-- be deleted, and the orders stay.
test "a user with orders cannot be deleted" = {
  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
  let id <- Db.insert Order { owner: me }
  let r <- Test.run (Db.delete User me)
  let after <- Db.get Order id
  Task.succeed ({ deleted: Result.toMaybe r.value, status: Maybe.map (\o -> o.status) after } == { deleted: Nothing, status: Just Open })
}

--- file: app/Orders/Index.alder ---
module Orders.Index

import Order
import Line
import Product
import Layout
import User

page "/" { } requires user

type Row = { order: Order, total: Int }

type Model = {
  rows: List Row,
  manager: Bool,
}

-- The surviving mutant that drops `Db.where (Line.order.in …)` below is equivalent: `row` keeps
-- only the lines whose `order` is its own order's id, so reading every line gives the same rows,
-- the same page and the same trace ("Db.all Line"), only slower; no input can tell the two apart.
load : Params -> Session -> Query Model
load params session = {
  let me <- Db.get User session.user
  let orders <-
    Db.from Order
      |> Db.where (Order.owner.eq session.user)
      |> Db.orderBy Order.created.desc
      |> Db.all
  let lines <- Db.from Line |> Db.where (Line.order.in (List.map (\o -> o.id) orders)) |> Db.all
  let products <- Db.from Product |> Db.all
  Query.succeed { rows: List.map (row products lines) orders, manager: User.isManager me }
}

row : List Product -> List Line -> Order -> Row
row products lines order =
  { order: order, total: Line.total products (List.filter (\l -> l.order == order.id) lines) }

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

    <button click={Create}>New order</button>

    <ul id="orders">
      {for r in model.rows ->
        <li>
          <a href={Orders.Show { id: r.order.id }}>Order {r.order.id}</a>
          <span class="status">{Order.statusLabel r.order.status}</span>
          <span class="total">{Text.fromInt r.total}</span>
        </li>}
    </ul>

    {if model.manager then <a href={Products.Index {}}>Products</a> else Html.none}

    <button click={App.Logout}>Log out</button>
  </Layout.Page>

test "creating an order opens it, empty" = {
  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
  let _ <- Test.login { user: Just me }
  let _ <- Test.get (Orders.Index {})
  let r <- Test.click Create
  let n <- Db.from Order |> Db.where (Order.owner.eq me) |> Db.count
  Task.succeed ({ orders: n, status: Test.text "#status" r.doc, total: Test.text "#total" r.doc } == { orders: 1, status: Just "Open", total: Just "0" })
}

test "the list shows my orders with status and total, newest first" = {
  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
  let other <- Db.insert User { email: Text.Email "other@example.com", name: Text.NonEmpty "Other", passwordHash: "" }
  let tea <- Db.insert Product { name: Text.NonEmpty "Tea", price: Int.NonNegative 500 }
  let cup <- Db.insert Product { name: Text.NonEmpty "Cup", price: Int.NonNegative 250 }
  let first <- Db.insert Order { owner: me }
  let second <- Db.insert Order { owner: me, status: Paid }
  let _ <- Db.insert Order { owner: other }
  let _ <- Db.insert Line { order: first, product: tea, quantity: Int.Positive 2 }
  let _ <- Db.insert Line { order: first, product: cup, quantity: Int.Positive 1 }
  let _ <- Db.insert Line { order: second, product: cup, quantity: Int.Positive 3 }
  let _ <- Test.login { user: Just me }
  let page <- Test.get (Orders.Index {})
  Task.succeed ({ statuses: Test.texts "#orders .status" page.doc, totals: Test.texts "#orders .total" page.doc, items: Test.count "#orders > li" page.doc } == { statuses: ["Paid", "Open"], totals: ["750", "1250"], items: 2 })
}

test "a visitor is sent to log in" = {
  let r <- Test.open (Orders.Index {})
  Task.succeed (r.answer == Redirected (Auth.Login {}))
}

--- file: app/Orders/Show.alder ---
module Orders.Show

import Order
import Line
import Product
import Layout
import User

page "/orders/{id}" { id: Order.OrderId } requires user

type Model = {
  order: Order,
  lines: List Line,
  products: List Product,
  total: Int,
}

-- The order is shown to its owner and to a Manager; to anyone else it is a 404.
load : Params -> Session -> Query Model
load params session = {
  let found <- Db.get Order params.id
  let me <- Db.get User session.user
  match found {
    Just order -> if order.owner == session.user || User.isManager me then details order else Query.notFound,
    Nothing -> Query.notFound,
  }
}

details : Order -> Query Model
details order = {
  let lines <- Db.from Line |> Db.where (Line.order.eq order.id) |> Db.all
  let products <- Db.from Product |> Db.orderBy Product.name.asc |> Db.all
  Query.succeed { order: order, lines: lines, products: products, total: Line.total products lines }
}

form NewLine = {
  quantity: Int.Positive,
}

-- The messages carry no order id: the page's parameter names the order, and `load` has checked
-- that the session's user may see it. The status is read again here, so a form posted from a
-- page loaded while the order was Open is refused once the order is not.
msg AddLine { product: Product.ProductId } form NewLine
msg Pay
msg Cancel

update : Msg -> Model -> Session -> Task Nav
update msg model session = match msg {
  AddLine p -> {
    let order <- current model.order.id
    if order.status == Open then addLine order.id p.product p.form.quantity else Task.succeed (Reject "This order is no longer open.")
  },
  Pay p -> setStatus model.order.id Paid,
  Cancel p -> setStatus model.order.id Cancelled,
}

current : Order.OrderId -> Task Order
current id = {
  let found <- Db.get Order id
  match found {
    Just order -> Task.succeed order,
    Nothing -> Task.notFound,
  }
}

-- sellable is the product a message names, when it exists and is active; otherwise a 404.
sellable : Product.ProductId -> Task Product
sellable id = {
  let found <- Db.get Product id
  match found {
    Just product -> if product.active then Task.succeed product else Task.notFound,
    Nothing -> Task.notFound,
  }
}

addLine : Order.OrderId -> Product.ProductId -> Int.Positive -> Task Nav
addLine id productId quantity = {
  let product <- sellable productId
  let _ <- Db.insert Line { order: id, product: product.id, quantity: quantity }
  Task.succeed (Redirect (Orders.Show { id: id }))
}

setStatus : Order.OrderId -> Order.Status -> Task Nav
setStatus id status = {
  let order <- current id
  if order.status == Open then close order.id status else Task.succeed Rerender
}

close : Order.OrderId -> Order.Status -> Task Nav
close id status = {
  let _ <- Db.update Order id (\o -> { o | status: status })
  Task.succeed (Redirect (Orders.Show { id: id }))
}

view : Model -> Session -> Html Msg
view model session =
  <Layout.Page title="Order">
    <h1>Order {model.order.id}</h1>

    <p>Status: <span id="status">{Order.statusLabel model.order.status}</span></p>

    <ul id="lines">
      {for line in model.lines ->
        <li>
          <span class="product">{Line.productName model.products line}</span>
          <span class="quantity">{Text.fromInt line.quantity}</span>
          <span class="subtotal">{Text.fromInt (Line.lineTotal model.products line)}</span>
        </li>}
    </ul>

    <p>Total: <span id="total">{Text.fromInt model.total}</span></p>

    {if model.order.status == Open then
      <div id="actions">
        {for product in List.filter (\p -> p.active) model.products ->
          <form submit={AddLine { product: product.id }}>
            <span class="name">{product.name}</span>
            <Errors form />
            <Input field="quantity" label="Quantity" />
            <Errors field="quantity" />
            <button>Add</button>
          </form>}
        <button click={Pay}>Mark paid</button>
        <button click={Cancel}>Cancel order</button>
      </div>
    else Html.none}

    <a href={Orders.Index {}}>Back</a>
  </Layout.Page>

test "an Open order's total over two lines" = {
  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
  let tea <- Db.insert Product { name: Text.NonEmpty "Tea", price: Int.NonNegative 500 }
  let cup <- Db.insert Product { name: Text.NonEmpty "Cup", price: Int.NonNegative 250 }
  let id <- Db.insert Order { owner: me }
  let _ <- Db.insert Line { order: id, product: tea, quantity: Int.Positive 2 }
  let _ <- Db.insert Line { order: id, product: cup, quantity: Int.Positive 1 }
  let _ <- Test.login { user: Just me }
  let page <- Test.get (Orders.Show { id: id })
  Task.succeed ({ status: Test.text "#status" page.doc, total: Test.text "#total" page.doc, model: page.model.total } == { status: Just "Open", total: Just "1250", model: 1250 })
}

test "adding a line through the form" = {
  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
  let tea <- Db.insert Product { name: Text.NonEmpty "Tea", price: Int.NonNegative 500 }
  let id <- Db.insert Order { owner: me }
  let _ <- Test.login { user: Just me }
  let _ <- Test.get (Orders.Show { id: id })
  let r <- Test.submit (AddLine { product: tea }) { quantity: Int.Positive 3 }
  Task.succeed ({ answer: r.answer, total: Test.text "#total" r.doc } == { answer: Redirected (Orders.Show { id: id }), total: Just "1500" })
}

test "adding a line to a Paid order is refused" = {
  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
  let tea <- Db.insert Product { name: Text.NonEmpty "Tea", price: Int.NonNegative 500 }
  let id <- Db.insert Order { owner: me }
  let _ <- Test.login { user: Just me }
  let _ <- Test.get (Orders.Show { id: id })
  let _ <- Db.update Order id (\o -> { o | status: Paid })
  let r <- Test.submit (AddLine { product: tea }) { quantity: Int.Positive 3 }
  let n <- Db.from Line |> Db.count
  Task.succeed ({ answer: r.answer, lines: n, status: Test.text "#status" r.doc } == { answer: Rerendered, lines: 0, status: Just "Paid" })
}

-- The Paid order's page no longer draws the form, so the browser cannot tell `Reject` from
-- `Rerender`; a test of `update` itself sees the `Nav` value.
test "AddLine on a Paid order answers Reject and writes nothing" = {
  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
  let tea <- Db.insert Product { name: Text.NonEmpty "Tea", price: Int.NonNegative 500 }
  let id <- Db.insert Order { owner: me, status: Paid }
  let r <- Test.run (update (AddLine { product: tea, form: { quantity: Int.Positive 3 } }) { order: { id: id, owner: me, status: Open, created: Time.epoch }, lines: [], products: [], total: 0 } { user: me })
  let n <- Db.from Line |> Db.count
  Task.succeed ({ value: r.value, lines: n } == { value: Ok (Reject "This order is no longer open."), lines: 0 })
}

-- An inactive product has no form on the page; a message posted for it anyway is a 404.
test "AddLine with an inactive product is not found" = {
  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
  let jam <- Db.insert Product { name: Text.NonEmpty "Jam", price: Int.NonNegative 300, active: False }
  let id <- Db.insert Order { owner: me }
  let r <- Test.run (update (AddLine { product: jam, form: { quantity: Int.Positive 3 } }) { order: { id: id, owner: me, status: Open, created: Time.epoch }, lines: [], products: [], total: 0 } { user: me })
  let n <- Db.from Line |> Db.count
  Task.succeed ({ value: r.value, lines: n } == { value: Err "not found", lines: 0 })
}

test "an order shows only its own lines, with product names" = {
  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
  let tea <- Db.insert Product { name: Text.NonEmpty "Tea", price: Int.NonNegative 500 }
  let cup <- Db.insert Product { name: Text.NonEmpty "Cup", price: Int.NonNegative 250 }
  let id <- Db.insert Order { owner: me }
  let other <- Db.insert Order { owner: me }
  let _ <- Db.insert Line { order: id, product: tea, quantity: Int.Positive 2 }
  let _ <- Db.insert Line { order: other, product: cup, quantity: Int.Positive 1 }
  let _ <- Test.login { user: Just me }
  let page <- Test.get (Orders.Show { id: id })
  Task.succeed ({ products: Test.texts "#lines .product" page.doc, quantities: Test.texts "#lines .quantity" page.doc, total: Test.text "#total" page.doc } == { products: ["Tea"], quantities: ["2"], total: Just "1000" })
}

test "active products are offered by name" = {
  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
  let _ <- Db.insert Product { name: Text.NonEmpty "Tea", price: Int.NonNegative 500 }
  let _ <- Db.insert Product { name: Text.NonEmpty "Cup", price: Int.NonNegative 250 }
  let _ <- Db.insert Product { name: Text.NonEmpty "Jam", price: Int.NonNegative 300, active: False }
  let id <- Db.insert Order { owner: me }
  let _ <- Test.login { user: Just me }
  let page <- Test.get (Orders.Show { id: id })
  Task.succeed (Test.texts "#actions .name" page.doc == ["Cup", "Tea"])
}

test "a Paid order's page has no forms or buttons" = {
  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
  let _ <- Db.insert Product { name: Text.NonEmpty "Tea", price: Int.NonNegative 500 }
  let id <- Db.insert Order { owner: me, status: Paid }
  let _ <- Test.login { user: Just me }
  let page <- Test.get (Orders.Show { id: id })
  Task.succeed ({ actions: Test.has "#actions" page.doc, status: Test.text "#status" page.doc } == { actions: False, status: Just "Paid" })
}

test "paying an order" = {
  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
  let id <- Db.insert Order { owner: me }
  let _ <- Test.login { user: Just me }
  let _ <- Test.get (Orders.Show { id: id })
  let r <- Test.click Pay
  Task.succeed ({ answer: r.answer, status: Test.text "#status" r.doc } == { answer: Redirected (Orders.Show { id: id }), status: Just "Paid" })
}

test "cancelling an order" = {
  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
  let id <- Db.insert Order { owner: me }
  let _ <- Test.login { user: Just me }
  let _ <- Test.get (Orders.Show { id: id })
  let r <- Test.click Cancel
  Task.succeed ({ answer: r.answer, status: Test.text "#status" r.doc } == { answer: Redirected (Orders.Show { id: id }), status: Just "Cancelled" })
}

test "a visitor is sent to log in from an order" = {
  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
  let id <- Db.insert Order { owner: me }
  let visitor <- Test.open (Orders.Show { id: id })
  Task.succeed (visitor.answer == Redirected (Auth.Login {}))
}

test "a Staff user opening another user's order is not found" = {
  let owner <- Db.insert User { email: Text.Email "owner@example.com", name: Text.NonEmpty "Owner", passwordHash: "" }
  let other <- Db.insert User { email: Text.Email "other@example.com", name: Text.NonEmpty "Other", passwordHash: "" }
  let id <- Db.insert Order { owner: owner }
  let _ <- Test.login { user: Just other }
  let r <- Test.open (Orders.Show { id: id })
  Task.succeed (r.answer == PageMissing)
}

test "a Manager opening another user's order sees it" = {
  let owner <- Db.insert User { email: Text.Email "owner@example.com", name: Text.NonEmpty "Owner", passwordHash: "" }
  let boss <- Db.insert User { email: Text.Email "boss@example.com", name: Text.NonEmpty "Boss", passwordHash: "", role: Manager }
  let id <- Db.insert Order { owner: owner }
  let _ <- Test.login { user: Just boss }
  let r <- Test.open (Orders.Show { id: id })
  Task.succeed ({ answer: r.answer, status: Test.text "#status" r.doc } == { answer: Shown, status: Just "Open" })
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

--- file: app/Products/Index.alder ---
module Products.Index

import Product
import Layout
import User

page "/products" { } requires user

type Model = { products: List Product }

-- Only a Manager is served this page; to a Staff user it is a 404.
load : Params -> Session -> Query Model
load params session = {
  let me <- Db.get User session.user
  if User.isManager me then listing else Query.notFound
}

listing : Query Model
listing = {
  let products <- Db.from Product |> Db.orderBy Product.name.asc |> Db.all
  Query.succeed { products: products }
}

form NewProduct = {
  name: Text.NonEmpty,
  price: Int.NonNegative,
}

msg Add form NewProduct
msg Toggle { id: Product.ProductId }

update : Msg -> Model -> Session -> Task Nav
update msg model session = match msg {
  Add p -> {
    let _ <- requireManager session
    let _ <- Db.insert Product { name: p.form.name, price: p.form.price }
    Task.succeed (Redirect (Products.Index {}))
  },
  Toggle p -> {
    let _ <- requireManager session
    let product <- existing p.id
    let _ <- Db.update Product product.id (\x -> { x | active: not x.active })
    Task.succeed (Redirect (Products.Index {}))
  },
}

-- requireManager is the session's user when a Manager; otherwise the message is a 404.
requireManager : Session -> Task User
requireManager session = {
  let me <- Db.get User session.user
  match me {
    Just u -> if u.role == Manager then Task.succeed u else Task.notFound,
    Nothing -> Task.notFound,
  }
}

existing : Product.ProductId -> Task Product
existing id = {
  let found <- Db.get Product id
  match found {
    Just product -> Task.succeed product,
    Nothing -> Task.notFound,
  }
}

view : Model -> Session -> Html Msg
view model session =
  <Layout.Page title="Products">
    <h1>Products</h1>

    <ul id="products">
      {for product in model.products ->
        <li class={if product.active then "active" else "inactive"}>
          <span class="name">{product.name}</span>
          <span class="price">{Text.fromInt product.price}</span>
          <button click={Toggle { id: product.id }}>{if product.active then "Deactivate" else "Activate"}</button>
        </li>}
    </ul>

    <form submit={Add}>
      <Errors form />
      <Input field="name" label="Name" />
      <Errors field="name" />
      <Input field="price" label="Price in cents" />
      <Errors field="price" />
      <button>Add</button>
    </form>

    <a href={Orders.Index {}}>Orders</a>
    <button click={App.Logout}>Log out</button>
  </Layout.Page>

test "a Staff user does not find the products page" = {
  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
  let _ <- Test.login { user: Just me }
  let r <- Test.open (Products.Index {})
  Task.succeed (r.answer == PageMissing)
}

test "a visitor is sent to log in from the products page" = {
  let visitor <- Test.open (Products.Index {})
  Task.succeed (visitor.answer == Redirected (Auth.Login {}))
}

test "a Manager adds a product" = {
  let boss <- Db.insert User { email: Text.Email "boss@example.com", name: Text.NonEmpty "Boss", passwordHash: "", role: Manager }
  let _ <- Test.login { user: Just boss }
  let _ <- Test.get (Products.Index {})
  let r <- Test.submit NewProduct { name: Text.NonEmpty "Tea", price: Int.NonNegative 500 }
  Task.succeed ({ answer: r.answer, names: Test.texts "#products .name" r.doc } == { answer: Redirected (Products.Index {}), names: ["Tea"] })
}

test "a Manager toggles a product" = {
  let boss <- Db.insert User { email: Text.Email "boss@example.com", name: Text.NonEmpty "Boss", passwordHash: "", role: Manager }
  let tea <- Db.insert Product { name: Text.NonEmpty "Tea", price: Int.NonNegative 500 }
  let _ <- Test.login { user: Just boss }
  let _ <- Test.get (Products.Index {})
  let r <- Test.click (Toggle { id: tea })
  let after <- Db.get Product tea
  Task.succeed ({ answer: r.answer, active: Maybe.map (\p -> p.active) after } == { answer: Redirected (Products.Index {}), active: Just False })
}

test "products are listed by name" = {
  let boss <- Db.insert User { email: Text.Email "boss@example.com", name: Text.NonEmpty "Boss", passwordHash: "", role: Manager }
  let _ <- Db.insert Product { name: Text.NonEmpty "Tea", price: Int.NonNegative 500 }
  let _ <- Db.insert Product { name: Text.NonEmpty "Cup", price: Int.NonNegative 250 }
  let _ <- Test.login { user: Just boss }
  let page <- Test.get (Products.Index {})
  Task.succeed (Test.texts "#products .name" page.doc == ["Cup", "Tea"])
}

-- A Staff user's page is a 404, so it has no form to post; `update` refuses the message anyway.
test "Add by a Staff user is not found" = {
  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
  let r <- Test.run (update (Add { form: { name: Text.NonEmpty "Tea", price: Int.NonNegative 500 } }) { products: [] } { user: me })
  let n <- Db.from Product |> Db.count
  Task.succeed ({ value: r.value, products: n } == { value: Err "not found", products: 0 })
}

test "Toggle by a Staff user is not found" = {
  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
  let tea <- Db.insert Product { name: Text.NonEmpty "Tea", price: Int.NonNegative 500 }
  let r <- Test.run (update (Toggle { id: tea }) { products: [] } { user: me })
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

-- isManager says whether the row found for the session's user is a Manager.
isManager : Maybe User -> Bool
isManager found = match found {
  Just u -> u.role == Manager,
  Nothing -> False,
}

test "a user without a row is not a manager" =
  isManager Nothing == False
