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
  <Layout.Page title="Error" nav={False}>
    <h1>Something went wrong</h1>
    <p>Please try again.</p>
  </Layout.Page>

notFound : Html Never
notFound =
  <Layout.Page title="Not found" nav={False}>
    <h1>Not found</h1>
  </Layout.Page>

-- A Rerender of a page that requires a user would also end at the login page, so only a test of
-- `update` itself tells Redirect from Rerender, and a dropped Session.put shows in the trace.
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

-- In a test, `Test.fake Password` replaces the adapter: it accepts any user who has a row,
-- whatever the password.
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
  <Layout.Page title="Log in" nav={False}>
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

test "an unknown email is rejected with a message" = {
  let _ <- Test.fake Password
  let _ <- Test.get (Auth.Login {})
  let r <- Test.submit Credentials { email: Text.Email "nobody@example.com", password: Text.Secret "secret" }
  Task.succeed ({ answer: r.answer, user: r.session.user } == { answer: FormRejected [{ field: "", message: "Email or password is incorrect." }], user: Nothing })
}

test "logging in stores the user and goes to the rooms" = {
  let _ <- Test.fake Password
  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
  let _ <- Test.get (Auth.Login {})
  let r <- Test.submit Credentials { email: Text.Email "me@example.com", password: Text.Secret "secret" }
  Task.succeed ({ answer: r.answer, user: r.session.user, title: Test.text "h1" r.doc } == { answer: Redirected (Rooms.Index {}), user: Just me, title: Just "Rooms" })
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

overlapMessage : Text
overlapMessage = "The room is already booked for some of these dates."

badRangeMessage : Text
badRangeMessage = "The end date must be after the start date."

-- The first date, day or later, that none of the bookings holds.
-- A booking holds its dates from `from` up to, not including, `to`.
nextFree : Date -> List Booking -> Date
nextFree day bookings = {
  let holding = List.find (\b -> b.from <= day && day < b.to) bookings
  match holding {
    Just b -> nextFree b.to bookings,
    Nothing -> day,
  }
}

-- A booking can be cancelled only while today is before its from date.
canCancel : Date -> Booking -> Bool
canCancel today booking = today < booking.from

test "next free date skips back-to-back bookings" = {
  let today <- Date.today
  let me = User.UserId "u"
  let room = Room.RoomId "r"
  Task.succeed (nextFree today [
    { id: BookingId "a", room: room, owner: me, from: Date.addDays (-3) today, to: Date.addDays (-1) today },
    { id: BookingId "b", room: room, owner: me, from: today, to: Date.addDays 2 today },
    { id: BookingId "c", room: room, owner: me, from: Date.addDays 2 today, to: Date.addDays 5 today },
    { id: BookingId "d", room: room, owner: me, from: Date.addDays 7 today, to: Date.addDays 9 today },
  ] == Date.addDays 5 today)
}

test "next free date of a room without bookings is today" = {
  let today <- Date.today
  Task.succeed (nextFree today [] == today)
}

test "a booking can be cancelled only before its from date" = {
  let today <- Date.today
  let me = User.UserId "u"
  let room = Room.RoomId "r"
  let later : Booking = { id: BookingId "a", room: room, owner: me, from: Date.addDays 1 today, to: Date.addDays 2 today }
  let starting : Booking = { id: BookingId "b", room: room, owner: me, from: today, to: Date.addDays 2 today }
  Task.succeed ({ later: canCancel today later, starting: canCancel today starting } == { later: True, starting: False })
}

-- `room` and `owner` use the default delete rule, @restrict: a room or a user that still has
-- bookings cannot be deleted, and the booking stays.
test "a room with bookings cannot be deleted" = {
  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
  let room <- Db.insert Room { name: Text.NonEmpty "Blue" }
  let today <- Date.today
  let id <- Db.insert Booking { room: room, owner: me, from: Date.addDays 1 today, to: Date.addDays 2 today }
  let r <- Test.run (Db.delete Room room)
  let after <- Db.get Booking id
  Task.succeed ({ deleted: Result.toMaybe r.value, stored: Maybe.isJust after } == { deleted: Nothing, stored: True })
}

test "a user with bookings cannot be deleted" = {
  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
  let room <- Db.insert Room { name: Text.NonEmpty "Blue" }
  let today <- Date.today
  let id <- Db.insert Booking { room: room, owner: me, from: Date.addDays 1 today, to: Date.addDays 2 today }
  let r <- Test.run (Db.delete User me)
  let after <- Db.get Booking id
  Task.succeed ({ deleted: Result.toMaybe r.value, stored: Maybe.isJust after } == { deleted: Nothing, stored: True })
}

--- file: app/Bookings/Index.alder ---
module Bookings.Index

import Room
import Booking
import Layout
import User

page "/bookings" { } requires user

type Entry = { booking: Booking, room: Text }

type Model = { entries: List Entry }

entry : List Room -> Booking -> Entry
entry rooms b = { booking: b, room: Room.nameOf rooms b.room }

load : Params -> Session -> Query Model
load params session = {
  let mine <-
    Db.from Booking
      |> Db.where (Booking.owner.eq session.user)
      |> Db.orderBy Booking.from.asc
      |> Db.all
  let rooms <- Db.from Room |> Db.all
  Query.succeed { entries: List.map (entry rooms) mine }
}

view : Model -> Session -> Html Msg
view model session =
  <Layout.Page title="My bookings" nav={True}>
    <h1>My bookings</h1>
    <ul id="mine">
      {for e in model.entries ->
        <li>
          <a href={Bookings.Show { id: e.booking.id }}>
            <span class="room">{e.room}</span>
          </a>
          {" "}
          <span class="from">{Date.toText e.booking.from}</span>
        </li>}
    </ul>
  </Layout.Page>

test "my bookings are listed by date, others are not" = {
  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
  let other <- Db.insert User { email: Text.Email "other@example.com", name: Text.NonEmpty "Other", passwordHash: "" }
  let room <- Db.insert Room { name: Text.NonEmpty "Blue" }
  let today <- Date.today
  let _ <- Db.insert Booking { room: room, owner: me, from: Date.addDays 5 today, to: Date.addDays 6 today }
  let _ <- Db.insert Booking { room: room, owner: me, from: Date.addDays 1 today, to: Date.addDays 2 today }
  let _ <- Db.insert Booking { room: room, owner: other, from: Date.addDays 2 today, to: Date.addDays 3 today }
  let _ <- Test.login { user: Just me }
  let page <- Test.get (Bookings.Index {})
  Task.succeed ({ from: Test.texts "#mine .from" page.doc, rooms: Test.texts "#mine .room" page.doc } == { from: [Date.toText (Date.addDays 1 today), Date.toText (Date.addDays 5 today)], rooms: ["Blue", "Blue"] })
}

test "a visitor is sent to log in from my bookings" = {
  let r <- Test.open (Bookings.Index {})
  Task.succeed (r.answer == Redirected (Auth.Login {}))
}

--- file: app/Bookings/Show.alder ---
module Bookings.Show

import Room
import Booking
import Layout
import User

page "/bookings/{id}" { id: Booking.BookingId } requires user

type Model = { booking: Booking, room: Text, today: Date }

detail : Booking -> Query Model
detail b = {
  let found <- Db.get Room b.room
  let today <- Date.today
  Query.succeed { booking: b, room: Room.nameOf (Maybe.toList found) b.room, today: today }
}

load : Params -> Session -> Query Model
load params session = {
  let found <- Db.get Booking params.id
  match found {
    Just b -> if b.owner == session.user then detail b else Query.notFound,
    Nothing -> Query.notFound,
  }
}

msg Cancel

update : Msg -> Model -> Session -> Task Nav
update msg model session = match msg {
  Cancel p -> {
    let today <- Date.today
    if Booking.canCancel today model.booking then {
      let _ <- Db.delete Booking model.booking.id
      Task.succeed (Redirect (Bookings.Index {}))
    } else Task.succeed Rerender
  },
}

view : Model -> Session -> Html Msg
view model session =
  <Layout.Page title="Booking" nav={True}>
    <h1>Booking</h1>
    <p>Room: <span id="room">{model.room}</span></p>
    <p>From: <span id="from">{Date.toText model.booking.from}</span></p>
    <p>To: <span id="to">{Date.toText model.booking.to}</span></p>
    {if Booking.canCancel model.today model.booking then
      <button id="cancel" click={Cancel}>Cancel booking</button>
    else Html.none}
    <a href={Bookings.Index {}}>Back</a>
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
  Task.succeed ({ button: Test.has "#cancel" page.doc, room: Test.text "#room" page.doc, answer: r.answer, stored: Maybe.isJust after } == { button: True, room: Just "Blue", answer: Redirected (Bookings.Index {}), stored: False })
}

test "a booking whose from date is today cannot be cancelled" = {
  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
  let room <- Db.insert Room { name: Text.NonEmpty "Blue" }
  let today <- Date.today
  let id <- Db.insert Booking { room: room, owner: me, from: today, to: Date.addDays 1 today }
  let _ <- Test.login { user: Just me }
  let page <- Test.get (Bookings.Show { id: id })
  let r <- Test.run (update Cancel { booking: { id: id, room: room, owner: me, from: today, to: Date.addDays 1 today }, room: "Blue", today: Date.addDays (-1) today } { user: me })
  let after <- Db.get Booking id
  Task.succeed ({ button: Test.has "#cancel" page.doc, value: r.value, stored: Maybe.isJust after } == { button: False, value: Ok Rerender, stored: True })
}

test "another user's booking is not found" = {
  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
  let room <- Db.insert Room { name: Text.NonEmpty "Blue" }
  let today <- Date.today
  let id <- Db.insert Booking { room: room, owner: me, from: Date.addDays 1 today, to: Date.addDays 2 today }
  let _ <- Test.login { user: Just (User.UserId "someone-else") }
  let r <- Test.open (Bookings.Show { id: id })
  Task.succeed (r.answer == PageMissing)
}

test "a visitor is sent to log in from a booking" = {
  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
  let room <- Db.insert Room { name: Text.NonEmpty "Blue" }
  let today <- Date.today
  let id <- Db.insert Booking { room: room, owner: me, from: Date.addDays 1 today, to: Date.addDays 2 today }
  let r <- Test.open (Bookings.Show { id: id })
  Task.succeed (r.answer == Redirected (Auth.Login {}))
}

--- file: app/Layout.alder ---
module Layout

Page : { title: Text, nav: Bool, children: List (Html a) } -> Html a
Page props =
  <html>
    <head>
      <meta charset="utf-8" />
      <title>{props.title}</title>
    </head>
    <body>
      {if props.nav then
        <nav>
          <a href={Rooms.Index {}}>Rooms</a>
          {" "}
          <a href={Bookings.Index {}}>My bookings</a>
          {" "}
          <button click={App.Logout}>Log out</button>
        </nav>
      else Html.none}
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

-- The name of the room with this id among the given rooms, or "".
nameOf : List Room -> RoomId -> Text
nameOf rooms id = {
  let found = List.find (\r -> r.id == id) rooms
  match found {
    Just r -> Text.trim r.name,
    Nothing -> "",
  }
}

--- file: app/Rooms/Index.alder ---
module Rooms.Index

import Room
import Booking
import Layout
import User

page "/rooms" { } requires user

type Entry = { room: Room, free: Date }

type Model = { rooms: List Entry }

entry : Date -> List Booking -> Room -> Entry
entry today bookings room = {
  room: room,
  free: Booking.nextFree today (List.filter (\b -> b.room == room.id) bookings),
}

load : Params -> Session -> Query Model
load params session = {
  let today <- Date.today
  let rooms <- Db.from Room |> Db.orderBy Room.name.asc |> Db.all
  let bookings <- Db.from Booking |> Db.all
  Query.succeed { rooms: List.map (entry today bookings) rooms }
}

view : Model -> Session -> Html Msg
view model session =
  <Layout.Page title="Rooms" nav={True}>
    <h1>Rooms</h1>
    <ul id="rooms">
      {for e in model.rooms ->
        <li>
          <a class="name" href={Rooms.Show { id: e.room.id }}>{e.room.name}</a>
          {" "}
          <span class="free">{Date.toText e.free}</span>
        </li>}
    </ul>
  </Layout.Page>

test "a visitor is sent to log in" = {
  let r <- Test.open (Rooms.Index {})
  Task.succeed (r.answer == Redirected (Auth.Login {}))
}

test "each room shows its next free date" = {
  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
  let green <- Db.insert Room { name: Text.NonEmpty "Green" }
  let blue <- Db.insert Room { name: Text.NonEmpty "Blue" }
  let today <- Date.today
  let _ <- Db.insert Booking { room: blue, owner: me, from: today, to: Date.addDays 2 today }
  let _ <- Db.insert Booking { room: blue, owner: me, from: Date.addDays 2 today, to: Date.addDays 5 today }
  let _ <- Test.login { user: Just me }
  let page <- Test.get (Rooms.Index {})
  Task.succeed ({ names: Test.texts "#rooms .name" page.doc, free: Test.texts "#rooms .free" page.doc } == { names: ["Blue", "Green"], free: [Date.toText (Date.addDays 5 today), Date.toText today] })
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

update : Msg -> Model -> Session -> Task Nav
update msg model session = match msg {
  Book p -> {
    let clashes <-
      Db.from Booking
        |> Db.where (Booking.room.eq model.room.id)
        |> Db.where (Booking.from.lt p.form.to)
        |> Db.where (Booking.to.gt p.form.from)
        |> Db.count
    if not (p.form.from < p.form.to) then Task.succeed (Reject Booking.badRangeMessage)
    else if clashes > 0 then Task.succeed (Reject Booking.overlapMessage)
    else {
      let _ <- Db.insert Booking { room: model.room.id, owner: session.user, from: p.form.from, to: p.form.to }
      Task.succeed (Redirect (Rooms.Show { id: model.room.id }))
    }
  },
}

view : Model -> Session -> Html Msg
view model session =
  <Layout.Page title={model.room.name} nav={True}>
    <h1 id="name">{model.room.name}</h1>
    <ul id="bookings">
      {for b in model.bookings ->
        <li>
          <span class="from">{Date.toText b.from}</span>
          {" to "}
          <span class="to">{Date.toText b.to}</span>
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
  </Layout.Page>

test "an overlapping booking is refused, a back-to-back one is stored" = {
  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
  let room <- Db.insert Room { name: Text.NonEmpty "Blue" }
  let today <- Date.today
  let _ <- Db.insert Booking { room: room, owner: me, from: Date.addDays 1 today, to: Date.addDays 3 today }
  let _ <- Test.login { user: Just me }
  let _ <- Test.get (Rooms.Show { id: room })
  let clash <- Test.submit NewBooking { from: Date.addDays 2 today, to: Date.addDays 4 today }
  let afterClash <- Db.from Booking |> Db.count
  let _ <- Test.get (Rooms.Show { id: room })
  let next <- Test.submit NewBooking { from: Date.addDays 3 today, to: Date.addDays 5 today }
  let afterNext <- Db.from Booking |> Db.count
  Task.succeed ({ clash: clash.answer, afterClash: afterClash, next: next.answer, afterNext: afterNext } == { clash: FormRejected [{ field: "", message: Booking.overlapMessage }], afterClash: 1, next: Redirected (Rooms.Show { id: room }), afterNext: 2 })
}

test "a booking next to a later booking is stored" = {
  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
  let room <- Db.insert Room { name: Text.NonEmpty "Blue" }
  let today <- Date.today
  let _ <- Db.insert Booking { room: room, owner: me, from: Date.addDays 5 today, to: Date.addDays 7 today }
  let _ <- Test.login { user: Just me }
  let _ <- Test.get (Rooms.Show { id: room })
  let ends <- Test.submit NewBooking { from: Date.addDays 3 today, to: Date.addDays 5 today }
  let _ <- Test.get (Rooms.Show { id: room })
  let before <- Test.submit NewBooking { from: Date.addDays 1 today, to: Date.addDays 3 today }
  let n <- Db.from Booking |> Db.count
  Task.succeed ({ ends: ends.answer, before: before.answer, n: n } == { ends: Redirected (Rooms.Show { id: room }), before: Redirected (Rooms.Show { id: room }), n: 3 })
}

test "bookings of different rooms do not conflict" = {
  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
  let blue <- Db.insert Room { name: Text.NonEmpty "Blue" }
  let green <- Db.insert Room { name: Text.NonEmpty "Green" }
  let today <- Date.today
  let _ <- Db.insert Booking { room: blue, owner: me, from: Date.addDays 1 today, to: Date.addDays 3 today }
  let _ <- Test.login { user: Just me }
  let _ <- Test.get (Rooms.Show { id: green })
  let r <- Test.submit NewBooking { from: Date.addDays 1 today, to: Date.addDays 3 today }
  let n <- Db.from Booking |> Db.count
  Task.succeed ({ answer: r.answer, n: n } == { answer: Redirected (Rooms.Show { id: green }), n: 2 })
}

test "a booking that ends when it starts is refused" = {
  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
  let room <- Db.insert Room { name: Text.NonEmpty "Blue" }
  let today <- Date.today
  let _ <- Test.login { user: Just me }
  let _ <- Test.get (Rooms.Show { id: room })
  let r <- Test.submit NewBooking { from: Date.addDays 2 today, to: Date.addDays 2 today }
  let n <- Db.from Booking |> Db.count
  Task.succeed ({ answer: r.answer, n: n } == { answer: FormRejected [{ field: "", message: Booking.badRangeMessage }], n: 0 })
}

test "a visitor is sent to log in from a room" = {
  let room <- Db.insert Room { name: Text.NonEmpty "Blue" }
  let r <- Test.open (Rooms.Show { id: room })
  Task.succeed (r.answer == Redirected (Auth.Login {}))
}

test "a room lists only its own bookings, ordered by from date" = {
  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
  let blue <- Db.insert Room { name: Text.NonEmpty "Blue" }
  let green <- Db.insert Room { name: Text.NonEmpty "Green" }
  let today <- Date.today
  let _ <- Db.insert Booking { room: blue, owner: me, from: Date.addDays 5 today, to: Date.addDays 6 today }
  let _ <- Db.insert Booking { room: green, owner: me, from: Date.addDays 3 today, to: Date.addDays 4 today }
  let _ <- Db.insert Booking { room: blue, owner: me, from: Date.addDays 1 today, to: Date.addDays 2 today }
  let _ <- Test.login { user: Just me }
  let page <- Test.get (Rooms.Show { id: blue })
  Task.succeed ({ name: Test.text "#name" page.doc, from: Test.texts "#bookings .from" page.doc, to: Test.texts "#bookings .to" page.doc } == { name: Just "Blue", from: [Date.toText (Date.addDays 1 today), Date.toText (Date.addDays 5 today)], to: [Date.toText (Date.addDays 2 today), Date.toText (Date.addDays 6 today)] })
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
