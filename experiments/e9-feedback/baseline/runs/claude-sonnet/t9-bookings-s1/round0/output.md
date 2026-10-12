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

-- How many bookings of the room hold a day of [fromDate, toDate). A booking holds its from date
-- up to, but not including, its to date, so two ranges overlap when each starts before the other ends.
conflicts : Room.RoomId -> Date -> Date -> Query Int
conflicts roomId fromDate toDate =
  Db.from Booking
    |> Db.where (Booking.room.eq roomId)
    |> Db.where (Booking.from.lt toDate)
    |> Db.where (Booking.to.gt fromDate)
    |> Db.count

-- The first date, today or later, that none of the bookings holds.
nextFree : Date -> List Booking -> Date
nextFree today bookings =
  List.foldl (\b d -> if b.from <= d && d < b.to then b.to else d) today (List.sortByText (\b -> Date.toText b.from) bookings)

-- A booking can be cancelled only while today is before its from date.
canCancel : Date -> Booking -> Bool
canCancel today booking = today < booking.from

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

--- file: app/Auth/Login.alder ---
module Auth.Login

import User
import Layout

-- Password checking is outside the language: a capability implemented in Go.
capability Password {
  verify : { email: Text.Email, password: Text } -> Task (Maybe User.UserId),
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
import User

page "/rooms" { } requires user

type Row = { room: Room, free: Date }

type Model = { rows: List Row }

rowFor : Date -> List Booking -> Room -> Row
rowFor today bookings room =
  { room: room, free: Booking.nextFree today (List.filter (\b -> b.room == room.id) bookings) }

load : Params -> Session -> Query Model
load params session = {
  let rooms <- Db.from Room |> Db.orderBy Room.name.asc |> Db.all
  let bookings <- Db.from Booking |> Db.all
  let today <- Date.today
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
          {" "}
          <span class="free">{Date.toText row.free}</span>
        </li>}
    </ul>
    <nav>
      <a href={Bookings.Index {}}>My bookings</a>
    </nav>
    <button click={App.Logout}>Log out</button>
  </Layout.Page>

test "the next free date skips back-to-back bookings" = {
  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
  let room <- Db.insert Room { name: Text.NonEmpty "Alpha" }
  let today <- Date.today
  let _ <- Db.insert Booking { room: room, owner: me, from: today, to: Date.addDays 2 today }
  let _ <- Db.insert Booking { room: room, owner: me, from: Date.addDays 2 today, to: Date.addDays 5 today }
  let _ <- Db.insert Booking { room: room, owner: me, from: Date.addDays 7 today, to: Date.addDays 9 today }
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
    Just room -> {
      let bookings <-
        Db.from Booking
          |> Db.where (Booking.room.eq room.id)
          |> Db.orderBy Booking.from.asc
          |> Db.all
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

overlapMessage : Text
overlapMessage = "The room is already booked for part of these dates."

badRangeMessage : Text
badRangeMessage = "The end date must be after the start date."

update : Msg -> Model -> Session -> Task Nav
update msg model session = match msg {
  Book p -> book model session p.form,
}

book : Model -> Session -> NewBooking -> Task Nav
book model session f =
  if f.to <= f.from then Task.succeed (Reject badRangeMessage) else {
    let clashes <- Booking.conflicts model.room.id f.from f.to
    if clashes > 0 then Task.succeed (Reject overlapMessage) else {
      let _ <- Db.insert Booking { room: model.room.id, owner: session.user, from: f.from, to: f.to }
      Task.succeed (Redirect (Rooms.Show { id: model.room.id }))
    }
  }

view : Model -> Session -> Html Msg
view model session =
  <Layout.Page title={model.room.name}>
    <h1 id="name">{model.room.name}</h1>
    <ul id="bookings">
      {for b in model.bookings ->
        <li><span class="from">{Date.toText b.from}</span> to <span class="to">{Date.toText b.to}</span></li>}
    </ul>
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

test "an overlapping booking is refused and not stored, a back-to-back one is stored" = {
  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
  let room <- Db.insert Room { name: Text.NonEmpty "Alpha" }
  let today <- Date.today
  let d1 = Date.addDays 1 today
  let d2 = Date.addDays 2 today
  let d3 = Date.addDays 3 today
  let d5 = Date.addDays 5 today
  let _ <- Db.insert Booking { room: room, owner: me, from: d1, to: d3 }
  let _ <- Test.login { user: Just me }
  let _ <- Test.get (Rooms.Show { id: room })
  let bad <- Test.submit NewBooking { from: d2, to: d5 }
  let afterBad <- Db.from Booking |> Db.count
  let good <- Test.submit NewBooking { from: d3, to: d5 }
  let afterGood <- Db.from Booking |> Db.count
  Task.succeed ({ bad: bad.answer, afterBad: afterBad, good: good.answer, afterGood: afterGood } == { bad: FormRejected [{ field: "", message: overlapMessage }], afterBad: 1, good: Redirected (Rooms.Show { id: room }), afterGood: 2 })
}

test "a booking that ends before it starts is refused" = {
  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
  let room <- Db.insert Room { name: Text.NonEmpty "Alpha" }
  let today <- Date.today
  let _ <- Test.login { user: Just me }
  let _ <- Test.get (Rooms.Show { id: room })
  let r <- Test.submit NewBooking { from: Date.addDays 3 today, to: Date.addDays 1 today }
  let n <- Db.from Booking |> Db.count
  Task.succeed ({ answer: r.answer, stored: n } == { answer: FormRejected [{ field: "", message: badRangeMessage }], stored: 0 })
}

test "bookings of different rooms never conflict" = {
  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
  let a <- Db.insert Room { name: Text.NonEmpty "Alpha" }
  let b <- Db.insert Room { name: Text.NonEmpty "Beta" }
  let today <- Date.today
  let _ <- Db.insert Booking { room: a, owner: me, from: today, to: Date.addDays 4 today }
  let _ <- Test.login { user: Just me }
  let _ <- Test.get (Rooms.Show { id: b })
  let r <- Test.submit NewBooking { from: Date.addDays 1 today, to: Date.addDays 2 today }
  Task.succeed (r.answer == Redirected (Rooms.Show { id: b }))
}

--- file: app/Bookings/Index.alder ---
module Bookings.Index

import Room
import Booking
import Layout
import User

page "/bookings" { } requires user

type Row = { booking: Booking, room: Room }

type Model = { rows: List Row }

rowFor : List Room -> Booking -> Maybe Row
rowFor rooms b = {
  let found = List.find (\r -> r.id == b.room) rooms
  match found {
    Just r -> Just { booking: b, room: r },
    Nothing -> Nothing,
  }
}

load : Params -> Session -> Query Model
load params session = {
  let rooms <- Db.from Room |> Db.all
  let bookings <-
    Db.from Booking
      |> Db.where (Booking.owner.eq session.user)
      |> Db.orderBy Booking.from.asc
      |> Db.all
  Query.succeed { rows: List.filterMap (rowFor rooms) bookings }
}

view : Model -> Session -> Html Msg
view model session =
  <Layout.Page title="My bookings">
    <h1>My bookings</h1>
    <ul id="mine">
      {for row in model.rows ->
        <li>
          <a class="room" href={Bookings.Show { id: row.booking.id }}>{row.room.name}</a>
          {" "}
          <span class="from">{Date.toText row.booking.from}</span>
          {" "}
          <span class="to">{Date.toText row.booking.to}</span>
        </li>}
    </ul>
    <nav>
      <a href={Rooms.Index {}}>Rooms</a>
    </nav>
    <button click={App.Logout}>Log out</button>
  </Layout.Page>

test "only my bookings are listed, ordered by from" = {
  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
  let other <- Db.insert User { email: Text.Email "other@example.com", name: Text.NonEmpty "Other", passwordHash: "" }
  let room <- Db.insert Room { name: Text.NonEmpty "Alpha" }
  let today <- Date.today
  let _ <- Db.insert Booking { room: room, owner: me, from: Date.addDays 5 today, to: Date.addDays 6 today }
  let _ <- Db.insert Booking { room: room, owner: me, from: Date.addDays 1 today, to: Date.addDays 2 today }
  let _ <- Db.insert Booking { room: room, owner: other, from: Date.addDays 3 today, to: Date.addDays 4 today }
  let _ <- Test.login { user: Just me }
  let page <- Test.get (Bookings.Index {})
  Task.succeed (Test.texts "#mine .from" page.doc == [Date.toText (Date.addDays 1 today), Date.toText (Date.addDays 5 today)])
}

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
    Just b -> if b.owner == session.user then withRoom b else Query.notFound,
    Nothing -> Query.notFound,
  }
}

withRoom : Booking -> Query Model
withRoom b = {
  let found <- Db.get Room b.room
  let today <- Date.today
  match found {
    Just r -> Query.succeed { booking: b, room: r, today: today },
    Nothing -> Query.notFound,
  }
}

msg Cancel

refusedMessage : Text
refusedMessage = "A booking cannot be cancelled once it has started."

update : Msg -> Model -> Session -> Task Nav
update msg model session = match msg {
  Cancel p -> {
    let today <- Date.today
    if Booking.canCancel today model.booking then {
      let _ <- Db.delete Booking model.booking.id
      Task.succeed (Redirect (Bookings.Index {}))
    } else Task.succeed (Reject refusedMessage)
  },
}

view : Model -> Session -> Html Msg
view model session =
  <Layout.Page title="Booking">
    <h1>Booking</h1>
    <p>Room: <span id="room">{model.room.name}</span></p>
    <p>From <span id="from">{Date.toText model.booking.from}</span> to <span id="to">{Date.toText model.booking.to}</span></p>
    {if Booking.canCancel model.today model.booking then <button id="cancel" click={Cancel}>Cancel</button> else Html.none}
    <a href={Bookings.Index {}}>Back</a>
  </Layout.Page>

test "the owner cancels a booking before its from date" = {
  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
  let room <- Db.insert Room { name: Text.NonEmpty "Alpha" }
  let today <- Date.today
  let id <- Db.insert Booking { room: room, owner: me, from: Date.addDays 1 today, to: Date.addDays 3 today }
  let _ <- Test.login { user: Just me }
  let page <- Test.get (Bookings.Show { id: id })
  let r <- Test.click Cancel
  let after <- Db.get Booking id
  Task.succeed ({ button: Test.has "#cancel" page.doc, answer: r.answer, stored: Maybe.isJust after } == { button: True, answer: Redirected (Bookings.Index {}), stored: False })
}

test "a booking whose from date is today cannot be cancelled" = {
  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
  let room <- Db.insert Room { name: Text.NonEmpty "Alpha" }
  let today <- Date.today
  let id <- Db.insert Booking { room: room, owner: me, from: today, to: Date.addDays 2 today }
  let _ <- Test.login { user: Just me }
  let page <- Test.get (Bookings.Show { id: id })
  let r <- Test.run (update Cancel page.model { user: me })
  let after <- Db.get Booking id
  Task.succeed ({ button: Test.has "#cancel" page.doc, value: r.value, stored: Maybe.isJust after } == { button: False, value: Ok (Reject refusedMessage), stored: True })
}

test "another user gets a 404 for my booking" = {
  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
  let other <- Db.insert User { email: Text.Email "other@example.com", name: Text.NonEmpty "Other", passwordHash: "" }
  let room <- Db.insert Room { name: Text.NonEmpty "Alpha" }
  let today <- Date.today
  let id <- Db.insert Booking { room: room, owner: me, from: Date.addDays 1 today, to: Date.addDays 3 today }
  let _ <- Test.login { user: Just other }
  let r <- Test.open (Bookings.Show { id: id })
  Task.succeed (r.answer == PageMissing)
}
