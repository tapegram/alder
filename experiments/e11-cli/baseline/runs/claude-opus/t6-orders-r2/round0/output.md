--- file: app/User.alder ---
module User

type UserId = brand Text

-- Staff keep their own orders. A Manager can also open anyone's order and manage the products.
-- The session holds only the user. Each page that needs the role reads it from the user's row,
-- so a change of role applies at once.
type Role = Staff | Manager

table User {
  id: UserId @id,
  email: Text.Email @unique,
  name: Text.NonEmpty,
  passwordHash: Text,
  role: Role = Staff,
  created: Time @created,
}

isManager : UserId -> Query Bool
isManager id = {
  let me <- Db.get User id
  Query.succeed (Maybe.map (\u -> u.role) me == Just Manager)
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

test "logging out through the page" = {
  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
  let _ <- Test.login { user: Just me }
  let _ <- Test.get (Orders.Index {})
  let out <- Test.click Logout
  Task.succeed ({ out: out.answer, user: out.session.user } == { out: Redirected (Auth.Login {}), user: Nothing })
}

--- file: app/Layout.alder ---
module Layout

Page : { title: Text, children: List (Html a) } -> Html a
Page props =
  <html>
    <head>
      <meta charset="utf-8" />
      <title>{props.title}</title>
      <link rel="stylesheet" href={Url.external "/static/al/al.css"} />
    </head>
    <body>
      <al-page width="narrow">
        <main>{props.children}</main>
      </al-page>
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

priceOf : List Product -> Product.ProductId -> Int
priceOf products id = {
  let found = List.find (\p -> p.id == id) products
  match found {
    Just p -> p.price,
    Nothing -> 0,
  }
}

nameOf : List Product -> Product.ProductId -> Text
nameOf products id = {
  let found = List.find (\p -> p.id == id) products
  match found {
    Just p -> p.name,
    Nothing -> "",
  }
}

lineTotal : List Product -> Line -> Int
lineTotal products l = l.quantity * priceOf products l.product

orderTotal : List Product -> List Line -> Int
orderTotal products lines = List.sum (List.map (lineTotal products) lines)

test "a total is the sum of quantity times price" =
  orderTotal [
    { id: Product.ProductId "a", name: Text.NonEmpty "Tea", price: Int.NonNegative 250, active: True },
    { id: Product.ProductId "b", name: Text.NonEmpty "Cake", price: Int.NonNegative 100, active: True },
  ] [
    { id: LineId "l1", order: Order.OrderId "o", product: Product.ProductId "a", quantity: Int.Positive 2 },
    { id: LineId "l2", order: Order.OrderId "o", product: Product.ProductId "b", quantity: Int.Positive 3 },
  ] == 800

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

test "an unknown email is rejected" = {
  let _ <- Test.fake Password
  let _ <- Test.get (Auth.Login {})
  let r <- Test.submit Credentials { email: Text.Email "nobody@example.com", password: Text.Secret "secret" }
  Task.succeed ({ answer: r.answer, user: r.session.user } == { answer: FormRejected [{ field: "", message: "Email or password is incorrect." }], user: Nothing })
}

test "logging in through the form" = {
  let _ <- Test.fake Password
  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
  let _ <- Test.get (Auth.Login {})
  let r <- Test.submit Credentials { email: Text.Email "me@example.com", password: Text.Secret "secret" }
  Task.succeed ({ answer: r.answer, user: r.session.user, title: Test.text "h1" r.doc } == { answer: Redirected (Orders.Index {}), user: Just me, title: Just "Orders" })
}

test "a visitor is sent to log in" = {
  let visitor <- Test.open (Orders.Index {})
  Task.succeed (visitor.answer == Redirected (Auth.Login {}))
}

--- file: app/Orders/Index.alder ---
module Orders.Index

import Order
import Line
import Product
import User
import Layout

page "/" { } requires user

type Row = {
  id: Order.OrderId,
  status: Order.Status,
  created: Time,
  total: Int,
}

type Model = { orders: List Row }

toRow : List Product -> List Line -> Order -> Row
toRow products lines o = {
  let mine = List.filter (\l -> l.order == o.id) lines
  { id: o.id, status: o.status, created: o.created, total: Line.orderTotal products mine }
}

load : Params -> Session -> Query Model
load params session = {
  let orders <-
    Db.from Order
      |> Db.where (Order.owner.eq session.user)
      |> Db.orderBy Order.created.desc
      |> Db.all
  let lines <- Db.from Line |> Db.where (Line.order.in (List.map (\o -> o.id) orders)) |> Db.all
  let products <- Db.from Product |> Db.all
  Query.succeed { orders: List.map (toRow products lines) orders }
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
      {for o in model.orders ->
        <li>
          <a href={Orders.Show { id: o.id }}>{Time.format "2006-01-02 15:04" o.created}</a>
          {" "}
          <span class="status">{Order.statusLabel o.status}</span>
          {" "}
          <span class="total">{Text.fromInt o.total}</span>
        </li>}
    </ul>
    <p><a href={Products.Index {}}>Products</a></p>
    <button click={App.Logout}>Log out</button>
  </Layout.Page>

test "creating an order opens it" = {
  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
  let _ <- Test.login { user: Just me }
  let _ <- Test.get (Orders.Index {})
  let r <- Test.click Create
  let orders <- Db.from Order |> Db.all
  let statuses = List.map (\o -> o.status) orders
  let opened = match orders {
    [o] -> r.answer == Redirected (Orders.Show { id: o.id }),
    _ -> False,
  }
  Task.succeed ({ opened: opened, statuses: statuses, status: Test.text "#status" r.doc } == { opened: True, statuses: [Open], status: Just "Open" })
}

test "my orders are listed newest first with status and total" = {
  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
  let other <- Db.insert User { email: Text.Email "other@example.com", name: Text.NonEmpty "Other", passwordHash: "" }
  let tea <- Db.insert Product { name: Text.NonEmpty "Tea", price: Int.NonNegative 250 }
  let first <- Db.insert Order { owner: me }
  let _ <- Db.insert Line { order: first, product: tea, quantity: Int.Positive 2 }
  let _ <- Db.insert Order { owner: me, status: Paid }
  let _ <- Db.insert Order { owner: other }
  let _ <- Test.login { user: Just me }
  let page <- Test.get (Orders.Index {})
  Task.succeed ({ statuses: Test.texts "#orders .status" page.doc, totals: Test.texts "#orders .total" page.doc } == { statuses: ["Paid", "Open"], totals: ["0", "500"] })
}

--- file: app/Orders/Show.alder ---
module Orders.Show

import Order
import Line
import Product
import User
import Layout

page "/orders/{id}" { id: Order.OrderId } requires user

type LineRow = {
  name: Text,
  quantity: Int,
  subtotal: Int,
}

type Model = {
  order: Order,
  lines: List LineRow,
  products: List Product,
  total: Int,
}

toLineRow : List Product -> Line -> LineRow
toLineRow products l = {
  let q : Int = l.quantity
  { name: Line.nameOf products l.product, quantity: q, subtotal: Line.lineTotal products l }
}

loadOrder : Order -> Query Model
loadOrder order = {
  let lines <- Db.from Line |> Db.where (Line.order.eq order.id) |> Db.all
  let products <- Db.from Product |> Db.orderBy Product.name.asc |> Db.all
  Query.succeed { order: order, lines: List.map (toLineRow products) lines, products: List.filter (\p -> p.active) products, total: Line.orderTotal products lines }
}

-- An order is the owner's, or a Manager's to see. Anyone else gets the same 404 as a missing
-- order. Every message runs this `load` first, so the messages are guarded too.
load : Params -> Session -> Query Model
load params session = {
  let found <- Db.get Order params.id
  let manager <- User.isManager session.user
  match found {
    Just order -> if order.owner == session.user || manager then loadOrder order else Query.notFound,
    Nothing -> Query.notFound,
  }
}

form NewLine = { quantity: Int.Positive }

msg AddLine { product: Product.ProductId } form NewLine
msg Pay
msg Cancel

-- The order's status is read again by `load` before each message, so a form from a page loaded
-- while the order was Open is refused once it no longer is.
update : Msg -> Model -> Session -> Task Nav
update msg model session = match msg {
  AddLine p -> {
    let found <- Db.get Product p.product
    let active = Maybe.map (\x -> x.active) found == Just True
    if model.order.status != Open then Task.fail "This order is not open."
    else if active then {
      let _ <- Db.insert Line { order: model.order.id, product: p.product, quantity: p.form.quantity }
      Task.succeed (Redirect (Orders.Show { id: model.order.id }))
    } else Task.notFound
  },
  Pay p ->
    if model.order.status == Open then {
      let _ <- Db.update Order model.order.id (\o -> { o | status: Paid })
      Task.succeed (Redirect (Orders.Show { id: model.order.id }))
    } else Task.fail "This order is not open.",
  Cancel p ->
    if model.order.status == Open then {
      let _ <- Db.update Order model.order.id (\o -> { o | status: Cancelled })
      Task.succeed (Redirect (Orders.Show { id: model.order.id }))
    } else Task.fail "This order is not open.",
}

view : Model -> Session -> Html Msg
view model session =
  <Layout.Page title="Order">
    <h1>Order</h1>
    <p>Status: <span id="status">{Order.statusLabel model.order.status}</span></p>
    <ul id="lines">
      {for l in model.lines ->
        <li>
          <span class="name">{l.name}</span>
          {" x "}
          <span class="quantity">{Text.fromInt l.quantity}</span>
          {" = "}
          <span class="subtotal">{Text.fromInt l.subtotal}</span>
        </li>}
    </ul>
    <p>Total: <span id="total">{Text.fromInt model.total}</span></p>
    {if model.order.status == Open then
      <div id="actions">
        {for p in model.products ->
          <form submit={AddLine { product: p.id }}>
            <span class="product">{p.name}</span>
            <Input field="quantity" label="Quantity" />
            <Errors field="quantity" />
            <button>Add</button>
          </form>}
        <button click={Pay}>Mark paid</button>
        <button click={Cancel}>Cancel order</button>
      </div>
    else Html.none}
    <p><a href={Orders.Index {}}>Back</a></p>
  </Layout.Page>

test "an Open order's total over two lines" = {
  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
  let tea <- Db.insert Product { name: Text.NonEmpty "Tea", price: Int.NonNegative 250 }
  let cake <- Db.insert Product { name: Text.NonEmpty "Cake", price: Int.NonNegative 100 }
  let o <- Db.insert Order { owner: me }
  let _ <- Db.insert Line { order: o, product: tea, quantity: Int.Positive 2 }
  let _ <- Db.insert Line { order: o, product: cake, quantity: Int.Positive 3 }
  let _ <- Test.login { user: Just me }
  let page <- Test.get (Orders.Show { id: o })
  Task.succeed ({ model: page.model.total, total: Test.text "#total" page.doc, status: Test.text "#status" page.doc } == { model: 800, total: Just "800", status: Just "Open" })
}

test "adding a line through the page" = {
  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
  let tea <- Db.insert Product { name: Text.NonEmpty "Tea", price: Int.NonNegative 250 }
  let o <- Db.insert Order { owner: me }
  let _ <- Test.login { user: Just me }
  let _ <- Test.get (Orders.Show { id: o })
  let r <- Test.submit (AddLine { product: tea }) { quantity: Int.Positive 4 }
  Task.succeed ({ answer: r.answer, total: Test.text "#total" r.doc } == { answer: Redirected (Orders.Show { id: o }), total: Just "1000" })
}

test "adding a line to a Paid order is refused" = {
  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
  let tea <- Db.insert Product { name: Text.NonEmpty "Tea", price: Int.NonNegative 250 }
  let o <- Db.insert Order { owner: me }
  let _ <- Test.login { user: Just me }
  let _ <- Test.get (Orders.Show { id: o })
  let _ <- Db.update Order o (\x -> { x | status: Paid })
  let r <- Test.submit (AddLine { product: tea }) { quantity: Int.Positive 1 }
  let n <- Db.from Line |> Db.where (Line.order.eq o) |> Db.count
  Task.succeed ({ answer: r.answer, lines: n } == { answer: RequestFailed "This order is not open.", lines: 0 })
}

test "paying an order hides its forms and buttons" = {
  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
  let _ <- Db.insert Product { name: Text.NonEmpty "Tea", price: Int.NonNegative 250 }
  let o <- Db.insert Order { owner: me }
  let _ <- Test.login { user: Just me }
  let _ <- Test.get (Orders.Show { id: o })
  let r <- Test.click Pay
  Task.succeed ({ answer: r.answer, status: Test.text "#status" r.doc, forms: Test.count "form" r.doc, actions: Test.has "#actions" r.doc } == { answer: Redirected (Orders.Show { id: o }), status: Just "Paid", forms: 0, actions: False })
}

test "a Staff user opening another user's order is not found" = {
  let owner <- Db.insert User { email: Text.Email "owner@example.com", name: Text.NonEmpty "Owner", passwordHash: "" }
  let staff <- Db.insert User { email: Text.Email "staff@example.com", name: Text.NonEmpty "Staff", passwordHash: "" }
  let o <- Db.insert Order { owner: owner }
  let _ <- Test.login { user: Just staff }
  let r <- Test.open (Orders.Show { id: o })
  Task.succeed (r.answer == PageMissing)
}

test "a Manager opening another user's order sees it" = {
  let owner <- Db.insert User { email: Text.Email "owner@example.com", name: Text.NonEmpty "Owner", passwordHash: "" }
  let boss <- Db.insert User { email: Text.Email "boss@example.com", name: Text.NonEmpty "Boss", passwordHash: "", role: Manager }
  let o <- Db.insert Order { owner: owner }
  let _ <- Test.login { user: Just boss }
  let r <- Test.open (Orders.Show { id: o })
  Task.succeed ({ answer: r.answer, status: Test.text "#status" r.doc } == { answer: Shown, status: Just "Open" })
}

--- file: app/Products/Index.alder ---
module Products.Index

import Product
import User
import Layout

page "/products" { } requires user

type Model = { products: List Product }

-- Only a Manager may see this page: the role is read from the user's row, and anyone else gets
-- the same 404 as a page that does not exist. Every message runs this `load` first.
load : Params -> Session -> Query Model
load params session = {
  let manager <- User.isManager session.user
  if manager then {
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
    let _ <- Db.update Product p.id (\x -> { x | active: not x.active })
    Task.succeed (Redirect (Products.Index {}))
  },
}

view : Model -> Session -> Html Msg
view model session =
  <Layout.Page title="Products">
    <h1>Products</h1>
    <ul id="products">
      {for p in model.products ->
        <li>
          <span class="name">{p.name}</span>
          {" "}
          <span class="price">{Text.fromInt p.price}</span>
          {" "}
          <span class="active">{if p.active then "active" else "inactive"}</span>
          {" "}
          <button click={Toggle { id: p.id }}>{if p.active then "Deactivate" else "Activate"}</button>
        </li>}
    </ul>
    <form submit={Add}>
      <Input field="name" label="Name" />
      <Errors field="name" />
      <Input field="price" label="Price (cents)" />
      <Errors field="price" />
      <button>Add product</button>
    </form>
    <p><a href={Orders.Index {}}>Orders</a></p>
  </Layout.Page>

test "a Staff user gets 404 on products" = {
  let staff <- Db.insert User { email: Text.Email "staff@example.com", name: Text.NonEmpty "Staff", passwordHash: "" }
  let _ <- Test.login { user: Just staff }
  let r <- Test.open (Products.Index {})
  Task.succeed (r.answer == PageMissing)
}

test "a Manager adds a product and toggles it" = {
  let boss <- Db.insert User { email: Text.Email "boss@example.com", name: Text.NonEmpty "Boss", passwordHash: "", role: Manager }
  let _ <- Test.login { user: Just boss }
  let _ <- Test.get (Products.Index {})
  let added <- Test.submit NewProduct { name: Text.NonEmpty "Tea", price: Int.NonNegative 250 }
  let products <- Db.from Product |> Db.all
  let toggled <- match products {
    [p] -> Test.click (Toggle { id: p.id }),
    _ -> Test.open (Products.Index {}),
  }
  Task.succeed ({ added: added.answer, names: Test.texts "#products .name" added.doc, toggled: toggled.answer, active: Test.texts "#products .active" toggled.doc } == { added: Redirected (Products.Index {}), names: ["Tea"], toggled: Redirected (Products.Index {}), active: ["inactive"] })
}
