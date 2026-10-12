# Tests asked only: t5-ownership

The calibration line for H1 (`findings.md` reads the baseline scores against it): the
reference's app code with only the tests the task's tests paragraph names, scored by
`alder test --mutate`.

The paragraph:

> Also write tests (`test "…" = …`, chapter 10) in the modules they test:
> deleting another user's note, or a missing id, is not found; the owner can
> delete. `alder test` must pass. Do not write tests that call the
> `Password` capability.

## How to reproduce

1. Copy `reference/t5-ownership/` to a scratch directory (drop this file and `SURVIVORS.md`).
2. Delete every `test` declaration in `app/` except the ones below, and add the ones below that are missing (same module, same text).
3. Run `alder test --mutate .` there.

## Score: 3/15 = 0.200

| Class | Survivors |
|---|---|
| authorization | 2 |
| data | 5 |
| flow | 4 |
| logic | 0 |
| schema | 1 |

By operator: redirect → rerender 3, session put dropped 2, requires dropped 2, reject → rerender 1, cascade ↔ restrict 1, where dropped 1, orderBy reversed 1, write replaced 1.

```text
3 passed, 0 failed
15 mutants: 3 killed, 12 survived  (score 0.20)
survived:
  App           update:15    session put dropped   let _ <- Session.put (\s -> { s | user: Nothing }) → let _ <- Task.succeed ()
  App           update:16    redirect → rerender   Redirect (Auth.Login {}) → Rerender
  Auth.Login    update:40    session put dropped   let _ <- Session.put (\s -> { s | user: Just id }) → let _ <- Task.succeed ()
  Auth.Login    update:41    redirect → rerender   Redirect (Notes.Index {}) → Rerender
  Auth.Login    update:43    reject → rerender     Reject "Email or password is incorrect." → Rerender
  Note          Note:12      cascade ↔ restrict    owner @ref User @restrict → owner @ref User @cascade
  Notes.Index   requires:7   requires dropped      requires user → served without user (user = User.UserId "mutant")
  Notes.Index   load:15      where dropped         Db.where (Note.owner.eq session.user) → the condition dropped
  Notes.Index   load:16      orderBy reversed      Db.orderBy Note.created.desc → Db.orderBy Note.created.asc
  Notes.Index   update:32    write replaced        let _ <- Db.insert Note {…} → let _ <- Task.succeed (Note.NoteId "mutant")
  Notes.Index   update:33    redirect → rerender   Redirect (Notes.Index {}) → Rerender
  Notes.Show    requires:7   requires dropped      requires user → served without user (user = User.UserId "mutant")
```

## The tests

`app/Notes/Index.alder`:

```alder
test "deleting another user's note is not found" = {
  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
  let other <- Db.insert User { email: Text.Email "other@example.com", name: Text.NonEmpty "Other", passwordHash: "" }
  let theirs <- Db.insert Note { title: Text.NonEmpty "Theirs", body: "", owner: other }
  let model <- load {} { user: me }
  let r <- Test.run (update (Delete { id: theirs }) model { user: me })
  let left <- Db.from Note |> Db.count
  Task.succeed ({ value: r.value, left: left } == { value: Err "not found", left: 1 })

test "deleting a missing note is not found" = {
  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
  let model <- load {} { user: me }
  let r <- Test.run (update (Delete { id: Note.NoteId "missing" }) model { user: me })
  Task.succeed (r.value == Err "not found")

test "the owner deletes a note through the page" = {
  let me <- Db.insert User { email: Text.Email "me@example.com", name: Text.NonEmpty "Me", passwordHash: "" }
  let id <- Db.insert Note { title: Text.NonEmpty "Milk", body: "", owner: me }
  let _ <- Test.login { user: Just me }
  let _ <- Test.get (Notes.Index {})
  let r <- Test.click (Delete { id: id })
  let left <- Db.from Note |> Db.count
  Task.succeed ({ answer: r.answer, items: Test.count "#notes li" r.doc, left: left } == { answer: Redirected (Notes.Index {}), items: 0, left: 0 })
```
