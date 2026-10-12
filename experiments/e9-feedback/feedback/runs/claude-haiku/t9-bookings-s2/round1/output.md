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

-- Logging out from a page that `requires user` ends at the login page either way, so only a test
-- of `update` itself tells Redirect from Rerender.
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

-- The bookings of one room, in order of their start.
bookingsOf : Room.RoomId -> List Booking -> List Booking
bookingsOf room bookings =
  List.sortByText (\b -> Date.toText b.from) (List.filter (\b -> b.room == room) bookings)

-- Moves the day past a booking that holds it: a booking holds its days from `from` up to `to`.
-- The comparison `day < b.to` cannot be `day <= b.to` in a way any input can tell apart: when
-- `day` is `b.to` the booking does not hold it, and skipping to `b.to` leaves `day` where it is.
skip : Booking -> Date -> Date
skip b day = if b.from <= day && day < b.to then b.to else day

-- The first date, from `today` on, that none of the room's bookings holds. The bookings are
-- taken in order of their start, so one pass past each booking is enough.
nextFree : Date -> Room.RoomId -> List Booking -> Date
nextFree today room bookings = List.foldl skip today (bookingsOf room bookings)

-- Whether a booking from `start` up to `stop` may be stored beside `existing`, the bookings of
-- its room. A booking may start on the day another one ends.
fits : Date -> Date -> List Booking -> Bool
fits start stop existing =
  start < stop && not (List.any (\b -> start < b.to && b.from < stop) existing)

test "a room's next free date skips back-to-back bookings" = {
  let today <- Date.today
  let room = Room.RoomId "oak"
  let first : Booking = { id: BookingId "a", room: room, owner: User.UserId "u", from: today, to: Date.addDays 2 today }
  let second : Booking = { id: BookingId "b", room: room, owner: User.UserId "u", from: Date.addDays 2 today, to: Date.addDays 4 today }
  Task.succeed (nextFree today room [second, first] == Date.addDays 4 today && nextFree today room [] == today)
}

test "a booking that holds today is skipped" = {
  let today <- Date.today
  let room = Room.RoomId "oak"
  let held : Booking = { id: BookingId "a", room: room, owner: User.UserId "u", from: today, to: Date.addDays 2 today }
  Task.succeed (nextFree today room [held] == Date.addDays 2 today)
}

test "a booking that ends today does not hold it" = {
  let today <- Date.today
  let room = Room.RoomId "oak"
  let ended : Booking = { id: BookingId "a", room: room, owner: User.UserId "u", from: Date.addDays (-2) today, to: today }
  Task.succeed (nextFree today room [ended] == today)
}

test "a booking that starts later does not move the free date" = {
  let today <- Date.today
  let room = Room.RoomId "oak"
  let later : Booking = { id: BookingId "a", room: room, owner: User.UserId "u", from: Date.addDays 5 today, to: Date.addDays 7 today }
  Task.succeed (nextFree today room [later] == today)
}

test "another room's bookings do not hold a room's days" = {
  let today <- Date.today
  let room = Room.RoomId "oak"
  let other : Booking = { id: BookingId "a", room: Room.RoomId "pine", owner: User.UserId "u", from: today, to: Date.addDays 2 today }
  Task.succeed (nextFree today room [other] == today)
}

test "a booking may start on the day another ends" = {
  let today <- Date.today
  let room = Room.RoomId "oak"
  let first : Booking = { id: BookingId "a", room: room, owner: User.UserId "u", from: today, to: Date.addDays 2 today }
  Task.succeed (fits (Date.addDays 2 today) (Date.addDays 3 today) [first] && not (fits today (Date.addDays 1 today) [first]))
}

test "a booking may end on the day another starts" = {
  let today <- Date.today
  let room = Room.RoomId "oak"
  let later : Booking = { id: BookingId "a", room: room, owner: User.UserId "u", from: Date.addDays 3 today, to: Date.addDays 5 today }
  Task.succeed (fits today (Date.addDays 3 today) [later])
}

test "a booking must end after it starts" = {
  let today <- Date.today
  Task.succeed (not (fits today today []) && not (fits (Date.addDays 2 today) today []))
}

test "a room with bookings cannot be deleted" = {
  let today <- Date.today
  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
  let room <- Db.insert Room { name: Text.NonEmpty "Oak" }
  let id <- Db.insert Booking { room: room, owner: me, from: today, to: Date.addDays 1 today }
  let r <- Test.run (Db.delete Room room)
  let after <- Db.get Booking id
  Task.succeed ({ deleted: Result.toMaybe r.value, booking: Maybe.map (\b -> b.id) after } == { deleted: Nothing, booking: Just id })
}

test "a user with bookings cannot be deleted" = {
  let today <- Date.today
  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
  let room <- Db.insert Room { name: Text.NonEmpty "Oak" }
  let id <- Db.insert Booking { room: room, owner: me, from: today, to: Date.addDays 1 today }
  let r <- Test.run (Db.delete User me)
  let after <- Db.get Booking id
  Task.succeed ({ deleted: Result.toMaybe r.value, booking: Maybe.map (\b -> b.id) after } == { deleted: Nothing, booking: Just id })
}

--- file: app/Bookings/Index.alder ---
module Bookings.Index

import Booking
import Room
import Layout
import User

page "/bookings" { } requires user

-- One of the user's bookings, with the name of its room.
type Mine = { booking: Booking, room: Text }

type Model = { items: List Mine }

roomName : List Room -> Room.RoomId -> Text
roomName rooms id = {
  let found = List.find (\r -> r.id == id) rooms
  match found {
    Just r -> r.name,
    Nothing -> "",
  }
}

entry : List Room -> Booking -> Mine
entry rooms b = { booking: b, room: roomName rooms b.room }

load : Params -> Session -> Query Model
load params session = {
  let mine <- Db.from Booking |> Db.where (Booking.owner.eq session.user) |> Db.orderBy Booking.from.asc |> Db.all
  let rooms <- Db.from Room |> Db.all
  Query.succeed { items: List.map (entry rooms) mine }
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
  </Layout.Page>

test "a visitor is sent to log in from my bookings" = {
  let r <- Test.open (Bookings.Index {})
  Task.succeed (r.answer == Redirected (Auth.Login {}))
}

test "the page lists only my bookings, in order of their start" = {
  let today <- Date.today
  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
  let other <- Db.insert User { email: Text.Email "other@example.com", name: Text.NonEmpty "Other", passwordHash: "" }
  let room <- Db.insert Room { name: Text.NonEmpty "Oak" }
  let _ <- Db.insert Booking { room: room, owner: me, from: Date.addDays 5 today, to: Date.addDays 6 today }
  let _ <- Db.insert Booking { room: room, owner: me, from: Date.addDays 2 today, to: Date.addDays 3 today }
  let _ <- Db.insert Booking { room: room, owner: other, from: Date.addDays 1 today, to: Date.addDays 2 today }
  let _ <- Test.login { user: Just me }
  let page <- Test.get (Bookings.Index {})
  Task.succeed ({ rooms: Test.texts ".room" page.doc, froms: Test.texts ".from" page.doc } == { rooms: ["Oak", "Oak"], froms: [Date.toText (Date.addDays 2 today), Date.toText (Date.addDays 5 today)] })
}

test "each booking shows the name of its own room" = {
  let today <- Date.today
  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
  let aspen <- Db.insert Room { name: Text.NonEmpty "Aspen" }
  let birch <- Db.insert Room { name: Text.NonEmpty "Birch" }
  let _ <- Db.insert Booking { room: birch, owner: me, from: Date.addDays 1 today, to: Date.addDays 2 today }
  let _ <- Test.login { user: Just me }
  let page <- Test.get (Bookings.Index {})
  Task.succeed (Test.texts ".room" page.doc == ["Birch"])
}

--- file: app/Bookings/Show.alder ---
module Bookings.Show

import Booking
import Room
import User
import Layout

page "/bookings/{id}" { id: Booking.BookingId } requires user

-- `cancellable` says whether the booking has not started yet; `update` checks the date again.
type Model = { booking: Booking, room: Text, cancellable: Bool }

load : Params -> Session -> Query Model
load params session = {
  let found <- Db.get Booking params.id
  match found {
    Just b -> if b.owner == session.user then showOf b else Query.notFound,
    Nothing -> Query.notFound,
  }
}

showOf : Booking -> Query Model
showOf b = {
  let room <- Db.get Room b.room
  let today <- Date.today
  match room {
    Just r -> Query.succeed { booking: b, room: r.name, cancellable: today < b.from },
    Nothing -> Query.notFound,
  }
}

msg Cancel

-- A cancel after the first day of the booking is refused: the booking stays and the page is shown
-- again.
update : Msg -> Model -> Session -> Task Nav
update msg model session = match msg {
  Cancel p -> {
    let today <- Date.today
    if today < model.booking.from then {
      let _ <- Db.delete Booking model.booking.id
      Task.succeed (Redirect (Bookings.Index {}))
    } else Task.succeed Rerender
  },
}

view : Model -> Session -> Html Msg
view model session =
  <Layout.Page title="Booking">
    <h1 id="room">{model.room}</h1>
    <p>From <span id="from">{Date.toText model.booking.from}</span> to <span id="to">{Date.toText model.booking.to}</span></p>
    {if model.cancellable then <button id="cancel" click={Cancel}>Cancel booking</button> else Html.none}
    <a href={Bookings.Index {}}>Back</a>
  </Layout.Page>

test "the owner cancels a booking before it starts" = {
  let today <- Date.today
  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
  let room <- Db.insert Room { name: Text.NonEmpty "Oak" }
  let id <- Db.insert Booking { room: room, owner: me, from: Date.addDays 5 today, to: Date.addDays 6 today }
  let _ <- Test.login { user: Just me }
  let _ <- Test.get (Bookings.Show { id: id })
  let r <- Test.click Cancel
  let after <- Db.get Booking id
  Task.succeed ({ answer: r.answer, booking: Maybe.map (\b -> b.id) after } == { answer: Redirected (Bookings.Index {}), booking: Nothing })
}

test "a booking that starts today cannot be cancelled" = {
  let today <- Date.today
  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
  let room <- Db.insert Room { name: Text.NonEmpty "Oak" }
  let id <- Db.insert Booking { room: room, owner: me, from: today, to: Date.addDays 2 today }
  let _ <- Test.login { user: Just me }
  let page <- Test.get (Bookings.Show { id: id })
  let r <- Test.run (update Cancel page.model { user: me })
  let after <- Db.get Booking id
  Task.succeed ({ shown: Test.has "#cancel" page.doc, value: r.value, kept: Maybe.map (\b -> b.id) after } == { shown: False, value: Ok Rerender, kept: Just id })
}

test "a visitor is sent to log in from a booking" = {
  let today <- Date.today
  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
  let room <- Db.insert Room { name: Text.NonEmpty "Oak" }
  let id <- Db.insert Booking { room: room, owner: me, from: Date.addDays 5 today, to: Date.addDays 6 today }
  let r <- Test.open (Bookings.Show { id: id })
  Task.succeed (r.answer == Redirected (Auth.Login {}))
}

test "a stranger cannot see someone else's booking" = {
  let today <- Date.today
  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
  let room <- Db.insert Room { name: Text.NonEmpty "Oak" }
  let id <- Db.insert Booking { room: room, owner: me, from: Date.addDays 5 today, to: Date.addDays 6 today }
  let _ <- Test.login { user: Just (User.UserId "someone-else") }
  let r <- Test.open (Bookings.Show { id: id })
  Task.succeed (r.answer == PageMissing)
}

test "the owner sees the room and the dates" = {
  let today <- Date.today
  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
  let room <- Db.insert Room { name: Text.NonEmpty "Oak" }
  let id <- Db.insert Booking { room: room, owner: me, from: Date.addDays 5 today, to: Date.addDays 6 today }
  let _ <- Test.login { user: Just me }
  let page <- Test.get (Bookings.Show { id: id })
  Task.succeed ({ room: Test.text "#room" page.doc, from: Test.text "#from" page.doc, to: Test.text "#to" page.doc } == { room: Just "Oak", from: Just (Date.toText (Date.addDays 5 today)), to: Just (Date.toText (Date.addDays 6 today)) })
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
      <nav>
        <a href={Rooms.Index {}}>Rooms</a>
        <a href={Bookings.Index {}}>My bookings</a>
        <button click={App.Logout}>Log out</button>
      </nav>
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

--- file: app/Rooms/Index.alder ---
module Rooms.Index

import Room
import Booking
import Layout
import User

page "/rooms" { } requires user

-- A room with the first date, from today on, that none of its bookings holds.
type Listing = { room: Room, free: Date }

type Model = { rows: List Listing }

listing : Date -> List Booking -> Room -> Listing
listing today bookings room = { room: room, free: Booking.nextFree today room.id bookings }

load : Params -> Session -> Query Model
load params session = {
  let today <- Date.today
  let rooms <- Db.from Room |> Db.orderBy Room.name.asc |> Db.all
  let bookings <- Db.from Booking |> Db.all
  Query.succeed { rows: List.map (listing today bookings) rooms }
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

test "the next free date follows back-to-back bookings" = {
  let today <- Date.today
  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
  let room <- Db.insert Room { name: Text.NonEmpty "Oak" }
  let _ <- Db.insert Booking { room: room, owner: me, from: today, to: Date.addDays 2 today }
  let _ <- Db.insert Booking { room: room, owner: me, from: Date.addDays 2 today, to: Date.addDays 4 today }
  let _ <- Test.login { user: Just me }
  let page <- Test.get (Rooms.Index {})
  Task.succeed (Test.texts ".free" page.doc == [Date.toText (Date.addDays 4 today)])
}

test "rooms are listed by name, each with its own next free date" = {
  let today <- Date.today
  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
  let birch <- Db.insert Room { name: Text.NonEmpty "Birch" }
  let aspen <- Db.insert Room { name: Text.NonEmpty "Aspen" }
  let _ <- Db.insert Booking { room: birch, owner: me, from: today, to: Date.addDays 3 today }
  let _ <- Db.insert Booking { room: aspen, owner: me, from: Date.addDays 5 today, to: Date.addDays 6 today }
  let _ <- Test.login { user: Just me }
  let page <- Test.get (Rooms.Index {})
  Task.succeed ({ names: Test.texts ".name" page.doc, free: Test.texts ".free" page.doc } == { names: ["Aspen", "Birch"], free: [Date.toText today, Date.toText (Date.addDays 3 today)] })
}

test "a visitor is sent to log in from the rooms" = {
  let r <- Test.open (Rooms.Index {})
  Task.succeed (r.answer == Redirected (Auth.Login {}))
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
    book model.room.id session.user p.form.from p.form.to existing
  },
}

-- Stores the booking when its last day is after its first and no booking of the room holds one
-- of its days; otherwise the form is shown again with the reason.
book : Room.RoomId -> User.UserId -> Date -> Date -> List Booking -> Task Nav
book room owner start stop existing =
  if stop <= start then Task.succeed (Reject "The booking must end after it starts.") else if not (Booking.fits start stop existing) then Task.succeed (Reject "This room is already booked on some of those days.") else {
    let _ <- Db.insert Booking { room: room, owner: owner, from: start, to: stop }
    Task.succeed (Redirect (Rooms.Show { id: room }))
  }

view : Model -> Session -> Html Msg
view model session =
  <Layout.Page title={model.room.name}>
    <h1 id="name">{model.room.name}</h1>
    <ul id="bookings">
      {for b in model.bookings ->
        <li>
          <span class="from">{Date.toText b.from}</span>
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
    <a href={Rooms.Index {}}>All rooms</a>
  </Layout.Page>

test "an overlapping booking is refused and not stored" = {
  let today <- Date.today
  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
  let room <- Db.insert Room { name: Text.NonEmpty "Oak" }
  let _ <- Db.insert Booking { room: room, owner: me, from: today, to: Date.addDays 3 today }
  let _ <- Test.login { user: Just me }
  let _ <- Test.get (Rooms.Show { id: room })
  let r <- Test.submit NewBooking { from: Date.addDays 2 today, to: Date.addDays 5 today }
  let count <- Db.from Booking |> Db.where (Booking.room.eq room) |> Db.count
  Task.succeed ({ answer: r.answer, count: count } == { answer: FormRejected [{ field: "", message: "This room is already booked on some of those days." }], count: 1 })
}

test "a booking may start on the day another ends" = {
  let today <- Date.today
  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
  let room <- Db.insert Room { name: Text.NonEmpty "Oak" }
  let _ <- Db.insert Booking { room: room, owner: me, from: today, to: Date.addDays 3 today }
  let _ <- Test.login { user: Just me }
  let _ <- Test.get (Rooms.Show { id: room })
  let r <- Test.submit NewBooking { from: Date.addDays 3 today, to: Date.addDays 5 today }
  let count <- Db.from Booking |> Db.where (Booking.room.eq room) |> Db.count
  Task.succeed ({ answer: r.answer, count: count } == { answer: Redirected (Rooms.Show { id: room }), count: 2 })
}

test "a booking must end after it starts" = {
  let today <- Date.today
  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
  let room <- Db.insert Room { name: Text.NonEmpty "Oak" }
  let _ <- Test.login { user: Just me }
  let _ <- Test.get (Rooms.Show { id: room })
  let r <- Test.submit NewBooking { from: Date.addDays 2 today, to: Date.addDays 2 today }
  let count <- Db.from Booking |> Db.where (Booking.room.eq room) |> Db.count
  Task.succeed ({ answer: r.answer, count: count } == { answer: FormRejected [{ field: "", message: "The booking must end after it starts." }], count: 0 })
}

test "bookings of another room do not conflict" = {
  let today <- Date.today
  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
  let oak <- Db.insert Room { name: Text.NonEmpty "Oak" }
  let pine <- Db.insert Room { name: Text.NonEmpty "Pine" }
  let _ <- Db.insert Booking { room: pine, owner: me, from: today, to: Date.addDays 3 today }
  let _ <- Test.login { user: Just me }
  let _ <- Test.get (Rooms.Show { id: oak })
  let r <- Test.submit NewBooking { from: today, to: Date.addDays 3 today }
  let count <- Db.from Booking |> Db.where (Booking.room.eq oak) |> Db.count
  Task.succeed ({ answer: r.answer, count: count } == { answer: Redirected (Rooms.Show { id: oak }), count: 1 })
}

test "a room lists only its own bookings, in order of their start" = {
  let today <- Date.today
  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
  let oak <- Db.insert Room { name: Text.NonEmpty "Oak" }
  let pine <- Db.insert Room { name: Text.NonEmpty "Pine" }
  let _ <- Db.insert Booking { room: oak, owner: me, from: Date.addDays 5 today, to: Date.addDays 6 today }
  let _ <- Db.insert Booking { room: oak, owner: me, from: Date.addDays 2 today, to: Date.addDays 3 today }
  let _ <- Db.insert Booking { room: pine, owner: me, from: Date.addDays 1 today, to: Date.addDays 2 today }
  let _ <- Test.login { user: Just me }
  let page <- Test.get (Rooms.Show { id: oak })
  Task.succeed ({ count: Test.count "#bookings li" page.doc, froms: Test.texts ".from" page.doc } == { count: 2, froms: [Date.toText (Date.addDays 2 today), Date.toText (Date.addDays 5 today)] })
}

test "a visitor is sent to log in from a room" = {
  let room <- Db.insert Room { name: Text.NonEmpty "Oak" }
  let r <- Test.open (Rooms.Show { id: room })
  Task.succeed (r.answer == Redirected (Auth.Login {}))
}

test "a room that does not exist is not found" = {
  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
  let _ <- Test.login { user: Just me }
  let r <- Test.open (Rooms.Show { id: Room.RoomId "nope" })
  Task.succeed (r.answer == PageMissing)
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
