--- file: app/App.alder ---
module App

import Layout

session { }

error : Error -> Html
error err =
  <Layout.Page title="Error">
    <h1>Something went wrong</h1>
    <p>Please try again.</p>
  </Layout.Page>

notFound : Html
notFound = <Layout.Page title="Not found"><h1>Not found</h1></Layout.Page>

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

--- file: app/Table/Item.alder ---
module Table.Item

type ItemId = brand Text

table Item {
  id: ItemId @id,
  name: Text.NonEmpty @unique,
  quantity: Int,
}

--- file: app/Pantry/Index.alder ---
module Pantry.Index

import Layout
import Table.Item

route Index  = GET    "/pantry"                     { }
route Row    = GET    "/pantry/items/{id}"          { id: Item.ItemId }
route Rename = PUT    "/pantry/items/{id}"          { id: Item.ItemId } form NewName
route Bump   = POST   "/pantry/items/{id}/quantity" { id: Item.ItemId }
route Remove = DELETE "/pantry/items/{id}"          { id: Item.ItemId }

type Model = { items: List Item }

form NewName = { name: Text.NonEmpty }

Index.load : Index.Params -> Session -> Query Model
Index.load params session = {
  let items <-
    Db.from Item
      |> Db.orderBy Item.name.asc
      |> Db.all
  Query.succeed { items: items }
}

Index.view : Model -> Session -> Html
Index.view model session =
  <Layout.Page title="Pantry">
    <h1>Pantry</h1>
    <ul id="items">{for item in model.items -> Row.view item session}</ul>
  </Layout.Page>

Row.load : Row.Params -> Session -> Query Item
Row.load params session = {
  let found <- Db.get Item params.id
  match found {
    Just item -> Query.succeed item,
    Nothing -> Query.notFound,
  }
}

Row.view : Item -> Session -> Html
Row.view item session =
  <li id={"item-" ++ item.id}>
    <span class="name">{item.name}</span>
    <span class="quantity">{Text.fromInt item.quantity}</span>
    <form submit={Rename { id: item.id }}>
      <Errors form />
      <Input field="name" label="New name" value={item.name} />
      <button>Rename</button>
    </form>
    <button click={Bump { id: item.id }}>Add one</button>
    <button click={Remove { id: item.id }}>Remove</button>
  </li>

-- Decide before writing: a refused rename stores nothing.
Rename.update : Rename.Params -> Session -> Task Response
Rename.update p session = {
  let taken <-
    Db.from Item
      |> Db.where (Item.name.eq p.form.name)
      |> Db.where (Item.id.ne p.id)
      |> Db.count
  if taken > 0 then
    Task.succeed (Response.invalid "name already taken")
  else {
    let _ <- Db.update Item p.id (\i -> { i | name: p.form.name })
    Task.succeed (Response.redirect (Index {}))
  }
}

Bump.update : Bump.Params -> Session -> Task Response
Bump.update p session = {
  let _ <- Db.update Item p.id (\i -> { i | quantity: i.quantity + 1 })
  Task.succeed (Response.render (Row { id: p.id }))
}

Remove.update : Remove.Params -> Session -> Task Response
Remove.update p session = {
  let _ <- Db.delete Item p.id
  Task.succeed (Response.refresh |> Response.toast "Removed")
}

test "renaming to another item's name is refused and stores nothing" = {
  let flour <- Db.insert Item { name: Text.NonEmpty "Flour", quantity: 2 }
  let _ <- Db.insert Item { name: Text.NonEmpty "Sugar", quantity: 1 }
  let _ <- Test.get (Index {})
  let r <- Test.submit (Rename { id: flour }) { name: Text.NonEmpty "Sugar" }
  let after <- Db.get Item flour
  Task.succeed (
    {
      answer: r.answer,
      status: r.status,
      name: Maybe.map (\i -> i.name) after,
    } == {
      answer: FormRejected [{ field: "", message: "name already taken" }],
      status: 422,
      name: Just (Text.NonEmpty "Flour"),
    }
  )
}

test "renaming to a free name is stored" = {
  let flour <- Db.insert Item { name: Text.NonEmpty "Flour", quantity: 2 }
  let _ <- Db.insert Item { name: Text.NonEmpty "Sugar", quantity: 1 }
  let _ <- Test.get (Index {})
  let r <- Test.submit (Rename { id: flour }) { name: Text.NonEmpty "Rice" }
  let after <- Db.get Item flour
  Task.succeed (
    {
      answer: r.answer,
      name: Maybe.map (\i -> i.name) after,
      names: Test.texts ".name" r.doc,
    } == {
      answer: Redirected (Index {}),
      name: Just (Text.NonEmpty "Rice"),
      names: ["Rice", "Sugar"],
    }
  )
}

test "renaming an item to its own name is stored" = {
  let flour <- Db.insert Item { name: Text.NonEmpty "Flour", quantity: 2 }
  let _ <- Test.get (Index {})
  let r <- Test.submit (Rename { id: flour }) { name: Text.NonEmpty "Flour" }
  Task.succeed (r.answer == Redirected (Index {}))
}

test "adding one renders the item's row with the new quantity" = {
  let flour <- Db.insert Item { name: Text.NonEmpty "Flour", quantity: 2 }
  let _ <- Db.insert Item { name: Text.NonEmpty "Sugar", quantity: 1 }
  let _ <- Test.get (Index {})
  let r <- Test.click (Bump { id: flour })
  let after <- Db.get Item flour
  Task.succeed (
    {
      answer: r.answer,
      quantities: Test.texts ".quantity" r.doc,
      stored: Maybe.map (\i -> i.quantity) after,
    } == {
      answer: Rendered (Row { id: flour }),
      quantities: ["3", "1"],
      stored: Just 3,
    }
  )
}

test "removing an item drops it from the page and shows the toast" = {
  let flour <- Db.insert Item { name: Text.NonEmpty "Flour", quantity: 2 }
  let _ <- Db.insert Item { name: Text.NonEmpty "Sugar", quantity: 1 }
  let _ <- Test.get (Index {})
  let r <- Test.click (Remove { id: flour })
  let after <- Db.get Item flour
  Task.succeed (
    {
      answer: r.answer,
      notices: r.notices,
      names: Test.texts ".name" r.doc,
      gone: after == Nothing,
    } == {
      answer: Refreshed,
      notices: ["Removed"],
      names: ["Sugar"],
      gone: True,
    }
  )
}
