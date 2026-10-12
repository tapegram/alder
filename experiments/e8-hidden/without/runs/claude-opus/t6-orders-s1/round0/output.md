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

-- isManager is whether the user a session names exists and is a Manager.
isManager : Maybe User -> Bool
isManager found = match found {
  Just u -> u.role == Manager,
  Nothing -> False,
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

-- subtotal is quantity × price for one line, in cents.
subtotal : List Product -> Line -> Int
subtotal products line = {
  let found = List.find (\p -> p.id == line.product) products
  match found {
    Just p -> line.quantity * p.price,
    Nothing -> 0,
  }
}

-- total is the sum of the lines' subtotals, in cents.
total : List Product -> List Line -> Int
total products lines = lines |> List.map (subtotal products) |> List.sum

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

--- file: app/Orders/Index.alder ---
module Orders.Index

import User
import Product
import Order
import Line
import Layout

page "/" { } requires user

type Row = {
  order: Order,
  total: Int,
}

type Model = {
  rows: List Row,
  manager: Bool,
}

load : Params -> Session -> Query Model
load params session = {
  let viewer <- Db.get User session.user
  let orders <-
    Db.from Order
      |> Db.where (Order.owner.eq session.user)
      |> Db.orderBy Order.created.desc
      |> Db.all
  let lines <-
    Db.from Line
      |> Db.where (Line.order.in (List.map (\o -> o.id) orders))
      |> Db.all
  let products <- Db.from Product |> Db.all
  Query.succeed { rows: List.map (row products lines) orders, manager: User.isManager viewer }
}

-- row is one order with its total: the sum over the lines that belong to it.
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
          <a href={Orders.Show { id: r.order.id }}>{Time.format "Jan 2, 2006 15:04" r.order.created}</a>
          {" "}
          <span class="status">{Order.statusLabel r.order.status}</span>
          {" "}
          <span class="total">{Text.fromInt r.total}</span>
        </li>}
    </ul>

    {if model.manager then <a href={Products.Index {}}>Products</a> else Html.none}

    <button click={App.Logout}>Log out</button>
  </Layout.Page>

--- file: app/Orders/Show.alder ---
module Orders.Show

import User
import Product
import Order
import Line
import Layout

page "/orders/{id}" { id: Order.OrderId } requires user

type Item = {
  name: Text.NonEmpty,
  quantity: Int.Positive,
  subtotal: Int,
}

type Model = {
  order: Order,
  items: List Item,
  total: Int,
  products: List Product,
}

-- The order is shown to its owner and to a Manager; to anyone else it is a 404.
load : Params -> Session -> Query Model
load params session = {
  let found <- Db.get Order params.id
  let viewer <- Db.get User session.user
  match found {
    Just order -> if order.owner == session.user || User.isManager viewer then details order else Query.notFound,
    Nothing -> Query.notFound,
  }
}

details : Order -> Query Model
details order = {
  let lines <- Db.from Line |> Db.where (Line.order.eq order.id) |> Db.all
  let products <- Db.from Product |> Db.orderBy Product.name.asc |> Db.all
  Query.succeed {
    order: order,
    items: List.filterMap (item products) lines,
    total: Line.total products lines,
    products: List.filter (\p -> p.active) products,
  }
}

item : List Product -> Line -> Maybe Item
item products line =
  List.find (\p -> p.id == line.product) products
    |> Maybe.map (toItem line)

toItem : Line -> Product -> Item
toItem line product =
  { name: product.name, quantity: line.quantity, subtotal: line.quantity * product.price }

form NewLine = {
  quantity: Int.Positive,
}

msg AddLine { product: Product.ProductId } form NewLine
msg Pay
msg Cancel

update : Msg -> Model -> Session -> Task Nav
update msg model session = match msg {
  AddLine p -> {
    let order <- current model session
    match order.status {
      Open -> addLine order p.product p.form.quantity,
      Paid -> Task.succeed (Reject "This order is no longer open."),
      Cancelled -> Task.succeed (Reject "This order is no longer open."),
    }
  },
  Pay p -> {
    let order <- current model session
    if order.status == Open then setStatus order Paid else Task.succeed (Redirect (Orders.Show { id: order.id }))
  },
  Cancel p -> {
    let order <- current model session
    if order.status == Open then setStatus order Cancelled else Task.succeed (Redirect (Orders.Show { id: order.id }))
  },
}

-- current reads the page's order again, inside the task: its status is the stored one, not the
-- one the page was rendered with, and the session's user must own it or be a Manager.
current : Model -> Session -> Task Order
current model session = {
  let found <- Db.get Order model.order.id
  let viewer <- Db.get User session.user
  match found {
    Just order -> if order.owner == session.user || User.isManager viewer then Task.succeed order else Task.notFound,
    Nothing -> Task.notFound,
  }
}

addLine : Order -> Product.ProductId -> Int.Positive -> Task Nav
addLine order productId quantity = {
  let found <- Db.get Product productId
  match found {
    Just product -> if product.active then insertLine order product quantity else Task.succeed (Reject "That product is not available."),
    Nothing -> Task.notFound,
  }
}

insertLine : Order -> Product -> Int.Positive -> Task Nav
insertLine order product quantity = {
  let _ <- Db.insert Line { order: order.id, product: product.id, quantity: quantity }
  Task.succeed (Redirect (Orders.Show { id: order.id }))
}

setStatus : Order -> Order.Status -> Task Nav
setStatus order status = {
  let _ <- Db.update Order order.id (\o -> { o | status: status })
  Task.succeed (Redirect (Orders.Show { id: order.id }))
}

view : Model -> Session -> Html Msg
view model session =
  <Layout.Page title="Order">
    <h1>Order</h1>

    <p>Status: <span id="status">{Order.statusLabel model.order.status}</span></p>

    <ul id="lines">
      {for i in model.items ->
        <li>
          <span class="name">{i.name}</span>
          {" "}
          <span class="quantity">{Text.fromInt i.quantity}</span>
          {" "}
          <span class="subtotal">{Text.fromInt i.subtotal}</span>
        </li>}
    </ul>

    <p>Total: <span id="total">{Text.fromInt model.total}</span></p>

    {if model.order.status == Open then
      <section id="controls">
        <h2>Add a line</h2>
        {for product in model.products ->
          <form submit={AddLine { product: product.id }}>
            <span class="product">{product.name}</span>
            <Errors form />
            <Input field="quantity" label="Quantity" />
            <Errors field="quantity" />
            <button>Add</button>
          </form>}
        <button click={Pay}>Mark paid</button>
        <button click={Cancel}>Cancel order</button>
      </section>
    else Html.none}

    <a href={Orders.Index {}}>Back to orders</a>
    <button click={App.Logout}>Log out</button>
  </Layout.Page>

--- file: app/Products/Index.alder ---
module Products.Index

import User
import Product
import Layout

page "/products" { } requires user

type Model = {
  products: List Product,
}

-- Only a Manager sees this page; for a Staff user it is a 404.
load : Params -> Session -> Query Model
load params session = {
  let viewer <- Db.get User session.user
  let products <- Db.from Product |> Db.orderBy Product.name.asc |> Db.all
  if User.isManager viewer then Query.succeed { products: products } else Query.notFound
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
    let _ <- manager session
    let _ <- Db.insert Product { name: p.form.name, price: p.form.price }
    Task.succeed (Redirect (Products.Index {}))
  },
  Toggle p -> {
    let _ <- manager session
    let found <- Db.get Product p.id
    match found {
      Just product -> toggle product,
      Nothing -> Task.notFound,
    }
  },
}

-- manager checks the session's user again in `update`: a message can be posted without the page.
manager : Session -> Task Unit
manager session = {
  let viewer <- Db.get User session.user
  if User.isManager viewer then Task.succeed () else Task.notFound
}

toggle : Product -> Task Nav
toggle product = {
  let _ <- Db.update Product product.id (\x -> { x | active: not x.active })
  Task.succeed (Redirect (Products.Index {}))
}

view : Model -> Session -> Html Msg
view model session =
  <Layout.Page title="Products">
    <h1>Products</h1>

    <form submit={Add}>
      <Errors form />
      <Input field="name" label="Name" />
      <Errors field="name" />
      <Input field="price" label="Price in cents" />
      <Errors field="price" />
      <button>Add product</button>
    </form>

    <ul id="products">
      {for product in model.products ->
        <li class={if product.active then "active" else "inactive"}>
          <span class="name">{product.name}</span>
          {" "}
          <span class="price">{Text.fromInt product.price}</span>
          <button click={Toggle { id: product.id }}>{if product.active then "Deactivate" else "Activate"}</button>
        </li>}
    </ul>

    <a href={Orders.Index {}}>Orders</a>
    <button click={App.Logout}>Log out</button>
  </Layout.Page>
