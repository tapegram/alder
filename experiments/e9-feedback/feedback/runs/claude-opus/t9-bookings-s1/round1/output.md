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

test "a visitor is sent to log in" = {
  let visitor <- Test.open (Rooms.Index {})
  Task.succeed ({ answer: visitor.answer, heading: Test.text "h1" visitor.doc } == { answer: Redirected (Auth.Login {}), heading: Just "Log in" })
}

-- The two tests below go through the login form with the fake in the adapter's place: the real
-- Password adapter is never called.
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
  Task.succeed ({ answer: r.answer, user: r.session.user, title: Test.text "h1" r.doc } == { answer: Redirected (Rooms.Index {}), user: Just me, title: Just "Rooms" })
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

-- advance moves a day held by the booking to the booking's end; any other day stays.
advance : Booking -> Date -> Date
advance booking day = if booking.from <= day && day < booking.to then booking.to else day

fromText : Booking -> Text
fromText booking = Date.toText booking.from

-- nextFree is the first date, today or later, that none of the bookings (of one room) holds.
nextFree : Date -> List Booking -> Date
nextFree today bookings = bookings |> List.sortByText fromText |> List.foldl advance today

-- A booking can be cancelled while today is before its `from` date.
cancellable : Date -> Booking -> Bool
cancellable today booking = today < booking.from

-- The mutant `day < booking.to → day <= booking.to` in `advance` is the same program: the two
-- differ only when `day == booking.to`, and then both branches give that same date.

test "the next free date of a room with back-to-back bookings" = {
  let today <- Date.today
  let free = nextFree today [
    { id: BookingId "b", room: Room.RoomId "r", owner: User.UserId "u", from: Date.addDays 2 today, to: Date.addDays 5 today },
    { id: BookingId "c", room: Room.RoomId "r", owner: User.UserId "u", from: Date.addDays 7 today, to: Date.addDays 9 today },
    { id: BookingId "a", room: Room.RoomId "r", owner: User.UserId "u", from: today, to: Date.addDays 2 today },
  ]
  Task.succeed (free == Date.addDays 5 today)
}

test "a room booked only later or earlier is free today" = {
  let today <- Date.today
  let free = nextFree today [
    { id: BookingId "a", room: Room.RoomId "r", owner: User.UserId "u", from: Date.addDays 1 today, to: Date.addDays 3 today },
    { id: BookingId "b", room: Room.RoomId "r", owner: User.UserId "u", from: Date.addDays (-3) today, to: today },
  ]
  Task.succeed (free == today)
}

test "a booking is cancellable only before its from date" = {
  let today <- Date.today
  let booking : Booking = { id: BookingId "a", room: Room.RoomId "r", owner: User.UserId "u", from: Date.addDays 1 today, to: Date.addDays 3 today }
  Task.succeed ([cancellable today booking, cancellable (Date.addDays 1 today) booking, cancellable (Date.addDays 2 today) booking] == [True, False, False])
}

-- `room` and `owner` are `@ref` with the default rule, `@restrict`: a room or a user that still
-- has bookings cannot be deleted, and the bookings stay.
test "a room with bookings cannot be deleted" = {
  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
  let room <- Db.insert Room { name: Text.NonEmpty "Oak" }
  let today <- Date.today
  let id <- Db.insert Booking { room: room, owner: me, from: Date.addDays 1 today, to: Date.addDays 3 today }
  let r <- Test.run (Db.delete Room room)
  let after <- Db.get Booking id
  Task.succeed ({ deleted: Result.toMaybe r.value, stored: Maybe.isJust after } == { deleted: Nothing, stored: True })
}

test "a user with bookings cannot be deleted" = {
  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
  let room <- Db.insert Room { name: Text.NonEmpty "Oak" }
  let today <- Date.today
  let id <- Db.insert Booking { room: room, owner: me, from: Date.addDays 1 today, to: Date.addDays 3 today }
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

type Row = { booking: Booking, room: Room }

type Model = { rows: List Row }

load : Params -> Session -> Query Model
load params session = {
  let bookings <- Db.from Booking |> Db.where (Booking.owner.eq session.user) |> Db.orderBy Booking.from.asc |> Db.all
  let rooms <- Db.from Room |> Db.all
  Query.succeed { rows: List.filterMap (row rooms) bookings }
}

isRoom : Room.RoomId -> Room -> Bool
isRoom id room = room.id == id

row : List Room -> Booking -> Maybe Row
row rooms booking = {
  let found = List.find (isRoom booking.room) rooms
  match found {
    Just room -> Just { booking: booking, room: room },
    Nothing -> Nothing,
  }
}

view : Model -> Session -> Html Msg
view model session =
  <Layout.Page title="My bookings">
    <h1>My bookings</h1>
    <nav>
      <a href={Rooms.Index {}}>Rooms</a>
      <a href={Bookings.Index {}}>My bookings</a>
    </nav>
    <ul id="mine">
      {for r in model.rows ->
        <li><a class="room" href={Bookings.Show { id: r.booking.id }}>{r.room.name}</a> from <span class="from">{Date.toText r.booking.from}</span></li>}
    </ul>
    <button click={App.Logout}>Log out</button>
  </Layout.Page>

test "only my bookings are listed, ordered by from" = {
  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
  let other <- Db.insert User { email: Text.Email "other@example.com", name: Text.NonEmpty "Other", passwordHash: "" }
  let oak <- Db.insert Room { name: Text.NonEmpty "Oak" }
  let ash <- Db.insert Room { name: Text.NonEmpty "Ash" }
  let today <- Date.today
  let _ <- Db.insert Booking { room: oak, owner: me, from: Date.addDays 5 today, to: Date.addDays 6 today }
  let _ <- Db.insert Booking { room: ash, owner: me, from: Date.addDays 1 today, to: Date.addDays 2 today }
  let _ <- Db.insert Booking { room: ash, owner: other, from: Date.addDays 3 today, to: Date.addDays 4 today }
  let _ <- Test.login { user: Just me }
  let page <- Test.get (Bookings.Index {})
  Task.succeed ({ rooms: Test.texts "#mine .room" page.doc, froms: Test.texts "#mine .from" page.doc } == { rooms: ["Ash", "Oak"], froms: [Date.toText (Date.addDays 1 today), Date.toText (Date.addDays 5 today)] })
}

test "a visitor is sent to log in from the bookings" = {
  let visitor <- Test.open (Bookings.Index {})
  Task.succeed (visitor.answer == Redirected (Auth.Login {}))
}

--- file: app/Bookings/Show.alder ---
module Bookings.Show

import Room
import Booking
import Layout
import User

page "/bookings/{id}" { id: Booking.BookingId } requires user

type Model = {
  booking: Booking,
  room: Room,
  canCancel: Bool,
}

load : Params -> Session -> Query Model
load params session = {
  let found <- Db.get Booking params.id
  match found {
    Just booking -> if booking.owner == session.user then details booking else Query.notFound,
    Nothing -> Query.notFound,
  }
}

details : Booking -> Query Model
details booking = {
  let found <- Db.get Room booking.room
  let today <- Date.today
  match found {
    Just room -> Query.succeed { booking: booking, room: room, canCancel: Booking.cancellable today booking },
    Nothing -> Query.notFound,
  }
}

-- The message carries no id: the page's parameter names the booking, and `load` has checked that
-- the session's user owns it. The date is checked again here: a page loaded the day before
-- still has its Cancel button.
msg Cancel

update : Msg -> Model -> Session -> Task Nav
update msg model session = match msg {
  Cancel p -> {
    let today <- Date.today
    if Booking.cancellable today model.booking then cancel model.booking.id else Task.succeed Rerender
  },
}

cancel : Booking.BookingId -> Task Nav
cancel id = {
  let _ <- Db.delete Booking id
  Task.succeed (Redirect (Bookings.Index {}))
}

view : Model -> Session -> Html Msg
view model session =
  <Layout.Page title="Booking">
    <h1 id="room">{model.room.name}</h1>
    <nav>
      <a href={Rooms.Index {}}>Rooms</a>
      <a href={Bookings.Index {}}>My bookings</a>
    </nav>
    <p>From <span id="from">{Date.toText model.booking.from}</span> to <span id="to">{Date.toText model.booking.to}</span></p>
    {if model.canCancel then <button id="cancel" click={Cancel}>Cancel</button> else Html.none}
    <button click={App.Logout}>Log out</button>
  </Layout.Page>

test "the owner cancels a booking before its from date" = {
  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
  let room <- Db.insert Room { name: Text.NonEmpty "Oak" }
  let today <- Date.today
  let id <- Db.insert Booking { room: room, owner: me, from: Date.addDays 1 today, to: Date.addDays 3 today }
  let _ <- Test.login { user: Just me }
  let page <- Test.get (Bookings.Show { id: id })
  let r <- Test.click Cancel
  let after <- Db.get Booking id
  Task.succeed ({ room: Test.text "#room" page.doc, from: Test.text "#from" page.doc, to: Test.text "#to" page.doc, button: Test.has "#cancel" page.doc, answer: r.answer, stored: Maybe.isJust after } == { room: Just "Oak", from: Just (Date.toText (Date.addDays 1 today)), to: Just (Date.toText (Date.addDays 3 today)), button: True, answer: Redirected (Bookings.Index {}), stored: False })
}

test "a booking whose from date is today cannot be cancelled" = {
  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
  let room <- Db.insert Room { name: Text.NonEmpty "Oak" }
  let today <- Date.today
  let id <- Db.insert Booking { room: room, owner: me, from: today, to: Date.addDays 2 today }
  let _ <- Test.login { user: Just me }
  let page <- Test.get (Bookings.Show { id: id })
  let model = page.model
  let r <- Test.run (update Cancel { model | canCancel: True } { user: me })
  let after <- Db.get Booking id
  Task.succeed ({ button: Test.has "#cancel" page.doc, value: r.value, stored: Maybe.isJust after } == { button: False, value: Ok Rerender, stored: True })
}

test "a stranger cannot see my booking" = {
  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
  let room <- Db.insert Room { name: Text.NonEmpty "Oak" }
  let today <- Date.today
  let id <- Db.insert Booking { room: room, owner: me, from: Date.addDays 1 today, to: Date.addDays 3 today }
  let _ <- Test.login { user: Just (User.UserId "someone-else") }
  let show <- Test.open (Bookings.Show { id: id })
  let after <- Db.get Booking id
  Task.succeed ({ show: show.answer, stored: Maybe.isJust after } == { show: PageMissing, stored: True })
}

test "a visitor is sent to log in from a booking" = {
  let visitor <- Test.open (Bookings.Show { id: Booking.BookingId "x" })
  Task.succeed (visitor.answer == Redirected (Auth.Login {}))
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

load : Params -> Session -> Query Model
load params session = {
  let rooms <- Db.from Room |> Db.orderBy Room.name.asc |> Db.all
  let bookings <- Db.from Booking |> Db.all
  let today <- Date.today
  Query.succeed { rows: List.map (row today bookings) rooms }
}

inRoom : Room.RoomId -> Booking -> Bool
inRoom id booking = booking.room == id

row : Date -> List Booking -> Room -> Row
row today bookings room = { room: room, free: Booking.nextFree today (List.filter (inRoom room.id) bookings) }

view : Model -> Session -> Html Msg
view model session =
  <Layout.Page title="Rooms">
    <h1>Rooms</h1>
    <nav>
      <a href={Rooms.Index {}}>Rooms</a>
      <a href={Bookings.Index {}}>My bookings</a>
    </nav>
    <ul id="rooms">
      {for r in model.rows ->
        <li><a class="name" href={Rooms.Show { id: r.room.id }}>{r.room.name}</a> free from <span class="free">{Date.toText r.free}</span></li>}
    </ul>
    <button click={App.Logout}>Log out</button>
  </Layout.Page>

test "each room is listed by name with its next free date" = {
  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
  let oak <- Db.insert Room { name: Text.NonEmpty "Oak" }
  let ash <- Db.insert Room { name: Text.NonEmpty "Ash" }
  let today <- Date.today
  let _ <- Db.insert Booking { room: oak, owner: me, from: Date.addDays 2 today, to: Date.addDays 4 today }
  let _ <- Db.insert Booking { room: oak, owner: me, from: today, to: Date.addDays 2 today }
  let _ <- Test.login { user: Just me }
  let page <- Test.get (Rooms.Index {})
  Task.succeed ({ names: Test.texts "#rooms .name" page.doc, free: Test.texts "#rooms .free" page.doc } == { names: ["Ash", "Oak"], free: [Date.toText today, Date.toText (Date.addDays 4 today)] })
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
    Just room -> {
      let bookings <- Db.from Booking |> Db.where (Booking.room.eq room.id) |> Db.orderBy Booking.from.asc |> Db.all
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

-- The message carries no room id: the page's parameter names the room. The decision is made
-- before anything is written, so a refused booking stores nothing.
update : Msg -> Model -> Session -> Task Nav
update msg model session = match msg {
  Book p -> {
    let clashes <-
      Db.from Booking
        |> Db.where (Booking.room.eq model.room.id)
        |> Db.where (Booking.from.lt p.form.to)
        |> Db.where (Booking.to.gt p.form.from)
        |> Db.count
    if not (p.form.from < p.form.to) then Task.succeed (Reject "A booking must end after it starts.") else if clashes > 0 then Task.succeed (Reject "The room is already booked for those dates.") else store model.room.id session.user p.form
  },
}

store : Room.RoomId -> User.UserId -> NewBooking -> Task Nav
store room owner booking = {
  let _ <- Db.insert Booking { room: room, owner: owner, from: booking.from, to: booking.to }
  Task.succeed (Redirect (Rooms.Show { id: room }))
}

view : Model -> Session -> Html Msg
view model session =
  <Layout.Page title={model.room.name}>
    <h1 id="name">{model.room.name}</h1>
    <nav>
      <a href={Rooms.Index {}}>Rooms</a>
      <a href={Bookings.Index {}}>My bookings</a>
    </nav>
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
    <button click={App.Logout}>Log out</button>
  </Layout.Page>

test "an overlapping booking is refused, one starting on the other's to date is stored" = {
  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
  let room <- Db.insert Room { name: Text.NonEmpty "Oak" }
  let today <- Date.today
  let _ <- Db.insert Booking { room: room, owner: me, from: Date.addDays 1 today, to: Date.addDays 3 today }
  let _ <- Test.login { user: Just me }
  let _ <- Test.get (Rooms.Show { id: room })
  let clash <- Test.submit NewBooking { from: Date.addDays 2 today, to: Date.addDays 4 today }
  let rows <- Db.from Booking |> Db.count
  let next <- Test.submit NewBooking { from: Date.addDays 3 today, to: Date.addDays 5 today }
  let page <- Test.get (Rooms.Show { id: room })
  Task.succeed ({ clash: clash.answer, rows: rows, next: next.answer, name: Test.text "#name" page.doc, froms: Test.texts "#bookings .from" page.doc, tos: Test.texts "#bookings .to" page.doc } == { clash: FormRejected [{ field: "", message: "The room is already booked for those dates." }], rows: 1, next: Redirected (Rooms.Show { id: room }), name: Just "Oak", froms: [Date.toText (Date.addDays 1 today), Date.toText (Date.addDays 3 today)], tos: [Date.toText (Date.addDays 3 today), Date.toText (Date.addDays 5 today)] })
}

test "a booking ending on another's from date is stored, one reaching into it is refused" = {
  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
  let room <- Db.insert Room { name: Text.NonEmpty "Oak" }
  let today <- Date.today
  let _ <- Db.insert Booking { room: room, owner: me, from: Date.addDays 3 today, to: Date.addDays 5 today }
  let _ <- Test.login { user: Just me }
  let _ <- Test.get (Rooms.Show { id: room })
  let clash <- Test.submit NewBooking { from: Date.addDays 1 today, to: Date.addDays 4 today }
  let rows <- Db.from Booking |> Db.count
  let before <- Test.submit NewBooking { from: Date.addDays 1 today, to: Date.addDays 3 today }
  let total <- Db.from Booking |> Db.count
  Task.succeed ({ clash: clash.answer, rows: rows, before: before.answer, total: total } == { clash: FormRejected [{ field: "", message: "The room is already booked for those dates." }], rows: 1, before: Redirected (Rooms.Show { id: room }), total: 2 })
}

test "a booking must end after it starts, and other rooms never conflict" = {
  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
  let oak <- Db.insert Room { name: Text.NonEmpty "Oak" }
  let ash <- Db.insert Room { name: Text.NonEmpty "Ash" }
  let today <- Date.today
  let _ <- Db.insert Booking { room: oak, owner: me, from: Date.addDays 1 today, to: Date.addDays 3 today }
  let _ <- Test.login { user: Just me }
  let _ <- Test.get (Rooms.Show { id: ash })
  let empty <- Test.submit NewBooking { from: Date.addDays 2 today, to: Date.addDays 2 today }
  let rows <- Db.from Booking |> Db.count
  let other <- Test.submit NewBooking { from: Date.addDays 1 today, to: Date.addDays 3 today }
  let total <- Db.from Booking |> Db.count
  Task.succeed ({ empty: empty.answer, rows: rows, other: other.answer, total: total } == { empty: FormRejected [{ field: "", message: "A booking must end after it starts." }], rows: 1, other: Redirected (Rooms.Show { id: ash }), total: 2 })
}

test "an unknown room is not found" = {
  let _ <- Test.login { user: Just (User.UserId "me") }
  let r <- Test.open (Rooms.Show { id: Room.RoomId "nope" })
  Task.succeed (r.answer == PageMissing)
}

test "a visitor is sent to log in from a room" = {
  let room <- Db.insert Room { name: Text.NonEmpty "Oak" }
  let visitor <- Test.open (Rooms.Show { id: room })
  Task.succeed (visitor.answer == Redirected (Auth.Login {}))
}

test "a room's page lists only that room's bookings" = {
  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
  let oak <- Db.insert Room { name: Text.NonEmpty "Oak" }
  let ash <- Db.insert Room { name: Text.NonEmpty "Ash" }
  let today <- Date.today
  let _ <- Db.insert Booking { room: oak, owner: me, from: Date.addDays 1 today, to: Date.addDays 3 today }
  let _ <- Db.insert Booking { room: ash, owner: me, from: Date.addDays 5 today, to: Date.addDays 6 today }
  let _ <- Test.login { user: Just me }
  let page <- Test.get (Rooms.Show { id: oak })
  Task.succeed ({ froms: Test.texts "#bookings .from" page.doc, items: Test.count "#bookings > li" page.doc, model: List.length page.model.bookings } == { froms: [Date.toText (Date.addDays 1 today)], items: 1, model: 1 })
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
