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

-- One page of entries after the first `after`, newest first. The eleventh row, if any, only
-- tells whether another page follows.
type Chunk = { after: Int, entries: List Entry, more: Bool }

chunkAfter : Int -> Query Chunk
chunkAfter after = {
  let rows <-
    Db.from Entry
      |> Db.orderBy Entry.created.desc
      |> Db.offset after
      |> Db.limit 11
      |> Db.all
  Query.succeed { after: after, entries: List.take 10 rows, more: List.length rows > 10 }
}

-- The element that asks for the entries after `next`, and is replaced by their answer.
sentinel : Int -> Html
sentinel next =
  <div id={"more-" ++ Text.fromInt next} intersect={More { after: next }}></div>

Index.load : Index.Params -> Session -> Query Chunk
Index.load params session = chunkAfter 0

Index.view : Chunk -> Session -> Html
Index.view chunk session =
  <Layout.Page title="Activity">
    <h1>Activity</h1>
    <al-card>
      <form submit={Post}>
        <al-input field="text" label="Post an entry" />
        <Errors field="text" />
        <al-button type="submit">Post</al-button>
      </form>
    </al-card>
    <ul>{for e in chunk.entries -> <li>{e.text}</li>}</ul>
    {if chunk.more then sentinel (chunk.after + 10) else Html.none}
  </Layout.Page>

More.load : More.Params -> Session -> Query Chunk
More.load params session = chunkAfter params.after

More.view : Chunk -> Session -> Html
More.view chunk session =
  <div id={"more-" ++ Text.fromInt chunk.after}>
    <ul>{for e in chunk.entries -> <li>{e.text}</li>}</ul>
    {if chunk.more then sentinel (chunk.after + 10) else Html.none}
  </div>

Post.update : Post.Params -> Session -> Task Response
Post.update p session = {
  let _ <- Db.insert Entry { text: p.form.text }
  Task.succeed (Response.redirect (Index {}))
}

-- Stores each text as a new entry, in order: the last one is the newest.
addEntries : List Text -> Task Unit
addEntries texts = match texts {
  [] -> Task.succeed (),
  [t, ..rest] -> {
    let _ <- Db.insert Entry { text: Text.NonEmpty t }
    addEntries rest
  },
}

test "scrolling twice through 25 entries shows each once, newest first" = {
  let _ <- addEntries (List.map (\n -> "entry " ++ Text.fromInt n) (List.range 1 25))
  let _ <- Test.get (Index {})
  let _ <- Test.intersect (More { after: 10 })
  let page <- Test.intersect (More { after: 20 })
  Task.succeed (
    {
      texts: Test.texts "li" page.doc,
      sentinel: Test.has "[data-on-intersect__once]" page.doc,
    } == {
      texts: List.map (\n -> "entry " ++ Text.fromInt n) (List.reverse (List.range 1 25)),
      sentinel: False,
    }
  )
}

test "a feed of exactly ten entries has no sentinel" = {
  let _ <- addEntries (List.map (\n -> "entry " ++ Text.fromInt n) (List.range 1 10))
  let page <- Test.get (Index {})
  Task.succeed (
    { rows: Test.count "li" page.doc, sentinel: Test.has "[data-on-intersect__once]" page.doc }
      == { rows: 10, sentinel: False }
  )
}

test "posting an entry puts it first" = {
  let _ <- addEntries ["older"]
  let _ <- Test.get (Index {})
  let r <- Test.submit (Post {}) { text: Text.NonEmpty "newest" }
  Task.succeed (
    { answer: r.answer, texts: Test.texts "li" r.doc }
      == { answer: Redirected (Index {}), texts: ["newest", "older"] }
  )
}
