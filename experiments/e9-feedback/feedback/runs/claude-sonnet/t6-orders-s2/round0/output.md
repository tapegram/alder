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
isManager me = match me {
  Just u -> u.role == Manager,
  Nothing -> False,
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
canView : Order -> User.UserId -> Query Bool
canView order viewer = {
  let me <- Db.get User viewer
  Query.succeed (order.owner == viewer || User.isManager me)
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

priceOf : List Product -> Product.ProductId -> Int
priceOf products id = {
  let found = List.find (\p -> p.id == id) products
  Maybe.map (\p -> Int.abs p.price) found ?? 0
}

total : List Product -> List Line -> Int
total products lines =
  List.sum (List.map (\l -> l.quantity * priceOf products l.product) lines)

--- file: app/Orders/Index.alder ---
module Orders.Index

import Order
import Line
import Product
import User
import Layout

page "/" { } requires user

type Row = { order: Order, total: Int }

type Model = { rows: List Row, manager: Bool }

row : List Product -> List Line -> Order -> Row
row products lines o = {
  order: o,
  total: Line.total products (List.filter (\l -> l.order == o.id) lines),
}

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
  Query.succeed { rows: List.map (row products lines) orders, manager: User.isManager me }
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

    <nav>
      {if model.manager then <a href={Products.Index {}}>Products</a> else Html.none}
    </nav>

    <button click={Create}>New order</button>

    <ul id="orders">
      {for r in model.rows ->
        <li>
          <a href={Orders.Show { id: r.order.id }}>Order from {Time.format "2006-01-02 15:04" r.order.created}</a>
          <span class="status">{Order.statusLabel r.order.status}</span>
          <span class="total">{Text.fromInt r.total}</span>
        </li>}
    </ul>

    <button click={App.Logout}>Log out</button>
  </Layout.Page>

--- file: app/Orders/Show.alder ---
module Orders.Show

import Order
import Line
import Product
import User
import Layout

page "/orders/{id}" { id: Order.OrderId } requires user

type Item = { name: Text, quantity: Int, cost: Int }

type Model = {
  order: Order,
  items: List Item,
  active: List Product,
  total: Int,
}

item : List Product -> Line -> Item
item products line = {
  name: Maybe.map (\p -> Text.trim p.name) (List.find (\p -> p.id == line.product) products) ?? "?",
  quantity: Int.abs line.quantity,
  cost: line.quantity * Line.priceOf products line.product,
}

build : Order -> Query Model
build order = {
  let lines <- Db.from Line |> Db.where (Line.order.eq order.id) |> Db.all
  let products <- Db.from Product |> Db.all
  Query.succeed {
    order: order,
    items: List.map (item products) lines,
    active: List.filter (\p -> p.active) products,
    total: Line.total products lines,
  }
}

-- Another user's order is a 404 unless the viewer is a Manager.
load : Params -> Session -> Query Model
load params session = {
  let found <- Db.get Order params.id
  match found {
    Nothing -> Query.notFound,
    Just order -> {
      let ok <- Order.canView order session.user
      if ok then build order else Query.notFound
    },
  }
}

form NewLine = { quantity: Int.Positive }

msg AddLine { product: Product.ProductId } form NewLine
msg Pay
msg Cancel

-- The order's status is read again here, so a form from a page loaded while the order was
-- Open is still refused once it is not.
isOpen : Order.OrderId -> Task Bool
isOpen id = {
  let found <- Db.get Order id
  Task.succeed (Maybe.map (\o -> o.status == Open) found ?? False)
}

finish : Order.OrderId -> Order.Status -> Task Nav
finish id status = {
  let ok <- isOpen id
  if ok then {
    let _ <- Db.update Order id (\o -> { o | status: status })
    Task.succeed (Redirect (Orders.Show { id: id }))
  } else Task.succeed (Reject "This order is not open.")
}

update : Msg -> Model -> Session -> Task Nav
update msg model session = match msg {
  AddLine p -> {
    let ok <- isOpen model.order.id
    let found <- Db.get Product p.product
    let usable = Maybe.map (\x -> x.active) found ?? False
    if ok && usable then {
      let _ <- Db.insert Line { order: model.order.id, product: p.product, quantity: p.form.quantity }
      Task.succeed (Redirect (Orders.Show { id: model.order.id }))
    } else Task.succeed (Reject "This order is not open, or the product is not available.")
  },
  Pay p -> finish model.order.id Paid,
  Cancel p -> finish model.order.id Cancelled,
}

view : Model -> Session -> Html Msg
view model session =
  <Layout.Page title="Order">
    <h1>Order</h1>
    <p>Status: <span id="status">{Order.statusLabel model.order.status}</span></p>

    <ul id="lines">
      {for i in model.items ->
        <li>{i.name} x {Text.fromInt i.quantity} = {Text.fromInt i.cost}</li>}
    </ul>

    <p>Total: <span id="total">{Text.fromInt model.total}</span></p>

    {if model.order.status == Open then
      <div id="controls">
        {for prod in model.active ->
          <form submit={AddLine { product: prod.id }}>
            <Errors form />
            <h3>{prod.name}</h3>
            <Input field="quantity" label="Quantity" />
            <Errors field="quantity" />
            <button>Add line</button>
          </form>}
        <button click={Pay}>Mark paid</button>
        <button click={Cancel}>Cancel order</button>
      </div>
    else Html.none}

    <a href={Orders.Index {}}>Back</a>
  </Layout.Page>

test "an open order's total sums its lines" = {
  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
  let tea <- Db.insert Product { name: Text.NonEmpty "Tea", price: Int.NonNegative 250 }
  let cake <- Db.insert Product { name: Text.NonEmpty "Cake", price: Int.NonNegative 100 }
  let o <- Db.insert Order { owner: me }
  let _ <- Db.insert Line { order: o, product: tea, quantity: Int.Positive 2 }
  let _ <- Db.insert Line { order: o, product: cake, quantity: Int.Positive 3 }
  let _ <- Test.login { user: Just me }
  let page <- Test.get (Orders.Show { id: o })
  Task.succeed ({ total: Test.text "#total" page.doc, status: Test.text "#status" page.doc, model: page.model.total } == { total: Just "800", status: Just "Open", model: 800 })
}

test "adding a line to a paid order is refused" = {
  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
  let tea <- Db.insert Product { name: Text.NonEmpty "Tea", price: Int.NonNegative 250 }
  let o <- Db.insert Order { owner: me }
  let _ <- Test.login { user: Just me }
  let page <- Test.get (Orders.Show { id: o })
  let _ <- Db.update Order o (\x -> { x | status: Paid })
  let r <- Test.run (update (AddLine { product: tea, form: { quantity: Int.Positive 1 } }) page.model { user: me })
  let n <- Db.from Line |> Db.count
  Task.succeed ({ value: r.value, lines: n } == { value: Ok (Reject "This order is not open, or the product is not available."), lines: 0 })
}

test "a staff user does not find another user's order" = {
  let owner <- Db.insert User { email: Text.Email "owner@example.com", name: Text.NonEmpty "Owner", passwordHash: "" }
  let other <- Db.insert User { email: Text.Email "other@example.com", name: Text.NonEmpty "Other", passwordHash: "" }
  let o <- Db.insert Order { owner: owner }
  let _ <- Test.login { user: Just other }
  let r <- Test.open (Orders.Show { id: o })
  Task.succeed (r.answer == PageMissing)
}

test "a manager finds another user's order" = {
  let owner <- Db.insert User { email: Text.Email "owner@example.com", name: Text.NonEmpty "Owner", passwordHash: "" }
  let boss <- Db.insert User { email: Text.Email "boss@example.com", name: Text.NonEmpty "Boss", passwordHash: "", role: Manager }
  let o <- Db.insert Order { owner: owner }
  let _ <- Test.login { user: Just boss }
  let r <- Test.open (Orders.Show { id: o })
  Task.succeed (r.answer == Shown)
}

--- file: app/Products/Index.alder ---
module Products.Index

import Product
import User
import Layout

page "/products" { } requires user

type Model = { products: List Product }

-- Only a Manager may see this page; for everyone else it is a 404. Messages run `load`
-- first, so `update` is guarded by the same check.
load : Params -> Session -> Query Model
load params session = {
  let me <- Db.get User session.user
  if User.isManager me then {
    let products <- Db.from Product |> Db.orderBy Product.name.asc |> Db.all
    Query.succeed { products: products }
  } else Query.notFound
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
      Nothing -> Task.notFound,
      Just prod -> {
        let _ <- Db.update Product prod.id (\x -> { x | active: not x.active })
        Task.succeed (Redirect (Products.Index {}))
      },
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
      {for prod in model.products ->
        <li>
          <span class="name">{prod.name}</span>
          <span class="price">{Text.fromInt prod.price}</span>
          <span class="state">{if prod.active then "Active" else "Inactive"}</span>
          <button click={Toggle { id: prod.id }}>{if prod.active then "Deactivate" else "Activate"}</button>
        </li>}
    </ul>

    <a href={Orders.Index {}}>Back</a>
  </Layout.Page>

test "a staff user gets 404 on products" = {
  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
  let _ <- Test.login { user: Just me }
  let r <- Test.open (Products.Index {})
  Task.succeed (r.answer == PageMissing)
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
