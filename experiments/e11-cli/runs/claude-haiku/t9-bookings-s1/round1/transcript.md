### Round 1

```text
$ alder new page Rooms.Show '/rooms/{id}' --param id:Room.RoomId --requires user --blank
wrote app/Rooms/Show.alder
app/Rooms/Show.alder:10:23: warning[W0002]: hole ?load has type Query Rooms.Show.Model
  load params session = ?load
                        ^^^^^
app/Rooms/Show.alder:13:22: warning[W0002]: hole ?view has type Html Rooms.Show.Msg
  view model session = ?view
                       ^^^^^
[exit 0]
$ alder new page Bookings.Index '/bookings' --requires user --blank
wrote app/Bookings/Index.alder
app/Bookings/Index.alder:8:23: warning[W0002]: hole ?load has type Query Bookings.Index.Model
  load params session = ?load
                        ^^^^^
app/Bookings/Index.alder:11:22: warning[W0002]: hole ?view has type Html Bookings.Index.Msg
  view model session = ?view
                       ^^^^^
[exit 0]
$ alder new page Bookings.Show '/bookings/{id}' --param id:Booking.BookingId --requires user --blank
wrote app/Bookings/Show.alder
app/Bookings/Show.alder:12:23: warning[W0002]: hole ?load has type Query Bookings.Show.Model
  load params session = ?load
                        ^^^^^
app/Bookings/Show.alder:15:22: warning[W0002]: hole ?view has type Html Bookings.Show.Msg
  view model session = ?view
                       ^^^^^
[exit 0]
$ alder fill Rooms.Index.load - --hole ?load <<'EOF'
{
  let rooms <- Db.from Room |> Db.orderBy Room.name.asc |> Db.all
  let rows <- Query.all (List.map rowOf rooms)
  Query.succeed { rows: rows }
}
EOF
edited app/Rooms/Index.alder
--- a/app/Rooms/Index.alder
+++ b/app/Rooms/Index.alder
@@ -10,7 +10,11 @@
 type Row = { id: Room.RoomId, name: Text, free: Date }
 
 load : Params -> Session -> Query Model
-load params session = ?load
+load params session = {
+  let rooms <- Db.from Room |> Db.orderBy Room.name.asc |> Db.all
+  let rows <- Query.all (List.map rowOf rooms)
+  Query.succeed { rows: rows }
+}
 
 view : Model -> Session -> Html Msg
 view model session = ?view
app/Rooms/Index.alder:20:22: warning[W0002]: hole ?view has type Html Rooms.Index.Msg
  view model session = ?view
                       ^^^^^
[exit 0]
$ alder fill Rooms.Index.view - --hole ?view <<'EOF'
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
edited app/Rooms/Index.alder
--- a/app/Rooms/Index.alder
+++ b/app/Rooms/Index.alder
@@ -1,6 +1,7 @@
 module Rooms.Index
 
 import Booking
+import Layout
 import Room
 
 page "/rooms" { } requires user
@@ -17,7 +18,19 @@
 }
 
 view : Model -> Session -> Html Msg
-view model session = ?view
+view model session = <Layout.Page title="Rooms">
+  <h1>Rooms</h1>
+  <nav>
+    <a href={Bookings.Index {}}>My bookings</a>
+  </nav>
+  <ul id="rooms">
+    {for r in model.rows ->
+      <li>
+        <a class="name" href={Rooms.Show { id: r.id }}>{r.name}</a>
+        <span class="free">{Date.toText r.free}</span>
+      </li>}
+  </ul>
+</Layout.Page>
 
 rowOf : Room -> Query Row
 rowOf = \r -> {
added import Layout to app/Rooms/Index.alder
[exit 0]
$ alder fill Auth.Login.update - --hole ?afterLogin <<'EOF'
Rooms.Index {}
EOF
edited app/Auth/Login.alder
--- a/app/Auth/Login.alder
+++ b/app/Auth/Login.alder
@@ -44,7 +44,7 @@
     match found {
       Just id -> {
         let _ <- Session.put (\s -> { s | user: Just id })
-        Task.succeed (Redirect (?afterLogin))
+        Task.succeed (Redirect (Rooms.Index {}))
       },
       Nothing -> Task.succeed (Reject "Email or password is incorrect."),
     }
app/Auth/Login.alder:86:85: warning[W0002]: hole ?afterLogin has type Page
    Task.succeed ({ answer: r.answer, user: r.session.user } == { answer: Redirected (?afterLogin), user: Just me })
                                                                                      ^^^^^^^^^^^
[exit 0]
$ alder fill 'Auth.Login."logging in through the form"' - --hole ?afterLogin <<'EOF'
Rooms.Index {}
EOF
edited app/Auth/Login.alder
--- a/app/Auth/Login.alder
+++ b/app/Auth/Login.alder
@@ -83,7 +83,7 @@
   let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "name-1", passwordHash: "passwordhash-1" }
   let _ <- Test.get (Auth.Login {})
   let r <- Test.submit Credentials { email: Text.Email "me@example.com", password: Text.Secret "secret" }
-  Task.succeed ({ answer: r.answer, user: r.session.user } == { answer: Redirected (?afterLogin), user: Just me })
+  Task.succeed ({ answer: r.answer, user: r.session.user } == { answer: Redirected (Rooms.Index {}), user: Just me })
 }
 
 test "a signed-in user logs out here" = {
[exit 0]
$ alder new type Bookings.Index "Row = { id: Booking.BookingId, room: Text, from: Date }"
edited app/Bookings/Index.alder
--- a/app/Bookings/Index.alder
+++ b/app/Bookings/Index.alder
@@ -1,9 +1,13 @@
 module Bookings.Index
 
+import Booking
+
 page "/bookings" { } requires user
 
 type Model = { }
 
+type Row = { id: Booking.BookingId, room: Text, from: Date }
+
 load : Params -> Session -> Query Model
 load params session = ?load
 
app/Bookings/Index.alder:12:23: warning[W0002]: hole ?load has type Query Bookings.Index.Model
  load params session = ?load
                        ^^^^^
app/Bookings/Index.alder:15:22: warning[W0002]: hole ?view has type Html Bookings.Index.Msg
  view model session = ?view
                       ^^^^^
[exit 0]
$ alder new type Bookings.Index "Lookup = { room: Room.RoomId, booking: Booking.BookingId, user: User.UserId }"
edited app/Bookings/Index.alder
--- a/app/Bookings/Index.alder
+++ b/app/Bookings/Index.alder
@@ -1,6 +1,8 @@
 module Bookings.Index
 
 import Booking
+import Room
+import User
 
 page "/bookings" { } requires user
 
@@ -8,6 +10,8 @@
 
 type Row = { id: Booking.BookingId, room: Text, from: Date }
 
+type Lookup = { room: Room.RoomId, booking: Booking.BookingId, user: User.UserId }
+
 load : Params -> Session -> Query Model
 load params session = ?load
 
app/Bookings/Index.alder:16:23: warning[W0002]: hole ?load has type Query Bookings.Index.Model
  load params session = ?load
                        ^^^^^
app/Bookings/Index.alder:19:22: warning[W0002]: hole ?view has type Html Bookings.Index.Msg
  view model session = ?view
                       ^^^^^
[exit 0]
$ alder fill --decl Bookings.Index.Model - <<'EOF'
{ items: List Row }
EOF
edited app/Bookings/Index.alder
--- a/app/Bookings/Index.alder
+++ b/app/Bookings/Index.alder
@@ -6,7 +6,7 @@
 
 page "/bookings" { } requires user
 
-type Model = { }
+type Model = { items: List Row }
 
 type Row = { id: Booking.BookingId, room: Text, from: Date }
 
app/Bookings/Index.alder:16:23: warning[W0002]: hole ?load has type Query Bookings.Index.Model
  load params session = ?load
                        ^^^^^
app/Bookings/Index.alder:19:22: warning[W0002]: hole ?view has type Html Bookings.Index.Msg
  view model session = ?view
                       ^^^^^
[exit 0]
$ alder new decl Bookings.Index withRoom "Booking -> Query Row"
edited app/Bookings/Index.alder
--- a/app/Bookings/Index.alder
+++ b/app/Bookings/Index.alder
@@ -18,6 +18,9 @@
 view : Model -> Session -> Html Msg
 view model session = ?view
 
+withRoom : Booking -> Query Row
+withRoom = ?withRoom
+
 test "a visitor is sent to log in" = {
   let r <- Test.open (Bookings.Index {})
   Task.succeed (r.answer == Redirected (Auth.Login {}))
app/Bookings/Index.alder:16:23: warning[W0002]: hole ?load has type Query Bookings.Index.Model
  load params session = ?load
                        ^^^^^
app/Bookings/Index.alder:19:22: warning[W0002]: hole ?view has type Html Bookings.Index.Msg
  view model session = ?view
                       ^^^^^
app/Bookings/Index.alder:22:12: warning[W0002]: hole ?withRoom has type Booking -> Query Bookings.Index.Row
  withRoom = ?withRoom
             ^^^^^^^^^
[exit 0]
$ alder fill Bookings.Index.withRoom - <<'EOF'
\b -> {
  let found <- Db.get Room b.room
  Query.succeed { id: b.id, room: Maybe.withDefault "?" (Maybe.map (\r -> Text.trim r.name) found), from: b.from }
}
EOF
edited app/Bookings/Index.alder
--- a/app/Bookings/Index.alder
+++ b/app/Bookings/Index.alder
@@ -19,7 +19,10 @@
 view model session = ?view
 
 withRoom : Booking -> Query Row
-withRoom = ?withRoom
+withRoom = \b -> {
+  let found <- Db.get Room b.room
+  Query.succeed { id: b.id, room: Maybe.withDefault "?" (Maybe.map (\r -> Text.trim r.name) found), from: b.from }
+}
 
 test "a visitor is sent to log in" = {
   let r <- Test.open (Bookings.Index {})
app/Bookings/Index.alder:16:23: warning[W0002]: hole ?load has type Query Bookings.Index.Model
  load params session = ?load
                        ^^^^^
app/Bookings/Index.alder:19:22: warning[W0002]: hole ?view has type Html Bookings.Index.Msg
  view model session = ?view
                       ^^^^^
[exit 0]
$ alder fill Bookings.Index.load - --hole ?load <<'EOF'
{
  let mine <- Db.mine Booking |> Db.orderBy Booking.from.asc |> Db.all
  let rows <- Query.all (List.map withRoom mine)
  Query.succeed { items: rows }
}
EOF
edited app/Bookings/Index.alder
--- a/app/Bookings/Index.alder
+++ b/app/Bookings/Index.alder
@@ -13,7 +13,11 @@
 type Lookup = { room: Room.RoomId, booking: Booking.BookingId, user: User.UserId }
 
 load : Params -> Session -> Query Model
-load params session = ?load
+load params session = {
+  let mine <- Db.mine Booking |> Db.orderBy Booking.from.asc |> Db.all
+  let rows <- Query.all (List.map withRoom mine)
+  Query.succeed { items: rows }
+}
 
 view : Model -> Session -> Html Msg
 view model session = ?view
app/Bookings/Index.alder:23:22: warning[W0002]: hole ?view has type Html Bookings.Index.Msg
  view model session = ?view
                       ^^^^^
[exit 0]
$ alder fill Bookings.Index.view - --hole ?view <<'EOF'
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
edited app/Bookings/Index.alder
--- a/app/Bookings/Index.alder
+++ b/app/Bookings/Index.alder
@@ -1,6 +1,7 @@
 module Bookings.Index
 
 import Booking
+import Layout
 import Room
 import User
 
@@ -20,7 +21,17 @@
 }
 
 view : Model -> Session -> Html Msg
-view model session = ?view
+view model session = <Layout.Page title="My bookings">
+  <h1>My bookings</h1>
+  <ul id="mine">
+    {for b in model.items ->
+      <li>
+        <a href={Bookings.Show { id: b.id }}><span class="room">{b.room}</span></a>
+        <span class="from">{Date.toText b.from}</span>
+      </li>}
+  </ul>
+  <a href={Rooms.Index {}}>All rooms</a>
+</Layout.Page>
 
 withRoom : Booking -> Query Row
 withRoom = \b -> {
added import Layout to app/Bookings/Index.alder
[exit 0]
$ alder new type Rooms.Show "Lookup = { room: Room.RoomId, booking: Booking.BookingId, user: User.UserId }"
edited app/Rooms/Show.alder
--- a/app/Rooms/Show.alder
+++ b/app/Rooms/Show.alder
@@ -1,11 +1,15 @@
 module Rooms.Show
 
+import Booking
 import Room
+import User
 
 page "/rooms/{id}" { id: Room.RoomId } requires user
 
 type Model = { }
 
+type Lookup = { room: Room.RoomId, booking: Booking.BookingId, user: User.UserId }
+
 load : Params -> Session -> Query Model
 load params session = ?load
 
app/Rooms/Show.alder:14:23: warning[W0002]: hole ?load has type Query Rooms.Show.Model
  load params session = ?load
                        ^^^^^
app/Rooms/Show.alder:17:22: warning[W0002]: hole ?view has type Html Rooms.Show.Msg
  view model session = ?view
                       ^^^^^
[exit 0]
$ alder fill --decl Rooms.Show.Model - <<'EOF'
{ room: Room, bookings: List Booking }
EOF
edited app/Rooms/Show.alder
--- a/app/Rooms/Show.alder
+++ b/app/Rooms/Show.alder
@@ -6,7 +6,7 @@
 
 page "/rooms/{id}" { id: Room.RoomId } requires user
 
-type Model = { }
+type Model = { room: Room, bookings: List Booking }
 
 type Lookup = { room: Room.RoomId, booking: Booking.BookingId, user: User.UserId }
 
app/Rooms/Show.alder:14:23: warning[W0002]: hole ?load has type Query Rooms.Show.Model
  load params session = ?load
                        ^^^^^
app/Rooms/Show.alder:17:22: warning[W0002]: hole ?view has type Html Rooms.Show.Msg
  view model session = ?view
                       ^^^^^
[exit 0]
$ alder fill Rooms.Show.load - --hole ?load <<'EOF'
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
edited app/Rooms/Show.alder
--- a/app/Rooms/Show.alder
+++ b/app/Rooms/Show.alder
@@ -11,7 +11,16 @@
 type Lookup = { room: Room.RoomId, booking: Booking.BookingId, user: User.UserId }
 
 load : Params -> Session -> Query Model
-load params session = ?load
+load params session = {
+  let found <- Db.get Room params.id
+  match found {
+    Nothing -> Query.notFound,
+    Just room -> {
+      let bookings <- Db.from Booking |> Db.where (Booking.room.eq params.id) |> Db.orderBy Booking.from.asc |> Db.all
+      Query.succeed { room: room, bookings: bookings }
+    },
+  }
+}
 
 view : Model -> Session -> Html Msg
 view model session = ?view
app/Rooms/Show.alder:26:22: warning[W0002]: hole ?view has type Html Rooms.Show.Msg
  view model session = ?view
                       ^^^^^
[exit 0]
$ alder fill Rooms.Show.view - --hole ?view <<'EOF'
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
edited app/Rooms/Show.alder
--- a/app/Rooms/Show.alder
+++ b/app/Rooms/Show.alder
@@ -1,6 +1,7 @@
 module Rooms.Show
 
 import Booking
+import Layout
 import Room
 import User
 
@@ -23,7 +24,17 @@
 }
 
 view : Model -> Session -> Html Msg
-view model session = ?view
+view model session = <Layout.Page title={model.room.name}>
+  <h1 id="name">{model.room.name}</h1>
+  <ul id="bookings">
+    {for b in model.bookings ->
+      <li>
+        <span class="from">{Date.toText b.from}</span>
+        <span class="to">{Date.toText b.to}</span>
+      </li>}
+  </ul>
+  <a href={Rooms.Index {}}>All rooms</a>
+</Layout.Page>
 
 test "a visitor is sent to log in" = {
   let room1 <- Db.insert Room { name: Text.NonEmpty "name-1" }
added import Layout to app/Rooms/Show.alder
[exit 0]
$ alder new form Rooms.Show NewBooking from:Date to:Date --msg Book
edited app/Rooms/Show.alder
--- a/app/Rooms/Show.alder
+++ b/app/Rooms/Show.alder
@@ -23,6 +23,18 @@
   }
 }
 
+form NewBooking = {
+  from: Date,
+  to: Date,
+}
+
+msg Book form NewBooking
+
+update : Msg -> Model -> Session -> Task Nav
+update msg model session = match msg {
+  Book p -> ?book,
+}
+
 view : Model -> Session -> Html Msg
 view model session = <Layout.Page title={model.room.name}>
   <h1 id="name">{model.room.name}</h1>
@@ -34,6 +46,16 @@
       </li>}
   </ul>
   <a href={Rooms.Index {}}>All rooms</a>
+
+  <al-card>
+    <h2 slot="header">New booking</h2>
+    <form submit={Book}>
+      <al-input field="from" label="From" />
+      <al-input field="to" label="To" />
+      <al-alert variant="destructive"><Errors form /></al-alert>
+      <al-button type="submit">New booking</al-button>
+    </form>
+  </al-card>
 </Layout.Page>
 
 test "a visitor is sent to log in" = {
@@ -41,3 +63,22 @@
   let r <- Test.open (Rooms.Show { id: room1 })
   Task.succeed (r.answer == Redirected (Auth.Login {}))
 }
+
+test "submitting NewBooking" = {
+  let today <- Date.today
+  let me <- Db.insert User { email: Text.Email "email-1@example.com", name: Text.NonEmpty "name-1", passwordHash: "passwordhash-1" }
+  let _ <- Test.login { user: Just me }
+  let room1 <- Db.insert Room { name: Text.NonEmpty "name-1" }
+  let _ <- Test.get (Rooms.Show { id: room1 })
+  let r <- Test.submit NewBooking { from: today, to: today }
+  Task.succeed (r.answer == ?expected)
+}
+
+test "NewBooking refuses an empty from" = {
+  let me <- Db.insert User { email: Text.Email "email-1@example.com", name: Text.NonEmpty "name-1", passwordHash: "passwordhash-1" }
+  let _ <- Test.login { user: Just me }
+  let room1 <- Db.insert Room { name: Text.NonEmpty "name-1" }
+  let _ <- Test.get (Rooms.Show { id: room1 })
+  let r <- Test.submitRaw NewBooking [{ name: "from", value: "" }, { name: "to", value: "2026-01-01" }]
+  Task.succeed (r.answer == FormRejected [{ field: "from", message: "is required" }])
+}
app/Rooms/Show.alder:35:13: warning[W0002]: hole ?book has type Task Nav
    Book p -> ?book,
              ^^^^^
app/Rooms/Show.alder:74:29: warning[W0002]: hole ?expected has type Answer
    Task.succeed (r.answer == ?expected)
                              ^^^^^^^^^
[exit 0]
$ alder fill --decl Rooms.Show.view - <<'EOF'
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
edited app/Rooms/Show.alder
--- a/app/Rooms/Show.alder
+++ b/app/Rooms/Show.alder
@@ -36,27 +36,26 @@
 }
 
 view : Model -> Session -> Html Msg
-view model session = <Layout.Page title={model.room.name}>
-  <h1 id="name">{model.room.name}</h1>
-  <ul id="bookings">
-    {for b in model.bookings ->
-      <li>
-        <span class="from">{Date.toText b.from}</span>
-        <span class="to">{Date.toText b.to}</span>
-      </li>}
-  </ul>
-  <a href={Rooms.Index {}}>All rooms</a>
-
-  <al-card>
-    <h2 slot="header">New booking</h2>
-    <form submit={Book}>
-      <al-input field="from" label="From" />
-      <al-input field="to" label="To" />
-      <al-alert variant="destructive"><Errors form /></al-alert>
-      <al-button type="submit">New booking</al-button>
-    </form>
-  </al-card>
-</Layout.Page>
+view model session =
+  <Layout.Page title={model.room.name}>
+    <h1 id="name">{model.room.name}</h1>
+    <ul id="bookings">
+      {for b in model.bookings ->
+        <li>
+          <span class="from">{Date.toText b.from}</span>
+          <span class="to">{Date.toText b.to}</span>
+        </li>}
+    </ul>
+    <al-card>
+      <form submit={Book}>
+        <Errors form />
+        <al-input field="from" label="From" />
+        <al-input field="to" label="To" />
+        <al-button type="submit">Book</al-button>
+      </form>
+    </al-card>
+    <a href={Rooms.Index {}}>All rooms</a>
+  </Layout.Page>
 
 test "a visitor is sent to log in" = {
   let room1 <- Db.insert Room { name: Text.NonEmpty "name-1" }
app/Rooms/Show.alder:35:13: warning[W0002]: hole ?book has type Task Nav
    Book p -> ?book,
              ^^^^^
app/Rooms/Show.alder:73:29: warning[W0002]: hole ?expected has type Answer
    Task.succeed (r.answer == ?expected)
                              ^^^^^^^^^
[exit 0]
$ alder fill --decl Rooms.Show.update - <<'EOF'
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
edited app/Rooms/Show.alder
--- a/app/Rooms/Show.alder
+++ b/app/Rooms/Show.alder
@@ -32,7 +32,14 @@
 
 update : Msg -> Model -> Session -> Task Nav
 update msg model session = match msg {
-  Book p -> ?book,
+  Book p -> {
+    let existing <- Db.from Booking |> Db.where (Booking.room.eq model.room.id) |> Db.all
+    let clash = List.any (Booking.overlaps p.form.from p.form.to) existing
+    if p.form.to <= p.form.from || clash then Task.succeed (Reject "Those dates cannot be booked.") else {
+      let _ <- Db.insert Booking { room: model.room.id, owner: session.user, from: p.form.from, to: p.form.to }
+      Task.succeed (Redirect (Rooms.Show { id: model.room.id }))
+    }
+  },
 }
 
 view : Model -> Session -> Html Msg
app/Rooms/Show.alder:80:29: warning[W0002]: hole ?expected has type Answer
    Task.succeed (r.answer == ?expected)
                              ^^^^^^^^^
[exit 0]
$ alder new test Rooms.Show "an overlapping booking is refused and not stored"
edited app/Rooms/Show.alder
--- a/app/Rooms/Show.alder
+++ b/app/Rooms/Show.alder
@@ -88,3 +88,7 @@
   let r <- Test.submitRaw NewBooking [{ name: "from", value: "" }, { name: "to", value: "2026-01-01" }]
   Task.succeed (r.answer == FormRejected [{ field: "from", message: "is required" }])
 }
+
+test "an overlapping booking is refused and not stored" = {
+  ?test
+}
app/Rooms/Show.alder:80:29: warning[W0002]: hole ?expected has type Answer
    Task.succeed (r.answer == ?expected)
                              ^^^^^^^^^
app/Rooms/Show.alder:93:3: warning[W0002]: hole ?test has type Task Bool
    ?test
    ^^^^^
[exit 0]
$ alder fill 'Rooms.Show."an overlapping booking is refused and not stored"' - <<'EOF'
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
edited app/Rooms/Show.alder
--- a/app/Rooms/Show.alder
+++ b/app/Rooms/Show.alder
@@ -90,5 +90,13 @@
 }
 
 test "an overlapping booking is refused and not stored" = {
-  ?test
+  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
+  let room <- Db.insert Room { name: Text.NonEmpty "Oak" }
+  let today <- Date.today
+  let _ <- Db.insert Booking { room: room, owner: me, from: Date.addDays 1 today, to: Date.addDays 3 today }
+  let _ <- Test.login { user: Just me }
+  let _ <- Test.get (Rooms.Show { id: room })
+  let r <- Test.submit NewBooking { from: Date.addDays 2 today, to: Date.addDays 4 today }
+  let n <- Db.from Booking |> Db.where (Booking.room.eq room) |> Db.count
+  Task.succeed ({ answer: r.answer, count: n } == { answer: FormRejected [{ field: "", message: "Those dates cannot be booked." }], count: 1 })
 }
app/Rooms/Show.alder:80:29: warning[W0002]: hole ?expected has type Answer
    Task.succeed (r.answer == ?expected)
                              ^^^^^^^^^
[exit 0]
$ alder new test Rooms.Show "a booking starting on the other end date is stored"
edited app/Rooms/Show.alder
--- a/app/Rooms/Show.alder
+++ b/app/Rooms/Show.alder
@@ -100,3 +100,7 @@
   let n <- Db.from Booking |> Db.where (Booking.room.eq room) |> Db.count
   Task.succeed ({ answer: r.answer, count: n } == { answer: FormRejected [{ field: "", message: "Those dates cannot be booked." }], count: 1 })
 }
+
+test "a booking starting on the other end date is stored" = {
+  ?test
+}
app/Rooms/Show.alder:80:29: warning[W0002]: hole ?expected has type Answer
    Task.succeed (r.answer == ?expected)
                              ^^^^^^^^^
app/Rooms/Show.alder:105:3: warning[W0002]: hole ?test has type Task Bool
    ?test
    ^^^^^
[exit 0]
$ alder fill 'Rooms.Show."a booking starting on the other end date is stored"' - <<'EOF'
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
edited app/Rooms/Show.alder
--- a/app/Rooms/Show.alder
+++ b/app/Rooms/Show.alder
@@ -102,5 +102,13 @@
 }
 
 test "a booking starting on the other end date is stored" = {
-  ?test
+  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
+  let room <- Db.insert Room { name: Text.NonEmpty "Oak" }
+  let today <- Date.today
+  let _ <- Db.insert Booking { room: room, owner: me, from: Date.addDays 1 today, to: Date.addDays 3 today }
+  let _ <- Test.login { user: Just me }
+  let _ <- Test.get (Rooms.Show { id: room })
+  let r <- Test.submit NewBooking { from: Date.addDays 3 today, to: Date.addDays 5 today }
+  let n <- Db.from Booking |> Db.where (Booking.room.eq room) |> Db.count
+  Task.succeed ({ answer: r.answer, count: n } == { answer: Redirected (Rooms.Show { id: room }), count: 2 })
 }
app/Rooms/Show.alder:80:29: warning[W0002]: hole ?expected has type Answer
    Task.succeed (r.answer == ?expected)
                              ^^^^^^^^^
[exit 0]
$ alder new type Bookings.Show "Lookup = { room: Room.RoomId, booking: Booking.BookingId, user: User.UserId }"
edited app/Bookings/Show.alder
--- a/app/Bookings/Show.alder
+++ b/app/Bookings/Show.alder
@@ -8,6 +8,8 @@
 
 type Model = { }
 
+type Lookup = { room: Room.RoomId, booking: Booking.BookingId, user: User.UserId }
+
 load : Params -> Session -> Query Model
 load params session = ?load
 
app/Bookings/Show.alder:14:23: warning[W0002]: hole ?load has type Query Bookings.Show.Model
  load params session = ?load
                        ^^^^^
app/Bookings/Show.alder:17:22: warning[W0002]: hole ?view has type Html Bookings.Show.Msg
  view model session = ?view
                       ^^^^^
[exit 0]
$ alder fill --decl Bookings.Show.Model - <<'EOF'
{ booking: Booking, room: Room, cancellable: Bool }
EOF
edited app/Bookings/Show.alder
--- a/app/Bookings/Show.alder
+++ b/app/Bookings/Show.alder
@@ -6,7 +6,7 @@
 
 page "/bookings/{id}" { id: Booking.BookingId } requires user
 
-type Model = { }
+type Model = { booking: Booking, room: Room, cancellable: Bool }
 
 type Lookup = { room: Room.RoomId, booking: Booking.BookingId, user: User.UserId }
 
app/Bookings/Show.alder:14:23: warning[W0002]: hole ?load has type Query Bookings.Show.Model
  load params session = ?load
                        ^^^^^
app/Bookings/Show.alder:17:22: warning[W0002]: hole ?view has type Html Bookings.Show.Msg
  view model session = ?view
                       ^^^^^
[exit 0]
$ alder fill Bookings.Show.load - --hole ?load <<'EOF'
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
edited app/Bookings/Show.alder
--- a/app/Bookings/Show.alder
+++ b/app/Bookings/Show.alder
@@ -11,7 +11,15 @@
 type Lookup = { room: Room.RoomId, booking: Booking.BookingId, user: User.UserId }
 
 load : Params -> Session -> Query Model
-load params session = ?load
+load params session = {
+  let booking <- Db.getMine Booking params.id
+  let found <- Db.get Room booking.room
+  let today <- Date.today
+  match found {
+    Nothing -> Query.notFound,
+    Just room -> Query.succeed { booking: booking, room: room, cancellable: today < booking.from },
+  }
+}
 
 view : Model -> Session -> Html Msg
 view model session = ?view
app/Bookings/Show.alder:25:22: warning[W0002]: hole ?view has type Html Bookings.Show.Msg
  view model session = ?view
                       ^^^^^
[exit 0]
$ alder fill Bookings.Show.view - --hole ?view <<'EOF'
<Layout.Page title="Booking">
  <h1>Booking</h1>
  <p id="room">{model.room.name}</p>
  <p id="from">{Date.toText model.booking.from}</p>
  <p id="to">{Date.toText model.booking.to}</p>
  <a href={Bookings.Index {}}>My bookings</a>
</Layout.Page>
EOF
edited app/Bookings/Show.alder
--- a/app/Bookings/Show.alder
+++ b/app/Bookings/Show.alder
@@ -1,6 +1,7 @@
 module Bookings.Show
 
 import Booking
+import Layout
 import Room
 import User
 
@@ -22,7 +23,13 @@
 }
 
 view : Model -> Session -> Html Msg
-view model session = ?view
+view model session = <Layout.Page title="Booking">
+  <h1>Booking</h1>
+  <p id="room">{model.room.name}</p>
+  <p id="from">{Date.toText model.booking.from}</p>
+  <p id="to">{Date.toText model.booking.to}</p>
+  <a href={Bookings.Index {}}>My bookings</a>
+</Layout.Page>
 
 test "a visitor is sent to log in" = {
   let today <- Date.today
added import Layout to app/Bookings/Show.alder
[exit 0]
$ alder new msg Bookings.Show Cancel
edited app/Bookings/Show.alder
--- a/app/Bookings/Show.alder
+++ b/app/Bookings/Show.alder
@@ -22,6 +22,13 @@
   }
 }
 
+msg Cancel
+
+update : Msg -> Model -> Session -> Task Nav
+update msg model session = match msg {
+  Cancel p -> ?cancel,
+}
+
 view : Model -> Session -> Html Msg
 view model session = <Layout.Page title="Booking">
   <h1>Booking</h1>
@@ -29,6 +36,8 @@
   <p id="from">{Date.toText model.booking.from}</p>
   <p id="to">{Date.toText model.booking.to}</p>
   <a href={Bookings.Index {}}>My bookings</a>
+
+  <al-button click={Cancel}>Cancel</al-button>
 </Layout.Page>
 
 test "a visitor is sent to log in" = {
@@ -39,3 +48,20 @@
   let r <- Test.open (Bookings.Show { id: booking1 })
   Task.succeed (r.answer == Redirected (Auth.Login {}))
 }
+
+-- Fill ?expected with what pressing Cancel must do, so that a test fails when the arm
+-- does something else: its answer (`r.answer`: Rerendered, Redirected …) and what it
+-- changed (`after`: the page opened again, `after.answer` Shown, or PageMissing when the arm
+-- deleted the page's row, and `after.doc` its HTML), as
+-- `{ answer: r.answer, … } == { answer: Rerendered, … }`.
+test "pressing Cancel" = {
+  let today <- Date.today
+  let me <- Db.insert User { email: Text.Email "email-1@example.com", name: Text.NonEmpty "name-1", passwordHash: "passwordhash-1" }
+  let _ <- Test.login { user: Just me }
+  let room1 <- Db.insert Room { name: Text.NonEmpty "name-1" }
+  let booking1 <- Db.insert Booking { room: room1, owner: me, from: today, to: today }
+  let _ <- Test.get (Bookings.Show { id: booking1 })
+  let r <- Test.click Cancel
+  let after <- Test.open (Bookings.Show { id: booking1 })
+  Task.succeed ?expected
+}
app/Bookings/Show.alder:29:15: warning[W0002]: hole ?cancel has type Task Nav
    Cancel p -> ?cancel,
                ^^^^^^^
app/Bookings/Show.alder:66:16: warning[W0002]: hole ?expected has type Bool
    Task.succeed ?expected
                 ^^^^^^^^^
[exit 0]
$ alder fill --decl Bookings.Show.view - <<'EOF'
<Layout.Page title="Booking">
  <h1>Booking</h1>
  <p id="room">{model.room.name}</p>
  <p id="from">{Date.toText model.booking.from}</p>
  <p id="to">{Date.toText model.booking.to}</p>
  {if model.cancellable then <button id="cancel" click={Cancel}>Cancel booking</button> else Html.none}
  <a href={Bookings.Index {}}>My bookings</a>
</Layout.Page>
EOF
edited app/Bookings/Show.alder
--- a/app/Bookings/Show.alder
+++ b/app/Bookings/Show.alder
@@ -30,15 +30,15 @@
 }
 
 view : Model -> Session -> Html Msg
-view model session = <Layout.Page title="Booking">
-  <h1>Booking</h1>
-  <p id="room">{model.room.name}</p>
-  <p id="from">{Date.toText model.booking.from}</p>
-  <p id="to">{Date.toText model.booking.to}</p>
-  <a href={Bookings.Index {}}>My bookings</a>
-
-  <al-button click={Cancel}>Cancel</al-button>
-</Layout.Page>
+view model session =
+  <Layout.Page title="Booking">
+    <h1>Booking</h1>
+    <p id="room">{model.room.name}</p>
+    <p id="from">{Date.toText model.booking.from}</p>
+    <p id="to">{Date.toText model.booking.to}</p>
+    {if model.cancellable then <button id="cancel" click={Cancel}>Cancel booking</button> else Html.none}
+    <a href={Bookings.Index {}}>My bookings</a>
+  </Layout.Page>
 
 test "a visitor is sent to log in" = {
   let today <- Date.today
app/Bookings/Show.alder:29:15: warning[W0002]: hole ?cancel has type Task Nav
    Cancel p -> ?cancel,
                ^^^^^^^
app/Bookings/Show.alder:66:16: warning[W0002]: hole ?expected has type Bool
    Task.succeed ?expected
                 ^^^^^^^^^
[exit 0]
$ alder fill --decl Bookings.Show.update - <<'EOF'
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
edited app/Bookings/Show.alder
--- a/app/Bookings/Show.alder
+++ b/app/Bookings/Show.alder
@@ -26,7 +26,13 @@
 
 update : Msg -> Model -> Session -> Task Nav
 update msg model session = match msg {
-  Cancel p -> ?cancel,
+  Cancel p -> {
+    let today <- Date.today
+    if today < model.booking.from then {
+      let _ <- Db.deleteMine Booking model.booking.id
+      Task.succeed (Redirect (Bookings.Index {}))
+    } else Task.fail "a booking cannot be cancelled from its first day"
+  },
 }
 
 view : Model -> Session -> Html Msg
app/Bookings/Show.alder:72:16: warning[W0002]: hole ?expected has type Bool
    Task.succeed ?expected
                 ^^^^^^^^^
[exit 0]
$ alder new test Bookings.Show "the owner cancels a booking before its first day"
edited app/Bookings/Show.alder
--- a/app/Bookings/Show.alder
+++ b/app/Bookings/Show.alder
@@ -71,3 +71,7 @@
   let after <- Test.open (Bookings.Show { id: booking1 })
   Task.succeed ?expected
 }
+
+test "the owner cancels a booking before its first day" = {
+  ?test
+}
app/Bookings/Show.alder:72:16: warning[W0002]: hole ?expected has type Bool
    Task.succeed ?expected
                 ^^^^^^^^^
app/Bookings/Show.alder:76:3: warning[W0002]: hole ?test has type Task Bool
    ?test
    ^^^^^
[exit 0]
$ alder fill 'Bookings.Show."the owner cancels a booking before its first day"' - <<'EOF'
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
edited app/Bookings/Show.alder
--- a/app/Bookings/Show.alder
+++ b/app/Bookings/Show.alder
@@ -73,5 +73,13 @@
 }
 
 test "the owner cancels a booking before its first day" = {
-  ?test
+  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
+  let room <- Db.insert Room { name: Text.NonEmpty "Oak" }
+  let today <- Date.today
+  let id <- Db.insert Booking { room: room, owner: me, from: Date.addDays 2 today, to: Date.addDays 4 today }
+  let _ <- Test.login { user: Just me }
+  let _ <- Test.get (Bookings.Show { id: id })
+  let r <- Test.click Cancel
+  let left <- Db.from Booking |> Db.count
+  Task.succeed ({ answer: r.answer, left: left } == { answer: Redirected (Bookings.Index {}), left: 0 })
 }
app/Bookings/Show.alder:72:16: warning[W0002]: hole ?expected has type Bool
    Task.succeed ?expected
                 ^^^^^^^^^
[exit 0]
$ alder new test Bookings.Show "a booking from today cannot be cancelled"
edited app/Bookings/Show.alder
--- a/app/Bookings/Show.alder
+++ b/app/Bookings/Show.alder
@@ -83,3 +83,7 @@
   let left <- Db.from Booking |> Db.count
   Task.succeed ({ answer: r.answer, left: left } == { answer: Redirected (Bookings.Index {}), left: 0 })
 }
+
+test "a booking from today cannot be cancelled" = {
+  ?test
+}
app/Bookings/Show.alder:72:16: warning[W0002]: hole ?expected has type Bool
    Task.succeed ?expected
                 ^^^^^^^^^
app/Bookings/Show.alder:88:3: warning[W0002]: hole ?test has type Task Bool
    ?test
    ^^^^^
[exit 0]
$ alder fill 'Bookings.Show."a booking from today cannot be cancelled"' - <<'EOF'
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
edited app/Bookings/Show.alder
--- a/app/Bookings/Show.alder
+++ b/app/Bookings/Show.alder
@@ -85,5 +85,13 @@
 }
 
 test "a booking from today cannot be cancelled" = {
-  ?test
+  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
+  let room <- Db.insert Room { name: Text.NonEmpty "Oak" }
+  let today <- Date.today
+  let id <- Db.insert Booking { room: room, owner: me, from: today, to: Date.addDays 2 today }
+  let _ <- Test.login { user: Just me }
+  let page <- Test.get (Bookings.Show { id: id })
+  let r <- Test.run (update Cancel page.model { user: me })
+  let left <- Db.from Booking |> Db.count
+  Task.succeed ({ shown: Test.has "#cancel" page.doc, value: r.value, left: left } == { shown: False, value: Err "a booking cannot be cancelled from its first day", left: 1 })
 }
app/Bookings/Show.alder:72:16: warning[W0002]: hole ?expected has type Bool
    Task.succeed ?expected
                 ^^^^^^^^^
[exit 0]
```

The program type-checks but `alder build` refused it; fix the cause below.

```text
app/Bookings/Show.alder:72:16: warning[W0002]: hole ?expected has type Bool
    Task.succeed ?expected
                 ^^^^^^^^^
app/Rooms/Show.alder:80:29: warning[W0002]: hole ?expected has type Answer
    Task.succeed (r.answer == ?expected)
                              ^^^^^^^^^
alder build: the program has 2 typed hole(s); fill them before building
```
