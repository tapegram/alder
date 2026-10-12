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

-- The role of a user. A session whose user has no row gets the least rights.
roleOf : UserId -> Query Role
roleOf id = {
  let found <- Db.get User id
  match found {
    Just u -> Query.succeed u.role,
    Nothing -> Query.succeed Staff,
  }
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

-- What one line costs in cents: its quantity times its product's price (0 if the product is gone).
lineCost : List Product -> Line -> Int
lineCost products line = {
  let found = products |> List.find (\p -> p.id == line.product)
  found |> Maybe.map (\p -> p.price * line.quantity) |> Maybe.withDefault 0
}

-- The total in cents of an order's lines.
totalOf : Order.OrderId -> Query Int
totalOf id = {
  let lines <- Db.from Line |> Db.where (Line.order.eq id) |> Db.all
  let products <- Db.from Product |> Db.all
  Query.succeed (List.sum (List.map (lineCost products) lines))
}

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

--- file: app/Orders/Index.alder ---
module Orders.Index

import Order
import Line
import Layout

page "/" { } requires user

type Row = { order: Order, total: Int }

type Model = { rows: List Row }

-- An order with its total in cents.
withTotal : Order -> Query Row
withTotal order = {
  let total <- Line.totalOf order.id
  Query.succeed { order: order, total: total }
}

load : Params -> Session -> Query Model
load params session = {
  let orders <-
    Db.from Order
      |> Db.where (Order.owner.eq session.user)
      |> Db.orderBy Order.created.desc
      |> Db.all
  let rows <- Query.all (List.map withTotal orders)
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
          <a href={Orders.Show { id: row.order.id }}>View</a>
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
import Layout

page "/orders/{id}" { id: Order.OrderId } requires user

type Model = {
  order: Order,
  lines: List Line,
  products: List Product,
  sellable: List Product,
  total: Int,
}

form NewLine = {
  quantity: Int.Positive,
}

msg AddLine { product: Product.ProductId } form NewLine
msg Pay
msg Cancel

-- A Manager may act on any order; anyone else only on their own.
mayAct : User.Role -> User.UserId -> Order -> Bool
mayAct role me order = order.owner == me || role == Manager

-- The order a message names, if the session's user may act on it; otherwise a 404. It is read
-- again here, because `load` ran earlier and the order may have changed since.
current : Order.OrderId -> Session -> Task Order
current id session = {
  let found <- Db.get Order id
  let role <- User.roleOf session.user
  match found {
    Just order -> if mayAct role session.user order then Task.succeed order else Task.notFound,
    Nothing -> Task.notFound,
  }
}

-- Only an Open order takes lines or changes. The order exists and the user may see it, so this
-- is a failure, not a 404. The page shows no form for a Paid order, so reaching here means a
-- stale page or a forged request. Task.fail is used because the state, not the input, refuses it.
requireOpen : Order -> Task Unit
requireOpen order =
  if order.status == Open then Task.succeed () else Task.fail "order is not open"

-- A line's product must exist (else 404) and be for sale (else a failure, as above).
requireSellable : Maybe Product -> Task Unit
requireSellable found = match found {
  Just product -> if product.active then Task.succeed () else Task.fail "product is not for sale",
  Nothing -> Task.notFound,
}

-- The name shown for a line's product.
nameOf : List Product -> Line -> Text
nameOf products line = {
  let found = products |> List.find (\p -> p.id == line.product)
  match found {
    Just p -> Text.trim p.name,
    Nothing -> "unknown product",
  }
}

-- The page's model for an order the user may see.
modelOf : Order -> Query Model
modelOf order = {
  let lines <- Db.from Line |> Db.where (Line.order.eq order.id) |> Db.all
  let products <- Db.from Product |> Db.all
  let sellable = List.filter (\p -> p.active) products
  let total = List.sum (List.map (Line.lineCost products) lines)
  Query.succeed { order: order, lines: lines, products: products, sellable: sellable, total: total }
}

load : Params -> Session -> Query Model
load params session = {
  let found <- Db.get Order params.id
  let role <- User.roleOf session.user
  match found {
    Just order -> if mayAct role session.user order then modelOf order else Query.notFound,
    Nothing -> Query.notFound,
  }
}

update : Msg -> Model -> Session -> Task Nav
update msg model session = match msg {
  AddLine p -> {
    let order <- current model.order.id session
    let _ <- requireOpen order
    let found <- Db.get Product p.product
    let _ <- requireSellable found
    let _ <- Db.insert Line { order: order.id, product: p.product, quantity: p.form.quantity }
    Task.succeed (Redirect (Orders.Show { id: order.id }))
  },
  Pay p -> {
    let order <- current model.order.id session
    let _ <- requireOpen order
    let _ <- Db.update Order order.id (\o -> { o | status: Paid })
    Task.succeed (Redirect (Orders.Show { id: order.id }))
  },
  Cancel p -> {
    let order <- current model.order.id session
    let _ <- requireOpen order
    let _ <- Db.update Order order.id (\o -> { o | status: Cancelled })
    Task.succeed (Redirect (Orders.Show { id: order.id }))
  },
}

view : Model -> Session -> Html Msg
view model session =
  <Layout.Page title="Order">
    <p><a href={Orders.Index {}}>All orders</a></p>
    <h1>Order</h1>
    <p>Status: <span id="status">{Order.statusLabel model.order.status}</span></p>
    <ul id="lines">
      {for line in model.lines -> <li>{nameOf model.products line} x {Text.fromInt line.quantity} = {Text.fromInt (Line.lineCost model.products line)}</li>}
    </ul>
    <p>Total: <span id="total">{Text.fromInt model.total}</span></p>
    {if model.order.status == Open then
      <div>
        <h2>Add a line</h2>
        {for p in model.sellable ->
          <form submit={AddLine { product: p.id }}>
            <Errors form />
            <Input field="quantity" label="Quantity" />
            <Errors field="quantity" />
            <button>Add {p.name}</button>
          </form>}
        <button click={Pay}>Mark paid</button>
        <button click={Cancel}>Cancel order</button>
      </div>
    else Html.none}
  </Layout.Page>

-- Refused with Task.fail, not Task.notFound: the order exists and belongs to the user, so it is
-- not a 404; its Paid state refuses the line. The page shows no form for a Paid order, so this
-- is a stale page or a forged request.
test "adding a line to a Paid order is refused" = {
  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
  let tea <- Db.insert Product { name: Text.NonEmpty "Tea", price: Int.NonNegative 300 }
  let order <- Db.insert Order { owner: me, status: Paid }
  let model <- load { id: order } { user: me }
  let r <- Test.run (update (AddLine { product: tea, form: { quantity: Int.Positive 1 } }) model { user: me })
  let lines <- Db.from Line |> Db.count
  Task.succeed ({ value: r.value, lines: lines } == { value: Err "order is not open", lines: 0 })
}

test "an open order's total is the sum over its lines" = {
  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
  let tea <- Db.insert Product { name: Text.NonEmpty "Tea", price: Int.NonNegative 300 }
  let cake <- Db.insert Product { name: Text.NonEmpty "Cake", price: Int.NonNegative 550 }
  let order <- Db.insert Order { owner: me }
  let _ <- Db.insert Line { order: order, product: tea, quantity: Int.Positive 2 }
  let _ <- Db.insert Line { order: order, product: cake, quantity: Int.Positive 1 }
  let _ <- Test.login { user: Just me }
  let page <- Test.get (Orders.Show { id: order })
  Task.succeed ({ total: Test.text "#total" page.doc, status: Test.text "#status" page.doc } == { total: Just "1150", status: Just "Open" })
}

test "a Staff user does not find another user's order" = {
  let owen <- Db.insert User { email: Text.Email "owen@example.com", name: Text.NonEmpty "Owen", passwordHash: "" }
  let sam <- Db.insert User { email: Text.Email "sam@example.com", name: Text.NonEmpty "Sam", passwordHash: "" }
  let order <- Db.insert Order { owner: owen }
  let _ <- Test.login { user: Just sam }
  let r <- Test.open (Orders.Show { id: order })
  Task.succeed (r.answer == PageMissing)
}

test "a Manager opens another user's order" = {
  let owen <- Db.insert User { email: Text.Email "owen@example.com", name: Text.NonEmpty "Owen", passwordHash: "" }
  let mia <- Db.insert User { email: Text.Email "mia@example.com", name: Text.NonEmpty "Mia", passwordHash: "", role: Manager }
  let order <- Db.insert Order { owner: owen }
  let _ <- Test.login { user: Just mia }
  let page <- Test.get (Orders.Show { id: order })
  Task.succeed (Test.text "#status" page.doc == Just "Open")
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
      let products <- Db.from Product |> Db.all
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

-- Only a Manager changes products; a Staff user's request is a 404, as the page is.
requireManager : Session -> Task Unit
requireManager session = {
  let role <- User.roleOf session.user
  if role == Manager then Task.succeed () else Task.notFound
}

requireFound : Maybe Product -> Task Unit
requireFound found = match found {
  Just _ -> Task.succeed (),
  Nothing -> Task.notFound,
}

update : Msg -> Model -> Session -> Task Nav
update msg model session = match msg {
  Add p -> {
    let _ <- requireManager session
    let _ <- Db.insert Product { name: p.form.name, price: p.form.price }
    Task.succeed (Redirect (Products.Index {}))
  },
  Toggle p -> {
    let _ <- requireManager session
    let found <- Db.get Product p.id
    let _ <- requireFound found
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
          <span class="state">{if p.active then "active" else "inactive"}</span>
          <button click={Toggle { id: p.id }}>{if p.active then "Deactivate" else "Activate"}</button>
        </li>}
    </ul>
  </Layout.Page>

test "a Staff user is not shown the products" = {
  let sam <- Db.insert User { email: Text.Email "sam@example.com", name: Text.NonEmpty "Sam", passwordHash: "" }
  let _ <- Test.login { user: Just sam }
  let r <- Test.open (Products.Index {})
  Task.succeed (r.answer == PageMissing)
}

test "a Manager sees the product list" = {
  let mia <- Db.insert User { email: Text.Email "mia@example.com", name: Text.NonEmpty "Mia", passwordHash: "", role: Manager }
  let _ <- Db.insert Product { name: Text.NonEmpty "Tea", price: Int.NonNegative 300 }
  let _ <- Test.login { user: Just mia }
  let page <- Test.get (Products.Index {})
  Task.succeed ({ names: Test.texts ".name" page.doc } == { names: ["Tea"] })
}
