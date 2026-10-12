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

priceOf : Product.ProductId -> List Product -> Int
priceOf id products = {
  let found = List.find (\p -> p.id == id) products
  match found {
    Just p -> Int.abs p.price,
    Nothing -> 0,
  }
}

nameOf : Product.ProductId -> List Product -> Text
nameOf id products = {
  let found = List.find (\p -> p.id == id) products
  match found {
    Just p -> Text.trim p.name,
    Nothing -> "?",
  }
}

lineTotal : Line -> List Product -> Int
lineTotal line products = line.quantity * priceOf line.product products

total : List Line -> List Product -> Int
total lines products = List.sum (List.map (\l -> lineTotal l products) lines)

--- file: app/Auth/Login.alder ---
module Auth.Login

import User
import Layout

-- Password checking is outside the language: a capability implemented in Go.
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

type Entry = { order: Order, total: Int }

type Model = { entries: List Entry }

entry : Order -> List Line -> List Product -> Entry
entry o lines products = {
  order: o,
  total: Line.total (List.filter (\l -> l.order == o.id) lines) products,
}

load : Params -> Session -> Query Model
load params session = {
  let orders <-
    Db.from Order
      |> Db.where (Order.owner.eq session.user)
      |> Db.orderBy Order.created.desc
      |> Db.all
  let ids = List.map (\o -> o.id) orders
  let lines <- Db.from Line |> Db.where (Line.order.in ids) |> Db.all
  let products <- Db.from Product |> Db.all
  Query.succeed { entries: List.map (\o -> entry o lines products) orders }
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
      {for e in model.entries ->
        <li>
          <a href={Orders.Show { id: e.order.id }}>{Time.format "Jan 2 15:04:05" e.order.created}</a>
          {" "}
          <span class="status">{Order.statusLabel e.order.status}</span>
          {" "}
          <span class="total">{Text.fromInt e.total}</span>
        </li>}
    </ul>
    <nav>
      <a href={Products.Index {}}>Products</a>
    </nav>
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
  lines: List Line,
  products: List Product,
}

load : Params -> Session -> Query Model
load params session = {
  let found <- Db.get Order params.id
  let me <- Db.get User session.user
  let lines <- Db.from Line |> Db.where (Line.order.eq params.id) |> Db.all
  let products <- Db.from Product |> Db.orderBy Product.name.asc |> Db.all
  let isManager = match me {
    Just u -> u.role == Manager,
    Nothing -> False,
  }
  match found {
    Just order ->
      if order.owner == session.user || isManager
      then Query.succeed { order: order, lines: lines, products: products }
      else Query.notFound,
    Nothing -> Query.notFound,
  }
}

form NewLine = { quantity: Int.Positive }

msg AddLine { product: Product.ProductId } form NewLine
msg Pay
msg Cancel

update : Msg -> Model -> Session -> Task Nav
update msg model session = match msg {
  AddLine p ->
    if model.order.status == Open
    then addLine model p.product p.form.quantity
    else Task.succeed (Reject "Only an open order can change."),
  Pay p -> setStatus model Paid,
  Cancel p -> setStatus model Cancelled,
}

addLine : Model -> Product.ProductId -> Int.Positive -> Task Nav
addLine model productId qty = {
  let found <- Db.get Product productId
  match found {
    Just pr -> checkActive model pr qty,
    Nothing -> Task.notFound,
  }
}

checkActive : Model -> Product -> Int.Positive -> Task Nav
checkActive model pr qty =
  if pr.active
  then insertLine model pr qty
  else Task.succeed (Reject "That product is not available.")

insertLine : Model -> Product -> Int.Positive -> Task Nav
insertLine model pr qty = {
  let _ <- Db.insert Line { order: model.order.id, product: pr.id, quantity: qty }
  Task.succeed (Redirect (Orders.Show { id: model.order.id }))
}

setStatus : Model -> Order.Status -> Task Nav
setStatus model status =
  if model.order.status == Open
  then changeStatus model status
  else Task.succeed (Reject "Only an open order can change.")

changeStatus : Model -> Order.Status -> Task Nav
changeStatus model status = {
  let _ <- Db.update Order model.order.id (\o -> { o | status: status })
  Task.succeed (Redirect (Orders.Show { id: model.order.id }))
}

view : Model -> Session -> Html Msg
view model session =
  <Layout.Page title="Order">
    <h1>Order</h1>
    <p>Status: <span id="status">{Order.statusLabel model.order.status}</span></p>
    <p>Total: <span id="total">{Text.fromInt (Line.total model.lines model.products)}</span></p>
    <ul id="lines">
      {for line in model.lines ->
        <li>{Line.nameOf line.product model.products} x {Text.fromInt line.quantity}</li>}
    </ul>
    {if model.order.status == Open then <Controls model={model} /> else Html.none}
    <a href={Orders.Index {}}>Back</a>
  </Layout.Page>

Controls : { model: Model } -> Html Msg
Controls props =
  <div>
    {for p in List.filter (\x -> x.active) props.model.products ->
      <form submit={AddLine { product: p.id }}>
        <strong>{p.name}</strong>
        <Errors form />
        <Input field="quantity" label="Quantity" />
        <Errors field="quantity" />
        <button>Add</button>
      </form>}
    <button click={Pay}>Mark paid</button>
    <button click={Cancel}>Cancel order</button>
  </div>

test "an open order's total sums its lines" = {
  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
  let pen <- Db.insert Product { name: Text.NonEmpty "Pen", price: Int.NonNegative 500 }
  let pad <- Db.insert Product { name: Text.NonEmpty "Pad", price: Int.NonNegative 250 }
  let o <- Db.insert Order { owner: me }
  let _ <- Db.insert Line { order: o, product: pen, quantity: Int.Positive 2 }
  let _ <- Db.insert Line { order: o, product: pad, quantity: Int.Positive 1 }
  let _ <- Test.login { user: Just me }
  let page <- Test.get (Orders.Show { id: o })
  Task.succeed ({ status: Test.text "#status" page.doc, total: Test.text "#total" page.doc } == { status: Just "Open", total: Just "1250" })
}

test "adding a line to a paid order is refused" = {
  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
  let pen <- Db.insert Product { name: Text.NonEmpty "Pen", price: Int.NonNegative 500 }
  let o <- Db.insert Order { owner: me }
  let _ <- Test.login { user: Just me }
  let _ <- Test.get (Orders.Show { id: o })
  let _ <- Db.update Order o (\x -> { x | status: Paid })
  let _ <- Test.submit (AddLine { product: pen }) { quantity: Int.Positive 1 }
  let n <- Db.from Line |> Db.count
  Task.succeed (n == 0)
}

test "a staff user cannot open another user's order" = {
  let owner <- Db.insert User { email: Text.Email "owner@example.com", name: Text.NonEmpty "Owner", passwordHash: "" }
  let staff <- Db.insert User { email: Text.Email "staff@example.com", name: Text.NonEmpty "Staff", passwordHash: "" }
  let o <- Db.insert Order { owner: owner }
  let _ <- Test.login { user: Just staff }
  let r <- Test.open (Orders.Show { id: o })
  Task.succeed (r.answer == PageMissing)
}

test "a manager can open another user's order" = {
  let owner <- Db.insert User { email: Text.Email "owner@example.com", name: Text.NonEmpty "Owner", passwordHash: "" }
  let boss <- Db.insert User { email: Text.Email "boss@example.com", name: Text.NonEmpty "Boss", passwordHash: "", role: Manager }
  let o <- Db.insert Order { owner: owner }
  let _ <- Test.login { user: Just boss }
  let r <- Test.open (Orders.Show { id: o })
  Task.succeed (r.answer == Shown)
}

--- file: app/Products/Index.alder ---
module Products.Index

import Product
import User
import Layout

page "/products" { } requires user

type Model = { products: List Product }

load : Params -> Session -> Query Model
load params session = {
  let me <- Db.get User session.user
  let products <- Db.from Product |> Db.orderBy Product.name.asc |> Db.all
  match me {
    Just u ->
      if u.role == Manager
      then Query.succeed { products: products }
      else Query.notFound,
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
      Just pr -> flipActive pr,
      Nothing -> Task.notFound,
    }
  },
}

flipActive : Product -> Task Nav
flipActive pr = {
  let _ <- Db.update Product pr.id (\x -> { x | active: not x.active })
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
      <Input field="price" label="Price (cents)" />
      <Errors field="price" />
      <button>Add</button>
    </form>
    <ul id="products">
      {for p in model.products ->
        <li>
          <span class="name">{p.name}</span>
          {" "}
          <span class="price">{Text.fromInt p.price}</span>
          {" "}
          <span class="state">{if p.active then "active" else "inactive"}</span>
          {" "}
          <button click={Toggle { id: p.id }}>{if p.active then "Deactivate" else "Activate"}</button>
        </li>}
    </ul>
    <a href={Orders.Index {}}>Back</a>
  </Layout.Page>
