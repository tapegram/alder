```alder-cli
# the cancel button is hidden from the booking's first day (replaces the test that
# clicked a Cancel button the page does not show for a booking from today)
alder fill --decl 'Bookings.Show."pressing Cancel"' - <<'EOF'
{
  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
  let room <- Db.insert Room { name: Text.NonEmpty "Oak" }
  let today <- Date.today
  let id <- Db.insert Booking { room: room, owner: me, from: today, to: Date.addDays 2 today }
  let _ <- Test.login { user: Just me }
  let page <- Test.get (Bookings.Show { id: id })
  Task.succeed (Test.has "#cancel" page.doc == False)
}
EOF

# the empty-from message is the decoder's, prefixed with the field name (the page has no
# <Errors field="from">)
alder fill --decl 'Rooms.Show."NewBooking refuses an empty from"' - <<'EOF'
{
  let me <- Db.insert User { email: Text.Email "email-1@example.com", name: Text.NonEmpty "name-1", passwordHash: "passwordhash-1" }
  let _ <- Test.login { user: Just me }
  let room1 <- Db.insert Room { name: Text.NonEmpty "name-1" }
  let _ <- Test.get (Rooms.Show { id: room1 })
  let r <- Test.submitRaw NewBooking [{ name: "from", value: "" }, { name: "to", value: "2026-01-01" }]
  Task.succeed (r.answer == FormRejected [{ field: "", message: "from must be a date (YYYY-MM-DD)" }])
}
EOF
```
