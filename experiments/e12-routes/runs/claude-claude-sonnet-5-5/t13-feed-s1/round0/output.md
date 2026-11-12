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

--- file: app/Table/Entry.alder ---
module Table.Entry

type EntryId = brand Text

table Entry {
  id: EntryId @id,
  text: Text.NonEmpty,
  created: Time @created,
}

--- file: app/Feed/Index.alder ---
module Feed.Index

import Layout
import Table.Entry

route Index = GET  "/feed"      { }
route More  = GET  "/feed/more" { after: Int }
route Post  = POST "/feed"      { } form NewEntry

type Model = { entries: List Entry, total: Int }

type MoreModel = { after: Int, entries: List Entry, total: Int }

form NewEntry = { text: Text.NonEmpty }

pageSize : Int
pageSize = 10

page : Int -> Query (List Entry)
page after =
  Db.from Entry
    |> Db.orderBy Entry.created.desc
    |> Db.orderBy Entry.id.desc
    |> Db.offset after
    |> Db.limit pageSize
    |> Db.all

row : Entry -> Html
row e = <li>{e.text}</li>

-- The element that asks for the entries after the first `next`; none when nothing follows.
sentinel : Int -> Int -> Html
sentinel next total =
  if next < total then
    <div id={"more-" ++ Text.fromInt next} intersect={More { after: next }}></div>
  else Html.none

Index.load : Index.Params -> Session -> Query Model
Index.load params session = {
  let entries <- page 0
  let total <- Db.from Entry |> Db.count
  Query.succeed { entries: entries, total: total }
}

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
    <ul id="entries">{for e in model.entries -> row e}</ul>
    {sentinel (List.length model.entries) model.total}
  </Layout.Page>

More.load : More.Params -> Session -> Query MoreModel
More.load params session = {
  let entries <- page params.after
  let total <- Db.from Entry |> Db.count
  Query.succeed { after: params.after, entries: entries, total: total }
}

More.view : MoreModel -> Session -> Html
More.view model session =
  <div id={"more-" ++ Text.fromInt model.after}>
    <ul>{for e in model.entries -> row e}</ul>
    {sentinel (model.after + List.length model.entries) model.total}
  </div>

Post.update : Post.Params -> Session -> Task Response
Post.update p session = {
  let _ <- Db.insert Entry { text: p.form.text }
  Task.succeed (Response.redirect (Index {}))
}

seed : Int -> Task (List Entry.EntryId)
seed n =
  Task.all (List.map (\i -> Db.insert Entry { text: Text.NonEmpty ("e" ++ Text.fromInt i) }) (List.range 1 n))

test "scrolling twice through 25 entries shows each once, newest first" = {
  let _ <- seed 25
  let first <- Test.get (Index {})
  let _ <- Test.intersect (More { after: 10 })
  let last <- Test.intersect (More { after: 20 })
  let expected = List.map (\i -> "e" ++ Text.fromInt i) (List.reverse (List.range 1 25))
  Task.succeed (
    {
      firstRows: Test.count "li" first.doc,
      firstSentinel: Test.has "[data-on-intersect__once]" first.doc,
      texts: Test.texts "li" last.doc,
      sentinel: Test.has "[data-on-intersect__once]" last.doc,
    } == {
      firstRows: 10,
      firstSentinel: True,
      texts: expected,
      sentinel: False,
    }
  )
}

test "a feed of exactly ten entries has no sentinel" = {
  let _ <- seed 10
  let r <- Test.get (Index {})
  Task.succeed (
    { rows: Test.count "li" r.doc, sentinel: Test.has "[data-on-intersect__once]" r.doc }
      == { rows: 10, sentinel: False }
  )
}

test "posting an entry puts it first" = {
  let _ <- seed 3
  let _ <- Test.get (Index {})
  let posted <- Test.submit (Post {}) { text: Text.NonEmpty "fresh" }
  let page <- Test.get (Index {})
  Task.succeed (
    { answer: posted.answer, first: List.head (Test.texts "li" page.doc), rows: Test.count "li" page.doc }
      == { answer: Redirected (Index {}), first: Just "fresh", rows: 4 }
  )
}
