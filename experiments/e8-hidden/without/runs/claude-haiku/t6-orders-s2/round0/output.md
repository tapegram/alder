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

type LineId = brand Text

table Line {
  id: LineId @id,
  order: Order.OrderId @ref Order @cascade,
  product: Product.ProductId @ref Product,
  quantity: Int.Positive,
}

type LineView = { name: Text, quantity: Int, cost: Int }

-- The cost of one line: its quantity times the product's price now.
lineCost : Line -> Query Int
lineCost line = {
  let found <- Db.get Product line.product
  Query.succeed (match found { Just p -> line.quantity * p.price, Nothing -> 0 })
}

-- One line as the order page shows it.
lineView : Line -> Query LineView
lineView line = {
  let found <- Db.get Product line.product
  let name : Text = match found { Just p -> p.name, Nothing -> "(removed product)" }
  let cost <- lineCost line
  Query.succeed { name: name, quantity: line.quantity, cost: cost }
}

-- The total of an order in cents: the sum of quantity times price over its lines.
orderTotal : Order.OrderId -> Query Int
orderTotal id = {
  let lines <- Db.from Line |> Db.where (Line.order.eq id) |> Db.all
  let costs <- Query.all (List.map lineCost lines)
  Query.succeed (List.sum costs)
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

--- file: app/Orders/Index.alder ---
module Orders.Index

import Order
import Line
import User
import Layout

page "/" { } requires user

type OrderRow = { order: Order, total: Int }

type Model = {
  rows: List OrderRow,
  manager: Bool,
}

load : Params -> Session -> Query Model
load params session = {
  let orders <-
    Db.from Order
      |> Db.where (Order.owner.eq session.user)
      |> Db.orderBy Order.created.desc
      |> Db.all
  let rows <- Query.all (List.map orderRow orders)
  let manager <- User.isManager session.user
  Query.succeed { rows: rows, manager: manager }
}

orderRow : Order -> Query OrderRow
orderRow order = {
  let total <- Line.orderTotal order.id
  Query.succeed { order: order, total: total }
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
          <a href={Orders.Show { id: r.order.id }}>Order</a>
          <span class="status">{Order.statusLabel r.order.status}</span>
          <span class="total">{Text.fromInt r.total}</span>
        </li>}
    </ul>
    {if model.manager then <a href={Products.Index {}}>Products</a> else Html.none}
    <button click={App.Logout}>Log out</button>
  </Layout.Page>

--- file: app/Orders/Show.alder ---
module Orders.Show

import Order
import Line
import Product
import User
import Layout

page "/orders/{id}" { id: Order.OrderId } requires user

type Model = {
  order: Order,
  lines: List Line.LineView,
  total: Int,
  products: List Product,
}

load : Params -> Session -> Query Model
load params session = {
  let found <- Db.get Order params.id
  let manager <- User.isManager session.user
  match found {
    Just order -> if order.owner == session.user || manager then viewOf order else Query.notFound,
    Nothing -> Query.notFound,
  }
}

-- The model of an order the user may see: its lines, its total, and the active products to add.
viewOf : Order -> Query Model
viewOf order = {
  let lines <- Db.from Line |> Db.where (Line.order.eq order.id) |> Db.all
  let views <- Query.all (List.map Line.lineView lines)
  let products <-
    Db.from Product
      |> Db.where (Product.active.eq True)
      |> Db.orderBy Product.name.asc
      |> Db.all
  Query.succeed { order: order, lines: views, total: List.sum (List.map (\v -> v.cost) views), products: products }
}

form NewLine = {
  quantity: Int.Positive,
}

msg AddLine { product: Product.ProductId } form NewLine
msg Pay
msg Cancel

-- Adds a line, if the order is still Open and the product is active. The order is read again
-- here, so a form from a page loaded while the order was Open is still refused once it is Paid.
addLine : Order.OrderId -> Product.ProductId -> Int.Positive -> Task Nav
addLine orderId productId quantity = {
  let order <- Db.get Order orderId
  let product <- Db.get Product productId
  let isOpen = match order { Just o -> o.status == Open, Nothing -> False }
  let sellable = match product { Just pr -> pr.active, Nothing -> False }
  if not isOpen then Task.succeed (Reject "This order is no longer open.") else if not sellable then Task.succeed (Reject "That product is not for sale.") else Task.map (\u -> Redirect (Orders.Show { id: orderId })) (Db.insert Line { order: orderId, product: productId, quantity: quantity })
}

-- Marks an Open order with the given status; an order that is no longer Open is refused.
setStatus : Order.OrderId -> Order.Status -> Task Nav
setStatus id next = {
  let found <- Db.get Order id
  let isOpen = match found { Just o -> o.status == Open, Nothing -> False }
  if isOpen then Task.map (\u -> Redirect (Orders.Show { id: id })) (Db.update Order id (\o -> { o | status: next })) else Task.succeed (Reject "This order is no longer open.")
}

update : Msg -> Model -> Session -> Task Nav
update msg model session = match msg {
  AddLine p -> addLine model.order.id p.product p.form.quantity,
  Pay p -> setStatus model.order.id Paid,
  Cancel p -> setStatus model.order.id Cancelled,
}

AddForms : { products: List Product } -> Html Msg
AddForms props =
  <ul>
    {for p in props.products ->
      <li>
        <form submit={AddLine { product: p.id }}>
          <Errors form />
          <span>{p.name}</span>
          <Input field="quantity" label="Quantity" />
          <Errors field="quantity" />
          <button>Add</button>
        </form>
      </li>}
  </ul>

-- The forms and buttons of an order; they are shown only while the order is Open.
Actions : { isOpen: Bool, products: List Product } -> Html Msg
Actions props =
  <div>
    {if props.isOpen then <AddForms products={props.products} /> else Html.none}
    {if props.isOpen then <button click={Pay}>Mark paid</button> else Html.none}
    {if props.isOpen then <button click={Cancel}>Cancel order</button> else Html.none}
  </div>

view : Model -> Session -> Html Msg
view model session =
  <Layout.Page title="Order">
    <h1>Order</h1>
    <p>Status: <span id="status">{Order.statusLabel model.order.status}</span></p>
    <ul id="lines">
      {for l in model.lines ->
        <li>
          <span class="name">{l.name}</span>
          <span class="quantity">{Text.fromInt l.quantity}</span>
          <span class="cost">{Text.fromInt l.cost}</span>
        </li>}
    </ul>
    <p>Total: <span id="total">{Text.fromInt model.total}</span></p>
    <Actions isOpen={model.order.status == Open} products={model.products} />
    <a href={Orders.Index {}}>Back</a>
  </Layout.Page>

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
import User
import Layout

page "/products" { } requires user

type Model = { products: List Product }

-- Only a Manager may see the products page; anyone else gets a 404.
load : Params -> Session -> Query Model
load params session = {
  let manager <- User.isManager session.user
  if manager then productsModel else Query.notFound
}

productsModel : Query Model
productsModel = {
  let products <- Db.from Product |> Db.orderBy Product.name.asc |> Db.all
  Query.succeed { products: products }
}

form NewProduct = {
  name: Text.NonEmpty,
  price: Int.NonNegative,
}

msg Add form NewProduct
msg Toggle { id: Product.ProductId }

-- Every message of this page is a Manager's; `update` checks it again, since a message can be
-- posted without the page.
requireManager : Session -> Task Unit
requireManager session = {
  let manager <- User.isManager session.user
  if manager then Task.succeed () else Task.notFound
}

update : Msg -> Model -> Session -> Task Nav
update msg model session = match msg {
  Add p -> {
    let _ <- requireManager session
    let _ <- Db.insert Product { name: p.form.name, price: p.form.price, active: True }
    Task.succeed (Redirect (Products.Index {}))
  },
  Toggle p -> {
    let _ <- requireManager session
    let _ <- Db.update Product p.id (\pr -> { pr | active: not pr.active })
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
      <Errors field="name" />
      <Input field="price" label="Price (cents)" />
      <Errors field="price" />
      <button>Add product</button>
    </form>
    <ul id="products">
      {for p in model.products ->
        <li>
          <span class="name">{p.name}</span>
          <span class="price">{Text.fromInt p.price}</span>
          {if p.active then <span class="active">active</span> else <span class="active">inactive</span>}
          <button click={Toggle { id: p.id }}>{if p.active then "Deactivate" else "Activate"}</button>
        </li>}
    </ul>
    <nav>
      <a href={Orders.Index {}}>Orders</a>
    </nav>
    <button click={App.Logout}>Log out</button>
  </Layout.Page>

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

-- Whether the user may manage products and see every order.
isManager : UserId -> Query Bool
isManager id = {
  let found <- Db.get User id
  Query.succeed (match found { Just u -> u.role == Manager, Nothing -> False })
}
