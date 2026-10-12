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

-- An order is visible to its owner and to every Manager.
canSee : Order -> User.UserId -> Query Bool
canSee order uid = {
  let found <- Db.get User uid
  let manager = match found {
    Just u -> u.role == Manager,
    Nothing -> False,
  }
  Query.succeed (order.owner == uid || manager)
}

--- file: app/Line.alder ---
module Line

import Order
import Product
import User

type LineId = brand Text

table Line {
  id: LineId @id,
  order: Order.OrderId @ref Order @cascade,
  product: Product.ProductId @ref Product,
  quantity: Int.Positive,
}

-- quantity x price of one line, in cents
cost : List Product -> Line -> Int
cost products line = {
  let found = List.find (\p -> p.id == line.product) products
  match found {
    Just p -> line.quantity * p.price,
    Nothing -> 0,
  }
}

total : List Product -> List Line -> Int
total products lines = List.sum (List.map (cost products) lines)

productName : List Product -> Line -> Text
productName products line = {
  let found = List.find (\p -> p.id == line.product) products
  match found {
    Just p -> Text.trim p.name,
    Nothing -> "?",
  }
}

test "an open order's total sums quantity times price" = {
  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
  let oid <- Db.insert Order { owner: me }
  let a <- Db.insert Product { name: Text.NonEmpty "A", price: Int.NonNegative 500 }
  let b <- Db.insert Product { name: Text.NonEmpty "B", price: Int.NonNegative 250 }
  let _ <- Db.insert Line { order: oid, product: a, quantity: Int.Positive 2 }
  let _ <- Db.insert Line { order: oid, product: b, quantity: Int.Positive 3 }
  let lines <- Db.from Line |> Db.where (Line.order.eq oid) |> Db.all
  let products <- Db.from Product |> Db.all
  let found <- Db.get Order oid
  let status = Maybe.map (\o -> o.status) found
  Task.succeed ({ status: status, total: total products lines } == { status: Just Open, total: 1750 })
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
import Product
import Line
import User
import Layout

page "/" { } requires user

type Row = { order: Order, total: Int }

type Model = { orders: List Row, manager: Bool }

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
  let me <- Db.get User session.user
  let manager = match me {
    Just u -> u.role == Manager,
    Nothing -> False,
  }
  Query.succeed { orders: List.map (row products lines) orders, manager: manager }
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
          <a href={Orders.Show { id: r.order.id }}>{Time.format "2006-01-02 15:04" r.order.created}</a>
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
import Product
import Line
import User
import Layout

page "/orders/{id}" { id: Order.OrderId } requires user

type Model = {
  order: Order,
  lines: List Line,
  products: List Product,
  total: Int,
}

load : Params -> Session -> Query Model
load params session = {
  let found <- Db.get Order params.id
  match found {
    Just o -> build o session,
    Nothing -> Query.notFound,
  }
}

-- The owner and every Manager may see an order; anyone else gets a 404.
build : Order -> Session -> Query Model
build o session = {
  let ok <- Order.canSee o session.user
  if ok then withLines o else Query.notFound
}

withLines : Order -> Query Model
withLines o = {
  let lines <- Db.from Line |> Db.where (Line.order.eq o.id) |> Db.all
  let products <- Db.from Product |> Db.all
  Query.succeed { order: o, lines: lines, products: products, total: Line.total products lines }
}

form NewLine = {
  quantity: Int.Positive,
}

msg AddLine { product: Product.ProductId } form NewLine
msg Pay
msg Cancel

-- The order is re-read inside the task, so a page loaded while the order was Open
-- cannot change it once it is not. Refusing is a Task.fail, not Task.notFound: the
-- order exists and the user may see it, so "not found" would be a lie; the message
-- is simply not valid in the order's current state. The task is undone and
-- nothing is written.
openOrder : Order.OrderId -> Task Order
openOrder id = {
  let found <- Db.get Order id
  match found {
    Just o -> if o.status == Open then Task.succeed o else Task.fail "order is not open",
    Nothing -> Task.notFound,
  }
}

addLine : Order -> Product -> Int.Positive -> Task Nav
addLine o pr qty = {
  let _ <- Db.insert Line { order: o.id, product: pr.id, quantity: qty }
  Task.succeed (Redirect (Orders.Show { id: o.id }))
}

update : Msg -> Model -> Session -> Task Nav
update msg model session = match msg {
  AddLine p -> {
    let o <- openOrder model.order.id
    let found <- Db.get Product p.product
    match found {
      Just pr -> if pr.active then addLine o pr p.form.quantity else Task.fail "product is not active",
      Nothing -> Task.notFound,
    }
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

view : Model -> Session -> Html Msg
view model session =
  <Layout.Page title="Order">
    <h1>Order</h1>
    <p>Status: <span id="status">{Order.statusLabel model.order.status}</span></p>
    <p>Total: <span id="total">{Text.fromInt model.total}</span></p>
    <ul id="lines">
      {for line in model.lines -> <LineItem line={line} products={model.products} />}
    </ul>
    {if model.order.status == Open then <Controls products={model.products} /> else Html.none}
    <a href={Orders.Index {}}>Back</a>
  </Layout.Page>

LineItem : { line: Line, products: List Product } -> Html a
LineItem props =
  <li>
    <span class="product">{Line.productName props.products props.line}</span>
    <span class="quantity">{Text.fromInt props.line.quantity}</span>
    <span class="subtotal">{Text.fromInt (Line.cost props.products props.line)}</span>
  </li>

Controls : { products: List Product } -> Html Msg
Controls props =
  <div>
    {for pr in List.filter (\p -> p.active) props.products ->
      <form submit={AddLine { product: pr.id }}>
        <strong>{pr.name}</strong>
        <Errors form />
        <Input field="quantity" label="Quantity" />
        <Errors field="quantity" />
        <button>Add</button>
      </form>}
    <button click={Pay}>Mark paid</button>
    <button click={Cancel}>Cancel order</button>
  </div>

test "adding a line to a Paid order is refused" = {
  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
  let oid <- Db.insert Order { owner: me }
  let pid <- Db.insert Product { name: Text.NonEmpty "A", price: Int.NonNegative 500 }
  let _ <- Test.login { user: Just me }
  let _ <- Test.get (Orders.Show { id: oid })
  let _ <- Db.update Order oid (\o -> { o | status: Paid })
  let r <- Test.submit (AddLine { product: pid }) { quantity: Int.Positive 1 }
  let n <- Db.from Line |> Db.count
  Task.succeed ({ answer: r.answer, lines: n } == { answer: RequestFailed "order is not open", lines: 0 })
}

test "a Staff user does not find another user's order" = {
  let owner <- Db.insert User { email: Text.Email "owner@example.com", name: Text.NonEmpty "Owner", passwordHash: "" }
  let other <- Db.insert User { email: Text.Email "other@example.com", name: Text.NonEmpty "Other", passwordHash: "" }
  let oid <- Db.insert Order { owner: owner }
  let _ <- Test.login { user: Just other }
  let r <- Test.open (Orders.Show { id: oid })
  Task.succeed (r.answer == PageMissing)
}

test "a Manager sees another user's order" = {
  let owner <- Db.insert User { email: Text.Email "owner@example.com", name: Text.NonEmpty "Owner", passwordHash: "" }
  let boss <- Db.insert User { email: Text.Email "boss@example.com", name: Text.NonEmpty "Boss", passwordHash: "", role: Manager }
  let oid <- Db.insert Order { owner: owner }
  let _ <- Test.login { user: Just boss }
  let r <- Test.open (Orders.Show { id: oid })
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
  match me {
    Just u -> if u.role == Manager then allProducts else Query.notFound,
    Nothing -> Query.notFound,
  }
}

allProducts : Query Model
allProducts = {
  let products <- Db.from Product |> Db.orderBy Product.name.asc |> Db.all
  Query.succeed { products: products }
}

form NewProduct = {
  name: Text.NonEmpty,
  price: Int.NonNegative,
}

msg Add form NewProduct
msg Toggle { id: Product.ProductId }

-- Messages come from the request: check the role again.
asManager : Session -> Task User
asManager session = {
  let me <- Db.get User session.user
  match me {
    Just u -> if u.role == Manager then Task.succeed u else Task.notFound,
    Nothing -> Task.notFound,
  }
}

toggle : Product -> Task Nav
toggle pr = {
  let _ <- Db.update Product pr.id (\x -> { x | active: not x.active })
  Task.succeed (Redirect (Products.Index {}))
}

update : Msg -> Model -> Session -> Task Nav
update msg model session = match msg {
  Add p -> {
    let _ <- asManager session
    let _ <- Db.insert Product { name: p.form.name, price: p.form.price }
    Task.succeed (Redirect (Products.Index {}))
  },
  Toggle p -> {
    let _ <- asManager session
    let found <- Db.get Product p.id
    match found {
      Just pr -> toggle pr,
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
      {for pr in model.products ->
        <li>
          <span class="name">{pr.name}</span>
          <span class="price">{Text.fromInt pr.price}</span>
          <button click={Toggle { id: pr.id }}>{if pr.active then "Deactivate" else "Activate"}</button>
        </li>}
    </ul>
    <a href={Orders.Index {}}>Back</a>
  </Layout.Page>

test "a Staff user gets 404 on the products page" = {
  let me <- Db.insert User { email: Text.Email "staff@example.com", name: Text.NonEmpty "Staff", passwordHash: "" }
  let _ <- Test.login { user: Just me }
  let r <- Test.open (Products.Index {})
  Task.succeed (r.answer == PageMissing)
}
