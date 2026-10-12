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

test "Logout clears the user and goes to log in" = {
  let r <- Test.run (update Logout { user: Just (User.UserId "me") })
  Task.succeed ({ value: r.value, trace: r.trace } == { value: Ok (Redirect (Auth.Login {})), trace: ["Session.put"] })
}

--- file: app/Layout.alder ---
module Layout

Page : { title: Text, children: List (Html a) } -> Html a
Page props =
  <html>
    <head>
      <meta charset="utf-8" />
      <title>{props.title}</title>
    </head>
    <body>
      <nav>
        <a href={Rooms.Index {}}>Rooms</a>
        <a href={Bookings.Index {}}>Your bookings</a>
        <button click={App.Logout}>Log out</button>
      </nav>
      <main>{props.children}</main>
    </body>
  </html>

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

--- file: app/Room.alder ---
module Room

type RoomId = brand Text

table Room {
  id: RoomId @id,
  name: Text.NonEmpty,
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

-- The bookings of a room that hold a day in the half-open range [startDate, endDate).
overlapping : Room.RoomId -> Date -> Date -> Query (Maybe Booking)
overlapping room startDate endDate =
  Db.from Booking
    |> Db.where (Booking.room.eq room)
    |> Db.where (Booking.from.lt endDate)
    |> Db.where (Booking.to.gt startDate)
    |> Db.first

-- If a booking holds `day`, the day after it ends; otherwise `day`.
skipBooking : Booking -> Date -> Date
skipBooking b day = if b.from <= day && b.to > day then b.to else day

-- The first day, from `today` on, that none of the bookings holds. The bookings must be
-- ordered by `from`, as `Db.orderBy Booking.from.asc` gives them.
nextFree : Date -> List Booking -> Date
nextFree today bookings = List.foldl skipBooking today bookings

test "back-to-back bookings leave the next day free" = {
  let today <- Date.today
  Task.succeed (nextFree today [{ id: BookingId "a", room: Room.RoomId "r", owner: User.UserId "u", from: today, to: Date.addDays 2 today }, { id: BookingId "b", room: Room.RoomId "r", owner: User.UserId "u", from: Date.addDays 2 today, to: Date.addDays 4 today }] == Date.addDays 4 today)
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

load : Params -> Session -> Query Model
load params session = {
  let today <- Date.today
  let rooms <- Db.from Room |> Db.orderBy Room.name.asc |> Db.all
  let rows <- Query.all (List.map (rowFor today) rooms)
  Query.succeed { rows: rows }
}

rowFor : Date -> Room -> Query Row
rowFor today room = {
  let bookings <-
    Db.from Booking
      |> Db.where (Booking.room.eq room.id)
      |> Db.where (Booking.to.gt today)
      |> Db.orderBy Booking.from.asc
      |> Db.all
  Query.succeed { room: room, free: Booking.nextFree today bookings }
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

test "a room lists the day after its bookings end" = {
  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
  let room <- Db.insert Room { name: Text.NonEmpty "Blue" }
  let today <- Date.today
  let _ <- Db.insert Booking { room: room, owner: me, from: today, to: Date.addDays 3 today }
  let _ <- Db.insert Booking { room: room, owner: me, from: Date.addDays 3 today, to: Date.addDays 5 today }
  let _ <- Test.login { user: Just me }
  let page <- Test.get (Rooms.Index {})
  Task.succeed (Test.texts "#rooms .free" page.doc == [Date.toText (Date.addDays 5 today)])
}

--- file: app/Rooms/Show.alder ---
module Rooms.Show

import Room
import Booking
import Layout
import User

page "/rooms/{id}" { id: Room.RoomId } requires user

type Model = { room: Room, bookings: List Booking }

load : Params -> Session -> Query Model
load params session = {
  let found <- Db.get Room params.id
  match found {
    Just room -> roomPage room,
    Nothing -> Query.notFound,
  }
}

roomPage : Room -> Query Model
roomPage room = {
  let bookings <-
    Db.from Booking
      |> Db.where (Booking.room.eq room.id)
      |> Db.orderBy Booking.from.asc
      |> Db.all
  Query.succeed { room: room, bookings: bookings }
}

form NewBooking = {
  from: Date,
  to: Date,
}

msg Book form NewBooking

update : Msg -> Model -> Session -> Task Nav
update msg model session = match msg {
  Book p -> {
    let taken <- Booking.overlapping model.room.id p.form.from p.form.to
    let valid = p.form.to > p.form.from
    if not valid then Task.succeed (Reject "A booking must end after it starts.")
    else if Maybe.isJust taken then Task.succeed (Reject "The room is already booked for some of those days.")
    else {
      let _ <- Db.insert Booking { room: model.room.id, owner: session.user, from: p.form.from, to: p.form.to }
      Task.succeed (Redirect (Rooms.Show { id: model.room.id }))
    },
  },
}

view : Model -> Session -> Html Msg
view model session =
  <Layout.Page title={model.room.name}>
    <h1 id="name">{model.room.name}</h1>
    <h2>Bookings</h2>
    <ul id="bookings">
      {for b in model.bookings ->
        <li><span class="from">{Date.toText b.from}</span> to <span class="to">{Date.toText b.to}</span></li>}
    </ul>
    <h2>Book this room</h2>
    <form submit={Book}>
      <Errors form />
      <Input field="from" label="From" />
      <Errors field="from" />
      <Input field="to" label="To" />
      <Errors field="to" />
      <button>Book</button>
    </form>
    <a href={Rooms.Index {}}>All rooms</a>
  </Layout.Page>

test "an overlapping booking is refused and not stored" = {
  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
  let room <- Db.insert Room { name: Text.NonEmpty "Blue" }
  let today <- Date.today
  let _ <- Db.insert Booking { room: room, owner: me, from: Date.addDays 2 today, to: Date.addDays 5 today }
  let _ <- Test.login { user: Just me }
  let _ <- Test.get (Rooms.Show { id: room })
  let r <- Test.submit NewBooking { from: Date.addDays 4 today, to: Date.addDays 8 today }
  let n <- Db.from Booking |> Db.where (Booking.room.eq room) |> Db.count
  let refused = match r.answer { FormRejected errs -> List.length errs == 1, _ -> False }
  Task.succeed ({ refused: refused, rows: n } == { refused: True, rows: 1 })
}

test "a booking that starts on the last day of another is stored" = {
  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
  let room <- Db.insert Room { name: Text.NonEmpty "Blue" }
  let today <- Date.today
  let _ <- Db.insert Booking { room: room, owner: me, from: Date.addDays 2 today, to: Date.addDays 5 today }
  let _ <- Test.login { user: Just me }
  let _ <- Test.get (Rooms.Show { id: room })
  let r <- Test.submit NewBooking { from: Date.addDays 5 today, to: Date.addDays 8 today }
  let n <- Db.from Booking |> Db.where (Booking.room.eq room) |> Db.count
  Task.succeed ({ answer: r.answer, rows: n } == { answer: Redirected (Rooms.Show { id: room }), rows: 2 })
}

--- file: app/Bookings/Index.alder ---
module Bookings.Index

import Room
import Booking
import Layout

page "/bookings" { } requires user

type Row = { id: Booking.BookingId, room: Text, from: Date, to: Date }

type Model = { rows: List Row }

load : Params -> Session -> Query Model
load params session = {
  let mine <-
    Db.from Booking
      |> Db.where (Booking.owner.eq session.user)
      |> Db.orderBy Booking.from.asc
      |> Db.all
  let rows <- Query.all (List.map rowFor mine)
  Query.succeed { rows: rows }
}

rowFor : Booking -> Query Row
rowFor b = {
  let found <- Db.get Room b.room
  match found {
    Just r -> Query.succeed { id: b.id, room: r.name, from: b.from, to: b.to },
    Nothing -> Query.notFound,
  }
}

view : Model -> Session -> Html Msg
view model session =
  <Layout.Page title="Your bookings">
    <h1>Your bookings</h1>
    <ul id="mine">
      {for row in model.rows ->
        <li><a href={Bookings.Show { id: row.id }}><span class="room">{row.room}</span> <span class="from">{Date.toText row.from}</span></a></li>}
    </ul>
  </Layout.Page>

--- file: app/Bookings/Show.alder ---
module Bookings.Show

import Room
import Booking
import User
import Layout

page "/bookings/{id}" { id: Booking.BookingId } requires user

type Model = { booking: Booking, room: Room, today: Date }

load : Params -> Session -> Query Model
load params session = {
  let found <- Db.get Booking params.id
  let today <- Date.today
  match found {
    Just b -> if b.owner == session.user then withRoom b today else Query.notFound,
    Nothing -> Query.notFound,
  }
}

withRoom : Booking -> Date -> Query Model
withRoom b today = {
  let found <- Db.get Room b.room
  match found {
    Just room -> Query.succeed { booking: b, room: room, today: today },
    Nothing -> Query.notFound,
  }
}

-- A booking can be cancelled only before the day it starts.
cancelable : Model -> Bool
cancelable model = model.today < model.booking.from

msg Cancel

update : Msg -> Model -> Session -> Task Nav
update msg model session = match msg {
  Cancel p -> {
    let today <- Date.today
    if today < model.booking.from then {
      let _ <- Db.delete Booking model.booking.id
      Task.succeed (Redirect (Bookings.Index {}))
    } else Task.succeed Rerender,
  },
}

view : Model -> Session -> Html Msg
view model session =
  <Layout.Page title="Booking">
    <p id="room">{model.room.name}</p>
    <p>From <span id="from">{Date.toText model.booking.from}</span> to <span id="to">{Date.toText model.booking.to}</span></p>
    {if cancelable model then <button id="cancel" click={Cancel}>Cancel booking</button> else Html.none}
    <a href={Bookings.Index {}}>Your bookings</a>
  </Layout.Page>

test "the owner cancels a booking before it starts" = {
  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
  let room <- Db.insert Room { name: Text.NonEmpty "Blue" }
  let today <- Date.today
  let b <- Db.insert Booking { room: room, owner: me, from: Date.addDays 3 today, to: Date.addDays 5 today }
  let _ <- Test.login { user: Just me }
  let _ <- Test.get (Bookings.Show { id: b })
  let r <- Test.click Cancel
  let left <- Db.get Booking b
  Task.succeed ({ answer: r.answer, found: Maybe.isJust left } == { answer: Redirected (Bookings.Index {}), found: False })
}

test "a booking that starts today cannot be cancelled" = {
  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
  let room <- Db.insert Room { name: Text.NonEmpty "Blue" }
  let today <- Date.today
  let b <- Db.insert Booking { room: room, owner: me, from: today, to: Date.addDays 2 today }
  let _ <- Test.login { user: Just me }
  let page <- Test.get (Bookings.Show { id: b })
  let r <- Test.run (update Cancel page.model { user: me })
  let left <- Db.get Booking b
  Task.succeed ({ button: Test.has "#cancel" page.doc, value: r.value, found: Maybe.isJust left } == { button: False, value: Ok Rerender, found: True })
}

--- file: app/Auth/Login.alder ---
module Auth.Login

import User
import Layout

-- Password checking is outside the language: a capability implemented in Go.
capability Password {
  verify : { email: Text.Email, password: Text } -> Task (Maybe User.UserId),
}

-- In a test, `Test.fake Password` puts this in the adapter's place.
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
    <form submit={Submit}>
      <Errors form />
      <Input field="email" label="Email" />
      <Errors field="email" />
      <Input field="password" label="Password" />
      <Errors field="password" />
      <button>Log in</button>
    </form>
  </Layout.Page>
