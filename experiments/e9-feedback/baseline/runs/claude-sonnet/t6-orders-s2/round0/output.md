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

isManager : Maybe User -> Bool
isManager found = match found {
  Just user -> user.role == Manager,
  Nothing -> False,
}

test "only a Manager is a manager" =
  { staff: isManager (Just { id: UserId "a", email: Text.Email "a@example.com", name: Text.NonEmpty "A", passwordHash: "", role: Staff, created: Time.epoch }), manager: isManager (Just { id: UserId "b", email: Text.Email "b@example.com", name: Text.NonEmpty "B", passwordHash: "", role: Manager, created: Time.epoch }), nobody: isManager Nothing }
    == { staff: False, manager: True, nobody: False }

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

test "each status has its label" =
  List.map statusLabel [Open, Paid, Cancelled] == ["Open", "Paid", "Cancelled"]

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

productOf : List Product -> Line -> Maybe Product
productOf products line = List.find (\p -> p.id == line.product) products

cost : List Product -> Line -> Int
cost products line = {
  let found = productOf products line
  match found {
    Just p -> line.quantity * p.price,
    Nothing -> 0,
  }
}

productName : List Product -> Line -> Text
productName products line = {
  let found = productOf products line
  match found {
    Just p -> Text.trim p.name,
    Nothing -> "unknown",
  }
}

total : List Line -> List Product -> Int
total lines products = lines |> List.map (cost products) |> List.sum

test "the total adds quantity times price over the lines" =
  total [
    { id: LineId "l1", order: Order.OrderId "o", product: Product.ProductId "a", quantity: Int.Positive 2 },
    { id: LineId "l2", order: Order.OrderId "o", product: Product.ProductId "b", quantity: Int.Positive 1 },
  ] [
    { id: Product.ProductId "a", name: Text.NonEmpty "Tea", price: Int.NonNegative 300, active: True },
    { id: Product.ProductId "b", name: Text.NonEmpty "Cake", price: Int.NonNegative 650, active: True },
  ] == 1250

--- file: app/Auth/Login.alder ---
module Auth.Login

import User
import Layout

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

import User
import Product
import Order
import Line
import Layout

page "/" { } requires user

type Row = { order: Order, total: Int }

type Model = { rows: List Row, manager: Bool }

row : List Line -> List Product -> Order -> Row
row lines products order =
  { order: order, total: Line.total (List.filter (\l -> l.order == order.id) lines) products }

load : Params -> Session -> Query Model
load params session = {
  let me <- Db.get User session.user
  let orders <-
    Db.from Order
      |> Db.where (Order.owner.eq session.user)
      |> Db.orderBy Order.created.desc
      |> Db.all
  let lines <- Db.from Line |> Db.where (Line.order.in (List.map (\o -> o.id) orders)) |> Db.all
  let products <- Db.from Product |> Db.all
  Query.succeed { rows: List.map (row lines products) orders, manager: User.isManager me }
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
      {for r in model.rows ->
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

import User
import Product
import Order
import Line
import Layout

page "/orders/{id}" { id: Order.OrderId } requires user

type LineRow = { line: Line, name: Text, cost: Int }

type Model = {
  order: Order,
  items: List LineRow,
  products: List Product,
  total: Int,
}

lineRow : List Product -> Line -> LineRow
lineRow products line =
  { line: line, name: Line.productName products line, cost: Line.cost products line }

build : Order -> Query Model
build order = {
  let lines <- Db.from Line |> Db.where (Line.order.eq order.id) |> Db.all
  let products <- Db.from Product |> Db.all
  Query.succeed {
    order: order,
    items: List.map (lineRow products) lines,
    products: List.filter (\p -> p.active) products,
    total: Line.total lines products,
  }
}

load : Params -> Session -> Query Model
load params session = {
  let found <- Db.get Order params.id
  let me <- Db.get User session.user
  match found {
    Just order -> if order.owner == session.user || User.isManager me then build order else Query.notFound,
    Nothing -> Query.notFound,
  }
}

form NewLine = {
  quantity: Int.Positive,
}

msg AddLine { product: Product.ProductId } form NewLine
msg Pay
msg Cancel

-- The model is loaded again for every message, so its order's status is the current one: a form
-- posted from a page loaded while the order was Open is still refused once it is not.
addLine : Model -> Product.ProductId -> Int.Positive -> Task Nav
addLine model product quantity = {
  let found <- Db.get Product product
  match found {
    Just pr -> if model.order.status == Open && pr.active then insertLine model product quantity else Task.succeed (Reject "This order cannot take that line."),
    Nothing -> Task.notFound,
  }
}

insertLine : Model -> Product.ProductId -> Int.Positive -> Task Nav
insertLine model product quantity = {
  let _ <- Db.insert Line { order: model.order.id, product: product, quantity: quantity }
  Task.succeed (Redirect (Orders.Show { id: model.order.id }))
}

changeStatus : Model -> Order.Status -> Task Nav
changeStatus model status = {
  let _ <- Db.update Order model.order.id (\o -> { o | status: status })
  Task.succeed (Redirect (Orders.Show { id: model.order.id }))
}

setStatus : Model -> Order.Status -> Task Nav
setStatus model status =
  if model.order.status == Open then changeStatus model status else Task.succeed (Redirect (Orders.Show { id: model.order.id }))

update : Msg -> Model -> Session -> Task Nav
update msg model session = match msg {
  AddLine p -> addLine model p.product p.form.quantity,
  Pay p -> setStatus model Paid,
  Cancel p -> setStatus model Cancelled,
}

view : Model -> Session -> Html Msg
view model session =
  <Layout.Page title="Order">
    <h1>Order</h1>
    <p>Status: <span id="status">{Order.statusLabel model.order.status}</span></p>
    <p>Total: <span id="total">{Text.fromInt model.total}</span></p>
    <ul id="lines">
      {for row in model.items ->
        <li>
          <span class="name">{row.name}</span>
          <span class="quantity">{Text.fromInt row.line.quantity}</span>
          <span class="cost">{Text.fromInt row.cost}</span>
        </li>}
    </ul>
    {if model.order.status == Open then
      <div id="controls">
        {for product in model.products ->
          <form submit={AddLine { product: product.id }}>
            <Errors form />
            <span>{product.name}</span>
            <Input field="quantity" label="Quantity" />
            <Errors field="quantity" />
            <button>Add</button>
          </form>}
        <button click={Pay}>Mark paid</button>
        <button click={Cancel}>Cancel order</button>
      </div>
    else Html.none}
    <a href={Orders.Index {}}>Back</a>
  </Layout.Page>

test "an Open order's total adds its lines" = {
  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
  let tea <- Db.insert Product { name: Text.NonEmpty "Tea", price: Int.NonNegative 300 }
  let cake <- Db.insert Product { name: Text.NonEmpty "Cake", price: Int.NonNegative 650 }
  let order <- Db.insert Order { owner: me }
  let _ <- Db.insert Line { order: order, product: tea, quantity: Int.Positive 2 }
  let _ <- Db.insert Line { order: order, product: cake, quantity: Int.Positive 1 }
  let _ <- Test.login { user: Just me }
  let page <- Test.get (Orders.Show { id: order })
  Task.succeed ({ status: Test.text "#status" page.doc, total: Test.text "#total" page.doc, lines: Test.count "#lines li" page.doc } == { status: Just "Open", total: Just "1250", lines: 2 })
}

test "adding a line to a Paid order is refused" = {
  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
  let tea <- Db.insert Product { name: Text.NonEmpty "Tea", price: Int.NonNegative 300 }
  let order <- Db.insert Order { owner: me }
  let _ <- Test.login { user: Just me }
  let _ <- Test.get (Orders.Show { id: order })
  let _ <- Db.update Order order (\o -> { o | status: Paid })
  let _ <- Test.submit (AddLine { product: tea }) { quantity: Int.Positive 1 }
  let n <- Db.from Line |> Db.count
  Task.succeed (n == 0)
}

test "a Staff user cannot open another user's order" = {
  let owner <- Db.insert User { email: Text.Email "owner@example.com", name: Text.NonEmpty "Owner", passwordHash: "" }
  let other <- Db.insert User { email: Text.Email "other@example.com", name: Text.NonEmpty "Other", passwordHash: "" }
  let order <- Db.insert Order { owner: owner }
  let _ <- Test.login { user: Just other }
  let r <- Test.open (Orders.Show { id: order })
  Task.succeed (r.answer == PageMissing)
}

test "a Manager can open another user's order" = {
  let owner <- Db.insert User { email: Text.Email "owner@example.com", name: Text.NonEmpty "Owner", passwordHash: "" }
  let boss <- Db.insert User { email: Text.Email "boss@example.com", name: Text.NonEmpty "Boss", passwordHash: "", role: Manager }
  let order <- Db.insert Order { owner: owner }
  let _ <- Test.login { user: Just boss }
  let r <- Test.open (Orders.Show { id: order })
  Task.succeed (r.answer == Shown)
}

--- file: app/Products/Index.alder ---
module Products.Index

import User
import Product
import Layout

page "/products" { } requires user

type Model = { products: List Product }

load : Params -> Session -> Query Model
load params session = {
  let me <- Db.get User session.user
  let products <- Db.from Product |> Db.orderBy Product.name.asc |> Db.all
  if User.isManager me then Query.succeed { products: products } else Query.notFound
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
      Just product -> {
        let _ <- Db.update Product product.id (\x -> { x | active: not x.active })
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
      <Input field="price" label="Price in cents" />
      <Errors field="price" />
      <button>Add</button>
    </form>
    <ul id="products">
      {for p in model.products ->
        <li>
          <span class="name">{p.name}</span>
          <span class="price">{Text.fromInt p.price}</span>
          <span class="active">{if p.active then "Active" else "Inactive"}</span>
          <button click={Toggle { id: p.id }}>{if p.active then "Deactivate" else "Activate"}</button>
        </li>}
    </ul>
    <a href={Orders.Index {}}>Orders</a>
  </Layout.Page>

test "a Staff user gets 404 on products" = {
  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
  let _ <- Test.login { user: Just me }
  let r <- Test.open (Products.Index {})
  Task.succeed (r.answer == PageMissing)
}
