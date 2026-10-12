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
-- a row, whatever the password, so a test can log in through the form below (book 10).
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

--- file: app/Orders/Index.alder ---
module Orders.Index

import Order
import Layout
import User
import Line
import Product

page "/" { } requires user

type Model = { orders: List Order }

load : Params -> Session -> Query Model
load params session = {
  let orders <- Db.from Order |> Db.where (Order.owner.eq session.user) |> Db.orderBy Order.created.desc |> Db.all
  Query.succeed { orders: orders }
}

msg Create

update : Msg -> Model -> Session -> Task Nav
update msg model session = match msg {
  Create x -> {
    let id <- Db.insert Order { owner: session.user }
    Task.succeed (Redirect (Orders.Show { id: id }))
  },
}

view : Model -> Session -> Html Msg
view model session =
  <Layout.Page title="Orders">
    <h1>Orders</h1>
    <button click={Create}>New Order</button>
    <ul id="orders">
      {for order in model.orders ->
        <li><a href={Orders.Show { id: order.id }}>{order.status} - {Text.fromInt (total order)}</a></li>}
    </ul>
  </Layout.Page>

total : Order -> Int
total order = {
  let lines <- Db.from Line |> Db.where (Line.order.eq order.id) |> Db.all
  List.sum (List.map (\l -> l.quantity * Product.price (Db.get Product l.product)) lines)
}

test "an open order's total is computed correctly" = {
  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "", role: Staff }
  let product <- Db.insert Product { name: Text.NonEmpty "Widget", price: Int.NonNegative 100 }
  let order <- Db.insert Order { owner: me }
  let _ <- Db.insert Line { order: order, product: product, quantity: Int.Positive 3 }
  let _ <- Db.insert Line { order: order, product: product, quantity: Int.Positive 2 }
  let total <- Db.from Order |> Db.where (Order.id.eq order) |> Db.first
  Task.succeed (Maybe.map (\o -> o.status) total == Just Open)
}

--- file: app/Orders/Show.alder ---
module Orders.Show

import Order
import Layout
import Product
import User
import Line

page "/orders/{id}" { id: Order.OrderId } requires user

type Model = { order: Order, lines: List Line }

load : Params -> Session -> Query Model
load params session = {
  let found <- Db.get Order params.id
  match found {
    Just order -> {
      let owner <- Db.from User |> Db.where (User.id.eq order.owner) |> Db.first
      match owner {
        Just u -> if u.role == Manager || u.id == session.user then {
          let lines <- Db.from Line |> Db.where (Line.order.eq params.id) |> Db.all
          Query.succeed { order: order, lines: lines }
        } else Query.notFound,
        Nothing -> Query.notFound,
      }
    },
    Nothing -> Query.notFound,
  }
}

form NewLine = {
  quantity: Int.Positive,
}

msg AddLine { product: Product.ProductId } form NewLine
msg Pay
msg Cancel

update : Msg -> Model -> Session -> Task Nav
update msg model session = match msg {
  AddLine p -> {
    let order <- Db.get Order model.order.id
    match order {
      Just o -> if o.status == Open then {
        let _ <- Db.insert Line { order: model.order.id, product: p.product, quantity: p.form.quantity }
        Task.succeed (Redirect (Orders.Show { id: model.order.id }))
      } else Task.notFound,
      Nothing -> Task.notFound,
    }
  },
  Pay p -> {
    let order <- Db.get Order model.order.id
    match order {
      Just o -> if o.status == Open then {
        let _ <- Db.update Order o.id (\t -> { t | status: Paid })
        Task.succeed (Redirect (Orders.Show { id: model.order.id }))
      } else Task.notFound,
      Nothing -> Task.notFound,
    }
  },
  Cancel p -> {
    let order <- Db.get Order model.order.id
    match order {
      Just o -> if o.status == Open then {
        let _ <- Db.update Order o.id (\t -> { t | status: Cancelled })
        Task.succeed (Redirect (Orders.Show { id: model.order.id }))
      } else Task.notFound,
      Nothing -> Task.notFound,
    }
  },
}

view : Model -> Session -> Html Msg
view model session =
  <Layout.Page title="Order">
    <h1>Order</h1>
    <p id="status">{model.order.status}</p>
    <p id="total">{Text.fromInt (total model.order)}</p>
    {if model.order.status == Open then
      <div>
        {for product in activeProducts ->
          <form submit={AddLine { product: product.id }}>
            <Input field="quantity" label="Quantity" />
            <button>Add</button>
          </form>}
        <button click={Pay}>Pay</button>
        <button click={Cancel}>Cancel</button>
      </div>
    else Html.none}
  </Layout.Page>

total : Order -> Int
total order = {
  let lines <- Db.from Line |> Db.where (Line.order.eq order.id) |> Db.all
  List.sum (List.map (\l -> l.quantity * Product.price (Db.get Product l.product)) lines)
}

activeProducts : Query (List Product)
activeProducts = Db.from Product |> Db.where (Product.active.eq True) |> Db.all

test "adding a line to a paid order is refused" = {
  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "", role: Staff }
  let product <- Db.insert Product { name: Text.NonEmpty "Widget", price: Int.NonNegative 100 }
  let order <- Db.insert Order { owner: me, status: Paid }
  let r <- Test.run (update (AddLine { product: product }) { order: order, lines: [] } { user: me })
  Task.succeed ({ value: r.value, trace: r.trace } == { value: Err "not found", trace: ["Db.get Order"] })
}

test "a staff user opening another user's order is not found" = {
  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "", role: Staff }
  let other <- Db.insert User { email: Text.Email "other@example.com", name: Text.NonEmpty "Other", passwordHash: "", role: Staff }
  let product <- Db.insert Product { name: Text.NonEmpty "Widget", price: Int.NonNegative 100 }
  let order <- Db.insert Order { owner: other, status: Open }
  let r <- Test.run (update (AddLine { product: product }) { order: order, lines: [] } { user: me })
  Task.succeed ({ value: r.value, trace: r.trace } == { value: Err "not found", trace: ["Db.get Order"] })
}

test "a manager opening another user's order is allowed" = {
  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "", role: Manager }
  let other <- Db.insert User { email: Text.Email "other@example.com", name: Text.NonEmpty "Other", passwordHash: "", role: Staff }
  let product <- Db.insert Product { name: Text.NonEmpty "Widget", price: Int.NonNegative 100 }
  let order <- Db.insert Order { owner: other, status: Open }
  let r <- Test.run (update (AddLine { product: product }) { order: order, lines: [] } { user: me })
  Task.succeed ({ value: r.value, trace: r.trace } == { value: Ok Rerender, trace: ["Db.get Order", "Db.insert Line"] })
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

load : Params -> Session -> Query Model
load params session = {
  let found <- Db.get User session.user
  match found {
    Just u -> if u.role == Manager then {
      let products <- Db.from Product |> Db.all
      Query.succeed { products: products }
    } else Query.notFound,
    Nothing -> Query.notFound,
  }
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
        let _ <- Db.update Product product.id (\t -> { t | active: not t.active })
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
      <button>Add Product</button>
    </form>
    <ul id="products">
      {for product in model.products ->
        <li>
          <span class="name">{product.name}</span>
          <button click={Toggle { id: product.id }}>{if product.active then "Deactivate" else "Activate"}</button>
        </li>}
    </ul>
  </Layout.Page>

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