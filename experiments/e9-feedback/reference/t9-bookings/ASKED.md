# Tests asked only: t9-bookings

The calibration line for H1 (`findings.md` reads the baseline scores against it): the
reference's app code with only the tests the task's tests paragraph names, scored by
`alder test --mutate`.

The paragraph:

> Also write tests (`test "…" = …`, chapter 10) in the modules they test: a
> booking that overlaps another of the same room is refused and not stored,
> while one that starts on the other's `to` date is stored; the next free
> date of a room with back-to-back bookings; the owner cancels a booking
> before its `from` date; a booking whose `from` date is today cannot be
> cancelled. `alder test` must pass. Do not write tests that call the
> `Password` capability.

## How to reproduce

1. Copy `reference/t9-bookings/` to a scratch directory (drop this file and `SURVIVORS.md`).
2. Delete every `test` declaration in `app/` except the ones below, and add the ones below that are missing (same module, same text).
3. Run `alder test --mutate .` there.

## Score: 15/40 = 0.375

| Class | Survivors |
|---|---|
| authorization | 6 |
| data | 12 |
| flow | 4 |
| logic | 1 |
| schema | 2 |

By operator: where dropped 6, orderBy reversed 4, requires dropped 4, session put dropped 2, redirect → rerender 2, reject → rerender 2, cascade ↔ restrict 2, guard dropped 2, operator flipped 1.

```text
4 passed, 0 failed
40 mutants: 15 killed, 25 survived  (score 0.38)
survived:
  App              update:15     session put dropped   let _ <- Session.put (\s -> { s | user: Nothing }) → let _ <- Task.succeed ()
  App              update:16     redirect → rerender   Redirect (Auth.Login {}) → Rerender
  Auth.Login       update:40     session put dropped   let _ <- Session.put (\s -> { s | user: Just id }) → let _ <- Task.succeed ()
  Auth.Login       update:41     redirect → rerender   Redirect (Rooms.Index {}) → Rerender
  Auth.Login       update:43     reject → rerender     Reject "Email or password is incorrect." → Rerender
  Booking          Booking:11    cascade ↔ restrict    room @ref Room @restrict → room @ref Room @cascade
  Booking          Booking:12    cascade ↔ restrict    owner @ref User @restrict → owner @ref User @cascade
  Booking          clash:22      where dropped         Db.where (Booking.room.eq room) → the condition dropped
  Booking          clash:23      where dropped         Db.where (Booking.from.lt to) → the condition dropped
  Booking          nextFree:38   where dropped         Db.where (Booking.room.eq room) → the condition dropped
  Booking          nextFree:39   where dropped         Db.where (Booking.to.gt today) → the condition dropped
  Booking          nextFree:40   orderBy reversed      Db.orderBy Booking.to.asc → Db.orderBy Booking.to.desc
  Bookings.Index   requires:8    requires dropped      requires user → served without user (user = User.UserId "mutant")
  Bookings.Index   load:16       where dropped         Db.where (Booking.owner.eq session.user) → the condition dropped
  Bookings.Index   load:16       orderBy reversed      Db.orderBy Booking.from.asc → Db.orderBy Booking.from.desc
  Bookings.Show    requires:8    requires dropped      requires user → served without user (user = User.UserId "mutant")
  Bookings.Show    load:18       guard dropped         if b.owner == session.user then … else Query.notFound → the notFound arm dropped
  Bookings.Show    update:34     guard dropped         if model.cancellable then … else … → the fail arm dropped
  Rooms.Index      requires:8    requires dropped      requires user → served without user (user = User.UserId "mutant")
  Rooms.Index      load:16       orderBy reversed      Db.orderBy Room.name.asc → Db.orderBy Room.name.desc
  Rooms.Show       requires:8    requires dropped      requires user → served without user (user = User.UserId "mutant")
  Rooms.Show       load:17       where dropped         Db.where (Booking.room.eq room.id) → the condition dropped
  Rooms.Show       load:17       orderBy reversed      Db.orderBy Booking.from.asc → Db.orderBy Booking.from.desc
  Rooms.Show       update:34     operator flipped      p.form.to <= p.form.from → p.form.to < p.form.from
  Rooms.Show       update:34     reject → rerender     Reject "A booking must end after the day it starts." → Rerender
```

## The tests

`app/Bookings/Show.alder`:

```alder
test "the owner cancels a booking before its from date" = {
  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
  let blue <- Db.insert Room { name: Text.NonEmpty "Blue" }
  let today <- Date.today
  let id <- Db.insert Booking { room: blue, owner: me, from: Date.addDays 1 today, to: Date.addDays 2 today }
  let _ <- Test.login { user: Just me }
  let _ <- Test.get (Bookings.Show { id: id })
  let r <- Test.click Cancel
  let left <- Db.from Booking |> Db.count
  Task.succeed ({ answer: r.answer, left: left } == { answer: Redirected (Bookings.Index {}), left: 0 })

test "a booking whose from date is today cannot be cancelled" = {
  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
  let blue <- Db.insert Room { name: Text.NonEmpty "Blue" }
  let today <- Date.today
  let id <- Db.insert Booking { room: blue, owner: me, from: today, to: Date.addDays 1 today }
  let _ <- Test.login { user: Just me }
  let page <- Test.get (Bookings.Show { id: id })
  let left <- Db.from Booking |> Db.count
  Task.succeed ({ button: Test.has "#cancel" page.doc, left: left } == { button: False, left: 1 })
```

`app/Rooms/Index.alder`:

```alder
test "the next free date of a room with back-to-back bookings" = {
  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
  let blue <- Db.insert Room { name: Text.NonEmpty "Blue" }
  let today <- Date.today
  let _ <- Db.insert Booking { room: blue, owner: me, from: today, to: Date.addDays 2 today }
  let _ <- Db.insert Booking { room: blue, owner: me, from: Date.addDays 2 today, to: Date.addDays 3 today }
  let _ <- Test.login { user: Just me }
  let page <- Test.get (Rooms.Index {})
  Task.succeed (Test.texts "#rooms .free" page.doc == ["2026-01-04"])
```

`app/Rooms/Show.alder`:

```alder
test "an overlapping booking is refused and not stored; one starting on its to date is stored" = {
  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
  let blue <- Db.insert Room { name: Text.NonEmpty "Blue" }
  let today <- Date.today
  let _ <- Db.insert Booking { room: blue, owner: me, from: Date.addDays 5 today, to: Date.addDays 8 today }
  let _ <- Test.login { user: Just me }
  let _ <- Test.get (Rooms.Show { id: blue })
  let overlap <- Test.submit NewBooking { from: Date.addDays 7 today, to: Date.addDays 9 today }
  let refused <- Db.from Booking |> Db.count
  let touching <- Test.submit NewBooking { from: Date.addDays 8 today, to: Date.addDays 9 today }
  let stored <- Db.from Booking |> Db.count
  let rejected = match overlap.answer {
    FormRejected _ -> True,
    _ -> False,
  }
  Task.succeed ({ rejected: rejected, refused: refused, touching: touching.answer, stored: stored } == { rejected: True, refused: 1, touching: Redirected (Rooms.Show { id: blue }), stored: 2 })
```
