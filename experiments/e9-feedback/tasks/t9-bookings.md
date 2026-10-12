# Task T9: booking rooms

Write a complete new Alder app (not the todo app) for booking the rooms of
an office:

- Users log in with an email and a password, checked by a `Password`
  capability as in the example.
- A room has a name. Rooms are added outside the app: no page adds one.
- A booking is a room, a `from` date and a `to` date, and the user who
  booked it. It holds the room from its `from` date up to, but not
  including, its `to` date: a booking from March 1 to March 3 holds March 1
  and March 2, and another booking of that room may start on March 3.
- No two bookings of one room overlap. A booking that would overlap
  another booking of the same room is refused, and so is one whose `to` is
  not after its `from`: the form is shown again with a message for the
  whole form, and nothing is stored. Bookings of different rooms never
  conflict.
- `/rooms` requires login and lists every room with its next free date:
  the first date, today or later, that none of the room's bookings holds.
- `/rooms/{id}` requires login and shows the room, its bookings, and a
  form to book it.
- `/bookings` requires login and lists the logged-in user's bookings, each
  linking to its page.
- `/bookings/{id}` requires login and shows one booking to the user who
  booked it; for anyone else it is a 404. Its owner can cancel it while
  today is before its `from` date: the page then has a Cancel button.
  From its `from` date on, the page has no Cancel button, and a Cancel the
  server receives anyway (from a page loaded the day before, say) is
  refused: the booking stays.
- `/login` and logout, an `App` module, a `Layout` module and a `User`
  module as in the reference.

**Names.** Use exactly these names; anything not listed here is your choice.
The tables are exactly the ones below, with exactly these fields: add no
further tables or fields.

- `app/User.alder`: module `User`, `type UserId = brand Text`, and
  `table User { id: UserId @id, email: Text.Email @unique, name: Text.NonEmpty, passwordHash: Text, created: Time @created }`.
- `app/App.alder`: module `App`, with exactly
  `session { user: Maybe User.UserId }`, `login = Auth.Login`, and
  `msg Logout`, which answers `Redirect (Auth.Login {})`.
- `app/Room.alder`: module `Room`, `type RoomId = brand Text`, and
  `table Room { id: RoomId @id, name: Text.NonEmpty }`.
- `app/Booking.alder`: module `Booking`, `type BookingId = brand Text`, and
  `table Booking { id: BookingId @id, room: Room.RoomId @ref Room, owner: User.UserId @ref User, from: Date, to: Date }`.
- Dates are shown as `YYYY-MM-DD` (`Date.toText`), as the whole text of
  their element.
- `app/Rooms/Index.alder`: page module `Rooms.Index`,
  `page "/rooms" { } requires user`. The rooms are listed in an element
  with `id="rooms"`, one `<li>` per room, ordered by name; inside it, an
  element with `class="name"` has the room's name as its whole text (a
  link to the room's page), and one with `class="free"` the room's next
  free date.
- `app/Rooms/Show.alder`: page module `Rooms.Show`,
  `page "/rooms/{id}" { id: Room.RoomId } requires user`. The room's name is
  the whole text of an element with `id="name"`. Its bookings are listed in
  an element with `id="bookings"`, one `<li>` per booking, ordered by
  `from`; inside it, elements with `class="from"` and `class="to"` have
  the booking's dates. `form NewBooking = { from: Date, to: Date }`, posted
  by `msg Book form NewBooking`, answers `Redirect (Rooms.Show { id: … })`
  (the same room) when the booking is stored; the form has
  `<Errors form />`, where a refused booking's message is shown.
- `app/Bookings/Index.alder`: page module `Bookings.Index`,
  `page "/bookings" { } requires user`. The user's bookings are listed in
  an element with `id="mine"`, one `<li>` per booking, ordered by `from`;
  inside it, an element with `class="room"` has the room's name, and one
  with `class="from"` the booking's `from` date.
- `app/Bookings/Show.alder`: page module `Bookings.Show`,
  `page "/bookings/{id}" { id: Booking.BookingId } requires user`. The
  room's name is the whole text of an element with `id="room"`, the dates
  of elements with `id="from"` and `id="to"`. `msg Cancel` (no
  parameters) is sent by a button with `id="cancel"`, shown only while the
  booking can be cancelled; it deletes the booking and answers
  `Redirect (Bookings.Index {})`.
- `app/Auth/Login.alder`: page module `Auth.Login`, `page "/login" { }`, with
  the `Password` capability, form `Credentials` and `msg Submit` as in the
  reference; a successful login answers `Redirect (Rooms.Index {})`.

Also write tests (`test "…" = …`, chapter 10) in the modules they test: a
booking that overlaps another of the same room is refused and not stored,
while one that starts on the other's `to` date is stored; the next free
date of a room with back-to-back bookings; the owner cancels a booking
before its `from` date; a booking whose `from` date is today cannot be
cancelled. `alder test` must pass. Do not write tests that call the
`Password` capability.

Output every file, in full, each preceded by a line
`--- file: app/<path> ---`. No explanation, only files.
