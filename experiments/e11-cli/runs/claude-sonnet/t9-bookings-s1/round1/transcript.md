### Round 1

```text
$ alder new page Rooms.Show '/rooms/{id}' --param id:Room.RoomId --blank --requires user
wrote app/Rooms/Show.alder
app/Rooms/Show.alder:10:23: warning[W0002]: hole ?load has type Query Rooms.Show.Model
  load params session = ?load
                        ^^^^^
app/Rooms/Show.alder:13:22: warning[W0002]: hole ?view has type Html Rooms.Show.Msg
  view model session = ?view
                       ^^^^^
[exit 0]
$ alder new page Bookings.Index /bookings --blank --requires user
wrote app/Bookings/Index.alder
app/Bookings/Index.alder:8:23: warning[W0002]: hole ?load has type Query Bookings.Index.Model
  load params session = ?load
                        ^^^^^
app/Bookings/Index.alder:11:22: warning[W0002]: hole ?view has type Html Bookings.Index.Msg
  view model session = ?view
                       ^^^^^
[exit 0]
$ alder new page Bookings.Show '/bookings/{id}' --param id:Booking.BookingId --blank --requires user
wrote app/Bookings/Show.alder
app/Bookings/Show.alder:12:23: warning[W0002]: hole ?load has type Query Bookings.Show.Model
  load params session = ?load
                        ^^^^^
app/Bookings/Show.alder:15:22: warning[W0002]: hole ?view has type Html Bookings.Show.Msg
  view model session = ?view
                       ^^^^^
[exit 0]
$ alder fill Rooms.Index.view - <<'EOF'
<Layout.Page title="Rooms">
  <h1>Rooms</h1>
  <ul id="rooms">
    {for r in model.rooms ->
      <li><a class="name" href={Rooms.Show { id: r.id }}>{r.name}</a> <span class="free">{Date.toText r.free}</span></li>}
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
@@ -16,7 +17,13 @@
 }
 
 view : Model -> Session -> Html Msg
-view model session = ?view
+view model session = <Layout.Page title="Rooms">
+  <h1>Rooms</h1>
+  <ul id="rooms">
+    {for r in model.rooms ->
+      <li><a class="name" href={Rooms.Show { id: r.id }}>{r.name}</a> <span class="free">{Date.toText r.free}</span></li>}
+  </ul>
+</Layout.Page>
 
 test "a visitor is sent to log in" = {
   let r <- Test.open (Rooms.Index {})
added import Layout to app/Rooms/Index.alder
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
$ alder fill Rooms.Show.load - <<'EOF'
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
edited app/Rooms/Show.alder
--- a/app/Rooms/Show.alder
+++ b/app/Rooms/Show.alder
@@ -8,7 +8,16 @@
 type Model = { room: Room, bookings: List Booking }
 
 load : Params -> Session -> Query Model
-load params session = ?load
+load params session = {
+  let found <- Db.get Room params.id
+  match found {
+    Just r -> {
+      let bs <- Db.from Booking |> Db.where (Booking.room.eq r.id) |> Db.orderBy Booking.from.asc |> Db.all
+      Query.succeed { room: r, bookings: bs }
+    },
+    Nothing -> Query.notFound,
+  }
+}
 
 view : Model -> Session -> Html Msg
 view model session = ?view
app/Rooms/Show.alder:23:22: warning[W0002]: hole ?view has type Html Rooms.Show.Msg
  view model session = ?view
                       ^^^^^
[exit 0]
$ alder fill Rooms.Show.view - <<'EOF'
<Layout.Page title="Room">
  <h1>Room</h1>
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
+  <h1>Room</h1>
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
   <h1>Room</h1>
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
$ alder fill --decl Rooms.Show.update - <<'EOF'
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
app/Rooms/Show.alder:32:28: error[E0003]: expected Task Nav, got Rooms.Show.Msg -> _ -> _ -> Query _
  update msg model session = \msg model session -> match msg {
                             ^
app/Rooms/Show.alder:34:66: error[E0003]: cannot infer the record type of `model`; add a type annotation
      let existing <- Db.from Booking |> Db.where (Booking.room.eq model.room.id) |> Db.all
                                                                   ^^^^^^^^^^
app/Rooms/Show.alder:34:66: error[E0003]: cannot infer the record type of `model.room`; add a type annotation
      let existing <- Db.from Booking |> Db.where (Booking.room.eq model.room.id) |> Db.all
                                                                   ^^^^^^^^^^^^^
app/Rooms/Show.alder:35:38: error[E0003]: expected Query _, got Task Nav
      if p.form.to <= p.form.from then Task.succeed (Reject "The end date must be after the start date.")
                                       ^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^
app/Rooms/Show.alder:36:67: error[E0003]: expected Query _, got Task Nav
      else if Booking.conflicts p.form.from p.form.to existing then Task.succeed (Reject "That room is already booked in that period.")
                                                                    ^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^
app/Rooms/Show.alder:38:16: error[E0401]: `Task` cannot run inside a `Query`; `load` and `Query` blocks can only read
        let _ <- Db.insert Booking { room: model.room.id, owner: session.user, from: p.form.from, to: p.form.to }
                 ^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^
app/Rooms/Show.alder:38:42: error[E0003]: cannot infer the record type of `model`; add a type annotation
        let _ <- Db.insert Booking { room: model.room.id, owner: session.user, from: p.form.from, to: p.form.to }
                                           ^^^^^^^^^^
app/Rooms/Show.alder:38:42: error[E0003]: cannot infer the record type of `model.room`; add a type annotation
        let _ <- Db.insert Booking { room: model.room.id, owner: session.user, from: p.form.from, to: p.form.to }
                                           ^^^^^^^^^^^^^
app/Rooms/Show.alder:38:64: error[E0003]: cannot infer the record type of `session`; add a type annotation
        let _ <- Db.insert Booking { room: model.room.id, owner: session.user, from: p.form.from, to: p.form.to }
                                                                 ^^^^^^^^^^^^
app/Rooms/Show.alder:39:7: error[E0003]: expected Query _, got Task Nav
        Task.succeed (Redirect (Rooms.Show { id: model.room.id }))
        ^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^
app/Rooms/Show.alder:39:48: error[E0003]: cannot infer the record type of `model`; add a type annotation
        Task.succeed (Redirect (Rooms.Show { id: model.room.id }))
                                                 ^^^^^^^^^^
app/Rooms/Show.alder:39:48: error[E0003]: cannot infer the record type of `model.room`; add a type annotation
        Task.succeed (Redirect (Rooms.Show { id: model.room.id }))
                                                 ^^^^^^^^^^^^^
app/Rooms/Show.alder:72:29: warning[W0002]: hole ?expected has type Answer
    Task.succeed (r.answer == ?expected)
                              ^^^^^^^^^
alder fill: the result does not check (the errors above): nothing was written
[exit 1]
```

The harness stopped at line 39 of your script: `alder fill --decl Rooms.Show.update - <<'EOF'`: it exited 1. The commands after it were not run.

Your script stopped at command 9; the app is as the commands before it left it; continue.

The program type-checks but `alder build` refused it; fix the cause below.

```text
app/Auth/Login.alder:47:33: warning[W0002]: hole ?afterLogin has type Page
          Task.succeed (Redirect (?afterLogin))
                                  ^^^^^^^^^^^
app/Auth/Login.alder:86:85: warning[W0002]: hole ?afterLogin has type Page
    Task.succeed ({ answer: r.answer, user: r.session.user } == { answer: Redirected (?afterLogin), user: Just me })
                                                                                      ^^^^^^^^^^^
app/Bookings/Index.alder:8:23: warning[W0002]: hole ?load has type Query Bookings.Index.Model
  load params session = ?load
                        ^^^^^
app/Bookings/Index.alder:11:22: warning[W0002]: hole ?view has type Html Bookings.Index.Msg
  view model session = ?view
                       ^^^^^
app/Bookings/Show.alder:12:23: warning[W0002]: hole ?load has type Query Bookings.Show.Model
  load params session = ?load
                        ^^^^^
app/Bookings/Show.alder:15:22: warning[W0002]: hole ?view has type Html Bookings.Show.Msg
  view model session = ?view
                       ^^^^^
app/Rooms/Show.alder:33:13: warning[W0002]: hole ?book has type Task Nav
    Book p -> ?book,
              ^^^^^
app/Rooms/Show.alder:64:29: warning[W0002]: hole ?expected has type Answer
    Task.succeed (r.answer == ?expected)
                              ^^^^^^^^^
alder build: the program has 8 typed hole(s); fill them before building
```
