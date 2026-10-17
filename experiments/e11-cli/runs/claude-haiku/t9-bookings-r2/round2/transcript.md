### Round 2

```text
$ alder fill --decl Rooms.Show.update - <<'EOF'
match msg {
  Book p -> {
    let ok <- Booking.book model.room.id session.user p.form.from p.form.to
    if ok then Task.succeed (Redirect (Rooms.Show { id: model.room.id })) else Task.succeed (Reject "Those dates are not free: the booking must end after it starts and not overlap another booking.")
  },
}
EOF
edited app/Rooms/Show.alder
--- a/app/Rooms/Show.alder
+++ b/app/Rooms/Show.alder
@@ -26,7 +26,7 @@
 update msg model session = match msg {
   Book p -> {
     let ok <- Booking.book model.room.id session.user p.form.from p.form.to
-    if ok then Task.succeed (Redirect (Rooms.Show { id: model.room.id })) else Task.succeed (Reject "The room is not free for those dates.")
+    if ok then Task.succeed (Redirect (Rooms.Show { id: model.room.id })) else Task.succeed (Reject "Those dates are not free: the booking must end after it starts and not overlap another booking.")
   },
 }
 
app/Rooms/Show.alder:63:29: warning[W0002]: hole ?expected has type Answer
    Task.succeed (r.answer == ?expected)
                              ^^^^^^^^^
[exit 0]
$ alder fill 'Rooms.Show."submitting NewBooking"' - <<'EOF'
{
  let today <- Date.today
  let me <- Db.insert User { email: Text.Email "email-1@example.com", name: Text.NonEmpty "name-1", passwordHash: "passwordhash-1" }
  let _ <- Test.login { user: Just me }
  let room1 <- Db.insert Room { name: Text.NonEmpty "name-1" }
  let _ <- Test.get (Rooms.Show { id: room1 })
  let r <- Test.submit NewBooking { from: today, to: today }
  Task.succeed (r.answer == FormRejected [{ field: "", message: "Those dates are not free: the booking must end after it starts and not overlap another booking." }])
}
EOF
alder fill: the hole ?expected is a `Answer`, not a test's whole block: send the expression for it, or, to replace the whole test, use `alder fill --decl 'Rooms.Show."submitting NewBooking"' -`
[exit 1]
```

The harness stopped at line 10 of your script: `alder fill 'Rooms.Show."submitting NewBooking"' - <<'EOF'`: it exited 1. The commands after it were not run.

Your script stopped at command 2; the app is as the commands before it left it; continue.

The program type-checks but `alder build` refused it; fix the cause below.

```text
app/Auth/Login.alder:86:85: warning[W0002]: hole ?afterLogin has type Page
    Task.succeed ({ answer: r.answer, user: r.session.user } == { answer: Redirected (?afterLogin), user: Just me })
                                                                                      ^^^^^^^^^^^
app/Bookings/Index.alder:16:22: warning[W0002]: hole ?view has type Html Bookings.Index.Msg
  view model session = ?view
                       ^^^^^
app/Rooms/Show.alder:63:29: warning[W0002]: hole ?expected has type Answer
    Task.succeed (r.answer == ?expected)
                              ^^^^^^^^^
alder build: the program has 3 typed hole(s); fill them before building
```
