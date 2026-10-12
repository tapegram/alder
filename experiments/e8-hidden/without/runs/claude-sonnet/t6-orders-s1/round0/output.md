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

statusLabel : Status -> Text
statusLabel status = match status {
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

import Order
import Line
import Product

lineTotal : List Product -> Line -> Int
lineTotal products line = {
  let found = List.find (\p -> p.id == line.product) products
  match found {
    Just p -> line.quantity * p.price,
    Nothing -> 0,
  }
}

total : List Line -> List Product -> Int
total lines products =
  lines
    |> List.map (lineTotal products)
    |> List.sum

forOrder : Order.OrderId -> Query Int
forOrder id = {
  let lines <- Db.from Line |> Db.where (Line.order.eq id) |> Db.all
  let products <- Db.from Product |> Db.all
  Query.succeed (total lines products)
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
      <nav>
        <a href={Orders.Index {}}>Orders</a>
        <a href={Products.Index {}}>Products</a>
        <button click={App.Logout}>Log out</button>
      </nav>
      <main>{props.children}</main>
    </body>
  </html>

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

--- file: app/Auth/Login.alder ---
module Auth.Login

import User
import Layout

-- Password checking is outside the language: a capability implemented in Go.
capability Password {
  verify : { email: Text.Email, password: Text } -> Task (Maybe User.UserId),
}

-- In a test, `Test.fake Password` replaces the adapter: it accepts any user who has a row.
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

--- file: app/Orders/Index.alder ---
module Orders.Index

import Order
import Totals
import Layout

page "/" { } requires user

type Row = { order: Order, total: Int }

type Model = { rows: List Row }

rowFor : Order -> Query Row
rowFor o = {
  let t <- Totals.forOrder o.id
  Query.succeed { order: o, total: t }
}

load : Params -> Session -> Query Model
load params session = {
  let orders <-
    Db.from Order
      |> Db.where (Order.owner.eq session.user)
      |> Db.orderBy Order.created.desc
      |> Db.all
  let rows <- Query.all (List.map rowFor orders)
  Query.succeed { rows: rows }
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
      {for row in model.rows ->
        <li>
          <a href={Orders.Show { id: row.order.id }}>{Time.format "2006-01-02 15:04:05" row.order.created}</a>
          <span class="status">{Order.statusLabel row.order.status}</span>
          <span class="total">{Text.fromInt row.total}</span>
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

type Item = { line: Line, name: Text, subtotal: Int }

type Model = {
  order: Order,
  items: List Item,
  total: Int,
  active: List Product,
}

canSee : Order -> Session -> Query Bool
canSee o session = {
  let viewer <- Db.get User session.user
  match viewer {
    Just u -> Query.succeed (o.owner == session.user || u.role == Manager),
    Nothing -> Query.succeed False,
  }
}

itemFor : List Product -> Line -> Item
itemFor products line = {
  let found = List.find (\p -> p.id == line.product) products
  match found {
    Just p -> { line: line, name: p.name, subtotal: Totals.lineTotal products line },
    Nothing -> { line: line, name: "?", subtotal: 0 },
  }
}

show : Order -> Query Model
show o = {
  let lines <- Db.from Line |> Db.where (Line.order.eq o.id) |> Db.all
  let products <- Db.from Product |> Db.orderBy Product.name.asc |> Db.all
  Query.succeed {
    order: o,
    items: List.map (itemFor products) lines,
    total: Totals.total lines products,
    active: List.filter (\p -> p.active) products,
  }
}

load : Params -> Session -> Query Model
load params session = {
  let found <- Db.get Order params.id
  match found {
    Nothing -> Query.notFound,
    Just o -> {
      let allowed <- canSee o session
      if allowed then show o else Query.notFound
    },
  }
}

form NewLine = {
  quantity: Int.Positive,
}

msg AddLine { product: Product.ProductId } form NewLine
msg Pay
msg Cancel

isOpen : Order.OrderId -> Task Bool
isOpen id = {
  let found <- Db.get Order id
  match found {
    Just o -> Task.succeed (o.status == Open),
    Nothing -> Task.succeed False,
  }
}

insertLine : Order.OrderId -> Product.ProductId -> Int.Positive -> Task Nav
insertLine id product quantity = {
  let _ <- Db.insert Line { order: id, product: product, quantity: quantity }
  Task.succeed (Redirect (Orders.Show { id: id }))
}

addLine : Order.OrderId -> Product.ProductId -> Int.Positive -> Task Nav
addLine id product quantity = {
  let ok <- isOpen id
  let found <- Db.get Product product
  match found {
    Just prod -> if ok && prod.active then insertLine id product quantity else Task.succeed (Reject "This order is not open."),
    Nothing -> Task.notFound,
  }
}

change : Order.OrderId -> Order.Status -> Task Nav
change id status = {
  let _ <- Db.update Order id (\o -> { o | status: status })
  Task.succeed (Redirect (Orders.Show { id: id }))
}

setStatus : Order.OrderId -> Order.Status -> Task Nav
setStatus id status = {
  let ok <- isOpen id
  if ok then change id status else Task.succeed (Reject "This order is not open.")
}

update : Msg -> Model -> Session -> Task Nav
update msg model session = match msg {
  AddLine p -> addLine model.order.id p.product p.form.quantity,
  Pay p -> setStatus model.order.id Paid,
  Cancel p -> setStatus model.order.id Cancelled,
}

view : Model -> Session -> Html Msg
view model session =
  <Layout.Page title="Order">
    <h1>Order</h1>
    <p>Status: <span id="status">{Order.statusLabel model.order.status}</span></p>
    <p>Total: <span id="total">{Text.fromInt model.total}</span></p>
    <ul id="lines">
      {for item in model.items ->
        <li>
          <span class="name">{item.name}</span>
          x
          <span class="quantity">{Text.fromInt item.line.quantity}</span>
          =
          <span class="subtotal">{Text.fromInt item.subtotal}</span>
        </li>}
    </ul>
    {if model.order.status == Open then
      <div id="actions">
        {for p in model.active ->
          <form submit={AddLine { product: p.id }}>
            <Errors form />
            <Input field="quantity" label={p.name} />
            <Errors field="quantity" />
            <button>Add</button>
          </form>}
        <button click={Pay}>Mark paid</button>
        <button click={Cancel}>Mark cancelled</button>
      </div>
    else Html.none}
    <a href={Orders.Index {}}>Back</a>
  </Layout.Page>

--- file: app/Products/Index.alder ---
module Products.Index

import Product
import User
import Layout

page "/products" { } requires user

type Model = { products: List Product }

loadProducts : Query Model
loadProducts = {
  let products <- Db.from Product |> Db.orderBy Product.name.asc |> Db.all
  Query.succeed { products: products }
}

load : Params -> Session -> Query Model
load params session = {
  let viewer <- Db.get User session.user
  match viewer {
    Just u -> if u.role == Manager then loadProducts else Query.notFound,
    Nothing -> Query.notFound,
  }
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
      <Errors form />
      <Input field="name" label="Name" />
      <Errors field="name" />
      <Input field="price" label="Price (cents)" />
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
