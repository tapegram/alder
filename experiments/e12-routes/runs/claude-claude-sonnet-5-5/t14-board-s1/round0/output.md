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

test "a new ticket is pending" = {
  let id <- Db.insert Ticket { title: Text.NonEmpty "Printer jam" }
  let found <- Db.get Ticket id
  Task.succeed (Maybe.map (\t -> t.status) found == Just Pending)
}

--- file: app/Board.alder ---
module Board

import Layout
import Table.Ticket

route Board  = GET "/board"        { }
route Recent = GET "/board/recent" { }
route Open   = GET "/board/open"   { }
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
    <ul id="pending">
      {for t in model.tickets ->
        <li id={"ticket-" ++ t.id}>
          <span>{t.title}</span>
          <button click={Close { id: t.id }}>Close</button>
        </li>}
    </ul>
    <section id="recent" load={Recent {}}>Loading…</section>
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
    <ul>{for t in tickets -> <li>{t.title}</li>}</ul>
  </section>

Open.load : Open.Params -> Session -> Query Int
Open.load params session =
  Db.from Ticket
    |> Db.where (Ticket.status.eq Pending)
    |> Db.count

Open.view : Int -> Session -> Html
Open.view n session = <span id="open" load={Open {}} every="10s">{Text.fromInt n}</span>

Close.update : Close.Params -> Session -> Task Response
Close.update p session = {
  let _ <- Db.update Ticket p.id (\t -> { t | status: Closed })
  Task.succeed Response.refresh
}

test "the badge counts the pending tickets" = {
  let _ <- Test.get (Board {})
  let none <- Test.load (Open {})
  let a <- Db.insert Ticket { title: Text.NonEmpty "A" }
  let _ <- Db.insert Ticket { title: Text.NonEmpty "B" }
  let two <- Test.load (Open {})
  let _ <- Db.insert Ticket { title: Text.NonEmpty "C" }
  let three <- Test.load (Open {})
  let _ <- Test.get (Board {})
  let _ <- Test.click (Close { id: a })
  let after <- Test.load (Open {})
  Task.succeed (
    {
      none: Test.text "#open" none.doc,
      two: Test.text "#open" two.doc,
      three: Test.text "#open" three.doc,
      after: Test.text "#open" after.doc,
    } == {
      none: Just "0",
      two: Just "2",
      three: Just "3",
      after: Just "2",
    }
  )
}

test "recent lists the five newest of six, newest first" = {
  let _ <- Db.insert Ticket { title: Text.NonEmpty "T1" }
  let _ <- Db.insert Ticket { title: Text.NonEmpty "T2" }
  let _ <- Db.insert Ticket { title: Text.NonEmpty "T3" }
  let _ <- Db.insert Ticket { title: Text.NonEmpty "T4" }
  let _ <- Db.insert Ticket { title: Text.NonEmpty "T5" }
  let _ <- Db.insert Ticket { title: Text.NonEmpty "T6" }
  let _ <- Test.get (Board {})
  let r <- Test.load (Recent {})
  Task.succeed (Test.texts "#recent li" r.doc == ["T6", "T5", "T4", "T3", "T2"])
}

test "closing a ticket through its button refreshes the board" = {
  let id <- Db.insert Ticket { title: Text.NonEmpty "Printer jam" }
  let before <- Test.get (Board {})
  let r <- Test.click (Close { id: id })
  let after <- Db.get Ticket id
  Task.succeed (
    {
      listed: Test.count "#pending li" before.doc,
      answer: r.answer,
      left: Test.count "#pending li" r.doc,
      status: Maybe.map (\t -> t.status) after,
    } == {
      listed: 1,
      answer: Refreshed,
      left: 0,
      status: Just Closed,
    }
  )
}
