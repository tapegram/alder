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
$ alder new type Booking "RoomFree = { room: Room, free: Date }"
edited app/Booking.alder
--- a/app/Booking.alder
+++ b/app/Booking.alder
@@ -5,6 +5,8 @@
 
 type BookingId = brand Text
 
+type RoomFree = { room: Room, free: Date }
+
 table Booking {
   id: BookingId @id,
   room: Room.RoomId @ref Room,
[exit 0]
$ alder new type Booking "RoomPage = { room: Room, bookings: List Booking }"
edited app/Booking.alder
--- a/app/Booking.alder
+++ b/app/Booking.alder
@@ -7,6 +7,8 @@
 
 type RoomFree = { room: Room, free: Date }
 
+type RoomPage = { room: Room, bookings: List Booking }
+
 table Booking {
   id: BookingId @id,
   room: Room.RoomId @ref Room,
[exit 0]
$ alder new type Booking "Line = { booking: Booking, room: Room }"
edited app/Booking.alder
--- a/app/Booking.alder
+++ b/app/Booking.alder
@@ -9,6 +9,8 @@
 
 type RoomPage = { room: Room, bookings: List Booking }
 
+type Line = { booking: Booking, room: Room }
+
 table Booking {
   id: BookingId @id,
   room: Room.RoomId @ref Room,
[exit 0]
$ alder new decl Booking nextFree "Date -> List Booking -> Date"
edited app/Booking.alder
--- a/app/Booking.alder
+++ b/app/Booking.alder
@@ -19,6 +19,9 @@
   to: Date,
 }
 
+nextFree : Date -> List Booking -> Date
+nextFree = ?nextFree
+
 test "a Booking keeps its room from being deleted" = {
   let today <- Date.today
   let target <- Db.insert Room { name: Text.NonEmpty "name-1" }
app/Booking.alder:23:12: warning[W0002]: hole ?nextFree has type Date -> List Booking -> Date
  nextFree = ?nextFree
             ^^^^^^^^^
[exit 0]
$ alder fill Booking.nextFree - <<'EOF'
\day bookings -> match List.find (\b -> b.from <= day && day < b.to) bookings {
  Just b -> nextFree b.to bookings,
  Nothing -> day,
}
EOF
edited app/Booking.alder
--- a/app/Booking.alder
+++ b/app/Booking.alder
@@ -20,7 +20,10 @@
 }
 
 nextFree : Date -> List Booking -> Date
-nextFree = ?nextFree
+nextFree = \day bookings -> match List.find (\b -> b.from <= day && day < b.to) bookings {
+  Just b -> nextFree b.to bookings,
+  Nothing -> day,
+}
 
 test "a Booking keeps its room from being deleted" = {
   let today <- Date.today
[exit 0]
$ alder new decl Booking freeOf "Date -> List Booking -> Room -> RoomFree"
edited app/Booking.alder
--- a/app/Booking.alder
+++ b/app/Booking.alder
@@ -25,6 +25,9 @@
   Nothing -> day,
 }
 
+freeOf : Date -> List Booking -> Room -> RoomFree
+freeOf = ?freeOf
+
 test "a Booking keeps its room from being deleted" = {
   let today <- Date.today
   let target <- Db.insert Room { name: Text.NonEmpty "name-1" }
app/Booking.alder:29:10: warning[W0002]: hole ?freeOf has type Date -> List Booking -> Room -> Booking.RoomFree
  freeOf = ?freeOf
           ^^^^^^^
[exit 0]
$ alder fill Booking.freeOf - <<'EOF'
\today bookings r -> {
  let f : RoomFree = { room: r, free: nextFree today (List.filter (\b -> b.room == r.id) bookings) }
  f
}
EOF
edited app/Booking.alder
--- a/app/Booking.alder
+++ b/app/Booking.alder
@@ -26,7 +26,10 @@
 }
 
 freeOf : Date -> List Booking -> Room -> RoomFree
-freeOf = ?freeOf
+freeOf = \today bookings r -> {
+  let f : RoomFree = { room: r, free: nextFree today (List.filter (\b -> b.room == r.id) bookings) }
+  f
+}
 
 test "a Booking keeps its room from being deleted" = {
   let today <- Date.today
[exit 0]
$ alder new decl Booking roomsFree "Query (List RoomFree)"
edited app/Booking.alder
--- a/app/Booking.alder
+++ b/app/Booking.alder
@@ -31,6 +31,9 @@
   f
 }
 
+roomsFree : Query (List RoomFree)
+roomsFree = ?roomsFree
+
 test "a Booking keeps its room from being deleted" = {
   let today <- Date.today
   let target <- Db.insert Room { name: Text.NonEmpty "name-1" }
app/Booking.alder:35:13: warning[W0002]: hole ?roomsFree has type Query (List Booking.RoomFree)
  roomsFree = ?roomsFree
              ^^^^^^^^^^
[exit 0]
$ alder fill Booking.roomsFree - <<'EOF'
{
  let today <- Date.today
  let rooms <- Db.from Room |> Db.orderBy Room.name.asc |> Db.all
  let bookings <- Db.from Booking |> Db.all
  Query.succeed (List.map (freeOf today bookings) rooms)
}
EOF
edited app/Booking.alder
--- a/app/Booking.alder
+++ b/app/Booking.alder
@@ -32,7 +32,12 @@
 }
 
 roomsFree : Query (List RoomFree)
-roomsFree = ?roomsFree
+roomsFree = {
+  let today <- Date.today
+  let rooms <- Db.from Room |> Db.orderBy Room.name.asc |> Db.all
+  let bookings <- Db.from Booking |> Db.all
+  Query.succeed (List.map (freeOf today bookings) rooms)
+}
 
 test "a Booking keeps its room from being deleted" = {
   let today <- Date.today
[exit 0]
$ alder new decl Booking roomPage "Room.RoomId -> Query RoomPage"
edited app/Booking.alder
--- a/app/Booking.alder
+++ b/app/Booking.alder
@@ -39,6 +39,9 @@
   Query.succeed (List.map (freeOf today bookings) rooms)
 }
 
+roomPage : Room.RoomId -> Query RoomPage
+roomPage = ?roomPage
+
 test "a Booking keeps its room from being deleted" = {
   let today <- Date.today
   let target <- Db.insert Room { name: Text.NonEmpty "name-1" }
app/Booking.alder:43:12: warning[W0002]: hole ?roomPage has type Room.RoomId -> Query Booking.RoomPage
  roomPage = ?roomPage
             ^^^^^^^^^
[exit 0]
$ alder fill Booking.roomPage - <<'EOF'
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
edited app/Booking.alder
--- a/app/Booking.alder
+++ b/app/Booking.alder
@@ -40,7 +40,17 @@
 }
 
 roomPage : Room.RoomId -> Query RoomPage
-roomPage = ?roomPage
+roomPage = \id -> {
+  let found <- Db.get Room id
+  match found {
+    Just room -> {
+      let bookings <- Db.from Booking |> Db.where (Booking.room.eq id) |> Db.orderBy Booking.from.asc |> Db.all
+      let p : RoomPage = { room: room, bookings: bookings }
+      Query.succeed p
+    },
+    Nothing -> Query.notFound,
+  }
+}
 
 test "a Booking keeps its room from being deleted" = {
   let today <- Date.today
[exit 0]
$ alder new decl Booking book "Room.RoomId -> User.UserId -> Date -> Date -> Task (Maybe Text)"
edited app/Booking.alder
--- a/app/Booking.alder
+++ b/app/Booking.alder
@@ -52,6 +52,9 @@
   }
 }
 
+book : Room.RoomId -> User.UserId -> Date -> Date -> Task (Maybe Text)
+book = ?book
+
 test "a Booking keeps its room from being deleted" = {
   let today <- Date.today
   let target <- Db.insert Room { name: Text.NonEmpty "name-1" }
app/Booking.alder:56:8: warning[W0002]: hole ?book has type Room.RoomId -> User.UserId -> Date -> Date -> Task (Maybe Text)
  book = ?book
         ^^^^^
[exit 0]
$ alder fill Booking.book - <<'EOF'
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
edited app/Booking.alder
--- a/app/Booking.alder
+++ b/app/Booking.alder
@@ -53,7 +53,15 @@
 }
 
 book : Room.RoomId -> User.UserId -> Date -> Date -> Task (Maybe Text)
-book = ?book
+book = \room user start end -> {
+  let clashes <- Db.from Booking |> Db.where (Booking.room.eq room) |> Db.where (Booking.from.lt end) |> Db.where (Booking.to.gt start) |> Db.count
+  if end <= start then Task.succeed (Just "The booking must end after it starts.")
+  else if clashes > 0 then Task.succeed (Just "The room is already booked for those dates.")
+  else {
+    let _ <- Db.insert Booking { room: room, owner: user, from: start, to: end }
+    Task.succeed Nothing
+  }
+}
 
 test "a Booking keeps its room from being deleted" = {
   let today <- Date.today
[exit 0]
$ alder new decl Booking lineOf "List Room -> Booking -> Maybe Line"
edited app/Booking.alder
--- a/app/Booking.alder
+++ b/app/Booking.alder
@@ -63,6 +63,9 @@
   }
 }
 
+lineOf : List Room -> Booking -> Maybe Line
+lineOf = ?lineOf
+
 test "a Booking keeps its room from being deleted" = {
   let today <- Date.today
   let target <- Db.insert Room { name: Text.NonEmpty "name-1" }
app/Booking.alder:67:10: warning[W0002]: hole ?lineOf has type List Room -> Booking -> Maybe Booking.Line
  lineOf = ?lineOf
           ^^^^^^^
[exit 0]
$ alder fill Booking.lineOf - <<'EOF'
\rooms b -> match List.find (\r -> r.id == b.room) rooms {
  Just r -> {
    let l : Line = { booking: b, room: r }
    Just l
  },
  Nothing -> Nothing,
}
EOF
edited app/Booking.alder
--- a/app/Booking.alder
+++ b/app/Booking.alder
@@ -64,7 +64,13 @@
 }
 
 lineOf : List Room -> Booking -> Maybe Line
-lineOf = ?lineOf
+lineOf = \rooms b -> match List.find (\r -> r.id == b.room) rooms {
+  Just r -> {
+    let l : Line = { booking: b, room: r }
+    Just l
+  },
+  Nothing -> Nothing,
+}
 
 test "a Booking keeps its room from being deleted" = {
   let today <- Date.today
[exit 0]
$ alder new decl Booking ofUser "User.UserId -> Query (List Line)"
edited app/Booking.alder
--- a/app/Booking.alder
+++ b/app/Booking.alder
@@ -72,6 +72,9 @@
   Nothing -> Nothing,
 }
 
+ofUser : User.UserId -> Query (List Line)
+ofUser = ?ofUser
+
 test "a Booking keeps its room from being deleted" = {
   let today <- Date.today
   let target <- Db.insert Room { name: Text.NonEmpty "name-1" }
app/Booking.alder:76:10: warning[W0002]: hole ?ofUser has type User.UserId -> Query (List Booking.Line)
  ofUser = ?ofUser
           ^^^^^^^
[exit 0]
$ alder fill Booking.ofUser - <<'EOF'
\user -> {
  let bookings <- Db.from Booking |> Db.where (Booking.owner.eq user) |> Db.orderBy Booking.from.asc |> Db.all
  let rooms <- Db.from Room |> Db.all
  Query.succeed (List.filterMap (lineOf rooms) bookings)
}
EOF
edited app/Booking.alder
--- a/app/Booking.alder
+++ b/app/Booking.alder
@@ -73,7 +73,11 @@
 }
 
 ofUser : User.UserId -> Query (List Line)
-ofUser = ?ofUser
+ofUser = \user -> {
+  let bookings <- Db.from Booking |> Db.where (Booking.owner.eq user) |> Db.orderBy Booking.from.asc |> Db.all
+  let rooms <- Db.from Room |> Db.all
+  Query.succeed (List.filterMap (lineOf rooms) bookings)
+}
 
 test "a Booking keeps its room from being deleted" = {
   let today <- Date.today
[exit 0]
$ alder new decl Booking line "User.UserId -> BookingId -> Query Line"
edited app/Booking.alder
--- a/app/Booking.alder
+++ b/app/Booking.alder
@@ -79,6 +79,9 @@
   Query.succeed (List.filterMap (lineOf rooms) bookings)
 }
 
+line : User.UserId -> BookingId -> Query Line
+line = ?line
+
 test "a Booking keeps its room from being deleted" = {
   let today <- Date.today
   let target <- Db.insert Room { name: Text.NonEmpty "name-1" }
app/Booking.alder:83:8: warning[W0002]: hole ?line has type User.UserId -> Booking.BookingId -> Query Booking.Line
  line = ?line
         ^^^^^
[exit 0]
$ alder fill Booking.line - <<'EOF'
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
edited app/Booking.alder
--- a/app/Booking.alder
+++ b/app/Booking.alder
@@ -80,7 +80,22 @@
 }
 
 line : User.UserId -> BookingId -> Query Line
-line = ?line
+line = \user id -> {
+  let found <- Db.from Booking |> Db.where (Booking.id.eq id) |> Db.where (Booking.owner.eq user) |> Db.first
+  match found {
+    Just b -> {
+      let room <- Db.get Room b.room
+      match room {
+        Just r -> {
+          let l : Line = { booking: b, room: r }
+          Query.succeed l
+        },
+        Nothing -> Query.notFound,
+      }
+    },
+    Nothing -> Query.notFound,
+  }
+}
 
 test "a Booking keeps its room from being deleted" = {
   let today <- Date.today
[exit 0]
$ alder new decl Booking canCancel "Date -> Booking -> Bool"
edited app/Booking.alder
--- a/app/Booking.alder
+++ b/app/Booking.alder
@@ -97,6 +97,9 @@
   }
 }
 
+canCancel : Date -> Booking -> Bool
+canCancel = ?canCancel
+
 test "a Booking keeps its room from being deleted" = {
   let today <- Date.today
   let target <- Db.insert Room { name: Text.NonEmpty "name-1" }
app/Booking.alder:101:13: warning[W0002]: hole ?canCancel has type Date -> Booking -> Bool
  canCancel = ?canCancel
              ^^^^^^^^^^
[exit 0]
$ alder fill Booking.canCancel - <<'EOF'
\today b -> today < b.from
EOF
edited app/Booking.alder
--- a/app/Booking.alder
+++ b/app/Booking.alder
@@ -98,7 +98,7 @@
 }
 
 canCancel : Date -> Booking -> Bool
-canCancel = ?canCancel
+canCancel = \today b -> today < b.from
 
 test "a Booking keeps its room from being deleted" = {
   let today <- Date.today
[exit 0]
$ alder new decl Booking cancel "BookingId -> Task Unit"
edited app/Booking.alder
--- a/app/Booking.alder
+++ b/app/Booking.alder
@@ -100,6 +100,9 @@
 canCancel : Date -> Booking -> Bool
 canCancel = \today b -> today < b.from
 
+cancel : BookingId -> Task Unit
+cancel = ?cancel
+
 test "a Booking keeps its room from being deleted" = {
   let today <- Date.today
   let target <- Db.insert Room { name: Text.NonEmpty "name-1" }
app/Booking.alder:104:10: warning[W0002]: hole ?cancel has type Booking.BookingId -> Task Unit
  cancel = ?cancel
           ^^^^^^^
[exit 0]
$ alder fill Booking.cancel - <<'EOF'
\id -> Db.delete Booking id
EOF
edited app/Booking.alder
--- a/app/Booking.alder
+++ b/app/Booking.alder
@@ -101,7 +101,7 @@
 canCancel = \today b -> today < b.from
 
 cancel : BookingId -> Task Unit
-cancel = ?cancel
+cancel = \id -> Db.delete Booking id
 
 test "a Booking keeps its room from being deleted" = {
   let today <- Date.today
[exit 0]
$ alder new page Rooms.Index /rooms --blank --requires user
wrote app/Rooms/Index.alder
app/Rooms/Index.alder:8:23: warning[W0002]: hole ?load has type Query Rooms.Index.Model
  load params session = ?load
                        ^^^^^
app/Rooms/Index.alder:11:22: warning[W0002]: hole ?view has type Html Rooms.Index.Msg
  view model session = ?view
                       ^^^^^
[exit 0]
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
$ alder fill --list
Auth.Login update.Submit ?afterLogin : Page
Auth.Login "logging in through the form" ?afterLogin : Page
Bookings.Index load ?load : Query Bookings.Index.Model
Bookings.Index view ?view : Html Bookings.Index.Msg
Bookings.Show load ?load : Query Bookings.Show.Model
Bookings.Show view ?view : Html Bookings.Show.Msg
Rooms.Index load ?load : Query Rooms.Index.Model
Rooms.Index view ?view : Html Rooms.Index.Msg
Rooms.Show load ?load : Query Rooms.Show.Model
Rooms.Show view ?view : Html Rooms.Show.Msg
[exit 0]
$ alder fill Auth.Login.update.Submit - --hole ?afterLogin <<'EOF'
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
$ alder fill --decl Rooms.Index.Model - <<'EOF'
{ rooms: List Booking.RoomFree }
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
+type Model = { rooms: List Booking.RoomFree }
 
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
$ alder fill Rooms.Index.load - <<'EOF'
{
  let rooms <- Booking.roomsFree
  Query.succeed { rooms: rooms }
}
EOF
edited app/Rooms/Index.alder
--- a/app/Rooms/Index.alder
+++ b/app/Rooms/Index.alder
@@ -7,7 +7,10 @@
 type Model = { rooms: List Booking.RoomFree }
 
 load : Params -> Session -> Query Model
-load params session = ?load
+load params session = {
+  let rooms <- Booking.roomsFree
+  Query.succeed { rooms: rooms }
+}
 
 view : Model -> Session -> Html Msg
 view model session = ?view
app/Rooms/Index.alder:16:22: warning[W0002]: hole ?view has type Html Rooms.Index.Msg
  view model session = ?view
                       ^^^^^
[exit 0]
$ alder fill Rooms.Index.view - <<'EOF'
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
edited app/Rooms/Index.alder
--- a/app/Rooms/Index.alder
+++ b/app/Rooms/Index.alder
@@ -1,6 +1,7 @@
 module Rooms.Index
 
 import Booking
+import Layout
 
 page "/rooms" { } requires user
 
@@ -13,7 +14,19 @@
 }
 
 view : Model -> Session -> Html Msg
-view model session = ?view
+view model session = <Layout.Page title="Rooms">
+  <h1>Rooms</h1>
+  <ul id="rooms">
+    {for r in model.rooms ->
+      <li>
+        <a class="name" href={Rooms.Show { id: r.room.id }}>{r.room.name}</a>
+        {" "}
+        <span class="free">{Date.toText r.free}</span>
+      </li>}
+  </ul>
+  <a href={Bookings.Index {}}>My bookings</a>
+  <button click={App.Logout}>Log out</button>
+</Layout.Page>
 
 test "a visitor is sent to log in" = {
   let r <- Test.open (Rooms.Index {})
added import Layout to app/Rooms/Index.alder
[exit 0]
$ alder fill --decl Rooms.Show.Model - <<'EOF'
{ data: Booking.RoomPage }
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
+type Model = { data: Booking.RoomPage }
 
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
  let data <- Booking.roomPage params.id
  Query.succeed { data: data }
}
EOF
edited app/Rooms/Show.alder
--- a/app/Rooms/Show.alder
+++ b/app/Rooms/Show.alder
@@ -8,7 +8,10 @@
 type Model = { data: Booking.RoomPage }
 
 load : Params -> Session -> Query Model
-load params session = ?load
+load params session = {
+  let data <- Booking.roomPage params.id
+  Query.succeed { data: data }
+}
 
 view : Model -> Session -> Html Msg
 view model session = ?view
app/Rooms/Show.alder:17:22: warning[W0002]: hole ?view has type Html Rooms.Show.Msg
  view model session = ?view
                       ^^^^^
[exit 0]
$ alder fill Rooms.Show.view - <<'EOF'
<Layout.Page title={model.data.room.name}>
  <h1 id="name">{model.data.room.name}</h1>
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
+view model session = <Layout.Page title={model.data.room.name}>
+  <h1 id="name">{model.data.room.name}</h1>
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
   Query.succeed { data: data }
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
 view model session = <Layout.Page title={model.data.room.name}>
   <h1 id="name">{model.data.room.name}</h1>
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
edited app/Rooms/Show.alder
--- a/app/Rooms/Show.alder
+++ b/app/Rooms/Show.alder
@@ -28,19 +28,27 @@
 }
 
 view : Model -> Session -> Html Msg
-view model session = <Layout.Page title={model.data.room.name}>
-  <h1 id="name">{model.data.room.name}</h1>
-
-  <al-card>
-    <h2 slot="header">New booking</h2>
+view model session =
+  <Layout.Page title={model.data.room.name}>
+    <h1 id="name">{model.data.room.name}</h1>
+    <ul id="bookings">
+      {for b in model.data.bookings ->
+        <li>
+          <span class="from">{Date.toText b.from}</span>
+          {" to "}
+          <span class="to">{Date.toText b.to}</span>
+        </li>}
+    </ul>
     <form submit={Book}>
-      <al-input field="from" label="From" />
-      <al-input field="to" label="To" />
-      <al-alert variant="destructive"><Errors form /></al-alert>
-      <al-button type="submit">New booking</al-button>
+      <Errors form />
+      <Input field="from" label="From" />
+      <Errors field="from" />
+      <Input field="to" label="To" />
+      <Errors field="to" />
+      <button>Book</button>
     </form>
-  </al-card>
-</Layout.Page>
+    <a href={Rooms.Index {}}>All rooms</a>
+  </Layout.Page>
 
 test "a visitor is sent to log in" = {
   let room1 <- Db.insert Room { name: Text.NonEmpty "name-1" }
app/Rooms/Show.alder:27:13: warning[W0002]: hole ?book has type Task Nav
    Book p -> ?book,
              ^^^^^
app/Rooms/Show.alder:66:29: warning[W0002]: hole ?expected has type Answer
    Task.succeed (r.answer == ?expected)
                              ^^^^^^^^^
[exit 0]
$ alder fill Rooms.Show.update.Book - <<'EOF'
{
  let refused <- Booking.book model.data.room.id session.user p.form.from p.form.to
  match refused {
    Just why -> Task.succeed (Reject why),
    Nothing -> Task.succeed (Redirect (Rooms.Show { id: model.data.room.id })),
  }
}
EOF
edited app/Rooms/Show.alder
--- a/app/Rooms/Show.alder
+++ b/app/Rooms/Show.alder
@@ -24,7 +24,13 @@
 
 update : Msg -> Model -> Session -> Task Nav
 update msg model session = match msg {
-  Book p -> ?book,
+  Book p -> {
+    let refused <- Booking.book model.data.room.id session.user p.form.from p.form.to
+    match refused {
+      Just why -> Task.succeed (Reject why),
+      Nothing -> Task.succeed (Redirect (Rooms.Show { id: model.data.room.id })),
+    }
+  },
 }
 
 view : Model -> Session -> Html Msg
app/Rooms/Show.alder:72:29: warning[W0002]: hole ?expected has type Answer
    Task.succeed (r.answer == ?expected)
                              ^^^^^^^^^
[exit 0]
$ alder fill --decl Bookings.Index.Model - <<'EOF'
{ mine: List Booking.Line }
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
+type Model = { mine: List Booking.Line }
 
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
  let mine <- Booking.ofUser session.user
  Query.succeed { mine: mine }
}
EOF
edited app/Bookings/Index.alder
--- a/app/Bookings/Index.alder
+++ b/app/Bookings/Index.alder
@@ -7,7 +7,10 @@
 type Model = { mine: List Booking.Line }
 
 load : Params -> Session -> Query Model
-load params session = ?load
+load params session = {
+  let mine <- Booking.ofUser session.user
+  Query.succeed { mine: mine }
+}
 
 view : Model -> Session -> Html Msg
 view model session = ?view
app/Bookings/Index.alder:16:22: warning[W0002]: hole ?view has type Html Bookings.Index.Msg
  view model session = ?view
                       ^^^^^
[exit 0]
$ alder fill Bookings.Index.view - <<'EOF'
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
edited app/Bookings/Index.alder
--- a/app/Bookings/Index.alder
+++ b/app/Bookings/Index.alder
@@ -1,6 +1,7 @@
 module Bookings.Index
 
 import Booking
+import Layout
 
 page "/bookings" { } requires user
 
@@ -13,7 +14,20 @@
 }
 
 view : Model -> Session -> Html Msg
-view model session = ?view
+view model session = <Layout.Page title="My bookings">
+  <h1>My bookings</h1>
+  <ul id="mine">
+    {for l in model.mine ->
+      <li>
+        <a href={Bookings.Show { id: l.booking.id }}>
+          <span class="room">{l.room.name}</span>
+          {" "}
+          <span class="from">{Date.toText l.booking.from}</span>
+        </a>
+      </li>}
+  </ul>
+  <a href={Rooms.Index {}}>Rooms</a>
+</Layout.Page>
 
 test "a visitor is sent to log in" = {
   let r <- Test.open (Bookings.Index {})
added import Layout to app/Bookings/Index.alder
[exit 0]
$ alder fill --decl Bookings.Show.Model - <<'EOF'
{ line: Booking.Line, today: Date }
EOF
edited app/Bookings/Show.alder
--- a/app/Bookings/Show.alder
+++ b/app/Bookings/Show.alder
@@ -6,7 +6,7 @@
 
 page "/bookings/{id}" { id: Booking.BookingId } requires user
 
-type Model = { }
+type Model = { line: Booking.Line, today: Date }
 
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
  let line <- Booking.line session.user params.id
  let today <- Date.today
  Query.succeed { line: line, today: today }
}
EOF
edited app/Bookings/Show.alder
--- a/app/Bookings/Show.alder
+++ b/app/Bookings/Show.alder
@@ -9,7 +9,11 @@
 type Model = { line: Booking.Line, today: Date }
 
 load : Params -> Session -> Query Model
-load params session = ?load
+load params session = {
+  let line <- Booking.line session.user params.id
+  let today <- Date.today
+  Query.succeed { line: line, today: today }
+}
 
 view : Model -> Session -> Html Msg
 view model session = ?view
app/Bookings/Show.alder:19:22: warning[W0002]: hole ?view has type Html Bookings.Show.Msg
  view model session = ?view
                       ^^^^^
[exit 0]
$ alder fill Bookings.Show.view - <<'EOF'
<Layout.Page title="Booking">
  <h1 id="room">{model.line.room.name}</h1>
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
 
@@ -16,7 +17,9 @@
 }
 
 view : Model -> Session -> Html Msg
-view model session = ?view
+view model session = <Layout.Page title="Booking">
+  <h1 id="room">{model.line.room.name}</h1>
+</Layout.Page>
 
 test "a visitor is sent to log in" = {
   let today <- Date.today
added import Layout to app/Bookings/Show.alder
[exit 0]
$ alder new msg Bookings.Show Cancel
edited app/Bookings/Show.alder
--- a/app/Bookings/Show.alder
+++ b/app/Bookings/Show.alder
@@ -16,9 +16,18 @@
   Query.succeed { line: line, today: today }
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
   <h1 id="room">{model.line.room.name}</h1>
+
+  <al-button click={Cancel}>Cancel</al-button>
 </Layout.Page>
 
 test "a visitor is sent to log in" = {
@@ -29,3 +38,20 @@
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
app/Bookings/Show.alder:23:15: warning[W0002]: hole ?cancel has type Task Nav
    Cancel p -> ?cancel,
                ^^^^^^^
app/Bookings/Show.alder:56:16: warning[W0002]: hole ?expected has type Bool
    Task.succeed ?expected
                 ^^^^^^^^^
[exit 0]
$ alder fill --decl Bookings.Show.view - <<'EOF'
<Layout.Page title="Booking">
  <h1 id="room">{model.line.room.name}</h1>
  <p>From <span id="from">{Date.toText model.line.booking.from}</span> to <span id="to">{Date.toText model.line.booking.to}</span></p>
  {if Booking.canCancel model.today model.line.booking then <button id="cancel" click={Cancel}>Cancel</button> else Html.none}
  <a href={Bookings.Index {}}>My bookings</a>
</Layout.Page>
EOF
edited app/Bookings/Show.alder
--- a/app/Bookings/Show.alder
+++ b/app/Bookings/Show.alder
@@ -24,11 +24,13 @@
 }
 
 view : Model -> Session -> Html Msg
-view model session = <Layout.Page title="Booking">
-  <h1 id="room">{model.line.room.name}</h1>
-
-  <al-button click={Cancel}>Cancel</al-button>
-</Layout.Page>
+view model session =
+  <Layout.Page title="Booking">
+    <h1 id="room">{model.line.room.name}</h1>
+    <p>From <span id="from">{Date.toText model.line.booking.from}</span> to <span id="to">{Date.toText model.line.booking.to}</span></p>
+    {if Booking.canCancel model.today model.line.booking then <button id="cancel" click={Cancel}>Cancel</button> else Html.none}
+    <a href={Bookings.Index {}}>My bookings</a>
+  </Layout.Page>
 
 test "a visitor is sent to log in" = {
   let today <- Date.today
app/Bookings/Show.alder:23:15: warning[W0002]: hole ?cancel has type Task Nav
    Cancel p -> ?cancel,
                ^^^^^^^
app/Bookings/Show.alder:58:16: warning[W0002]: hole ?expected has type Bool
    Task.succeed ?expected
                 ^^^^^^^^^
[exit 0]
$ alder fill Bookings.Show.update.Cancel - <<'EOF'
{
  let today <- Date.today
  if Booking.canCancel today model.line.booking then {
    let _ <- Booking.cancel model.line.booking.id
    Task.succeed (Redirect (Bookings.Index {}))
  } else Task.succeed (Reject "This booking has started: it can no longer be cancelled.")
}
EOF
edited app/Bookings/Show.alder
--- a/app/Bookings/Show.alder
+++ b/app/Bookings/Show.alder
@@ -20,7 +20,13 @@
 
 update : Msg -> Model -> Session -> Task Nav
 update msg model session = match msg {
-  Cancel p -> ?cancel,
+  Cancel p -> {
+    let today <- Date.today
+    if Booking.canCancel today model.line.booking then {
+      let _ <- Booking.cancel model.line.booking.id
+      Task.succeed (Redirect (Bookings.Index {}))
+    } else Task.succeed (Reject "This booking has started: it can no longer be cancelled.")
+  },
 }
 
 view : Model -> Session -> Html Msg
app/Bookings/Show.alder:64:16: warning[W0002]: hole ?expected has type Bool
    Task.succeed ?expected
                 ^^^^^^^^^
[exit 0]
$ alder new test Rooms.Show "an overlapping booking is refused, a back-to-back one is stored"
edited app/Rooms/Show.alder
--- a/app/Rooms/Show.alder
+++ b/app/Rooms/Show.alder
@@ -80,3 +80,7 @@
   let r <- Test.submitRaw NewBooking [{ name: "from", value: "" }, { name: "to", value: "2026-01-01" }]
   Task.succeed (r.answer == FormRejected [{ field: "from", message: "is required" }])
 }
+
+test "an overlapping booking is refused, a back-to-back one is stored" = {
+  ?test
+}
app/Rooms/Show.alder:72:29: warning[W0002]: hole ?expected has type Answer
    Task.succeed (r.answer == ?expected)
                              ^^^^^^^^^
app/Rooms/Show.alder:85:3: warning[W0002]: hole ?test has type Task Bool
    ?test
    ^^^^^
[exit 0]
$ alder fill 'Rooms.Show."an overlapping booking is refused, a back-to-back one is stored"' - <<'EOF'
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
edited app/Rooms/Show.alder
--- a/app/Rooms/Show.alder
+++ b/app/Rooms/Show.alder
@@ -82,5 +82,18 @@
 }
 
 test "an overlapping booking is refused, a back-to-back one is stored" = {
-  ?test
+  let me <- Db.insert User.User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
+  let oak <- Db.insert Room.Room { name: Text.NonEmpty "Oak" }
+  let elm <- Db.insert Room.Room { name: Text.NonEmpty "Elm" }
+  let today <- Date.today
+  let start = Date.addDays 10 today
+  let _ <- Db.insert Booking.Booking { room: oak, owner: me, from: start, to: Date.addDays 2 start }
+  let _ <- Db.insert Booking.Booking { room: elm, owner: me, from: Date.addDays 2 start, to: Date.addDays 4 start }
+  let _ <- Test.login { user: Just me }
+  let _ <- Test.get (Rooms.Show { id: oak })
+  let clash <- Test.submit NewBooking { from: Date.addDays 1 start, to: Date.addDays 3 start }
+  let next <- Test.submit NewBooking { from: Date.addDays 2 start, to: Date.addDays 4 start }
+  let backwards <- Test.submit NewBooking { from: Date.addDays 20 start, to: Date.addDays 20 start }
+  let after <- Booking.roomPage oak
+  Task.succeed ({ clash: clash.answer, next: next.answer, backwards: backwards.answer, froms: List.map (\b -> Date.toText b.from) after.bookings } == { clash: FormRejected [{ field: "", message: "The room is already booked for those dates." }], next: Redirected (Rooms.Show { id: oak }), backwards: FormRejected [{ field: "", message: "The booking must end after it starts." }], froms: [Date.toText start, Date.toText (Date.addDays 2 start)] })
 }
app/Rooms/Show.alder:72:29: warning[W0002]: hole ?expected has type Answer
    Task.succeed (r.answer == ?expected)
                              ^^^^^^^^^
[exit 0]
$ alder new test Rooms.Index "the next free date skips back-to-back bookings"
edited app/Rooms/Index.alder
--- a/app/Rooms/Index.alder
+++ b/app/Rooms/Index.alder
@@ -32,3 +32,7 @@
   let r <- Test.open (Rooms.Index {})
   Task.succeed (r.answer == Redirected (Auth.Login {}))
 }
+
+test "the next free date skips back-to-back bookings" = {
+  ?test
+}
app/Rooms/Index.alder:37:3: warning[W0002]: hole ?test has type Task Bool
    ?test
    ^^^^^
[exit 0]
$ alder fill 'Rooms.Index."the next free date skips back-to-back bookings"' - <<'EOF'
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
edited app/Rooms/Index.alder
--- a/app/Rooms/Index.alder
+++ b/app/Rooms/Index.alder
@@ -2,6 +2,8 @@
 
 import Booking
 import Layout
+import Room
+import User
 
 page "/rooms" { } requires user
 
@@ -34,5 +36,14 @@
 }
 
 test "the next free date skips back-to-back bookings" = {
-  ?test
+  let me <- Db.insert User.User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
+  let oak <- Db.insert Room.Room { name: Text.NonEmpty "Oak" }
+  let _ <- Db.insert Room.Room { name: Text.NonEmpty "Elm" }
+  let today <- Date.today
+  let _ <- Db.insert Booking.Booking { room: oak, owner: me, from: today, to: Date.addDays 2 today }
+  let _ <- Db.insert Booking.Booking { room: oak, owner: me, from: Date.addDays 2 today, to: Date.addDays 5 today }
+  let _ <- Db.insert Booking.Booking { room: oak, owner: me, from: Date.addDays 6 today, to: Date.addDays 7 today }
+  let _ <- Test.login { user: Just me }
+  let page <- Test.get (Rooms.Index {})
+  Task.succeed ({ names: Test.texts "#rooms .name" page.doc, free: Test.texts "#rooms .free" page.doc } == { names: ["Elm", "Oak"], free: [Date.toText today, Date.toText (Date.addDays 5 today)] })
 }
added import User to app/Rooms/Index.alder
added import Room to app/Rooms/Index.alder
[exit 0]
$ alder new test Bookings.Show "the owner cancels a booking before its from date"
edited app/Bookings/Show.alder
--- a/app/Bookings/Show.alder
+++ b/app/Bookings/Show.alder
@@ -63,3 +63,7 @@
   let after <- Test.open (Bookings.Show { id: booking1 })
   Task.succeed ?expected
 }
+
+test "the owner cancels a booking before its from date" = {
+  ?test
+}
app/Bookings/Show.alder:64:16: warning[W0002]: hole ?expected has type Bool
    Task.succeed ?expected
                 ^^^^^^^^^
app/Bookings/Show.alder:68:3: warning[W0002]: hole ?test has type Task Bool
    ?test
    ^^^^^
[exit 0]
$ alder fill 'Bookings.Show."the owner cancels a booking before its from date"' - <<'EOF'
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
edited app/Bookings/Show.alder
--- a/app/Bookings/Show.alder
+++ b/app/Bookings/Show.alder
@@ -65,5 +65,13 @@
 }
 
 test "the owner cancels a booking before its from date" = {
-  ?test
+  let me <- Db.insert User.User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
+  let oak <- Db.insert Room.Room { name: Text.NonEmpty "Oak" }
+  let today <- Date.today
+  let id <- Db.insert Booking.Booking { room: oak, owner: me, from: Date.addDays 1 today, to: Date.addDays 3 today }
+  let _ <- Test.login { user: Just me }
+  let page <- Test.get (Bookings.Show { id: id })
+  let r <- Test.click Cancel
+  let left <- Booking.ofUser me
+  Task.succeed ({ button: Test.has "#cancel" page.doc, answer: r.answer, left: List.length left } == { button: True, answer: Redirected (Bookings.Index {}), left: 0 })
 }
app/Bookings/Show.alder:64:16: warning[W0002]: hole ?expected has type Bool
    Task.succeed ?expected
                 ^^^^^^^^^
[exit 0]
$ alder new test Bookings.Show "a booking whose from date is today cannot be cancelled"
edited app/Bookings/Show.alder
--- a/app/Bookings/Show.alder
+++ b/app/Bookings/Show.alder
@@ -75,3 +75,7 @@
   let left <- Booking.ofUser me
   Task.succeed ({ button: Test.has "#cancel" page.doc, answer: r.answer, left: List.length left } == { button: True, answer: Redirected (Bookings.Index {}), left: 0 })
 }
+
+test "a booking whose from date is today cannot be cancelled" = {
+  ?test
+}
app/Bookings/Show.alder:64:16: warning[W0002]: hole ?expected has type Bool
    Task.succeed ?expected
                 ^^^^^^^^^
app/Bookings/Show.alder:80:3: warning[W0002]: hole ?test has type Task Bool
    ?test
    ^^^^^
[exit 0]
$ alder fill 'Bookings.Show."a booking whose from date is today cannot be cancelled"' - <<'EOF'
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
edited app/Bookings/Show.alder
--- a/app/Bookings/Show.alder
+++ b/app/Bookings/Show.alder
@@ -77,5 +77,14 @@
 }
 
 test "a booking whose from date is today cannot be cancelled" = {
-  ?test
+  let me <- Db.insert User.User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
+  let oak <- Db.insert Room.Room { name: Text.NonEmpty "Oak" }
+  let today <- Date.today
+  let id <- Db.insert Booking.Booking { room: oak, owner: me, from: today, to: Date.addDays 2 today }
+  let _ <- Test.login { user: Just me }
+  let page <- Test.get (Bookings.Show { id: id })
+  let m = page.model
+  let r <- Test.run (update Cancel { m | today: Date.addDays (-1) today } { user: me })
+  let left <- Booking.ofUser me
+  Task.succeed ({ button: Test.has "#cancel" page.doc, value: r.value, left: List.length left } == { button: False, value: Ok (Reject "This booking has started: it can no longer be cancelled."), left: 1 })
 }
app/Bookings/Show.alder:64:16: warning[W0002]: hole ?expected has type Bool
    Task.succeed ?expected
                 ^^^^^^^^^
[exit 0]
$ alder fill --list
Auth.Login "logging in through the form" ?afterLogin : Page
Bookings.Show "pressing Cancel" ?expected : Bool
Rooms.Show "submitting NewBooking" ?expected : Answer
[exit 0]
$ alder check
app/Auth/Login.alder:86:85: warning[W0002]: hole ?afterLogin has type Page
    Task.succeed ({ answer: r.answer, user: r.session.user } == { answer: Redirected (?afterLogin), user: Just me })
                                                                                      ^^^^^^^^^^^
app/Bookings/Show.alder:64:16: warning[W0002]: hole ?expected has type Bool
    Task.succeed ?expected
                 ^^^^^^^^^
app/Rooms/Show.alder:72:29: warning[W0002]: hole ?expected has type Answer
    Task.succeed (r.answer == ?expected)
                              ^^^^^^^^^
alder check: 3 typed holes left
[exit 0]
$ alder test
app/Auth/Login.alder:86:85: warning[W0002]: hole ?afterLogin has type Page
    Task.succeed ({ answer: r.answer, user: r.session.user } == { answer: Redirected (?afterLogin), user: Just me })
                                                                                      ^^^^^^^^^^^
app/Bookings/Show.alder:64:16: warning[W0002]: hole ?expected has type Bool
    Task.succeed ?expected
                 ^^^^^^^^^
app/Rooms/Show.alder:72:29: warning[W0002]: hole ?expected has type Answer
    Task.succeed (r.answer == ?expected)
                              ^^^^^^^^^
alder test: no tests ran (fill the hole): ?afterLogin : Page, ?expected : Bool, ?expected : Answer
[exit 1]
```

The harness stopped at line 338 of your script: `alder test`: it exited 1. The commands after it were not run.

Your script stopped at command 62; the app is as the commands before it left it; continue.

The program type-checks but `alder build` refused it; fix the cause below.

```text
app/Auth/Login.alder:86:85: warning[W0002]: hole ?afterLogin has type Page
    Task.succeed ({ answer: r.answer, user: r.session.user } == { answer: Redirected (?afterLogin), user: Just me })
                                                                                      ^^^^^^^^^^^
app/Bookings/Show.alder:64:16: warning[W0002]: hole ?expected has type Bool
    Task.succeed ?expected
                 ^^^^^^^^^
app/Rooms/Show.alder:72:29: warning[W0002]: hole ?expected has type Answer
    Task.succeed (r.answer == ?expected)
                              ^^^^^^^^^
alder build: the program has 3 typed hole(s); fill them before building
```
