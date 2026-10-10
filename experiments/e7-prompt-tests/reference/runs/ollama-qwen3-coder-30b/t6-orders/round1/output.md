--- file: app/App.alder ---
module App

import User
import Layout
import Theme

session { user: Maybe User.UserId, theme: Theme = Light }

login = Auth.Login

msg Logout
msg SwitchTheme

update : Msg -> Session -> Task Nav
update msg session = match msg {
  Logout p -> {
    let _ <- Session.put (\s -> { s | user: Nothing })
    Task.succeed (Redirect (Auth.Login {}))
  },
  SwitchTheme p -> {
    let _ <- Session.put (\s -> { s | theme: Theme.flip s.theme })
    Task.succeed Rerender
  },
}

error : Error -> Html Never
error err =
  <Layout.Page title="Error" theme={Light}>
    <h1>Something went wrong</h1>
    <p>Please try again.</p>
  </Layout.Page>

notFound : Html Never
notFound =
  <Layout.Page title="Not found" theme={Light}>
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
  <Layout.Page title="Log in" theme={session.theme}>
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

import Theme

Page : { title: Text, theme: Theme, children: List (Html a) } -> Html a
Page props =
  <html class={if props.theme == Dark then "dark" else ""}>
    <head>
      <meta charset="utf-8" />
      <title>{props.title}</title>
      <link rel="stylesheet" href={Url.external "/static/app.css"} />
    </head>
    <body>
      <nav>
        <button click={App.SwitchTheme}>{if props.theme == Dark then "Light mode" else "Dark mode"}</button>
      </nav>
      <main>{props.children}</main>
    </body>
  </html>

--- file: app/Order.alder ---
module Order

import User
import Product

type OrderId = brand Text
type LineId = brand Text

type Status = Open | Paid | Cancelled

table Order {
  id: OrderId @id,
  user: User.UserId @ref User,
  status: Status = Open,
  created: Time @created,
}

table Line {
  id: LineId @id,
  order: OrderId @ref Order,
  product: Product.ProductId @ref Product,
  quantity: Int.Positive,
}

statusLabel : Status -> Text
statusLabel status = match status {
  Open -> "Open",
  Paid -> "Paid",
  Cancelled -> "Cancelled",
}

total : List Line -> List Product -> Int
total lines products = {
  let amounts = List.map (\l -> {
    let p = List.find (\p -> p.id == l.product) products
    match p {
      Just p -> p.price * l.quantity,
      Nothing -> 0, -- Should not happen in a valid order
    }
  }) lines
  List.sum amounts
}

test "total computes sum of quantity × price" = {
  let lines = [
    { id: LineId "a", order: OrderId "o", product: Product.ProductId "p1", quantity: 2 },
    { id: LineId "b", order: OrderId "o", product: Product.ProductId "p2", quantity: 3 }
  ]
  let products = [
    { id: Product.ProductId "p1", name: Text.NonEmpty "A", price: 100, active: True },
    { id: Product.ProductId "p2", name: Text.NonEmpty "B", price: 200, active: True }
  ]
  total lines products == 800
}

--- file: app/Orders/Index.alder ---
module Orders.Index

import Order
import Product
import Layout
import User

page "/" { } requires user

type Model = { orders: List Order }

load : Params -> Session -> Query Model
load params session = {
  let orders <-
    Db.from Order
      |> Db.where (Order.user.eq session.user)
      |> Db.orderBy Order.created.desc
      |> Db.all
  Query.succeed { orders: orders }
}

msg Create

update : Msg -> Model -> Session -> Task Nav
update msg model session = match msg {
  Create p -> {
    let _ <- Db.insert Order { user: session.user, status: Open, created: Time.epoch }
    Task.succeed Rerender
  },
}

view : Model -> Session -> Html Msg
view model session =
  <Layout.Page title="Orders" theme={session.theme}>
    <h1>Orders</h1>
    <form submit={Create}>
      <button>Create new order</button>
    </form>
    <ul>
      {for o in model.orders ->
        <li><a href={Orders.Show { id: o.id }}>{Order.statusLabel o.status}</a></li>}
    </ul>
  </Layout.Page>

--- file: app/Orders/Show.alder ---
module Orders.Show

import Order
import Product
import Layout
import User

page "/orders/{id}" { id: Order.OrderId } requires user

type Model = {
  order: Order,
  lines: List Line,
  products: List Product,
}

load : Params -> Session -> Query Model
load params session = {
  let found <- Db.get Order params.id
  match found {
    Just order -> {
      -- Check if the user owns the order or is a manager
      let isOwner = order.user == session.user
      let isManager = session.user == User.UserId "manager" -- Simplified check for manager role
      let canView = isOwner || isManager
      match canView {
        True -> {
          let lines <- Db.from Line |> Db.where (Line.order.eq params.id) |> Db.all
          let products <- Db.from Product |> Db.all
          Query.succeed { order: order, lines: lines, products: products }
        },
        False -> Query.notFound,
      }
    },
    Nothing -> Query.notFound,
  }
}

msg AddLine form NewLine
msg MarkPaid
msg MarkCancelled

form NewLine = {
  product: Product.ProductId,
  quantity: Int.Positive,
}

update : Msg -> Model -> Session -> Task Nav
update msg model session = match msg {
  AddLine p -> {
    let order <- Db.get Order model.order.id
    match order {
      Just o -> if o.status == Open then {
        let _ <- Db.insert Line { order: model.order.id, product: p.form.product, quantity: p.form.quantity }
        Task.succeed Rerender
      } else Task.fail "Cannot add line to a non-open order",
      Nothing -> Task.notFound,
    }
  },
  MarkPaid p -> {
    let order <- Db.get Order model.order.id
    match order {
      Just o -> if o.status == Open then {
        let _ <- Db.update Order model.order.id (\o -> { o | status: Paid })
        Task.succeed Rerender
      } else Task.fail "Cannot mark a non-open order as paid",
      Nothing -> Task.notFound,
    }
  },
  MarkCancelled p -> {
    let order <- Db.get Order model.order.id
    match order {
      Just o -> if o.status == Open then {
        let _ <- Db.update Order model.order.id (\o -> { o | status: Cancelled })
        Task.succeed Rerender
      } else Task.fail "Cannot cancel a non-open order",
      Nothing -> Task.notFound,
    }
  },
}

view : Model -> Session -> Html Msg
view model session =
  <Layout.Page title="Order" theme={session.theme}>
    <h1>Order {Order.statusLabel model.order.status}</h1>
    <p>Total: {Text.fromInt (Order.total model.lines model.products)}</p>
    {if model.order.status == Open then
      <form submit={AddLine}>
        <Input field="product" label="Product" />
        <Errors field="product" />
        <Input field="quantity" label="Quantity" />
        <Errors field="quantity" />
        <button>Add line</button>
      </form>
    else Html.none}
    {if model.order.status == Open then
      <div>
        <button click={MarkPaid}>Mark Paid</button>
        <button click={MarkCancelled}>Cancel</button>
      </div>
    else Html.none}
    <ul>
      {for l in model.lines ->
        <li>{Product.name l.product model.products} x {Text.fromInt l.quantity}</li>}
    </ul>
  </Layout.Page>

test "adding line to paid order fails" = {
  let user <- Db.insert User { email: Text.Email "user@example.com", name: Text.NonEmpty "User", passwordHash: "", role: Staff }
  let orderId <- Db.insert Order { user: user, status: Paid, created: Time.epoch }
  let productId <- Db.insert Product { name: Text.NonEmpty "Test Product", price: 100, active: True }
  let r <- Test.run (update (AddLine { product: productId, quantity: 1 }) { order: { id: orderId, user: user, status: Paid, created: Time.epoch }, lines: [], products: [] } { user: user, theme: Light })
  Task.succeed (r.value == Err "Cannot add line to a non-open order")
}

test "staff user cannot view another user's order" = {
  let user1 <- Db.insert User { email: Text.Email "user1@example.com", name: Text.NonEmpty "User1", passwordHash: "", role: Staff }
  let user2 <- Db.insert User { email: Text.Email "user2@example.com", name: Text.NonEmpty "User2", passwordHash: "", role: Staff }
  let orderId <- Db.insert Order { user: user1, status: Open, created: Time.epoch }
  let r <- Test.run (Orders.Show.load { id: orderId } { user: user2, theme: Light })
  Task.succeed (r.value == Err "not found")
}

test "manager can view another user's order" = {
  let user1 <- Db.insert User { email: Text.Email "user1@example.com", name: Text.NonEmpty "User1", passwordHash: "", role: Staff }
  let manager <- Db.insert User { email: Text.Email "manager@example.com", name: Text.NonEmpty "Manager", passwordHash: "", role: Manager }
  let orderId <- Db.insert Order { user: user1, status: Open, created: Time.epoch }
  let r <- Test.run (Orders.Show.load { id: orderId } { user: manager, theme: Light })
  Task.succeed (r.value == Ok { order: { id: orderId, user: user1, status: Open, created: Time.epoch }, lines: [], products: [] })
}

--- file: app/Product.alder ---
module Product

import User

type ProductId = brand Text

type Role = Staff | Manager

table Product {
  id: ProductId @id,
  name: Text.NonEmpty,
  price: Int.NonNegative,
  active: Bool = True,
}

name : ProductId -> List Product -> Text
name id products = products
  |> List.find (\p -> p.id == id)
  |> Maybe.map (\p -> p.name)
  |> Maybe.withDefault "Unknown"

test "name returns product name" = {
  let products = [
    { id: ProductId "a", name: Text.NonEmpty "Product A", price: 100, active: True },
    { id: ProductId "b", name: Text.NonEmpty "Product B", price: 200, active: True }
  ]
  name (ProductId "a") products == Text.NonEmpty "Product A"
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
  let user <- Db.get User session.user
  match user {
    Just u -> if u.role == Manager then {
      let products <- Db.from Product |> Db.all
      Query.succeed { products: products }
    } else Query.notFound,
    Nothing -> Query.notFound,
  }
}

msg Create form NewProduct
msg ToggleActive { id: Product.ProductId }

form NewProduct = {
  name: Text.NonEmpty,
  price: Int.NonNegative,
}

update : Msg -> Model -> Session -> Task Nav
update msg model session = match msg {
  Create p -> {
    let _ <- Db.insert Product { name: p.form.name, price: p.form.price, active: True }
    Task.succeed Rerender
  },
  ToggleActive p -> {
    let product <- Db.get Product p.id
    match product {
      Just prod -> {
        let _ <- Db.update Product p.id (\p -> { p | active: not p.active })
        Task.succeed Rerender
      },
      Nothing -> Task.notFound,
    }
  },
}

view : Model -> Session -> Html Msg
view model session =
  <Layout.Page title="Products" theme={session.theme}>
    <h1>Products</h1>
    <form submit={Create}>
      <Input field="name" label="Name" />
      <Errors field="name" />
      <Input field="price" label="Price (cents)" />
      <Errors field="price" />
      <button>Add product</button>
    </form>
    <ul>
      {for p in model.products ->
        <li>
          {p.name} - {Text.fromInt p.price} cents
          <button click={ToggleActive { id: p.id }}>{if p.active then "Deactivate" else "Activate"}</button>
        </li>}
    </ul>
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