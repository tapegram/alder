--- file: app/Table/User.alder ---
module Table.User

type UserId = brand Text

table User {
  id: UserId @id,
  email: Text.Email @unique,
  name: Text.NonEmpty,
  passwordHash: Text,
  created: Time @created,
}

--- file: app/Table/Room.alder ---
module Table.Room

type RoomId = brand Text

table Room {
  id: RoomId @id,
  name: Text.NonEmpty,
}

--- file: app/Table/Booking.alder ---
module Table.Booking

import Table.Room
import Table.User

type BookingId = brand Text

-- A booking holds its room from `from` up to, but not including, `to`.
table Booking {
  id: BookingId @id,
  room: Room.RoomId @ref Room,
  owner: User.UserId @ref User,
  from: Date,
  to: Date,
}

day : Text -> Date
day s = Date.fromText s ?? Date.epoch

-- The first date, `today` or later, that none of the bookings holds.
nextFree : Date -> List Booking -> Date
nextFree today bookings =
  bookings
    |> List.sortBy (\b -> Date.diffDays Date.epoch b.from)
    |> List.foldl (\b c -> if b.from <= c && c < b.to then b.to else c) today

mk : Room.RoomId -> User.UserId -> Text -> Text -> Booking
mk room owner a b = { id: BookingId "x", room: room, owner: owner, from: day a, to: day b }

test "next free date with back-to-back bookings" =
  nextFree (day "2026-03-01") [
    mk (Room.RoomId "r") (User.UserId "u") "2026-03-03" "2026-03-05",
    mk (Room.RoomId "r") (User.UserId "u") "2026-03-01" "2026-03-03",
  ] == day "2026-03-05"

test "next free date is today when nothing holds it" =
  nextFree (day "2026-02-20") [mk (Room.RoomId "r") (User.UserId "u") "2026-03-01" "2026-03-03"]
    == day "2026-02-20"

test "next free date skips past bookings" =
  nextFree (day "2026-03-10") [mk (Room.RoomId "r") (User.UserId "u") "2026-03-01" "2026-03-03"]
    == day "2026-03-10"

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
      <al-page width="narrow">
        <main>
          <al-stack gap="lg">
            <nav>
              <a href={Rooms.Index.Index {}}>Rooms</a>
              <a href={Bookings.Index.Index {}}>My bookings</a>
            </nav>
            {props.children}
          </al-stack>
        </main>
      </al-page>
    </body>
  </html>

--- file: app/App.alder ---
module App

import Layout
import Table.User

session { user: Maybe User.UserId }

login = Auth.Login.Login

route Logout = DELETE "/session" { }

Logout.update : Logout.Params -> Session -> Task Response
Logout.update p session = {
  let _ <- Session.put (\s -> { s | user: Nothing })
  Task.succeed (Response.redirect (Auth.Login.Login {}))
}

error : Error -> Html
error err =
  <Layout.Page title="Error">
    <h1>Something went wrong</h1>
    <p>Please try again.</p>
  </Layout.Page>

notFound : Html
notFound = <Layout.Page title="Not found"><h1>Not found</h1></Layout.Page>

test "Logout clears the user and goes to log in" = {
  let r <- Test.run (Logout.update {} { user: Just (User.UserId "me") })
  Task.succeed (
    { value: r.value, trace: r.trace }
      == { value: Ok (Response.redirect (Auth.Login.Login {})), trace: ["Session.put"] }
  )
}

--- file: app/Auth/Login.alder ---
module Auth.Login

import Layout
import Table.User

capability Password {
  verify : { email: Text.Email, password: Text } -> Task (Maybe User.UserId),
}

fake Password {
  verify args = {
    let found <-
      Db.from User
        |> Db.where (User.email.eq args.email)
        |> Db.first
    Task.succeed (Maybe.map (\u -> u.id) found)
  }
}

route Login  = GET  "/login"   { }
route Submit = POST "/session" { } form Credentials

type Model = { }

Login.load : Login.Params -> Session -> Query Model
Login.load params session = Query.succeed {}

form Credentials = { email: Text.Email, password: Text.Secret }

Submit.update : Submit.Params -> Session -> Task Response
Submit.update p session = {
  let found <- Password.verify { email: p.form.email, password: p.form.password }
  match found {
    Just id -> {
      let _ <- Session.put (\s -> { s | user: Just id })
      Task.succeed (Response.redirect (Rooms.Index.Index {}))
    },
    Nothing -> Task.succeed (Response.invalid "Email or password is incorrect."),
  }
}

Login.view : Model -> Session -> Html
Login.view model session =
  <Layout.Page title="Log in">
    <h1>Log in</h1>
    <al-card>
      <form submit={Submit}>
        <Errors form />
        <al-input field="email" label="Email" />
        <al-input field="password" label="Password" />
        <al-button type="submit">Log in</al-button>
      </form>
    </al-card>
  </Layout.Page>

test "an unknown email is rejected" = {
  let _ <- Test.fake Password
  let _ <- Test.get (Login {})
  let r <- Test.submit Submit {
    email: Text.Email "nobody@example.com",
    password: Text.Secret "secret",
  }
  Task.succeed (
    { answer: r.answer, user: r.session.user }
      == {
        answer: FormRejected [{ field: "", message: "Email or password is incorrect." }],
        user: Nothing,
      }
  )
}

test "logging in through the form" = {
  let _ <- Test.fake Password
  let me <- Db.insert User {
    email: Text.Email "me@example.com",
    name: Text.NonEmpty "Me",
    passwordHash: "",
  }
  let _ <- Test.get (Login {})
  let r <- Test.submit Submit { email: Text.Email "me@example.com", password: Text.Secret "secret" }
  Task.succeed (
    { answer: r.answer, user: r.session.user }
      == { answer: Redirected (Rooms.Index.Index {}), user: Just me }
  )
}

--- file: app/Rooms/Index.alder ---
module Rooms.Index

import Layout
import Table.Booking
import Table.Room
import Table.User

route Index = GET "/rooms" { } requires user

type Row = { room: Room, free: Date }

type Model = { rows: List Row }

Index.load : Index.Params -> Session -> Query Model
Index.load params session = {
  let rooms <-
    Db.from Room
      |> Db.orderBy Room.name.asc
      |> Db.all
  let bookings <- Db.from Booking |> Db.all
  let today <- Date.today
  Query.succeed {
    rows: List.map (\r -> { room: r, free: Booking.nextFree today (List.filter (\b -> b.room == r.id) bookings) }) rooms,
  }
}

Index.view : Model -> Session -> Html
Index.view model session =
  <Layout.Page title="Rooms">
    <h1>Rooms</h1>
    <ul id="rooms">
      {for row in model.rows ->
        <li>
          <a class="name" href={Rooms.Show.Show { id: row.room.id }}>{row.room.name}</a>
          <span class="free">{Date.toText row.free}</span>
        </li>}
    </ul>
    <button click={App.Logout}>Log out</button>
  </Layout.Page>

test "rooms are listed with their next free date" = {
  let me <- Db.insert User {
    email: Text.Email "me@example.com",
    name: Text.NonEmpty "Me",
    passwordHash: "",
  }
  let a <- Db.insert Room { name: Text.NonEmpty "A" }
  let _ <- Db.insert Room { name: Text.NonEmpty "B" }
  let _ <- Db.insert Booking { room: a, owner: me, from: Booking.day "2026-03-01", to: Booking.day "2026-03-03" }
  let _ <- Db.insert Booking { room: a, owner: me, from: Booking.day "2026-03-03", to: Booking.day "2026-03-05" }
  let _ <- Test.setClock (Time.fromDate (Booking.day "2026-03-01"))
  let _ <- Test.login { user: Just me }
  let page <- Test.get (Index {})
  Task.succeed (
    { names: Test.texts ".name" page.doc, free: Test.texts ".free" page.doc }
      == { names: ["A", "B"], free: ["2026-03-05", "2026-03-01"] }
  )
}

--- file: app/Rooms/Show.alder ---
module Rooms.Show

import Layout
import Table.Booking
import Table.Room
import Table.User

route Show = GET  "/rooms/{id}"          { id: Room.RoomId }               requires user
route Book = POST "/rooms/{id}/bookings" { id: Room.RoomId } form NewBooking requires user

type Model = { room: Room, bookings: List Booking }

form NewBooking = { from: Date, to: Date }

Show.load : Show.Params -> Session -> Query Model
Show.load params session = {
  let found <- Db.get Room params.id
  match found {
    Nothing -> Query.notFound,
    Just room -> {
      let bookings <-
        Db.from Booking
          |> Db.where (Booking.room.eq params.id)
          |> Db.orderBy Booking.from.asc
          |> Db.all
      Query.succeed { room: room, bookings: bookings }
    },
  }
}

conflictMsg : Text
conflictMsg = "The room is already booked for part of that period."

orderMsg : Text
orderMsg = "The end date must be after the start date."

Book.update : Book.Params -> Session -> Task Response
Book.update p session = {
  let found <- Db.get Room p.id
  match found {
    Nothing -> Task.notFound,
    Just room ->
      if p.form.to <= p.form.from then
        Task.succeed (Response.invalid orderMsg)
      else {
        let clashes <-
          Db.from Booking
            |> Db.where (Booking.room.eq p.id)
            |> Db.where (Booking.from.lt p.form.to)
            |> Db.where (Booking.to.gt p.form.from)
            |> Db.count
        if clashes > 0 then
          Task.succeed (Response.invalid conflictMsg)
        else {
          let _ <- Db.insert Booking {
            room: p.id,
            owner: session.user,
            from: p.form.from,
            to: p.form.to,
          }
          Task.succeed (Response.redirect (Show { id: p.id }))
        }
      },
  }
}

Show.view : Model -> Session -> Html
Show.view model session =
  <Layout.Page title={model.room.name}>
    <h1 id="name">{model.room.name}</h1>
    <ul id="bookings">
      {for b in model.bookings ->
        <li>
          <span class="from">{Date.toText b.from}</span>
          <span class="to">{Date.toText b.to}</span>
        </li>}
    </ul>
    <al-card>
      <form submit={Book { id: model.room.id }}>
        <Errors form />
        <al-input field="from" label="From" />
        <al-input field="to" label="To" />
        <al-button type="submit">Book</al-button>
      </form>
    </al-card>
  </Layout.Page>

test "an overlapping booking is refused and not stored" = {
  let me <- Db.insert User {
    email: Text.Email "me@example.com",
    name: Text.NonEmpty "Me",
    passwordHash: "",
  }
  let room <- Db.insert Room { name: Text.NonEmpty "Alpha" }
  let _ <- Db.insert Booking { room: room, owner: me, from: Booking.day "2026-03-01", to: Booking.day "2026-03-03" }
  let _ <- Test.login { user: Just me }
  let _ <- Test.get (Show { id: room })
  let r <- Test.submit (Book { id: room }) { from: Booking.day "2026-03-02", to: Booking.day "2026-03-04" }
  let n <- Db.from Booking |> Db.count
  Task.succeed (
    { answer: r.answer, n: n }
      == { answer: FormRejected [{ field: "", message: conflictMsg }], n: 1 }
  )
}

test "a booking starting on another's end date is stored" = {
  let me <- Db.insert User {
    email: Text.Email "me@example.com",
    name: Text.NonEmpty "Me",
    passwordHash: "",
  }
  let room <- Db.insert Room { name: Text.NonEmpty "Alpha" }
  let _ <- Db.insert Booking { room: room, owner: me, from: Booking.day "2026-03-01", to: Booking.day "2026-03-03" }
  let _ <- Test.login { user: Just me }
  let _ <- Test.get (Show { id: room })
  let r <- Test.submit (Book { id: room }) { from: Booking.day "2026-03-03", to: Booking.day "2026-03-05" }
  let n <- Db.from Booking |> Db.count
  Task.succeed ({ answer: r.answer, n: n } == { answer: Redirected (Show { id: room }), n: 2 })
}

test "a booking that ends before it starts is refused" = {
  let me <- Db.insert User {
    email: Text.Email "me@example.com",
    name: Text.NonEmpty "Me",
    passwordHash: "",
  }
  let room <- Db.insert Room { name: Text.NonEmpty "Alpha" }
  let _ <- Test.login { user: Just me }
  let _ <- Test.get (Show { id: room })
  let r <- Test.submit (Book { id: room }) { from: Booking.day "2026-03-03", to: Booking.day "2026-03-03" }
  let n <- Db.from Booking |> Db.count
  Task.succeed (
    { answer: r.answer, n: n }
      == { answer: FormRejected [{ field: "", message: orderMsg }], n: 0 }
  )
}

test "bookings of different rooms do not conflict" = {
  let me <- Db.insert User {
    email: Text.Email "me@example.com",
    name: Text.NonEmpty "Me",
    passwordHash: "",
  }
  let a <- Db.insert Room { name: Text.NonEmpty "Alpha" }
  let b <- Db.insert Room { name: Text.NonEmpty "Beta" }
  let _ <- Db.insert Booking { room: a, owner: me, from: Booking.day "2026-03-01", to: Booking.day "2026-03-03" }
  let _ <- Test.login { user: Just me }
  let _ <- Test.get (Show { id: b })
  let r <- Test.submit (Book { id: b }) { from: Booking.day "2026-03-01", to: Booking.day "2026-03-03" }
  let n <- Db.from Booking |> Db.count
  Task.succeed ({ answer: r.answer, n: n } == { answer: Redirected (Show { id: b }), n: 2 })
}

--- file: app/Bookings/Index.alder ---
module Bookings.Index

import Layout
import Table.Booking
import Table.Room

route Index = GET "/bookings" { } requires user

type Row = { booking: Booking, roomName: Text }

type Model = { rows: List Row }

Index.load : Index.Params -> Session -> Query Model
Index.load params session = {
  let bookings <-
    Db.from Booking
      |> Db.where (Booking.owner.eq session.user)
      |> Db.orderBy Booking.from.asc
      |> Db.all
  let rooms <- Db.from Room |> Db.all
  Query.succeed {
    rows: List.map (\b -> {
      booking: b,
      roomName: Maybe.withDefault "" (Maybe.map (\r -> Text.trim r.name) (List.find (\r -> r.id == b.room) rooms)),
    }) bookings,
  }
}

Index.view : Model -> Session -> Html
Index.view model session =
  <Layout.Page title="My bookings">
    <h1>My bookings</h1>
    <ul id="mine">
      {for row in model.rows ->
        <li>
          <a href={Bookings.Show.Show { id: row.booking.id }}>
            <span class="room">{row.roomName}</span>
            <span class="from">{Date.toText row.booking.from}</span>
          </a>
        </li>}
    </ul>
  </Layout.Page>

--- file: app/Bookings/Show.alder ---
module Bookings.Show

import Layout
import Table.Booking
import Table.Room
import Table.User

route Show   = GET    "/bookings/{id}" { id: Booking.BookingId } requires user
route Cancel = DELETE "/bookings/{id}" { id: Booking.BookingId } requires user

type Model = { booking: Booking, room: Room, today: Date }

Show.load : Show.Params -> Session -> Query Model
Show.load params session = {
  let found <- Db.get Booking params.id
  match found {
    Nothing -> Query.notFound,
    Just b ->
      if b.owner != session.user then
        Query.notFound
      else {
        let room <- Db.get Room b.room
        let today <- Date.today
        match room {
          Nothing -> Query.notFound,
          Just r -> Query.succeed { booking: b, room: r, today: today },
        }
      },
  }
}

Cancel.update : Cancel.Params -> Session -> Task Response
Cancel.update p session = {
  let found <- Db.get Booking p.id
  match found {
    Nothing -> Task.notFound,
    Just b ->
      if b.owner != session.user then
        Task.notFound
      else {
        let today <- Date.today
        if today < b.from then {
          let _ <- Db.delete Booking b.id
          Task.succeed (Response.redirect (Bookings.Index.Index {}))
        } else
          Task.succeed (Response.refresh |> Response.error "This booking can no longer be cancelled.")
      },
  }
}

Show.view : Model -> Session -> Html
Show.view model session =
  <Layout.Page title="Booking">
    <h1>Booking</h1>
    <p>Room: <span id="room">{model.room.name}</span></p>
    <p>From: <span id="from">{Date.toText model.booking.from}</span></p>
    <p>To: <span id="to">{Date.toText model.booking.to}</span></p>
    {if model.today < model.booking.from then
      <button id="cancel" click={Cancel { id: model.booking.id }}>Cancel</button>
    else
      Html.none}
  </Layout.Page>

test "the owner cancels a booking before its from date" = {
  let me <- Db.insert User {
    email: Text.Email "me@example.com",
    name: Text.NonEmpty "Me",
    passwordHash: "",
  }
  let room <- Db.insert Room { name: Text.NonEmpty "Alpha" }
  let id <- Db.insert Booking { room: room, owner: me, from: Booking.day "2026-03-01", to: Booking.day "2026-03-03" }
  let _ <- Test.setClock (Time.fromDate (Booking.day "2026-02-28"))
  let _ <- Test.login { user: Just me }
  let page <- Test.get (Show { id: id })
  let r <- Test.click (Cancel { id: id })
  let after <- Db.get Booking id
  Task.succeed (
    {
      cancel: Test.has "#cancel" page.doc,
      answer: r.answer,
      gone: Maybe.isJust after,
    } == {
      cancel: True,
      answer: Redirected (Bookings.Index.Index {}),
      gone: False,
    }
  )
}

test "a booking whose from date is today cannot be cancelled" = {
  let me <- Db.insert User {
    email: Text.Email "me@example.com",
    name: Text.NonEmpty "Me",
    passwordHash: "",
  }
  let room <- Db.insert Room { name: Text.NonEmpty "Alpha" }
  let id <- Db.insert Booking { room: room, owner: me, from: Booking.day "2026-03-01", to: Booking.day "2026-03-03" }
  let _ <- Test.setClock (Time.fromDate (Booking.day "2026-03-01"))
  let _ <- Test.login { user: Just me }
  let page <- Test.get (Show { id: id })
  let _ <- Test.run (Cancel.update { id: id } { user: me })
  let after <- Db.get Booking id
  Task.succeed (
    { cancel: Test.has "#cancel" page.doc, kept: Maybe.isJust after }
      == { cancel: False, kept: True }
  )
}

test "another user's booking is not found" = {
  let me <- Db.insert User {
    email: Text.Email "me@example.com",
    name: Text.NonEmpty "Me",
    passwordHash: "",
  }
  let room <- Db.insert Room { name: Text.NonEmpty "Alpha" }
  let id <- Db.insert Booking { room: room, owner: me, from: Booking.day "2026-03-01", to: Booking.day "2026-03-03" }
  let _ <- Test.login { user: Just (User.UserId "someone-else") }
  let page <- Test.open (Show { id: id })
  Task.succeed (page.answer == PageMissing)
}
