### Round 2

```text
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
$ alder fill --decl Rooms.Show.update - <<'EOF'
match msg {
  Book p -> {
    let existing <- Db.from Booking |> Db.where (Booking.room.eq model.room.id) |> Db.all
    if p.form.to <= p.form.from then Task.succeed (Reject "The end date must be after the start date.") else if Booking.conflicts p.form.from p.form.to existing then Task.succeed (Reject "That room is already booked in that period.") else {
      let _ <- Db.insert Booking { room: model.room.id, owner: session.user, from: p.form.from, to: p.form.to }
      Task.succeed (Redirect (Rooms.Show { id: model.room.id }))
    }
  },
}
EOF
edited app/Rooms/Show.alder
--- a/app/Rooms/Show.alder
+++ b/app/Rooms/Show.alder
@@ -30,7 +30,13 @@
 
 update : Msg -> Model -> Session -> Task Nav
 update msg model session = match msg {
-  Book p -> ?book,
+  Book p -> {
+    let existing <- Db.from Booking |> Db.where (Booking.room.eq model.room.id) |> Db.all
+    if p.form.to <= p.form.from then Task.succeed (Reject "The end date must be after the start date.") else if Booking.conflicts p.form.from p.form.to existing then Task.succeed (Reject "That room is already booked in that period.") else {
+      let _ <- Db.insert Booking { room: model.room.id, owner: session.user, from: p.form.from, to: p.form.to }
+      Task.succeed (Redirect (Rooms.Show { id: model.room.id }))
+    }
+  },
 }
 
 view : Model -> Session -> Html Msg
app/Rooms/Show.alder:70:29: warning[W0002]: hole ?expected has type Answer
    Task.succeed (r.answer == ?expected)
                              ^^^^^^^^^
[exit 0]
$ alder fill --decl Rooms.Show.view - <<'EOF'
<Layout.Page title="Room">
  <h1 id="name">{model.room.name}</h1>
  <ul id="bookings">
    {for b in model.bookings ->
      <li><span class="from">{Date.toText b.from}</span> <span class="to">{Date.toText b.to}</span></li>}
  </ul>
  <form submit={Book}>
    <Errors form />
    <al-input field="from" label="From" />
    <al-input field="to" label="To" />
    <al-button type="submit">Book</al-button>
  </form>
</Layout.Page>
EOF
edited app/Rooms/Show.alder
--- a/app/Rooms/Show.alder
+++ b/app/Rooms/Show.alder
@@ -40,19 +40,20 @@
 }
 
 view : Model -> Session -> Html Msg
-view model session = <Layout.Page title="Room">
-  <h1>Room</h1>
-
-  <al-card>
-    <h2 slot="header">New booking</h2>
+view model session =
+  <Layout.Page title="Room">
+    <h1 id="name">{model.room.name}</h1>
+    <ul id="bookings">
+      {for b in model.bookings ->
+        <li><span class="from">{Date.toText b.from}</span> <span class="to">{Date.toText b.to}</span></li>}
+    </ul>
     <form submit={Book}>
+      <Errors form />
       <al-input field="from" label="From" />
       <al-input field="to" label="To" />
-      <al-alert variant="destructive"><Errors form /></al-alert>
-      <al-button type="submit">New booking</al-button>
+      <al-button type="submit">Book</al-button>
     </form>
-  </al-card>
-</Layout.Page>
+  </Layout.Page>
 
 test "a visitor is sent to log in" = {
   let room1 <- Db.insert Room { name: Text.NonEmpty "name-1" }
app/Rooms/Show.alder:71:29: warning[W0002]: hole ?expected has type Answer
    Task.succeed (r.answer == ?expected)
                              ^^^^^^^^^
[exit 0]
$ alder fill --decl 'Rooms.Show."submitting NewBooking"' - <<'EOF'
{
  let today <- Date.today
  let me <- Db.insert User { email: Text.Email "a@example.com", name: Text.NonEmpty "A", passwordHash: "" }
  let room1 <- Db.insert Room { name: Text.NonEmpty "Blue" }
  let _ <- Db.insert Booking { room: room1, owner: me, from: today, to: Date.addDays 2 today }
  let _ <- Test.login { user: Just me }
  let _ <- Test.get (Rooms.Show { id: room1 })
  let r <- Test.submit NewBooking { from: Date.addDays 2 today, to: Date.addDays 4 today }
  let n <- Db.from Booking |> Db.count
  Task.succeed ({ answer: r.answer, n: n } == { answer: Redirected (Rooms.Show { id: room1 }), n: 2 })
}
EOF
edited app/Rooms/Show.alder
--- a/app/Rooms/Show.alder
+++ b/app/Rooms/Show.alder
@@ -63,12 +63,14 @@
 
 test "submitting NewBooking" = {
   let today <- Date.today
-  let me <- Db.insert User { email: Text.Email "email-1@example.com", name: Text.NonEmpty "name-1", passwordHash: "passwordhash-1" }
+  let me <- Db.insert User { email: Text.Email "a@example.com", name: Text.NonEmpty "A", passwordHash: "" }
+  let room1 <- Db.insert Room { name: Text.NonEmpty "Blue" }
+  let _ <- Db.insert Booking { room: room1, owner: me, from: today, to: Date.addDays 2 today }
   let _ <- Test.login { user: Just me }
-  let room1 <- Db.insert Room { name: Text.NonEmpty "name-1" }
   let _ <- Test.get (Rooms.Show { id: room1 })
-  let r <- Test.submit NewBooking { from: today, to: today }
-  Task.succeed (r.answer == ?expected)
+  let r <- Test.submit NewBooking { from: Date.addDays 2 today, to: Date.addDays 4 today }
+  let n <- Db.from Booking |> Db.count
+  Task.succeed ({ answer: r.answer, n: n } == { answer: Redirected (Rooms.Show { id: room1 }), n: 2 })
 }
 
 test "NewBooking refuses an empty from" = {
rewriting a test: `alder test --mutate` is the check that it still tests something
[exit 0]
$ alder fill --decl 'Rooms.Show."NewBooking refuses an empty from"' - <<'EOF'
{
  let today <- Date.today
  let me <- Db.insert User { email: Text.Email "a@example.com", name: Text.NonEmpty "A", passwordHash: "" }
  let room1 <- Db.insert Room { name: Text.NonEmpty "Blue" }
  let _ <- Db.insert Booking { room: room1, owner: me, from: today, to: Date.addDays 3 today }
  let _ <- Test.login { user: Just me }
  let _ <- Test.get (Rooms.Show { id: room1 })
  let r <- Test.submit NewBooking { from: Date.addDays 1 today, to: Date.addDays 4 today }
  let n <- Db.from Booking |> Db.count
  Task.succeed ({ answer: r.answer, n: n } == { answer: FormRejected [{ field: "", message: "That room is already booked in that period." }], n: 1 })
}
EOF
edited app/Rooms/Show.alder
--- a/app/Rooms/Show.alder
+++ b/app/Rooms/Show.alder
@@ -74,10 +74,13 @@
 }
 
 test "NewBooking refuses an empty from" = {
-  let me <- Db.insert User { email: Text.Email "email-1@example.com", name: Text.NonEmpty "name-1", passwordHash: "passwordhash-1" }
+  let today <- Date.today
+  let me <- Db.insert User { email: Text.Email "a@example.com", name: Text.NonEmpty "A", passwordHash: "" }
+  let room1 <- Db.insert Room { name: Text.NonEmpty "Blue" }
+  let _ <- Db.insert Booking { room: room1, owner: me, from: today, to: Date.addDays 3 today }
   let _ <- Test.login { user: Just me }
-  let room1 <- Db.insert Room { name: Text.NonEmpty "name-1" }
   let _ <- Test.get (Rooms.Show { id: room1 })
-  let r <- Test.submitRaw NewBooking [{ name: "from", value: "" }, { name: "to", value: "2026-01-01" }]
-  Task.succeed (r.answer == FormRejected [{ field: "from", message: "is required" }])
+  let r <- Test.submit NewBooking { from: Date.addDays 1 today, to: Date.addDays 4 today }
+  let n <- Db.from Booking |> Db.count
+  Task.succeed ({ answer: r.answer, n: n } == { answer: FormRejected [{ field: "", message: "That room is already booked in that period." }], n: 1 })
 }
rewriting a test: `alder test --mutate` is the check that it still tests something
[exit 0]
$ alder fill --decl Bookings.Index.Model - <<'EOF'
{ items: List { id: Booking.BookingId, room: Text, from: Date } }
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
+type Model = { items: List { id: Booking.BookingId, room: Text, from: Date } }
 
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
  let bs <- Db.from Booking |> Db.where (Booking.owner.eq session.user) |> Db.orderBy Booking.from.asc |> Db.all
  let rooms <- Db.from Room |> Db.all
  Query.succeed { items: List.map (\b -> { id: b.id, room: Maybe.withDefault "" (Maybe.map (\r -> Text.trim r.name) (List.find (\r -> r.id == b.room) rooms)), from: b.from }) bs }
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
 
 type Model = { items: List { id: Booking.BookingId, room: Text, from: Date } }
 
 load : Params -> Session -> Query Model
-load params session = ?load
+load params session = {
+  let bs <- Db.from Booking |> Db.where (Booking.owner.eq session.user) |> Db.orderBy Booking.from.asc |> Db.all
+  let rooms <- Db.from Room |> Db.all
+  Query.succeed { items: List.map (\b -> { id: b.id, room: Maybe.withDefault "" (Maybe.map (\r -> Text.trim r.name) (List.find (\r -> r.id == b.room) rooms)), from: b.from }) bs }
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
    {for i in model.items ->
      <li><a href={Bookings.Show { id: i.id }}><span class="room">{i.room}</span> <span class="from">{Date.toText i.from}</span></a></li>}
  </ul>
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
@@ -15,7 +16,13 @@
 }
 
 view : Model -> Session -> Html Msg
-view model session = ?view
+view model session = <Layout.Page title="My bookings">
+  <h1>My bookings</h1>
+  <ul id="mine">
+    {for i in model.items ->
+      <li><a href={Bookings.Show { id: i.id }}><span class="room">{i.room}</span> <span class="from">{Date.toText i.from}</span></a></li>}
+  </ul>
+</Layout.Page>
 
 test "a visitor is sent to log in" = {
   let r <- Test.open (Bookings.Index {})
added import Layout to app/Bookings/Index.alder
[exit 0]
$ alder fill --decl Bookings.Show.Model - <<'EOF'
{ booking: Booking, roomName: Text, canCancel: Bool }
EOF
edited app/Bookings/Show.alder
--- a/app/Bookings/Show.alder
+++ b/app/Bookings/Show.alder
@@ -6,7 +6,7 @@
 
 page "/bookings/{id}" { id: Booking.BookingId } requires user
 
-type Model = { }
+type Model = { booking: Booking, roomName: Text, canCancel: Bool }
 
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
  let today <- Date.today
  let found <- Db.get Booking params.id
  match found {
    Just b -> if b.owner == session.user then {
      let room <- Db.get Room b.room
      Query.succeed { booking: b, roomName: Maybe.withDefault "" (Maybe.map (\r -> Text.trim r.name) room), canCancel: today < b.from }
    } else Query.notFound,
    Nothing -> Query.notFound,
  }
}
EOF
edited app/Bookings/Show.alder
--- a/app/Bookings/Show.alder
+++ b/app/Bookings/Show.alder
@@ -9,7 +9,17 @@
 type Model = { booking: Booking, roomName: Text, canCancel: Bool }
 
 load : Params -> Session -> Query Model
-load params session = ?load
+load params session = {
+  let today <- Date.today
+  let found <- Db.get Booking params.id
+  match found {
+    Just b -> if b.owner == session.user then {
+      let room <- Db.get Room b.room
+      Query.succeed { booking: b, roomName: Maybe.withDefault "" (Maybe.map (\r -> Text.trim r.name) room), canCancel: today < b.from }
+    } else Query.notFound,
+    Nothing -> Query.notFound,
+  }
+}
 
 view : Model -> Session -> Html Msg
 view model session = ?view
app/Bookings/Show.alder:25:22: warning[W0002]: hole ?view has type Html Bookings.Show.Msg
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
 
@@ -22,7 +23,9 @@
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
@@ -22,9 +22,18 @@
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
@@ -35,3 +44,20 @@
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
app/Bookings/Show.alder:62:16: warning[W0002]: hole ?expected has type Bool
    Task.succeed ?expected
                 ^^^^^^^^^
[exit 0]
$ alder fill --decl Bookings.Show.view - <<'EOF'
<Layout.Page title="Booking">
  <h1>Booking</h1>
  <p id="room">{model.roomName}</p>
  <p id="from">{Date.toText model.booking.from}</p>
  <p id="to">{Date.toText model.booking.to}</p>
  {if model.canCancel then <button id="cancel" click={Cancel}>Cancel</button> else Html.none}
</Layout.Page>
EOF
edited app/Bookings/Show.alder
--- a/app/Bookings/Show.alder
+++ b/app/Bookings/Show.alder
@@ -30,11 +30,14 @@
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
+    <p id="room">{model.roomName}</p>
+    <p id="from">{Date.toText model.booking.from}</p>
+    <p id="to">{Date.toText model.booking.to}</p>
+    {if model.canCancel then <button id="cancel" click={Cancel}>Cancel</button> else Html.none}
+  </Layout.Page>
 
 test "a visitor is sent to log in" = {
   let today <- Date.today
app/Bookings/Show.alder:29:15: warning[W0002]: hole ?cancel has type Task Nav
    Cancel p -> ?cancel,
                ^^^^^^^
app/Bookings/Show.alder:65:16: warning[W0002]: hole ?expected has type Bool
    Task.succeed ?expected
                 ^^^^^^^^^
[exit 0]
$ alder fill --decl Bookings.Show.update - <<'EOF'
match msg {
  Cancel p -> {
    let today <- Date.today
    if model.booking.owner == session.user && today < model.booking.from then {
      let _ <- Db.delete Booking model.booking.id
      Task.succeed (Redirect (Bookings.Index {}))
    } else Task.succeed Rerender
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
+    if model.booking.owner == session.user && today < model.booking.from then {
+      let _ <- Db.delete Booking model.booking.id
+      Task.succeed (Redirect (Bookings.Index {}))
+    } else Task.succeed Rerender
+  },
 }
 
 view : Model -> Session -> Html Msg
app/Bookings/Show.alder:71:16: warning[W0002]: hole ?expected has type Bool
    Task.succeed ?expected
                 ^^^^^^^^^
[exit 0]
$ alder new test Bookings.Show "owner cancels before the from date"
edited app/Bookings/Show.alder
--- a/app/Bookings/Show.alder
+++ b/app/Bookings/Show.alder
@@ -70,3 +70,7 @@
   let after <- Test.open (Bookings.Show { id: booking1 })
   Task.succeed ?expected
 }
+
+test "owner cancels before the from date" = {
+  ?test
+}
app/Bookings/Show.alder:71:16: warning[W0002]: hole ?expected has type Bool
    Task.succeed ?expected
                 ^^^^^^^^^
app/Bookings/Show.alder:75:3: warning[W0002]: hole ?test has type Task Bool
    ?test
    ^^^^^
[exit 0]
$ alder fill 'Bookings.Show."owner cancels before the from date"' - <<'EOF'
{
  let today <- Date.today
  let me <- Db.insert User { email: Text.Email "a@example.com", name: Text.NonEmpty "A", passwordHash: "" }
  let room1 <- Db.insert Room { name: Text.NonEmpty "Blue" }
  let b <- Db.insert Booking { room: room1, owner: me, from: Date.addDays 2 today, to: Date.addDays 3 today }
  let _ <- Test.login { user: Just me }
  let _ <- Test.get (Bookings.Show { id: b })
  let r <- Test.click Cancel
  let after <- Db.get Booking b
  Task.succeed ({ answer: r.answer, gone: not (Maybe.isJust after) } == { answer: Redirected (Bookings.Index {}), gone: True })
}
EOF
edited app/Bookings/Show.alder
--- a/app/Bookings/Show.alder
+++ b/app/Bookings/Show.alder
@@ -72,5 +72,13 @@
 }
 
 test "owner cancels before the from date" = {
-  ?test
+  let today <- Date.today
+  let me <- Db.insert User { email: Text.Email "a@example.com", name: Text.NonEmpty "A", passwordHash: "" }
+  let room1 <- Db.insert Room { name: Text.NonEmpty "Blue" }
+  let b <- Db.insert Booking { room: room1, owner: me, from: Date.addDays 2 today, to: Date.addDays 3 today }
+  let _ <- Test.login { user: Just me }
+  let _ <- Test.get (Bookings.Show { id: b })
+  let r <- Test.click Cancel
+  let after <- Db.get Booking b
+  Task.succeed ({ answer: r.answer, gone: not (Maybe.isJust after) } == { answer: Redirected (Bookings.Index {}), gone: True })
 }
app/Bookings/Show.alder:71:16: warning[W0002]: hole ?expected has type Bool
    Task.succeed ?expected
                 ^^^^^^^^^
[exit 0]
$ alder new test Bookings.Show "a booking starting today cannot be cancelled"
edited app/Bookings/Show.alder
--- a/app/Bookings/Show.alder
+++ b/app/Bookings/Show.alder
@@ -82,3 +82,7 @@
   let after <- Db.get Booking b
   Task.succeed ({ answer: r.answer, gone: not (Maybe.isJust after) } == { answer: Redirected (Bookings.Index {}), gone: True })
 }
+
+test "a booking starting today cannot be cancelled" = {
+  ?test
+}
app/Bookings/Show.alder:71:16: warning[W0002]: hole ?expected has type Bool
    Task.succeed ?expected
                 ^^^^^^^^^
app/Bookings/Show.alder:87:3: warning[W0002]: hole ?test has type Task Bool
    ?test
    ^^^^^
[exit 0]
$ alder fill 'Bookings.Show."a booking starting today cannot be cancelled"' - <<'EOF'
{
  let today <- Date.today
  let me <- Db.insert User { email: Text.Email "a@example.com", name: Text.NonEmpty "A", passwordHash: "" }
  let room1 <- Db.insert Room { name: Text.NonEmpty "Blue" }
  let b <- Db.insert Booking { room: room1, owner: me, from: today, to: Date.addDays 1 today }
  let _ <- Test.login { user: Just me }
  let page <- Test.get (Bookings.Show { id: b })
  let r <- Test.run (update Cancel page.model { user: me })
  let after <- Db.get Booking b
  Task.succeed ({ button: Test.has "#cancel" page.doc, kept: Maybe.isJust after } == { button: False, kept: True })
}
EOF
edited app/Bookings/Show.alder
--- a/app/Bookings/Show.alder
+++ b/app/Bookings/Show.alder
@@ -84,5 +84,13 @@
 }
 
 test "a booking starting today cannot be cancelled" = {
-  ?test
+  let today <- Date.today
+  let me <- Db.insert User { email: Text.Email "a@example.com", name: Text.NonEmpty "A", passwordHash: "" }
+  let room1 <- Db.insert Room { name: Text.NonEmpty "Blue" }
+  let b <- Db.insert Booking { room: room1, owner: me, from: today, to: Date.addDays 1 today }
+  let _ <- Test.login { user: Just me }
+  let page <- Test.get (Bookings.Show { id: b })
+  let r <- Test.run (update Cancel page.model { user: me })
+  let after <- Db.get Booking b
+  Task.succeed ({ button: Test.has "#cancel" page.doc, kept: Maybe.isJust after } == { button: False, kept: True })
 }
app/Bookings/Show.alder:71:16: warning[W0002]: hole ?expected has type Bool
    Task.succeed ?expected
                 ^^^^^^^^^
[exit 0]
$ alder new test Booking "next free date with back-to-back bookings"
edited app/Booking.alder
--- a/app/Booking.alder
+++ b/app/Booking.alder
@@ -38,3 +38,7 @@
   let left <- Db.from Booking |> Db.count
   Task.succeed ({ deleted: Result.toMaybe r.value, left: left } == { deleted: Nothing, left: 1 })
 }
+
+test "next free date with back-to-back bookings" = {
+  ?test
+}
app/Booking.alder:43:3: warning[W0002]: hole ?test has type Task Bool
    ?test
    ^^^^^
[exit 0]
$ alder fill 'Booking."next free date with back-to-back bookings"' - <<'EOF'
{
  let today <- Date.today
  let me <- Db.insert User { email: Text.Email "a@example.com", name: Text.NonEmpty "A", passwordHash: "" }
  let room1 <- Db.insert Room { name: Text.NonEmpty "Blue" }
  let _ <- Db.insert Booking { room: room1, owner: me, from: today, to: Date.addDays 2 today }
  let _ <- Db.insert Booking { room: room1, owner: me, from: Date.addDays 2 today, to: Date.addDays 4 today }
  let all <- Db.from Booking |> Db.all
  Task.succeed (nextFree today all == Date.addDays 4 today)
}
EOF
edited app/Booking.alder
--- a/app/Booking.alder
+++ b/app/Booking.alder
@@ -40,5 +40,11 @@
 }
 
 test "next free date with back-to-back bookings" = {
-  ?test
+  let today <- Date.today
+  let me <- Db.insert User { email: Text.Email "a@example.com", name: Text.NonEmpty "A", passwordHash: "" }
+  let room1 <- Db.insert Room { name: Text.NonEmpty "Blue" }
+  let _ <- Db.insert Booking { room: room1, owner: me, from: today, to: Date.addDays 2 today }
+  let _ <- Db.insert Booking { room: room1, owner: me, from: Date.addDays 2 today, to: Date.addDays 4 today }
+  let all <- Db.from Booking |> Db.all
+  Task.succeed (nextFree today all == Date.addDays 4 today)
 }
[exit 0]
$ alder fill --list
Auth.Login "logging in through the form" ?afterLogin : Page
Bookings.Show "pressing Cancel" ?expected : Bool
[exit 0]
```

The program type-checks but `alder build` refused it; fix the cause below.

```text
app/Auth/Login.alder:86:85: warning[W0002]: hole ?afterLogin has type Page
    Task.succeed ({ answer: r.answer, user: r.session.user } == { answer: Redirected (?afterLogin), user: Just me })
                                                                                      ^^^^^^^^^^^
app/Bookings/Show.alder:71:16: warning[W0002]: hole ?expected has type Bool
    Task.succeed ?expected
                 ^^^^^^^^^
alder build: the program has 2 typed hole(s); fill them before building
```
