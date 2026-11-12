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

test "logout goes to log in and clears the user" = {
  let me <- Db.insert User {
    email: Text.Email "me@example.com",
    name: Text.NonEmpty "Me",
    passwordHash: "",
  }
  let _ <- Test.login { user: Just me }
  let _ <- Test.get (Orders.Index.Index {})
  let out <- Test.click Logout
  Task.succeed (
    { out: out.answer, user: out.session.user }
      == { out: Redirected (Auth.Login.Login {}), user: Nothing }
  )
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

test "logging in goes to the orders" = {
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

-- The price of one line: its quantity times the product's price, in cents.
priceOf : List Product -> Line -> Int
priceOf products line =
  match List.find (\p -> p.id == line.product) products {
    Just p -> p.price * line.quantity,
    Nothing -> 0,
  }

-- The name of a line's product, for the page.
productName : List Product -> Line -> Text
productName products line =
  match List.find (\p -> p.id == line.product) products {
    Just p -> "{p.name}",
    Nothing -> "?",
  }

-- The total in cents of the lines that belong to one order.
totalOf : List Product -> List Line -> Order.OrderId -> Int
totalOf products lines order =
  lines
    |> List.filter (\l -> l.order == order)
    |> List.map (priceOf products)
    |> List.sum

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
statusText s = match s {
  Open -> "Open",
  Paid -> "Paid",
  Cancelled -> "Cancelled",
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

Index.load : Index.Params -> Session -> Query (List Row)
Index.load params session = {
  let orders <-
    Db.from Order
      |> Db.where (Order.owner.eq session.user)
      |> Db.orderBy Order.created.desc
      |> Db.all
  let lines <- Db.from Line |> Db.all
  let products <- Db.from Product |> Db.all
  Query.succeed (List.map (\o -> { order: o, total: Line.totalOf products lines o.id }) orders)
}

Create.update : Create.Params -> Session -> Task Response
Create.update p session = {
  let id <- Db.insert Order { owner: session.user }
  Task.succeed (Response.redirect (Orders.Show.Show { id: id }))
}

Index.view : List Row -> Session -> Html
Index.view rows session =
  <Layout.Page title="Orders">
    <h1>Orders</h1>
    <al-button click={Create}>New order</al-button>
    <ul id="orders">
      {for r in rows ->
        <li>
          <span class="status">{Order.statusText r.order.status}</span>
          <span class="total">{Text.fromInt r.total}</span>
          <a href={Orders.Show.Show { id: r.order.id }}>Open</a>
        </li>}
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
route Pay     = POST "/orders/{id}/pay"    { id: Order.OrderId } requires user
route Cancel  = POST "/orders/{id}/cancel" { id: Order.OrderId } requires user

form NewLine = { quantity: Int.Positive }

type Model = { order: Order, lines: List Line, products: List Product, available: List Product }

-- The order if the signed-in user may see it: their own, or any order for a Manager.
-- Anyone else gets a 404, as if the order did not exist.
visibleOrder : User.UserId -> Order.OrderId -> Query Order
visibleOrder user id = {
  let found <- Db.get Order id
  let manager <- User.isManager user
  match found {
    Just o -> {
      if o.owner == user || manager then Query.succeed o else Query.notFound
    },
    Nothing -> Query.notFound,
  }
}

-- The same check for write routes: a Task, so a refused write is a 404 and changes nothing.
ownOrderTask : User.UserId -> Order.OrderId -> Task Order
ownOrderTask user id = {
  let found <- Db.get Order id
  let manager <- User.isManager user
  match found {
    Just o -> {
      if o.owner == user || manager then Task.succeed o else Task.notFound
    },
    Nothing -> Task.notFound,
  }
}

Show.load : Show.Params -> Session -> Query Model
Show.load params session = {
  let order <- visibleOrder session.user params.id
  let lines <-
    Db.from Line
      |> Db.where (Line.order.eq params.id)
      |> Db.all
  let products <- Db.from Product |> Db.all
  let available <-
    Db.from Product
      |> Db.where (Product.active.eq True)
      |> Db.all
  Query.succeed { order: order, lines: lines, products: products, available: available }
}

-- Adding a line is refused unless the order is still Open, whatever page it was posted from.
AddLine.update : AddLine.Params -> Session -> Task Response
AddLine.update p session = {
  let order <- ownOrderTask session.user p.id
  if order.status != Open then
    Task.succeed (Response.invalid "This order is no longer open.")
  else {
    let _ <- Db.insert Line { order: p.id, product: p.product, quantity: p.form.quantity }
    Task.succeed (Response.redirect (Show { id: p.id }))
  }
}

Pay.update : Pay.Params -> Session -> Task Response
Pay.update p session = {
  let order <- ownOrderTask session.user p.id
  if order.status != Open then
    Task.succeed (Response.invalid "This order is no longer open.")
  else {
    let _ <- Db.update Order order.id (\o -> { o | status: Paid })
    Task.succeed (Response.redirect (Show { id: p.id }))
  }
}

Cancel.update : Cancel.Params -> Session -> Task Response
Cancel.update p session = {
  let order <- ownOrderTask session.user p.id
  if order.status != Open then
    Task.succeed (Response.invalid "This order is no longer open.")
  else {
    let _ <- Db.update Order order.id (\o -> { o | status: Cancelled })
    Task.succeed (Response.redirect (Show { id: p.id }))
  }
}

Show.view : Model -> Session -> Html
Show.view model session =
  <Layout.Page title="Order">
    <h1>Order</h1>
    <p id="status">{Order.statusText model.order.status}</p>
    <ul>
      {for l in model.lines -> <li>{Line.productName model.products l} × {Text.fromInt l.quantity}</li>}
    </ul>
    <p id="total">{Text.fromInt (Line.totalOf model.products model.lines model.order.id)}</p>
    {if model.order.status == Open then
      <div>
        {for p in model.available ->
          <form submit={AddLine { id: model.order.id, product: p.id }}>
            <span>{p.name}</span>
            <al-input field="quantity" label="Quantity" />
            <Errors form />
            <al-button type="submit">Add</al-button>
          </form>}
        <al-button click={Pay { id: model.order.id }}>Mark paid</al-button>
        <al-button variant="destructive" click={Cancel { id: model.order.id }}>Cancel order</al-button>
      </div>
    else Html.none}
    <a href={Orders.Index.Index {}}>Back</a>
  </Layout.Page>

test "an open order totals its lines" = {
  let me <- Db.insert User {
    email: Text.Email "me@example.com",
    name: Text.NonEmpty "Me",
    passwordHash: "",
  }
  let mug <- Db.insert Product { name: Text.NonEmpty "Mug", price: Int.NonNegative 500 }
  let tea <- Db.insert Product { name: Text.NonEmpty "Tea", price: Int.NonNegative 250 }
  let order <- Db.insert Order { owner: me }
  let _ <- Db.insert Line { order: order, product: mug, quantity: Int.Positive 2 }
  let _ <- Db.insert Line { order: order, product: tea, quantity: Int.Positive 1 }
  let _ <- Test.login { user: Just me }
  let page <- Test.get (Show { id: order })
  Task.succeed (
    { total: Test.text "#total" page.doc, status: Test.text "#status" page.doc }
      == { total: Just "1250", status: Just "Open" }
  )
}

test "adding a line to a paid order is refused" = {
  let me <- Db.insert User {
    email: Text.Email "me@example.com",
    name: Text.NonEmpty "Me",
    passwordHash: "",
  }
  let mug <- Db.insert Product { name: Text.NonEmpty "Mug", price: Int.NonNegative 500 }
  let order <- Db.insert Order { owner: me }
  let _ <- Test.login { user: Just me }
  let _ <- Test.get (Show { id: order })
  let _ <- Db.update Order order (\o -> { o | status: Paid })
  let r <- Test.submit (AddLine { id: order, product: mug }) { quantity: Int.Positive 3 }
  let lines <- Db.from Line |> Db.count
  Task.succeed ({ status: r.status, lines: lines } == { status: 422, lines: 0 })
}

test "a staff user opening another user's order is not found" = {
  let me <- Db.insert User {
    email: Text.Email "staff@example.com",
    name: Text.NonEmpty "Sam",
    passwordHash: "",
  }
  let other <- Db.insert User {
    email: Text.Email "other@example.com",
    name: Text.NonEmpty "Other",
    passwordHash: "",
  }
  let order <- Db.insert Order { owner: other }
  let _ <- Test.login { user: Just me }
  let r <- Test.open (Show { id: order })
  Task.succeed (r.answer == PageMissing)
}

test "a manager opening another user's order is shown" = {
  let boss <- Db.insert User {
    email: Text.Email "boss@example.com",
    name: Text.NonEmpty "Boss",
    passwordHash: "",
    role: Manager,
  }
  let other <- Db.insert User {
    email: Text.Email "other@example.com",
    name: Text.NonEmpty "Other",
    passwordHash: "",
  }
  let order <- Db.insert Order { owner: other }
  let _ <- Test.login { user: Just boss }
  let page <- Test.get (Show { id: order })
  Task.succeed (Test.text "#status" page.doc == Just "Open")
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

--- file: app/Products/Index.alder ---
module Products.Index

import Layout
import Product
import User

route Index  = GET  "/products"             { } requires user
route Add    = POST "/products"             { } form NewProduct requires user
route Toggle = POST "/products/{id}/toggle" { id: Product.ProductId } requires user

form NewProduct = { name: Text.NonEmpty, price: Int.NonNegative }

type Model = { products: List Product }

Index.load : Index.Params -> Session -> Query Model
Index.load params session = {
  let _ <- User.requireManager session.user
  let products <-
    Db.from Product
      |> Db.orderBy Product.name.asc
      |> Db.all
  Query.succeed { products: products }
}

Add.update : Add.Params -> Session -> Task Response
Add.update p session = {
  let _ <- User.requireManagerTask session.user
  let _ <- Db.insert Product { name: p.form.name, price: p.form.price }
  Task.succeed (Response.redirect (Index {}))
}

Toggle.update : Toggle.Params -> Session -> Task Response
Toggle.update p session = {
  let _ <- User.requireManagerTask session.user
  let found <- Db.get Product p.id
  match found {
    Just _ -> {
      let _ <- Db.update Product p.id (\x -> { x | active: not x.active })
      Task.succeed (Response.redirect (Index {}))
    },
    Nothing -> Task.notFound,
  }
}

Index.view : Model -> Session -> Html
Index.view model session =
  <Layout.Page title="Products">
    <h1>Products</h1>
    <ul id="products">
      {for p in model.products ->
        <li>
          <span class="name">{p.name}</span>
          <span class="price">{Text.fromInt p.price}</span>
          <al-button click={Toggle { id: p.id }}>{if p.active then "Deactivate" else "Activate"}</al-button>
        </li>}
    </ul>
    <form submit={Add}>
      <al-input field="name" label="Name" />
      <al-input field="price" label="Price (cents)" />
      <Errors form />
      <al-button type="submit">Add product</al-button>
    </form>
    <a href={Orders.Index.Index {}}>Orders</a>
  </Layout.Page>

test "a staff user is not found on the products page" = {
  let me <- Db.insert User {
    email: Text.Email "staff@example.com",
    name: Text.NonEmpty "Sam",
    passwordHash: "",
  }
  let _ <- Test.login { user: Just me }
  let r <- Test.open (Index {})
  Task.succeed (r.answer == PageMissing)
}

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

-- The session holds only the user, so a role check reads the user's row again on each request.
isManager : UserId -> Query Bool
isManager id = {
  let found <- Db.get User id
  Query.succeed (Maybe.map (\u -> u.role) found == Just Manager)
}

requireManager : UserId -> Query Unit
requireManager id = {
  let manager <- isManager id
  if manager then Query.succeed () else Query.notFound
}

requireManagerTask : UserId -> Task Unit
requireManagerTask id = {
  let manager <- isManager id
  if manager then Task.succeed () else Task.notFound
}
