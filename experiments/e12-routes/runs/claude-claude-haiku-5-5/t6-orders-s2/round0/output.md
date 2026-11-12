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

-- Whether the user is a Manager. A user with no row is not.
isManager : UserId -> Query Bool
isManager id = {
  let found <- Db.get User id
  Query.succeed (Maybe.map (\u -> u.role == Manager) found ?? False)
}

-- Lets a Manager on; anyone else is answered as not found.
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

-- `owner` is `@owner`: `Db.mine Order` lists the signed-in user's orders (book 06, "Rules").
table Order {
  id: OrderId @id,
  owner: User.UserId @ref User @owner,
  status: Status = Open,
  created: Time @created,
}

statusText : Status -> Text
statusText status = match status {
  Open -> "Open",
  Paid -> "Paid",
  Cancelled -> "Cancelled",
}

-- The order, if the signed-in user may see it: their own, or any order for a Manager.
-- Otherwise it is not found.
seen : OrderId -> User.UserId -> Query Order
seen id user = {
  let found <- Db.get Order id
  match found {
    Nothing -> Query.notFound,
    Just order -> {
      let manager <- User.isManager user
      if order.owner == user || manager then Query.succeed order else Query.notFound
    },
  }
}

-- Moves an open order to another status. False when the order was not open, and nothing changes.
moveFromOpen : OrderId -> User.UserId -> Status -> Task Bool
moveFromOpen id user next = {
  let order <- seen id user
  if order.status == Open then {
    let _ <- Db.update Order id (\o -> { o | status: next })
    Task.succeed True
  } else Task.succeed False
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

type LineRow = { name: Text, quantity: Int.Positive, cents: Int }

-- A line's cost in cents: its quantity times its product's price (0 when the product is gone).
costOf : Line -> Query Int
costOf line = {
  let product <- Db.get Product line.product
  Query.succeed (Maybe.map (\p -> line.quantity * p.price) product ?? 0)
}

-- The order's total in cents: the sum of the costs of its lines.
total : Order.OrderId -> Query Int
total id = {
  let lines <- Db.from Line |> Db.where (Line.order.eq id) |> Db.all
  let costs <- Query.all (List.map costOf lines)
  Query.succeed (List.sum costs)
}

-- A line as its order's page shows it.
rowOf : Line -> Query LineRow
rowOf line = {
  let product <- Db.get Product line.product
  let cost <- costOf line
  Query.succeed {
    name: Maybe.map (\p -> Text.trim p.name) product ?? "",
    quantity: line.quantity,
    cents: cost,
  }
}

test "an open order's total is the sum of its lines" = {
  let me <- Db.insert User {
    email: Text.Email "me@example.com",
    name: Text.NonEmpty "Me",
    passwordHash: "",
  }
  let order <- Db.insert Order { owner: me }
  let pen <- Db.insert Product { name: Text.NonEmpty "Pen", price: Int.NonNegative 150 }
  let mug <- Db.insert Product { name: Text.NonEmpty "Mug", price: Int.NonNegative 700 }
  let _ <- Db.insert Line { order: order, product: pen, quantity: Int.Positive 2 }
  let _ <- Db.insert Line { order: order, product: mug, quantity: Int.Positive 1 }
  let amount <- total order
  Task.succeed (amount == 1000)
}

--- file: app/Orders/Index.alder ---
module Orders.Index

import Layout
import Line
import Order

route Index  = GET  "/"       { } requires user
route Create = POST "/orders" { } requires user

type OrderRow = { order: Order, total: Int }

type Model = { orders: List OrderRow }

withTotal : Order -> Query OrderRow
withTotal order = {
  let total <- Line.total order.id
  Query.succeed { order: order, total: total }
}

Index.load : Index.Params -> Session -> Query Model
Index.load params session = {
  let orders <-
    Db.mine Order
      |> Db.orderBy Order.created.desc
      |> Db.all
  let rows <- Query.all (List.map withTotal orders)
  Query.succeed { orders: rows }
}

-- Creates a new, empty, Open order for the signed-in user and opens it.
Create.update : Create.Params -> Session -> Task Response
Create.update p session = {
  let id <- Db.insertMine Order { status: Open }
  Task.succeed (Response.redirect (Orders.Show.Show { id: id }))
}

OrderItem : { row: OrderRow } -> Html
OrderItem props =
  <li>
    <a href={Orders.Show.Show { id: props.row.order.id }}>View</a>
    <span class="status">{Order.statusText props.row.order.status}</span>
    <span class="total">{Text.fromInt props.row.total}</span>
  </li>

Index.view : Model -> Session -> Html
Index.view model session =
  <Layout.Page title="Orders">
    <h1>Orders</h1>
    <form submit={Create {}}>
      <al-button type="submit">New order</al-button>
    </form>
    <ul id="orders">
      {for row in model.orders -> <OrderItem row={row} />}
    </ul>
    <al-button variant="outline" click={App.Logout}>Log out</al-button>
  </Layout.Page>

--- file: app/Orders/Show.alder ---
module Orders.Show

import Layout
import Line
import Order
import Product
import User

route Show    = GET  "/orders/{id}"        { id: Order.OrderId } requires user
route AddLine = POST "/orders/{id}/lines"  { id: Order.OrderId, product: Product.ProductId } form NewLine requires user
route Pay     = PUT  "/orders/{id}/paid"   { id: Order.OrderId } requires user
route Cancel  = PUT  "/orders/{id}/cancelled" { id: Order.OrderId } requires user

form NewLine = { quantity: Int.Positive }

type Model = {
  order: Order,
  lines: List Line.LineRow,
  total: Int,
  products: List Product,
}

-- A refused write: the page again, with a notice that the order is no longer open.
refuse : Text -> Response
refuse text = Response.refresh |> Response.error text

-- The answer once an order has been moved (or was not open).
reply : Bool -> Order.OrderId -> Response
reply moved id =
  if moved then Response.redirect (Show { id: id })
  else refuse "This order is no longer open."

Show.load : Show.Params -> Session -> Query Model
Show.load params session = {
  let order <- Order.seen params.id session.user
  let lines <- Db.from Line |> Db.where (Line.order.eq order.id) |> Db.all
  let rows <- Query.all (List.map Line.rowOf lines)
  let total <- Line.total order.id
  let products <-
    Db.from Product
      |> Db.where (Product.active.eq True)
      |> Db.orderBy Product.name.asc
      |> Db.all
  Query.succeed { order: order, lines: rows, total: total, products: products }
}

-- Adds a line to an open order. Refused, with a notice, when the order is no longer open
-- (even if this page was loaded while it was) or the product is not for sale.
AddLine.update : AddLine.Params -> Session -> Task Response
AddLine.update p session = {
  let order <- Order.seen p.id session.user
  let product <- Db.get Product p.product
  let active = Maybe.map (\x -> x.active) product ?? False
  if order.status != Open then Task.succeed (refuse "This order is no longer open.")
  else if not active then Task.succeed (refuse "This product is not for sale.")
  else {
    let _ <- Db.insert Line { order: p.id, product: p.product, quantity: p.form.quantity }
    Task.succeed (Response.redirect (Show { id: p.id }))
  }
}

Pay.update : Pay.Params -> Session -> Task Response
Pay.update p session = {
  let moved <- Order.moveFromOpen p.id session.user Paid
  Task.succeed (reply moved p.id)
}

Cancel.update : Cancel.Params -> Session -> Task Response
Cancel.update p session = {
  let moved <- Order.moveFromOpen p.id session.user Cancelled
  Task.succeed (reply moved p.id)
}

-- The forms and buttons of an open order.
Actions : { order: Order, products: List Product } -> Html
Actions props =
  <div>
    {for p in props.products ->
      <form submit={AddLine { id: props.order.id, product: p.id }}>
        <Errors form />
        <al-input field="quantity" label="Quantity" />
        <al-button type="submit">Add {p.name}</al-button>
      </form>}
    <al-button click={Pay { id: props.order.id }}>Mark paid</al-button>
    <al-button variant="destructive" click={Cancel { id: props.order.id }}>Cancel order</al-button>
  </div>

Show.view : Model -> Session -> Html
Show.view model session =
  <Layout.Page title="Order">
    <h1>Order</h1>
    <p>Status: <span id="status">{Order.statusText model.order.status}</span></p>
    <ul id="lines">
      {for row in model.lines -> <li>{row.name} x {Text.fromInt row.quantity} = {Text.fromInt row.cents}</li>}
    </ul>
    <p>Total: <span id="total">{Text.fromInt model.total}</span></p>
    {if model.order.status == Open then <Actions order={model.order} products={model.products} /> else Html.none}
    <al-button variant="ghost" href={Orders.Index.Index {}}>Back</al-button>
  </Layout.Page>

test "adding a line to a paid order is refused" = {
  let me <- Db.insert User {
    email: Text.Email "me@example.com",
    name: Text.NonEmpty "Me",
    passwordHash: "",
  }
  let order <- Db.insert Order { owner: me }
  let pen <- Db.insert Product { name: Text.NonEmpty "Pen", price: Int.NonNegative 150 }
  let _ <- Test.login { user: Just me }
  let _ <- Test.get (Show { id: order })
  let _ <- Db.update Order order (\o -> { o | status: Paid })
  let r <- Test.submit (AddLine { id: order, product: pen }) { quantity: Int.Positive 2 }
  let lines <- Db.from Line |> Db.where (Line.order.eq order) |> Db.count
  Task.succeed (
    { answer: r.answer, notices: r.notices, lines: lines }
      == {
        answer: Refreshed,
        notices: ["This order is no longer open."],
        lines: 0,
      }
  )
}

test "a staff user cannot open another user's order" = {
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
  let order <- Db.insert Order { owner: owner }
  let _ <- Test.login { user: Just staff }
  let r <- Test.open (Show { id: order })
  Task.succeed (r.answer == PageMissing)
}

test "a manager opens another user's order" = {
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
  let order <- Db.insert Order { owner: owner }
  let _ <- Test.login { user: Just boss }
  let r <- Test.get (Show { id: order })
  Task.succeed (Test.text "#status" r.doc == Just "Open")
}

--- file: app/Products/Index.alder ---
module Products.Index

import Layout
import Product
import User

route Index  = GET   "/products"      { } requires user
route Add    = POST  "/products"      { } form NewProduct requires user
route Toggle = PATCH "/products/{id}" { id: Product.ProductId } requires user

form NewProduct = { name: Text.NonEmpty, price: Int.NonNegative }

type Model = { products: List Product }

Index.load : Index.Params -> Session -> Query Model
Index.load params session = {
  let _ <- User.managerOnly session.user
  let products <-
    Db.from Product
      |> Db.orderBy Product.name.asc
      |> Db.all
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
  let _ <- Db.update Product p.id (\x -> { x | active: not x.active })
  Task.succeed (Response.redirect (Index {}))
}

Item : { product: Product } -> Html
Item props =
  <li class={if props.product.active then "" else "inactive"}>
    <span class="name">{props.product.name}</span>
    <span>{Text.fromInt props.product.price}</span>
    <al-button variant="outline" size="sm" click={Toggle { id: props.product.id }}>
      {if props.product.active then "Deactivate" else "Activate"}
    </al-button>
  </li>

Index.view : Model -> Session -> Html
Index.view model session =
  <Layout.Page title="Products">
    <h1>Products</h1>
    <form submit={Add {}}>
      <Errors form />
      <al-input field="name" label="Name" />
      <al-input field="price" label="Price (cents)" />
      <al-button type="submit">Add product</al-button>
    </form>
    <ul id="products">
      {for p in model.products -> <Item product={p} />}
    </ul>
  </Layout.Page>

test "a staff user is not found on products" = {
  let me <- Db.insert User {
    email: Text.Email "staff@example.com",
    name: Text.NonEmpty "Staff",
    passwordHash: "",
  }
  let _ <- Test.login { user: Just me }
  let r <- Test.open (Index {})
  Task.succeed (r.answer == PageMissing)
}

--- file: app/Auth/Login.alder ---
module Auth.Login

import Layout
import User

-- Password checking is outside the language: a capability implemented in Go.
capability Password {
  verify : { email: Text.Email, password: Text } -> Task (Maybe User.UserId),
}

-- In a test, `Test.fake Password` puts this in the adapter's place: it accepts any user who has
-- a row, whatever the password.
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

form Credentials = { email: Text.Email, password: Text.Secret }

Login.load : Login.Params -> Session -> Query Model
Login.load params session = Query.succeed {}

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
      <form submit={Submit {}}>
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
  let r <- Test.submit (Submit {}) {
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
  let r <- Test.submit (Submit {}) {
    email: Text.Email "me@example.com",
    password: Text.Secret "secret",
  }
  Task.succeed (
    {
      answer: r.answer,
      user: r.session.user,
      title: Test.text "h1" r.doc,
    } == {
      answer: Redirected (Orders.Index.Index {}),
      user: Just me,
      title: Just "Orders",
    }
  )
}
