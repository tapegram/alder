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
isManager found = match found {
  Just user -> user.role == Manager,
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
      <link rel="stylesheet" href={Url.external "/static/app.css"} />
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

--- file: app/Auth/Login.alder ---
module Auth.Login

import User
import Layout

-- Password checking is outside the language: a capability implemented in Go.
capability Password {
  verify : { email: Text.Email, password: Text } -> Task (Maybe User.UserId),
}

-- In a test, `Test.fake Password` accepts any user who has a row, whatever the password.
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
import Line
import Product
import Layout

page "/" { } requires user

type Row = { order: Order, total: Int }

type Model = { orders: List Row }

row : List Product -> List Line -> Order -> Row
row products lines o = {
  order: o,
  total: Line.total products (List.filter (\l -> l.order == o.id) lines),
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
  Query.succeed { orders: List.map (row products lines) orders }
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
          <a href={Orders.Show { id: r.order.id }}>{Time.format "2006-01-02 15:04:05" r.order.created}</a>
          <span class="status">{Order.statusLabel r.order.status}</span>
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
import User
import Layout

page "/orders/{id}" { id: Order.OrderId } requires user

type Entry = { name: Text.NonEmpty, quantity: Int.Positive, subtotal: Int }

type Model = {
  order: Order,
  entries: List Entry,
  total: Int,
  products: List Product,
}

mkEntry : Line -> Product -> Entry
mkEntry line p = { name: p.name, quantity: line.quantity, subtotal: line.quantity * p.price }

entryOf : List Product -> Line -> Maybe Entry
entryOf products line =
  Maybe.map (mkEntry line) (List.find (\p -> p.id == line.product) products)

build : Order -> Query Model
build order = {
  let lines <- Db.from Line |> Db.where (Line.order.eq order.id) |> Db.all
  let products <- Db.from Product |> Db.all
  let active <- Db.from Product |> Db.where (Product.active.eq True) |> Db.orderBy Product.name.asc |> Db.all
  Query.succeed {
    order: order,
    entries: List.filterMap (entryOf products) lines,
    total: Line.total products lines,
    products: active,
  }
}

load : Params -> Session -> Query Model
load params session = {
  let found <- Db.get Order params.id
  match found {
    Nothing -> Query.notFound,
    Just order -> {
      let me <- Db.get User session.user
      if order.owner == session.user || User.isManager me then build order else Query.notFound
    },
  }
}

form NewLine = {
  quantity: Int.Positive,
}

msg AddLine { product: Product.ProductId } form NewLine
msg Pay
msg Cancel

backToOrder : Model -> Nav
backToOrder model = Redirect (Orders.Show { id: model.order.id })

insertLine : Model -> Product.ProductId -> Int.Positive -> Task Nav
insertLine model productId quantity = {
  let found <- Db.get Product productId
  match found {
    Nothing -> Task.notFound,
    Just prod -> {
      if prod.active then addLine model prod quantity else Task.succeed (Reject "This product is not available.")
    },
  }
}

addLine : Model -> Product -> Int.Positive -> Task Nav
addLine model prod quantity = {
  let _ <- Db.insert Line { order: model.order.id, product: prod.id, quantity: quantity }
  Task.succeed (backToOrder model)
}

applyStatus : Model -> Order.Status -> Task Nav
applyStatus model status = {
  let _ <- Db.update Order model.order.id (\o -> { o | status: status })
  Task.succeed (backToOrder model)
}

setStatus : Model -> Order.Status -> Task Nav
setStatus model status =
  if model.order.status != Open
    then Task.succeed (Reject "This order is not open.")
    else applyStatus model status

update : Msg -> Model -> Session -> Task Nav
update msg model session = match msg {
  AddLine p ->
    if model.order.status != Open
      then Task.succeed (Reject "This order is not open.")
      else insertLine model p.product p.form.quantity,
  Pay p -> setStatus model Paid,
  Cancel p -> setStatus model Cancelled,
}

view : Model -> Session -> Html Msg
view model session =
  <Layout.Page title="Order">
    <h1>Order</h1>
    <p>Status: <span id="status">{Order.statusLabel model.order.status}</span></p>

    <ul id="lines">
      {for e in model.entries ->
        <li>
          <span class="name">{e.name}</span> x <span class="quantity">{Text.fromInt e.quantity}</span> = <span class="subtotal">{Text.fromInt e.subtotal}</span>
        </li>}
    </ul>

    <p>Total: <span id="total">{Text.fromInt model.total}</span></p>

    {if model.order.status == Open then
      <div id="actions">
        {for p in model.products ->
          <form submit={AddLine { product: p.id }}>
            <span class="product">{p.name}</span>
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

--- file: app/Products/Index.alder ---
module Products.Index

import Product
import User
import Layout

page "/products" { } requires user

type Model = { products: List Product }

listAll : Query Model
listAll = {
  let products <- Db.from Product |> Db.orderBy Product.name.asc |> Db.all
  Query.succeed { products: products }
}

load : Params -> Session -> Query Model
load params session = {
  let me <- Db.get User session.user
  if User.isManager me then listAll else Query.notFound
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
      Nothing -> Task.notFound,
      Just prod -> {
        let _ <- Db.update Product prod.id (\x -> { x | active: not x.active })
        Task.succeed (Redirect (Products.Index {}))
      },
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
      {for p in model.products ->
        <li>
          <span class="name">{p.name}</span>
          <span class="price">{Text.fromInt p.price}</span>
          <span class="active">{if p.active then "active" else "inactive"}</span>
          <button click={Toggle { id: p.id }}>{if p.active then "Deactivate" else "Activate"}</button>
        </li>}
    </ul>

    <a href={Orders.Index {}}>Orders</a>
  </Layout.Page>
