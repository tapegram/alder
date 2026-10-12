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

-- isManager says whether the row found for the session's user is a Manager; a session whose
-- user has no row is nobody's manager.
isManager : Maybe User -> Bool
isManager found = match found {
  Just u -> u.role == Manager,
  Nothing -> False,
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

activeOnly : List Product -> List Product
activeOnly products = List.filter (\p -> p.active) products

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

-- lineCents is quantity × price for one line, in cents; the product is looked up in the list.
lineCents : List Product -> Line -> Int
lineCents products line = {
  let found = List.find (\p -> p.id == line.product) products
  match found {
    Just p -> line.quantity * p.price,
    Nothing -> 0,
  }
}

total : List Product -> List Line -> Int
total products lines = lines |> List.map (lineCents products) |> List.sum

productName : List Product -> Line -> Text
productName products line = {
  let found = List.find (\p -> p.id == line.product) products
  match found {
    Just p -> "{p.name}",
    Nothing -> "",
  }
}

test "total sums quantity times price" =
  total [
    { id: Product.ProductId "tea", name: Text.NonEmpty "Tea", price: Int.NonNegative 450, active: True },
    { id: Product.ProductId "mug", name: Text.NonEmpty "Mug", price: Int.NonNegative 350, active: True },
  ] [
    { id: LineId "a", order: Order.OrderId "o", product: Product.ProductId "tea", quantity: Int.Positive 2 },
    { id: LineId "b", order: Order.OrderId "o", product: Product.ProductId "mug", quantity: Int.Positive 1 },
  ] == 1250

--- file: app/Auth/Login.alder ---
module Auth.Login

import User
import Layout

-- Password checking is outside the language: a capability implemented in Go.
capability Password {
  verify : { email: Text.Email, password: Text } -> Task (Maybe User.UserId),
}

-- In a test, `Test.fake Password` puts this in the adapter's place: it accepts any user who has
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
import Layout
import User

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
  let lines <-
    Db.from Line
      |> Db.where (Line.order.in (List.map (\o -> o.id) orders))
      |> Db.all
  let products <- Db.from Product |> Db.all
  let viewer <- Db.get User session.user
  Query.succeed { rows: List.map (row products lines) orders, manager: User.isManager viewer }
}

row : List Product -> List Line -> Order -> Row
row products lines order =
  { order: order, total: Line.total products (List.filter (\l -> l.order == order.id) lines) }

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
          <a href={Orders.Show { id: r.order.id }}>{Time.format "Jan 2, 2006 15:04" r.order.created}</a>
          {" "}
          <span class="status">{Order.statusLabel r.order.status}</span>
          {" "}
          <span class="total">{Text.fromInt r.total}</span>
        </li>}
    </ul>

    <button click={App.Logout}>Log out</button>
  </Layout.Page>

test "a created order is listed Open with total 0" = {
  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
  let _ <- Test.login { user: Just me }
  let _ <- Test.get (Orders.Index {})
  let _ <- Test.click Create
  let page <- Test.get (Orders.Index {})
  Task.succeed ({ statuses: Test.texts "#orders .status" page.doc, totals: Test.texts "#orders .total" page.doc } == { statuses: ["Open"], totals: ["0"] })
}

test "the list shows each order's total and only my orders" = {
  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
  let other <- Db.insert User { email: Text.Email "other@example.com", name: Text.NonEmpty "Other", passwordHash: "" }
  let tea <- Db.insert Product { name: Text.NonEmpty "Tea", price: Int.NonNegative 450 }
  let mine <- Db.insert Order { owner: me }
  let theirs <- Db.insert Order { owner: other }
  let _ <- Db.insert Line { order: mine, product: tea, quantity: Int.Positive 3 }
  let _ <- Db.insert Line { order: theirs, product: tea, quantity: Int.Positive 1 }
  let _ <- Test.login { user: Just me }
  let page <- Test.get (Orders.Index {})
  Task.succeed ({ items: Test.count "#orders li" page.doc, totals: Test.texts "#orders .total" page.doc } == { items: 1, totals: ["1350"] })
}

--- file: app/Orders/Show.alder ---
module Orders.Show

import Order
import Line
import Product
import Layout
import User

page "/orders/{id}" { id: Order.OrderId } requires user

type Model = {
  order: Order,
  lines: List Line,
  products: List Product,
  choices: List Product,
  total: Int,
}

-- The order is shown to its owner and to any Manager; to anyone else it is a 404.
load : Params -> Session -> Query Model
load params session = {
  let found <- Db.get Order params.id
  let viewer <- Db.get User session.user
  match found {
    Just order -> if order.owner == session.user || User.isManager viewer then loaded order else Query.notFound,
    Nothing -> Query.notFound,
  }
}

loaded : Order -> Query Model
loaded order = {
  let lines <- Db.from Line |> Db.where (Line.order.eq order.id) |> Db.all
  let products <- Db.from Product |> Db.orderBy Product.name.asc |> Db.all
  Query.succeed {
    order: order,
    lines: lines,
    products: products,
    choices: Product.activeOnly products,
    total: Line.total products lines,
  }
}

form NewLine = {
  quantity: Int.Positive,
}

-- The messages carry no order id: the page's parameter names the order, and `load` has checked
-- that the session's user may see it, so `update` acts on `model.order`.
msg AddLine { product: Product.ProductId } form NewLine
msg Pay
msg Cancel

update : Msg -> Model -> Session -> Task Nav
update msg model session = match msg {
  AddLine p -> {
    let order <- openOrder model.order.id
    let product <- activeProduct p.product
    let _ <- Db.insert Line { order: order.id, product: product.id, quantity: p.form.quantity }
    Task.succeed (Redirect (Orders.Show { id: order.id }))
  },
  Pay p -> {
    let order <- openOrder model.order.id
    let _ <- Db.update Order order.id (\o -> { o | status: Paid })
    Task.succeed (Redirect (Orders.Show { id: order.id }))
  },
  Cancel p -> {
    let order <- openOrder model.order.id
    let _ <- Db.update Order order.id (\o -> { o | status: Cancelled })
    Task.succeed (Redirect (Orders.Show { id: order.id }))
  },
}

-- openOrder reads the order again inside the task and refuses one that is not Open, so a form
-- from a page loaded while the order was still Open cannot change it afterwards.
-- The refusal is `Task.notFound`, not `Task.fail`: a Paid or Cancelled order has no "add line",
-- "pay" or "cancel" action, so a message that asks for one names something that is not there.
-- That is a 404 for a stale or forged request, where `Task.fail` would answer 500 and log an
-- error as if the server had broken. Either way nothing is written.
openOrder : Order.OrderId -> Task Order
openOrder id = {
  let found <- Db.get Order id
  match found {
    Just order -> if order.status == Open then Task.succeed order else Task.notFound,
    Nothing -> Task.notFound,
  }
}

-- The product id comes from the request: it must name a product that exists and is active.
activeProduct : Product.ProductId -> Task Product
activeProduct id = {
  let found <- Db.get Product id
  match found {
    Just product -> if product.active then Task.succeed product else Task.notFound,
    Nothing -> Task.notFound,
  }
}

view : Model -> Session -> Html Msg
view model session =
  <Layout.Page title="Order">
    <h1>Order</h1>

    <p>Status: <span id="status">{Order.statusLabel model.order.status}</span></p>

    <ul id="lines">
      {for line in model.lines ->
        <li>
          <span class="product">{Line.productName model.products line}</span>
          {" "}
          <span class="quantity">{Text.fromInt line.quantity}</span>
          {" "}
          <span class="cents">{Text.fromInt (Line.lineCents model.products line)}</span>
        </li>}
    </ul>

    <p>Total: <span id="total">{Text.fromInt model.total}</span></p>

    {if model.order.status == Open then <Controls choices={model.choices} /> else Html.none}

    <a href={Orders.Index {}}>Back</a>
  </Layout.Page>

Controls : { choices: List Product } -> Html Msg
Controls props =
  <section id="controls">
    <ul id="add">
      {for product in props.choices ->
        <li>
          <form submit={AddLine { product: product.id }}>
            <span class="choice">{product.name}</span>
            <Input field="quantity" label="Quantity" />
            <Errors field="quantity" />
            <button>Add</button>
          </form>
        </li>}
    </ul>
    <button click={Pay}>Mark paid</button>
    <button click={Cancel}>Cancel order</button>
  </section>

test "an Open order's total over two lines" = {
  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
  let tea <- Db.insert Product { name: Text.NonEmpty "Tea", price: Int.NonNegative 450 }
  let mug <- Db.insert Product { name: Text.NonEmpty "Mug", price: Int.NonNegative 350 }
  let id <- Db.insert Order { owner: me }
  let _ <- Db.insert Line { order: id, product: tea, quantity: Int.Positive 2 }
  let _ <- Db.insert Line { order: id, product: mug, quantity: Int.Positive 1 }
  let _ <- Test.login { user: Just me }
  let page <- Test.get (Orders.Show { id: id })
  Task.succeed ({ status: Test.text "#status" page.doc, total: Test.text "#total" page.doc, model: page.model.total } == { status: Just "Open", total: Just "1250", model: 1250 })
}

-- The page is loaded while the order is Open, so the browser holds the add-line form; the order
-- is then paid, and the form is posted. `openOrder` answers `Task.notFound` (see its comment for
-- why not `Task.fail`), which is a 404, and no line is written.
test "adding a line to a Paid order is refused" = {
  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
  let tea <- Db.insert Product { name: Text.NonEmpty "Tea", price: Int.NonNegative 450 }
  let id <- Db.insert Order { owner: me }
  let _ <- Test.login { user: Just me }
  let _ <- Test.get (Orders.Show { id: id })
  let _ <- Db.update Order id (\o -> { o | status: Paid })
  let r <- Test.submit (AddLine { product: tea }) { quantity: Int.Positive 2 }
  let lines <- Db.from Line |> Db.count
  Task.succeed ({ answer: r.answer, lines: lines } == { answer: PageMissing, lines: 0 })
}

test "adding a line to an Open order changes the total" = {
  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
  let tea <- Db.insert Product { name: Text.NonEmpty "Tea", price: Int.NonNegative 450 }
  let id <- Db.insert Order { owner: me }
  let _ <- Test.login { user: Just me }
  let _ <- Test.get (Orders.Show { id: id })
  let r <- Test.submit (AddLine { product: tea }) { quantity: Int.Positive 2 }
  Task.succeed ({ answer: r.answer, total: Test.text "#total" r.doc } == { answer: Redirected (Orders.Show { id: id }), total: Just "900" })
}

test "a Paid order's page has no forms or buttons" = {
  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
  let _ <- Db.insert Product { name: Text.NonEmpty "Tea", price: Int.NonNegative 450 }
  let id <- Db.insert Order { owner: me }
  let _ <- Test.login { user: Just me }
  let _ <- Test.get (Orders.Show { id: id })
  let r <- Test.click Pay
  Task.succeed ({ answer: r.answer, status: Test.text "#status" r.doc, controls: Test.has "#controls" r.doc } == { answer: Redirected (Orders.Show { id: id }), status: Just "Paid", controls: False })
}

test "a Staff user opening another user's order is not found" = {
  let owner <- Db.insert User { email: Text.Email "owner@example.com", name: Text.NonEmpty "Owner", passwordHash: "" }
  let staff <- Db.insert User { email: Text.Email "staff@example.com", name: Text.NonEmpty "Staff", passwordHash: "", role: Staff }
  let id <- Db.insert Order { owner: owner }
  let _ <- Test.login { user: Just staff }
  let r <- Test.open (Orders.Show { id: id })
  Task.succeed (r.answer == PageMissing)
}

test "a Manager opening another user's order sees it" = {
  let owner <- Db.insert User { email: Text.Email "owner@example.com", name: Text.NonEmpty "Owner", passwordHash: "" }
  let boss <- Db.insert User { email: Text.Email "boss@example.com", name: Text.NonEmpty "Boss", passwordHash: "", role: Manager }
  let id <- Db.insert Order { owner: owner }
  let _ <- Test.login { user: Just boss }
  let r <- Test.open (Orders.Show { id: id })
  Task.succeed ({ answer: r.answer, status: Test.text "#status" r.doc } == { answer: Shown, status: Just "Open" })
}

--- file: app/Products/Index.alder ---
module Products.Index

import Product
import Layout
import User

page "/products" { } requires user

type Model = { products: List Product }

-- Only a Manager has this page; for a Staff user it is a 404.
load : Params -> Session -> Query Model
load params session = {
  let viewer <- Db.get User session.user
  if User.isManager viewer then listed else Query.notFound
}

listed : Query Model
listed = {
  let products <- Db.from Product |> Db.orderBy Product.name.asc |> Db.all
  Query.succeed { products: products }
}

form NewProduct = {
  name: Text.NonEmpty,
  price: Int.NonNegative,
}

msg Add form NewProduct
msg Toggle { id: Product.ProductId }

-- The role is checked again here, inside the task that writes.
update : Msg -> Model -> Session -> Task Nav
update msg model session = {
  let viewer <- Db.get User session.user
  if User.isManager viewer then apply msg else Task.notFound
}

apply : Msg -> Task Nav
apply msg = match msg {
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
      <Input field="name" label="Name" />
      <Errors field="name" />
      <Input field="price" label="Price in cents" />
      <Errors field="price" />
      <button>Add</button>
    </form>

    <ul id="products">
      {for product in model.products ->
        <li class={if product.active then "active" else "inactive"}>
          <span class="name">{product.name}</span>
          {" "}
          <span class="price">{Text.fromInt product.price}</span>
          <button click={Toggle { id: product.id }}>{if product.active then "Deactivate" else "Activate"}</button>
        </li>}
    </ul>

    <a href={Orders.Index {}}>Orders</a>
  </Layout.Page>

test "a Staff user does not get the products page" = {
  let staff <- Db.insert User { email: Text.Email "staff@example.com", name: Text.NonEmpty "Staff", passwordHash: "" }
  let _ <- Test.login { user: Just staff }
  let r <- Test.open (Products.Index {})
  Task.succeed (r.answer == PageMissing)
}

test "a Manager adds a product and toggles it" = {
  let boss <- Db.insert User { email: Text.Email "boss@example.com", name: Text.NonEmpty "Boss", passwordHash: "", role: Manager }
  let _ <- Test.login { user: Just boss }
  let _ <- Test.get (Products.Index {})
  let added <- Test.submit NewProduct { name: Text.NonEmpty "Tea", price: Int.NonNegative 450 }
  let found <- Db.from Product |> Db.first
  let toggled <- match found {
    Just product -> Test.click (Toggle { id: product.id }),
    Nothing -> Task.fail "no product was added",
  }
  Task.succeed ({ answer: added.answer, names: Test.texts "#products .name" added.doc, toggled: toggled.answer, inactive: Test.count "#products li.inactive" toggled.doc } == { answer: Redirected (Products.Index {}), names: ["Tea"], toggled: Redirected (Products.Index {}), inactive: 1 })
}
