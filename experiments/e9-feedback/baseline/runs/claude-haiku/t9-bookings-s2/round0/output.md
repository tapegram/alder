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

--- file: app/Auth/Login.alder ---
module Auth.Login

import User
import Layout

-- Password checking is outside the language: a capability implemented in Go.
capability Password {
  verify : { email: Text.Email, password: Text } -> Task (Maybe User.UserId),
}

-- In a test, `Test.fake Password` may replace the adapter with this: it accepts any user who has
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

--- file: app/Room.alder ---
module Room

type RoomId = brand Text

table Room {
  id: RoomId @id,
  name: Text.NonEmpty,
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

-- Two half-open spans [aFrom, aTo) and [bFrom, bTo) share a day.
clash : Date -> Date -> Date -> Date -> Bool
clash aFrom aTo bFrom bTo = aFrom < bTo && bFrom < aTo

-- Whether the span [start, stop) shares a day with the booking.
overlaps : Date -> Date -> Booking -> Bool
overlaps start stop b = clash start stop b.from b.to

-- A booking can be cancelled while its first day is still ahead.
cancellable : Date -> Booking -> Bool
cancellable today b = today < b.from

-- The first day, from `day` on, that no booking holds. A booking holds its days from `from` up to,
-- but not including, `to`, so a booking that starts on another's `to` leaves no gap.
freeFrom : Date -> List Booking -> Date
freeFrom day bookings = {
  let busy = List.find (\b -> b.from <= day && day < b.to) bookings
  match busy {
    Just b -> freeFrom b.to bookings,
    Nothing -> day,
  }
}

test "a booking starting on another's last day does not clash" = {
  let today <- Date.today
  Task.succeed (not (clash today (Date.addDays 3 today) (Date.addDays 3 today) (Date.addDays 5 today)))
}

test "overlapping spans clash" = {
  let today <- Date.today
  Task.succeed (clash today (Date.addDays 3 today) (Date.addDays 2 today) (Date.addDays 5 today))
}

test "back-to-back bookings leave the day after them free" = {
  let today <- Date.today
  let spans : List Booking = [
    { id: BookingId "a", room: Room.RoomId "r", owner: User.UserId "u", from: today, to: Date.addDays 2 today },
    { id: BookingId "b", room: Room.RoomId "r", owner: User.UserId "u", from: Date.addDays 2 today, to: Date.addDays 5 today },
  ]
  Task.succeed (freeFrom today spans == Date.addDays 5 today)
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
  let rooms <- Db.from Room |> Db.orderBy Room.name.asc |> Db.all
  let bookings <- Db.from Booking |> Db.all
  let today <- Date.today
  Query.succeed { rows: List.map (rowFor today bookings) rooms }
}

rowFor : Date -> List Booking -> Room -> Row
rowFor today bookings room = {
  room: room,
  free: Booking.freeFrom today (List.filter (\b -> b.room == room.id) bookings),
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
    <nav>
      <a href={Bookings.Index {}}>My bookings</a>
    </nav>
    <button click={App.Logout}>Log out</button>
  </Layout.Page>

test "a room's next free date skips back-to-back bookings" = {
  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
  let room <- Db.insert Room { name: Text.NonEmpty "Oak" }
  let today <- Date.today
  let _ <- Db.insert Booking { room: room, owner: me, from: today, to: Date.addDays 2 today }
  let _ <- Db.insert Booking { room: room, owner: me, from: Date.addDays 2 today, to: Date.addDays 5 today }
  let _ <- Test.login { user: Just me }
  let page <- Test.get (Rooms.Index {})
  Task.succeed (Test.text ".free" page.doc == Just (Date.toText (Date.addDays 5 today)))
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
  let bookings <- Db.from Booking |> Db.where (Booking.room.eq params.id) |> Db.orderBy Booking.from.asc |> Db.all
  match found {
    Just room -> Query.succeed { room: room, bookings: bookings },
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
    bookIfFree model.room.id existing session.user p.form
  },
}

-- The decision is made before anything is written: a refused booking stores nothing.
bookIfFree : Room.RoomId -> List Booking -> User.UserId -> NewBooking -> Task Nav
bookIfFree room existing owner want =
  if want.to <= want.from then Task.succeed (Reject "The booking must end after it starts.")
  else if List.any (Booking.overlaps want.from want.to) existing then Task.succeed (Reject "The room is already booked for some of those days.")
  else storeBooking room owner want

storeBooking : Room.RoomId -> User.UserId -> NewBooking -> Task Nav
storeBooking room owner want = {
  let _ <- Db.insert Booking { room: room, owner: owner, from: want.from, to: want.to }
  Task.succeed (Redirect (Rooms.Show { id: room }))
}

view : Model -> Session -> Html Msg
view model session =
  <Layout.Page title={model.room.name}>
    <h1 id="name">{model.room.name}</h1>
    <h2>Bookings</h2>
    <ul id="bookings">
      {for b in model.bookings ->
        <li><span class="from">{Date.toText b.from}</span> <span class="to">{Date.toText b.to}</span></li>}
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
    <button click={App.Logout}>Log out</button>
  </Layout.Page>

test "an overlapping booking is refused and not stored" = {
  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
  let room <- Db.insert Room { name: Text.NonEmpty "Oak" }
  let today <- Date.today
  let _ <- Db.insert Booking { room: room, owner: me, from: today, to: Date.addDays 3 today }
  let _ <- Test.login { user: Just me }
  let _ <- Test.get (Rooms.Show { id: room })
  let r <- Test.submit NewBooking { from: Date.addDays 2 today, to: Date.addDays 4 today }
  let n <- Db.from Booking |> Db.where (Booking.room.eq room) |> Db.count
  Task.succeed ({ answer: r.answer, rows: n } == { answer: FormRejected [{ field: "", message: "The room is already booked for some of those days." }], rows: 1 })
}

test "a booking starting on another's last day is stored" = {
  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
  let room <- Db.insert Room { name: Text.NonEmpty "Oak" }
  let today <- Date.today
  let _ <- Db.insert Booking { room: room, owner: me, from: today, to: Date.addDays 3 today }
  let _ <- Test.login { user: Just me }
  let _ <- Test.get (Rooms.Show { id: room })
  let r <- Test.submit NewBooking { from: Date.addDays 3 today, to: Date.addDays 5 today }
  let n <- Db.from Booking |> Db.where (Booking.room.eq room) |> Db.count
  Task.succeed ({ answer: r.answer, rows: n } == { answer: Redirected (Rooms.Show { id: room }), rows: 2 })
}

--- file: app/Bookings/Index.alder ---
module Bookings.Index

import Room
import Booking
import Layout

page "/bookings" { } requires user

type Item = { booking: Booking, room: Text }

type Model = { items: List Item }

load : Params -> Session -> Query Model
load params session = {
  let bookings <- Db.from Booking |> Db.where (Booking.owner.eq session.user) |> Db.orderBy Booking.from.asc |> Db.all
  let rooms <- Db.from Room |> Db.all
  Query.succeed { items: List.map (toItem rooms) bookings }
}

toItem : List Room -> Booking -> Item
toItem rooms booking = { booking: booking, room: roomName rooms booking.room }

roomName : List Room -> Room.RoomId -> Text
roomName rooms id = {
  let found = List.find (\r -> r.id == id) rooms
  match found {
    Just r -> r.name,
    Nothing -> "(removed room)",
  }
}

view : Model -> Session -> Html Msg
view model session =
  <Layout.Page title="My bookings">
    <h1>My bookings</h1>
    <ul id="mine">
      {for item in model.items ->
        <li>
          <a href={Bookings.Show { id: item.booking.id }}>
            <span class="room">{item.room}</span>
            <span class="from">{Date.toText item.booking.from}</span>
          </a>
        </li>}
    </ul>
    <a href={Rooms.Index {}}>Rooms</a>
    <button click={App.Logout}>Log out</button>
  </Layout.Page>

--- file: app/Bookings/Show.alder ---
module Bookings.Show

import Room
import Booking
import Layout
import User

page "/bookings/{id}" { id: Booking.BookingId } requires user

type Model = { booking: Booking, room: Room, today: Date }

load : Params -> Session -> Query Model
load params session = {
  let found <- Db.get Booking params.id
  match found {
    Just b -> if b.owner == session.user then showBooking b else Query.notFound,
    Nothing -> Query.notFound,
  }
}

showBooking : Booking -> Query Model
showBooking b = {
  let room <- Db.get Room b.room
  let today <- Date.today
  match room {
    Just r -> Query.succeed { booking: b, room: r, today: today },
    Nothing -> Query.notFound,
  }
}

msg Cancel

update : Msg -> Model -> Session -> Task Nav
update msg model session = match msg {
  Cancel p -> {
    let today <- Date.today
    cancelBooking today model.booking
  },
}

-- The server checks the date again: a Cancel sent from a page loaded earlier is refused, and the
-- booking stays (the page is shown again, without its Cancel button).
cancelBooking : Date -> Booking -> Task Nav
cancelBooking today booking =
  if Booking.cancellable today booking then deleteBooking booking
  else Task.succeed Rerender

deleteBooking : Booking -> Task Nav
deleteBooking booking = {
  let _ <- Db.delete Booking booking.id
  Task.succeed (Redirect (Bookings.Index {}))
}

view : Model -> Session -> Html Msg
view model session =
  <Layout.Page title="Booking">
    <h1 id="room">{model.room.name}</h1>
    <p><span id="from">{Date.toText model.booking.from}</span> to <span id="to">{Date.toText model.booking.to}</span></p>
    {if Booking.cancellable model.today model.booking then <button id="cancel" click={Cancel}>Cancel</button> else Html.none}
    <a href={Bookings.Index {}}>Back to my bookings</a>
    <button click={App.Logout}>Log out</button>
  </Layout.Page>

test "the owner cancels a booking before its first day" = {
  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
  let room <- Db.insert Room { name: Text.NonEmpty "Oak" }
  let today <- Date.today
  let id <- Db.insert Booking { room: room, owner: me, from: Date.addDays 3 today, to: Date.addDays 5 today }
  let _ <- Test.login { user: Just me }
  let page <- Test.get (Bookings.Show { id: id })
  let r <- Test.click Cancel
  let after <- Db.get Booking id
  Task.succeed ({ shown: Test.has "#cancel" page.doc, answer: r.answer, kept: Maybe.isJust after } == { shown: True, answer: Redirected (Bookings.Index {}), kept: False })
}

test "a booking whose first day is today cannot be cancelled" = {
  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
  let room <- Db.insert Room { name: Text.NonEmpty "Oak" }
  let today <- Date.today
  let id <- Db.insert Booking { room: room, owner: me, from: today, to: Date.addDays 2 today }
  let _ <- Test.login { user: Just me }
  let page <- Test.get (Bookings.Show { id: id })
  let r <- Test.run (update Cancel page.model { user: me })
  let after <- Db.get Booking id
  Task.succeed ({ shown: Test.has "#cancel" page.doc, value: r.value, kept: Maybe.isJust after } == { shown: False, value: Ok Rerender, kept: True })
}
