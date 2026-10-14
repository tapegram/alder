```alder-cli
# the remaining pages
alder new page Rooms.Show '/rooms/{id}' --param id:Room.RoomId --blank --requires user
alder new page Bookings.Index /bookings --blank --requires user
alder new page Bookings.Show '/bookings/{id}' --param id:Booking.BookingId --blank --requires user

# rooms index view
alder fill Rooms.Index.view - <<'EOF'
<Layout.Page title="Rooms">
  <h1>Rooms</h1>
  <ul id="rooms">
    {for r in model.rooms ->
      <li><a class="name" href={Rooms.Show { id: r.id }}>{r.name}</a> <span class="free">{Date.toText r.free}</span></li>}
  </ul>
</Layout.Page>
EOF

# room page
alder fill --decl Rooms.Show.Model - <<'EOF'
{ room: Room, bookings: List Booking }
EOF
alder fill Rooms.Show.load - <<'EOF'
{
  let found <- Db.get Room params.id
  match found {
    Just r -> {
      let bs <- Db.from Booking |> Db.where (Booking.room.eq r.id) |> Db.orderBy Booking.from.asc |> Db.all
      Query.succeed { room: r, bookings: bs }
    },
    Nothing -> Query.notFound,
  }
}
EOF
alder fill Rooms.Show.view - <<'EOF'
<Layout.Page title="Room">
  <h1>Room</h1>
</Layout.Page>
EOF
alder new form Rooms.Show NewBooking from:Date to:Date --msg Book
alder fill --decl Rooms.Show.update - <<'EOF'
\msg model session -> match msg {
  Book p -> {
    let existing <- Db.from Booking |> Db.where (Booking.room.eq model.room.id) |> Db.all
    if p.form.to <= p.form.from then Task.succeed (Reject "The end date must be after the start date.")
    else if Booking.conflicts p.form.from p.form.to existing then Task.succeed (Reject "That room is already booked in that period.")
    else {
      let _ <- Db.insert Booking { room: model.room.id, owner: session.user, from: p.form.from, to: p.form.to }
      Task.succeed (Redirect (Rooms.Show { id: model.room.id }))
    }
  },
}
EOF
alder fill --decl Rooms.Show.view - <<'EOF'
\model session ->
  <Layout.Page title={model.room.name}>
    <h1 id="name">{model.room.name}</h1>
    <ul id="bookings">
      {for b in model.bookings ->
        <li><span class="from">{Date.toText b.from}</span> <span class="to">{Date.toText b.to}</span></li>}
    </ul>
    <form submit={Book}>
      <Errors form />
      <Input field="from" label="From" />
      <Input field="to" label="To" />
      <button>Book</button>
    </form>
  </Layout.Page>
EOF

# bookings index
alder fill --decl Bookings.Index.Model - <<'EOF'
{ items: List { id: Booking.BookingId, room: Text, from: Date } }
EOF
alder fill Bookings.Index.load - <<'EOF'
{
  let bs <- Db.from Booking |> Db.where (Booking.owner.eq session.user) |> Db.orderBy Booking.from.asc |> Db.all
  let rooms <- Db.from Room |> Db.all
  Query.succeed { items: List.map (\b -> { id: b.id, room: Maybe.withDefault "" (Maybe.map (\r -> Text.trim r.name) (List.find (\r -> r.id == b.room) rooms)), from: b.from }) bs }
}
EOF
alder fill Bookings.Index.view - <<'EOF'
<Layout.Page title="My bookings">
  <h1>My bookings</h1>
  <ul id="mine">
    {for i in model.items ->
      <li><a href={Bookings.Show { id: i.id }}><span class="room">{i.room}</span></a> <span class="from">{Date.toText i.from}</span></li>}
  </ul>
</Layout.Page>
EOF

# booking page
alder fill --decl Bookings.Show.Model - <<'EOF'
{ booking: Booking, room: Text, canCancel: Bool }
EOF
alder fill Bookings.Show.load - <<'EOF'
{
  let found <- Db.get Booking params.id
  match found {
    Just b -> {
      if b.owner != session.user then Query.notFound else {
        let r <- Db.get Room b.room
        let today <- Date.today
        Query.succeed { booking: b, room: Maybe.withDefault "" (Maybe.map (\x -> Text.trim x.name) r), canCancel: today < b.from }
      }
    },
    Nothing -> Query.notFound,
  }
}
EOF
alder fill Bookings.Show.view - <<'EOF'
<Layout.Page title="Booking">
  <h1>Booking</h1>
</Layout.Page>
EOF
alder new msg Bookings.Show Cancel
alder fill --decl Bookings.Show.update - <<'EOF'
\msg model session -> match msg {
  Cancel p -> {
    let today <- Date.today
    if today < model.booking.from then {
      let _ <- Db.delete Booking model.booking.id
      Task.succeed (Redirect (Bookings.Index {}))
    } else Task.succeed Rerender
  },
}
EOF
alder fill --decl Bookings.Show.view - <<'EOF'
\model session ->
  <Layout.Page title="Booking">
    <h1>Booking</h1>
    <p id="room">{model.room}</p>
    <p id="from">{Date.toText model.booking.from}</p>
    <p id="to">{Date.toText model.booking.to}</p>
    {if model.canCancel then <button id="cancel" click={Cancel}>Cancel</button> else Html.none}
  </Layout.Page>
EOF

# login target
alder fill Auth.Login.update --hole ?afterLogin - <<'EOF'
Rooms.Index {}
EOF

# tests
alder new test Booking "next free date with back-to-back bookings"
alder fill 'Booking."next free date with back-to-back bookings"' - <<'EOF'
{
  let today <- Date.today
  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
  let room <- Db.insert Room { name: Text.NonEmpty "Blue" }
  let _ <- Db.insert Booking { room: room, owner: me, from: today, to: Date.addDays 2 today }
  let _ <- Db.insert Booking { room: room, owner: me, from: Date.addDays 2 today, to: Date.addDays 5 today }
  let bs <- Db.from Booking |> Db.all
  Task.succeed (nextFree today bs == Date.addDays 5 today)
}
EOF
alder new test Rooms.Show "overlap is refused and back-to-back is stored"
alder fill 'Rooms.Show."overlap is refused and back-to-back is stored"' - <<'EOF'
{
  let today <- Date.today
  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
  let room <- Db.insert Room { name: Text.NonEmpty "Blue" }
  let _ <- Db.insert Booking { room: room, owner: me, from: today, to: Date.addDays 2 today }
  let _ <- Test.login { user: Just me }
  let _ <- Test.get (Rooms.Show { id: room })
  let bad <- Test.submit NewBooking { from: Date.addDays 1 today, to: Date.addDays 3 today }
  let good <- Test.submit NewBooking { from: Date.addDays 2 today, to: Date.addDays 4 today }
  let n <- Db.from Booking |> Db.count
  Task.succeed ({ bad: bad.answer, good: good.answer, n: n } == { bad: FormRejected [{ field: "", message: "That room is already booked in that period." }], good: Redirected (Rooms.Show { id: room }), n: 2 })
}
EOF
alder new test Bookings.Show "the owner cancels before the from date"
alder fill 'Bookings.Show."the owner cancels before the from date"' - <<'EOF'
{
  let today <- Date.today
  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
  let room <- Db.insert Room { name: Text.NonEmpty "Blue" }
  let id <- Db.insert Booking { room: room, owner: me, from: Date.addDays 1 today, to: Date.addDays 3 today }
  let _ <- Test.login { user: Just me }
  let _ <- Test.get (Bookings.Show { id: id })
  let r <- Test.click Cancel
  let after <- Db.get Booking id
  Task.succeed ({ answer: r.answer, there: Maybe.isJust after } == { answer: Redirected (Bookings.Index {}), there: False })
}
EOF
alder new test Bookings.Show "a booking from today cannot be cancelled"
alder fill 'Bookings.Show."a booking from today cannot be cancelled"' - <<'EOF'
{
  let today <- Date.today
  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
  let room <- Db.insert Room { name: Text.NonEmpty "Blue" }
  let id <- Db.insert Booking { room: room, owner: me, from: today, to: Date.addDays 2 today }
  let _ <- Test.login { user: Just me }
  let page <- Test.get (Bookings.Show { id: id })
  let r <- Test.run (update Cancel page.model { user: me })
  let after <- Db.get Booking id
  Task.succeed ({ button: Test.has "#cancel" page.doc, value: r.value, there: Maybe.isJust after } == { button: False, value: Ok Rerender, there: True })
}
EOF
alder check
alder test
```
