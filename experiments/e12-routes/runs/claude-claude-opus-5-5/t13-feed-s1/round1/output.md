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
route Post  = POST "/feed"      { }            form NewEntry

-- `after` is the number of entries shown before this page; `more` says whether any follow it.
type Model = {
  after: Int,
  entries: List Entry,
  more: Bool,
}

form NewEntry = { text: Text.NonEmpty }

-- Ten entries after the first `after`, newest first; an eleventh row tells whether more follow.
entriesAfter : Int -> Query Model
entriesAfter after = {
  let rows <-
    Db.from Entry
      |> Db.orderBy Entry.created.desc
      |> Db.offset after
      |> Db.limit 11
      |> Db.all
  Query.succeed {
    after: after,
    entries: List.take 10 rows,
    more: List.length rows > 10,
  }
}

-- The element that asks for the entries after this page's; none after the last entry.
sentinel : Model -> Html
sentinel model =
  if model.more then
    <div
      id={"more-" ++ Text.fromInt (model.after + 10)}
      class="sentinel"
      intersect={More { after: model.after + 10 }}
    ></div>
  else
    Html.none

Index.load : Index.Params -> Session -> Query Model
Index.load params session = entriesAfter 0

Index.view : Model -> Session -> Html
Index.view model session =
  <Layout.Page title="Feed">
    <h1>Feed</h1>
    <al-card>
      <form submit={Post}>
        <al-input field="text" label="What is happening?" />
        <al-button type="submit">Post</al-button>
      </form>
    </al-card>
    <ul>{for e in model.entries -> <li>{e.text}</li>}</ul>
    {sentinel model}
  </Layout.Page>

More.load : More.Params -> Session -> Query Model
More.load params session = entriesAfter params.after

More.view : Model -> Session -> Html
More.view model session =
  <div id={"more-" ++ Text.fromInt model.after}>
    <ul>{for e in model.entries -> <li>{e.text}</li>}</ul>
    {sentinel model}
  </div>

Post.update : Post.Params -> Session -> Task Response
Post.update p session = {
  let _ <- Db.insert Entry { text: p.form.text }
  Task.succeed (Response.redirect (Index {}))
}

test "scrolling twice shows all 25 entries once, newest first, and stops" = {
  let inserts = List.map (\n -> Db.insert Entry { text: Text.NonEmpty "Entry {Text.fromInt n}" }) (List.range 1 25)
  let _ <- Task.all inserts
  let first <- Test.get (Index {})
  let _ <- Test.intersect (More { after: 10 })
  let r <- Test.intersect (More { after: 20 })
  Task.succeed (
    {
      firstItems: Test.count "li" first.doc,
      firstSentinel: Test.has ".sentinel" first.doc,
      texts: Test.texts "li" r.doc,
      sentinel: Test.has ".sentinel" r.doc,
    } == {
      firstItems: 10,
      firstSentinel: True,
      texts: List.map (\n -> "Entry {Text.fromInt n}") (List.reverse (List.range 1 25)),
      sentinel: False,
    }
  )
}

test "a feed of exactly ten entries has no sentinel" = {
  let inserts = List.map (\n -> Db.insert Entry { text: Text.NonEmpty "Entry {Text.fromInt n}" }) (List.range 1 10)
  let _ <- Task.all inserts
  let r <- Test.get (Index {})
  Task.succeed (
    {
      items: Test.count "li" r.doc,
      sentinel: Test.has ".sentinel" r.doc,
      more: r.model.more,
    } == {
      items: 10,
      sentinel: False,
      more: False,
    }
  )
}

test "posting an entry puts it first" = {
  let _ <- Db.insert Entry { text: Text.NonEmpty "Older" }
  let _ <- Db.insert Entry { text: Text.NonEmpty "Old" }
  let _ <- Test.get (Index {})
  let posted <- Test.submit (Post {}) { text: Text.NonEmpty "Newest" }
  let r <- Test.get (Index {})
  Task.succeed (
    { answer: posted.answer, texts: Test.texts "li" r.doc }
      == { answer: Redirected (Index {}), texts: ["Newest", "Old", "Older"] }
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
