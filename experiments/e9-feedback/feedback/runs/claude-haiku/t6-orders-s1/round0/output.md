--- file: app/User.alder ---
```alder
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

-- roleOf is the role of the user with this id. A missing user gets the least power, Staff.
roleOf : UserId -> Query Role
roleOf id = {
  let found <- Db.get User id
  match found {
    Just u -> Query.succeed u.role,
    Nothing -> Query.succeed Staff,
  }
}
```

--- file: app/App.alder ---
```alder
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
```

--- file: app/Layout.alder ---
```alder
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
```

--- file: app/Product.alder ---
```alder
module Product

type ProductId = brand Text

table Product {
  id: ProductId @id,
  name: Text.NonEmpty,
  price: Int.NonNegative,
  active: Bool = True,
}
```

--- file: app/Order.alder ---
```alder
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
```

--- file: app/Line.alder ---
```alder
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

-- lineAmount is what one line costs: its quantity times its product's price in cents. Nothing
-- when the product is not in the list.
lineAmount : Line -> List Product -> Maybe Int
lineAmount line products =
  Maybe.map (\p -> line.quantity * p.price) (List.find (\p -> p.id == line.product) products)

-- orderTotal sums, in cents, the lines that belong to one order.
orderTotal : Order.OrderId -> List Line -> List Product -> Int
orderTotal id lines products =
  lines
    |> List.filter (\l -> l.order == id)
    |> List.filterMap (\l -> lineAmount l products)
    |> List.sum

test "an order's total is the sum of its lines" =
  orderTotal (Order.OrderId "o1") [
    { id: LineId "l1", order: Order.OrderId "o1", product: Product.ProductId "p1", quantity: Int.Positive 2 },
    { id: LineId "l2", order: Order.OrderId "o1", product: Product.ProductId "p2", quantity: Int.Positive 1 },
    { id: LineId "l3", order: Order.OrderId "o2", product: Product.ProductId "p1", quantity: Int.Positive 9 },
  ] [
    { id: Product.ProductId "p1", name: Text.NonEmpty "Tea", price: Int.NonNegative 500, active: True },
    { id: Product.ProductId "p2", name: Text.NonEmpty "Cup", price: Int.NonNegative 250, active: True },
  ] == 1250
```

--- file: app/Orders/Index.alder ---
```alder
module Orders.Index

import Order
import Line
import Product
import Layout
import User

page "/" { } requires user

type OrderRow = { id: Order.OrderId, status: Order.Status, total: Int }

type Model = {
  rows: List OrderRow,
  manager: Bool,
}

toRow : List Line -> List Product -> Order -> OrderRow
toRow lines products o = { id: o.id, status: o.status, total: Line.orderTotal o.id lines products }

load : Params -> Session -> Query Model
load params session = {
  let orders <-
    Db.from Order
      |> Db.where (Order.owner.eq session.user)
      |> Db.orderBy Order.created.desc
      |> Db.all
  let lines <- Db.from Line |> Db.where (Line.order.in (List.map (\o -> o.id) orders)) |> Db.all
  let products <- Db.from Product |> Db.all
  let role <- User.roleOf session.user
  Query.succeed { rows: List.map (toRow lines products) orders, manager: role == Manager }
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
    <form submit={Create}>
      <button>New order</button>
    </form>
    <ul id="orders">
      {for row in model.rows ->
        <li>
          <a href={Orders.Show { id: row.id }}>View</a>
          <span class="status">{Order.statusLabel row.status}</span>
          <span class="total">{Text.fromInt row.total}</span>
        </li>}
    </ul>
    {if model.manager then <a href={Products.Index {}}>Products</a> else Html.none}
    <button click={App.Logout}>Log out</button>
  </Layout.Page>
```

--- file: app/Orders/Show.alder ---
```alder
module Orders.Show

import Order
import Line
import Product
import Layout
import User

page "/orders/{id}" { id: Order.OrderId } requires user

type LineView = { name: Text, quantity: Int, amount: Int }

type Model = {
  order: Order,
  lines: List LineView,
  total: Int,
  products: List Product,
}

form NewLine = { quantity: Int.Positive }

msg AddLine { product: Product.ProductId } form NewLine
msg Pay
msg Cancel

-- lineView is one line as the page shows it.
lineView : Line -> Product -> LineView
lineView line p = { name: p.name, quantity: line.quantity, amount: line.quantity * p.price }

-- lineRow is the page's row for a line, when its product is known.
lineRow : List Product -> Line -> Maybe LineView
lineRow products line =
  Maybe.map (lineView line) (List.find (\p -> p.id == line.product) products)

-- pageFor is the model of an order the session may see.
pageFor : Order -> Query Model
pageFor order = {
  let lines <- Db.from Line |> Db.where (Line.order.eq order.id) |> Db.all
  let allProducts <- Db.from Product |> Db.orderBy Product.name.asc |> Db.all
  Query.succeed {
    order: order,
    lines: List.filterMap (lineRow allProducts) lines,
    total: Line.orderTotal order.id lines allProducts,
    products: List.filter (\p -> p.active) allProducts,
  }
}

load : Params -> Session -> Query Model
load params session = {
  let found <- Db.get Order params.id
  let role <- User.roleOf session.user
  match found {
    Just order -> if order.owner == session.user || role == Manager then pageFor order else Query.notFound,
    Nothing -> Query.notFound,
  }
}

-- visibleOrder is the order a message names, when the session may act on it: its owner, or a
-- Manager. Otherwise the message is a 404 (authorize in update too).
visibleOrder : Order.OrderId -> Session -> Task Order
visibleOrder id session = {
  let found <- Db.get Order id
  let role <- User.roleOf session.user
  match found {
    Just order -> if order.owner == session.user || role == Manager then Task.succeed order else Task.notFound,
    Nothing -> Task.notFound,
  }
}

-- openOrder is an order that may still change. A Paid or Cancelled order refuses the change,
-- even when the form came from a page loaded while it was Open.
openOrder : Order.OrderId -> Session -> Task Order
openOrder id session = {
  let order <- visibleOrder id session
  if order.status == Open then Task.succeed order else Task.fail "the order is not open"
}

activeProduct : Maybe Product -> Task Product
activeProduct found = match found {
  Just product -> if product.active then Task.succeed product else Task.fail "that product is not active",
  Nothing -> Task.notFound,
}

update : Msg -> Model -> Session -> Task Nav
update msg model session = match msg {
  AddLine p -> {
    let order <- openOrder model.order.id session
    let found <- Db.get Product p.product
    let product <- activeProduct found
    let _ <- Db.insert Line { order: order.id, product: product.id, quantity: p.form.quantity }
    Task.succeed (Redirect (Orders.Show { id: order.id }))
  },
  Pay p -> {
    let order <- openOrder model.order.id session
    let _ <- Db.update Order order.id (\o -> { o | status: Paid })
    Task.succeed (Redirect (Orders.Show { id: order.id }))
  },
  Cancel p -> {
    let order <- openOrder model.order.id session
    let _ <- Db.update Order order.id (\o -> { o | status: Cancelled })
    Task.succeed (Redirect (Orders.Show { id: order.id }))
  },
}

-- OpenControls are the forms and buttons of an Open order.
OpenControls : { products: List Product } -> Html Msg
OpenControls props =
  <div>
    {for p in props.products ->
      <form submit={AddLine { product: p.id }}>
        <Errors form />
        <Input field="quantity" label="Quantity" />
        <Errors field="quantity" />
        <button>Add {p.name}</button>
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
      {for l in model.lines ->
        <li><span class="name">{l.name}</span> x <span class="quantity">{Text.fromInt l.quantity}</span> = <span class="amount">{Text.fromInt l.amount}</span></li>}
    </ul>
    <p>Total: <span id="total">{Text.fromInt model.total}</span></p>
    {if model.order.status == Open then <OpenControls products={model.products} /> else Html.none}
    <a href={Orders.Index {}}>Back</a>
    <button click={App.Logout}>Log out</button>
  </Layout.Page>

test "an open order's total is the sum of its lines" = {
  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
  let tea <- Db.insert Product { name: Text.NonEmpty "Tea", price: Int.NonNegative 500 }
  let cup <- Db.insert Product { name: Text.NonEmpty "Cup", price: Int.NonNegative 250 }
  let id <- Db.insert Order { owner: me }
  let _ <- Db.insert Line { order: id, product: tea, quantity: Int.Positive 2 }
  let _ <- Db.insert Line { order: id, product: cup, quantity: Int.Positive 1 }
  let _ <- Test.login { user: Just me }
  let page <- Test.get (Orders.Show { id: id })
  Task.succeed ({ total: Test.text "#total" page.doc, status: Test.text "#status" page.doc } == { total: Just "1250", status: Just "Open" })
}

test "adding a line to a paid order is refused" = {
  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
  let tea <- Db.insert Product { name: Text.NonEmpty "Tea", price: Int.NonNegative 500 }
  let id <- Db.insert Order { owner: me }
  let _ <- Test.login { user: Just me }
  let _ <- Test.get (Orders.Show { id: id })
  let _ <- Db.update Order id (\o -> { o | status: Paid })
  let r <- Test.submit (AddLine { product: tea }) { quantity: Int.Positive 2 }
  let lines <- Db.from Line |> Db.where (Line.order.eq id) |> Db.count
  Task.succeed ({ answer: r.answer, lines: lines } == { answer: RequestFailed "the order is not open", lines: 0 })
}

test "a staff user opening another user's order is not found" = {
  let owner <- Db.insert User { email: Text.Email "owner@example.com", name: Text.NonEmpty "Owner", passwordHash: "" }
  let staffer <- Db.insert User { email: Text.Email "sam@example.com", name: Text.NonEmpty "Sam", passwordHash: "" }
  let id <- Db.insert Order { owner: owner }
  let _ <- Test.login { user: Just staffer }
  let r <- Test.open (Orders.Show { id: id })
  Task.succeed (r.answer == PageMissing)
}

test "a manager opening another user's order is shown it" = {
  let owner <- Db.insert User { email: Text.Email "owner@example.com", name: Text.NonEmpty "Owner", passwordHash: "" }
  let boss <- Db.insert User { email: Text.Email "boss@example.com", name: Text.NonEmpty "Boss", passwordHash: "", role: Manager }
  let id <- Db.insert Order { owner: owner }
  let _ <- Test.login { user: Just boss }
  let r <- Test.open (Orders.Show { id: id })
  Task.succeed (r.answer == Shown)
}
```

--- file: app/Products/Index.alder ---
```alder
module Products.Index

import Product
import Layout
import User

page "/products" { } requires user

type Model = { products: List Product }

load : Params -> Session -> Query Model
load params session = {
  let role <- User.roleOf session.user
  match role {
    Manager -> {
      let products <- Db.from Product |> Db.orderBy Product.name.asc |> Db.all
      Query.succeed { products: products },
    },
    Staff -> Query.notFound,
  }
}

form NewProduct = {
  name: Text.NonEmpty,
  price: Int.NonNegative,
}

msg Add form NewProduct
msg Toggle { id: Product.ProductId }

-- requireManager ends the request with a 404 for a Staff user.
requireManager : Session -> Task Unit
requireManager session = {
  let role <- User.roleOf session.user
  match role {
    Manager -> Task.succeed (),
    Staff -> Task.notFound,
  }
}

update : Msg -> Model -> Session -> Task Nav
update msg model session = {
  let _ <- requireManager session
  match msg {
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
}

view : Model -> Session -> Html Msg
view model session =
  <Layout.Page title="Products">
    <h1>Products</h1>
    <form submit={Add}>
      <Input field="name" label="Name" />
      <Errors field="name" />
      <Input field="price" label="Price in cents" />
      <Errors field="price" />
      <button>Add product</button>
    </form>
    <ul id="products">
      {for p in model.products ->
        <li>
          <span class="name">{p.name}</span>
          <span class="price">{Text.fromInt p.price}</span>
          <span class="state">{if p.active then "active" else "inactive"}</span>
          <button click={Toggle { id: p.id }}>{if p.active then "Deactivate" else "Activate"}</button>
        </li>}
    </ul>
    <a href={Orders.Index {}}>Orders</a>
    <button click={App.Logout}>Log out</button>
  </Layout.Page>

test "a staff user cannot open the product list" = {
  let sam <- Db.insert User { email: Text.Email "sam@example.com", name: Text.NonEmpty "Sam", passwordHash: "" }
  let _ <- Test.login { user: Just sam }
  let r <- Test.open (Products.Index {})
  Task.succeed (r.answer == PageMissing)
}
```

--- file: app/Auth/Login.alder ---
```alder
module Auth.Login

import User
import Layout

-- Password checking is outside the language: a capability implemented in Go.
capability Password {
  verify : { email: Text.Email, password: Text } -> Task (Maybe User.UserId),
}

-- In a test, `Test.fake Password` puts this in the adapter's place.
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
```
