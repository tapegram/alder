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

clashMessage : Text
clashMessage = "This room is already booked for part of those dates."

backwardsMessage : Text
backwardsMessage = "The booking must end after it starts."

-- A booking holds its `from` date up to, but not including, its `to` date.
holds : Booking -> Date -> Bool
holds b d = b.from <= d && d < b.to

overlaps : Date -> Date -> Booking -> Bool
overlaps start finish b = start < b.to && b.from < finish

-- Why a booking of [start, finish) next to the room's existing bookings is refused, if it is.
refusal : Date -> Date -> List Booking -> Maybe Text
refusal start finish existing = {
  let backwards = not (start < finish)
  let clash = List.any (overlaps start finish) existing
  if backwards then Just backwardsMessage else if clash then Just clashMessage else Nothing
}

-- The first date, today or later, that none of the bookings holds.
nextFree : Date -> List Booking -> Date
nextFree today bookings = {
  let sorted = List.sortByText (\b -> Date.toText b.from) bookings
  List.foldl (\b free -> if holds b free then b.to else free) today sorted
}

canCancel : Date -> Booking -> Bool
canCancel today b = today < b.from

--- file: app/Auth/Login.alder ---
module Auth.Login

import User
import Layout

-- Password checking is outside the language: a capability implemented in Go.
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
import User
import Layout

page "/rooms" { } requires user

type Entry = { room: Room, free: Date }

type Model = { entries: List Entry }

entry : Date -> List Booking -> Room -> Entry
entry today bookings r = { room: r, free: Booking.nextFree today (List.filter (\b -> b.room == r.id) bookings) }

load : Params -> Session -> Query Model
load params session = {
  let today <- Date.today
  let rooms <- Db.from Room |> Db.orderBy Room.name.asc |> Db.all
  let bookings <- Db.from Booking |> Db.all
  Query.succeed { entries: List.map (entry today bookings) rooms }
}

view : Model -> Session -> Html Msg
view model session =
  <Layout.Page title="Rooms">
    <nav>
      <a href={Rooms.Index {}}>Rooms</a>
      <a href={Bookings.Index {}}>My bookings</a>
      <button click={App.Logout}>Log out</button>
    </nav>
    <h1>Rooms</h1>
    <ul id="rooms">
      {for e in model.entries ->
        <li>
          <a class="name" href={Rooms.Show { id: e.room.id }}>{e.room.name}</a>
          <span class="free">{Date.toText e.free}</span>
        </li>}
    </ul>
  </Layout.Page>

test "the next free date of rooms with back-to-back bookings" = {
  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
  let a <- Db.insert Room { name: Text.NonEmpty "A" }
  let b <- Db.insert Room { name: Text.NonEmpty "B" }
  let today <- Date.today
  let _ <- Db.insert Booking { room: a, owner: me, from: today, to: Date.addDays 2 today }
  let _ <- Db.insert Booking { room: a, owner: me, from: Date.addDays 2 today, to: Date.addDays 5 today }
  let _ <- Db.insert Booking { room: b, owner: me, from: Date.addDays (-3) today, to: today }
  let _ <- Db.insert Booking { room: b, owner: me, from: Date.addDays 1 today, to: Date.addDays 2 today }
  let _ <- Test.login { user: Just me }
  let page <- Test.get (Rooms.Index {})
  Task.succeed (Test.texts "#rooms .free" page.doc == [Date.toText (Date.addDays 5 today), Date.toText today])
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
    Just r -> {
      let bookings <- Db.from Booking |> Db.where (Booking.room.eq r.id) |> Db.orderBy Booking.from.asc |> Db.all
      Query.succeed { room: r, bookings: bookings }
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
    let verdict = Booking.refusal p.form.from p.form.to existing
    match verdict {
      Just reason -> Task.succeed (Reject reason),
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
    <nav>
      <a href={Rooms.Index {}}>Rooms</a>
      <a href={Bookings.Index {}}>My bookings</a>
      <button click={App.Logout}>Log out</button>
    </nav>
    <h1 id="name">{model.room.name}</h1>
    <ul id="bookings">
      {for b in model.bookings ->
        <li><span class="from">{Date.toText b.from}</span> <span class="to">{Date.toText b.to}</span></li>}
    </ul>
    <form submit={Book}>
      <Errors form />
      <Input field="from" label="From" />
      <Errors field="from" />
      <Input field="to" label="To" />
      <Errors field="to" />
      <button>Book</button>
    </form>
  </Layout.Page>

test "an overlapping booking is refused and not stored" = {
  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
  let room <- Db.insert Room { name: Text.NonEmpty "Blue" }
  let today <- Date.today
  let _ <- Db.insert Booking { room: room, owner: me, from: Date.addDays 1 today, to: Date.addDays 3 today }
  let _ <- Test.login { user: Just me }
  let _ <- Test.get (Rooms.Show { id: room })
  let r <- Test.submit NewBooking { from: Date.addDays 2 today, to: Date.addDays 4 today }
  let n <- Db.from Booking |> Db.count
  Task.succeed ({ answer: r.answer, count: n } == { answer: FormRejected [{ field: "", message: Booking.clashMessage }], count: 1 })
}

test "a booking that starts on the other's to date is stored" = {
  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
  let room <- Db.insert Room { name: Text.NonEmpty "Blue" }
  let today <- Date.today
  let _ <- Db.insert Booking { room: room, owner: me, from: Date.addDays 1 today, to: Date.addDays 3 today }
  let _ <- Test.login { user: Just me }
  let _ <- Test.get (Rooms.Show { id: room })
  let r <- Test.submit NewBooking { from: Date.addDays 3 today, to: Date.addDays 5 today }
  let n <- Db.from Booking |> Db.count
  Task.succeed ({ answer: r.answer, count: n } == { answer: Redirected (Rooms.Show { id: room }), count: 2 })
}

test "a booking of another room never conflicts" = {
  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
  let blue <- Db.insert Room { name: Text.NonEmpty "Blue" }
  let red <- Db.insert Room { name: Text.NonEmpty "Red" }
  let today <- Date.today
  let _ <- Db.insert Booking { room: blue, owner: me, from: Date.addDays 1 today, to: Date.addDays 3 today }
  let _ <- Test.login { user: Just me }
  let _ <- Test.get (Rooms.Show { id: red })
  let r <- Test.submit NewBooking { from: Date.addDays 1 today, to: Date.addDays 3 today }
  let n <- Db.from Booking |> Db.count
  Task.succeed ({ answer: r.answer, count: n } == { answer: Redirected (Rooms.Show { id: red }), count: 2 })
}

test "a booking that ends before it starts is refused" = {
  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
  let room <- Db.insert Room { name: Text.NonEmpty "Blue" }
  let today <- Date.today
  let _ <- Test.login { user: Just me }
  let _ <- Test.get (Rooms.Show { id: room })
  let r <- Test.submit NewBooking { from: Date.addDays 2 today, to: Date.addDays 2 today }
  let n <- Db.from Booking |> Db.count
  Task.succeed ({ answer: r.answer, count: n } == { answer: FormRejected [{ field: "", message: Booking.backwardsMessage }], count: 0 })
}

--- file: app/Bookings/Index.alder ---
module Bookings.Index

import Room
import Booking
import User
import Layout

page "/bookings" { } requires user

type Row = { booking: Booking, roomName: Text }

type Model = { rows: List Row }

roomName : Room -> Text
roomName r = r.name

row : List Room -> Booking -> Row
row rooms b = { booking: b, roomName: Maybe.withDefault "?" (Maybe.map roomName (List.find (\r -> r.id == b.room) rooms)) }

load : Params -> Session -> Query Model
load params session = {
  let mine <- Db.from Booking |> Db.where (Booking.owner.eq session.user) |> Db.orderBy Booking.from.asc |> Db.all
  let rooms <- Db.from Room |> Db.all
  Query.succeed { rows: List.map (row rooms) mine }
}

view : Model -> Session -> Html Msg
view model session =
  <Layout.Page title="My bookings">
    <nav>
      <a href={Rooms.Index {}}>Rooms</a>
      <a href={Bookings.Index {}}>My bookings</a>
      <button click={App.Logout}>Log out</button>
    </nav>
    <h1>My bookings</h1>
    <ul id="mine">
      {for r in model.rows ->
        <li>
          <a href={Bookings.Show { id: r.booking.id }}><span class="room">{r.roomName}</span> <span class="from">{Date.toText r.booking.from}</span></a>
        </li>}
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

withRoom : Booking -> Date -> Query Model
withRoom b today = {
  let found <- Db.get Room b.room
  match found {
    Just r -> Query.succeed { booking: b, room: r, today: today },
    Nothing -> Query.notFound,
  }
}

load : Params -> Session -> Query Model
load params session = {
  let found <- Db.get Booking params.id
  let today <- Date.today
  match found {
    Just b -> if b.owner == session.user then withRoom b today else Query.notFound,
    Nothing -> Query.notFound,
  }
}

msg Cancel

-- The page's parameter names the booking and `load` has checked that the session's user owns
-- it, so `update` acts on `model.booking`. The date is checked again: the page may be stale.
update : Msg -> Model -> Session -> Task Nav
update msg model session = match msg {
  Cancel p -> {
    let today <- Date.today
    if Booking.canCancel today model.booking then {
      let _ <- Db.delete Booking model.booking.id
      Task.succeed (Redirect (Bookings.Index {}))
    } else Task.succeed (Reject "This booking has started and can no longer be cancelled.")
  },
}

view : Model -> Session -> Html Msg
view model session =
  <Layout.Page title="Booking">
    <nav>
      <a href={Rooms.Index {}}>Rooms</a>
      <a href={Bookings.Index {}}>My bookings</a>
      <button click={App.Logout}>Log out</button>
    </nav>
    <h1>Booking</h1>
    <p>Room: <span id="room">{model.room.name}</span></p>
    <p>From <span id="from">{Date.toText model.booking.from}</span> to <span id="to">{Date.toText model.booking.to}</span></p>
    {if Booking.canCancel model.today model.booking then <button id="cancel" click={Cancel}>Cancel</button> else Html.none}
  </Layout.Page>

test "the owner cancels a booking before its from date" = {
  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
  let room <- Db.insert Room { name: Text.NonEmpty "Blue" }
  let today <- Date.today
  let id <- Db.insert Booking { room: room, owner: me, from: Date.addDays 1 today, to: Date.addDays 2 today }
  let _ <- Test.login { user: Just me }
  let page <- Test.get (Bookings.Show { id: id })
  let r <- Test.click Cancel
  let after <- Db.get Booking id
  Task.succeed ({ button: Test.has "#cancel" page.doc, answer: r.answer, stays: Maybe.isJust after } == { button: True, answer: Redirected (Bookings.Index {}), stays: False })
}

test "a booking whose from date is today cannot be cancelled" = {
  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
  let room <- Db.insert Room { name: Text.NonEmpty "Blue" }
  let today <- Date.today
  let id <- Db.insert Booking { room: room, owner: me, from: today, to: Date.addDays 2 today }
  let _ <- Test.login { user: Just me }
  let page <- Test.get (Bookings.Show { id: id })
  let r <- Test.run (update Cancel page.model { user: me })
  let after <- Db.get Booking id
  Task.succeed ({ button: Test.has "#cancel" page.doc, stays: Maybe.isJust after } == { button: False, stays: True })
}

test "a stranger's booking page is not found" = {
  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
  let room <- Db.insert Room { name: Text.NonEmpty "Blue" }
  let today <- Date.today
  let id <- Db.insert Booking { room: room, owner: me, from: Date.addDays 1 today, to: Date.addDays 2 today }
  let _ <- Test.login { user: Just (User.UserId "someone-else") }
  let r <- Test.open (Bookings.Show { id: id })
  Task.succeed (r.answer == PageMissing)
}
