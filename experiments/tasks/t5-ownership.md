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

Also write tests (`test "…" = …`, chapter 10) in the modules they test:
deleting another user's note, or a missing id, is not found; the owner can
delete. `alder test` must pass. Do not write tests that call the
`Password` capability.

Output every file, in full, each preceded by a line
`--- file: app/<path> ---`. No explanation, only files.
