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

-- The first date, `today` or later, that none of the (non-overlapping) bookings holds.
nextFree : Date -> List Booking -> Date
nextFree today bookings =
  List.foldl
    (\b d -> if b.from <= d && b.to > d then b.to else d)
    today
    (List.sortByText (\b -> Date.toText b.from) bookings)

sample : Date -> Date -> Booking
sample f t = { id: BookingId "x", room: Room.RoomId "r", owner: User.UserId "u", from: f, to: t }

test "next free date skips back-to-back bookings" = {
  let today <- Date.today
  let a = Date.addDays 2 today
  let b = Date.addDays 5 today
  Task.succeed ({
    none: nextFree today [],
    back: nextFree today [sample a b, sample today a],
    gap: nextFree today [sample a b],
    later: nextFree today [sample b (Date.addDays 7 today), sample today a],
  } == { none: today, back: b, gap: today, later: a })
}

--- file: app/Layout.alder ---
module Layout

Page : { title: Text, children: List (Html a) } -> Html a
Page props =
  <html>
    <head>
      <meta charset="utf-8" />
      <title>{props.title}</title>
      <link rel="stylesheet" href={Url.external "/static/app.css"} />
    </head>
    <body>
      <main>{props.children}</main>
    </body>
  </html>

Member : { title: Text, children: List (Html a) } -> Html a
Member props =
  <html>
    <head>
      <meta charset="utf-8" />
      <title>{props.title}</title>
      <link rel="stylesheet" href={Url.external "/static/app.css"} />
    </head>
    <body>
      <nav>
        <a href={Rooms.Index {}}>Rooms</a>
        <a href={Bookings.Index {}}>My bookings</a>
        <button click={App.Logout}>Log out</button>
      </nav>
      <main>{props.children}</main>
    </body>
  </html>

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

--- file: app/Auth/Login.alder ---
module Auth.Login

import User
import Layout

-- Password checking is outside the language: a capability implemented in Go.
capability Password {
  verify : { email: Text.Email, password: Text } -> Task (Maybe User.UserId),
}

-- In a test, `Test.fake Password` puts this in the adapter's place: it accepts any user who has
-- a row, whatever the password.
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

--- file: app/Rooms/Index.alder ---
module Rooms.Index

import Room
import Booking
import Layout

page "/rooms" { } requires user

type Row = { room: Room, free: Date }

type Model = { rows: List Row }

load : Params -> Session -> Query Model
load params session = {
  let today <- Date.today
  let rooms <- Db.from Room |> Db.orderBy Room.name.asc |> Db.all
  let bookings <- Db.from Booking |> Db.where (Booking.to.gt today) |> Db.all
  Query.succeed { rows: List.map (rowOf today bookings) rooms }
}

rowOf : Date -> List Booking -> Room -> Row
rowOf today bookings room =
  { room: room, free: Booking.nextFree today (List.filter (\b -> b.room == room.id) bookings) }

view : Model -> Session -> Html Msg
view model session =
  <Layout.Member title="Rooms">
    <h1>Rooms</h1>
    <ul id="rooms">
      {for row in model.rows ->
        <li>
          <a class="name" href={Rooms.Show { id: row.room.id }}>{row.room.name}</a>
          <span class="free">{Date.toText row.free}</span>
        </li>}
    </ul>
  </Layout.Member>

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
    Just room -> forRoom room,
    Nothing -> Query.notFound,
  }
}

forRoom : Room -> Query Model
forRoom room = {
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

overlapMessage : Text
overlapMessage = "The room is already booked during those dates."

orderMessage : Text
orderMessage = "The booking must end after it starts."

update : Msg -> Model -> Session -> Task Nav
update msg model session = match msg {
  Book p -> book model session p.form,
}

book : Model -> Session -> NewBooking -> Task Nav
book model session f = {
  let conflicts <-
    Db.from Booking
      |> Db.where (Booking.room.eq model.room.id)
      |> Db.where (Booking.from.lt f.to)
      |> Db.where (Booking.to.gt f.from)
      |> Db.count
  decide model session f conflicts
}

decide : Model -> Session -> NewBooking -> Int -> Task Nav
decide model session f conflicts =
  if f.to <= f.from then Task.succeed (Reject orderMessage)
  else if conflicts > 0 then Task.succeed (Reject overlapMessage)
  else store model session f

store : Model -> Session -> NewBooking -> Task Nav
store model session f = {
  let _ <- Db.insert Booking { room: model.room.id, owner: session.user, from: f.from, to: f.to }
  Task.succeed (Redirect (Rooms.Show { id: model.room.id }))
}

view : Model -> Session -> Html Msg
view model session =
  <Layout.Member title={model.room.name}>
    <h1 id="name">{model.room.name}</h1>
    <ul id="bookings">
      {for b in model.bookings ->
        <li>
          <span class="from">{Date.toText b.from}</span>
          <span class="to">{Date.toText b.to}</span>
          {if b.owner == session.user then <a href={Bookings.Show { id: b.id }}>Open</a> else Html.none}
        </li>}
    </ul>
    <form submit={Book}>
      <Errors form />
      <Input field="from" label="From" />
      <Errors field="from" />
      <Input field="to" label="To" />
      <Errors field="to" />
      <button>Book</button>
    </form>
  </Layout.Member>

test "an overlapping booking is refused, a back-to-back one is stored" = {
  let today <- Date.today
  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
  let room <- Db.insert Room { name: Text.NonEmpty "Blue" }
  let _ <- Db.insert Booking { room: room, owner: me, from: Date.addDays 1 today, to: Date.addDays 3 today }
  let _ <- Test.login { user: Just me }
  let _ <- Test.get (Rooms.Show { id: room })
  let clash <- Test.submit NewBooking { from: Date.addDays 2 today, to: Date.addDays 4 today }
  let n1 <- Db.from Booking |> Db.count
  let ok <- Test.submit NewBooking { from: Date.addDays 3 today, to: Date.addDays 5 today }
  let n2 <- Db.from Booking |> Db.count
  Task.succeed ({ clash: clash.answer, n1: n1, ok: ok.answer, n2: n2 } == { clash: FormRejected [{ field: "", message: overlapMessage }], n1: 1, ok: Redirected (Rooms.Show { id: room }), n2: 2 })
}

test "bookings of different rooms do not conflict" = {
  let today <- Date.today
  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
  let blue <- Db.insert Room { name: Text.NonEmpty "Blue" }
  let red <- Db.insert Room { name: Text.NonEmpty "Red" }
  let _ <- Db.insert Booking { room: blue, owner: me, from: Date.addDays 1 today, to: Date.addDays 3 today }
  let _ <- Test.login { user: Just me }
  let _ <- Test.get (Rooms.Show { id: red })
  let r <- Test.submit NewBooking { from: Date.addDays 1 today, to: Date.addDays 3 today }
  Task.succeed (r.answer == Redirected (Rooms.Show { id: red }))
}

test "a booking that ends before it starts is refused" = {
  let today <- Date.today
  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
  let room <- Db.insert Room { name: Text.NonEmpty "Blue" }
  let _ <- Test.login { user: Just me }
  let _ <- Test.get (Rooms.Show { id: room })
  let r <- Test.submit NewBooking { from: Date.addDays 3 today, to: Date.addDays 3 today }
  let n <- Db.from Booking |> Db.count
  Task.succeed ({ answer: r.answer, n: n } == { answer: FormRejected [{ field: "", message: orderMessage }], n: 0 })
}

--- file: app/Bookings/Index.alder ---
module Bookings.Index

import Room
import Booking
import Layout

page "/bookings" { } requires user

type Row = { booking: Booking, roomName: Text }

type Model = { rows: List Row }

load : Params -> Session -> Query Model
load params session = {
  let bookings <-
    Db.from Booking
      |> Db.where (Booking.owner.eq session.user)
      |> Db.orderBy Booking.from.asc
      |> Db.all
  let rooms <- Db.from Room |> Db.all
  Query.succeed { rows: List.map (rowOf rooms) bookings }
}

rowOf : List Room -> Booking -> Row
rowOf rooms b = {
  let found = List.find (\r -> r.id == b.room) rooms
  let name = match found {
    Just r -> Text.trim r.name,
    Nothing -> "",
  }
  { booking: b, roomName: name }
}

view : Model -> Session -> Html Msg
view model session =
  <Layout.Member title="My bookings">
    <h1>My bookings</h1>
    <ul id="mine">
      {for row in model.rows ->
        <li>
          <span class="room">{row.roomName}</span>
          <span class="from">{Date.toText row.booking.from}</span>
          <a href={Bookings.Show { id: row.booking.id }}>Details</a>
        </li>}
    </ul>
  </Layout.Member>

--- file: app/Bookings/Show.alder ---
module Bookings.Show

import Room
import Booking
import Layout
import User

page "/bookings/{id}" { id: Booking.BookingId } requires user

type Model = { booking: Booking, room: Room, canCancel: Bool }

load : Params -> Session -> Query Model
load params session = {
  let found <- Db.get Booking params.id
  match found {
    Just b -> if b.owner == session.user then build b else Query.notFound,
    Nothing -> Query.notFound,
  }
}

build : Booking -> Query Model
build b = {
  let today <- Date.today
  let found <- Db.get Room b.room
  match found {
    Just room -> Query.succeed { booking: b, room: room, canCancel: today < b.from },
    Nothing -> Query.notFound,
  }
}

msg Cancel

update : Msg -> Model -> Session -> Task Nav
update msg model session = match msg {
  Cancel p -> {
    let today <- Date.today
    if today < model.booking.from then remove model else Task.succeed Rerender
  },
}

remove : Model -> Task Nav
remove model = {
  let _ <- Db.delete Booking model.booking.id
  Task.succeed (Redirect (Bookings.Index {}))
}

view : Model -> Session -> Html Msg
view model session =
  <Layout.Member title="Booking">
    <h1>Booking</h1>
    <p>Room: <span id="room">{model.room.name}</span></p>
    <p>From: <span id="from">{Date.toText model.booking.from}</span></p>
    <p>To: <span id="to">{Date.toText model.booking.to}</span></p>
    {if model.canCancel then <button id="cancel" click={Cancel}>Cancel</button> else Html.none}
    <a href={Bookings.Index {}}>Back</a>
  </Layout.Member>

test "the owner cancels a booking before its from date" = {
  let today <- Date.today
  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
  let room <- Db.insert Room { name: Text.NonEmpty "Blue" }
  let id <- Db.insert Booking { room: room, owner: me, from: Date.addDays 2 today, to: Date.addDays 4 today }
  let _ <- Test.login { user: Just me }
  let page <- Test.get (Bookings.Show { id: id })
  let r <- Test.click Cancel
  let after <- Db.get Booking id
  Task.succeed ({ button: Test.has "#cancel" page.doc, answer: r.answer, stays: Maybe.isJust after } == { button: True, answer: Redirected (Bookings.Index {}), stays: False })
}

test "a booking whose from date is today cannot be cancelled" = {
  let today <- Date.today
  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
  let room <- Db.insert Room { name: Text.NonEmpty "Blue" }
  let id <- Db.insert Booking { room: room, owner: me, from: today, to: Date.addDays 2 today }
  let _ <- Test.login { user: Just me }
  let page <- Test.get (Bookings.Show { id: id })
  let r <- Test.run (update Cancel page.model { user: me })
  let after <- Db.get Booking id
  Task.succeed ({ button: Test.has "#cancel" page.doc, value: r.value, stays: Maybe.isJust after } == { button: False, value: Ok Rerender, stays: True })
}

test "another user's booking is a 404" = {
  let today <- Date.today
  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
  let room <- Db.insert Room { name: Text.NonEmpty "Blue" }
  let id <- Db.insert Booking { room: room, owner: me, from: Date.addDays 2 today, to: Date.addDays 4 today }
  let _ <- Test.login { user: Just (User.UserId "someone-else") }
  let r <- Test.open (Bookings.Show { id: id })
  Task.succeed (r.answer == PageMissing)
}
