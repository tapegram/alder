# Task T2: a profile page

Add a route module to the example app with a GET route at `/profile` that:

- requires a logged-in user,
- shows the user's name and email,
- shows how many todos they have and how many are done,
- has a form to change the user's name (which must not be empty), posted
  by a write route of your choice, and
- has a button to log out (the `App` module already has `route Logout`,
  `App.Logout`; do not duplicate it — send it from the button as the
  language allows, or explain in a comment if you cannot).

Also add a link to the profile page from the to-do list (`Todos.Index`).

Also write tests (`test "…" = …`, chapter 10) in the modules they test: the
profile route's `load` counts only the user's todos; renaming to an empty
name is refused. `alder test` must pass. Do not call the real `Password`
adapter from a test; installing the fake with `Test.fake Password` is fine.

Output only the files you add or change: not the files you leave as they are,
not the standard library, nothing outside `app/`. Give each in full, preceded by
a line `--- file: app/<path> ---`. No explanation, only files.
