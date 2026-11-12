--- file: app/App.alder ---
```alder
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
```

--- file: app/Layout.alder ---
```alder
module Layout

Page : { title: Text, children: List Html } -> Html
Page props =
  <html>
    <head>
      <meta charset="utf-8" />
      <script>try{document.documentElement.classList.toggle("dark",localStorage.theme==="dark"||(!("theme" in localStorage)&&matchMedia("(prefers-color-scheme: dark)").matches))}catch(e){}</script>
      <title>{props.title}</title>
      <link rel="stylesheet" href={Url.external "/static/al/al.css"} />
      <script type="module" src="/static/al/al.js"></script>
      <link rel="stylesheet" href={Url.external "/static/app.css"} />
    </head>
    <body>
      <al-page width="narrow"><main><al-stack gap="lg">{props.children}</al-stack></main></al-page>
    </body>
  </html>
```

--- file: app/Table/User.alder ---
```alder
module Table.User

type UserId = brand Text

table User {
  id: UserId @id,
  email: Text.Email @unique,
  name: Text.NonEmpty,
  passwordHash: Text,
  created: Time @created,
}
```

--- file: app/Table/Room.alder ---
```alder
module Table.Room

type RoomId = brand Text

table Room {
  id: RoomId @id,
  name: Text.NonEmpty,
}
```

--- file: app/Table/Booking.alder ---
```alder
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

-- A date written out in a test.
dateOf : Text -> Date
dateOf text = Date.fromText text ?? Date.epoch

-- Whether the stay from `start` up to, not including, `finish` shares a day with booking `b`.
clashes : Date -> Date -> Booking -> Bool
clashes start finish b = start < b.to && b.from < finish

-- The first day, from `start` on, that no booking of the list holds.
nextFree : List Booking -> Date -> Date
nextFree bookings start =
  match List.find (\(b : Booking) -> b.from <= start && start < b.to) bookings {
    Just b -> nextFree bookings b.to,
    Nothing -> start,
  }

test "the next free day skips back-to-back bookings" =
  nextFree [
    { id: BookingId "a", room: Room.RoomId "r", owner: User.UserId "u", from: dateOf "2026-03-01", to: dateOf "2026-03-03" },
    { id: BookingId "b", room: Room.RoomId "r", owner: User.UserId "u", from: dateOf "2026-03-03", to: dateOf "2026-03-05" },
  ] (dateOf "2026-03-01") == dateOf "2026-03-05"

test "a free room's next free day is the day asked about" =
  nextFree [] (dateOf "2026-03-01") == dateOf "2026-03-01"

test "a stay that starts on the other's last day does not clash" =
  not (clashes (dateOf "2026-03-03") (dateOf "2026-03-05") { id: BookingId "a", room: Room.RoomId "r", owner: User.UserId "u", from: dateOf "2026-03-01", to: dateOf "2026-03-03" })

test "a stay that starts inside the other clashes" =
  clashes (dateOf "2026-03-02") (dateOf "2026-03-04") { id: BookingId "a", room: Room.RoomId "r", owner: User.UserId "u", from: dateOf "2026-03-01", to: dateOf "2026-03-03" }
```

--- file: app/Auth/Login.alder ---
```alder
module Auth.Login

import Layout
import Table.User

-- Password checking is outside the language: a capability implemented in Go.
capability Password {
  verify : { email: Text.Email, password: Text } -> Task (Maybe User.UserId),
}

-- In a test, `Test.fake Password` puts this in the adapter's place: it accepts any user who has
-- a row, whatever the password.
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
      <form submit={Submit {}}>
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
  let r <- Test.submit (Submit {}) { email: Text.Email "nobody@example.com", password: Text.Secret "secret" }
  Task.succeed (
    { answer: r.answer, user: r.session.user }
      == {
        answer: FormRejected [{ field: "", message: "Email or password is incorrect." }],
        user: Nothing,
      }
  )
}

test "logging in goes to the rooms" = {
  let _ <- Test.fake Password
  let me <- Db.insert User {
    email: Text.Email "me@example.com",
    name: Text.NonEmpty "Me",
    passwordHash: "",
  }
  let _ <- Test.get (Login {})
  let r <- Test.submit (Submit {}) { email: Text.Email "me@example.com", password: Text.Secret "secret" }
  Task.succeed (
    { answer: r.answer, user: r.session.user, title: Test.text "h1" r.doc }
      == { answer: Redirected (Rooms.Index.Index {}), user: Just me, title: Just "Rooms" }
  )
}
```

--- file: app/Rooms/Index.alder ---
```alder
module Rooms.Index

import Layout
import Table.Booking
import Table.Room
import Table.User

route Index = GET "/rooms" { } requires user

type Row = { room: Room, free: Date }
type Model = { rows: List Row }

-- A room with its next free day, given every booking and today.
rowOf : Date -> List Booking -> Room -> Row
rowOf today bookings room = {
  room: room,
  free: Booking.nextFree (List.filter (\(b : Booking) -> b.room == room.id) bookings) today,
}

Index.load : Index.Params -> Session -> Query Model
Index.load params session = {
  let today <- Date.today
  let rooms <-
    Db.from Room
      |> Db.orderBy Room.name.asc
      |> Db.all
  let bookings <- Db.from Booking |> Db.all
  Query.succeed { rows: List.map (rowOf today bookings) rooms }
}

Index.view : Model -> Session -> Html
Index.view model session =
  <Layout.Page title="Rooms">
    <h1>Rooms</h1>
    <ul id="rooms">
      {for row in model.rows ->
        <li>
          <a class="name" href={Rooms.Show.Show { id: row.room.id }}>{row.room.name}</a>
          <span class="free">{Date.toText row.free}</span>
        </li>}
    </ul>
    <p><a href={Bookings.Index.Index {}}>Your bookings</a></p>
    <al-button variant="outline" click={App.Logout}>Log out</al-button>
  </Layout.Page>

test "the next free date skips back-to-back bookings" = {
  let me <- Db.insert User {
    email: Text.Email "me@example.com",
    name: Text.NonEmpty "Me",
    passwordHash: "",
  }
  let room <- Db.insert Room { name: Text.NonEmpty "Blue" }
  let _ <- Db.insert Booking {
    room: room,
    owner: me,
    from: Booking.dateOf "2026-03-01",
    to: Booking.dateOf "2026-03-03",
  }
  let _ <- Db.insert Booking {
    room: room,
    owner: me,
    from: Booking.dateOf "2026-03-03",
    to: Booking.dateOf "2026-03-05",
  }
  let _ <- Test.setClock (Time.fromDate (Booking.dateOf "2026-03-01"))
  let _ <- Test.login { user: Just me }
  let page <- Test.get (Index {})
  Task.succeed (Test.text ".free" page.doc == Just "2026-03-05")
}
```

--- file: app/Rooms/Show.alder ---
```alder
module Rooms.Show

import Layout
import Table.Booking
import Table.Room
import Table.User

route Show = GET  "/rooms/{id}"          { id: Room.RoomId } requires user
route Book = POST "/rooms/{id}/bookings" { id: Room.RoomId } form NewBooking requires user

type Model = { room: Room, bookings: List Booking }

form NewBooking = { from: Date, to: Date }

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

Show.view : Model -> Session -> Html
Show.view model session =
  <Layout.Page title={model.room.name}>
    <h1 id="name">{model.room.name}</h1>
    <ul id="bookings">
      {for b in model.bookings ->
        <li>
          <span class="from">{Date.toText b.from}</span> to <span class="to">{Date.toText b.to}</span>
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
    <p><a href={Rooms.Index.Index {}}>All rooms</a></p>
  </Layout.Page>

Book.update : Book.Params -> Session -> Task Response
Book.update p session = {
  let existing <-
    Db.from Booking
      |> Db.where (Booking.room.eq p.id)
      |> Db.all
  let clash = List.any (Booking.clashes p.form.from p.form.to) existing
  if p.form.to <= p.form.from then Task.succeed (Response.invalid "A booking must end after it starts.") else if clash then Task.succeed (Response.invalid "That room is already booked for those days.") else {
    let _ <- Db.insert Booking {
      room: p.id,
      owner: session.user,
      from: p.form.from,
      to: p.form.to,
    }
    Task.succeed (Response.redirect (Show { id: p.id }))
  }
}

test "an overlapping booking is refused and not stored" = {
  let me <- Db.insert User {
    email: Text.Email "me@example.com",
    name: Text.NonEmpty "Me",
    passwordHash: "",
  }
  let room <- Db.insert Room { name: Text.NonEmpty "Blue" }
  let _ <- Db.insert Booking {
    room: room,
    owner: me,
    from: Booking.dateOf "2026-03-01",
    to: Booking.dateOf "2026-03-03",
  }
  let _ <- Test.login { user: Just me }
  let _ <- Test.get (Show { id: room })
  let r <- Test.submit (Book { id: room }) { from: Booking.dateOf "2026-03-02", to: Booking.dateOf "2026-03-04" }
  let n <- Db.from Booking |> Db.count
  Task.succeed (
    { answer: r.answer, bookings: n }
      == {
        answer: FormRejected [{ field: "", message: "That room is already booked for those days." }],
        bookings: 1,
      }
  )
}

test "a booking that starts on the other's last day is stored" = {
  let me <- Db.insert User {
    email: Text.Email "me@example.com",
    name: Text.NonEmpty "Me",
    passwordHash: "",
  }
  let room <- Db.insert Room { name: Text.NonEmpty "Blue" }
  let _ <- Db.insert Booking {
    room: room,
    owner: me,
    from: Booking.dateOf "2026-03-01",
    to: Booking.dateOf "2026-03-03",
  }
  let _ <- Test.login { user: Just me }
  let _ <- Test.get (Show { id: room })
  let r <- Test.submit (Book { id: room }) { from: Booking.dateOf "2026-03-03", to: Booking.dateOf "2026-03-05" }
  let n <- Db.from Booking |> Db.count
  Task.succeed (
    { answer: r.answer, bookings: n }
      == { answer: Redirected (Show { id: room }), bookings: 2 }
  )
}
```

--- file: app/Bookings/Index.alder ---
```alder
module Bookings.Index

import Layout
import Table.Booking
import Table.Room

route Index = GET "/bookings" { } requires user

type Row = { id: Booking.BookingId, room: Text, from: Date }
type Model = { rows: List Row }

nameOf : Room -> Text
nameOf room = room.name

roomName : List Room -> Room.RoomId -> Text
roomName rooms roomId =
  Maybe.withDefault "" (Maybe.map nameOf (List.find (\(r : Room) -> r.id == roomId) rooms))

rowOf : List Room -> Booking -> Row
rowOf rooms b = { id: b.id, room: roomName rooms b.room, from: b.from }

Index.load : Index.Params -> Session -> Query Model
Index.load params session = {
  let bookings <-
    Db.from Booking
      |> Db.where (Booking.owner.eq session.user)
      |> Db.orderBy Booking.from.asc
      |> Db.all
  let rooms <- Db.from Room |> Db.all
  Query.succeed { rows: List.map (rowOf rooms) bookings }
}

Index.view : Model -> Session -> Html
Index.view model session =
  <Layout.Page title="Your bookings">
    <h1>Your bookings</h1>
    <ul id="mine">
      {for row in model.rows ->
        <li>
          <a href={Bookings.Show.Show { id: row.id }}>
            <span class="room">{row.room}</span>
            <span class="from">{Date.toText row.from}</span>
          </a>
        </li>}
    </ul>
    <p><a href={Rooms.Index.Index {}}>Rooms</a></p>
  </Layout.Page>
```

--- file: app/Bookings/Show.alder ---
```alder
module Bookings.Show

import Layout
import Table.Booking
import Table.Room
import Table.User

route Show   = GET    "/bookings/{id}" { id: Booking.BookingId } requires user
route Cancel = DELETE "/bookings/{id}" { id: Booking.BookingId } requires user

type Model = { booking: Booking, room: Text, cancellable: Bool }

nameOf : Room -> Text
nameOf room = room.name

-- The page of a booking, with whether its owner may still cancel it: before its first day.
showOf : Booking -> Query Model
showOf b = {
  let today <- Date.today
  let room <- Db.get Room b.room
  Query.succeed {
    booking: b,
    room: Maybe.withDefault "" (Maybe.map nameOf room),
    cancellable: today < b.from,
  }
}

Show.load : Show.Params -> Session -> Query Model
Show.load params session = {
  let found <- Db.get Booking params.id
  match found {
    Nothing -> Query.notFound,
    Just b -> if b.owner != session.user then Query.notFound else showOf b,
  }
}

Show.view : Model -> Session -> Html
Show.view model session =
  <Layout.Page title="Booking">
    <h1 id="room">{model.room}</h1>
    <p>From <span id="from">{Date.toText model.booking.from}</span></p>
    <p>To <span id="to">{Date.toText model.booking.to}</span></p>
    {if model.cancellable then <al-button id="cancel" variant="destructive" click={Cancel { id: model.booking.id }}>Cancel booking</al-button> else Html.none}
    <p><a href={Bookings.Index.Index {}}>All bookings</a></p>
  </Layout.Page>

-- Refused once the booking has started: the booking stays, and the page says why.
Cancel.update : Cancel.Params -> Session -> Task Response
Cancel.update p session = {
  let found <- Db.get Booking p.id
  let today <- Date.today
  match found {
    Nothing -> Task.notFound,
    Just b -> {
      if b.owner != session.user then Task.notFound else if today < b.from then {
        let _ <- Db.delete Booking p.id
        Task.succeed (Response.redirect (Bookings.Index.Index {}))
      } else {
        Task.succeed (Response.redirect (Show { id: p.id }) |> Response.error "A booking cannot be cancelled once it has started.")
      }
    },
  }
}

test "the owner cancels a booking before its first day" = {
  let me <- Db.insert User {
    email: Text.Email "me@example.com",
    name: Text.NonEmpty "Me",
    passwordHash: "",
  }
  let room <- Db.insert Room { name: Text.NonEmpty "Blue" }
  let id <- Db.insert Booking {
    room: room,
    owner: me,
    from: Booking.dateOf "2026-03-05",
    to: Booking.dateOf "2026-03-07",
  }
  let _ <- Test.setClock (Time.fromDate (Booking.dateOf "2026-03-01"))
  let _ <- Test.login { user: Just me }
  let _ <- Test.get (Show { id: id })
  let r <- Test.click (Cancel { id: id })
  let after <- Db.get Booking id
  Task.succeed (
    { answer: r.answer, kept: Maybe.isJust after }
      == { answer: Redirected (Bookings.Index.Index {}), kept: False }
  )
}

test "a booking that starts today cannot be cancelled" = {
  let me <- Db.insert User {
    email: Text.Email "me@example.com",
    name: Text.NonEmpty "Me",
    passwordHash: "",
  }
  let room <- Db.insert Room { name: Text.NonEmpty "Blue" }
  let id <- Db.insert Booking {
    room: room,
    owner: me,
    from: Booking.dateOf "2026-03-05",
    to: Booking.dateOf "2026-03-07",
  }
  let _ <- Test.setClock (Time.fromDate (Booking.dateOf "2026-03-05"))
  let _ <- Test.login { user: Just me }
  let page <- Test.get (Show { id: id })
  let r <- Test.run (Cancel.update { id: id } { user: me })
  let after <- Db.get Booking id
  Task.succeed (
    {
      button: Test.has "#cancel" page.doc,
      kept: Maybe.isJust after,
      cancelled: r.value == Ok (Response.redirect (Bookings.Index.Index {})),
    } == { button: False, kept: True, cancelled: False }
  )
}

test "another user's booking is not found" = {
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
  let room <- Db.insert Room { name: Text.NonEmpty "Blue" }
  let id <- Db.insert Booking {
    room: room,
    owner: me,
    from: Booking.dateOf "2026-03-05",
    to: Booking.dateOf "2026-03-07",
  }
  let _ <- Test.login { user: Just other }
  let r <- Test.open (Show { id: id })
  Task.succeed (r.answer == PageMissing)
}
```
