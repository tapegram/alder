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

-- Whether booking b holds a day of the half-open range [from, to): the two overlap.
clashes : Date -> Date -> Booking -> Bool
clashes from to b = b.from < to && from < b.to

-- The first day, from `day` on, that no booking holds.
freeFrom : Date -> List Booking -> Date
freeFrom day bookings =
  match List.find (\b -> b.from <= day && day < b.to) bookings {
    Just b -> freeFrom b.to bookings,
    Nothing -> day,
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

type RoomRow = { room: Room, free: Date }

type Model = { rows: List RoomRow }

load : Params -> Session -> Query Model
load params session = {
  let today <- Date.today
  let rooms <-
    Db.from Room
      |> Db.orderBy Room.name.asc
      |> Db.all
  let rows <- Query.all (List.map (roomRow today) rooms)
  Query.succeed { rows: rows }
}

roomRow : Date -> Room -> Query RoomRow
roomRow today room = {
  let bookings <-
    Db.from Booking
      |> Db.where (Booking.room.eq room.id)
      |> Db.all
  Query.succeed { room: room, free: Booking.freeFrom today bookings }
}

view : Model -> Session -> Html Msg
view model session =
  <Layout.Page title="Rooms">
    <h1>Rooms</h1>
    <ul id="rooms">
      {for r in model.rows ->
        <li>
          <a class="name" href={Rooms.Show { id: r.room.id }}>{r.room.name}</a>
          free from <span class="free">{Date.toText r.free}</span>
        </li>}
    </ul>
  </Layout.Page>

test "the next free date skips back-to-back bookings" = {
  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
  let room <- Db.insert Room { name: Text.NonEmpty "Blue" }
  let today <- Date.today
  let _ <- Db.insert Booking { room: room, owner: me, from: today, to: Date.addDays 2 today }
  let _ <- Db.insert Booking { room: room, owner: me, from: Date.addDays 2 today, to: Date.addDays 4 today }
  let _ <- Test.login { user: Just me }
  let page <- Test.get (Rooms.Index {})
  Task.succeed (Test.text "#rooms .free" page.doc == Just (Date.toText (Date.addDays 4 today)))
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
    Just room -> roomPage room,
    Nothing -> Query.notFound,
  }
}

roomPage : Room -> Query Model
roomPage room = {
  let bookings <-
    Db.from Booking
      |> Db.where (Booking.room.eq room.id)
      |> Db.orderBy Booking.from.asc
      |> Db.all
  Query.succeed { room: room, bookings: bookings }
}

form NewBooking = {
  from: Date,
  to: Date,
}

msg Book form NewBooking

update : Msg -> Model -> Session -> Task Nav
update msg model session = match msg {
  Book p -> {
    let booked <- Db.from Booking |> Db.where (Booking.room.eq model.room.id) |> Db.all
    book model.room.id session.user p.form booked
  },
}

-- A booking must end after it starts and must not overlap a booking of the room. A refused
-- booking stores nothing; the form shows the message.
book : Room.RoomId -> User.UserId -> NewBooking -> List Booking -> Task Nav
book room user f booked =
  if f.to <= f.from then Task.succeed (Reject "The end date must be after the start date.") else if List.any (Booking.clashes f.from f.to) booked then Task.succeed (Reject "The room is already booked for some of those days.") else store room user f

store : Room.RoomId -> User.UserId -> NewBooking -> Task Nav
store room user f = {
  let _ <- Db.insert Booking { room: room, owner: user, from: f.from, to: f.to }
  Task.succeed (Redirect (Rooms.Show { id: room }))
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

test "an overlapping booking is refused and a back-to-back one is stored" = {
  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
  let room <- Db.insert Room { name: Text.NonEmpty "Blue" }
  let today <- Date.today
  let _ <- Db.insert Booking { room: room, owner: me, from: Date.addDays 1 today, to: Date.addDays 3 today }
  let _ <- Test.login { user: Just me }
  let _ <- Test.get (Rooms.Show { id: room })
  let clash <- Test.submit NewBooking { from: Date.addDays 2 today, to: Date.addDays 4 today }
  let stored <- Db.from Booking |> Db.count
  let adjacent <- Test.submit NewBooking { from: Date.addDays 3 today, to: Date.addDays 5 today }
  let total <- Db.from Booking |> Db.count
  Task.succeed ({ clash: clash.answer, stored: stored, adjacent: adjacent.answer, total: total } == { clash: FormRejected [{ field: "", message: "The room is already booked for some of those days." }], stored: 1, adjacent: Redirected (Rooms.Show { id: room }), total: 2 })
}

--- file: app/Bookings/Index.alder ---
module Bookings.Index

import Booking
import Room
import Layout

page "/bookings" { } requires user

type Mine = { booking: Booking, room: Room }

type Model = { rows: List Mine }

load : Params -> Session -> Query Model
load params session = {
  let mine <-
    Db.from Booking
      |> Db.where (Booking.owner.eq session.user)
      |> Db.orderBy Booking.from.asc
      |> Db.all
  let rows <- Query.all (List.map withRoom mine)
  Query.succeed { rows: rows }
}

withRoom : Booking -> Query Mine
withRoom b = {
  let found <- Db.get Room b.room
  match found {
    Just room -> Query.succeed { booking: b, room: room },
    Nothing -> Query.notFound,
  }
}

view : Model -> Session -> Html Msg
view model session =
  <Layout.Page title="My bookings">
    <h1>My bookings</h1>
    <ul id="mine">
      {for m in model.rows ->
        <li>
          <a href={Bookings.Show { id: m.booking.id }}><span class="room">{m.room.name}</span></a>
          <span class="from">{Date.toText m.booking.from}</span>
        </li>}
    </ul>
  </Layout.Page>

--- file: app/Bookings/Show.alder ---
module Bookings.Show

import Booking
import Room
import User
import Layout

page "/bookings/{id}" { id: Booking.BookingId } requires user

type Model = { booking: Booking, room: Room, cancellable: Bool }

load : Params -> Session -> Query Model
load params session = {
  let found <- Db.get Booking params.id
  match found {
    Just b -> if b.owner == session.user then loadBooking b else Query.notFound,
    Nothing -> Query.notFound,
  }
}

loadBooking : Booking -> Query Model
loadBooking b = {
  let room <- Db.get Room b.room
  let today <- Date.today
  match room {
    Just r -> Query.succeed { booking: b, room: r, cancellable: today < b.from },
    Nothing -> Query.notFound,
  }
}

msg Cancel

update : Msg -> Model -> Session -> Task Nav
update msg model session = match msg {
  Cancel p -> {
    let today <- Date.today
    if today < model.booking.from then cancelBooking model.booking.id else Task.succeed (Reject "A booking can be cancelled only before it starts.")
  },
}

cancelBooking : Booking.BookingId -> Task Nav
cancelBooking id = {
  let _ <- Db.delete Booking id
  Task.succeed (Redirect (Bookings.Index {}))
}

view : Model -> Session -> Html Msg
view model session =
  <Layout.Page title="Booking">
    <h1 id="room">{model.room.name}</h1>
    <p>From <span id="from">{Date.toText model.booking.from}</span> to <span id="to">{Date.toText model.booking.to}</span></p>
    {if model.cancellable then <button id="cancel" click={Cancel}>Cancel booking</button> else Html.none}
    <a href={Bookings.Index {}}>My bookings</a>
  </Layout.Page>

test "the owner cancels a booking before it starts" = {
  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
  let room <- Db.insert Room { name: Text.NonEmpty "Blue" }
  let today <- Date.today
  let id <- Db.insert Booking { room: room, owner: me, from: Date.addDays 2 today, to: Date.addDays 4 today }
  let _ <- Test.login { user: Just me }
  let _ <- Test.get (Bookings.Show { id: id })
  let r <- Test.click Cancel
  let left <- Db.from Booking |> Db.count
  Task.succeed ({ answer: r.answer, left: left } == { answer: Redirected (Bookings.Index {}), left: 0 })
}

test "a booking that starts today cannot be cancelled" = {
  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
  let room <- Db.insert Room { name: Text.NonEmpty "Blue" }
  let today <- Date.today
  let id <- Db.insert Booking { room: room, owner: me, from: today, to: Date.addDays 2 today }
  let _ <- Test.login { user: Just me }
  let page <- Test.get (Bookings.Show { id: id })
  let shown = Test.has "#cancel" page.doc
  let b : Booking = { id: id, room: room, owner: me, from: today, to: Date.addDays 2 today }
  let rm : Room = { id: room, name: Text.NonEmpty "Blue" }
  let r <- Test.run (update Cancel { booking: b, room: rm, cancellable: True } { user: me })
  let left <- Db.from Booking |> Db.count
  Task.succeed ({ shown: shown, value: r.value, left: left } == { shown: False, value: Ok (Reject "A booking can be cancelled only before it starts."), left: 1 })
}

test "a stranger cannot see a booking" = {
  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
  let room <- Db.insert Room { name: Text.NonEmpty "Blue" }
  let today <- Date.today
  let id <- Db.insert Booking { room: room, owner: me, from: Date.addDays 2 today, to: Date.addDays 4 today }
  let stranger = User.UserId "someone-else"
  let _ <- Test.login { user: Just stranger }
  let show <- Test.open (Bookings.Show { id: id })
  Task.succeed (show.answer == PageMissing)
}
