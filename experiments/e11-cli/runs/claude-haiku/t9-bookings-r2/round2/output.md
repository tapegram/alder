```alder-cli
# Rooms.Show: a booking the room cannot take gets one message for the whole form
alder fill --decl Rooms.Show.update - <<'EOF'
match msg {
  Book p -> {
    let ok <- Booking.book model.room.id session.user p.form.from p.form.to
    if ok then Task.succeed (Redirect (Rooms.Show { id: model.room.id })) else Task.succeed (Reject "Those dates are not free: the booking must end after it starts and not overlap another booking.")
  },
}
EOF
alder fill 'Rooms.Show."submitting NewBooking"' - <<'EOF'
{
  let today <- Date.today
  let me <- Db.insert User { email: Text.Email "email-1@example.com", name: Text.NonEmpty "name-1", passwordHash: "passwordhash-1" }
  let _ <- Test.login { user: Just me }
  let room1 <- Db.insert Room { name: Text.NonEmpty "name-1" }
  let _ <- Test.get (Rooms.Show { id: room1 })
  let r <- Test.submit NewBooking { from: today, to: today }
  Task.succeed (r.answer == FormRejected [{ field: "", message: "Those dates are not free: the booking must end after it starts and not overlap another booking." }])
}
EOF
alder fill 'Rooms.Show."NewBooking refuses an empty from"' - <<'EOF'
{
  let me <- Db.insert User { email: Text.Email "email-1@example.com", name: Text.NonEmpty "name-1", passwordHash: "passwordhash-1" }
  let _ <- Test.login { user: Just me }
  let room1 <- Db.insert Room { name: Text.NonEmpty "name-1" }
  let _ <- Test.get (Rooms.Show { id: room1 })
  let r <- Test.submitRaw NewBooking [{ name: "from", value: "" }, { name: "to", value: "2026-01-01" }]
  let rejected = match r.answer {
    FormRejected errs -> List.length errs == 1,
    _ -> False,
  }
  Task.succeed rejected
}
EOF
alder new test Rooms.Show "a free range is booked and redirects to the room"
alder fill 'Rooms.Show."a free range is booked and redirects to the room"' - <<'EOF'
{
  let today <- Date.today
  let me <- Db.insert User { email: Text.Email "email-1@example.com", name: Text.NonEmpty "name-1", passwordHash: "passwordhash-1" }
  let _ <- Test.login { user: Just me }
  let room1 <- Db.insert Room { name: Text.NonEmpty "name-1" }
  let _ <- Test.get (Rooms.Show { id: room1 })
  let r <- Test.submit NewBooking { from: Date.addDays 1 today, to: Date.addDays 3 today }
  let p <- Booking.roomPage room1
  Task.succeed ({ answer: r.answer, stored: List.length p.bookings } == { answer: Redirected (Rooms.Show { id: room1 }), stored: 1 })
}
EOF

# Bookings.Index: its view needs Bookings.Show, which comes next
alder new page Bookings.Show '/bookings/{id}' --param id:Booking.BookingId --requires user --blank
alder fill --decl Bookings.Show.Model - <<'EOF'
{ booking: Booking, room: Room, cancellable: Bool }
EOF
alder fill Bookings.Show.load - --hole ?load <<'EOF'
{
  let p <- Booking.bookingPage params.id session.user
  Query.succeed { booking: p.booking, room: p.room, cancellable: p.cancellable }
}
EOF
alder fill Bookings.Show.view - --hole ?view <<'EOF'
<Layout.Page title="Booking">
  <h1 id="room">{model.room.name}</h1>
  <p>
    From <span id="from">{Date.toText model.booking.from}</span>
    to <span id="to">{Date.toText model.booking.to}</span>
  </p>
  {if model.cancellable then <al-button id="cancel" variant="destructive" click={Cancel}>Cancel</al-button> else Html.none}
  <al-button variant="ghost" href={Bookings.Index {}}>Back</al-button>
</Layout.Page>
EOF
alder new msg Bookings.Show Cancel
alder fill --decl Bookings.Show.update - <<'EOF'
match msg {
  Cancel p -> {
    if model.cancellable then {
      let _ <- Db.delete Booking model.booking.id
      Task.succeed (Redirect (Bookings.Index {}))
    } else Task.succeed (Redirect (Bookings.Show { id: model.booking.id })),
  },
}
EOF
alder fill 'Bookings.Show."a visitor is sent to log in"' - <<'EOF'
{
  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
  let room <- Db.insert Room { name: Text.NonEmpty "Oak" }
  let today <- Date.today
  let b <- Db.insert Booking { room: room, owner: me, from: Date.addDays 1 today, to: Date.addDays 3 today }
  let visitor <- Test.open (Bookings.Show { id: b })
  Task.succeed (visitor.answer == Redirected (Auth.Login {}))
}
EOF
alder new test Bookings.Show "the owner cancels a booking before it starts"
alder fill 'Bookings.Show."the owner cancels a booking before it starts"' - <<'EOF'
{
  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
  let room <- Db.insert Room { name: Text.NonEmpty "Oak" }
  let today <- Date.today
  let b <- Db.insert Booking { room: room, owner: me, from: Date.addDays 1 today, to: Date.addDays 3 today }
  let _ <- Test.login { user: Just me }
  let _ <- Test.get (Bookings.Show { id: b })
  let r <- Test.click Cancel
  let after <- Db.get Booking b
  Task.succeed ({ answer: r.answer, kept: Maybe.isJust after } == { answer: Redirected (Bookings.Index {}), kept: False })
}
EOF
alder new test Bookings.Show "a booking that starts today cannot be cancelled"
alder fill 'Bookings.Show."a booking that starts today cannot be cancelled"' - <<'EOF'
{
  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
  let room <- Db.insert Room { name: Text.NonEmpty "Oak" }
  let today <- Date.today
  let b <- Db.insert Booking { room: room, owner: me, from: today, to: Date.addDays 2 today }
  let _ <- Test.login { user: Just me }
  let page <- Test.get (Bookings.Show { id: b })
  let p <- Booking.bookingPage b me
  let r <- Test.run (update Cancel { booking: p.booking, room: p.room, cancellable: p.cancellable } { user: me })
  let after <- Db.get Booking b
  Task.succeed ({ button: Test.has "#cancel" page.doc, value: r.value, kept: Maybe.isJust after } == { button: False, value: Ok (Redirect (Bookings.Show { id: b })), kept: True })
}
EOF
alder new test Bookings.Show "a stranger gets not found for a booking"
alder fill 'Bookings.Show."a stranger gets not found for a booking"' - <<'EOF'
{
  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
  let room <- Db.insert Room { name: Text.NonEmpty "Oak" }
  let today <- Date.today
  let b <- Db.insert Booking { room: room, owner: me, from: Date.addDays 1 today, to: Date.addDays 3 today }
  let _ <- Test.login { user: Just (User.UserId "someone-else") }
  let r <- Test.open (Bookings.Show { id: b })
  Task.succeed (r.answer == PageMissing)
}
EOF

# Bookings.Index view, now that Bookings.Show exists
alder fill Bookings.Index.view - --hole ?view <<'EOF'
<Layout.Page title="Bookings">
  <h1>Bookings</h1>
  <ul id="mine">
    {for b in model.rows -> <li><a href={Bookings.Show { id: b.booking.id }}><span class="room">{b.room.name}</span></a> <span class="from">{Date.toText b.booking.from}</span></li>}
  </ul>
</Layout.Page>
EOF

# Booking module tests for the overlap rule
alder new test Booking "an overlapping booking is refused and not stored"
alder fill 'Booking."an overlapping booking is refused and not stored"' - <<'EOF'
{
  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
  let room <- Db.insert Room { name: Text.NonEmpty "Oak" }
  let today <- Date.today
  let first <- book room me (Date.addDays 1 today) (Date.addDays 3 today)
  let second <- book room me (Date.addDays 2 today) (Date.addDays 4 today)
  let n <- Db.from Booking |> Db.where (Booking.room.eq room) |> Db.count
  Task.succeed ({ first: first, second: second, stored: n } == { first: True, second: False, stored: 1 })
}
EOF
alder new test Booking "a booking that starts when another ends is stored"
alder fill 'Booking."a booking that starts when another ends is stored"' - <<'EOF'
{
  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
  let room <- Db.insert Room { name: Text.NonEmpty "Oak" }
  let today <- Date.today
  let first <- book room me today (Date.addDays 2 today)
  let second <- book room me (Date.addDays 2 today) (Date.addDays 4 today)
  let n <- Db.from Booking |> Db.where (Booking.room.eq room) |> Db.count
  Task.succeed ({ first: first, second: second, stored: n } == { first: True, second: True, stored: 2 })
}
EOF

# which name the generated login test has
alder fill --list
```
