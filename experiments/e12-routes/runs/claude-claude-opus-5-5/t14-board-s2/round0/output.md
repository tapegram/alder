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

--- file: app/Table/Ticket.alder ---
module Table.Ticket

type TicketId = brand Text

type Status = Pending | Closed

table Ticket {
  id: TicketId @id,
  title: Text.NonEmpty,
  status: Status = Pending,
  created: Time @created,
}

--- file: app/Board.alder ---
module Board

import Layout
import Table.Ticket

route Board  = GET "/board"                     { }
route Recent = GET "/board/recent"              { }
route Open   = GET "/board/open"                { }
route Close  = PUT "/board/tickets/{id}/closed" { id: Ticket.TicketId }

type Model = { tickets: List Ticket }

Board.load : Board.Params -> Session -> Query Model
Board.load params session = {
  let tickets <-
    Db.from Ticket
      |> Db.where (Ticket.status.eq Pending)
      |> Db.orderBy Ticket.created.desc
      |> Db.all
  Query.succeed { tickets: tickets }
}

Board.view : Model -> Session -> Html
Board.view model session =
  <Layout.Page title="Support board">
    <h1>Support board</h1>
    <p>Pending: <span id="open" load={Open {}} every="10s">…</span></p>
    <h2>Newest tickets</h2>
    <section id="recent" load={Recent {}}>Loading…</section>
    <h2>Pending tickets</h2>
    <div id="pending">
      {for t in model.tickets ->
        <div class="ticket">
          <span class="title">{t.title}</span>
          <al-button variant="secondary" size="sm" click={Close { id: t.id }}>Close</al-button>
        </div>}
    </div>
  </Layout.Page>

Recent.load : Recent.Params -> Session -> Query (List Ticket)
Recent.load params session =
  Db.from Ticket
    |> Db.orderBy Ticket.created.desc
    |> Db.limit 5
    |> Db.all

Recent.view : List Ticket -> Session -> Html
Recent.view tickets session =
  <section id="recent">
    {if List.isEmpty tickets then Html.none else <ul>{for t in tickets -> <li>{t.title}</li>}</ul>}
  </section>

Open.load : Open.Params -> Session -> Query Int
Open.load params session =
  Db.from Ticket
    |> Db.where (Ticket.status.eq Pending)
    |> Db.count

-- The badge renders its own `load` and `every` again: the timer keeps running.
Open.view : Int -> Session -> Html
Open.view n session = <span id="open" load={Open {}} every="10s">{Text.fromInt n}</span>

Close.update : Close.Params -> Session -> Task Response
Close.update p session = {
  let _ <- Db.update Ticket p.id (\t -> { t | status: Closed })
  Task.succeed Response.refresh
}

test "the badge counts pending tickets, again after an add and a close" = {
  let a <- Db.insert Ticket { title: Text.NonEmpty "Printer jam" }
  let _ <- Db.insert Ticket { title: Text.NonEmpty "VPN down" }
  let _ <- Db.insert Ticket { title: Text.NonEmpty "Old one", status: Closed }
  let _ <- Test.get (Board {})
  let first <- Test.load (Open {})
  let _ <- Db.insert Ticket { title: Text.NonEmpty "New laptop" }
  let added <- Test.load (Open {})
  let _ <- Test.click (Close { id: a })
  let closed <- Test.load (Open {})
  Task.succeed (
    {
      first: Test.text "#open" first.doc,
      added: Test.text "#open" added.doc,
      closed: Test.text "#open" closed.doc,
    } == {
      first: Just "2",
      added: Just "3",
      closed: Just "2",
    }
  )
}

test "recent lists the five newest of six, newest first" = {
  let _ <- Db.insert Ticket { title: Text.NonEmpty "T1" }
  let _ <- Db.insert Ticket { title: Text.NonEmpty "T2" }
  let _ <- Db.insert Ticket { title: Text.NonEmpty "T3", status: Closed }
  let _ <- Db.insert Ticket { title: Text.NonEmpty "T4" }
  let _ <- Db.insert Ticket { title: Text.NonEmpty "T5" }
  let _ <- Db.insert Ticket { title: Text.NonEmpty "T6" }
  let _ <- Test.get (Board {})
  let r <- Test.load (Recent {})
  Task.succeed (Test.texts "#recent li" r.doc == ["T6", "T5", "T4", "T3", "T2"])
}

test "closing a ticket through its button refreshes the board" = {
  let a <- Db.insert Ticket { title: Text.NonEmpty "Printer jam" }
  let _ <- Db.insert Ticket { title: Text.NonEmpty "VPN down" }
  let _ <- Test.get (Board {})
  let r <- Test.click (Close { id: a })
  let after <- Db.get Ticket a
  Task.succeed (
    {
      answer: r.answer,
      status: Maybe.map (\t -> t.status) after,
      pending: Test.texts "#pending .title" r.doc,
    } == {
      answer: Refreshed,
      status: Just Closed,
      pending: ["VPN down"],
    }
  )
}
