# Task T15: a pantry whose rows answer for themselves

Write a complete new Alder app (not the todo app) for a pantry:

- There are no users and no login: anyone may see the pantry and change it.
- An item has a name and a quantity. No two items have the same name.
  Items are added outside the app (the app has no form to add one).
- `/pantry` lists every item, each with its name and its quantity, a form
  to rename it, a button that adds one to its quantity and a button that
  removes it.
- Renaming an item to a name another item already has is refused with the
  message "name already taken": nothing is stored, and the page is shown
  again. An item may be "renamed" to its own name. Any other new name is
  stored, and the browser is sent back to `/pantry`.
- Adding one answers with that item's row alone, not the whole page.
- Removing an item deletes it and shows the page again with a toast,
  "Removed".
- An `App` module and a `Layout` module as in the reference.

**Names.** Use exactly these names; anything not listed here is your choice.
The tables are exactly the ones below, with exactly these fields: add no
further tables or fields.

- `app/App.alder`: module `App`, with exactly `session { }` and no `login`.
- `app/Table/Item.alder`: module `Table.Item`, `type ItemId = brand Text`, and
  `table Item { id: ItemId @id, name: Text.NonEmpty @unique, quantity: Int }`.
- `app/Pantry/Index.alder`: route module `Pantry.Index`,
  `route Index = GET "/pantry" { }`: an element `<ul id="items">` whose
  children are the items' `<li>`s, one per item, each `Row`'s view of that
  item, and nothing else (no `<li>` for an empty pantry).
  `route Row = GET "/pantry/items/{id}" { id: Item.ItemId }`: its view is
  the item's `<li id="item-{id}">` (`"item-" ++ item.id`, as chapter 6's
  `Row`). In it, the item's name is the whole text of an element with
  `class="name"` and its quantity, as digits only (`3`), the whole text of
  an element with `class="quantity"`; no other element in the list has
  either class. The `<li>` also holds the item's rename form, its "add
  one" button (`click={Bump { id: … }}`) and its remove button
  (`click={Remove { id: … }}`).
  `form NewName = { name: Text.NonEmpty }`, posted by
  `route Rename { id: Item.ItemId } form NewName`, which answers
  `Response.invalid "name already taken"` when another item has the name
  (storing nothing), and otherwise stores the name and answers
  `Response.redirect (Pantry.Index.Index {})`.
  `route Bump { id: Item.ItemId }` adds one to the item's quantity and
  answers `Response.render (Pantry.Index.Row { id: … })`.
  `route Remove { id: Item.ItemId }` deletes the item and answers
  `Response.refresh |> Response.toast "Removed"`.

Also write tests (`test "…" = …`, chapter 10) in the modules they test:
renaming to another item's name is refused and stores nothing; renaming to
a free name is stored; adding one renders the item's row with the new
quantity; removing an item drops it from the page and shows the toast.
`alder test` must pass.

Output every file, in full, each preceded by a line
`--- file: app/<path> ---`. No explanation, only files.
