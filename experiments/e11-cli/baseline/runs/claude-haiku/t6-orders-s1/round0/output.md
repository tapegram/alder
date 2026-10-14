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

test "Logout signs the user out" = {
  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
  let _ <- Test.login { user: Just me }
  let _ <- Test.get (Orders.Index {})
  let out <- Test.click App.Logout
  Task.succeed ({ answer: out.answer, user: out.session.user } == { answer: Redirected (Auth.Login {}), user: Nothing })
}

--- file: app/Layout.alder ---
module Layout

Page : { title: Text, children: List (Html a) } -> Html a
Page props =
  <html>
    <head>
      <meta charset="utf-8" />
      <title>{props.title}</title>
      <link rel="stylesheet" href={Url.external "/static/al/al.css"} />
      <link rel="stylesheet" href={Url.external "/static/app.css"} />
    </head>
    <body>
      <al-page width="narrow">
        <main>{props.children}</main>
      </al-page>
    </body>
  </html>

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

--- file: app/Product.alder ---
module Product

type ProductId = brand Text

table Product {
  id: ProductId @id,
  name: Text.NonEmpty,
  price: Int.NonNegative,
  active: Bool = True,
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

statusText : Status -> Text
statusText s = match s {
  Open -> "Open",
  Paid -> "Paid",
  Cancelled -> "Cancelled",
}

--- file: app/Line.alder ---
module Line

import User
import Order
import Product

type LineId = brand Text

table Line {
  id: LineId @id,
  order: Order.OrderId @ref Order @cascade,
  product: Product.ProductId @ref Product,
  quantity: Int.Positive,
}

type LineRow = { name: Text, quantity: Int, amount: Int }

rowOf : Line -> Query LineRow
rowOf line = {
  let pid = line.product
  let product <- Db.get Product pid
  let name = Maybe.withDefault "?" (Maybe.map (\p -> Text.trim p.name) product)
  let amount = Maybe.withDefault 0 (Maybe.map (\p -> line.quantity * p.price) product)
  Query.succeed { name: name, quantity: line.quantity, amount: amount }
}

linesOf : Order.OrderId -> Query (List LineRow)
linesOf orderId = {
  let lines <- Db.from Line |> Db.where (Line.order.eq orderId) |> Db.all
  let rows <- Query.all (List.map rowOf lines)
  Query.succeed rows
}

totalOf : Order.OrderId -> Query Int
totalOf orderId = {
  let rows <- linesOf orderId
  Query.succeed (List.sum (List.map (\r -> r.amount) rows))
}

test "an order's total is the sum of its lines" = {
  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
  let order <- Db.insert Order { owner: me }
  let tea <- Db.insert Product { name: Text.NonEmpty "Tea", price: Int.NonNegative 250 }
  let mug <- Db.insert Product { name: Text.NonEmpty "Mug", price: Int.NonNegative 1000 }
  let _ <- Db.insert Line { order: order, product: tea, quantity: Int.Positive 2 }
  let _ <- Db.insert Line { order: order, product: mug, quantity: Int.Positive 1 }
  let total <- totalOf order
  Task.succeed (total == 1500)
}

--- file: app/Auth/Login.alder ---
module Auth.Login

import User
import Layout

-- Password checking is outside the language: a capability implemented in Go.
capability Password {
  verify : { email: Text.Email, password: Text } -> Task (Maybe User.UserId),
}

-- In a test, `Test.fake Password` puts this in the adapter's place.
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
    <al-card>
      <form submit={Submit}>
        <Errors form />
        <al-input field="email" label="Email" />
        <al-input field="password" label="Password" />
        <al-button type="submit">Log in</al-button>
      </form>
    </al-card>
  </Layout.Page>

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
  Task.succeed ({ answer: r.answer, user: r.session.user } == { answer: Redirected (Orders.Index {}), user: Just me })
}

--- file: app/Orders/Index.alder ---
module Orders.Index

import Order
import Line
import Layout

page "/" { } requires user

type Summary = { id: Order.OrderId, status: Order.Status, total: Int }

type Model = { orders: List Summary }

summaryOf : Order -> Query Summary
summaryOf order = {
  let total <- Line.totalOf order.id
  Query.succeed { id: order.id, status: order.status, total: total }
}

load : Params -> Session -> Query Model
load params session = {
  let found <- Db.from Order |> Db.where (Order.owner.eq session.user) |> Db.orderBy Order.created.desc |> Db.all
  let rows <- Query.all (List.map summaryOf found)
  Query.succeed { orders: rows }
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
      <al-button type="submit">New order</al-button>
    </form>
    <ul id="orders">
      {for o in model.orders ->
        <li>
          <a href={Orders.Show { id: o.id }}>Order</a>
          <span class="status">{Order.statusText o.status}</span>
          <span class="total">{Text.fromInt o.total}</span>
        </li>}
    </ul>
    <al-button variant="outline" click={App.Logout}>Log out</al-button>
  </Layout.Page>

--- file: app/Orders/Show.alder ---
module Orders.Show

import User
import Order
import Line
import Product
import Layout

page "/orders/{id}" { id: Order.OrderId } requires user

type Model = {
  order: Order,
  lines: List Line.LineRow,
  total: Int,
  products: List Product,
}

form NewLine = { quantity: Int.Positive }

msg AddLine { product: Product.ProductId } form NewLine
msg Pay
msg Cancel

-- The order is shown to its owner, and to a Manager; anyone else is a 404.
showOrder : Order -> Query Model
showOrder order = {
  let lines <- Line.linesOf order.id
  let total <- Line.totalOf order.id
  let products <- Db.from Product |> Db.where (Product.active.eq True) |> Db.all
  Query.succeed { order: order, lines: lines, total: total, products: products }
}

load : Params -> Session -> Query Model
load params session = {
  let me <- Db.get User session.user
  let found <- Db.get Order params.id
  let manager = Maybe.map (\u -> u.role) me == Just Manager
  match found {
    Nothing -> Query.notFound,
    Just order -> if order.owner == session.user || manager then showOrder order else Query.notFound,
  }
}

addLine : Order.OrderId -> Product.ProductId -> Int.Positive -> Task Nav
addLine orderId product quantity = {
  let _ <- Db.insert Line { order: orderId, product: product, quantity: quantity }
  Task.succeed (Redirect (Orders.Show { id: orderId }))
}

setStatus : Order.OrderId -> Order.Status -> Task Nav
setStatus orderId next = {
  let _ <- Db.update Order orderId (\o -> { o | status: next })
  Task.succeed (Redirect (Orders.Show { id: orderId }))
}

-- A line can be added to, and an order closed, only while it is Open, even from a page that was
-- loaded while it was Open.
update : Msg -> Model -> Session -> Task Nav
update msg model session = match msg {
  AddLine p ->
    if model.order.status == Open then addLine model.order.id p.product p.form.quantity else Task.fail "the order is not open",
  Pay p ->
    if model.order.status == Open then setStatus model.order.id Paid else Task.fail "the order is not open",
  Cancel p ->
    if model.order.status == Open then setStatus model.order.id Cancelled else Task.fail "the order is not open",
}

view : Model -> Session -> Html Msg
view model session =
  <Layout.Page title="Order">
    <h1>Order</h1>
    <p>Status: <span id="status">{Order.statusText model.order.status}</span></p>
    <ul id="lines">
      {for r in model.lines -> <li>{r.name} {Text.fromInt r.quantity} x {Text.fromInt r.amount}</li>}
    </ul>
    <p>Total <span id="total">{Text.fromInt model.total}</span></p>
    {if model.order.status == Open then <div>
      {for p in model.products ->
        <form submit={AddLine { product: p.id }}>
          <al-input field="quantity" label="Quantity" />
          <al-button type="submit">Add {p.name}</al-button>
        </form>}
      <al-button click={Pay}>Mark paid</al-button>
      <al-button variant="destructive" click={Cancel}>Cancel order</al-button>
    </div> else Html.none}
    <a href={Orders.Index {}}>All orders</a>
  </Layout.Page>

test "a Staff user cannot open another user's order" = {
  let owner <- Db.insert User { email: Text.Email "owner@example.com", name: Text.NonEmpty "Owner", passwordHash: "" }
  let other <- Db.insert User { email: Text.Email "other@example.com", name: Text.NonEmpty "Other", passwordHash: "" }
  let order <- Db.insert Order { owner: owner }
  let _ <- Test.login { user: Just other }
  let r <- Test.open (Orders.Show { id: order })
  Task.succeed (r.answer == PageMissing)
}

test "a Manager can open another user's order" = {
  let owner <- Db.insert User { email: Text.Email "owner@example.com", name: Text.NonEmpty "Owner", passwordHash: "" }
  let boss <- Db.insert User { email: Text.Email "boss@example.com", name: Text.NonEmpty "Boss", passwordHash: "", role: Manager }
  let order <- Db.insert Order { owner: owner }
  let _ <- Test.login { user: Just boss }
  let page <- Test.get (Orders.Show { id: order })
  Task.succeed ({ id: page.model.order.id, status: Test.text "#status" page.doc } == { id: order, status: Just "Open" })
}

test "adding a line to a Paid order is refused" = {
  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
  let order <- Db.insert Order { owner: me }
  let tea <- Db.insert Product { name: Text.NonEmpty "Tea", price: Int.NonNegative 250 }
  let _ <- Test.login { user: Just me }
  let _ <- Test.get (Orders.Show { id: order })
  let _ <- Db.update Order order (\o -> { o | status: Paid })
  let r <- Test.submit (AddLine { product: tea }) { quantity: Int.Positive 1 }
  let lines <- Db.from Line |> Db.where (Line.order.eq order) |> Db.count
  Task.succeed ({ answer: r.answer, lines: lines } == { answer: RequestFailed "the order is not open", lines: 0 })
}

--- file: app/Products/Index.alder ---
module Products.Index

import Product
import Layout
import User

page "/products" { } requires user

type Model = { products: List Product }

-- Only a Manager may see the products; `load` runs before every message, so the check covers them too.
load : Params -> Session -> Query Model
load params session = {
  let me <- Db.get User session.user
  let manager = Maybe.map (\u -> u.role) me == Just Manager
  if manager then {
    let products <- Db.from Product |> Db.all
    Query.succeed { products: products }
  } else Query.notFound
}

form NewProduct = { name: Text.NonEmpty, price: Int.NonNegative }

msg Add form NewProduct
msg Toggle { id: Product.ProductId }

update : Msg -> Model -> Session -> Task Nav
update msg model session = match msg {
  Add p -> {
    let _ <- Db.insert Product { name: p.form.name, price: p.form.price }
    Task.succeed (Redirect (Products.Index {}))
  },
  Toggle p -> {
    let found <- Db.get Product p.id
    match found {
      Nothing -> Task.notFound,
      Just product -> {
        let _ <- Db.update Product p.id (\x -> { x | active: not x.active })
        Task.succeed (Redirect (Products.Index {}))
      },
    }
  },
}

view : Model -> Session -> Html Msg
view model session =
  <Layout.Page title="Products">
    <h1>Products</h1>
    <al-card>
      <form submit={Add}>
        <al-input field="name" label="Name" />
        <al-input field="price" label="Price (cents)" />
        <al-button type="submit">Add product</al-button>
      </form>
    </al-card>
    <ul id="products">
      {for p in model.products ->
        <li class={if p.active then "active" else "inactive"}>
          <span class="name">{p.name}</span>
          <span class="price">{Text.fromInt p.price}</span>
          <al-button variant="secondary" size="sm" click={Toggle { id: p.id }}>{if p.active then "Deactivate" else "Activate"}</al-button>
        </li>}
    </ul>
    <al-button variant="outline" click={App.Logout}>Log out</al-button>
  </Layout.Page>

test "a Staff user gets not found" = {
  let me <- Db.insert User { email: Text.Email "staff@example.com", name: Text.NonEmpty "Staff", passwordHash: "" }
  let _ <- Test.login { user: Just me }
  let r <- Test.open (Products.Index {})
  Task.succeed (r.answer == PageMissing)
}
