# Task T6: a small shop's orders

Write a complete new Alder app (not the todo app) for the staff of a small
shop:

- Users log in with an email and a password, checked by a `Password`
  capability as in the example. A user has a role, `Staff` or `Manager`.
- A product has a name, a price in cents (not negative), and an active flag.
- An order belongs to the user who created it, has a status (`Open`, `Paid`
  or `Cancelled`) and a created time, and has line items: each line names a
  product and a quantity (positive).
- `/` requires login and lists the logged-in user's orders, newest first,
  with each order's status and total (the sum of quantity × price over its
  lines), linking to each order. A form creates a new, empty, Open order.
- `/orders/{id}` requires login and shows one order with its lines and
  total. While the order is Open: a form adds a line (choose an active
  product, give a quantity), and buttons mark it Paid or Cancelled. Another
  user's order is 404 — unless the logged-in user is a Manager.
- Adding a line to an order that is not Open is refused by the server, even
  when the form comes from a page loaded while the order was still Open.
- `/products` requires login and a Manager: it lists every product, has a
  form to add one, and a button per product to toggle active. A Staff user
  gets 404.
- `/login` and logout, an `App` module, a `Layout` module and a `User`
  module as in the reference.

**Names.** Use exactly these names; anything not listed here is your choice.
The tables are exactly the ones below, with exactly these fields: add no
further tables or fields.

- `app/User.alder`: module `User`, `type UserId = brand Text`,
  `type Role = Staff | Manager`, and
  `table User { id: UserId @id, email: Text.Email @unique, name: Text.NonEmpty, passwordHash: Text, role: Role = Staff, created: Time @created }`.
- `app/App.alder`: module `App`, with exactly
  `session { user: Maybe User.UserId }`, `login = Auth.Login`, and
  `msg Logout`, which answers `Redirect (Auth.Login {})`.
- `app/Product.alder`: module `Product`, `type ProductId = brand Text`, and
  `table Product { id: ProductId @id, name: Text.NonEmpty, price: Int.NonNegative, active: Bool = True }`.
- `app/Order.alder`: module `Order`, `type OrderId = brand Text`,
  `type Status = Open | Paid | Cancelled`, and
  `table Order { id: OrderId @id, owner: User.UserId @ref User, status: Status = Open, created: Time @created }`.
- `app/Line.alder`: module `Line`, `type LineId = brand Text`, and
  `table Line { id: LineId @id, order: Order.OrderId @ref Order @cascade, product: Product.ProductId @ref Product, quantity: Int.Positive }`.
- Totals are shown in cents, digits only (`1250`, not `$12.50`).
- `app/Orders/Index.alder`: page module `Orders.Index`, `page "/" { } requires user`.
  `msg Create` (no parameters, sent by a button) creates the order and
  answers `Redirect (Orders.Show { id: … })` (the new order). The orders are
  listed in an element with `id="orders"`, one `<li>` per order; inside
  it, an element with `class="status"` has the status (`Open`, `Paid` or
  `Cancelled`) as its whole text, and one with `class="total"` the total.
- `app/Orders/Show.alder`: page module `Orders.Show`,
  `page "/orders/{id}" { id: Order.OrderId } requires user`. The status is
  the whole text of an element with `id="status"`, the total of one with
  `id="total"`. One form per active product adds a line:
  `form NewLine = { quantity: Int.Positive }`, posted by
  `msg AddLine { product: Product.ProductId } form NewLine`; `msg Pay` and
  `msg Cancel` (no parameters) are buttons. Each of the three answers
  `Redirect (Orders.Show { id: … })` (the same order). Only an Open order's
  page shows these forms and buttons.
- `app/Products/Index.alder`: page module `Products.Index`,
  `page "/products" { } requires user`. The products are listed in an
  element with `id="products"`; each product's name is the whole text of an
  element with `class="name"`. `form NewProduct = { name: Text.NonEmpty, price: Int.NonNegative }`,
  posted by `msg Add form NewProduct`, and `msg Toggle { id: Product.ProductId }`,
  a button per product, each answer `Redirect (Products.Index {})`.
- `app/Auth/Login.alder`: page module `Auth.Login`, `page "/login" { }`, with
  the `Password` capability, form `Credentials` and `msg Submit` as in the
  reference.

Also write tests (`test "…" = …`, chapter 10) in the modules they test: an
Open order's total over two lines; adding a line to a Paid order is refused
(say in a comment whether you chose `Task.fail` or `Task.notFound`, and
why); a Staff user opening another user's order is not found; a Manager
opening it is. `alder test` must pass. Do not write tests that call the
`Password` capability.

Output every file, in full, each preceded by a line
`--- file: app/<path> ---`. No explanation, only files.
