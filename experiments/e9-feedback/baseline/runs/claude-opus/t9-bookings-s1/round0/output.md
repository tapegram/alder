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

-- A booking holds its room from its `from` date up to, but not including, its `to` date.
holds : Date -> Booking -> Bool
holds day booking = booking.from <= day && day < booking.to

-- nextFree is the first date, `today` or later, that none of the bookings (of one room) holds.
nextFree : Date -> List Booking -> Date
nextFree today bookings =
  bookings
    |> List.sortByText (\b -> Date.toText b.from)
    |> List.foldl (\b day -> if holds day b then b.to else day) today

-- overlapping counts the bookings of a room that hold some date of start..stop (stop excluded).
overlapping : Room.RoomId -> Date -> Date -> Query Int
overlapping room start stop =
  Db.from Booking
    |> Db.where (Booking.room.eq room)
    |> Db.where (Booking.from.lt stop)
    |> Db.where (Booking.to.gt start)
    |> Db.count

-- A booking can be cancelled while today is before its `from` date.
cancellable : Date -> Booking -> Bool
cancellable today booking = today < booking.from

test "the next free date skips back-to-back bookings" = {
  let today <- Date.today
  Task.succeed (nextFree today [
    { id: BookingId "b", room: Room.RoomId "r", owner: User.UserId "u", from: Date.addDays 2 today, to: Date.addDays 4 today },
    { id: BookingId "a", room: Room.RoomId "r", owner: User.UserId "u", from: today, to: Date.addDays 2 today },
    { id: BookingId "c", room: Room.RoomId "r", owner: User.UserId "u", from: Date.addDays 5 today, to: Date.addDays 6 today },
  ] == Date.addDays 4 today)
}

test "a room without bookings is free today" = {
  let today <- Date.today
  Task.succeed (nextFree today [] == today)
}

test "a later booking leaves today free" = {
  let today <- Date.today
  Task.succeed (nextFree today [
    { id: BookingId "a", room: Room.RoomId "r", owner: User.UserId "u", from: Date.addDays 1 today, to: Date.addDays 3 today },
  ] == today)
}

test "a booking holds its from date but not its to date" = {
  let today <- Date.today
  let b : Booking = { id: BookingId "a", room: Room.RoomId "r", owner: User.UserId "u", from: today, to: Date.addDays 2 today }
  Task.succeed ([holds today b, holds (Date.addDays 1 today) b, holds (Date.addDays 2 today) b] == [True, True, False])
}

test "a booking can be cancelled only before its from date" = {
  let today <- Date.today
  let b : Booking = { id: BookingId "a", room: Room.RoomId "r", owner: User.UserId "u", from: Date.addDays 1 today, to: Date.addDays 2 today }
  Task.succeed ([cancellable today b, cancellable (Date.addDays 1 today) b, cancellable (Date.addDays 2 today) b] == [True, False, False])
}

test "overlapping counts only the same room's overlapping bookings" = {
  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
  let oak <- Db.insert Room { name: Text.NonEmpty "Oak" }
  let ash <- Db.insert Room { name: Text.NonEmpty "Ash" }
  let today <- Date.today
  let _ <- Db.insert Booking { room: oak, owner: me, from: Date.addDays 1 today, to: Date.addDays 3 today }
  let inside <- overlapping oak (Date.addDays 2 today) (Date.addDays 4 today)
  let after <- overlapping oak (Date.addDays 3 today) (Date.addDays 5 today)
  let before <- overlapping oak today (Date.addDays 1 today)
  let other <- overlapping ash (Date.addDays 1 today) (Date.addDays 3 today)
  Task.succeed ([inside, after, before, other] == [1, 0, 0, 0])
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

test "the login page is shown to a visitor" = {
  let r <- Test.open (Auth.Login {})
  Task.succeed ({ answer: r.answer, heading: Test.text "h1" r.doc, user: r.session.user } == { answer: Shown, heading: Just "Log in", user: Nothing })
}

--- file: app/Rooms/Index.alder ---
module Rooms.Index

import Room
import Booking
import User
import Layout

page "/rooms" { } requires user

type Row = {
  room: Room,
  free: Date,
}

type Model = {
  rows: List Row,
}

load : Params -> Session -> Query Model
load params session = {
  let today <- Date.today
  let rooms <- Db.from Room |> Db.orderBy Room.name.asc |> Db.all
  let bookings <- Db.from Booking |> Db.orderBy Booking.from.asc |> Db.all
  Query.succeed { rows: List.map (row today bookings) rooms }
}

-- row is a room with its next free date: only the room's own bookings count.
row : Date -> List Booking -> Room -> Row
row today bookings room =
  { room: room, free: Booking.nextFree today (List.filter (\b -> b.room == room.id) bookings) }

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
        <li>
          <a class="name" href={Rooms.Show { id: r.room.id }}>{r.room.name}</a>
          <span class="free">{Date.toText r.free}</span>
        </li>}
    </ul>
    <button click={App.Logout}>Log out</button>
  </Layout.Page>

test "a visitor is sent to log in from the rooms" = {
  let r <- Test.open (Rooms.Index {})
  Task.succeed (r.answer == Redirected (Auth.Login {}))
}

test "rooms are listed by name with their next free date" = {
  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
  let oak <- Db.insert Room { name: Text.NonEmpty "Oak" }
  let ash <- Db.insert Room { name: Text.NonEmpty "Ash" }
  let elm <- Db.insert Room { name: Text.NonEmpty "Elm" }
  let today <- Date.today
  let _ <- Db.insert Booking { room: oak, owner: me, from: Date.addDays 2 today, to: Date.addDays 3 today }
  let _ <- Db.insert Booking { room: oak, owner: me, from: today, to: Date.addDays 2 today }
  let _ <- Db.insert Booking { room: ash, owner: me, from: Date.addDays 1 today, to: Date.addDays 2 today }
  let _ <- Test.login { user: Just me }
  let page <- Test.get (Rooms.Index {})
  Task.succeed ({ names: Test.texts "#rooms .name" page.doc, free: Test.texts "#rooms .free" page.doc, items: Test.count "#rooms > li" page.doc } == { names: ["Ash", "Elm", "Oak"], free: [Date.toText today, Date.toText today, Date.toText (Date.addDays 3 today)], items: 3 })
}

--- file: app/Rooms/Show.alder ---
module Rooms.Show

import Room
import Booking
import User
import Layout

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
  Book p -> book model.room.id session.user p.form.from p.form.to,
}

-- book decides before it writes: a refused booking stores nothing.
book : Room.RoomId -> User.UserId -> Date -> Date -> Task Nav
book room user start stop =
  if stop <= start then Task.succeed (Reject "The to date must be after the from date.") else {
    let clashes <- Booking.overlapping room start stop
    if clashes > 0 then Task.succeed (Reject "The room is already booked on those dates.") else {
      let _ <- Db.insert Booking { room: room, owner: user, from: start, to: stop }
      Task.succeed (Redirect (Rooms.Show { id: room }))
    }
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
    <button click={App.Logout}>Log out</button>
  </Layout.Page>

test "an overlapping booking is refused, one starting on the other's to date is stored" = {
  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
  let oak <- Db.insert Room { name: Text.NonEmpty "Oak" }
  let today <- Date.today
  let _ <- Db.insert Booking { room: oak, owner: me, from: Date.addDays 1 today, to: Date.addDays 3 today }
  let _ <- Test.login { user: Just me }
  let _ <- Test.get (Rooms.Show { id: oak })
  let clash <- Test.submit NewBooking { from: Date.addDays 2 today, to: Date.addDays 4 today }
  let afterClash <- Db.from Booking |> Db.count
  let adjacent <- Test.submit NewBooking { from: Date.addDays 3 today, to: Date.addDays 5 today }
  let afterAdjacent <- Db.from Booking |> Db.count
  Task.succeed ({ clash: clash.answer, afterClash: afterClash, adjacent: adjacent.answer, afterAdjacent: afterAdjacent, froms: Test.texts "#bookings .from" adjacent.doc, tos: Test.texts "#bookings .to" adjacent.doc } == { clash: FormRejected [{ field: "", message: "The room is already booked on those dates." }], afterClash: 1, adjacent: Redirected (Rooms.Show { id: oak }), afterAdjacent: 2, froms: [Date.toText (Date.addDays 1 today), Date.toText (Date.addDays 3 today)], tos: [Date.toText (Date.addDays 3 today), Date.toText (Date.addDays 5 today)] })
}

test "a booking that ends on another's from date is stored" = {
  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
  let oak <- Db.insert Room { name: Text.NonEmpty "Oak" }
  let today <- Date.today
  let _ <- Db.insert Booking { room: oak, owner: me, from: Date.addDays 2 today, to: Date.addDays 4 today }
  let _ <- Test.login { user: Just me }
  let _ <- Test.get (Rooms.Show { id: oak })
  let r <- Test.submit NewBooking { from: Date.addDays 1 today, to: Date.addDays 2 today }
  let rows <- Db.from Booking |> Db.count
  Task.succeed ({ answer: r.answer, rows: rows } == { answer: Redirected (Rooms.Show { id: oak }), rows: 2 })
}

test "a booking whose to is not after its from is refused" = {
  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
  let oak <- Db.insert Room { name: Text.NonEmpty "Oak" }
  let today <- Date.today
  let _ <- Test.login { user: Just me }
  let _ <- Test.get (Rooms.Show { id: oak })
  let same <- Test.submit NewBooking { from: Date.addDays 1 today, to: Date.addDays 1 today }
  let backwards <- Test.submit NewBooking { from: Date.addDays 3 today, to: Date.addDays 1 today }
  let rows <- Db.from Booking |> Db.count
  Task.succeed ({ same: same.answer, backwards: backwards.answer, rows: rows } == { same: FormRejected [{ field: "", message: "The to date must be after the from date." }], backwards: FormRejected [{ field: "", message: "The to date must be after the from date." }], rows: 0 })
}

test "bookings of different rooms never conflict" = {
  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
  let oak <- Db.insert Room { name: Text.NonEmpty "Oak" }
  let ash <- Db.insert Room { name: Text.NonEmpty "Ash" }
  let today <- Date.today
  let _ <- Db.insert Booking { room: oak, owner: me, from: Date.addDays 1 today, to: Date.addDays 3 today }
  let _ <- Test.login { user: Just me }
  let _ <- Test.get (Rooms.Show { id: ash })
  let r <- Test.submit NewBooking { from: Date.addDays 1 today, to: Date.addDays 3 today }
  let rows <- Db.from Booking |> Db.where (Booking.room.eq ash) |> Db.count
  Task.succeed ({ answer: r.answer, rows: rows, name: Test.text "#name" r.doc, shown: Test.count "#bookings > li" r.doc } == { answer: Redirected (Rooms.Show { id: ash }), rows: 1, name: Just "Ash", shown: 1 })
}

test "an unknown room is not found" = {
  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
  let _ <- Test.login { user: Just me }
  let r <- Test.open (Rooms.Show { id: Room.RoomId "nope" })
  Task.succeed (r.answer == PageMissing)
}

--- file: app/Bookings/Index.alder ---
module Bookings.Index

import Room
import Booking
import User
import Layout

page "/bookings" { } requires user

type Entry = {
  booking: Booking,
  room: Room,
}

type Model = {
  entries: List Entry,
}

load : Params -> Session -> Query Model
load params session = {
  let bookings <-
    Db.from Booking
      |> Db.where (Booking.owner.eq session.user)
      |> Db.orderBy Booking.from.asc
      |> Db.all
  let rooms <- Db.from Room |> Db.all
  Query.succeed { entries: List.filterMap (entry rooms) bookings }
}

entry : List Room -> Booking -> Maybe Entry
entry rooms booking = {
  let found = List.find (\r -> r.id == booking.room) rooms
  match found {
    Just r -> Just { booking: booking, room: r },
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
      {for e in model.entries ->
        <li>
          <a class="room" href={Bookings.Show { id: e.booking.id }}>{e.room.name}</a>
          <span class="from">{Date.toText e.booking.from}</span>
        </li>}
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
  Task.succeed ({ rooms: Test.texts "#mine .room" page.doc, froms: Test.texts "#mine .from" page.doc, items: Test.count "#mine > li" page.doc } == { rooms: ["Ash", "Oak"], froms: [Date.toText (Date.addDays 1 today), Date.toText (Date.addDays 5 today)], items: 2 })
}

test "a visitor is sent to log in from the bookings" = {
  let r <- Test.open (Bookings.Index {})
  Task.succeed (r.answer == Redirected (Auth.Login {}))
}

--- file: app/Bookings/Show.alder ---
module Bookings.Show

import Room
import Booking
import User
import Layout

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
    Just r -> Query.succeed { booking: booking, room: r, canCancel: Booking.cancellable today booking },
    Nothing -> Query.notFound,
  }
}

-- The message carries no id: the page's parameter names the booking, and `load` has checked
-- that the session's user owns it. The date is checked again here: the page may be a day old.
msg Cancel

update : Msg -> Model -> Session -> Task Nav
update msg model session = match msg {
  Cancel p -> {
    let today <- Date.today
    if Booking.cancellable today model.booking then {
      let _ <- Db.delete Booking model.booking.id
      Task.succeed (Redirect (Bookings.Index {}))
    } else Task.succeed (Reject "A booking cannot be cancelled once it has started.")
  },
}

view : Model -> Session -> Html Msg
view model session =
  <Layout.Page title="Booking">
    <h1 id="room">{model.room.name}</h1>
    <nav>
      <a href={Rooms.Index {}}>Rooms</a>
      <a href={Bookings.Index {}}>My bookings</a>
    </nav>
    <p>
      <span id="from">{Date.toText model.booking.from}</span>
      <span id="to">{Date.toText model.booking.to}</span>
    </p>
    {if model.canCancel then <button id="cancel" click={Cancel}>Cancel</button> else Html.none}
    <button click={App.Logout}>Log out</button>
  </Layout.Page>

test "the owner cancels a booking before its from date" = {
  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
  let oak <- Db.insert Room { name: Text.NonEmpty "Oak" }
  let today <- Date.today
  let id <- Db.insert Booking { room: oak, owner: me, from: Date.addDays 1 today, to: Date.addDays 3 today }
  let _ <- Test.login { user: Just me }
  let page <- Test.get (Bookings.Show { id: id })
  let r <- Test.click Cancel
  let after <- Db.get Booking id
  Task.succeed ({ room: Test.text "#room" page.doc, from: Test.text "#from" page.doc, to: Test.text "#to" page.doc, button: Test.has "#cancel" page.doc, answer: r.answer, gone: after == Nothing } == { room: Just "Oak", from: Just (Date.toText (Date.addDays 1 today)), to: Just (Date.toText (Date.addDays 3 today)), button: True, answer: Redirected (Bookings.Index {}), gone: True })
}

test "a booking whose from date is today cannot be cancelled" = {
  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
  let oak <- Db.insert Room { name: Text.NonEmpty "Oak" }
  let today <- Date.today
  let id <- Db.insert Booking { room: oak, owner: me, from: today, to: Date.addDays 2 today }
  let _ <- Test.login { user: Just me }
  let page <- Test.get (Bookings.Show { id: id })
  let r <- Test.run (update Cancel page.model { user: me })
  let after <- Db.get Booking id
  Task.succeed ({ button: Test.has "#cancel" page.doc, value: r.value, stays: Maybe.map (\b -> b.from) after } == { button: False, value: Ok (Reject "A booking cannot be cancelled once it has started."), stays: Just today })
}

test "a booking is not found for anyone but its owner" = {
  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
  let oak <- Db.insert Room { name: Text.NonEmpty "Oak" }
  let today <- Date.today
  let id <- Db.insert Booking { room: oak, owner: me, from: Date.addDays 1 today, to: Date.addDays 3 today }
  let visitor <- Test.open (Bookings.Show { id: id })
  let _ <- Test.login { user: Just (User.UserId "someone-else") }
  let stranger <- Test.open (Bookings.Show { id: id })
  Task.succeed ({ visitor: visitor.answer, stranger: stranger.answer } == { visitor: Redirected (Auth.Login {}), stranger: PageMissing })
}
