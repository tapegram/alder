# Task T4: a notes app with a theme switch

Write a complete new Alder app (not the todo app) for personal notes:

- A note has a title (required), a body (may be empty), and a created time.
  Notes belong to the logged-in user.
- `/` requires login and lists the user's notes newest first, each linking
  to its page, with a form to create a note.
- `/notes/{id}` requires login, shows one note, and lets the user edit it
  (title and body) or delete it. A user cannot see another user's note.
- The session stores a theme, `Light` or `Dark`, defaulting to `Light`.
  Every page shows a "Switch to dark" / "Switch to light" button that flips
  it, and the `<body>` has `class="dark"` when the theme is `Dark`.
- Every page also has a "Log out" button. Logging out works from any page.
- `/login`, an `App` module, a `Layout` module, and a `User` module as in
  the reference.

**Names.** Use exactly these names; anything not listed here is your choice.
The tables are exactly the ones below, with exactly these fields: add no
further tables or fields.

- `app/User.alder`: module `User`, `type UserId = brand Text`, and
  `table User { id: UserId @id, email: Text.Email @unique, name: Text.NonEmpty, passwordHash: Text, created: Time @created }`.
- `app/Theme.alder`: module `Theme`, `type Theme = Light | Dark`.
- `app/App.alder`: module `App`, with exactly
  `session { user: Maybe User.UserId, theme: Theme.Theme = Light }`,
  `login = Auth.Login.Login`, and two routes: `route SwitchTheme` (flips
  the session's theme and answers `Response.refresh`) and `route Logout`
  (sets `user` to `Nothing` and answers
  `Response.redirect (Auth.Login.Login {})`).
- `app/Note.alder`: module `Note`, `type NoteId = brand Text`, and
  `table Note { id: NoteId @id, title: Text.NonEmpty, body: Text, owner: User.UserId @ref User, created: Time @created }`.
- `app/Notes/Index.alder`: route module `Notes.Index`,
  `route Index = GET "/" { } requires user`.
  `form NewNote = { title: Text.NonEmpty, body: Text }`, posted by
  `route Add form NewNote`, which answers
  `Response.redirect (Notes.Index.Index {})`.
  The notes are listed in an element with `id="notes"`, one `<li>` per
  note; each note's link to its page has `class="title"` and the note's
  title as its whole text.
- `app/Notes/Show.alder`: route module `Notes.Show`,
  `route Show = GET "/notes/{id}" { id: Note.NoteId } requires user`.
  Another user's note, or a missing one, is a 404 (`Query.notFound`), and
  so is a `Save` or `Delete` of one (`Task.notFound`): nothing changes.
  The title is the whole text of an element with `id="title"`, the body of
  one with `id="body"`. `form EditNote = { title: Text.NonEmpty, body: Text }`,
  posted by `route Save { id: Note.NoteId } form EditNote`, answers
  `Response.redirect (Notes.Show.Show { id: … })` (the same note);
  `route Delete { id: Note.NoteId }` answers
  `Response.redirect (Notes.Index.Index {})`.
- `app/Auth/Login.alder`: route module `Auth.Login`,
  `route Login = GET "/login" { }`, with the `Password` capability and
  form `Credentials` as in the reference.
- Every page, `/login` included: the theme button has `id="theme"`, sends
  `App.SwitchTheme`, and its whole text is "Switch to dark" when the theme is
  `Light` and "Switch to light" when it is `Dark`; the "Log out" button sends
  `App.Logout`.

Also write tests (`test "…" = …`, chapter 10) in the modules they test: a
user cannot see another user's note (not found); the theme flips. `alder
test` must pass. Do not call the real `Password` adapter from a test;
installing the fake with `Test.fake Password` is fine.

Output every file, in full, each preceded by a line
`--- file: app/<path> ---`. No explanation, only files.
