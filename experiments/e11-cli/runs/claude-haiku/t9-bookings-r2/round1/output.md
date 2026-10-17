```alder-cli
# the login goes to the room list
alder fill Auth.Login.update - --hole ?afterLogin <<'EOF'
Rooms.Index {}
EOF

# the room page: its model, its load, and a placeholder view so the form can be added
alder new page Rooms.Show '/rooms/{id}' --param id:Room.RoomId --requires user --blank
alder fill --decl Rooms.Show.Model - <<'EOF'
{ room: Room, bookings: List Booking }
EOF
alder fill Rooms.Show.load - --hole ?load <<'EOF'
{
  let p <- Booking.roomPage params.id
  Query.succeed { room: p.room, bookings: p.bookings }
}
EOF
alder fill Rooms.Show.view - --hole ?view <<'EOF'
<Layout.Page title={model.room.name}>
  <h1 id="name">{model.room.name}</h1>
</Layout.Page>
EOF
alder new form Rooms.Show NewBooking from:Date to:Date --msg Book
alder fill --decl Rooms.Show.view - <<'EOF'
<Layout.Page title={model.room.name}>
  <h1 id="name">{model.room.name}</h1>
  <ul id="bookings">
    {for b in model.bookings -> <li><span class="from">{Date.toText b.from}</span> to <span class="to">{Date.toText b.to}</span></li>}
  </ul>
  <al-card>
    <form submit={Book}>
      <Errors form />
      <al-input field="from" label="From" />
      <al-input field="to" label="To" />
      <al-button type="submit">Book</al-button>
    </form>
  </al-card>
</Layout.Page>
EOF
alder fill --decl Rooms.Show.update - <<'EOF'
match msg {
  Book p -> {
    let ok <- Booking.book model.room.id session.user p.form.from p.form.to
    if ok then Task.succeed (Redirect (Rooms.Show { id: model.room.id })) else Task.succeed (Reject "The room is not free for those dates.")
  },
}
EOF

# the room list, now that Rooms.Show exists
alder fill Rooms.Index.view - --hole ?view <<'EOF'
<Layout.Page title="Rooms">
  <h1>Rooms</h1>
  <ul id="rooms">
    {for r in model.rows -> <li><a class="name" href={Rooms.Show { id: r.room.id }}>{r.room.name}</a> <span class="free">{Date.toText r.free}</span></li>}
  </ul>
  <al-button variant="outline" click={App.Logout}>Log out</al-button>
</Layout.Page>
EOF

# the user's bookings
alder new page Bookings.Index '/bookings' --requires user --blank
alder fill --decl Bookings.Index.Model - <<'EOF'
{ rows: List Booking.MyBooking }
EOF
alder fill Bookings.Index.load - --hole ?load <<'EOF'
{
  let rows <- Booking.myBookings session.user
  Query.succeed { rows: rows }
}
EOF
alder fill Bookings.Index.view - --hole ?view <<'EOF'
<Layout.Page title="Bookings">
  <h1>Bookings</h1>
  <ul id="mine">
    {for b in model.rows -> <li><a href={Bookings.Show { id: b.booking.id }}><span class="room">{b.room.name}</span></a> <span class="from">{Date.toText b.booking.from}</span></li>}
  </ul>
</Layout.Page>
EOF

# one booking: its model, load, and a placeholder view so the Cancel message can be added
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
</Layout.Page>
EOF
alder new msg Bookings.Show Cancel
alder fill --decl Bookings.Show.view - <<'EOF'
<Layout.Page title="Booking">
  <h1 id="room">{model.room.name}</h1>
  <p>From <span id="from">{Date.toText model.booking.from}</span> to <span id="to">{Date.toText model.booking.to}</span></p>
  {if model.cancellable then <button id="cancel" click={Cancel}>Cancel booking</button> else Html.none}
  <a href={Bookings.Index {}}>Back to bookings</a>
</Layout.Page>
EOF
alder fill --decl Bookings.Show.update - <<'EOF'
match msg {
  Cancel p -> {
    let today <- Date.today
    if Booking.cancellable model.booking today then {
      let _ <- Db.delete Booking model.booking.id
      Task.succeed (Redirect (Bookings.Index {}))
    } else Task.succeed (Redirect (Bookings.Show { id: model.booking.id })),
  },
}
EOF

# tests of the room page: an overlap is refused, a back-to-back booking is stored
alder new test Rooms.Show "a booking that overlaps another is refused and not stored"
alder fill 'Rooms.Show."a booking that overlaps another is refused and not stored"' - <<'EOF'
{
  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
  let room <- Db.insert Room { name: Text.NonEmpty "Oak" }
  let today <- Date.today
  let _ <- Db.insert Booking { room: room, owner: me, from: Date.addDays 1 today, to: Date.addDays 3 today }
  let _ <- Test.login { user: Just me }
  let _ <- Test.get (Rooms.Show { id: room })
  let r <- Test.submit NewBooking { from: Date.addDays 2 today, to: Date.addDays 4 today }
  let n <- Db.from Booking |> Db.where (Booking.room.eq room) |> Db.count
  Task.succeed ({ answer: r.answer, n: n } == { answer: FormRejected [{ field: "", message: "The room is not free for those dates." }], n: 1 })
}
EOF
alder new test Rooms.Show "a booking that starts on the other's to date is stored"
alder fill 'Rooms.Show."a booking that starts on the other'"'"'s to date is stored"' - <<'EOF'
{
  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
  let room <- Db.insert Room { name: Text.NonEmpty "Oak" }
  let today <- Date.today
  let _ <- Db.insert Booking { room: room, owner: me, from: Date.addDays 1 today, to: Date.addDays 3 today }
  let _ <- Test.login { user: Just me }
  let _ <- Test.get (Rooms.Show { id: room })
  let r <- Test.submit NewBooking { from: Date.addDays 3 today, to: Date.addDays 5 today }
  let n <- Db.from Booking |> Db.where (Booking.room.eq room) |> Db.count
  Task.succeed ({ answer: r.answer, n: n } == { answer: Redirected (Rooms.Show { id: room }), n: 2 })
}
EOF

# tests of cancelling
alder new test Bookings.Show "the owner cancels a booking before its from date"
alder fill 'Bookings.Show."the owner cancels a booking before its from date"' - <<'EOF'
{
  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
  let room <- Db.insert Room { name: Text.NonEmpty "Oak" }
  let today <- Date.today
  let id <- Db.insert Booking { room: room, owner: me, from: Date.addDays 5 today, to: Date.addDays 6 today }
  let _ <- Test.login { user: Just me }
  let _ <- Test.get (Bookings.Show { id: id })
  let r <- Test.click Cancel
  let after <- Db.get Booking id
  Task.succeed ({ answer: r.answer, kept: Maybe.isJust after } == { answer: Redirected (Bookings.Index {}), kept: False })
}
EOF
alder new test Bookings.Show "a booking from today cannot be cancelled"
alder fill 'Bookings.Show."a booking from today cannot be cancelled"' - <<'EOF'
{
  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
  let room <- Db.insert Room { name: Text.NonEmpty "Oak" }
  let today <- Date.today
  let id <- Db.insert Booking { room: room, owner: me, from: today, to: Date.addDays 2 today }
  let _ <- Test.login { user: Just me }
  let page <- Test.get (Bookings.Show { id: id })
  let r <- Test.run (update Cancel page.model { user: me })
  let after <- Db.get Booking id
  Task.succeed ({ shown: Test.has "#cancel" page.doc, value: r.value, kept: Maybe.isJust after } == { shown: False, value: Ok (Redirect (Bookings.Show { id: id })), kept: True })
}
EOF

# the login hole in its test (last: the test's name is the generator's)
alder fill 'Auth.Login."logging in through the form"' - --hole ?afterLogin <<'EOF'
Rooms.Index {}
EOF
```
