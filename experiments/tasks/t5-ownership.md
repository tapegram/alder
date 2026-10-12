# Task T5: shared notes, each user's own

Write a complete new Alder app (not the todo app) for notes shared by many
users on one server:

- Users log in with an email and a password, checked by a `Password`
  capability as in the example.
- A note has a title and a body, and belongs to the user who wrote it.
- `/` requires login and lists only the logged-in user's notes, each
  linking to its page, with a form to add a note.
- Messages: add a note, and delete a note by its id.
- Deleting a note that belongs to another user, or that does not exist,
  must answer 404 (`Task.notFound`).
- `/notes/{id}` requires login and shows one note. Another user's note is
  404 there too.
- `/login` and logout, an `App` module, a `Layout` module, and a `User`
  module as in the reference.

**Names.** Use exactly these names; anything not listed here is your choice.
The tables are exactly the ones below, with exactly these fields: add no
further tables or fields.

- `app/User.alder`: module `User`, `type UserId = brand Text`, and
  `table User { id: UserId @id, email: Text.Email @unique, name: Text.NonEmpty, passwordHash: Text, created: Time @created }`.
- `app/App.alder`: module `App`, with exactly
  `session { user: Maybe User.UserId }`, `login = Auth.Login`, and
  `msg Logout`, which answers `Redirect (Auth.Login {})`.
- `app/Note.alder`: module `Note`, `type NoteId = brand Text`, and
  `table Note { id: NoteId @id, title: Text.NonEmpty, body: Text, owner: User.UserId @ref User, created: Time @created }`.
- `app/Notes/Index.alder`: page module `Notes.Index`, `page "/" { } requires user`.
  `form NewNote = { title: Text.NonEmpty, body: Text }`, posted by
  `msg Add form NewNote`, which answers `Redirect (Notes.Index {})`.
  `msg Delete { id: Note.NoteId }`, sent by a "Delete" button beside each
  note, answers `Redirect (Notes.Index {})`. The notes are listed in an
  element with `id="notes"`, one `<li>` per note; each note's link to its
  page has `class="title"` and the note's title as its whole text.
- `app/Notes/Show.alder`: page module `Notes.Show`,
  `page "/notes/{id}" { id: Note.NoteId } requires user`. The title is the
  whole text of an element with `id="title"`, the body of one with
  `id="body"`.
- `app/Auth/Login.alder`: page module `Auth.Login`, `page "/login" { }`, with
  the `Password` capability, form `Credentials` and `msg Submit` as in the
  reference.

Also write tests (`test "…" = …`, chapter 10) in the modules they test:
deleting another user's note, or a missing id, is not found; the owner can
delete. `alder test` must pass. Do not call the real `Password` adapter from
a test; installing the fake with `Test.fake Password` is fine.

Output every file, in full, each preceded by a line
`--- file: app/<path> ---`. No explanation, only files.
