### Round 0

```text
$ alder new table User email:Text.Email@unique name:Text.NonEmpty passwordHash:Text created:Time@created --no-timestamps
wrote app/User.alder
wrote migrations/0001_initial.sql
  create table User
wrote migrations/schema.json
[exit 0]
$ alder new table Room name:Text.NonEmpty --no-timestamps
wrote app/Room.alder
wrote migrations/0002_create_room.sql
  create table Room
edited migrations/schema.json
[exit 0]
$ alder new table Booking room:Room.RoomId@ref=Room owner:User.UserId@ref=User from:Date to:Date --no-timestamps
wrote app/Booking.alder
wrote migrations/0003_create_booking.sql
  create table Booking
  add index on Booking.owner
  add index on Booking.room
edited migrations/schema.json
[exit 0]
$ alder new auth --users User
wrote adapters/auth/login_password.go
wrote adapters/auth/login_password_test.go
edited app/App.alder
wrote app/Auth/Login.alder
--- a/app/App.alder
+++ b/app/App.alder
@@ -1,9 +1,22 @@
 module App
 
 import Layout
+import User
 
-session { }
+session { user: Maybe User.UserId }
 
+login = Auth.Login
+
+msg Logout
+
+update : Msg -> Session -> Task Nav
+update msg session = match msg {
+  Logout p -> {
+    let _ <- Session.put (\s -> { s | user: Nothing })
+    Task.succeed (Redirect (Auth.Login {}))
+  },
+}
+
 error : Error -> Html Never
 error err =
   <Layout.Page title="Error">
@@ -16,3 +29,10 @@
   <Layout.Page title="Not found">
     <h1>Not found</h1>
   </Layout.Page>
+
+-- Every Log out button is on a page that requires a user, so a browser test cannot tell this
+-- Redirect from a Rerender (book 10, "The logout redirect"): the test calls update itself.
+test "Logout clears the user and goes to log in" = {
+  let r <- Test.run (update Logout { user: Just (User.UserId "me") })
+  Task.succeed ({ value: r.value, trace: r.trace } == { value: Ok (Redirect (Auth.Login {})), trace: ["Session.put"] })
+}
app/Auth/Login.alder:47:33: warning[W0002]: hole ?afterLogin has type Page
          Task.succeed (Redirect (?afterLogin))
                                  ^^^^^^^^^^^
app/Auth/Login.alder:86:85: warning[W0002]: hole ?afterLogin has type Page
    Task.succeed ({ answer: r.answer, user: r.session.user } == { answer: Redirected (?afterLogin), user: Just me })
                                                                                      ^^^^^^^^^^^
[exit 0]
$ alder new decl Booking overlaps "Date -> Date -> Booking -> Bool"
edited app/Booking.alder
--- a/app/Booking.alder
+++ b/app/Booking.alder
@@ -13,6 +13,9 @@
   to: Date,
 }
 
+overlaps : Date -> Date -> Booking -> Bool
+overlaps = ?overlaps
+
 test "a Booking keeps its room from being deleted" = {
   let today <- Date.today
   let target <- Db.insert Room { name: Text.NonEmpty "name-1" }
app/Booking.alder:17:12: warning[W0002]: hole ?overlaps has type Date -> Date -> Booking -> Bool
  overlaps = ?overlaps
             ^^^^^^^^^
[exit 0]
$ alder fill Booking.overlaps - <<'EOF'
\from to b -> from < b.to && b.from < to
EOF
edited app/Booking.alder
--- a/app/Booking.alder
+++ b/app/Booking.alder
@@ -14,7 +14,7 @@
 }
 
 overlaps : Date -> Date -> Booking -> Bool
-overlaps = ?overlaps
+overlaps = \from to b -> from < b.to && b.from < to
 
 test "a Booking keeps its room from being deleted" = {
   let today <- Date.today
[exit 0]
$ alder new decl Booking canCancel "Date -> Date -> Bool"
edited app/Booking.alder
--- a/app/Booking.alder
+++ b/app/Booking.alder
@@ -16,6 +16,9 @@
 overlaps : Date -> Date -> Booking -> Bool
 overlaps = \from to b -> from < b.to && b.from < to
 
+canCancel : Date -> Date -> Bool
+canCancel = ?canCancel
+
 test "a Booking keeps its room from being deleted" = {
   let today <- Date.today
   let target <- Db.insert Room { name: Text.NonEmpty "name-1" }
app/Booking.alder:20:13: warning[W0002]: hole ?canCancel has type Date -> Date -> Bool
  canCancel = ?canCancel
              ^^^^^^^^^^
[exit 0]
$ alder fill Booking.canCancel - <<'EOF'
\today from -> today < from
EOF
edited app/Booking.alder
--- a/app/Booking.alder
+++ b/app/Booking.alder
@@ -17,7 +17,7 @@
 overlaps = \from to b -> from < b.to && b.from < to
 
 canCancel : Date -> Date -> Bool
-canCancel = ?canCancel
+canCancel = \today from -> today < from
 
 test "a Booking keeps its room from being deleted" = {
   let today <- Date.today
[exit 0]
$ alder new decl Booking nextFree "Date -> Room.RoomId -> List Booking -> Date"
edited app/Booking.alder
--- a/app/Booking.alder
+++ b/app/Booking.alder
@@ -19,6 +19,9 @@
 canCancel : Date -> Date -> Bool
 canCancel = \today from -> today < from
 
+nextFree : Date -> Room.RoomId -> List Booking -> Date
+nextFree = ?nextFree
+
 test "a Booking keeps its room from being deleted" = {
   let today <- Date.today
   let target <- Db.insert Room { name: Text.NonEmpty "name-1" }
app/Booking.alder:23:12: warning[W0002]: hole ?nextFree has type Date -> Room.RoomId -> List Booking -> Date
  nextFree = ?nextFree
             ^^^^^^^^^
[exit 0]
$ alder fill Booking.nextFree - <<'EOF'
\today room bookings -> bookings
  |> List.filter (\b -> b.room == room)
  |> List.sortByText (\b -> Date.toText b.from)
  |> List.foldl (\b d -> if b.from <= d && d < b.to then b.to else d) today
EOF
edited app/Booking.alder
--- a/app/Booking.alder
+++ b/app/Booking.alder
@@ -20,7 +20,10 @@
 canCancel = \today from -> today < from
 
 nextFree : Date -> Room.RoomId -> List Booking -> Date
-nextFree = ?nextFree
+nextFree = \today room bookings -> bookings
+  |> List.filter (\b -> b.room == room)
+  |> List.sortByText (\b -> Date.toText b.from)
+  |> List.foldl (\b d -> if b.from <= d && d < b.to then b.to else d) today
 
 test "a Booking keeps its room from being deleted" = {
   let today <- Date.today
[exit 0]
$ alder new page Rooms.Index /rooms --requires user --blank
wrote app/Rooms/Index.alder
app/Rooms/Index.alder:8:23: warning[W0002]: hole ?load has type Query Rooms.Index.Model
  load params session = ?load
                        ^^^^^
app/Rooms/Index.alder:11:22: warning[W0002]: hole ?view has type Html Rooms.Index.Msg
  view model session = ?view
                       ^^^^^
[exit 0]
$ alder new page Rooms.Show '/rooms/{id}' --param id:Room.RoomId --requires user --blank
wrote app/Rooms/Show.alder
app/Rooms/Show.alder:10:23: warning[W0002]: hole ?load has type Query Rooms.Show.Model
  load params session = ?load
                        ^^^^^
app/Rooms/Show.alder:13:22: warning[W0002]: hole ?view has type Html Rooms.Show.Msg
  view model session = ?view
                       ^^^^^
[exit 0]
$ alder new page Bookings.Index /bookings --requires user --blank
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
$ alder fill --decl Rooms.Index.Model - <<'EOF'
{ rows: List { id: Room.RoomId, name: Text.NonEmpty, free: Date } }
EOF
edited app/Rooms/Index.alder
--- a/app/Rooms/Index.alder
+++ b/app/Rooms/Index.alder
@@ -1,8 +1,10 @@
 module Rooms.Index
 
+import Room
+
 page "/rooms" { } requires user
 
-type Model = { }
+type Model = { rows: List { id: Room.RoomId, name: Text.NonEmpty, free: Date } }
 
 load : Params -> Session -> Query Model
 load params session = ?load
app/Rooms/Index.alder:10:23: warning[W0002]: hole ?load has type Query Rooms.Index.Model
  load params session = ?load
                        ^^^^^
app/Rooms/Index.alder:13:22: warning[W0002]: hole ?view has type Html Rooms.Index.Msg
  view model session = ?view
                       ^^^^^
added import Room to app/Rooms/Index.alder
[exit 0]
$ alder fill Rooms.Index.load - <<'EOF'
{
  let rooms <- Db.from Room |> Db.orderBy Room.name.asc |> Db.all
  let bookings <- Db.from Booking |> Db.all
  let today <- Date.today
  Query.succeed { rows: List.map (\r -> { id: r.id, name: r.name, free: Booking.nextFree today r.id bookings }) rooms }
}
EOF
edited app/Rooms/Index.alder
--- a/app/Rooms/Index.alder
+++ b/app/Rooms/Index.alder
@@ -1,5 +1,6 @@
 module Rooms.Index
 
+import Booking
 import Room
 
 page "/rooms" { } requires user
@@ -7,7 +8,12 @@
 type Model = { rows: List { id: Room.RoomId, name: Text.NonEmpty, free: Date } }
 
 load : Params -> Session -> Query Model
-load params session = ?load
+load params session = {
+  let rooms <- Db.from Room |> Db.orderBy Room.name.asc |> Db.all
+  let bookings <- Db.from Booking |> Db.all
+  let today <- Date.today
+  Query.succeed { rows: List.map (\r -> { id: r.id, name: r.name, free: Booking.nextFree today r.id bookings }) rooms }
+}
 
 view : Model -> Session -> Html Msg
 view model session = ?view
app/Rooms/Index.alder:19:22: warning[W0002]: hole ?view has type Html Rooms.Index.Msg
  view model session = ?view
                       ^^^^^
added import Booking to app/Rooms/Index.alder
[exit 0]
$ alder fill Rooms.Index.view - <<'EOF'
<Layout.Page title="Rooms">
  <h1>Rooms</h1>
  <ul id="rooms">
    {for row in model.rows ->
      <li>
        <a class="name" href={Rooms.Show { id: row.id }}>{row.name}</a>
        <span class="free">{Date.toText row.free}</span>
      </li>}
  </ul>
  <p><a href={Bookings.Index {}}>My bookings</a></p>
  <button click={App.Logout}>Log out</button>
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
@@ -16,7 +17,18 @@
 }
 
 view : Model -> Session -> Html Msg
-view model session = ?view
+view model session = <Layout.Page title="Rooms">
+  <h1>Rooms</h1>
+  <ul id="rooms">
+    {for row in model.rows ->
+      <li>
+        <a class="name" href={Rooms.Show { id: row.id }}>{row.name}</a>
+        <span class="free">{Date.toText row.free}</span>
+      </li>}
+  </ul>
+  <p><a href={Bookings.Index {}}>My bookings</a></p>
+  <button click={App.Logout}>Log out</button>
+</Layout.Page>
 
 test "a visitor is sent to log in" = {
   let r <- Test.open (Rooms.Index {})
added import Layout to app/Rooms/Index.alder
[exit 0]
$ alder fill --decl Rooms.Show.Model - <<'EOF'
{ id: Room.RoomId, name: Text.NonEmpty, bookings: List { from: Date, to: Date } }
EOF
edited app/Rooms/Show.alder
--- a/app/Rooms/Show.alder
+++ b/app/Rooms/Show.alder
@@ -4,7 +4,7 @@
 
 page "/rooms/{id}" { id: Room.RoomId } requires user
 
-type Model = { }
+type Model = { id: Room.RoomId, name: Text.NonEmpty, bookings: List { from: Date, to: Date } }
 
 load : Params -> Session -> Query Model
 load params session = ?load
app/Rooms/Show.alder:10:23: warning[W0002]: hole ?load has type Query Rooms.Show.Model
  load params session = ?load
                        ^^^^^
app/Rooms/Show.alder:13:22: warning[W0002]: hole ?view has type Html Rooms.Show.Msg
  view model session = ?view
                       ^^^^^
[exit 0]
$ alder fill Rooms.Show.load - <<'EOF'
{
  let found <- Db.get Room params.id
  match found {
    Just room -> {
      let bookings <- Db.from Booking |> Db.where (Booking.room.eq room.id) |> Db.orderBy Booking.from.asc |> Db.all
      Query.succeed { id: room.id, name: room.name, bookings: List.map (\b -> { from: b.from, to: b.to }) bookings }
    },
    Nothing -> Query.notFound,
  }
}
EOF
edited app/Rooms/Show.alder
--- a/app/Rooms/Show.alder
+++ b/app/Rooms/Show.alder
@@ -1,5 +1,6 @@
 module Rooms.Show
 
+import Booking
 import Room
 
 page "/rooms/{id}" { id: Room.RoomId } requires user
@@ -7,7 +8,16 @@
 type Model = { id: Room.RoomId, name: Text.NonEmpty, bookings: List { from: Date, to: Date } }
 
 load : Params -> Session -> Query Model
-load params session = ?load
+load params session = {
+  let found <- Db.get Room params.id
+  match found {
+    Just room -> {
+      let bookings <- Db.from Booking |> Db.where (Booking.room.eq room.id) |> Db.orderBy Booking.from.asc |> Db.all
+      Query.succeed { id: room.id, name: room.name, bookings: List.map (\b -> { from: b.from, to: b.to }) bookings }
+    },
+    Nothing -> Query.notFound,
+  }
+}
 
 view : Model -> Session -> Html Msg
 view model session = ?view
app/Rooms/Show.alder:23:22: warning[W0002]: hole ?view has type Html Rooms.Show.Msg
  view model session = ?view
                       ^^^^^
added import Booking to app/Rooms/Show.alder
[exit 0]
$ alder fill Rooms.Show.view - <<'EOF'
<Layout.Page title="Room">
  <h1 id="name">{model.name}</h1>
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
 
 page "/rooms/{id}" { id: Room.RoomId } requires user
@@ -20,7 +21,9 @@
 }
 
 view : Model -> Session -> Html Msg
-view model session = ?view
+view model session = <Layout.Page title="Room">
+  <h1 id="name">{model.name}</h1>
+</Layout.Page>
 
 test "a visitor is sent to log in" = {
   let room1 <- Db.insert Room { name: Text.NonEmpty "name-1" }
added import Layout to app/Rooms/Show.alder
[exit 0]
$ alder new form Rooms.Show NewBooking from:Date to:Date --msg Book
edited app/Rooms/Show.alder
--- a/app/Rooms/Show.alder
+++ b/app/Rooms/Show.alder
@@ -3,6 +3,7 @@
 import Booking
 import Layout
 import Room
+import User
 
 page "/rooms/{id}" { id: Room.RoomId } requires user
 
@@ -20,9 +21,31 @@
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
 view model session = <Layout.Page title="Room">
   <h1 id="name">{model.name}</h1>
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
@@ -30,3 +53,22 @@
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
app/Rooms/Show.alder:33:13: warning[W0002]: hole ?book has type Task Nav
    Book p -> ?book,
              ^^^^^
app/Rooms/Show.alder:64:29: warning[W0002]: hole ?expected has type Answer
    Task.succeed (r.answer == ?expected)
                              ^^^^^^^^^
[exit 0]
$ alder fill Rooms.Show.update.Book - <<'EOF'
{
  let others <- Db.from Booking |> Db.where (Booking.room.eq model.id) |> Db.all
  let badRange = p.form.to <= p.form.from
  let clash = List.any (Booking.overlaps p.form.from p.form.to) others
  if badRange then Task.succeed (Reject "The end date must be after the start date.") else if clash then Task.succeed (Reject "The room is already booked during these dates.") else {
    let _ <- Db.insert Booking { room: model.id, owner: session.user, from: p.form.from, to: p.form.to }
    Task.succeed (Redirect (Rooms.Show { id: model.id }))
  }
}
EOF
edited app/Rooms/Show.alder
--- a/app/Rooms/Show.alder
+++ b/app/Rooms/Show.alder
@@ -30,7 +30,15 @@
 
 update : Msg -> Model -> Session -> Task Nav
 update msg model session = match msg {
-  Book p -> ?book,
+  Book p -> {
+    let others <- Db.from Booking |> Db.where (Booking.room.eq model.id) |> Db.all
+    let badRange = p.form.to <= p.form.from
+    let clash = List.any (Booking.overlaps p.form.from p.form.to) others
+    if badRange then Task.succeed (Reject "The end date must be after the start date.") else if clash then Task.succeed (Reject "The room is already booked during these dates.") else {
+      let _ <- Db.insert Booking { room: model.id, owner: session.user, from: p.form.from, to: p.form.to }
+      Task.succeed (Redirect (Rooms.Show { id: model.id }))
+    }
+  },
 }
 
 view : Model -> Session -> Html Msg
app/Rooms/Show.alder:72:29: warning[W0002]: hole ?expected has type Answer
    Task.succeed (r.answer == ?expected)
                              ^^^^^^^^^
[exit 0]
$ alder fill --decl Rooms.Show.view - <<'EOF'
<Layout.Page title={model.name}>
  <h1 id="name">{model.name}</h1>
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
  <p><a href={Rooms.Index {}}>All rooms</a></p>
</Layout.Page>
EOF
edited app/Rooms/Show.alder
--- a/app/Rooms/Show.alder
+++ b/app/Rooms/Show.alder
@@ -42,19 +42,26 @@
 }
 
 view : Model -> Session -> Html Msg
-view model session = <Layout.Page title="Room">
-  <h1 id="name">{model.name}</h1>
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
+  <Layout.Page title={model.name}>
+    <h1 id="name">{model.name}</h1>
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
+    <p><a href={Rooms.Index {}}>All rooms</a></p>
+  </Layout.Page>
 
 test "a visitor is sent to log in" = {
   let room1 <- Db.insert Room { name: Text.NonEmpty "name-1" }
app/Rooms/Show.alder:79:29: warning[W0002]: hole ?expected has type Answer
    Task.succeed (r.answer == ?expected)
                              ^^^^^^^^^
[exit 0]
$ alder fill --decl Bookings.Index.Model - <<'EOF'
{ rows: List { id: Booking.BookingId, room: Text.NonEmpty, from: Date } }
EOF
edited app/Bookings/Index.alder
--- a/app/Bookings/Index.alder
+++ b/app/Bookings/Index.alder
@@ -1,8 +1,10 @@
 module Bookings.Index
 
+import Booking
+
 page "/bookings" { } requires user
 
-type Model = { }
+type Model = { rows: List { id: Booking.BookingId, room: Text.NonEmpty, from: Date } }
 
 load : Params -> Session -> Query Model
 load params session = ?load
app/Bookings/Index.alder:10:23: warning[W0002]: hole ?load has type Query Bookings.Index.Model
  load params session = ?load
                        ^^^^^
app/Bookings/Index.alder:13:22: warning[W0002]: hole ?view has type Html Bookings.Index.Msg
  view model session = ?view
                       ^^^^^
added import Booking to app/Bookings/Index.alder
[exit 0]
$ alder fill Bookings.Index.load - <<'EOF'
{
  let mine <- Db.from Booking |> Db.where (Booking.owner.eq session.user) |> Db.orderBy Booking.from.asc |> Db.all
  let rooms <- Db.from Room |> Db.orderBy Room.name.asc |> Db.all
  Query.succeed { rows: List.filterMap (\b -> Maybe.map (\r -> { id: b.id, room: r.name, from: b.from }) (List.find (\r -> r.id == b.room) rooms)) mine }
}
EOF
edited app/Bookings/Index.alder
--- a/app/Bookings/Index.alder
+++ b/app/Bookings/Index.alder
@@ -1,13 +1,18 @@
 module Bookings.Index
 
 import Booking
+import Room
 
 page "/bookings" { } requires user
 
 type Model = { rows: List { id: Booking.BookingId, room: Text.NonEmpty, from: Date } }
 
 load : Params -> Session -> Query Model
-load params session = ?load
+load params session = {
+  let mine <- Db.from Booking |> Db.where (Booking.owner.eq session.user) |> Db.orderBy Booking.from.asc |> Db.all
+  let rooms <- Db.from Room |> Db.orderBy Room.name.asc |> Db.all
+  Query.succeed { rows: List.filterMap (\b -> Maybe.map (\r -> { id: b.id, room: r.name, from: b.from }) (List.find (\r -> r.id == b.room) rooms)) mine }
+}
 
 view : Model -> Session -> Html Msg
 view model session = ?view
app/Bookings/Index.alder:18:22: warning[W0002]: hole ?view has type Html Bookings.Index.Msg
  view model session = ?view
                       ^^^^^
added import Room to app/Bookings/Index.alder
[exit 0]
$ alder fill Bookings.Index.view - <<'EOF'
<Layout.Page title="My bookings">
  <h1>My bookings</h1>
  <ul id="mine">
    {for row in model.rows ->
      <li>
        <a href={Bookings.Show { id: row.id }}>
          <span class="room">{row.room}</span>
          <span class="from">{Date.toText row.from}</span>
        </a>
      </li>}
  </ul>
  <p><a href={Rooms.Index {}}>All rooms</a></p>
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
 
 page "/bookings" { } requires user
@@ -15,7 +16,19 @@
 }
 
 view : Model -> Session -> Html Msg
-view model session = ?view
+view model session = <Layout.Page title="My bookings">
+  <h1>My bookings</h1>
+  <ul id="mine">
+    {for row in model.rows ->
+      <li>
+        <a href={Bookings.Show { id: row.id }}>
+          <span class="room">{row.room}</span>
+          <span class="from">{Date.toText row.from}</span>
+        </a>
+      </li>}
+  </ul>
+  <p><a href={Rooms.Index {}}>All rooms</a></p>
+</Layout.Page>
 
 test "a visitor is sent to log in" = {
   let r <- Test.open (Bookings.Index {})
added import Layout to app/Bookings/Index.alder
[exit 0]
$ alder fill --decl Bookings.Show.Model - <<'EOF'
{ id: Booking.BookingId, room: Text.NonEmpty, from: Date, to: Date, today: Date }
EOF
edited app/Bookings/Show.alder
--- a/app/Bookings/Show.alder
+++ b/app/Bookings/Show.alder
@@ -6,7 +6,7 @@
 
 page "/bookings/{id}" { id: Booking.BookingId } requires user
 
-type Model = { }
+type Model = { id: Booking.BookingId, room: Text.NonEmpty, from: Date, to: Date, today: Date }
 
 load : Params -> Session -> Query Model
 load params session = ?load
app/Bookings/Show.alder:12:23: warning[W0002]: hole ?load has type Query Bookings.Show.Model
  load params session = ?load
                        ^^^^^
app/Bookings/Show.alder:15:22: warning[W0002]: hole ?view has type Html Bookings.Show.Msg
  view model session = ?view
                       ^^^^^
[exit 0]
$ alder fill Bookings.Show.load - <<'EOF'
{
  let found <- Db.get Booking params.id
  let today <- Date.today
  match found {
    Just b -> {
      let room <- Db.from Room |> Db.where (Room.id.eq b.room) |> Db.first
      match room {
        Just r -> if b.owner == session.user then Query.succeed { id: b.id, room: r.name, from: b.from, to: b.to, today: today } else Query.notFound,
        Nothing -> Query.notFound,
      }
    },
    Nothing -> Query.notFound,
  }
}
EOF
edited app/Bookings/Show.alder
--- a/app/Bookings/Show.alder
+++ b/app/Bookings/Show.alder
@@ -9,7 +9,20 @@
 type Model = { id: Booking.BookingId, room: Text.NonEmpty, from: Date, to: Date, today: Date }
 
 load : Params -> Session -> Query Model
-load params session = ?load
+load params session = {
+  let found <- Db.get Booking params.id
+  let today <- Date.today
+  match found {
+    Just b -> {
+      let room <- Db.from Room |> Db.where (Room.id.eq b.room) |> Db.first
+      match room {
+        Just r -> if b.owner == session.user then Query.succeed { id: b.id, room: r.name, from: b.from, to: b.to, today: today } else Query.notFound,
+        Nothing -> Query.notFound,
+      }
+    },
+    Nothing -> Query.notFound,
+  }
+}
 
 view : Model -> Session -> Html Msg
 view model session = ?view
app/Bookings/Show.alder:28:22: warning[W0002]: hole ?view has type Html Bookings.Show.Msg
  view model session = ?view
                       ^^^^^
[exit 0]
$ alder fill Bookings.Show.view - <<'EOF'
<Layout.Page title="Booking">
  <h1>Booking</h1>
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
 
@@ -25,7 +26,9 @@
 }
 
 view : Model -> Session -> Html Msg
-view model session = ?view
+view model session = <Layout.Page title="Booking">
+  <h1>Booking</h1>
+</Layout.Page>
 
 test "a visitor is sent to log in" = {
   let today <- Date.today
added import Layout to app/Bookings/Show.alder
[exit 0]
$ alder new msg Bookings.Show Cancel
edited app/Bookings/Show.alder
--- a/app/Bookings/Show.alder
+++ b/app/Bookings/Show.alder
@@ -25,9 +25,18 @@
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
+
+  <al-button click={Cancel}>Cancel</al-button>
 </Layout.Page>
 
 test "a visitor is sent to log in" = {
@@ -38,3 +47,20 @@
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
app/Bookings/Show.alder:32:15: warning[W0002]: hole ?cancel has type Task Nav
    Cancel p -> ?cancel,
                ^^^^^^^
app/Bookings/Show.alder:65:16: warning[W0002]: hole ?expected has type Bool
    Task.succeed ?expected
                 ^^^^^^^^^
[exit 0]
$ alder fill Bookings.Show.update.Cancel - <<'EOF'
{
  let today <- Date.today
  if Booking.canCancel today model.from then {
    let _ <- Db.delete Booking model.id
    Task.succeed (Redirect (Bookings.Index {}))
  } else Task.succeed Rerender
}
EOF
edited app/Bookings/Show.alder
--- a/app/Bookings/Show.alder
+++ b/app/Bookings/Show.alder
@@ -29,7 +29,13 @@
 
 update : Msg -> Model -> Session -> Task Nav
 update msg model session = match msg {
-  Cancel p -> ?cancel,
+  Cancel p -> {
+    let today <- Date.today
+    if Booking.canCancel today model.from then {
+      let _ <- Db.delete Booking model.id
+      Task.succeed (Redirect (Bookings.Index {}))
+    } else Task.succeed Rerender
+  },
 }
 
 view : Model -> Session -> Html Msg
app/Bookings/Show.alder:71:16: warning[W0002]: hole ?expected has type Bool
    Task.succeed ?expected
                 ^^^^^^^^^
[exit 0]
$ alder fill --decl Bookings.Show.view - <<'EOF'
<Layout.Page title="Booking">
  <h1>Booking</h1>
  <p id="room">{model.room}</p>
  <p id="from">{Date.toText model.from}</p>
  <p id="to">{Date.toText model.to}</p>
  {if Booking.canCancel model.today model.from then <button id="cancel" click={Cancel}>Cancel</button> else Html.none}
  <p><a href={Bookings.Index {}}>My bookings</a></p>
</Layout.Page>
EOF
edited app/Bookings/Show.alder
--- a/app/Bookings/Show.alder
+++ b/app/Bookings/Show.alder
@@ -39,11 +39,15 @@
 }
 
 view : Model -> Session -> Html Msg
-view model session = <Layout.Page title="Booking">
-  <h1>Booking</h1>
-
-  <al-button click={Cancel}>Cancel</al-button>
-</Layout.Page>
+view model session =
+  <Layout.Page title="Booking">
+    <h1>Booking</h1>
+    <p id="room">{model.room}</p>
+    <p id="from">{Date.toText model.from}</p>
+    <p id="to">{Date.toText model.to}</p>
+    {if Booking.canCancel model.today model.from then <button id="cancel" click={Cancel}>Cancel</button> else Html.none}
+    <p><a href={Bookings.Index {}}>My bookings</a></p>
+  </Layout.Page>
 
 test "a visitor is sent to log in" = {
   let today <- Date.today
app/Bookings/Show.alder:75:16: warning[W0002]: hole ?expected has type Bool
    Task.succeed ?expected
                 ^^^^^^^^^
[exit 0]
$ alder fill --decl Auth.Login.update - <<'EOF'
match msg {
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
$ alder new test Rooms.Index "the next free date of a room with back-to-back bookings"
edited app/Rooms/Index.alder
--- a/app/Rooms/Index.alder
+++ b/app/Rooms/Index.alder
@@ -34,3 +34,7 @@
   let r <- Test.open (Rooms.Index {})
   Task.succeed (r.answer == Redirected (Auth.Login {}))
 }
+
+test "the next free date of a room with back-to-back bookings" = {
+  ?test
+}
app/Rooms/Index.alder:39:3: warning[W0002]: hole ?test has type Task Bool
    ?test
    ^^^^^
[exit 0]
$ alder fill 'Rooms.Index."the next free date of a room with back-to-back bookings"' - <<'EOF'
{
  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
  let known <- Db.from User |> Db.where (User.email.eq (Text.Email "me@example.com")) |> Db.count
  let busy <- Db.insert Room { name: Text.NonEmpty "Busy" }
  let _ <- Db.insert Room { name: Text.NonEmpty "Empty" }
  let today <- Date.today
  let _ <- Db.insert Booking { room: busy, owner: me, from: today, to: Date.addDays 2 today }
  let _ <- Db.insert Booking { room: busy, owner: me, from: Date.addDays 2 today, to: Date.addDays 5 today }
  let _ <- Db.insert Booking { room: busy, owner: me, from: Date.addDays 7 today, to: Date.addDays 8 today }
  let _ <- Test.login { user: Just me }
  let page <- Test.get (Rooms.Index {})
  Task.succeed ({ known: known, names: Test.texts "#rooms .name" page.doc, free: Test.texts "#rooms .free" page.doc } == { known: 1, names: ["Busy", "Empty"], free: [Date.toText (Date.addDays 5 today), Date.toText today] })
}
EOF
edited app/Rooms/Index.alder
--- a/app/Rooms/Index.alder
+++ b/app/Rooms/Index.alder
@@ -3,6 +3,7 @@
 import Booking
 import Layout
 import Room
+import User
 
 page "/rooms" { } requires user
 
@@ -36,5 +37,15 @@
 }
 
 test "the next free date of a room with back-to-back bookings" = {
-  ?test
+  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
+  let known <- Db.from User |> Db.where (User.email.eq (Text.Email "me@example.com")) |> Db.count
+  let busy <- Db.insert Room { name: Text.NonEmpty "Busy" }
+  let _ <- Db.insert Room { name: Text.NonEmpty "Empty" }
+  let today <- Date.today
+  let _ <- Db.insert Booking { room: busy, owner: me, from: today, to: Date.addDays 2 today }
+  let _ <- Db.insert Booking { room: busy, owner: me, from: Date.addDays 2 today, to: Date.addDays 5 today }
+  let _ <- Db.insert Booking { room: busy, owner: me, from: Date.addDays 7 today, to: Date.addDays 8 today }
+  let _ <- Test.login { user: Just me }
+  let page <- Test.get (Rooms.Index {})
+  Task.succeed ({ known: known, names: Test.texts "#rooms .name" page.doc, free: Test.texts "#rooms .free" page.doc } == { known: 1, names: ["Busy", "Empty"], free: [Date.toText (Date.addDays 5 today), Date.toText today] })
 }
added import User to app/Rooms/Index.alder
[exit 0]
$ alder new test Rooms.Show "an overlapping booking is refused and not stored"
edited app/Rooms/Show.alder
--- a/app/Rooms/Show.alder
+++ b/app/Rooms/Show.alder
@@ -87,3 +87,7 @@
   let r <- Test.submitRaw NewBooking [{ name: "from", value: "" }, { name: "to", value: "2026-01-01" }]
   Task.succeed (r.answer == FormRejected [{ field: "from", message: "is required" }])
 }
+
+test "an overlapping booking is refused and not stored" = {
+  ?test
+}
app/Rooms/Show.alder:79:29: warning[W0002]: hole ?expected has type Answer
    Task.succeed (r.answer == ?expected)
                              ^^^^^^^^^
app/Rooms/Show.alder:92:3: warning[W0002]: hole ?test has type Task Bool
    ?test
    ^^^^^
[exit 0]
$ alder fill 'Rooms.Show."an overlapping booking is refused and not stored"' - <<'EOF'
{
  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
  let known <- Db.from User |> Db.where (User.email.eq (Text.Email "me@example.com")) |> Db.count
  let room <- Db.insert Room { name: Text.NonEmpty "Blue" }
  let today <- Date.today
  let _ <- Db.insert Booking { room: room, owner: me, from: Date.addDays 1 today, to: Date.addDays 3 today }
  let _ <- Test.login { user: Just me }
  let _ <- Test.get (Rooms.Show { id: room })
  let r <- Test.submit NewBooking { from: Date.addDays 2 today, to: Date.addDays 4 today }
  let n <- Db.from Booking |> Db.count
  Task.succeed ({ known: known, answer: r.answer, n: n } == { known: 1, answer: FormRejected [{ field: "", message: "The room is already booked during these dates." }], n: 1 })
}
EOF
edited app/Rooms/Show.alder
--- a/app/Rooms/Show.alder
+++ b/app/Rooms/Show.alder
@@ -89,5 +89,14 @@
 }
 
 test "an overlapping booking is refused and not stored" = {
-  ?test
+  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
+  let known <- Db.from User |> Db.where (User.email.eq (Text.Email "me@example.com")) |> Db.count
+  let room <- Db.insert Room { name: Text.NonEmpty "Blue" }
+  let today <- Date.today
+  let _ <- Db.insert Booking { room: room, owner: me, from: Date.addDays 1 today, to: Date.addDays 3 today }
+  let _ <- Test.login { user: Just me }
+  let _ <- Test.get (Rooms.Show { id: room })
+  let r <- Test.submit NewBooking { from: Date.addDays 2 today, to: Date.addDays 4 today }
+  let n <- Db.from Booking |> Db.count
+  Task.succeed ({ known: known, answer: r.answer, n: n } == { known: 1, answer: FormRejected [{ field: "", message: "The room is already booked during these dates." }], n: 1 })
 }
app/Rooms/Show.alder:79:29: warning[W0002]: hole ?expected has type Answer
    Task.succeed (r.answer == ?expected)
                              ^^^^^^^^^
[exit 0]
$ alder new test Rooms.Show "a booking starting on the other's end date is stored"
edited app/Rooms/Show.alder
--- a/app/Rooms/Show.alder
+++ b/app/Rooms/Show.alder
@@ -100,3 +100,7 @@
   let n <- Db.from Booking |> Db.count
   Task.succeed ({ known: known, answer: r.answer, n: n } == { known: 1, answer: FormRejected [{ field: "", message: "The room is already booked during these dates." }], n: 1 })
 }
+
+test "a booking starting on the other's end date is stored" = {
+  ?test
+}
app/Rooms/Show.alder:79:29: warning[W0002]: hole ?expected has type Answer
    Task.succeed (r.answer == ?expected)
                              ^^^^^^^^^
app/Rooms/Show.alder:105:3: warning[W0002]: hole ?test has type Task Bool
    ?test
    ^^^^^
[exit 0]
$ alder fill 'Rooms.Show."a booking starting on the other'"'"'s end date is stored"' - <<'EOF'
{
  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
  let room <- Db.insert Room { name: Text.NonEmpty "Blue" }
  let today <- Date.today
  let _ <- Db.insert Booking { room: room, owner: me, from: Date.addDays 1 today, to: Date.addDays 3 today }
  let _ <- Test.login { user: Just me }
  let _ <- Test.get (Rooms.Show { id: room })
  let r <- Test.submit NewBooking { from: Date.addDays 3 today, to: Date.addDays 4 today }
  let n <- Db.from Booking |> Db.count
  Task.succeed ({ answer: r.answer, n: n } == { answer: Redirected (Rooms.Show { id: room }), n: 2 })
}
EOF
edited app/Rooms/Show.alder
--- a/app/Rooms/Show.alder
+++ b/app/Rooms/Show.alder
@@ -102,5 +102,13 @@
 }
 
 test "a booking starting on the other's end date is stored" = {
-  ?test
+  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
+  let room <- Db.insert Room { name: Text.NonEmpty "Blue" }
+  let today <- Date.today
+  let _ <- Db.insert Booking { room: room, owner: me, from: Date.addDays 1 today, to: Date.addDays 3 today }
+  let _ <- Test.login { user: Just me }
+  let _ <- Test.get (Rooms.Show { id: room })
+  let r <- Test.submit NewBooking { from: Date.addDays 3 today, to: Date.addDays 4 today }
+  let n <- Db.from Booking |> Db.count
+  Task.succeed ({ answer: r.answer, n: n } == { answer: Redirected (Rooms.Show { id: room }), n: 2 })
 }
app/Rooms/Show.alder:79:29: warning[W0002]: hole ?expected has type Answer
    Task.succeed (r.answer == ?expected)
                              ^^^^^^^^^
[exit 0]
$ alder new test Rooms.Show "a booking whose to is not after its from is refused"
edited app/Rooms/Show.alder
--- a/app/Rooms/Show.alder
+++ b/app/Rooms/Show.alder
@@ -112,3 +112,7 @@
   let n <- Db.from Booking |> Db.count
   Task.succeed ({ answer: r.answer, n: n } == { answer: Redirected (Rooms.Show { id: room }), n: 2 })
 }
+
+test "a booking whose to is not after its from is refused" = {
+  ?test
+}
app/Rooms/Show.alder:79:29: warning[W0002]: hole ?expected has type Answer
    Task.succeed (r.answer == ?expected)
                              ^^^^^^^^^
app/Rooms/Show.alder:117:3: warning[W0002]: hole ?test has type Task Bool
    ?test
    ^^^^^
[exit 0]
$ alder fill 'Rooms.Show."a booking whose to is not after its from is refused"' - <<'EOF'
{
  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
  let room <- Db.insert Room { name: Text.NonEmpty "Blue" }
  let today <- Date.today
  let _ <- Test.login { user: Just me }
  let _ <- Test.get (Rooms.Show { id: room })
  let r <- Test.submit NewBooking { from: Date.addDays 2 today, to: Date.addDays 2 today }
  let n <- Db.from Booking |> Db.count
  Task.succeed ({ answer: r.answer, n: n } == { answer: FormRejected [{ field: "", message: "The end date must be after the start date." }], n: 0 })
}
EOF
edited app/Rooms/Show.alder
--- a/app/Rooms/Show.alder
+++ b/app/Rooms/Show.alder
@@ -114,5 +114,12 @@
 }
 
 test "a booking whose to is not after its from is refused" = {
-  ?test
+  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
+  let room <- Db.insert Room { name: Text.NonEmpty "Blue" }
+  let today <- Date.today
+  let _ <- Test.login { user: Just me }
+  let _ <- Test.get (Rooms.Show { id: room })
+  let r <- Test.submit NewBooking { from: Date.addDays 2 today, to: Date.addDays 2 today }
+  let n <- Db.from Booking |> Db.count
+  Task.succeed ({ answer: r.answer, n: n } == { answer: FormRejected [{ field: "", message: "The end date must be after the start date." }], n: 0 })
 }
app/Rooms/Show.alder:79:29: warning[W0002]: hole ?expected has type Answer
    Task.succeed (r.answer == ?expected)
                              ^^^^^^^^^
[exit 0]
$ alder new test Bookings.Show "the owner cancels a booking before its from date"
edited app/Bookings/Show.alder
--- a/app/Bookings/Show.alder
+++ b/app/Bookings/Show.alder
@@ -74,3 +74,7 @@
   let after <- Test.open (Bookings.Show { id: booking1 })
   Task.succeed ?expected
 }
+
+test "the owner cancels a booking before its from date" = {
+  ?test
+}
app/Bookings/Show.alder:75:16: warning[W0002]: hole ?expected has type Bool
    Task.succeed ?expected
                 ^^^^^^^^^
app/Bookings/Show.alder:79:3: warning[W0002]: hole ?test has type Task Bool
    ?test
    ^^^^^
[exit 0]
$ alder fill 'Bookings.Show."the owner cancels a booking before its from date"' - <<'EOF'
{
  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
  let known <- Db.from User |> Db.where (User.email.eq (Text.Email "me@example.com")) |> Db.count
  let room <- Db.insert Room { name: Text.NonEmpty "Blue" }
  let today <- Date.today
  let id <- Db.insert Booking { room: room, owner: me, from: Date.addDays 2 today, to: Date.addDays 4 today }
  let _ <- Test.login { user: Just me }
  let page <- Test.get (Bookings.Show { id: id })
  let r <- Test.click Cancel
  let after <- Db.get Booking id
  Task.succeed ({ known: known, button: Test.has "#cancel" page.doc, answer: r.answer, still: Maybe.isJust after } == { known: 1, button: True, answer: Redirected (Bookings.Index {}), still: False })
}
EOF
edited app/Bookings/Show.alder
--- a/app/Bookings/Show.alder
+++ b/app/Bookings/Show.alder
@@ -76,5 +76,14 @@
 }
 
 test "the owner cancels a booking before its from date" = {
-  ?test
+  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
+  let known <- Db.from User |> Db.where (User.email.eq (Text.Email "me@example.com")) |> Db.count
+  let room <- Db.insert Room { name: Text.NonEmpty "Blue" }
+  let today <- Date.today
+  let id <- Db.insert Booking { room: room, owner: me, from: Date.addDays 2 today, to: Date.addDays 4 today }
+  let _ <- Test.login { user: Just me }
+  let page <- Test.get (Bookings.Show { id: id })
+  let r <- Test.click Cancel
+  let after <- Db.get Booking id
+  Task.succeed ({ known: known, button: Test.has "#cancel" page.doc, answer: r.answer, still: Maybe.isJust after } == { known: 1, button: True, answer: Redirected (Bookings.Index {}), still: False })
 }
app/Bookings/Show.alder:75:16: warning[W0002]: hole ?expected has type Bool
    Task.succeed ?expected
                 ^^^^^^^^^
[exit 0]
$ alder new test Bookings.Show "a booking whose from date is today cannot be cancelled"
edited app/Bookings/Show.alder
--- a/app/Bookings/Show.alder
+++ b/app/Bookings/Show.alder
@@ -87,3 +87,7 @@
   let after <- Db.get Booking id
   Task.succeed ({ known: known, button: Test.has "#cancel" page.doc, answer: r.answer, still: Maybe.isJust after } == { known: 1, button: True, answer: Redirected (Bookings.Index {}), still: False })
 }
+
+test "a booking whose from date is today cannot be cancelled" = {
+  ?test
+}
app/Bookings/Show.alder:75:16: warning[W0002]: hole ?expected has type Bool
    Task.succeed ?expected
                 ^^^^^^^^^
app/Bookings/Show.alder:92:3: warning[W0002]: hole ?test has type Task Bool
    ?test
    ^^^^^
[exit 0]
$ alder fill 'Bookings.Show."a booking whose from date is today cannot be cancelled"' - <<'EOF'
{
  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
  let room <- Db.insert Room { name: Text.NonEmpty "Blue" }
  let today <- Date.today
  let id <- Db.insert Booking { room: room, owner: me, from: today, to: Date.addDays 2 today }
  let _ <- Test.login { user: Just me }
  let page <- Test.get (Bookings.Show { id: id })
  let r <- Test.run (update Cancel page.model { user: me })
  let after <- Db.get Booking id
  Task.succeed ({ button: Test.has "#cancel" page.doc, value: r.value, still: Maybe.isJust after } == { button: False, value: Ok Rerender, still: True })
}
EOF
edited app/Bookings/Show.alder
--- a/app/Bookings/Show.alder
+++ b/app/Bookings/Show.alder
@@ -89,5 +89,13 @@
 }
 
 test "a booking whose from date is today cannot be cancelled" = {
-  ?test
+  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
+  let room <- Db.insert Room { name: Text.NonEmpty "Blue" }
+  let today <- Date.today
+  let id <- Db.insert Booking { room: room, owner: me, from: today, to: Date.addDays 2 today }
+  let _ <- Test.login { user: Just me }
+  let page <- Test.get (Bookings.Show { id: id })
+  let r <- Test.run (update Cancel page.model { user: me })
+  let after <- Db.get Booking id
+  Task.succeed ({ button: Test.has "#cancel" page.doc, value: r.value, still: Maybe.isJust after } == { button: False, value: Ok Rerender, still: True })
 }
app/Bookings/Show.alder:75:16: warning[W0002]: hole ?expected has type Bool
    Task.succeed ?expected
                 ^^^^^^^^^
[exit 0]
$ alder new test Bookings.Show "a stranger gets a 404"
edited app/Bookings/Show.alder
--- a/app/Bookings/Show.alder
+++ b/app/Bookings/Show.alder
@@ -99,3 +99,7 @@
   let after <- Db.get Booking id
   Task.succeed ({ button: Test.has "#cancel" page.doc, value: r.value, still: Maybe.isJust after } == { button: False, value: Ok Rerender, still: True })
 }
+
+test "a stranger gets a 404" = {
+  ?test
+}
app/Bookings/Show.alder:75:16: warning[W0002]: hole ?expected has type Bool
    Task.succeed ?expected
                 ^^^^^^^^^
app/Bookings/Show.alder:104:3: warning[W0002]: hole ?test has type Task Bool
    ?test
    ^^^^^
[exit 0]
$ alder fill 'Bookings.Show."a stranger gets a 404"' - <<'EOF'
{
  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
  let room <- Db.insert Room { name: Text.NonEmpty "Blue" }
  let today <- Date.today
  let id <- Db.insert Booking { room: room, owner: me, from: Date.addDays 2 today, to: Date.addDays 4 today }
  let _ <- Test.login { user: Just (User.UserId "someone-else") }
  let r <- Test.open (Bookings.Show { id: id })
  Task.succeed (r.answer == PageMissing)
}
EOF
edited app/Bookings/Show.alder
--- a/app/Bookings/Show.alder
+++ b/app/Bookings/Show.alder
@@ -101,5 +101,11 @@
 }
 
 test "a stranger gets a 404" = {
-  ?test
+  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
+  let room <- Db.insert Room { name: Text.NonEmpty "Blue" }
+  let today <- Date.today
+  let id <- Db.insert Booking { room: room, owner: me, from: Date.addDays 2 today, to: Date.addDays 4 today }
+  let _ <- Test.login { user: Just (User.UserId "someone-else") }
+  let r <- Test.open (Bookings.Show { id: id })
+  Task.succeed (r.answer == PageMissing)
 }
app/Bookings/Show.alder:75:16: warning[W0002]: hole ?expected has type Bool
    Task.succeed ?expected
                 ^^^^^^^^^
[exit 0]
$ alder check
app/Auth/Login.alder:86:85: warning[W0002]: hole ?afterLogin has type Page
    Task.succeed ({ answer: r.answer, user: r.session.user } == { answer: Redirected (?afterLogin), user: Just me })
                                                                                      ^^^^^^^^^^^
app/Bookings/Show.alder:75:16: warning[W0002]: hole ?expected has type Bool
    Task.succeed ?expected
                 ^^^^^^^^^
app/Rooms/Show.alder:79:29: warning[W0002]: hole ?expected has type Answer
    Task.succeed (r.answer == ?expected)
                              ^^^^^^^^^
alder check: 3 typed holes left
[exit 0]
$ alder test
app/Auth/Login.alder:86:85: warning[W0002]: hole ?afterLogin has type Page
    Task.succeed ({ answer: r.answer, user: r.session.user } == { answer: Redirected (?afterLogin), user: Just me })
                                                                                      ^^^^^^^^^^^
app/Bookings/Show.alder:75:16: warning[W0002]: hole ?expected has type Bool
    Task.succeed ?expected
                 ^^^^^^^^^
app/Rooms/Show.alder:79:29: warning[W0002]: hole ?expected has type Answer
    Task.succeed (r.answer == ?expected)
                              ^^^^^^^^^
alder test: no tests ran (fill the hole): ?afterLogin : Page, ?expected : Bool, ?expected : Answer
[exit 1]
```

The harness stopped at line 309 of your script: `alder test`: it exited 1. The commands after it were not run.

Your script stopped at command 49; the app is as the commands before it left it; continue.

The program type-checks but `alder build` refused it; fix the cause below.

```text
app/Auth/Login.alder:86:85: warning[W0002]: hole ?afterLogin has type Page
    Task.succeed ({ answer: r.answer, user: r.session.user } == { answer: Redirected (?afterLogin), user: Just me })
                                                                                      ^^^^^^^^^^^
app/Bookings/Show.alder:75:16: warning[W0002]: hole ?expected has type Bool
    Task.succeed ?expected
                 ^^^^^^^^^
app/Rooms/Show.alder:79:29: warning[W0002]: hole ?expected has type Answer
    Task.succeed (r.answer == ?expected)
                              ^^^^^^^^^
alder build: the program has 3 typed hole(s); fill them before building
```
