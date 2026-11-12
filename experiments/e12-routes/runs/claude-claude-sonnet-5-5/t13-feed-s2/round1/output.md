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

--- file: app/Feed/Index.alder ---
module Feed.Index

import Layout
import Table.Entry

route Index = GET  "/feed"      { }
route More  = GET  "/feed/more" { after: Int }
route Post  = POST "/feed"      { } form NewEntry

type Model = { entries: List Entry, after: Int, total: Int }

form NewEntry = { text: Text.NonEmpty }

loadPage : Int -> Query Model
loadPage after = {
  let entries <-
    Db.from Entry
      |> Db.orderBy Entry.created.desc
      |> Db.orderBy Entry.id.asc
      |> Db.offset after
      |> Db.limit 10
      |> Db.all
  let total <- Db.from Entry |> Db.count
  Query.succeed { entries: entries, after: after, total: total }
}

rows : List Entry -> Html
rows entries =
  <ul>
    {for e in entries -> <li>{e.text}</li>}
  </ul>

-- The element that asks for the entries after the first `next`; none after the last entry.
sentinel : Int -> Int -> Html
sentinel next total =
  if next < total then
    <div id={"more-" ++ Text.fromInt next} intersect={More { after: next }}></div>
  else Html.none

Index.load : Index.Params -> Session -> Query Model
Index.load params session = loadPage 0

Index.view : Model -> Session -> Html
Index.view model session =
  <Layout.Page title="Feed">
    <h1>Feed</h1>
    <al-card>
      <form submit={Post}>
        <Errors form />
        <al-input field="text" label="What happened?" />
        <al-button type="submit">Post</al-button>
      </form>
    </al-card>
    <section>
      {rows model.entries}
      {sentinel (model.after + List.length model.entries) model.total}
    </section>
  </Layout.Page>

More.load : More.Params -> Session -> Query Model
More.load params session = loadPage params.after

More.view : Model -> Session -> Html
More.view model session =
  <div id={"more-" ++ Text.fromInt model.after}>
    {rows model.entries}
    {sentinel (model.after + List.length model.entries) model.total}
  </div>

Post.update : Post.Params -> Session -> Task Response
Post.update p session = {
  let _ <- Db.insert Entry { text: p.form.text }
  Task.succeed (Response.redirect (Feed.Index.Index {}))
}

label : Int -> Text
label n = "e" ++ Text.fromInt n

seed : Int -> Task (List Entry.EntryId)
seed n = Task.all (List.map (\i -> Db.insert Entry { text: Text.NonEmpty (label i) }) (List.range 1 n))

test "scrolling twice through 25 entries shows each once, newest first" = {
  let _ <- seed 25
  let first <- Test.get (Index {})
  let second <- Test.intersect (More { after: 10 })
  let third <- Test.intersect (More { after: 20 })
  Task.succeed (
    {
      firstCount: Test.count "li" first.doc,
      secondCount: Test.count "li" second.doc,
      texts: Test.texts "li" third.doc,
      sentinel: Test.has "[data-on-intersect__once]" third.doc,
    } == {
      firstCount: 10,
      secondCount: 20,
      texts: List.map label (List.reverse (List.range 1 25)),
      sentinel: False,
    }
  )
}

test "a feed of exactly ten entries has no sentinel" = {
  let _ <- seed 10
  let page <- Test.get (Index {})
  Task.succeed (
    { items: Test.count "li" page.doc, sentinel: Test.has "[data-on-intersect__once]" page.doc }
      == { items: 10, sentinel: False }
  )
}

test "posting an entry puts it first" = {
  let _ <- seed 2
  let _ <- Test.get (Index {})
  let r <- Test.submit (Post {}) { text: Text.NonEmpty "fresh" }
  let page <- Test.get (Index {})
  Task.succeed (
    { answer: r.answer, texts: Test.texts "li" page.doc }
      == { answer: Redirected (Index {}), texts: ["fresh", "e2", "e1"] }
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

--- file: app/Table/Entry.alder ---
module Table.Entry

type EntryId = brand Text

table Entry {
  id: EntryId @id,
  text: Text.NonEmpty,
  created: Time @created,
}
