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

isManager : UserId -> Query Bool
isManager id = {
  let me <- Db.get User id
  Query.succeed (Maybe.map (\u -> u.role) me == Just Manager)
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

statusText : Status -> Text
statusText s = match s {
  Open -> "Open",
  Paid -> "Paid",
  Cancelled -> "Cancelled",
}

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

import Line
import Product

priceOf : Product.ProductId -> List Product -> Int
priceOf pid products = match List.find (\p -> p.id == pid) products {
  Just p -> Int.max 0 p.price,
  Nothing -> 0,
}

nameOf : Product.ProductId -> List Product -> Text
nameOf pid products = match List.find (\p -> p.id == pid) products {
  Just p -> Text.trim p.name,
  Nothing -> "",
}

sumLines : List Line -> List Product -> Int
sumLines lines products =
  List.sum (List.map (\l -> l.quantity * priceOf l.product products) lines)

--- file: app/Auth/Login.alder ---
module Auth.Login

import User
import Layout

capability Password {
  verify : { email: Text.Email, password: Text } -> Task (Maybe User.UserId),
}

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
      <Input field="password" label="Password" />
      <button>Log in</button>
    </form>
  </Layout.Page>

--- file: app/Orders/Index.alder ---
module Orders.Index

import Order
import Line
import Product
import Layout
import Totals

page "/" { } requires user

type Row = { order: Order, total: Int }

type Model = { rows: List Row }

row : List Line -> List Product -> Order -> Row
row lines products o = {
  order: o,
  total: Totals.sumLines (List.filter (\l -> l.order == o.id) lines) products,
}

load : Params -> Session -> Query Model
load params session = {
  let orders <-
    Db.from Order
      |> Db.where (Order.owner.eq session.user)
      |> Db.orderBy Order.created.desc
      |> Db.all
  let lines <- Db.from Line |> Db.where (Line.order.in (List.map (\o -> o.id) orders)) |> Db.all
  let products <- Db.from Product |> Db.all
  Query.succeed { rows: List.map (row lines products) orders }
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
      {for r in model.rows ->
        <li>
          <a href={Orders.Show { id: r.order.id }}>Order {Time.format "2006-01-02 15:04:05" r.order.created}</a>
          <span class="status">{Order.statusText r.order.status}</span>
          <span class="total">{Text.fromInt r.total}</span>
        </li>}
    </ul>
    <button click={App.Logout}>Log out</button>
  </Layout.Page>

--- file: app/Orders/Show.alder ---
module Orders.Show

import Order
import Line
import Product
import Layout
import User
import Totals

page "/orders/{id}" { id: Order.OrderId } requires user

type LineRow = { id: Line.LineId, name: Text, quantity: Int, subtotal: Int }

type Model = {
  order: Order,
  lines: List LineRow,
  products: List Product,
  total: Int,
}

lineRow : List Product -> Line -> LineRow
lineRow products l = {
  id: l.id,
  name: Totals.nameOf l.product products,
  quantity: l.quantity,
  subtotal: l.quantity * Totals.priceOf l.product products,
}

loadModel : Order -> Query Model
loadModel order = {
  let lines <- Db.from Line |> Db.where (Line.order.eq order.id) |> Db.all
  let products <- Db.from Product |> Db.orderBy Product.name.asc |> Db.all
  Query.succeed {
    order: order,
    lines: List.map (lineRow products) lines,
    products: List.filter (\p -> p.active) products,
    total: Totals.sumLines lines products,
  }
}

load : Params -> Session -> Query Model
load params session = {
  let found <- Db.get Order params.id
  match found {
    Nothing -> Query.notFound,
    Just order -> {
      let manager <- User.isManager session.user
      if order.owner == session.user || manager then loadModel order else Query.notFound
    },
  }
}

form NewLine = { quantity: Int.Positive }

msg AddLine { product: Product.ProductId } form NewLine
msg Pay
msg Cancel

setStatus : Model -> Order.Status -> Task Nav
setStatus model s =
  if model.order.status == Open then {
    let _ <- Db.update Order model.order.id (\o -> { o | status: s })
    Task.succeed (Redirect (Orders.Show { id: model.order.id }))
  } else Task.succeed Rerender

update : Msg -> Model -> Session -> Task Nav
update msg model session = match msg {
  AddLine p -> {
    let prod <- Db.get Product p.product
    let ok = model.order.status == Open && Maybe.map (\x -> x.active) prod == Just True
    if ok then {
      let _ <- Db.insert Line { order: model.order.id, product: p.product, quantity: p.form.quantity }
      Task.succeed (Redirect (Orders.Show { id: model.order.id }))
    } else Task.succeed (Reject "This order cannot take new lines.")
  },
  Pay p -> setStatus model Paid,
  Cancel p -> setStatus model Cancelled,
}

Controls : { model: Model } -> Html Msg
Controls props =
  <div>
    {for p in props.model.products ->
      <form submit={AddLine { product: p.id }}>
        <h3>{p.name}</h3>
        <Errors form />
        <Input field="quantity" label="Quantity" />
        <button>Add line</button>
      </form>}
    <button click={Pay}>Mark paid</button>
    <button click={Cancel}>Cancel order</button>
  </div>

view : Model -> Session -> Html Msg
view model session =
  <Layout.Page title="Order">
    <h1>Order</h1>
    <p>Status: <span id="status">{Order.statusText model.order.status}</span></p>
    <ul id="lines">
      {for l in model.lines ->
        <li>
          <span class="name">{l.name}</span>
          <span class="quantity">{Text.fromInt l.quantity}</span>
          <span class="subtotal">{Text.fromInt l.subtotal}</span>
        </li>}
    </ul>
    <p>Total: <span id="total">{Text.fromInt model.total}</span></p>
    {if model.order.status == Open then <Controls model={model} /> else Html.none}
    <a href={Orders.Index {}}>Back</a>
  </Layout.Page>

test "an open order's total over two lines" = {
  let me <- Db.insert User { email: Text.Email "a@x.com", name: Text.NonEmpty "A", passwordHash: "" }
  let p1 <- Db.insert Product { name: Text.NonEmpty "Pen", price: Int.NonNegative 250 }
  let p2 <- Db.insert Product { name: Text.NonEmpty "Ink", price: Int.NonNegative 400 }
  let o <- Db.insert Order { owner: me }
  let _ <- Db.insert Line { order: o, product: p1, quantity: Int.Positive 2 }
  let _ <- Db.insert Line { order: o, product: p2, quantity: Int.Positive 1 }
  let _ <- Test.login { user: Just me }
  let page <- Test.get (Orders.Show { id: o })
  Task.succeed ({ status: Test.text "#status" page.doc, total: Test.text "#total" page.doc } == { status: Just "Open", total: Just "900" })
}

test "adding a line to a paid order is refused" = {
  let me <- Db.insert User { email: Text.Email "a@x.com", name: Text.NonEmpty "A", passwordHash: "" }
  let p1 <- Db.insert Product { name: Text.NonEmpty "Pen", price: Int.NonNegative 250 }
  let o <- Db.insert Order { owner: me }
  let _ <- Test.login { user: Just me }
  let _ <- Test.get (Orders.Show { id: o })
  let _ <- Db.update Order o (\x -> { x | status: Paid })
  let _ <- Test.submit (AddLine { product: p1 }) { quantity: Int.Positive 1 }
  let n <- Db.from Line |> Db.count
  Task.succeed (n == 0)
}

test "a staff user opening another user's order is not found" = {
  let owner <- Db.insert User { email: Text.Email "a@x.com", name: Text.NonEmpty "A", passwordHash: "" }
  let other <- Db.insert User { email: Text.Email "b@x.com", name: Text.NonEmpty "B", passwordHash: "" }
  let o <- Db.insert Order { owner: owner }
  let _ <- Test.login { user: Just other }
  let r <- Test.open (Orders.Show { id: o })
  Task.succeed (r.answer == PageMissing)
}

test "a manager opening another user's order sees it" = {
  let owner <- Db.insert User { email: Text.Email "a@x.com", name: Text.NonEmpty "A", passwordHash: "" }
  let boss <- Db.insert User { email: Text.Email "m@x.com", name: Text.NonEmpty "M", passwordHash: "", role: Manager }
  let o <- Db.insert Order { owner: owner }
  let _ <- Test.login { user: Just boss }
  let r <- Test.open (Orders.Show { id: o })
  Task.succeed (r.answer == Shown)
}

--- file: app/Products/Index.alder ---
module Products.Index

import Product
import Layout
import User

page "/products" { } requires user

type Model = { products: List Product }

allProducts : Query Model
allProducts = {
  let ps <- Db.from Product |> Db.orderBy Product.name.asc |> Db.all
  Query.succeed { products: ps }
}

load : Params -> Session -> Query Model
load params session = {
  let manager <- User.isManager session.user
  if manager then allProducts else Query.notFound
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
    let _ <- Db.update Product p.id (\x -> { x | active: not x.active })
    Task.succeed (Redirect (Products.Index {}))
  },
}

view : Model -> Session -> Html Msg
view model session =
  <Layout.Page title="Products">
    <h1>Products</h1>
    <form submit={Add}>
      <Errors form />
      <Input field="name" label="Name" />
      <Input field="price" label="Price (cents)" />
      <button>Add product</button>
    </form>
    <ul id="products">
      {for p in model.products ->
        <li>
          <span class="name">{p.name}</span>
          <span class="price">{Text.fromInt p.price}</span>
          <button click={Toggle { id: p.id }}>{if p.active then "Deactivate" else "Activate"}</button>
        </li>}
    </ul>
    <a href={Orders.Index {}}>Orders</a>
  </Layout.Page>
