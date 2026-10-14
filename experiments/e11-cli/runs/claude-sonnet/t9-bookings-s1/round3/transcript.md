### Round 3

```text
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
$ alder fill --decl 'Bookings.Show."pressing Cancel"' - <<'EOF'
{
  let today <- Date.today
  let me <- Db.insert User { email: Text.Email "b@example.com", name: Text.NonEmpty "B", passwordHash: "" }
  let _ <- Test.login { user: Just me }
  let room1 <- Db.insert Room { name: Text.NonEmpty "Green" }
  let booking1 <- Db.insert Booking { room: room1, owner: me, from: Date.addDays 3 today, to: Date.addDays 4 today }
  let _ <- Test.get (Bookings.Show { id: booking1 })
  let r <- Test.click Cancel
  let after <- Test.open (Bookings.Show { id: booking1 })
  Task.succeed ({ answer: r.answer, after: after.answer } == { answer: Redirected (Bookings.Index {}), after: PageMissing })
}
EOF
edited app/Bookings/Show.alder
--- a/app/Bookings/Show.alder
+++ b/app/Bookings/Show.alder
@@ -54,21 +54,16 @@
   Task.succeed (r.answer == Redirected (Auth.Login {}))
 }
 
--- Fill ?expected with what pressing Cancel must do, so that a test fails when the arm
--- does something else: its answer (`r.answer`: Rerendered, Redirected …) and what it
--- changed (`after`: the page opened again, `after.answer` Shown, or PageMissing when the arm
--- deleted the page's row, and `after.doc` its HTML), as
--- `{ answer: r.answer, … } == { answer: Rerendered, … }`.
 test "pressing Cancel" = {
   let today <- Date.today
-  let me <- Db.insert User { email: Text.Email "email-1@example.com", name: Text.NonEmpty "name-1", passwordHash: "passwordhash-1" }
+  let me <- Db.insert User { email: Text.Email "b@example.com", name: Text.NonEmpty "B", passwordHash: "" }
   let _ <- Test.login { user: Just me }
-  let room1 <- Db.insert Room { name: Text.NonEmpty "name-1" }
-  let booking1 <- Db.insert Booking { room: room1, owner: me, from: today, to: today }
+  let room1 <- Db.insert Room { name: Text.NonEmpty "Green" }
+  let booking1 <- Db.insert Booking { room: room1, owner: me, from: Date.addDays 3 today, to: Date.addDays 4 today }
   let _ <- Test.get (Bookings.Show { id: booking1 })
   let r <- Test.click Cancel
   let after <- Test.open (Bookings.Show { id: booking1 })
-  Task.succeed ?expected
+  Task.succeed ({ answer: r.answer, after: after.answer } == { answer: Redirected (Bookings.Index {}), after: PageMissing })
 }
 
 test "owner cancels before the from date" = {
rewriting a test: `alder test --mutate` is the check that it still tests something
[exit 0]
$ alder fill --list
[exit 0]
$ alder test
ok   App "Logout clears the user and goes to log in"
ok   Auth.Login "an unknown email is rejected"
ok   Auth.Login "logging in through the form"
ok   Auth.Login "a signed-in user logs out here"
ok   Booking "a Booking keeps its room from being deleted"
ok   Booking "a Booking keeps its owner from being deleted"
ok   Booking "next free date with back-to-back bookings"
ok   Bookings.Index "a visitor is sent to log in"
ok   Bookings.Show "a visitor is sent to log in"
ok   Bookings.Show "pressing Cancel"
ok   Bookings.Show "owner cancels before the from date"
ok   Bookings.Show "a booking starting today cannot be cancelled"
ok   Rooms.Index "a visitor is sent to log in"
ok   Rooms.Show "a visitor is sent to log in"
ok   Rooms.Show "submitting NewBooking"
ok   Rooms.Show "NewBooking refuses an empty from"
16 passed, 0 failed
[exit 0]
```

The harness ran `alder check`, `alder build` and `alder test` on the app: all pass.
