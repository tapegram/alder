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
import User

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

test "total sums quantity times price over the lines" =
  total
    [
      { id: Product.ProductId "p1", name: Text.NonEmpty "Tea", price: Int.NonNegative 250, active: True },
      { id: Product.ProductId "p2", name: Text.NonEmpty "Cake", price: Int.NonNegative 100, active: False },
    ]
    [
      { id: LineId "l1", order: Order.OrderId "o", product: Product.ProductId "p1", quantity: Int.Positive 2 },
      { id: LineId "l2", order: Order.OrderId "o", product: Product.ProductId "p2", quantity: Int.Positive 3 },
    ] == 800

test "total of no lines is zero" =
  total [] [] == 0

test "priceOf finds the product's price, and is zero for an unknown one" =
  { found: priceOf [{ id: Product.ProductId "p1", name: Text.NonEmpty "Tea", price: Int.NonNegative 250, active: True }] (Product.ProductId "p1"), unknown: priceOf [] (Product.ProductId "x") } == { found: 250, unknown: 0 }

test "deleting an order deletes its lines" = {
  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
  let tea <- Db.insert Product { name: Text.NonEmpty "Tea", price: Int.NonNegative 250 }
  let o <- Db.insert Order { owner: me }
  let _ <- Db.insert Line { order: o, product: tea, quantity: Int.Positive 1 }
  let _ <- Db.delete Order o
  let n <- Db.from Line |> Db.count
  Task.succeed (n == 0)
}

test "a product with lines cannot be deleted" = {
  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
  let tea <- Db.insert Product { name: Text.NonEmpty "Tea", price: Int.NonNegative 250 }
  let o <- Db.insert Order { owner: me }
  let _ <- Db.insert Line { order: o, product: tea, quantity: Int.Positive 1 }
  let r <- Test.run (Db.delete Product tea)
  let n <- Db.from Line |> Db.count
  Task.succeed ({ deleted: Result.toMaybe r.value, lines: n } == { deleted: Nothing, lines: 1 })
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

test "each status has its label" =
  List.map statusLabel [Open, Paid, Cancelled] == ["Open", "Paid", "Cancelled"]

test "a new order is open" = {
  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
  let id <- Db.insert Order { owner: me }
  let after <- Db.get Order id
  Task.succeed (Maybe.map (\o -> o.status) after == Just Open)
}

test "a user with orders cannot be deleted" = {
  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
  let id <- Db.insert Order { owner: me }
  let r <- Test.run (Db.delete User me)
  let after <- Db.get Order id
  Task.succeed ({ deleted: Result.toMaybe r.value, order: Maybe.map (\o -> o.owner) after } == { deleted: Nothing, order: Just me })
}

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

test "a visitor is sent to log in from the order list" = {
  let r <- Test.open (Orders.Index {})
  Task.succeed (r.answer == Redirected (Auth.Login {}))
}

test "the list shows only my orders, newest first, with status and total" = {
  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
  let other <- Db.insert User { email: Text.Email "other@example.com", name: Text.NonEmpty "Other", passwordHash: "" }
  let tea <- Db.insert Product { name: Text.NonEmpty "Tea", price: Int.NonNegative 250 }
  let o1 <- Db.insert Order { owner: me }
  let _ <- Db.insert Line { order: o1, product: tea, quantity: Int.Positive 2 }
  let _ <- Db.update Order o1 (\x -> { x | status: Paid })
  let _ <- Db.insert Order { owner: me }
  let o3 <- Db.insert Order { owner: other }
  let _ <- Db.insert Line { order: o3, product: tea, quantity: Int.Positive 7 }
  let _ <- Test.login { user: Just me }
  let page <- Test.get (Orders.Index {})
  Task.succeed ({ items: Test.count "#orders li" page.doc, statuses: Test.texts "#orders .status" page.doc, totals: Test.texts "#orders .total" page.doc } == { items: 2, statuses: ["Open", "Paid"], totals: ["0", "500"] })
}

test "only a manager is told so" = {
  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
  let boss <- Db.insert User { email: Text.Email "boss@example.com", name: Text.NonEmpty "Boss", passwordHash: "", role: Manager }
  let _ <- Test.login { user: Just me }
  let staffPage <- Test.get (Orders.Index {})
  let _ <- Test.login { user: Just boss }
  let bossPage <- Test.get (Orders.Index {})
  Task.succeed ({ staff: staffPage.model.manager, boss: bossPage.model.manager } == { staff: False, boss: True })
}

test "the new order button creates an open order of mine and shows it" = {
  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
  let _ <- Test.login { user: Just me }
  let _ <- Test.get (Orders.Index {})
  let r <- Test.click Create
  let orders <- Db.from Order |> Db.all
  let expected = List.map (\o -> Redirected (Orders.Show { id: o.id })) orders
  Task.succeed ({ answer: [r.answer], owners: List.map (\o -> o.owner) orders, statuses: List.map (\o -> o.status) orders, shown: Test.text "#status" r.doc } == { answer: expected, owners: [me], statuses: [Open], shown: Just "Open" })
}

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

test "an order shows its own lines, with names and costs, and not another order's" = {
  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
  let tea <- Db.insert Product { name: Text.NonEmpty "Tea", price: Int.NonNegative 250 }
  let cake <- Db.insert Product { name: Text.NonEmpty "Cake", price: Int.NonNegative 100 }
  let o <- Db.insert Order { owner: me }
  let o2 <- Db.insert Order { owner: me }
  let _ <- Db.insert Line { order: o, product: tea, quantity: Int.Positive 2 }
  let _ <- Db.insert Line { order: o, product: cake, quantity: Int.Positive 3 }
  let _ <- Db.insert Line { order: o2, product: tea, quantity: Int.Positive 7 }
  let _ <- Test.login { user: Just me }
  let page <- Test.get (Orders.Show { id: o })
  let lines = List.sortByText (\t -> t) (Test.texts "#lines li" page.doc)
  Task.succeed ({ lines: lines, total: Test.text "#total" page.doc } == { lines: ["Cake x 3 = 300", "Tea x 2 = 500"], total: Just "800" })
}

test "a visitor is sent to log in from an order" = {
  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
  let o <- Db.insert Order { owner: me }
  let r <- Test.open (Orders.Show { id: o })
  Task.succeed (r.answer == Redirected (Auth.Login {}))
}

test "an order that does not exist is not found" = {
  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
  let _ <- Test.login { user: Just me }
  let r <- Test.open (Orders.Show { id: Order.OrderId "nope" })
  Task.succeed (r.answer == PageMissing)
}

test "a staff user finds their own order" = {
  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
  let o <- Db.insert Order { owner: me }
  let _ <- Test.login { user: Just me }
  let r <- Test.open (Orders.Show { id: o })
  Task.succeed (r.answer == Shown)
}

test "paying an open order marks it paid" = {
  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
  let o <- Db.insert Order { owner: me }
  let _ <- Test.login { user: Just me }
  let page <- Test.get (Orders.Show { id: o })
  let r <- Test.run (update Pay page.model { user: me })
  let after <- Db.get Order o
  Task.succeed ({ value: r.value, status: Maybe.map (\x -> x.status) after } == { value: Ok (Redirect (Orders.Show { id: o })), status: Just Paid })
}

test "cancelling an open order marks it cancelled" = {
  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
  let o <- Db.insert Order { owner: me }
  let _ <- Test.login { user: Just me }
  let page <- Test.get (Orders.Show { id: o })
  let r <- Test.run (update Cancel page.model { user: me })
  let after <- Db.get Order o
  Task.succeed ({ value: r.value, status: Maybe.map (\x -> x.status) after } == { value: Ok (Redirect (Orders.Show { id: o })), status: Just Cancelled })
}

test "a paid order can be neither paid nor cancelled again" = {
  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
  let o <- Db.insert Order { owner: me }
  let _ <- Test.login { user: Just me }
  let page <- Test.get (Orders.Show { id: o })
  let _ <- Db.update Order o (\x -> { x | status: Paid })
  let pay <- Test.run (update Pay page.model { user: me })
  let cancel <- Test.run (update Cancel page.model { user: me })
  let after <- Db.get Order o
  Task.succeed ({ pay: pay.value, cancel: cancel.value, status: Maybe.map (\x -> x.status) after } == { pay: Ok (Reject "This order is not open."), cancel: Ok (Reject "This order is not open."), status: Just Paid })
}

test "an order that is gone is refused, not paid" = {
  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
  let o <- Db.insert Order { owner: me }
  let _ <- Test.login { user: Just me }
  let page <- Test.get (Orders.Show { id: o })
  let model = page.model
  let order = model.order
  let ghostOrder = { order | id: Order.OrderId "nope" }
  let ghost = { model | order: ghostOrder }
  let r <- Test.run (update Pay ghost { user: me })
  Task.succeed (r.value == Ok (Reject "This order is not open."))
}

test "adding a line to an open order, with an active product, adds it" = {
  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
  let tea <- Db.insert Product { name: Text.NonEmpty "Tea", price: Int.NonNegative 250 }
  let o <- Db.insert Order { owner: me }
  let _ <- Test.login { user: Just me }
  let page <- Test.get (Orders.Show { id: o })
  let r <- Test.run (update (AddLine { product: tea, form: { quantity: Int.Positive 3 } }) page.model { user: me })
  let lines <- Db.from Line |> Db.where (Line.order.eq o) |> Db.all
  Task.succeed ({ value: r.value, quantities: List.map (\l -> Int.abs l.quantity) lines } == { value: Ok (Redirect (Orders.Show { id: o })), quantities: [3] })
}

test "a line for an inactive or unknown product is refused" = {
  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
  let gone <- Db.insert Product { name: Text.NonEmpty "Gone", price: Int.NonNegative 90, active: False }
  let o <- Db.insert Order { owner: me }
  let _ <- Test.login { user: Just me }
  let page <- Test.get (Orders.Show { id: o })
  let inactive <- Test.run (update (AddLine { product: gone, form: { quantity: Int.Positive 1 } }) page.model { user: me })
  let unknown <- Test.run (update (AddLine { product: Product.ProductId "nope", form: { quantity: Int.Positive 1 } }) page.model { user: me })
  let n <- Db.from Line |> Db.count
  Task.succeed ({ inactive: inactive.value, unknown: unknown.value, lines: n } == { inactive: Ok (Reject "This order is not open, or the product is not available."), unknown: Ok (Reject "This order is not open, or the product is not available."), lines: 0 })
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

test "a stale form posted after the order was paid adds nothing" = {
  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
  let tea <- Db.insert Product { name: Text.NonEmpty "Tea", price: Int.NonNegative 250 }
  let o <- Db.insert Order { owner: me }
  let _ <- Test.login { user: Just me }
  let _ <- Test.get (Orders.Show { id: o })
  let _ <- Db.update Order o (\x -> { x | status: Paid })
  let _ <- Test.submit (AddLine { product: tea }) { quantity: Int.Positive 1 }
  let n <- Db.from Line |> Db.count
  Task.succeed (n == 0)
}

test "adding a line and paying through the page" = {
  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
  let tea <- Db.insert Product { name: Text.NonEmpty "Tea", price: Int.NonNegative 250 }
  let o <- Db.insert Order { owner: me }
  let _ <- Test.login { user: Just me }
  let _ <- Test.get (Orders.Show { id: o })
  let added <- Test.submit (AddLine { product: tea }) { quantity: Int.Positive 2 }
  let paid <- Test.click Pay
  Task.succeed ({ added: added.answer, total: Test.text "#total" added.doc, paid: paid.answer, status: Test.text "#status" paid.doc, controls: Test.has "#controls" paid.doc } == { added: Redirected (Orders.Show { id: o }), total: Just "500", paid: Redirected (Orders.Show { id: o }), status: Just "Paid", controls: False })
}

test "cancelling through the page" = {
  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
  let o <- Db.insert Order { owner: me }
  let _ <- Test.login { user: Just me }
  let _ <- Test.get (Orders.Show { id: o })
  let r <- Test.click Cancel
  Task.succeed ({ answer: r.answer, status: Test.text "#status" r.doc } == { answer: Redirected (Orders.Show { id: o }), status: Just "Cancelled" })
}

test "only active products get a form, and a closed order has no controls" = {
  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
  let _ <- Db.insert Product { name: Text.NonEmpty "Tea", price: Int.NonNegative 250 }
  let _ <- Db.insert Product { name: Text.NonEmpty "Gone", price: Int.NonNegative 90, active: False }
  let o <- Db.insert Order { owner: me }
  let _ <- Test.login { user: Just me }
  let open <- Test.get (Orders.Show { id: o })
  let _ <- Db.update Order o (\x -> { x | status: Paid })
  let closed <- Test.get (Orders.Show { id: o })
  Task.succeed ({ active: List.map (\p -> p.name) open.model.active, forms: Test.count "form" open.doc, closedForms: Test.count "form" closed.doc, closedButtons: Test.count "button" closed.doc, controls: Test.has "#controls" closed.doc } == { active: [Text.NonEmpty "Tea"], forms: 1, closedForms: 0, closedButtons: 0, controls: False })
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

--- file: app/Product.alder ---
module Product

type ProductId = brand Text

table Product {
  id: ProductId @id,
  name: Text.NonEmpty,
  price: Int.NonNegative,
  active: Bool = True,
}

test "a new product is active" = {
  let id <- Db.insert Product { name: Text.NonEmpty "Tea", price: Int.NonNegative 250 }
  let after <- Db.get Product id
  Task.succeed (Maybe.map (\p -> p.active) after == Just True)
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

test "a visitor is sent to log in from the products" = {
  let r <- Test.open (Products.Index {})
  Task.succeed (r.answer == Redirected (Auth.Login {}))
}

test "a manager sees every product, sorted by name" = {
  let boss <- Db.insert User { email: Text.Email "boss@example.com", name: Text.NonEmpty "Boss", passwordHash: "", role: Manager }
  let _ <- Db.insert Product { name: Text.NonEmpty "Banana", price: Int.NonNegative 30 }
  let _ <- Db.insert Product { name: Text.NonEmpty "Apple", price: Int.NonNegative 20, active: False }
  let _ <- Test.login { user: Just boss }
  let r <- Test.open (Products.Index {})
  let page <- Test.get (Products.Index {})
  Task.succeed ({ answer: r.answer, names: Test.texts "#products .name" page.doc } == { answer: Shown, names: ["Apple", "Banana"] })
}

test "a manager adds a product through the page" = {
  let boss <- Db.insert User { email: Text.Email "boss@example.com", name: Text.NonEmpty "Boss", passwordHash: "", role: Manager }
  let _ <- Test.login { user: Just boss }
  let _ <- Test.get (Products.Index {})
  let r <- Test.submit NewProduct { name: Text.NonEmpty "Tea", price: Int.NonNegative 250 }
  let products <- Db.from Product |> Db.all
  Task.succeed ({ answer: r.answer, products: List.map (\p -> { name: p.name, price: Int.abs p.price, active: p.active }) products, listed: Test.texts "#products .name" r.doc } == { answer: Redirected (Products.Index {}), products: [{ name: Text.NonEmpty "Tea", price: 250, active: True }], listed: ["Tea"] })
}

test "a manager toggles a product off and on" = {
  let boss <- Db.insert User { email: Text.Email "boss@example.com", name: Text.NonEmpty "Boss", passwordHash: "", role: Manager }
  let tea <- Db.insert Product { name: Text.NonEmpty "Tea", price: Int.NonNegative 250 }
  let _ <- Test.login { user: Just boss }
  let _ <- Test.get (Products.Index {})
  let off <- Test.click (Toggle { id: tea })
  let afterOff <- Db.get Product tea
  let on <- Test.click (Toggle { id: tea })
  let afterOn <- Db.get Product tea
  Task.succeed ({ off: off.answer, offActive: Maybe.map (\p -> p.active) afterOff, on: on.answer, onActive: Maybe.map (\p -> p.active) afterOn } == { off: Redirected (Products.Index {}), offActive: Just False, on: Redirected (Products.Index {}), onActive: Just True })
}

test "toggling an unknown product is not found" = {
  let boss <- Db.insert User { email: Text.Email "boss@example.com", name: Text.NonEmpty "Boss", passwordHash: "", role: Manager }
  let _ <- Test.login { user: Just boss }
  let page <- Test.get (Products.Index {})
  let r <- Test.run (update (Toggle { id: Product.ProductId "nope" }) page.model { user: boss })
  Task.succeed (r.value == Err "not found")
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

test "only a manager is a manager" =
  {
    manager: isManager (Just { id: UserId "m", email: Text.Email "m@example.com", name: Text.NonEmpty "M", passwordHash: "", role: Manager, created: Time.epoch }),
    staff: isManager (Just { id: UserId "s", email: Text.Email "s@example.com", name: Text.NonEmpty "S", passwordHash: "", role: Staff, created: Time.epoch }),
    nobody: isManager Nothing,
  } == { manager: True, staff: False, nobody: False }

test "a new user is staff" = {
  let id <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
  let after <- Db.get User id
  Task.succeed (Maybe.map (\u -> u.role) after == Just Staff)
}
