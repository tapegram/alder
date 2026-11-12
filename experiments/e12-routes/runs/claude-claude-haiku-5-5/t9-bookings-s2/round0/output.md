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

test "logging out signs the user out" = {
  let me <- Db.insert User {
    email: Text.Email "me@example.com",
    name: Text.NonEmpty "Me",
    passwordHash: "",
  }
  let _ <- Test.login { user: Just me }
  let _ <- Test.get (Rooms.Index.Index {})
  let out <- Test.click Logout
  Task.succeed (
    { answer: out.answer, user: out.session.user }
      == { answer: Redirected (Auth.Login.Login {}), user: Nothing }
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

-- The bookings of `room` that hold a day of [start, stop).
clashes : Room.RoomId -> Date -> Date -> Select Booking
clashes room start stop =
  Db.from Booking
    |> Db.where (Booking.room.eq room)
    |> Db.where (Booking.from.lt stop)
    |> Db.where (Booking.to.gt start)

-- The booking holds `day` when it starts on or before it and ends after it: then the first day
-- free of it is its `to`.
holdsDay : Booking -> Date -> Date
holdsDay b day = if b.from <= day && day < b.to then b.to else day

-- The first day, from `start` on, that none of the bookings holds. The bookings of one room do
-- not overlap, so walking them in order of `from` finds it in one pass.
nextFree : Date -> List Booking -> Date
nextFree start bookings =
  bookings
    |> List.sortByText (\b -> Date.toText b.from)
    |> List.foldl holdsDay start

dayOf : Int -> Date
dayOf n = Date.addDays n Date.epoch

test "back-to-back bookings are one stretch" =
  nextFree (dayOf 1) [
    { id: BookingId "a", room: Room.RoomId "r", owner: User.UserId "u", from: dayOf 0, to: dayOf 2 },
    { id: BookingId "b", room: Room.RoomId "r", owner: User.UserId "u", from: dayOf 2, to: dayOf 5 },
  ] == dayOf 5

test "a free day before the first booking is the day asked" =
  nextFree (dayOf 0) [
    { id: BookingId "a", room: Room.RoomId "r", owner: User.UserId "u", from: dayOf 2, to: dayOf 4 },
  ] == dayOf 0

--- file: app/Auth/Login.alder ---
module Auth.Login

import Layout
import Table.User

-- Password checking is outside the language: a capability implemented in Go.
capability Password {
  verify : { email: Text.Email, password: Text } -> Task (Maybe User.UserId),
}

-- In a test, `Test.fake Password` puts this in the adapter's place: it accepts any user who has
-- a row, whatever the password.
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

form Credentials = { email: Text.Email, password: Text.Secret }

Login.load : Login.Params -> Session -> Query Model
Login.load params session = Query.succeed {}

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
      <form submit={Submit {}}>
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
  let r <- Test.submit (Submit {}) {
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
  let r <- Test.submit (Submit {}) {
    email: Text.Email "me@example.com",
    password: Text.Secret "secret",
  }
  Task.succeed (
    {
      answer: r.answer,
      user: r.session.user,
      title: Test.text "h1" r.doc,
    } == {
      answer: Redirected (Rooms.Index.Index {}),
      user: Just me,
      title: Just "Rooms",
    }
  )
}

test "a visitor is sent to log in" = {
  let visitor <- Test.open (Rooms.Index.Index {})
  Task.succeed (visitor.answer == Redirected (Login {}))
}

--- file: app/Rooms/Index.alder ---
module Rooms.Index

import Layout
import Table.Booking
import Table.Room
import Table.User

route Index = GET "/rooms" { } requires user

type Line = { id: Room.RoomId, name: Text, free: Date }

type Model = { lines: List Line }

-- One room's line: its name, and the first day from `today` on that none of its bookings holds.
lineOf : Date -> List Booking -> Room -> Line
lineOf today bookings room =
  {
    id: room.id,
    name: Text.trim room.name,
    free: Booking.nextFree today (List.filter (\b -> b.room == room.id) bookings),
  }

Index.load : Index.Params -> Session -> Query Model
Index.load params session = {
  let rooms <- Db.from Room |> Db.orderBy Room.name.asc |> Db.all
  let bookings <- Db.from Booking |> Db.all
  let today <- Date.today
  let lines : List Line = rooms |> List.map (lineOf today bookings)
  Query.succeed { lines: lines }
}

Index.view : Model -> Session -> Html
Index.view model session =
  <Layout.Page title="Rooms">
    <h1>Rooms</h1>
    <ul id="rooms">
      {for line in model.lines ->
        <li>
          <a class="name" href={Rooms.Show.Show { id: line.id }}>{line.name}</a>
          <span class="free">{Date.toText line.free}</span>
        </li>}
    </ul>
    <nav>
      <a href={Bookings.Index.Index {}}>My bookings</a>
    </nav>
    <al-button variant="outline" click={App.Logout}>Log out</al-button>
  </Layout.Page>

test "the next free date skips back-to-back bookings" = {
  let me <- Db.insert User {
    email: Text.Email "me@example.com",
    name: Text.NonEmpty "Me",
    passwordHash: "",
  }
  let oak <- Db.insert Room { name: Text.NonEmpty "Oak" }
  let today <- Date.today
  let _ <- Db.insert Booking {
    room: oak,
    owner: me,
    from: today,
    to: Date.addDays 2 today,
  }
  let _ <- Db.insert Booking {
    room: oak,
    owner: me,
    from: Date.addDays 2 today,
    to: Date.addDays 4 today,
  }
  let _ <- Test.login { user: Just me }
  let page <- Test.get (Index {})
  Task.succeed (Test.texts "#rooms .free" page.doc == [Date.toText (Date.addDays 4 today)])
}

--- file: app/Rooms/Show.alder ---
module Rooms.Show

import Layout
import Table.Booking
import Table.Room
import Table.User

route Show = GET "/rooms/{id}" { id: Room.RoomId } requires user
route Book = POST "/rooms/{id}/bookings" { id: Room.RoomId } form NewBooking requires user

form NewBooking = { from: Date, to: Date }

type Model = { id: Room.RoomId, name: Text, bookings: List Booking }

Show.load : Show.Params -> Session -> Query Model
Show.load params session = {
  let room <- Db.get Room params.id
  match room {
    Nothing -> Query.notFound,
    Just r -> {
      let bookings <-
        Db.from Booking
          |> Db.where (Booking.room.eq params.id)
          |> Db.orderBy Booking.from.asc
          |> Db.all
      Query.succeed { id: params.id, name: Text.trim r.name, bookings: bookings }
    },
  }
}

-- A refused booking is answered with the form again; a stored one goes back to the room.
Book.update : Book.Params -> Session -> Task Response
Book.update p session = {
  let clashes <- Db.count (Booking.clashes p.id p.form.from p.form.to)
  if p.form.to <= p.form.from then Task.succeed (Response.invalid "The last day must be after the first day.")
  else if clashes > 0 then Task.succeed (Response.invalid "That room is already booked on some of those days.")
  else {
    let _ <- Db.insert Booking {
      room: p.id,
      owner: session.user,
      from: p.form.from,
      to: p.form.to,
    }
    Task.succeed (Response.redirect (Show { id: p.id }))
  }
}

Show.view : Model -> Session -> Html
Show.view model session =
  <Layout.Page title={model.name}>
    <h1 id="name">{model.name}</h1>

    <al-card>
      <form submit={Book { id: model.id }}>
        <Errors form />
        <al-input field="from" label="From" />
        <al-input field="to" label="To" />
        <al-button type="submit">Book</al-button>
      </form>
    </al-card>

    <ul id="bookings">
      {for b in model.bookings ->
        <li><span class="from">{Date.toText b.from}</span> to <span class="to">{Date.toText b.to}</span></li>}
    </ul>

    <a href={Rooms.Index.Index {}}>All rooms</a>
  </Layout.Page>

test "an overlapping booking is refused and not stored" = {
  let me <- Db.insert User {
    email: Text.Email "me@example.com",
    name: Text.NonEmpty "Me",
    passwordHash: "",
  }
  let oak <- Db.insert Room { name: Text.NonEmpty "Oak" }
  let today <- Date.today
  let _ <- Db.insert Booking {
    room: oak,
    owner: me,
    from: Date.addDays 1 today,
    to: Date.addDays 3 today,
  }
  let _ <- Test.login { user: Just me }
  let _ <- Test.get (Show { id: oak })
  let r <- Test.submit (Book { id: oak }) {
    from: Date.addDays 2 today,
    to: Date.addDays 4 today,
  }
  let rows <- Db.from Booking |> Db.count
  Task.succeed (
    { answer: r.answer, rows: rows }
      == {
        answer: FormRejected [{ field: "", message: "That room is already booked on some of those days." }],
        rows: 1,
      }
  )
}

test "a booking that starts on the other's last day is stored" = {
  let me <- Db.insert User {
    email: Text.Email "me@example.com",
    name: Text.NonEmpty "Me",
    passwordHash: "",
  }
  let oak <- Db.insert Room { name: Text.NonEmpty "Oak" }
  let today <- Date.today
  let _ <- Db.insert Booking {
    room: oak,
    owner: me,
    from: Date.addDays 1 today,
    to: Date.addDays 3 today,
  }
  let _ <- Test.login { user: Just me }
  let _ <- Test.get (Show { id: oak })
  let r <- Test.submit (Book { id: oak }) {
    from: Date.addDays 3 today,
    to: Date.addDays 5 today,
  }
  let rows <- Db.from Booking |> Db.count
  Task.succeed (
    { answer: r.answer, rows: rows }
      == { answer: Redirected (Show { id: oak }), rows: 2 }
  )
}

--- file: app/Bookings/Index.alder ---
module Bookings.Index

import Layout
import Table.Booking
import Table.Room

route Index = GET "/bookings" { } requires user

type Line = { id: Booking.BookingId, room: Text, from: Date }

type Model = { lines: List Line }

-- The room's name, trimmed, or "" when the room is gone.
roomName : List Room -> Room.RoomId -> Text
roomName rooms id = {
  let found = rooms |> List.find (\r -> r.id == id)
  match found {
    Just r -> Text.trim r.name,
    Nothing -> "",
  }
}

lineFor : List Room -> Booking -> Line
lineFor rooms booking =
  {
    id: booking.id,
    room: roomName rooms booking.room,
    from: booking.from,
  }

Index.load : Index.Params -> Session -> Query Model
Index.load params session = {
  let bookings <-
    Db.from Booking
      |> Db.where (Booking.owner.eq session.user)
      |> Db.orderBy Booking.from.asc
      |> Db.all
  let rooms <- Db.from Room |> Db.all
  let lines : List Line = bookings |> List.map (lineFor rooms)
  Query.succeed { lines: lines }
}

Index.view : Model -> Session -> Html
Index.view model session =
  <Layout.Page title="My bookings">
    <h1>My bookings</h1>
    <ul id="mine">
      {for line in model.lines ->
        <li>
          <a href={Bookings.Show.Show { id: line.id }}><span class="room">{line.room}</span></a>
          <span class="from">{Date.toText line.from}</span>
        </li>}
    </ul>
    <nav>
      <a href={Rooms.Index.Index {}}>Rooms</a>
    </nav>
    <al-button variant="outline" click={App.Logout}>Log out</al-button>
  </Layout.Page>

--- file: app/Bookings/Show.alder ---
module Bookings.Show

import Layout
import Table.Booking
import Table.Room
import Table.User

route Show   = GET    "/bookings/{id}" { id: Booking.BookingId } requires user
route Cancel = DELETE "/bookings/{id}" { id: Booking.BookingId } requires user

type Model = { id: Booking.BookingId, room: Text, from: Date, to: Date, cancellable: Bool }

-- The booking, when it is the user's: another user's booking is not found, as a missing one is.
ownBooking : Booking.BookingId -> User.UserId -> Query Booking
ownBooking id user = {
  let found <- Db.get Booking id
  match found {
    Just b -> if b.owner == user then Query.succeed b else Query.notFound,
    Nothing -> Query.notFound,
  }
}

Show.load : Show.Params -> Session -> Query Model
Show.load params session = {
  let booking <- ownBooking params.id session.user
  let room <- Db.get Room booking.room
  let today <- Date.today
  let name =
    room
      |> Maybe.map (\r -> Text.trim r.name)
      |> Maybe.withDefault ""
  Query.succeed {
    id: booking.id,
    room: name,
    from: booking.from,
    to: booking.to,
    cancellable: today < booking.from,
  }
}

-- A booking can be cancelled only before its first day. Otherwise the booking stays and the
-- page is shown again.
Cancel.update : Cancel.Params -> Session -> Task Response
Cancel.update p session = {
  let booking <- ownBooking p.id session.user
  let today <- Date.today
  if today < booking.from then {
    let _ <- Db.delete Booking p.id
    Task.succeed (Response.redirect (Bookings.Index.Index {}))
  } else Task.succeed (Response.redirect (Show { id: p.id }))
}

cancelButton : Model -> Html
cancelButton model =
  <al-button id="cancel" variant="destructive" click={Cancel { id: model.id }}>Cancel</al-button>

Show.view : Model -> Session -> Html
Show.view model session =
  <Layout.Page title="Booking">
    <h1 id="room">{model.room}</h1>
    <p><span id="from">{Date.toText model.from}</span> to <span id="to">{Date.toText model.to}</span></p>
    {if model.cancellable then cancelButton model else Html.none}
    <a href={Bookings.Index.Index {}}>My bookings</a>
  </Layout.Page>

test "the owner cancels a booking before its first day" = {
  let me <- Db.insert User {
    email: Text.Email "me@example.com",
    name: Text.NonEmpty "Me",
    passwordHash: "",
  }
  let oak <- Db.insert Room { name: Text.NonEmpty "Oak" }
  let today <- Date.today
  let id <- Db.insert Booking {
    room: oak,
    owner: me,
    from: Date.addDays 1 today,
    to: Date.addDays 2 today,
  }
  let _ <- Test.login { user: Just me }
  let _ <- Test.get (Show { id: id })
  let r <- Test.click (Cancel { id: id })
  let after <- Db.get Booking id
  Task.succeed (
    { answer: r.answer, stored: Maybe.isJust after }
      == { answer: Redirected (Bookings.Index.Index {}), stored: False }
  )
}

test "a booking that starts today cannot be cancelled" = {
  let me <- Db.insert User {
    email: Text.Email "me@example.com",
    name: Text.NonEmpty "Me",
    passwordHash: "",
  }
  let oak <- Db.insert Room { name: Text.NonEmpty "Oak" }
  let today <- Date.today
  let id <- Db.insert Booking {
    room: oak,
    owner: me,
    from: today,
    to: Date.addDays 1 today,
  }
  let _ <- Test.login { user: Just me }
  let page <- Test.get (Show { id: id })
  let r <- Test.run (Cancel.update { id: id } { user: me })
  let after <- Db.get Booking id
  Task.succeed (
    { button: Test.has "#cancel" page.doc, value: r.value, stored: Maybe.isJust after }
      == {
        button: False,
        value: Ok (Response.redirect (Show { id: id })),
        stored: True,
      }
  )
}

test "another user's booking is not found" = {
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
  let oak <- Db.insert Room { name: Text.NonEmpty "Oak" }
  let today <- Date.today
  let id <- Db.insert Booking {
    room: oak,
    owner: me,
    from: Date.addDays 1 today,
    to: Date.addDays 2 today,
  }
  let _ <- Test.login { user: Just other }
  let r <- Test.open (Show { id: id })
  Task.succeed (r.answer == PageMissing)
}
