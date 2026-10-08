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

Output every file, in full, each preceded by a line
`--- file: app/<path> ---`. No explanation, only files.
