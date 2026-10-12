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
  <Layout.Page title="Error" manager={False}>
    <h1>Something went wrong</h1>
    <p>Please try again.</p>
  </Layout.Page>

notFound : Html Never
notFound =
  <Layout.Page title="Not found" manager={False}>
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
  <Layout.Page title="Log in" manager={False}>
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

Page : { title: Text, manager: Bool, children: List (Html a) } -> Html a
Page props =
  <html>
    <head>
      <meta charset="utf-8" />
      <title>{props.title}</title>
    </head>
    <body>
      <header>
        <a href={Orders.Index {}}>Orders</a>
        {if props.manager then <a href={Products.Index {}}>Products</a> else Html.none}
        <button click={App.Logout}>Log out</button>
      </header>
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

-- One line of an order, with the product it names and its cost in cents.
type LineRow = { product: Product, quantity: Int, cost: Int }

mkRow : Product -> Line -> LineRow
mkRow p line = { product: p, quantity: line.quantity, cost: line.quantity * p.price }

rowOf : List Product -> Line -> Maybe LineRow
rowOf products line = Maybe.map (\p -> mkRow p line) (List.find (\p -> p.id == line.product) products)

rowsOf : List Product -> List Line -> List LineRow
rowsOf products lines = List.filterMap (rowOf products) lines

totalOf : List LineRow -> Int
totalOf rows = List.sum (List.map (\r -> r.cost) rows)

-- The total in cents of the given lines: quantity times price, summed.
orderTotal : List Product -> List Line -> Int
orderTotal products lines = totalOf (rowsOf products lines)

test "an order's total sums its lines" =
  orderTotal
    [{ id: Product.ProductId "p1", name: Text.NonEmpty "Tea", price: Int.NonNegative 250, active: True }]
    [{ id: LineId "l1", order: Order.OrderId "o1", product: Product.ProductId "p1", quantity: Int.Positive 2 }] == 500

test "a line whose product is unknown costs nothing" =
  orderTotal []
    [{ id: LineId "l1", order: Order.OrderId "o1", product: Product.ProductId "p1", quantity: Int.Positive 2 }] == 0

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

statusText : Status -> Text
statusText status = match status {
  Open -> "Open",
  Paid -> "Paid",
  Cancelled -> "Cancelled",
}

test "statuses read as words" = [statusText Open, statusText Paid, statusText Cancelled] == ["Open", "Paid", "Cancelled"]

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
  orders: List Row,
  manager: Bool,
}

orderRow : List Product -> List Line -> Order -> Row
orderRow products lines order = {
  order: order,
  total: Line.orderTotal products (List.filter (\l -> l.order == order.id) lines),
}

load : Params -> Session -> Query Model
load params session = {
  let me <- Db.get User session.user
  let orders <-
    Db.from Order
      |> Db.where (Order.owner.eq session.user)
      |> Db.orderBy Order.created.desc
      |> Db.all
  let lines <- Db.from Line |> Db.all
  let products <- Db.from Product |> Db.all
  Query.succeed { orders: List.map (orderRow products lines) orders, manager: User.isManager me }
}

update : Msg -> Model -> Session -> Task Nav
update msg model session = match msg {
  Create p -> {
    let id <- Db.insert Order { owner: session.user }
    Task.succeed (Redirect (Orders.Show { id: id }))
  },
}

view : Model -> Session -> Html Msg
view model session =
  <Layout.Page title="Orders" manager={model.manager}>
    <h1>Orders</h1>
    <button click={Create}>New order</button>
    <ul id="orders">
      {for row in model.orders ->
        <li>
          <a href={Orders.Show { id: row.order.id }}>Details</a>
          <span class="status">{Order.statusText row.order.status}</span>
          <span class="total">{Text.fromInt row.total}</span>
        </li>}
    </ul>
  </Layout.Page>

msg Create

test "a new order starts open and empty" = {
  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
  let _ <- Test.login { user: Just me }
  let _ <- Test.get (Orders.Index {})
  let r <- Test.click Create
  Task.succeed ({ status: Test.text "#status" r.doc, total: Test.text "#total" r.doc } == { status: Just "Open", total: Just "0" })
}

test "New order answers with a redirect to the new order" = {
  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
  let _ <- Test.login { user: Just me }
  let _ <- Test.get (Orders.Index {})
  let r <- Test.click Create
  let made <- Db.from Order |> Db.first
  Task.succeed (Maybe.map (\o -> Redirected (Orders.Show { id: o.id })) made == Just r.answer)
}

test "a visitor is sent to log in" = {
  let r <- Test.open (Orders.Index {})
  Task.succeed (r.answer == Redirected (Auth.Login {}))
}

test "each order shows its status and total, newest first" = {
  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
  let tea <- Db.insert Product { name: Text.NonEmpty "Tea", price: Int.NonNegative 250 }
  let paid <- Db.insert Order { owner: me, status: Paid }
  let _ <- Db.insert Line { order: paid, product: tea, quantity: Int.Positive 2 }
  let _ <- Db.insert Order { owner: me }
  let _ <- Test.login { user: Just me }
  let page <- Test.get (Orders.Index {})
  Task.succeed ({ statuses: Test.texts ".status" page.doc, totals: Test.texts ".total" page.doc } == { statuses: ["Open", "Paid"], totals: ["0", "500"] })
}

test "another user's orders are not listed" = {
  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
  let other <- Db.insert User { email: Text.Email "other@example.com", name: Text.NonEmpty "Other", passwordHash: "" }
  let _ <- Db.insert Order { owner: other }
  let _ <- Test.login { user: Just me }
  let page <- Test.get (Orders.Index {})
  Task.succeed (Test.count "#orders li" page.doc == 0)
}

test "the header offers Products to a manager only" = {
  let staff <- Db.insert User { email: Text.Email "sam@example.com", name: Text.NonEmpty "Sam", passwordHash: "" }
  let boss <- Db.insert User { email: Text.Email "boss@example.com", name: Text.NonEmpty "Boss", passwordHash: "", role: Manager }
  let _ <- Test.login { user: Just staff }
  let a <- Test.get (Orders.Index {})
  let _ <- Test.login { user: Just boss }
  let b <- Test.get (Orders.Index {})
  Task.succeed ({ staff: Test.count "header a" a.doc, boss: Test.count "header a" b.doc } == { staff: 1, boss: 2 })
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
  manager: Bool,
  lines: List Line.LineRow,
  total: Int,
  products: List Product,
}

-- The order as the page shows it: its lines, their total, and the products that can be added.
shown : Bool -> Order -> Query Model
shown manager order = {
  let lines <- Db.from Line |> Db.where (Line.order.eq order.id) |> Db.all
  let products <- Db.from Product |> Db.orderBy Product.name.asc |> Db.all
  Query.succeed {
    order: order,
    manager: manager,
    lines: Line.rowsOf products lines,
    total: Line.orderTotal products lines,
    products: List.filter (\p -> p.active) products,
  }
}

load : Params -> Session -> Query Model
load params session = {
  let found <- Db.get Order params.id
  let me <- Db.get User session.user
  let manager = User.isManager me
  match found {
    Just order -> if order.owner == session.user || manager then shown manager order else Query.notFound,
    Nothing -> Query.notFound,
  }
}

form NewLine = {
  quantity: Int.Positive,
}

msg AddLine { product: Product.ProductId } form NewLine
msg Pay
msg Cancel

-- Adding a line is refused unless the order is still Open and the product is active; the
-- check runs here, so a page loaded while the order was Open cannot add to a Paid one.
-- The two Reject answers below are not seen by a user: a refused line leaves the page without
-- the form that sent it (an order that is not Open has no forms, and an inactive product has
-- none), so the page is drawn again without the text and the answer is Rerendered, the same as
-- for Rerender. No input tells Reject from Rerender here; the refusals are tested by their
-- answer and by the lines they leave behind.
addLine : Order -> Product -> Int.Positive -> Task Nav
addLine order product quantity =
  if order.status != Open then Task.succeed (Reject "This order is no longer open.") else if product.active then addRow order product quantity else Task.succeed (Reject "That product is no longer for sale.")

addRow : Order -> Product -> Int.Positive -> Task Nav
addRow order product quantity = {
  let _ <- Db.insert Line { order: order.id, product: product.id, quantity: quantity }
  Task.succeed (Redirect (Orders.Show { id: order.id }))
}

setStatus : Order -> Order.Status -> Task Nav
setStatus order status =
  if order.status == Open then saveStatus order status else Task.succeed (Redirect (Orders.Show { id: order.id }))

saveStatus : Order -> Order.Status -> Task Nav
saveStatus order status = {
  let _ <- Db.update Order order.id (\o -> { o | status: status })
  Task.succeed (Redirect (Orders.Show { id: order.id }))
}

update : Msg -> Model -> Session -> Task Nav
update msg model session = match msg {
  AddLine p -> {
    let found <- Db.get Product p.product
    match found {
      Just product -> addLine model.order product p.form.quantity,
      Nothing -> Task.notFound,
    }
  },
  Pay p -> setStatus model.order Paid,
  Cancel p -> setStatus model.order Cancelled,
}

-- The forms and buttons of an Open order: one form per active product, and the two buttons.
Actions : { products: List Product } -> Html Msg
Actions props =
  <div class="actions">
    {for p in props.products ->
      <form submit={AddLine { product: p.id }}>
        <Errors form />
        <span>{p.name}</span>
        <Input field="quantity" label="Quantity" />
        <Errors field="quantity" />
        <button>Add</button>
      </form>}
    <button click={Pay}>Mark paid</button>
    <button click={Cancel}>Cancel order</button>
  </div>

view : Model -> Session -> Html Msg
view model session =
  <Layout.Page title="Order" manager={model.manager}>
    <h1>Order</h1>
    <p>Status: <span id="status">{Order.statusText model.order.status}</span></p>
    <ul id="lines">
      {for row in model.lines ->
        <li>{row.product.name}: {Text.fromInt row.quantity} x {Text.fromInt row.cost}</li>}
    </ul>
    <p>Total: <span id="total">{Text.fromInt model.total}</span></p>
    {if model.order.status == Open then <Actions products={model.products} /> else Html.none}
    <a href={Orders.Index {}}>Back to orders</a>
  </Layout.Page>

test "an open order's total is the sum of its lines" = {
  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
  let tea <- Db.insert Product { name: Text.NonEmpty "Tea", price: Int.NonNegative 250 }
  let cake <- Db.insert Product { name: Text.NonEmpty "Cake", price: Int.NonNegative 1000 }
  let order <- Db.insert Order { owner: me }
  let _ <- Test.login { user: Just me }
  let _ <- Test.get (Orders.Show { id: order })
  let _ <- Test.submit (AddLine { product: tea }) { quantity: Int.Positive 2 }
  let r <- Test.submit (AddLine { product: cake }) { quantity: Int.Positive 1 }
  Task.succeed ({ total: Test.text "#total" r.doc, lines: Test.count "#lines li" r.doc } == { total: Just "1500", lines: 2 })
}

test "a line cannot be added to a paid order" = {
  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
  let tea <- Db.insert Product { name: Text.NonEmpty "Tea", price: Int.NonNegative 250 }
  let order <- Db.insert Order { owner: me }
  let _ <- Test.login { user: Just me }
  let _ <- Test.get (Orders.Show { id: order })
  let _ <- Db.update Order order (\o -> { o | status: Paid })
  let r <- Test.submit (AddLine { product: tea }) { quantity: Int.Positive 1 }
  let lines <- Db.from Line |> Db.count
  Task.succeed ({ lines: lines, status: Test.text "#status" r.doc } == { lines: 0, status: Just "Paid" })
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
  let boss <- Db.insert User { email: Text.Email "boss@example.com", name: Text.NonEmpty "Boss", passwordHash: "", role: Manager }
  let owner <- Db.insert User { email: Text.Email "owner@example.com", name: Text.NonEmpty "Owner", passwordHash: "" }
  let order <- Db.insert Order { owner: owner }
  let _ <- Test.login { user: Just boss }
  let page <- Test.get (Orders.Show { id: order })
  Task.succeed (Test.text "#status" page.doc == Just "Open")
}

test "a visitor is sent to log in" = {
  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
  let order <- Db.insert Order { owner: me }
  let r <- Test.open (Orders.Show { id: order })
  Task.succeed (r.answer == Redirected (Auth.Login {}))
}

test "an owner can open their own order" = {
  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
  let order <- Db.insert Order { owner: me }
  let _ <- Test.login { user: Just me }
  let r <- Test.open (Orders.Show { id: order })
  Task.succeed (r.answer == Shown)
}

test "an unknown order is not found" = {
  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
  let _ <- Test.login { user: Just me }
  let r <- Test.open (Orders.Show { id: Order.OrderId "nope" })
  Task.succeed (r.answer == PageMissing)
}

test "a page lists only its own order's lines" = {
  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
  let tea <- Db.insert Product { name: Text.NonEmpty "Tea", price: Int.NonNegative 250 }
  let cake <- Db.insert Product { name: Text.NonEmpty "Cake", price: Int.NonNegative 1000 }
  let a <- Db.insert Order { owner: me }
  let b <- Db.insert Order { owner: me }
  let _ <- Db.insert Line { order: a, product: tea, quantity: Int.Positive 2 }
  let _ <- Db.insert Line { order: b, product: cake, quantity: Int.Positive 1 }
  let _ <- Test.login { user: Just me }
  let r <- Test.get (Orders.Show { id: a })
  Task.succeed ({ total: Test.text "#total" r.doc, lines: Test.count "#lines li" r.doc } == { total: Just "500", lines: 1 })
}

test "products are offered by name, active ones only" = {
  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
  let order <- Db.insert Order { owner: me }
  let _ <- Db.insert Product { name: Text.NonEmpty "Tea", price: Int.NonNegative 250 }
  let _ <- Db.insert Product { name: Text.NonEmpty "Cake", price: Int.NonNegative 1000 }
  let _ <- Db.insert Product { name: Text.NonEmpty "Bread", price: Int.NonNegative 300, active: False }
  let _ <- Test.login { user: Just me }
  let r <- Test.get (Orders.Show { id: order })
  Task.succeed (Test.texts "form > span" r.doc == ["Cake", "Tea"])
}

test "adding a line answers with a redirect to the order" = {
  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
  let tea <- Db.insert Product { name: Text.NonEmpty "Tea", price: Int.NonNegative 250 }
  let order <- Db.insert Order { owner: me }
  let _ <- Test.login { user: Just me }
  let _ <- Test.get (Orders.Show { id: order })
  let r <- Test.submit (AddLine { product: tea }) { quantity: Int.Positive 1 }
  Task.succeed (r.answer == Redirected (Orders.Show { id: order }))
}

test "a product deactivated after the page was loaded is refused" = {
  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
  let tea <- Db.insert Product { name: Text.NonEmpty "Tea", price: Int.NonNegative 250 }
  let order <- Db.insert Order { owner: me }
  let _ <- Test.login { user: Just me }
  let _ <- Test.get (Orders.Show { id: order })
  let _ <- Db.update Product tea (\p -> { p | active: False })
  let r <- Test.submit (AddLine { product: tea }) { quantity: Int.Positive 1 }
  let lines <- Db.from Line |> Db.count
  Task.succeed ({ answer: r.answer, lines: lines } == { answer: Rerendered, lines: 0 })
}

test "a product that is gone cannot be added" = {
  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
  let tea <- Db.insert Product { name: Text.NonEmpty "Tea", price: Int.NonNegative 250 }
  let order <- Db.insert Order { owner: me }
  let _ <- Test.login { user: Just me }
  let _ <- Test.get (Orders.Show { id: order })
  let _ <- Db.delete Product tea
  let r <- Test.submit (AddLine { product: tea }) { quantity: Int.Positive 1 }
  Task.succeed (r.answer == PageMissing)
}

test "Mark paid sets the status to Paid" = {
  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
  let order <- Db.insert Order { owner: me }
  let _ <- Test.login { user: Just me }
  let _ <- Test.get (Orders.Show { id: order })
  let r <- Test.click Pay
  let after <- Db.get Order order
  Task.succeed ({ answer: r.answer, status: Maybe.map (\o -> o.status) after } == { answer: Redirected (Orders.Show { id: order }), status: Just Paid })
}

test "Cancel sets the status to Cancelled" = {
  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
  let order <- Db.insert Order { owner: me }
  let _ <- Test.login { user: Just me }
  let _ <- Test.get (Orders.Show { id: order })
  let r <- Test.click Cancel
  let after <- Db.get Order order
  Task.succeed ({ answer: r.answer, status: Maybe.map (\o -> o.status) after } == { answer: Redirected (Orders.Show { id: order }), status: Just Cancelled })
}

test "a stale Cancel does not change a paid order" = {
  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
  let order <- Db.insert Order { owner: me }
  let _ <- Test.login { user: Just me }
  let _ <- Test.get (Orders.Show { id: order })
  let _ <- Db.update Order order (\o -> { o | status: Paid })
  let r <- Test.click Cancel
  let after <- Db.get Order order
  Task.succeed ({ answer: r.answer, status: Maybe.map (\o -> o.status) after } == { answer: Redirected (Orders.Show { id: order }), status: Just Paid })
}

test "only an open order shows its actions" = {
  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
  let openOrder <- Db.insert Order { owner: me }
  let paidOrder <- Db.insert Order { owner: me, status: Paid }
  let _ <- Test.login { user: Just me }
  let a <- Test.get (Orders.Show { id: openOrder })
  let b <- Test.get (Orders.Show { id: paidOrder })
  Task.succeed ({ open: Test.count "div.actions" a.doc, paid: Test.count "div.actions" b.doc } == { open: 1, paid: 0 })
}

test "deleting an order deletes its lines" = {
  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
  let tea <- Db.insert Product { name: Text.NonEmpty "Tea", price: Int.NonNegative 250 }
  let order <- Db.insert Order { owner: me }
  let _ <- Db.insert Line { order: order, product: tea, quantity: Int.Positive 1 }
  let _ <- Db.delete Order order
  let left <- Db.from Line |> Db.count
  Task.succeed (left == 0)
}

test "a product with lines cannot be deleted" = {
  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
  let tea <- Db.insert Product { name: Text.NonEmpty "Tea", price: Int.NonNegative 250 }
  let order <- Db.insert Order { owner: me }
  let _ <- Db.insert Line { order: order, product: tea, quantity: Int.Positive 1 }
  let r <- Test.run (Db.delete Product tea)
  let lines <- Db.from Line |> Db.count
  Task.succeed ({ refused: Result.toMaybe r.value == Nothing, lines: lines } == { refused: True, lines: 1 })
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
import Layout
import User

page "/products" { } requires user

type Model = { products: List Product }

-- The product list, for a Manager only; `load` answers any other user with a 404.
listed : Query Model
listed = {
  let products <- Db.from Product |> Db.orderBy Product.name.asc |> Db.all
  Query.succeed { products: products }
}

load : Params -> Session -> Query Model
load params session = {
  let me <- Db.get User session.user
  let manager = User.isManager me
  if manager then listed else Query.notFound
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
  <Layout.Page title="Products" manager={True}>
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
      {for p in model.products ->
        <li class={if p.active then "" else "inactive"}>
          <span class="name">{p.name}</span>
          <span class="price">{Text.fromInt p.price}</span>
          <button click={Toggle { id: p.id }}>{if p.active then "Deactivate" else "Activate"}</button>
        </li>}
    </ul>
  </Layout.Page>

test "a staff user cannot open products" = {
  let staff <- Db.insert User { email: Text.Email "sam@example.com", name: Text.NonEmpty "Sam", passwordHash: "" }
  let _ <- Test.login { user: Just staff }
  let r <- Test.open (Products.Index {})
  Task.succeed (r.answer == PageMissing)
}

test "a visitor is sent to log in" = {
  let r <- Test.open (Products.Index {})
  Task.succeed (r.answer == Redirected (Auth.Login {}))
}

test "a manager adds a product with its price" = {
  let boss <- Db.insert User { email: Text.Email "boss@example.com", name: Text.NonEmpty "Boss", passwordHash: "", role: Manager }
  let _ <- Test.login { user: Just boss }
  let _ <- Test.get (Products.Index {})
  let r <- Test.submit NewProduct { name: Text.NonEmpty "Tea", price: Int.NonNegative 250 }
  Task.succeed ({ answer: r.answer, names: Test.texts ".name" r.doc, prices: Test.texts ".price" r.doc } == { answer: Redirected (Products.Index {}), names: ["Tea"], prices: ["250"] })
}

test "products are listed by name" = {
  let boss <- Db.insert User { email: Text.Email "boss@example.com", name: Text.NonEmpty "Boss", passwordHash: "", role: Manager }
  let _ <- Db.insert Product { name: Text.NonEmpty "Tea", price: Int.NonNegative 250 }
  let _ <- Db.insert Product { name: Text.NonEmpty "Cake", price: Int.NonNegative 1000 }
  let _ <- Test.login { user: Just boss }
  let page <- Test.get (Products.Index {})
  Task.succeed (Test.texts ".name" page.doc == ["Cake", "Tea"])
}

test "Toggle deactivates a product" = {
  let boss <- Db.insert User { email: Text.Email "boss@example.com", name: Text.NonEmpty "Boss", passwordHash: "", role: Manager }
  let tea <- Db.insert Product { name: Text.NonEmpty "Tea", price: Int.NonNegative 250 }
  let _ <- Test.login { user: Just boss }
  let _ <- Test.get (Products.Index {})
  let r <- Test.click (Toggle { id: tea })
  let after <- Db.get Product tea
  Task.succeed ({ answer: r.answer, active: Maybe.map (\p -> p.active) after } == { answer: Redirected (Products.Index {}), active: Just False })
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

-- Whether the user is a Manager. A visitor who is not logged in is not one.
isManager : Maybe User -> Bool
isManager found = match found {
  Just u -> u.role == Manager,
  Nothing -> False,
}

test "a Manager is a manager" =
  isManager (Just { id: UserId "u1", email: Text.Email "boss@example.com", name: Text.NonEmpty "Ann", passwordHash: "", role: Manager, created: Time.epoch }) == True

test "a Staff user is not a manager" =
  isManager (Just { id: UserId "u2", email: Text.Email "sam@example.com", name: Text.NonEmpty "Sam", passwordHash: "", role: Staff, created: Time.epoch }) == False

test "a visitor is not a manager" = isManager Nothing == False
