```alder-cli
# the to-do-free booking form: a same-day booking is refused, so the form's message is shown
alder fill 'Rooms.Show."submitting NewBooking"' - --hole ?expected <<'EOF'
FormRejected [{ field: "", message: "Those dates cannot be booked." }]
EOF

# an empty from: the decoder refuses it, the message goes to the form list with the field's name
alder fill 'Rooms.Show."NewBooking refuses an empty from"' - <<'EOF'
{
  let me <- Db.insert User { email: Text.Email "email-1@example.com", name: Text.NonEmpty "name-1", passwordHash: "passwordhash-1" }
  let _ <- Test.login { user: Just me }
  let room1 <- Db.insert Room { name: Text.NonEmpty "name-1" }
  let _ <- Test.get (Rooms.Show { id: room1 })
  let r <- Test.submitRaw NewBooking [{ name: "from", value: "" }, { name: "to", value: "2026-01-01" }]
  Task.succeed (r.answer == FormRejected [{ field: "", message: "from must be a date (YYYY-MM-DD)" }])
}
EOF

# a stranger opening a booking gets a 404
alder fill 'Bookings.Show."pressing Cancel"' - <<'EOF'
{
  let me <- Db.insert User { email: Text.Email "email-1@example.com", name: Text.NonEmpty "name-1", passwordHash: "passwordhash-1" }
  let other <- Db.insert User { email: Text.Email "email-2@example.com", name: Text.NonEmpty "name-2", passwordHash: "passwordhash-2" }
  let room1 <- Db.insert Room { name: Text.NonEmpty "name-1" }
  let today <- Date.today
  let booking1 <- Db.insert Booking { room: room1, owner: me, from: Date.addDays 2 today, to: Date.addDays 4 today }
  let _ <- Test.login { user: Just other }
  let r <- Test.open (Bookings.Show { id: booking1 })
  Task.succeed (r.answer == PageMissing)
}
EOF

alder check
alder test
```
