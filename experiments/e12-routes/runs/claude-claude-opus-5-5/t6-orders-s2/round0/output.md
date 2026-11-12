--- file: app/User.alder ---
module User

type UserId = brand Text

-- A Staff user sees their own orders. A Manager also sees everyone's orders and manages the
-- products. The session holds only the user: the role is read from the user's row on every
-- request that needs it, so a change of role applies at once.
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

-- The start of every route for Managers only: anyone else gets a 404.
managerOnly : UserId -> Query Unit
managerOnly id = {
  let manager <- isManager id
  if manager then Query.succeed () else Query.notFound
}

test "a Manager is a manager, a Staff user is not" = {
  let staff <- Db.insert User {
    email: Text.Email "staff@example.com",
    name: Text.NonEmpty "Staff",
    passwordHash: "",
  }
  let boss <- Db.insert User {
    email: Text.Email "boss@example.com",
    name: Text.NonEmpty "Boss",
    passwordHash: "",
    role: Manager,
  }
  let a <- isManager staff
  let b <- isManager boss
  Task.succeed ({ staff: a, boss: b } == { staff: False, boss: True })
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

linesOf : Order.OrderId -> Query (List Line)
linesOf id =
  Db.from Line
    |> Db.where (Line.order.eq id)
    |> Db.all

-- Quantity times the product's price, in cents.
lineTotal : List Product -> Line -> Int
lineTotal products line = match List.find (\p -> p.id == line.product) products {
  Just p -> line.quantity * p.price,
  Nothing -> 0,
}

total : List Product -> List Line -> Int
total products lines =
  lines
    |> List.map (lineTotal products)
    |> List.sum

test "a total is the sum of quantity times price" =
  total [
    {
      id: Product.ProductId "tea",
      name: Text.NonEmpty "Tea",
      price: Int.NonNegative 250,
      active: True,
    },
    {
      id: Product.ProductId "bun",
      name: Text.NonEmpty "Bun",
      price: Int.NonNegative 100,
      active: True,
    },
  ] [
    {
      id: LineId "a",
      order: Order.OrderId "o",
      product: Product.ProductId "tea",
      quantity: Int.Positive 2,
    },
    {
      id: LineId "b",
      order: Order.OrderId "o",
      product: Product.ProductId "bun",
      quantity: Int.Positive 3,
    },
  ] == 800

--- file: app/App.alder ---
module App

import Layout
import User

session { user: Maybe User.UserId }

login = Auth.Login.Login

route Logout = DELETE "/session" { }

Logout.update : Logout.Params -> Session -> Task Response
Logout.update p session = {
  let _ <- Session.put (\s -> { s | user: Nothing })
  Task.succeed (Response.redirect (Auth.Login.Login {}))
}

error : Error -> Html
error err =
  <Layout.Page title="Error">
    <h1>Something went wrong</h1>
    <p>Please try again.</p>
  </Layout.Page>

notFound : Html
notFound = <Layout.Page title="Not found"><h1>Not found</h1></Layout.Page>

test "Logout clears the user and goes to log in" = {
  let r <- Test.run (Logout.update {} { user: Just (User.UserId "me") })
  Task.succeed (
    { value: r.value, trace: r.trace }
      == { value: Ok (Response.redirect (Auth.Login.Login {})), trace: ["Session.put"] }
  )
}

--- file: app/Layout.alder ---
module Layout

Page : { title: Text, children: List Html } -> Html
Page props =
  <html>
    <head>
      <meta charset="utf-8" />
      <title>{props.title}</title>
      <link rel="stylesheet" href={Url.external "/static/al/al.css"} />
      <script type="module" src="/static/al/al.js"></script>
    </head>
    <body>
      <al-page width="narrow"><main><al-stack gap="lg">{props.children}</al-stack></main></al-page>
    </body>
  </html>

--- file: app/Auth/Login.alder ---
module Auth.Login

import Layout
import User

-- Password checking is outside the language: a capability implemented in Go.
capability Password {
  verify : { email: Text.Email, password: Text } -> Task (Maybe User.UserId),
}

-- In a test, `Test.fake Password` puts this in the adapter's place: it accepts any user who has
-- a row, whatever the password.
fake Password {
  verify args = {
    let found <-
      Db.from User
        |> Db.where (User.email.eq args.email)
        |> Db.first
    Task.succeed (Maybe.map (\u -> u.id) found)
  }
}

route Login  = GET  "/login"   { }
route Submit = POST "/session" { } form Credentials

type Model = { }

Login.load : Login.Params -> Session -> Query Model
Login.load params session = Query.succeed {}

form Credentials = { email: Text.Email, password: Text.Secret }

Submit.update : Submit.Params -> Session -> Task Response
Submit.update p session = {
  let found <- Password.verify { email: p.form.email, password: p.form.password }
  match found {
    Just id -> {
      let _ <- Session.put (\s -> { s | user: Just id })
      Task.succeed (Response.redirect (Orders.Index.Index {}))
    },
    Nothing -> Task.succeed (Response.invalid "Email or password is incorrect."),
  }
}

Login.view : Model -> Session -> Html
Login.view model session =
  <Layout.Page title="Log in">
    <h1>Log in</h1>
    <al-card>
      <form submit={Submit}>
        <Errors form />
        <al-input field="email" label="Email" />
        <al-input field="password" label="Password" />
        <al-button type="submit">Log in</al-button>
      </form>
    </al-card>
  </Layout.Page>

test "an unknown email is rejected" = {
  let _ <- Test.fake Password
  let _ <- Test.get (Login {})
  let r <- Test.submit Submit {
    email: Text.Email "nobody@example.com",
    password: Text.Secret "secret",
  }
  Task.succeed (
    { answer: r.answer, user: r.session.user }
      == {
        answer: FormRejected [{ field: "", message: "Email or password is incorrect." }],
        user: Nothing,
      }
  )
}

test "logging in through the form" = {
  let _ <- Test.fake Password
  let me <- Db.insert User {
    email: Text.Email "me@example.com",
    name: Text.NonEmpty "Me",
    passwordHash: "",
  }
  let _ <- Test.get (Login {})
  let r <- Test.submit Submit { email: Text.Email "me@example.com", password: Text.Secret "secret" }
  Task.succeed (
    { answer: r.answer, user: r.session.user }
      == { answer: Redirected (Orders.Index.Index {}), user: Just me }
  )
}

test "a visitor is sent to log in, and can log out" = {
  let me <- Db.insert User {
    email: Text.Email "me@example.com",
    name: Text.NonEmpty "Me",
    passwordHash: "",
  }
  let visitor <- Test.open (Orders.Index.Index {})
  let _ <- Test.login { user: Just me }
  let _ <- Test.get (Orders.Index.Index {})
  let out <- Test.click App.Logout
  Task.succeed (
    {
      visitor: visitor.answer,
      out: out.answer,
      user: out.session.user,
    } == {
      visitor: Redirected (Login {}),
      out: Redirected (Login {}),
      user: Nothing,
    }
  )
}

--- file: app/Orders/Index.alder ---
module Orders.Index

import Layout
import Line
import Order
import Product
import User

route Index  = GET  "/"       { } requires user
route Create = POST "/orders" { } requires user

type Entry = { order: Order, total: Int }

type Model = { orders: List Entry, manager: Bool }

Index.load : Index.Params -> Session -> Query Model
Index.load params session = {
  let orders <-
    Db.from Order
      |> Db.where (Order.owner.eq session.user)
      |> Db.orderBy Order.created.desc
      |> Db.all
  let products <- Db.from Product |> Db.all
  let lines <- Query.all (List.map (\o -> Line.linesOf o.id) orders)
  let manager <- User.isManager session.user
  Query.succeed {
    orders: List.map2 (\o ls -> { order: o, total: Line.total products ls }) orders lines,
    manager: manager,
  }
}

Create.update : Create.Params -> Session -> Task Response
Create.update p session = {
  let id <- Db.insert Order { owner: session.user }
  Task.succeed (Response.redirect (Orders.Show.Show { id: id }))
}

Index.view : Model -> Session -> Html
Index.view model session =
  <Layout.Page title="Orders">
    <h1>Orders</h1>
    <al-button click={Create}>New order</al-button>
    <ul id="orders">
      {for e in model.orders -> <OrderItem entry={e} />}
    </ul>
    {if model.manager then <a href={Products.Index.Index {}}>Products</a> else Html.none}
    <al-button variant="outline" click={App.Logout}>Log out</al-button>
  </Layout.Page>

OrderItem : { entry: Entry } -> Html
OrderItem props =
  <li>
    <a href={Orders.Show.Show { id: props.entry.order.id }}>
      Order of {Time.format "2006-01-02 15:04" props.entry.order.created}
    </a>
    <span class="status">{Order.statusLabel props.entry.order.status}</span>
    <span class="total">{Text.fromInt props.entry.total}</span>
  </li>

test "the list shows each order's status and total, newest first" = {
  let me <- Db.insert User {
    email: Text.Email "me@example.com",
    name: Text.NonEmpty "Me",
    passwordHash: "",
  }
  let tea <- Db.insert Product { name: Text.NonEmpty "Tea", price: Int.NonNegative 250 }
  let first <- Db.insert Order { owner: me }
  let _ <- Db.insert Line {
    order: first,
    product: tea,
    quantity: Int.Positive 2,
  }
  let _ <- Db.insert Order { owner: me, status: Paid }
  let _ <- Test.login { user: Just me }
  let page <- Test.get (Index {})
  Task.succeed (
    { statuses: Test.texts ".status" page.doc, totals: Test.texts ".total" page.doc }
      == { statuses: ["Paid", "Open"], totals: ["0", "500"] }
  )
}

test "the button creates an empty Open order" = {
  let me <- Db.insert User {
    email: Text.Email "me@example.com",
    name: Text.NonEmpty "Me",
    passwordHash: "",
  }
  let _ <- Test.login { user: Just me }
  let _ <- Test.get (Index {})
  let r <- Test.click (Create {})
  let orders <-
    Db.from Order
      |> Db.where (Order.owner.eq me)
      |> Db.all
  Task.succeed (
    { status: Test.text "#status" r.doc, orders: List.map (\o -> o.status) orders }
      == { status: Just "Open", orders: [Open] }
  )
}

--- file: app/Orders/Show.alder ---
module Orders.Show

import Layout
import Line
import Order
import Product
import User

route Show    = GET  "/orders/{id}"           { id: Order.OrderId }                                       requires user
route AddLine = POST "/orders/{id}/lines"     { id: Order.OrderId, product: Product.ProductId } form NewLine requires user
route Pay     = PUT  "/orders/{id}/paid"      { id: Order.OrderId }                                       requires user
route Cancel  = PUT  "/orders/{id}/cancelled" { id: Order.OrderId }                                       requires user

form NewLine = { quantity: Int.Positive }

type Row = { line: Line, product: Maybe Product, total: Int }

type Model = {
  order: Order,
  rows: List Row,
  products: List Product,
  total: Int,
}

-- The order, when it is the user's own or the user is a Manager; otherwise a 404. The write
-- routes run no `load`, so each of them begins with this too.
visible : User.UserId -> Order.OrderId -> Query Order
visible user id = {
  let found <- Db.get Order id
  let manager <- User.isManager user
  match found {
    Just order -> if order.owner == user || manager then Query.succeed order else Query.notFound,
    Nothing -> Query.notFound,
  }
}

Show.load : Show.Params -> Session -> Query Model
Show.load params session = {
  let order <- visible session.user params.id
  let lines <- Line.linesOf order.id
  let products <- Db.from Product |> Db.all
  Query.succeed {
    order: order,
    rows: List.map (\l -> {
      line: l,
      product: List.find (\x -> x.id == l.product) products,
      total: Line.lineTotal products l,
    }) lines,
    products: List.filter (\x -> x.active) products,
    total: Line.total products lines,
  }
}

-- Refused unless the order is still Open, whatever page the form came from.
AddLine.update : AddLine.Params -> Session -> Task Response
AddLine.update p session = {
  let order <- visible session.user p.id
  let product <- Db.get Product p.product
  let active = Maybe.map (\x -> x.active) product == Just True
  if order.status != Open then
    Task.succeed (
      Response.invalid "This order is no longer open."
        |> Response.error "This order is no longer open."
    )
  else if not active then
    Task.notFound
  else {
    let _ <- Db.insert Line {
      order: order.id,
      product: p.product,
      quantity: p.form.quantity,
    }
    Task.succeed (Response.redirect (Show { id: order.id }))
  }
}

setStatus : User.UserId -> Order.OrderId -> Order.Status -> Task Response
setStatus user id status = {
  let order <- visible user id
  let _ <-
    if order.status == Open then Db.update Order order.id (\o -> { o | status: status })
    else Task.succeed ()
  Task.succeed (Response.redirect (Show { id: order.id }))
}

Pay.update : Pay.Params -> Session -> Task Response
Pay.update p session = setStatus session.user p.id Paid

Cancel.update : Cancel.Params -> Session -> Task Response
Cancel.update p session = setStatus session.user p.id Cancelled

Show.view : Model -> Session -> Html
Show.view model session =
  <Layout.Page title="Order">
    <h1>Order</h1>
    <p>Status: <span id="status">{Order.statusLabel model.order.status}</span></p>
    <ul id="lines">
      {for r in model.rows ->
        <li>
          <span class="product">
            {match r.product {
              Just x -> Html.text x.name,
              Nothing -> Html.none,
            }}
          </span>
          {" × "}
          <span class="quantity">{Text.fromInt r.line.quantity}</span>
          {" = "}
          <span class="line-total">{Text.fromInt r.total}</span>
        </li>}
    </ul>
    <p>Total: <span id="total">{Text.fromInt model.total}</span></p>
    {if model.order.status == Open then
      <section id="actions">
        <ul id="add">
          {for x in model.products ->
            <li>
              <form submit={AddLine { id: model.order.id, product: x.id }}>
                <Errors form />
                <span class="name">{x.name}</span>
                <al-input field="quantity" label="Quantity" />
                <al-button type="submit">Add</al-button>
              </form>
            </li>}
        </ul>
        <al-button click={Pay { id: model.order.id }}>Mark paid</al-button>
        <al-button variant="destructive" click={Cancel { id: model.order.id }}>Cancel</al-button>
      </section>
    else Html.none}
    <al-button variant="ghost" href={Orders.Index.Index {}}>Back</al-button>
  </Layout.Page>

test "an Open order's total over two lines" = {
  let me <- Db.insert User {
    email: Text.Email "me@example.com",
    name: Text.NonEmpty "Me",
    passwordHash: "",
  }
  let tea <- Db.insert Product { name: Text.NonEmpty "Tea", price: Int.NonNegative 250 }
  let bun <- Db.insert Product { name: Text.NonEmpty "Bun", price: Int.NonNegative 100 }
  let o <- Db.insert Order { owner: me }
  let _ <- Db.insert Line {
    order: o,
    product: tea,
    quantity: Int.Positive 2,
  }
  let _ <- Db.insert Line {
    order: o,
    product: bun,
    quantity: Int.Positive 3,
  }
  let _ <- Test.login { user: Just me }
  let page <- Test.get (Show { id: o })
  Task.succeed (
    { status: Test.text "#status" page.doc, total: Test.text "#total" page.doc }
      == { status: Just "Open", total: Just "800" }
  )
}

test "adding a line to a Paid order is refused" = {
  let me <- Db.insert User {
    email: Text.Email "me@example.com",
    name: Text.NonEmpty "Me",
    passwordHash: "",
  }
  let tea <- Db.insert Product { name: Text.NonEmpty "Tea", price: Int.NonNegative 250 }
  let o <- Db.insert Order { owner: me }
  let _ <- Test.login { user: Just me }
  let _ <- Test.get (Show { id: o })
  let _ <- Db.update Order o (\x -> { x | status: Paid })
  let r <- Test.submit (AddLine { id: o, product: tea }) { quantity: Int.Positive 1 }
  let lines <-
    Db.from Line
      |> Db.where (Line.order.eq o)
      |> Db.count
  Task.succeed (
    { answer: r.answer, status: r.status, lines: lines }
      == { answer: FormRejected [], status: 422, lines: 0 }
  )
}

test "adding a line to an Open order" = {
  let me <- Db.insert User {
    email: Text.Email "me@example.com",
    name: Text.NonEmpty "Me",
    passwordHash: "",
  }
  let tea <- Db.insert Product { name: Text.NonEmpty "Tea", price: Int.NonNegative 250 }
  let o <- Db.insert Order { owner: me }
  let _ <- Test.login { user: Just me }
  let _ <- Test.get (Show { id: o })
  let r <- Test.submit (AddLine { id: o, product: tea }) { quantity: Int.Positive 2 }
  Task.succeed (
    { answer: r.answer, total: Test.text "#total" r.doc }
      == { answer: Redirected (Show { id: o }), total: Just "500" }
  )
}

test "paying an order hides its forms and buttons" = {
  let me <- Db.insert User {
    email: Text.Email "me@example.com",
    name: Text.NonEmpty "Me",
    passwordHash: "",
  }
  let _ <- Db.insert Product { name: Text.NonEmpty "Tea", price: Int.NonNegative 250 }
  let o <- Db.insert Order { owner: me }
  let _ <- Test.login { user: Just me }
  let _ <- Test.get (Show { id: o })
  let r <- Test.click (Pay { id: o })
  Task.succeed (
    {
      answer: r.answer,
      status: Test.text "#status" r.doc,
      forms: Test.count "form" r.doc,
    } == {
      answer: Redirected (Show { id: o }),
      status: Just "Paid",
      forms: 0,
    }
  )
}

test "a Staff user opening another user's order is not found" = {
  let owner <- Db.insert User {
    email: Text.Email "owner@example.com",
    name: Text.NonEmpty "Owner",
    passwordHash: "",
  }
  let staff <- Db.insert User {
    email: Text.Email "staff@example.com",
    name: Text.NonEmpty "Staff",
    passwordHash: "",
  }
  let o <- Db.insert Order { owner: owner }
  let _ <- Test.login { user: Just staff }
  let page <- Test.open (Show { id: o })
  Task.succeed (page.answer == PageMissing)
}

test "a Manager opening another user's order sees it" = {
  let owner <- Db.insert User {
    email: Text.Email "owner@example.com",
    name: Text.NonEmpty "Owner",
    passwordHash: "",
  }
  let boss <- Db.insert User {
    email: Text.Email "boss@example.com",
    name: Text.NonEmpty "Boss",
    passwordHash: "",
    role: Manager,
  }
  let o <- Db.insert Order { owner: owner }
  let _ <- Test.login { user: Just boss }
  let page <- Test.open (Show { id: o })
  Task.succeed (
    { answer: page.answer, status: Test.text "#status" page.doc }
      == { answer: Shown, status: Just "Open" }
  )
}

test "a Staff user cannot pay another user's order" = {
  let owner <- Db.insert User {
    email: Text.Email "owner@example.com",
    name: Text.NonEmpty "Owner",
    passwordHash: "",
  }
  let staff <- Db.insert User {
    email: Text.Email "staff@example.com",
    name: Text.NonEmpty "Staff",
    passwordHash: "",
  }
  let o <- Db.insert Order { owner: owner }
  let _ <- Test.login { user: Just owner }
  let _ <- Test.get (Show { id: o })
  let _ <- Test.login { user: Just staff }
  let r <- Test.click (Pay { id: o })
  let after <- Db.get Order o
  Task.succeed (
    { answer: r.answer, status: Maybe.map (\x -> x.status) after }
      == { answer: PageMissing, status: Just Open }
  )
}

--- file: app/Products/Index.alder ---
module Products.Index

import Layout
import Product
import User

route Index  = GET   "/products"      { }                                  requires user
route Add    = POST  "/products"      { }                 form NewProduct requires user
route Toggle = PATCH "/products/{id}" { id: Product.ProductId }            requires user

form NewProduct = { name: Text.NonEmpty, price: Int.NonNegative }

type Model = { products: List Product }

Index.load : Index.Params -> Session -> Query Model
Index.load params session = {
  let _ <- User.managerOnly session.user
  let products <-
    Db.from Product
      |> Db.orderBy Product.name.asc
      |> Db.all
  Query.succeed { products: products }
}

Add.update : Add.Params -> Session -> Task Response
Add.update p session = {
  let _ <- User.managerOnly session.user
  let _ <- Db.insert Product { name: p.form.name, price: p.form.price }
  Task.succeed (Response.redirect (Index {}))
}

Toggle.update : Toggle.Params -> Session -> Task Response
Toggle.update p session = {
  let _ <- User.managerOnly session.user
  let found <- Db.get Product p.id
  match found {
    Just x -> {
      let _ <- Db.update Product x.id (\y -> { y | active: not y.active })
      Task.succeed (Response.redirect (Index {}))
    },
    Nothing -> Task.notFound,
  }
}

Index.view : Model -> Session -> Html
Index.view model session =
  <Layout.Page title="Products">
    <h1>Products</h1>
    <al-card>
      <form submit={Add}>
        <Errors form />
        <al-input field="name" label="Name" />
        <al-input field="price" label="Price (cents)" />
        <al-button type="submit">Add product</al-button>
      </form>
    </al-card>
    <ul id="products">
      {for x in model.products ->
        <li>
          <span class="name">{x.name}</span>
          {" "}
          <span class="price">{Text.fromInt x.price}</span>
          {" "}
          <span class="active">{if x.active then "Active" else "Inactive"}</span>
          <al-button variant="secondary" size="sm" click={Toggle { id: x.id }}>
            {if x.active then "Deactivate" else "Activate"}
          </al-button>
        </li>}
    </ul>
    <al-button variant="ghost" href={Orders.Index.Index {}}>Orders</al-button>
  </Layout.Page>

test "a Staff user gets 404 from the products page" = {
  let staff <- Db.insert User {
    email: Text.Email "staff@example.com",
    name: Text.NonEmpty "Staff",
    passwordHash: "",
  }
  let _ <- Test.login { user: Just staff }
  let page <- Test.open (Index {})
  Task.succeed (page.answer == PageMissing)
}

test "a Manager adds a product and toggles it" = {
  let boss <- Db.insert User {
    email: Text.Email "boss@example.com",
    name: Text.NonEmpty "Boss",
    passwordHash: "",
    role: Manager,
  }
  let _ <- Test.login { user: Just boss }
  let _ <- Test.get (Index {})
  let added <- Test.submit (Add {}) { name: Text.NonEmpty "Tea", price: Int.NonNegative 250 }
  let tea <- Db.from Product |> Db.first
  let toggled <- match tea {
    Just x -> Test.click (Toggle { id: x.id })
    Nothing -> Test.refresh,
  }
  let after <- Db.from Product |> Db.all
  Task.succeed (
    {
      answer: added.answer,
      names: Test.texts ".name" added.doc,
      active: List.map (\x -> x.active) after,
    } == {
      answer: Redirected (Index {}),
      names: ["Tea"],
      active: [False],
    }
  )
}

test "a Staff user cannot toggle a product" = {
  let boss <- Db.insert User {
    email: Text.Email "boss@example.com",
    name: Text.NonEmpty "Boss",
    passwordHash: "",
    role: Manager,
  }
  let staff <- Db.insert User {
    email: Text.Email "staff@example.com",
    name: Text.NonEmpty "Staff",
    passwordHash: "",
  }
  let tea <- Db.insert Product { name: Text.NonEmpty "Tea", price: Int.NonNegative 250 }
  let _ <- Test.login { user: Just boss }
  let _ <- Test.get (Index {})
  let _ <- Test.login { user: Just staff }
  let r <- Test.click (Toggle { id: tea })
  let after <- Db.get Product tea
  Task.succeed (
    { answer: r.answer, active: Maybe.map (\x -> x.active) after }
      == { answer: PageMissing, active: Just True }
  )
}
