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

priceOf : Product.ProductId -> List Product -> Int
priceOf id products = match (List.find (\p -> p.id == id) products) {
  Just p -> p.price,
  Nothing -> 0,
}

productName : Product.ProductId -> List Product -> Text
productName id products = match (List.find (\p -> p.id == id) products) {
  Just p -> p.name,
  Nothing -> "",
}

total : List Line -> List Product -> Int
total lines products =
  lines
    |> List.map (\l -> l.quantity * priceOf l.product products)
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

statusLabel : Status -> Text
statusLabel status = match status {
  Open -> "Open",
  Paid -> "Paid",
  Cancelled -> "Cancelled",
}

--- file: app/Orders/Index.alder ---
module Orders.Index

import Order
import Line
import Product
import User
import Layout

page "/" { } requires user

type Model = {
  orders: List Order,
  lines: List Line,
  products: List Product,
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
  let products <-
    Db.from Product
      |> Db.where (Product.id.in (List.map (\l -> l.product) lines))
      |> Db.all
  Query.succeed { orders: orders, lines: lines, products: products }
}

msg Create

update : Msg -> Model -> Session -> Task Nav
update msg model session = match msg {
  Create p -> {
    let id <- Db.insert Order { owner: session.user }
    Task.succeed (Redirect (Orders.Show { id: id }))
  },
}

orderTotal : Model -> Order -> Int
orderTotal model o =
  Line.total (List.filter (\l -> l.order == o.id) model.lines) model.products

view : Model -> Session -> Html Msg
view model session =
  <Layout.Page title="Orders">
    <h1>Orders</h1>

    <button click={Create}>New order</button>

    <ul id="orders">
      {for o in model.orders ->
        <li>
          <a href={Orders.Show { id: o.id }}>{Time.format "2006-01-02 15:04:05" o.created}</a>
          <span class="status">{Order.statusLabel o.status}</span>
          <span class="total">{Text.fromInt (orderTotal model o)}</span>
        </li>}
    </ul>

    <p><a href={Products.Index {}}>Products</a></p>

    <button click={App.Logout}>Log out</button>
  </Layout.Page>

test "a visitor is sent to log in from the orders list" = {
  let r <- Test.open (Orders.Index {})
  Task.succeed (r.answer == Redirected (Auth.Login {}))
}

test "the list shows my orders, newest first, with status and total" = {
  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
  let other <- Db.insert User { email: Text.Email "other@example.com", name: Text.NonEmpty "Other", passwordHash: "" }
  let tea <- Db.insert Product { name: Text.NonEmpty "Tea", price: Int.NonNegative 250 }
  let cake <- Db.insert Product { name: Text.NonEmpty "Cake", price: Int.NonNegative 500 }
  let o1 <- Db.insert Order { owner: me }
  let _ <- Db.insert Line { order: o1, product: tea, quantity: Int.Positive 2 }
  let _ <- Db.insert Line { order: o1, product: cake, quantity: Int.Positive 1 }
  let o2 <- Db.insert Order { owner: me }
  let _ <- Db.insert Line { order: o2, product: tea, quantity: Int.Positive 3 }
  let _ <- Db.update Order o2 (\x -> { x | status: Paid })
  let o3 <- Db.insert Order { owner: me }
  let _ <- Db.update Order o3 (\x -> { x | status: Cancelled })
  let theirs <- Db.insert Order { owner: other }
  let _ <- Db.insert Line { order: theirs, product: cake, quantity: Int.Positive 4 }
  let _ <- Test.login { user: Just me }
  let page <- Test.get (Orders.Index {})
  Task.succeed ({ statuses: Test.texts "#orders .status" page.doc, totals: Test.texts "#orders .total" page.doc, items: Test.count "#orders li" page.doc } == { statuses: ["Cancelled", "Paid", "Open"], totals: ["0", "750", "1000"], items: 3 })
}

test "creating an order makes an empty open order of mine and shows it" = {
  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
  let _ <- Test.login { user: Just me }
  let _ <- Test.get (Orders.Index {})
  let r <- Test.click Create
  let created <- Db.from Order |> Db.all
  let ids = List.map (\o -> o.id) created
  let owners = List.map (\o -> o.owner) created
  let statuses = List.map (\o -> o.status) created
  let expected = Maybe.map (\i -> Redirected (Orders.Show { id: i })) (List.head ids)
  Task.succeed ({ answer: Just r.answer, owners: owners, statuses: statuses, status: Test.text "#status" r.doc } == { answer: expected, owners: [me], statuses: [Open], status: Just "Open" })
}

--- file: app/Orders/Show.alder ---
module Orders.Show

import Order
import Line
import Product
import User
import Layout

page "/orders/{id}" { id: Order.OrderId } requires user

type Model = {
  order: Order,
  lines: List Line,
  products: List Product,
}

mayAccess : Order -> User.UserId -> Maybe User -> Bool
mayAccess order uid me = order.owner == uid || User.isManager me

withLines : Order -> Query Model
withLines order = {
  let lines <- Db.from Line |> Db.where (Line.order.eq order.id) |> Db.all
  let products <- Db.from Product |> Db.all
  Query.succeed { order: order, lines: lines, products: products }
}

load : Params -> Session -> Query Model
load params session = {
  let found <- Db.get Order params.id
  let me <- Db.get User session.user
  match found {
    Just order -> if mayAccess order session.user me then withLines order else Query.notFound,
    Nothing -> Query.notFound,
  }
}

form NewLine = {
  quantity: Int.Positive,
}

msg AddLine { product: Product.ProductId } form NewLine
msg Pay
msg Cancel

notOpenMessage : Text
notOpenMessage = "This order is not open, or the product is not available."

isOpenNow : Order.OrderId -> Task Bool
isOpenNow id = {
  let found <- Db.get Order id
  Task.succeed ((Maybe.map (\o -> o.status == Open) found) ?? False)
}

insertLine : Model -> Product.ProductId -> Int.Positive -> Task Nav
insertLine model productId qty = {
  let _ <- Db.insert Line { order: model.order.id, product: productId, quantity: qty }
  Task.succeed (Redirect (Orders.Show { id: model.order.id }))
}

addLine : Model -> Product.ProductId -> Int.Positive -> Task Nav
addLine model productId qty = {
  let isOpen <- isOpenNow model.order.id
  let found <- Db.get Product productId
  let usable = (Maybe.map (\p -> p.active) found) ?? False
  if isOpen && usable then insertLine model productId qty else Task.succeed (Reject notOpenMessage)
}

finish : Model -> Order.Status -> Task Nav
finish model status = {
  let _ <- Db.update Order model.order.id (\o -> { o | status: status })
  Task.succeed (Redirect (Orders.Show { id: model.order.id }))
}

setStatus : Model -> Order.Status -> Task Nav
setStatus model status = {
  let isOpen <- isOpenNow model.order.id
  if isOpen then finish model status else Task.succeed Rerender
}

update : Msg -> Model -> Session -> Task Nav
update msg model session = match msg {
  AddLine p -> addLine model p.product p.form.quantity,
  Pay p -> setStatus model Paid,
  Cancel p -> setStatus model Cancelled,
}

Controls : { model: Model } -> Html Msg
Controls props =
  <div id="controls">
    {for pr in (List.filter (\p -> p.active) props.model.products) ->
      <form submit={AddLine { product: pr.id }}>
        <h3>{pr.name}</h3>
        <Errors form />
        <Input field="quantity" label="Quantity" />
        <Errors field="quantity" />
        <button>Add</button>
      </form>}
    <button click={Pay}>Mark paid</button>
    <button click={Cancel}>Cancel order</button>
  </div>

view : Model -> Session -> Html Msg
view model session =
  <Layout.Page title="Order">
    <h1>Order</h1>
    <p>Status: <span id="status">{Order.statusLabel model.order.status}</span></p>
    <p>Total: <span id="total">{Text.fromInt (Line.total model.lines model.products)}</span></p>

    <ul id="lines">
      {for l in model.lines ->
        <li>
          <span class="product">{Line.productName l.product model.products}</span>
          x
          <span class="quantity">{Text.fromInt l.quantity}</span>
        </li>}
    </ul>

    {if model.order.status == Open then <Controls model={model} /> else Html.none}

    <a href={Orders.Index {}}>Back</a>
  </Layout.Page>

test "an open order's total over two lines" = {
  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
  let tea <- Db.insert Product { name: Text.NonEmpty "Tea", price: Int.NonNegative 250 }
  let cake <- Db.insert Product { name: Text.NonEmpty "Cake", price: Int.NonNegative 500 }
  let order <- Db.insert Order { owner: me }
  let _ <- Db.insert Line { order: order, product: tea, quantity: Int.Positive 2 }
  let _ <- Db.insert Line { order: order, product: cake, quantity: Int.Positive 1 }
  let _ <- Test.login { user: Just me }
  let page <- Test.get (Orders.Show { id: order })
  Task.succeed ({ status: Test.text "#status" page.doc, total: Test.text "#total" page.doc } == { status: Just "Open", total: Just "1000" })
}

test "adding a line to a paid order is refused" = {
  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
  let tea <- Db.insert Product { name: Text.NonEmpty "Tea", price: Int.NonNegative 250 }
  let order <- Db.insert Order { owner: me }
  let _ <- Test.login { user: Just me }
  let page <- Test.get (Orders.Show { id: order })
  let _ <- Db.update Order order (\o -> { o | status: Paid })
  let r <- Test.run (update (AddLine { product: tea, form: { quantity: Int.Positive 1 } }) page.model { user: me })
  let n <- Db.from Line |> Db.count
  Task.succeed ({ value: r.value, lines: n } == { value: Ok (Reject notOpenMessage), lines: 0 })
}

test "a staff user cannot open another user's order" = {
  let owner <- Db.insert User { email: Text.Email "owner@example.com", name: Text.NonEmpty "Owner", passwordHash: "" }
  let other <- Db.insert User { email: Text.Email "other@example.com", name: Text.NonEmpty "Other", passwordHash: "" }
  let order <- Db.insert Order { owner: owner }
  let _ <- Test.login { user: Just other }
  let r <- Test.open (Orders.Show { id: order })
  Task.succeed (r.answer == PageMissing)
}

test "a manager can open another user's order" = {
  let owner <- Db.insert User { email: Text.Email "owner@example.com", name: Text.NonEmpty "Owner", passwordHash: "" }
  let boss <- Db.insert User { email: Text.Email "boss@example.com", name: Text.NonEmpty "Boss", passwordHash: "", role: Manager }
  let order <- Db.insert Order { owner: owner }
  let _ <- Test.login { user: Just boss }
  let r <- Test.open (Orders.Show { id: order })
  Task.succeed ({ answer: r.answer, status: Test.text "#status" r.doc } == { answer: Shown, status: Just "Open" })
}

test "a visitor is sent to log in from an order" = {
  let owner <- Db.insert User { email: Text.Email "owner@example.com", name: Text.NonEmpty "Owner", passwordHash: "" }
  let order <- Db.insert Order { owner: owner }
  let r <- Test.open (Orders.Show { id: order })
  Task.succeed (r.answer == Redirected (Auth.Login {}))
}

test "a missing order is not found, even for a manager" = {
  let boss <- Db.insert User { email: Text.Email "boss@example.com", name: Text.NonEmpty "Boss", passwordHash: "", role: Manager }
  let _ <- Test.login { user: Just boss }
  let r <- Test.open (Orders.Show { id: Order.OrderId "nope" })
  Task.succeed (r.answer == PageMissing)
}

test "the page lists lines and offers only active products" = {
  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
  let tea <- Db.insert Product { name: Text.NonEmpty "Tea", price: Int.NonNegative 250 }
  let old <- Db.insert Product { name: Text.NonEmpty "Old", price: Int.NonNegative 100, active: False }
  let order <- Db.insert Order { owner: me }
  let _ <- Db.insert Line { order: order, product: tea, quantity: Int.Positive 2 }
  let _ <- Db.insert Line { order: order, product: old, quantity: Int.Positive 1 }
  let _ <- Test.login { user: Just me }
  let page <- Test.get (Orders.Show { id: order })
  Task.succeed ({ offered: Test.texts "#controls h3" page.doc, lines: Test.count "#lines li" page.doc, total: Test.text "#total" page.doc, controls: Test.has "#controls" page.doc } == { offered: ["Tea"], lines: 2, total: Just "600", controls: True })
}

test "adding a line through the form" = {
  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
  let tea <- Db.insert Product { name: Text.NonEmpty "Tea", price: Int.NonNegative 250 }
  let order <- Db.insert Order { owner: me }
  let _ <- Test.login { user: Just me }
  let _ <- Test.get (Orders.Show { id: order })
  let r <- Test.submit (AddLine { product: tea }) { quantity: Int.Positive 3 }
  let n <- Db.from Line |> Db.count
  Task.succeed ({ answer: r.answer, lines: n, total: Test.text "#total" r.doc } == { answer: Redirected (Orders.Show { id: order }), lines: 1, total: Just "750" })
}

test "an inactive or missing product cannot be added" = {
  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
  let old <- Db.insert Product { name: Text.NonEmpty "Old", price: Int.NonNegative 100, active: False }
  let order <- Db.insert Order { owner: me }
  let _ <- Test.login { user: Just me }
  let page <- Test.get (Orders.Show { id: order })
  let r1 <- Test.run (update (AddLine { product: old, form: { quantity: Int.Positive 1 } }) page.model { user: me })
  let r2 <- Test.run (update (AddLine { product: Product.ProductId "nope", form: { quantity: Int.Positive 1 } }) page.model { user: me })
  let n <- Db.from Line |> Db.count
  Task.succeed ({ a: r1.value, b: r2.value, lines: n } == { a: Ok (Reject notOpenMessage), b: Ok (Reject notOpenMessage), lines: 0 })
}

test "paying an open order marks it paid and hides the controls" = {
  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
  let order <- Db.insert Order { owner: me }
  let _ <- Test.login { user: Just me }
  let _ <- Test.get (Orders.Show { id: order })
  let r <- Test.click Pay
  let after <- Db.get Order order
  Task.succeed ({ answer: r.answer, status: Maybe.map (\o -> o.status) after, shown: Test.text "#status" r.doc, controls: Test.has "#controls" r.doc } == { answer: Redirected (Orders.Show { id: order }), status: Just Paid, shown: Just "Paid", controls: False })
}

test "cancelling an open order marks it cancelled and hides the controls" = {
  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
  let order <- Db.insert Order { owner: me }
  let _ <- Test.login { user: Just me }
  let _ <- Test.get (Orders.Show { id: order })
  let r <- Test.click Cancel
  let after <- Db.get Order order
  Task.succeed ({ answer: r.answer, status: Maybe.map (\o -> o.status) after, shown: Test.text "#status" r.doc, controls: Test.has "#controls" r.doc } == { answer: Redirected (Orders.Show { id: order }), status: Just Cancelled, shown: Just "Cancelled", controls: False })
}

test "a closed order cannot change status again" = {
  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
  let o1 <- Db.insert Order { owner: me }
  let o2 <- Db.insert Order { owner: me }
  let _ <- Test.login { user: Just me }
  let p1 <- Test.get (Orders.Show { id: o1 })
  let _ <- Db.update Order o1 (\o -> { o | status: Cancelled })
  let r1 <- Test.run (update Pay p1.model { user: me })
  let p2 <- Test.get (Orders.Show { id: o2 })
  let _ <- Db.update Order o2 (\o -> { o | status: Paid })
  let r2 <- Test.run (update Cancel p2.model { user: me })
  let a <- Db.get Order o1
  let b <- Db.get Order o2
  Task.succeed ({ v1: r1.value, v2: r2.value, s1: Maybe.map (\o -> o.status) a, s2: Maybe.map (\o -> o.status) b } == { v1: Ok Rerender, v2: Ok Rerender, s1: Just Cancelled, s2: Just Paid })
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

import Product
import User
import Layout

page "/products" { } requires user

type Model = { products: List Product }

listAll : Query Model
listAll = {
  let products <- Db.from Product |> Db.orderBy Product.name.asc |> Db.all
  Query.succeed { products: products }
}

load : Params -> Session -> Query Model
load params session = {
  let me <- Db.get User session.user
  if User.isManager me then listAll else Query.notFound
}

form NewProduct = {
  name: Text.NonEmpty,
  price: Int.NonNegative,
}

msg Add form NewProduct
msg Toggle { id: Product.ProductId }

toggle : Product.ProductId -> Task Nav
toggle id = {
  let found <- Db.get Product id
  match found {
    Just prod -> {
      let _ <- Db.update Product prod.id (\x -> { x | active: not x.active })
      Task.succeed (Redirect (Products.Index {}))
    },
    Nothing -> Task.notFound,
  }
}

add : NewProduct -> Task Nav
add f = {
  let _ <- Db.insert Product { name: f.name, price: f.price }
  Task.succeed (Redirect (Products.Index {}))
}

act : Msg -> Task Nav
act msg = match msg {
  Add p -> add p.form,
  Toggle p -> toggle p.id,
}

update : Msg -> Model -> Session -> Task Nav
update msg model session = {
  let me <- Db.get User session.user
  if User.isManager me then act msg else Task.notFound
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
      {for pr in model.products ->
        <li>
          <span class="name">{pr.name}</span>
          <span class="price">{Text.fromInt pr.price}</span>
          <span class="active">{if pr.active then "Active" else "Inactive"}</span>
          <button click={Toggle { id: pr.id }}>{if pr.active then "Deactivate" else "Activate"}</button>
        </li>}
    </ul>

    <a href={Orders.Index {}}>Back</a>
  </Layout.Page>

test "a visitor is sent to log in from the products page" = {
  let r <- Test.open (Products.Index {})
  Task.succeed (r.answer == Redirected (Auth.Login {}))
}

test "a staff user gets not found on the products page" = {
  let staff <- Db.insert User { email: Text.Email "staff@example.com", name: Text.NonEmpty "Staff", passwordHash: "" }
  let _ <- Test.login { user: Just staff }
  let r <- Test.open (Products.Index {})
  Task.succeed (r.answer == PageMissing)
}

test "a manager sees every product ordered by name" = {
  let boss <- Db.insert User { email: Text.Email "boss@example.com", name: Text.NonEmpty "Boss", passwordHash: "", role: Manager }
  let _ <- Db.insert Product { name: Text.NonEmpty "Tea", price: Int.NonNegative 250 }
  let _ <- Db.insert Product { name: Text.NonEmpty "Cake", price: Int.NonNegative 500, active: False }
  let _ <- Test.login { user: Just boss }
  let r <- Test.open (Products.Index {})
  Task.succeed ({ answer: r.answer, names: Test.texts ".name" r.doc } == { answer: Shown, names: ["Cake", "Tea"] })
}

test "a manager toggles a product on and off" = {
  let boss <- Db.insert User { email: Text.Email "boss@example.com", name: Text.NonEmpty "Boss", passwordHash: "", role: Manager }
  let p <- Db.insert Product { name: Text.NonEmpty "Tea", price: Int.NonNegative 250 }
  let _ <- Test.login { user: Just boss }
  let _ <- Test.get (Products.Index {})
  let r1 <- Test.click (Toggle { id: p })
  let mid <- Db.get Product p
  let r2 <- Test.click (Toggle { id: p })
  let end <- Db.get Product p
  Task.succeed ({ a1: r1.answer, a2: r2.answer, mid: Maybe.map (\x -> x.active) mid, end: Maybe.map (\x -> x.active) end } == { a1: Redirected (Products.Index {}), a2: Redirected (Products.Index {}), mid: Just False, end: Just True })
}

test "a manager adds a product through the form" = {
  let boss <- Db.insert User { email: Text.Email "boss@example.com", name: Text.NonEmpty "Boss", passwordHash: "", role: Manager }
  let _ <- Test.login { user: Just boss }
  let _ <- Test.get (Products.Index {})
  let r <- Test.submit NewProduct { name: Text.NonEmpty "Cake", price: Int.NonNegative 500 }
  let all <- Db.from Product |> Db.all
  Task.succeed ({ answer: r.answer, names: Test.texts ".name" r.doc, prices: List.map (\x -> x.price) all, actives: List.map (\x -> x.active) all } == { answer: Redirected (Products.Index {}), names: ["Cake"], prices: [Int.NonNegative 500], actives: [True] })
}

test "a staff user cannot toggle or add by posting messages" = {
  let staff <- Db.insert User { email: Text.Email "staff@example.com", name: Text.NonEmpty "Staff", passwordHash: "" }
  let p <- Db.insert Product { name: Text.NonEmpty "Tea", price: Int.NonNegative 250 }
  let r1 <- Test.run (update (Toggle { id: p }) { products: [] } { user: staff })
  let r2 <- Test.run (update (Add { form: { name: Text.NonEmpty "Cake", price: Int.NonNegative 500 } }) { products: [] } { user: staff })
  let after <- Db.get Product p
  let n <- Db.from Product |> Db.count
  Task.succeed ({ a: r1.value, b: r2.value, active: Maybe.map (\x -> x.active) after, count: n } == { a: Err "not found", b: Err "not found", active: Just True, count: 1 })
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

isManager : Maybe User -> Bool
isManager me = match me {
  Just u -> u.role == Manager,
  Nothing -> False,
}
