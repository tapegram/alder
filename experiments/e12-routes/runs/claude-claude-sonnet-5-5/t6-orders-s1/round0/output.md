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

-- The session holds only the user, so the role is read from the row each time.
isManager : UserId -> Query Bool
isManager id = {
  let me <- Db.get User id
  Query.succeed (Maybe.map (\u -> u.role) me == Just Manager)
}

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

statusLabel : Status -> Text
statusLabel status = match status {
  Open -> "Open",
  Paid -> "Paid",
  Cancelled -> "Cancelled",
}

-- The owner and every Manager may see an order.
canSee : User.UserId -> Order -> Query Bool
canSee user order =
  if order.owner == user then Query.succeed True else User.isManager user

-- The order, or a 404 when it is missing or not the user's (and the user is no Manager).
visible : User.UserId -> OrderId -> Query Order
visible user id = {
  let found <- Db.get Order id
  match found {
    Nothing -> Query.notFound,
    Just order -> {
      let ok <- canSee user order
      if ok then Query.succeed order else Query.notFound
    },
  }
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

plain : Int -> Int
plain n = n

plainText : Text -> Text
plainText t = t

priceOf : List Product -> Line -> Int
priceOf products line = {
  let found = List.find (\p -> p.id == line.product) products
  match found {
    Just p -> plain p.price,
    Nothing -> 0,
  }
}

nameOf : List Product -> Line -> Text
nameOf products line = {
  let found = List.find (\p -> p.id == line.product) products
  match found {
    Just p -> plainText p.name,
    Nothing -> "",
  }
}

subtotal : List Product -> Line -> Int
subtotal products line = plain line.quantity * priceOf products line

total : List Product -> List Line -> Int
total products lines = List.sum (List.map (subtotal products) lines)

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

--- file: app/Auth/Login.alder ---
module Auth.Login

import Layout
import User

capability Password {
  verify : { email: Text.Email, password: Text } -> Task (Maybe User.UserId),
}

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

route Index  = GET  "/"       { } requires user
route Create = POST "/orders" { } requires user

type Row = { order: Order, total: Int }

type Model = { orders: List Row }

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
  Query.succeed {
    orders: List.map (\o -> {
      order: o,
      total: Line.total products (List.filter (\l -> l.order == o.id) lines),
    }) orders,
  }
}

Create.update : Create.Params -> Session -> Task Response
Create.update p session = {
  let id <- Db.insert Order { owner: session.user }
  Task.succeed (Response.redirect (Orders.Show.Show { id: id }))
}

Item : { row: Row } -> Html
Item props =
  <li>
    <a href={Orders.Show.Show { id: props.row.order.id }}>View order</a>
    <span class="status">{Order.statusLabel props.row.order.status}</span>
    <span class="total">{Text.fromInt props.row.total}</span>
  </li>

Index.view : Model -> Session -> Html
Index.view model session =
  <Layout.Page title="Orders">
    <h1>Orders</h1>
    <al-button click={Create}>New order</al-button>
    <ul id="orders">
      {for r in model.orders -> <Item row={r} />}
    </ul>
    <nav>
      <a href={Products.Index.Index {}}>Products</a>
    </nav>
    <al-button variant="outline" click={App.Logout}>Log out</al-button>
  </Layout.Page>

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

form NewLine = { quantity: Int.Positive }

type Row = { name: Text, quantity: Int, subtotal: Int }

type Model = { order: Order, rows: List Row, products: List Product, total: Int }

Show.load : Show.Params -> Session -> Query Model
Show.load params session = {
  let order <- Order.visible session.user params.id
  let lines <-
    Db.from Line
      |> Db.where (Line.order.eq order.id)
      |> Db.all
  let products <- Db.from Product |> Db.all
  Query.succeed {
    order: order,
    rows: List.map (\l -> {
      name: Line.nameOf products l,
      quantity: Line.plain l.quantity,
      subtotal: Line.subtotal products l,
    }) lines,
    products: products,
    total: Line.total products lines,
  }
}

-- The server decides, whatever page the form came from.
AddLine.update : AddLine.Params -> Session -> Task Response
AddLine.update p session = {
  let order <- Order.visible session.user p.id
  let product <- Db.get Product p.product
  let usable = Maybe.map (\x -> x.active) product == Just True
  if order.status != Open then
    Task.succeed (Response.invalid "Only an open order accepts lines.")
  else if not usable then
    Task.succeed (Response.invalid "That product is not available.")
  else {
    let _ <- Db.insert Line { order: order.id, product: p.product, quantity: p.form.quantity }
    Task.succeed (Response.redirect (Orders.Show.Show { id: p.id }))
  }
}

Pay.update : Pay.Params -> Session -> Task Response
Pay.update p session = {
  let order <- Order.visible session.user p.id
  if order.status == Open then {
    let _ <- Db.update Order order.id (\o -> { o | status: Paid })
    Task.succeed (Response.redirect (Orders.Show.Show { id: p.id }))
  } else {
    Task.succeed Response.refresh
  }
}

Cancel.update : Cancel.Params -> Session -> Task Response
Cancel.update p session = {
  let order <- Order.visible session.user p.id
  if order.status == Open then {
    let _ <- Db.update Order order.id (\o -> { o | status: Cancelled })
    Task.succeed (Response.redirect (Orders.Show.Show { id: p.id }))
  } else {
    Task.succeed Response.refresh
  }
}

Actions : { order: Order, products: List Product } -> Html
Actions props =
  <section id="actions">
    {for p in List.filter (\x -> x.active) props.products ->
      <form submit={AddLine { id: props.order.id, product: p.id }}>
        <h3>{p.name}</h3>
        <Errors form />
        <al-input field="quantity" label="Quantity" />
        <al-button type="submit">Add</al-button>
      </form>}
    <al-button click={Pay { id: props.order.id }}>Mark paid</al-button>
    <al-button variant="destructive" click={Cancel { id: props.order.id }}>Cancel order</al-button>
  </section>

Show.view : Model -> Session -> Html
Show.view model session =
  <Layout.Page title="Order">
    <h1>Order</h1>
    <p>Status: <span id="status">{Order.statusLabel model.order.status}</span></p>
    <p>Total: <span id="total">{Text.fromInt model.total}</span></p>
    <ul id="lines">
      {for r in model.rows ->
        <li>{r.name} x {Text.fromInt r.quantity} = {Text.fromInt r.subtotal}</li>}
    </ul>
    {if model.order.status == Open then <Actions order={model.order} products={model.products} /> else Html.none}
    <a href={Orders.Index.Index {}}>Back</a>
  </Layout.Page>

test "an Open order's total adds its lines" = {
  let me <- Db.insert User {
    email: Text.Email "me@example.com",
    name: Text.NonEmpty "Me",
    passwordHash: "",
  }
  let pen <- Db.insert Product { name: Text.NonEmpty "Pen", price: Int.NonNegative 250 }
  let ink <- Db.insert Product { name: Text.NonEmpty "Ink", price: Int.NonNegative 1000 }
  let order <- Db.insert Order { owner: me }
  let _ <- Db.insert Line { order: order, product: pen, quantity: Int.Positive 2 }
  let _ <- Db.insert Line { order: order, product: ink, quantity: Int.Positive 1 }
  let _ <- Test.login { user: Just me }
  let page <- Test.get (Show { id: order })
  Task.succeed (
    { total: Test.text "#total" page.doc, status: Test.text "#status" page.doc }
      == { total: Just "1500", status: Just "Open" }
  )
}

test "adding a line to a Paid order is refused" = {
  let me <- Db.insert User {
    email: Text.Email "me@example.com",
    name: Text.NonEmpty "Me",
    passwordHash: "",
  }
  let pen <- Db.insert Product { name: Text.NonEmpty "Pen", price: Int.NonNegative 250 }
  let order <- Db.insert Order { owner: me }
  let _ <- Test.login { user: Just me }
  let _ <- Test.get (Show { id: order })
  let _ <- Db.update Order order (\o -> { o | status: Paid })
  let r <- Test.submit (AddLine { id: order, product: pen }) { quantity: Int.Positive 1 }
  let n <- Db.from Line |> Db.count
  Task.succeed ({ lines: n, status: r.status } == { lines: 0, status: 422 })
}

test "a Staff user opening another user's order is not found" = {
  let owner <- Db.insert User {
    email: Text.Email "owner@example.com",
    name: Text.NonEmpty "Owner",
    passwordHash: "",
  }
  let other <- Db.insert User {
    email: Text.Email "other@example.com",
    name: Text.NonEmpty "Other",
    passwordHash: "",
  }
  let order <- Db.insert Order { owner: owner }
  let _ <- Test.login { user: Just other }
  let r <- Test.open (Show { id: order })
  Task.succeed (r.answer == PageMissing)
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
  let order <- Db.insert Order { owner: owner }
  let _ <- Test.login { user: Just boss }
  let page <- Test.get (Show { id: order })
  Task.succeed (Test.text "#status" page.doc == Just "Open")
}

--- file: app/Products/Index.alder ---
module Products.Index

import Layout
import Product
import User

route Index  = GET   "/products"      { }                         requires user
route Add    = POST  "/products"      { } form NewProduct         requires user
route Toggle = PATCH "/products/{id}" { id: Product.ProductId }   requires user

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
  Task.succeed (Response.redirect (Products.Index.Index {}))
}

Toggle.update : Toggle.Params -> Session -> Task Response
Toggle.update p session = {
  let _ <- User.managerOnly session.user
  let found <- Db.get Product p.id
  match found {
    Nothing -> Task.notFound,
    Just x -> {
      let _ <- Db.update Product x.id (\y -> { y | active: not y.active })
      Task.succeed (Response.redirect (Products.Index.Index {}))
    },
  }
}

Item : { product: Product } -> Html
Item props =
  <li>
    <span class="name">{props.product.name}</span>
    <span class="price">{Text.fromInt props.product.price}</span>
    <al-button size="sm" click={Toggle { id: props.product.id }}>
      {if props.product.active then "Deactivate" else "Activate"}
    </al-button>
  </li>

Index.view : Model -> Session -> Html
Index.view model session =
  <Layout.Page title="Products">
    <h1>Products</h1>
    <al-card>
      <form submit={Add}>
        <Errors form />
        <al-input field="name" label="Name" />
        <al-input field="price" label="Price in cents" />
        <al-button type="submit">Add product</al-button>
      </form>
    </al-card>
    <ul id="products">
      {for pr in model.products -> <Item product={pr} />}
    </ul>
    <a href={Orders.Index.Index {}}>Orders</a>
  </Layout.Page>

test "a Staff user gets 404 on the products page" = {
  let me <- Db.insert User {
    email: Text.Email "me@example.com",
    name: Text.NonEmpty "Me",
    passwordHash: "",
  }
  let _ <- Test.login { user: Just me }
  let r <- Test.open (Index {})
  Task.succeed (r.answer == PageMissing)
}

test "a Manager sees the products" = {
  let boss <- Db.insert User {
    email: Text.Email "boss@example.com",
    name: Text.NonEmpty "Boss",
    passwordHash: "",
    role: Manager,
  }
  let _ <- Db.insert Product { name: Text.NonEmpty "Pen", price: Int.NonNegative 250 }
  let _ <- Test.login { user: Just boss }
  let page <- Test.get (Index {})
  Task.succeed (Test.texts "#products .name" page.doc == ["Pen"])
}
