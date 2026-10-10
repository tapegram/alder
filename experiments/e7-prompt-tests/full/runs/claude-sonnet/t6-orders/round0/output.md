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
    </head>
    <body>
      <nav>
        <a href={Orders.Index {}}>Orders</a>
        <a href={Products.Index {}}>Products</a>
        <button click={App.Logout}>Log out</button>
      </nav>
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

-- isManager is True when the user exists and has the Manager role.
isManager : UserId -> Query Bool
isManager id = {
  let found <- Db.get User id
  match found {
    Just u -> Query.succeed (u.role == Manager),
    Nothing -> Query.succeed False,
  }
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

-- plainInt widens a refined Int to a plain Int, so that it can be used in arithmetic.
plainInt : Int -> Int
plainInt n = n

-- money writes an amount of cents as units with two decimals.
money : Int -> Text
money cents = Float.format { decimals: 2 } (Int.toFloat cents / 100.0)

label : Product -> Text
label p = "{Text.trim p.name} ({money (plainInt p.price)})"

nameOf : List Product -> ProductId -> Text
nameOf products pid = {
  let found = List.find (\p -> p.id == pid) products
  match found {
    Just p -> Text.trim p.name,
    Nothing -> "?",
  }
}

--- file: app/ShopOrder.alder ---
module ShopOrder

import User

type OrderId = brand Text

type Status = Open | Paid | Cancelled

table ShopOrder {
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

--- file: app/OrderLine.alder ---
module OrderLine

import User
import ShopOrder
import Product

type LineId = brand Text

table OrderLine {
  id: LineId @id,
  order: ShopOrder.OrderId @ref ShopOrder,
  product: Product.ProductId @ref Product,
  quantity: Int.Positive,
}

forOrder : ShopOrder.OrderId -> Query (List OrderLine)
forOrder oid = Db.from OrderLine |> Db.where (OrderLine.order.eq oid) |> Db.all

priceOf : List Product -> Product.ProductId -> Int
priceOf products pid = {
  let found = List.find (\p -> p.id == pid) products
  match found {
    Just p -> Product.plainInt p.price,
    Nothing -> 0,
  }
}

lineTotal : List Product -> OrderLine -> Int
lineTotal products line = Product.plainInt line.quantity * priceOf products line.product

-- total is the sum of quantity x price over the lines.
total : List Product -> List OrderLine -> Int
total products lines = lines |> List.map (lineTotal products) |> List.sum

describe : List Product -> OrderLine -> Text
describe products line =
  "{Product.nameOf products line.product} x {Text.fromInt (Product.plainInt line.quantity)}: {Product.money (lineTotal products line)}"

test "an open order totals quantity times price over its two lines" = {
  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
  let pen <- Db.insert Product { name: Text.NonEmpty "Pen", price: Int.NonNegative 150, active: True }
  let ink <- Db.insert Product { name: Text.NonEmpty "Ink", price: Int.NonNegative 400, active: True }
  let oid <- Db.insert ShopOrder { owner: me }
  let _ <- Db.insert OrderLine { order: oid, product: pen, quantity: Int.Positive 2 }
  let _ <- Db.insert OrderLine { order: oid, product: ink, quantity: Int.Positive 1 }
  let lines <- forOrder oid
  let products <- Db.from Product |> Db.all
  Task.succeed (total products lines == 700)
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

--- file: app/Orders/Index.alder ---
module Orders.Index

import ShopOrder
import OrderLine
import Product
import Layout

page "/" { } requires user

type Row = { order: ShopOrder, total: Int }

type Model = { rows: List Row }

rowOf : List Product -> List OrderLine -> ShopOrder -> Row
rowOf products lines order = {
  order: order,
  total: OrderLine.total products (List.filter (\l -> l.order == order.id) lines),
}

load : Params -> Session -> Query Model
load params session = {
  let orders <-
    Db.from ShopOrder
      |> Db.where (ShopOrder.owner.eq session.user)
      |> Db.orderBy ShopOrder.created.desc
      |> Db.all
  let lines <-
    Db.from OrderLine
      |> Db.where (OrderLine.order.in (List.map (\o -> o.id) orders))
      |> Db.all
  let products <- Db.from Product |> Db.all
  Query.succeed { rows: List.map (rowOf products lines) orders }
}

msg Create

update : Msg -> Model -> Session -> Task Nav
update msg model session = match msg {
  Create p -> {
    let id <- Db.insert ShopOrder { owner: session.user }
    Task.succeed (Redirect (Orders.Show { id: id }))
  },
}

view : Model -> Session -> Html Msg
view model session =
  <Layout.Page title="Orders">
    <h1>Your orders</h1>

    <form submit={Create}>
      <button>New order</button>
    </form>

    <ul id="orders">
      {for row in model.rows -> <Item row={row} />}
    </ul>
  </Layout.Page>

Item : { row: Row } -> Html Msg
Item props =
  <li>
    <a href={Orders.Show { id: props.row.order.id }}>{Time.format "Jan 2 15:04" props.row.order.created}</a>
    <span class="status">{ShopOrder.statusLabel props.row.order.status} - {Product.money props.row.total}</span>
  </li>

--- file: app/Orders/Show.alder ---
module Orders.Show

import ShopOrder
import OrderLine
import Product
import User
import Layout

page "/orders/{id}" { id: ShopOrder.OrderId } requires user

type Model = {
  order: ShopOrder,
  lines: List OrderLine,
  products: List Product,
}

load : Params -> Session -> Query Model
load params session = {
  let found <- Db.get ShopOrder params.id
  match found {
    Just order -> visible order session,
    Nothing -> Query.notFound,
  }
}

-- visible: the owner and any Manager may see an order; for everyone else it is a 404.
visible : ShopOrder -> Session -> Query Model
visible order session = {
  let manager <- User.isManager session.user
  if order.owner == session.user || manager then fetch order else Query.notFound
}

fetch : ShopOrder -> Query Model
fetch order = {
  let lines <- OrderLine.forOrder order.id
  let products <- Db.from Product |> Db.all
  Query.succeed { order: order, lines: lines, products: products }
}

form NewLine = {
  quantity: Int.Positive,
}

-- The messages name no order: the page's {id} is the order, and `load` authorized it.
-- AddLine names a product, which comes from the request, so `update` checks it again.
msg AddLine { product: Product.ProductId } form NewLine
msg Pay
msg Cancel

update : Msg -> Model -> Session -> Task Nav
update msg model session = match msg {
  AddLine p -> {
    let order <- openOrder model.order.id
    let prod <- activeProduct p.product
    let _ <- Db.insert OrderLine { order: order.id, product: prod.id, quantity: p.form.quantity }
    Task.succeed Rerender
  },
  Pay p -> {
    let order <- openOrder model.order.id
    let _ <- Db.update ShopOrder order.id (\o -> { o | status: Paid })
    Task.succeed Rerender
  },
  Cancel p -> {
    let order <- openOrder model.order.id
    let _ <- Db.update ShopOrder order.id (\o -> { o | status: Cancelled })
    Task.succeed Rerender
  },
}

-- openOrder is the stored order when it is still Open. A change to an order that is Paid or
-- Cancelled is refused with Task.fail, not Task.notFound: the order exists and the user may see
-- it, so nothing is "not there"; the request is well-formed but invalid in the order's current
-- state (only a stale page or a forged post can send it, as the view offers no such action).
-- Task.fail rolls everything back and answers with App.error. A missing row is a 404.
openOrder : ShopOrder.OrderId -> Task ShopOrder
openOrder id = {
  let found <- Db.get ShopOrder id
  match found {
    Just order -> if order.status == Open then Task.succeed order else Task.fail "order is not open",
    Nothing -> Task.notFound,
  }
}

activeProduct : Product.ProductId -> Task Product
activeProduct pid = {
  let found <- Db.get Product pid
  match found {
    Just prod -> if prod.active then Task.succeed prod else Task.fail "product is not active",
    Nothing -> Task.notFound,
  }
}

view : Model -> Session -> Html Msg
view model session =
  <Layout.Page title="Order">
    <h1>Order</h1>
    <p>Status: {ShopOrder.statusLabel model.order.status}</p>

    <ul id="lines">
      {for line in model.lines -> <LineItem line={line} products={model.products} />}
    </ul>

    <p id="total">Total: {Product.money (OrderLine.total model.products model.lines)}</p>

    {if model.order.status == Open then <Actions products={model.products} /> else Html.none}

    <a href={Orders.Index {}}>Back</a>
  </Layout.Page>

LineItem : { line: OrderLine, products: List Product } -> Html Msg
LineItem props =
  <li>{OrderLine.describe props.products props.line}</li>

Actions : { products: List Product } -> Html Msg
Actions props =
  <div id="actions">
    <h2>Add a line</h2>
    {for prod in List.filter (\p -> p.active) props.products ->
      <form submit={AddLine { product: prod.id }}>
        <Errors form />
        <Input field="quantity" label={Product.label prod} />
        <Errors field="quantity" />
        <button>Add</button>
      </form>}
    <button click={Pay}>Mark paid</button>
    <button click={Cancel}>Mark cancelled</button>
  </div>

test "adding a line to a paid order is refused" = {
  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
  let pid <- Db.insert Product { name: Text.NonEmpty "Pen", price: Int.NonNegative 150, active: True }
  let oid <- Db.insert ShopOrder { owner: me, status: Paid }
  let model : Model = { order: { id: oid, owner: me, status: Paid, created: Time.epoch }, lines: [], products: [] }
  let r <- Test.run (update (AddLine { product: pid, form: { quantity: Int.Positive 1 } }) model { user: me })
  let lines <- OrderLine.forOrder oid
  Task.succeed ({ value: r.value, lines: List.length lines } == { value: Err "order is not open", lines: 0 })
}

test "a staff user opening another user's order is not found" = {
  let owner <- Db.insert User { email: Text.Email "owner@example.com", name: Text.NonEmpty "Owner", passwordHash: "" }
  let other <- Db.insert User { email: Text.Email "other@example.com", name: Text.NonEmpty "Other", passwordHash: "" }
  let oid <- Db.insert ShopOrder { owner: owner }
  let r <- Test.run {
    let m <- load { id: oid } { user: other }
    Task.succeed m
  }
  Task.succeed (r.value == Err "not found")
}

test "a manager opening another user's order sees it" = {
  let owner <- Db.insert User { email: Text.Email "owner@example.com", name: Text.NonEmpty "Owner", passwordHash: "" }
  let boss <- Db.insert User { email: Text.Email "boss@example.com", name: Text.NonEmpty "Boss", passwordHash: "", role: Manager }
  let oid <- Db.insert ShopOrder { owner: owner }
  let r <- Test.run {
    let m <- load { id: oid } { user: boss }
    Task.succeed m
  }
  let seen = Maybe.map (\m -> m.order.id) (Result.toMaybe r.value)
  Task.succeed (seen == Just oid)
}

--- file: app/Products/Index.alder ---
module Products.Index

import Product
import User
import Layout

page "/products" { } requires user

type Model = { products: List Product }

load : Params -> Session -> Query Model
load params session = {
  let manager <- User.isManager session.user
  if manager then listProducts else Query.notFound
}

listProducts : Query Model
listProducts = {
  let products <- Db.from Product |> Db.orderBy Product.name.asc |> Db.all
  Query.succeed { products: products }
}

form NewProduct = {
  name: Text.NonEmpty,
  price: Int.NonNegative,
}

msg Add form NewProduct
msg Toggle { id: Product.ProductId }

-- Messages come from the request, so `update` checks the role again.
requireManager : Session -> Task Unit
requireManager session = {
  let manager <- User.isManager session.user
  if manager then Task.succeed () else Task.notFound
}

update : Msg -> Model -> Session -> Task Nav
update msg model session = match msg {
  Add p -> {
    let _ <- requireManager session
    let _ <- Db.insert Product { name: p.form.name, price: p.form.price, active: True }
    Task.succeed (Redirect (Products.Index {}))
  },
  Toggle p -> {
    let _ <- requireManager session
    let found <- Db.get Product p.id
    match found {
      Just prod -> {
        let _ <- Db.update Product prod.id (\x -> { x | active: not x.active })
        Task.succeed Rerender
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
      <button>Add product</button>
    </form>

    <ul id="products">
      {for prod in model.products -> <Item product={prod} />}
    </ul>
  </Layout.Page>

Item : { product: Product } -> Html Msg
Item props =
  <li class={if props.product.active then "active" else "inactive"}>
    <span>{Product.label props.product}</span>
    <button click={Toggle { id: props.product.id }}>{if props.product.active then "Deactivate" else "Activate"}</button>
  </li>

test "a staff user opening the products page is not found" = {
  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
  let r <- Test.run {
    let m <- load { } { user: me }
    Task.succeed m
  }
  Task.succeed (r.value == Err "not found")
}

--- file: adapters/password.go ---
package adapters

import (
	"context"

	"alder/rt"
	"app"
)

// Caps are the implementations of the app's capabilities.
func Caps() app.Caps {
	return app.Caps{Auth__Login__Password: password{}}
}

// password is a placeholder of Auth.Login.Password that accepts nobody. Replace it with a real
// check of the User table's passwordHash before deploying the app.
type password struct{}

func (password) Verify(ctx context.Context, args app.Args__Auth__Login__Password__Verify) (rt.Maybe[string], error) {
	return rt.Nothing[string](), nil
}
