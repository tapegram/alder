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
      <al-toasts></al-toasts>
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

table Room { id: RoomId @id, name: Text.NonEmpty }

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

-- The first date, `today` or later, that none of the bookings holds. The bookings are one
-- room's, which never overlap, ordered by `from`.
nextFree : Date -> List Booking -> Date
nextFree today bookings =
  List.foldl (\b d -> if b.from <= d && b.to > d then b.to else d) today bookings

-- A booking can be cancelled while today is before its `from` date.
cancellable : Date -> Booking -> Bool
cancellable today booking = today < booking.from

test "next free date skips back-to-back bookings" =
  nextFree (Date.addDays 10 Date.epoch) [
    {
      id: BookingId "a",
      room: Room.RoomId "r",
      owner: User.UserId "u",
      from: Date.addDays 9 Date.epoch,
      to: Date.addDays 12 Date.epoch,
    },
    {
      id: BookingId "b",
      room: Room.RoomId "r",
      owner: User.UserId "u",
      from: Date.addDays 12 Date.epoch,
      to: Date.addDays 14 Date.epoch,
    },
    {
      id: BookingId "c",
      room: Room.RoomId "r",
      owner: User.UserId "u",
      from: Date.addDays 15 Date.epoch,
      to: Date.addDays 16 Date.epoch,
    },
  ] == Date.addDays 14 Date.epoch

test "a free room is free today" = nextFree Date.epoch [] == Date.epoch

--- file: app/Auth/Login.alder ---
module Auth.Login

import Layout
import Table.User

-- Password checking is outside the language: a capability implemented in Go.
capability Password {
  verify : { email: Text.Email, password: Text } -> Task (Maybe User.UserId),
}

-- In a test, `Test.fake Password` accepts any user who has a row, whatever the password.
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

test "a visitor is sent to log in, and can log out" = {
  let me <- Db.insert User {
    email: Text.Email "me@example.com",
    name: Text.NonEmpty "Me",
    passwordHash: "",
  }
  let visitor <- Test.open (Rooms.Index.Index {})
  let _ <- Test.login { user: Just me }
  let _ <- Test.get (Rooms.Index.Index {})
  let out <- Test.click App.Logout
  Task.succeed (
    {
      visitor: visitor.answer,
      out: out.answer,
      user: out.session.user,
    } == {
      visitor: Redirected (Login {}),
      out: Redirected (Login {}),
      user: Nothing,
    }
  )
}

--- file: app/Rooms/Index.alder ---
module Rooms.Index

import Layout
import Table.Booking
import Table.Room
import Table.User

route Index = GET "/rooms" { } requires user

type RoomRow = { room: Room, free: Date }

type Model = { rooms: List RoomRow }

Index.load : Index.Params -> Session -> Query Model
Index.load params session = {
  let today <- Date.today
  let rooms <-
    Db.from Room
      |> Db.orderBy Room.name.asc
      |> Db.all
  let bookings <-
    Db.from Booking
      |> Db.where (Booking.to.gt today)
      |> Db.orderBy Booking.from.asc
      |> Db.all
  Query.succeed {
    rooms: List.map (\r -> { room: r, free: Booking.nextFree today (List.filter (\b -> b.room == r.id) bookings) }) rooms,
  }
}

Index.view : Model -> Session -> Html
Index.view model session =
  <Layout.Page title="Rooms">
    <h1>Rooms</h1>
    <ul id="rooms">
      {for row in model.rooms ->
        <li>
          <a class="name" href={Rooms.Show.Show { id: row.room.id }}>{row.room.name}</a>
          {" "}
          <span class="free">{Date.toText row.free}</span>
        </li>}
    </ul>
    <nav><a href={Bookings.Index.Index {}}>My bookings</a></nav>
    <al-button variant="outline" click={App.Logout}>Log out</al-button>
  </Layout.Page>

test "the next free date of a room with back-to-back bookings" = {
  let _ <- Test.setClock (Time.fromMillis 1772582400000)
  let today <- Date.today
  let me <- Db.insert User {
    email: Text.Email "me@example.com",
    name: Text.NonEmpty "Me",
    passwordHash: "",
  }
  let blue <- Db.insert Room { name: Text.NonEmpty "Blue" }
  let _ <- Db.insert Room { name: Text.NonEmpty "Green" }
  let _ <- Db.insert Booking {
    room: blue,
    owner: me,
    from: Date.addDays 2 today,
    to: Date.addDays 5 today,
  }
  let _ <- Db.insert Booking {
    room: blue,
    owner: me,
    from: today,
    to: Date.addDays 2 today,
  }
  let _ <- Db.insert Booking {
    room: blue,
    owner: me,
    from: Date.addDays 6 today,
    to: Date.addDays 7 today,
  }
  let _ <- Test.login { user: Just me }
  let page <- Test.get (Index {})
  Task.succeed (
    {
      names: Test.texts "#rooms .name" page.doc,
      free: Test.texts "#rooms .free" page.doc,
    } == {
      names: ["Blue", "Green"],
      free: [Date.toText (Date.addDays 5 today), Date.toText today],
    }
  )
}

--- file: app/Rooms/Show.alder ---
module Rooms.Show

import Layout
import Table.Booking
import Table.Room
import Table.User

route Show = GET  "/rooms/{id}"          { id: Room.RoomId }                 requires user
route Book = POST "/rooms/{id}/bookings" { id: Room.RoomId } form NewBooking requires user

type Model = { room: Room, bookings: List Booking }

Show.load : Show.Params -> Session -> Query Model
Show.load params session = {
  let found <- Db.get Room params.id
  let bookings <-
    Db.from Booking
      |> Db.where (Booking.room.eq params.id)
      |> Db.orderBy Booking.from.asc
      |> Db.all
  match found {
    Just room -> Query.succeed { room: room, bookings: bookings },
    Nothing -> Query.notFound,
  }
}

form NewBooking = { from: Date, to: Date }

-- Two bookings of one room overlap when each starts before the other ends. The checks come
-- before the insert, so a refused booking stores nothing.
Book.update : Book.Params -> Session -> Task Response
Book.update p session = {
  let found <- Db.get Room p.id
  let clashes <-
    Db.from Booking
      |> Db.where (Booking.room.eq p.id)
      |> Db.where (Booking.from.lt p.form.to)
      |> Db.where (Booking.to.gt p.form.from)
      |> Db.count
  match found {
    Nothing -> Task.notFound,
    Just _ ->
      if p.form.to <= p.form.from then
        Task.succeed (Response.invalid "The end date must be after the start date.")
      else if clashes > 0 then
        Task.succeed (Response.invalid "The room is already booked on some of those dates.")
      else {
        let _ <- Db.insert Booking {
          room: p.id,
          owner: session.user,
          from: p.form.from,
          to: p.form.to,
        }
        Task.succeed (Response.redirect (Show { id: p.id }))
      },
  }
}

Show.view : Model -> Session -> Html
Show.view model session =
  <Layout.Page title={model.room.name}>
    <h1 id="name">{model.room.name}</h1>
    <ul id="bookings">
      {for b in model.bookings ->
        <li><span class="from">{Date.toText b.from}</span> to <span class="to">{Date.toText b.to}</span></li>}
    </ul>
    <al-card>
      <form submit={Book { id: model.room.id }}>
        <Errors form />
        <al-input field="from" label="From" />
        <al-input field="to" label="To" />
        <al-button type="submit">Book</al-button>
      </form>
    </al-card>
    <nav><a href={Rooms.Index.Index {}}>All rooms</a></nav>
  </Layout.Page>

test "an overlapping booking is refused, one from the other's end date is stored" = {
  let _ <- Test.setClock (Time.fromMillis 1772582400000)
  let today <- Date.today
  let me <- Db.insert User {
    email: Text.Email "me@example.com",
    name: Text.NonEmpty "Me",
    passwordHash: "",
  }
  let room <- Db.insert Room { name: Text.NonEmpty "Blue" }
  let _ <- Db.insert Booking {
    room: room,
    owner: me,
    from: Date.addDays 1 today,
    to: Date.addDays 3 today,
  }
  let _ <- Test.login { user: Just me }
  let _ <- Test.get (Show { id: room })
  let clash <- Test.submit (Book { id: room }) {
    from: Date.addDays 2 today,
    to: Date.addDays 4 today,
  }
  let afterClash <- Db.from Booking |> Db.count
  let next <- Test.submit (Book { id: room }) {
    from: Date.addDays 3 today,
    to: Date.addDays 5 today,
  }
  let page <- Test.get (Show { id: room })
  Task.succeed (
    {
      clash: clash.answer,
      afterClash: afterClash,
      next: next.answer,
      froms: Test.texts "#bookings .from" page.doc,
    } == {
      clash: FormRejected [{ field: "", message: "The room is already booked on some of those dates." }],
      afterClash: 1,
      next: Redirected (Show { id: room }),
      froms: [Date.toText (Date.addDays 1 today), Date.toText (Date.addDays 3 today)],
    }
  )
}

test "bookings of different rooms never conflict" = {
  let _ <- Test.setClock (Time.fromMillis 1772582400000)
  let today <- Date.today
  let me <- Db.insert User {
    email: Text.Email "me@example.com",
    name: Text.NonEmpty "Me",
    passwordHash: "",
  }
  let blue <- Db.insert Room { name: Text.NonEmpty "Blue" }
  let green <- Db.insert Room { name: Text.NonEmpty "Green" }
  let _ <- Db.insert Booking {
    room: blue,
    owner: me,
    from: Date.addDays 1 today,
    to: Date.addDays 3 today,
  }
  let _ <- Test.login { user: Just me }
  let _ <- Test.get (Show { id: green })
  let r <- Test.submit (Book { id: green }) {
    from: Date.addDays 1 today,
    to: Date.addDays 3 today,
  }
  let n <- Db.from Booking |> Db.count
  Task.succeed ({ answer: r.answer, n: n } == { answer: Redirected (Show { id: green }), n: 2 })
}

test "a booking whose end is not after its start is refused" = {
  let _ <- Test.setClock (Time.fromMillis 1772582400000)
  let today <- Date.today
  let me <- Db.insert User {
    email: Text.Email "me@example.com",
    name: Text.NonEmpty "Me",
    passwordHash: "",
  }
  let room <- Db.insert Room { name: Text.NonEmpty "Blue" }
  let _ <- Test.login { user: Just me }
  let _ <- Test.get (Show { id: room })
  let r <- Test.submit (Book { id: room }) {
    from: Date.addDays 2 today,
    to: Date.addDays 2 today,
  }
  let n <- Db.from Booking |> Db.count
  Task.succeed (
    { answer: r.answer, n: n }
      == {
        answer: FormRejected [{ field: "", message: "The end date must be after the start date." }],
        n: 0,
      }
  )
}

--- file: app/Bookings/Index.alder ---
module Bookings.Index

import Layout
import Table.Booking
import Table.Room

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

roomName : List Room -> Room.RoomId -> Text
roomName rooms id = match List.find (\r -> r.id == id) rooms {
  Just r -> r.name,
  Nothing -> "",
}

Index.view : Model -> Session -> Html
Index.view model session =
  <Layout.Page title="My bookings">
    <h1>My bookings</h1>
    <ul id="mine">
      {for b in model.bookings ->
        <li>
          <a href={Bookings.Show.Show { id: b.id }}><span class="room">{roomName model.rooms b.room}</span></a>
          {" "}
          <span class="from">{Date.toText b.from}</span>
        </li>}
    </ul>
    <nav><a href={Rooms.Index.Index {}}>All rooms</a></nav>
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

type Model = {
  booking: Booking,
  room: Room,
  canCancel: Bool,
}

-- The booking, when it is the user's: anyone else's, like a missing one, is a 404.
ownBooking : User.UserId -> Booking.BookingId -> Query Booking
ownBooking user id = {
  let found <- Db.get Booking id
  match found {
    Just b -> if b.owner == user then Query.succeed b else Query.notFound,
    Nothing -> Query.notFound,
  }
}

Show.load : Show.Params -> Session -> Query Model
Show.load params session = {
  let booking <- ownBooking session.user params.id
  let room <- Db.get Room booking.room
  let today <- Date.today
  match room {
    Just r -> Query.succeed {
      booking: booking,
      room: r,
      canCancel: Booking.cancellable today booking,
    },
    Nothing -> Query.notFound,
  }
}

-- No `load` runs before a write: the owner and the date are checked again here.
Cancel.update : Cancel.Params -> Session -> Task Response
Cancel.update p session = {
  let booking <- ownBooking session.user p.id
  let today <- Date.today
  if Booking.cancellable today booking then {
    let _ <- Db.delete Booking p.id
    Task.succeed (Response.redirect (Bookings.Index.Index {}))
  } else {
    Task.succeed (
      Response.redirect (Show { id: p.id })
        |> Response.error "This booking has started: it can no longer be cancelled."
    )
  }
}

Show.view : Model -> Session -> Html
Show.view model session =
  <Layout.Page title="Booking">
    <h1 id="room">{model.room.name}</h1>
    <p>
      From <span id="from">{Date.toText model.booking.from}</span> to <span id="to">{Date.toText model.booking.to}</span>
    </p>
    {if model.canCancel then <button id="cancel" click={Cancel { id: model.booking.id }}>Cancel</button> else Html.none}
    <nav><a href={Bookings.Index.Index {}}>My bookings</a></nav>
  </Layout.Page>

test "the owner cancels a booking before its from date" = {
  let _ <- Test.setClock (Time.fromMillis 1772582400000)
  let today <- Date.today
  let me <- Db.insert User {
    email: Text.Email "me@example.com",
    name: Text.NonEmpty "Me",
    passwordHash: "",
  }
  let room <- Db.insert Room { name: Text.NonEmpty "Blue" }
  let id <- Db.insert Booking {
    room: room,
    owner: me,
    from: Date.addDays 1 today,
    to: Date.addDays 2 today,
  }
  let _ <- Test.login { user: Just me }
  let page <- Test.get (Show { id: id })
  let r <- Test.click (Cancel { id: id })
  let after <- Db.get Booking id
  Task.succeed (
    {
      button: Test.has "#cancel" page.doc,
      room: Test.text "#room" page.doc,
      answer: r.answer,
      left: Maybe.isJust after,
    } == {
      button: True,
      room: Just "Blue",
      answer: Redirected (Bookings.Index.Index {}),
      left: False,
    }
  )
}

test "a booking whose from date is today cannot be cancelled" = {
  let _ <- Test.setClock (Time.fromMillis 1772582400000)
  let today <- Date.today
  let me <- Db.insert User {
    email: Text.Email "me@example.com",
    name: Text.NonEmpty "Me",
    passwordHash: "",
  }
  let room <- Db.insert Room { name: Text.NonEmpty "Blue" }
  let id <- Db.insert Booking {
    room: room,
    owner: me,
    from: Date.addDays 1 today,
    to: Date.addDays 3 today,
  }
  let _ <- Test.login { user: Just me }
  let before <- Test.get (Show { id: id })
  let _ <- Test.advance (24 * 60 * 60 * 1000)
  let r <- Test.click (Cancel { id: id })
  let page <- Test.get (Show { id: id })
  let after <- Db.get Booking id
  Task.succeed (
    {
      before: Test.has "#cancel" before.doc,
      answer: r.answer,
      button: Test.has "#cancel" page.doc,
      left: Maybe.isJust after,
    } == {
      before: True,
      answer: Redirected (Show { id: id }),
      button: False,
      left: True,
    }
  )
}

test "another user's booking is not found" = {
  let me <- Db.insert User {
    email: Text.Email "me@example.com",
    name: Text.NonEmpty "Me",
    passwordHash: "",
  }
  let today <- Date.today
  let room <- Db.insert Room { name: Text.NonEmpty "Blue" }
  let id <- Db.insert Booking {
    room: room,
    owner: me,
    from: Date.addDays 1 today,
    to: Date.addDays 2 today,
  }
  let _ <- Test.login { user: Just (User.UserId "someone-else") }
  let r <- Test.open (Show { id: id })
  Task.succeed (r.answer == PageMissing)
}
