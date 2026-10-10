--- file: app/App.alder ---
module App

import User
import Layout
import Theme

session { user: Maybe User.UserId, theme: Theme = Light }

login = Auth.Login

msg Logout
msg SwitchTheme

update : Msg -> Session -> Task Nav
update msg session = match msg {
  Logout p -> {
    let _ <- Session.put (\s -> { s | user: Nothing })
    Task.succeed (Redirect (Auth.Login {}))
  },
  SwitchTheme p -> {
    let _ <- Session.put (\s -> { s | theme: Theme.flip s.theme })
    Task.succeed Rerender
  },
}

error : Error -> Html Never
error err =
  <Layout.Page title="Error" theme={Light}>
    <h1>Something went wrong</h1>
    <p>Please try again.</p>
  </Layout.Page>

notFound : Html Never
notFound =
  <Layout.Page title="Not found" theme={Light}>
    <h1>Not found</h1>
  </Layout.Page>

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
  <Layout.Page title="Log in" theme={session.theme}>
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

--- file: app/Layout.alder ---
module Layout

import Theme

Page : { title: Text, theme: Theme, children: List (Html a) } -> Html a
Page props =
  <html class={if props.theme == Dark then "dark" else ""}>
    <head>
      <meta charset="utf-8" />
      <title>{props.title}</title>
      <link rel="stylesheet" href={Url.external "/static/app.css"} />
    </head>
    <body>
      <nav>
        <a href={Orders.Index {}}>Orders</a>
        <a href={Products {}}>Products</a>
        <button click={App.SwitchTheme}>{if props.theme == Dark then "Light mode" else "Dark mode"}</button>
        <button click={App.Logout}>Log out</button>
      </nav>
      <main>{props.children}</main>
    </body>
  </html>

--- file: app/Theme.alder ---
module Theme

type Theme = Light | Dark

flip : Theme -> Theme
flip theme = match theme {
  Light -> Dark,
  Dark -> Light,
}

test "flip alternates" = flip (flip Light) == Light

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

--- file: app/Product.alder ---
module Product

type ProductId = brand Text

table Product {
  id: ProductId @id,
  name: Text.NonEmpty,
  price: Int.NonNegative,
  active: Bool = True,
  created: Time @created,
}

-- cents writes an amount in cents as pounds and pence: 1205 is "12.05".
cents : Int -> Text
cents amount = {
  let pence = amount % 100
  let pad = if pence < 10 then "0" else ""
  "{Text.fromInt (amount / 100)}.{pad}{Text.fromInt pence}"
}

test "cents pads the pence" = cents 1205 == "12.05" && cents 1250 == "12.50"

--- file: app/Sale.alder ---
module Sale

import User

type SaleId = brand Text

type Status = Open | Paid | Cancelled

table Sale {
  id: SaleId @id,
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

--- file: app/SaleLine.alder ---
module SaleLine

import Sale
import Product

type LineId = brand Text

table SaleLine {
  id: LineId @id,
  sale: Sale.SaleId @ref Sale,
  product: Product.ProductId @ref Product,
  quantity: Int.Positive,
}

-- total is what an order's lines add up to, in cents: each line's quantity times its product's price.
total : List SaleLine -> List Product -> Int
total lines products = lines |> List.map (lineCents products) |> List.sum

-- lineCents is one line's amount in cents. A line whose product is not in `products` adds nothing.
lineCents : List Product -> SaleLine -> Int
lineCents products line = {
  let found = List.find (\p -> p.id == line.product) products
  match found {
    Just p -> lineAmount line.quantity p.price,
    Nothing -> 0,
  }
}

lineAmount : Int -> Int -> Int
lineAmount quantity price = quantity * price

test "an open order totals quantity times price over its lines" =
  total [
    { id: LineId "a", sale: Sale.SaleId "s", product: Product.ProductId "tea", quantity: Int.Positive 2 },
    { id: LineId "b", sale: Sale.SaleId "s", product: Product.ProductId "cup", quantity: Int.Positive 1 },
  ] [
    { id: Product.ProductId "tea", name: Text.NonEmpty "Tea", price: Int.NonNegative 300, active: True, created: Time.epoch },
    { id: Product.ProductId "cup", name: Text.NonEmpty "Cup", price: Int.NonNegative 150, active: True, created: Time.epoch },
  ] == 750

--- file: app/Orders/Index.alder ---
module Orders.Index

import Sale
import SaleLine
import Product
import Layout

page "/" { } requires user

type Model = { rows: List Row }

type Row = { sale: Sale, total: Int }

load : Params -> Session -> Query Model
load params session = {
  let sales <-
    Db.from Sale
      |> Db.where (Sale.owner.eq session.user)
      |> Db.orderBy Sale.created.desc
      |> Db.all
  let products <- Db.from Product |> Db.all
  let rows <- Query.all (List.map (rowFor products) sales)
  Query.succeed { rows: rows }
}

rowFor : List Product -> Sale -> Query Row
rowFor products sale = {
  let lines <- Db.from SaleLine |> Db.where (SaleLine.sale.eq sale.id) |> Db.all
  Query.succeed { sale: sale, total: SaleLine.total lines products }
}

msg Create

update : Msg -> Model -> Session -> Task Nav
update msg model session = match msg {
  Create p -> {
    let id <- Db.insert Sale { owner: session.user }
    Task.succeed (Redirect (Orders.Show { id: id }))
  },
}

view : Model -> Session -> Html Msg
view model session =
  <Layout.Page title="Orders" theme={session.theme}>
    <h1>Orders</h1>
    <form submit={Create}>
      <button>New order</button>
    </form>
    <ul id="orders">
      {for row in model.rows -> <OrderRow row={row} />}
    </ul>
  </Layout.Page>

OrderRow : { row: Row } -> Html a
OrderRow props =
  <li>
    <a href={Orders.Show { id: props.row.sale.id }}>{Sale.statusLabel props.row.sale.status}, {Product.cents props.row.total}</a>
  </li>

--- file: app/Orders/Show.alder ---
module Orders.Show

import Sale
import SaleLine
import Product
import User
import Theme
import Layout

page "/orders/{id}" { id: Sale.SaleId } requires user

type Model = {
  sale: Sale,
  lines: List SaleLine,
  products: List Product,
}

form AddQuantity = {
  quantity: Int.Positive,
}

msg AddLine { product: Product.ProductId } form AddQuantity
msg Pay
msg Cancel

-- load shows an order to its owner and to a manager. Anyone else gets a 404.
load : Params -> Session -> Query Model
load params session = {
  let found <- Db.get Sale params.id
  let me <- Db.get User session.user
  match found {
    Just sale -> if canSee me session sale then pageFor sale else Query.notFound,
    Nothing -> Query.notFound,
  }
}

pageFor : Sale -> Query Model
pageFor sale = {
  let lines <- Db.from SaleLine |> Db.where (SaleLine.sale.eq sale.id) |> Db.all
  let products <- Db.from Product |> Db.orderBy Product.name.asc |> Db.all
  Query.succeed { sale: sale, lines: lines, products: products }
}

-- canSee is true when the order is the session user's, or the session user is a manager.
canSee : Maybe User -> Session -> Sale -> Bool
canSee me session sale = match me {
  Just u -> sale.owner == session.user || u.role == Manager,
  Nothing -> False,
}

-- update checks the order again: `load` checked it for the page, but a message can be posted
-- by anyone with the page's id. A line can be added, and the order paid or cancelled, only
-- while it is still Open in the database.
update : Msg -> Model -> Session -> Task Nav
update msg model session = match msg {
  AddLine p -> {
    let sale <- editable model.sale session
    let _ <- activeProduct p.product
    let _ <- Db.insert SaleLine { sale: sale.id, product: p.product, quantity: p.form.quantity }
    Task.succeed Rerender
  },
  Pay p -> {
    let sale <- editable model.sale session
    let _ <- Db.update Sale sale.id (\s -> { s | status: Paid })
    Task.succeed Rerender
  },
  Cancel p -> {
    let sale <- editable model.sale session
    let _ <- Db.update Sale sale.id (\s -> { s | status: Cancelled })
    Task.succeed Rerender
  },
}

-- editable is the order as it is now in the database, if the session may change it and it is
-- still Open. Otherwise the task stops: 404 for an order the user may not see, and a failure
-- for an order that is no longer open.
editable : Sale -> Session -> Task Sale
editable sale session = {
  let found <- Db.get Sale sale.id
  let me <- Db.get User session.user
  match found {
    Just now -> if canSee me session now then openOrFail now else Task.notFound,
    Nothing -> Task.notFound,
  }
}

-- An order that is not Open is refused with Task.fail, not Task.notFound: the order exists and
-- the user may see it, so "not found" would be a lie. A Task.fail rolls back the whole task
-- (nothing is inserted or updated) and the page answers 500. The page only offers changes on an
-- Open order, so this is reached by a stale page or a message sent by hand.
openOrFail : Sale -> Task Sale
openOrFail sale =
  if sale.status == Open then Task.succeed sale else Task.fail "the order is not open"

activeProduct : Product.ProductId -> Task Product
activeProduct productId = {
  let found <- Db.get Product productId
  match found {
    Just p -> if p.active then Task.succeed p else Task.fail "that product is not for sale",
    Nothing -> Task.notFound,
  }
}

view : Model -> Session -> Html Msg
view model session =
  <Layout.Page title="Order" theme={session.theme}>
    <h1>Order</h1>
    <p>Status: {Sale.statusLabel model.sale.status}</p>
    <ul id="lines">
      {for line in model.lines -> <LineRow line={line} products={model.products} />}
    </ul>
    <p id="total">Total: {Product.cents (SaleLine.total model.lines model.products)}</p>
    {match model.sale.status {
      Open -> <OpenOrder products={model.products} />,
      Paid -> <p>This order is paid.</p>,
      Cancelled -> <p>This order was cancelled.</p>,
    }}
    <a href={Orders.Index {}}>All orders</a>
    <button click={App.Logout}>Log out</button>
  </Layout.Page>

LineRow : { line: SaleLine, products: List Product } -> Html a
LineRow props =
  <li>{nameOf props.products props.line.product} x {Text.fromInt props.line.quantity} = {Product.cents (SaleLine.lineCents props.products props.line)}</li>

nameOf : List Product -> Product.ProductId -> Text
nameOf products productId = {
  let found = List.find (\p -> p.id == productId) products
  match found {
    Just p -> p.name,
    Nothing -> "unknown product",
  }
}

OpenOrder : { products: List Product } -> Html Msg
OpenOrder props =
  <div>
    {for p in List.filter (\p -> p.active) props.products -> <ProductForm product={p} />}
    <div>
      <button click={Pay}>Mark paid</button>
      <button click={Cancel}>Cancel order</button>
    </div>
  </div>

ProductForm : { product: Product } -> Html Msg
ProductForm props =
  <form submit={AddLine { product: props.product.id }}>
    <Errors form />
    <Input field="quantity" label="Quantity" />
    <Errors field="quantity" />
    <button>Add {props.product.name}</button>
  </form>

saleIdOf : Model -> Sale.SaleId
saleIdOf model = model.sale.id

test "adding a line to a paid order is refused" = {
  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "", role: Staff }
  let sid <- Db.insert Sale { owner: me, status: Paid }
  let pid <- Db.insert Product { name: Text.NonEmpty "Tea", price: Int.NonNegative 300 }
  let model : Model = { sale: { id: sid, owner: me, status: Paid, created: Time.epoch }, lines: [], products: [] }
  let r <- Test.run (update (AddLine { product: pid, form: { quantity: Int.Positive 1 } }) model { user: me, theme: Light })
  let lines <- Db.from SaleLine |> Db.where (SaleLine.sale.eq sid) |> Db.count
  Task.succeed ({ value: r.value, lines: lines } == { value: Err "the order is not open", lines: 0 })
}

test "a staff user opening another user's order is not found" = {
  let owner <- Db.insert User { email: Text.Email "owner@example.com", name: Text.NonEmpty "Owner", passwordHash: "" }
  let staff <- Db.insert User { email: Text.Email "staff@example.com", name: Text.NonEmpty "Staff", passwordHash: "", role: Staff }
  let sid <- Db.insert Sale { owner: owner }
  let r <- Test.run {
    let m <- load { id: sid } { user: staff, theme: Light }
    Task.succeed m
  }
  Task.succeed (Result.map saleIdOf r.value == Err "not found")
}

test "a manager opening another user's order is found" = {
  let owner <- Db.insert User { email: Text.Email "owner2@example.com", name: Text.NonEmpty "Owner", passwordHash: "" }
  let boss <- Db.insert User { email: Text.Email "boss@example.com", name: Text.NonEmpty "Boss", passwordHash: "", role: Manager }
  let sid <- Db.insert Sale { owner: owner }
  let r <- Test.run {
    let m <- load { id: sid } { user: boss, theme: Light }
    Task.succeed m
  }
  Task.succeed (Result.map saleIdOf r.value == Ok sid)
}

--- file: app/Products.alder ---
module Products

import Product
import User
import Layout

page "/products" { } requires user

type Model = { products: List Product }

-- Only a manager sees the list; anyone else gets a 404.
load : Params -> Session -> Query Model
load params session = {
  let found <- Db.get User session.user
  match found {
    Just me -> if me.role == Manager then listAll else Query.notFound,
    Nothing -> Query.notFound,
  }
}

listAll : Query Model
listAll = {
  let products <- Db.from Product |> Db.orderBy Product.name.asc |> Db.all
  Query.succeed { products: products }
}

form NewProduct = {
  name: Text.NonEmpty,
  price: Int.NonNegative,
}

msg Add form NewProduct
msg Toggle { id: Product.ProductId }

-- managerOnly stops the task unless the session's user is a manager now. `load` checked it for
-- the page; a message can be posted by anyone, so `update` checks it again.
managerOnly : Session -> Task Unit
managerOnly session = {
  let found <- Db.get User session.user
  match found {
    Just me -> if me.role == Manager then Task.succeed () else Task.notFound,
    Nothing -> Task.notFound,
  }
}

update : Msg -> Model -> Session -> Task Nav
update msg model session = match msg {
  Add p -> {
    let _ <- managerOnly session
    let _ <- Db.insert Product { name: p.form.name, price: p.form.price }
    Task.succeed Rerender
  },
  Toggle p -> {
    let _ <- managerOnly session
    let _ <- Db.update Product p.id (\pr -> { pr | active: not pr.active })
    Task.succeed Rerender
  },
}

view : Model -> Session -> Html Msg
view model session =
  <Layout.Page title="Products" theme={session.theme}>
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
      {for p in model.products -> <ProductRow product={p} />}
    </ul>
    <a href={Orders.Index {}}>Orders</a>
  </Layout.Page>

ProductRow : { product: Product } -> Html Msg
ProductRow props =
  <li class={if props.product.active then "" else "inactive"}>
    {props.product.name} {Product.cents props.product.price}
    <button click={Toggle { id: props.product.id }}>{if props.product.active then "Deactivate" else "Activate"}</button>
  </li>
