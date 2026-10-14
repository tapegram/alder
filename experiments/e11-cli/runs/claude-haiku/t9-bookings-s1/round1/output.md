```alder-cli
# the pages that the first views link to, created blank
alder new page Rooms.Show '/rooms/{id}' --param id:Room.RoomId --requires user --blank
alder new page Bookings.Index '/bookings' --requires user --blank
alder new page Bookings.Show '/bookings/{id}' --param id:Booking.BookingId --requires user --blank

# Rooms.Index: its load and its view
alder fill Rooms.Index.load - --hole ?load <<'EOF'
{
  let rooms <- Db.from Room |> Db.orderBy Room.name.asc |> Db.all
  let rows <- Query.all (List.map rowOf rooms)
  Query.succeed { rows: rows }
}
EOF
alder fill Rooms.Index.view - --hole ?view <<'EOF'
<Layout.Page title="Rooms">
  <h1>Rooms</h1>
  <nav>
    <a href={Bookings.Index {}}>My bookings</a>
  </nav>
  <ul id="rooms">
    {for r in model.rows ->
      <li>
        <a class="name" href={Rooms.Show { id: r.id }}>{r.name}</a>
        <span class="free">{Date.toText r.free}</span>
      </li>}
  </ul>
</Layout.Page>
EOF

# the login goes to the rooms (the update and the test that logs in through the form)
alder fill Auth.Login.update - --hole ?afterLogin <<'EOF'
Rooms.Index {}
EOF
alder fill 'Auth.Login."logging in through the form"' - --hole ?afterLogin <<'EOF'
Rooms.Index {}
EOF

# Bookings.Index: its row type, its lookup of the room, its model, its load and its view
alder new type Bookings.Index "Row = { id: Booking.BookingId, room: Text, from: Date }"
alder new type Bookings.Index "Lookup = { room: Room.RoomId, booking: Booking.BookingId, user: User.UserId }"
alder fill --decl Bookings.Index.Model - <<'EOF'
{ items: List Row }
EOF
alder new decl Bookings.Index withRoom "Booking -> Query Row"
alder fill Bookings.Index.withRoom - <<'EOF'
\b -> {
  let found <- Db.get Room b.room
  Query.succeed { id: b.id, room: Maybe.withDefault "?" (Maybe.map (\r -> Text.trim r.name) found), from: b.from }
}
EOF
alder fill Bookings.Index.load - --hole ?load <<'EOF'
{
  let mine <- Db.mine Booking |> Db.orderBy Booking.from.asc |> Db.all
  let rows <- Query.all (List.map withRoom mine)
  Query.succeed { items: rows }
}
EOF
alder fill Bookings.Index.view - --hole ?view <<'EOF'
<Layout.Page title="My bookings">
  <h1>My bookings</h1>
  <ul id="mine">
    {for b in model.items ->
      <li>
        <a href={Bookings.Show { id: b.id }}><span class="room">{b.room}</span></a>
        <span class="from">{Date.toText b.from}</span>
      </li>}
  </ul>
  <a href={Rooms.Index {}}>All rooms</a>
</Layout.Page>
EOF

# Rooms.Show: model, load, a view without the form (the form needs the view filled first)
alder new type Rooms.Show "Lookup = { room: Room.RoomId, booking: Booking.BookingId, user: User.UserId }"
alder fill --decl Rooms.Show.Model - <<'EOF'
{ room: Room, bookings: List Booking }
EOF
alder fill Rooms.Show.load - --hole ?load <<'EOF'
{
  let found <- Db.get Room params.id
  match found {
    Nothing -> Query.notFound,
    Just room -> {
      let bookings <- Db.from Booking |> Db.where (Booking.room.eq params.id) |> Db.orderBy Booking.from.asc |> Db.all
      Query.succeed { room: room, bookings: bookings }
    },
  }
}
EOF
alder fill Rooms.Show.view - --hole ?view <<'EOF'
<Layout.Page title={model.room.name}>
  <h1 id="name">{model.room.name}</h1>
  <ul id="bookings">
    {for b in model.bookings ->
      <li>
        <span class="from">{Date.toText b.from}</span>
        <span class="to">{Date.toText b.to}</span>
      </li>}
  </ul>
  <a href={Rooms.Index {}}>All rooms</a>
</Layout.Page>
EOF

# the booking form and its message, then the full view and the update
alder new form Rooms.Show NewBooking from:Date to:Date --msg Book
alder fill --decl Rooms.Show.view - <<'EOF'
<Layout.Page title={model.room.name}>
  <h1 id="name">{model.room.name}</h1>
  <ul id="bookings">
    {for b in model.bookings ->
      <li>
        <span class="from">{Date.toText b.from}</span>
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
  <a href={Rooms.Index {}}>All rooms</a>
</Layout.Page>
EOF
alder fill --decl Rooms.Show.update - <<'EOF'
match msg {
  Book p -> {
    let existing <- Db.from Booking |> Db.where (Booking.room.eq model.room.id) |> Db.all
    let clash = List.any (Booking.overlaps p.form.from p.form.to) existing
    if p.form.to <= p.form.from || clash then Task.succeed (Reject "Those dates cannot be booked.") else {
      let _ <- Db.insert Booking { room: model.room.id, owner: session.user, from: p.form.from, to: p.form.to }
      Task.succeed (Redirect (Rooms.Show { id: model.room.id }))
    }
  },
}
EOF

# Rooms.Show tests
alder new test Rooms.Show "an overlapping booking is refused and not stored"
alder fill 'Rooms.Show."an overlapping booking is refused and not stored"' - <<'EOF'
{
  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
  let room <- Db.insert Room { name: Text.NonEmpty "Oak" }
  let today <- Date.today
  let _ <- Db.insert Booking { room: room, owner: me, from: Date.addDays 1 today, to: Date.addDays 3 today }
  let _ <- Test.login { user: Just me }
  let _ <- Test.get (Rooms.Show { id: room })
  let r <- Test.submit NewBooking { from: Date.addDays 2 today, to: Date.addDays 4 today }
  let n <- Db.from Booking |> Db.where (Booking.room.eq room) |> Db.count
  Task.succeed ({ answer: r.answer, count: n } == { answer: FormRejected [{ field: "", message: "Those dates cannot be booked." }], count: 1 })
}
EOF
alder new test Rooms.Show "a booking starting on the other end date is stored"
alder fill 'Rooms.Show."a booking starting on the other end date is stored"' - <<'EOF'
{
  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
  let room <- Db.insert Room { name: Text.NonEmpty "Oak" }
  let today <- Date.today
  let _ <- Db.insert Booking { room: room, owner: me, from: Date.addDays 1 today, to: Date.addDays 3 today }
  let _ <- Test.login { user: Just me }
  let _ <- Test.get (Rooms.Show { id: room })
  let r <- Test.submit NewBooking { from: Date.addDays 3 today, to: Date.addDays 5 today }
  let n <- Db.from Booking |> Db.where (Booking.room.eq room) |> Db.count
  Task.succeed ({ answer: r.answer, count: n } == { answer: Redirected (Rooms.Show { id: room }), count: 2 })
}
EOF

# Bookings.Show: lookup type, model, load, a view without the button
alder new type Bookings.Show "Lookup = { room: Room.RoomId, booking: Booking.BookingId, user: User.UserId }"
alder fill --decl Bookings.Show.Model - <<'EOF'
{ booking: Booking, room: Room, cancellable: Bool }
EOF
alder fill Bookings.Show.load - --hole ?load <<'EOF'
{
  let booking <- Db.getMine Booking params.id
  let found <- Db.get Room booking.room
  let today <- Date.today
  match found {
    Nothing -> Query.notFound,
    Just room -> Query.succeed { booking: booking, room: room, cancellable: today < booking.from },
  }
}
EOF
alder fill Bookings.Show.view - --hole ?view <<'EOF'
<Layout.Page title="Booking">
  <h1>Booking</h1>
  <p id="room">{model.room.name}</p>
  <p id="from">{Date.toText model.booking.from}</p>
  <p id="to">{Date.toText model.booking.to}</p>
  <a href={Bookings.Index {}}>My bookings</a>
</Layout.Page>
EOF

# the Cancel message, then the full view with the button and the update
alder new msg Bookings.Show Cancel
alder fill --decl Bookings.Show.view - <<'EOF'
<Layout.Page title="Booking">
  <h1>Booking</h1>
  <p id="room">{model.room.name}</p>
  <p id="from">{Date.toText model.booking.from}</p>
  <p id="to">{Date.toText model.booking.to}</p>
  {if model.cancellable then <button id="cancel" click={Cancel}>Cancel booking</button> else Html.none}
  <a href={Bookings.Index {}}>My bookings</a>
</Layout.Page>
EOF
alder fill --decl Bookings.Show.update - <<'EOF'
match msg {
  Cancel p -> {
    let today <- Date.today
    if today < model.booking.from then {
      let _ <- Db.deleteMine Booking model.booking.id
      Task.succeed (Redirect (Bookings.Index {}))
    } else Task.fail "a booking cannot be cancelled from its first day"
  },
}
EOF

# Bookings.Show tests
alder new test Bookings.Show "the owner cancels a booking before its first day"
alder fill 'Bookings.Show."the owner cancels a booking before its first day"' - <<'EOF'
{
  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
  let room <- Db.insert Room { name: Text.NonEmpty "Oak" }
  let today <- Date.today
  let id <- Db.insert Booking { room: room, owner: me, from: Date.addDays 2 today, to: Date.addDays 4 today }
  let _ <- Test.login { user: Just me }
  let _ <- Test.get (Bookings.Show { id: id })
  let r <- Test.click Cancel
  let left <- Db.from Booking |> Db.count
  Task.succeed ({ answer: r.answer, left: left } == { answer: Redirected (Bookings.Index {}), left: 0 })
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
  let left <- Db.from Booking |> Db.count
  Task.succeed ({ shown: Test.has "#cancel" page.doc, value: r.value, left: left } == { shown: False, value: Err "a booking cannot be cancelled from its first day", left: 1 })
}
EOF
```
