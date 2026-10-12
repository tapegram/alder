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

-- isManager is whether the row a session's user names is a Manager; a user without a row is not.
isManager : Maybe User -> Bool
isManager found = match found {
  Just u -> u.role == Manager,
  Nothing -> False,
}

test "only a Manager row is a manager" =
  [
    isManager Nothing,
    isManager (Just { id: UserId "a", email: Text.Email "a@example.com", name: Text.NonEmpty "A", passwordHash: "", role: Staff, created: Time.epoch }),
    isManager (Just { id: UserId "b", email: Text.Email "b@example.com", name: Text.NonEmpty "B", passwordHash: "", role: Manager, created: Time.epoch }),
  ] == [False, False, True]

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

-- label is a product's name as plain text.
label : Product -> Text
label product = product.name

-- nameOf is the name of the product with this id among products, or "" when it is not there.
nameOf : List Product -> ProductId -> Text
nameOf products id = match List.find (\p -> p.id == id) products {
  Just p -> label p,
  Nothing -> "",
}

test "nameOf finds the product's name" =
  nameOf [
    { id: ProductId "a", name: Text.NonEmpty "Tea", price: Int.NonNegative 450, active: True },
    { id: ProductId "b", name: Text.NonEmpty "Cup", price: Int.NonNegative 350, active: False },
  ] (ProductId "b") == "Cup"

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

-- lineTotal is quantity × price in cents; a line whose product is not among products counts 0.
lineTotal : List Product -> Line -> Int
lineTotal products line = {
  let amounts = products |> List.filter (\p -> p.id == line.product) |> List.map (\p -> line.quantity * p.price)
  List.sum amounts
}

-- sumLines is the total of these lines in cents.
sumLines : List Product -> List Line -> Int
sumLines products lines = List.sum (List.map (lineTotal products) lines)

-- orderTotal is the total of the lines that belong to one order.
orderTotal : List Product -> List Line -> Order.OrderId -> Int
orderTotal products lines id = sumLines products (List.filter (\l -> l.order == id) lines)

test "orderTotal sums quantity times price over the order's lines" =
  orderTotal [
    { id: Product.ProductId "tea", name: Text.NonEmpty "Tea", price: Int.NonNegative 450, active: True },
    { id: Product.ProductId "cup", name: Text.NonEmpty "Cup", price: Int.NonNegative 350, active: True },
  ] [
    { id: LineId "a", order: Order.OrderId "o", product: Product.ProductId "tea", quantity: Int.Positive 2 },
    { id: LineId "b", order: Order.OrderId "o", product: Product.ProductId "cup", quantity: Int.Positive 1 },
    { id: LineId "c", order: Order.OrderId "other", product: Product.ProductId "cup", quantity: Int.Positive 5 },
  ] (Order.OrderId "o") == 1250

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

test "a visitor is sent to log in, and can log out" = {
  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
  let visitor <- Test.open (Orders.Index {})
  let _ <- Test.login { user: Just me }
  let _ <- Test.get (Orders.Index {})
  let out <- Test.click App.Logout
  Task.succeed ({ visitor: visitor.answer, out: out.answer, user: out.session.user } == { visitor: Redirected (Auth.Login {}), out: Redirected (Auth.Login {}), user: Nothing })
}

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
  let lines <- Db.from Line |> Db.where (Line.order.in (List.map (\o -> o.id) orders)) |> Db.all
  let products <- Db.from Product |> Db.all
  let viewer <- Db.get User session.user
  Query.succeed { rows: List.map (toRow products lines) orders, manager: User.isManager viewer }
}

toRow : List Product -> List Line -> Order -> Row
toRow products lines order = { order: order, total: Line.orderTotal products lines order.id }

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
      {for row in model.rows -> <Item row={row} />}
    </ul>

    {if model.manager then <a href={Products.Index {}}>Products</a> else Html.none}

    <button click={App.Logout}>Log out</button>
  </Layout.Page>

Item : { row: Row } -> Html Msg
Item props =
  <li>
    <a href={Orders.Show { id: props.row.order.id }}>{Time.format "Jan 2, 2006 15:04" props.row.order.created}</a>
    {" "}
    <span class="status">{Order.statusLabel props.row.order.status}</span>
    {" "}
    <span class="total">{Text.fromInt props.row.total}</span>
  </li>

test "creating an order through the page" = {
  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
  let _ <- Test.login { user: Just me }
  let _ <- Test.get (Orders.Index {})
  let r <- Test.click Create
  let found <- Db.from Order |> Db.first
  let page <- Test.get (Orders.Index {})
  Task.succeed ({ made: Maybe.map (\o -> o.status) found, shown: Test.text "#status" r.doc, statuses: Test.texts "#orders .status" page.doc, totals: Test.texts "#orders .total" page.doc } == { made: Just Open, shown: Just "Open", statuses: ["Open"], totals: ["0"] })
}

test "the list shows my orders, newest first, with status and total" = {
  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
  let other <- Db.insert User { email: Text.Email "other@example.com", name: Text.NonEmpty "Other", passwordHash: "" }
  let tea <- Db.insert Product { name: Text.NonEmpty "Tea", price: Int.NonNegative 450 }
  let first <- Db.insert Order { owner: me, status: Paid }
  let second <- Db.insert Order { owner: me }
  let theirs <- Db.insert Order { owner: other }
  let _ <- Db.insert Line { order: first, product: tea, quantity: Int.Positive 2 }
  let _ <- Db.insert Line { order: theirs, product: tea, quantity: Int.Positive 7 }
  let _ <- Test.login { user: Just me }
  let page <- Test.get (Orders.Index {})
  Task.succeed ({ items: Test.count "#orders li" page.doc, statuses: Test.texts "#orders .status" page.doc, totals: Test.texts "#orders .total" page.doc } == { items: 2, statuses: ["Open", "Paid"], totals: ["0", "900"] })
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
  total: Int,
}

load : Params -> Session -> Query Model
load params session = {
  let order <- visible params.id session
  let lines <- Db.from Line |> Db.where (Line.order.eq order.id) |> Db.all
  let products <- Db.from Product |> Db.orderBy Product.name.asc |> Db.all
  Query.succeed { order: order, lines: lines, products: products, total: Line.sumLines products lines }
}

-- visible is the order when the session's user owns it or is a Manager; otherwise a 404.
visible : Order.OrderId -> Session -> Query Order
visible id session = {
  let found <- Db.get Order id
  let viewer <- Db.get User session.user
  match found {
    Just order -> if order.owner == session.user || User.isManager viewer then Query.succeed order else Query.notFound,
    Nothing -> Query.notFound,
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

closed : Text
closed = "This order is no longer open."

update : Msg -> Model -> Session -> Task Nav
update msg model session = match msg {
  AddLine p -> {
    let order <- current model
    match order.status {
      Open -> addTo order p.product p.form.quantity,
      Paid -> Task.succeed (Reject closed),
      Cancelled -> Task.succeed (Reject closed),
    }
  },
  Pay p -> setStatus model Paid,
  Cancel p -> setStatus model Cancelled,
}

-- current reads the order again inside the task, so the status checked is the stored one.
current : Model -> Task Order
current model = {
  let found <- Db.get Order model.order.id
  match found {
    Just order -> Task.succeed order,
    Nothing -> Task.notFound,
  }
}

-- addTo adds a line for an active product; a missing or inactive product is a 404.
addTo : Order -> Product.ProductId -> Int.Positive -> Task Nav
addTo order productId quantity = {
  let found <- Db.get Product productId
  match found {
    Just product -> if product.active then insertLine order.id product.id quantity else Task.notFound,
    Nothing -> Task.notFound,
  }
}

insertLine : Order.OrderId -> Product.ProductId -> Int.Positive -> Task Nav
insertLine orderId productId quantity = {
  let _ <- Db.insert Line { order: orderId, product: productId, quantity: quantity }
  Task.succeed (Redirect (Orders.Show { id: orderId }))
}

-- setStatus changes an Open order's status; an order that is not Open is left as it is.
setStatus : Model -> Order.Status -> Task Nav
setStatus model status = {
  let order <- current model
  let _ <- if order.status == Open then Db.update Order order.id (\o -> { o | status: status }) else Task.succeed ()
  Task.succeed (Redirect (Orders.Show { id: order.id }))
}

view : Model -> Session -> Html Msg
view model session =
  <Layout.Page title="Order">
    <h1>Order</h1>

    <p>Status: <span id="status">{Order.statusLabel model.order.status}</span></p>

    <ul id="lines">
      {for line in model.lines -> <LineItem line={line} products={model.products} />}
    </ul>

    <p>Total: <span id="total">{Text.fromInt model.total}</span></p>

    {if model.order.status == Open then <Controls products={List.filter (\p -> p.active) model.products} /> else Html.none}

    <a href={Orders.Index {}}>Back</a>
    <button click={App.Logout}>Log out</button>
  </Layout.Page>

LineItem : { line: Line, products: List Product } -> Html Msg
LineItem props =
  <li>
    <span class="product">{Product.nameOf props.products props.line.product}</span>
    {" x "}
    <span class="quantity">{Text.fromInt props.line.quantity}</span>
    {" = "}
    <span class="amount">{Text.fromInt (Line.lineTotal props.products props.line)}</span>
  </li>

Controls : { products: List Product } -> Html Msg
Controls props =
  <section id="controls">
    <div id="add">
      {for product in props.products -> <AddForm product={product} />}
    </div>
    <button click={Pay}>Mark paid</button>
    <button click={Cancel}>Cancel order</button>
  </section>

AddForm : { product: Product } -> Html Msg
AddForm props =
  <div class="add">
    <span class="choice">{props.product.name}</span>
    <form submit={AddLine { product: props.product.id }}>
      <Errors form />
      <Input field="quantity" label="Quantity" />
      <Errors field="quantity" />
      <button>Add line</button>
    </form>
  </div>

test "an Open order's total over two lines" = {
  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
  let tea <- Db.insert Product { name: Text.NonEmpty "Tea", price: Int.NonNegative 450 }
  let cup <- Db.insert Product { name: Text.NonEmpty "Cup", price: Int.NonNegative 350 }
  let id <- Db.insert Order { owner: me }
  let _ <- Db.insert Line { order: id, product: tea, quantity: Int.Positive 2 }
  let _ <- Db.insert Line { order: id, product: cup, quantity: Int.Positive 1 }
  let _ <- Test.login { user: Just me }
  let page <- Test.get (Orders.Show { id: id })
  Task.succeed ({ status: Test.text "#status" page.doc, total: Test.text "#total" page.doc, model: page.model.total } == { status: Just "Open", total: Just "1250", model: 1250 })
}

test "adding a line through the page" = {
  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
  let tea <- Db.insert Product { name: Text.NonEmpty "Tea", price: Int.NonNegative 450 }
  let id <- Db.insert Order { owner: me }
  let _ <- Test.login { user: Just me }
  let _ <- Test.get (Orders.Show { id: id })
  let r <- Test.submit (AddLine { product: tea }) { quantity: Int.Positive 3 }
  Task.succeed ({ answer: r.answer, total: Test.text "#total" r.doc } == { answer: Redirected (Orders.Show { id: id }), total: Just "1350" })
}

test "adding a line to a Paid order is rejected" = {
  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
  let tea <- Db.insert Product { name: Text.NonEmpty "Tea", price: Int.NonNegative 450 }
  let id <- Db.insert Order { owner: me, status: Paid }
  let _ <- Test.login { user: Just me }
  let page <- Test.get (Orders.Show { id: id })
  let r <- Test.run (update (AddLine { product: tea, form: { quantity: Int.Positive 3 } }) page.model { user: me })
  let lines <- Db.from Line |> Db.count
  Task.succeed ({ value: r.value, lines: lines, controls: Test.has "#controls" page.doc } == { value: Ok (Reject "This order is no longer open."), lines: 0, controls: False })
}

-- The form comes from a page loaded while the order was Open; the order is paid meanwhile.
test "a stale page cannot add a line to a Paid order" = {
  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
  let tea <- Db.insert Product { name: Text.NonEmpty "Tea", price: Int.NonNegative 450 }
  let id <- Db.insert Order { owner: me }
  let _ <- Test.login { user: Just me }
  let _ <- Test.get (Orders.Show { id: id })
  let _ <- Db.update Order id (\o -> { o | status: Paid })
  let r <- Test.submit (AddLine { product: tea }) { quantity: Int.Positive 3 }
  let lines <- Db.from Line |> Db.count
  Task.succeed ({ lines: lines, status: Test.text "#status" r.doc } == { lines: 0, status: Just "Paid" })
}

test "a Staff user opening another user's order is not found" = {
  let owner <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
  let other <- Db.insert User { email: Text.Email "other@example.com", name: Text.NonEmpty "Other", passwordHash: "" }
  let id <- Db.insert Order { owner: owner }
  let _ <- Test.login { user: Just other }
  let r <- Test.open (Orders.Show { id: id })
  Task.succeed (r.answer == PageMissing)
}

test "a Manager opening another user's order sees it" = {
  let owner <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
  let boss <- Db.insert User { email: Text.Email "boss@example.com", name: Text.NonEmpty "Boss", passwordHash: "", role: Manager }
  let id <- Db.insert Order { owner: owner }
  let _ <- Test.login { user: Just boss }
  let r <- Test.open (Orders.Show { id: id })
  Task.succeed ({ answer: r.answer, status: Test.text "#status" r.doc } == { answer: Shown, status: Just "Open" })
}

test "paying an order through the page" = {
  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
  let id <- Db.insert Order { owner: me }
  let _ <- Test.login { user: Just me }
  let _ <- Test.get (Orders.Show { id: id })
  let r <- Test.click Pay
  Task.succeed ({ answer: r.answer, status: Test.text "#status" r.doc, controls: Test.has "#controls" r.doc } == { answer: Redirected (Orders.Show { id: id }), status: Just "Paid", controls: False })
}

test "cancelling an order through the page" = {
  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
  let id <- Db.insert Order { owner: me }
  let _ <- Test.login { user: Just me }
  let _ <- Test.get (Orders.Show { id: id })
  let r <- Test.click Cancel
  Task.succeed ({ answer: r.answer, status: Test.text "#status" r.doc } == { answer: Redirected (Orders.Show { id: id }), status: Just "Cancelled" })
}

--- file: app/Products/Index.alder ---
module Products.Index

import Product
import Layout
import User

page "/products" { } requires user

type Model = { products: List Product }

load : Params -> Session -> Query Model
load params session = {
  let viewer <- Db.get User session.user
  let products <- Db.from Product |> Db.orderBy Product.name.asc |> Db.all
  if User.isManager viewer then Query.succeed { products: products } else Query.notFound
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
    let _ <- manager session
    let _ <- Db.insert Product { name: p.form.name, price: p.form.price }
    Task.succeed (Redirect (Products.Index {}))
  },
  Toggle p -> {
    let _ <- manager session
    let found <- Db.get Product p.id
    match found {
      Just product -> flip product,
      Nothing -> Task.notFound,
    }
  },
}

-- manager is a 404 unless the session's user is a Manager: a message can be posted without the page.
manager : Session -> Task Unit
manager session = {
  let viewer <- Db.get User session.user
  if User.isManager viewer then Task.succeed () else Task.notFound
}

flip : Product -> Task Nav
flip product = {
  let _ <- Db.update Product product.id (\x -> { x | active: not x.active })
  Task.succeed (Redirect (Products.Index {}))
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
      {for product in model.products -> <Item product={product} />}
    </ul>

    <a href={Orders.Index {}}>Orders</a>
    <button click={App.Logout}>Log out</button>
  </Layout.Page>

Item : { product: Product } -> Html Msg
Item props =
  <li class={if props.product.active then "active" else "inactive"}>
    <span class="name">{props.product.name}</span>
    {" "}
    <span class="price">{Text.fromInt props.product.price}</span>
    {" "}
    <button click={Toggle { id: props.product.id }}>{if props.product.active then "Deactivate" else "Activate"}</button>
  </li>

test "a Staff user does not find the products page" = {
  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
  let _ <- Test.login { user: Just me }
  let r <- Test.open (Products.Index {})
  Task.succeed (r.answer == PageMissing)
}

test "a Manager adds a product" = {
  let boss <- Db.insert User { email: Text.Email "boss@example.com", name: Text.NonEmpty "Boss", passwordHash: "", role: Manager }
  let _ <- Test.login { user: Just boss }
  let _ <- Test.get (Products.Index {})
  let r <- Test.submit NewProduct { name: Text.NonEmpty "Tea", price: Int.NonNegative 450 }
  Task.succeed ({ answer: r.answer, names: Test.texts "#products .name" r.doc } == { answer: Redirected (Products.Index {}), names: ["Tea"] })
}

test "a Manager toggles a product" = {
  let boss <- Db.insert User { email: Text.Email "boss@example.com", name: Text.NonEmpty "Boss", passwordHash: "", role: Manager }
  let tea <- Db.insert Product { name: Text.NonEmpty "Tea", price: Int.NonNegative 450 }
  let _ <- Test.login { user: Just boss }
  let _ <- Test.get (Products.Index {})
  let r <- Test.click (Toggle { id: tea })
  let after <- Db.get Product tea
  Task.succeed ({ answer: r.answer, active: Maybe.map (\p -> p.active) after } == { answer: Redirected (Products.Index {}), active: Just False })
}

test "Toggle by a Staff user is not found" = {
  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
  let tea <- Db.insert Product { name: Text.NonEmpty "Tea", price: Int.NonNegative 450 }
  let r <- Test.run (update (Toggle { id: tea }) { products: [] } { user: me })
  let after <- Db.get Product tea
  Task.succeed ({ value: r.value, active: Maybe.map (\p -> p.active) after } == { value: Err "not found", active: Just True })
}
