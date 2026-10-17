### Round 2

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
$ alder test
ok   App "Logout clears the user and goes to log in"
ok   Auth.Login "an unknown email is rejected"
ok   Auth.Login "logging in through the form"
ok   Auth.Login "a signed-in user logs out here"
ok   Booking "a Booking keeps its room from being deleted"
ok   Booking "a Booking keeps its owner from being deleted"
ok   Bookings.Index "a visitor is sent to log in"
ok   Bookings.Show "a visitor is sent to log in"
ok   Bookings.Show "pressing Cancel"
ok   Bookings.Show "the owner cancels a booking before its from date"
ok   Bookings.Show "a booking whose from date is today cannot be cancelled"
ok   Bookings.Show "a stranger gets a 404"
ok   Rooms.Index "a visitor is sent to log in"
ok   Rooms.Index "the next free date of a room with back-to-back bookings"
ok   Rooms.Show "a visitor is sent to log in"
ok   Rooms.Show "submitting NewBooking"
ok   Rooms.Show "NewBooking refuses an empty from"
ok   Rooms.Show "an overlapping booking is refused and not stored"
ok   Rooms.Show "a booking starting on the other's end date is stored"
ok   Rooms.Show "a booking whose to is not after its from is refused"
20 passed, 0 failed
[exit 0]
```

The harness ran `alder check`, `alder build` and `alder test` on the app: all pass.
