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
$ alder new table Booking room:Room.RoomId@ref=Room owner:User.UserId@ref=User@owner from:Date to:Date --no-timestamps
wrote app/Booking.alder
wrote migrations/0003_create_booking.sql
  create table Booking
  add index on Booking.owner
  add index on Booking.room
edited migrations/schema.json
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
\s e b -> s < b.to && b.from < e
EOF
edited app/Booking.alder
--- a/app/Booking.alder
+++ b/app/Booking.alder
@@ -14,7 +14,7 @@
 }
 
 overlaps : Date -> Date -> Booking -> Bool
-overlaps = ?overlaps
+overlaps = \s e b -> s < b.to && b.from < e
 
 test "a Booking keeps its room from being deleted" = {
   let today <- Date.today
[exit 0]
$ alder new decl Booking startText "Booking -> Text"
edited app/Booking.alder
--- a/app/Booking.alder
+++ b/app/Booking.alder
@@ -16,6 +16,9 @@
 overlaps : Date -> Date -> Booking -> Bool
 overlaps = \s e b -> s < b.to && b.from < e
 
+startText : Booking -> Text
+startText = ?startText
+
 test "a Booking keeps its room from being deleted" = {
   let today <- Date.today
   let target <- Db.insert Room { name: Text.NonEmpty "name-1" }
app/Booking.alder:20:13: warning[W0002]: hole ?startText has type Booking -> Text
  startText = ?startText
              ^^^^^^^^^^
[exit 0]
$ alder fill Booking.startText - <<'EOF'
\b -> Date.toText b.from
EOF
edited app/Booking.alder
--- a/app/Booking.alder
+++ b/app/Booking.alder
@@ -17,7 +17,7 @@
 overlaps = \s e b -> s < b.to && b.from < e
 
 startText : Booking -> Text
-startText = ?startText
+startText = \b -> Date.toText b.from
 
 test "a Booking keeps its room from being deleted" = {
   let today <- Date.today
[exit 0]
$ alder new decl Booking passOn "Booking -> Date -> Date"
edited app/Booking.alder
--- a/app/Booking.alder
+++ b/app/Booking.alder
@@ -19,6 +19,9 @@
 startText : Booking -> Text
 startText = \b -> Date.toText b.from
 
+passOn : Booking -> Date -> Date
+passOn = ?passOn
+
 test "a Booking keeps its room from being deleted" = {
   let today <- Date.today
   let target <- Db.insert Room { name: Text.NonEmpty "name-1" }
app/Booking.alder:23:10: warning[W0002]: hole ?passOn has type Booking -> Date -> Date
  passOn = ?passOn
           ^^^^^^^
[exit 0]
$ alder fill Booking.passOn - <<'EOF'
\b day -> if b.from <= day && day < b.to then b.to else day
EOF
edited app/Booking.alder
--- a/app/Booking.alder
+++ b/app/Booking.alder
@@ -20,7 +20,7 @@
 startText = \b -> Date.toText b.from
 
 passOn : Booking -> Date -> Date
-passOn = ?passOn
+passOn = \b day -> if b.from <= day && day < b.to then b.to else day
 
 test "a Booking keeps its room from being deleted" = {
   let today <- Date.today
[exit 0]
$ alder new decl Booking freeFrom "Room.RoomId -> Date -> Query Date"
edited app/Booking.alder
--- a/app/Booking.alder
+++ b/app/Booking.alder
@@ -22,6 +22,9 @@
 passOn : Booking -> Date -> Date
 passOn = \b day -> if b.from <= day && day < b.to then b.to else day
 
+freeFrom : Room.RoomId -> Date -> Query Date
+freeFrom = ?freeFrom
+
 test "a Booking keeps its room from being deleted" = {
   let today <- Date.today
   let target <- Db.insert Room { name: Text.NonEmpty "name-1" }
app/Booking.alder:26:12: warning[W0002]: hole ?freeFrom has type Room.RoomId -> Date -> Query Date
  freeFrom = ?freeFrom
             ^^^^^^^^^
[exit 0]
$ alder fill Booking.freeFrom - <<'EOF'
\room today -> {
  let existing <- Db.from Booking |> Db.where (Booking.room.eq room) |> Db.all
  Query.succeed (List.foldl passOn today (List.sortByText startText existing))
}
EOF
edited app/Booking.alder
--- a/app/Booking.alder
+++ b/app/Booking.alder
@@ -23,7 +23,10 @@
 passOn = \b day -> if b.from <= day && day < b.to then b.to else day
 
 freeFrom : Room.RoomId -> Date -> Query Date
-freeFrom = ?freeFrom
+freeFrom = \room today -> {
+  let existing <- Db.from Booking |> Db.where (Booking.room.eq room) |> Db.all
+  Query.succeed (List.foldl passOn today (List.sortByText startText existing))
+}
 
 test "a Booking keeps its room from being deleted" = {
   let today <- Date.today
[exit 0]
$ alder new test Booking "the next free date skips back-to-back bookings"
edited app/Booking.alder
--- a/app/Booking.alder
+++ b/app/Booking.alder
@@ -47,3 +47,7 @@
   let left <- Db.from Booking |> Db.count
   Task.succeed ({ deleted: Result.toMaybe r.value, left: left } == { deleted: Nothing, left: 1 })
 }
+
+test "the next free date skips back-to-back bookings" = {
+  ?test
+}
app/Booking.alder:52:3: warning[W0002]: hole ?test has type Task Bool
    ?test
    ^^^^^
[exit 0]
$ alder fill 'Booking."the next free date skips back-to-back bookings"' - <<'EOF'
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
edited app/Booking.alder
--- a/app/Booking.alder
+++ b/app/Booking.alder
@@ -49,5 +49,11 @@
 }
 
 test "the next free date skips back-to-back bookings" = {
-  ?test
+  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
+  let room <- Db.insert Room { name: Text.NonEmpty "Oak" }
+  let today <- Date.today
+  let _ <- Db.insert Booking { room: room, owner: me, from: today, to: Date.addDays 2 today }
+  let _ <- Db.insert Booking { room: room, owner: me, from: Date.addDays 2 today, to: Date.addDays 4 today }
+  let free <- freeFrom room today
+  Task.succeed (free == Date.addDays 4 today)
 }
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
$ alder new type Rooms.Index "Row = { id: Room.RoomId, name: Text, free: Date }"
edited app/Rooms/Index.alder
--- a/app/Rooms/Index.alder
+++ b/app/Rooms/Index.alder
@@ -1,9 +1,13 @@
 module Rooms.Index
 
+import Room
+
 page "/rooms" { } requires user
 
 type Model = { }
 
+type Row = { id: Room.RoomId, name: Text, free: Date }
+
 load : Params -> Session -> Query Model
 load params session = ?load
 
app/Rooms/Index.alder:12:23: warning[W0002]: hole ?load has type Query Rooms.Index.Model
  load params session = ?load
                        ^^^^^
app/Rooms/Index.alder:15:22: warning[W0002]: hole ?view has type Html Rooms.Index.Msg
  view model session = ?view
                       ^^^^^
[exit 0]
$ alder fill --decl Rooms.Index.Model - <<'EOF'
{ rows: List Row }
EOF
edited app/Rooms/Index.alder
--- a/app/Rooms/Index.alder
+++ b/app/Rooms/Index.alder
@@ -4,7 +4,7 @@
 
 page "/rooms" { } requires user
 
-type Model = { }
+type Model = { rows: List Row }
 
 type Row = { id: Room.RoomId, name: Text, free: Date }
 
app/Rooms/Index.alder:12:23: warning[W0002]: hole ?load has type Query Rooms.Index.Model
  load params session = ?load
                        ^^^^^
app/Rooms/Index.alder:15:22: warning[W0002]: hole ?view has type Html Rooms.Index.Msg
  view model session = ?view
                       ^^^^^
[exit 0]
$ alder new decl Rooms.Index rowOf "Room -> Query Row"
edited app/Rooms/Index.alder
--- a/app/Rooms/Index.alder
+++ b/app/Rooms/Index.alder
@@ -14,6 +14,9 @@
 view : Model -> Session -> Html Msg
 view model session = ?view
 
+rowOf : Room -> Query Row
+rowOf = ?rowOf
+
 test "a visitor is sent to log in" = {
   let r <- Test.open (Rooms.Index {})
   Task.succeed (r.answer == Redirected (Auth.Login {}))
app/Rooms/Index.alder:12:23: warning[W0002]: hole ?load has type Query Rooms.Index.Model
  load params session = ?load
                        ^^^^^
app/Rooms/Index.alder:15:22: warning[W0002]: hole ?view has type Html Rooms.Index.Msg
  view model session = ?view
                       ^^^^^
app/Rooms/Index.alder:18:9: warning[W0002]: hole ?rowOf has type Room -> Query Rooms.Index.Row
  rowOf = ?rowOf
          ^^^^^^
[exit 0]
$ alder fill Rooms.Index.rowOf - <<'EOF'
\r -> {
  let today <- Date.today
  let free <- Booking.freeFrom r.id today
  Query.succeed { id: r.id, name: Text.trim r.name, free: free }
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
@@ -15,7 +16,11 @@
 view model session = ?view
 
 rowOf : Room -> Query Row
-rowOf = ?rowOf
+rowOf = \r -> {
+  let today <- Date.today
+  let free <- Booking.freeFrom r.id today
+  Query.succeed { id: r.id, name: Text.trim r.name, free: free }
+}
 
 test "a visitor is sent to log in" = {
   let r <- Test.open (Rooms.Index {})
app/Rooms/Index.alder:13:23: warning[W0002]: hole ?load has type Query Rooms.Index.Model
  load params session = ?load
                        ^^^^^
app/Rooms/Index.alder:16:22: warning[W0002]: hole ?view has type Html Rooms.Index.Msg
  view model session = ?view
                       ^^^^^
added import Booking to app/Rooms/Index.alder
[exit 0]
$ alder fill --decl Rooms.Index.load - <<'EOF'
\params session -> {
  let rooms <- Db.from Room |> Db.orderBy Room.name.asc |> Db.all
  let rows <- Query.all (List.map rowOf rooms)
  Query.succeed { rows: rows }
}
EOF
app/Rooms/Index.alder:13:23: error[E0003]: expected Query Rooms.Index.Model, got _ -> _ -> Query { rows: List Rooms.Index.Row }
  load params session = \params session -> {
                        ^
app/Rooms/Index.alder:20:22: warning[W0002]: hole ?view has type Html Rooms.Index.Msg
  view model session = ?view
                       ^^^^^
alder fill: the result does not check (the errors above): nothing was written
[exit 1]
```

The harness stopped at line 60 of your script: `alder fill --decl Rooms.Index.load - <<'EOF'`: it exited 1. The commands after it were not run.

Your script stopped at command 20; the app is as the commands before it left it; continue.

The program type-checks but `alder build` refused it; fix the cause below.

```text
app/Auth/Login.alder:47:33: warning[W0002]: hole ?afterLogin has type Page
          Task.succeed (Redirect (?afterLogin))
                                  ^^^^^^^^^^^
app/Auth/Login.alder:86:85: warning[W0002]: hole ?afterLogin has type Page
    Task.succeed ({ answer: r.answer, user: r.session.user } == { answer: Redirected (?afterLogin), user: Just me })
                                                                                      ^^^^^^^^^^^
app/Rooms/Index.alder:13:23: warning[W0002]: hole ?load has type Query Rooms.Index.Model
  load params session = ?load
                        ^^^^^
app/Rooms/Index.alder:16:22: warning[W0002]: hole ?view has type Html Rooms.Index.Msg
  view model session = ?view
                       ^^^^^
alder build: the program has 4 typed hole(s); fill them before building
```
