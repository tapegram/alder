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

test "Logout clears the user and goes to log in" = {
  let r <- Test.run (update Logout { user: Just (User.UserId "me") })
  Task.succeed ({ value: r.value, trace: r.trace } == { value: Ok (Redirect (Auth.Login {})), trace: ["Session.put"] })
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
        <button click={App.Logout}>Log out</button>
      </nav>
      <main>{props.children}</main>
    </body>
  </html>

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

type Priced = { quantity: Int.Positive, price: Int.NonNegative }

total : List Priced -> Int
total items = List.sum (List.map (\i -> i.quantity * i.price) items)

statusLabel : Status -> Text
statusLabel s = match s {
  Open -> "Open",
  Paid -> "Paid",
  Cancelled -> "Cancelled",
}

test "an open order's total is the sum over its two lines" =
  total [{ quantity: Int.Positive 2, price: Int.NonNegative 150 }, { quantity: Int.Positive 1, price: Int.NonNegative 75 }] == 375

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

priced : List Line -> List Product -> List Order.Priced
priced lines products = List.filterMap (priceOf products) lines

priceOf : List Product -> Line -> Maybe Order.Priced
priceOf products line = {
  let found = List.find (\p -> p.id == line.product) products
  match found {
    Just p -> {
      let item : Order.Priced = { quantity: line.quantity, price: p.price }
      Just item
    },
    Nothing -> Nothing,
  }
}

--- file: app/Auth/Login.alder ---
module Auth.Login

import User
import Layout

-- Password checking is outside the language: a capability implemented in Go.
capability Password {
  verify : { email: Text.Email, password: Text } -> Task (Maybe User.UserId),
}

-- In a test, `Test.fake Password` may replace the adapter with this: it accepts any user who has
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

import Order
import Line
import Product
import User
import Layout

page "/" { } requires user

type Row = { order: Order, total: Int }

type Model = {
  rows: List Row,
  manager: Bool,
}

load : Params -> Session -> Query Model
load params session = {
  let orders <-
    Db.from Order
      |> Db.where (Order.owner.eq session.user)
      |> Db.orderBy Order.created.desc
      |> Db.all
  let lines <- Db.from Line |> Db.all
  let products <- Db.from Product |> Db.all
  let me <- Db.get User session.user
  Query.succeed {
    rows: List.map (rowOf products lines) orders,
    manager: Maybe.withDefault False (Maybe.map (\u -> u.role == Manager) me),
  }
}

rowOf : List Product -> List Line -> Order -> Row
rowOf products lines o = {
  let total = Order.total (Line.priced (List.filter (\l -> l.order == o.id) lines) products)
  let row : Row = { order: o, total: total }
  row
}

form NewOrderNone = { }

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
          <a href={Orders.Show { id: row.order.id }}>Open</a>
          <span class="status">{Order.statusLabel row.order.status}</span>
          <span class="total">{Text.fromInt row.total}</span>
        </li>}
    </ul>
    {if model.manager then <a href={Products.Index {}}>Products</a> else Html.none}
  </Layout.Page>

test "a new order is Open and empty" = {
  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
  let _ <- Test.login { user: Just me }
  let _ <- Test.get (Orders.Index {})
  let r <- Test.click Create
  Task.succeed ({ status: Test.text "#status" r.doc, total: Test.text "#total" r.doc } == { status: Just "Open", total: Just "0" })
}

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
  total: Int,
}

load : Params -> Session -> Query Model
load params session = {
  let found <- Db.get Order params.id
  let me <- Db.get User session.user
  let manager = Maybe.withDefault False (Maybe.map (\u -> u.role == Manager) me)
  match found {
    Just o -> if o.owner == session.user || manager then loaded o else Query.notFound,
    Nothing -> Query.notFound,
  }
}

loaded : Order -> Query Model
loaded o = {
  let allLines <- Db.from Line |> Db.all
  let lines = List.filter (\l -> l.order == o.id) allLines
  let products <- Db.from Product |> Db.all
  Query.succeed { order: o, lines: lines, products: products, total: Order.total (Line.priced lines products) }
}

form NewLine = {
  quantity: Int.Positive,
}

msg AddLine { product: Product.ProductId } form NewLine
msg Pay
msg Cancel

update : Msg -> Model -> Session -> Task Nav
update msg model session = match msg {
  AddLine p -> {
    let product <- Db.get Product p.product
    let active = Maybe.withDefault False (Maybe.map (\pr -> pr.active) product)
    if model.order.status != Open then Task.fail "the order is not open" else if not active then Task.fail "the product is not active" else {
      let _ <- Db.insert Line { order: model.order.id, product: p.product, quantity: p.form.quantity }
      Task.succeed (Redirect (Orders.Show { id: model.order.id }))
    }
  },
  Pay p -> moveTo model Paid,
  Cancel p -> moveTo model Cancelled,
}

moveTo : Model -> Order.Status -> Task Nav
moveTo model next = if model.order.status != Open then Task.fail "the order is not open" else {
  let _ <- Db.update Order model.order.id (\o -> { o | status: next })
  Task.succeed (Redirect (Orders.Show { id: model.order.id }))
}

productName : List Product -> Line -> Text
productName products line = {
  let found = List.find (\p -> p.id == line.product) products
  match found {
    Just p -> Text.trim p.name,
    Nothing -> "?",
  }
}

actions : Model -> Html Msg
actions model =
  if model.order.status != Open then Html.none
  else <div>
    {for p in List.filter (\p -> p.active) model.products ->
      <form submit={AddLine { product: p.id }}>
        <Errors form />
        <span>{p.name}</span>
        <Input field="quantity" label="Quantity" />
        <Errors field="quantity" />
        <button>Add</button>
      </form>}
    <button click={Pay}>Mark paid</button>
    <button click={Cancel}>Cancel order</button>
  </div>

view : Model -> Session -> Html Msg
view model session =
  <Layout.Page title="Order">
    <h1>Order</h1>
    <p>Status: <span id="status">{Order.statusLabel model.order.status}</span></p>
    <ul id="lines">
      {for line in model.lines ->
        <li>{productName model.products line} x {Text.fromInt line.quantity}</li>}
    </ul>
    <p>Total: <span id="total">{Text.fromInt model.total}</span></p>
    {actions model}
    <a href={Orders.Index {}}>Back</a>
  </Layout.Page>

test "an open order's total is over its lines" = {
  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
  let tea <- Db.insert Product { name: Text.NonEmpty "Tea", price: Int.NonNegative 150 }
  let cake <- Db.insert Product { name: Text.NonEmpty "Cake", price: Int.NonNegative 75 }
  let o <- Db.insert Order { owner: me }
  let _ <- Db.insert Line { order: o, product: tea, quantity: Int.Positive 2 }
  let _ <- Db.insert Line { order: o, product: cake, quantity: Int.Positive 1 }
  let _ <- Test.login { user: Just me }
  let page <- Test.get (Orders.Show { id: o })
  Task.succeed ({ total: Test.text "#total" page.doc, status: Test.text "#status" page.doc } == { total: Just "375", status: Just "Open" })
}

test "a line added to a paid order is refused" = {
  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
  let tea <- Db.insert Product { name: Text.NonEmpty "Tea", price: Int.NonNegative 150 }
  let o <- Db.insert Order { owner: me }
  let _ <- Test.login { user: Just me }
  let _ <- Test.get (Orders.Show { id: o })
  let _ <- Db.update Order o (\x -> { x | status: Paid })
  let r <- Test.submit (AddLine { product: tea }) { quantity: Int.Positive 1 }
  let lines <- Db.from Line |> Db.where (Line.order.eq o) |> Db.count
  Task.succeed ({ answer: r.answer, lines: lines } == { answer: RequestFailed "the order is not open", lines: 0 })
}

test "a staff user cannot open another user's order" = {
  let owner <- Db.insert User { email: Text.Email "owner@example.com", name: Text.NonEmpty "Owen", passwordHash: "" }
  let staff <- Db.insert User { email: Text.Email "staff@example.com", name: Text.NonEmpty "Sam", passwordHash: "" }
  let o <- Db.insert Order { owner: owner }
  let _ <- Test.login { user: Just staff }
  let r <- Test.open (Orders.Show { id: o })
  Task.succeed (r.answer == PageMissing)
}

test "a manager opens another user's order" = {
  let owner <- Db.insert User { email: Text.Email "owner@example.com", name: Text.NonEmpty "Owen", passwordHash: "" }
  let boss <- Db.insert User { email: Text.Email "boss@example.com", name: Text.NonEmpty "Boss", passwordHash: "", role: Manager }
  let o <- Db.insert Order { owner: owner }
  let _ <- Test.login { user: Just boss }
  let page <- Test.get (Orders.Show { id: o })
  Task.succeed (Test.text "#status" page.doc == Just "Open")
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
    Just u -> if u.role == Manager then listAll else Query.notFound,
    Nothing -> Query.notFound,
  }
}

listAll : Query Model
listAll = {
  let rows <- Db.from Product |> Db.orderBy Product.name.asc |> Db.all
  Query.succeed { products: rows }
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
      Just pr -> {
        let _ <- Db.update Product pr.id (\x -> { x | active: not x.active })
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
      <button>Add</button>
    </form>
    <ul id="products">
      {for p in model.products ->
        <li>
          <span class="name">{p.name}</span>
          <span class="price">{Text.fromInt p.price}</span>
          {if p.active then <span>active</span> else <span>inactive</span>}
          <button click={Toggle { id: p.id }}>{if p.active then "Deactivate" else "Activate"}</button>
        </li>}
    </ul>
  </Layout.Page>

test "a staff user cannot open the product list" = {
  let me <- Db.insert User { email: Text.Email "staff@example.com", name: Text.NonEmpty "Sam", passwordHash: "" }
  let _ <- Test.login { user: Just me }
  let r <- Test.open (Products.Index {})
  Task.succeed (r.answer == PageMissing)
}

test "a manager sees the products" = {
  let boss <- Db.insert User { email: Text.Email "boss@example.com", name: Text.NonEmpty "Boss", passwordHash: "", role: Manager }
  let _ <- Db.insert Product { name: Text.NonEmpty "Tea", price: Int.NonNegative 150 }
  let _ <- Test.login { user: Just boss }
  let page <- Test.get (Products.Index {})
  Task.succeed (Test.texts ".name" page.doc == ["Tea"])
}
