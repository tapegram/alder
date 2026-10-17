```alder-cli
# the last hole: pressing Cancel on an open booking (its from date is in the future,
# so the button is shown), then the booking is gone
alder fill --decl 'Bookings.Show."pressing Cancel"' - <<'EOF'
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
alder check
alder test
```
