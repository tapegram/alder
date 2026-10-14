--- file: app/App.alder ---
module App

import User
import Layout

session { user: Maybe User.UserId }

login = Auth.Login

msg Logout

update : Msg -> Session -> Task Nav
update msg session = match msg {
  Logout p -> {
    let _ <- Session.put (\s -> { s | user: Nothing })
    Task.succeed (Redirect (Auth.Login {}))
  },
}

error : Error -> Html Never
error err =
  <Layout.Page title="Error">
    <h1>Something went wrong</h1>
    <p>Please try again.</p>
  </Layout.Page>

notFound : Html Never
notFound =
  <Layout.Page title="Not found">
    <h1>Not found</h1>
  </Layout.Page>

test "Logout clears the user" = {
  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
  let _ <- Test.login { user: Just me }
  let _ <- Test.get (Rooms.Index {})
  let out <- Test.click Logout
  Task.succeed ({ out: out.answer, user: out.session.user } == { out: Redirected (Auth.Login {}), user: Nothing })
}

--- file: app/Auth/Login.alder ---
module Auth.Login

import User
import Layout

-- Password checking is outside the language: a capability implemented in Go.
capability Password {
  verify : { email: Text.Email, password: Text } -> Task (Maybe User.UserId),
}

-- In a test, `Test.fake Password` puts this in the adapter's place: it accepts any user who has
-- a row, whatever the password, so a test can log in through the form below.
fake Password {
  verify args = {
    let found <- Db.from User |> Db.where (User.email.eq args.email) |> Db.first
    Task.succeed (Maybe.map (\u -> u.id) found)
  }
}

page "/login" { }

type Model = { }

load : Params -> Session -> Query Model
load params session = Query.succeed { }

form Credentials = {
  email: Text.Email,
  password: Text.Secret,
}

msg Submit form Credentials

update : Msg -> Model -> Session -> Task Nav
update msg model session = match msg {
  Submit p -> {
    let found <- Password.verify { email: p.form.email, password: p.form.password }
    match found {
      Just id -> {
        let _ <- Session.put (\s -> { s | user: Just id })
        Task.succeed (Redirect (Rooms.Index {}))
      },
      Nothing -> Task.succeed (Reject "Email or password is incorrect."),
    }
  },
}

view : Model -> Session -> Html Msg
view model session =
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
  let _ <- Test.get (Auth.Login {})
  let r <- Test.submit Credentials { email: Text.Email "nobody@example.com", password: Text.Secret "secret" }
  Task.succeed ({ answer: r.answer, user: r.session.user } == { answer: FormRejected [{ field: "", message: "Email or password is incorrect." }], user: Nothing })
}

test "logging in through the form" = {
  let _ <- Test.fake Password
  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
  let _ <- Test.get (Auth.Login {})
  let r <- Test.submit Credentials { email: Text.Email "me@example.com", password: Text.Secret "secret" }
  Task.succeed ({ answer: r.answer, user: r.session.user } == { answer: Redirected (Rooms.Index {}), user: Just me })
}

--- file: app/Booking.alder ---
module Booking

import Room
import User

type BookingId = brand Text

-- A booking holds its room from `from` up to, but not including, `to`.
table Booking {
  id: BookingId @id,
  room: Room.RoomId @ref Room,
  owner: User.UserId @ref User,
  from: Date,
  to: Date,
}

-- The first day, from `today` on, that no booking in `bookings` holds. The bookings are walked in
-- order of their first day; a booking that holds the current day moves it to that booking's `to`.
freeFrom : Date -> List Booking -> Date
freeFrom today bookings =
  bookings
    |> List.sortByText (\b -> Date.toText b.from)
    |> List.foldl (\b cur -> if b.from <= cur && cur < b.to then b.to else cur) today

-- Whether a room's bookings hold any day of [fromDay, toDay).
clashes : Room.RoomId -> Date -> Date -> Query Bool
clashes room fromDay toDay = {
  let rows <- Db.from Booking |> Db.where (Booking.room.eq room) |> Db.all
  Query.succeed (rows |> List.any (\b -> b.from < toDay && fromDay < b.to))
}

test "back-to-back bookings leave the next free date after both" = {
  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
  let room <- Db.insert Room { name: Text.NonEmpty "Oak" }
  let today <- Date.today
  let _ <- Db.insert Booking { room: room, owner: me, from: today, to: Date.addDays 2 today }
  let _ <- Db.insert Booking { room: room, owner: me, from: Date.addDays 2 today, to: Date.addDays 4 today }
  let bookings <- Db.from Booking |> Db.where (Booking.room.eq room) |> Db.all
  Task.succeed (freeFrom today bookings == Date.addDays 4 today)
}

test "a booking that starts on another's last day is not a clash" = {
  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
  let room <- Db.insert Room { name: Text.NonEmpty "Oak" }
  let today <- Date.today
  let _ <- Db.insert Booking { room: room, owner: me, from: today, to: Date.addDays 2 today }
  let touching <- clashes room (Date.addDays 2 today) (Date.addDays 4 today)
  let overlapping <- clashes room (Date.addDays 1 today) (Date.addDays 4 today)
  Task.succeed ({ touching: touching, overlapping: overlapping } == { touching: False, overlapping: True })
}

--- file: app/Bookings/Index.alder ---
module Bookings.Index

import Booking
import Room
import Layout
import User

page "/bookings" { } requires user

type Model = {
  bookings: List Booking,
  rooms: List Room,
}

load : Params -> Session -> Query Model
load params session = {
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
  let found = rooms |> List.find (\r -> r.id == b.room)
  match found {
    Just r -> Text.trim r.name,
    Nothing -> "",
  }
}

view : Model -> Session -> Html Msg
view model session =
  <Layout.Page title="My bookings">
    <h1>My bookings</h1>
    <ul id="mine">
      {for b in model.bookings ->
        <li>
          <a href={Bookings.Show { id: b.id }}>
            <span class="room">{roomName model.rooms b}</span>
          </a>
          <span class="from">{Date.toText b.from}</span>
        </li>}
    </ul>
  </Layout.Page>

test "my bookings list my bookings only" = {
  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
  let other <- Db.insert User { email: Text.Email "other@example.com", name: Text.NonEmpty "Other", passwordHash: "" }
  let room <- Db.insert Room { name: Text.NonEmpty "Oak" }
  let today <- Date.today
  let _ <- Db.insert Booking { room: room, owner: me, from: Date.addDays 1 today, to: Date.addDays 2 today }
  let _ <- Db.insert Booking { room: room, owner: other, from: Date.addDays 5 today, to: Date.addDays 6 today }
  let _ <- Test.login { user: Just me }
  let page <- Test.get (Bookings.Index {})
  Task.succeed ({ rooms: Test.texts "#mine .room" page.doc, items: Test.count "#mine li" page.doc } == { rooms: ["Oak"], items: 1 })
}

--- file: app/Bookings/Show.alder ---
module Bookings.Show

import Booking
import Room
import Layout
import User

page "/bookings/{id}" { id: Booking.BookingId } requires user

type Model = {
  booking: Booking,
  room: Room,
  today: Date,
}

load : Params -> Session -> Query Model
load params session = {
  let found <- Db.get Booking params.id
  let today <- Date.today
  match found {
    Nothing -> Query.notFound,
    Just booking -> {
      let found <- Db.get Room booking.room
      match found {
        Nothing -> Query.notFound,
        Just room -> if booking.owner == session.user then Query.succeed { booking: booking, room: room, today: today } else Query.notFound,
      }
    },
  }
}

msg Cancel

-- Deletes the booking, which the owner may do only before its first day.
cancelBooking : Booking -> Task Nav
cancelBooking booking = {
  let _ <- Db.delete Booking booking.id
  Task.succeed (Redirect (Bookings.Index {}))
}

update : Msg -> Model -> Session -> Task Nav
update msg model session = match msg {
  Cancel p -> {
    let found <- Db.get Booking model.booking.id
    let today <- Date.today
    match found {
      Nothing -> Task.notFound,
      Just booking -> if booking.owner != session.user then Task.notFound else if today < booking.from then cancelBooking booking else Task.succeed Rerender,
    }
  },
}

view : Model -> Session -> Html Msg
view model session =
  <Layout.Page title="Booking">
    <h1 id="room">{model.room.name}</h1>
    <p>From <span id="from">{Date.toText model.booking.from}</span> to <span id="to">{Date.toText model.booking.to}</span></p>
    {if model.today < model.booking.from then <al-button id="cancel" variant="destructive" click={Cancel}>Cancel booking</al-button> else Html.none}
  </Layout.Page>

test "the owner cancels a booking before its first day" = {
  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
  let room <- Db.insert Room { name: Text.NonEmpty "Oak" }
  let today <- Date.today
  let id <- Db.insert Booking { room: room, owner: me, from: Date.addDays 3 today, to: Date.addDays 4 today }
  let _ <- Test.login { user: Just me }
  let _ <- Test.get (Bookings.Show { id: id })
  let r <- Test.click Cancel
  let left <- Db.from Booking |> Db.count
  Task.succeed ({ answer: r.answer, left: left } == { answer: Redirected (Bookings.Index {}), left: 0 })
}

test "a booking whose first day is today cannot be cancelled" = {
  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
  let room <- Db.insert Room { name: Text.NonEmpty "Oak" }
  let today <- Date.today
  let id <- Db.insert Booking { room: room, owner: me, from: today, to: Date.addDays 1 today }
  let _ <- Test.login { user: Just me }
  let page <- Test.get (Bookings.Show { id: id })
  let r <- Test.run (update Cancel { booking: { id: id, room: room, owner: me, from: today, to: Date.addDays 1 today }, room: { id: room, name: Text.NonEmpty "Oak" }, today: today } { user: me })
  let kept <- Db.from Booking |> Db.count
  Task.succeed ({ button: Test.has "#cancel" page.doc, value: r.value, kept: kept } == { button: False, value: Ok Rerender, kept: 1 })
}

test "another user's booking is not found" = {
  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
  let other <- Db.insert User { email: Text.Email "other@example.com", name: Text.NonEmpty "Other", passwordHash: "" }
  let room <- Db.insert Room { name: Text.NonEmpty "Oak" }
  let today <- Date.today
  let id <- Db.insert Booking { room: room, owner: me, from: Date.addDays 3 today, to: Date.addDays 4 today }
  let _ <- Test.login { user: Just other }
  let r <- Test.open (Bookings.Show { id: id })
  Task.succeed (r.answer == PageMissing)
}

--- file: app/Layout.alder ---
module Layout

Page : { title: Text, children: List (Html a) } -> Html a
Page props =
  <html>
    <head>
      <meta charset="utf-8" />
      <title>{props.title}</title>
      <link rel="stylesheet" href={Url.external "/static/al/al.css"} />
    </head>
    <body>
      <al-page width="narrow">
        <main>
          <nav>
            <a href={Rooms.Index {}}>Rooms</a>
            <a href={Bookings.Index {}}>My bookings</a>
            <al-button variant="outline" click={App.Logout}>Log out</al-button>
          </nav>
          {props.children}
        </main>
      </al-page>
    </body>
  </html>

--- file: app/Room.alder ---
module Room

type RoomId = brand Text

table Room {
  id: RoomId @id,
  name: Text.NonEmpty,
}

--- file: app/Rooms/Index.alder ---
module Rooms.Index

import Room
import Booking
import Layout
import User

page "/rooms" { } requires user

type Row = { room: Room, free: Date }

type Model = { rows: List Row }

rowFor : Date -> List Booking -> Room -> Row
rowFor today bookings room = {
  room: room,
  free: Booking.freeFrom today (bookings |> List.filter (\b -> b.room == room.id)),
}

load : Params -> Session -> Query Model
load params session = {
  let today <- Date.today
  let rooms <- Db.from Room |> Db.orderBy Room.name.asc |> Db.all
  let bookings <- Db.from Booking |> Db.all
  Query.succeed { rows: List.map (rowFor today bookings) rooms }
}

view : Model -> Session -> Html Msg
view model session =
  <Layout.Page title="Rooms">
    <h1>Rooms</h1>
    <ul id="rooms">
      {for row in model.rows ->
        <li>
          <a class="name" href={Rooms.Show { id: row.room.id }}>{row.room.name}</a>
          <span class="free">{Date.toText row.free}</span>
        </li>}
    </ul>
  </Layout.Page>

test "the room lists its next free date" = {
  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
  let room <- Db.insert Room { name: Text.NonEmpty "Oak" }
  let today <- Date.today
  let _ <- Db.insert Booking { room: room, owner: me, from: today, to: Date.addDays 2 today }
  let _ <- Db.insert Booking { room: room, owner: me, from: Date.addDays 2 today, to: Date.addDays 4 today }
  let _ <- Test.login { user: Just me }
  let page <- Test.get (Rooms.Index {})
  Task.succeed (Test.texts "#rooms .free" page.doc == [Date.toText (Date.addDays 4 today)])
}

--- file: app/Rooms/Show.alder ---
module Rooms.Show

import Room
import Booking
import Layout
import User

page "/rooms/{id}" { id: Room.RoomId } requires user

type Model = {
  room: Room,
  bookings: List Booking,
}

load : Params -> Session -> Query Model
load params session = {
  let found <- Db.get Room params.id
  match found {
    Nothing -> Query.notFound,
    Just room -> {
      let bookings <-
        Db.from Booking
          |> Db.where (Booking.room.eq room.id)
          |> Db.orderBy Booking.from.asc
          |> Db.all
      Query.succeed { room: room, bookings: bookings }
    },
  }
}

form NewBooking = {
  from: Date,
  to: Date,
}

msg Book form NewBooking

book : Room.RoomId -> User.UserId -> Date -> Date -> Task Nav
book room owner fromDay toDay = {
  let _ <- Db.insert Booking { room: room, owner: owner, from: fromDay, to: toDay }
  Task.succeed (Redirect (Rooms.Show { id: room }))
}

update : Msg -> Model -> Session -> Task Nav
update msg model session = match msg {
  Book p -> {
    let clash <- Booking.clashes model.room.id p.form.from p.form.to
    if p.form.to <= p.form.from then Task.succeed (Reject "The booking must end after it starts.") else if clash then Task.succeed (Reject "The room is already booked for part of those dates.") else book model.room.id session.user p.form.from p.form.to
  },
}

view : Model -> Session -> Html Msg
view model session =
  <Layout.Page title={model.room.name}>
    <h1 id="name">{model.room.name}</h1>
    <h2>Bookings</h2>
    <ul id="bookings">
      {for b in model.bookings ->
        <li>
          <span class="from">{Date.toText b.from}</span> to <span class="to">{Date.toText b.to}</span>
        </li>}
    </ul>
    <al-card>
      <form submit={Book}>
        <Errors form />
        <al-input field="from" label="From" />
        <al-input field="to" label="To" />
        <al-button type="submit">Book</al-button>
      </form>
    </al-card>
  </Layout.Page>

test "an overlapping booking is refused and not stored" = {
  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
  let room <- Db.insert Room { name: Text.NonEmpty "Oak" }
  let today <- Date.today
  let _ <- Db.insert Booking { room: room, owner: me, from: Date.addDays 2 today, to: Date.addDays 5 today }
  let _ <- Test.login { user: Just me }
  let _ <- Test.get (Rooms.Show { id: room })
  let r <- Test.submit NewBooking { from: Date.addDays 4 today, to: Date.addDays 6 today }
  let stored <- Db.from Booking |> Db.where (Booking.room.eq room) |> Db.count
  Task.succeed ({ answer: r.answer, stored: stored } == { answer: FormRejected [{ field: "", message: "The room is already booked for part of those dates." }], stored: 1 })
}

test "a booking that starts on another's last day is stored" = {
  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
  let room <- Db.insert Room { name: Text.NonEmpty "Oak" }
  let today <- Date.today
  let _ <- Db.insert Booking { room: room, owner: me, from: Date.addDays 2 today, to: Date.addDays 5 today }
  let _ <- Test.login { user: Just me }
  let _ <- Test.get (Rooms.Show { id: room })
  let r <- Test.submit NewBooking { from: Date.addDays 5 today, to: Date.addDays 7 today }
  let stored <- Db.from Booking |> Db.where (Booking.room.eq room) |> Db.count
  Task.succeed ({ answer: r.answer, stored: stored } == { answer: Redirected (Rooms.Show { id: room }), stored: 2 })
}

test "a booking that ends before it starts is refused" = {
  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
  let room <- Db.insert Room { name: Text.NonEmpty "Oak" }
  let today <- Date.today
  let _ <- Test.login { user: Just me }
  let _ <- Test.get (Rooms.Show { id: room })
  let r <- Test.submit NewBooking { from: Date.addDays 3 today, to: Date.addDays 3 today }
  Task.succeed (r.answer == FormRejected [{ field: "", message: "The booking must end after it starts." }])
}

--- file: app/User.alder ---
module User

type UserId = brand Text

table User {
  id: UserId @id,
  email: Text.Email @unique,
  name: Text.NonEmpty,
  passwordHash: Text,
  created: Time @created,
}
