```alder-cli
alder fill 'Auth.Login."logging in through the form"' - --hole ?afterLogin <<'EOF'
Rooms.Index {}
EOF
alder fill --decl 'Bookings.Show."pressing Cancel"' - <<'EOF'
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
alder fill --list
alder test
```
