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

form NewEntry = { text: Text.NonEmpty }

-- `after` is how many entries were shown before these; `more` says whether any follow them.
type Model = {
  after: Int,
  entries: List Entry,
  more: Bool,
}

pageSize : Int
pageSize = 10

-- One page of entries, newest first, after the first `after`. One row more than a page is read,
-- so the page knows whether a sentinel should follow it.
entriesAfter : Int -> Query Model
entriesAfter after = {
  let rows <-
    Db.from Entry
      |> Db.orderBy Entry.created.desc
      |> Db.orderBy Entry.id.desc
      |> Db.offset after
      |> Db.limit (pageSize + 1)
      |> Db.all
  Query.succeed {
    after: after,
    entries: List.take pageSize rows,
    more: List.length rows > pageSize,
  }
}

row : Entry -> Html
row entry = <li>{entry.text}</li>

sentinelId : Int -> Text
sentinelId shown = "more-" ++ Text.fromInt shown

-- The element that asks for the entries after this page; none after the last entry.
sentinel : Model -> Html
sentinel model =
  if model.more then
    <div
      id={sentinelId (model.after + pageSize)}
      intersect={More { after: model.after + pageSize }}
    ></div>
  else Html.none

Index.load : Index.Params -> Session -> Query Model
Index.load params session = entriesAfter 0

Index.view : Model -> Session -> Html
Index.view model session =
  <Layout.Page title="Feed">
    <h1>Feed</h1>
    <form submit={Post}>
      <Errors form />
      <Input field="text" label="What's happening?" />
      <button>Post</button>
    </form>
    <ul id="entries">{for entry in model.entries -> row entry}</ul>
    {sentinel model}
  </Layout.Page>

More.load : More.Params -> Session -> Query Model
More.load params session = entriesAfter (Int.max 0 params.after)

More.view : Model -> Session -> Html
More.view model session =
  <div id={sentinelId model.after}>
    <ul>{for entry in model.entries -> row entry}</ul>
    {sentinel model}
  </div>

Post.update : Post.Params -> Session -> Task Response
Post.update p session = {
  let _ <- Db.insert Entry { text: p.form.text }
  Task.succeed (Response.redirect (Index {}))
}

test "scrolling twice through 25 entries shows each once, newest first" = {
  let _ <-
    List.range 1 25
      |> List.map (\i -> Db.insert Entry { text: Text.NonEmpty "Entry {Text.fromInt i}" })
      |> Task.all
  let first <- Test.get (Index {})
  let second <- Test.intersect (More { after: 10 })
  let third <- Test.intersect (More { after: 20 })
  Task.succeed (
    {
      firstRows: Test.count "li" first.doc,
      firstSentinel: Test.has "#more-10" first.doc,
      secondRows: Test.count "li" second.doc,
      secondSentinel: Test.has "#more-20" second.doc,
      texts: Test.texts "li" third.doc,
      sentinel: Test.has "[data-on-intersect__once]" third.doc,
    } == {
      firstRows: 10,
      firstSentinel: True,
      secondRows: 20,
      secondSentinel: True,
      texts: List.map (\i -> "Entry {Text.fromInt i}") (List.reverse (List.range 1 25)),
      sentinel: False,
    }
  )
}

test "a feed of exactly ten entries has no sentinel" = {
  let _ <-
    List.range 1 10
      |> List.map (\i -> Db.insert Entry { text: Text.NonEmpty "Entry {Text.fromInt i}" })
      |> Task.all
  let page <- Test.get (Index {})
  Task.succeed (
    {
      rows: Test.count "li" page.doc,
      sentinel: Test.has "[data-on-intersect__once]" page.doc,
    } == {
      rows: 10,
      sentinel: False,
    }
  )
}

test "posting an entry puts it first" = {
  let _ <- Db.insert Entry { text: Text.NonEmpty "Older" }
  let _ <- Db.insert Entry { text: Text.NonEmpty "Newer" }
  let _ <- Test.get (Index {})
  let r <- Test.submit Post { text: Text.NonEmpty "Hello" }
  Task.succeed (
    {
      answer: r.answer,
      url: r.url,
      texts: Test.texts "li" r.doc,
    } == {
      answer: Redirected (Index {}),
      url: "/feed",
      texts: ["Hello", "Newer", "Older"],
    }
  )
}
