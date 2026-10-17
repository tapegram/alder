### Round 1

```text
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
$ alder new page Rooms.Show '/rooms/{id}' --param id:Room.RoomId --requires user --blank
wrote app/Rooms/Show.alder
app/Rooms/Show.alder:10:23: warning[W0002]: hole ?load has type Query Rooms.Show.Model
  load params session = ?load
                        ^^^^^
app/Rooms/Show.alder:13:22: warning[W0002]: hole ?view has type Html Rooms.Show.Msg
  view model session = ?view
                       ^^^^^
[exit 0]
$ alder fill --decl Rooms.Show.Model - <<'EOF'
{ room: Room, bookings: List Booking }
EOF
edited app/Rooms/Show.alder
--- a/app/Rooms/Show.alder
+++ b/app/Rooms/Show.alder
@@ -1,10 +1,11 @@
 module Rooms.Show
 
+import Booking
 import Room
 
 page "/rooms/{id}" { id: Room.RoomId } requires user
 
-type Model = { }
+type Model = { room: Room, bookings: List Booking }
 
 load : Params -> Session -> Query Model
 load params session = ?load
app/Rooms/Show.alder:11:23: warning[W0002]: hole ?load has type Query Rooms.Show.Model
  load params session = ?load
                        ^^^^^
app/Rooms/Show.alder:14:22: warning[W0002]: hole ?view has type Html Rooms.Show.Msg
  view model session = ?view
                       ^^^^^
added import Booking to app/Rooms/Show.alder
[exit 0]
$ alder fill Rooms.Show.load - --hole ?load <<'EOF'
{
  let p <- Booking.roomPage params.id
  Query.succeed { room: p.room, bookings: p.bookings }
}
EOF
edited app/Rooms/Show.alder
--- a/app/Rooms/Show.alder
+++ b/app/Rooms/Show.alder
@@ -8,7 +8,10 @@
 type Model = { room: Room, bookings: List Booking }
 
 load : Params -> Session -> Query Model
-load params session = ?load
+load params session = {
+  let p <- Booking.roomPage params.id
+  Query.succeed { room: p.room, bookings: p.bookings }
+}
 
 view : Model -> Session -> Html Msg
 view model session = ?view
app/Rooms/Show.alder:17:22: warning[W0002]: hole ?view has type Html Rooms.Show.Msg
  view model session = ?view
                       ^^^^^
[exit 0]
$ alder fill Rooms.Show.view - --hole ?view <<'EOF'
<Layout.Page title={model.room.name}>
  <h1 id="name">{model.room.name}</h1>
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
@@ -14,7 +15,9 @@
 }
 
 view : Model -> Session -> Html Msg
-view model session = ?view
+view model session = <Layout.Page title={model.room.name}>
+  <h1 id="name">{model.room.name}</h1>
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
 
@@ -14,9 +15,31 @@
   Query.succeed { room: p.room, bookings: p.bookings }
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
@@ -24,3 +47,22 @@
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
app/Rooms/Show.alder:27:13: warning[W0002]: hole ?book has type Task Nav
    Book p -> ?book,
              ^^^^^
app/Rooms/Show.alder:58:29: warning[W0002]: hole ?expected has type Answer
    Task.succeed (r.answer == ?expected)
                              ^^^^^^^^^
[exit 0]
$ alder fill --decl Rooms.Show.view - <<'EOF'
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
edited app/Rooms/Show.alder
--- a/app/Rooms/Show.alder
+++ b/app/Rooms/Show.alder
@@ -28,19 +28,21 @@
 }
 
 view : Model -> Session -> Html Msg
-view model session = <Layout.Page title={model.room.name}>
-  <h1 id="name">{model.room.name}</h1>
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
+      {for b in model.bookings -> <li><span class="from">{Date.toText b.from}</span> to <span class="to">{Date.toText b.to}</span></li>}
+    </ul>
+    <al-card>
+      <form submit={Book}>
+        <Errors form />
+        <al-input field="from" label="From" />
+        <al-input field="to" label="To" />
+        <al-button type="submit">Book</al-button>
+      </form>
+    </al-card>
+  </Layout.Page>
 
 test "a visitor is sent to log in" = {
   let room1 <- Db.insert Room { name: Text.NonEmpty "name-1" }
app/Rooms/Show.alder:27:13: warning[W0002]: hole ?book has type Task Nav
    Book p -> ?book,
              ^^^^^
app/Rooms/Show.alder:60:29: warning[W0002]: hole ?expected has type Answer
    Task.succeed (r.answer == ?expected)
                              ^^^^^^^^^
[exit 0]
$ alder fill --decl Rooms.Show.update - <<'EOF'
match msg {
  Book p -> {
    let ok <- Booking.book model.room.id session.user p.form.from p.form.to
    if ok then Task.succeed (Redirect (Rooms.Show { id: model.room.id })) else Task.succeed (Reject "The room is not free for those dates.")
  },
}
EOF
edited app/Rooms/Show.alder
--- a/app/Rooms/Show.alder
+++ b/app/Rooms/Show.alder
@@ -24,7 +24,10 @@
 
 update : Msg -> Model -> Session -> Task Nav
 update msg model session = match msg {
-  Book p -> ?book,
+  Book p -> {
+    let ok <- Booking.book model.room.id session.user p.form.from p.form.to
+    if ok then Task.succeed (Redirect (Rooms.Show { id: model.room.id })) else Task.succeed (Reject "The room is not free for those dates.")
+  },
 }
 
 view : Model -> Session -> Html Msg
app/Rooms/Show.alder:63:29: warning[W0002]: hole ?expected has type Answer
    Task.succeed (r.answer == ?expected)
                              ^^^^^^^^^
[exit 0]
$ alder fill Rooms.Index.view - --hole ?view <<'EOF'
<Layout.Page title="Rooms">
  <h1>Rooms</h1>
  <ul id="rooms">
    {for r in model.rows -> <li><a class="name" href={Rooms.Show { id: r.room.id }}>{r.room.name}</a> <span class="free">{Date.toText r.free}</span></li>}
  </ul>
  <al-button variant="outline" click={App.Logout}>Log out</al-button>
</Layout.Page>
EOF
edited app/Rooms/Index.alder
--- a/app/Rooms/Index.alder
+++ b/app/Rooms/Index.alder
@@ -1,6 +1,7 @@
 module Rooms.Index
 
 import Booking
+import Layout
 
 page "/rooms" { } requires user
 
@@ -14,7 +15,13 @@
 }
 
 view : Model -> Session -> Html Msg
-view model session = ?view
+view model session = <Layout.Page title="Rooms">
+  <h1>Rooms</h1>
+  <ul id="rooms">
+    {for r in model.rows -> <li><a class="name" href={Rooms.Show { id: r.room.id }}>{r.room.name}</a> <span class="free">{Date.toText r.free}</span></li>}
+  </ul>
+  <al-button variant="outline" click={App.Logout}>Log out</al-button>
+</Layout.Page>
 
 test "a visitor is sent to log in" = {
   let r <- Test.open (Rooms.Index {})
added import Layout to app/Rooms/Index.alder
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
$ alder fill --decl Bookings.Index.Model - <<'EOF'
{ rows: List Booking.MyBooking }
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
+type Model = { rows: List Booking.MyBooking }
 
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
$ alder fill Bookings.Index.load - --hole ?load <<'EOF'
{
  let rows <- Booking.myBookings session.user
  Query.succeed { rows: rows }
}
EOF
edited app/Bookings/Index.alder
--- a/app/Bookings/Index.alder
+++ b/app/Bookings/Index.alder
@@ -7,7 +7,10 @@
 type Model = { rows: List Booking.MyBooking }
 
 load : Params -> Session -> Query Model
-load params session = ?load
+load params session = {
+  let rows <- Booking.myBookings session.user
+  Query.succeed { rows: rows }
+}
 
 view : Model -> Session -> Html Msg
 view model session = ?view
app/Bookings/Index.alder:16:22: warning[W0002]: hole ?view has type Html Bookings.Index.Msg
  view model session = ?view
                       ^^^^^
[exit 0]
$ alder fill Bookings.Index.view - --hole ?view <<'EOF'
<Layout.Page title="Bookings">
  <h1>Bookings</h1>
  <ul id="mine">
    {for b in model.rows -> <li><a href={Bookings.Show { id: b.booking.id }}><span class="room">{b.room.name}</span></a> <span class="from">{Date.toText b.booking.from}</span></li>}
  </ul>
</Layout.Page>
EOF
app/Bookings/Index.alder:20:42: error[E0002]: unknown `Bookings.Show`
      {for b in model.rows -> <li><a href={Bookings.Show { id: b.booking.id }}><span class="room">{b.room.name}</span></a> <span class="from">{Date.toText b.booking.from}</span></li>}
                                           ^^^^^^^^^^^^^
alder fill: the result does not check (the errors above): nothing was written
[exit 1]
```

The harness stopped at line 70 of your script: `alder fill Bookings.Index.view - --hole ?view <<'EOF'`: it exited 1. The commands after it were not run.

Your script stopped at command 13; the app is as the commands before it left it; continue.

The program type-checks but `alder build` refused it; fix the cause below.

```text
app/Auth/Login.alder:86:85: warning[W0002]: hole ?afterLogin has type Page
    Task.succeed ({ answer: r.answer, user: r.session.user } == { answer: Redirected (?afterLogin), user: Just me })
                                                                                      ^^^^^^^^^^^
app/Bookings/Index.alder:16:22: warning[W0002]: hole ?view has type Html Bookings.Index.Msg
  view model session = ?view
                       ^^^^^
app/Rooms/Show.alder:63:29: warning[W0002]: hole ?expected has type Answer
    Task.succeed (r.answer == ?expected)
                              ^^^^^^^^^
alder build: the program has 3 typed hole(s); fill them before building
```
