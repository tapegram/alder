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

isManager : Maybe User -> Bool
isManager viewer = match viewer {
  Just u -> u.role == Manager,
  Nothing -> False,
}

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

--- file: app/Layout.alder ---
module Layout

Page : { title: Text, children: List (Html a) } -> Html a
Page props =
  <html>
    <head>
      <meta charset="utf-8" />
      <title>{props.title}</title>
    </head>
    <body>
      <nav>
        <a href={Orders.Index {}}>Orders</a>
        <a href={Products.Index {}}>Products</a>
        <button click={App.Logout}>Log out</button>
      </nav>
      <main>{props.children}</main>
    </body>
  </html>

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

statusLabel : Status -> Text
statusLabel status = match status {
  Open -> "Open",
  Paid -> "Paid",
  Cancelled -> "Cancelled",
}

-- The owner may see and act on an order; so may a Manager.
canAccess : Order -> Maybe User -> User.UserId -> Bool
canAccess order viewer uid = order.owner == uid || User.isManager viewer

--- file: app/Line.alder ---
module Line

import Order
import Product

type LineId = brand Text

table Line {
  id: LineId @id,
  order: Order.OrderId @ref Order @cascade,
  product: Product.ProductId @ref Product,
  quantity: Int.Positive,
}

--- file: app/Totals.alder ---
module Totals

import Order
import Line
import Product
import User

lineValue : List Product -> Line -> Int
lineValue products line = {
  let found = List.find (\p -> p.id == line.product) products
  match found {
    Just p -> line.quantity * p.price,
    Nothing -> 0,
  }
}

total : List Product -> List Line -> Int
total products lines = lines |> List.map (lineValue products) |> List.sum

orderTotal : Order.OrderId -> Query Int
orderTotal oid = {
  let lines <- Db.from Line |> Db.where (Line.order.eq oid) |> Db.all
  let products <- Db.from Product |> Db.all
  Query.succeed (total products lines)
}

test "an open order's total over two lines" = {
  let me <- Db.insert User { email: Text.Email "a@example.com", name: Text.NonEmpty "A", passwordHash: "" }
  let pen <- Db.insert Product { name: Text.NonEmpty "Pen", price: Int.NonNegative 250 }
  let ink <- Db.insert Product { name: Text.NonEmpty "Ink", price: Int.NonNegative 1000 }
  let oid <- Db.insert Order { owner: me }
  let _ <- Db.insert Line { order: oid, product: pen, quantity: Int.Positive 2 }
  let _ <- Db.insert Line { order: oid, product: ink, quantity: Int.Positive 1 }
  let n <- orderTotal oid
  Task.succeed (n == 1000 + 500)
}

--- file: app/Orders/Index.alder ---
module Orders.Index

import Order
import Totals
import Layout

page "/" { } requires user

type Row = { order: Order, total: Int }

type Model = { orders: List Row }

rowOf : Order -> Query Row
rowOf order = {
  let t <- Totals.orderTotal order.id
  Query.succeed { order: order, total: t }
}

load : Params -> Session -> Query Model
load params session = {
  let orders <-
    Db.from Order
      |> Db.where (Order.owner.eq session.user)
      |> Db.orderBy Order.created.desc
      |> Db.all
  let rows <- Query.all (List.map rowOf orders)
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
    <button click={Create}>New order</button>
    <ul id="orders">
      {for r in model.orders ->
        <li>
          <a href={Orders.Show { id: r.order.id }}>Order {Time.format "2006-01-02 15:04:05" r.order.created}</a>
          <span class="status">{Order.statusLabel r.order.status}</span>
          <span class="total">{Text.fromInt r.total}</span>
        </li>}
    </ul>
  </Layout.Page>

--- file: app/Orders/Show.alder ---
module Orders.Show

import Order
import Line
import Product
import User
import Totals
import Layout

page "/orders/{id}" { id: Order.OrderId } requires user

type Row = { id: Line.LineId, name: Text, quantity: Int, subtotal: Int }

type Model = {
  order: Order,
  lines: List Row,
  total: Int,
  products: List Product,
}

row : List Product -> Line -> Row
row products line = {
  let found = List.find (\p -> p.id == line.product) products
  match found {
    Just p -> { id: line.id, name: p.name, quantity: line.quantity, subtotal: line.quantity * p.price },
    Nothing -> { id: line.id, name: "?", quantity: line.quantity, subtotal: 0 },
  }
}

build : Order -> Query Model
build order = {
  let lines <- Db.from Line |> Db.where (Line.order.eq order.id) |> Db.all
  let products <- Db.from Product |> Db.orderBy Product.name.asc |> Db.all
  Query.succeed {
    order: order,
    lines: List.map (row products) lines,
    total: Totals.total products lines,
    products: List.filter (\p -> p.active) products,
  }
}

load : Params -> Session -> Query Model
load params session = {
  let found <- Db.get Order params.id
  let me <- Db.get User session.user
  match found {
    Just order -> if Order.canAccess order me session.user then build order else Query.notFound,
    Nothing -> Query.notFound,
  }
}

form NewLine = {
  quantity: Int.Positive,
}

msg AddLine { product: Product.ProductId } form NewLine
msg Pay
msg Cancel

-- The order as it is now, not as the page saw it: a form from a page loaded while the order was
-- Open must not change an order that has since been paid or cancelled.
-- Refusal is `Task.fail`, not `Task.notFound`: the order exists and the user may see it; the
-- request conflicts with its state. A 404 would claim the order is missing. Failing rolls back
-- everything, so nothing is written.
openOrder : Order.OrderId -> Task Order
openOrder id = {
  let found <- Db.get Order id
  match found {
    Just o -> if o.status == Open then Task.succeed o else Task.fail "order is not open",
    Nothing -> Task.notFound,
  }
}

activeProduct : Product.ProductId -> Task Product
activeProduct id = {
  let found <- Db.get Product id
  match found {
    Just prod -> if prod.active then Task.succeed prod else Task.fail "product is not active",
    Nothing -> Task.notFound,
  }
}

update : Msg -> Model -> Session -> Task Nav
update msg model session = match msg {
  AddLine p -> {
    let o <- openOrder model.order.id
    let prod <- activeProduct p.product
    let _ <- Db.insert Line { order: o.id, product: prod.id, quantity: p.form.quantity }
    Task.succeed (Redirect (Orders.Show { id: o.id }))
  },
  Pay p -> {
    let o <- openOrder model.order.id
    let _ <- Db.update Order o.id (\x -> { x | status: Paid })
    Task.succeed (Redirect (Orders.Show { id: o.id }))
  },
  Cancel p -> {
    let o <- openOrder model.order.id
    let _ <- Db.update Order o.id (\x -> { x | status: Cancelled })
    Task.succeed (Redirect (Orders.Show { id: o.id }))
  },
}

OpenActions : { products: List Product } -> Html Msg
OpenActions props =
  <div id="actions">
    <ul>
      {for prod in props.products ->
        <li>
          <form submit={AddLine { product: prod.id }}>
            <span class="product">{prod.name}</span>
            <Input field="quantity" label="Quantity" />
            <Errors field="quantity" />
            <button>Add line</button>
          </form>
        </li>}
    </ul>
    <button click={Pay}>Mark paid</button>
    <button click={Cancel}>Cancel order</button>
  </div>

view : Model -> Session -> Html Msg
view model session =
  <Layout.Page title="Order">
    <h1>Order</h1>
    <p>Status: <span id="status">{Order.statusLabel model.order.status}</span></p>
    <ul id="lines">
      {for r in model.lines ->
        <li>
          <span class="name">{r.name}</span>
          x <span class="quantity">{Text.fromInt r.quantity}</span>
          = <span class="subtotal">{Text.fromInt r.subtotal}</span>
        </li>}
    </ul>
    <p>Total: <span id="total">{Text.fromInt model.total}</span></p>
    {if model.order.status == Open then <OpenActions products={model.products} /> else Html.none}
    <a href={Orders.Index {}}>Back</a>
  </Layout.Page>

test "adding a line to a paid order is refused" = {
  let me <- Db.insert User { email: Text.Email "a@example.com", name: Text.NonEmpty "A", passwordHash: "" }
  let pid <- Db.insert Product { name: Text.NonEmpty "Pen", price: Int.NonNegative 250 }
  let oid <- Db.insert Order { owner: me }
  let _ <- Db.update Order oid (\o -> { o | status: Paid })
  let r <- Test.run (update (AddLine { product: pid, form: { quantity: Int.Positive 1 } }) { order: { id: oid, owner: me, status: Open, created: Time.epoch }, lines: [], total: 0, products: [] } { user: me })
  let n <- Db.from Line |> Db.count
  Task.succeed ({ value: r.value, lines: n } == { value: Err "order is not open", lines: 0 })
}

test "a staff user opening another user's order gets not found" = {
  let owner <- Db.insert User { email: Text.Email "o@example.com", name: Text.NonEmpty "Owner", passwordHash: "" }
  let other <- Db.insert User { email: Text.Email "s@example.com", name: Text.NonEmpty "Staff", passwordHash: "" }
  let oid <- Db.insert Order { owner: owner }
  let _ <- Test.login { user: Just other }
  let r <- Test.open (Orders.Show { id: oid })
  Task.succeed (r.answer == PageMissing)
}

test "a manager can open another user's order" = {
  let owner <- Db.insert User { email: Text.Email "o@example.com", name: Text.NonEmpty "Owner", passwordHash: "" }
  let boss <- Db.insert User { email: Text.Email "m@example.com", name: Text.NonEmpty "Boss", passwordHash: "", role: Manager }
  let oid <- Db.insert Order { owner: owner }
  let _ <- Test.login { user: Just boss }
  let page <- Test.get (Orders.Show { id: oid })
  Task.succeed ({ id: page.model.order.id, status: Test.text "#status" page.doc } == { id: oid, status: Just "Open" })
}

--- file: app/Products/Index.alder ---
module Products.Index

import Product
import User
import Layout

page "/products" { } requires user

type Model = { products: List Product }

loadAll : Query Model
loadAll = {
  let products <- Db.from Product |> Db.orderBy Product.name.asc |> Db.all
  Query.succeed { products: products }
}

load : Params -> Session -> Query Model
load params session = {
  let me <- Db.get User session.user
  if User.isManager me then loadAll else Query.notFound
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
    let _ <- Db.insert Product { name: p.form.name, price: p.form.price }
    Task.succeed (Redirect (Products.Index {}))
  },
  Toggle p -> {
    let found <- Db.get Product p.id
    match found {
      Just prod -> {
        let _ <- Db.update Product prod.id (\x -> { x | active: not x.active })
        Task.succeed (Redirect (Products.Index {}))
      },
      Nothing -> Task.notFound,
    }
  },
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
      {for prod in model.products ->
        <li>
          <span class="name">{prod.name}</span>
          <span class="price">{Text.fromInt prod.price}</span>
          <span class="active">{if prod.active then "active" else "inactive"}</span>
          <button click={Toggle { id: prod.id }}>{if prod.active then "Deactivate" else "Activate"}</button>
        </li>}
    </ul>
  </Layout.Page>

--- file: app/Auth/Login.alder ---
module Auth.Login

import User
import Layout

-- Password checking is outside the language: a capability implemented in Go.
capability Password {
  verify : { email: Text.Email, password: Text } -> Task (Maybe User.UserId),
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
