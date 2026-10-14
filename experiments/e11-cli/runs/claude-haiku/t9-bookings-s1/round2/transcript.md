### Round 2

```text
$ alder fill 'Rooms.Show."submitting NewBooking"' - --hole ?expected <<'EOF'
FormRejected [{ field: "", message: "Those dates cannot be booked." }]
EOF
edited app/Rooms/Show.alder
--- a/app/Rooms/Show.alder
+++ b/app/Rooms/Show.alder
@@ -77,7 +77,7 @@
   let room1 <- Db.insert Room { name: Text.NonEmpty "name-1" }
   let _ <- Test.get (Rooms.Show { id: room1 })
   let r <- Test.submit NewBooking { from: today, to: today }
-  Task.succeed (r.answer == ?expected)
+  Task.succeed (r.answer == FormRejected [{ field: "", message: "Those dates cannot be booked." }])
 }
 
 test "NewBooking refuses an empty from" = {
[exit 0]
$ alder fill 'Rooms.Show."NewBooking refuses an empty from"' - <<'EOF'
{
  let me <- Db.insert User { email: Text.Email "email-1@example.com", name: Text.NonEmpty "name-1", passwordHash: "passwordhash-1" }
  let _ <- Test.login { user: Just me }
  let room1 <- Db.insert Room { name: Text.NonEmpty "name-1" }
  let _ <- Test.get (Rooms.Show { id: room1 })
  let r <- Test.submitRaw NewBooking [{ name: "from", value: "" }, { name: "to", value: "2026-01-01" }]
  Task.succeed (r.answer == FormRejected [{ field: "", message: "from must be a date (YYYY-MM-DD)" }])
}
EOF
alder fill: `Rooms.Show."NewBooking refuses an empty from"` has no typed hole (`alder fill --list` lists the app's holes)
[exit 1]
```

The harness stopped at line 7 of your script: `alder fill 'Rooms.Show."NewBooking refuses an empty from"' - <<'EOF'`: it exited 1. The commands after it were not run.

Your script stopped at command 2; the app is as the commands before it left it; continue.

The program type-checks but `alder build` refused it; fix the cause below.

```text
app/Bookings/Show.alder:72:16: warning[W0002]: hole ?expected has type Bool
    Task.succeed ?expected
                 ^^^^^^^^^
alder build: the program has 1 typed hole(s); fill them before building
```
