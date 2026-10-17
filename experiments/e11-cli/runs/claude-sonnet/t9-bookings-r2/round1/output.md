```alder-cli
alder fill --decl 'Bookings.Show."pressing Cancel"' - <<'EOF'
{
  let today <- Date.today
  let me <- Db.insert User { email: Text.Email "email-1@example.com", name: Text.NonEmpty "name-1", passwordHash: "passwordhash-1" }
  let _ <- Test.login { user: Just me }
  let room1 <- Db.insert Room { name: Text.NonEmpty "name-1" }
  let booking1 <- Db.insert Booking { room: room1, owner: me, from: Date.addDays 3 today, to: Date.addDays 5 today }
  let _ <- Test.get (Bookings.Show { id: booking1 })
  let r <- Test.click Cancel
  let after <- Test.open (Bookings.Show { id: booking1 })
  Task.succeed ({ answer: r.answer, after: after.answer } == { answer: Redirected (Bookings.Index {}), after: PageMissing })
}
EOF
alder fill --decl 'Rooms.Show."submitting NewBooking"' - <<'EOF'
{
  let today <- Date.today
  let me <- Db.insert User { email: Text.Email "email-1@example.com", name: Text.NonEmpty "name-1", passwordHash: "passwordhash-1" }
  let _ <- Test.login { user: Just me }
  let room1 <- Db.insert Room { name: Text.NonEmpty "name-1" }
  let _ <- Test.get (Rooms.Show { id: room1 })
  let r <- Test.submit NewBooking { from: today, to: Date.addDays 1 today }
  Task.succeed (r.answer == Redirected (Rooms.Show { id: room1 }))
}
EOF
alder fill --list --json
```
