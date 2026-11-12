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

-- The role is read from the row on each use, so a demotion applies at once.
isManager : UserId -> Query Bool
isManager id = {
  let me <- Db.get User id
  Query.succeed (Maybe.map (\u -> u.role) me == Just Manager)
}

managerOnly : UserId -> Query Unit
managerOnly id = {
  let ok <- isManager id
  if ok then Query.succeed () else Query.notFound
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
statusLabel s = match s {
  Open -> "Open",
  Paid -> "Paid",
  Cancelled -> "Cancelled",
}

-- The order, when it is the user's or the user is a Manager; otherwise not found.
visible : User.UserId -> OrderId -> Query Order
visible uid id = {
  let found <- Db.get Order id
  match found {
    Nothing -> Query.notFound,
    Just o -> {
      let mgr <- User.isManager uid
      if o.owner == uid || mgr then Query.succeed o else Query.notFound
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

lineTotal : List Product -> Line -> Int
lineTotal products l =
  List.find (\p -> p.id == l.product) products
    |> Maybe.map (\p -> l.quantity * p.price)
    |> Maybe.withDefault 0

total : List Product -> List Line -> Int
total products lines = List.sum (List.map (lineTotal products) lines)

productName : List Product -> Line -> Text
productName products l =
  List.find (\p -> p.id == l.product) products
    |> Maybe.map (\p -> Text.trim p.name)
    |> Maybe.withDefault ""

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

--- file: app/Orders/Index.alder ---
module Orders.Index

import Layout
import Line
import Order
import Product

route Index  = GET  "/"       { } requires user
route Create = POST "/orders" { } requires user

type Row = { order: Order, total: Int }

type Model = { rows: List Row }

Index.load : Index.Params -> Session -> Query Model
Index.load params session = {
  let orders <-
    Db.from Order
      |> Db.where (Order.owner.eq session.user)
      |> Db.orderBy Order.created.desc
      |> Db.all
  let products <- Db.from Product |> Db.all
  let lines <-
    Db.from Line
      |> Db.where (Line.order.in (List.map (\o -> o.id) orders))
      |> Db.all
  let rows = List.map (\o -> {
    order: o,
    total: Line.total products (List.filter (\l -> l.order == o.id) lines),
  }) orders
  Query.succeed { rows: rows }
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
      {for r in model.rows ->
        <li>
          <a href={Orders.Show.Show { id: r.order.id }}>{Time.format "2006-01-02 15:04" r.order.created}</a>
          <span class="status">{Order.statusLabel r.order.status}</span>
          <span class="total">{Text.fromInt r.total}</span>
        </li>}
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

route Show    = GET {- order -} "/orders/{id}"           { id: Order.OrderId }                                  requires user
route AddLine = POST "/orders/{id}/lines"                { id: Order.OrderId, product: Product.ProductId } form NewLine requires user
route Pay     = PUT "/orders/{id}/paid"                  { id: Order.OrderId }                                  requires user
route Cancel  = PUT "/orders/{id}/cancelled"             { id: Order.OrderId }                                  requires user

type Model = {
  order: Order,
  lines: List Line,
  products: List Product,
  total: Int,
}

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
    lines: lines,
    products: products,
    total: Line.total products lines,
  }
}

form NewLine = { quantity: Int.Positive }

AddLine.update : AddLine.Params -> Session -> Task Response
AddLine.update p session = {
  let order <- Order.visible session.user p.id
  let product <- Db.get Product p.product
  let usable = Maybe.map (\x -> x.active) product == Just True
  if order.status != Open then
    Task.succeed (Response.invalid "This order is not open.")
  else if not usable then
    Task.succeed (Response.invalid "That product is not available.")
  else {
    let _ <- Db.insert Line { order: p.id, product: p.product, quantity: p.form.quantity }
    Task.succeed (Response.redirect (Show { id: p.id }))
  }
}

settle : Order.Status -> Order.OrderId -> User.UserId -> Task Response
settle next id user = {
  let order <- Order.visible user id
  if order.status == Open then {
    let _ <- Db.update Order id (\o -> { o | status: next })
    Task.succeed (Response.redirect (Show { id: id }))
  } else Task.succeed (Response.redirect (Show { id: id }))
}

Pay.update : Pay.Params -> Session -> Task Response
Pay.update p session = settle Paid p.id session.user

Cancel.update : Cancel.Params -> Session -> Task Response
Cancel.update p session = settle Cancelled p.id session.user

Actions : { order: Order, products: List Product } -> Html
Actions props =
  <section id="actions">
    <al-button click={Pay { id: props.order.id }}>Mark paid</al-button>
    <al-button variant="destructive" click={Cancel { id: props.order.id }}>Cancel order</al-button>
    {for p in List.filter (\x -> x.active) props.products ->
      <form submit={AddLine { id: props.order.id, product: p.id }}>
        <h3>{p.name}</h3>
        <Errors form />
        <al-input field="quantity" label="Quantity" />
        <al-button type="submit">Add</al-button>
      </form>}
  </section>

Show.view : Model -> Session -> Html
Show.view model session =
  <Layout.Page title="Order">
    <h1>Order</h1>
    <p>Status: <span id="status">{Order.statusLabel model.order.status}</span></p>
    <p>Total: <span id="total">{Text.fromInt model.total}</span></p>
    <ul id="lines">
      {for l in model.lines ->
        <li>{Line.productName model.products l} x {Text.fromInt l.quantity} = {Text.fromInt (Line.lineTotal model.products l)}</li>}
    </ul>
    {if model.order.status == Open then <Actions order={model.order} products={model.products} /> else Html.none}
    <a href={Orders.Index.Index {}}>Back</a>
  </Layout.Page>

test "an open order's total adds its lines" = {
  let me <- Db.insert User {
    email: Text.Email "ann@example.com",
    name: Text.NonEmpty "Ann",
    passwordHash: "",
  }
  let pen <- Db.insert Product { name: Text.NonEmpty "Pen", price: Int.NonNegative 250 }
  let ink <- Db.insert Product { name: Text.NonEmpty "Ink", price: Int.NonNegative 100 }
  let oid <- Db.insert Order { owner: me }
  let _ <- Db.insert Line { order: oid, product: pen, quantity: Int.Positive 2 }
  let _ <- Db.insert Line { order: oid, product: ink, quantity: Int.Positive 3 }
  let _ <- Test.login { user: Just me }
  let page <- Test.get (Show { id: oid })
  Task.succeed (
    { status: Test.text "#status" page.doc, total: Test.text "#total" page.doc }
      == { status: Just "Open", total: Just "800" }
  )
}

test "adding a line to a paid order is refused" = {
  let me <- Db.insert User {
    email: Text.Email "ann@example.com",
    name: Text.NonEmpty "Ann",
    passwordHash: "",
  }
  let pen <- Db.insert Product { name: Text.NonEmpty "Pen", price: Int.NonNegative 250 }
  let oid <- Db.insert Order { owner: me, status: Paid }
  let r <- Test.run (AddLine.update { id: oid, product: pen, form: { quantity: Int.Positive 1 } } { user: me })
  let n <- Db.from Line |> Db.count
  Task.succeed (
    { value: r.value, lines: n }
      == { value: Ok (Response.invalid "This order is not open."), lines: 0 }
  )
}

test "a staff user cannot open another user's order" = {
  let ann <- Db.insert User {
    email: Text.Email "ann@example.com",
    name: Text.NonEmpty "Ann",
    passwordHash: "",
  }
  let bob <- Db.insert User {
    email: Text.Email "bob@example.com",
    name: Text.NonEmpty "Bob",
    passwordHash: "",
  }
  let oid <- Db.insert Order { owner: ann }
  let _ <- Test.login { user: Just bob }
  let r <- Test.open (Show { id: oid })
  Task.succeed (r.answer == PageMissing)
}

test "a manager can open another user's order" = {
  let ann <- Db.insert User {
    email: Text.Email "ann@example.com",
    name: Text.NonEmpty "Ann",
    passwordHash: "",
  }
  let mia <- Db.insert User {
    email: Text.Email "mia@example.com",
    name: Text.NonEmpty "Mia",
    passwordHash: "",
    role: Manager,
  }
  let oid <- Db.insert Order { owner: ann }
  let _ <- Test.login { user: Just mia }
  let r <- Test.open (Show { id: oid })
  Task.succeed (r.answer == Shown)
}

--- file: app/Products/Index.alder ---
module Products.Index

import Layout
import Product
import User

route Index  = GET   "/products"      { }                         requires user
route Add    = POST  "/products"      { } form NewProduct         requires user
route Toggle = PATCH "/products/{id}" { id: Product.ProductId }   requires user

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

form NewProduct = { name: Text.NonEmpty, price: Int.NonNegative }

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
      {for p in model.products ->
        <li>
          <span class="name">{p.name}</span>
          <span class="price">{Text.fromInt p.price}</span>
          <span class="active">{if p.active then "active" else "inactive"}</span>
          <al-button variant="secondary" size="sm" click={Toggle { id: p.id }}>Toggle active</al-button>
        </li>}
    </ul>
    <a href={Orders.Index.Index {}}>Orders</a>
  </Layout.Page>
