```alder
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
```

```alder
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
```

```alder
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
```

```alder
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

-- isManager: only a manager may see every order and change the products. A visitor is not one.
isManager : Maybe User -> Bool
isManager me = match me {
  Just u -> u.role == Manager,
  Nothing -> False,
}
```

```alder
--- file: app/Product.alder ---
module Product

type ProductId = brand Text

table Product {
  id: ProductId @id,
  name: Text.NonEmpty,
  price: Int.NonNegative,
  active: Bool = True,
}

-- sellable: a product that exists and is active can be added to an order.
sellable : Maybe Product -> Bool
sellable found = match found {
  Just p -> p.active,
  Nothing -> False,
}

test "a product that is gone is not for sale" = not (sellable Nothing)
```

```alder
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
statusText s = match s {
  Open -> "Open",
  Paid -> "Paid",
  Cancelled -> "Cancelled",
}

-- canSee: an order is visible to its owner and to managers. A visitor with no user sees nothing.
canSee : Maybe User -> Order -> Bool
canSee me o = match me {
  Just u -> o.owner == u.id || u.role == Manager,
  Nothing -> False,
}
```

```alder
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

-- nameOf is the name of the product a line names, or a placeholder when that product is gone.
nameOf : List Product -> Line -> Text
nameOf products line = {
  let found = List.find (\p -> p.id == line.product) products
  match found {
    Just p -> p.name,
    Nothing -> "(removed product)",
  }
}

-- amount is one line's price in cents: its quantity times its product's price.
amount : List Product -> Line -> Int
amount products line = {
  let found = List.find (\p -> p.id == line.product) products
  match found {
    Just p -> line.quantity * p.price,
    Nothing -> 0,
  }
}

-- total is an order's price in cents: the sum over its lines.
total : List Product -> List Line -> Int
total products lines = List.sum (List.map (amount products) lines)

-- orderTotal reads an order's lines and products and gives the order's total.
orderTotal : Order -> Query Int
orderTotal o = {
  let lines <- Db.from Line |> Db.where (Line.order.eq o.id) |> Db.all
  let products <- Db.from Product |> Db.all
  Query.succeed (total products lines)
}
```

```alder
--- file: app/Orders/Index.alder ---
module Orders.Index

import Order
import Line
import Layout

page "/" { } requires user

type Summary = { order: Order, total: Int }

type Model = { summaries: List Summary }

load : Params -> Session -> Query Model
load params session = {
  let orders <-
    Db.from Order
      |> Db.where (Order.owner.eq session.user)
      |> Db.orderBy Order.created.desc
      |> Db.all
  let rows <- Query.all (List.map summarize orders)
  Query.succeed { summaries: rows }
}

summarize : Order -> Query Summary
summarize o = {
  let total <- Line.orderTotal o
  Query.succeed { order: o, total: total }
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
      {for s in model.summaries -> <OrderRow summary={s} />}
    </ul>
    <a href={Products.Index {}}>Products</a>
    <button click={App.Logout}>Log out</button>
  </Layout.Page>

OrderRow : { summary: Summary } -> Html Msg
OrderRow props =
  <li>
    <a href={Orders.Show { id: props.summary.order.id }}>View</a>
    <span class="status">{Order.statusText props.summary.order.status}</span>
    <span class="total">{Text.fromInt props.summary.total}</span>
  </li>
```

```alder
--- file: app/Orders/Show.alder ---
module Orders.Show

import Order
import Line
import Product
import User
import Layout

page "/orders/{id}" { id: Order.OrderId } requires user

type LineRow = { name: Text, quantity: Int, amount: Int }

type Model = {
  order: Order,
  lines: List LineRow,
  products: List Product,
  total: Int,
}

load : Params -> Session -> Query Model
load params session = {
  let found <- Db.get Order params.id
  let me <- Db.get User session.user
  match found {
    Just o -> if Order.canSee me o then modelFor o else Query.notFound,
    Nothing -> Query.notFound,
  }
}

-- modelFor reads an order's lines, and the products that the page needs.
modelFor : Order -> Query Model
modelFor o = {
  let lines <- Db.from Line |> Db.where (Line.order.eq o.id) |> Db.all
  let products <- Db.from Product |> Db.orderBy Product.name.asc |> Db.all
  Query.succeed {
    order: o,
    lines: List.map (lineRow products) lines,
    products: List.filter (\p -> p.active) products,
    total: Line.total products lines,
  }
}

lineRow : List Product -> Line -> LineRow
lineRow products line = { name: Line.nameOf products line, quantity: line.quantity, amount: Line.amount products line }

form NewLine = {
  quantity: Int.Positive,
}

msg AddLine { product: Product.ProductId } form NewLine
msg Pay
msg Cancel

-- visibleOrder is the order a message names, when the session's user may see it; otherwise a
-- 404. A message's id comes from the request, so `update` checks it again.
visibleOrder : Order.OrderId -> User.UserId -> Task Order
visibleOrder id userId = {
  let found <- Db.get Order id
  let me <- Db.get User userId
  match found {
    Just o -> if Order.canSee me o then Task.succeed o else Task.notFound,
    Nothing -> Task.notFound,
  }
}

-- moveIfOpen sets the status of an Open order; any other order keeps its status.
moveIfOpen : Order -> Order.Status -> Task Unit
moveIfOpen o status = match o.status {
  Open -> Db.update Order o.id (\x -> { x | status: status }),
  _ -> Task.succeed (),
}

update : Msg -> Model -> Session -> Task Nav
update msg model session = match msg {
  AddLine p -> {
    let o <- visibleOrder model.order.id session.user
    let product <- Db.get Product p.product
    let problem = if o.status != Open then Just "This order is no longer open." else if not (Product.sellable product) then Just "That product is not for sale." else Nothing
    match problem {
      Just why -> Task.succeed (Reject why),
      Nothing -> {
        let _ <- Db.insert Line { order: o.id, product: p.product, quantity: p.form.quantity }
        Task.succeed (Redirect (Orders.Show { id: o.id }))
      },
    }
  },
  Pay p -> {
    let o <- visibleOrder model.order.id session.user
    let _ <- moveIfOpen o Paid
    Task.succeed (Redirect (Orders.Show { id: o.id }))
  },
  Cancel p -> {
    let o <- visibleOrder model.order.id session.user
    let _ <- moveIfOpen o Cancelled
    Task.succeed (Redirect (Orders.Show { id: o.id }))
  },
}

view : Model -> Session -> Html Msg
view model session =
  <Layout.Page title="Order">
    <h1>Order</h1>
    <p>Status: <span id="status">{Order.statusText model.order.status}</span></p>
    <ul id="lines">
      {for l in model.lines -> <li>{l.name} x {Text.fromInt l.quantity}: {Text.fromInt l.amount}</li>}
    </ul>
    <p>Total: <span id="total">{Text.fromInt model.total}</span></p>
    {if model.order.status == Open then <Editing products={model.products} /> else Html.none}
    <a href={Orders.Index {}}>All orders</a>
  </Layout.Page>

Editing : { products: List Product } -> Html Msg
Editing props =
  <div>
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

test "an open order's total is over its lines" = {
  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "", role: Staff }
  let tea <- Db.insert Product { name: Text.NonEmpty "Tea", price: Int.NonNegative 250 }
  let mug <- Db.insert Product { name: Text.NonEmpty "Mug", price: Int.NonNegative 1000 }
  let oid <- Db.insert Order { owner: me }
  let _ <- Db.insert Line { order: oid, product: tea, quantity: Int.Positive 2 }
  let _ <- Db.insert Line { order: oid, product: mug, quantity: Int.Positive 1 }
  let _ <- Test.login { user: Just me }
  let page <- Test.get (Orders.Show { id: oid })
  Task.succeed ({ status: Test.text "#status" page.doc, total: Test.text "#total" page.doc } == { status: Just "Open", total: Just "1500" })
}

test "adding a line to a paid order is refused" = {
  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "", role: Staff }
  let tea <- Db.insert Product { name: Text.NonEmpty "Tea", price: Int.NonNegative 250 }
  let oid <- Db.insert Order { owner: me }
  let _ <- Test.login { user: Just me }
  let page <- Test.get (Orders.Show { id: oid })
  let _ <- Db.update Order oid (\o -> { o | status: Paid })
  let r <- Test.run (update (AddLine { product: tea, form: { quantity: Int.Positive 1 } }) page.model { user: me })
  let lines <- Db.from Line |> Db.count
  Task.succeed ({ value: r.value, lines: lines } == { value: Ok (Reject "This order is no longer open."), lines: 0 })
}

test "a staff member is not shown another user's order" = {
  let owner <- Db.insert User { email: Text.Email "owner@example.com", name: Text.NonEmpty "Owner", passwordHash: "", role: Staff }
  let other <- Db.insert User { email: Text.Email "other@example.com", name: Text.NonEmpty "Other", passwordHash: "", role: Staff }
  let oid <- Db.insert Order { owner: owner }
  let _ <- Test.login { user: Just other }
  let r <- Test.open (Orders.Show { id: oid })
  Task.succeed (r.answer == PageMissing)
}

test "a manager is shown another user's order" = {
  let owner <- Db.insert User { email: Text.Email "owner@example.com", name: Text.NonEmpty "Owner", passwordHash: "", role: Staff }
  let boss <- Db.insert User { email: Text.Email "boss@example.com", name: Text.NonEmpty "Boss", passwordHash: "", role: Manager }
  let oid <- Db.insert Order { owner: owner }
  let _ <- Test.login { user: Just boss }
  let page <- Test.get (Orders.Show { id: oid })
  Task.succeed (Test.text "#status" page.doc == Just "Open")
}
```

```alder
--- file: app/Products/Index.alder ---
module Products.Index

import Product
import User
import Layout

page "/products" { } requires user

type Model = { products: List Product }

load : Params -> Session -> Query Model
load params session = {
  let me <- Db.get User session.user
  let products <- Db.from Product |> Db.orderBy Product.name.asc |> Db.all
  if User.isManager me then Query.succeed { products: products } else Query.notFound
}

form NewProduct = {
  name: Text.NonEmpty,
  price: Int.NonNegative,
}

msg Add form NewProduct
msg Toggle { id: Product.ProductId }

-- asManager ends the request with a 404 unless the session's user is a manager.
asManager : User.UserId -> Task Unit
asManager userId = {
  let me <- Db.get User userId
  if User.isManager me then Task.succeed () else Task.notFound
}

update : Msg -> Model -> Session -> Task Nav
update msg model session = match msg {
  Add p -> {
    let _ <- asManager session.user
    let _ <- Db.insert Product { name: p.form.name, price: p.form.price }
    Task.succeed (Redirect (Products.Index {}))
  },
  Toggle p -> {
    let _ <- asManager session.user
    let found <- Db.get Product p.id
    match found {
      Just product -> {
        let _ <- Db.update Product product.id (\x -> { x | active: not product.active })
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
      <Input field="price" label="Price (cents)" />
      <Errors field="price" />
      <button>Add product</button>
    </form>
    <ul id="products">
      {for p in model.products -> <Item product={p} />}
    </ul>
    <a href={Orders.Index {}}>Orders</a>
  </Layout.Page>

Item : { product: Product } -> Html Msg
Item props =
  <li class={if props.product.active then "active" else "inactive"}>
    <span class="name">{props.product.name}</span>
    <span class="price">{Text.fromInt props.product.price}</span>
    <button click={Toggle { id: props.product.id }}>{if props.product.active then "Deactivate" else "Activate"}</button>
  </li>
```
