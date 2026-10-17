### Round 4

```text
$ alder fill Bookings.Index.view - --hole ?view <<'EOF'
<Layout.Page title="Bookings">
  <h1>Bookings</h1>
  <ul id="mine">
    {for b in model.rows -> <li><a href={Bookings.Show { id: b.booking.id }}><span class="room">{b.room.name}</span></a> <span class="from">{Date.toText b.booking.from}</span></li>}
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
 
 page "/bookings" { } requires user
 
@@ -13,7 +14,12 @@
 }
 
 view : Model -> Session -> Html Msg
-view model session = ?view
+view model session = <Layout.Page title="Bookings">
+  <h1>Bookings</h1>
+  <ul id="mine">
+    {for b in model.rows -> <li><a href={Bookings.Show { id: b.booking.id }}><span class="room">{b.room.name}</span></a> <span class="from">{Date.toText b.booking.from}</span></li>}
+  </ul>
+</Layout.Page>
 
 test "a visitor is sent to log in" = {
   let r <- Test.open (Bookings.Index {})
added import Layout to app/Bookings/Index.alder
[exit 0]
$ alder fill Bookings.Show.view - --hole ?view <<'EOF'
<Layout.Page title={model.room.name}>
  <h1 id="room">{model.room.name}</h1>
  <p>From <span id="from">{Date.toText model.booking.from}</span> to <span id="to">{Date.toText model.booking.to}</span></p>
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
 
@@ -15,7 +16,10 @@
 }
 
 view : Model -> Session -> Html Msg
-view model session = ?view
+view model session = <Layout.Page title={model.room.name}>
+  <h1 id="room">{model.room.name}</h1>
+  <p>From <span id="from">{Date.toText model.booking.from}</span> to <span id="to">{Date.toText model.booking.to}</span></p>
+</Layout.Page>
 
 test "a visitor is sent to log in" = {
   let today <- Date.today
added import Layout to app/Bookings/Show.alder
[exit 0]
$ alder new msg Bookings.Show Cancel
edited app/Bookings/Show.alder
--- a/app/Bookings/Show.alder
+++ b/app/Bookings/Show.alder
@@ -15,10 +15,19 @@
   Query.succeed { booking: p.booking, room: p.room, cancellable: p.cancellable }
 }
 
+msg Cancel
+
+update : Msg -> Model -> Session -> Task Nav
+update msg model session = match msg {
+  Cancel p -> ?cancel,
+}
+
 view : Model -> Session -> Html Msg
 view model session = <Layout.Page title={model.room.name}>
   <h1 id="room">{model.room.name}</h1>
   <p>From <span id="from">{Date.toText model.booking.from}</span> to <span id="to">{Date.toText model.booking.to}</span></p>
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
app/Bookings/Show.alder:22:15: warning[W0002]: hole ?cancel has type Task Nav
    Cancel p -> ?cancel,
                ^^^^^^^
app/Bookings/Show.alder:56:16: warning[W0002]: hole ?expected has type Bool
    Task.succeed ?expected
                 ^^^^^^^^^
[exit 0]
$ alder fill --decl Bookings.Show.update - <<'EOF'
match msg {
  Cancel p -> {
    if model.cancellable then {
      let _ <- Db.delete Booking model.booking.id
      Task.succeed (Redirect (Bookings.Index {}))
    } else Task.succeed Rerender
  },
}
EOF
edited app/Bookings/Show.alder
--- a/app/Bookings/Show.alder
+++ b/app/Bookings/Show.alder
@@ -19,7 +19,12 @@
 
 update : Msg -> Model -> Session -> Task Nav
 update msg model session = match msg {
-  Cancel p -> ?cancel,
+  Cancel p -> {
+    if model.cancellable then {
+      let _ <- Db.delete Booking model.booking.id
+      Task.succeed (Redirect (Bookings.Index {}))
+    } else Task.succeed Rerender
+  },
 }
 
 view : Model -> Session -> Html Msg
app/Bookings/Show.alder:61:16: warning[W0002]: hole ?expected has type Bool
    Task.succeed ?expected
                 ^^^^^^^^^
[exit 0]
$ alder fill --decl Bookings.Show.view - <<'EOF'
<Layout.Page title={model.room.name}>
  <h1 id="room">{model.room.name}</h1>
  <p>From <span id="from">{Date.toText model.booking.from}</span> to <span id="to">{Date.toText model.booking.to}</span></p>
  {if model.cancellable then <al-button id="cancel" variant="destructive" click={Cancel}>Cancel</al-button> else Html.none}
</Layout.Page>
EOF
edited app/Bookings/Show.alder
--- a/app/Bookings/Show.alder
+++ b/app/Bookings/Show.alder
@@ -28,12 +28,12 @@
 }
 
 view : Model -> Session -> Html Msg
-view model session = <Layout.Page title={model.room.name}>
-  <h1 id="room">{model.room.name}</h1>
-  <p>From <span id="from">{Date.toText model.booking.from}</span> to <span id="to">{Date.toText model.booking.to}</span></p>
-
-  <al-button click={Cancel}>Cancel</al-button>
-</Layout.Page>
+view model session =
+  <Layout.Page title={model.room.name}>
+    <h1 id="room">{model.room.name}</h1>
+    <p>From <span id="from">{Date.toText model.booking.from}</span> to <span id="to">{Date.toText model.booking.to}</span></p>
+    {if model.cancellable then <al-button id="cancel" variant="destructive" click={Cancel}>Cancel</al-button> else Html.none}
+  </Layout.Page>
 
 test "a visitor is sent to log in" = {
   let today <- Date.today
app/Bookings/Show.alder:61:16: warning[W0002]: hole ?expected has type Bool
    Task.succeed ?expected
                 ^^^^^^^^^
[exit 0]
$ alder new test Bookings.Show "the owner cancels a booking before its from date"
edited app/Bookings/Show.alder
--- a/app/Bookings/Show.alder
+++ b/app/Bookings/Show.alder
@@ -60,3 +60,7 @@
   let after <- Test.open (Bookings.Show { id: booking1 })
   Task.succeed ?expected
 }
+
+test "the owner cancels a booking before its from date" = {
+  ?test
+}
app/Bookings/Show.alder:61:16: warning[W0002]: hole ?expected has type Bool
    Task.succeed ?expected
                 ^^^^^^^^^
app/Bookings/Show.alder:65:3: warning[W0002]: hole ?test has type Task Bool
    ?test
    ^^^^^
[exit 0]
$ alder fill 'Bookings.Show."the owner cancels a booking before its from date"' - <<'EOF'
{
  let today <- Date.today
  let me <- Db.insert User { email: Text.Email "cancel-owner@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
  let room <- Db.insert Room { name: Text.NonEmpty "Oak" }
  let id <- Db.insert Booking { room: room, owner: me, from: Date.addDays 2 today, to: Date.addDays 3 today }
  let _ <- Test.login { user: Just me }
  let page <- Test.get (Bookings.Show { id: id })
  let r <- Test.click Cancel
  let n <- Db.from Booking |> Db.count
  Task.succeed ({ shown: Test.has "#cancel" page.doc, answer: r.answer, bookings: n } == { shown: True, answer: Redirected (Bookings.Index {}), bookings: 0 })
}
EOF
edited app/Bookings/Show.alder
--- a/app/Bookings/Show.alder
+++ b/app/Bookings/Show.alder
@@ -62,5 +62,13 @@
 }
 
 test "the owner cancels a booking before its from date" = {
-  ?test
+  let today <- Date.today
+  let me <- Db.insert User { email: Text.Email "cancel-owner@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
+  let room <- Db.insert Room { name: Text.NonEmpty "Oak" }
+  let id <- Db.insert Booking { room: room, owner: me, from: Date.addDays 2 today, to: Date.addDays 3 today }
+  let _ <- Test.login { user: Just me }
+  let page <- Test.get (Bookings.Show { id: id })
+  let r <- Test.click Cancel
+  let n <- Db.from Booking |> Db.count
+  Task.succeed ({ shown: Test.has "#cancel" page.doc, answer: r.answer, bookings: n } == { shown: True, answer: Redirected (Bookings.Index {}), bookings: 0 })
 }
app/Bookings/Show.alder:61:16: warning[W0002]: hole ?expected has type Bool
    Task.succeed ?expected
                 ^^^^^^^^^
[exit 0]
$ alder new test Bookings.Show "a booking whose from date is today cannot be cancelled"
edited app/Bookings/Show.alder
--- a/app/Bookings/Show.alder
+++ b/app/Bookings/Show.alder
@@ -72,3 +72,7 @@
   let n <- Db.from Booking |> Db.count
   Task.succeed ({ shown: Test.has "#cancel" page.doc, answer: r.answer, bookings: n } == { shown: True, answer: Redirected (Bookings.Index {}), bookings: 0 })
 }
+
+test "a booking whose from date is today cannot be cancelled" = {
+  ?test
+}
app/Bookings/Show.alder:61:16: warning[W0002]: hole ?expected has type Bool
    Task.succeed ?expected
                 ^^^^^^^^^
app/Bookings/Show.alder:77:3: warning[W0002]: hole ?test has type Task Bool
    ?test
    ^^^^^
[exit 0]
$ alder fill 'Bookings.Show."a booking whose from date is today cannot be cancelled"' - <<'EOF'
{
  let today <- Date.today
  let me <- Db.insert User { email: Text.Email "today-owner@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
  let room <- Db.insert Room { name: Text.NonEmpty "Oak" }
  let id <- Db.insert Booking { room: room, owner: me, from: today, to: Date.addDays 1 today }
  let _ <- Test.login { user: Just me }
  let page <- Test.get (Bookings.Show { id: id })
  let r <- Test.run (update Cancel page.model { user: me })
  let n <- Db.from Booking |> Db.count
  Task.succeed ({ shown: Test.has "#cancel" page.doc, refused: r.value == Ok Rerender, bookings: n } == { shown: False, refused: True, bookings: 1 })
}
EOF
edited app/Bookings/Show.alder
--- a/app/Bookings/Show.alder
+++ b/app/Bookings/Show.alder
@@ -74,5 +74,13 @@
 }
 
 test "a booking whose from date is today cannot be cancelled" = {
-  ?test
+  let today <- Date.today
+  let me <- Db.insert User { email: Text.Email "today-owner@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
+  let room <- Db.insert Room { name: Text.NonEmpty "Oak" }
+  let id <- Db.insert Booking { room: room, owner: me, from: today, to: Date.addDays 1 today }
+  let _ <- Test.login { user: Just me }
+  let page <- Test.get (Bookings.Show { id: id })
+  let r <- Test.run (update Cancel page.model { user: me })
+  let n <- Db.from Booking |> Db.count
+  Task.succeed ({ shown: Test.has "#cancel" page.doc, refused: r.value == Ok Rerender, bookings: n } == { shown: False, refused: True, bookings: 1 })
 }
app/Bookings/Show.alder:61:16: warning[W0002]: hole ?expected has type Bool
    Task.succeed ?expected
                 ^^^^^^^^^
[exit 0]
$ alder new test Bookings.Show "a stranger cannot open a booking"
edited app/Bookings/Show.alder
--- a/app/Bookings/Show.alder
+++ b/app/Bookings/Show.alder
@@ -84,3 +84,7 @@
   let n <- Db.from Booking |> Db.count
   Task.succeed ({ shown: Test.has "#cancel" page.doc, refused: r.value == Ok Rerender, bookings: n } == { shown: False, refused: True, bookings: 1 })
 }
+
+test "a stranger cannot open a booking" = {
+  ?test
+}
app/Bookings/Show.alder:61:16: warning[W0002]: hole ?expected has type Bool
    Task.succeed ?expected
                 ^^^^^^^^^
app/Bookings/Show.alder:89:3: warning[W0002]: hole ?test has type Task Bool
    ?test
    ^^^^^
[exit 0]
$ alder fill 'Bookings.Show."a stranger cannot open a booking"' - <<'EOF'
{
  let today <- Date.today
  let me <- Db.insert User { email: Text.Email "owner-2@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
  let stranger <- Db.insert User { email: Text.Email "stranger@example.com", name: Text.NonEmpty "Other", passwordHash: "" }
  let room <- Db.insert Room { name: Text.NonEmpty "Oak" }
  let id <- Db.insert Booking { room: room, owner: me, from: Date.addDays 2 today, to: Date.addDays 3 today }
  let _ <- Test.login { user: Just stranger }
  let r <- Test.open (Bookings.Show { id: id })
  Task.succeed (r.answer == PageMissing)
}
EOF
edited app/Bookings/Show.alder
--- a/app/Bookings/Show.alder
+++ b/app/Bookings/Show.alder
@@ -86,5 +86,12 @@
 }
 
 test "a stranger cannot open a booking" = {
-  ?test
+  let today <- Date.today
+  let me <- Db.insert User { email: Text.Email "owner-2@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
+  let stranger <- Db.insert User { email: Text.Email "stranger@example.com", name: Text.NonEmpty "Other", passwordHash: "" }
+  let room <- Db.insert Room { name: Text.NonEmpty "Oak" }
+  let id <- Db.insert Booking { room: room, owner: me, from: Date.addDays 2 today, to: Date.addDays 3 today }
+  let _ <- Test.login { user: Just stranger }
+  let r <- Test.open (Bookings.Show { id: id })
+  Task.succeed (r.answer == PageMissing)
 }
app/Bookings/Show.alder:61:16: warning[W0002]: hole ?expected has type Bool
    Task.succeed ?expected
                 ^^^^^^^^^
[exit 0]
$ alder new test Booking "a booking that overlaps another is refused and not stored"
edited app/Booking.alder
--- a/app/Booking.alder
+++ b/app/Booking.alder
@@ -136,3 +136,7 @@
   let second <- nextFree room (Date.addDays 1 today)
   Task.succeed ({ first: first, second: second } == { first: today, second: Date.addDays 5 today })
 }
+
+test "a booking that overlaps another is refused and not stored" = {
+  ?test
+}
app/Booking.alder:141:3: warning[W0002]: hole ?test has type Task Bool
    ?test
    ^^^^^
[exit 0]
$ alder fill 'Booking."a booking that overlaps another is refused and not stored"' - <<'EOF'
{
  let today <- Date.today
  let me <- Db.insert User { email: Text.Email "booker@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
  let room <- Db.insert Room { name: Text.NonEmpty "Oak" }
  let first <- book room me (Date.addDays 1 today) (Date.addDays 4 today)
  let second <- book room me (Date.addDays 3 today) (Date.addDays 6 today)
  let n <- Db.from Booking |> Db.count
  Task.succeed ({ first: first, second: second, bookings: n } == { first: True, second: False, bookings: 1 })
}
EOF
edited app/Booking.alder
--- a/app/Booking.alder
+++ b/app/Booking.alder
@@ -138,5 +138,11 @@
 }
 
 test "a booking that overlaps another is refused and not stored" = {
-  ?test
+  let today <- Date.today
+  let me <- Db.insert User { email: Text.Email "booker@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
+  let room <- Db.insert Room { name: Text.NonEmpty "Oak" }
+  let first <- book room me (Date.addDays 1 today) (Date.addDays 4 today)
+  let second <- book room me (Date.addDays 3 today) (Date.addDays 6 today)
+  let n <- Db.from Booking |> Db.count
+  Task.succeed ({ first: first, second: second, bookings: n } == { first: True, second: False, bookings: 1 })
 }
[exit 0]
$ alder new test Booking "a booking may start on the day another ends"
edited app/Booking.alder
--- a/app/Booking.alder
+++ b/app/Booking.alder
@@ -146,3 +146,7 @@
   let n <- Db.from Booking |> Db.count
   Task.succeed ({ first: first, second: second, bookings: n } == { first: True, second: False, bookings: 1 })
 }
+
+test "a booking may start on the day another ends" = {
+  ?test
+}
app/Booking.alder:151:3: warning[W0002]: hole ?test has type Task Bool
    ?test
    ^^^^^
[exit 0]
$ alder fill 'Booking."a booking may start on the day another ends"' - <<'EOF'
{
  let today <- Date.today
  let me <- Db.insert User { email: Text.Email "back-to-back@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
  let room <- Db.insert Room { name: Text.NonEmpty "Oak" }
  let first <- book room me (Date.addDays 1 today) (Date.addDays 3 today)
  let second <- book room me (Date.addDays 3 today) (Date.addDays 5 today)
  let n <- Db.from Booking |> Db.count
  Task.succeed ({ first: first, second: second, bookings: n } == { first: True, second: True, bookings: 2 })
}
EOF
edited app/Booking.alder
--- a/app/Booking.alder
+++ b/app/Booking.alder
@@ -148,5 +148,11 @@
 }
 
 test "a booking may start on the day another ends" = {
-  ?test
+  let today <- Date.today
+  let me <- Db.insert User { email: Text.Email "back-to-back@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
+  let room <- Db.insert Room { name: Text.NonEmpty "Oak" }
+  let first <- book room me (Date.addDays 1 today) (Date.addDays 3 today)
+  let second <- book room me (Date.addDays 3 today) (Date.addDays 5 today)
+  let n <- Db.from Booking |> Db.count
+  Task.succeed ({ first: first, second: second, bookings: n } == { first: True, second: True, bookings: 2 })
 }
[exit 0]
$ alder test
app/Bookings/Show.alder:61:16: warning[W0002]: hole ?expected has type Bool
    Task.succeed ?expected
                 ^^^^^^^^^
alder test: no tests ran (fill the hole): ?expected : Bool
[exit 1]
```

The harness stopped at line 109 of your script: `alder test`: it exited 1. The commands after it were not run.

Your script stopped at command 16; the app is as the commands before it left it; continue.

The program type-checks but `alder build` refused it; fix the cause below.

```text
app/Bookings/Show.alder:61:16: warning[W0002]: hole ?expected has type Bool
    Task.succeed ?expected
                 ^^^^^^^^^
alder build: the program has 1 typed hole(s); fill them before building
```
