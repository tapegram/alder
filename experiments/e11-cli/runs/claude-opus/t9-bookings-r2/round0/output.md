```alder-cli
# users and logging in
alder new table User email:Text.Email@unique name:Text.NonEmpty passwordHash:Text created:Time@created --no-timestamps
alder new auth --users User

# the tables
alder new table Room name:Text.NonEmpty --no-timestamps
alder new table Booking room:Room.RoomId@ref=Room owner:User.UserId@ref=User from:Date to:Date --no-timestamps

# the records the pages show
alder new type Booking "RoomFree = { room: Room, free: Date }"
alder new type Booking "RoomPage = { room: Room, bookings: List Booking }"
alder new type Booking "Line = { booking: Booking, room: Room }"

# the next free date: the first day from `day` on that no booking holds
alder new decl Booking nextFree "Date -> List Booking -> Date"
alder fill Booking.nextFree - <<'EOF'
\day bookings -> match List.find (\b -> b.from <= day && day < b.to) bookings {
  Just b -> nextFree b.to bookings,
  Nothing -> day,
}
EOF

alder new decl Booking freeOf "Date -> List Booking -> Room -> RoomFree"
alder fill Booking.freeOf - <<'EOF'
\today bookings r -> {
  let f : RoomFree = { room: r, free: nextFree today (List.filter (\b -> b.room == r.id) bookings) }
  f
}
EOF

alder new decl Booking roomsFree "Query (List RoomFree)"
alder fill Booking.roomsFree - <<'EOF'
{
  let today <- Date.today
  let rooms <- Db.from Room |> Db.orderBy Room.name.asc |> Db.all
  let bookings <- Db.from Booking |> Db.all
  Query.succeed (List.map (freeOf today bookings) rooms)
}
EOF

alder new decl Booking roomPage "Room.RoomId -> Query RoomPage"
alder fill Booking.roomPage - <<'EOF'
\id -> {
  let found <- Db.get Room id
  match found {
    Just room -> {
      let bookings <- Db.from Booking |> Db.where (Booking.room.eq id) |> Db.orderBy Booking.from.asc |> Db.all
      let p : RoomPage = { room: room, bookings: bookings }
      Query.succeed p
    },
    Nothing -> Query.notFound,
  }
}
EOF

# books the room, or answers why not (nothing is stored then)
alder new decl Booking book "Room.RoomId -> User.UserId -> Date -> Date -> Task (Maybe Text)"
alder fill Booking.book - <<'EOF'
\room user start end -> {
  let clashes <- Db.from Booking |> Db.where (Booking.room.eq room) |> Db.where (Booking.from.lt end) |> Db.where (Booking.to.gt start) |> Db.count
  if end <= start then Task.succeed (Just "The booking must end after it starts.")
  else if clashes > 0 then Task.succeed (Just "The room is already booked for those dates.")
  else {
    let _ <- Db.insert Booking { room: room, owner: user, from: start, to: end }
    Task.succeed Nothing
  }
}
EOF

alder new decl Booking lineOf "List Room -> Booking -> Maybe Line"
alder fill Booking.lineOf - <<'EOF'
\rooms b -> match List.find (\r -> r.id == b.room) rooms {
  Just r -> {
    let l : Line = { booking: b, room: r }
    Just l
  },
  Nothing -> Nothing,
}
EOF

alder new decl Booking ofUser "User.UserId -> Query (List Line)"
alder fill Booking.ofUser - <<'EOF'
\user -> {
  let bookings <- Db.from Booking |> Db.where (Booking.owner.eq user) |> Db.orderBy Booking.from.asc |> Db.all
  let rooms <- Db.from Room |> Db.all
  Query.succeed (List.filterMap (lineOf rooms) bookings)
}
EOF

# one booking of the user; anyone else's, like a missing one, is not found
alder new decl Booking line "User.UserId -> BookingId -> Query Line"
alder fill Booking.line - <<'EOF'
\user id -> {
  let found <- Db.from Booking |> Db.where (Booking.id.eq id) |> Db.where (Booking.owner.eq user) |> Db.first
  match found {
    Just b -> {
      let room <- Db.get Room b.room
      match room {
        Just r -> {
          let l : Line = { booking: b, room: r }
          Query.succeed l
        },
        Nothing -> Query.notFound,
      }
    },
    Nothing -> Query.notFound,
  }
}
EOF

alder new decl Booking canCancel "Date -> Booking -> Bool"
alder fill Booking.canCancel - <<'EOF'
\today b -> today < b.from
EOF

alder new decl Booking cancel "BookingId -> Task Unit"
alder fill Booking.cancel - <<'EOF'
\id -> Db.delete Booking id
EOF

# the pages
alder new page Rooms.Index /rooms --blank --requires user
alder new page Rooms.Show '/rooms/{id}' --param id:Room.RoomId --blank --requires user
alder new page Bookings.Index /bookings --blank --requires user
alder new page Bookings.Show '/bookings/{id}' --param id:Booking.BookingId --blank --requires user

alder fill --list
alder fill Auth.Login.update.Submit - --hole ?afterLogin <<'EOF'
Rooms.Index {}
EOF

# Rooms.Index
alder fill --decl Rooms.Index.Model - <<'EOF'
{ rooms: List Booking.RoomFree }
EOF
alder fill Rooms.Index.load - <<'EOF'
{
  let rooms <- Booking.roomsFree
  Query.succeed { rooms: rooms }
}
EOF
alder fill Rooms.Index.view - <<'EOF'
<Layout.Page title="Rooms">
  <h1>Rooms</h1>
  <ul id="rooms">
    {for r in model.rooms ->
      <li>
        <a class="name" href={Rooms.Show { id: r.room.id }}>{r.room.name}</a>
        {" "}
        <span class="free">{Date.toText r.free}</span>
      </li>}
  </ul>
  <a href={Bookings.Index {}}>My bookings</a>
  <button click={App.Logout}>Log out</button>
</Layout.Page>
EOF

# Rooms.Show
alder fill --decl Rooms.Show.Model - <<'EOF'
{ data: Booking.RoomPage }
EOF
alder fill Rooms.Show.load - <<'EOF'
{
  let data <- Booking.roomPage params.id
  Query.succeed { data: data }
}
EOF
alder fill Rooms.Show.view - <<'EOF'
<Layout.Page title={model.data.room.name}>
  <h1 id="name">{model.data.room.name}</h1>
</Layout.Page>
EOF
alder new form Rooms.Show NewBooking from:Date to:Date --msg Book
alder fill --decl Rooms.Show.view - <<'EOF'
<Layout.Page title={model.data.room.name}>
  <h1 id="name">{model.data.room.name}</h1>
  <ul id="bookings">
    {for b in model.data.bookings ->
      <li>
        <span class="from">{Date.toText b.from}</span>
        {" to "}
        <span class="to">{Date.toText b.to}</span>
      </li>}
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
EOF
alder fill Rooms.Show.update.Book - <<'EOF'
{
  let refused <- Booking.book model.data.room.id session.user p.form.from p.form.to
  match refused {
    Just why -> Task.succeed (Reject why),
    Nothing -> Task.succeed (Redirect (Rooms.Show { id: model.data.room.id })),
  }
}
EOF

# Bookings.Index
alder fill --decl Bookings.Index.Model - <<'EOF'
{ mine: List Booking.Line }
EOF
alder fill Bookings.Index.load - <<'EOF'
{
  let mine <- Booking.ofUser session.user
  Query.succeed { mine: mine }
}
EOF
alder fill Bookings.Index.view - <<'EOF'
<Layout.Page title="My bookings">
  <h1>My bookings</h1>
  <ul id="mine">
    {for l in model.mine ->
      <li>
        <a href={Bookings.Show { id: l.booking.id }}>
          <span class="room">{l.room.name}</span>
          {" "}
          <span class="from">{Date.toText l.booking.from}</span>
        </a>
      </li>}
  </ul>
  <a href={Rooms.Index {}}>Rooms</a>
</Layout.Page>
EOF

# Bookings.Show
alder fill --decl Bookings.Show.Model - <<'EOF'
{ line: Booking.Line, today: Date }
EOF
alder fill Bookings.Show.load - <<'EOF'
{
  let line <- Booking.line session.user params.id
  let today <- Date.today
  Query.succeed { line: line, today: today }
}
EOF
alder fill Bookings.Show.view - <<'EOF'
<Layout.Page title="Booking">
  <h1 id="room">{model.line.room.name}</h1>
</Layout.Page>
EOF
alder new msg Bookings.Show Cancel
alder fill --decl Bookings.Show.view - <<'EOF'
<Layout.Page title="Booking">
  <h1 id="room">{model.line.room.name}</h1>
  <p>From <span id="from">{Date.toText model.line.booking.from}</span> to <span id="to">{Date.toText model.line.booking.to}</span></p>
  {if Booking.canCancel model.today model.line.booking then <button id="cancel" click={Cancel}>Cancel</button> else Html.none}
  <a href={Bookings.Index {}}>My bookings</a>
</Layout.Page>
EOF
alder fill Bookings.Show.update.Cancel - <<'EOF'
{
  let today <- Date.today
  if Booking.canCancel today model.line.booking then {
    let _ <- Booking.cancel model.line.booking.id
    Task.succeed (Redirect (Bookings.Index {}))
  } else Task.succeed (Reject "This booking has started: it can no longer be cancelled.")
}
EOF

# tests
alder new test Rooms.Show "an overlapping booking is refused, a back-to-back one is stored"
alder fill 'Rooms.Show."an overlapping booking is refused, a back-to-back one is stored"' - <<'EOF'
{
  let me <- Db.insert User.User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
  let oak <- Db.insert Room.Room { name: Text.NonEmpty "Oak" }
  let elm <- Db.insert Room.Room { name: Text.NonEmpty "Elm" }
  let today <- Date.today
  let start = Date.addDays 10 today
  let _ <- Db.insert Booking.Booking { room: oak, owner: me, from: start, to: Date.addDays 2 start }
  let _ <- Db.insert Booking.Booking { room: elm, owner: me, from: Date.addDays 2 start, to: Date.addDays 4 start }
  let _ <- Test.login { user: Just me }
  let _ <- Test.get (Rooms.Show { id: oak })
  let clash <- Test.submit NewBooking { from: Date.addDays 1 start, to: Date.addDays 3 start }
  let next <- Test.submit NewBooking { from: Date.addDays 2 start, to: Date.addDays 4 start }
  let backwards <- Test.submit NewBooking { from: Date.addDays 20 start, to: Date.addDays 20 start }
  let after <- Booking.roomPage oak
  Task.succeed ({ clash: clash.answer, next: next.answer, backwards: backwards.answer, froms: List.map (\b -> Date.toText b.from) after.bookings } == { clash: FormRejected [{ field: "", message: "The room is already booked for those dates." }], next: Redirected (Rooms.Show { id: oak }), backwards: FormRejected [{ field: "", message: "The booking must end after it starts." }], froms: [Date.toText start, Date.toText (Date.addDays 2 start)] })
}
EOF

alder new test Rooms.Index "the next free date skips back-to-back bookings"
alder fill 'Rooms.Index."the next free date skips back-to-back bookings"' - <<'EOF'
{
  let me <- Db.insert User.User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
  let oak <- Db.insert Room.Room { name: Text.NonEmpty "Oak" }
  let _ <- Db.insert Room.Room { name: Text.NonEmpty "Elm" }
  let today <- Date.today
  let _ <- Db.insert Booking.Booking { room: oak, owner: me, from: today, to: Date.addDays 2 today }
  let _ <- Db.insert Booking.Booking { room: oak, owner: me, from: Date.addDays 2 today, to: Date.addDays 5 today }
  let _ <- Db.insert Booking.Booking { room: oak, owner: me, from: Date.addDays 6 today, to: Date.addDays 7 today }
  let _ <- Test.login { user: Just me }
  let page <- Test.get (Rooms.Index {})
  Task.succeed ({ names: Test.texts "#rooms .name" page.doc, free: Test.texts "#rooms .free" page.doc } == { names: ["Elm", "Oak"], free: [Date.toText today, Date.toText (Date.addDays 5 today)] })
}
EOF

alder new test Bookings.Show "the owner cancels a booking before its from date"
alder fill 'Bookings.Show."the owner cancels a booking before its from date"' - <<'EOF'
{
  let me <- Db.insert User.User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
  let oak <- Db.insert Room.Room { name: Text.NonEmpty "Oak" }
  let today <- Date.today
  let id <- Db.insert Booking.Booking { room: oak, owner: me, from: Date.addDays 1 today, to: Date.addDays 3 today }
  let _ <- Test.login { user: Just me }
  let page <- Test.get (Bookings.Show { id: id })
  let r <- Test.click Cancel
  let left <- Booking.ofUser me
  Task.succeed ({ button: Test.has "#cancel" page.doc, answer: r.answer, left: List.length left } == { button: True, answer: Redirected (Bookings.Index {}), left: 0 })
}
EOF

alder new test Bookings.Show "a booking whose from date is today cannot be cancelled"
alder fill 'Bookings.Show."a booking whose from date is today cannot be cancelled"' - <<'EOF'
{
  let me <- Db.insert User.User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
  let oak <- Db.insert Room.Room { name: Text.NonEmpty "Oak" }
  let today <- Date.today
  let id <- Db.insert Booking.Booking { room: oak, owner: me, from: today, to: Date.addDays 2 today }
  let _ <- Test.login { user: Just me }
  let page <- Test.get (Bookings.Show { id: id })
  let m = page.model
  let r <- Test.run (update Cancel { m | today: Date.addDays (-1) today } { user: me })
  let left <- Booking.ofUser me
  Task.succeed ({ button: Test.has "#cancel" page.doc, value: r.value, left: List.length left } == { button: False, value: Ok (Reject "This booking has started: it can no longer be cancelled."), left: 1 })
}
EOF

alder fill --list
alder check
alder test
```
