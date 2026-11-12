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

table Booking {
  id: BookingId @id,
  room: Room.RoomId @ref Room,
  owner: User.UserId @ref User,
  from: Date,
  to: Date,
}

-- A date counted from the epoch: a small helper for building dates.
day : Int -> Date
day n = Date.addDays n Date.epoch

-- The first date, today or later, that none of the bookings holds.
-- A booking holds its `from` date up to, but not including, its `to` date.
nextFree : Date -> List Booking -> Date
nextFree today bookings =
  bookings
    |> List.sortByText (\(b : Booking) -> Date.toText b.from)
    |> List.foldl (\(b : Booking) (c : Date) -> if b.from <= c && c < b.to then b.to else c) today

mk : Int -> Int -> Booking
mk a b = {
  id: BookingId "x",
  room: Room.RoomId "r",
  owner: User.UserId "u",
  from: day a,
  to: day b,
}

test "next free date skips back-to-back bookings" =
  {
    skipped: nextFree (day 100) [mk 100 103, mk 103 105, mk 110 112],
    past: nextFree (day 100) [mk 90 100],
    later: nextFree (day 100) [mk 101 103],
    none: nextFree (day 100) [],
  } == {
    skipped: day 105,
    past: day 100,
    later: day 100,
    none: day 100,
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

type Model = { today: Date, rooms: List Room, bookings: List Booking }

Index.load : Index.Params -> Session -> Query Model
Index.load params session = {
  let today <- Date.today
  let rooms <-
    Db.from Room
      |> Db.orderBy Room.name.asc
      |> Db.all
  let bookings <- Db.from Booking |> Db.all
  Query.succeed { today: today, rooms: rooms, bookings: bookings }
}

freeDate : Model -> Room -> Text
freeDate model r =
  Date.toText (Booking.nextFree model.today (List.filter (\b -> b.room == r.id) model.bookings))

Index.view : Model -> Session -> Html
Index.view model session =
  <Layout.Page title="Rooms">
    <h1>Rooms</h1>
    <ul id="rooms">
      {for r in model.rooms ->
        <li>
          <a class="name" href={Rooms.Show.Show { id: r.id }}>{r.name}</a>
          <span class="free">{freeDate model r}</span>
        </li>}
    </ul>
    <nav>
      <a href={Bookings.Index.Index {}}>My bookings</a>
    </nav>
    <al-button variant="outline" click={App.Logout}>Log out</al-button>
  </Layout.Page>

test "rooms list their next free date" = {
  let me <- Db.insert User {
    email: Text.Email "me@example.com",
    name: Text.NonEmpty "Me",
    passwordHash: "",
  }
  let a <- Db.insert Room { name: Text.NonEmpty "Alpha" }
  let _ <- Db.insert Room { name: Text.NonEmpty "Beta" }
  let _ <- Db.insert Booking { room: a, owner: me, from: Booking.day 100, to: Booking.day 103 }
  let _ <- Db.insert Booking { room: a, owner: me, from: Booking.day 103, to: Booking.day 105 }
  let _ <- Test.setClock (Time.fromDate (Booking.day 100))
  let _ <- Test.login { user: Just me }
  let page <- Test.get (Index {})
  Task.succeed (
    {
      names: Test.texts "#rooms .name" page.doc,
      free: Test.texts "#rooms .free" page.doc,
    } == {
      names: ["Alpha", "Beta"],
      free: [Date.toText (Booking.day 105), Date.toText (Booking.day 100)],
    }
  )
}

--- file: app/Rooms/Show.alder ---
module Rooms.Show

import Layout
import Table.Booking
import Table.Room
import Table.User

route Show = GET  "/rooms/{id}" { id: Room.RoomId }                 requires user
route Book = POST "/rooms/{id}" { id: Room.RoomId } form NewBooking requires user

type Model = { room: Room, bookings: List Booking }

form NewBooking = { from: Date, to: Date }

clashMessage : Text
clashMessage = "This room is already booked for part of that period."

rangeMessage : Text
rangeMessage = "The end date must be after the start date."

Show.load : Show.Params -> Session -> Query Model
Show.load params session = {
  let found <- Db.get Room params.id
  match found {
    Just room -> {
      let bookings <-
        Db.from Booking
          |> Db.where (Booking.room.eq params.id)
          |> Db.orderBy Booking.from.asc
          |> Db.all
      Query.succeed { room: room, bookings: bookings }
    },
    Nothing -> Query.notFound,
  }
}

Book.update : Book.Params -> Session -> Task Response
Book.update p session = {
  let found <- Db.get Room p.id
  match found {
    Nothing -> Task.notFound,
    Just room ->
      if p.form.to <= p.form.from then
        Task.succeed (Response.invalid rangeMessage)
      else {
        let clashes <-
          Db.from Booking
            |> Db.where (Booking.room.eq p.id)
            |> Db.where (Booking.from.lt p.form.to)
            |> Db.where (Booking.to.gt p.form.from)
            |> Db.count
        if clashes > 0 then
          Task.succeed (Response.invalid clashMessage)
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
    <a href={Rooms.Index.Index {}}>All rooms</a>
  </Layout.Page>

test "an overlapping booking is refused, a back-to-back one is stored" = {
  let me <- Db.insert User {
    email: Text.Email "me@example.com",
    name: Text.NonEmpty "Me",
    passwordHash: "",
  }
  let room <- Db.insert Room { name: Text.NonEmpty "Alpha" }
  let _ <- Db.insert Booking { room: room, owner: me, from: Booking.day 100, to: Booking.day 103 }
  let _ <- Test.login { user: Just me }
  let _ <- Test.get (Show { id: room })
  let bad <- Test.submit (Book { id: room }) { from: Booking.day 102, to: Booking.day 105 }
  let n1 <- Db.from Booking |> Db.count
  let ok <- Test.submit (Book { id: room }) { from: Booking.day 103, to: Booking.day 105 }
  let n2 <- Db.from Booking |> Db.count
  Task.succeed (
    {
      bad: bad.answer,
      n1: n1,
      ok: ok.answer,
      n2: n2,
    } == {
      bad: FormRejected [{ field: "", message: clashMessage }],
      n1: 1,
      ok: Redirected (Show { id: room }),
      n2: 2,
    }
  )
}

test "bookings of different rooms never conflict" = {
  let me <- Db.insert User {
    email: Text.Email "me@example.com",
    name: Text.NonEmpty "Me",
    passwordHash: "",
  }
  let a <- Db.insert Room { name: Text.NonEmpty "Alpha" }
  let b <- Db.insert Room { name: Text.NonEmpty "Beta" }
  let _ <- Db.insert Booking { room: a, owner: me, from: Booking.day 100, to: Booking.day 103 }
  let _ <- Test.login { user: Just me }
  let _ <- Test.get (Show { id: b })
  let r <- Test.submit (Book { id: b }) { from: Booking.day 100, to: Booking.day 103 }
  let n <- Db.from Booking |> Db.count
  Task.succeed ({ answer: r.answer, n: n } == { answer: Redirected (Show { id: b }), n: 2 })
}

test "a booking must end after it starts" = {
  let me <- Db.insert User {
    email: Text.Email "me@example.com",
    name: Text.NonEmpty "Me",
    passwordHash: "",
  }
  let room <- Db.insert Room { name: Text.NonEmpty "Alpha" }
  let _ <- Test.login { user: Just me }
  let _ <- Test.get (Show { id: room })
  let same <- Test.submit (Book { id: room }) { from: Booking.day 105, to: Booking.day 105 }
  let n <- Db.from Booking |> Db.count
  Task.succeed (
    { answer: same.answer, n: n }
      == { answer: FormRejected [{ field: "", message: rangeMessage }], n: 0 }
  )
}

--- file: app/Bookings/Index.alder ---
module Bookings.Index

import Layout
import Table.Booking
import Table.Room
import Table.User

route Index = GET "/bookings" { } requires user

type Model = { bookings: List Booking, rooms: List Room }

Index.load : Index.Params -> Session -> Query Model
Index.load params session = {
  let bookings <-
    Db.from Booking
      |> Db.where (Booking.owner.eq session.user)
      |> Db.orderBy Booking.from.asc
      |> Db.all
  let rooms <- Db.from Room |> Db.all
  Query.succeed { bookings: bookings, rooms: rooms }
}

roomName : List Room -> Booking -> Text
roomName rooms b = {
  let found = List.find (\r -> r.id == b.room) rooms
  Maybe.withDefault "" (Maybe.map (\r -> Text.trim r.name) found)
}

Index.view : Model -> Session -> Html
Index.view model session =
  <Layout.Page title="My bookings">
    <h1>My bookings</h1>
    <ul id="mine">
      {for b in model.bookings ->
        <li>
          <a href={Bookings.Show.Show { id: b.id }}>
            <span class="room">{roomName model.rooms b}</span>
          </a>
          <span class="from">{Date.toText b.from}</span>
        </li>}
    </ul>
    <nav>
      <a href={Rooms.Index.Index {}}>Rooms</a>
    </nav>
    <al-button variant="outline" click={App.Logout}>Log out</al-button>
  </Layout.Page>

test "only my bookings are listed, by from date" = {
  let me <- Db.insert User {
    email: Text.Email "me@example.com",
    name: Text.NonEmpty "Me",
    passwordHash: "",
  }
  let other <- Db.insert User {
    email: Text.Email "other@example.com",
    name: Text.NonEmpty "Other",
    passwordHash: "",
  }
  let a <- Db.insert Room { name: Text.NonEmpty "Alpha" }
  let b <- Db.insert Room { name: Text.NonEmpty "Beta" }
  let _ <- Db.insert Booking { room: b, owner: me, from: Booking.day 200, to: Booking.day 201 }
  let _ <- Db.insert Booking { room: a, owner: me, from: Booking.day 100, to: Booking.day 101 }
  let _ <- Db.insert Booking { room: a, owner: other, from: Booking.day 150, to: Booking.day 151 }
  let _ <- Test.login { user: Just me }
  let page <- Test.get (Index {})
  Task.succeed (
    {
      rooms: Test.texts "#mine .room" page.doc,
      froms: Test.texts "#mine .from" page.doc,
    } == {
      rooms: ["Alpha", "Beta"],
      froms: [Date.toText (Booking.day 100), Date.toText (Booking.day 200)],
    }
  )
}

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
  let today <- Date.today
  match found {
    Just b -> if b.owner == session.user then withRoom b today else Query.notFound,
    Nothing -> Query.notFound,
  }
}

withRoom : Booking -> Date -> Query Model
withRoom b today = {
  let room <- Db.get Room b.room
  match room {
    Just r -> Query.succeed { booking: b, room: r, today: today },
    Nothing -> Query.notFound,
  }
}

Cancel.update : Cancel.Params -> Session -> Task Response
Cancel.update p session = {
  let found <- Db.get Booking p.id
  let today <- Date.today
  match found {
    Nothing -> Task.notFound,
    Just b ->
      if b.owner != session.user then
        Task.notFound
      else if today < b.from then {
        let _ <- Db.delete Booking b.id
        Task.succeed (Response.redirect (Bookings.Index.Index {}))
      } else
        Task.succeed (Response.refresh |> Response.error "This booking has started and cannot be cancelled."),
  }
}

Show.view : Model -> Session -> Html
Show.view model session =
  <Layout.Page title="Booking">
    <h1>Booking</h1>
    <al-card>
      <p id="room">{model.room.name}</p>
      <p id="from">{Date.toText model.booking.from}</p>
      <p id="to">{Date.toText model.booking.to}</p>
      {if model.today < model.booking.from then
        <button id="cancel" click={Cancel { id: model.booking.id }}>Cancel</button>
      else
        Html.none}
    </al-card>
    <a href={Bookings.Index.Index {}}>My bookings</a>
  </Layout.Page>

test "the owner cancels a booking before its from date" = {
  let me <- Db.insert User {
    email: Text.Email "me@example.com",
    name: Text.NonEmpty "Me",
    passwordHash: "",
  }
  let room <- Db.insert Room { name: Text.NonEmpty "Alpha" }
  let id <- Db.insert Booking { room: room, owner: me, from: Booking.day 100, to: Booking.day 102 }
  let _ <- Test.setClock (Time.fromDate (Booking.day 99))
  let _ <- Test.login { user: Just me }
  let page <- Test.get (Show { id: id })
  let r <- Test.click (Cancel { id: id })
  let after <- Db.get Booking id
  Task.succeed (
    {
      shown: Test.has "#cancel" page.doc,
      answer: r.answer,
      gone: not (Maybe.isJust after),
    } == {
      shown: True,
      answer: Redirected (Bookings.Index.Index {}),
      gone: True,
    }
  )
}

test "a booking whose from date is today has no Cancel button" = {
  let me <- Db.insert User {
    email: Text.Email "me@example.com",
    name: Text.NonEmpty "Me",
    passwordHash: "",
  }
  let room <- Db.insert Room { name: Text.NonEmpty "Alpha" }
  let id <- Db.insert Booking { room: room, owner: me, from: Booking.day 100, to: Booking.day 102 }
  let _ <- Test.setClock (Time.fromDate (Booking.day 100))
  let _ <- Test.login { user: Just me }
  let page <- Test.get (Show { id: id })
  Task.succeed (
    {
      cancel: Test.has "#cancel" page.doc,
      room: Test.text "#room" page.doc,
      from: Test.text "#from" page.doc,
      to: Test.text "#to" page.doc,
    } == {
      cancel: False,
      room: Just "Alpha",
      from: Just (Date.toText (Booking.day 100)),
      to: Just (Date.toText (Booking.day 102)),
    }
  )
}

test "a Cancel received on the from date is refused" = {
  let me <- Db.insert User {
    email: Text.Email "me@example.com",
    name: Text.NonEmpty "Me",
    passwordHash: "",
  }
  let room <- Db.insert Room { name: Text.NonEmpty "Alpha" }
  let id <- Db.insert Booking { room: room, owner: me, from: Booking.day 100, to: Booking.day 102 }
  let _ <- Test.setClock (Time.fromDate (Booking.day 99))
  let _ <- Test.login { user: Just me }
  let _ <- Test.get (Show { id: id })
  let _ <- Test.setClock (Time.fromDate (Booking.day 100))
  let r <- Test.click (Cancel { id: id })
  let after <- Db.get Booking id
  Task.succeed ({ answer: r.answer, kept: Maybe.isJust after } == { answer: Refreshed, kept: True })
}

test "another user's booking is not found" = {
  let me <- Db.insert User {
    email: Text.Email "me@example.com",
    name: Text.NonEmpty "Me",
    passwordHash: "",
  }
  let room <- Db.insert Room { name: Text.NonEmpty "Alpha" }
  let id <- Db.insert Booking { room: room, owner: me, from: Booking.day 100, to: Booking.day 102 }
  let _ <- Test.login { user: Just (User.UserId "someone-else") }
  let r <- Test.open (Show { id: id })
  Task.succeed (r.answer == PageMissing)
}
