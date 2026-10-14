```alder-cli
# users and login (auth creates App's session, Logout, Auth.Login and Password)
alder new table User email:Text.Email@unique name:Text.NonEmpty passwordHash:Text created:Time@created --no-timestamps
alder new auth --users User

# rooms and bookings
alder new table Room name:Text.NonEmpty --no-timestamps
alder new table Booking room:Room.RoomId@ref=Room owner:User.UserId@ref=User@owner from:Date to:Date --no-timestamps

# booking helpers: overlap, the day a booking ends, next free day
alder new decl Booking overlaps "Date -> Date -> Booking -> Bool"
alder fill Booking.overlaps - <<'EOF'
\s e b -> s < b.to && b.from < e
EOF

alder new decl Booking startText "Booking -> Text"
alder fill Booking.startText - <<'EOF'
\b -> Date.toText b.from
EOF

alder new decl Booking passOn "Booking -> Date -> Date"
alder fill Booking.passOn - <<'EOF'
\b day -> if b.from <= day && day < b.to then b.to else day
EOF

alder new decl Booking freeFrom "Room.RoomId -> Date -> Query Date"
alder fill Booking.freeFrom - <<'EOF'
\room today -> {
  let existing <- Db.from Booking |> Db.where (Booking.room.eq room) |> Db.all
  Query.succeed (List.foldl passOn today (List.sortByText startText existing))
}
EOF

alder new test Booking "the next free date skips back-to-back bookings"
alder fill 'Booking."the next free date skips back-to-back bookings"' - <<'EOF'
{
  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
  let room <- Db.insert Room { name: Text.NonEmpty "Oak" }
  let today <- Date.today
  let _ <- Db.insert Booking { room: room, owner: me, from: today, to: Date.addDays 2 today }
  let _ <- Db.insert Booking { room: room, owner: me, from: Date.addDays 2 today, to: Date.addDays 4 today }
  let free <- freeFrom room today
  Task.succeed (free == Date.addDays 4 today)
}
EOF

# /rooms
alder new page Rooms.Index '/rooms' --requires user --blank
alder new type Rooms.Index "Row = { id: Room.RoomId, name: Text, free: Date }"
alder fill --decl Rooms.Index.Model - <<'EOF'
{ rows: List Row }
EOF
alder new decl Rooms.Index rowOf "Room -> Query Row"
alder fill Rooms.Index.rowOf - <<'EOF'
\r -> {
  let today <- Date.today
  let free <- Booking.freeFrom r.id today
  Query.succeed { id: r.id, name: Text.trim r.name, free: free }
}
EOF
alder fill --decl Rooms.Index.load - <<'EOF'
\params session -> {
  let rooms <- Db.from Room |> Db.orderBy Room.name.asc |> Db.all
  let rows <- Query.all (List.map rowOf rooms)
  Query.succeed { rows: rows }
}
EOF
alder fill --decl Rooms.Index.view - <<'EOF'
\model session ->
  <Layout.Page title="Rooms">
    <h1>Rooms</h1>
    <ul id="rooms">
      {for r in model.rows ->
        <li>
          <a class="name" href={Rooms.Show { id: r.id }}>{r.name}</a>
          <span class="free">{Date.toText r.free}</span>
        </li>}
    </ul>
  </Layout.Page>
EOF

# the login goes to /rooms
alder fill --decl Auth.Login.update - <<'EOF'
\msg model session -> match msg {
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
EOF

# /rooms/{id}
alder new page Rooms.Show '/rooms/{id}' --param id:Room.RoomId --requires user --blank
alder fill --decl Rooms.Show.Model - <<'EOF'
{ room: Room, bookings: List Booking }
EOF
alder fill --decl Rooms.Show.load - <<'EOF'
\params session -> {
  let room <- Db.get Room params.id
  let bookings <- Db.from Booking |> Db.where (Booking.room.eq params.id) |> Db.orderBy Booking.from.asc |> Db.all
  match room {
    Just r -> Query.succeed { room: r, bookings: bookings },
    Nothing -> Query.notFound,
  }
}
EOF
alder fill --decl Rooms.Show.view - <<'EOF'
\model session ->
  <Layout.Page title={model.room.name}>
    <h1 id="name">{model.room.name}</h1>
    <ul id="bookings">
      {for b in model.bookings -> <li><span class="from">{Date.toText b.from}</span> to <span class="to">{Date.toText b.to}</span></li>}
    </ul>
  </Layout.Page>
EOF
alder new form Rooms.Show NewBooking from:Date to:Date --msg Book
alder fill --decl Rooms.Show.update - <<'EOF'
\msg model session -> match msg {
  Book p -> {
    let existing <- Db.from Booking |> Db.where (Booking.room.eq model.room.id) |> Db.all
    let clash = List.any (Booking.overlaps p.form.from p.form.to) existing
    let problem = if p.form.to <= p.form.from then "The booking must end after it starts." else if clash then "Those days are already booked for this room." else ""
    if problem == "" then {
      let _ <- Db.insertMine Booking { room: model.room.id, from: p.form.from, to: p.form.to }
      Task.succeed (Redirect (Rooms.Show { id: model.room.id }))
    } else Task.succeed (Reject problem)
  },
}
EOF
alder fill --decl Rooms.Show.view - <<'EOF'
\model session ->
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

alder new test Rooms.Show "an overlapping booking is refused and not stored"
alder fill 'Rooms.Show."an overlapping booking is refused and not stored"' - <<'EOF'
{
  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
  let room <- Db.insert Room { name: Text.NonEmpty "Oak" }
  let today <- Date.today
  let _ <- Db.insert Booking { room: room, owner: me, from: today, to: Date.addDays 3 today }
  let _ <- Test.login { user: Just me }
  let _ <- Test.get (Rooms.Show { id: room })
  let r <- Test.submit NewBooking { from: Date.addDays 2 today, to: Date.addDays 5 today }
  let n <- Db.from Booking |> Db.where (Booking.room.eq room) |> Db.count
  Task.succeed ({ answer: r.answer, n: n } == { answer: FormRejected [{ field: "", message: "Those days are already booked for this room." }], n: 1 })
}
EOF

alder new test Rooms.Show "a booking that starts on the other's last day is stored"
alder fill 'Rooms.Show."a booking that starts on the other'"'"'s last day is stored"' - <<'EOF'
{
  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
  let room <- Db.insert Room { name: Text.NonEmpty "Oak" }
  let today <- Date.today
  let _ <- Db.insert Booking { room: room, owner: me, from: today, to: Date.addDays 3 today }
  let _ <- Test.login { user: Just me }
  let _ <- Test.get (Rooms.Show { id: room })
  let r <- Test.submit NewBooking { from: Date.addDays 3 today, to: Date.addDays 5 today }
  let n <- Db.from Booking |> Db.where (Booking.room.eq room) |> Db.count
  Task.succeed ({ answer: r.answer, n: n } == { answer: Redirected (Rooms.Show { id: room }), n: 2 })
}
EOF

alder new test Rooms.Show "a booking that ends before it starts is refused"
alder fill 'Rooms.Show."a booking that ends before it starts is refused"' - <<'EOF'
{
  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
  let room <- Db.insert Room { name: Text.NonEmpty "Oak" }
  let today <- Date.today
  let _ <- Test.login { user: Just me }
  let _ <- Test.get (Rooms.Show { id: room })
  let r <- Test.submit NewBooking { from: Date.addDays 4 today, to: Date.addDays 2 today }
  let n <- Db.from Booking |> Db.where (Booking.room.eq room) |> Db.count
  Task.succeed ({ answer: r.answer, n: n } == { answer: FormRejected [{ field: "", message: "The booking must end after it starts." }], n: 0 })
}
EOF

# /bookings
alder new page Bookings.Index '/bookings' --requires user --blank
alder new type Bookings.Index "Row = { id: Booking.BookingId, room: Text, from: Date }"
alder fill --decl Bookings.Index.Model - <<'EOF'
{ rows: List Row }
EOF
alder new decl Bookings.Index roomText "Room -> Text"
alder fill Bookings.Index.roomText - <<'EOF'
\r -> Text.trim r.name
EOF
alder new decl Bookings.Index rowOf "Booking -> Query Row"
alder fill Bookings.Index.rowOf - <<'EOF'
\b -> {
  let room <- Db.get Room b.room
  Query.succeed { id: b.id, room: Maybe.withDefault "?" (Maybe.map roomText room), from: b.from }
}
EOF
alder fill --decl Bookings.Index.load - <<'EOF'
\params session -> {
  let bookings <- Db.mine Booking |> Db.orderBy Booking.from.asc |> Db.all
  let rows <- Query.all (List.map rowOf bookings)
  Query.succeed { rows: rows }
}
EOF
alder fill --decl Bookings.Index.view - <<'EOF'
\model session ->
  <Layout.Page title="Your bookings">
    <h1>Your bookings</h1>
    <ul id="mine">
      {for b in model.rows ->
        <li>
          <a href={Bookings.Show { id: b.id }}><span class="room">{b.room}</span> <span class="from">{Date.toText b.from}</span></a>
        </li>}
    </ul>
  </Layout.Page>
EOF

# /bookings/{id}
alder new page Bookings.Show '/bookings/{id}' --param id:Booking.BookingId --requires user --blank
alder fill --decl Bookings.Show.Model - <<'EOF'
{ booking: Booking, room: Room, cancellable: Bool }
EOF
alder fill --decl Bookings.Show.load - <<'EOF'
\params session -> {
  let booking <- Db.getMine Booking params.id
  let room <- Db.get Room booking.room
  let today <- Date.today
  match room {
    Just r -> Query.succeed { booking: booking, room: r, cancellable: today < booking.from },
    Nothing -> Query.notFound,
  }
}
EOF
alder fill --decl Bookings.Show.view - <<'EOF'
\model session ->
  <Layout.Page title="Booking">
    <h1>Booking</h1>
    <p>Room: <span id="room">{model.room.name}</span></p>
    <p>From <span id="from">{Date.toText model.booking.from}</span> to <span id="to">{Date.toText model.booking.to}</span></p>
  </Layout.Page>
EOF
alder new msg Bookings.Show Cancel
alder fill --decl Bookings.Show.update - <<'EOF'
\msg model session -> match msg {
  Cancel p -> {
    let today <- Date.today
    if today < model.booking.from then {
      let _ <- Db.deleteMine Booking model.booking.id
      Task.succeed (Redirect (Bookings.Index {}))
    } else Task.succeed Rerender
  },
}
EOF
alder fill --decl Bookings.Show.view - <<'EOF'
\model session ->
  <Layout.Page title="Booking">
    <h1>Booking</h1>
    <p>Room: <span id="room">{model.room.name}</span></p>
    <p>From <span id="from">{Date.toText model.booking.from}</span> to <span id="to">{Date.toText model.booking.to}</span></p>
    {if model.cancellable then <button id="cancel" click={Cancel}>Cancel booking</button> else Html.none}
  </Layout.Page>
EOF

alder new test Bookings.Show "the owner cancels a booking before its first day"
alder fill 'Bookings.Show."the owner cancels a booking before its first day"' - <<'EOF'
{
  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
  let room <- Db.insert Room { name: Text.NonEmpty "Oak" }
  let today <- Date.today
  let id <- Db.insert Booking { room: room, owner: me, from: Date.addDays 1 today, to: Date.addDays 2 today }
  let _ <- Test.login { user: Just me }
  let _ <- Test.get (Bookings.Show { id: id })
  let r <- Test.click Cancel
  let after <- Db.get Booking id
  Task.succeed ({ answer: r.answer, left: Maybe.isJust after } == { answer: Redirected (Bookings.Index {}), left: False })
}
EOF

alder new test Bookings.Show "a booking that starts today cannot be cancelled"
alder fill 'Bookings.Show."a booking that starts today cannot be cancelled"' - <<'EOF'
{
  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
  let room <- Db.insert Room { name: Text.NonEmpty "Oak" }
  let today <- Date.today
  let id <- Db.insert Booking { room: room, owner: me, from: today, to: Date.addDays 1 today }
  let _ <- Test.login { user: Just me }
  let page <- Test.get (Bookings.Show { id: id })
  let r <- Test.run (update Cancel page.model { user: me })
  let after <- Db.get Booking id
  Task.succeed ({ shown: Test.has "#cancel" page.doc, value: r.value, left: Maybe.isJust after } == { shown: False, value: Ok Rerender, left: True })
}
EOF

# final check of the whole app
alder check
alder test
```
