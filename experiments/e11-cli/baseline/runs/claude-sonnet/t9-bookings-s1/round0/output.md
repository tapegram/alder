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

table Booking {
  id: BookingId @id,
  room: Room.RoomId @ref Room,
  owner: User.UserId @ref User,
  from: Date,
  to: Date,
}

orderText : Text
orderText = "The end date must be after the start date."

overlapText : Text
overlapText = "The room is already booked for those dates."

startedText : Text
startedText = "This booking has already started and cannot be cancelled."

-- A booking holds its from date up to, but not including, its to date.
overlaps : Date -> Date -> Booking -> Bool
overlaps start end b = b.from < end && start < b.to

refusal : Date -> Date -> List Booking -> Maybe Text
refusal start end existing =
  if end <= start then Just orderText else if List.any (overlaps start end) existing then Just overlapText else Nothing

step : Booking -> Date -> Date
step b d = if b.from <= d && d < b.to then b.to else d

-- The first date, today or later, that none of the bookings holds.
nextFree : Date -> List Booking -> Date
nextFree today bookings =
  List.foldl step today (List.sortByText (\b -> Date.toText b.from) bookings)

canCancel : Date -> Booking -> Bool
canCancel today b = today < b.from

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
        <main>{props.children}</main>
      </al-page>
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

capability Password {
  verify : { email: Text.Email, password: Text } -> Task (Maybe User.UserId),
}

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
      <al-input field="email" label="Email" />
      <al-input field="password" label="Password" />
      <al-button type="submit">Log in</al-button>
    </form>
  </Layout.Page>

test "logging in through the form" = {
  let _ <- Test.fake Password
  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
  let _ <- Test.get (Auth.Login {})
  let r <- Test.submit Credentials { email: Text.Email "me@example.com", password: Text.Secret "secret" }
  Task.succeed ({ answer: r.answer, user: r.session.user } == { answer: Redirected (Rooms.Index {}), user: Just me })
}

--- file: app/Rooms/Index.alder ---
module Rooms.Index

import Room
import Booking
import User
import Layout

page "/rooms" { } requires user

type Row = { id: Room.RoomId, name: Text, free: Date }

type Model = { rows: List Row }

row : Date -> List Booking -> Room -> Row
row today bookings r = {
  let mine = List.filter (\b -> b.room == r.id) bookings
  { id: r.id, name: Text.trim r.name, free: Booking.nextFree today mine }
}

load : Params -> Session -> Query Model
load params session = {
  let rooms <- Db.from Room |> Db.orderBy Room.name.asc |> Db.all
  let bookings <- Db.from Booking |> Db.all
  let today <- Date.today
  Query.succeed { rows: List.map (row today bookings) rooms }
}

view : Model -> Session -> Html Msg
view model session =
  <Layout.Page title="Rooms">
    <h1>Rooms</h1>
    <nav>
      <a href={Bookings.Index {}}>My bookings</a>
      <button click={App.Logout}>Log out</button>
    </nav>
    <ul id="rooms">
      {for r in model.rows ->
        <li>
          <a class="name" href={Rooms.Show { id: r.id }}>{r.name}</a>
          <span class="free">{Date.toText r.free}</span>
        </li>}
    </ul>
  </Layout.Page>

test "next free date of a room with back-to-back bookings" = {
  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
  let room <- Db.insert Room { name: Text.NonEmpty "Blue" }
  let today <- Date.today
  let _ <- Db.insert Booking { room: room, owner: me, from: today, to: Date.addDays 2 today }
  let _ <- Db.insert Booking { room: room, owner: me, from: Date.addDays 2 today, to: Date.addDays 5 today }
  let _ <- Test.login { user: Just me }
  let page <- Test.get (Rooms.Index {})
  Task.succeed ({ names: Test.texts "#rooms .name" page.doc, free: Test.texts "#rooms .free" page.doc } == { names: ["Blue"], free: [Date.toText (Date.addDays 5 today)] })
}

--- file: app/Rooms/Show.alder ---
module Rooms.Show

import Room
import Booking
import User
import Layout

page "/rooms/{id}" { id: Room.RoomId } requires user

type Model = { room: Room, bookings: List Booking }

load : Params -> Session -> Query Model
load params session = {
  let found <- Db.get Room params.id
  match found {
    Just room -> {
      let bookings <- Db.from Booking |> Db.where (Booking.room.eq params.id) |> Db.orderBy Booking.from.asc |> Db.all
      Query.succeed { room: room, bookings: bookings }
    },
    Nothing -> Query.notFound,
  }
}

form NewBooking = {
  from: Date,
  to: Date,
}

msg Book form NewBooking

update : Msg -> Model -> Session -> Task Nav
update msg model session = match msg {
  Book p -> {
    let existing <- Db.from Booking |> Db.where (Booking.room.eq model.room.id) |> Db.all
    match Booking.refusal p.form.from p.form.to existing {
      Just why -> Task.succeed (Reject why),
      Nothing -> {
        let _ <- Db.insert Booking { room: model.room.id, owner: session.user, from: p.form.from, to: p.form.to }
        Task.succeed (Redirect (Rooms.Show { id: model.room.id }))
      },
    }
  },
}

view : Model -> Session -> Html Msg
view model session =
  <Layout.Page title={model.room.name}>
    <h1 id="name">{model.room.name}</h1>
    <ul id="bookings">
      {for b in model.bookings ->
        <li><span class="from">{Date.toText b.from}</span> <span class="to">{Date.toText b.to}</span></li>}
    </ul>
    <form submit={Book}>
      <Errors form />
      <al-input field="from" label="From" />
      <al-input field="to" label="To" />
      <al-button type="submit">Book</al-button>
    </form>
    <a href={Rooms.Index {}}>All rooms</a>
  </Layout.Page>

test "an overlapping booking is refused and not stored" = {
  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
  let room <- Db.insert Room { name: Text.NonEmpty "Blue" }
  let today <- Date.today
  let _ <- Db.insert Booking { room: room, owner: me, from: today, to: Date.addDays 3 today }
  let _ <- Test.login { user: Just me }
  let _ <- Test.get (Rooms.Show { id: room })
  let r <- Test.submit NewBooking { from: Date.addDays 2 today, to: Date.addDays 5 today }
  let n <- Db.from Booking |> Db.count
  Task.succeed ({ answer: r.answer, n: n } == { answer: FormRejected [{ field: "", message: Booking.overlapText }], n: 1 })
}

test "a booking starting on another's to date is stored" = {
  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
  let room <- Db.insert Room { name: Text.NonEmpty "Blue" }
  let today <- Date.today
  let _ <- Db.insert Booking { room: room, owner: me, from: today, to: Date.addDays 3 today }
  let _ <- Test.login { user: Just me }
  let _ <- Test.get (Rooms.Show { id: room })
  let r <- Test.submit NewBooking { from: Date.addDays 3 today, to: Date.addDays 5 today }
  let n <- Db.from Booking |> Db.count
  Task.succeed ({ answer: r.answer, n: n } == { answer: Redirected (Rooms.Show { id: room }), n: 2 })
}

test "a booking whose to is not after its from is refused" = {
  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
  let room <- Db.insert Room { name: Text.NonEmpty "Blue" }
  let today <- Date.today
  let _ <- Test.login { user: Just me }
  let _ <- Test.get (Rooms.Show { id: room })
  let r <- Test.submit NewBooking { from: Date.addDays 2 today, to: Date.addDays 2 today }
  let n <- Db.from Booking |> Db.count
  Task.succeed ({ answer: r.answer, n: n } == { answer: FormRejected [{ field: "", message: Booking.orderText }], n: 0 })
}

test "bookings of different rooms never conflict" = {
  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
  let blue <- Db.insert Room { name: Text.NonEmpty "Blue" }
  let red <- Db.insert Room { name: Text.NonEmpty "Red" }
  let today <- Date.today
  let _ <- Db.insert Booking { room: blue, owner: me, from: today, to: Date.addDays 3 today }
  let _ <- Test.login { user: Just me }
  let _ <- Test.get (Rooms.Show { id: red })
  let r <- Test.submit NewBooking { from: today, to: Date.addDays 3 today }
  Task.succeed (r.answer == Redirected (Rooms.Show { id: red }))
}

--- file: app/Bookings/Index.alder ---
module Bookings.Index

import Room
import Booking
import User
import Layout

page "/bookings" { } requires user

type Row = { id: Booking.BookingId, room: Text, from: Date }

type Model = { rows: List Row }

row : List Room -> Booking -> Row
row rooms b = {
  let found = List.find (\r -> r.id == b.room) rooms
  { id: b.id, room: Maybe.withDefault "" (Maybe.map (\r -> Text.trim r.name) found), from: b.from }
}

load : Params -> Session -> Query Model
load params session = {
  let bookings <- Db.from Booking |> Db.where (Booking.owner.eq session.user) |> Db.orderBy Booking.from.asc |> Db.all
  let rooms <- Db.from Room |> Db.all
  Query.succeed { rows: List.map (row rooms) bookings }
}

view : Model -> Session -> Html Msg
view model session =
  <Layout.Page title="My bookings">
    <h1>My bookings</h1>
    <ul id="mine">
      {for r in model.rows ->
        <li>
          <a href={Bookings.Show { id: r.id }}><span class="room">{r.room}</span> <span class="from">{Date.toText r.from}</span></a>
        </li>}
    </ul>
    <nav>
      <a href={Rooms.Index {}}>Rooms</a>
      <button click={App.Logout}>Log out</button>
    </nav>
  </Layout.Page>

test "my bookings lists only my own" = {
  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
  let other <- Db.insert User { email: Text.Email "other@example.com", name: Text.NonEmpty "Other", passwordHash: "" }
  let room <- Db.insert Room { name: Text.NonEmpty "Blue" }
  let today <- Date.today
  let _ <- Db.insert Booking { room: room, owner: me, from: today, to: Date.addDays 1 today }
  let _ <- Db.insert Booking { room: room, owner: other, from: Date.addDays 4 today, to: Date.addDays 5 today }
  let _ <- Test.login { user: Just me }
  let page <- Test.get (Bookings.Index {})
  Task.succeed ({ rooms: Test.texts "#mine .room" page.doc, froms: Test.texts "#mine .from" page.doc } == { rooms: ["Blue"], froms: [Date.toText today] })
}

--- file: app/Bookings/Show.alder ---
module Bookings.Show

import Room
import Booking
import User
import Layout

page "/bookings/{id}" { id: Booking.BookingId } requires user

type Model = { booking: Booking, roomName: Text, today: Date }

load : Params -> Session -> Query Model
load params session = {
  let found <- Db.get Booking params.id
  let today <- Date.today
  match found {
    Just b -> if b.owner == session.user then {
      let room <- Db.get Room b.room
      Query.succeed { booking: b, roomName: Maybe.withDefault "" (Maybe.map (\r -> Text.trim r.name) room), today: today }
    } else Query.notFound,
    Nothing -> Query.notFound,
  }
}

msg Cancel

update : Msg -> Model -> Session -> Task Nav
update msg model session = match msg {
  Cancel p -> {
    let today <- Date.today
    if today < model.booking.from then {
      let _ <- Db.delete Booking model.booking.id
      Task.succeed (Redirect (Bookings.Index {}))
    } else Task.succeed (Reject Booking.startedText)
  },
}

view : Model -> Session -> Html Msg
view model session =
  <Layout.Page title="Booking">
    <h1>Booking</h1>
    <p>Room: <span id="room">{model.roomName}</span></p>
    <p>From: <span id="from">{Date.toText model.booking.from}</span></p>
    <p>To: <span id="to">{Date.toText model.booking.to}</span></p>
    {if Booking.canCancel model.today model.booking then <button id="cancel" click={Cancel}>Cancel</button> else Html.none}
    <a href={Bookings.Index {}}>My bookings</a>
  </Layout.Page>

test "the owner cancels a booking before its from date" = {
  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
  let room <- Db.insert Room { name: Text.NonEmpty "Blue" }
  let today <- Date.today
  let id <- Db.insert Booking { room: room, owner: me, from: Date.addDays 2 today, to: Date.addDays 4 today }
  let _ <- Test.login { user: Just me }
  let page <- Test.get (Bookings.Show { id: id })
  let r <- Test.click Cancel
  let n <- Db.from Booking |> Db.count
  Task.succeed ({ button: Test.has "#cancel" page.doc, answer: r.answer, n: n } == { button: True, answer: Redirected (Bookings.Index {}), n: 0 })
}

test "a booking whose from date is today cannot be cancelled" = {
  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
  let room <- Db.insert Room { name: Text.NonEmpty "Blue" }
  let today <- Date.today
  let id <- Db.insert Booking { room: room, owner: me, from: today, to: Date.addDays 2 today }
  let _ <- Test.login { user: Just me }
  let page <- Test.get (Bookings.Show { id: id })
  let r <- Test.run (update Cancel page.model { user: me })
  let n <- Db.from Booking |> Db.count
  Task.succeed ({ button: Test.has "#cancel" page.doc, value: r.value, n: n } == { button: False, value: Ok (Reject Booking.startedText), n: 1 })
}

test "another user gets a 404 for my booking" = {
  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
  let room <- Db.insert Room { name: Text.NonEmpty "Blue" }
  let today <- Date.today
  let id <- Db.insert Booking { room: room, owner: me, from: Date.addDays 2 today, to: Date.addDays 4 today }
  let _ <- Test.login { user: Just (User.UserId "someone-else") }
  let r <- Test.open (Bookings.Show { id: id })
  Task.succeed (r.answer == PageMissing)
}
