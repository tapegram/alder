--- file: app/App.alder ---
module App

import Layout
import Table.User

session { user: Maybe User.UserId }

login = Auth.Login.Login

route Logout = DELETE "/session" { }

Logout.update : Logout.Params -> Session -> Task Response
Logout.update p session = {
  let _ <- Session.put (\s -> { s | user: Nothing })
  Task.succeed (Response.redirect (Auth.Login.Login {}))
}

error : Error -> Html
error err =
  <Layout.Page title="Error">
    <h1>Something went wrong</h1>
    <p>Please try again.</p>
  </Layout.Page>

notFound : Html
notFound = <Layout.Page title="Not found"><h1>Not found</h1></Layout.Page>

test "Logout clears the user and goes to log in" = {
  let r <- Test.run (Logout.update {} { user: Just (User.UserId "me") })
  Task.succeed (
    { value: r.value, trace: r.trace }
      == { value: Ok (Response.redirect (Auth.Login.Login {})), trace: ["Session.put"] }
  )
}

--- file: app/Layout.alder ---
module Layout

Page : { title: Text, children: List Html } -> Html
Page props =
  <html>
    <head>
      <meta charset="utf-8" />
      <title>{props.title}</title>
      <link rel="stylesheet" href={Url.external "/static/al/al.css"} />
      <script type="module" src="/static/al/al.js"></script>
    </head>
    <body>
      <al-page width="narrow"><main><al-stack gap="lg">{props.children}</al-stack></main></al-page>
    </body>
  </html>

nav : Html
nav =
  <nav>
    <a href={Rooms.Index.Index {}}>Rooms</a>
    {" "}
    <a href={Bookings.Index.Index {}}>My bookings</a>
    {" "}
    <button click={App.Logout}>Log out</button>
  </nav>

--- file: app/Table/User.alder ---
module Table.User

type UserId = brand Text

table User {
  id: UserId @id,
  email: Text.Email @unique,
  name: Text.NonEmpty,
  passwordHash: Text,
  created: Time @created,
}

--- file: app/Table/Room.alder ---
module Table.Room

type RoomId = brand Text

table Room { id: RoomId @id, name: Text.NonEmpty }

-- The name of the room with that id among `rooms`, or "" when it is not there.
nameOf : List Room -> RoomId -> Text
nameOf rooms id = match List.find (\r -> r.id == id) rooms {
  Just r -> r.name,
  Nothing -> "",
}

--- file: app/Table/Booking.alder ---
module Table.Booking

import Table.Room
import Table.User

type BookingId = brand Text

-- A booking holds its room from `from` up to, but not including, `to`.
table Booking {
  id: BookingId @id,
  room: Room.RoomId @ref Room,
  owner: User.UserId @ref User,
  from: Date,
  to: Date,
}

-- The first date, `today` or later, that none of `bookings` holds.
nextFree : Date -> List Booking -> Date
nextFree today bookings =
  bookings
    |> List.sortBy (\b -> Date.diffDays Date.epoch b.from)
    |> List.foldl (\b d -> if b.from <= d && d < b.to then b.to else d) today

--- file: app/Auth/Login.alder ---
module Auth.Login

import Layout
import Table.User

-- Password checking is outside the language: a capability implemented in Go.
capability Password {
  verify : { email: Text.Email, password: Text } -> Task (Maybe User.UserId),
}

-- In a test, `Test.fake Password` accepts any user who has a row, whatever the password.
fake Password {
  verify args = {
    let found <-
      Db.from User
        |> Db.where (User.email.eq args.email)
        |> Db.first
    Task.succeed (Maybe.map (\u -> u.id) found)
  }
}

route Login  = GET  "/login"   { }
route Submit = POST "/session" { } form Credentials

type Model = { }

Login.load : Login.Params -> Session -> Query Model
Login.load params session = Query.succeed {}

form Credentials = { email: Text.Email, password: Text.Secret }

Submit.update : Submit.Params -> Session -> Task Response
Submit.update p session = {
  let found <- Password.verify { email: p.form.email, password: p.form.password }
  match found {
    Just id -> {
      let _ <- Session.put (\s -> { s | user: Just id })
      Task.succeed (Response.redirect (Rooms.Index.Index {}))
    },
    Nothing -> Task.succeed (Response.invalid "Email or password is incorrect."),
  }
}

Login.view : Model -> Session -> Html
Login.view model session =
  <Layout.Page title="Log in">
    <h1>Log in</h1>
    <al-card>
      <form submit={Submit}>
        <Errors form />
        <al-input field="email" label="Email" />
        <al-input field="password" label="Password" />
        <al-button type="submit">Log in</al-button>
      </form>
    </al-card>
  </Layout.Page>

test "an unknown email is rejected" = {
  let _ <- Test.fake Password
  let _ <- Test.get (Login {})
  let r <- Test.submit Submit {
    email: Text.Email "nobody@example.com",
    password: Text.Secret "secret",
  }
  Task.succeed (
    { answer: r.answer, user: r.session.user }
      == {
        answer: FormRejected [{ field: "", message: "Email or password is incorrect." }],
        user: Nothing,
      }
  )
}

test "logging in through the form" = {
  let _ <- Test.fake Password
  let me <- Db.insert User {
    email: Text.Email "me@example.com",
    name: Text.NonEmpty "Me",
    passwordHash: "",
  }
  let _ <- Test.get (Login {})
  let r <- Test.submit Submit { email: Text.Email "me@example.com", password: Text.Secret "secret" }
  Task.succeed (
    {
      answer: r.answer,
      user: r.session.user,
      title: Test.text "h1" r.doc,
    } == {
      answer: Redirected (Rooms.Index.Index {}),
      user: Just me,
      title: Just "Rooms",
    }
  )
}

test "a visitor is sent to log in" = {
  let visitor <- Test.open (Rooms.Index.Index {})
  Task.succeed (visitor.answer == Redirected (Login {}))
}

--- file: app/Rooms/Index.alder ---
module Rooms.Index

import Layout
import Table.Booking
import Table.Room
import Table.User

route Index = GET "/rooms" { } requires user

type Row = { room: Room, free: Date }

type Model = { rows: List Row }

Index.load : Index.Params -> Session -> Query Model
Index.load params session = {
  let today <- Date.today
  let rooms <-
    Db.from Room
      |> Db.orderBy Room.name.asc
      |> Db.all
  let bookings <-
    Db.from Booking
      |> Db.where (Booking.to.gt today)
      |> Db.orderBy Booking.from.asc
      |> Db.all
  Query.succeed { rows: List.map (row today bookings) rooms }
}

row : Date -> List Booking -> Room -> Row
row today bookings room = {
  room: room,
  free: Booking.nextFree today (List.filter (\b -> b.room == room.id) bookings),
}

Index.view : Model -> Session -> Html
Index.view model session =
  <Layout.Page title="Rooms">
    {Layout.nav}
    <h1>Rooms</h1>
    <ul id="rooms">
      {for r in model.rows ->
        <li>
          <a class="name" href={Rooms.Show.Show { id: r.room.id }}>{r.room.name}</a>
          {" free from "}
          <span class="free">{Date.toText r.free}</span>
        </li>}
    </ul>
  </Layout.Page>

test "the next free date follows back-to-back bookings" = {
  let me <- Db.insert User {
    email: Text.Email "me@example.com",
    name: Text.NonEmpty "Me",
    passwordHash: "",
  }
  let oak <- Db.insert Room { name: Text.NonEmpty "Oak" }
  let _ <- Db.insert Room { name: Text.NonEmpty "Elm" }
  let today <- Date.today
  let _ <- Db.insert Booking {
    room: oak,
    owner: me,
    from: Date.addDays (-5) today,
    to: Date.addDays (-3) today,
  }
  let _ <- Db.insert Booking {
    room: oak,
    owner: me,
    from: today,
    to: Date.addDays 2 today,
  }
  let _ <- Db.insert Booking {
    room: oak,
    owner: me,
    from: Date.addDays 2 today,
    to: Date.addDays 4 today,
  }
  let _ <- Test.login { user: Just me }
  let page <- Test.get (Index {})
  Task.succeed (
    {
      names: Test.texts "#rooms .name" page.doc,
      free: Test.texts "#rooms .free" page.doc,
    } == {
      names: ["Elm", "Oak"],
      free: [Date.toText today, Date.toText (Date.addDays 4 today)],
    }
  )
}

--- file: app/Rooms/Show.alder ---
module Rooms.Show

import Layout
import Table.Booking
import Table.Room
import Table.User

route Show = GET  "/rooms/{id}"          { id: Room.RoomId }                 requires user
route Book = POST "/rooms/{id}/bookings" { id: Room.RoomId } form NewBooking requires user

type Model = { room: Room, bookings: List Booking }

Show.load : Show.Params -> Session -> Query Model
Show.load params session = {
  let found <- Db.get Room params.id
  let bookings <-
    Db.from Booking
      |> Db.where (Booking.room.eq params.id)
      |> Db.orderBy Booking.from.asc
      |> Db.all
  match found {
    Just room -> Query.succeed { room: room, bookings: bookings },
    Nothing -> Query.notFound,
  }
}

form NewBooking = { from: Date, to: Date }

overlapMessage : Text
overlapMessage = "The room is already booked on some of those dates."

orderMessage : Text
orderMessage = "The end date must be after the start date."

-- Decide first, then write: a refused booking stores nothing.
Book.update : Book.Params -> Session -> Task Response
Book.update p session =
  if p.form.to <= p.form.from then
    Task.succeed (Response.invalid orderMessage)
  else {
    let clashes <-
      Db.from Booking
        |> Db.where (Booking.room.eq p.id)
        |> Db.where (Booking.from.lt p.form.to)
        |> Db.where (Booking.to.gt p.form.from)
        |> Db.count
    if clashes > 0 then
      Task.succeed (Response.invalid overlapMessage)
    else {
      let _ <- Db.insert Booking {
        room: p.id,
        owner: session.user,
        from: p.form.from,
        to: p.form.to,
      }
      Task.succeed (Response.redirect (Show { id: p.id }))
    }
  }

Show.view : Model -> Session -> Html
Show.view model session =
  <Layout.Page title={model.room.name}>
    {Layout.nav}
    <h1 id="name">{model.room.name}</h1>
    <ul id="bookings">
      {for b in model.bookings ->
        <li>
          <span class="from">{Date.toText b.from}</span>
          {" to "}
          <span class="to">{Date.toText b.to}</span>
        </li>}
    </ul>
    <al-card>
      <form submit={Book { id: model.room.id }}>
        <Errors form />
        <al-input field="from" label="From" />
        <al-input field="to" label="To" />
        <al-button type="submit">Book</al-button>
      </form>
    </al-card>
  </Layout.Page>

test "an overlapping booking is refused, a back-to-back one is stored" = {
  let me <- Db.insert User {
    email: Text.Email "me@example.com",
    name: Text.NonEmpty "Me",
    passwordHash: "",
  }
  let oak <- Db.insert Room { name: Text.NonEmpty "Oak" }
  let elm <- Db.insert Room { name: Text.NonEmpty "Elm" }
  let today <- Date.today
  let _ <- Db.insert Booking {
    room: oak,
    owner: me,
    from: Date.addDays 1 today,
    to: Date.addDays 3 today,
  }
  let _ <- Test.login { user: Just me }
  let _ <- Test.get (Show { id: oak })
  let overlapping <- Test.submit (Book { id: oak }) {
    from: Date.addDays 2 today,
    to: Date.addDays 4 today,
  }
  let backwards <- Test.submit (Book { id: oak }) {
    from: Date.addDays 6 today,
    to: Date.addDays 6 today,
  }
  let afterRefusals <- Db.from Booking |> Db.count
  let adjacent <- Test.submit (Book { id: oak }) {
    from: Date.addDays 3 today,
    to: Date.addDays 5 today,
  }
  let _ <- Test.get (Show { id: elm })
  let other <- Test.submit (Book { id: elm }) {
    from: Date.addDays 2 today,
    to: Date.addDays 4 today,
  }
  let total <- Db.from Booking |> Db.count
  let page <- Test.get (Show { id: oak })
  Task.succeed (
    {
      overlapping: overlapping.answer,
      backwards: backwards.answer,
      afterRefusals: afterRefusals,
      adjacent: adjacent.answer,
      other: other.answer,
      total: total,
      froms: Test.texts "#bookings .from" page.doc,
    } == {
      overlapping: FormRejected [{ field: "", message: overlapMessage }],
      backwards: FormRejected [{ field: "", message: orderMessage }],
      afterRefusals: 1,
      adjacent: Redirected (Show { id: oak }),
      other: Redirected (Show { id: elm }),
      total: 3,
      froms: [Date.toText (Date.addDays 1 today), Date.toText (Date.addDays 3 today)],
    }
  )
}

--- file: app/Bookings/Index.alder ---
module Bookings.Index

import Layout
import Table.Booking
import Table.Room
import Table.User

route Index = GET "/bookings" { } requires user

type Row = { booking: Booking, room: Text }

type Model = { rows: List Row }

Index.load : Index.Params -> Session -> Query Model
Index.load params session = {
  let bookings <-
    Db.from Booking
      |> Db.where (Booking.owner.eq session.user)
      |> Db.orderBy Booking.from.asc
      |> Db.all
  let rooms <- Db.from Room |> Db.all
  Query.succeed { rows: List.map (row rooms) bookings }
}

row : List Room -> Booking -> Row
row rooms b = { booking: b, room: Room.nameOf rooms b.room }

Index.view : Model -> Session -> Html
Index.view model session =
  <Layout.Page title="My bookings">
    {Layout.nav}
    <h1>My bookings</h1>
    <ul id="mine">
      {for r in model.rows ->
        <li>
          <a href={Bookings.Show.Show { id: r.booking.id }}>
            <span class="room">{r.room}</span>
          </a>
          {" from "}
          <span class="from">{Date.toText r.booking.from}</span>
        </li>}
    </ul>
  </Layout.Page>

test "only my bookings are listed, by date" = {
  let me <- Db.insert User {
    email: Text.Email "me@example.com",
    name: Text.NonEmpty "Me",
    passwordHash: "",
  }
  let other <- Db.insert User {
    email: Text.Email "other@example.com",
    name: Text.NonEmpty "Other",
    passwordHash: "",
  }
  let oak <- Db.insert Room { name: Text.NonEmpty "Oak" }
  let elm <- Db.insert Room { name: Text.NonEmpty "Elm" }
  let today <- Date.today
  let _ <- Db.insert Booking {
    room: oak,
    owner: me,
    from: Date.addDays 3 today,
    to: Date.addDays 4 today,
  }
  let _ <- Db.insert Booking {
    room: elm,
    owner: me,
    from: Date.addDays 1 today,
    to: Date.addDays 2 today,
  }
  let _ <- Db.insert Booking {
    room: elm,
    owner: other,
    from: Date.addDays 5 today,
    to: Date.addDays 6 today,
  }
  let _ <- Test.login { user: Just me }
  let page <- Test.get (Index {})
  Task.succeed (
    {
      rooms: Test.texts "#mine .room" page.doc,
      froms: Test.texts "#mine .from" page.doc,
    } == {
      rooms: ["Elm", "Oak"],
      froms: [Date.toText (Date.addDays 1 today), Date.toText (Date.addDays 3 today)],
    }
  )
}

--- file: app/Bookings/Show.alder ---
module Bookings.Show

import Layout
import Table.Booking
import Table.Room
import Table.User

route Show   = GET    "/bookings/{id}" { id: Booking.BookingId } requires user
route Cancel = DELETE "/bookings/{id}" { id: Booking.BookingId } requires user

type Model = { booking: Booking, room: Text, cancellable: Bool }

-- Another user's booking is answered as a missing one: not found.
Show.load : Show.Params -> Session -> Query Model
Show.load params session = {
  let found <- Db.get Booking params.id
  match found {
    Just b -> if b.owner == session.user then details b else Query.notFound,
    Nothing -> Query.notFound,
  }
}

details : Booking -> Query Model
details b = {
  let room <- Db.get Room b.room
  let today <- Date.today
  Query.succeed {
    booking: b,
    room: Room.nameOf (Maybe.toList room) b.room,
    cancellable: today < b.from,
  }
}

-- No `load` runs before a write: the owner and the date are checked again here, so a Cancel
-- from a page loaded before the booking began is refused and the booking stays.
Cancel.update : Cancel.Params -> Session -> Task Response
Cancel.update p session = {
  let found <- Db.get Booking p.id
  let today <- Date.today
  match found {
    Just b -> cancel today session.user b,
    Nothing -> Task.notFound,
  }
}

cancel : Date -> User.UserId -> Booking -> Task Response
cancel today user b =
  if b.owner != user then Task.notFound
  else if today < b.from then {
    let _ <- Db.delete Booking b.id
    Task.succeed (Response.redirect (Bookings.Index.Index {}))
  } else {
    Task.succeed (
      Response.refresh |> Response.error "This booking has begun: it can no longer be cancelled."
    )
  }

Show.view : Model -> Session -> Html
Show.view model session =
  <Layout.Page title="Booking">
    {Layout.nav}
    <h1 id="room">{model.room}</h1>
    <p>From <span id="from">{Date.toText model.booking.from}</span></p>
    <p>To <span id="to">{Date.toText model.booking.to}</span></p>
    {if model.cancellable then
      <button id="cancel" click={Cancel { id: model.booking.id }}>Cancel</button>
    else Html.none}
    <a href={Bookings.Index.Index {}}>My bookings</a>
  </Layout.Page>

test "the owner cancels a booking before its from date" = {
  let me <- Db.insert User {
    email: Text.Email "me@example.com",
    name: Text.NonEmpty "Me",
    passwordHash: "",
  }
  let oak <- Db.insert Room { name: Text.NonEmpty "Oak" }
  let today <- Date.today
  let id <- Db.insert Booking {
    room: oak,
    owner: me,
    from: Date.addDays 2 today,
    to: Date.addDays 4 today,
  }
  let _ <- Test.login { user: Just me }
  let page <- Test.get (Show { id: id })
  let r <- Test.click (Cancel { id: id })
  let after <- Db.get Booking id
  Task.succeed (
    {
      room: Test.text "#room" page.doc,
      answer: r.answer,
      gone: after == Nothing,
    } == {
      room: Just "Oak",
      answer: Redirected (Bookings.Index.Index {}),
      gone: True,
    }
  )
}

-- 1772582400000 is 2026-03-04T00:00:00Z.
test "a booking whose from date is today cannot be cancelled" = {
  let _ <- Test.setClock (Time.fromMillis 1772582400000)
  let me <- Db.insert User {
    email: Text.Email "me@example.com",
    name: Text.NonEmpty "Me",
    passwordHash: "",
  }
  let oak <- Db.insert Room { name: Text.NonEmpty "Oak" }
  let today <- Date.today
  let started <- Db.insert Booking {
    room: oak,
    owner: me,
    from: today,
    to: Date.addDays 1 today,
  }
  let tomorrow <- Db.insert Booking {
    room: oak,
    owner: me,
    from: Date.addDays 1 today,
    to: Date.addDays 2 today,
  }
  let _ <- Test.login { user: Just me }
  let startedPage <- Test.get (Show { id: started })
  let early <- Test.get (Show { id: tomorrow })
  let _ <- Test.advance (24 * 60 * 60 * 1000)
  let r <- Test.click (Cancel { id: tomorrow })
  let kept <- Db.get Booking tomorrow
  let again <- Test.get (Show { id: tomorrow })
  Task.succeed (
    {
      startedButton: Test.has "#cancel" startedPage.doc,
      earlyButton: Test.has "#cancel" early.doc,
      answer: r.answer,
      kept: Maybe.isJust kept,
      laterButton: Test.has "#cancel" again.doc,
    } == {
      startedButton: False,
      earlyButton: True,
      answer: Refreshed,
      kept: True,
      laterButton: False,
    }
  )
}

test "someone else's booking is not found" = {
  let me <- Db.insert User {
    email: Text.Email "me@example.com",
    name: Text.NonEmpty "Me",
    passwordHash: "",
  }
  let oak <- Db.insert Room { name: Text.NonEmpty "Oak" }
  let today <- Date.today
  let id <- Db.insert Booking {
    room: oak,
    owner: me,
    from: Date.addDays 2 today,
    to: Date.addDays 3 today,
  }
  let _ <- Test.login { user: Just (User.UserId "someone-else") }
  let page <- Test.open (Show { id: id })
  let after <- Db.get Booking id
  Task.succeed (
    { answer: page.answer, kept: Maybe.isJust after } == { answer: PageMissing, kept: True }
  )
}
