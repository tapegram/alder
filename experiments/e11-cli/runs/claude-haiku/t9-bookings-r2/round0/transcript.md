### Round 0

```text
$ alder new table User email:Text.Email@unique name:Text.NonEmpty passwordHash:Text created:Time@created --no-timestamps
wrote app/User.alder
wrote migrations/0001_initial.sql
  create table User
wrote migrations/schema.json
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
$ alder new type Booking "RoomPage = { room: Room, bookings: List Booking }"
edited app/Booking.alder
--- a/app/Booking.alder
+++ b/app/Booking.alder
@@ -5,6 +5,8 @@
 
 type BookingId = brand Text
 
+type RoomPage = { room: Room, bookings: List Booking }
+
 table Booking {
   id: BookingId @id,
   room: Room.RoomId @ref Room,
[exit 0]
$ alder new type Booking "FreeRoom = { room: Room, free: Date }"
edited app/Booking.alder
--- a/app/Booking.alder
+++ b/app/Booking.alder
@@ -7,6 +7,8 @@
 
 type RoomPage = { room: Room, bookings: List Booking }
 
+type FreeRoom = { room: Room, free: Date }
+
 table Booking {
   id: BookingId @id,
   room: Room.RoomId @ref Room,
[exit 0]
$ alder new type Booking "MyBooking = { booking: Booking, room: Room }"
edited app/Booking.alder
--- a/app/Booking.alder
+++ b/app/Booking.alder
@@ -9,6 +9,8 @@
 
 type FreeRoom = { room: Room, free: Date }
 
+type MyBooking = { booking: Booking, room: Room }
+
 table Booking {
   id: BookingId @id,
   room: Room.RoomId @ref Room,
[exit 0]
$ alder new type Booking "BookingPage = { booking: Booking, room: Room, cancellable: Bool }"
edited app/Booking.alder
--- a/app/Booking.alder
+++ b/app/Booking.alder
@@ -11,6 +11,8 @@
 
 type MyBooking = { booking: Booking, room: Room }
 
+type BookingPage = { booking: Booking, room: Room, cancellable: Bool }
+
 table Booking {
   id: BookingId @id,
   room: Room.RoomId @ref Room,
[exit 0]
$ alder new decl Booking overlaps "Date -> Date -> Date -> Date -> Bool"
edited app/Booking.alder
--- a/app/Booking.alder
+++ b/app/Booking.alder
@@ -21,6 +21,9 @@
   to: Date,
 }
 
+overlaps : Date -> Date -> Date -> Date -> Bool
+overlaps = ?overlaps
+
 test "a Booking keeps its room from being deleted" = {
   let today <- Date.today
   let target <- Db.insert Room { name: Text.NonEmpty "name-1" }
app/Booking.alder:25:12: warning[W0002]: hole ?overlaps has type Date -> Date -> Date -> Date -> Bool
  overlaps = ?overlaps
             ^^^^^^^^^
[exit 0]
$ alder fill Booking.overlaps - <<'EOF'
\aFrom aTo bFrom bTo -> aFrom < bTo && bFrom < aTo
EOF
edited app/Booking.alder
--- a/app/Booking.alder
+++ b/app/Booking.alder
@@ -22,7 +22,7 @@
 }
 
 overlaps : Date -> Date -> Date -> Date -> Bool
-overlaps = ?overlaps
+overlaps = \aFrom aTo bFrom bTo -> aFrom < bTo && bFrom < aTo
 
 test "a Booking keeps its room from being deleted" = {
   let today <- Date.today
[exit 0]
$ alder new decl Booking stepFree "Booking -> Date -> Date"
edited app/Booking.alder
--- a/app/Booking.alder
+++ b/app/Booking.alder
@@ -24,6 +24,9 @@
 overlaps : Date -> Date -> Date -> Date -> Bool
 overlaps = \aFrom aTo bFrom bTo -> aFrom < bTo && bFrom < aTo
 
+stepFree : Booking -> Date -> Date
+stepFree = ?stepFree
+
 test "a Booking keeps its room from being deleted" = {
   let today <- Date.today
   let target <- Db.insert Room { name: Text.NonEmpty "name-1" }
app/Booking.alder:28:12: warning[W0002]: hole ?stepFree has type Booking -> Date -> Date
  stepFree = ?stepFree
             ^^^^^^^^^
[exit 0]
$ alder fill Booking.stepFree - <<'EOF'
\b cur -> if b.to <= cur then cur else if b.from <= cur then b.to else cur
EOF
edited app/Booking.alder
--- a/app/Booking.alder
+++ b/app/Booking.alder
@@ -25,7 +25,7 @@
 overlaps = \aFrom aTo bFrom bTo -> aFrom < bTo && bFrom < aTo
 
 stepFree : Booking -> Date -> Date
-stepFree = ?stepFree
+stepFree = \b cur -> if b.to <= cur then cur else if b.from <= cur then b.to else cur
 
 test "a Booking keeps its room from being deleted" = {
   let today <- Date.today
[exit 0]
$ alder new decl Booking nextFree "Room.RoomId -> Date -> Query Date"
edited app/Booking.alder
--- a/app/Booking.alder
+++ b/app/Booking.alder
@@ -27,6 +27,9 @@
 stepFree : Booking -> Date -> Date
 stepFree = \b cur -> if b.to <= cur then cur else if b.from <= cur then b.to else cur
 
+nextFree : Room.RoomId -> Date -> Query Date
+nextFree = ?nextFree
+
 test "a Booking keeps its room from being deleted" = {
   let today <- Date.today
   let target <- Db.insert Room { name: Text.NonEmpty "name-1" }
app/Booking.alder:31:12: warning[W0002]: hole ?nextFree has type Room.RoomId -> Date -> Query Date
  nextFree = ?nextFree
             ^^^^^^^^^
[exit 0]
$ alder fill Booking.nextFree - <<'EOF'
\room today -> {
  let bs <- Db.from Booking |> Db.where (Booking.room.eq room) |> Db.orderBy Booking.from.asc |> Db.all
  Query.succeed (List.foldl stepFree today bs)
}
EOF
edited app/Booking.alder
--- a/app/Booking.alder
+++ b/app/Booking.alder
@@ -28,7 +28,10 @@
 stepFree = \b cur -> if b.to <= cur then cur else if b.from <= cur then b.to else cur
 
 nextFree : Room.RoomId -> Date -> Query Date
-nextFree = ?nextFree
+nextFree = \room today -> {
+  let bs <- Db.from Booking |> Db.where (Booking.room.eq room) |> Db.orderBy Booking.from.asc |> Db.all
+  Query.succeed (List.foldl stepFree today bs)
+}
 
 test "a Booking keeps its room from being deleted" = {
   let today <- Date.today
[exit 0]
$ alder new decl Booking freeRow "Date -> Room -> Query FreeRoom"
edited app/Booking.alder
--- a/app/Booking.alder
+++ b/app/Booking.alder
@@ -33,6 +33,9 @@
   Query.succeed (List.foldl stepFree today bs)
 }
 
+freeRow : Date -> Room -> Query FreeRoom
+freeRow = ?freeRow
+
 test "a Booking keeps its room from being deleted" = {
   let today <- Date.today
   let target <- Db.insert Room { name: Text.NonEmpty "name-1" }
app/Booking.alder:37:11: warning[W0002]: hole ?freeRow has type Date -> Room -> Query Booking.FreeRoom
  freeRow = ?freeRow
            ^^^^^^^^
[exit 0]
$ alder fill Booking.freeRow - <<'EOF'
\today r -> {
  let free <- nextFree r.id today
  Query.succeed { room: r, free: free }
}
EOF
edited app/Booking.alder
--- a/app/Booking.alder
+++ b/app/Booking.alder
@@ -34,7 +34,10 @@
 }
 
 freeRow : Date -> Room -> Query FreeRoom
-freeRow = ?freeRow
+freeRow = \today r -> {
+  let free <- nextFree r.id today
+  Query.succeed { room: r, free: free }
+}
 
 test "a Booking keeps its room from being deleted" = {
   let today <- Date.today
[exit 0]
$ alder new decl Booking allFree "Date -> Query (List FreeRoom)"
edited app/Booking.alder
--- a/app/Booking.alder
+++ b/app/Booking.alder
@@ -39,6 +39,9 @@
   Query.succeed { room: r, free: free }
 }
 
+allFree : Date -> Query (List FreeRoom)
+allFree = ?allFree
+
 test "a Booking keeps its room from being deleted" = {
   let today <- Date.today
   let target <- Db.insert Room { name: Text.NonEmpty "name-1" }
app/Booking.alder:43:11: warning[W0002]: hole ?allFree has type Date -> Query (List Booking.FreeRoom)
  allFree = ?allFree
            ^^^^^^^^
[exit 0]
$ alder fill Booking.allFree - <<'EOF'
\today -> {
  let rooms <- Db.from Room |> Db.orderBy Room.name.asc |> Db.all
  Query.all (List.map (freeRow today) rooms)
}
EOF
edited app/Booking.alder
--- a/app/Booking.alder
+++ b/app/Booking.alder
@@ -40,7 +40,10 @@
 }
 
 allFree : Date -> Query (List FreeRoom)
-allFree = ?allFree
+allFree = \today -> {
+  let rooms <- Db.from Room |> Db.orderBy Room.name.asc |> Db.all
+  Query.all (List.map (freeRow today) rooms)
+}
 
 test "a Booking keeps its room from being deleted" = {
   let today <- Date.today
[exit 0]
$ alder new decl Booking conflicts "Room.RoomId -> Date -> Date -> Query Bool"
edited app/Booking.alder
--- a/app/Booking.alder
+++ b/app/Booking.alder
@@ -45,6 +45,9 @@
   Query.all (List.map (freeRow today) rooms)
 }
 
+conflicts : Room.RoomId -> Date -> Date -> Query Bool
+conflicts = ?conflicts
+
 test "a Booking keeps its room from being deleted" = {
   let today <- Date.today
   let target <- Db.insert Room { name: Text.NonEmpty "name-1" }
app/Booking.alder:49:13: warning[W0002]: hole ?conflicts has type Room.RoomId -> Date -> Date -> Query Bool
  conflicts = ?conflicts
              ^^^^^^^^^^
[exit 0]
$ alder fill Booking.conflicts - <<'EOF'
\room start stop -> {
  let bs <- Db.from Booking |> Db.where (Booking.room.eq room) |> Db.all
  Query.succeed (List.any (\b -> overlaps start stop b.from b.to) bs)
}
EOF
edited app/Booking.alder
--- a/app/Booking.alder
+++ b/app/Booking.alder
@@ -46,7 +46,10 @@
 }
 
 conflicts : Room.RoomId -> Date -> Date -> Query Bool
-conflicts = ?conflicts
+conflicts = \room start stop -> {
+  let bs <- Db.from Booking |> Db.where (Booking.room.eq room) |> Db.all
+  Query.succeed (List.any (\b -> overlaps start stop b.from b.to) bs)
+}
 
 test "a Booking keeps its room from being deleted" = {
   let today <- Date.today
[exit 0]
$ alder new decl Booking book "Room.RoomId -> User.UserId -> Date -> Date -> Task Bool"
edited app/Booking.alder
--- a/app/Booking.alder
+++ b/app/Booking.alder
@@ -51,6 +51,9 @@
   Query.succeed (List.any (\b -> overlaps start stop b.from b.to) bs)
 }
 
+book : Room.RoomId -> User.UserId -> Date -> Date -> Task Bool
+book = ?book
+
 test "a Booking keeps its room from being deleted" = {
   let today <- Date.today
   let target <- Db.insert Room { name: Text.NonEmpty "name-1" }
app/Booking.alder:55:8: warning[W0002]: hole ?book has type Room.RoomId -> User.UserId -> Date -> Date -> Task Bool
  book = ?book
         ^^^^^
[exit 0]
$ alder fill Booking.book - <<'EOF'
\room user start stop -> {
  let clash <- conflicts room start stop
  if stop > start && not clash then {
    let _ <- Db.insert Booking { room: room, owner: user, from: start, to: stop }
    Task.succeed True
  } else Task.succeed False
}
EOF
edited app/Booking.alder
--- a/app/Booking.alder
+++ b/app/Booking.alder
@@ -52,7 +52,13 @@
 }
 
 book : Room.RoomId -> User.UserId -> Date -> Date -> Task Bool
-book = ?book
+book = \room user start stop -> {
+  let clash <- conflicts room start stop
+  if stop > start && not clash then {
+    let _ <- Db.insert Booking { room: room, owner: user, from: start, to: stop }
+    Task.succeed True
+  } else Task.succeed False
+}
 
 test "a Booking keeps its room from being deleted" = {
   let today <- Date.today
[exit 0]
$ alder new decl Booking cancellable "Booking -> Date -> Bool"
edited app/Booking.alder
--- a/app/Booking.alder
+++ b/app/Booking.alder
@@ -60,6 +60,9 @@
   } else Task.succeed False
 }
 
+cancellable : Booking -> Date -> Bool
+cancellable = ?cancellable
+
 test "a Booking keeps its room from being deleted" = {
   let today <- Date.today
   let target <- Db.insert Room { name: Text.NonEmpty "name-1" }
app/Booking.alder:64:15: warning[W0002]: hole ?cancellable has type Booking -> Date -> Bool
  cancellable = ?cancellable
                ^^^^^^^^^^^^
[exit 0]
$ alder fill Booking.cancellable - <<'EOF'
\b today -> today < b.from
EOF
edited app/Booking.alder
--- a/app/Booking.alder
+++ b/app/Booking.alder
@@ -61,7 +61,7 @@
 }
 
 cancellable : Booking -> Date -> Bool
-cancellable = ?cancellable
+cancellable = \b today -> today < b.from
 
 test "a Booking keeps its room from being deleted" = {
   let today <- Date.today
[exit 0]
$ alder new decl Booking listed "Booking -> Query MyBooking"
edited app/Booking.alder
--- a/app/Booking.alder
+++ b/app/Booking.alder
@@ -63,6 +63,9 @@
 cancellable : Booking -> Date -> Bool
 cancellable = \b today -> today < b.from
 
+listed : Booking -> Query MyBooking
+listed = ?listed
+
 test "a Booking keeps its room from being deleted" = {
   let today <- Date.today
   let target <- Db.insert Room { name: Text.NonEmpty "name-1" }
app/Booking.alder:67:10: warning[W0002]: hole ?listed has type Booking -> Query Booking.MyBooking
  listed = ?listed
           ^^^^^^^
[exit 0]
$ alder fill Booking.listed - <<'EOF'
\b -> {
  let found <- Db.get Room b.room
  match found {
    Just r -> Query.succeed { booking: b, room: r },
    Nothing -> Query.notFound,
  }
}
EOF
edited app/Booking.alder
--- a/app/Booking.alder
+++ b/app/Booking.alder
@@ -64,7 +64,13 @@
 cancellable = \b today -> today < b.from
 
 listed : Booking -> Query MyBooking
-listed = ?listed
+listed = \b -> {
+  let found <- Db.get Room b.room
+  match found {
+    Just r -> Query.succeed { booking: b, room: r },
+    Nothing -> Query.notFound,
+  }
+}
 
 test "a Booking keeps its room from being deleted" = {
   let today <- Date.today
[exit 0]
$ alder new decl Booking myBookings "User.UserId -> Query (List MyBooking)"
edited app/Booking.alder
--- a/app/Booking.alder
+++ b/app/Booking.alder
@@ -72,6 +72,9 @@
   }
 }
 
+myBookings : User.UserId -> Query (List MyBooking)
+myBookings = ?myBookings
+
 test "a Booking keeps its room from being deleted" = {
   let today <- Date.today
   let target <- Db.insert Room { name: Text.NonEmpty "name-1" }
app/Booking.alder:76:14: warning[W0002]: hole ?myBookings has type User.UserId -> Query (List Booking.MyBooking)
  myBookings = ?myBookings
               ^^^^^^^^^^^
[exit 0]
$ alder fill Booking.myBookings - <<'EOF'
\user -> {
  let bs <- Db.from Booking |> Db.where (Booking.owner.eq user) |> Db.orderBy Booking.from.asc |> Db.all
  Query.all (List.map listed bs)
}
EOF
edited app/Booking.alder
--- a/app/Booking.alder
+++ b/app/Booking.alder
@@ -73,7 +73,10 @@
 }
 
 myBookings : User.UserId -> Query (List MyBooking)
-myBookings = ?myBookings
+myBookings = \user -> {
+  let bs <- Db.from Booking |> Db.where (Booking.owner.eq user) |> Db.orderBy Booking.from.asc |> Db.all
+  Query.all (List.map listed bs)
+}
 
 test "a Booking keeps its room from being deleted" = {
   let today <- Date.today
[exit 0]
$ alder new decl Booking roomPage "Room.RoomId -> Query RoomPage"
edited app/Booking.alder
--- a/app/Booking.alder
+++ b/app/Booking.alder
@@ -78,6 +78,9 @@
   Query.all (List.map listed bs)
 }
 
+roomPage : Room.RoomId -> Query RoomPage
+roomPage = ?roomPage
+
 test "a Booking keeps its room from being deleted" = {
   let today <- Date.today
   let target <- Db.insert Room { name: Text.NonEmpty "name-1" }
app/Booking.alder:82:12: warning[W0002]: hole ?roomPage has type Room.RoomId -> Query Booking.RoomPage
  roomPage = ?roomPage
             ^^^^^^^^^
[exit 0]
$ alder fill Booking.roomPage - <<'EOF'
\room -> {
  let found <- Db.get Room room
  match found {
    Just r -> {
      let bs <- Db.from Booking |> Db.where (Booking.room.eq room) |> Db.orderBy Booking.from.asc |> Db.all
      Query.succeed { room: r, bookings: bs }
    },
    Nothing -> Query.notFound,
  }
}
EOF
edited app/Booking.alder
--- a/app/Booking.alder
+++ b/app/Booking.alder
@@ -79,7 +79,16 @@
 }
 
 roomPage : Room.RoomId -> Query RoomPage
-roomPage = ?roomPage
+roomPage = \room -> {
+  let found <- Db.get Room room
+  match found {
+    Just r -> {
+      let bs <- Db.from Booking |> Db.where (Booking.room.eq room) |> Db.orderBy Booking.from.asc |> Db.all
+      Query.succeed { room: r, bookings: bs }
+    },
+    Nothing -> Query.notFound,
+  }
+}
 
 test "a Booking keeps its room from being deleted" = {
   let today <- Date.today
[exit 0]
$ alder new decl Booking bookingPage "BookingId -> User.UserId -> Query BookingPage"
edited app/Booking.alder
--- a/app/Booking.alder
+++ b/app/Booking.alder
@@ -90,6 +90,9 @@
   }
 }
 
+bookingPage : BookingId -> User.UserId -> Query BookingPage
+bookingPage = ?bookingPage
+
 test "a Booking keeps its room from being deleted" = {
   let today <- Date.today
   let target <- Db.insert Room { name: Text.NonEmpty "name-1" }
app/Booking.alder:94:15: warning[W0002]: hole ?bookingPage has type Booking.BookingId -> User.UserId -> Query Booking.BookingPage
  bookingPage = ?bookingPage
                ^^^^^^^^^^^^
[exit 0]
$ alder fill Booking.bookingPage - <<'EOF'
\bid user -> {
  let found <- Db.get Booking bid
  match found {
    Just b -> {
      let room <- Db.get Room b.room
      let today <- Date.today
      match room {
        Just r -> if b.owner == user then Query.succeed { booking: b, room: r, cancellable: cancellable b today } else Query.notFound,
        Nothing -> Query.notFound,
      }
    },
    Nothing -> Query.notFound,
  }
}
EOF
edited app/Booking.alder
--- a/app/Booking.alder
+++ b/app/Booking.alder
@@ -91,7 +91,20 @@
 }
 
 bookingPage : BookingId -> User.UserId -> Query BookingPage
-bookingPage = ?bookingPage
+bookingPage = \bid user -> {
+  let found <- Db.get Booking bid
+  match found {
+    Just b -> {
+      let room <- Db.get Room b.room
+      let today <- Date.today
+      match room {
+        Just r -> if b.owner == user then Query.succeed { booking: b, room: r, cancellable: cancellable b today } else Query.notFound,
+        Nothing -> Query.notFound,
+      }
+    },
+    Nothing -> Query.notFound,
+  }
+}
 
 test "a Booking keeps its room from being deleted" = {
   let today <- Date.today
[exit 0]
$ alder new test Booking "the next free date skips back-to-back bookings"
edited app/Booking.alder
--- a/app/Booking.alder
+++ b/app/Booking.alder
@@ -125,3 +125,7 @@
   let left <- Db.from Booking |> Db.count
   Task.succeed ({ deleted: Result.toMaybe r.value, left: left } == { deleted: Nothing, left: 1 })
 }
+
+test "the next free date skips back-to-back bookings" = {
+  ?test
+}
app/Booking.alder:130:3: warning[W0002]: hole ?test has type Task Bool
    ?test
    ^^^^^
[exit 0]
$ alder fill 'Booking."the next free date skips back-to-back bookings"' - <<'EOF'
{
  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
  let room <- Db.insert Room { name: Text.NonEmpty "Oak" }
  let today <- Date.today
  let _ <- Db.insert Booking { room: room, owner: me, from: Date.addDays 1 today, to: Date.addDays 3 today }
  let _ <- Db.insert Booking { room: room, owner: me, from: Date.addDays 3 today, to: Date.addDays 5 today }
  let first <- nextFree room today
  let second <- nextFree room (Date.addDays 1 today)
  Task.succeed ({ first: first, second: second } == { first: today, second: Date.addDays 5 today })
}
EOF
edited app/Booking.alder
--- a/app/Booking.alder
+++ b/app/Booking.alder
@@ -127,5 +127,12 @@
 }
 
 test "the next free date skips back-to-back bookings" = {
-  ?test
+  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
+  let room <- Db.insert Room { name: Text.NonEmpty "Oak" }
+  let today <- Date.today
+  let _ <- Db.insert Booking { room: room, owner: me, from: Date.addDays 1 today, to: Date.addDays 3 today }
+  let _ <- Db.insert Booking { room: room, owner: me, from: Date.addDays 3 today, to: Date.addDays 5 today }
+  let first <- nextFree room today
+  let second <- nextFree room (Date.addDays 1 today)
+  Task.succeed ({ first: first, second: second } == { first: today, second: Date.addDays 5 today })
 }
[exit 0]
$ alder fill --decl Layout.Page - <<'EOF'
<html>
  <head>
    <meta charset="utf-8" />
    <title>{props.title}</title>
    <link rel="stylesheet" href={Url.external "/static/al/al.css"} />
  </head>
  <body>
    <al-page width="narrow">
      <main>{props.children}</main>
    </al-page>
  </body>
</html>
EOF
edited app/Layout.alder
--- a/app/Layout.alder
+++ b/app/Layout.alder
@@ -7,7 +7,6 @@
       <meta charset="utf-8" />
       <title>{props.title}</title>
       <link rel="stylesheet" href={Url.external "/static/al/al.css"} />
-      <link rel="stylesheet" href={Url.external "/static/app.css"} />
     </head>
     <body>
       <al-page width="narrow">
[exit 0]
$ alder new page Rooms.Index '/rooms' --requires user --blank
wrote app/Rooms/Index.alder
app/Rooms/Index.alder:8:23: warning[W0002]: hole ?load has type Query Rooms.Index.Model
  load params session = ?load
                        ^^^^^
app/Rooms/Index.alder:11:22: warning[W0002]: hole ?view has type Html Rooms.Index.Msg
  view model session = ?view
                       ^^^^^
[exit 0]
$ alder fill --decl Rooms.Index.Model - <<'EOF'
{ rows: List Booking.FreeRoom }
EOF
edited app/Rooms/Index.alder
--- a/app/Rooms/Index.alder
+++ b/app/Rooms/Index.alder
@@ -1,8 +1,10 @@
 module Rooms.Index
 
+import Booking
+
 page "/rooms" { } requires user
 
-type Model = { }
+type Model = { rows: List Booking.FreeRoom }
 
 load : Params -> Session -> Query Model
 load params session = ?load
app/Rooms/Index.alder:10:23: warning[W0002]: hole ?load has type Query Rooms.Index.Model
  load params session = ?load
                        ^^^^^
app/Rooms/Index.alder:13:22: warning[W0002]: hole ?view has type Html Rooms.Index.Msg
  view model session = ?view
                       ^^^^^
added import Booking to app/Rooms/Index.alder
[exit 0]
$ alder fill Rooms.Index.load - --hole ?load <<'EOF'
{
  let today <- Date.today
  let rows <- Booking.allFree today
  Query.succeed { rows: rows }
}
EOF
edited app/Rooms/Index.alder
--- a/app/Rooms/Index.alder
+++ b/app/Rooms/Index.alder
@@ -7,7 +7,11 @@
 type Model = { rows: List Booking.FreeRoom }
 
 load : Params -> Session -> Query Model
-load params session = ?load
+load params session = {
+  let today <- Date.today
+  let rows <- Booking.allFree today
+  Query.succeed { rows: rows }
+}
 
 view : Model -> Session -> Html Msg
 view model session = ?view
app/Rooms/Index.alder:17:22: warning[W0002]: hole ?view has type Html Rooms.Index.Msg
  view model session = ?view
                       ^^^^^
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
app/Rooms/Index.alder:21:55: error[E0002]: unknown `Rooms.Show`
      {for r in model.rows -> <li><a class="name" href={Rooms.Show { id: r.room.id }}>{r.room.name}</a> <span class="free">{Date.toText r.free}</span></li>}
                                                        ^^^^^^^^^^
alder fill: the result does not check (the errors above): nothing was written
[exit 1]
```

The harness stopped at line 166 of your script: `alder fill Rooms.Index.view - --hole ?view <<'EOF'`: it exited 1. The commands after it were not run.

Your script stopped at command 39; the app is as the commands before it left it; continue.

The program type-checks but `alder build` refused it; fix the cause below.

```text
app/Auth/Login.alder:47:33: warning[W0002]: hole ?afterLogin has type Page
          Task.succeed (Redirect (?afterLogin))
                                  ^^^^^^^^^^^
app/Auth/Login.alder:86:85: warning[W0002]: hole ?afterLogin has type Page
    Task.succeed ({ answer: r.answer, user: r.session.user } == { answer: Redirected (?afterLogin), user: Just me })
                                                                                      ^^^^^^^^^^^
app/Rooms/Index.alder:17:22: warning[W0002]: hole ?view has type Html Rooms.Index.Msg
  view model session = ?view
                       ^^^^^
alder build: the program has 3 typed hole(s); fill them before building
```
