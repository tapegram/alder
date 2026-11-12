--- file: app/User.alder ---
module User

type UserId = brand Text

-- A Staff user keeps their own orders. A Manager may also open anyone's order and manage the
-- products. The session holds only the user: the role is read from the user's row each time.
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

-- Begins every route only a Manager may use: anyone else gets a 404.
managerOnly : UserId -> Query Unit
managerOnly id = {
  let manager <- isManager id
  if manager then Query.succeed () else Query.notFound
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

statusText : Status -> Text
statusText status = match status {
  Open -> "Open",
  Paid -> "Paid",
  Cancelled -> "Cancelled",
}

-- The order, when the user may see it: their own, or anyone's for a Manager. Any other order,
-- like a missing one, is not found.
visible : User.UserId -> OrderId -> Query Order
visible user id = {
  let found <- Db.get Order id
  let manager <- User.isManager user
  match found {
    Just o -> if o.owner == user || manager then Query.succeed o else Query.notFound,
    Nothing -> Query.notFound,
  }
}

test "each status has its text" =
  List.map statusText [Open, Paid, Cancelled] == ["Open", "Paid", "Cancelled"]

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

type Item = { name: Text, quantity: Int, subtotal: Int }

-- quantity × price of the line's product.
lineTotal : List Product -> Line -> Int
lineTotal products line =
  match List.find (\p -> p.id == line.product) products {
    Just p -> line.quantity * p.price,
    Nothing -> 0,
  }

total : List Product -> List Line -> Int
total products lines = List.sum (List.map (lineTotal products) lines)

describe : List Product -> Line -> Item
describe products line = {
  let found = List.find (\p -> p.id == line.product) products
  let name : Text = Maybe.withDefault "" (Maybe.map (\p -> "{p.name}") found)
  { name: name, quantity: line.quantity, subtotal: lineTotal products line }
}

--- file: app/App.alder ---
module App

import Layout
import User

session { user: Maybe User.UserId }

login = Auth.Login.Login

route Logout = DELETE "/session" { }

Logout.update : Logout.Params -> Session -> Task Response
Logout.update p session = {
  let _ <- Session.put (\s -> { s | user: Nothing })
  Task.succeed (Response.redirect (Auth.Login.Login {}))
}

error : Error -> Html
error err =
  <Layout.Page title="Error">
    <h1>Something went wrong</h1>
    <p>Please try again.</p>
  </Layout.Page>

notFound : Html
notFound = <Layout.Page title="Not found"><h1>Not found</h1></Layout.Page>

test "Logout clears the user and goes to log in" = {
  let r <- Test.run (Logout.update {} { user: Just (User.UserId "me") })
  Task.succeed (
    { value: r.value, trace: r.trace }
      == { value: Ok (Response.redirect (Auth.Login.Login {})), trace: ["Session.put"] }
  )
}

--- file: app/Layout.alder ---
module Layout

Page : { title: Text, children: List Html } -> Html
Page props =
  <html>
    <head>
      <meta charset="utf-8" />
      <title>{props.title}</title>
      <link rel="stylesheet" href={Url.external "/static/al/al.css"} />
      <script type="module" src="/static/al/al.js"></script>
    </head>
    <body>
      <al-page width="narrow"><main><al-stack gap="lg">{props.children}</al-stack></main></al-page>
      <al-toasts></al-toasts>
    </body>
  </html>

--- file: app/Auth/Login.alder ---
module Auth.Login

import Layout
import User

-- Password checking is outside the language: a capability implemented in Go.
capability Password {
  verify : { email: Text.Email, password: Text } -> Task (Maybe User.UserId),
}

-- In a test, `Test.fake Password` accepts any user who has a row, whatever the password.
fake Password {
  verify args = {
    let found <-
      Db.from User
        |> Db.where (User.email.eq args.email)
        |> Db.first
    Task.succeed (Maybe.map (\u -> u.id) found)
  }
}

route Login  = GET  "/login"   { }
route Submit = POST "/session" { } form Credentials

type Model = { }

Login.load : Login.Params -> Session -> Query Model
Login.load params session = Query.succeed {}

form Credentials = { email: Text.Email, password: Text.Secret }

Submit.update : Submit.Params -> Session -> Task Response
Submit.update p session = {
  let found <- Password.verify { email: p.form.email, password: p.form.password }
  match found {
    Just id -> {
      let _ <- Session.put (\s -> { s | user: Just id })
      Task.succeed (Response.redirect (Orders.Index.Index {}))
    },
    Nothing -> Task.succeed (Response.invalid "Email or password is incorrect."),
  }
}

Login.view : Model -> Session -> Html
Login.view model session =
  <Layout.Page title="Log in">
    <h1>Log in</h1>
    <al-card>
      <form submit={Submit}>
        <Errors form />
        <al-input field="email" label="Email" />
        <al-input field="password" label="Password" />
        <al-button type="submit">Log in</al-button>
      </form>
    </al-card>
  </Layout.Page>

test "an unknown email is rejected" = {
  let _ <- Test.fake Password
  let _ <- Test.get (Login {})
  let r <- Test.submit Submit {
    email: Text.Email "nobody@example.com",
    password: Text.Secret "secret",
  }
  Task.succeed (
    { answer: r.answer, user: r.session.user }
      == {
        answer: FormRejected [{ field: "", message: "Email or password is incorrect." }],
        user: Nothing,
      }
  )
}

test "logging in through the form" = {
  let _ <- Test.fake Password
  let me <- Db.insert User {
    email: Text.Email "me@example.com",
    name: Text.NonEmpty "Me",
    passwordHash: "",
  }
  let _ <- Test.get (Login {})
  let r <- Test.submit Submit { email: Text.Email "me@example.com", password: Text.Secret "secret" }
  Task.succeed (
    { answer: r.answer, user: r.session.user }
      == { answer: Redirected (Orders.Index.Index {}), user: Just me }
  )
}

--- file: app/Orders/Index.alder ---
module Orders.Index

import Layout
import Line
import Order
import Product
import User

route Index  = GET  "/"       { } requires user
route Create = POST "/orders" { } requires user

type Row = { id: Order.OrderId, label: Text, status: Order.Status, total: Int }

type Model = { orders: List Row }

row : List Product -> List Line -> Order -> Row
row products lines order = {
  id: order.id,
  label: Time.format "Jan 2 15:04" order.created,
  status: order.status,
  total: Line.total products (List.filter (\l -> l.order == order.id) lines),
}

Index.load : Index.Params -> Session -> Query Model
Index.load params session = {
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
  Query.succeed { orders: List.map (row products lines) orders }
}

Create.update : Create.Params -> Session -> Task Response
Create.update p session = {
  let id <- Db.insert Order { owner: session.user }
  Task.succeed (Response.redirect (Orders.Show.Show { id: id }))
}

Index.view : Model -> Session -> Html
Index.view model session =
  <Layout.Page title="Orders">
    <h1>Orders</h1>
    <al-button click={Create}>New order</al-button>
    <ul id="orders">
      {for o in model.orders ->
        <li>
          <a href={Orders.Show.Show { id: o.id }}>Order of {o.label}</a>
          {" "}
          <span class="status">{Order.statusText o.status}</span>
          {" "}
          <span class="total">{Text.fromInt o.total}</span>
        </li>}
    </ul>
    <al-button variant="outline" click={App.Logout}>Log out</al-button>
  </Layout.Page>

test "a new order is Open and listed" = {
  let me <- Db.insert User {
    email: Text.Email "me@example.com",
    name: Text.NonEmpty "Me",
    passwordHash: "",
  }
  let _ <- Test.login { user: Just me }
  let _ <- Test.get (Index {})
  let _ <- Test.click Create
  let page <- Test.get (Index {})
  Task.succeed (
    { statuses: Test.texts "#orders .status" page.doc, totals: Test.texts "#orders .total" page.doc }
      == { statuses: ["Open"], totals: ["0"] }
  )
}

--- file: app/Orders/Show.alder ---
module Orders.Show

import Layout
import Line
import Order
import Product
import User

route Show    = GET  "/orders/{id}"           { id: Order.OrderId }                                  requires user
route AddLine = POST "/orders/{id}/lines"     { id: Order.OrderId, product: Product.ProductId } form NewLine requires user
route Pay     = PUT  "/orders/{id}/paid"      { id: Order.OrderId }                                  requires user
route Cancel  = PUT  "/orders/{id}/cancelled" { id: Order.OrderId }                                  requires user

type Model = {
  order: Order,
  lines: List Line.Item,
  total: Int,
  products: List Product,
}

form NewLine = { quantity: Int.Positive }

Show.load : Show.Params -> Session -> Query Model
Show.load params session = {
  let order <- Order.visible session.user params.id
  let lines <-
    Db.from Line
      |> Db.where (Line.order.eq params.id)
      |> Db.all
  let products <- Db.from Product |> Db.all
  Query.succeed {
    order: order,
    lines: List.map (Line.describe products) lines,
    total: Line.total products lines,
    products: List.filter (\x -> x.active) products,
  }
}

-- No `load` runs before a write: each one checks the order itself, so a stale page cannot add
-- to an order that is no longer Open, and another user's order is a 404 (unless a Manager).
AddLine.update : AddLine.Params -> Session -> Task Response
AddLine.update p session = {
  let order <- Order.visible session.user p.id
  let product <- Db.get Product p.product
  let active = Maybe.map (\x -> x.active) product == Just True
  if order.status != Open then
    Task.succeed (Response.invalid "This order is no longer open.")
  else if not active then
    Task.notFound
  else {
    let _ <- Db.insert Line { order: p.id, product: p.product, quantity: p.form.quantity }
    Task.succeed (Response.redirect (Show { id: p.id }))
  }
}

finish : User.UserId -> Order.OrderId -> Order.Status -> Task Response
finish user id status = {
  let order <- Order.visible user id
  if order.status == Open then {
    let _ <- Db.update Order id (\o -> { o | status: status })
    Task.succeed (Response.redirect (Show { id: id }))
  } else {
    Task.succeed (Response.redirect (Show { id: id }))
  }
}

Pay.update : Pay.Params -> Session -> Task Response
Pay.update p session = finish session.user p.id Paid

Cancel.update : Cancel.Params -> Session -> Task Response
Cancel.update p session = finish session.user p.id Cancelled

Show.view : Model -> Session -> Html
Show.view model session =
  <Layout.Page title="Order">
    <h1>Order</h1>
    <p>Status: <span id="status">{Order.statusText model.order.status}</span></p>
    <ul id="lines">
      {for l in model.lines ->
        <li>{l.name} x {Text.fromInt l.quantity}: {Text.fromInt l.subtotal}</li>}
    </ul>
    <p>Total: <span id="total">{Text.fromInt model.total}</span></p>
    {if model.order.status == Open then
      <div id="actions">
        {for pr in model.products ->
          <al-card>
            <p>{pr.name} ({Text.fromInt pr.price})</p>
            <form submit={AddLine { id: model.order.id, product: pr.id }}>
              <Errors form />
              <al-input field="quantity" label={"Quantity of {pr.name}"} />
              <al-button type="submit">Add</al-button>
            </form>
          </al-card>}
        <al-button click={Pay { id: model.order.id }}>Mark paid</al-button>
        <al-button variant="destructive" click={Cancel { id: model.order.id }}>Cancel order</al-button>
      </div>
    else Html.none}
    <al-button variant="ghost" href={Orders.Index.Index {}}>Back</al-button>
  </Layout.Page>

test "an Open order's total is the sum of its lines" = {
  let me <- Db.insert User {
    email: Text.Email "me@example.com",
    name: Text.NonEmpty "Me",
    passwordHash: "",
  }
  let tea <- Db.insert Product { name: Text.NonEmpty "Tea", price: Int.NonNegative 250 }
  let cake <- Db.insert Product { name: Text.NonEmpty "Cake", price: Int.NonNegative 100 }
  let id <- Db.insert Order { owner: me }
  let _ <- Db.insert Line { order: id, product: tea, quantity: Int.Positive 2 }
  let _ <- Db.insert Line { order: id, product: cake, quantity: Int.Positive 3 }
  let _ <- Test.login { user: Just me }
  let page <- Test.get (Show { id: id })
  Task.succeed (
    {
      total: page.model.total,
      shown: Test.text "#total" page.doc,
      status: Test.text "#status" page.doc,
    } == {
      total: 800,
      shown: Just "800",
      status: Just "Open",
    }
  )
}

test "adding a line to a Paid order is refused" = {
  let me <- Db.insert User {
    email: Text.Email "me@example.com",
    name: Text.NonEmpty "Me",
    passwordHash: "",
  }
  let tea <- Db.insert Product { name: Text.NonEmpty "Tea", price: Int.NonNegative 250 }
  let id <- Db.insert Order { owner: me }
  let _ <- Test.login { user: Just me }
  let _ <- Test.get (Show { id: id })
  let _ <- Db.update Order id (\o -> { o | status: Paid })
  let r <- Test.submit (AddLine { id: id, product: tea }) { quantity: Int.Positive 2 }
  let n <-
    Db.from Line
      |> Db.where (Line.order.eq id)
      |> Db.count
  Task.succeed ({ status: r.status, lines: n } == { status: 422, lines: 0 })
}

test "a Staff user opening another user's order is not found" = {
  let owner <- Db.insert User {
    email: Text.Email "owner@example.com",
    name: Text.NonEmpty "Owner",
    passwordHash: "",
  }
  let staff <- Db.insert User {
    email: Text.Email "staff@example.com",
    name: Text.NonEmpty "Staff",
    passwordHash: "",
  }
  let id <- Db.insert Order { owner: owner }
  let _ <- Test.login { user: Just staff }
  let page <- Test.open (Show { id: id })
  Task.succeed (page.answer == PageMissing)
}

test "a Manager opening another user's order sees it" = {
  let owner <- Db.insert User {
    email: Text.Email "owner@example.com",
    name: Text.NonEmpty "Owner",
    passwordHash: "",
  }
  let boss <- Db.insert User {
    email: Text.Email "boss@example.com",
    name: Text.NonEmpty "Boss",
    passwordHash: "",
    role: Manager,
  }
  let id <- Db.insert Order { owner: owner }
  let _ <- Test.login { user: Just boss }
  let page <- Test.open (Show { id: id })
  Task.succeed (
    { answer: page.answer, status: Test.text "#status" page.doc }
      == { answer: Shown, status: Just "Open" }
  )
}

--- file: app/Products/Index.alder ---
module Products.Index

import Layout
import Product
import User

route Index  = GET   "/products"      { }                                     requires user
route Add    = POST  "/products"      { }                     form NewProduct requires user
route Toggle = PATCH "/products/{id}" { id: Product.ProductId }               requires user

type Model = { products: List Product }

form NewProduct = { name: Text.NonEmpty, price: Int.NonNegative }

-- Every route here is a Manager's: each begins with `User.managerOnly`, which is a 404 for
-- anyone else, since no `load` runs before a write.
Index.load : Index.Params -> Session -> Query Model
Index.load params session = {
  let _ <- User.managerOnly session.user
  let products <- Db.from Product |> Db.all
  Query.succeed { products: products }
}

Add.update : Add.Params -> Session -> Task Response
Add.update p session = {
  let _ <- User.managerOnly session.user
  let _ <- Db.insert Product { name: p.form.name, price: p.form.price }
  Task.succeed (Response.redirect (Index {}))
}

Toggle.update : Toggle.Params -> Session -> Task Response
Toggle.update p session = {
  let _ <- User.managerOnly session.user
  let found <- Db.get Product p.id
  match found {
    Just x -> {
      let _ <- Db.update Product x.id (\y -> { y | active: not y.active })
      Task.succeed (Response.redirect (Index {}))
    },
    Nothing -> Task.notFound,
  }
}

Index.view : Model -> Session -> Html
Index.view model session =
  <Layout.Page title="Products">
    <h1>Products</h1>
    <al-card>
      <form submit={Add}>
        <Errors form />
        <al-input field="name" label="Name" />
        <al-input field="price" label="Price (cents)" />
        <al-button type="submit">Add</al-button>
      </form>
    </al-card>
    <ul id="products">
      {for p in model.products ->
        <li>
          <span class="name">{p.name}</span>
          {" "}
          <span class="price">{Text.fromInt p.price}</span>
          {" "}
          <al-button variant="secondary" size="sm" click={Toggle { id: p.id }}>
            {if p.active then "Deactivate" else "Activate"}
          </al-button>
        </li>}
    </ul>
    <al-button variant="ghost" href={Orders.Index.Index {}}>Orders</al-button>
  </Layout.Page>

test "a Staff user cannot open the products" = {
  let staff <- Db.insert User {
    email: Text.Email "staff@example.com",
    name: Text.NonEmpty "Staff",
    passwordHash: "",
  }
  let _ <- Test.login { user: Just staff }
  let page <- Test.open (Index {})
  Task.succeed (page.answer == PageMissing)
}

test "a Manager adds a product" = {
  let boss <- Db.insert User {
    email: Text.Email "boss@example.com",
    name: Text.NonEmpty "Boss",
    passwordHash: "",
    role: Manager,
  }
  let _ <- Test.login { user: Just boss }
  let _ <- Test.get (Index {})
  let r <- Test.submit (Add {}) { name: Text.NonEmpty "Tea", price: Int.NonNegative 250 }
  let page <- Test.get (Index {})
  Task.succeed (
    { answer: r.answer, names: Test.texts "#products .name" page.doc }
      == { answer: Redirected (Index {}), names: ["Tea"] }
  )
}
