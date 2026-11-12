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

--- file: app/Product.alder ---
module Product

type ProductId = brand Text

table Product {
  id: ProductId @id,
  name: Text.NonEmpty,
  price: Int.NonNegative,
  active: Bool = True,
}

test "a new product is active by default" = {
  let p <- Db.insert Product { name: Text.NonEmpty "Mug", price: Int.NonNegative 500 }
  let row <- Db.get Product p
  Task.succeed (Maybe.map (\r -> r.active) row == Just True)
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
import Order
import Line
import Product
import User

route Index  = GET  "/"              { } requires user
route Create = POST "/orders"        { } requires user

type Row = { order: Order, total: Int }

Index.load : Index.Params -> Session -> Query (List Row)
Index.load params session = {
  let orders <-
    Db.mine Order
      |> Db.orderBy Order.created.desc
      |> Db.all
  let rows <- Query.all (List.map (\o -> {
    let lines <- Db.from Line |> Db.where (Line.order.eq o.id) |> Db.all
    let prices <- Query.all (List.map (\l -> {
      let p <- Db.get Product l.product
      Query.succeed (l.quantity * Maybe.withDefault 0 (Maybe.map (\x -> x.price) p))
    }) lines)
    Query.succeed { order: o, total: List.sum prices }
  }) orders)
  Query.succeed rows
}

Create.update : Create.Params -> Session -> Task Response
Create.update p session = {
  let id <- Db.insertMine Order {}
  Task.succeed (Response.redirect (Orders.Show.Show { id: id }))
}

Index.view : List Row -> Session -> Html
Index.view rows session =
  <Layout.Page title="Orders">
    <h1>Orders</h1>
    <form submit={Create}>
      <al-button type="submit">New order</al-button>
    </form>
    <ul id="orders">
      {for r in rows ->
        <li>
          <span class="status">{statusText r.order.status}</span>
          <span class="total">{Text.fromInt r.total}</span>
          <a href={Orders.Show.Show { id: r.order.id }}>Open</a>
        </li>}
    </ul>
    <al-button variant="outline" click={App.Logout}>Log out</al-button>
  </Layout.Page>

statusText : Order.Status -> Text
statusText s = match s {
  Open -> "Open",
  Paid -> "Paid",
  Cancelled -> "Cancelled",
}

--- file: app/Orders/Show.alder ---
module Orders.Show

import Layout
import Order
import Line
import Product
import User

route Show   = GET  "/orders/{id}" { id: Order.OrderId } requires user, role Manager | Staff
route AddLine = POST "/orders/{id}/lines" { id: Order.OrderId, product: Product.ProductId } form NewLine requires user
route Pay    = POST "/orders/{id}/pay"    { id: Order.OrderId } requires user
route Cancel = POST "/orders/{id}/cancel" { id: Order.OrderId } requires user

form NewLine = { quantity: Int.Positive }

type Model = { order: Order, lines: List { line: Line, product: Product }, total: Int, products: List Product }

Show.load : Show.Params -> Session -> Query Model
Show.load params session = {
  let order <- Db.getMine Order params.id
  let lines <- Db.from Line |> Db.where (Line.order.eq params.id) |> Db.all
  let products <- Db.from Product |> Db.where (Product.active.eq True) |> Db.all
  let pairs <- Query.all (List.map (\l -> {
    let p <- Db.get Product l.product
    Query.succeed { line: l, product: Maybe.withDefault { id: l.product, name: Text.NonEmpty "?", price: Int.NonNegative 0, active: False } p }
  }) lines)
  let total = List.sum (List.map (\x -> x.line.quantity * x.product.price) pairs)
  Query.succeed { order: order, lines: pairs, total: total, products: products }
}

-- Adding a line is refused unless the order is still Open, whatever page it was posted from.
AddLine.update : AddLine.Params -> Session -> Task Response
AddLine.update p session = {
  let order <- Db.getMine Order p.id
  if order.status != Open then
    Task.succeed (Response.invalid "This order is no longer open.")
  else {
    let _ <- Db.insertMine Line { order: p.id, product: p.product, quantity: p.form.quantity }
    Task.succeed (Response.redirect (Orders.Show.Show { id: p.id }))
  }
}

Pay.update : Pay.Params -> Session -> Task Response
Pay.update p session = {
  let _ <- Db.updateMine Order p.id (\o -> { o | status: Paid })
  Task.succeed (Response.redirect (Orders.Show.Show { id: p.id }))
}

Cancel.update : Cancel.Params -> Session -> Task Response
Cancel.update p session = {
  let _ <- Db.updateMine Order p.id (\o -> { o | status: Cancelled })
  Task.succeed (Response.redirect (Orders.Show.Show { id: p.id }))
}

Show.view : Model -> Session -> Html
Show.view model session =
  <Layout.Page title="Order">
    <h1>Order</h1>
    <p id="status">{statusText model.order.status}</p>
    <ul>
      {for x in model.lines ->
        <li>{x.product.name} × {Text.fromInt x.line.quantity}</li>}
    </ul>
    <p id="total">{Text.fromInt model.total}</p>
    {if model.order.status == Open then
      <div>
        {for p in model.products ->
          <form submit={AddLine { id: model.order.id, product: p.id }}>
            <span>{p.name}</span>
            <al-input field="quantity" label="Quantity" />
            <al-button type="submit">Add</al-button>
          </form>}
        <al-button click={Pay { id: model.order.id }}>Mark paid</al-button>
        <al-button variant="destructive" click={Cancel { id: model.order.id }}>Cancel order</al-button>
      </div>
    else Html.none}
    <a href={Orders.Index.Index {}}>Back</a>
  </Layout.Page>

statusText : Order.Status -> Text
statusText s = match s {
  Open -> "Open",
  Paid -> "Paid",
  Cancelled -> "Cancelled",
}

--- file: app/Products/Index.alder ---
module Products.Index

import Layout
import Product
import User

route Index  = GET  "/products"          { } requires user, role Manager
route Add    = POST "/products"          { } form NewProduct requires user, role Manager
route Toggle = POST "/products/{id}/toggle" { id: Product.ProductId } requires user, role Manager

form NewProduct = { name: Text.NonEmpty, price: Int.NonNegative }

type Model = { products: List Product }

Index.load : Index.Params -> Session -> Query Model
Index.load params session = {
  let products <- Db.from Product |> Db.orderBy Product.name.asc |> Db.all
  Query.succeed { products: products }
}

Add.update : Add.Params -> Session -> Task Response
Add.update p session = {
  let _ <- Db.insert Product { name: p.form.name, price: p.form.price }
  Task.succeed (Response.redirect (Products.Index.Index {}))
}

Toggle.update : Toggle.Params -> Session -> Task Response
Toggle.update p session = {
  let found <- Db.get Product p.id
  match found {
    Just prod -> {
      let _ <- Db.update Product p.id (\x -> { x | active: not x.active })
      Task.succeed (Response.redirect (Products.Index.Index {}))
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
          <span>{Text.fromInt p.price}</span>
          <al-button click={Toggle { id: p.id }}>{if p.active then "Deactivate" else "Activate"}</al-button>
        </li>}
    </ul>
    <form submit={Add}>
      <al-input field="name" label="Name" />
      <al-input field="price" label="Price (cents)" />
      <al-button type="submit">Add product</al-button>
    </form>
    <a href={Orders.Index.Index {}}>Orders</a>
  </Layout.Page>
