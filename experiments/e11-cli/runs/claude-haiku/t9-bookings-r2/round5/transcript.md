### Round 5

```text
$ alder fill --decl 'Bookings.Show."pressing Cancel"' - <<'EOF'
{
  let today <- Date.today
  let me <- Db.insert User { email: Text.Email "press-cancel@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
  let _ <- Test.login { user: Just me }
  let room1 <- Db.insert Room { name: Text.NonEmpty "Oak" }
  let booking1 <- Db.insert Booking { room: room1, owner: me, from: Date.addDays 2 today, to: Date.addDays 3 today }
  let _ <- Test.get (Bookings.Show { id: booking1 })
  let r <- Test.click Cancel
  let again <- Test.open (Bookings.Show { id: booking1 })
  Task.succeed ({ answer: r.answer, again: again.answer } == { answer: Redirected (Bookings.Index {}), again: PageMissing })
}
EOF
edited app/Bookings/Show.alder
--- a/app/Bookings/Show.alder
+++ b/app/Bookings/Show.alder
@@ -44,21 +44,16 @@
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
+  let me <- Db.insert User { email: Text.Email "press-cancel@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
   let _ <- Test.login { user: Just me }
-  let room1 <- Db.insert Room { name: Text.NonEmpty "name-1" }
-  let booking1 <- Db.insert Booking { room: room1, owner: me, from: today, to: today }
+  let room1 <- Db.insert Room { name: Text.NonEmpty "Oak" }
+  let booking1 <- Db.insert Booking { room: room1, owner: me, from: Date.addDays 2 today, to: Date.addDays 3 today }
   let _ <- Test.get (Bookings.Show { id: booking1 })
   let r <- Test.click Cancel
-  let after <- Test.open (Bookings.Show { id: booking1 })
-  Task.succeed ?expected
+  let again <- Test.open (Bookings.Show { id: booking1 })
+  Task.succeed ({ answer: r.answer, again: again.answer } == { answer: Redirected (Bookings.Index {}), again: PageMissing })
 }
 
 test "the owner cancels a booking before its from date" = {
rewriting a test: `alder test --mutate` is the check that it still tests something
[exit 0]
$ alder check
[exit 0]
$ alder test
ok   App "Logout clears the user and goes to log in"
ok   Auth.Login "an unknown email is rejected"
ok   Auth.Login "logging in through the form"
ok   Auth.Login "a signed-in user logs out here"
ok   Booking "a Booking keeps its room from being deleted"
ok   Booking "a Booking keeps its owner from being deleted"
ok   Booking "the next free date skips back-to-back bookings"
ok   Booking "a booking that overlaps another is refused and not stored"
ok   Booking "a booking may start on the day another ends"
ok   Bookings.Index "a visitor is sent to log in"
ok   Bookings.Show "a visitor is sent to log in"
ok   Bookings.Show "pressing Cancel"
ok   Bookings.Show "the owner cancels a booking before its from date"
ok   Bookings.Show "a booking whose from date is today cannot be cancelled"
ok   Bookings.Show "a stranger cannot open a booking"
ok   Rooms.Index "a visitor is sent to log in"
ok   Rooms.Show "a visitor is sent to log in"
ok   Rooms.Show "submitting NewBooking"
ok   Rooms.Show "NewBooking refuses an empty from"
19 passed, 0 failed
[exit 0]
```

The harness ran `alder check`, `alder build` and `alder test` on the app: all pass.
