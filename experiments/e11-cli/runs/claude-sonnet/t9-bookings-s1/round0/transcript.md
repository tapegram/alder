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
$ alder new decl Booking nextFree "Date -> List Booking -> Date"
edited app/Booking.alder
--- a/app/Booking.alder
+++ b/app/Booking.alder
@@ -13,6 +13,9 @@
   to: Date,
 }
 
+nextFree : Date -> List Booking -> Date
+nextFree = ?nextFree
+
 test "a Booking keeps its room from being deleted" = {
   let today <- Date.today
   let target <- Db.insert Room { name: Text.NonEmpty "name-1" }
app/Booking.alder:17:12: warning[W0002]: hole ?nextFree has type Date -> List Booking -> Date
  nextFree = ?nextFree
             ^^^^^^^^^
[exit 0]
$ alder fill Booking.nextFree - <<'EOF'
\today bs -> List.foldl (\b d -> if b.from <= d && d < b.to then b.to else d) today (List.sortByText (\b -> Date.toText b.from) bs)
EOF
edited app/Booking.alder
--- a/app/Booking.alder
+++ b/app/Booking.alder
@@ -14,7 +14,7 @@
 }
 
 nextFree : Date -> List Booking -> Date
-nextFree = ?nextFree
+nextFree = \today bs -> List.foldl (\b d -> if b.from <= d && d < b.to then b.to else d) today (List.sortByText (\b -> Date.toText b.from) bs)
 
 test "a Booking keeps its room from being deleted" = {
   let today <- Date.today
[exit 0]
$ alder new decl Booking conflicts "Date -> Date -> List Booking -> Bool"
edited app/Booking.alder
--- a/app/Booking.alder
+++ b/app/Booking.alder
@@ -16,6 +16,9 @@
 nextFree : Date -> List Booking -> Date
 nextFree = \today bs -> List.foldl (\b d -> if b.from <= d && d < b.to then b.to else d) today (List.sortByText (\b -> Date.toText b.from) bs)
 
+conflicts : Date -> Date -> List Booking -> Bool
+conflicts = ?conflicts
+
 test "a Booking keeps its room from being deleted" = {
   let today <- Date.today
   let target <- Db.insert Room { name: Text.NonEmpty "name-1" }
app/Booking.alder:20:13: warning[W0002]: hole ?conflicts has type Date -> Date -> List Booking -> Bool
  conflicts = ?conflicts
              ^^^^^^^^^^
[exit 0]
$ alder fill Booking.conflicts - <<'EOF'
\f t bs -> List.any (\b -> f < b.to && b.from < t) bs
EOF
edited app/Booking.alder
--- a/app/Booking.alder
+++ b/app/Booking.alder
@@ -17,7 +17,7 @@
 nextFree = \today bs -> List.foldl (\b d -> if b.from <= d && d < b.to then b.to else d) today (List.sortByText (\b -> Date.toText b.from) bs)
 
 conflicts : Date -> Date -> List Booking -> Bool
-conflicts = ?conflicts
+conflicts = \f t bs -> List.any (\b -> f < b.to && b.from < t) bs
 
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
$ alder fill --decl Rooms.Index.Model - <<'EOF'
{ rooms: List { id: Room.RoomId, name: Text, free: Date } }
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
+type Model = { rooms: List { id: Room.RoomId, name: Text, free: Date } }
 
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
  Query.succeed { rooms: List.map (\r -> { id: r.id, name: r.name, free: Booking.nextFree today (List.filter (\b -> b.room == r.id) bookings) }) rooms }
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
 type Model = { rooms: List { id: Room.RoomId, name: Text, free: Date } }
 
 load : Params -> Session -> Query Model
-load params session = ?load
+load params session = {
+  let rooms <- Db.from Room |> Db.orderBy Room.name.asc |> Db.all
+  let bookings <- Db.from Booking |> Db.all
+  let today <- Date.today
+  Query.succeed { rooms: List.map (\r -> { id: r.id, name: r.name, free: Booking.nextFree today (List.filter (\b -> b.room == r.id) bookings) }) rooms }
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
    {for r in model.rooms ->
      <li><a class="name" href={Rooms.Show { id: r.id }}>{r.name}</a> <span class="free">{Date.toText r.free}</span></li>}
  </ul>
</Layout.Page>
EOF
app/Rooms/Index.alder:24:33: error[E0002]: unknown `Rooms.Show`
        <li><a class="name" href={Rooms.Show { id: r.id }}>{r.name}</a> <span class="free">{Date.toText r.free}</span></li>}
                                  ^^^^^^^^^^
alder fill: the result does not check (the errors above): nothing was written
[exit 1]
```

The harness stopped at line 32 of your script: `alder fill Rooms.Index.view - <<'EOF'`: it exited 1. The commands after it were not run.

Your script stopped at command 12; the app is as the commands before it left it; continue.

The program type-checks but `alder build` refused it; fix the cause below.

```text
app/Auth/Login.alder:47:33: warning[W0002]: hole ?afterLogin has type Page
          Task.succeed (Redirect (?afterLogin))
                                  ^^^^^^^^^^^
app/Auth/Login.alder:86:85: warning[W0002]: hole ?afterLogin has type Page
    Task.succeed ({ answer: r.answer, user: r.session.user } == { answer: Redirected (?afterLogin), user: Just me })
                                                                                      ^^^^^^^^^^^
app/Rooms/Index.alder:19:22: warning[W0002]: hole ?view has type Html Rooms.Index.Msg
  view model session = ?view
                       ^^^^^
alder build: the program has 3 typed hole(s); fill them before building
```
