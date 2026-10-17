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

-- Rooms are added outside the app: no page inserts one.
table Room {
  id: RoomId @id,
  name: Text.NonEmpty,
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

-- Whether the booking holds the room on that day.
holds : Date -> Booking -> Bool
holds day booking = booking.from <= day && day < booking.to

-- The first day, `day` or later, that none of the bookings holds. A booking that holds `day`
-- ends after it, so each step moves forward and the search ends.
nextFree : Date -> List Booking -> Date
nextFree day bookings = match List.find (holds day) bookings {
  Just b -> nextFree b.to bookings,
  Nothing -> day,
}

test "back-to-back bookings move the next free date to the last one's end" = {
  let today <- Date.today
  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
  let room <- Db.insert Room { name: Text.NonEmpty "Blue" }
  let _ <- Db.insert Booking { room: room, owner: me, from: today, to: Date.addDays 2 today }
  let _ <- Db.insert Booking { room: room, owner: me, from: Date.addDays 2 today, to: Date.addDays 4 today }
  let bookings <- Db.from Booking |> Db.all
  Task.succeed (Date.toText (nextFree today bookings) == Date.toText (Date.addDays 4 today))
}

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

test "a visitor is sent to log in, and can log out" = {
  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
  let visitor <- Test.open (Rooms.Index {})
  let _ <- Test.login { user: Just me }
  let _ <- Test.get (Rooms.Index {})
  let out <- Test.click App.Logout
  Task.succeed ({ visitor: visitor.answer, out: out.answer, user: out.session.user } == { visitor: Redirected (Auth.Login {}), out: Redirected (Auth.Login {}), user: Nothing })
}

--- file: app/Rooms/Index.alder ---
module Rooms.Index

import Room
import Booking
import User
import Layout

page "/rooms" { } requires user

type Row = { room: Room, free: Date }

type Model = { rooms: List Row }

row : Date -> List Booking -> Room -> Row
row today bookings room = {
  let own = List.filter (\b -> b.room == room.id) bookings
  { room: room, free: Booking.nextFree today own }
}

load : Params -> Session -> Query Model
load params session = {
  let today <- Date.today
  let rooms <- Db.from Room |> Db.orderBy Room.name.asc |> Db.all
  let bookings <- Db.from Booking |> Db.where (Booking.to.gt today) |> Db.all
  Query.succeed { rooms: List.map (row today bookings) rooms }
}

view : Model -> Session -> Html Msg
view model session =
  <Layout.Page title="Rooms">
    <h1>Rooms</h1>
    <ul id="rooms">
      {for r in model.rooms ->
        <li>
          <a class="name" href={Rooms.Show { id: r.room.id }}>{r.room.name}</a>
          <span class="free">{Date.toText r.free}</span>
        </li>}
    </ul>
    <p><a href={Bookings.Index {}}>My bookings</a></p>
    <al-button variant="outline" click={App.Logout}>Log out</al-button>
  </Layout.Page>

test "the next free date follows back-to-back bookings" = {
  let today <- Date.today
  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
  let blue <- Db.insert Room { name: Text.NonEmpty "Blue" }
  let _ <- Db.insert Room { name: Text.NonEmpty "Amber" }
  let _ <- Db.insert Booking { room: blue, owner: me, from: today, to: Date.addDays 2 today }
  let _ <- Db.insert Booking { room: blue, owner: me, from: Date.addDays 2 today, to: Date.addDays 4 today }
  let _ <- Test.login { user: Just me }
  let page <- Test.get (Rooms.Index {})
  Task.succeed ({ names: Test.texts "#rooms .name" page.doc, free: Test.texts "#rooms .free" page.doc } == { names: ["Amber", "Blue"], free: [Date.toText today, Date.toText (Date.addDays 4 today)] })
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

form NewBooking = {
  from: Date,
  to: Date,
}

msg Book form NewBooking

-- Two bookings of one room overlap when each starts before the other ends. Everything is
-- decided before the insert, so a refused booking stores nothing.
update : Msg -> Model -> Session -> Task Nav
update msg model session = match msg {
  Book p -> {
    let start = p.form.from
    let end = p.form.to
    let clash <-
      Db.from Booking
        |> Db.where (Booking.room.eq model.room.id)
        |> Db.where (Booking.from.lt end)
        |> Db.where (Booking.to.gt start)
        |> Db.first
    if end <= start then Task.succeed (Reject "The end date must be after the start date.") else match clash {
      Just other -> Task.succeed (Reject "The room is already booked for some of those dates."),
      Nothing -> {
        let _ <- Db.insert Booking { room: model.room.id, owner: session.user, from: start, to: end }
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
        <li>
          <span class="from">{Date.toText b.from}</span>
          {" "}
          <span class="to">{Date.toText b.to}</span>
        </li>}
    </ul>
    <al-card>
      <form submit={Book}>
        <Errors form />
        <al-input field="from" label="From" />
        <al-input field="to" label="To" />
        <al-button type="submit">Book</al-button>
      </form>
    </al-card>
    <p><a href={Rooms.Index {}}>All rooms</a></p>
    <al-button variant="outline" click={App.Logout}>Log out</al-button>
  </Layout.Page>

test "an overlapping booking is refused, one starting on the other's end is stored" = {
  let today <- Date.today
  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
  let room <- Db.insert Room { name: Text.NonEmpty "Blue" }
  let _ <- Db.insert Booking { room: room, owner: me, from: Date.addDays 10 today, to: Date.addDays 12 today }
  let _ <- Test.login { user: Just me }
  let _ <- Test.get (Rooms.Show { id: room })
  let overlap <- Test.submit NewBooking { from: Date.addDays 11 today, to: Date.addDays 13 today }
  let afterOverlap <- Db.from Booking |> Db.count
  let adjacent <- Test.submit NewBooking { from: Date.addDays 12 today, to: Date.addDays 14 today }
  let afterAdjacent <- Db.from Booking |> Db.count
  Task.succeed ({ overlap: overlap.answer, n1: afterOverlap, adjacent: adjacent.answer, n2: afterAdjacent } == { overlap: FormRejected [{ field: "", message: "The room is already booked for some of those dates." }], n1: 1, adjacent: Redirected (Rooms.Show { id: room }), n2: 2 })
}

test "a booking whose end is not after its start is refused" = {
  let today <- Date.today
  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
  let room <- Db.insert Room { name: Text.NonEmpty "Blue" }
  let _ <- Test.login { user: Just me }
  let _ <- Test.get (Rooms.Show { id: room })
  let r <- Test.submit NewBooking { from: Date.addDays 5 today, to: Date.addDays 5 today }
  let n <- Db.from Booking |> Db.count
  Task.succeed ({ answer: r.answer, n: n } == { answer: FormRejected [{ field: "", message: "The end date must be after the start date." }], n: 0 })
}

test "bookings of different rooms never conflict" = {
  let today <- Date.today
  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
  let blue <- Db.insert Room { name: Text.NonEmpty "Blue" }
  let red <- Db.insert Room { name: Text.NonEmpty "Red" }
  let _ <- Db.insert Booking { room: blue, owner: me, from: Date.addDays 10 today, to: Date.addDays 12 today }
  let _ <- Test.login { user: Just me }
  let _ <- Test.get (Rooms.Show { id: red })
  let r <- Test.submit NewBooking { from: Date.addDays 10 today, to: Date.addDays 12 today }
  let page <- Test.get (Rooms.Show { id: red })
  Task.succeed ({ answer: r.answer, froms: Test.texts "#bookings .from" page.doc } == { answer: Redirected (Rooms.Show { id: red }), froms: [Date.toText (Date.addDays 10 today)] })
}

--- file: app/Bookings/Index.alder ---
module Bookings.Index

import Room
import Booking
import User
import Layout

page "/bookings" { } requires user

type Model = { bookings: List Booking, rooms: List Room }

load : Params -> Session -> Query Model
load params session = {
  let bookings <-
    Db.from Booking
      |> Db.where (Booking.owner.eq session.user)
      |> Db.orderBy Booking.from.asc
      |> Db.all
  let rooms <- Db.from Room |> Db.all
  Query.succeed { bookings: bookings, rooms: rooms }
}

roomName : List Room -> Room.RoomId -> Html Msg
roomName rooms id = match List.find (\r -> r.id == id) rooms {
  Just r -> Html.text r.name,
  Nothing -> Html.none,
}

view : Model -> Session -> Html Msg
view model session =
  <Layout.Page title="My bookings">
    <h1>My bookings</h1>
    <ul id="mine">
      {for b in model.bookings ->
        <li>
          <a class="room" href={Bookings.Show { id: b.id }}>{roomName model.rooms b.room}</a>
          {" "}
          <span class="from">{Date.toText b.from}</span>
        </li>}
    </ul>
    <p><a href={Rooms.Index {}}>All rooms</a></p>
    <al-button variant="outline" click={App.Logout}>Log out</al-button>
  </Layout.Page>

test "only my bookings are listed, by date" = {
  let today <- Date.today
  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
  let other <- Db.insert User { email: Text.Email "other@example.com", name: Text.NonEmpty "Other", passwordHash: "" }
  let blue <- Db.insert Room { name: Text.NonEmpty "Blue" }
  let red <- Db.insert Room { name: Text.NonEmpty "Red" }
  let _ <- Db.insert Booking { room: blue, owner: me, from: Date.addDays 5 today, to: Date.addDays 6 today }
  let _ <- Db.insert Booking { room: red, owner: me, from: Date.addDays 2 today, to: Date.addDays 3 today }
  let _ <- Db.insert Booking { room: red, owner: other, from: Date.addDays 8 today, to: Date.addDays 9 today }
  let _ <- Test.login { user: Just me }
  let page <- Test.get (Bookings.Index {})
  Task.succeed ({ rooms: Test.texts "#mine .room" page.doc, froms: Test.texts "#mine .from" page.doc } == { rooms: ["Red", "Blue"], froms: [Date.toText (Date.addDays 2 today), Date.toText (Date.addDays 5 today)] })
}

--- file: app/Bookings/Show.alder ---
module Bookings.Show

import Room
import Booking
import User
import Layout

page "/bookings/{id}" { id: Booking.BookingId } requires user

type Model = { booking: Booking, room: Room, canCancel: Bool }

-- Only the user who booked it may see a booking: for anyone else it is a 404, like a missing one.
load : Params -> Session -> Query Model
load params session = {
  let found <- Db.get Booking params.id
  let today <- Date.today
  match found {
    Just b -> {
      let room <- Db.get Room b.room
      match room {
        Just r -> if b.owner == session.user then Query.succeed { booking: b, room: r, canCancel: today < b.from } else Query.notFound,
        Nothing -> Query.notFound,
      }
    },
    Nothing -> Query.notFound,
  }
}

msg Cancel

-- The date is checked again here: a Cancel sent from a page loaded before the booking started
-- is refused, and the booking stays.
update : Msg -> Model -> Session -> Task Nav
update msg model session = match msg {
  Cancel p -> {
    let today <- Date.today
    if today < model.booking.from then {
      let _ <- Db.delete Booking model.booking.id
      Task.succeed (Redirect (Bookings.Index {}))
    } else Task.succeed (Reject "This booking has started and can no longer be cancelled.")
  },
}

view : Model -> Session -> Html Msg
view model session =
  <Layout.Page title="Booking">
    <h1 id="room">{model.room.name}</h1>
    <p>From <span id="from">{Date.toText model.booking.from}</span></p>
    <p>To <span id="to">{Date.toText model.booking.to}</span></p>
    {if model.canCancel then <al-button id="cancel" variant="destructive" click={Cancel}>Cancel</al-button> else Html.none}
    <p><a href={Bookings.Index {}}>My bookings</a></p>
    <al-button variant="outline" click={App.Logout}>Log out</al-button>
  </Layout.Page>

test "the owner cancels a booking before its from date" = {
  let today <- Date.today
  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
  let room <- Db.insert Room { name: Text.NonEmpty "Blue" }
  let id <- Db.insert Booking { room: room, owner: me, from: Date.addDays 3 today, to: Date.addDays 5 today }
  let _ <- Test.login { user: Just me }
  let page <- Test.get (Bookings.Show { id: id })
  let r <- Test.click Cancel
  let after <- Db.get Booking id
  Task.succeed ({ room: Test.text "#room" page.doc, from: Test.text "#from" page.doc, answer: r.answer, gone: after == Nothing } == { room: Just "Blue", from: Just (Date.toText (Date.addDays 3 today)), answer: Redirected (Bookings.Index {}), gone: True })
}

test "a booking whose from date is today cannot be cancelled" = {
  let today <- Date.today
  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
  let room <- Db.insert Room { name: Text.NonEmpty "Blue" }
  let id <- Db.insert Booking { room: room, owner: me, from: today, to: Date.addDays 2 today }
  let _ <- Test.login { user: Just me }
  let page <- Test.get (Bookings.Show { id: id })
  let r <- Test.run (update Cancel page.model { user: me })
  let n <- Db.from Booking |> Db.count
  Task.succeed ({ button: Test.has "#cancel" page.doc, value: r.value, n: n } == { button: False, value: Ok (Reject "This booking has started and can no longer be cancelled."), n: 1 })
}

test "another user's booking is not found" = {
  let today <- Date.today
  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
  let room <- Db.insert Room { name: Text.NonEmpty "Blue" }
  let id <- Db.insert Booking { room: room, owner: me, from: Date.addDays 3 today, to: Date.addDays 5 today }
  let _ <- Test.login { user: Just (User.UserId "someone-else") }
  let r <- Test.open (Bookings.Show { id: id })
  Task.succeed (r.answer == PageMissing)
}
