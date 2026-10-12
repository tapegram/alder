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
  <Layout.Page title="Error" role={Nothing}>
    <h1>Something went wrong</h1>
    <p>Please try again.</p>
  </Layout.Page>

notFound : Html Never
notFound =
  <Layout.Page title="Not found" role={Nothing}>
    <h1>Not found</h1>
  </Layout.Page>
```

--- file: app/Layout.alder ---
```alder
module Layout

import User

Page : { title: Text, role: Maybe User.Role, children: List (Html a) } -> Html a
Page props =
  <html>
    <head>
      <meta charset="utf-8" />
      <title>{props.title}</title>
      <link rel="stylesheet" href={Url.external "/static/app.css"} />
    </head>
    <body>
      {match props.role {
        Nothing -> Html.none,
        Just role -> <Menu role={role} />,
      }}
      <main>{props.children}</main>
    </body>
  </html>

Menu : { role: User.Role } -> Html a
Menu props =
  <nav>
    <a href={Orders.Index {}}>Orders</a>
    {if props.role == Manager then <a href={Products.Index {}}>Products</a> else Html.none}
    <button click={App.Logout}>Log out</button>
  </nav>
```

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
statusLabel s = match s {
  Open -> "Open",
  Paid -> "Paid",
  Cancelled -> "Cancelled",
}
```

--- file: app/Line.alder ---
```alder
module Line

import User
import Order
import Product

type LineId = brand Text

table Line {
  id: LineId @id,
  order: Order.OrderId @ref Order @cascade,
  product: Product.ProductId @ref Product,
  quantity: Int.Positive,
}

type LineRow = { name: Text, quantity: Int, cost: Int }

-- rowOf is what a line shows: its product's name, its quantity, and its cost in cents.
rowOf : Line -> Query LineRow
rowOf line = {
  let found <- Db.get Product line.product
  match found {
    Just p -> Query.succeed { name: p.name, quantity: line.quantity, cost: line.quantity * p.price },
    Nothing -> Query.succeed { name: "(removed)", quantity: line.quantity, cost: 0 },
  }
}

-- rowsOf is the lines of one order.
rowsOf : Order.OrderId -> Query (List LineRow)
rowsOf orderId = {
  let lines <- Db.from Line |> Db.where (Line.order.eq orderId) |> Db.all
  Query.all (List.map rowOf lines)
}

-- orderTotal is the sum of an order's lines in cents, whatever the order's status.
orderTotal : Order.OrderId -> Query Int
orderTotal orderId = {
  let rows <- rowsOf orderId
  Query.succeed (List.sum (List.map (\r -> r.cost) rows))
}

test "an open order's total is the sum of its lines" = {
  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
  let order <- Db.insert Order { owner: me }
  let tea <- Db.insert Product { name: Text.NonEmpty "Tea", price: Int.NonNegative 250 }
  let cake <- Db.insert Product { name: Text.NonEmpty "Cake", price: Int.NonNegative 400 }
  let _ <- Db.insert Line { order: order, product: tea, quantity: Int.Positive 2 }
  let _ <- Db.insert Line { order: order, product: cake, quantity: Int.Positive 1 }
  let total <- orderTotal order
  Task.succeed (total == 900)
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
  <Layout.Page title="Log in" role={Nothing}>
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

--- file: app/Orders/Index.alder ---
```alder
module Orders.Index

import Order
import Line
import User
import Layout

page "/" { } requires user

type Summary = { id: Order.OrderId, status: Order.Status, total: Int }

type Model = { orders: List Summary, role: User.Role }

load : Params -> Session -> Query Model
load params session = {
  let orders <-
    Db.from Order
      |> Db.where (Order.owner.eq session.user)
      |> Db.orderBy Order.created.desc
      |> Db.all
  let summaries <- Query.all (List.map summarize orders)
  let me <- Db.get User session.user
  match me {
    Just u -> Query.succeed { orders: summaries, role: u.role },
    Nothing -> Query.notFound,
  }
}

summarize : Order -> Query Summary
summarize order = {
  let total <- Line.orderTotal order.id
  Query.succeed { id: order.id, status: order.status, total: total }
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
  <Layout.Page title="Orders" role={Just model.role}>
    <h1>Orders</h1>
    <button click={Create}>New order</button>
    <ul id="orders">
      {for o in model.orders ->
        <li>
          <a href={Orders.Show { id: o.id }}>View</a>
          <span class="status">{Order.statusLabel o.status}</span>
          <span class="total">{Text.fromInt o.total}</span>
        </li>}
    </ul>
  </Layout.Page>
```

--- file: app/Orders/Show.alder ---
```alder
module Orders.Show

import Order
import Line
import Product
import User
import Layout

page "/orders/{id}" { id: Order.OrderId } requires user

type Model = {
  order: Order,
  lines: List Line.LineRow,
  total: Int,
  products: List Product,
  role: User.Role,
}

form NewLine = {
  quantity: Int.Positive,
}

msg AddLine { product: Product.ProductId } form NewLine
msg Pay
msg Cancel

-- A user may see an order that is theirs, and a Manager may see any order.
mayOpen : Order -> User -> Bool
mayOpen order u = order.owner == u.id || u.role == Manager

load : Params -> Session -> Query Model
load params session = {
  let found <- Db.get Order params.id
  let me <- Db.get User session.user
  match found {
    Nothing -> Query.notFound,
    Just order -> openFor order me,
  }
}

-- openFor is the order's page for the user, or a 404 when the user may not see it.
openFor : Order -> Maybe User -> Query Model
openFor order me = match me {
  Nothing -> Query.notFound,
  Just u -> if mayOpen order u then openPage order u else Query.notFound,
}

openPage : Order -> User -> Query Model
openPage order u = {
  let lines <- Line.rowsOf order.id
  let total <- Line.orderTotal order.id
  let products <- Db.from Product |> Db.where (Product.active.eq True) |> Db.orderBy Product.name.asc |> Db.all
  Query.succeed { order: order, lines: lines, total: total, products: products, role: u.role }
}

-- openOrder is the order as it is now, for a change by the signed-in user. The page may have
-- been loaded while the order was still Open, so the status is read again here.
openOrder : Order.OrderId -> Session -> Task Order
openOrder id session = {
  let found <- Db.get Order id
  let me <- Db.get User session.user
  match found {
    Nothing -> Task.notFound,
    Just order -> changeable order me,
  }
}

-- A refused change to an order that is no longer Open is Task.fail, not Task.notFound: the order
-- exists and the user may see it, so a 404 would be a false answer. The failure rolls back.
changeable : Order -> Maybe User -> Task Order
changeable order me = match me {
  Nothing -> Task.notFound,
  Just u -> if not (mayOpen order u) then Task.notFound else if order.status == Open then Task.succeed order else Task.fail "the order is not open",
}

activeProduct : Product.ProductId -> Task Product
activeProduct id = {
  let found <- Db.get Product id
  match found {
    Nothing -> Task.notFound,
    Just product -> if product.active then Task.succeed product else Task.fail "the product is not active",
  }
}

update : Msg -> Model -> Session -> Task Nav
update msg model session = match msg {
  AddLine p -> {
    let order <- openOrder model.order.id session
    let product <- activeProduct p.product
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

view : Model -> Session -> Html Msg
view model session =
  <Layout.Page title="Order" role={Just model.role}>
    <h1>Order</h1>
    <p>Status: <span id="status">{Order.statusLabel model.order.status}</span></p>
    <p>Total: <span id="total">{Text.fromInt model.total}</span></p>
    <ul id="lines">
      {for l in model.lines -> <li>{l.name} x {Text.fromInt l.quantity}, {Text.fromInt l.cost}</li>}
    </ul>
    {if model.order.status == Open then <Adding products={model.products} /> else Html.none}
  </Layout.Page>

Adding : { products: List Product } -> Html Msg
Adding props =
  <div>
    <h2>Add a line</h2>
    {for p in props.products ->
      <form submit={AddLine { product: p.id }}>
        <Errors form />
        <span>{p.name} at {Text.fromInt p.price} each</span>
        <Input field="quantity" label="Quantity" />
        <Errors field="quantity" />
        <button>Add</button>
      </form>}
    <button click={Pay}>Mark paid</button>
    <button click={Cancel}>Cancel order</button>
  </div>

test "adding a line to a paid order is refused" = {
  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
  let order <- Db.insert Order { owner: me }
  let tea <- Db.insert Product { name: Text.NonEmpty "Tea", price: Int.NonNegative 250 }
  let _ <- Test.login { user: Just me }
  let _ <- Test.get (Orders.Show { id: order })
  let _ <- Db.update Order order (\o -> { o | status: Paid })
  let r <- Test.submit (AddLine { product: tea }) { quantity: Int.Positive 1 }
  let lines <- Db.from Line |> Db.count
  Task.succeed ({ answer: r.answer, lines: lines } == { answer: RequestFailed "the order is not open", lines: 0 })
}

test "a staff user cannot open another user's order" = {
  let owner <- Db.insert User { email: Text.Email "owner@example.com", name: Text.NonEmpty "Owner", passwordHash: "" }
  let staff <- Db.insert User { email: Text.Email "staff@example.com", name: Text.NonEmpty "Staff", passwordHash: "" }
  let order <- Db.insert Order { owner: owner }
  let _ <- Test.login { user: Just staff }
  let r <- Test.open (Orders.Show { id: order })
  Task.succeed ({ answer: r.answer } == { answer: PageMissing })
}

test "a manager opens another user's order" = {
  let owner <- Db.insert User { email: Text.Email "owner@example.com", name: Text.NonEmpty "Owner", passwordHash: "" }
  let boss <- Db.insert User { email: Text.Email "boss@example.com", name: Text.NonEmpty "Boss", passwordHash: "", role: Manager }
  let order <- Db.insert Order { owner: owner }
  let _ <- Test.login { user: Just boss }
  let page <- Test.get (Orders.Show { id: order })
  Task.succeed ({ status: Test.text "#status" page.doc, total: Test.text "#total" page.doc } == { status: Just "Open", total: Just "0" })
}
```

--- file: app/Products/Index.alder ---
```alder
module Products.Index

import Product
import User
import Layout

page "/products" { } requires user

type Model = { products: List Product, role: User.Role }

load : Params -> Session -> Query Model
load params session = {
  let me <- Db.get User session.user
  match me {
    Nothing -> Query.notFound,
    Just u -> if u.role == Manager then listing u.role else Query.notFound,
  }
}

listing : User.Role -> Query Model
listing role = {
  let products <- Db.from Product |> Db.orderBy Product.name.asc |> Db.all
  Query.succeed { products: products, role: role }
}

form NewProduct = {
  name: Text.NonEmpty,
  price: Int.NonNegative,
}

msg Add form NewProduct
msg Toggle { id: Product.ProductId }

-- managerOnly fails with a 404 unless the signed-in user is a Manager; update checks it too.
managerOnly : Session -> Task Unit
managerOnly session = {
  let me <- Db.get User session.user
  match me {
    Nothing -> Task.notFound,
    Just u -> if u.role == Manager then Task.succeed () else Task.notFound,
  }
}

update : Msg -> Model -> Session -> Task Nav
update msg model session = match msg {
  Add p -> {
    let _ <- managerOnly session
    let _ <- Db.insert Product { name: p.form.name, price: p.form.price }
    Task.succeed (Redirect (Products.Index {}))
  },
  Toggle p -> {
    let _ <- managerOnly session
    let found <- Db.get Product p.id
    match found {
      Nothing -> Task.notFound,
      Just product -> {
        let _ <- Db.update Product product.id (\x -> { x | active: not x.active })
        Task.succeed (Redirect (Products.Index {}))
      },
    }
  },
}

view : Model -> Session -> Html Msg
view model session =
  <Layout.Page title="Products" role={Just model.role}>
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
      {for p in model.products -> <Row product={p} />}
    </ul>
  </Layout.Page>

Row : { product: Product } -> Html Msg
Row props =
  <li>
    <span class="name">{props.product.name}</span>
    <span class="price">{Text.fromInt props.product.price}</span>
    <span class="state">{if props.product.active then "active" else "inactive"}</span>
    <button click={Toggle { id: props.product.id }}>{if props.product.active then "Deactivate" else "Activate"}</button>
  </li>

test "a staff user cannot open the products page" = {
  let staff <- Db.insert User { email: Text.Email "staff@example.com", name: Text.NonEmpty "Staff", passwordHash: "" }
  let _ <- Test.login { user: Just staff }
  let r <- Test.open (Products.Index {})
  Task.succeed ({ answer: r.answer } == { answer: PageMissing })
}
```
