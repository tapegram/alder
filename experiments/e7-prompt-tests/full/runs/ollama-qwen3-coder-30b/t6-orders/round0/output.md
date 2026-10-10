--- file: app/App.alder ---
module App

import User
import Layout

session { user: Maybe User.UserId, role: User.Role = Staff }

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
  <Layout.Page title="Error" theme={Light}>
    <h1>Something went wrong</h1>
    <p>Please try again.</p>
  </Layout.Page>

notFound : Html Never
notFound =
  <Layout.Page title="Not found" theme={Light}>
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
        Task.succeed (Redirect (Shop.Index {}))
      },
      Nothing -> Task.succeed (Reject "Email or password is incorrect."),
    }
  },
}

view : Model -> Session -> Html Msg
view model session =
  <Layout.Page title="Log in" theme={Light}>
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

--- file: app/Shop/Index.alder ---
module Shop.Index

import User
import Product
import Order

page "/" { } requires user

type Model = { orders: List Order }

load : Params -> Session -> Query Model
load params session = {
  let orders <-
    Db.from Order
      |> Db.where (Order.owner.eq session.user)
      |> Db.orderBy Order.created.desc
      |> Db.all
  Query.succeed { orders: orders }
}

form NewOrder = { }

msg Submit form NewOrder
msg View { id: Order.OrderId }

update : Msg -> Model -> Session -> Task Nav
update msg model session = match msg {
  Submit p -> {
    let _ <- Db.insert Order { owner: session.user, status: Open, created: Time.epoch }
    Task.succeed Rerender
  },
  View p -> {
    Task.succeed (Redirect (Shop.Show { id: p.id }))
  },
}

view : Model -> Session -> Html Msg
view model session =
  <Layout.Page title="Orders">
    <h1>Orders</h1>
    <form submit={Submit}>
      <button>Add Order</button>
    </form>
    <ul>
      {for order in model.orders ->
        <li>
          <a href={Shop.Show { id: order.id }}>{order.status} - {Order.total order}</a>
        </li>}
    </ul>
  </Layout.Page>

--- file: app/Shop/Show.alder ---
module Shop.Show

import User
import Product
import Order

page "/orders/{id}" { id: Order.OrderId } requires user

type Model = { order: Order }

load : Params -> Session -> Query Model
load params session = {
  let found <- Db.get Order params.id
  match found {
    Just order ->
      if order.owner == session.user || session.role == Manager then Query.succeed { order: order }
      else Query.notFound,
    Nothing -> Query.notFound,
  }
}

form NewLine = {
  product: Product.ProductId,
  quantity: Int.Positive,
}

msg AddLine form NewLine
msg MarkPaid
msg MarkCancelled

update : Msg -> Model -> Session -> Task Nav
update msg model session = match msg {
  AddLine p -> {
    let order <- Db.get Order model.order.id
    match order {
      Just o ->
        if o.status == Open then {
          let product <- Db.get Product p.form.product
          match product {
            Just prod -> {
              let _ <- Db.insert OrderLine { orderId: model.order.id, productId: p.form.product, quantity: p.form.quantity }
              Task.succeed Rerender
            },
            Nothing -> Task.fail "Product not found",
          }
        } else Task.fail "Order is not open",
      Nothing -> Task.notFound,
    }
  },
  MarkPaid p -> {
    let order <- Db.get Order model.order.id
    match order {
      Just o ->
        if o.status == Open then {
          let _ <- Db.update Order model.order.id (\t -> { t | status: Paid })
          Task.succeed Rerender
        } else Task.fail "Order is not open",
      Nothing -> Task.notFound,
    }
  },
  MarkCancelled p -> {
    let order <- Db.get Order model.order.id
    match order {
      Just o ->
        if o.status == Open then {
          let _ <- Db.update Order model.order.id (\t -> { t | status: Cancelled })
          Task.succeed Rerender
        } else Task.fail "Order is not open",
      Nothing -> Task.notFound,
    }
  },
}

view : Model -> Session -> Html Msg
view model session =
  <Layout.Page title="Order">
    <h1>Order {model.order.id}</h1>
    <p>Status: {model.order.status}</p>
    <p>Total: {Order.total model.order}</p>
    {if model.order.status == Open ->
      <form submit={AddLine}>
        <Input field="product" label="Product" />
        <Errors field="product" />
        <Input field="quantity" label="Quantity" />
        <Errors field="quantity" />
        <button>Add Line</button>
      </form>
    }
    {if model.order.status == Open ->
      <div>
        <button click={MarkPaid}>Mark Paid</button>
        <button click={MarkCancelled}>Cancel</button>
      </div>
    }
    <ul>
      {for line in Order.lines model.order ->
        <li>{Product.name line.productId} x {line.quantity}</li>}
    </ul>
  </Layout.Page>

--- file: app/Order.alder ---
module Order

import User
import Product

type OrderId = brand Text

type Status = Open | Paid | Cancelled

table Order {
  id: OrderId @id,
  owner: User.UserId @ref User,
  status: Status,
  created: Time @created,
}

table OrderLine {
  id: Text @id,
  orderId: OrderId @ref Order,
  productId: Product.ProductId @ref Product,
  quantity: Int.Positive,
}

total : Order -> Int
total order = {
  let lines <- Db.from OrderLine |> Db.where (OrderLine.orderId.eq order.id) |> Db.all
  let sum = List.sum (List.map (\l -> Product.price l.productId * l.quantity) lines)
  sum
}

lines : Order -> List OrderLine
lines order = {
  let lines <- Db.from OrderLine |> Db.where (OrderLine.orderId.eq order.id) |> Db.all
  lines
}

test "total over two lines" = {
  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "", role: Staff }
  let prod1 <- Db.insert Product { name: Text.NonEmpty "Product 1", price: 100, active: True }
  let prod2 <- Db.insert Product { name: Text.NonEmpty "Product 2", price: 200, active: True }
  let orderId <- Db.insert Order { owner: me, status: Open, created: Time.epoch }
  let _ <- Db.insert OrderLine { id: "line1", orderId: orderId, productId: prod1, quantity: 2 }
  let _ <- Db.insert OrderLine { id: "line2", orderId: orderId, productId: prod2, quantity: 3 }
  let order = { id: orderId, owner: me, status: Open, created: Time.epoch }
  Task.succeed (total order == 800)
}

test "adding line to paid order is refused" = {
  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "", role: Staff }
  let orderId <- Db.insert Order { owner: me, status: Paid, created: Time.epoch }
  let r <- Test.run (update (AddLine { product: Product.ProductId "prod1", quantity: 1 }) { order: { id: orderId, owner: me, status: Paid, created: Time.epoch } } { user: me, role: Staff })
  Task.succeed (r.value == Err "Order is not open")
}

--- file: app/Product.alder ---
module Product

import User

type ProductId = brand Text

table Product {
  id: ProductId @id,
  name: Text.NonEmpty,
  price: Int.NonNegative,
  active: Bool = True,
}

name : ProductId -> Text
name id = {
  let found <- Db.get Product id
  match found {
    Just p -> p.name,
    Nothing -> "Unknown",
  }
}

price : ProductId -> Int
price id = {
  let found <- Db.get Product id
  match found {
    Just p -> p.price,
    Nothing -> 0,
  }
}

test "product name and price" = {
  let id <- Db.insert Product { name: Text.NonEmpty "Test Product", price: 150, active: True }
  Task.succeed (name id == "Test Product" && price id == 150)
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
  role: Role,
  created: Time @created,
}

--- file: app/Shop/Products.alder ---
module Shop.Products

import User
import Product

page "/products" { } requires user

type Model = { products: List Product }

load : Params -> Session -> Query Model
load params session = {
  let products <-
    Db.from Product
      |> Db.orderBy Product.name.asc
      |> Db.all
  Query.succeed { products: products }
}

form NewProduct = {
  name: Text.NonEmpty,
  price: Int.NonNegative,
}

msg Submit form NewProduct
msg ToggleActive { id: Product.ProductId }

update : Msg -> Model -> Session -> Task Nav
update msg model session = match msg {
  Submit p -> {
    let _ <- Db.insert Product { name: p.form.name, price: p.form.price, active: True }
    Task.succeed Rerender
  },
  ToggleActive p -> {
    let product <- Db.get Product p.id
    match product {
      Just prod -> {
        let _ <- Db.update Product prod.id (\t -> { t | active: not t.active })
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
    {if session.role == Manager ->
      <form submit={Submit}>
        <Input field="name" label="Name" />
        <Errors field="name" />
        <Input field="price" label="Price (cents)" />
        <Errors field="price" />
        <button>Add Product</button>
      </form>
    }
    {if session.role == Manager ->
      <ul>
        {for product in model.products ->
          <li>
            {product.name} - {product.price} cents
            <button click={ToggleActive { id: product.id }}>Toggle Active</button>
          </li>}
      </ul>
    }
  </Layout.Page>

--- file: app/Shop/Products.alder ---
module Shop.Products

import User
import Product

page "/products" { } requires user

type Model = { products: List Product }

load : Params -> Session -> Query Model
load params session = {
  let products <-
    Db.from Product
      |> Db.orderBy Product.name.asc
      |> Db.all
  Query.succeed { products: products }
}

form NewProduct = {
  name: Text.NonEmpty,
  price: Int.NonNegative,
}

msg Submit form NewProduct
msg ToggleActive { id: Product.ProductId }

update : Msg -> Model -> Session -> Task Nav
update msg model session = match msg {
  Submit p -> {
    let _ <- Db.insert Product { name: p.form.name, price: p.form.price, active: True }
    Task.succeed Rerender
  },
  ToggleActive p -> {
    let product <- Db.get Product p.id
    match product {
      Just prod -> {
        let _ <- Db.update Product prod.id (\t -> { t | active: not t.active })
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
    {if session.role == Manager ->
      <form submit={Submit}>
        <Input field="name" label="Name" />
        <Errors field="name" />
        <Input field="price" label="Price (cents)" />
        <Errors field="price" />
        <button>Add Product</button>
      </form>
    }
    {if session.role == Manager ->
      <ul>
        {for product in model.products ->
          <li>
            {product.name} - {product.price} cents
            <button click={ToggleActive { id: product.id }}>Toggle Active</button>
          </li>}
      </ul>
    }
  </Layout.Page>

--- file: app/Shop/Index.alder ---
module Shop.Index

import User
import Product
import Order

page "/" { } requires user

type Model = { orders: List Order }

load : Params -> Session -> Query Model
load params session = {
  let orders <-
    Db.from Order
      |> Db.where (Order.owner.eq session.user)
      |> Db.orderBy Order.created.desc
      |> Db.all
  Query.succeed { orders: orders }
}

form NewOrder = { }

msg Submit form NewOrder
msg View { id: Order.OrderId }

update : Msg -> Model -> Session -> Task Nav
update msg model session = match msg {
  Submit p -> {
    let _ <- Db.insert Order { owner: session.user, status: Open, created: Time.epoch }
    Task.succeed Rerender
  },
  View p -> {
    Task.succeed (Redirect (Shop.Show { id: p.id }))
  },
}

view : Model -> Session -> Html Msg
view model session =
  <Layout.Page title="Orders">
    <h1>Orders</h1>
    <form submit={Submit}>
      <button>Add Order</button>
    </form>
    <ul>
      {for order in model.orders ->
        <li>
          <a href={Shop.Show { id: order.id }}>{order.status} - {Order.total order}</a>
        </li>}
    </ul>
  </Layout.Page>

--- file: app/Shop/Show.alder ---
module Shop.Show

import User
import Product
import Order

page "/orders/{id}" { id: Order.OrderId } requires user

type Model = { order: Order }

load : Params -> Session -> Query Model
load params session = {
  let found <- Db.get Order params.id
  match found {
    Just order ->
      if order.owner == session.user || session.role == Manager then Query.succeed { order: order }
      else Query.notFound,
    Nothing -> Query.notFound,
  }
}

form NewLine = {
  product: Product.ProductId,
  quantity: Int.Positive,
}

msg AddLine form NewLine
msg MarkPaid
msg MarkCancelled

update : Msg -> Model -> Session -> Task Nav
update msg model session = match msg {
  AddLine p -> {
    let order <- Db.get Order model.order.id
    match order {
      Just o ->
        if o.status == Open then {
          let product <- Db.get Product p.form.product
          match product {
            Just prod -> {
              let _ <- Db.insert OrderLine { id: "line1", orderId: model.order.id, productId: p.form.product, quantity: p.form.quantity }
              Task.succeed Rerender
            },
            Nothing -> Task.fail "Product not found",
          }
        } else Task.fail "Order is not open",
      Nothing -> Task.notFound,
    }
  },
  MarkPaid p -> {
    let order <- Db.get Order model.order.id
    match order {
      Just o ->
        if o.status == Open then {
          let _ <- Db.update Order model.order.id (\t -> { t | status: Paid })
          Task.succeed Rerender
        } else Task.fail "Order is not open",
      Nothing -> Task.notFound,
    }
  },
  MarkCancelled p -> {
    let order <- Db.get Order model.order.id
    match order {
      Just o ->
        if o.status == Open then {
          let _ <- Db.update Order model.order.id (\t -> { t | status: Cancelled })
          Task.succeed Rerender
        } else Task.fail "Order is not open",
      Nothing -> Task.notFound,
    }
  },
}

view : Model -> Session -> Html Msg
view model session =
  <Layout.Page title="Order">
    <h1>Order {model.order.id}</h1>
    <p>Status: {model.order.status}</p>
    <p>Total: {Order.total model.order}</p>
    {if model.order.status == Open ->
      <form submit={AddLine}>
        <Input field="product" label="Product" />
        <Errors field="product" />
        <Input field="quantity" label="Quantity" />
        <Errors field="quantity" />
        <button>Add Line</button>
      </form>
    }
    {if model.order.status == Open ->
      <div>
        <button click={MarkPaid}>Mark Paid</button>
        <button click={MarkCancelled}>Cancel</button>
      </div>
    }
    <ul>
      {for line in Order.lines model.order ->
        <li>{Product.name line.productId} x {line.quantity}</li>}
    </ul>
  </Layout.Page>

--- file: app/Order.alder ---
module Order

import User
import Product

type OrderId = brand Text

type Status = Open | Paid | Cancelled

table Order {
  id: OrderId @id,
  owner: User.UserId @ref User,
  status: Status,
  created: Time @created,
}

table OrderLine {
  id: Text @id,
  orderId: OrderId @ref Order,
  productId: Product.ProductId @ref Product,
  quantity: Int.Positive,
}

total : Order -> Int
total order = {
  let lines <- Db.from OrderLine |> Db.where (OrderLine.orderId.eq order.id) |> Db.all
  let sum = List.sum (List.map (\l -> Product.price l.productId * l.quantity) lines)
  sum
}

lines : Order -> List OrderLine
lines order = {
  let lines <- Db.from OrderLine |> Db.where (OrderLine.orderId.eq order.id) |> Db.all
  lines
}

test "total over two lines" = {
  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "", role: Staff }
  let prod1 <- Db.insert Product { name: Text.NonEmpty "Product 1", price: 100, active: True }
  let prod2 <- Db.insert Product { name: Text.NonEmpty "Product 2", price: 200, active: True }
  let orderId <- Db.insert Order { owner: me, status: Open, created: Time.epoch }
  let _ <- Db.insert OrderLine { id: "line1", orderId: orderId, productId: prod1, quantity: 2 }
  let _ <- Db.insert OrderLine { id: "line2", orderId: orderId, productId: prod2, quantity: 3 }
  let order = { id: orderId, owner: me, status: Open, created: Time.epoch }
  Task.succeed (total order == 800)
}

test "adding line to paid order is refused" = {
  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "", role: Staff }
  let orderId <- Db.insert Order { owner: me, status: Paid, created: Time.epoch }
  let r <- Test.run (update (AddLine { product: Product.ProductId "prod1", quantity: 1 }) { order: { id: orderId, owner: me, status: Paid, created: Time.epoch } } { user: me, role: Staff })
  Task.succeed (r.value == Err "Order is not open")
}

test "staff user opening another user's order is not found" = {
  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "", role: Staff }
  let otherUser <- Db.insert User { email: Text.Email "other@example.com", name: Text.NonEmpty "Other", passwordHash: "", role: Staff }
  let orderId <- Db.insert Order { owner: otherUser, status: Open, created: Time.epoch }
  let r <- Test.run (update (AddLine { product: Product.ProductId "prod1", quantity: 1 }) { order: { id: orderId, owner: otherUser, status: Open, created: Time.epoch } } { user: me, role: Staff })
  Task.succeed (r.value == Err "not found")
}

test "manager user opening another user's order is allowed" = {
  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "", role: Manager }
  let otherUser <- Db.insert User { email: Text.Email "other@example.com", name: Text.NonEmpty "Other", passwordHash: "", role: Staff }
  let orderId <- Db.insert Order { owner: otherUser, status: Open, created: Time.epoch }
  let r <- Test.run (update (AddLine { product: Product.ProductId "prod1", quantity: 1 }) { order: { id: orderId, owner: otherUser, status: Open, created: Time.epoch } } { user: me, role: Manager })
  Task.succeed (r.value == Err "Product not found")
}

--- file: app/Product.alder ---
module Product

import User

type ProductId = brand Text

table Product {
  id: ProductId @id,
  name: Text.NonEmpty,
  price: Int.NonNegative,
  active: Bool = True,
}

name : ProductId -> Text
name id = {
  let found <- Db.get Product id
  match found {
    Just p -> p.name,
    Nothing -> "Unknown",
  }
}

price : ProductId -> Int
price id = {
  let found <- Db.get Product id
  match found {
    Just p -> p.price,
    Nothing -> 0,
  }
}

test "product name and price" = {
  let id <- Db.insert Product { name: Text.NonEmpty "Test Product", price: 150, active: True }
  Task.succeed (name id == "Test Product" && price id == 150)
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
  role: Role,
  created: Time @created,
}