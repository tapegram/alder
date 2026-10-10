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
- `/products` requires login and a Manager: it lists every product, has a
  form to add one, and a button per product to toggle active. A Staff user
  gets 404.
- `/login` and logout, an `App` module, a `Layout` module and a `User`
  module as in the reference.

Also write tests (`test "…" = …`, chapter 10) in the modules they test: an
Open order's total over two lines; adding a line to a Paid order is refused
(say in a comment whether you chose `Task.fail` or `Task.notFound`, and
why); a Staff user opening another user's order is not found; a Manager
opening it is. `alder test` must pass. Do not write tests that call the
`Password` capability.

Output every file, in full, each preceded by a line
`--- file: app/<path> ---`. No explanation, only files.
