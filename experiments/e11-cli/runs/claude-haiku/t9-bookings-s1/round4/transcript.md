### Round 4

```text
$ alder fill --decl 'Rooms.Show."NewBooking refuses an empty from"' - <<'EOF'
{
  let me <- Db.insert User { email: Text.Email "email-1@example.com", name: Text.NonEmpty "name-1", passwordHash: "passwordhash-1" }
  let _ <- Test.login { user: Just me }
  let room1 <- Db.insert Room { name: Text.NonEmpty "name-1" }
  let _ <- Test.get (Rooms.Show { id: room1 })
  let r <- Test.submitRaw NewBooking [{ name: "from", value: "" }, { name: "to", value: "2026-01-01" }]
  Task.succeed (r.answer == FormRejected [{ field: "from", message: "is required" }])
}
EOF
edited app/Rooms/Show.alder
--- a/app/Rooms/Show.alder
+++ b/app/Rooms/Show.alder
@@ -86,7 +86,7 @@
   let room1 <- Db.insert Room { name: Text.NonEmpty "name-1" }
   let _ <- Test.get (Rooms.Show { id: room1 })
   let r <- Test.submitRaw NewBooking [{ name: "from", value: "" }, { name: "to", value: "2026-01-01" }]
-  Task.succeed (r.answer == FormRejected [{ field: "", message: "from must be a date (YYYY-MM-DD)" }])
+  Task.succeed (r.answer == FormRejected [{ field: "from", message: "is required" }])
 }
 
 test "an overlapping booking is refused and not stored" = {
rewriting a test: `alder test --mutate` is the check that it still tests something
[exit 0]
$ alder test
ok   App "Logout clears the user and goes to log in"
ok   Auth.Login "an unknown email is rejected"
ok   Auth.Login "logging in through the form"
ok   Auth.Login "a signed-in user logs out here"
ok   Booking "a Booking keeps its room from being deleted"
ok   Booking "a Booking keeps its owner from being deleted"
ok   Booking "the next free date skips back-to-back bookings"
ok   Bookings.Index "a visitor is sent to log in"
ok   Bookings.Show "a visitor is sent to log in"
ok   Bookings.Show "pressing Cancel"
ok   Bookings.Show "the owner cancels a booking before its first day"
ok   Bookings.Show "a booking from today cannot be cancelled"
ok   Rooms.Index "a visitor is sent to log in"
ok   Rooms.Show "a visitor is sent to log in"
ok   Rooms.Show "submitting NewBooking"
ok   Rooms.Show "NewBooking refuses an empty from"
ok   Rooms.Show "an overlapping booking is refused and not stored"
ok   Rooms.Show "a booking starting on the other end date is stored"
18 passed, 0 failed
[exit 0]
```

The harness ran `alder check`, `alder build` and `alder test` on the app: all pass.
