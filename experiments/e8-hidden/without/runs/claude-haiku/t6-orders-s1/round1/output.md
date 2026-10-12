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

-- amount is what one line costs in cents: its quantity times its product's price. A line whose
-- product is not in the list costs 0.
amount : List Product -> Line -> Int
amount products line = {
  let found = List.find (\p -> p.id == line.product) products
  match found {
    Just p -> line.quantity * p.price,
    Nothing -> 0,
  }
}

-- total is the sum of the amounts of the lines, in cents.
total : List Product -> List Line -> Int
total products lines = List.sum (List.map (amount products) lines)

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
import Layout

page "/" { } requires user

type OrderRow = { id: Order.OrderId, status: Order.Status, total: Int }

type Model = { orders: List OrderRow }

load : Params -> Session -> Query Model
load params session = {
  let orders <- Db.from Order |> Db.where (Order.owner.eq session.user) |> Db.orderBy Order.created.desc |> Db.all
  let ids = List.map (\o -> o.id) orders
  let lines <- Db.from Line |> Db.where (Line.order.in ids) |> Db.all
  let products <- Db.from Product |> Db.all
  Query.succeed { orders: List.map (orderRow products lines) orders }
}

orderRow : List Product -> List Line -> Order -> OrderRow
orderRow products lines order =
  { id: order.id, status: order.status, total: Line.total products (List.filter (\l -> l.order == order.id) lines) }

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
      {for o in model.orders -> <OrderItem order={o} />}
    </ul>
    <p><a href={Products.Index {}}>Products</a></p>
    <button click={App.Logout}>Log out</button>
  </Layout.Page>

OrderItem : { order: OrderRow } -> Html Msg
OrderItem props =
  <li>
    <a href={Orders.Show { id: props.order.id }}>View</a>
    <span class="status">{Order.statusLabel props.order.status}</span>
    <span class="total">{Text.fromInt props.order.total}</span>
  </li>

--- file: app/Orders/Show.alder ---
module Orders.Show

import Order
import Product
import Line
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

-- Only the order's owner, or a Manager, may see it; anyone else gets a 404.
load : Params -> Session -> Query Model
load params session = {
  let found <- Db.get Order params.id
  let me <- Db.get User session.user
  let products <- Db.from Product |> Db.orderBy Product.name.asc |> Db.all
  let lines <- Db.from Line |> Db.where (Line.order.eq params.id) |> Db.all
  match found {
    Just order -> if order.owner == session.user || User.isManager me then Query.succeed { order: order, lines: List.map (lineRow products) lines, products: products, total: Line.total products lines } else Query.notFound,
    Nothing -> Query.notFound,
  }
}

lineRow : List Product -> Line -> LineRow
lineRow products line =
  { name: productName products line.product, quantity: line.quantity, amount: Line.amount products line }

productName : List Product -> Product.ProductId -> Text
productName products id = {
  let found = List.find (\p -> p.id == id) products
  match found {
    Just p -> p.name,
    Nothing -> "(removed)",
  }
}

isOpenOrder : Maybe Order -> Bool
isOpenOrder found = match found {
  Just o -> o.status == Open,
  Nothing -> False,
}

isActive : Maybe Product -> Bool
isActive found = match found {
  Just p -> p.active,
  Nothing -> False,
}

refusal : Maybe Order -> Text
refusal found =
  if isOpenOrder found then "That product is not available." else "This order is no longer open."

form NewLine = {
  quantity: Int.Positive,
}

msg AddLine { product: Product.ProductId } form NewLine
msg Pay
msg Cancel

-- Every change re-reads the order, so a refused change is refused on the current state.
update : Msg -> Model -> Session -> Task Nav
update msg model session = match msg {
  AddLine p -> {
    let found <- Db.get Order model.order.id
    let product <- Db.get Product p.product
    if isOpenOrder found && isActive product then {
      let _ <- Db.insert Line { order: model.order.id, product: p.product, quantity: p.form.quantity }
      Task.succeed (Redirect (Orders.Show { id: model.order.id }))
    } else Task.succeed (Reject (refusal found))
  },
  Pay p -> setStatus model.order.id Paid,
  Cancel p -> setStatus model.order.id Cancelled,
}

-- setStatus changes an Open order; any other order is left alone and the page is shown again.
setStatus : Order.OrderId -> Order.Status -> Task Nav
setStatus id status = {
  let found <- Db.get Order id
  if isOpenOrder found then {
    let _ <- Db.update Order id (\o -> { o | status: status })
    Task.succeed (Redirect (Orders.Show { id: id }))
  } else Task.succeed (Redirect (Orders.Show { id: id }))
}

view : Model -> Session -> Html Msg
view model session =
  <Layout.Page title="Order">
    <h1>Order</h1>
    <p>Status <span id="status">{Order.statusLabel model.order.status}</span></p>
    <ul id="lines">
      {for line in model.lines -> <LineItem line={line} />}
    </ul>
    <p>Total <span id="total">{Text.fromInt model.total}</span></p>
    {if model.order.status == Open then <AddForms products={model.products} /> else Html.none}
    {if model.order.status == Open then <div><button click={Pay}>Mark paid</button> <button click={Cancel}>Cancel order</button></div> else Html.none}
    <p><a href={Orders.Index {}}>Back to orders</a></p>
  </Layout.Page>

LineItem : { line: LineRow } -> Html Msg
LineItem props =
  <li>
    <span class="name">{props.line.name}</span>
    <span class="quantity">{Text.fromInt props.line.quantity}</span>
    <span class="amount">{Text.fromInt props.line.amount}</span>
  </li>

AddForms : { products: List Product } -> Html Msg
AddForms props =
  <div>
    <h2>Add a line</h2>
    {for p in (List.filter (\x -> x.active) props.products) -> <AddForm product={p} />}
  </div>

AddForm : { product: Product } -> Html Msg
AddForm props =
  <form submit={AddLine { product: props.product.id }}>
    <p>{props.product.name}</p>
    <Errors form />
    <Input field="quantity" label="Quantity" />
    <Errors field="quantity" />
    <button>Add</button>
  </form>

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

-- Only a Manager sees the page; a Staff user gets a 404.
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

update : Msg -> Model -> Session -> Task Nav
update msg model session = match msg {
  Add p -> {
    let _ <- Db.insert Product { name: p.form.name, price: p.form.price }
    Task.succeed (Redirect (Products.Index {}))
  },
  Toggle p -> {
    let found <- Db.get Product p.id
    match found {
      Just product -> {
        let _ <- Db.update Product product.id (\x -> { x | active: not x.active })
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
    <ul id="products">
      {for p in model.products -> <ProductItem product={p} />}
    </ul>
    <h2>Add a product</h2>
    <form submit={Add}>
      <Errors form />
      <Input field="name" label="Name" />
      <Errors field="name" />
      <Input field="price" label="Price in cents" />
      <Errors field="price" />
      <button>Add</button>
    </form>
    <p><a href={Orders.Index {}}>Orders</a></p>
  </Layout.Page>

ProductItem : { product: Product } -> Html Msg
ProductItem props =
  <li class={if props.product.active then "" else "inactive"}>
    <span class="name">{props.product.name}</span>
    <span class="price">{Text.fromInt props.product.price}</span>
    <button click={Toggle { id: props.product.id }}>{if props.product.active then "Deactivate" else "Activate"}</button>
  </li>

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
isManager found = match found {
  Just u -> u.role == Manager,
  Nothing -> False,
}
